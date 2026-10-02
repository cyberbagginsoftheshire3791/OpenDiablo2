package d2gamescreen

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
)

// M4.6 B3 REVIEW (29 Sep 2026): the fixes to the save verb, each tested on a
// game screen that SAVES -- every world system built, as CreateGame builds
// them, around a bare player -- so a check the save makes can be seen to
// refuse, and its control to pass. B3's own unit tests stopped at NOT_READY
// (no systems); the whole-game fixture is still the playtest's (act 7).

const b3SavableSeed int64 = 2026

// b3SetField sets one field of *ptr, exported or not.
func b3SetField(t *testing.T, ptr interface{}, name string, value interface{}) {
	t.Helper()

	v := reflect.ValueOf(ptr).Elem().FieldByName(name)
	require.True(t, v.IsValid(), "no field %s", name)

	reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

// b3SavableGame is a game screen past every refusal, with every world system
// built on the game's seed: SaveWorld to a path writes a whole world file.
// Its hero is Saver the Amazon, at 50 of 60, 33.5 stamina.
func b3SavableGame(t *testing.T) (*Game, string) {
	t.Helper()

	asset := b3Asset(t)
	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, asset)
	engine.ResetAuthoredMap(d2enum.RegionAct1Town, 40, 40)
	engine.SetSeed(b3SavableSeed)

	save := filepath.Join(t.TempDir(), "0.od2")
	v := b3Game(t, engine, save)
	v.gameClient.Seed = b3SavableSeed

	b3SetField(t, v.localPlayer, "name", "Saver")
	v.localPlayer.Class = d2enum.HeroAmazon
	v.localPlayer.Stats.Stamina = 33.5

	v.worldClock = d2world.NewClock(d2world.DefaultClockDials())
	v.light = d2world.NewLight(v.worldClock, d2world.DefaultLightDials())
	v.squads = d2world.NewSquads(v.worldClock, d2world.DefaultMeterDials(), squadDeployer{game: v})
	v.meters = v.squads.PlayerMeters()
	v.pursuit = d2world.NewPursuit(mapRouter{engine: engine}, d2world.DefaultPursuitDials())
	v.notice = d2world.NewNotice(mapSight{engine: engine}, v.light, d2world.DefaultNoticeDials())
	v.spawner = &gameSpawner{engine: engine, asset: asset, adopt: v.adoptNPCBody, release: v.releaseNPCBody}
	v.spawns = d2world.NewSpawns(v.worldClock, v.notice, v.spawner, v.pursuit, v.light,
		d2rand.Derive(b3SavableSeed, d2rand.StreamSpawns), d2world.DefaultSpawnDials())
	v.combat = d2world.NewCombat(v.worldClock, v.notice, v.squads, v.light, v, v.spawns, v, v.spawns, v.pursuit,
		d2rand.Derive(b3SavableSeed, d2rand.StreamCombat), shippedCombatDials())
	v.corpses = d2world.NewCorpses(nil, nil, nil)
	v.combat.SetCorpses(v.corpses)
	v.rising = d2world.NewRising(v.corpses, v.spawns.Band, v.worldClock.Stage,
		d2rand.Derive(b3SavableSeed, d2rand.StreamRising), d2world.DefaultRisingDials())
	// The raid's R2: every game has Seek, as CreateGame builds it.
	v.seek = d2world.NewSeek(v.notice, v.spawns, v.combat, d2world.DefaultSeekDials())
	v.seek.SetQuarries(v.seekQuarries)
	v.seek.SetResolver(worldResolver{v})
	// The raid's R3a: every game has its households, as CreateGame builds
	// them -- from the map's village (this map places none).
	households, err := d2world.NewHouseholds(villagePlaces(engine.Village()))
	require.NoError(t, err)
	v.households = households
	v.lastStage = v.worldClock.Stage()
	// Fog of war F3: every game has a fog, as CreateGame builds it -- off,
	// as a game is with -fog=false, so it saves an empty grid.
	v.fog = newGameFog(mapTileSight{v}, false)

	t.Cleanup(v.releaseWorld)

	return v, save
}

