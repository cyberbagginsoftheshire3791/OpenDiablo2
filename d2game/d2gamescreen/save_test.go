package d2gamescreen

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
)

// M4.6 B3: the save verb's refusals, each one leaving every file as it was.
// The acts that need a real fight and a real death are TestSaveResume act 7's
// (playtest/save_resume_test.go); these are the screen's own states, which a
// unit test can set exactly.

func b3Asset(t *testing.T) *d2asset.AssetManager {
	t.Helper()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	require.NoError(t, err)
	require.NoError(t, asset.AddSource(filepath.Join("..", ".."), types.AssetSourceFileSystem))

	return asset
}

// b3Files is a hero's save folder with every file a save writes already in
// it, each holding its own name, so a save that touched one is seen.
func b3Files(t *testing.T) (save string, snapshot func() map[string]string) {
	t.Helper()

	dir := t.TempDir()
	save = filepath.Join(dir, "0.od2")

	for _, p := range []string{save, save + ".bak", d2items.SidecarPath(save), d2save.WorldPath(save), d2save.WorldPath(save) + ".bak"} {
		require.NoError(t, os.WriteFile(p, []byte("before: "+filepath.Base(p)), 0o600))
	}

	snapshot = func() map[string]string {
		out := map[string]string{}

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			require.NoError(t, err)

			out[e.Name()] = string(data)
		}

		return out
	}

	return save, snapshot
}

// b3Game is a game screen that is in the world and doing nothing a save
// refuses -- but built without its world systems, so a save that got past
// every refusal of the screen's own is refused NOT_READY for those. The
// cases below each set one state and read which refusal came first.
func b3Game(t *testing.T, engine *d2mapengine.MapEngine, save string) *Game {
	t.Helper()

	v := &Game{
		gameClient:  &d2client.GameClient{MapEngine: engine, SaveFilePath: save},
		localPlayer: &d2mapentity.Player{Stats: &d2hero.HeroStatsState{Health: 50, MaxHealth: 60}},
		kit:         &d2items.Kit{},
		kitPath:     d2items.SidecarPath(save),
	}
	v.Logger = d2util.NewLogger()
	v.Logger.SetLevel(d2util.LogLevelNone)

	return v
}

func b3Refused(t *testing.T, v *Game, code string, opts SaveOptions) {
	t.Helper()

	_, err := v.SaveWorld(opts)

	var r *SaveRefusal

	require.True(t, errors.As(err, &r), "want a %s refusal, got %v", code, err)
	require.Equal(t, code, r.Code, "reason: %s", r.Reason)
	require.ErrorIs(t, err, ErrSaveRefused)
}

