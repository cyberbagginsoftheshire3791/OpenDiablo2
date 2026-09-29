package d2gamescreen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2saveref"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// M4.6 B4a: THE LOAD, IN A UNIT GAME. b3SavableGame builds every world system
// as CreateGame does; b4Game adds what the load leans on that it lacked -- the
// corpses' open-count callback wired as CreateGame wires it (trap 5), the
// rising's clocks, and a real torch-and-blade kit (D1's torch) -- so a game
// can be saved, a second one made fresh, and the file loaded into it through
// the load's own functions in their order: restoreClock (step 2), checkLoad
// (step 4), resumeLoad (steps 5-7).

func b4Catalog(t *testing.T) *d2items.Catalog {
	t.Helper()

	data, err := b3Asset(t).LoadFile(itemCatalogPath)
	require.NoError(t, err)

	cat, err := d2items.Load(data)
	require.NoError(t, err)

	return cat
}

func b4Game(t *testing.T) (*Game, string) {
	t.Helper()

	v, save := b3SavableGame(t)

	v.corpses = d2world.NewCorpses(v.spawns.RowIsHuman, nil, func(delta int) {
		v.spawns.SetOpenBodies(max(0, v.spawns.OpenBodies()+delta))
	})
	v.combat.SetCorpses(v.corpses)
	v.rising = d2world.NewRising(v.corpses, v.spawns.Band, v.worldClock.Stage,
		d2rand.Derive(b3SavableSeed, d2rand.StreamRising), d2world.DefaultRisingDials())
	v.rising.SetClock(v.worldClock.WorldMinutes)
	v.corpses.SetClock(v.worldClock.WorldMinutes)

	kit, err := b4Catalog(t).NewKit("torch-and-blade")
	require.NoError(t, err)

	v.kit = kit

	// Nothing arrives -- the unit spawner has no bestiary to place a pack
	// from -- and nothing rises: the unit game has no one to stand a body up
	// as (a quiet evening; the rising still rolls, and draws, every band).
	require.NoError(t, v.spawns.HarnessSet("chance", 0.0))
	require.NoError(t, v.rising.HarnessSet("p", 0.0))
	require.NoError(t, v.rising.HarnessSet("edge_floor", 0.0))

	return v, save
}

// b4Busy fills v with an evening: the dead laid, one staked and one in a hasty
// grave, his torch lit from the kit as the L key lights it, the world run into
// the night (the rising rolls, the meters drain, the tables check), and the
// screen's own bookkeeping and his wind, place and run toggle set.
func b4Busy(t *testing.T, v *Game) {
	t.Helper()

	for i, id := range []string{"dead:1", "dead:2", "dead:3", "dead:4"} {
		v.corpses.FallHuman(id, "a man", float64(10+i), 12.5)
		v.fieldDead = append(v.fieldDead, id)
	}

	require.True(t, v.corpses.Close("dead:1"))
	require.True(t, v.corpses.Bury("dead:2"))

	for i := 0; i < 300; i++ { // 300 s at the day rate: past true dark
		v.advanceWorld(1)
	}

	require.Equal(t, d2world.StageNight, v.worldClock.Stage(), "the fixture runs into the night")

	// His torch, lit last, so it has minutes at the save (lit earlier, the
	// run into the night would have burnt it out and the kit's zero and the
	// light model's would agree for nothing -- the first unit control, U2,
	// was green on exactly that).
	_, torch, ok := v.kit.OffHandTorch()
	require.True(t, ok, "torch-and-blade holds a torch")

	src := v.light.Add(d2world.SourceTorch, true, 0, 0)
	src.Burn, torch.BurnLeft = torch.BurnLeft, 0
	require.Positive(t, src.Burn, "the torch is lit with minutes left")

	v.lastStage, v.dawnPaidDay = v.worldClock.Stage(), v.worldClock.DayIndex()
	v.watchStood, v.watchClock, v.watchClockSet = 12.5, v.worldClock.WorldMinutes(), true

	v.localPlayer.Stats.Health, v.localPlayer.Stats.Stamina = 41, 17.25
	v.localPlayer.StandAt(143.4, 112.2, 0)
	v.localPlayer.ToggleRunWalk()
	v.localPlayer.SetIsRunning(true)
}

