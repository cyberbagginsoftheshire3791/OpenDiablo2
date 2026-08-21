# Gotchas

Append when a format surprises you. Newest first. Cite the file and line
where the behaviour lives, so it can be re-found when the code moves.
Harvested 2026-08-21 from code comments and git history at c26dc732; entries
marked (harvest) were not observed in play — they are what the code says about itself.
Every commit hash below was verified against `git log` at harvest time.

## 2026-08-21 — AnimData test fixtures were Blizzard-derived (harvest)
**Where:** commit `f99802d4`
**Symptom:** `d2animdata/testdata/AnimData.d2` matched `d2exp.mpq`'s `data\global\animdata.d2` in size but not SHA-256 — a lightly mutated copy of shipped game data sitting in a public GPL repo.
**Cause:** upstream committed them in `65cce60e` (2020-09-08) without provenance.
**Fix / workaround:** decoder tests synthesize their bytes in code (`synthesizeAnimData()` builds a full 256-block file); every remaining `testdata/` file is justified in `docs/fixtures-manifest.md`. Check compliance with `git ls-files`, never by reading `.gitignore`.

## 2026-08-21 — AnimData's hash table only ever holds index 0 (harvest)
**Where:** `d2common/d2fileformats/d2animdata/animdata.go:125` and `:154`
**Symptom:** none in play — nothing reads `hashTable`.
**Cause:** `hashIdx := 0` is declared, `animdata.hashTable[hashIdx] = hashName(name)` runs per record, and `hashIdx` is never incremented. Line dates to `65cce60e` (2020-09-08); observed and logged in `f99802d4`.
**Fix / workaround:** do not trust `hashTable`. If you need name→record lookup use `animdata.entries`, which is keyed correctly.

## 2026-08-20 — ebiten ≥ v2.1 panics on a zero-width surface
**Where:** commit `87fe2aa0`, `d2core/d2render/ebiten/ebiten_renderer.go:140`
**Symptom:** `panic: ebiten: width at NewImage must be positive but 0` during the post-character-select load; process to desktop.
**Cause:** the engine was written against ebiten v2.0.2, which tolerated non-positive dimensions. The escape → key-binding menu lays out an empty-text label, which measures to width 0, and `Renderer.NewSurface(0, h)` reached `ebiten.NewImage`.
**Fix / workaround:** clamp both dimensions to a 1×1 minimum in `NewSurface` — the single dimension-driven `NewImage` call in the tree. Any new renderer backend needs the same tolerance.

## 2026-08-20 — the go directive raise silently changed two runtime contracts
**Where:** commit `d54a3b24`
**Symptom:** `TestRandFunction` passed for the wrong reason; `go vet` newly flagged 8 non-constant format strings.
**Cause:** ebiten v2.9.10 forces `go 1.25.0` in go.mod. At go≥1.24 top-level `rand.Seed` is a no-op, so the seeded-sequence assertion tested nothing; vet's printf analyzer also activates at go≥1.24.
**Fix / workaround:** the test now asserts the actual contract (coin flip between operands). Never lower the go directive to dodge these — one dep bump per commit, and re-read the test after any raise.

## 2026-08-20 — CRLF checkouts make `gofmt -l` flag the whole tree
**Where:** commits `0b6388ff`, `8d775bb9`
**Symptom:** every `.go` file appears unformatted on a Windows machine; diffs are whole-file.
**Cause:** global `core.autocrlf=true` produced a CRLF working tree.
**Fix / workaround:** `.gitattributes` pins LF for Go and text files, repo-local `core.autocrlf=false`, and `8d775bb9` renormalized the already-committed CRLF files. Never commit CRLF.

## 2026-08-20 — modern gofmt inserts a blank `//` before `//nolint`
**Where:** commit `1e9ff704` (28 files)
**Symptom:** a mechanical re-format produces churn in files you did not touch.
**Cause:** the Go 1.27 formatter normalizes doc comments, separating directive lines like `//nolint:funlen` from the doc text above them.
**Fix / workaround:** run the format pass once and commit it alone. When editing near a `//nolint`, keep the directive attached to its declaration — reflowing a comment block can detach it and re-break the build's lint gate.

