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

	// His Move: two tiles east, a real walk while the world is stopped.
	v.localPlayer.StandAt(110, 100, 0)

	for i := 0; i < 3; i++ {
		v.advanceTheMap(1.0 / 60)
	}

	require.Greater(t, ahead(), sky,
		"(26,20) is four tiles from where he stands: his torch lights it for the sim in a held turn (BUG-110)")
	require.Greater(t, v.light.Level(22, 20), sky, "and lit where he stands now")

	// The world runs again: nothing new -- advanceWorld finds him where the
	// map left him.
	v.journalOpen = false

	v.advanceWorld(1.0 / 60)
	require.Greater(t, ahead(), v.light.Level(60, 60), "a running world reads him where the held frames left him")
}
