---
name: d2-formats
description: Reference for Diablo II binary file formats (MPQ, DC6, DCC, DS1, DT1, COF, PL2, DAT, TBL, TXT, AnimData) and the decoders in this repo that read them. Use this skill whenever the task touches game asset loading, sprite or animation decoding, map data, palettes, string tables, data tables, or anything under d2common/d2fileformats, d2common/d2loader, or d2core/d2asset. Also use it when debugging corrupted or misrendered assets, writing or changing a decoder, adding an encoder or a loose-file asset path, or creating test fixtures — even if the user doesn't name a format explicitly. These formats are reverse-engineered and undocumented outside this repo, so do not rely on general knowledge about them; read this skill instead.
---

# Diablo II File Formats

Every asset in this game lives inside an MPQ archive in one of a handful of
reverse-engineered binary formats. There is no authoritative external spec. The
decoders in `d2common/` are the specification — they work against real retail
MPQs, which is the only test that matters.

**Do not infer field layouts from general knowledge of similar formats.** These
formats predate most conventions and break them freely. When something here
disagrees with what you'd expect, this file is right. Every reference cites
`path/to/file.go:LINE` at commit `c26dc732` (21 Aug 2026); if the line has
moved, grep for the quoted identifier. Where the code has a constant with no
explanation, the references say **UNEXPLAINED** rather than inventing one.

## Patch version

This fork targets Diablo II + Lord of Destruction **1.14b** — the retail
installers on the development machine and the MPQ set the loader expects
(`d2core/d2config/defaults.go:25-37`; `docs/purchase.md`). The decoders' own
version gates are the real contract: DT1 accepts **only 7.6**
(`d2common/d2fileformats/d2dt1/dt1.go:70-73`); DS1 handles **v3–v18** with
per-version predicates (`d2common/d2fileformats/d2ds1/ds1_version.go`); DC6,
DCC, COF, PL2 and TBL read a version byte and **never check it**. A fixture or a
wiki page from another patch may not match — check the gate before trusting it.

## The formats at a glance

| Format | Holds | Decoder (entry point) | Test | Reference |
|---|---|---|---|---|
| **MPQ** | Everything — Blizzard's archive container | `d2common/d2fileformats/d2mpq` — `FromFile`, `ReadFile` (`mpq.go:59,96`) | none (indirect via `d2loader`) | `references/mpq.md` |
| **DC6** | Sprites: UI, fonts, item icons, ground items | `d2common/d2fileformats/d2dc6` — `Load` (`dc6.go:52`) | `dc6_test.go` | `references/sprites.md` |
| **DCC** | Compressed animations: units, missiles, overlays | `d2common/d2fileformats/d2dcc` — `Load` (`dcc.go:24`) | **none** | `references/sprites.md` |
| **DT1** | Isometric tile graphics + sub-tile flags | `d2common/d2fileformats/d2dt1` — `LoadDT1` (`dt1.go:54`) | `subtile_test.go` only | `references/maps.md` |
| **DS1** | Map stamps — which tiles go where, in layers | `d2common/d2fileformats/d2ds1` — `Unmarshal` (`ds1.go:49`) | 3 files | `references/maps.md` |
| **COF** | Animation composition: layers, order, events | `d2common/d2fileformats/d2cof` — `Unmarshal` (`cof.go:59`) | `cof_test.go` | `references/animation.md` |
| **AnimData.d2** | Frame counts, speeds, per-frame events | `d2common/d2fileformats/d2animdata` — `Load` (`animdata.go:122`) | `animdata_test.go` (synthesized) | `references/animation.md` |
| **DAT** | 256-colour palette, 3 bytes each, B-G-R | `d2common/d2fileformats/d2dat` — `Load` (`dat.go:16`) | **none** | `references/palette.md` |
| **PL2** | Palette + precomputed transform tables (parsed, **unused**) | `d2common/d2fileformats/d2pl2` — `Load` (`pl2.go:32`) | `pl2_test.go` | `references/palette.md` |
| **String TBL** | Localisation key → text | `d2common/d2fileformats/d2tbl` — `LoadTextDictionary` (`text_dictionary.go:118`) | `text_dictionary_test.go` | `references/tables.md` |
| **Font TBL** | Glyph metrics for a DC6 sheet | `d2common/d2fileformats/d2font` — `Load` (`font.go:37`) | **none** | `references/tables.md` |
| **TXT** | Game data tables (tab-separated; BIN is never read) | `d2common/d2fileformats/d2txt` → `d2core/d2records` (86 loaders) | `object_lookup_record_test.go` only | `references/tables.md` |

