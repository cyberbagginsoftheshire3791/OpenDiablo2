package d2world

// WHO THE NIGHT'S HUNTERS CHOOSE, IN UNIT FORM (the raid's R2, 29 Sep 2026):
// the brief's R2 assertions over fakes, each with a control. The real game is
// playtest/seek_test.go (TestTheNearestLiving, TestHisSleepHidesOnlyHim).
//
// Where a control breaks the TEST'S side -- the two quarries swapped, the
// protection lifted, the stagger turned off, a monster not yet in the fight --
// it runs here, beside the assertion, and must come out the other way. Where
// it breaks the CODE (the global hidden put back, the living side let into
// AwarePairs, the fighter's check removed) it is a source mutation, applied,
// run and reverted by a script: "The Village at Night - R2 Build Note - 29
// Sep 2026.md" lists each.

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// seekSight is a Sight that blocks the lines to the targets it names and
// clears every other, counting the rays.
type seekSight struct {
	blocked map[[2]float64]bool
	rays    int
}

func (s *seekSight) Clear(_, _, toX, toY float64) bool {
	s.rays++

	return !s.blocked[[2]float64{toX, toY}]
}

// seekWorld is a notice model and Seek over fakes: every line clear unless
// blocked, every tile unlit (reach 12), no spawns and no combat unless set.
type seekWorld struct {
	sight    *seekSight
	notice   *Notice
	seek     *Seek
	quarries []Quarry
}

func newSeekWorld(t *testing.T, combat *Combat, spawns *Spawns) *seekWorld {
	t.Helper()

	w := &seekWorld{sight: &seekSight{blocked: map[[2]float64]bool{}}}
	w.notice = NewNotice(w.sight, &fakeIllumination{}, DefaultNoticeDials())

	if combat != nil {
		w.notice = combat.notice
	}

	w.seek = NewSeek(w.notice, spawns, combat, DefaultSeekDials())
	w.seek.SetQuarries(func() []Quarry { return w.quarries })

	t.Cleanup(w.seek.Close)

	return w
}

// minute runs one world minute in the night's 24 frames, notice first, as
// the game steps them (the tables step the notice model, then Seek).
func (w *seekWorld) minute() {
	for i := 0; i < 24; i++ {
		w.notice.Advance(1.0 / 24)
		w.seek.Advance(1.0 / 24)
	}
}

// targetOf is the quarry a watcher's watch is on.
func (w *seekWorld) targetOf(id string) string {
	if wt, ok := w.notice.watches[id]; ok && wt.target != nil {
		return wt.target.QuarryID()
	}

	return ""
}

func (w *seekWorld) row(t *testing.T, id string) *seekRow {
	t.Helper()

	r, ok := w.seek.rows[id]
	require.True(t, ok, "Seek has no row for %s", id)

	return r
}

// --- R2 assertion 1: a watcher with two visible quarries takes the nearer ----

// nearerOf: a wolf at the origin watching him, and a villager; him at hx and
// the villager at vx, both in the open. What does the wolf end up watching?
func nearerOf(t *testing.T, hx, vx float64) (target string, retargets, notices, checks int) {
	t.Helper()

	w := newSeekWorld(t, nil, nil)
	him, villager := &fakeQuarry{id: "p:1", x: hx}, &fakeQuarry{id: "v:1", x: vx, y: 0}
	w.quarries = []Quarry{him, villager}

	wolf := &fakeWatcher{id: "m:1"}
	w.notice.Watch(wolf, him)

	n0 := w.notice.watches["m:1"].notices

	w.minute()

	wt := w.notice.watches["m:1"]

	return w.targetOf("m:1"), w.seek.retargets, wt.notices - n0, wt.checks
}

