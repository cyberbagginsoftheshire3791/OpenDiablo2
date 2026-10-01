package d2world

import (
	"fmt"
)

// PursuitSnapshot is the chases' state as the world save carries it (M4.6
// B2b, build plan §1).
//
// SAVED: every chase -- who hunts, what it hunts, and what its last solve left
// behind -- the total solves, and how many of them restarted a chase on
// another quarry (rechase_solves, version 3: the raid R2 review's B2).
//
// NOT SAVED: the dials (derived, like every system's) and the router (the
// game's wiring). Nor the route itself: the hunter is WALKING it, so it lives
// in the entity's motion (d2mapentity.Motion), which the load restores before
// this. A restored chase solves nothing until its own saved clock says so.
type PursuitSnapshot struct {
	Chases        []ChaseSnapshot `json:"chases"`
	Solves        int             `json:"solves"`
	RechaseSolves int             `json:"rechase_solves"`
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
		Chases:        make([]ChaseSnapshot, 0, len(p.chases)),
		Solves:        p.solves,
		RechaseSolves: p.rechases,
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
//
// It restores only into a pursuit that runs NO CHASE (the B2b review's B4, as
// B2a's corpses and squads refuse a used target): a live chase replaced would
// leave a hunter walking a route no saved chase owns.
func (p *Pursuit) Restore(snap PursuitSnapshot, r Resolver) error {
	chases, err := p.b2bValidate(snap, r)
	if err != nil {
		return err
	}

	p.chases = chases
	p.solves = snap.Solves
	p.rechases = snap.RechaseSolves

	return nil
}

// Validate is Restore's check and nothing else (D4): every refusal Restore
// would make, through the same Resolver, without changing the pursuit.
func (p *Pursuit) Validate(snap PursuitSnapshot, r Resolver) error {
	_, err := p.b2bValidate(snap, r)

	return err
}

// CheckSnapshot is every check Validate makes of the snapshot itself --
// everything but its refusal of a pursuit already in use. Game.SaveWorld runs it on
// the snapshot it has just taken from this live pursuit, so a save never writes a
// block the load's own Validate would refuse (the M4.6 B3 review, B2: "strict
// at save" stopped at the file's own checks). Validate is the in-use refusal
// and this, so the two cannot disagree.
func (p *Pursuit) CheckSnapshot(snap PursuitSnapshot, r Resolver) error {
	_, err := p.b2bBuild(snap, r)

	return err
}

// b2bValidate checks the whole snapshot and returns the chases it would
// restore. Nothing in p is changed.
func (p *Pursuit) b2bValidate(snap PursuitSnapshot, r Resolver) (map[string]*chase, error) {
	if len(p.chases) != 0 {
		return nil, fmt.Errorf("d2world: pursuit: restore into a pursuit that runs no chase; this one runs %d", len(p.chases))
	}

	return p.b2bBuild(snap, r)
}

// b2bBuild is b2bValidate's checks of the snapshot itself, and the chases it
// would restore: nothing of the pursuit's own state is read or changed.
func (p *Pursuit) b2bBuild(snap PursuitSnapshot, r Resolver) (map[string]*chase, error) {
	if err := b2bCheckNumbers("pursuit", true, b2bNum{"solves", float64(snap.Solves)},
		b2bNum{"rechase_solves", float64(snap.RechaseSolves)}); err != nil {
		return nil, err
	}

	if snap.RechaseSolves > snap.Solves {
		return nil, fmt.Errorf("d2world: pursuit: %d rechase solves of %d solves: each is one of them", snap.RechaseSolves, snap.Solves)
	}

	chases := make(map[string]*chase, len(snap.Chases))

	for i := range snap.Chases {
		cs := snap.Chases[i]

		if _, dup := chases[cs.Hunter]; dup {
			return nil, fmt.Errorf("d2world: pursuit: hunter %s is saved twice", cs.Hunter)
		}

		what := "pursuit chase " + cs.Hunter

		if err := b2bCheckNumbers(what, false, b2bNum{"solved_at_x", cs.SolvedAtX}, b2bNum{"solved_at_y", cs.SolvedAtY}); err != nil {
			return nil, err
		}

		if err := b2bCheckNumbers(what, true, b2bNum{"solved_distance", cs.SolvedDistance},
			b2bNum{"since_solve_minutes", cs.SinceSolve}, b2bNum{"solves", float64(cs.Solves)}); err != nil {
			return nil, err
		}

		hunter, err := b2bResolveHunter(r, cs.Hunter)
		if err != nil {
			return nil, err
		}

		quarry, err := b2bResolveQuarry(r, cs.Quarry)
		if err != nil {
			return nil, fmt.Errorf("pursuit: hunter %s: %w", cs.Hunter, err)
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

	return chases, nil
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
