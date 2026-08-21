# MPQ (archive + loading)

**Decoder:** `d2common/d2fileformats/d2mpq/` (`mpq.go`, `mpq_header.go`, `mpq_hash.go`, `mpq_block.go`, `mpq_stream.go`, `crypto.go`, `mpq_data_stream.go`, `mpq_file_record.go`) · **Tests:** none for `d2mpq` itself; the only coverage is indirect, via `d2common/d2loader/loader_test.go` · **Fixtures:** `d2common/d2loader/testdata/D.mpq` — 891 bytes, hand-built upstream (`docs/fixtures-manifest.md`), the only tracked file matching the .gitignore archive block. There is **no MPQ writer** in the repo (`crypto.go:118` marks `encrypt` `//nolint:unused,deadcode … will use this for creating mpq's`), so a test cannot synthesize an archive; D.mpq must stay a file. Sibling loose-file fixtures: `testdata/{A,B,C}/common.txt` + `exclusive_{a,b,c}.txt`, 2 bytes each.

All citations are to commit `c26dc732`.

## What it holds

Everything Diablo II ships: the TXT data tables, TBL string tables, DC6/DCC sprites, DT1/DS1 map tiles and stamps, COF animations, PL2/DAT palettes, WAV audio, video. The engine reads them only through the loader; no decoder opens an MPQ directly. `utils/extract-mpq/extract-mpq.go:37` is the one standalone consumer (`FromFile` → `Listfile` → per-file extract).

## Structure

**Header** — `Header` struct at `mpq_header.go:10-20`, filled by one `binary.Read(mpq.file, binary.LittleEndian, &mpq.header)` at `mpq_header.go:27` after `Seek(0, io.SeekStart)` (`mpq_header.go:23`). Fixed 32 bytes:

| Off | Size | Field | Notes |
|---|---|---|---|
| 0x00 | 4 | `Magic [4]byte` | must equal `"MPQ\x1A"` else `errors.New("invalid mpq header")` (`mpq_header.go:31-33`) |
| 0x04 | 4 | `HeaderSize` uint32 | never validated; used only as a **seek target** in `loadSingleUnit` (`mpq_stream.go:158`) |
| 0x08 | 4 | `ArchiveSize` uint32 | returned by `Size()` (`mpq.go:179`); otherwise unused |
| 0x0C | 2 | `FormatVersion` uint16 | **never branched on anywhere** — grep finds no reader. v0/v2/v3/v4 archives are all parsed as v1 |
| 0x0E | 2 | `BlockSize` uint16 | sector size = `0x200 << BlockSize` (`mpq_stream.go:40`, `//nolint:gomnd // MPQ magic`) |
| 0x10 | 4 | `HashTableOffset` uint32 | absolute file offset (`mpq_hash.go:21`) |
| 0x14 | 4 | `BlockTableOffset` uint32 | absolute file offset (`mpq_block.go:58`) |
| 0x18 | 4 | `HashTableEntries` uint32 | count, not bytes |
| 0x1C | 4 | `BlockTableEntries` uint32 | count, not bytes |

No 64-bit/HET/BET/extended header fields exist in the struct; bytes past 0x1F are never read.

**Hash table** — 16 bytes/entry (`mpq_hash.go:6-12`): `A` uint32, `B` uint32, then one uint32 split `Locale = hi16`, `Platform = lo16` (`mpq_hash.go:37-38`, carrying the comment link `github.com/OpenDiablo2/OpenDiablo2/issues/812` — the split is the reverse of the conventional locale-low/platform-high order, and **neither field is read by any code**), then `BlockIndex` uint32. The whole table is read through `decryptTable(file, HashTableEntries, "(hash table)")` (`mpq_hash.go:25`).

**Lookup is not a hash-table probe.** Entries are stuffed into `map[uint64]*Hash` keyed by `Name64() = A<<32|B` (`mpq_hash.go:15-17,41`), and a filename is resolved by `hashFilename(name) = hashString(name,1)<<32 | hashString(name,2)` (`crypto.go:99-102`) looked up in that map (`mpq.go:78`, `mpq.go:173`). Slot index, chaining, locale selection and free/deleted markers are all ignored.

