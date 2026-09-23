package d2mapengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// longWall builds a 60x60-tile map (300x300 subtiles) with a wall down subtile
// column 150 from the top to row 280, so the only way from the west half to
// the east is a detour south round its end -- far too long for one bounded
// subtile search.
func longWall() *MapEngine {
	m := testEngine(60, 60)

	for y := 0; y < 280; y++ {
		block(m, 150, y)
	}

	return m
}

func TestTheLongWayRoundIsFound(t *testing.T) {
	m := longWall()

	from, goal := subTile{100, 20}, subTile{200, 20}

	// The need, measured: the ordinary search alone runs out of budget.
	alone := m.search(from, goal)
	require.False(t, alone.exact, "control: one bounded search cannot walk round a 56-tile wall")
	require.Equal(t, maxExpandedNodes, alone.expanded, "and it failed on BUDGET, not on a sealed goal")
	require.True(t, alone.exhausted, "which the result says")

	path := m.PathFind(d2vector.NewPosition(100.5, 20.5), d2vector.NewPosition(200.5, 20.5))
	require.NotEmpty(t, path)

	final := path[len(path)-1]
	assert.InDelta(t, 200.5, final.X(), 1e-9, "the route arrives")
	assert.InDelta(t, 20.5, final.Y(), 1e-9, "the route arrives")

	deepest := 0.0
	for _, p := range path {
		if p.Y() > deepest {
			deepest = p.Y()
		}
	}

	assert.Greater(t, deepest, 280.0, "it went round the wall's south end")
}

// Every step the corridor joins is a legal step: adjacent, onto an open
// subtile, and never cutting a corner.
func TestTheCorridorStepsAreLegal(t *testing.T) {
	m := longWall()
	from, goal := subTile{100, 20}, subTile{200, 20}

	steps, ok := m.corridorRoute(from, goal)
	require.True(t, ok)
	require.Equal(t, goal, steps[len(steps)-1], "the last step is the goal")

	prev := from
	for i, s := range steps {
		dx, dy := s.x-prev.x, s.y-prev.y
		require.True(t, abs(dx) <= 1 && abs(dy) <= 1 && (dx != 0 || dy != 0), "step %d %v -> %v is not adjacent", i, prev, s)
		require.False(t, m.blockedAt(s.x, s.y), "step %d lands on a blocked subtile %v", i, s)

		if dx != 0 && dy != 0 {
			require.False(t, m.blockedAt(prev.x+dx, prev.y) || m.blockedAt(prev.x, prev.y+dy), "step %d cuts a corner", i)
		}

		prev = s
	}
}

// A wall one subtile thick, running between two open tile centres, stops the
// corridor -- the coarse grid checks the line between centres, not just the
// centres. Here the wall is complete, so there is no corridor at all and
// PathFind returns the same best partial it always did.
func TestAThinCompleteWallStopsTheCorridor(t *testing.T) {
	m := testEngine(60, 60)

	for y := 0; y < 300; y++ {
		block(m, 150, y) // subtile 150 is tile 30's west edge; its centre (152) is open
	}

	require.Nil(t, m.coarsePath(subTile{20, 4}, subTile{40, 4}), "no corridor through a complete wall")

	from, goal := subTile{100, 20}, subTile{200, 20}
	before := m.search(from, goal)
	require.False(t, before.exact)

	path := m.PathFind(d2vector.NewPosition(100.5, 20.5), d2vector.NewPosition(200.5, 20.5))
	require.NotEmpty(t, path, "still walks as far as it can")
	assert.Less(t, path[len(path)-1].X(), 150.0, "and stops on this side")

	// Byte for byte the old partial: the corridor changed nothing here.
	want := waypoints(from, before.route(from), d2vector.NewPosition(200.5, 20.5), false)
	require.Equal(t, len(want), len(path))

	for i := range want {
		assert.Equal(t, want[i].X(), path[i].X(), "waypoint %d x", i)
		assert.Equal(t, want[i].Y(), path[i].Y(), "waypoint %d y", i)
	}
}

// samePartial asserts PathFind returned exactly the ordinary search's partial.
func samePartial(t *testing.T, m *MapEngine, from, goal subTile) {
	t.Helper()

	before := m.search(from, goal)
	require.False(t, before.exact)

	dest := d2vector.NewPosition(float64(goal.x)+0.5, float64(goal.y)+0.5)
	want := waypoints(from, before.route(from), dest, false)
	got := m.PathFind(d2vector.NewPosition(float64(from.x)+0.5, float64(from.y)+0.5), dest)

	require.Equal(t, len(want), len(got))

	for i := range want {
		assert.Equal(t, want[i].X(), got[i].X(), "waypoint %d x", i)
		assert.Equal(t, want[i].Y(), got[i].Y(), "waypoint %d y", i)
	}
}

