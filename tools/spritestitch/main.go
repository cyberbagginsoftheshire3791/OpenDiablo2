// Command spritestitch assembles equally sized PNG frames into the grid used
// by Strigoi's PNG animation loader: columns are frames and rows are facings.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

type manifest struct {
	Directions         int  `json:"directions"`
	FramesPerDirection int  `json:"frames_per_direction"`
	FrameWidth         int  `json:"frame_width"`
	FrameHeight        int  `json:"frame_height"`
	OriginAtBottom     bool `json:"origin_at_bottom"`
}

func stitch(input string, directions, frames int) (*image.NRGBA, manifest, error) {
	if directions <= 0 || frames <= 0 {
		return nil, manifest{}, fmt.Errorf("directions and frames must be positive")
	}

	var sheet *image.NRGBA
	var cell image.Rectangle
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
		}
	}

	return sheet, manifest{
		Directions: directions, FramesPerDirection: frames,
		FrameWidth: cell.Dx(), FrameHeight: cell.Dy(), OriginAtBottom: true,
	}, nil
}

func main() {
	input := flag.String("input", "", "folder containing direction-NN-frame-NN.png files")
	output := flag.String("output", "", "output spritesheet PNG")
	directions := flag.Int("directions", 8, "number of facing rows")
	frames := flag.Int("frames", 1, "number of frame columns")
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
