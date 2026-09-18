package d2mapengine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// BUG-8, the sight burst. checkLos walked i <= int(N) and incremented BEFORE
// sampling, so it took int(N)+1 unit steps along a segment only N long and
// inspected one subtile PAST the destination. The overshoot is exactly one
// full subtile whenever N is a whole number -- every axis-aligned and 45-degree
// ray between grid-aligned entities -- and a fraction of one otherwise.
//
// It was live, not academic: mapSight.Clear (d2game/d2gamescreen/game.go) is
// the only Sight implementation in the build and notice.go:341 gates awareness
// on it ONE-DIRECTIONALLY, so the forward/reverse asymmetry below never
// averaged out. It also reached playtest evidence through harness_obs.go's
// StraightLineClear, the control that proves a route was a genuine detour.
//
// These assert the SHAPE of the traversal through behaviour, which is what no
// existing test did: accessors_test.go asserts only NotPanics and loose bounds,
// and its one clear case passes by luck (its last sample lands at 18,18 on a
// map whose last valid subtile is 19). notice_test.go uses a fakeSight and
// never runs the real cast at all.

// blockAt marks one subtile as blocking, and fails loudly if the coordinate is
// off the map -- a nil there would make an assertion below pass for the wrong
// reason.
func blockAt(t *testing.T, m *MapEngine, x, y int) {
	t.Helper()

	flags := m.SubTileAt(x, y)
	if flags == nil {
		t.Fatalf("test setup: subtile (%d,%d) is off the map", x, y)
	}

	flags.BlockWalk = true
}

func TestCheckLosStopsAtItsDestination(t *testing.T) {
	m := testEngine(4, 4) // subtiles 0..19 in both axes

	// The audit's case, exactly. A ray from (2,2) to (4,2) has N == 2 and must
	// sample (3,2) and (4,2). The old loop also sampled (5,2).
	blockAt(t, m, 5, 2)

	clear, _ := m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(4, 2))
	assert.True(t, clear, "a blocker one subtile PAST the destination must not block the ray")
}

func TestCheckLosIsSymmetric(t *testing.T) {
	m := testEngine(4, 4)

	// Same geometry as above. Awareness is evaluated in one direction per
	// watcher (notice.go:341), so an asymmetric ray means cover works or does
	// not depending purely on which side of the player the watcher stands.
	blockAt(t, m, 5, 2)

	forward, _ := m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(4, 2))
	reverse, _ := m.checkLos(d2vector.NewPosition(4, 2), d2vector.NewPosition(2, 2))
	assert.Equal(t, forward, reverse, "the same two points must give the same answer in both directions")

	// And at subtile centres, which is what NewPositionTile produces for a
	// live entity -- the overshoot was not an artefact of integer coordinates.
	c := testEngine(4, 4)
	blockAt(t, c, 5, 2)

	forwardC, _ := c.checkLos(d2vector.NewPosition(2.5, 2.5), d2vector.NewPosition(4.5, 2.5))
	reverseC, _ := c.checkLos(d2vector.NewPosition(4.5, 2.5), d2vector.NewPosition(2.5, 2.5))
	assert.Equal(t, forwardC, reverseC, "subtile centres are symmetric too")
}

func TestCheckLosStillSamplesItsDestination(t *testing.T) {
	// The trap in the obvious fix. Changing <= to < takes int(N) steps, which
	// lands on the endpoint only when N is WHOLE; for a fractional N the last
	// sample falls short and the destination is never examined -- trading a
	// false negative for a watcher seeing through a blocker standing on the
	// player. Both cases are asserted so the cheap fix cannot pass.
	whole := testEngine(4, 4)
	blockAt(t, whole, 4, 2)

	clear, _ := whole.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(4, 2))
	assert.False(t, clear, "a blocker ON the destination blocks, whole N")

	frac := testEngine(4, 4)
	blockAt(t, frac, 4, 2)

	// N == 1.9: int(N) == 1, so a '<' loop stops at x == 2.5+1.0 and never
	// reaches subtile 4.
	clearFrac, _ := frac.checkLos(d2vector.NewPosition(2.5, 2.5), d2vector.NewPosition(4.4, 2.5))
	assert.False(t, clearFrac, "a blocker ON the destination blocks, fractional N")
}

func TestCheckLosReachesALegalMapEdgeEndpoint(t *testing.T) {
	m := testEngine(4, 4) // last valid subtile is 19

	// Nothing is blocked. The old loop sampled (20,2), where SubTileAt returns
	// nil, and nil is read as a wall -- so a perfectly legal endpoint on the
	// last column was rejected.
	clear, _ := m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(19, 2))
	assert.True(t, clear, "an endpoint on the last valid subtile is reachable")

	// And a FRACTIONAL endpoint on that last subtile, which is what a live
	// entity position actually looks like. This is the assertion that fails if
	// the final step is not clamped to N: ceil(17.5) == 18 steps of one subtile
	// would read (20,2), off the map, and nil is read as a wall.
	clearFrac, _ := m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(19.5, 2))
	assert.True(t, clearFrac, "a fractional endpoint on the last valid subtile is reachable")
}

func TestCheckLosStillBlocksAndStillIgnoresTheStartCell(t *testing.T) {
	// The behaviour that must NOT change while the endpoint is fixed.
	between := testEngine(4, 4)
	blockAt(t, between, 3, 2)

	clear, _ := between.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(4, 2))
	assert.False(t, clear, "a blocker between start and end still blocks")

	// Standing in a blocking subtile does not blind you: the start cell is
	// never sampled, which is correct for sight and is why the loop increments
	// before it reads.
	onStart := testEngine(4, 4)
	blockAt(t, onStart, 2, 2)

	clearFromBlocked, _ := onStart.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(4, 2))
	assert.True(t, clearFromBlocked, "the start subtile is not sampled")

	// A ray to where it already is has nothing to sample and is clear -- and
	// that holds even standing IN a blocker, which is the start-cell rule taken
	// to its limit. The old loop ran once for N == 0 and sampled the start,
	// so this case changes; it is pinned here rather than left to accident.
	zero, _ := between.checkLos(d2vector.NewPosition(7, 7), d2vector.NewPosition(7, 7))
	assert.True(t, zero, "a zero-length ray is clear")

	zeroOnBlocked, _ := onStart.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(2, 2))
	assert.True(t, zeroOnBlocked, "a zero-length ray from inside a blocker is clear")
}
