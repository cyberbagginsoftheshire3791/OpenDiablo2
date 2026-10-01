package d2world

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// M4.6 B2b: the notice model's snapshot. The round trip and the run in step
// are the world tests in spawns_snapshot_test.go, which restore all three
// systems together; these are notice's own fields and refusals.

// b2bWatchWhere finds a saved watch by a predicate.
func b2bWatchWhere(t *testing.T, s *b2bSnap, what string, ok func(WatchSnapshot) bool) *WatchSnapshot {
	t.Helper()

	for i := range s.Notice.Watches {
		if ok(s.Notice.Watches[i]) {
			return &s.Notice.Watches[i]
		}
	}

	t.Fatalf("no saved watch that %s", what)

	return nil
}

// b2bSeeing is a watch that sees him and is part-way to its next look, so
// every one of its numbers is something to lose.
func b2bSeeing(t *testing.T, s *b2bSnap) *WatchSnapshot {
	return b2bWatchWhere(t, s, "sees", func(w WatchSnapshot) bool { return w.Noticed && w.Sees && w.SinceCheck > 0 })
}

func b2bRemembering(t *testing.T, s *b2bSnap) *WatchSnapshot {
	return b2bWatchWhere(t, s, "remembers", func(w WatchSnapshot) bool { return w.Noticed && !w.Sees && w.SinceSeen > 0 })
}

// Every saved notice field, lost or altered, is seen -- or refused.
func TestNoticeSnapshotEveryFieldIsSeen(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)
	want := b2bRecord(t, a, b2bAdvanceTicks)

	watch := func(name string, pick func(*testing.T, *b2bSnap) *WatchSnapshot, expect b2bExpect, f func(*WatchSnapshot)) b2bMutation {
		return b2bMutation{"notice.watch." + name, expect, func(t *testing.T, s *b2bSnap) { f(pick(t, s)) }}
	}

	// The player's id in the SAVED world: a snapshot that wrote it instead of
	// the word would name a man the relaunch does not have.
	oldPlayer := a.player.id

	outcomes := b2bSweep(t, sv, want, []b2bMutation{
		{"notice.checks lost", b2bDiverge, func(_ *testing.T, s *b2bSnap) { s.Notice.Checks = 0 }},
		{"notice.notices lost", b2bDiverge, func(_ *testing.T, s *b2bSnap) { s.Notice.Notices = 0 }},
		{"notice.watches lost", b2bDiverge, func(_ *testing.T, s *b2bSnap) { s.Notice.Watches = nil }},
		{"notice.a watch lost", b2bDiverge, func(_ *testing.T, s *b2bSnap) { s.Notice.Watches = s.Notice.Watches[1:] }},

		watch("sees lost", b2bSeeing, b2bDiverge, func(w *WatchSnapshot) { w.Sees = false }),
		watch("noticed lost", b2bSeeing, b2bDiverge, func(w *WatchSnapshot) { w.Noticed = false }),
		watch("distance lost", b2bSeeing, b2bDiverge, func(w *WatchSnapshot) { w.Distance = 0 }),
		watch("light_at_target lost", b2bSeeing, b2bDiverge, func(w *WatchSnapshot) { w.LightAtTarget = 0 }),
		watch("reach lost", b2bSeeing, b2bDiverge, func(w *WatchSnapshot) { w.Reach = 0 }),
		watch("since_check lost", b2bSeeing, b2bDiverge, func(w *WatchSnapshot) { w.SinceCheck = 0 }),
		watch("since_seen lost", b2bRemembering, b2bDiverge, func(w *WatchSnapshot) { w.SinceSeen = 0 }),
		watch("checks lost", b2bSeeing, b2bDiverge, func(w *WatchSnapshot) { w.Checks = 0 }),
		watch("notices lost", b2bSeeing, b2bDiverge, func(w *WatchSnapshot) { w.Notices = 0 }),
		watch("distance negative", b2bSeeing, b2bRefuse, func(w *WatchSnapshot) { w.Distance = -1 }),

		// The raid's R2: the side the watch is on (a hostile wolf restored as
		// a villager on watch would never come for him).
		watch("side lost", b2bSeeing, b2bRefuse, func(w *WatchSnapshot) { w.Side = "" }),
		watch("side unknown", b2bSeeing, b2bRefuse, func(w *WatchSnapshot) { w.Side = "neutral" }),
		watch("side living", b2bSeeing, b2bDiverge, func(w *WatchSnapshot) { w.Side = string(SideLiving) }),

		// Identity.
		watch("watcher empty", b2bSeeing, b2bRefuse, func(w *WatchSnapshot) { w.Watcher = "" }),
		watch("watcher unknown", b2bSeeing, b2bRefuse, func(w *WatchSnapshot) { w.Watcher = "e:999" }),
		watch("watcher is the word player", b2bSeeing, b2bRefuse, func(w *WatchSnapshot) { w.Watcher = PlayerRef }),
		{"notice.watch.watcher saved twice", b2bRefuse, func(_ *testing.T, s *b2bSnap) {
			s.Notice.Watches[1].Watcher = s.Notice.Watches[0].Watcher
		}},
		watch("target unknown", b2bSeeing, b2bRefuse, func(w *WatchSnapshot) { w.Target = "e:999" }),
		watch("target empty", b2bSeeing, b2bRefuse, func(w *WatchSnapshot) { w.Target = "" }),
		watch("target the old player id", b2bSeeing, b2bRefuse, func(w *WatchSnapshot) { w.Target = oldPlayer }),
		// A watch pointed at an entity that IS in the world restores -- and
		// the wolf is now watching a dog. It must show.
		{"notice.watch.target another entity", b2bDiverge, func(t *testing.T, s *b2bSnap) {
			w := b2bSeeing(t, s)
			for _, o := range s.Notice.Watches {
				if o.Watcher != w.Watcher {
					w.Target = o.Watcher

					return
				}
			}

			t.Fatal("one watch only")
		}},
	})

	b2aExercised(t, outcomes, b2bNoticeClasses()...)
}

