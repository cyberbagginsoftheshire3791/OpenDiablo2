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
// EVERY CONSTRUCTOR THAT MAKES A MAP ENTITY CONSUMES IT (build plan §5: every
// constructor that calls newMapEntity), as its first act, so a set id is used
// by exactly one construction and can never linger for a later one:
//   - NewNPC and NewCreature WEAR it. They are the two kinds a save rebuilds
//     (spawned, risen, deployed and placed monsters).
//   - NewPlayer, NewMissile, NewItem and NewCastOverlay SPEND it and do not
//     wear it: the player's id is his connection's, and the others are not
//     saved. Wearing it would be worse than losing it -- the engine's map and
//     the notice model would then hold a monster's saved name on a missile.
//     So the monster the load meant is built with a fresh id instead, and the
//     load's own check (the rebuilt entity must answer to its saved id, and
//     every Resolver refuses one that does not) refuses the file.
//
// NewObject never took part: it does not call newMapEntity and makes its id
// itself, and objects are stamped by the map, never saved.
// TestEveryEntityConstructorTakesTheWaitingID pins the rule, so a constructor
// added later cannot slip past the seam.
func (f *MapEntityFactory) SetNextEntityID(id string) error {
	switch {
	case id == "" || id == "player": // d2world.PlayerRef, which this package does not import
		return fmt.Errorf("%w: %q is not an entity id", ErrEntityID, id)
	case f.nextEntityID != "":
		return fmt.Errorf("%w: %q is still waiting for its entity, so %q cannot be set", ErrEntityID, f.nextEntityID, id)
	case f.givenIDs[id]:
		return fmt.Errorf("%w: %q has already been used by a construction", ErrEntityID, id)
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
// fresh id from the uuid stream. It is consumed by every construction, first,
// whether or not the construction then succeeds, so a failed rebuild can
// never hand its id to whatever is built next. A consumed id counts as given,
// worn or not: a load that lost one cannot quietly set it again.
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

// spendEntityID is takeEntityID for the kinds that never wear a saved id (see
// SetNextEntityID): the waiting id is consumed and thrown away.
func (f *MapEntityFactory) spendEntityID() {
	_ = f.takeEntityID()
}
