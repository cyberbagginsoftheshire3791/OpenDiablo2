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

	// BUG-9: this is a SIGHT test, so its blockers set the SIGHT bit. Until
	// 19 Sep 2026 checkLos consulted BlockWalk and these set BlockWalk to
	// match -- the test encoded the defect, which is why the swap turned three
	// untagged tests red and why none of them could ever have caught it.
	flags.BlockWalk = true
	flags.BlockLOS = true
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

// BUG-9, and the three cases no test had: WHICH bits stop a ray.
//
// checkLos consulted BlockWalk alone from the fork onwards, so BlockLOS -- the
// bit D2 authored for exactly this question -- was decoded and read nowhere. The
// whole corpus built its blockers with BlockWalk and so encoded the defect it
// should have caught.
//
// THE SHIPPED RULE IS THE SIGHT BIT ALONE, AND IT IS A DESIGN DECISION MADE ON
// MEASUREMENTS, not a typo repair. The census found BlockLOS to be a strict
// SUBSET of BlockWalk across Act 1 (1,194 against 10,252, BlockLOS-only ZERO),
// so the shipped rule removes 9,058 blockers. Both worlds were then measured
// over a full cycle at shipped dials: walls-as-cover gives 1 encounter and peak
// 0-1 hunters aware, darkness-as-cover gives 12 encounters and a player who
// survives on 13 of 240. The first is not a harder game, it is an absent one.
// sightBlocked's comment carries the whole argument.
func TestCheckLosNoLongerStopsAtAWalkOnlyBlocker(t *testing.T) {
	m := testEngine(4, 4)

	// A low wall, water, a ledge, a cart. 9,058 of Act 1's subtiles are exactly
	// this (tools/subtilecensus), and under the shipped rule the eye passes all
	// of them -- which is the 88.4% cut, seen one subtile at a time.
	flags := m.SubTileAt(3, 2)
	if flags == nil {
		t.Fatal("test setup: subtile (3,2) is off the map")
	}

	flags.BlockWalk = true

	clear, _ := m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(5, 2))
	assert.True(t, clear, "a subtile that blocks WALKING does not block SIGHT under the shipped rule")

	// AND BOTH ALTERNATIVES ARE REACHABLE, because this is a dial and the record
	// needs to be able to re-measure under the rule it was written beneath.
	m.setSightRule(SightBlockedByEither)

	clear, _ = m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(5, 2))
	assert.False(t, clear, "the union stops it -- on this art that rule IS the pre-fork rule")

	m.setSightRule(SightBlockedByWalkFlag)

	clear, _ = m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(5, 2))
	assert.False(t, clear, "and so does the pre-19-September rule every old number was measured under")
}

// THE ASSERTION BUG-9 WAS ACTUALLY ABOUT: the sight bit is read at all. Nothing
// in the engine read it before 19 September 2026, and under the pre-fork rule
// this test is red.
func TestCheckLosHonoursASightOnlyBlocker(t *testing.T) {
	m := testEngine(4, 4)

	// Opaque but walkable. Act 1 carries none of these (BlockLOS-only = 0 across
	// 38,425 subtiles), so this pins the RULE rather than the data -- if the
	// slice's own art ever marks one, sight obeys it.
	flags := m.SubTileAt(3, 2)
	if flags == nil {
		t.Fatal("test setup: subtile (3,2) is off the map")
	}

	flags.BlockLOS = true

	clear, _ := m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(5, 2))
	assert.False(t, clear, "a subtile that blocks SIGHT must block sight even when walkable")

	// The negative control that names the defect: under the pre-19-September
	// rule the engine cannot see this blocker at all.
	m.setSightRule(SightBlockedByWalkFlag)

	clear, _ = m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(5, 2))
	assert.True(t, clear, "BUG-9 itself: the old rule reads the walk bit and misses an opaque subtile")
}

// The DEFAULT is pinned, and every rule with it, because "which bits ship" is
// the single most consequential line in the sight model: it is the difference
// between 1 encounter a night and 12. A silent flip either way changes the whole
// game, so it fails a test.
func TestSightShipsTheSightFlagAndTheZeroValueIsThatRule(t *testing.T) {
	m := testEngine(4, 4)

	assert.Equal(t, SightBlockedBySightFlag, m.sightRuleOf(), "the shipped rule is the sight bit")
	assert.Equal(t, SightBlockedBySightFlag, SightRule(0),
		"and the ZERO VALUE is that same rule on purpose: this helper builds &MapEngine{}, and a field "+
			"nobody sets must mean what the game means -- the first cut of this had another rule at zero "+
			"and these assertions pinned the helper instead of the engine")

	for _, tc := range []struct {
		name      string
		rule      SightRule
		walk, los bool
		wantClear bool
	}{
		{"shipped: a properly marked wall stops it", SightBlockedBySightFlag, true, true, false},
		{"shipped: a walk-only blocker does not", SightBlockedBySightFlag, true, false, true},
		{"shipped: an opaque walkable subtile does", SightBlockedBySightFlag, false, true, false},
		{"shipped: open ground is clear", SightBlockedBySightFlag, false, false, true},
		{"union: a walk-only blocker stops it", SightBlockedByEither, true, false, false},
		{"union: an opaque walkable subtile stops it", SightBlockedByEither, false, true, false},
		{"union: open ground is still clear", SightBlockedByEither, false, false, true},
		{"pre-fork: the sight bit is invisible", SightBlockedByWalkFlag, false, true, true},
		{"pre-fork: the walk bit is everything", SightBlockedByWalkFlag, true, false, false},
	} {
		m := testEngine(4, 4)
		m.setSightRule(tc.rule)

		flags := m.SubTileAt(3, 2)
		if flags == nil {
			t.Fatal("test setup: subtile (3,2) is off the map")
		}

		flags.BlockWalk, flags.BlockLOS = tc.walk, tc.los

		clear, _ := m.checkLos(d2vector.NewPosition(2, 2), d2vector.NewPosition(5, 2))
		assert.Equal(t, tc.wantClear, clear, tc.name)
	}
}
