package d2world

// BUG-73 (the raid-r1 follow-up, 29 Sep 2026; the merge scout's N3,
// strigoi-harness-runs\wt-merge-scout\merge-notes.md): a world file whose two
// clock fights had their quarries swapped was accepted by every check and
// resumed a world that was not the saved one. CheckClockWatches holds each
// live clock fight's living enemies against the file's watches and chases.
//
//	TestSwappedClockQuarriesAreRefused         the scout's N3, unit-sized: the
//	                                           swapped file refused, the true
//	                                           one taken, and why (the swapped
//	                                           world, run one step, is not the
//	                                           saved one)
//	TestTheWatchCheckTakesWhatTheGameMakes      one enemy's watch and chase at a
//	                                           time: what is refused, and every
//	                                           state the game makes that is not
//	TestEveryFrameOfTwoClockFightsPassesTheWatchCheck
//	                                           the save's side: a live model
//	                                           with two clock fights, forty
//	                                           frames of joins, retargets, a
//	                                           Downed man standing again, an
//	                                           unwatch, deaths and a quarry's
//	                                           end, every frame's file checked

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// watchWorld is clockFixture's world (c:1 live on v:1 with a dead, a routed,
// a broken-off and a late enemy; c:2 ended) with a second clock fight, c:3,
// live on v:9 beside it, and the chases the game would have started. The
// fixture's pursuit is a fake that keeps only WHO chases; chaseOf keeps
// what, as Game.startChasesForTheAware would have started it: every aware
// watcher not already chasing chases what it is aware of, and a chase is
// never moved after.
type watchWorld struct {
	*clockWorld
	chaseOf map[string]string
}

// ref is a quarry as the file writes it: PlayerRef for him, else its id.
func (w *watchWorld) ref(q Quarry) string {
	if q.QuarryID() == w.target.QuarryID() {
		return PlayerRef
	}

	return q.QuarryID()
}

// startChases is Game.startChasesForTheAware over the fake pursuit.
func (w *watchWorld) startChases() {
	for _, p := range w.notice.AwarePairs() {
		if id := p.Watcher.WatcherID(); !w.chases.chasing[id] {
			w.chases.chasing[id] = true
			w.chaseOf[id] = w.ref(p.Target)
		}
	}
}

// frame is one world minute in the game's order (Game.advanceWorld): the
// notice model steps (the tables step it), the aware start their chases, and
// then combat.
func (w *watchWorld) frame() {
	w.notice.Advance(1.0)
	w.startChases()
	w.c.Advance(1.0)
}

// pursuit is the pursuit block the save would write.
func (w *watchWorld) pursuit() PursuitSnapshot {
	ids := make([]string, 0, len(w.chases.chasing))
	for id, on := range w.chases.chasing {
		if on {
			ids = append(ids, id)
		}
	}

	sort.Strings(ids)

	p := PursuitSnapshot{}
	for _, id := range ids {
		p.Chases = append(p.Chases, ChaseSnapshot{Hunter: id, Quarry: w.chaseOf[id]})
	}

	return p
}

// file is the three blocks a save takes now: combat (refused, as the save
// is, if the model holds anything no save may drop), notice and pursuit.
func (w *watchWorld) file(t *testing.T) (CombatSnapshot, NoticeSnapshot, PursuitSnapshot) {
	t.Helper()

	s, err := w.c.Snapshot()
	require.NoError(t, err, "a fight he is not in never stops a save")

	n, err := w.notice.Snapshot(clockResolver{w.clockWorld})
	require.NoError(t, err)

	return s, n, w.pursuit()
}

// twoClockFights is the world at the moment c:3 has opened beside c:1.
func twoClockFights(t *testing.T) *watchWorld {
	t.Helper()

	w := &watchWorld{clockWorld: clockFixture(t), chaseOf: map[string]string{}}

	// The fixture's monsters were set chasing their quarries (monster()).
	for id := range w.chases.chasing {
		if tw := w.notice.watches[id]; tw != nil {
			w.chaseOf[id] = w.ref(tw.target)
		}
	}

	v9 := w.quarry("v:9", 80, 50, 40)
	w.monster(t, "m:10", 81, 50, 5000, Profile{Row: "wolves", Group: "g:10", Speed: 2, DamageMin: 1, DamageMax: 3}, v9)
	w.chaseOf["m:10"] = "v:9"

	w.frame()

	require.Len(t, w.c.clockFights, 2, "c:3 opened beside c:1")
	require.Equal(t, "c:1", w.c.clockFights[0].id)
	require.Equal(t, "c:3", w.c.clockFights[1].id)

	return w
}

// swapped is s with its first two live clock fights' quarries exchanged: the
// scout's N3.
func swapped(s CombatSnapshot) CombatSnapshot {
	live := append([]CombatClockFightSnapshot{}, s.Clock.Live...)
	live[0].Quarry, live[1].Quarry = live[1].Quarry, live[0].Quarry
	s.Clock.Live = live

	return s
}

