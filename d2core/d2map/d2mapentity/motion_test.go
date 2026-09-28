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

	for _, m := range []mutation{
		{"pos lost", false, func(mo *Motion) { mo.Pos = [2]float64{} }},
		{"target lost", false, func(mo *Motion) { mo.Target = [2]float64{} }},
		{"velocity lost", false, func(mo *Motion) { mo.Velocity = [2]float64{} }},
		{"path lost", false, func(mo *Motion) { mo.Path = nil }},
		{"a waypoint lost", false, func(mo *Motion) { mo.Path = mo.Path[1:] }},
		{"speed lost", false, func(mo *Motion) { mo.Speed = 0 }},
		{"dir altered", false, func(mo *Motion) { mo.Dir = (mo.Dir + 3) % 8 }},
		{"mode altered", false, func(mo *Motion) { mo.Mode = "idle" }},
		{"an action added", false, func(mo *Motion) { mo.Action = "attack" }},
		{"corpse added", false, func(mo *Motion) { mo.Corpse = true }},
		{"mode lost", true, func(mo *Motion) { mo.Mode = "" }},
		{"mode unknown", true, func(mo *Motion) { mo.Mode = "WL" }},
		{"action unknown", true, func(mo *Motion) { mo.Action = "fly" }},
		{"speed negative", true, func(mo *Motion) { mo.Speed = -1 }},
		{"pos NaN", true, func(mo *Motion) { mo.Pos[0] = math.NaN() }},
		{"waypoint infinite", true, func(mo *Motion) { mo.Path[0][1] = math.Inf(1) }},
		{"corpse with an action", true, func(mo *Motion) { mo.Corpse, mo.Action = true, "attack" }},
	} {
		changed := mo
		changed.Path = append([][2]float64(nil), mo.Path...)
		m.mutate(&changed)

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

// A dog saved the moment it starts a bite resumes the bite and finishes it on
// the same frame. (Saved MID-bite, it would restart the bite: the frame is
// not carried -- see Motion.)
func TestCreatureMotionResumesAnActionFromItsStart(t *testing.T) {
	a := b2bWalker(t)
	require.NoError(t, a.StartAction(d2enum.MonsterAnimationModeAttack1, nil))

	mo := a.MotionSnapshot()
	require.Equal(t, "attack", mo.Action)

	b, err := b2bRebuild(t, a.ID(), mo)
	require.NoError(t, err)
	require.True(t, b.held)

	want := b2bRun(t, a, 60)
	got := b2bRun(t, b, 60)
	require.Equal(t, -1, b2bFirstDiff(want, got))
	require.False(t, a.held, "the bite finished within the run")
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
	require.Error(t, b.RestoreMotion(Motion{Action: "ZZ"}), "an unknown held mode is refused")
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
