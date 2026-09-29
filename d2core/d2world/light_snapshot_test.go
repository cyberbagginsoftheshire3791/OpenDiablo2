package d2world

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// b2aLightWorld is a clock in the deep night and a light model with every kind
// of source the game makes: a carried torch lit and burning, a placed hearth,
// a placed torch burning, a placed torch burnt out, a removed source, and a
// newest torch removed -- so next_id is not one past the last source, as in a
// game after his torch has left the model and been lit again.
//
// WHAT TAKES A TORCH OUT OF THE MODEL (corrected at the B2a review, C6: this
// said "a douse"). A douse does NOT: L on a lit torch only unlights it, and the
// source keeps its id and its minutes. A torch leaves the model when it BURNS
// OUT (the game spends it from his hand, Light.Remove) or is PUT AWAY
// (UnequipSlot -> returnTorchToPack moves its minutes back into the kit and
// removes the source); the next light Adds a new source with a new id.
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
	require.NoError(t, l.HarnessSet("place_source", map[string]interface{}{"kind": "torch", "x": 1.0, "y": 1.0})) // 6
	require.NoError(t, l.HarnessSet("remove_source", 6.0))

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

// b2aLightClasses is every field of the light model and of a source, labelled.
func b2aLightClasses() []b2aClass {
	return []b2aClass{
		{Light{}, map[string]string{
			"carriedBurnRate": "D: his talents (T3), set again when the load applies his progress",
			"dials":           "W: construction dials; the game builds the model with the defaults",
			"clock":           "W: the clock it reads; restored on its own",
			"sources":         "S:sources",
			"nextID":          "S:next_id",
			"playerX":         "D: the player's position, set by SetPlayer every frame",
			"playerY":         "D: the player's position, set by SetPlayer every frame",
		}},
		{Source{}, map[string]string{
			"ID":      "S:sources[0].id",
			"Kind":    "S:sources[0].kind",
			"Radius":  "D: the kind's dial (TorchRadius, HearthRadius), set by radiusOf at Add and at Restore (D2)",
			"Burn":    "S:sources[0].burn",
			"Lit":     "S:sources[0].lit",
			"Carried": "S:sources[0].carried",
			"X":       "S:sources[0].x",
			"Y":       "S:sources[0].y",
		}},
	}
}

func TestLightSnapshotFieldsClassified(t *testing.T) {
	_, l := b2aLightWorld(t)
	snap := l.Snapshot()

	for _, c := range b2aLightClasses() {
		b2aClassified(t, c.kind, snap, c.fields)
	}
}

func TestLightSnapshotRoundTrip(t *testing.T) {
	c, l := b2aLightWorld(t)

	snap := b2aThroughJSON(t, l.Snapshot())
	require.Equal(t, l.Snapshot(), snap, "the snapshot survives JSON exactly")
	require.Len(t, snap.Sources, 4)
	require.Equal(t, 7, snap.NextID, "not one past the last source (5): the doused torch spent 6")

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
		"next_id zero":        func(s *LightSnapshot) { s.NextID = 0 },
		"an id at next_id":    func(s *LightSnapshot) { s.NextID = s.Sources[len(s.Sources)-1].ID },
		"ids out of order":    func(s *LightSnapshot) { s.Sources[0], s.Sources[1] = s.Sources[1], s.Sources[0] },
		"a kind with no name": func(s *LightSnapshot) { s.Sources[1].Kind = "" },
		"two carried":         func(s *LightSnapshot) { s.Sources[1].Carried = true },
		"a repeated id":       func(s *LightSnapshot) { s.Sources[1].ID = s.Sources[0].ID },

		// The B2a review's B2: only what Snapshot could have written. Source
		// 1 is his lit torch, 2 the hearth, 4 a placed torch burnt out.
		"a torch that never burns down": func(s *LightSnapshot) { s.Sources[0].Burn = -1 },
		"a lit torch with nothing left": func(s *LightSnapshot) { s.Sources[0].Burn = 0 },
		"a hearth that burns down":      func(s *LightSnapshot) { s.Sources[1].Burn = 30 },
		"a hearth at zero":              func(s *LightSnapshot) { s.Sources[1].Burn = 0 },
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

	exempt := map[string]string{
		"sources[0].x": "the carried torch shines from the player (Light.at); where it was lit is read by nothing",
		"sources[0].y": "the carried torch shines from the player (Light.at); where it was lit is read by nothing",
	}

	try := func(raw []byte) (string, error) {
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
	}

	b2aExercised(t, b2aSweep(t, snap, ref, exempt, try), b2aLightClasses()...)

	// next_id, the ids and the kinds are only ever refused when zeroed; a valid
	// other value must show. The fixture removed its newest torch, so the saved
	// next_id is NOT one past the last source, as a game's is after his torch
	// burns out or is put away and is lit again.
	//
	// A kind changes with its burn, since the two must agree (Validate): a
	// hearth is fuel-fed (-1), a torch has its minutes. The radius follows the
	// kind from the dials (D2), so a kind that is ignored is seen in the light.
	b2aMustDiverge(t, snap, ref, map[string]func(s *LightSnapshot){
		"next_id one past the last source": func(s *LightSnapshot) { s.NextID = s.Sources[len(s.Sources)-1].ID + 1 },
		"a source renumbered":              func(s *LightSnapshot) { s.Sources[3].ID++ },
		"the hearth a torch":               func(s *LightSnapshot) { s.Sources[1].Kind, s.Sources[1].Burn = SourceTorch, 60 },
		"a placed torch a hearth":          func(s *LightSnapshot) { s.Sources[2].Kind, s.Sources[2].Burn = SourceHearth, -1 },
	}, try)
}

