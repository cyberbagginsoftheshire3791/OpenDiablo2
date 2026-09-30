package d2asset

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// THE BUG-87 REVIEW FIXES (29 Sep 2026): its B1 (BUG-91) and BUG-90.

// b91Bad are times into a frame no play could have: a second before it (the
// review's playtest, which crashed the game through a composite's negative
// frame), far past it (1e17 s and more wrapped a composite's frame negative
// too, and stalled a sheet's first Advance), exactly one frame's length (the
// bound itself: Advance never leaves a whole frame unspent) and the
// non-numbers. frameLength is the art's own.
func b91Bad(frameLength float64) map[string]float64 {
	return map[string]float64{
		"-1.0":                   -1.0,
		"a hair past the floor":  math.Nextafter(ElapsedFloor, -1),
		"1e17":                   1e17,
		"1e18":                   1e18,
		"exactly a frame":        frameLength,
		"a hair past a frame":    math.Nextafter(frameLength, 1),
		"not a number":           math.NaN(),
		"infinitely far past it": math.Inf(1),
		"infinitely before it":   math.Inf(-1),
	}
}

// b91Good are the extremes a running animation can hold: a few ulps below
// zero (Advance subtracting whole frames from a sum of frame times), zero,
// and the last value short of a whole frame.
func b91Good(frameLength float64) []float64 {
	return []float64{-1e-17, ElapsedFloor, 0, frameLength / 2, math.Nextafter(frameLength, 0)}
}

// B1 (BUG-91): A SHEET'S SetProgress REFUSES A TIME NO PLAY COULD HAVE, and a
// refusal changes nothing; the times a play can have are taken.
func TestASheetRefusesATimeNoPlayCouldHave(t *testing.T) {
	a := b87Layer(t, 12, 1.0/12, 56) // a creature's death sheet: 12 frames in 1.0 s
	length := a.FrameLength()
	require.InDelta(t, 1.0/12, length, 1e-15)

	require.NoError(t, a.SetProgress(3, 0.01))

	for name, e := range b91Bad(length) {
		require.Error(t, a.SetProgress(5, e), "%s (%v s into a frame of %v s)", name, e, length)

		f, el := a.Progress()
		require.Equal(t, [2]interface{}{3, 0.01}, [2]interface{}{f, el}, "%s: a refusal changes nothing", name)
	}

	for _, e := range b91Good(length) {
		require.NoError(t, a.SetProgress(5, e), "%v s into a frame of %v s", e, length)

		f, el := a.Progress()
		require.Equal(t, [2]interface{}{5, e}, [2]interface{}{f, el})
		require.NoError(t, a.Advance(b87Tick), "and it plays on")
	}
}

// B1 (BUG-91): A COMPOSITE'S SetProgress REFUSES THEM TOO -- the review's
// crash: -1.0 s made the first Advance take eight frames off the mode's
// frame, and Render index Priority[dir][-8]. After every refusal the mode
// and its layers are where they were, and every time taken plays on with the
// mode's frame inside the mode.
func TestACompositeRefusesATimeNoPlayCouldHave(t *testing.T) {
	c := b87Composite(t, 16, b87Speed, 56, 16, 8)
	length := c.FrameLength()
	require.Equal(t, b87Speed, length, "the mode's frame length is its speed")

	require.NoError(t, c.SetProgress(3, 0.01))
	before := b87Seen(c)

	for name, e := range b91Bad(length) {
		require.Error(t, c.SetProgress(5, e), "%s (%v s into a frame of %v s)", name, e, length)
		require.Equal(t, before, b87Seen(c), "%s: a refusal changes nothing", name)
	}

	for _, e := range b91Good(length) {
		require.NoError(t, c.SetProgress(15, e), "%v s into a frame of %v s", e, length)

		for i := 0; i < 5; i++ {
			require.NoError(t, c.Advance(b87Tick))

			f, _, _ := c.Progress()
			require.True(t, f >= 0 && f < 16, "%v s: the mode's frame %d is one Render can draw", e, f)
		}
	}

	require.Zero(t, (&Composite{}).FrameLength(), "no mode, no frame length")
}

// THE LAYERS ARE SET UNCHECKED, because the mode's check is the one that
// matters and a layer's frame length -- the mode's speed times its frames,
// over its frames -- can be an ulp shorter than the mode's: a time the mode
// takes must not be refused by a layer. Found on a speed where the two
// differ, the last time short of the mode's frame is taken, and each layer
// holds it.
func TestACompositesLayersTakeEveryTimeItsModeTakes(t *testing.T) {
	speed, n := 0.0, 0

	for s := 1; s <= 255 && speed == 0; s++ {
		sp := 1.0 / (float64(s) * speedUnit)

		for k := 1; k <= 40; k++ {
			if (sp*float64(k))/float64(k) < sp {
				speed, n = sp, k

				break
			}
		}
	}

	require.NotZero(t, speed, "some D2 speed and frame count round a layer's frame an ulp short")

	c := b87Composite(t, n, speed, 56, n)
	layerLength := c.mode.layers[0].FrameLength()
	require.Less(t, layerLength, speed)

	e := math.Nextafter(speed, 0)
	require.GreaterOrEqual(t, e, layerLength, "a time the mode takes and the layer's own check would refuse")

	require.NoError(t, c.SetProgress(0, e))

	_, le := c.mode.layers[0].Progress()
	require.Equal(t, e, le, "the layer holds the mode's time")
	t.Logf("speed %v, %d frames: a layer's frame is %v s, an ulp short; %v s taken", speed, n, layerLength, e)
}

// BUG-90: A TURN KEEPS EVERY LAYER'S FRAME. A composite mid-swing is turned:
// each layer is at the frame and the time into it that it held before the
// turn -- a DCC or PNG layer's own SetDirection put it back to 0 -- so the
// layers stay in step with the mode's frame, which is what a load sets them
// to (SetProgress: the frame modulo each layer's own count). So the turned
// composite and one rebuilt from its progress, as a load rebuilds an NPC,
// draw the same on every tick after. (Before the fix, the turned one drew its
// layers from frame 0 while its mode ran on.)
func TestATurnKeepsTheLayersFrames(t *testing.T) {
	a := b87Composite(t, 16, b87Speed, 56, 16, 16, 8)

	for i := 0; i < 20; i++ {
		require.NoError(t, a.Advance(b87Tick))
	}

	layers := func(c *Composite) [][2]interface{} {
		var out [][2]interface{}

		for _, l := range c.mode.layers {
			if l != nil {
				f, e := l.Progress()
				out = append(out, [2]interface{}{f, e})
			}
		}

		return out
	}

	before := layers(a)
	require.Positive(t, before[0][0], "the layers are a frame in")

	a.SetDirection(24)
	require.Equal(t, 24, a.GetDirection())
	require.Equal(t, before, layers(a), "the turn kept every layer's frame and time")

	frame, elapsed, _ := a.Progress()

	b := b87Composite(t, 16, b87Speed, 0, 16, 16, 8)
	b.SetDirection(24)
	require.NoError(t, b.SetProgress(frame, elapsed))
	require.Equal(t, b87Seen(a), b87Seen(b), "the turned composite is the one a load rebuilds")

	for i := 0; i < 60; i++ {
		require.NoError(t, a.Advance(b87Tick))
		require.NoError(t, b.Advance(b87Tick))
		require.Equal(t, b87Seen(a), b87Seen(b), "tick %d after", i)
	}
}