// b3SaveTo saves v to a fresh path beside his save and returns the file read
// back, the path, and the save's error.
func b3SaveTo(t *testing.T, v *Game, save string) (*d2save.World, string, error) {
	t.Helper()

	to := filepath.Join(filepath.Dir(save), "copy.world.json")

	_, err := v.SaveWorld(SaveOptions{To: to})
	if err != nil {
		return nil, to, err
	}

	data, rerr := os.ReadFile(to)
	require.NoError(t, rerr)

	w, derr := d2save.Decode(data)
	require.NoError(t, derr)

	return w, to, nil
}

// THE CONTROL for every test below: a savable game saves, and the file names
// the hero, his facing and stamina (B6), and carries its own generation in
// its sidecar (B7).
func TestASavableGameSaves(t *testing.T) {
	v, save := b3SavableGame(t)

	w, to, err := b3SaveTo(t, v, save)
	require.NoError(t, err)

	require.Equal(t, "Saver", w.Hero.Name)
	require.Equal(t, "Amazon", w.Hero.Class)
	require.Equal(t, 33.5, w.Hero.Stamina)
	require.Equal(t, v.localPlayer.Facing(), w.Hero.Facing)
	require.NoError(t, w.SameMoment(w.Sidecar), "the embedded sidecar is of the file's own generation")

	res, err := v.SaveWorld(SaveOptions{To: to})
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(res.WorldPath))
}

// B2: THE SAVE HOLDS EVERY BLOCK TO THE LOAD'S OWN CHECK OF IT. Each probe
// corrupts one block after the snapshots are taken -- in a way the file's own
// Check does not look at -- and the save must fail with that block named,
// touching no file. Before the fix the file's Check was the save's last word,
// and a lit torch at no minutes, two carried torches or a source past next_id
// were written for a load to refuse.
//
// The fog probe (the F3 review's C1, 1 Oct 2026): the live game's grid is
// always of its own map, so only a probe reaches the save's fog check. Take
// the check out of validateSnapshots (the reviewer's m24) and the probe fails,
// "fog: a block the load would refuse was written" (strigoi-harness-runs\
// wt-fog3\nc\ncc1-save-skips-fog.txt).
func TestTheSaveRunsTheLoadsOwnChecks(t *testing.T) {
	probes := map[string]func(w *d2save.World){
		"clock": func(w *d2save.World) { w.Clock.Elapsed = -1 },
		"light": func(w *d2save.World) {
			w.Light = d2world.LightSnapshot{NextID: 2, Sources: []d2world.LightSourceSnapshot{
				{ID: 1, Kind: d2world.SourceTorch, Burn: 0, Lit: true, Carried: true},
			}}
		},
		"squads": func(w *d2save.World) { w.Squads.Selected = "s:9" },
		"corpses": func(w *d2save.World) {
			w.Corpses.Bodies = append(w.Corpses.Bodies, d2world.CorpseSnapshot{
				ID: "b:1", Row: "wolves", Class: d2world.CorpseBeast, State: d2world.CorpseRisen, X: 1, Y: 1,
			})
		},
		"rising":  func(w *d2save.World) { w.Rising.LastBand, w.Rising.LastStage = 1, "day" },
		"combat":  func(w *d2save.World) { w.Combat.Ended = w.Combat.Started + 1 },
		"spawns":  func(w *d2save.World) { w.Spawns.NextID = 0 },
		"spawner": func(w *d2save.World) { w.Spawner.Arrival = -1 },
		"notice":  func(w *d2save.World) { w.Notice.Checks = -1 },
		"pursuit": func(w *d2save.World) { w.Pursuit.Solves = -1 },
		// Fog F3 (the F3 review's C1): a well-formed grid, keyed on the file's
		// own map (so the file's check passes), of another size than the map.
		"fog": func(w *d2save.World) {
			f := d2world.NewFog(d2world.DefaultFogDials(), nil)
			f.Update(39, 40, nil)
			w.Fog = f.Snapshot(w.Map.SHA)
		},
	}

	// The review's three light cases, each its own probe.
	light := map[string]d2world.LightSnapshot{
		"a lit torch at no minutes": {NextID: 2, Sources: []d2world.LightSourceSnapshot{
			{ID: 1, Kind: d2world.SourceTorch, Lit: true, Carried: true}}},
		"two carried sources": {NextID: 3, Sources: []d2world.LightSourceSnapshot{
			{ID: 1, Kind: d2world.SourceTorch, Burn: 5, Carried: true}, {ID: 2, Kind: d2world.SourceTorch, Burn: 5, Carried: true}}},
		"a source id past next_id": {NextID: 1, Sources: []d2world.LightSourceSnapshot{
			{ID: 1, Kind: d2world.SourceTorch, Burn: 5, Carried: true}}},
	}

	for name, snap := range light {
		snap := snap
		probes["light: "+name] = func(w *d2save.World) { w.Light = snap }
	}

	t.Cleanup(func() { saveProbe = nil })

	for name, probe := range probes {
		v, save := b3SavableGame(t)
		saveProbe = probe

		_, to, err := b3SaveTo(t, v, save)
		saveProbe = nil

		block := strings.SplitN(name, ":", 2)[0]

		require.Error(t, err, "%s: a block the load would refuse was written", name)

		var r *SaveRefusal
		require.False(t, errors.As(err, &r), "%s: a disagreement is an error (INTERNAL), not a refusal: %v", name, err)
		require.Contains(t, err.Error(), "the "+block+" block", "%s: the error names the block", name)

		_, serr := os.Stat(to)
		require.True(t, os.IsNotExist(serr), "%s: nothing is written", name)
	}

	// And a LIVE world that holds what no load takes, no probe: a carried
	// torch lit with its minutes gone (the light model normally removes it on
	// the frame it is seen).
	v, save := b3SavableGame(t)
	src := v.light.Add(d2world.SourceTorch, true, 0, 0)
	src.Burn = 0

	_, _, err := b3SaveTo(t, v, save)
	require.Error(t, err)
	require.Contains(t, err.Error(), "the light block")
}

