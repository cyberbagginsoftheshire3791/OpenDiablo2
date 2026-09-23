package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S1 §6.3, the edge-arrival floor (M4.7 step 5): in the third deep-night band
// one nameless dead man stands up at the edge of the night whatever lies open
// or closed -- even at P 0 with nothing to roll -- and he is given a body, so
// the machine treats him like any risen man. Not in the first two bands.
func TestEdgeFloor(t *testing.T) {
	dials := DefaultRisingDials()
	dials.P = 0

	r, corpses, night := newTestRising(t, dials)

	n := 0

	r.SetWander(func() (string, float64, float64) {
		n++

		return "w:" + itoa(n), 30, 31
	})

	for b := 0; b < dials.EdgeBand; b++ {
		night.set(b, StageNight)
		r.Advance()
	}

	assert.Zero(t, r.wandered, "not before the third band")

	night.set(dials.EdgeBand, StageNight)
	r.Advance()

	require.Equal(t, 1, r.wandered)

	b, ok := corpses.Get("wanderer:1")
	require.True(t, ok, "he has a body")
	assert.Equal(t, CorpseRisen, b.State)
	assert.Equal(t, CorpseHuman, b.Class)
	assert.Equal(t, [2]float64{30, 31}, [2]float64{b.X, b.Y})

	// He falls like any risen man: Downed, the same body.
	corpses.Fall("w:1", RisenRow, 32, 33)

	b, _ = corpses.Get("wanderer:1")
	assert.Equal(t, CorpseDowned, b.State)
}

// The control: an edge floor of 0 stands no one up.
func TestEdgeFloorZero(t *testing.T) {
	dials := DefaultRisingDials()
	dials.EdgeFloor = 0

	r, _, night := newTestRising(t, dials)
	r.SetWander(func() (string, float64, float64) { return "w:1", 0, 0 })

	night.set(dials.EdgeBand, StageNight)
	r.Advance()

	assert.Zero(t, r.wandered)
}

// A wanderer stands in the risen row's ring around the target, not on it.
func TestRaiseWandererRing(t *testing.T) {
	s, _, _, spawner, _ := newTestSpawns(t)

	id, _, _ := s.RaiseWanderer()
	require.NotEmpty(t, id)

	row, _ := s.rowNamed(RisenRow)
	assert.Equal(t, row.MinTiles, spawner.lastMin)
	assert.Equal(t, row.MaxTiles, spawner.lastMax)
	assert.Positive(t, spawner.lastMin, "at the edge, not at his feet")

	p, ok := s.ProfileOf(id)
	require.True(t, ok)
	assert.True(t, p.Dead)
}
