package d2world

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// b2aRisingWorld is a rising wired the way the game wires it: a raise that
// stands a body up as a member (named from the body and the minute, so two
// worlds name it alike with no counter of their own), a first light that lays
// the walking dead down, an edge wanderer, and a clock for the Downed window.
type b2aRisingWorld struct {
	night  fakeNight
	w      *b2aCorpsesWorld
	r      *Rising
	events []string
}

func newB2aRising(seed int64, night fakeNight) *b2aRisingWorld {
	rw := &b2aRisingWorld{night: night, w: newB2aCorpses()}

	dials := DefaultRisingDials()
	dials.EdgeFloor = 1

	rw.r = NewRising(rw.w.c, func() int { return rw.night.band }, func() Stage { return rw.night.stage }, seed, dials)
	rw.r.SetClock(func() float64 { return rw.w.minute })
	rw.r.SetRaise(func(b Corpse) string {
		m := fmt.Sprintf("r:%s@%g", b.ID, rw.w.minute)
		rw.events = append(rw.events, "raise "+m)

		return m
	})
	rw.r.SetWander(func() (string, float64, float64) {
		m := fmt.Sprintf("w@%g", rw.w.minute)
		rw.events = append(rw.events, "wander "+m)

		return m, 30, 31
	}, "a stranger")
	rw.r.SetFirstLight(func() {
		rw.events = append(rw.events, "first light")

		for _, b := range rw.w.c.All() {
			if m := rw.w.c.walker[b.ID]; m != "" {
				rw.w.c.Fall(m, "", b.X+0.5, b.Y)
			}
		}
	})

	return rw
}

func (rw *b2aRisingWorld) at(band int, stage Stage, minute float64) {
	rw.night.set(band, stage)
	rw.w.minute = minute
	rw.r.Advance()
}

// b2aRisingFixture runs a whole night and a dawn, and into the next night's
// first band: men fall, some rise, one is cut down and stands again when his
// window runs out, a wanderer comes at the edge, the dawn counts the open
// dead into the pressure and first light lays the walkers down, and a rite
// takes a little of it back.
func b2aRisingFixture(t *testing.T) *b2aRisingWorld {
	t.Helper()

	return b2aRisingFixtureOnSeed(t, d2rand.Derive(b2aWorldSeed, d2rand.StreamRising))
}

func b2aRisingFixtureOnSeed(t *testing.T, seed int64) *b2aRisingWorld {
	t.Helper()

	rw := newB2aRising(seed, fakeNight{band: -1, stage: StageDusk})
	c := rw.w.c

	for i := 0; i < 10; i++ {
		c.Fall(fmt.Sprintf("m:%d", i), "men", float64(i), 2)
	}

	c.Fall("dog:1", "dogs", 20, 2)
	c.Fall("m:grave", "men", 21, 2)
	c.Bury("m:grave")
	c.Fall("m:staked", "men", 22, 2)
	c.Close("m:staked")

	rw.at(0, StageNight, 100)
	rw.at(1, StageNight, 150)

	// Cut one walker down; his window runs out three minutes on.
	for _, b := range c.All() {
		if m := c.walker[b.ID]; m != "" {
			c.Fall(m, "", b.X, b.Y+1)
			break
		}
	}

	rw.at(1, StageNight, 155)
	rw.at(2, StageNight, 200)
	rw.at(-1, StageDawn, 300)
	rw.r.Rite()
	rw.at(-1, StageDay, 600)
	rw.at(0, StageNight, 1500)

	require.Positive(t, rw.r.risen, "the fixture raises the dead")
	require.Positive(t, rw.r.stoodAgain, "and stands a Downed man again")
	require.Positive(t, rw.r.wandered, "and brings a wanderer")
	require.NotZero(t, rw.r.accrued, "and moves the pressure")

	return rw
}

