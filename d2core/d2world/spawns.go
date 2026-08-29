package d2world

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// Spawns is M4.3b's spine: the hostile stage tables, keyed to the clock.
//
// It decides WHAT arrives, WHEN, and HOW MANY. It does not decide WHERE in any
// detail and it does not create anything, because d2world does not know what a
// map or an entity is -- it asks a Spawner for "count of <code>, somewhere
// between min and max tiles from here" and gets back things that can say where
// they are. It then hands each one to Notice, which decides whether it has
// seen you, and the game screen hands the aware ones to Pursuit. The three
// systems compose in that order and none of them knows about the others'
// internals.
//
// WHAT IT OWNS, by signature (M4.3b, 28 Aug):
//   - the two stage tables, beasts and humans, weighted by the clock
//   - the [3] deep-night bands, computed HERE and not in the clock, so the
//     clock stays the calendar rather than the difficulty curve (ask 4)
//   - the morale STATE -- morale and routing -- which M4.5's resolver reads.
//     The rout BEHAVIOUR is M4.5's; nothing here makes a pack run (ask 2)
//   - open_bodies, reported and harness-settable, so N1's carrion weighting is
//     exercised and assertable before the corpse machine exists (ask 3)
//
// WHAT IT DOES NOT OWN: the corpse state machine and the rising roll, split
// into their own milestone shared with M4.6; combat of any kind; quick-resolve;
// the human row's kit, which waits on E3.
//
// Not safe for concurrent use; it lives on the game goroutine.
type Spawns struct {
	dials  SpawnDials
	clock  *Clock
	notice *Notice
	illum  Illumination

	spawner Spawner
	target  Quarry

	rng *rand.Rand

	groups  map[string]*group
	nextID  int
	sinceCk float64 // world minutes since the last table check

	openBodies int

	checks    int
	rolls     int
	spawned   int
	failures  int
	despawned int // members taken back out of the world
	cleared   int // groups sent home at daybreak
}

// Spawner is the only thing Spawns needs from the world outside: make count
// creatures of a monstats code somewhere in a ring around a point, and hand
// back things that can say where they are.
//
// It returns Watchers rather than ids because what a freshly spawned thing is
// FOR, in this milestone, is noticing you -- so the shape the game screen
// builds is the shape Notice wants, and no second adapter is needed.
//
// A Spawner that cannot place anything returns nil, and that is a counted
// failure rather than a panic: an unknown monstats code must be visible in the
// provider, not fatal in the field.
type Spawner interface {
	Spawn(code string, count int, aroundX, aroundY, minTiles, maxTiles float64) []Watcher

	// Despawn takes the members back out of the world.
	//
	// It arrived a milestone late, and the gap is worth naming. M4.3b shipped
	// with only the filling half: Spawns.Despawn dropped the bookkeeping and
	// left the creatures standing on the map, and nothing in a shipped build
	// called even that. THE FIRST PROVIDER RULE APPLIES TO THE WORLD OUTSIDE
	// THE MODEL, not only to the model's own collection.
	Despawn(members []Watcher)
}

// SpawnRow is one line of a stage table. Every number is a [DIAL].
//
// THE CODES ARE D2 STAND-INS AND SAYING SO MATTERS. N1 §5 signs the roster as
// feral dogs, wolves, wild boar (bear authored-only, lynx not an enemy), but
// this build has no art of its own and plan §5 Phase 4 says stand-ins
// throughout. Mapping N1's roster onto real monstats ids is content work for
// a session that has the MPQs open; it is not this milestone, and the Code
// field is settable so that session does not have to touch this file.
type SpawnRow struct {
	// Name is the design's name for the row and never changes with the code.
	Name string

	// Code is the monstats.txt Id actually spawned -- a stand-in [DIAL].
	Code string

	// StageWeight is the row's weight in each stage, indexed by Stage. Zero
	// means the row never fires then. An ARRAY rather than a map on purpose:
	// ranging a map is randomised in Go and these weights decide entity
	// creation, which is inside the state digest.
	StageWeight [4]float64

	// BandWeight multiplies StageWeight inside the deep night, by band. S1 §4
	// subdivides the night into [3] bands with rising weights, which is what
	// makes 2 a.m. worse than 11 p.m. even on an unauthored night.
	BandWeight [3]float64

	// MinCount/MaxCount is the pack size, inclusive (N1 §5: dogs 2-4,
	// wolves 3-6).
	MinCount, MaxCount int

	// MinTiles/MaxTiles is the ring it arrives in, in world tiles from the
	// target. Wolves come "from the woods" and dogs come "near bodies and
	// roads"; without a corpse machine or a road model, distance is the only
	// honest expression of that difference in v0.
	MinTiles, MaxTiles float64

	// Carrion weights the row by open bodies within radius (N1's own sharper
	// assertion). Beast rows only.
	Carrion bool

	// LightDrawn weights the row by how lit the target is. S1 §6.4 weights the
	// human row by "the player's visible wealth and light"; there is no wealth
	// to read until Phase 6's inventory, so v0 carries the light half and
	// names the other as deferred rather than pretending to model it.
	LightDrawn bool

	// Morale is what a group of this row starts at (R2 §3's test-and-rout).
	Morale float64
}

