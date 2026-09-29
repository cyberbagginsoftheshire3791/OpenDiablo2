package d2mapentity

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// M4.6 B2b: the motion snapshot, on a real creature built by the factory from
// the repo's own sheets. A save rebuilds it in ANOTHER factory -- a relaunch
// -- with its saved id (the seam) and its saved motion, and the two must then
// take the same steps.

const b2bFrame = 0.04 // one frame at 25 fps, in seconds

// b2bFrames is how long a rebuilt creature is run beside the saved one: long
// enough to walk the rest of its route and stand.
const b2bFrames = 200

// b2bSeen is everything a script, the digest or the renderer can see of a
// creature, exactly: its harness state, its velocity and its raw position.
func b2bSeen(t *testing.T, c *Creature) string {
	t.Helper()

	v := c.GetVelocity()

	b, err := json.Marshal(map[string]interface{}{
		"state":    c.HarnessState(),
		"id":       c.ID(),
		"velocity": [2]float64{v.X(), v.Y()},
		"pos":      [2]float64{c.Position.X(), c.Position.Y()},
	})
	require.NoError(t, err)

	return string(b)
}

// b2bRun is what a creature shows now and after each of n frames.
func b2bRun(t *testing.T, c *Creature, n int) []string {
	t.Helper()

	out := []string{b2bSeen(t, c)}

	for i := 0; i < n; i++ {
		c.Advance(b2bFrame)
		out = append(out, b2bSeen(t, c))
	}

	return out
}

func b2bFirstDiff(a, b []string) int {
	for i := range a {
		if a[i] != b[i] {
			return i
		}
	}

	return -1
}

// b2bWalker is a dog part-way along a four-waypoint walk: a current target,
// waypoints ahead, a velocity and a facing, all mid-stride.
func b2bWalker(t *testing.T) *Creature {
	t.Helper()

	c := b2bNewDog(t, b2bFactory(t), 50, 50)
	c.SetSpeed(7)
	c.SetPath([]d2vector.Position{
		d2vector.NewPosition(58, 53), d2vector.NewPosition(64, 60),
		d2vector.NewPosition(60, 71), d2vector.NewPosition(52, 75),
	}, nil)

	for i := 0; i < 9; i++ {
		c.Advance(b2bFrame)
	}

	require.True(t, c.IsMoving())
	require.NotEmpty(t, c.path, "waypoints still ahead")
	require.Equal(t, "walk", string(c.mode))

	return c
}

// b2bRebuild is the load: a fresh factory (a relaunch), the saved id through
// the seam, then the saved motion -- read back through JSON, as the world
// file will.
func b2bRebuild(t *testing.T, id string, mo Motion) (*Creature, error) {
	t.Helper()

	b, err := json.Marshal(mo)
	require.NoError(t, err)

	var back Motion
	require.NoError(t, json.Unmarshal(b, &back))

	f := b2bFactory(t)
	require.NoError(t, f.SetNextEntityID(id))

	c := b2bNewDog(t, f, 0, 0) // built anywhere; the motion puts it where it was

	return c, c.RestoreMotion(back)
}

// Round trip, and the three steps a relaunch must take in step.
func TestCreatureMotionRoundTripsAndAdvancesInStep(t *testing.T) {
	a := b2bWalker(t)
	mo := a.MotionSnapshot()

	b, err := b2bRebuild(t, a.ID(), mo)
	require.NoError(t, err)

	require.Equal(t, a.ID(), b.ID(), "rebuilt with its saved id")
	require.Equal(t, b2bSeen(t, a), b2bSeen(t, b), "the rebuilt dog is the saved one")
	require.Equal(t, mo, b.MotionSnapshot(), "and snapshots to the same motion")

	want := b2bRun(t, a, b2bFrames)
	got := b2bRun(t, b, b2bFrames)
	require.Equal(t, -1, b2bFirstDiff(want, got), "the rebuilt dog left the saved one's path")

	// It walked to the end of its route and stood: the run covered the walk.
	require.False(t, a.IsMoving())
	require.NotEqual(t, want[0], want[len(want)-1])
}