// B5: to is "somewhere else": never one of his own files. A relative to is
// made absolute, and world_path says where it went.
func TestToIsNeverOneOfHisFiles(t *testing.T) {
	v, save := b3SavableGame(t)
	world := d2save.WorldPath(save)

	for _, to := range []string{world, world + ".bak", save, save + ".bak", d2items.SidecarPath(save), save + ".anything"} {
		_, err := v.SaveWorld(SaveOptions{To: to})
		require.ErrorIs(t, err, ErrBadSaveArgument, "to %s", to)

		_, err = v.SaveWorld(SaveOptions{To: to, Omit: []string{"corpses"}})
		require.ErrorIs(t, err, ErrBadSaveArgument, "to %s with omit", to)

		_, serr := os.Stat(to)
		require.True(t, os.IsNotExist(serr), "%s was written", to)
	}

	// The control: a neighbour's name in the same folder is not his.
	res, err := v.SaveWorld(SaveOptions{To: filepath.Join(filepath.Dir(save), "10.od2.world.json")})
	require.NoError(t, err)
	require.Len(t, res.Written, 1)

	// A relative to, made absolute from the working directory.
	t.Chdir(t.TempDir())

	res, err = v.SaveWorld(SaveOptions{To: "relative.world.json"})
	require.NoError(t, err)

	wd, _ := os.Getwd()
	require.Equal(t, filepath.Join(wd, "relative.world.json"), res.WorldPath)
	require.True(t, filepath.IsAbs(res.WorldPath))
}

