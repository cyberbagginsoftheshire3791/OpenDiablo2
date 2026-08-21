# Sprites: DC6 and DCC

**Decoders:** `d2common/d2fileformats/d2dc6/` and `d2common/d2fileformats/d2dcc/` · **Tests:** `d2common/d2fileformats/d2dc6/dc6_test.go` only (`TestDC6New`, `TestDC6Unmarshal`, `TestDC6Clone`); d2dcc has no test file at all · **Fixtures:** none on disk (dc6_test builds a `*DC6` struct in code and round-trips it through `Marshal`; dcc has no test)

All citations are to commit `c26dc732`.

## What they hold, and which is used where

The scaffold's pre-filled claim ("DC6 = UI/fonts/icons/statics; DCC = animated units") is **broadly right but imprecise**. Corrected against the callers:

Dispatch is purely by file extension, with no content sniffing: `d2core/d2asset/asset_manager.go:174` switches on `types.Ext2AssetType(filepath.Ext(animationPath))` → `AssetTypeDC6` → `am.loadDC6` (`asset_manager.go:176`), `AssetTypeDCC` → `am.loadDCC` (`asset_manager.go:181`); anything else is an error (`asset_manager.go:186`). The extension table is `d2common/d2loader/asset/types/asset_types.go:37-38`. `loadDC6` (`asset_manager.go:375`) reads the file and calls `d2dc6.Load` with **no cache of the decoded DC6**; `loadDCC` (`asset_manager.go:393`) goes through `am.LoadDCC` (`asset_manager.go:556`), which *does* cache the decoded `*d2dcc.DCC` in `am.dccs`. Both then wrap in an animation (`newDC6Animation` / `newDCCAnimation`) and the resulting animation is cached under `"path;palette;effect"` (`asset_manager.go:159,189`).

**DC6 call paths:**
- All 85 hardcoded UI paths in `d2common/d2resource/resource_paths.go` are `.dc6`; **not one `.dcc` path exists in d2resource**.
- Fonts: `d2core/d2ui/label.go:31` and `d2core/d2gui/layout.go:542` call `LoadFont(base+".tbl", base+".dc6", pal)`; `LoadFont` loads the sprite sheet via `LoadAnimation` (`asset_manager.go:219`) and hands it to `d2font.Font.SetBackground` (`d2common/d2fileformats/d2font/font.go:65`), which then indexes glyphs by frame (`font.go:124`).
- Item ground/flippy graphics: `d2core/d2map/d2mapentity/factory.go:163` (`"%s/%s.DC6"`), and inventory icons `d2game/d2player/inventory_grid.go:25` (`"/data/global/items/inv%s.dc6"`).

**DCC call paths:**
- Missiles: `d2core/d2map/d2mapentity/factory.go:129` (`"%s/%s.dcc"`).
- Cast overlays: `factory.go:231` (`"/data/Global/Overlays/%s.dcc"`).
- Composite units (players, monsters, objects) per COF layer: `d2core/d2asset/composite.go:317`.

**Important correction:** `composite.go:316-319` builds a two-element path list, `.dcc` first then `.dc6`, but the loop at `composite.go:321-325` **returns an error on the first path that does not exist** instead of falling through to the second. So the `.dc6` fallback for composite layers is unreachable — a unit layer that only ships as DC6 fails to load. Composites are therefore DCC-only in practice.

---

## DC6

### Structure

All multi-byte values are **little-endian** (`d2common/d2datautils/stream_reader.go:71`, `stream_writer.go:115-120`). Header, 24 bytes, read in `dc6.go:93-121`:

| Off | Size | Field | Type |
|---|---|---|---|
| 0x00 | 4 | Version | int32 (`dc6.go:96`) |
| 0x04 | 4 | Flags | uint32 (`dc6.go:100`) |
| 0x08 | 4 | Encoding | uint32 (`dc6.go:104`) |
| 0x0C | 4 | Termination | raw bytes, `terminationSize = 4` (`dc6.go:11,108`) |
| 0x10 | 4 | Directions | uint32 (`dc6.go:112`) |
| 0x14 | 4 | FramesPerDirection | uint32 (`dc6.go:116`) |

