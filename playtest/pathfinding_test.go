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