## Reading order for common tasks

- **"Why does this sprite render wrong?"** → `sprites.md`, then `palette.md`.
  Index 0 is transparent by code, not by palette; PL2 transforms are never
  applied; four DrawEffects are silently no-ops.
- **"Add a new monster / new unit art"** → `tables.md` (monstats → monstats2 →
  `Code` token), then `animation.md` (the COF/DCC naming convention, which is
  the thing nobody documents), then `sprites.md` (DCC).
- **"The map is broken"** → `maps.md`, then `sprites.md` for DT1 block
  decoding. The DS1 layer encoder has several known bugs — check gotchas first.
- **"Text is garbled or missing"** → `tables.md`, string-TBL section (first
  table wins: patch → expansion → base; a raw key on screen is a lookup miss).
- **"Anything at all is failing to load"** → `mpq.md` first. Source order and
  path normalisation cause more bugs than every decoder combined; a loose file
  beside the exe shadows the MPQs.
- **"Add loose PNG/OGG/JSON assets" (Phase 5)** → `mpq.md` "Archive
  precedence and loading": the filesystem source already precedes the MPQs;
  what's missing is the extension table, an asset-manager `Load*`, and a fix
  for the always-false `Exists`.
- **Before touching any decoder** → `gotchas.md`. Assume the weird thing you
  just found is already documented there.

## Hard rules

**Never modify a decoder without a test.** Format decoders are pure functions
over bytes. Add or update a table-driven test in the same commit, with bytes
synthesized in code (see `d2animdata/animdata_test.go` for the pattern).

**Never commit game assets — no exceptions, no "small slices".** Project law
(Constitution, Article V; `CLAUDE.md` law 4) bars MPQs and anything extracted
or derived from them. Fixtures are synthesized. Every file under any
`testdata/` is listed in `docs/fixtures-manifest.md` with origin and
justification; compliance is checked with `git ls-files`, never by reading
`.gitignore`. An extracted slice would need a Constitution amendment first.

**Never write to a game asset file.** Loading is read-only. If a task appears
to need writing DC6 or MPQ data, it belongs in a separate encoder tool with its
own output directory, not in the loading path (there is no MPQ writer in the
repo; `encrypt` in `crypto.go` is dead code).

**Prefer the existing decoder over a new one.** These formats have long tails
of edge cases already handled. If a decoder seems not to support something,
read it carefully before concluding it doesn't — the support is often there
under an unexpected name.

**Update this skill in the same commit as the code.** A skill maintained on a
separate cadence is a skill that's wrong. Cross-references are by path and
line so drift is greppable. (An edit hook for `d2common/` arrives at M2.4.)

## Fixtures

Canonical list and rule: `docs/fixtures-manifest.md` (the skill's
`assets/fixture-manifest.md` only points there). As of `c26dc732` the only
tracked file that even matches the asset patterns is
`d2common/d2loader/testdata/D.mpq`, an 891-byte archive a contributor hand-built
around two text files. Everything else is built in code.

## What this skill does not cover

Gameplay systems built on these formats (maps, entities, inventory, lighting —
there is none) are mapped in `docs/architecture-as-found.md`. D2's combat and
progression math is a separate future skill (`arpg-design`).
