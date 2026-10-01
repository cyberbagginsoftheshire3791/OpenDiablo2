package d2maprenderer

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2geom"
)

type worldTrans struct {
	x float64
	y float64
}

const (
	center     = 0
	left       = 1
	right      = 2
	tileWidth  = 80
	tileHeight = 40
	half       = 2
)

const (
	worldToOrthoOffsetX = 3
)

const (
	// defaultScale is the viewport's zoom when nobody has set one: one screen
	// pixel per orthogonal pixel, which is the only scale the shipped game
	// ever draws at. A zero-value Viewport reads as this too (scaleOrDefault), so
	// adding the field changed nothing that existed before it.
	defaultScale = 1.0
	// minScale and maxScale bound SetScale. A scale of 0 would divide by zero
	// in ScreenToOrtho and hand d2vector.NewPosition an Inf, which panics; a
	// negative one would mirror the world. Both ends are [DIAL].
	minScale = 0.0625
	maxScale = 16.0

	// cullMarginTop and cullMarginBottom are how far outside the screen, in
	// SCREEN pixels, the renderer's two cull probes sit. They were the bare
	// -200 and 1050 (= 600 + 450) in MapRenderer.Render.
	//
	// Screen pixels is the right unit at every zoom: the probes go through
	// ScreenToWorld, which divides by the scale, so the world range they
	// describe widens by 1/scale as the view zooms out. The art is scaled with
	// the view (MapRenderer.drawTileArt, since the 28 Sep review), so what it
	// reaches past its anchor in SCREEN pixels shrinks and grows with the zoom
	// exactly as the tiles do; the tall authored strips that reach further than
	// this margin are authoredCullRows' job, at every scale. [DIAL]
	cullMarginTop    = 200
	cullMarginBottom = 450

	// maxAuthoredArtHeight is d2maptiled's tallest structure strip, 768 px
	// (maxStructureHeight, d2core/d2map/d2maptiled/tiled.go:141).
	maxAuthoredArtHeight = 768

	// authoredCullRowsAtScale1 is how many extra tile rows the wall pass has to
	// draw on an authored map at scale 1.0, so a house whose front tiles are
	// just below the screen still shows its roof: the tallest strip, less what
	// cullMarginBottom already covers, over the height of a tile row.
	// (768-450)/40 = 7.95, and the ceiling of that is the 8 this replaces.
	authoredCullRowsAtScale1 = (maxAuthoredArtHeight - cullMarginBottom) / float64(tileHeight)
)

// Viewport is used for converting vectors between screen (pixel), orthogonal (Camera) and world (isometric) space.
type Viewport struct {
	defaultScreenRect d2geom.Rectangle
	screenRect        d2geom.Rectangle
	transStack        []worldTrans
	transCurrent      worldTrans
	camera            *Camera
	align             int
	// scale is the zoom: screen pixels per orthogonal pixel. It sits between
	// orthogonal and screen space, NOT between world and orthogonal space,
	// because the Camera's position is stored in orthogonal pixels
	// (MapRenderer.CreateMapRenderer feeds it WorldToOrtho, renderer.go:98,
	// and d2gamescreen/game.go:849 re-aims it the same way every frame).
	// Scaling world->ortho would move the camera's target every time the zoom
	// changed and make Camera.advanceToTarget smooth across two different
	// spaces. 0 means "never set" and reads as defaultScale.
	scale float64
}

// NewViewport creates a new Viewport with the given parameters and returns a pointer to it.
func NewViewport(x, y, width, height int) *Viewport {
	return &Viewport{
		screenRect: d2geom.Rectangle{
			Left:   x,
			Top:    y,
			Width:  width,
			Height: height,
		},
		defaultScreenRect: d2geom.Rectangle{
			Left:   x,
			Top:    y,
			Width:  width,
			Height: height,
		},
		scale: defaultScale,
	}
}

