# Maps: DS1 and DT1

**Decoders:** `d2common/d2fileformats/d2ds1/` and `d2common/d2fileformats/d2dt1/` · **Tests:** `d2ds1/ds1_test.go`, `d2ds1/ds1_layers_test.go`, `d2ds1/layer_test.go`, `d2dt1/subtile_test.go` · **Fixtures:** none on disk (tests build their bytes in code) · **Consumers:** `d2core/d2map/d2mapstamp`, `d2core/d2map/d2mapengine`, `d2core/d2map/d2maprenderer` (plus `d2core/d2asset/asset_manager.go:487,510` as the loading front door and `d2core/d2map/d2mapgen` as the only generator).

All citations are to commit `c26dc732`.

## What they hold

A **DT1** is a tile *atlas*: an array of tiles, each with palette-indexed graphics split into blocks, plus 25 sub-tile flag bytes describing walkability/LOS per 1/25th of the tile. A **DS1** is a *stamp*: a grid of cells, where each cell names tiles by (style, sequence, type) across several layers — floors, walls, shadows, substitutions — plus an object list, substitution groups, and NPC waypoint paths. DS1 holds no pixels; DT1 holds no map layout. `d2ds1/doc.go:1` and `d2dt1/doc.go:1` (which cites `https://d2mods.info/forum/viewtopic.php?t=65163`).

## DT1 — structure

**Header** (`d2dt1/dt1.go:60-85`), 276 bytes total:

| Offset | Size | Field |
|---|---|---|
| 0 | 4 | `majorVersion` int32 |
| 4 | 4 | `minorVersion` int32 |
| 8 | 260 | skipped — `numUnknownHeaderBytes = 260` (`dt1.go:22`), UNEXPLAINED |
| 268 | 4 | `numberOfTiles` int32 |
| 272 | 4 | `bodyPosition` int32 — absolute file offset of the tile-header array |

Version gate (`dt1.go:70-73`), with `knownMajorVersion = 7`, `knownMinorVersion = 6` (`dt1.go:23-24`):

```go
return nil, fmt.Errorf(fmtErr, result.majorVersion, result.minorVersion) // "expected to have a version of 7.6, but got %d.%d instead"
```

Anything but exactly 7.6 is rejected. `New()` (`dt1.go:42-49`) stamps 7.6 into fresh DT1s.

**Per-tile header** — 96 bytes, read in order at `dt1.go:94-184`, seek to `bodyPosition` first (`dt1.go:87`):

| Off | Size | Field (`d2dt1/tile.go:4-19`) |
|---|---|---|
| 0 | 4 | `Direction` int32 |
| 4 | 2 | `RoofHeight` int16 |
| 6 | 2 | material flags uint16 → `NewMaterialFlags` (`material.go:19`); the code calls this "sound index" nowhere — it decodes to Other/Water/WoodObject/InsideStone/OutsideStone/Dirt/Sand/Wood/Lava/Snow at bits 0x0001..0x0400 (0x0200 unused, UNEXPLAINED) |
| 8 | 4 | `Height` int32 (negative for walls; `AbsInt32` at `tile_cache.go:100`) |
| 12 | 4 | `Width` int32 |
| 16 | 4 | skip, `numUnknownTileBytes1 = 4` (`dt1.go:25`) UNEXPLAINED |
| 20 | 4 | `Type` int32 — the orientation (see TileType below) |
| 24 | 4 | `Style` int32 — "main index" |
| 28 | 4 | `Sequence` int32 — "sub index" |
| 32 | 4 | `RarityFrameIndex` int32 — rarity for statics, frame index for animated (see `tile_cache.go:81-85`) |
| 36 | 4 | `unknown2` — retained verbatim for round-trip (`dt1.go:145`, `dt1.go:270`) UNEXPLAINED |
| 40 | 25 | `SubTileFlags [25]SubTileFlags`, one byte each (`dt1.go:150-159`) |
| 65 | 7 | skip, `numUnknownTileBytes3 = 7` UNEXPLAINED |
| 72 | 4 | `blockHeaderPointer` int32 |
| 76 | 4 | `blockHeaderSize` int32 (read, never used after) |
| 80 | 4 | block count → `tile.Blocks = make([]Block, numBlocks)` (`dt1.go:180`) |
| 84 | 12 | skip, `numUnknownTileBytes4 = 12` UNEXPLAINED |

