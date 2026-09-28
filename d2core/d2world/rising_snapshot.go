package d2world

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// RisingSnapshot is the rising's saved state (M4.6 B2a): soul pressure, which
// persists -- quitting cannot reset it -- the band and stage it last saw, its
// counters, and where its stream stands.
//
// lastBand and lastStage are what decide whether the next frame rolls a band
// or reads a dawn. NewRising seeds them from the clock, so a load that did not
// restore them would treat the saved band as already rolled or not, and a
// resume that crossed a dawn would pay (or skip) it.
//
// The dials -- P, the hasty weight, the edge floor -- are NOT saved: they are
// the game's numbers, and the harness's writes to them are test setup. Nor is
// anything wired by the game (raise, first light, the clock, the wanderer and
// what he was).
type RisingSnapshot struct {
	Pressure   float64   `json:"pressure"`
	LastBand   int       `json:"last_band"`
	LastStage  string    `json:"last_stage"`
	Rolls      int       `json:"rolls"`
	Risen      int       `json:"risen"`
	StoodAgain int       `json:"stood_again"`
	Wandered   int       `json:"wandered"`
	RNG        b2aStream `json:"rng"`
}

// b2aStream is a counted stream's position, {seed, draws} (d2rand.Stream).
// The seed is an int64 and a wall-clock seed is past 2^53, where a float64
// reader cannot hold it (B1 notes, section 2), so it is written as a decimal
// string: exact through any JSON reader, typed or not.
//
// Private to B2a's snapshots and named b2a* so that B2b's, built in parallel,
// cannot collide with it; the two can become one type when both are merged.
type b2aStream struct {
	Seed  int64  `json:"seed,string"`
	Draws uint64 `json:"draws"`
}

func b2aStreamOf(st *d2rand.Stream) b2aStream {
	return b2aStream{Seed: st.Seeded(), Draws: st.Draws()}
}

// restore puts st where s says, through Stream.Restore -- which replaces the
// rand and its counted source together -- and nothing else (B1 notes,
// section 3).
func (s b2aStream) restore(st *d2rand.Stream) { st.Restore(s.Seed, s.Draws) }

// b2aParseStage reads a Stage back from its String().
func b2aParseStage(name string) (Stage, error) {
	for _, s := range []Stage{StageNight, StageDawn, StageDay, StageDusk} {
		if s.String() == name {
			return s, nil
		}
	}

	return 0, fmt.Errorf("no stage %q (night, dawn, day, dusk)", name)
}

// Snapshot is the rising as it stands.
func (r *Rising) Snapshot() RisingSnapshot {
	return RisingSnapshot{
		Pressure: r.pressure, LastBand: r.lastBand, LastStage: r.lastStage.String(),
		Rolls: r.rolls, Risen: r.risen, StoodAgain: r.stoodAgain, Wandered: r.wandered,
		RNG: b2aStreamOf(r.rng),
	}
}

// Restore puts the saved rising back, stream included. It is checked whole
// before anything changes.
//
// Load order (B4): after the corpses, whose bodies the next band rolls, and
// before the first frame -- NewRising sampled the band and stage from the
// clock, and this replaces that sample with the saved one.
//
// wandered is not only a count: the next edge wanderer's body is named
// "wanderer:<wandered+1>", so a rising that forgot it would name a new
// wanderer after one already lying in the registry, and he would never fall.
func (r *Rising) Restore(s RisingSnapshot) error {
	stage, err := b2aParseStage(s.LastStage)
	if err != nil {
		return fmt.Errorf("rising snapshot: %w", err)
	}

	switch {
	case !b2aFinite(s.Pressure):
		return fmt.Errorf("rising snapshot: pressure %v is not a number", s.Pressure)
	case s.LastBand < -1 || s.LastBand >= risingBands:
		return fmt.Errorf("rising snapshot: last band %d; bands run 0..%d, -1 out of the night", s.LastBand, risingBands-1)
	case s.Rolls < 0 || s.Risen < 0 || s.StoodAgain < 0 || s.Wandered < 0:
		return fmt.Errorf("rising snapshot: a negative count (rolls %d, risen %d, stood again %d, wandered %d)",
			s.Rolls, s.Risen, s.StoodAgain, s.Wandered)
	}

	r.pressure, r.lastBand, r.lastStage = s.Pressure, s.LastBand, stage
	r.rolls, r.risen, r.stoodAgain, r.wandered = s.Rolls, s.Risen, s.StoodAgain, s.Wandered
	s.RNG.restore(r.rng)

	return nil
}
