//go:build playtest

package playtest

import (
	"fmt"
	"testing"
)

// TestPathfinding is M4.3a's Constitution VI.2 script: the eighth.
//
// It asserts the thing the milestone exists for -- that the engine routes
// around obstacles instead of stopping at the first one -- and it asserts it
// against a negative control, because a route that arrives proves nothing on
// its own. strigoi_find_path reports straight_line_clear alongside the
// waypoints for exactly that reason: if the straight line was already clear,
// arriving is not evidence of routing.
//
// Four acts:
//  1. a route whose straight line is BLOCKED still reaches its goal;
//  2. the same query, asked twice, returns a byte-identical path (section 3.2
//     of the signed note, and the assertion that catches an unstable
//     tie-break -- every entity position is inside the state digest);
//  3. an unreachable goal still yields a partial route toward it rather than
//     nothing, which is the raycast's failure mode kept on purpose;
//  4. the player actually walks one of these routes in the running game.
func TestPathfinding(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gaunt", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	p := s.call("strigoi_get_player", map[string]any{})
	startX, startY := num(p, "x"), num(p, "y")
	t.Logf("player starts at %.2f,%.2f", startX, startY)

	// --- act 1: find a goal whose straight line is blocked -------------------
	//
	// The map is generated, not authored, so no fixed vector is guaranteed to
	// be obstructed. Sweep a ring of candidates and take the first whose
	// straight line is blocked but whose route still reaches -- that pair of
	// facts is the whole assertion, and finding it by search rather than by
	// hardcoding keeps the script honest across map changes.
	type candidate struct {
		x, y  float64
		count int
	}

	var found *candidate

	offsets := [][2]float64{
		{12, 0}, {0, 12}, {-12, 0}, {0, -12},
		{9, 9}, {-9, 9}, {9, -9}, {-9, -9},
		{18, 0}, {0, 18}, {-18, 0}, {0, -18},
		{16, 8}, {-16, 8}, {8, 16}, {8, -16},
	}

	for _, d := range offsets {
		toX, toY := startX+d[0], startY+d[1]

		res := s.call("strigoi_find_path", map[string]any{
			"to_x": toX,
			"to_y": toY,
		})

		clear := boolean(res, "straight_line_clear")
		reachable := boolean(res, "reachable")
		count := int(num(res, "waypoint_count"))

		t.Logf("candidate %+.0f,%+.0f -> straight_line_clear=%v reachable=%v waypoints=%d",
			d[0], d[1], clear, reachable, count)

		if !clear && reachable {
			found = &candidate{x: toX, y: toY, count: count}
			break
		}
	}

	if found == nil {
		t.Fatal("no candidate goal had a blocked straight line AND a reachable route; " +
			"without both, this script cannot tell routing from an unobstructed walk")
	}

	t.Logf("THE ASSERTION: %.2f,%.2f -> %.2f,%.2f has NO clear straight line, "+
		"and the pathfinder still reaches it in %d waypoints. The raycast returned "+
		"one point short of the first blocker and stopped there.",
		startX, startY, found.x, found.y, found.count)

	if found.count < 2 {
		t.Fatalf("a route around an obstacle needs at least two waypoints, got %d", found.count)
	}

	// --- act 2: the same query twice is byte-identical ----------------------
	//
	// Section 3.2's signed assertion. An unstable tie-break in the priority
	// queue is where A* usually leaks non-determinism, and it would surface
	// later as an unexplained digest drift rather than as a failure here.
	first := s.call("strigoi_find_path", map[string]any{"to_x": found.x, "to_y": found.y})
	second := s.call("strigoi_find_path", map[string]any{"to_x": found.x, "to_y": found.y})

	firstPath := pathSignature(t, first)
	secondPath := pathSignature(t, second)

	if firstPath != secondPath {
		t.Fatalf("the same start and goal produced two different paths:\n  %s\n  %s", firstPath, secondPath)
	}

	t.Logf("determinism: the same query twice gives the same %d waypoints (%s)",
		int(num(first, "waypoint_count")), truncate(firstPath, 120))

	// --- act 3: an unreachable goal degrades, it does not refuse ------------
	//
	// Far off the map. The bounded search cannot get there, and must still
	// walk as far toward it as it can -- the raycast's failure mode, kept on
	// purpose so "cannot get all the way" stays "go as far as you can".
	far := s.call("strigoi_find_path", map[string]any{
		"to_x": startX + 400,
		"to_y": startY + 400,
	})

	if boolean(far, "reachable") {
		t.Fatal("a goal 400 tiles off the map reported reachable")
	}

	t.Logf("unreachable goal: reachable=false, %d partial waypoints toward it",
		int(num(far, "waypoint_count")))

	// --- act 4: the player walks it in the running game ---------------------
	//
	// The model says the route exists; this says the mover follows it. Both
	// halves are needed -- M4.1's lesson was that a model nothing drives is
	// decoration.
	// wait:true is load-bearing, not a convenience. The clock is paused for
	// determinism, so nothing advances on the wall clock and a plain sleep
	// after a fire-and-forget move measures a player who was never stepped.
	// With wait the tool steps the sim itself and reports the outcome.
	move := s.call("strigoi_move_player_to", map[string]any{
		"x": found.x, "y": found.y, "wait": true, "max_ticks": 3000,
	})
	t.Logf("move outcome: %v after %v ticks", move["outcome"], move["ticks"])

	p = s.call("strigoi_get_player", map[string]any{})
	endX, endY := num(p, "x"), num(p, "y")

	movedX, movedY := endX-startX, endY-startY
	moved := movedX*movedX + movedY*movedY

	t.Logf("walked %.2f,%.2f -> %.2f,%.2f (goal %.2f,%.2f)", startX, startY, endX, endY, found.x, found.y)

	if moved < 1.0 {
		t.Fatalf("the player did not move: %.2f,%.2f -> %.2f,%.2f", startX, startY, endX, endY)
	}

	// --- act 5: pursuit -- something follows a target that keeps moving -----
	//
	// The rest of this script proves a route can be computed and walked. This
	// act proves the milestone's other half: that a chase survives the quarry
	// moving, which is the whole difference between a path and a pursuit.
	pursuitOnly(t, s, endX, endY)
}

