//go:build harness

package d2app

// Phase 3 playtest harness — the MCP server and the session/action tools
// (M3.2, P3 spec §4.1, §4.4). Observation tools live in harness_obs.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	mrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2term"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2server"
)

func harnessBoolPtr(b bool) *bool { return &b }

// annotation presets (P3 §4: honest annotations; openWorldHint false everywhere)
func harnessAnnRO(idem bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		IdempotentHint:  idem,
		OpenWorldHint:   harnessBoolPtr(false),
		DestructiveHint: harnessBoolPtr(false),
	}
}

func harnessAnnMut(destructive bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		OpenWorldHint:   harnessBoolPtr(false),
		DestructiveHint: harnessBoolPtr(destructive),
	}
}

func harnessText(format string, args ...interface{}) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}
}

func harnessErr(code, msg, hint string) error {
	if hint == "" {
		return fmt.Errorf("%s: %s", code, msg)
	}

	return fmt.Errorf("%s: %s — %s", code, msg, hint)
}

var harnessHeroClasses = map[string]d2enum.Hero{
	"barbarian":   d2enum.HeroBarbarian,
	"necromancer": d2enum.HeroNecromancer,
	"paladin":     d2enum.HeroPaladin,
	"assassin":    d2enum.HeroAssassin,
	"sorceress":   d2enum.HeroSorceress,
	"amazon":      d2enum.HeroAmazon,
	"druid":       d2enum.HeroDruid,
}

func harnessHeroName(h d2enum.Hero) string {
	for name, v := range harnessHeroClasses {
		if v == h {
			return name
		}
	}

	return fmt.Sprintf("hero-%d", int(h))
}

func (a *App) harnessServe() {
	srv := mcp.NewServer(&mcp.Implementation{Name: "strigoi-harness", Version: harnessVersion}, nil)

	a.harnessAddSessionTools(srv)
	a.harnessAddObservationTools(srv)
	a.harnessAddActionTools(srv)
	a.harnessAddTimeTools(srv)

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{})

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)

	server := &http.Server{
		Addr:              *harness.addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		a.Errorf("harness: server stopped: %v", err)
	}
}

// ---------------------------------------------------------------- session ---

type harnessPingOut struct {
	Ok      bool    `json:"ok"`
	Commit  string  `json:"commit"`
	Branch  string  `json:"branch"`
	Version string  `json:"harness_version"`
	Mode    string  `json:"mode"`
	Tick    int64   `json:"tick"`
	UptimeS float64 `json:"uptime_s"`
	InGame  bool    `json:"in_game"`
	RunDir  string  `json:"run_dir"`
}

type harnessGameInfoOut struct {
	Screen      string   `json:"screen"`
	Loading     bool     `json:"loading"`
	InGame      bool     `json:"in_game"`
	SavePath    string   `json:"save_path,omitempty"`
	HeroName    string   `json:"hero_name,omitempty"`
	HeroClass   string   `json:"hero_class,omitempty"`
	Act         int      `json:"act,omitempty"`
	Seed        int64    `json:"seed,omitempty"`
	TimeMode    string   `json:"time_mode"`
	Tick        int64    `json:"tick"`
	EntityCount int      `json:"entity_count"`
	Player      string   `json:"player,omitempty"`
	RunDir      string   `json:"run_dir"`
	Systems     []string `json:"systems"`
}

type harnessNavigateIn struct {
	Screen string `json:"screen" jsonschema:"one of: main_menu, character_select, select_hero, credits"`
}

type harnessNavigateOut struct {
	Screen string `json:"screen"`
}

type harnessStartGameIn struct {
	SavePath    string  `json:"save_path,omitempty" jsonschema:"path to an existing .od2 save; mutually exclusive with hero"`
	HeroName    string  `json:"hero_name,omitempty" jsonschema:"with hero_class: create a fresh hero and save it first"`
	HeroClass   string  `json:"hero_class,omitempty" jsonschema:"one of: amazon, assassin, barbarian, druid, necromancer, paladin, sorceress"`
	Seed        *int64  `json:"seed,omitempty" jsonschema:"nonzero: seed map generation, the world RNG, and entity IDs for a reproducible run (P3 E3/E5); overrides a pending set_seed"`
	WaitSeconds float64 `json:"wait_seconds,omitempty" jsonschema:"how long to wait for the world to be ready; default 25"`
}