// SpawnDials are the numbers M4.3b ships with. Every one is a [DIAL].
//
// EVERY RATE IS IN WORLD MINUTES AND CARRIES ITS FRAME COST, for the reason
// spelled out on NoticeDials: the clock compresses and DAY is the tight case
// (DayRate 4.0 against NightRate 2.5), so a world-minute budget buys FEWER
// frames in daylight. At the harness's default tick of 1/60 s, one world
// minute is ~15 stepped frames by day and ~24 at night.
type SpawnDials struct {
	// CheckMinutes floors how often the tables are consulted. ~75 stepped
	// frames by day, ~120 at night. Deliberately looser than the notice
	// re-evaluation: arriving late is atmosphere, noticing late is a bug.
	CheckMinutes float64

	// Chance scales every row's weight into a probability per check. A script
	// sets it to a large number to force the next eligible row to fire, which
	// is how the tables are testable without waiting out a night.
	Chance float64

	// CarrionRadius is how far an open body still draws beasts, in world
	// tiles, and CarrionPerBody is the weight each one adds. CarrionCap
	// bounds the product, so a field of bodies is dangerous rather than
	// arithmetically absurd.
	CarrionRadius  float64
	CarrionPerBody float64
	CarrionCap     float64

	// LightWeight is how much a fully lit target multiplies a light-drawn
	// row. The other half of R2 §3's dark-into-light trade: a torch does not
	// only make you easier to SEE, it makes you worth coming to look at.
	LightWeight float64

	// RoutAt is the morale at or below which a group reports routing. The
	// STATE lives here; the behaviour is M4.5's.
	RoutAt float64

	// MaxGroups bounds how many live groups the tables will create, so a
	// forced-chance script cannot fill the map.
	MaxGroups int

	// Rows are the tables themselves.
	Rows []SpawnRow
}

// DefaultSpawnDials returns the signed §5 starting values, with N1 §5's roster
// wearing D2 stand-in codes (see SpawnRow.Code).
func DefaultSpawnDials() SpawnDials {
	return SpawnDials{
		CheckMinutes:   5.0,
		Chance:         0.35,
		CarrionRadius:  8,
		CarrionPerBody: 0.25,
		CarrionCap:     3,
		LightWeight:    2,
		RoutAt:         25,
		MaxGroups:      8,
		Rows: []SpawnRow{
			{
				// N1 §5: dusk into night, near bodies and roads, drawn by
				// carrion and noise. The nearest row, and the first thing a
				// player meets.
				Name: "dogs", Code: "fallen1",
				StageWeight: stageWeights(0.6, 0, 0, 1.0),
				BandWeight:  [3]float64{1.0, 0.8, 0.6},
				MinCount:    2, MaxCount: 4,
				MinTiles: 8, MaxTiles: 16,
				Carrion: true,
				Morale:  50,
			},
			{
				// N1 §5: deep night, from the woods, bolder as the night's
				// bands advance -- which is the band curve running the other
				// way from the dogs'.
				Name: "wolves", Code: "zombie1",
				StageWeight: stageWeights(1.0, 0, 0, 0),
				BandWeight:  [3]float64{0.5, 1.0, 1.5},
				MinCount:    3, MaxCount: 6,
				MinTiles: 14, MaxTiles: 24,
				Carrion: true,
				Morale:  60,
			},
			{
				// N1 §5: dawn and dusk, field and wood edges. Solitary or a
				// small sounder; the daylight row, so the night is not the
				// only time something happens.
				Name: "boar", Code: "skeleton1",
				StageWeight: stageWeights(0, 0.5, 0, 0.5),
				BandWeight:  [3]float64{1, 1, 1},
				MinCount:    1, MaxCount: 2,
				MinTiles: 10, MaxTiles: 18,
				Morale: 70,
			},
			{
				// S1 §6.4's human row: dusk and pre-dawn, "when men move",
				// weighted by the player's light. The kit waits on E3; the
				// mechanism is here now.
				Name: "opportunists", Code: "fallen1",
				StageWeight: stageWeights(0.4, 0.6, 0, 0.8),
				BandWeight:  [3]float64{0.8, 0.6, 0.4},
				MinCount:    2, MaxCount: 3,
				MinTiles: 12, MaxTiles: 20,
				LightDrawn: true,
				Morale:     40,
			},
		},
	}
}

