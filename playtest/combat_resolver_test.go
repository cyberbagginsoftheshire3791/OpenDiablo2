//go:build playtest

package playtest

import (
	"math"
	"testing"
)

// TestCombatResolver is M4.5 step 4's Constitution VI.2 script: the thirteenth.
//
// THE MILESTONE'S ASSERTION IN ONE SENTENCE: a blow does something, the game
// runs BOTH sides of the fight on the world clock, and a dog can take the
// player's health to zero and end the encounter saying so.
//
// It copies the twelfth's argument exactly, and the argument is the point:
// THERE IS NO START-A-FIGHT TOOL, NO SET-HEALTH TOOL, NO LAND-A-BLOW TOOL AND
// NO SET-ANIMATION TOOL. The only outcome a script can steer is the BAND, and
// even that leaves the resolver to do the rest. Every earlier milestone could
// be driven into looking right by a script calling the verb the game was
// supposed to call -- M4.1's placed light and M4.3b's awareness both shipped
// hollow that way -- so the acts below arrange a situation and then only read.
//
// WHAT THE SESSION THAT WROTE THIS MEASURED FIRST, because two of these acts
// are not what the step-4 brief specified and the reasons are findings:
//
//   - The player's max_health at seed 1462 is 240, not the 166 the brief
//     carried from M4.2. Act 1 reads it rather than trusting either number.
//   - strigoi_step_world(1.0) regularly advances ~1.67 world minutes and so
//     TWO rounds. The provider therefore reports every action of the last
//     Advance CALL, and act 2 compares a health delta against the whole
//     step's damage instead of one round's.
//   - A pursuer's route USED to end on the quarry's OWN tile, so every
//     participant in a settled fight floored to one tile and sampled one light
//     level, and dark-into-light could not fire at any placement. M4.5 ask 8
//     changed the router: a hunter now stops BESIDE its quarry. Act 7 still
//     asserts the DIAL PLUMBING rather than sweeping placements, because a
//     uniform night hands both tiles the same level -- proving the rule in a
//     playtest needs a PLACED source between two participants, which is its
//     own act. The rule itself stays proved in the unit tests with fakes.
func TestCombatResolver(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gaunt", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")
	playerHandle := str(p, "handle")
	playerID := str(p, "id")

	maxHealth := num(sub(p, "state"), "max_health")
	if maxHealth <= 0 {
		t.Fatalf("could not read the player's max_health: %v", p)
	}

	t.Logf("player %s at %.2f,%.2f with %.0f health", playerID, px, py, maxHealth)

	// --- act 1: a fight the GAME makes -------------------------------------
	spot := clearNeighbour(t, s, px, py)

	dog := spawnNPC(t, s, "fallen1", spot[0], spot[1])
	dogID := entityID(t, s, dog)

	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": playerHandle})

	fightNow(t, s)

	combat := combatState(s)

	if got := num(combat, "round"); got != 1 {
		t.Fatalf("act 1: opening the fight frame by frame must catch it on round 1; round=%.0f", got)
	}

	if got := str(combat, "initiator"); got != "enemy" {
		t.Fatalf("act 1: v0 has exactly one initiator and it is the enemy; got %q", got)
	}

	if flag(t, combat, "surprised") {
		t.Fatalf("act 1: an idle player is not caught head-down: %v", combat)
	}

	order, _ := combat["order"].([]any)
	if len(order) == 0 || str2(order[0]) != playerID {
		t.Fatalf("act 1: D8 puts the player's side first when he is not surprised; order=%v", order)
	}

	if got := participant(t, combat, dogID)["profile"]; got != "placeholder" {
		t.Fatalf("act 1: a harness-spawned dog has no spawn row and must say so; profile=%v", got)
	}

	t.Logf("act 1: encounter %v round %.0f order %v first_side=%v",
		combat["encounter"], num(combat, "round"), order, combat["first_side"])

	// THE LIVE ENEMY HAS A BAR (M4.4c-1 clause 7, the positive half). It is
	// asserted HERE so that act 4's "the corpse has none" is a control rather
	// than a claim: same entity, same script, one difference -- it died in
	// between. The fight has resolved a round against the dog by now, so its
	// body exists (Bodies.BodyOf adopts on the resolver's read).
	if b := barFor(t, uiState(s), dogID, false); b.fill <= 0 {
		t.Fatalf("act 1: the live dog's overhead bar reads empty (%+v) -- a living enemy's bar "+
			"must show its health", b)
	}

	t.Logf("act 1: the live dog has an overhead bar")

	// --- act 2: a blow does something, and the arithmetic is checkable -----
	//
	// THE DISCRIMINATING ASSERTION OF THE WHOLE SCRIPT. The test chooses the
	// formula -- max(1, base*factor) -- and the system reports base, band and
	// damage. A resolver wired to a constant, or one whose reported band does
	// not match the damage it actually did, fails here rather than passing
	// quietly. That is the same shape as the twelfth's act 4.
	dogBefore := num(participant(t, combat, dogID), "health")
	playerBefore := num(metersState(s), "health")

	var rows []map[string]any

	combat, rows = stepToNewRound(t, s)
	if len(rows) == 0 {
		t.Fatalf("act 2: a resolved round must report at least one blow: %v", combat)
	}

	dials := sub(combat, "dials")
	toDog, toPlayer := 0.0, 0.0

	for _, row := range rows {
		base, damage := num(row, "base"), num(row, "damage")
		band := str(row, "band")

		want := armouredDamage(t, row, playerID, bandFactor(t, dials, band))
		if damage != want {
			t.Fatalf("act 2: %s's %s of base %.0f must do %.0f damage, reported %.0f: %v",
				str(row, "attacker"), band, base, want, damage, row)
		}

		switch str(row, "attacker") {
		case playerID:
			if base < 12 || base > 20 {
				t.Fatalf("act 2: the player's placeholder melee profile is 12-20; drew %.0f: %v", base, row)
			}
		case dogID:
			if base < 2 || base > 5 {
				t.Fatalf("act 2: a profile-less enemy bites 2-5; drew %.0f: %v", base, row)
			}
		}

		switch str(row, "target") {
		case dogID:
			toDog += damage
		case playerID:
			toPlayer += damage
		}
	}

	// THE HEALTH A BLOW REPORTS IS THE HEALTH THE WORLD HOLDS, and it is
	// checked per blow rather than as a delta over the step. A step is 15 to
	// 24 frames and can resolve more than one round, so a delta would have to
	// know which rounds fell inside it; target_health_after is the blow's own
	// fact and needs no such alignment.
	if last, ok := lastAgainst(rows, dogID); ok {
		if got := mustNum(t, participant(t, combat, dogID), "health"); got != mustNum(t, last, "target_health_after") {
			t.Fatalf("act 2: the dog's last blow reported it at %.0f and the fight reports %.0f: %v",
				num(last, "target_health_after"), got, last)
		}
	}

	// YOU CAN BE HURT. Before step 4 the resolver could reach a wolf's health
	// and not the player's, so a fight could only ever go one way.
	last, hurt := lastAgainst(rows, playerID)
	if !hurt {
		t.Fatalf("act 2: the dog must be able to hurt the player; actions=%v", rows)
	}

	if got := mustNum(t, metersState(s), "health"); got != mustNum(t, last, "target_health_after") {
		t.Fatalf("act 2: the blow reported the player at %.0f and the METERS report %.0f -- the resolver "+
			"and the meters must be writing one field through one adapter: %v",
			num(last, "target_health_after"), got, last)
	}

	if num(metersState(s), "health") >= playerBefore {
		t.Fatalf("act 2: the player's health did not fall (was %.0f)", playerBefore)
	}

	if num(participant(t, combat, dogID), "health") >= dogBefore {
		t.Fatalf("act 2: the dog's health did not fall (was %.0f)", dogBefore)
	}

	t.Logf("act 2 PASS: %d blow(s), %.0f to the dog and %.0f to the player, every damage == max(1, base*factor)",
		len(rows), toDog, toPlayer)

	// --- act 3: a forced band, both ways ------------------------------------
	for _, band := range []string{"crit", "graze"} {
		s.call("strigoi_set_system_field", map[string]any{
			"system": "combat", "field": "forced_band", "value": band,
		})

		var forced []map[string]any

		combat, forced = stepToNewRound(t, s)

		if got := str(sub(combat, "dials"), "forced_band"); got != band {
			t.Fatalf("act 3: the dial must read back what was written; wrote %q got %q", band, got)
		}

		if len(forced) == 0 {
			t.Fatalf("act 3: no blows landed with forced_band=%q: %v", band, combat)
		}

		for _, row := range forced {
			if str(row, "band") != band {
				t.Fatalf("act 3: forced_band=%q must apply to every blow: %v", band, row)
			}

			want := armouredDamage(t, row, playerID, bandFactor(t, sub(combat, "dials"), band))
			if num(row, "damage") != want {
				t.Fatalf("act 3: %s damage must follow the forced band: %v", band, row)
			}
		}

		t.Logf("act 3: forced_band=%q applied to %d blow(s)", band, len(forced))
	}

	s.call("strigoi_set_system_field", map[string]any{
		"system": "combat", "field": "forced_band", "value": "",
	})

	// --- act 4: the swing is SEEN -------------------------------------------
	//
	// THE HELD-MODE MEASUREMENT, and it is the act that proves NPC.StartAction
	// rather than the resolver. Hold the player's side so only the dog acts,
	// then step FRAMES and read the dog's animation_mode out of the entity
	// provider. A1 must appear.
	//
	// If it never does, that is the measurement and not a flake: fail with the
	// frame log. The fix is the held flag, never a weaker assertion.
	// A FRESH, TOUGH ENEMY, because the dog is dead. Acts 2 and 3 spent about
	// 45 of its 61 hit points and act 3's crits finished it -- the first run
	// of this act read DT on all forty frames, which is a corpse, not a
	// broken animator. A zombie1 carries 181 and the player's side is HELD,
	// so nothing kills it while it is being watched.
	//
	// Re-opening the fight here also exercises tryStart's dead filter for
	// free: the dog's corpse is still noticed and still in reach, and the new
	// encounter must contain only the zombie.
	// THE CORPSE HAS NO BAR (M4.4c-1 clause 7, and the c-1 review's first
	// finding, 15 Sep 2026). The dog is dead here, and NOTHING in this build
	// removes a killed NPC from the map -- ActDie only plays the animation and
	// the corpse machine is M4.7 -- so without the health guard in
	// Game.OverheadBars its empty bar hangs over the ground until dawn, and
	// every kill adds another rect to the band night_render_test.go samples.
	// Act 1 asserted this same entity HAD a bar while it lived.
	dogMode := str(sub(s.call("strigoi_get_entity", map[string]any{"handle": dog}), "state"), "animation_mode")

	if b := barFor(t, uiState(s), dogID, true); b != nil {
		t.Fatalf("act 4: the dead dog %s (animation %s) still has an overhead bar %+v -- "+
			"a corpse must never be barred", dogID, dogMode, b)
	}

	t.Logf("act 4: the dead dog (animation %s) has no overhead bar; the live one in act 1 did", dogMode)

	setField(s, "combat", "player_action", "hold")

	wolf := spawnNPC(t, s, "zombie1", spot[0], spot[1])
	s.call("strigoi_watch", map[string]any{"watcher": wolf, "target": playerHandle})

	fightNow(t, s)

	modes := []string{}
	sawSwing := false

	for i := 0; i < 40 && !sawSwing; i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})

		mode := str(sub(s.call("strigoi_get_entity", map[string]any{"handle": wolf}), "state"), "animation_mode")
		modes = append(modes, mode)

		if mode == "A1" {
			sawSwing = true
		}
	}

	if !sawSwing {
		t.Fatalf("act 4: the zombie never played A1 across 40 frames of a fight it is acting in. "+
			"A mode set from outside is being overridden, or the animator is not wired. Frames: %v", modes)
	}

	t.Logf("act 4 PASS: the enemy swung (A1) and the held mode survived the tick; frames=%v", modes)

	// --- act 5: YOU CAN LOSE, and the game ends the fight ONCE --------------
	//
	// THE MILESTONE'S SENTENCE. Three more dogs, the player's side still held,
	// and the clock stepped until he is dead. Then five more minutes to prove
	// the corpse does not restart it -- without tryStart's dead filter, the
	// frame after player_dead opens a fresh encounter against a 0-HP player
	// and every counter below climbs once a round for the rest of the night.
	for i := 0; i < 3; i++ {
		extra := spawnNPC(t, s, "fallen1", spot[0], spot[1])
		s.call("strigoi_watch", map[string]any{"watcher": extra, "target": playerHandle})
	}

	// DISENGAGE AND LET THE FIGHT RE-OPEN, and the reason is a finding this
	// act turned up rather than a trick.
	//
	// AT STEP 4 tryStart built the participant list ONCE and pruneOrEnd only
	// ever REMOVED from it, so a monster that arrived after a fight started
	// never joined it. The first run of this act spent ninety world minutes
	// with three fresh dogs standing next to the player and took him from 240
	// to 2, because only the one enemy present when the encounter opened was
	// ever biting. Step 5's reinforce() closed that. The disengage dial stays
	// because it is still the shortest way to put everyone in one fight
	// deliberately rather than waiting for arrivals.
	setField(s, "combat", "disengage", true)
	fightNow(t, s)

	var elapsed int

	for i := 0; i < 120; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1.0})

		elapsed++

		if !flag(t, combatState(s), "fighting") && mustNum(t, metersState(s), "health") <= 0 {
			break
		}
	}

	combat = combatState(s)
	meters := metersState(s)

	// mustNum, not num: a renamed health field reads 0 here, which IS "dead",
	// so this whole act would pass while measuring nothing.
	if mustNum(t, meters, "health") > 0 {
		t.Fatalf("act 5: the pack failed to kill the player in %d world minutes (health %.0f of %.0f). "+
			"Either the player's side is not being hurt, or the placeholder profiles are too kind.",
			elapsed, num(meters, "health"), maxHealth)
	}

	if got := str(combat, "ended_reason"); got != "player_dead" {
		t.Fatalf("act 5: the encounter must end saying why; ended_reason=%q: %v", got, combat)
	}

	if !flag(t, meters, "dead") {
		t.Fatalf("act 5: the meters and the resolver must agree the player is dead: %v", meters)
	}

	// Both baselines are strict, because the assertion below is a NEGATIVE one
	// -- "these two numbers did not move" -- and a renamed field would make it
	// 0 == 0 on both sides.
	encountersAfterDeath := mustNum(t, combat, "encounters")
	deathsAfterDeath := mustNum(t, combat, "ended_player_dead")

	if deathsAfterDeath != 1 {
		t.Fatalf("act 5: the player died once; ended_player_dead=%.0f", deathsAfterDeath)
	}

	for i := 0; i < 5; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1.0})
	}

	combat = combatState(s)

	if mustNum(t, combat, "encounters") != encountersAfterDeath ||
		mustNum(t, combat, "ended_player_dead") != deathsAfterDeath {
		t.Fatalf("act 5: THE DEAD RESTARTED THE FIGHT. encounters %.0f -> %.0f, ended_player_dead %.0f -> %.0f. "+
			"tryStart must skip a quarry whose body reads 0, and since step 5 a death also withdraws it "+
			"from the notice model and the chase.",
			encountersAfterDeath, num(combat, "encounters"), deathsAfterDeath, num(combat, "ended_player_dead"))
	}

	if flag(t, combat, "fighting") {
		t.Fatalf("act 5: a dead player is not in a fight: %v", combat)
	}

	t.Logf("act 5 PASS: the GAME killed the player in ~%d world minutes with the harness silent on every "+
		"verb that matters, ended the encounter once as player_dead, and the corpse did not restart it",
		elapsed)
}

