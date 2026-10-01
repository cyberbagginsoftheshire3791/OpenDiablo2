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
// A SEEN TARGET IS STICKY (the R2 review's B1, 1 Oct 2026). "The nearest" with
// no hysteresis turned a hunter between two near-equal quarries on every look
// (0.4 to 0.8 s of real time at night), and the chase followed it in the same
// frame. So a watch whose own target is still in reach and in sight keeps it
// (reason "held") unless the nearest is nearer by SwitchMarginTiles, and for
// DwellMinutes after a switch it keeps the new target while it is seen at all.
// A target that is out of sight, out of reach, hidden, protected or dead is
// no reason to wait: the nearest in sight takes the watch at once.
//
// SEEK SOLVES NO ROUTE; EACH RETARGET COSTS ONE PURSUIT SOLVE. Seek itself
// casts rays only (D-S2 = 0). But the game restarts the chase on a moved
// watch in the same frame (startChasesForTheAware), and Pursuit.Chase solves
// at once: one A* per retarget, counted by Pursuit (rechase_solves) and
// measured, quarries moving, by the benchmark beside this file (the review's
// B2).
//
// THE STAGGER, BUILT FROM THE START (S0's M0.2): each watcher looks every
// RetargetMinutes on a phase of its own -- the minute cut into StaggerSlots
// phases -- so a pack that arrives in one frame looks across the minute's
// night frames, not all in one. A new row takes the LEAST-LOADED phase of the
// rows already looking, counted on the absolute frames they fall due (the
// review's C1: round-robin by creation count bunched two groups made a few
// frames apart). At 4x the brief's N and M the unstaggered frame is over
// budget (3.4 ms); the benchmark beside this file measures both.
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

	// slots counts the stagger phases given out (one per row made).
	slots int

	// looks counts the looks taken, retargets the watches moved, holds the
	// looks that kept a seen target over a nearer one (the margin or the
	// dwell), rays the sight tests the looks cast.
	looks, retargets, holds, rays int
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

	// dwell is the world minutes left in which the row keeps a target it
	// switched to while that target is seen (DwellMinutes at a switch, then
	// counting down to zero).
	dwell float64
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
	// SeekHeld is a row that saw a nearer quarry at its last look and kept
	// its own target, still in reach and in sight: the nearer was not nearer
	// by the margin, or the row was inside its dwell (the review's B1).
	SeekHeld = "held"
)

func validSeekReason(r string) bool {
	return r == SeekPending || r == SeekLiving || r == SeekNone || r == SeekFighting || r == SeekHeld
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
	// 24, the night frames of one world minute. Each new row takes the
	// least-loaded phase, so while the minute's frames are 1/24 minute (the
	// night) at most ceil(N/24) of N watchers look in any night frame.
	StaggerSlots int

	// SwitchMarginTiles is how much nearer another quarry must be before a
	// watch leaves its own target while that target is in reach and in
	// sight: 1.5, Pursuit's RepathTiles (the review's B1). 0 is the strict
	// nearest R2 first shipped.
	SwitchMarginTiles float64

	// DwellMinutes is how long after a switch the row keeps its new target
	// while it is seen, whatever comes nearer: 2.0, Pursuit's
	// MinRepathMinutes (the review's B1). 0 is no dwell.
	DwellMinutes float64
}

