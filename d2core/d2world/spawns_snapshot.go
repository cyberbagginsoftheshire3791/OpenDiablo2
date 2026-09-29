package d2world

import (
	"errors"
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// SpawnsSnapshot is the spawn tables' state as the world save carries it
// (M4.6 B2b, build plan §1).
//
// SAVED: every group, the next group's number, the minutes run toward the
// next table check, the open-body count, the eight counters, and where the
// tables' stream stands.
//
// NOT SAVED, and why:
//   - sheltered is TRANSIENT. It is true only inside a sleep (talk.go sets it
//     and defers it back), and Snapshot refuses to run while it is set.
//   - the dials are DERIVED: DefaultSpawnDials. A dial a script moved is the
//     script's, not the save's; a save that carried dials would carry the
//     next build's tuning backwards.
//   - the clock, notice, illumination, spawner, chases, target and layDead
//     hook are the game's wiring, rebound when the game is built.
//
// Members are saved by entity id, dead ones included (member_ids, B1): the
// group keeps its dead, and the game reads them there.
type SpawnsSnapshot struct {
	Groups []SpawnGroupSnapshot `json:"groups"`

	NextID     int     `json:"next_id"`
	SinceCheck float64 `json:"since_check_minutes"`
	OpenBodies int     `json:"open_bodies"`

	Checks    int `json:"checks"`
	Rolls     int `json:"rolls"`
	Spawned   int `json:"spawned"`
	Failures  int `json:"failures"`
	Dropped   int `json:"dropped"`
	Despawned int `json:"despawned"`
	Cleared   int `json:"cleared"`
	Released  int `json:"released"`

	// RNG is the tables' stream (d2rand.Stream) as {seed, draws}: the one
	// shape every saved stream takes (d2rand.StreamState, which B2a's and
	// B2b's two private copies became at the B2 review).
	RNG d2rand.StreamState `json:"rng"`
}

// SpawnGroupSnapshot is one arrival.
type SpawnGroupSnapshot struct {
	ID      string                `json:"id"`
	Row     string                `json:"row"`
	Code    string                `json:"code"`
	Members []SpawnMemberSnapshot `json:"members"`

	Morale    float64      `json:"morale"`
	BornAt    float64      `json:"born_at"`
	Band      int          `json:"band"`
	Stage     string       `json:"stage"`
	Weight    float64      `json:"weight"`
	Spawned   int          `json:"spawned"`
	BornWhere [][2]float64 `json:"born_where"`
}

// SpawnMemberSnapshot is one member: his entity id, where he stood when the
// save was made (world tiles), and whether he was on the map at all.
//
// THE POSITION IS AN IDENTITY CHECK, not a second copy of his motion. Restore
// holds the entity the Resolver returns to it: a member that resolves to an
// entity standing somewhere else is the wrong entity, or one whose motion was
// not restored first, and either way the pack would be hunting with the
// wrong man.
//
// GONE is a member who was in his group and NOT on the map when the save was
// made: a slain man whose body stood up as one of the dead, so the rising
// took his remains away (Game.takeOffTheMap). It is decided at save, by the
// same Resolver the load uses, so neither side guesses: a gone member must
// not resolve at load, and every other member must.
type SpawnMemberSnapshot struct {
	ID   string  `json:"id"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Gone bool    `json:"gone,omitempty"`
}

// Snapshot is the tables' state now. r is a Resolver over the LIVE world --
// the one the load will use, built over the map as it stands -- and it
// decides which members are on the map (see SpawnMemberSnapshot.Gone).
//
// It refuses while he shelters: that flag is transient and is never set when
// a save is allowed.
func (s *Spawns) Snapshot(r Resolver) (SpawnsSnapshot, error) {
	if s.sheltered {
		return SpawnsSnapshot{}, errors.New("d2world: spawns: cannot snapshot while he shelters (a sleep is running)")
	}

	if r == nil {
		return SpawnsSnapshot{}, fmt.Errorf("%w: spawns: no resolver to say which members are on the map", ErrUnresolvedRef)
	}

	snap := SpawnsSnapshot{
		Groups:     make([]SpawnGroupSnapshot, 0, len(s.groups)),
		NextID:     s.nextID,
		SinceCheck: s.sinceCk,
		OpenBodies: s.openBodies,
		Checks:     s.checks,
		Rolls:      s.rolls,
		Spawned:    s.spawned,
		Failures:   s.failures,
		Dropped:    s.dropped,
		Despawned:  s.despawned,
		Cleared:    s.cleared,
		Released:   s.released,
	}

	if s.rng != nil {
		snap.RNG = d2rand.StateOf(s.rng)
	}

	for _, id := range s.groupIDs() {
		g := s.groups[id]

		members := make([]SpawnMemberSnapshot, 0, len(g.members))

		for i, m := range g.members {
			if m == nil {
				return SpawnsSnapshot{}, fmt.Errorf("d2world: spawns: group %s member %d is nil", g.id, i)
			}

			ms, err := s.b2bSaveMember(m, g.id, r)
			if err != nil {
				return SpawnsSnapshot{}, err
			}

			members = append(members, ms)
		}

		snap.Groups = append(snap.Groups, SpawnGroupSnapshot{
			ID:        g.id,
			Row:       g.row,
			Code:      g.code,
			Members:   members,
			Morale:    g.morale,
			BornAt:    g.bornAt,
			Band:      g.band,
			Stage:     g.stage.String(),
			Weight:    g.weight,
			Spawned:   g.spawned,
			BornWhere: append([][2]float64{}, g.bornWhere...),
		})
	}

	return snap, nil
}

// Restore puts the tables where a snapshot left them, resolving every member
// through r. worldSeed is the saved game's seed: the stream must be the one
// that game ran the tables on (d2rand.StreamState.Check). It is
// all-or-nothing: a snapshot that fails any check (Validate's) leaves the
// tables exactly as they were.
//
// It restores only into tables that hold NO GROUP (the B2b review's B4, as
// B2a's corpses and squads refuse a used target): a group's members are map
// entities the notice model watches and pursuit chases with, and replacing a
// live group would leave them standing, watched and chasing, in no pack --
// and a daybreak despawn would never take them home.
//
// ORDER AT LOAD: the entities (ids and motion) first, then this, then Notice,
// then Pursuit (build plan §5). A member that resolves must stand where the
// save says he stood, so his motion must already be restored.
//
// It watches nothing, spawns nothing and releases nothing. The notice model's
// watches come back through Notice.Restore, and a restore that called Watch
// here would evaluate every member a second time and move its counters.
func (s *Spawns) Restore(snap SpawnsSnapshot, r Resolver, worldSeed int64) error {
	groups, err := s.b2bValidate(snap, r, worldSeed)
	if err != nil {
		return err
	}

	s.groups = groups
	s.nextID = snap.NextID
	s.sinceCk = snap.SinceCheck
	s.openBodies = snap.OpenBodies
	s.checks, s.rolls, s.spawned = snap.Checks, snap.Rolls, snap.Spawned
	s.failures, s.dropped, s.despawned = snap.Failures, snap.Dropped, snap.Despawned
	s.cleared, s.released = snap.Cleared, snap.Released

	// Stream.Restore replaces the rand and its counted source together.
	if s.rng == nil {
		s.rng = d2rand.NewStream(snap.RNG.Seed)
	}

	snap.RNG.RestoreInto(s.rng)

	return nil
}

// Validate is Restore's check and nothing else (D4): every refusal Restore
// would make, through the same Resolver, without changing the tables.
func (s *Spawns) Validate(snap SpawnsSnapshot, r Resolver, worldSeed int64) error {
	_, err := s.b2bValidate(snap, r, worldSeed)

	return err
}

// CheckSnapshot is every check Validate makes of the snapshot itself --
// everything but its refusal of a table set already in use. Game.SaveWorld runs it on
// the snapshot it has just taken from this live table set, so a save never writes a
// block the load's own Validate would refuse (the M4.6 B3 review, B2: "strict
// at save" stopped at the file's own checks). Validate is the in-use refusal
// and this, so the two cannot disagree.
func (s *Spawns) CheckSnapshot(snap SpawnsSnapshot, r Resolver, worldSeed int64) error {
	_, err := s.b2bBuild(snap, r, worldSeed)

	return err
}

// b2bValidate checks the whole snapshot and returns the groups it would
// restore. Nothing in s is changed.
func (s *Spawns) b2bValidate(snap SpawnsSnapshot, r Resolver, worldSeed int64) (map[string]*group, error) {
	if len(s.groups) != 0 {
		return nil, fmt.Errorf("d2world: spawns: restore into tables that hold no group; these hold %d", len(s.groups))
	}

	return s.b2bBuild(snap, r, worldSeed)
}

// b2bBuild is b2bValidate's checks of the snapshot itself, and the groups it
// would restore: nothing of the tables' own state is read or changed.
func (s *Spawns) b2bBuild(snap SpawnsSnapshot, r Resolver, worldSeed int64) (map[string]*group, error) {
	if err := s.b2bCheckTop(snap); err != nil {
		return nil, err
	}

	if err := snap.RNG.Check(worldSeed, d2rand.StreamSpawns); err != nil {
		return nil, fmt.Errorf("d2world: spawns: %w", err)
	}

	groups := make(map[string]*group, len(snap.Groups))
	members := make(map[string]string)

	for i := range snap.Groups {
		gs := snap.Groups[i]

		g, err := s.b2bGroup(gs, snap.NextID, r, members)
		if err != nil {
			return nil, err
		}

		if _, dup := groups[g.id]; dup {
			return nil, fmt.Errorf("d2world: spawns: group %s is saved twice", g.id)
		}

		groups[g.id] = g
	}

	return groups, nil
}

// b2bCheckTop checks the snapshot's own numbers.
func (s *Spawns) b2bCheckTop(snap SpawnsSnapshot) error {
	if snap.NextID < 1 {
		return fmt.Errorf("d2world: spawns: next_id %d; the first group is g:1", snap.NextID)
	}

	counts := []b2bNum{
		{"open_bodies", float64(snap.OpenBodies)}, {"checks", float64(snap.Checks)},
		{"rolls", float64(snap.Rolls)}, {"spawned", float64(snap.Spawned)},
		{"failures", float64(snap.Failures)}, {"dropped", float64(snap.Dropped)},
		{"despawned", float64(snap.Despawned)}, {"cleared", float64(snap.Cleared)},
		{"released", float64(snap.Released)}, {"since_check_minutes", snap.SinceCheck},
	}

	return b2bCheckNumbers("spawns", true, counts...)
}

// b2bGroup rebuilds one saved group. members maps every member id seen so far
// to its group, so an id saved in two places is refused.
func (s *Spawns) b2bGroup(gs SpawnGroupSnapshot, nextID int, r Resolver, members map[string]string) (*group, error) {
	var n int
	if _, err := fmt.Sscanf(gs.ID, "g:%d", &n); err != nil || fmt.Sprintf("g:%d", n) != gs.ID || n < 1 {
		return nil, fmt.Errorf("d2world: spawns: %q is not a group id", gs.ID)
	}

	// A group numbered at or past next_id would be minted AGAIN by the next
	// arrival, and the second would overwrite the first in the map.
	if n >= nextID {
		return nil, fmt.Errorf("d2world: spawns: group %s is at or past next_id %d", gs.ID, nextID)
	}

	if _, ok := s.rowNamed(gs.Row); !ok {
		return nil, fmt.Errorf("d2world: spawns: group %s is of row %q, which this build's tables do not have", gs.ID, gs.Row)
	}

	stage, ok := b2bStageNamed(gs.Stage)
	if !ok {
		return nil, fmt.Errorf("d2world: spawns: group %s was born in stage %q", gs.ID, gs.Stage)
	}

	if bands := len(SpawnRow{}.BandWeight); gs.Band < -1 || gs.Band >= bands {
		return nil, fmt.Errorf("d2world: spawns: group %s was born in band %d", gs.ID, gs.Band)
	}

	what := "spawns group " + gs.ID

	if err := b2bCheckNumbers(what, false, b2bNum{"morale", gs.Morale}, b2bNum{"weight", gs.Weight}); err != nil {
		return nil, err
	}

	if err := b2bCheckNumbers(what, true, b2bNum{"born_at", gs.BornAt}, b2bNum{"spawned", float64(gs.Spawned)}); err != nil {
		return nil, err
	}

	for _, p := range gs.BornWhere {
		if err := b2bCheckNumbers(what, false, b2bNum{"born_where", p[0]}, b2bNum{"born_where", p[1]}); err != nil {
			return nil, err
		}
	}

	// adopt never makes a group with no one in it (spawn returns first).
	if len(gs.Members) == 0 {
		return nil, fmt.Errorf("d2world: spawns: group %s has no members", gs.ID)
	}

	g := &group{
		id:        gs.ID,
		row:       gs.Row,
		code:      gs.Code,
		members:   make([]Watcher, 0, len(gs.Members)),
		morale:    gs.Morale,
		bornAt:    gs.BornAt,
		band:      gs.Band,
		stage:     stage,
		weight:    gs.Weight,
		spawned:   gs.Spawned,
		bornWhere: append([][2]float64{}, gs.BornWhere...),
	}

	for _, ms := range gs.Members {
		m, err := b2bMember(ms, gs.ID, r, members)
		if err != nil {
			return nil, err
		}

		g.members = append(g.members, m)
	}

	return g, nil
}

// b2bSaveMember writes one member, asking r whether he is on the map. One who
// is must be the very entity the group holds -- an id the live world answers
// with a different thing would be saved as a lie.
//
// GONE IS CHECKED AGAINST THE NOTICE MODEL, not taken on the Resolver's word.
// A man leaves the map while his pack lives on only after he has died (his
// body stood up and took his remains), and a death unwatches him
// (Combat.withdraw) before that. So a member the Resolver cannot find while
// the notice model still watches him is a living wolf the Resolver lost --
// and saved as gone, he would come back a stand-in while his entity walked on
// beside the pack, and daybreak would send the pack home without him. That is
// refused at save.
func (s *Spawns) b2bSaveMember(m Watcher, groupID string, r Resolver) (SpawnMemberSnapshot, error) {
	id := m.WatcherID()
	x, y := m.WatcherAt()
	ms := SpawnMemberSnapshot{ID: id, X: x, Y: y}

	live, ok := r.Watcher(id)
	if !ok || live == nil {
		if s.notice != nil {
			if _, watched := s.notice.Noticed(id); watched {
				return ms, fmt.Errorf("%w: spawns: group %s member %s is not in the live world, and the notice model still watches him",
					ErrUnresolvedRef, groupID, id)
			}
		}

		ms.Gone = true

		return ms, nil
	}

	if got := live.WatcherID(); got != id {
		return ms, fmt.Errorf("%w: spawns: group %s member %s is %q in the live world", ErrUnresolvedRef, groupID, id, got)
	}

	if lx, ly := live.WatcherAt(); lx != x || ly != y {
		return ms, fmt.Errorf("%w: spawns: group %s member %s is at %v,%v and the live world's %s at %v,%v",
			ErrUnresolvedRef, groupID, id, x, y, id, lx, ly)
	}

	return ms, nil
}

// b2bMember resolves one saved member.
func b2bMember(ms SpawnMemberSnapshot, groupID string, r Resolver, members map[string]string) (Watcher, error) {
	if ms.ID == "" || ms.ID == PlayerRef {
		return nil, fmt.Errorf("d2world: spawns: group %s has a member with id %q", groupID, ms.ID)
	}

	if other, dup := members[ms.ID]; dup {
		return nil, fmt.Errorf("d2world: spawns: member %s is saved in %s and in %s", ms.ID, other, groupID)
	}

	members[ms.ID] = groupID

	if err := b2bCheckNumbers("spawns member "+ms.ID, false, b2bNum{"x", ms.X}, b2bNum{"y", ms.Y}); err != nil {
		return nil, err
	}

	if r == nil {
		return nil, fmt.Errorf("%w: spawns: no resolver for member %s", ErrUnresolvedRef, ms.ID)
	}

	w, ok := r.Watcher(ms.ID)

	if ms.Gone {
		// Off the map when saved, so off it now. An entity answering to a
		// dead man's id is two things with one name.
		if ok && w != nil {
			return nil, fmt.Errorf("%w: spawns: member %s was off the map when saved and something answers to him now",
				ErrUnresolvedRef, ms.ID)
		}

		return &b2bGoneMember{id: ms.ID, x: ms.X, y: ms.Y}, nil
	}

	if !ok || w == nil {
		return nil, fmt.Errorf("%w: spawns: member %s of group %s is not in it", ErrUnresolvedRef, ms.ID, groupID)
	}

	if got := w.WatcherID(); got != ms.ID {
		return nil, fmt.Errorf("%w: spawns: member %s came back as %q", ErrUnresolvedRef, ms.ID, got)
	}

	if x, y := w.WatcherAt(); x != ms.X || y != ms.Y {
		return nil, fmt.Errorf("%w: spawns: member %s stands at %v,%v, saved at %v,%v -- the wrong entity, or its motion is not restored yet",
			ErrUnresolvedRef, ms.ID, x, y, ms.X, ms.Y)
	}

	return w, nil
}

// b2bGoneMember stands in for a pack member who is in his group and not on the
// map: a slain man whose body stood up as one of the dead, so the rising took
// his remains away (Game.takeOffTheMap). The group keeps him -- spent() reads
// him, his profile still names his pack, member_ids reports him -- and in the
// saved game he was a chaser wrapping an entity no longer on the map. What
// the game ever asked of that entity is his id and where he last stood, and
// this answers both.
//
// A despawn skips him (the game spawner takes only its own chasers off the
// map), which is what the saved game did too: his remains were already gone
// and his body already released.
type b2bGoneMember struct {
	id   string
	x, y float64
}

func (g *b2bGoneMember) WatcherID() string { return g.id }

func (g *b2bGoneMember) WatcherAt() (x, y float64) { return g.x, g.y }

// b2bStageNamed is Stage.String read backwards.
func b2bStageNamed(name string) (Stage, bool) {
	for _, st := range []Stage{StageNight, StageDawn, StageDay, StageDusk} {
		if st.String() == name {
			return st, true
		}
	}

	return 0, false
}
