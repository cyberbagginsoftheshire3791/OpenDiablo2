package d2mapengine

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
)

// SetNextEntityID is the entity factory's next-id seam (M4.6 B2b; see
// d2mapentity.MapEntityFactory.SetNextEntityID) with the one check the
// factory cannot make: an id already on THIS map is refused. It shadows the
// factory's method, so a load that goes through the engine -- which every
// game-side rebuild does -- gets both checks.
//
// Two entities with one id cannot share the engine's map (AddEntity keys on
// the id and would silently overwrite the first), and every registry that
// keys on ids would then be pointing at whichever came last.
func (m *MapEngine) SetNextEntityID(id string) error {
	if m.MapEntityFactory == nil {
		return fmt.Errorf("%w: the map has no entity factory", d2mapentity.ErrEntityID)
	}

	if _, on := m.entities[id]; on {
		return fmt.Errorf("%w: %q is already on the map", d2mapentity.ErrEntityID, id)
	}

	return m.MapEntityFactory.SetNextEntityID(id)
}