func TestSeekTakesTheNearerOfTwoVisibleQuarries(t *testing.T) {
	target, retargets, notices, _ := nearerOf(t, 8, 5)
	require.Equal(t, "v:1", target, "the villager at 5 tiles is nearer than him at 8: the wolf turns to her")
	require.Equal(t, 1, retargets)
	require.Zero(t, notices, "a retarget is not a new notice: the watch was aware of him and is aware of her")

	// THE CONTROL (the brief's): the two quarries' positions swapped, and the
	// choice flips.
	target, retargets, _, _ = nearerOf(t, 5, 8)
	require.Equal(t, "p:1", target, "swapped: he is nearer, and the wolf keeps him")
	require.Zero(t, retargets)
}

// A quarry it cannot see -- behind a wall, or past the reach -- is no choice:
// the nearest IN SIGHT is the nearest.
func TestSeekChoosesOnlyWhatItCanSee(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block bool
		vx    float64
		want  string
	}{
		{"the villager in the open, nearer", false, 5, "v:1"},
		{"the villager nearer, behind a wall", true, 5, "p:1"},
		{"the villager past the reach (12 unlit)", false, 13, "p:1"},
	} {
		w := newSeekWorld(t, nil, nil)
		him, villager := &fakeQuarry{id: "p:1", x: 0, y: 8}, &fakeQuarry{id: "v:1", x: tc.vx, y: 0}

		if tc.block {
			w.sight.blocked[[2]float64{tc.vx, 0}] = true
		}

		w.quarries = []Quarry{villager, him}
		w.notice.Watch(&fakeWatcher{id: "m:1"}, him)
		w.minute()

		require.Equal(t, tc.want, w.targetOf("m:1"), tc.name)
		require.Equal(t, SeekLiving, w.row(t, "m:1").reason, tc.name)
	}
}

// Nothing living in sight: the watch is kept as it was, and Notice's memory
// runs on as ever.
func TestSeekKeepsTheWatchWithNothingInSight(t *testing.T) {
	w := newSeekWorld(t, nil, nil)
	him := &fakeQuarry{id: "p:1", x: 30}
	w.quarries = []Quarry{him, &fakeQuarry{id: "v:1", x: 20}}
	w.notice.Watch(&fakeWatcher{id: "m:1"}, him)
	w.minute()

	require.Equal(t, "p:1", w.targetOf("m:1"))
	require.Equal(t, SeekNone, w.row(t, "m:1").reason)
	require.Zero(t, w.seek.retargets)
}

// The watch's own target is always its own candidate: whoever made the watch
// made it a quarry, and a wolf watching the villager beside it does not turn
// to a man further off (the raid R1 scripts' B watching A, one tile apart).
func TestSeekKeepsTheWatchOnWhatItWasMadeFor(t *testing.T) {
	w := newSeekWorld(t, nil, nil)
	him, prey := &fakeQuarry{id: "p:1", x: 9}, &fakeQuarry{id: "a:1", x: 1}
	w.quarries = []Quarry{him} // a:1 is in no source: only the watch names it

	w.notice.Watch(&fakeWatcher{id: "m:1"}, prey)
	w.minute()

	require.Equal(t, "a:1", w.targetOf("m:1"))
	require.Zero(t, w.seek.retargets)

	// The control: the same prey further off than him, and the wolf turns.
	w2 := newSeekWorld(t, nil, nil)
	w2.quarries = []Quarry{him}
	w2.notice.Watch(&fakeWatcher{id: "m:1"}, &fakeQuarry{id: "a:1", x: 11})
	w2.minute()

	require.Equal(t, "p:1", w2.targetOf("m:1"))
}

// --- R2 assertion 2: a fighter keeps its target ------------------------------

