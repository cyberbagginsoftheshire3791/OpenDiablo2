package d2mapentity

import (
	"errors"
	"fmt"
)

// ErrEntityID is what SetNextEntityID returns when it refuses an id.
var ErrEntityID = errors.New("d2mapentity: entity id refused")

// SetNextEntityID hands the next entity the factory builds -- the next NewNPC
// or NewCreature -- a chosen id, once (M4.6 B2b, build plan §2).
//
// It is how a load rebuilds an entity with the id it was saved under. Seven
// registries key on entity ids (squad models, pack members, notice watches,
// chases, the corpse maps, Game.bodies and the engine's own entity map), so
// an entity that comes back with its old id puts all seven back at once,
// with no remap table for a bug to hide in. It follows SetNextGameSeed's
// pattern: set, consumed by the next construction, then generation resumes
// from the uuid stream as before.
//
// A DUPLICATE IS REFUSED WITH AN ERROR, not a panic: a save carrying one id
// twice is a bad file, and a bad file is set aside at load (rule 7) rather
// than taking the game down. Refused:
//   - an empty id, or the word "player" (the player's id belongs to his
//     connection and is new every launch; a save writes the word instead);
//   - an id while another is still waiting: the entity it was set for was
//     never built, and the next one would take the wrong id;
//   - an id this factory has already handed out through the seam.
//
// The factory does not know which entities are on the map; MapEngine's own
// SetNextEntityID (d2mapengine/entity_id.go) refuses an id already there too,
// and is the one a load should call.
//
// Only NewNPC and NewCreature take the id -- the two kinds a save rebuilds
// (spawned, risen, deployed and placed monsters). NewPlayer has its own id,
// and missiles, items, overlays and objects are not saved.
func (f *MapEntityFactory) SetNextEntityID(id string) error {
	switch {
	case id == "" || id == "player":
		return fmt.Errorf("%w: %q is not an entity id", ErrEntityID, id)
	case f.nextEntityID != "":
		return fmt.Errorf("%w: %q is still waiting for its entity, so %q cannot be set", ErrEntityID, f.nextEntityID, id)
	case f.givenIDs[id]:
		return fmt.Errorf("%w: %q has already been given to an entity", ErrEntityID, id)
	}

	f.nextEntityID = id

	return nil
}

// PendingEntityID is the id waiting for the next entity, if any. A load that
// has rebuilt everything should find nothing waiting.
func (f *MapEntityFactory) PendingEntityID() (string, bool) {
	return f.nextEntityID, f.nextEntityID != ""
}

// takeEntityID consumes the waiting id: "" when none is set, which means a
// fresh id from the uuid stream. It is consumed by every construction that
// takes it, whether or not the construction then succeeds, so a failed
// rebuild can never hand its id to whatever is built next.
func (f *MapEntityFactory) takeEntityID() string {
	id := f.nextEntityID
	f.nextEntityID = ""

	if id != "" {
		if f.givenIDs == nil {
			f.givenIDs = make(map[string]bool)
		}

		f.givenIDs[id] = true
	}

	return id
}