**Block headers** — 20 bytes each, read in a second pass after seeking to `blockHeaderPointer` (`dt1.go:187-230`): `X` int16, `Y` int16, 2 skipped bytes (`numUnknownBlockBytes = 2`, UNEXPLAINED), `GridX` byte, `GridY` byte, `format` int16, `Length` int32, 2 more skipped bytes, `FileOffset` int32. Encoded bytes are then read from `blockHeaderPointer + FileOffset` for `Length` bytes (`dt1.go:232-241`) — the offset is **relative to the tile's block-header pointer**, not the file.

`Block.Format()` (`block.go:16-22`): `format == 1` → `BlockFormatIsometric`, anything else → `BlockFormatRLE` (`dt1.go:15-18`).

**Decoding** (`d2dt1/gfx_decode.go:8`, `DecodeTileGfxData(blocks, pixels, tileYOffset, tileWidth)`), writing palette indices into a flat byte buffer:

- *3D isometric* (`gfx_decode.go:11-36`): fixed 256-byte payload (`blockDataLength = 256`, `gfx_decode.go:4`). Two hard-coded tables drive a 15-row diamond: `xjump := []int32{14, 12, 10, 8, 6, 4, 2, 0, 2, 4, 6, 8, 10, 12, 14}` and `nbpix := []int32{4, 8, 12, 16, 20, 24, 28, 32, 28, 24, 20, 16, 12, 8, 4}` (sums to exactly 256). Row `y` starts at `x = xjump[y]` and copies `nbpix[y]` consecutive source bytes to `((blockY + y + tileYOffset) * tileWidth) + (blockX + x)`. No transparency: every byte is written.
- *RLE* (`gfx_decode.go:38-69`): loop while `length > 0`. Read two control bytes `b1` (skip count) and `b2` (run length), `length -= 2`. If `(b1 | b2) == 0`, that's an end-of-row marker: `x = 0; y++`, continue. Otherwise `x += b1` (transparent skip — nothing written), `length -= b2`, then copy `b2` literal bytes one at a time to the same offset formula, advancing `x` and the source index.

**Sub-tile flags** (`d2dt1/subtile.go:69-80`) — one byte, LSB first:

| Bit | Value | Name (verbatim) |
|---|---|---|
| 0 | `data&1` | `BlockWalk` |
| 1 | `data&2` | `BlockLOS` (line of sight) |
| 2 | `data&4` | `BlockJump` |
| 3 | `data&8` | `BlockPlayerWalk` |
| 4 | `data&16` | `Unknown1` UNEXPLAINED |
| 5 | `data&32` | `BlockLight` |
| 6 | `data&64` | `Unknown2` UNEXPLAINED |
| 7 | `data&128` | `Unknown3` UNEXPLAINED |

`Combine` ORs two sets together (`subtile.go:16-25`) — that is how stacked layers accumulate collision. `Encode` (`subtile.go:83`) is the exact inverse. Only `BlockWalk` is ever read by game code.

## DS1 — structure

Entry: `d2ds1.Unmarshal(fileData)` → `loadHeader` then `loadBody` (`ds1.go:49-68`).

**Header** (`ds1.go:70-124`):

1. `version` int32 (`ds1.go:75`).
2. `width` int32, `height` int32, then **`width++; height++`** (`ds1.go:92-93`) — the file stores size-1; `Marshal` writes `width - 1` back (`ds1.go:545-546`).
3. Act, if `specifiesAct()` — `return v >= v8` (`ds1_version.go:29-32`). Clamped: `ds1.Act = d2math.MinInt32(d2enum.ActsNumber, ds1.Act+1)` (`ds1.go:103`), `ActsNumber = 5` (`d2enum/quests.go:11`).
4. `SubstitutionType` int32, if `specifiesSubstitutionType()` — `return v >= v10` (`ds1_version.go:34-37`). Values 1 or 2 (`subType1`/`subType2`, `ds1.go:14-15`) push one substitution layer (`ds1.go:112-116`).
5. File list, if `hasFileList()` — `return v >= v3` (`ds1_version.go:54-57`): an int32 count, then that many NUL-terminated strings built byte-by-byte (`ds1.go:203-225`).

**Body** (`ds1.go:126-195`):

