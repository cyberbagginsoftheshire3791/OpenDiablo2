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
7. One dep bump per commit. Never `go get -u ./...`. akara stays pinned.

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

## Map of the code (as-found; archaeology pass pending, Phase 2)

- `d2app/` boot, flags, config load · `d2core/d2config/` defaults
- `d2common/d2fileformats/` MPQ/DC6/DCC/DS1/DT1/COF/TBL decoders
- `d2common/d2loader/` archive + filesystem asset sources (loose files
  already work) · `d2script/` embedded JS engine · `d2networking/`
  client/server scaffolding · `docs/status.md` upstream's 2021 state
