package d2world

import (
	"fmt"
	"math"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// Seek is who the night's hunters choose among the living (the raid milestone
// "the village at night", burst R2, 29 Sep 2026; the brief's P2 (c)).
//
// Ruling 1 of 28 Sep: "a general notice model (any hostile watcher, any
// living quarry)". Notice keeps its signed fence -- it may know LINE OF SIGHT,
// DISTANCE and THE LIGHT AT THE TARGET, nothing else -- so it cannot be what
// chooses among quarries: the dead's rule needs the router (ruling 3), the
// corpse registry and the light sources. Target choice moves here, into a
// small system of its own, and Notice only carries out the choice
// (Notice.Retarget).
//
// WHAT R2 BUILDS, AND WHAT IT LEAVES:
//   - BEASTS AND MEN: a hostile watcher takes THE NEAREST QUARRY IT CAN SEE
//     within Notice's own reach (the same arithmetic, Notice.reachAt, and the
//     same Sight), not always him. Nothing in view: it keeps its watch as it
//     is, and Notice's memory runs on as ever.
//   - A FIGHTER KEEPS ITS TARGET: a watcher that is a living enemy of a live
//     fight -- his, or one he is not in -- is not retargeted until its part
//     in that fight ends (R2's pinned engagement; the raid R1 review's J-R1-3,
//     default (a): a monster at his throat does not walk off mid-blow).
//   - THE DEAD ARE NOT SERVED YET: their draw is P6 (the nearest REACHABLE
//     living, then light, then unrited bodies), which is R5's and R7's. A
//     watcher of the spawn tables' dead row keeps the watch the tables gave
//     it, exactly as before R2.
//   - THE VILLAGE'S OWN SIDE (SideLiving) is never served: a villager on
//     watch chooses nothing and hunts nothing.
//
// THE CANDIDATES are the living the game names (SetQuarries: him, the
// deployed squad models, the map's villagers), the stand-ins a script names
// (stand_in, until R3b's members), and the watch's own current target, which
// is always its own candidate -- whoever made the watch made it a quarry, and
// a wolf watching the villager beside it does not turn to a man twelve tiles
// off. Each must pass the same four gates: not the watcher itself, not
// protected (the raid's S0-1 (a): the four speakers are no quarry until
// their death art lands -- the one rule Combat.SetProtected holds, read from
// there), not hidden (his sleep: Notice.SetHidden), and not dead by its body.
//
// "NEAREST" IS BY STRAIGHT LINE (the raid's S0-2, default (a)): no A* inside a
// frame (D-S2 = 0), because ordering by route cost 45 ms in the worst frame
// at N 24 x M 30 against a 2 ms budget (S0's M0.2). Beasts and men choose by
// sight, which is a straight line anyway; the dead's "reachable" is the
// region lookup's, and lands with them.
//
// THE STAGGER, BUILT FROM THE START (S0's M0.2): each watcher looks every
// RetargetMinutes on a phase of its own -- the minute cut into StaggerSlots
// phases, given out round-robin as rows are made -- so a pack that arrives in
// one frame looks across the minute's night frames, not all in one. At 4x the
// brief's N and M the unstaggered frame is over budget (3.4 ms); the
// benchmark beside this file measures both.
//
// Like every system here it steps on the world clock, never the wall clock,
// and it iterates in a fixed order: its choices move entities, which are
// inside the state digest.
type Seek struct {
	dials SeekDials

	notice *Notice
	spawns *Spawns
	combat *Combat

	// quarries is the game's living (SetQuarries); resolver finds a stand-in
	// by its id (SetResolver). Both are the game screen's wiring.
	quarries func() []Quarry
	resolver Resolver

	// standIns are entity ids a script has named living (the provider's
	// stand_in / stand_in_remove), in the order they were named: the
	// villagers a playtest stands in for until R3b puts members on the map.
	standIns []string

	rows map[string]*seekRow

	// slots counts the stagger phases given out: row n takes phase
	// n mod StaggerSlots.
	slots int

	// looks counts the looks taken, retargets the watches moved, rays the
	// sight tests the looks cast.
	looks, retargets, rays int
}

// seekEpsilon is how near zero a row's untilLook may be and still come due:
// far below a frame's minutes (1/24 at night), far above a float's error.
const seekEpsilon = 1e-9

// seekRow is one hostile watcher's standing with Seek.
type seekRow struct {
	// target is what the row last found its watch on, written as a snapshot
	// writes a quarry (PlayerRef for him); reason is why (the Seek* reasons).
	target string
	reason string

	// untilLook is the world minutes until the row next looks. Its phase is
	// built in: a look adds whole RetargetMinutes, so the phase a row was
	// given when it was made is the phase it keeps.
	untilLook float64

	// candidates is how many living quarries were within reach at the last
	// look (each cost one ray until the nearest clear one was found).
	candidates int
}

// Why a row holds the target it holds.
const (
	// SeekPending is a row made and not yet looked: its phase is still to come.
	SeekPending = "pending"
	// SeekLiving is a row whose watch is on the nearest living quarry it
	// could see at its last look (moved there, or already there).
	SeekLiving = "living"
	// SeekNone is a row that saw no living quarry within reach at its last
	// look: the watch was kept as it was.
	SeekNone = "none"
	// SeekFighting is a row whose watcher was a living enemy of a live fight
	// at its last look: a fighter keeps its target.
	SeekFighting = "fighting"
)

func validSeekReason(r string) bool {
	return r == SeekPending || r == SeekLiving || r == SeekNone || r == SeekFighting
}

// SeekDials are Seek's numbers. Every one is a [DIAL], and none is saved.
//
// EVERY RATE IS IN WORLD MINUTES, for the reason on NoticeDials: at the
// harness's tick of 1/60 s a world minute is ~24 stepped frames at night
// (NightRate 2.5) and ~15 by day (DayRate 4.0).
type SeekDials struct {
	// RetargetMinutes is how often one watcher re-chooses its quarry: D-S1,
	// starting at Notice's own ReEvaluateMinutes (1.0), so a watcher looks
	// for the nearest living as often as it looks at all. ~24 night frames.
	RetargetMinutes float64

	// StaggerSlots is how many phases a minute of looks is spread across:
	// 24, the night frames of one world minute, so at most ceil(N/24) of N
	// watchers look in any night frame.
	StaggerSlots int
}

// DefaultSeekDials returns the brief's starting values (§5, D-S1).
func DefaultSeekDials() SeekDials {
	return SeekDials{RetargetMinutes: 1.0, StaggerSlots: 24}
}

// NewSeek builds the system and registers the "seek" harness provider. It
// reads the notice model's watches and carries out its choice through it,
// asks the spawn tables which row a watcher is of (the dead are not served
// yet), and asks combat who is fighting, who is dead by his body and who is
// protected. Any of the three may be nil: nil notice sees nothing, nil
// spawns knows no row (every hostile watcher is served), nil combat has no
// fights, no bodies and no protection.
func NewSeek(notice *Notice, spawns *Spawns, combat *Combat, dials SeekDials) *Seek {
	s := &Seek{
		dials:  dials,
		notice: notice,
		spawns: spawns,
		combat: combat,
		rows:   map[string]*seekRow{},
	}

	d2harness.Register(s)

	return s
}

// Close unregisters the provider.
func (s *Seek) Close() { d2harness.Unregister(s) }

// SetQuarries attaches the game's living: every quarry a hostile might
// choose, in any order (Seek sorts them). Nil names nobody but each watch's
// own target and the stand-ins.
func (s *Seek) SetQuarries(living func() []Quarry) { s.quarries = living }

// SetResolver attaches what finds a stand-in by its id, and names him (a row
// on him is written PlayerRef): the game screen's worldResolver, as combat's.
func (s *Seek) SetResolver(r Resolver) { s.resolver = r }

// Dials exposes the current dials.
func (s *Seek) Dials() SeekDials { return s.dials }

// Advance steps every row by the world minutes that passed, and lets each row
// whose phase has come look once.
//
// A long step -- a sleep's hour, a labour's -- looks ONCE per row however
// many phases it spans, and the row's phase is kept: the world moved in one
// piece, so there is one moment to look at it.
func (s *Seek) Advance(worldMinutes float64) {
	if worldMinutes <= 0 || s.notice == nil {
		return
	}

	fresh := s.sync()

	if len(s.rows) == 0 {
		return
	}

	r := s.dials.RetargetMinutes
	if r <= 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		return
	}

	var living []Quarry

	built := false

	for _, id := range s.rowIDs() {
		row := s.rows[id]

		// A row made this step does not age in it: its phase counts from here.
		if !fresh[id] {
			row.untilLook -= worldMinutes
		}

		// The epsilon is the round loop's lesson (BUG-85): a phase of k/24
		// minutes, less k frames of 1/24 minute each, lands a hair either
		// side of zero, and a hair above it would push the row's look into
		// the next frame -- two phases bunched in one frame, the stagger
		// undone by rounding.
		if row.untilLook > seekEpsilon {
			continue
		}

		row.untilLook += r * (math.Floor((seekEpsilon-row.untilLook)/r) + 1)

		if !built {
			living, built = s.living(), true
		}

		s.look(id, row, living)
	}
}

