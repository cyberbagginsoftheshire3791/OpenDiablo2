//go:build playtest

package playtest

import (
	"strings"
	"testing"
)

// TestJournal is J1 (24 Sep 2026): his diary, in place of Diablo II's quest
// log. Acts:
//
//  1. He starts with the start entries and the 17 June page already written,
//     SILENTLY (no notice). Q opens the journal: the world is held and named
//     "journal", step_world refuses it, the page is on top of the days part
//     and counts his Ramazan (the eighteenth). The panel is modal: K does not
//     forage under it. Parts turn with the arrow keys and a click on a tab;
//     a part turned away from is read. The mini-panel's quest button opens
//     it too, and under it the HUD's own buttons are disabled.
//  2. A wary well woman writes her guard, not the trough. Talk writes: meeting the headman writes him; the ditch he asks for is
//     an OPEN task, dug it is DONE, and the water rung writes its line. Q in
//     a talk opens nothing (the talk is modal). The notice says so.
//  3. A watch promised is an open task.
//  4. THE NIGHT. Night 1's dead rise beside him and come: the sampler notes
//     the risen, and before the priest's tale that is "a man who was a man".
//     Q in the fight is refused, and says why. At first light the dead lie
//     down (written), the 18 June page is written -- the nineteenth of
//     Ramazan -- and the watch he never stood is FAILED.
//  5. Saved and read back: everything written stays written, nothing is
//     written twice, and the reload writes silently.
func TestJournal(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Diarist", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	s.call("strigoi_step", map[string]any{"frames": 4})

	// --- 1: the first page, and the panel ---------------------------------------------------
	j := journalState(s)
	if !flag(t, j, "bound") {
		t.Fatalf("act 1: the journal is bound: %v", j)
	}

	for _, id := range []string{"b1_taken", "d_four", "s_peksimet", "v_stand_start", "sky_long_days"} {
		if !journalWrote(j, id) {
			t.Fatalf("act 1: the start entry %s is written: %v", id, j["written"])
		}
	}

	if pages := strings.Join(strs(j["pages"]), ","); pages != "1462-06-17" {
		t.Fatalf("act 1: the first page is the 17th, and only it: %q", pages)
	}

	if n := str(uiState(s), "journal_notice"); n != "" {
		t.Fatalf("act 1: the start entries are written silently; the notice reads %q", n)
	}

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	ui := uiState(s)
	if !flag(t, ui, "journal_open") || str(ui, "world_held_by") != "journal" {
		t.Fatalf("act 1: Q opens the journal and holds the world: open %v, held by %q",
			flag(t, ui, "journal_open"), str(ui, "world_held_by"))
	}

	before := worldMinutes(t, s)
	s.call("strigoi_step", map[string]any{"frames": 60})

	if after := worldMinutes(t, s); after != before {
		t.Fatalf("act 1: the world is held while he reads; the clock ran %.2f -> %.2f", before, after)
	}

	if msg := s.callErr("strigoi_step_world", map[string]any{"world_minutes": 10.0}); !strings.Contains(msg, "WORLD_HELD") || !strings.Contains(msg, `"journal"`) {
		t.Fatalf("act 1: step_world with the journal open: got %q, want WORLD_HELD naming \"journal\"", msg)
	}

	view := sub(uiState(s), "journal_view")
	rows := asList(view["rows"])

	if str(view, "part") != "days" || len(rows) == 0 || str(rows[0].(map[string]any), "id") != "page:1462-06-17" {
		t.Fatalf("act 1: the journal opens on the days, the page on top: %v", view)
	}

	if text := strings.Join(strs(view["text"]), " "); !strings.Contains(text, "The eighteenth day of Ramazan.") || !strings.Contains(text, "Sunset 19:50") {
		t.Fatalf("act 1: the page counts his Ramazan and gives the sky: %q", text)
	}

	// Evidence for a human: the panel as drawn.
	shot := s.call("strigoi_screenshot", map[string]any{"name": "journal-first-page"})
	t.Logf("the journal, first page: %s", str(shot, "path"))

	// Modal: K under the open journal gathers nothing.
	landBefore := mustNum(t, villageState(s), "land_left")
	s.call("strigoi_key", map[string]any{"key": "k"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if left := mustNum(t, villageState(s), "land_left"); left != landBefore || journalEvents(s, "foraged") != 0 {
		t.Fatalf("act 1: K foraged under the open journal: land %.0f -> %.0f", landBefore, left)
	}

	s.call("strigoi_key", map[string]any{"key": "right"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if part := str(sub(uiState(s), "journal_view"), "part"); part != "self" {
		t.Fatalf("act 1: Right turns to the next part: %q", part)
	}

	clickJournalTab(t, s, "tasks")

	s.call("strigoi_key", map[string]any{"key": "escape"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	ui = uiState(s)
	if flag(t, ui, "journal_open") || str(ui, "world_held_by") != "" {
		t.Fatalf("act 1: Escape closes the journal and the world runs: open %v, held by %q",
			flag(t, ui, "journal_open"), str(ui, "world_held_by"))
	}

	// The mini-panel's quest button is Q (questAction). And with the journal
	// up, the HUD's own buttons -- d2ui widgets, which take a click whatever
	// the controls answer -- are disabled (review B1): the mini-panel does
	// not open under it.
	if !flag(t, uiState(s), "mini_panel_open") {
		clickMiniButton(t, s, "open_close")
	}

	clickMiniButton(t, s, "quest")

	if ui := uiState(s); !flag(t, ui, "journal_open") || flag(t, ui, "quest_log_open") {
		t.Fatalf("act 1: the mini-panel's quest button opens the journal: open %v, quest log %v",
			flag(t, ui, "journal_open"), flag(t, ui, "quest_log_open"))
	}

	clickMiniButton(t, s, "open_close")

	if ui := uiState(s); flag(t, ui, "mini_panel_open") || !flag(t, ui, "journal_open") {
		t.Fatalf("act 1: the HUD's menu button worked under the open journal: mini-panel %v, journal %v",
			flag(t, ui, "mini_panel_open"), flag(t, ui, "journal_open"))
	}

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, uiState(s), "mini_panel_open") {
		clickMiniButton(t, s, "open_close")
	}

	unread := sub(journalState(s), "unread")
	if num(unread, "days") != 0 || num(unread, "self") != 0 || num(unread, "village") == 0 {
		t.Fatalf("act 1: the parts he turned from are read and the village is not: %v", unread)
	}

	// --- 2: talk writes -----------------------------------------------------------------------
	// A wary villager tells him nothing, and the journal writes only what she
	// did: below the water rung the well woman guards the rope, and nothing
	// she says at the trough is written (review A1: keyed on the node).
	well := villager(t, s, "Kashya")
	walkNear(t, s, well)
	openTalkWith(t, s, well)

	if node := str(villageState(s), "node"); node != "well_wary" {
		t.Fatalf("act 2: at 10 the well woman is wary: %q", node)
	}

	answer(t, s, 1) // leave

	if j := journalState(s); !journalWrote(j, "v_well_woman") || journalWrote(j, "v_trough") || journalWrote(j, "s_fletching") {
		t.Fatalf("act 2: the wary well woman writes her guard and not the trough: %v", j["written"])
	}

	headman := villager(t, s, "Warriv")
	walkNear(t, s, headman)
	openTalkWith(t, s, headman)

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, uiState(s), "journal_open") {
		t.Fatal("act 2: Q in a talk opened the journal; the talk is modal")
	}

	answer(t, s, 1) // "Only to live through the nights..." -> headman_work

	if got := journalTask(s, "t_ditch"); got != "open" {
		t.Fatalf("act 2: the ditch he is asked to mend is an open task: %q", got)
	}

	answer(t, s, 1) // dig out the ditch

	j = journalState(s)
	if journalTask(s, "t_ditch") != "done" || !journalWrote(j, "v_headman") || !journalWrote(j, "v_stand_water") {
		t.Fatalf("act 2: dug, the ditch is done, the headman met and the water rung written: tasks %v, written %v",
			j["tasks"], j["written"])
	}

	answer(t, s, 1) // leave

	if n := str(uiState(s), "journal_notice"); !strings.Contains(n, "Written in my journal") {
		t.Fatalf("act 2: the notice says what was written: %q", n)
	}

	// --- 3: the watch promised -----------------------------------------------------------------
	openTalkWith(t, s, headman)
	answer(t, s, answerIndex(t, s, "stand the watch"))
	answer(t, s, 1) // promise

	if got := journalTask(s, "t_watch"); got != "open" {
		t.Fatalf("act 3: a promised watch is an open task: %q", got)
	}

	// No post is near enough: the watch cannot be stood wherever the night
	// finds him.
	setField(s, "village", "watch_radius", 0.01)

	// --- 4: the night -----------------------------------------------------------------------------
	health := mustNum(t, metersState(s), "health")
	keepAlive := func() {
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
		setField(s, "meters", "health", health)
	}

	body := asList(corpsesState(s)["bodies"])[0].(map[string]any)
	walkTo(t, s, num(body, "x"), num(body, "y"))

	setField(s, "rising", "p", 1.0)
	setField(s, "combat", "player_action", "hold")
	setField(s, "combat", "round_minutes", 5.0)

	for i := 0; i < 250 && !flag(t, combatState(s), "fighting"); i++ {
		if risenGroups(s) == 0 {
			s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		} else {
			s.call("strigoi_step", map[string]any{"frames": 6})
		}

		keepAlive()
	}

	if !flag(t, combatState(s), "fighting") {
		t.Fatalf("act 4: beside the body when it rose, the dead find him: risen groups %d, %v", risenGroups(s), corpsesState(s))
	}

	s.call("strigoi_step", map[string]any{"frames": 2})

	j = journalState(s)
	if journalEvents(s, "beast:risen") < 1 || journalEvents(s, "risen_seen_untold") < 1 || !journalWrote(j, "d_man_who_was") {
		t.Fatalf("act 4: the risen noted, before the tale, and written: events %v, written %v", j["events"], j["written"])
	}

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if ui := uiState(s); flag(t, ui, "journal_open") || !strings.Contains(str(ui, "journal_notice"), "not in a fight") {
		t.Fatalf("act 4: Q in a fight is refused and says why: open %v, notice %q",
			flag(t, ui, "journal_open"), str(ui, "journal_notice"))
	}

	wasNight := false

	for i := 0; i < 200; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 20.0})
		keepAlive()

		st := str(clockState(s), "stage")
		if st == "night" {
			wasNight = true
		}

		if wasNight && st != "night" {
			break
		}
	}

	s.call("strigoi_step", map[string]any{"frames": 4})

	j = journalState(s)
	if !strings.Contains(strings.Join(strs(j["pages"]), ","), "1462-06-18") {
		t.Fatalf("act 4: first light writes the 18th's page: %v (clock %v)", j["pages"], clockState(s))
	}

	if got := journalTask(s, "t_watch"); got != "failed" {
		t.Fatalf("act 4: a watch promised and never stood is failed at dawn: %q (events %v)", got, j["events"])
	}

	for _, id := range []string{"d_lay_down", "sky_stages"} {
		if !journalWrote(j, id) {
			t.Fatalf("act 4: %s written by the night: %v (events %v)", id, j["written"], j["events"])
		}
	}

	if journalEvents(s, "night_survived") < 1 {
		t.Fatalf("act 4: the night lived through is noted: %v", j["events"])
	}

	// The page itself, as he reads it.
	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	view = sub(uiState(s), "journal_view")
	if str(view, "part") != "days" {
		clickJournalTab(t, s, "days")
		view = sub(uiState(s), "journal_view")
	}

	if text := strings.Join(strs(view["text"]), " "); str(view, "selected") != "page:1462-06-18" || !strings.Contains(text, "The nineteenth day of Ramazan.") {
		t.Fatalf("act 4: the new page is on top and counts on: %q %q", str(view, "selected"), text)
	}

	shot = s.call("strigoi_screenshot", map[string]any{"name": "journal-second-page"})
	t.Logf("the journal, the second page: %s", str(shot, "path"))

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- 5: saved, and read back ----------------------------------------------------------------
	written := strings.Join(strs(journalState(s)["written"]), ",")
	pages := strings.Join(strs(journalState(s)["pages"]), ",")
	tasks := sub(journalState(s), "tasks")

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	s.call("strigoi_start_game", map[string]any{"save_path": str(game, "save_path"), "seed": 1462, "wait_seconds": 90})
	setField(s, "spawns", "chance", 0)
	s.call("strigoi_step", map[string]any{"frames": 4})

	j = journalState(s)
	if got := strings.Join(strs(j["written"]), ","); !containsAll(got, written) {
		t.Fatalf("act 5: everything written stays written:\n was %s\n now %s", written, got)
	}

	if got := strings.Join(strs(j["pages"]), ","); got != pages {
		t.Fatalf("act 5: the pages are as they were (none twice): %q -> %q", pages, got)
	}

	for id, state := range tasks {
		if got := journalTask(s, id); got != state {
			t.Fatalf("act 5: task %s was %v, reads %q", id, state, got)
		}
	}

	if n := str(uiState(s), "journal_notice"); n != "" {
		t.Fatalf("act 5: a reload writes silently; the notice reads %q", n)
	}

	t.Logf("journal: %d written, pages %s, tasks %v", len(strs(j["written"])), pages, tasks)
}

func journalState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "journal"}), "state")
}

