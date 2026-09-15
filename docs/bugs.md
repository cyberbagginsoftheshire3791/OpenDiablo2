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

## Defects

| id | first seen | build | reproduction | rate | owner | status |
|---|---|---|---|---|---|---|
| BUG-1 | pre-fork (upstream) | any | `MapEngine.addDT1` logs a failed `LoadDT1` then dereferences `dt1.Tiles` — nil panic on a DT1 that will not load (`d2core/d2map/d2mapengine/engine.go:102-107`) | intermittent (map load) | the DT1 milestone | PARKED |
| BUG-2 | pre-fork (upstream) | any | `d2dt1.DecodeTileGfxData` indexes `(*pixels)[offset]` with no bound — index-out-of-range on malformed tile gfx (`d2common/d2fileformats/d2dt1/gfx_decode.go:30,60`) | intermittent (map load) | the DT1 milestone | PARKED |
| BUG-3 | pre-fork (upstream) | any | `MainMenu.createLogos` logs then dereferences a nil logo (`d2game/d2gamescreen/main_menu.go:325-331`) | intermittent (startup) | the DT1 milestone | PARKED |
| BUG-4 | ~Aug 2026 | any | the black floor: cached floor tiles are provably fully coloured while the composite draws nothing. A NIGHT screenshot is useless as evidence — the night scripts sample daylight control frames for exactly this reason | unmeasured, per launch | unassigned | PARKED |
| BUG-5 | ~Aug 2026 | any | the startup / map-load crash — presents as one of BUG-1/2/3. Intermittent; **re-run before investigating**. Every playtest launch is counted in the ledger below so the rate can be measured before friends build #1 (9 Sep G9) | intermittent | unassigned | PARKED |

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

*The night is the enemy · history is the clock · grounded, then supernatural · one region done deeply.*
