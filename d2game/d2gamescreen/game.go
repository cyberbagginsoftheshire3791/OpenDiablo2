package d2gamescreen

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2gui"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2audio"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2screen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket"
)

const hideZoneTextAfterSeconds = 2.0

const (
	moveErrStr         = "failed to send MovePlayer packet to the server, playerId: %s, x: %g, x: %g\n"
	bindControlsErrStr = "failed to add gameControls as input handler for player: %s\n"
	castErrStr         = "failed to send CastSkill packet to the server, playerId: %s, skillId: %d, x: %g, x: %g\n"
	spawnItemErrStr    = "failed to send SpawnItem packet to the server: (%d, %d) %+v"
)

const (
	black50alpha = 0x0000007f // rgba
)

// CreateGame creates the Gameplay screen and returns a pointer to it
func CreateGame(
	navigator d2interface.Navigator,
	asset *d2asset.AssetManager,
	ui *d2ui.UIManager,
	renderer d2interface.Renderer,
	inputManager d2interface.InputManager,
	audioProvider d2interface.AudioProvider,
	gameClient *d2client.GameClient,
	term d2interface.Terminal,
	l d2util.LogLevel,
	guiManager *d2gui.GuiManager,
) (*Game, error) {
	// find the local player and its initial location
	var startX, startY float64

	for _, player := range gameClient.Players {
		if player.ID() != gameClient.PlayerID {
			continue
		}

		worldPosition := player.Position.World()
		startX, startY = worldPosition.X(), worldPosition.Y()

		break
	}

	keyMap := d2player.GetDefaultKeyMap(asset)

	game := &Game{
		asset:                asset,
		gameClient:           gameClient,
		gameControls:         nil,
		localPlayer:          nil,
		lastRegionType:       d2enum.RegionNone,
		ticksSinceLevelCheck: 0,
		mapRenderer: d2maprenderer.CreateMapRenderer(asset, renderer,
			gameClient.MapEngine, term, l, startX, startY),
		escapeMenu:    d2player.NewEscapeMenu(navigator, renderer, audioProvider, ui, guiManager, asset, l, keyMap),
		inputManager:  inputManager,
		audioProvider: audioProvider,
		renderer:      renderer,
		terminal:      term,
		soundEngine:   d2audio.NewSoundEngine(audioProvider, asset, l, term),
		uiManager:     ui,
		guiManager:    guiManager,
		keyMap:        keyMap,
		logLevel:      l,
	}
	// The world clock and the light it drives (M4.1, S1 §3–§4). Built here,
	// at construction, so they are registered providers from the screen's
	// first frame; Game.OnUnload closes them.
	game.worldClock = d2world.NewClock(d2world.DefaultClockDials())
	game.light = d2world.NewLight(game.worldClock, d2world.DefaultLightDials())
	game.light.SetPlayer(startX, startY)

	// The squads the player commands (M4.4c-1, S1 §5), the "meters" provider.
	// s:1 is the player's squad of one; the health it spends belongs to a
	// player entity that does not exist yet, so the body is attached on the
	// first frame that has one — see advanceWorld — exactly as the single
	// meters was. Each squad has its own non-registering meters; the owner
	// holds the one Register call. squadDeployer is how a second squad's model
	// becomes a real map entity (ruled ask 10(b)); s:1 deploys nothing.
	game.squads = d2world.NewSquads(game.worldClock, d2world.DefaultMeterDials(), squadDeployer{game: game})
	game.meters = game.squads.PlayerMeters()

	// Pursuit (M4.3a) routes through the map engine and steps on the same
	// world clock as everything else here.
	game.pursuit = d2world.NewPursuit(mapRouter{engine: gameClient.MapEngine}, d2world.DefaultPursuitDials())

	// Notice and the spawn tables (M4.3b). The three read as one sentence:
	// the tables decide what arrives, Notice decides it has seen you, Pursuit
	// keeps the chase honest afterwards.
	//
	// Notice takes the light model directly because *Light already satisfies
	// its one-method Illumination interface — the same trick LightSampler
	// uses in the other direction. It may know sight, distance and the light
	// at the target and NOTHING else; that fence is signed (ask 5).
	//
	// The spawn tables are seeded from the RUN's seed, not from a fresh one,
	// so two launches of one build at one seed see the same night.
	game.notice = d2world.NewNotice(
		mapSight{engine: gameClient.MapEngine},
		game.light,
		d2world.DefaultNoticeDials(),
	)
	game.spawns = d2world.NewSpawns(
		game.worldClock,
		game.notice,
		&gameSpawner{
			engine:  gameClient.MapEngine,
			asset:   asset,
			adopt:   game.adoptNPCBody,
			release: game.releaseNPCBody,
		},

		// Pursuit is the Chases seam: despawning a pack releases its members'
		// chases so a group sent home at daybreak leaves no ghost pursuit
		// re-pathing forever (item 4). *Pursuit already satisfies Chases via
		// Release; it is built above, so it is in hand here.
		game.pursuit,

		game.light,
		gameClient.Seed,
		d2world.DefaultSpawnDials(),
	)

	// Combat comes last of the world systems because it is downstream of all
	// of them: it asks Notice who is aware, LOOKS UP a squad's derived facts
	// through the Squads owner's FitnessOf (M4.4c-1: the player commands
	// squads, so "the player's fatigue" is a lookup by entity id, not a single
	// handle), and samples the light model at a participant's tile. It seeds
	// from the run's seed like the tables, so two launches of one build fight
	// the same fight: since step 4 that seed drives D8's initiative tie-break
	// and every roll of every blow.
	game.combat = d2world.NewCombat(
		game.worldClock,
		game.notice,
		game.squads,
		game.light,
		game,

		// The spawn tables answer what an enemy fights as (its pack, its
		// authored Speed and its bite), and the screen itself drives the
		// sprites -- d2world cannot import d2mapentity, so it asks through
		// the Animator interface exactly as it asks for bodies.
		game.spawns,
		game,

		// STEP 5's TWO NEW SEAMS, and they are what turn three deferred rows
		// into wired ones. The spawn tables own the morale state and the rout
		// threshold, so the resolver HURTS them and READS them rather than
		// keeping a second copy; and Pursuit is how a chase ends, which is
		// the other half of a death -- releasing without unwatching is undone
		// by startChasesForTheAware on the very next frame, so the resolver
		// does both and this is the half it cannot reach on its own.
		game.spawns,
		game.pursuit,

		gameClient.Seed,
		d2world.DefaultCombatDials(),
	)

	// The renderer asks the light model how lit each tile is; it knows the
	// model only as a LightSampler, so d2maprenderer imports no world code.
	game.mapRenderer.SetLightSampler(game.light)

	game.Logger = d2util.NewLogger()
	game.Logger.SetLevel(l)
	game.Logger.SetPrefix(logPrefix)

	game.soundEnv = d2audio.NewSoundEnvironment(game.soundEngine)

	game.escapeMenu.OnLoad()

	if err := inputManager.BindHandler(game.escapeMenu); err != nil {
		return nil, errors.New("failed to add gameplay screen as event handler")
	}

	return game, nil
}

