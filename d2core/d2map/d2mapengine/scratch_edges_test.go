package d2mapengine

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2geom"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// The scratch's and the RouteQuery's edges (the BUG-115 review's B): each
// test is red under the mutation it names (strigoi-harness-runs\wt-b115\nc2).

// requireMaster asks q (or a one-shot PathFind) and master the same question.
func requireMaster(t *testing.T, m *MapEngine, q *RouteQuery, start, dest d2vector.Position, msg string) {
	t.Helper()

	want := masterOf(m).PathFind(start, dest)

	if q != nil {
		require.Equal(t, want, q.PathFind(dest), "%s (query)", msg)
	} else {
		require.Equal(t, want, m.PathFind(start, dest), "%s", msg)
	}
}

// STAMPS WRAP CLEAN AFTER REAL USE. The scratch is used for real -- one
// search to its cap from the hunter, and one corridor tile search, so the
// first stamp (1) is written over a wide region -- then both stamps are moved
// FORWARD to their last value, as only use moves them, and the same hunter's
// questions are asked across the wrap: master's answers every time, and the
// tile search master's corridor. Control M4 (the wrap leaves the cells'
// stamps): the first search after the wrap reads the old search's cells as
// its own, red. (The tile grid's edges cache, M19, cannot mislead on an
// unchanged map -- a line is a pure function of the walls -- so it is not
// claimed here.)
func TestStampsWrapCleanAfterRealUse(t *testing.T) {
	m := referenceMaze(4)
	rng := rand.New(rand.NewSource(4)) // nolint:gosec // a test's pairs

	from := subTile{100, 100}
	if m.blockedAt(from.x, from.y) {
		t.Fatal("the fixture's hunter stands on a wall")
	}

	used := m.search(from, subTile{199, 5})
	require.Greater(t, used.expanded, 1000, "real use: a wide first search")
	require.Equal(t, masterCoarse(masterOf(m), subTile{20, 20}, subTile{3, 30}), m.coarsePath(subTile{20, 20}, subTile{3, 30}))

	s := m.acquireScratch()
	require.Equal(t, uint32(1), s.stamp, "the first search wrote stamp 1")
	require.Equal(t, uint32(1), s.tileStamp, "the first tile search wrote stamp 1")

	s.stamp, s.tileStamp = math.MaxUint32, math.MaxUint32
	m.releaseScratch(s)

	start := d2vector.NewPosition(100.5, 100.5)

	for i := 0; i < 30; i++ {
		requireMaster(t, m, nil, start, d2vector.NewPosition(rng.Float64()*200, rng.Float64()*200), "across the wrap")
	}

	require.Equal(t, masterCoarse(masterOf(m), subTile{20, 20}, subTile{3, 30}), m.coarsePath(subTile{20, 20}, subTile{3, 30}), "the tile search across its wrap")

	s = m.acquireScratch()
	require.Less(t, s.stamp, uint32(1000), "the stamp wrapped")
	m.releaseScratch(s)
}

// MARKS WRAP CLEAN AFTER REAL USE. A query proves a yard sealed; the yard's
// wall is then opened (the editor changes a map under a live scratch) and the
// marks' base moved forward to the edge of wrapping; the next query, which
// takes the base the first one had, must not read the old "sealed". Control
// M5 (the wrap leaves the marks): the yard reads proven, the open yard is
// skipped, red.
func TestMarksWrapCleanAfterRealUse(t *testing.T) {
	m := sealedYard()

	// A second yard, west, sealed for good: the failure that arms the proofs.
	for k := 49; k <= 60; k++ {
		block(m, k, 49)
		block(m, k, 60)
		block(m, 49, k)
		block(m, 60, k)
	}

	hunter := d2vector.NewPositionTile(20.5, 40.5)
	inYard := d2vector.NewPositionTile(32.5, 31.5)
	inWest := d2vector.NewPositionTile(11, 11)

	q := m.NewRouteQuery(hunter)
	requireMaster(t, m, q, hunter, inYard, "the yard, sealed")
	require.True(t, q.ProvenUnreachable(inYard, 32, 31), "the first query proves the yard sealed")

	base := q.base
	q.Close()

	// A door in the yard's south wall.
	for x := 152; x <= 167; x++ {
		m.tiles[(170/5)*m.size.Width+x/5].SubTiles[(4-170%5)*5+x%5].BlockWalk = false
	}

	forgetMaster(m)

	s := m.acquireScratch()
	if v := uint32(math.MaxUint32 - 2*markSpan); v > s.markBase {
		s.markBase = v
	}
	m.releaseScratch(s)

	q = m.NewRouteQuery(hunter)
	defer q.Close()

	requireMaster(t, m, q, hunter, inWest, "the west yard, sealed")
	require.True(t, q.failed, "a failure arms the proofs")
	require.Equal(t, base, q.base, "the wrapped query has the first one's base")
	require.False(t, q.ProvenUnreachable(inYard, 32, 31), "the opened yard is not proven")
	requireMaster(t, m, q, hunter, inYard, "the yard, opened")
	require.True(t, endsIn(masterOf(m).PathFind(hunter, inYard), 32.5, 31.5), "master walks in")
}

