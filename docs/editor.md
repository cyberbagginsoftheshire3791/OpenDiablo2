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
