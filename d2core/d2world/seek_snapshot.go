package d2world

import (
	"fmt"
)

// SeekSnapshot is Seek's state as the world save carries it: the world file's
// "seek" block (the raid's R2, an amendment of version 2's shape with no bump
// -- the milestone's rule, docs/m4.6-world-save-notes.md).
//
// SAVED: every row -- which watcher, the target it last found its watch on,
// why, the minutes until its next look (its stagger phase is in there) and how
// many living were in reach at its last look -- the stand-ins a script named,
// the stagger phases given out, and the three totals.
//
// NOT SAVED, and why:
//   - the dials (RetargetMinutes, StaggerSlots) are DIALS, never saved (trap 7).
//   - notice, spawns, combat, the game's living and the resolver are WIRING.
//
// NOTHING IN IT IS RESOLVED, on purpose. A row is Seek's bookkeeping about a
// watch, not the watch: the watch -- who watches whom -- is the notice block's,
// resolved there. A row whose watcher no longer watches is dropped at the next
// step (sync) in the resumed game exactly as in the saved one, so a save made
// on the frame a monster died, or was sent home, is taken and resumes alike.
// A stand-in that no longer resolves is simply not among the living at the
// next look, as in the saved game. Requiring either to resolve would refuse
// saves the game makes (the raid R1 review's B1, BUG-68, was that mistake in
// the combat block).
type SeekSnapshot struct {
	Rows     []SeekRowSnapshot `json:"rows"`
	StandIns []string          `json:"stand_ins"`

	// Slots is how many stagger phases have been given out: the next row
	// made takes phase Slots mod StaggerSlots.
	Slots int `json:"slots"`

	Looks     int `json:"looks"`
	Retargets int `json:"retargets"`
	Rays      int `json:"rays"`
}

// SeekRowSnapshot is one hostile watcher's row.
type SeekRowSnapshot struct {
	Watcher string `json:"watcher"`

	// Target is written as every quarry is: "player" for him, an entity id
	// otherwise. Reason is pending, living, none or fighting.
	Target string `json:"target"`
	Reason string `json:"reason"`

	// UntilLook is the world minutes until the row next looks, always above
	// zero between two steps: a row that came due looked and took its next
	// phase.
	UntilLook float64 `json:"until_look_minutes"`

	Candidates int `json:"candidates"`
}

// Snapshot is Seek's state now, rows in watcher order. It refuses nothing:
// Seek holds nothing a save may not carry.
func (s *Seek) Snapshot() SeekSnapshot {
	snap := SeekSnapshot{
		Rows:      make([]SeekRowSnapshot, 0, len(s.rows)),
		StandIns:  append([]string{}, s.standIns...),
		Slots:     s.slots,
		Looks:     s.looks,
		Retargets: s.retargets,
		Rays:      s.rays,
	}

	for _, id := range s.rowIDs() {
		r := s.rows[id]
		snap.Rows = append(snap.Rows, SeekRowSnapshot{
			Watcher:    id,
			Target:     r.target,
			Reason:     r.reason,
			UntilLook:  r.untilLook,
			Candidates: r.candidates,
		})
	}

	return snap
}

// Restore puts Seek where a snapshot left it. It is all-or-nothing, and it
// restores only into a Seek that holds no row and no stand-in (the B2
// convention: refuse a used target).
func (s *Seek) Restore(snap SeekSnapshot) error {
	rows, err := s.validate(snap)
	if err != nil {
		return err
	}

	s.rows = rows
	s.standIns = append([]string{}, snap.StandIns...)
	s.slots = snap.Slots
	s.looks, s.retargets, s.rays = snap.Looks, snap.Retargets, snap.Rays

	return nil
}

// Validate is Restore's check and nothing else (D4).
func (s *Seek) Validate(snap SeekSnapshot) error {
	_, err := s.validate(snap)

	return err
}

// CheckSnapshot is every check Validate makes of the snapshot itself, without
// its refusal of a Seek in use: the save runs it on the snapshot it has just
// taken from the live Seek (the M4.6 B3 review's B2, as the others').
func (s *Seek) CheckSnapshot(snap SeekSnapshot) error {
	_, err := seekRowsOf(snap)

	return err
}

func (s *Seek) validate(snap SeekSnapshot) (map[string]*seekRow, error) {
	if len(s.rows) != 0 || len(s.standIns) != 0 {
		return nil, fmt.Errorf("d2world: seek: restore into a Seek that holds nothing; this one holds %d row(s) and %d stand-in(s)",
			len(s.rows), len(s.standIns))
	}

	return seekRowsOf(snap)
}

// seekRowsOf checks the snapshot whole and builds the rows it would restore.
func seekRowsOf(snap SeekSnapshot) (map[string]*seekRow, error) {
	if err := b2bCheckNumbers("seek", true,
		b2bNum{"slots", float64(snap.Slots)}, b2bNum{"looks", float64(snap.Looks)},
		b2bNum{"retargets", float64(snap.Retargets)}, b2bNum{"rays", float64(snap.Rays)}); err != nil {
		return nil, err
	}

	if snap.Retargets > snap.Looks {
		return nil, fmt.Errorf("d2world: seek: %d retargets in %d looks: a watch is moved only by a look", snap.Retargets, snap.Looks)
	}

	if len(snap.Rows) > snap.Slots {
		return nil, fmt.Errorf("d2world: seek: %d rows and only %d stagger phases given out: every row took one", len(snap.Rows), snap.Slots)
	}

	stand := map[string]bool{}

	for _, id := range snap.StandIns {
		switch {
		case id == "" || id == PlayerRef:
			return nil, fmt.Errorf("d2world: seek: stand-in %q is not an entity id", id)
		case stand[id]:
			return nil, fmt.Errorf("d2world: seek: stand-in %q is named twice", id)
		}

		stand[id] = true
	}

	rows := make(map[string]*seekRow, len(snap.Rows))
	last := ""

	for i, r := range snap.Rows {
		switch {
		case r.Watcher == "" || r.Watcher == PlayerRef:
			return nil, fmt.Errorf("d2world: seek: row %d: watcher %q is not an entity id", i, r.Watcher)
		case i > 0 && r.Watcher <= last:
			return nil, fmt.Errorf("d2world: seek: row %s is out of watcher order (after %s), or saved twice", r.Watcher, last)
		case r.Target == "":
			return nil, fmt.Errorf("d2world: seek: row %s has no target", r.Watcher)
		case r.Target == r.Watcher:
			return nil, fmt.Errorf("d2world: seek: row %s names itself its target", r.Watcher)
		case !validSeekReason(r.Reason):
			return nil, fmt.Errorf("d2world: seek: row %s: reason %q is not pending, living, none or fighting", r.Watcher, r.Reason)
		}

		if err := b2bCheckNumbers("seek row "+r.Watcher, true,
			b2bNum{"until_look_minutes", r.UntilLook}, b2bNum{"candidates", float64(r.Candidates)}); err != nil {
			return nil, err
		}

		if !(r.UntilLook > 0) {
			return nil, fmt.Errorf("d2world: seek: row %s looks in %v minutes: between two steps every row's next look is still to come", r.Watcher, r.UntilLook)
		}

		if r.Reason == SeekPending && r.Candidates != 0 {
			return nil, fmt.Errorf("d2world: seek: row %s has not looked and counts %d candidates", r.Watcher, r.Candidates)
		}

		last = r.Watcher
		rows[r.Watcher] = &seekRow{target: r.Target, reason: r.Reason, untilLook: r.UntilLook, candidates: r.Candidates}
	}

	return rows, nil
}
