//go:build harness

package d2app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// M4.6 B3 REVIEW, B5: strigoi_save_game's to has a fence. A relative to used
// to resolve in the game's working directory -- the playtest launcher sets it
// to the repository -- and a file there was moved to .unread and replaced.
// Now to must be under %AppData% (the test's private home under the
// playtest) or a temporary folder, and never inside a source tree of this
// game; the path the save gets is absolute.
func TestTheSavesToStaysOutOfTheSourceTree(t *testing.T) {
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)

	refused := []string{
		"relative.world.json", // the working directory is d2app, in the tree
		filepath.Join(repo, "docs", "copy.world.json"),
		filepath.Join(repo, "go.mod"),
		filepath.Join(filepath.VolumeName(repo)+string(filepath.Separator), "b3-nowhere", "copy.world.json"),
	}

	for _, to := range refused {
		if got, err := harnessSaveTo(to); err == nil {
			t.Errorf("to %q was allowed (as %s)", to, got)
		}
	}

	allowed := []string{
		filepath.Join(t.TempDir(), "copy.world.json"),
		filepath.Join(home, "OpenDiablo2", "Saves", "copy.world.json"),
	}

	for _, to := range allowed {
		got, err := harnessSaveTo(to)
		if err != nil {
			t.Errorf("to %q was refused: %v", to, err)

			continue
		}

		if !filepath.IsAbs(got) || !strings.EqualFold(got, filepath.Clean(to)) {
			t.Errorf("to %q came back as %q", to, got)
		}
	}

	// The instrument: the temporary folder and the home are not in a tree.
	if tree := harnessSourceTree(filepath.Join(os.TempDir(), "x")); tree != "" {
		t.Fatalf("the temporary folder is inside the source tree %s", tree)
	}
}