## 2021-04-07 — DS1 v16+ Marshal wrote the wall count where the floor count belongs
**Where:** commit `e48bc48a`, `d2common/d2fileformats/d2ds1/ds1.go:574`
**Symptom:** a re-encoded v16+ DS1 loads with the wrong number of floor layers, or fails outright.
**Cause:** `if ds1.version.specifiesFloors() { sw.PushInt32(int32(len(ds1.Walls))) }` — copy-paste of the line above it.
**Fix / workaround:** fixed to `len(ds1.Floors)`. Round-trip every version gate you touch; the encoder and decoder are separate code paths that only agree by hand.

## 2021-04-07 — `cullNilLayers` truncated the group and skipped index 0
**Where:** commit `ce7586fa`, `d2common/d2fileformats/d2ds1/ds1_layers.go:74`
**Symptom:** a nil layer in the middle discards every layer after it; a nil at index 0 is never removed.
**Cause:** the loop ran `idx > 0` and did `*group = (*group)[:idx]`.
**Fix / workaround:** now `idx >= 0` with `append((*group)[:idx], (*group)[idx+1:]...)`.

## 2021-03-30 — the DS1 NPC gates are strict `>`, not `>=`
**Where:** `d2common/d2fileformats/d2ds1/ds1_version.go:66-72`
**Symptom:** off-by-one when you reason about the version table: v14 has no NPC block, v15 has NPCs but no NPC actions.
**Cause:** `specifiesNPCs()` is `v > v14` and `specifiesNPCActions()` is `v > v15`, while every neighbouring predicate (`specifiesFloors`, `specifiesAct`) uses `>=`.
**Fix / workaround:** read the predicate, not the constant name. Dated 2021-03-30 (`91209fd5`).

## 2021-03-27 — DS1 `PushWall` used to also push an orientation layer
**Where:** commit `41c1d8e8`, `d2common/d2fileformats/d2ds1/ds1.go:157`
**Symptom:** wall counts and layer indices drift after any insert; the "insert bug".
**Cause:** `loadBody` called `PushWall` *and* `PushOrientation` per wall, and the layer model carried a separate `orientationLayerGroup` with its own max.
**Fix / workaround:** the orientation group was removed entirely; wall orientation rides with the wall layer. If you re-add a parallel layer group, audit every `maxXLayers` constant with it.

## 2021-03-28 — an empty `Font{}` could not Marshal
**Where:** commit `d72b3e33`, `d2common/d2fileformats/d2font/d2fontglyph/font_glyph.go`
**Symptom:** `(&Font{}).Marshal()` produced a file the loader rejected.
**Cause:** the three `unknownN` byte slices were struct fields, so a zero-value glyph wrote empty runs where the format needs fixed constants (1, 3 and 4 bytes, values from d2mods.info thread 42044).
**Fix / workaround:** the unknown bytes became package constants returned by `Unknown1/2/3()`. Related earlier corrections: `75742936`, `a4f12f6e`, `a9ccda18` (argument order in `Create`).

## 2021-03-07 — DAT palette Marshal emitted 256 leading zero bytes
**Where:** commit `7b770119`, `d2common/d2fileformats/d2dat/dat.go:30`
**Symptom:** an encoded palette is 1024 bytes with the real 768 at the end; every colour index is wrong.
**Cause:** `make([]byte, len(p.colors))` followed by `append`.
**Fix / workaround:** `make([]byte, 0)`. This exact `make(len)+append` pattern is still live in `d2dcc` — see below.

## 2021-03-05 — TBL Marshal wrote a signed key count
**Where:** commit `da3fe0ed`, `d2common/d2fileformats/d2tbl/text_dictionary.go:191`
**Symptom:** tables that round-tripped through `Marshal` did not decode identically despite issue #1080 being "fixed".
**Cause:** `PushInt32(int32(len(keys)))` where the format wants an unsigned dword; per-rune `PushBytes` loops also mangled multi-byte values.
**Fix / workaround:** `PushUint32` and `PushBytes([]byte(s)...)`.

## 2021-03-01 — DS1 tile bitfields: encode order must mirror mask order exactly
**Where:** `d2common/d2fileformats/d2ds1/tile.go:8-31`, `:81-88`
**Symptom:** a re-encoded tile decodes into shuffled Prop1/Sequence/Style values.
**Cause:** decode is mask-and-shift (`prop1` at bit 0, `sequence` 8, `unknown1` 14, `style` 20, `unknown2` 26, `hidden` 31); encode is a sequence of `PushBits32` calls whose *order and lengths* (8,6,6,6,5,1 = 32) implicitly reproduce that layout. Nothing checks the two agree.
**Fix / workaround:** change masks and `PushBits32` calls together, and assert a round-trip. Note `unknown1` and `unknown2` are 6 and 5 bits — not byte-aligned. Dated `5dfca5e9`.