// stageWeights builds the per-stage array in the order the Stage constants
// declare (night, dawn, day, dusk), so a row reads in the order a day happens
// rather than in the order iota does.
func stageWeights(night, dawn, day, dusk float64) [4]float64 {
	var w [4]float64
	w[StageNight] = night
	w[StageDawn] = dawn
	w[StageDay] = day
	w[StageDusk] = dusk

	return w
}

// group is one arrival: the members, and the morale state M4.5 will read.
type group struct {
	id      string
	row     string
	code    string
	members []Watcher

	morale  float64
	bornAt  float64 // world minutes on the clock when it arrived
	band    int
	stage   Stage
	weight  float64
	spawned int
}

// NewSpawns builds the tables and registers the "spawns" harness provider.
//
// seed is the run's seed: pack sizes and the table rolls are drawn from an RNG
// of this system's own, seeded here, so two launches of one build at one seed
// produce the same arrivals. Every draw below happens in a fixed order for the
// same reason the A* never ranges a map.
func NewSpawns(clock *Clock, notice *Notice, spawner Spawner, illum Illumination,
	seed int64, dials SpawnDials) *Spawns {
	s := &Spawns{
		dials:   dials,
		clock:   clock,
		notice:  notice,
		illum:   illum,
		spawner: spawner,
		rng:     rand.New(rand.NewSource(seed)), // nolint:gosec // gameplay RNG, seeded for reproducibility
		groups:  make(map[string]*group),
		nextID:  1,
	}

	d2harness.Register(s)

	return s
}

// Close unregisters the provider.
func (s *Spawns) Close() { d2harness.Unregister(s) }

// SetTarget names the thing the tables spawn around and the thing every
// spawned watcher watches. In the slice that is the player.
func (s *Spawns) SetTarget(q Quarry) { s.target = q }

// Band is which third of the deep night the clock is in: 0, 1, 2 as the night
// deepens, and -1 when it is not night at all.
//
// It is computed HERE rather than on the clock by signature (ask 4). The clock
// is the calendar; the difficulty curve belongs to the thing that makes the
// difficulty. Night wraps midnight, so the arithmetic is done in minutes since
// NightStart modulo a day.
func (s *Spawns) Band() int {
	if s.clock == nil || s.clock.Stage() != StageNight {
		return -1
	}

	d := s.clock.dials

	length := d.DawnStart - d.NightStart
	if length <= 0 {
		length += minutesPerDay
	}

	since := s.clock.MinuteOfDay() - d.NightStart
	if since < 0 {
		since += minutesPerDay
	}

	bands := len(SpawnRow{}.BandWeight)

	band := int(float64(bands) * since / length)
	if band >= bands {
		band = bands - 1
	}

	if band < 0 {
		band = 0
	}

	return band
}

// CarrionWeight is the multiplier open bodies put on the beast rows: 1 with
// none, rising by CarrionPerBody each, capped at CarrionCap.
//
// The radius dial is reported and honoured in shape but every body counts in
// v0, because there is nowhere to PUT a body until the corpse machine exists
// -- open_bodies is a count the harness sets, not a set of positions. That is
// the deliberate half-measure ask 3 signs: the weighting is exercised and
// assertable now, and the corpse machine drives the same field later.
func (s *Spawns) CarrionWeight() float64 {
	w := 1 + s.dials.CarrionPerBody*float64(s.openBodies)

	return math.Min(w, s.dials.CarrionCap)
}

