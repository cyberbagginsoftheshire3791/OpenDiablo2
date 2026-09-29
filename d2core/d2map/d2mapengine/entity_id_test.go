package d2mapengine

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
)

// b2bOnMap is an entity standing on the map under an id.
type b2bOnMap struct{ id string }

func (e b2bOnMap) ID() string                   { return e.id }
func (b2bOnMap) Render(d2interface.Surface)     {}
func (b2bOnMap) Advance(float64)                {}
func (b2bOnMap) GetPosition() d2vector.Position { return d2vector.NewPosition(0, 0) }
func (b2bOnMap) GetVelocity() d2vector.Vector   { return *d2vector.NewVector(0, 0) }
func (b2bOnMap) GetSize() (width, height int)   { return 1, 1 }
func (b2bOnMap) GetLayer() int                  { return 0 }
func (b2bOnMap) GetPositionF() (x, y float64)   { return 0, 0 }
func (b2bOnMap) Label() string                  { return "" }
func (b2bOnMap) Selectable() bool               { return false }
func (b2bOnMap) Highlight()                     {}

// M4.6 B2b: through the engine, the next-id seam also refuses an id already on
// the map -- AddEntity keys on the id, and a second entity under it would
// silently replace the first in the engine and in every registry.
func TestEngineRefusesAnIDAlreadyOnTheMap(t *testing.T) {
	m := &MapEngine{
		entities:         map[string]d2interface.MapEntity{},
		MapEntityFactory: &d2mapentity.MapEntityFactory{},
	}

	m.AddEntity(b2bOnMap{"e:villager"})

	err := m.SetNextEntityID("e:villager")
	require.True(t, errors.Is(err, d2mapentity.ErrEntityID), "%v", err)

	_, waiting := m.PendingEntityID()
	require.False(t, waiting, "the refused id was not set")

	require.NoError(t, m.SetNextEntityID("e:wolf"), "an id not on the map passes to the factory")

	id, waiting := m.PendingEntityID()
	require.True(t, waiting)
	require.Equal(t, "e:wolf", id)

	// The factory's own refusals still apply through the engine.
	require.True(t, errors.Is(m.SetNextEntityID("e:dog"), d2mapentity.ErrEntityID), "one still waiting")

	bare := &MapEngine{entities: map[string]d2interface.MapEntity{}}
	require.True(t, errors.Is(bare.SetNextEntityID("e:1"), d2mapentity.ErrEntityID), "no factory, no seam")
}

// M4.6 B4b: RekeyEntity, the load's re-key verb. The entity on the map keeps
// being the same pointer, under the new id; the refusals change nothing.
func TestRekeyEntityMovesTheMapsKey(t *testing.T) {
	m := &MapEngine{
		entities:         map[string]d2interface.MapEntity{},
		MapEntityFactory: &d2mapentity.MapEntityFactory{},
	}

	villager := &d2mapentity.NPC{}
	require.Empty(t, villager.ID())

	// An NPC with an id: re-keyed from "" is refused as not on the map.
	require.True(t, errors.Is(m.RekeyEntity(villager, "0saved"), d2mapentity.ErrEntityID), "not on the map")

	require.NoError(t, m.MapEntityFactory.Rekey(villager, "0built"))
	m.AddEntity(villager)
	m.AddEntity(b2bOnMap{"0wolf"})

	for _, id := range []string{"0wolf", "", "player"} {
		require.True(t, errors.Is(m.RekeyEntity(villager, id), d2mapentity.ErrEntityID), "%q", id)
		require.Equal(t, "0built", villager.ID(), "%q: nothing changed", id)
		require.Same(t, villager, m.entities["0built"])
	}

	require.True(t, errors.Is(m.RekeyEntity(b2bOnMap{"0wolf"}, "0x"), d2mapentity.ErrEntityID),
		"a kind a save does not carry is not re-keyed")
	require.True(t, errors.Is(m.RekeyEntity(nil, "0x"), d2mapentity.ErrEntityID))

	require.NoError(t, m.RekeyEntity(villager, "0saved"))
	require.Equal(t, "0saved", villager.ID())
	require.Same(t, villager, m.entities["0saved"], "the map is keyed by the new id")

	_, old := m.entities["0built"]
	require.False(t, old, "and not by the old")

	require.True(t, errors.Is(m.SetNextEntityID("0saved"), d2mapentity.ErrEntityID), "the seam refuses it: on the map")

	bare := &MapEngine{entities: map[string]d2interface.MapEntity{"0saved": villager}}
	require.True(t, errors.Is(bare.RekeyEntity(villager, "0y"), d2mapentity.ErrEntityID), "no factory, no re-key")
}