// b4Load loads w into v through the load's steps, in order, as CreateGame and
// the first frame do; the kit the load binds is the sidecar's (its torch holds
// the minutes the save wrote into the document).
func b4Load(t *testing.T, v *Game, w *d2save.World) error {
	t.Helper()

	if r := v.restoreClock(w); r != nil {
		return r
	}

	if r := v.checkLoad(w); r != nil {
		return r
	}

	var doc struct {
		Kit *d2items.Kit `json:"kit"`
	}

	require.NoError(t, json.Unmarshal(w.Sidecar, &doc))
	doc.Kit.Bind(b4Catalog(t))

	v.kit = doc.Kit

	return v.resumeLoad(w)
}

// b4File saves v to a world file beside his save and reads it back.
func b4File(t *testing.T, v *Game, save, name string) (*d2save.World, []byte) {
	t.Helper()

	to := filepath.Join(filepath.Dir(save), name)

	_, err := v.SaveWorld(SaveOptions{To: to})
	require.NoError(t, err)

	data, err := os.ReadFile(to)
	require.NoError(t, err)

	w, err := d2save.Decode(data)
	require.NoError(t, err)

	return w, data
}

// b4Moment is a world file with its moment's stamp blanked (saved_at, and the
// sidecar's generation, which is the same stamp): what two saves of one moment
// share.
func b4Moment(t *testing.T, data []byte) string {
	t.Helper()

	w, err := d2save.Decode(data)
	require.NoError(t, err)

	var sc map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Sidecar, &sc))

	sc["generation"] = ""
	sidecar, err := json.Marshal(sc)
	require.NoError(t, err)

	w.SavedAt, w.Sidecar = "", sidecar

	out, err := json.Marshal(w)
	require.NoError(t, err)

	return string(out)
}

// b4Blocks is a world file's world blocks alone: b4Moment without the sidecar
// (a refusal test that takes his kit away puts it back as another value).
func b4Blocks(t *testing.T, data []byte) string {
	t.Helper()

	w, err := d2save.Decode(data)
	require.NoError(t, err)

	w.SavedAt, w.Sidecar = "", json.RawMessage(`{}`)

	out, err := json.Marshal(w)
	require.NoError(t, err)

	return string(out)
}

// THE ACCEPTANCE TEST IN MINIATURE: save an evening at T, load it into a
// fresh game, and the loaded game saves the same moment -- every block, byte
// for byte but the stamp -- and, run on as far as the saved one, stays equal.
// The load's steps ran in the authoritative order.
func TestTheLoadResumesTheSavedMoment(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	w, atT := b4File(t, saved, save, "t.world.json")
	require.NotEmpty(t, w.Light.Sources, "the fixture carries his torch")
	require.NotEmpty(t, w.Corpses.Bodies)
	require.NotZero(t, w.Rising.RNG.Draws, "the rising rolled")
	require.NotZero(t, w.Spawns.OpenBodies)

	resumed, _ := b4Game(t)
	require.NoError(t, b4Load(t, resumed, w))

	require.Equal(t, []string{
		"clock", "validated", "health", "squads", "light", "torch", "corpses", "rising",
		"spawns", "spawner", "notice", "pursuit", "combat", "scene", "hero", "world_rng",
	}, resumed.loadSteps, "the load order (docs/m4.6-world-save-notes.md)")

	// D1: the light model's torch, whole, and the kit's minutes zeroed --
	// where the sidecar the load bound held them.
	var bound struct {
		Kit *d2items.Kit `json:"kit"`
	}

	require.NoError(t, json.Unmarshal(w.Sidecar, &bound))
	bound.Kit.Bind(b4Catalog(t))

	_, inFile, _ := bound.Kit.OffHandTorch()
	require.Equal(t, saved.light.Carried().Burn, inFile.BurnLeft, "the file's sidecar carries the torch's minutes")

	_, torch, _ := resumed.kit.OffHandTorch()
	require.Zero(t, torch.BurnLeft, "the kit's torch holds zero, as the L key leaves it")
	require.Equal(t, *saved.light.Carried(), *resumed.light.Carried(), "the carried torch, id, burn and lit")

	// Trap 5: the open count is the file's, not the callback's.
	require.Equal(t, w.Spawns.OpenBodies, resumed.spawns.OpenBodies())

	// Rule 4: he stands where he was saved, bit-exact.
	require.Equal(t, saved.localPlayer.Position, resumed.localPlayer.Position)
	require.False(t, resumed.localPlayer.IsMoving())
	require.True(t, resumed.localPlayer.IsRunToggled() && resumed.localPlayer.IsRunning())

	_, atR0 := b4File(t, resumed, save, "r0.world.json")
	require.Equal(t, b4Moment(t, atT), b4Moment(t, atR0), "S_R0 = S_T: the loaded game saves the saved moment")

	for i := 0; i < 240; i++ { // four world hours at the night rate
		saved.advanceWorld(1)
		resumed.advanceWorld(1)
	}

	_, atU := b4File(t, saved, save, "u.world.json")
	_, atR := b4File(t, resumed, save, "r.world.json")
	require.Equal(t, b4Moment(t, atU), b4Moment(t, atR), "S_R = S_U: run on as far, the two are one world")
	require.NotEqual(t, b4Moment(t, atT), b4Moment(t, atU), "and the run on moved the world (the comparison has teeth)")
}

