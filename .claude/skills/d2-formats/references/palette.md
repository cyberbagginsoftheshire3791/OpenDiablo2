# Palettes: DAT and PL2

**Decoders:** `d2common/d2fileformats/d2dat/` (DAT palettes) and `d2common/d2fileformats/d2pl2/` (PL2 transforms) · **Tests:** only `d2common/d2fileformats/d2pl2/pl2_test.go` (there is no `d2dat` test — `d2common/d2fileformats/d2dat/` contains only `dat.go`, `dat_color.go`, `dat_palette.go`, `doc.go`) · **Fixtures:** none on disk (no `*.dat` / `*.pl2` files in the repo) · **Consumers:** `d2core/d2asset/asset_manager.go:244` (`LoadPalette`), `:316` (`LoadPaletteTransform`), `d2common/d2util/palette.go:11` (`ImgIndexToRGBA`), `d2core/d2map/d2maprenderer/tile_cache.go:105,149,220`, `d2core/d2asset/dc6_animation.go:171`, `d2core/d2asset/dcc_animation.go:188`, `d2core/d2asset/animation.go` (effects/colormod), `d2core/d2ui/*`, `d2core/d2gui/style.go`, `d2game/d2player/*`, `d2game/d2gamescreen/*`.

All citations are to commit `c26dc732`.

## What they hold

A `.dat` palette is a flat 256-entry RGB lookup table with no header. Indexed image formats (DC6, DCC, DT1) decode to one byte per pixel; that byte is an index into the DAT palette chosen by the caller. PL2 is a separate file (`Pal.pl2`) holding the same 256 colours plus a large stack of 256-byte index-remap tables (light levels, blends, hue shifts, text colours). In this codebase PL2 is decoded but never consulted at draw time — see "Which palette applies where".

## DAT — structure

`d2common/d2fileformats/d2dat/dat.go:7-13` declares the offsets as a bare iota block:

```go
b = iota; g; r; o   // dat.go:9-12 → b=0, g=1, r=2, o=3
```

`o` is the entry stride (3), used as the multiplier: `palette.colors[i] = &DATColor{b: data[i*o+b], g: data[i*o+g], r: data[i*o+r]}` (`dat.go:21`). So each entry is **3 bytes in B, G, R order**, 768 bytes total, no header, no footer, no magic.

- `Load` loops `for i := 0; i < 256; i++` (`dat.go:19`) and **always returns `nil` error** (`dat.go:24`). It returns `d2interface.Palette` so a caller cannot inspect anything DAT-specific.
- **No bounds check.** `data` is indexed up to `data[767]`; any input shorter than 768 bytes panics with an index-out-of-range at `dat.go:21`.
- `Marshal` (`dat.go:28-36`) writes back `B(), G(), R()` per entry — round-trip preserves the on-disk order.
- `DATPalette` is `colors [256]d2interface.Color` (`dat_palette.go:14-16`, `numColors = 256` at `:10`). `GetColor(idx)` returns an error only when the entry is nil (`dat_palette.go:39-45`) — it does **not** range-check `idx`, so an index ≥ 256 panics.
- `DATColor` (`dat_color.go:4-9`) has `r, g, b, a uint8`. **`A()` ignores the stored field and returns `mask` = `0xff`** (`dat_color.go:39-41`) — DAT has no alpha channel, every entry is fully opaque.
- Note the inconsistency: `RGBA()`/`BGRA()` (`dat_color.go:44-56`) use `c.a`, which `Load` never sets, so they yield alpha 0 for loaded palettes while `A()` yields 255. `RGBA()` is not used by the render path (`ImgIndexToRGBA` calls `R()/G()/B()/A()`).
- Interface: `d2interface.Palette` = `NumColors() int`, `GetColors() [256]Color`, `GetColor(idx int) (Color, error)`; `Color` = `R/G/B/A() uint8`, `RGBA()/SetRGBA`, `BGRA()/SetBGRA` (`d2common/d2interface/palette.go:3-22` — `Color` and `Palette` are both in `palette.go`; there is no `color.go`).

