# Tables: string TBL, font TBL, and the TXT data tables

> **WARNING — two unrelated binary formats share the `.tbl` extension.** `string.tbl` / `expansionstring.tbl` / `patchstring.tbl` under `/data/local/lng/{LANG}/` are **string dictionaries** (hash table of key → localized text, decoded by `d2common/d2fileformats/d2tbl`). `font16.tbl`, `fontformal12.tbl` etc. under `/data/local/FONT/{LANG_FONT}/` are **glyph metric tables** for a paired DC6 sprite sheet (decoded by `d2common/d2fileformats/d2font`). They share no header, no magic, and no code path. A third thing entirely — the "data tables" everyone means when they say *table* — are the tab-separated `.txt` files in `/data/global/excel/`, decoded by `d2common/d2fileformats/d2txt` and turned into typed records by `d2core/d2records`.

**Decoders:** `d2common/d2fileformats/d2tbl/` (string tables) · `d2common/d2fileformats/d2font/` (+ `d2fontglyph/`) · `d2common/d2fileformats/d2txt/` (TXT) · `d2core/d2records/` (typed records)
**Tests:** `d2common/d2fileformats/d2tbl/text_dictionary_test.go` (2 tests) and `d2core/d2records/object_lookup_record_test.go` are the only tests touching any of this. **There is no d2font test and no d2txt test.**
**Fixtures:** none on disk. `docs/fixtures-manifest.md` records the TBL test as "Constructed in code, marshal."

All citations are to commit `c26dc732`.

---

## String TBL — structure

`d2tbl/text_dictionary.go:118` `LoadTextDictionary(dictionaryData []byte) (TextDictionary, error)`. `TextDictionary` is `map[string]string` (`:11`). Header reads, in order:

| Field | Size | Line | Use |
|---|---|---|---|
| CRC | 2 bytes | `:113-115`, `:124` | **skipped, never verified** (`crcByteCount = 2`) |
| number of elements | uint16 | `:136` | length of the element-index array |
| hash table size | uint32 | `:141` | number of hash entries |
| version | 1 byte | `:147` | read, **discarded** |
| string offset | uint32 | `:152` | read, discarded |
| max hash-miss retry count | uint32 | `:154-156` | read, discarded — commented "When the number of times you have missed a match with a hash key equals this value, you give up" |
| file size | uint32 | `:158` | read, discarded |
| element index | `numberOfElements` × uint16 | `:160-166` | read into `elementIndex`, then **never used** |

Hash table entries (`loadHashEntries:13`), 17 bytes each, read in a first pass: `IsActive` (1 byte, `active > 0`, `:17-22`), `Index` uint16 (`:24`), `HashValue` uint32 (`:29`), `IndexString` uint32 (`:34` — file offset of the key), `NameString` uint32 (`:39` — file offset of the value), `NameLength` uint16 (`:44` — value length *including* its NUL). Struct at `:104-111`.

A **second pass** (`:52-60`) then resolves only entries with `IsActive` (`loadHashEntry:65`). The value is read by seeking to `NameString` and taking `NameLength - 1` bytes (`:66-73`); the key is read by seeking to `IndexString` and consuming bytes until a `0` (`:75-90`).

Two rewrite rules, both load-bearing:
- `if key == "x" || key == "X" { key = "#" + strconv.Itoa(idx) }` (`:92-94`) — `idx` is the **hash-entry index**, not the string's own numeric ID.
- Duplicate keys: `_, exists := td[key]; if !exists { td[key] = value }` (`:96-99`) — **first occurrence wins within one file**.

`Marshal()` at `:179-260` re-encodes: 2 zero bytes for CRC (`:183`), `numberOfElements = 0` (`:185`), `hashTableSize = len(keys)` (`:192`), version 0, and three zeroed uint32s. Data start is computed as `len(header) + 17*len(*td)` (`:210`). Keys beginning with `#` are written back out as the literal `"x"` (`:227-232`, `:244-246`).

### Runtime loading and lookup

`d2app/initialization.go:165-180` `loadStrings()` loads exactly three tables, in this order:

```go
tablePaths := []string{d2resource.PatchStringTable, d2resource.ExpansionStringTable, d2resource.StringTable}
```

Paths (`d2common/d2resource/resource_paths.go:216-218`): `/data/local/lng/{LANG}/expansionstring.tbl`, `.../string.tbl`, `.../patchstring.tbl`. `{LANG}` is `LanguageTableToken` (`:9`); `{LANG_FONT}` is `LanguageFontToken` (`:8`). Both are substituted at load time in `d2common/d2loader/loader.go:79-84` (`strings.ReplaceAll`). The language comes from the **first byte** of `/data/local/use` (`asset_manager.go:132-149`), mapped through `d2resource/languages_map.go:3-18` (0x00→ENG … 0x0C→ENG) and to a charset at `:29-43` (ENG→`LATIN`, RUS→`CYR`, POL→`LATIN2`, JPN/KOR/CHI→own). A missing file falls back to `defaultLanguage` (`asset_manager.go:136`).

`LoadStringTable` (`asset_manager.go:271-287`) appends each table to `am.tables []d2tbl.TextDictionary` (`:284`, field at `:68`) — **no cache, no merge**. `TranslateString` (`:292-313`) accepts `string`, `fmt.Stringer`, or `int` (int becomes `fmt.Sprintf("#%d", d2enum.BaseLabelNumbers(s+am.languageModifier))`, `:301`) and searches linearly:

