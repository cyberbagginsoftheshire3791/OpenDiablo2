# Fog of war (F1: black until explored; F2: the night closes it) -- 1 Oct 2026

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
with his torch lit recomputes once a tile step, not once a frame; between
steps the lit set is the one computed as he entered the tile -- the drawn
light follows him exactly, and the seen edge of his own torch can lag it by
under a tile. Measured over a whole dusk frame by frame
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
model: the sim (Notice, the combat resolver's lit/dark rule, spawns) reads
the light model as master does, so fog stays display only. Whenever the world
has just run, the view is the light model to the bit
(`TestTheViewIsTheLightWhereHeStands`). The sim's own half of BUG-110 is
Josh's to rule.

**The HUD asks the same question (BUG-107).** The overhead bars and the
game's enemy-bar list, the hover and talk label and its hit test, the corpse
marks, the tactical diamonds and click-to-strike all ask
`MapRenderer.Shows` / `ShowsEntity`, the predicate the entity draw asks: what
he does not see is not named, barred, marked or struck at -- and, since a
hidden creature is never hovered, never highlighted (the F1 review's C3).

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
| `memory_level` | 0.45 | the look; Josh's eye sets it at F1's launch |
| `memory_saturation` | 0.25 | the look; ditto |

## What it is not (yet)

- **Display only.** Fog never feeds Notice, Combat, Seek or Pursuit, and its
  light view never writes the light model. A beast he cannot see still sees
  him -- by night at 12 to 24 tiles, while he sees 1.5 to 5. That asymmetry
  is the design ("the night is the enemy"), not a bug. Proved twice: d2world's
  `TestFogNeverTouchesTheSim` (the light model's state is untouched by a
  night of fog updates; the plan's one-liner, fog calling `SetPlayer`, goes
  red) and the playtest `TestFogNeverTouchesTheSim` (fog on and off, one seed,
  a torch-lit walk at 23:00 and a fight: every system's world hash agrees,
  fog's and the ui's included, and the ui less its `bars` and `hover_label`
  is equal). (fd3c2e6d's docs said the hashes agreed "but fog's and the
  ui's": the test allowed those two to differ and asserted nothing about the
  ui -- the F2 review's C2. Since its B6 the ui's bars, which fog decides,
  are in the digest's process part, so fog reaches no world part at all.)
- **Fog off is master's frame**, at night with a torch too: one seeded frame
  at 22:45 and one at noon, 0 of 480,000 pixels differ from master `b84a7241`
  (`strigoi-harness-runs\wt-fog2\pix-compare.txt`; the control, master's
  night against the branch's noon, differs in 345,371).
- **Not saved (F3).** The explored grid is not in the world file: a load, "load
  last save" or a death's reload starts black. F3 adds the `fog` block and the
  world file's version 3.
- **No raised sight (F4).** Talents, gear, height and towers.
- **Known small gaps from the F1 review (1 Oct).**
  - *A hovered hidden creature flashed highlighted for one frame* when it came
    into view (the hover reached hidden entities). **Closed in F2:** the hover
    gate means a hidden creature is never hovered, so never highlighted.
  - *With fog off the DRAW is master's, the harness output is not quite:* the
    `fog` provider is always registered, so the digest carries a `fog` system
    and `strigoi_get_entity` always reports `shown`. Accepted (plan §3.12).
  - *The explored grid is reset only when the map's SIZE changes.* Harmless
    while every load is a new Game; F3 must key it on the map itself.
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
`TestFogAtNight` (F2's acts 5-9) and `TestFogNeverTouchesTheSim` (F2, two
launches).
