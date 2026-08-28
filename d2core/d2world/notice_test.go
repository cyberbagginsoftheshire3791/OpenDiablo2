package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fakes below stand in for the map engine and the light model. Notice
// talks to both through interfaces precisely so this file needs neither --
// no MPQs, no ebiten, no game running. fakeQuarry is reused from
// pursuit_test.go on purpose: the thing worth noticing and the thing worth
// chasing are the same thing, which is why Notice targets a Quarry.

type fakeSight struct {
	clear bool
	calls int
}

func (s *fakeSight) Clear(fromX, fromY, toX, toY float64) bool {
	s.calls++

	return s.clear
}

type fakeIllumination struct{ level float64 }

func (i *fakeIllumination) Level(tileX, tileY int) float64 { return i.level }

type fakeWatcher struct {
	id   string
	x, y float64
}

func (w *fakeWatcher) WatcherID() string             { return w.id }
func (w *fakeWatcher) WatcherAt() (float64, float64) { return w.x, w.y }

// newTestNotice wires a model with everything visible and unlit by default.
func newTestNotice(clear bool, light float64) (*Notice, *fakeSight, *fakeIllumination) {
	sight := &fakeSight{clear: clear}
	illum := &fakeIllumination{level: light}

	return NewNotice(sight, illum, DefaultNoticeDials()), sight, illum
}

func TestNoticeSeesInTheOpen(t *testing.T) {
	n, sight, _ := newTestNotice(true, 0)

	w := &fakeWatcher{id: "n:1", x: 0, y: 0}
	q := &fakeQuarry{id: "p:1", x: 5, y: 0}

	n.Watch(w, q)

	noticed, watching := n.Noticed("n:1")
	require.True(t, watching, "Watch should register the watcher")
	assert.True(t, noticed, "a clear line at 5 tiles is inside the 12-tile radius")
	assert.Equal(t, 1, sight.calls, "Watch evaluates once immediately")
	assert.Equal(t, []string{"n:1"}, n.Aware())
	assert.Equal(t, 1, n.Notices())
}

// The negative control, and the reason ask 6 exists: a watcher that CANNOT see
// must not notice, and the report has to make the two cases distinguishable.
func TestNoticeBlockedLineOfSightDoesNotNotice(t *testing.T) {
	n, _, _ := newTestNotice(false, 0)

	n.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 5})

	noticed, _ := n.Noticed("n:1")
	assert.False(t, noticed, "a blocked line must not notice, however close")
	assert.Empty(t, n.Aware())
	assert.Equal(t, 0, n.Notices())

	report := n.Report()
	require.Len(t, report, 1)
	assert.False(t, report[0]["sees"].(bool))
	assert.InDelta(t, 5.0, report[0]["distance"].(float64), 0.001,
		"distance is reported even when nothing was seen, so a script can tell "+
			"'too far' from 'behind a wall'")
}

func TestNoticeOutOfRangeDoesNotNotice(t *testing.T) {
	n, sight, _ := newTestNotice(true, 0)

	n.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 13})

	noticed, _ := n.Noticed("n:1")
	assert.False(t, noticed, "13 tiles is outside the 12-tile radius")
	assert.Equal(t, 0, sight.calls,
		"distance is arithmetic and the raycast walks the grid, so the cheap "+
			"test must gate the dear one")
}

// The torch trade: R2 §3's dark-into-light advantage has a detection half, and
// this is it. A lit target is seen from beyond the radius that would hide it.
func TestNoticeLitTargetIsSeenFurther(t *testing.T) {
	dark, _, _ := newTestNotice(true, 0)
	dark.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 20})

	noticed, _ := dark.Noticed("n:1")
	require.False(t, noticed, "unlit at 20 tiles is beyond the 12-tile radius")

	lit, _, _ := newTestNotice(true, 0.5)
	lit.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 20})

	noticed, _ = lit.Noticed("n:1")
	assert.True(t, noticed,
		"a lit target at 20 tiles is inside 12 x 2 -- carrying a light is what "+
			"moves it")

	report := lit.Report()
	assert.InDelta(t, 24.0, report[0]["reach"].(float64), 0.001)
	assert.InDelta(t, 0.5, report[0]["light_at_quarry"].(float64), 0.001)
}

// The threshold is a boundary, so pin both sides of it rather than one.
func TestNoticeLitThresholdIsInclusive(t *testing.T) {
	dials := DefaultNoticeDials()

	just := NewNotice(&fakeSight{clear: true}, &fakeIllumination{level: dials.LitLevel}, dials)
	just.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 20})
	noticed, _ := just.Noticed("n:1")
	assert.True(t, noticed, "at exactly LitLevel the target counts as lit")

	under := NewNotice(&fakeSight{clear: true}, &fakeIllumination{level: dials.LitLevel - 0.01}, dials)
	under.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 20})
	noticed, _ = under.Noticed("n:1")
	assert.False(t, noticed, "a hair under LitLevel and the extra reach is gone")
}