Then `Directions * FramesPerDirection` uint32 frame pointers (`dc6.go:74-82`). Then that many frames, each (`dc6.go:126-170`): Flipped uint32, Width uint32, Height uint32, OffsetX **int32**, OffsetY **int32**, Unknown uint32, NextBlock uint32, Length uint32 (32 bytes), followed by `Length` bytes of `FrameData` and a 3-byte `Terminator` (`terminatorSize = 3`, `dc6.go:12,165`). Struct: `dc6_frame.go:4-15`.

`dc6_header.go` and `dc6_frame_header.go` define `DC6Header`/`DC6FrameHeader` with `struct:` tags, but **nothing in the repo references either type** — they are dead. `dc6.ksy` is a Kaitai spec that disagrees with the Go code on signedness (it declares `next_block`/`length` as `s4`, and `directions`/`frames_per_dir` as `s4`); the Go decoder is ground truth.

**RLE scanline encoding** (`dc6.go:210-259`). Constants: `endOfScanLine = 0x80`, `maxRunLength = 0x7f` (`dc6.go:8-9`). `scanlineType` (`dc6.go:249`) classifies each command byte `b`:
- `b == 0x80` exactly → `endOfLine`
- `(b & 0x80) > 0` (i.e. 0x81–0xFF) → `runOfTransparentPixels`
- otherwise (0x00–0x7F) → `runOfOpaquePixels`

The decode loop starts at `x = 0, y = Height-1` (`dc6.go:215-216`) — **scanlines are stored bottom-up** and the decoder walks the destination buffer upward so the output is top-down. Per command:
- `endOfLine`: if `y == 0` break out of the loop entirely; else `y--` and `x = 0` (`dc6.go:225-232`).
- `runOfTransparentPixels`: `x += b & 0x7f` — advance without writing, leaving the buffer's zero bytes (`dc6.go:233-235`).
- `runOfOpaquePixels`: copy `b` literal bytes from the stream into `indexData[x + y*Width + i]`, then `x += b` (`dc6.go:236-242`).

### How the decoder handles it

`Load(data []byte) (*DC6, error)` (`dc6.go:52`) → `New()` (`dc6.go:36`) → `Unmarshal` (`dc6.go:64`). Unmarshal reads the header, the pointer table, then `loadFrames` **sequentially** — `FramePointers` are never used to seek (`dc6.go:126` iterates only to get the count). Errors are `io.EOF` from the reader, never format-validation errors.

Pixels are produced lazily and per frame: `DecodeFrame(frameIndex int) []byte` (`dc6.go:211`) returns a fresh `Width*Height` **indexed** byte slice; it is not stored on the `DC6`. `(direction, frame)` flattens as `startFrame := directionIndex * FramesPerDirection; Frames[startFrame+frameIndex]` (`d2core/d2asset/dc6_animation.go:115-117,168-170`) — one flat array, direction-major.

`Marshal()` (`dc6.go:176`) writes the exact inverse byte-for-byte. `Clone()` (`dc6.go:262`) is a shallow copy plus a per-frame struct copy; see gotchas.

Consumption: `DC6Animation.init` sizes `directions[Directions]` × `frames[FramesPerDirection]` (`dc6_animation.go:60-63`); `decodeFrame` copies only Width/Height/OffsetX/OffsetY into the animation frame (`dc6_animation.go:119-124`); the actual surface is built on renderer bind (`dc6_animation.go:32-38,162-182`).

### Frame placement

`OffsetX`/`OffsetY` become `animationFrame.offsetX/offsetY` verbatim (`dc6_animation.go:122-123`) and are applied as a plain translation before blitting: `target.PushTranslation(frame.offsetX, frame.offsetY)` (`d2core/d2asset/animation.go:169`, and identically in `RenderSection` at `animation.go:232`). Shadows halve Y: `PushTranslation(frame.offsetX, int(float64(frame.offsetY)*half))` (`animation.go:136`).

