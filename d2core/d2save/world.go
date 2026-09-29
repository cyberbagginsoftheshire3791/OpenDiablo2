// Package d2save is the world save's file (M4.6 B3): N.od2.world.json, beside
// the hero's N.od2 and his kit-and-progress sidecar N.od2.strigoi.json.
//
// Josh's ruling (25 Sep 2026): the save "stops at that point then resumes at
// that point". This package is the file that makes that possible: its shape,
// its version, the checks a file must pass before a load may read it, and the
// write that never leaves half a file behind. It does not take the snapshots
// (Game.SaveWorld does, through the B2 verbs) and it does not load them (B4).
//
// THE FILE IS SELF-SUFFICIENT. It carries its own copy of the sidecar as it
// stood at the save, so a resume never pairs the world of one moment with the
// kit of another (build plan section 6).
//
// THE SHAPE is the B2 snapshot types themselves, not a second model of them:
// a field B2 adds to a snapshot is in the file the day it is added, and the
// classification tests B2 wrote over each system's struct are what keep the
// file complete. What is new here is only what B2 did not snapshot -- the
// hero, the entity list, the monster bodies, the screen's own bookkeeping
// (the scene) and the map -- each reported on a harness provider before it
// was saved (B1's rule: observability before serialisation).
package d2save

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2saveref"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// Version is the one version of the world file this build reads and writes.
// Any other is refused (ErrWorldVersion) and the file is set aside, never
// overwritten (rule 7). There is no migration: when the shape changes
// incompatibly, this number moves and the old file is set aside.
const Version = 1

// ErrWorldVersion is what a file of any version but Version is refused with.
// The error is a *VersionError naming the version the file holds.
var ErrWorldVersion = errors.New("d2save: this build reads world files of version 1 only")

// ErrWorldFile is what a version-1 file that no load could read is refused
// with: not JSON, a block missing or null, a block this build does not know,
// or a block that disagrees with another (World.Check).
var ErrWorldFile = errors.New("d2save: world file refused")

// VersionError is a refusal on the version: Version is what the file holds,
// as written ("2", "\"1\"", "1.0") or "absent".
type VersionError struct {
	Version string
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("%v: the file is version %s", ErrWorldVersion, e.Version)
}

// Unwrap makes errors.Is(err, ErrWorldVersion) true.
func (e *VersionError) Unwrap() error { return ErrWorldVersion }

// Blocks is every top-level key of the world file, in the order it is
// written. Every one is required: a load that found one missing would restore
// an empty system in its place, and a resumed night would diverge without a
// word -- so a missing block is refused, never defaulted.
//
// The order is World's field order; TestBlocksAreTheWorldsFields holds the two
// together.
var Blocks = []string{
	"version", "build", "saved_at",
	"map", "seed", "rng", "hero", "sidecar",
	"clock", "light", "squads", "spawns", "spawner", "notice", "pursuit",
	"corpses", "rising", "combat", "bodies", "entities", "scene",
}

// World is the whole file.
type World struct {
	// Version is 1. Build is the build that wrote the file (informational: a
	// load does not refuse another build's file of the same version). SavedAt
	// is the wall-clock moment of the save, RFC 3339 in UTC -- the one field
	// two saves of one moment may differ in.
	Version int    `json:"version"`
	Build   string `json:"build"`
	SavedAt string `json:"saved_at"`

	// Map is the world the game was built from (D5: a load compares the SHA
	// and sets the file aside on a changed map).
	Map Map `json:"map"`

	// Seed is the game's seed. It is written as a JSON STRING: a wall-clock
	// seed (UnixNano, ~1.8e18) is past 2^53, where a float64 reader -- every
	// playtest's mustNum -- reads a different number (B1 notes, section 2).
	Seed int64 `json:"seed,string"`

	// RNG is where every counted stream stands.
	RNG RNG `json:"rng"`

	// Hero is the player's own state that the .od2 does not carry, or carries
	// at the wrong moment: where he stands, his health, and the run toggle.
	Hero Hero `json:"hero"`

	// Sidecar is the kit-and-progress file as SaveWorld wrote it in the same
	// save, byte for byte the document N.od2.strigoi.json holds: version, kit,
	// progress, village, land and journal (d2items.HeroBytes). On load it wins
	// over whatever the sidecar file holds.
	Sidecar json.RawMessage `json:"sidecar"`

	// The world systems, each its B2 snapshot.
	Clock   d2world.ClockSnapshot   `json:"clock"`
	Light   d2world.LightSnapshot   `json:"light"`
	Squads  d2world.SquadsSnapshot  `json:"squads"`
	Spawns  d2world.SpawnsSnapshot  `json:"spawns"`
	Spawner Spawner                 `json:"spawner"`
	Notice  d2world.NoticeSnapshot  `json:"notice"`
	Pursuit d2world.PursuitSnapshot `json:"pursuit"`
	Corpses d2world.CorpsesSnapshot `json:"corpses"`
	Rising  d2world.RisingSnapshot  `json:"rising"`
	Combat  d2world.CombatSnapshot  `json:"combat"`

	// Bodies is every monster's health, by entity id, sorted by id: the only
	// place a wolf's wounds live (the game screen's npcBody).
	Bodies []Body `json:"bodies"`

	// Entities is every map entity the save carries, sorted by id: every NPC
	// and creature on the map (never the player, whose id is his connection's
	// and new every launch; never an item, a missile or an object).
	Entities []Entity `json:"entities"`

	// Scene is the game screen's own bookkeeping.
	Scene Scene `json:"scene"`
}