## PL2 — structure

`d2common/d2fileformats/d2pl2/pl2.go:10-29`, unpacked in declaration order by restruct. **There are no struct tags at all** — every field size comes from the Go type. Sizes below are computed from the types; the file total is **443,175 bytes**.

| Field (actual name) | Type | Bytes | Notes |
|---|---|---|---|
| `BasePalette` | `PL2Palette` = `Colors [256]PL2Color` (`pl2_palette.go:5`) | 1024 | `PL2Color` is `R,G,B uint8` + one blank `_ uint8` pad (`pl2_color.go:13-18`) — **RGB + 1 pad byte, not RGBA**; the 4th byte is discarded and `RGBA()` hardcodes alpha `mask` (`pl2_color.go:22`) |
| `LightLevelVariations` | `[32]PL2PaletteTransform` | 8192 | 32 UNEXPLAINED |
| `InvColorVariations` | `[16]PL2PaletteTransform` | 4096 | 16 UNEXPLAINED |
| `SelectedUintShift` | `PL2PaletteTransform` | 256 | name is a typo for "UnitShift"; kept as-is in code and test |
| `AlphaBlend` | `[3][256]PL2PaletteTransform` | 196608 | 3 UNEXPLAINED |
| `AdditiveBlend` | `[256]PL2PaletteTransform` | 65536 | |
| `MultiplicativeBlend` | `[256]PL2PaletteTransform` | 65536 | |
| `HueVariations` | `[111]PL2PaletteTransform` | 28416 | 111 UNEXPLAINED |
| `RedTones` / `GreenTones` / `BlueTones` | `PL2PaletteTransform` each | 768 | |
| `UnknownVariations` | `[14]PL2PaletteTransform` | 3584 | 14 UNEXPLAINED; name itself says unknown |
| `MaxComponentBlend` | `[256]PL2PaletteTransform` | 65536 | |
| `DarkendColorShift` | `PL2PaletteTransform` | 256 | spelling "Darkend" is in the code |
| `TextColors` | `[13]PL2Color24Bits` | 39 | `PL2Color24Bits` is `R,G,B uint8`, **no pad byte** (`pl2_color_24bits.go:4-8`); 13 UNEXPLAINED |
| `TextColorShifts` | `[13]PL2PaletteTransform` | 3328 | 13 UNEXPLAINED |

`PL2PaletteTransform` is `Indices [256]uint8` (`pl2_palette_transform.go:4-6`) — a 256-byte index→index remap.

`Load` requires `restruct.EnableExprBeta()` before `restruct.Unpack` (`pl2.go:35-37`), and `Marshal` calls it again before `Pack` and **panics on pack error** (`pl2.go:47-52`). Dependency is `github.com/go-restruct/restruct v1.2.0-alpha` (`go.mod:7`). There is **no magic-number check and no length validation** — a short or wrong file surfaces only as whatever error restruct returns from `Unpack`.

## Endianness and packing

`restruct.Unpack(data, binary.LittleEndian, &result)` (`pl2.go:37`) and `Pack(binary.LittleEndian, p)` (`pl2.go:49`). Every field is `uint8`, so the byte order argument has no observable effect; the layout is byte-sequential, tightly packed, no alignment padding beyond the explicit `_ uint8` in `PL2Color`. DAT is byte-sequential too (`dat.go:21`) — endianness is irrelevant there.

## Which palette applies where

Constants, `d2common/d2resource/resource_paths.go:466-483`: `PaletteAct1`..`PaletteAct5` (`/data/global/palette/act{1..5}/pal.dat`), `PaletteEndGame` (`endgame/pal.dat`), `PaletteEndGame2` (`endgame2/pal.dat`), `PaletteFechar`, `PaletteLoading`, `PaletteMenu0`..`PaletteMenu4`, `PaletteSky`, `PaletteStatic`, `PaletteTrademark`, `PaletteUnits` — all `/data/global/palette/<name>/pal.dat`. Transform constants `PaletteTransformAct1`..`PaletteTransformTrademark` at `:487-502` (`Pal.pl2`, capital P).

