# The World Editor (v0, M5.4)

Josh builds the world by clicking; Claude builds it by writing the same records.
The `.tmj` file is the contract between the two, and this screen is a **view onto
it**, not a second source of truth.

    go build -o OpenDiablo2.exe .
    ./OpenDiablo2.exe -editor                             # the village
    ./OpenDiablo2.exe -editor data/strigoi/maps/other.tmj # some other map
    ./OpenDiablo2.exe -editor=data/strigoi/maps/other.tmj # the same, with =

or press **WORLD EDITOR** on the main menu, which opens the village.

`-editor` is a flag that may stand alone or carry a map; `-map`, `-classic`,
`-hero`, `-fonts` and `-strings` all mean what they always meant, and `-editor`
is resolved after them, so `-editor -map other.tmj` edits the map the rest of the
launch would play.

## The keys

They are drawn along the top of the screen, and this list, `editorVerbs` in
`d2game/d2gamescreen/editor.go` and `Editor.OnKeyDown` are the same set.

| | |
|---|---|
| left-click (map) | select what is on the tile, or PLACE when a palette row is picked |
| left-click (palette) | pick a tab, or pick a piece to place |
| right-drag | pan. Dragging right moves the world right |
| wheel (map) | zoom about the cursor |
| wheel (palette) | scroll the list |
| `Tab` | next palette tab, wrapping |
| `G` | the tile grid on and off |
| `Escape` | cancel a placement, then clear the selection, then leave -- and on an unsaved map the first `Escape` asks, and a second one leaves the changes behind |
| `Ctrl+Z` / `Ctrl+Y` | undo / redo |
| `Ctrl+S` | save (the previous generation is kept beside it as `.bak`) |
| `Delete` | delete the selected object |
| `D` | duplicate the selected structure, one footprint east |
| `P` | playtest |

## What is on the screen

**The map** is drawn by the game's own `MapRenderer`, out of a real `MapEngine`
that the document is laid into by `d2mapgen.LayAuthoredMap` -- the same function
the game calls. After every edit the document is rendered back to `.tmj` bytes,
re-parsed by the engine's own `d2maptiled.Parse`, and laid in again. So what is
on screen is what the game would build from the file **as it now stands**, and
the editing view cannot drift from the playing view.

**The palette** down the right is `d2core/d2mappalette` drawn. It catalogues the
open map's own embedded tilesets, and nothing else: v0 places only pieces the
map's tileset already carries, because adding a tile to an embedded tileset is a
bigger change than v0 makes, and offering art it cannot actually place would be
exactly the misleading preview the palette package exists to prevent.

The three tabs that are **not in v0** -- Terrain, Creatures, People -- are still
shown, greyed, with their count and the first three lines of their reason;
clicking one puts the whole reason in the status line. The reasons are measured,
carry file and line citations, and live in `d2mappalette`'s `WhyNoTerrain`,
`WhyNoCreatures` and `WhyNoPeople`. A piece that cannot be placed is greyed the
same way and says why.

**The status area** along the bottom is four lines: the map's name and path, the
unsaved marker, the zoom, the kinds used against the loader's cap of 256 and the
undo depth; then the last thing that happened; then the validator -- either "the
game would take this map as it stands" or the number of problems and the first
one's text; and then, only when it is true, a loud line saying the map is
**SEALED**: nothing can walk from the `player_start` to the edge of the map.

**The placement ghost** is drawn on the tile under the cursor when a piece is
picked: the footprint it would really take, green when it may go there and red
with the reason when it may not. The ghost and the click ask the same function
(`Editor.canPlace`), so they cannot disagree. It refuses what the loader would
refuse: off the map, a structure or a wall already there, bare ground under a
footprint, and -- measured on 28 Sep 2026, after the first version let it
through -- a person standing inside the footprint, which the loader rejects
because a structure's footprint is forced blocked.

## Where a click lands

A structure's anchor is its footprint's **bottom corner**, and the footprint runs
BACK from it with `Max` exclusive (`d2maptiled/tiled.go:1013-1014`). The anchor
tile is therefore not itself covered. So the tile a designer clicks becomes the
footprint's front tile and the anchor is one past it on both axes -- which is
what `editorStructureAnchor` returns, and what
`TestEditorStructureAnchorIsOnePastTheClick` holds against every structure in the
shipped village.

## Saving, and playtesting

`Ctrl+S` validates first and **refuses to write a map the game would refuse**
(`d2mapedit.Doc.Save`), keeping the previous generation as a `.bak`. A map the
game refuses does not stop the game -- `d2mapgen` quietly builds Diablo II's Act 1
instead -- so a designer who saved a broken file would otherwise get a world his
file had nothing to do with.

