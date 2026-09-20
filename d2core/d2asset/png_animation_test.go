package d2asset

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
)

// M5.1, the modern asset path. These are the first tests in this package, and
// what they pin is the arithmetic of turning a spritesheet into the engine's
// [direction][frame] model -- deliberately NOT the surfaces. Surfaces need a
// renderer, a renderer is ebiten, and d2asset has no ebiten in its import graph
// (checked: `go list -deps ./d2core/d2asset | grep -c ebiten` -> 0). Dragging one
// in to test a rectangle would cost this package the ability to be tested on a
// headless runner at all, which is the mistake docs/bugs.md BUG-13 records from
// the night before. framePixels exists as its own method for exactly this reason.

// sheetPNG builds an in-memory spritesheet whose every cell is a solid, unique
// colour, so a test can tell which cell it was handed.
//
// The colour encodes the cell: red = column (frame), green = row (direction).
// That is what makes "it cut the right cell" an assertion rather than a hope --
// an off-by-one in either axis names itself.
func sheetPNG(t *testing.T, cols, rows, cellW, cellH int) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			c := color.NRGBA{R: uint8(10 + col), G: uint8(100 + row), B: 200, A: 255}

			for y := 0; y < cellH; y++ {
				for x := 0; x < cellW; x++ {
					img.SetNRGBA(col*cellW+x, row*cellH+y, c)
				}
			}
		}
	}

	buf := &bytes.Buffer{}
	require.NoError(t, png.Encode(buf, img))

	return buf.Bytes()
}

func solidPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}

	buf := &bytes.Buffer{}
	require.NoError(t, png.Encode(buf, img))

	return buf.Bytes()
}

// A LOOSE PNG WITH NO MANIFEST IS USABLE, and this is the property that makes
// the asset path worth having: the first replacement sprite needs no tooling, no
// atlas packer and no build step. Somebody saves a file and the game draws it.
func TestPNGAnimationWithNoManifestIsOneStillFrame(t *testing.T) {
	anim, err := newPNGAnimation(solidPNG(t, 24, 40, color.NRGBA{R: 255, A: 255}), nil, d2enum.DrawEffectNone)
	require.NoError(t, err)

	assert.Equal(t, 1, anim.GetDirectionCount())
	assert.Equal(t, 1, anim.GetFrameCount())

	w, h := anim.GetCurrentFrameSize()
	assert.Equal(t, 24, w, "the whole image is the frame")
	assert.Equal(t, 40, h)
}

// THE MOST VALUABLE ASSERTION IN THIS FILE. d2dcc.Dir64ToDcc has lookup tables
// for 4, 8, 16, 32 and 64 directions and returns 0 for every other count -- so a
// sheet authored with 6 rows would load, render, and silently aim all 64 facings
// at row 0. That reads as broken art, and whoever drew the art would be the last
// person to suspect the manifest. It is refused at load with the reason.
func TestPNGSheetRefusesADirectionCountTheEngineCannotMap(t *testing.T) {
	for _, bad := range []int{2, 3, 5, 6, 7, 12, 63} {
		_, err := newPNGAnimation(
			sheetPNG(t, 2, bad, 8, 8),
			[]byte(`{"directions":`+itoa(bad)+`,"frames_per_direction":2}`),
			d2enum.DrawEffectNone)
		require.Error(t, err, "directions %d must be refused", bad)
		assert.Contains(t, err.Error(), "Dir64ToDcc",
			"the error must name the mapper, or the reason is a mystery")
	}

	// THE CONTROL. Without it this test would pass against a build that refused
	// every sheet, including the legal ones.
	for _, good := range []int{1, 4, 8, 16} {
		anim, err := newPNGAnimation(
			sheetPNG(t, 2, good, 8, 8),
			[]byte(`{"directions":`+itoa(good)+`,"frames_per_direction":2}`),
			d2enum.DrawEffectNone)
		require.NoError(t, err, "directions %d is a count the engine maps", good)
		assert.Equal(t, good, anim.GetDirectionCount())
	}
}

func TestPNGSheetDerivesFrameSizeFromTheGrid(t *testing.T) {
	anim, err := newPNGAnimation(
		sheetPNG(t, 4, 8, 16, 24),
		[]byte(`{"directions":8,"frames_per_direction":4}`),
		d2enum.DrawEffectNone)
	require.NoError(t, err)

	assert.Equal(t, 8, anim.GetDirectionCount())
	assert.Equal(t, 4, anim.GetFrameCount())

	w, h := anim.GetCurrentFrameSize()
	assert.Equal(t, 16, w, "128 px of sheet over 4 frames")
	assert.Equal(t, 24, h, "192 px of sheet over 8 directions")
}

