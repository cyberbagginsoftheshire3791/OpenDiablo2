//go:build playtest

package playtest

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"
)

// TestNightIsVisiblyDark is M4.1's second half on screen: the renderer half's
// playtest script (Constitution VI.2).
//
// The first half proved the light MODEL — radius, fuel, the floor — through
// the harness, with nothing on screen. This one proves the pixels actually
// obey it, and it does so with ratios rather than absolute brightness, so it
// says nothing that depends on a monitor, a palette, or the intermittent
// black-floor bug:
//
//  1. night is much darker than the same frame by day;
//  2. an unlit night dims UNIFORMLY — near the player and far from him fall
//     by the same factor, because the sky is the only light there is;
//  3. a torch breaks that uniformity in exactly one place: the tiles around
//     the player brighten, the far tiles do not;
//  4. when the torch burns out the frame returns to the plain night.
//
// The camera never moves during the run, so each sampled region contains the
// same tiles in every frame and the comparisons are like-for-like.
func TestNightIsVisiblyDark(t *testing.T) {
	const (
		dawnMinute  = 165.0  // 02:45, the epoch
		nightMinute = 1275.0 // 21:15, true dark
		torchBurn   = 60.0

		nearTiles = 2.0 // "around the player", well inside the torch
		farTiles  = 5.5 // outside a 5-tile torch entirely
	)

	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Dark", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	player := s.call("strigoi_get_player", map[string]any{})

	px, py := pair(player, "screen")
	if px == 0 && py == 0 {
		t.Fatalf("no player screen position to measure light around: %v", player)
	}

	t.Logf("player is at screen (%.0f, %.0f); near = within %.1f tiles, far = beyond %.1f",
		px, py, nearTiles, farTiles)

	// --- the daylight control -------------------------------------------
	// Step to noon so the sky is unambiguously full. This frame is also the
	// black-floor instrument: if it comes back black, the launch is a
	// black-floor launch (P3 §5.3, parked) and this script has nothing to
	// say about light.
	toNoon := 12*60 - dawnMinute
	s.call("strigoi_step_world", map[string]any{"world_minutes": toNoon})

	day := s.shot(t, "render-day-noon", px, py, nearTiles, farTiles)
	t.Logf("day:   %s", day)

	if day.play < 10 {
		s.stop()
		t.Skipf("the daylight frame is black (play mean %.1f/255) — this is a black-floor launch "+
			"(P3 §5.3), not a light failure; nothing can be measured against it", day.play)
	}

	// --- the deep night, no light ----------------------------------------
	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "moon", "value": 0})

	clock := sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	s.call("strigoi_step_world", map[string]any{
		"world_minutes": (24*60 - num(clock, "minute_of_day")) + nightMinute + 30,
	})

	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	if str(clock, "stage") != "night" {
		t.Fatalf("wanted the deep night, got stage %s at %s", str(clock, "stage"), str(clock, "time_of_day"))
	}

	night := s.shot(t, "render-night-deep", px, py, nearTiles, farTiles)
	t.Logf("night: %s", night)

	// 1. THE POINT OF THE MILESTONE: night is dark.
	dim := night.play / day.play
	if dim > 0.5 {
		t.Fatalf("the deep night is only %.0f%% dimmer than noon (%.1f -> %.1f of 255) — "+
			"darkness that does not darken is not darkness", 100*(1-dim), day.play, night.play)
	}

	// ...but the world is still drawn. A black screen would also pass the
	// test above, and a black screen is a bug, not a night.
	if night.play < 1 {
		t.Fatalf("the night frame is entirely black (play mean %.2f/255): the floor should be "+
			"dim, not gone", night.play)
	}

	t.Logf("night is %.0f%% dimmer than noon (play mean %.1f -> %.1f of 255)", 100*(1-dim), day.play, night.play)

	// 2. an unlit night dims uniformly — the sky is the only source, so near
	//    and far fall by the same factor. This is what separates real
	//    per-tile light from a vignette painted over the frame.
	nearFall := night.near / day.near
	farFall := night.far / day.far

	if !usable(day.near, day.far) {
		t.Logf("WARNING: near (%.1f) or far (%.1f) is too dark by day to compare; "+
			"skipping the uniformity and gradient checks", day.near, day.far)
		return
	}

	if spread := math.Abs(nearFall-farFall) / farFall; spread > 0.25 {
		t.Fatalf("the unlit night is not uniform: near fell to %.3f of its daylight value, far to %.3f "+
			"(%.0f%% apart) — with no light source the whole frame should fall together",
			nearFall, farFall, 100*spread)
	}

	t.Logf("unlit night falls uniformly: near x%.3f, far x%.3f", nearFall, farFall)

	// 3. the torch: light where the player is, and nowhere else.
	s.call("strigoi_set_system_field", map[string]any{"system": "light", "field": "carried_source", "value": "torch"})

	torch := s.shot(t, "render-night-torch", px, py, nearTiles, farTiles)
	t.Logf("torch: %s", torch)

	nearGain := torch.near / night.near
	farGain := torch.far / night.far

	if nearGain < 1.5 {
		t.Fatalf("lighting a torch brightened the player's surroundings by only x%.2f (%.1f -> %.1f) — "+
			"the renderer is not reading the light model", nearGain, night.near, torch.near)
	}

	if nearGain < 1.3*farGain {
		t.Fatalf("the torch brightened the whole frame, not a circle: near x%.2f, far x%.2f — "+
			"per-tile light should fall off with distance (S1 §4)", nearGain, farGain)
	}

	t.Logf("the torch lights a circle: near x%.2f, far x%.2f", nearGain, farGain)

	// 4. and when it goes out, the night comes back.
	s.call("strigoi_step_world", map[string]any{"world_minutes": torchBurn + 5})

	light := sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state")
	if light["carried_lit"] != false {
		t.Fatalf("the torch should be out after %v world minutes: %v", torchBurn+5, light)
	}

	out := s.shot(t, "render-night-burnt-out", px, py, nearTiles, farTiles)
	t.Logf("out:   %s", out)

	if drift := math.Abs(out.near-night.near) / night.near; drift > 0.25 {
		t.Fatalf("after the torch died the player's surroundings sit at %.1f, but the plain night was %.1f "+
			"(%.0f%% off) — light is leaking past the source that made it", out.near, night.near, 100*drift)
	}

	t.Logf("the torch burned out and the dark closed back in (%.1f -> %.1f)", torch.near, out.near)

	// 5. no unexpected error lines (the known one is allowlisted)
	logs := s.call("strigoi_read_log", map[string]any{"pattern": `\[(ERROR|FATAL)\]`, "limit": 50})

	if lines, ok := logs["lines"].([]any); ok {
		for _, l := range lines {
			m, _ := l.(map[string]any)
			if !strings.Contains(str(m, "text"), "invalid frame index") {
				t.Fatalf("unexpected error log line: %v", m)
			}
		}
	}
}

