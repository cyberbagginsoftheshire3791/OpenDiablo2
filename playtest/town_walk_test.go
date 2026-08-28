//go:build playtest

package playtest

import (
	"image/png"
	"math"
	"os"
	"strings"
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
	t.Logf("spawned at %.2f, %.2f (seed %d)", spawnX, spawnY, int64(num(game, "seed")))

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

	// 4. walk and confirm the player moved. The map is unseeded until M3.3,
	//    so no fixed vector is guaranteed clear: try the four cardinal
	//    directions until one moves the player. Since M4.3a a blocked
	//    direction is no longer a reason to stop -- the A* routes around
	//    obstacles -- so the loop now only guards against a direction whose
	//    goal is genuinely unreachable. The M3.3 version pins the seed and
	//    one vector; TestPathfinding (M4.3a) is where routing is asserted.
	moved := 0.0

	for _, d := range [][2]float64{{6, 0}, {-6, 0}, {0, 6}, {0, -6}} {
		p = s.call("strigoi_get_player", map[string]any{})
		fromX, fromY := num(p, "x"), num(p, "y")

		s.call("strigoi_move_player_to", map[string]any{"x": fromX + d[0], "y": fromY + d[1]})
		time.Sleep(2500 * time.Millisecond)

		p = s.call("strigoi_get_player", map[string]any{})
		dist := math.Hypot(num(p, "x")-fromX, num(p, "y")-fromY)
		t.Logf("direction %+v: moved %.2f tiles to %.2f, %.2f", d, dist, num(p, "x"), num(p, "y"))

		if dist >= 2 {
			moved = dist
			break
		}
	}

	if moved < 2 {
		walk := s.call("strigoi_dump_map", map[string]any{
			"x": int(spawnX) - 6, "y": int(spawnY) - 6, "w": 13, "h": 13, "layer": "walk",
		})
		t.Fatalf("player could not move >= 2 tiles in any cardinal direction; walkability around spawn:\n%s", str(walk, "text"))
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

	// 5b. floor observation — recorded, never asserted (P3 spec §5.3). The
	//     black town floor of 22 Aug did NOT reproduce on 24 Aug (frame and
	//     cache both healthy), so the bug is intermittent; this record is the
	//     instrument that catches it in the act.
	floorLit, floorTotal := 0, 0

	for y := 150; y < 450; y += 3 {
		for x := 100; x < 700; x += 3 {
			r, g, b, _ := img.At(x, y).RGBA()
			floorTotal++

			if r>>8+g>>8+b>>8 > 30 {
				floorLit++
			}
		}
	}

	t.Logf("floor observation: %d/%d sampled play-area pixels lit (%.0f%%) — a black-floor run would be ~0%%",
		floorLit, floorTotal, 100*float64(floorLit)/float64(floorTotal))

	// 6. no ERROR/FATAL log lines, except the known pre-existing ones:
	//    - "[UI Manager][ERROR] Error while setting frame (N): invalid frame
	//      index" fires during normal play (HUD frame setting; predates the
	//      harness — recorded in docs/harness.md findings, not chased here).
	logs := s.call("strigoi_read_log", map[string]any{"pattern": `\[(ERROR|FATAL)\]`, "limit": 50})

	if lines, ok := logs["lines"].([]any); ok {
		var unexpected []any

		for _, l := range lines {
			m, _ := l.(map[string]any)
			if strings.Contains(str(m, "text"), "invalid frame index") {
				continue // known, pre-existing
			}

			unexpected = append(unexpected, l)
		}

		if len(unexpected) > 0 {
			t.Fatalf("unexpected error log lines: %v", unexpected)
		}
	}

	// 7. the black-floor diagnostic (§5.3): record, do not assert
	dump := s.call("strigoi_dump_surface", map[string]any{"kind": "floor_tile", "max": 4})
	t.Logf("floor-tile cache dump (the black-floor experiment): %v", dump["items"])

	// 8. session teardown happens in cleanup (strigoi_quit)
}