// B3: A NATIVE IS KNOWN BY WHO HE IS AND WHERE THE MAP PUT HIM, NOT HIS ID.
// B4b's load re-keys a rebuilt map's villager to his saved id; natives were
// recorded by the id the map gave him, so the save after a load would mark no
// villager native. Held by the entity, the re-keyed villager is still his.
// Two natives sharing the pair are refused: the load could not tell them
// apart.
func TestNativesAreKnownByWhoAndWhereNotByID(t *testing.T) {
	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, b3Asset(t))
	engine.ResetAuthoredMap(d2enum.RegionAct1Town, 40, 40)

	native := b3Dog(t, engine, 50, 55)
	native.SetCreatureID("feral-dog")

	v := &Game{gameClient: &d2client.GameClient{MapEngine: engine}}
	v.natives = nativesOf(engine)

	// B4b's re-key: the same entity, the saved id.
	engine.RemoveEntity(native)
	b3SetField(t, native, "uuid", "0a-the-saved-id")
	engine.AddEntity(native)

	list, err := v.saveEntities()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "0a-the-saved-id", list[0].ID)
	require.True(t, list[0].Native, "the re-keyed villager is still native")
	require.Equal(t, &[2]float64{10, 11}, list[0].Born)

	// Two natives of one name at one place.
	engine2 := d2mapengine.CreateMapEngine(d2util.LogLevelNone, b3Asset(t))
	engine2.ResetAuthoredMap(d2enum.RegionAct1Town, 40, 40)

	for i := 0; i < 2; i++ {
		b3Dog(t, engine2, 50, 55).SetCreatureID("feral-dog")
	}

	v2 := &Game{gameClient: &d2client.GameClient{MapEngine: engine2}}
	v2.natives = nativesOf(engine2)

	_, err = v2.saveEntities()
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not tell them apart")
}

// THE COMBAT MODEL'S OWN REFUSAL IS THE FIGHT'S, and comes before TALKING,
// JOURNAL and LOADOUT (the review's order item): experience or paced minutes
// not yet taken are a fight not settled, whatever else is open.
func TestTheCombatModelsRefusalIsTheFights(t *testing.T) {
	v, save := b3SavableGame(t)
	b3SetField(t, v.combat, "owedMinutes", 1.5)
	v.talk = &d2dialogue.Talk{NodeID: "greet"}

	b3Refused(t, v, SaveRefusedFighting, SaveOptions{To: filepath.Join(filepath.Dir(save), "x.world.json")})

	// The control: with the minutes taken, the talk is what refuses.
	b3SetField(t, v.combat, "owedMinutes", 0.0)
	b3Refused(t, v, SaveRefusedTalking, SaveOptions{})
}

// A kit save between world saves carries the last world save's generation, so
// the sidecar and the world file agree until the next world save (B7). Left
// out, every equip after a save would make the pair look torn.
func TestAKitSaveCarriesTheWorldSavesGeneration(t *testing.T) {
	v, save := b3SavableGame(t)
	v.saveGeneration = "2026-09-29T08:00:00Z"

	v.saveKit()

	data, err := os.ReadFile(d2items.SidecarPath(save))
	require.NoError(t, err)

	var sc struct {
		Generation string `json:"generation"`
	}

	require.NoError(t, json.Unmarshal(data, &sc))
	require.Equal(t, v.saveGeneration, sc.Generation)
}

// B4's other half, on the screen: ForgetBody drops a body and says whether
// there was one.
func TestForgetBody(t *testing.T) {
	v := &Game{}
	v.adoptNPCBody("0a-wolf", 40)

	require.True(t, v.ForgetBody("0a-wolf"))
	require.Equal(t, 0, v.BodiesKnown())
	require.False(t, v.ForgetBody("0a-wolf"))
}

// A1, the hero screen's half: Delete removes every file of his.
func TestTheHeroScreensDeleteRemovesEveryFileOfHis(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "1.od2")

	for _, p := range []string{save, save + ".bak", d2items.SidecarPath(save), d2save.WorldPath(save),
		d2save.WorldPath(save) + ".bak", d2save.WorldPath(save) + ".v2.unread", filepath.Join(dir, "10.od2")} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	}

	v := &CharacterSelect{gameStates: []*d2hero.HeroState{{FilePath: save}}, Logger: d2util.NewLogger()}
	v.Logger.SetLevel(d2util.LogLevelNone)
	v.deleteSelectedHero()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the neighbour 10.od2 is left")
	require.Equal(t, "10.od2", entries[0].Name())
}