// Map is the world the game was built from. For an authored map, Path is the
// .tmj the game built (d2mapgen.HostMap) and SHA the SHA-256 of its bytes, in
// hex. Diablo II's generated Act 1 has neither -- it is the seed's -- and says
// so with Generated.
type Map struct {
	Path      string `json:"path"`
	SHA       string `json:"sha"`
	Generated bool   `json:"generated"`
}

// RNG is every counted stream: the map engine's world stream, and copies of
// the three gameplay streams whose own blocks carry them (spawns.rng,
// combat.rng, rising.rng). The copies must equal those blocks -- a file where
// they disagree is refused (Check) -- so this is the one place a load reads
// to check every stream against the seed (the B4 load order, step 4), and it
// can never be a second truth.
//
// UUID is the harness's seeded uuid stream, in a harness build whose game was
// seeded; absent in the shipped game, where ids come from crypto/rand.
type RNG struct {
	World  d2rand.StreamState `json:"world"`
	Spawns d2rand.StreamState `json:"spawns"`
	Combat d2rand.StreamState `json:"combat"`
	Rising d2rand.StreamState `json:"rising"`
	UUID   *UUIDStream        `json:"uuid,omitempty"`
}

// UUIDStream is the uuid reader's position: its seed (a JSON string, exact
// past 2^53) and the bytes it has handed out -- counted in bytes, not draws,
// because it reads through rand.Rand.Read (d2rand.Reader). B4b restores it
// with d2rand.RestoreReader AFTER the entities are rebuilt (trap 6).
type UUIDStream struct {
	Seed  int64  `json:"seed,string"`
	Bytes uint64 `json:"bytes"`
}

// Hero is the player's saved place, health and run toggle.
//
// X and Y are WORLD TILES, the units every other position in the file but a
// motion's is in (the spawns members', the harness's). Pos is the same point
// in SUB-TILES, the engine's own units, so a load can put him back bit-exact:
// world tiles times five is not always the sub-tile it came from in floating
// point. Check holds the two together.
//
// Rule 4: a walk he was in the middle of does not continue, so no motion is
// carried for him -- he stands at Pos after a load.
type Hero struct {
	X      float64    `json:"x"`
	Y      float64    `json:"y"`
	Pos    [2]float64 `json:"pos"`
	Health int        `json:"health"`
	Run    bool       `json:"run"`
}

// Spawner is the game spawner's one saved number (d2gamescreen's
// SpawnerSnapshot, whose shape this is: that type lives in the game screen,
// which imports this package, so the file names the same JSON here).
type Spawner struct {
	Arrival int `json:"arrival"`
}

// Body is one monster's health.
type Body struct {
	ID        string `json:"id"`
	Health    int    `json:"health"`
	MaxHealth int    `json:"max_health"`
}

// Entity kinds.
const (
	KindNPC      = "npc"      // an inherited monster (*d2mapentity.NPC), rebuilt by NewNPC from Monstat
	KindCreature = "creature" // a project creature (*d2mapentity.Creature), rebuilt by NewCreature from its bestiary entry
)

