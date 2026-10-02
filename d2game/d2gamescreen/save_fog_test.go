package d2gamescreen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// FOG OF WAR F3, "KEPT": the explored grid goes through the world file -- the
// save takes it, the load puts it back -- and what he sees now does not.

// TestAFoggedGameIsKept: a game with fog on looks from (10,10) and has a patch
// explored by hand at (30,30); it saves, and a new game loading the file
// remembers exactly that ground, keyed on the same map, and sees nothing
// until its first fog update -- which sees from where he stands.
//
// Negative control (1 Oct 2026, strigoi-harness-runs\wt-fog3\nc\): take the
// fog restore out of resumeLoad and this fails, the resumed grid empty (nc5).
func TestAFoggedGameIsKept(t *testing.T) {
	saved, save := b4Game(t)
	saved.fog = newGameFog(mapTileSight{saved}, true)

	w, h := saved.fogMapSize()
	require.Equal(t, [2]int{40, 40}, [2]int{w, h}, "the fixture's map")

	saved.fog.fog.Update(w, h, []d2world.Eye{{ID: fogEyeID, X: 10.5, Y: 10.5}})
	saved.fog.fog.Explore(30.5, 30.5, 3)

	explored, _ := saved.fog.fog.Counts()
	require.Greater(t, explored, 100, "he has seen ground")

	file, _ := b4File(t, saved, save, "t.world.json")
	require.False(t, file.Fog.Empty(), "the file keeps the grid")
	require.Equal(t, saved.fog.fog.Snapshot(fogMapID()), file.Fog)
	require.Equal(t, file.Map.SHA, file.Fog.Map, "keyed on the file's map")
	require.Equal(t, [2]int{40, 40}, [2]int{file.Fog.W, file.Fog.H})

	resumed, _ := b4Game(t)
	resumed.fog = newGameFog(mapTileSight{resumed}, true)
	require.NoError(t, b4Load(t, resumed, file))
	require.Contains(t, resumed.loadSteps, "fog")

	require.Equal(t, file.Fog, resumed.fog.fog.Snapshot(fogMapID()), "the resumed game remembers the same ground")

	got, visible := resumed.fog.fog.Counts()
	require.Equal(t, explored, got)
	require.Zero(t, visible, "what he saw is not restored: it is seen again")
	require.Equal(t, d2world.FogExplored, resumed.fog.fog.At(30, 30), "the patch explored by hand")
	require.Equal(t, d2world.FogUnexplored, resumed.fog.fog.At(39, 0), "ground never seen")

	resumed.fog.fog.Update(w, h, []d2world.Eye{{ID: fogEyeID, X: 10.5, Y: 10.5}})
	require.Equal(t, d2world.FogVisible, resumed.fog.fog.At(10, 10), "the first update sees from where he stands")

	saved.fog.fog.Update(w, h, []d2world.Eye{{ID: fogEyeID, X: 12.5, Y: 10.5}})
	resumed.fog.fog.Update(w, h, []d2world.Eye{{ID: fogEyeID, X: 12.5, Y: 10.5}})
	require.Equal(t, saved.fog.fog.Rows(), resumed.fog.fog.Rows(), "and both run on as one fog")
}

// TestAGameWithFogOffSavesAnEmptyGrid: the shipped game without -fog never
// looks, so it saves the empty block (no map, no grid) -- and a grid loaded
// into a game with fog off is kept, undrawn, and saved again (fog's view is
// -fog, as the zoom's is ui.zoom; the grid is the world's).
func TestAGameWithFogOffSavesAnEmptyGrid(t *testing.T) {
	off, save := b4Game(t)
	require.False(t, off.fog.wanted, "the fixture's fog is off, as the shipped game's is")

	file, _ := b4File(t, off, save, "off.world.json")
	require.Equal(t, d2world.FogSnapshot{}, file.Fog, "a game that never looked keeps nothing")

	on, onSave := b4Game(t)
	on.fog = newGameFog(mapTileSight{on}, true)
	on.fog.fog.Update(40, 40, []d2world.Eye{{ID: fogEyeID, X: 20.5, Y: 20.5}})
	kept, _ := b4File(t, on, onSave, "on.world.json")

	resumed, resumedSave := b4Game(t) // fog off
	require.NoError(t, b4Load(t, resumed, kept))
	require.Equal(t, kept.Fog, resumed.fog.fog.Snapshot(fogMapID()), "the grid is kept with fog off")
	require.Equal(t, fogOffFlag, resumed.fogOffReason(), "and not drawn")

	again, _ := b4File(t, resumed, resumedSave, "again.world.json")
	require.Equal(t, kept.Fog, again.Fog, "and saved again as it was")
}