// LightWeight is the multiplier the target's own light puts on the rows drawn
// to it. 1 in the dark, rising to LightWeight when the target is fully lit.
func (s *Spawns) LightWeight() float64 {
	if s.illum == nil || s.target == nil {
		return 1
	}

	x, y := s.target.QuarryAt()
	level := s.illum.Level(int(math.Floor(x)), int(math.Floor(y)))

	return 1 + (s.dials.LightWeight-1)*clamp01(level)
}

// Weight is one row's current weight: its stage weight, times its band weight
// inside the deep night, times whichever of the carrion and light multipliers
// it is subject to. Zero means the row cannot fire now.
func (s *Spawns) Weight(row SpawnRow) float64 {
	if s.clock == nil {
		return 0
	}

	stage := s.clock.Stage()

	w := row.StageWeight[stage]
	if w <= 0 {
		return 0
	}

	if band := s.Band(); band >= 0 {
		w *= row.BandWeight[band]
	}

	if row.Carrion {
		w *= s.CarrionWeight()
	}

	if row.LightDrawn {
		w *= s.LightWeight()
	}

	return w
}

// Advance runs the tables on the world minutes that just passed, then steps
// the notice model, so a group spawned this tick is evaluated this tick rather
// than standing blind until the next one.
func (s *Spawns) Advance(worldMinutes float64) {
	if worldMinutes <= 0 {
		return
	}

	s.sinceCk += worldMinutes

	if s.sinceCk >= s.dials.CheckMinutes {
		s.sinceCk = 0

		s.check()
	}

	if s.notice != nil {
		s.notice.Advance(worldMinutes)
	}

	// After the notice model, so a group that noticed the player on this very
	// tick is not sent home a moment later.
	s.clearAtDaybreak()
}

// clearAtDaybreak sends home every group that has not noticed the player,
// once the sun is properly up.
//
// M4.3b shipped without it, and the omission was worse than it looks. Nothing
// in a shipped build called Despawn at all -- its only caller was HarnessSet
// -- and check() refuses to spawn once len(groups) reaches MaxGroups (8). So
// the eighth pack was the last one the game would ever produce: not a leak,
// a permanent SPAWN STALL, with the night simply ceasing to happen for the
// life of the screen. Found by the reachability register on its first run and
// ruled a burst of its own by Josh, 28 Aug.
//
// StageDay rather than !Stage().IsDark(), and the difference is not cosmetic:
// IsDark() is true for StageNight ALONE (clock.go:52), so "not dark" would
// include dusk -- and the dogs row carries a 0.6 dusk weight, so a pack could
// be cleared on the tick after it arrived. Daylight is the rule; the dark
// stages either side of it are when things are out.
//
// A group that has noticed the player is left alone. Daylight is a reason to
// go home, not a reason to forget the thing already coming for you, and what
// happens to a pack that catches you at dawn is M4.5's.
func (s *Spawns) clearAtDaybreak() {
	if s.clock == nil || s.clock.Stage() != StageDay || len(s.groups) == 0 {
		return
	}

	// groupIDs returns a sorted snapshot, so deleting inside the loop is safe
	// and the order is the same on every launch.
	for _, id := range s.groupIDs() {
		if s.aware(id) {
			continue
		}

		if s.Despawn(id) {
			s.cleared++
		}
	}
}

// aware reports whether any member of a group has noticed the player.
func (s *Spawns) aware(groupID string) bool {
	g, ok := s.groups[groupID]
	if !ok || s.notice == nil {
		return false
	}

	for _, m := range g.members {
		if noticed, watching := s.notice.Noticed(m.WatcherID()); watching && noticed {
			return true
		}
	}

	return false
}