// D1 (28 Sep 2026): THE LIGHT MODEL IS THE TRUTH ON LOAD. His carried torch
// comes back exactly as it was -- the same id, the same next_id, lit or doused,
// its minutes to the hundredth -- and burns on in step. Both states matter: a
// douse keeps the source and its minutes (L only unlights it), and a load that
// "re-lit through the L path", as trap 4 first prescribed, would have given
// the torch a new id, moved next_id, and LIT the doused one.
func TestLightRestoresTheCarriedTorchExactly(t *testing.T) {
	for _, lit := range []bool{true, false} {
		c, l := b2aLightWorld(t)

		carried := l.Carried()
		require.NotNil(t, carried)
		carried.Lit = lit

		snap := b2aThroughJSON(t, l.Snapshot())
		burnAtSave := carried.Burn

		c2 := NewClock(DefaultClockDials())
		t.Cleanup(c2.Close)
		require.NoError(t, c2.Restore(c.Snapshot()))

		l2 := NewLight(c2, DefaultLightDials())
		t.Cleanup(l2.Close)
		require.NoError(t, l2.Restore(snap))
		l2.SetPlayer(12.5, 11.5)

		got := l2.Carried()
		require.NotNil(t, got, "lit=%v: the carried torch is restored, not left for the L key to re-add", lit)
		require.Equal(t, *carried, *got, "lit=%v: id, kind, radius, burn, lit and carried, exactly", lit)
		require.Equal(t, l.nextID, l2.nextID, "lit=%v: next_id unchanged", lit)

		for i := 0; i < 5; i++ {
			l.Advance(c.Advance(6))
			l2.Advance(c2.Advance(6))
			require.Equal(t, *l.Carried(), *l2.Carried(), "lit=%v, step %d: they burn on in step", lit, i)
		}

		if lit {
			require.Less(t, l2.Carried().Burn, burnAtSave, "the lit one burned on")
		} else {
			require.Equal(t, burnAtSave, l2.Carried().Burn, "the doused one kept its minutes")
		}

		require.Equal(t, b2aJSON(t, l.HarnessState()), b2aJSON(t, l2.HarnessState()), "lit=%v", lit)
	}
}

// D2 (28 Sep 2026): a value derived from dials is not saved. A source's radius
// is its kind's dial, so the snapshot carries no radius and a restore takes
// the radius from the dials of the model it restores into -- a save made
// before a retune loads with the new tuning, never the old one.
func TestLightRadiusIsTheDialsNotTheSave(t *testing.T) {
	_, l := b2aLightWorld(t)
	snap := l.Snapshot()

	tree := b2aTree(t, snap)
	for i := range snap.Sources {
		_, has := b2aResolve(tree, "sources["+itoa(i)+"].radius")
		require.False(t, has, "source %d: no radius in the save", i)
	}

	c2 := NewClock(DefaultClockDials())
	t.Cleanup(c2.Close)

	dials := DefaultLightDials()
	dials.TorchRadius, dials.HearthRadius = 7, 11

	l2 := NewLight(c2, dials)
	t.Cleanup(l2.Close)
	require.NoError(t, l2.Restore(snap))

	for _, src := range l2.sources {
		want := 7.0
		if src.Kind == SourceHearth {
			want = 11
		}

		require.Equal(t, want, src.Radius, "source %d (%s) takes its radius from the restoring model's dials", src.ID, src.Kind)
	}
}
