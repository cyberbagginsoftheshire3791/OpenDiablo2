package d2maprenderer

import (
	"math"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// The viewport is the one piece of this burst every editor click goes through,
// so these tests are built around two things: that scale 1.0 is the arithmetic
// the shipped game has always run, bit for bit (the control), and that
// ScreenToWorld keeps inverting WorldToScreen to the pixel at every other scale
// (the capability). "To the pixel" rather than exactly, because the measurement
// said so -- see TestScreenToWorldInvertsWorldToScreenAtEveryScale.

const epsilon = 1e-9

// newTestViewport builds the viewport the game builds -- 800x600 at the origin
// -- with a camera at the given orthogonal position. The camera's position is in
// ORTHOGONAL pixels, not sub tiles: CreateMapRenderer feeds it WorldToOrtho
// (renderer.go:98) even though d2vector.Position's own doc comment says sub
// tiles.
func newTestViewport(camOrthoX, camOrthoY float64) *Viewport {
	v := NewViewport(0, 0, 800, 600)

	position := d2vector.NewPosition(camOrthoX, camOrthoY)
	camera := &Camera{position: &position}
	v.SetCamera(camera)

	return v
}

// moveTestCamera puts the camera at an orthogonal position, the way ZoomAt does.
func moveTestCamera(v *Viewport, camOrthoX, camOrthoY float64) {
	position := d2vector.NewPosition(camOrthoX, camOrthoY)
	v.camera.MoveTo(&position)
}

// legacyOrthoToScreenF is OrthoToScreenF exactly as it read before the scale
// went in, and legacyScreenToOrtho the same for ScreenToOrtho. They are the
// control for the shipped game: the unzoomed branch must be this arithmetic,
// not arithmetic that agrees with it to within a rounding error, because
// OrthoToScreen floors and one bit low is one pixel out.
func legacyOrthoToScreenF(v *Viewport, x, y float64) (screenX, screenY float64) {
	camOrthoX, camOrthoY := v.getCameraOffset()

	return x - camOrthoX + float64(v.screenRect.Left), y - camOrthoY + float64(v.screenRect.Top)
}

func legacyScreenToOrtho(v *Viewport, x, y int) (orthoX, orthoY float64) {
	camX, camY := v.getCameraOffset()

	return float64(x) + camX - float64(v.screenRect.Left), float64(y) + camY - float64(v.screenRect.Top)
}

// scaleOneCameras are awkward on purpose: fractions that do not survive being
// added to 400 and taken off again are exactly what the unzoomed fast path
// exists to protect.
//
//nolint:gochecknoglobals // test table
var scaleOneCameras = [][2]float64{
	{0, 0},
	{1234, 567},
	{-880.5, 320.25},
	{0.1, -0.1},
	{40.30000000000001, 2400.7},
}

func TestScaleOneIsTheUnzoomedTransformExactly(t *testing.T) {
	worlds := [][2]float64{{0, 0}, {1, 0}, {0, 1}, {12.5, 7.5}, {47.9, 3.1}, {-6.25, 9.375}}

	for _, cam := range scaleOneCameras {
		v := newTestViewport(cam[0], cam[1])

		if got := v.Scale(); got != 1.0 {
			t.Fatalf("a fresh viewport reports scale %v, want 1.0", got)
		}

		for _, w := range worlds {
			orthoX, orthoY := v.WorldToOrtho(w[0], w[1])

			wantX, wantY := legacyOrthoToScreenF(v, orthoX, orthoY)
			gotX, gotY := v.OrthoToScreenF(orthoX, orthoY)

			if gotX != wantX || gotY != wantY {
				t.Errorf("cam %v world %v: OrthoToScreenF = (%v, %v), the unzoomed transform gives (%v, %v)",
					cam, w, gotX, gotY, wantX, wantY)
			}

			gotIntX, gotIntY := v.OrthoToScreen(orthoX, orthoY)
			if gotIntX != int(math.Floor(wantX)) || gotIntY != int(math.Floor(wantY)) {
				t.Errorf("cam %v world %v: OrthoToScreen = (%d, %d), want (%d, %d)",
					cam, w, gotIntX, gotIntY, int(math.Floor(wantX)), int(math.Floor(wantY)))
			}
		}

		for _, p := range [][2]int{{0, 0}, {400, 300}, {799, 599}, {-200, 1050}, {13, 7}} {
			wantX, wantY := legacyScreenToOrtho(v, p[0], p[1])
			gotX, gotY := v.ScreenToOrtho(p[0], p[1])

			if gotX != wantX || gotY != wantY {
				t.Errorf("cam %v pixel %v: ScreenToOrtho = (%v, %v), the unzoomed transform gives (%v, %v)",
					cam, p, gotX, gotY, wantX, wantY)
			}
		}
	}
}

// TestOrthoToWorldInvertsWorldToOrtho is the guard on the two divisors that used
// to be the literals 80 and 40. It binds them to tileWidth and tileHeight: change
// one without the other and the round trip stops closing.
func TestOrthoToWorldInvertsWorldToOrtho(t *testing.T) {
	v := newTestViewport(0, 0)

	if gotX, gotY := v.OrthoToWorld(tileWidth, tileHeight); gotX != 1 || gotY != 0 {
		t.Errorf("OrthoToWorld(tileWidth, tileHeight) = (%v, %v), want (1, 0)", gotX, gotY)
	}

	for _, w := range [][2]float64{{0, 0}, {1, 0}, {0, 1}, {12.5, 7.5}, {47, 3}, {-6.25, 9.375}} {
		gotX, gotY := v.OrthoToWorld(v.WorldToOrtho(w[0], w[1]))

		if math.Abs(gotX-w[0]) > epsilon || math.Abs(gotY-w[1]) > epsilon {
			t.Errorf("world %v round tripped through ortho to (%v, %v)", w, gotX, gotY)
		}
	}
}

// scales covers the band SetScale allows, with 1.0 first as the control: a
// regression in the unzoomed game shows up in the same table as the zoomed one.
//
//nolint:gochecknoglobals // test table
var scales = []float64{1.0, 0.0625, 0.25, 0.5, 0.75, 1.3, 2.0, 3.0, 4.0, 16.0}

// TestScreenToOrthoInvertsOrthoToScreenFAtEveryScale is the inner half of the
// round trip every click depends on.
func TestScreenToOrthoInvertsOrthoToScreenFAtEveryScale(t *testing.T) {
	for _, scale := range scales {
		v := newTestViewport(-1240, 3160)
		v.SetScale(scale)

		if got := v.Scale(); got != scale {
			t.Fatalf("SetScale(%v) then Scale() = %v", scale, got)
		}

		for _, p := range [][2]int{{0, 0}, {400, 300}, {799, 599}, {-200, 1050}, {13, 7}} {
			orthoX, orthoY := v.ScreenToOrtho(p[0], p[1])
			backX, backY := v.OrthoToScreenF(orthoX, orthoY)

			if math.Abs(backX-float64(p[0])) > epsilon || math.Abs(backY-float64(p[1])) > epsilon {
				t.Errorf("scale %v: pixel %v -> ortho (%v, %v) -> pixel (%v, %v)",
					scale, p, orthoX, orthoY, backX, backY)
			}
		}
	}
}

// TestScreenToWorldInvertsWorldToScreenAtEveryScale is the headline property,
// stated as it MEASURES rather than as one would like it to read.
//
// A pixel turned into a world position and back comes out as the pixel it started
// as, or one pixel lower, and never higher. That is not a tolerance chosen to make
// a test pass: it was measured over 4 camera positions x 12 scales x ~26,000
// pixels each, and the interval is exactly [-1, 0] at every one of those scales
// INCLUDING 1.0. The loss is WorldToScreen's math.Floor together with the
// world<->ortho division by 80 and 40, both of which predate the scale, so the
// pre-scale engine never round tripped exactly either. What this test defends is
// that introducing the scale did not widen the loss: if the zoomed path ever
// drifts by two pixels, or the unzoomed one starts rounding up, this goes red.
//
// The exactness that IS available lives one transform in, between screen and
// orthogonal space, and TestScreenToOrthoInvertsOrthoToScreenFAtEveryScale holds
// it to 1e-9.
func TestScreenToWorldInvertsWorldToScreenAtEveryScale(t *testing.T) {
	pixels := [][2]int{{0, 0}, {1, 1}, {400, 300}, {799, 599}, {123, 457}, {-200, 1050}, {640, 17}}

	for _, scale := range scales {
		for _, cam := range [][2]float64{{0, 0}, {-1240, 3160}, {77.5, -19.25}} {
			v := newTestViewport(cam[0], cam[1])
			v.SetScale(scale)

			for _, p := range pixels {
				worldX, worldY := v.ScreenToWorld(p[0], p[1])
				backX, backY := v.WorldToScreen(worldX, worldY)

				if backX-p[0] > 0 || backX-p[0] < -1 || backY-p[1] > 0 || backY-p[1] < -1 {
					t.Errorf("scale %v cam %v: pixel %v -> world (%v, %v) -> pixel (%d, %d); the round trip is measured to land on the pixel or one below it, never elsewhere",
						scale, cam, p, worldX, worldY, backX, backY)
				}
			}
		}
	}
}

// TestWorldToScreenRoundTripsBackToTheSameWorldPoint is the other direction.
// WorldToScreen floors to a whole pixel, so the world point cannot come back
// untouched -- it comes back within the pixel it was drawn in, and how much world
// one pixel is worth is itself a function of the scale. Zoomed out, one pixel
// covers more world, and this test says so rather than hiding it in a fixed
// tolerance.
func TestWorldToScreenRoundTripsBackToTheSameWorldPoint(t *testing.T) {
	for _, scale := range scales {
		v := newTestViewport(-1240, 3160)
		v.SetScale(scale)

		// One screen pixel is 1/scale orthogonal pixels; the isometric transform
		// turns that into 1/(scale*tileHeight) of a tile at worst, and the floor
		// can lose one whole pixel in each of x and y.
		tolerance := two / (scale * tileHeight)

		for _, w := range [][2]float64{{0, 0}, {12.5, 7.5}, {31.25, 2.75}, {-6.5, 9.5}} {
			screenX, screenY := v.WorldToScreen(w[0], w[1])
			backX, backY := v.ScreenToWorld(screenX, screenY)

			if math.Abs(backX-w[0]) > tolerance || math.Abs(backY-w[1]) > tolerance {
				t.Errorf("scale %v: world %v -> pixel (%d, %d) -> world (%v, %v), off by more than %v",
					scale, w, screenX, screenY, backX, backY, tolerance)
			}
		}
	}
}

// TestZoomAtScreenHoldsTheWorldPointUnderTheCursor is what "zoom around the
// cursor" means, asserted as the property rather than as a picture: the tile
// under a pixel before the zoom is the tile under that same pixel after it, and
// the world point itself has not moved either.
func TestZoomAtScreenHoldsTheWorldPointUnderTheCursor(t *testing.T) {
	// A tile well off the centre of the screen, because a zoom anchored on the
	// middle of the screen would pass a test taken at the middle of the screen.
	const tileX, tileY = 12, 7

	for _, from := range scales {
		for _, to := range scales {
			v := newTestViewport(1600, 800)
			v.SetScale(from)

			// The centre of the tile, so the floor below is not sitting on an edge.
			pixelX, pixelY := v.WorldToScreen(tileX+0.5, tileY+0.5)

			beforeX, beforeY := v.ScreenToWorld(pixelX, pixelY)
			if int(math.Floor(beforeX)) != tileX || int(math.Floor(beforeY)) != tileY {
				t.Fatalf("scale %v: the test's own pixel is over tile (%d, %d), not (%d, %d)",
					from, int(math.Floor(beforeX)), int(math.Floor(beforeY)), tileX, tileY)
			}

			camX, camY := v.ZoomAtScreen(pixelX, pixelY, to)
			v.SetScale(to)
			moveTestCamera(v, camX, camY)

			afterX, afterY := v.ScreenToWorld(pixelX, pixelY)

			if int(math.Floor(afterX)) != tileX || int(math.Floor(afterY)) != tileY {
				t.Errorf("zoom %v -> %v at pixel (%d, %d): tile under the cursor became (%d, %d), want (%d, %d)",
					from, to, pixelX, pixelY,
					int(math.Floor(afterX)), int(math.Floor(afterY)), tileX, tileY)
			}

			if math.Abs(afterX-beforeX) > epsilon || math.Abs(afterY-beforeY) > epsilon {
				t.Errorf("zoom %v -> %v at pixel (%d, %d): world point moved from (%v, %v) to (%v, %v)",
					from, to, pixelX, pixelY, beforeX, beforeY, afterX, afterY)
			}
		}
	}
}

// TestZoomAtScreenAtTheSameScaleDoesNotMoveTheCamera: zero is a scale change too,
// and a wheel that reported no movement must not nudge the view.
func TestZoomAtScreenAtTheSameScaleDoesNotMoveTheCamera(t *testing.T) {
	v := newTestViewport(1600, 800)
	v.SetScale(0.5)

	camX, camY := v.ZoomAtScreen(137, 421, 0.5)

	if camX != 1600 || camY != 800 {
		t.Errorf("ZoomAtScreen with no change of scale moved the camera to (%v, %v), want (1600, 800)", camX, camY)
	}
}

func TestSetScaleClampsAndAZeroValueViewportIsUnzoomed(t *testing.T) {
	var zero Viewport

	if got := zero.Scale(); got != 1.0 {
		t.Errorf("a zero-value Viewport reports scale %v; it must read as unzoomed so that nothing built without NewViewport changed", got)
	}

	v := newTestViewport(0, 0)

	v.SetScale(0)

	if got := v.Scale(); got != minScale {
		t.Errorf("SetScale(0) gave %v, want it clamped to %v -- scale 0 divides by zero in ScreenToOrtho", got, minScale)
	}

	v.SetScale(-4)

	if got := v.Scale(); got != minScale {
		t.Errorf("SetScale(-4) gave %v, want it clamped to %v", got, minScale)
	}

	v.SetScale(1e6)

	if got := v.Scale(); got != maxScale {
		t.Errorf("SetScale(1e6) gave %v, want it clamped to %v", got, maxScale)
	}
}

// TestCullProbesAtScaleOneAreTheOldLiterals pins the shipped game's cull. The
// three numbers used to be written into MapRenderer.Render as 400, -200 and 1050,
// and if the derivation ever stops producing them the game's culling has changed.
func TestCullProbesAtScaleOneAreTheOldLiterals(t *testing.T) {
	v := newTestViewport(0, 0)

	x, topY, bottomY := v.cullProbes()

	if x != 400 || topY != -200 || bottomY != 1050 {
		t.Errorf("cullProbes() = (%d, %d, %d), want (400, -200, 1050)", x, topY, bottomY)
	}

	// The aligned half-viewports must not narrow them: the renderer still draws
	// across the whole screen when the viewport is pushed left or right.
	v.toLeft()

	if x2, t2, b2 := v.cullProbes(); x2 != x || t2 != topY || b2 != bottomY {
		t.Errorf("after toLeft, cullProbes() = (%d, %d, %d), want it unchanged", x2, t2, b2)
	}

	v.toRight()

	if x2, t2, b2 := v.cullProbes(); x2 != x || t2 != topY || b2 != bottomY {
		t.Errorf("after toRight, cullProbes() = (%d, %d, %d), want it unchanged", x2, t2, b2)
	}
}

// TestCullRangeAtScaleOneIsTheOldArithmetic is the control that protects the
// shipped game's culling: the range must equal what the two literal probes,
// the floors, the ceilings and the clamps produced before any of this moved.
func TestCullRangeAtScaleOneIsTheOldArithmetic(t *testing.T) {
	const mapWidth, mapHeight = 48, 48

	for _, cam := range scaleOneCameras {
		v := newTestViewport(cam[0], cam[1])

		stxf, styf := v.ScreenToWorld(400, -200)
		etxf, etyf := v.ScreenToWorld(400, 1050)

		wantStartX := int(math.Max(0, math.Floor(stxf)))
		wantStartY := int(math.Max(0, math.Floor(styf)))
		wantEndX := int(math.Min(float64(mapWidth), math.Ceil(etxf)))
		wantEndY := int(math.Min(float64(mapHeight), math.Ceil(etyf)))

		startX, startY, endX, endY := v.cullRange(mapWidth, mapHeight)

		if startX != wantStartX || startY != wantStartY || endX != wantEndX || endY != wantEndY {
			t.Errorf("cam %v: cullRange = (%d, %d, %d, %d), the old arithmetic gives (%d, %d, %d, %d)",
				cam, startX, startY, endX, endY, wantStartX, wantStartY, wantEndX, wantEndY)
		}
	}
}

// TestAuthoredCullRowsFollowTheScale. The tall-structure allowance was a bare 8
// tile rows. Since the 28 Sep review the ART is scaled with the view, so the
// count is worked in orthogonal pixels (see authoredCullRows): zoomed out the
// bottom probe already reaches past the tallest strip and no extra row is
// needed; zoomed in, more are. Scale 1.0 must still be 8, or the shipped game's
// wall pass has changed.
//
// Negative control (28 Sep 2026): restore the old ceil(7.95/scale) and this
// fails -- "at scale 2 authoredCullRows() = 4, want 14" -- and so does
// TestAuthoredCullRowsReachTheTallestStrip below.
func TestAuthoredCullRowsFollowTheScale(t *testing.T) {
	cases := []struct {
		scale float64
		want  int
	}{
		{1.0, 8},  // the control: the constant this replaced
		{0.6, 1},  // (768 - 750)/40 = 0.45, the last scale with a row to add
		{0.5, 0},  // the probe is 900 ortho pixels down: past any strip
		{0.25, 0}, // and further still
		{2.0, 14}, // (768 - 225)/40 = 13.6
		{4.0, 17}, // (768 - 112.5)/40 = 16.4
	}

	for _, c := range cases {
		v := newTestViewport(0, 0)
		v.SetScale(c.scale)

		if got := v.authoredCullRows(); got != c.want {
			t.Errorf("at scale %v authoredCullRows() = %d, want %d", c.scale, got, c.want)
		}
	}

	var zero Viewport

	if got := zero.authoredCullRows(); got != 8 {
		t.Errorf("a zero-value Viewport gives %d authored cull rows, want the unzoomed 8", got)
	}
}

// TestAuthoredCullRowsReachTheTallestStrip is the property the table above
// encodes, measured rather than restated: in SCREEN pixels, the rows the wall
// pass adds plus the bottom probe's margin must reach at least as far as the
// tallest authored strip stands above its anchor -- which, with the art scaled,
// is maxAuthoredArtHeight*scale. A shortfall is a roof that vanishes when its
// house's front tiles are just below the screen.
func TestAuthoredCullRowsReachTheTallestStrip(t *testing.T) {
	for _, scale := range []float64{minScale, 0.08, 0.25, 0.5, 0.6, 0.75, 1, 1.5, 2, 3, 4, 8, maxScale} {
		v := newTestViewport(0, 0)
		v.SetScale(scale)

		reach := float64(v.authoredCullRows())*tileHeight*scale + cullMarginBottom
		need := maxAuthoredArtHeight * scale

		if reach < need {
			t.Errorf("at scale %v the wall pass reaches %.1f screen px below the screen, the tallest strip stands %.1f",
				scale, reach, need)
		}
	}
}

// TestCullRangeCoversEveryTileOnScreenAtEveryScale is the measurement, not a
// restatement of the design: it walks the four corners of the screen, works out
// the smallest tile rectangle that contains them, and checks the cull range holds
// it. A tile whose anchor is on screen and outside the cull range is a tile that
// vanishes.
//
// The map is made large and the camera put well inside it so the 0 and mapSize
// clamps cannot do the covering for us.
func TestCullRangeCoversEveryTileOnScreenAtEveryScale(t *testing.T) {
	const mapWidth, mapHeight = 1024, 1024

	for _, scale := range scales {
		v := newTestViewport(0, 40*1024)
		v.SetScale(scale)

		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)

		for _, corner := range [][2]int{{0, 0}, {799, 0}, {0, 599}, {799, 599}} {
			wx, wy := v.ScreenToWorld(corner[0], corner[1])
			minX, minY = math.Min(minX, wx), math.Min(minY, wy)
			maxX, maxY = math.Max(maxX, wx), math.Max(maxY, wy)
		}

		startX, startY, endX, endY := v.cullRange(mapWidth, mapHeight)

		if float64(startX) > minX || float64(startY) > minY {
			t.Errorf("scale %v: cull starts at (%d, %d) but the screen reaches back to (%v, %v)",
				scale, startX, startY, minX, minY)
		}

		if float64(endX) < maxX || float64(endY) < maxY {
			t.Errorf("scale %v: cull ends at (%d, %d) but the screen reaches out to (%v, %v)",
				scale, endX, endY, maxX, maxY)
		}
	}
}

