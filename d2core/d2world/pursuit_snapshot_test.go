package d2world

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// M4.6 B2b: pursuit's snapshot. The round trip and the run in step are the
// world tests in spawns_snapshot_test.go; these are pursuit's own fields and
// refusals.

func b2bChaseWhere(t *testing.T, s *b2bSnap, what string, ok func(ChaseSnapshot) bool) *ChaseSnapshot {
	t.Helper()

	for i := range s.Pursuit.Chases {
		if ok(s.Pursuit.Chases[i]) {
			return &s.Pursuit.Chases[i]
		}
	}

	t.Fatalf("no saved chase that %s", what)

	return nil
}

func b2bWalkingChase(t *testing.T, s *b2bSnap) *ChaseSnapshot {
	return b2bChaseWhere(t, s, "is walking", func(c ChaseSnapshot) bool { return !c.Arrived && c.SinceSolve > 0 })
}

func b2bArrivedChase(t *testing.T, s *b2bSnap) *ChaseSnapshot {
	return b2bChaseWhere(t, s, "has arrived", func(c ChaseSnapshot) bool { return c.Arrived })
}

// b2bIdleWatcher is a watched entity that is not chasing: a straggler who
// never saw him.
func b2bIdleWatcher(t *testing.T, s *b2bSnap) string {
	t.Helper()

	chasing := map[string]bool{}
	for _, c := range s.Pursuit.Chases {
		chasing[c.Hunter] = true
	}

	for _, w := range s.Notice.Watches {
		if !chasing[w.Watcher] {
			return w.Watcher
		}
	}

	t.Fatal("every watcher is chasing")

	return ""
}

// Every saved pursuit field, lost or altered, is seen -- or refused.
func TestPursuitSnapshotEveryFieldIsSeen(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)
	want := b2bRecord(t, a, b2bAdvanceTicks)

	chase := func(name string, pick func(*testing.T, *b2bSnap) *ChaseSnapshot, expect b2bExpect, f func(*ChaseSnapshot)) b2bMutation {
		return b2bMutation{"pursuit.chase." + name, expect, func(t *testing.T, s *b2bSnap) { f(pick(t, s)) }}
	}

	oldPlayer := a.player.id

	outcomes := b2bSweep(t, sv, want, []b2bMutation{
		{"pursuit.solves lost", b2bDiverge, func(_ *testing.T, s *b2bSnap) { s.Pursuit.Solves = 0 }},
		{"pursuit.rechase_solves moved", b2bDiverge, func(_ *testing.T, s *b2bSnap) { s.Pursuit.RechaseSolves++ }},
		{"pursuit.rechase_solves past solves", b2bRefuse, func(_ *testing.T, s *b2bSnap) { s.Pursuit.RechaseSolves = s.Pursuit.Solves + 1 }},
		{"pursuit.chases lost", b2bDiverge, func(_ *testing.T, s *b2bSnap) { s.Pursuit.Chases = nil }},
		{"pursuit.a chase lost", b2bDiverge, func(_ *testing.T, s *b2bSnap) { s.Pursuit.Chases = s.Pursuit.Chases[1:] }},

		chase("solved_at_x lost", b2bWalkingChase, b2bDiverge, func(c *ChaseSnapshot) { c.SolvedAtX = 0 }),
		chase("solved_at_y lost", b2bWalkingChase, b2bDiverge, func(c *ChaseSnapshot) { c.SolvedAtY = 0 }),
		chase("solved_distance lost", b2bWalkingChase, b2bDiverge, func(c *ChaseSnapshot) { c.SolvedDistance = 0 }),
		chase("since_solve lost", b2bWalkingChase, b2bDiverge, func(c *ChaseSnapshot) { c.SinceSolve = 0 }),
		chase("reachable flipped", b2bWalkingChase, b2bDiverge, func(c *ChaseSnapshot) { c.Reachable = !c.Reachable }),
		chase("solves lost", b2bWalkingChase, b2bDiverge, func(c *ChaseSnapshot) { c.Solves = 0 }),
		chase("arrived lost", b2bArrivedChase, b2bDiverge, func(c *ChaseSnapshot) { c.Arrived = false }),
		chase("since_solve negative", b2bWalkingChase, b2bRefuse, func(c *ChaseSnapshot) { c.SinceSolve = -1 }),

		// Identity.
		chase("hunter empty", b2bWalkingChase, b2bRefuse, func(c *ChaseSnapshot) { c.Hunter = "" }),
		chase("hunter unknown", b2bWalkingChase, b2bRefuse, func(c *ChaseSnapshot) { c.Hunter = "e:999" }),
		{"pursuit.chase.hunter saved twice", b2bRefuse, func(_ *testing.T, s *b2bSnap) {
			s.Pursuit.Chases[1].Hunter = s.Pursuit.Chases[0].Hunter
		}},
		// A chase handed to a straggler who never saw him restores -- and
		// the wrong wolf is coming. It must show.
		{"pursuit.chase.hunter another entity", b2bDiverge, func(t *testing.T, s *b2bSnap) {
			b2bWalkingChase(t, s).Hunter = b2bIdleWatcher(t, s)
		}},
		chase("quarry unknown", b2bWalkingChase, b2bRefuse, func(c *ChaseSnapshot) { c.Quarry = "e:999" }),
		chase("quarry empty", b2bWalkingChase, b2bRefuse, func(c *ChaseSnapshot) { c.Quarry = "" }),
		chase("quarry the old player id", b2bWalkingChase, b2bRefuse, func(c *ChaseSnapshot) { c.Quarry = oldPlayer }),
		{"pursuit.chase.quarry another entity", b2bDiverge, func(t *testing.T, s *b2bSnap) {
			c := b2bWalkingChase(t, s)
			c.Quarry = b2bIdleWatcher(t, s)
		}},
	})

	b2aExercised(t, outcomes, b2bPursuitClasses()...)
}

