//go:build playtest

package playtest

import (
	"fmt"
	"math"
	"testing"
)

// TestCombatRout is M4.5 step 5's Constitution VI.2 script: the fourteenth.
//
// THE STEP'S ASSERTION IN ONE SENTENCE: a pack that is losing BREAKS, the game
// is what breaks it, and a fight the player survived because they broke is not
// reported as a fight in which he killed them all.
//
// It is written against a REAL SPAWN-TABLE PACK rather than a harness-spawned
// stand-in, and that is the whole difficulty of the script. Rout is a fact
// about a GROUP -- N1 §5 writes every rout phrasing about packs, and the
// decrement scales with the pack's starting size -- and a monster placed by
// strigoi_spawn_entity belongs to no group at all. So the arrival is forced
// the only way the harness allows: the chance dial goes to certainty and the
// real table fires. Forcing the table is evidence about the game; a spawn verb
// would be evidence about the verb, which is why there is no spawn verb.
//
// WHAT THE SCRIPT MAY NOT DO, and the list is the argument:
//   - it never writes morale. Act A's whole point is that the number falls
//     because the resolver hurt the pack. `morale` is a settable field and
//     using it here would prove nothing.
//   - it never calls strigoi_pursue or the release verb. Act C's point is
//     that a death ends the chase by itself.
//   - it never starts a fight. There is no such verb.
//
// The two dials it DOES write are loss_weight and quick_resolve_advantage, and
// both are written in BOTH directions so the assertion is an assertion: act B
// turns rout off and shows the number stop moving, then turns it back on and
// shows it move again.
func TestCombatRout(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gaunt", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	p := s.call("strigoi_get_player", map[string]any{})
	playerHandle := str(p, "handle")
	playerID := str(p, "id")

	t.Logf("player %s (%s)", playerID, playerHandle)

	combat := combatState(s)
	for _, k := range []string{"has_morale", "has_chases"} {
		if !flag(t, combat, k) {
			t.Fatalf("the step's two new seams must be wired in a real build; %s=false: %v", k, combat)
		}
	}

	t.Logf("seams: has_morale=%t has_chases=%t has_profiles=%t",
		flag(t, combat, "has_morale"), flag(t, combat, "has_chases"),
		flag(t, combat, "has_profiles"))

	// --- getting a real pack into a real fight -------------------------------
	//
	// MEASURED FIRST, AND IT CHANGED THIS SETUP. A first run of this script
	// waited for a fight by stepping world minutes and found none, while the
	// counters showed FIVE encounters had started and ended between reads --
	// the same "invisible between two harness reads" problem the combat
	// counters were added for. It also showed something the brief did not
	// anticipate: in a real build the pack does NOT arrive together. Members
	// trickle into reach one at a time, so a fight is usually against one
	// member of a group of four, and a rout has nobody left to break.
	//
	// So the setup does three things. It gives the player a torch, because at
	// deep night an unlit player is not reliably seen at all. It holds the
	// player's action, so a fight that opens STAYS open and the rest of the
	// pack has time to walk in -- which is what makes the reinforcement path
	// part of the setup rather than a separate act. And it forces grazes
	// while holding, so the waiting does not kill him.
	for i := 0; i < 40 && str(spawnsState(s), "stage") != "night"; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 60})
	}

	if stage := str(spawnsState(s), "stage"); stage != "night" {
		t.Fatalf("never reached the deep night, stage=%q", stage)
	}

	setField(s, "spawns", "chance", 100)
	setField(s, "spawns", "notice_radius", 24)
	setField(s, "light", "carried_source", "torch")
	setField(s, "combat", "player_action", "hold")
	setField(s, "combat", "forced_band", "graze")

	// MEASURED, AND BOTH OF THESE ARE THE MEASUREMENT SPEAKING. Three earlier
	// runs of this script produced NINE fights and every one of them was one
	// enemy against the player, ending enemies_dead with joined:0. Two facts
	// made that inevitable: at AdjacentTiles 1 only what is already on top of
	// the player counts as in reach, so a pack that arrives strung out over
	// twenty tiles arrives one at a time; and a held player still RIPOSTES,
	// which killed each arrival before the next one closed.
	//
	// So the reach dial is widened -- it is a dial, and widening it is how a
	// script proves anything about a pack -- and the player's fatigue is put
	// past NoReactionFatigue so he has no Reaction to spend. Both are read
	// back below rather than assumed.
	setField(s, "combat", "adjacent_tiles", 10)
	setField(s, "meters", "fatigue", 90)

	// And the pack must not win while it is still gathering. A graze already
	// floors at 1 damage, so this makes every blow of the waiting phase cost
	// exactly that -- the smallest wound the model allows, which is R2 §3
	// bullet 5's "no binary miss" seen from the useful end.
	setField(s, "combat", "graze_factor", 0.01)

	// adjacent_tiles is reported at the TOP level, not in the dials block --
	// reading it from the wrong one silently yields 0, which is what the
	// first run of this check did.
	if got := num(combatState(s), "adjacent_tiles"); got != 10 {
		t.Fatalf("the reach dial did not take: %v", got)
	}

	if flag(t, combatState(s), "has_fitness") && num(metersState(s), "fatigue") < 75 {
		t.Fatalf("fatigue must be past the no-reaction threshold, got %.0f", num(metersState(s), "fatigue"))
	}

	group, groupID, inFight := walkToAPack(t, s, 2)

	spawned := num(group, "spawned")
	moraleAtArrival := num(group, "morale")

	if spawned < 1 || moraleAtArrival <= 0 {
		t.Fatalf("the pack must arrive with members and nerve: %v", group)
	}

	t.Logf("fight opened against %s (%s): %d of %d in reach, morale %.0f",
		groupID, str(group, "row"), inFight, int(spawned), moraleAtArrival)

	// Held now, because a rout ENDS the fight and takes the participant list
	// with it -- acts C and F read these ids afterwards.
	packIDs := enemyIDsOf(t, s, groupID)
	if len(packIDs) < 2 {
		t.Fatalf("expected at least two of %s in the fight, got %v", groupID, packIDs)
	}

	// ended_enemies_dead is CUMULATIVE, and the difference is the assertion.
	// Fights happen while the player is walking to the pack, so the counter
	// is already several by the time this one opens; what act F proves is
	// that THIS fight did not add to it.
	deadEndingsBefore := num(combatState(s), "ended_enemies_dead")

	// --- act B first: the dial OFF, and the number does not move -------------
	//
	// Run before act A deliberately. A control that runs after the effect has
	// already fired is a control against a changed world; this one runs while
	// the pack is whole.
	//
	// IT NEEDS THREE OF THE PACK IN THE FIGHT, because it spends one of them:
	// with two, act A would kill the last member and there would be nobody
	// left to break. When the night only ever delivers two, the act is
	// SKIPPED and said so, and the negative control is the unit test that
	// drives the same dial to zero against a pack of four.
	setField(s, "combat", "quick_resolve_advantage", 5) // out of reach: this fight is about rout
	setField(s, "combat", "forced_band", "crit")

	if inFight >= 3 {
		setField(s, "combat", "loss_weight", 0)
		setField(s, "combat", "player_action", "attack")

		killed := stepUntilADeath(t, s, groupID)
		if killed == 0 {
			t.Fatalf("act B: the resolver never killed a member of %s", groupID)
		}

		setField(s, "combat", "player_action", "hold")

		off := groupNamed(t, s, groupID)
		if off == nil {
			t.Fatalf("act B: the pack vanished from the provider")
		}

		if got := num(off, "morale"); got != moraleAtArrival {
			t.Fatalf("act B: with loss_weight 0 a death must cost the pack nothing; %.2f -> %.2f",
				moraleAtArrival, got)
		}

		t.Logf("act B PASS: %d dead and morale still %.0f -- the trigger is the dial, not a coincidence",
			killed, num(off, "morale"))
	} else {
		t.Logf("act B SKIPPED: only %d of %s reached the fight, and the act spends one. "+
			"TestResolverRoutIsOffWhenTheDialIsZero is the negative control.", inFight, groupID)
	}

	// --- act A: the game hurts the pack, and the arithmetic is checkable -----
	//
	// THE DISCRIMINATING ASSERTION OF THE SCRIPT. The test chooses the
	// formula -- 100 / the pack's STARTING size -- and the system reports the
	// starting size and the resulting morale. A build that decremented a flat
	// amount, or that wrote a number the harness handed it, fails here.
	setField(s, "combat", "loss_weight", 1)
	setField(s, "combat", "player_action", "attack")

	before := num(groupNamed(t, s, groupID), "morale")
	wantDrop := 100.0 / spawned

	if !stepUntilMoraleMoves(t, s, groupID, before) {
		t.Fatalf("act A: morale never moved after another death; still %.2f", before)
	}

	after := groupNamed(t, s, groupID)
	got := before - num(after, "morale")

	t.Logf("act A state: %s morale %.2f routing=%t members=%.0f | fight %s",
		groupID, num(after, "morale"), flag(t, after, "routing"), num(after, "members"),
		describeFight(t, s))

	// One step can resolve more than one round, so more than one member can
	// die inside it. Accept whole multiples of the per-loss cost and report
	// which one landed.
	losses := math.Round(got / wantDrop)
	if losses < 1 || math.Abs(got-losses*wantDrop) > 0.001 {
		t.Fatalf("act A: a loss out of %d must cost %.4f (or a whole multiple); the pack lost %.4f",
			int(spawned), wantDrop, got)
	}

	t.Logf("act A PASS: %.0f loss(es) out of %d took morale %.2f -> %.2f, %.4f each -- "+
		"and the script never wrote morale",
		losses, int(spawned), before, num(after, "morale"), wantDrop)

	// --- act C: release AND unwatch, and the second frame is the assertion ---
	//
	// One frame is exactly what undoes a release without an unwatch:
	// startChasesForTheAware runs every frame with no liveness filter, so a
	// corpse that is still watched is chasing again on the next one.
	dead := deadParticipant(t, s, packIDs)
	if dead == "" {
		t.Fatalf("act C: nothing in %s is reported dead; %v", groupID, combatState(s))
	}

	if chasing(t, s, dead) {
		t.Fatalf("act C: a dead enemy must not still be chasing: %s", dead)
	}

	s.call("strigoi_step", map[string]any{"frames": 1})

	if chasing(t, s, dead) {
		t.Fatalf("act C: %s is chasing again one frame later -- released but never unwatched", dead)
	}

	t.Logf("act C PASS: %s released and still released a frame on -- strigoi_pursue called nowhere", dead)

	// --- act F: the rout ending is labelled, and it is not a kill ------------
	//
	// THE MOST IMPORTANT ASSERTION IN THE STEP. A fight the player survived
	// because the pack broke must report enemies_routed. The failure this
	// guards is not cosmetic: removing routers from the participant list
	// makes pruneOrEnd's loop run over an empty slice, which leaves allDead
	// true and reports the whole thing as enemies_dead.
	end := stepUntilFightEnds(t, s)

	t.Logf("the fight ended %q (dead %v, routed %v, quick %v)",
		str(end, "ended_reason"), end["ended_enemies_dead"],
		end["ended_routed"], end["quick_resolved"])

	if str(end, "ended_reason") != "enemies_routed" {
		t.Fatalf("act F: a pack driven under its rout threshold must end the fight enemies_routed, got %q: %v",
			str(end, "ended_reason"), end)
	}

	if num(end, "ended_routed") < 1 {
		t.Fatalf("act F: the rout counter must count it; %v", end["ended_routed"])
	}

	if got := num(end, "ended_enemies_dead"); got != deadEndingsBefore {
		t.Fatalf("act F: a rout is NOT a kill, and ended_enemies_dead is what the thirteenth "+
			"script asserts on; it went %.0f -> %.0f", deadEndingsBefore, got)
	}

	routing := groupNamed(t, s, groupID)
	if routing == nil || !flag(t, routing, "routing") {
		t.Fatalf("act F: the pack that broke must report routing: %v", routing)
	}

	// The ones that broke were withdrawn too, for the same reason the dead one
	// was: without it, startChasesForTheAware re-chases them next frame and
	// tryStart opens a fresh fight against the pack that just fled.
	for _, id := range packIDs {
		if id != dead && chasing(t, s, id) {
			t.Fatalf("act F: %s broke and is still chasing -- a routed member must be released too", id)
		}
	}

	t.Logf("act F PASS: %s broke at morale %.2f and the fight ended enemies_routed, "+
		"with ended_enemies_dead still %.0f -- and every member of it stopped chasing",
		groupID, num(routing, "morale"), deadEndingsBefore)

	// --- act D: a reinforcement joins without rebuilding the fight -----------
	//
	// The join itself is the GAME's: the harness only puts a monster on the
	// map and points its attention at the player, which is what every earlier
	// script does to start a fight. What is asserted is that the participant
	// list GREW inside a fight that kept its identity and its round.
	pp := s.call("strigoi_get_player", map[string]any{})
	px, py := num(pp, "x"), num(pp, "y")

	setField(s, "combat", "player_action", "hold")
	setField(s, "combat", "forced_band", "graze")

	first := spawnNPC(t, s, "fallen1", px+1, py)
	s.call("strigoi_watch", map[string]any{"watcher": first, "target": playerHandle})
	fightNow(t, s)

	opened := combatState(s)
	encounterID := str(opened, "encounter")
	joinedBefore := num(opened, "joined")

	spot := clearNeighbour(t, s, px, py)
	second := spawnNPC(t, s, "fallen1", spot[0], spot[1])
	secondID := entityID(t, s, second)
	s.call("strigoi_watch", map[string]any{"watcher": second, "target": playerHandle})

	joined := false

	for i := 0; i < 200 && !joined; i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})

		state := combatState(s)
		if !flag(t, state, "fighting") {
			t.Fatalf("act D: the fight ended before the reinforcement arrived: %v", state)
		}

		joined = num(state, "joined") > joinedBefore
	}

	if !joined {
		t.Fatalf("act D: a second aware monster in reach never joined the running fight: %v", combatState(s))
	}

	after2 := combatState(s)
	if str(after2, "encounter") != encounterID {
		t.Fatalf("act D: joining must not restart the fight; encounter %q -> %q",
			encounterID, str(after2, "encounter"))
	}

	// participant() fails with the whole list if the id is not there, which is
	// the assertion: the arrival must have a row.
	if row := participant(t, after2, secondID); str(row, "side") != "enemy" {
		t.Fatalf("act D: the arrival must be an enemy participant: %v", row)
	}

	t.Logf("act D PASS: joined %.0f -> %.0f in encounter %s, round %.0f -- same fight, one more in it",
		joinedBefore, num(after2, "joined"), encounterID, num(after2, "round"))

	// --- act E: quick-resolve fires, and only against what the tables placed --
	//
	// The two monsters now in this fight were placed by the harness and belong
	// to no group, so they are NOT mundane animals however far ahead the
	// player gets. That is the half of R2 §2B's prohibition this build can
	// honestly assert: the never-the-dead half is a named deferral to M4.7,
	// because there are no dead rows to test against.
	setField(s, "combat", "quick_resolve_advantage", 0.1)
	setField(s, "combat", "player_action", "attack")
	setField(s, "combat", "forced_band", "crit")

	quickBefore := num(combatState(s), "quick_resolved")

	strays := stepUntilFightEnds(t, s)

	if num(strays, "quick_resolved") != quickBefore {
		t.Fatalf("act E: nothing the spawn tables never placed may quick-resolve; %v -> %v",
			quickBefore, strays["quick_resolved"])
	}

	t.Logf("act E PASS: a fight against two stand-ins ended %q with quick_resolved still %.0f -- "+
		"an enemy with no known group is not a mundane animal",
		str(strays, "ended_reason"), quickBefore)

	// THE OTHER HALF OF N1 section 5's SENTENCE -- quick-resolve FIRING
	// against a pack already beaten -- IS PROVED IN THE UNIT TESTS AND NOT
	// HERE, and saying so is the point rather than an apology. Reaching that
	// state in a real build needs a SECOND pack walked to and then ground
	// down to one member, and by that point in the night the player has been
	// chewed on by everything standing between the two: a run that tried it
	// died on the way at 18 health. TestResolverQuickResolveFiresAgainstA-
	// PackAlreadyBeaten drives exactly that state with fakes, and
	// TestResolverQuickResolveMeasuresTheFightNotTheTable pins the A-severity
	// defect THIS script found in the real build -- advantage measured
	// against the AUTHORED pack size fired quick-resolve on fights in which
	// nothing had died, because a pack does not arrive together.

	t.Logf("PASS: the pack broke because the game hurt it, the ending said so, " +
		"the dead stopped chasing, and a reinforcement joined without restarting the fight")
}

