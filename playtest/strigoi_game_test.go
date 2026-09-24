//go:build playtest

package playtest

import (
	"strings"
	"testing"
)

// TestStrigoiIsTheGame (23 Sep 2026: "Our version can officially be Strigoi"):
// the game launched with NO switches is Strigoi's own -- the authored
// village, the Janissary's sheets, Strigoi's fonts and words -- and a first
// hour of play reads none of Diablo II's tiles, class art, fonts or string
// tables. (Straight into a game: the character-select screen, which still
// shows Diablo II's classes, is not visited.) Control first: -classic, as
// every other script launches, reads Diablo II's tiles and fonts.
func TestStrigoiIsTheGame(t *testing.T) {
	mpqIn := func(a map[string]any, area string) float64 {
		m, ok := sub(a, "by_area")[area].(map[string]any)
		if !ok {
			return 0
		}

		return num(m, "mpq")
	}

	classic := startWith(t, "-classic") // explicitly: STRIGOI_PLAYTEST_GAME=default changes start
	classic.call("strigoi_pause", map[string]any{})
	classic.call("strigoi_start_game", map[string]any{
		"hero_name": "Classic", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	c := sub(classic.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")
	if mpqIn(c, "data/global/tiles") == 0 || mpqIn(c, "data/local/font") == 0 || str(c, "font_set") != "" {
		t.Fatalf("control: -classic read %.0f Diablo II tile and %.0f font files, font set %q; want some, some, none",
			mpqIn(c, "data/global/tiles"), mpqIn(c, "data/local/font"), str(c, "font_set"))
	}

	classic.stop()

	s := startWith(t) // no switches
	s.call("strigoi_pause", map[string]any{})

	g := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Janissary", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	if !strings.HasSuffix(str(g, "map_built"), "village.tmj") || str(g, "map_error") != "" {
		t.Fatalf("the world is %q (error %q), want the village", str(g, "map_built"), str(g, "map_error"))
	}

	if !strings.Contains(str(g, "hero_used"), "data/strigoi/hero/") || str(g, "hero_error") != "" {
		t.Fatalf("the hero is drawn from %q (error %q), want Strigoi's sheets", str(g, "hero_used"), str(g, "hero_error"))
	}

	if body := str(sub(s.call("strigoi_get_player", map[string]any{}), "state"), "body"); body != "png" {
		t.Fatalf("the player is drawn by %q, want png", body)
	}

	for _, k := range []string{"i", "i", "t", "t", "h", "h"} {
		s.call("strigoi_key", map[string]any{"key": k})
		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	// The HUD's button menu opens Strigoi's panels, as the keys do: its
	// inventory button is the kit and its skill button the talents. Until
	// 23 Sep 2026 they opened Diablo II's grid (with OpenDiablo2's test items
	// in it) and Diablo II's skill tree.
	miniButton := func(name string) {
		t.Helper()

		b := sub(sub(uiState(s), "mini_panel_buttons"), name)
		if len(b) == 0 {
			t.Fatalf("no mini-panel button %q: %v", name, uiState(s)["mini_panel_buttons"])
		}

		if !flag(t, b, "visible") {
			t.Fatalf("the mini-panel's %q button is not drawn: %v", name, b)
		}

		s.call("strigoi_click", map[string]any{
			"x": int(mustNum(t, b, "x") + mustNum(t, b, "w")/2), "y": int(mustNum(t, b, "y") + mustNum(t, b, "h")/2),
			"button": "left",
		})
		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	for _, c := range []struct{ button, strigoi, diablo, key string }{
		{"inventory", "kit_open", "inventory_open", "i"},
		{"skills", "talent_open", "skilltree_open", "t"},
	} {
		if !flag(t, uiState(s), "mini_panel_open") {
			miniButton("open_close")
		}

		if !flag(t, uiState(s), "mini_panel_open") {
			t.Fatalf("the HUD's menu button did not open the mini-panel: %v", uiState(s)["mini_panel_buttons"])
		}

		miniButton(c.button)

		ui := uiState(s)
		if !flag(t, ui, c.strigoi) || flag(t, ui, c.diablo) {
			t.Fatalf("the mini-panel's %s button opened %s=%v, Diablo II's %s=%v; want Strigoi's",
				c.button, c.strigoi, ui[c.strigoi], c.diablo, ui[c.diablo])
		}

		s.call("strigoi_screenshot", map[string]any{"name": "mini-panel-" + c.button})
		s.call("strigoi_key", map[string]any{"key": c.key}) // closed as the key opens it
		s.call("strigoi_step", map[string]any{"frames": 4})

		if flag(t, uiState(s), c.strigoi) {
			t.Fatalf("the %s key did not close what the %s button opened", c.key, c.button)
		}
	}

	for i := 0; i < 4; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 15.0})
	}

	s.call("strigoi_screenshot", map[string]any{"name": "strigoi"})

	a := sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")

	if str(a, "font_set") != "/data/strigoi/fonts/fonts.json" || str(a, "string_set") != "/data/strigoi/strings/strings.json" {
		t.Fatalf("font set %q, string table %q: want Strigoi's", str(a, "font_set"), str(a, "string_set"))
	}

	// Diablo II's tables: 14 of its 83 at most (history items 111, 113, 116)
	// -- the generated world's four load only when one is built, the
	// missiles and cast overlays only when a skill is cast.
	if n := mpqIn(a, "data/global/excel"); n == 0 || n > 14 {
		t.Errorf("data/global/excel: %.0f table(s) from Diablo II's MPQs, want 1..14", n)
	}

	// The census's area names are what this reads: an area it still reads
	// from the MPQs (the UI) must show, or the zeros below prove nothing.
	if mpqIn(a, "data/global/ui") == 0 {
		t.Fatalf("the census shows no Diablo II UI file; its areas are not what this script reads: %v", sub(a, "by_area"))
	}

	// data/local is Diablo II's language file (data/local/use), unread when
	// the words and fonts are Strigoi's.
	for _, area := range []string{"data/global/tiles", "data/global/chars", "data/local/font", "data/local/lng", "data/local"} {
		if n := mpqIn(a, area); n != 0 {
			t.Errorf("%s: %.0f file(s) from Diablo II's MPQs, want none", area, n)
		}
	}

	// Every word the game asked for is in Strigoi's table: a missing key is
	// drawn as the key itself ("panelcmini" on the mini-panel's close tooltip
	// was the first found this way, 23 Sep 2026).
	if mustNum(t, a, "strings_missing") != 0 {
		var missing []string

		for _, raw := range asList(a["strings"]) {
			if k, ok := raw.(map[string]any); !ok {
				t.Fatalf("a string census row is not an object: %v", raw)
			} else if k["found"] == false {
				missing = append(missing, str(k, "key"))
			}
		}

		t.Errorf("%.0f key(s) the game asked for are not in Strigoi's string table: %v", mustNum(t, a, "strings_missing"), missing)
	}

	// A right-click still casts Diablo II's right skill (history item 116),
	// and the missile and overlay tables load with the first cast, not before.
	cast := []string{"/data/global/excel/missiles.txt", "/data/global/excel/overlay.txt"}

	for _, table := range cast {
		if censusHas(a, table) {
			t.Errorf("%s was read before anything was cast", table)
		}
	}

	sx, sy := pair(s.call("strigoi_get_player", map[string]any{}), "screen")
	if sx == 0 && sy == 0 {
		t.Fatal("strigoi_get_player reported no screen position; the right-click would miss him")
	}

	s.call("strigoi_click", map[string]any{"x": int(sx) + 60, "y": int(sy), "button": "right"})
	s.call("strigoi_step", map[string]any{"frames": 30})

	after := sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")
	for _, table := range cast {
		if !censusHas(after, table) {
			t.Errorf("a right-click beside him (screen %.0f,%.0f) did not read %s", sx+60, sy, table)
		}
	}

	// J1 (24 Sep 2026): Q opens his journal now, not Diablo II's quest log --
	// so the quest log's art is never read, before Q or after it.
	const questArt = "/data/global/ui/menu/a1q1.dc6"

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 4})

	if ui := uiState(s); !flag(t, ui, "journal_open") || flag(t, ui, "quest_log_open") {
		t.Fatalf("Q: journal_open %v, quest_log_open %v; want the journal and not the quest log",
			flag(t, ui, "journal_open"), flag(t, ui, "quest_log_open"))
	}

	if censusHas(sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state"), questArt) {
		t.Errorf("Q read %s: Diablo II's quest log was opened behind the journal", questArt)
	}

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 4})

	if flag(t, uiState(s), "journal_open") {
		t.Fatal("Q did not close the journal")
	}

	// Diablo II's character panel loads its art when first opened (history
	// item 114): none of it was read above, it still opens and closes on its
	// key, and opening it does read its art -- so the absence above was a
	// measurement.
	for _, c := range []struct{ key, open, art string }{
		{"c", "hero_stats_open", "/data/global/ui/panel/invchar6.dc6"},
	} {
		if censusHas(a, c.art) {
			t.Errorf("%s was read before its panel (%s) was opened", c.art, c.open)
		}

		s.call("strigoi_key", map[string]any{"key": c.key})
		s.call("strigoi_step", map[string]any{"frames": 4})

		if !flag(t, uiState(s), c.open) {
			t.Fatalf("%s did not open its panel (%s)", c.key, c.open)
		}

		if !censusHas(sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state"), c.art) {
			t.Errorf("opening %s did not read %s; the census cannot see the panel's art", c.open, c.art)
		}

		s.call("strigoi_key", map[string]any{"key": c.key})
		s.call("strigoi_step", map[string]any{"frames": 4})

		if flag(t, uiState(s), c.open) {
			t.Fatalf("%s did not close its panel (%s)", c.key, c.open)
		}
	}

	t.Logf("Strigoi, as launched: %.0f MPQ files, %.0f of ours, %.0f string keys asked (%.0f missing)",
		num(a, "mpq_files"), num(a, "native_files"), num(a, "strings_asked"), num(a, "strings_missing"))
}

// censusHas reports whether the assets census lists a file (paths compared
// case-blind and slash-normalised, as the loader records them).
func censusHas(assets map[string]any, path string) bool {
	want := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))

	for _, raw := range asList(assets["files"]) {
		if f, ok := raw.(map[string]any); ok && strings.ToLower(strings.ReplaceAll(str(f, "path"), `\`, "/")) == want {
			return true
		}
	}

	return false
}
