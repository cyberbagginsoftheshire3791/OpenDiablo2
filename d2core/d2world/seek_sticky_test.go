package d2world

// THE R2 REVIEW'S FIXES, IN UNIT FORM (1 Oct 2026): a sticky target (B1), the
// dead-body gate (B3), the stagger's absolute phases (C1) and the tie-break
// (C2). Each assertion has its control beside it; where the control breaks
// the CODE (the gate deleted, the tie-break deleted, the round-robin put
// back) it is a source mutation run by a script, its red and green logs kept
// in strigoi-harness-runs\wt-r2\fix\logs (the R2 fix note lists each).

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// flicker is the review's probe made a test: a wolf at the origin between
// him and a villager at near-equal distance, he stepping 0.1 tile back and
// forth each minute so the nearer of the two alternates. It returns the
// watch's target after each minute and the retargets.
func flicker(t *testing.T, margin, dwell float64) ([]string, int) {
	t.Helper()

	w := newSeekWorld(t, nil, nil)
	w.seek.dials.SwitchMarginTiles, w.seek.dials.DwellMinutes = margin, dwell

	him, villager := &fakeQuarry{id: "p:1", x: 5}, &fakeQuarry{id: "v:1", y: 5.05}
	w.quarries = []Quarry{him, villager}
	w.notice.Watch(&fakeWatcher{id: "m:1"}, him)

	var targets []string

	for i := 0; i < 10; i++ {
		him.x = 5.0
		if i%2 == 0 {
			him.x = 5.1
		}

		w.minute()
		targets = append(targets, w.targetOf("m:1"))
	}

	return targets, w.seek.retargets
}

// B1: two quarries 0.05 tile apart in distance, the nearer alternating each
// minute. Sticky, the watch stays where it is (reason held); the control is
// R2 as first shipped (no margin, no dwell), which flips on every look.
func TestSeekHoldsASeenTargetAgainstANearEqualOne(t *testing.T) {
	targets, retargets := flicker(t, DefaultSeekDials().SwitchMarginTiles, DefaultSeekDials().DwellMinutes)
	require.Zero(t, retargets, "a 0.05-tile difference moved the watch: %v", targets)

	for i, tg := range targets {
		require.Equal(t, "p:1", tg, "minute %d", i)
	}

	// THE CONTROL: no margin and no dwell -- the strict nearest -- and the
	// watch flips on every look (the review's 10 of 10).
	targets, retargets = flicker(t, 0, 0)
	require.Equal(t, 10, retargets, "the control: the strict nearest flips every look: %v", targets)
}

// stickyWorld is a wolf at the origin watching him at (5,0), with a villager.
func stickyWorld(t *testing.T, vx, vy float64) (*seekWorld, *fakeQuarry, *fakeQuarry) {
	t.Helper()

	w := newSeekWorld(t, nil, nil)
	him, villager := &fakeQuarry{id: "p:1", x: 5}, &fakeQuarry{id: "v:1", x: vx, y: vy}
	w.quarries = []Quarry{him, villager}
	w.notice.Watch(&fakeWatcher{id: "m:1"}, him)

	return w, him, villager
}

// B1, the margin: a villager nearer by less than SwitchMarginTiles is no
// reason to leave him (held); nearer by more, she takes the watch.
func TestSeekSwitchesOnlyForAQuarryNearerByTheMargin(t *testing.T) {
	m := DefaultSeekDials().SwitchMarginTiles
	require.Equal(t, 1.5, m, "the dial: Pursuit's RepathTiles")

	w, _, _ := stickyWorld(t, 0, 5-m+0.1) // 1.4 nearer
	w.minute()
	require.Equal(t, "p:1", w.targetOf("m:1"), "nearer by less than the margin: he keeps the watch")
	require.Equal(t, SeekHeld, w.row(t, "m:1").reason)
	require.Equal(t, 1, w.seek.holds)
	require.Zero(t, w.seek.retargets)

	// THE CONTROL: the same villager 0.2 nearer still, past the margin.
	w, _, _ = stickyWorld(t, 0, 5-m-0.1) // 1.6 nearer
	w.minute()
	require.Equal(t, "v:1", w.targetOf("m:1"), "nearer by more than the margin: she takes it")
	require.Equal(t, SeekLiving, w.row(t, "m:1").reason)
	require.Equal(t, 1, w.seek.retargets)
	require.Zero(t, w.seek.holds)
}

