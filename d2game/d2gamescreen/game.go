package d2gamescreen

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"strconv"

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

	// The survival meters (M4.2, S1 §5) read the same clock. The health they
	// spend belongs to a player entity that does not exist yet, so the body
	// is attached on the first frame that has one — see advanceWorld.
	game.meters = d2world.NewMeters(game.worldClock, d2world.DefaultMeterDials())

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
		&gameSpawner{engine: gameClient.MapEngine, asset: asset},
		game.light,
		gameClient.Seed,
		d2world.DefaultSpawnDials(),
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
	// providers. metersBodied records that the local player's health has
	// been handed to the meters, which cannot happen at construction because
	// the player entity does not exist yet.
	worldClock   *d2world.Clock
	light        *d2world.Light
	meters       *d2world.Meters
	metersBodied bool
	pursuit      *d2world.Pursuit
	notice       *d2world.Notice
	spawns       *d2world.Spawns

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

	if v.meters != nil {
		v.meters.Close()
	}

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

	if err := v.terminal.Unbind("spawnitemat", "spawnitem", "spawnmon"); err != nil {
		return err
	}

	if err := v.OnPlayerSave(); err != nil {
		return err
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
	v.advanceWorld(elapsed)

	v.soundEngine.Advance(elapsed)

	if (v.escapeMenu != nil && !v.escapeMenu.IsOpen()) || len(v.gameClient.Players) != 1 {
		v.gameClient.MapEngine.Advance(elapsed)
	}

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

	if v.meters != nil {
		if !v.metersBodied && v.localPlayer != nil && v.localPlayer.Stats != nil {
			v.meters.SetBody(playerBody{player: v.localPlayer})
			v.metersBodied = true
		}

		v.meters.Advance(worldMinutes)
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
}

// mapRouter adapts the map engine's pathfinder to d2world.Router, so pursuit
// can ask for a route without d2world knowing what a MapEngine is (M4.3a).
// The conversion between world tiles and the subtile space the search works
// in lives here, in one place.
type mapRouter struct{ engine *d2mapengine.MapEngine }

// routeNeighbours are the eight tiles around a goal, in a fixed order. When
// the quarry's own tile cannot be routed to, standing next to it is the right
// answer -- and the order is fixed because these routes move entities, and
// entity positions are inside the state digest.
var routeNeighbours = [8][2]float64{
	{0, -1}, {1, -1}, {1, 0}, {1, 1},
	{0, 1}, {-1, 1}, {-1, 0}, {-1, -1},
}

func (r mapRouter) Route(fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	if r.engine == nil {
		return nil, false
	}

	path, reachable := r.routeExact(fromX, fromY, toX, toY)
	if reachable {
		return path, true
	}

	// The quarry's own subtile is often not walkable -- it is standing on it,
	// and a thing's own footprint is not a place another thing can path to.
	// Without this a pursuer stops several tiles short and reports failure,
	// which is what the first playtest run measured: distance 2.80 and
	// reachable=false while the hunter was plainly right there. Standing
	// beside the quarry is what "the pursuer arrives" means at M4.3a, since
	// there is no combat for it to start.
	for _, n := range routeNeighbours {
		beside, ok := r.routeExact(fromX, fromY, toX+n[0], toY+n[1])
		if ok {
			return beside, true
		}
	}

	// Nothing adjacent is reachable either. Return the direct partial route,
	// which still walks the hunter as far toward the quarry as it can get.
	return path, false
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
		out = append(out, chaser{entity: npc})
	}

	return out
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

// WorldClock returns the screen's world clock, or nil before it exists.
func (v *Game) WorldClock() *d2world.Clock { return v.worldClock }

// Light returns the screen's light model, or nil before it exists.
func (v *Game) Light() *d2world.Light { return v.light }

// Meters returns the screen's survival meters, or nil before they exist.
func (v *Game) Meters() *d2world.Meters { return v.meters }

// Pursuit returns the screen's pursuit system, or nil before it exists.
func (v *Game) Pursuit() *d2world.Pursuit { return v.pursuit }

// Spawns returns the screen's spawn tables, or nil before they exist.
func (v *Game) Spawns() *d2world.Spawns { return v.spawns }

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
			v.gameClient.IsSinglePlayer(), v.gameClient.Players)

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