// Every field, lost or altered, is seen -- or refused before anything changes.
func TestCreatureMotionEveryFieldIsSeen(t *testing.T) {
	a := b2bWalker(t)
	mo := a.MotionSnapshot()
	want := b2bRun(t, b2bMust(b2bRebuild(t, a.ID(), mo)), b2bFrames)

	type mutation struct {
		name   string
		refuse bool
		mutate func(*Motion)
	}

	seen := map[string]bool{}

	for _, m := range []mutation{
		{"pos lost", false, func(mo *Motion) { mo.Pos = [2]float64{} }},
		{"target lost", false, func(mo *Motion) { mo.Target = [2]float64{} }},
		{"velocity lost", false, func(mo *Motion) { mo.Velocity = [2]float64{} }},
		{"path lost", false, func(mo *Motion) { mo.Path = nil }},
		{"a waypoint lost", false, func(mo *Motion) { mo.Path = mo.Path[1:] }},
		{"speed lost", false, func(mo *Motion) { mo.Speed = 0 }},
		{"dir altered", false, func(mo *Motion) { mo.Dir = (mo.Dir + 3) % 8 }},
		{"mode altered", false, func(mo *Motion) { mo.Mode = "idle" }},
		{"an action added", false, func(mo *Motion) { mo.Action, mo.ActionAt = "attack", &ActionProgress{Frame: 3} }},
		{"corpse added", false, func(mo *Motion) { mo.Corpse = true }},
		{"mode lost", true, func(mo *Motion) { mo.Mode = "" }},
		{"mode unknown", true, func(mo *Motion) { mo.Mode = "WL" }},
		{"action unknown", true, func(mo *Motion) { mo.Action = "fly" }},
		{"speed negative", true, func(mo *Motion) { mo.Speed = -1 }},
		{"pos NaN", true, func(mo *Motion) { mo.Pos[0] = math.NaN() }},
		{"waypoint infinite", true, func(mo *Motion) { mo.Path[0][1] = math.Inf(1) }},
		{"corpse with an action", true, func(mo *Motion) { mo.Corpse, mo.Action, mo.ActionAt = true, "attack", &ActionProgress{} }},
		// BUG-87: a held action is carried at its frame, and only a held
		// action is.
		{"an action with no frame", true, func(mo *Motion) { mo.Action = "attack" }},
		{"a frame and no action", true, func(mo *Motion) { mo.ActionAt = &ActionProgress{Frame: 2} }},
		{"a frame the sheet lacks", true, func(mo *Motion) { mo.Action, mo.ActionAt = "attack", &ActionProgress{Frame: 10} }},
		{"a negative frame", true, func(mo *Motion) { mo.Action, mo.ActionAt = "attack", &ActionProgress{Frame: -1} }},
		{"a NaN time into a frame", true, func(mo *Motion) { mo.Action, mo.ActionAt = "attack", &ActionProgress{Elapsed: math.NaN()} }},
	} {
		changed := mo
		changed.Path = append([][2]float64(nil), mo.Path...)
		m.mutate(&changed)

		for _, key := range b2bMotionKeysChanged(mo, changed) {
			seen[key] = true
		}

		f := b2bFactory(t)
		require.NoError(t, f.SetNextEntityID(a.ID()))
		c := b2bNewDog(t, f, 0, 0)
		before := b2bSeen(t, c)

		// Straight into RestoreMotion, not through JSON: NaN cannot be
		// written to JSON, and a restore must refuse it rather than panic.
		err := c.RestoreMotion(changed)

		if m.refuse {
			require.Error(t, err, m.name)
			require.Equal(t, before, b2bSeen(t, c), "%s: a refused motion must change nothing", m.name)
			t.Logf("%-24s refused: %v", m.name, err)

			continue
		}

		require.NoError(t, err, m.name)

		at := b2bFirstDiff(want, b2bRun(t, c, b2bFrames))
		require.GreaterOrEqual(t, at, 0, "%s: LOST WITHOUT A TRACE -- a hole in observability, not a pass", m.name)
		t.Logf("%-24s diverged at frame %d", m.name, at)
	}

	// Every field labelled saved is exercised by a mutation above (the B2b
	// review's B5, with the B2a review's B3 teeth).
	b2bMotionExercised(t, seen, b2bMapEntityClass(), b2bCreatureClass())
}

func b2bMust(c *Creature, err error) *Creature {
	if err != nil {
		panic(fmt.Sprintf("rebuild: %v", err))
	}

	return c
}