- 8 unknown bytes if `hasUnknown1Bytes()` — `return v >= v9 && v <= v13` (`ds1_version.go:20-23`), `unknown1BytesCount = 8` (`ds1.go:26`). UNEXPLAINED (source comment: "just after the header will be some meaningless (?) bytes").
- Defaults before reading: `defaultNumFloors = 1`, `defaultNumShadows = maxShadowLayers` (=1), `defaultNumSubstitutions = 0` (`ds1.go:43-45`).
- Wall count int32 if `specifiesWalls()` — `return v >= v4` (`ds1_version.go:44-47`); **nested inside it**, floor count int32 if `specifiesFloors()` — `return v >= v16` (`ds1_version.go:49-52`). A v16+ file that somehow lacked walls would never read the floor count (`ds1.go:144-158`).
- Layers are pushed in the order walls → shadows → floors → substitutions (`ds1.go:160-174`), capped by `maxWallLayers = 4`, `maxFloorLayers = 2`, `maxShadowLayers = 1`, `maxSubstitutionLayers = 1` (`ds1_layers.go:4-7`); `push` silently drops anything over the cap (`ds1_layers.go:153-155`).

**Layer stream order** — `getLayerSchema()` (`ds1.go:342-391`). If `hasStandardLayers()` (`return v < v4`, `ds1_version.go:39-42`) the order is fixed: Wall1, Floor1, Orientation1, Substitute1, Shadow1. Otherwise walls and orientations **interleave** — for each wall `i`: `layerStreamWall1 + i` then `layerStreamOrientation1 + i` — then all floors, then one shadow if present, then one substitution if present. `layerStreamType` ordering is `Wall1..Wall4, Orientation1..Orientation4, Floor1, Floor2, Shadow1, Substitute1` (`layer.go:8-20`). `ds1_test.go:224-247` pins the interleaving for 2 walls + 1 floor + 1 shadow: `{Wall1, Orientation1, Wall2, Orientation2, Floor1, Shadow1}`.

Each layer is one uint32 per cell, row-major, `y` outer / `x` inner (`ds1.go:492-528`).

**Per-cell record** — walls, floors and shadows share the identical bit layout (`tile.go:72-79` `DecodeWall`, `tile.go:91-98` `decodeFloorShadow`, reached via `DecodeFloor`/`DecodeShadow`):

| Field | Mask | Shift | Bits |
|---|---|---|---|
| `Prop1` | `0x000000FF` | 0 | 8 |
| `Sequence` | `0x00003F00` | 8 | 6 |
| `Unknown1` | `0x000FC000` | 14 | 6 — UNEXPLAINED |
| `Style` | `0x03F00000` | 20 | 6 |
| `Unknown2` | `0x7C000000` | 26 | 5 — UNEXPLAINED |
| `HiddenBytes` | `0x80000000` | 31 | 1 |

(`tile.go:9-32`.) `Hidden()` is `HiddenBytes > 0` (`tile.go:67`). Note the field names: `Style` is the DT1 "main index", `Sequence` the "sub index"; `Prop1` is used only as a non-zero "this cell has content" test by the renderer (`renderer.go:362,368,374`).

Orientation cells are decoded separately (`ds1.go:504-517`): `c := int32(dw & wallTypeBitmask)` with `wallTypeBitmask = 0x000000FF` becomes `tile.Type`, and `tile.Zero = byte((dw & wallZeroBitmask) >> wallZeroOffset)` with `wallZeroBitmask = 0xFFFFFF00`, `wallZeroOffset = 8` (`ds1.go:19-21`).

Substitution cells store the raw dword: `ds1.Substitutions[0].Tile(x, y).Substitution = dw` (`ds1.go:524`); the struct comment marks it `// unknown` (`tile.go:50`).

**Objects** — if `hasObjects()` (`return v >= v3`, `ds1_version.go:58-60`): int32 count then five int32s per object — Type, ID, X, Y, Flags (`ds1.go:236-278`). Type 1 is `ObjectTypeCharacter`, and `Flags` is never read.

**Substitution groups** — gated on `hasSubstitutions()` (`return v >= v12`, `ds1_version.go:62-64`) **and** `SubstitutionType` being 1 or 2 (`ds1.go:286-287`). A uint32 `unknown2` precedes the list if `hasUnknown2Bytes()` (`return v >= v18`, `ds1_version.go:25-27`). Then a count and five int32s per group: TileX, TileY, WidthInTiles, HeightInTiles, Unknown (`substitution_group.go:4-10`, `ds1.go:308-337`).

