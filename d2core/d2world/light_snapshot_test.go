package d2world

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// b2aLightWorld is a clock in the deep night and a light model with every kind
// of source the game makes: a carried torch lit and burning, a placed hearth,
// a placed torch burning, a placed torch burnt out, and a removed source (so
// next_id is not simply one past the last).
func b2aLightWorld(t *testing.T) (*Clock, *Light) {
	t.Helper()

	c := NewClock(DefaultClockDials())
	t.Cleanup(c.Close)

	l := NewLight(c, DefaultLightDials())
	t.Cleanup(l.Close)

	advanceToMinuteOfDay(t, c, DefaultClockDials().NightStart+30)

	l.SetPlayer(12.5, 11.5)
	require.NoError(t, l.HarnessSet("carried_source", "torch"))                                                      // 1
	require.NoError(t, l.HarnessSet("place_source", map[string]interface{}{"kind": "hearth", "x": 20.0, "y": 11.0})) // 2
	require.NoError(t, l.HarnessSet("place_source", map[string]interface{}{"kind": "torch", "x": 3.0, "y": 3.0}))    // 3
	require.NoError(t, l.HarnessSet("place_source", map[string]interface{}{"kind": "torch", "x": 16.0, "y": 14.0}))  // 4
	require.NoError(t, l.HarnessSet("place_source", map[string]interface{}{"kind": "torch", "x": 8.0, "y": 6.0}))    // 5
	require.NoError(t, l.HarnessSet("remove_source", 3.0))

	// Torch 5 has burnt out; the carried torch and torch 4 have burnt part way.
	for _, s := range l.sources {
		if s.ID == 5 {
			s.Burn = 7
		}
	}

	l.Advance(c.Advance(8))

	return c, l
}

// b2aLightSteps runs the model on: the player walks from his own torch into the
// hearth's circle and out, the lights burn down, and a new torch is placed (its
// id is the next id). Each step reads the provider, the radius, and the level
// on a grid that covers every source.
func b2aLightSteps(t *testing.T, c *Clock, l *Light) string {
	t.Helper()

	var trace strings.Builder

	observe := func() {
		trace.WriteString(b2aJSON(t, l.HarnessState()))

		for y := 0; y < 18; y += 2 {
			for x := 0; x < 24; x += 2 {
				trace.WriteString(b2aJSON(t, l.Level(x, y)))
			}
		}
	}

	// The first frame after a load, before anything burns.
	l.SetPlayer(12.5, 11.5)
	observe()

	walk := [][2]float64{{12.5, 11.5}, {18.5, 11.5}, {20.5, 11.5}, {16.5, 13.5}, {8.5, 6.5}, {12.5, 11.5}}

	for i, at := range walk {
		l.SetPlayer(at[0], at[1])
		l.Advance(c.Advance(7))

		if i == 3 {
			l.Add(SourceTorch, false, 9, 12)
		}

		observe()
	}

	return trace.String()
}

func TestLightSnapshotFieldsClassified(t *testing.T) {
	_, l := b2aLightWorld(t)
	snap := l.Snapshot()

	b2aClassified(t, Light{}, snap, map[string]string{
		"carriedBurnRate": "D: his talents (T3), set again when the load applies his progress",
		"dials":           "W: construction dials; the game builds the model with the defaults",
		"clock":           "W: the clock it reads; restored on its own",
		"sources":         "S:sources",
		"nextID":          "S:next_id",
		"playerX":         "D: the player's position, set by SetPlayer every frame",
		"playerY":         "D: the player's position, set by SetPlayer every frame",
	})

	b2aClassified(t, Source{}, snap, map[string]string{
		"ID":      "S:sources[0].id",
		"Kind":    "S:sources[0].kind",
		"Radius":  "S:sources[0].radius",
		"Burn":    "S:sources[0].burn",
		"Lit":     "S:sources[0].lit",
		"Carried": "S:sources[0].carried",
		"X":       "S:sources[0].x",
		"Y":       "S:sources[0].y",
	})
}

