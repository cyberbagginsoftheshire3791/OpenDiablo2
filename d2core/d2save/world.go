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
	"sort"
	"strconv"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2saveref"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// Version is the one version of the world file this build reads and writes.
// Any other is refused (ErrWorldVersion) and the file is set aside, never
// overwritten (rule 7). There is no migration: when the shape changes
// incompatibly, this number moves and the old file is set aside.
//
// NOTHING ELSE NAMES THE NUMBER (the raid's R0.5, 29 Sep 2026). Decode, the
// refusal's message, WriteWorld's keep-or-set-aside and every test derive it
// from here, so a bump is this line, testdata/world-v<Version>.json
// regenerated and its shape hash recorded (world_shape_test.go), nothing more.
//
// VERSION 2 (the raid's R1, 29 Sep 2026): the combat block gained its clock
// -- the fights he is not in, their stream and records, and the clock fights
// live at the save (the raid's Q5 (a)) -- and rng gained combat_clock. A
// version-1 file is refused and set aside as .v1.unread (rule 7), never read
// and never lost.
//
// THAT COSTS JOSH NOTHING, AND IT WAS MEASURED (the R1 review's B3, decision
// (a), accepted): on 29 Sep his %APPDATA%\OpenDiablo2\Saves held no world file
// at all, and the save verb is not a player's until B5, so no version-1 file
// of his exists to be set aside -- the HUNTED nights B4a keeps for B4b
// included.
//
// This is the raid milestone's one bump, and THE RULE UNTIL THE MILESTONE
// SHIPS IS THIS: a later raid burst may amend version 2's shape without a
// second bump. A world file written by a build between two such bursts is
// then refused by the later build -- FILE, a shape it does not know (Decode
// refuses a field missing or unknown at any depth) -- and set aside beside
// his save, never read with a field at zero, never overwritten, never lost.
// Josh plays between bursts, so this is his to know (the raid brief, section
// 4; the R1 build note).
//
// VERSION 2, AMENDED ONCE (M4.6 BUG-87, 29 Sep 2026): an entity's motion
// gained action_at -- a held action's frame and the time into it -- so a
// swing, a blow taken or a death saved half-played resumes at its frame. The
// same rule, taken by M4.6 before saving is a player's (B5): no bump. A
// version-2 file from before it that holds an action cannot exist -- the save
// refused while one was held (BUG-76) -- and one that did would be refused at
// the load's step 3 (ENTITY, NATIVES: RestoreMotion refuses a held action
// that carries no frame). One that holds none is the same file either side
// of the amendment: action_at is written only with an action.
//
// SAVING IS A PLAYER'S SINCE THE SAVE-HELD x B5 MERGE (29 Sep 2026): the
// escape menu, the window's close and the dawn write world files in his own
// folder now, so an amendment from here on is refused FILE in every save he
// has made -- set aside, never read with a field at zero and never lost, but
// his to know before it ships. The golden file and its shape hash stood
// through the merge (B5 changed no shape).
//
// VERSION 3 (the raid's R2, 29 Sep 2026, and its review fixes, 1 Oct 2026).
// Josh's ruling of 30 Sep: every shape change bumps Version from here on --
// saving is a player's since B5, so a file of his is in play.
// Version 3 adds notice.watches[].side (a watch is hostile or living), a new
// top-level block, seek (who the night's hunters choose among the living:
// its rows with their dwell, its stand-ins, phases and four totals), and
// pursuit.rechase_solves (a retarget's solve, counted). A version-2 file is
// refused on its version (ErrWorldVersion) and set aside as .v2.unread, never
// read and never lost; he begins at dawn with his hero, kit and progress (the
// sidecar), as at every refused load. Version 2's golden file and hash stay
// as the build that wrote them left them (the R2 review's C4).
//
// VERSION 4 (fog of war F3, "kept", 1 Oct 2026): a new top-level block, fog
// -- the explored grid (d2world.FogSnapshot: the map it was explored on, its
// size, and the grid as base64 bits), so the ground he has seen is still
// remembered after a load. The dials are not saved and what he sees now is
// not (it is derived: recomputed from his eyes on the first frame). A
// version-3 file is refused on its version and set aside as .v3.unread, never
// read and never lost; he begins at dawn with his hero, kit and progress, as
// at the last bump -- the decided default, no migration, Josh's to overturn
// (docs/m4.6-world-save-notes.md, "Version 4"). MEASURED, 1 Oct 2026: his
// %APPDATA%\OpenDiablo2\Saves holds no world file of any version, so none of
// his is set aside by this bump.
//
// VERSION 5 (the per-frame A* budget's second review, A; 2 Oct 2026):
// pursuit.chases[].from_watch -- a WORLD chase, started or moved by
// Pursuit.Rechase from a watch, as against a script's -- written only when
// true. Until now a load read every chase as a script's, so a world chase
// saved while its re-chase was deferred, whose watch then forgot, stood in the
// resumed game and was released in the game that ran on (BUG-108's give-up):
// the resume diverged. A version-4 file is refused on its version and set
// aside as .v4.unread, never read and never lost; he begins at dawn with his
// hero, kit and progress, as at every bump -- the decided default, no
// migration, Josh's to overturn (docs/m4.6-world-save-notes.md, "The second
// review of the per-frame budget"). MEASURED, 2 Oct 2026: his
// %APPDATA%\OpenDiablo2\Saves holds no world file of any version, so none of
// his is set aside by this bump.
const Version = 5

