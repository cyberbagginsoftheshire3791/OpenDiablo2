package d2world

import (
	"fmt"
)

// PursuitSnapshot is the chases' state as the world save carries it (M4.6
// B2b, build plan §1).
//
// SAVED: every chase -- who hunts, what it hunts, and what its last solve left
// behind -- and the total solves.
//
// NOT SAVED: the dials (derived, like every system's) and the router (the
// game's wiring). Nor the route itself: the hunter is WALKING it, so it lives
// in the entity's motion (d2mapentity.Motion), which the load restores before
// this. A restored chase solves nothing until its own saved clock says so.
type PursuitSnapshot struct {
	Chases []ChaseSnapshot `json:"chases"`
	Solves int             `json:"solves"`
}

// ChaseSnapshot is one hunter's pursuit of one quarry. The hunter is written
// as its entity id; the quarry as PlayerRef when it is the player, and as its
// entity id otherwise.
type ChaseSnapshot struct {
	Hunter string `json:"hunter"`
	Quarry string `json:"quarry"`

	SolvedAtX      float64 `json:"solved_at_x"`
	SolvedAtY      float64 `json:"solved_at_y"`
	SolvedDistance float64 `json:"solved_distance"`
	SinceSolve     float64 `json:"since_solve_minutes"`

	Reachable bool `json:"reachable"`
	Solves    int  `json:"solves"`
	Arrived   bool `json:"arrived"`
}

// Snapshot is the chases' state now, in the stable hunter order. r is a
// Resolver over the LIVE world: it names the player, and every hunter and
// quarry must resolve through it now, or the load could not resolve them.
func (p *Pursuit) Snapshot(r Resolver) (PursuitSnapshot, error) {
	snap := PursuitSnapshot{
		Chases: make([]ChaseSnapshot, 0, len(p.chases)),
		Solves: p.solves,
	}

	if len(p.chases) == 0 {
		return snap, nil
	}

	playerID, err := b2bLivePlayerID(r, "pursuit")
	if err != nil {
		return PursuitSnapshot{}, err
	}

	for _, id := range p.hunterIDs() {
		c := p.chases[id]

		cs := ChaseSnapshot{
			Hunter:         id,
			Quarry:         b2bQuarryRef(c.quarry, playerID),
			SolvedAtX:      c.solvedAtX,
			SolvedAtY:      c.solvedAtY,
			SolvedDistance: c.solvedDistance,
			SinceSolve:     c.sinceSolve,
			Reachable:      c.reachable,
			Solves:         c.solves,
			Arrived:        c.arrived,
		}

		if _, err := b2bResolveHunter(r, cs.Hunter); err != nil {
			return PursuitSnapshot{}, err
		}

		if _, err := b2bResolveQuarry(r, cs.Quarry); err != nil {
			return PursuitSnapshot{}, fmt.Errorf("pursuit: hunter %s: %w", cs.Hunter, err)
		}

		snap.Chases = append(snap.Chases, cs)
	}

	return snap, nil
}

// Restore puts the chases where a snapshot left them, resolving every hunter
// and quarry through r. A chase that does not resolve is an ERROR, not a
// silent drop. It is all-or-nothing, and it solves no route and hands no
// hunter a path: the walk is the entity's, restored with its motion.
func (p *Pursuit) Restore(snap PursuitSnapshot, r Resolver) error {
	if err := b2bCheckNumbers("pursuit", true, b2bNum{"solves", float64(snap.Solves)}); err != nil {
		return err
	}

	chases := make(map[string]*chase, len(snap.Chases))

	for i := range snap.Chases {
		cs := snap.Chases[i]

		if _, dup := chases[cs.Hunter]; dup {
			return fmt.Errorf("d2world: pursuit: hunter %s is saved twice", cs.Hunter)
		}

		what := "pursuit chase " + cs.Hunter

		if err := b2bCheckNumbers(what, false, b2bNum{"solved_at_x", cs.SolvedAtX}, b2bNum{"solved_at_y", cs.SolvedAtY}); err != nil {
			return err
		}

		if err := b2bCheckNumbers(what, true, b2bNum{"solved_distance", cs.SolvedDistance},
			b2bNum{"since_solve_minutes", cs.SinceSolve}, b2bNum{"solves", float64(cs.Solves)}); err != nil {
			return err
		}

		hunter, err := b2bResolveHunter(r, cs.Hunter)
		if err != nil {
			return err
		}

		quarry, err := b2bResolveQuarry(r, cs.Quarry)
		if err != nil {
			return fmt.Errorf("pursuit: hunter %s: %w", cs.Hunter, err)
		}

		chases[cs.Hunter] = &chase{
			hunter:         hunter,
			quarry:         quarry,
			solvedAtX:      cs.SolvedAtX,
			solvedAtY:      cs.SolvedAtY,
			solvedDistance: cs.SolvedDistance,
			sinceSolve:     cs.SinceSolve,
			reachable:      cs.Reachable,
			solves:         cs.Solves,
			arrived:        cs.Arrived,
		}
	}

	p.chases = chases
	p.solves = snap.Solves

	return nil
}

// b2bResolveHunter resolves a hunter: a watcher by id that can also walk --
// the same type assertion startChasesForTheAware makes on the live path.
func b2bResolveHunter(r Resolver, id string) (Hunter, error) {
	w, err := b2bResolveWatcher(r, id)
	if err != nil {
		return nil, fmt.Errorf("pursuit: %w", err)
	}

	h, ok := w.(Hunter)
	if !ok {
		return nil, fmt.Errorf("%w: pursuit: %s resolves to a watcher that cannot hunt (%T)", ErrUnresolvedRef, id, w)
	}

	if got := h.HunterID(); got != id {
		return nil, fmt.Errorf("%w: pursuit: hunter %s came back as %q", ErrUnresolvedRef, id, got)
	}

	return h, nil
}
