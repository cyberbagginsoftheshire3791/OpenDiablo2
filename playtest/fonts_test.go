//go:build playtest

package playtest

import (
	"testing"
)

// TestStrigoiFonts is M5.3's fonts (23 Sep 2026): with -fonts, every word is
// drawn from Strigoi's font set and not one of Diablo II's font files is read.
//
//  1. control -- the game as shipped reads Diablo II's fonts from the MPQs
//     (the census's data/local/font row), and reports no font set;
//  2. with -fonts data/strigoi/fonts/fonts.json: the census reports the set,
//     and after the menus, a new game, the kit, the talents and the help
//     panel -- every font-bearing screen a day opens -- data/local/font has
//     NO file from an MPQ. Screenshots of the menu and the kit as evidence.
func TestStrigoiFonts(t *testing.T) {
	const set = "data/strigoi/fonts/fonts.json"

	fontFiles := func(s *session) (mpq float64, fontSet string) {
		a := sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")
		if area, ok := sub(a, "by_area")["data/local/font"].(map[string]any); ok {
			mpq = num(area, "mpq")
		}

		return mpq, str(a, "font_set")
	}

	day := func(s *session, name string) {
		s.call("strigoi_pause", map[string]any{})
		s.call("strigoi_step", map[string]any{"frames": 30})
		s.call("strigoi_screenshot", map[string]any{"name": name + "-menu"})

		s.call("strigoi_start_game", map[string]any{
			"hero_name": "Letters", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
		})

		for _, k := range []string{"i", "i", "t", "t", "h", "h", "i"} {
			s.call("strigoi_key", map[string]any{"key": k})
			s.call("strigoi_step", map[string]any{"frames": 4})
		}

		s.call("strigoi_screenshot", map[string]any{"name": name + "-kit"})
	}

	shipped := start(t)
	day(shipped, "fonts-d2")

	if mpq, fontSet := fontFiles(shipped); mpq == 0 || fontSet != "" {
		t.Fatalf("control: as shipped the game reads %v Diablo II font files, font set %q; want some, and none", mpq, fontSet)
	}

	shipped.stop()

	ours := startWith(t, "-classic", "-fonts", set)
	day(ours, "fonts-strigoi")

	mpq, fontSet := fontFiles(ours)
	if fontSet != "/"+set {
		t.Fatalf("the font set in use is %q, want /%s", fontSet, set)
	}

	if mpq != 0 {
		t.Fatalf("with the font set, %v font files still came out of an MPQ", mpq)
	}
}
