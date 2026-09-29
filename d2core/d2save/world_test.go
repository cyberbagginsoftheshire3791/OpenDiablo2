package d2save

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// b3Seed is past 2^53, where a float64 stops holding every integer: the seed a
// wall-clock game has (UnixNano, ~1.8e18) is up here.
const b3Seed int64 = 1<<62 + 12345

// b3Worldish returns the world point of a sub-tile position, as the engine
// computes it (GetPositionF).
func b3World(pos [2]float64) (float64, float64) {
	p := d2vector.NewPosition(pos[0], pos[1])
	w := p.World()

	return w.X(), w.Y()
}

func b3Stream(name string, draws uint64) d2rand.StreamState {
	seed, err := d2rand.SeedFor(b3Seed, name)
	if err != nil {
		panic(err)
	}

	return d2rand.StreamState{Seed: seed, Draws: draws}
}

// b3Fixture is a world file with every block holding something: a hero with a
// lit torch, a pack of two (one on the map, one gone), a watch and a chase, a
// villager, a body, a corpse, a fight's counters, and every stream moved.
func b3Fixture() *World {
	wolf, villager := "0a-wolf", "7f-villager"

	wolfPos := [2]float64{531.2, 612.7}
	wx, wy := b3World(wolfPos)

	vilPos := [2]float64{505, 590}
	vx, vy := b3World(vilPos)

	heroPos := [2]float64{540.4, 611.3}
	hx, hy := b3World(heroPos)

	spawns, combat, rising := b3Stream(d2rand.StreamSpawns, 17), b3Stream(d2rand.StreamCombat, 230), b3Stream(d2rand.StreamRising, 9)
	// The raid's R1: the fights he is not in draw from combat's second stream.
	clock := b3Stream(d2rand.StreamCombatClock, 31)

	return &World{
		Version: Version,
		Build:   "save-b3 test",
		SavedAt: "2026-09-28T21:14:05.123Z",
		Map:     Map{Path: "/data/strigoi/maps/village.tmj", SHA: strings.Repeat("ab", 32)},
		Seed:    b3Seed,
		RNG: RNG{
			World: b3Stream(d2rand.StreamWorld, 4242), Spawns: spawns, Combat: combat, Rising: rising, CombatClock: clock,
			UUID: &UUIDStream{Seed: b3Seed, Bytes: 16 * 9},
		},
		Hero:    Hero{Name: "Saver", Class: "Amazon", X: hx, Y: hy, Pos: heroPos, Facing: 3, Health: 187, Stamina: 42.5, Run: true},
		Sidecar: json.RawMessage(`{"version": 1, "generation": "2026-09-28T21:14:05.123Z", "kit": {"worn": {"off": "torch"}}, "progress": {"xp": 50}}`),
		Clock:   d2world.ClockSnapshot{Elapsed: 1092.5},
		Light: d2world.LightSnapshot{NextID: 3, Sources: []d2world.LightSourceSnapshot{
			{ID: 2, Kind: d2world.SourceTorch, Burn: 41.5, Lit: true, Carried: true},
		}},
		Squads: d2world.SquadsSnapshot{NextID: 2, Selected: "s:1", Squads: []d2world.SquadSnapshot{{
			ID: "s:1", Owner: "player", Ordinal: 1, Morale: 100,
			Members: []d2world.SquadMemberSnapshot{{Entity: "player"}},
			Meters:  d2world.MetersSnapshot{Food: 71.25, Water: 64, Fatigue: 12.5, Activity: d2world.ActivityIdle, Damage: 0.25},
		}}},
		Spawns: d2world.SpawnsSnapshot{
			NextID: 2, SinceCheck: 3.5, OpenBodies: 1, Checks: 40, Rolls: 40, Spawned: 2, Released: 1, RNG: spawns,
			Groups: []d2world.SpawnGroupSnapshot{{
				ID: "g:1", Row: "wolves", Code: "zombie1", Morale: 60, BornAt: 1030, Band: 1, Stage: "night",
				Weight: 1.5, Spawned: 2, BornWhere: [][2]float64{{100, 120}, {101, 121}},
				Members: []d2world.SpawnMemberSnapshot{{ID: wolf, X: wx, Y: wy}, {ID: "0b-gone", X: 90, Y: 91, Gone: true}},
			}},
		},
		Spawner: Spawner{Arrival: 2},
		Notice: d2world.NoticeSnapshot{Checks: 55, Notices: 1, Watches: []d2world.WatchSnapshot{
			{Watcher: wolf, Target: "player", Sees: true, Noticed: true, Distance: 6.25, Reach: 9, Checks: 55, Notices: 1},
		}},
		Pursuit: d2world.PursuitSnapshot{Solves: 3, Chases: []d2world.ChaseSnapshot{
			{Hunter: wolf, Quarry: "player", SolvedAtX: 108, SolvedAtY: 122, SolvedDistance: 6.25, SinceSolve: 0.5, Reachable: true, Solves: 3},
		}},
		Corpses: d2world.CorpsesSnapshot{
			Bodies:  []d2world.CorpseSnapshot{{ID: "dead:1", Row: "men", Class: d2world.CorpseHuman, State: d2world.CorpseFresh, X: 99.5, Y: 100.5, Was: "a carter"}},
			RisenAs: map[string]string{}, Walker: map[string]string{}, Last: map[string]string{},
		},
		Rising: d2world.RisingSnapshot{Accrued: 2.5, LastBand: 1, LastStage: "night", Rolls: 4, RNG: rising},
		Combat: d2world.CombatSnapshot{NextID: 3, Started: 2, Ended: 2, Rounds: 7, Actions: 12, EndedReason: "enemies_dead", EndedEnemiesDead: 2, RNG: combat,
			Clock: d2world.CombatClockSnapshot{NextID: 2, RNG: clock, Started: 1, Ended: 1, Rounds: 4, Actions: 9,
				EndedReason: "quarry_dead", EndedQuarryDead: 1, Released: 1}},
		Bodies: []Body{{ID: wolf, Health: 40, MaxHealth: 181}},
		Entities: []Entity{
			{ID: wolf, Kind: KindCreature, Creature: "wolf", NameKey: "Wolf", X: wx, Y: wy, Motion: d2mapentity.Motion{
				Pos: wolfPos, Target: [2]float64{533, 612.5}, Velocity: [2]float64{0.5, -0.1},
				Path: [][2]float64{{535, 612.5}, {540, 612.5}}, Speed: 6, Dir: 3, Mode: "walk",
			}},
			{ID: villager, Kind: KindNPC, Native: true, Monstat: "warriv", NameKey: "Warriv", Born: &[2]float64{vx, vy}, X: vx, Y: vy,
				Motion: d2mapentity.Motion{Pos: vilPos, Target: vilPos, Path: [][2]float64{}, Speed: 3, Mode: "NU"}},
		},
		Scene: Scene{WatchStood: 30.25, FieldDead: []string{"dead:1", "dead:2"}, DawnPaidDay: 1, LastStage: "night", WatchClock: 1080, WatchClockSet: true},
	}
}

