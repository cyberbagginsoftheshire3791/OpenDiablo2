package masterref

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
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

// RekeyEntity is the load's re-key verb (M4.6 B4b; the factory's Rekey says
// why it exists): an entity ON THIS MAP -- a villager the map built -- given
// another id in place, and the engine's own map keyed by the new id. The
// entity is the same pointer before and after, so everything that holds it
// (the game screen's natives, a talk, the headman's post) still holds it.
//
// Refused with d2mapentity.ErrEntityID, changing nothing: an entity that is
// not on this map under the id it answers to, an id another entity on the map
// holds, and everything the factory's Rekey refuses.
func (m *MapEngine) RekeyEntity(e d2interface.MapEntity, id string) error {
	if m.MapEntityFactory == nil {
		return fmt.Errorf("%w: the map has no entity factory", d2mapentity.ErrEntityID)
	}

	if e == nil {
		return fmt.Errorf("%w: no entity to re-key", d2mapentity.ErrEntityID)
	}

	old := e.ID()
	if on, ok := m.entities[old]; !ok || on != e {
		return fmt.Errorf("%w: %q is not on this map", d2mapentity.ErrEntityID, old)
	}

	if other, on := m.entities[id]; on && other != e {
		return fmt.Errorf("%w: %q is already on the map as another entity", d2mapentity.ErrEntityID, id)
	}

	if err := m.MapEntityFactory.Rekey(e, id); err != nil {
		return err
	}

	delete(m.entities, old)
	m.entities[id] = e

	return nil
}