// Game represents the Gameplay screen
type Game struct {
	*d2mapentity.MapEntityFactory
	asset                *d2asset.AssetManager
	gameClient           *d2client.GameClient
	mapRenderer          *d2maprenderer.MapRenderer
	uiManager            *d2ui.UIManager
	gameControls         *d2player.GameControls
	localPlayer          *d2mapentity.Player
	lastRegionType       d2enum.RegionIdType
	ticksSinceLevelCheck float64
	escapeMenu           *d2player.EscapeMenu
	soundEngine          *d2audio.SoundEngine
	soundEnv             d2audio.SoundEnvironment
	guiManager           *d2gui.GuiManager
	keyMap               *d2player.KeyMap

	// The simulated world's own systems (M4.1, M4.2). They advance from the
	// same delta this screen receives — the harness's when it is stepping —
	// and register themselves as the "clock", "light" and "meters" harness
	// providers. Since M4.4c-1 the "meters" provider is the Squads owner: the
	// player commands squads, each with its own meters, and squads is the one
	// registered provider (its flat face is the selected squad's). meters
	// points at the player's squad (s:1) for the fighting-activity edge.
	// metersBodied records that the local player's health has been handed to
	// s:1, which cannot happen at construction because the player does not
	// exist yet.
	worldClock   *d2world.Clock
	light        *d2world.Light
	squads       *d2world.Squads
	meters       *d2world.Meters
	metersBodied bool
	pursuit      *d2world.Pursuit
	notice       *d2world.Notice
	spawns       *d2world.Spawns
	combat       *d2world.Combat

	// wasFighting and activityBeforeFight are how "fighting is labour" is
	// applied and then taken back. S1 §5's Food row signs the DRAIN --
	// "faster when digging, fighting, carrying" -- and this is the edge that
	// applies it: the meters' activity goes to labour when a fight opens and
	// back to whatever the body was doing when it ends.
	wasFighting         bool
	activityBeforeFight d2world.Activity

	// The pace instrument (M4.4c-2a). It is written HERE and not in d2world:
	// that package has no logger and must not grow one -- zero ebiten and zero
	// logging is what the gate checks on it -- and the two facts the lines
	// need that d2world cannot know, the world clock stamp and the carried
	// torch, both live on this screen.
	//
	// paceRound is the round the last ROUND line was written for: the round
	// EDGE is Combat.Round() changing, and this is what remembers where it
	// was. paceOpen/paceOpenClock hold the fight's opening stamp until it
	// closes; torchLitRounds counts the rounds the player spent lit, which is
	// a light fact and so cannot come from the combat provider.
	paceRoundKey   string
	paceOpenClock  string
	paceOpenDay    int
	torchLitRounds int
	torchWasOut    bool

	// bodies is where a monster's health lives (M4.5 step 3), keyed by the
	// entity id d2world knows it by. It is on the screen rather than on the
	// entity for the reasons npc_body.go states. Game satisfies
	// d2world.Bodies through it.
	bodies map[string]*npcBody

	renderer      d2interface.Renderer
	inputManager  d2interface.InputManager
	audioProvider d2interface.AudioProvider
	terminal      d2interface.Terminal

	*d2util.Logger
	logLevel d2util.LogLevel
}

// OnLoad loads the resources for the Gameplay screen
func (v *Game) OnLoad(_ d2screen.LoadingState) {
	v.audioProvider.PlayBGM("")

	commands := []struct {
		name string
		desc string
		args []string
		fn   func([]string) error
	}{
		{"spawnitem", "spawns an item at the local player position",
			[]string{"code1", "code2", "code3", "code4", "code5"}, v.commandSpawnItem},
		{"spawnitemat", "spawns an item at the x,y coordinates",
			[]string{"x", "y", "code1", "code2", "code3", "code4", "code5"}, v.commandSpawnItemAt},
		{"spawnmon", "spawn monster at the local player position", []string{"name"}, v.commandSpawnMon},
		// The wish note. "text..." is the variadic marker (d2term.Terminal
		// Execute): the rest of the line arrives as one argument, so a friend
		// writes `wish I wanted to hide` and does not have to remember quotes.
		{"wish", "note what you reached for that is not there",
			[]string{"text..."}, v.commandWish},
	}

	for _, cmd := range commands {
		if err := v.terminal.Bind(cmd.name, cmd.desc, cmd.args, cmd.fn); err != nil {
			v.Errorf("%s", err.Error())
		}
	}

	if err := v.asset.BindTerminalCommands(v.terminal); err != nil {
		v.Errorf("%s", err.Error())
	}
}

// OnUnload releases the resources of Gameplay screen
func (v *Game) OnUnload() error {
	d2harness.Unregister(v.gameControls) // the "ui" provider dies with the screen

	// The world's systems die with it too (M4.1).
	if v.worldClock != nil {
		v.worldClock.Close()
	}

	if v.light != nil {
		v.light.Close()
	}

	if v.spawns != nil {
		v.spawns.Close()
	}

	if v.pursuit != nil {
		v.pursuit.Close()
	}

	if v.squads != nil {
		v.squads.Close() // the registered "meters" provider; s:1's meters does not register
	}

	if v.combat != nil {
		v.combat.Close()
	}

	// The bodies go with the screen: they are keyed by entity ids that mean
	// nothing on the next map.
	v.bodies = nil

	if err := v.gameControls.UnbindTerminalCommands(v.terminal); err != nil {
		return err
	}

	// https://github.com/OpenDiablo2/OpenDiablo2/issues/792
	if err := v.inputManager.UnbindHandler(v.gameControls); err != nil {
		return err
	}

	// https://github.com/OpenDiablo2/OpenDiablo2/issues/792
	if err := v.inputManager.UnbindHandler(v.escapeMenu); err != nil {
		return err
	}

	if err := v.terminal.Unbind("spawnitemat", "spawnitem", "spawnmon", "wish"); err != nil {
		return err
	}

	// Do not write a dead hero (12 Sep 2026 ruling; audit A2): a saved 0-HP hero
	// loads as an un-killable, un-feedable corpse on every launch of that .od2.
	// The load path revives one anyway (d2hero.reviveIfDead), and the death
	// screen v0's "quit" must not save either (attack catch 4a). Before the
	// controls bind localPlayer is nil, and the original always-save stands.
	if shouldSaveOnUnload(v.localPlayer) {
		if err := v.OnPlayerSave(); err != nil {
			return err
		}
	}

	if err := v.gameClient.Close(); err != nil {
		return err
	}

	if err := v.asset.UnbindTerminalCommands(v.terminal); err != nil {
		return err
	}

	if err := v.mapRenderer.UnbindTerminalCommands(v.terminal); err != nil {
		return err
	}

	if err := v.soundEngine.UnbindTerminalCommands(v.terminal); err != nil {
		return err
	}

	v.soundEngine.Reset()

	return nil
}

// Render renders the Gameplay screen
func (v *Game) Render(screen d2interface.Surface) {
	if v.gameClient.RegenMap {
		v.gameClient.RegenMap = false
		v.mapRenderer.RegenerateTileCache()
		v.gameClient.MapEngine.IsLoading = false
	}

	screen.Clear(color.Black)
	v.mapRenderer.Render(screen)

	if v.gameControls != nil {
		if v.gameControls.HelpOverlay != nil && v.gameControls.HelpOverlay.IsOpen() {
			screen.DrawRect(screenWidth, screenHeight, d2util.Color(black50alpha))
		}

		if err := v.gameControls.Render(screen); err != nil {
			return
		}
	}
}

