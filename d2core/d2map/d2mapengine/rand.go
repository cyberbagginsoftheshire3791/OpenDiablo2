package d2mapengine

import (
	"math/rand"
)

// The world RNG (P3 spec E4): one seeded generator per map engine that every
// SIMULATION consumer draws from — map generation, stamp selection, and NPC
// behaviour seeding — replacing the global math/rand, whose top-level Seed
// has been a no-op since Go 1.24 (the map seed never actually reached the
// overworld, and the server's and client's wilderness could diverge).
// Presentation randomness (audio variants, object start frames) deliberately
// stays on the global generator so it cannot shift a simulation roll.
//
// countingSource wraps the seeded source and counts draws; the count is part
// of the harness's state digest, so a stray consumer shows up as a digest
// mismatch with a name.
type countingSource struct {
	src   rand.Source64
	draws uint64
}

func (c *countingSource) Int63() int64 {
	c.draws++
	return c.src.Int63()
}

func (c *countingSource) Uint64() uint64 {
	c.draws++
	return c.src.Uint64()
}

func (c *countingSource) Seed(seed int64) {
	c.src.Seed(seed)
}

func newCountingSource(seed int64) *countingSource {
	src, ok := rand.NewSource(seed).(rand.Source64)
	if !ok {
		// rand.NewSource's result implements Source64 in every supported Go;
		// this fallback only defends against a future stdlib change.
		src = rand.New(rand.NewSource(seed))
	}

	return &countingSource{src: src}
}

// initRand (re)builds the world RNG from the given seed and hands it to the
// embedded stamp and entity factories. Called by SetSeed and ReseedRand.
func (m *MapEngine) initRand(seed int64) {
	m.randSource = newCountingSource(seed)
	m.rand = rand.New(m.randSource)

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

// RandDraws returns how many values have been drawn from the world RNG since
// it was last seeded. Part of the determinism digest.
func (m *MapEngine) RandDraws() uint64 {
	if m.randSource == nil {
		return 0
	}

	return m.randSource.draws
}
