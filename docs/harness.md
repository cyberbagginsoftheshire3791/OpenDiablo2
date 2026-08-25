# The playtest harness (Phase 3) — v0, M3.2

An MCP (Model Context Protocol) server compiled into the game behind the
`harness` build tag, so an agent or a Go test can start a game, read state,
take screenshots, and drive the terminal. Designed in
`P3 - Playtest Harness Design Spec.md` (Video Game folder, SIGNED 24 Aug 2026);
this file is the operator's card and the living tool reference.

## Build and run

    go build -tags harness -o od2-harness.exe .   # the debug build (release builds contain none of this)
    ./od2-harness.exe -harness                    # serve MCP on http://127.0.0.1:6670/mcp
    ./od2-harness.exe -harness -harness-addr 127.0.0.1:6670 -harness-out D:\runs

Loopback only — a non-loopback `-harness-addr` is refused. No authentication:
single user, single machine, opt-in flag.

**Attach Claude Code:** the repo's `.mcp.json` declares the server; with the
game running, Claude Code sessions in this repo see the `strigoi_*` tools.
Manual: `claude mcp add --transport http strigoi http://127.0.0.1:6670/mcp`.

**Playtest scripts** (laptop only, never CI — no MPQs there):

    go test -tags playtest ./playtest/... -v -count=1

Scripts build + launch the harness binary themselves; set
`STRIGOI_HARNESS_ADDR=127.0.0.1:6670` to attach to a game you started by hand.

## Outputs (Article V)

Everything the harness writes — screenshots, surface dumps, `run.json` — goes
to `%LOCALAPPDATA%\Strigoi\harness\runs\<stamp>\` by default, or `-harness-out`.
The playtest launcher uses `<Projects>\strigoi-harness-runs\` (outside the
repo, reachable by the device bridge). Screenshots render Blizzard art: they
never enter the repo (`/harness-runs/` is gitignored and hook-blocked as
belt-and-braces), never go on public issues, never ship.

## The tools (M3.2 — 16)

Coordinates are **world tiles** (`Position.World()`); the local player is
always handle `p:1`. Errors come back as `CODE: message — hint`.

| Tool | What |
|---|---|
| `strigoi_ping` | Liveness, commit, tick, mode |
| `strigoi_get_game_info` | Screen hint, loading, hero, seed, entity count, systems |
| `strigoi_navigate` | main_menu · character_select · select_hero · credits |
| `strigoi_start_game` | Load a save or create a hero; waits until the player exists |
| `strigoi_save_game` | Write the `.od2` |
| `strigoi_quit` | Manifest + exit (confirm: true) |
| `strigoi_get_entities` | Paginated entity list; kind/near filters |
| `strigoi_get_entity` / `strigoi_get_player` | One entity, kind-specific state |
| `strigoi_get_tile` | Region, level name, 5×5 walkability |
| `strigoi_dump_map` | walk / entities / region window, ≤64×64 tiles |
| `strigoi_read_log` | Ring of the last 5000 logger lines, cursor + RE2 filter |
| `strigoi_run_console` | Any bound terminal command, output captured |
| `strigoi_move_player_to` | MovePlayer packet toward a world-tile target |
| `strigoi_screenshot` | PNG of the next frame; crop; inline image |
| `strigoi_dump_surface` | floor_tile: the black-floor diagnostic (spec §5.3) |

M3.3 adds the time tools (`pause/resume/step/step_world/set_seed/reseed_world/
get_state_digest`) and seed injection; M3.4 adds providers
(`list_systems/get_system_state/set_system_field`), low-level input, and
spawn/remove. Contract details: the P3 spec §4.

## Architecture in one breath

Tools never touch game memory off the game goroutine: an update queue drains
at the top of `d2app.advance`, a draw queue at the end of `render`
(screenshot/dump run there). `harness_off.go` makes every call site a no-op in
untagged builds. The log ring tees the stdlib `log` writer, which every
`d2util.Logger` writes through. The provider registry (`d2core/d2harness`)
compiles everywhere; Phase-4 systems register into it — *a system is not done
until its provider exposes what its S1 §12 assertion needs.*

## Findings log

- **Minimized / locked-session behaviour:** RECORDED AT M3.2 RUN — see below.
- (M3.2 run notes land here.)

## Determinism leak register (opens at M3.3)

Empty until the digest test exists. A leak is a bug with a name, never a flake.