// TestCombatResolverStanceAndReactions is the second half of the thirteenth
// script. It is a separate function because acts 1-5 end with the player dead,
// and every branch below needs a live one.
//
// ACT 7 IS NOT WHAT THE STEP-4 BRIEF SPECIFIED, and the change is a finding
// rather than a convenience. The brief had this act SWEEP placements around a
// hearth until two adjacent tiles straddled lit_level. At step 4 no placement
// could: a pursuer's route ended on the quarry's own tile, so it walked to
// distance 0.000 and every participant floored to ONE tile and read ONE light
// level. The sweep would have failed six times for a reason that has nothing
// to do with light.
//
// M4.5 ASK 8 REMOVED THAT REASON: a pursuer stops on the best free NEIGHBOUR,
// so act 7a asserts THAT premise directly rather than inferring it from the
// light. The sweep is still not restored, because at night the level is
// uniform and both tiles read the same -- restoring it needs a PLACED source
// between two participants, which is its own act and its own burst. So act 7
// asserts the dial plumbing, and 7b REPORTS the levels rather than asserting
// them: the day they differ, that log line is the news.
func TestCombatResolverStanceAndReactions(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gaunt", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")
	playerHandle, playerID := str(p, "handle"), str(p, "id")
	spot := clearNeighbour(t, s, px, py)

	// --- act 6: clause 5 of the DoD, BOTH DIRECTIONS ------------------------
	//
	// "At fatigue >= 75 the resolver offers no Reaction" (M4.2 ask 1, signed).
	// The player's side is HELD so the dog survives -- under `attack` a 61-HP
	// dog dies in about four rounds and the window closes before the control
	// has run. forced_band graze makes every enemy blow the miss a Riposte
	// answers.
	dog := spawnNPC(t, s, "fallen1", spot[0], spot[1])
	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": playerHandle})

	fightNow(t, s)

	setField(s, "combat", "player_action", "hold")
	setField(s, "combat", "forced_band", "graze")
	setField(s, "meters", "fatigue", 80)

	if flag(t, metersState(s), "reaction_available") {
		t.Fatalf("act 6: fatigue 80 must put the Reaction out of reach: %v", metersState(s))
	}

	grazes, ripostes := stepAndCount(t, s, 10, playerID)
	if grazes == 0 {
		t.Fatalf("act 6: the control is empty -- no enemy graze landed in 10 world minutes to be answered")
	}

	if ripostes != 0 {
		t.Fatalf("act 6: no Reaction is available at fatigue 80, so %d riposte(s) is too many", ripostes)
	}

	t.Logf("act 6a: %d graze(s) on the player at fatigue 80 and no riposte", grazes)

	setField(s, "meters", "fatigue", 10)

	grazes, ripostes = stepAndCount(t, s, 10, playerID)
	if grazes == 0 {
		t.Fatalf("act 6: the positive case is empty -- no enemy graze landed to be answered")
	}

	if ripostes == 0 {
		t.Fatalf("act 6: at fatigue 10 a graze on the player must buy a Riposte; %d graze(s) bought none", grazes)
	}

	t.Logf("act 6b PASS: at fatigue 10 the same kind of graze bought %d riposte(s) -- clause 5 both ways",
		ripostes)

	// SHAKEN'S ACCURACY PENALTY, briefly, under `attack`. What a playtest can
	// see of Shaken is the penalty on the player's own blows: "Shaken blocks
	// Riposte" is unobservable in a shipped build, because ShakenFatigue (90)
	// already exceeds NoReactionFatigue (75), and it lives in the unit tests.
	setField(s, "meters", "fatigue", 95)
	setField(s, "combat", "player_action", "attack")
	setField(s, "combat", "forced_band", "")

	// A FRESH, TOUGH ENEMY AND A FRESH FIGHT. The dog has been grazed and
	// riposted for twenty rounds and does not survive being switched to
	// `attack` -- the first run of this act found no new round at all,
	// because there was nothing left to fight. A zombie1 carries 181.
	tough := spawnNPC(t, s, "zombie1", spot[0], spot[1])
	s.call("strigoi_watch", map[string]any{"watcher": tough, "target": playerHandle})

	if flag(t, combatState(s), "fighting") {
		setField(s, "combat", "disengage", true)
	}

	fightNow(t, s)

	if !flag(t, metersState(s), "shaken") {
		t.Fatalf("act 6: fatigue 95 must be Shaken: %v", metersState(s))
	}

	// A NEW round, not the retained one: the log survives between rounds, so
	// reading straight after the write can measure a blow struck before it.
	// The first run of this act "found" a Riposte from round 22 that happened
	// long before the fatigue was raised.
	_, shakenRows := stepToNewRound(t, s)

	sawShaken := false

	for _, row := range shakenRows {
		if str(row, "attacker") != playerID {
			continue
		}

		sawShaken = true

		if str(row, "advantage_why") != "shaken" || num(row, "mod") > -15 {
			t.Fatalf("act 6: a Shaken player's blow carries the reason and the penalty: %v", row)
		}
	}

	if !sawShaken {
		t.Fatalf("act 6: the player took no action to be penalised")
	}

	t.Logf("act 6c PASS: Shaken costs the player accuracy and says so")

	// --- act 7: the light dials, and WHY the rule cannot fire ---------------
	combat := combatState(s)

	levels := map[string]float64{}

	parts, _ := combat["participants"].([]any)
	for _, raw := range parts {
		row, _ := raw.(map[string]any)
		levels[str(row, "id")] = num(row, "light_here")
	}

	// THE CONTROL FOR THIS CONTROL. num() is fail-open: a missing light_here
	// reads as 0, so with no participants, or with the light model unwired,
	// "everyone shares one level" would pass while measuring nothing. Both
	// preconditions are asserted before the comparison is believed.
	if len(parts) < 2 {
		t.Fatalf("act 7: need both sides in the fight to compare their light; got %v", parts)
	}

	for _, raw := range parts {
		row, _ := raw.(map[string]any)
		if _, ok := row["light_here"]; !ok {
			t.Fatalf("act 7: a participant reports no light_here at all, so this act measures nothing: %v", row)
		}
	}

	var (
		first float64
		seen  bool
		same  = true
	)

	for _, v := range levels {
		if !seen {
			first, seen = v, true
		}

		if v != first {
			same = false
		}
	}

	// THE TRIPWIRE FIRED, AND THIS IS WHAT IT LOOKS LIKE ON THE OTHER SIDE.
	// It used to fail here if participants stopped sharing a light level,
	// because that could only mean the router had changed. The router HAS
	// changed -- the pursuer-adjacent burst made a hunter stop BESIDE its
	// quarry instead of on top of it -- so the premise is now asserted
	// directly rather than inferred from the light.
	tiles := map[string][2]int{}

	for _, raw := range parts {
		row, _ := raw.(map[string]any)
		tiles[str(row, "id")] = [2]int{
			int(math.Floor(num(row, "x"))), int(math.Floor(num(row, "y"))),
		}
	}

	playerTile, ok := tiles[playerID]
	if !ok {
		t.Fatalf("act 7: the player must be a participant to compare tiles: %v", tiles)
	}

	for id, tile := range tiles {
		if id == playerID {
			continue
		}

		dx, dy := tile[0]-playerTile[0], tile[1]-playerTile[1]
		if dx == 0 && dy == 0 {
			t.Fatalf("act 7: %s is standing ON the player's tile (%v). Since the pursuer-adjacent "+
				"burst a hunter routes to the best free NEIGHBOUR, so this must not happen: "+
				"either the router regressed or Route's neighbour attempt is being skipped.", id, tile)
		}

		if abs(dx) > 1 || abs(dy) > 1 {
			t.Fatalf("act 7: %s is at %v against the player's %v -- adjacent means the eight "+
				"neighbours, and a fight is only supposed to open in reach", id, tile, playerTile)
		}
	}

	t.Logf("act 7a PASS: every enemy stands BESIDE the player rather than on him (%v vs %v) -- "+
		"the one change that lets dark-into-light, flanking and facing ever fire", tiles, playerTile)

	if same {
		t.Logf("act 7b: every participant still reads light %.4f. They are on DIFFERENT tiles now, "+
			"so this is the night being uniformly dark rather than the old one-tile collapse. "+
			"Proving dark-into-light in a playtest now needs a PLACED source between them, which "+
			"is its own act and its own burst.", first)
	} else {
		t.Logf("act 7b: participants read different light levels (%v) -- dark-into-light is "+
			"observable in a real build for the first time. The step-4 brief's sweep is now "+
			"worth restoring.", levels)
	}

	for _, level := range []float64{0.01, 0.99, 0.30} {
		setField(s, "combat", "lit_level", level)

		if got := num(sub(combatState(s), "dials"), "lit_level"); got != level {
			t.Fatalf("act 7: lit_level must read back what was written; wrote %v got %v", level, got)
		}
	}

	t.Logf("act 7 PASS: the dial moves in both directions, and every participant reads its OWN tile "+
		"(first reading %.4f) -- the rule itself is proved in the unit tests until a placed source "+
		"makes two of those readings differ", first)
}

