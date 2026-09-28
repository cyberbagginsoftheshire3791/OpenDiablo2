package d2maprenderer

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2ds1"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
)

// THE 28 SEP REVIEW'S A1: ZOOM MOVED THE ART BUT DID NOT SCALE IT. The viewport
// scaled every position and nothing scaled the pictures, so the World Editor's
// first screen -- the whole village at ~0.08 -- was full-size trees and houses
// on anchors 0.08 apart, and every floor tile hung (80(1-s), 40(1-s)) pixels
// off its own diamond. The earlier tests all measured POSITIONS, which were
// right; these measure PIXELS.
//
// paintSurface is a software d2interface.Surface that does what the ebiten
// surface does with its state stack (d2core/d2render/ebiten/ebiten_surface.go):
// PushTranslation ADDS, PushScale ASSIGNS, and Render draws the source scaled
// about its own top-left and THEN translated (createDrawImageOptions: GeoM.Scale
// before GeoM.Translate), sampling the source nearest-neighbour at each
// destination pixel's centre, as a GPU does with the default filter. It paints
// which destination pixels a picture covers, so a test can ask where the art
// really went. This package links no ebiten (the headless-CI rule), which is
// why the surface is written out here rather than borrowed.
type paintState struct {
	x, y   int
	sx, sy float64
}

type paintSurface struct {
	w, h   int
	opaque []bool // for a source: which pixels are ink; for a target: which were painted

	cur   paintState
	stack []paintState

	renders, scalePushes int
}

func newPaintSurface(w, h int) *paintSurface {
	return &paintSurface{w: w, h: h, opaque: make([]bool, w*h), cur: paintState{sx: 1, sy: 1}}
}

// diamondArt is a floor tile's picture: an opaque diamond touching the middle
// of each edge of a w x h box, the shape of every floor tile in the game.
func diamondArt(w, h int) *paintSurface {
	s := newPaintSurface(w, h)
	hw, hh := float64(w)/2, float64(h)/2

	for v := 0; v < h; v++ {
		for u := 0; u < w; u++ {
			if math.Abs(float64(u)+0.5-hw)/hw+math.Abs(float64(v)+0.5-hh)/hh <= 1 {
				s.opaque[v*w+u] = true
			}
		}
	}

	return s
}

// blockArt is a wall's picture: every pixel ink.
func blockArt(w, h int) *paintSurface {
	s := newPaintSurface(w, h)
	for i := range s.opaque {
		s.opaque[i] = true
	}

	return s
}

func (p *paintSurface) push() { p.stack = append(p.stack, p.cur) }

func (p *paintSurface) Renderer() d2interface.Renderer { return nil }
func (p *paintSurface) Clear(color.Color)              {}
func (p *paintSurface) DrawRect(int, int, color.Color) {}
func (p *paintSurface) DrawLine(int, int, color.Color) {}
func (p *paintSurface) DrawTextf(string, ...interface{}) {
}
func (p *paintSurface) GetSize() (width, height int) { return p.w, p.h }
func (p *paintSurface) GetDepth() int                { return len(p.stack) }

func (p *paintSurface) Pop() {
	p.cur = p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
}

func (p *paintSurface) PopN(n int) {
	for i := 0; i < n; i++ {
		p.Pop()
	}
}

func (p *paintSurface) PushColor(color.Color)        { p.push() }
func (p *paintSurface) PushEffect(d2enum.DrawEffect) { p.push() }
func (p *paintSurface) PushFilter(d2enum.Filter)     { p.push() }
func (p *paintSurface) PushSkew(float64, float64)    { p.push() }
func (p *paintSurface) PushBrightness(float64)       { p.push() }
func (p *paintSurface) PushSaturation(float64)       { p.push() }
func (p *paintSurface) ReplacePixels([]byte)         {}
func (p *paintSurface) Screenshot() *image.RGBA      { return nil }
func (p *paintSurface) RenderSection(d2interface.Surface, image.Rectangle) {
}

func (p *paintSurface) PushTranslation(x, y int) {
	p.push()
	p.cur.x += x
	p.cur.y += y
}

func (p *paintSurface) PushScale(x, y float64) {
	p.push()
	p.scalePushes++
	p.cur.sx, p.cur.sy = x, y
}

// Render paints every destination pixel whose centre samples an ink pixel of
// src under the current scale-then-translate.
func (p *paintSurface) Render(sfc d2interface.Surface) {
	src := sfc.(*paintSurface)
	p.renders++

	x0, y0 := float64(p.cur.x), float64(p.cur.y)
	x1, y1 := x0+float64(src.w)*p.cur.sx, y0+float64(src.h)*p.cur.sy

	for j := int(math.Floor(y0)); j < int(math.Ceil(y1)); j++ {
		for i := int(math.Floor(x0)); i < int(math.Ceil(x1)); i++ {
			if i < 0 || j < 0 || i >= p.w || j >= p.h {
				continue
			}

			u := int(math.Floor((float64(i) + 0.5 - x0) / p.cur.sx))
			v := int(math.Floor((float64(j) + 0.5 - y0) / p.cur.sy))

			if u < 0 || v < 0 || u >= src.w || v >= src.h || !src.opaque[v*src.w+u] {
				continue
			}

			p.opaque[j*p.w+i] = true
		}
	}
}

