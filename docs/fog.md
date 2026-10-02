# Fog of war (F1: black until explored; F2: the night closes it; F3: kept; F4: raised sight) -- 1-2 Oct 2026

**Josh, 1 Oct 2026:** "Since we are zooming out, I think we should add a fog of
war with similar functionality to how Age of Empires 2 uses. Black until
explored with line of sight that can be upgraded and extended." The rulings are
`claude/rulings-2026-10-01-camera-scale-and-fog.md` §2 and the build plan is
`claude/fog-of-war-build-plan.md`; this is its first burst, **F1**, with Josh's
answer to Q1: **a squad sees 12 tiles by day**, the range beasts notice him at.

## What he sees

| A tile is... | It is drawn... | People and creatures on it |
|---|---|---|
| **unexplored** (never seen) | not at all: the screen is cleared black each frame, so it is exact black | not drawn |
| **explored, not seen now** (remembered) | greyed: saturation `memory_saturation` (0.25), brightness `min(its light, memory_level)` (0.45) -- a min, never a product, so at night nothing darkens twice | **not drawn** (ruling 2: "no creatures or people") |
| **visible** (seen now) | as it always was | drawn |

A tile is **visible** when it is his own tile, or when its centre is within his
day sight (**12** tiles) of the centre of HIS tile AND the straight line to it crosses no tile that
blocks sight strictly between -- the tile itself may block: a wall is seen by
its face, and what stands behind it is not. Seen once, a tile stays **explored**
for the rest of the game.

**He sees from the centre of his tile** (the review's B1). Fog is tile-sized
and recomputes only when he steps onto another tile, so what he sees is a
function of his tile alone; measured from his exact point it depended on where
he had ENTERED the tile.

The line is `MapEngine.TileSightClear` (`d2core/d2map/d2mapengine/tile_sight.go`):
a tile-stepped grid walk that asks each tile's centre subtile the map's ONE
sight rule (`sightBlocked`, the shipped `SightRule` the beasts' notice model
obeys too), so his eyes and theirs never disagree about what is opaque. Where
the line passes exactly through a tile corner it is stopped only if both tiles
beside the corner block (a diagonal wall is opaque; one post beside the
diagonal is not).

**A structure is seen whole.** A house's art stands on its front tiles, which
its own footprint hides from an eye behind or beside it, so a structure with
ANY footprint tile visible is visible whole (its art is drawn whole), and one
with any tile explored is remembered whole (greyed whole). The footprints are
the authored map's (`d2mapgen.LayAuthoredMap` -> `MapEngine.SetStructures`);
the plan had this in F2 and it was pulled into F1 on 1 Oct 2026, so the first
look shows no house missing.

Fog is in **world tiles**: the camera is not in its rule, so zooming out shows
more of the map's black and grey, never more of what he sees.

## The night (F2)

Josh's answers of 1 Oct 2026 (the plan's §7): **Q2** "1.5 tiles, rising to
about 4 under a full moon"; **Q3** lit ground with a clear line is seen at
any distance; **Q4** the enemies of his own fight are shown for as long as the
fight lasts; **Q5** (the default) villagers' eyes do not count.

A tile is **visible** to an eye (each at the centre of its tile) when it is the
eye's own tile, or when the straight line to it is clear (as above) AND either

- its centre is within the eye's **unlit reach** -- the day sight (12) by day,
  tonight's **dark radius** at deep night, and between them at dusk and dawn
  by the sky fraction, exactly as the light model's own radius blends
  (`Fog.UnlitReach`; the sky is quantised to 1/64, 0.16 tiles of reach); the
  dark radius is `lerp(dark_radius 1.5, moon_dark_radius 4, moon)`, the moon
  read from the world clock (`Clock.Moon`, the day table's illuminated
  fraction); or
