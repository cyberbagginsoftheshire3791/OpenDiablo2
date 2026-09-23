package d2world

import (
	"fmt"
	"math"
	"testing"
)

// fakeNight is a band and a stage a test can move by hand.
type fakeNight struct {
	band  int
	stage Stage
}

func (n *fakeNight) set(band int, stage Stage) { n.band, n.stage = band, stage }

func newTestRising(t *testing.T, dials RisingDials) (*Rising, *Corpses, *fakeNight) {
	t.Helper()

	night := &fakeNight{band: -1, stage: StageDay}
	corpses := NewCorpses(nil, nil)
	r := NewRising(corpses, func() int { return night.band }, func() Stage { return night.stage }, 1462, dials)

	return r, corpses, night
}

// S1 §6.3, the M4.7 note's step-2 assertion: over [200] seeded rolls an open
// man's body rises within ±[0.05] of P; a closed one never; a carcass never.
func TestRisingRate(t *testing.T) {
	dials := DefaultRisingDials()
	r, corpses, night := newTestRising(t, dials)

	for i := 0; i < 200; i++ {
		corpses.FallHuman(fmt.Sprintf("open:%d", i), 0, 0)
		corpses.FallHuman(fmt.Sprintf("closed:%d", i), 0, 0)
		corpses.Close(fmt.Sprintf("closed:%d", i))
		corpses.Fall(fmt.Sprintf("dog:%d", i), "dogs", 0, 0)
	}

	night.set(0, StageNight)
	r.Advance()

	rose := map[string]int{}
	for _, b := range corpses.All() {
		if b.State == CorpseRisen {
			rose[b.ID[:3]]++
		}
	}

	if got := float64(rose["ope"]) / 200; math.Abs(got-dials.P) > 0.05 {
		t.Errorf("an open body rises at P=%.2f: %.3f", dials.P, got)
	}

	if rose["clo"] != 0 || rose["dog"] != 0 {
		t.Errorf("a closed body and a carcass never rise: %v", rose)
	}

	if r.rolls != 1 || r.risen != rose["ope"] {
		t.Errorf("one band, one roll: rolls %d, risen %d", r.rolls, r.risen)
	}
}

// A hasty grave rolls at P x HastyWeight: never at weight 0, always at P=1 and
// weight 1.
func TestRisingHastyGrave(t *testing.T) {
	for _, c := range []struct {
		weight float64
		rises  bool
	}{{0, false}, {1, true}} {
		dials := DefaultRisingDials()
		dials.P, dials.HastyWeight = 1, c.weight

		r, corpses, night := newTestRising(t, dials)
		corpses.FallHuman("grave", 0, 0)
		corpses.Bury("grave")

		night.set(0, StageNight)
		r.Advance()

		if got := corpses.All()[0].State == CorpseRisen; got != c.rises {
			t.Errorf("hasty weight %.0f: rose %v, want %v", c.weight, got, c.rises)
		}
	}
}

// Once per band, and only in the deep night: dusk and dawn never roll, a
// frame inside a band does not roll it again, and a jump over bands (a long
// sleep) rolls every band it crossed.
func TestRisingOncePerBand(t *testing.T) {
	dials := DefaultRisingDials()
	dials.P = 0

	r, _, night := newTestRising(t, dials)

	for _, s := range []Stage{StageDay, StageDusk, StageDay} {
		night.set(-1, s)
		r.Advance()
	}

	if r.rolls != 0 {
		t.Fatalf("nothing rolls outside the deep night: %d", r.rolls)
	}

	night.set(0, StageNight)
	r.Advance()
	r.Advance()

	if r.rolls != 1 {
		t.Fatalf("a band rolls once however many frames it lasts: %d", r.rolls)
	}

	night.set(2, StageNight) // slept through band 1
	r.Advance()

	if r.rolls != 3 {
		t.Fatalf("a jump rolls every band it crossed: %d", r.rolls)
	}

	night.set(-1, StageDawn)
	r.Advance()

	if r.rolls != 3 {
		t.Fatalf("dawn does not roll: %d", r.rolls)
	}

	// A new night entered in its first band, then left at once (a jump to
	// day): the bands it skipped are rolled on the way out -- three, no more.
	night.set(-1, StageDay)
	r.Advance()
	night.set(0, StageNight)
	r.Advance()
	night.set(-1, StageDay)
	r.Advance()

	if r.rolls != 6 {
		t.Fatalf("leaving the night rolls what was left of it: %d", r.rolls)
	}
}

