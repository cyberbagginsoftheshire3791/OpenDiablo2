package d2asset

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A ground-standing PNG sheet is placed only through its manifest's offsets
// (animation.go RenderFromOrigin: the frame hangs from the foot point, then
// moves by offset_x / offset_y). With no offsets the frame's bottom-LEFT
// corner lands on the foot point, so the figure is drawn half a frame to the
// right of where it stands and its empty lower margin lifts it off the ground.
//
// That shipped on 24 Sep 2026 (bb04888f) for the Janissary and for every
// bestiary creature: drawn ~62 px right and ~31 px up of where he stood, while
// his bar, his clicks and his depth sort used the true spot. No test looked:
// TestHeroArt asserted the sheet name, the creature-art test logged a screen
// position and asserted nothing about it. This test measures the ART and holds
// the manifest to it -- two numbers from two sources:
//
//   - horizontally, the median alpha centroid of the frames must sit on the
//     foot point: |centroid + offset_x| <= anchorXTolerance;
//   - vertically, the median lowest opaque row (the feet) must sit on it:
//     |feet - (frame_height - offset_y)| <= anchorYTolerance.
//
// Only the standing modes are measured. A death or a dead sheet lies down, so
// its lowest row is not the feet; it must carry the SAME offsets as its
// creature's idle sheet, or the body would jump when it falls.
const (
	anchorXTolerance = 6 // px: an attack's swing moves the centroid a little
	anchorYTolerance = 4 // px
	anchorAlphaMin   = 32
)

var standingModes = map[string]bool{"idle": true, "walk": true, "run": true, "attack": true, "hit": true, "block": true}

// knownUnanchored names sheet folders whose art is known to be drawn off its
// foot point, each with the bug that owns it. An entry that PASSES fails the
// test, so the list can only shrink: fix the manifests, then delete the line.
var knownUnanchored = map[string]string{
	"hero/janissary": "BUG-18: GPT re-stitches the Janissary with offset_x -64, offset_y 29 (and hero.json height 70)",
}

type anchorManifest struct {
	Directions         int  `json:"directions"`
	FramesPerDirection int  `json:"frames_per_direction"`
	FrameWidth         int  `json:"frame_width"`
	FrameHeight        int  `json:"frame_height"`
	OffsetX            int  `json:"offset_x"`
	OffsetY            int  `json:"offset_y"`
	OriginAtBottom     bool `json:"origin_at_bottom"`
}

