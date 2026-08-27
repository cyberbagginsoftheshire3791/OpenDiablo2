//go:build playtest

package playtest

import (
	"image"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

// TestPlacedLightLightsWhereItStands closes M4.1's reopening: the harness can
// now put a light somewhere the player is not standing, and it stays there.
//
// The gap it exists to catch: Light.Add has always taken a position, but
// until place_source the only non-test caller was carried_source, which
// hardcodes the player's — so every light the running game could make was on
// the player. Both night scripts lit a carried torch, so they agreed with
// each other and with the model while all of them missed it. This script
// fails on that build, because a player-centred light cannot brighten ground
// nine tiles away while leaving the player's own at the night floor.
//
// Nine tiles, not three, and the geometry is forced rather than chosen. The
// hearth's radius is 8, so anything closer engulfs the player and a placed
// source becomes indistinguishable from a carried one — the very confusion
// under test. Nine is the first whole tile past the radius, and it is also
// past what the camera can hold: one tile is (80, 40) screen pixels, so the
// hearth's own tile sits off the edge and only the near side of its lit
// region is on screen. So the model half asserts what the camera cannot see —
// that the hearth's own tile is lit — and the pixel half asserts the thing
// the camera is good for: that the light on screen is centred away from the
// player.
func TestPlacedLightLightsWhereItStands(t *testing.T) {
	const (
		dawnMinute  = 165.0  // 02:45, the epoch
		nightMinute = 1275.0 // 21:15, true dark

		hearthWest = 9.0 // tiles along -x, "west" of the player

		playerBand = 1.0  // tiles from the player: the ground he stands on
		hearthBand = 6.0  // tiles from the hearth: its lit region, the part on screen
		beyondBand = 11.0 // tiles from the hearth: the far side of the player
		noBand     = 99.0 // a band nothing falls in

		tilePixelX = 80.0
		tilePixelY = 40.0

		floorRadius  = 1.5 // dials: what the player sees at the night floor
		hearthRadius = 8.0

		minPixels = 500  // below this the sampling geometry has moved
		quiet     = 1.25 // a band this close to the unlit night is untouched
	)

	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Ember", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	player := s.call("strigoi_get_player", map[string]any{})

	px, py := pair(player, "screen")
	if px == 0 && py == 0 {
		t.Fatalf("no player screen position to measure light around: %v", player)
	}

	tx, ty := pair(player, "tile")
	hearthX, hearthY := tx-hearthWest, ty

	// The hearth's screen point, by the engine's own isometric step: one tile
	// in x is (+80, +40) screen pixels, one tile in y is (-80, +40). It lands
	// off screen, which is fine — measure only needs a centre to sort by.
	hx := px + tilePixelX*((hearthX-tx)-(hearthY-ty))
	hy := py + tilePixelY*((hearthX-tx)+(hearthY-ty))

	t.Logf("player: tile (%.0f, %.0f) at screen (%.0f, %.0f) · hearth: tile (%.0f, %.0f) at screen (%.0f, %.0f)",
		tx, ty, px, py, hearthX, hearthY, hx, hy)

	// --- the daylight control (the black-floor instrument, P3 §5.3) ------
	s.call("strigoi_step_world", map[string]any{"world_minutes": 12*60 - dawnMinute})

	day := measure("day/player", s.frame(t, "placed-day-noon"), px, py, playerBand, noBand)
	t.Logf("day:    %s", day)

	if day.play < 10 {
		s.stop()
		t.Skipf("the daylight frame is black (play mean %.1f/255) — this is a black-floor launch "+
			"(P3 §5.3), not a light failure; nothing can be measured against it", day.play)
	}

	// --- the deep night, nothing lit -------------------------------------
	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "moon", "value": 0})

	clock := sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	s.call("strigoi_step_world", map[string]any{
		"world_minutes": (24*60 - num(clock, "minute_of_day")) + nightMinute + 30,
	})

	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	if str(clock, "stage") != "night" {
		t.Fatalf("wanted the deep night, got stage %s at %s", str(clock, "stage"), str(clock, "time_of_day"))
	}

	light := sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state")
	if n := num(light, "lit_sources"); n != 0 {
		t.Fatalf("the night should open unlit, but %.0f source(s) burn: %v", n, light)
	}

	floor := num(light, "player_level")

	darkImg := s.frame(t, "placed-night-unlit")
	darkP := measure("unlit/player", darkImg, px, py, playerBand, noBand)
	darkH := measure("unlit/hearth", darkImg, hx, hy, hearthBand, beyondBand)

	t.Logf("unlit:  %s", darkP)
	t.Logf("unlit:  %s", darkH)

	if darkP.nearPix < minPixels || darkH.nearPix < minPixels || darkH.farPix < minPixels {
		t.Fatalf("too few pixels to measure (player %d, hearth %d, beyond %d): the sampling geometry "+
			"has moved and these ratios would mean nothing", darkP.nearPix, darkH.nearPix, darkH.farPix)
	}

	if darkP.near < 0.5 || darkH.near < 0.5 || darkH.far < 0.5 {
		t.Fatalf("the unlit night is entirely black (player %.2f, hearth %.2f, beyond %.2f of 255): "+
			"the floor should be dim, not gone", darkP.near, darkH.near, darkH.far)
	}

	// --- put a fire nine tiles west --------------------------------------
	after := sub(s.call("strigoi_set_system_field", map[string]any{
		"system": "light", "field": "place_source",
		"value": map[string]any{"kind": "hearth", "x": hearthX, "y": hearthY},
	}), "state")

	// 1. THE MODEL: placed, not carried; lighting its own ground and leaving
	//    the player's at the floor.
	if str(after, "carried_source") != "" {
		t.Fatalf("the player must be carrying nothing for this to prove anything: %v", after)
	}

	list, _ := after["source_list"].([]any)
	if len(list) != 1 {
		t.Fatalf("source_list should hold the one hearth: %v", after["source_list"])
	}

	src, _ := list[0].(map[string]any)

	if src["carried"] != false {
		t.Fatalf("place_source made a carried light: %v", src)
	}

	if num(src, "x") != hearthX || num(src, "y") != hearthY {
		t.Fatalf("the hearth reports itself at (%v, %v); it was placed at (%.0f, %.0f)",
			src["x"], src["y"], hearthX, hearthY)
	}

	if lvl := num(src, "level_here"); lvl < 0.9 {
		t.Fatalf("the hearth's own tile is at level %.3f — a placed light must light where it stands", lvl)
	}

	if got := num(after, "player_level"); math.Abs(got-floor) > 1e-9 {
		t.Fatalf("the player's tile went from %.3f to %.3f when a fire was lit nine tiles away — "+
			"the light is following the player", floor, got)
	}

	if got := num(after, "radius"); math.Abs(got-floorRadius) > 1e-9 {
		t.Fatalf("visible radius %.2f, want the night floor %.2f: a hearth the player stands "+
			"outside of must not help him see", got, floorRadius)
	}

	t.Logf("the model: hearth (%.0f, %.0f) at level %.3f · the player's tile at %.3f · radius %.2f",
		hearthX, hearthY, num(src, "level_here"), num(after, "player_level"), num(after, "radius"))

	// 2. THE PIXELS: the same claim, measured on screen in ratios against
	//    the unlit night, so nothing depends on a monitor or a palette.
	litImg := s.frame(t, "placed-night-hearth")
	litP := measure("hearth/player", litImg, px, py, playerBand, noBand)
	litH := measure("hearth/hearth", litImg, hx, hy, hearthBand, beyondBand)

	t.Logf("lit:    %s", litP)
	t.Logf("lit:    %s", litH)

	hearthGain := litH.near / darkH.near
	playerGain := litP.near / darkP.near
	beyondGain := litH.far / darkH.far

	if hearthGain < 1.5 {
		t.Fatalf("the hearth brightened its own ground by only x%.2f (%.1f -> %.1f) — the renderer "+
			"is not reading the placed source", hearthGain, darkH.near, litH.near)
	}

	// THIS is the assertion M4.1 shipped without. A carried torch takes the
	// player's own ground to x8 or more; a fire nine tiles away must not
	// touch it at all.
	if playerGain > quiet {
		t.Fatalf("the player's own ground brightened by x%.2f (%.1f -> %.1f) when a fire was lit "+
			"nine tiles away — the light is coming from the player, not the hearth",
			playerGain, darkP.near, litP.near)
	}

	if hearthGain < 1.5*playerGain {
		t.Fatalf("the fire lit the frame rather than a place: hearth x%.2f, player x%.2f — "+
			"a placed light must be brighter around its own ground than around the player's",
			hearthGain, playerGain)
	}

	// 3. and it stops where the dials say it stops.
	if beyondGain > quiet {
		t.Fatalf("light reached %.0f tiles from an %.0f-tile hearth (x%.2f) — the falloff is leaking",
			beyondBand, hearthRadius, beyondGain)
	}

	t.Logf("the pixels: the hearth's ground x%.2f, the player's x%.2f, beyond the radius x%.2f",
		hearthGain, playerGain, beyondGain)

	// 4. no unexpected error lines (the known one is allowlisted)
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

// frame takes a screenshot and decodes it. shot measures a frame from one
// centre; a placed light has to be measured from two — the player's ground
// and the fire's — so the decode lives here and both callers share it.
func (s *session) frame(t *testing.T, name string) image.Image {
	t.Helper()

	out := s.call("strigoi_screenshot", map[string]any{"name": name})

	path := str(out, "path")

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("screenshot %s missing on disk: %v", name, err)
	}

	img, err := png.Decode(f)
	_ = f.Close()

	if err != nil {
		t.Fatalf("screenshot %s decode: %v", name, err)
	}

	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Fatalf("screenshot %s: want 800x600, got %dx%d", name, b.Dx(), b.Dy())
	}

	return img
}