DC6 animations are additionally created with `originAtBottom: true` (`dc6_animation.go:30`) — the only place this is set in the repo — so `RenderFromOrigin` pushes an extra `PushTranslation(0, -frame.height)` (`animation.go:199-205`) before rendering. DCC animations do not set it (`dcc_animation.go:27-39`).

The **flipped** flag is decoded into `DC6Frame.Flipped` (`dc6.go:129`) and re-emitted by `Marshal` (`dc6.go:195`), but it is **never read** — no flip is applied anywhere in the engine.

### Palette and transparency

Yes — `DecodeFrame` yields palette **indices**, one byte per pixel. Conversion to RGBA happens in `d2common/d2util/palette.go:11` `ImgIndexToRGBA`, called from `dc6_animation.go:171` (and `dcc_animation.go:188`, and three sites in `d2core/d2map/d2maprenderer/tile_cache.go`). Index 0 is transparent, decided in exactly one place:

`d2common/d2util/palette.go:17` — `if indexData[i] == 0 { continue }`, with the comment "Index zero is hardcoded transparent regardless of palette". The output slice was zero-filled, so index-0 pixels get RGBA `0,0,0,0`. The renderer plays no part in this decision. Non-zero indices take `A()` from the palette entry (`palette.go:29`).

### Edge cases and gotchas

- **No version check.** `Version` is read and never compared to anything. Neither is `Encoding` — the decoder assumes RLE unconditionally. `Termination` is read blindly (`dc6.go:108`) and the per-frame 3-byte `Terminator` likewise (`dc6.go:165`); neither is validated. A non-DC6 file is only rejected if it happens to run out of bytes.
- **`Unknown`** (`dc6_frame.go:10`, read at `dc6.go:149`) is round-tripped and otherwise unused. `NextBlock` is also read/written and never used to seek.
- **`DecodeFrame` has no bounds checks.** `indexData[x+y*Width+i]` (`dc6.go:238`) can run past the row into the next scanline, or panic past the buffer, on a corrupt run length; `frame.FrameData[offset]` (`dc6.go:221`) panics if the data ends before a `0x80` arrives with `y == 0`. The only loop exit is that condition (`dc6.go:225-229`) — end-of-data is not a terminator.
- **`Clone` is not a deep copy** (`dc6.go:262-274`). `clone := *d` copies slice headers, so `copy(clone.Termination, d.Termination)` and `copy(clone.FramePointers, ...)` copy each slice onto *itself* — no-ops. Termination and FramePointers stay shared with the original, and the cloned frames' `FrameData`/`Terminator` slices are shared too. Since `LoadAnimationWithEffect` returns `Clone()` on every cache hit (`asset_manager.go:162`), all users of one DC6 path share this state.
- **`DC6Animation.SetDirection` indexes with the unmapped index** (`dc6_animation.go:78-84`): it computes `direction := d2dcc.Dir64ToDcc(directionIndex, len(a.directions))` but then tests `a.directions[directionIndex].decoded`. For a 1-direction DC6 asked for direction 5, that is an out-of-range panic. `DCCAnimation.SetDirection` gets this right (`dcc_animation.go:87-88`).

---

## DCC

### Structure

The whole file is read through a `BitMuncher` (`d2common/d2datautils/bitmuncher.go`), whose `offset` is measured in **bits** (`bitmuncher.go:64`: `v.data[v.offset/byteLen] >> uint(v.offset%byteLen)`) and which reads LSB-first, so byte-aligned multi-byte reads come out little-endian.