// b2aRisingSteps runs the next night on: the band already rolled is not rolled
// again, a walker is cut down and stands when his window is up, the edge band
// brings a wanderer, a dawn is read, and the night after rolls once more.
func b2aRisingSteps(t *testing.T, rw *b2aRisingWorld) string {
	t.Helper()

	var trace strings.Builder

	observe := func() {
		trace.WriteString(b2aJSON(t, rw.r.HarnessState()))
		trace.WriteString(b2aJSON(t, rw.w.c.HarnessState()))
		trace.WriteString(b2aJSON(t, rw.events))
	}

	observe()

	for _, step := range []struct {
		band   int
		stage  Stage
		minute float64
		cut    bool
	}{
		{0, StageNight, 1510, true}, {0, StageNight, 1520, false}, {1, StageNight, 1560, false},
		{2, StageNight, 1600, false}, {-1, StageDawn, 1700, false}, {-1, StageDay, 1900, false},
		{0, StageNight, 2900, false}, {1, StageNight, 2950, false},
	} {
		if step.cut {
			for _, b := range rw.w.c.All() {
				if m := rw.w.c.walker[b.ID]; m != "" {
					rw.w.c.Fall(m, "", b.X, b.Y+1)
					break
				}
			}
		}

		rw.at(step.band, step.stage, step.minute)
		observe()
	}

	return trace.String()
}

// b2aRisingCopy is a world at the original's moment: the corpses restored, a
// rising built on a DIFFERENT game's stream (so its stream can only be the
// snapshot's) and restored as the saved game's (b2aWorldSeed).
func b2aRisingCopy(orig *b2aRisingWorld, corpses CorpsesSnapshot, snap RisingSnapshot) (*b2aRisingWorld, error) {
	rw := newB2aRising(d2rand.Derive(7, d2rand.StreamRising), orig.night)
	rw.w.minute = orig.w.minute

	if err := rw.w.c.Restore(corpses); err != nil {
		return nil, err
	}

	if err := rw.r.Restore(snap, b2aWorldSeed); err != nil {
		return nil, err
	}

	return rw, nil
}

// b2aRisingClasses is every field of the rising, labelled.
func b2aRisingClasses() []b2aClass {
	return []b2aClass{{Rising{}, map[string]string{
		"dials":      "W: the game's numbers (P, hasty weight, edge floor, the pressure constant); harness writes are test setup",
		"corpses":    "W: the registry it rolls, restored on its own",
		"band":       "W: the spawn tables' band, read from the clock",
		"stage":      "W: the clock's stage",
		"rng":        "S:rng",
		"accrued":    "S:pressure_accrued",
		"lastBand":   "S:last_band",
		"lastStage":  "S:last_stage",
		"rolls":      "S:rolls",
		"risen":      "S:risen",
		"raise":      "W: what stands a body up in the world (the game screen)",
		"firstLight": "W: what hears the night end (the game screen)",
		"now":        "W: the world minutes the Downed window runs on",
		"stoodAgain": "S:stood_again",
		"wander":     "W: what stands an edge wanderer up (the spawn tables)",
		"wanderWas":  "W: what the bestiary calls a wanderer, given by the game",
		"wandered":   "S:wandered",
	}}}
}

func TestRisingSnapshotFieldsClassified(t *testing.T) {
	rw := b2aRisingFixture(t)

	for _, c := range b2aRisingClasses() {
		b2aClassified(t, c.kind, rw.r.Snapshot(), c.fields)
	}
}

func TestRisingSnapshotRoundTrip(t *testing.T) {
	orig := b2aRisingFixture(t)
	corpses := orig.w.c.Snapshot()

	snap := b2aThroughJSON(t, orig.r.Snapshot())
	require.Equal(t, orig.r.Snapshot(), snap, "the snapshot survives JSON exactly")
	require.Equal(t, "night", snap.LastStage)
	require.Positive(t, snap.RNG.Draws)

	cp, err := b2aRisingCopy(orig, corpses, snap)
	require.NoError(t, err)

	require.Equal(t, orig.r.Snapshot(), cp.r.Snapshot(), "restored is the same rising, stream included")

	orig.events = nil
	require.Equal(t, b2aRisingSteps(t, orig), b2aRisingSteps(t, cp), "and it rolls the same night on")
}