// A manifest that claims more than the image holds is a broken pair of files,
// and the failure has to name which axis and by how much -- otherwise the author
// is left comparing two numbers neither of which is in the message.
func TestPNGSheetRefusesAGridLargerThanItsSheet(t *testing.T) {
	image64 := sheetPNG(t, 4, 4, 16, 16) // 64x64

	_, err := newPNGAnimation(image64,
		[]byte(`{"directions":4,"frames_per_direction":4,"frame_width":32,"frame_height":16}`),
		d2enum.DrawEffectNone)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wide")

	_, err = newPNGAnimation(image64,
		[]byte(`{"directions":4,"frames_per_direction":4,"frame_width":16,"frame_height":32}`),
		d2enum.DrawEffectNone)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tall")

	// The control: the same sheet with a grid that fits.
	_, err = newPNGAnimation(image64,
		[]byte(`{"directions":4,"frames_per_direction":4,"frame_width":16,"frame_height":16}`),
		d2enum.DrawEffectNone)
	require.NoError(t, err)
}

// Columns are frames and rows are directions. An off-by-one in either axis is
// the defect this catches, and it is caught by the cell's own colour rather than
// by its size, because every cell in a sheet is the same size.
func TestPNGAnimationCutsTheCellTheGridNames(t *testing.T) {
	const cols, rows, cell = 3, 4, 8

	raw, err := newPNGAnimation(
		sheetPNG(t, cols, rows, cell, cell),
		[]byte(`{"directions":4,"frames_per_direction":3}`),
		d2enum.DrawEffectNone)
	require.NoError(t, err)

	anim, ok := raw.(*PNGAnimation)
	require.True(t, ok)

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			pixels := anim.framePixels(row, col)
			require.Len(t, pixels, cell*cell*4, "a frame is tightly packed RGBA")

			// Every pixel of the cell, not only the first: a copy that ran off
			// the end of a row would leave the first pixel right and the rest
			// wrong, which is precisely the stride bug worth catching.
			for i := 0; i < len(pixels); i += 4 {
				assert.Equal(t, uint8(10+col), pixels[i+0],
					"direction %d frame %d, byte %d: red carries the column", row, col, i)
				assert.Equal(t, uint8(100+row), pixels[i+1],
					"direction %d frame %d, byte %d: green carries the row", row, col, i)
			}
		}
	}
}

// PREMULTIPLIED ALPHA, which never mattered until now. D2's index-to-RGBA path
// only ever emitted (0,0,0,0) or (r,g,b,255) -- both already premultiplied -- so
// nothing in the engine had to think about it. A PNG brings a real 8-bit alpha
// channel, and ebiten's ReplacePixels wants it premultiplied: without this, art
// with soft edges draws with bright halos.
func TestPNGAnimationPremultipliesAlpha(t *testing.T) {
	const half = 128

	raw, err := newPNGAnimation(
		solidPNG(t, 4, 4, color.NRGBA{R: 255, G: 255, B: 255, A: half}),
		nil, d2enum.DrawEffectNone)
	require.NoError(t, err)

	anim, ok := raw.(*PNGAnimation)
	require.True(t, ok)

	pixels := anim.framePixels(0, 0)
	require.NotEmpty(t, pixels)

	// White at 50% alpha premultiplies to roughly (128,128,128,128). Un-
	// premultiplied it would still be (255,255,255,128), which is the failure
	// this pins -- so the tolerance is wide enough for rounding and nowhere near
	// wide enough to admit 255.
	assert.InDelta(t, half, int(pixels[0]), 2, "red is premultiplied, not 255")
	assert.InDelta(t, half, int(pixels[1]), 2)
	assert.InDelta(t, half, int(pixels[2]), 2)
	assert.Equal(t, uint8(half), pixels[3], "alpha itself is untouched")
}

