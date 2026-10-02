package d2world

// THE PER-FRAME SOLVE BUDGET (2 Oct 2026). Pursuit solves at most
// SolvesPerFrame routes between two Advances -- re-paths, first routes, the
// world's chase starts and its re-chases together -- and serves what it
// cannot afford on later frames, oldest first, with the queue derived from the
// chases the world file already carries. Each assertion here had a negative
// control at the moment it was written (the notes: docs/m4.6-world-save-notes.md,
// "The per-frame A* budget").

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// budgetFrame is one frame in the game's order -- Pursuit steps, then the
// world starts or moves its chases (Game.startChasesForTheAware), then what
// they left of the budget goes to the chases owing a first route
// (Pursuit.ServeQueued, at the end of that loop) -- and returns the routes
// solved in it.
func budgetFrame(p *Pursuit, dt float64, after func()) int {
	s0 := p.solves

	p.Advance(dt)

	if after != nil {
		after()
	}

	p.ServeQueued()

	return p.solves - s0
}

// A pack of eight noticing him in one frame: the frame solves the budget's
// worth and the rest stand, live chases owing their route, served two a frame
// after -- one by Advance, one by ServeQueued from the unit Advance kept back
// and no re-chase used -- oldest first. Control: no budget (SolvesPerFrame
// 0), all eight in the one frame. (Before the second review's C the kept-back
// unit went unused and the six took six frames.)
func TestABurstOfStartsIsSpreadByTheBudget(t *testing.T) {
	run := func(solvesPerFrame int) (first, framesToAll, worst int, order []string, hunters []*fakeHunter, p *Pursuit) {
		p, _ = newTestPursuit(true)
		t.Cleanup(p.Close)
		p.dials.SolvesPerFrame = solvesPerFrame

		him := &fakeQuarry{id: "p:1", x: 20}

		for i := 0; i < 8; i++ {
			hunters = append(hunters, &fakeHunter{id: fmt.Sprintf("m:%d", 7-i), y: float64(i)})
		}

		first = budgetFrame(p, 1.0/24, func() {
			for _, h := range hunters {
				require.True(t, p.Rechase(h, him), "a start is never refused: the chase is live")
			}
		})

		require.Equal(t, 8, p.Count(), "every start is a live chase from its frame")
		require.Len(t, p.ChasersOf("p:1"), 8, "so the combat status reads him chased at once")

		seen := map[string]bool{}

		for _, h := range hunters {
			if h.routes > 0 {
				seen[h.id] = true
			}
		}

		for framesToAll = 0; p.Queued() > 0 && framesToAll < 50; framesToAll++ {
			n := budgetFrame(p, 1.0/24, nil)
			if n > worst {
				worst = n
			}

			var now []string

			for _, h := range hunters {
				if h.routes > 0 && !seen[h.id] {
					seen[h.id] = true
					now = append(now, h.id)
				}
			}

			sort.Strings(now) // two a frame: within one frame, by id
			order = append(order, now...)
		}

		return first, framesToAll, worst, order, hunters, p
	}

	first, after, worst, order, hunters, p := run(DefaultPursuitDials().SolvesPerFrame)
	require.Equal(t, 2, first, "the budget's two in the burst frame")
	require.Equal(t, 3, after, "the other six two a frame")
	require.Equal(t, 2, worst, "never more than the budget in a frame after")
	require.Equal(t, []string{"m:0", "m:1", "m:2", "m:3", "m:4", "m:5"}, order,
		"equal waits are served in hunter order (m:7 and m:6 were started first, in the burst)")
	require.GreaterOrEqual(t, p.queued, 6)

	for _, h := range hunters {
		require.Equal(t, 1, h.routes, "%s: one route each, no solve paid twice", h.id)
	}

	first, after, _, _, _, _ = run(0)
	require.Equal(t, 8, first, "the control: no budget, all eight in the one frame")
	require.Zero(t, after)
}

// A queued chaser is never starved. Ten chases are due every frame (the quarry
// moves two tiles a frame, a frame is two world minutes: every chase wants a
// re-path every frame) and Advance may serve one: oldest first makes it a
// round, so no chase waits more than ten frames for its route. Control (a
// source mutation): order the wanted solves by hunter id alone -- the last ids
// are never served.
func TestAQueuedChaserIsServedWithinABoundedWait(t *testing.T) {
	p, _ := newTestPursuit(true)
	t.Cleanup(p.Close)

	q := &fakeQuarry{id: "p:1", x: 10}

	var hunters []*fakeHunter

	for i := 0; i < 10; i++ {
		h := &fakeHunter{id: fmt.Sprintf("m:%02d", i), y: float64(i)}
		hunters = append(hunters, h)
		p.Chase(h, q) // a script's chases: solved at once
	}

	last := map[string]int{}
	worstWait := 0

	for f := 1; f <= 100; f++ {
		q.x += 2

		before := map[string]int{}
		for _, h := range hunters {
			before[h.id] = h.routes
		}

		n := budgetFrame(p, 2.0, nil)
		require.LessOrEqual(t, n, p.advanceBudget(), "frame %d", f)

		for _, h := range hunters {
			if h.routes > before[h.id] {
				last[h.id] = f
			}

			if w := f - last[h.id]; w > worstWait {
				worstWait = w
			}
		}
	}

	require.Equal(t, 1, p.advanceBudget(), "the fixture: Advance serves one a frame")
	require.LessOrEqual(t, worstWait, 9, "no chase waits ten frames or more: a round of ten, one a frame")

	for _, h := range hunters {
		require.GreaterOrEqual(t, h.routes, 10, "%s was served its share", h.id)
	}
}