// A HALF-RESTORED WORLD NEVER RUNS (step 8): checkLoad refuses before anything
// but the clock is restored, and resumeLoad's own checks refuse before its
// first restore -- a refused load leaves every block as the fresh game made it.
func TestARefusedLoadRestoresNothing(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	fresh := func() (*Game, string) {
		v, _ := b4Game(t)
		_, data := b4File(t, v, save, "fresh.world.json")

		return v, b4Blocks(t, data)
	}

	decode := func() *d2save.World {
		data, err := d2save.Encode(good, nil)
		require.NoError(t, err)

		w, err := d2save.Decode(data)
		require.NoError(t, err)

		return w
	}

	checks := map[string]struct {
		code   string
		break_ func(w *d2save.World)
	}{
		"another seed": {LoadRefusedSeed, func(w *d2save.World) { w.Seed++ }},
		"another map (D5)": {LoadRefusedMap, func(w *d2save.World) {
			w.Map = d2save.Map{Path: "village.tmj", SHA: "0000000000000000000000000000000000000000000000000000000000000000"}
		}},
		"a villager the map does not build": {LoadRefusedNatives, func(w *d2save.World) {
			w.Entities = append(w.Entities, d2save.Entity{ID: "0ak", Kind: d2save.KindNPC, Monstat: "Akara",
				Native: true, NameKey: "Akara", Born: &[2]float64{10, 10}, X: 10, Y: 10,
				Motion: d2mapentity.Motion{Pos: [2]float64{50, 50}}})
		}},
		"light: a source past next_id":   {LoadRefusedBlock, func(w *d2save.World) { w.Light.NextID = 1 }},
		"squads: another squad selected": {LoadRefusedBlock, func(w *d2save.World) { w.Squads.Selected = "s:9" }},
		"corpses: a beast risen": {LoadRefusedBlock, func(w *d2save.World) {
			w.Corpses.Bodies = append(w.Corpses.Bodies, d2world.CorpseSnapshot{
				ID: "b:9", Row: "wolves", Class: d2world.CorpseBeast, State: d2world.CorpseRisen, X: 1, Y: 1})
		}},
		"rising: a band by day":      {LoadRefusedBlock, func(w *d2save.World) { w.Rising.LastBand, w.Rising.LastStage = 1, "day" }},
		"combat: an end never begun": {LoadRefusedBlock, func(w *d2save.World) { w.Combat.Ended = w.Combat.Started + 1 }},
		"spawns: next id 0":          {LoadRefusedBlock, func(w *d2save.World) { w.Spawns.NextID = 0 }},
		"spawner: -1 arrivals":       {LoadRefusedBlock, func(w *d2save.World) { w.Spawner.Arrival = -1 }},
		"notice: -1 checks":          {LoadRefusedBlock, func(w *d2save.World) { w.Notice.Checks = -1 }},
		"pursuit: -1 solves":         {LoadRefusedBlock, func(w *d2save.World) { w.Pursuit.Solves = -1 }},
		"world rng past the cap":     {LoadRefusedBlock, func(w *d2save.World) { w.RNG.World.Draws = d2rand.MaxDraws + 1 }},
		"scene: no stage":            {LoadRefusedBlock, func(w *d2save.World) { w.Scene.LastStage = "noon" }},
	}

	for name, c := range checks {
		v, before := fresh()
		w := decode()
		c.break_(w)

		r := v.restoreClock(w)
		if r == nil {
			r = v.checkLoad(w)
		}

		require.NotNil(t, r, "%s: refused", name)
		require.Equal(t, c.code, r.Code, "%s: %v", name, r)
		require.ErrorIs(t, r, ErrLoadRefused)

		// Nothing but the clock was restored: put the fresh clock back and
		// the game is the fresh one.
		require.NoError(t, v.worldClock.Restore(d2world.ClockSnapshot{}))
		_, after := b4File(t, v, save, "after.world.json")
		require.Equal(t, before, b4Blocks(t, after), "%s: a refused load restored something", name)
	}

	// resumeLoad's own checks, which need his kit and progress bound.
	bound := map[string]func(v *Game, w *d2save.World){
		"no kit":                  func(v *Game, w *d2save.World) { v.kit = nil },
		"health past his maximum": func(v *Game, w *d2save.World) { w.Hero.Health = v.localPlayer.Stats.MaxHealth + 1 },
		"a carried torch and none in his off-hand": func(v *Game, w *d2save.World) { v.kit.SpendOffHand() },
	}

	for name, brk := range bound {
		v, before := fresh()
		w := decode()
		kit := v.kit

		require.Nil(t, v.restoreClock(w))
		require.Nil(t, v.checkLoad(w), "%s: checkLoad passes a good file", name)

		health := v.localPlayer.Stats.Health
		brk(v, w)

		err := v.resumeLoad(w)

		var r *LoadRefusal
		require.True(t, errors.As(err, &r), "%s: refused, got %v", name, err)
		require.Equal(t, LoadRefusedBlock, r.Code)
		require.Equal(t, health, v.localPlayer.Stats.Health, "%s: his health was restored", name)
		require.Equal(t, []string{"clock", "validated"}, v.loadSteps, "%s: a step ran", name)

		v.kit = kit
		require.NoError(t, v.worldClock.Restore(d2world.ClockSnapshot{}))

		_, after := b4File(t, v, save, "after.world.json")
		require.Equal(t, before, b4Blocks(t, after), "%s: a refused load restored something", name)
	}
}

