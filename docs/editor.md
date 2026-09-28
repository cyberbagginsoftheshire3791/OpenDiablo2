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

**The file is fixed when it opens** (28 Sep review, B1). The path is made
absolute once, at open, and `Ctrl+S` writes THAT file whatever the working
directory is by then; the harness reports it. The game reads maps from its own
folder, so a map it can play lives under `data/strigoi/maps` beside the game: a
relative path means what it always meant, and an absolute path outside the
game's folders is refused with the reason, because the game could load neither
it nor its art. **The playtest harness refuses to open the working tree's own
`data/strigoi/maps/village.tmj`** (`-editor` alone, or the menu button): it runs
the game with the repository as its working directory, so a scripted `Ctrl+S`
would have written the shipped village. A script edits a copy and passes the
copy's absolute path.

## The keys

They are drawn along the top of the screen, and this list, `editorVerbs` in
`d2game/d2gamescreen/editor.go` and `Editor.OnKeyDown` are the same set.

| | |
|---|---|
| left-click (map) | select what is on the tile, or PLACE when a palette row is picked |
| left-click (palette) | pick a tab, or pick a piece to place |
| right-drag | pan. Dragging right moves the world right |
| wheel (map) | zoom about the cursor -- the art zooms with the map (since 28 Sep; before, only the positions did) |
| wheel (palette) | scroll the list |
| `Tab` | next palette tab, wrapping |
| `G` | the tile grid on and off |
| `Escape` | cancel a placement, then clear the selection, then leave -- and on an unsaved map the first `Escape` asks, and a second one leaves the changes behind |
| `Ctrl+Z` / `Ctrl+Y` | undo / redo |
| `Ctrl+S` | save, once the validator AND the game's own loader take the map (the previous generation is kept beside it as `.bak`) |
| `Delete` | delete the selected object |
| `D` | duplicate the selected structure, one footprint east |
| `P` | playtest: straight into a real game on the map as it stands, with a throwaway hero; when that game ends the editor comes back as you left it |

## What is on the screen

**The map** is drawn by the game's own `MapRenderer`, out of a real `MapEngine`
that the document is laid into by `d2mapgen.LayAuthoredMap` -- the same function
the game calls. After every edit the document is rendered back to `.tmj` bytes,
re-parsed by the engine's own `d2maptiled.Parse`, and laid in again. So what is
on screen is what the game would build from the file **as it now stands**, and
the editing view cannot drift from the playing view.

**The art zooms with the map** (28 Sep review, A1). The viewport scaled every
position from the editor-zoom burst on, and until the review nothing scaled the
pictures: the editor opens on the whole village at about 0.08, and that first
screen was full-size trees and houses on anchors a twelfth of their size apart,
every floor tile hanging (80(1-s), 40(1-s)) pixels off its own diamond and a
placed house hidden behind the forest. `MapRenderer.drawTileArt` now pushes the
zoom onto the surface for every floor, wall and shadow at any scale but 1.0,
with one pixel of overdraw so neighbouring tiles on whole-pixel anchors leave no
hairline seams. **At 1.0 -- the only scale the game ever draws at -- it is the
old `target.Render(img)` and nothing else.**

**People and the start are marked** (B5). The view is the engine's map with no
entities in it, so each person the map places gets his tile outlined and a
square where he stands, in blue, with his name beside it; the `player_start` is
an orange cross named "start". The marks are sized with the zoom, and names that
would land on one another are moved a line up or down with a leader line back to
the mark. They are selectable as they always were -- a click on the tile selects
the person on it -- and cannot be dragged (v1).

**A map the game refuses says so where you are looking** (C). The engine's own
parser runs on every open and every edit; when it refuses, a red notice sits in
the middle of the map area with the reason -- on open, "THE GAME REFUSES THIS
MAP -- it would build Diablo II's Act 1 instead"; after an edit, that the view
shows the last version it took -- and the status area's third line says the
same, where it used to repeat the validator's "the game would take this map".

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
unsaved marker (which follows the undo history: undo back to the saved map and
it goes out -- C), the zoom, the kinds used against the loader's cap of 256 and the
undo depth; then the last thing that happened; then the validator -- either "the
game would take this map as it stands" or the number of problems and the first
one's text; and then, only when it is true, a loud line saying the map is
**SEALED**: nothing can walk from the `player_start` to the edge of the map.

