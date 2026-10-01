package d2player

import (
	"math"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
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
	// The sprite is scaled and THEN halved, as it is drawn: a 62 x 98 sprite
	// is 31 x 49 at 0.5, and its half is 15 x 24 -- halving first gives 16 x 25
	// (round(15.5), round(24.5)). The review's M13 (nc20-halve-before-scale.txt).
	l, r, tp, b = spriteHitRect(400, 300, 62, 98, 0.5)
	wl, wr, wt, wb = 400-15-hoverLabelOuterPad, 400+15+hoverLabelOuterPad, 300-24-hoverLabelOuterPad, 300+24-hoverLabelOuterPad

	if l != wl || r != wr || tp != wt || b != wb {
		t.Fatalf("at 0.5 a 62 x 98 sprite's rect is (%d,%d,%d,%d); scaled then halved it is (%d,%d,%d,%d)",
			l, r, tp, b, wl, wr, wt, wb)
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
	keepGameZoom(t)

	v := &fakeZoom{scale: 1}

	var acc float64

	want := []float64{0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.4, 0.4}
	for i, w := range want {
		if !zoomByWheel(v, &acc, -1) {
			t.Fatalf("notch %d out did nothing", i+1)
		}

		if v.scale != w {
			t.Fatalf("notch %d out: %v, want %v", i+1, v.scale, w)
		}
	}

	back := []float64{0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.0, 1.0}
	for i, w := range back {
		zoomByWheel(v, &acc, 1)

		if v.scale != w {
			t.Fatalf("notch %d back: %v, want %v", i+1, v.scale, w)
		}
	}

	if zoomByWheel(v, &acc, 0) || v.sets != 16 {
		t.Fatalf("a roll with no y zoomed (sets %d, want 16)", v.sets)
	}

	if got := nextGameZoom(1, 3); got != 1 {
		t.Fatalf("a notch in from 1.0 went to %v; the game does not zoom in past its shipped view", got)
	}
}

// TestATouchpadScrollIsOneNotchPerWholeUnit (the zoom review, B1): a touchpad
// reports fractions, many events a second. Ten events of 0.1 are one notch --
// nine do nothing, the tenth steps once -- and a mouse's notch of 1.0 is one
// notch; three units at once are three.
//
// Negative control (1 Oct 2026): make wheelNotches step once per event, by the
// sign of dy (the first commit's behaviour), and this fails: "event 1 of ten
// 0.1 scrolls zoomed to 0.9; a tenth of a notch must wait"
// (nc17-notch-per-event.txt).
func TestATouchpadScrollIsOneNotchPerWholeUnit(t *testing.T) {
	keepGameZoom(t)

	v := &fakeZoom{scale: 1}

	var acc float64

	for i := 1; i <= 10; i++ {
		zoomed := zoomByWheel(v, &acc, -0.1)

		if i < 10 && (zoomed || v.scale != 1) {
			t.Fatalf("event %d of ten 0.1 scrolls zoomed to %v; a tenth of a notch must wait", i, v.scale)
		}

		if i == 10 && (!zoomed || v.scale != 0.9) {
			t.Fatalf("the tenth 0.1 scroll left the zoom at %v; ten tenths are one notch, 0.9", v.scale)
		}
	}

	if acc != 0 {
		t.Fatalf("after one whole notch %v of a notch is left over; it must be 0", acc)
	}

	if !zoomByWheel(v, &acc, -1) || v.scale != 0.8 {
		t.Fatalf("a mouse notch of 1.0 zoomed to %v, want 0.8", v.scale)
	}

	if !zoomByWheel(v, &acc, 3) || v.scale != 1.0 {
		t.Fatalf("three units back zoomed to %v, want 1.0", v.scale)
	}

	acc = 0
	zoomByWheel(v, &acc, -0.6)
	zoomByWheel(v, &acc, 0.3)

	if v.scale != 1.0 || math.Abs(acc+0.3) > 1e-9 {
		t.Fatalf("0.6 out then 0.3 back: zoom %v, %v pending; want 1.0 and -0.3", v.scale, acc)
	}
}

// TestTheWheelSetsTheZoomTheNextGameStartsAt (the zoom review, C2): a zoom the
// player chose survives a death's load or a new game, so the wheel and the
// harness field both set the process's starting zoom; it is never written to
// the world.
//
// Negative control (1 Oct 2026): drop SetGameZoom from setViewZoom and this
// fails: "after the wheel zoomed to 0.9 a new game would start at 1"
// (nc18-wheel-not-process-wide.txt).
func TestTheWheelSetsTheZoomTheNextGameStartsAt(t *testing.T) {
	keepGameZoom(t)
	SetGameZoom(1)

	v := &fakeZoom{scale: 1}

	var acc float64

	zoomByWheel(v, &acc, -1)

	if GameZoom() != 0.9 {
		t.Fatalf("after the wheel zoomed to %v a new game would start at %v", v.scale, GameZoom())
	}

	if err := setZoomField(v, 0.6); err != nil || GameZoom() != 0.6 {
		t.Fatalf("after the harness zoomed to 0.6 (err %v) a new game would start at %v", err, GameZoom())
	}

	SetGameZoom(0.1)

	if GameZoom() != 0.4 {
		t.Fatalf("SetGameZoom(0.1) starts games at %v; the game's range stops at 0.4", GameZoom())
	}
}

// TestTheWheelIsRefusedWhereAClickIs (the zoom review, C1): nothing zooms
// under the loadout choice, over the kit or the talent panel, or with the
// skill-select menu open -- the click path's own guards -- nor under the
// earlier modals; with none of them up it zooms.
//
// Negative control (1 Oct 2026): drop the loadout, talent and skill-select
// cases from wheelBlocked and this fails: "the wheel zoomed under the loadout
// choice" (nc19-wheel-guards.txt).
func TestTheWheelIsRefusedWhereAClickIs(t *testing.T) {
	keepGameZoom(t)

	newControls := func() *GameControls {
		return &GameControls{hud: &HUD{}, mapRenderer: d2maprenderer.NewViewOnlyMapRenderer(0, 0)}
	}

	wheel := func(g *GameControls, x, y int) bool {
		return g.OnMouseWheel(fakeWheel{x: x, y: y, dy: -1})
	}

	if g := newControls(); !wheel(g, 400, 300) || g.mapRenderer.Scale() != 0.9 {
		t.Fatalf("the control: with nothing open the wheel did not zoom (scale %v)", g.mapRenderer.Scale())
	}

	cases := map[string]func(g *GameControls){
		"under the loadout choice": func(g *GameControls) { g.kitHolder = fakeKit{choosing: true} },
		"over the open talent panel": func(g *GameControls) {
			g.hud.talents = &talentOverlay{open: true}
		},
		"with the skill-select menu open": func(g *GameControls) {
			g.hud.skillSelectMenu = &SkillSelectMenu{LeftPanel: &SkillPanel{isOpen: true}, RightPanel: &SkillPanel{}}
		},
		"in a talk": func(g *GameControls) { g.hud.talk = &talkOverlay{open: true} },
	}

	for name, open := range cases {
		g := newControls()
		open(g)

		if wheel(g, talentPanelX+5, talentPanelY+5) || g.mapRenderer.Scale() != 1 {
			t.Errorf("the wheel zoomed %s (scale %v)", name, g.mapRenderer.Scale())
		}
	}
}

func keepGameZoom(t *testing.T) {
	t.Helper()

	was := GameZoom()
	t.Cleanup(func() { SetGameZoom(was) })
}

type fakeWheel struct {
	d2interface.MouseWheelEvent
	x, y int
	dy   float64
}

func (w fakeWheel) X() int           { return w.x }
func (w fakeWheel) Y() int           { return w.y }
func (w fakeWheel) ScrollY() float64 { return w.dy }
func (w fakeWheel) ScrollX() float64 { return 0 }

type fakeKit struct {
	KitHolder
	choosing bool
}

func (k fakeKit) ChoosingLoadout() bool { return k.choosing }

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
	keepGameZoom(t)

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

// TestViewScaleIsInTheDigestsProcessPart: the zoom is this process's view, so
// the digest's world part -- which a resumed game must reproduce -- leaves it
// out and the process part carries it.
//
// Negative control (1 Oct 2026, the review's M7): leave view_scale off the
// list splitUIDigest deletes from the world part and this fails: "view_scale
// is in the digest's world part" (nc26-view-scale-in-world.txt).
func TestViewScaleIsInTheDigestsProcessPart(t *testing.T) {
	world, process := splitUIDigest(map[string]interface{}{"view_scale": 0.5, "free_cam": false})

	if _, ok := world["view_scale"]; ok {
		t.Fatal("view_scale is in the digest's world part; a resumed game starts at its own zoom")
	}

	if process["view_scale"] != 0.5 {
		t.Fatalf("the digest's process part has view_scale %v, want 0.5", process["view_scale"])
	}

	if _, ok := world["free_cam"]; !ok {
		t.Fatal("the control: free_cam, world state, left the world part")
	}
}
