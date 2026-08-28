package d2mapgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Act 1 town layout used to be drawn blind, and a draw that landed on a
// layout with no wilderness generator produced a village in a void -- so
// whether a world existed outside the palisade was a property of the seed.
// These pin the choosing, which is the half that can be tested without MPQs.

// realTownFiles is the shape the preset table has: several layouts, only some
// of which this generator can build a world around.
var realTownFiles = []string{"townE1.ds1", "townS1.ds1", "townW1.ds1", "townN1.ds1"}

// usableTownCount is how many of those the generator can build a world around.
const usableTownCount = 3

func TestUsableTownIndexNeverPicksALayoutWithNoWilderness(t *testing.T) {
	// Whatever the chooser returns, the answer must never be the north layout.
	//
	// The bound is usableTownCount and NOT len(realTownFiles), because the
	// chooser is called with the count of usable names -- which is the
	// invariant documented on usableTownIndex, and which the first draft of
	// this loop promptly violated and panicked on. usableTownIndex is left
	// strict rather than clamping: a chooser out of contract is a caller bug,
	// and clamping would have hidden exactly the mistake this comment records.
	for pick := 0; pick < usableTownCount; pick++ {
		got := usableTownIndex(realTownFiles, townPresetsWithWilderness, func(int) int { return pick })

		require.NotEqual(t, autoFileIndex, got, "chooser %d fell through to a free draw", pick)
		require.Less(t, got, len(realTownFiles))
		assert.NotEqual(t, "townN1.ds1", realTownFiles[got],
			"chooser %d picked the layout with no wilderness generator", pick)
	}
}

// The chooser is called with the count of USABLE names and its answer indexes
// the usable list. Getting that wrong would index the wrong list and quietly
// pick the layout this function exists to avoid.
func TestUsableTownIndexAsksTheChooserForTheUsableCount(t *testing.T) {
	asked := -1

	usableTownIndex(realTownFiles, townPresetsWithWilderness, func(n int) int {
		asked = n
		return 0
	})

	assert.Equal(t, usableTownCount, asked, "three of the four layouts have a wilderness generator")
}

// Every reachable answer must actually match, not merely be in range.
func TestUsableTownIndexAnswersAlwaysMatch(t *testing.T) {
	for pick := 0; pick < usableTownCount; pick++ {
		got := usableTownIndex(realTownFiles, townPresetsWithWilderness, func(int) int { return pick })

		name := realTownFiles[got]
		matched := false

		for _, want := range townPresetsWithWilderness {
			if len(want) > 0 && containsFold(name, want) {
				matched = true
			}
		}

		assert.True(t, matched, "index %d is %q, which matches nothing in %v",
			got, name, townPresetsWithWilderness)
	}
}

// With nothing usable the caller must fall back to LoadStamp's own free draw
// rather than returning a wrong index -- degrade to today's behaviour, not to
// a panic and not to a silently bad map.
func TestUsableTownIndexFallsBackWhenNothingMatches(t *testing.T) {
	assert.Equal(t, autoFileIndex,
		usableTownIndex([]string{"townN1.ds1"}, townPresetsWithWilderness, func(int) int { return 0 }))
	assert.Equal(t, autoFileIndex,
		usableTownIndex(nil, townPresetsWithWilderness, func(int) int { return 0 }))
	assert.Equal(t, autoFileIndex,
		usableTownIndex(realTownFiles, nil, func(int) int { return 0 }))
}

// A layout matching two wanted substrings must be offered once, not twice --
// otherwise it would be over-weighted in the draw.
func TestUsableTownIndexCountsEachLayoutOnce(t *testing.T) {
	asked := -1

	usableTownIndex([]string{"townE1S1.ds1", "townN1.ds1"}, townPresetsWithWilderness,
		func(n int) int {
			asked = n
			return 0
		})

	assert.Equal(t, 1, asked, "one layout matches, however many substrings it contains")
}

func containsFold(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}

		return false
	})()
}
