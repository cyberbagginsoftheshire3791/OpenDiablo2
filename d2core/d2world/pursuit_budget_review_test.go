package d2world

// THE PER-FRAME BUDGET'S SECOND REVIEW (2 Oct 2026; probes in
// strigoi-wt-rev-budget, zz_review_budget_test.go). Its findings, folded in as
// tests: A (a world chase's from_watch is saved, world file version 5), B (the
// reserve is max(1, RechasesPerFrame) at a budget of 2 or more), C (what the
// world leaves of the reserve goes to the chases owing a first route), and the
// four mutants its probes killed and the suite did not. Each assertion here
// was shown red under its mutant (docs/m4.6-world-save-notes.md, "The second
// review of the per-frame budget").

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// fwResolver resolves the from_watch scenario's hunters, quarries and player.
type fwResolver struct {
	hunters  map[string]*rechaseBenchHunter
	quarries map[string]*fakeQuarry
	player   *fakeQuarry
}

func (r fwResolver) Watcher(id string) (Watcher, bool) {
	h, ok := r.hunters[id]
	if !ok {
		return nil, false
	}

	return h, true
}

func (r fwResolver) Quarry(ref string) (Quarry, bool) {
	if ref == PlayerRef {
		return r.player, true
	}

	q, ok := r.quarries[ref]
	if !ok {
		return nil, false
	}

	return q, true
}

// fromWatchAfterAForget is the review's A: a wolf's world chase on villager A
// (legal to save: he is not chased) is saved and, when resume is set, put back
// through Snapshot, JSON and Restore into a new Pursuit (mutate, when given,
// alters the snapshot between); Seek moves the watch to villager B; the frame's
// budget defers the re-chase (Advance serves the wolf's re-path, a start takes
// the kept-back unit); the watch forgets B. It returns who chases A then: the
// game that ran on releases the wolf (a world chase whose watch forgot).
func fromWatchAfterAForget(t *testing.T, resume bool, mutate func(*PursuitSnapshot)) []string {
	t.Helper()

	n, sight, _ := newTestNotice(true, 0)
	p, _ := newTestPursuit(true)

	a := &fakeQuarry{id: "v:1", x: 3}
	b := &fakeQuarry{id: "v:2", x: -3}
	him := &fakeQuarry{id: "p:1", x: 9}
	wolf := &rechaseBenchHunter{id: "n:2"}

	n.Watch(wolf, a)
	require.True(t, p.Rechase(wolf, a))
	p.Advance(0.01)

	if resume {
		r := fwResolver{
			hunters:  map[string]*rechaseBenchHunter{"n:2": wolf},
			quarries: map[string]*fakeQuarry{"v:1": a, "v:2": b},
			player:   him,
		}

		snap, err := p.Snapshot(r)
		require.NoError(t, err)

		data, err := json.Marshal(snap)
		require.NoError(t, err)

		var read PursuitSnapshot
		require.NoError(t, json.Unmarshal(data, &read))

		if mutate != nil {
			mutate(&read)
		}

		p.Close()

		p, _ = newTestPursuit(true)
		require.NoError(t, p.Restore(read, r))
	}

	defer p.Close()

	require.True(t, n.Retarget("n:2", b))

	for i := 0; i < 3; i++ {
		p.Chase(&fakeHunter{id: fmt.Sprintf("x:%d", i), y: float64(i)}, him)
	}

	a.x += 9
	p.Advance(3.0)
	require.True(t, p.Rechase(&fakeHunter{id: "m:0", y: 7}, b), "a start")
	require.False(t, p.Rechase(wolf, b), "the fixture: the re-chase deferred by the budget")

	sight.clear = false
	n.Advance(DefaultNoticeDials().ReEvaluateMinutes)
	n.Advance(DefaultNoticeDials().MemoryMinutes)

	p.GiveUpOnTheForgotten(n)

	return p.ChasersOf("v:1")
}

// A: the resumed game releases the wolf as the game that ran on does.
// Control (a source mutation): Snapshot does not write FromWatch -- the
// resumed game keeps the wolf on A.
func TestAResumedWorldChaseEndsWhenItsWatchForgets(t *testing.T) {
	ranOn := fromWatchAfterAForget(t, false, nil)
	require.Empty(t, ranOn, "the fixture: the game that ran on released the wolf")
	require.Equal(t, ranOn, fromWatchAfterAForget(t, true, nil), "the resumed game diverged")
}

