//go:build playtest

package playtest

import (
	"testing"
)

// TestTheDownedDead is M4.7 step 3b (23 Sep 2026): a risen man cut down lies
// Downed, the fight holds, and only a stake keeps him down.
//
//  1. By day three of Night 1's dead are staked, so one door is left.
//  2. At night he rises and comes; under the player's own control, strikes
//     cut him down: he lies Downed and the fight does NOT end.
//  3. THE CONTROL: left unstaked, turn after turn, his window runs out and he
//     stands again -- a new body in the same fight, at full health.
//  4. Cut down again, X on the player's turn drives the stake through him as
//     the Action: the body is Closed, a stake is spent, and the fight is won.
func TestTheDownedDead(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Stakeman", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
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

	bodies := asList(corpsesState(s)["bodies"])
	door := bodies[0].(map[string]any)

	// --- 1: three stakes by day ------------------------------------------------------------------
	for i := 0; i < 2; i++ {
		s.call("strigoi_key", map[string]any{"key": "k"})
		s.call("strigoi_step", map[string]any{"frames": 2})
	}

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	for i := 0; i < 4; i++ {
		clickRecipe(t, s, "whittle-stake")
	}

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := packCount(t, s, "Stake"); got != 4 {
		t.Fatalf("act 1: four stakes whittled: %d", got)
	}

	for _, raw := range bodies[1:] {
		b := raw.(map[string]any)
		walkTo(t, s, num(b, "x"), num(b, "y"))
		s.call("strigoi_key", map[string]any{"key": "x"})
		s.call("strigoi_step", map[string]any{"frames": 2})
	}

	if got := mustNum(t, corpsesState(s), "closed_human"); got != 3 {
		t.Fatalf("act 1: three staked, one door left: %v", corpsesState(s))
	}

	// --- 2: he comes, and falls Downed ---------------------------------------------------------------
	noticeRadius := mustNum(t, spawnsState(s), "notice_radius")
	setField(s, "spawns", "notice_radius", 0.05)
	walkAwayFrom(t, s, num(door, "x"), num(door, "y"), 6)

	for i := 0; i < 400 && risenGroups(s) == 0; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		keepAlive()
	}

	if risenGroups(s) != 1 {
		t.Fatalf("act 2: the one door stands: %v", corpsesState(s))
	}

	setField(s, "combat", "player_control", "human")
	setField(s, "spawns", "notice_radius", noticeRadius)

	for i := 0; i < 300 && !flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 6})
		keepAlive()
	}

	if !flag(t, combatState(s), "fighting") {
		t.Fatalf("act 2: he comes for him: %v", combatState(s))
	}

	cutDown(t, s, keepAlive, "act 2")

	if !flag(t, combatState(s), "fighting") {
		t.Fatalf("act 2: with the dead man Downed the fight holds: ended %q", str(combatState(s), "ended_reason"))
	}

	// --- 3: unstaked, he stands again (the control) ------------------------------------------------
	for i := 0; i < 20 && mustNum(t, risingState(s), "stood_again") < 1; i++ {
		openTurn(t, s)
		s.call("strigoi_key", map[string]any{"key": "e"}) // hold: the Action unspent
		s.call("strigoi_step", map[string]any{"frames": 4})
		keepAlive()
	}

	if mustNum(t, risingState(s), "stood_again") != 1 || mustNum(t, corpsesState(s), "risen_human") != 1 {
		t.Fatalf("act 3: his window run out, he stands again: %v %v", risingState(s), corpsesState(s))
	}

	// ...and he is back in the same fight at once, at full health (Rejoin).
	fight := combatState(s)
	if !flag(t, fight, "fighting") {
		t.Fatalf("act 3: he stands in the fight he fell in: ended %q", str(fight, "ended_reason"))
	}

	standing := 0

	for _, raw := range asList(fight["participants"]) {
		if row := raw.(map[string]any); str(row, "side") == "enemy" && str(row, "profile") == "risen" && row["dead"] == false {
			standing++
		}
	}

	if standing != 1 {
		t.Fatalf("act 3: one of the dead on his feet in the fight: %v", fight["participants"])
	}

	// --- 4: cut down again, and staked as the Action --------------------------------------------------
	cutDown(t, s, keepAlive, "act 4")

	// The killing blow spent this turn's Action: close it, and stake on the
	// next -- still inside his three-round window.
	if flag(t, combatState(s), "awaiting") && flag(t, combatState(s), "action_spent") {
		s.call("strigoi_key", map[string]any{"key": "e"})
		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	openTurn(t, s)

	stakes := packCount(t, s, "Stake")
	s.call("strigoi_key", map[string]any{"key": "x"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := mustNum(t, corpsesState(s), "closed_human"); got != 4 {
		t.Fatalf("act 4: the stake through the Downed man closes him: %v (combat %v)", corpsesState(s), combatState(s))
	}

	if got := packCount(t, s, "Stake"); got != stakes-1 {
		t.Fatalf("act 4: one stake spent: %d -> %d", stakes, got)
	}

	for i := 0; i < 100 && flag(t, combatState(s), "fighting"); i++ {
		if flag(t, combatState(s), "awaiting") {
			s.call("strigoi_key", map[string]any{"key": "e"})
		}

		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	if flag(t, combatState(s), "fighting") || str(combatState(s), "ended_reason") != "enemies_dead" {
		t.Fatalf("act 4: staked, the fight is won: %v", combatState(s))
	}

	t.Logf("cut down twice; stood again once unstaked; staked as the Action the second time")
}

// cutDown strikes on each of his turns until the one risen man is Downed.
func cutDown(t *testing.T, s *session, keepAlive func(), act string) {
	t.Helper()

	for i := 0; i < 40; i++ {
		if _, ok := corpsesState(s)["downed_human"]; ok {
			return
		}

		openTurn(t, s)
		s.call("strigoi_key", map[string]any{"key": "f"})
		s.call("strigoi_step", map[string]any{"frames": 2})

		if _, ok := corpsesState(s)["downed_human"]; ok {
			return
		}

		if flag(t, combatState(s), "awaiting") {
			s.call("strigoi_key", map[string]any{"key": "e"})
			s.call("strigoi_step", map[string]any{"frames": 4})
		}

		keepAlive()
	}

	t.Fatalf("%s: forty turns and he is not down: %v %v", act, corpsesState(s), combatState(s))
}
