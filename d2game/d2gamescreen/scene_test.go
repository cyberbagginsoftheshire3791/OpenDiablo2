package d2gamescreen

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
)

// M4.6 B1: the "scene" provider reports the screen's own bookkeeping, which
// the world save carries and nothing reported before. Each value below is one
// the test set, read back through the provider.
func TestSceneReportsTheScreensBookkeeping(t *testing.T) {
	v := &Game{
		// Laid in THIS order on purpose: the i-th laid carries the i-th
		// writing, so the provider must keep the order and never sort it.
		fieldDead:     []string{"dead:3", "dead:1", "dead:2"},
		dawnPaidDay:   2,
		lastStage:     d2world.StageDusk,
		watchClock:    812.5,
		watchClockSet: true,
		spawner:       &gameSpawner{arrival: 4},
		bodies: map[string]*npcBody{
			"wolf": {health: 40, maxHealth: 181},
			"dog":  newNPCBody(61),
		},
	}

	state := sceneProvider{v}.HarnessState()

	assert.Equal(t, []string{"dead:3", "dead:1", "dead:2"}, state["field_dead"], "laid order, not sorted")
	assert.Equal(t, 2, state["dawn_paid_day"])
	assert.Equal(t, d2world.StageDusk.String(), state["last_stage"])
	assert.Equal(t, 812.5, state["watch_clock"])
	assert.Equal(t, true, state["watch_clock_set"])
	assert.Equal(t, 4, state["spawner_arrival"])

	assert.Equal(t, []map[string]interface{}{
		{"id": "dog", "health": 61, "max_health": 61},
		{"id": "wolf", "health": 40, "max_health": 181},
	}, state["bodies"], "every monster's health, in id order")

	assert.Equal(t, map[string]interface{}{"seed": int64(0), "draws": uint64(0)}, state["world_rng"],
		"no client, no world stream -- reported as nothing drawn, not omitted")

	// A copy: a reader cannot reorder the dead through the provider.
	state["field_dead"].([]string)[0] = "intruder"
	assert.Equal(t, "dead:3", v.fieldDead[0])

	_, err := json.Marshal(state)
	require.NoError(t, err, "the digest JSON-encodes every provider")
}

// world_rng is the map engine's stream -- the digest's rng part, readable.
func TestSceneReportsTheWorldStream(t *testing.T) {
	engine := &d2mapengine.MapEngine{}
	engine.SetSeed(1462)

	for i := 0; i < 3; i++ {
		engine.Rand().Intn(10)
	}

	v := &Game{gameClient: &d2client.GameClient{MapEngine: engine}}

	assert.Equal(t, map[string]interface{}{"seed": int64(1462), "draws": uint64(3)},
		sceneProvider{v}.HarnessState()["world_rng"])
}

// OnUnload unregisters with a FRESH sceneProvider{v}; that only works because
// the provider is a comparable value keyed by the screen. If it ever grew a
// slice or a map, Unregister would silently miss it and a stale "scene" would
// shadow the next game's.
func TestSceneUnregistersByValue(t *testing.T) {
	v := &Game{}
	d2harness.Register(sceneProvider{v})

	p, ok := d2harness.Lookup("scene")
	require.True(t, ok)
	assert.Equal(t, "scene", p.HarnessName())

	d2harness.Unregister(sceneProvider{v})

	_, ok = d2harness.Lookup("scene")
	assert.False(t, ok, "a fresh value of the same screen unregisters it")
}