// Advance runs the update logic on the Gameplay screen
// nolint:gocyclo // not need to change
func (v *Game) Advance(elapsed float64) error {
	v.soundEngine.Advance(elapsed)

	// The world pauses under the escape menu (ruled 12 Sep 2026). At 344da610
	// advanceWorld ran ABOVE this gate, so the clock, meters, spawns, chases and
	// combat rounds all ran while the menu was up -- a friend who pressed Esc to
	// answer the door came back to a clock that had run ~4 world minutes per real
	// second (audit A1; §0 measured +40 world minutes over 600 stepped frames
	// under the menu). advanceWorld now shares MapEngine.Advance's condition: the
	// HUD strip may still refresh (gameControls.Advance, below), but the world
	// does not move. The gameClient.Players fence is the c-1 note's, and its
	// stated reason -- the escape-menu pause -- is finally true.
	// M4.4c-2a: the world sim and the map's animations part company here.
	//
	// advanceWorld is gated on worldRunning(), which now carries a second
	// term: an open player turn stops the world exactly as the escape menu
	// does. MapEngine.Advance is NOT gated on it, and that is a decision
	// rather than an oversight -- sprites keep breathing, a swing plays out,
	// and the player's Move (a real walk) completes while he thinks about the
	// rest of his turn. A frozen tableau would read as a hang.
	//
	// CONSEQUENCE, NAMED: an enemy already walking its last computed path
	// keeps walking during the think. It cannot strike him -- resolveBlow's
	// only callers sit under resolveRound, which the gate stops -- and
	// Pursuit does not re-path, because that lives inside advanceWorld. R2 §3
	// bullet 1's "actors outside the encounter freeze" is contradicted by
	// this, knowingly, and carries a dated marker saying so.
	if v.worldRunning() {
		v.advanceWorld(elapsed)
	}

	// The map keeps its ORIGINAL condition: the escape menu still freezes the
	// animations, because that pause is a pause of the whole screen.
	if (v.escapeMenu != nil && !v.escapeMenu.IsOpen()) || len(v.gameClient.Players) != 1 {
		v.gameClient.MapEngine.Advance(elapsed)
	}

	// The decision timer counts only frames on which the world stopped FOR THE
	// COMBAT REASON: Wait() tests awaiting itself, so a frame paused under the
	// escape menu adds nothing to the player's thinking time. It also drives
	// the fight's wall clock, which runs on every frame once the first turn
	// has opened.
	if v.combat != nil {
		v.combat.Wait(elapsed)
	}

	v.writeRoundLine()
	v.writeTorchOut()

	if v.gameControls != nil {
		if err := v.gameControls.Advance(elapsed); err != nil {
			return err
		}
	}

	v.ticksSinceLevelCheck += elapsed
	if v.ticksSinceLevelCheck > 1 {
		v.ticksSinceLevelCheck = 0
		if v.localPlayer != nil {
			tilePosition := v.localPlayer.Position.Tile()
			tile := v.gameClient.MapEngine.TileAt(int(tilePosition.X()), int(tilePosition.Y()))

			if tile != nil {
				levelDetails := v.asset.Records.Level.Details[int(tile.RegionType)]
				v.soundEnv.SetEnv(levelDetails.SoundEnvironmentID)

				// skip showing zone change text the first time we enter the world
				if v.lastRegionType != d2enum.RegionNone && v.lastRegionType != tile.RegionType {
					areaName := levelDetails.LevelDisplayName
					areaChgStr := fmt.Sprintf("Entering The %s", areaName)
					v.gameControls.SetZoneChangeText(areaChgStr)
					v.gameControls.ShowZoneChangeText()
					v.gameControls.HideZoneChangeTextAfter(hideZoneTextAfterSeconds)
				}

				v.lastRegionType = tile.RegionType
			}
		}
	}

	// Bind the game controls to the player once it exists
	if v.gameControls == nil {
		if err := v.bindGameControls(); err != nil {
			return err
		}
	}

	// Update the camera to focus on the player
	if v.localPlayer != nil && !v.gameControls.FreeCam {
		worldPosition := v.localPlayer.Position.World()
		rx, ry := v.mapRenderer.WorldToOrtho(worldPosition.X(), worldPosition.Y())
		position := d2vector.NewPosition(rx, ry)
		v.mapRenderer.SetCameraTarget(&position)
	}

	v.soundEnv.Advance(elapsed)

	if v.gameControls != nil {
		if v.gameControls.PartyPanel != nil {
			v.gameControls.PartyPanel.UpdatePlayersList(v.gameClient.Players)
		}
	}

	return nil
}

// advanceWorld moves the world clock and everything that hangs off it
// (M4.1). It is driven by the same delta the rest of the screen receives, so
// under the playtest harness's stepped clock the world is reproducible and
// nothing here ever reads the wall clock.
// worldRunning is one home for one truth, with three readers: advanceWorld's
// gate, the pace timer, and the harness.
//
// BOTH ORIGINAL TERMS ARE KEPT -- the world runs when the menu is closed OR
// the game is not single-player, because the menu never pauses a multiplayer
// world, and v1.0 of c-2's note inverted that clause. The new term is the
// player's own turn: an open turn stops the world, which is R2 §2A's "paused
// clock" and the DecisionRate dial at zero.
func (v *Game) worldRunning() bool {
	menuClosed := v.escapeMenu != nil && !v.escapeMenu.IsOpen()
	if !menuClosed && len(v.gameClient.Players) == 1 {
		return false
	}

	return v.combat == nil || !v.combat.Awaiting()
}

func (v *Game) advanceWorld(elapsed float64) {
	if v.worldClock == nil {
		return
	}

	worldMinutes := v.worldClock.Advance(elapsed)

	if v.light != nil {
		if v.localPlayer != nil {
			world := v.localPlayer.Position.World()
			v.light.SetPlayer(world.X(), world.Y())
		}

		v.light.Advance(worldMinutes)
	}

	if v.squads != nil {
		if !v.metersBodied && v.localPlayer != nil && v.localPlayer.Stats != nil {
			v.squads.BindPlayer(playerBody{player: v.localPlayer}, v.localPlayer.ID())
			v.metersBodied = true
		}

		// Every squad drains, so a harness-placed second squad drains
		// independently of the player's (brief §10). s:1's meters IS v.meters,
		// so it is advanced here exactly once and the fighting-activity edge
		// below still reads the same instance.
		v.squads.Advance(worldMinutes)
	}

	if v.pursuit != nil {
		v.pursuit.Advance(worldMinutes)
	}

	// The spawn tables come last, and they step the notice model themselves,
	// so a group that arrives this tick is evaluated this tick rather than
	// standing blind until the next one. Do not also advance v.notice here.
	//
	// The target is attached on the first frame that has a player, the same
	// way the meters' body is: the tables spawn AROUND something and watch
	// it, and at construction there is nothing to be.
	if v.spawns != nil {
		if v.localPlayer != nil {
			v.spawns.SetTarget(prey{entity: v.localPlayer})
		}

		v.spawns.Advance(worldMinutes)
	}

	v.startChasesForTheAware()

	// Combat last, and after startChasesForTheAware deliberately: a thing that
	// noticed the player on this very tick and is already in reach opens a
	// fight on the same tick rather than standing blind for one. This call
	// list IS the turn structure -- there is no queue anywhere in this engine,
	// and every system takes world minutes as a float (M4.5 §3.7).
	if v.combat != nil {
		v.combat.Advance(worldMinutes)

		v.applyFightingActivity()
	}
}

// applyFightingActivity makes a fight cost what S1 §5 says a fight costs.
//
// AFTER combat.Advance, NEVER BEFORE, and the ordering is load-bearing rather
// than tidy: the resolver reads the player's stance inside tryStart to decide
// D8 §9's caught-head-down branch, so if this ran first every fight would open
// against a player who was "labouring" and every single one would be a
// surprise. Writing here means what the resolver read is what the player was
// actually doing when the pack arrived.
//
// It gives Combat.Fighting and Meters.SetActivity their first game callers --
// both had been reachable only from the harness, which is the hollow shape
// M4.1 and M4.3b each shipped once.
// writeRoundLine writes one ROUND line per closed round. The edge is
// Combat.Round() changing, which is the only edge the game screen can see
// without the combat model calling back into it.
//
// The format is fixed and greppable, because a friend's log is read by a grep
// and not by a parser:
//
//	ROUND encounter=e:7 round=3 decide_s=8.4 action=strike move=false torch=lit
func (v *Game) writeRoundLine() {
	if v.combat == nil {
		return
	}

	row := v.combat.LastRound()
	if row.Encounter == "" {
		return
	}

	// The edge is the ROW changing, not Combat.Round(), and the difference is
	// one line per fight. Measured: gating on Fighting() lost the LAST round
	// of every fight, because the blow that ends it closes the encounter
	// before this screen looks again -- so a three-round fight wrote two ROUND
	// lines, and act 4's "one line per round" could never hold.
	key := fmt.Sprintf("%s#%d", row.Encounter, row.Round)
	if key == v.paceRoundKey {
		return
	}

	v.paceRoundKey = key

	torch := "none"

	if v.light != nil {
		if carried := v.light.Carried(); carried != nil {
			torch = "unlit"
			if carried.Lit {
				torch = "lit"

				v.torchLitRounds++
			}
		}
	}

	v.Infof("ROUND encounter=%s round=%d decide_s=%.1f action=%s move=%v torch=%s",
		row.Encounter, row.Round, row.DecideSeconds, actionOrNone(row.Action), row.Move, torch)
}