func TestEverySaveRefusalTouchesNoFile(t *testing.T) {
	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, b3Asset(t))

	cases := []struct {
		name string
		code string
		set  func(v *Game)
	}{
		{"no game", SaveRefusedNotReady, func(v *Game) { v.gameClient = nil }},
		{"no hero save", SaveRefusedNotReady, func(v *Game) { v.gameClient.SaveFilePath = "" }},
		{"not in the world yet", SaveRefusedNotReady, func(v *Game) { v.localPlayer = nil }},
		{"dead: the death screen's flag", SaveRefusedDead, func(v *Game) { v.died = true }},
		{"dead: his body at 0", SaveRefusedDead, func(v *Game) { v.localPlayer.Stats.Health = 0 }},
		{"dead in a fight is DEAD", SaveRefusedDead, func(v *Game) { v.died, v.wasFighting = true, true }},
		{"the fight's edge not taken back", SaveRefusedFighting, func(v *Game) { v.wasFighting = true }},
		{"a strike waiting on a walk", SaveRefusedFighting, func(v *Game) { v.pendingStrike = "0a-wolf" }},
		{"a tactical pace borrowed", SaveRefusedFighting, func(v *Game) { v.tacticalPace = map[string]float64{"0a-wolf": 2} }},
		{"a tactical tile reserved", SaveRefusedFighting, func(v *Game) { v.tacticalReserved = map[string][2]int{"0a-wolf": {1, 2}} }},
		{"the journal reading a fight", SaveRefusedFighting, func(v *Game) { v.journalFight = "e:3" }},
		{"talking", SaveRefusedTalking, func(v *Game) { v.talk = &d2dialogue.Talk{NodeID: "greet"} }},
		{"a talk on its last line", SaveRefusedTalking, func(v *Game) { v.talk = &d2dialogue.Talk{} }},
		{"the journal open", SaveRefusedJournal, func(v *Game) { v.journalOpen = true }},
		{"choosing a loadout", SaveRefusedLoadout, func(v *Game) { v.choosingLoadout = true }},
		{"no kit", SaveRefusedNotReady, func(v *Game) { v.kit = nil }},
		// THE CONTROL: none of the screen's states set, and the save goes on
		// past every one of them to the world systems this game lacks.
		{"nothing refused but the missing systems", SaveRefusedNotReady, func(v *Game) {}},
	}

	for _, tc := range cases {
		save, snapshot := b3Files(t)
		before := snapshot()

		v := b3Game(t, engine, save)
		tc.set(v)

		b3Refused(t, v, tc.code, SaveOptions{})
		require.Equal(t, before, snapshot(), "%s: a refusal touches no file", tc.name)

		// Nor does a refused save to another path write anything there.
		to := filepath.Join(filepath.Dir(save), "elsewhere.world.json")

		b3Refused(t, v, tc.code, SaveOptions{To: to, Omit: []string{"corpses"}})
		require.Equal(t, before, snapshot(), "%s: a refused save to another path writes nothing", tc.name)
	}

	// The control's reason is the systems, not a screen state.
	save, _ := b3Files(t)
	_, err := b3Game(t, engine, save).SaveWorld(SaveOptions{})
	require.Contains(t, err.Error(), "systems")
}

// Rule 9: a network game is not saved -- the host's TCP handlers race the
// counted world stream (C13).
func TestANetworkGameIsNotSaved(t *testing.T) {
	asset := b3Asset(t)

	for _, conn := range []d2clientconnectiontype.ClientConnectionType{d2clientconnectiontype.LANServer, d2clientconnectiontype.LANClient} {
		client, err := d2client.Create(conn, asset, d2util.LogLevelNone, nil)
		require.NoError(t, err)

		save, snapshot := b3Files(t)
		before := snapshot()

		v := b3Game(t, client.MapEngine, save)
		v.gameClient = client
		client.SaveFilePath = save

		b3Refused(t, v, SaveRefusedNetwork, SaveOptions{})
		require.Equal(t, before, snapshot())
	}

	// The control: a local game's client is past this refusal.
	client, err := d2client.Create(d2clientconnectiontype.Local, asset, d2util.LogLevelNone, nil)
	require.NoError(t, err)

	save, _ := b3Files(t)
	v := b3Game(t, client.MapEngine, save)
	v.gameClient = client
	client.SaveFilePath = save

	_, err = v.SaveWorld(SaveOptions{})

	var r *SaveRefusal
	require.True(t, errors.As(err, &r))
	require.NotEqual(t, SaveRefusedNetwork, r.Code)
}

// A save that omits a block writes a file no load reads, so it must go
// somewhere else: never over his real save.
func TestOmitNeedsTo(t *testing.T) {
	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, b3Asset(t))
	save, snapshot := b3Files(t)
	before := snapshot()

	_, err := b3Game(t, engine, save).SaveWorld(SaveOptions{Omit: []string{"corpses"}})
	require.ErrorIs(t, err, errOmitNeedsTo)
	require.Equal(t, before, snapshot())
}