## 2021-03-01 — `Logger.Fatal` returned instead of exiting
**Where:** commit `12f9efde`, `d2common/d2util/logger.go:137`
**Symptom:** a fatal decoder error below the configured log level let the process continue with a half-parsed asset.
**Cause:** `os.Exit(1)` sat after the `l.level < LogLevelFatal` early return.
**Fix / workaround:** `defer os.Exit(1)` at the top. `Fatal` now always exits regardless of level.

## 2021-02-28 — TBL keys `x` and `X` are placeholders, rewritten to `#<index>`
**Where:** `d2common/d2fileformats/d2tbl/text_dictionary.go:92-94`, and `:244-246` on the way out
**Symptom:** a string you expect under key `"x"` is missing; the game asks for `"#123"`.
**Cause:** the shipped tables use a literal `x`/`X` for unnamed strings. The loader rewrites the key to `"#" + strconv.Itoa(idx)`, i.e. the *hash-entry index*, and Marshal turns any `#`-prefixed key back into `"x"` (2 bytes: `x` + separator).
**Fix / workaround:** keys starting with `#` are positional and only stable for a given table layout. Commits `4104d9d9`, `de1c0ebe`.

## 2021-02-25 — the TBL version byte is read and discarded
**Where:** commit `c933e4b8`, `d2common/d2fileformats/d2tbl/text_dictionary.go:145-149`
**Symptom:** malformed or foreign TBLs load silently instead of erroring.
**Cause:** the `version != 0` check was deliberately removed so the startup error screen (which needs strings before validation) would work — commit message: "tbl: removed version check (because of error screen; we don't need to check version)".
**Fix / workaround:** none — intentional. If you add validation back, the MPQ-missing error screen is the thing that breaks.

## 2021-02-25 — TBL Marshal opens with two bytes it never explains
**Where:** `d2common/d2fileformats/d2tbl/text_dictionary.go:182-184`
**Symptom:** the encoder writes `0, 0` before anything else.
**Cause:** that's the CRC field the loader skips at `:123` (`crcByteCount = 2`); the encoder does not compute it. Commented with the tracking link `OpenDiablo2/issues/1043`.
**Fix / workaround:** anything that validates the CRC will reject our output. Also `// nolint:gomnd // 17 comes from the size of one "data-header index"` at `:208` — the 17-byte stride is load-bearing for `dataPos`.

## 2021-02-25 — PL2 decoding depends on a restruct beta feature
**Where:** `d2common/d2fileformats/d2pl2/pl2.go:34`, `:47`
**Symptom:** if `restruct.EnableExprBeta()` is not called first, `Unpack`/`Pack` fail or mis-size the fixed arrays.
**Cause:** the PL2 struct leans on restruct's expression evaluation for its many fixed-size transform arrays; the whole 443,175-byte file is one `restruct.Unpack` against `binary.LittleEndian`.
**Fix / workaround:** `Load` and `Marshal` each call `EnableExprBeta()` on every invocation. `Marshal` **panics** on a pack error rather than returning one. Commit `d61d829b`.

## 2021-02-05 — `PushBit` and the byte-oriented Push methods do not compose
**Where:** `d2common/d2datautils/stream_writer.go:36-52`
**Symptom:** bits silently vanish from the output. The comment says so: *"WARNING: if you'll use PushBit, offset'll be less than 8, and if you'll use another Push... method, bits'll not be pushed"*.
**Cause:** `PushBit` accumulates into `bitCache` and only flushes on the 8th bit. `PushBytes`/`PushUint32` write straight to the buffer, bypassing and stranding the cache.
**Fix / workaround:** finish every bit run on a byte boundary before any byte-level push. There is no `Flush`. Commit `9227de34`.

## 2021-02-05 — `PushBits*` is LSB-first and takes bits, not bytes
**Where:** `d2common/d2datautils/stream_writer.go:55-94`
**Symptom:** fields come back bit-reversed, or the writer logs "input bits number must be less (or equal) than N" and writes garbage anyway.
**Cause:** all three variants loop `PushBit(val&1 == 1); val >>= 1` — least-significant bit first, matching `BitMuncher.GetBits`. The over-range guard only `log.Print`s; it does not clamp or return.
**Fix / workaround:** pass a bit count, check it yourself, and read the matching decoder to confirm bit order before trusting a round-trip.

