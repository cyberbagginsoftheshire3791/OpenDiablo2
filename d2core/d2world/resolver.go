package d2world

import (
	"errors"
	"fmt"
	"math"
)

// PlayerRef is the word a snapshot writes wherever a saved record points at
// the player (M4.6 B2b).
//
// The player's entity id is his CONNECTION's uuid, and it is new on every
// launch (build plan §2). A save that wrote it would name a man who is not in
// the resumed game, and every watch and chase that pointed at him would fail
// to resolve. Every other entity is rebuilt with its saved id (the
// d2mapentity next-id seam), so he alone needs a word.
const PlayerRef = "player"

// Resolver turns the ids a snapshot saved back into the live things they name
// in the resumed game (M4.6 B2b).
//
// d2world does not know what an entity is, so it cannot find one; whoever
// rebuilt the entities at load (the game screen) can. It is the Spawner seam
// run backwards: the Spawner hands d2world things that can say where they
// are, and the Resolver hands the same things back by id.
//
// THERE IS NO REMAP TABLE. Each entity is rebuilt with its saved id, so a
// Resolver is a lookup, not a translation. And every Restore checks that what
// comes back answers to the id it was asked for, so a Resolver that hands back
// the wrong entity is an error -- not a pack hunting a man who is not there.
type Resolver interface {
	// Watcher is the live watcher with this entity id: a spawned member, a
	// risen man, anything Notice watches or Pursuit chases with. A chase's
	// hunter resolves here too and must also be a Hunter; the game's chaser
	// adapter is both, which is the seam startChasesForTheAware already uses.
	Watcher(id string) (Watcher, bool)

	// Quarry is the live thing a saved record was watching or chasing:
	// PlayerRef is the player, anything else is an entity id.
	Quarry(ref string) (Quarry, bool)
}

// ErrUnresolvedRef is what a Restore returns when a saved id names nothing in
// the resumed game, or names the wrong thing. It is an error, never a silent
// drop: a watch dropped on load is a wolf that forgets him, and a chase bound
// to the wrong entity is a pack hunting a man who is not there.
var ErrUnresolvedRef = errors.New("d2world: a saved id does not resolve in the resumed world")

// b2bQuarryRef is how a snapshot writes a quarry: the player as PlayerRef,
// anything else by its own id.
func b2bQuarryRef(q Quarry, playerID string) string {
	id := q.QuarryID()
	if playerID != "" && id == playerID {
		return PlayerRef
	}

	return id
}

// b2bResolveWatcher resolves one saved watcher id, strictly.
func b2bResolveWatcher(r Resolver, id string) (Watcher, error) {
	switch {
	case r == nil:
		return nil, fmt.Errorf("%w: watcher %q: no resolver", ErrUnresolvedRef, id)
	case id == "" || id == PlayerRef:
		return nil, fmt.Errorf("%w: %q is not an entity id", ErrUnresolvedRef, id)
	}

	w, ok := r.Watcher(id)
	if !ok || w == nil {
		return nil, fmt.Errorf("%w: watcher %q is not in it", ErrUnresolvedRef, id)
	}

	if got := w.WatcherID(); got != id {
		return nil, fmt.Errorf("%w: watcher %q came back as %q", ErrUnresolvedRef, id, got)
	}

	return w, nil
}

// b2bResolveQuarry resolves one saved quarry ref, strictly.
func b2bResolveQuarry(r Resolver, ref string) (Quarry, error) {
	switch {
	case r == nil:
		return nil, fmt.Errorf("%w: quarry %q: no resolver", ErrUnresolvedRef, ref)
	case ref == "":
		return nil, fmt.Errorf("%w: an empty quarry ref", ErrUnresolvedRef)
	}

	q, ok := r.Quarry(ref)
	if !ok || q == nil {
		return nil, fmt.Errorf("%w: quarry %q is not in it", ErrUnresolvedRef, ref)
	}

	// The player's id is new every launch, so only an entity can be held to
	// the id it was saved under.
	if ref != PlayerRef && q.QuarryID() != ref {
		return nil, fmt.Errorf("%w: quarry %q came back as %q", ErrUnresolvedRef, ref, q.QuarryID())
	}

	return q, nil
}

// b2bNum is one saved number and its name, for b2bCheckNumbers.
type b2bNum struct {
	name  string
	value float64
}

// b2bCheckNumbers refuses a saved number that no running game could have
// produced: NaN or an infinity anywhere, and a negative where the value is a
// count, a distance or a span of minutes. A restore that accepted one would
// resume a world that never existed. The first bad number, in the order
// given, is the one reported.
func b2bCheckNumbers(what string, nonNegative bool, nums ...b2bNum) error {
	for _, n := range nums {
		if math.IsNaN(n.value) || math.IsInf(n.value, 0) {
			return fmt.Errorf("d2world: %s: %s is %v", what, n.name, n.value)
		}

		if nonNegative && n.value < 0 {
			return fmt.Errorf("d2world: %s: %s is negative (%v)", what, n.name, n.value)
		}
	}

	return nil
}