// b2bNoticeClasses is every field of the notice model and of a watch,
// labelled at its path in the saved moment (the B2b review's B5).
func b2bNoticeClasses() []b2aClass {
	return []b2aClass{
		{Notice{}, map[string]string{
			"hidden":  "T: true only inside a sleep (talk.go sets it and defers it back); Snapshot refuses while set",
			"player":  "W: his entity id, bound every frame by the game screen (the raid's R2: whom hidden hides)",
			"gone":    "W: the combat model's deadByBody, wired by NewCombat (the R2 review B's B1)",
			"dials":   "D: the notice dials, the radius above all -- Quiet Step's bonus is applied to them when the game is built (trap 7)",
			"sight":   "W: the map's line of sight",
			"illum":   "W: the light model a target is lit by",
			"watches": "S:notice.watches",
			"checks":  "S:notice.checks",
			"notices": "S:notice.notices",
		}},
		{watch{}, map[string]string{
			"watcher":       "S:notice.watches[0].watcher",
			"target":        "S:notice.watches[0].target",
			"side":          "S:notice.watches[0].side",
			"noticed":       "S:notice.watches[0].noticed",
			"sees":          "S:notice.watches[0].sees",
			"distance":      "S:notice.watches[0].distance",
			"lightAtTarget": "S:notice.watches[0].light_at_target",
			"reach":         "S:notice.watches[0].reach",
			"sinceCheck":    "S:notice.watches[0].since_check_minutes",
			"sinceSeen":     "S:notice.watches[0].since_seen_minutes",
			"checks":        "S:notice.watches[0].checks",
			"notices":       "S:notice.watches[0].notices",
		}},
	}
}

func TestNoticeSnapshotFieldsClassified(t *testing.T) {
	a := b2bFilledWorld(t)
	snap := a.snapshot(t)

	for _, c := range b2bNoticeClasses() {
		b2aClassified(t, c.kind, snap, c.fields)
	}

	b2aTransientsRefused(t, b2bNoticeClasses()[0], func() (interface{}, func() error) {
		w := b2bFilledWorld(t)

		return w.notice, func() error { _, err := w.notice.Snapshot(b2bResolver{w}); return err }
	})
}

