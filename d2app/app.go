// Package d2app contains the OpenDiablo2 application shell
package d2app

import (
	"bytes"
	"container/ring"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/gif"
	"image/png"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"

	"github.com/pkg/profile"
	"golang.org/x/image/colornames"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	ebiten2 "github.com/OpenDiablo2/OpenDiablo2/d2core/d2audio/ebiten"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2config"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2gui"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2input"
	ebiteninput "github.com/OpenDiablo2/OpenDiablo2/d2core/d2input/ebiten"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2render/ebiten"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2screen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2term"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2server"
	"github.com/OpenDiablo2/OpenDiablo2/d2script"
)

// these are used for debug print info
const (
	fpsX, fpsY         = 5, 565
	memInfoX, memInfoY = 670, 5
	debugLineHeight    = 16
	errMsgPadding      = 20
)

// App represents the main application for the engine
type App struct {
	lastTime          float64
	lastScreenAdvance float64
	showFPS           bool
	timeScale         float64
	captureState      captureState
	capturePath       string
	captureFrames     []*image.RGBA
	gitBranch         string
	gitCommit         string
	language          string
	charset           string
	asset             *d2asset.AssetManager
	inputManager      d2interface.InputManager
	terminal          d2interface.Terminal
	scriptEngine      *d2script.ScriptEngine
	audio             d2interface.AudioProvider
	renderer          d2interface.Renderer
	screen            *d2screen.ScreenManager
	ui                *d2ui.UIManager
	tAllocSamples     *ring.Ring
	guiManager        *d2gui.GuiManager
	config            *d2config.Configuration
	*d2util.Logger
	errorMessage error
	*Options

	// reloadPath is a save waiting to be opened once the game it replaces
	// has fully gone (death screen v0's "load last save"; see ReloadGame).
	reloadPath   string
	reloadFrames int
}

// Options is used to store all of the app options that can be set with arguments
type Options struct {
	Debug    *bool
	profiler *string
	fontSet  *string
	strings  *string
	Server   *d2networking.ServerOptions
	LogLevel *d2util.LogLevel

	// classic is -classic: Diablo II's game (d2asset.AssetManager.Classic).
	classic bool

	// editor is -editor: open the World Editor instead of the main menu, and
	// editorMap is the map it opens ("" is the village, d2gamescreen's
	// DefaultEditorMap).
	editor    bool
	editorMap string
}

const (
	bytesToMegabyte = 1024 * 1024
	nSamplesTAlloc  = 100
	debugPopN       = 6
)

const (
	appLoggerPrefix = "App"
)

// Create creates a new instance of the application
func Create(gitBranch, gitCommit string) *App {
	runtime.LockOSThread()

	logger := d2util.NewLogger()
	logger.SetPrefix(appLoggerPrefix)

	app := &App{
		Logger:    logger,
		gitBranch: gitBranch,
		gitCommit: gitCommit,
		Options: &Options{
			Server: &d2networking.ServerOptions{},
		},
	}
	app.harnessEarlyInit() // no-op unless built with -tags harness
	app.Infof("OpenDiablo2 - Open source Diablo 2 engine")

	app.parseArguments()

	app.SetLevel(*app.Options.LogLevel)

	app.asset, app.errorMessage = d2asset.NewAssetManager(*app.Options.LogLevel)
	if app.asset != nil {
		app.asset.SetClassic(app.Options.classic)
	}

	return app
}

func updateNOOP() error {
	return nil
}

func (a *App) startDedicatedServer() error {
	min, max := d2networking.ServerMinPlayers, d2networking.ServerMaxPlayersDefault
	maxPlayers := d2math.ClampInt(*a.Options.Server.MaxPlayers, min, max)

	srvChanIn := make(chan int)
	srvChanLog := make(chan string)

	srvErr := d2networking.StartDedicatedServer(a.asset, srvChanIn, srvChanLog, *a.Options.LogLevel, maxPlayers)
	if srvErr != nil {
		return srvErr
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM) // This traps Control-c to safely shut down the server

	go func() {
		<-c
		srvChanIn <- d2networking.ServerEventStop
	}()

	for {
		for data := range srvChanLog {
			a.Info(data)
		}
	}
}