// TestCombatResolverCaughtHeadDown is D8 §9's branch and what a corpse does
// afterwards -- acts 8 and 9 of the thirteenth script.
//
// D8 was SIGNED on 1 September and this is the first code to build it. The
// branch is live in the model and harness-only in the game, and will stay so
// until a forage or watch VERB exists (M4.4's turn UI): there is no symbol to
// register for "a branch nothing in the game reaches", so it is written here,
// in combat.go's doc comment, and in docs/reachability.md.
func TestCombatResolverCaughtHeadDown(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gaunt", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")
	playerHandle, playerID := str(p, "handle"), str(p, "id")
	spot := clearNeighbour(t, s, px, py)

	// The stance is set BEFORE anything is placed: the resolver reads it once,
	// at the moment the fight opens, and the game sets `labour` only after
	// combat.Advance returns. If those two ever swap, every fight becomes a
	// surprise and this act is what says so.
	setField(s, "meters", "activity", "forage")

	dog := spawnNPC(t, s, "fallen1", spot[0], spot[1])
	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": playerHandle})

	fightNow(t, s)

	combat := combatState(s)

	if !flag(t, combat, "surprised") || str(combat, "surprise_why") != "caught-foraging" {
		t.Fatalf("act 8: a player caught foraging is surprised and says why: %v", combat)
	}

	if got := str(combat, "first_side"); got != "enemy" {
		t.Fatalf("act 8: the pack acts first in a surprised round one; first_side=%q", got)
	}

	order, _ := combat["order"].([]any)
	if len(order) == 0 || str2(order[len(order)-1]) != playerID {
		t.Fatalf("act 8: the caught player goes LAST in round one; order=%v", order)
	}

	// §4.6.3 being seen: the GAME, not the script, put the meters on labour.
	if got := str(metersState(s), "activity"); got != "labour" {
		t.Fatalf("act 8: fighting is labour -- S1 §5's signed drain finally has a consumer; activity=%q", got)
	}

	t.Logf("act 8 PASS: caught foraging -- enemy first, player last in the order, meters on labour")

	bodiesBefore := mustNum(t, combat, "bodies_known")

	setField(s, "combat", "forced_band", "crit")

	for i := 0; i < 30 && flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1.0})
	}

	combat = combatState(s)

	if got := str(combat, "ended_reason"); got != "enemies_dead" {
		t.Fatalf("act 9: the dog should be dead and the fight over; ended_reason=%q: %v", got, combat)
	}

	if got := str(metersState(s), "activity"); got != "forage" {
		t.Fatalf("act 9: when the fight ends the body goes back to what it was doing; activity=%q", got)
	}

	if got := mustNum(t, combat, "bodies_known"); got != bodiesBefore {
		t.Fatalf("act 9: bodies_known counts NPC bodies and nothing left the registry; %.0f -> %.0f",
			bodiesBefore, got)
	}

	encounters := mustNum(t, combat, "encounters")

	for i := 0; i < 5; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1.0})
	}

	if got := mustNum(t, combatState(s), "encounters"); got != encounters {
		t.Fatalf("act 9: THE CORPSE RESTARTED THE FIGHT; encounters %.0f -> %.0f. Since step 5 a death "+
			"withdraws the dead thing from the notice model AND the chase, and tryStart must still "+
			"skip a watcher whose body reads 0.", encounters, got)
	}

	// The fence held: nothing was despawned. The dog is still on the map, in
	// its spawn group, holding a corpse's animation -- withdrawn from the
	// notice model and the chase since step 5, but never removed.
	mode := str(sub(s.call("strigoi_get_entity", map[string]any{"handle": dog}), "state"), "animation_mode")
	if mode != "DT" && mode != "DD" {
		t.Fatalf("act 9: a dead dog holds DT or DD; animation_mode=%q", mode)
	}

	t.Logf("act 9 PASS: the dog is dead and still on the map in %s, bodies_known unchanged at %.0f, "+
		"the activity went back to forage, and five further minutes started no new fight",
		mode, bodiesBefore)
}

