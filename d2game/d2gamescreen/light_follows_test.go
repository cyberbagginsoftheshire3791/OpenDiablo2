package d2gamescreen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// BUG-110, THE SIM'S HALF: during a held turn his torch's light is where he
// stands, for the model the sim reads -- not where the turn opened. A unit
// game at night, his torch lit; the world held (his journal open stops it
// exactly as an open turn does: worldRunning is false); he walks two tiles
// east, his Move; the frame's map step runs. The tile four ahead of him is
// six from where the turn opened -- outside the torch's five -- and four from
// where he stands. Before the fix the light model read 0.125 there, the
// night's floor, and the resolver's lit/dark rule read it so.
func TestHisTorchFollowsHimThroughAHeldTurnForTheSim(t *testing.T) {
	v, _ := b4Game(t)

	for i := 0; i < 300; i++ { // 300 s at the day rate: past true dark
		v.advanceWorld(1)
	}

	require.Equal(t, d2world.StageNight, v.worldClock.Stage(), "the fixture runs into the night")

	src := v.light.Add(d2world.SourceTorch, true, 0, 0)
	src.Burn = 120

	// He stands at tile (20,20) and the world runs: the model knows him there.
	v.localPlayer.StandAt(100, 100, 0)
	v.advanceWorld(1.0 / 60)

	sky := v.light.Level(60, 60) // nowhere near a light: the night as drawn
	ahead := func() float64 { return v.light.Level(26, 20) }

	require.Greater(t, v.light.Level(20, 20), sky, "lit where he stands")
	require.Equal(t, sky, ahead(), "the control: (26,20) is six tiles from his torch, outside its five")

	// The turn opens: the world is held.
	v.journalOpen = true
	require.False(t, v.worldRunning(), "the world is held")

	// His Move: two tiles east, a real walk while the world is stopped. The
	// frame's MAP STEP moves him (stepTheMap; a unit game has no map he walks
	// on, so a walk of a fifth of a tile a frame stands in for the engine's),
	// and after every frame the light model holds where that step left him
	// (the review's R4: told before the map step, it would hold the frame
	// before's).
	x := 100.0
	v.mapStepForTest = func(float64) {
		if x < 110 {
			x++
			v.localPlayer.StandAt(x, 100, 0)
		}
	}

	moved := 0

	for i := 0; i < 12; i++ {
		before := v.localPlayer.Position.World()

		v.advanceTheMap(1.0 / 60)

		at := v.localPlayer.Position.World()
		if at.X() != before.X() || at.Y() != before.Y() {
			moved++
		}

		lx, ly := carriedAt(t, v)
		require.Equal(t, at.X(), lx, "frame %d: the light model holds his x after this frame's map step", i)
		require.Equal(t, at.Y(), ly, "frame %d: the light model holds his y after this frame's map step", i)
	}

	require.Equal(t, 10, moved, "the map step walked him, frame by frame (the control that this is a walk, not a stand)")
	require.InDelta(t, 22.0, v.localPlayer.Position.World().X(), 0.01, "he walked to tile (22,20)")

	require.Greater(t, ahead(), sky,
		"(26,20) is four tiles from where he stands: his torch lights it for the sim in a held turn (BUG-110)")
	require.Greater(t, v.light.Level(22, 20), sky, "and lit where he stands now")

	// The world runs again: nothing new -- advanceWorld finds him where the
	// map left him.
	v.journalOpen = false

	v.advanceWorld(1.0 / 60)
	require.Greater(t, ahead(), v.light.Level(60, 60), "a running world reads him where the held frames left him")
}

// carriedAt is where the light model's carried source shines from: the
// player position Light.SetPlayer last gave it (the light provider's
// source_list x/y).
func carriedAt(t *testing.T, v *Game) (x, y float64) {
	t.Helper()

	for _, src := range v.light.HarnessState()["source_list"].([]map[string]interface{}) {
		if src["carried"] == true {
			return src["x"].(float64), src["y"].(float64)
		}
	}

	t.Fatal("no carried source")

	return 0, 0
}