// Velocity is the one field a Step recomputes before it uses it: it is saved
// for the renderer's debug overlay (GetVelocity), which is the only thing that
// reads it between two frames. Pinned so the claim stays true.
func TestCreatureMotionVelocityIsOnlySeenBeforeTheNextStep(t *testing.T) {
	a := b2bWalker(t)
	mo := a.MotionSnapshot()
	want := b2bRun(t, b2bMust(b2bRebuild(t, a.ID(), mo)), b2bFrames)

	lost := mo
	lost.Velocity = [2]float64{}

	got := b2bRun(t, b2bMust(b2bRebuild(t, a.ID(), lost)), b2bFrames)
	require.Equal(t, 0, b2bFirstDiff(want, got), "seen at the load")
	require.Equal(t, want[1:], got[1:], "and never again: the first step recomputes it")
}

// b2bTick is the harness's frame, 1/60 s: the tick every playtest steps, and
// the one BUG-87's measurements were made at. (b2bFrame, 25 fps, is B2b's.)
const b2bTick = 1.0 / 60

// b2bRunAt is b2bRun at another tick.
func b2bRunAt(t *testing.T, c *Creature, n int, tick float64) []string {
	t.Helper()

	out := []string{b2bSeen(t, c)}

	for i := 0; i < n; i++ {
		c.Advance(tick)
		out = append(out, b2bSeen(t, c))
	}

	return out
}

// b2bHeldActions are the three actions the fight plays on a monster
// (Game.Animate): a swing, a blow taken, a death.
var b2bHeldActions = []struct {
	name string
	mode d2enum.MonsterAnimationMode
	held creatureMode
}{
	{"a bite", d2enum.MonsterAnimationModeAttack1, creatureAttack},
	{"a blow taken", d2enum.MonsterAnimationModeGetHit, creatureHit},
	{"a death", d2enum.MonsterAnimationModeDeath, creatureDeath},
}

// b2bHeldTicks is how many ticks a fresh walker holds the action.
func b2bHeldTicks(t *testing.T, mode d2enum.MonsterAnimationMode) int {
	t.Helper()

	c := b2bWalker(t)
	require.NoError(t, c.StartAction(mode, nil))

	n := 0
	for ; n < 1000 && c.held; n++ {
		c.Advance(b2bTick)
	}

	require.False(t, c.held)

	return n
}

// A HELD ACTION RESUMES AT ITS FRAME (M4.6 BUG-87). A dog saved mid-bite,
// mid-blow and mid-death -- on its first tick, a tick in, a sheet frame in,
// half-way, and on the last tick before it ends -- is rebuilt in another
// factory from the motion read back through JSON, and is the saved dog on
// every frame after: the same sheet frame, the same time into it (both in its
// harness state), the action ending on the same frame -- back to idle and on
// walking, or the Dead pose and a corpse where it fell -- and the same steps.
// (B2b's version of this test could only save the FIRST tick: the frame was
// not carried, and a dog saved mid-bite restarted the bite.)
func TestCreatureMotionResumesAHeldActionAtItsFrame(t *testing.T) {
	for _, act := range b2bHeldActions {
		ticks := b2bHeldTicks(t, act.mode)

		for _, in := range []int{0, 1, 7, ticks / 2, ticks - 1} {
			a := b2bWalker(t)
			require.NoError(t, a.StartAction(act.mode, nil))

			for i := 0; i < in; i++ {
				a.Advance(b2bTick)
			}

			require.True(t, a.held, "%s, %d ticks in: still held", act.name, in)

			mo := a.MotionSnapshot()
			require.Equal(t, string(act.held), mo.Action)
			require.NotNil(t, mo.ActionAt, "%s: a held action is carried at its frame", act.name)

			frame, elapsed := a.animation.Progress()
			require.Equal(t, ActionProgress{Frame: frame, Elapsed: elapsed}, *mo.ActionAt)

			b, err := b2bRebuild(t, a.ID(), mo)
			require.NoError(t, err, "%s, %d ticks in", act.name, in)
			require.Equal(t, mo, b.MotionSnapshot(), "%s, %d ticks in: read back equal", act.name, in)

			want := b2bRunAt(t, a, ticks+60, b2bTick)
			got := b2bRunAt(t, b, ticks+60, b2bTick)
			require.Equal(t, -1, b2bFirstDiff(want, got), "%s, %d ticks in: the rebuilt dog left the saved one", act.name, in)

			// The run covered the action's end: a finished action ends
			// identically.
			require.False(t, a.held, "%s: the action ended within the run", act.name)
			require.Equal(t, act.mode == d2enum.MonsterAnimationModeDeath, a.corpse, "%s", act.name)
			require.Equal(t, a.corpse, b.corpse)
		}

		t.Logf("%s: held %d ticks at 1/60 s; resumed at its first, second, seventh, middle and last tick, each the saved dog to the end", act.name, ticks)
	}
}

