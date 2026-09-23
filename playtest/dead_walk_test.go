//go:build playtest

package playtest

import (
	"math"
	"testing"
)

// TestTheDeadWalk is M4.7 step 3 (23 Sep 2026): the risen stand up in the
// world, come for him, and break off at first light.
//
//  1. With the odds certain and him well off (and nothing able to notice him:
//     the notice model is not this script's subject), the deep night's first
//     band stands Night 1's four dead up where they lay: four bodies risen,
//     four groups of the risen row on the map -- and no fight (the control
//     for the fight in act 3).
//  2. They are still standing at a quarter past two: the dead do not leave
//     before first light.
//  3. The notice radius restored, they find him and come: a fight opens in
//     which every enemy is the risen row, speed 0 (last in every round), and
//     he is not surprised.
//  4. He holds; the rounds carry the clock past first light, and the fight
//     ends "dawn". Each risen lies down where he stood: four open bodies of
//     men again, and no risen group on the map.
func TestTheDeadWalk(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Vigil", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 1.0)

	health := mustNum(t, metersState(s), "health")
	keepAlive := func() {
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
		setField(s, "meters", "health", health)
	}

	c := corpsesState(s)
	if mustNum(t, c, "fresh_human") != 4 {
		t.Fatalf("four of Night 1's dead: %v", c)
	}

	body := asList(c["bodies"])[0].(map[string]any)
	bx, by := num(body, "x"), num(body, "y")

	// --- 1: they rise, and nothing has found him ------------------------------------------------
	// The camp's walls allow about fifteen tiles; the notice radius is taken
	// down until act 3 so the distance is not what the control rests on.
	noticeRadius := mustNum(t, spawnsState(s), "notice_radius")
	setField(s, "spawns", "notice_radius", 0.05)
	walkAwayFrom(t, s, bx, by, 8)

	for i := 0; i < 400 && mustNum(t, risingState(s), "rolls") < 1; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		keepAlive()
	}

	c = corpsesState(s)
	if mustNum(t, c, "risen_human") != 4 {
		t.Fatalf("act 1: at odds 1 the first band stands all four up: %v", c)
	}

	if got := risenGroups(s); got != 4 {
		t.Fatalf("act 1: four of the risen row on the map, got %d", got)
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatal("act 1: nothing has found him yet")
	}

	// --- 2: still standing before first light -----------------------------------------------------
	for i := 0; i < 400; i++ {
		if m := mustNum(t, clockState(s), "minute_of_day"); m >= 2*60+10 && m < 2*60+45 {
			break
		}

		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		keepAlive()
	}

	if st := str(clockState(s), "stage"); st != "night" {
		t.Fatalf("act 2: wanted the last of the night, got %q", st)
	}

	if got := risenGroups(s); got != 4 {
		t.Fatalf("act 2: the dead do not leave before first light: %d risen groups", got)
	}

	// --- 3: he walks back, and they come -----------------------------------------------------------
	setField(s, "spawns", "notice_radius", noticeRadius)
	setField(s, "combat", "player_action", "hold")
	setField(s, "combat", "round_minutes", 5.0)

	// He stands where he is: a walk still in flight when the fight opens
	// carries him out of it (measured: "disengaged" one round in). The dead
	// come to him.
	for i := 0; i < 200 && !flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 6})
	}

	fight := combatState(s)
	if !flag(t, fight, "fighting") {
		t.Fatalf("act 3: at the bodies' place, the dead find him: %v", fight)
	}

	if flag(t, fight, "surprised") {
		t.Fatalf("act 3: the dead take neither surprise branch: %v", fight)
	}

	enemies := 0

	for _, raw := range asList(fight["participants"]) {
		row := raw.(map[string]any)
		if str(row, "side") != "enemy" {
			continue
		}

		enemies++

		if str(row, "profile") != "risen" || num(row, "speed") != 0 {
			t.Fatalf("act 3: every enemy is the risen row at speed 0: %v", row)
		}
	}

	if enemies == 0 {
		t.Fatalf("act 3: a fight with no enemy in it: %v", fight)
	}

	t.Logf("act 3: the fight opened at %s with %d of the dead: %v", str(clockState(s), "time_of_day"), enemies, fight["participants"])

	// --- 4: first light ------------------------------------------------------------------------------
	for i := 0; i < 400 && flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 12})
		keepAlive()
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatalf("act 4: the fight outlasted the night: %v", clockState(s))
	}

	if why := str(combatState(s), "ended_reason"); why != "dawn" {
		t.Fatalf("act 4: the dead break off at first light: ended %q at %s (%s); combat %v",
			why, str(clockState(s), "time_of_day"), str(clockState(s), "stage"), combatState(s))
	}

	s.call("strigoi_step", map[string]any{"frames": 2})

	c = corpsesState(s)
	if mustNum(t, c, "fresh_human") != 4 {
		t.Fatalf("act 4: each risen lies down, open again: %v", c)
	}

	if _, ok := c["risen_human"]; ok {
		t.Fatalf("act 4: none left standing: %v", c)
	}

	if got := risenGroups(s); got != 0 {
		t.Fatalf("act 4: no risen group on the map: %d", got)
	}

	if got := mustNum(t, combatState(s), "ended_dawn"); got != 1 {
		t.Fatalf("act 4: one fight left at first light: %.0f", got)
	}

	t.Logf("four rose, came for him at 02:15, and lay down at first light (%d in the fight)", enemies)
}

// risenGroups counts the risen row's groups on the map.
func risenGroups(s *session) int {
	n := 0

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		if g, ok := raw.(map[string]any); ok && str(g, "row") == "risen" {
			n++
		}
	}

	return n
}

// walkAwayFrom walks him at least min tiles from a point, in whichever
// direction the map allows.
func walkAwayFrom(t *testing.T, s *session, x, y, min float64) {
	t.Helper()

	d := min + 4

	for _, off := range [][2]float64{{d, 0}, {-d, 0}, {0, d}, {0, -d}, {d, d}, {-d, -d}, {d, -d}, {-d, d}} {
		s.call("strigoi_move_player_to", map[string]any{"x": x + off[0], "y": y + off[1]})

		for i := 0; i < 120; i++ {
			p := s.call("strigoi_get_player", map[string]any{})
			if math.Hypot(num(p, "x")-x, num(p, "y")-y) >= min {
				return
			}

			s.call("strigoi_step", map[string]any{"frames": 6})
		}
	}

	t.Fatalf("could not walk %.0f tiles from %.1f,%.1f", min, x, y)
}
