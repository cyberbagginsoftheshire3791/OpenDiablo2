//go:build playtest

package playtest

import (
	"image/png"
	"math"
	"os"
	"testing"
	"time"
)

// TestTownWalk is the first playtest script (P3 spec §5.1, M3.2 version —
// live time, non-deterministic movement allowed). The M3.3 version replaces
// the wall-clock waits with strigoi_step and runs the scenario twice with one
// seed, comparing state digests.
//
// The floor assertion is deliberately absent: the town floor renders black
// (the known pre-existing compositing bug). The script asserts on the HUD
// region instead and records the floor-tile cache dump as evidence (§5.3).
func TestTownWalk(t *testing.T) {
	s := start(t)

	// 1. liveness
	ping := s.call("strigoi_ping", map[string]any{})
	if str(ping, "commit") == "" {
		t.Fatal("ping: empty commit")
	}

	// 2. into the world with a fresh hero
	game := s.call("strigoi_start_game", map[string]any{
		"hero_name":    "Harness",
		"hero_class":   "amazon",
		"wait_seconds": 90,
	})

	player := str(game, "player")
	if player != "p:1" {
		t.Fatalf("start_game: want player p:1, got %q", player)
	}

	spawnX, spawnY := pair(game, "spawn_tile")
	t.Logf("spawned at %.2f, %.2f (seed %v)", spawnX, spawnY, game["seed"])

	// 3. the fresh player is sane
	p := s.call("strigoi_get_player", map[string]any{})
	state := sub(p, "state")

	if got := num(state, "stamina"); got <= 0 {
		t.Fatalf("stamina: want > 0, got %v", got)
	}

	if got := num(state, "act"); got != 1 {
		t.Fatalf("act: want 1, got %v", got)
	}

	if got := num(state, "gold"); got < 0 {
		t.Fatalf("gold: want >= 0, got %v", got)
	}

	// 4. walk east and confirm the player moved (live time: wall-clock wait)
	s.call("strigoi_move_player_to", map[string]any{"x": spawnX + 6, "y": spawnY})
	time.Sleep(3 * time.Second)

	p = s.call("strigoi_get_player", map[string]any{})
	movedX, movedY := num(p, "x"), num(p, "y")
	dist := math.Hypot(movedX-spawnX, movedY-spawnY)
	t.Logf("after move: %.2f, %.2f (moved %.2f tiles)", movedX, movedY, dist)

	if dist < 2 {
		t.Fatalf("player barely moved: %.2f tiles (raycast pathing may be blocked; adjust the vector)", dist)
	}

	// 5. screenshot: the HUD region must not be black (the floor may be — known bug)
	shot := s.call("strigoi_screenshot", map[string]any{"name": "town"})
	shotPath := str(shot, "path")

	f, err := os.Open(shotPath)
	if err != nil {
		t.Fatalf("screenshot missing on disk: %v", err)
	}

	img, err := png.Decode(f)
	_ = f.Close()

	if err != nil {
		t.Fatalf("screenshot decode: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 800 || bounds.Dy() != 600 {
		t.Fatalf("screenshot: want 800x600, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	hudLit := false

	for y := bounds.Max.Y - 60; y < bounds.Max.Y && !hudLit; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8+g>>8+b>>8 > 30 {
				hudLit = true
				break
			}
		}
	}

	if !hudLit {
		t.Fatal("the HUD region (bottom 60 rows) is black — the HUD should render even while the floor bug stands")
	}

	// 6. no ERROR/FATAL log lines (warnings, e.g. 'Unknown tile ID', are allowed)
	logs := s.call("strigoi_read_log", map[string]any{"pattern": `\[(ERROR|FATAL)\]`, "limit": 50})

	if lines, ok := logs["lines"].([]any); ok && len(lines) > 0 {
		t.Fatalf("unexpected error log lines: %v", lines)
	}

	// 7. the black-floor diagnostic (§5.3): record, do not assert
	dump := s.call("strigoi_dump_surface", map[string]any{"kind": "floor_tile", "max": 4})
	t.Logf("floor-tile cache dump (the black-floor experiment): %v", dump["items"])

	// 8. session teardown happens in cleanup (strigoi_quit)
}
