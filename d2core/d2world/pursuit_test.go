package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fakes below stand in for the map engine and its entities. Pursuit talks
// to both through interfaces precisely so this file needs neither -- no MPQs,
// no ebiten, no game running.

type fakeRouter struct {
	calls     int
	reachable bool
	// lastFrom/lastTo record what the system asked for, so a test can assert
	// that a re-path aimed at where the quarry IS rather than where it was.
	lastFromX, lastFromY float64
	lastToX, lastToY     float64
}

func (r *fakeRouter) Route(fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	r.calls++
	r.lastFromX, r.lastFromY = fromX, fromY
	r.lastToX, r.lastToY = toX, toY

	return [][2]float64{{toX, toY}}, r.reachable
}

type fakeHunter struct {
	id        string
	x, y      float64
	following bool
	routes    int
	lastRoute [][2]float64
}

func (h *fakeHunter) HunterID() string             { return h.id }
func (h *fakeHunter) HunterAt() (float64, float64) { return h.x, h.y }
func (h *fakeHunter) Following() bool              { return h.following }

func (h *fakeHunter) Follow(waypoints [][2]float64) {
	h.routes++
	h.lastRoute = waypoints
	h.following = true
}

type fakeQuarry struct {
	id   string
	x, y float64
}

func (q *fakeQuarry) QuarryID() string             { return q.id }
func (q *fakeQuarry) QuarryAt() (float64, float64) { return q.x, q.y }

func newTestPursuit(reachable bool) (*Pursuit, *fakeRouter) {
	r := &fakeRouter{reachable: reachable}
	p := NewPursuit(r, DefaultPursuitDials())

	return p, r
}

func TestChaseSolvesImmediately(t *testing.T) {
	p, router := newTestPursuit(true)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 10, y: 0}

	p.Chase(h, q)

	// A hunter that waits for the next tick before starting to move looks
	// broken to anyone watching, so the first route is solved on the spot.
	assert.Equal(t, 1, router.calls, "the first route is solved when the chase starts")
	assert.Equal(t, 1, h.routes, "and handed straight to the hunter")
	assert.Equal(t, 1, p.Count(), "one live chase")
	assert.Equal(t, [][2]float64{{10, 0}}, h.lastRoute)
}

func TestQuarryStandingStillCostsNoSolves(t *testing.T) {
	p, router := newTestPursuit(true)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 10, y: 0}

	p.Chase(h, q)
	require.Equal(t, 1, router.calls)

	// Ten world minutes of a quarry that has not moved. The route is still
	// good, so re-solving it would be pure waste -- and on a night with a
	// pack out, waste is measured in frames.
	for i := 0; i < 10; i++ {
		p.Advance(1)
	}

	assert.Equal(t, 1, router.calls, "a stationary quarry never invalidates the route")
}

func TestQuarryMovingFarEnoughForcesARepath(t *testing.T) {
	p, router := newTestPursuit(true)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 10, y: 0}

	p.Chase(h, q)
	require.Equal(t, 1, router.calls)

	// Under the dial: still no solve.
	q.x = 11.0 // moved 1.0, dial is 1.5
	p.Advance(1)
	assert.Equal(t, 1, router.calls, "a small drift does not invalidate the route")

	// Over the dial: solve, and aimed at where the quarry IS now.
	q.x = 13.0 // moved 3.0 from the solve point
	p.Advance(1)

	assert.Equal(t, 2, router.calls, "the quarry outran the dial and the route was recomputed")
	assert.Equal(t, 13.0, router.lastToX, "the new route aims at where the quarry is now")
	assert.Equal(t, 2, h.routes, "and the hunter was given it")
}

func TestMinRepathMinutesFloorsTheSolveRate(t *testing.T) {
	p, router := newTestPursuit(true)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 10, y: 0}

	p.Chase(h, q)
	require.Equal(t, 1, router.calls)

	// A quarry teleporting back and forth across the dial on every tick must
	// not be able to make the hunter solve on every tick.
	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			q.x = 30
		} else {
			q.x = 10
		}

		p.Advance(0.05) // well under MinRepathMinutes of 0.25
	}

	assert.LessOrEqual(t, router.calls, 5,
		"the floor bounds the solve rate no matter how the quarry jitters")
}

func TestArrivingStopsTheChaseWithoutEndingIt(t *testing.T) {
	p, router := newTestPursuit(true)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 10, y: 0}

	p.Chase(h, q)
	require.Equal(t, 1, router.calls)

	// The hunter catches up. M4.3a has no combat, so the honest end of the
	// milestone is that it stands there -- and keeps standing there without
	// burning a search every tick.
	h.x, h.y = 9.5, 0

	p.Advance(1)
	p.Advance(1)

	assert.Equal(t, 1, router.calls, "an arrived hunter does not keep re-solving")
	assert.Equal(t, 1, p.Count(), "but the chase is still live -- it did not end itself")

	state := p.HarnessState()
	list, ok := state["chase_list"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, list, 1)
	assert.Equal(t, true, list[0]["arrived"], "and it reports that it arrived")
}

func TestAHunterThatStoppedWalkingGetsANewRoute(t *testing.T) {
	p, router := newTestPursuit(true)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 10, y: 0}

	p.Chase(h, q)
	require.Equal(t, 1, router.calls)

	// The quarry has not moved, but the hunter has run out of route -- it
	// walked a partial path to a goal it could not reach. Without this it
	// would stand still forever holding a stale, finished route.
	h.following = false
	h.x = 4

	p.Advance(5) // past MinRepathMinutes

	assert.Equal(t, 2, router.calls, "a hunter with no route left is given a fresh one")
	assert.Equal(t, 4.0, router.lastFromX, "solved from where the hunter actually is")
}