// Scale returns the viewport's zoom: screen pixels per orthogonal pixel. 1.0 is
// unzoomed, and is what the shipped game runs at by default (-zoom and the
// game's mouse wheel move it, between 0.4 and 1.0; docs/camera.md).
func (v *Viewport) Scale() float64 {
	return v.scaleOrDefault()
}

// SetScale sets the viewport's zoom, clamped to [minScale, maxScale]. All four
// space transforms follow it and stay inverses of each other, and the map's own
// art follows it too: MapRenderer.drawTileArt pushes the same scale onto the
// surface for every floor, wall and shadow it draws at any scale but 1.0 (the
// 28 Sep review's A1 -- until then only the POSITIONS followed the zoom, and the
// art was drawn at full size on scaled anchors). Surface.PushScale assigns
// rather than multiplies and scales the art about its own top-left, not where
// it goes, so it is pushed AFTER the translation. Entities follow it too, since
// the game zoom (1 Oct 2026): MapRenderer.renderEntity draws every entity
// through a viewScaledSurface at any scale but 1.0, which scales the sprite AND
// every offset the entity pushes from its feet (entity_scale.go). The game
// starts at 1.0 unless -zoom says otherwise, and 1.0 draws exactly as before.
func (v *Viewport) SetScale(scale float64) {
	v.scale = clampScale(scale)
}

// scaleOrDefault is the zoom to use, treating the zero value as unzoomed so
// that a Viewport built without NewViewport behaves exactly as it did before
// there was a scale at all.
func (v *Viewport) scaleOrDefault() float64 {
	if v.scale <= 0 {
		return defaultScale
	}

	return v.scale
}

func clampScale(scale float64) float64 {
	if scale < minScale {
		return minScale
	}

	if scale > maxScale {
		return maxScale
	}

	return scale
}

// halfScreen is the half width and height getCameraOffset takes off the camera.
// The integer division is deliberate and must stay: it is what the unzoomed
// transform has always done, and an odd viewport width truncates.
func (v *Viewport) halfScreen() (halfWidth, halfHeight float64) {
	return float64(v.screenRect.Width / half), float64(v.screenRect.Height / half)
}

// SetCamera sets the current Camera to the given value.
func (v *Viewport) SetCamera(camera *Camera) {
	v.camera = camera
}

// WorldToScreen returns the screen space for the given world coordinates as two integers.
func (v *Viewport) WorldToScreen(x, y float64) (screenX, screenY int) {
	return v.OrthoToScreen(v.WorldToOrtho(x, y))
}

// WorldToScreenF returns the screen space for the given world coordinates as two float64s.
func (v *Viewport) WorldToScreenF(x, y float64) (screenX, screenY float64) {
	return v.OrthoToScreenF(v.WorldToOrtho(x, y))
}

// ScreenToWorld returns the world position for the given screen coordinates.
func (v *Viewport) ScreenToWorld(x, y int) (worldX, worldY float64) {
	return v.OrthoToWorld(v.ScreenToOrtho(x, y))
}

// OrthoToWorld returns the world position for the given orthogonal coordinates.
// It inverts WorldToOrtho to within float64 rounding (measured at 1e-9 by
// TestOrthoToWorldInvertsWorldToOrtho; dividing by 80 and 40 is not exact).
//
// The two divisors were the literals 80 and 40 until the scale went in. They
// happened to equal tileWidth and tileHeight, so this is the same arithmetic --
// but a literal that only happens to match the constant its inverse uses is a
// trap: change tileWidth and WorldToOrtho follows while this does not, and the
// round trip every click depends on quietly stops closing.
func (v *Viewport) OrthoToWorld(x, y float64) (worldX, worldY float64) {
	worldX = (x/tileWidth + y/tileHeight) / half
	worldY = (y/tileHeight - x/tileWidth) / half

	return worldX, worldY
}