// ErrWorldVersion is what a file of any version but Version is refused with.
// The error is a *VersionError naming the version the file holds. Its message
// names Version, not a literal (the raid's R0.5, 29 Sep 2026: "the version
// follows Version", so a bump is the constant, the golden file and the hash).
var ErrWorldVersion = fmt.Errorf("d2save: this build reads world files of version %d only", Version)

// ErrWorldFile is what a file of this Version that no load could read is refused
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

// FileError is a refusal of a file of this Version: Reason names the one rule that
// refused it (the Reason* constants), Detail says what it found. Every
// refusal Decode, Check, CheckHeroFile and SameMoment make is one, and
// errors.Is(err, ErrWorldFile) holds for each.
//
// The reason is for the tests first (the B3 review, A2: a test that asserted
// only ErrWorldFile stayed green with a rule deleted, because another rule
// refused the same file) and for the load's log second.
type FileError struct {
	Reason string
	Detail string
}

func (e *FileError) Error() string { return fmt.Sprintf("%v: %s", ErrWorldFile, e.Detail) }

// Unwrap makes errors.Is(err, ErrWorldFile) true.
func (e *FileError) Unwrap() error { return ErrWorldFile }

// ReasonOf is the Reason of the *FileError in err's chain -- ReasonVersion
// for a *VersionError -- or "".
func ReasonOf(err error) string {
	var (
		fe *FileError
		ve *VersionError
	)

	switch {
	case errors.As(err, &fe):
		return fe.Reason
	case errors.As(err, &ve):
		return ReasonVersion
	}

	return ""
}

