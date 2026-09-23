//go:build playtest

package playtest

import (
	"math"
	"strings"
	"testing"
)

// TestForage is T7 (23 Sep 2026): the gathering verb, and the rule it makes
// live.
//
//  1. K sends him head-down for exactly 30 world minutes and he comes back
//     with 2 branches, from a land that holds 12.
//  2. THE CONTROL: a beast that reaches a man standing idle opens an ordinary
//     fight -- not a surprise.
//  3. The same beast reaching him while he forages opens the fight with him
//     CAUGHT (D8 §9, "caught-foraging"), and he gathers nothing.
//  4. The land runs out: six forages empty it and a seventh is refused.
func TestForage(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Forager", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	// --- 1: a forage ---------------------------------------------------------------
	before := worldMinutes(t, s)
	s.call("strigoi_key", map[string]any{"key": "k"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if spent := worldMinutes(t, s) - before; math.Abs(spent-30) > 0.5 {
		t.Fatalf("act 1: a forage is 30 world minutes; the clock moved %.2f", spent)
	}

	if got := packCount(t, s, "Branches"); got != 2 {
		t.Fatalf("act 1: a forage brings back 2 branches; he carries %d", got)
	}

	if left := mustNum(t, villageState(s), "land_left"); left != 10 {
		t.Fatalf("act 1: the land had 12 and gave 2: %.0f left", left)
	}

	// T9: and the kit panel says so.
	if !kitSays(t, s, "Land: 10 branches left") {
		t.Fatal("act 1: the kit panel's status line shows what the land still holds")
	}

	// --- 2: the control -- idle when it arrives ------------------------------------
	p := s.call("strigoi_get_player", map[string]any{})
	dog := spawnNPC(t, s, "fallen1", num(p, "x")+1, num(p, "y"))
	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": str(p, "handle")})
	fightNow(t, s)

	if c := combatState(s); flag(t, c, "surprised") {
		t.Fatalf("act 2 control: reached while idle, he is not caught: %v", c["surprise_why"])
	}

	finishFight(t, s)

	// --- 3: caught foraging -----------------------------------------------------------
	p = s.call("strigoi_get_player", map[string]any{})
	dog = spawnNPC(t, s, "fallen1", num(p, "x")+1, num(p, "y"))
	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": str(p, "handle")})

	s.call("strigoi_key", map[string]any{"key": "k"}) // head-down before it has reached him
	s.call("strigoi_step", map[string]any{"frames": 2})

	c := combatState(s)
	if !flag(t, c, "fighting") || !flag(t, c, "surprised") || str(c, "surprise_why") != "caught-foraging" {
		t.Fatalf("act 3: a beast that reaches him mid-forage catches him head-down: fighting %v surprised %v why %q",
			c["fighting"], c["surprised"], str(c, "surprise_why"))
	}

	if got := packCount(t, s, "Branches"); got != 2 {
		t.Fatalf("act 3: caught at it, he gathers nothing: %d branches", got)
	}

	finishFight(t, s)

	// And it passes: the fight over, he is no longer bent over the brush. The
	// review traced the first version leaving him "foraging" for good, so that
	// every later fight opened with him caught.
	if act := str(metersState(s), "activity"); act != "idle" {
		t.Fatalf("act 3: after the fight his stance is idle again, not %q", act)
	}

	p = s.call("strigoi_get_player", map[string]any{})
	dog = spawnNPC(t, s, "fallen1", num(p, "x")+1, num(p, "y"))
	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": str(p, "handle")})
	fightNow(t, s)

	if c := combatState(s); flag(t, c, "surprised") {
		t.Fatalf("act 3: the NEXT beast finds him idle, not caught: %v", c["surprise_why"])
	}

	finishFight(t, s)

	// --- 4: the land runs out -----------------------------------------------------------
	for i := 0; i < 5; i++ {
		s.call("strigoi_key", map[string]any{"key": "k"})
		s.call("strigoi_step", map[string]any{"frames": 2})
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
	}

	if got, left := packCount(t, s, "Branches"), mustNum(t, villageState(s), "land_left"); got != 12 || left != 0 {
		t.Fatalf("act 4: twelve branches gathered and the land bare: %d, %.0f left", got, left)
	}

	before = worldMinutes(t, s)
	s.call("strigoi_key", map[string]any{"key": "k"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := packCount(t, s, "Branches"); got != 12 || worldMinutes(t, s)-before > 0.5 {
		t.Fatalf("act 4: a bare land gives nothing and takes no time: %d branches", got)
	}

	// --- 5: the land remembers --------------------------------------------------
	savePath := str(game, "save_path")

	if savePath != "" {
		s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
		s.call("strigoi_step", map[string]any{"frames": 30})
		s.call("strigoi_start_game", map[string]any{"save_path": savePath, "seed": 1462, "wait_seconds": 90})

		if left := mustNum(t, villageState(s), "land_left"); left != 0 {
			t.Fatalf("act 5: a bare land stays bare across a reload: %.0f left", left)
		}
	} else {
		t.Fatalf("act 5: no save path to reload from: %v", game)
	}

	t.Logf("foraged 12 branches; caught once head-down; the land stays bare across a reload")
}

// finishFight has him fight until it is over.
func finishFight(t *testing.T, s *session) {
	t.Helper()

	setField(s, "combat", "player_action", "attack")

	for i := 0; i < 150 && flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 12})
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatal("the fight never ended")
	}
}

// kitSays opens the kit panel, reports whether any row contains text, and
// closes it again.
func kitSays(t *testing.T, s *session, text string) bool {
	t.Helper()

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	found := false

	for _, raw := range asList(uiState(s)["kit_rows"]) {
		if row, ok := raw.(map[string]any); ok && strings.Contains(str(row, "text"), text) {
			found = true
		}
	}

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	return found
}
