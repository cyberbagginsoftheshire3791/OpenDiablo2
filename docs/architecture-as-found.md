# Architecture as found — OpenDiablo2 fork @ `87fe2aa0`

**Project Strigoi · M2.1 archaeology pass · 21 August 2026 (amended the same day after M2.3 — see §0.6, §6, §10).** This is the map of the codebase *as it actually is*, read from code — not from `docs/status.md` (a March 2021 AbyssEngine-migration notice; see §9), not from memory, not from upstream's README. It seeds `CLAUDE.md` and is the reference every later doc points to. Measurements are dated; re-derive them before trusting them a month from now (Constitution II.5).

**Method.** Four parallel read-only passes over a fresh clone of the fork at `87fe2aa0` (boot/loop; formats/loader; world/gameplay; deps/tests/hygiene), a scripted import-graph census, spot-checks of every load-bearing claim against the source, and two runs on Josh's machine (the green gate, and a fixture-provenance check against the installed MPQs). Every factual claim carries a `path:line` citation to `87fe2aa0`; where the code has an unexplained constant or branch the entry says **UNEXPLAINED** rather than guessing.

**Gate, as measured 2026-08-21 on joshslaptop (Go 1.27.0, windows/amd64):** `master` == `origin/master` at `87fe2aa0`, tag `v0.0.1-firstlight`; working tree clean except the untracked `rh.ini`; `go build ./...` OK; `go vet ./...` OK; `gofmt -l .` empty; `go test ./...` — 20 packages `ok`, 0 failures. **7 commits over the upstream fork point `7f92c571`** (earlier session notes said 8; git says 7 — `git log --oneline 7f92c571..HEAD | wc -l`).

---

## 0. Headline findings — the ten things that change plans

1. **There is no ECS.** `github.com/gravestench/akara` is imported in exactly two files, both for its `BitSet` type (`d2core/d2records/monster_unique_affix_record.go:22`, `monster_unique_affix_loader.go:86`). No `akara.World`, system, or component exists anywhere. Entities are plain structs with embedding (`d2core/d2map/d2mapentity/map_entity.go:14`). "akara stays pinned" is harmless, but it is a whole framework pinned for one bitset on a 2020 pseudo-version.
2. **There is no lighting.** No light radius, no light sources, no ambient term, no day/night, anywhere in the renderer. "Shadow" means the DS1's pre-baked static shadow layer, drawn at a hardcoded alpha of 160 (`d2core/d2map/d2maprenderer/renderer.go:446`). The hook exists — `PushColor`/`PushBrightness`/`PushSaturation`/`PushEffect` on `Surface` (`d2common/d2interface/surface.go:21-28`) — and D2's own light data is parsed but unread (`d2pl2.PL2.LightLevelVariations`, `d2common/d2fileformats/d2pl2/pl2.go:13`; monster/object light fields in `d2records`). **M4.1 builds a light layer from scratch.**
3. **There is no combat, no AI, and no monster type.** A repo-wide search for attack/damage/health/death/aggro function names returns only `SoundEngine.commandKillSounds` (`d2core/d2audio/sound_engine.go:263`). `NPC` doubles as monster (`d2mapentity/npc.go:17`, `factory.go:183`) and only walks DS1-authored waypoints and idles (`npc.go:94-145`). `monai.txt`, `monstats`, `levels.txt` spawn densities are all **parsed into records and consumed by nothing** (`d2records/monster_ai_loader.go:9`; `level_details_record.go:228,234,253`). **The plan's "reuses D2's monster systems where they exist" resolves to: the data exists; the systems do not.**
4. **The script engine is real but unbound.** otto is wired (`d2script/engine.go:9,21`) and exposes exactly one global, `debugPrint` (`engine.go:22-25`), plus a server-side `getMapEngines` (`d2networking/d2server/game_server.go:120-126`). `RunScript` has **zero call sites**; nothing loads `scripts/`. The only live path is the `js` terminal command (`d2app/console_commands.go:28,61-71`). Quest scripting means building the bindings.
5. **The inventory is split in two.** `d2core/d2inventory` is a paperdoll DTO — eight flat equipment fields with a `// S1-S8?` comment (`character_equipment.go:4-14`), no container, no grid, no add/remove. The *working* grid, slot enum (belt, gloves, rings…), and fit logic live in the UI layer, `d2game/d2player/inventory_grid.go:40,121,187` and `inventory.go:167-178`, under a second, incompatible `InventoryItem` type, with hardcoded test items and **no pickup code at all**. UNEXPLAINED: no comment or issue explains the split.
6. **Three tracked files matched the Article V block at `87fe2aa0`** — resolved at `f99802d4` (M2.3, 21 Aug): the two `.d2` files are removed from HEAD, the animdata tests synthesize their bytes, and `docs/fixtures-manifest.md` is the rule and the list. As found, `git ls-files` matched the Strigoi `.gitignore` block three times: `d2common/d2fileformats/d2animdata/testdata/AnimData.d2` (570,304 B), `BadData.d2` (570,368 B, a scrambled sibling), and `d2common/d2loader/testdata/D.mpq` (891 B, synthesized, nominal). Verified on Josh's machine with the engine's own decoder: the fixture is **exactly the size** of `data\global\animdata.d2` in `d2exp.mpq` (1.14b) but a different sha256 — another patch's copy or a re-marshaled derivative; provenance UNEXPLAINED, Blizzard-derived beyond reasonable doubt (3,558 real animation tokens inside). They predate the fork by six years (`65cce60e`, 2020-09-08); `.gitignore` does not untrack existing files. Decision taken 21 Aug (§10 item 1 — done; history rewrite deferred).
7. **The loader's cache is decorative, `Exists` is broken, and `LoadDS1` uses the wrong cache.** `Loader.Cache` is allocated (`d2common/d2loader/loader.go:44`) and never read or written by `Load` (`loader.go:87-102`); `filesystem.Source.Exists` returns `os.IsExist(nil)` — always false (`filesystem/source.go:31-33`); `AssetManager.LoadDS1` reads from and writes to the **DT1** cache (`d2core/d2asset/asset_manager.go:511,525`). None of this bites today; all of it bites Phase 5 (loose-file assets) unless fixed first.
8. **Single-player runs a real TCP server in-process.** `LocalClientConnection.Open` constructs a `GameServer` that binds `127.0.0.1:6669` (`d2networking/d2client/d2localclient/local_client_connection.go:69-85`; `d2networking/d2server/game_server.go:134,141`), but the local client's packets are direct method calls, not sockets (`local_client_connection.go:42-44,104-106`). Player state flows as `HeroState` JSON through `MovePlayer`/`SavePlayer`/`CastSkill`/`SpawnItem` packets (`d2game/d2gamescreen/game.go:326-387`).
9. **Game logic runs on a wall-clock variable delta.** `advance` derives `elapsed` from `d2util.Now()` scaled by `timeScale` (`d2app/app.go:404-410`); there is no fixed-step accumulator. The ebiten `Game` callbacks are passed swapped relative to their names — `a.update` renders, `a.advance` updates (`d2app/app.go:317`; `d2core/d2render/ebiten/ebiten_renderer.go:109-111`). **M4.4's world clock needs its own deterministic step**, and the Phase 3 harness's `step_frames` must not assume one exists.
10. **There is no CI, and the repo carries 6 MB of dead binaries.** `.github/workflows/` holds only an auto-assign action; `.circleci/config.yml` has unsubstituted `{{ORG_NAME}}` placeholders (line 9) and pins Go 1.16; `.golangci.yml` names linters that no longer exist; `dependabot.yml` watches GitHub Actions, not `gomod`. `rh.exe` (5,479,424 B) was **Resource Hacker**, added 2019 for a deleted Travis icon-embedding step and referenced by nothing in the tree — removed at `1866ffa7` (21 Aug); `d2logo.ico` and `d2discord.png` remain orphaned.