## 2021-02-05 — COF carries 24 bytes nobody has identified
**Where:** `d2common/d2fileformats/d2cof/cof.go:11-13`, `:69-71`, `:95`, `:132`
**Symptom:** a COF built from scratch renders wrong, or an editor drops data on save.
**Cause:** `numUnknownHeaderBytes = 21` (header bytes between the direction count and the speed byte, sliced out as `b[headerNumDirs+1 : headerSpeed]`) plus `numUnknownBodyBytes = 3` at the start of the body. `New()` zero-fills both.
**Fix / workaround:** preserve them verbatim on any load→edit→save path; `Marshal` pushes them back at `:191` and `:193`. Do not assume zeros are safe. Commit `248eaf9d`.

## 2021-01-11 — the stream reader hand-rolls little-endian and never bounds-checks skips
**Where:** `d2common/d2datautils/stream_reader.go:54`, `:71`, `:88-89`, `:125`
**Symptom:** reading a big-endian field returns a plausible but wrong number; a bad length field walks `position` past the end with no error until the *next* read returns `io.EOF`.
**Cause:** every multi-byte read is an explicit LSB-first OR-shift (`uint32(b[0]) | uint32(b[1])<<8 | ...`) — all D2 formats are little-endian, there is no endian switch. `SkipBytes` is a bare `v.position += uint64(count)`.
**Fix / workaround:** validate counts before skipping. The `// nolint` on `ReadUInt32`/`ReadUInt64` (`:64`, `:81`) exists only to silence the shift chain. Commit `87d53181`.

## 2021-01-11 — DC6 never checks its version
**Where:** `d2common/d2fileformats/d2dc6/dc6.go:93-121`
**Symptom:** a non-DC6 or corrupt file parses into nonsense frame counts and then fails deep inside `loadFrames` with `io.EOF`, or allocates enormous slices from a garbage `Directions * FramesPerDirection`.
**Cause:** `loadHeader` reads `Version`, `Flags`, `Encoding` and a 4-byte `Termination` and validates none of them. Contrast DT1, which hard-gates on 7.6.
**Fix / workaround:** sanity-check `Directions`/`FramesPerDirection` before trusting `frameCount` if you feed the DC6 loader untrusted bytes.

## 2021-01-10 — `filesystem.Source.Exists` is always false
**Where:** `d2common/d2loader/filesystem/source.go:31-33`
**Symptom:** loose files on disk are invisible to any `Exists` probe, though `Load` finds them fine.
**Cause:** `_, err := os.Stat(...); return os.IsExist(err)` — `os.Stat` returns a nil error on success, and `os.IsExist(nil)` is false. The correct test is `err == nil` (or `!os.IsNotExist(err)`).
**Fix / workaround:** must be fixed before loose-asset overrides can be relied on. Commit `c99810ad`.

## 2021-01-10 — the file Loader's cache is created and never consulted
**Where:** `d2common/d2loader/loader.go:43` (created), `:74-102` (`Load`)
**Symptom:** every `Load` re-reads and re-decompresses from the MPQ; the 512 MB budget is dead weight.
**Cause:** `Loader` embeds `d2interface.Cache` and allocates it, but `Load` walks `l.Sources` directly. Two comments in `Load`/`Exists` even say *"if it isn't in the cache, we check if each source can open the file"* — describing code that was never written.
**Fix / workaround:** caching happens one layer up, per-format, in `d2asset.AssetManager`. Don't add a second layer without deciding which one owns invalidation.

## 2021-01-10 — `LoadDS1` reads and writes the DT1 cache
**Where:** `d2core/d2asset/asset_manager.go:511`, `:525`
**Symptom:** a DT1 and a DS1 with the same path string collide; the retrieve does an unchecked `.(*d2ds1.DS1)` type assertion on whatever it finds, so a collision panics.
**Cause:** `am.dt1s.Retrieve(ds1Path)` and `am.dt1s.Insert(ds1Path, ds1, ...)` — the `ds1s` cache exists at `:70` and is cleared at `:479`, but is never used.
**Fix / workaround:** both are prefixed `/data/global/tiles/` and the extensions differ, so it does not fire today. Fix before adding any path aliasing.

