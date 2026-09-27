package d2player

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// handsDir is an empty data folder and an asset manager reading it.
func handsDir(t *testing.T) (string, *d2asset.AssetManager) {
	t.Helper()

	// Not t.TempDir: its cleanup fails the test on Windows while the loader
	// still holds a file.
	dir, err := os.MkdirTemp("", "hands")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	am, err := d2asset.NewAssetManager(d2util.LogLevelNone)
	if err != nil {
		t.Fatal(err)
	}

	if err := am.AddSource(dir, types.AssetSourceFileSystem); err != nil {
		t.Fatal(err)
	}

	return dir, am
}

// dropArt writes a w x h PNG at the data path p under dir, and a manifest
// beside it when one is given.
func dropArt(t *testing.T, dir, p string, w, h int, manifest string) {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, h/2, color.NRGBA{R: 200, G: 160, B: 90, A: 255})
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	full := filepath.Join(dir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(full, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	if manifest != "" {
		if err := os.WriteFile(full+".json", []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// A hand draws its art when the file is there and fits, and its key's letter
// otherwise -- no art, or art that does not fit (27 Sep 2026: the hands' art
// drops in with no code). The torch's frame follows its lit state.
//
// Negative control (27 Sep 2026): make handArt answer nil whatever is there
// and the art halves fail -- the blade draws "F".
func TestAHandDrawsItsArtWhenThereIsSome(t *testing.T) {
	dir, am := handsDir(t)

	hand := func(name, path string, frames int) *handIcon {
		t.Helper()

		art, err := handArt(am, path, frames)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		return &handIcon{name: name, art: art}
	}

	// No art: the letter.
	if drew, _ := hand("blade", BladeArtPath, 1).choose("F"); drew != "F" {
		t.Fatalf("with no blade art the left hand draws %q; want its key's letter F", drew)
	}

	// The blade dropped in: one 48x48 frame.
	dropArt(t, dir, BladeArtPath, 48, 48, "")

	if drew, frame := hand("blade", BladeArtPath, 1).choose("F"); drew != "art:blade" || frame != 0 {
		t.Fatalf("with blade.png there the left hand draws %q frame %d; want art:blade frame 0", drew, frame)
	}

	// The torch: two frames, unlit then lit, and the frame follows the flame.
	dropArt(t, dir, TorchArtPath, 96, 48, `{"directions": 1, "frames_per_direction": 2}`)

	torch := hand("torch", TorchArtPath, 2)
	lit := false
	torch.frame = func() int {
		if lit {
			return torchFrameLit
		}

		return torchFrameUnlit
	}

	if drew, frame := torch.choose("L"); drew != "art:torch" || frame != torchFrameUnlit {
		t.Fatalf("an unlit torch draws %q frame %d; want art:torch frame 0", drew, frame)
	}

	lit = true

	if _, frame := torch.choose("L"); frame != torchFrameLit {
		t.Fatalf("a lit torch draws frame %d; want 1", frame)
	}

	if err := torch.art.SetCurrentFrame(torchFrameLit); err != nil {
		t.Fatalf("the torch art has no lit frame to draw: %v", err)
	}
}

// Art that is there and does not fit is refused -- reported, and the letter
// drawn -- rather than drawn over the globes.
func TestHandArtThatDoesNotFitIsRefused(t *testing.T) {
	for name, c := range map[string]struct {
		path     string
		frames   int
		w, h     int
		manifest string
	}{
		"a blade the wrong size":         {BladeArtPath, 1, 64, 64, ""},
		"a torch sheet with no manifest": {TorchArtPath, 2, 96, 48, ""},
		"a torch of one frame":           {TorchArtPath, 2, 48, 48, ""},
		"a torch sheet the wrong size": {TorchArtPath, 2, 128, 64,
			`{"directions": 1, "frames_per_direction": 2}`},
	} {
		dir, am := handsDir(t)
		dropArt(t, dir, c.path, c.w, c.h, c.manifest)

		art, err := handArt(am, c.path, c.frames)
		if art != nil || err == nil {
			t.Errorf("%s: loaded %v, err %v; want it refused with a reason", name, art != nil, err)
		}

		if drew, _ := (&handIcon{name: "x", art: art}).choose("F"); drew != "F" {
			t.Errorf("%s: a refused hand draws %q; want its letter", name, drew)
		}
	}
}
