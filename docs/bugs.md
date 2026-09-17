# Bug database and launch ledger

The error log Constitution I.3 and Plan §7 require, and the home Bethke's *"bugs
without owners simply will not be addressed"* asks for. Every known defect gets a
row: **id · first seen · build · reproduction · rate · owner · status.** A bug
without an owner is not logged. New defects are added here the day they are found,
not at some future audit.

Seeded 14 September 2026 by the hardening burst with the five defects that were
living as prose under "Parked" in `state.md`. The three map-load faces are parked
together on Josh's ruling (28 Aug): `MapEngine.asset` is a concrete type, so there
is no seam to inject a failing loader, and a nil guard would make the game *quiet
rather than correct* — **do not "just add the return"**; they go into the DT1
milestone with the loader-interface extraction that makes them testable. All are
intermittent: **re-run before investigating.**

Added 17 September 2026 (Cowork): **BUG-8 and BUG-9**, the two sight defects. An
independent Codex audit of `72bc622c` found the `checkLos` endpoint overshoot; a
verification pass reproduced it with a line-for-line port and measured its incidence
over the signed notice radius. BUG-9 was visible in the d2-formats reference as an
as-found observation and had never been given a row. **Neither has an owner yet.** The rule
above (*"a bug without an owner is not logged"*) is served by naming the decision
rather than the fix: **Josh names the owner** -- as he already does for BUG-4 and
BUG-5, which stand unowned and PARKED. Until he does, these two are logged so they
cannot be lost, not assigned so they look handled. **Neither is fixed here** --
neither is in c-2's signed scope.

## Defects

| id | first seen | build | reproduction | rate | owner | status |
|---|---|---|---|---|---|---|
| BUG-1 | pre-fork (upstream) | any | `MapEngine.addDT1` logs a failed `LoadDT1` then dereferences `dt1.Tiles` — nil panic on a DT1 that will not load (`d2core/d2map/d2mapengine/engine.go:102-107`) | intermittent (map load) | the DT1 milestone | PARKED |
| BUG-2 | pre-fork (upstream) | any | `d2dt1.DecodeTileGfxData` indexes `(*pixels)[offset]` with no bound — index-out-of-range on malformed tile gfx (`d2common/d2fileformats/d2dt1/gfx_decode.go:30,60`) | intermittent (map load) | the DT1 milestone | PARKED |
| BUG-3 | pre-fork (upstream) | any | `MainMenu.createLogos` logs then dereferences a nil logo (`d2game/d2gamescreen/main_menu.go:325-331`) | intermittent (startup) | the DT1 milestone | PARKED |
| BUG-4 | ~Aug 2026 | any | the black floor: cached floor tiles are provably fully coloured while the composite draws nothing. A NIGHT screenshot is useless as evidence — the night scripts sample daylight control frames for exactly this reason | unmeasured, per launch | unassigned | PARKED |
| BUG-6 | 2026-09-15 | M4.4c-1 | a squad model that dies keeps its squad's overhead bar: `Squads.Bars` lists every member entity with `entity != ""` regardless of `health` (`d2core/d2world/squads.go`), and the corpse guard added to `Game.OverheadBars` only covers the ENEMY loop. **It cannot happen in friends build #1** -- `s:1`'s one model is the player, and a dead player is the death screen -- so the fix waits for the milestone that ships a second squad in a shipped build | not reproducible in build #1 (needs N>1) | M4.4c-2 | OPEN |
| BUG-7 | 2026-09-15 | M4.4c-1 | **no playtest can exercise a HELD mouse button.** `strigoi_click` presses and releases inside one frame (`d2app/harness_input.go`), so `repeatDue(now, now)` is false by construction and `GameControls.OnMouseButtonRepeat` is unreachable from any script. The squad guard added there on 15 Sep is verified by READING only. A harness verb that holds a button for N frames would close it | always (instrument gap) | M4.4c-2 | OPEN |
| BUG-5 | ~Aug 2026 | any | the startup / map-load crash — presents as one of BUG-1/2/3. Intermittent; **re-run before investigating**. Every playtest launch is counted in the ledger below so the rate can be measured before friends build #1 (9 Sep G9) | intermittent | unassigned | PARKED |
| BUG-8 | 2026-09-17 | `72bc622c` | **line of sight samples one subtile PAST its destination.** `MapEngine.checkLos` loops `for i := 0; i <= int(N); i++` and increments the coordinates BEFORE sampling (`d2core/d2map/d2mapengine/pathfind.go:124-136`), so it takes `int(N)+1` steps along a segment `N` long -- a full extra subtile whenever `N` is whole, which is every axis-aligned and 45 deg ray between grid-aligned entities. A ray (2,2)->(4,2) samples (3,2), (4,2), **(5,2)**: a blocker beyond the target blocks it, while the reverse ray is clear, and a legal map-edge endpoint is rejected because `SubTileAt` returns nil off-map and `:134` treats nil as a wall. **Reachable in live gameplay, not just pathfinding:** `d2game/d2gamescreen/game.go:839-850` (`mapSight.Clear`) is the only `Sight` implementation and `d2core/d2world/notice.go:341` gates awareness on it **one-directionally**, so the forward/reverse asymmetry never averages out -- cover works or does not depending on which side of the player the watcher stands. It also contaminates evidence: `d2app/harness_obs.go:559` uses it for `StraightLineClear`, the negative control that proves a route was a genuine detour. **The fix is NOT simply `<=` -> `<`:** with `<` the loop takes `int(N)` steps, which lands exactly on the endpoint only when `N` is WHOLE -- for a FRACTIONAL `N` (any ray between entities that are not grid-aligned) the last sample falls short and the destination cell is never examined, so a watcher sees through a blocker standing on the player. It trades this false-negative for a false-positive on a different set of rays. Use `ceil(N)` steps with the final step clamped to the endpoint (`t := min(float64(i), N)`), keep not sampling the start cell, and assert WHICH cells the ray samples plus forward/reverse symmetry -- no existing test does (`accessors_test.go:118-146` asserts only `NotPanics` and loose bounds, and its one clear case passes by luck; `d2world/notice_test.go:40-45` uses a `fakeSight` and never runs the real cast). **Re-measure real-map awareness before retuning spawn aggression -- failed awareness resembles a pacing problem.** | **57.4%** of 200,000 simulated awareness rays inside the signed 12-tile radius inspect at least one subtile the segment never reaches (port; live-map incidence unmeasured) | unassigned -- **Josh to name**; recommended before friends build #1 | OPEN |
| BUG-9 | 2026-09-17 | `72bc622c` (pre-fork behaviour) | **the sight test consults the WALK flag.** `checkLos` blocks on `flags.BlockWalk` (`pathfind.go:134`); `BlockLOS` is decoded and never read anywhere in the engine, recorded verbatim at `.claude/skills/d2-formats/references/maps.md:184` (*"Only `BlockWalk` is consulted anywhere in the engine; `BlockLOS`, `BlockJump`, `BlockPlayerWalk` and `BlockLight` are decoded and never read"*); `docs/architecture-as-found.md:140` states only the narrower *"walkability is one bit"*. **Re-verified 17 Sep:** `git grep -n BlockLOS -- '*.go'` returns only `d2common/d2fileformats/d2dt1/subtile.go` and its unit test. Anything walkable-but-opaque, or blocking-but-see-through, is therefore wrong for sight. **Distinct from BUG-8 and must not be folded into it:** fixing the endpoint does not make a sight test read the sight flag, and reading the sight flag does not fix the endpoint. Described in the as-found docs since the hardening burst; never given a row until now | unmeasured -- depends on how many D2 tiles disagree between the two flags | unassigned -- **Josh to name** | OPEN |