// A wall-clock GAME seed is past 2^53. The rising's stream runs on the seed
// Derive gives it (below 2^31, exact in any reader); what must survive is the
// check against the game seed, which the load reads as an int64 (B1 notes,
// section 2). The stream's own round trip through JSON, at a seed past 2^53,
// is d2rand's TestStreamStateSeedSurvivesJSON.
func TestRisingSnapshotSeedSurvivesJSON(t *testing.T) {
	const world = int64(1)<<62 + 12345 // past 2^53; float64 cannot hold it

	require.NotEqual(t, world, int64(float64(world)), "the control: float64 loses this seed")

	rw := newB2aRising(d2rand.Derive(world, d2rand.StreamRising), fakeNight{band: -1, stage: StageDay})
	_ = rw.r.rng.Float64()
	snap := b2aThroughJSON(t, rw.r.Snapshot())

	cp := newB2aRising(d2rand.Derive(1, d2rand.StreamRising), fakeNight{band: -1, stage: StageDay})
	require.NoError(t, cp.r.Restore(snap, world))
	require.Equal(t, rw.r.rng.Float64(), cp.r.rng.Float64(), "the restored stream is the saved game's")

	lost := newB2aRising(d2rand.Derive(1, d2rand.StreamRising), fakeNight{band: -1, stage: StageDay})
	require.Error(t, lost.r.Restore(snap, int64(float64(world))), "the game seed read through float64 is another game")
}

// A new game saved at once, by day, before any band.
func TestRisingSnapshotOfANewGame(t *testing.T) {
	orig := newB2aRising(d2rand.Derive(b2aWorldSeed, d2rand.StreamRising), fakeNight{band: -1, stage: StageDawn})
	cp := newB2aRising(d2rand.Derive(5, d2rand.StreamRising), fakeNight{band: 1, stage: StageNight})

	require.NoError(t, cp.r.Restore(b2aThroughJSON(t, orig.r.Snapshot()), b2aWorldSeed))
	require.Equal(t, b2aJSON(t, orig.r.HarnessState()), b2aJSON(t, cp.r.HarnessState()))
}

func TestRisingSnapshotRefusesWhatCannotBe(t *testing.T) {
	orig := b2aRisingFixture(t)
	corpses, good := orig.w.c.Snapshot(), orig.r.Snapshot()

	require.Equal(t, 0, good.LastBand, "the fixture saves in band 0 of a night")
	require.Positive(t, good.Wandered, "and after a wanderer came")

	combat := d2rand.StreamState{Seed: d2rand.Derive(b2aWorldSeed, d2rand.StreamCombat), Draws: good.RNG.Draws}

	for name, bad := range map[string]func(s *RisingSnapshot){
		"no stage":       func(s *RisingSnapshot) { s.LastStage = "" },
		"a band too far": func(s *RisingSnapshot) { s.LastBand = risingBands },
		"a band before":  func(s *RisingSnapshot) { s.LastBand = -2 },
		"a count below":  func(s *RisingSnapshot) { s.Wandered = -1 },

		// The B2a review's B2 and B1: only what Snapshot could have written.
		"a band by day":                   func(s *RisingSnapshot) { s.LastStage = StageDay.String() },
		"a night with no band":            func(s *RisingSnapshot) { s.LastBand = -1 },
		"fewer wanderers than lie there":  func(s *RisingSnapshot) { s.Wandered = 0 },
		"the combat stream's block":       func(s *RisingSnapshot) { s.RNG = combat },
		"another game's stream":           func(s *RisingSnapshot) { s.RNG.Seed = d2rand.Derive(7, d2rand.StreamRising) },
		"more draws than a save may hold": func(s *RisingSnapshot) { s.RNG.Draws = d2rand.MaxDraws + 1 },
	} {
		s := good
		bad(&s)

		rw := newB2aRising(d2rand.Derive(3, d2rand.StreamRising), fakeNight{band: -1, stage: StageDay})
		require.NoError(t, rw.w.c.Restore(corpses))

		before := rw.r.Snapshot()
		require.Error(t, rw.r.Validate(s, b2aWorldSeed, corpses), "%s: Validate", name)
		require.Error(t, rw.r.Restore(s, b2aWorldSeed), name)
		require.Equal(t, before, rw.r.Snapshot(), "%s: a refused restore changes nothing", name)
	}

	// The control: the untouched snapshot passes both.
	rw := newB2aRising(d2rand.Derive(3, d2rand.StreamRising), fakeNight{band: -1, stage: StageDay})
	require.NoError(t, rw.w.c.Restore(corpses))
	require.NoError(t, rw.r.Validate(good, b2aWorldSeed, corpses))
	require.NoError(t, rw.r.Restore(good, b2aWorldSeed))
}