**NPC paths** — if `specifiesNPCs()` (`return v > v14`, `ds1_version.go:66-68`, i.e. v15+): int32 NPC count; per NPC, `numPaths`, `npcX`, `npcY` int32s (`ds1.go:400-419`). The NPC is matched to an object by exact X/Y equality (`ds1.go:423-429`); if no object matches, the path block is skipped as `numPaths * 3` dwords when `specifiesNPCActions()` else `numPaths * 2` (`ds1.go:437-441`) — `specifiesNPCActions()` is `return v > v15` (`ds1_version.go:70-72`, i.e. v16+). Each path point is X int32, Y int32, plus Action int32 only when `specifiesNPCActions()` (`ds1.go:456-475`); positions become `d2vector.NewPosition` in **sub-tile** units.

**Version list, confirmed against `ds1_version.go:5-18`:** the scaffold's claim "DS1 has several versions with differing layer counts" is correct but understates it — versions also differ in header fields and trailing records. The declared constants are exactly `v3, v4, v7, v8, v9, v10, v12, v13, v14, v15, v16, v18`. There is no v5, v6, v11, or v17 constant (v7 appears only as the `dirLookup` cutoff, v13 only as the upper bound of `hasUnknown1Bytes`), and comparisons are `<`/`>=` so intermediate numbers behave as their nearest lower predicate. The test data uses `version: 17` (`ds1_test.go:178`), a value with no named constant.

## Endianness and packing

Everything multi-byte is **little-endian**: `ReadUInt32` is `uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24` (`d2common/d2datautils/stream_reader.go:66-68`), and `PushUint32` writes the same order (`stream_writer.go:113-118`). Structures are packed with no alignment padding — the decoders read field-by-field from a byte cursor, so the only "padding" is the explicitly skipped unknown runs. DS1 cell records are bit-packed within a single little-endian dword; the encoder writes them with `PushBits32`, which emits **LSB-first** one bit at a time (`stream_writer.go:79-90`, via `PushBit` at `stream_writer.go:38-51`), so the six `EncodeWall` pushes (8+6+6+6+5+1 = 32 bits) land exactly on the decode masks.

## How a DS1 cell references a DT1 tile

The DS1 cell carries `(Style, Sequence, Type)`; DT1 tiles carry the same triple as `(Style, Sequence, Type)` (`d2dt1/tile.go:11-13`). Lookup is a **linear scan collecting all matches**: `MapEngine.GetTiles(style, sequence, tileType)` walks `m.dt1TileData` and returns every tile whose three fields match, `nil` + a `"Unknown tile ID [%d %d %d]"` warning if none (`engine.go:246-264`). `GetTileData` adds a fourth match on `RarityFrameIndex` (`engine.go:328-337`). `Stamp.TileData` is the stamp-local equivalent (`stamp.go:96-105`).

Choosing among candidates happens once, at placement time, in `MapTile.PrepareTile` (`map_tile.go:31-81`): for each wall it calls `GetTiles(Style, Sequence, wall.Type)`, for each floor `GetTiles(Style, Sequence, 0)`, for each shadow `GetTiles(Style, Sequence, 13)` (a literal, = `d2enum.TileShadow`), then `wall.RandomIndex = getRandomTile(options, x, y, me.seed)` and ORs the chosen tile's 25 sub-tile flags into the MapTile via `Combine` (`map_tile.go:42-44`). Lava floors bypass the random pick: `floor.Animated = true; floor.RandomIndex = 0` (`map_tile.go:55-58`).

`getRandomTile` (`map_tile.go:85-123`) is a seeded weighted pick, labelled "Walker's Alias Method … with xorshifting": `tileSeed = uint64(seed) + uint64(x); tileSeed *= uint64(y)`, then xorshift 13/17/5, `random := tileSeed % uint64(weightSum)` where `weightSum` is the sum of `RarityFrameIndex` over the candidates, and it returns the first index where the running sum `>= random`. Weight sum 0 returns index 0.

**Pre-v7 orientation remap** (`ds1.go:484-513`): a 25-entry table