// The 0-63 facing the entities carry has to collapse onto the sheet's rows, and
// a frame index left over from the previous facing is a visible glitch.
func TestPNGAnimationMapsTheSixtyFourFacings(t *testing.T) {
	raw, err := newPNGAnimation(
		sheetPNG(t, 2, 8, 8, 8),
		[]byte(`{"directions":8,"frames_per_direction":2}`),
		d2enum.DrawEffectNone)
	require.NoError(t, err)

	anim, ok := raw.(*PNGAnimation)
	require.True(t, ok)

	require.NoError(t, anim.SetCurrentFrame(1))
	require.NoError(t, anim.SetDirection(40))

	assert.Equal(t, 0, anim.GetCurrentFrame(), "a new facing starts at its first frame")

	seen := map[int]bool{}

	for facing := 0; facing < 64; facing++ {
		require.NoError(t, anim.SetDirection(facing), "facing %d", facing)

		got := anim.GetDirection()
		require.GreaterOrEqual(t, got, 0, "facing %d mapped below zero", facing)
		require.Less(t, got, 8, "facing %d mapped outside the sheet's 8 rows", facing)

		seen[got] = true
	}

	assert.Len(t, seen, 8, "all eight rows are reachable from the 64 facings: %v", seen)

	assert.Error(t, anim.SetDirection(64), "64 is not a facing")
}

// A composite clones one animation per layer per entity, so a clone that copied
// the decoded sheet would multiply every monster's art cost by its clone count.
// The sheet is immutable after load and is shared on purpose; the playback state
// is not.
func TestPNGAnimationCloneSharesTheSheetAndNotThePlayhead(t *testing.T) {
	raw, err := newPNGAnimation(
		sheetPNG(t, 3, 4, 8, 8),
		[]byte(`{"directions":4,"frames_per_direction":3}`),
		d2enum.DrawEffectNone)
	require.NoError(t, err)

	anim, ok := raw.(*PNGAnimation)
	require.True(t, ok)

	clone, ok := anim.Clone().(*PNGAnimation)
	require.True(t, ok)

	assert.Same(t, anim.sheet, clone.sheet, "the decoded sheet is shared, not copied")
	assert.Equal(t, anim.def, clone.def)

	require.NoError(t, clone.SetCurrentFrame(2))
	assert.Equal(t, 2, clone.GetCurrentFrame())
	assert.Equal(t, 0, anim.GetCurrentFrame(), "the original's playhead did not move")
}

// The manifest's offsets are what places ground art relative to the entity, the
// way a DC6 frame's own offsets do, and origin_at_bottom is why a monster stands
// on the tile instead of hanging from it.
func TestPNGSheetCarriesOffsetsAndOrigin(t *testing.T) {
	raw, err := newPNGAnimation(
		sheetPNG(t, 1, 1, 8, 8),
		[]byte(`{"directions":1,"frames_per_direction":1,"offset_x":-4,"offset_y":-12,"origin_at_bottom":true}`),
		d2enum.DrawEffectNone)
	require.NoError(t, err)

	anim, ok := raw.(*PNGAnimation)
	require.True(t, ok)

	assert.Equal(t, -4, anim.directions[0].frames[0].offsetX)
	assert.Equal(t, -12, anim.directions[0].frames[0].offsetY)
	assert.True(t, anim.originAtBottom, "origin_at_bottom reaches the animation, not just the manifest")

	// The control: the default is the other value in both cases, so a manifest
	// that was ignored entirely would fail this pair.
	bare, err := newPNGAnimation(sheetPNG(t, 1, 1, 8, 8), nil, d2enum.DrawEffectNone)
	require.NoError(t, err)

	bareAnim, ok := bare.(*PNGAnimation)
	require.True(t, ok)

	assert.Equal(t, 0, bareAnim.directions[0].frames[0].offsetX)
	assert.False(t, bareAnim.originAtBottom)
}

func TestPNGAnimationRejectsWhatItCannotDecode(t *testing.T) {
	_, err := newPNGAnimation([]byte("this is not a png"), nil, d2enum.DrawEffectNone)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding sprite image")

	_, err = newPNGAnimation(solidPNG(t, 4, 4, color.NRGBA{A: 255}),
		[]byte(`{"directions":`), d2enum.DrawEffectNone)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest", "a broken manifest says so")

	_, err = newPNGAnimation(solidPNG(t, 4, 4, color.NRGBA{A: 255}),
		[]byte(`{"directions":1,"frames_per_direction":0}`), d2enum.DrawEffectNone)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "frames_per_direction")
}

// itoa avoids pulling strconv in for three call sites in a table.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var b []byte

	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}

	return string(b)
}
