package d2maprenderer

import (
	"image"
	"math"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// THE GAME ZOOM (1 Oct 2026): entities scale with the view. These tests draw a
// stand-in entity through renderEntity onto the paintSurface (draw_scale_test.go)
// and measure the pixels it painted against where the viewport, independently,
// puts the entity's feet.

// inkEntity is a map entity that draws the way the game's do: the sub-tile
// offset of its feet from the tile's anchor (Player.Render, NPC.Render), then the
// sprite's origin at its bottom (Animation.RenderFromOrigin: up by the frame's
// height), then the frame's own offset (Animation.Render: left by half its
// width), then the picture. shadowScale, when set, is pushed before the picture
// the way Animation.renderShadow pushes its 1 x 0.5.
type inkEntity struct {
	d2interface.MapEntity // nil: only Render is called

	art         *paintSurface
	feetX       int // the feet's ortho offset from the tile's anchor
	feetY       int
	shadowScale bool

	got d2interface.Surface // the surface Render was handed
}

func (e *inkEntity) Render(target d2interface.Surface) {
	e.got = target

	target.PushTranslation(e.feetX, e.feetY)
	defer target.Pop()

	target.PushTranslation(0, -e.art.h)
	defer target.Pop()

	target.PushTranslation(-e.art.w/2, 0)
	defer target.Pop()

	if e.shadowScale {
		target.PushScale(1, 0.5)
		defer target.Pop()
	}

	target.Render(e.art)
}

// The entity stands at world (tx + 0.5, ty + 0.25): ortho offset
// ((0.5-0.25)*80, (0.5+0.25)*40) = (20, 30) from its tile's anchor.
const (
	entTileX, entTileY = 30, 20
	entFracX, entFracY = 0.5, 0.25
	entFeetX, entFeetY = 20, 30
	entW, entH         = 60, 100
)

// drawInkEntity draws e on its tile the way renderPass2 and renderPass3 do: the
// tile's translation through the viewport, then renderEntity.
func drawInkEntity(scale float64, e *inkEntity) (*paintSurface, *MapRenderer) {
	target := newPaintSurface(800, 600)
	mr := &MapRenderer{viewport: newTestViewport(4000, 2000)}
	mr.viewport.SetScale(scale)

	cx, cy := mr.viewport.WorldToOrtho(entTileX+entFracX, entTileY+entFracY)
	moveTestCamera(mr.viewport, cx, cy)

	mr.viewport.PushTranslationWorld(entTileX, entTileY)
	target.PushTranslation(mr.viewport.GetTranslationScreen())
	mr.renderEntity(target, e)
	target.Pop()
	mr.viewport.PopTranslation()

	return target, mr
}

// TestRenderEntityScalesTheSprite: at every game zoom (and two outside it) the
// sprite's ink is its picture at the scale, standing on the feet the viewport
// puts at the entity's world position. Tolerance 2 px: one for flooring the
// tile's translation, one for rounding the scaled offsets.
//
// Negative control (1 Oct 2026): make renderEntity call e.Render(target) at
// every scale -- entities as they were -- and this fails at every scale but
// 1.0, e.g. "at scale 0.5 the sprite painted (380,215)-(440,315), drawn at
// the scale it is (385,250)-(415,300)" (strigoi-harness-runs\wt-zoom\nc\
// nc1-no-entity-scale.txt). Make viewScaledSurface.PushTranslation forward the
// entity's offsets unscaled and it fails too: the picture shrinks but hangs at
// its full-size offset from the feet, "at scale 0.5 the sprite painted
// (380,215)-(410,265)" -- 35 pixels above where he stands
// (nc3-unscaled-offsets.txt).
func TestRenderEntityScalesTheSprite(t *testing.T) {
	for _, scale := range []float64{0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.25, 2} {
		e := &inkEntity{art: blockArt(entW, entH), feetX: entFeetX, feetY: entFeetY}
		target, mr := drawInkEntity(scale, e)

		fx, fy := mr.viewport.WorldToScreenF(entTileX+entFracX, entTileY+entFracY)
		want := image.Rect(
			int(math.Round(fx-entW/2*scale)), int(math.Round(fy-entH*scale)),
			int(math.Round(fx+entW/2*scale)), int(math.Round(fy)))
		got := target.inkBounds()

		if !near(got, want, 2) {
			t.Errorf("at scale %v the sprite painted %v, drawn at the scale it is %v (feet at %.1f,%.1f)",
				scale, got, want, fx, fy)
		}

		if target.GetDepth() != 0 {
			t.Errorf("at scale %v renderEntity left %d state(s) pushed on the target", scale, target.GetDepth())
		}
	}
}

// TestRenderEntityAtScale1IsTheUnscaledDraw: at 1.0 the entity is handed the
// target itself -- no wrapper, no scale pushed -- and paints its picture pixel
// for pixel at the translation it always had.
//
// Negative control (1 Oct 2026): drop renderEntity's scale-1 early return, so
// 1.0 goes through the wrapper like every other scale, and this fails: "at 1.0
// the entity was handed *d2maprenderer.viewScaledSurface, not the target"
// (nc2-no-scale1-branch.txt).
func TestRenderEntityAtScale1IsTheUnscaledDraw(t *testing.T) {
	e := &inkEntity{art: blockArt(entW, entH), feetX: entFeetX, feetY: entFeetY}
	target, mr := drawInkEntity(1, e)

	if e.got != d2interface.Surface(target) {
		t.Fatalf("at 1.0 the entity was handed %T, not the target", e.got)
	}

	if target.scalePushes != 0 {
		t.Fatalf("at 1.0 renderEntity pushed %d scale(s); the shipped game's draw is e.Render(target) alone",
			target.scalePushes)
	}

	mr.viewport.PushTranslationWorld(entTileX, entTileY)
	ax, ay := mr.viewport.GetTranslationScreen()
	mr.viewport.PopTranslation()

	x0, y0 := ax+entFeetX-entW/2, ay+entFeetY-entH
	want := image.Rect(x0, y0, x0+entW, y0+entH)

	if got := target.inkBounds(); got != want {
		t.Fatalf("at 1.0 the sprite painted %v; unscaled it is %v", got, want)
	}

	if n := countInk(target); n != entW*entH {
		t.Fatalf("at 1.0 the sprite painted %d pixels; its picture has %d", n, entW*entH)
	}
}

// TestRenderEntityScalesAnEntitysOwnScale: an entity that pushes a scale of its
// own (a shadow's 1 x 0.5) draws at that scale times the view's, because the
// surface ASSIGNS a scale and would otherwise lose the view's.
//
// Negative control (1 Oct 2026): make viewScaledSurface.PushScale pass x, y
// through unmultiplied and this fails: "at scale 0.5 the shadow painted 60 x
// 50, its picture at the scale is 30 x 25" (nc4-own-scale-unmultiplied.txt).
func TestRenderEntityScalesAnEntitysOwnScale(t *testing.T) {
	for _, scale := range []float64{0.4, 0.5, 0.8} {
		e := &inkEntity{art: blockArt(entW, entH), feetX: entFeetX, feetY: entFeetY, shadowScale: true}
		target, _ := drawInkEntity(scale, e)

		got := target.inkBounds()
		wantW, wantH := float64(entW)*scale, float64(entH)*0.5*scale

		if math.Abs(float64(got.Dx())-wantW) > 1.5 || math.Abs(float64(got.Dy())-wantH) > 1.5 {
			t.Errorf("at scale %v the shadow painted %d x %d, its picture at the scale is %.0f x %.0f",
				scale, got.Dx(), got.Dy(), wantW, wantH)
		}
	}
}

// TestViewScaledSurfaceDoesNotAccumulateRounding: a chain of small offsets
// lands where their sum, scaled once, does -- not where each one rounded on its
// own would put it (ten 3s at 0.5 are 15, not ten 2s).
//
// Negative control (1 Oct 2026): make PushTranslation forward
// round(x*s), round(y*s) per push and this fails: "ten pushes of 3 at 0.5 moved
// the target 20, 20; the sum scaled is 15, 15" (nc5-per-push-rounding.txt).
func TestViewScaledSurfaceDoesNotAccumulateRounding(t *testing.T) {
	target := newPaintSurface(10, 10)
	v := newViewScaledSurface(target, 0.5)

	for i := 0; i < 10; i++ {
		v.PushTranslation(3, 3)
	}

	if target.cur.x != 15 || target.cur.y != 15 {
		t.Fatalf("ten pushes of 3 at 0.5 moved the target %d, %d; the sum scaled is 15, 15", target.cur.x, target.cur.y)
	}

	v.PopN(10)

	if target.cur.x != 0 || target.cur.y != 0 || target.GetDepth() != 0 {
		t.Fatalf("after ten pops the target is at %d, %d with %d state(s); it must be back where it began",
			target.cur.x, target.cur.y, target.GetDepth())
	}
}

// TestScaleLength: 1.0 is the identity, odd and negative numbers included;
// other scales round to the nearest pixel.
//
// Negative control (1 Oct 2026): make ScaleLength truncate instead of round and
// this fails: "ScaleLength(101, 0.5) = 50, want 51" (nc6-truncate.txt).
func TestScaleLength(t *testing.T) {
	for _, n := range []int{-101, -1, 0, 1, 5, 99, 100, 101} {
		if got := ScaleLength(n, 1); got != n {
			t.Errorf("ScaleLength(%d, 1) = %d; at 1.0 it must be %d", n, got, n)
		}
	}

	for _, c := range []struct {
		n    int
		s    float64
		want int
	}{
		{100, 0.5, 50}, {101, 0.5, 51}, {99, 0.4, 40}, {-100, 0.5, -50}, {5, 0.4, 2}, {60, 0.7, 42},
	} {
		if got := ScaleLength(c.n, c.s); got != c.want {
			t.Errorf("ScaleLength(%d, %v) = %d, want %d", c.n, c.s, got, c.want)
		}
	}
}

func countInk(p *paintSurface) int {
	n := 0

	for _, o := range p.opaque {
		if o {
			n++
		}
	}

	return n
}