// actionOrNone keeps the field present when the policy resolved the round --
// an absent field reads as a missing round rather than a round with no commit,
// and the units guard in state.md is about exactly that difference.
func actionOrNone(action string) string {
	if action == "" {
		return "none"
	}

	return action
}

// writeTorchOut fires once, when the carried source burns out. Criterion 3 of
// R2 §4 asks for telemetry that light exhaustion ACTUALLY OCCURS, and a count
// of lit rounds does not say that -- a torch can be lit all night and never
// run out.
func (v *Game) writeTorchOut() {
	if v.light == nil || v.worldClock == nil {
		return
	}

	carried := v.light.Carried()
	out := carried != nil && carried.Burn <= 0

	if out && !v.torchWasOut {
		encounter := "-"
		if v.combat != nil && v.combat.Fighting() {
			encounter = v.combat.LastRound().Encounter
		}

		v.Infof("TORCH_OUT day=%d clock=%s encounter=%s",
			v.worldClock.DayIndex(), v.worldClock.TimeOfDay(), encounter)
	}

	v.torchWasOut = out
}

func (v *Game) applyFightingActivity() {
	if v.meters == nil {
		return
	}

	fighting := v.combat.Fighting()

	switch {
	case fighting && !v.wasFighting:
		v.activityBeforeFight = v.meters.Activity()

		v.meters.SetActivity(d2world.ActivityLabour)

		v.wasFighting = true

		// The fight edge the PACE line hangs on. The opening stamp is taken
		// here because the clock has moved by the time the fight closes.
		if v.worldClock != nil {
			v.paceOpenDay, v.paceOpenClock = v.worldClock.DayIndex(), v.worldClock.TimeOfDay()
		}

		v.torchLitRounds = 0
		v.paceRoundKey = ""

	case !fighting && v.wasFighting:
		// ONLY PUT BACK WHAT WE REPLACED. If anything wrote the activity
		// while the fight ran -- a script today, a real verb after M4.4 --
		// restoring the pre-fight value would silently clobber it.
		if v.meters.Activity() == d2world.ActivityLabour {
			v.meters.SetActivity(v.activityBeforeFight)
		}

		v.wasFighting = false

		v.writePaceLine()
	}
}

// writePaceLine writes one PACE line per fight, at the close. The night's
// total -- the number R2 §4 governs -- is the sum of wall_s over one day=, and
// a break-away that reopens is several rows in that sum.
//
// A fight the player never got a turn in writes NOTHING: the pace row's
// encounter is empty until the first turn opened, because a fight resolved by
// the policy in 0.4 seconds measured nothing about a person. A policy-
// controlled fight that DID open a turn writes the line with control=policy
// and decide_s=0.0, which is the control that says the timer measures people
// rather than fights.
func (v *Game) writePaceLine() {
	if v.combat == nil {
		return
	}

	row := v.combat.LastPace()
	if row.Encounter == "" {
		return
	}

	perRound, decidePerRound := 0.0, 0.0
	if row.Rounds > 0 {
		perRound = row.WallSeconds / float64(row.Rounds)
		decidePerRound = row.DecideSeconds / float64(row.Rounds)
	}

	// NO TORCH IS NOT A BURNT-OUT TORCH. Measured: defaulting torch_out to
	// true reported light exhaustion on a fight fought with no torch at all --
	// criterion 3's telemetry saying the opposite of what happened.
	burnAfter, torchesUsed, torchOut := 0.0, 0, false

	if v.light != nil {
		if carried := v.light.Carried(); carried != nil {
			burnAfter, torchesUsed, torchOut = carried.Burn, 1, carried.Burn <= 0
		}
	}

	closeClock := ""
	if v.worldClock != nil {
		closeClock = v.worldClock.TimeOfDay()
	}

	v.Infof("PACE  encounter=%s day=%d open=%s close=%s rounds=%d wall_s=%.1f decide_s=%.1f "+
		"s_per_round=%.2f decide_per_round=%.2f hp=%d->%d torch_lit_rounds=%d torch_out=%v "+
		"burn_after=%.0f torches_used=%d end=%s enemies=%d kinds=%s initiator=%s surprised=%v control=%s",
		row.Encounter, v.paceOpenDay, v.paceOpenClock, closeClock, row.Rounds,
		row.WallSeconds, row.DecideSeconds, perRound, decidePerRound,
		row.HealthOpen, row.HealthClose, v.torchLitRounds, torchOut,
		burnAfter, torchesUsed, row.EndReason, row.Enemies,
		strings.Join(row.Kinds, ","), row.Initiator, row.Surprised, row.Control)
}

// commandWish writes the "what he reached for that isn't there" column of the
// instrument (state.md:168, ruling (b)). One console line, appended to the same
// log the ROUND and PACE lines go to, so that a friend's sentence lands beside
// the numbers it belongs to instead of in a separate feedback form nobody fills
// in:
//
//	WISH day=1 clock=21:15 encounter=e:7 round=3 text="I wanted to hide"
//
// The fight is read LIVE, at the moment the line is written. Outside a fight
// the encounter is "-" and the round is 0; reading either from a source that
// survives the fight's end would report a fight the player is not in, which is
// the break this note's negative control makes on purpose.
func (v *Game) commandWish(args []string) error {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		return errors.New("wish: say what you reached for")
	}

	day, clock := 0, "--:--"
	if v.worldClock != nil {
		day, clock = v.worldClock.DayIndex(), v.worldClock.TimeOfDay()
	}

	encounter, round := "-", 0
	if v.combat != nil && v.combat.Fighting() {
		encounter, round = v.combat.Encounter(), v.combat.Round()
	}

	v.Infof("WISH day=%d clock=%s encounter=%s round=%d text=%q", day, clock, encounter, round, text)

	return nil
}

// startChasesForTheAware is the line that makes M4.3b a milestone rather than
// a diorama: a thing that has noticed you comes for you.
//
// IT WAS MISSING FROM M4.3b AS SHIPPED, and an audit found it. Notice worked
// out awareness, Pursuit could route a chase, and nothing joined them outside
// the harness -- so in any real build a wolf spotted the player and stood
// there, while the playtest passed because the script called strigoi_pursue
// itself. Same shape as Light.Remove reopening M4.1, one milestone larger.
//
// The type assertion is the seam, not a shortcut: d2world knows a Watcher and
// a Hunter, and the game screen's chaser adapter deliberately satisfies both,
// because the thing that notices you is the thing that then comes for you.
// Anything that can watch but not walk is skipped rather than forced.
func (v *Game) startChasesForTheAware() {
	if v.notice == nil || v.pursuit == nil {
		return
	}

	for _, pair := range v.notice.AwarePairs() {
		hunter, ok := pair.Watcher.(d2world.Hunter)
		if !ok {
			continue
		}

		// Already chasing: leave it alone. Chase() replaces, so restarting
		// here every tick would reset the re-path clock and reproduce M4.3a's
		// 218-solves bug from the other direction.
		if v.pursuit.Chasing(hunter.HunterID()) {
			continue
		}

		v.pursuit.Chase(hunter, pair.Target)
	}
}

