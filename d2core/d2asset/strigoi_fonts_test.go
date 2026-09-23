package d2asset

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
)

// fontAssets is an asset manager over one directory -- no MPQ in sight.
func fontAssets(t *testing.T, dir string) *AssetManager {
	t.Helper()

	am, err := NewAssetManager(d2util.LogLevelError)
	require.NoError(t, err)
	require.NoError(t, am.AddSource(dir, types.AssetSourceFileSystem))

	return am
}

func TestParseFontSet(t *testing.T) {
	set, err := parseFontSet([]byte(`{"default":{"face":"gofont:regular","size":12},
		"fonts":{"fontexocet10":{"face":"faces/Title.TTF","size":14,"color":"#c8a860"}}}`), "/data/strigoi/fonts/fonts.json")
	require.NoError(t, err)

	assert.Equal(t, fontDefaultColor, set.Default.Color, "no colour is the default ink")
	assert.Equal(t, "/data/strigoi/fonts/faces/Title.TTF", set.Fonts["fontexocet10"].Face, "a relative face is the manifest's folder's")

	for name, c := range map[string]struct{ manifest, says string }{
		"unknown key":     {`{"default":{"face":"gofont:regular","size":12},"defualt":{}}`, "defualt"},
		"no default face": {`{"default":{"size":12}}`, "no face"},
		"a font typo":     {`{"default":{"face":"gofont:regular","size":12},"fonts":{"fnt16":{"face":"gofont:bold","size":9}}}`, "fnt16"},
		"no such go face": {`{"default":{"face":"gofont:gothic","size":12}}`, "gofont:gothic"},
		"not a font file": {`{"default":{"face":"faces/title.woff","size":12}}`, "title.woff"},
		"size zero":       {`{"default":{"face":"gofont:regular","size":0}}`, "size 0"},
		"size huge":       {`{"default":{"face":"gofont:regular","size":400}}`, "size 400"},
		"bad colour":      {`{"default":{"face":"gofont:regular","size":12,"color":"gold"}}`, "gold"},
		"trailing":        {`{"default":{"face":"gofont:regular","size":12}} {}`, "after"},
	} {
		_, err := parseFontSet([]byte(c.manifest), "/f.json")
		if assert.Error(t, err, name) {
			assert.Contains(t, err.Error(), c.says, name)
		}
	}
}

// A Strigoi font draws every printable character, each as wide as the face
// says, and measures text by those widths.
func TestAStrigoiFontDrawsAndMeasures(t *testing.T) {
	am := fontAssets(t, t.TempDir())
	am.fontSet = &FontSet{Default: FontSpec{Face: "gofont:regular", Size: 16, Color: "#ffffff"}}

	f, err := am.loadStrigoiFont("/data/local/FONT/LATIN/font16.tbl")
	require.NoError(t, err)

	for r := rune(32); r < 127; r++ {
		require.Contains(t, f.Glyphs, r, "character %q", r)
	}

	for _, r := range "ăâîșțĂÂÎȘȚ" {
		require.Contains(t, f.Glyphs, r, "Romanian's %q", r)
	}

	w, h := f.GetTextMetrics("W")
	wi, _ := f.GetTextMetrics("i")
	both, bothH := f.GetTextMetrics("iW")

	assert.Greater(t, w, wi, "W is wider than i")
	assert.Equal(t, w+wi, both, "a string is as wide as its characters")
	assert.Equal(t, h, bothH)
	assert.InDelta(t, 19, h, 3, "a 16 px face stands about 19 px tall, ascent and descent")

	two, twoH := f.GetTextMetrics("i\nWW")
	assert.Equal(t, 2*w, two, "the wider line")
	assert.Equal(t, 2*h, twoH, "two lines")

	sheet := f.Sheet().(*PNGAnimation)
	ink := func(r rune) int {
		n, px := 0, sheet.framePixels(0, f.Glyphs[r].FrameIndex())
		for i := 3; i < len(px); i += 4 {
			if px[i] > 0 {
				n++
			}
		}

		return n
	}

	assert.Greater(t, ink('A'), 20, "A is drawn")
	assert.Zero(t, ink(' '), "a space is not")
	assert.Greater(t, ink('W'), ink('.'), "W has more ink than a full stop")
}

// With a font set, every font Diablo II's UI asks for comes from it -- the
// set's default covering any it does not name -- and no Diablo II file is
// read: this asset manager has none to read. Without one, the same request
// fails for want of the .tbl.
func TestEveryDiabloFontComesFromTheSet(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fonts.json"),
		[]byte(`{"default":{"face":"gofont:regular","size":12},"fonts":{"font42":{"face":"gofont:smallcaps","size":36}}}`), 0o600))

	am := fontAssets(t, dir)

	_, err := am.LoadFont(d2resource.Font16+".tbl", d2resource.Font16+".dc6", d2resource.PaletteUnits)
	require.Error(t, err, "control: with no font set, Diablo II's font is asked for, and this manager has none")

	require.NoError(t, am.UseFontSet("fonts.json"))
	assert.Equal(t, "/fonts.json", am.FontSetPath())

	for _, base := range []string{
		d2resource.Font6, d2resource.Font8, d2resource.Font16, d2resource.Font24, d2resource.Font30, d2resource.Font42,
		d2resource.FontFormal10, d2resource.FontFormal11, d2resource.FontFormal12,
		d2resource.FontExocet8, d2resource.FontExocet10, d2resource.FontRediculous,
	} {
		f, err := am.LoadFont(base+".tbl", base+".dc6", d2resource.PaletteStatic)
		require.NoError(t, err, base)
		require.Contains(t, diabloFontNames, fontName(base+".tbl"), "%s is a name the set can list", base)

		_, h := f.GetTextMetrics("A")
		if strings.HasSuffix(base, "font42") {
			assert.Greater(t, h, 30, "font42 is the set's 36 px face")
		} else {
			assert.Less(t, h, 20, "%s is the default 12 px face", base)
		}
	}

	again, err := am.LoadFont(d2resource.Font16+".tbl", d2resource.Font16+".dc6", d2resource.PaletteUnits)
	require.NoError(t, err)

	first, _ := am.LoadFont(d2resource.Font16+".tbl", d2resource.Font16+".dc6", d2resource.PaletteStatic)
	assert.Same(t, first, again, "one font per name, whatever the palette")

	require.Error(t, am.UseFontSet("missing.json"), "a set that is not there is refused")
}

