package d2mappalette

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// This file is the reason [CheckArt] is allowed to exist.
//
// CheckArt is a transcription of rules that live somewhere else, and a
// transcription is a claim. Every test here puts the claim in front of the real
// d2maptiled.Parse: it builds a map that places one piece of art on one layer,
// hands it to the loader, and asserts that the loader accepts it exactly when
// this package says it is placeable. Nothing about the engine is taken on trust
// or from a comment, including the two height bounds this package has to copy
// because the loader keeps them unexported.

// synthPNG returns a w x h PNG.
func synthPNG(t *testing.T, w, h int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	c := color.RGBA{R: 90, G: 60, B: 30, A: 255}

	for i := 0; i < w*h; i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = c.R, c.G, c.B, c.A
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// synthSide is the test map's side. A 16x16 footprint standing on its bottom
// corner at tile 16,16 covers rect(0,0,16,16), so 20 leaves room for the art and
// for a player_start out of its way.
const synthSide = 20

// placeOn builds a map that puts one test tile on one layer and hands it to the
// real loader. fp is the footprint the tile DECLARES: the zero Point emits no
// footprint property at all.
//
// It returns the loader's error, or nil when the loader accepted the map.
func placeOn(t *testing.T, layer d2maptiled.Layer, fp image.Point, w, h int) error {
	t.Helper()

	floor := make([]any, synthSide*synthSide)
	walls := make([]any, synthSide*synthSide)

	for i := range floor {
		floor[i] = 1
	}

	objects := []any{
		// tile 18.5,18.5: grass, unblocked, and clear of a 16x16 footprint
		// anchored at 16,16.
		map[string]any{"id": 1, "type": "player_start", "x": 80.0 * 18.5, "y": 80.0 * 18.5},
	}

	switch layer {
	case d2maptiled.LayerFloor:
		floor[0] = 2
	case d2maptiled.LayerWall:
		walls[0] = 2
	case d2maptiled.LayerStructure:
		objects = append(objects, map[string]any{
			"id": 2, "type": "structure", "gid": 2, "x": 80.0 * 16, "y": 80.0 * 16,
		})
	}

	props := []any{map[string]any{"name": "blocked", "type": "bool", "value": true}}
	if fp.X != 0 {
		props = append(props, map[string]any{"name": "footprint_w", "type": "int", "value": fp.X})
	}

	if fp.Y != 0 {
		props = append(props, map[string]any{"name": "footprint_h", "type": "int", "value": fp.Y})
	}

	m := map[string]any{
		"type": "map", "orientation": "isometric", "infinite": false,
		"width": synthSide, "height": synthSide, "tilewidth": 160, "tileheight": 80,
		"layers": []any{
			map[string]any{"type": "tilelayer", "name": "floor", "width": synthSide, "height": synthSide, "data": floor},
			map[string]any{"type": "tilelayer", "name": "walls", "width": synthSide, "height": synthSide, "data": walls},
			map[string]any{"type": "objectgroup", "name": "objects", "objects": objects},
		},
		"tilesets": []any{map[string]any{
			"firstgid": 1, "name": "synth", "tilewidth": 160, "tileheight": 80, "tilecount": 2,
			"tiles": []any{
				map[string]any{"id": 0, "image": "tiles/grass.png"},
				map[string]any{"id": 1, "image": "tiles/test.png", "properties": props},
			},
		}},
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{
		"/maps/tiles/grass.png": synthPNG(t, TileWidth, TileHeight),
		"/maps/tiles/test.png":  synthPNG(t, w, h),
	}

	_, err = d2maptiled.Parse(data, "/maps", func(p string) ([]byte, error) {
		b, ok := files[p]
		if !ok {
			return nil, errors.New("no such file: " + p)
		}

		return b, nil
	})

	return err
}

// artCase is one piece of art on one layer, and what this package says about it.
type artCase struct {
	name   string
	layer  d2maptiled.Layer
	fp     image.Point
	w, h   int
	accept bool // what we expect BOTH this package and the loader to say
}

// engineCases straddle every bound the loader has. The "accept" column is the
// expectation; the test proves the loader and this package both meet it, so a
// wrong expectation cannot hide behind a matching pair of bugs.
var engineCases = []artCase{
	// Floors: exactly 160x80 and nothing else (tiled.go:822).
	{"floor exact", d2maptiled.LayerFloor, image.Point{}, 160, 80, true},
	{"floor one px tall", d2maptiled.LayerFloor, image.Point{}, 160, 81, false},
	{"floor one px short", d2maptiled.LayerFloor, image.Point{}, 160, 79, false},
	{"floor one px narrow", d2maptiled.LayerFloor, image.Point{}, 159, 80, false},
	{"floor double wide", d2maptiled.LayerFloor, image.Point{}, 320, 80, false},

	// Walls: 160 wide, 80..512 tall (tiled.go:829). The 512/513 pair is what
	// pins MaxWallHeight.
	{"wall shortest", d2maptiled.LayerWall, image.Point{}, 160, 80, true},
	{"wall too short", d2maptiled.LayerWall, image.Point{}, 160, 79, false},
	{"wall village house", d2maptiled.LayerWall, image.Point{}, 160, 200, true},
	{"wall village tower", d2maptiled.LayerWall, image.Point{}, 160, 330, true},
	{"wall tallest", d2maptiled.LayerWall, image.Point{}, 160, 512, true},
	{"wall one px too tall", d2maptiled.LayerWall, image.Point{}, 160, 513, false},
	{"wall one px wide", d2maptiled.LayerWall, image.Point{}, 161, 200, false},
	{"wall half wide", d2maptiled.LayerWall, image.Point{}, 80, 200, false},

	// Structures: (w+h)*80 wide, 80..768 tall, square, 1..16
	// (tiled.go:811-814, tiled.go:845, tiled.go:880).
	{"structure 1x1 shortest", d2maptiled.LayerStructure, image.Pt(1, 1), 160, 80, true},
	{"structure 1x1 tallest", d2maptiled.LayerStructure, image.Pt(1, 1), 160, 768, true},
	{"structure 1x1 one px too tall", d2maptiled.LayerStructure, image.Pt(1, 1), 160, 769, false},
	{"structure 1x1 too short", d2maptiled.LayerStructure, image.Pt(1, 1), 160, 79, false},
	{"structure no footprint", d2maptiled.LayerStructure, image.Point{}, 480, 448, false},
	{"structure half a footprint", d2maptiled.LayerStructure, image.Pt(0, 3), 480, 448, false},
	{"structure footprint 17", d2maptiled.LayerStructure, image.Pt(17, 17), 2720, 80, false},
	{"structure footprint 16", d2maptiled.LayerStructure, image.Pt(16, 16), 2560, 80, true},

	// NON-SQUARE WITH THE RIGHT WIDTH. These exist because the three illegal
	// Dealu modules break BOTH rules at once, so they cannot tell the square
	// rule from the width rule: deleting the squareness check leaves all three
	// still refused by width alone, and a negative control proved it. Here the
	// width formula is satisfied exactly, so only tiled.go:880 can refuse them.
	{"non-square 2x4 correct width", d2maptiled.LayerStructure, image.Pt(2, 4), 480, 400, false},
	{"non-square 4x2 correct width", d2maptiled.LayerStructure, image.Pt(4, 2), 480, 400, false},
	{"non-square 3x7 correct width", d2maptiled.LayerStructure, image.Pt(3, 7), 800, 400, false},
	{"non-square 1x2 correct width", d2maptiled.LayerStructure, image.Pt(1, 2), 240, 160, false},

	// The village's own houses, and one pixel off them.
	{"peasant house 3x3", d2maptiled.LayerStructure, image.Pt(3, 3), 480, 448, true},
	{"peasant house one px wide", d2maptiled.LayerStructure, image.Pt(3, 3), 481, 448, false},

	// The Dealu monastery, all six modules at their real declared footprints
	// and real render sizes.
	{"dealu church 6x6 960x768", d2maptiled.LayerStructure, image.Pt(6, 6), 960, 768, true},
	{"dealu gate 3x3 480x576", d2maptiled.LayerStructure, image.Pt(3, 3), 480, 576, true},
	{"dealu well 2x2 320x384", d2maptiled.LayerStructure, image.Pt(2, 2), 320, 384, true},
	{"dealu cells 3x7 1120x720", d2maptiled.LayerStructure, image.Pt(3, 7), 1120, 720, false},
	{"dealu refectory 3x6 960x640", d2maptiled.LayerStructure, image.Pt(3, 6), 960, 640, false},
	{"dealu wall 3x1 480x384", d2maptiled.LayerStructure, image.Pt(3, 1), 480, 384, false},

	// The cells' art is not the problem: the same 1120x720 render declared 7x7
	// satisfies the width formula and the height bound and loads. What refuses
	// the cells is its own declared 3x7.
	{"dealu cells art as 7x7", d2maptiled.LayerStructure, image.Pt(7, 7), 1120, 720, true},
}

// TestPaletteAgreesWithTheEngine is the differential test. For every case it
// runs the real d2maptiled.Parse and this package's CheckArt over the same art on
// the same layer, and fails if they disagree in either direction -- a palette
// that greys out art the engine would load is as wrong as one that offers art it
// would refuse.
func TestPaletteAgreesWithTheEngine(t *testing.T) {
	for _, c := range engineCases {
		t.Run(c.name, func(t *testing.T) {
			loaderErr := placeOn(t, c.layer, c.fp, c.w, c.h)
			problems := CheckArt(c.layer, c.fp, c.w, c.h)

			loaderOK := loaderErr == nil
			paletteOK := len(problems) == 0

			if loaderOK != c.accept {
				t.Errorf("the LOADER %s this art, the case expects %s (err: %v)",
					acceptedWord(loaderOK), acceptedWord(c.accept), loaderErr)
			}

			if paletteOK != c.accept {
				t.Errorf("the PALETTE %s this art, the case expects %s (problems: %v)",
					acceptedWord(paletteOK), acceptedWord(c.accept), problems)
			}

			if loaderOK != paletteOK {
				t.Fatalf("DISAGREEMENT on %s %v %dx%d: loader %s, palette %s (loader err %v, palette %v)",
					c.layer, c.fp, c.w, c.h, acceptedWord(loaderOK), acceptedWord(paletteOK), loaderErr, problems)
			}
		})
	}
}

func acceptedWord(ok bool) string {
	if ok {
		return "accepts"
	}

	return "refuses"
}

// TestHeightBoundsMatchTheLoader pins the two constants this package is forced to
// copy, because the loader keeps maxWallHeight (tiled.go:120) and
// maxStructureHeight (tiled.go:141) unexported.
//
// A copied constant is a bug waiting for somebody to change the original, so
// this does not compare numbers -- it asks the real loader about art at exactly
// the bound and exactly one pixel past it, and fails if the bound moved.
func TestHeightBoundsMatchTheLoader(t *testing.T) {
	if err := placeOn(t, d2maptiled.LayerWall, image.Point{}, TileWidth, MaxWallHeight); err != nil {
		t.Errorf("the loader refuses a wall %d tall, so MaxWallHeight (%d) is too big: %v",
			MaxWallHeight, MaxWallHeight, err)
	}

	if err := placeOn(t, d2maptiled.LayerWall, image.Point{}, TileWidth, MaxWallHeight+1); err == nil {
		t.Errorf("the loader accepts a wall %d tall, so MaxWallHeight (%d) is too small",
			MaxWallHeight+1, MaxWallHeight)
	}

	if err := placeOn(t, d2maptiled.LayerStructure, image.Pt(1, 1), TileWidth, MaxStructureHeight); err != nil {
		t.Errorf("the loader refuses a structure %d tall, so MaxStructureHeight (%d) is too big: %v",
			MaxStructureHeight, MaxStructureHeight, err)
	}

	if err := placeOn(t, d2maptiled.LayerStructure, image.Pt(1, 1), TileWidth, MaxStructureHeight+1); err == nil {
		t.Errorf("the loader accepts a structure %d tall, so MaxStructureHeight (%d) is too small",
			MaxStructureHeight+1, MaxStructureHeight)
	}

	// MaxFootprint, the same way: 16 loads and 17 does not.
	if err := placeOn(t, d2maptiled.LayerStructure, image.Pt(MaxFootprint, MaxFootprint),
		StructureWidthFor(image.Pt(MaxFootprint, MaxFootprint)), TileHeight); err != nil {
		t.Errorf("the loader refuses a %dx%d footprint, so MaxFootprint is too big: %v",
			MaxFootprint, MaxFootprint, err)
	}

	over := MaxFootprint + 1
	if err := placeOn(t, d2maptiled.LayerStructure, image.Pt(over, over),
		StructureWidthFor(image.Pt(over, over)), TileHeight); err == nil {
		t.Errorf("the loader accepts a %dx%d footprint, so MaxFootprint (%d) is too small",
			over, over, MaxFootprint)
	}
}

// TestLayerNarrowingMatchesTheLoader holds the other half of kind()'s rule
// (tiled.go:689-694): a footprinted tile may ONLY be a tile object, and an
// unfootprinted one may only be on a tile layer. This package encodes that as
// shape.allowed, and the loader enforces it before it ever looks at the art's
// size -- so both cases below use art that would otherwise be perfectly legal.
func TestLayerNarrowingMatchesTheLoader(t *testing.T) {
	// A 3x3 structure's art, correct in every dimension, put on the walls
	// layer. 480 wide is not a legal wall anyway, so use a 1x1 structure whose
	// 160x200 art is a legal wall: only the footprint can refuse it.
	if err := placeOn(t, d2maptiled.LayerWall, image.Pt(1, 1), TileWidth, 200); err == nil {
		t.Error("the loader accepted a footprinted tile on the walls layer; tiled.go:692 says it refuses one")
	}

	if got := CheckArt(d2maptiled.LayerWall, image.Pt(1, 1), TileWidth, 200); len(got) == 0 {
		t.Error("CheckArt accepted a footprinted tile on the walls layer")
	}

	if err := placeOn(t, d2maptiled.LayerFloor, image.Pt(1, 1), TileWidth, TileHeight); err == nil {
		t.Error("the loader accepted a footprinted tile on the floor layer; tiled.go:692 says it refuses one")
	}

	// And the reverse: a tile object with no footprint, whose art is a legal
	// 1x1 structure size.
	if err := placeOn(t, d2maptiled.LayerStructure, image.Point{}, TileWidth, 200); err == nil {
		t.Error("the loader accepted an unfootprinted tile object; tiled.go:690 says only structures are tile objects")
	}

	got := CheckArt(d2maptiled.LayerStructure, image.Point{}, TileWidth, 200)
	if len(got) == 0 {
		t.Fatal("CheckArt accepted an unfootprinted tile object")
	}

	// And it must say the USEFUL thing. A negative control found this branch
	// triple-guarded: with the early return disabled, the footprint-range check
	// and then the width formula still refuse the art, so accept/refuse alone
	// could not tell whether the branch was there. What the user is told can.
	// "a 0x0 footprint wants exactly 0 px wide" is technically a refusal and
	// useless advice.
	if !strings.Contains(got[0], "only structures are tile objects") {
		t.Errorf("an unfootprinted tile object is refused with %q; it should name the actual mistake", got[0])
	}
}

// TestPNGSizeAgreesWithTheImageDecoder holds the IHDR short-cut against a real
// decode. The engine's loader decodes the image for real (tiled.go:788); if
// reading two big-endian words out of the header ever disagreed with that, the
// palette would measure art the engine sizes differently.
func TestPNGSizeAgreesWithTheImageDecoder(t *testing.T) {
	for _, wh := range [][2]int{{160, 80}, {160, 512}, {480, 448}, {960, 768}, {1120, 720}, {2560, 80}, {1, 1}} {
		data := synthPNG(t, wh[0], wh[1])

		gotW, gotH, err := PNGSize(data)
		if err != nil {
			t.Fatalf("PNGSize on a %dx%d PNG: %v", wh[0], wh[1], err)
		}

		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}

		if gotW != cfg.Width || gotH != cfg.Height {
			t.Errorf("PNGSize says %dx%d, the decoder says %dx%d", gotW, gotH, cfg.Width, cfg.Height)
		}
	}
}

// TestPNGSizeRefusesWhatIsNotAPNG keeps the short-cut from inventing a size for
// bytes it cannot read, which is how a palette would end up offering a .txt as
// art.
func TestPNGSizeRefusesWhatIsNotAPNG(t *testing.T) {
	full := synthPNG(t, 160, 80)

	cases := map[string][]byte{
		"empty":            {},
		"too short":        full[:23],
		"not a png":        []byte("GIF89a............................"),
		"png with no ihdr": append(append([]byte{}, full[:12]...), []byte("IDAT............")...),
	}

	for name, data := range cases {
		if w, h, err := PNGSize(data); err == nil {
			t.Errorf("%s: PNGSize returned %dx%d and no error", name, w, h)
		}
	}
}

// helper so a failure message can print a footprint compactly.
func (c artCase) String() string { return fmt.Sprintf("%s %v %dx%d", c.layer, c.fp, c.w, c.h) }
