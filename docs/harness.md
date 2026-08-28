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
hand. The seven scripts: `town_walk_test.go` (live mode, §5.1),
`determinism_test.go` (the two-launch digest proof, §5.2),
`ui_inventory_test.go` (scripted input through the `ui` provider, M3.4),
`night_light_test.go` (S1 §4's night-and-light assertion, M4.1),
`night_render_test.go` (M4.1's second half: the same darkness, measured in
pixels off four screenshots) and `night_placed_test.go` (M4.1's placed
sources: a hearth nine tiles away lights its own ground and not the player's,
asserted in the model and in pixels off the same frames, then put out so the
dark closes back in) and `meters_test.go` (M4.2's survival meters: they drain
on the world clock, the fatigue and thirst thresholds flip the flags M4.5
will read, consuming moves them back the other way, and neglect runs the body
to zero health).

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

## The tools (33; harness 0.6.0)

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
| `strigoi_find_path` | The route the pathfinder would take, **without walking it**: waypoints in travel order (world tiles + subtiles), `reachable`, and `straight_line_clear`. That last field is the negative control — a route that arrives proves nothing unless the straight line did not. `from_x`/`from_y` default to the player. Added M4.3a. |
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
| `strigoi_pursue` | Put one entity on another's trail (M4.3a). The hunter paths to the quarry and re-paths when the quarry outruns `repath_tiles`; it stops within `arrive_within` and stands there, because M4.3a has no combat. Read the chases with `get_system_state pursuit`; end one with `set_system_field pursuit.release`. |
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
mechanical). The planned names the tools already know: `spawns` / `dead`
(M4.3, M4.6), `combat` (M4.5), `reputation` / `inventory` / `region` (Phase
6), `soul_pressure` (Phase 4 dashboard sim). Asking for one early returns
`NOT_IMPLEMENTED` with the milestone. A name leaves this map in the same
commit its provider registers — `meters` did at M4.2.

Registered today — **`clock`**, **`light`**, **`meters`** and **`ui`**, all
while a game screen is live.

**`clock`** (M4.1, `d2core/d2world`): `world_minutes` since the epoch,
`minute_of_day`, `time_of_day`, `date` + `year`/`month`/`day` in the Julian
calendar, `weekday`, `day_index`, `stage` (dawn/day/dusk/night), `rate` (world
minutes per simulated second — day and night differ), `moon`, `frozen`.
Settable: `frozen` (D7's hearth time-freeze; nothing but the harness sets it
until houses exist) and `moon`. **The time itself is not settable** — the
clock is stepped, never set (P3 §4.5), so `set_system_field clock.world_minutes`
fails and tells you to use `strigoi_step_world`.

