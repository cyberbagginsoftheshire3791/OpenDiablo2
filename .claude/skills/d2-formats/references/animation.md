# Animation: COF and the composite system

**Decoder:** `d2common/d2fileformats/d2cof/` · **Tests:** `d2common/d2fileformats/d2cof/cof_test.go` · **Consumers:** `d2core/d2asset/composite.go`, `d2core/d2asset/asset_manager.go` (`LoadCOF`), `d2core/d2map/d2mapentity/{factory,player,npc,object}.go` · **Speed data:** `d2common/d2fileformats/d2animdata/`

All citations are to commit `c26dc732`.

## What COF holds

A COF is the *assembly manifest* for one (entity token, animation mode, weapon class) triple. It carries no pixels. It holds: how many layers the entity is built from and which composite slot each occupies (`d2common/d2fileformats/d2cof/cof.go:145`), each layer's shadow/selectable/transparency/draw-effect flags and its own weapon-class token (`cof.go:146-152`), how many frames per direction and how many directions exist (`cof.go:130-131`), a per-frame event marker array (`cof.go:161`), a nominal animation speed (`cof.go:133`), and the per-direction/per-frame layer draw-order table (`cof.go:169`). Sprites, frame counts used at runtime, and playback speed come from elsewhere — see the gotchas.

## Structure

`Unmarshal` (`cof.go:83`) reads a fixed 25-byte header (`numHeaderBytes = 4 + numUnknownHeaderBytes`, `cof.go:13`), then 3 more bytes, then variable-length sections:

| Offset | Size | Field | Source |
|---|---|---|---|
| 0 | 1 | `NumberOfLayers` | `cof.go:129` |
| 1 | 1 | `FramesPerDirection` | `cof.go:130` |
| 2 | 1 | `NumberOfDirections` | `cof.go:131` |
| 3–23 | 21 | `unknownHeaderBytes` | `cof.go:132` |
| 24 | 1 | `Speed` | `cof.go:133` |
| 25–27 | 3 | `unknownBodyBytes` | `cof.go:95` |
| 28… | 9×L | layer records | `cof.go:136` |
| … | F | `AnimationFrames` | `cof.go:108` |
| … | D×F×L | priority table | `cof.go:115` |

The 21 unknown header bytes are `numUnknownHeaderBytes = 21` (`cof.go:11`) and the 3 body bytes are `numUnknownBodyBytes = 3` (`cof.go:12`). **UNEXPLAINED** — no comment in the repo says what either block contains; they are read, stored, and written back verbatim.

**Speed? Yes.** `headerSpeed = numHeaderBytes - 1` (`cof.go:21`) puts it at byte 24, decoded as a plain `uint8`.

**Layer records — 9 bytes each** (`numLayerBytes = 9`, `cof.go:14`), field offsets from the const block at `cof.go:24-31`: `+0` composite type, `+1` shadow (a raw `byte`, not a bool), `+2` selectable (`> 0`), `+3` transparent (`> 0`), `+4` draw effect, `+5..+8` weapon class — **4 bytes**: a 3-character ASCII token plus a NUL terminator. There are no *unknown* bytes inside a layer record; the "extra" byte is the terminator. Decoding strips NULs and trims spaces before the enum lookup (`cof.go:151`).

**Animation-frames array:** `FramesPerDirection` bytes, each a `d2enum.AnimationFrame` (`cof.go:161-167`) — `NoEvent/Attack/Missile/Sound/Skill` (`d2common/d2enum/animation_frame.go:8-12`). One array, shared across all directions.

**Priority / draw-order table:** `priorityLen := c.FramesPerDirection * c.NumberOfDirections * c.NumberOfLayers` (`cof.go:115`), read as a flat byte run and reshaped by nested loops in `loadPriority` (`cof.go:169-182`). Nesting is **direction (outer) → frame → priority slot (inner)**, so the indexing is `Priority[direction][frame][slot]`. Draw order therefore varies **both by direction and by frame** — the code allocates a fresh `[]CompositeType` for every (direction, frame) pair (`cof.go:175`), which is exactly what a torso/arm swap during a swing needs.

