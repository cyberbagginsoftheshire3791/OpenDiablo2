package d2asset

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpriteOverridePath(t *testing.T) {
	for in, want := range map[string]string{
		`\data\global\ui\CURSOR\ohand.DC6`:             "/data/strigoi/override/data/global/ui/cursor/ohand.png",
		`/data/global/monsters/FA/TR/fatrlitnuhth.dcc`: "/data/strigoi/override/data/global/monsters/fa/tr/fatrlitnuhth.png",
		`/data/local/UI/{LANG}/Buttons/SomeBtn.dc6`:    "/data/strigoi/override/data/local/ui/{LANG}/buttons/somebtn.png",
		`/data/local/FONT/{LANG_FONT}/font16.dc6`:      "/data/strigoi/override/data/local/font/{LANG_FONT}/font16.png",
		`/data/strigoi/creatures/wolf/idle.png`:        "",
		`/data/global/excel/monstats.txt`:              "",
	} {
		assert.Equal(t, want, SpriteOverridePath(in), in)
	}
}

// A Diablo II sprite with a Strigoi PNG at its override path is drawn from
// the PNG -- no Diablo II file, no palette -- and described as such; one
// without is still asked for from Diablo II (and, here, not found).
func TestAnOverrideStandsInForADiabloIISprite(t *testing.T) {
	dir, err := os.MkdirTemp("", "override")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sheet := image.NewNRGBA(image.Rect(0, 0, 3*20, 10))
	for x := 0; x < 60; x++ {
		sheet.Set(x, 5, color.NRGBA{R: 255, A: 255})
	}

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, sheet))

	at := filepath.Join(dir, "data", "strigoi", "override", "data", "global", "ui", "cursor")
	require.NoError(t, os.MkdirAll(at, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(at, "ohand.png"), buf.Bytes(), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(at, "ohand.png.json"),
		[]byte(`{"directions":1,"frames_per_direction":3,"frame_width":20,"frame_height":10}`), 0o600))

	am := fontAssets(t, dir)

	info, err := am.DescribeSprite(`\data\global\ui\CURSOR\ohand.DC6`, "")
	require.NoError(t, err, "the override is drawn; no palette or Diablo II file is needed")

	assert.Equal(t, "/data/strigoi/override/data/global/ui/cursor/ohand.png", info.Override)
	assert.Equal(t, 1, info.Directions)
	assert.Equal(t, 3, info.Frames)
	assert.Equal(t, [][2]int{{20, 10}, {20, 10}, {20, 10}}, info.Sizes)
	assert.Equal(t, 20, info.MaxW)

	_, err = am.DescribeSprite(`/data/global/ui/CURSOR/pentspin.DC6`, "")
	assert.Error(t, err, "control: a sprite with no override is Diablo II's, and this manager has no Diablo II")
}

// A broken drop-in is refused and Diablo II's sprite is asked for instead: the
// error, here, is the missing Diablo II file's, not the PNG's.
func TestABrokenOverrideFallsBackToDiabloII(t *testing.T) {
	dir, err := os.MkdirTemp("", "override")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	at := filepath.Join(dir, "data", "strigoi", "override", "data", "global", "ui", "cursor")
	require.NoError(t, os.MkdirAll(at, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(at, "ohand.png"), []byte("not a png"), 0o600))

	am := fontAssets(t, dir)

	_, err = am.LoadAnimation(`/data/global/ui/CURSOR/ohand.DC6`, "")
	require.Error(t, err, "there is no Diablo II here to fall back to")
	assert.NotContains(t, err.Error(), "decoding sprite image", "the PNG's failure is not what stopped it")
}
