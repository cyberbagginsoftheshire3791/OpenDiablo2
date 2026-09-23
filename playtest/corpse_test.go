//go:build playtest

package playtest

import (
	"math"
	"strings"
	"testing"
)

// TestCorpses is M4.7 step 1 (23 Sep 2026): the slain are open bodies, and the
// stake closes one.
//
//  1. Night 1's dead (Q2a PLACEHOLDER) lie near where he enters: four open
//     bodies of men, and the carrion count reads them.
//  2. THE CONTROL: at a body with no stake, X is refused and nothing moves.
//  3. Foraged branch -> whittled stake -> X at the body: five minutes, the
//     stake spent, the body Closed, the carrion count one lower.
//  4. A beast he kills falls as carrion: the count rises, and X will not stake
//     a carcass (Q1a).
func TestCorpses(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gravedigger", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	// --- 1: Night 1's dead ------------------------------------------------------------
	c := corpsesState(s)
	if mustNum(t, c, "open") != 4 || mustNum(t, c, "fresh_human") != 4 {
		t.Fatalf("act 1: four open bodies of men lie near where he enters: %v", c)
	}

	if got := mustNum(t, spawnsState(s), "open_bodies"); got != 4 {
		t.Fatalf("act 1: the carrion count reads them: %.0f", got)
	}

	shot := s.call("strigoi_screenshot", map[string]any{"name": "m47-the-dead"})
	t.Logf("act 1: the dead's marks, %s", str(shot, "path"))

	body := asList(c["bodies"])[0].(map[string]any)

	// --- 2: no stake (the control) -------------------------------------------------------
	walkTo(t, s, num(body, "x"), num(body, "y"))

	before := worldMinutes(t, s)
	s.call("strigoi_key", map[string]any{"key": "x"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if mustNum(t, corpsesState(s), "open") != 4 || worldMinutes(t, s)-before > 0.5 {
		t.Fatalf("act 2: without a stake nothing is staked and no time passes: %v", corpsesState(s))
	}

	// --- 3: forage, whittle, stake ---------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "k"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	clickRecipe(t, s, "whittle-stake")
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := packCount(t, s, "Stake"); got != 1 {
		t.Fatalf("act 3: a whittled stake in the pack: %d", got)
	}

	walkTo(t, s, num(body, "x"), num(body, "y"))

	before = worldMinutes(t, s)
	s.call("strigoi_key", map[string]any{"key": "x"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	c = corpsesState(s)
	if mustNum(t, c, "open") != 3 || mustNum(t, c, "closed_human") != 1 {
		t.Fatalf("act 3: the stake closes one: %v", c)
	}

	if spent := worldMinutes(t, s) - before; math.Abs(spent-5) > 0.5 {
		t.Fatalf("act 3: staking takes 5 world minutes: %.2f", spent)
	}

	if got := packCount(t, s, "Stake"); got != 0 {
		t.Fatalf("act 3: the stake is spent in the body: %d left", got)
	}

	if got := mustNum(t, spawnsState(s), "open_bodies"); got != 3 {
		t.Fatalf("act 3: the carrion count falls with it: %.0f", got)
	}

	// --- 4: a beast is carrion ---------------------------------------------------------------
	p := s.call("strigoi_get_player", map[string]any{})
	dog := spawnNPC(t, s, "fallen1", num(p, "x")+1, num(p, "y"))
	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": str(p, "handle")})
	fightNow(t, s)
	finishFight(t, s)

	c = corpsesState(s)
	if mustNum(t, c, "fresh_beast") != 1 || mustNum(t, c, "open") != 4 {
		t.Fatalf("act 4: the dog he killed lies as carrion: %v", c)
	}

	if got := mustNum(t, spawnsState(s), "open_bodies"); got != 4 {
		t.Fatalf("act 4: carrion counts toward the beasts' draw: %.0f", got)
	}

	var carcass map[string]any
	for _, raw := range asList(c["bodies"]) {
		if b := raw.(map[string]any); str(b, "class") == "beast" {
			carcass = b
		}
	}

	walkTo(t, s, num(carcass, "x"), num(carcass, "y"))

	// A stake in hand, so the refusal is the carcass's and not the kit's.
	s.call("strigoi_key", map[string]any{"key": "k"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	clickRecipe(t, s, "whittle-stake")
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	s.call("strigoi_key", map[string]any{"key": "x"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := mustNum(t, corpsesState(s), "fresh_beast"); got != 1 || packCount(t, s, "Stake") != 1 {
		t.Fatalf("act 4: a carcass is not staked -- it does not rise: %v", corpsesState(s))
	}

	t.Logf("four placed dead, one staked; a dog's carcass counted as carrion and left alone")
}

func corpsesState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "corpses"}), "state")
}

// walkTo walks him onto a tile and fails if he cannot get within a tile.
func walkTo(t *testing.T, s *session, x, y float64) {
	t.Helper()

	s.call("strigoi_move_player_to", map[string]any{"x": x, "y": y})

	for i := 0; i < 100; i++ {
		p := s.call("strigoi_get_player", map[string]any{})
		if math.Hypot(num(p, "x")-x, num(p, "y")-y) <= 1 {
			return
		}

		s.call("strigoi_step", map[string]any{"frames": 6})
	}

	p := s.call("strigoi_get_player", map[string]any{})
	if d := math.Hypot(num(p, "x")-x, num(p, "y")-y); d > 1.4 {
		t.Fatalf("could not walk to %.1f,%.1f: %.1f tiles short (%s)", x, y, d, strings.TrimSpace(str(p, "handle")))
	}
}