// What has no route at all is served before what is walking a stale one: a
// chase begun past the budget goes ahead of a re-path that has waited longer.
// Control (a source mutation): drop the first-routes key -- the re-path, older,
// is served and the new chase stands another frame.
func TestFirstRoutesComeBeforeRepaths(t *testing.T) {
	p, _ := newTestPursuit(true)
	t.Cleanup(p.Close)

	q := &fakeQuarry{id: "p:1", x: 10}
	old := &fakeHunter{id: "m:0"}
	p.Chase(old, q)

	// Spend the frame: two script chases elsewhere.
	p.Advance(1.0 / 24)
	p.Chase(&fakeHunter{id: "x:1", y: 5}, &fakeQuarry{id: "v:1", x: -10})
	p.Chase(&fakeHunter{id: "x:2", y: 6}, &fakeQuarry{id: "v:2", x: -10})

	fresh := &fakeHunter{id: "m:9", y: 1}
	require.True(t, p.Rechase(fresh, q))
	require.Equal(t, 1, p.Queued(), "the fixture: the start owes its route")
	require.Zero(t, fresh.routes, "and its hunter stands")

	// The old chase is due a re-path and has waited far longer.
	q.x += 3
	old.following = false

	routes := old.routes
	budgetFrame(p, 5.0, nil)

	require.Equal(t, 1, fresh.routes, "the start is served first")
	require.Equal(t, routes, old.routes, "the re-path waits a frame")

	budgetFrame(p, 1.0/24, nil)
	require.Equal(t, routes+1, old.routes, "and is served the next")
}

// A re-chase is never starved by re-paths: Advance keeps RechasesPerFrame of
// the budget back, so a watch Seek moved is followed in the frame it is asked,
// however busy the frame. Control (a source mutation): keep nothing back --
// the re-paths take the whole budget every frame and the re-chase is refused
// for good.
func TestAReChaseFindsItsUnitEveryFrame(t *testing.T) {
	p, _ := newTestPursuit(true)
	t.Cleanup(p.Close)

	q := &fakeQuarry{id: "p:1", x: 10}
	her := &fakeQuarry{id: "v:1", x: -10}

	var hunters []*fakeHunter

	for i := 0; i < 10; i++ {
		h := &fakeHunter{id: fmt.Sprintf("m:%02d", i), y: float64(i)}
		hunters = append(hunters, h)
		p.Chase(h, q)
	}

	moved := 0

	for f := 1; f <= 5; f++ {
		q.x += 2

		budgetFrame(p, 2.0, func() {
			h := hunters[f]
			if p.Rechase(h, her) {
				moved++
			}
		})

		require.LessOrEqual(t, p.solvedThisFrame, p.dials.SolvesPerFrame, "frame %d: within the budget", f)
	}

	require.Equal(t, 5, moved, "each re-chase is made in the frame it is asked")
	require.Equal(t, 5, p.rechases)
}

// A long step (a sleep's or a labour's ten minutes) makes every chase due at
// once; the budget spreads them. Control: no budget, all twelve in the frame.
func TestALongStepIsSpreadByTheBudget(t *testing.T) {
	run := func(solvesPerFrame int) int {
		p, _ := newTestPursuit(true)
		t.Cleanup(p.Close)
		p.dials.SolvesPerFrame = solvesPerFrame

		q := &fakeQuarry{id: "p:1", x: 10}

		for i := 0; i < 12; i++ {
			p.Chase(&fakeHunter{id: fmt.Sprintf("m:%02d", i), y: float64(i)}, q)
		}

		q.x += 5

		return budgetFrame(p, 10, nil)
	}

	require.Equal(t, 1, run(DefaultPursuitDials().SolvesPerFrame), "Advance's share of the budget in the long frame")
	require.Equal(t, 12, run(0), "the control: no budget, every chase in the one frame")
}

// THE QUEUE IS DERIVED, SO A RESUME IS EXACT. The b2b world (the M4.6 B2b
// fixture) runs with the world's own Rechase and a budget of one -- every
// pack that notices him queues -- and is saved at the first tick a chase owes
// its route; the resumed world shows tick for tick what the world that ran on
// showed. Control (a source mutation): key the queue on the unsaved fromWatch
// as well as on no solve yet -- the resume diverges.
func TestAQueuedChaseResumesExactly(t *testing.T) {
	a := b2bNewWorld(t, 1462, "p:0")
	a.rechase = true
	a.pursuit.dials.SolvesPerFrame = 1

	saved := false

	for i := 0; i < 20000 && !saved; i++ {
		a.tick()

		if a.pursuit.Queued() > 0 {
			saved = true
		}
	}

	require.True(t, saved, "the fixture: a tick at which a chase owes its route")

	sv := a.save(t)

	queuedAtSave := 0

	for _, c := range sv.snap.Pursuit.Chases {
		if c.Solves == 0 {
			queuedAtSave++
		}
	}

	require.Positive(t, queuedAtSave, "the saved chases carry the queue (solves 0)")

	want := b2bRecord(t, a, b2bAdvanceTicks)

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)

	b.rechase = true
	b.pursuit.dials.SolvesPerFrame = 1

	got := b2bRecord(t, b, b2bAdvanceTicks)
	require.Equal(t, -1, b2bFirstDifference(want, got), "the resumed world diverged")
}
