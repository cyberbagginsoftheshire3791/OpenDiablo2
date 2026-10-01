# The game's camera zoom (1 Oct 2026)

**The game zooms out; the art and the world's distances do not change** (Josh,
1 Oct 2026). A Dawn of War 2-scale view: the world drawn smaller so a squad of
five does not fill the 800x600 screen. Nothing in the world is resized or moved
-- a torch still lights 5 tiles, a wolf still notices at 12, a step is still a
step. Only the camera's scale changes.

**The default is 1.0, and 1.0 draws exactly what it drew before the zoom** --
no scale is pushed, no wrapper is made, and every hit test and anchor does the
arithmetic it did (pinned by `TestRenderEntityAtScale1IsTheUnscaledDraw`,
`TestDrawTileArtAtScale1IsTheUnscaledDraw`, `TestSpriteHitRectAt1IsTheHoverLoopsRect`,
`TestHeadAnchorAt1IsTheOldArithmetic`, and `TestScaleOneIsTheUnzoomedTransformExactly`).

## Controls

| Control | Effect |
|---|---|
| `-zoom <float>` | the scale every game starts at, clamped to 0.4..1.0 (default 1.0). A load is a new game and starts at it too. |
| mouse wheel (in a game) | one notch is 0.1, toward the player zooms out, away zooms in, clamped to 0.4..1.0. About the middle of the screen: the camera stays on the hero. A touchpad's fractional scrolls are summed and step once per whole unit (ten scrolls of 0.1 are one notch). Not while the death screen, a talk, the journal, the escape menu, the help overlay, the loadout choice or the skill-select menu is up, nor over the kit or the talent panel -- wherever a click is refused. |
| the zoom a new game starts at | `-zoom`'s value until the wheel or the harness moves the view; then the view the player chose, so a death's load, "load last save" and a new game keep his zoom. Process-wide (`d2player.GameZoom`), never in the world file. |
| harness `ui.zoom` | settable: a number in 0.4..1.0 (refused, not clamped, outside it), set as the wheel sets it -- including the zoom the next game starts at. Classified not-a-dial in `harnessNotDials`: a load does not re-apply it; it carries over because the next game starts at it. |
| harness `ui.view_scale` | read-only: the scale the map is drawn at. In the state digest's process part (a resumed game starts at its own `-zoom`). |

The wheel was bound to nothing in the game before this. `SelectPreviousSkill`
and `SelectNextSkill` name `KeyMouseWheelUp` / `KeyMouseWheelDown` in
`d2player/key_map.go`, but the ebiten adapter has no row for those keys and the
wheel never arrives as a key (`d2core/d2input/ebiten/ebiten_input.go`, `Wheel`):
it arrives as a `MouseWheelEvent`, which only the World Editor handled.

The World Editor's own zoom (0.0625..16, about the cursor) is separate and
unchanged; see docs/editor.md.

## What scales

Everything drawn in world space, at the one scale the viewport holds:

- **Positions** -- the viewport's four transforms (`viewport.go`, since the
  editor-zoom burst). Clicks go through `ScreenToWorld`, which divides by the
  scale, so a click lands on the world point drawn under it.
- **Floors, walls, shadows and the authored strips** -- `MapRenderer.drawTileArt`
  (since the 28 Sep editor review).