// The control: P = 0 and no pressure -- a whole night, nothing rises.
func TestRisingNothingAtZero(t *testing.T) {
	dials := DefaultRisingDials()
	dials.P = 0

	r, corpses, night := newTestRising(t, dials)
	corpses.FallHuman("a", 0, 0)

	for b := 0; b < risingBands; b++ {
		night.set(b, StageNight)
		r.Advance()
	}

	if corpses.All()[0].State != CorpseFresh || r.risen != 0 {
		t.Fatalf("P=0: nothing rises: %v", corpses.All()[0])
	}
}

// Soul pressure: +PerOpenAtDawn for each man's body open at dawn (a carcass,
// a grave and a closed body do not count), -PerRite for each rite; it adds to
// P, and each dawn counts once.
func TestRisingPressure(t *testing.T) {
	dials := DefaultRisingDials()
	dials.P = 0

	r, corpses, night := newTestRising(t, dials)

	corpses.FallHuman("open1", 0, 0)
	corpses.FallHuman("open2", 0, 0)
	corpses.FallHuman("grave", 0, 0)
	corpses.Bury("grave")
	corpses.FallHuman("staked", 0, 0)
	corpses.Close("staked")
	corpses.Fall("dog", "dogs", 0, 0)

	r.Rite()

	night.set(0, StageNight)
	r.Advance()
	night.set(-1, StageDawn)
	r.Advance()

	want := 2*dials.PerOpenAtDawn - dials.PerRite
	if math.Abs(r.Pressure()-want) > 1e-9 {
		t.Fatalf("pressure after one dawn with two open: %.4f, want %.4f", r.Pressure(), want)
	}

	if math.Abs(r.Chance()-want) > 1e-9 {
		t.Fatalf("pressure adds to P: %.4f", r.Chance())
	}
}

// T3's lesson: the band a session opens in is seeded, not rolled -- a session
// that opens in band 1 rolls band 2 on entering it, and no more.
func TestRisingSeededBand(t *testing.T) {
	night := &fakeNight{band: 1, stage: StageNight}
	r := NewRising(NewCorpses(nil, nil), func() int { return night.band }, func() Stage { return night.stage }, 1, DefaultRisingDials())

	r.Advance()

	if r.rolls != 0 {
		t.Fatalf("the opening band is not rolled: %d", r.rolls)
	}

	night.set(2, StageNight)
	r.Advance()

	if r.rolls != 1 {
		t.Fatalf("the next band is: %d", r.rolls)
	}
}

// The harness may make the night certain or empty, never nonsense.
func TestRisingHarnessSet(t *testing.T) {
	r, _, _ := newTestRising(t, DefaultRisingDials())

	for _, bad := range []struct {
		field string
		value interface{}
	}{{"p", 1.5}, {"p", -0.1}, {"hasty_weight", 2.0}, {"p", "x"}, {"nope", 1.0}} {
		if err := r.HarnessSet(bad.field, bad.value); err == nil {
			t.Errorf("%s=%v accepted", bad.field, bad.value)
		}
	}

	if err := r.HarnessSet("p", 1.0); err != nil || r.Chance() != 1 {
		t.Fatalf("p=1: %v %.2f", err, r.Chance())
	}
}

// Bury and Close and Rise move the open count exactly once each, and refuse
// what their state does not allow.
func TestCorpsesStep2Transitions(t *testing.T) {
	open := 0
	c := NewCorpses(nil, func(d int) { open += d })

	c.FallHuman("a", 0, 0)
	c.FallHuman("b", 0, 0)

	if !c.Bury("a") || c.Bury("a") || open != 1 {
		t.Fatalf("bury: open %d", open)
	}

	if !c.Close("a") || open != 1 {
		t.Fatalf("staking a grave closes it without touching the open count: %d", open)
	}

	if c.Rise("a") || c.Bury("a") {
		t.Fatal("a closed body neither rises nor is buried")
	}

	if !c.Rise("b") || c.Rise("b") || open != 0 || c.Close("b") {
		t.Fatalf("rise: open %d", open)
	}

	if g, ok := c.Get("b"); !ok || g.State != CorpseRisen {
		t.Fatalf("get reads the state now: %v %v", g, ok)
	}

	if _, ok := c.Get("nobody"); ok {
		t.Fatal("get: no such body")
	}
}