// Without memory, breaking line of sight cancels awareness on the next tick
// and cover stops being a tactic and becomes a switch.
func TestNoticeRemembersAfterLosingSight(t *testing.T) {
	n, sight, _ := newTestNotice(true, 0)
	n.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 5})

	noticed, _ := n.Noticed("n:1")
	require.True(t, noticed)

	sight.clear = false

	// One re-evaluation: it can no longer see, but it has not forgotten.
	n.Advance(DefaultNoticeDials().ReEvaluateMinutes)

	noticed, _ = n.Noticed("n:1")
	assert.True(t, noticed, "memory keeps it coming after sight is lost")

	report := n.Report()
	assert.False(t, report[0]["sees"].(bool),
		"sees and noticed differ on purpose while memory is running")

	// Past the memory window it gives up.
	n.Advance(DefaultNoticeDials().MemoryMinutes)

	noticed, _ = n.Noticed("n:1")
	assert.False(t, noticed, "past MemoryMinutes the watcher forgets")
}

// The rate limit is in WORLD minutes and the clock compresses, so this pins
// the floor in the unit the dial is written in -- the confusion that produced
// M4.3a's 218 route solves.
func TestNoticeReEvaluateFloorHoldsTheSightTest(t *testing.T) {
	n, sight, _ := newTestNotice(true, 0)
	n.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 5})

	require.Equal(t, 1, sight.calls, "the immediate evaluation at Watch")

	floor := DefaultNoticeDials().ReEvaluateMinutes

	// Ten ticks that together fall just short of the floor: no new sight test.
	for i := 0; i < 10; i++ {
		n.Advance(floor / 20)
	}

	assert.Equal(t, 1, sight.calls, "under the floor the raycast must not run")

	// Crossing it runs exactly one more.
	n.Advance(floor)
	assert.Equal(t, 2, sight.calls)
	assert.Equal(t, 2, n.Checks())
}

// A wiring bug must fail safe AND fail visibly: nothing notices, and Wired()
// says why. The alternative default -- everything notices through walls --
// would read as a design choice in play rather than as a defect.
func TestNoticeUnwiredNoticesNothing(t *testing.T) {
	n := NewNotice(nil, nil, DefaultNoticeDials())
	assert.False(t, n.Wired())

	n.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 1})

	noticed, watching := n.Noticed("n:1")
	assert.True(t, watching, "the watch is still registered")
	assert.False(t, noticed, "but an unwired model can notice nothing")
	assert.Empty(t, n.Aware())
}

// The first provider rule: a collection needs a verb that fills it and one
// that empties it.
func TestNoticeWatchAndUnwatch(t *testing.T) {
	n, _, _ := newTestNotice(true, 0)

	n.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 1})
	n.Watch(&fakeWatcher{id: "n:2"}, &fakeQuarry{id: "p:1", x: 1})
	assert.Equal(t, 2, n.Count())

	assert.True(t, n.Unwatch("n:1"))
	assert.Equal(t, 1, n.Count())
	assert.False(t, n.Unwatch("n:1"), "unwatching twice is not an error, it is false")
	assert.False(t, n.Unwatch("nobody"))

	_, watching := n.Noticed("n:1")
	assert.False(t, watching)
}

// Watching twice replaces rather than duplicating: one watcher watches one
// thing, and a creature aware of two targets at once is M4.5's question.
func TestNoticeWatchReplaces(t *testing.T) {
	n, _, _ := newTestNotice(true, 0)

	w := &fakeWatcher{id: "n:1"}
	n.Watch(w, &fakeQuarry{id: "p:1", x: 1})
	n.Watch(w, &fakeQuarry{id: "p:2", x: 2})

	require.Equal(t, 1, n.Count())

	report := n.Report()
	require.Len(t, report, 1)
	assert.Equal(t, "p:2", report[0]["quarry"])
}

// Ranging a map is randomised in Go and what this decides feeds entity
// movement, which is inside the state digest.
func TestNoticeReportOrderIsStable(t *testing.T) {
	n, _, _ := newTestNotice(true, 0)

	for _, id := range []string{"n:9", "n:3", "n:11", "n:1"} {
		n.Watch(&fakeWatcher{id: id}, &fakeQuarry{id: "p:1", x: 1})
	}

	want := []string{"n:1", "n:11", "n:3", "n:9"}

	for i := 0; i < 8; i++ {
		got := make([]string, 0, 4)
		for _, row := range n.Report() {
			got = append(got, row["watcher"].(string))
		}

		require.Equal(t, want, got, "Report must be sorted on every call")
		require.Equal(t, want, n.Aware(), "Aware must be sorted too")
	}
}