## 2021-01-08 — the MPQ hash map throws away locale and platform (issue 812)
**Where:** `d2common/d2fileformats/d2mpq/mpq_hash.go:31-41`
**Symptom:** in an archive that stores the same filename for several locales, only one entry survives — whichever the loop hit last.
**Cause:** `mpq.hashes[e.Name64()] = e` keys the map on `A<<32|B` (the filename hash alone). `Locale` and `Platform` are decoded from `hashData[n+2]` and stored on the struct, but are not part of the key and are never compared in `getFileBlockData`. The issue link sits on the decode line.
**Fix / workaround:** localized archives need a composite key. Cited as `OpenDiablo2/issues/812`. Commit `db838145`.

## 2021-01-08 — single-unit MPQ files are read from `header.HeaderSize`, not the block position
**Where:** `d2common/d2fileformats/d2mpq/mpq_stream.go:157-176`
**Symptom:** a `FileSingleUnit` file decompresses to garbage or fails, while every multi-block file in the same archive is fine.
**Cause:** `loadSingleUnit` seeks `int64(v.MPQ.header.HeaderSize)` — the end of the archive header — instead of `v.Block.FilePosition`, which every other path uses (`loadBlock` at `:192-195`, `loadBlockOffsets` at `:56`). It happens to work for the first single-unit file in an archive laid out that way.
**Fix / workaround:** treat any single-unit read failure as this bug first.

## 2021-01-08 — half of MPQ decompression is unimplemented
**Where:** `d2common/d2fileformats/d2mpq/mpq_stream.go:226-283`
**Symptom:** `"huffman decompression not supported"`, `"bzip2..."`, `"lzma..."`, `"sparse decompression + ..."`, or `"decompression not supported for unknown compression type %X"`.
**Cause:** `decompressMulti` implements only zlib (2), PKWARE implode (8, via `github.com/JoshVarga/blast`), IMA ADPCM mono/stereo (0x40/0x80) and the huffman+wav combos (0x41/0x81). Standalone huffman (1) is refused even though `d2compression.HuffmanDecompress` exists.
**Fix / workaround:** the D2 MPQs only use the supported set. A repacked or third-party MPQ can trip this. `FilePatchFile` is also refused outright at `:42-44`.

## 2021-01-08 — the MPQ crypto constants are deliberately unexplained
**Where:** `d2common/d2fileformats/d2mpq/crypto.go:22`, `:40`, `:55`, `:72`, `:104`, `:118`
**Symptom:** you cannot tell a typo from the algorithm.
**Cause:** five `//nolint:gomnd // Decryption magic` markers cover the 0x500-entry buffer init, the `hashString` seeds (`0x7FED7FED`, `0xEEEEEEEE`), and the block cipher rounds. `:118` also carries `//nolint:unused,deadcode` on an encrypt path kept "for creating mpq's". The globals at `:9-10` are `//nolint:gochecknoglobals // will fix later..`.
**Fix / workaround:** treat this file as a transcription of the published MPQ algorithm; verify changes against a real archive, not by reading.

## 2021-01-02 — DCC frame size comes from the direction box, not the frame box
**Where:** commit `826b1224`, `d2core/d2asset/dcc_animation.go:129-137`
**Symptom:** before `39ab8d19` (2020-08-01), frames were sized from `dccFrame.Width/Height` and rendered clipped or misaligned.
**Cause:** every frame in a DCC direction is composited into the direction's union bounding box; the per-frame box is a sub-rectangle. `CreateDCCDirection` computes that union at `d2common/d2fileformats/d2dcc/dcc_direction.go:73-79`.
**Fix / workaround:** use `dccDirection.Box.Width/Height/Left/Top` for the surface, and the frame box only for placement.

## 2020-10-26 — `DCC.Clone` returns twice as many directions, half of them nil
**Where:** `d2common/d2fileformats/d2dcc/dcc.go:66-78`
**Symptom:** a nil-pointer dereference on a cloned DCC's first direction. Still live at HEAD.
**Cause:** `clone.Directions = make([]*DCCDirection, len(d.Directions))` then `clone.Directions = append(clone.Directions, ...)` in the loop — the exact bug fixed for `d2dat` in `7b770119` and for `DC6.Clone` (`d2dc6/dc6.go:266-271`, which now assigns by index). The two `copy(clone.X, d.X)` calls above are also no-ops: `clone := *d` copies the slice headers, so source and destination alias.
**Fix / workaround:** assign by index, and allocate before copying if you want a real deep copy. Commit `ec9c0c3d`.