// THE T LABELS HAVE TEETH (the review's C item, B2's b2aTransientsRefused for
// the screen). Every Game field labelled T is one of two kinds:
//
//	"refused"      setting it makes the save refuse -- shown by setting it
//	               on a game that saves otherwise
//	"reset:<fn>"   stale between fights, and never read before <fn> writes it
//	               -- shown by reading <fn>'s source for the assignment
var b3TransientKind = map[string]string{
	"pendingStrike":    "refused",
	"tacticalPace":     "refused",
	"tacticalReserved": "refused",
	"choosingLoadout":  "refused",
	"journalOpen":      "refused",
	"journalFight":     "refused",
	"talk":             "refused",
	"died":             "refused",
	"wasFighting":      "refused",
	"pendingLoad":      "refused",
	"loadAbandoned":    "refused",
	"combatGrace":      "refused",

	"loadFailed": "reset:failPendingLoad",

	"pendingStrikeStill":    "reset:OnTacticalTarget",
	"journalRows":           "reset:sampleFight",
	"death":                 "reset:noticeDeath",
	"activityBeforeFight":   "reset:applyFightingActivity",
	"paceOpenClock":         "reset:applyFightingActivity",
	"paceOpenDay":           "reset:applyFightingActivity",
	"torchLitRounds":        "reset:applyFightingActivity",
	"torchOutThisFight":     "reset:applyFightingActivity",
	"torchesBurntThisFight": "reset:applyFightingActivity",
}

func TestEveryTransientIsRefusedOrReset(t *testing.T) {
	var labelled []string

	for name, class := range b3GameClasses {
		if strings.HasPrefix(class, "T: ") {
			labelled = append(labelled, name)
		}
	}

	sort.Strings(labelled)

	for _, name := range labelled {
		if _, ok := b3TransientKind[name]; !ok {
			t.Errorf("Game.%s is labelled T, and b3TransientKind does not say how the save stands clear of it", name)
		}
	}

	for name := range b3TransientKind {
		if !strings.HasPrefix(b3GameClasses[name], "T: ") {
			t.Errorf("b3TransientKind names %s, which is not labelled T", name)
		}
	}

	// The control: nothing set, and the game saves.
	v, save := b3SavableGame(t)
	_, _, err := b3SaveTo(t, v, save)
	require.NoError(t, err)

	assigns := b3Assignments(t)

	for _, name := range labelled {
		kind := b3TransientKind[name]

		switch {
		case kind == "refused":
			v, save := b3SavableGame(t)
			b3SetNonZero(t, v, name)

			_, err := v.SaveWorld(SaveOptions{To: filepath.Join(filepath.Dir(save), "t.world.json")})

			var r *SaveRefusal
			if !errors.As(err, &r) {
				t.Errorf("Game.%s is labelled T and refused, but the save goes on with it set (%v)", name, err)
			}
		case strings.HasPrefix(kind, "reset:"):
			fn := strings.TrimPrefix(kind, "reset:")
			if !assigns[fn][name] {
				t.Errorf("Game.%s is labelled T and reset by %s, which does not assign it", name, fn)
			}
		default:
			t.Errorf("Game.%s: kind %q", name, kind)
		}
	}
}

