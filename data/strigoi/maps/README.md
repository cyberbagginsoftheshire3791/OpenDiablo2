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
forest on the slope to the north-west. Josh decides the shape. Since fog of
war F4 (2 Oct 2026; the plan's Q7 on its default) a watchtower stands just
inside the gate (tile 25,34; `tiles/placeholder-watchtower.png`, a labelled
PLACEHOLDER) and the churchyard is the high ground (height 1).

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
  `building` (bool, the raid's R3a): whether a household's door may be
  beside it -- true by default for a structure that is not a tower, false
  for everything else; a building must be `blocked` and is never placed on
  the floor layer.
- Raised sight (fog of war F4, `docs/fog.md`): a **floor** tile may carry
  `height` (int 0..8): an eye standing on it sees 2 tiles further a level
  (the churchyard tile is `height` 1 -- the church and cemetery's high
  ground). A **1x1 structure** may carry `sight_radius` (int 1..64): it is a
  tower, an eye of its own (the `placeholder-watchtower` at the gate sees 16;
  it is `blocks_sight` false, so the beasts see past it as before).
  `height` anywhere but the floor, and `sight_radius` on anything but a 1x1
  structure, are refused.
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
- **The village's own objects** (the raid's R3a, 2 Oct 2026), all **point**
  objects:
  - **`household`**: placed on the household's **door tile** -- a walkable
    tile right beside (not diagonally) its building. **Buildings are marked**
    (the R3a review's B2, Josh's to overturn): a structure is a building
    unless its tile says `building` false or it is a tower (a
    `sight_radius`); a walls tile is a building only when its tile says
    `building` true -- the village marks `placeholder-house`,
    `placeholder-smithy`, `placeholder-church` and `placeholder-church-tower`.
    So a fence, a tree or the gate's watchtower is no building. A building
    drawn in walls tiles is the run of them touching side to side. Its **name** is what an npc's `household`
    names. Properties, all optional: `members` (string: the household's
    people by role, comma-separated: `man`, `woman`, `old`, `youth`,
    `child`; none is an empty house), `incense` and `stakes` (int 0..99:
    what the house starts with) and `church` (bool: the church, the one
    house that is always kept; one at most). One household to a building
    and to a door tile, and no two of one name.
  - **`hotar`**: the village's boundary, where it carries its dead out to.
    One at most, walkable, outside every `inside` area, no properties.
  - **`watch_post`**: where the watch stands; one string property `post`,
    `gate` or `corner`.
  - An **`npc`** may carry a string `household` naming its household, so
    the speakers are members of a house.
  - All three are **points** (Tiled's point tool): one drawn as a rectangle,
    an ellipse or a polygon is refused. A property left null is refused.

  **The village's are a proposal (the raid's R3a), Josh's to move:** a
  household at the door of each of the six peasant houses and the church's
  (the church `16,16`, the headman's `28,21`, the well house `27,27`, the
  west house `17,27`, the smith's `20,31`, the north-east house `30,16`, the
  north house `24,16`; the burned house keeps none), each with 3 incense and
  2 stakes; the hotar on the road south of the gate (`22,40`); a gate post
  at `24,34` (by the tower) and a corner post at `33,13` (inside the
  north-east gap); and the speakers in their houses -- the headman's, the
  well house (the woman at the well), the smith's and the church (the
  priest). Nothing reads the houses yet but the households provider and the
  world save (R3b gives the members bodies).
- Refused because the game would silently ignore them: hidden, translucent,
  offset, parallax or tinted layers; tile offsets; collision shapes drawn in
  the Tile Collision Editor (use `blocked`); animated tiles; flipped or rotated
  tiles; tile objects.

The full contract is the package comment in `d2core/d2map/d2maptiled/tiled.go`.