**`light`** (M4.1): `radius` (what the player can see, in world tiles — the
value S1 §4's assertion is written in), `ambient`, `night_floor`,
`player_level` (`Level` on the tile the player stands on), `stage`, `moon`,
`sources` / `lit_sources`, `carried_source` / `carried_burn` / `carried_lit`,
and `source_list` sorted by id so the digest is stable — each entry carrying
`x`/`y` (where it shines FROM: a carried light reports the player's position,
not the one it was lit at) and `level_here` (`Level` on its own tile).
Settable: `carried_source` (`"torch"`, `"hearth"`, or `""` to take it away —
the give-the-player-a-light verb), `carried_burn`, `carried_lit`,
`place_source` and `remove_source`.

**`place_source`** is the put-a-fire-over-there verb, and its value is an
object rather than a scalar: `{"kind": "hearth", "x": 22, "y": 14}`, x and y
in world tiles. **`remove_source`** is its other half — value a source id from
`source_list` — and it earns its place twice over: `Light.Remove` had been an
exported method with no non-test caller since M4.1, which is the shape
unreachable wiring takes just before it rots, and a hearth is fuel-fed and
never burns down (S1 §4), so *put the fire out and the dark closes back in* is
unprovable for a placed light without it. Both are settable fields and not
tools of their own on purpose —
the three provider tools are the only ones that know about providers and do
not change as systems are added, `carried_source` was already a verb wearing a
field's clothes, and a field shows up in `strigoi_list_systems`'
`settable_fields` for free. The cost is real and worth naming: `value` is
typed `interface{}`, so the object shape has no JSON schema and a malformed
call is a runtime error rather than a schema error. The errors name the shape.

**The gap this closed (M4.1 reopened 26 Aug, closed 27 Aug):** the model had
always supported PLACED sources (`Light.Add(kind, carried, x, y)`, unit-tested
in `d2core/d2world/light_test.go`) but nothing could create one — the only
non-test caller was inside `HarnessSet("carried_source")`, which hardcodes
`carried=true` at the player, so `source_list` could never hold anything but
the player's own torch. Two rules came out of it and both are now mechanical:
**a provider that reports a collection needs a verb that can put something in
it**, and **a provider whose value is read per-position has to report it at
the positions its assertion names** — `radius` is player-centric by
construction, so without `player_level` and `level_here` a placed light was
unassertable in the model and only the pixels could see it.

The map renderer reads the same model, one tile at a time, through a
one-method interface it declares itself (`d2maprenderer.LightSampler`, which
`Level(tileX, tileY int) float64` already satisfied). So the pixels on screen
and the number the provider reports come from one source of truth, and
`d2maprenderer` still imports no world code — set no sampler and every tile
reports full daylight, which is what the engine did before M4.1.

**`meters`** (M4.2, `d2core/d2world`): the three survival meters and
everything S1 §5's assertion is written in — `food`, `water`, `fatigue`,
`activity`, the warning and death bands (`hungry` / `starving` / `thirsty` /
`parched`), the two states R2's combat rules need (`reaction_available`,
`shaken`, plus the `shaken_threshold` thirst lowers), `dying`, `dead`,
`neglect_damage`, `has_body`, and `health` / `max_health` when a body is
attached. Settable: `food`, `water`, `fatigue`, `activity`, and `consume`
(`{"kind": "food"|"water"|"rest", "amount": <points>}`).

**Direction, because it is the easiest thing here to get backwards:** Food
and Water run 100 (full) down to 0 (empty); **Fatigue runs the other way**,
0 (rested) up to 100 (exhausted), because S1 §5's thresholds are written as
"at fatigue ≥ 75%" and "at food = 0". Resting lowers fatigue.

The meters are directly settable *and* have `consume` on purpose. A script
has to be able to stand the body at a threshold without stepping a day to get
there — and separately, **a provider that reports a value needs a verb that
can move it in both directions**, or the script can only ever watch the number
fall and "eating restores Food" is untested wiring. That is the third rule in
the set the M4.1 reopening started; `consume` is also the method Phase 6's
inventory item will drive, so the game verb and the test verb are one.

`health` is reported here *and* by the player entity. The duplication is
deliberate: the meters' own assertion ("at food = 0 health decrements per hour
until death") is written in it, and both readings come from the same field, so
they cannot drift. The meters reach it through a `d2world.Body` interface the
package declares and the game screen satisfies — which is how `d2core/d2world`
still imports no entity or hero code and links zero ebiten.

