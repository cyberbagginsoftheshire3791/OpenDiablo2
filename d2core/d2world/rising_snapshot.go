package d2world

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// RisingSnapshot is the rising's saved state (M4.6 B2a): the soul pressure it
// has accrued, which persists -- quitting cannot reset it -- the band and
// stage it last saw, its counters, and where its stream stands.
//
// lastBand and lastStage are what decide whether the next frame rolls a band
// or reads a dawn. NewRising seeds them from the clock, so a load that did not
// restore them would treat the saved band as already rolled or not, and a
// resume that crossed a dawn would pay (or skip) it.
//
// PRESSURE IS SAVED AS ITS ACCRUED PART ONLY (D2, 28 Sep 2026). Soul pressure
// is the dial's v0 constant (RisingDials.Pressure) plus what the dawns and the
// rites added and took; the constant is the build's, re-read from the dials at
// load, so a retuned constant reaches a saved game and an old one does not
// ride along in the file.
//
// The dials -- P, the hasty weight, the edge floor, the pressure constant --
// are NOT saved: they are the game's numbers, and the harness's writes to them
// are test setup. Nor is anything wired by the game (raise, first light, the
// clock, the wanderer and what he was).
type RisingSnapshot struct {
	Accrued    float64            `json:"pressure_accrued"`
	LastBand   int                `json:"last_band"`
	LastStage  string             `json:"last_stage"`
	Rolls      int                `json:"rolls"`
	Risen      int                `json:"risen"`
	StoodAgain int                `json:"stood_again"`
	Wandered   int                `json:"wandered"`
	RNG        d2rand.StreamState `json:"rng"`
}

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
		Accrued: r.accrued, LastBand: r.lastBand, LastStage: r.lastStage.String(),
		Rolls: r.rolls, Risen: r.risen, StoodAgain: r.stoodAgain, Wandered: r.wandered,
		RNG: d2rand.StateOf(r.rng),
	}
}

// Restore puts the saved rising back, stream included. worldSeed is the
// saved game's seed: the stream must be the one that game ran the rising on
// (d2rand.StreamState.Check). It is checked whole first -- against the bodies
// the registry holds NOW, so the corpses are restored before it -- and a
// refused snapshot changes nothing.
//
// Load order (B4): after the corpses, whose bodies the next band rolls, and
// before the first frame -- NewRising sampled the band and stage from the
// clock, and this replaces that sample with the saved one.
//
// wandered is not only a count: the next edge wanderer's body is named
// "wanderer:<wandered+1>", so a rising that forgot it would name a new
// wanderer after one already lying in the registry, and he would never fall.
func (r *Rising) Restore(s RisingSnapshot, worldSeed int64) error {
	bodies := CorpsesSnapshot{}
	if r.corpses != nil {
		bodies = r.corpses.Snapshot()
	}

	if err := r.Validate(s, worldSeed, bodies); err != nil {
		return err
	}

	stage, _ := b2aParseStage(s.LastStage) // Validate parsed it

	r.accrued, r.lastBand, r.lastStage = s.Accrued, s.LastBand, stage
	r.rolls, r.risen, r.stoodAgain, r.wandered = s.Rolls, s.Risen, s.StoodAgain, s.Wandered
	s.RNG.RestoreInto(r.rng)

	return nil
}

// Validate is Restore's check and nothing else (D4). bodies is the corpses
// block the rising will run on: at a load that validates every block before
// restoring any, the FILE's corpses, since the registry is still empty.
//
// It accepts only what Snapshot could have written (the B2a review's B1, B2):
//   - the stream on the seed a game seeded worldSeed runs the rising on, at
//     no more than d2rand.MaxDraws -- so the combat stream's block, swapped
//     in, is refused;
//   - a band only at night: the band is the spawn tables' deep-night band,
//     -1 whenever the stage is not night, and both are sampled together;
//   - wandered at least the number of every wanderer's body the registry
//     holds: each one was named wanderer:<n> when he came, n counting up.
func (r *Rising) Validate(s RisingSnapshot, worldSeed int64, bodies CorpsesSnapshot) error {
	stage, err := b2aParseStage(s.LastStage)
	if err != nil {
		return fmt.Errorf("rising snapshot: %w", err)
	}

	switch {
	case !b2aFinite(s.Accrued):
		return fmt.Errorf("rising snapshot: accrued pressure %v is not a number", s.Accrued)
	case s.LastBand < -1 || s.LastBand >= risingBands:
		return fmt.Errorf("rising snapshot: last band %d; bands run 0..%d, -1 out of the night", s.LastBand, risingBands-1)
	case (s.LastBand >= 0) != (stage == StageNight):
		return fmt.Errorf("rising snapshot: last band %d in stage %s; the bands are the night's, -1 at every other stage",
			s.LastBand, s.LastStage)
	case s.Rolls < 0 || s.Risen < 0 || s.StoodAgain < 0 || s.Wandered < 0:
		return fmt.Errorf("rising snapshot: a negative count (rolls %d, risen %d, stood again %d, wandered %d)",
			s.Rolls, s.Risen, s.StoodAgain, s.Wandered)
	}

	if err := s.RNG.Check(worldSeed, d2rand.StreamRising); err != nil {
		return fmt.Errorf("rising snapshot: %w", err)
	}

	for _, b := range bodies.Bodies {
		var n int
		if _, err := fmt.Sscanf(b.ID, "wanderer:%d", &n); err != nil || fmt.Sprintf("wanderer:%d", n) != b.ID {
			continue
		}

		if n > s.Wandered {
			return fmt.Errorf("rising snapshot: wandered %d, but body %s lies in the registry; the next wanderer would be "+
				"named after a body already there", s.Wandered, b.ID)
		}
	}

	return nil
}
