//go:build playtest

package playtest

import (
	"math"
	"testing"
)

// TestSearch is J2b (24 Sep 2026): U searches the dead man at his feet, and
// Night 1's first two dead carry the last two writings. Acts:
//
//  1. U at the first of the four: ten minutes head-down, then his journal
//     opens at the comrade's amulet (w_r09) and the first read pays 5.
//  2. U at the second: the Sultan's paper (w_r10), 10, and a task opens --
//     he must be able to say how he came by it.
//  3. U at the third: nothing he can read -- no page, no read, but the
//     minutes are spent.
//  4. THE CONTROL: U at the first again opens the amulet again and pays
//     nothing.
//  5. THE CONTROL: U with no body at his feet is refused and takes no time.
func TestSearch(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Searcher", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	s.call("strigoi_step", map[string]any{"frames": 4})

	bodies := asList(corpsesState(s)["bodies"])
	if len(bodies) < 3 {
		t.Fatalf("Night 1's dead: want at least three, got %v", bodies)
	}

	body := func(i int) map[string]any { return bodies[i].(map[string]any) }
	xp := func() float64 { return mustNum(t, progressState(s), "xp") }

	// search walks to a body and presses U, and reports the world clock
	// between the two: the walk is not the search's time (the J2b review).
	search := func(i int) float64 {
		t.Helper()

		b := body(i)
		walkTo(t, s, num(b, "x"), num(b, "y"))

		clock := worldMinutes(t, s)
		s.call("strigoi_key", map[string]any{"key": "u"})
		s.call("strigoi_step", map[string]any{"frames": 2})

		return clock
	}

	closeJournal := func() {
		s.call("strigoi_key", map[string]any{"key": "escape"})
		s.call("strigoi_step", map[string]any{"frames": 2})
	}

	opened := func(act, row string) {
		t.Helper()

		ui := uiState(s)
		view := sub(ui, "journal_view")

		if !flag(t, ui, "journal_open") || str(view, "part") != "writings" || str(view, "selected") != row {
			t.Fatalf("%s: the journal opens at %s: open %v, part %q, selected %q",
				act, row, flag(t, ui, "journal_open"), str(view, "part"), str(view, "selected"))
		}
	}

	// --- 1: the amulet ------------------------------------------------------------------------
	before := xp()
	clock := search(0)
	opened("act 1", "w_r09")

	if got := xp() - before; got != 5 {
		t.Fatalf("act 1: the first read of the amulet pays 5; it paid %.0f", got)
	}

	if spent := worldMinutes(t, s) - clock; math.Abs(spent-10) > 1 {
		t.Fatalf("act 1: a search is ten minutes; the clock moved %.1f", spent)
	}

	closeJournal()

	// --- 2: the Sultan's paper ----------------------------------------------------------------
	before = xp()
	search(1)
	opened("act 2", "w_r10")

	if got := xp() - before; got != 10 || journalTask(s, "t_paper") != "open" {
		t.Fatalf("act 2: the paper pays 10 and opens its task: +%.0f, task %q", got, journalTask(s, "t_paper"))
	}

	closeJournal()

	// --- 3: nothing to read --------------------------------------------------------------------
	before = xp()
	reads := len(sub(journalState(s), "reads"))
	clock = search(2)

	if flag(t, uiState(s), "journal_open") || len(sub(journalState(s), "reads")) != reads || xp() != before {
		t.Fatalf("act 3: the third man carries nothing he can read: journal %v, reads %v, +%.0f",
			flag(t, uiState(s), "journal_open"), journalState(s)["reads"], xp()-before)
	}

	if spent := worldMinutes(t, s) - clock; math.Abs(spent-10) > 1 {
		t.Fatalf("act 3: the search still took its ten minutes; the clock moved %.1f", spent)
	}

	// --- 4: the amulet again (the control) -----------------------------------------------------
	before = xp()
	search(0)
	opened("act 4", "w_r09")

	if got := xp() - before; got != 0 || num(sub(journalState(s), "reads"), "R09") != 2 {
		t.Fatalf("act 4: a second read pays nothing and is counted: +%.0f, reads %v", got, journalState(s)["reads"])
	}

	closeJournal()

	// --- 5: no body (the control) --------------------------------------------------------------
	b := body(0)
	walkAwayFrom(t, s, num(b, "x"), num(b, "y"), 6)

	for _, raw := range bodies {
		o := raw.(map[string]any)
		p := s.call("strigoi_get_player", map[string]any{})

		if math.Hypot(num(p, "x")-num(o, "x"), num(p, "y")-num(o, "y")) < 2 {
			t.Fatalf("act 5: walked away from one body and onto another at %v; move the control", o)
		}
	}

	clock = worldMinutes(t, s)
	s.call("strigoi_key", map[string]any{"key": "u"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if spent := worldMinutes(t, s) - clock; spent > 0.5 || flag(t, uiState(s), "journal_open") {
		t.Fatalf("act 5: U with no body at his feet is refused and takes no time: %.1f minutes, journal %v",
			spent, flag(t, uiState(s), "journal_open"))
	}
}