// A monster in his fight keeps him when a nearer quarry appears (J-R1-3 (a)),
// and so does a monster in a fight he is not in; the control beside each is
// the same monster out of the fight, which turns.
func TestSeekAFighterKeepsItsTarget(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "m:2", 5000, Profile{})
	f.open(t)
	require.True(t, f.c.engaged("m:2"), "the fixture: m:2 is his enemy")

	w := newSeekWorld(t, f.c, nil)
	villager := &fakeQuarry{id: "v:9", x: 41, y: 40.4} // 0.4 from m:2; he is 1 off
	w.quarries = []Quarry{f.target, villager}

	// The fighter's rule alone: no margin (the review's B1 stickiness would
	// hold him for m:3 too, the villager only 0.6 nearer, and the control
	// below would prove nothing about the fight).
	w.seek.dials.SwitchMarginTiles = 0

	// The control, in the same world: m:3 stands where m:2 stands, aware of
	// him and not yet in the fight (combat has not stepped since).
	f.add(t, "m:3", 5000, Profile{})
	require.False(t, f.c.engaged("m:3"), "the fixture: m:3 is in no fight yet")

	w.minute()

	require.Equal(t, "p:1", w.targetOf("m:2"), "his enemy keeps him")
	require.Equal(t, SeekFighting, w.row(t, "m:2").reason)
	require.Equal(t, "v:9", w.targetOf("m:3"), "the control: out of the fight, the nearer villager takes the watch")

	// A fight he is not in: m:5 kills v:1 by the clock; a nearer stand-in
	// appears, and m:5 keeps v:1.
	g, s := pacedFight(t, nil)
	g.c.SetPlayer("p:1")
	v1 := clockQuarry(t, g, "v:1", 80, 40, 5000, "m:5", 5000)
	clockFrame(g, s, false)
	require.NotNil(t, clockFightFor(g, "v:1"), "the fixture: a clock fight on v:1")
	require.True(t, g.c.engaged("m:5"))

	gw := newSeekWorld(t, g.c, nil)
	gw.quarries = []Quarry{g.target, v1, &fakeQuarry{id: "v:7", x: 81, y: 40.3}}
	gw.minute()

	require.Equal(t, "v:1", gw.targetOf("m:5"), "a monster in a village fight finishes the job")
	require.Equal(t, SeekFighting, gw.row(t, "m:5").reason)
}

// --- R2 assertion 3: his sleep hides only him ---------------------------------

func TestHisSleepHidesOnlyHimInTheModel(t *testing.T) {
	n := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	n.SetPlayer("p:1")
	n.SetHidden(true)

	n.Watch(&fakeWatcher{id: "m:1"}, &fakeQuarry{id: "p:1", x: 5})
	n.Watch(&fakeWatcher{id: "m:2", y: 10}, &fakeQuarry{id: "v:1", x: 5, y: 10})

	noticed, _ := n.Noticed("m:1")
	require.False(t, noticed, "asleep, he is seen by nothing")

	noticed, _ = n.Noticed("m:2")
	require.True(t, noticed, "and the villager is seen as ever: his sleep hides him, not the village")

	// T6's first rule, kept for a model with no player bound (every unit
	// fixture before R2): hidden hides every quarry.
	legacy := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	legacy.SetHidden(true)
	legacy.Watch(&fakeWatcher{id: "m:2"}, &fakeQuarry{id: "v:1", x: 5})

	noticed, _ = legacy.Noticed("m:2")
	require.False(t, noticed, "unbound, hidden is T6's global rule")

	// And Seek: a wolf watching him while he sleeps turns to the villager it
	// can see -- he is no candidate while hidden.
	w := newSeekWorld(t, nil, nil)
	him, villager := &fakeQuarry{id: "p:1", x: 3}, &fakeQuarry{id: "v:1", x: 6}
	w.quarries = []Quarry{him, villager}
	w.notice.SetPlayer("p:1")
	w.notice.Watch(&fakeWatcher{id: "m:1"}, him)
	w.notice.SetHidden(true)
	w.minute()

	require.Equal(t, "v:1", w.targetOf("m:1"), "a hostile keeps seeing the villager while he sleeps")

	nt, _ := w.notice.Noticed("m:1")
	require.True(t, nt, "and is aware of her")
}

// --- R2 assertion 4: a living-side watch never makes a chase or a fight ----

