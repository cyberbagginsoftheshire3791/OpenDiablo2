package d2asset

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	slashpath "path"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/gofont/gosmallcaps"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2font"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2font/d2fontglyph"
)

// STRIGOI'S OWN FONTS (M5.3, the census's `data/local/font` row).
//
// Every word on screen was drawn from Diablo II's fonts: a .tbl glyph table
// and a .dc6 sprite sheet per font, out of the MPQs -- twenty files a friend
// could not have. A FONT SET replaces all of them at once: a manifest naming,
// for each of Diablo II's font names (font16, fontexocet10, ...), a TrueType
// or OpenType face, a size and a colour, with a default for any name it does
// not list -- so no Diablo II font is ever asked for.
//
// A face is drawn ONCE, at load, into a glyph sheet: one cell per character,
// the cells a PNGAnimation's frames, the characters a d2font.Font's glyphs.
// So a Strigoi font is the same d2font.Font every label and button already
// uses -- the same SetColor, GetTextMetrics and RenderText -- and nothing
// that draws text knows which kind it has.
//
// Faces: "gofont:<name>" is one of the Go fonts, compiled into the game (BSD
// licensed, no file); anything else is a game-relative .ttf or .otf path,
// resolved against the manifest's folder when relative.

// FontSpec is one face at one size in one colour.
type FontSpec struct {
	Face  string  `json:"face"`
	Size  float64 `json:"size"`            // the em size, in pixels
	Color string  `json:"color,omitempty"` // "#rrggbb"; default fontDefaultColor
}

// FontSet is the manifest: a default, and faces by Diablo II font name.
type FontSet struct {
	Default FontSpec            `json:"default"`
	Fonts   map[string]FontSpec `json:"fonts,omitempty"`
}

// fontDefaultColor is the ink when a spec names none: a warm white, so the
// tints labels already set (SetColor multiplies) read as they were meant.
const fontDefaultColor = "#f0e8d8"

// fontMaxSize bounds a spec's size; a sheet is one cell per character.
const fontMaxSize = 128

// diabloFontNames are the fonts Diablo II's UI asks for
// (d2resource.Font6 ... FontRediculous, whose file is "fontridiculous").
var diabloFontNames = []string{
	"font6", "font8", "font16", "font24", "font30", "font42",
	"fontformal10", "fontformal11", "fontformal12",
	"fontexocet8", "fontexocet10", "fontridiculous",
}

// goFaces are the faces compiled in.
var goFaces = map[string][]byte{
	"regular":   goregular.TTF,
	"medium":    gomedium.TTF,
	"bold":      gobold.TTF,
	"italic":    goitalic.TTF,
	"smallcaps": gosmallcaps.TTF,
	"mono":      gomono.TTF,
}

const goFacePrefix = "gofont:"