func TestLightSnapshotRoundTrip(t *testing.T) {
	c, l := b2aLightWorld(t)

	snap := b2aThroughJSON(t, l.Snapshot())
	require.Equal(t, l.Snapshot(), snap, "the snapshot survives JSON exactly")
	require.Len(t, snap.Sources, 4)
	require.Equal(t, 6, snap.NextID)

	c2 := NewClock(DefaultClockDials())
	t.Cleanup(c2.Close)
	require.NoError(t, c2.Restore(c.Snapshot()))

	l2 := NewLight(c2, DefaultLightDials())
	t.Cleanup(l2.Close)

	// A restore replaces whatever was there.
	require.NoError(t, l2.HarnessSet("carried_source", "hearth"))
	require.NoError(t, l2.Restore(snap))
	l2.SetPlayer(12.5, 11.5)

	require.Equal(t, b2aJSON(t, l.HarnessState()), b2aJSON(t, l2.HarnessState()), "restored is the same light")
	require.Equal(t, b2aLightSteps(t, c, l), b2aLightSteps(t, c2, l2), "and stays the same light when both run on")
}

// A new game saved at once: no sources, next id 1.
func TestLightSnapshotOfANewGame(t *testing.T) {
	c := NewClock(DefaultClockDials())
	t.Cleanup(c.Close)

	orig := NewLight(c, DefaultLightDials())
	t.Cleanup(orig.Close)

	cp := NewLight(c, DefaultLightDials())
	t.Cleanup(cp.Close)
	require.NoError(t, cp.HarnessSet("carried_source", "torch"))

	require.NoError(t, cp.Restore(b2aThroughJSON(t, orig.Snapshot())))
	require.Equal(t, b2aJSON(t, orig.HarnessState()), b2aJSON(t, cp.HarnessState()), "the torch is gone and next_id is 1")
}

func TestLightSnapshotRefusesWhatCannotBe(t *testing.T) {
	_, l := b2aLightWorld(t)
	good := l.Snapshot()

	for name, bad := range map[string]func(s *LightSnapshot){
		"next_id zero":           func(s *LightSnapshot) { s.NextID = 0 },
		"an id at next_id":       func(s *LightSnapshot) { s.NextID = s.Sources[len(s.Sources)-1].ID },
		"ids out of order":       func(s *LightSnapshot) { s.Sources[0], s.Sources[1] = s.Sources[1], s.Sources[0] },
		"a kind with no name":    func(s *LightSnapshot) { s.Sources[1].Kind = "" },
		"two carried":            func(s *LightSnapshot) { s.Sources[1].Carried = true },
		"a radius below nothing": func(s *LightSnapshot) { s.Sources[1].Radius = -1 },
		"a repeated id":          func(s *LightSnapshot) { s.Sources[1].ID = s.Sources[0].ID },
	} {
		var s LightSnapshot

		raw, _ := json.Marshal(good)
		require.NoError(t, json.Unmarshal(raw, &s))
		bad(&s)

		before := b2aJSON(t, l.HarnessState())
		require.Error(t, l.Restore(s), name)
		require.Equal(t, before, b2aJSON(t, l.HarnessState()), "%s: a refused restore changes nothing", name)
	}
}

func TestLightSnapshotEveryFieldIsLoadBearing(t *testing.T) {
	c, l := b2aLightWorld(t)
	clockSnap, snap := c.Snapshot(), l.Snapshot()
	ref := b2aLightSteps(t, c, l)

	b2aSweep(t, snap, ref, map[string]string{
		"sources[0].x": "the carried torch shines from the player (Light.at); where it was lit is read by nothing",
		"sources[0].y": "the carried torch shines from the player (Light.at); where it was lit is read by nothing",
	}, func(raw []byte) (string, error) {
		var s LightSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		c2 := NewClock(DefaultClockDials())
		defer c2.Close()

		if err := c2.Restore(clockSnap); err != nil {
			return "", err
		}

		l2 := NewLight(c2, DefaultLightDials())
		defer l2.Close()

		if err := l2.Restore(s); err != nil {
			return "", err
		}

		return b2aLightSteps(t, c2, l2), nil
	})
}