func TestAnUnreachableQuarryDoesNotBurnASearchEveryTick(t *testing.T) {
	// The regression test for what the playtest caught: a hunter that cannot
	// reach its quarry walks its short partial route, runs out, and asks the
	// same unanswerable question again. Across 600 stepped frames that was
	// 218 solves. The quarry has not moved, so the answer cannot have changed.
	p, router := newTestPursuit(false)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 10, y: 0}

	p.Chase(h, q)
	require.Equal(t, 1, router.calls)

	for i := 0; i < 50; i++ {
		h.following = false // out of route, every single tick
		p.Advance(5)        // and well past the time floor
	}

	assert.Equal(t, 1, router.calls,
		"a hunter that is making no progress stops asking an unanswerable question")

	// But the moment the quarry actually moves, the question is new again.
	q.x = 30
	p.Advance(5)

	assert.Equal(t, 2, router.calls, "a quarry that moves is worth re-solving for")
}

func TestAnUnreachableQuarryIsReAskedFromCloserGround(t *testing.T) {
	// The other half of that guard, and the reason it is progress rather than
	// a flat refusal: a partial route still carries the hunter closer, and
	// from closer the question is a genuinely new one. Without this a pursuer
	// stalls short of a quarry it could have reached from two tiles nearer --
	// which the playtest saw as a hunter parked 2.80 tiles away.
	p, router := newTestPursuit(false)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 20, y: 0}

	p.Chase(h, q)
	require.Equal(t, 1, router.calls)

	// The hunter walks its partial route and gains real ground.
	h.following = false
	h.x = 8

	p.Advance(5)
	assert.Equal(t, 2, router.calls, "ground gained buys another question")

	// It gains no more. The asking stops.
	for i := 0; i < 20; i++ {
		h.following = false
		p.Advance(5)
	}

	assert.Equal(t, 2, router.calls, "and stops again once the progress does")
}

func TestReleaseTakesAChaseBackOut(t *testing.T) {
	p, _ := newTestPursuit(true)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 10, y: 0}

	p.Chase(h, q)
	require.Equal(t, 1, p.Count())

	assert.True(t, p.Release("n:1"), "releasing a live chase reports that it did something")
	assert.Equal(t, 0, p.Count(), "and the collection shrinks")
	assert.False(t, p.Release("n:1"), "releasing it twice is not a silent success")
}

func TestOneHunterChasesOneThing(t *testing.T) {
	p, _ := newTestPursuit(true)
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	first := &fakeQuarry{id: "p:1", x: 10, y: 0}
	second := &fakeQuarry{id: "p:2", x: -10, y: 0}

	p.Chase(h, first)
	p.Chase(h, second)

	assert.Equal(t, 1, p.Count(), "the second chase replaced the first")

	state := p.HarnessState()
	list, _ := state["chase_list"].([]map[string]interface{})
	require.Len(t, list, 1)
	assert.Equal(t, "p:2", list[0]["quarry"], "and it is chasing the newer quarry")
}

func TestAdvanceOrderIsStable(t *testing.T) {
	// Ranging a map is randomised in Go, and these solves move entity
	// positions, which are inside the state digest. The order must not vary.
	p, _ := newTestPursuit(true)
	defer p.Close()

	q := &fakeQuarry{id: "p:1", x: 100, y: 0}
	for _, id := range []string{"n:3", "n:1", "n:2"} {
		p.Chase(&fakeHunter{id: id}, q)
	}

	first := p.hunterIDs()
	for i := 0; i < 20; i++ {
		assert.Equal(t, first, p.hunterIDs(), "hunter order is stable across reads")
	}

	assert.Equal(t, []string{"n:1", "n:2", "n:3"}, first, "and it is sorted")
}

func TestUnreachableQuarryStillGetsWalkedToward(t *testing.T) {
	p, router := newTestPursuit(false) // the router never reaches
	defer p.Close()

	h := &fakeHunter{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 10, y: 0}

	p.Chase(h, q)

	require.Equal(t, 1, router.calls)
	assert.Equal(t, 1, h.routes, "a partial route is still handed over and still walked")

	state := p.HarnessState()
	list, _ := state["chase_list"].([]map[string]interface{})
	require.Len(t, list, 1)
	assert.Equal(t, false, list[0]["reachable"], "and the chase reports it cannot get there")
}

func TestHarnessSetMovesTheDialsAndRefusesNonsense(t *testing.T) {
	p, _ := newTestPursuit(true)
	defer p.Close()

	require.NoError(t, p.HarnessSet("repath_tiles", 4.0))
	assert.Equal(t, 4.0, p.dials.RepathTiles)

	require.NoError(t, p.HarnessSet("arrive_within", 2.0))
	assert.Equal(t, 2.0, p.dials.ArriveWithin)

	assert.Error(t, p.HarnessSet("repath_tiles", "far"), "a dial wants a number")
	assert.Error(t, p.HarnessSet("repath_tiles", -1.0), "a negative re-path distance is meaningless")
	assert.Error(t, p.HarnessSet("release", 7), "release wants a hunter id")
	assert.Error(t, p.HarnessSet("release", "n:99"), "releasing a chase that is not running is an error")
	assert.Error(t, p.HarnessSet("chase", "n:1"), "there is deliberately no chase field here")
}

func TestPursuitReportsItsName(t *testing.T) {
	p, _ := newTestPursuit(true)
	defer p.Close()

	assert.Equal(t, "pursuit", p.HarnessName())
	assert.Contains(t, p.HarnessSettableFields(), "release")
}