```go
for idx := range am.tables { if value, found := am.tables[idx][key]; found { return value } }
```

**First table with the key wins** (`:304-308`), so the effective precedence is patchstring → expansionstring → string. On a miss it returns the key itself (`:312`); the panic that used to fire is commented out (`:310-311`).

## Font TBL (+ DC6 glyph sheet) — structure

`d2font/font.go:37` `Load(data []byte) (*Font, error)`. Magic is `knownSignature = "Woo!\x01"` (`:15`), 5 bytes, compared exactly; mismatch → `"invalid font table format"` (`:45-47`). Then `sr.SkipBytes(unknownHeaderBytesCount)` — **7 bytes, UNEXPLAINED** (`:23`, `:54`). Header total is `numHeaderBytes = 12` (`:19`). `Marshal` (`:190-213`) says the constant bytes are `{1,0,0,0,0,1}` plus "Expected Height of character cell and Expected Width of character cell" `{0,0}` — but that is 6+2 = 8 after the 5-byte magic, i.e. **13 bytes, one more than `Load` skips** (see Gotchas).

Glyph records: `bytesPerGlyph = 14` (`:20`), read in `initGlyphs:145-187`:

| Offset | Size | Field | Line |
|---|---|---|---|
| 0 | uint16 | character code (used as `rune` map key) | `:150`, `:181` |
| 2 | 1 | unknown1 — commented "byte of 0", **UNEXPLAINED** | `:156` |
| 3 | 1 | width | `:158` |
| 4 | 1 | height | `:163` |
| 5 | 3 | unknown2 — commented "1, 0, 0", **UNEXPLAINED** | `:169` |
| 8 | uint16 | frame index into the DC6 sheet | `:171` |
| 10 | 4 | unknown3 — commented "1, 0, 0, character code repeated, and further 0", **UNEXPLAINED** | `:177` |

The loop is `for i := numHeaderBytes; true; i += bytesPerGlyph` — `i` is unused; termination is the `ReadUInt16` error at `:150-153`, i.e. it reads until EOF. `FontGlyph` (`d2fontglyph/font_glyph.go:18-22`) stores only `frame`, `width`, `height`; `Unknown1/2/3` (`:55-67`) are hardcoded constants used only by `Marshal`.

**Glyph → DC6 frame:** `SetBackground(sheet d2interface.Animation)` (`font.go:65-74`) attaches the sprite sheet and **overwrites every glyph's height** with the sheet's frame height (`f.Glyphs[i].SetSize(f.Glyphs[i].Width(), h)`); only the per-glyph *width* survives from the table. `RenderText` (`:107-143`) does `f.sheet.SetCurrentFrame(glyph.FrameIndex())` then `Render`, advancing by `glyph.Width()` (`:124-133`); unknown runes are silently skipped (`:119-121`); `\n` splits lines and `PopN` unwinds the translation stack. `GetTextMetrics` (`:82-104`) sums widths per line, takes max height per line, max width across lines.

**Pairing:** `AssetManager.LoadFont(tablePath, spritePath, palettePath)` (`asset_manager.go:212-240`) loads the DC6 animation first, then the table, then `font.SetBackground(sheet)` (`:236`); cache key is `"table;sprite;palette"` (`:213`). Callers pass a base path and append extensions: `d2ui/label.go:31` `ui.asset.LoadFont(fontPath+".tbl", fontPath+".dc6", palettePath)`; `d2gui/layout.go:542` does the same from `getFontStyleConfig` (`d2gui/style.go:28-38`: `FontStyle16Units`→Font16/PaletteUnits, `FontStyle30Units`, `FontStyle42Units`, `FontStyleExocet10`, `FontStyleFormal10Static`/PaletteStatic, `FontStyleFormal11Units`, `FontStyleFormal12Static`, `FontStyleRediculous`).

Font constants at `resource_paths.go:203-215`, all `/data/local/FONT/{LANG_FONT}/…`. Reference counts across the tree: Font16 72, Font6 21, Font30 18, FontFormal12 10, FontFormal11 5, FontFormal10 5, FontExocet10 5, FontRediculous 5, Font42 3, Font24 1. **Font8, FontExocet8 and FontSucker are declared but never referenced.**

## TXT data tables — how they're read

`d2txt/data_dictionary.go:21` `LoadDataDictionary(buf []byte) *DataDictionary` wraps `encoding/csv` with `cr.Comma = '\t'` and `cr.ReuseRecord = true` (`:22-24`). The **first row is the header**: read at `:26`, and **any error panics** (`:28` `panic(err)`) — an empty or unreadable file kills the process. Header names are mapped to column indices in `data.lookup` (`:36-38`).

- `Next() bool` (`:45-61`): `io.EOF` → false; any other error is stashed in `d.Err` and returns false; **a row whose first cell is exactly `"Expansion"` is skipped by recursing** (`:56-58`).
- `String(field)` (`:64-66`): `d.record[d.lookup[field]]`.
- `Number(field)` (`:69-76`): `strconv.Atoi(d.String(field))`, and **on any conversion error returns 0** — blank cells, `"-"`, decimals and text all silently become 0.
- `List(field)` (`:79-82`): `strings.Split(str, ",")` — a blank cell yields `[]string{""}`, length 1, not 0.
- `Bool(field)` (`:85-92`): calls `Number`, `log.Panic`s if the value is `> 1`, and returns `n == 1`. So **only literal `1` is true**; `0`, blank and non-numeric are false; `2` crashes the game.