// THE GAME SCREEN'S FIELDS, EACH CLASSIFIED (M4.6 B3), as B2 classified every
// world system's: a field added to Game fails here until someone decides what
// the world save does with it.
//
//	"S:<path>"  saved, at that path of the world file (checked against the
//	            file's blocks)
//	"D: why"    derived on load, or re-taken
//	"T: why"    only ever set while a save is refused, or empty whenever one
//	            is allowed (fightUnsettled, saveRefusal)
//	"W: why"    wiring: tables, handles, the engine, the UI
var b3GameClasses = map[string]string{
	"MapEntityFactory": "W: the engine's entity factory, the client's",
	"asset":            "W: the asset manager",
	"bestiary":         "W: the bestiary table",
	"deadNames":        "W: the words for the dead, loaded from the tables",

	"pendingStrike":      "T: a strike waits on a walk only in a fight (fightUnsettled refuses)",
	"pendingStrikeStill": "T: the strike's settle count; read only while pendingStrike is set",
	"tacticalPace":       "T: borrowed paces, returned every frame no fight runs (fightUnsettled refuses a non-empty table)",
	"tacticalReserved":   "T: reserved tiles, emptied every frame no fight runs (fightUnsettled refuses)",

	"items":           "W: the item table",
	"kit":             "S:sidecar",
	"kitPath":         "D: the sidecar's path, from the save path the load opens",
	"choosingLoadout": "T: LOADOUT refuses",
	"talents":         "W: the talent table",
	"dialogue":        "W: the dialogue table",
	"recipes":         "W: the craft table",
	"land":            "S:sidecar",
	"journalBook":     "W: the journal table",
	"journal":         "S:sidecar",
	"journalOpen":     "T: JOURNAL refuses",
	"journalQuiet":    "D: true on the first frame after any bind: a resumed game's first frame writes what the saved one would, without the notice",
	"journalFight":    "T: set only while a fight runs, cleared on the first frame none does (fightUnsettled refuses)",
	"journalRows":     "T: the rows seen in journalFight's encounter, cleared with it",
	"fieldDead":       "S:scene.field_dead",
	"spawner":         "S:spawner",
	"natives":         "D: captured when the screen is made, from the map the load builds again, by (name_key, born) and held by the entity; saved as entities[].native and born",
	"saveGeneration":  "S:saved_at",
	"autosave":        "D: the dawn autosave (M4.6 B5): a save takes a waiting one (cover), so none is pending in a file, and the file's dawn_paid_day keeps the resumed day from arming another",
	"lastSave":        "D: the last save this screen asked for, for the harness's save provider (process history)",
	"loadNotice":      "D: what the load that opened this screen told him (M4.6 B5); a resumed game's own load says its own",
	"autosaveOff":     "D: the harness's dial save.autosave, re-applied by a load as every dial is; false in the shipped game",
	"closing":         "D: the close hook under way (the B5 review, A1): its settle frames take no autosave of their own, and the process ends after it; never in a file",
	"exitSavedHero":   "D: a SAVE AND EXIT or close whose save wrote his .od2 and sidecar, so the unload does not write them again (the B5 review, C2); any frame clears it",
	"closeAskedAt":    "D: when a window's close in combat last asked (the combat-status review, A2), wall time; a person's hand on the window, never in a file",

	// The combat status (30 Sep 2026): a save is never made in combat, so
	// nothing of it is in the file.
	"combatGrace":        "T: the grace after combat's last trigger, above 0 only while he is in combat (COMBAT refuses)",
	"combatLast":         "D: the last trigger the combat status saw, for the grace's reason and the log's COMBAT in/out lines; empty from the frame he is out of combat, so at every save the game makes",
	"combatSpent":        "D: seconds of play since COMBAT in, for the log's COMBAT out line; zeroed at COMBAT out, so 0 at every save the game makes (none is made in combat)",
	"combatAfter":        "D: seconds of play since combat's last trigger, for the log's COMBAT out line; zeroed at a trigger and at COMBAT out, so 0 at every save the game makes",
	"combatGraceSeconds": "D: the grace's dial: DefaultCombatGraceSeconds in the shipped game, the harness's save.combat_grace re-applied by a load as every dial is",
	"pendingLoad":        "T: a world save checked and not yet restored: its first frame restores it before anything else runs, and NOT_READY refuses a save meanwhile (M4.6 B4a)",
	"loadSteps":          "D: the load's own record of the steps it ran, for the harness's report; a fresh screen's is empty",
	"loadAbandoned":      "T: a load refused on its first frame: the game is torn down, and NOT_READY refuses a save from it",
	"loadFailed":         "T: that refusal, held from the bind to the teardown within one Advance",
	"watchStood":         "S:scene.watch_stood",
	"watchClock":         "S:scene.watch_clock",
	"watchClockSet":      "S:scene.watch_clock_set",
	"corpses":            "S:corpses",
	"rising":             "S:rising",
	"standing":           "S:sidecar",
	"talk":               "T: TALKING refuses",
	"progress":           "S:sidecar",
	"lastStage":          "S:scene.last_stage",
	"dawnPaidDay":        "S:scene.dawn_paid_day",

	"navigator":            "W: the App",
	"joinRefusalShown":     "D: set only in a network game whose host refused the join; NETWORK refuses every network game, so it is false in every game a save is made from",
	"died":                 "T: DEAD refuses",
	"death":                "T: how he died; set only with died",
	"heroAtEntry":          "D: the death screen's copy of his sidecar, re-taken by every save",
	"noticeDelta":          "D: the Quiet Step radius change, applied again from his talents by applyProgress",
	"gameClient":           "W: the client; its seed is saved as seed",
	"mapRenderer":          "W: the renderer",
	"uiManager":            "W: the UI",
	"gameControls":         "W: the controls",
	"localPlayer":          "S:hero",
	"lastRegionType":       "D: the zone-change banner's edge: presentation, re-read within a second",
	"ticksSinceLevelCheck": "D: the zone-change banner's clock: presentation",
	"escapeMenu":           "W: the escape menu",
	"soundEngine":          "W: the sound engine",
	"soundEnv":             "W: the sound environment",
	"guiManager":           "W: the UI",
	"keyMap":               "W: the key map",

	"worldClock":   "S:clock",
	"light":        "S:light",
	"fog":          "W: fog of war F1 (game_fog.go): display only, and NOT saved until F3 -- a load starts black (docs/fog.md)",
	"squads":       "S:squads",
	"meters":       "S:squads",
	"metersBodied": "D: s:1 is bound to his body on the first frame that has one",
	"pursuit":      "S:pursuit",
	"notice":       "S:notice",
	"spawns":       "S:spawns",
	"combat":       "S:combat",
	"seek":         "S:seek",

	"wasFighting":         "T: true only from a fight's first frame to the frame its end is applied (fightUnsettled refuses)",
	"activityBeforeFight": "T: read only at a fight's end, written at its start; stale between fights by design",

	"paceRoundKey":          "D: the last ROUND line written; B4 sets it from combat.last_round so the first frame after a load writes no line for a round already logged",
	"paceOpenClock":         "T: stamped at a fight's start, read at its end",
	"paceOpenDay":           "T: stamped at a fight's start, read at its end",
	"torchLitRounds":        "T: reset at a fight's start, read at its end",
	"torchWasOut":           "D: always false at the end of a frame: a burnt-out torch is removed on the frame it is seen",
	"torchOutThisFight":     "T: reset at a fight's start, read at its end",
	"torchesBurntThisFight": "T: reset at a fight's start, read at its end",

	"bodies": "S:bodies",

	"renderer":      "W: the renderer",
	"inputManager":  "W: the input manager",
	"audioProvider": "W: the audio provider",
	"terminal":      "W: the console",
	"Logger":        "W: the log",
	"logLevel":      "W: the log level",
}