**Crypt table** — `cryptoInitialize` (`crypto.go:23-38`) builds `[0x500]uint32` lazily on first `cryptoLookup` (`crypto.go:12-20`, guarded by package globals `cryptoBuffer`/`cryptoBufferReady`, `//nolint:gochecknoglobals`): `seed = 0x00100001`, then for each of `0x100` outer indices, five rounds of `seed = (seed*125 + 3) % 0x2AAAAB`, packing two 16-bit halves into `temp1|temp2` and striding `index2 += 0x100`.

**Hash types** — `hashString(key, hashType)` (`crypto.go:105-116`): seeds `0x7FED7FED` / `0xEEEEEEEE`, iterates `strings.ToUpper(key)`, indexing `cryptoLookup(hashType*0x100 + char)`. Type 1 → name-A, type 2 → name-B, type 3 → table/file key seed. Case handling: **uppercasing inside `hashString` is the only case normalization anywhere in the path**, so lookups within an archive are case-insensitive. Path separator is backslash: `calculateEncryptionSeed` takes the basename via `strings.LastIndex(fileName, "\\")` (`mpq_block.go:51`); a forward-slash path would hash the whole string as one name. The `(listfile)` is just a file whose name hashes like any other (`mpq.go:147`).

**Block table** — `Block` 16 bytes on disk (`mpq_block.go:35-43`): `FilePosition`, `CompressedFileSize`, `UncompressedFileSize`, `Flags`; the struct also carries two host-only fields, `FileName` and `EncryptionSeed`. Read via `decryptTable(file, BlockTableEntries, "(block table)")` (`mpq_block.go:62`).

**Flags** (`mpq_block.go:11-32`): `FileImplode 0x00000100`, `FileCompress 0x00000200`, `FileEncrypted 0x00010000`, `FileFixKey 0x00020000`, `FilePatchFile 0x00100000`, `FileSingleUnit 0x01000000`, `FileDeleteMarker 0x02000000`, `FileSectorCrc 0x04000000`, `FileExists 0x80000000`. Of these only Implode, Compress, Encrypted, FixKey, PatchFile and SingleUnit are ever tested. `FileExists`, `FileDeleteMarker` and `FileSectorCrc` are declared and **never checked** — a deletion marker reads back as a 0/1-byte file, and sector CRCs are neither verified nor skipped.

**Sector layout** — `loadBlockOffsets` (`mpq_stream.go:55-81`) runs only when `(Compress || Implode) && !SingleUnit` (`mpq_stream.go:46`). It seeks to `Block.FilePosition` and reads `blockPositionCount = ((UncompressedFileSize + Size - 1) / Size) + 1` little-endian uint32s — sector end offsets relative to `FilePosition`, one extra for the tail. If encrypted, the table is decrypted with `EncryptionSeed - 1` and sanity-checked: `Positions[0] != blockPositionCount<<2` → `"decryption of MPQ failed"`, and `Positions[1] > Size + blockPosSize` → same error (`mpq_stream.go:68-77`).

## Endianness and packing

Little-endian throughout: the header via `binary.Read(..., binary.LittleEndian, ...)` (`mpq_header.go:27`), the sector offset table via `binary.Read` on a `[]uint32` (`mpq_stream.go:63`), and both tables through `binary.LittleEndian.Uint32` inside `decryptTable` (`crypto.go:88`) and `decryptBytes` (`crypto.go:60`). The header struct is naturally packed (no padding: 4+4+4+2+2+4+4+4+4 = 32). Tables are read as flat `[]uint32` and re-sliced four words per entry (`mpq_hash.go:32`, `mpq_block.go:67`), not as structs.

## How the decoder handles it

