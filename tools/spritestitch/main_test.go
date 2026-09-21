package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestStitchPlacesFramesAcrossAndDirectionsDown(t *testing.T) {
	dir := t.TempDir()
	for direction := 0; direction < 2; direction++ {
		for frame := 0; frame < 3; frame++ {
			img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
			fill := color.NRGBA{R: uint8(20 + frame), G: uint8(80 + direction), A: 255}
			for y := 0; y < 2; y++ {
				for x := 0; x < 2; x++ {
					img.SetNRGBA(x, y, fill)
				}
			}
			path := filepath.Join(dir, frameName(direction, frame))
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, img); err != nil {
				t.Fatal(err)
			}
			_ = file.Close()
		}
	}

	sheet, def, err := stitch(dir, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.Bounds().Dx() != 6 || sheet.Bounds().Dy() != 4 {
		t.Fatalf("sheet = %v", sheet.Bounds())
	}
	if def.FrameWidth != 2 || def.FrameHeight != 2 || !def.OriginAtBottom {
		t.Fatalf("manifest = %+v", def)
	}
	got := color.NRGBAModel.Convert(sheet.At(5, 3)).(color.NRGBA)
	if got.R != 22 || got.G != 81 {
		t.Fatalf("bottom-right cell = %+v", got)
	}
}

func frameName(direction, frame int) string {
	return fmt.Sprintf("direction-%02d-frame-%02d.png", direction, frame)
}