// B: no starvation at the dial corners. With the re-chase cap off
// (RechasesPerFrame 0) the reserve is still one, so a re-chase asked while ten
// chases are due every frame is made in its first frame. At a budget of 1 the
// one slot is SHARED (documented, pinned): Advance takes it on every frame it
// wants a solve, and here it always does. Control (a source mutation): the
// reserve is RechasesPerFrame as before -- at (2,0) and (3,0) the re-chase is
// refused for good.
func TestAReChaseIsNotStarvedAtTheDialCorners(t *testing.T) {
	run := func(solves, rechases int) (moved bool, frames int) {
		p, _ := newTestPursuit(true)
		defer p.Close()

		p.dials.SolvesPerFrame = solves
		p.dials.RechasesPerFrame = rechases

		q := &fakeQuarry{id: "p:1", x: 10}
		her := &fakeQuarry{id: "v:1", x: -10}

		var hunters []*fakeHunter

		for i := 0; i < 10; i++ {
			h := &fakeHunter{id: fmt.Sprintf("m:%02d", i), y: float64(i)}
			hunters = append(hunters, h)
			p.Chase(h, q)
		}

		for f := 1; f <= 200 && !moved; f++ {
			q.x += 2
			frames = f

			budgetFrame(p, 2.0, func() { moved = p.Rechase(hunters[9], her) })
		}

		return moved, frames
	}

	for _, c := range [][2]int{{2, 1}, {2, 0}, {3, 0}, {3, 1}, {4, 2}} {
		moved, frames := run(c[0], c[1])
		require.True(t, moved, "SolvesPerFrame %d, RechasesPerFrame %d: the re-chase is made", c[0], c[1])
		require.Equal(t, 1, frames, "SolvesPerFrame %d, RechasesPerFrame %d: in the frame it is asked", c[0], c[1])
	}

	moved, _ := run(1, 1)
	require.False(t, moved, "at a budget of 1 the slot is shared: re-paths due every frame keep it (documented)")
}

// A start past the budget is a WORLD chase, so BUG-108's give-up covers it.
// Control (the review's mutant start-no-fromwatch): the queued start made
// without fromWatch.
func TestAQueuedStartIsAWorldChase(t *testing.T) {
	p, _ := newTestPursuit(true)
	defer p.Close()

	q := &fakeQuarry{id: "p:1", x: 10}

	budgetFrame(p, 1.0/24, func() {
		for i := 0; i < 3; i++ {
			p.Rechase(&fakeHunter{id: fmt.Sprintf("m:%d", i), y: float64(i)}, q)
		}
	})

	require.Equal(t, 1, p.Queued())
	require.True(t, p.chases["m:2"].fromWatch, "the queued start is a world chase")
}

// A start owing its route whose hunter already stands beside its quarry is
// still served by Advance, first of all it serves -- here ahead of a re-path
// that has waited longer, on a frame whose kept-back unit a start takes, so
// ServeQueued has nothing left. Control (the review's mutant
// first-route-skips-arrived): Advance skips an arrived chase before it asks
// whether it owes a first route -- the hunter beside its quarry stands.
func TestAQueuedStartBesideItsQuarryIsServed(t *testing.T) {
	p, _ := newTestPursuit(true)
	defer p.Close()

	q := &fakeQuarry{id: "p:1", x: 10}
	old := &fakeHunter{id: "a:0", y: 9}
	p.Chase(old, q)

	near := &fakeHunter{id: "m:9", x: 10.5}

	budgetFrame(p, 1.0/24, func() {
		p.Rechase(&fakeHunter{id: "m:0"}, q)
		p.Rechase(&fakeHunter{id: "m:1", y: 3}, q)
		p.Rechase(near, q)
	})

	require.Zero(t, near.routes, "the fixture: it owes its route")

	old.following = false // ran out of route: due a fresh one

	budgetFrame(p, 3.0, func() { p.Rechase(&fakeHunter{id: "m:5", y: -5}, q) })
	require.Equal(t, 1, near.routes, "served though beside its quarry")
	require.Zero(t, p.Queued())
}