// WorldToOrtho returns the orthogonal position for the given world coordinates.
//
// This is the isometric transform and it is scale-free on purpose: orthogonal
// space is where the Camera lives, so the zoom sits on the far side of it, in
// ScreenToOrtho and OrthoToScreen. See the Viewport.scale field.
func (v *Viewport) WorldToOrtho(x, y float64) (orthoX, orthoY float64) {
	orthoX = (x - y) * tileWidth
	orthoY = (x + y) * tileHeight

	return orthoX, orthoY
}

// ScreenToOrtho returns the orthogonal position for the given screen coordinates.
// It inverts OrthoToScreenF at every scale, to within float64 rounding -- measured
// at 1e-9 by TestScreenToOrthoInvertsOrthoToScreenFAtEveryScale.
//
// The zoom is anchored on the camera, which stays at the middle of the viewport
// at every scale: getCameraOffset has already taken the half screen off the
// camera's orthogonal position, so adding it back gives the camera itself.
func (v *Viewport) ScreenToOrtho(x, y int) (orthoX, orthoY float64) {
	camX, camY := v.getCameraOffset()

	if scale := v.scaleOrDefault(); scale != defaultScale {
		halfWidth, halfHeight := v.halfScreen()
		orthoX = (float64(x)-float64(v.screenRect.Left)-halfWidth)/scale + camX + halfWidth
		orthoY = (float64(y)-float64(v.screenRect.Top)-halfHeight)/scale + camY + halfHeight

		return orthoX, orthoY
	}

	// The unzoomed path, character for character as it was. Routing scale 1.0
	// through the branch above would be the same arithmetic reassociated, and
	// float64 reassociation moves the last bit: OrthoToScreen floors its result,
	// so one bit low is one pixel out, and the shipped game goes down this
	// branch on every frame and every click.
	orthoX = float64(x) + camX - float64(v.screenRect.Left)
	orthoY = float64(y) + camY - float64(v.screenRect.Top)

	return orthoX, orthoY
}

// OrthoToScreen returns the screen position for the given orthogonal coordinates as two ints.
func (v *Viewport) OrthoToScreen(x, y float64) (screenX, screenY int) {
	fx, fy := v.OrthoToScreenF(x, y)

	return int(math.Floor(fx)), int(math.Floor(fy))
}

// OrthoToScreenF returns the screen position for the given orthogonal coordinates as two float64s.
// It inverts ScreenToOrtho; see there.
func (v *Viewport) OrthoToScreenF(x, y float64) (screenX, screenY float64) {
	camOrthoX, camOrthoY := v.getCameraOffset()

	if scale := v.scaleOrDefault(); scale != defaultScale {
		halfWidth, halfHeight := v.halfScreen()
		screenX = (x-camOrthoX-halfWidth)*scale + float64(v.screenRect.Left) + halfWidth
		screenY = (y-camOrthoY-halfHeight)*scale + float64(v.screenRect.Top) + halfHeight

		return screenX, screenY
	}

	// The unzoomed path, kept exact. See ScreenToOrtho.
	screenX = x - camOrthoX + float64(v.screenRect.Left)
	screenY = y - camOrthoY + float64(v.screenRect.Top)

	return screenX, screenY
}

// ZoomAtScreen returns the orthogonal position the camera must move to for the
// world point currently under the given screen pixel to stay under that same
// pixel once the scale becomes newScale. newScale is clamped exactly as SetScale
// clamps it, so the answer matches the scale the viewport will take.
//
// It changes nothing: the caller sets the scale and moves the camera, which is
// what MapRenderer.ZoomAt does.
func (v *Viewport) ZoomAtScreen(screenX, screenY int, newScale float64) (camOrthoX, camOrthoY float64) {
	from, to := v.scaleOrDefault(), clampScale(newScale)

	camX, camY := v.getCameraOffset()
	halfWidth, halfHeight := v.halfScreen()

	// getCameraOffset has taken the half screen off the camera; put it back.
	camOrthoX, camOrthoY = camX+halfWidth, camY+halfHeight

	// ScreenToOrtho reads ortho = (pixel - left - half)/scale + camera. Holding
	// ortho fixed across a change of scale and solving for the new camera gives
	// the offset below.
	offsetX := float64(screenX) - float64(v.screenRect.Left) - halfWidth
	offsetY := float64(screenY) - float64(v.screenRect.Top) - halfHeight

	camOrthoX += offsetX * (1/from - 1/to)
	camOrthoY += offsetY * (1/from - 1/to)

	return camOrthoX, camOrthoY
}