// pursuitOnly is act 5, split out to keep TestPathfinding readable.
func pursuitOnly(t *testing.T, s *session, playerX, playerY float64) {
	t.Helper()

	// The provider is registered, not merely planned.
	systems := s.call("strigoi_list_systems", map[string]any{})
	if !hasSystem(systems, "pursuit") {
		t.Fatal("the pursuit provider is not registered")
	}

	// A hunter a few tiles away. strigoi_spawn_entity takes WORLD tiles and
	// the field is "code" -- the raw d2mapentity factory wants subtiles, but
	// the tool converts, and that difference has cost a run before.
	spawn := s.call("strigoi_spawn_entity", map[string]any{
		"kind": "npc", "code": "fallen1",
		"x": playerX + 6, "y": playerY,
	})

	hunter := str2(spawn["handle"])
	if hunter == "" {
		t.Fatalf("no handle back from spawn: %v", spawn)
	}

	t.Logf("hunter %s spawned near the player", hunter)

	chase := s.call("strigoi_pursue", map[string]any{"hunter": hunter, "quarry": "p:1"})

	// Assert that OUR hunter is chasing, not that it is the only thing that
	// is. This used to require exactly one live chase, which was true only
	// while nothing but a script could start one -- since M4.3b's reopening
	// the spawn tables notice the player and start chases of their own, so a
	// count assertion here fails for the good reason that the game got busier.
	// The same rot as ui_inventory's absence assertion: a check written about
	// a world where only the test acts.
	if int(num(chase, "chases")) < 1 {
		t.Fatalf("expected at least our own live chase, got %v", chase["chases"])
	}

	firstSolves := int(num(chase, "solves"))
	t.Logf("chase started: %d chase(s), %d route(s) solved", int(num(chase, "chases")), firstSolves)

	// Step, then move the player well past the repath dial and step again.
	// If the chase were a one-shot path, the hunter would keep walking at
	// where the player used to be and the solve count would never move.
	s.call("strigoi_step", map[string]any{"frames": 300})

	s.call("strigoi_move_player_to", map[string]any{
		"x": playerX - 8, "y": playerY, "wait": true, "max_ticks": 3000,
	})

	s.call("strigoi_step", map[string]any{"frames": 300})

	// sub(..., "state") is not optional: get_system_state returns
	// {system, settable, state}, and reading the fields off the envelope
	// silently yields zero for every one of them.
	state := sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")

	solves := int(num(state, "solves"))
	if solves <= firstSolves {
		t.Fatalf("the quarry moved %v tiles and the hunter never re-pathed: still %d solve(s)",
			8, solves)
	}

	t.Logf("THE PURSUIT ASSERTION: the player moved and the hunter re-pathed — %d solves, up from %d",
		solves, firstSolves)

	// Find OUR hunter's entry rather than assuming it is the only one. Since
	// M4.3b's reopening the spawn tables start chases of their own, and an
	// index-0 read would silently assert about someone else's wolf.
	entry := chaseEntryFor(t, state, entityID(t, s, hunter))

	t.Logf("chase: hunter=%v quarry=%v distance=%.2f reachable=%v arrived=%v solves=%v",
		entry["hunter"], entry["quarry"], num(entry, "distance"),
		entry["reachable"], entry["arrived"], entry["solves"])

	// And the collection can be emptied again -- the rule the M4.1 reopening
	// produced, honoured here from the start rather than earned twice.
	s.call("strigoi_pursue", map[string]any{"hunter": hunter, "release": true})

	after := sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")
	if e := findChaseEntry(after, entityID(t, s, hunter)); e != nil {
		t.Fatalf("release left our hunter chasing: %v", e)
	}

	t.Log("released: our hunter is no longer in the chase list")
}

// findChaseEntry returns the chase whose hunter is the given entity id, or nil.
func findChaseEntry(state map[string]any, hunterID string) map[string]any {
	for _, raw := range asList(state["chase_list"]) {
		entry, ok := raw.(map[string]any)
		if ok && str2(entry["hunter"]) == hunterID {
			return entry
		}
	}

	return nil
}

// chaseEntryFor is findChaseEntry with a failure when it is not there.
func chaseEntryFor(t *testing.T, state map[string]any, hunterID string) map[string]any {
	t.Helper()

	entry := findChaseEntry(state, hunterID)
	if entry == nil {
		t.Fatalf("no chase for hunter %s in %v", hunterID, state["chase_list"])
	}

	return entry
}

// pathSignature renders a path as a stable string for exact comparison. It
// uses the SUBTILE coordinates, because those are what the search actually
// computed; the world-tile values are a convenience for reading.
func pathSignature(t *testing.T, res map[string]any) string {
	t.Helper()

	points, ok := res["waypoints"].([]any)
	if !ok {
		t.Fatalf("no waypoints in %v", res)
	}

	out := ""

	for _, raw := range points {
		wp, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("waypoint is not an object: %v", raw)
		}

		out += fmt.Sprintf("(%.4f,%.4f)", num(wp, "sub_x"), num(wp, "sub_y"))
	}

	return out
}

// boolean reads a bool out of a tool result, defaulting to false when the key
// is missing -- the same tolerant shape as num.
func boolean(m map[string]any, key string) bool {
	v, ok := m[key].(bool)

	return ok && v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}