func (a *App) loadEngine() error {
	// Create our renderer
	renderer, err := ebiten.CreateRenderer(a.config)
	if err != nil {
		return err
	}

	a.renderer = renderer

	if a.errorMessage != nil {
		return a.renderer.Run(a.updateInitError, updateNOOP, 800, 600, "OpenDiablo2")
	}

	audio := ebiten2.CreateAudio(*a.Options.LogLevel, a.asset)

	// The harness build wraps the real keyboard/mouse in a scripted overlay
	// (P3 spec E6); untagged builds get the real service back unchanged.
	inputManager := d2input.NewInputManagerWithService(a.harnessInputService(ebiteninput.InputService{}))

	term, err := d2term.New(inputManager)
	if err != nil {
		return err
	}

	scriptEngine := d2script.CreateScriptEngine()

	uiManager := d2ui.NewUIManager(a.asset, renderer, inputManager, *a.Options.LogLevel, audio)

	a.inputManager = inputManager
	a.terminal = term
	a.scriptEngine = scriptEngine
	a.audio = audio
	a.ui = uiManager
	a.tAllocSamples = createZeroedRing(nSamplesTAlloc)

	return nil
}

func (a *App) parseArguments() {
	const (
		descProfile = "Profiles the program,\none of (cpu, mem, block, goroutine, trace, thread, mutex)"
		descPlayers = "Sets the number of max players for the dedicated server"
		descLogging = "Enables verbose logging. Log levels will include those below it.\n" +
			" 0 disables log messages\n" +
			" 1 shows fatal\n" +
			" 2 shows error\n" +
			" 3 shows warning\n" +
			" 4 shows info\n" +
			" 5 shows debug\n"
	)

	a.Options.profiler = flag.String("profile", "", descProfile)
	a.Options.Server.Dedicated = flag.Bool("dedicated", false, "Starts a dedicated server")
	a.Options.Server.MaxPlayers = flag.Int("players", 0, descPlayers)
	a.Options.LogLevel = flag.Int("l", d2util.LogLevelDefault, descLogging)
	showVersion := flag.Bool("v", false, "Show version")
	showHelp := flag.Bool("h", false, "Show help")
	// Strigoi's own four are the default (strigoi_defaults.go); "diablo" names
	// Diablo II's for any one, -classic for all of them.
	classic := flag.Bool("classic", false, classicFlagHelp)
	a.Options.strings = flag.String("strings", "", "the string table every label is answered from (default "+defaultStrings+"; diablo = Diablo II's)")
	a.Options.fontSet = flag.String("fonts", "", "the font set every word is drawn in (default "+defaultFonts+"; diablo = Diablo II's)")
	heroArt := flag.String("hero", "", "the hero's PNG manifest (default "+defaultHero+"; diablo = Diablo II's class art)")
	authoredMap := flag.String("map", "", "the authored Tiled map played on (default "+defaultMap+"; diablo = Diablo II's generated Act 1)")
	editorFlag := &editorFlagValue{}
	flag.Var(editorFlag, "editor", "open the World Editor on a map instead of the main menu (default "+defaultMap+"): -editor, -editor <path> or -editor=<path>")
	serverPort := flag.String("server-port", "6669", "the port a local game's server listens on for other players (0 = any free port, which is how the playtest harness runs several games at once)")

	flag.Usage = func() {
		fmt.Printf("usage: %s [<flags>]\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	a.harnessRegisterFlags() // no-op unless built with -tags harness
	flag.Parse()

	if *a.Options.LogLevel >= d2util.LogLevelUnspecified {
		*a.Options.LogLevel = d2util.LogLevelDefault
	}

	// Set before any game exists, so the first world is the chosen one.
	given := map[string]string{}

	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "map", "hero", "fonts", "strings":
			given[f.Name] = f.Value.String()
		}
	})

	launch := resolveLaunch(*classic, given)
	a.Options.classic = *classic

	d2server.SetPort(*serverPort)

	d2mapgen.SetAuthoredMap(launch.Map)
	d2mapentity.SetHeroArt(launch.Hero)
	*a.Options.fontSet = launch.Fonts
	*a.Options.strings = launch.Strings

	_, _ = authoredMap, heroArt // read through flag.Visit above

	// -editor is resolved AFTER the launch choice, so -editor -map <other.tmj>
	// still edits the map the rest of the game would play.
	a.Options.editor, a.Options.editorMap = editorFlag.resolve(launch.Map, flag.Args())

	if *showVersion {
		a.Infof("version: OpenDiablo2 (%s %s)", a.gitBranch, a.gitCommit)
		os.Exit(0)
	}

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}
}