---

## 1. Shape

**77 packages · 603 `.go` files · 71,587 lines · 28 test files in 20 packages** (scripted census, 2026-08-21). Module path is still `github.com/OpenDiablo2/OpenDiablo2` (`go.mod:1`) — every import in the tree uses it; renaming the module is a tree-wide mechanical change for a later burst.

| Directory | Files / LOC | What it actually is |
|---|---|---|
| `main.go`, `d2app/` | 1 + 4 / 1,034 | Entry, flags, config, engine assembly, the update/render callbacks, terminal commands |
| `d2common/` | 216 / 16,638 | Enums, interfaces, math, the loader, every file-format decoder, records of nothing — **the stable, testable floor** |
| `d2core/` | 310 / 38,965 | Asset manager, records (173 files), map engine/gen/stamp/renderer, entities, UI (two toolkits), hero/stats/item/inventory, render/audio/input backends, screen manager, terminal |
| `d2game/` | 33 / 12,308 | Screens (menu, select, game, credits, debug) and the in-game HUD/panels (`d2player`, 8,679 LOC — the most complete subsystem in the repo) |
| `d2networking/` | 34 / 2,357 | Client/server, packets, local/remote/TCP; UDP is dead |
| `d2script/` | 2 / 92 | otto wrapper (§4.6) |
| `d2thread/` | 1 / 89 | **Dead.** A copy of faiface/mainthread that nothing imports; `d2app` calls `runtime.LockOSThread()` directly (`d2app/app.go:104`) |
| `utils/extract-mpq/` | 2 / 104 | Working CLI that dumps an MPQ to disk with the in-repo decoder — the one genuinely useful tool |
| `scripts/` | 2 JS | Placeholders; never loaded |
| `docs/` | 9 md + site | Upstream GitHub Pages + 11 MB of screenshots; see §9 |

**Package map** (one line each; in-degree = number of internal packages importing it; full table with LOC in Appendix A):

*d2common* — `d2enum` (54 files, in=26: every enum — animation modes, slots, regions, packet types); `d2interface` (in=24: Surface, Renderer, Asset manager, MapEntity, Palette, InputManager…); `d2util` (in=23: logger, timers, color, debug_print — imports ebiten for `ebitenutil`, the only ebiten leak outside backends); `d2math`/`d2vector` (in=12/10: the densest test suite); `d2datautils` (in=10: bit/byte stream readers — the primitive under every decoder); `d2resource` (in=11: every MPQ path constant); `d2geom`, `d2path`, `d2cache` (LRU, used by d2asset), `d2calculation` + `d2lexer` + `d2parser` (D2 formula expressions), `d2data/d2compression` (MPQ-internal Huffman + IMA-ADPCM), `d2data/d2video` (BIK header only), `d2loader` + `asset` + `asset/types` + `filesystem` + `mpq` (§3.2), `d2fileformats/*` (§3.1).