- `New(fileName)` (`mpq.go:37`) — opens the file (`openIgnoreCase` on Linux, plain `os.Open` elsewhere, `mpq.go:41-45`) and reads **only** the header. On header failure: `"failed to read reader: %v"` (`mpq.go:52`). Used by `types.CheckSourceType` (`asset/types/source_types.go:40`) to sniff extension-less macOS MPQs.
- `FromFile(fileName)` (`mpq.go:59`) — `New` + `readHashTable` + `readBlockTable`; errors wrap as `"failed to read hash table: %v"` / `"failed to read block table: %v"`.
- `getFileBlockData` (`mpq.go:77`) — map lookup; missing name → `errors.New("file not found")`; `BlockIndex >= len(blocks)` → `"invalid block index"`.
- `ReadFile` (`mpq.go:96`) — sets `Block.FileName = strings.ToLower(fileName)` (the lowercased copy is never used for hashing), creates a stream, allocates `UncompressedFileSize` and reads it in one call. On any error it returns `[]byte{}, err` — **empty slice, not nil**.
- `ReadFileStream` (`mpq.go:118`) — same but wraps in `MpqDataStream`; returns `nil, err` on failure.
- `ReadTextFile` (`mpq.go:135`) — `ReadFile` + `string(...)`.
- `Listfile` (`mpq.go:146`) — `ReadFile("(listfile)")`, `strings.TrimRight(..., "\x00")`, `bufio.Scanner` line split.
- `Contains` (`mpq.go:172`) — map membership only; no flag check.
- `Close` (`mpq.go:91`) — closes the `*os.File`; nothing else is released.
- **No caching.** Every `ReadFile`/`ReadFileStream` builds a new `Stream` and re-reads from disk; the `Stream` keeps only the single most recent decompressed sector (`Stream.Data` + `Stream.Index`, `mpq_stream.go:140-155`).

**Stream reader** (`mpq_stream.go`): `CreateStream` seeds `Index = 0xFFFFFFFF` (`mpq_stream.go:33`, "MPQ magic" — it is a never-valid sector index sentinel), computes `Size`, calls `calculateEncryptionSeed` if `FileFixKey` (seed = `hashString(basename,3) + FilePosition ^ UncompressedFileSize`, `mpq_block.go:53`), and rejects `FilePatchFile` with `"patching is not supported"` (`mpq_stream.go:43`). `Read` dispatches single-unit vs sectored (`mpq_stream.go:84`) and loops until `count` is satisfied or a short read returns 0. `loadBlock` (`mpq_stream.go:178`) picks `offset/toRead` from the sector table when compressed/imploded, else `blockIndex*Size` for `expectedLength` bytes; adds `FilePosition`; decrypts in place with `decryptBytes(data, blockIndex + EncryptionSeed)` when `FileEncrypted && UncompressedFileSize > 3` (a zero seed here → `"unable to determine encryption key"`); then decompresses **only if `toRead != expectedLength`** — a stored (already-full-size) sector is passed through verbatim.

**Linux case fallback** — `openIgnoreCase` (`mpq.go:182-207`): try `os.Open` as given; on failure list the parent directory and pick the first entry matching `strings.EqualFold`. Linux only (`runtime.GOOS == "linux"`, `mpq.go:41`); Windows and macOS get the raw `os.Open`, which is why `initialization.go:63` tells the user "Capitalization in the file name matters."

## Compression algorithms: supported and rejected

`decompressMulti` (`mpq_stream.go:227-284`) switches on the **exact value** of `data[0]` — a bitmask combination not listed below falls through to `fmt.Errorf("decompression not supported for unknown compression type %X", …)` (`mpq_stream.go:283`).

| Byte | Meaning | Result |
|---|---|---|
| `0x01` | Huffman | error `"huffman decompression not supported"` (`:232`) |
| `0x02` | zlib/deflate | `deflate(data[1:])` via stdlib `compress/zlib` (`:234`, `:286`) |
| `0x08` | PKWARE implode | `pkDecompress(data[1:])` via `github.com/JoshVarga/blast` (`:236`, `:309`) |
| `0x10` | BZip2 | error `"bzip2 decompression not supported"` (`:238`) |
| `0x12` | LZMA | error `"lzma decompression not supported"` (`:244`) |
| `0x22` | sparse + zlib | error (`:248`) |
| `0x30` | sparse + bzip2 | error (`:251`) |
| `0x40` | IMA ADPCM mono | `d2compression.WavDecompress(data[1:], 1)` (`:242`) |
| `0x41` | Huffman + ADPCM mono | `WavDecompress(HuffmanDecompress(data[1:]), 1)` (`:253`) |
| `0x48` | PK + wav | error `"pk + mpqwav decompression not supported"` (`:266`) |
| `0x80` | IMA ADPCM stereo | `d2compression.WavDecompress(data[1:], 2)` (`:240`) |
| `0x81` | Huffman + ADPCM stereo | `WavDecompress(HuffmanDecompress(data[1:]), 2)` (`:268`) |
| `0x88` | PK + wav stereo | error `"pk + wav decompression not supported"` (`:280`) |