// cullProbes returns the two screen-space points MapRenderer.Render walks to
// find the range of tiles it must draw: one above the top of the screen and one
// below the bottom, both on the screen's vertical centre line.
//
// It reads defaultScreenRect, not screenRect, so the aligned half-viewports
// (toLeft/toRight) do not narrow it -- they still draw over the whole screen,
// which is what IsOrthoRectVisible tests against too. At the 800x600 the game
// builds, this is (400, -200, 1050): the three literals it replaces.
func (v *Viewport) cullProbes() (x, topY, bottomY int) {
	x = v.defaultScreenRect.Left + v.defaultScreenRect.Width/half
	topY = v.defaultScreenRect.Top - cullMarginTop
	bottomY = v.defaultScreenRect.Top + v.defaultScreenRect.Height + cullMarginBottom

	return x, topY, bottomY
}

// cullRange returns the half-open range of tiles MapRenderer.Render draws,
// clamped to a map of the given size. The scale reaches it through
// ScreenToWorld: the probes are fixed screen pixels, and a zoomed-out view turns
// the same pixels into a wider stretch of world.
func (v *Viewport) cullRange(mapWidth, mapHeight int) (startX, startY, endX, endY int) {
	probeX, topY, bottomY := v.cullProbes()

	stxf, styf := v.ScreenToWorld(probeX, topY)
	etxf, etyf := v.ScreenToWorld(probeX, bottomY)

	startX = int(math.Max(0, math.Floor(stxf)))
	startY = int(math.Max(0, math.Floor(styf)))

	endX = int(math.Min(float64(mapWidth), math.Ceil(etxf)))
	endY = int(math.Min(float64(mapHeight), math.Ceil(etyf)))

	return startX, startY, endX, endY
}

// authoredCullRows is how many extra tile rows the wall pass draws on an
// authored map, so a structure whose tiles are just below the screen still shows
// its roof.
//
// THE ART IS SCALED WITH THE VIEW (28 Sep review, A1), so the arithmetic is done
// in ORTHOGONAL pixels, where nothing depends on the zoom but the probe: a strip
// stands maxAuthoredArtHeight ortho pixels above its anchor at every scale, a
// tile row is tileHeight ortho pixels at every scale, and the bottom probe is
// cullMarginBottom SCREEN pixels below the screen, which is cullMarginBottom/scale
// ortho pixels. So the rows it must reach past the probe are
//
//	(maxAuthoredArtHeight - cullMarginBottom/scale) / tileHeight
//
// and none when that is negative. At 1.0 that is ceil(7.95) = 8, the bare
// constant this replaced, and 1.0 returns it without the division. Zoomed out
// the probe already reaches past the tallest strip, so it is 0; zoomed IN it
// grows -- 14 at 2.0 -- which the version that divided by the scale got
// backwards (4 at 2.0: the roofs of houses just below the screen went missing,
// and 99 rows at the editor's fit zoom, drawn for nothing).
func (v *Viewport) authoredCullRows() int {
	scale := v.scaleOrDefault()
	if scale == defaultScale {
		return int(math.Ceil(authoredCullRowsAtScale1))
	}

	rows := (maxAuthoredArtHeight - cullMarginBottom/scale) / float64(tileHeight)
	if rows <= 0 {
		return 0
	}

	return int(math.Ceil(rows))
}

