package d2mapengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	b.RestoreRand(seed, draws)

	assert.Equal(t, seed, b.RandSeed())
	assert.Equal(t, draws, b.RandDraws(), "a restored stream reports the count it was restored at")
	assert.Equal(t, want, worldDraws(b, 24), "restore then draw is the stream that kept going")
	assert.Equal(t, a.RandDraws(), b.RandDraws(), "and counts the same draws from there")

	short := &MapEngine{}
	short.RestoreRand(seed, draws-1)
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