// b3SetNonZero sets one field of v to a value that is not its zero.
func b3SetNonZero(t *testing.T, v *Game, name string) {
	t.Helper()

	fv := reflect.ValueOf(v).Elem().FieldByName(name)
	require.True(t, fv.IsValid(), "no field %s", name)

	f := reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem()

	switch f.Kind() {
	case reflect.Bool:
		f.SetBool(true)
	case reflect.Float64:
		f.SetFloat(1)
	case reflect.String:
		f.SetString("b3")
	case reflect.Map:
		m := reflect.MakeMap(f.Type())
		m.SetMapIndex(reflect.Zero(f.Type().Key()), reflect.Zero(f.Type().Elem()))
		f.Set(m)
	case reflect.Ptr:
		f.Set(reflect.New(f.Type().Elem()))
	default:
		t.Fatalf("b3SetNonZero: %s is a %s; teach this helper its non-zero", name, f.Kind())
	}
}

// b3Assignments is, for every method of Game in this package's source, the
// fields it assigns (v.<field> = ...).
func b3Assignments(t *testing.T) map[string]map[string]bool {
	t.Helper()

	fset := token.NewFileSet()

	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	require.NoError(t, err)

	out := map[string]map[string]bool{}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}

				fields := map[string]bool{}

				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if as, ok := n.(*ast.AssignStmt); ok {
						for _, lhs := range as.Lhs {
							if sel, ok := lhs.(*ast.SelectorExpr); ok {
								if id, ok := sel.X.(*ast.Ident); ok && id.Name == "v" {
									fields[sel.Sel.Name] = true
								}
							}
						}
					}

					return true
				})

				out[fn.Name.Name] = fields
			}
		}
	}

	return out
}

// B6: THE PLAYER'S AND THE CONTROLS' FIELDS, EACH CLASSIFIED, as the game
// screen's are (TestGameFieldsClassified). B3 saved his place, health and
// run toggle and nothing classified the rest, so his facing -- which the
// digest compares -- was dropped without a word. A field added to either
// fails here until someone decides.
//
//	"S:hero.<key>"  saved at that key of the world file's hero block
//	"O: why"        the .od2's (upstream's save), which Strigoi's game does not
//	                change after the hero is made -- or re-derives on load
//	"D: why"        derived on load
//	"T: why"        refused while set
//	"W: why"        wiring
var b3PlayerClasses = map[string]string{
	"mapEntity":         "W: embedded; its fields are labelled below",
	"name":              "S:hero.name",
	"animationMode":     "D: the body's mode, re-read from it every frame; a load stands him still (rule 4)",
	"composite":         "S:hero.facing",
	"staminaRunDrain":   "D: his class's run drain, read from charstats when he is made",
	"Equipment":         "O: Diablo II's equipment; Strigoi's gear is the kit, in the sidecar",
	"Stats":             "W: his stats; the fields are labelled below",
	"Skills":            "O: Diablo II's skills; Strigoi's hero has none",
	"LeftSkill":         "O: Diablo II's skills; Strigoi's hero has none",
	"RightSkill":        "O: Diablo II's skills; Strigoi's hero has none",
	"Class":             "S:hero.class",
	"Gold":              "O: never changed by Strigoi's game",
	"lastPathSize":      "D: the length of his path, re-read every frame; a load stands him still",
	"isInTown":          "D: set by the client from the region under him",
	"isRunToggled":      "S:hero.run",
	"isRunning":         "D: set with the run toggle (the HUD's run button, SetIsRunning); a load sets it from hero.run",
	"isCasting":         "T: his swing still playing puts him in combat, and COMBAT refuses the save (FIGHTING's until the combat status, 30 Sep 2026)",
	"castMode":          "D: requested skill pose; StandAt clears it on reconstruction (rule 4)",
	"actionHeld":        "T: his hit or block reaction still playing puts him in combat, and COMBAT refuses the save (the combat status closed BUG-105, 30 Sep 2026); his death is DEAD's; StandAt normalizes the living hero to idle (rule 4)",
	"actionMode":        "D: visual reaction only; StandAt clears it on reconstruction (rule 4)",
	"corpse":            "D: visual terminal death; dead heroes cannot save and StandAt reconstructs a living pose",
	"onFinishedCasting": "W: a cast's callback; nil in Strigoi's game",
	"Act":               "O: never changed by Strigoi's game",

	// mapEntity, the player's.
	"uuid":        "D: his connection's id, new every launch; the entity list leaves him out",
	"Position":    "S:hero.pos",
	"Target":      "D: rule 4 -- a walk does not continue; he stands at pos",
	"velocity":    "D: rule 4; zero while he stands",
	"Speed":       "D: set from the run toggle and his stamina (SetIsRunning, Advance)",
	"path":        "D: rule 4; empty while he stands",
	"drawLayer":   "D: never set on the player",
	"done":        "W: a walk's arrival callback",
	"directioner": "W: the facing hook",
	"highlight":   "D: a render flag, cleared every frame",

	// HeroStatsState.
	"Level":        "O: Strigoi's experience is the sidecar's progress; this is never changed",
	"Experience":   "O: Strigoi's experience is the sidecar's progress; this is never changed",
	"Strength":     "O: Diablo II's attribute; 0 and never changed in Strigoi's game",
	"Energy":       "O: Diablo II's attribute; 0 and never changed in Strigoi's game",
	"Dexterity":    "O: Diablo II's attribute; 0 and never changed in Strigoi's game",
	"Vitality":     "O: Diablo II's attribute; 0 and never changed in Strigoi's game",
	"StatsPoints":  "O: Diablo II's; 0 and never changed in Strigoi's game",
	"SkillPoints":  "O: Diablo II's; 0 and never changed in Strigoi's game",
	"Health":       "S:hero.health",
	"MaxHealth":    "D: re-derived on every load from the sidecar's BaseMaxHealth and his talents (applyProgress); the .od2's is read once, to seed BaseMaxHealth",
	"Mana":         "O: Diablo II's; 0 and never changed in Strigoi's game",
	"MaxMana":      "O: Diablo II's; 0 and never changed in Strigoi's game",
	"Stamina":      "S:hero.stamina",
	"MaxStamina":   "O: his body's (the manifest), set when he is made",
	"NextLevelExp": "D: computed, never saved",
}