**Not re-iterable.** `AssetManager.LoadDataDictionary` (`asset_manager.go:341-356`) deliberately does not cache the dictionary: "The underlying csv.Reader does not implement io.Seeker, so after it has been iterated through, we cannot iterate through it again." The raw file bytes are cached instead; a re-read constructs a new `DataDictionary`.

**TXT or BIN?** TXT only. Searching the tree for `.bin` yields two comments in `d2tbl/text_dictionary.go` (`:133`, `:218`) about the *original* game's string-key indices; no code opens a `.bin`, and every data path in `d2resource` ends in `.txt`.

## Which tables this fork reads

`d2core/d2records/record_manager.go` `init()` binds **86** path→loader pairs at `:181-266`. `AddLoader` (`:282-290`) appends into `boundLoaders map[string][]recordLoader` and the comment at `:39` says "there can be more than one loader bound for a file" — but **as of `c26dc732` no path has more than one loader**; every path constant and every resolved file path is unique. `Load` (`:293-324`) runs all loaders bound to a path in order. **83** of the 86 are bootstrapped at startup (see "Load order"); three are bound but never loaded.

Paths below are `/data/global/excel/…` unless noted.

**Levels** — LevelType `LvlTypes.txt` `levelTypesLoader`→`Level.Types` · LevelPreset `LvlPrest.txt` `levelPresetLoader`→`Level.Presets` · LevelWarp `LvlWarp.txt` `levelWarpsLoader`→`Level.Warp` · LevelDetails `Levels.txt` `levelDetailsLoader`→`Level.Details` · LevelMaze `LvlMaze.txt` `levelMazeDetailsLoader`→`Level.Maze` · LevelSubstitutions `LvlSub.txt` `levelSubstitutionsLoader`→`Level.Sub` · AutoMap `AutoMap.txt` `autoMapLoader`→`Level.AutoMaps`

**Monsters** — MonStats `monstats.txt` `monsterStatsLoader`→`Monster.Stats` · MonStats2 `monstats2.txt` `monsterStats2Loader`→`Monster.Stats2` · MonPreset `monpreset.txt` `monsterPresetLoader`→`Monster.Presets` · MonProp `Monprop.txt` `monsterPropertiesLoader`→`Monster.Props` · MonType `Montype.txt` `monsterTypesLoader`→`Monster.Types` · MonMode `monmode.txt` `monsterModeLoader`→`Monster.Modes` · MonsterAI `monai.txt` `monsterAiLoader`→`Monster.AI` · MonsterEquipment `monequip.txt` `monsterEquipmentLoader`→`Monster.Equipment` · MonsterLevel `monlvl.txt` `monsterLevelsLoader` (defined in `monster_levels_record.go:7`)→`Monster.Levels` · MonsterSound `monsounds.txt` `monsterSoundsLoader`→`Monster.Sounds` · MonsterSequence `monseq.txt` `monsterSequencesLoader`→`Monster.Sequences` · MonsterPlacement `MonPlace.txt` `monsterPlacementsLoader`→`Monster.Placements` · SuperUniques `SuperUniques.txt` `monsterSuperUniqeLoader`→`Monster.Unique.Super` · MonsterUniqueModifier `monumod.txt` `monsterUniqModifiersLoader`→`Monster.Unique.Mods` + `.Constants` · UniqueAppellation `UniqueAppellation.txt` `uniqueAppellationsLoader`→`Monster.Unique.Appellations` · UniquePrefix / UniqueSuffix `UniquePrefix.txt`/`UniqueSuffix.txt` `uniqueMonsterPrefixLoader`/`uniqueMonsterSuffixLoader`→`Monster.Name.Prefix`/`.Suffix` · PetType `pettype.txt` `petTypesLoader`→`PetTypes` · NPC `npc.txt` `npcLoader`→`NPCs`

**Objects** — ObjectType `objtype.txt` `objectTypesLoader`→`Object.Types` · ObjectDetails `Objects.txt` `objectDetailsLoader`→`Object.Details` · ObjectMode `ObjMode.txt` `objectModesLoader`→`Object.Modes` · ObjectGroup `objgroup.txt` `objectGroupsLoader`→**nothing** (records built then discarded, `object_groups_loader.go:12-36`) · Shrines `shrines.txt` `shrineLoader`→`Object.Shrines`

**Items** — Weapons `weapons.txt`→`Item.Weapons` · Armor `armor.txt`→`Item.Armors` · Misc `misc.txt`→`Item.Misc` (all three via `loadCommonItems`) · Books `books.txt`→`Item.Books` · Belts `belts.txt`→`Item.Belts` · ItemTypes `ItemTypes.txt`→`Item.Types` + `Item.Equivalency` · UniqueItems `UniqueItems.txt`→`Item.Unique` · MagicPrefix/MagicSuffix→`Item.Magic.Prefix`/`.Suffix` + `MagicPrefixGroups`/`MagicSuffixGroups` · RarePrefix/RareSuffix→`Item.Rare.Prefix`/`.Suffix` · ItemStatCost→`Item.Stats` · ItemRatio `itemratio.txt`→`Item.Ratios` · QualityItems→`Item.Quality` · LowQualityItems→`Item.LowQualityPrefixes` · Runes `runes.txt`→`Item.Runewords` · Sets/SetItems→`Item.Sets`/`Item.SetItems` · Gems→`Item.Gems` · AutoMagic `automagic.txt`→`Item.AutoMagic` · CubeRecipes `cubemain.txt`→`Item.Cube.Recipes` · CubeModifier `CubeMod.txt`→`Item.Cube.Modifiers` · CubeType `CubeType.txt`→`Item.Cube.Types` · TreasureClass/TreasureClassEx→`Item.Treasure.Normal`/`.Expansion` · StorePage→`Item.StorePages` · Gamble `gamble.txt`→`Gamble` · Inventory `inventory.txt`→`Layout.Inventory` · BodyLocations `bodylocs.txt`→`BodyLocations` · Properties→`Properties` · Colors `colors.txt`→`Colors`