// check consults every row once, in declaration order.
func (s *Spawns) check() {
	s.checks++

	if s.spawner == nil || s.target == nil {
		return
	}

	for i := range s.dials.Rows {
		row := s.dials.Rows[i]

		weight := s.Weight(row)
		if weight <= 0 {
			continue
		}

		// The roll happens for every eligible row whether or not the group cap
		// allows a spawn, so that the RNG's draw sequence does not depend on
		// how many groups happen to be alive -- otherwise despawning a group
		// would change every later arrival, and the two-launch determinism
		// proof would fail for a reason nobody could find.
		roll := s.rng.Float64()
		s.rolls++

		if roll >= clamp01(weight*s.dials.Chance) {
			continue
		}

		if len(s.groups) >= s.dials.MaxGroups {
			continue
		}

		s.spawn(row, weight)
	}
}

// spawn creates one group from one row.
func (s *Spawns) spawn(row SpawnRow, weight float64) {
	count := row.MinCount
	if row.MaxCount > row.MinCount {
		count += s.rng.Intn(row.MaxCount - row.MinCount + 1)
	}

	tx, ty := s.target.QuarryAt()

	members := s.spawner.Spawn(row.Code, count, tx, ty, row.MinTiles, row.MaxTiles)
	if len(members) == 0 {
		// An unknown monstats code or a map with nowhere to put them. Counted
		// and reported rather than fatal: the provider is where a bad stand-in
		// code should show up, not a crash in the field.
		s.failures++

		return
	}

	g := &group{
		id:      fmt.Sprintf("g:%d", s.nextID),
		row:     row.Name,
		code:    row.Code,
		members: members,
		morale:  row.Morale,
		band:    s.Band(),
		weight:  weight,
		spawned: len(members),
	}

	if s.clock != nil {
		g.bornAt = s.clock.WorldMinutes()
		g.stage = s.clock.Stage()
	}

	s.nextID++
	s.groups[g.id] = g
	s.spawned += len(members)

	if s.notice != nil {
		for _, m := range members {
			s.notice.Watch(m, s.target)
		}
	}
}

// Despawn drops a group and stops watching its members. The first provider
// rule: a collection needs a verb that fills it and one that empties it.
func (s *Spawns) Despawn(groupID string) bool {
	g, ok := s.groups[groupID]
	if !ok {
		return false
	}

	// The world first, then the bookkeeping. Before 29 Aug this method did
	// only the second half, so even a harness despawn left the creatures
	// standing on the map with nothing watching them.
	if s.spawner != nil {
		s.spawner.Despawn(g.members)
	}

	if s.notice != nil {
		for _, m := range g.members {
			s.notice.Unwatch(m.WatcherID())
		}
	}

	delete(s.groups, groupID)

	s.despawned += len(g.members)

	return true
}

// SetMorale writes one group's morale. It is a WRITE rather than a decrement
// because nothing in M4.3b can hurt a pack -- there is no combat until M4.5,
// and this is the state M4.5's resolver will drive. Until then the harness is
// the only thing that moves it, which is exactly the third provider rule: a
// reported value needs a verb that moves it in both directions, or a script
// can only ever watch the number sit still.
func (s *Spawns) SetMorale(groupID string, morale float64) bool {
	g, ok := s.groups[groupID]
	if !ok {
		return false
	}

	g.morale = morale

	return true
}

// Routing reports whether a group is at or below the rout threshold. The
// STATE is this milestone's; what a routing pack DOES is M4.5's.
func (s *Spawns) Routing(groupID string) (routing, known bool) {
	g, ok := s.groups[groupID]
	if !ok {
		return false, false
	}

	return g.morale <= s.dials.RoutAt, true
}

// Groups is how many groups are live.
func (s *Spawns) Groups() int { return len(s.groups) }

// OpenBodies is the count the carrion weighting reads.
func (s *Spawns) OpenBodies() int { return s.openBodies }

// SetOpenBodies writes it. Negative counts are refused.
func (s *Spawns) SetOpenBodies(n int) bool {
	if n < 0 {
		return false
	}

	s.openBodies = n

	return true
}

// groupIDs returns the live group ids in a stable order. "g:2" must sort
// before "g:10", so this sorts on the numeric suffix rather than the string.
func (s *Spawns) groupIDs() []string {
	ids := make([]string, 0, len(s.groups))
	for id := range s.groups {
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		return groupOrdinal(ids[i]) < groupOrdinal(ids[j])
	})

	return ids
}

