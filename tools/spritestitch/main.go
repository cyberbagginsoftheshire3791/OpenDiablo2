// Command spritestitch assembles equally sized PNG frames into the grid used
// by Strigoi's PNG animation loader: columns are frames and rows are facings.
//
// IT ALSO ANCHORS THE SHEET (26 Sep 2026). A ground-standing frame is placed
// only through its manifest's offsets: with none, the frame's bottom-LEFT
// corner lands on the entity's foot point, so the figure is drawn half a frame
// to the right of where it stands and lifted by its empty lower margin. That
// shipped for the Janissary and every creature this tool stitched. So the
// manifest now carries offset_x = -(the figure's centre) and offset_y =
// frame_height - (its feet row), each the median over every frame: the centre
// is the mean x of the opaque pixels, the feet the lowest opaque row. The
// figure's centre, not the frame's: a boar drawn side-on sits 7 px left of its
// frame's middle, and half-a-frame would have stood it 7 px off. A lying sheet
// (death, dead) must not measure itself -- its centre and lowest row are not a
// standing figure's -- so pass -anchor-from <the idle sheet's .png.json> and
// both offsets are copied from it, so the body does not jump when it falls.
// d2core/d2asset's TestGroundSheetsStandOnTheirFootPoint holds every sheet to it.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
)

type manifest struct {
	Directions         int  `json:"directions"`
	FramesPerDirection int  `json:"frames_per_direction"`
	FrameWidth         int  `json:"frame_width"`
	FrameHeight        int  `json:"frame_height"`
	OffsetX            int  `json:"offset_x"`
	OffsetY            int  `json:"offset_y"`
	OriginAtBottom     bool `json:"origin_at_bottom"`
}

// alphaMin is the alpha above which a pixel is part of the figure.
const alphaMin = 32

// medianOf is the median of what the frames measured.
func medianOf(v []float64) float64 {
	sorted := append([]float64(nil), v...)
	sort.Float64s(sorted)

	return sorted[len(sorted)/2]
}

// figureCentre is the mean x of img's opaque pixels, relative to its bounds,
// or -1 for a transparent frame.
func figureCentre(img image.Image) float64 {
	b := img.Bounds()
	sum, n := 0.0, 0

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a>>8 > alphaMin {
				sum += float64(x - b.Min.X)
				n++
			}
		}
	}

	if n == 0 {
		return -1
	}

	return sum / float64(n)
}

// lowestOpaqueRow is the last row of img (relative to its bounds) holding a
// pixel with alpha above alphaMin, or -1 for a transparent frame.
func lowestOpaqueRow(img image.Image) int {
	b := img.Bounds()

	for y := b.Max.Y - 1; y >= b.Min.Y; y-- {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a>>8 > alphaMin {
				return y - b.Min.Y
			}
		}
	}

	return -1
}

func stitch(input string, directions, frames int) (*image.NRGBA, manifest, error) {
	if directions <= 0 || frames <= 0 {
		return nil, manifest{}, fmt.Errorf("directions and frames must be positive")
	}

	var sheet *image.NRGBA
	var cell image.Rectangle
	var lows, centres []float64
	for direction := 0; direction < directions; direction++ {
		for frame := 0; frame < frames; frame++ {
			path := filepath.Join(input, fmt.Sprintf("direction-%02d-frame-%02d.png", direction, frame))
			file, err := os.Open(path)
			if err != nil {
				return nil, manifest{}, err
			}
			decoded, err := png.Decode(file)
			_ = file.Close()
			if err != nil {
				return nil, manifest{}, fmt.Errorf("decode %s: %w", path, err)
			}

			bounds := decoded.Bounds()
			if sheet == nil {
				cell = image.Rect(0, 0, bounds.Dx(), bounds.Dy())
				sheet = image.NewNRGBA(image.Rect(0, 0, bounds.Dx()*frames, bounds.Dy()*directions))
			}
			if bounds.Dx() != cell.Dx() || bounds.Dy() != cell.Dy() {
				return nil, manifest{}, fmt.Errorf("%s is %dx%d, want %dx%d",
					path, bounds.Dx(), bounds.Dy(), cell.Dx(), cell.Dy())
			}

			destination := image.Rect(
				frame*cell.Dx(), direction*cell.Dy(),
				(frame+1)*cell.Dx(), (direction+1)*cell.Dy(),
			)
			draw.Draw(sheet, destination, decoded, bounds.Min, draw.Src)

			if low := lowestOpaqueRow(decoded); low >= 0 {
				lows = append(lows, float64(low))
				centres = append(centres, figureCentre(decoded))
			}
		}
	}

	if len(lows) == 0 {
		return nil, manifest{}, fmt.Errorf("every frame is transparent; there are no feet to anchor")
	}

	return sheet, manifest{
		Directions: directions, FramesPerDirection: frames,
		FrameWidth: cell.Dx(), FrameHeight: cell.Dy(),
		OffsetX: -int(math.Round(medianOf(centres))), OffsetY: cell.Dy() - int(medianOf(lows)),
		OriginAtBottom: true,
	}, nil
}

func main() {
	input := flag.String("input", "", "folder containing direction-NN-frame-NN.png files")
	output := flag.String("output", "", "output spritesheet PNG")
	directions := flag.Int("directions", 8, "number of facing rows")
	frames := flag.Int("frames", 1, "number of frame columns")
	anchorFrom := flag.String("anchor-from", "", "copy offset_x and offset_y from this manifest instead of measuring (a death or dead sheet: the idle sheet's .png.json)")
	flag.Parse()

	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "input and output are required")
		os.Exit(2)
	}

	sheet, def, err := stitch(*input, *directions, *frames)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *anchorFrom != "" {
		x, y, err := anchorOf(*anchorFrom)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		def.OffsetX, def.OffsetY = x, y
	}

	fmt.Printf("%s: %dx%d frames, anchored offset_x %d offset_y %d\n", *output, def.FrameWidth, def.FrameHeight, def.OffsetX, def.OffsetY)
	if err := os.MkdirAll(filepath.Dir(*output), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	file, err := os.Create(*output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := png.Encode(file, sheet); err != nil {
		_ = file.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := file.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	manifestData, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	manifestData = append(manifestData, '\n')
	if err := os.WriteFile(*output+".json", manifestData, 0o640); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// anchorOf reads the offsets from another sheet's manifest.
func anchorOf(path string) (x, y int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}

	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return 0, 0, fmt.Errorf("%s: %w", path, err)
	}

	if m.OffsetX == 0 && m.OffsetY == 0 {
		return 0, 0, fmt.Errorf("%s carries no offsets to copy -- stitch its sheet with this tool first", path)
	}

	return m.OffsetX, m.OffsetY, nil
}