// TestMapRendererZoomAtHoldsTheTileUnderTheCursor is the same property through the
// door the editor actually knocks on. MapRenderer.ZoomAt is one call -- read the
// wheel event's position and amount, call this -- and it has to do the scale and
// the camera move together, because doing either one alone slides the map.
func TestMapRendererZoomAtHoldsTheTileUnderTheCursor(t *testing.T) {
	const tileX, tileY = 30, 11

	for _, to := range []float64{0.25, 0.5, 0.9, 1.0, 1.1, 2.0, 4.0} {
		v := newTestViewport(2400, 1600)
		mr := &MapRenderer{viewport: v, Camera: *v.camera}
		v.SetCamera(&mr.Camera)

		if got := mr.Scale(); got != 1.0 {
			t.Fatalf("a fresh MapRenderer reports scale %v, want 1.0", got)
		}

		pixelX, pixelY := mr.WorldToScreen(tileX+0.5, tileY+0.5)

		beforeX, beforeY := mr.ScreenToWorld(pixelX, pixelY)
		if int(math.Floor(beforeX)) != tileX || int(math.Floor(beforeY)) != tileY {
			t.Fatalf("the test's own pixel is over tile (%d, %d), not (%d, %d)",
				int(math.Floor(beforeX)), int(math.Floor(beforeY)), tileX, tileY)
		}

		mr.ZoomAt(pixelX, pixelY, to)

		if got := mr.Scale(); got != to {
			t.Errorf("after ZoomAt(%v) the scale is %v", to, got)
		}

		afterX, afterY := mr.ScreenToWorld(pixelX, pixelY)

		if int(math.Floor(afterX)) != tileX || int(math.Floor(afterY)) != tileY {
			t.Errorf("ZoomAt to %v at pixel (%d, %d): tile under the cursor became (%d, %d), want (%d, %d)",
				to, pixelX, pixelY, int(math.Floor(afterX)), int(math.Floor(afterY)), tileX, tileY)
		}

		if math.Abs(afterX-beforeX) > epsilon || math.Abs(afterY-beforeY) > epsilon {
			t.Errorf("ZoomAt to %v: world point under the cursor moved from (%v, %v) to (%v, %v)",
				to, beforeX, beforeY, afterX, afterY)
		}
	}
}

// TestMapRendererSetScaleLeavesTheCameraAlone separates the two verbs: SetScale is
// the plain zoom (the middle of the screen holds still) and ZoomAt is the one that
// moves the camera. A SetScale that also moved the camera would make a reset
// button jump the view.
func TestMapRendererSetScaleLeavesTheCameraAlone(t *testing.T) {
	v := newTestViewport(2400, 1600)
	mr := &MapRenderer{viewport: v, Camera: *v.camera}
	v.SetCamera(&mr.Camera)

	mr.SetScale(0.5)

	if got := mr.Camera.GetPosition().X(); got != 2400 {
		t.Errorf("SetScale moved the camera to x=%v, want 2400", got)
	}

	// The middle of the screen is where the camera is, at any scale.
	worldX, worldY := mr.ScreenToWorld(400, 300)
	orthoX, orthoY := mr.WorldToOrtho(worldX, worldY)

	if math.Abs(orthoX-2400) > epsilon || math.Abs(orthoY-1600) > epsilon {
		t.Errorf("at scale 0.5 the middle of the screen is ortho (%v, %v), want the camera's (2400, 1600)", orthoX, orthoY)
	}
}