Crucially, the *values* stored at `[direction][frame][slot]` are **composite types, not layer indices** (`cof.go:177`). Slot 0 is drawn first (backmost). The renderer uses them directly as indices into a `CompositeTypeMax`-sized layer array (`d2core/d2asset/composite.go:70-71`).

**Marshal symmetry** (`cof.go:185-244`): writes the same order — 3 header counts, unknown header bytes, speed, unknown body bytes, layers, frames, priority. It is *not* fully symmetric:

- Weapon class is re-encoded from `WeaponClass.String()` plus one terminator (`cof.go:218-228`). For `WeaponClassNone`, whose token is the empty string (`d2common/d2enum/weapon_class.go:11`), that emits **1 byte instead of 4**, producing a 6-byte layer record and a file no longer parseable by `Unmarshal`. Round-trip is only safe when every layer has a 3-character weapon class.
- `maxCodeLength = 3` with the guard `if idx > maxCodeLength` (`cof.go:214`, `cof.go:221`) would allow 4 characters through; harmless because every enum token is 3 characters.
- Marshal reads `c.Priority[direction][frame][i]` using the header counts (`cof.go:238`), so mutating `NumberOfLayers`/`NumberOfDirections` without resizing `Priority` panics.
- `unknownHeaderBytes` is a **subslice aliasing the caller's input buffer** (`cof.go:132`), not a copy.

## Endianness and packing

Every COF field is a single byte, so endianness never arises inside COF. The shared reader is little-endian for wider types (`d2common/d2datautils/stream_reader.go:52`, `:68`), which matters for AnimData's `uint32`/`uint16` fields. Nothing is bit-packed: COF uses `ReadBytes` on byte boundaries throughout, unlike DCC. There is no padding or alignment — sections abut exactly, and total size is `28 + 9L + F + (D×F×L)`. `Unmarshal` does **not** verify it consumed the whole buffer (contrast `d2animdata.Load`, `d2common/d2fileformats/d2animdata/animdata.go:206`), so trailing bytes are silently ignored.

## The composite system

`AssetManager.LoadComposite(baseType, token, palettePath)` (`d2core/d2asset/asset_manager.go:195`) just builds the struct and sets direction 0 — no I/O. Base directory comes from `baseString` (`composite.go:374-385`): `/data/global/chars` for `ObjectTypePlayer`, `/data/global/monsters` for `ObjectTypeCharacter`, `/data/global/objects` for `ObjectTypeItem`.

Real work happens in `createMode` (`composite.go:257`), called by `SetMode` (`composite.go:114`) and `Equip` (`composite.go:131`). It:

1. Builds the COF path and checks existence, erroring out if absent (`composite.go:258-261`).
2. Loads and caches the COF (`asset_manager.go:533`).
3. Looks up AnimData by `strings.ToUpper(c.token + animationMode.String() + weaponClass)` (`composite.go:268-270`).
4. Takes `frameCount` and `animationSpeed` from **AnimData record `[0]`**, not from the COF (`composite.go:280-281`).
5. For each `cof.CofLayers` entry: picks `c.equipment[cofLayer.Type]`, defaulting to `"lit"` when empty (`composite.go:285-288`); applies `cofLayer.DrawEffect` **only if `cofLayer.Transparent`**, else `DrawEffectNone` (`composite.go:290-294`); loads the sprite; sets play speed, direction, and `SetShadow(cofLayer.Shadow != 0)` (`composite.go:299-305`); stores it at `mode.layers[cofLayer.Type]`.

Layer load failures are swallowed — `if err == nil` (`composite.go:298`) leaves the slot `nil`, and rendering skips nils.

