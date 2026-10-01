package d2player

import (
	"math"
	"testing"
)

// THE GAME ZOOM (1 Oct 2026): the hit boxes and the head anchor measure the
// sprite at the view's scale, and at 1.0 they are the arithmetic the hover loop,
// the squad hit test and the overhead bars had before the zoom.

// legacyHitRect is the hover loop's rect as it read before the zoom
// (hud.go hoveredEntityWhere, squad_selection.go squadAtScreen).
func legacyHitRect(ex, ey, w, h int) (l, r, t, b int) {
	halfW, halfH := w>>1, h>>1

	return ex - halfW - hoverLabelOuterPad, ex + halfW + hoverLabelOuterPad,
		ey - halfH - hoverLabelOuterPad, ey + halfH - hoverLabelOuterPad
}

// TestSpriteHitRectAt1IsTheHoverLoopsRect: at 1.0, for odd and even sizes, the
// rect is the old one exactly.
func TestSpriteHitRectAt1IsTheHoverLoopsRect(t *testing.T) {
	for _, c := range [][4]int{{400, 300, 60, 100}, {123, 456, 61, 99}, {0, 0, 1, 1}, {799, 599, 300, 301}} {
		l, r, tp, b := spriteHitRect(c[0], c[1], c[2], c[3], 1)
		wl, wr, wt, wb := legacyHitRect(c[0], c[1], c[2], c[3])

		if l != wl || r != wr || tp != wt || b != wb {
			t.Errorf("at 1.0 %v: rect (%d,%d,%d,%d), the hover loop's is (%d,%d,%d,%d)", c, l, r, tp, b, wl, wr, wt, wb)
		}
	}
}

// TestSpriteHitRectFollowsTheSprite: at 0.5 a 60 x 100 sprite is 30 x 50 on
// screen, so the rect is that plus the UI pad -- a click 40 pixels above the
// feet of a half-size man misses him, as it would miss his sprite.
//
// Negative control (1 Oct 2026): make spriteHitRect ignore the scale and this
// fails: "at 0.5 the rect is (365,435,245,345), the half-size sprite's is
// (380,420,270,320)" (strigoi-harness-runs\wt-zoom\nc\nc7-hitrect-unscaled.txt).
func TestSpriteHitRectFollowsTheSprite(t *testing.T) {
	l, r, tp, b := spriteHitRect(400, 300, 60, 100, 0.5)
	wl, wr, wt, wb := 400-15-hoverLabelOuterPad, 400+15+hoverLabelOuterPad, 300-25-hoverLabelOuterPad, 300+25-hoverLabelOuterPad

	if l != wl || r != wr || tp != wt || b != wb {
		t.Fatalf("at 0.5 the rect is (%d,%d,%d,%d), the half-size sprite's is (%d,%d,%d,%d)", l, r, tp, b, wl, wr, wt, wb)
	}
}

// TestHeadAnchorAt1IsTheOldArithmetic and TestHeadAnchorHangsOverTheScaledHead:
// the label and the bar hang over the head as drawn; the pad stays UI pixels.
//
// Negative control (1 Oct 2026): make headAnchor leave the height unscaled and
// the second fails: "at 0.5 a 100-pixel man's bar hangs from y 195; his head
// is drawn at y 250, so it hangs from 245" (nc8-anchor-unscaled.txt).
func TestHeadAnchorAt1IsTheOldArithmetic(t *testing.T) {
	for _, c := range [][5]int{{400, 300, 3, 4, 100}, {10, 20, 1, 6, 77}, {400, 300, 0, 0, 0}} {
		x, y := headAnchor(c[0], c[1], c[2], c[3], c[4], 1)
		wx, wy := c[0]-c[2], c[1]-c[3]-c[4]-hoverLabelOuterPad

		if x != wx || y != wy {
			t.Errorf("at 1.0 %v: anchor (%d,%d), before the zoom (%d,%d)", c, x, y, wx, wy)
		}
	}
}

