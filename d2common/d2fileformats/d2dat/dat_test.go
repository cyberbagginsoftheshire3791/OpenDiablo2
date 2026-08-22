package d2dat

import (
	"testing"

	testify "github.com/stretchr/testify/assert"
)

// The palette bytes are generated in-code, so no Blizzard-derived data enters
// the repo (Constitution, Article V). A DAT palette is 256 entries of three
// bytes each, laid out B, G, R.

func sampleColor(i int) (b, g, r uint8) {
	return uint8(i), uint8(255 - i), uint8((i * 7) & 0xff)
}

func buildPalette() []byte {
	data := make([]byte, numColors*3)

	for i := 0; i < numColors; i++ {
		b, g, r := sampleColor(i)
		data[i*3+0] = b
		data[i*3+1] = g
		data[i*3+2] = r
	}

	return data
}

func TestLoad_DecodesBGRLayout(t *testing.T) {
	assert := testify.New(t)

	data := buildPalette()

	pal, err := Load(data)
	assert.NoError(err) // Load never errors on well-sized input
	assert.Equal(numColors, pal.NumColors())

	dp, ok := pal.(*DATPalette)
	assert.True(ok, "Load should return a *DATPalette")

	colors := dp.GetColors()

	for _, i := range []int{0, 1, 127, 200, 255} {
		b, g, r := sampleColor(i)
		assert.Equalf(b, colors[i].B(), "blue at index %d", i)
		assert.Equalf(g, colors[i].G(), "green at index %d", i)
		assert.Equalf(r, colors[i].R(), "red at index %d", i)
		assert.Equalf(uint8(0xff), colors[i].A(), "alpha at index %d is always opaque", i)
	}
}

func TestMarshal_RoundTrips(t *testing.T) {
	assert := testify.New(t)

	data := buildPalette()

	pal, err := Load(data)
	assert.NoError(err)

	dp := pal.(*DATPalette)
	assert.Equal(data, dp.Marshal(), "Marshal should reproduce the B,G,R bytes it was loaded from")
}

func TestLoad_PanicsOnShortInput(t *testing.T) {
	assert := testify.New(t)

	// Load does no bounds check: fewer than 256*3 bytes indexes past the end.
	assert.Panics(func() { _, _ = Load(make([]byte, 100)) })
}
