package d2mapengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// block marks one subtile impassable on a hand-built engine.
func block(m *MapEngine, subX, subY int) {
	flags := m.SubTileAt(subX, subY)
	if flags == nil {
		panic("test blocked a subtile that is not on the map")
	}

	flags.BlockWalk = true
}

// blockColumn walls off a whole column of subtiles, leaving a gap of the given
// height at the bottom. gap <= 0 walls the column off completely.
func blockColumn(m *MapEngine, subX, height, gap int) {
	for y := 0; y < height-gap; y++ {
		block(m, subX, y)
	}
}

func TestPathFindRoutesAroundAWall(t *testing.T) {
	// 8x8 tiles = 40x40 subtiles. A wall down column 20 with a gap at the
	// bottom: the straight line from (10,5) to (30,5) is blocked, and the only
	// way through is south, round, and back north. The raycast this replaced
	// returned a single point short of the wall and stopped there.
	m := testEngine(8, 8)
	blockColumn(m, 20, 40, 4)

	start := d2vector.NewPosition(10, 5)
	dest := d2vector.NewPosition(30, 5)

	// The old behaviour, for contrast: line of sight is blocked.
	clear, _ := m.checkLos(start, dest)
	require.False(t, clear, "the wall blocks the straight line")

	path := m.PathFind(start, dest)
	require.NotEmpty(t, path, "a route around the wall exists and must be found")

	final := path[len(path)-1]
	assert.InDelta(t, 30.0, final.X(), 0.001, "the route ends at the destination")
	assert.InDelta(t, 5.0, final.Y(), 0.001, "the route ends at the destination")

	// It had to go south of the gap to get there, so some waypoint is well
	// below the straight line between start and dest.
	deepest := 0.0
	for _, p := range path {
		if p.Y() > deepest {
			deepest = p.Y()
		}
	}

	assert.Greater(t, deepest, 30.0, "the route detours south through the gap")

	// And it is a handful of corners, not hundreds of subtile steps.
	assert.Less(t, len(path), 12, "collinear runs are collapsed to corners")
}

func TestPathFindIsDeterministicAcrossEngines(t *testing.T) {
	// Two separately built engines with the same obstacles must produce a
	// byte-identical path. This is the assertion that catches an unstable
	// tie-break, which is where A* usually leaks non-determinism -- and every
	// entity position is inside the state digest.
	build := func() *MapEngine {
		m := testEngine(8, 8)
		blockColumn(m, 20, 40, 4)
		block(m, 15, 15)
		block(m, 16, 15)
		block(m, 25, 30)

		return m
	}

	start := d2vector.NewPosition(3, 3)
	dest := d2vector.NewPosition(36, 12)

	first := build().PathFind(start, dest)
	second := build().PathFind(start, dest)

	require.NotEmpty(t, first)
	require.Equal(t, len(first), len(second), "the two paths have the same length")

	for i := range first {
		assert.Equal(t, first[i].X(), second[i].X(), "waypoint %d x", i)
		assert.Equal(t, first[i].Y(), second[i].Y(), "waypoint %d y", i)
	}
}

func TestPathFindReturnsTheBestPartialWhenWalledOff(t *testing.T) {
	// The goal is sealed behind a complete wall. The search cannot reach it,
	// and must still walk as far toward it as it can rather than standing
	// still -- the raycast's failure mode, kept on purpose.
	m := testEngine(8, 8)
	blockColumn(m, 20, 40, 0)

	start := d2vector.NewPosition(5, 5)
	dest := d2vector.NewPosition(35, 5)

	path := m.PathFind(start, dest)
	require.NotEmpty(t, path, "an unreachable goal still yields a partial route")

	final := path[len(path)-1]
	assert.Less(t, final.X(), 20.0, "the route stops on this side of the wall")
	assert.Greater(t, final.X(), 5.0, "but it did move toward the goal")
}