var b3ControlsClasses = map[string]string{
	"keyMap":                 "W: the key map",
	"actionableRegions":      "W: the HUD's clickable rectangles",
	"asset":                  "W: the asset manager",
	"renderer":               "W: the renderer",
	"inputListener":          "W: the input listener",
	"hero":                   "W: the player, classified on its own",
	"heroState":              "W: the hero factory",
	"mapRenderer":            "W: the map renderer",
	"escapeMenu":             "W: the escape menu",
	"ui":                     "W: the UI manager",
	"inventory":              "W: the inventory panel; a load opens with it closed, as a new game does",
	"hud":                    "W: the HUD; its strip is recomputed from the clock",
	"skilltree":              "W: the skill tree panel",
	"heroStatsPanel":         "W: the stats panel",
	"PartyPanel":             "W: the party panel",
	"questLog":               "W: the quest log",
	"HelpOverlay":            "W: the help overlay",
	"bottomMenuRect":         "W: a layout rectangle",
	"leftMenuRect":           "W: a layout rectangle",
	"rightMenuRect":          "W: a layout rectangle",
	"lastMouseX":             "D: the pointer, re-read on its next move",
	"lastMouseY":             "D: the pointer, re-read on its next move",
	"lastLeftBtnActionTime":  "D: a held click's repeat stamp against clock, which restarts with it",
	"lastRightBtnActionTime": "D: a held click's repeat stamp against clock, which restarts with it",
	// BUG-120 (integrate-4: its fields met this table in the merge): a held
	// left button's latch and its press point, set by the press and cleared
	// by the next press or the release -- a new game has no button held.
	"heldOnSquad":    "D: a held select's latch, set by the left press that begins the hold; no button is held in a new game",
	"heldPressX":     "D: the held press's screen point, written by every left press",
	"heldPressY":     "D: the held press's screen point, written by every left press",
	"FreeCam":        "D: the debug camera, off in every new game",
	"isSinglePlayer": "W: from the client",
	"combat":         "W: the combat model, saved as the combat block",
	"light":          "W: the light model, saved as the light block",
	"torchesCarried": "D: the ration of a hero with no kit; read only when no kit is bound, and a save with no kit is refused (NOT_READY)",
	"torchVerbs":     "D: a count of this process's torch verbs for the harness (ui.torch_verbs); every process starts it at 0",
	"kitHolder":      "W: the game screen",
	"worldHolder":    "W: the game screen",
	"progressHolder": "W: the game screen",
	"deathHolder":    "W: the game screen",
	"talkHolder":     "W: the game screen",
	"forageHolder":   "W: the game screen",
	"corpseHolder":   "W: the game screen",
	"journalHolder":  "W: the game screen",
	"combatHolder":   "W: the game screen (the combat marker's status, 30 Sep 2026)",
	"squads":         "W: the squads owner, saved as the squads block",
	"clock":          "D: the controls' own seconds for click repeat; only differences are read, and it restarts with the stamps",
	"wheelAcc":       "D: a fraction of a wheel notch not yet turned; every process starts it at 0",
	"Logger":         "W: the log",
}