func journalWrote(j map[string]any, id string) bool {
	for _, w := range strs(j["written"]) {
		if w == id {
			return true
		}
	}

	return false
}

func journalTask(s *session, id string) string {
	return str(sub(journalState(s), "tasks"), id)
}

func journalEvents(s *session, event string) float64 {
	return num(sub(journalState(s), "events"), event)
}

// clickJournalTab clicks a part's tab where the panel drew it.
func clickJournalTab(t *testing.T, s *session, part string) {
	t.Helper()

	for _, raw := range asList(sub(uiState(s), "journal_view")["tabs"]) {
		tab := raw.(map[string]any)
		if str(tab, "id") != part {
			continue
		}

		s.call("strigoi_click", map[string]any{
			"x": int(num(tab, "x")) + 10, "y": int(num(tab, "y")) + 6, "button": "left",
		})
		s.call("strigoi_step", map[string]any{"frames": 2})

		if got := str(sub(uiState(s), "journal_view"), "part"); got != part {
			t.Fatalf("a click on the %s tab turned to %q", part, got)
		}

		return
	}

	t.Fatalf("no %s tab drawn: %v", part, sub(uiState(s), "journal_view"))
}

// clickMiniButton clicks a HUD mini-panel button where it is drawn.
func clickMiniButton(t *testing.T, s *session, name string) {
	t.Helper()

	b := sub(sub(uiState(s), "mini_panel_buttons"), name)
	if len(b) == 0 {
		t.Fatalf("no mini-panel button %q: %v", name, uiState(s)["mini_panel_buttons"])
	}

	s.call("strigoi_click", map[string]any{
		"x": int(mustNum(t, b, "x") + mustNum(t, b, "w")/2), "y": int(mustNum(t, b, "y") + mustNum(t, b, "h")/2),
		"button": "left",
	})
	s.call("strigoi_step", map[string]any{"frames": 4})
}

func strs(v any) []string {
	var out []string

	for _, x := range asList(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}

	return out
}

func containsAll(have, want string) bool {
	set := map[string]bool{}
	for _, h := range strings.Split(have, ",") {
		set[h] = true
	}

	for _, w := range strings.Split(want, ",") {
		if w != "" && !set[w] {
			return false
		}
	}

	return true
}
