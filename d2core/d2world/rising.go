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
	// DownedMinutes is a Downed man's window at night: R2 §3's [3] rounds at
	// the combat dials' 1 world minute a round. Staked inside it he stays
	// down; not, and he stands again, "possibly while the fight still runs".
	DownedMinutes float64
	// EdgeFloor is how many nameless dead stand up at the edge of the night
	// in band EdgeBand, whatever lies open or closed (S1 §6.3: "plus an
	// edge-arrival floor of wandering dead even when every body is Closed").
	EdgeFloor int
	EdgeBand  int
}

// DefaultRisingDials are the M4.7 note's step-2 numbers.
func DefaultRisingDials() RisingDials {
	return RisingDials{P: 0.3, HastyWeight: 0.25, Pressure: 0, PerOpenAtDawn: 0.02, PerRite: 0.01, DownedMinutes: 3,
		EdgeFloor: 1, EdgeBand: 2}
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
	// now is world minutes, for a Downed man's window (step 3b).
	now        func() float64
	stoodAgain int

	// wander stands a nameless dead man up at the edge (step 5) and says who
	// and where; nil in tests that do not want him.
	wander   func() (string, float64, float64)
	wandered int
}

// SetWander attaches what stands the edge floor's wanderers up.
func (r *Rising) SetWander(fn func() (string, float64, float64)) { r.wander = fn }

// SetClock attaches the world minutes a Downed man's window is measured on.
func (r *Rising) SetClock(now func() float64) { r.now = now }

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
			r.roll(b)
		}

		r.standTheDowned()
	case r.lastBand >= 0:
		for b := r.lastBand + 1; b < risingBands; b++ {
			r.roll(b)
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
func (r *Rising) roll(band int) {
	r.rolls++

	if band == r.dials.EdgeBand {
		defer r.wanderers()
	}

	p := r.Chance()

	for _, b := range r.corpses.All() {
		if !b.Door() {
			continue
		}

		odds := p

		switch b.State {
		case CorpseHasty:
			odds *= r.dials.HastyWeight
		case CorpseDowned:
			// Q7a: a Downed man stands certainly in the next deep-night band
			// -- once his window is up. One cut down a minute before a band
			// turns keeps the rest of his window (the step-3b review);
			// standTheDowned stands him when it runs out.
			odds = 1
			if !r.windowUp(b) {
				odds = 0
			}
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

// wanderers is the edge floor: EdgeFloor nameless dead stand up at the edge
// of the night. Each is given a body where he stood up, so everything the
// machine does with the dead -- Downed, laid down at first light, staked --
// it does with him. After the band's roll, so the roll never sees him.
func (r *Rising) wanderers() {
	for i := 0; i < r.dials.EdgeFloor && r.wander != nil; i++ {
		member, x, y := r.wander()
		if member == "" {
			continue
		}

		r.wandered++
		id := fmt.Sprintf("wanderer:%d", r.wandered)

		r.corpses.FallHuman(id, x, y)

		if r.corpses.Rise(id) {
			r.corpses.Raised(id, member)
		}
	}
}

// windowUp reports a Downed man's window run out (always, with no clock).
func (r *Rising) windowUp(b Corpse) bool {
	return r.now == nil || r.now()-b.DownedAt >= r.dials.DownedMinutes
}

// standTheDowned is the window running out at night: each Downed man whose
// minutes are up stands again where he lies (S1 §6.2). Not a roll -- no draw.
func (r *Rising) standTheDowned() {
	if r.now == nil {
		return
	}

	for _, b := range r.corpses.All() {
		if b.State != CorpseDowned || !r.windowUp(b) {
			continue
		}

		member := ""
		if r.raise != nil {
			if member = r.raise(b); member == "" {
				continue
			}
		}

		if r.corpses.Rise(b.ID) {
			r.stoodAgain++
			r.corpses.Raised(b.ID, member)
		}
	}
}

func (r *Rising) dawn() {
	for _, b := range r.corpses.All() {
		// A Downed man is an unrited body too.
		if b.Class == CorpseHuman && (b.State == CorpseFresh || b.State == CorpseDowned) {
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
		"stood_again": r.stoodAgain, "downed_minutes": r.dials.DownedMinutes,
		"edge_floor": r.dials.EdgeFloor, "wandered": r.wandered,
	}
}

// HarnessSettableFields are the odds, so a script can make the night certain
// or empty.
func (r *Rising) HarnessSettableFields() []string {
	return []string{"edge_floor", "hasty_weight", "p", "pressure"}
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
	case "edge_floor":
		if f < 0 {
			return fmt.Errorf("edge_floor cannot be negative, got %v", f)
		}

		r.dials.EdgeFloor = int(f)
	default:
		return fmt.Errorf("rising has no settable field %q", field)
	}

	return nil
}
