package d2asset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"

	// Registers the PNG decoder with image.Decode. The blank import is the
	// point: nothing in this file calls into image/png directly.
	_ "image/png"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2dcc"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

var _ d2interface.Animation = &PNGAnimation{} // Static check to confirm struct conforms to
// interface

// THE MODERN ASSET PATH (Plan §5, M5.1), and the reason it is the first thing
// built after 19 September 2026.
//
// Everything this game is supposed to have of its own -- its monsters, its
// gear, its village, its faces -- is blocked on one fact: the engine could only
// load sprites out of D2's DC6 and DCC formats. So a "wolf" was `zombie1` and a
// "boar" was `skeleton1`, because those were the sprites that existed. The plan
// calls the asset-independence ratchet and "the game becoming its own thing"
// the same work item, and this is where that starts being true.
//
// A PNG IS NOT A DC6, AND THE DIFFERENCES ARE THE WHOLE DESIGN:
//
//   - D2 sprites carry per-frame OffsetX/OffsetY, a direction count, and a
//     frames-per-direction count INSIDE the file. A PNG carries none of that,
//     so a sidecar manifest supplies it: `wolf.png` is described by
//     `wolf.png.json`. With no sidecar a PNG is one direction of one frame,
//     which makes any loose image usable as a still sprite immediately.
//   - D2 pixel data is 8-bit palette indices, coloured later by
//     d2util.ImgIndexToRGBA, and index 0 is hardcoded transparent -- D2 art has
//     1-bit alpha and no palette in this engine ever sets a real alpha byte. A
//     PNG has an 8-bit alpha channel, so it needs NO palette at all, and the
//     palette fetch in LoadAnimationWithEffect had to move inside the DC6 and
//     DCC cases for this format to be loadable without lying about one.
//   - ebiten's ReplacePixels wants PREMULTIPLIED alpha. ImgIndexToRGBA only
//     ever emitted (0,0,0,0) or (r,g,b,255), both already premultiplied, so
//     this never came up before. A PNG with genuine partial alpha renders with
//     bright halos unless it is premultiplied first, and image/png hands back
//     *image.NRGBA for most files. draw.Draw into an *image.RGBA does the
//     conversion, which is why the decode goes through image.Decode rather than
//     asserting a concrete type.
type PNGAnimation struct {
	Animation
	sheet *image.RGBA
	def   PNGSheet
}

// PNGSheet describes how one PNG is cut into directions and frames.
//
// It is the sidecar file's schema, and it is deliberately small: a spritesheet
// is a grid, rows are directions and columns are frames. Anything richer (named
// modes, per-frame offsets, trimmed atlases) is a later decision, and adding it
// must not change what an existing sheet means.
type PNGSheet struct {
	// Directions is how many rows the sheet has. It must be one of the counts
	// the engine's direction mapper understands -- 1, 4, 8, 16, 32 or 64 --
	// because d2dcc.Dir64ToDcc returns direction 0 for every other count, which
	// would silently point every facing at the same row.
	Directions int `json:"directions"`

	// FramesPerDirection is how many columns the sheet has.
	FramesPerDirection int `json:"frames_per_direction"`

	// FrameWidth and FrameHeight are the size of one cell. Zero means "derive
	// it from the image and the counts", which is right for an evenly cut sheet
	// and is what a hand-made sheet will almost always be.
	FrameWidth  int `json:"frame_width,omitempty"`
	FrameHeight int `json:"frame_height,omitempty"`

	// OffsetX and OffsetY place every frame relative to the entity's position,
	// the way a DC6 frame's own offsets do. One pair for the whole sheet rather
	// than one per frame: per-frame offsets are what a trimmed atlas needs, and
	// this format does not trim.
	OffsetX int `json:"offset_x,omitempty"`
	OffsetY int `json:"offset_y,omitempty"`

	// OriginAtBottom matches DC6's behaviour, where a sprite hangs from the
	// bottom of its frame rather than the top. Ground-standing art wants it;
	// UI art does not. DC6 sets it always, DCC never, so it is a field rather
	// than a constant.
	OriginAtBottom bool `json:"origin_at_bottom,omitempty"`
}

// pngSheetExt is appended to a sprite's own path to find its manifest, so the
// sheet is named after the image it describes and sorts beside it.
const pngSheetExt = ".json"