Huffman is implemented (`d2common/d2data/d2compression/huffman.go`) and reachable only through `0x41`/`0x81`; standalone `0x01` still errors. Sparse and LZMA have no implementation at all. Note `decompressMulti` ignores its `expectedLength` argument entirely (`mpq_stream.go:227`: `/*expectedLength*/ _ uint32`) — output length is never verified.

## Archive precedence and loading

**Source list order at runtime.** `d2app/app.go:272-273` adds two filesystem sources *before* the config is even parsed:

1. `filepath.Dir(d2config.LocalConfigPath())` — the **executable's directory** (`default_directories.go:23-25`, `filepath.Dir(os.Args[0])`).
2. `filepath.Dir(d2config.DefaultConfigPath())` — the **user config dir**, `os.UserConfigDir()/OpenDiablo2` (`default_directories.go:14-20`).

Then `initialize()` → `initConfig` appends the MPQs in `MpqLoadOrder` order, each as `filepath.Join(filepath.Clean(config.MpqPath), mpqName)` (`initialization.go:70-79`). The default list (`d2core/d2config/defaults.go:25-37`) is exactly:

`patch_d2.mpq, d2exp.mpq, d2xmusic.mpq, d2xtalk.mpq, d2xvideo.mpq, d2data.mpq, d2char.mpq, d2music.mpq, d2sfx.mpq, d2video.mpq, d2speech.mpq`

A missing MPQ is fatal, with the multi-line `fmtErrSourceNotFound` message (`initialization.go:57-64`). Default `MpqPath` is per-OS (`defaults.go:23,44,47,50`).

**First hit wins.** `Loader.Load` (`loader.go:76-105`) iterates `l.Sources` in append order and returns the first `source.Open(subPath)` that does not error; every miss logs `"Checked \`%s\`, file not found"` and continues; exhausting the list returns `fmt.Errorf("file not found: %s", subPath)` (`loader.go:23,104`). There is no priority, no patch-chain merge, no delete-marker handling — collisions resolve purely by list position. **"A loose file beside the exe shadows the MPQ" means:** because the exe directory is source #1 and `patch_d2.mpq` is source #3, a file at `<exedir>/data/global/excel/monstats.txt` is opened by the filesystem source first and every archive copy is never consulted for that path.

**Path normalization.** `Load`/`Exists` call `filepath.Clean(subPath)` (`loader.go:77`, `:132`) then substitute tokens `d2resource.LanguageFontToken` and `d2resource.LanguageTableToken` — but **only if `l.language != nil`** (`loader.go:79-85`), i.e. only after `initLanguage` calls `SetLanguage`/`SetCharset` (`initialization.go:86,89`). Inside the MPQ source, `cleanName` (`mpq/source.go:52-60`) replaces `/` with `\` and strips one leading `\`. Case is **never** normalized by the loader — MPQ lookups survive on `strings.ToUpper` inside `hashString`, filesystem lookups are at the host FS's mercy (case-sensitive on Linux). Note `filepath.Clean` is host-flavored: on Windows it already flips `/`→`\`, so an MPQ path is cleaned as a Windows path before `cleanName` sees it.

**Broken filesystem `Exists`** (`filesystem/source.go:31-34`): `_, err := os.Stat(...); return os.IsExist(err)`. `os.Stat` returns `nil` error on success, and `os.IsExist(nil)` is false — so this returns **false for every path**, existing or not. `Loader.Exists` (`loader.go:131`) and therefore `AssetManager.FileExists` (`d2core/d2asset/asset_manager.go:123-129`) can only ever be satisfied by an MPQ source (`mpq/source.go:37-40`, which correctly delegates to `Contains`).

**Unused `Loader.Cache`.** `NewLoader` builds `d2cache.CreateCache(1024*1024*512)` and embeds it (`loader.go:22,44,58`), and `Load` even carries the comment `// if it isn't in the cache, we check if each source…` (`loader.go:87`) — but nothing ever calls `Retrieve` or `Insert` on it. Only `loader_test.go:32` touches it (asserting non-nil). Every `Load` is a fresh disk read.

