//go:build playtest

package playtest

import (
	"math"
	"testing"
)

// TestTheHearth is M4.7 step 4 (23 Sep 2026): the priest's rite, the hearth
// unlock, and a staking the village sees.
//
//  1. By day, with the whole village "watching" (seen_radius wide), a stake
//     costs standing -5 and marks it seen; a second stake by day costs nothing
//     more (the first time only).
//  2. THE CONTROLS: a hasty grave dug before the rite is granted is still a
//     hasty grave after first light; and a risen man, before the priest's
//     tale, carries no bar and the hover does not call him the dead.
//  3. At the hearth the priest tells the tale (heard_tale) and grants the rite
//     (rite_granted).
//  4. The next night the risen man carries a bar and the hover names him
//     "the dead"; at first light the priest closes the grave.
func TestTheHearth(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Sexton", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "spawns", "notice_radius", 0.05) // the dead stand; they never come
	setField(s, "rising", "p", 1.0)
	setField(s, "rising", "hasty_weight", 0.0)
	setField(s, "village", "rep", 60.0)
	setField(s, "village", "rite_radius", 500.0) // the church, wherever the map put the dead
	setField(s, "village", "seen_radius", 500.0) // the whole village is watching

	bodies := asList(corpsesState(s)["bodies"])
	grave, stakedA, stakedB, door := bodies[0].(map[string]any), bodies[1].(map[string]any),
		bodies[2].(map[string]any), bodies[3].(map[string]any)

	// --- 1: seen staking ------------------------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "k"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	clickRecipe(t, s, "whittle-stake")
	clickRecipe(t, s, "whittle-stake")
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := packCount(t, s, "Stake"); got != 2 {
		t.Fatalf("act 1: two stakes whittled: %d", got)
	}

	walkTo(t, s, num(stakedA, "x"), num(stakedA, "y"))
	s.call("strigoi_key", map[string]any{"key": "x"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if rep := mustNum(t, villageState(s), "rep"); rep != 55 || !hasFlag(t, s, "seen_staking") {
		t.Fatalf("act 1: a staking seen by day costs 5, once: rep %.0f %v", rep, villageState(s))
	}

	walkTo(t, s, num(stakedB, "x"), num(stakedB, "y"))
	s.call("strigoi_key", map[string]any{"key": "x"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if rep := mustNum(t, villageState(s), "rep"); rep != 55 || mustNum(t, corpsesState(s), "closed_human") != 2 {
		t.Fatalf("act 1: the second staking is staked and costs nothing more: rep %.0f %v", rep, corpsesState(s))
	}

	// Two stakes were two rites: soul pressure is back to nothing, so the
	// odds are the p this script set.
	setField(s, "rising", "pressure", 0.0)

	// --- 2: before the hearth (the controls) ----------------------------------------------------
	walkTo(t, s, num(grave, "x"), num(grave, "y"))
	s.call("strigoi_key", map[string]any{"key": "d"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if mustNum(t, corpsesState(s), "hasty_human") != 1 {
		t.Fatalf("act 2: a hasty grave: %v", corpsesState(s))
	}

	setField(s, "village", "rep", 60.0)
	walkNearPoint(t, s, num(door, "x"), num(door, "y"))
	untilRisen(t, s)

	if n := enemyBars(t, uiState(s)); n != 0 {
		t.Fatalf("act 2: before the tale the dead carry no bar: %d enemy bars", n)
	}

	if got := hoverAt(t, s, num(door, "x"), num(door, "y")); got == "the dead" || got == "" {
		t.Fatalf("act 2: before the tale he looks like a man: hover %q", got)
	}

	throughTheNight(t, s)

	if mustNum(t, corpsesState(s), "hasty_human") != 1 {
		t.Fatalf("act 2: without the rite the grave is not closed at first light: %v", corpsesState(s))
	}

	// --- 3: the hearth ----------------------------------------------------------------------------
	priest := villager(t, s, "Akara")
	walkNear(t, s, priest)
	openTalkWith(t, s, priest)
	answer(t, s, answerIndex(t, s, "Listen"))
	answer(t, s, answerIndex(t, s, "Thank him"))

	openTalkWith(t, s, priest)
	answer(t, s, answerIndex(t, s, "rite for the dead"))
	answer(t, s, answerIndex(t, s, "Thank him"))

	if !hasFlag(t, s, "heard_tale") || !hasFlag(t, s, "rite_granted") {
		t.Fatalf("act 3: the tale told and the rite granted: %v", villageState(s))
	}

	// --- 4: after the hearth ----------------------------------------------------------------------
	// He lay down at first light where he stood, which need not be where the
	// body first lay: read the body again.
	for _, raw := range asList(corpsesState(s)["bodies"]) {
		if b := raw.(map[string]any); str(b, "id") == str(door, "id") {
			door = b
		}
	}

	walkNearPoint(t, s, num(door, "x"), num(door, "y"))
	untilRisen(t, s)

	if n := enemyBars(t, uiState(s)); n < 1 {
		t.Fatalf("act 4: after the tale the dead carry a bar: %d enemy bars", n)
	}

	if got := hoverAt(t, s, num(door, "x"), num(door, "y")); got != "the dead" {
		t.Fatalf("act 4: after the tale the hover names him: %q", got)
	}

	throughTheNight(t, s)

	c := corpsesState(s)
	if _, ok := c["hasty_human"]; ok || mustNum(t, c, "closed_human") != 3 {
		t.Fatalf("act 4: at first light the priest closes the grave: %v", c)
	}

	t.Logf("seen once (-5), the grave held until the rite, the dead unnamed until the tale")
}

// untilRisen steps the world, feeding him, until a risen group stands.
func untilRisen(t *testing.T, s *session) {
	t.Helper()

	for i := 0; i < 250 && risenGroups(s) == 0; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
	}

	if risenGroups(s) == 0 {
		t.Fatalf("no risen stood: %v %v", corpsesState(s), risingState(s))
	}

	s.call("strigoi_step", map[string]any{"frames": 4})
}

// hoverAt puts the cursor on the nearest NPC to a point and reads the label.
func hoverAt(t *testing.T, s *session, x, y float64) string {
	t.Helper()

	near := s.call("strigoi_get_entities", map[string]any{"kind": "npc", "near": []float64{x, y, 3}, "limit": 10})

	best, bestD := "", math.Inf(1)

	for _, raw := range asList(near["items"]) {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if d := math.Hypot(num(row, "x")-x, num(row, "y")-y); d < bestD {
			best, bestD = str(row, "handle"), d
		}
	}

	if best == "" {
		t.Fatalf("no NPC within 3 tiles of %.1f,%.1f", x, y)
	}

	sx, sy := screenOf(t, s, best)
	s.call("strigoi_move_cursor", map[string]any{"x": sx, "y": sy})
	s.call("strigoi_step", map[string]any{"frames": 2})

	return str(uiState(s), "hover_label")
}

// walkNearPoint walks him to within about three tiles of a point.
func walkNearPoint(t *testing.T, s *session, x, y float64) {
	t.Helper()

	s.call("strigoi_move_player_to", map[string]any{"x": x + 2, "y": y + 2})

	for i := 0; i < 100; i++ {
		p := s.call("strigoi_get_player", map[string]any{})
		if math.Hypot(num(p, "x")-x, num(p, "y")-y) <= 3.5 {
			return
		}

		s.call("strigoi_step", map[string]any{"frames": 6})
	}
}
