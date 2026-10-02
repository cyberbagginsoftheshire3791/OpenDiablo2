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

	// FOG OFF, THE SHIPPED CAMERA (F5, 2 Oct 2026). This script's subject is
	// light: with fog on, ground he has not explored is black and remembered
	// ground greyed, so its "far" ratios would measure fog, not the night
	// (fog's own night is TestFogAtNight). The zoom is the shipped 0.5, and
	// the tile buckets are measured at the view's scale.
	s := startGame(t, "-fog=false")

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Dark", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	// M4.7 step 3: the risen would stand in the frame and break its evenness;
	// this script measures the unlit night, so the dead stay down.
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
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
	//
	// NOTHING ALIVE IN THE FRAME, AND THIS IS A MEASUREMENT (19 Sep 2026,
	// BUG-11). Reaching the deep night costs a full day of world time and the
	// spawn tables run the whole way. Until the group cap was fixed they stalled
	// after a couple of packs, so the night frame happened to contain almost
	// nothing; with the cap recycling the slots of packs the player has beaten,
	// a monster can be standing inside the two-tile "near" bucket when the
	// shutter opens. Measured: `near` rose from x0.136 to x0.153 of its daylight
	// value while `far` did not move at all, and the uniformity check failed at
	// 32% against its 25% tolerance -- a sprite, not a vignette.
	//
	// The claim this script makes is about PER-TILE GROUND LIGHT. A lit sprite in
	// one bucket and not the other is noise in exactly the term the check
	// divides by, so the tap goes off and the map is cleared first. Widening the
	// tolerance instead would have hidden the one thing the check exists to
	// catch.
	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "chance", "value": 0,
	})

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		if row, ok := raw.(map[string]any); ok {
			s.call("strigoi_set_system_field", map[string]any{
				"system": "spawns", "field": "despawn", "value": str(row, "group"),
			})
		}
	}

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

	// The daylight regions are the denominator of every check below. If they
	// are too dark to divide by, this run cannot measure the thing the script
	// exists to measure — and an unmeasurable run is not a passing one.
	if !usable(day.near, day.far) {
		t.Fatalf("cannot measure the light: the daylight control is too dark in the very regions the "+
			"three checks below divide by — %s. This is NOT the parked black floor (P3 §5.3): a "+
			"black-floor launch is caught above by the play-area control, which passed. An empty "+
			"bucket also reads 0.0, and the far bucket is only a few hundred pixels wide even when "+
			"healthy, so suspect the sampling geometry first — but whatever it is, uniformity, the "+
			"torch gradient and the burn-out ARE the milestone's claim, and a run that cannot "+
			"measure them has not proved it", day)
	}

	// 2. an unlit night dims uniformly — the sky is the only source, so near
	//    and far fall by the same factor. This is what separates real
	//    per-tile light from a vignette painted over the frame.
	nearFall := night.near / day.near
	farFall := night.far / day.far

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