**Skills / missiles** — Skills `skills.txt`→`Skill.Details` · SkillDesc `skilldesc.txt`→`Skill.Descriptions` · SkillCalc `skillcalc.txt`→`Calculation.Skills` · MissileCalc `misscalc.txt`→`Calculation.Missiles` (both via `loadCalculations`) · Missiles `Missiles.txt`→`Missiles` + `missilesByName`

**Sounds** — SoundSettings `Sounds.txt` `soundDetailsLoader`→`Sound.Details` · SoundEnvirons `soundenviron.txt`→`Sound.Environment`

**Characters / misc** — CharStats→`Character.Stats` · Experience→`Character.Experience` + `Character.MaxLevel` · PlayerClass→`Character.Classes` · PlrMode `PlrMode.txt`→`Character.Modes` · Events→`Character.Events` · Hireling/HirelingDescription `hireling.txt`/`HireDesc.txt`→`Hireling.Details`/`.Descriptions` · DifficultyLevels→`DifficultyLevels` · Overlays `Overlay.txt`→`Layout.Overlays` · States→`States` · ElemType `ElemTypes.txt`→`ElemTypes` · CompCode `compcode.txt`→`ComponentCodes`

**Animation-mode tokens** (all commented `// anim mode tokens` at `record_manager.go:256-260`) — ArmorType `ArmType.txt`→`Animation.Token.Armor` · WeaponClass `WeaponClass.txt`→`Animation.Token.Weapon` · PlayerType `PlrType.txt`→`Animation.Token.Player` · Composite `Composit.txt`→`Animation.Token.Composite` · HitClass `HitClass.txt`→`Animation.Token.HitClass`. Each stores only `{Name, Token}` keyed by Name (`armor_type_loader.go:11-17`, `hit_class_loader.go:11-17` reads `"Hit Class"`/`"Code"`). **Nothing in the codebase ever reads `Records.Animation.Token.*` — the five tables are parsed and dead.**

## Column schemas for the tables this game will touch first

Only columns the loaders actually read.

**monstats.txt** — `monster_stats_loader.go:9-280`, keyed by `Id`. Identity/graphics: `Id, hcIdx, BaseId, NextInClass, TransLvl, NameStr, MonStatsEx, MonProp, MonType, AI, DescStr, Code`. Flags: `enabled, rangedtype, placespawn, SetBoss, BossXfer, isSpawn, isMelee, npc, interact, inventory, inTown, lUndead, hUndead, demon, flying, opendoors, boss, primeevil, killable, switchai, noAura, nomultishot, neverCount, petIgnore, deathDmg, genericSpawn, noRatio, NoShldBlock, SplGetModeChart, SplEndGeneric, SplClientEnd`. Spawning: `spawn, spawnx, spawny, spawnmode, minion1, minion2, PartyMin, PartyMax, MinGrp, MaxGrp, sparsePopulate, Rarity`. Movement: `Velocity, Run`. Level/AI: `Level, Level(N), Level(H), threat, aidel, aidel(N), aidel(H), aidist, aidist(N), aidist(H), aip1–aip8, aip1(N)–aip8(N), aip1(H)–aip8(H)`. Sound: `MonSound, UMonSound`. Missiles: `MissA1, MissA2, MissS1–MissS4, MissC, MissSQ`. Skills: `Skill1–Skill8, Sk1mode–Sk8mode, Sk1lvl–Sk8lvl, SkillDamage`. Defense: `Drain, Drain(N), Drain(H), coldeffect(+N/H), ResDm, ResMa, ResFi, ResLi, ResCo, ResPo` (each ×3 difficulties), `DamageRegen, ToBlock(+N/H), Crit, minHP/MinHP(N)/MinHP(H), maxHP/MaxHP(N)/MaxHP(H), AC/AC(N)/AC(H), Exp/Exp(N)/Exp(H)`. Attacks: `A1MinD/A1MaxD/A2MinD/A2MaxD/S1MinD/S1MaxD` and `A1TH/A2TH/S1TH`, each ×3 difficulties. Elemental: `El1Mode–El3Mode, El1Type–El3Type, El1Pct–El3Pct, El1MinD–El3MinD, El1MaxD–El3MaxD, El1Dur–El3Dur`, each ×3 difficulties. Loot: `TreasureClass1–4` ×3 difficulties, `TCQuestId, TCQuestCP`. Misc: `Align, SplEndDeath`.

