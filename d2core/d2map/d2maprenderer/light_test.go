package d2maprenderer

import (
	"math"
	"testing"
)

// fakeSampler answers what it was told to and remembers what it was asked.
type fakeSampler struct {
	level float64
	asked [][2]int
}

func (f *fakeSampler) Level(tileX, tileY int) float64 {
	f.asked = append(f.asked, [2]int{tileX, tileY})

	return f.level
}

// TestNoSamplerIsDaylight pins the promise this change was allowed to make:
// with no light model set, every tile draws at exactly 1 — the one value at
// which the surface's brightness guard skips the colour matrix altogether.
// Anything else and M4.1 would have changed how the game looks by day, and
// every screenshot taken before tonight would stop being comparable.
func TestNoSamplerIsDaylight(t *testing.T) {
	mr := &MapRenderer{}

	for _, tile := range [][2]int{{0, 0}, {5, 9}, {-3, 400}} {
		if got := mr.tileLight(tile[0], tile[1]); got != fullyLit {
			t.Fatalf("tile %v with no sampler: got %v, want %v", tile, got, fullyLit)
		}
	}
}

// TestTheSamplerIsAskedAboutTheTileBeingDrawn catches the coordinate slip
// that would light the wrong tile — swapped x and y, or an off-by-one from
// the viewport translation — which on screen looks like light that trails
// the player instead of following him.
func TestTheSamplerIsAskedAboutTheTileBeingDrawn(t *testing.T) {
	sampler := &fakeSampler{level: 0.5}
	mr := &MapRenderer{}
	mr.SetLightSampler(sampler)

	if got := mr.tileLight(12, 7); got != 0.5 {
		t.Fatalf("tileLight: got %v, want the sampler's 0.5", got)
	}

	if len(sampler.asked) != 1 || sampler.asked[0] != [2]int{12, 7} {
		t.Fatalf("sampler was asked %v, want exactly one question about {12 7}", sampler.asked)
	}
}

// TestNonsenseFromTheSamplerCannotPoisonTheFrame: a NaN reaches ebiten's
// colour matrix intact and takes the whole draw with it, so an out-of-range
// or undefined level is clamped here rather than trusted. NaN dims nothing,
// on the principle that a broken light model should leave the world visible.
func TestNonsenseFromTheSamplerCannotPoisonTheFrame(t *testing.T) {
	cases := []struct {
		name  string
		level float64
		want  float64
	}{
		{"a normal level passes through", 0.125, 0.125},
		{"pitch dark is allowed", 0, 0},
		{"below zero clamps to pitch", -1, 0},
		{"above daylight clamps to daylight", 2, fullyLit},
		{"negative infinity clamps to pitch", math.Inf(-1), 0},
		{"positive infinity clamps to daylight", math.Inf(1), fullyLit},
		{"NaN dims nothing", math.NaN(), fullyLit},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mr := &MapRenderer{}
			mr.SetLightSampler(&fakeSampler{level: c.level})

			if got := mr.tileLight(0, 0); got != c.want {
				t.Fatalf("level %v: got %v, want %v", c.level, got, c.want)
			}
		})
	}
}

// TestClearingTheSamplerRestoresDaylight — the escape hatch has to work, or
// there is no way back to the pre-light renderer for a diagnostic.
func TestClearingTheSamplerRestoresDaylight(t *testing.T) {
	mr := &MapRenderer{}
	mr.SetLightSampler(&fakeSampler{level: 0})

	if got := mr.tileLight(0, 0); got != 0 {
		t.Fatalf("with a pitch-dark sampler: got %v, want 0", got)
	}

	mr.SetLightSampler(nil)

	if got := mr.tileLight(0, 0); got != fullyLit {
		t.Fatalf("after clearing the sampler: got %v, want %v", got, fullyLit)
	}
}