// --- helpers ---------------------------------------------------------------

// clearNeighbour finds one of the four ORTHOGONAL neighbours of the player
// with a clear line, and fails if there is none.
//
// Orthogonal on purpose. It was chosen when Pursuit's ArriveWithin was 1.0,
// which a diagonal neighbour's 1.41 tiles by Euclid sat OUTSIDE, so a diagonal
// took a different path into the fight. Ask 8 moved ArriveWithin to 1.5 and
// both readings now arrive, so this is conservative rather than necessary --
// Combat's own reach is Chebyshev and treats the two alike either way.
//
// Seed 1462 draws a GENERATED map, so the bearing is swept rather than
// hardcoded -- a fixed offset is a test that passes for one worldgen.
func clearNeighbour(t *testing.T, s *session, px, py float64) [2]float64 {
	t.Helper()

	for _, d := range [][2]float64{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
		x, y := px+d[0], py+d[1]
		if flag(t, s.call("strigoi_find_path", map[string]any{"to_x": x, "to_y": y}), "straight_line_clear") {
			return [2]float64{x, y}
		}
	}

	t.Fatalf("no orthogonal neighbour of %.2f,%.2f has a clear line; cannot arrange a fight", px, py)

	return [2]float64{}
}

// fightNow steps FRAMES until the game opens an encounter, and stopping on a
// frame rather than on a world minute is the point.
//
// advanceWorld runs once per frame, so one stepped world minute is 15 to 24
// calls into the resolver and can carry a fight several rounds past its start.
// A single Advance either OPENS an encounter or ticks one, never both, so the
// frame this returns on is round 1 with nothing yet resolved -- which is the
// only moment D8 section 9's surprised round one can be observed at all.
//
// It loops because the notice model re-evaluates on its own cadence
// (ReEvaluateMinutes) and need not fire on the first frame; the twelfth script
// loops for the same reason, in coarser units.
func fightNow(t *testing.T, s *session) {
	t.Helper()

	for i := 0; i < 150; i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})

		if flag(t, combatState(s), "fighting") {
			return
		}
	}

	t.Fatalf("an aware monster one tile away never became a fight in 150 frames: %v", combatState(s))
}