- it is **lit**: drawn brighter than the sky (`Level > quantise(Ambient)`,
  never the raw ambient -- the level is quantised, so a raw comparison calls
  the whole night lit), **at any distance** (Q3). Fog finds lit ground by
  iterating the lit SOURCES' discs, never the whole map, and tries each lit
  tile's lines once a recompute (`Fog.seeLitGround`).

  *Art, not sight* (the F2 review's C6): a lit wall face seen from afar makes
  its house visible whole (the whole-structure reveal), so at night a lamp on
  one face across the village shows the whole house's ART -- its roof and far
  walls -- though only the lit face is what he can really see. Only the
  structure's own tiles are drawn; nothing standing in it is shown unless its
  own tile is seen.

**When fog recomputes at night.** The key is the eyes' tiles and the light:
the sky fraction in 1/64 steps, the sky AS DRAWN (the ambient's quantised
band -- the two step at different moments, and the lit set follows the band;
the F2 review's C1), the moon, the fixed sources exactly, and a CARRIED
source (his torch) by the TILE it shines from (the review's B4). So a walk
with his torch lit recomputes once a tile step, not once a frame. **Fog sees
by his torch from the centre of his tile** (the F3 review's A1): the disc it
iterates and the "lit" it asks (`LitCarriedAt`) both put the carried source
there, as his eye is there -- so what his own torch shows him is a function
of his tile alone. (Keyed by tile but lit from his exact point, the lit set was
the one computed where he ENTERED the tile, and a game saved mid-tile resumed
to other ground: 101 vs 110 explored, `TestAMidTileTorchSaveResumesToTheSameGrid`.)
The drawn light follows him exactly, and the seen edge of his own torch can
differ from the drawn one by under a tile. Measured over a whole dusk frame by frame
(`TestDuskKeepsTheLitSetAndItsCost`): 2,241 frames, 79 recomputes, and on
every frame the tiles fog sees by the lit term are exactly the tiles drawn
lit.

**Every one of his squads' LIVING models is an eye**
(`Squads.LivingModelEntities`: a deployed model at 0 health, or his own body
at 0, sees nothing -- the F2 review's B3; s:1 is the player). Villagers are
not (Q5).

**His fight's enemies are shown** (Q4): every enemy of his own fight that is
not gone (dead, routed or broke off at first light: `TacticalEnemy.Gone`) is
a **contact** -- it shows the tile it stands on and nothing else, so the
enemy is drawn (at its tile's own light) and its bar, diamond and click target
are there, lit or not. A contact's tile is SHOWN, not seen (the F2 review's
B1/B2): it is shown after the whole-structure reveal, so a wolf standing in a
house does not reveal the house, and it is never explored, so when the fight
ends ground he never saw is black again (the renderer draws a tile that is
visible though unexplored). A contact sees nothing for him: no line is tried
from it, not even to lit ground.

**The light he sees by is where he stands (BUG-110).** The light model's
carried torch shines from where the world last ran (`Light.SetPlayer` runs in
the gated `advanceWorld`), so during a held turn his torch stayed where the
turn opened while he walked his Move. Fog reads a `d2world.LightView` -- the
same light with his carried sources where he stands THIS frame -- and while
fog is drawn the renderer draws by the same view (`draws_by: view`), so "he
sees it" and "it is drawn lit" are one fact. The view never writes the light
model, so fog stays display only. Whenever the world has just run, the view
is the light model to the bit (`TestTheViewIsTheLightWhereHeStands`). **The
sim's own half of BUG-110 was fixed on 1 Oct (`harness-fixes`):** the game
tells the light model where he stands every frame, after the map moves him
(`Game.advanceTheMap`, `lightFollowsHim`) -- not only in the gated
`advanceWorld` -- so the combat resolver's lit/dark advantage in a held turn
reads his torch where he stands, and a fog-off game draws it there too
(`TestHisTorchFollowsHimThroughAHeldTurnForTheSim`, d2gamescreen). In a game
the view and the model now agree on every frame, so **the view is now a
guard, and it is kept**: fog and the renderer still read the light through
`LightView`, which never writes the model, so fog cannot reach the sim
whatever the game does with the model, and fog-f3 builds on the type
(`LitCarriedAt`). Do not remove it as redundant.

**The HUD asks the same question (BUG-107).** The overhead bars and the
game's enemy-bar list, the hover and talk label and its hit test, the corpse
marks, the tactical diamonds and click-to-strike all ask
`MapRenderer.Shows` / `ShowsEntity`, the predicate the entity draw asks: what
he does not see is not named, barred, marked or struck at -- and, since a
hidden creature is never hovered, never highlighted (the F1 review's C3).

## Raised sight (F4)

**Josh, 1 Oct 2026 (ruling 4):** sight is raised by "all of them" --
talents, structures, gear / squad type, and height. The plan's Q6-Q9 were
still open when F4 was built (2 Oct, Josh away), so each is **decided on its
recommended default, Josh's to overturn**:

| Q | Decided on the default | Built |
|---|---|---|
| **Q6** towers | no garrison; a tower sees its radius by day; at night, like a squad, only what is lit plus a short dark radius; a lit beacon on it lets it see by night | a structure with `sight_radius` is an eye of its own (`tower/<x>,<y>`), always; by night its unlit reach is tonight's dark radius; a light placed at it is lit ground the tower has a line to |
| **Q7** placement | a tower at the village gate, and the church / cemetery high ground at height 1 (G4's "cemetery on the high ground"); placeholder art, labelled | `village.tmj`: a 1x1 `placeholder-watchtower` at tile (25,34), just inside the fence east of the road through the gate, `sight_radius` 16; every churchyard floor tile (`placeholder-churchyard`, the church and the graves stand on it) `height` 1 |
| **Q8** talent | Night Eyes also widens his dark radius by 1 (it keeps its +10) | `talents.json`: Night Eyes `dark_sight: 1`; `dark_sight` is a known effect (additive); his eye's dark radius is tonight's + it |
| **Q9** gear | an equipped composite bow: +2; squad TYPES add theirs when squads have a type | `items.json`: the composite bow `sight: 2` (an item's optional `sight`, given only by what is in his hands); squad types are NOT built (nothing would set them) |

**An eye's sight** is its base -- the day sight (`day_sight`, 12) for a
squad, a tower's own `sight_radius` -- plus its gear, plus `height_tiles` (2)
for every level of the ground it stands on. **Its dark radius** is tonight's
(1.5 rising to 4 with the moon) plus its talent. Its unlit reach blends the
two by the sky exactly as before, never past its own sight. The rule is
otherwise F2's: lit ground with a clear line is seen at any distance, from any
eye, a tower's included.

- **Height adds radius only.** v1 does not see over blockers: a man on the
  high ground sees further, not over the church wall. The plan's §6.
- **A tower is 1x1 for now** (the parser refuses a larger one): it sees from
  its one tile, and a line is never read through the eye's own tile, so it
  sees past its own walls. A larger footprint would need its eye lifted out of
  its walls.
- **The beacon** is a light placed at the tower (today the light provider's
  `place_source`; map fires are authored after M4.7): its lit ground is seen
  by the tower's lines, and by anyone else's with a clear line (Q3).
- **The bow may be held, not shot.** Until F4 the kit refused a bow the hand
  ("shooting is not built yet"), which would have left Q9's +2 with nothing
  that could ever give it. He may now take it up out of a fight -- both hands,
  never past a lit torch, as any two-hander -- to look along it; it still does
  not shoot (`MainBite` gives no bite for a ranged weapon, so in a fight he
  strikes as with an empty hand). The kit panel shows it as "(+2 sight in
  hand; no shooting yet)". **Josh's to overturn** (the F4 report's question).
- **Talents and kit are his:** the other models of his squads have neither
  term; squad types are not built.
- **The map:** `height` (an int 0..8 on a FLOOR tile) and `sight_radius` (an
  int 1..64 on a 1x1 STRUCTURE) are read by the game (`d2maptiled`), the World
  Editor (`d2mapedit`, which also says "height 1" and "a tower: sees 16" for
  a clicked tile) and its palette; anything else is refused with a message.
  The edit was made through the editor's own records (the tileset tile added
  to the tree as Tiled writes it, the tower placed with `PlaceStructure`, the
  bytes `Doc.Checked` takes) -- a 49-line diff.
- **D5:** the map's SHA changed, so a world file saved on the old village is
  set aside when loaded. Measured 2 Oct 2026: his saves folder
  (`%APPDATA%\OpenDiablo2\Saves`, 8,059 files) holds `.od2` heroes and their
  `.od2.strigoi.json` sidecars and **no world file**, so nothing of his is set
  aside.
- **The world file's shape is unchanged** (the fog block is still the
  explored grid; sight is derived): **no version bump**, it stays version 4.

**The cost, answered.** F1-F2 measured 24 eyes at sight 16 at ~0.85 ms and
24 eyes with 16 lit hearths at ~0.98 ms, over the plan's 0.5 ms frame. F4 cuts
it two ways:

1. **Every eye's lines are cached per tile it stands on** (`eyeLines`): tiles
   never change after generation (plan §1.4) and an eye sees from its tile's
   centre, so a recompute for the sky, the moon, a lit source or a dial reads
   the cache, a tower walks its lines once a game, and only an eye that
   changed tile walks again. (`forget` drops the caches: the one way to make
   fog see a map whose tiles changed -- owed, with §3.9's memory, the day a
   house can burn.)
2. **The walk is recorded once per offset and replayed against fog's own
   grid.** The map's walk moved to `d2geom.TileLineReads` /
   `TileLineClear`, which `MapEngine.TileSightClear` still walks; fog reads
   the map's opacity once a map (`MapEngine.TileBlocksSight`) and, since from
   one tile's centre to another's the walk depends only on the offset,
   replays each offset's recorded reads from every eye. Same answer and the
   same cell count, to the bit (`TestFogsGridWalksTheMapsLines`, day and
   night, 24 eyes, 40 steps).

`BenchmarkFogRaisedSight` and the re-run F1/F2 benchmarks
(`d2mapgen/fog_bench_test.go`; 2 Oct 2026, Intel Core Ultra 7 258V; the real
village with its tower; every iteration EVERY eye steps a tile, the worst
frame, or -- "sky" -- the recompute is forced with nothing moving):

| frame | 1 eye | 6 eyes | 24 eyes |
|---|---|---|---|
| step, sight 12 (raised: bows, high ground, the tower) | 0.028 ms | 0.080 ms | 0.18 ms |
| step, sight 16 | 0.040 ms | 0.13 ms | **0.30 ms** |
| sky, sight 16 (every line cached) | 0.022 ms | 0.046 ms | 0.13 ms |
| night, 16 hearths lit, every eye stepping | 0.18 ms | 0.28 ms | **0.46 ms** |

(F1-F3's numbers for the same frames: 24 eyes at 16 0.85 ms, at night with
16 hearths 0.98 ms.) Every 24-eye frame is now under the 0.5 ms budget; the
night with 16 fires is the closest.

## Turning it on

| Control | Effect |
|---|---|
| `-fog` | every game this process starts has fog. **Off by default** (F1 is opt-in; F5 turns it on with the zoom's default). |
| `-classic` | never fog: Diablo II's game has none. |
| a network game | never fog (more than one player: shared sight has no rule yet). |
| the first frame | with `-fog` the renderer holds the fog from the game's first frame, before he exists: a fog that has seen nothing is all black, so no frame shows the village unfogged (the review's B5). |
| the World Editor | never fog: its renderer has no fog sampler. |
| harness `fog.enabled` | turns this game's fog off or on (not under `-classic`). A load is a new game at `-fog`'s value. |

**With fog off the frame is the frame master drew**: the renderer holds no
`FogSampler`, and a nil sampler is the unfogged draw call for call
(`TestNoFogSamplerIsTheUnfoggedDraw`; and on the real game, one seeded frame
at 0 of 480,000 pixels differing from master `5510ef56`).

## The dials

`d2world.FogDials` (`d2core/d2world/fog.go`), each settable on the harness's
`fog` provider:

| Dial | Value | Why |
|---|---|---|
| `day_sight` | **12** tiles | Josh's Q1: the same range beasts notice him at. At zoom 0.5 that reaches past the screen's sides (7.1 tiles) and nearly to its corners (12.7), so most of the black on screen is ground behind houses and walls; at 0.4 the corners (15.9) show it. |
| `dark_radius` | **1.5** tiles [DIAL] | Josh's Q2, the new-moon end: the light model's FloorRadius, S1 §4's "the tile they stand on and little else" |
| `moon_dark_radius` | **4** tiles [DIAL] | Josh's Q2, the full-moon end ("about 4"); tonight's is lerp(1.5, 4, moon) |
| `height_tiles` | **2** tiles a level [DIAL] | F4, the plan's §2.2: an eye on ground of height h sees 2h further (the churchyard, height 1: 14) |
| `memory_level` | 0.45 | the look; Josh's eye sets it at F1's launch |
| `memory_saturation` | 0.25 | the look; ditto |

## Kept (F3)

**The explored grid is saved** (fog of war F3, 1 Oct 2026; the build plan's
§4 F3 and §3.7). The world file -- **version 4** -- carries a `fog` block:

```json
"fog": { "map": "<the map's SHA-256>", "w": 48, "h": 48, "explored": "<base64>" }
```

the grid as bits (row-major, eight tiles a byte, lowest bit first; the
village's 2,304 tiles are 288 bytes, 384 characters), its size, and **the map
it was explored on** (the authored map's SHA-256, the file's own `map.sha`).
A load, "load last save" and a death's reload put it back: the ground he had
explored is remembered, greyed, and the first frame sees again from where he
stands.

- **Saved:** the explored grid. **Not saved:** what he sees now (derived --
  recomputed from his eyes on the first frame), the dials (this build's
  tuning, never the file's), the counters and the probe (this process's), and
  whether fog is drawn (`-fog` is the view, as `ui.zoom` is).
- **Keyed on the map** (the F1 review's C6): a grid of another map -- even of
  the right size -- is refused, and the file set aside (D5's rule); so is one
  of another size. A map edit already sets the whole file aside (D5).
- **Fog off saves an empty grid** (`"map": "", "w": 0, "h": 0, "explored":
  ""`): a game without `-fog` never looks. A grid loaded into a game without
  `-fog` is kept, undrawn, and saved again unchanged, so one launch without
  the switch never throws away the ground he had explored.
- **His older files:** a version-3 world file is set aside (`.v3.unread`)
  and he begins at dawn with his hero, kit and progress, as at the last
  bump. Measured on 1 Oct 2026: his saves folder held no world file, so
  nothing of his was set aside.
- **The harness:** the `fog` provider reports `saved` (true), `map` and
  `grid`; `map`, `w`, `h`, `explored` and `grid` are in the digest's world
  part, so a resumed game must reproduce them (`TestFogIsKept`, by day and
  at 23:00 with his torch lit, saved mid-tile).
- **`-fog` / `fog.enabled` changes the world part of the digest** (the F3
  review's C3): a fog-off game never explores, so resuming a save with a
  different `-fog` than the game that saved it gives S_R != S_U by design.

`d2core/d2world/fog_snapshot.go` (`Fog.Snapshot/Validate/Restore`,
`FogSnapshot.Check`); the world file's notes are
`docs/m4.6-world-save-notes.md`, "Fog of war F3".

## What it is not (yet)

- **Display only.** Fog never feeds Notice, Combat, Seek or Pursuit, and its
  light view never writes the light model. A beast he cannot see still sees
  him -- by night at 12 to 24 tiles, while he sees 1.5 to 5. That asymmetry
  is the design ("the night is the enemy"), not a bug. Proved twice: d2world's
  `TestFogNeverTouchesTheSim` (the light model's state is untouched by a
  night of fog updates; the plan's one-liner, fog calling `SetPlayer`, goes
  red) and the playtest `TestFogNeverTouchesTheSim` (fog on and off, one seed,
  a torch-lit walk at 23:00 and a fight: every system's world hash agrees,
  the ui's included -- fog's own since F3 differs by design, its explored
  grid being world state -- and the ui less its `bars` and `hover_label`
  is equal). (fd3c2e6d's docs said the hashes agreed "but fog's and the
  ui's": the test allowed those two to differ and asserted nothing about the
  ui -- the F2 review's C2. Since its B6 the ui's bars, which fog decides,
  are in the digest's process part, so fog reaches no world part at all.)
- **Fog off is master's frame**, at night with a torch too: one seeded frame
  at 22:45 and one at noon, 0 of 480,000 pixels differ from master `b84a7241`
  (`strigoi-harness-runs\wt-fog2\pix-compare.txt`; the control, master's
  night against the branch's noon, differs in 345,371).

- **Known small gaps from the F1 review (1 Oct).**
  - *A hovered hidden creature flashed highlighted for one frame* when it came
    into view (the hover reached hidden entities). **Closed in F2:** the hover
    gate means a hidden creature is never hovered, so never highlighted.
  - *With fog off the DRAW is master's, the harness output is not quite:* the
    `fog` provider is always registered, so the digest carries a `fog` system
    and `strigoi_get_entity` always reports `shown`. Accepted (plan §3.12).
  - *The explored grid is reset only when the map's SIZE changes.* Harmless
    while every load is a new Game. **Closed in F3:** the saved grid is keyed
    on the map's SHA, and a grid of another map is refused (below).
  - *No unit test yet* for a harness write being followed by a fog update, or
    for fog running after the map moves him (both need a live player and a
    full frame advance); the playtest covers the outcome, not the order.
- **Not on by default (F5).** It flips with the zoom's default, so the
  screenshot-based checks are re-baselined once.

## Cost (F2, at night)

`BenchmarkFogRecomputeAtNight` (the real village, deep night, new moon, his
torch lit, hearths in a ring 14 tiles out; 1 Oct 2026, the same machine):

| hearths | 1 eye | 6 eyes | 24 eyes |
|---|---|---|---|
| 0 (his torch only) | 0.004 ms | 0.004 ms | 0.004 ms |
| 4 | 0.10 ms | 0.25 ms | 0.53 ms |
| 16 | 0.27 ms | 0.54 ms | 0.98 ms |

The lit term (Q3, any distance) is a line from every eye to every lit tile:
24 eyes and 16 fires is over the plan's 0.5 ms frame budget, which F4 (the
24-eye horizon) must answer. Those are the frames on which something changed;
**a walk with his torch lit recomputes once a tile step** (the review's B4),
so `BenchmarkFogWalkWithATorch` -- a frame of a walk at ~4 tiles a second --
averages 0.0003 ms (his torch only), 0.006 ms (4 hearths) and 0.017 ms (16
hearths) a frame, with a recompute on 6% of frames (fd3c2e6d recomputed on
every one). With no torch, fog recomputes only when an eye changes tile, the
sky moves 1/64 or a band, the moon or a lit source changes.

## Cost (F1, by day)

`BenchmarkFogRecompute` (`d2core/d2map/d2mapgen/fog_bench_test.go`, the real
village; 1 Oct 2026, Intel Core Ultra 7 258V): one recompute with his one eye
at sight 12 is about **0.07 ms** (~3,800 tiles read); fog recomputes only when
he steps onto another tile (or a dial or the grid changes), and every other
frame is a comparison (`skipped`). For F2/F4's scale: 6 eyes at 12 about
0.20 ms, 24 eyes 0.38 ms; at sight 16, 1 / 6 / 24 eyes about 0.11 / 0.39 /
0.85 ms -- the last over the plan's 0.5 ms frame budget, which F4 must answer.

## For scripts

The `fog` provider (docs/harness.md) and `strigoi_get_entity`'s `shown` (false
only with fog on and the entity on ground he does not see now).
`playtest/fog_test.go` is the script: `TestFogOfWar` (F1, by day),
`TestFogAtNight` (F2's acts 5-9), `TestFogRaisedSight` (F4's acts 10-16:
the tower by day, by night and by its beacon, the high ground, the bow taken
up through the kit panel, Night Eyes taken through the talent panel, and
Josh's frame of the tower at night), `TestFogNeverTouchesTheSim` (F2, two
launches; since F3 fog's own world state is exempt -- with fog off nothing is
explored -- and required to differ) and `TestFogIsKept` (F3: explore, save,
resume, the same grid and the same day after; a night act, his torch lit,
saved mid-tile; the emptied block diverges; a grid of another map is
refused).
