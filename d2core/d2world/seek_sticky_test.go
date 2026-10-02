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

// --- The second review of R2 (wt-rev-r2b, 1 Oct 2026) ------------------------

// ownTargetGoes is the reviewer's probe: a wolf watching a villager one tile
// off, him four; then she is gone -- dead by her body, or protected -- and he
// comes within the margin of where she stands (he 2 off, she 1). It returns
// what the wolf watches after the next look.
func ownTargetGoes(t *testing.T, how string) string {
	t.Helper()

	f := newResolverFight(t, 1462)
	f.c.SetPlayer("p:1")
	vb := &fakeBody{health: 60, maxHealth: 60}
	f.bodies.known["v:1"] = vb

	w := newSeekWorld(t, f.c, nil)
	tx, ty := f.target.QuarryAt()
	villager := &fakeQuarry{id: "v:1", x: tx + 3, y: ty}
	w.quarries = []Quarry{f.target, villager}
	w.notice.Watch(&fakeWatcher{id: "m:5", x: tx + 4, y: ty}, villager)
	w.minute()
	require.Equal(t, "v:1", w.targetOf("m:5"), "the fixture: the wolf on her")

	f.target.x = tx + 2

	switch how {
	case "dead":
		vb.health = 0
	case "protected":
		f.c.SetProtected(func(id string) bool { return id == "v:1" })
	}

	w.minute()

	return w.targetOf("m:5")
}

// C1: a watch's own target that is dead or protected holds nothing: the
// nearest eligible takes the watch, within the margin or not. The control is
// the same villager unchanged, which holds the watch (she is the nearer).
// (Source controls: current-skips-dead, current-skips-protected -- the
// watch's own target let past the gate -- red here.)
func TestSeekLeavesAnOwnTargetThatIsGone(t *testing.T) {
	require.Equal(t, "p:1", ownTargetGoes(t, "dead"), "dead by her body, she holds nothing")
	require.Equal(t, "p:1", ownTargetGoes(t, "protected"), "protected, she holds nothing")
	require.Equal(t, "v:1", ownTargetGoes(t, ""), "the control: alive and unprotected, she keeps the watch")
}

// C2: a watch's own target out of reach holds nothing either. The wolf
// watches a villager 12.5 off (past the unlit reach of 12); he is 12 off, in
// reach, nearer by less than the margin: he takes the watch. The control:
// the villager inside the reach (11.9), and the margin holds her. (Source
// control: held-out-of-reach -- the own target counted before the reach
// test -- red here.)
func TestSeekDoesNotHoldATargetOutOfReach(t *testing.T) {
	run := func(vx float64) string {
		w := newSeekWorld(t, nil, nil)
		him, villager := &fakeQuarry{id: "p:1", y: 12}, &fakeQuarry{id: "v:1", x: vx}
		w.quarries = []Quarry{him, villager}
		w.notice.Watch(&fakeWatcher{id: "m:1"}, villager)
		w.minute()

		return w.targetOf("m:1") + "/" + w.row(t, "m:1").reason
	}

	require.Equal(t, "p:1/living", run(12.5), "out of reach, she holds nothing")
	require.Equal(t, "v:1/living", run(11.9), "the control: in reach (and the nearer), she keeps it")
}

// C2: the margin is "nearer by AT LEAST SwitchMarginTiles": exactly 1.5 nearer
// switches. (Source control: margin-strict, < for <=: red here.)
func TestSeekSwitchesAtExactlyTheMargin(t *testing.T) {
	w, _, _ := stickyWorld(t, 0, 5-1.5) // him at 5, she at 3.5: exactly the margin
	w.minute()
	require.Equal(t, "v:1", w.targetOf("m:1"), "exactly the margin nearer: she takes it")

	// The control: a hair short of the margin, and he keeps it.
	w, _, _ = stickyWorld(t, 0, 5-1.5+0.01)
	w.minute()
	require.Equal(t, "p:1", w.targetOf("m:1"))
}