**monstats2.txt** — `monster_stats2_loader.go:13-170`, keyed by `Id`. `Id, Height, OverlayHeight, pixHeight, SizeX, SizeY, spawnCol, MeleeRng, BaseW, HitClass, TotalPieces`; equipment lists `HDv, TRv, LGv, Rav, Lav, RHv, LHv, SHv, S1v–S8v` (via `List`); component flags `HD, TR, LG, RA, LA, RH, LH, SH, S1–S8` (via `Bool`); animation-mode flags `mDT, mNU, mWL, mGH, mA1, mA2, mBL, mSC, mS1–mS4, mDD, mKB, mSQ, mRN` and direction counts `dDT…dRN` (same 16 order); move-while flags `A1mv, A2mv, SCmv, S1mv–S4mv`; hit-test `noGfxHitTest, htTop, htLeft, htWidth, htHeight`; selection `isSel, alSel, shiftSel, noSel, corpseSel, isAtt, revive`; body `critter, small, large, soft, inert, objCol, deadCol, unflatDead, Shadow, noUniqueShift, compositeDeath`; render `restore, automapCel, noMap, noOvly, localBlood, Bleed, Light, light-r, light-g, light-b, Utrans, Utrans(N), Utrans(H)`; misc `Heart, BodyPart, InfernoLen, InfernoAnim, InfernoRollback, ResurrectMode, ResurrectSkill`.

**levels.txt** — `level_details_loader.go:10-173`, keyed by `Id`. `Name ` (**trailing space**), `Id, Pal, Act, QuestFlag, QuestFlagEx, Layer, SizeX, SizeY, SizeX(N), SizeY(N), SizeX(H), SizeY(H), OffsetX, OffsetY, Depend, Teleport, Rain, Mud, NoPer, LOSDraw, FloorFilter, BlankScreen, DrawEdges, IsInside, DrlgType, LevelType, SubType, SubTheme, SubWaypoint, SubShrine, Vis0–Vis7, Warp0–Warp7, Intensity, Red, Green, Blue, Portal, Position, SaveMonsters, Quest, WarpDist, MonLvl1, MonLvl2, MonLvl3, MonLvl1Ex, MonLvl2Ex, MonLvl3Ex, MonDen, MonDen(N), MonDen(H), MonUMin, MonUMin(N), MonUMin(H), MonUMax, MonUMax(N), MonUMax(H), MonWndr, MonSpcWalk, NumMon, mon1–mon10, nmon1–nmon10, rangedspawn, umon1–umon10, cmon1–cmon4, cpct1–cpct4, SoundEnv, Waypoint, LevelName, LevelWarp, EntryFile, ObjGrp0–ObjGrp7, ObjPrb0–ObjPrb7`.

**LvlTypes.txt** — `level_types_loader.go:8-63`: `File 1`…`File 32` (DT1 filenames), `Name, Id, Act, Beta, Expansion`. Records are **appended to a slice**, so index = row order, not `Id`.

**LvlPrest.txt** — `level_presets_loader.go:8-56`, keyed by `Def`: `Name, Def, LevelId, Populate, Logicals, Outdoors, Animate, KillEdge, FillBlanks, SizeX, SizeY, AutoMap, Scan, Pops, PopPad, files, File1–File6` (DS1 filenames), `Dt1Mask, Beta, Expansion`.

**objects.txt** — `object_details_loader.go:8-232`, keyed by row index (`Index: i`): `Name, description - not loaded, Id, Token, SpawnMax, Selectable0–7, TrapProb, SizeX, SizeY, nTgtFX, nTgtFY, nTgtBX, nTgtBY, FrameCnt0–7, FrameDelta0–7, CycleAnim0–7, Lit0–7, BlocksLight0–7, HasCollision0–7, IsAttackable0, Start0–7, EnvEffect, IsDoor, BlocksVis, Orientation, Trans, OrderFlag0–7, PreOperate, Mode0–7, Yoffset, Xoffset, Draw, Red, Green, Blue, HD, TR, LG, RA, LA, RH, LH, SH, S1–S8, TotalPieces, SubClass, Xspace, Yspace, NameOffset, MonsterOK, OperateRange, ShrineFunction, Restore, Parm0–7, Act, Lockable, Gore, Sync, Flicker, Damage, Beta, Overlay, CollisionSubst, Left, Top, Width, Height, OperateFn, PopulateFn, InitFn, ClientFn, RestoreVirgins, BlockMissile, DrawUnder, OpenWarp, AutoMap`.

**weapons.txt / armor.txt / misc.txt** — one shared reader, `item_common_loader.go:14-183`, keyed by `code`: `name, version, compactsave, rarity, spawnable, minac, maxac, absorbs, speed, reqstr, block, durability, nodurability, level, levelreq, cost, gamble cost, code, namestr, magic lvl, auto prefix, alternategfx, OpenBetaGfx, normcode, ubercode, ultracode, spelloffset, component, invwidth, invheight, hasinv, gemsockets, gemapplytype, flippyfile, invfile, uniqueinvfile, setinvfile, rArm, lArm, Torso, Legs, rSPad, lSPad, useable, throwable, stackable, minstack, maxstack, type, type2, dropsound, dropsfxframe, usesound, unique, transparent, transtbl, quivered, lightradius, belt, quest, missiletype, durwarning, qntwarning, mindam, maxdam, StrBonus, DexBonus, gemoffset, bitfield1, Source Art, Game Art, Transform, InvTrans, SkipName, NightmareUpgrade, HellUpgrade, Nameable, 1or2handed, 2handed, 2handmindam, 2handmaxdam, minmisdam, maxmisdam, misspeed, rangeadder, reqdex, wclass, 2handedwclass, hit class, spawnstack, special, questdiffcheck, PermStoreItem, szFlavorText, Transmogrify, TMogType, TMogMin, TMogMax, autobelt, spellicon, pSpell, state, cstate1, cstate2, len, spelldesc, spelldescstr, spelldesccalc, BetterGem, multibuy`, plus `stat0–stat2` / `calc0–calc2` (`:222-230`) and, for each of 17 vendors (`Charsi, Gheed, Akara, Fara, Lysander, Drognan, Hralti, Alkor, Ormus, Elzix, Asheara, Cain, Halbu, Jamella, Larzuk, Malah, Drehya`), `<Vendor>Min, <Vendor>Max, <Vendor>MagicMin, <Vendor>MagicMax, <Vendor>MagicLvl` (`:185-220`).