// A start that takes the kept-back unit leaves nothing for a re-chase later in
// the same frame: the re-chase is deferred and the frame stays within the
// budget. Control (the review's mutant rechase-unbudgeted): a re-chase past
// the budget is made anyway.
func TestAReChaseAfterAStartIsDeferred(t *testing.T) {
	p, _ := newTestPursuit(true)
	defer p.Close()

	q := &fakeQuarry{id: "p:1", x: 10}
	her := &fakeQuarry{id: "v:1", x: -10}
	old := &fakeHunter{id: "m:5"}
	p.Chase(old, q)

	for i := 0; i < 3; i++ {
		p.Chase(&fakeHunter{id: fmt.Sprintf("a:%d", i), y: float64(i)}, q)
	}

	q.x += 5

	var moved bool

	budgetFrame(p, 3.0, func() {
		p.Rechase(&fakeHunter{id: "m:0", y: -4}, q) // a start, first in the aware order
		moved = p.Rechase(old, her)
	})

	require.False(t, moved, "no unit left for the re-chase")
	require.LessOrEqual(t, p.solvedThisFrame, p.dials.SolvesPerFrame)
}

// At SolvesPerFrame 1 Advance still serves at most one a frame. Control (the
// review's mutant no-clamp): the kept-back share not held below the budget --
// at 1 Advance's share is 0, read as no limit.
func TestTheDialOneStillBindsAdvance(t *testing.T) {
	p, _ := newTestPursuit(true)
	defer p.Close()

	p.dials.SolvesPerFrame = 1

	q := &fakeQuarry{id: "p:1", x: 10}
	for i := 0; i < 10; i++ {
		p.Chase(&fakeHunter{id: fmt.Sprintf("m:%02d", i), y: float64(i)}, q)
	}

	q.x += 5
	require.Equal(t, 1, budgetFrame(p, 3.0, nil))
}

// C: how long a burst stands while chases already running fall due every
// frame (the review's stand-time probe; dt 2 world minutes, every chase due
// every frame). With ServeQueued the burst drains two a frame: 24 running and
// 24 noticing, the last routed 12 frames after (0.2 s); 48 and 48, 24 frames.
// The running chases get no re-path while the burst drains (first routes come
// first). Control (a source mutation): ServeQueued serves nothing -- one a
// frame, 23 and 47 frames, as the review measured.
func TestABurstStandsHalfAsLong(t *testing.T) {
	for _, c := range []struct{ existing, burst, want int }{{0, 8, 3}, {24, 24, 12}, {48, 48, 24}} {
		p, _ := newTestPursuit(true)
		q := &fakeQuarry{id: "p:1", x: 10}

		var old []*fakeHunter

		for i := 0; i < c.existing; i++ {
			h := &fakeHunter{id: fmt.Sprintf("a:%02d", i), y: float64(i)}
			old = append(old, h)
			p.Chase(h, q)
		}

		frames, frozen := 0, 0

		for f := 0; f < 400; f++ {
			q.x += 2

			before := 0
			for _, h := range old {
				before += h.routes
			}

			budgetFrame(p, 2.0, func() {
				if f == 0 {
					for i := 0; i < c.burst; i++ {
						p.Rechase(&fakeHunter{id: fmt.Sprintf("m:%02d", i), y: float64(-i)}, q)
					}
				}
			})

			after := 0
			for _, h := range old {
				after += h.routes
			}

			if f > 0 && after == before && c.existing > 0 {
				frozen++
			}

			if p.Queued() == 0 {
				frames = f

				break
			}
		}

		t.Logf("existing %d, burst %d: the last new hunter routed %d frames after the burst (%.2f s at 60 fps); "+
			"the running chases had no re-path on %d of them", c.existing, c.burst, frames, float64(frames)/60, frozen)
		require.Equal(t, c.want, frames, "existing %d, burst %d", c.existing, c.burst)
		p.Close()
	}
}

// ServeQueued serves nothing in a frame whose world stood still (a held
// fight's), as Advance does not. Control (a source mutation): the frame check
// removed -- the queued start is routed in a held frame.
func TestServeQueuedIdlesInAHeldFrame(t *testing.T) {
	p, _ := newTestPursuit(true)
	defer p.Close()

	q := &fakeQuarry{id: "p:1", x: 10}

	budgetFrame(p, 1.0/24, func() {
		for i := 0; i < 3; i++ {
			p.Rechase(&fakeHunter{id: fmt.Sprintf("m:%d", i), y: float64(i)}, q)
		}
	})
	require.Equal(t, 1, p.Queued(), "the fixture")

	require.Zero(t, budgetFrame(p, 0, nil), "a held frame routes nothing")
	require.Equal(t, 1, p.Queued())

	require.Equal(t, 1, budgetFrame(p, 1.0/24, nil), "the next running frame does")
}
