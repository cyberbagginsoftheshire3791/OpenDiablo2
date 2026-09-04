//go:build playtest

package playtest

import (
	"testing"
)

// TestCombatBody is M4.5 step 3's Constitution VI.2 script: the twelfth.
//
// THE MILESTONE'S ASSERTION IN ONE SENTENCE: a monster in a fight has a body,
// the body's size comes from that monster's own record, and the game -- not
// the harness -- is what put it there.
//
// Every earlier milestone could be driven into looking right by a script.
// M4.1's placed light and M4.3b's awareness both passed while being hollow in
// a real build, because the script called the verb the game was supposed to
// call. So this script deliberately has no verb for the thing it is testing:
// THERE IS NO START-A-FIGHT TOOL AND NO SET-HEALTH TOOL. It places two
// monsters, tells the notice model to watch, and steps the clock. If the
// encounter appears, the game made it; if the bodies are there, the game
// screen adopted them through gameSpawner's callback and Game.BodyOf.
//
// The discriminating assertion is act 4, and it is the one that would catch a
// body faked with a constant: the two monsters are DIFFERENT codes with
// different hit-point bands in monstats.txt -- zombie1 at 181 and fallen1 at
// 61, measured by tools/animcensus on 31 Aug 2026. The test chose those
// numbers; the system reports them. A body wired to a literal, or to the
// wrong record, or to the player's health, fails here rather than passing
// quietly. That is the same shape as the units assertion in spawns_test.go,
// and it exists for the same reason.
//
// Four acts:
//  1. the combat provider knows where bodies live at all;
//  2. two monsters of different kinds, adjacent, aware;
//  3. THE GAME starts the encounter -- no tool was called to start it;
//  4. each body reports its own record's maximum, and the player's does not
//     appear on the enemy side;
//  5. THE GAME adopts bodies for a pack the spawn tables placed, before any
//     fight exists -- the act that tells the eager path from the fallback.
func TestCombatBody(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gaunt", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")
	playerHandle := str(p, "handle")
	t.Logf("player at %.2f,%.2f (%s)", px, py, playerHandle)

	// --- act 1: the provider knows where bodies live ------------------------
	//
	// has_bodies is false in every unit test in d2world, because the model is
	// built there with no registry. It must be TRUE in a real build, and this
	// is the cheapest possible statement that the wiring in NewGame happened.
	combat := combatState(s)
	if !flag(t, combat, "has_bodies") {
		t.Fatalf("the combat model has no body registry in a real build: %v", combat)
	}

	if flag(t, combat, "fighting") {
		t.Fatalf("a fight is already running before anything was placed: %v", combat)
	}

	t.Logf("act 1: has_bodies=true, adjacent_tiles=%.0f, round_minutes=%.1f",
		num(combat, "adjacent_tiles"), num(combat, "round_minutes"))

	// --- act 2: two kinds of monster, adjacent, aware -----------------------
	//
	// Adjacency is Chebyshev on floored tiles with AdjacentTiles=1, so one of
	// the eight neighbours is in reach and the map's own geometry decides
	// which neighbours have a clear line. Sweep for a clear one rather than
	// assuming a bearing: seed 1462 draws a generated map, not an authored
	// one, and a hardcoded offset is a test that passes for one worldgen.
	var (
		spotX, spotY float64
		haveSpot     bool
	)

	for _, d := range [][2]float64{
		{1, 0}, {0, 1}, {-1, 0}, {0, -1},
		{1, 1}, {-1, 1}, {1, -1}, {-1, -1},
	} {
		x, y := px+d[0], py+d[1]

		if flag(t, s.call("strigoi_find_path", map[string]any{"to_x": x, "to_y": y}), "straight_line_clear") {
			spotX, spotY, haveSpot = x, y, true

			break
		}
	}

	if !haveSpot {
		t.Fatalf("no neighbouring tile of %.2f,%.2f has a clear line; cannot arrange a fight", px, py)
	}

	t.Logf("act 2: placing both monsters at %.1f,%.1f (one tile from the player)", spotX, spotY)

	wolf := spawnNPC(t, s, "zombie1", spotX, spotY) // the wolves' stand-in
	dog := spawnNPC(t, s, "fallen1", spotX, spotY)  // the dogs' stand-in
	wolfID := entityID(t, s, wolf)
	dogID := entityID(t, s, dog)

	s.call("strigoi_watch", map[string]any{"watcher": wolf, "target": playerHandle})
	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": playerHandle})

	// --- act 3: THE GAME starts the fight -----------------------------------
	//
	// Nothing below calls a combat tool. The clock is stepped and the game's
	// own advanceWorld runs notice, then the chases, then combat -- which is
	// where an aware thing in reach becomes an encounter. Failing here means
	// the seam is not joined in a real build, which is exactly the defect
	// that reopened M4.1 and M4.3b.
	// STEPPED IN FRAMES SINCE M4.5 STEP 4, AND NOT ONE ASSERTION BELOW MOVED.
	//
	// This loop used to step whole world minutes. That was fine while a fight
	// was only a fact; now that a blow does something, it is not, because
	// advanceWorld runs once per FRAME and a stepped "world minute" is 15 to
	// 24 calls into the resolver. The encounter therefore opened on the first
	// frame of a step and the rest of that step resolved a round -- so act 4
	// read the zombie at 166 of 181 and failed on "adopted at full health".
	//
	// A single Advance either OPENS an encounter or ticks one, never both. So
	// stopping on the frame the fight opens is the moment this script always
	// meant to read at: round 1, nothing resolved, every body still exactly as
	// the game screen adopted it. The step-4 brief's reviewer reasoned about
	// Advance and not about a harness step, and that is the whole gap.
	var fighting bool

	for i := 0; i < 150 && !fighting; i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})

		fighting = flag(t, combatState(s), "fighting")
	}

	if !fighting {
		t.Fatalf("after 150 frames with two aware monsters one tile away, "+
			"the game never started an encounter: %v", combatState(s))
	}

	if got := num(combatState(s), "round"); got != 1 {
		t.Fatalf("stopping on the frame the fight opens must catch round 1, before any blow; round=%.0f", got)
	}

	combat = combatState(s)
	t.Logf("act 3: encounter %v, round %.0f, order %v",
		combat["encounter"], num(combat, "round"), combat["order"])

	// --- act 4: each body carries its own record's maximum ------------------
	parts, ok := combat["participants"].([]any)
	if !ok {
		t.Fatalf("the combat provider must report participants; got %v", combat["participants"])
	}

	rows := map[string]map[string]any{}

	var playerRow map[string]any

	for _, raw := range parts {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if str(row, "side") == "player" {
			playerRow = row

			continue
		}

		rows[str(row, "id")] = row
	}

	// MaxHPNormal from monstats.txt, measured by tools/animcensus on
	// 31 Aug 2026 and written here by hand. The test chose these; the system
	// reports them. If monstats ever changes, or a body is ever wired to a
	// constant, this is the line that says so.
	for _, want := range []struct {
		what string
		id   string
		max  float64
	}{
		{"zombie1 (the wolves' stand-in)", wolfID, 181},
		{"fallen1 (the dogs' stand-in)", dogID, 61},
	} {
		row, ok := rows[want.id]
		if !ok {
			t.Fatalf("act 4: %s (%s) is not a participant; the fight holds %v", want.what, want.id, rows)
		}

		if !flag(t, row, "has_body") {
			t.Fatalf("act 4: %s is in a fight with no body: %v", want.what, row)
		}

		if got := num(row, "max_health"); got != want.max {
			t.Fatalf("act 4: %s must report max_health %.0f from its own monstats record, got %.0f: %v",
				want.what, want.max, got, row)
		}

		if got, max := num(row, "health"), num(row, "max_health"); got != max {
			t.Fatalf("act 4: %s is adopted at full health; got %.0f of %.0f", want.what, got, max)
		}

		t.Logf("act 4: %s -> has_body=true health=%.0f/%.0f",
			want.what, num(row, "health"), num(row, "max_health"))
	}

	// The two bands must actually differ, or the assertion above could pass
	// against a body that reads whichever record it likes.
	if num(rows[wolfID], "max_health") == num(rows[dogID], "max_health") {
		t.Fatalf("act 4: both monsters report the same maximum, so the body is not reading its own record")
	}

	// The PLAYER's side reports the meters' two flags and NOT a health of its
	// own: that already lives on the meters provider, and one truth with two
	// homes is the disease this project keeps treating.
	if playerRow == nil {
		t.Fatalf("act 4: the fight has no player side: %v", parts)
	}

	if _, dup := playerRow["health"]; dup {
		t.Fatalf("act 4: the player's health is reported by the meters, not by combat: %v", playerRow)
	}

	if _, ok := playerRow["reaction_available"]; !ok {
		t.Fatalf("act 4: the player side must still carry reaction_available: %v", playerRow)
	}

	t.Logf("act 4: player side intact (reaction_available=%v, shaken=%v), no duplicated health",
		playerRow["reaction_available"], playerRow["shaken"])

	// --- act 5: the GAME adopts, not the harness ----------------------------
	//
	// THIS IS THE ACT THAT MATTERS MOST, and it exists because acts 1-4 could
	// not have caught the failure this project has now shipped twice. Both
	// monsters above arrived through strigoi_spawn_entity, so their bodies
	// could have come from Game.BodyOf's on-demand fallback with the eager
	// path in gameSpawner.Spawn deleted, and every assertion so far would
	// still pass. That is the M4.1 and M4.3b shape precisely: a thing the
	// game is supposed to do that only the harness ever really does.
	//
	// So: raise the chance dial until the REAL spawn tables fire -- there is
	// no spawn verb by design, which is what makes a forced table an
	// observation about the game -- and watch bodies_known rise by the pack's
	// size. Those members are far away and in nobody's encounter, so nothing
	// has asked BodyOf about them; only adoption at spawn time can move this
	// number here.
	before := int(num(combatState(s), "bodies_known"))
	membersBefore := int(num(spawnsState(s), "spawned"))

	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "chance", "value": 100,
	})

	for i := 0; i < 12 && num(spawnsState(s), "groups") == 0; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 6})
	}

	spawned := spawnsState(s)
	if num(spawned, "groups") == 0 {
		t.Fatalf("act 5: a certainty must actually fire; %d check(s), %d roll(s), %d failure(s)",
			int(num(spawned, "checks")), int(num(spawned, "rolls")),
			int(num(spawned, "spawn_failures")))
	}

	arrivals := int(num(spawned, "spawned")) - membersBefore
	if arrivals <= 0 {
		t.Fatalf("act 5: a group arrived but no npcs were spawned: %v", spawned)
	}

	after := int(num(combatState(s), "bodies_known"))
	if after-before != arrivals {
		t.Fatalf("act 5: the spawn tables placed %d monster(s) and the game adopted %d body/bodies "+
			"(%d -> %d). The eager adoption in gameSpawner.Spawn is what should have done this; "+
			"if it is gone, every other assertion in this file still passes and nothing notices.",
			arrivals, after-before, before, after)
	}

	t.Logf("act 5 PASS: the tables placed %d monster(s) and bodies_known went %d -> %d "+
		"with no fight involving them -- the GAME adopted, not the harness",
		arrivals, before, after)
}

// combatState reads the combat provider's block.
//
// Through sub(..., "state"): strigoi_get_system_state returns
// {system, settable, state}, and reading a field off the top level silently
// yields zero for every one of them.
func combatState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "combat"}), "state")
}
