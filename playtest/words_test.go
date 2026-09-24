//go:build playtest

package playtest

import (
	"sort"
	"testing"
)

// TestStrigoiWords is M5.3's string table (23 Sep 2026): with -strings, every
// label is answered from Strigoi's own words and Diablo II's string tables
// are never read.
//
//  1. control -- as shipped, the game reads Diablo II's string tables from
//     the MPQs (the census's data/local/lng row) and reports no string table;
//  2. with -strings data/strigoi/strings/strings.json: after the menu, a new
//     game, the kit, talents and help panels, the character panel, a talk
//     with the headman and two hours of world time, data/local/lng has NO
//     file from an MPQ, and every key the UI
//     asked for was in the table (strings_missing 0 -- the ones missing are
//     named if not). Screenshots of the menu and the character panel.
func TestStrigoiWords(t *testing.T) {
	const table = "data/strigoi/strings/strings.json"

	census := func(s *session) map[string]any {
		return sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")
	}

	lng := func(a map[string]any) float64 {
		if area, ok := sub(a, "by_area")["data/local/lng"].(map[string]any); ok {
			return num(area, "mpq")
		}

		return 0
	}

	day := func(s *session, name, headmanName string) {
		// Past the trademark screen, to the menu's buttons.
		s.call("strigoi_pause", map[string]any{})
		s.call("strigoi_step", map[string]any{"frames": 30})
		s.call("strigoi_screenshot", map[string]any{"name": name + "-trademark"})
		s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
		s.call("strigoi_step", map[string]any{"frames": 30})
		s.call("strigoi_screenshot", map[string]any{"name": name + "-menu"})

		s.call("strigoi_start_game", map[string]any{
			"hero_name": "Words", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
		})

		for _, k := range []string{"i", "i", "t", "t", "h", "h", "tab", "tab", "c"} {
			s.call("strigoi_key", map[string]any{"key": k})
			s.call("strigoi_step", map[string]any{"frames": 4})
		}

		s.call("strigoi_screenshot", map[string]any{"name": name + "-character"})
		s.call("strigoi_key", map[string]any{"key": "c"})

		// The census day's talk, and a stretch of world time.
		// The headman's name is translated too: Warriv in Diablo II's words.
		headman := villager(t, s, headmanName)
		walkNear(t, s, headman)
		openTalkWith(t, s, headman)
		s.call("strigoi_key", map[string]any{"key": "escape"})
		s.call("strigoi_step", map[string]any{"frames": 4})

		for i := 0; i < 8; i++ {
			s.call("strigoi_step_world", map[string]any{"world_minutes": 15.0})
		}
	}

	shipped := start(t)
	day(shipped, "words-d2", "Warriv")

	if a := census(shipped); lng(a) == 0 || str(a, "string_set") != "" {
		t.Fatalf("control: as shipped the game reads %v Diablo II string tables, string table %q; want some, and none", lng(a), str(a, "string_set"))
	}

	shipped.stop()

	ours := startWith(t, "-classic", "-strings", table)
	day(ours, "words-strigoi", "The headman")

	a := census(ours)

	if str(a, "string_set") != "/"+table {
		t.Fatalf("the string table in use is %q, want /%s", str(a, "string_set"), table)
	}

	if n := lng(a); n != 0 {
		t.Fatalf("with Strigoi's string table, %v of Diablo II's were still read", n)
	}

	var missing []string

	for _, raw := range asList(a["strings"]) {
		k := raw.(map[string]any)
		if k["found"] != true {
			missing = append(missing, str(k, "key"))
		}
	}

	sort.Strings(missing)

	if len(missing) > 0 {
		t.Fatalf("the UI asked for %d key(s) the table lacks: %v", len(missing), missing)
	}

	t.Logf("%.0f keys asked, all answered from %s", num(a, "strings_asked"), table)
}
