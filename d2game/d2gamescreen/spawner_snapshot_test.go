package d2gamescreen

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// M4.6 B2b: the game spawner's arrival count, which decides where the next
// pack comes from. There is no engine here, so its advance -- one arrival per
// placed pack -- is followed through packSpots, which is exactly what the
// count is for: the anchors of the next arrivals.

// b2bNextAnchors is where the next n packs would stand, around one point.
func b2bNextAnchors(g *gameSpawner, n int) [][2]float64 {
	out := make([][2]float64, 0, n)

	for k := 1; k <= n; k++ {
		out = append(out, packSpots(40, 40, 8, 16, 3, g.arrival+k)[0])
	}

	return out
}

func b2bThroughJSON(t *testing.T, s SpawnerSnapshot) SpawnerSnapshot {
	t.Helper()

	b, err := json.Marshal(s)
	require.NoError(t, err)

	var back SpawnerSnapshot
	require.NoError(t, json.Unmarshal(b, &back))

	return back
}

// Round trip; the next packs come from the same places; a lost count sends
// them from elsewhere; a negative count is refused.
func TestGameSpawnerSnapshot(t *testing.T) {
	saved := &gameSpawner{arrival: 5}
	snap := b2bThroughJSON(t, saved.Snapshot())

	resumed := &gameSpawner{}
	require.NoError(t, resumed.Restore(snap))
	require.Equal(t, saved.Snapshot(), resumed.Snapshot())

	// The scene provider reports it, so the save's field is observable.
	require.Equal(t, 5, sceneProvider{&Game{spawner: resumed}}.HarnessState()["spawner_arrival"])

	require.Equal(t, b2bNextAnchors(saved, 6), b2bNextAnchors(resumed, 6), "the next six packs come from the same places")

	lost := &gameSpawner{}
	require.NoError(t, lost.Restore(SpawnerSnapshot{}))
	require.NotEqual(t, b2bNextAnchors(saved, 1), b2bNextAnchors(lost, 1), "lost, the next pack comes from elsewhere")

	require.Error(t, resumed.Restore(SpawnerSnapshot{Arrival: -1}))
	require.Equal(t, 5, resumed.arrival, "a refused restore changes nothing")
}