// b4Files is a hero's folder holding a world file w, his .od2 (name and class
// as given) and his sidecar -- the world file's own, of its generation, as a
// save leaves them.
func b4Files(t *testing.T, w *d2save.World, name string, class int) string {
	t.Helper()

	save := filepath.Join(t.TempDir(), "0.od2")

	data, err := d2save.Encode(w, nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(d2save.WorldPath(save), data, 0o600))

	od2, err := json.Marshal(map[string]interface{}{"heroName": name, "heroType": class})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(save, od2, 0o600))

	var doc bytes.Buffer
	require.NoError(t, json.Indent(&doc, w.Sidecar, "", "  "))
	require.NoError(t, os.WriteFile(d2items.SidecarPath(save), doc.Bytes(), 0o600))

	return save
}

// amazon is his class's number in the .od2 (d2enum.HeroAmazon).
const b4Amazon = 6

// STEP 1'S REFUSALS: each sets the file aside (rule 7) -- rule 9's too since
// the B4a review (A2) -- but a hunted night's, which stays for B4b (the
// review's B2); each writes nothing over his sidecar, keeps no copy of it, and
// says why. THE CONTROL: a good file is taken, his own sidecar kept as
// .preload first (A1), and his sidecar written from the file, byte for byte
// the document the save wrote.
func TestPrepareLoadRefusals(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	copyOf := func() *d2save.World {
		data, err := d2save.Encode(good, nil)
		require.NoError(t, err)

		w, err := d2save.Decode(data)
		require.NoError(t, err)

		return w
	}

	// The control.
	dir := b4Files(t, copyOf(), "Saver", b4Amazon)
	his := []byte(`{"version": 1, "generation": "` + good.SavedAt + `", "kit": {}}`)
	require.NoError(t, os.WriteFile(d2items.SidecarPath(dir), his, 0o600))

	w, r := PrepareLoad(dir, false)
	require.Nil(t, r)
	require.NotNil(t, w)

	kept, err := os.ReadFile(preloadPath(dir))
	require.NoError(t, err)
	require.Equal(t, string(his), string(kept), "his own sidecar is kept before step 1 writes over it (A1)")

	written, err := os.ReadFile(d2items.SidecarPath(dir))
	require.NoError(t, err)
	forgetPreload(dir)

	// The document the save wrote into both files: the live kit's torch reads
	// zero (the L key) and heroBytes writes the carried torch's minutes in.
	// His sidecar is that document, byte for byte -- the torch's minutes kept
	// (decision B4a-1: the load zeroes them in the kit it binds, not here).
	want, err := saved.heroBytes(good.SavedAt)
	require.NoError(t, err)
	require.Equal(t, string(want), string(written), "his sidecar is the save's document, byte for byte")

	var doc struct {
		Kit *d2items.Kit `json:"kit"`
	}

	require.NoError(t, json.Unmarshal(written, &doc))
	doc.Kit.Bind(b4Catalog(t))

	_, torch, _ := doc.Kit.OffHandTorch()
	require.Equal(t, saved.light.Carried().Burn, torch.BurnLeft, "the sidecar keeps the torch's minutes")
	require.Equal(t, good.SavedAt, LastLoad().SavedAt)

	// No world file: nothing to resume, nothing refused.
	w, r = PrepareLoad(filepath.Join(t.TempDir(), "1.od2"), false)
	require.Nil(t, w)
	require.Nil(t, r)

	entity := d2save.Entity{ID: "0dog", Kind: d2save.KindNPC, Monstat: "fallen1", X: 10, Y: 10,
		Motion: d2mapentity.Motion{Pos: [2]float64{50, 50}, Target: [2]float64{50, 50}}}

	// Where a refused file goes: .v<Version>.unread, the version it holds
	// (derived from d2save.Version, so the raid's version bump -- R0.5 --
	// does not turn this red), or nowhere: a hunted night STAYS for B4b (the
	// B4a review, B2; decision B4a-R3).
	unread, stays := fmt.Sprintf(".v%d.unread", d2save.Version), ""

	cases := map[string]struct {
		code, aside string
		files       func(w *d2save.World) string
		says        string
	}{
		"a newer build's file": {LoadRefusedVersion, fmt.Sprintf(".v%d.unread", d2save.Version+1), func(w *d2save.World) string {
			s := b4Files(t, w, "Saver", b4Amazon)
			require.NoError(t, os.WriteFile(d2save.WorldPath(s), []byte(fmt.Sprintf(`{"version": %d}`, d2save.Version+1)), 0o600))

			return s
		}, ""},
		"not a world file": {LoadRefusedFile, ".unread", func(w *d2save.World) string {
			s := b4Files(t, w, "Saver", b4Amazon)
			require.NoError(t, os.WriteFile(d2save.WorldPath(s), []byte("torn"), 0o600))

			return s
		}, ""},
		"another hero's": {LoadRefusedHero, unread, func(w *d2save.World) string {
			return b4Files(t, w, "Otherman", b4Amazon)
		}, ""},
		"another class": {LoadRefusedHero, unread, func(w *d2save.World) string {
			return b4Files(t, w, "Saver", b4Amazon-1)
		}, ""},
		"a torn save": {LoadRefusedTorn, unread, func(w *d2save.World) string {
			s := b4Files(t, w, "Saver", b4Amazon)
			require.NoError(t, os.WriteFile(d2items.SidecarPath(s), []byte(`{"version": 1, "generation": "an older save", "kit": {}}`), 0o600))

			return s
		}, ""},
		"no sidecar": {LoadRefusedTorn, unread, func(w *d2save.World) string {
			s := b4Files(t, w, "Saver", b4Amazon)
			require.NoError(t, os.Remove(d2items.SidecarPath(s)))

			return s
		}, ""},
		"a monster not the map's": {LoadRefusedHunted, stays, func(w *d2save.World) string {
			w.Entities = []d2save.Entity{entity}

			return b4Files(t, w, "Saver", b4Amazon)
		}, "is not one the map builds"},
		"a monster's body": {LoadRefusedHunted, stays, func(w *d2save.World) string {
			w.Entities, w.Bodies = []d2save.Entity{entity}, []d2save.Body{{ID: "0dog", Health: 3, MaxHealth: 10}}

			return b4Files(t, w, "Saver", b4Amazon)
		}, "have a body"},
		"a pack on the map": {LoadRefusedHunted, stays, func(w *d2save.World) string {
			w.Entities = []d2save.Entity{entity}
			w.Spawns.Groups = []d2world.SpawnGroupSnapshot{{ID: "g:1", Row: "wolves", Code: "fallen1",
				Members: []d2world.SpawnMemberSnapshot{{ID: "0dog", X: 10, Y: 10}}, Stage: "night"}}

			return b4Files(t, w, "Saver", b4Amazon)
		}, "pack(s) on the map"},
		"a watch": {LoadRefusedHunted, stays, func(w *d2save.World) string {
			w.Entities = []d2save.Entity{entity}
			w.Notice.Watches = []d2world.WatchSnapshot{{Watcher: "0dog", Target: d2saveref.Player}}

			return b4Files(t, w, "Saver", b4Amazon)
		}, "watch(es)"},
		"a chase": {LoadRefusedHunted, stays, func(w *d2save.World) string {
			w.Entities = []d2save.Entity{entity}
			w.Pursuit.Chases = []d2world.ChaseSnapshot{{Hunter: "0dog", Quarry: d2saveref.Player}}

			return b4Files(t, w, "Saver", b4Amazon)
		}, "chase(s)"},
		"a deployed squad (D3)": {LoadRefusedHunted, stays, func(w *d2save.World) string {
			w.Entities = []d2save.Entity{entity}
			w.Squads.NextID = 3
			w.Squads.Squads = append(w.Squads.Squads, d2world.SquadSnapshot{ID: "s:2", Owner: "player", Ordinal: 2,
				Members: []d2world.SquadMemberSnapshot{{Entity: "0dog", Health: 10, Max: 10}}})

			return b4Files(t, w, "Saver", b4Amazon)
		}, "deployed model"},
	}

	for name, c := range cases {
		s := c.files(copyOf())
		world := d2save.WorldPath(s)

		before, err := os.ReadFile(world)
		require.NoError(t, err)

		sidecar, sidecarErr := os.ReadFile(d2items.SidecarPath(s))

		w, r := PrepareLoad(s, false)
		require.Nil(t, w, name)
		require.NotNil(t, r, name)
		require.Equal(t, c.code, r.Code, "%s: %v", name, r)
		require.ErrorIs(t, r, ErrLoadRefused)
		require.Contains(t, r.Detail, c.says, "%s: refused by its own clause", name)

		now, nowErr := os.ReadFile(d2items.SidecarPath(s))
		require.Equal(t, sidecarErr == nil, nowErr == nil, "%s: the sidecar is as it was", name)
		require.Equal(t, sidecar, now, "%s: a refused load wrote his sidecar", name)

		_, err = os.Stat(preloadPath(s))
		require.True(t, os.IsNotExist(err), "%s: a refusal at step 1 keeps no copy of his sidecar", name)

		rep := LastLoad()
		require.Equal(t, c.code, rep.Refused, name)
		require.False(t, rep.Resumed, name)

		if c.aside == stays {
			// B2: a hunted night stays where it is, whole, for B4b.
			left, err := os.ReadFile(world)
			require.NoError(t, err, "%s: a hunted night stays for B4b", name)
			require.Equal(t, before, left, "%s: and is not touched", name)
			require.Empty(t, rep.SetAside, name)

			continue
		}

		aside, err := os.ReadFile(world + c.aside)
		require.NoError(t, err, "%s: set aside as %s", name, c.aside)
		require.Equal(t, before, aside, "%s: set aside whole", name)

		_, err = os.Stat(world)
		require.True(t, os.IsNotExist(err), "%s: the world file is out of the way", name)
		require.Equal(t, world+c.aside, rep.SetAside, name)
	}

	// Rule 9: refused, and SET ASIDE like every other refusal (the B4a
	// review, A2; decision B4a-R1 overturns B4a-2, which left it for the next
	// single-player load -- and that load resumed the old moment over the
	// network evening, BUG-61). His hero, kit and progress carry on: his
	// sidecar is not touched.
	s := b4Files(t, copyOf(), "Saver", b4Amazon)
	sidecar, _ := os.ReadFile(d2items.SidecarPath(s))
	file, _ := os.ReadFile(d2save.WorldPath(s))

	w, r = PrepareLoad(s, true)
	require.Nil(t, w)
	require.Equal(t, LoadRefusedNetwork, r.Code)

	_, err = os.Stat(d2save.WorldPath(s))
	require.True(t, os.IsNotExist(err), "a network game sets the world file aside")

	aside, err := os.ReadFile(d2save.WorldPath(s) + unread)
	require.NoError(t, err)
	require.Equal(t, file, aside, "set aside whole")
	require.Equal(t, d2save.WorldPath(s)+unread, LastLoad().SetAside)

	now, _ := os.ReadFile(d2items.SidecarPath(s))
	require.Equal(t, sidecar, now, "his sidecar is his, untouched")
}

