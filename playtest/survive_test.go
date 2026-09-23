//go:build playtest

package playtest

import (
	"math"
	"strings"
	"testing"
)

// TestSurvive is T6 (23 Sep 2026): the survival loop's two missing verbs.
//
//  1. He eats from his own pack: a click on the peksimet row feeds exactly one
//     piece's 20 and spends one piece. THE CONTROL: on a full stomach it is
//     refused and nothing is spent.
//  2. The shelter rung means a night indoors: at shelter, by night, the
//     headman lets him sleep in the byre -- four hours pass, 60 fatigue comes
//     off, and no pack arrives even at a spawn chance that is certain outside
//     (THE CONTROL: the same chance, an hour outdoors, brings one).
func TestSurvive(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Survivor", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	// M4.7 step 3: Night 1's dead stay down -- this script's subject is eating and shelter,
	// and a risen man coming for him would be a different script.
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)

	// --- 1: eat ------------------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	clickPackRow(t, s, "Peksimet")

	if note := str(uiState(s), "kit_notice"); !strings.Contains(note, "not hungry") || packCount(t, s, "Peksimet") != 6 {
		t.Fatalf("act 1 control: on a full stomach he does not eat: %q, %d left", note, packCount(t, s, "Peksimet"))
	}

	setField(s, "meters", "food", 50.0)
	clickPackRow(t, s, "Peksimet")

	if food := mustNum(t, metersState(s), "food"); math.Abs(food-70) > 0.5 {
		t.Fatalf("act 1: one piece of hardtack is 20 food: 50 -> %.1f", food)
	}

	if got := packCount(t, s, "Peksimet"); got != 5 {
		t.Fatalf("act 1: one piece is spent: %d left", got)
	}

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- 2: a night indoors ----------------------------------------------------------
	setField(s, "village", "rep", 40.0) // shelter

	// To the night: the clock opens at dawn; step it past dusk.
	for i := 0; i < 40 && str(sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state"), "stage") != "night"; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 60.0})
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
	}

	// THE CONTROL: at a certain chance, an hour outdoors brings a pack.
	setField(s, "spawns", "chance", 1.0)
	groups0 := int(mustNum(t, spawnsState(s), "groups"))
	s.call("strigoi_step_world", map[string]any{"world_minutes": 60.0})

	groups1 := int(mustNum(t, spawnsState(s), "groups"))
	if groups1 <= groups0 {
		t.Fatalf("act 2 control: at chance 1 an hour outdoors by night brings a pack: %d -> %d", groups0, groups1)
	}

	// Chance back to nothing while he walks to the headman (a pack arriving on
	// the way opens a fight, and a fight rightly refuses a talk -- the flake
	// this test first had), and send the control's packs home: the act is
	// about ARRIVALS while he sleeps, not a fight with what already came.
	setField(s, "spawns", "chance", 0)

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		if g, ok := raw.(map[string]any); ok {
			setField(s, "spawns", "despawn", str(g, "group"))
		}
	}

	if live := mustNum(t, spawnsState(s), "live_groups"); live != 0 {
		t.Fatalf("act 2: the control's packs must be gone before he walks: %.0f live", live)
	}

	setField(s, "meters", "fatigue", 80.0)

	headman := villager(t, s, "Warriv")
	walkNear(t, s, headman)

	openTalkWith(t, s, headman)

	if str(villageState(s), "node") == "headman_first" {
		answer(t, s, 3) // say nothing, and go -- met
		openTalkWith(t, s, headman)
	}

	answer(t, s, answerIndex(t, s, "sleep inside"))

	// Certain arrival outside, set while the talk holds the world.
	setField(s, "spawns", "chance", 1.0)

	before := worldMinutes(t, s)
	groups2 := int(mustNum(t, spawnsState(s), "groups"))

	answer(t, s, 1) // sleep four hours

	if slept := worldMinutes(t, s) - before; math.Abs(slept-240) > 0.5 {
		t.Fatalf("act 2: four hours pass: %.1f", slept)
	}

	if fatigue := mustNum(t, metersState(s), "fatigue"); fatigue > 40 {
		t.Fatalf("act 2: sleep takes 60 fatigue off what four hours add: %.1f", fatigue)
	}

	if groups3 := int(mustNum(t, spawnsState(s), "groups")); groups3 > groups2 {
		t.Fatalf("act 2: no pack arrives while he sleeps inside: %d -> %d", groups2, groups3)
	}

	// Once a night: asked again, the headman does not offer the byre.
	openTalkWith(t, s, headman)

	for _, a := range asList(sub(uiState(s), "talk_view")["answers"]) {
		if strings.Contains(a.(string), "sleep inside") {
			t.Fatal("act 2: one sleep a night -- shelter shortens a night, it never skips it")
		}
	}

	s.call("strigoi_key", map[string]any{"key": "escape"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	t.Logf("ate 50 -> 70; slept four hours inside, %d packs before and after", groups2)
}

// clickPackRow clicks the first pack row whose text starts with name.
func clickPackRow(t *testing.T, s *session, name string) {
	t.Helper()

	for _, raw := range asList(uiState(s)["kit_rows"]) {
		row, _ := raw.(map[string]any)
		if num(row, "pack") >= 0 && strings.HasPrefix(strings.TrimSpace(str(row, "text")), name) {
			s.call("strigoi_click", map[string]any{"x": int(num(row, "x")), "y": int(num(row, "y")), "button": "left"})
			s.call("strigoi_step", map[string]any{"frames": 2})

			return
		}
	}

	t.Fatalf("no pack row %q: %v", name, uiState(s)["kit_rows"])
}