// Entity is one map entity.
//
// NATIVE entities are the ones the map built -- the villagers, in file order
// -- and were on it when the game screen was made. A load does not rebuild
// them: the map does. It re-keys each to its saved id instead (B4b), matched
// by NameKey and Born, the place the map put it. Every other entity is
// rebuilt through the next-id seam with its saved id.
//
// X and Y are world tiles, and Motion.Pos the same point in sub-tiles (Check
// holds them together); a spawns member resolves to the entity standing
// EXACTLY at his saved x, y.
type Entity struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Native   bool   `json:"native"`
	Monstat  string `json:"monstat,omitempty"`
	Creature string `json:"creature,omitempty"`
	NameKey  string `json:"name_key,omitempty"`

	Born *[2]float64 `json:"born,omitempty"`

	X      float64            `json:"x"`
	Y      float64            `json:"y"`
	Motion d2mapentity.Motion `json:"motion"`
}

// Scene is the game screen's own bookkeeping (the "scene" and "village"
// providers report every field).
//
//   - WatchStood: the minutes of tonight's watch stood so far.
//   - FieldDead: Night 1's placed dead IN THE ORDER THEY WERE LAID -- the i-th
//     carries the i-th writing, so the order is state.
//   - DawnPaidDay and LastStage: which dawn has paid its night, and the stage
//     the last frame saw. Restored together from the file (trap 1, BUG-17).
//   - WatchClock and WatchClockSet: the watch's last-frame clock. The plan
//     had it derived (D); it is saved, because it is NOT always the clock's
//     own minutes: a labour verb moves the clock inside a frame and the next
//     keepWatch credits the jump (capped), so a save between the two that
//     derived it would credit nothing.
type Scene struct {
	WatchStood    float64  `json:"watch_stood"`
	FieldDead     []string `json:"field_dead"`
	DawnPaidDay   int      `json:"dawn_paid_day"`
	LastStage     string   `json:"last_stage"`
	WatchClock    float64  `json:"watch_clock"`
	WatchClockSet bool     `json:"watch_clock_set"`
}

// Encode writes the world file: every block in Blocks' order, indented two
// spaces, with a trailing newline. It is deterministic -- the same World
// encodes to the same bytes, and a file decoded and encoded again is the file
// it was (TestEncodeDecodeEncodeIsByteStable).
//
// omit drops the named top-level blocks. It exists for the negative controls
// (burst B6: every block dropped in turn must make a resumed night diverge or
// be refused), and a file written with it is one no load will read. A name
// that is not a block is an error.
func Encode(w *World, omit []string) ([]byte, error) {
	if w == nil {
		return nil, fmt.Errorf("%w: no world to encode", ErrWorldFile)
	}

	drop := map[string]bool{}

	for _, name := range omit {
		if !isBlock(name) {
			return nil, fmt.Errorf("d2save: %q is not a block of the world file (%s)", name, strings.Join(Blocks, ", "))
		}

		drop[name] = true
	}

	compact, err := json.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("d2save: encoding the world: %w", err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(compact, &top); err != nil {
		return nil, fmt.Errorf("d2save: encoding the world: %w", err)
	}

	var buf bytes.Buffer

	buf.WriteByte('{')

	first := true

	for _, name := range Blocks {
		raw, ok := top[name]
		if !ok {
			return nil, fmt.Errorf("d2save: World has no %q field -- Blocks and World have drifted apart", name)
		}

		if drop[name] {
			continue
		}

		if !first {
			buf.WriteByte(',')
		}

		first = false

		key, _ := json.Marshal(name)
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(raw)
	}

	buf.WriteByte('}')

	var out bytes.Buffer
	if err := json.Indent(&out, buf.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("d2save: encoding the world: %w", err)
	}

	out.WriteByte('\n')

	return out.Bytes(), nil
}

// isBlock reports whether name is a top-level block.
func isBlock(name string) bool {
	for _, b := range Blocks {
		if b == name {
			return true
		}
	}

	return false
}

// Decode reads a world file, refusing anything a load could not restore
// exactly. In order:
//
//  1. Not a JSON object: ErrWorldFile.
//  2. A version that is not the integer 1 -- another number, a string, a
//     float, or none -- is a *VersionError (ErrWorldVersion), and nothing
//     else is read: rule 7 sets such a file aside unread.
//  3. A block missing, null, or unknown: ErrWorldFile.
//  4. Any field this build does not know, at any depth: ErrWorldFile (the
//     decoder disallows unknown fields). A file from a build whose snapshot
//     grew a field it did not bump the version for is refused, not half read.
//  5. Check.
//
// Seeds decode as int64 through typed fields, never through a float64.
func Decode(data []byte) (*World, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("%w: not a JSON object: %v", ErrWorldFile, err)
	}

	if top == nil {
		return nil, fmt.Errorf("%w: not a JSON object (null)", ErrWorldFile)
	}

	version, ok := top["version"]
	if !ok {
		return nil, &VersionError{Version: "absent"}
	}

	if v := string(bytes.TrimSpace(version)); v != "1" {
		return nil, &VersionError{Version: v}
	}

	for _, name := range Blocks {
		raw, ok := top[name]

		switch {
		case !ok:
			return nil, fmt.Errorf("%w: the %q block is missing", ErrWorldFile, name)
		case string(bytes.TrimSpace(raw)) == "null":
			return nil, fmt.Errorf("%w: the %q block is null", ErrWorldFile, name)
		}
	}

	for name := range top {
		if !isBlock(name) {
			return nil, fmt.Errorf("%w: %q is not a block this build knows", ErrWorldFile, name)
		}
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var w World
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWorldFile, err)
	}

	if dec.More() {
		return nil, fmt.Errorf("%w: data after the world object", ErrWorldFile)
	}

	if err := w.Check(); err != nil {
		return nil, err
	}

	return &w, nil
}