// THE DAWN AFTER A REFUSAL THAT TORE A GAME DOWN keeps the refusal as its
// report (FellBack): it finds no world file, having set it aside, and the
// harness still reads why. A refusal before the game opened is not kept past
// the next load of another hero.
func TestTheDawnAfterATornDownLoadKeepsItsReport(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")
	s := b4Files(t, good, "Saver", b4Amazon)

	w, r := PrepareLoad(s, false)
	require.Nil(t, r)
	require.NotNil(t, w)

	aside := SetLoadAside(s, refuseLoad(LoadRefusedMap, "the map moved"), true)
	require.Equal(t, d2save.WorldPath(s)+fmt.Sprintf(".v%d.unread", d2save.Version), aside)

	w, r = PrepareLoad(s, false)
	require.Nil(t, w)
	require.Nil(t, r)

	rep := LastLoad()
	require.True(t, rep.FellBack)
	require.Equal(t, LoadRefusedMap, rep.Refused)
	require.Equal(t, aside, rep.SetAside)

	// For that dawn only: the hero's next load starts a report of its own
	// (the B4a review fixes: act 6f read the old refusal as its own).
	_, _ = PrepareLoad(s, false)
	require.False(t, LastLoad().FellBack, "the next load of the same hero reports itself")
	require.Empty(t, LastLoad().Refused)

	_, _ = PrepareLoad(filepath.Join(t.TempDir(), "2.od2"), false)
	require.Empty(t, LastLoad().Refused, "another hero's load starts a report of its own")
}

// A LOAD IN PROGRESS, OR ONE REFUSED ON ITS FIRST FRAME, IS NOT A MOMENT: the
// save refuses NOT_READY.
func TestNoSaveWhileALoadIsPendingOrAbandoned(t *testing.T) {
	v, save := b4Game(t)
	v.pendingLoad = &d2save.World{}
	b3Refused(t, v, SaveRefusedNotReady, SaveOptions{To: filepath.Join(filepath.Dir(save), "p.world.json")})

	v.pendingLoad, v.loadAbandoned = nil, true
	b3Refused(t, v, SaveRefusedNotReady, SaveOptions{To: filepath.Join(filepath.Dir(save), "a.world.json")})
}