// mapRouter adapts the map engine's pathfinder to d2world.Router, so pursuit
// can ask for a route without d2world knowing what a MapEngine is (M4.3a).
// The conversion between world tiles and the subtile space the search works
// in lives here, in one place.
type mapRouter struct{ engine *d2mapengine.MapEngine }

// routeNeighbours are the eight tiles around a goal, in a fixed order. The
// order is fixed because these routes move entities, and entity positions are
// inside the state digest.
var routeNeighbours = [8][2]float64{
	{0, -1}, {1, -1}, {1, 0}, {1, 1},
	{0, 1}, {-1, 1}, {-1, 0}, {-1, -1},
}

// Route walks a hunter to a tile BESIDE its quarry, and only onto the quarry's
// own tile when nothing beside it can be reached.
//
// THE ORDER OF THOSE TWO ATTEMPTS IS THE WHOLE CHANGE, and it was measured
// before it was made. M4.3a wrote the exact route first and the neighbours as
// a fallback, on the reasonable assumption that a thing's own footprint is not
// a place another thing can path to. IT IS: entities do not block the A*, so
// routeExact succeeded every time and the fallback never fired. Step 4's
// section 0 measured the consequence -- a pursuer walks to distance 0.000 and
// stops ON the player -- and step 4's resolver comment recorded what it costs:
// every participant in a settled fight floors to ONE tile and samples ONE
// light level, so R2 section 3's dark-into-light advantage cannot fire in a v0
// build AT ANY PLACEMENT, and neither can flanking or facing when they arrive.
// Three signed rules, all built, none able to fire, for want of one tile.
//
// The neighbours are tried NEAREST FIRST rather than in the table's order.
// Fixed order alone would send a hunter approaching from the south around to
// the north tile, which is both absurd to watch and a longer walk; nearest
// first is still fully deterministic -- the distance is computed from the
// hunter's own position and ties break on the table's index -- and it is
// cheaper, because the first candidate is usually the one that routes.
//
// M4.5 ask 8, its own burst as signed. ArriveWithin moves with it: a diagonal
// stop sits at 1.414, and against the old 1.0 it would never report arrived
// and would re-solve on the re-path cadence forever.
func (r mapRouter) Route(fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	if r.engine == nil {
		return nil, false
	}

	for _, n := range unblockedNeighbours(fromX, fromY, toX, toY, r.blockedTile) {
		beside, ok := r.routeExact(fromX, fromY, toX+n[0], toY+n[1])
		if ok {
			return beside, true
		}
	}

	// Nothing beside the quarry can be reached -- it is in a doorway, or
	// walled in. Standing on its own tile is then the best available answer
	// and is what every build before this one did everywhere.
	path, reachable := r.routeExact(fromX, fromY, toX, toY)
	if reachable {
		return path, true
	}

	// Not even that. Return the direct partial route, which still walks the
	// hunter as far toward the quarry as it can get.
	return path, false
}

// neighboursNearest orders the eight offsets around a goal by how close each
// candidate TILE would be to where the hunter already stands, ties broken by
// the table's fixed index.
//
// The offsets are relative to the GOAL, so the distance has to be measured
// against `to` plus the offset -- comparing the bare offset against the
// hunter's position measures nothing, which is what the first draft of this
// did.
//
// It is a pure function of two positions over a fixed table, so it is
// deterministic -- which is what keeps entity positions, and therefore the
// state digest, reproducible across two launches of one build. It is split out
// with no receiver so it can be tested without a map engine.
func neighboursNearest(fromX, fromY, toX, toY float64) [8][2]float64 {
	idx := [8]int{0, 1, 2, 3, 4, 5, 6, 7}

	dist := func(k int) float64 {
		dx := toX + routeNeighbours[k][0] - fromX
		dy := toY + routeNeighbours[k][1] - fromY

		return dx*dx + dy*dy
	}

	// Insertion sort: eight elements, and it keeps the tie-break on index
	// explicit rather than depending on a sort's stability guarantees.
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && (dist(idx[j]) < dist(idx[j-1]) ||
			(dist(idx[j]) == dist(idx[j-1]) && idx[j] < idx[j-1])); j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}

	var out [8][2]float64
	for i, k := range idx {
		out[i] = routeNeighbours[k]
	}

	return out
}

// unblockedNeighbours is the eight tiles around a goal, nearest to the hunter
// first (neighboursNearest), with the ones that are THEMSELVES blocked dropped
// before Route pays for a search toward them.
//
// A blocked goal never enters the A*'s open set, so routeExact on one expands to
// the whole budget (maxExpandedNodes) and returns nothing usable -- and Route
// tries up to nine tiles per solve, every re-path, for every chasing hunter. A
// quarry with a wall or fence beside it (a friend inside a yard, wolves outside)
// is the reachable worst case (audit B3, 12 Sep 2026). Skipping a blocked
// candidate changes no output -- routeExact would have returned ok=false for it
// anyway -- so the route Pursuit gets is identical; only the wasted searches go.
//
// It takes a plain predicate so it is testable without a map engine, exactly as
// neighboursNearest is; the nearest-first order is preserved, only blocked
// candidates drop out.
func unblockedNeighbours(fromX, fromY, toX, toY float64, blocked func(tileX, tileY float64) bool) [][2]float64 {
	nearest := neighboursNearest(fromX, fromY, toX, toY)

	out := make([][2]float64, 0, len(nearest))

	for _, n := range nearest {
		if blocked(toX+n[0], toY+n[1]) {
			continue
		}

		out = append(out, n)
	}

	return out
}

// blockedTile reports whether a world tile is blocked, by asking the map engine
// about the subtile routeExact would target: PathFind floors
// NewPositionTile(tileX, tileY), i.e. the subtile at floor(tileX * 5).
func (r mapRouter) blockedTile(tileX, tileY float64) bool {
	if r.engine == nil {
		return false
	}

	return r.engine.BlockedAt(
		int(math.Floor(tileX*subTilesPerTile)),
		int(math.Floor(tileY*subTilesPerTile)),
	)
}

// routeExact asks for a route to precisely the given point.
func (r mapRouter) routeExact(fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	path := r.engine.PathFind(
		d2vector.NewPositionTile(fromX, fromY),
		d2vector.NewPositionTile(toX, toY),
	)

	out := make([][2]float64, 0, len(path))

	for i := range path {
		world := path[i].World()
		out = append(out, [2]float64{world.X(), world.Y()})
	}

	// Reachable means the route ends on the goal tile rather than at the best
	// partial approach the bounded search managed.
	reachable := false
	if n := len(out); n > 0 {
		reachable = math.Floor(out[n-1][0]) == math.Floor(toX) &&
			math.Floor(out[n-1][1]) == math.Floor(toY)
	}

	return out, reachable
}

// pathWalker is the part of a map entity that pursuit drives. Player and NPC
// both satisfy it through the mapEntity they embed; naming it here rather
// than widening d2interface.MapEntity keeps the surface to what is actually
// needed.
type pathWalker interface {
	ID() string
	GetPositionF() (float64, float64)
	IsMoving() bool
	SetPath(path []d2vector.Position, done func())
}

// chaser adapts a map entity to d2world.Hunter.
type chaser struct{ entity pathWalker }

func (c chaser) HunterID() string { return c.entity.ID() }

