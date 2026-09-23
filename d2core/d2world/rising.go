package d2world

import (
	"fmt"
	"math/rand"
)

// M4.7 step 2 (23 Sep 2026): the rising roll. S1 §6.3: each open body of a
// man rolls once per deep-night band against soul pressure; a hasty grave
// rolls at a reduced weight; a closed body never rises. Nothing rolls at dusk
// or dawn -- the bands are the deep night's, computed by the spawn tables
// (M4.3b ask 4), and the rising reads them rather than keeping its own.
//
// Soul pressure is a v0 constant plus local deltas (S1 §12 marker, 12 Sep): a
// little more for every man's body left open at dawn, a little less for every
// rite. It is NEVER displayed (S1 §3.4); only the harness reads it.
//
// What a risen body DOES is step 3's. In step 2 it leaves where it lay, and
// the morning finds the place empty.

// RisingDials are the rising's numbers. Every one is a [DIAL].
type RisingDials struct {
	// P is the chance an open body of a man rises in one deep-night band.
	P float64
	// HastyWeight multiplies P for a body in a hasty grave.
	HastyWeight float64
	// Pressure is soul pressure's v0 constant; it adds to P.
	Pressure float64
	// PerOpenAtDawn is added to pressure for each man's body open at dawn.
	PerOpenAtDawn float64
	// PerRite is taken off pressure for each rite (a stake is one).
	PerRite float64
}

// DefaultRisingDials are the M4.7 note's step-2 numbers.
func DefaultRisingDials() RisingDials {
	return RisingDials{P: 0.3, HastyWeight: 0.25, Pressure: 0, PerOpenAtDawn: 0.02, PerRite: 0.01}
}

// Rising rolls the doors once per band.
type Rising struct {
	dials   RisingDials
	corpses *Corpses
	band    func() int
	stage   func() Stage
	rng     *rand.Rand

	pressure  float64
	lastBand  int
	lastStage Stage
	rolls     int
	risen     int

	// raise stands a body up in the world (step 3) and names the member it
	// walks as; nil in step 2's tests, where a risen body simply leaves. An
	// empty answer keeps it lying.
	raise func(Corpse) string
	// firstLight hears the night end (step 3): the dead break off.
	firstLight func()
}

// SetRaise attaches what stands a body up in the world.
func (r *Rising) SetRaise(raise func(Corpse) string) { r.raise = raise }

// SetFirstLight attaches what hears the night end.
func (r *Rising) SetFirstLight(fn func()) { r.firstLight = fn }

// NewRising builds the roll. band is the deep-night band (0..bands-1, -1 out
// of the night) and stage the clock's stage; both are sampled NOW, so the
// band a session opens in is not rolled for a second time (T3's lesson: seed
// the last-seen value from the clock, never from zero).
func NewRising(corpses *Corpses, band func() int, stage func() Stage, seed int64, dials RisingDials) *Rising {
	r := &Rising{
		dials:    dials,
		corpses:  corpses,
		band:     band,
		stage:    stage,
		rng:      rand.New(rand.NewSource(seed)), // nolint:gosec // gameplay RNG, seeded for reproducibility
		pressure: dials.Pressure,
	}

	r.lastBand, r.lastStage = band(), stage()

	return r
}

// risingBands is how many deep-night bands there are (S1 §4's [3]).
var risingBands = len(SpawnRow{}.BandWeight)

// Advance is called every frame. Entering a band rolls it; a jump over bands
// (a long sleep, a harness step) rolls every band it crossed; leaving the
// night rolls what was left of it, then counts the open bodies at dawn.
func (r *Rising) Advance() {
	band, stage := r.band(), r.stage()

	switch {
	case band >= 0:
		for b := r.lastBand + 1; b <= band; b++ {
			r.roll()
		}
	case r.lastBand >= 0:
		for b := r.lastBand + 1; b < risingBands; b++ {
			r.roll()
		}
	}

	if r.lastStage == StageNight && stage != StageNight {
		r.dawn()

		// First light after the count: a risen man laid down now is not a
		// body "left open at dawn" by him (M4.7 note step 3 -> Q7).
		if r.firstLight != nil {
			r.firstLight()
		}
	}

	r.lastBand, r.lastStage = band, stage
}

// Chance is the roll's odds for an open body now: P plus soul pressure, 0..1.
func (r *Rising) Chance() float64 { return clamp01(r.dials.P + r.pressure) }

// roll is one band. Every door draws, in the order the bodies fell, whether
// or not an earlier one rose, so a run at one seed is the same every time.
func (r *Rising) roll() {
	r.rolls++

	p := r.Chance()

	for _, b := range r.corpses.All() {
		if !b.Door() {
			continue
		}

		odds := p
		if b.State == CorpseHasty {
			odds *= r.dials.HastyWeight
		}

		if r.rng.Float64() >= odds {
			continue
		}

		// Stood up in the world first: a body with nowhere to stand (no
		// walkable tile, no stand-in) stays where it lies.
		member := ""
		if r.raise != nil {
			if member = r.raise(b); member == "" {
				continue
			}
		}

		if r.corpses.Rise(b.ID) {
			r.risen++
			r.corpses.Raised(b.ID, member)
		}
	}
}

func (r *Rising) dawn() {
	for _, b := range r.corpses.All() {
		if b.Class == CorpseHuman && b.State == CorpseFresh {
			r.pressure += r.dials.PerOpenAtDawn
		}
	}
}

// Rite lowers soul pressure: a body closed by the stake or the priest.
func (r *Rising) Rite() { r.pressure -= r.dials.PerRite }

// Pressure is soul pressure now. Never shown to the player.
func (r *Rising) Pressure() float64 { return r.pressure }

// HarnessName is the "rising" system.
func (r *Rising) HarnessName() string { return "rising" }

// HarnessState reports the roll.
func (r *Rising) HarnessState() map[string]interface{} {
	return map[string]interface{}{
		"p": r.dials.P, "hasty_weight": r.dials.HastyWeight, "pressure": r.pressure,
		"chance": r.Chance(), "band": r.lastBand, "rolls": r.rolls, "risen": r.risen,
	}
}

// HarnessSettableFields are the odds, so a script can make the night certain
// or empty.
func (r *Rising) HarnessSettableFields() []string {
	return []string{"hasty_weight", "p", "pressure"}
}

// HarnessSet writes one of them.
func (r *Rising) HarnessSet(field string, value interface{}) error {
	f, ok := toFloat(value)
	if !ok {
		return fmt.Errorf("%s wants a number, got %T", field, value)
	}

	switch field {
	case "p", "hasty_weight":
		if f < 0 || f > 1 {
			return fmt.Errorf("%s is a chance, 0..1, got %v", field, f)
		}

		if field == "p" {
			r.dials.P = f
		} else {
			r.dials.HastyWeight = f
		}
	case "pressure":
		r.pressure = f
	default:
		return fmt.Errorf("rising has no settable field %q", field)
	}

	return nil
}