// b2bPursuitClasses is every field of pursuit and of a chase, labelled at its
// path in the saved moment (the B2b review's B5).
func b2bPursuitClasses() []b2aClass {
	return []b2aClass{
		{Pursuit{}, map[string]string{
			"dials":    "D: the pursuit dials, the game's numbers; a script's writes are test setup (trap 7)",
			"router":   "W: the map's router",
			"chases":   "S:pursuit.chases",
			"solves":   "S:pursuit.solves",
			"rechases": "S:pursuit.rechase_solves",
		}},
		{chase{}, map[string]string{
			"hunter":         "S:pursuit.chases[0].hunter",
			"quarry":         "S:pursuit.chases[0].quarry",
			"solvedAtX":      "S:pursuit.chases[0].solved_at_x",
			"solvedAtY":      "S:pursuit.chases[0].solved_at_y",
			"solvedDistance": "S:pursuit.chases[0].solved_distance",
			"sinceSolve":     "S:pursuit.chases[0].since_solve_minutes",
			"reachable":      "S:pursuit.chases[0].reachable",
			"solves":         "S:pursuit.chases[0].solves",
			"arrived":        "S:pursuit.chases[0].arrived",
		}},
	}
}

func TestPursuitSnapshotFieldsClassified(t *testing.T) {
	a := b2bFilledWorld(t)
	snap := a.snapshot(t)

	for _, c := range b2bPursuitClasses() {
		b2aClassified(t, c.kind, snap, c.fields)
	}
}

// A restore into a pursuit that already runs a chase is refused (the B2b
// review's B4): the live chase's hunter would walk a route no saved chase owns.
func TestPursuitRestoreRefusesAPursuitInUse(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)

	before := b.observe(t)

	require.Error(t, b.pursuit.Validate(sv.snap.Pursuit, b2bResolver{b}))
	require.Error(t, b.pursuit.Restore(sv.snap.Pursuit, b2bResolver{b}), "the very snapshot, over itself")
	require.Equal(t, before, b.observe(t))
}