// HunterAt returns WORLD TILES, which is what GetPositionF already gives --
// its own doc says "the entity's current tile position where 0.2 is one sub
// tile", and it is Position.World(), already divided by five.
//
// M4.3a DIVIDED BY FIVE AGAIN HERE, and M4.3b's first playtest run is what
// caught it: a watcher placed six tiles from the player reported a distance of
// 1.19. See the note on prey.QuarryAt for what that cost.
func (c chaser) HunterAt() (x, y float64) { return c.entity.GetPositionF() }

func (c chaser) Following() bool { return c.entity.IsMoving() }

func (c chaser) Follow(waypoints [][2]float64) {
	path := make([]d2vector.Position, 0, len(waypoints))
	for _, w := range waypoints {
		path = append(path, d2vector.NewPositionTile(w[0], w[1]))
	}

	c.entity.SetPath(path, nil)
}

// WatcherID and WatcherAt make a chaser a d2world.Watcher as well as a
// d2world.Hunter. The two are the same adapter on purpose: the thing that
// notices you is the thing that then comes for you, and Notice targets a
// Quarry for the mirror-image reason.
func (c chaser) WatcherID() string { return c.entity.ID() }

func (c chaser) WatcherAt() (x, y float64) { return c.HunterAt() }

// mapSight adapts the map engine's line-of-sight test to d2world.Sight. It is
// the raycast PathFind used to be before M4.3a replaced it with a real
// search, kept because whether a thing can SEE you is not whether it can WALK
// to you.
type mapSight struct{ engine *d2mapengine.MapEngine }

func (m mapSight) Clear(fromX, fromY, toX, toY float64) bool {
	if m.engine == nil {
		return false
	}

	return m.engine.LineOfSight(
		d2vector.NewPositionTile(fromX, fromY),
		d2vector.NewPositionTile(toX, toY),
	)
}

// gameSpawner adapts the entity factory to d2world.Spawner: the tables decide
// what and how many, this puts them somewhere walkable and hands back things
// that can say where they are.
//
// PLACEMENT IS DELIBERATELY NOT RANDOM. The pack size and which row fires are
// already drawn from the spawn system's own seeded RNG; adding a second RNG
// here would be a second thing to keep deterministic for no design gain. So
// members land on evenly spaced bearings around the ring, with the starting
// bearing walked on by each arrival so successive packs do not all come from
// due east. A real "wolves come from the woods, dogs come from the road"
// model needs terrain semantics this build does not have, and it is content
// work rather than this milestone's.
type gameSpawner struct {
	engine  *d2mapengine.MapEngine
	asset   *d2asset.AssetManager
	arrival int

	// adopt and release hand a spawned monster its body and take it back
	// (M4.5 step 3). They are callbacks rather than a *Game so that the
	// spawner still knows only what it needs: it has the monstats record in
	// hand at the moment it creates the entity, which is the only moment the
	// hit-point band and the entity id are both to hand.
	adopt   func(id string, maxHealth int)
	release func(id string)
}

const (
	spawnBearingStep = 2.39996 // ~137.5 degrees, so successive arrivals spread
	spawnSearchRings = 6       // how far out to look for walkable ground
)

func (g *gameSpawner) Spawn(code string, count int, aroundX, aroundY,
	minTiles, maxTiles float64) []d2world.Watcher {
	if g.engine == nil || g.asset == nil || count <= 0 {
		return nil
	}

	monstat := g.asset.Records.Monster.Stats[code]
	if monstat == nil {
		// An unknown stand-in code. The spawn system counts this as a failure
		// and reports it; a bad [DIAL] should show up in the provider rather
		// than crash in the field.
		return nil
	}

	g.arrival++
	bearing := float64(g.arrival) * spawnBearingStep

	out := make([]d2world.Watcher, 0, count)

	for i := 0; i < count; i++ {
		angle := bearing + 2*math.Pi*float64(i)/float64(count)

		reach := minTiles
		if count > 1 {
			reach += (maxTiles - minTiles) * float64(i) / float64(count-1)
		}

		x, y, ok := g.walkableNear(
			aroundX+reach*math.Cos(angle),
			aroundY+reach*math.Sin(angle),
		)
		if !ok {
			continue
		}

		npc, err := g.engine.NewNPC(int(x*subTilesPerTile), int(y*subTilesPerTile), monstat, 0)
		if err != nil {
			continue
		}

		g.engine.AddEntity(npc)

		// The night hands the monster a body. This is the game-side caller
		// that keeps the whole thing off the harness-only ladder: Spawns
		// drives it from advanceWorld, so a shipped build adopts bodies with
		// no script involved.
		if g.adopt != nil {
			g.adopt(npc.ID(), monstat.MaxHPNormal)
		}

		out = append(out, chaser{entity: npc})
	}

	return out
}

// Despawn takes a group's members back off the map.
//
// The type assertion is the seam, not a shortcut: d2world hands back Watchers
// because what a spawned thing is FOR is noticing you, and only this package
// knows that a Watcher it produced is a chaser wrapping a real map entity.
// Anything else -- a hand-made watcher from a script, say -- is skipped rather
// than forced, the same way startChasesForTheAware skips a watcher that cannot
// walk.
func (g *gameSpawner) Despawn(members []d2world.Watcher) {
	if g.engine == nil {
		return
	}

	for _, m := range members {
		c, ok := m.(chaser)
		if !ok {
			continue
		}

		entity, ok := c.entity.(d2interface.MapEntity)
		if !ok {
			continue
		}

		g.engine.RemoveEntity(entity)

		// Off the map, so the body goes too. Paired with the removal rather
		// than with the loop, so a member that was never really on the map
		// keeps whatever body BodyOf may since have adopted for it.
		if g.release != nil {
			g.release(c.entity.ID())
		}
	}
}

// walkableNear finds ground near a wanted point, spiralling outward a few
// tiles. A ring position can easily land in a wall or off the map; giving up
// silently would make a spawn table look broken when the geometry was simply
// unlucky, so this tries the neighbourhood before returning false.
func (g *gameSpawner) walkableNear(wantX, wantY float64) (x, y float64, ok bool) {
	for ring := 0; ring <= spawnSearchRings; ring++ {
		for dy := -ring; dy <= ring; dy++ {
			for dx := -ring; dx <= ring; dx++ {
				// Only the shell of each ring; the inside was tried already.
				if ring > 0 && absInt(dx) != ring && absInt(dy) != ring {
					continue
				}

				tx := math.Floor(wantX) + float64(dx)
				ty := math.Floor(wantY) + float64(dy)

				if g.walkable(tx, ty) {
					return tx + 0.5, ty + 0.5, true
				}
			}
		}
	}

	return 0, 0, false
}