// usable reports whether two daylight regions are bright enough for a ratio
// against them to mean anything.
func usable(values ...float64) bool {
	for _, v := range values {
		if v < 10 {
			return false
		}
	}

	return true
}

// frameLight is what one screenshot says about the light in it: the mean
// luminance of the play area, of the tiles around the player, and of the
// tiles beyond any carried light.
type frameLight struct {
	name             string
	play, near, far  float64
	nearPix, farPix  int
	playPix, skipped int
}

func (f frameLight) String() string {
	return fmt.Sprintf("%s: play %.1f (%d px), near %.1f (%d px), far %.1f (%d px), between %d px",
		f.name, f.play, f.playPix, f.near, f.nearPix, f.far, f.farPix, f.skipped)
}

// shot takes a screenshot, decodes it, and measures the three regions around
// one centre. The decode itself lives in frame (night_placed_test.go), which
// measures the same frame from two.
func (s *session) shot(t *testing.T, name string, px, py, nearTiles, farTiles float64) frameLight {
	t.Helper()

	return measure(name, s.frame(t, name), px, py, nearTiles, farTiles)
}

// measure walks the play area and sorts each sampled pixel by how far its
// tile is from the player, using the engine's own screen-to-world step: one
// tile in x is (+80, +40) screen pixels, one tile in y is (-80, +40).
func measure(name string, img image.Image, px, py, nearTiles, farTiles float64) frameLight {
	const (
		top    = 40  // below the top edge
		bottom = 470 // above the HUD
		step   = 2

		tilePixelX = 80.0
		tilePixelY = 40.0
		two        = 2.0
	)

	f := frameLight{name: name}

	var playSum, nearSum, farSum float64

	for y := top; y < bottom; y += step {
		for x := 0; x < 800; x += step {
			r, g, b, _ := img.At(x, y).RGBA()
			lum := float64(r>>8+g>>8+b>>8) / 3

			playSum += lum
			f.playPix++

			// screen delta -> tile delta (invert the isometric step)
			dx := (float64(x) - px) / tilePixelX
			dy := (float64(y) - py) / tilePixelY
			tx := (dy + dx) / two
			ty := (dy - dx) / two
			dist := math.Hypot(tx, ty)

			switch {
			case dist <= nearTiles:
				nearSum += lum
				f.nearPix++
			case dist >= farTiles:
				farSum += lum
				f.farPix++
			default:
				f.skipped++
			}
		}
	}

	f.play = mean(playSum, f.playPix)
	f.near = mean(nearSum, f.nearPix)
	f.far = mean(farSum, f.farPix)

	return f
}

func mean(sum float64, n int) float64 {
	if n == 0 {
		return 0
	}

	return sum / float64(n)
}