// restoredInto is twoClockFights' world with its combat model replaced by a
// fresh one into which snap is restored (clockCopy's shape): the load.
func restoredInto(t *testing.T, snap CombatSnapshot) (*watchWorld, error) {
	t.Helper()

	w := twoClockFights(t)
	w.c.Close()

	dials := DefaultCombatDials()
	dials.PlayerControl = PlayerControlHuman
	clock := NewClock(DefaultClockDials())
	t.Cleanup(clock.Close)

	w.c = NewCombat(clock, w.notice, w.fitness, w.illum, w.bodies,
		w.profiles, w.animator, w.morale, w.chases, d2rand.Derive(99, d2rand.StreamCombat), dials)
	t.Cleanup(w.c.Close)
	w.c.SetPlayer("p:1")
	w.c.SetCorpses(w.corpses)
	w.c.SetResolver(clockResolver{w.clockWorld})

	return w, w.c.Restore(snap, b2aWorldSeed)
}

func liveIDs(c *Combat) []string {
	out := []string{}
	for _, e := range c.clockFights {
		out = append(out, e.id+"@"+e.target.QuarryID())
	}

	return out
}

// TestSwappedClockQuarriesAreRefused is BUG-73: the scout's N3 at unit size.
// Two clock fights live, c:1 on v:1 and c:3 on v:9; the file with their
// quarries swapped passes Combat.Validate -- every id resolves -- and is
// refused by CheckClockWatches, naming the living enemy aware of the other
// quarry. The file as saved is taken. And the reason it must be refused: the
// swapped file, restored, is not the world that was saved -- one step on, the
// swapped c:1 has let its enemies go and ended (they go to the fight on the
// quarry they are aware of), where the saved world fights on.
func TestSwappedClockQuarriesAreRefused(t *testing.T) {
	w := twoClockFights(t)
	s, n, p := w.file(t)

	require.NoError(t, w.c.Validate(s, b2aWorldSeed))
	require.NoError(t, CheckClockWatches(s, n, p), "the file as the save took it")

	bad := swapped(s)
	require.Equal(t, "v:9", bad.Clock.Live[0].Quarry)
	require.NoError(t, w.c.Validate(bad, b2aWorldSeed), "the gap: Combat.Validate alone takes the swapped file")

	err := CheckClockWatches(bad, n, p)
	require.Error(t, err, "the swapped file is refused")
	require.Contains(t, err.Error(), `clock.live[0] c:1 (after "v:9"): its living enemy m:3 is aware of "v:1" and chases "v:1"`)

	// The watch alone refuses it: with no pursuit block at all (a watcher
	// that cannot walk chases nothing), still refused.
	require.Error(t, CheckClockWatches(bad, n, PursuitSnapshot{}))

	// Swapped back, it is the saved file again.
	require.NoError(t, CheckClockWatches(swapped(bad), n, p))

	// WHY: restored, the true file fights on as saved; the swapped one ends
	// both fights at the first step (each enemy aware of another's quarry is
	// let go) and opens new ones, a world no save ever held.
	good, err := restoredInto(t, s)
	require.NoError(t, err)
	good.frame()
	require.Equal(t, []string{"c:1@v:1", "c:3@v:9"}, liveIDs(good.c), "the true file fights on")

	moved, err := restoredInto(t, bad)
	require.NoError(t, err, "Restore takes it too (Validate is its check)")
	require.Equal(t, []string{"c:1@v:9", "c:3@v:1"}, liveIDs(moved.c))
	moved.frame()
	require.NotEqual(t, liveIDs(good.c), liveIDs(moved.c), "the swapped file resumes another world")

	require.NotContains(t, liveIDs(moved.c), "c:1@v:9", "the swapped c:1 let its enemies go and ended")
	require.Greater(t, moved.c.clockBook.ended, good.c.clockBook.ended, "a fight the saved world fights on ended")

	t.Logf("one step on: saved %v; swapped %v (ended %d -> %d: disengaged %d -> %d, quarry_dead %d -> %d)",
		liveIDs(good.c), liveIDs(moved.c), good.c.clockBook.ended, moved.c.clockBook.ended,
		good.c.clockBook.endedDisengaged, moved.c.clockBook.endedDisengaged,
		good.c.clockBook.quarryDead, moved.c.clockBook.quarryDead)
}

