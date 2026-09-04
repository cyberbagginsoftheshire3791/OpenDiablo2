# CLAUDE.md — Project Strigoi (OpenDiablo2 fork)

Wallachia, June 1462: a historical open-world survival RPG with horror at
night, built on a fork of the archived OpenDiablo2 engine. D2 is
scaffolding and design reference, not the destination.

**Read first, every session:** `state.md` in the claude.ai project ("Dunno
Yet") + the Notion Strigoi Project Center (workstreams, gates, decisions).
The plan (`Project Plan.md`) and Constitution live at Josh's Video Game
folder root; the Constitution governs every session on any surface. In
Claude Code the SessionStart hook prints the live facts (branch, tree,
build, Article V status) and `.claude/FOCUS.md` — trust those over the
frozen numbers below.

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
8. **An audit verifies its own instruments before anyone acts on it**
   (Constitution VI.4, added 28 Aug). Nothing sits above an audit to
   catch its mistakes. So: every finding carries the command that
   produced it; every instrument passes a positive AND a negative
   control first -- a grep included, and findings that AGREE with you
   get the controls too, because the expected number is the one nobody
   checks; one headline finding is re-measured with a DIFFERENT
   instrument, since repetition confirms and only independence can
   disconfirm; and the audit states what it did NOT look for. The check
   anyone can run, no expertise needed: **"what did you run to get
   that?"** A finding whose command cannot be produced is withdrawn,
   not defended.
9. **Harness-reachable is not game-reachable** (A2, 28 Aug). A symbol
   the playtest harness can call and the shipped game cannot is a
   milestone that looks wired and is hollow. It cost M4.1
   (`Light.Remove`) and M4.3b (the whole notice->pursuit seam), and a
   lesson did not stop it twice, so it is now a gate:
   `go run ./tools/reachcheck` from the repo root, with `deadcode` on
   PATH. It is a **curated allowlist, not a sweep** -- the d2harness
   registry makes every system reflection-live, which blinds `deadcode`
   to exactly this class, so the register is hand-maintained and **a
   symbol absent from it is not checked**. When a milestone ships a
   system, its verbs go on the register in the same commit, each in one
   of four buckets: wire, observe, defer (naming the milestone that
   picks it up), delete. See `docs/reachability.md`.

## Skills — invoke them, do not re-derive them

Three saved skills carry the procedures this project learned the
expensive way. They live on Josh's claude.ai account (saved 3 Sep 2026),
so they are available on **every** surface — Cowork, Claude Code, chat —
and they are deliberately **not** copied into `.claude/skills/`, which
holds only the repo-local `d2-formats` skill. Two writable homes for one
procedure drift, and the stale one is what a cold session reads first.

The spine of a burst, in order:

1. **`brief-then-attack`** — before any milestone step, research topic or
   finding is built or filed. One session writes the plan; a second,
   independent agent tries to break it. This has caught four A-severity
   findings across three M4.5 steps, and it is why C3 is filed correctly:
   the lead pass read its load-bearing sentence backwards.
2. **`strigoi-measure-first`** — at the *start* of any build burst whose
   design depends on engine behaviour, before code or assertions.
   Throwaway playtest scaffold, deleted before the commit. Step 4's §0
   pass changed three design decisions before they cost anything (player
   `max_health` is 240 not 166; a pursuer's route ends on the quarry's
   **own** tile; `advanceWorld` runs once per **frame**). **A measurement
   that contradicts the brief beats the brief** — say so in the build
   note and move on.
3. **`strigoi-burst-closeout`** — when a step is finished and needs
   filing: the gate, the FULL reachability register, every playtest
   script, negative controls, commit, CI, Notion, the tracker, state.md,
   the handoff.

They compose; they do not substitute. Closeout is not the gate and the
brief is not §0. **If a skill is not in the session's skill list, say so**
rather than improvising a shortened version of it — an improvised
closeout is how a step ships with the register half-run.

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
    go vet -tags harness ./... && go vet -tags "harness playtest" ./...   # the gate, tagged
    go run ./tools/strigoihook check-fixtures         # Article V (CI runs it too)
    go build -tags harness -o od2-harness.exe . && ./od2-harness.exe -harness   # the playtest harness (docs/harness.md)
    go test -tags playtest ./playtest/... -v -count=1  # playtest scripts: laptop only, never CI

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
  **Headless rule (CI ubuntu has no display; xvfb was dropped at M2.5):**
  ebiten's package init calls GLFW, so any *test binary* that links ebiten
  panics on Linux. Keep ebiten adapters in leaf packages the app wires
  (`d2render/ebiten`, `d2audio/ebiten`, `d2input/ebiten`); before adding tests
  to a package, `go list -deps <pkg> | grep hajimehoshi/ebiten` must be empty
  (the M3.3 FrameDeltas placement, the GlyphPrinter move, and the M3.4
  d2input fix were all this rule).
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
- Dead: `d2thread/`, `d2udpclientconnection`, `BlizzardIntro`, `build.sh`,
  `tagdev.bat`, `docs/status.md` (2021 AbyssEngine notice — do not follow).
  `rh.exe` (an orphaned Resource Hacker binary) was removed 21 Aug.

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
- **Fixtures (Article V):** decoder tests synthesize their bytes in code;
  every file under `testdata/` is listed in `docs/fixtures-manifest.md` with
  origin and justification. The only tracked file matching the .gitignore
  block is `d2loader/testdata/D.mpq` (hand-built, not extracted). The
  inherited Blizzard-derived AnimData fixtures were removed 21 Aug (M2.3);
  check compliance with `git ls-files`, never by reading `.gitignore`.
- **CI (M2.4, 21 Aug):** `.github/workflows/ci.yml` runs gofmt, vet, build,
  test and the Article V check on ubuntu and windows for every push and PR.
  The inherited `.circleci/`, `.golangci.yml` and auto-author-assign
  workflow were deleted (dead). **Hooks:** `.claude/settings.json` runs
  `go run ./tools/strigoihook <event>` — SessionStart prints live facts +
  FOCUS.md; PreToolUse denies D2-format writes, the `/extracted/` and
  `/assets-d2/` drop zones, `go get -u`, `git add -f`, and *asks* before
  destructive git (force-push, reset --hard, clean -f, filter-repo) or an
  edit to the guardrails themselves; PostToolUse runs gofmt/build/vet after
  a Go edit and reminds about d2-formats after a `d2common/` edit. The rules
  are unit-tested (`tools/strigoihook/guard_test.go`). A hook that cannot
  run (no Go on PATH) fails open with a warning; CI is the backstop.