*d2core* — `d2asset` (in=21: the asset manager, §3.3); `d2records` (173 files, 19,654 LOC: typed loaders for 86 TXT tables (83 bootstrapped), §3.4); `d2hero` (save/load, stats state); `d2stats` + `diablo2stats`, `d2item` + `diablo2item` (affix rolling, descriptors); `d2inventory` (paperdoll DTO, §4.4); `d2map/d2mapengine` (tiles, walkability bit, raycast "pathfinder"); `d2map/d2mapgen` (+`d2wilderness`: **Act 1 only**); `d2map/d2mapstamp` (DS1 stamps; the entire world-population system); `d2map/d2maprenderer` (four-pass painter's renderer, tile cache; no lighting); `d2map/d2mapentity` (Player, NPC, Object, Item, Missile, CastOverlay, AnimatedEntity); `d2ui` (sprite-bound widgets) and `d2gui` (layout-driven menus) — two toolkits; `d2screen` (screen manager, loading screen); `d2render/ebiten`, `d2audio/ebiten`, `d2input/ebiten` (backends); `d2config` (defaults + JSON); `d2term` (in-game console).

*d2game* — `d2gamescreen` (main menu, character select/create, game, credits, map-engine test, dead BlizzardIntro); `d2player` (HUD, panels, inventory grid, skills, key bindings, escape menu).

*d2networking* — `d2client` (+`d2clientconnectiontype`, `d2localclient`, `d2remoteclient`), `d2server` (+`d2tcpclientconnection`; `d2udpclientconnection` is unreferenced), `d2netpacket` (+`d2netpackettype`).

**Top 10 packages by incoming dependency count** (the prompt asked for files; Go dependencies are package-level, so this is the meaningful measure):

| # | Package | Importers | Note |
|---|---|---|---|
| 1 | `d2common/d2enum` | 26 | Touch with care — everything sees it |
| 2 | `d2common/d2interface` | 24 | The engine's contract surface |
| 3 | `d2common/d2util` | 23 | Logger/time; leaks ebiten |
| 4 | `d2core/d2asset` | 21 | The asset manager; 19 outgoing — the hub |
| 5 | `d2common/d2math` | 12 | |
| 6 | `d2common/d2resource` | 11 | MPQ path constants |
| 7 | `d2core/d2records` | 10 | |
| 8 | `d2common/d2math/d2vector` | 10 | |
| 9 | `d2core/d2hero` | 10 | Save format lives here |
| 10 | `d2common/d2datautils` | 10 | |

Packages with **no internal importers** (dead or entry points): `.` (main), `d2common` and `d2common/d2data` (doc-only), `d2thread` (dead), `d2networking/d2server/d2udpclientconnection` (dead), `utils/extract-mpq` (tool).

---

## 2. Boot sequence and main loop

### 2.1 `main.go` → first rendered frame

1. `main.go:20-24` — stdlib log flags; `d2app.Create(GitBranch, GitCommit)` (build-injected globals `main.go:11,17`); `instance.Run()`.
2. `d2app.Create` (`d2app/app.go:103`): `runtime.LockOSThread()` (`:104`); logger (`:106-107`); **flags** via `parseArguments` (`:195`): `-profile`, `-dedicated`, `-players`, `-l`, `-v`, `-h`; `flag.Parse()` (`:219`). **The asset manager is built before config is read** (`:123` → `d2core/d2asset/d2asset.go:12-42`: loader + record manager + eight caches); errors are stashed in `app.errorMessage`, not returned.
3. `App.Run` (`d2app/app.go:270`): two **filesystem sources** are added first — the executable's directory (`:272`; `d2core/d2config/default_directories.go:23-24`) then `os.UserConfigDir()/OpenDiablo2/` (`:273`; `default_directories.go:14-19`). `Loader.AddSource` appends (`d2common/d2loader/loader.go:125`); `Loader.Load` scans in insertion order, first hit wins (`loader.go:88-102`). **So loose files beside the exe or in the config dir already shadow MPQ content** — the Phase 5 loose-asset path is half-open today.
4. **Config**: `LoadConfig` (`:237`) asks the loader for the bare name `config.json` (`:239-241`); on miss, `d2config.DefaultConfig()` is written to disk (`:247-257`). Defaults: `d2core/d2config/defaults.go:16-52` — `TicksPerSecond: -1`, VSync on, BGM 0.3, SFX 1.0, per-GOOS `MpqPath` (Windows `C:/Program Files (x86)/Diablo II` at `:23`; 386 override `:44`; darwin `:47`; linux/Wine `:50`), and the 11-entry patch-first `MpqLoadOrder` (`:25-37`). **Gotcha:** `LoadConfig` sets the path to `DefaultConfigPath()` unconditionally (`:239-244`) — a `config.json` beside the exe is *read* but a later `Save()` writes to the user dir; the load error is discarded (`:241`).
5. Profiler (`:280-285`); `-dedicated` branch (`:288-292`; WIP — `d2networking/dedicated_server.go:31`).
6. `loadEngine` (`:159`): **renderer** `ebiten.CreateRenderer` (`:161` → `d2core/d2render/ebiten/ebiten_renderer.go:80-84`: cursor, fullscreen, vsync, `ebiten.SetMaxTPS(config.TicksPerSecond)`); **audio** (`:172` → `d2core/d2audio/ebiten/ebiten_audio_provider.go:22-33`, 44.1 kHz); **input** (`:174`); **terminal** (`:176`); **script engine** (`:181`); **UI manager** (`:183`).
7. `initialize` (`:302` → `d2app/initialization.go:17`): **MPQ sources** appended in `MpqLoadOrder` order (`initialization.go:70-79`; a missing MPQ aborts boot with a formatted message `:56-64,:77`); `{LANG}`/`{LANG_FONT}` token substitution (`:84-90`, consumed at `loader.go:83-84`); **83 TXT dictionaries** loaded through the record manager (`:93-131`; 86 loaders are bound, three — Belts, Gamble, ObjectMode — are never bootstrapped); `AnimData.d2` loaded out-of-band (`:145-162`; **bug:** a load error is logged, not returned, and falls through to a nil dereference at `:158`); GUI manager (`:36`), screen manager (`:43`), volumes, string tables, `ui.Initialize()` (`:51`).
8. `ToMainMenu()` (`:315` → `:599-606`) then `renderer.Run(a.update, a.advance, 800, 600, title)` (`:317` → `ebiten.RunGame` at `ebiten_renderer.go:117`).
9. **The first rendered frame is the loading screen, not the trademark screen.** `MainMenu` implements `OnLoad` (`d2game/d2gamescreen/main_menu.go:184`), so the first `Advance` shows the load screen and runs `OnLoad` on a goroutine while `currentScreen` is nil (`d2core/d2screen/screen_manager.go:69-95`; black clear + loading animation at `d2core/d2gui/gui_manager.go:102-134`). Trademark mode only after `OnLoad` completes (`main_menu.go:200-204,485-486`).
10. **Menu → game:** any click/key on trademark → main menu (`main_menu.go:556-578`); "Single Player" → character select if saves exist, else hero creation (`main_menu.go:432-440`; save detection `d2core/d2hero/hero_state_factory.go:173-178`); OK → `ToCreateGame(FilePath, connType, host)` (`character_select.go:539`; new hero saves an `.od2` first, `select_hero_class.go:507-514`) → game client created and opened, `Game` screen pushed (`d2app/app.go:621-646`).

### 2.2 The loop

`*ebiten.Renderer` implements ebiten's `Game`: `Update` (`ebiten_renderer.go:44`) calls `a.advance`; `Draw` (`:55`) wraps the frame in a surface and calls `a.update` → `a.render` (screen, UI, GUI, debug overlay, capture, terminal — `d2app/app.go:384-401`; surface-stack leak check `:437-439`); `Layout` is a fixed 800×600 (`:67-69`). `advance` (`:403-429`) uses a **wall-clock delta** (`d2util.Now()`, `d2common/d2util/timeutils.go:10-12`) times `timeScale` — no fixed step. `TicksPerSecond: -1` is passed straight to `ebiten.SetMaxTPS` (`ebiten_renderer.go:84`); it compiles and runs against v2.9.10, where that name is deprecated but present — **VERIFY** its exact semantics against the pinned version before the clock work.

### 2.3 Single-player through `d2networking`

`d2client.Create` → `d2localclient.Create(asset, l, false)` for `Local` (`d2networking/d2client/game_client.go:86-87`). A **`GameClient`** owns the `MapEngine`, a map generator, `Players`, `PlayerID`, `Seed`, and a `ServerConnection`, and is the `ClientListener` the server calls back (`game_client.go:37-51,96,132`). `Open` loads the `.od2` save into `HeroState` (`local_client_connection.go:72`; `hero_state_factory.go:187-200`), constructs `NewGameServer(asset, false, l, 30)` — which **generates the Act 1 overworld server-side before any client connects** (`game_server.go:107-118`) — starts it on `127.0.0.1:6669` (`:134,141`), and registers. On connect the server force-sets the spawn position (flagged as a hack, issue #829, `:327-333`) and sends `UpdateServerInfo`, `GenerateMap`, `AddPlayer` (`:342-391`; handled at `game_client.go:201-229`). Server-closed calls `os.Exit(0)` mid-loop (issue #802, `game_client.go:167-172`). Ping/Pong is half-dead (`CreatePingPacket` has no caller); UDP is dead.

---

## 3. Formats, loader, assets, records

*Byte-level detail for every format lives in the `d2-formats` skill (`.claude/skills/d2-formats/`, M2.2): per-format references with offsets, bit layouts, decoder behaviour and a harvested gotchas file. This section stays at the map level.*

### 3.1 Every place a D2 format is parsed

| Format | Package / entry | Test | What to know |
|---|---|---|---|
| **MPQ** archive | `d2mpq`: `New` `mpq.go:37`, `FromFile` `:59`; `ReadFile` `:96`, `ReadFileStream` `:118`, `Listfile` `:146`, `Contains` `:172` | no | Magic `MPQ\x1A` checked (`mpq_header.go:31`); `FormatVersion` read, **never branched on** (`:14`). Linux-only case-insensitive open fallback (`mpq.go:41,182`). `decompressMulti` (`mpq_stream.go:227`) supports only 2/8/0x80/0x40/0x41/0x81; Huffman, BZip2, LZMA, sparse, PK+wav branches **return errors** (`:231-277`) — `extract-mpq` wraps each file in `recover()` for this reason |
| **DC6** sprite | `d2dc6.Load` `dc6.go:52` | yes | No magic/version check (`Version` read `:96`, unchecked); `Termination`/`Terminator` stored blind (`:108,:165`); RLE `0x80`/`0x7f` (`:8-9`) |
| **DCC** sprite | `d2dcc.Load` `dcc.go:24` | no | Signature `0x74` (`:9,:33`). **UNEXPLAINED:** `if bm.GetInt32() != 1 { "this value isn't 1. It has to be 1" }` (`:43-45`); `directionOffsetMultiplier = 8` (`:10`) |
| **DS1** map | `d2ds1.Unmarshal` `ds1.go:49` | yes (3) | The richest version logic: v3–v18 (`ds1_version.go:5-18`); predicates for unknown bytes v9–13 (`:20`), ≥v18 (`:25`), act ≥v8 (`:29`), floors ≥v16 (`:49`), NPCs >v14 (`:66`). **UNEXPLAINED:** "meaningless (?) bytes" after the header (`:21`), `unknown2` (`ds1.go:39,294`). Pre-v7 wall orientations remapped via `dirLookup` (`ds1.go:509-511`); width/height are `+1` (`:92-93`) |
| **DT1** tiles | `d2dt1.LoadDT1` `dt1.go:54` | partial | **Hard gate: only version 7.6** (`:70-73`); 260 unknown header bytes skipped (`:22,:75`) + four more unknown runs (`:25-28`) |
| **COF** animation | `d2cof.Unmarshal` `cof.go:59`, `Marshal` `:54` | yes | **UNEXPLAINED:** `numUnknownHeaderBytes = 21`, `numUnknownBodyBytes = 3` (`:11-12`); layout inferred by offsets (`:128-134`) |
| **TBL** strings | `d2tbl.LoadTextDictionary` `text_dictionary.go:118` | yes | 2-byte CRC skipped unchecked (`:114,:124`); version byte discarded (`:147`). **Gotcha:** keys `x`/`X` become `#index` (`:92-94`); duplicate keys are *not* overwritten (`:96-99`) |
| **Font** TBL+DC6 | `d2font.Load` `font.go:37` | no | Magic `Woo!\x01` (`:15,:45`); 7 unknown header bytes (`:27,:54`); 14 bytes/glyph with unknown runs (`:20-25`); glyph sheet attached via `SetBackground` (`:65`) |
| **PL2** palette transforms | `d2pl2.Load` `pl2.go:32` | yes | Pure `restruct` unpack (`:37`), no magic/length check. **UNEXPLAINED** array sizes: `HueVariations [111]`, `UnknownVariations [14]`, `TextColors [13]` (`:19-27`). `LightLevelVariations [32]` (`:13`) — parsed, used by nothing |
| **DAT** palette | `d2dat.Load` `dat.go:16` | no | **Gotcha:** always returns `nil` error (`:24`), no bounds check — a short file panics at `:21`. B,G,R byte order (`:9-12`) |
| **TXT** tables | `d2txt.LoadDataDictionary` `data_dictionary.go:21` | no | Tab-separated; **panics** on an unreadable header (`:28`); rows whose first column is `Expansion` silently skipped (`:56-57`); not re-iterable (see `asset_manager.go:342-346`) |
| **AnimData.d2** | `d2animdata.Load` `animdata.go:122` | yes | Fixed shape `256` blocks × ≤`67` records × `144` events (`:12-16`); **UNEXPLAINED:** `speedDivisor = 256`, `speedBaseFPS = 25` (`:17-18`) |
| **WAV** (ADPCM) | `d2compression.WavDecompress` `wav.go:10` | no | MPQ-internal IMA-ADPCM only; playable WAV is decoded by **ebiten's** `wav.Decode` (`ebiten_audio_provider.go:80,154`) |
| **BIK** video | `d2video.CreateBinkDecoder` | no | Header-only; used by the dead `BlizzardIntro` screen |

### 3.2 The loader (`d2common/d2loader`) — the Phase 5 seam

`Source` interface: `Open(name) (io.ReadSeeker, error)`, `Path()`, `Exists(subPath)` (`asset/source.go:9`). Filesystem source = `os.Open(filepath.Join(Root, subPath))` (`filesystem/source.go:27,36`); MPQ source wraps `d2mpq.FromFile` and `ReadFileStream` (`mpq/source.go:16,33`). **Precedence** is append order, first `Open` wins (`loader.go:88-102,125`). **Normalization** is `filepath.Clean` + token substitution only (`loader.go:77,83-84`); case is never normalized; slashes are flipped to `\` only inside the MPQ source (`mpq/source.go:52-59`). **Bugs/dead:** cache never used (`loader.go:44,87`); `Exists` always false (`filesystem/source.go:31-33`) — feeds `AssetManager.FileExists` (`asset_manager.go:128`), used by audio (`ebiten_audio_provider.go:140`) and composites (`composite.go:259,322`); `filesystem.Asset`/`mpq.Asset` implement `asset.Asset` but nothing constructs them.

**To add loose PNG/OGG/JSON (M5.1) you touch:** `types.Ext2AssetType` (`asset/types/asset_types.go:30-43` — `json` already maps at `:31`); the `LoadAnimation` extension switch (`asset_manager.go:174-187`, errors on anything but DC6/DCC at `:186`); a new `Load*` + cache in the asset manager; and the `Exists` fix. You do **not** need a new `Source`; the filesystem source is format-agnostic and already precedes the MPQs.

### 3.3 The asset manager (`d2core/d2asset`)

Embeds `*d2loader.Loader` (`asset_manager.go:66`) with eight `d2cache` budgets (`d2asset.go:31-38`; `asset_manager.go:40-47`). `LoadAnimation` `:152` (DC6 `:375`/DCC `:393` by extension; cache key `path;palette;effect` `:159`) · `LoadComposite` `:195` · `LoadFont` `:212` (`d2font.Load` + DC6 sheet) · `LoadPalette` `:244` (`.dat` only `:249`) · `LoadStringTable` `:271` (no cache — appended to a slice searched linearly by `TranslateString` `:304`) · `LoadPaletteTransform` `:316` · `LoadDataDictionary` `:341` (deliberately uncached, `:342-347`) · `LoadRecords` `:360` · `LoadDT1` `:487` · `LoadDS1` `:510` (**wrong cache**, `:511,525` — a DT1 and DS1 sharing a name would type-assert-panic at `:489`) · `LoadCOF` `:533` · `LoadDCC` `:556`. **Gotcha:** `LoadDT1`/`LoadDS1` hardcode the `/data/global/tiles/` prefix (`:492,515`). Terminal: `assetspam`, `assetstat`, `assetclear` (`:410-418`).

### 3.4 Records (`d2core/d2records`) — where Diablo's data lives, and where ours will

173 files, paired `*_loader.go`/`*_record.go`. `RecordManager.init()` registers **86 path→loader pairs** (`record_manager.go:176-267`; 83 are loaded at startup): levels (types/presets/details), objects, monsters (stats/stats2/presets/AI/uniques), items (weapons/armor/misc/uniques/types), missiles, skills, treasure classes, anim-mode tokens, sounds, and more. `AddLoader` (`:282`) and `Load` (`:293`) are the API. **Ordering hazards:** `itemTypesLoader` must follow weapons/armor/misc (`// WARN` at `:193`; merge at `:307-320`). Bootstrap order is a separate hardcoded list in `d2app/initialization.go:126-131`. **This is the layer a Wallachia data set replaces** — a JSON/CSV loader that produces the same record types is the ratchet's first real win.

---

## 4. World and gameplay systems

### 4.1 Maps

A runtime map is a flat tile slice plus a UUID-keyed entity map (`d2core/d2map/d2mapengine/engine.go:24`; index `x + y*width` `:198`; 5×5 subtiles per tile `:45`). **Walkability is one bit** (`BlockWalk`, `d2dt1/subtile.go:5`, via `SubTileAt` `engine.go:203`). **There is no collision between entities** — `mapEntity.Step` applies velocity toward a target (`d2mapentity/map_entity.go:77`). **There is no pathfinder:** `MapEngine.PathFind` returns the last unblocked point along a straight raycast (`pathfind.go:10-19`); clicking behind a wall walks you to the wall.

**Generation is Act 1 only.** `GenerateAct1Overworld` (`d2mapgen/act1_overworld.go:38`) builds a fixed 150×150 map (`:31`), loads the town DS1 (`:47`), and branches on the DS1 filename to place wilderness east/south/west (`:86,152,206`); the `default:` branch yields **town with no wilderness** (`:80-82`). Wilderness = grass fill (`:283-292`), a forced Den of Evil entrance (`:276,316`), and exactly 25 stamps from a hardcoded 19-entry list (`:294-333`) via `areaEmpty` rejection sampling (`map_generator.go:47`). No Act 2–5, no dungeons. Live entry points: `game_server.go:116` and `game_client.go:193` (guarded on `RegionAct1Town`). `d2wilderness` is a bare enum (`wilderness_tile_types.go:5-55`).

**Stamps** (`d2mapstamp`): `LoadStamp` picks a random DS1 from the level preset unless indexed (`factory.go:74-77`), from `/data/global/tiles/` (`:84`). **`Stamp.Entities` is the entire world-population system** (`stamp.go:108`): DS1 characters → `NewNPC` via monpreset/monstat (`:112-127`), DS1 items → `NewObject` (`:130-151`); `place_` objects unhandled — "idk how to handle those yet" (`:115`).

**Renderer** (`d2maprenderer`): four painter's passes over the visible rect (`renderer.go:165-197`: lower walls/shadows/floors `:231`, entities below walls `:243`, upper walls + entities `:295`, roofs `:349`), tiles pre-rasterized into cached surfaces (`tile_cache.go:101,145,210`) with a per-act palette (`renderer.go:627`). Debug: `mapdebugvis`, `entitydebugvis` (`:96,102`). **Lighting: none** — see Headline 2; the only "light" identifiers are debug colors (`renderer.go:39,42`).

### 4.2 Entities

Types: `Player` (`player.go:15`), `NPC` (`npc.go:17`), `Missile` (`missile.go:12`), `Item` (`item.go:13`), `Object`, `AnimatedEntity`, `CastOverlay`; built by `MapEntityFactory` (`factory.go:47`). **No Monster type.** NPC behaviour: walk DS1 waypoints, idle 3–5 animation loops (`npc.go:94-145`, constants `:35-36`); no target, no perception, no state machine. Missiles are pure VFX (`missile.go:43-48`; self-delete on range via `game_client.go:405-407`). **Spawning, exhaustively:** DS1-authored NPCs at stamp time (`stamp.go:121`); skill summons (`game_client.go:340`); terminal `spawnmon`/`spawnitem`/`spawnitemat` (`d2game/d2gamescreen/game.go:142-144,415`). Stamina is the one simulated stat — drained running outside town, regenerated otherwise (`player.go:135-146`); health is written once and never decremented.

### 4.3 Hero, stats, saves

`HeroState` is a JSON DTO (`d2core/d2hero/hero_state.go:9-23`: name, class, act, equipment, stats, skills, x/y, left/right skill, gold, difficulty). **Saves are JSON files named `<n>.od2`** in `os.UserConfigDir()/OpenDiablo2/Saves` (`hero_state_factory.go:220-227`), first free integer name (`:234`), `json.MarshalIndent` at 0600 (`:243-258`), scanned by suffix (`:80`); skills round-trip through a shallow ID/points struct (`:205-215`). `HeroStatsState` (`hero_stats_state.go:21-24`) from class records (`:45-52`). `d2stats`/`d2item` are small interfaces with Diablo-2 implementations in `diablo2stats/` and `diablo2item/` (affix rolling). Direct disk reads outside the loader: saves (`hero_state_factory.go:75,175,188,253`), scripts (`d2script/engine.go:62`), `CONTRIBUTORS` for the credits screen (`credits.go:75`, CWD-relative), config (`d2config.go:31`), captures/pprof outputs.

### 4.4 Inventory — two models

See Headline 5. `d2core/d2inventory`: `InventoryItem` interface (`inventory_item.go:6-17`), `CharacterEquipment` paperdoll (`character_equipment.go:4-14`), three item kinds, a per-class starting-gear factory (`inventory_item_factory.go:73`); its real job is feeding the composite's equipment layers (`d2mapentity/factory.go:68-77`) and the save file. The working grid: `d2game/d2player/inventory_grid.go` — `ItemGrid` `:40`, `Add` `:121`, `canFit` `:187`, `SlotToScreen`/`ScreenToSlot` `:83,91`, `ChangeEquippedSlot` `:113` — with the full `EquippedSlot` enum (`inventory.go:167-178`) and hardcoded test contents (`:167-189`). **No pickup.** S1's "`d2inventory` as base" is **PARTIAL**: usable as the save/composite schema; the container must be built or lifted out of `d2player`.

### 4.5 HUD and panels (`d2game/d2player`) — the most finished thing here

Working: HUD, stamina bar (`hud.go:508`), experience bar (`:525`), belt (`:534`), skill icons (`:452,468`), hover labels (`:585`), left/right panel management (`game_controls.go:594,607`), hero stats, inventory, skill tree, party, quest log (cosmetic — label from string table by index, no quest state, `quest_log.go:456`), mini panel, help overlay, escape menu (audio/gamma/contrast options are literal `"TODO"` strings, `escape_menu.go:198-200,215-216`), key bindings, move-gold. Input: left-click moves; shift-left/right-click cast (`game_controls.go:454-484`). Terminal: `freecam`, `setleftskill`, `setrightskill`, `learnskills`, `learnskillid` (`:927-943`).

### 4.6 Scripting (`d2script`)

89 lines over otto + underscore (`engine.go:9-10`). Exposed: `debugPrint` (`:22-25`); server-side `getMapEngines` (`game_server.go:120-126`). `RunScript` (`:61`) — zero call sites; `scripts/test.js` fully commented out; `scripts/server/server.js` one line, never loaded. `Eval` (`:78`) gated by `isEvalAllowed` (`:33`; enabled for Local/LANServer at `game_client.go:107`), consumed only by the `js` terminal command. Main-menu engine field annotated unused (`main_menu.go:173`). S1's "`d2script` for scripted events" is **PARTIAL**: the VM is there; the bindings and the harness are not.

### 4.7 UI toolkits, the label crash, the terminal

`d2ui` (Button, Checkbox, Label, LabelButton, Scrollbar, Sprite, TextBox, Tooltip, WidgetGroup; `ui_manager.go:15`) and `d2gui` (Box, Button, Label, Layout, Scrollbar, Spacers, Sprites; `gui_manager.go:18`, `SetLayout` `:64`, load screen `:163`). The crash fixed at `87fe2aa0`: `d2gui.Label.SetText` measures then allocates a surface of exactly that size (`d2gui/label.go:111-113`); an empty string → 0×0 → `ebiten.NewImage(0,h)` panics on ≥ v2.1; `NewSurface` now clamps to ≥1 (`ebiten_renderer.go:139-155`). `d2term`: `Bind(name, description, arguments, fn)` (`terminal.go:109`); built-ins `ls`, `clear`; registered commands: `dumpheap fullscreen capframe capgifstart capgifstop vsync fps timescale quit js` (`d2app/console_commands.go:18-28`), `spawnitem spawnitemat spawnmon`, `freecam setleftskill setrightskill learnskills learnskillid`, `mapdebugvis entitydebugvis`, `playsoundid playsound activesounds killsounds` (`sound_engine.go:137-152`), `assetspam assetstat assetclear`. **The terminal is the proto-harness** — Phase 3's MCP surface can start by wrapping these.

### 4.8 Audio

WAV only, via ebiten's decoder (`ebiten_audio_provider.go:80,154`); no OGG/MP3 decoder imported — **M5.1's OGG needs ebiten's `audio/vorbis`** (present in the ebiten module; VERIFY). `SoundEngine` plays by ID or handle with pan and loop (`sound_engine.go:124,201,234,71`). `SoundEnvironment` swaps BGM/ambience per area and fires timed one-shots (`sound_environment.go:21,57-68`) — **reading only `DayAmbience`/`DayEvent`** (`:49,62`); `NightAmbience`/`NightEvent` are parsed into records (`sound_environment_loader.go:16,18`) and read by nothing. Two free fields waiting for M4.1.

---

## 5. Dependencies

`go.mod:3` — `go 1.25.0` (raised from 1.16 by `d54a3b24`; the minimum the new graph accepts). 9 direct requires; `go.sum` is 57 lines.

| Dependency | Pinned | Import sites | Note |
|---|---|---|---|
| `hajimehoshi/ebiten/v2` | v2.9.10 | 9 files: render (5), input (1), audio (2), `d2util/debug_print.go:8` | Renderer/audio/input backend; the only backend |
| `google/uuid` | v1.1.2 | 4 (entities, client connections) | |
| `golang.org/x/image` | v0.43.0 | 3 (`colornames`, `bmp`) | |
| `stretchr/testify` | v1.7.0 | 3 test files | 25 other tests use stdlib `testing` |
| `robertkrimen/otto` | ef014fd (2020-09) | 2 | Script VM; pseudo-version |
| `gravestench/akara` | a64208a (2020-10) | 2 | **BitSet only**; pseudo-version |
| `go-restruct/restruct` | v1.2.0-alpha | 1 (`d2pl2`) | Alpha |
| `JoshVarga/blast` | 681c804 (2018-04) | 1 (`d2mpq`) | PKWARE DCL; pseudo-version |
| `pkg/profile` | v1.5.0 | 1 | |

Four of nine direct deps are unversioned pseudo-versions or alphas. M2.5's "one group per commit" list stands; akara's group could be *removal* (a 20-line local bitset) rather than a bump — **decision for Josh (§10)**, since the plan says "stays pinned."

---

## 6. Tests and fixtures

28 `_test.go` files in 20 packages; all green on Josh's machine 2026-08-21. Coverage is a floor, not a net: decoders DC6/COF/DS1/PL2/TBL/AnimData have round-trip tests; **DCC, DT1 (subtile only), MPQ, DAT, TXT, font have none**; nothing above `d2common` is tested except `d2term`, `d2mapentity.Step`, `diablo2stats`, `diablo2item`, and one records index. No test touches the renderer, the loader-to-screen path, or the game loop (the lesson of 20 Aug: build ≠ run ≠ play).

**Fixtures:** `d2animdata/testdata/AnimData.d2` + `BadData.d2` (Blizzard-derived — Headline 6; added `65cce60e` 2020-09-08); `d2loader/testdata/` — `A/ B/ C/` with 2-byte `common.txt`/`exclusive_*.txt` and `D.mpq` (891 B, hand-built, synthesized; added `50d40fb5`). **Done at `f99802d4` (M2.3):** the two `.d2` files are gone from HEAD, `animdata_test.go` synthesizes a full 256-block file in code (plus table-driven bad-data cases and a block-capacity test), and `docs/fixtures-manifest.md` is the canonical rule and list. History still contains the files as inherited upstream content; whether to rewrite history before any friends build is Josh's call, deferred.

---

## 7. Gotchas (seed of `gotchas.md`)

- **`docs/` is the documentation directory, lowercase.** The plan writes `Docs/`; on Josh's case-insensitive Windows checkout a capitalized `Docs/` would alias the existing `docs/` and produce a case-collision on Linux clones. This file lives at `docs/architecture-as-found.md`; all references should use lowercase.
- Windows checkouts must stay LF: `.gitattributes` + repo-local `core.autocrlf=false` (fork commits `0b6388ff`, `8d775bb9`); if `gofmt -l` ever lists the world, check line endings first.
- Raising the `go` directive changes stdlib behaviour (`rand.Seed` is a no-op ≥1.24; `TestRandFunction` rewritten in `d54a3b24`); vet's printf analyzer activates with it.
- ebiten ≥ v2.1 panics on `NewImage` with a non-positive dimension; `NewSurface` clamps (`ebiten_renderer.go:139-155`). Any new surface allocation path must go through `NewSurface`.
- `TicksPerSecond: -1` → `ebiten.SetMaxTPS(-1)` (`defaults.go:18`; `ebiten_renderer.go:84`) — deprecated name, works today; VERIFY semantics per ebiten version before relying on it.
- `filesystem.Source.Exists` is always false (`filesystem/source.go:31-33`); `Loader.Cache` is never used (`loader.go:44,87`); `LoadDS1` uses the DT1 cache (`asset_manager.go:511,525`).
- `d2dat.Load` never errors and panics on short input (`dat.go:21,24`); `d2txt.LoadDataDictionary` panics on a bad header (`data_dictionary.go:28`); `AnimData` load errors fall through to a nil dereference (`initialization.go:154-158`).
- `LoadDT1`/`LoadDS1` prepend `/data/global/tiles/` themselves (`asset_manager.go:492,515`).
- TBL: `x`/`X` keys rewritten to `#index`; duplicates not overwritten (`text_dictionary.go:92-99`).
- TXT rows with first column `Expansion` are silently skipped (`data_dictionary.go:56-57`).
- `itemTypesLoader` must load after weapons/armor/misc (`record_manager.go:193,307-320`).
- The local client still binds TCP `127.0.0.1:6669` (`game_server.go:134,141`) — a second instance or a firewall prompt is a symptom, not a bug in your change.
- The server force-sets spawn position on connect (issue #829, `game_server.go:327-333`); server-closed exits the process (issue #802, `game_client.go:167-172`).
- `.golangci.yml` enables `godox`, which **fails on TODO/FIXME comments** (`.golangci.yml:66`) — the tree's six TODOs (five are literal `"TODO"` strings shown to the user in the escape menu) are not a measure of debt; the lint pushed debt out of comments into issue links and silence.
- `rh.ini` in the repo root was generated by `rh.exe` (Resource Hacker, removed 21 Aug); harmless, untracked; delete at leisure.
- `d2animdata.Load` never increments `hashIdx`, so `hashTable` only ever holds index 0 (`animdata.go:125,154`); nothing reads `hashTable`, so it is harmless — noted, not fixed.

---

## 8. Verdicts on S1's engine assumptions (S1 §12 depends on these)

| S1 assumption | Verdict | Consequence |
|---|---|---|
| (a) `d2core/d2inventory` as the inventory base | **PARTIAL** | Schema yes, container no; the grid is in `d2player` under a second item type. Phase 6 inventory = lift the grid down or build a container |
| (b) `d2script` for scripted events/quests | **PARTIAL** | VM present, zero bindings, never loads scripts. Phase 6's one side-quest needs a binding layer (entities, flags, clock) — size it as new work |
| (c) Act 1 town + wilderness as stand-in geography | **VERIFIED** | Works in the real loop; caveats: Act 1 only, raycast pathing, no entity collision, `default:` branch drops wilderness |
| (d) A surface/light layer a clock could drive | **REFUTED** | No lighting exists. `Surface.PushBrightness`/`PushColor` are the hook; M4.1 is from-scratch work (per-tile or per-pass darkening + light sources). `NightAmbience`/`NightEvent` exist in records for free |
| (e) D2 monster spawn/AI reusable for spawn tables | **REFUTED** | No combat, AI, or monster type; spawn-table fields parsed, unread. M4.3 and M4.5 are new systems on top of loaded *data* — R2 already assumed combat was ours; spawns/AI now are too |

**Plan impact (for Josh, not decided here):** Phase 4's sizing text ("reuses D2's monster systems where they exist") should read "reuses D2's *data*; builds the systems." M4.1 and M4.3 are larger than the plan's phrasing implies; M4.5 is as R2 expected. The clock (M4.4) needs a deterministic world step the loop does not have — which is also what the Phase 3 harness needs, so they should be one design.

---

## 9. Repo hygiene

- **No CI runs.** `.github/workflows/auto-author-assign.yml` only; `.circleci/config.yml` is template-broken (`{{ORG_NAME}}`, line 9) and pins Go 1.16; `.travis.yml` deleted 2021-03-04 (`fbd675c0`). M2.4 should add a GitHub Actions build/vet/test workflow — it is the cheapest net this repo can have.
- **`.golangci.yml`** enables 39 linters incl. removed ones (`deadcode`, `structcheck`, `varcheck`, `golint`, `interfacer`); current golangci-lint will error on the names before linting. `govet` disables `sigchanyzer`, which `d7c88c99` fixed anyway.
- **`dependabot.yml`** watches `github-actions` only.
- **Dead weight:** `rh.exe` 5.5 MB (Resource Hacker; added `fc8191c8` 2019-11-02 as `ResourceHacker.exe`, renamed in `be2b9c63`, last used by the deleted Travis step `./rh.exe -open OpenDiablo2.exe … -res d2logo.ico`); `d2logo.ico` 200 KB and `d2discord.png` 110 KB unreferenced; `docs/` images ~11 MB; `build.sh` (hardcodes Go 1.16, `sudo`s package installs), `tagdev.bat` (force-pushes a `dev` tag). `d2logo.png` is live (`initialization.go:32`).
- **`docs/`**: `status.md` (2021-03-29, announces the AbyssEngine migration — stale, do not follow), `building.md` (GOPATH-era, wrong), `install.md` (Patreon binaries), `purchase.md`/`mpq.md` (still right: classic D2 1.14b), `profiling.md` (still right: `--profile=cpu`), `debug.md`, `development.md`, `faq.md`, `roadmap.md` (a Google Doc link), `index.html` with a Google Analytics tag for `opendiablo2.com`. Keep `purchase.md`, `mpq.md`, `profiling.md`; the rest is upstream history.
- **Fork history** (`7f92c571..HEAD`, all 2026-08-20): `d7c88c99` vet sigchanyzer · `d54a3b24` ebiten v2.0.2→v2.9.10 (+go 1.25.0, oto/v3, printf fixes, `TestRandFunction`) · `0b6388ff` `.gitattributes` LF · `8d775bb9` CRLF renormalize · `1e9ff704` gofmt · `52ca9e84` Strigoi `.gitignore` + starter `CLAUDE.md` · `87fe2aa0` `NewSurface` clamp. Upstream's last engine work was 2021-05-14 (`a688d660`); the two later commits are README edits.

---

## 10. What this pass recommends (decisions for Josh; nothing below is done)

1. **Fixtures (Article V):** ~~replace `AnimData.d2`/`BadData.d2` with synthesized fixtures and remove them from HEAD; start the manifest~~ — **done, `f99802d4`**. Still open: whether history gets rewritten before friends build #1.
2. **`rh.exe`:** ~~remove~~ — **done, `1866ffa7`** (Josh deleted it). `d2logo.ico`, `d2discord.png`, `build.sh`, `tagdev.bat` remain; same reasoning, Josh's call.
3. **akara:** replace the BitSet with a local type and drop the dependency at M2.5, or keep pinned per the plan — a one-line decision either way.
4. **M2.4 hooks:** add a GitHub Actions build/vet/test workflow alongside the write-blocking hooks; fix or delete `.circleci/` and `.golangci.yml`.
5. **Plan v1.4 text:** Phase 4 sizing per §8; `Docs/` → `docs/`; "8 commits" → 7.
6. **Phase 3 + M4.4 share a design:** a deterministic world step (the harness's `step_frames` and the clock's tick are the same thing).

---

## Appendix A — Package table (scripted census, 2026-08-21)

| Package | Files | LOC | In | Out |
|---|---|---|---|---|
| `d2app` | 4 | 1,007 | 1 | 20 |
| `d2common/d2cache` | 3 | 241 | 2 | 1 |
| `d2common/d2calculation` (+`d2lexer` 2/354, `d2parser` 4/809) | 2 | 114 | 2 | 0 |
| `d2common/d2data/d2compression` | 2 | 560 | 1 | 1 |
| `d2common/d2data/d2video` | 2 | 237 | 1 | 1 |
| `d2common/d2datautils` | 9 | 845 | 10 | 0 |
| `d2common/d2enum` | 54 | 1,970 | 26 | 0 |
| `d2common/d2fileformats/d2animdata` | 7 | 717 | 2 | 1 |
| `d2common/d2fileformats/d2cof` | 6 | 380 | 1 | 2 |
| `d2common/d2fileformats/d2dat` | 4 | 162 | 1 | 1 |
| `d2common/d2fileformats/d2dc6` | 6 | 384 | 1 | 1 |
| `d2common/d2fileformats/d2dcc` | 7 | 757 | 1 | 3 |
| `d2common/d2fileformats/d2ds1` | 11 | 2,047 | 4 | 5 |
| `d2common/d2fileformats/d2dt1` | 8 | 680 | 4 | 1 |
| `d2common/d2fileformats/d2font` (+`d2fontglyph` 1/67) | 2 | 216 | 3 | 4 |
| `d2common/d2fileformats/d2mpq` | 9 | 863 | 3 | 3 |
| `d2common/d2fileformats/d2pl2` | 7 | 172 | 1 | 0 |
| `d2common/d2fileformats/d2tbl` | 3 | 324 | 1 | 1 |
| `d2common/d2fileformats/d2txt` | 2 | 94 | 2 | 0 |
| `d2common/d2geom` | 4 | 45 | 7 | 0 |
| `d2common/d2interface` | 17 | 373 | 24 | 3 |
| `d2common/d2loader` (+`asset` 3/37, `asset/types` 3/101, `filesystem` 4/155, `mpq` 3/160) | 3 | 316 | 1 | 8 |
| `d2common/d2math` (+`d2vector` 5/1,387) | 5 | 664 | 12 | 0 |
| `d2common/d2path` | 2 | 11 | 3 | 1 |
| `d2common/d2resource` | 4 | 654 | 11 | 1 |
| `d2common/d2util` (+`assets` 2/65) | 8 | 671 | 23 | 2 |
| `d2core/d2asset` | 7 | 1,807 | 21 | 19 |
| `d2core/d2audio` (+`ebiten` 2/273) | 3 | 340 | 1 | 4 |
| `d2core/d2config` | 4 | 148 | 2 | 0 |
| `d2core/d2gui` | 14 | 2,397 | 4 | 9 |
| `d2core/d2hero` | 6 | 397 | 10 | 4 |
| `d2core/d2input` (+`ebiten` 1/169) | 8 | 445 | 1 | 3 |
| `d2core/d2inventory` | 8 | 397 | 4 | 2 |
| `d2core/d2item` (+`diablo2item` 5/2,318) | 4 | 33 | 1 | 1 |
| `d2core/d2map/d2mapengine` | 4 | 511 | 6 | 10 |
| `d2core/d2map/d2mapentity` | 12 | 1,507 | 6 | 10 |
| `d2core/d2map/d2mapgen` (+`d2wilderness` 1/55) | 3 | 406 | 3 | 8 |
| `d2core/d2map/d2maprenderer` | 5 | 1,205 | 2 | 11 |
| `d2core/d2map/d2mapstamp` | 3 | 267 | 2 | 12 |
| `d2core/d2records` | 173 | 19,654 | 10 | 7 |
| `d2core/d2render/ebiten` | 6 | 657 | 1 | 5 |
| `d2core/d2screen` | 5 | 179 | 2 | 4 |
| `d2core/d2stats` (+`diablo2stats` 8/1,485) | 4 | 92 | 3 | 0 |
| `d2core/d2term` | 6 | 644 | 1 | 4 |
| `d2core/d2ui` | 18 | 3,579 | 7 | 7 |
| `d2game/d2gamescreen` | 10 | 3,629 | 1 | 22 |
| `d2game/d2player` | 23 | 8,679 | 1 | 15 |
| `d2networking` (+`d2client` 3/488, `d2clientconnectiontype` 2/13, `d2localclient` 2/123, `d2remoteclient` 2/235, `d2netpacket` 15/657, `d2netpackettype` 2/79, `d2server` 3/518, `d2tcpclientconnection` 1/60, `d2udpclientconnection` 1/104) | 3 | 80 | 4 | 4 |
| `d2script` | 2 | 92 | 4 | 0 |
| `d2thread` | 1 | 89 | 0 | 0 |
| `utils/extract-mpq` | 2 | 104 | 0 | 2 |

*Census method: parse every `.go` file's import block; count internal imports per package; in-degree = distinct importing packages. Reproduce with a ~40-line script over the tree; the numbers are for `87fe2aa0` only.*