// sync makes a row for every served watcher that has none -- each given the
// next stagger phase, in watcher-id order -- and drops the row of every
// watcher that no longer watches, or is no longer served. It returns the rows
// it made.
func (s *Seek) sync() map[string]bool {
	for id := range s.rows {
		if !s.served(id) {
			delete(s.rows, id)
		}
	}

	var fresh map[string]bool

	for _, id := range s.notice.watcherIDs() {
		if _, ok := s.rows[id]; ok || !s.served(id) {
			continue
		}

		row := &seekRow{
			target:    s.ref(s.notice.watches[id].target),
			reason:    SeekPending,
			untilLook: s.phase(s.slots),
		}
		s.slots++
		s.rows[id] = row

		if fresh == nil {
			fresh = map[string]bool{}
		}

		fresh[id] = true
	}

	return fresh
}

// phase is the stagger phase of the n-th row made: the minute cut into
// StaggerSlots, given out round-robin. Row 0's phase is 0: it looks at once.
func (s *Seek) phase(n int) float64 {
	slots := s.dials.StaggerSlots
	if slots < 1 {
		slots = 1
	}

	return s.dials.RetargetMinutes * float64(n%slots) / float64(slots)
}

// served reports that Seek chooses for this watcher: a hostile watch that is
// not the dead's (R2 serves beasts and men; the dead's draw is R5's).
func (s *Seek) served(id string) bool {
	w, ok := s.notice.watches[id]
	if !ok || w.side != SideHostile {
		return false
	}

	if s.spawns != nil {
		if _, dead, known := s.spawns.memberRow(id); known && dead {
			return false
		}
	}

	return true
}