**sounds.txt** — `sound_details_loader.go:8-50`, keyed by `Sound`: `Sound, Index, FileName, Volume, Group Size, Loop, Fade In, Fade Out, Defer Inst, Stop Inst, Duration, Compound, Reverb, Falloff, Cache, Async Only, Priority, Stream, Stereo, Tracking, Solo, Music Vol, Block 1, Block 2, Block 3` (note the spaces in header names).

**Animation-mode tokens** — `monmode.txt` (`monster_mode_loader.go:11-17`) reads `name, token, code`. The animation-mode strings themselves are **not** table-driven: they come from the stringer on `d2enum.MonsterAnimationMode` (`d2common/d2enum/monster_animation_mode.go:9-27`: DT, NU, WL, GH, A1, A2, BL, SC, S1–S4, DD, GH, xx, RN). `monstats2.txt`'s `ResurrectMode` is validated against a 3-entry map — Neutral, Skill1, Sequence only (`monster_stats2_loader.go:173-186`); anything else aborts the load with `unhandled MonsterAnimationMode`.

## Cross-table foreign keys

- **monstats → monstats2:** `d2mapentity/factory.go:189` `monstatEx: f.asset.Records.Monster.Stats2[monstat.ExtraDataKey]` — the join key is monstats' **`MonStatsEx` column** (`monster_stats_loader.go:20`) against monstats2's `Id` (`monster_stats2_loader.go:23`), *not* `Id`↔`Id`.
- **monstats → animation token → COF → DCC:** `AnimationDirectoryToken` is monstats' **`Code`** column (`monster_stats_loader.go:25`); `factory.go:194-195` calls `LoadComposite(d2enum.ObjectTypeCharacter, monstat.AnimationDirectoryToken, PaletteUnits)`, which resolves a base path of `/data/global/monsters` (`d2asset/composite.go:374-385`; players `/data/global/chars`, objects `/data/global/objects`). The COF path is built at `composite.go:258`: `"%s/%s/COF/%s%s%s.COF"` = base/token/COF/token+mode+weaponclass; the animdata key is `strings.ToUpper(token + mode + weaponClass)` (`:268-270`). Weapon class comes from monstats2's **`BaseW`** (`factory.go:207-208`). Equipment strings come from monstats2's 16 `*v` list columns, one random pick each (`factory.go:192-196`), defaulting to `"lit"` when empty (`composite.go:288-291`).
- **DS1 → monpreset → monstats:** `d2mapstamp/stamp.go:113-115` `monPreset := …Monster.Presets[mr.ds1.Act][object.ID]` then `monstat := …Monster.Stats[monPreset]`; a nil monstat is skipped with the comment "it is a place_ type object, idk how to handle those yet."
- **levels → lvltypes → DT1; lvlprest → DS1:** `d2mapstamp/factory.go:47-48` `levelType: *f.asset.Records.Level.Types[levelType]` (index, not Id) and `levelPreset: f.asset.Records.Level.Presets[levelPreset]`; `:51-61` iterates `levelType.Files` skipping `""` and `"0"` and calls `LoadDT1`; `:63-95` collects non-empty `levelPreset.Files`, picks one at random unless `fileIndex` is in range, then `LoadFile("/data/global/tiles/" + regionPath)` → `d2ds1.Unmarshal`. `panic("no level files to pick from")` if the preset has none (`:80`). Level details are fetched by ID via `RecordManager.GetLevelDetails` (linear scan, `record_manager.go:337-345`) — used by `d2mapgen/act1_overworld.go:41,87,153,207,274`; `LevelPreset(id)` (`:348-356`) is a linear scan that **panics** on an unknown ID, called from `d2mapgen/map_generator.go:40`.
- **DS1 object → objects.txt → object token:** `stamp.go:134-139` `LookupObject(act, type, id)` (hardcoded table `object_lookup_record_data.go`) → `Object.Details[lookup.ObjectsTxtId]`; then `d2mapentity/factory.go:275-278` `objectType := f.asset.Records.Object.Types[objectRec.Index]` and `LoadComposite(ObjectTypeItem, objectType.Token, …)`. `LookupObject` is fatal on a miss (`record_manager.go:409-416`).
- **items → ItemTypes → equivalency:** `item_types_loader.go:88-121` walks `Equiv1`/`Equiv2` recursively from each item's `type`/`type2` to build `Item.Equivalency`; `updateEquivalencies` **`log.Fatal`s** on an unknown item type (`:126-128`, message "invalid data file. Please ensure, you're using the newest patch_d2.mpq file!"). Reverse lookup `FindEquivalentTypesByItemCommonRecord` (`record_manager.go:360-383`) memoizes into `Item.EquivalenceByRecord`.
- **treasure classes:** `d2item/diablo2item/item_factory.go:263` `f.asset.Records.Item.Treasure.Normal[picked.Code]` — only `Item.Treasure.Normal` (TreasureClass.txt) is consulted; `Item.Treasure.Expansion` (TreasureClassEx.txt) is loaded and never read. Monster loot keys (`TreasureClass1…4`) are stored on `MonStatRecord` but no code resolves them.
- **strings:** `TranslateString(monstat.NameString)` (`factory.go:222`) and `TranslateString(objectRec.Name)` (`factory.go:272`) are the joins from TXT into the string TBLs.