// B1, the dwell: after a switch the row keeps its new target while it is seen
// for DwellMinutes, even against a quarry nearer by the margin; then the
// nearer takes it. The control: no dwell, and it turns back at the next look.
func TestSeekDwellsOnANewTarget(t *testing.T) {
	run := func(dwell float64) []string {
		w, him, _ := stickyWorld(t, 0, 2) // she is 3 nearer: a switch
		w.seek.dials.DwellMinutes = dwell

		w.minute()
		require.Equal(t, "v:1", w.targetOf("m:1"), "the fixture: the switch")
		// The switch came at the minute's first frame; 23 frames have run since.
		require.InDelta(t, math.Max(0, dwell-23.0/24), w.row(t, "m:1").dwell, 1e-9, "the switch starts the dwell")

		him.x = 0.2 // now he is 1.8 nearer than she is: past the margin

		var out []string

		for i := 0; i < 3; i++ {
			w.minute()
			out = append(out, fmt.Sprintf("%s/%s", w.targetOf("m:1"), w.row(t, "m:1").reason))
		}

		return out
	}

	// Default dwell 2.0: the looks at +1 and +2 minutes hold her (the dwell
	// still above zero at +1, and at 0 at +2 -- so the +2 look switches).
	got := run(DefaultSeekDials().DwellMinutes)
	require.Equal(t, []string{"v:1/held", "p:1/living", "p:1/living"}, got)

	// THE CONTROL: no dwell, and he takes it back at once.
	got = run(0)
	require.Equal(t, []string{"p:1/living", "p:1/living", "p:1/living"}, got)
}

// B1: stickiness is for a target still SEEN. A target that goes behind a wall
// is no reason to wait, dwell or no dwell: the nearest in sight takes the
// watch at once. The control: the same wall lifted, and the dwell holds.
func TestSeekDoesNotHoldATargetItCannotSee(t *testing.T) {
	for _, blocked := range []bool{true, false} {
		w, him, _ := stickyWorld(t, 0, 2)
		w.minute()
		require.Equal(t, "v:1", w.targetOf("m:1"), "the fixture: the switch, and its dwell")

		him.x = 1 // he is 1 nearer: under the margin, and inside the dwell

		if blocked {
			w.sight.blocked[[2]float64{0, 2}] = true // she goes behind a wall
		}

		w.minute()

		if blocked {
			require.Equal(t, "p:1", w.targetOf("m:1"), "out of sight, she is no reason to wait")
			require.Equal(t, SeekLiving, w.row(t, "m:1").reason)
		} else {
			require.Equal(t, "v:1", w.targetOf("m:1"), "the control: in sight, the dwell holds her")
			require.Equal(t, SeekHeld, w.row(t, "m:1").reason)
		}
	}
}

// B3: a quarry dead by its body is never a candidate. A villager whose body
// lies at 0 health two tiles from the wolf, him five: the wolf keeps him. The
// control: the same villager alive, and she takes the watch. (The source
// control deletes the gate in eligible: red here.)
func TestSeekPassesOverTheDead(t *testing.T) {
	run := func(health int) string {
		f := newResolverFight(t, 1462)
		f.c.SetPlayer("p:1")
		f.bodies.known["v:1"] = &fakeBody{health: health, maxHealth: 60}

		w := newSeekWorld(t, f.c, nil)
		tx, ty := f.target.QuarryAt()
		w.quarries = []Quarry{f.target, &fakeQuarry{id: "v:1", x: tx + 3, y: ty}}
		w.notice.Watch(&fakeWatcher{id: "m:5", x: tx + 5, y: ty}, f.target)
		w.minute()

		return w.targetOf("m:5")
	}

	require.Equal(t, "p:1", run(0), "a body at 0 is no quarry: the wolf keeps him")
	require.Equal(t, "v:1", run(60), "the control: alive, the villager 3 nearer takes the watch")
}

