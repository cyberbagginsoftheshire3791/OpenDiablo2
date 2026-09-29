package d2mapengine

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapstamp"
)

// worldDraws takes k values from the engine's world RNG, through the two
// method paths its consumers use (the entity factory's Int63 and Intn, the
// stamp factory's Float64).
func worldDraws(m *MapEngine, k int) []int64 {
	out := make([]int64, 0, k)

	for i := 0; i < k; i++ {
		switch i % 3 {
		case 0:
			out = append(out, m.Rand().Int63())
		case 1:
			out = append(out, int64(m.Rand().Intn(1000)))
		default:
			out = append(out, int64(m.Rand().Float64()*1e9))
		}
	}

	return out
}

// TestRestoreRandResumesTheWorldStream is M4.6's contract on the world RNG:
// a game saved at (seed, draws) and restored there draws what the unbroken
// game would have drawn next. Restoring one draw short is the negative
// control that proves the count is exact rather than close.
func TestRestoreRandResumesTheWorldStream(t *testing.T) {
	a := &MapEngine{}
	a.SetSeed(1462)

	worldDraws(a, 37)

	seed, draws := a.RandSeed(), a.RandDraws()
	require.Equal(t, int64(1462), seed)
	require.NotZero(t, draws)

	want := worldDraws(a, 24)

	b := &MapEngine{}
	b.SetSeed(99) // a different world: RestoreRand must replace its stream wholesale
	require.NoError(t, b.RestoreRand(seed, draws))

	assert.Equal(t, seed, b.RandSeed())
	assert.Equal(t, draws, b.RandDraws(), "a restored stream reports the count it was restored at")
	assert.Equal(t, want, worldDraws(b, 24), "restore then draw is the stream that kept going")
	assert.Equal(t, a.RandDraws(), b.RandDraws(), "and counts the same draws from there")

	short := &MapEngine{}
	require.NoError(t, short.RestoreRand(seed, draws-1))
	assert.NotEqual(t, want, worldDraws(short, 24), "one draw short must not reproduce the stream")
}

// TestRandSeedFollowsTheStream: a save writes RandSeed, so it must name the
// stream actually running -- the engine seed until ReseedRand picks another.
func TestRandSeedFollowsTheStream(t *testing.T) {
	m := &MapEngine{}
	m.SetSeed(1462)
	assert.Equal(t, int64(1462), m.RandSeed())

	m.ReseedRand(7)
	assert.Equal(t, int64(7), m.RandSeed(), "reseed_world moves the stream's seed, not the map's")
	assert.Equal(t, int64(1462), m.Seed(), "the map seed is untouched")

	fresh := &MapEngine{}
	fresh.seed = 55
	assert.Equal(t, int64(55), fresh.RandSeed(), "before first use it is the seed first use will take")
	assert.Zero(t, fresh.RandDraws())
}

// withFactories is an engine as the game builds one: with the entity and stamp
// factories it hands the world RNG to. Zero-value factories are enough; the
// hand-off is a field.
func withFactories() *MapEngine {
	return &MapEngine{
		MapEntityFactory: &d2mapentity.MapEntityFactory{},
		StampFactory:     &d2mapstamp.StampFactory{},
	}
}

// stdlibAfter is the value a PLAIN stdlib stream at seed hands out after draws
// values -- the reference, so a counter and a restore that are wrong together
// cannot agree with themselves.
func stdlibAfter(seed int64, draws uint64) int64 {
	r := rand.New(rand.NewSource(seed)) // nolint:gosec // test

	for i := uint64(0); i < draws; i++ {
		r.Int63()
	}

	return r.Int63()
}

// M4.6 B1 review, C4: after a restore, the FACTORIES draw from the restored
// stream. Every entity the game builds rolls its behaviour seed and equipment
// through the entity factory's copy of the world RNG, and every stamp through
// the stamp factory's; an engine that swapped its own stream but left theirs
// would resume a world whose next wolf rolls from the old, uncounted stream.
func TestRestoreRandHandsTheStreamToTheFactories(t *testing.T) {
	a := withFactories()
	a.SetSeed(1462)
	worldDraws(a, 37)

	seed, draws := a.RandSeed(), a.RandDraws()

	b := withFactories()
	b.SetSeed(99) // the factories now hold the 99 stream
	require.Same(t, b.Rand(), b.MapEntityFactory.WorldRand())

	require.NoError(t, b.RestoreRand(seed, draws))

	require.Same(t, b.Rand(), b.MapEntityFactory.WorldRand(), "the entity factory draws the restored stream")
	require.Same(t, b.Rand(), b.StampFactory.WorldRand(), "and so does the stamp factory")

	// A draw through the factory is the uninterrupted stream's next value, and
	// the engine counts it.
	assert.Equal(t, stdlibAfter(seed, draws), b.MapEntityFactory.WorldRand().Int63(),
		"the entity factory's next roll is the one the saved world would have made")
	assert.Equal(t, draws+1, b.RandDraws(), "a factory's draw is a counted world draw")

	assert.Equal(t, stdlibAfter(seed, draws+1), b.StampFactory.WorldRand().Int63())
	assert.Equal(t, draws+2, b.RandDraws())
}

// A draw count past d2rand.MaxDraws is refused, and the stream running is
// left as it was (the B2a review's B1: a count is replayed a step at a time,
// so it is bounded, not trusted).
func TestRestoreRandRefusesACountPastTheCap(t *testing.T) {
	m := withFactories()
	m.SetSeed(1462)
	worldDraws(m, 5)

	before, draws := m.Rand(), m.RandDraws()

	require.ErrorIs(t, m.RestoreRand(1462, d2rand.MaxDraws+1), d2rand.ErrStreamState)
	require.Same(t, before, m.Rand(), "a refused restore leaves the stream running")
	require.Equal(t, draws, m.RandDraws())
	require.NoError(t, m.RestoreRand(1462, 3), "and a count under the cap still restores")
}
