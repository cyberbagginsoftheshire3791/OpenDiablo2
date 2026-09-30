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
//     defers it back), and Snapshot refuses to run while it is set. Since the
//     raid's R2 it hides him alone (player, below), and it is still one flag:
//     the brief's "a list of quarry refs" has one member, him, and is refused
//     while set, so the file never carries it (no shape change).
//   - player is WIRING: his id, bound every frame by the game screen.
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

	// Side is the raid's R2: "hostile" or "living" (WatchSide). An amendment
	// of version 2's shape, no bump (the milestone's rule). Required: a watch
	// with no side, or one the model does not know, is refused -- read as
	// either, it would make a wolf a villager or a villager a wolf.
	Side string `json:"side"`

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
			Side:          string(w.side),
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
//
// It restores only into a model that watches NO ONE (the B2b review's B4, as
// B2a's corpses and squads refuse a used target): a live watch replaced by a
// saved one would leave its watcher forgotten mid-hunt, and one kept beside
// the saved ones would be a watch no saved moment had.
func (n *Notice) Restore(snap NoticeSnapshot, r Resolver) error {
	watches, err := n.b2bValidate(snap, r)
	if err != nil {
		return err
	}

	n.watches = watches
	n.checks = snap.Checks
	n.notices = snap.Notices

	return nil
}

// Validate is Restore's check and nothing else (D4): every refusal Restore
// would make, through the same Resolver, without changing the model.
func (n *Notice) Validate(snap NoticeSnapshot, r Resolver) error {
	_, err := n.b2bValidate(snap, r)

	return err
}

// CheckSnapshot is every check Validate makes of the snapshot itself --
// everything but its refusal of a model already in use. Game.SaveWorld runs it on
// the snapshot it has just taken from this live model, so a save never writes a
// block the load's own Validate would refuse (the M4.6 B3 review, B2: "strict
// at save" stopped at the file's own checks). Validate is the in-use refusal
// and this, so the two cannot disagree.
func (n *Notice) CheckSnapshot(snap NoticeSnapshot, r Resolver) error {
	_, err := n.b2bBuild(snap, r)

	return err
}

// b2bValidate checks the whole snapshot and returns the watches it would
// restore. Nothing in n is changed.
func (n *Notice) b2bValidate(snap NoticeSnapshot, r Resolver) (map[string]*watch, error) {
	if len(n.watches) != 0 {
		return nil, fmt.Errorf("d2world: notice: restore into a model that watches no one; this one watches %d", len(n.watches))
	}

	return n.b2bBuild(snap, r)
}

// b2bBuild is b2bValidate's checks of the snapshot itself, and the watches it
// would restore: nothing of the model's own state is read or changed.
func (n *Notice) b2bBuild(snap NoticeSnapshot, r Resolver) (map[string]*watch, error) {
	if err := b2bCheckNumbers("notice", true,
		b2bNum{"checks", float64(snap.Checks)}, b2bNum{"notices", float64(snap.Notices)}); err != nil {
		return nil, err
	}

	watches := make(map[string]*watch, len(snap.Watches))

	for i := range snap.Watches {
		ws := snap.Watches[i]

		if _, dup := watches[ws.Watcher]; dup {
			return nil, fmt.Errorf("d2world: notice: watcher %s is saved twice", ws.Watcher)
		}

		if err := b2bCheckNumbers("notice watch "+ws.Watcher, true,
			b2bNum{"distance", ws.Distance}, b2bNum{"light_at_target", ws.LightAtTarget},
			b2bNum{"reach", ws.Reach}, b2bNum{"since_check_minutes", ws.SinceCheck},
			b2bNum{"since_seen_minutes", ws.SinceSeen}, b2bNum{"checks", float64(ws.Checks)},
			b2bNum{"notices", float64(ws.Notices)}); err != nil {
			return nil, err
		}

		if !validSide(WatchSide(ws.Side)) {
			return nil, fmt.Errorf("d2world: notice: watcher %s: side %q is neither %q nor %q",
				ws.Watcher, ws.Side, SideHostile, SideLiving)
		}

		watcher, err := b2bResolveWatcher(r, ws.Watcher)
		if err != nil {
			return nil, fmt.Errorf("notice: %w", err)
		}

		target, err := b2bResolveQuarry(r, ws.Target)
		if err != nil {
			return nil, fmt.Errorf("notice: watcher %s: %w", ws.Watcher, err)
		}

		watches[ws.Watcher] = &watch{
			watcher:       watcher,
			target:        target,
			side:          WatchSide(ws.Side),
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

	return watches, nil
}