// setField writes one provider field and fails on a refusal.
func setField(s *session, system, field string, value any) {
	s.call("strigoi_set_system_field", map[string]any{
		"system": system, "field": field, "value": value,
	})
}

func metersState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "meters"}), "state")
}

// actionRows is the blows of the most recent Advance call.
func actionRows(t *testing.T, combat map[string]any) []map[string]any {
	t.Helper()

	raw, ok := combat["actions"].([]any)
	if !ok {
		t.Fatalf("the combat provider must report an actions log; got %v", combat["actions"])
	}

	out := make([]map[string]any, 0, len(raw))

	for _, r := range raw {
		if row, ok := r.(map[string]any); ok {
			out = append(out, row)
		}
	}

	return out
}

// participant finds one combatant's row by id.
func participant(t *testing.T, combat map[string]any, id string) map[string]any {
	t.Helper()

	parts, ok := combat["participants"].([]any)
	if !ok {
		t.Fatalf("the combat provider must report participants; got %v", combat["participants"])
	}

	for _, raw := range parts {
		row, ok := raw.(map[string]any)
		if ok && str(row, "id") == id {
			return row
		}
	}

	t.Fatalf("%s is not a participant: %v", id, parts)

	return nil
}

// stepAndCount steps world minutes and counts enemy grazes on the player and
// the ripostes they bought. Both halves matter: a Riposte assertion with no
// graze behind it is a control that measured nothing.
func stepAndCount(t *testing.T, s *session, minutes int, playerID string) (grazes, ripostes int) {
	t.Helper()

	// COUNT EACH ROUND ONCE, AND NEVER THE ONE ALREADY ON THE BOARD. The log
	// holds the last RESOLVED round and is retained across the frames between
	// rounds, so two consecutive steps can show the same round twice -- and
	// the round showing when this is CALLED belongs to whatever came before,
	// which for act 6b is act 6a's last round, from before the fatigue write.
	// Seeding from the current value is what keeps the two halves separate.
	seen := int(num(combatState(s), "actions_round"))

	for i := 0; i < minutes; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1.0})

		combat := combatState(s)

		round := int(num(combat, "actions_round"))
		if round == seen {
			continue
		}

		seen = round

		for _, row := range actionRows(t, combat) {
			if str(row, "target") == playerID && str(row, "band") == "graze" {
				grazes++
			}

			if str(row, "attacker") == playerID && str(row, "reaction") == "riposte" {
				ripostes++
			}
		}
	}

	return grazes, ripostes
}