```go
dirLookup := []int32{0x00, 0x01, 0x02, 0x01, 0x02, 0x03, 0x03, 0x05, 0x05, 0x06, ...}
```

is applied — `if ds1.version < v7 { if c < int32(len(dirLookup)) { c = dirLookup[c] } }` — only to the orientation stream, remapping old direction codes into `d2enum.TileType` space. Values ≥ 25 pass through unchanged. UNEXPLAINED which historical encoding it maps from.

**Special tiles**: `TileSpecialTile1` (10) and `TileSpecialTile2` (11) (`d2enum/tile.go:18-19`) are matched by `Type.Special()` (`d2enum/tile.go:64-71`). `MapEngine.GetStartPosition` scans every tile for a wall with `Type.Special() && Style == 30` and returns that tile centre `+0.5, +0.5` in **tile** units, falling back to the map centre (`engine.go:268-281`). Style 30 as "player start" is UNEXPLAINED — a bare magic number. The debug visualiser prints `s: style-sequence` for special walls (`renderer.go:583-587`); nothing else consumes them.

## Layer model

Four groups, each a `[]*Layer` of tile grids (`ds1_layers.go:39-45`), with `LayerGroupType` = Floor/Wall/Shadow/Substitution (`ds1_layers.go:14-19`) and caps 2/4/1/1 (`GetMaxGroupLen`, `ds1_layers.go:355-370`).

**Orientation values** (`d2enum/tile.go:7-28`) are the wall `Type`: 0 `TileFloor`; 1 `TileLeftWall`; 2 `TileRightWall`; 3/4 the right/left halves of a north corner wall; 5 `TileLeftEndWall`; 6 `TileRightEndWall`; 7 `TileSouthCornerWall`; 8/9 left/right wall with door; 10/11 special; 12 pillars/columns/standalone objects; 13 `TileShadow`; 14 `TileTree`; 15 `TileRoof`; 16-19 the "lower walls equivalent to" left / right / right-left north corner / south corner.

The renderer makes **four passes** (`renderer.go:155-197`, doc comment verbatim):

1. `renderPass1` (`renderer.go:231`) → `renderTilePass1` (`renderer.go:360-378`): walls where `!wall.Hidden() && wall.Prop1 != 0 && wall.Type.LowerWall()` (types 16-19, `d2enum/tile.go:31-41`), then floors, then shadows — each guarded on `!Hidden() && Prop1 != 0`.
2. `renderPass2` (`renderer.go:243`): entities with `GetLayer() != 1` filtered out — i.e. layer 1 entities only — iterated per sub-tile 5×5 in `SubTileOffset` order.
3. `renderPass3` (`renderer.go:295`) → `renderTilePass2` (`renderer.go:380-386`): walls where `!wall.Hidden() && wall.Type.UpperWall()` (types 1-9, 12, 14 — `d2enum/tile.go:44-61`; note `Prop1` is **not** checked here), then all other entities.
4. `renderPass4` (`renderer.go:349`) → `renderTilePass3` (`renderer.go:388-394`): only `wall.Type == d2enum.TileRoof`, no hidden check at all.

Substitution layers are decoded and copied into the stamp tile (`stamp.go:88-90`) and counted by `TileExists` (`engine.go:311`) but are **never rendered and never prepared** — `PrepareTile` has no substitution branch.

Y placement comes from the tile cache: shadows get `tile.YAdjust = int(tileMinY + shadowAdjustY)` with `shadowAdjustY = 80` (`tile_cache.go:13,138`); roofs get `-int(tileData.RoofHeight)`; other walls `int(tileMinY) + tileSurfaceHeight` where `tileSurfaceHeight = 80` (`tile_cache.go:21,194-198`). Walls always render into a `tileSurfaceWidth = 160`-wide surface (`tile_cache.go:20,210-214`); `TileRightPartOfNorthCornerWall` additionally composites the matching `TileLeftPartOfNorthCornerWall` graphic into the same buffer (`tile_cache.go:169-174, 216-218`). Shadows render at 62.5% alpha: `color.RGBA{R: 255, G: 255, B: 255, A: 160}` (`renderer.go:446`).

## Isometric coordinate convention

One world unit = one map tile. `WorldToOrtho` (`viewport.go:84-89`) with `tileWidth = 80`, `tileHeight = 40` (`viewport.go:18-19`):