func TestGameFieldsClassified(t *testing.T) {
	typ := reflect.TypeOf(Game{})
	have := map[string]bool{}

	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		have[name] = true

		class, ok := b3GameClasses[name]

		switch {
		case !ok:
			t.Errorf("Game.%s is not classified: decide whether the world save carries it (S:<path>), "+
				"derives it (D), refuses while it is set (T), or it is wiring (W)", name)
		case strings.HasPrefix(class, "S:"):
			block := strings.Split(strings.TrimPrefix(class, "S:"), ".")[0]
			if !b3IsBlock(block) {
				t.Errorf("Game.%s is saved at %q, and the world file has no %q block", name, class, block)
			}
		case strings.HasPrefix(class, "D: "), strings.HasPrefix(class, "T: "), strings.HasPrefix(class, "W: "):
			if len(strings.TrimSpace(class[3:])) < 6 {
				t.Errorf("Game.%s: a %c label needs its reason", name, class[0])
			}
		default:
			t.Errorf("Game.%s: label %q is not S:, D:, T: or W:", name, class)
		}
	}

	var stale []string

	for name := range b3GameClasses {
		if !have[name] {
			stale = append(stale, name)
		}
	}

	sort.Strings(stale)
	require.Empty(t, stale, "classified, but not fields of Game")
}