Actual usage across the tree:

- **Map tiles (floors, walls, shadows):** `MapRenderer.loadPaletteForAct` (`renderer.go:627-652`) switches on `d2enum.RegionIdType` → `PaletteAct1` for all Act 1 regions (`:635`), `PaletteAct2` (`:638`), `PaletteAct3` (`:641`), `PaletteAct4` for Act4 Town/Mesa/Lava **and `RegionAct5Lava`** (`:642-643`), `PaletteAct5` for the remaining Act 5 regions (`:646`); anything else returns `errors.New("failed to find palette for region")` (`:648`). Called once from `generateTileCache` (`tile_cache.go:26`); the result is stored on `mr.palette` and used for every tile surface.
- **Entities / units / objects:** `PaletteUnits` — `d2mapentity/factory.go:80,130,164,199,232` and `d2mapstamp/stamp.go:144`.
- **Cursor:** `PaletteUnits` (`d2gui/gui_manager.go:33`).
- **Fonts:** palette is per-font-style. `d2gui/style.go:29-36` maps `FontStyle16Units`/`30Units`/`42Units`/`Exocet10`/`Formal11Units`/`Rediculous` → `PaletteUnits`, and `FontStyleFormal10Static`/`Formal12Static` → `PaletteStatic`. In-game labels overwhelmingly use `PaletteStatic` (`hud.go:131`, `inventory.go:140`, `quest_log.go:244,250`, `main_menu.go:244-269`, …).
- **UI sprites/buttons:** mixed `PaletteSky` and `PaletteUnits`, chosen per button style in the `getButtonLayouts` table (`d2core/d2ui/button.go:225-780`); tooltips use `PaletteSky` (`button.go:922-949`). `PaletteSky` is the most-used constant in the tree (111 references), `PaletteUnits` second (69), `PaletteStatic` third (19). `d2ui/textbox.go:35,47,48` uses `PaletteUnits`.
- **Popups/checkbox:** `PaletteFechar` (`d2ui/checkbox.go:34`, `character_select.go:259`, `select_hero_class.go:772`, `main_menu.go:235`).
- **Loading screen:** `PaletteLoading` (`d2gui/gui_manager.go:38`).
- **Never referenced anywhere outside the constant block:** `PaletteEndGame`, `PaletteEndGame2`, `PaletteMenu0`–`PaletteMenu4`, `PaletteTrademark`, and **every one of the 16 `PaletteTransform*` constants**.

**PL2 transforms are parsed and unused.** `LoadPaletteTransform` (`asset_manager.go:316-338`) has **no callers** — the only occurrences of the identifier are its own definition and the `AssetTypePaletteTransform` enum (`d2common/d2loader/asset/types/asset_types.go:15,35`). Grepping `LightLevelVariations`, `InvColorVariations`, `SelectedUintShift`, `AlphaBlend`, `AdditiveBlend`, `MultiplicativeBlend`, `HueVariations`, `RedTones`, `GreenTones`, `BlueTones`, `UnknownVariations`, `MaxComponentBlend`, `DarkendColorShift`, `TextColors`, `TextColorShifts`, `BasePalette` across the repo returns hits **only** in `pl2.go` and `pl2_test.go`. No transform table is read at draw time. (`PaletteTransform` fields in `d2records/automagic_record.go:94` and `set_item_record.go:43,49` are unrelated `.txt` column integers.)

## How the renderer applies colour

`d2util.ImgIndexToRGBA(indexData, palette)` (`d2common/d2util/palette.go:11-33`) expands one index byte to 4 bytes RGBA:

