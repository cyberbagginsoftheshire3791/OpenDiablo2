# The playtest harness (Phase 3) — v1, M3.4 (+ M4.1's night and light)

An MCP (Model Context Protocol) server compiled into the game behind the
`harness` build tag, so an agent (Claude Code) or a Go test can start a game,
freeze and step the simulation, read any entity or system, inject input, spawn
things, take screenshots, and get the same answer every time it runs the same
script with the same seed. Designed in `P3 - Playtest Harness Design Spec.md`
(Video Game folder, SIGNED 24 Aug 2026); this file is the operator's card and
the living tool reference. Phase 3 closed with M3.4 on 26 Aug 2026.

## Build and run

    go build -tags harness -o od2-harness.exe .   # the debug build (release builds contain none of this)
    ./od2-harness.exe -harness                    # serve MCP on http://127.0.0.1:6670/mcp
    ./od2-harness.exe -harness -harness-addr 127.0.0.1:6670 -harness-out D:\runs

Loopback only — a non-loopback `-harness-addr` is refused. No authentication:
single user, single machine, opt-in flag. The harness build without `-harness`
plays exactly like the release build (the input overlay is a pass-through).

**Attach Claude Code:** the repo's `.mcp.json` declares the server; with the
game running, Claude Code sessions in this repo see the `strigoi_*` tools.
Manual: `claude mcp add --transport http strigoi http://127.0.0.1:6670/mcp`.

**Playtest scripts** (laptop only, never CI — no MPQs there):

    go test -tags playtest ./playtest/... -v -count=1