**Drawing** (`composite.go:63-86`) is two full passes over `Priority[direction][frameIndex]`: first `RenderFromOrigin(target, true)` for shadows, then `RenderFromOrigin(target, false)` for the sprites, both in stored priority order. A transparent layer never casts a shadow — `RenderFromOrigin` bails when `a.effect.Transparent()` (`d2core/d2asset/animation.go:207`, `d2common/d2enum/draw_effect.go:45`). `GetSize` walks the same priority list for the bounding box (`composite.go:347-364`).

**Directions.** The engine's facing is a 0–63 value. `d2cof.Dir64ToCof(direction, numDirections)` (`d2common/d2fileformats/d2cof/cof_dir_lookup.go:14`) maps it onto the COF's stored direction count using five 64-entry tables selected by `four directionCount = 4 << iota` → 4, 8, 16, 32, 64 (`cof_dir_lookup.go:5-11`); anything else (including a 1-direction COF) hits `default: return 0`. The COF tables are block-ordered starting at 0. **Sprites use a different table:** `d2dcc.Dir64ToDcc` (`d2common/d2fileformats/d2dcc/dcc_dir_lookup.go:15`, "Special thanks for Necrolis for these tables!") — its `dir8` starts `4, 4, 4, 4, 0, 0, ...`, a different permutation. Composite direction and layer direction are set through the two different lookups (`composite.go:68` vs `animation.go:313`); do not assume they agree. `Animation.SetDirection` rejects indices ≥ 64 (`animation.go:308-311`).

**Draw effects** are the eight PL2 blend modes plus a sentinel (`d2common/d2enum/draw_effect.go:10-41`): 25/50/75% alpha, Modulate, Burn, Normal, Mod2XTrans, Mod2X, then `DrawEffectNone`. Only the three alpha modes and Modulate do anything at draw time — see `palette.md`.

## The layer-to-file naming convention

End to end for an equipped player item:

1. **Data table.** `armor.txt`/`weapons.txt`/`misc.txt` → `ItemCommonRecord` (`d2core/d2records/item_common_record.go`). Relevant columns: `code` → `Code`, `wclass`/`2handedwclass` → `WeaponClass`/`WeaponClass2Hand` (`d2core/d2records/item_common_loader.go:133-134`), and the composite-layer columns `rArm/lArm/Torso/Legs/rSPad/lSPad` → `AnimRightArm` … `AnimLeftShoulderPad` (`item_common_loader.go:66-71`), documented as "these come from ArmType.txt" (`item_common_record.go:70-71`).
2. **Item → layer string.** Armor contributes its **armor class token** (`InventoryItemArmor.GetArmorClass`, `d2core/d2inventory/inventory_item_armor.go:19`, defaulting to `"lit"`); weapons and shields contribute their **item code** (`inventory_item_weapon.go:72`). Tokens are `lit`/`med`/`hvy` (`d2common/d2enum/item_armor_class.go:8-10`) from `ArmType.txt` (`d2core/d2records/armor_type_record.go`, `d2common/d2resource/resource_paths.go:362`); layer slot names come from `Composit.txt` (`resource_paths.go:340`).
3. **Slot assignment.** `NewPlayer` fills `[d2enum.CompositeTypeMax]string` — HD/TR/LG/RA/LA from armor classes, RH/LH/SH from item codes (`d2core/d2map/d2mapentity/factory.go:65-76`). Monsters fill the same 16-slot array from `monstats2.txt` columns `HDv/TRv/LGv/Rav/Lav/RHv/…` (`d2core/d2records/monster_stats2_loader.go:33-39`, `factory.go:190-193`). Enum order is HD, TR, LG, RA, LA, RH, LH, SH, S1–S8 (`d2common/d2enum/composite_type.go:14-30`; tokens in `composite_type_string.go:30`).
4. **COF selection.** `composite.go:258`:
   `fmt.Sprintf("%s/%s/COF/%s%s%s.COF", c.basePath, c.token, c.token, animationMode, weaponClass)`