// A hunter must be able to walk: a watcher that cannot hunt is refused, the
// same type assertion the live path makes.
type b2bStillResolver struct{ b2bResolver }

type b2bStill struct {
	id   string
	x, y float64
}

func (s b2bStill) WatcherID() string             { return s.id }
func (s b2bStill) WatcherAt() (float64, float64) { return s.x, s.y }

func (r b2bStillResolver) Watcher(id string) (Watcher, bool) {
	e, ok := r.w.entities[id]
	if !ok {
		return nil, false
	}

	return b2bStill{e.id, e.x, e.y}, true // a Watcher and NOT a Hunter
}

func TestPursuitRestoreRefusesAHunterThatCannotWalk(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b, err := b2bResumeSkipping(t, sv, sv.snap, "pursuit")
	require.NoError(t, err)

	err = b.pursuit.Restore(sv.snap.Pursuit, b2bStillResolver{b2bResolver{b}})
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)
}

// A chase of an entity, not the player -- the harness can start one -- is
// saved by the entity's id and resolves back to that entity.
func TestPursuitSnapshotChaseOfAnEntity(t *testing.T) {
	w := b2bNewWorld(t, 1, "p:0")
	h := &b2bEntity{id: "e:1", x: 1, y: 1, speed: 0.5}
	q := &b2bEntity{id: "e:2", x: 9, y: 1}
	w.entities[h.id], w.entities[q.id] = h, q

	w.pursuit.Chase(h, b2bEntityQuarry{q})

	snap, err := w.pursuit.Snapshot(b2bResolver{w})
	require.NoError(t, err)
	require.Equal(t, "e:2", snap.Chases[0].Quarry)

	b := b2bNewWorld(t, 2, "p:1")
	b.entities[h.id], b.entities[q.id] = h.clone(), q.clone()
	require.NoError(t, b.pursuit.Restore(snap, b2bResolver{b}))
	require.Equal(t, "e:2", b.pursuit.chases["e:1"].quarry.QuarryID())
}

// Restore solves nothing and hands no hunter a route: the walk is the
// entity's, restored with its motion. And it is all-or-nothing.
func TestPursuitRestoreSolvesNothingAndIsAllOrNothing(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)
	require.Equal(t, sv.snap.Pursuit.Solves, b.pursuit.Solves(), "a restore solves no route")

	for id, e := range b.entities {
		require.Equal(t, sv.entities[id].route, e.route, "a restore hands no hunter a path")
	}

	// A relaunch whose pursuit is still as built: the only one a Restore takes.
	c, err := b2bResumeSkipping(t, sv, sv.snap, "pursuit")
	require.NoError(t, err)

	before := c.observe(t)

	bad := b2bThroughJSON(t, sv.snap)
	bad.Pursuit.Solves += 100
	bad.Pursuit.Chases[len(bad.Pursuit.Chases)-1].Hunter = "e:999"

	require.True(t, errors.Is(c.pursuit.Validate(bad.Pursuit, b2bResolver{c}), ErrUnresolvedRef), "Validate")

	err = c.pursuit.Restore(bad.Pursuit, b2bResolver{c})
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)
	require.Equal(t, before, c.observe(t), "a refused restore must leave the chases exactly as they were")
}

// Losing a chase's since-solve moves WHEN it next re-paths, not just a number.
func TestPursuitSinceSolveIsBehaviour(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	lost := b2bThroughJSON(t, sv.snap)
	for i := range lost.Pursuit.Chases {
		lost.Pursuit.Chases[i].SinceSolve = 0
	}

	at := b2bProbeDiverges(t, sv, lost, func(w *b2bWorld) float64 { return float64(w.pursuit.Solves()) })
	require.GreaterOrEqual(t, at, 1, "the route solves must fall on other ticks")
}