// walkToAPack forces the table, then WALKS THE PLAYER TO THE PACK and waits
// for the fight to open with at least `want` of one group in it.
//
// MEASURED, AND IT IS WHY THIS FUNCTION EXISTS. Two earlier runs of this
// script waited for a pack to come to the player and never got a usable
// fight, while the counters showed five encounters had begun and ended
// between reads. The provider said why: at deep night in a town the notice
// checks report sees:false at seven tiles -- the line of sight is blocked by
// buildings -- so eight full groups stood 8 to 20 tiles away, never became
// aware, and never chased. Waiting harder would not have fixed it.
//
// So the player goes to them, with strigoi_move_player_to, which is what a
// player does. Nothing here starts the fight, chooses the participants or
// touches morale: the walk ends beside the pack and the GAME does the rest --
// notice, chase, tryStart, and the reinforcement path as the stragglers
// arrive.
func walkToAPack(t *testing.T, s *session, want int) (map[string]any, string, int) {
	t.Helper()

	target, targetID, spots := packTour(t, s, want)

	for i := 0; i < 40 && targetID == ""; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 5})

		target, targetID, spots = packTour(t, s, want)
	}

	if targetID == "" {
		t.Fatalf("the table never placed a findable group of %d: %v", want, spawnsState(s)["group_list"])
	}

	t.Logf("touring %s (%s, %d spawned) at %d known positions",
		targetID, str(target, "row"), int(num(target, "spawned")), len(spots))

	best := 0

	var bestGroup map[string]any

	bestID := ""

	check := func() bool {
		if !flag(t, combatState(s), "fighting") {
			return false
		}

		if g, id, n := fightingPack(t, s); n > best {
			bestGroup, bestID, best = g, id, n
		}

		return best >= want
	}

	// THE WALK IS SLICED, AND THAT IS A MEASUREMENT TOO. A wait:true walk
	// steps thousands of ticks inside ONE call: a run that did it reported
	// nine encounters begun and ended with the script never once seeing a
	// fight. A fire-and-forget move is worse -- the player does not close at
	// all. Short waited slices with a poll between them do both jobs.
	for _, spot := range spots {
		for slice := 0; slice < 12; slice++ {
			move := s.call("strigoi_move_player_to", map[string]any{
				"x": spot[0], "y": spot[1], "wait": true, "max_ticks": 150,
			})

			if check() {
				// STOP WALKING, and this line is a measurement. A move packet
				// outlives the slice that issued it, so the player kept
				// striding away after the fight opened -- and a run watched
				// the slower of two dogs fall out of reach and be pruned from
				// the participant list, which left the pack with nobody to
				// break when the other one died. Re-issuing a move to where
				// he already is halts him.
				now := s.call("strigoi_get_player", map[string]any{})
				s.call("strigoi_move_player_to", map[string]any{
					"x": num(now, "x"), "y": num(now, "y"),
				})

				t.Logf("tour: %d of %s in the fight; halted at %.1f,%.1f",
					best, bestID, num(now, "x"), num(now, "y"))

				return bestGroup, bestID, best
			}

			if h := num(metersState(s), "health"); h <= 25 {
				t.Fatalf("the player was worn down to %.0f health on the tour", h)
			}

			if str(move, "outcome") == "arrived" {
				break
			}
		}

		t.Logf("tour: visited %.1f,%.1f -- best pack in the fight is %d", spot[0], spot[1], best)
	}

	for i := 0; i < 700; i++ {
		if flag(t, combatState(s), "fighting") {
			if g, id, n := fightingPack(t, s); n > best {
				bestGroup, bestID, best = g, id, n
			}

			if best >= want {
				return bestGroup, bestID, best
			}
		}

		if i%40 == 0 {
			st := combatState(s)
			t.Logf("closing %d: fighting=%t participants=%d joined=%.0f encounters=%.0f "+
				"round=%.0f best=%d health=%.0f",
				i, flag(t, st, "fighting"), len(asList(st["participants"])),
				num(st, "joined"), num(st, "encounters"), num(st, "round"),
				best, num(metersState(s), "health"))
		}

		if h := num(metersState(s), "health"); h <= 25 {
			t.Fatalf("the player was worn down to %.0f health while the pack closed "+
				"(best pack in a fight so far: %d)", h, best)
		}

		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	if best >= 2 {
		t.Logf("only %d of one pack ever reached the fight at once; running with that", best)

		return bestGroup, bestID, best
	}

	t.Fatalf("standing next to a pack of %d never produced a fight with two of them in it: combat=%v",
		int(num(target, "spawned")), combatState(s))

	return nil, "", 0
}