// TestTheWatchCheckTakesWhatTheGameMakes holds one living enemy's watch and
// chase in the true file to each state in turn. Refused: a NOTICED watch on
// someone else with no chase on the fight's quarry -- what pruneOrEnd lets go
// at the end of every step. Taken: everything the game makes -- a watch on
// another not yet noticed (a Downed man standing again into the village's
// fight watches him; a retarget not yet noticed), no watch (a harness
// unwatch), a noticed retarget whose chase is still on the quarry (a harness
// strigoi_watch between frames), and any watch at all on a gone enemy's row.
func TestTheWatchCheckTakesWhatTheGameMakes(t *testing.T) {
	w := twoClockFights(t)
	s, n, p := w.file(t)

	require.Equal(t, "c:1", s.Clock.Live[0].ID)
	require.Contains(t, s.Clock.Live[0].Enemies, "m:3")
	require.Contains(t, s.Clock.Live[0].Dead, "d:1")
	require.Contains(t, s.Clock.Live[0].Enemies, "d:1", "the dead dog keeps its row")

	// with sets watcher's watch (nil: none) and chase ("": none) in copies
	// of the blocks.
	with := func(watcher string, watch *WatchSnapshot, chase string) (NoticeSnapshot, PursuitSnapshot) {
		nn := NoticeSnapshot{Checks: n.Checks, Notices: n.Notices}

		for _, ws := range n.Watches {
			if ws.Watcher != watcher {
				nn.Watches = append(nn.Watches, ws)
			}
		}

		if watch != nil {
			ws := *watch
			ws.Watcher = watcher
			nn.Watches = append(nn.Watches, ws)
		}

		pp := PursuitSnapshot{Solves: p.Solves}

		for _, cs := range p.Chases {
			if cs.Hunter != watcher {
				pp.Chases = append(pp.Chases, cs)
			}
		}

		if chase != "" {
			pp.Chases = append(pp.Chases, ChaseSnapshot{Hunter: watcher, Quarry: chase})
		}

		return nn, pp
	}

	cases := []struct {
		name    string
		watcher string
		watch   *WatchSnapshot
		chase   string
		refused bool
	}{
		{"aware of its quarry, chasing it", "m:3", &WatchSnapshot{Target: "v:1", Noticed: true, Sees: true}, "v:1", false},
		{"aware of its quarry, chasing him", "m:3", &WatchSnapshot{Target: "v:1", Noticed: true}, PlayerRef, false},
		{"watching its quarry, not aware", "m:3", &WatchSnapshot{Target: "v:1"}, "", false},
		{"aware of the other fight's quarry, chasing it", "m:3", &WatchSnapshot{Target: "v:9", Noticed: true}, "v:9", true},
		{"aware of the other fight's quarry, no chase", "m:3", &WatchSnapshot{Target: "v:9", Noticed: true}, "", true},
		{"aware of him, no chase", "m:3", &WatchSnapshot{Target: PlayerRef, Noticed: true}, "", true},
		{"aware of an entity in no fight, chasing it", "m:3", &WatchSnapshot{Target: "v:5", Noticed: true}, "v:5", true},
		{"retargeted to him, not yet aware (a Downed man stood again)", "m:3", &WatchSnapshot{Target: PlayerRef}, "", false},
		{"retargeted to the other quarry, not yet aware", "m:3", &WatchSnapshot{Target: "v:9"}, "v:9", false},
		{"no watch (strigoi_unwatch), chasing its quarry", "m:3", nil, "v:1", false},
		{"no watch, no chase", "m:3", nil, "", false},
		{"no watch, chasing another", "m:3", nil, "v:9", false},
		{"aware of another, still chasing its quarry (strigoi_watch between frames)", "m:3", &WatchSnapshot{Target: "v:9", Noticed: true}, "v:1", false},
		{"a dead enemy's row, aware of another", "d:1", &WatchSnapshot{Target: "v:9", Noticed: true}, "", false},
		{"a routed enemy's row, aware of him", "d:2", &WatchSnapshot{Target: PlayerRef, Noticed: true}, PlayerRef, false},
		{"the other fight's enemy, aware of this one's quarry", "m:10", &WatchSnapshot{Target: "v:1", Noticed: true}, "v:1", true},
	}

	for _, tc := range cases {
		nn, pp := with(tc.watcher, tc.watch, tc.chase)
		err := CheckClockWatches(s, nn, pp)

		if tc.refused {
			require.Error(t, err, "%s: taken", tc.name)
			require.Contains(t, err.Error(), "its living enemy "+tc.watcher+" is aware of", tc.name)
		} else {
			require.NoError(t, err, "%s: refused", tc.name)
		}
	}

	// Nothing live, nothing to hold against: a file with no clock fight is
	// taken whatever its watches.
	quiet := s
	quiet.Clock.Live = nil
	nn, pp := with("m:3", &WatchSnapshot{Target: "v:9", Noticed: true}, "")
	require.NoError(t, CheckClockWatches(quiet, nn, pp))
}