- `if indexData[i] == 0 { continue }` with the comment *"Index zero is hardcoded transparent regardless of palette"* (`palette.go:16-19`) — the 4 output bytes stay zero, i.e. transparent black.
- Otherwise R/G/B/A are copied from the palette entry (`palette.go:26-29`); since `DATColor.A()` returns `0xff` (`dat_color.go:41`), **every non-zero index is fully opaque**.
- A `GetColor` error is only `log.Print`ed and then dereferenced (`palette.go:22-26`) — a nil color would panic. In practice `DATPalette` entries are always non-nil after `Load`.

The RGBA buffer goes straight into a GPU surface via `ReplacePixels`: tiles at `tile_cache.go:105,149,220`, DC6 frames at `dc6_animation.go:171-179`, DCC frames at `dcc_animation.go:188`.

Per-draw colour is applied on top, through the surface state stack (`d2common/d2interface/surface.go:21-28`: `PushColor`, `PushEffect`, `PushFilter`, `PushBrightness`, `PushSaturation`). Ebiten implementation `d2core/d2render/ebiten/ebiten_surface.go`:

- `PushColor` → `colorToColorM` builds a cached `ebiten.ColorM` scaled by `rf,gf,bf,af` with premultiply-divide by alpha (`ebiten_surface.go:291-343`); alpha 0 returns a `Scale(0,0,0,0)` matrix (`:299-304`). Cache is LRU-ish, capped at `cacheLimit = 512` (`:23,315-327`).
- `PushBrightness`/`PushSaturation` → applied only when either differs from 1, as `opts.ColorM.ChangeHSV(0, saturation, brightness)` (`:143-145`, `:156-158`). Note `RenderSection` uses the different guard `if s.stateCurrent.brightness != 0` (`:169`) — inconsistent with `Render`.
- `PushEffect` → `handleStateEffect` (`:200-218`), the full mapping:
  - `DrawEffectPctTransparency25/50/75` → `opts.ColorM.Translate(0,0,0,-0.25/-0.50/-0.75)` (constants at `:23-25`)
  - `DrawEffectModulate` → `opts.CompositeMode = ebiten.CompositeModeLighter` (`:209`)
  - `DrawEffectBurn`, `DrawEffectNormal`, `DrawEffectMod2XTrans`, `DrawEffectMod2X` → **empty cases, no-ops** (`:211-214`, with a TODO link to OpenDiablo2 issue 822)
  - `DrawEffectNone` → `ebiten.CompositeModeSourceOver` (`:216`)
- Effects come from COF layer bytes: `layer.DrawEffect = d2enum.DrawEffect(b[layerDrawEffect])` (`d2common/d2fileformats/d2cof/cof.go:149`), used at `d2core/d2asset/composite.go:290-293`. `DrawEffectModulate` is also set explicitly for glow sprites (`d2mapentity/factory.go:140`, `d2ui/button.go:897`, `main_menu.go:330,340`, `select_hero_class.go:782`).

`d2core/d2asset/animation.go`: `Render` pushes translation, then `a.effect`, then `a.colorMod` (`animation.go:169-178`); `RenderSection` does the same (`:232-241`). `SetColorMod` (`:380`), `SetEffect` (`:395`), `SetShadow` (`:400`). Drop shadow is `renderShadow` (`:129-152`): linear filter, translate with y halved, `PushScale(full, half)`, `PushSkew(half, zero)`, `PushEffect(DrawEffectPctTransparency25)`, `PushBrightness(zero)` — a squashed, skewed, 25%-alpha, zero-brightness copy of the frame, i.e. the shadow is produced by a brightness matrix, not by a palette table. `RenderFromOrigin` only draws it when `shadow && !a.effect.Transparent() && a.hasShadow` (`:207`), and `DrawEffect.Transparent()` is `d != DrawEffectNone` (`d2common/d2enum/draw_effect.go:46`) — so any non-`None` effect suppresses shadows entirely. Font colouring goes through the same path: `f.sheet.SetColorMod(f.color)` (`d2common/d2fileformats/d2font/font.go:108`).