`Load` (`dcc.go:24-57`), reading from bit 0:
- `Signature = GetByte()`; must equal `dccFileSignature = 0x74` or `Load` errors (`dcc.go:9,31-35`).
- `Version = GetByte()` (`dcc.go:37`) — read, never checked.
- `NumberOfDirections = GetByte()` (`dcc.go:38`).
- `FramesPerDirection = GetInt32()` (`dcc.go:39`).
- **`dcc.go:43-45`**: `if bm.GetInt32() != 1 { return nil, errors.New("this value isn't 1. It has to be 1") }`. Exactly one signed 32-bit LE value is consumed and discarded after comparison; it is not stored in any field and the code gives it no name. **UNEXPLAINED** — the source states no reason why it must be 1.
- `dcc.go:47`: `bm.GetInt32() // TotalSizeCoded` — a second int32 read and thrown away; the only description is that trailing comment. The scaffold called this "final DC6 length"; the code does not — do not assert a meaning. **UNEXPLAINED**.
- Then, per direction i: `directionOffsets[i] = int(bm.GetInt32())` immediately followed by `Directions[i] = decodeDirection(i)` (`dcc.go:51-54`). The nested decode uses its own BitMuncher, so `bm` keeps walking the offset table.

`directionOffsetMultiplier = 8` (`dcc.go:10`) is applied at `dcc.go:62`: `CreateBitMuncher(d.fileData, d.directionOffsets[direction]*directionOffsetMultiplier)`. Because BitMuncher offsets are bit offsets, **the stored direction offsets are byte offsets from the start of the file**, multiplied by 8 to convert to bits.

Header size: 15 bytes fixed + 4 bytes per direction.

### Direction header

`CreateDCCDirection` (`dcc_direction.go:48-63`) reads, in order: `OutSizeCoded = GetUInt32()` (32 bits), `CompressionFlags = GetBits(2)`, then **seven 4-bit indices**, each looked up in a width table before being stored — `Variable0Bits`, `WidthBits`, `HeightBits`, `XOffsetBits`, `YOffsetBits`, `OptionalDataBits`, `CodedBytesBits`. Total 62 bits. The table (`dcc_direction.go:50`, named `crazyBitTable` in the source):

```go
var crazyBitTable = []byte{0, 1, 2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 26, 28, 30, 32}
```

**Frame headers** follow immediately, `FramesPerDirection` of them, still in the same bitstream (`dcc_direction.go:71-77` → `CreateDCCDirectionFrame`, `dcc_direction_frame.go:28-55`), each: `GetBits(Variable0Bits)` discarded; `Width = GetBits(WidthBits)`; `Height = GetBits(HeightBits)`; `XOffset = GetSignedBits(XOffsetBits)`; `YOffset = GetSignedBits(YOffsetBits)`; `NumberOfOptionalBytes = GetBits(OptionalDataBits)`; `NumberOfCodedBytes = GetBits(CodedBytesBits)`; `FrameIsBottomUp = GetBit() == 1`. Bottom-up frames `log.Panic` (`dcc_direction_frame.go:41-42`). Top-down box: `Left = XOffset`, `Top = YOffset - Height + 1`, `Width`, `Height` (`dcc_direction_frame.go:44-49`).

**Direction bounding box** (`dcc_direction.go:65-79`): min of all frames' `Box.Left`/`Box.Top`, max of `Box.Right()`/`Box.Bottom()` (`d2common/d2geom/rectangle.go`), seeded with `baseMinx/baseMiny = 100000`, `baseMaxx/baseMaxy = -100000` (`dcc_direction.go:13-17` — magnitude **UNEXPLAINED**). Result: `Box = {minx, miny, maxx-minx, maxy-miny}`.

Then: `OptionalDataBits > 0` → `log.Panic("Optional bits in DCC data is not currently supported.")` (`dcc_direction.go:81-83`); `CompressionFlags & 0x2` → `EqualCellsBitstreamSize = GetBits(20)` (`:86-88`); `PixelMaskBitstreamSize = GetBits(20)` **unconditionally** (`:90`); `CompressionFlags & 0x1` → `EncodingTypeBitsreamSize = GetBits(20)` and `RawPixelCodesBitstreamSize = GetBits(20)` (`:93-96`). The 20-bit width and the 0x1/0x2 flag bits are **UNEXPLAINED**.