```go
orthoX = (x - y) * tileWidth; orthoY = (x + y) * tileHeight
```

So a tile diamond is 160 px wide and 80 px tall on screen; 80/40 are the *half*-extents. The inverse `OrthoToWorld` (`viewport.go:76-81`) hard-codes the same numbers rather than the constants: `worldX = (x/80 + y/40) / half`, `worldY = (y/40 - x/80) / half`. Ortho → screen just subtracts the camera and adds the viewport rect origin (`viewport.go:101-107`), with the camera offset itself pre-shifted by half the screen rect (`viewport.go:184-194`) — so the camera position is the screen centre, and world (0,0) sits at the *top vertex* of tile 0,0's diamond. `WorldToScreen` is the composition (`viewport.go:61-63`); `MapRenderer` re-exports all three (`renderer.go:215-228, 539-546`).

Sub-tiles: 5×5 per tile (`subtilesPerTile = 5` in `d2mapengine/engine.go:45`, `d2mapstamp/stamp.go:17`, `d2maprenderer/renderer.go:46`, `d2mapentity/factory.go:20`, and `subTilesPerTile float64 = 5` in `d2vector/position.go:11`). A sub-tile is `orthoSubTileWidth = 16` by `orthoSubTileHeight = 8` (`renderer.go:47-48`) — matching `subtileWidth = 16`, `subtileHeight = 8` in `d2mapentity/animated_entity.go:23-24`.

Entity positions are stored **in sub-tile units**: "Position is a vector in world space. The stored value is the sub tile position" (`d2vector/position.go:19`). `World()` divides by 5, `Tile()` floors that, `SubTileOffset()` is the remainder, and `RenderOffset()` is `SubTileOffset() + 1` — commented as placing the vector at the bottom vertex of the sub-tile diamond because indices increase down-right and down-left (`position.go:57-79`). Entities draw at `translateSubtile(ox-oy, ox+oy)` = `(int(fx*16), int(fy*8) - (-5))` — the `subtileOffsetY = -5` nudge is UNEXPLAINED (`animated_entity.go:25, 32-34, 37-42`). Objects placed from a DS1 use `(tileOffsetX*5)+object.X` — object X/Y in the file are already sub-tile offsets within the stamp (`stamp.go:120,143-144`, `convertPaths` at `stamp.go:157-167`).

Tiles are stored flat, `x + (y * width)` (`engine.go:198-200`).

## Walkability and collision

Walkability lives entirely in the DT1 sub-tile flags. `MapTile.SubTiles [25]d2dt1.SubTileFlags` (`map_tile.go:14`) is filled by `Combine`-ing every rendered layer's flags in `PrepareTile`. Indexing is *bottom-up*: `GetSubTileFlags(x, y)` uses `subtileLookup = [5][5]int{{20,21,22,23,24},{15,...},{10,...},{5,...},{0,1,2,3,4}}` (`map_tile.go:19-27`), so row 0 maps to indices 20-24. `MapEngine.SubTileAt(subX, subY)` = `TileAt(subX/5, subY/5).GetSubTileFlags(subX%5, subY%5)` (`engine.go:203-207`).

There is **no pathfinder**. `MapEngine.PathFind` (`pathfind.go:10-16`) returns a single point: the result of `checkLos`. `checkLos` (`pathfind.go:19-48`) is a DDA raycast — step count `N = max(|dx|, |dy|)`, unit step `1/N` — that samples `m.SubTileAt(floor(x), floor(y)).BlockWalk` each step and returns the last unblocked position on a hit, else the destination. Only `BlockWalk` is consulted anywhere in the engine; `BlockLOS`, `BlockJump`, `BlockPlayerWalk` and `BlockLight` are decoded and never read. There is no entity-vs-entity collision at all. The debug overlay draws a 5×5 red dot grid from the same flag (`renderer.go:590-603`).

## How the decoders handle it (entry points, types, allocation)