func TestPlayerAndControlsFieldsClassified(t *testing.T) {
	heroKeys := map[string]bool{}

	typ := reflect.TypeOf(d2save.Hero{})
	for i := 0; i < typ.NumField(); i++ {
		heroKeys[strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]] = true
	}

	check := func(classes map[string]string, types ...reflect.Type) {
		have := map[string]bool{}

		for _, typ := range types {
			for i := 0; i < typ.NumField(); i++ {
				name := typ.Field(i).Name
				have[name] = true

				class, ok := classes[name]

				switch {
				case !ok:
					t.Errorf("%s.%s is not classified: decide whether the world save carries it (S:hero.<key>), "+
						"the .od2 does and Strigoi never changes it (O), a load derives it (D), a save refuses it (T), or it is wiring (W)",
						typ.Name(), name)
				case strings.HasPrefix(class, "S:hero."):
					if !heroKeys[strings.TrimPrefix(class, "S:hero.")] {
						t.Errorf("%s.%s is saved at %q, and the hero block has no such key", typ.Name(), name, class)
					}
				case strings.HasPrefix(class, "O: "), strings.HasPrefix(class, "D: "),
					strings.HasPrefix(class, "T: "), strings.HasPrefix(class, "W: "):
					if len(strings.TrimSpace(class[3:])) < 6 {
						t.Errorf("%s.%s: a %c label needs its reason", typ.Name(), name, class[0])
					}
				default:
					t.Errorf("%s.%s: label %q is not S:hero., O:, D:, T: or W:", typ.Name(), name, class)
				}
			}
		}

		for name := range classes {
			if !have[name] {
				t.Errorf("%q is classified, and is not a field", name)
			}
		}
	}

	mapEntity, ok := reflect.TypeOf(d2mapentity.Player{}).FieldByName("mapEntity")
	require.True(t, ok)

	check(b3PlayerClasses, reflect.TypeOf(d2mapentity.Player{}), mapEntity.Type, reflect.TypeOf(d2hero.HeroStatsState{}))
	check(b3ControlsClasses, reflect.TypeOf(d2player.GameControls{}))

	// The two T: his swing still playing, and his hit or block reaction (BUG-105),
	// each put him in combat, which the save refuses (the combat status, 30 Sep
	// 2026: his swing was FIGHTING's before it).
	v, save := b3SavableGame(t)
	b3SetField(t, v.localPlayer, "isCasting", true)
	b3Refused(t, v, SaveRefusedCombat, SaveOptions{To: filepath.Join(filepath.Dir(save), "c.world.json")})

	v, save = b3SavableGame(t)
	b3SetField(t, v.localPlayer, "actionHeld", true)
	b3SetField(t, v.localPlayer, "actionMode", d2enum.PlayerAnimationModeGetHit)
	b3Refused(t, v, SaveRefusedCombat, SaveOptions{To: filepath.Join(filepath.Dir(save), "r.world.json")})
}
