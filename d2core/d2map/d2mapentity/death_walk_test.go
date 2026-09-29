package d2mapentity

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// THE B4b REVIEW FIXES (29 Sep 2026), BUG-75: A DEATH ENDS THE WALK WHERE IT
// BEGINS. The review's B1 found a monster slain on its way in -- the quick
// resolve's last quarter, still walking a chase -- walking on through its
// whole death: its corpse came to rest up to a tile further along its route
// than where the resolver recorded its fall (Combat.fallCorpse). A dog
// mid-walk is killed here: from the first frame of its death it has no route,
// no target but where it stands, no velocity, and it faces the way it was
// going; it does not move through the death, not even along a route handed
// to it while it dies; and its corpse lies where it fell.
func TestADeathEndsTheWalkWhereItBegins(t *testing.T) {
	c := b2bWalker(t)
	fell := [2]float64{c.Position.X(), c.Position.Y()}
	facing := c.direction

	require.NoError(t, c.StartAction(d2enum.MonsterAnimationModeDeath, nil))

	mo := c.MotionSnapshot()
	require.Equal(t, "death", mo.Action)
	require.Equal(t, fell, mo.Pos)
	require.Equal(t, fell, mo.Target, "no step ahead")
	require.Empty(t, mo.Path, "no route")
	require.Equal(t, [2]float64{}, mo.Velocity, "no velocity")
	require.False(t, c.IsMoving())
	require.Equal(t, facing, c.direction, "it falls facing the way it was going")

	// A route handed to a monster while it dies is not walked (Advance's
	// guard): nothing may carry a dying body along.
	c.SetPath([]d2vector.Position{d2vector.NewPosition(80, 80), d2vector.NewPosition(90, 90)}, nil)

	for i := 0; i < 100 && !c.corpse; i++ {
		c.Advance(b2bFrame)
		require.Equal(t, fell, [2]float64{c.Position.X(), c.Position.Y()}, "frame %d of the death: it does not move", i)
	}

	require.True(t, c.corpse, "the death played through")
	require.Equal(t, fell, c.MotionSnapshot().Pos, "the corpse lies where it fell")
	require.Empty(t, c.MotionSnapshot().Path)

	// And a corpse stays put, as it always has.
	for i := 0; i < 30; i++ {
		c.Advance(b2bFrame)
	}

	require.Equal(t, fell, c.MotionSnapshot().Pos)
}

// Any other action does not end the walk: a bite or a blow taken is played
// through with the route kept (the fight's own pacing, not this, decides
// whether a body walks while it swings).
func TestOnlyADeathEndsTheWalk(t *testing.T) {
	for _, mode := range []d2enum.MonsterAnimationMode{d2enum.MonsterAnimationModeAttack1, d2enum.MonsterAnimationModeGetHit} {
		c := b2bWalker(t)
		before := c.MotionSnapshot()

		require.NoError(t, c.StartAction(mode, nil))

		after := c.MotionSnapshot()
		require.Equal(t, before.Path, after.Path, "%v: the route is kept", mode)
		require.Equal(t, before.Target, after.Target, "%v: the target is kept", mode)
		require.True(t, c.IsMoving(), "%v", mode)
	}
}