func TestPathFindWillNotCutACorner(t *testing.T) {
	// Two blocked subtiles meeting at a corner. A naive 8-way search steps
	// diagonally between them, which walks the mover through the seam of two
	// walls. Both orthogonal neighbours must be open for a diagonal to count.
	m := testEngine(4, 4)
	block(m, 10, 9)
	block(m, 9, 10)

	// The diagonal from (9,9) to (10,10) is exactly that seam.
	result := m.search(subTile{9, 9}, subTile{10, 10})
	steps := result.route(subTile{9, 9})

	require.NotEmpty(t, steps, "there is a way round")
	assert.NotEqual(t, subTile{10, 10}, steps[0], "the first step is not the corner cut")
	assert.Greater(t, len(steps), 1, "it goes around rather than through")
}

func TestPathFindSameSubtileReturnsTheDestination(t *testing.T) {
	m := testEngine(4, 4)

	dest := d2vector.NewPosition(7.5, 7.5)
	path := m.PathFind(d2vector.NewPosition(7.2, 7.9), dest)

	require.Len(t, path, 1, "a walk inside one subtile is one waypoint")
	assert.Equal(t, dest.X(), path[0].X())
	assert.Equal(t, dest.Y(), path[0].Y())
}

func TestPathFindOffTheMapDoesNotPanic(t *testing.T) {
	// The accessors refuse off-map coordinates now; the search must handle the
	// refusal rather than inheriting it as a crash.
	m := testEngine(4, 4)

	require.NotPanics(t, func() {
		m.PathFind(d2vector.NewPosition(5, 5), d2vector.NewPosition(500, 500))
	})

	require.NotPanics(t, func() {
		m.PathFind(d2vector.NewPosition(-50, -50), d2vector.NewPosition(5, 5))
	})
}

func TestOctileDistanceIsAdmissible(t *testing.T) {
	// The heuristic must never exceed the true cost of an unobstructed walk,
	// or the search stops returning shortest paths.
	assert.Equal(t, 0, octileDistance(3, 3, 3, 3), "no distance to itself")
	assert.Equal(t, costOrthogonal*4, octileDistance(0, 0, 4, 0), "a straight run")
	assert.Equal(t, costDiagonal*3, octileDistance(0, 0, 3, 3), "a pure diagonal")
	assert.Equal(t, costDiagonal*2+costOrthogonal*3, octileDistance(0, 0, 5, 2), "mixed")
	assert.Equal(t, octileDistance(0, 0, 5, 2), octileDistance(5, 2, 0, 0), "symmetric")

	// Never worse than the true diagonal cost, which is what admissible means
	// here: 14 per diagonal step against an exact 10*sqrt(2) = 14.142.
	assert.LessOrEqual(t, costDiagonal, 15, "the diagonal cost does not overestimate")
}

func TestNodeQueueOrderingIsTotal(t *testing.T) {
	// Equal f must fall through to h, then y, then x -- so no two distinct
	// nodes ever compare equal and the pop order cannot vary between runs.
	q := nodeQueue{
		{x: 5, y: 2, g: 10, h: 10}, // f 20
		{x: 1, y: 2, g: 12, h: 8},  // f 20, smaller h
		{x: 3, y: 1, g: 10, h: 10}, // f 20, same h, smaller y
		{x: 2, y: 2, g: 10, h: 10}, // f 20, same h, same y, smaller x
	}

	assert.True(t, q.Less(1, 0), "lower h wins at equal f")
	assert.True(t, q.Less(2, 0), "lower y wins at equal f and h")
	assert.True(t, q.Less(3, 0), "lower x wins at equal f, h and y")
	assert.False(t, q.Less(0, 0), "a node does not precede itself")

	// Antisymmetry across the whole set: exactly one direction holds for any
	// distinct pair.
	for i := range q {
		for j := range q {
			if i == j {
				continue
			}

			assert.NotEqual(t, q.Less(i, j), q.Less(j, i), "pair %d,%d is ordered one way", i, j)
		}
	}
}
