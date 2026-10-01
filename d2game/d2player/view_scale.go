package d2player

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
)

// THE GAME ZOOMS OUT (Josh, 1 Oct 2026). The map renderer draws every entity at
// the view's scale (d2maprenderer.renderEntity), so everything that measures an
// entity in screen pixels -- the hover and selection hit boxes, the tactical
// hit test, the point the hover label and the overhead bar hang from -- has to
// measure the sprite at that scale too, or the box is the full-size sprite's
// around a half-size man. The bar, the label and the pad keep their UI pixel
// size; only what comes from the entity is scaled.
//
// At exactly 1.0 d2maprenderer.ScaleLength returns its argument untouched, so
// every function here is the arithmetic its caller had before the zoom.

// Game zoom range and wheel step. 1.0 is the shipped view and stays the
// default; 0.4 is the furthest out the wheel, -zoom and the harness go. [DIAL]
const (
	gameZoomMin  = 0.4
	gameZoomMax  = 1.0
	gameZoomStep = 0.1

	// gameZoomStepsPerUnit is 1/gameZoomStep, the grid nextGameZoom rounds
	// to. Rounding as round(z*10)/10 rather than round(z/0.1)*0.1 gives the
	// double nearest each decimal step -- 7*0.1 is 0.7000000000000001, 7/10
	// is 0.7 -- so a step lands on the same number a script or -zoom names.
	gameZoomStepsPerUnit = 10
)

// ClampGameZoom is z held to the game's zoom range. NaN, which no comparison
// catches, is the unzoomed view.
func ClampGameZoom(z float64) float64 {
	if math.IsNaN(z) {
		return gameZoomMax
	}

	return math.Max(gameZoomMin, math.Min(gameZoomMax, z))
}

// nextGameZoom is the zoom one wheel notch moves to from cur: a step out when
// the wheel rolls toward the player (dy < 0), a step in when it rolls away,
// clamped, and rounded to the step so every notch lands on the decimal it
// names -- 0.8, not 0.7999999999999999 -- and the way back ends on 1.0
// exactly. dy == 0 is no change.
func nextGameZoom(cur, dy float64) float64 {
	switch {
	case dy > 0:
		cur += gameZoomStep
	case dy < 0:
		cur -= gameZoomStep
	default:
		return cur
	}

	return ClampGameZoom(math.Round(cur*gameZoomStepsPerUnit) / gameZoomStepsPerUnit)
}

// spriteHitRect is the screen rectangle the hover and squad-selection hit tests
// take for an entity whose feet are at (ex, ey) and whose unscaled sprite is
// w x h: the sprite at the view's scale, centred on the feet, padded by the
// hover pad. Its shape (centred on the feet, the pad taken off the bottom as
// well as the top) is the hover loop's, unchanged.
func spriteHitRect(ex, ey, w, h int, scale float64) (l, r, t, b int) {
	w, h = d2maprenderer.ScaleLength(w, scale), d2maprenderer.ScaleLength(h, scale)
	halfW, halfH := w>>1, h>>1

	return ex - halfW - hoverLabelOuterPad, ex + halfW + hoverLabelOuterPad,
		ey - halfH - hoverLabelOuterPad, ey + halfH - hoverLabelOuterPad
}

// headAnchor is the point the hover label and the overhead bar hang from for an
// entity whose feet are at (ex, ey): back by its render offset and up by its
// sprite's height, both at the view's scale, then up by the hover pad, which is
// UI pixels and is not scaled.
func headAnchor(ex, ey, xOff, yOff, h int, scale float64) (x, y int) {
	return ex - d2maprenderer.ScaleLength(xOff, scale),
		ey - d2maprenderer.ScaleLength(yOff, scale) - d2maprenderer.ScaleLength(h, scale) - hoverLabelOuterPad
}

// viewScale is the scale the map is drawn at, 1.0 before the renderer exists.
func (g *GameControls) viewScale() float64 {
	if g.mapRenderer == nil {
		return gameZoomMax
	}

	return g.mapRenderer.Scale()
}

// viewZoom is what the game zoom needs of the map renderer: its scale, read and
// set. *d2maprenderer.MapRenderer is one; a test's fake is another.
type viewZoom interface {
	Scale() float64
	SetScale(scale float64)
}

// zoomByNotch moves the view one wheel notch (nextGameZoom) and says whether the
// wheel did anything. MapRenderer.SetScale moves no camera: the world point at
// the middle of the screen stays there, and the camera follows the hero, so he
// stays where he was and the world closes in or opens out around him.
func zoomByNotch(v viewZoom, dy float64) bool {
	if dy == 0 {
		return false
	}

	v.SetScale(nextGameZoom(v.Scale(), dy))

	return true
}

// setZoomField is the harness's "zoom" write: a number in the game's range, set
// as the wheel sets it. A number outside the range is refused, not clamped, so
// a script learns its number was not the one drawn.
func setZoomField(v viewZoom, value interface{}) error {
	var z float64

	switch n := value.(type) {
	case float64:
		z = n
	case int:
		z = float64(n)
	default:
		return fmt.Errorf("zoom wants a number, got %T", value)
	}

	if math.IsNaN(z) || z < gameZoomMin || z > gameZoomMax {
		return fmt.Errorf("zoom %v is outside the game's range %v..%v", z, gameZoomMin, gameZoomMax)
	}

	if v == nil {
		return fmt.Errorf("the game has no map renderer yet")
	}

	v.SetScale(z)

	return nil
}

// OnMouseWheel zooms the game's view a step per notch about the middle of the
// screen (the camera stays on the hero). The wheel was bound to nothing in the
// game before this: SelectPreviousSkill and SelectNextSkill name
// KeyMouseWheelUp and KeyMouseWheelDown in key_map.go, but the ebiten adapter
// has no row for those keys and the wheel never arrives as a key
// (d2core/d2input/ebiten/ebiten_input.go, Wheel). It zooms nothing while a
// modal screen is up: the death screen, a talk, the journal, the escape menu or
// the help overlay.
func (g *GameControls) OnMouseWheel(event d2interface.MouseWheelEvent) bool {
	if g.mapRenderer == nil || g.dead() || g.talking() || g.journalOpen() {
		return false
	}

	if g.escapeMenu != nil && g.escapeMenu.IsOpen() {
		return false
	}

	if g.HelpOverlay != nil && g.HelpOverlay.IsOpen() {
		return false
	}

	return zoomByNotch(g.mapRenderer, event.ScrollY())
}

// HarnessSettableFields lists the one field a script may write on "ui": the
// game zoom. The harness has no wheel verb, so a scripted zoom sets the scale
// the way the wheel does (zoomByNotch and setZoomField both end in SetScale).
func (g *GameControls) HarnessSettableFields() []string { return []string{"zoom"} }

// HarnessSet writes the zoom (setZoomField).
func (g *GameControls) HarnessSet(field string, value interface{}) error {
	if field != "zoom" {
		return fmt.Errorf("only zoom is settable on ui")
	}

	if g.mapRenderer == nil {
		return setZoomField(nil, value)
	}

	return setZoomField(g.mapRenderer, value)
}