// D4: Validate checks against the bodies it is GIVEN -- at a load that checks
// every block first, the file's corpses, while the registry is still empty.
// Restore checks against the registry, which by then holds them.
func TestRisingValidateReadsTheBodiesItIsGiven(t *testing.T) {
	orig := b2aRisingFixture(t)
	corpses, snap := orig.w.c.Snapshot(), orig.r.Snapshot()
	snap.Wandered = 0

	rw := newB2aRising(d2rand.Derive(3, d2rand.StreamRising), fakeNight{band: -1, stage: StageDay})
	require.Error(t, rw.r.Validate(snap, b2aWorldSeed, corpses), "the file's bodies hold wanderer:1")
	require.NoError(t, rw.r.Validate(snap, b2aWorldSeed, CorpsesSnapshot{}), "the control: with no bodies, 0 is fine")
}

// D2 (28 Sep 2026): pressure is saved as the part the game ACCRUED, never the
// dial's constant. A game played with one constant and loaded by a build with
// another runs on the loading build's constant plus what was accrued; and a
// script's pressure is a dial the save does not carry.
func TestRisingSavesOnlyTheAccruedPressure(t *testing.T) {
	dials := DefaultRisingDials()
	dials.Pressure = 0.25

	w := newB2aCorpses()
	r := NewRising(w.c, func() int { return -1 }, func() Stage { return StageDay },
		d2rand.Derive(b2aWorldSeed, d2rand.StreamRising), dials)

	r.Rite()
	r.Rite()

	accrued := -2 * dials.PerRite
	require.InDelta(t, 0.25+accrued, r.Pressure(), 1e-12)

	snap := b2aThroughJSON(t, r.Snapshot())
	require.Equal(t, accrued, snap.Accrued, "the save holds what was accrued, not the constant")

	require.NoError(t, r.HarnessSet("pressure", 0.9))
	require.InDelta(t, 0.9, r.Pressure(), 1e-12, "a script's pressure reads back")
	require.Equal(t, snap, r.Snapshot(), "and is a dial: the saved part does not move")

	w2 := newB2aCorpses()
	r2 := NewRising(w2.c, func() int { return -1 }, func() Stage { return StageDay },
		d2rand.Derive(9, d2rand.StreamRising), DefaultRisingDials())
	require.NoError(t, r2.Restore(snap, b2aWorldSeed))
	require.Equal(t, DefaultRisingDials().Pressure+accrued, r2.Pressure(),
		"loaded, it is the loading build's constant plus the accrued part")
	require.Equal(t, accrued, r2.HarnessState()["pressure_accrued"], "and the provider reports the saved part")
}

func TestRisingSnapshotEveryFieldIsLoadBearing(t *testing.T) {
	orig := b2aRisingFixture(t)
	corpses, snap := orig.w.c.Snapshot(), orig.r.Snapshot()

	// THE ORIGINAL, run on: a second fixture, built the same way, never saved
	// and never restored (the B2a review's C1 -- this was a restored copy, so
	// the baseline compared copy with copy and could not see a restore that
	// differs from the original in the same way every time).
	ref := func() string {
		o := b2aRisingFixture(t)
		o.events = nil

		return b2aRisingSteps(t, o)
	}()

	try := func(raw []byte) (string, error) {
		var s RisingSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		cp, err := b2aRisingCopy(orig, corpses, s)
		if err != nil {
			return "", err
		}

		return b2aRisingSteps(t, cp), nil
	}

	b2aExercised(t, b2aSweep(t, snap, ref, nil, try), b2aRisingClasses()...)

	// last_stage and last_band move together (Validate). Saved in band 0 of a
	// night; a valid other moment -- the dusk before, no band yet -- must show.
	// (A Restore that parses last_stage and ignores it keeps NewRising's
	// sampled stage, which here is the saved one; TestRisingSnapshotOfANewGame
	// samples another, and is where that is caught.)
	b2aMustDiverge(t, snap, ref, map[string]func(s *RisingSnapshot){
		"the dusk before the night": func(s *RisingSnapshot) { s.LastStage, s.LastBand = StageDusk.String(), -1 },
		"one wanderer more":         func(s *RisingSnapshot) { s.Wandered++ },
	}, try)
}