## Launch ledger

One line per playtest / harness run: **date · build · launches · failures · note.**
Seeded from §0(a) and the counts already in the record. Every playtest launch from
the hardening burst on is added here.

| date | build | launches | failures | note |
|---|---|---|---|---|
| 2026-09-09 | winnability run | 7 | 0 | 7/7 clean (seed×class matrix; `Winnability Run - 9 Sep 2026.md`) |
| 2026-09-10 | 10 Sep suite | ≥15 | (unrecorded) | the playtest suite (`Session - Claude Code 10 Sep 2026.md`) |
| 2026-09-11 | M4.4a | 4 | (unrecorded) | M4.4a build note |
| 2026-09-14 | `344da610` (pre-edits) | 1 | 0 | §0(a) escape-pause act; clean launch, no map-load crash (`game-20260914-203931.log`) |
| 2026-09-14 | hardening burst (harness) | 17 | 0 | full playtest suite via `scripts\playtest.ps1`, all 24 tests green, no map-load crash (`playtest-hardening-2026-09-14.txt`) |
| 2026-09-15 | M4.4c-1 (overnight, Claude Code) | (unrecorded) | 0 | the build's own closeout run: 25 tests green including `TestSquadsOnScreen`, per the session transcript; the launch count was not recorded before the session hit its usage limit |
| 2026-09-15 | M4.4c-1 review repairs (Cowork) | 31 | 0 | 1 squads act + 4 squads/resolver + 8 negative-control runs + the 18-launch full suite. **Seven of those runs went red on purpose** (the negative controls); **no map-load crash in any of the 31** (`playtest-c1-2026-09-15.txt`) |

*The night is the enemy · history is the clock · grounded, then supernatural · one region done deeply.*