**The caches that do work** are in `d2core/d2asset/asset_manager.go:69-76`: `dt1s, ds1s, cofs, dccs, animations, fonts, palettes, transforms`, with budgets at `:40-47`. They cache **decoded objects keyed by path**, checked before `LoadFile` (e.g. `LoadDT1`, `:488-503`). Raw bytes are not cached: `LoadFile` → `LoadAsset` → `Loader.Load` + `ioutil.ReadAll` every time (`:90-120`).

## Edge cases the decoder already handles

- Bad magic rejected before any table read (`mpq_header.go:31`).
- Out-of-range `BlockIndex` rejected (`mpq.go:83`).
- `FilePatchFile` refused explicitly rather than silently mis-decoding (`mpq_stream.go:42`).
- Encrypted sector-offset tables get two structural sanity checks after decryption (`mpq_stream.go:71-77`).
- `FileFixKey` position-adjusted keys computed from the basename only (`mpq_block.go:50-54`).
- Stored (uncompressed) sectors inside a compressed file pass through untouched via the `toRead != expectedLength` guard (`mpq_stream.go:211,219`).
- Files ≤ 3 bytes skip decryption (`mpq_stream.go:203`), matching the encryptor's 4-byte word stride (`crypto.go:58`).
- Trailing NULs trimmed from `(listfile)` (`mpq.go:153`).
- Extension-less macOS archives detected by trial `New()` (`asset/types/source_types.go:38-43`).

## Known gotchas

- **`loadSingleUnit` seeks to `header.HeaderSize`, not `Block.FilePosition`** (`mpq_stream.go:158`) and reads `v.Size` (a full sector) rather than `CompressedFileSize` (`:162`). Any single-unit file that is not the first thing after the header decompresses the wrong bytes. This is why `Listfile()` cannot work on D.mpq (see below). **UNEXPLAINED.**
- `MpqDataStream.Seek` computes `Position = offset + whence` (`mpq_data_stream.go:20`) — it *adds the whence constant* instead of interpreting it, so `io.SeekCurrent`/`io.SeekEnd` are silently wrong and `SeekStart` works only by `0` being the constant.
- `Stream.Read` treats `read == 0` as end-of-data and returns `nil` error (`mpq_stream.go:96-98`), so truncated reads look like success.
- Hash-map lookup collapses all empty slots (`A=B=0xFFFFFFFF`) onto one key, and drops locale/platform disambiguation entirely (`mpq_hash.go:41`; issue 812 is cited in-file).
- `cleanName` indexes `name[0]` (`mpq/source.go:55`) — an empty path panics.
- `d2mpq` globals `cryptoBuffer`/`cryptoBufferReady` are lazily initialized without a mutex (`crypto.go:9-20`); `decryptTable` reads `cryptoBuffer` directly (`crypto.go:82`), bypassing `cryptoLookup`'s init guard — it only works because `hashString` on line 74 runs first.
- `nolint` markers cluster where the format is magic: `//nolint:gomnd` on `readHashTable`/`readBlockTable`/all of `crypto.go`, `//nolint:gosec // Will fix later` on all three `os.Open` calls (`mpq.go:44,184,203`), `//nolint:gochecknoglobals` on the crypt buffer, `//nolint:unused,deadcode` on `encrypt`.
- Linux-only case-insensitive fallback (`mpq.go:41`) means the same config works on Linux and fails on Windows/macOS with mis-cased MPQ names.
- `FormatVersion` is parsed and ignored; v2–v4 archives are read as v1.

## Worked example — `d2common/d2loader/testdata/D.mpq`

```
0x00  4d 50 51 1a  d0 00 00 00  7b 03 00 00  03 00 05 00
0x10  ab 02 00 00  2b 03 00 00  08 00 00 00  05 00 00 00
0x20  00 00 00 00  00 00 00 00  00 00 00 00  7b 03 00 00
0x30  00 00 00 00  07 02 00 00  00 00 00 00  c2 01 00 00
```