- `d2dt1.LoadDT1([]byte) (*DT1, error)` (`dt1.go:54`) — three sequential passes (tile headers, block headers, block payloads) over one `StreamReader`, seeking absolutely. `DT1.Marshal() []byte` (`dt1.go:248`) reconstructs, zero-filling every unknown run via `unknownHeaderBytes()`/`unknown1()`/`unknown3()`/`unknown4()` (`dt1.go:320`, `tile.go:21-31`) and sorting blocks by `blockHeaderPointer` (`dt1.go:283-294`).
- `d2ds1.Unmarshal([]byte) (*DS1, error)` (`ds1.go:49`), or the method form for reuse (`ds1.go:54`); `DS1.Marshal()` at `ds1.go:539`. Layers allocate via `Layer.SetSize` → `SetWidth`/`SetHeight`, which copy the existing grid into a fresh one (`layer.go:60-130`).
- `AssetManager.LoadDT1` / `LoadDS1` (`d2core/d2asset/asset_manager.go:487,510`) prepend `/data/global/tiles/` and cache the result.
- `DecodeTileGfxData` (`gfx_decode.go:8`) writes into a caller-allocated `[]byte` of size `width * height`; the caller then maps indices through the act palette (`tile_cache.go:103-107`).

## Edge cases the decoders already handle

