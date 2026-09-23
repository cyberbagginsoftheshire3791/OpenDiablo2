# Strigoi maps (M5.4)

Maps here are drawn in **Tiled** (free, https://www.mapeditor.org) and saved as
JSON (`.tmj`). The game builds the world from one when started with

    OpenDiablo2.exe -map data/strigoi/maps/village.tmj

(or `strigoi_start_game{map: ...}` in the harness). Without `-map` the game
still generates Diablo II's Act 1 world.

## village.tmj

The v0 village, written once by `go run ./tools/villagemap` and meant to be
edited in Tiled from now on (do not re-run the tool over an edited map). Its
layout is a **proposal** from S1 §9.1 and G4: a hasty ditch and wattle fence
(ADAPTED, per the 11 Sep ruling) with the gate to the south and the north-east
run left unfinished, the church and graves on the high ground at the north end,
a well on the green, timber houses on their yards, the smithy by the gate,
forest on the slope to the north-west. Josh decides the shape.

**Every image in `tiles/` is a PLACEHOLDER** — flat-coloured shapes drawn by
the tool so the map has something to show. Art is Josh's and GPT's lane; the
names say `placeholder-` so nobody mistakes them.

## Making tile art that drops in

- **Floors:** exactly **160×80** PNG, a diamond touching all four edges,
  transparent outside it.
- **Anything that stands up** (walls, houses, trees, fences): **160 wide**,
  **80 to 512 tall**. The bottom 80 pixels are the floor diamond it stands on;
  it is drawn over the people behind it and under the people in front.
- Replace a placeholder by saving over its file (same name, same size rules),
  or add a new tile to the tileset in Tiled.

## Rules the game enforces (it refuses a map that breaks them, and says why)

- Isometric, 160×80 tiles, fixed size, tile layer format **CSV**.
- Tile layers `floor` (required) and `walls`; object layer `objects`.
- Tilesets **embedded** in the map (Map → Embed Tileset).
- Tile properties: `blocked` (bool) and `blocks_sight` (bool; defaults to
  `blocked`). A fence or ditch is `blocked` but not `blocks_sight`.
- People are **point** objects: one `player_start`, and `npc` objects with a string
  property `monstat` (the D2 stand-in: `warriv1`, `kashya`, `charsi`, `akara`
  are the four speakers).
- **Structures** (anything bigger than a tile: a house) are **tile objects**
  on the objects layer. Their tileset tile carries two int properties,
  `footprint_w` and `footprint_h`; the art is (w + h) × 80 pixels wide,
  up to 768 tall, drawn bottom-centre on the footprint's bottom corner --
  where Tiled draws it. Snap to the grid, don't resize the object, keep
  the tileset's Object Alignment at bottom, and keep footprints square
  for now. The whole footprint is solid. The game draws a structure in
  80-pixel strips along its two front faces, so people walking past its
  sides are drawn in front of or behind it correctly.
  The village's houses are the strigoi-art renders
  (`data/strigoi/structures/`, provenance there).
- **`inside` rectangles** mark ground the night does not arrive on: whatever
  comes from the dark is placed outside every inside area and has to come in
  by the gate. The village's covers the fence ring (tiles 12-35). Draw it
  with the rectangle tool; every tile it touches is inside.
- Refused because the game would silently ignore them: hidden, translucent,
  offset, parallax or tinted layers; tile offsets; collision shapes drawn in
  the Tile Collision Editor (use `blocked`); animated tiles; flipped or rotated
  tiles; tile objects.

The full contract is the package comment in `d2core/d2map/d2maptiled/tiled.go`.
