//go:build playtest

package playtest

import (
	"math"
	"strings"
	"testing"
)

// TestCraft is T5 (23 Sep 2026): making and mending, fed by the village.
//
//  1. Materials come from labour, not from nowhere: the headman's ditch gives
//     branches, the well's geese feathers, the forge arrowheads and wire.
//  2. THE CONTROL: before he has them, the fletch row is refused and nothing
//     is spent.
//  3. With them, a click on the fletch row makes exactly three arrows and
//     costs exactly 45 world minutes.
//  4. Field mending closes rings in worn mail -- after a fight has worn it --
//     and stops short of new; the smith's mend is refused on sound mail
//     before a minute is spent.
func TestCraft(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Fletcher", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	home := s.call("strigoi_get_player", map[string]any{})

	arrows0 := packCount(t, s, "War arrows")
	if arrows0 != 12 {
		t.Fatalf("the loadout carries twelve war arrows; the panel reads %d (a count this test cannot read proves nothing)", arrows0)
	}

	// --- 2 (first, as the control): nothing to make them from ------------------
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	clickRecipe(t, s, "fletch-arrows")

	if note := str(uiState(s), "kit_notice"); !strings.Contains(note, "not enough") {
		t.Fatalf("act 2: with no materials the fletch row is refused and says why: %q", note)
	}

	if got := packCount(t, s, "War arrows"); got != arrows0 {
		t.Fatalf("act 2: a refused make spends and makes nothing: %d -> %d", arrows0, got)
	}

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- 1: the materials, by labour -------------------------------------------
	headman := villager(t, s, "Warriv")
	walkNear(t, s, headman)
	openTalkWith(t, s, headman)
	answer(t, s, 1) // "Only to live..."
	answer(t, s, 1) // dig: +branches
	answer(t, s, 1) // "Leave."

	setField(s, "village", "rep", 30.0) // trade: the forge talks to him

	well := villager(t, s, "Kashya")
	walkNear(t, s, well)
	openTalkWith(t, s, well)
	answer(t, s, answerIndex(t, s, "pluck")) // +6 feathers

	smith := villager(t, s, "Charsi")
	walkNear(t, s, smith)
	openTalkWith(t, s, smith)

	// The smith's mend on SOUND mail is refused before anything moves.
	before := worldMinutes(t, s)
	answer(t, s, answerIndex(t, s, "mends your mail"))

	view := sub(uiState(s), "talk_view")
	if !strings.Contains(str(view, "notice"), "nothing to mend") || worldMinutes(t, s) != before {
		t.Fatalf("act 4: the smith will not mend sound mail, and no time passes: %v", view)
	}

	answer(t, s, answerIndex(t, s, "arrowheads")) // +6 arrowheads; ends the talk
	openTalkWith(t, s, smith)
	answer(t, s, answerIndex(t, s, "wire")) // +2 wire

	for name, want := range map[string]int{"Branches": 3, "Goose feathers": 6, "Iron arrowheads": 6, "Iron wire": 2} {
		if got := packCount(t, s, name); got != want {
			t.Fatalf("act 1: labour pays %d %s; he carries %d", want, name, got)
		}
	}

	// --- 3: fletching ------------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	before = worldMinutes(t, s)
	clickRecipe(t, s, "fletch-arrows")

	if got := packCount(t, s, "War arrows"); got != arrows0+3 {
		t.Fatalf("act 3: one fletching makes three arrows: %d -> %d", arrows0, got)
	}

	if spent := worldMinutes(t, s) - before; math.Abs(spent-45) > 0.5 {
		t.Fatalf("act 3: fletching takes 45 world minutes; the clock moved %.2f", spent)
	}

	if got := packCount(t, s, "Branches"); got != 2 {
		t.Fatalf("act 3: it spent one branch: %d left", got)
	}

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- 4: field mending after a fight -------------------------------------------
	// Back to open ground where he started: beside the forge the fight's
	// beast spawns against its walls and the fight disengages.
	s.call("strigoi_move_player_to", map[string]any{"x": num(home, "x"), "y": num(home, "y")})

	for i := 0; i < 80; i++ {
		p := s.call("strigoi_get_player", map[string]any{})
		if math.Hypot(num(p, "x")-num(home, "x"), num(p, "y")-num(home, "y")) < 1 {
			break
		}

		s.call("strigoi_step", map[string]any{"frames": 6})
	}

	blocked, absorbed := fightForBlows(t, s)

	// Finish it: he fights back until it is over. Nothing is made in a fight.
	setField(s, "combat", "player_action", "attack")

	for i := 0; i < 120 && flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 12})
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatal("act 4: the fight never ended")
	}

	worn := mailPoints(t, s)
	t.Logf("act 4: the fight: %d blocked, %d absorbed; mail at %d", blocked, absorbed, worn)
	if worn >= 14 {
		t.Fatalf("act 4: the fight must wear the mail for this act to mean anything: %d", worn)
	}

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	clickRecipe(t, s, "field-mend-mail")

	if note := str(uiState(s), "kit_notice"); note != "" {
		t.Logf("act 4: the panel says %q", note)
	}

	mended := mailPoints(t, s)
	if want := int(math.Max(float64(worn), math.Min(float64(worn+4), 10))); mended != want {
		t.Fatalf("act 4: a field mend closes up to 4 points, never past 10 of 14: %d -> %d, want %d", worn, mended, want)
	}

	// Worn only a little -- already past the field cap -- the mend is refused
	// and says so (review finding: this branch was only logged).
	if worn >= 10 && !strings.Contains(str(uiState(s), "kit_notice"), "nothing to mend") {
		t.Fatalf("act 4: mail at %d is past the field cap and the mend must say so: %q", worn, str(uiState(s), "kit_notice"))
	}

	t.Logf("fletched %d -> %d arrows; mail %d -> %d by the field kit", arrows0, arrows0+3, worn, mended)
}