func groupOrdinal(id string) int {
	var n int
	if _, err := fmt.Sscanf(id, "g:%d", &n); err != nil {
		return math.MaxInt32
	}

	return n
}

// ------------------------------------------------------------ harness ------

// HarnessName identifies the provider (P3 §3.5).
func (s *Spawns) HarnessName() string { return "spawns" }

// HarnessState reports the tables, the groups, and the notice block for every
// member of every group.
//
// THE NOTICE BLOCK IS HERE RATHER THAN IN A PROVIDER OF ITS OWN because that
// is what M4.3b ask 6 signs, and the ask exists because M4.3a's §3.2 was
// signed with an assertion nothing in the harness could write. A chase that
// starts is not evidence that noticing works -- a chase can start for the
// wrong reason and look identical from outside -- so the inputs (sees,
// distance, light_at_quarry) are reported beside the verdict (noticed), and a
// script can put a watcher that CAN see beside one that CANNOT and tell them
// apart.
//
// Every list is ordered. An assertion that reads element 0 must read the same
// one on every run.
func (s *Spawns) HarnessState() map[string]interface{} {
	byWatcher := map[string]map[string]interface{}{}

	if s.notice != nil {
		for _, row := range s.notice.Report() {
			if id, ok := row["watcher"].(string); ok {
				byWatcher[id] = row
			}
		}
	}

	groups := make([]map[string]interface{}, 0, len(s.groups))

	for _, id := range s.groupIDs() {
		g := s.groups[id]

		seen := make([]map[string]interface{}, 0, len(g.members))
		aware := 0

		for _, m := range g.members {
			row, ok := byWatcher[m.WatcherID()]
			if !ok {
				continue
			}

			if noticed, _ := row["noticed"].(bool); noticed {
				aware++
			}

			seen = append(seen, row)
		}

		groups = append(groups, map[string]interface{}{
			"group":      g.id,
			"row":        g.row,
			"code":       g.code,
			"members":    len(g.members),
			"spawned":    g.spawned,
			"morale":     g.morale,
			"routing":    g.morale <= s.dials.RoutAt,
			"born_at":    g.bornAt,
			"born_stage": g.stage.String(),
			"born_band":  g.band,
			"weight":     g.weight,
			"aware":      aware,
			"notice":     seen,
		})
	}

	rows := make([]map[string]interface{}, 0, len(s.dials.Rows))

	for i := range s.dials.Rows {
		r := s.dials.Rows[i]
		weight := s.Weight(r)

		rows = append(rows, map[string]interface{}{
			"row":       r.Name,
			"code":      r.Code,
			"weight":    weight,
			"chance":    clamp01(weight * s.dials.Chance),
			"eligible":  weight > 0,
			"min_count": r.MinCount,
			"max_count": r.MaxCount,
			"carrion":   r.Carrion,
			"light":     r.LightDrawn,
		})
	}

	stage := "no-clock"
	if s.clock != nil {
		stage = s.clock.Stage().String()
	}

	out := map[string]interface{}{
		"groups":         len(s.groups),
		"group_list":     groups,
		"rows":           rows,
		"stage":          stage,
		"band":           s.Band(),
		"open_bodies":    s.openBodies,
		"carrion_weight": s.CarrionWeight(),
		"light_weight":   s.LightWeight(),
		"checks":         s.checks,
		"rolls":          s.rolls,
		"spawned":        s.spawned,
		"spawn_failures": s.failures,
		"despawned":      s.despawned,
		"cleared":        s.cleared,
		"check_minutes":  s.dials.CheckMinutes,
		"chance":         s.dials.Chance,
		"rout_at":        s.dials.RoutAt,
		"max_groups":     s.dials.MaxGroups,
		"has_target":     s.target != nil,
		"has_spawner":    s.spawner != nil,
	}

	// The notice dials and counters ride along here because the notice model
	// has no provider of its own -- by signature, ask 6 puts its reporting on
	// this provider. notice_wired is the one that matters when nothing has
	// noticed anything: it tells a script "nothing could have" apart from
	// "nothing did".
	if s.notice != nil {
		d := s.notice.Dials()
		out["notice_wired"] = s.notice.Wired()
		out["notice_watching"] = s.notice.Count()
		out["notice_checks"] = s.notice.Checks()
		out["notices"] = s.notice.Notices()
		out["notice_radius"] = d.Radius
		out["notice_lit_multiplier"] = d.LitMultiplier
		out["notice_lit_level"] = d.LitLevel
		out["notice_re_evaluate_minutes"] = d.ReEvaluateMinutes
		out["notice_memory_minutes"] = d.MemoryMinutes
		out["notice_aware"] = s.notice.Aware()

		// EVERY watcher, not only the grouped ones. The per-group blocks above
		// are the view the tables produce; this is the view ask 6 actually
		// promises, and the difference is not cosmetic -- a watcher started by
		// hand (strigoi_watch, which is how the negative control gets placed
		// exactly where it must be) belongs to no group and was invisible here
		// until the playtest went looking for it and could not find it. The
		// provider rule again, in its sixth costume: an assertion that names a
		// watcher needs a provider that reports THAT watcher.
		out["notice_list"] = s.notice.Report()
	} else {
		out["notice_wired"] = false
	}

	return out
}