// A restore into a model that already watches someone is refused (the B2b
// review's B4): the live watch would be forgotten mid-hunt, or kept as a watch
// no saved moment had.
func TestNoticeRestoreRefusesAModelInUse(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)

	before := b.observe(t)

	require.Error(t, b.notice.Validate(sv.snap.Notice, b2bResolver{b}))
	require.Error(t, b.notice.Restore(sv.snap.Notice, b2bResolver{b}), "the very snapshot, over itself")
	require.Equal(t, before, b.observe(t))
}

// A save that cannot name the player is refused before it is written: every
// watch pointing at him would be saved as an id no relaunch has.
func TestNoticeSnapshotNeedsThePlayerNamed(t *testing.T) {
	a := b2bFilledWorld(t)

	_, err := a.notice.Snapshot(b2bNoPlayer{b2bResolver{a}})
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)

	_, err = a.notice.Snapshot(nil)
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)

	// And a watch on something no longer in the live world is refused at
	// save, not written as a file no load could read.
	for id := range a.notice.watches {
		delete(a.entities, id)

		break
	}

	_, err = a.notice.Snapshot(b2bResolver{a})
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)
}

// b2bNoPlayer is a live-world resolver with no player in it.
type b2bNoPlayer struct{ b2bResolver }

func (b2bNoPlayer) Quarry(string) (Quarry, bool) { return nil, false }

// Saving is refused while he is hidden; the flag is transient.
func TestNoticeSnapshotRefusedWhileHidden(t *testing.T) {
	a := b2bFilledWorld(t)
	a.notice.SetHidden(true)

	_, err := a.notice.Snapshot(b2bResolver{a})
	require.Error(t, err)

	a.notice.SetHidden(false)

	_, err = a.notice.Snapshot(b2bResolver{a})
	require.NoError(t, err)
}

// Restore evaluates nothing (Watch would run a sight test and move every
// counter) and is all-or-nothing.
func TestNoticeRestoreEvaluatesNothingAndIsAllOrNothing(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)
	require.Equal(t, sv.snap.Notice.Checks, b.notice.Checks(), "a restore runs no sight test")

	// A relaunch whose notice model is still as built: the only one a Restore
	// takes.
	c, err := b2bResumeSkipping(t, sv, sv.snap, "notice")
	require.NoError(t, err)

	before := c.observe(t)

	bad := b2bThroughJSON(t, sv.snap)
	bad.Notice.Checks += 100
	bad.Notice.Watches[len(bad.Notice.Watches)-1].Watcher = "e:999"

	require.True(t, errors.Is(c.notice.Validate(bad.Notice, b2bResolver{c}), ErrUnresolvedRef), "Validate")

	err = c.notice.Restore(bad.Notice, b2bResolver{c})
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)
	require.Equal(t, before, c.observe(t), "a refused restore must leave the watches exactly as they were")
}

// The quiet step's radius is a dial and is not saved, so a restore cannot
// apply the bonus twice: a restored model keeps the radius it was built with.
func TestNoticeSnapshotCarriesNoDial(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b := b2bNewWorld(t, 7, "p:resumed")
	require.True(t, b.notice.SetRadius(9)) // the talent, applied when the game is built

	for id, e := range sv.entities {
		b.entities[id] = e.clone()
	}

	require.NoError(t, b.notice.Restore(sv.snap.Notice, b2bResolver{b}))
	require.Equal(t, 9.0, b.notice.Dials().Radius)
}

// Losing a watch's since-check moves WHEN it next looks, not just a number:
// the total of sight tests falls on other ticks.
func TestNoticeSinceCheckIsBehaviour(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	lost := b2bThroughJSON(t, sv.snap)
	for i := range lost.Notice.Watches {
		lost.Notice.Watches[i].SinceCheck = 0
	}

	at := b2bProbeDiverges(t, sv, lost, func(w *b2bWorld) float64 { return float64(w.notice.Checks()) })
	require.GreaterOrEqual(t, at, 1, "the sight tests must fall on other ticks")
}