Then 256 single bits: for `i` in 0..255, if the bit is set, `PaletteEntries[count++] = byte(i)` (`dcc_direction.go:98-104`) — a compaction table mapping a per-direction dense code to a real 8-bit palette index.

### The decompression scheme, step by step

**Carving the five bitstreams** (`dcc_direction.go:106-127`). Each is a *copy* of the muncher at the current bit position (`CopyBitMuncher` resets `bitsRead` but keeps `offset`, `bitmuncher.go:37-40`), followed by `bm.SkipBits(size)`. Order: equal-cells, pixel-mask, encoding-type, raw-pixel-codes, and finally pixel-code-and-displacement, which gets no declared size and simply runs to the end. The in-source comment "HERE BE GIANTS" (`:106-110`) records that these boundaries are at arbitrary **bit** offsets, not byte-aligned.

**The cell model.** `cellsPerRow = 4` (`dcc_direction.go:19`) — the name is misleading; it is the cell edge length in pixels, i.e. 4×4 cells. `calculateCells()` (`:414-463`) tiles the direction box: `HorizontalCellCount = 1 + (Box.Width-1)/4`, `VerticalCellCount = 1 + (Box.Height-1)/4`; all cells are 4 wide/tall except the last in each axis, which takes the remainder (`:428,441`); offsets step by 4 (`:458,461`). `frame.recalculateCells(dir)` (`dcc_direction_frame.go:57-133`) tiles each *frame* against that grid, but the first column/row is short: `w = 4 - ((frame.Box.Left - dir.Box.Left) % 4)`, `h = 4 - ((frame.Box.Top - dir.Box.Top) % 4)` (`:59,74`), so frame cells align to the direction grid. Cell counts: 1 if `Width - w <= 1`, else `2 + (Width-w-1)/4`, decremented when `(Width-w-1) % 4 == 0` (`:61-71`, same for vertical `:76-86`). `DCCCell` is `{Width, Height, XOffset, YOffset, LastWidth, LastHeight, LastXOffset, LastYOffset}` (`dcc_cell.go`).

**Pass 1 — `fillPixelBuffer`** (`dcc_direction.go:280-412`). The pixel-mask lookup table is:

```go
var pixelMaskLookup = []int{0, 1, 1, 2, 1, 2, 2, 3, 1, 2, 2, 3, 2, 3, 3, 4}
```

(`dcc_direction.go:281`) — indexed by the 4-bit mask, its value is the number of set bits in that index.

The buffer is sized `maxCellX*maxCellY` where both are **sums** of every frame's cell counts (`:287-296`) — an over-allocation, despite the `max` names. All entries start `Frame = -1, FrameCellIndex = -1` (`:298-301`). `cellBuffer` holds one pointer per direction-grid cell (`:303`).

For each frame, `originCellX = (frame.Box.Left - v.Box.Left)/4`, `originCellY = (frame.Box.Top - v.Box.Top)/4` (`:312-313`); for each frame cell, `currentCell = originCellX + cellX + currentCellY*v.HorizontalCellCount` (`:319`).

- If that grid slot was already written by an earlier frame (`cellBuffer[currentCell] != nil`): read one bit from the equal-cells stream if `EqualCellsBitstreamSize > 0`, else treat as 0 (`:324-328`). Bit 0 → read `pixelMask = pm.GetBits(4)` (`:331`). Bit 1 → `continue`, emitting **no** pixel-buffer entry for this cell (`:333,339-341`).
- First touch of a slot: `pixelMask = 0x0F` (`:336`), no stream reads.

