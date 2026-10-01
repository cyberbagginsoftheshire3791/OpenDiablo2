package d2maprenderer

import (
	"image"
	"image/color"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// THE GAME ZOOMS OUT; THE ART AND THE WORLD'S DISTANCES DO NOT CHANGE (Josh, 1
// Oct 2026). drawTileArt scales the floors, walls and shadows with the view;
// renderEntity, below, does the same for every entity the map renderer draws --
// the hero's composite or PNG sheets, NPCs, the Strigoi PNG creatures, objects,
// missiles and cast overlays, and the shadows their animations draw -- so a
// squad of five at 0.5 stands on the ground at half size instead of at full
// size on half-size ground.

// renderEntity draws one entity at the target's current translation, which the
// render pass has already set to the entity's tile through the viewport.
//
// AT SCALE 1.0 THIS IS e.Render(target) AND NOTHING ELSE -- the call the passes
// made before there was a zoom, call for call: no wrapper, no PushScale, no Pop.
// TestRenderEntityAtScale1IsTheUnscaledDraw pins it.
//
// AT ANY OTHER SCALE the entity draws through a viewScaledSurface: the view's
// scale is pushed on the target AFTER the tile's translation (the ebiten surface
// scales a picture about its own top-left and then translates it, so the scale
// must ride with the translation, exactly as in drawTileArt), and every
// translation the entity pushes on its own -- its sub-tile offset, each frame's
// offset from the sprite's origin, the shadow's half-height shift -- is scaled
// with it. Scaling only the pictures would leave each one at its full-size
// offset from the feet: a 100-pixel hero at 0.5 would hang 50 pixels above
// where he stands.
//
// A SHRUNK SPRITE IS SAMPLED LINEARLY (the zoom review, B2). The surface's
// default filter is nearest, which at 0.4 keeps two pixels in five and throws
// the rest away: a figure's outline and a blade's edge break up and shimmer as
// he walks. ebiten (2.9) mipmaps a linear minification, so FilterLinear is
// pushed under the scale; an entity that pushes a filter of its own (the
// shadow's linear) keeps it. Tiles stay nearest (drawTileArt): their seams
// are measured against a nearest draw, and a linear one is a real-render
// check still owed (docs/camera.md).
func (mr *MapRenderer) renderEntity(target d2interface.Surface, e d2interface.MapEntity) {
	s := mr.viewport.Scale()
	if s == defaultScale {
		e.Render(target)
		return
	}

	target.PushFilter(d2enum.FilterLinear)
	target.PushScale(s, s)
	e.Render(newViewScaledSurface(target, s))
	target.PopN(2)
}

// NewViewOnlyMapRenderer is a MapRenderer with an 800x600 viewport and a camera
// at the given orthogonal point and nothing else -- no map engine, no assets,
// nothing to draw. Its transforms, Scale, SetScale and ScaleLength work; Render
// does not. It is for code that projects through a renderer without a map: the
// HUD's hit tests and anchors in other packages' tests.
func NewViewOnlyMapRenderer(camOrthoX, camOrthoY float64) *MapRenderer {
	position := d2vector.NewPosition(camOrthoX, camOrthoY)
	mr := &MapRenderer{viewport: NewViewport(0, 0, 800, 600)}
	mr.Camera = Camera{position: &position}
	mr.viewport.SetCamera(&mr.Camera)

	return mr
}

// ScaleLength is n screen pixels of something drawn in world space -- an
// entity's width or height, an offset from its feet -- at the view's scale. At
// exactly 1.0 it is n, unrounded and untouched, so every caller's arithmetic at
// 1.0 is the arithmetic it had before the zoom.
func ScaleLength(n int, scale float64) int {
	if scale == defaultScale {
		return n
	}

	return int(math.Round(float64(n) * scale))
}

// ScaleLength is n world-space screen pixels at this renderer's view scale. The
// overhead bars, the hover label, the selection and tactical hit tests all size
// an entity through it, so what they test is the sprite the player sees.
func (mr *MapRenderer) ScaleLength(n int) int {
	return ScaleLength(n, mr.viewport.Scale())
}

// viewScaledSurface is the target as an entity sees it at a view scale s: a
// d2interface.Surface whose translations, scales and drawn lengths are all
// multiplied by s about the point it was made at (the entity's tile anchor),
// so code that draws an entity at full size draws it at s without knowing.
//
// Every Push on it is exactly one Push on the target, and every Pop one Pop,
// so the target's state stack is balanced exactly as the entity balances this
// one. Translations are kept as the entity's own unscaled sum and the target is
// moved to the ROUNDED scaled sum each time, so rounding never accumulates
// across a chain of pushes (composite layer -> frame offset -> origin).
//
// Text is not scaled: DrawTextf prints at the scaled position at the font's
// own size. Nothing in an entity's Render draws text today.
type viewScaledSurface struct {
	d2interface.Surface // the target: Renderer, Clear, GetSize, GetDepth, Screenshot... pass through

	s float64

	cur   viewScaledState
	stack []viewScaledState
}

type viewScaledState struct {
	ux, uy int     // the entity's own translation, unscaled, since the wrapper was made
	ix, iy int     // what has been pushed on the target for it: round(u * s)
	sx, sy float64 // the entity's own scale (PushScale assigns), unscaled
}

func newViewScaledSurface(target d2interface.Surface, s float64) *viewScaledSurface {
	return &viewScaledSurface{Surface: target, s: s, cur: viewScaledState{sx: 1, sy: 1}}
}

func (v *viewScaledSurface) save() { v.stack = append(v.stack, v.cur) }

// PushTranslation moves the target to the scaled sum of every translation the
// entity has pushed.
func (v *viewScaledSurface) PushTranslation(x, y int) {
	v.save()

	v.cur.ux += x
	v.cur.uy += y

	ix := int(math.Round(float64(v.cur.ux) * v.s))
	iy := int(math.Round(float64(v.cur.uy) * v.s))

	v.Surface.PushTranslation(ix-v.cur.ix, iy-v.cur.iy)
	v.cur.ix, v.cur.iy = ix, iy
}

// PushScale is the entity's own scale (the shadow's 1 x 0.5) times the view's.
// The surface ASSIGNS a scale, so this assigns the product.
func (v *viewScaledSurface) PushScale(x, y float64) {
	v.save()
	v.cur.sx, v.cur.sy = x, y
	v.Surface.PushScale(x*v.s, y*v.s)
}

// The rest of the state stack passes through, one push for one push.

func (v *viewScaledSurface) PushSkew(x, y float64) { v.save(); v.Surface.PushSkew(x, y) }
func (v *viewScaledSurface) PushColor(c color.Color) {
	v.save()
	v.Surface.PushColor(c)
}
func (v *viewScaledSurface) PushEffect(e d2enum.DrawEffect) { v.save(); v.Surface.PushEffect(e) }
func (v *viewScaledSurface) PushFilter(f d2enum.Filter)     { v.save(); v.Surface.PushFilter(f) }
func (v *viewScaledSurface) PushBrightness(b float64)       { v.save(); v.Surface.PushBrightness(b) }
func (v *viewScaledSurface) PushSaturation(s float64)       { v.save(); v.Surface.PushSaturation(s) }

// Pop restores the entity's previous state and the target's with it.
func (v *viewScaledSurface) Pop() {
	n := len(v.stack)
	if n == 0 {
		panic("empty stack")
	}

	v.cur = v.stack[n-1]
	v.stack = v.stack[:n-1]
	v.Surface.Pop()
}

// PopN pops n states.
func (v *viewScaledSurface) PopN(n int) {
	for i := 0; i < n; i++ {
		v.Pop()
	}
}

// DrawRect and DrawLine take lengths in the entity's pixels; the surface draws
// them unscaled, so they are scaled here.
func (v *viewScaledSurface) DrawRect(width, height int, c color.Color) {
	v.Surface.DrawRect(ScaleLength(width, v.s), ScaleLength(height, v.s), c)
}

func (v *viewScaledSurface) DrawLine(x, y int, c color.Color) {
	v.Surface.DrawLine(ScaleLength(x, v.s), ScaleLength(y, v.s), c)
}

// Render and RenderSection draw through the target, whose state already holds
// the scaled translation and scale. They are written out so the wrapper, not
// the embedded target, is what satisfies the interface for every draw.
func (v *viewScaledSurface) Render(sfc d2interface.Surface) { v.Surface.Render(sfc) }

func (v *viewScaledSurface) RenderSection(sfc d2interface.Surface, bound image.Rectangle) {
	v.Surface.RenderSection(sfc, bound)
}

var _ d2interface.Surface = (*viewScaledSurface)(nil)
