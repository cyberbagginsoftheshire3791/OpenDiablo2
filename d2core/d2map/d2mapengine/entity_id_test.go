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