Decoding the up-to-4 palette codes (`:344-378`): `numberOfPixelBits = pixelMaskLookup[pixelMask]`; `encodingType = et.GetBit()` only when `numberOfPixelBits != 0 && EncodingTypeBitsreamSize > 0`, else 0 (`:350-354`). Per code: if `encodingType != 0`, `pixelStack[i] = rp.GetBits(8)` — a raw 8-bit code. Otherwise a delta: start from `lastPixel`, add a 4-bit displacement from the pixel-code-and-displacement stream, and **keep adding further 4-bit nibbles while the nibble reads exactly 15** (`:363-368`) — 15 is the escape/continue value. If a decoded value equals `lastPixel`, it is zeroed and the loop breaks early (`:371-373`); otherwise `lastPixel` advances and `decodedPixel++`.

Filling the entry (`:380-401`): walking `i` 0..3, a set bit `i` in `pixelMask` consumes `pixelStack[curIdx]` with `curIdx` starting at `decodedPixel-1` and **counting down** (so the stack is reversed), or 0 once exhausted; a clear bit inherits `oldEntry.Value[i]` — the previous entry for that same grid cell. Then `cellBuffer[currentCell]` points at the new entry, tagged with `Frame = frameIndex` and `FrameCellIndex = cellX + cellY*frame.HorizontalCellCount` (`:399-401`).

Finally every entry's four values are remapped through the direction's palette table: `Value[x] = PaletteEntries[Value[x]]` (`:407-411`).

**Pass 2 — `generateFrames`** (`dcc_direction.go:177-277`). All direction cells get `LastWidth = LastHeight = -1` (`:181-183`). `v.PixelData` is a direction-sized scratch canvas, `Box.Width*Box.Height` (`:185`); each frame allocates its own canvas of the **same** size (`:191`). `pbIdx` walks the pixel buffer in order. For each frame cell, the owning grid cell is `cellX = cell.XOffset/4`, `cellY = cell.YOffset/4`, `cellIndex = cellX + cellY*HorizontalCellCount` (`:197-200`).

- If `pbe.Frame != frameIndex || pbe.FrameCellIndex != c` (`:203`) the cell was skipped by an equal-cells bit: if its size differs from the grid cell's last size, the region on the scratch canvas is zeroed (`:205-211`) and nothing is copied into the frame (which is already zero); if the sizes match, the previous cell's pixels are blitted from `bufferCell.LastXOffset/LastYOffset` to the new position on the scratch canvas, then blitted again into the frame canvas (`:214-230`). `pbIdx` is **not** advanced.
- Otherwise the entry belongs here: if `Value[0] == Value[1]` the whole cell is filled with `Value[0]` (`:233-239`); else `bitsToRead = 1`, or 2 when `Value[1] != Value[2]` (`:242-245`), and every pixel reads that many bits from the pixel-code-and-displacement stream as an index into `Value[0..3]` (`:246-251`). The cell is then blitted into the frame canvas and `pbIdx++` (`:255-261`).

Either way the grid cell records `LastWidth/LastHeight/LastXOffset/LastYOffset` from this frame's cell (`:264-267`). Frame cells are freed as they go (`:271`), and `Cells`, `PixelData`, `PixelBuffer` are nulled at the end (`:274-276`, plus `:143`).

`verify` then panics unless each of the four sized streams read exactly its declared bit count (`:153-174`), and `bm.SkipBits(pcd.BitsRead())` advances the outer muncher (`:148`).

### How the decoder handles it

Entry point `d2dcc.Load(fileData []byte) (*DCC, error)` (`dcc.go:24`). Unlike DC6, **decoding is eager and total** — every direction and every frame is fully expanded to indexed pixels during `Load`. Produced type: `DCCDirectionFrame.PixelData []byte`, one byte of palette index per pixel, sized `direction.Box.Width * direction.Box.Height` for *every* frame in that direction.

`d2core/d2asset/dcc_animation.go` consumes it: `decodeFrame` takes width/height/offset from the **direction** box, not the frame (`dcc_animation.go:132-138`) — so all frames in a DCC direction share dimensions and offset. `createFrameSurface` asserts `len(indexData) == width*height` and errors "pixel data incorrect" otherwise (`dcc_animation.go:184-186`), then runs `ImgIndexToRGBA` with the same index-0-transparent rule as DC6 (`dcc_animation.go:188`).