func TestALivingWatchStartsNoFight(t *testing.T) {
	for _, bound := range []bool{true, false} {
		f := newResolverFight(t, 1462)
		if bound {
			f.c.SetPlayer("p:1")
		}

		// A villager on watch at 60,40 sees a monster beside her.
		villager := &fakeWatcher{id: "v:1", x: 60, y: 40}
		monster := &fakeQuarry{id: "m:2", x: 61, y: 40}
		f.bodies.known["v:1"] = &fakeBody{health: 60, maxHealth: 60}
		f.bodies.known["m:2"] = &fakeBody{health: 60, maxHealth: 60}

		require.True(t, f.notice.WatchAs(villager, monster, SideLiving))

		noticed, _ := f.notice.Noticed("v:1")
		require.True(t, noticed, "bound=%v: she sees it", bound)
		require.Empty(t, f.notice.AwarePairs(), "bound=%v: and no hostile pair is handed out to act on", bound)
		require.Equal(t, []string{"v:1"}, f.notice.AwareOf(SideLiving))
		require.Empty(t, f.notice.Aware(), "Aware and AwarePairs agree: hostile only")

		for i := 0; i < 5; i++ {
			f.c.Advance(1.0)
		}

		require.False(t, f.c.Fighting(), "bound=%v: the village's own side opened a fight", bound)
		require.Equal(t, 0, clockBlock(f)["started"], "bound=%v: or a clock fight", bound)

		w := newSeekWorld(t, f.c, nil)
		w.seek.Advance(1.0)
		require.Empty(t, w.seek.rows, "bound=%v: Seek chooses nothing for the living side", bound)
	}

	n := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	require.False(t, n.WatchAs(&fakeWatcher{id: "v:1"}, &fakeQuarry{id: "m:2"}, "neutral"), "a side the model does not know is refused")
	require.Zero(t, n.Count())
}

// --- S0-1 (a): a protected speaker is never a candidate -----------------------

func TestAProtectedSpeakerIsNoCandidate(t *testing.T) {
	run := func(protect bool) (string, int) {
		f := newResolverFight(t, 1462)
		f.c.SetPlayer("p:1")

		if protect {
			f.c.SetProtected(func(id string) bool { return id == "s:1" })
		}

		w := newSeekWorld(t, f.c, nil)
		speaker := &fakeQuarry{id: "s:1", x: 72, y: 40}
		w.quarries = []Quarry{f.target, speaker} // him 30 tiles off, past the reach

		w.notice.Watch(&fakeWatcher{id: "m:5", x: 70, y: 40}, f.target)
		w.minute()

		return w.targetOf("m:5"), w.seek.retargets
	}

	target, retargets := run(true)
	require.Equal(t, "p:1", target, "a hostile with only a speaker in view keeps its target")
	require.Zero(t, retargets)

	// THE CONTROL (the brief's): the protection lifted, and it retargets.
	target, retargets = run(false)
	require.Equal(t, "s:1", target, "unprotected, the speaker two tiles off takes the watch")
	require.Equal(t, 1, retargets)
}

// The dead are not served yet (their draw is P6, R5's): a risen man keeps the
// watch the tables gave him. The control beside it: a beast of the tables is
// served, and turns.
func TestSeekLeavesTheDeadToTheirOwnDraw(t *testing.T) {
	s, _, notice, _, him := newTestSpawns(t)

	risen := s.Raise(50, 40) // 10 from him
	require.NotEmpty(t, risen)

	s.spawn(DefaultSpawnDials().Rows[0], 1) // a pack of the first row, at 40+MinTiles

	seek := NewSeek(notice, s, nil, DefaultSeekDials())
	t.Cleanup(seek.Close)

	near := &fakeQuarry{id: "v:1", x: 50.5, y: 40}
	seek.SetQuarries(func() []Quarry { return []Quarry{him, near} })

	var beast string

	for _, id := range notice.watcherIDs() {
		if id != risen {
			beast = id
		}
	}

	require.NotEmpty(t, beast, "the fixture: a beast of the tables")

	// Place the beast where the villager is nearest to it too.
	notice.watches[beast].watcher.(*fakeWatcher).x = 50
	notice.watches[beast].watcher.(*fakeWatcher).y = 40.2

	seek.Advance(1.0 / 24) // the rows are made, each on its phase
	seek.Advance(1.0)      // and a minute on, every one has looked

	_, served := seek.rows[risen]
	require.False(t, served, "the dead have no row")
	require.Equal(t, "p:1", notice.watches[risen].target.QuarryID(), "the risen keeps the watch the tables gave it")
	require.Equal(t, "v:1", notice.watches[beast].target.QuarryID(), "the control: a beast turns to the nearer villager")
}