// LoadConfig loads the OpenDiablo2 config file
func (a *App) LoadConfig() (*d2config.Configuration, error) {
	// by now the, the loader has initialized and added our config dirs as sources...
	configBaseName := filepath.Base(d2config.DefaultConfigPath())

	configAsset, _ := a.asset.LoadAsset(configBaseName)

	config := &d2config.Configuration{}
	config.SetPath(d2config.DefaultConfigPath())

	// create the default if not found
	if configAsset == nil {
		config = d2config.DefaultConfig()

		fullPath := filepath.Join(config.Dir(), config.Base())
		config.SetPath(fullPath)

		a.Infof("creating default configuration file at %s...", fullPath)

		saveErr := config.Save()

		return config, saveErr
	}

	if err := json.NewDecoder(configAsset).Decode(config); err != nil {
		return nil, err
	}

	a.Infof("loaded configuration file from %s", config.Path())

	return config, nil
}

// Run executes the application and kicks off the entire game process
func (a *App) Run() (err error) {
	// add our possible config directories
	_ = a.asset.AddSource(filepath.Dir(d2config.LocalConfigPath()), types.AssetSourceFileSystem)
	_ = a.asset.AddSource(filepath.Dir(d2config.DefaultConfigPath()), types.AssetSourceFileSystem)

	if a.config, err = a.LoadConfig(); err != nil {
		return err
	}

	// start profiler if argument was supplied
	if len(*a.Options.profiler) > 0 {
		profiler := enableProfiler(*a.Options.profiler, a)
		if profiler != nil {
			defer profiler.Stop()
		}
	}

	// start the server if `--listen` option was supplied
	if *a.Options.Server.Dedicated {
		if err := a.startDedicatedServer(); err != nil {
			return err
		}
	}

	if err := a.loadEngine(); err != nil {
		a.renderer.ShowPanicScreen(err.Error())
		return err
	}

	windowTitle := fmt.Sprintf("OpenDiablo2 (%s)", a.gitBranch)

	// If we fail to initialize, we will show the error screen
	if err := a.initialize(); err != nil {
		if a.errorMessage == nil {
			a.errorMessage = err // if there was an error during init, don't clobber it
		}

		gameErr := a.renderer.Run(a.updateInitError, updateNOOP, 800, 600, windowTitle)
		if gameErr != nil {
			return gameErr
		}

		return err
	}

	if a.Options.editor {
		a.ToWorldEditor(a.Options.editorMap)
	} else {
		a.ToMainMenu()
	}

	a.harnessStart() // no-op unless built with -tags harness and run with -harness

	if err := a.renderer.Run(a.update, a.advance, 800, 600, windowTitle); err != nil {
		return err
	}

	return nil
}