// A HELD ACTION IS ALWAYS ON ITS FIRST PLAY (BUG-87), which is why the play
// count is not carried: Advance ends the action on the tick the count reaches
// 1, so at every tick a save can see, a held action's count is 0.
func TestAHeldActionIsAlwaysOnItsFirstPlay(t *testing.T) {
	for _, act := range b2bHeldActions {
		c := b2bWalker(t)
		require.NoError(t, c.StartAction(act.mode, nil))

		for i := 0; i < 1000 && c.held; i++ {
			require.Zero(t, c.animation.GetPlayedCount(), "%s, tick %d", act.name, i)
			c.Advance(b2bTick)
		}

		require.False(t, c.held)
	}
}

// EVERY PART OF A HELD ACTION, LOST OR ALTERED, IS SEEN -- or refused before
// anything changes (BUG-87). The dog is mid-bite, its sheet a frame in and
// part-way through it; each mutation of the held action, rebuilt, must leave
// the saved dog's path or be refused.
func TestCreatureHeldMotionEveryFieldIsSeen(t *testing.T) {
	a := b2bWalker(t)
	require.NoError(t, a.StartAction(d2enum.MonsterAnimationModeAttack1, nil))

	for i := 0; i < 9; i++ {
		a.Advance(b2bTick)
	}

	mo := a.MotionSnapshot()
	require.Positive(t, mo.ActionAt.Frame, "a frame in")
	require.Positive(t, mo.ActionAt.Elapsed, "and part-way through it")

	want := b2bRunAt(t, b2bMust(b2bRebuild(t, a.ID(), mo)), 120, b2bTick)

	for _, m := range []struct {
		name   string
		refuse bool
		mutate func(*Motion)
	}{
		{"the frame put back to the first", false, func(mo *Motion) { mo.ActionAt.Frame = 0 }},
		{"the frame moved on", false, func(mo *Motion) { mo.ActionAt.Frame++ }},
		{"the time into the frame lost", false, func(mo *Motion) { mo.ActionAt.Elapsed = 0 }},
		{"the time into the frame a hair off", false, func(mo *Motion) { mo.ActionAt.Elapsed = math.Nextafter(mo.ActionAt.Elapsed, 1) }},
		{"another action", false, func(mo *Motion) { mo.Action = "hit" }},
		{"the held action dropped", false, func(mo *Motion) { mo.Action, mo.ActionAt = "", nil }},
		{"the frame dropped", true, func(mo *Motion) { mo.ActionAt = nil }},
		{"a frame the sheet lacks", true, func(mo *Motion) { mo.ActionAt.Frame = 10 }},
	} {
		changed := mo
		changed.Path = append([][2]float64(nil), mo.Path...)
		at := *mo.ActionAt
		changed.ActionAt = &at
		m.mutate(&changed)

		f := b2bFactory(t)
		require.NoError(t, f.SetNextEntityID(a.ID()))
		c := b2bNewDog(t, f, 0, 0)
		before := b2bSeen(t, c)

		err := c.RestoreMotion(changed)

		if m.refuse {
			require.Error(t, err, m.name)
			require.Equal(t, before, b2bSeen(t, c), "%s: a refused motion must change nothing", m.name)
			t.Logf("%-36s refused: %v", m.name, err)

			continue
		}

		require.NoError(t, err, m.name)

		at2 := b2bFirstDiff(want, b2bRunAt(t, c, 120, b2bTick))
		require.GreaterOrEqual(t, at2, 0, "%s: LOST WITHOUT A TRACE", m.name)
		t.Logf("%-36s diverged at tick %d", m.name, at2)
	}
}

// A dead dog comes back dead: in the Dead pose, and it does not walk.
func TestCreatureMotionKeepsACorpseDead(t *testing.T) {
	a := b2bWalker(t)
	require.NoError(t, a.StartAction(d2enum.MonsterAnimationModeDeath, nil))

	for i := 0; i < 60 && !a.corpse; i++ {
		a.Advance(b2bFrame)
	}

	require.True(t, a.corpse)

	mo := a.MotionSnapshot()
	require.True(t, mo.Corpse)

	b, err := b2bRebuild(t, a.ID(), mo)
	require.NoError(t, err)

	want := b2bRun(t, a, 30)
	got := b2bRun(t, b, 30)
	require.Equal(t, -1, b2bFirstDiff(want, got))
	require.Equal(t, "dead", string(b.mode))
}

