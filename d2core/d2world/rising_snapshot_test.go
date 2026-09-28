package d2world

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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

	rw := newB2aRising(1462, fakeNight{band: -1, stage: StageDusk})
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
	require.NotZero(t, rw.r.pressure, "and moves the pressure")

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
// rising built on a DIFFERENT seed (so its stream can only be the snapshot's)
// and restored.
func b2aRisingCopy(orig *b2aRisingWorld, corpses CorpsesSnapshot, snap RisingSnapshot) (*b2aRisingWorld, error) {
	rw := newB2aRising(7, orig.night)
	rw.w.minute = orig.w.minute

	if err := rw.w.c.Restore(corpses); err != nil {
		return nil, err
	}

	if err := rw.r.Restore(snap); err != nil {
		return nil, err
	}

	return rw, nil
}

func TestRisingSnapshotFieldsClassified(t *testing.T) {
	rw := b2aRisingFixture(t)

	b2aClassified(t, Rising{}, rw.r.Snapshot(), map[string]string{
		"dials":      "W: the game's numbers (P, hasty weight, edge floor); harness writes to them are test setup",
		"corpses":    "W: the registry it rolls, restored on its own",
		"band":       "W: the spawn tables' band, read from the clock",
		"stage":      "W: the clock's stage",
		"rng":        "S:rng",
		"pressure":   "S:pressure",
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
	})
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

// Seeds are int64 and a wall-clock seed is past 2^53: the snapshot must carry
// one exactly through a typed decode AND through a reader that decodes numbers
// into float64 (B1 notes, section 2).
func TestRisingSnapshotSeedSurvivesJSON(t *testing.T) {
	const seed = int64(1)<<62 + 12345 // past 2^53; float64 cannot hold it

	require.NotEqual(t, seed, int64(float64(seed)), "the control: float64 loses this seed")

	rw := newB2aRising(seed, fakeNight{band: -1, stage: StageDay})
	snap := rw.r.Snapshot()

	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	var typed RisingSnapshot
	require.NoError(t, json.Unmarshal(raw, &typed))
	require.Equal(t, seed, typed.RNG.Seed, "a typed decode is exact")

	var loose map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &loose))

	got, ok := loose["rng"].(map[string]interface{})["seed"].(string)
	require.True(t, ok, "the seed is written as a string, got %T", loose["rng"].(map[string]interface{})["seed"])
	require.Equal(t, strconv.FormatInt(seed, 10), got, "and a float64 reader still reads it exactly")

	cp := newB2aRising(1, fakeNight{band: -1, stage: StageDay})
	require.NoError(t, cp.r.Restore(typed))
	require.Equal(t, rw.r.rng.Float64(), cp.r.rng.Float64(), "the restored stream is the seed's")
}

// A new game saved at once, by day, before any band.
func TestRisingSnapshotOfANewGame(t *testing.T) {
	orig := newB2aRising(1462, fakeNight{band: -1, stage: StageDawn})
	cp := newB2aRising(5, fakeNight{band: 1, stage: StageNight})

	require.NoError(t, cp.r.Restore(b2aThroughJSON(t, orig.r.Snapshot())))
	require.Equal(t, b2aJSON(t, orig.r.HarnessState()), b2aJSON(t, cp.r.HarnessState()))
}

func TestRisingSnapshotRefusesWhatCannotBe(t *testing.T) {
	good := b2aRisingFixture(t).r.Snapshot()

	for name, bad := range map[string]func(s *RisingSnapshot){
		"no stage":       func(s *RisingSnapshot) { s.LastStage = "" },
		"a band too far": func(s *RisingSnapshot) { s.LastBand = risingBands },
		"a band before":  func(s *RisingSnapshot) { s.LastBand = -2 },
		"a count below":  func(s *RisingSnapshot) { s.Wandered = -1 },
	} {
		s := good
		bad(&s)

		rw := newB2aRising(3, fakeNight{band: -1, stage: StageDay})
		before := rw.r.Snapshot()
		require.Error(t, rw.r.Restore(s), name)
		require.Equal(t, before, rw.r.Snapshot(), "%s: a refused restore changes nothing", name)
	}
}

func TestRisingSnapshotEveryFieldIsLoadBearing(t *testing.T) {
	orig := b2aRisingFixture(t)
	corpses, snap := orig.w.c.Snapshot(), orig.r.Snapshot()

	ref := func() string {
		cp, err := b2aRisingCopy(orig, corpses, snap)
		require.NoError(t, err)

		return b2aRisingSteps(t, cp)
	}()

	b2aSweep(t, snap, ref, nil, func(raw []byte) (string, error) {
		var s RisingSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		cp, err := b2aRisingCopy(orig, corpses, s)
		if err != nil {
			return "", err
		}

		return b2aRisingSteps(t, cp), nil
	})
}