## 2020-10-26 — `//nolint:gomnd` blankets the decoders
**Where:** `d2common/d2fileformats/d2dcc/dcc_direction.go:49-95`, `d2common/d2fileformats/d2dt1/material.go:18`, `d2common/d2fileformats/d2dt1/subtile.go:68`, `d2common/d2data/d2compression/wav.go:9`, `d2common/d2data/d2video/binkdecoder.go:114`, `d2common/d2datautils/bitstream.go:42`
**Symptom:** a wrong literal reads as intentional; grep for constants finds nothing.
**Cause:** three lint sweeps (`6f8b43f8` #846, `209cc19c` #781, `c938b745`) suppressed `gomnd`/`funlen`/`gocyclo`/`dupl` across every binary decoder rather than naming the values. `d2compression/huffman.go` alone carries seven `//nolint:dupl // it doesnt matter here` markers on the compression-type tables at `:113-177`.
**Fix / workaround:** when a decoder is wrong, read the format spec — the code's own comments will not tell you what a number means.

## 2020-10-25 — animation play length is an unresolved design question (issue 813)
**Where:** `d2core/d2asset/animation.go:57`, `:374`
**Symptom:** `SetPlaySpeed` and `SetPlayLength` interact confusingly; frame timing drifts from what COF speed implies.
**Cause:** the `playLength float64` field and `SetPlayLength` both carry a bare `OpenDiablo2/issues/813` link and no explanation. Related: `d2common/d2enum/npc_action_type.go:7` marks the whole NPC action enum with `issues/811`.
**Fix / workaround:** treat both as unmodelled. Commit `025ee94e`.

## 2020-10-22 — animation speed uses a hardcoded 25 FPS and a 1/256 divisor
**Where:** `d2core/d2asset/composite.go:14-18`, `:150`, `:281`
**Symptom:** everything animates at a rate that is right for D2 and arbitrary for anything else.
**Cause:** `speedUnit = hardcodedFPS(25.0) * hardcodedDivisor(1.0/256.0)`, and `animationSpeed = 1.0 / (float64(speed) * speedUnit)` where `speed` is the COF/AnimData speed byte. The names say "hardcoded" out loud.
**Fix / workaround:** any new content format must either reproduce the 256-unit speed scale or route around `Composite`. Commit `209cc19c`.

## 2020-11-03 — a missing string returns the key instead of panicking
**Where:** `d2core/d2asset/asset_manager.go:310-312`
**Symptom:** raw keys like `#123` or `strChatMessage` appear in the UI instead of text.
**Cause:** the `log.Panicf("Could not find a string for the key '%s'", key)` is commented out — *"Fix to allow v.setDescLabels("#123") to be bypassed for a patch in issue #360. Reenable later."* `TranslateString` returns `key`.
**Fix / workaround:** a visible key in the UI means a table lookup miss, not a formatting bug. Note `int` keys are offset by `am.languageModifier` at `:307`.

## 2020-09-14 — MPQ paths are backslash paths, and `cleanName` panics on empty
**Where:** `d2common/d2loader/mpq/source.go:52-60`
**Symptom:** a forward-slash path silently misses; an empty filename panics with an index-out-of-range.
**Cause:** `cleanName` does `strings.ReplaceAll(name, "/", "\\")` then `if string(name[0]) == "\\" { name = name[1:] }` — unguarded index on a zero-length string. Both `Open` and `Exists` route through it.
**Fix / workaround:** normalize and length-check before calling into the MPQ source. Commit `7f6ae1b7`.

## 2020-09-14 — palette index 0 is transparent, and a bad index nil-derefs
**Where:** `d2common/d2util/palette.go:16-29`
**Symptom:** index-0 pixels are always fully transparent no matter what the palette says; an out-of-range index crashes just after logging.
**Cause:** `// Index zero is hardcoded transparent regardless of palette` then `continue`. Below it, `c, err := palette.GetColor(...)` logs `err` and then calls `c.R()` — but `DATPalette.GetColor` returns `nil, error` (`d2common/d2fileformats/d2dat/dat_palette.go:39-45`).
**Fix / workaround:** never index 0 for a visible colour. Return early on the error. Commit `c66a3d5e` earlier fixed the same call site from bare slice indexing.

## 2020-09-08 — BitMuncher counts in bits from a byte-array base, and `Copy` resets only the counter
**Where:** `d2common/d2datautils/bitmuncher.go:63-69`, `:36-40`, `:113-138`
**Symptom:** sub-streams read from the wrong place if you assume byte offsets.
**Cause:** `offset` is an absolute **bit** index: `GetBit` reads `data[offset/8] >> (offset%8) & 1`, LSB-first within the byte. `CreateBitMuncher(data, offset)` therefore takes bits — note `d2dcc/dcc.go:61` multiplies the file's direction offsets by 8. `Copy()` preserves `offset` and zeroes only `bitsRead`. `MakeSigned` has a documented special case: *"If its a single bit, a value of 1 is -1 automagically"* (`:117`).
**Fix / workaround:** always convert byte offsets to bits at the boundary. Commit `0218cad7`.

## 2020-09-08 — the TXT loader panics on a bad header and silently drops "Expansion" rows
**Where:** `d2common/d2fileformats/d2txt/data_dictionary.go:21-40`, `:56-58`
**Symptom:** a truncated or non-tab-separated `.txt` takes the process down at load; rows whose first column is `Expansion` never appear.
**Cause:** `fieldNames, err := cr.Read(); if err != nil { panic(err) }` — the only error path in the constructor. `Next()` recurses past `Expansion` marker rows. `Number()` swallows `strconv` failures and returns 0.
**Fix / workaround:** validate the file exists and is tab-separated before calling. A silently-zero stat usually means a column-name mismatch in `lookup`, not bad data.

## 2020-08-06 — huffman `insertNode` calls `adjustTree` twice on purpose
**Where:** `d2common/d2data/d2compression/huffman.go:257-261`
**Symptom:** looks like a duplicated line; deleting it corrupts type-0 decompression.
**Cause:** the comment is explicit — *"ISSUE #680: For compression type 0, adjustTree should be called once for every value written and only once here"*. The double call is a stand-in for the missing per-value adjustment.
**Fix / workaround:** leave it. Fixing it properly means threading the write count through `decompress`. Commit `16b8a646`.

## 2020-07-26 — the bink decoder's seek is a comment, not code
**Where:** `d2common/d2data/d2video/binkdecoder.go:94`, `:105`
**Symptom:** `GetNextFrame` reads sequentially and cannot seek; the frame index table is decoded and unused.
**Cause:** `//nolint:gocritic // v.streamReader.SetPosition(uint64(v.FrameIndexTable[i] & 0xFFFFFFFE))` — a lint marker used to park dead code. Below it, `SkipBytes(int(lengthOfAudioPackets) - 4)` with `//nolint:gomnd // decode magic`.
**Fix / workaround:** the bink path is not a working decoder. Commit `7da1843f`.

## 2020-07-05 — `d2dat.Load` can never fail
**Where:** `d2common/d2fileformats/d2dat/dat.go:16-25`
**Symptom:** a truncated palette panics with an index-out-of-range inside the loop instead of returning an error.
**Cause:** the signature returns `(d2interface.Palette, error)` but the body always returns `nil` for the error, and the loop indexes `data[i*3+n]` for a fixed 256 entries with no length check. The offset helpers are an `iota` block (`b,g,r,o = 0,1,2,3`) — the file is **BGR**, not RGB.
**Fix / workaround:** check `len(data) >= 768` before calling. Commit `c1a88c9c`.

## 2020-06-28 — DCC bit widths come from a lookup table, not the raw value
**Where:** `d2common/d2fileformats/d2dcc/dcc_direction.go:50`, `:55-61`
**Symptom:** reading the 4-bit width fields literally gives nonsense frame geometry.
**Cause:** `crazyBitTable = {0,1,2,4,6,8,10,12,14,16,20,24,26,28,30,32}` — each 4-bit header field is an *index* into it. The name is upstream's.
**Fix / workaround:** every `*Bits` field on `DCCDirection` is already the decoded width; do not re-map. Frame cell maths at `dcc_direction_frame.go:58-84` then works in 4-pixel cells with a ragged first and last column. Commit `255ffc75`.

## 2020-06-27 — `MpqDataStream.Seek` adds `whence` to the offset
**Where:** `d2common/d2fileformats/d2mpq/mpq_data_stream.go:19-22`
**Symptom:** `Seek(0, io.SeekCurrent)` lands at byte 1, `Seek(0, io.SeekEnd)` at byte 2; `Seek(n, io.SeekEnd)` reads from the front of the file.
**Cause:** `m.stream.Position = uint32(offset + int64(whence))` — `whence` is an enum (0/1/2), and it is being added as if it were a base offset. Only `io.SeekStart` (0) is accidentally correct.
**Fix / workaround:** callers get away with it because everything seeks `(0, 0)` (see `d2loader/mpq/asset.go:58`, `:72`). Never pass a non-zero whence to an MPQ asset. Commit `490c00b7`.

## 2020-06-24 — DS1 tile filenames carry absolute Windows paths from 1998
**Where:** `d2core/d2map/d2mapengine/engine.go:121-128`
**Symptom:** a DS1's `Files` entries look like `c:\d2\data\global\tiles\act1\town\floor.tg1` and resolve to nothing.
**Cause:** the shipped DS1s embed the level designer's local paths and an obsolete extension. The engine fixes them up with two `strings.ReplaceAll` calls each annotated `// Yes they did...` — strip `c:`, rewrite `.tg1` → `.dt1` — then strips `\d2\data\global\tiles\` and flips separators to `/`.
**Fix / workaround:** any code that reads `ds1.Files` directly must repeat all four rewrites; the fixups live in the map engine, not in the DS1 decoder. Commit `a24c05ef`.

## 2020-06-22 — DT1 hard-gates on version 7.6 and skips 260 header bytes
**Where:** `d2common/d2fileformats/d2dt1/dt1.go:23-24`, `:69-74`
**Symptom:** `"expected to have a version of 7.6, but got %d.%d instead"` on any other DT1.
**Cause:** `knownMajorVersion = 7`, `knownMinorVersion = 6`, checked before anything else; then `br.SkipBytes(numUnknownHeaderBytes)` for 260 undocumented bytes before the tile count and body position. Tiles carry four more unknown runs (4, 4, 7, 12 bytes) and blocks two more — all round-tripped by `Marshal` at `:254-320`.
**Fix / workaround:** the gate is real protection, unlike DC6's absence of one. Preserve the unknown runs on any edit. Commit `5e1725dd`.

## 2020-02-01 — DCC has a header dword that must equal 1
**Where:** `d2common/d2fileformats/d2dcc/dcc.go:43-45`
**Symptom:** `"this value isn't 1. It has to be 1"`.
**Cause:** after the direction and frame counts, the format has a dword nobody has named. The loader asserts it is 1 and gives up otherwise. The signature check above it (`0x74`) is the only other validation.
**Fix / workaround:** if a real DCC trips this, the direction/frame counts above are probably misread — check the byte-vs-bit offset first. Commit `6606774e`.

## 2020-01-31 — HERE BE GIANTS: DCC sub-streams start at a bit offset, not a byte offset
**Where:** `d2common/d2fileformats/d2dcc/dcc_direction.go:106-127`
**Symptom:** DCC pixel data decodes to noise if you treat the five sub-stream sizes as byte counts.
**Cause:** the comment spells it out — *"Because of the way this thing mashes bits together, BIT offset matters here. For example, if you are on byte offset 3, bit offset 6, and the EqualCellsBitstreamSize is 20 bytes, then the next bit stream will be located at byte 23, bit offset 6!"* Each sub-stream is a `CopyBitMuncher` of the parent followed by `SkipBits(size)`.
**Fix / workaround:** never convert these sizes to bytes. `verify()` at `:153-174` re-checks each stream's `BitsRead()` afterwards.

## 2020-01-31 — the DCC decoder panics rather than erroring
**Where:** `d2common/d2fileformats/d2dcc/dcc_direction.go:81-83`, `:159-173`
**Symptom:** `log.Panic("Optional bits in DCC data is not currently supported.")` or four identical `log.Panic("Did not read the correct number of bits!")` — process down, no file name in the message.
**Cause:** `CreateDCCDirection` returns no error, so every failure path is a panic. `OptionalDataBits > 0` is simply unimplemented.
**Fix / workaround:** wrap DCC loads if you care about staying up on a bad asset. A bit-count panic almost always means the sub-stream offsets above drifted. Commit `2461142f`.

## 2020-01-31 — MPQ filename matching is case-insensitive only on Linux
**Where:** `d2common/d2fileformats/d2mpq/mpq.go:41-45`, `:182-205`
**Symptom:** `d2exp.MPQ` opens on Windows and fails on Linux — or the reverse, depending on the disk.
**Cause:** `New` branches on `runtime.GOOS == "linux"` to `openIgnoreCase`, which retries by listing the directory and comparing lowercased names. macOS falls through to a plain `os.Open` and relies on the filesystem being case-insensitive.
**Fix / workaround:** match the MPQ filename case exactly in config on any non-Linux host. Inside an archive, matching is via the hash (`hashFilename` → `hashString`), which **uppercases** the key with `strings.ToUpper` (`crypto.go:110`) — that part is case-insensitive everywhere.