### Edge cases and gotchas

- **UNEXPLAINED magic numbers**, each with only a `//nolint:gomnd // binary data` marker: `0x74` signature (`dcc.go:9`); the `!= 1` check (`dcc.go:43`); the discarded "TotalSizeCoded" int32 (`dcc.go:47`); the 16 entries of `crazyBitTable` (`dcc_direction.go:50`); `CompressionFlags` bits `0x1`/`0x2` (`:86,93`); the 20-bit stream-size widths (`:87,90,94,95`); the 256-bit palette map (`:98`); displacement escape value 15 (`:365`); `pixelMaskLookup` (`:281`); `baseMinx`… = ±100000 (`:13-17`); the 4-pixel cell size (`:19`).
- **Optional bytes are not supported and not skipped.** The guard at `dcc_direction.go:81` tests `OptionalDataBits` (the *bit width* field) and `log.Panic`s — it hard-crashes the process rather than returning an error. `NumberOfOptionalBytes` is decoded per frame (`dcc_direction_frame.go:37`) and then never read, so even if the panic were removed nothing would consume the optional block. Likewise `OutSizeCoded` (`dcc_direction.go:53`) and `NumberOfCodedBytes` (`dcc_direction_frame.go:38`) are stored and never used.
- **Three panic paths, no error return:** optional bits (`dcc_direction.go:82`), bottom-up frames (`dcc_direction_frame.go:42`), and the four `verify` checks (`dcc_direction.go:160,164,168,172`). A malformed DCC takes the process down; only the signature failure produces a clean `error`.
- **Direction index mapping** — `Dir64ToDcc(direction, numDirections int) int` (`dcc_dir_lookup.go:15-60`) holds five hardcoded 64-entry tables (`dir4`, `dir8`, `dir16`, `dir32`, `dir64`) and switches on `numDirections` against `four/eight/sixteen/thirtyTwo/sixtyFour` (= 4/8/16/32/64, `dcc_dir_lookup.go:6-11`). **Anything not in that set — including 1 and 2 directions — falls to `default: return 0`** (`:57-59`). So a 1-direction asset always renders direction 0. The input `direction` is a 0..63 index; callers guard only the upper bound (`dcc_animation.go:82-84`, `animation.go:308-310`). It is used by `DCCAnimation.SetDirection` (`dcc_animation.go:87`), by the base `Animation.SetDirection` (`animation.go:313`), and — despite living in the DCC package — by `DC6Animation.SetDirection` (`dc6_animation.go:78`). Composites use a separate `d2cof.Dir64ToCof` (`composite.go:68`).
- **`DCC.Clone` is broken** (`dcc.go:66-78`): it allocates `make([]*DCCDirection, len(d.Directions))` and then **appends** to it, producing a slice of length `2N` whose first `N` entries are `nil`. `copy(clone.directionOffsets, ...)` and `copy(clone.fileData, ...)` are self-copies (no-ops). It is called on every animation cache hit via `DCCAnimation.Clone` (`dcc_animation.go:74`); it survives only because the copied `Animation` retains the original's `onBindRenderer` closure, which captures the *original* `DCCAnimation` (`dcc_animation.go:31-38`), and `Animation.Clone`'s `copy(clone.directions, a.directions)` (`animation.go:407`) is also a self-copy, leaving the clone aliasing the original's decoded directions and surfaces.

---

## Known gotchas (both)