// C2: among quarries at an equal distance the watch's own target comes first
// (then ids). With no margin and no dwell, so the tie-break alone decides: a
// wolf watching v:2 with him (p:1, the lower id) at the same distance keeps
// v:2. (The source control deletes the tie-break: red here.)
func TestSeekBreaksATieForItsOwnTarget(t *testing.T) {
	w := newSeekWorld(t, nil, nil)
	w.seek.dials.SwitchMarginTiles, w.seek.dials.DwellMinutes = 0, 0

	him, v2 := &fakeQuarry{id: "p:1", x: 5}, &fakeQuarry{id: "v:2", x: -5}
	w.quarries = []Quarry{him, v2}
	w.notice.Watch(&fakeWatcher{id: "m:1"}, v2)
	w.minute()

	require.Equal(t, "v:2", w.targetOf("m:1"), "an equal distance: its own target first")
	require.Equal(t, SeekLiving, w.row(t, "m:1").reason)
	require.Zero(t, w.seek.retargets)

	// The test's own control: him a hair nearer, and he takes it (the strict
	// nearest, the dials at 0).
	him.x = 4.99
	w.minute()
	require.Equal(t, "p:1", w.targetOf("m:1"))
}

// C1: phases are absolute frames. Two watchers made at frame 0 and two made
// 23 night frames later: at most one look in any frame (ceil(4/24)). The
// review measured 2 under the round-robin by creation count. (The source
// control puts the round-robin back: red here.)
func TestSeekStaggersGroupsMadeOnDifferentFrames(t *testing.T) {
	w := newSeekWorld(t, nil, nil)
	him := &fakeQuarry{id: "p:1", x: 5}
	w.quarries = []Quarry{him}

	frame := func() int {
		l0 := w.seek.looks
		w.notice.Advance(1.0 / 24)
		w.seek.Advance(1.0 / 24)

		return w.seek.looks - l0
	}

	var per []int

	w.notice.Watch(&fakeWatcher{id: "m:1"}, him)
	w.notice.Watch(&fakeWatcher{id: "m:2"}, him)

	for i := 0; i < 23; i++ {
		per = append(per, frame())
	}

	w.notice.Watch(&fakeWatcher{id: "m:3"}, him)
	w.notice.Watch(&fakeWatcher{id: "m:4"}, him)

	for i := 0; i < 48; i++ {
		per = append(per, frame())
	}

	total := 0

	for i, n := range per {
		require.LessOrEqual(t, n, 1, "frame %d: %d looks of 4 watchers over 24 phases (%v)", i, n, per)
		total += n
	}

	require.Equal(t, 2+2*2+2*2, total, "every row looked once a minute: %v", per)
	require.Equal(t, 4, w.seek.slots, "one phase given out per row")
}

// B2: a retarget costs the chase one solve, and Pursuit counts it. A chase
// started, started again on the same quarry, then on another: three solves,
// one of them a rechase. (The source control drops the count: red here.)
func TestPursuitCountsTheRechaseSolves(t *testing.T) {
	p, _ := newTestPursuit(true)
	t.Cleanup(p.Close)

	h := &fakeHunter{id: "n:1"}
	him, her := &fakeQuarry{id: "p:1", x: 9}, &fakeQuarry{id: "v:1", y: 9}

	p.Chase(h, him)
	p.Chase(h, him)
	require.Zero(t, p.rechases, "a chase restarted on its own quarry is no retarget")

	p.Chase(h, her)
	require.Equal(t, 1, p.rechases, "a chase restarted on another quarry: one rechase solve")
	require.Equal(t, 3, p.Solves(), "and it is one of the solves")
	require.Equal(t, 1, p.HarnessState()["rechase_solves"])

	// A released chase started again is a new chase, not a rechase.
	p.Release("n:1")
	p.Chase(h, him)
	require.Equal(t, 1, p.rechases)
}
