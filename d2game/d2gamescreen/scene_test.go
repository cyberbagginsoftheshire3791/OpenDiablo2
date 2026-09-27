package d2gamescreen

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
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

	assert.Equal(t, map[string]interface{}{"present": false}, state["world_rng"],
		"no client, no world stream -- reported as absent, not as a stream at draw 0 of seed 0")

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

	assert.Equal(t, map[string]interface{}{"present": true, "seed": int64(1462), "seed_str": "1462", "draws": uint64(3)},
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

// --- M4.6 B1 review, C10: the providers go with the screen ---------------------

var errStop = errors.New("stop here")

// unbindFails is a terminal whose Unbind errors, so OnUnload returns at its
// first unbind -- which is how the test sees what OnUnload let go of BEFORE
// anything in it could fail.
type unbindFails struct{ d2interface.Terminal }

func (unbindFails) Unbind(...string) error { return errStop }

// binds is an input manager whose BindHandler answers err.
type binds struct {
	d2interface.InputManager
	err error
}

func (b binds) BindHandler(d2interface.InputEventHandler) error { return b.err }

// registered reports whether this exact provider value is registered. By
// identity, not by name: another test's "scene" must not answer for this one.
func registered(p d2harness.Provider) bool {
	for _, q := range d2harness.Providers() {
		if q == p {
			return true
		}
	}

	return false
}

// screenWithAWorld is a game screen holding the providers CreateGame registers
// itself (corpses, rising, scene) and one a world system registers in its
// constructor (the clock), all registered.
func screenWithAWorld(t *testing.T) (*Game, []d2harness.Provider) {
	t.Helper()

	clock := d2world.NewClock(d2world.DefaultClockDials())
	corpses := d2world.NewCorpses(nil, nil)
	rising := d2world.NewRising(corpses, func() int { return -1 }, clock.Stage, 1462, d2world.DefaultRisingDials())

	v := &Game{worldClock: clock, corpses: corpses, rising: rising}

	d2harness.Register(corpses)
	d2harness.Register(rising)
	d2harness.Register(sceneProvider{v})

	t.Cleanup(v.releaseWorld)

	providers := []d2harness.Provider{sceneProvider{v}, corpses, rising, clock}
	for _, p := range providers {
		require.True(t, registered(p), "%s must start registered", p.HarnessName())
	}

	return v, providers
}

// OnUnload lets go of every provider the screen registered, and does it before
// the first thing that can fail and return early. Before this test, deleting
// the scene's Unregister stayed green.
func TestOnUnloadUnregistersTheScreensProviders(t *testing.T) {
	v, providers := screenWithAWorld(t)
	v.terminal = unbindFails{}

	require.ErrorIs(t, v.OnUnload(), errStop, "OnUnload stops at its first unbind")

	for _, p := range providers {
		assert.False(t, registered(p), "%q outlived its screen", p.HarnessName())
	}
}

// CreateGame's one failure after the world is built releases the world, and
// its success keeps it.
func TestCreateGameFailureReleasesItsProviders(t *testing.T) {
	v, providers := screenWithAWorld(t)

	game, err := v.bindOrRelease(binds{err: errStop})
	require.Error(t, err)
	require.Nil(t, game)

	for _, p := range providers {
		assert.False(t, registered(p), "a failed CreateGame left %q registered", p.HarnessName())
	}

	w, kept := screenWithAWorld(t)

	game, err = w.bindOrRelease(binds{})
	require.NoError(t, err)
	require.Same(t, w, game)

	for _, p := range kept {
		assert.True(t, registered(p), "a successful CreateGame keeps %q", p.HarnessName())
	}
}