## Load order and ordering hazards

`d2app/initialization.go:92-139` `initDataDictionaries()` iterates a hardcoded `dictPaths` slice of **83** entries (`:93-122`), calling `asset.LoadRecords(path)` in order (`:126-131`) — this is the **only** caller of `LoadRecords` in the tree. Then `initAnimationData(d2resource.AnimationData)` loads `/data/global/animdata.d2` into `Records.Animation.Data` (`:133-137`, `:145-163`). `initialize()` calls `initLanguage()` **before** `initDataDictionaries()` (`:22-26`) and `loadStrings()` **after** the GUI/screen managers are up (`:47-49`).

Hazards:

1. **`itemTypesLoader` must run after weapons, armor and misc** — flagged inline: `{d2resource.ItemTypes, itemTypesLoader}, // WARN: needs to be after weapons, armor, and misc` (`record_manager.go:193`). It consumes `r.Item.All` (`item_types_loader.go:73`), which only exists post-merge.
2. **The `Item.All` merge is post-hoc and order-sensitive.** `RecordManager.Load` (`:307-321`) checks after *every* table load: `if r.Item.All == nil && r.Item.Armors != nil && r.Item.Weapons != nil && r.Item.Misc != nil` and only then merges Armors, Weapons, Misc into `Item.All` keyed by code. Anything loaded before all three lands is silently missing. `dictPaths` orders Weapons, Armor, Misc, Books before ItemTypes (`initialization.go:95-96`), which satisfies the constraint by luck of list order, not by any dependency mechanism.
3. **Three loaders are bound but never bootstrapped:** `d2resource.Belts`, `d2resource.Gamble` and `d2resource.ObjectMode` appear in `record_manager.go` (`:191`, `:210`, `:186`) but not in `dictPaths` — `Item.Belts`, `Gamble` and `Object.Modes` are nil at runtime unless something calls `LoadRecords` for them (nothing does).
4. `armorLoader` alone guards against a double load: `if r.Item.Armors != nil { return nil }` (`item_armor_loader.go:10-12`). Weapons and misc have no such guard.
5. Failure semantics differ per loader: most return `d.Err`, but `monsterStats2Loader` (`:161-163`), `armorTypesLoader`, `compositeTypeLoader`, `hitClassLoader`, `playerTypeLoader` and `weaponClassesLoader` **`panic(d.Err)`**, and `objectGroupsLoader` panics on out-of-range density/probability (`object_groups_loader.go:44-58`). Any error from `LoadRecords` aborts startup (`initialization.go:128-130`).

## Edge cases the decoders already handle

- `StreamReader.ReadBytes` bounds-checks and returns `io.EOF` rather than panicking (`d2datautils/stream_reader.go:108-122`); `ReadByte` likewise (`:29-38`). The font glyph loop relies on this for termination (`font.go:150-153`).
- `d2font.Load` rejects a wrong magic before touching anything else (`font.go:45-47`); `AssetManager.LoadFont` wraps the error with the table path (`asset_manager.go:232-234`).
- `TextDictionary` tolerates inactive hash slots (`text_dictionary.go:52-56`) and a hash table larger than the live entry count.
- `d2txt.Next` distinguishes clean EOF from a real error and preserves the error for the caller (`data_dictionary.go:48-54`); every loader checks `d.Err` after the loop.
- `Number` never panics on garbage cells (`:69-76`), so partially-filled D2 tables load.
- `LoadDataDictionary` is explicitly not cached because the CSV reader can't seek (`asset_manager.go:341-347`).
- `LoadStamp` skips `""` and `"0"` placeholder filenames in both LvlTypes and LvlPrest (`d2mapstamp/factory.go:52-53`, `:66-69`).
- `TranslateString` degrades to returning the key rather than crashing on a missing string (`asset_manager.go:310-312`).
- `objectGroupsLoader` skips the `"EXPANSION"` group-name marker (`object_groups_loader.go:16-18`, constant at `object_groups_record.go:9`) — a second, uppercase variant of the row skip `d2txt` already does for `"Expansion"`.

## Known gotchas

