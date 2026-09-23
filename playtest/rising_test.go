//go:build playtest

package playtest

import (
	"math"
	"testing"
)

// TestRising is M4.7 step 2 (23 Sep 2026): the open dead rise in the deep
// night; a hasty grave and a stake keep them down.
//
//  1. One of Night 1's dead is staked (X), one laid in a hasty grave (D, half
//     an hour); THE CONTROL for the grave: D over a staked body is refused and
//     nothing moves.
//  2. THE CONTROL for the roll: with the odds at 0 a whole night passes and
//     nothing rises -- and soul pressure, never shown, has moved by the rules:
//     -0.01 for the rite, +0.02 for each man left open at dawn.
//  3. With the odds certain (and the grave's weight 0) he starts a grave for a
//     third body a quarter-hour before true dark: the band turns while he
//     digs, the body rises under his spade, and the dig is refused (the M4.7
//     step-2 review: work spends world time). The two left open are up and
//     gone, the grave and the staked body are where they were, and the
//     carrion count has fallen with them; at first light the two lie down
//     again where they stand (step 3), open.
func TestRising(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Sexton", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	// Step 3 stands the risen up to come for him; this script is about the
	// roll, so nothing notices him (the dead's walk is dead_walk_test.go's).
	setField(s, "spawns", "notice_radius", 0.5)
	setField(s, "rising", "edge_floor", 0) // the roll alone: no wanderer at the edge

	c := corpsesState(s)
	if mustNum(t, c, "fresh_human") != 4 {
		t.Fatalf("four of Night 1's dead: %v", c)
	}

	bodies := asList(c["bodies"])
	staked, buried := bodies[0].(map[string]any), bodies[1].(map[string]any)

	// --- 1: a stake and a grave ------------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "k"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	clickRecipe(t, s, "whittle-stake")
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	walkTo(t, s, num(staked, "x"), num(staked, "y"))
	s.call("strigoi_key", map[string]any{"key": "x"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := mustNum(t, corpsesState(s), "closed_human"); got != 1 {
		t.Fatalf("act 1: staked: %v", corpsesState(s))
	}

	before := worldMinutes(t, s)
	s.call("strigoi_key", map[string]any{"key": "d"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if _, ok := corpsesState(s)["hasty_human"]; ok || worldMinutes(t, s)-before > 0.5 {
		t.Fatalf("act 1: no grave for a staked body, and no time spent: %v", corpsesState(s))
	}

	walkTo(t, s, num(buried, "x"), num(buried, "y"))

	before = worldMinutes(t, s)
	s.call("strigoi_key", map[string]any{"key": "d"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := mustNum(t, corpsesState(s), "hasty_human"); got != 1 {
		t.Fatalf("act 1: a hasty grave: %v", corpsesState(s))
	}

	if spent := worldMinutes(t, s) - before; math.Abs(spent-30) > 0.5 {
		t.Fatalf("act 1: a grave is half an hour: %.2f", spent)
	}

	if got := mustNum(t, spawnsState(s), "open_bodies"); got != 2 {
		t.Fatalf("act 1: a grave is not carrion: open_bodies %.0f", got)
	}

	// --- 2: odds 0 (the control) ---------------------------------------------------------------
	setField(s, "rising", "p", 0.0)
	throughTheNight(t, s)

	c = corpsesState(s)
	if mustNum(t, c, "fresh_human") != 2 || mustNum(t, c, "hasty_human") != 1 || mustNum(t, c, "closed_human") != 1 {
		t.Fatalf("act 2: at odds 0 nothing rises: %v", c)
	}

	r := risingState(s)
	if mustNum(t, r, "rolls") != 3 {
		t.Fatalf("act 2: three deep-night bands, three rolls: %v", r)
	}

	if got := mustNum(t, r, "pressure"); math.Abs(got-0.03) > 1e-6 {
		t.Fatalf("act 2: pressure -0.01 for the rite, +0.02 for each of two open at dawn: %.4f", got)
	}

	// --- 3: odds certain ------------------------------------------------------------------------
	setField(s, "rising", "p", 1.0)
	setField(s, "rising", "hasty_weight", 0.0)

	third := bodies[2].(map[string]any)
	walkTo(t, s, num(third, "x"), num(third, "y"))

	for i := 0; i < 200 && mustNum(t, clockState(s), "minute_of_day") < 21*60; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
	}

	if m := mustNum(t, clockState(s), "minute_of_day"); m < 21*60 || m >= 21*60+15 {
		t.Fatalf("act 3: wanted a quarter-hour before true dark (21:15): minute %.0f", m)
	}

	s.call("strigoi_key", map[string]any{"key": "d"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	c = corpsesState(s)
	if mustNum(t, c, "hasty_human") != 1 || mustNum(t, risingState(s), "rolls") != 4 {
		t.Fatalf("act 3: the band turned during the dig and the body rose -- no second grave: %v %v", c, risingState(s))
	}

	if mustNum(t, c, "risen_human") != 2 || mustNum(t, c, "closed_human") != 1 {
		t.Fatalf("act 3: the two left open are up; the grave and the stake held: %v", c)
	}

	if _, ok := c["fresh_human"]; ok {
		t.Fatalf("act 3: no open body of a man is left: %v", c)
	}

	if got := mustNum(t, spawnsState(s), "open_bodies"); got != 0 {
		t.Fatalf("act 3: the carrion count falls with them: %.0f", got)
	}

	throughTheNight(t, s)

	c = corpsesState(s)
	if mustNum(t, c, "downed_human") != 2 || mustNum(t, c, "hasty_human") != 1 || mustNum(t, c, "closed_human") != 1 {
		t.Fatalf("act 3: at first light the two lie down, Downed: %v", c)
	}

	if r := risenGroups(s); r != 0 {
		t.Fatalf("act 3: none left standing after first light: %d", r)
	}

	if r := risingState(s); mustNum(t, r, "rolls") != 6 || math.Abs(mustNum(t, r, "pressure")-0.03) > 1e-6 {
		t.Fatalf("act 3: three more rolls, and no man open at the dawn count to move the pressure: %v", r)
	}

	t.Logf("one staked, one buried, two left open: nothing at odds 0 (pressure 0.03), both up at odds 1 (one under his spade) and down again at first light")
}

func risingState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "rising"}), "state")
}