5. **Layer → sprite.** `composite.go:317-318`:
   `fmt.Sprintf("%s/%s/%s/%s%s%s%s%s.dcc", c.basePath, c.token, layerKey, c.token, layerKey, layerValue, animationMode, weaponClass)`
   — where `layerKey = cofLayer.Type.String()` (e.g. `TR`), `layerValue` is the equipment string or `"lit"`, and **`weaponClass` here is the layer's own `cofLayer.WeaponClass.String()`** (`composite.go:296-297`), which may differ from the mode's weapon class.

**Example A — Barbarian, neutral, hand-to-hand, torso in light armor.** `token = "BA"` (`d2common/d2enum/hero.go:26`), mode `"NU"` (`player_animation_mode_string.go:33`), default weapon class `"hth"` (`inventory_item_weapon.go:22`), default layer value `"lit"`:
- COF → `/data/global/chars/BA/COF/BANUhth.COF`
- TR layer → `/data/global/chars/BA/TR/BATRlitNUhth.dcc`
- AnimData key → `BANUHTH`

**Example B — Sorceress casting with a staff, right hand holding item code `wnd`.** `token = "SO"` (`hero.go:34`), mode `SC` (`player_animation_mode_string.go:33`), weapon class `stf` (`weapon_class.go:16`):
- COF → `/data/global/chars/SO/COF/SOSCstf.COF`
- RH layer → `/data/global/chars/SO/RH/SORHwndSCstf.dcc`
- AnimData key → `SOSCSTF`

Objects use uppercase `"HTH"` (`d2core/d2map/d2mapentity/object.go:31`), giving `/data/global/objects/<TOKEN>/COF/<TOKEN>NUHTH.COF`. Case is harmless: MPQ hashing uppercases the key (`d2common/d2fileformats/d2mpq/crypto.go:110`) and slashes are converted to backslashes at the source (`d2common/d2loader/mpq/source.go:53`).

## Animation modes and direction counts

The scaffold's pre-filled claim is **mostly right but incomplete**. Corrections against the code:

**Player** (`d2common/d2enum/player_animation_mode.go:10-30`) — 21 entries: `DT` Death, `NU` Neutral, `WL` Walk, `RN` Run, `GH` GetHit, **`TN` TownNeutral**, **`TW` TownWalk**, `A1`, `A2`, `BL` Block, `SC` Cast, `TH` Throw, `KK` Kick, `S1`–`S4`, `DD` Dead, plus `Sequence` and `KnockBack` (both re-using `GH`) and `None` (intended `""`). TN and TW were missing from the claim.

**Monster** (`monster_animation_mode.go:10-25`) — 16 entries: `DT`, `NU`, `WL`, `GH`, `A1`, `A2`, `BL`, `SC`, `S1`–`S4`, `DD`, Knockback (`GH`), Sequence (**`xx`**), `RN`. No `TH`/`KK`/`TN`/`TW`.

**Object** (`object_animation_mode.go:11-18`) — 8 entries: `NU`, **`OP`** Operating, **`ON`** Opened, `S1`–`S5`. `OP`/`ON` and `S5` appear nowhere in the claim.

Weapon-class tokens: `""`, `hth`, `bow`, `1hs`, `1ht`, `stf`, `2hs`, `2ht`, `xbw`, `1js`, `1jt`, `1ss`, `1st`, `ht1`, `ht2` (`weapon_class.go:11-25`).

Direction counts: the code recognises exactly **4, 8, 16, 32, 64** (`cof_dir_lookup.go:5-11`). A count of **1 is not in the table** — it falls through to `default: return 0`, which happens to be correct behaviour for a single-direction COF but is not an explicit case.

## AnimData.d2 — speeds and events

Loaded once at startup from `/data/global/animdata.d2` (`d2common/d2resource/resource_paths.go:352`) into `Records.Animation.Data` (`d2app/initialization.go:145-160`).