// VersionOf is the version a file holds, as written, without reading the rest:
// "1" for a file this build may read, another number or token for one it may
// not, "absent" for a JSON object with no version, and "" for bytes that are
// not a JSON object at all.
func VersionOf(data []byte) string {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
		return ""
	}

	v, ok := top["version"]
	if !ok {
		return "absent"
	}

	return string(bytes.TrimSpace(v))
}

var shaHex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Check is every refusal the file can make on its own, with no game to hand:
// the blocks agree with each other, every stream is the one this seed runs,
// and every id a live record points at is an entity the file carries. The
// systems' own checks (each B2 Validate) need the game the load is building,
// and are B4's (D4).
//
// SaveWorld runs it on the file it is about to write, so a file no load could
// read is refused before it is written (B2b's "strict at save").
//
// The checks are called one by one, not from a slice of method values: a
// method value is a func() error that escapes, and the reachability gate's
// call graph then lets ANY dynamic call of a func() error in the program --
// pkg/profile's, measured -- reach it, so d2rand's check read as wired in the
// shipped game (the reach gate's first B3 run, 28 Sep 2026).
func (w *World) Check() error {
	err := w.checkHeader()

	if err == nil {
		err = w.checkMap()
	}

	if err == nil {
		err = w.checkRNG()
	}

	if err == nil {
		err = w.checkHero()
	}

	if err == nil {
		err = w.checkSidecar()
	}

	if err == nil {
		err = w.checkClock()
	}

	if err == nil {
		err = w.checkEntities()
	}

	if err == nil {
		err = w.checkBodies()
	}

	if err == nil {
		err = w.checkRefs()
	}

	if err == nil {
		err = w.checkScene()
	}

	if err != nil {
		return fmt.Errorf("%w: %v", ErrWorldFile, err)
	}

	return nil
}

func (w *World) checkHeader() error {
	if w.Version != Version {
		return fmt.Errorf("version %d", w.Version)
	}

	return nil
}

func (w *World) checkMap() error {
	switch {
	case w.Map.Generated && (w.Map.Path != "" || w.Map.SHA != ""):
		return fmt.Errorf("map: a generated world has no path or sha (%q, %q)", w.Map.Path, w.Map.SHA)
	case !w.Map.Generated && w.Map.Path == "":
		return errors.New("map: an authored world names its .tmj")
	case !w.Map.Generated && !shaHex.MatchString(w.Map.SHA):
		return fmt.Errorf("map: %q is not the SHA-256 of %s in hex", w.Map.SHA, w.Map.Path)
	}

	return nil
}

