//go:build playtest

package playtest

import (
	"strings"
	"testing"
)

// TestWritings is J2 (24 Sep 2026): the things he can read, and what reading
// gives him. Acts:
//
//  1. At the hearth the priest reads him the book on the stand: the talk
//     ends, his journal opens at the writing, its text is there, and the
//     first read pays its experience (5).
//  2. THE CONTROL: read again, the journal opens at it again and pays
//     nothing.
//  3. The list of their dead gives a tip: "Only on Saturday" is written
//     before the priest's tale, which is otherwise the only way to it.
//  4. The places: the church, where the priest keeps his books, is in the
//     village part, and from eight tiles off it says which way and how far.
//     The well woman's answer about their dead writes the churchyard.
func TestWritings(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Reader", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	setField(s, "village", "rep", 50.0) // the hearth
	s.call("strigoi_step", map[string]any{"frames": 4})

	xp := func() float64 { return mustNum(t, progressState(s), "xp") }

	priest := villager(t, s, "Akara")
	walkNear(t, s, priest)

	read := func(act, phrase string) {
		t.Helper()

		talkTo(t, s, priest)
		answer(t, s, answerIndex(t, s, "church's writings"))
		answer(t, s, answerIndex(t, s, phrase))

		ui := uiState(s)
		if flag(t, ui, "talk_open") || !flag(t, ui, "journal_open") {
			t.Fatalf("%s: the read ends the talk and opens the journal: talk %v, journal %v",
				act, flag(t, ui, "talk_open"), flag(t, ui, "journal_open"))
		}
	}

	closeJournal := func() {
		s.call("strigoi_key", map[string]any{"key": "escape"})
		s.call("strigoi_step", map[string]any{"frames": 2})
	}

	// --- 1: the first read ------------------------------------------------------------------
	before := xp()
	read("act 1", "book open on the stand")

	view := sub(uiState(s), "journal_view")
	if str(view, "part") != "writings" || str(view, "selected") != "w_r01" {
		t.Fatalf("act 1: the journal opens at the writing: part %q, selected %q", str(view, "part"), str(view, "selected"))
	}

	if text := strings.Join(strs(view["text"]), " "); !strings.Contains(text, "He that dwells in the help of the Highest") {
		t.Fatalf("act 1: the writing's text is on the page: %q", text)
	}

	shot := s.call("strigoi_screenshot", map[string]any{"name": "writings-psalter"})
	t.Logf("the psalter, read: %s", str(shot, "path"))

	if got := xp() - before; got != 5 {
		t.Fatalf("act 1: the first read pays 5; it paid %.0f", got)
	}

	closeJournal()

	// --- 2: read again (the control) ----------------------------------------------------------
	before = xp()
	read("act 2", "book open on the stand")

	if got := xp() - before; got != 0 || num(sub(journalState(s), "reads"), "R01") != 2 {
		t.Fatalf("act 2: a second read pays nothing and is counted: +%.0f, reads %v", got, journalState(s)["reads"])
	}

	closeJournal()

	// --- 3: a tip -------------------------------------------------------------------------------
	if journalWrote(journalState(s), "d_saturday") {
		t.Fatal("act 3: \"Only on Saturday\" before the tale or the list; the tip would prove nothing")
	}

	before = xp()
	deadUnread := num(sub(journalState(s), "unread"), "dead")
	read("act 3", "list of names")

	if !journalWrote(journalState(s), "d_saturday") || xp()-before != 10 {
		t.Fatalf("act 3: the list of their dead writes \"Only on Saturday\" and pays 10: %v, +%.0f",
			journalState(s)["written"], xp()-before)
	}

	// The tip is still unread where it was written, and said once he closes
	// the journal (J2 review B2).
	if num(sub(journalState(s), "unread"), "dead") != deadUnread+1 {
		t.Fatalf("act 3: the tip is unread in the dead part: %v", journalState(s)["unread"])
	}

	closeJournal()

	if n := str(uiState(s), "journal_notice"); !strings.Contains(n, "Only on Saturday") {
		t.Fatalf("act 3: closing the journal says what the read wrote: %q", n)
	}

	// --- 4: places ------------------------------------------------------------------------------
	well := villager(t, s, "Kashya")
	talkTo(t, s, well)
	answer(t, s, answerIndex(t, s, "where their dead are buried"))

	if !journalWrote(journalState(s), "p_churchyard") || !journalWrote(journalState(s), "w_r08") {
		t.Fatalf("act 4: the well woman's answer writes the graves and the churchyard: %v", journalState(s)["written"])
	}

	closeJournal()
	walkFrom(t, s, priest, 8)

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	clickJournalTab(t, s, "village")

	for _, raw := range asList(sub(uiState(s), "journal_view")["rows"]) {
		row := raw.(map[string]any)
		if str(row, "id") != "p_church" {
			continue
		}

		s.call("strigoi_click", map[string]any{"x": int(num(row, "x")) + 20, "y": int(num(row, "y")) + 6, "button": "left"})
		s.call("strigoi_step", map[string]any{"frames": 2})

		text := strings.Join(strs(sub(uiState(s), "journal_view")["text"]), " ")
		if !strings.Contains(text, "It lies ") || !strings.Contains(text, " of me, about ") {
			t.Fatalf("act 4: eight tiles off, the church says which way and how far: %q", text)
		}

		shot = s.call("strigoi_screenshot", map[string]any{"name": "writings-place"})
		t.Logf("the church, placed: %s (%s)", str(shot, "path"), text)

		return
	}

	t.Fatalf("act 4: no church in the village part: %v", sub(uiState(s), "journal_view")["rows"])
}

// talkTo opens a talk with a villager, walking back to him and clicking again
// if the first click lands on the ground: in the full suite a click at 1.8
// tiles once found no hover (the hero's own sprite over the priest's) and
// walked him instead (measured 24 Sep).
func talkTo(t *testing.T, s *session, handle string) {
	t.Helper()

	for try := 0; try < 3; try++ {
		walkNear(t, s, handle)

		x, y := screenOf(t, s, handle)
		s.call("strigoi_click", map[string]any{"x": x, "y": y, "button": "left"})
		s.call("strigoi_step", map[string]any{"frames": 2})

		if flag(t, uiState(s), "talk_open") {
			return
		}
	}

	openTalkWith(t, s, handle) // the last try, with openTalkWith's diagnosis
}