// parseFontSet reads a font set manifest, refusing anything it would
// otherwise ignore: unknown keys, a font name Diablo II never asks for (a
// typo would silently fall to the default), something after the closing
// brace, a face that is neither compiled in nor a .ttf/.otf, a size out of
// range, a colour that is not #rrggbb.
func parseFontSet(data []byte, manifestPath string) (FontSet, error) {
	var set FontSet

	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&set); err != nil {
		return FontSet{}, fmt.Errorf("font set %s: %w", manifestPath, err)
	}

	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return FontSet{}, fmt.Errorf("font set %s has something after its closing brace", manifestPath)
	}

	dir := slashpath.Dir(manifestPath)

	check := func(name string, spec *FontSpec) error {
		if spec.Face == "" {
			return fmt.Errorf("font set %s: %s names no face", manifestPath, name)
		}

		if strings.HasPrefix(spec.Face, goFacePrefix) {
			if goFaces[strings.TrimPrefix(spec.Face, goFacePrefix)] == nil {
				return fmt.Errorf("font set %s: %s asks for %q; the compiled-in faces are gofont:bold, italic, medium, mono, regular, smallcaps",
					manifestPath, name, spec.Face)
			}
		} else {
			face := strings.ReplaceAll(spec.Face, `\`, "/")
			ext := strings.ToLower(slashpath.Ext(face))

			if ext != ".ttf" && ext != ".otf" {
				return fmt.Errorf("font set %s: %s face %q is neither gofont:<name> nor a .ttf/.otf file", manifestPath, name, spec.Face)
			}

			if !strings.HasPrefix(face, "/") {
				face = slashpath.Join(dir, face)
			}

			spec.Face = face
		}

		if spec.Size <= 0 || spec.Size > fontMaxSize {
			return fmt.Errorf("font set %s: %s size %v is not in (0, %d]", manifestPath, name, spec.Size, fontMaxSize)
		}

		if spec.Color == "" {
			spec.Color = fontDefaultColor
		}

		if _, err := parseHexColor(spec.Color); err != nil {
			return fmt.Errorf("font set %s: %s: %w", manifestPath, name, err)
		}

		return nil
	}

	if err := check("default", &set.Default); err != nil {
		return FontSet{}, err
	}

	for name, spec := range set.Fonts {
		known := false

		for _, n := range diabloFontNames {
			known = known || n == name
		}

		if !known {
			return FontSet{}, fmt.Errorf("font set %s: %q is not a font the game asks for (%s)",
				manifestPath, name, strings.Join(diabloFontNames, ", "))
		}

		if err := check(name, &spec); err != nil {
			return FontSet{}, err
		}

		set.Fonts[name] = spec
	}

	return set, nil
}

func parseHexColor(s string) (color.RGBA, error) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, fmt.Errorf("colour %q is not #rrggbb", s)
	}

	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("colour %q is not #rrggbb", s)
	}

	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, nil
}

// UseFontSet makes every later LoadFont draw from the font set at p
// (game-relative, e.g. data/strigoi/fonts/fonts.json) instead of Diablo II's
// fonts. It must be called before the first font is loaded -- fonts are
// cached -- and a refused set leaves Diablo II's fonts in use.
func (am *AssetManager) UseFontSet(p string) error {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, "/"))
	if p == "" {
		am.fontSet, am.fontSetPath = nil, ""
		return nil
	}

	p = slashpath.Clean("/" + p)

	data, err := am.LoadFile(p)
	if err != nil {
		return fmt.Errorf("loading font set %s: %w", p, err)
	}

	set, err := parseFontSet(data, p)
	if err != nil {
		return err
	}

	if am.fontSet != nil && am.fontSetPath != p {
		// Fonts are cached by name: a second set would be served the
		// first one's.
		return fmt.Errorf("font set %s refused: %s is already in use", p, am.fontSetPath)
	}

	// Every font the UI can ask for is built now, so a face that is missing,
	// corrupt or draws nothing refuses the SET -- Diablo II's fonts stand --
	// rather than failing some label later (a nil label panics its caller).
	trial := &AssetManager{Logger: am.Logger, Loader: am.Loader, fontSet: &set}

	for _, name := range diabloFontNames {
		if _, err := trial.loadStrigoiFont(name + ".tbl"); err != nil {
			return fmt.Errorf("font set %s refused: %w", p, err)
		}
	}

	am.fontSet, am.fontSetPath = &set, p

	return nil
}

// FontSetPath is the font set in use, or "" for Diablo II's fonts.
func (am *AssetManager) FontSetPath() string { return am.fontSetPath }

// fontName is the Diablo II font a table path names: ".../font16.tbl" is font16.
func fontName(tablePath string) string {
	base := slashpath.Base(strings.ReplaceAll(tablePath, `\`, "/"))

	return strings.ToLower(strings.TrimSuffix(base, slashpath.Ext(base)))
}

// loadStrigoiFont builds the font set's face for the font a table path names.
func (am *AssetManager) loadStrigoiFont(tablePath string) (*d2font.Font, error) {
	name := fontName(tablePath)

	spec, ok := am.fontSet.Fonts[name]
	if !ok {
		spec = am.fontSet.Default
	}

	var data []byte

	if strings.HasPrefix(spec.Face, goFacePrefix) {
		data = goFaces[strings.TrimPrefix(spec.Face, goFacePrefix)]
	} else {
		var err error

		if data, err = am.LoadFile(spec.Face); err != nil {
			return nil, fmt.Errorf("font %s: loading face %s: %w", name, spec.Face, err)
		}
	}

	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("font %s: face %s: %w", name, spec.Face, err)
	}

	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: spec.Size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, fmt.Errorf("font %s: face %s at %v px: %w", name, spec.Face, spec.Size, err)
	}

	defer face.Close()

	ink, _ := parseHexColor(spec.Color) // checked by parseFontSet

	return drawFont(face, ink)
}

// fontRunes are the characters a Strigoi font draws: printable ASCII, Latin-1,
// Latin Extended-A, Romanian's comma-below letters, and the typographer's
// handful (quotes, dashes, ellipsis, bullet). A face without some of them
// simply lacks those glyphs; text asking for them skips them, as it did
// with Diablo II's fonts.
func fontRunes() []rune {
	var runes []rune

	for r := rune(32); r < 127; r++ {
		runes = append(runes, r)
	}

	// Latin-1 and Latin Extended-A: Romanian's ă and â and î, and the rest
	// of Europe's.
	for r := rune(160); r < 0x180; r++ {
		runes = append(runes, r)
	}

	// Romanian's ș and ț (comma below), and the typographer's handful.
	return append(runes, 'Ș', 'ș', 'Ț', 'ț', '‘', '’', '“', '”', '–', '—', '…', '•')
}

// drawFont draws every character the face has into one row of equal cells
// and returns the font over it: each glyph a frame, as wide as its advance
// and as tall as the cell. A glyph that reaches left of its origin is drawn
// in from the cell's edge by `pad`, and the sheet's offset puts it back.
//
// The cell is as tall as the tallest glyph and as deep as the deepest, not
// the face's ascent and descent: a face may report an ascent below its
// accented capitals (Go Smallcaps does, by up to half a cap height), and a
// cell cut to it would clip them.
func drawFont(face font.Face, ink color.RGBA) (*d2font.Font, error) {
	metrics := face.Metrics()
	top, bottom := metrics.Ascent.Ceil(), metrics.Descent.Ceil()

	type drawn struct {
		r       rune
		advance int
	}

	var (
		chars []drawn
		pad   int
		right int
	)

	for _, r := range fontRunes() {
		advance, ok := face.GlyphAdvance(r)
		if !ok {
			continue
		}

		bounds, _, ok := face.GlyphBounds(r)
		if !ok {
			continue
		}

		if l := -bounds.Min.X.Floor(); l > pad {
			pad = l
		}

		if up := -bounds.Min.Y.Floor(); up > top {
			top = up
		}

		if down := bounds.Max.Y.Ceil(); down > bottom {
			bottom = down
		}

		if w := bounds.Max.X.Ceil(); w > right {
			right = w
		}

		if a := advance.Ceil(); a > right {
			right = a
		}

		chars = append(chars, drawn{r: r, advance: advance.Round()})
	}

	cellH := top + bottom

	if len(chars) == 0 || cellH <= 0 {
		return nil, errors.New("the face draws none of the characters a font needs")
	}

	cellW := pad + right

	sheet := image.NewRGBA(image.Rect(0, 0, cellW*len(chars), cellH))
	drawer := &font.Drawer{Dst: sheet, Src: image.NewUniform(ink), Face: face}
	glyphs := make(map[rune]*d2fontglyph.FontGlyph, len(chars))

	for i, c := range chars {
		drawer.Dot = fixed.P(i*cellW+pad, top)
		drawer.DrawString(string(c.r))

		glyphs[c.r] = d2fontglyph.Create(i, c.advance, cellH)
	}

	anim, err := pngAnimationFromRGBA(sheet, PNGSheet{
		Directions: 1, FramesPerDirection: len(chars),
		FrameWidth: cellW, FrameHeight: cellH,
		OffsetX: -pad,
	}, d2enum.DrawEffectNone)
	if err != nil {
		return nil, err
	}

	f := d2font.New(glyphs)
	f.SetBackground(anim)

	return f, nil
}
