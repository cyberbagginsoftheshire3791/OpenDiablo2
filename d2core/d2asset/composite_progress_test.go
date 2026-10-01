package d2asset

import (
	"fmt"
	"image"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// M4.6 BUG-87: A HELD ACTION IS SAVED AT ITS FRAME. An NPC's held action ends
// when its composite's mode has played through once (NPC.Advance), and the
// world save carries the mode's frame and the time already spent on it
// (Composite.Progress) and puts them back on a rebuilt NPC's composite
// (Composite.SetProgress). A real composite needs a COF and animdata from the
// MPQs, so these tests build the mode by hand -- a mode's frame count and
// speed, and PNG layers cut from an in-memory sheet -- which is everything
// Advance reads. The playtests carry the real composites.

// b87Tick is the harness's frame, 1/60 s.
const b87Tick = 1.0 / 60

// b87Speed is a D2 animation speed's frame length (256 -> 1/25 s), the one
// measured on fallen1's and zombie1's A1 and GH.
const b87Speed = 1.0 / (256 * speedUnit)

// b87Layer is a PNG layer of eight facings and n frames, at the mode's speed,
// played forward and facing as the composite does (createMode).
func b87Layer(t *testing.T, n int, speed float64, facing int) d2interface.Animation {
	t.Helper()

	a, err := pngAnimationFromRGBA(image.NewRGBA(image.Rect(0, 0, n*2, 8*2)),
		PNGSheet{Directions: 8, FramesPerDirection: n}, d2enum.DrawEffectNone)
	require.NoError(t, err)

	a.SetPlaySpeed(speed)
	a.PlayForward()
	require.NoError(t, a.SetDirection(facing))

	return a
}

// b87Composite is a composite in a fresh mode of frames frames at speed,
// facing facing, with one layer per entry of layers (each that many frames).
func b87Composite(t *testing.T, frames int, speed float64, facing int, layers ...int) *Composite {
	t.Helper()

	mode := &compositeMode{
		animationMode:  d2enum.MonsterAnimationModeAttack1,
		weaponClass:    "HTH",
		layers:         make([]d2interface.Animation, d2enum.CompositeTypeMax),
		frameCount:     frames,
		animationSpeed: speed,
	}

	for i, n := range layers {
		mode.layers[i] = b87Layer(t, n, speed, facing)
	}

	return &Composite{mode: mode, direction: facing}
}

// b87Seen is everything that decides or draws the composite's next frame:
// the mode's frame, time and play count, and every layer's frame and time.
func b87Seen(c *Composite) string {
	f, e, p := c.Progress()
	out := fmt.Sprintf("mode %d %v %d", f, e, p)

	for i, l := range c.mode.layers {
		if l != nil {
			lf, le := l.Progress()
			out += fmt.Sprintf(" | layer %d %d %v", i, lf, le)
		}
	}

	return out
}

// A COMPOSITE PUT AT A SAVED PROGRESS RUNS ON AS THE SAVED ONE (BUG-87): a
// mode of 16 frames (zombie1's A1) and of 19 at another speed (its DT), each
// with layers of the mode's frame count and one of fewer, saved on its first
// tick, a tick in, a frame in, half-way and on the last tick before it plays
// through; a fresh composite set to that progress is the saved one on every
// tick after -- the mode's frame, the time into it, the play count reaching 1
// on the same tick (the tick an NPC's held action ends), and every layer.
func TestACompositeResumesAtItsProgress(t *testing.T) {
	for _, m := range []struct {
		name   string
		frames int
		speed  float64
	}{
		{"a swing of 16 frames", 16, b87Speed},
		{"a death of 19 frames", 19, 1.0 / (200 * speedUnit)},
	} {
		// Ticks until the mode has played through once.
		probe := b87Composite(t, m.frames, m.speed, 56, m.frames)
		ticks := 0

		for ; ticks < 1000 && probe.GetPlayedCount() == 0; ticks++ {
			require.NoError(t, probe.Advance(b87Tick))
		}

		for _, in := range []int{0, 1, 3, ticks / 2, ticks - 1} {
			a := b87Composite(t, m.frames, m.speed, 56, m.frames, m.frames, m.frames/2)

			for i := 0; i < in; i++ {
				require.NoError(t, a.Advance(b87Tick))
			}

			frame, elapsed, played := a.Progress()
			require.Zero(t, played, "%s, %d ticks in: still on its first play", m.name, in)

			// The load: a composite built fresh -- its mode at frame 0, its
			// layers faced -- then turned as saved, then set to the progress.
			b := b87Composite(t, m.frames, m.speed, 0, m.frames, m.frames, m.frames/2)
			b.SetDirection(56)
			require.NoError(t, b.SetProgress(frame, elapsed))
			require.Equal(t, b87Seen(a), b87Seen(b), "%s, %d ticks in: set equal", m.name, in)

			for i := 0; i < ticks+30; i++ {
				require.NoError(t, a.Advance(b87Tick))
				require.NoError(t, b.Advance(b87Tick))
				require.Equal(t, b87Seen(a), b87Seen(b), "%s, %d ticks in, tick %d after", m.name, in, i)
			}

			require.Positive(t, a.GetPlayedCount(), "%s: the run covered its end", m.name)
		}

		t.Logf("%s: plays through in %d ticks at 1/60 s; resumed at its first, second, fourth, middle and last tick, each the saved one to the end", m.name, ticks)
	}
}

// WITHOUT THE PROGRESS -- the frame put back to the first, or the time into
// it lost -- the resumed mode plays through on another tick: what the world
// file's action_at is for (BUG-87).
func TestACompositeWithoutItsProgressEndsOnAnotherTick(t *testing.T) {
	endsAt := func(c *Composite) int {
		for i := 0; i < 1000; i++ {
			require.NoError(t, c.Advance(b87Tick))

			if c.GetPlayedCount() > 0 {
				return i
			}
		}

		return -1
	}

	a := b87Composite(t, 16, b87Speed, 56, 16)
	for i := 0; i < 20; i++ {
		require.NoError(t, a.Advance(b87Tick))
	}

	frame, elapsed, _ := a.Progress()
	require.Positive(t, frame)
	require.Positive(t, elapsed)

	whole := b87Composite(t, 16, b87Speed, 56, 16)
	require.NoError(t, whole.SetProgress(frame, elapsed))

	first := b87Composite(t, 16, b87Speed, 56, 16)
	require.NoError(t, first.SetProgress(0, 0))

	noTime := b87Composite(t, 16, b87Speed, 56, 16)
	require.NoError(t, noTime.SetProgress(frame, 0))

	want := endsAt(a)
	require.Equal(t, want, endsAt(whole), "the whole progress: the same tick")
	require.NotEqual(t, want, endsAt(first), "the frame put back to the first: another tick")
	require.NotEqual(t, want, endsAt(noTime), "the time into the frame lost: another tick")
}

// SetProgress puts the mode on its FIRST play: a held action always is, and
// SetMode short-circuits on the mode a composite is already in, so a restore
// onto such a composite cannot trust its count. It refuses a frame the mode
// does not have, and a composite with no mode.
func TestACompositesProgressIsSetOnItsFirstPlay(t *testing.T) {
	c := b87Composite(t, 8, b87Speed, 56, 8)

	for c.GetPlayedCount() < 2 {
		require.NoError(t, c.Advance(b87Tick))
	}

	require.NoError(t, c.SetProgress(3, 0.01))

	frame, elapsed, played := c.Progress()
	require.Equal(t, 3, frame)
	require.Equal(t, 0.01, elapsed)
	require.Zero(t, played)

	for _, bad := range []int{-1, 8, 99} {
		require.Error(t, c.SetProgress(bad, 0), "frame %d of 8", bad)
	}

	f, e, p := c.Progress()
	require.Equal(t, [3]interface{}{3, 0.01, 0}, [3]interface{}{f, e, p}, "a refusal changes nothing")

	none := &Composite{}
	require.Error(t, none.SetProgress(0, 0), "no mode")

	f, e, p = none.Progress()
	require.Equal(t, [3]interface{}{0, 0.0, 0}, [3]interface{}{f, e, p})
}

// An animation's progress is its frame and the time into it, and SetProgress
// refuses a frame the facing does not have.
func TestAnAnimationsProgressRoundTrips(t *testing.T) {
	a := b87Layer(t, 10, 0.1, 56)

	for i := 0; i < 23; i++ {
		require.NoError(t, a.Advance(b87Tick))
	}

	frame, elapsed := a.Progress()
	require.Equal(t, a.GetCurrentFrame(), frame)

	b := b87Layer(t, 10, 0.1, 56)
	require.NoError(t, b.SetProgress(frame, elapsed))

	for i := 0; i < 80; i++ {
		f1, e1 := a.Progress()
		f2, e2 := b.Progress()
		require.Equal(t, [2]interface{}{f1, e1}, [2]interface{}{f2, e2}, "tick %d", i)
		require.Equal(t, a.GetPlayedCount(), b.GetPlayedCount(), "tick %d", i)

		require.NoError(t, a.Advance(b87Tick))
		require.NoError(t, b.Advance(b87Tick))
	}

	require.Error(t, b.SetProgress(10, 0))
	require.Error(t, b.SetProgress(-1, 0))
}
