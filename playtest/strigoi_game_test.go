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

	for i := 0; i < 4; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 15.0})
	}

	s.call("strigoi_screenshot", map[string]any{"name": "strigoi"})

	a := sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")

	if str(a, "font_set") != "/data/strigoi/fonts/fonts.json" || str(a, "string_set") != "/data/strigoi/strings/strings.json" {
		t.Fatalf("font set %q, string table %q: want Strigoi's", str(a, "font_set"), str(a, "string_set"))
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

	t.Logf("Strigoi, as launched: %.0f MPQ files, %.0f of ours, %.0f string keys asked (%.0f missing)",
		num(a, "mpq_files"), num(a, "native_files"), num(a, "strings_asked"), num(a, "strings_missing"))
}