// singleFrameSheet is what a PNG with no sidecar means: one frame, one
// direction, the whole image, origin at the top.
func singleFrameSheet() PNGSheet {
	return PNGSheet{Directions: 1, FramesPerDirection: 1}
}

// validate fills in what the sheet left out and rejects what cannot be drawn.
//
// IT REFUSES A DIRECTION COUNT THE ENGINE CANNOT MAP, and that refusal is the
// most valuable line in this file. d2dcc.Dir64ToDcc has lookup tables for 4, 8,
// 16, 32 and 64 directions and returns 0 for anything else -- so a sheet
// authored with, say, 6 rows would load, render, and point all 64 facings at
// row 0, which looks like a broken animation rather than a bad manifest.
func (s *PNGSheet) validate(bounds image.Rectangle) error {
	if s.Directions <= 0 {
		return fmt.Errorf("directions must be positive, got %d", s.Directions)
	}

	switch s.Directions {
	case 1, 4, 8, 16, 32, 64:
	default:
		return fmt.Errorf(
			"directions %d is not a count the engine can map: use 1, 4, 8, 16, 32 or 64 "+
				"(d2dcc.Dir64ToDcc has no table for anything else and would aim every facing at row 0)",
			s.Directions)
	}

	if s.FramesPerDirection <= 0 {
		return fmt.Errorf("frames_per_direction must be positive, got %d", s.FramesPerDirection)
	}

	if s.FrameWidth == 0 {
		s.FrameWidth = bounds.Dx() / s.FramesPerDirection
	}

	if s.FrameHeight == 0 {
		s.FrameHeight = bounds.Dy() / s.Directions
	}

	if s.FrameWidth <= 0 || s.FrameHeight <= 0 {
		return fmt.Errorf("frame size resolves to %dx%d", s.FrameWidth, s.FrameHeight)
	}

	if need := s.FrameWidth * s.FramesPerDirection; need > bounds.Dx() {
		return fmt.Errorf("sheet is %d px wide but %d frames of %d px need %d",
			bounds.Dx(), s.FramesPerDirection, s.FrameWidth, need)
	}

	if need := s.FrameHeight * s.Directions; need > bounds.Dy() {
		return fmt.Errorf("sheet is %d px tall but %d directions of %d px need %d",
			bounds.Dy(), s.Directions, s.FrameHeight, need)
	}

	return nil
}

// decodePNGSheet reads a manifest. An empty or absent body is the single-frame
// default rather than an error, so a loose PNG needs no ceremony to be usable.
func decodePNGSheet(manifest []byte) (PNGSheet, error) {
	if len(bytes.TrimSpace(manifest)) == 0 {
		return singleFrameSheet(), nil
	}

	sheet := PNGSheet{}
	if err := json.Unmarshal(manifest, &sheet); err != nil {
		return PNGSheet{}, fmt.Errorf("sprite sheet manifest: %w", err)
	}

	return sheet, nil
}

// decodePNG turns image bytes into premultiplied RGBA, whatever colour model
// the file used. See the type comment for why the premultiply is not optional.
func decodePNG(data []byte) (*image.RGBA, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding sprite image: %w", err)
	}

	bounds := src.Bounds()
	if bounds.Empty() {
		return nil, errors.New("sprite image has no pixels")
	}

	// draw.Draw onto a zeroed RGBA both normalises the origin to (0,0) and
	// premultiplies, because *image.RGBA is premultiplied by definition.
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, bounds.Min, draw.Src)

	return rgba, nil
}

func newPNGAnimation(
	data, manifest []byte,
	effect d2enum.DrawEffect,
) (d2interface.Animation, error) {
	rgba, err := decodePNG(data)
	if err != nil {
		return nil, err
	}

	sheet, err := decodePNGSheet(manifest)
	if err != nil {
		return nil, err
	}

	png, err := pngAnimationFromRGBA(rgba, sheet, effect)
	if err != nil {
		return nil, err
	}

	return png, nil
}

