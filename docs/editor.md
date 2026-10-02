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
it nor its art. **The playtest harness refuses to open anything in the source
tree** (second review, 28 Sep -- until then it refused `village.tmj` alone): it
runs the game with the repository as its working directory, so a scripted
`Ctrl+S` would have written the working tree's file. Under `-harness` a map is
opened only from the game's own folders -- the `data/strigoi` mirror beside the
exe, or `%AppData%\OpenDiablo2` -- never by the working-directory fallback, and
never from under a folder whose `go.mod` is this game's module. A script edits a
copy there and passes the copy's absolute path. Without `-harness` nothing
changes: `Play Strigoi.bat` starts the game from its own folder, so the village
is found under that folder and opens and saves as it always has.

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
zoom onto the surface for every floor, wall and shadow at any scale but 1.0.
**At 1.0 -- the only scale the game ever draws at -- it is the old
`target.Render(img)` and nothing else.**

**The ground has no seams at the zooms measured** -- which is what can be said,
and all that is said. The first fix drew each tile one pixel bigger than the
zoom and called the seams closed; the second review (28 Sep) turned the grid
off and counted **449 of 84,552 map pixels empty at the fit zoom (0.53%)** --
the grid is drawn along the tile edges, exactly where the seams are, so with it
on the acceptance script saw half of them. Now each tile's art is sized to the
grid of floored anchors itself (`MapRenderer.tileArtScale`: its far corner lands
where the viewport floors the far corner of its own box, the pixel the next
tile's anchor is floored to, plus one pixel for the diagonal neighbours). On the
running editor, grid off, `TestWorldEditor` counts **0 empty pixels inside the
map's diamond at the fit zoom (0.0771), 0.15, 0.25 and 0.5** (84,535, 235,144,
282,240 and 282,240 pixels looked at); with the first fix put back, 465, 328, 0
and 16. Other zooms are covered by a software model of the same draw
(`TestTheGroundHasNoSeamsAtTheEditorsZooms`, twelve zooms from 0.0769 to 0.75),
not by the GPU -- and the model is not the GPU: it counted 0 for the first fix at
0.15 and 0.5, where the game showed 328 and 16.

**People and the start are marked** (B5). The view is the engine's map with no
entities in it, so each person the map places gets his tile outlined and a
square where he stands, in blue, with his name beside it; the `player_start` is
an orange cross named "start". The marks are sized with the zoom. They are
selectable as they always were -- a click on the tile selects the person on it
-- and cannot be dragged (v1).

**Names** (second review, 28 Sep: at the fit zoom "start" was drawn over the
smith's mark, and at 0.25 the palette cut the headman's name to "headman ...").
Below a zoom of 0.2 (`edLabelZoom`) only the person under the cursor, and the
one selected, is named -- at the fit zoom the village's people stand a few
pixels apart, and the marks say where everyone is. At 0.2 and above everyone is
named. A name is drawn **whole and wholly inside the map's view, or not at
all** -- never cut short, never under the palette -- and clear of every other
person's mark (by 8 px, so it never sits against someone else's and reads as
his) and every other name: beside its mark on the right, or the left, or
centred just above or below it, and failing those up to three lines away with a
leader line back to the mark (`editorPlaceLabel`). A person scrolled out of the
view, or behind the palette, is not named.

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

**The village's objects** (the raid's R3a, 2 Oct 2026). The map reads three
more point objects -- `household` (on its door tile, beside one building),
`hotar` and `watch_post` -- and an npc's optional `household`
(`data/strigoi/maps/README.md` has the rules; `d2maptiled/households.go`).
The validator refuses what the loader refuses for them, rule for rule
(`d2mapedit/village.go`; `TestTheLoaderAndTheValidatorAgree`'s village
cases), so the village with its households saves. The screen does not draw
or place them yet -- that is the People tab, the raid's R3c; until then a
household's door tile selects it like a person's. The records are written by
two edits, `Doc.PlacePoint` and `Doc.SetProperty` (`d2mapedit/people.go`),
which the village's proposal was written with and which the People tab will
run.

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
  So is raised ground (fog of war F4, 2 Oct 2026): `height` (int 0..8) is a
  FLOOR tile's property, and a tower is a 1x1 structure tile carrying
  `sight_radius` (int 1..64). The editor reads both as the game does
  (`parseKindProps`, the same refusals; `TestTheLoaderAndTheValidatorAgree`'s
  seven new cases), round-trips them untouched, and says them when a tile is
  clicked ("height 1", "a tower: sees 16"). It cannot add a tile to a tileset
  (v0), so a new height or tower kind is added in Tiled.
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
- **`Ctrl+S`, `Ctrl+Z`, `Ctrl+Y`, `P` and `G` ARE driven**, on the keyboard,
  by `playtest/editor_test.go` -- against a copy of the village in the game's
  own folder, never a file in the source tree, which the harness refuses to open
  in the editor (`TestWorldEditorRefusesTheWorkingTree` shows it). A palette
  row, the greyed Terrain tab and a map tile are clicked with the mouse, the tile
  being one the ghost itself said yes to.
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