func refuse(reason, format string, args ...interface{}) error {
	return &FileError{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// The rules a file is refused by (FileError.Reason).
const (
	// Decode's own.
	ReasonJSON         = "json"          // not a JSON object, or something after it
	ReasonBlockMissing = "block-missing" // a top-level block absent
	ReasonBlockNull    = "block-null"    // a top-level block null
	ReasonBlockUnknown = "block-unknown" // a top-level key this build does not know
	ReasonField        = "field"         // a field this build does not know, or a value of the wrong type
	ReasonFieldMissing = "field-missing" // a field this build writes and the file lacks (B3-8; the review's B1)

	// Check's.
	ReasonVersion       = "version"
	ReasonMap           = "map"
	ReasonRNGCopy       = "rng-copy"
	ReasonRNGStream     = "rng-stream"
	ReasonHeroIdentity  = "hero-identity"
	ReasonHeroFinite    = "hero-finite"
	ReasonHeroPlace     = "hero-place"
	ReasonHeroDead      = "hero-dead"
	ReasonHeroFacing    = "hero-facing"
	ReasonHeroStamina   = "hero-stamina"
	ReasonSidecar       = "sidecar"
	ReasonGeneration    = "generation"
	ReasonClock         = "clock"
	ReasonEntityID      = "entity-id"
	ReasonEntityTwice   = "entity-twice"
	ReasonEntityOrder   = "entity-order"
	ReasonEntityKind    = "entity-kind"
	ReasonEntityNative  = "entity-native"
	ReasonNativeTwice   = "native-twice"
	ReasonEntityFinite  = "entity-finite"
	ReasonEntityPlace   = "entity-place"
	ReasonBodyTwice     = "body-twice"
	ReasonBodyOrder     = "body-order"
	ReasonBodyOrphan    = "body-orphan"
	ReasonBodyHealth    = "body-health"
	ReasonMemberGone    = "member-gone"
	ReasonMemberMissing = "member-missing"
	ReasonMemberPlace   = "member-place"
	ReasonWatch         = "watch"
	ReasonChase         = "chase"
	ReasonSquadModel    = "squad-model"
	ReasonSceneFinite   = "scene-finite"
	ReasonSceneWatch    = "scene-watch"
	ReasonSceneStage    = "scene-stage"
	ReasonFog           = "fog" // fog of war F3: a fog block Snapshot could not have written, or of another map than the file's

	// The pairing with the files beside it (B4's step 1).
	ReasonPairHero   = "pair-hero"   // CheckHeroFile: another hero's .od2
	ReasonPairMoment = "pair-moment" // SameMoment: a sidecar of another generation
)

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
	"clock", "light", "squads", "spawns", "spawner", "notice", "pursuit", "seek",
	"corpses", "rising", "combat", "bodies", "entities", "scene", "fog",
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

	// The world systems, each its B2 snapshot. Seek is the raid's R2 (who
	// the night's hunters choose among the living), new in version 3 (see
	// Version).
	Clock   d2world.ClockSnapshot   `json:"clock"`
	Light   d2world.LightSnapshot   `json:"light"`
	Squads  d2world.SquadsSnapshot  `json:"squads"`
	Spawns  d2world.SpawnsSnapshot  `json:"spawns"`
	Spawner Spawner                 `json:"spawner"`
	Notice  d2world.NoticeSnapshot  `json:"notice"`
	Pursuit d2world.PursuitSnapshot `json:"pursuit"`
	Seek    d2world.SeekSnapshot    `json:"seek"`
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

	// Fog is the explored grid (fog of war F3, new in version 4): every tile
	// he has seen, on the map it was explored on. Empty (0 x 0) for a game
	// that never looked -- fog off -- or had not looked yet. Check holds its
	// map to the file's.
	Fog d2world.FogSnapshot `json:"fog"`
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
//
// CombatClock is the fights he is not in (the raid's R1): combat's second
// stream, combat-clock, whose own block is combat.clock.rng.
type RNG struct {
	World       d2rand.StreamState `json:"world"`
	Spawns      d2rand.StreamState `json:"spawns"`
	Combat      d2rand.StreamState `json:"combat"`
	CombatClock d2rand.StreamState `json:"combat_clock"`
	Rising      d2rand.StreamState `json:"rising"`
	UUID        *UUIDStream        `json:"uuid,omitempty"`
}

// UUIDStream is the uuid reader's position: its seed (a JSON string, exact
// past 2^53) and the bytes it has handed out -- counted in bytes, not draws,
// because it reads through rand.Rand.Read (d2rand.Reader). B4b restores it
// with d2rand.RestoreReader AFTER the entities are rebuilt (trap 6).
type UUIDStream struct {
	Seed  int64  `json:"seed,string"`
	Bytes uint64 `json:"bytes"`
}

// Hero is who the player is, and his saved place, facing, health, stamina and
// run toggle.
//
// Name and Class are the .od2's heroName and heroType (by name, "Amazon"): the
// file says whose it is, and a load refuses it beside another hero's .od2
// (CheckHeroFile; the B3 review, A1).
//
// X and Y are WORLD TILES, the units every other position in the file but a
// motion's is in (the spawns members', the harness's). Pos is the same point
// in SUB-TILES, the engine's own units, so a load can put him back bit-exact:
// world tiles times five is not always the sub-tile it came from in floating
// point. Check holds the two together.
//
// Facing is the direction his body faces (Player.Facing), and Stamina his
// stamina, which the .od2 does not carry (its json:"-"): both were missing
// from B3's file, and the digest compares both (the B3 review, B6).
//
// Rule 4: a walk he was in the middle of does not continue, so no motion is
// carried for him -- he stands at Pos after a load, facing Facing.
type Hero struct {
	Name    string     `json:"name"`
	Class   string     `json:"class"`
	X       float64    `json:"x"`
	Y       float64    `json:"y"`
	Pos     [2]float64 `json:"pos"`
	Facing  int        `json:"facing"`
	Health  int        `json:"health"`
	Stamina float64    `json:"stamina"`
	Run     bool       `json:"run"`
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
//  2. A version that is not the integer Version -- another number, a string, a
//     float, or none -- is a *VersionError (ErrWorldVersion), and nothing
//     else is read: rule 7 sets such a file aside unread.
//  3. A block missing, null, or unknown: ErrWorldFile.
//  4. Any field this build does not know, at any depth: ErrWorldFile (the
//     decoder disallows unknown fields). A file from a build whose snapshot
//     grew a field it did not bump the version for is refused, not half read.
//  5. Any field this build WRITES that the file lacks, at any depth
//     (missingKey; the B3 review, B1): a file from before a field existed is
//     refused, never read with the field at zero.
//  6. Check.
//
// Every refusal but the version's is a *FileError naming its rule (ReasonOf).
// Seeds decode as int64 through typed fields, never through a float64.
func Decode(data []byte) (*World, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, refuse(ReasonJSON, "not a JSON object: %v", err)
	}

	if top == nil {
		return nil, refuse(ReasonJSON, "not a JSON object (null)")
	}

	version, ok := top["version"]
	if !ok {
		return nil, &VersionError{Version: "absent"}
	}

	// The version follows Version (R0.5): never a literal, so the milestone's
	// one bump is the constant, the golden file and its hash (B3-8).
	if v := string(bytes.TrimSpace(version)); v != strconv.Itoa(Version) {
		return nil, &VersionError{Version: v}
	}

	for _, name := range Blocks {
		raw, ok := top[name]

		switch {
		case !ok:
			return nil, refuse(ReasonBlockMissing, "the %q block is missing", name)
		case string(bytes.TrimSpace(raw)) == "null":
			return nil, refuse(ReasonBlockNull, "the %q block is null", name)
		}
	}

	for name := range top {
		if !isBlock(name) {
			return nil, refuse(ReasonBlockUnknown, "%q is not a block this build knows", name)
		}
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var w World
	if err := dec.Decode(&w); err != nil {
		return nil, refuse(ReasonField, "%v", err)
	}

	if dec.More() {
		return nil, refuse(ReasonJSON, "data after the world object")
	}

	again, err := json.Marshal(&w)
	if err != nil {
		return nil, refuse(ReasonField, "re-encoding what was read: %v", err)
	}

	var want, have interface{}

	if err := json.Unmarshal(again, &want); err != nil {
		return nil, refuse(ReasonField, "re-encoding what was read: %v", err)
	}

	if err := json.Unmarshal(data, &have); err != nil {
		return nil, refuse(ReasonJSON, "%v", err)
	}

	if path := missingKey(want, have, ""); path != "" {
		return nil, refuse(ReasonFieldMissing, "%s is missing: this build writes it, and a file without it is of an older shape "+
			"whose version was not bumped (B3-8) -- it would load as zero", path)
	}

	if err := w.Check(); err != nil {
		return nil, err
	}

	return &w, nil
}

// VersionOf is the version a file holds, as written, without reading the rest:
// Version, written as a whole number, for a file this build may read; another
// number or token for one it may not; "absent" for a JSON object with no
// version; and "" for bytes that are not a JSON object at all.
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
// and are B4's (D4) -- and SaveWorld's, which runs every one of them on the
// snapshots it takes before it writes (the B3 review, B2).
//
// Every refusal is a *FileError whose Reason names the one rule that refused
// it (the B3 review, A2): a test of a rule asserts ITS reason, so a rule that
// is deleted turns its case red even where another rule would still have
// refused the file.
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

	if err == nil {
		err = w.checkFog()
	}

	return err
}

// checkFog (fog of war F3): the fog block is one Fog.Snapshot could have
// written -- empty, or a grid of exactly its size in canonical base64 -- and a
// grid that is not empty was explored on the file's own map (the F1 review's
// C6). Whether it fits the map the load builds is the load's (Fog.Validate);
// a map that is not the file's is refused before, MAP (D5).
func (w *World) checkFog() error {
	if err := w.Fog.Check(); err != nil {
		return refuse(ReasonFog, "%v", err)
	}

	if !w.Fog.Empty() && w.Fog.Map != w.Map.SHA {
		return refuse(ReasonFog, "the fog grid was explored on map %q and the file was saved on map %q", w.Fog.Map, w.Map.SHA)
	}

	return nil
}

func (w *World) checkHeader() error {
	if w.Version != Version {
		return refuse(ReasonVersion, "version %d", w.Version)
	}

	return nil
}

func (w *World) checkMap() error {
	switch {
	case w.Map.Generated && (w.Map.Path != "" || w.Map.SHA != ""):
		return refuse(ReasonMap, "map: a generated world has no path or sha (%q, %q)", w.Map.Path, w.Map.SHA)
	case !w.Map.Generated && w.Map.Path == "":
		return refuse(ReasonMap, "map: an authored world names its .tmj")
	case !w.Map.Generated && !shaHex.MatchString(w.Map.SHA):
		return refuse(ReasonMap, "map: %q is not the SHA-256 of %s in hex", w.Map.SHA, w.Map.Path)
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
		{d2rand.StreamCombatClock, w.RNG.CombatClock, w.Combat.Clock.RNG},
		{d2rand.StreamRising, w.RNG.Rising, w.Rising.RNG},
	}

	for _, c := range copies {
		if c.copy != c.block {
			return refuse(ReasonRNGCopy, "rng.%s is %+v and the %s block's own stream is %+v", c.name, c.copy, c.name, c.block)
		}
	}

	streams := []struct {
		name string
		s    d2rand.StreamState
	}{
		{d2rand.StreamWorld, w.RNG.World},
		{d2rand.StreamSpawns, w.RNG.Spawns},
		{d2rand.StreamCombat, w.RNG.Combat},
		{d2rand.StreamCombatClock, w.RNG.CombatClock},
		{d2rand.StreamRising, w.RNG.Rising},
	}

	for _, st := range streams {
		if err := st.s.Check(w.Seed, st.name); err != nil {
			return refuse(ReasonRNGStream, "%v", err)
		}
	}

	return nil
}

// heroDirections bounds a facing: Diablo II's art has at most 64 directions,
// and a hero's body uses 16 or fewer.
const heroDirections = 64

func (w *World) checkHero() error {
	h := w.Hero

	// WHOSE FILE THIS IS (the B3 review, A1): a world file names its hero, so
	// a load can refuse one that is not the .od2's it sits beside
	// (CheckHeroFile).
	if h.Name == "" || !isHeroClass(h.Class) {
		return refuse(ReasonHeroIdentity, "hero: name %q, class %q -- a world file names the hero it is his", h.Name, h.Class)
	}

	if err := finite("hero", h.X, h.Y, h.Pos[0], h.Pos[1], h.Stamina); err != nil {
		return refuse(ReasonHeroFinite, "%v", err)
	}

	if err := samePlace("hero", h.X, h.Y, h.Pos); err != nil {
		return refuse(ReasonHeroPlace, "%v", err)
	}

	// A dead hero is never saved (DEAD, the 12 Sep ruling).
	if h.Health <= 0 {
		return refuse(ReasonHeroDead, "hero: health %d -- a dead hero is never saved", h.Health)
	}

	if h.Facing < 0 || h.Facing >= heroDirections {
		return refuse(ReasonHeroFacing, "hero: facing %d is no direction (0-%d)", h.Facing, heroDirections-1)
	}

	if h.Stamina < 0 {
		return refuse(ReasonHeroStamina, "hero: stamina %v", h.Stamina)
	}

	return nil
}

// isHeroClass reports whether class names one of Diablo II's seven classes, as
// d2enum.Hero spells it (the .od2's heroType, by name).
func isHeroClass(class string) bool {
	for h := d2enum.HeroBarbarian; h <= d2enum.HeroDruid; h++ {
		if h.String() == class {
			return true
		}
	}

	return false
}

// embeddedSidecar is what the world file reads of the sidecar it carries.
type embeddedSidecar struct {
	Version    int             `json:"version"`
	Generation string          `json:"generation"`
	Kit        json.RawMessage `json:"kit"`
}

// checkSidecar: the embedded sidecar is the kit file's document, of the
// version d2items writes (d2items.SidecarVersion, not a literal: the B3
// review's C item), with a kit -- and of THIS save's generation: the saved_at
// the world file carries, which the save writes into both (the B3 review,
// B7).
func (w *World) checkSidecar() error {
	var sc embeddedSidecar

	if err := json.Unmarshal(w.Sidecar, &sc); err != nil {
		return refuse(ReasonSidecar, "sidecar: %v", err)
	}

	if sc.Version != d2items.SidecarVersion || len(sc.Kit) == 0 || string(sc.Kit) == "null" {
		return refuse(ReasonSidecar, "sidecar: version %d with kit %s -- the kit file's document is version %d and has a kit",
			sc.Version, sc.Kit, d2items.SidecarVersion)
	}

	if sc.Generation != w.SavedAt {
		return refuse(ReasonGeneration, "sidecar: generation %q, and this file was saved at %q -- the save writes the one into the other",
			sc.Generation, w.SavedAt)
	}

	return nil
}

func (w *World) checkClock() error {
	if e := w.Clock.Elapsed; math.IsNaN(e) || math.IsInf(e, 0) || e < 0 {
		return refuse(ReasonClock, "clock: elapsed %v", e)
	}

	return nil
}

// nativePair is how a load knows a native entity: who stands in for him and
// where the map put him (B3-6; the B3 review, B3).
type nativePair struct {
	nameKey string
	x, y    float64
}

func (w *World) checkEntities() error {
	seen := map[string]bool{}
	natives := map[nativePair]string{}

	for i, e := range w.Entities {
		what := fmt.Sprintf("entities[%d] %q", i, e.ID)

		switch {
		case e.ID == "" || e.ID == d2saveref.Player:
			return refuse(ReasonEntityID, "%s: not an entity id", what)
		case seen[e.ID]:
			return refuse(ReasonEntityTwice, "%s: saved twice", what)
		case i > 0 && w.Entities[i-1].ID > e.ID:
			return refuse(ReasonEntityOrder, "%s: the list is sorted by id", what)
		}

		seen[e.ID] = true

		switch e.Kind {
		case KindNPC:
			if e.Monstat == "" || e.Creature != "" {
				return refuse(ReasonEntityKind, "%s: an npc names its monstat and no creature (%q, %q)", what, e.Monstat, e.Creature)
			}
		case KindCreature:
			if e.Creature == "" || e.Monstat != "" {
				return refuse(ReasonEntityKind, "%s: a creature names its bestiary id and no monstat (%q, %q)", what, e.Creature, e.Monstat)
			}
		default:
			return refuse(ReasonEntityKind, "%s: kind %q is not %s or %s", what, e.Kind, KindNPC, KindCreature)
		}

		if e.Native != (e.Born != nil) {
			return refuse(ReasonEntityNative, "%s: a native entity, and only one, carries where the map put it (native %v, born %v)",
				what, e.Native, e.Born)
		}

		nums := []float64{e.X, e.Y, e.Motion.Pos[0], e.Motion.Pos[1], e.Motion.Target[0], e.Motion.Target[1],
			e.Motion.Velocity[0], e.Motion.Velocity[1], e.Motion.Speed}
		for _, p := range e.Motion.Path {
			nums = append(nums, p[0], p[1])
		}

		if e.Born != nil {
			nums = append(nums, e.Born[0], e.Born[1])
		}

		// A held action's time into its frame (BUG-87).
		if e.Motion.ActionAt != nil {
			nums = append(nums, e.Motion.ActionAt.Elapsed)
		}

		if err := finite(what, nums...); err != nil {
			return refuse(ReasonEntityFinite, "%v", err)
		}

		// And a held action's point is one a play can have (the BUG-87
		// review's B1, BUG-91: an NPC's swing at -1.0 s crashed the game): no
		// negative frame, no time below the floor. Its ceiling -- one frame's
		// length -- is the art's, which this package does not know; the
		// load's restore holds it (d2mapentity's ResumeMotion, d2asset's
		// SetProgress), and refuses ENTITY or NATIVES.
		if at := e.Motion.ActionAt; at != nil {
			if err := at.Check(); err != nil {
				return refuse(ReasonEntityFinite, "%s: its held %q %v", what, e.Motion.Action, err)
			}
		}

		if err := samePlace(what, e.X, e.Y, e.Motion.Pos); err != nil {
			return refuse(ReasonEntityPlace, "%v", err)
		}

		// B4b re-keys a native by (name_key, born), so no two may share the
		// pair: the load could not tell them apart (the B3 review, B3).
		if e.Native {
			pair := nativePair{nameKey: e.NameKey, x: e.Born[0], y: e.Born[1]}
			if other, dup := natives[pair]; dup {
				return refuse(ReasonNativeTwice, "%s and %q are both %q born at %v,%v: a load re-keys a native by name_key and born, and could not tell them apart",
					what, other, e.NameKey, e.Born[0], e.Born[1])
			}

			natives[pair] = e.ID
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
			return refuse(ReasonBodyTwice, "%s: saved twice", what)
		case i > 0 && w.Bodies[i-1].ID > b.ID:
			return refuse(ReasonBodyOrder, "%s: the list is sorted by id", what)
		case !entities[b.ID]:
			return refuse(ReasonBodyOrphan, "%s: a body with no entity in the file", what)
		case b.MaxHealth < 1 || b.Health < 0 || b.Health > b.MaxHealth:
			return refuse(ReasonBodyHealth, "%s: health %d of %d", what, b.Health, b.MaxHealth)
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
				return refuse(ReasonMemberGone, "spawns %s: %s is gone, and the file carries an entity with his id", g.ID, m.ID)
			case m.Gone:
			case !ok:
				return refuse(ReasonMemberMissing, "spawns %s: member %s is on the map and not in the entity list", g.ID, m.ID)
			case e.X != m.X || e.Y != m.Y:
				return refuse(ReasonMemberPlace, "spawns %s: member %s stood at %v,%v and his entity at %v,%v", g.ID, m.ID, m.X, m.Y, e.X, e.Y)
			}
		}
	}

	for _, wt := range w.Notice.Watches {
		if !isEntity(wt.Watcher) || !isQuarry(wt.Target) {
			return refuse(ReasonWatch, "notice: watcher %q or target %q is not in the entity list", wt.Watcher, wt.Target)
		}
	}

	for _, c := range w.Pursuit.Chases {
		if !isEntity(c.Hunter) || !isQuarry(c.Quarry) {
			return refuse(ReasonChase, "pursuit: hunter %q or quarry %q is not in the entity list", c.Hunter, c.Quarry)
		}
	}

	for _, sq := range w.Squads.Squads {
		for _, m := range sq.Members {
			if !isQuarry(m.Entity) {
				return refuse(ReasonSquadModel, "squads %s: model %q is not in the entity list", sq.ID, m.Entity)
			}
		}
	}

	return nil
}

func (w *World) checkScene() error {
	s := w.Scene
	if err := finite("scene", s.WatchStood, s.WatchClock); err != nil {
		return refuse(ReasonSceneFinite, "%v", err)
	}

	if s.WatchStood < 0 {
		return refuse(ReasonSceneWatch, "scene: watch_stood %v", s.WatchStood)
	}

	for _, stage := range []d2world.Stage{d2world.StageNight, d2world.StageDawn, d2world.StageDay, d2world.StageDusk} {
		if stage.String() == s.LastStage {
			return nil
		}
	}

	return refuse(ReasonSceneStage, "scene: last_stage %q is no stage", s.LastStage)
}

func (w *World) entityIDs() map[string]bool {
	ids := make(map[string]bool, len(w.Entities))
	for _, e := range w.Entities {
		ids[e.ID] = true
	}

	return ids
}

// CheckHeroFile refuses a world file that is not the hero's whose .od2 it sits
// beside (the B3 review, A1): his name and class must be the .od2's heroName
// and heroType. B4's load calls it in step 1, before anything is read into the
// game, and sets a mismatched file aside (rule 7).
//
// Name and class are all the identity a hero has: there is no id or creation
// stamp in the .od2. Two heroes of one name and class are told apart by their
// number (N.od2), which the hero screen now never hands out while a file of a
// deleted hero is left (d2hero.DeleteHero, firstFreeFileName).
func (w *World) CheckHeroFile(od2 []byte) error {
	var hero struct {
		Name string      `json:"heroName"`
		Type d2enum.Hero `json:"heroType"`
	}

	if err := json.Unmarshal(od2, &hero); err != nil {
		return refuse(ReasonPairHero, "his .od2 is not a hero save: %v", err)
	}

	if hero.Name != w.Hero.Name || hero.Type.String() != w.Hero.Class {
		return refuse(ReasonPairHero, "this world file is %s the %s's, and the .od2 beside it is %s the %s's",
			w.Hero.Name, w.Hero.Class, hero.Name, hero.Type.String())
	}

	return nil
}

// SameMoment refuses a world file whose sidecar file is not of its generation
// (the B3 review, B7): the sidecar's generation must be this file's saved_at.
// A save writes the world file first, then his .od2, then his sidecar with
// the same generation, and every kit save after it carries the generation
// forward; so a sidecar of another generation means a save was cut off between
// its files. B4's load calls it in step 1 and, on a refusal, falls back per
// rule 7.
//
// A sidecar of NO generation is the other way the pair parts (the B4a review,
// A2): a game that did not resume a world file carries none in its kit saves
// (d2gamescreen's bindKit), so his sidecar has moved on past this file, which
// is older than what he has -- and is refused as such, never resumed over it.
func (w *World) SameMoment(sidecar []byte) error {
	var sc embeddedSidecar

	if err := json.Unmarshal(sidecar, &sc); err != nil {
		return refuse(ReasonPairMoment, "his sidecar is not a kit file: %v", err)
	}

	if sc.Generation == "" && w.SavedAt != "" {
		return refuse(ReasonPairMoment, "his sidecar is of no world save -- a game that did not resume this world file of %q "+
			"has written it since, so this file is older than what he has", w.SavedAt)
	}

	if sc.Generation != w.SavedAt {
		return refuse(ReasonPairMoment, "his sidecar is of generation %q and this world file of %q: a save was cut off between its files",
			sc.Generation, w.SavedAt)
	}

	return nil
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

// missingKey is the first key, as a path ("hero.facing",
// "entities[1].motion.dir"), that the file as this build would write it has
// and the file as it was read does not -- or "" when there is none.
//
// THIS IS WHAT HOLDS B3-8 (the B3 review, B1): "any change to the file's
// shape bumps Version". encoding/json reads a missing field as its zero, so a
// file from before a field existed would load that field as zero and the
// resumed night would quietly diverge. Decode re-encodes what it read and
// refuses any key the re-encoding writes that the file lacks: a field added
// without a bump turns every older file into a refusal, which
// TestTheV1ShapeIsTheGoldenFile turns into a red test the day it happens.
// (A field REMOVED is refused already: the old file's key is unknown.) An
// omitempty field absent from both is not missing, and a null where an
// object is written is.
func missingKey(want, have interface{}, path string) string {
	switch w := want.(type) {
	case map[string]interface{}:
		h, ok := have.(map[string]interface{})
		if !ok {
			return path
		}

		keys := make([]string, 0, len(w))
		for k := range w {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			sub := k
			if path != "" {
				sub = path + "." + k
			}

			hv, ok := h[k]
			if !ok {
				return sub
			}

			if p := missingKey(w[k], hv, sub); p != "" {
				return p
			}
		}
	case []interface{}:
		h, ok := have.([]interface{})
		if !ok {
			return path
		}

		for i := range w {
			if i >= len(h) {
				return fmt.Sprintf("%s[%d]", path, i)
			}

			if p := missingKey(w[i], h[i], fmt.Sprintf("%s[%d]", path, i)); p != "" {
				return p
			}
		}
	}

	return ""
}
