package d2world

import (
	"errors"
	"fmt"
)

// NoticeSnapshot is the awareness model's state as the world save carries it
// (M4.6 B2b, build plan §1).
//
// SAVED: every watch -- who watches, what it watches, and everything its last
// sight test left behind -- and the two totals.
//
// NOT SAVED, and why:
//   - hidden is TRANSIENT. It is true only inside a sleep (talk.go sets it and
//     defers it back), and Snapshot refuses to run while it is set.
//   - the dials, the radius above all, are DERIVED. The Quiet Step talent
//     takes its bonus off Dials().Radius when the game is built, so a saved
//     radius restored over it would take the bonus twice (the plan's trap 7).
//     Reach, below, is not a dial: it is what the last sight test used, and
//     it stays until the next one.
//   - sight and illumination are the game's wiring.
//
// A watch is keyed by its watcher, and the watcher is written as its entity
// id; the target is written as PlayerRef when it is the player (his id is new
// every launch) and as its entity id otherwise.
type NoticeSnapshot struct {
	Watches []WatchSnapshot `json:"watches"`
	Checks  int             `json:"checks"`
	Notices int             `json:"notices"`
}

// WatchSnapshot is one watcher's awareness of one target.
type WatchSnapshot struct {
	Watcher string `json:"watcher"`
	Target  string `json:"target"`

	Sees    bool `json:"sees"`
	Noticed bool `json:"noticed"`

	Distance      float64 `json:"distance"`
	LightAtTarget float64 `json:"light_at_target"`
	Reach         float64 `json:"reach"`

	SinceCheck float64 `json:"since_check_minutes"`
	SinceSeen  float64 `json:"since_seen_minutes"`

	Checks  int `json:"checks"`
	Notices int `json:"notices"`
}

// Snapshot is the model's state now, in the stable watcher order. r is a
// Resolver over the LIVE world: it names the player (every target that is he
// is written as PlayerRef), and every watcher and target must resolve through
// it now, or the load could not resolve them either. Refused while he is
// hidden (a sleep is running).
func (n *Notice) Snapshot(r Resolver) (NoticeSnapshot, error) {
	if n.hidden {
		return NoticeSnapshot{}, errors.New("d2world: notice: cannot snapshot while he is hidden (a sleep is running)")
	}

	snap := NoticeSnapshot{
		Watches: make([]WatchSnapshot, 0, len(n.watches)),
		Checks:  n.checks,
		Notices: n.notices,
	}

	if len(n.watches) == 0 {
		return snap, nil
	}

	playerID, err := b2bLivePlayerID(r, "notice")
	if err != nil {
		return NoticeSnapshot{}, err
	}

	for _, id := range n.watcherIDs() {
		w := n.watches[id]

		ws := WatchSnapshot{
			Watcher:       id,
			Target:        b2bQuarryRef(w.target, playerID),
			Sees:          w.sees,
			Noticed:       w.noticed,
			Distance:      w.distance,
			LightAtTarget: w.lightAtTarget,
			Reach:         w.reach,
			SinceCheck:    w.sinceCheck,
			SinceSeen:     w.sinceSeen,
			Checks:        w.checks,
			Notices:       w.notices,
		}

		if err := b2bSavedRefsResolve(r, "notice", ws.Watcher, ws.Target); err != nil {
			return NoticeSnapshot{}, err
		}

		snap.Watches = append(snap.Watches, ws)
	}

	return snap, nil
}

// Restore puts the model where a snapshot left it, resolving every watcher and
// target through r. A watch that does not resolve is an ERROR, not a silent
// drop. It is all-or-nothing: a snapshot that fails any check leaves the
// model exactly as it was.
//
// It evaluates nothing. Watch() runs a sight test at once, which would move
// every counter and reset every since-check; a restored watch looks again
// when its own saved clock says so.
func (n *Notice) Restore(snap NoticeSnapshot, r Resolver) error {
	if err := b2bCheckNumbers("notice", true,
		b2bNum{"checks", float64(snap.Checks)}, b2bNum{"notices", float64(snap.Notices)}); err != nil {
		return err
	}

	watches := make(map[string]*watch, len(snap.Watches))

	for i := range snap.Watches {
		ws := snap.Watches[i]

		if _, dup := watches[ws.Watcher]; dup {
			return fmt.Errorf("d2world: notice: watcher %s is saved twice", ws.Watcher)
		}

		if err := b2bCheckNumbers("notice watch "+ws.Watcher, true,
			b2bNum{"distance", ws.Distance}, b2bNum{"light_at_target", ws.LightAtTarget},
			b2bNum{"reach", ws.Reach}, b2bNum{"since_check_minutes", ws.SinceCheck},
			b2bNum{"since_seen_minutes", ws.SinceSeen}, b2bNum{"checks", float64(ws.Checks)},
			b2bNum{"notices", float64(ws.Notices)}); err != nil {
			return err
		}

		watcher, err := b2bResolveWatcher(r, ws.Watcher)
		if err != nil {
			return fmt.Errorf("notice: %w", err)
		}

		target, err := b2bResolveQuarry(r, ws.Target)
		if err != nil {
			return fmt.Errorf("notice: watcher %s: %w", ws.Watcher, err)
		}

		watches[ws.Watcher] = &watch{
			watcher:       watcher,
			target:        target,
			noticed:       ws.Noticed,
			sees:          ws.Sees,
			distance:      ws.Distance,
			lightAtTarget: ws.LightAtTarget,
			reach:         ws.Reach,
			sinceCheck:    ws.SinceCheck,
			sinceSeen:     ws.SinceSeen,
			checks:        ws.Checks,
			notices:       ws.Notices,
		}
	}

	n.watches = watches
	n.checks = snap.Checks
	n.notices = snap.Notices

	return nil
}

// b2bLivePlayerID is the live player's id, which a snapshot writes as
// PlayerRef. A world with watches or chases and no player to name cannot be
// saved: every record pointing at him would be written as an id the next
// launch never has.
func b2bLivePlayerID(r Resolver, what string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("%w: %s: no resolver to name the player", ErrUnresolvedRef, what)
	}

	q, ok := r.Quarry(PlayerRef)
	if !ok || q == nil || q.QuarryID() == "" {
		return "", fmt.Errorf("%w: %s: the live world has no player to write as %q", ErrUnresolvedRef, what, PlayerRef)
	}

	return q.QuarryID(), nil
}

// b2bSavedRefsResolve checks, at save, that a record's watcher and quarry refs
// resolve in the live world -- a file whose refs cannot resolve now could not
// resolve at load either, and is refused before it is written.
func b2bSavedRefsResolve(r Resolver, what, watcherID, quarryRef string) error {
	if _, err := b2bResolveWatcher(r, watcherID); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}

	if _, err := b2bResolveQuarry(r, quarryRef); err != nil {
		return fmt.Errorf("%s: %s: %w", what, watcherID, err)
	}

	return nil
}