// packTour picks the group with the most members and returns where each of
// them is standing.
//
// MEASURED, AND IT IS THE FOURTH THING THIS SETUP HAD TO LEARN. A pack does
// not arrive together: Spawns.spawn scatters its members between the row's
// MinTiles and MaxTiles of the player independently, so a group of four can be
// four separate corners of the map. A run that looked for three members of one
// group within five tiles of each other found a best of ONE, across all eight
// live groups.
//
// So the player TOURS them. Walking to a member gets it to see him and start a
// chase, and a chase does not stop when he walks on -- so by the time he has
// visited two of them, two of one pack are following him and the fight has a
// pack in it. That is a player walking around at night, which is the thing the
// slice is about.
func packTour(t *testing.T, s *session, want int) (map[string]any, string, [][2]float64) {
	t.Helper()

	entities := s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200})

	at := map[string][2]float64{}

	for _, raw := range asList(entities["items"]) {
		row, ok := raw.(map[string]any)
		if ok {
			at[str(row, "id")] = [2]float64{num(row, "x"), num(row, "y")}
		}
	}

	var (
		best   map[string]any
		bestID string
		spots  [][2]float64
	)

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		g, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		var here [][2]float64

		for _, nb := range asList(g["notice"]) {
			row, ok := nb.(map[string]any)
			if !ok {
				continue
			}

			if p, seen := at[str(row, "watcher")]; seen {
				here = append(here, p)
			}
		}

		if len(here) > len(spots) {
			best, bestID, spots = g, str(g, "group"), here
		}
	}

	if len(spots) < want {
		return nil, "", nil
	}

	return best, bestID, spots
}