1. `Animation.Clone` (`animation.go:405-409`) and both format `Clone`s are shallow; `LoadAnimationWithEffect` returns a clone on every cache hit (`asset_manager.go:162`), so every consumer of a given sprite path shares frame surfaces and, for DC6, frame data. Play state (`frameIndex`, `playMode`) *is* independent because those are value fields.
2. Index 0 is transparent for both formats, decided only at `d2common/d2util/palette.go:17`. A sprite that legitimately wants palette entry 0 cannot express it.
3. Surfaces are built lazily on `BindRenderer` (`dc6_animation.go:32`, `dcc_animation.go:31`), and `Animation.Render` self-binds from the target if unbound (`animation.go:162-164`). Errors during bind are only `log.Println`'d (`animation.go:188-190`).
4. Neither decoder validates a version field.
5. Both formats' offsets flow into the same `PushTranslation(offsetX, offsetY)` (`animation.go:169`), but DC6 adds `originAtBottom` (`dc6_animation.go:30` → `animation.go:199-205`) and DCC does not; DCC bakes vertical placement into `Box.Top = YOffset - Height + 1` instead.
6. `composite.go:321-325` returns early instead of trying the second candidate path — the DC6 fallback for composite layers is dead code.

## Worked example

**DC6.** `dc6_test.go:15-41` builds a `*DC6` **struct**, not bytes; `TestDC6Unmarshal` (`:43`) round-trips it through `Marshal` → `Load`. Annotating the marshaled output (73 bytes total):

| Off | Bytes | Field | Value |
|---|---|---|---|
| 0x00 | `06 00 00 00` | Version | 6 |
| 0x04 | `01 00 00 00` | Flags | 1 |
| 0x08 | `00 00 00 00` | Encoding | 0 |
| 0x0C | `EE EE EE EE` | Termination | 238×4 |
| 0x10 | `01 00 00 00` | Directions | 1 |
| 0x14 | `01 00 00 00` | FramesPerDirection | 1 |
| 0x18 | `38 00 00 00` | FramePointers[0] | 56 |
| 0x1C | `00 00 00 00` | Flipped | 0 |
| 0x20 | `20 00 00 00` | Width | 32 |
| 0x24 | `1A 00 00 00` | Height | 26 |
| 0x28 | `2D 00 00 00` | OffsetX | 45 |
| 0x2C | `18 00 00 00` | OffsetY | 24 |
| 0x30 | `00 00 00 00` | Unknown | 0 |
| 0x34 | `32 00 00 00` | NextBlock | 50 |
| 0x38 | `0A 00 00 00` | Length | 10 |
| 0x3C | `02 17 22 80 35 40 27 2B 7B 0C` | FrameData | 10 bytes |
| 0x46 | `02 08 05` | Terminator | 3 bytes |

The frame actually begins at 0x1C, so the fixture's FramePointer of 56 (0x38) is arbitrary — harmless because the decoder never seeks by it. The pixel data is also not decodable: tracing `DecodeFrame` from `x=0, y=25`, byte `02` writes 2 literals, `80` ends the line, then `35` (53) demands 53 more literal bytes from a 10-byte buffer and would panic. **No test calls `DecodeFrame`** — the RLE loop and the palette path are entirely untested.

**DCC.** No fixture exists anywhere in the repo and there is no `d2dcc` test file. Header byte layout (`dcc.go:31-52`), byte-aligned because it starts at bit 0:

| Off | Size | Field | Notes |
|---|---|---|---|
| 0x00 | 1 | Signature | must be `0x74` |
| 0x01 | 1 | Version | unchecked |
| 0x02 | 1 | NumberOfDirections | |
| 0x03 | 4 | FramesPerDirection | int32 LE |
| 0x07 | 4 | (unnamed) | must equal 1 — UNEXPLAINED |
| 0x0B | 4 | (unnamed) | discarded; comment says "TotalSizeCoded" — UNEXPLAINED |
| 0x0F + 4·i | 4 | directionOffsets[i] | int32 LE **byte** offset from file start; ×8 to get the bit offset |

Everything after is bit-packed and unaligned: at `directionOffsets[i]*8` sits the 62-bit direction header (32-bit OutSizeCoded, 2-bit CompressionFlags, 7×4-bit width-table indices), then the frame headers, then the conditional 20-bit stream sizes, the 256-bit palette map, and the five bitstreams back to back.