// IsTileVisible returns false if no part of the tile is within the game screen.
func (v *Viewport) IsTileVisible(x, y float64) bool {
	orthoX1, orthoY1 := v.WorldToOrtho(x-worldToOrthoOffsetX, y)
	orthoX2, orthoY2 := v.WorldToOrtho(x+worldToOrthoOffsetX, y)

	return v.IsOrthoRectVisible(orthoX1, orthoY1, orthoX2, orthoY2)
}

// IsTileRectVisible returns false if none of the tiles rects are within the game screen.
func (v *Viewport) IsTileRectVisible(rect d2geom.Rectangle) bool {
	left := float64((rect.Left - rect.Bottom()) * tileWidth)
	top := float64((rect.Left + rect.Top) * tileHeight)
	right := float64((rect.Right() - rect.Top) * tileWidth)
	bottom := float64((rect.Right() + rect.Bottom()) * tileHeight)

	return v.IsOrthoRectVisible(left, top, right, bottom)
}

// IsOrthoRectVisible returns false if the given orthogonal position is outside the game screen.
func (v *Viewport) IsOrthoRectVisible(x1, y1, x2, y2 float64) bool {
	screenX1, screenY1 := v.OrthoToScreen(x1, y1)
	screenX2, screenY2 := v.OrthoToScreen(x2, y2)

	return !(screenX1 >= v.defaultScreenRect.Width || screenX2 < 0 || screenY1 >= v.defaultScreenRect.Height || screenY2 < 0)
}

// GetTranslationOrtho returns the viewport's current orthogonal space translation.
func (v *Viewport) GetTranslationOrtho() (orthoX, orthoY float64) {
	return v.transCurrent.x, v.transCurrent.y
}

// GetTranslationScreen returns the viewport's current screen space translation.
func (v *Viewport) GetTranslationScreen() (screenX, screenY int) {
	return v.OrthoToScreen(v.transCurrent.x, v.transCurrent.y)
}

// PushTranslationOrtho adds a new orthogonal translation to the stack.
func (v *Viewport) PushTranslationOrtho(x, y float64) *Viewport {
	v.transStack = append(v.transStack, v.transCurrent)
	v.transCurrent.x += x
	v.transCurrent.y += y

	return v
}

// PushTranslationWorld adds a new world translation to the stack, converting it to orthogonal space.
func (v *Viewport) PushTranslationWorld(x, y float64) {
	v.PushTranslationOrtho(v.WorldToOrtho(x, y))
}

// PushTranslationScreen adds a new screen translation to the stack, converting it to orthogonal space.
func (v *Viewport) PushTranslationScreen(x, y int) {
	v.PushTranslationOrtho(v.ScreenToOrtho(x, y))
}

// PopTranslation pops a translation from the stack.
func (v *Viewport) PopTranslation() {
	count := len(v.transStack)
	if count == 0 {
		panic("empty stack")
	}

	v.transCurrent = v.transStack[count-1]
	v.transStack = v.transStack[:count-1]
}

func (v *Viewport) getCameraOffset() (camX, camY float64) {
	if v.camera != nil {
		camPosition := v.camera.GetPosition()
		camX, camY = camPosition.X(), camPosition.Y()
	}

	camX -= float64(v.screenRect.Width / half)
	camY -= float64(v.screenRect.Height / half)

	return camX, camY
}

func (v *Viewport) toLeft() {
	if v.align == left {
		return
	}

	v.screenRect.Width = v.defaultScreenRect.Width / half
	v.screenRect.Left = v.defaultScreenRect.Left + v.defaultScreenRect.Width/half
	v.align = left
}

func (v *Viewport) toRight() {
	if v.align == right {
		return
	}

	v.screenRect.Width = v.defaultScreenRect.Width / half
	v.align = right
}

func (v *Viewport) resetAlign() {
	if v.align == center {
		return
	}

	v.screenRect.Width = v.defaultScreenRect.Width
	v.screenRect.Left = v.defaultScreenRect.Left
	v.align = center
}