1. **`DataDictionary.String` silently returns column 0 for an unknown column.** `d.record[d.lookup[field]]` (`data_dictionary.go:65`) — a missing map key yields index `0`. Typos and schema drift produce the first column's value, never an error. Every `Number` built on it then returns 0 via `Atoi` failure. This is the single most dangerous behaviour for anyone authoring replacement tables.
2. **`Bool` crashes on any value above 1** (`:85-92`, `log.Panic`), while `Number` swallows everything. A `2` in `monstats2.txt`'s `HD` column kills the process; a `2` in `Level` does not.
3. **A malformed header panics at load** (`:26-29`) — no error return, no recovery.
4. **levels.txt Hell monster IDs read the Nightmare columns.** `level_details_loader.go:110-119` assigns `MonsterID1Hell … MonsterID10Hell` from `d.String("nmon1")…("nmon10")`, identical to the Nightmare block at `:100-109`. The `hmon*` columns are never read.
5. **`SaveMerchantStates` reads `SaveMonsters`** (`level_details_loader.go:68-69`) — both fields get the same column.
6. **`EnablePerspective: d.Number("NoPer") > 0`** (`:34`) — the field name is the logical inverse of the column name; the code does not negate.
7. **`Name ` in levels.txt has a trailing space** (`:15`). Combined with gotcha 1, an authored table with `Name` (no space) will silently read column 0 instead of failing.
8. **`d2font.Marshal` does not round-trip `d2font.Load`.** `Load` skips 5 magic + 7 unknown = `numHeaderBytes` 12 (`font.go:19,40,54`); `Marshal` writes 5 + 6 + 2 = 13 (`:193-200`). Glyph order is also map-iteration order (`:202`). `Marshal` has no test and no non-test caller.
9. **TBL `Marshal` round-trips `#N` keys only by accident.** A `#N` key is written as literal `"x"` (`:227-232`, `:244-245`) and re-read as `"#" + hashEntryIndex` (`:92-94`). Since `Marshal` iterates a Go map, two or more `#N` keys will come back renumbered by position. `key[0]` also panics on an empty-string key (`:227`).
10. **The TBL `Index`, `HashValue`, `elementIndex` and the CRC are all decoded and discarded** (`:24-33`, `:160-166`, `:124`) — hash lookup and integrity checking are not implemented; the map is rebuilt in memory.
11. **Keys are assembled with `key += string(b)` per byte** (`text_dictionary.go:89`), which UTF-8-encodes each byte as a code point; any key byte ≥ 0x80 is silently re-encoded to two bytes. Values, read as `string([]byte)` (`:73`), keep raw bytes. For non-LATIN charsets the two halves of a table disagree about encoding, and `Font.RenderText`/`GetTextMetrics` iterate runes (`font.go:88`, `:118`), so mismatched glyph keys just render nothing.
12. **String-table precedence is first-match-wins across a three-element slice** (`asset_manager.go:304-308`) with load order patch → expansion → base (`initialization.go:166-170`). Adding a table appends to the end, so a new table is the *lowest* priority.
13. **Five tables (`ArmType`, `WeaponClass`, `PlrType`, `Composit`, `HitClass`) and `objgroup.txt` are parsed and thrown away**, and `TreasureClassEx` is loaded but never queried.
14. `LevelTypes` is a slice indexed by row position (`level_types_loader.go:11,52`) while `LevelPresets`/`LevelDetails` are maps keyed by `Def`/`Id` — mixing them up gives an off-by-one region.
15. `RecordManager.LevelPreset(id)` **panics** on an unknown preset (`record_manager.go:355`), and `LookupObject` is **fatal** on a miss (`:412`).

## Worked example

`d2common/d2fileformats/d2tbl/text_dictionary_test.go` is the only executable specification of the string TBL, and it exercises `Marshal` → `LoadTextDictionary` as a round trip.

`exampleData()` (`:7-15`) builds a `*TextDictionary` with three pairs: `"abc"→"def"`, `"someStr"→"Some long string"`, `"teststring"→"TeStxwsas123 long strin122*8:wq"` (the third value deliberately contains an embedded `x` and punctuation, proving the `x`/`X` rewrite is keyed on the *whole key*, not a substring, and that values are length-delimited rather than terminator-scanned).

`TestTBL_Marshal` (`:17-37`) calls `tbl.Marshal()`. Field by field, the produced buffer is: 2 zero bytes standing in for the CRC (`text_dictionary.go:183` — the loader skips them unverified, so zeroes are fine); `uint16(0)` for the element count (`:185`), which makes the loader's `elementIndex` loop a no-op; `uint32(3)` for the hash table size (`:192`); a zero version byte (`:195`); and three zero `uint32`s for string offset, max retry count and file size (`:198-204`), all of which the loader reads and discards. Then, for each of the three keys in map order, a 17-byte hash entry: `1` for in-use (`:214`), `uint16(0)` for the unused string-key index, `uint32(0)` for the unused hash, a `uint32` key offset, a `uint32` value offset, and `uint16(len(value)+1)` for `NameLength` (`:237`) — the `+1` is exactly what the loader subtracts at `:68`. Offsets are computed from `dataPos := len(header) + 17*len(*td)` (`:210`) and advanced by `len(key)+1` then `len(value)+1` (`:230-235`). Finally the data block writes each key, a `0`, each value, a `0` (`:241-257`). The test then asserts every original key maps to its original value after `LoadTextDictionary` (`:26-36`).

`TestTBL_MarshalNoNameString` (`:39-62`) is the `#`-key case: a single-entry table `{"#0": "OKEY"}`. `Marshal` detects `key[0] == '#'` and writes the literal `"x"` plus its separator, advancing `dataPos` by 2 rather than `len(key)+1` (`:227-232`, `:244-245`); `LoadTextDictionary` reads back the key `"x"` and rewrites it to `"#" + strconv.Itoa(idx)` with `idx == 0` (`:92-94`), recovering `"#0"`. The test passes **only because there is exactly one entry**, so the hash-entry index coincides with the number in the key — see gotcha 9.