func (a *App) renderDebug(target d2interface.Surface) {
	if !a.showFPS {
		return
	}

	vsyncEnabled := a.renderer.GetVSyncEnabled()
	fps := a.renderer.CurrentFPS()
	cx, cy := a.renderer.GetCursorPos()

	target.PushTranslation(fpsX, fpsY)
	target.DrawTextf("vsync:" + strconv.FormatBool(vsyncEnabled) + "\nFPS:" + strconv.Itoa(int(fps)))
	target.Pop()

	var m runtime.MemStats

	runtime.ReadMemStats(&m)
	target.PushTranslation(memInfoX, memInfoY)
	target.DrawTextf("Alloc    " + strconv.FormatInt(int64(m.Alloc)/bytesToMegabyte, 10))
	target.PushTranslation(0, debugLineHeight)
	target.DrawTextf("TAlloc/s " + strconv.FormatFloat(a.allocRate(m.TotalAlloc, fps), 'f', 2, 64))
	target.PushTranslation(0, debugLineHeight)
	target.DrawTextf("Pause    " + strconv.FormatInt(int64(m.PauseTotalNs/bytesToMegabyte), 10))
	target.PushTranslation(0, debugLineHeight)
	target.DrawTextf("HeapSys  " + strconv.FormatInt(int64(m.HeapSys/bytesToMegabyte), 10))
	target.PushTranslation(0, debugLineHeight)
	target.DrawTextf("NumGC    " + strconv.FormatInt(int64(m.NumGC), 10))
	target.PushTranslation(0, debugLineHeight)
	target.DrawTextf("Coords   " + strconv.FormatInt(int64(cx), 10) + "," + strconv.FormatInt(int64(cy), 10))
	target.PopN(debugPopN)
}