// checkRNG: the copies equal their blocks, and every stream is the one a game
// of this seed runs, no further on than MaxDraws (d2rand.StreamState.Check).
func (w *World) checkRNG() error {
	copies := []struct {
		name        string
		copy, block d2rand.StreamState
	}{
		{d2rand.StreamSpawns, w.RNG.Spawns, w.Spawns.RNG},
		{d2rand.StreamCombat, w.RNG.Combat, w.Combat.RNG},
		{d2rand.StreamRising, w.RNG.Rising, w.Rising.RNG},
	}

	for _, c := range copies {
		if c.copy != c.block {
			return fmt.Errorf("rng.%s is %+v and the %s block's own stream is %+v", c.name, c.copy, c.name, c.block)
		}
	}

	streams := []struct {
		name string
		s    d2rand.StreamState
	}{
		{d2rand.StreamWorld, w.RNG.World},
		{d2rand.StreamSpawns, w.RNG.Spawns},
		{d2rand.StreamCombat, w.RNG.Combat},
		{d2rand.StreamRising, w.RNG.Rising},
	}

	for _, st := range streams {
		if err := st.s.Check(w.Seed, st.name); err != nil {
			return err
		}
	}

	return nil
}

func (w *World) checkHero() error {
	h := w.Hero
	if err := finite("hero", h.X, h.Y, h.Pos[0], h.Pos[1]); err != nil {
		return err
	}

	if err := samePlace("hero", h.X, h.Y, h.Pos); err != nil {
		return err
	}

	// A dead hero is never saved (DEAD, the 12 Sep ruling).
	if h.Health <= 0 {
		return fmt.Errorf("hero: health %d -- a dead hero is never saved", h.Health)
	}

	return nil
}

// checkSidecar: the embedded sidecar is the kit file's document, version 1,
// with a kit.
func (w *World) checkSidecar() error {
	var sc struct {
		Version int             `json:"version"`
		Kit     json.RawMessage `json:"kit"`
	}

	if err := json.Unmarshal(w.Sidecar, &sc); err != nil {
		return fmt.Errorf("sidecar: %v", err)
	}

	if sc.Version != 1 || len(sc.Kit) == 0 || string(sc.Kit) == "null" {
		return fmt.Errorf("sidecar: version %d with kit %s -- the kit file's document is version 1 and has a kit", sc.Version, sc.Kit)
	}

	return nil
}

func (w *World) checkClock() error {
	if e := w.Clock.Elapsed; math.IsNaN(e) || math.IsInf(e, 0) || e < 0 {
		return fmt.Errorf("clock: elapsed %v", e)
	}

	return nil
}

func (w *World) checkEntities() error {
	seen := map[string]bool{}

	for i, e := range w.Entities {
		what := fmt.Sprintf("entities[%d] %q", i, e.ID)

		switch {
		case e.ID == "" || e.ID == d2saveref.Player:
			return fmt.Errorf("%s: not an entity id", what)
		case seen[e.ID]:
			return fmt.Errorf("%s: saved twice", what)
		case i > 0 && w.Entities[i-1].ID > e.ID:
			return fmt.Errorf("%s: the list is sorted by id", what)
		}

		seen[e.ID] = true

		switch e.Kind {
		case KindNPC:
			if e.Monstat == "" || e.Creature != "" {
				return fmt.Errorf("%s: an npc names its monstat and no creature (%q, %q)", what, e.Monstat, e.Creature)
			}
		case KindCreature:
			if e.Creature == "" || e.Monstat != "" {
				return fmt.Errorf("%s: a creature names its bestiary id and no monstat (%q, %q)", what, e.Creature, e.Monstat)
			}
		default:
			return fmt.Errorf("%s: kind %q is not %s or %s", what, e.Kind, KindNPC, KindCreature)
		}

		if e.Native != (e.Born != nil) {
			return fmt.Errorf("%s: a native entity, and only one, carries where the map put it (native %v, born %v)", what, e.Native, e.Born)
		}

		nums := []float64{e.X, e.Y, e.Motion.Pos[0], e.Motion.Pos[1], e.Motion.Target[0], e.Motion.Target[1],
			e.Motion.Velocity[0], e.Motion.Velocity[1], e.Motion.Speed}
		for _, p := range e.Motion.Path {
			nums = append(nums, p[0], p[1])
		}

		if e.Born != nil {
			nums = append(nums, e.Born[0], e.Born[1])
		}

		if err := finite(what, nums...); err != nil {
			return err
		}

		if err := samePlace(what, e.X, e.Y, e.Motion.Pos); err != nil {
			return err
		}
	}

	return nil
}