// lastAgainst is the most recent blow in the log that landed on one target.
func lastAgainst(rows []map[string]any, id string) (map[string]any, bool) {
	for i := len(rows) - 1; i >= 0; i-- {
		if str(rows[i], "target") == id {
			return rows[i], true
		}
	}

	return nil, false
}

// bandFactor reads the factor the SYSTEM says it used for a band, so the
// arithmetic below is checked against the live dial rather than a constant the
// script also chose.
func bandFactor(t *testing.T, dials map[string]any, band string) float64 {
	t.Helper()

	switch band {
	case "graze":
		return num(dials, "graze_factor")
	case "hit":
		return num(dials, "hit_factor")
	case "crit":
		return num(dials, "crit_factor")
	}

	t.Fatalf("no such band %q; dials=%v", band, dials)

	return 0
}

// armouredDamage is expectedDamage after his mail (T2, 23 Sep 2026): a blow ON
// the player loses what the mail absorbs, and the absorption is checked against
// the number the TEST knows from data/strigoi/items.json -- the default kit's
// mail takes 4 off a cut and 2 off a thrust -- rather than trusted from the
// row. A blow BY the player absorbs nothing, because no enemy wears armour.
func armouredDamage(t *testing.T, row map[string]any, playerID string, factor float64) float64 {
	t.Helper()

	absorbed := mustNum(t, row, "absorbed")

	if str(row, "target") != playerID {
		if absorbed != 0 {
			t.Fatalf("no enemy wears armour, yet a blow on one absorbed %.0f: %v", absorbed, row)
		}

		return expectedDamage(mustNum(t, row, "base"), factor)
	}

	// Sound, damaged (past half its points: the reduction halves) or ruined
	// (none) -- three values and no others, because a long script wears it.
	mail := map[string]float64{"cut": 4, "thrust": 2, "blunt": 1}[mustStr(t, row, "damage_class")]
	if absorbed != mail && absorbed != math.Round(mail/2) && absorbed != 0 {
		t.Fatalf("his mail takes %.0f off a %s (or half, worn, or none, ruined); the row says %.0f: %v",
			mail, str(row, "damage_class"), absorbed, row)
	}

	if d := float64(int(mustNum(t, row, "base")*factor)) - absorbed; d > 1 {
		return d
	}

	return 1
}