**What M4.2 deliberately does not do**, per its signed build note: no meter
HUD (the meters are readable only through the harness; a HUD is wanted and
its home is undecided), no inventory or item consumption (Phase 6 — `consume`
stands in), no combat use of the flags (M4.5 reads them), and **no death
screen** (M4.6's, on S1 §6.5 and R2 §3 — ordinary deaths reload). A body at
zero health stops draining and nothing else happens, which is correct and
named rather than missing.

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

**The proof, M4.2 build, 27 Aug 2026** (`TestTownWalkDeterministic`, seed
1462, two launches): identical spawn (31,14), identical walk (east, stuck at
33.80,14.00 after exactly 150 ticks), byte-identical digests at all three
checkpoints — after load `5dc478f8b7c2…`, after the walk `8b22345a3241…`,
after 600 idle ticks `c6e9b2317361…`. The world clock, the light model and
now the meters are inside those digests, which is what keeps Phase 4
deterministic by construction rather than by retrofit.

**Those numbers moved twice on 27 Aug, and that is the rule, not an
exception.** The `systems` part of the digest hashes what each provider
reports, so it changes whenever a provider reports more: first when `light`
gained `player_level` and a per-source `level_here`, then when the `meters`
provider registered at all. (It did *not* move for `remove_source`, because
that added a settable field and no state — the digest hashes state, not
capability.) Digests are build-specific by design: the proof is that two
launches of the same build agree, never that a value matches yesterday's.
(Earlier builds, same scenario: M4.1 with placed sources `b9d8e7168236` /
`ac13cd808406` / `ec66a1ed93d1`; M4.1 before them `22b54a6ff64c` /
`a9ada311d3f1` / `827268ad303d`; M3.4 `bb00c5aef522` / `c10fd2968b04` /
`ef0c2a80f237`; M3.3 `b0cae4bd` / `79688e9f` / `461e72a5` on the leaner
digest.)

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

**M4.2, survival meters (27 Aug 2026)**

- **Reading the engine before building found a scope failure worth a whole
  burst.** S1 §5's signed playtest assertion is three clauses, and at the
  time only ONE was buildable: there is no combat state machine until M4.5,
  nothing in the tree wrote `Stats.Health` (a grep for assignments returned
  zero), there was no player death path or death screen, and no
  item-consumption verb existed for "replenished by items". That is the M4.1
  trap one milestone early — build the meters, assert clause 1, and the
  milestone looks done with two thirds of a signed sentence unbuilt. The
  split was written into the build note, signed, and is now in the
  milestone's DoD instead of waiting to be discovered from a screenshot.
- **A script that walks to nightfall must top the body up before measuring a
  drain.** The first run of `meters_test.go` failed at its own setup: getting
  to a deep night costs ~43 world hours, and at the shipped dials that empties
  every meter twice over — water goes in 22 hours, food in 33. The dials are
  working (S1 §5 wants eating and drinking to be a daily cost); the script was
  measuring a corpse. **M4.3 will hit this too** — anything that steps to a
  night and then asserts a rate needs a known body first, which is what the
  settable meters are for.
- **The measurement is taken against the clock's own elapsed minutes, not the
  hours the script asked for.** `strigoi_step_world` overshoots its target by
  a fraction of a tick, so a script that assumes it got exactly four hours is
  wrong by a hair every time. Read `world_minutes` before and after and divide.
  Measured, seed 1462, over 4.01 night hours: food 87.98, water 81.98, fatigue
  14.02 — each on its dial. The Reaction goes at 75 fatigue, Shaken at 90, and
  at 80 when thirsty. Neglect took 166 → 156 health over 2.54 starving,
  parched hours and ran the body to zero, where the meters stop.
- **An assertion that pins the ABSENCE of a system has to move as milestones
  land.** `ui_inventory_test.go` checked that `meters` answered
  `NOT_IMPLEMENTED` naming M4.2; it failed the moment M4.2 shipped, which is
  the assertion doing its job rather than a regression. It now asks about
  `spawns` (M4.3) and separately asserts that `meters` IS registered. Next in
  the queue: `dead` (M4.3 / M4.6), then `combat` (M4.5).

**M4.1, placed sources (27 Aug 2026) — the reopening closed**

- **The intermittent map-load crash has a SECOND face, and this one names a
  one-line defect.** One full-suite run in two today died in 2.5 s during
  `strigoi_start_game` — not the filed `d2dt1.DecodeTileGfxData` index panic
  but `runtime error: invalid memory address or nil pointer dereference` in
  `d2mapengine.(*MapEngine).addDT1` (`engine.go:107`), from `ResetMap`
  (`:86`) by way of `game_server.go:108`. Upstream of every line this burst
  touched; the re-run passed all six scripts. The immediate cause is visible
  in five lines: `addDT1` calls `m.asset.LoadDT1(fileName)`, and on error
  calls `m.Error(err.Error())` **and then falls through** to
  `append(m.dt1TileData, dt1.Tiles...)` — there is no `return`, so a failed
  load dereferences nil and takes the process down instead of leaving one
  tileset missing. It sits directly downstream of the asset-cache defect
  already on file: `LoadDS1` reads AND writes `am.dt1s`, the DT1 cache
  (`asset_manager.go:511`, `:526`), so DS1 entries evict DT1s, `LoadDT1`
  re-decodes under pressure, and its `dt1Value.(*d2dt1.DT1)` assertion at
  `:489` is unchecked. **Filed, not fixed.** For a session that hits it: a
  playtest run that dies in ~3 s with a game-output tail is one of these two;
  re-run before investigating.
- **And the one-liner is NOT a free win — this is the stop sign.** The missing
  `return` above reads like a fifteen-minute fix. It is not, for two reasons,
  and Josh parked it on both (27 Aug). **First, it cannot be tested:**
  `MapEngine.asset` is a concrete `*d2asset.AssetManager` (`engine.go:26`,
  `CreateMapEngine` at `:52`), not an interface, so there is no seam to inject
  a loader that fails — proving the error path needs either a small extracted
  interface or a real `AssetManager` pointed at the MPQs, and Article V bars
  the second from CI. One line of code, but a behaviour change with no test,
  against Constitution VI.1. (`d2mapengine` links **0** ebiten, so tests there
  would otherwise be headless-safe; that is not the obstacle.) **Second, it
  makes the game quiet rather than correct:** a hard crash becomes a region
  with wrong-looking walls and nothing that flags it — and with the black
  floor still parked, trading a loud intermittent failure for a silent one
  costs diagnostic signal in a project whose main instrument is screenshots
  measured in ratios. It goes into the **DT1 milestone** with the decoder
  bounds-guard, the two `tile_cache.go` bugs and the asset-cache defect, where
  the interface extraction is worth doing once for the whole set.
- **The assertion that would have caught it, written as the negative control
  it deserves.** Before `night_placed_test.go` landed, `place_source` was
  reverted to the old shape (`carried=true` at the player) and the script run
  against it: it fails at the first model assertion and dumps `carried:true`,
  `x:31 y:14`, `player_level:1`, `radius:8` — the pathology itself. With the
  verb correct: the hearth's own tile at level 1.000, the player's at 0.125
  (the night floor), radius still 1.5, and on screen the hearth's ground
  ×6.64 against the unlit night while the player's stays at ×1.00 and the
  ground beyond the radius at ×1.00. The same instrument on a *carried* torch
  puts the player's own ground at ×8.59, which is what makes ×1.00 evidence
  rather than a shrug.
- **The dials, not the wish, set the test's geometry.** Josh's assertion was
  "a hearth three tiles east". It cannot be written: the hearth's radius is 8
  and its falloff does not start until 4.8, so a hearth three tiles away lights
  the player *fully* — in the model as well as the pixels — and a placed
  source that engulfs the player is indistinguishable from a carried one.
  Nine tiles is the first whole tile past the radius. It is also past what the
  camera can hold: one tile is (80, 40) screen pixels and 5 tiles already
  spans the viewport, so **no placement exists that puts a source's own tile
  on screen while leaving the player's ground at the night floor**. Hence the
  split — the model asserts the hearth's own tile (the camera cannot see it),
  the pixels assert that the light on screen is centred away from the player.
  Worth knowing before M4.3 writes anything else that wants a light and a dark
  place in one frame.
- **An exported method with no non-test caller is the same defect one step
  earlier.** `Light.Remove` had been exactly that since M4.1 — the shape
  `Light.Add`'s placed path was in, one week before it was found. `remove_source`
  (Josh's call, same day) makes it reachable and buys the act a fuel-fed hearth
  cannot otherwise perform: the fire goes out and the hearth's ground returns
  to the plain night, measured at 23.1 → 3.5 of 255 against an unlit baseline
  of 3.5. The carried torch proves the same thing by burning down; a hearth
  never burns down (S1 §4), so without the verb there was no way to show that
  the light stops when the source does.
- **Two provider rules, now mechanical.** A provider that reports a collection
  needs a verb that can put something in it — and, the corollary that cost a
  milestone, one that can take it back out. And a provider whose value is
  read *per position* has to report it at the positions its assertion names —
  `radius` is player-centric by construction, so a light the player stands
  outside of moved nothing the provider reported, and the placed source was
  unassertable in the model until `player_level` and `level_here` existed.

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
  **Closed 27 Aug by `place_source` and `night_placed_test.go`.** Still out of
  scope and unscoped: lighting the *map's own* fires, which is content work
  against E6 (which D2 object ids count as fire) and needs Josh first.

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