func (w *World) checkBodies() error {
	entities := w.entityIDs()
	seen := map[string]bool{}

	for i, b := range w.Bodies {
		what := fmt.Sprintf("bodies[%d] %q", i, b.ID)

		switch {
		case seen[b.ID]:
			return fmt.Errorf("%s: saved twice", what)
		case i > 0 && w.Bodies[i-1].ID > b.ID:
			return fmt.Errorf("%s: the list is sorted by id", what)
		case !entities[b.ID]:
			return fmt.Errorf("%s: a body with no entity in the file", what)
		case b.MaxHealth < 1 || b.Health < 0 || b.Health > b.MaxHealth:
			return fmt.Errorf("%s: health %d of %d", what, b.Health, b.MaxHealth)
		}

		seen[b.ID] = true
	}

	return nil
}

// checkRefs: every id a LIVE record points at is an entity the file carries --
// a spawns member on the map (standing exactly where the entity stands), a
// watcher and its target, a hunter and its quarry, a deployed squad's model.
// The player is the word "player". A gone member is the one id that must NOT
// be an entity (B2b). The corpse registry is history, not a live reference:
// its maps name members long gone.
func (w *World) checkRefs() error {
	entities := map[string]Entity{}
	for _, e := range w.Entities {
		entities[e.ID] = e
	}

	isEntity := func(id string) bool { _, ok := entities[id]; return ok }
	isQuarry := func(ref string) bool { return ref == d2saveref.Player || isEntity(ref) }

	for _, g := range w.Spawns.Groups {
		for _, m := range g.Members {
			e, ok := entities[m.ID]

			switch {
			case m.Gone && ok:
				return fmt.Errorf("spawns %s: %s is gone, and the file carries an entity with his id", g.ID, m.ID)
			case m.Gone:
			case !ok:
				return fmt.Errorf("spawns %s: member %s is on the map and not in the entity list", g.ID, m.ID)
			case e.X != m.X || e.Y != m.Y:
				return fmt.Errorf("spawns %s: member %s stood at %v,%v and his entity at %v,%v", g.ID, m.ID, m.X, m.Y, e.X, e.Y)
			}
		}
	}

	for _, wt := range w.Notice.Watches {
		if !isEntity(wt.Watcher) || !isQuarry(wt.Target) {
			return fmt.Errorf("notice: watcher %q or target %q is not in the entity list", wt.Watcher, wt.Target)
		}
	}

	for _, c := range w.Pursuit.Chases {
		if !isEntity(c.Hunter) || !isQuarry(c.Quarry) {
			return fmt.Errorf("pursuit: hunter %q or quarry %q is not in the entity list", c.Hunter, c.Quarry)
		}
	}

	for _, sq := range w.Squads.Squads {
		for _, m := range sq.Members {
			if !isQuarry(m.Entity) {
				return fmt.Errorf("squads %s: model %q is not in the entity list", sq.ID, m.Entity)
			}
		}
	}

	return nil
}

func (w *World) checkScene() error {
	s := w.Scene
	if err := finite("scene", s.WatchStood, s.WatchClock); err != nil {
		return err
	}

	if s.WatchStood < 0 {
		return fmt.Errorf("scene: watch_stood %v", s.WatchStood)
	}

	for _, stage := range []d2world.Stage{d2world.StageNight, d2world.StageDawn, d2world.StageDay, d2world.StageDusk} {
		if stage.String() == s.LastStage {
			return nil
		}
	}

	return fmt.Errorf("scene: last_stage %q is no stage", s.LastStage)
}

func (w *World) entityIDs() map[string]bool {
	ids := make(map[string]bool, len(w.Entities))
	for _, e := range w.Entities {
		ids[e.ID] = true
	}

	return ids
}

// finite refuses a NaN or an infinity: no running game has one, and d2vector
// panics on one.
func finite(what string, nums ...float64) error {
	for _, n := range nums {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("%s: %v is not a number a game can have", what, n)
		}
	}

	return nil
}

// samePlace holds world tiles and sub-tiles together: x, y must be exactly the
// world point the engine computes from pos (d2vector.Position.World, what
// GetPositionF returns).
func samePlace(what string, x, y float64, pos [2]float64) error {
	p := d2vector.NewPosition(pos[0], pos[1])
	world := p.World()

	if world.X() != x || world.Y() != y {
		return fmt.Errorf("%s: x, y %v,%v is not the world point of pos %v (%v,%v)", what, x, y, pos, world.X(), world.Y())
	}

	return nil
}