// fightingPack is the pack with the most LIVING members in the current fight.
func fightingPack(t *testing.T, s *session) (map[string]any, string, int) {
	t.Helper()

	counts := map[string]int{}

	for _, raw := range asList(combatState(s)["participants"]) {
		row, ok := raw.(map[string]any)
		if !ok || str(row, "side") != "enemy" || flag(t, row, "dead") || flag(t, row, "routed") {
			continue
		}

		if pack := str(row, "pack"); pack != "" {
			counts[pack]++
		}
	}

	bestID, best := "", 0

	for id, n := range counts {
		if n > best || (n == best && id < bestID) {
			bestID, best = id, n
		}
	}

	if bestID == "" {
		return nil, "", 0
	}

	return groupNamed(t, s, bestID), bestID, best
}

// groupNamed finds one group block in the spawns provider, or nil.
func groupNamed(t *testing.T, s *session, id string) map[string]any {
	t.Helper()

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		row, ok := raw.(map[string]any)
		if ok && str(row, "group") == id {
			return row
		}
	}

	return nil
}

// stepUntilADeath steps frames until the fight reports a dead enemy, and
// returns how many.
func stepUntilADeath(t *testing.T, s *session, groupID string) int {
	t.Helper()

	for i := 0; i < 400; i++ {
		n := 0

		for _, raw := range asList(combatState(s)["participants"]) {
			row, ok := raw.(map[string]any)
			if ok && str(row, "pack") == groupID && flag(t, row, "dead") {
				n++
			}
		}

		if n > 0 {
			return n
		}

		if !flag(t, combatState(s), "fighting") {
			return 0
		}

		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	return 0
}

// stepUntilMoraleMoves steps until one group's morale differs from `from`.
func stepUntilMoraleMoves(t *testing.T, s *session, groupID string, from float64) bool {
	t.Helper()

	for i := 0; i < 400; i++ {
		g := groupNamed(t, s, groupID)
		if g == nil {
			return false
		}

		if i%10 == 0 {
			t.Logf("waiting %d: %s morale %.2f | %s", i, groupID, num(g, "morale"), describeFight(t, s))
		}

		if num(g, "morale") != from {
			return true
		}

		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	return false
}

// stepUntilFightEnds returns the combat state once nothing is fighting.
func stepUntilFightEnds(t *testing.T, s *session) map[string]any {
	t.Helper()

	for i := 0; i < 600; i++ {
		state := combatState(s)
		if !flag(t, state, "fighting") {
			return state
		}

		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	t.Fatalf("the fight never ended: %v", combatState(s))

	return nil
}

// deadParticipant is the id of one enemy that has reached zero.
//
// IT FALLS BACK TO THE ACTION LOG, and that is not belt-and-braces: a rout
// ENDS the fight, and when it does the participant list goes with it. The
// blow that killed the member whose death caused the rout is still in
// lastActions, with target_health_after 0, because the log survives the
// encounter for exactly this reason.
func deadParticipant(t *testing.T, s *session, candidates []string) string {
	t.Helper()

	state := combatState(s)

	for _, raw := range asList(state["participants"]) {
		row, ok := raw.(map[string]any)
		if ok && str(row, "side") == "enemy" && flag(t, row, "dead") {
			return str(row, "id")
		}
	}

	want := map[string]bool{}
	for _, id := range candidates {
		want[id] = true
	}

	for _, raw := range asList(state["actions"]) {
		row, ok := raw.(map[string]any)
		if ok && want[str(row, "target")] && num(row, "target_health_after") == 0 {
			return str(row, "target")
		}
	}

	return ""
}

// describeFight is a one-line dump of who is in the fight and what state they
// are in, for when an assertion about a pack fails and the reason is which
// members were still in it.
func describeFight(t *testing.T, s *session) string {
	t.Helper()

	state := combatState(s)

	out := "fighting=" + str(state, "ended_reason")
	if flag(t, state, "fighting") {
		out = "running"
	}

	for _, raw := range asList(state["participants"]) {
		row, ok := raw.(map[string]any)
		if !ok || str(row, "side") != "enemy" {
			continue
		}

		out += fmt.Sprintf(" [%s pack=%s dead=%t routed=%t adj=%t]",
			str(row, "id")[:8], str(row, "pack"),
			flag(t, row, "dead"), flag(t, row, "routed"), flag(t, row, "adjacent"))
	}

	return out
}

// enemyIDsOf is every enemy in the current fight that belongs to one pack.
func enemyIDsOf(t *testing.T, s *session, groupID string) []string {
	t.Helper()

	out := []string{}

	for _, raw := range asList(combatState(s)["participants"]) {
		row, ok := raw.(map[string]any)
		if ok && str(row, "pack") == groupID {
			out = append(out, str(row, "id"))
		}
	}

	return out
}

// chasing reports whether the pursuit provider still holds a chase for one
// hunter. It reads the list rather than a per-hunter field, because what is
// being asserted is an absence.
func chasing(t *testing.T, s *session, hunterID string) bool {
	t.Helper()

	state := sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")

	for _, raw := range asList(state["chase_list"]) {
		row, ok := raw.(map[string]any)
		if ok && str(row, "hunter") == hunterID {
			return true
		}
	}

	return false
}