func b3IsBlock(name string) bool {
	for _, b := range d2save.Blocks {
		if b == name {
			return true
		}
	}

	return false
}

// b3Dog places a feral dog (Strigoi's own PNG art, no MPQs) at sub-tiles x, y.
func b3Dog(t *testing.T, engine *d2mapengine.MapEngine, x, y int) *d2mapentity.Creature {
	t.Helper()

	dog, err := engine.MapEntityFactory.NewCreature(x, y, "Feral dog", d2mapentity.CreatureAnimationPaths{
		Idle: "/data/strigoi/creatures/feral-dog/idle.png",
		Walk: "/data/strigoi/creatures/feral-dog/walk.png",
	}, 0, nil)
	require.NoError(t, err)
	engine.AddEntity(dog)

	return dog
}

// THE ENTITY LIST (M4.6 B3): every creature on the map, sorted by id, with its
// bestiary entry, its place in world tiles and its motion in sub-tiles; the
// ones the map had when the screen was made are native, with where they
// stood. A creature built from no bestiary entry is refused -- no load could
// rebuild it -- rather than saved as something else.
func TestTheEntityList(t *testing.T) {
	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, b3Asset(t))
	engine.ResetAuthoredMap(d2enum.RegionAct1Town, 40, 40)

	native := b3Dog(t, engine, 50, 55)
	native.SetCreatureID("feral-dog")

	v := &Game{gameClient: &d2client.GameClient{MapEngine: engine}}
	v.natives = nativesOf(engine)

	placed := b3Dog(t, engine, 80, 60)
	placed.SetCreatureID("feral-dog")

	list, err := v.saveEntities()
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.True(t, list[0].ID < list[1].ID, "sorted by id")

	byID := map[string]d2save.Entity{}
	for _, e := range list {
		byID[e.ID] = e
	}

	n, p := byID[native.ID()], byID[placed.ID()]
	require.True(t, n.Native)
	require.Equal(t, &[2]float64{10, 11}, n.Born, "where the map had it, in world tiles")
	require.False(t, p.Native)
	require.Nil(t, p.Born)
	require.Equal(t, d2save.KindCreature, p.Kind)
	require.Equal(t, "feral-dog", p.Creature)
	require.Equal(t, [2]float64{80, 60}, p.Motion.Pos, "the motion is in sub-tiles")
	require.Equal(t, 16.0, p.X, "x, y are world tiles")

	// Observable first: the scene provider reports the natives.
	natives := sceneProvider{v}.HarnessState()["natives"].([]map[string]interface{})
	require.Len(t, natives, 1)
	require.Equal(t, native.ID(), natives[0]["id"])

	// A creature from no bestiary entry cannot be saved.
	b3Dog(t, engine, 90, 90)

	_, err = v.saveEntities()
	require.Error(t, err)
	require.Contains(t, err.Error(), "bestiary")
}

// The save's Resolver is a lookup over the live map, as the live path wraps
// things: an entity id is that entity as a chaser; "player" is the local
// player as prey; anything else, or nothing, does not resolve.
func TestTheWorldResolver(t *testing.T) {
	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, b3Asset(t))
	engine.ResetAuthoredMap(d2enum.RegionAct1Town, 40, 40)
	dog := b3Dog(t, engine, 50, 55)

	v := &Game{gameClient: &d2client.GameClient{MapEngine: engine}}
	r := worldResolver{v}

	w, ok := r.Watcher(dog.ID())
	require.True(t, ok)
	require.Equal(t, dog.ID(), w.WatcherID())

	q, ok := r.Quarry(dog.ID())
	require.True(t, ok)
	require.Equal(t, dog.ID(), q.QuarryID())

	for _, id := range []string{"", "player", "no-such-entity"} {
		_, ok := r.Watcher(id)
		require.False(t, ok, "watcher %q", id)
	}

	_, ok = r.Quarry("player")
	require.False(t, ok, "no player in the world, no quarry called player")

	v.localPlayer = &d2mapentity.Player{}
	_, ok = r.Quarry("player")
	require.True(t, ok)
}