Layout (`d2common/d2fileformats/d2animdata/animdata.go:129-204`): **256 blocks** (`numBlocks`), each a little-endian `uint32` record count followed by that many records. A count above `maxRecordsPerBlock = 67` is rejected (`animdata.go:135`). Each record is **8-byte name** (byte 7 must be NUL — `animdata.go:147`; effective max name length 7 chars, which is exactly `token+mode+wclass`), **`uint32` framesPerDirection**, **`uint16` speed**, **2 padding bytes** skipped (`animdata.go:166`), then **144 event bytes** (`numEvents`), one per frame, non-zero ones kept in a sparse map (`animdata.go:170-180`). Load fails unless the reader lands exactly on EOF (`animdata.go:206`).

FPS formula (`record.go:32-38`): `baseFPS * speed / speedDivisor` with `speedDivisor = 256` and `speedBaseFPS = 25` (`animdata.go:17-18`). **UNEXPLAINED** — no comment justifies either constant. `FrameDurationMS = 1000 / FPS` (`record.go:41-43`). The same pair is duplicated in `d2cof/helpers.go:6-7` (with a `fps == 0 → 25` fallback) and again as `hardcodedFPS`/`hardcodedDivisor` in `composite.go:14-18`.

Events (`d2common/d2fileformats/d2animdata/events.go:8-12`): `None`, `Attack`, `Missile`, `Sound`, `Skill` — the same five values, in the same order, as `d2enum.AnimationFrame`.

Record name construction: `strings.ToUpper(token + animationMode.String() + weaponClass)` in `composite.go:268`, then `GetRecords(animationKey)` (`composite.go:270`).

**hashName block placement.** `hashName` sums the uppercased name's bytes mod 256 (`hash.go:7-18`) and that is the block a record belongs to. `Load` computes it but writes every result to `hashTable[0]` — `hashIdx := 0` (`animdata.go:125`) is **never incremented** (only two references exist: `:125` and `:154`), so the hash table is garbage and unused. `Marshal` (`animdata.go:215-295`) ignores hashing entirely, packing records into blocks in Go map-iteration order — output is neither byte-identical to input nor hash-correct. The in-code test fixture (`animdata_test.go` `synthesizeAnimData`) places records in their hash blocks the way a real file does.

## Per-frame events

**Unconsumed.** `COF.AnimationFrames` is decoded (`cof.go:161`) and re-marshaled (`cof.go:231`) but has no reader outside `d2cof`. AnimData's event map has accessors (`record.go:46-63`), but the only references to `AnimationEventAttack`/`Missile`/`Sound`/`Skill` anywhere in the tree are the enum definition and `animdata_test.go`. Nothing in `d2mapentity`, `d2asset`, or `d2game` reads them.

The one frame-driven callback that exists is hand-rolled: skills fire when the cast animation passes 50%, computed from frame index over frame count, not from an event byte (`d2core/d2map/d2mapentity/player.go:104-110`).

## Edge cases the decoder already handles

- Zero layers / frames / directions parse cleanly: `ReadBytes(count <= 0)` returns `(nil, nil)` (`stream_reader.go:~105`), so a header of zeros yields empty slices rather than an error. This is what `cof_test.go:19` exercises.
- Weapon-class tokens are NUL-stripped and space-trimmed before lookup (`cof.go:151-152`, `badCharacter` at `cof.go:34`).
- Truncation anywhere surfaces as `io.EOF` through the `err` returns at `cof.go:89`, `:96`, `:109`, `:119`.
- `New()` pre-allocates `unknownHeaderBytes`/`unknownBodyBytes` (`cof.go:40-41`), so a hand-built COF marshals to a valid header.
- AnimData rejects: over-full blocks, missing name terminator, and any trailing or missing byte (`animdata.go:135`, `:147`, `:206`) — all covered by `animdata_test.go`.

## Known gotchas

