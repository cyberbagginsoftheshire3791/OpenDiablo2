package d2world

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ------------------------------------------------------------ the clock --

func TestClockSnapshotFieldsClassified(t *testing.T) {
	c := NewClock(DefaultClockDials())
	t.Cleanup(c.Close)
	c.Advance(10)

	for _, cl := range b2aClockClasses() {
		b2aClassified(t, cl.kind, c.Snapshot(), cl.fields)
	}
}

// b2aClockClasses is every field of the clock, labelled.
func b2aClockClasses() []b2aClass {
	return []b2aClass{{Clock{}, map[string]string{
		"dials":        "W: the construction dials; the game builds the clock with the defaults",
		"elapsed":      "S:elapsed",
		"frozen":       "D: the harness's hold (SetFrozen has no caller in the game); a resumed game is not held",
		"moonOverride": "D: the harness's SetMoon (no caller in the game); the sky is read from the day table",
	}}}
}

// b2aClockSteps advances a clock across dusk into the night, reading the
// provider after each step: the rate changes at the stage edge, so an
// elapsed that is off by any amount shows in where the clock lands.
func b2aClockSteps(t *testing.T, c *Clock) string {
	t.Helper()

	var trace strings.Builder

	// Read before the first step too: what a load shows on its first frame
	// is the save's to get right, whatever running on would overwrite.
	trace.WriteString(b2aJSON(t, c.HarnessState()))

	for i := 0; i < 12; i++ {
		c.Advance(20)
		trace.WriteString(b2aJSON(t, c.HarnessState()))
	}

	return trace.String()
}

func TestClockSnapshotRoundTrip(t *testing.T) {
	orig := NewClock(DefaultClockDials())
	t.Cleanup(orig.Close)

	// Mid-afternoon of day 1: close enough to dusk that the steps cross it.
	orig.Advance(3900)

	snap := b2aThroughJSON(t, orig.Snapshot())
	require.Equal(t, orig.Snapshot(), snap, "the snapshot survives JSON exactly")

	restored := NewClock(DefaultClockDials())
	t.Cleanup(restored.Close)
	require.NoError(t, restored.Restore(snap))

	require.Equal(t, b2aJSON(t, orig.HarnessState()), b2aJSON(t, restored.HarnessState()), "restored is the same clock")
	require.Equal(t, b2aClockSteps(t, orig), b2aClockSteps(t, restored), "and stays the same clock when both run on")

	// Refused: a time that is not a time, by Validate and by Restore alike.
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		require.Error(t, NewClock(DefaultClockDials()).Validate(ClockSnapshot{Elapsed: bad}), "elapsed %v", bad)
		require.Error(t, NewClock(DefaultClockDials()).Restore(ClockSnapshot{Elapsed: bad}), "elapsed %v", bad)
	}
}

// A new game saved at once: the empty case every system must also carry.
func TestClockSnapshotOfANewGame(t *testing.T) {
	orig := NewClock(DefaultClockDials())
	t.Cleanup(orig.Close)

	cp := NewClock(DefaultClockDials())
	t.Cleanup(cp.Close)
	cp.Advance(100)

	require.NoError(t, cp.Restore(b2aThroughJSON(t, orig.Snapshot())))
	require.Equal(t, b2aJSON(t, orig.HarnessState()), b2aJSON(t, cp.HarnessState()))
}

func TestClockSnapshotEveryFieldIsLoadBearing(t *testing.T) {
	orig := NewClock(DefaultClockDials())
	t.Cleanup(orig.Close)
	orig.Advance(3900)

	snap := orig.Snapshot()
	ref := b2aClockSteps(t, orig)

	outcomes := b2aSweep(t, snap, ref, nil, func(raw []byte) (string, error) {
		var s ClockSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		c := NewClock(DefaultClockDials())
		defer c.Close()

		if err := c.Restore(s); err != nil {
			return "", err
		}

		return b2aClockSteps(t, c), nil
	})

	b2aExercised(t, outcomes, b2aClockClasses()...)
}