// expectedDamage is the formula the TEST chooses: max(1, base*factor).
func expectedDamage(base, factor float64) float64 {
	if d := float64(int(base * factor)); d > 1 {
		return d
	}

	return 1
}

// stepToNewRound steps world minutes until a NEW round's blows appear, and
// returns the state and that round's actions.
//
// THE LOG IS RETAINED BETWEEN ROUNDS, deliberately -- advanceWorld runs once
// per frame, so a log cleared per call would be empty on almost every read.
// The cost of that choice is that a script which writes a dial and then reads
// once can be looking at a round that resolved BEFORE its write took effect.
// actions_round is what tells them apart, and this is the helper that uses it.
// Act 6 found the failure the hard way: it asserted on a Riposte from round 22
// that had happened long before the fatigue it was testing was ever set.
func stepToNewRound(t *testing.T, s *session) (map[string]any, []map[string]any) {
	t.Helper()

	before := int(num(combatState(s), "actions_round"))

	for i := 0; i < 12; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1.0})

		combat := combatState(s)
		if int(num(combat, "actions_round")) != before {
			return combat, actionRows(t, combat)
		}
	}

	t.Fatalf("no new round resolved in 12 world minutes: %v", combatState(s))

	return nil, nil
}

// abs is the integer absolute value the tile comparisons in act 7 need; math's
// is float-only and a tile index is not a float.
func abs(n int) int {
	if n < 0 {
		return -n
	}

	return n
}