// look is one row's look: a fighter keeps its target; otherwise the nearest
// living quarry the watcher can see within reach takes the watch, and with
// none in view the watch is kept as it was.
func (s *Seek) look(id string, row *seekRow, living []Quarry) {
	w := s.notice.watches[id]

	if s.combat.engaged(id) {
		row.target, row.reason = s.ref(w.target), SeekFighting

		return
	}

	s.looks++

	chosen, considered := s.nearest(w, living)
	row.candidates = considered

	if chosen == nil {
		row.target, row.reason = s.ref(w.target), SeekNone

		return
	}

	if chosen.QuarryID() != w.target.QuarryID() {
		s.notice.Retarget(id, chosen)
		s.retargets++
	}

	row.target, row.reason = s.ref(chosen), SeekLiving
}

// seekCandidate is one living quarry as a look weighs it.
type seekCandidate struct {
	q       Quarry
	id      string
	x, y, d float64
	current bool
}

// nearest is the nearest quarry w's watcher can see within Notice's reach, or
// nil, and how many were within reach. Candidates are taken nearest first
// (the watch's own target first among equals, then by id), and the first
// clear line ends the search: the nearest in sight is the nearest.
func (s *Seek) nearest(w *watch, living []Quarry) (Quarry, int) {
	if !s.notice.Wired() {
		return nil, 0
	}

	wid := w.watcher.WatcherID()
	wx, wy := w.watcher.WatcherAt()

	cands := make([]seekCandidate, 0, len(living)+1)
	seen := map[string]bool{}

	add := func(q Quarry, current bool) {
		if q == nil {
			return
		}

		id := q.QuarryID()
		if id == "" || id == wid || seen[id] || !s.eligible(id) {
			return
		}

		seen[id] = true
		x, y := q.QuarryAt()
		cands = append(cands, seekCandidate{q: q, id: id, x: x, y: y, d: distance(wx, wy, x, y), current: current})
	}

	add(w.target, true)

	for _, q := range living {
		add(q, false)
	}

	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.d != b.d {
			return a.d < b.d
		}

		if a.current != b.current {
			return a.current
		}

		return a.id < b.id
	})

	considered := 0

	var chosen Quarry

	for _, c := range cands {
		if _, reach := s.notice.reachAt(c.x, c.y); c.d > reach {
			continue
		}

		considered++

		if chosen != nil {
			continue
		}

		s.rays++

		if s.notice.sight.Clear(wx, wy, c.x, c.y) {
			chosen = c.q
		}
	}

	return chosen, considered
}

// eligible is the four gates every candidate passes but the first (not the
// watcher itself, which nearest checks): not protected, not hidden, not dead
// by its body.
func (s *Seek) eligible(id string) bool {
	if s.notice.hides(id) {
		return false
	}

	if s.combat != nil {
		if s.combat.protected != nil && s.combat.protected(id) {
			return false
		}

		if s.combat.deadByBody(id) {
			return false
		}
	}

	return true
}