**This is the seam for M4.1 (lighting).** `PushBrightness`/`PushColor` on the surface stack are the only per-draw colour controls that exist; a light layer will be built on them (or on a new per-tile darkening pass), not on PL2's `LightLevelVariations`, which nothing reads.

## Index 0 and reserved indices

Index 0 is the only reserved index and it is reserved **in code, not in the palette file**: `ImgIndexToRGBA` skips it unconditionally (`d2common/d2util/palette.go:16-19`). Whatever RGB the DAT stores at entry 0 is never drawn. Indices 1–255 are all treated identically; nothing in the codebase reserves a shadow index, a UI index, or a "keep" range, and no PL2 table is consulted to reinterpret them.

## Quantization target for new art (for the Phase 5 asset pipeline)

Stated strictly from code:

- **Map tiles (DT1 blocks):** must be indexed into the DAT the map renderer will load for that region — `/data/global/palette/act1/pal.dat` for all Act 1 regions, `act2`, `act3`, `act4` (which also covers `RegionAct5Lava`), `act5` for the rest (`renderer.go:631-646`). One palette is used for the whole level (`tile_cache.go:26`), so floors, walls and shadow tiles in a region share it.
- **Sprites (DC6/DCC) for entities, objects and the cursor:** `/data/global/palette/units/pal.dat` (`d2mapentity/factory.go:80,130,164,199,232`, `d2mapstamp/stamp.go:144`, `d2gui/gui_manager.go:33`).
- **UI sprites:** no single answer in code — it is per-widget, `/data/global/palette/sky/pal.dat` or `units/pal.dat` picked per button style (`d2ui/button.go:225-780`), plus `fechar/pal.dat` for popups (`d2ui/checkbox.go:34`).
- **Fonts:** `static/pal.dat` or `units/pal.dat` depending on font style (`d2gui/style.go:29-36`).
- **Index 0 must be reserved for transparency** and must not be used for any visible colour (`d2common/d2util/palette.go:16-19`).
- **Alpha:** the DAT carries none. `DATColor.A()` returns `0xff` unconditionally (`dat_color.go:41`), so every drawn pixel is opaque; partial transparency is only achievable via `DrawEffectPctTransparency25/50/75` at the surface level (`ebiten_surface.go:202-207`), which applies to a whole draw, not per-pixel.
- **PL2 is not a target.** Since no transform table is read, art does not need to be compatible with any light-level or blend ramp.
- **Acceptable quantization error metric: UNKNOWN.** Nothing in the code measures or constrains colour error — the pipeline decision needs (a) a colour distance metric (e.g. sRGB ΔE vs. plain RGB L2), (b) a per-pixel and per-image threshold, (c) a dithering policy, and (d) a rule for what happens when source art needs a colour absent from the fixed act palette. None of these exist in the repo. Note also the ratchet (Constitution V.2): once the modern loader (M5.1) takes PNG, new art need not be palettized at all — this section describes the constraint *only* for art that must pass through the D2 formats.

## Edge cases the decoders already handle

Very few, and mostly by accident:

- DAT `Load` never fails, so a caller cannot distinguish a valid palette from garbage (`dat.go:24`).
- `LoadPalette` rejects a non-`.dat` extension before decoding: `if types.Ext2AssetType(filepath.Ext(palettePath)) != types.AssetTypePalette` → `"not an instance of a palette: %s"` (`asset_manager.go:249-251`). `LoadPaletteTransform` performs **no** equivalent extension check (`:316-338`).
- Both loaders are cached: `am.palettes` keyed on path (`asset_manager.go:245,265`), `am.transforms` keyed on path with weight 1 (`:317,333`). Animations are cached on `path;palette;effect` (`:159`), so the same sprite under two palettes yields two cached animations.
- `DATPalette.GetColor` returns a descriptive error for a nil entry (`dat_palette.go:44`), and `ImgIndexToRGBA` logs rather than aborting (`palette.go:23-25`).
- PL2 `Marshal` and `Load` both re-enable expr-beta, so calling either in isolation works (`pl2.go:35,47`).