// --- The stagger (S0's M0.2): a minute of looks spread across its frames ------

func seekLooksPerFrame(t *testing.T, slots, watchers int) []int {
	t.Helper()

	w := newSeekWorld(t, nil, nil)
	w.seek.dials.StaggerSlots = slots
	him := &fakeQuarry{id: "p:1", x: 500, y: 500}

	for i := 0; i < watchers; i++ {
		w.notice.Watch(&fakeWatcher{id: fmt.Sprintf("w:%03d", i), y: float64(i)}, him)
	}

	out := make([]int, 0, 48)

	for f := 0; f < 48; f++ {
		l0 := w.seek.looks
		w.notice.Advance(1.0 / 24)
		w.seek.Advance(1.0 / 24)
		out = append(out, w.seek.looks-l0)
	}

	return out
}

func TestSeekStaggersItsLooksAcrossTheMinute(t *testing.T) {
	per := seekLooksPerFrame(t, 24, 48)

	total := 0
	for i, n := range per {
		require.LessOrEqual(t, n, 2, "frame %d: %d looks; 48 watchers over 24 phases is 2 a frame", i, n)
		total += n
	}

	require.Equal(t, 96, total, "every watcher looks once a minute, two minutes")

	// THE CONTROL: no stagger (one phase), and the whole pack looks in one
	// frame -- the worst frame the benchmark measures unstaggered.
	per = seekLooksPerFrame(t, 1, 48)
	require.Equal(t, 48, per[0], "unstaggered, all 48 look in the first frame")
	require.Equal(t, 0, per[1])
}

// A long step -- a sleep's hour -- looks once, and keeps the row's phase.
func TestSeekLooksOnceInALongStep(t *testing.T) {
	w := newSeekWorld(t, nil, nil)
	w.notice.Watch(&fakeWatcher{id: "m:1"}, &fakeQuarry{id: "p:1", x: 500})
	w.notice.Watch(&fakeWatcher{id: "m:2"}, &fakeQuarry{id: "p:1", x: 500})

	w.seek.Advance(1.0 / 24) // m:1 looks (phase 0); m:2 is made at phase 1/24
	require.Equal(t, 1, w.seek.looks)

	u2 := w.seek.rows["m:2"].untilLook

	w.seek.Advance(60)
	require.Equal(t, 3, w.seek.looks, "an hour in one step: one look each")
	require.InDelta(t, u2, w.seek.rows["m:2"].untilLook, 1e-9, "m:2 kept its phase")
	require.Greater(t, w.seek.rows["m:1"].untilLook, 0.0)
}

// The stand-in collection: a verb that fills it and one that empties it.
func TestSeekStandInsFillAndEmpty(t *testing.T) {
	w := newSeekWorld(t, nil, nil)

	require.NoError(t, w.seek.HarnessSet("stand_in", "v:1"))
	require.Error(t, w.seek.HarnessSet("stand_in", "v:1"), "named twice")
	require.Error(t, w.seek.HarnessSet("stand_in", PlayerRef), "he is no stand-in")
	require.Equal(t, []string{"v:1"}, w.seek.HarnessState()["stand_ins"])
	require.NoError(t, w.seek.HarnessSet("stand_in_remove", "v:1"))
	require.Error(t, w.seek.HarnessSet("stand_in_remove", "v:1"), "not a stand-in any more")
	require.Error(t, w.seek.HarnessSet("stagger_slots", 0.0))
	require.Error(t, w.seek.HarnessSet("retarget_minutes", -1.0))
	require.NoError(t, w.seek.HarnessSet("retarget_minutes", 0.5))
	require.Error(t, w.seek.HarnessSet("route_solves", 3.0), "no A* inside a frame: not a dial here")
}