func (g *gameSpawner) walkable(tileX, tileY float64) bool {
	if !g.engine.TileExists(int(tileX), int(tileY)) {
		return false
	}

	flags := g.engine.SubTileAt(int(tileX*subTilesPerTile)+2, int(tileY*subTilesPerTile)+2)

	return flags != nil && !flags.BlockWalk
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

// prey adapts a map entity to d2world.Quarry.
type prey struct{ entity pathWalker }

func (p prey) QuarryID() string { return p.entity.ID() }

// QuarryAt returns WORLD TILES. Same correction as chaser.HunterAt, and this
// is the place to record what the bug actually did, because the M4.3a closeout
// reported a number that was wrong in both size and sign.
//
// mapRouter.Route takes and returns world tiles -- its own comment says the
// conversion "lives here, in one place" -- so feeding it fifth-scale
// coordinates did not merely shrink the numbers, it routed between the WRONG
// POINTS. A hunter at tile 35.2 asked for a route from tile 7.04 to a player
// reported at tile 6.2, and walked toward the map's corner. The distance the
// system reported shrank while the real gap grew.
//
// So M4.3a's closeout claim -- "a fallen spawned six tiles away chased the
// player through an eight-tile move and closed to 2.80 tiles" -- was false.
// The dials were wrong by the same factor: ArriveWithin 1.0 meant five tiles,
// RepathTiles 1.5 meant seven and a half. They are left at their signed values
// because those values were chosen as TILES; they only now mean it.
func (p prey) QuarryAt() (x, y float64) { return p.entity.GetPositionF() }

// subTilesPerTile mirrors d2vector's own constant, which is unexported.
const subTilesPerTile = 5.0

// Pursue puts one entity on another's trail. It is the seam the harness (and,
// at M4.3b, the awareness model) uses to start a chase: d2world cannot look
// entities up by handle, so whoever owns the handles starts the chase and
// pursuit keeps it honest afterwards.
func (v *Game) Pursue(hunter, quarry interface{}) bool {
	h, hok := hunter.(pathWalker)
	q, qok := quarry.(pathWalker)

	if v.pursuit == nil || !hok || !qok {
		return false
	}

	v.pursuit.Chase(chaser{entity: h}, prey{entity: q})

	return true
}

// Watch makes one entity aware of another, and Unwatch stops it. Same
// translation problem Game.Pursue solves and the same answer: d2world speaks
// entity ids and knows nothing about handles, so whoever owns the handles
// starts the watch.
//
// In the running game the spawn tables do this themselves for every member of
// every group they create. This exists so a script can put a watcher EXACTLY
// where it needs one -- in the open for the positive control, behind a wall
// for the negative -- which is what makes "it noticed you" evidence rather
// than a coincidence. That distinction is the whole reason M4.3b ask 6 asked
// for the notice block in the first place.
func (v *Game) Watch(watcher, target interface{}) bool {
	w, wok := watcher.(pathWalker)
	q, qok := target.(pathWalker)

	if v.notice == nil || !wok || !qok {
		return false
	}

	v.notice.Watch(chaser{entity: w}, prey{entity: q})

	return true
}

// Unwatch stops one watcher. It takes the entity's id rather than a handle,
// because that is what d2world stored.
func (v *Game) Unwatch(watcherID string) bool {
	if v.notice == nil {
		return false
	}

	return v.notice.Unwatch(watcherID)
}

// playerBody adapts the local player's hero stats to d2world.Body, so the
// meters can spend health without d2world knowing what a hero is (M4.2).
// It is the first thing in this codebase to write Stats.Health.
type playerBody struct{ player *d2mapentity.Player }

func (b playerBody) CurrentHealth() int { return b.player.Stats.Health }
func (b playerBody) MaxHealth() int     { return b.player.Stats.MaxHealth }
func (b playerBody) SetHealth(h int)    { b.player.Stats.Health = h }

// squadDeployer places and removes the map entity a squad MODEL is (M4.4c-1,
// ruled ask 10(b): a model is a real thing on the map). It is the Squads
// owner's Deployer, the same shape as gameSpawner: d2world names the act, the
// screen builds the entity, and the body is adopted HERE so a deployed model
// counts in bodies_known -- which is why the N-path asserts on that and never
// on world_draws (NewNPC draws the world RNG by construction, unreadable and
// hashed). It places STANDALONE entities via AddEntity, never into a spawn
// group (clearAtDaybreak would despawn the player's own men at first light) and
// never into gameClient.Players (the escape-menu pause fence). s:1 deploys
// nothing: its one model is the player, already on the map.
type squadDeployer struct{ game *Game }

// squadModelCode is the stand-in monstats.txt Id a deployed squad model uses.
// The sprite is irrelevant in c-1: build #1's only squad is the player (no
// deployment), and the N-path's second squad is harness-placed for the
// collection assertion and selected by squad membership, not by its sprite.
const squadModelCode = "fallen1"

func (d squadDeployer) Deploy(x, y float64) (string, int, bool) {
	v := d.game
	if v == nil || v.gameClient == nil || v.gameClient.MapEngine == nil || v.asset == nil {
		return "", 0, false
	}

	// A missing position deploys near the player, two tiles east of him.
	if x == 0 && y == 0 && v.localPlayer != nil {
		world := v.localPlayer.Position.World()
		x, y = world.X()+2, world.Y()
	}

	monstat := v.asset.Records.Monster.Stats[squadModelCode]
	if monstat == nil {
		return "", 0, false
	}

	npc, err := v.gameClient.MapEngine.NewNPC(int(x*subTilesPerTile), int(y*subTilesPerTile), monstat, 0)
	if err != nil {
		return "", 0, false
	}

	v.gameClient.MapEngine.AddEntity(npc)
	v.adoptNPCBody(npc.ID(), monstat.MaxHPNormal)

	return npc.ID(), monstat.MaxHPNormal, true
}

func (d squadDeployer) Recall(id string) bool {
	v := d.game
	if v == nil || v.gameClient == nil || v.gameClient.MapEngine == nil {
		return false
	}

	entity, ok := v.gameClient.MapEngine.Entities()[id]
	if !ok {
		return false
	}

	v.gameClient.MapEngine.RemoveEntity(entity)
	v.releaseNPCBody(id)

	return true
}

// deadSpawnRows are the SPAWN TABLE rows that count as "the dead": R2 §1
// fences the UNKNOWN (the risen dead) -- no bar until the hearth unlocks the
// bestiary (ruled ask 4/7). N1 §5's four rows are beasts and men (dogs, wolves,
// boar, opportunists), so this gate is EMPTY and never fires in friends build
// #1; it is written NOW, keyed on the row, so M4.7 inherits a gate rather than
// retrofitting one into shipped UI, and M4.7 fills in its risen-corpse rows.
var deadSpawnRows = map[string]bool{}

// deadMonsterGroups is the FALLBACK gate, for an NPC the spawn tables did not
// place (a map-native monster, or a harness placement): for a real D2 monster
// the monstats MonType group is its kind.
var deadMonsterGroups = map[string]bool{
	"skeleton": true, "zombie": true, "undead": true, "wraith": true, "ghost": true,
}

// ShowsBar reports whether an entity gets an overhead bar: beasts and men yes,
// the dead never (ruled ask 4/7).
//
// THE SPAWN ROW IS THE AUTHORITY, not the monstats code, because N1 §5's codes
// are STAND-IN SPRITES: the wolves wear "zombie1" and the boar wears
// "skeleton1" (d2core/d2world/spawns.go:238, :251). Keying this gate on
// MonStatRecord.MonsterGroup -- which it did until the c-1 review, 15 Sep 2026
// -- asks what the ART is, not what the THING is, and would have taken the bar
// off night one's main threat, inverting the ruling it was written to serve.
// The monstats group survives below as the fallback for an NPC the tables did
// not place. The player is not judged here: the player's squad bar is decided
// by the Squads owner.
func (v *Game) ShowsBar(id string) bool {
	if v.gameClient == nil || v.gameClient.MapEngine == nil {
		return false
	}

	entity, ok := v.gameClient.MapEngine.Entities()[id]
	if !ok {
		return false
	}

	npc, ok := entity.(*d2mapentity.NPC)
	if !ok {
		return false
	}

	if v.spawns != nil {
		if p, ok := v.spawns.ProfileOf(id); ok {
			return !deadSpawnRows[strings.ToLower(p.Row)]
		}
	}

	monstat := npc.MonStat()
	if monstat == nil {
		return false
	}

	return !deadMonsterGroups[strings.ToLower(monstat.MonsterGroup)]
}

// OverheadBars assembles the bars the HUD draws (M4.4c-1): one per player-squad
// model (its squad's pooled health and stage cues), and one per beast or man
// with a body, never the dead. It is READ-ONLY -- it peeks v.bodies directly
// and never calls BodyOf, which adopts on read and would break the
// eager-adoption test (combat_body_test.go act 5). The enemy bars are sorted by
// id so the "ui" provider's list is deterministic (determinism_test.go).
func (v *Game) OverheadBars() []d2player.OverheadBar {
	out := []d2player.OverheadBar{}

	if v.squads != nil {
		for _, sb := range v.squads.Bars() {
			for _, ent := range sb.Entities {
				out = append(out, d2player.OverheadBar{
					Entity:   ent,
					Cur:      sb.Cur,
					Max:      sb.Max,
					Cues:     sb.Cues,
					Selected: sb.Selected,
				})
			}
		}
	}

	ids := make([]string, 0, len(v.bodies))
	for id := range v.bodies {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	for _, id := range ids {
		body := v.bodies[id]
		if body == nil {
			continue
		}

		if v.squads != nil && v.squads.SquadOf(id) != "" {
			continue // a player squad model, already barred above
		}

		// NEVER THE DEAD (clause 7) in its literal sense: a body at or below
		// 0 health is a CORPSE, and nothing in this build removes a killed
		// NPC from the map -- ActDie only plays the animation, MapEngine
		// .Advance removes nothing, and the only despawn is a spawn group at
		// rout or daybreak. Without this guard every kill leaves an empty bar
		// hanging over the ground until dawn, and each one puts another rect
		// in the band night_render_test.go samples.
		if body.CurrentHealth() <= 0 {
			continue
		}

		if !v.ShowsBar(id) {
			continue
		}

		out = append(out, d2player.OverheadBar{
			Entity: id,
			Cur:    body.CurrentHealth(),
			Max:    body.MaxHealth(),
			Enemy:  true,
		})
	}

	return out
}

// shouldSaveOnUnload reports whether OnUnload should persist the hero. It saves
// unless the local player is DEFINITIVELY dead (Stats.IsDead): a saved 0-HP hero
// loads as an un-killable, un-feedable corpse (audit A2, 12 Sep 2026 ruling).
// A nil player -- before the controls bind -- cannot be judged dead, so the
// original always-save behaviour is kept (IsDead is nil-safe for nil stats).
func shouldSaveOnUnload(p *d2mapentity.Player) bool {
	return p == nil || !p.Stats.IsDead()
}

// Only Pursuit and Notice get an accessor, and the asymmetry is deliberate:
// d2app does not import d2core/d2world, so it cannot name those two types, and
// Notice is the one world system with no provider of its own. Every other
// system is reached through the d2harness registry instead.
//
// WorldClock, Light, Meters and Spawns used to sit here too. The reachability
// register found that none of the four had ever had a caller in any commit --
// they were a second door onto systems that already had one -- and they were
// deleted on 28 Aug 2026 by Josh's ruling. See docs/reachability.md.

// Pursuit returns the screen's pursuit system, or nil before it exists.
func (v *Game) Pursuit() *d2world.Pursuit { return v.pursuit }

// Notice returns the screen's awareness model, or nil before it exists.
func (v *Game) Notice() *d2world.Notice { return v.notice }

func (v *Game) bindGameControls() error {
	for _, player := range v.gameClient.Players {
		if player.ID() != v.gameClient.PlayerID {
			continue
		}

		v.localPlayer = player

		var err error
		v.gameControls, err = d2player.NewGameControls(v.asset, v.renderer, player, v.gameClient.MapEngine,
			v.escapeMenu, v.mapRenderer, v, v.terminal, v.uiManager, v.keyMap, v.audioProvider, v.logLevel,
			v.gameClient.IsSinglePlayer(), v.gameClient.Players, v.worldClock, v.squads, v.combat, v.light, v)

		if err != nil {
			return err
		}

		v.gameControls.Load()

		if err := v.inputManager.BindHandler(v.gameControls); err != nil {
			v.Error(bindControlsErrStr + player.ID())
		}

		// The controls are the harness's "ui" system while this screen lives
		// (P3 spec §3.5); OnUnload unregisters them.
		d2harness.Register(v.gameControls)

		break
	}

	return nil
}

// OnPlayerMove sends the player move action to the server
func (v *Game) OnPlayerMove(targetX, targetY float64) {
	// THE MOVE IS A PIP, and spending it is what can close a both-spent turn.
	// This is the third entry point into finishRound, beside Advance and
	// Commit: every sentence in the design says a Move click ends a turn whose
	// Action is already spent, and a turn-over check that lived only inside
	// Commit could never deliver it, because a Move is not a commit.
	//
	// SpendMove does nothing unless a turn is open, so an ordinary walk
	// outside a fight is untouched.
	if v.combat != nil {
		v.combat.SpendMove()
	}

	worldPosition := v.localPlayer.Position.World()

	playerID, worldX, worldY := v.gameClient.PlayerID, worldPosition.X(), worldPosition.Y()

	createMovePlayerPacket, err := d2netpacket.CreateMovePlayerPacket(playerID, worldX, worldY, targetX, targetY)
	if err != nil {
		v.Errorf("MovePlayerPacket: %v", err)
	}

	err = v.gameClient.SendPacketToServer(createMovePlayerPacket)

	if err != nil {
		v.Errorf(moveErrStr, v.gameClient.PlayerID, targetX, targetY)
	}
}

// OnPlayerSave instructs the server to save our player data
func (v *Game) OnPlayerSave() error {
	playerState := v.gameClient.Players[v.gameClient.PlayerID]

	sp, err := d2netpacket.CreateSavePlayerPacket(playerState, d2enum.DifficultyNormal)
	if err != nil {
		return fmt.Errorf("SavePlayerPacket: %v", err)
	}

	err = v.gameClient.SendPacketToServer(sp)

	if err != nil {
		return err
	}

	return nil
}

// OnPlayerCast sends the casting skill action to the server
func (v *Game) OnPlayerCast(skillID int, targetX, targetY float64) {
	cp, err := d2netpacket.CreateCastPacket(v.gameClient.PlayerID, skillID, targetX, targetY)
	if err != nil {
		v.Errorf("CastPacket: %v", err)
	}

	err = v.gameClient.SendPacketToServer(cp)
	if err != nil {
		v.Errorf(castErrStr, v.gameClient.PlayerID, skillID, targetX, targetY)
	}
}

func (v *Game) debugSpawnItemAtPlayer(codes ...string) {
	if v.localPlayer == nil {
		return
	}

	pos := v.localPlayer.GetPosition()
	tile := pos.Tile()
	x, y := int(tile.X()), int(tile.Y())

	v.debugSpawnItemAtLocation(x, y, codes...)
}

func (v *Game) debugSpawnItemAtLocation(x, y int, codes ...string) {
	packet, err := d2netpacket.CreateSpawnItemPacket(x, y, codes...)
	if err != nil {
		v.Errorf("SpawnItemPacket: %v", err)
	}

	err = v.gameClient.SendPacketToServer(packet)
	if err != nil {
		v.Errorf(spawnItemErrStr, x, y, codes)
	}
}

func (v *Game) commandSpawnItem(args []string) error {
	v.debugSpawnItemAtPlayer(args...)

	return nil
}

func (v *Game) commandSpawnItemAt(args []string) error {
	x, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid argument")
	}

	y, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid argument")
	}

	v.debugSpawnItemAtLocation(x, y, args[2:]...)

	return nil
}

func (v *Game) commandSpawnMon(args []string) error {
	name := args[0]
	x := int(v.localPlayer.Position.X())
	y := int(v.localPlayer.Position.Y())

	monstat := v.asset.Records.Monster.Stats[name]
	if monstat == nil {
		v.terminal.Errorf("no monstat entry for \"%s\"", name)
		return nil
	}

	monster, npcErr := v.gameClient.MapEngine.NewNPC(x, y, monstat, 0)
	if npcErr != nil {
		v.terminal.Errorf("error generating monster \"%s\": %v", name, npcErr)
		return nil
	}

	v.gameClient.MapEngine.AddEntity(monster)

	return nil
}