// B1 (the second review): a watch whose own target is dead by its body is no
// AWARE pair -- no chase or fight starts on the corpse -- and the watch stays
// (its memory and counters). The control: alive, the pair is handed out.
func TestAWatchOnTheDeadIsNoAwarePair(t *testing.T) {
	for _, alive := range []bool{true, false} {
		f := newResolverFight(t, 1462)
		vb := &fakeBody{health: 60, maxHealth: 60}
		f.bodies.known["v:1"] = vb

		f.notice.Watch(&fakeWatcher{id: "m:7", x: 60, y: 40}, &fakeQuarry{id: "v:1", x: 61, y: 40})
		f.notice.Advance(1.0)

		if !alive {
			vb.health = 0
		}

		noticed, watching := f.notice.Noticed("m:7")
		require.True(t, noticed && watching, "alive=%v: the watch itself stays as it was", alive)

		var pairs []string
		for _, p := range f.notice.AwarePairs() {
			pairs = append(pairs, p.Watcher.WatcherID())
		}

		if alive {
			require.Contains(t, pairs, "m:7", "the control: alive, she is coming for")
			require.Contains(t, f.notice.Aware(), "m:7")
		} else {
			require.NotContains(t, pairs, "m:7", "dead by her body, nothing comes for her")
			require.NotContains(t, f.notice.Aware(), "m:7", "Aware and AwarePairs agree")
		}
	}
}

// B2 (the second review): the reviewer's long-step probe. Eight hunters
// chase him; a villager steps out two tiles from them; one long step (a
// sleep's hour) lets every Seek row look in ONE frame, and all eight watches
// move. The game restarts their chases under the frame's budget: at most
// RechasesPerFrame (1) in that frame, the rest one a frame after, all eight
// on her within eight frames. The control: the budget off (0), and all eight
// solve in the one frame (the review measured 8; worst solve 42 ms).
func TestRechaseIsCappedAFrame(t *testing.T) {
	run := func(perFrame int) (inTheLongFrame, framesToAll int) {
		w := newSeekWorld(t, nil, nil)
		p, _ := newTestPursuit(true)
		t.Cleanup(p.Close)

		p.dials.RechasesPerFrame = perFrame
		him := &fakeQuarry{id: "p:1", x: 10}
		w.quarries = []Quarry{him}

		for i := 0; i < 8; i++ {
			w.notice.Watch(&rechaseBenchHunter{id: fmt.Sprintf("m:%d", i), y: float64(i) * 0.1}, him)
		}

		// The game's order: Pursuit, the notice model, Seek, the chases.
		frame := func(dt float64) int {
			p.Advance(dt)
			w.notice.Advance(dt)
			w.seek.Advance(dt)

			r0 := p.rechases

			for _, pair := range w.notice.AwarePairs() {
				h := pair.Watcher.(Hunter)
				if whom, chasing := p.ChasingWhom(h.HunterID()); chasing && whom == pair.Target.QuarryID() {
					continue
				}

				p.Rechase(h, pair.Target)
			}

			return p.rechases - r0
		}

		for i := 0; i < 72; i++ {
			frame(1.0 / 24)
		}

		require.Equal(t, 8, p.Count(), "the fixture: eight chases on him")

		w.quarries = append(w.quarries, &fakeQuarry{id: "v:1", x: 2})
		r0 := w.seek.retargets
		inTheLongFrame = frame(60)
		require.Equal(t, 8, w.seek.retargets-r0, "the fixture: all eight watches moved in the long step")

		onHer := func() int {
			n := 0

			for i := 0; i < 8; i++ {
				if whom, _ := p.ChasingWhom(fmt.Sprintf("m:%d", i)); whom == "v:1" {
					n++
				}
			}

			return n
		}

		for framesToAll = 0; onHer() < 8 && framesToAll < 48; framesToAll++ {
			frame(1.0 / 24)
		}

		return inTheLongFrame, framesToAll
	}

	in, after := run(DefaultPursuitDials().RechasesPerFrame)
	require.Equal(t, 1, in, "one re-chase solve in the long step's frame")
	require.Equal(t, 7, after, "and the other seven one a frame after")

	in, after = run(0)
	require.Equal(t, 8, in, "the control: no budget, all eight solve in the one frame")
	require.Zero(t, after)
}

// C2 (the second review): a fighter's dwell runs down while it fights, as
// every row's does; the fight does not freeze or renew it, so when the
// fighter's part ends the margin alone governs its next look. (Source
// control: fighter-dwell-frozen -- the fighter's look renews the dwell --
// red here.)
func TestSeekAFightersDwellRunsDown(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "m:2", 5000, Profile{})
	f.open(t)
	require.True(t, f.c.engaged("m:2"), "the fixture: m:2 is his enemy")

	w := newSeekWorld(t, f.c, nil)
	w.quarries = []Quarry{f.target}
	w.minute()
	require.Equal(t, SeekFighting, w.row(t, "m:2").reason)

	w.row(t, "m:2").dwell = DefaultSeekDials().DwellMinutes // a switch just before the fight

	for i := 0; i < 3; i++ {
		w.minute()
	}

	require.True(t, f.c.engaged("m:2"), "still fighting")
	require.Zero(t, w.row(t, "m:2").dwell, "three minutes on, a two-minute dwell is spent, fight or no fight")
}