func b3Encode(t *testing.T, w *World, omit ...string) []byte {
	t.Helper()

	data, err := Encode(w, omit)
	require.NoError(t, err)

	return data
}

// The fixture is a file a load reads: the control every refusal below is
// measured against.
func TestTheFixtureIsAReadableFile(t *testing.T) {
	w := b3Fixture()
	require.NoError(t, w.Check())

	back, err := Decode(b3Encode(t, w))
	require.NoError(t, err)

	// The sidecar is carried as JSON, re-indented inside the file: the same
	// document, compared compact.
	require.Equal(t, b3Compact(t, w.Sidecar), b3Compact(t, back.Sidecar))

	back.Sidecar = w.Sidecar
	require.Equal(t, w, back, "what is read is what was written")
}

func b3Compact(t *testing.T, raw []byte) string {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, json.Compact(&buf, raw))

	return buf.String()
}

// Blocks is World's JSON keys in World's order, so the file's order and the
// struct cannot drift apart.
func TestBlocksAreTheWorldsFields(t *testing.T) {
	typ := reflect.TypeOf(World{})
	require.Equal(t, len(Blocks), typ.NumField())

	for i := 0; i < typ.NumField(); i++ {
		require.Equal(t, Blocks[i], strings.Split(typ.Field(i).Tag.Get("json"), ",")[0], "field %d", i)
	}
}