// THE SCRATCH FOLLOWS A RESIZE IN EITHER DIMENSION. Control M6 (the size
// check reads the width alone): the taller map indexes past the old grid, red.
func TestTheScratchFollowsAResize(t *testing.T) {
	m := testEngine(20, 10)
	block(m, 30, 20)
	requireMaster(t, m, nil, d2vector.NewPosition(5.5, 5.5), d2vector.NewPosition(90.5, 40.5), "20x10")

	for _, size := range [][2]int{{20, 30}, {20, 4}, {35, 30}} {
		m.size = d2geom.Size{Width: size[0], Height: size[1]}
		m.tiles = make([]MapTile, size[0]*size[1])
		block(m, 30, 2)
		forgetMaster(m)

		far := d2vector.NewPosition(float64(size[0]*5)-1.5, float64(size[1]*5)-1.5)
		requireMaster(t, m, nil, d2vector.NewPosition(5.5, 5.5), far, "resized")
		requireMaster(t, m, nil, far, d2vector.NewPosition(2.5, 2.5), "resized, back")
	}
}

// A TILE REACHED ONLY ON ITS LAST ROW IS NOT PROVEN. The goal subtile (the
// tile's corner) is a sealed pocket of one, the tile's first four rows are
// wall, and its last row is open to the south: the old route ends there,
// which routeExact counts as reaching. Control M7 (the tile proof stops a row
// short): the tile reads proven, red.
func TestATileReachedOnlyOnItsLastRowIsNotProven(t *testing.T) {
	m := testEngine(20, 20)

	for y := 45; y <= 53; y++ {
		for x := 45; x <= 59; x++ {
			if x != 50 || y != 50 {
				block(m, x, y)
			}
		}
	}

	start, dest := d2vector.NewPosition(52.5, 80.5), d2vector.NewPositionTile(10, 10)
	want := masterOf(m).PathFind(start, dest)
	require.True(t, endsIn(want, 10, 10), "master's route ends in the tile, on its last row")

	q := m.NewRouteQuery(start)
	defer q.Close()

	require.Equal(t, want, q.PathFind(dest))
	require.True(t, q.unreachable(subTile{50, 50}), "the goal subtile is proven sealed")
	require.False(t, q.ProvenUnreachable(dest, 10, 10), "but the tile is not")
}

// THE START'S OWN SUBTILE IS NEVER PROVEN, even when it is a wall. A hunter
// standing inside a walled tile cannot step anywhere (his search runs dry at
// once), yet a route to his own subtile is his own position -- master returns
// it and routeExact calls it reached. Control M8 (no start exemption): his
// tile reads proven, red.
func TestTheStartsOwnSubtileIsNeverProven(t *testing.T) {
	m := testEngine(20, 20)

	for k := 0; k < 25; k++ {
		block(m, 50+k%5, 50+k/5)
	}

	start := d2vector.NewPosition(52.5, 52.5)
	q := m.NewRouteQuery(start)
	defer q.Close()

	requireMaster(t, m, q, start, d2vector.NewPosition(80.5, 80.5), "anywhere else: walled in")
	require.True(t, q.sealedIn, "his search ran dry")

	require.True(t, endsIn(masterOf(m).PathFind(start, start), 10.5, 10.5), "master: his own subtile is reached")
	require.False(t, q.ProvenUnreachable(start, 10, 10), "so his tile is not proven")
	requireMaster(t, m, q, start, start, "his own subtile")
}

// A DRY FAILED SEARCH, THEN A REACHABLE CANDIDATE. A hunter in a sealed yard
// asks for somewhere outside (his search runs dry: his whole reach is what it
// visited), then for tiles inside his own yard: none is proven, and each
// route is master's. Control M21 (the search forgets to record what it
// visits): only his own subtile reads reachable, every tile in his yard reads
// proven, red.
func TestADryFailureThenAReachableCandidate(t *testing.T) {
	m := sealedYard()
	hunter := d2vector.NewPositionTile(31.5, 31.5)

	q := m.NewRouteQuery(hunter)
	defer q.Close()

	requireMaster(t, m, q, hunter, d2vector.NewPositionTile(50.5, 12.5), "outside his yard")
	require.True(t, q.sealedIn, "the search ran dry")

	for _, tile := range [][2]int{{30, 30}, {33, 33}, {32, 31}, {30, 33}} {
		dest := d2vector.NewPositionTile(float64(tile[0])+0.5, float64(tile[1])+0.5)
		require.False(t, q.ProvenUnreachable(dest, tile[0], tile[1]), "tile %v in his yard", tile)
		requireMaster(t, m, q, hunter, dest, "inside his yard")
	}
}