// pngAnimationFromRGBA cuts an already-decoded, premultiplied sheet --
// Strigoi's fonts are drawn in memory and never encoded as a PNG at all.
func pngAnimationFromRGBA(rgba *image.RGBA, sheet PNGSheet, effect d2enum.DrawEffect) (*PNGAnimation, error) {
	if err := sheet.validate(rgba.Bounds()); err != nil {
		return nil, err
	}

	png := &PNGAnimation{sheet: rgba, def: sheet}

	anim := &Animation{
		playLength:     defaultPlayLength,
		playLoop:       true,
		effect:         effect,
		originAtBottom: sheet.OriginAtBottom,
		onBindRenderer: func(r d2interface.Renderer) error {
			if png.renderer != r {
				png.renderer = r
				return png.createSurfaces()
			}

			return nil
		},
	}

	png.Animation = *anim

	png.init()

	return png, nil
}

func (a *PNGAnimation) init() {
	a.directions = make([]animationDirection, a.def.Directions)

	for directionIndex := range a.directions {
		a.directions[directionIndex].frames = make([]animationFrame, a.def.FramesPerDirection)
		a.directions[directionIndex].decoded = true

		for frameIndex := range a.directions[directionIndex].frames {
			a.directions[directionIndex].frames[frameIndex] = animationFrame{
				decoded: true,
				width:   a.def.FrameWidth,
				height:  a.def.FrameHeight,
				offsetX: a.def.OffsetX,
				offsetY: a.def.OffsetY,
			}
		}
	}
}

// Clone creates a copy of the animation.
//
// The decoded sheet is SHARED rather than copied: it is immutable after load,
// and a composite clones one animation per layer per entity, so copying a
// spritesheet per clone would multiply the memory cost of every monster on
// screen by the size of its art.
func (a *PNGAnimation) Clone() d2interface.Animation {
	clone := &PNGAnimation{}
	clone.Animation = *a.Animation.Clone().(*Animation)
	clone.sheet = a.sheet
	clone.def = a.def

	return clone
}

// SetDirection places the animation in the direction of an animation.
//
// It follows the DCC version rather than the DC6 one, deliberately: DC6's
// override tests `decoded` against the UN-mapped index and then assigns the
// mapped one, which is a latent bug this file does not copy.
func (a *PNGAnimation) SetDirection(directionIndex int) error {
	const smallestInvalidDirectionIndex = 64
	if directionIndex >= smallestInvalidDirectionIndex {
		return errors.New("invalid direction index")
	}

	a.directionIndex = d2dcc.Dir64ToDcc(directionIndex, len(a.directions))
	a.frameIndex = 0

	return nil
}

func (a *PNGAnimation) createSurfaces() error {
	if a.renderer == nil {
		return errors.New("no renderer")
	}

	for directionIndex := range a.directions {
		for frameIndex := range a.directions[directionIndex].frames {
			surface, err := a.createFrameSurface(directionIndex, frameIndex)
			if err != nil {
				return err
			}

			a.directions[directionIndex].frames[frameIndex].image = surface
		}
	}

	return nil
}

// frameRect is the cell one frame occupies in the sheet: columns are frames,
// rows are directions.
func (a *PNGAnimation) frameRect(directionIndex, frameIndex int) image.Rectangle {
	x := frameIndex * a.def.FrameWidth
	y := directionIndex * a.def.FrameHeight

	return image.Rect(x, y, x+a.def.FrameWidth, y+a.def.FrameHeight)
}

func (a *PNGAnimation) createFrameSurface(directionIndex, frameIndex int) (d2interface.Surface, error) {
	if a.renderer == nil {
		return nil, errors.New("no renderer")
	}

	pixels := a.framePixels(directionIndex, frameIndex)

	sfc := a.renderer.NewSurface(a.def.FrameWidth, a.def.FrameHeight)
	sfc.ReplacePixels(pixels)

	return sfc, nil
}

// framePixels copies one cell out of the sheet into the tightly packed,
// row-major RGBA buffer Surface.ReplacePixels expects.
//
// It is a separate method from createFrameSurface so the cut can be asserted
// without a renderer -- d2asset has no ebiten in its import graph, and a test
// that needed a real surface would drag one in.
func (a *PNGAnimation) framePixels(directionIndex, frameIndex int) []byte {
	rect := a.frameRect(directionIndex, frameIndex)
	out := make([]byte, a.def.FrameWidth*a.def.FrameHeight*4)

	for y := 0; y < a.def.FrameHeight; y++ {
		srcStart := a.sheet.PixOffset(rect.Min.X, rect.Min.Y+y)
		dstStart := y * a.def.FrameWidth * 4

		copy(out[dstStart:dstStart+a.def.FrameWidth*4], a.sheet.Pix[srcStart:])
	}

	return out
}