// DefaultSeekDials returns the brief's starting values (§5, D-S1) and the
// review's stickiness (B1).
func DefaultSeekDials() SeekDials {
	return SeekDials{RetargetMinutes: 1.0, StaggerSlots: 24, SwitchMarginTiles: 1.5, DwellMinutes: 2.0}
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

	r := s.dials.RetargetMinutes
	if r <= 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		return
	}

	fresh := s.sync(worldMinutes)

	if len(s.rows) == 0 {
		return
	}

	var living []Quarry

	built := false

	for _, id := range s.rowIDs() {
		row := s.rows[id]

		// A row made this step does not age in it: its phase counts from here.
		if !fresh[id] {
			row.untilLook -= worldMinutes

			if row.dwell > 0 {
				row.dwell = math.Max(0, row.dwell-worldMinutes)
			}
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
// least-loaded stagger phase, in watcher-id order -- and drops the row of
// every watcher that no longer watches, or is no longer served. It returns
// the rows it made. step is the world minutes this Advance is about to age
// the rows that were already there by.
func (s *Seek) sync(step float64) map[string]bool {
	for id := range s.rows {
		if !s.served(id) {
			delete(s.rows, id)
		}
	}

	var (
		fresh map[string]bool
		load  []int
	)

	for _, id := range s.notice.watcherIDs() {
		if _, ok := s.rows[id]; ok || !s.served(id) {
			continue
		}

		if load == nil {
			load = s.phaseLoad(step)
		}

		k := leastLoaded(load)
		load[k]++

		row := &seekRow{
			target:    s.ref(s.notice.watches[id].target),
			reason:    SeekPending,
			untilLook: s.phase(k),
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

// staggerSlots is the dial, floored at one phase.
func (s *Seek) staggerSlots() int {
	if s.dials.StaggerSlots < 1 {
		return 1
	}

	return s.dials.StaggerSlots
}

// phase is the untilLook of stagger phase k: the minute cut into StaggerSlots.
// Phase 0 looks at once.
func (s *Seek) phase(k int) float64 {
	slots := s.staggerSlots()

	return s.dials.RetargetMinutes * float64(k%slots) / float64(slots)
}

// phaseLoad counts the rows already looking on each phase, as a new row made
// in this step would count it: an existing row is aged by step before it
// looks, and a new one is not, so an existing row's phase is its untilLook
// less step, rounded to the nearest phase (the review's C1: phases are
// absolute frames, not creation counts).
func (s *Seek) phaseLoad(step float64) []int {
	slots := s.staggerSlots()
	load := make([]int, slots)
	r := s.dials.RetargetMinutes
	width := r / float64(slots)

	for _, id := range s.rowIDs() {
		u := math.Mod(s.rows[id].untilLook-step, r)
		if u < 0 {
			u += r
		}

		load[int(math.Round(u/width))%slots]++
	}

	return load
}

// leastLoaded is the phase with the fewest rows, the earliest among equals.
func leastLoaded(load []int) int {
	best := 0

	for k := range load {
		if load[k] < load[best] {
			best = k
		}
	}

	return best
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
// living quarry the watcher can see within reach takes the watch -- unless
// the watch's own target is still in reach and in sight and the nearest is
// not nearer by the margin, or the row is inside its dwell (held) -- and with
// none in view the watch is kept as it was.
func (s *Seek) look(id string, row *seekRow, living []Quarry) {
	w := s.notice.watches[id]

	if s.combat.engaged(id) {
		row.target, row.reason = s.ref(w.target), SeekFighting

		return
	}

	s.looks++

	lk := s.nearest(w, living)
	row.candidates = lk.considered

	if lk.chosen == nil {
		row.target, row.reason = s.ref(w.target), SeekNone

		return
	}

	if lk.chosen.current {
		row.target, row.reason = s.ref(lk.chosen.q), SeekLiving

		return
	}

	if s.keepsOwn(w, row, lk) {
		s.holds++
		row.target, row.reason = s.ref(w.target), SeekHeld

		return
	}

	s.notice.Retarget(id, lk.chosen.q)
	s.retargets++

	row.dwell = s.dwellDial()
	row.target, row.reason = s.ref(lk.chosen.q), SeekLiving
}

// keepsOwn reports that the watch keeps its own target over the nearer chosen:
// its target is a candidate in reach, the nearer is not nearer by the margin
// or the row is inside its dwell, and its target is in sight. The sight test
// is cast only when it decides something (a target nearer than the chosen
// was cast already, and failed).
func (s *Seek) keepsOwn(w *watch, row *seekRow, lk seekLook) bool {
	cur := lk.current
	if cur == nil {
		return false
	}

	if row.dwell <= 0 && lk.chosen.d <= cur.d-s.marginDial() {
		return false
	}

	if lk.currentCast {
		return lk.currentSeen
	}

	s.rays++

	wx, wy := w.watcher.WatcherAt()

	return s.notice.sight.Clear(wx, wy, cur.x, cur.y)
}

// marginDial and dwellDial are the stickiness dials, a bad value read as 0.
func (s *Seek) marginDial() float64 {
	if m := s.dials.SwitchMarginTiles; m > 0 && !math.IsInf(m, 0) {
		return m
	}

	return 0
}

func (s *Seek) dwellDial() float64 {
	if d := s.dials.DwellMinutes; d > 0 && !math.IsInf(d, 0) {
		return d
	}

	return 0
}

// seekCandidate is one living quarry as a look weighs it.
type seekCandidate struct {
	q       Quarry
	id      string
	x, y, d float64
	current bool
}

// seekLook is what one look found: the nearest candidate in sight (nil for
// none), the watch's own target when it is a candidate within reach, whether
// its line was cast and came out clear, and how many were within reach.
type seekLook struct {
	chosen, current          *seekCandidate
	currentCast, currentSeen bool
	considered               int
}

// nearest finds the nearest quarry w's watcher can see within Notice's reach,
// and how many were within reach. Candidates are taken nearest first (the
// watch's own target first among equals, then by id), and the first clear
// line ends the search: the nearest in sight is the nearest.
func (s *Seek) nearest(w *watch, living []Quarry) seekLook {
	var lk seekLook

	if !s.notice.Wired() {
		return lk
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

	for i := range cands {
		c := &cands[i]

		if _, reach := s.notice.reachAt(c.x, c.y); c.d > reach {
			continue
		}

		lk.considered++

		if c.current {
			lk.current = c
		}

		if lk.chosen != nil {
			continue
		}

		s.rays++

		clear := s.notice.sight.Clear(wx, wy, c.x, c.y)

		if c.current {
			lk.currentCast, lk.currentSeen = true, clear
		}

		if clear {
			lk.chosen = c
		}
	}

	return lk
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
			"dwell_minutes":      row.dwell,
		})
	}

	return map[string]interface{}{
		"rows":      rows,
		"stand_ins": append([]string{}, s.standIns...),
		"slots":     s.slots,
		"looks":     s.looks,
		"retargets": s.retargets,
		"holds":     s.holds,
		"rays":      s.rays,
		// Seek's own route solves: always 0, it casts rays only. Each
		// retarget costs the chase one solve, which Pursuit counts
		// (rechase_solves).
		"route_solves": 0,
		"wired":        s.notice != nil && s.notice.Wired(),
		"dials": map[string]interface{}{
			"retarget_minutes":    s.dials.RetargetMinutes,
			"stagger_slots":       s.dials.StaggerSlots,
			"switch_margin_tiles": s.dials.SwitchMarginTiles,
			"dwell_minutes":       s.dials.DwellMinutes,
		},
	}
}

// HarnessSettableFields lists the writes the system allows: its four dials,
// and the stand-in collection's two verbs.
func (s *Seek) HarnessSettableFields() []string {
	return []string{"dwell_minutes", "retarget_minutes", "stagger_slots", "stand_in", "stand_in_remove", "switch_margin_tiles"}
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
	case "switch_margin_tiles", "dwell_minutes":
		v, ok := value.(float64)
		if !ok || !(v >= 0) || math.IsInf(v, 0) {
			return fmt.Errorf("%s wants a number from 0 up, got %v", field, value)
		}

		if field == "dwell_minutes" {
			s.dials.DwellMinutes = v
		} else {
			s.dials.SwitchMarginTiles = v
		}
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