// packCount reads a pack item's count off the kit panel's rows.
func packCount(t *testing.T, s *session, name string) int {
	t.Helper()

	opened := false
	if !flag(t, uiState(s), "kit_open") {
		s.call("strigoi_key", map[string]any{"key": "i"})
		s.call("strigoi_step", map[string]any{"frames": 2})

		opened = true
	}

	total := 0

	for _, raw := range asList(uiState(s)["kit_rows"]) {
		row, _ := raw.(map[string]any)
		if num(row, "pack") < 0 {
			continue
		}

		text := strings.TrimSpace(str(row, "text"))
		if !strings.HasPrefix(text, name) {
			continue
		}

		n := 1
		if i := strings.Index(text, " x"); i >= 0 {
			var c int
			for _, ch := range text[i+2:] {
				if ch < '0' || ch > '9' {
					break
				}

				c = c*10 + int(ch-'0')
			}

			n = c
		}

		total += n
	}

	if opened {
		s.call("strigoi_key", map[string]any{"key": "i"})
		s.call("strigoi_step", map[string]any{"frames": 2})
	}

	return total
}

// clickRecipe clicks a make row on the open kit panel.
func clickRecipe(t *testing.T, s *session, id string) {
	t.Helper()

	for _, raw := range asList(uiState(s)["kit_rows"]) {
		row, _ := raw.(map[string]any)
		if str(row, "recipe") == id {
			s.call("strigoi_click", map[string]any{"x": int(num(row, "x")), "y": int(num(row, "y")), "button": "left"})
			s.call("strigoi_step", map[string]any{"frames": 2})

			return
		}
	}

	t.Fatalf("no make row for %q on the kit panel: %v", id, uiState(s)["kit_rows"])
}

// answer presses answer n (1-based) in the open talk.
func answer(t *testing.T, s *session, n int) {
	t.Helper()

	s.call("strigoi_key", map[string]any{"key": string(rune('0' + n))})
	s.call("strigoi_step", map[string]any{"frames": 2})
}

// answerIndex finds the 1-based answer whose text contains a phrase.
func answerIndex(t *testing.T, s *session, phrase string) int {
	t.Helper()

	for i, a := range asList(sub(uiState(s), "talk_view")["answers"]) {
		if strings.Contains(strings.ToLower(a.(string)), strings.ToLower(phrase)) {
			return i + 1
		}
	}

	t.Fatalf("no answer containing %q: %v", phrase, sub(uiState(s), "talk_view"))

	return 0
}

// mailPoints reads the worn mail's points off the kit panel's body row.
func mailPoints(t *testing.T, s *session) int {
	t.Helper()

	opened := false
	if !flag(t, uiState(s), "kit_open") {
		s.call("strigoi_key", map[string]any{"key": "i"})
		s.call("strigoi_step", map[string]any{"frames": 2})

		opened = true
	}

	points := -1

	for _, raw := range asList(uiState(s)["kit_rows"]) {
		row, _ := raw.(map[string]any)
		if str(row, "slot") != "body" {
			continue
		}

		text := str(row, "text")
		if i := strings.Index(text, " pts"); i > 0 {
			j := i
			for j > 0 && text[j-1] >= '0' && text[j-1] <= '9' {
				j--
			}

			points = 0
			for _, ch := range text[j:i] {
				points = points*10 + int(ch-'0')
			}
		}
	}

	if opened {
		s.call("strigoi_key", map[string]any{"key": "i"})
		s.call("strigoi_step", map[string]any{"frames": 2})
	}

	if points < 0 {
		t.Fatalf("no mail points on the body row: %v", uiState(s)["kit_rows"])
	}

	return points
}
