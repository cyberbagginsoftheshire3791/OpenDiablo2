package d2mapengine

import (
	"fmt"
	"math/rand"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// The world RNG (P3 spec E4): one seeded generator per map engine that every
// SIMULATION consumer draws from — map generation, stamp selection, and NPC
// behaviour seeding — replacing the global math/rand, whose top-level Seed
// has been a no-op since Go 1.24 (the map seed never actually reached the
// overworld, and the server's and client's wilderness could diverge).
// Presentation randomness (audio variants, object start frames) deliberately
// stays on the global generator so it cannot shift a simulation roll.
//
// Its source counts draws; the count is part of the harness's state digest,
// so a stray consumer shows up as a digest mismatch with a name. The counting
// source lived here until M4.6 B1 moved it to d2common/d2rand, where the
// spawn tables, combat and the rising share it and where Restore puts a
// stream back where a saved game left it. The count is the same count: Int63
// and Uint64 are one draw each, as they always were here.

// initRand (re)builds the world RNG from the given seed. Called by SetSeed and
// ReseedRand.
func (m *MapEngine) initRand(seed int64) {
	m.useRand(d2rand.NewSource(seed))
}

// useRand installs a counted source as the world RNG and hands the RNG to the
// embedded stamp and entity factories.
func (m *MapEngine) useRand(src *d2rand.Source) {
	m.randSource = src
	m.rand = rand.New(src) // nolint:gosec // simulation RNG, seeded for reproducibility

	if m.StampFactory != nil {
		m.StampFactory.SetRand(m.rand)
	}

	if m.MapEntityFactory != nil {
		m.MapEntityFactory.SetRand(m.rand)
	}
}

// Rand returns the world RNG, seeding it from the engine seed on first use.
func (m *MapEngine) Rand() *rand.Rand {
	if m.rand == nil {
		m.initRand(m.seed)
	}

	return m.rand
}

// ReseedRand reseeds the world RNG mid-game without regenerating the map
// (the harness's reseed_world tool uses it for repeated-roll tests).
func (m *MapEngine) ReseedRand(seed int64) {
	m.initRand(seed)
}

// RestoreRand puts the world RNG back where a saved game left it: seeded with
// seed and advanced past its first draws values, so the next value drawn is
// the one the saved game would have drawn next (M4.6). seed and draws are
// what RandSeed and RandDraws reported when the game was saved. Like
// ReseedRand it leaves the map alone.
//
// NOTHING CALLS IT YET. It is B1's half of the world save; the load that
// calls it is burst B4, and it must run AFTER the entities are rebuilt,
// because rebuilding them draws from this stream (the plan's trap 6:
// NewCreature and NewNPC draw a behaviour seed and roll equipment from it).
//
// It refuses a draw count past d2rand.MaxDraws, and changes nothing when it
// does (the B2a review's B1: a count is replayed a step at a time, so it is
// bounded rather than trusted). The seed needs no check here: the world
// stream runs on the game seed, and the load hands the server that seed
// (SetNextGameSeed) from the same file; B4 checks the two agree with
// d2rand.StreamState.Check(seed, d2rand.StreamWorld).
func (m *MapEngine) RestoreRand(seed int64, draws uint64) error {
	if draws > d2rand.MaxDraws {
		return fmt.Errorf("%w: the world stream is saved at %d draws, past the %d a save may claim",
			d2rand.ErrStreamState, draws, d2rand.MaxDraws)
	}

	m.useRand(d2rand.Restore(seed, draws))

	return nil
}

// RandDraws returns how many values have been drawn from the world RNG since
// it was last seeded. Part of the determinism digest.
func (m *MapEngine) RandDraws() uint64 {
	if m.randSource == nil {
		return 0
	}

	return m.randSource.Draws()
}

// RandSeed returns the seed the world RNG was last seeded with -- the engine
// seed, unless ReseedRand or RestoreRand chose another. Before the RNG's first
// use it is the engine seed, which is what first use will seed it with.
func (m *MapEngine) RandSeed() int64 {
	if m.randSource == nil {
		return m.seed
	}

	return m.randSource.Seeded()
}