## Known gotchas

1. **Short DAT input panics.** No length check anywhere; `dat.go:21` reads through byte 767.
2. **`DATColor.RGBA()`/`BGRA()` return alpha 0 for loaded palettes** because `Load` never sets `a`, while `A()` returns `0xff` — two different answers for the same colour (`dat.go:21` vs `dat_color.go:41,45`).
3. **B,G,R on disk, not R,G,B.** Writing a naive RGB triplet file will swap red and blue (`dat.go:9-21`).
4. **`o` as a stride constant.** `o` is the fourth iota value used as the multiplier; it looks like a fourth channel offset but is the entry size.
5. **PL2 has no validation.** No magic, no size assertion; a truncated file just errors out of restruct, a longer one silently ignores the tail.
6. **PL2 base colours are 4 bytes each** (`R,G,B,_`) while `TextColors` are 3 bytes each — mixing the two up shifts the whole tail of the file.
7. **Typos are load-bearing:** `SelectedUintShift` (should be UnitShift) and `DarkendColorShift` (should be Darkened) are the real field names (`pl2.go:15,25`).
8. **Four DrawEffects are silently ignored** (`Burn`, `Normal`, `Mod2XTrans`, `Mod2X` — `ebiten_surface.go:211-214`), so COF layers requesting them render as plain source-over.
9. **Any non-`None` effect disables the drop shadow** (`animation.go:207` + `draw_effect.go:46`).
10. **`RenderSection`'s brightness guard is `!= 0` where `Render`'s is `!= 1`** (`ebiten_surface.go:169` vs `:156`).
11. **`RegionAct5Lava` uses the Act 4 palette** (`renderer.go:642`) — deliberate or not, it is what the switch says.
12. **`d2enum.RegonAct5Town`** (missing "i") is the real enum name used in the Act 5 case (`renderer.go:644`).

## Worked example

**PL2 round trip** — `d2common/d2fileformats/d2pl2/pl2_test.go`. `exampleData()` (`:7-21`) builds a `PL2` **in Go, not from a file**: it explicitly zero-initialises `BasePalette`, `SelectedUintShift`, `RedTones`, `GreenTones`, `BlueTones`, `DarkendColorShift` (the array fields are left implicitly zero), then sets exactly two bytes of signal — `result.BasePalette.Colors[0].R = 8` (`:17`) and `result.DarkendColorShift.Indices[0] = 123` (`:18`). `TestPL2_MarshalUnmarshal` (`:23-40`) packs it with `Marshal()`, reloads with `Load()`, and asserts two things: the whole `PL2Color` struct at `BasePalette.Colors[0]` compares equal (`:33`, so the pad byte survives as 0), and `DarkendColorShift.Indices[0]` is still 123 (`:37`). That second assertion is the meaningful one — it proves the ~439 KB of intervening fields are sized correctly, because any wrong array length would shift `DarkendColorShift` off its offset. The failure messages (`"unexpected length"`, `"unexpected index set"`) are the only diagnostics. Nothing tests a real Blizzard `.pl2`, and nothing tests the DAT decoder at all.

**DAT entry layout** — 3 bytes per entry, B then G then R, 256 entries, 768 bytes:

```
offset:  0    1    2  |  3    4    5  |  6    7    8
bytes:  00   00   00  | FF   00   00  | 00   80   FF
entry:   idx 0        |  idx 1        |  idx 2
means:  B=0 G=0 R=0   | B=255 G=0 R=0 | B=0 G=128 R=255
```

Index 0 is never drawn (`d2common/d2util/palette.go:16-19`), so its stored `00 00 00` is irrelevant. Index 1 renders as pure blue `RGBA(0, 0, 255, 255)`, index 2 as orange `RGBA(255, 128, 0, 255)` — alpha 255 in both cases because `A()` is hardcoded (`dat_color.go:41`).
