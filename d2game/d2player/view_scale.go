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

// processGameZoom is the zoom a new game starts at: -zoom's value
// (d2gamescreen.SetGameZoom) until the player or a script moves the view, and
// then the view he chose -- a death's load, "load last save" and a new game
// keep his zoom rather than snapping back to the flag (the zoom review, C2).
// It is this process's, never the world's: no save carries it.
//
// nolint:gochecknoglobals // one value per process, set from the command line and the wheel
var processGameZoom = gameZoomMax

// SetGameZoom sets the zoom new games start at, clamped to the game's range.
func SetGameZoom(z float64) { processGameZoom = ClampGameZoom(z) }

// GameZoom is the zoom new games start at.
func GameZoom() float64 { return processGameZoom }

// viewZoom is what the game zoom needs of the map renderer: its scale, read and
// set. *d2maprenderer.MapRenderer is one; a test's fake is another.
type viewZoom interface {
	Scale() float64
	SetScale(scale float64)
}

// setViewZoom is the one line every zoom of a game goes through -- the wheel and
// the harness's field: the view's scale, and the zoom the next game starts at.
// MapRenderer.SetScale moves no camera: the world point at the middle of the
// screen stays there, and the camera follows the hero, so he stays where he was
// and the world closes in or opens out around him.
func setViewZoom(v viewZoom, z float64) {
	z = ClampGameZoom(z)
	v.SetScale(z)
	SetGameZoom(z)
}

// wheelNotchEpsilon absorbs the float error of summed fractional scrolls: ten
// touchpad events of 0.1 sum to 0.9999999999999999, which is one notch.
const wheelNotchEpsilon = 1e-9

// wheelNotches adds one wheel event's dy to the running sum and takes the whole
// notches out of it (the zoom review, B1). A mouse wheel reports 1 a notch; a
// touchpad reports fractions many times a second, and a zoom of a whole step
// per event made a two-finger scroll fling the view from 1.0 to 0.4 at once.
// Whole units of the sum are notches, the remainder waits for the next event,
// and a turn of direction eats into it first.
func wheelNotches(acc *float64, dy float64) int {
	*acc += dy

	var n int
	if *acc >= 0 {
		n = int(*acc + wheelNotchEpsilon)
	} else {
		n = int(*acc - wheelNotchEpsilon)
	}

	*acc -= float64(n)
	if math.Abs(*acc) < wheelNotchEpsilon {
		*acc = 0
	}

	return n
}

// zoomByWheel moves the view by the whole notches the event completes and says
// whether it zoomed (a fraction of a notch does nothing yet).
func zoomByWheel(v viewZoom, acc *float64, dy float64) bool {
	n := wheelNotches(acc, dy)
	if n == 0 {
		return false
	}

	z := v.Scale()

	for ; n > 0; n-- {
		z = nextGameZoom(z, 1)
	}

	for ; n < 0; n++ {
		z = nextGameZoom(z, -1)
	}

	setViewZoom(v, z)

	return true
}

// setZoomField is the harness's "zoom" write: a number in the game's range, set
// as the wheel sets it (setViewZoom). A number outside the range is refused,
// not clamped, so a script learns its number was not the one drawn.
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

	setViewZoom(v, z)

	return nil
}

// wheelBlocked says whether the wheel must not zoom: a modal screen is up (the
// death screen, a talk, the journal, the escape menu, the help overlay, the
// loadout choice, the skill-select menu), or the cursor is over the kit or the
// talent panel -- the click path's own guards (OnMouseButtonDown,
// OnMouseButtonRepeat), so the wheel is refused wherever a click is (the zoom
// review, C1).
func (g *GameControls) wheelBlocked(mx, my int) bool {
	switch {
	case g.dead(), g.talking(), g.journalOpen():
		return true
	case g.escapeMenu != nil && g.escapeMenu.IsOpen():
		return true
	case g.HelpOverlay != nil && g.HelpOverlay.IsOpen():
		return true
	case g.kitHolder != nil && (g.kitHolder.ChoosingLoadout() || g.overKitPanel(mx, my)):
		return true
	case g.overTalentPanel(mx, my):
		return true
	case g.hud != nil && g.hud.skillSelectMenu != nil && g.hud.skillSelectMenu.IsOpen():
		return true
	}

	return false
}

// OnMouseWheel zooms the game's view about the middle of the screen (the camera
// stays on the hero), a 0.1 step per whole notch. The wheel was bound to
// nothing in the game before this: SelectPreviousSkill and SelectNextSkill
// name KeyMouseWheelUp and KeyMouseWheelDown in key_map.go, but the ebiten
// adapter has no row for those keys and the wheel never arrives as a key
// (d2core/d2input/ebiten/ebiten_input.go, Wheel).
func (g *GameControls) OnMouseWheel(event d2interface.MouseWheelEvent) bool {
	if g.mapRenderer == nil || g.wheelBlocked(event.X(), event.Y()) {
		return false
	}

	return zoomByWheel(g.mapRenderer, &g.wheelAcc, event.ScrollY())
}

// HarnessSettableFields lists the one field a script may write on "ui": the
// game zoom. The harness has no wheel verb, so a scripted zoom goes through
// setViewZoom, the line the wheel goes through.
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

// spriteUnder is the hover loop's and squad selection's hit test for one
// entity: is the screen point inside its sprite as drawn, at the view's scale
// (spriteHitRect)?
func spriteUnder(mr *d2maprenderer.MapRenderer, ent d2interface.MapEntity, mx, my int) bool {
	sxf, syf := mr.WorldToScreenF(ent.GetPositionF())
	ex, ey := int(math.Floor(sxf)), int(math.Floor(syf))
	w, h := ent.GetSize()

	l, r, t, b := spriteHitRect(ex, ey, w, h, mr.Scale())

	return l <= mx && r >= mx && t <= my && b >= my
}

// tacticalSpriteHit is the paced fight's hit test on an enemy's sprite: the
// sprite as a PNG creature draws it, feet-anchored and centred, at the view's
// scale.
func tacticalSpriteHit(mr *d2maprenderer.MapRenderer, ent d2interface.MapEntity, mx, my int) bool {
	sx, sy := mr.WorldToScreenF(ent.GetPositionF())
	w, hgt := ent.GetSize()
	w, hgt = mr.ScaleLength(w), mr.ScaleLength(hgt)

	return float64(mx) >= sx-float64(w)/2 && float64(mx) <= sx+float64(w)/2 &&
		float64(my) >= sy-float64(hgt) && float64(my) <= sy
}
