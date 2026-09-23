package d2items

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// THE KIT IS SAVED BESIDE THE HERO, NOT INSIDE HIM. A hero's .od2 save belongs
// to OpenDiablo2's networking layer -- it reaches the game screen through the
// local server's add-player packet -- and threading Strigoi's kit through five
// upstream files for a single-player game is the wrong trade. The sidecar sits
// next to the exact save the game opened (N.od2 -> N.od2.strigoi.json), which
// is unique per hero: keying it by hero NAME would let one test run's worn
// mail leak into the next hero with the same name.

// sidecarVersion is bumped when the file's shape changes incompatibly.
const sidecarVersion = 1

type sidecar struct {
	Version int  `json:"version"`
	Kit     *Kit `json:"kit"`

	// Progress is the hero's standing (T3), carried here as raw JSON so this
	// package does not import the progression package. Optional: a T2 file
	// without it loads as a hero with no experience.
	Progress json.RawMessage `json:"progress,omitempty"`

	// Village is what the village thinks of him (T4), raw for the same
	// reason. Optional: without it he is a stranger at the gate.
	Village json.RawMessage `json:"village,omitempty"`

	// Land is what the country around still holds for him to gather (T7):
	// S1 §8.3's finite local resources, which do not regenerate in the run.
	Land json.RawMessage `json:"land,omitempty"`
}

// Extras is what rides in the file beside the kit, raw, so this package
// imports neither the progression nor the dialogue package.
type Extras struct {
	Progress json.RawMessage
	Village  json.RawMessage
	Land     json.RawMessage
}

// SidecarPath is the kit file for a hero save.
func SidecarPath(savePath string) string {
	if savePath == "" {
		return ""
	}

	return savePath + ".strigoi.json"
}

// ErrNoSidecar means the hero has never chosen a loadout.
var ErrNoSidecar = errors.New("no kit saved for this hero")

// LoadHero reads a hero's kit and what rides beside it (raw; nil when none).
func LoadHero(path string, c *Catalog) (*Kit, Extras, error) {
	if path == "" {
		return nil, Extras{}, ErrNoSidecar
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, Extras{}, ErrNoSidecar
	}

	if err != nil {
		return nil, Extras{}, err
	}

	var sc sidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, Extras{}, fmt.Errorf("kit file %s: %w", path, err)
	}

	if sc.Version != sidecarVersion || sc.Kit == nil {
		return nil, Extras{}, fmt.Errorf("kit file %s: version %d, want %d", path, sc.Version, sidecarVersion)
	}

	sc.Kit.Bind(c)

	return sc.Kit, Extras{Progress: sc.Progress, Village: sc.Village, Land: sc.Land}, nil
}

// SaveHero writes a hero's kit and what rides beside it,
// atomically: a half-written file would cost him his gear and his levels.
func SaveHero(path string, k *Kit, x Extras) error {
	if path == "" || k == nil {
		return nil
	}

	data, err := json.MarshalIndent(sidecar{Version: sidecarVersion, Kit: k, Progress: x.Progress, Village: x.Village, Land: x.Land}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}

	return WriteFileAtomic(path, data)
}

// Rename retries: how many, and how long between.
const (
	renameTries = 20
	renamePause = 25 * time.Millisecond
)

// WriteFileAtomic writes data to path through a temporary file and a rename,
// so a crash mid-write never leaves half a file.
//
// ON WINDOWS THE RENAME CAN BE REFUSED ("Access is denied") while anything else
// has the target open -- measured 23 Sep 2026: the kit playtest's polling
// reader, and in the field an antivirus scan, the search indexer or Explorer's
// preview would do the same. The save was simply lost. So the rename is
// retried for half a second, and if the target stays locked the data is
// written in place: a save that is not atomic beats a save that is not made.
func WriteFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}

	var err error

	for i := 0; i < renameTries; i++ {
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}

		time.Sleep(renamePause)
	}

	if werr := os.WriteFile(path, data, 0o600); werr != nil {
		return fmt.Errorf("save %s: rename: %v; direct write: %w", path, err, werr)
	}

	_ = os.Remove(tmp)

	return nil
}