// The shipped font set parses, and names only faces that load.
func TestTheShippedFontSet(t *testing.T) {
	am := fontAssets(t, filepath.Join("..", ".."))
	require.NoError(t, am.UseFontSet("data/strigoi/fonts/fonts.json"))

	for _, name := range diabloFontNames {
		_, err := am.loadStrigoiFont("/x/" + name + ".tbl")
		require.NoError(t, err, name)
	}
}

// No glyph is clipped by its cell: each cell holds exactly the ink the face
// draws for that character on an open canvas. The smallcaps faces are the
// hard case, reporting an ascent well below their accented capitals; the
// shipped faces are the ones that matter.
func TestNoGlyphIsClipped(t *testing.T) {
	root := filepath.Join("..", "..")

	for _, spec := range []FontSpec{
		{Face: "gofont:smallcaps", Size: 36, Color: "#ffffff"},
		{Face: "gofont:smallcaps", Size: 12, Color: "#ffffff"},
		{Face: "gofont:regular", Size: 13, Color: "#ffffff"},
		{Face: "/data/strigoi/fonts/IMFeENrm28P.ttf", Size: 15, Color: "#ffffff"},
		{Face: "/data/strigoi/fonts/IMFeENsc28P.ttf", Size: 13, Color: "#ffffff"},
		{Face: "/data/strigoi/fonts/UncialAntiqua-Regular.ttf", Size: 34, Color: "#ffffff"},
	} {
		am := fontAssets(t, root)
		am.fontSet = &FontSet{Default: spec}

		f, err := am.loadStrigoiFont("x.tbl")
		require.NoError(t, err)

		data := goFaces[strings.TrimPrefix(spec.Face, goFacePrefix)]
		if data == nil {
			data, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(spec.Face)))
			require.NoError(t, err)
		}

		parsed, err := opentype.Parse(data)
		require.NoError(t, err)

		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: spec.Size, DPI: 72, Hinting: font.HintingFull})
		require.NoError(t, err)

		sheet := f.Sheet().(*PNGAnimation)
		side := int(spec.Size) * 4
		clipped := []string{}

		for r, g := range f.Glyphs {
			canvas := image.NewRGBA(image.Rect(0, 0, side, side))
			d := font.Drawer{Dst: canvas, Src: image.White, Face: face, Dot: fixed.P(side/2, side/2)}
			d.DrawString(string(r))

			if want, got := alphaSum(canvas.Pix), alphaSum(sheet.framePixels(0, g.FrameIndex())); got != want {
				clipped = append(clipped, fmt.Sprintf("%q %d/%d", r, got, want))
			}
		}

		face.Close()

		assert.Empty(t, clipped, "%s at %v px: glyphs losing ink to their cell", spec.Face, spec.Size)
	}
}

func alphaSum(px []byte) int {
	n := 0
	for i := 3; i < len(px); i += 4 {
		n += int(px[i])
	}

	return n
}

// A set whose face cannot be built is refused whole, before any label asks
// for a font -- and Diablo II's fonts stand. A second, different set is
// refused too: fonts are cached by name.
func TestAFontSetThatCannotDrawIsRefused(t *testing.T) {
	// Not t.TempDir: the loader keeps a file it has read open, and Windows
	// will not delete an open file, so the automatic cleanup fails the test.
	dir, err := os.MkdirTemp("", "fontset")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.json"),
		[]byte(`{"default":{"face":"gofont:regular","size":12},"fonts":{"font30":{"face":"missing.ttf","size":26}}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "corrupt.ttf"), []byte("not a font"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "corrupt.json"),
		[]byte(`{"default":{"face":"corrupt.ttf","size":12}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "good.json"), []byte(`{"default":{"face":"gofont:regular","size":12}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.json"), []byte(`{"default":{"face":"gofont:bold","size":12}}`), 0o600))

	am := fontAssets(t, dir)

	err = am.UseFontSet("broken.json")
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "font30")
	}

	assert.Error(t, am.UseFontSet("corrupt.json"), "a face that is not a font")
	assert.Empty(t, am.FontSetPath(), "a refused set leaves Diablo II's fonts")

	require.NoError(t, am.UseFontSet("good.json"))
	require.NoError(t, am.UseFontSet("good.json"), "the same set again is harmless")
	assert.Error(t, am.UseFontSet("other.json"), "a second set would be served the first one's cached fonts")
	assert.Equal(t, "/good.json", am.FontSetPath())
}