`P` writes the document to `playtest-scratch.tmj` **beside** the authoring file
(the tileset's image paths are relative to the map) and starts a real game on it
through the hero screens. The authoring file is not touched, saved or not, so a
run's deaths and destroyed objects can never reach it.

## What v0 does not do

No dragging an object (place, select, delete and duplicate only), no groups, no
terrain painting, no adding art to the map's tileset, and no prompt to set aside
a `.tmj` that will not parse -- the editor refuses such a map and says why. Each
of those is a `defer` row with a milestone against its name in
`tools/reachcheck/register.go`.

## What the engine cannot do, stated plainly

The brief this was built from asks that unavailable capabilities be reported
rather than mocked up. Each line below was measured this burst; the citation is
where to check it.

- **There is no elevation, and no height of any kind.** No `z` exists anywhere:
  `Vector` is `{x, y}` and `WorldToOrtho` takes two arguments. What looks like
  height is (a) the pixel height of wall and structure art and (b) `tile.YAdjust`,
  a screen-pixel draw offset. The village's "high ground" is a darker floor tile
  (`tools/villagemap/main.go`). **No slope or cliff tile is offered, because
  shipping one labelled as elevation would be a lie about movement and sight.**
- **There is no rotation.** Flipped and rotated tiles are refused outright
  (`tiled.go:631`), and a structure or `inside` rect with a rotation is refused
  (`:987`, `:1054`). Worse, a rotation on a POINT object (a person) is **silently
  dropped** -- not refused -- so the editor must never write a facing as Tiled
  `rotation` and pretend it means something. The editor therefore offers no
  orientation control at all, not a reduced one.
- **There is no walkability-override region and no sight-block region.** Both are
  properties of the tileset TILE (`blocked`, `blocks_sight`), cached per gid and
  layer, not areas. Painting an area impassable means placing tiles that are.
- **There is no sub-tile collision.** The authored path writes one answer to all
  25 sub-tiles (`d2mapgen/authored.go:316-326`), and Tiled's Tile Collision
  Editor shapes are refused (`tiled.go:458`). The narrowest gap anything can walk
  through is **one whole tile**.
- **`inside` is not a spawn area -- it is the opposite.** It marks ground the
  night does NOT arrive on; arrivals are pushed outside it and have to come in by
  the gate. Calling it a spawn area would invert its meaning.
- **There is no shelter region.** Shelter is an effect on a dialogue answer,
  applied without reference to where anyone is standing.
- **There is no holy ground.** Zero implementation; the only trace in the tree is
  one journal entry's flavour text.
- **There is no autotiling and no transition tiles.** Nothing in the engine would
  read them.

## A gate is a GAP, not a see-through wall

Measured on 27 September in the running game, with a red control
(`strigoi-harness-runs\zz-gate-measure4.txt`, and
`Claude doc outputs\World Editor v0 - S0 Measurements - 27 Sep 2026.md`):

- a 3x3 module blocks exactly its 9 tiles, and a 6x6 exactly its 36;
- **a one-tile gap between two modules is fully walkable and passes sight**,
  while the module columns either side block both;
- closing that gap with one fence tile makes the far side unreachable, which is
  the control that proves the gap carried the route.

So a monastery is built as **square modules arranged with gaps**: the gate is a
gap, the enclosure is wall tiles, the yard is ordinary floor. This needs no
engine change at all.

**The trap to know about.** A structure's whole footprint is solid --
`Map.Blocked` returns true for any tile carrying a structure, whatever its
properties say (`tiled.go:246`), and `blocked=false` on a structure is refused
(`:886-890`). So a 3x3 "gate" module whose ART shows an open archway is a solid
3x3: you would see a gateway you cannot walk through. The Dealu monastery's gate
module ships exactly like this, in `open` and `closed` states, and its art
satisfies every engine rule -- so the palette will call it placeable and cannot
know better. **Its opening is decorative.** Build the passable gate as a gap.

## What has NOT been driven by a script

Said here rather than left to be assumed:

- **The mouse wheel and the right-drag pan have never been driven end to end by a
  test.** The harness has no wheel verb and no press-and-hold (37 tools, checked).
  The transform underneath them is unit-tested in `d2core/d2map/d2maprenderer`,
  the editor opens at a zoom that only works if the scale and the inverse agree,
  and the screenshots in `strigoi-harness-runs\editor-v0-shots\` show both
  working by hand -- but nobody has turned a wheel under a script.
- **`Ctrl+S` and `P` were not run by the acceptance script against the shipped
  village**, because both would write into the tree. `playtest/editor_test.go`
  runs them against a copy instead, and act 4 proves the authoring file survives
  a real game running on an edited map.
- **The WORLD EDITOR menu button is unreadable in the default game.** Not a fault
  of this burst: EVERY main-menu label is blank under Strigoi's own build and
  correct under `-classic` (BUG-27 in `docs/bugs.md`). Until that is fixed,
  `-editor` on the command line is the reliable way in.
