package d2util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFrameDeltasReproducesLiveArithmetic pins FrameDeltas to the exact
// formula d2app.advance used before the P3 E1 extraction:
//
//	current := d2util.Now()
//	elapsedUnscaled := current - a.lastTime
//	elapsed := elapsedUnscaled * a.timeScale
//	elapsedLastScreenAdvance := (current - a.lastScreenAdvance) * a.timeScale
//
// A change that breaks this test changes live-mode behaviour.
//
// Every case here has both raw deltas at or below the 0.25 s clamp (item 3), so
// the clamp does not alter them -- this test still pins the pure arithmetic. The
// clamp itself is pinned by TestFrameDeltasClampsALongStall below.
func TestFrameDeltasReproducesLiveArithmetic(t *testing.T) {
	cases := []struct {
		name                             string
		now, lastTime, lastScreen, scale float64
	}{
		{"steady 60fps", 100.0166667, 100.0, 100.0, 1.0},
		{"timescale 2x", 200.2, 200.0, 200.1, 2.0},
		{"timescale half", 1000.1, 1000.0, 1000.05, 0.5},
		{"screen advanced separately", 50.3, 50.1, 50.2, 1.0},
		{"zero delta", 7.0, 7.0, 7.0, 1.0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			unscaled, scaled, screen := FrameDeltas(c.now, c.lastTime, c.lastScreen, c.scale)

			wantUnscaled := c.now - c.lastTime
			wantScaled := wantUnscaled * c.scale
			wantScreen := (c.now - c.lastScreen) * c.scale

			assert.Equal(t, wantUnscaled, unscaled)
			assert.Equal(t, wantScaled, scaled)
			assert.Equal(t, wantScreen, screen)
		})
	}
}

// TestFrameDeltasClampsALongStall pins item 3 (12 Sep 2026): a stall longer than
// maxDelta -- a closed lid, a debugger break, a long synchronous load -- is
// applied as a single 0.25 s tick, not as the whole gap. Without the clamp a
// ten-minute sleep runs 2,400 world minutes and about -160 HP of neglect in one
// frame (audit B1); ebiten delivers one Update per frame with no catch-up.
//
// Negative control: remove the clamp in FrameDeltas and every assertion below
// goes red (the raw 600 s delta comes straight through).
func TestFrameDeltasClampsALongStall(t *testing.T) {
	const maxDelta = 0.25

	// Both raw deltas exceed the clamp (now=600, last=0): everything is bounded,
	// and elapsed is exactly maxDelta x timeScale.
	for _, c := range []struct {
		name  string
		scale float64
	}{
		{"ten-minute sleep", 1.0},
		{"stall at a 2x timescale", 2.0},
	} {
		t.Run(c.name, func(t *testing.T) {
			unscaled, scaled, screen := FrameDeltas(600.0, 0.0, 0.0, c.scale)

			assert.Equal(t, maxDelta, unscaled, "the unscaled delta is clamped at maxDelta")
			assert.Equal(t, maxDelta*c.scale, scaled, "elapsed = clamped delta x timeScale")
			assert.Equal(t, maxDelta*c.scale, screen, "the screen delta is clamped too")
		})
	}

	// The screen delta clamps independently: a small time step, but a screen that
	// has not advanced for a long time.
	t.Run("screen clamps on its own", func(t *testing.T) {
		unscaled, scaled, screen := FrameDeltas(100.1, 100.0, 0.0, 1.0)

		assert.InDelta(t, 0.1, unscaled, 1e-9, "a small time delta is NOT clamped")
		assert.InDelta(t, 0.1, scaled, 1e-9)
		assert.Equal(t, maxDelta, screen, "but the long screen delta is clamped")
	})
}