- **Every entity** -- `MapRenderer.renderEntity` (`d2maprenderer/entity_scale.go`):
  the hero (composite or PNG sheets), NPCs and villagers, the Strigoi PNG
  creatures, objects, missiles, cast overlays, and the shadows their animations
  draw. At any scale but 1.0 the entity draws through a `viewScaledSurface`,
  which pushes the view's scale after the tile's translation AND scales every
  translation the entity pushes itself (its sub-tile offset, each frame's
  offset from the sprite's origin, the shadow's shift), multiplies any scale it
  pushes (the shadow's 1 x 0.5), and scales the lengths of `DrawRect` /
  `DrawLine` (a skew is a ratio and passes through). Scaling the pictures alone
  would hang each one at its full-size offset from the feet. A shrunk sprite is
  sampled with `FilterLinear` (ebiten mipmaps a linear minification), so a
  figure's outline does not break up at 0.4; an entity that pushes its own
  filter keeps it. At 1.0 nothing is pushed.
- **Everything anchored to an entity** (`d2player/view_scale.go`): the hover
  and squad-selection hit boxes (`spriteHitRect`), the tactical hit test, the
  point the hover label and the overhead bar hang from (`headAnchor`), and the
  entity debug box. The overhead bar, its stage cues, the hover label, the
  corpse marks and the hover pad keep their UI pixel size; they sit at the
  scaled position.
- **The tactical diamonds** -- they were already four world corners through
  `WorldToScreenF`.

## Light and darkness are world-radius already

The night's darkness is not a screen-space mask. It is a brightness pushed per
world tile: `MapRenderer.pushTileLight` asks the light model (`d2world.Light.Level`)
how lit each tile is, and the floors, walls and entities on that tile draw at
that brightness. A torch's radius (`LightDials.TorchRadius`, 5) and every other
source's is in world tiles. So at scale s a torch of radius R tiles lights R
tiles of ground, which is R*s of the screen's pixels: zooming out shows more
ground, and the extra ground is as dark as the light model says it is.

## Culling

The renderer's cull probes are screen pixels through `ScreenToWorld`, so the
tile range widens by 1/scale as the view zooms out
(`TestCullRangeCoversEveryTileOnScreenAtEveryScale`, scales 0.0625..16, which
bracket 0.4..1.0). Entities are culled with their tiles (the render passes draw
the entities standing on the tiles in range), and the authored strips' extra
rows are `authoredCullRows`, which is 0 at every scale below about 0.59
(`TestAuthoredCullRowsFollowTheScale`).

## What a zoomed-out view changes for design (not code)

Several world dials were written with "about five tiles in any direction are on
screen" in mind (`TacticalEngageTiles` 5, `NoticeDials.Radius` 12 "deliberately
larger than the viewport"). At 0.5 about ten tiles are on screen in each
direction. The distances are unchanged by ruling; whether a fight should still
open at 5 tiles when the player can see 10 is a design question, not a zoom bug.

## Known gaps

- **Enemies show in the dark when zoomed out (BUG-107, OPEN, owner: fog of war
  F2).** The overhead bars, hover label, corpse marks and the tactical and
  click hit tests ask no light. At 1.0 the screen is about the torch's 5 tiles;
  at 0.4 it reaches about 9 x 13 tiles from the hero, inside the 12-tile notice
  band, so a bar floats over a wolf the dark hides. Deliberately not fixed here:
  gating them on light changes frames at 1.0, and F2's one visibility predicate
  owns it.
- **Moving sprites can drift by about a pixel at any scale but 1.0** (the
  review's C3). Every translation is a whole pixel: the tile anchor is floored
  by the viewport and the entity's offsets are rounded once by the wrapper, so
  a walking man's feet sit within about one pixel of `WorldToScreenF` of his
  position and can step by a pixel between frames where the true point moved
  less. Not visible in the software tests' 2 px tolerance; a real-render look.
- **Tiles are still drawn nearest** (`drawTileArt`). Their seam measurements
  are against a nearest draw; whether linear sampling would read better on the
  ground, and whether it opens seams, is a real-render check not yet made.

## Verifying on screen

Software-surface tests measure the painted pixels (`entity_scale_test.go`,
`draw_scale_test.go`); a real look is still owed: launch with `-zoom 0.5` and
check the hero, villagers and a creature stand on their feet at half size, the
bars sit just above their heads, hovering and clicking a monster and a squad
model hit the half-size sprite, and at night the torch's lit ground is the same
5 tiles (half the screen pixels it covers at 1.0).
