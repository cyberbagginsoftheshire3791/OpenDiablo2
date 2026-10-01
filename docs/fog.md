# Fog of war (F1: black until explored, by day) -- 1 Oct 2026

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
| `memory_level` | 0.45 | the look; Josh's eye sets it at F1's launch |
| `memory_saturation` | 0.25 | the look; ditto |

## What it is not (yet)

- **Display only.** Fog never feeds Notice, Combat, Seek or Pursuit. A beast he
  cannot see still sees him -- by night at 12 to 24 tiles, while he sees 1.5 to 5.
  That asymmetry is the design ("the night is the enemy"), not a bug.
- **No night (F2).** F1's rule is the day's at every hour: the sky fraction is
  pinned to 1. F2 shrinks sight at night to the dark radius (Josh's Q2: 1.5
  tiles, rising to about 4 under a full moon) plus what is lit, sees lit ground
  at any distance with a clear line (Q3), shows the enemies in his own fight
  (Q4), makes every squad an eye, and gates the HUD's five leaks (the
  overhead bars, the hover label, the corpse marks, the tactical diamonds and
  click-to-strike; BUG-107).
- **Not saved (F3).** The explored grid is not in the world file: a load, "load
  last save" or a death's reload starts black. F3 adds the `fog` block and the
  world file's version 3.
- **No raised sight (F4).** Talents, gear, height and towers.
- **Not on by default (F5).** It flips with the zoom's default, so the
  screenshot-based checks are re-baselined once.

## Cost

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
`playtest/fog_test.go` is the script.