// TestEveryFrameOfTwoClockFightsPassesTheWatchCheck is the save's side: the
// save runs the load's checks (B3, validateSnapshots), so CheckClockWatches
// must take every file the live game can make. Forty frames of a live model
// with two clock fights, in the game's frame order, through each thing that
// moves a watch, a chase or a row -- a late joiner; a living enemy's watch
// moved to him, never noticed (he is forty tiles off); a Downed man standing
// again into the village's fight (Rejoin), watching him as a spawn table's
// member does, with no chase; a living enemy unwatched and kept; a living
// enemy's watch moved, noticed, to the other fight's quarry, and let go at
// the next step; the fights' own deaths, routs and a quarry's end -- and the
// file checked at every frame, and at every harness verb's moment between
// frames. None is refused.
func TestEveryFrameOfTwoClockFightsPassesTheWatchCheck(t *testing.T) {
	w := twoClockFights(t)
	r := clockResolver{w.clockWorld}

	checked, two := 0, 0
	check := func(when string) {
		t.Helper()

		s, n, p := w.file(t)
		require.NoError(t, w.c.Validate(s, b2aWorldSeed), "%s", when)
		require.NoError(t, CheckClockWatches(s, n, p), "%s: the live game's own file refused", when)

		checked++
		if len(s.Clock.Live) >= 2 {
			two++
		}
	}

	aware := func(id string) (noticed, watching bool) { return w.notice.Noticed(id) }

	check("the two fights open")

	for i := 1; i <= 40; i++ {
		switch i {
		case 2: // a late joiner into c:3
			m11 := &fakeWatcher{id: "m:11", x: 79, y: 50}
			w.watchers["m:11"] = m11
			w.bodies.known["m:11"] = &fakeBody{health: 5000, maxHealth: 5000}
			w.profiles.byID["m:11"] = Profile{Row: "wolves", Group: "g:11", Speed: 1, DamageMin: 1, DamageMax: 3}
			w.notice.Watch(m11, w.quarries["v:9"])
			check("m:11 aware of v:9, before the step")
		case 4: // m:3's watch moved to him: forty tiles off, never noticed
			w.notice.Watch(w.watchers["m:3"], w.target)
			noticed, _ := aware("m:3")
			require.False(t, noticed, "m:3 cannot see him")
			check("m:3 retargeted to him, before the step")
		case 6: // a Downed man stands again into the village's fight
			r12 := &fakeWatcher{id: "r:12", x: 81, y: 40}
			w.watchers["r:12"] = r12
			w.bodies.known["r:12"] = &fakeBody{health: 30, maxHealth: 30}
			w.profiles.byID["r:12"] = Profile{Row: RisenRow, Dead: true}

			in, his := w.c.RejoinFight("d:1", r12)
			require.True(t, in && !his, "r:12 stands into c:1")

			w.notice.Watch(r12, w.target) // Spawns.Raise: a member watching the target, him
			noticed, _ := aware("r:12")
			require.False(t, noticed, "r:12 cannot see him")
			check("r:12 stood into c:1, before the step")
		case 8: // strigoi_unwatch on a living enemy of c:1
			require.True(t, w.notice.Unwatch("m:7"))
			check("m:7 unwatched, before the step")
		case 10: // m:10's watch moved to v:1, ten tiles off: noticed at once
			w.notice.Watch(w.watchers["m:10"], w.quarries["v:1"])
			noticed, _ := aware("m:10")
			require.True(t, noticed, "m:10 sees v:1")
			check("m:10 aware of v:1 in c:3, before the step (its chase is still on v:9)")
		}

		w.frame()
		check("after the step")

		switch i {
		case 2:
			require.True(t, clockHolds(w.resolverFight, "m:11"), "m:11 joined c:3")
		case 4:
			require.True(t, clockHolds(w.resolverFight, "m:3"), "m:3, aware of no one else, stays in c:1")
		case 6:
			require.True(t, clockHolds(w.resolverFight, "r:12"), "r:12, not aware of him, stays in c:1")
		case 8:
			require.True(t, clockHolds(w.resolverFight, "m:7"), "m:7, watching no one, stays in c:1 while in reach")
		case 10:
			for _, e := range w.c.clockFights {
				if e.target.QuarryID() == "v:9" {
					require.Nil(t, e.enemyByID("m:10"), "c:3 let m:10 go at the step its watch moved")
				}
			}
		}
	}

	_, err := w.notice.Snapshot(r)
	require.NoError(t, err)

	t.Logf("checked %d files, %d with two clock fights live; clock: started %d, ended %d (quarry_dead %d, disengaged %d), joined %d",
		checked, two, w.c.clockBook.started, w.c.clockBook.ended, w.c.clockBook.quarryDead,
		w.c.clockBook.endedDisengaged, w.c.clockBook.joined)
	require.GreaterOrEqual(t, two, 10, "two clock fights live on most frames")
}