func TestHeadAnchorHangsOverTheScaledHead(t *testing.T) {
	x, y := headAnchor(400, 300, 0, 0, 100, 0.5)

	if x != 400 || y != 300-50-hoverLabelOuterPad {
		t.Fatalf("at 0.5 a 100-pixel man's bar hangs from y %d; his head is drawn at y 250, so it hangs from %d",
			y, 300-50-hoverLabelOuterPad)
	}

	x, y = headAnchor(400, 300, 4, 6, 100, 0.5)
	if x != 398 || y != 300-3-50-hoverLabelOuterPad {
		t.Fatalf("at 0.5 the render offset (4, 6) moves the anchor to (%d,%d); scaled it is (398,%d)",
			x, y, 300-3-50-hoverLabelOuterPad)
	}
}

// TestTheWheelStepsBetween04And1: six notches out from 1.0 reach 0.4 and stop
// there, and every notch out and back lands EXACTLY on the decimal it names
// (0.8, not 0.7999999999999999 -- a harness script compares view_scale with
// ==), ending on 1.0, the shipped view; a roll with no y does nothing.
//
// Negative control (1 Oct 2026): drop nextGameZoom's rounding to the step and
// this fails: "notch 3 out: 0.7000000000000001, want 0.7"
// (nc9-no-step-rounding.txt). The first rounding written, round(z/0.1)*0.1,
// fails it too, the same way: 7*0.1 is 0.7000000000000001 (nc9b-round-times-step.txt).
func TestTheWheelStepsBetween04And1(t *testing.T) {
	v := &fakeZoom{scale: 1}

	want := []float64{0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.4, 0.4}
	for i, w := range want {
		if !zoomByNotch(v, -1) {
			t.Fatalf("notch %d out did nothing", i+1)
		}

		if v.scale != w {
			t.Fatalf("notch %d out: %v, want %v", i+1, v.scale, w)
		}
	}

	back := []float64{0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.0, 1.0}
	for i, w := range back {
		zoomByNotch(v, 1)

		if v.scale != w {
			t.Fatalf("notch %d back: %v, want %v", i+1, v.scale, w)
		}
	}

	if v.scale != 1.0 {
		t.Fatalf("eight notches back from 0.4 reach %v, not 1.0", v.scale)
	}

	if zoomByNotch(v, 0) || v.sets != 16 {
		t.Fatalf("a roll with no y zoomed (sets %d, want 16)", v.sets)
	}

	if got := nextGameZoom(1, 3); got != 1 {
		t.Fatalf("a notch in from 1.0 went to %v; the game does not zoom in past its shipped view", got)
	}
}

// TestClampGameZoom: the flag's value is held to 0.4..1.0; NaN is 1.0.
func TestClampGameZoom(t *testing.T) {
	for _, c := range [][2]float64{{1, 1}, {0.5, 0.5}, {0.1, 0.4}, {-3, 0.4}, {2, 1}, {math.Inf(1), 1}, {math.NaN(), 1}} {
		if got := ClampGameZoom(c[0]); got != c[1] {
			t.Errorf("ClampGameZoom(%v) = %v, want %v", c[0], got, c[1])
		}
	}
}

// TestTheHarnessZoomField: a number in range is set; a number out of range, a
// non-number and NaN are refused and set nothing.
func TestTheHarnessZoomField(t *testing.T) {
	v := &fakeZoom{scale: 1}

	if err := setZoomField(v, 0.5); err != nil || v.scale != 0.5 {
		t.Fatalf("zoom 0.5: err %v, scale %v", err, v.scale)
	}

	if err := setZoomField(v, 1); err != nil || v.scale != 1 {
		t.Fatalf("zoom 1 (an int): err %v, scale %v", err, v.scale)
	}

	for _, bad := range []interface{}{0.3, 1.5, math.NaN(), "0.5", nil} {
		if err := setZoomField(v, bad); err == nil {
			t.Errorf("zoom %v was taken", bad)
		}
	}

	if v.sets != 2 {
		t.Fatalf("refused writes set the scale: %d sets, want 2", v.sets)
	}

	if err := setZoomField(nil, 0.5); err == nil {
		t.Fatal("zoom with no map renderer was taken")
	}

	g := &GameControls{}
	if err := g.HarnessSet("zoom", 0.5); err == nil {
		t.Fatal("ui's zoom with no map renderer was taken")
	}

	if err := g.HarnessSet("view_scale", 0.5); err == nil {
		t.Fatal("view_scale is read-only and was taken")
	}
}

type fakeZoom struct {
	scale float64
	sets  int
}

func (f *fakeZoom) Scale() float64     { return f.scale }
func (f *fakeZoom) SetScale(s float64) { f.scale = s; f.sets++ }
