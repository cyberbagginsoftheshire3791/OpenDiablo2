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
func TestFrameDeltasReproducesLiveArithmetic(t *testing.T) {
	cases := []struct {
		name                             string
		now, lastTime, lastScreen, scale float64
	}{
		{"steady 60fps", 100.0166667, 100.0, 100.0, 1.0},
		{"timescale 2x", 200.5, 200.0, 200.25, 2.0},
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