| Off | Bytes | Field | Value |
|---|---|---|---|
| 0x00 | `4d 50 51 1a` | Magic | `"MPQ\x1A"` — passes `mpq_header.go:31` |
| 0x04 | `d0 00 00 00` | HeaderSize | **208**, not 32 |
| 0x08 | `7b 03 00 00` | ArchiveSize | 891 = exact file size |
| 0x0C | `03 00` | FormatVersion | **3** (= MPQ v4) — never read |
| 0x0E | `05 00` | BlockSize | 5 → sector = `0x200 << 5` = 16384 |
| 0x10 | `ab 02 00 00` | HashTableOffset | 683 |
| 0x14 | `2b 03 00 00` | BlockTableOffset | 811 = 683 + 8×16 ✓ |
| 0x18 | `08 00 00 00` | HashTableEntries | 8 |
| 0x1C | `05 00 00 00` | BlockTableEntries | 5 → 811 + 5×16 = 891 ✓ |

**Contradictions to flag.** Bytes 0x20–0xCF are outside the decoder's struct but are *not* padding: `HeaderSize = 208` with `FormatVersion = 3` describes an MPQ v4 extended header, and the bytes are consistent with one — 0x2C `ArchiveSize64 = 891`, 0x34 `BetTablePos64 = 0x207` (519), 0x3C `HetTablePos64 = 0x1C2` (450), 0x44 `HashTableSize64 = 0x80`, 0x4C `BlockTableSize64 = 0x50`. So the archive carries HET/BET tables at 450/519 that this decoder ignores completely, resolving names through the v1 hash/block tables at 683/811 instead. Nothing breaks because both sets are present and consistent. Second flag: the first file's data starts at offset **208 — exactly `HeaderSize`** — which is what keeps the `loadSingleUnit` bug latent for block 0 and fatal for anything else.

**Contents** (replaying `cryptoInitialize` + `hashString` + `decryptTable` over the fixture): hash slots 0 `common.txt`→block 0, 1 `(listfile)`→block 3, 2 `exclusive_d.txt`→block 1, 3–5 empty (`A=B=0xFFFFFFFF`), 6 `(attributes)`→block 4, 7 `dir\common.txt`→block 2. Blocks: 0/1/2 at 208/234/260, `csize=10 usize=2 flags=0x80000200` (Exists|Compress); 3 `(listfile)` at 286, `csize=40 usize=45`, and 4 `(attributes)` at 342, `csize=92 usize=149`, both `flags=0x81000200` (Exists|SingleUnit|Compress). Nothing is encrypted (`0x00010000` is clear everywhere), so no key derivation runs.

Sector table for block 0 at offset 208: `08 00 00 00 0a 00 00 00` — `Positions[0]=8` (equals `blockPositionCount<<2` = 2<<2), `Positions[1]=10`; payload at 216 is `64 0a` = `"d\n"`. `toRead == expectedLength == 2`, so `mpq_stream.go:211` skips decompression and the raw bytes are returned. Block 3 begins `02 18 95 …` — a `0x02` zlib marker followed by a valid zlib header — but `loadSingleUnit` would seek to 208 and hand `decompressMulti` a leading `0x08`, routing the listfile through PKWARE blast. **`Listfile()` on this fixture cannot work**; no test calls it.

**What `loader_test.go` expects inside it.** Sources are added B(fs) → D.mpq(mpq) → A(fs) → C(fs) (`loader_test.go:72-91`). Constants at `:14-27`: `sourcePathD = "testdata/D.mpq"`, `exclusiveD = "exclusive_d.txt"`, `subdirCommonD = "dir\\common.txt"` (backslash literal in the test source). Assertions: `exclusive_d.txt` loads and its first byte is `"d"` (`:134`); `dir\common.txt` loads from the MPQ and its first byte is `"d"` (`:114,135`); `common.txt` resolves to `"b"` (`:130`) even though D.mpq also holds one — the precedence proof; `a/bad/file/path.txt` must error (`:118`). Separately, `TestLoader_AddSource:43` adds `testdata/D.mpq` as a **filesystem** source and expects success, which passes only because `filesystem.OnAddSource` (`filesystem/loader_provider.go:101-107`) never touches the disk; `:44` adds `/x/y/z.mpq` as an MPQ source and expects the error.
