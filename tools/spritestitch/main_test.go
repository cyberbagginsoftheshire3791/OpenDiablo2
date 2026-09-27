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

// TestStitchAnchorsTheFeet: a figure standing in a frame with an empty margin
// below it must come out with its centre and its feet on the foot point --
// numbers this test chooses (a 10x12 frame, feet on row 8) against the
// manifest the tool writes. Before 26 Sep the manifest carried no offsets and
// every sheet was drawn half a frame right of where it stood.
func TestStitchAnchorsTheFeet(t *testing.T) {
	dir := t.TempDir()

	for direction := 0; direction < 2; direction++ {
		for frame := 0; frame < 3; frame++ {
			img := image.NewNRGBA(image.Rect(0, 0, 10, 12))
			// A 4-wide figure from row 2 down to its feet on row 8; rows 9-11 empty.
			for y := 2; y <= 8; y++ {
				for x := 3; x < 7; x++ {
					img.SetNRGBA(x, y, color.NRGBA{R: 200, A: 255})
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

	_, def, err := stitch(dir, 2, 3)
	if err != nil {
		t.Fatal(err)
	}

	if def.OffsetX != -5 || def.OffsetY != 12-8 {
		t.Fatalf("anchor = (%d,%d), want (-5,4): the figure's centre 4.5 (columns 3-6) rounds to 5, and 12 minus the feet row 8", def.OffsetX, def.OffsetY)
	}
}

// TestStitchRefusesAFigureWithNoFeet: an all-transparent sheet has nothing to
// anchor, and a manifest measured from nothing would be a silent zero.
func TestStitchRefusesAFigureWithNoFeet(t *testing.T) {
	dir := t.TempDir()

	for frame := 0; frame < 2; frame++ {
		path := filepath.Join(dir, frameName(0, frame))
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
	}

	if _, _, err := stitch(dir, 1, 2); err == nil {
		t.Fatal("stitching transparent frames must fail: there are no feet to anchor")
	}
}

// TestAnchorFromCopiesTheIdleSheetsOffsets: a lying sheet takes its offsets
// from the idle manifest, and a manifest with none is refused rather than
// copied as a silent zero.
func TestAnchorFromCopiesTheIdleSheetsOffsets(t *testing.T) {
	dir := t.TempDir()
	idle := filepath.Join(dir, "idle.png.json")

	if err := os.WriteFile(idle, []byte(`{"directions":8,"frames_per_direction":6,"frame_width":96,"frame_height":96,"offset_x":-47,"offset_y":31,"origin_at_bottom":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if x, y, err := anchorOf(idle); err != nil || x != -47 || y != 31 {
		t.Fatalf("anchorOf(idle) = (%d,%d,%v), want (-47,31,nil)", x, y, err)
	}

	bare := filepath.Join(dir, "bare.png.json")
	if err := os.WriteFile(bare, []byte(`{"directions":8,"frames_per_direction":6,"origin_at_bottom":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := anchorOf(bare); err == nil {
		t.Fatal("anchorOf must refuse a manifest with no offsets to copy")
	}
}