func (a *App) renderCapture(target d2interface.Surface) error {
	cleanupCapture := func() {
		a.captureState = captureStateNone
		a.capturePath = ""
		a.captureFrames = nil
	}

	switch a.captureState {
	case captureStateFrame:
		defer cleanupCapture()

		if err := a.doCaptureFrame(target); err != nil {
			return err
		}
	case captureStateGif:
		a.doCaptureGif(target)
	case captureStateNone:
		if len(a.captureFrames) > 0 {
			defer cleanupCapture()

			if err := a.convertFramesToGif(); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *App) render(target d2interface.Surface) {
	a.screen.Render(target)
	a.ui.Render(target)

	if err := a.guiManager.Render(target); err != nil {
		return
	}

	a.renderDebug(target)

	if err := a.renderCapture(target); err != nil {
		return
	}

	if err := a.terminal.Render(target); err != nil {
		return
	}

	a.harnessDrainDraw(target) // no-op unless built with -tags harness
}

func (a *App) advance() error {
	a.harnessDrainUpdate() // no-op unless built with -tags harness

	// While the harness holds the simulation paused, frames advance with zero
	// deltas and a frozen clock (P3 spec §3.4); stepping happens inside the
	// harness's queued commands via advanceOnce, never through the wall clock.
	if u, s, scr, cur, held := a.harnessStepDeltas(); held {
		return a.advanceOnce(u, s, scr, cur)
	}

	current := d2util.Now()
	elapsedUnscaled, elapsed, elapsedLastScreenAdvance := d2util.FrameDeltas(current, a.lastTime, a.lastScreenAdvance, a.timeScale)
	a.lastTime = current
	a.lastScreenAdvance = current

	return a.advanceOnce(elapsedUnscaled, elapsed, elapsedLastScreenAdvance, current)
}

// advanceOnce runs one simulation tick with the given deltas (seconds) and
// clock reading. The playtest harness drives it directly when stepping.
func (a *App) advanceOnce(elapsedUnscaled, elapsed, elapsedLastScreenAdvance, current float64) error {
	if err := a.screen.Advance(elapsedLastScreenAdvance); err != nil {
		return err
	}

	a.advanceReload()

	a.ui.Advance(elapsed)

	if err := a.inputManager.Advance(elapsed, current); err != nil {
		return err
	}

	if err := a.guiManager.Advance(elapsed); err != nil {
		return err
	}

	if err := a.terminal.Advance(elapsedUnscaled); err != nil {
		return err
	}

	return nil
}

func (a *App) update(target d2interface.Surface) error {
	a.render(target)

	if target.GetDepth() > 0 {
		return errors.New("detected surface stack leak")
	}

	return nil
}

func (a *App) allocRate(totalAlloc uint64, fps float64) float64 {
	a.tAllocSamples.Value = totalAlloc
	a.tAllocSamples = a.tAllocSamples.Next()
	deltaAllocPerFrame := float64(totalAlloc-a.tAllocSamples.Value.(uint64)) / nSamplesTAlloc

	return deltaAllocPerFrame * fps / bytesToMegabyte
}

func (a *App) doCaptureFrame(target d2interface.Surface) error {
	fp, err := os.Create(a.capturePath)
	if err != nil {
		a.terminal.Errorf("failed to create %q", a.capturePath)
		return err
	}

	screenshot := target.Screenshot()
	if err := png.Encode(fp, screenshot); err != nil {
		return err
	}

	if err := fp.Close(); err != nil {
		a.terminal.Errorf("failed to create %q", a.capturePath)
		return nil
	}

	a.terminal.Infof("saved frame to %s", a.capturePath)

	return nil
}

func (a *App) doCaptureGif(target d2interface.Surface) {
	screenshot := target.Screenshot()
	a.captureFrames = append(a.captureFrames, screenshot)
}

func (a *App) convertFramesToGif() error {
	fp, err := os.Create(a.capturePath)
	if err != nil {
		return err
	}

	defer func() {
		if err := fp.Close(); err != nil {
			a.Fatal(err.Error())
		}
	}()

	var (
		framesTotal  = len(a.captureFrames)
		framesPal    = make([]*image.Paletted, framesTotal)
		frameDelays  = make([]int, framesTotal)
		framesPerCPU = framesTotal / runtime.NumCPU()
	)

	var waitGroup sync.WaitGroup

	for i := 0; i < framesTotal; i += framesPerCPU {
		waitGroup.Add(1)

		go func(start, end int) {
			defer waitGroup.Done()

			for j := start; j < end; j++ {
				var buffer bytes.Buffer
				if err := gif.Encode(&buffer, a.captureFrames[j], nil); err != nil {
					panic(err)
				}

				framePal, err := gif.Decode(&buffer)
				if err != nil {
					panic(err)
				}

				framesPal[j] = framePal.(*image.Paletted)
				frameDelays[j] = 5
			}
		}(i, d2math.MinInt(i+framesPerCPU, framesTotal))
	}

	waitGroup.Wait()

	if err := gif.EncodeAll(fp, &gif.GIF{Image: framesPal, Delay: frameDelays}); err != nil {
		return err
	}

	a.Infof("saved animation to %s", a.capturePath)

	return nil
}

func createZeroedRing(n int) *ring.Ring {
	r := ring.New(n)
	for i := 0; i < n; i++ {
		r.Value = uint64(0)
		r = r.Next()
	}

	return r
}

func enableProfiler(profileOption string, a *App) interface{ Stop() } {
	var options []func(*profile.Profile)

	switch strings.ToLower(strings.Trim(profileOption, " ")) {
	case "cpu":
		a.Logger.Debug("CPU profiling is enabled.")

		options = append(options, profile.CPUProfile)
	case "mem":
		a.Logger.Debug("Memory profiling is enabled.")

		options = append(options, profile.MemProfile)
	case "block":
		a.Logger.Debug("Block profiling is enabled.")

		options = append(options, profile.BlockProfile)
	case "goroutine":
		a.Logger.Debug("Goroutine profiling is enabled.")

		options = append(options, profile.GoroutineProfile)
	case "trace":
		a.Logger.Debug("Trace profiling is enabled.")

		options = append(options, profile.TraceProfile)
	case "thread":
		a.Logger.Debug("Thread creation profiling is enabled.")

		options = append(options, profile.ThreadcreationProfile)
	case "mutex":
		a.Logger.Debug("Mutex profiling is enabled.")

		options = append(options, profile.MutexProfile)
	}

	options = append(options, profile.ProfilePath("./pprof/"))

	if len(options) > 1 {
		return profile.Start(options...)
	}

	return nil
}

func (a *App) updateInitError(target d2interface.Surface) error {
	target.Clear(colornames.Darkred)
	target.PushTranslation(errMsgPadding, errMsgPadding)
	target.DrawTextf(a.errorMessage.Error())

	return nil
}

// ToMainMenu forces the game to transition to the Main Menu
func (a *App) ToMainMenu(errorMessageOptional ...string) {
	buildInfo := d2gamescreen.BuildInfo{Branch: a.gitBranch, Commit: a.gitCommit}

	mainMenu, err := d2gamescreen.CreateMainMenu(a, a.asset, a.renderer, a.inputManager, a.audio, a.ui, buildInfo,
		*a.Options.LogLevel, errorMessageOptional...)
	if err != nil {
		a.Error(err.Error())
		return
	}

	a.screen.SetNextScreen(mainMenu)
	a.harnessNoteScreen("main_menu") // no-op unless built with -tags harness
}

// ToSelectHero forces the game to transition to the Select Hero (create character) screen
func (a *App) ToSelectHero(connType d2clientconnectiontype.ClientConnectionType, host string) {
	selectHero, err := d2gamescreen.CreateSelectHeroClass(a, a.asset, a.renderer, a.audio, a.ui, connType, *a.Options.LogLevel, host)
	if err != nil {
		a.Error(err.Error())
		return
	}

	a.screen.SetNextScreen(selectHero)
}

// ToCreateGame forces the game to transition to the Create Game screen
func (a *App) ToCreateGame(filePath string, connType d2clientconnectiontype.ClientConnectionType, host string) {
	a.harnessGameBegins() // no-op unless built with -tags harness

	gameClient, err := d2client.Create(connType, a.asset, *a.Options.LogLevel, a.scriptEngine)
	if err != nil || gameClient == nil {
		reason := "could not create client"
		if err != nil {
			reason = err.Error()
		}

		a.Error(reason)
		a.ToMainMenu(gameStartFailed + reason)

		return
	}

	game, reason := startGame(gameClient, host, filePath, func() (*d2gamescreen.Game, error) {
		return d2gamescreen.CreateGame(
			a, a.asset, a.ui, a.renderer, a.inputManager, a.audio, gameClient, a.terminal, *a.Options.LogLevel, a.guiManager,
		)
	})

	// On game == nil, not reason: a nil screen must never reach SetNextScreen.
	if game == nil {
		a.Error(reason)
		a.ToMainMenu(reason)

		return
	}

	a.screen.SetNextScreen(game)
	a.harnessNoteGame(gameClient, game) // no-op unless built with -tags harness
}

// gameStartFailed begins the main menu's line when a game could not start.
const gameStartFailed = "The game could not start: "

// gameConn is what startGame asks of a game client (d2client.GameClient).
type gameConn interface {
	Open(connectionString, saveFilePath string) error
	Close() error
}

// startGame opens the client and builds the game screen on it, and answers
// either the screen or why there is none -- never both, never neither.
//
// A FAILED CreateGame CLOSES THE CLIENT (BUG-25, 27 Sep 2026). It used to be
// logged and the screen set anyway: SetNextScreen got a nil *Game, and the
// client stayed open -- for a local game, a server holding the port, so the
// next game could not connect either. Now the client is closed and the
// player goes back to the main menu with the reason, as a refused join does.
// A failed Open closes nothing: it did not open (and a local connection
// whose server was never made cannot be closed).
func startGame(client gameConn, host, filePath string, create func() (*d2gamescreen.Game, error)) (*d2gamescreen.Game, string) {
	if err := client.Open(host, filePath); err != nil {
		return nil, fmt.Sprintf("can not connect to the host: %s", host)
	}

	game, err := create()
	if err == nil && game != nil {
		return game, ""
	}

	if err == nil {
		err = errors.New("no game screen")
	}

	if closeErr := client.Close(); closeErr != nil {
		err = fmt.Errorf("%w (and closing the client: %v)", err, closeErr)
	}

	return nil, gameStartFailed + err.Error()
}

// ToCharacterSelect forces the game to transition to the Character Select (load character) screen
func (a *App) ToCharacterSelect(connType d2clientconnectiontype.ClientConnectionType, connHost string) {
	characterSelect, err := d2gamescreen.CreateCharacterSelect(a, a.asset, a.renderer, a.inputManager,
		a.audio, a.ui, connType, *a.Options.LogLevel, connHost)
	if err != nil {
		a.Errorf("unable to create character select screen: %s", err)
	}

	a.screen.SetNextScreen(characterSelect)
}

// ToMapEngineTest forces the game to transition to the map engine test screen
func (a *App) ToMapEngineTest(region, level int) {
	met, err := d2gamescreen.CreateMapEngineTest(region, level, a.asset, a.terminal, a.renderer, a.inputManager, a.audio,
		*a.Options.LogLevel, a.screen)
	if err != nil {
		a.Error(err.Error())
		return
	}

	a.screen.SetNextScreen(met)
}

// ToWorldEditor opens the World Editor on mapPath ("" is the village).
//
// A map that will not open is NOT shown as an empty grid: the editor refuses,
// and the reason goes to the main menu where the other start-up failures go. A
// designer who asked to edit a file and got a blank screen has been told a lie
// about it.
func (a *App) ToWorldEditor(mapPath string) {
	editor, err := d2gamescreen.CreateEditor(mapPath, a.asset, a.terminal, a.renderer,
		a.inputManager, a.ui, a, *a.Options.LogLevel)
	if err != nil {
		a.Error(err.Error())
		a.ToMainMenu("The world editor could not open that map: " + err.Error())

		return
	}

	// The harness must be able to tell "the editor opened" from "the editor
	// refused and we are on the menu". Without this a script that launches with
	// -editor and reads the screen back cannot fail, which is the hollow test
	// this project has been caught by before.
	a.harnessNoteScreen("world_editor") // no-op unless built with -tags harness

	a.screen.SetNextScreen(editor)
}

// ToPlaytest starts a real game on a map the editor has just written.
//
// It uses the two mechanisms that ALREADY work rather than a third: the authored
// map is a process-wide setting (d2mapgen.SetAuthoredMap -- how -map reaches a
// game at :269 and how the harness swaps a map, harness_tools.go:378-380), and
// the way into a game is the hero screens the main menu uses
// (main_menu.go:430-438). The editor names a FILE and has no save path to hand
// to ToCreateGame, so it cannot go there directly; the player picks his hero and
// the world he lands in is the one the editor wrote.
func (a *App) ToPlaytest(mapPath string) {
	if mapPath != "" {
		d2mapgen.SetAuthoredMap(mapPath)
	}

	factory, err := d2hero.NewHeroStateFactory(a.asset)
	if err != nil {
		a.Error(err.Error())
		a.ToSelectHero(d2clientconnectiontype.Local, "")

		return
	}

	if factory.HasGameStates() {
		a.ToCharacterSelect(d2clientconnectiontype.Local, "")
		return
	}

	a.ToSelectHero(d2clientconnectiontype.Local, "")
}

// editorFlagValue is -editor, which may be given alone or with a map.
//
// flag has no optional-argument string, so this is a flag.Value that also
// answers IsBoolFlag: "-editor" on its own then parses as the bare switch, and
// "-editor=<path>" carries a path. "-editor <path>" leaves the path in
// flag.Args(), which resolve picks up -- so all three spellings work and none of
// them changes what -map, -classic or the rest do.
type editorFlagValue struct {
	set  bool
	path string
}

func (f *editorFlagValue) String() string {
	if f == nil {
		return ""
	}

	return f.path
}

func (f *editorFlagValue) Set(v string) error {
	f.set = true

	if v != "true" {
		f.path = v
	}

	return nil
}

// IsBoolFlag lets "-editor" stand alone. flag checks for this method by
// interface assertion.
func (f *editorFlagValue) IsBoolFlag() bool { return true }

// resolve says whether the editor was asked for and which map it opens: the
// value given to the flag, else a .tmj left in the positional arguments, else
// the map the rest of the launch would play, else the editor's own default.
func (f *editorFlagValue) resolve(launchMap string, args []string) (bool, string) {
	if !f.set {
		return false, ""
	}

	if f.path != "" {
		return true, f.path
	}

	for _, a := range args {
		if strings.EqualFold(filepath.Ext(a), ".tmj") {
			return true, a
		}
	}

	return true, launchMap
}

// ToCredits forces the game to transition to the credits screen
func (a *App) ToCredits() {
	a.screen.SetNextScreen(d2gamescreen.CreateCredits(a, a.asset, a.renderer, *a.Options.LogLevel, a.ui))
}

// ToCinematics forces the game to transition to the cinematics menu
func (a *App) ToCinematics() {
	a.screen.SetNextScreen(d2gamescreen.CreateCinematics(a, a.asset, a.renderer, a.audio, *a.Options.LogLevel, a.ui))
}