// THE ROUND TRIP IS BYTE-STABLE: save, decode, encode again, and the bytes are
// the bytes (build plan section 4's act 5 rests on this: a second save of one
// moment is the first, but for saved_at). And the top-level keys are in
// Blocks' order.
func TestEncodeDecodeEncodeIsByteStable(t *testing.T) {
	first := b3Encode(t, b3Fixture())

	w, err := Decode(first)
	require.NoError(t, err)

	second := b3Encode(t, w)
	require.True(t, bytes.Equal(first, second), "re-encoded:\n%s\nfirst:\n%s", second, first)

	// Encoding is deterministic: the same World twice is one file.
	require.Equal(t, first, b3Encode(t, b3Fixture()))

	var keys []string

	dec := json.NewDecoder(bytes.NewReader(first))
	_, _ = dec.Token()

	for dec.More() {
		tok, err := dec.Token()
		require.NoError(t, err)
		keys = append(keys, tok.(string))

		var skip json.RawMessage
		require.NoError(t, dec.Decode(&skip))
	}

	require.Equal(t, Blocks, keys, "every block, in order")
	require.True(t, bytes.HasSuffix(first, []byte("}\n")))
}

// SEEDS PAST 2^53 SURVIVE, both ways a reader takes them: exactly through the
// typed decode, and as an exact decimal STRING through a float64 reader (a
// playtest's mustNum), where a JSON number would have come back another seed.
func TestSeedsPast2To53Survive(t *testing.T) {
	data := b3Encode(t, b3Fixture())

	w, err := Decode(data)
	require.NoError(t, err)
	require.Equal(t, b3Seed, w.Seed)
	require.Equal(t, b3Seed, w.RNG.World.Seed)
	require.Equal(t, b3Seed, w.RNG.UUID.Seed)

	var loose map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &loose))

	want := strconv.FormatInt(b3Seed, 10)
	require.Equal(t, want, loose["seed"], "the seed is an exact string to a float64 reader")
	require.Equal(t, want, loose["rng"].(map[string]interface{})["world"].(map[string]interface{})["seed"])
	require.Equal(t, want, loose["rng"].(map[string]interface{})["uuid"].(map[string]interface{})["seed"])

	// The premise: the same seed as a JSON number, read as a float64, is a
	// different seed. Without it the string above would be decoration.
	var asNumber map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{"seed": `+want+`}`), &asNumber))
	require.NotEqual(t, b3Seed, int64(asNumber["seed"].(float64)))
}

// ANY VERSION BUT THIS BUILD'S IS REFUSED, as a *VersionError naming it,
// before a single other block is read.
//
// THE VERSION FOLLOWS Version (the raid's R0.5, 29 Sep 2026). The test was
// written at version 1 and named for it; every literal that stood for the
// build's own version now reads Version (cur), and the one that stood for
// the next build's reads Version+1, so each case asserts what it asserted at
// version 1 and still does after a bump. The name is kept: the notes and the
// raid brief find it by it.
func TestAVersionOtherThanOneIsRefused(t *testing.T) {
	cur, next := strconv.Itoa(Version), strconv.Itoa(Version+1)

	data := b3Encode(t, b3Fixture())
	require.Contains(t, string(data), "\n  \"version\": "+cur+",\n")

	for _, tc := range []struct{ version, named string }{
		{"0", "0"}, {next, next}, {"-1", "-1"}, {`"` + cur + `"`, `"` + cur + `"`}, {cur + ".0", cur + ".0"},
		{"99999999999", "99999999999"}, {"null", "null"},
	} {
		bad := bytes.Replace(data, []byte("\"version\": "+cur+","), []byte("\"version\": "+tc.version+","), 1)

		_, err := Decode(bad)
		require.ErrorIs(t, err, ErrWorldVersion, "version %s", tc.version)

		var ve *VersionError
		require.True(t, errors.As(err, &ve))
		require.Equal(t, tc.named, ve.Version)
		require.Equal(t, tc.named, VersionOf(bad))
	}

	var noVersion map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &noVersion))
	delete(noVersion, "version")

	absent, err := json.Marshal(noVersion)
	require.NoError(t, err)

	_, err = Decode(absent)
	require.ErrorIs(t, err, ErrWorldVersion)
	require.Equal(t, "absent", VersionOf(absent))

	// A file of this build's version is not refused on its version (the
	// control).
	_, err = Decode(data)
	require.NoError(t, err)
	require.Equal(t, cur, VersionOf(data))
	require.Equal(t, "", VersionOf([]byte("not json")))
}

// EVERY BLOCK IS REQUIRED: one missing, or null, is refused -- never read as
// an empty system. A block or a field this build does not know is refused too.
func TestEveryBlockIsRequired(t *testing.T) {
	w := b3Fixture()

	for _, name := range Blocks {
		if name == "version" {
			continue
		}

		_, err := Decode(b3Encode(t, w, name))
		require.ErrorIs(t, err, ErrWorldFile, "without %q", name)
		require.Equal(t, ReasonBlockMissing, ReasonOf(err), "without %q", name)
		require.Contains(t, err.Error(), name)

		nulled := b3SetBlock(t, b3Encode(t, w), name, "null")
		_, err = Decode(nulled)
		require.ErrorIs(t, err, ErrWorldFile, "%q null", name)
		require.Equal(t, ReasonBlockNull, ReasonOf(err), "%q null", name)
	}

	_, err := Decode(b3SetBlock(t, b3Encode(t, w), "weather", `{"rain": true}`))
	require.Equal(t, ReasonBlockUnknown, ReasonOf(err), "an unknown block")

	_, err = Decode(bytes.Replace(b3Encode(t, w), []byte(`"elapsed": 1092.5`), []byte(`"elapsed": 1092.5, "rate": 2`), 1))
	require.Equal(t, ReasonField, ReasonOf(err), "an unknown field inside a block")

	_, err = Decode(append(b3Encode(t, w), []byte(`{}`)...))
	require.Equal(t, ReasonJSON, ReasonOf(err), "data after the object")

	for _, junk := range []string{"", "[]", "null", "42", "{"} {
		_, err = Decode([]byte(junk))
		require.Error(t, err, "%q", junk)
	}
}

// b3SetBlock replaces (or adds) one top-level block's raw JSON.
func b3SetBlock(t *testing.T, data []byte, name, raw string) []byte {
	t.Helper()

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))

	top[name] = json.RawMessage(raw)

	out, err := json.Marshal(top)
	require.NoError(t, err)

	return out
}

// OMIT drops exactly the named blocks, keeps the rest in order, and refuses a
// name that is not a block.
func TestEncodeOmit(t *testing.T) {
	w := b3Fixture()

	for _, name := range Blocks {
		data := b3Encode(t, w, name)

		var top map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &top))
		require.Len(t, top, len(Blocks)-1)
		require.NotContains(t, top, name)
	}

	data := b3Encode(t, w, "corpses", "entities")

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))
	require.Len(t, top, len(Blocks)-2)

	_, err := Encode(w, []string{"weather"})
	require.Error(t, err)

	_, err = Encode(nil, nil)
	require.ErrorIs(t, err, ErrWorldFile)
}

// CHECK refuses a file whose blocks disagree. Every case is the fixture -- a
// file Check accepts -- with one thing wrong, and EACH NAMES THE RULE THAT
// MUST REFUSE IT (the B3 review, A2). Asserting only ErrWorldFile let a rule
// be deleted with its case still green, because another rule refused the same
// file: the entity's own place check, deleted, left "an entity off its
// motion" refused by the spawns member's place check instead.
func TestCheckRefusesWhatNoLoadCouldRestore(t *testing.T) {
	cases := map[string]struct {
		reason string
		mutate func(w *World)
	}{
		// The next build's version (R0.5: Version+1, not the literal 2).
		"the next version in a struct":     {ReasonVersion, func(w *World) { w.Version = Version + 1 }},
		"generated with a path":            {ReasonMap, func(w *World) { w.Map.Generated = true }},
		"authored with no path":            {ReasonMap, func(w *World) { w.Map.Path = "" }},
		"authored with no sha":             {ReasonMap, func(w *World) { w.Map.SHA = "" }},
		"a sha that is not hex":            {ReasonMap, func(w *World) { w.Map.SHA = strings.Repeat("zz", 32) }},
		"rng.spawns is not the spawns'":    {ReasonRNGCopy, func(w *World) { w.RNG.Spawns.Draws++ }},
		"rng.combat is not the combat's":   {ReasonRNGCopy, func(w *World) { w.RNG.Combat.Draws++ }},
		"rng.rising is not the rising's":   {ReasonRNGCopy, func(w *World) { w.Rising.RNG.Draws++ }},
		"the world stream on another seed": {ReasonRNGStream, func(w *World) { w.RNG.World.Seed++ }},
		"two streams swapped": {ReasonRNGStream, func(w *World) {
			w.RNG.Combat, w.RNG.Rising = w.RNG.Rising, w.RNG.Combat
			w.Combat.RNG, w.Rising.RNG = w.RNG.Combat, w.RNG.Rising
		}},
		"a stream past MaxDraws":       {ReasonRNGStream, func(w *World) { w.RNG.World.Draws = d2rand.MaxDraws + 1 }},
		"a hero with no name":          {ReasonHeroIdentity, func(w *World) { w.Hero.Name = "" }},
		"a hero of no class":           {ReasonHeroIdentity, func(w *World) { w.Hero.Class = "Janissary" }},
		"a hero of class None":         {ReasonHeroIdentity, func(w *World) { w.Hero.Class = "" }},
		"the hero off his own pos":     {ReasonHeroPlace, func(w *World) { w.Hero.X += 0.2 }},
		"a dead hero":                  {ReasonHeroDead, func(w *World) { w.Hero.Health = 0 }},
		"a NaN hero":                   {ReasonHeroFinite, func(w *World) { w.Hero.Pos[0] = math.NaN() }},
		"a NaN stamina":                {ReasonHeroFinite, func(w *World) { w.Hero.Stamina = math.NaN() }},
		"a facing that is no facing":   {ReasonHeroFacing, func(w *World) { w.Hero.Facing = 64 }},
		"a negative facing":            {ReasonHeroFacing, func(w *World) { w.Hero.Facing = -1 }},
		"a negative stamina":           {ReasonHeroStamina, func(w *World) { w.Hero.Stamina = -0.5 }},
		"a sidecar of version 2":       {ReasonSidecar, func(w *World) { w.Sidecar = b3Sidecar(w, 2, `{}`) }},
		"a sidecar with no kit":        {ReasonSidecar, func(w *World) { w.Sidecar = b3Sidecar(w, 1, `null`) }},
		"a sidecar that is not JSON":   {ReasonSidecar, func(w *World) { w.Sidecar = json.RawMessage(`[1]`) }},
		"a sidecar of another save":    {ReasonGeneration, func(w *World) { w.SavedAt = "2026-09-28T21:14:06Z" }},
		"a sidecar of no save":         {ReasonGeneration, func(w *World) { w.Sidecar = json.RawMessage(`{"version": 1, "kit": {}}`) }},
		"a negative clock":             {ReasonClock, func(w *World) { w.Clock.Elapsed = -1 }},
		"an entity saved twice":        {ReasonEntityTwice, func(w *World) { w.Entities = append(w.Entities, w.Entities[1]) }},
		"entities out of order":        {ReasonEntityOrder, func(w *World) { w.Entities[0], w.Entities[1] = w.Entities[1], w.Entities[0] }},
		"an entity called player":      {ReasonEntityID, func(w *World) { w.Entities[1].ID = "player" }},
		"an entity of no kind":         {ReasonEntityKind, func(w *World) { w.Entities[0].Kind = "object" }},
		"an npc with no monstat":       {ReasonEntityKind, func(w *World) { w.Entities[1].Monstat = "" }},
		"a creature with no entry":     {ReasonEntityKind, func(w *World) { w.Entities[0].Creature = "" }},
		"a creature with a monstat":    {ReasonEntityKind, func(w *World) { w.Entities[0].Monstat = "zombie1" }},
		"a native with no birthplace":  {ReasonEntityNative, func(w *World) { w.Entities[1].Born = nil }},
		"a birthplace on a non-native": {ReasonEntityNative, func(w *World) { w.Entities[0].Born = &[2]float64{1, 1} }},
		"two natives of one name and birthplace": {ReasonNativeTwice, func(w *World) {
			twin := w.Entities[1]
			twin.ID = "7g-twin"
			w.Entities = append(w.Entities, twin)
		}},
		"an entity off its motion":  {ReasonEntityPlace, func(w *World) { w.Entities[0].X += 1 }},
		"an infinite path":          {ReasonEntityFinite, func(w *World) { w.Entities[0].Motion.Path[1][0] = math.Inf(1) }},
		"a body with no entity":     {ReasonBodyOrphan, func(w *World) { w.Bodies[0].ID = "0z-nobody" }},
		"a body over its max":       {ReasonBodyHealth, func(w *World) { w.Bodies[0].Health = 182 }},
		"a body with no max":        {ReasonBodyHealth, func(w *World) { w.Bodies[0].MaxHealth = 0 }},
		"a body saved twice":        {ReasonBodyTwice, func(w *World) { w.Bodies = append(w.Bodies, w.Bodies[0]) }},
		"bodies out of order":       {ReasonBodyOrder, func(w *World) { w.Bodies = []Body{{ID: w.Entities[1].ID, Health: 1, MaxHealth: 1}, w.Bodies[0]} }},
		"a member not in the list":  {ReasonMemberMissing, func(w *World) { w.Spawns.Groups[0].Members[0].ID = "0z-nobody" }},
		"a member off his entity":   {ReasonMemberPlace, func(w *World) { w.Spawns.Groups[0].Members[0].X += 0.2 }},
		"a gone member on the map":  {ReasonMemberGone, func(w *World) { w.Spawns.Groups[0].Members[1].ID = w.Entities[1].ID }},
		"a watcher not in the list": {ReasonWatch, func(w *World) { w.Notice.Watches[0].Watcher = "0z-nobody" }},
		"a watch on nobody":         {ReasonWatch, func(w *World) { w.Notice.Watches[0].Target = "0z-nobody" }},
		"a hunter not in the list":  {ReasonChase, func(w *World) { w.Pursuit.Chases[0].Hunter = "0z-nobody" }},
		"a chase after nobody":      {ReasonChase, func(w *World) { w.Pursuit.Chases[0].Quarry = "0z-nobody" }},
		"a squad model not in the list": {ReasonSquadModel, func(w *World) {
			w.Squads.Squads[0].Members = append(w.Squads.Squads[0].Members, d2world.SquadMemberSnapshot{Entity: "0z-nobody"})
		}},
		"a stage that is none":    {ReasonSceneStage, func(w *World) { w.Scene.LastStage = "noon" }},
		"a negative watch":        {ReasonSceneWatch, func(w *World) { w.Scene.WatchStood = -1 }},
		"an infinite watch clock": {ReasonSceneFinite, func(w *World) { w.Scene.WatchClock = math.Inf(-1) }},
	}

	for name, tc := range cases {
		w := b3Fixture()
		tc.mutate(w)

		err := w.Check()
		require.ErrorIs(t, err, ErrWorldFile, name)
		require.Equal(t, tc.reason, ReasonOf(err), "%s: refused by the wrong rule: %v", name, err)

		// And through the bytes, where the case can be written at all (a NaN
		// or an infinity cannot: encoding/json refuses to write one).
		data, err := Encode(w, nil)
		if err == nil {
			_, err = Decode(data)
			require.Equal(t, tc.reason, ReasonOf(err), "%s through Decode: %v", name, err)
		}
	}
}

// b3Sidecar is an embedded sidecar of the given version and kit, of w's
// generation.
func b3Sidecar(w *World, version int, kit string) json.RawMessage {
	return json.RawMessage(`{"version": ` + strconv.Itoa(version) + `, "generation": "` + w.SavedAt + `", "kit": ` + kit + `}`)
}

// THE FILE AND THE .OD2 BESIDE IT ARE ONE HERO'S (the B3 review, A1): a world
// file names its hero, and a load refuses it beside another hero's .od2 -- the
// deleted hero's world file the next hero's number used to inherit.
func TestAWorldFileIsOneHerosFile(t *testing.T) {
	w := b3Fixture()

	od2 := func(name string, class d2enum.Hero) []byte {
		data, err := json.Marshal(map[string]interface{}{"heroName": name, "heroType": class, "act": 1})
		require.NoError(t, err)

		return data
	}

	require.NoError(t, w.CheckHeroFile(od2("Saver", d2enum.HeroAmazon)), "his own .od2")

	for what, data := range map[string][]byte{
		"another name":    od2("Vlad", d2enum.HeroAmazon),
		"another class":   od2("Saver", d2enum.HeroPaladin),
		"not a hero save": []byte("garbage"),
	} {
		err := w.CheckHeroFile(data)
		require.ErrorIs(t, err, ErrWorldFile, what)
		require.Equal(t, ReasonPairHero, ReasonOf(err), what)
	}
}

// ONE MOMENT IN BOTH FILES (the B3 review, B7): the sidecar file must be of
// the world file's generation -- its saved_at, which the save writes into
// both. A save cut off after the world file and before the sidecar leaves the
// sidecar of the save before, and the load can tell.
func TestTheSidecarIsOfTheWorldFilesGeneration(t *testing.T) {
	w := b3Fixture()

	require.NoError(t, w.SameMoment(w.Sidecar), "the sidecar this save wrote")

	for what, data := range map[string][]byte{
		"the save before":   []byte(`{"version": 1, "generation": "2026-09-28T20:00:00Z", "kit": {}}`),
		"no world save yet": []byte(`{"version": 1, "kit": {}}`),
		"not a kit file":    []byte(`[]`),
	} {
		err := w.SameMoment(data)
		require.ErrorIs(t, err, ErrWorldFile, what)
		require.Equal(t, ReasonPairMoment, ReasonOf(err), what)
	}
}

// A FIELD THE FILE LACKS IS REFUSED, AT ANY DEPTH (the B3 review, B1): B3-8
// says any change to the file's shape bumps Version, and nothing held it --
// encoding/json reads a missing field as zero, so a file from before a field
// existed would load it as zero. Decode now re-encodes what it read and
// refuses any key the re-encoding writes that the file lacks.
func TestAFieldTheFileLacksIsRefused(t *testing.T) {
	data := b3Encode(t, b3Fixture())

	for _, path := range []string{
		"spawner.arrival", "hero.facing", "hero.stamina", "clock.elapsed", "rng.world.draws",
		"light.sources[0].burn", "entities[0].motion.dir", "combat.last_round.encounter", "scene.watch_clock_set",
	} {
		cut := b3Cut(t, data, path)

		_, err := Decode(cut)
		require.ErrorIs(t, err, ErrWorldFile, path)
		require.Equal(t, ReasonFieldMissing, ReasonOf(err), "%s cut: %v", path, err)
		require.Contains(t, err.Error(), path)
	}

	// A null where an object is written reads as zero too.
	nulled := bytes.Replace(data, []byte(`"last_round": {`), []byte(`"last_round": null, "x_was": {`), 1)
	nulled = b3Cut(t, nulled, "combat.x_was")

	_, err := Decode(nulled)
	require.Equal(t, ReasonFieldMissing, ReasonOf(err), "a nested null: %v", err)

	// An omitempty field written by neither is not missing (the control).
	_, err = Decode(data)
	require.NoError(t, err)
}

// b3Cut removes the key at a dotted path ("a.b[0].c") from a JSON document.
func b3Cut(t *testing.T, data []byte, path string) []byte {
	t.Helper()

	var tree interface{}
	require.NoError(t, json.Unmarshal(data, &tree))

	cur := tree
	parts := strings.Split(path, ".")

	for _, p := range parts[:len(parts)-1] {
		name, index := p, -1
		if i := strings.Index(p, "["); i >= 0 {
			name = p[:i]
			n, err := strconv.Atoi(strings.TrimSuffix(p[i+1:], "]"))
			require.NoError(t, err)
			index = n
		}

		cur = cur.(map[string]interface{})[name]
		if index >= 0 {
			cur = cur.([]interface{})[index]
		}
	}

	m := cur.(map[string]interface{})
	_, ok := m[parts[len(parts)-1]]
	require.True(t, ok, "%s is not in the file", path)
	delete(m, parts[len(parts)-1])

	out, err := json.Marshal(tree)
	require.NoError(t, err)

	return out
}

// The generated world is a map too: no path, no sha, and it says so.
func TestAGeneratedWorldIsAMap(t *testing.T) {
	w := b3Fixture()
	w.Map = Map{Generated: true}

	_, err := Decode(b3Encode(t, w))
	require.NoError(t, err)
}