Scripts build + launch the harness binary themselves and keep the game's own
stdout/stderr in `<Projects>\strigoi-harness-runs\game-<stamp>.log` (a script
that dies with a transport error prints the tail — the game's last words).
Set `STRIGOI_HARNESS_ADDR=127.0.0.1:6670` to attach to a game you started by
hand. The five scripts: `town_walk_test.go` (live mode, §5.1),
`determinism_test.go` (the two-launch digest proof, §5.2),
`ui_inventory_test.go` (scripted input through the `ui` provider, M3.4),
`night_light_test.go` (S1 §4's night-and-light assertion, M4.1) and
`night_render_test.go` (M4.1's second half: the same darkness, measured in
pixels off four screenshots).

## Outputs (Article V)

Everything the harness writes — screenshots, surface dumps, `run.json` — goes
to `%LOCALAPPDATA%\Strigoi\harness\runs\<stamp>\` by default, or `-harness-out`.
The playtest launcher uses `<Projects>\strigoi-harness-runs\` (outside the
repo, reachable by the device bridge). Screenshots render Blizzard art: they
never enter the repo (`/harness-runs/` is gitignored and hook-blocked as
belt-and-braces), never go on public issues, never ship. Run directories and
game logs are the user's to delete; the harness prunes nothing.

## The recipe

1. `strigoi_pause` — freeze the clock BEFORE the world exists.
2. `strigoi_start_game{hero_name, hero_class, seed}` — the world is born
   frozen; the call returns only after the game screen has run its first frame
   with the player in it (first-frame initialisations done; see the register).
3. Read (`get_player`, `get_entities`, `get_system_state`), act
   (`move_player_to{wait}`, `key`, `click`, `spawn_entity`), and advance only
   with `step` / `step_world`.
4. `strigoi_get_state_digest` at every checkpoint; compare across launches.

Coordinates are **world tiles** (`Position.World()`) everywhere except the
low-level input tools, which take **screen pixels** (800×600). The local
player is always handle `p:1`; other entities are `e:N` in first-seen order
(stable under a seed, per process). Errors come back as `CODE: message — hint`
with codes `NOT_IN_GAME · ALREADY_IN_GAME · SAVE_NOT_FOUND · TIMEOUT_LOADING ·
GAME_NOT_TICKING · NOT_IMPLEMENTED · UNKNOWN_HANDLE · UNKNOWN_SYSTEM ·
FIELD_NOT_SETTABLE · OUT_OF_BOUNDS · BAD_ARGUMENT · INTERNAL`.

## The tools (33; harness 0.5.0)

### Session (M3.2)

| Tool | What |
|---|---|
| `strigoi_ping` | Liveness, commit, harness version, mode, tick, uptime |
| `strigoi_get_game_info` | Screen hint, loading, hero, seed, tick, entity count, registered systems |
| `strigoi_navigate` | main_menu · character_select · select_hero · credits |
| `strigoi_start_game` | Load a save or create a hero (`seed` pins map, world RNG, entity IDs); returns after the first game frame |
| `strigoi_save_game` | Write the `.od2` |
| `strigoi_quit` | Manifest + exit (confirm: true) |

### Time and determinism (M3.3)

| Tool | What |
|---|---|
| `strigoi_get_time_mode` | live or paused; dt; stepped sim_seconds |
| `strigoi_pause` / `strigoi_resume` | Freeze / release the simulation clock (frames still render and poll input) |
| `strigoi_step` | Advance exactly N fixed-dt ticks (600/frame batches); returns the digest |
| `strigoi_step_world` | Advance by sim seconds, or by `world_minutes` — steps until the world clock reports the target (M4.1) |
| `strigoi_set_seed` | One-shot seed for the next start_game |
| `strigoi_reseed_world` | Reseed the world RNG mid-game (repeated-roll tests) |
| `strigoi_get_state_digest` | Per-part SHA-256: sim · world · entities · rng · systems |

### Observation (M3.2, M3.4)

| Tool | What |
|---|---|
| `strigoi_get_entities` | Paginated entity list; kind/near filters |
| `strigoi_get_entity` / `strigoi_get_player` | One entity with its kind-specific state (`HarnessState`), plus its `screen` pixel position for aiming clicks |
| `strigoi_get_tile` | Region, level name, 5×5 walkability |
| `strigoi_dump_map` | walk / entities / region window, ≤64×64 tiles |
| `strigoi_read_log` | Ring of the last 5000 logger lines, cursor + RE2 filter |
| `strigoi_screenshot` | PNG of the next frame; crop; inline image |
| `strigoi_dump_surface` | floor_tile: the black-floor diagnostic (spec §5.3) |
| `strigoi_list_systems` | Registered providers with their settable fields; the planned systems not yet registered |
| `strigoi_get_system_state` | A provider's state as the system exposes it; `NOT_IMPLEMENTED` names the milestone that adds a planned one |

### Action (M3.2, M3.3, M3.4)

| Tool | What |
|---|---|
| `strigoi_run_console` | Any bound terminal command, output captured |
| `strigoi_move_player_to` | MovePlayer packet toward a world-tile target; `wait`/`max_ticks` step until arrived / stuck / timeout |
| `strigoi_set_system_field` | Write one allow-listed provider field (test setup); `FIELD_NOT_SETTABLE` otherwise |
| `strigoi_spawn_entity` | npc (monstats Id) · item (item codes) · object (objects.txt index or name) at a world tile, through the engine's own factory; returns a handle |
| `strigoi_remove_entity` | Remove by handle (never a player) |
| `strigoi_key` | tap · down · up, by name (`i`, `escape`, `f5`, `kp7`, `graveaccent`, …) |
| `strigoi_click` | left/right/middle at screen pixels, optional shift/control/alt; walks the player like a real click |
| `strigoi_move_cursor` | Place the scripted cursor (holds until the real mouse moves) |
| `strigoi_type_text` | Printable characters on one input poll |

## Providers — how Phase 4 becomes observable for free

`d2core/d2harness` compiles in every build. A system registers a `Provider`
(`HarnessName`, `HarnessState`) at construction; optionally `Settable`
(`HarnessSet`, an explicit allow-list) and `FieldLister`. Entities implement
the entity half, `Stateful` (`HarnessState` only) — `Player` and `NPC` do
today. `list_systems`, `get_system_state`, and `set_system_field` are the only
tools that know about providers, and they never change as systems are added;
the digest hashes every provider's state, so "observable" and "deterministic"
are one checklist.

**The rule, from Phase 4 on: a system is not done until its provider exposes
every value its S1 §12 playtest assertion needs** (Constitution VI.2, made
mechanical). The planned names the tools already know: `meters` (M4.2),
`spawns` / `dead` (M4.3, M4.6), `combat` (M4.5), `reputation` / `inventory` /
`region` (Phase 6), `soul_pressure` (Phase 4 dashboard sim). Asking for one
early returns `NOT_IMPLEMENTED` with the milestone.

Registered today — **`clock`**, **`light`** and **`ui`**, all while a game
screen is live.

**`clock`** (M4.1, `d2core/d2world`): `world_minutes` since the epoch,
`minute_of_day`, `time_of_day`, `date` + `year`/`month`/`day` in the Julian
calendar, `weekday`, `day_index`, `stage` (dawn/day/dusk/night), `rate` (world
minutes per simulated second — day and night differ), `moon`, `frozen`.
Settable: `frozen` (D7's hearth time-freeze; nothing but the harness sets it
until houses exist) and `moon`. **The time itself is not settable** — the
clock is stepped, never set (P3 §4.5), so `set_system_field clock.world_minutes`
fails and tells you to use `strigoi_step_world`.

**`light`** (M4.1): `radius` (what the player can see, in world tiles — the
value S1 §4's assertion is written in), `ambient`, `night_floor`, `stage`,
`moon`, `sources` / `lit_sources`, `carried_source` / `carried_burn` /
`carried_lit`, and `source_list` sorted by id so the digest is stable.
Settable: `carried_source` (`"torch"`, `"hearth"`, or `""` to take it away —
this is the harness's give-the-player-a-light verb), `carried_burn`,
`carried_lit`.

**Known gap, M4.1 reopened 26 Aug:** the model supports PLACED sources
(`Light.Add(kind, carried, x, y)`, unit-tested in `d2core/d2world/light_test.go`)
but nothing can create one. The only non-test caller is inside
`HarnessSet("carried_source")`, which hardcodes `carried=true` at the player, so
`source_list` can never hold anything but the player's own torch and the map's
fires stay dark at night. S1 §4's design says carried *and placed*. The next
burst adds the place-light verb and a script assertion for it.

The map renderer reads the same model, one tile at a time, through a
one-method interface it declares itself (`d2maprenderer.LightSampler`, which
`Level(tileX, tileY int) float64` already satisfied). So the pixels on screen
and the number the provider reports come from one source of truth, and
`d2maprenderer` still imports no world code — set no sampler and every tile
reports full daylight, which is what the engine did before M4.1.

**`ui`** — the game controls:
`inventory_open`, `skilltree_open`, `hero_stats_open`, `quest_log_open`,
`party_open`, `help_open`, `escape_menu_open`, `skill_select_open`,
`left_panel_open`, `right_panel_open`, `free_cam`, `clock` (the controls' own
accumulated seconds — not the world clock). Read-only. It registers in
`bindGameControls` and unregisters in `Game.OnUnload`; `clock` and `light`
register when the game screen is constructed and close on unload.

Entity state today — `Player`: name, class, act, gold, level, experience,
health/mana/stamina with maxima, the four attributes, in_town, running,
run_toggled, casting, animation_mode, direction, path_len, speed, left/right
skill ids. `NPC`: name, monstat (+ id), has_paths, paths, path_index, action,
repetitions, done, animation_mode, direction, path_len, speed.

## Input — two layers

High-level, preferred: `move_player_to`, `run_console`, and the game verbs
Phase 4 adds. Low-level: a `d2input.ScriptedInputService` overlay wraps the
real ebiten service (`NewInputManagerWithService`); a scripted press is "just
pressed" for exactly one input poll and "pressed" until released, a tap
releases on the next poll, and everything merges with the real keyboard and
mouse so a human is never locked out. Each input tool queues its action onto
the game goroutine and returns only after the frame that polled it, so the
very next tool call sees the effect. The game controls keep their own clock
(accumulated from the frame deltas) for click-repeat timing instead of the
wall clock, so scripted clicks replay identically under the stepped clock.

## Determinism

The contract (spec §3.3): same script + same seed + same build → the same
state digest at every checkpoint, across separate process launches. Scoped to
the simulation; pixels, animation frames, audio, logs, and raw frame ticks are
excluded by design. Digests are build-specific: a change to what entities or
providers expose changes the numbers (they did between M3.3 and M3.4) — the
proof is that two launches of the same build agree, not the value.

**The proof, M4.1 build, 26 Aug 2026** (`TestTownWalkDeterministic`, seed
1462, two launches): identical spawn (31,14), identical walk (east, stuck at
33.80,14.00 after exactly 150 ticks), byte-identical digests at all three
checkpoints — after load `22b54a6ff64c…`, after the walk `a9ada311d3f1…`,
after 600 idle ticks `827268ad303d…`. The world clock and the light model are
inside those digests, which is what keeps M4.1 deterministic. (Earlier
builds, same scenario: M3.4 `bb00c5aef522` / `c10fd2968b04` / `ef0c2a80f237`;
M3.3 `b0cae4bd` / `79688e9f` / `461e72a5` on the leaner digest.)

**The renderer half did not move them.** Re-proved on `ebcc01d3` after the
map renderer began dimming every tile: the same three digests, byte for byte.
That is the digest's scope working as designed — brightness is presentation,
and presentation is excluded (spec §3.6). A renderer change that HAD moved a
digest would have been a leak worth a register entry.

Notes: `sim_seconds` accumulates float error in display (2.4999…96 for 150
ticks at 1/60) — deterministic, identical across runs, harmless. Entity
handles (`e:N`) are per-process; digests compare across fresh launches, not
across two games inside one process. An unseeded `start_game` restores
crypto-random entity IDs and the wall-clock map seed.

## Determinism leak register (opened 26 Aug 2026)

| # | Found | Part(s) | What | Status |
|---|---|---|---|---|
| 1 | 26 Aug 2026, M3.4, the first run of the richer digest | `entities`, `systems` | **First-frame initialisation.** The state right after `start_game` depended on how many zero-delta frames had run before the digest: the player's `animation_mode` is set by its first `Advance`, and the `ui` provider registers at the end of the first game frame that finds the player. Checkpoints B and C (after stepping) matched; only "after load" diverged. Not an RNG leak — a readiness gap in the harness. | **CLOSED the same day**: `start_game` now returns only after the controls are bound and one further full frame has run. Proof re-run: all three checkpoints match. |

A future mismatch names its part (sim / world / entities / rng / systems) in
the test output; record it here as a bug with a name, never a flake to retry.

Known wall-clock sites that are *not* leaks because they never reach the
digest: `HideZoneChangeTextAfter` (a `time.AfterFunc` on the zone-change
banner), cursor/label blink, packet timestamps (spec §2.2).

## Architecture in one breath

Tools never touch game memory off the game goroutine: an update queue drains
at the top of `d2app.advance`, a draw queue at the end of `render`
(screenshot/dump run there). `harness_off.go` makes every call site a no-op in
untagged builds (the input overlay hands the real service straight back). The
log ring tees the stdlib `log` writer, which every `d2util.Logger` writes
through. Files: `d2app/harness.go` (state, queues, ring, handles, run dir),
`harness_time.go` (pause/step/seed/digest), `harness_tools.go` (server,
session, actions), `harness_obs.go` (observation), `harness_providers.go`,
`harness_input.go`, `harness_spawn.go`; `d2core/d2harness` (registry);
`d2core/d2input/scripted.go` + `key_names.go` (the E6 overlay); accessors in
`d2game/d2gamescreen`, `d2core/d2screen`, `d2core/d2term`,
`d2core/d2map/d2maprenderer`.

## Findings log

**M4.1, the renderer half (26 Aug 2026, night)**

- **The screenshot found the hole the tests could not: placed light sources
  were shipped unreachable.** Every assertion in both night scripts lights a
  CARRIED torch, because that is the only source the game can make — so the
  scripts agreed with each other and with the model, and all of them missed
  that the camp's own campfire and wall torches stay dark. The unit tests DO
  cover placed sources; the wiring to reach them does not exist. The lesson is
  narrower than "look at the screen": a provider that reports a collection
  (`source_list`, with per-source `x`, `y`, `carried`) needs a verb that can
  put something in it, or the reporting is decoration. M4.1 reopened.

- **Night reads as night, and the numbers say so without a monitor.** Deep
  night at a new moon is **88% dimmer** than the same frame at noon (play-area
  mean 30.1 → 3.5 of 255) while the world is still drawn, not black. The
  unlit night dims **uniformly** — the tiles around the player fall to ×0.116
  of their daylight value, the tiles beyond a torch's reach to ×0.118 — which
  is the measurement that separates real per-tile light from a vignette
  painted over the frame. A lit torch then breaks that uniformity in exactly
  one place: near ×8.59, far ×1.63.
- **The lit region has visible tile edges.** Brightness is per tile (the
  signed shape, ask 2), a tile is an 80×40 px diamond, and a 5-tile torch
  whose falloff starts at 60% of its radius has about two tiles of gradient to
  work with — so the boundary steps rather than fades, and the mid-zone shows
  a faint diamond moiré between the 16 quantisation steps and the tile grid.
  It is the cost of the chosen shape, not a defect in it. Cheapest softening
  if it ever matters: drop `FalloffStart` so the gradient spans more tiles.
  **Josh's call, deliberately not taken here** — readability is D5's question.
- **The "far" bucket is thin, and tall sprites leak into it.** A 5-tile radius
  covers nearly the whole 800×600 viewport, so only ~335 of 86,000 sampled
  pixels sit beyond 5.5 tiles. Those far pixels still brightened ×1.63 with
  the torch, because a wall or tent sprite anchored on a *lit* tile paints
  pixels far up the screen. Pixel distance is not tile distance; the script
  says so where it matters.
- **A new intermittent engine bug, upstream of everything M4.1 touched.** One
  full-suite run in four panicked during map load: `index out of range [1152]
  with length 1152` in `d2dt1.DecodeTileGfxData`, from `generateWallCache`
  (`tile_cache.go:214`) inside `CreateMapRenderer` — i.e. before a single line
  of light code runs. Both town_walk scripts died with it; re-running each
  alone passed, the clean pre-M4.1 head passed a full suite, and the M4.1 head
  passed two more. Filed as observed, not fixed.
- **Read afterwards, and it changes the diagnosis: the index EQUALS the
  length**, which is the signature of a cursor walking off the end of the
  SOURCE buffer, not of a 2D offset landing on the end of the destination.
  `d2dt1.go:235` fills `block.EncodedData` with exactly `block.Length` bytes,
  and the RLE loop advances `idx` in lockstep with `length`; a final run whose
  `b2` claims more pixels than remain reads one past the end. **Every access in
  `DecodeTileGfxData` is unchecked** — the `EncodedData[idx]` reads and the
  `(*pixels)[offset]` writes, in both branches — so a bounds guard there
  (a pure `d2common` function, unit-testable with synthesized bytes, no
  ebiten) turns a dead process into one mis-drawn wall. Two further real bugs
  sit beside it: the wall pixel buffer is sized from `target.Blocks` where
  `target` is whichever of two tiles is SHORTER, and then BOTH are decoded
  into it (`tile_cache.go:178-218`); and `newTileData = &newTileOptions[
  tile.RandomIndex]` (`:174`) indexes the left-part options with an index
  chosen against the right-part options, unchecked and un-nil-checked. For the
  intermittency itself the first question is whether the map engine's seed is
  applied before `generateTileCache` runs — `RandomIndex` comes from
  `getRandomTile(options, x, y, me.seed)` and is deterministic if it is. If it
  is, the next suspect is the asset cache: `LoadDS1` reads AND writes
  `am.dt1s`, the DT1 cache (`asset_manager.go:511`, `:526`), so DS1s evict
  DT1s under pressure and `LoadDT1`'s `dt1Value.(*d2dt1.DT1)` assertion is
  unchecked.
- **`gate.ps1` needs `-ExecutionPolicy Bypass`.** Without it PowerShell
  refuses the script and `Start-Process` fails *silently* — the log never
  appears and it looks like the gate is still running. Both reusable scripts
  are affected.

**M4.1, the model half (26 Aug 2026, evening)**

- **The world clock is a harness citizen from its first line.** `clock` and
  `light` (`d2core/d2world`) register as providers at construction, so
  `step_world{world_minutes}` and the S1 §4 assertion worked the day the
  systems existed rather than needing a retrofit. The Phase 3 provider
  contract paid for itself immediately.
- **`Level()` is quantised, `Ambient()` and `Radius()` are not.** The first
  M4.1 test failure was the test's fault: it compared a 16-step quantised
  tile level (0.125) against the continuous ambient (0.10). The rule is now
  in the code comment — the renderer sees bands, the dials stay exact.
- **A stale `.git/index.lock`** left by a device-VM git call (which cannot
  unlink it — "Operation not permitted" on the mounted filesystem) silently
  made every `git add` fail while builds and tests still passed. If staging
  seems to do nothing, look for the lock.

**M3.4 (26 Aug 2026)**

- **The main menu never unbound its input handler** (issue #792 was fixed for
  `CharacterSelect` and `MapEngineTest`, not `MainMenu`), so the unloaded
  menu kept hearing keys in-game. The UI script's second Escape reached the
  stale menu's "exit on Escape" branch (`MainMenu.OnKeyUp` in main-menu mode)
  and the process exited with code 0 and no message. Humans dodged it only
  because clicking Single Player leaves the menu in `Unknown` mode. Fixed:
  `MainMenu.OnUnload` unbinds like the other screens.
- **A loop error ended the process in silence** — `main.go` discarded
  `Run()`'s error. It now logs the cause and exits 1.
- **A scripted click walks exactly as the math says**: 120 px right and 60 px
  down of the player is 1.5 tiles east, and the player arrived at +1.50, 0.00.
- Two new test packages (`d2input`, `d2player`); 31 in all.

**M3.2 first runs (24 Aug 2026)**

- **The black town floor is INTERMITTENT per process launch — and the harness
  caught it.** Four launches the same evening: two rendered the full scene,
  two rendered sprite+HUD on black (runs `20260824-222411` healthy vs
  `20260824-222604` and `20260824-222748` black, in
  `<Projects>\strigoi-harness-runs\`). In the black runs `strigoi_dump_surface`
  shows the cached floor tiles FULLY COLORED (6400/6400 opaque, ~6k non-black):
  the content exists and `Screenshot()`/ReadPixels sees it, but compositing
  draws none of it, while `NewImageFromImage` sprites (player, HUD) draw fine.
  So the 22 Aug diagnosis narrows to: per-launch-intermittent, content present,
  `DrawImage` of `NewSurface`+`ReplacePixels` surfaces yields nothing.
  Suspicion for the eventual fix (NOT Phase 3 work): those surfaces are built
  on the Game screen's OnLoad goroutine (the screen manager loads screens off
  the main goroutine) — an off-main-goroutine image upload race in ebiten
  would be launch-intermittent exactly like this. `town_walk_test.go` records
  a floor-luminance observation every run (~1% lit = black, ~50%+ = healthy;
  26 Aug's runs: 98% and 96%).
- **Minimized window: the loop keeps ticking** (~63 ticks/s over a 5 s
  minimize; `TestMinimizedTick`, opt-in via `STRIGOI_TEST_MINIMIZED=1`).
  Locked-session behaviour still UNKNOWN (needs a deliberate lock test).
- **Claude Code attach works end-to-end**: with the game running `-harness`,
  a headless `claude -p` session in the repo used the `.mcp.json` server
  (with `enableAllProjectMcpServers` in `.claude/settings.json`) to ping,
  create a hero, read state, screenshot, and quit the game.
- **Known benign log lines during normal play**: `[UI Manager][ERROR] Error
  while setting frame (N): invalid frame index` (HUD frame setting;
  pre-existing). The scripts allowlist exactly this.
- **`strigoi_ping` reports commit "build"** when the binary is built without
  the ldflags injection (`go build` plain, as the playtest launcher does) —
  the flag-injected build stamps a real commit.