// TestNightIsVisiblyDarkThroughFog is the night's pixels with fog ON, the
// shipped game (the F5 review's B1): TestNightIsVisiblyDark measures light
// with fog off, so nothing measured the night as it is drawn through fog --
// a fog that drew every visible tile at full brightness at night (the
// review's M7) passed the whole suite. The near band (2 tiles) is what he
// sees by the dark radius and his torch: unlit, it falls to about x0.14 of
// noon as it does with fog off; his torch lifts it several times over. The
// far band at night is remembered or unexplored ground, which fog draws no
// brighter than memory_level -- so it is bounded, not compared.
//
// Negative control (2 Oct 2026): the review's M7 (under fog a visible tile
// drawn at brightness 1) and this fails (wt-fog5\nc-m7-visible-fullbright.txt).
func TestNightIsVisiblyDarkThroughFog(t *testing.T) {
	const (
		dawnMinute  = 165.0  // 02:45, the epoch
		nightMinute = 1275.0 // 21:15, true dark

		nearTiles = 2.0
		farTiles  = 5.5

		unlitMax  = 0.25 // the unlit near band against noon: ~0.14 measured, fog off and on
		torchMin  = 3.0  // his torch against the unlit night, near: x7.0 fog off
		memoryMax = 0.45 // fog's memory_level: remembered ground is never brighter
	)

	s := start(t) // the shipped game: fog on, 0.5

	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Fogged", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)

	if f := fogState(s); !flag(t, f, "enabled") {
		if str(f, "off_reason") == "classic" {
			s.stop()
			t.Skip("-classic has no fog; TestNightIsVisiblyDark measures its night")
		}

		t.Fatalf("the shipped game has fog: %v", f)
	}

	px, py := pair(s.call("strigoi_get_player", map[string]any{}), "screen")
	if px == 0 && py == 0 {
		t.Fatal("no player screen position to measure light around")
	}

	s.call("strigoi_step_world", map[string]any{"world_minutes": 12*60 - dawnMinute})

	day := s.shot(t, "fog-render-day-noon", px, py, nearTiles, farTiles)
	t.Logf("day:   %s", day)

	if day.play < 10 {
		s.stop()
		t.Skipf("the daylight frame is black (play mean %.1f/255): a black-floor launch (P3 §5.3)", day.play)
	}

	if !usable(day.near) {
		t.Fatalf("the daylight near band is too dark to divide by: %s", day)
	}

	setField(s, "spawns", "chance", 0)

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		if row, ok := raw.(map[string]any); ok {
			setField(s, "spawns", "despawn", str(row, "group"))
		}
	}

	setField(s, "clock", "moon", 0)

	clock := clockState(s)
	s.call("strigoi_step_world", map[string]any{
		"world_minutes": (24*60 - num(clock, "minute_of_day")) + nightMinute + 30,
	})

	if c := clockState(s); str(c, "stage") != "night" {
		t.Fatalf("wanted the deep night, got stage %s at %s", str(c, "stage"), str(c, "time_of_day"))
	}

	night := s.shot(t, "fog-render-night-deep", px, py, nearTiles, farTiles)
	t.Logf("night: %s", night)

	if night.near < 1 {
		t.Fatalf("the unlit night's near band is black (%.2f): he sees his dark radius, dim, not gone", night.near)
	}

	if fall := night.near / day.near; fall > unlitMax {
		t.Fatalf("through fog the unlit night's near band fell only to x%.3f of noon (%.1f -> %.1f), over x%.2f -- "+
			"the ground he sees in the dark is drawn as if lit", fall, day.near, night.near, unlitMax)
	}

	if day.far >= 10 && night.far > memoryMax*day.far {
		t.Fatalf("through fog the night's far band (remembered or unexplored ground) reads %.1f, over memory_level %.2f "+
			"of noon's %.1f", night.far, memoryMax, day.far)
	}

	setField(s, "light", "carried_source", "torch")

	torch := s.shot(t, "fog-render-night-torch", px, py, nearTiles, farTiles)
	t.Logf("torch: %s", torch)

	if gain := torch.near / night.near; gain < torchMin {
		t.Fatalf("through fog his torch brightened the near band by only x%.2f (%.1f -> %.1f), under x%.1f",
			gain, night.near, torch.near, torchMin)
	}

	t.Logf("through fog: the unlit near band x%.3f of noon, the torch x%.2f, the far band %.1f (noon %.1f)",
		night.near/day.near, torch.near/night.near, night.far, day.far)
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

	return measure(name, s.frame(t, name), px, py, nearTiles, farTiles, viewScale(t, s))
}

// measure walks the play area and sorts each sampled pixel by how far its
// tile is from the player, using the engine's own screen-to-world step: one
// tile in x is (+80, +40) screen pixels, one tile in y is (-80, +40), each
// times the view's scale (F5: 0.5 in the shipped game).
func measure(name string, img image.Image, px, py, nearTiles, farTiles, scale float64) frameLight {
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
			dx := (float64(x) - px) / (tilePixelX * scale)
			dy := (float64(y) - py) / (tilePixelY * scale)
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