(`TestTheHarnessRefusesToEditTheShippedVillage`, B1's row, is
`TestTheHarnessRefusesToEditTheSourceTree` since the second review; A1's seam
pad and B5's "labels kept apart" were both found short by it -- below.)

**What this did not do.** No wheel event or right-drag pan has been driven by a
script (above). The notice is drawn over a map the engine refused; the editor
still offers no "set the file aside" prompt (v1, `SetAside` is a defer row). A
playtest's hero cannot be chosen: he is always Playtest with the default
loadout. People can be selected and deleted but not moved or added (v1).

## Second review (28 Sep) and fixes

A second independent review drove the editor on `f833e691` and confirmed it is
now built properly: the data side and the first review's fixes held. It found
one B and five C. Josh asked for every fix to be documented; each is below with
its fix, the test that holds it, and that test's **negative control** -- the fix
taken out and the test watched going red. Where an option was open the
reviewer's recommended one was taken. Logs: `strigoi-harness-runs\wt-editor-fix2\`
(the reds in `nc\`, the green acceptance run `pt-green2.txt`, the scale-1.0
comparison `scale1\compare.txt`, and the whole playtest suite `pt-suite.txt`:
69 of 73 green; the four reds -- each a missed click on a HUD, inventory or
crafting button, none in code this branch touched -- are green run one at a time,
`pt-rerun4-branch.txt`; another session was running its own gate on the laptop). New bugs: BUG-35..BUG-38 in `docs/bugs.md`;
BUG-28 and BUG-33 are brought up to date there.

| | finding | fix | test (negative control) |
|---|---|---|---|
| B | **The harness guard protected only `village.tmj`.** Under `-harness`, `-editor=data/strigoi/maps/second.tmj` (a copy in the working tree) opened the working-tree file -- `editorResolve` falls back to the working directory, which the launcher sets to the repository -- and a scripted Delete + `Ctrl+S` overwrote it. (BUG-35) | The recommended option. Under `-harness` the editor refuses any map found by the working-directory fallback (`editorResolve` now says when it used it) and any map under a folder whose `go.mod` is this game's module (`harnessSourceTree`) -- which also covers a harness build run from the repository root, whose own folder IS the tree. What is left is the game's own folders outside the tree: the mirror beside the exe, `%AppData%\OpenDiablo2`. **Josh's launch is untouched:** the game sets no guard, and a game started from its own folder -- `Play Strigoi.bat` -- finds the village under that folder, not by the fallback (`TestEditorResolve`'s new case; and live, `wt-editor-fix2\josh-layout\`: this branch's game copied with `data/strigoi` into a folder of its own, started from that folder with a bare `-editor`, opened that folder's village and `Ctrl+S` saved it). | `TestWorldEditorRefusesTheWorkingTree` (playtest, the reviewer's own steps): a second map in the working tree, opened by the same relative path, goes to the main menu with the guard's reason (read from the `ui` provider's new `main_menu_error`), and the file is unchanged. Red with the guard returning nil: "opened ... the working tree's own file" (`nc\nc2-pt-no-guard.txt`). `TestTheHarnessRefusesToEditTheSourceTree` (unit; red with the first guard put back: `nc\nc1-unit-village-only.txt`). Act 9 still refuses the menu button's village. |
| C | **Seams at the fit zoom.** One pixel of overdraw left rows of one-pixel holes: 449 of 84,552 map pixels empty with the grid off (0.53%), over the script's own 0.5% line; act 2 measured with the grid ON, whose lines sit on the seams, and saw half. (BUG-28, reopened and fixed) | Act 2 turns the grid off with `G` (and checks the provider says so) before it counts. Each tile is sized to its neighbours' floored anchors plus one pixel (`MapRenderer.tileArtScale`) -- measured against 2 px of plain overdraw and a symmetric pad, all three 0 in the model; this one draws the least over. The hole instrument counts the background colour EXACTLY (every background pixel of the reviewer's own grid-off screenshot is exact but one), since within two levels it counted a dark timber at 0.5. `docs/editor.md`'s "no hairline seams" is replaced by what is measured (above). | `TestWorldEditor` acts 2-3, grid off: **0 holes at 0.0771, 0.15, 0.25 and 0.5**; red with the first fix put back: 465, 328, 0 and 16 (`nc\nc6-pt-seam-pad-only.txt`). `TestTheGroundHasNoSeamsAtTheEditorsZooms` (a 40 x 40 field of the game's diamond through `renderFloor`, twelve zooms, five sub-pixel camera offsets: 0 holes; red: 1,222 at 0.077 -- `nc\nc5-seam-pad-only.txt`), with its own control `TestTheSeamInstrumentSeesAMissingTile`. **Scale 1.0 unchanged:** master `ddc3ea7c` and this branch, the same seeded, paused, stepped frames -- **0 of 480,000 pixels differ** at all three (md5s identical to the reviewer's); control 437,893 (`scale1\compare.txt`). `TestDrawTileArtAtScale1IsTheUnscaledDraw` still holds. |
| C | **Throwaway playtest hero folders were left behind** when the game exited during a playtest or within ~1.5 s after one; three sat in `%TEMP%`. (BUG-36) | A playtest hero's folder is named for its process (`strigoi-playtest-<pid>-<random>`). At start-up `App.Run` clears every `strigoi-playtest-*` folder that is **not in use** -- its process no longer running (`processAlive`: OpenProcess and the exit code, since an exited process can still be opened); an older build's folder with no process in its name once it is 10 minutes old -- so four games sharing the laptop never take each other's hero. On the way out -- the window closing, the console's `quit`, the harness's `strigoi_quit` -- a game removes its own. What that misses (a killed process; the main menu's EXIT button, which calls `os.Exit` from `d2gamescreen`) the next start-up clears. | `TestStalePlaytestFoldersAreClearedAndLiveOnesKept` (unit: a running process's folder kept, an exited one's and an old legacy one cleared, a fresh legacy one kept; red when liveness is not asked: `nc\nc3-sweep-ignores-live.txt`); `TestAProcessClearsItsOwnPlaytestFoldersAsItExits`. **Live:** the three leftovers (`...-1390003002`, `...-2358539398`, `...-3739554505`) were cleared by this branch's first game at start-up (`temp-leftovers-before.txt`, `temp-leftovers-after.txt`). |
| C | **B4 was only unit-tested** (a held piece judged by its own tab). | `TestWorldEditor` act 4 clicks the greyed Terrain tab after picking the house and before the map click, and checks the editor shows Terrain and still holds the house. | Red with BUG-33's fix reverted: "the ghost said yes to none of the tiles tried: Terrain: ..." (`nc\nc7-pt-viewed-tab.txt`). |
| C | **Names overlapped at low zoom**: at the fit zoom "start" sat over the smith's mark; at 0.25 the palette cut the headman's name to "headman ...". (BUG-37) | The recommended option, both halves. Below 0.2 only the person under the cursor (and the one selected) is named; a name is placed whole and wholly inside the map's view or not at all, clear of every other mark by 8 px and of every name, right / left / above / below its mark and then a line or more away with a leader (`editorPlaceLabel`). The first attempt at this fix flipped the headman's name left, where it began one pixel after the woman's mark and read as hers; hence the 8 px. | `TestWorldEditor` act 2 (fit: no name with the cursor off the map; the smith's alone with it on his tile) and act 3 (0.25: every person named, whole, inside the view, 2 px clear of every mark, no two names overlapping) read the provider's new record of what was drawn (`people[].mark`, `label`, `label_text`). Red with the old placement: "start's name lies over or against ... the smith's mark", "headman (Warriv)'s name is drawn as "headman ..."" (`nc\nc8-pt-old-names.txt`). `TestEditorPlacesANameWholeInsideTheViewAndClearOfMarks` (unit). **Screenshots, looked at:** before -- the reviewer's `wt-editor-review2\out\20260928-170744-16652\r2-01-fit-00001246.png` and `r2-02-zoom025-00001253.png`; after -- `pt\TestWorldEditor\20260928-175059-34864\editor-fit-00000020.png`, `editor-fit-hover-smith-00000031.png`, `editor-025-00000072.png`, `editor-050-00000056.png`. |
| C | **A playtest could outlive its game** (code reading): if the death screen's "load last save" gave up waiting during a playtest, the game sat on the REAL main menu with the playtest still active and the scratch map still set. (BUG-38) | `advanceReload`'s give-up now ends the playtest (the editor comes back, the map setting is restored, the hero's folder is queued). And `App.advancePlaytestEnd`, every frame, ends any playtest whose game has gone by a way that did not end it: a playtest running, no reload pending, the screen settled, and not a game. | Not reachable by a script (the give-up needs the old game's server to hold its port for 600 frames), so `TestEveryWayOffAPlaytestEndsIt` drives the state transition: the give-up ends it; a pending reload does not; a settled non-game screen does. Red with the give-up's `endPlaytest` removed and the net dark (`nc\nc4-playtest-outlives-reload.txt`), and with the net alone dark (`nc\nc4b-no-net.txt`). |

**What this did not do.** The main menu's EXIT button still calls `os.Exit` from
`d2gamescreen` and does not clear a playtest folder on the way out (the next
start-up does). A playtest folder whose process id has been reused by a running
program is kept until that program exits. The seam count is proven at four
zooms on the GPU and twelve in the software model, not at every zoom. No wheel
or right-drag pan has been driven by a script (above).