// living is every quarry the game and the scripts name, stand-ins resolved
// now: a stand-in that no longer resolves (taken off the map) is simply not
// there, as a villager who has gone inside is not.
func (s *Seek) living() []Quarry {
	var out []Quarry

	if s.quarries != nil {
		out = append(out, s.quarries()...)
	}

	if s.resolver != nil {
		for _, id := range s.standIns {
			if q, ok := s.resolver.Quarry(id); ok && q != nil && q.QuarryID() == id {
				out = append(out, q)
			}
		}
	}

	return out
}

// ref writes a quarry as a snapshot writes one: PlayerRef for him.
func (s *Seek) ref(q Quarry) string {
	if q == nil {
		return ""
	}

	id := q.QuarryID()

	if s.resolver != nil {
		if p, ok := s.resolver.Quarry(PlayerRef); ok && p != nil && p.QuarryID() == id {
			return PlayerRef
		}
	}

	return id
}

// rowIDs returns the rows' watcher ids in a stable order.
func (s *Seek) rowIDs() []string {
	ids := make([]string, 0, len(s.rows))
	for id := range s.rows {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}

// addStandIn names an entity living (a script's stand-in villager).
func (s *Seek) addStandIn(id string) error {
	if id == "" || id == PlayerRef {
		return fmt.Errorf("a stand-in is an entity id, not %q", id)
	}

	for _, have := range s.standIns {
		if have == id {
			return fmt.Errorf("%q is a stand-in already", id)
		}
	}

	s.standIns = append(s.standIns, id)

	return nil
}

// removeStandIn takes a stand-in back out (the first provider rule).
func (s *Seek) removeStandIn(id string) error {
	for i, have := range s.standIns {
		if have == id {
			s.standIns = append(s.standIns[:i:i], s.standIns[i+1:]...)

			return nil
		}
	}

	return fmt.Errorf("%q is not a stand-in", id)
}

// HarnessName names the provider.
func (s *Seek) HarnessName() string { return "seek" }

// HarnessState reports every row, in watcher order, and the totals. Every
// list is ordered: an assertion that reads element 0 must read the same one
// on every run.
func (s *Seek) HarnessState() map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(s.rows))

	for _, id := range s.rowIDs() {
		row := s.rows[id]

		spawnRow := ""
		if s.spawns != nil {
			spawnRow, _, _ = s.spawns.memberRow(id)
		}

		rows = append(rows, map[string]interface{}{
			"watcher":            id,
			"row":                spawnRow,
			"target":             row.target,
			"reason":             row.reason,
			"until_look_minutes": row.untilLook,
			"candidates":         row.candidates,
		})
	}

	return map[string]interface{}{
		"rows":         rows,
		"stand_ins":    append([]string{}, s.standIns...),
		"slots":        s.slots,
		"looks":        s.looks,
		"retargets":    s.retargets,
		"rays":         s.rays,
		"route_solves": 0,
		"wired":        s.notice != nil && s.notice.Wired(),
		"dials": map[string]interface{}{
			"retarget_minutes": s.dials.RetargetMinutes,
			"stagger_slots":    s.dials.StaggerSlots,
		},
	}
}

// HarnessSettableFields lists the writes the system allows: its two dials,
// and the stand-in collection's two verbs.
func (s *Seek) HarnessSettableFields() []string {
	return []string{"retarget_minutes", "stagger_slots", "stand_in", "stand_in_remove"}
}

// HarnessSet applies one write.
func (s *Seek) HarnessSet(field string, value interface{}) error {
	switch field {
	case "retarget_minutes":
		v, ok := value.(float64)
		if !ok || !(v > 0) || math.IsInf(v, 0) {
			return fmt.Errorf("retarget_minutes wants a positive number of world minutes, got %v", value)
		}

		s.dials.RetargetMinutes = v
	case "stagger_slots":
		v, ok := value.(float64)
		if !ok || v < 1 || v != math.Trunc(v) || v > 1440 {
			return fmt.Errorf("stagger_slots wants a whole number from 1 to 1440, got %v", value)
		}

		s.dials.StaggerSlots = int(v)
	case "stand_in":
		id, ok := value.(string)
		if !ok {
			return fmt.Errorf("stand_in wants an entity id, got %v", value)
		}

		return s.addStandIn(id)
	case "stand_in_remove":
		id, ok := value.(string)
		if !ok {
			return fmt.Errorf("stand_in_remove wants an entity id, got %v", value)
		}

		return s.removeStandIn(id)
	default:
		return fmt.Errorf("seek has no settable field %q", field)
	}

	return nil
}
