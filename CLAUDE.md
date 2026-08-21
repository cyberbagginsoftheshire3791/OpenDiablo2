# CLAUDE.md — Project Strigoi (OpenDiablo2 fork)

Wallachia, June 1462: a historical open-world survival RPG with horror at
night, built on a fork of the archived OpenDiablo2 engine. D2 is
scaffolding and design reference, not the destination.

**Read first, every session:** `state.md` in the claude.ai project ("Dunno
Yet") + the Notion Strigoi Project Center (workstreams, gates, decisions).
The plan (`Project Plan.md`) and Constitution live at Josh's Video Game
folder root; the Constitution governs every session on any surface.

## The law (condensed — Constitution is canonical)

1. One goal per burst; state it in one sentence before working.
2. Verify against the live system (git status, build, tests, the running
   game) before asserting anything. Code outranks memory and docs.
3. Closeout is mandatory: state.md updated, work committed cleanly,
   Notion status updated, claims backed by same-burst listings.
4. **Blizzard content never enters the repo.** MPQs and anything
   extracted/derived are blocked by .gitignore (see the Strigoi block).
   New fixtures need explicit justification in a manifest. Builds go
   only to people who own D2. The public repo is the GPL strategy.
5. The asset ratchet only tightens: D2-asset dependency shrinks, never
   grows; new content never targets D2 binary formats.
6. Case-against rule: no major direction change without the strongest
   honest case against it written down first. Binds Claude and Josh.
7. One dep bump per commit. Never `go get -u ./...`. akara stays pinned
   (it is only a BitSet in two files; removal is an M2.5 decision).

## Verified facts (2026-08-20, Josh's machine, Windows/amd64)

- Toolchain: Go 1.27.0 at `C:\Program Files\Go\bin` (go.mod directive
  1.25.0 — the minimum the dep graph accepts; do not lower it).
- `go build ./...` clean · `go vet ./...` clean · all 20 test packages
  pass · gofmt-clean tree (LF enforced by .gitattributes — do not
  commit CRLF; repo-local core.autocrlf=false).
- Renderer: ebitengine v2.9.10 (bumped from v2.0.2, runtime-verified).
  Audio backend is oto/v3 via ebiten.
- Runtime: boots against real MPQs at `C:\Program Files (x86)\Diablo II`
  (the engine's Windows default MpqPath). Config file:
  `%AppData%\OpenDiablo2\config.json` (created on first run).
- M1.1 verified by hand: trademark screen → single player → character
  created → Rogue Encampment walkable.

## Commands

    go build -o OpenDiablo2.exe .   # build the game
    ./OpenDiablo2.exe -l 4          # run with info logging (-l 5 debug)
    go build ./... && go vet ./... && go test ./...   # the green gate

## Map of the code — verified by the M2.1 archaeology pass (2026-08-21)

Full map with `path:line` evidence: **`docs/architecture-as-found.md`**
(note: the docs directory is lowercase `docs/` — upstream's; never create a
capitalized `Docs/`, it aliases on Windows and collides on Linux).

- `main.go` → `d2app/` (flags, config, engine assembly, update/render
  callbacks, terminal commands). Loader sources in precedence order: exe
  dir → `%AppData%\OpenDiablo2\` → the 11 MPQs in `MpqLoadOrder`. Loose
  files beside the exe already shadow MPQ content.
- `d2common/` — enums, interfaces, math, bit/byte streams, the loader
  (`d2loader`), and every decoder (`d2fileformats/`: MPQ, DC6, DCC, DS1,
  DT1, COF, TBL, font, PL2, DAT, TXT, AnimData). The stable, testable floor.
- `d2core/` — `d2asset` (asset manager, the hub), `d2records` (76 TXT tables
  → typed records; the layer our own data replaces), `d2map/*` (engine,
  Act-1-only generator, DS1 stamps, four-pass renderer), `d2mapentity`
  (plain structs — **there is no ECS**; akara is used only for a BitSet),
  `d2hero` (JSON `.od2` saves in `%AppData%\OpenDiablo2\Saves`), `d2ui` +
  `d2gui` (two toolkits), `d2term` (in-game console — the proto-harness).
- `d2game/` — screens and the HUD/panels (`d2player`, the most finished
  code in the repo). `d2networking/` — single-player runs an in-process
  TCP server on 127.0.0.1:6669; local packets are direct calls.
- `d2script/` — otto VM, wired to nothing but the `js` console command.
- Dead: `d2thread/`, `d2udpclientconnection`, `BlizzardIntro`, `rh.exe`
  (Resource Hacker, 5.5 MB, unreferenced), `build.sh`, `tagdev.bat`,
  `docs/status.md` (2021 AbyssEngine notice — do not follow).

## Engine truths that size the work (do not re-derive; see the doc)

- **No lighting exists** (renderer has no light radius/ambient/day-night;
  hook = `Surface.PushBrightness/PushColor`). **No combat, AI, or monster
  type** (NPC walks DS1 waypoints; `monai`/spawn tables are parsed, unread).
  **No pathfinder** (raycast to the last unblocked tile) and no entity
  collision. Game logic runs on a wall-clock variable delta — no fixed step.
- Inventory is split: `d2core/d2inventory` is a paperdoll DTO; the working
  grid lives in `d2game/d2player/inventory_grid.go` under a second item
  type; there is no pickup code.
- Loader bugs to fix before loose assets (Phase 5): `filesystem.Source.Exists`
  always false; `Loader.Cache` unused; `LoadDS1` uses the DT1 cache.
- **Article V flag:** three tracked files match the Strigoi .gitignore block
  (`d2animdata/testdata/AnimData.d2`, `BadData.d2` — Blizzard-derived;
  `d2loader/testdata/D.mpq` — synthesized). Pending Josh's decision at M2.3;
  do not add more, and do not copy them anywhere.
- No CI runs (`.circleci` template-broken; `.golangci.yml` names dead
  linters). The green gate is the only gate until M2.4 adds a workflow.