// HarnessSettableFields lists the writes the system allows.
//
// There is no "spawn" field, and the omission is the same one Pursuit
// documents: creating a group needs the map and the entity factory, which
// d2world cannot reach. A script forces an arrival by raising "chance" and
// stepping the clock, which exercises the real table rather than bypassing it
// -- a spawn verb here would let a script prove something the game never does.
func (s *Spawns) HarnessSettableFields() []string {
	return []string{
		"chance", "check_minutes", "despawn", "morale",
		"notice_lit_level", "notice_radius", "open_bodies", "rout_at",
	}
}

// HarnessSet writes one allow-listed field.
func (s *Spawns) HarnessSet(field string, value interface{}) error {
	switch field {
	case "open_bodies":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("open_bodies wants a number, got %T", value)
		}

		if !s.SetOpenBodies(int(f)) {
			return fmt.Errorf("open_bodies cannot be negative, got %v", f)
		}

		return nil

	case "chance":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("chance wants a number, got %T", value)
		}

		if f < 0 {
			return fmt.Errorf("chance cannot be negative, got %v", f)
		}

		s.dials.Chance = f

		return nil

	case "check_minutes":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("check_minutes wants a number of WORLD minutes, got %T", value)
		}

		if f <= 0 {
			return fmt.Errorf("check_minutes wants a positive number of world minutes, got %v", f)
		}

		s.dials.CheckMinutes = f

		return nil

	case "rout_at":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("rout_at wants a number, got %T", value)
		}

		s.dials.RoutAt = f

		return nil

	case "notice_radius":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("notice_radius wants a number in world tiles, got %T", value)
		}

		if s.notice == nil || !s.notice.SetRadius(f) {
			return fmt.Errorf("notice_radius wants a positive number of world tiles, got %v", f)
		}

		return nil

	case "notice_lit_level":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("notice_lit_level wants a number in [0,1], got %T", value)
		}

		if s.notice == nil || !s.notice.SetLitLevel(f) {
			return fmt.Errorf("notice_lit_level wants a number in [0,1], got %v", f)
		}

		return nil

	case "despawn":
		id, ok := value.(string)
		if !ok {
			return fmt.Errorf(`despawn wants a group id like "g:1", got %T`, value)
		}

		if !s.Despawn(id) {
			return fmt.Errorf("no live group %q", id)
		}

		return nil

	case "morale":
		return s.setMorale(value)
	}

	return fmt.Errorf("spawns has no settable field %q", field)
}

// setMorale is the object-valued verb: {"group": "g:1", "value": 10}. It takes
// an object because morale is per group, and the alternative -- a field per
// group -- would make the settable list grow with the world.
func (s *Spawns) setMorale(value interface{}) error {
	obj, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`morale wants an object {"group": "g:1", "value": 10}, got %T`, value)
	}

	id, ok := obj["group"].(string)
	if !ok {
		return fmt.Errorf(`morale wants a "group" string like "g:1"`)
	}

	f, ok := toFloat(obj["value"])
	if !ok {
		return fmt.Errorf(`morale wants a numeric "value", got %T`, obj["value"])
	}

	if !s.SetMorale(id, f) {
		return fmt.Errorf("no live group %q", id)
	}

	return nil
}
