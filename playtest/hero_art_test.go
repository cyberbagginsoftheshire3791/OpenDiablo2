//go:build playtest

package playtest

import (
	"strings"
	"testing"
)

// TestHeroArt is M5.3's hero path: the player drawn from Strigoi's own PNG
// sheets (data/strigoi/hero/<name>/hero.json) instead of Diablo II's class
// composite. The shipped hero is a PLACEHOLDER (tools/heroplaceholder); the
// real Janissary drops in by replacing its sheets.
//
//  1. control -- a hero manifest that does not exist is refused and the
//     player is drawn by the composite, as before;
//  2. the placeholder hero: reported used, the player's body is "png", and
//     he walks -- the body draws its WALK sheet while he moves and its IDLE
//     sheet when he arrives (body_sheet: what the mode resolved to, which
//     the composite has no notion of). A screenshot is taken as evidence.
func TestHeroArt(t *testing.T) {
	const (
		hero    = "/data/strigoi/hero/placeholder/hero.json"
		missing = "/data/strigoi/hero/nobody/hero.json"
	)

	s := start(t)

	bad := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Heroic", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
		"hero_art": missing,
	})

	if str(bad, "hero_used") != "" || !strings.Contains(str(bad, "hero_error"), "nobody") {
		t.Fatalf("a missing hero: used %q error %q; want refused by name", str(bad, "hero_used"), str(bad, "hero_error"))
	}

	control := sub(s.call("strigoi_get_player", map[string]any{}), "state")
	if body, sheet := str(control, "body"), str(control, "body_sheet"); body != "composite" || sheet != "" {
		t.Fatalf("control: the refused hero is drawn by %q (sheet %q), want the composite (no sheet)", body, sheet)
	}

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	s.call("strigoi_step", map[string]any{"frames": 30})

	g := s.call("strigoi_start_game", map[string]any{
		"save_path": str(bad, "save_path"), "seed": 1462, "wait_seconds": 90,
		"hero_art": strings.TrimPrefix(hero, "/"),
	})

	if str(g, "hero_used") != hero || str(g, "hero_error") != "" {
		t.Fatalf("the placeholder hero: used %q error %q", str(g, "hero_used"), str(g, "hero_error"))
	}

	p := s.call("strigoi_get_player", map[string]any{})
	if body := str(sub(p, "state"), "body"); body != "png" {
		t.Fatalf("the player is drawn by %q, want png", body)
	}

	// He walks, and the body follows the player's mode. (The run of his
	// frames is the body's; the mode is the player's, reported back by it.)
	x, y := num(p, "x"), num(p, "y")
	s.call("strigoi_move_player_to", map[string]any{"x": x + 3, "y": y})
	s.call("strigoi_step", map[string]any{"frames": 20})

	walking := sub(s.call("strigoi_get_player", map[string]any{}), "state")
	if mode, sheet := str(walking, "animation_mode"), str(walking, "body_sheet"); mode != "TW" || sheet != "walk" {
		t.Fatalf("walking, the player's mode is %q drawn with the %q sheet, want TW with walk", mode, sheet)
	}

	shot := s.call("strigoi_screenshot", map[string]any{"name": "hero-placeholder"})
	t.Logf("the placeholder hero, walking: %s (read back in %.1f ms)", str(shot, "path"), num(shot, "read_ms"))

	// The screenshot reports its own readback time (docs/bugs.md, the TownWalk
	// stall): absent, it would read 0.
	if num(shot, "read_ms") <= 0 {
		t.Fatalf("the screenshot reports no readback time: %v", shot)
	}

	s.call("strigoi_step", map[string]any{"frames": 300})

	arrived := sub(s.call("strigoi_get_player", map[string]any{}), "state")
	if mode, sheet := str(arrived, "animation_mode"), str(arrived, "body_sheet"); mode != "TN" || sheet != "idle" {
		t.Fatalf("arrived, the player's mode is %q drawn with the %q sheet, want TN with idle", mode, sheet)
	}
}