1. **`WeaponClassFromString` panics** on an unknown token (`d2common/d2enum/weapon_class_string2enum.go:17`). A modded or corrupt COF crashes the process from inside `Unmarshal`.
2. **Marshal loses layers with an empty weapon class** (see Structure) — 6-byte records instead of 9.
3. **`CompositeLayers` collapses duplicates**: `c.CompositeLayers[layer.Type] = i` (`cof.go:155`), last layer of a type wins.
4. **Priority values are composite types**, not indices into `CofLayers` — indexing `CofLayers[p]` is wrong.
5. **The `.dc6` fallback is unreachable.** `loadCompositeLayer` returns an error on the first non-existent path (`composite.go:322-325`), so if the `.dcc` is missing the `.dc6` at `composite.go:318` is never tried.
6. **Frame count and speed come from AnimData, not the COF.** `frameCount: animationData[0].FramesPerDirection()` (`composite.go:280`). If AnimData reports more frames than the COF's `FramesPerDirection`, `Priority[direction][c.mode.frameIndex]` (`composite.go:70`) indexes out of range.
7. **`animationData[0]` vs `GetRecord`.** The composite uses the *first* record for a name; `GetRecord` returns the *last* (`animdata.go:50`). Real files have several records per name.
8. **`Advance` divides by `frameCount`** (`composite.go:48-49`) — a zero-frame AnimData record panics.
9. **`COF.Speed` has no runtime consumer**, and `COF.FPS()`/`Duration()` (`helpers.go:4`, `:19`) have no callers at all.
10. **`PlayerAnimationModeNone.String()` returns `"PlayerAnimationMode(20)"`**, not `""` — the generated index has 21 entries so value 20 is out of range (`player_animation_mode_string.go:35-41`), despite the `// ""` comment at `player_animation_mode.go:30`. Callers must guard by enum value, as `StartCasting` does (`player.go:230`).
11. **Duplicate mode tokens**: player Sequence and KnockBack both stringify to `GH`; monster Sequence is `xx`. Round-tripping a mode through its string is lossy.
12. **`d2resource.PlayerAnimationBase = "/data/global/CHARS"`** (`resource_paths.go:353`) is dead — `composite.go:374` hardcodes its own lowercase copy. Two sources of truth.
13. **COF and DCC direction tables differ** (see Directions above).
14. **`SetDirection` fans out one goroutine per layer** and joins on a `WaitGroup` (`composite.go:165-184`) — 16 goroutines per facing change.
15. **`SetAnimSpeed`/`SetPlayLoop`/`SetSubLoop`/`SetCurrentFrame` dereference `c.mode` without a nil check** (`composite.go:150`, `:203`, `:213`, `:223`), unlike `Advance`/`Render`/`GetPlayedCount` which do check.

## Worked example

`TestCOF_Marshal_Unmarshal` (`cof_test.go:13`) round-trips through the smallest legal COF. `cof1.Unmarshal(make([]byte, 1000))` — 1000 zero bytes — decodes as:

| Bytes | Value | Meaning |
|---|---|---|
| `[0] = 0x00` | 0 | `NumberOfLayers` — no layer records follow |
| `[1] = 0x00` | 0 | `FramesPerDirection` — empty `AnimationFrames` |
| `[2] = 0x00` | 0 | `NumberOfDirections` — empty `Priority` |
| `[3..23]` | 21 × `0x00` | `unknownHeaderBytes` (UNEXPLAINED) |
| `[24] = 0x00` | 0 | `Speed` |
| `[25..27]` | 3 × `0x00` | `unknownBodyBytes` (UNEXPLAINED) |
| `[28..999]` | — | **ignored**; no length check exists |

`priorityLen` is `0 × 0 × 0 = 0`, so both variable sections read zero bytes and the remaining 972 bytes are dropped on the floor.

The test then sets `cof1.Speed = 255` and marshals. Output is exactly **28 bytes**: `00 00 00`, twenty-one `00`, `FF`, `00 00 00`. Re-parsing gives `Speed == 255` and the assertion at `cof_test.go:32` passes.

Note what this example does *not* cover: because `NumberOfLayers` is 0, the empty-weapon-class marshal bug (gotcha 2) is never reached, and neither the layer records nor the priority table are exercised by any test in the package.