- Version gate on DT1 with a specific error string (`dt1.go:70-73`).
- Every read is error-checked and wrapped with context (`fmt.Errorf("reading X: %w", err)`) throughout `ds1.go`.
- The `width++/height++` convention and its inverse on marshal (`ds1.go:92-93`, `545-546`).
- Act clamped to `ActsNumber` (`ds1.go:103`).
- Missing NPC-to-object match skips the right number of bytes for the version (`ds1.go:437-441`).
- Layer caps enforced on push and insert (`ds1_layers.go:153, 213`); nil layers culled before every operation (`ds1_layers.go:66-93`).
- DS1 file-list path fixups for shipped data: `"c:"` stripped, `.tg1` → `.dt1`, `\d2\data\global\tiles\` stripped, backslashes normalised — with the source comment "Yes they did..." (`engine.go:121-128`). Filenames `""` and `"0"` are treated as absent (`engine.go:88`, `factory.go:52`).
- Lava floors are animated across all rarity frames instead of one random pick (`map_tile.go:55-58`, `tile_cache.go:68-85`), cycling 10 frames at 0.1 s (`renderer.go:607-625`).

## Known gotchas

- **`Layer.SetTile` is a no-op for every valid coordinate.** `if l.Width() > x || l.Height() > y { return }` (`layer.go:43`) — the comparison is inverted; it returns early precisely when in bounds.
- **`Layer.Tile` bounds check is off by one** — `if l.Width() < x || l.Height() < y` (`layer.go:34`) admits `x == Width()` and panics on the index.
- **`Layer.Width()` panics on a zero-value Layer** — it reads `l.tiles[0]` before checking `len(l.tiles)` (`layer.go:52`).
- **DS1 v3 and earlier panic.** `hasStandardLayers()` (v < 4) returns a schema starting with `layerStreamWall1`, but `specifiesWalls()` (v ≥ 4) means no wall layer was ever pushed, so `ds1.Walls[0]` indexes an empty slice (`ds1.go:346-352` vs `ds1.go:144-162`, `ds1.go:503`).
- **Orientation `Zero` never survives a round-trip.** Encode does `(uint32(Zero) & wallZeroBitmask) << wallZeroOffset` with `wallZeroBitmask = 0xFFFFFF00` (`ds1.go:635`); since `Zero` is a byte, the mask always yields 0. Decode's `>> 8` is not mirrored.
- **NPC path encoding is broken twice.** `for objectIdx := range objectsWithPaths` iterates the slice *index*, not the stored object index, so it writes `ds1.Objects[0]` regardless (`ds1.go:664-667`); and the action guard is `ds1.version >= v15` (`ds1.go:673`) while the decoder uses `v > v15` (`ds1_version.go:70`) — v15 files written by `Marshal` will not read back.
- **`DeleteSubstitution` deletes a shadow**: `l.delete(ShadowLayerGroup, idx)` (`ds1_layers.go:333`).
- **`LoadDS1` stores DS1s in the DT1 cache** — `am.dt1s.Retrieve(ds1Path)` and `am.dt1s.Insert(ds1Path, ds1, ...)` (`asset_manager.go:511, 525`); a name collision between a `.ds1` and `.dt1` key would return the wrong type and panic on the assertion.
- **`SubTileAt` dereferences a possibly-nil tile** — `TileAt` returns nil out of range (`engine.go:213`) and `SubTileAt` calls a method on it unchecked (`engine.go:206`); reachable from `checkLos`.
- **Tile randomisation degenerates on row y = 0**: `tileSeed *= uint64(y)` (`map_tile.go:88`) zeroes the seed, so the whole row picks index 0. Also `PrepareTile(x, y, m)` is passed the **stamp-local** coordinates, not map coordinates (`engine.go:185`), so identical stamps randomise identically; and the weighted walk uses `sum >= int(random)` (`map_tile.go:117`), off by one against a `random` in `[0, weightSum)`.
- **Marshal is lossy for out-of-range field values** — `Sequence`/`Unknown1`/`Style` are `byte` but only 6 bits wide, `Unknown2` 5, `HiddenBytes` 1 (`tile.go:83-88`).
- **`RandomIndex`, `YAdjust`, `Animated` are decoder-struct fields with no wire representation** (`tile.go:34-51`); they are populated later by `PrepareTile` and the tile cache and are dropped by `Marshal`.
- Negative `numberOfTiles` in a DT1 would panic in `make` (`dt1.go:89`); block payload reads trust `Length` and `FileOffset` unvalidated (`dt1.go:233-235`); `DecodeTileGfxData` performs no bounds check on the destination offset (`gfx_decode.go:27, 63`).

## Worked example

**DS1 from `ds1_test.go:10-185`** — version 17, 2×2, Act 1, `SubstitutionType` 0, files `["a.dt1", "bfile.dt1"]`, 1 floor / 2 walls / 1 shadow, 4 objects (object index 1 owns one empty path), `unknown2 = 20`.

`Marshal` (`ds1.go:539`) emits, in order:

| Bytes (hex, LE) | Meaning |
|---|---|
| `11 00 00 00` | version 17 |
| `01 00 00 00` | width − 1 (grid is 2) |
| `01 00 00 00` | height − 1 |
| `00 00 00 00` | Act − 1 (17 ≥ v8) |
| `00 00 00 00` | SubstitutionType 0 (17 ≥ v10) |
| `02 00 00 00` | file count (17 ≥ v3) |
| `61 2E 64 74 31 00` | `"a.dt1"` + NUL |
| `62 66 69 6C 65 2E 64 74 31 00` | `"bfile.dt1"` + NUL |
| — | no unknown1: `hasUnknown1Bytes` needs v ≤ 13 |
| `02 00 00 00` | wall count 2 (17 ≥ v4) |
| `01 00 00 00` | floor count 1 (17 ≥ v16) |
| 6 × 4 cells × 4 B = 96 B | layers, in schema order Wall1, Orientation1, Wall2, Orientation2, Floor1, Shadow1 |
| `04 00 00 00` + 4 × 5 dwords | objects |
| — | substitutions skipped: `SubstitutionType` is 0 |
| `01 00 00 00` | NPC count (one object has paths) |
| `00 00 00 00 00 00 00 00 00 00 00 00` | numPaths / X / Y — all zero, because the encoder wrote `Objects[0]` instead of `Objects[1]` |

`exampleWall1` (Prop1 3, Sequence 89, Unknown1 213, Style 28, Unknown2 53, Hidden 7) packs to `0xD5C55903` — bytes `03 59 C5 D5` — because Sequence truncates to 6 bits (89 → 25), Unknown1 to 6 (213 → 21), Unknown2 to 5 (53 → 21) and Hidden to 1 (7 → 1). Its orientation dword is `02 00 00 00`: `TileRightWall` = 2, `Zero` lost. `TestDS1_MarshalUnmarshal` (`ds1_test.go:187`) only asserts that re-parsing does not error — it does not compare the round-tripped struct, which is why all of the above survives the suite.

**DT1 sub-tile from `subtile_test.go:9-24`** — `data := []byte{1, 2, 4, 8, 16, 32, 64, 128}`, each byte decoded on its own by `NewSubTileFlags`, asserting that byte *i* sets exactly flag *i*: `1`→`BlockWalk`, `2`→`BlockLOS`, `4`→`BlockJump`, `8`→`BlockPlayerWalk`, `16`→`Unknown1`, `32`→`BlockLight`, `64`→`Unknown2`, `128`→`Unknown3`. A real fully-blocking sub-tile byte would be `0x2F` (walk+LOS+jump+player-walk+light). `Encode`, `Combine` and `DebugString` are untested.