// inkBounds is the smallest rectangle holding every painted pixel.
func (p *paintSurface) inkBounds() image.Rectangle {
	r := image.Rectangle{}

	for j := 0; j < p.h; j++ {
		for i := 0; i < p.w; i++ {
			if p.opaque[j*p.w+i] {
				r = r.Union(image.Rect(i, j, i+1, j+1))
			}
		}
	}

	return r
}

// testRenderer is a MapRenderer with nothing but a viewport and an image cache:
// enough for renderFloor and renderWall, which is what the zoom touches.
func testRenderer(scale float64, img d2interface.Surface, style, sequence byte, tileType d2enum.TileType) *MapRenderer {
	mr := &MapRenderer{viewport: newTestViewport(4000, 2000)}
	mr.viewport.SetScale(scale)
	mr.setImageCacheRecord(style, sequence, tileType, 0, img)

	return mr
}

// near says whether two rectangles agree edge by edge to within tol pixels.
func near(a, b image.Rectangle, tol int) bool {
	abs := func(n int) int {
		if n < 0 {
			return -n
		}

		return n
	}

	return abs(a.Min.X-b.Min.X) <= tol && abs(a.Min.Y-b.Min.Y) <= tol &&
		abs(a.Max.X-b.Max.X) <= tol && abs(a.Max.Y-b.Max.Y) <= tol
}

// TestDrawTileArtScalesTheArt draws one floor tile through the real renderFloor
// at the editor's zooms and measures the pixels it painted against the tile's
// own diamond, worked out independently through WorldToScreen at its four
// corners. Tolerance: 2 px -- one for flooring the translation to a whole pixel,
// one for drawTileArt's seam pad.
//
// Negative control (28 Sep 2026): make drawTileArt call target.Render(img) at
// every scale -- the code as the review found it -- and this fails at every
// zoom tried: "at scale 0.08 the floor tile painted (394,296)-(552,376), its
// diamond on screen is (393,296)-(407,304)" -- ink 158 pixels wide where the
// diamond is 14 (strigoi-harness-runs\wt-editor-fix\nc\nc1-no-pushscale.txt).
func TestDrawTileArtScalesTheArt(t *testing.T) {
	const tx, ty = 30, 20

	for _, scale := range []float64{0.08, 0.25, 0.5, 2} {
		target := newPaintSurface(800, 600)
		mr := testRenderer(scale, diamondArt(tileSurfaceWidth, tileSurfaceHeight), 3, 7, d2enum.TileFloor)

		// Aim the camera at the tile so it is on the canvas at every zoom.
		cx, cy := mr.viewport.WorldToOrtho(tx+0.5, ty+0.5)
		moveTestCamera(mr.viewport, cx, cy)

		var tile d2ds1.Tile
		tile.Style, tile.Sequence = 3, 7

		mr.viewport.PushTranslationWorld(tx, ty)
		mr.renderFloor(tile, target)
		mr.viewport.PopTranslation()

		topX, topY := mr.viewport.WorldToScreenF(tx, ty)
		rightX, _ := mr.viewport.WorldToScreenF(tx+1, ty)
		_, bottomY := mr.viewport.WorldToScreenF(tx+1, ty+1)
		leftX, _ := mr.viewport.WorldToScreenF(tx, ty+1)

		want := image.Rect(int(math.Floor(leftX)), int(math.Floor(topY)),
			int(math.Ceil(rightX)), int(math.Ceil(bottomY)))
		got := target.inkBounds()

		if !near(got, want, 2) {
			t.Errorf("at scale %v the floor tile painted %v, its diamond on screen is %v (top corner at %.1f,%.1f)",
				scale, got, want, topX, topY)
		}

		if target.GetDepth() != 0 {
			t.Errorf("at scale %v renderFloor left %d state(s) pushed", scale, target.GetDepth())
		}
	}
}

