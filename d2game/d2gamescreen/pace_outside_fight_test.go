package d2gamescreen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
)

// TestNoPaceIsBorrowedOutsideAFight pins what lets the world save leave a
// walk's arrival callback behind (M4.6 B2b review; d2mapentity Motion).
//
// A paced fight's walk is handed borrowPace's return as its callback, and the
// save cannot carry a function. It needs not: saves are refused during a
// fight, and on every live frame with no fight running tacticalAdvance calls
// returnAllPace, which gives each borrowed pace back and EMPTIES the table the
// callback reads -- so the callback, fired later, finds nothing held and does
// nothing. This holds that frame to it: a creature left walking at the
// borrowed pace when its fight ended has its own speed back, and nothing is
// held, after one frame.
func TestNoPaceIsBorrowedOutsideAFight(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	require.NoError(t, err)
	require.NoError(t, asset.AddSource(filepath.Join("..", ".."), types.AssetSourceFileSystem))

	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, asset)
	engine.ResetAuthoredMap(d2enum.RegionAct1Town, 40, 40)

	dog, err := engine.MapEntityFactory.NewCreature(50, 50, "Feral dog", d2mapentity.CreatureAnimationPaths{
		Idle: "/data/strigoi/creatures/feral-dog/idle.png",
		Walk: "/data/strigoi/creatures/feral-dog/walk.png",
	}, 0, nil)
	require.NoError(t, err)
	engine.AddEntity(dog)

	const own = 2.0

	dog.SetSpeed(own)

	clock := d2world.NewClock(d2world.DefaultClockDials())
	t.Cleanup(clock.Close)

	v := &Game{gameClient: &d2client.GameClient{MapEngine: engine}}
	v.combat = d2world.NewCombat(clock, nil, nil, nil, v, nil, nil, nil, nil, 1, d2world.DefaultCombatDials())
	t.Cleanup(v.combat.Close)

	// A paced walk, as StepToward orders one: the pace borrowed, its return
	// handed to the walk as the arrival callback.
	back := v.borrowPace(dog.ID(), dog)
	require.NotNil(t, back, "the control: a slow body borrows the tactical pace")
	dog.SetPath([]d2vector.Position{d2vector.NewPositionTile(20, 20)}, back)
	v.tacticalReserved = map[string][2]int{dog.ID(): {20, 20}}

	require.Equal(t, tacticalWalkSpeed, dog.GetSpeed(), "walking at the borrowed pace")
	require.Len(t, v.tacticalPace, 1)
	require.False(t, v.combat.Fighting(), "no fight is running")

	v.tacticalAdvance(1.0 / 60)

	require.Empty(t, v.tacticalPace, "outside a fight, one frame hands every borrowed pace back")
	require.Empty(t, v.tacticalReserved)
	require.Equal(t, own, dog.GetSpeed(), "at its own speed again")

	// And the callback the walk still carries is now a no-op: fired, it
	// changes nothing -- which is why a save may drop it.
	dog.SetSpeed(own + 1)
	back()
	require.Equal(t, own+1, dog.GetSpeed(), "the leftover callback holds nothing to give back")
}