// An NPC's walk round-trips too. With no composite (no MPQs) there is no
// animation to pose, so it is stepped as a map entity.
func TestNPCMotionRoundTripsAndStepsInStep(t *testing.T) {
	a := &NPC{mapEntity: newMapEntity(50, 50)}
	a.SetSpeed(6)
	a.SetPath([]d2vector.Position{d2vector.NewPosition(60, 55), d2vector.NewPosition(70, 58)}, nil)

	for i := 0; i < 5; i++ {
		a.Step(b2bFrame)
	}

	mo := a.MotionSnapshot()

	b := &NPC{mapEntity: newMapEntity(0, 0)}
	require.NoError(t, b.RestoreMotion(mo))
	require.Equal(t, a.HarnessState(), b.HarnessState())
	require.Equal(t, mo, b.MotionSnapshot())

	for i := 0; i < 60; i++ {
		a.Step(b2bFrame)
		b.Step(b2bFrame)
		require.Equal(t, a.HarnessState(), b.HarnessState(), "frame %d", i)
	}

	require.Error(t, b.RestoreMotion(Motion{Speed: -1}), "refused")
	require.Error(t, b.RestoreMotion(Motion{Action: "ZZ", ActionAt: &ActionProgress{}}), "an unknown held mode is refused")
	require.Error(t, b.RestoreMotion(Motion{Action: "A1"}), "a held mode with no frame is refused (BUG-87)")
	require.Error(t, b.RestoreMotion(Motion{ActionAt: &ActionProgress{}}), "a frame with no held mode is refused (BUG-87)")
}

// An NPC's pose flags -- a corpse, a held action -- round-trip, and each one
// lost changes what a re-save writes. With no composite (no MPQs) an NPC
// cannot Advance, so this is the snapshot half only: the creature tests above
// carry the behaviour of the same two flags, and B4b's playtest carries the
// composite.
func TestNPCMotionCarriesItsPose(t *testing.T) {
	npcSeen := map[string]bool{}

	for _, a := range []*NPC{
		{mapEntity: newMapEntity(50, 50), corpse: true},
		{mapEntity: newMapEntity(50, 50), held: true, heldMode: d2enum.MonsterAnimationModeAttack1},
	} {
		mo := a.MotionSnapshot()

		b := &NPC{mapEntity: newMapEntity(0, 0)}
		require.NoError(t, b.RestoreMotion(mo))
		require.Equal(t, a.corpse, b.corpse)
		require.Equal(t, a.held, b.held)
		require.Equal(t, a.heldMode, b.heldMode)
		require.Equal(t, mo, b.MotionSnapshot(), "a re-save writes what was loaded")

		lost := mo
		lost.Corpse, lost.Action, lost.ActionAt = false, "", nil

		c := &NPC{mapEntity: newMapEntity(0, 0)}
		require.NoError(t, c.RestoreMotion(lost))
		require.NotEqual(t, mo, c.MotionSnapshot(), "the pose lost is seen")

		for _, key := range b2bMotionKeysChanged(mo, lost) {
			npcSeen[key] = true
		}
	}

	b2bMotionExercised(t, npcSeen, b2bNPCClass())

	held := &NPC{mapEntity: newMapEntity(0, 0), held: true, heldMode: d2enum.MonsterAnimationModeAttack1}
	require.Equal(t, "A1", held.MotionSnapshot().Action, "the held mode is written by its two letters")
	require.Equal(t, &ActionProgress{}, held.MotionSnapshot().ActionAt,
		"and carried at its frame: with no composite (no MPQs), the first (BUG-87; d2asset's TestACompositeResumesAtItsProgress carries the composite)")
	require.Error(t, (&NPC{mapEntity: newMapEntity(0, 0)}).RestoreMotion(Motion{Corpse: true, Action: "A1", ActionAt: &ActionProgress{}}),
		"a corpse holds no action")
}

// The NPC's mode names read back: every monster mode's two letters parse to a
// mode that prints them again.
func TestMonsterModeNamesReadBack(t *testing.T) {
	for m := d2enum.MonsterAnimationModeDeath; m <= d2enum.MonsterAnimationModeRun; m++ {
		got, ok := monsterModeNamed(m.String())
		require.True(t, ok, m.String())
		require.Equal(t, m.String(), got.String())
	}

	_, ok := monsterModeNamed("walk")
	require.False(t, ok)
}
