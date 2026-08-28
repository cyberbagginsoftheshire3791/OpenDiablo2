package d2mapengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2ds1"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2geom"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// These cover the three accessor bugs M4.3a fixes before replacing the
// pathfinder, plus the one consequence in checkLos. The grid is built by hand
// -- no AssetManager, no MPQs, no ebiten -- which is why these were worth
// fixing here and the addDT1 missing return was not.

// testEngine builds a bare w*h map with no tile content.
func testEngine(w, h int) *MapEngine {
	m := &MapEngine{}
	m.size = d2geom.Size{Width: w, Height: h}
	m.tiles = make([]MapTile, w*h)

	return m
}

func TestTileCoordinateToIndexRefusesOffMapCoordinates(t *testing.T) {
	m := testEngine(10, 10)

	assert.Equal(t, 0, m.tileCoordinateToIndex(0, 0), "origin")
	assert.Equal(t, 54, m.tileCoordinateToIndex(4, 5), "an ordinary tile")
	assert.Equal(t, 99, m.tileCoordinateToIndex(9, 9), "the last tile")

	// The bug: x was never bounded, so these wrapped onto real tiles on the
	// neighbouring row -- (-1, 5) gave 49 and (10, 5) gave 60, both valid
	// indexes into a coordinate that is not on the map.
	assert.Equal(t, -1, m.tileCoordinateToIndex(-1, 5), "one step off the left edge")
	assert.Equal(t, -1, m.tileCoordinateToIndex(10, 5), "one step off the right edge")
	assert.Equal(t, -1, m.tileCoordinateToIndex(0, -1), "above the map")
	assert.Equal(t, -1, m.tileCoordinateToIndex(0, 10), "below the map")
}

func TestTileAtRefusesTheWrappedCoordinate(t *testing.T) {
	m := testEngine(10, 10)

	// TileAt guarded the index but not the coordinate, so it returned the
	// wrapped tile rather than nil. Same tile, reached two ways, is the shape
	// of the bug: prove they are no longer the same.
	assert.NotNil(t, m.TileAt(9, 4), "the tile (-1, 5) used to alias onto")
	assert.Nil(t, m.TileAt(-1, 5), "one step off the left edge is not a tile")
	assert.Nil(t, m.TileAt(10, 5), "one step off the right edge is not a tile")
}

func TestTileExistsAtExactlyTheTileCount(t *testing.T) {
	m := testEngine(4, 4)

	// The bound was `tileIndex <= len(m.tiles)`, so a coordinate resolving to
	// index 16 on a 16-tile map indexed one past the end. (0, 4) is that
	// coordinate. It must answer false, and it must not panic.
	require.NotPanics(t, func() { m.TileExists(0, 4) })
	assert.False(t, m.TileExists(0, 4), "the row past the last one holds no tile")
	assert.False(t, m.TileExists(-1, 0), "off the left edge holds no tile")

	// An empty in-range tile has no features, so it does not "exist" either.
	assert.False(t, m.TileExists(1, 1), "an in-range tile with no features")

	// One with a floor does.
	m.tiles[m.tileCoordinateToIndex(1, 1)].Components.Floors = make([]d2ds1.Tile, 1)
	assert.True(t, m.TileExists(1, 1), "an in-range tile carrying a floor")
}

func TestFloorDivModKeepsNegativesNegative(t *testing.T) {
	cases := []struct {
		a, b, wantQ, wantR int
	}{
		{0, 5, 0, 0},
		{4, 5, 0, 4},
		{5, 5, 1, 0},
		{-1, 5, -1, 4},
		{-5, 5, -1, 0},
		{-6, 5, -2, 4},
	}

	for _, c := range cases {
		q, r := floorDivMod(c.a, c.b)
		assert.Equal(t, c.wantQ, q, "quotient of %d/%d", c.a, c.b)
		assert.Equal(t, c.wantR, r, "remainder of %d%%%d", c.a, c.b)
		assert.GreaterOrEqual(t, r, 0, "the remainder is never negative")
	}
}

func TestSubTileAtOffTheMapReturnsNil(t *testing.T) {
	m := testEngine(4, 4) // 20x20 subtiles

	// In range: the flags come back and are the tile's own.
	flags := m.SubTileAt(7, 7)
	require.NotNil(t, flags, "a subtile inside the map")

	flags.BlockWalk = true
	assert.True(t, m.SubTileAt(7, 7).BlockWalk, "the same subtile, read twice")
	assert.False(t, m.SubTileAt(8, 7).BlockWalk, "its neighbour is untouched")

	// Off the map, two ways, neither of which may panic. subX = -1 used to
	// resolve to tile 0 with a subtile offset of -1 and index the lookup table
	// out of range; subX = 20 used to reach TileAt(4, ...) and dereference nil.
	require.NotPanics(t, func() { m.SubTileAt(-1, 7) })
	require.NotPanics(t, func() { m.SubTileAt(20, 7) })
	require.NotPanics(t, func() { m.SubTileAt(7, -1) })
	require.NotPanics(t, func() { m.SubTileAt(7, 20) })

	assert.Nil(t, m.SubTileAt(-1, 7), "one subtile off the left edge")
	assert.Nil(t, m.SubTileAt(20, 7), "one subtile off the right edge")
	assert.Nil(t, m.SubTileAt(7, -1), "one subtile above the map")
	assert.Nil(t, m.SubTileAt(7, 20), "one subtile below the map")
}

func TestCheckLosStopsAtTheMapEdgeInsteadOfPanicking(t *testing.T) {
	m := testEngine(4, 4) // subtiles 0..19 in both axes

	// Walking east off the map. checkLos does no bounds checking of its own,
	// so before SubTileAt was guarded this crashed the process -- which is
	// what "an off-map move target can panic today" meant.
	start := d2vector.NewPosition(10, 10)
	dest := d2vector.NewPosition(40, 10)

	var (
		clear bool
		stop  d2vector.Position
	)

	require.NotPanics(t, func() { clear, stop = m.checkLos(start, dest) })
	assert.False(t, clear, "the map edge blocks line of sight")
	assert.LessOrEqual(t, stop.X(), float64(20), "it stops on or before the edge")

	// And west, where the old truncating division misbehaved instead.
	require.NotPanics(t, func() {
		clear, stop = m.checkLos(d2vector.NewPosition(10, 10), d2vector.NewPosition(-20, 10))
	})
	assert.False(t, clear, "the map edge blocks line of sight westward too")
	assert.GreaterOrEqual(t, stop.X(), float64(-1), "it stops at the edge, not beyond it")

	// A walk that stays on the map still reports clear.
	clear, _ = m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(17, 17))
	assert.True(t, clear, "an unobstructed walk inside the map is clear")
}