// A goal on a blocked subtile, far enough to exhaust the ordinary search:
// refused before any corridor work, the old partial returned.
func TestABlockedGoalGetsTheOldPartial(t *testing.T) {
	m := longWall()
	block(m, 200, 20)

	_, ok := m.corridorRoute(subTile{100, 20}, subTile{200, 20})
	require.False(t, ok)

	samePartial(t, m, subTile{100, 20}, subTile{200, 20})
}

// A corridor that reaches the goal's TILE, but a goal sealed inside it by a
// ring of subtiles: the last leg fails, and the old partial comes back byte
// for byte.
func TestASealedPocketGetsTheOldPartial(t *testing.T) {
	m := longWall()

	// The goal subtile 203,22 inside tile 40,4, ringed in.
	for x := 202; x <= 204; x++ {
		for y := 21; y <= 23; y++ {
			if x != 203 || y != 22 {
				block(m, x, y)
			}
		}
	}

	require.NotNil(t, m.coarsePath(subTile{20, 4}, subTile{40, 4}), "the corridor reaches the pocket's tile")

	_, ok := m.corridorRoute(subTile{100, 20}, subTile{203, 22})
	require.False(t, ok, "but no leg reaches into the pocket")

	samePartial(t, m, subTile{100, 20}, subTile{203, 22})
}

// The known limit: a gap in a wall that does not cross any tile-centre line
// is invisible to the coarse grid, and the corridor fails safe.
func TestAnOffCentreGapIsNotACorridor(t *testing.T) {
	m := testEngine(60, 60)

	// A complete wall down subtile column 150 except rows 150-151, which lie
	// in tile row 30 but off its centre line (row 152).
	for y := 0; y < 300; y++ {
		if y != 150 && y != 151 {
			block(m, 150, y)
		}
	}

	require.Nil(t, m.coarsePath(subTile{20, 4}, subTile{40, 4}), "the coarse grid does not see the gap")
	samePartial(t, m, subTile{100, 20}, subTile{200, 20})
}

// A goal walled off in a small map runs the open set dry inside the budget:
// not exhausted, so the corridor is never tried.
func TestASealedSearchIsNotExhausted(t *testing.T) {
	m := testEngine(8, 8)
	blockColumn(m, 20, 40, 0)

	res := m.search(subTile{5, 5}, subTile{35, 5})
	require.False(t, res.exact)
	assert.False(t, res.exhausted, "the open set ran dry: the goal is walled off, not far")
	assert.Less(t, res.expanded, maxExpandedNodes)
}

// A short corridor is not tried: its one leg would be the search that failed.
func TestAShortCorridorIsNotTried(t *testing.T) {
	m := testEngine(20, 20)
	_, ok := m.corridorRoute(subTile{10, 10}, subTile{30, 10})
	assert.False(t, ok, "a four-tile corridor is left to the ordinary search")
}

func TestTheLongWayRoundIsDeterministic(t *testing.T) {
	a := longWall().PathFind(d2vector.NewPosition(100.5, 20.5), d2vector.NewPosition(200.5, 20.5))
	b := longWall().PathFind(d2vector.NewPosition(100.5, 20.5), d2vector.NewPosition(200.5, 20.5))

	require.Equal(t, len(a), len(b))

	for i := range a {
		assert.Equal(t, a[i].X(), b[i].X(), "waypoint %d x", i)
		assert.Equal(t, a[i].Y(), b[i].Y(), "waypoint %d y", i)
	}
}

// Off the map, and a goal on the far side of the negative edge: no corridor,
// no panic.
func TestTheCorridorRefusesOffTheMap(t *testing.T) {
	m := longWall()

	_, ok := m.corridorRoute(subTile{100, 20}, subTile{-3, 20})
	assert.False(t, ok)

	_, ok = m.corridorRoute(subTile{100, 20}, subTile{5000, 20})
	assert.False(t, ok)
}

// The cost, for the record: a route round the long wall, and a search toward
// a goal sealed off by a complete wall (the case that pays for the ordinary
// search's whole budget AND a fruitless corridor).
func BenchmarkPathFindTheLongWayRound(b *testing.B) {
	m := longWall()
	from, to := d2vector.NewPosition(100.5, 20.5), d2vector.NewPosition(200.5, 20.5)

	for i := 0; i < b.N; i++ {
		m.PathFind(from, to)
	}
}

func BenchmarkPathFindWalledOff(b *testing.B) {
	m := testEngine(60, 60)
	for y := 0; y < 300; y++ {
		block(m, 150, y)
	}

	from, to := d2vector.NewPosition(100.5, 20.5), d2vector.NewPosition(200.5, 20.5)

	for i := 0; i < b.N; i++ {
		m.PathFind(from, to)
	}
}

func BenchmarkSearchAloneWalledOff(b *testing.B) {
	m := testEngine(60, 60)
	for y := 0; y < 300; y++ {
		block(m, 150, y)
	}

	for i := 0; i < b.N; i++ {
		m.search(subTile{100, 20}, subTile{200, 20})
	}
}