// TestDrawTileArtScalesATallWall is the same for the art that was most wrong on
// the review's screenshots: a tall wall -- a tree, a house strip -- standing up
// from its tile. At 0.08 the review's trees were drawn a dozen times too big and
// hid a placed house. The wall's box is where the viewport puts its top-left
// (AuthoredWallLeft, YAdjust) and its size times the scale.
func TestDrawTileArtScalesATallWall(t *testing.T) {
	const (
		tx, ty = 12, 9
		w, h   = 160, 300
	)

	for _, scale := range []float64{0.08, 0.25, 0.5, 2} {
		target := newPaintSurface(800, 600)
		mr := testRenderer(scale, blockArt(w, h), d2mapengine.AuthoredStyle, 4, d2mapengine.AuthoredWallType)

		// Aim the camera at the middle of the wall, so a 600-pixel wall at 2.0
		// is on the canvas.
		cx, cy := mr.viewport.WorldToOrtho(tx, ty)
		moveTestCamera(mr.viewport, cx, cy-float64(h-80)+float64(h)/2)

		var tile d2ds1.Tile
		tile.Style, tile.Sequence, tile.Type, tile.YAdjust = d2mapengine.AuthoredStyle, 4, d2mapengine.AuthoredWallType, -(h - 80)

		mr.viewport.PushTranslationWorld(tx, ty)
		mr.renderWall(tile, mr.viewport, target)
		mr.viewport.PopTranslation()

		ox, oy := mr.viewport.WorldToOrtho(tx, ty)
		ox += d2mapengine.AuthoredWallLeft(d2mapengine.AuthoredWallType, w)
		oy += float64(tile.YAdjust)

		x0, y0 := mr.viewport.OrthoToScreenF(ox, oy)
		want := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)),
			int(math.Floor(x0))+int(math.Round(w*scale)), int(math.Floor(y0))+int(math.Round(h*scale)))
		got := target.inkBounds()

		if !near(got, want, 2) {
			t.Errorf("at scale %v a %dx%d wall painted %v (%dx%d), want %v (%dx%d)",
				scale, w, h, got, got.Dx(), got.Dy(), want, want.Dx(), want.Dy())
		}
	}
}

// TestDrawTileArtAtScale1IsTheUnscaledDraw is the hard requirement: the shipped
// game, which never leaves 1.0, must draw exactly what it drew before. At 1.0
// drawTileArt pushes NO scale, and the tile's ink is exactly the picture at the
// whole-pixel translation the viewport gives -- the pre-zoom arithmetic, not
// something within a tolerance of it.
//
// Negative control (28 Sep 2026): drop drawTileArt's scale-1 early return, so
// 1.0 goes through PushScale(seamScale(...)) like every other scale, and this
// fails: "at 1.0 drawTileArt pushed 1 scale(s); the shipped game's draw call
// must be target.Render alone" (nc2-no-scale1-branch.txt).
func TestDrawTileArtAtScale1IsTheUnscaledDraw(t *testing.T) {
	const tx, ty = 30, 20

	target := newPaintSurface(800, 600)
	art := diamondArt(tileSurfaceWidth, tileSurfaceHeight)
	mr := testRenderer(1, art, 3, 7, d2enum.TileFloor)

	cx, cy := mr.viewport.WorldToOrtho(tx+0.5, ty+0.5)
	moveTestCamera(mr.viewport, cx, cy)

	var tile d2ds1.Tile
	tile.Style, tile.Sequence = 3, 7

	mr.viewport.PushTranslationWorld(tx, ty)

	// The translation renderFloor gives the picture, computed the way it always
	// has been: -80 ortho, the YAdjust, then the viewport's floor to a pixel.
	mr.viewport.PushTranslationOrtho(-80, 0)
	x, y := mr.viewport.GetTranslationScreen()
	mr.viewport.PopTranslation()

	mr.renderFloor(tile, target)
	mr.viewport.PopTranslation()

	if target.scalePushes != 0 {
		t.Fatalf("at 1.0 drawTileArt pushed %d scale(s); the shipped game's draw call must be target.Render alone",
			target.scalePushes)
	}

	// The ink is the picture's own ink, shifted by the translation, pixel for
	// pixel.
	for v := 0; v < art.h; v++ {
		for u := 0; u < art.w; u++ {
			i, j := x+u, y+v
			if i < 0 || j < 0 || i >= target.w || j >= target.h {
				continue
			}

			if target.opaque[j*target.w+i] != art.opaque[v*art.w+u] {
				t.Fatalf("at 1.0 pixel %d,%d of the tile drew %v at %d,%d, the picture has %v",
					u, v, target.opaque[j*target.w+i], i, j, art.opaque[v*art.w+u])
			}
		}
	}

	if got, want := target.inkBounds(), image.Rect(x, y, x+tileSurfaceWidth, y+tileSurfaceHeight).Intersect(image.Rect(0, 0, 800, 600)); !got.In(want) {
		t.Fatalf("at 1.0 the ink %v strays outside the picture's own box %v", got, want)
	}
}

// TestSeamScale pins the seam pad's arithmetic: n pixels at scale s become
// n*s + seamPad screen pixels.
func TestSeamScale(t *testing.T) {
	for _, c := range []struct {
		n int
		s float64
	}{{160, 0.08}, {80, 0.25}, {300, 0.5}, {160, 2}} {
		if got, want := float64(c.n)*seamScale(c.n, c.s), float64(c.n)*c.s+seamPad; math.Abs(got-want) > 1e-9 {
			t.Errorf("seamScale(%d, %v) draws %v px, want %v", c.n, c.s, got, want)
		}
	}

	if got := seamScale(0, 0.5); got != 0.5 {
		t.Errorf("an empty image is drawn at the view's scale, got %v", got)
	}
}