**The placement ghost** is drawn on the tile under the cursor when a piece is
picked: the footprint it would really take, green when it may go there and red
with the reason when it may not. The ghost and the click ask the same function
(`Editor.canPlace`), so they cannot disagree, and it judges the held piece by
ITS OWN palette tab, whatever tab is on show (B4: holding a house while reading
the greyed Terrain tab used to refuse every click with Terrain's reason). It refuses what the loader would
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

`Ctrl+S` **refuses to write a map the game would refuse**, and since the 28 Sep
review it asks twice (`d2mapedit.Doc.Save`, `Doc.Checked`): the validator, which
reproduces the loader's refusals and reads only a PNG's 24-byte header, and then
the game's OWN parser, `d2maptiled.Parse`, run with the game's loader over the
exact bytes about to be written -- because the loader decodes the picture. A
village copy with a tile PNG cut off after its header passed the validator and
was saved, and the game refused it and built Act 1 (B2). A map the game refuses
does not stop the game -- `d2mapgen` quietly builds Diablo II's Act 1 instead --
so a designer who saved a broken file would otherwise get a world his file had
nothing to do with. The save keeps the previous generation as a `.bak`, goes to
the file the editor opened (by its absolute path), and a placed structure is
written with its kind's name -- `peasant-house`, as the village's own are, where
it used to be the loader's `village-placeholder#16`.

`P` asks the same two questions of the same bytes, then writes them to
`playtest-scratch.tmj` **beside** the authoring file (the tileset's image paths
are relative to the map; the name is gitignored, as `*.tmj.bak` is), reads it
back **the way the game will** -- by its path, through the game's loader -- and
refuses unless that is what it just wrote: the loader looks in the game's
folders in its own order, and a playtest of some other copy would be a playtest
of the wrong map. Then it starts a real game on it AT ONCE with a throwaway
hero, **Playtest**, made fresh in a temporary folder of his own with the default
loadout -- no hero screen, the player's own heroes untouched and unlisted,
nothing written into his Saves, and the folder removed once the game has let go
of it (B3). The authoring file is not touched, saved or not.

**When the playtest's game ends, the editor comes back as you left it** (A2): the
document, its unsaved changes, its undo history, the zoom and the view. The App
keeps the editor screen through the playtest (`d2app/playtest.go`), and every
way out of a game ends in `App.ToMainMenu`, which during a playtest hands the
editor back instead: the escape menu's exit, the death screen's menu button, and
a game that could not start (its reason goes in the status line). The death
screen's "load last save" goes on playtesting with the same hero. And the
process's map setting is put back to what it was before `P`, so the next game
started from the menu is the launch map again, not the scratch (B3).

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

Said here rather than left to be assumed (brought up to date 28 Sep):

- **A mouse WHEEL has still never been turned by a script**: the harness has no
  wheel verb. What a script does drive now is the zoom itself -- the `editor`
  harness provider's settable `zoom` field calls `Editor.zoomAbout`, the
  function the wheel handler calls with its new scale -- and `TestWorldEditor`
  zooms through it and measures the screenshot. The handler's own two lines
  (read the notch, multiply or divide by 1.15) are the part no script has run.
- **The right-drag pan has not been driven by a script**: `strigoi_click` can
  hold a button down but cannot move the cursor while it does.
- **`Ctrl+S`, `Ctrl+Z`, `Ctrl+Y` and `P` ARE driven**, on the keyboard, by
  `playtest/editor_test.go` -- against a copy of the village in the game's own
  folder, never the shipped file, which the harness now refuses to open in the
  editor. A palette row and a map tile are picked with the mouse, the tile being
  one the ghost itself said yes to.
- **The WORLD EDITOR menu button was unreadable in the default game** until
  BUG-27 was fixed (28 Sep, `docs/bugs.md`): every main-menu label was drawn in
  the stone's own grey under Strigoi's fonts. It reads now; `-editor` on the
  command line still opens the editor directly.

## 27-28 Sep review and fixes

An independent reviewer drove v0 on 27-28 September and found the data side
sound and the screen not: zoom did not scale the art, `P` threw away unsaved
edits, and the acceptance script could pass on nothing. Josh: "go ahead and make
the fixes, just be sure to document it." Each finding below has its fix, the
test that holds it, and that test's **negative control** -- the fix taken out,
the test watched going red. The reds are logged in
`strigoi-harness-runs\wt-editor-fix\nc\` (unit tests) and
`strigoi-harness-runs\wt-editor-fix\nc-r*.txt` (the acceptance script). The
defects are BUG-28..BUG-34 in `docs/bugs.md`.

| | finding | fix | test (negative control) |
|---|---|---|---|
| A1 | **Zoom moved the art but did not scale it.** `renderFloor`, `renderWall` and `renderShadow` drew every image at full size; only the anchors followed the viewport's scale. The editor opens at ~0.08, so the first screen was full-size trees on anchors a twelfth apart, the ground hung (80(1-s), 40(1-s)) px off its diamonds, and a placed house was hidden behind the forest. | `MapRenderer.drawTileArt` pushes the zoom (plus one pixel of seam overdraw) after the translation for all three, at any scale but 1.0; at 1.0 it is `target.Render(img)` alone. `authoredCullRows` is worked in ortho pixels and no longer divides by the scale (it gave 4 rows at 2.0, where 14 are needed). The viewport comments that said the art is drawn unscaled are corrected. | `TestDrawTileArtScalesTheArt`, `...ScalesATallWall` measure PAINTED pixels on a software surface (red: 160 px of ink where the diamond is 13 -- `nc1-no-pushscale.txt`); `TestAuthoredCullRows*` (red: `nc3-old-cullrows.txt`); `TestWorldEditor` act 2 measures the screenshot -- 0 map pixels outside the diamond at fit, red 61713 with the scale taken out, and act 4's new house changed 0 of 3253 footprint pixels -- the review's hidden house (`nc-r1-noscale-and-realsaves.txt`). Seam pad: 228 empty ground pixels of 84535 at fit, 767 with the pad at 0 (`measure-r4b-seampad0.txt`, red at the 0.5% line). |
| A1 | **Scale 1.0 must stay pixel-identical** (the hard requirement). | The 1.0 branch is kept, call for call. | `TestDrawTileArtAtScale1IsTheUnscaledDraw` (no PushScale, ink exactly the picture; red when the branch is dropped -- `nc2-no-scale1-branch.txt`). And a cross-build screenshot comparison: the same seeded, paused, stepped in-game frame from master `0997f9ae` and from this branch -- **0 of 480000 pixels differ** at two frames; the control, two frames of one build, differ in 437893 (`wt-editor-fix\scale1\compare-1.txt`). `TestTownWalkDeterministic` still green. |
| A2 | **`P` silently threw away unsaved edits.** It wrote the scratch map, left the screen and dropped the document and its undo stack; coming back through the menu reopened the file. | The App keeps the editor through the playtest (`d2app/playtest.go`); `App.ToMainMenu`, where every game ends, hands it back during a playtest with document, dirty mark, undo history and view intact. **The recommended option, not the fallback of asking.** | `TestWorldEditor` acts 6-8: an unsaved tree, `P`, the game ends, and the editor is back with depth 2, dirty, the tree's own undo label; `Ctrl+S` then writes the tree. Red with the hand-back taken out: the game lands on the main menu (`nc-r2-noreturn-and-nonotice.txt`). |
| B1 | **The acceptance script could pass on nothing**: an unedited save through the API, no `P` at all (the launcher's mirror made both hash checks equal by construction), and an unread screenshot. And under the harness a bare `-editor` opened the working tree's village, so a scripted `Ctrl+S` would have overwritten it. | `TestWorldEditor` rewritten to drive the real screen: a copy in the game's own folder by its absolute path, a palette row and a ghost-approved tile clicked, `Ctrl+S`/`Ctrl+Z`/`Ctrl+Y`/`P` on the keyboard, the saved file parsed by the ENGINE, screenshots measured. The editor fixes its file as an absolute path at open (`editorResolve`); the harness refuses the working tree's village (`harnessEditorGuard`). | The script's own acts. `TestEditorSavesToTheFileItOpenedWhateverTheWorkingDirectory` (red: the save lands in the working directory -- `nc10-relative-save.txt`); `TestTheHarnessRefusesToEditTheShippedVillage` (red: `nc11-no-guard.txt`); act 9 clicks the menu's WORLD EDITOR button and is refused. |
| B2 | **Save and `P` trusted the validator alone**, which reads only a PNG's header: a copy with a truncated tile PNG validated clean, was saved, and the game refused it and built Act 1. | `Doc.Checked` / `Doc.Save` take an `Engine` -- `d2maptiled.Parse` with the game's loader over the exact bytes -- and `P` uses the same bytes; a nil engine is a refusal. | `TestSaveRefusesArtTheEngineCannotDecode` (red: written anyway -- `nc4-save-skips-engine.txt`); `TestEditorSaveAndPlaytestAskTheEngine` (red: `nc9-editor-no-engine.txt`); `TestWorldEditor` act 0 on the reviewer's case at the real screen: `Ctrl+S` "NOT SAVED", `P` refused, file unchanged. |
| B3 | **A playtest changed the process for good and spent a real hero.** Every later game was built from the scratch map, and `P` went to character select with the player's own heroes. | The launch map setting is restored when the playtest ends. `P` makes a throwaway hero, **Playtest**, in a temporary folder with the default loadout, and goes straight into the game -- **the recommended option**; the folder is removed once the game has let go of it. | `TestWorldEditor` act 7 (the player is "Playtest"; the test's Saves stay empty -- red when the hero is saved the old way: "a playtest wrote 2 file(s) into the player's Saves", `nc-r1-...`), act 8 (the map setting is the launch one -- red: `nc-r3-nomarkers-and-norestore.txt`) and act 9 (the next normal game is built from the launch map); `TestAFinishedPlaytestsHeroIsRemovedOnceItsGameHasGone` (red: `nc12-cleanup-no-delay.txt`). |
| B4 | **The placement check read the palette tab on SCREEN**, so holding a house and reading the greyed Terrain tab refused every click with Terrain's reason. | `canPlace` and `canPickable` ask the held piece's own tab (`entryTab`). | `TestEditorPlacesTheHeldPieceWhateverTabIsShowing` (red: `nc7-viewed-tab.txt`). |
| B5 | **People and the player start were not drawn**: the smith was found by dropping a house on him. | `drawPeople`: each person's tile outlined, a zoom-sized mark and his name, labels kept apart; the start is an orange cross. Selectable as before (click the tile); not draggable (v1). | `TestWorldEditor` act 2 finds each marker's colour at the position the provider reports (red: all five missing -- `nc-r3-...`). |
| C | A map the engine refuses opened as an empty grid, the reason one line of the status bar -- which went on saying "the game would take this map". | `drawNotice` in the map area; the status line says the engine refuses. | `TestWorldEditor` act 0: 1372 notice pixels (red: 0 -- `nc-r2-...`). |
| C | Placed structures were named `village-placeholder#16`. | `StructureName`: the kind, from the art's `structures/<kind>/` folder (`peasant-house`). | `TestAPlacedStructureIsNamedForItsKind` (red: `nc5-tile-id-name.txt`); act 5 reads the name from the saved file. |
| C | The unsaved mark stayed on after undoing back to the saved map. | `Stack.MarkSaved` / `Stack.Dirty`: the saved point in the history. | `TestTheUnsavedMarkFollowsTheHistory` (red: `nc6-dirty-flag.txt`); act 5's `Ctrl+Z` / `Ctrl+Y`. |
| C | `playtest-scratch.tmj` and `*.tmj.bak` were not ignored. | `.gitignore`. | `git check-ignore -v` on both, and the village itself NOT ignored (`wt-editor-fix\gitignore-check.txt`). |
| C | Duplicating at the east edge said "bare ground". | `footprintIsClear` checks the map's bounds first. | `TestEditorDuplicateAtTheEdgeSaysOffTheMap` (red: `nc8-dup-bare-ground.txt`). |
| C | `ToPlaytest` did not tell the harness its screen. | It notes `playtest` (then `game`), and the way back notes `world_editor`; `strigoi_get_game_info` also reports `playtest`, `playtest_save_dir` and the map setting (`map_asked`, `map_built`, `map_error`). | `TestWorldEditor` acts 7-8 read them. |

**What this did not do.** No wheel event or right-drag pan has been driven by a
script (above). The notice is drawn over a map the engine refused; the editor
still offers no "set the file aside" prompt (v1, `SetAside` is a defer row). A
playtest's hero cannot be chosen: he is always Playtest with the default
loadout. People can be selected and deleted but not moved or added (v1).