// Ask 6 names four fields. Pin them, so a rename cannot silently break the
// assertion the milestone exists to make.
func TestNoticeReportCarriesTheSignedFields(t *testing.T) {
	n, _, _ := newTestNotice(true, 0.4)
	n.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 3})

	report := n.Report()
	require.Len(t, report, 1)

	for _, field := range []string{"sees", "distance", "light_at_quarry", "noticed"} {
		_, ok := report[0][field]
		assert.True(t, ok, "ask 6 names %q; the report must carry it", field)
	}
}

func TestNoticeDialsAreSettableWithinBounds(t *testing.T) {
	n, _, _ := newTestNotice(true, 0)

	assert.True(t, n.SetRadius(20))
	assert.InDelta(t, 20.0, n.Dials().Radius, 0.001)
	assert.False(t, n.SetRadius(0), "a zero radius is not a radius")
	assert.False(t, n.SetRadius(-1))

	assert.True(t, n.SetLitLevel(0.75))
	assert.InDelta(t, 0.75, n.Dials().LitLevel, 0.001)
	assert.False(t, n.SetLitLevel(-0.1))
	assert.False(t, n.SetLitLevel(1.1))
}

// Moving the radius must change the answer, not just the reported number --
// the third provider rule, checked rather than assumed.
func TestNoticeRadiusChangeMovesTheVerdict(t *testing.T) {
	n, _, _ := newTestNotice(true, 0)
	n.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 15})

	noticed, _ := n.Noticed("n:1")
	require.False(t, noticed, "15 tiles is outside the default 12")

	require.True(t, n.SetRadius(20))
	n.Advance(DefaultNoticeDials().ReEvaluateMinutes)

	noticed, _ = n.Noticed("n:1")
	assert.True(t, noticed, "widening the radius must actually widen it")
}

// AwarePairs is what lets something OUTSIDE act on awareness. M4.3b shipped
// without it and the milestone was hollow: nothing in a non-harness build ever
// read the verdict, so a wolf that had seen the player stood there. These pin
// the shape the game screen depends on.
func TestNoticeAwarePairsCarryWhatIsNeededToAct(t *testing.T) {
	n, sight, _ := newTestNotice(true, 0)

	seer := &fakeWatcher{id: "n:1", x: 0, y: 0}
	blind := &fakeWatcher{id: "n:2", x: 0, y: 0}
	target := &fakeQuarry{id: "p:1", x: 3, y: 0}

	n.Watch(seer, target)

	pairs := n.AwarePairs()
	require.Len(t, pairs, 1, "one watcher can see")
	assert.Equal(t, "n:1", pairs[0].Watcher.WatcherID())
	assert.Equal(t, "p:1", pairs[0].Target.QuarryID(),
		"the pair must carry WHAT it is aware of, or the caller cannot start a chase")

	// A watcher that cannot see must not appear, however close it is.
	sight.clear = false
	n.Watch(blind, target)

	for _, p := range n.AwarePairs() {
		assert.NotEqual(t, "n:2", p.Watcher.WatcherID(),
			"a blocked watcher must never be handed out as aware")
	}
}

// AwarePairs and Aware must never disagree: one is for acting, one is for
// reporting, and a build where a script sees an aware watcher that the game
// never chases would be indistinguishable from the bug this fixes.
func TestNoticeAwarePairsAgreeWithAware(t *testing.T) {
	n, sight, _ := newTestNotice(true, 0)

	for _, id := range []string{"n:3", "n:1", "n:2"} {
		n.Watch(&fakeWatcher{id: id}, &fakeQuarry{id: "p:1", x: 2})
	}

	ids := make([]string, 0, 3)
	for _, p := range n.AwarePairs() {
		ids = append(ids, p.Watcher.WatcherID())
	}

	assert.Equal(t, n.Aware(), ids, "same set, same order")

	// And when nothing can see, both must be empty rather than one of them.
	sight.clear = false
	loseSightAndForget(n)

	assert.Empty(t, n.Aware())
	assert.Empty(t, n.AwarePairs())
}

// loseSightAndForget steps far enough for a watcher that can no longer see its
// target to stop coming for it.
//
// It takes TWO steps and that is the whole reason it exists. Memory only
// begins accruing once an evaluation has flipped `sees` to false, so a single
// Advance of ReEvaluateMinutes+MemoryMinutes does not forget: the first
// evaluation happens at the END of that step with sinceSeen still zero. Two
// call sites got this wrong before it was written down once -- this one and
// act 4 of playtest/spawns_test.go.
func loseSightAndForget(n *Notice) {
	d := DefaultNoticeDials()

	n.Advance(d.ReEvaluateMinutes) // an evaluation runs; sees goes false
	n.Advance(d.MemoryMinutes)     // now the memory window can elapse
}