func TestGroundSheetsStandOnTheirFootPoint(t *testing.T) {
	root := filepath.Join("..", "..", "data", "strigoi")

	manifests, err := filepath.Glob(filepath.Join(root, "*", "*", "*.png.json"))
	if err != nil {
		t.Fatal(err)
	}

	byDir := map[string]map[string]anchorManifest{}
	failures := map[string][]string{}
	checked := 0

	for _, path := range manifests {
		var m anchorManifest

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		rel, _ := filepath.Rel(root, filepath.Dir(path))
		dir := filepath.ToSlash(rel)

		if !m.OriginAtBottom {
			// A creature or a hero that does not hang from its foot point is not
			// exempt from this test -- it is wrong (and dropping the flag was the
			// one way to walk out of this check unseen; review, 26 Sep).
			if strings.HasPrefix(dir, "creatures/") || strings.HasPrefix(dir, "hero/") {
				failures[dir] = append(failures[dir], fmt.Sprintf("%s: a ground-standing sheet without origin_at_bottom", filepath.Base(path)))
				if byDir[dir] == nil {
					byDir[dir] = map[string]anchorManifest{}
				}
			}

			continue
		}
		mode := strings.TrimSuffix(filepath.Base(path), ".png.json")

		if byDir[dir] == nil {
			byDir[dir] = map[string]anchorManifest{}
		}

		byDir[dir][mode] = m

		if !standingModes[mode] {
			continue
		}

		centroid, feet, err := measureSheet(strings.TrimSuffix(path, ".json"), m)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		checked++

		if dx := centroid + float64(m.OffsetX); dx > anchorXTolerance || dx < -anchorXTolerance {
			failures[dir] = append(failures[dir], fmt.Sprintf("%s: the figure's centre is %.1f px from its foot point horizontally "+
				"(centroid %.1f, offset_x %d; want offset_x about %d)", mode, dx, centroid, m.OffsetX, -int(centroid+0.5)))
		}

		ground := float64(m.FrameHeight - m.OffsetY)
		if dy := feet - ground; dy > anchorYTolerance || dy < -anchorYTolerance {
			failures[dir] = append(failures[dir], fmt.Sprintf("%s: the feet are %.1f px from the foot point vertically "+
				"(feet row %.0f, frame_height %d, offset_y %d; want offset_y about %d)", mode, dy, feet, m.FrameHeight, m.OffsetY, m.FrameHeight-int(feet)))
		}
	}

	for dir, modes := range byDir {
		idle, ok := modes["idle"]
		if !ok {
			for mode := range modes {
				if !standingModes[mode] {
					failures[dir] = append(failures[dir], fmt.Sprintf("%s: a lying sheet with no idle sheet to take its offsets from", mode))
				}
			}

			continue
		}

		for mode, m := range modes {
			if standingModes[mode] {
				continue
			}

			if m.OffsetX != idle.OffsetX || m.OffsetY != idle.OffsetY {
				failures[dir] = append(failures[dir], fmt.Sprintf("%s: offsets (%d,%d) differ from idle's (%d,%d); a falling body would jump",
					mode, m.OffsetX, m.OffsetY, idle.OffsetX, idle.OffsetY))
			}
		}
	}

	if checked == 0 {
		t.Fatal("no ground-standing sheet was measured -- the glob found nothing, so this test proved nothing")
	}

	// A known entry must name a folder that was measured: one whose folder
	// vanished, or stopped being ground-standing, would otherwise never expire.
	for dir, why := range knownUnanchored {
		if _, ok := byDir[dir]; !ok {
			t.Errorf("knownUnanchored names %s (%s) but no ground-standing sheet there was measured -- delete the entry", dir, why)
		}
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}

	sort.Strings(dirs)

	for _, dir := range dirs {
		why, known := knownUnanchored[dir]

		switch {
		case known && len(failures[dir]) == 0:
			t.Errorf("%s now stands on its foot point -- delete it from knownUnanchored (%s)", dir, why)
		case known:
			t.Logf("%s: KNOWN, %s:\n  %s", dir, why, strings.Join(failures[dir], "\n  "))
		case len(failures[dir]) > 0:
			t.Errorf("%s is drawn off its foot point:\n  %s", dir, strings.Join(failures[dir], "\n  "))
		}
	}

	t.Logf("%d standing sheets measured in %d folders", checked, len(byDir))
}

// measureSheet returns the median, over every frame, of the alpha centroid's x
// and of the lowest opaque row, both in frame pixels.
func measureSheet(path string, m anchorManifest) (centroid, feet float64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return 0, 0, err
	}

	fw, fh := m.FrameWidth, m.FrameHeight
	if fw == 0 || fh == 0 {
		b := img.Bounds()
		fw, fh = b.Dx()/m.FramesPerDirection, b.Dy()/m.Directions
	}

	var cxs, feets []float64

	for d := 0; d < m.Directions; d++ {
		for fr := 0; fr < m.FramesPerDirection; fr++ {
			cell := image.Rect(fr*fw, d*fh, (fr+1)*fw, (d+1)*fh)
			sum, n, low := 0.0, 0, -1

			for y := cell.Min.Y; y < cell.Max.Y; y++ {
				for x := cell.Min.X; x < cell.Max.X; x++ {
					if _, _, _, a := img.At(x, y).RGBA(); a>>8 > anchorAlphaMin {
						sum += float64(x - cell.Min.X)
						n++
						low = y - cell.Min.Y
					}
				}
			}

			if n > 0 {
				cxs = append(cxs, sum/float64(n))
				feets = append(feets, float64(low))
			}
		}
	}

	if len(cxs) == 0 {
		return 0, 0, fmt.Errorf("every frame is transparent")
	}

	return anchorMedian(cxs), anchorMedian(feets), nil
}

func anchorMedian(v []float64) float64 {
	sort.Float64s(v)
	return v[len(v)/2]
}