type harnessStartGameOut struct {
	Player   string     `json:"player"`
	SavePath string     `json:"save_path"`
	Seed     int64      `json:"seed"`
	Spawn    [2]float64 `json:"spawn_tile"`
	WaitedS  float64    `json:"waited_s"`
}

type harnessSaveGameOut struct {
	SavePath string `json:"save_path"`
}

type harnessQuitIn struct {
	Confirm bool `json:"confirm" jsonschema:"must be true"`
}

type harnessQuitOut struct {
	Quitting bool `json:"quitting"`
}

func (a *App) harnessAddSessionTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_ping",
		Description: "Liveness and identity of the running game: build commit, harness version, time mode, tick count. Safe anytime.",
		Annotations: harnessAnnRO(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessPingOut, error) {
		harnessLogCall("strigoi_ping")

		client, _ := harnessGame()
		out := harnessPingOut{
			Ok:      true,
			Commit:  a.gitCommit,
			Branch:  a.gitBranch,
			Version: harnessVersion,
			Mode:    harnessTimeSnapshot().Mode,
			Tick:    atomic.LoadInt64(&harness.tick),
			UptimeS: time.Since(harness.started).Seconds(),
			InGame:  client != nil,
			RunDir:  harness.runDir,
		}

		return harnessText("ok · commit %s · tick %d · in_game=%v", out.Commit, out.Tick, out.InGame), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_get_game_info",
		Description: "Everything a script needs to orient: screen (a hint — a human clicking through menus is not tracked), loading state, hero, seed, tick, entity count, run dir, registered harness systems. Reads on the game goroutine.",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessGameInfoOut, error) {
		harnessLogCall("strigoi_get_game_info")

		var out harnessGameInfoOut

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			out.Screen = harnessScreenHint()
			out.Loading = a.screen.IsLoading()
			out.TimeMode = harnessTimeSnapshot().Mode
			out.Tick = atomic.LoadInt64(&harness.tick)
			out.RunDir = harness.runDir
			out.Systems = harnessSystemNames()

			if client == nil {
				return
			}

			out.InGame = true
			out.Seed = client.Seed
			out.EntityCount = len(client.MapEngine.Entities())

			if client.GameState != nil {
				out.SavePath = client.GameState.FilePath
				out.HeroName = client.GameState.HeroName
				out.HeroClass = harnessHeroName(client.GameState.HeroType)
				out.Act = client.GameState.Act
			}

			if client.PlayerID != "" {
				out.Player = harnessHandleFor(client.PlayerID, client.PlayerID)
			}
		})
		if err != nil {
			return nil, out, err
		}

		return harnessText("screen=%s in_game=%v entities=%d tick=%d", out.Screen, out.InGame, out.EntityCount, out.Tick), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_navigate",
		Description: "Jump between screens without the mouse: main_menu, character_select, select_hero, credits. Starting a game goes through strigoi_start_game instead.",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessNavigateIn) (*mcp.CallToolResult, harnessNavigateOut, error) {
		harnessLogCall("strigoi_navigate")

		target := strings.ToLower(strings.TrimSpace(in.Screen))

		err := harnessOnUpdate(func() {
			switch target {
			case "main_menu":
				a.ToMainMenu()
			case "character_select":
				a.ToCharacterSelect(d2clientconnectiontype.Local, "")
				a.harnessNoteScreen("character_select")
			case "select_hero":
				a.ToSelectHero(d2clientconnectiontype.Local, "")
				a.harnessNoteScreen("select_hero")
			case "credits":
				a.ToCredits()
				a.harnessNoteScreen("credits")
			}
		})
		if err != nil {
			return nil, harnessNavigateOut{}, err
		}

		switch target {
		case "main_menu", "character_select", "select_hero", "credits":
		default:
			return nil, harnessNavigateOut{}, harnessErr("BAD_ARGUMENT",
				fmt.Sprintf("unknown screen %q", in.Screen),
				"use main_menu, character_select, select_hero, or credits")
		}

		return harnessText("navigating to %s", target), harnessNavigateOut{Screen: target}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_start_game",
		Description: "Load a .od2 save (save_path) or create a fresh hero (hero_name + hero_class) and enter the world, waiting until the local player exists. Coordinates everywhere are world tiles.",
		Annotations: harnessAnnMut(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessStartGameIn) (*mcp.CallToolResult, harnessStartGameOut, error) {
		harnessLogCall("strigoi_start_game")

		var out harnessStartGameOut

		hasHero := in.HeroName != "" || in.HeroClass != ""
		if (in.SavePath == "") == !hasHero {
			return nil, out, harnessErr("BAD_ARGUMENT", "provide exactly one of save_path or hero_name+hero_class", "")
		}

		wait := in.WaitSeconds
		if wait <= 0 {
			wait = 25
		}

		// step 1, on the game goroutine: resolve the save and start the game
		var startErr error

		savePath := in.SavePath
		seed := harnessConsumeStartSeed(in.Seed)

		err := harnessOnUpdate(func() {
			if client, _ := harnessGame(); client != nil {
				startErr = harnessErr("ALREADY_IN_GAME", "a game is already running", "strigoi_navigate to main_menu first")
				return
			}

			// Seed the in-process server's map generation (one-shot, E3) and
			// make entity IDs reproducible (E5). Unseeded runs restore the
			// default crypto-random IDs and the wall-clock map seed.
			if seed != 0 {
				d2server.SetNextGameSeed(seed)
				uuid.SetRand(mrand.New(mrand.NewSource(seed)))
			} else {
				d2server.SetNextGameSeed(0)
				uuid.SetRand(nil)
			}

			if hasHero {
				heroClass, ok := harnessHeroClasses[strings.ToLower(strings.TrimSpace(in.HeroClass))]
				if !ok {
					startErr = harnessErr("BAD_ARGUMENT", fmt.Sprintf("unknown hero_class %q", in.HeroClass), "use amazon, assassin, barbarian, druid, necromancer, paladin, or sorceress")
					return
				}

				name := in.HeroName
				if name == "" {
					name = "Harness"
				}

				factory, err := d2hero.NewHeroStateFactory(a.asset)
				if err != nil {
					startErr = harnessErr("INTERNAL", fmt.Sprintf("hero factory: %v", err), "")
					return
				}

				classStats := a.asset.Records.Character.Stats[heroClass]
				statsState := factory.CreateHeroStatsState(heroClass, classStats)

				state, err := factory.CreateHeroState(name, heroClass, statsState)
				if err != nil {
					startErr = harnessErr("INTERNAL", fmt.Sprintf("hero state: %v", err), "")
					return
				}

				if err := factory.Save(state); err != nil {
					startErr = harnessErr("INTERNAL", fmt.Sprintf("saving the new hero: %v", err), "")
					return
				}

				savePath = state.FilePath
			} else if _, err := os.Stat(savePath); err != nil {
				startErr = harnessErr("SAVE_NOT_FOUND", fmt.Sprintf("no save at %q", savePath), "pass hero_name+hero_class to create one")
				return
			}

			a.ToCreateGame(savePath, d2clientconnectiontype.Local, "")
		})
		if err != nil {
			return nil, out, err
		}

		if startErr != nil {
			return nil, out, startErr
		}

		// step 2: poll until the world is ready
		deadline := time.Now().Add(time.Duration(wait * float64(time.Second)))
		begin := time.Now()

		for {
			ready := false

			err := harnessOnUpdate(func() {
				client, _ := harnessGame()
				if client == nil || client.PlayerID == "" {
					return
				}

				player, ok := client.Players[client.PlayerID]
				if !ok || player == nil {
					return
				}

				if a.screen.IsLoading() || a.screen.CurrentScreen() == nil {
					return
				}

				ready = true
				out.Player = harnessHandleFor(client.PlayerID, client.PlayerID)
				out.Seed = client.Seed
				out.SavePath = savePath

				world := player.Position.World()
				out.Spawn = [2]float64{world.X(), world.Y()}
			})
			if err != nil {
				return nil, out, err
			}

			if ready {
				out.WaitedS = time.Since(begin).Seconds()
				return harnessText("in game · player %s at tile %.1f,%.1f · seed %d", out.Player, out.Spawn[0], out.Spawn[1], out.Seed), out, nil
			}

			if time.Now().After(deadline) {
				return nil, out, harnessErr("TIMEOUT_LOADING", fmt.Sprintf("the world was not ready after %.0fs", wait), "raise wait_seconds; first boot loads MPQs and is slow")
			}

			time.Sleep(100 * time.Millisecond)
		}
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_save_game",
		Description: "Write the current hero state to its .od2 save.",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessSaveGameOut, error) {
		harnessLogCall("strigoi_save_game")

		var out harnessSaveGameOut

		var saveErr error

		err := harnessOnUpdate(func() {
			client, game := harnessGame()
			if client == nil || game == nil {
				saveErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			if err := game.OnPlayerSave(); err != nil {
				saveErr = harnessErr("INTERNAL", fmt.Sprintf("save failed: %v", err), "")
				return
			}

			if client.GameState != nil {
				out.SavePath = client.GameState.FilePath
			}
		})
		if err != nil {
			return nil, out, err
		}

		if saveErr != nil {
			return nil, out, saveErr
		}

		return harnessText("saved to %s", out.SavePath), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_quit",
		Description: "Write the run manifest and exit the game process. Requires confirm=true.",
		Annotations: harnessAnnMut(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessQuitIn) (*mcp.CallToolResult, harnessQuitOut, error) {
		harnessLogCall("strigoi_quit")

		if !in.Confirm {
			return nil, harnessQuitOut{}, harnessErr("BAD_ARGUMENT", "confirm must be true", "this exits the game process")
		}

		a.harnessWriteManifest()

		go func() {
			time.Sleep(harnessQuitDelay)
			os.Exit(0)
		}()

		return harnessText("quitting"), harnessQuitOut{Quitting: true}, nil
	})
}

func (a *App) harnessWriteManifest() {
	harness.mu.Lock()
	calls := make([]string, len(harness.toolCalls))
	copy(calls, harness.toolCalls)
	harness.mu.Unlock()

	manifest := map[string]interface{}{
		"harness_version": harnessVersion,
		"commit":          a.gitCommit,
		"branch":          a.gitBranch,
		"started":         harness.started.Format(time.RFC3339),
		"ended":           time.Now().Format(time.RFC3339),
		"tick":            atomic.LoadInt64(&harness.tick),
		"tool_calls":      calls,
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(filepath.Join(harness.runDir, "run.json"), data, 0o640)
}

// ---------------------------------------------------------------- actions ---

type harnessRunConsoleIn struct {
	Command string `json:"command" jsonschema:"a bound terminal command, e.g. 'fps' or 'spawnmon fallen'"`
}

type harnessRunConsoleOut struct {
	Output []string `json:"output"`
}

type harnessMoveIn struct {
	X        float64 `json:"x" jsonschema:"target world-tile x"`
	Y        float64 `json:"y" jsonschema:"target world-tile y"`
	Wait     bool    `json:"wait,omitempty" jsonschema:"block until arrived/stuck/timeout; steps the sim when paused, polls the wall clock when live"`
	MaxTicks int     `json:"max_ticks,omitempty" jsonschema:"wait budget in simulation ticks; default 1800 (30 sim-seconds)"`
}

type harnessMoveOut struct {
	Outcome  string     `json:"outcome"`
	Position [2]float64 `json:"position_tile"`
	Ticks    int        `json:"ticks,omitempty"`
}

// harnessPlayerPos reads the local player's world-tile position on the game
// goroutine. ok is false when no game or player exists.
func harnessPlayerPos() (x, y float64, ok bool) {
	_ = harnessOnUpdate(func() {
		client, _ := harnessGame()
		if client == nil || client.PlayerID == "" {
			return
		}

		player, exists := client.Players[client.PlayerID]
		if !exists || player == nil {
			return
		}

		world := player.Position.World()
		x, y, ok = world.X(), world.Y(), true
	})

	return x, y, ok
}

func (a *App) harnessAddActionTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_run_console",
		Description: "Execute a bound in-game terminal command and return its output lines. The terminal's own commands include fps, timescale, spawnmon, spawnitem, freecam, js; some are destructive.",
		Annotations: harnessAnnMut(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessRunConsoleIn) (*mcp.CallToolResult, harnessRunConsoleOut, error) {
		harnessLogCall("strigoi_run_console")

		var out harnessRunConsoleOut

		var cmdErr error

		err := harnessOnUpdate(func() {
			term, ok := a.terminal.(*d2term.Terminal)
			if !ok {
				cmdErr = harnessErr("INTERNAL", "terminal is not the expected implementation", "")
				return
			}

			_, before := term.HarnessOutput(1 << 30)

			if err := term.Execute(in.Command); err != nil {
				cmdErr = harnessErr("BAD_ARGUMENT", fmt.Sprintf("console: %v", err), "check the command name and argument count")
			}

			lines, _ := term.HarnessOutput(before)
			out.Output = lines
		})
		if err != nil {
			return nil, out, err
		}

		if cmdErr != nil {
			return nil, out, cmdErr
		}

		return harnessText("ran %q · %d output line(s)", in.Command, len(out.Output)), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_move_player_to",
		Description: "Send the player toward a world-tile target via the MovePlayer packet (the pathing is the engine's raycast: it stops at the last unblocked point). Fire-and-forget in M3.2; wait semantics arrive with M3.3 stepping.",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessMoveIn) (*mcp.CallToolResult, harnessMoveOut, error) {
		harnessLogCall("strigoi_move_player_to")

		var out harnessMoveOut

		var moveErr error

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			if client == nil || client.PlayerID == "" {
				moveErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			player, ok := client.Players[client.PlayerID]
			if !ok || player == nil {
				moveErr = harnessErr("NOT_IN_GAME", "the local player does not exist yet", "wait for strigoi_start_game to report ready")
				return
			}

			world := player.Position.World()

			packet, err := d2netpacket.CreateMovePlayerPacket(client.PlayerID, world.X(), world.Y(), in.X, in.Y)
			if err != nil {
				moveErr = harnessErr("INTERNAL", fmt.Sprintf("packet: %v", err), "")
				return
			}

			if err := client.SendPacketToServer(packet); err != nil {
				moveErr = harnessErr("INTERNAL", fmt.Sprintf("send: %v", err), "")
				return
			}

			out.Outcome = "sent"
			out.Position = [2]float64{world.X(), world.Y()}
		})
		if err != nil {
			return nil, out, err
		}

		if moveErr != nil {
			return nil, out, moveErr
		}

		if !in.Wait {
			return harnessText("move sent toward %.1f,%.1f from %.1f,%.1f", in.X, in.Y, out.Position[0], out.Position[1]), out, nil
		}

		// Wait for arrival: stepped when the clock is paused (deterministic),
		// wall-clock polling when live. The raycast pathfinder stops at the
		// first blocked point, so "stuck" is a normal outcome, not an error.
		const (
			checkEvery   = 30  // ticks between position checks [DIAL]
			arriveWithin = 0.3 // world tiles [DIAL]
		)

		maxTicks := in.MaxTicks
		if maxTicks <= 0 {
			maxTicks = 1800
		}

		paused := harnessTimeSnapshot().Mode == "paused"
		lastX, lastY := out.Position[0], out.Position[1]

		for used := 0; used < maxTicks; used += checkEvery {
			if paused {
				if err := a.harnessStep(checkEvery, harnessTimeSnapshot().DT); err != nil {
					return nil, out, err
				}
			} else {
				harnessSleep(checkEvery * time.Second / 60)
			}

			x, y, ok := harnessPlayerPos()
			if !ok {
				return nil, out, harnessErr("NOT_IN_GAME", "the player vanished mid-wait", "")
			}

			out.Position = [2]float64{x, y}
			out.Ticks = used + checkEvery

			if math.Hypot(x-in.X, y-in.Y) <= arriveWithin {
				out.Outcome = "arrived"
				return harnessText("arrived at %.2f,%.2f after %d ticks", x, y, out.Ticks), out, nil
			}

			if math.Hypot(x-lastX, y-lastY) < 1e-9 {
				out.Outcome = "stuck"
				return harnessText("stuck at %.2f,%.2f after %d ticks (raycast pathing stops at the first blocked point)", x, y, out.Ticks), out, nil
			}

			lastX, lastY = x, y
		}

		out.Outcome = "timeout"

		return harnessText("timeout at %.2f,%.2f after %d ticks", out.Position[0], out.Position[1], out.Ticks), out, nil
	})
}
