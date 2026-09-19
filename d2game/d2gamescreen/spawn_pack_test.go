package d2gamescreen

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BUG-12: a pack has to arrive as a pack.
//
// These assert the arithmetic of packSpots without a map, an asset manager or
// ebiten, which is the whole reason it is a pure function. What they pin is the
// property the design depends on and the old placement destroyed: the members
// of one group stand near EACH OTHER. Before 19 Sep 2026 a pack of four landed
// on four compass points at four different distances, 15 to 30 tiles apart, and
// the measured consequence was that never more than one member of a group was
// ever in a fight -- so morale, rout and quick-resolve could not fire at all.
func TestPackSpotsPutAPackTogether(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 6} {
		spots := packSpots(100, 100, 8, 16, count, 3)
		require.Len(t, spots, count)

		// THE ASSERTION: every member is within a pack's width of every other.
		// 2 * packSpread is the diameter of the ring they stand on.
		for i := range spots {
			for j := range spots {
				d := math.Hypot(spots[i][0]-spots[j][0], spots[i][1]-spots[j][1])
				assert.LessOrEqual(t, d, 2*packSpread+1e-9,
					"pack of %d: members %d and %d are %.2f tiles apart", count, i, j, d)
			}
		}

		// And the whole knot sits inside the row's band, give or take its own
		// width -- a pack that arrives on top of the player is not an arrival.
		for i, s := range spots {
			d := math.Hypot(s[0]-100, s[1]-100)
			assert.GreaterOrEqual(t, d, 8-packSpread-1e-9, "pack of %d member %d at %.2f tiles", count, i, d)
			assert.LessOrEqual(t, d, 16+packSpread+1e-9, "pack of %d member %d at %.2f tiles", count, i, d)
		}
	}
}

// The negative control in test form: the OLD scheme fails the assertion above,
// so the test is not vacuous. This reproduces it rather than describing it,
// because "a pack of four was 15 to 30 tiles apart" is the kind of claim that
// should be checkable by the person reading it.
func TestTheOldPlacementScatteredAPack(t *testing.T) {
	const (
		count            = 4
		minTiles, maxTil = 8.0, 16.0
	)

	bearing := 3 * spawnBearingStep

	old := make([][2]float64, 0, count)

	for i := 0; i < count; i++ {
		angle := bearing + 2*math.Pi*float64(i)/float64(count)
		reach := minTiles + (maxTil-minTiles)*float64(i)/float64(count-1)
		old = append(old, [2]float64{100 + reach*math.Cos(angle), 100 + reach*math.Sin(angle)})
	}

	worst := 0.0

	for i := range old {
		for j := range old {
			if d := math.Hypot(old[i][0]-old[j][0], old[i][1]-old[j][1]); d > worst {
				worst = d
			}
		}
	}

	assert.Greater(t, worst, 15.0,
		"the old scheme put two members of one pack at least 15 tiles apart -- this is the bug, "+
			"kept as a control so the fix above cannot pass vacuously (measured %.1f)", worst)
}

// Successive arrivals must still differ in BOTH direction and distance, which
// is what spawnBearingStep and spawnRadialStep are for. Without the radial half
// every pack of the night arrives at exactly MinTiles.
func TestPackSpotsWalkDirectionAndDistancePerArrival(t *testing.T) {
	type polar struct{ bearing, reach float64 }

	seen := make([]polar, 0, 8)

	for arrival := 1; arrival <= 8; arrival++ {
		anchor := packSpots(100, 100, 8, 16, 3, arrival)[0]
		dx, dy := anchor[0]-100, anchor[1]-100
		seen = append(seen, polar{math.Atan2(dy, dx), math.Hypot(dx, dy)})
	}

	for i := range seen {
		for j := i + 1; j < len(seen); j++ {
			assert.Greater(t, math.Abs(seen[i].bearing-seen[j].bearing), 0.05,
				"arrivals %d and %d come from the same direction", i+1, j+1)
			assert.Greater(t, math.Abs(seen[i].reach-seen[j].reach), 0.05,
				"arrivals %d and %d arrive at the same distance", i+1, j+1)
		}
	}

	// Deterministic: the same arrival number is the same place, every launch.
	assert.Equal(t, packSpots(100, 100, 8, 16, 4, 5), packSpots(100, 100, 8, 16, 4, 5))
}
