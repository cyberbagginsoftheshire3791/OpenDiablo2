package d2items

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
}

// Extras is what rides in the file beside the kit, raw, so this package
// imports neither the progression nor the dialogue package.
type Extras struct {
	Progress json.RawMessage
	Village  json.RawMessage
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

	return sc.Kit, Extras{Progress: sc.Progress, Village: sc.Village}, nil
}

// SaveHero writes a hero's kit and what rides beside it,
// atomically: a half-written file would cost him his gear and his levels.
func SaveHero(path string, k *Kit, x Extras) error {
	if path == "" || k == nil {
		return nil
	}

	data, err := json.MarshalIndent(sidecar{Version: sidecarVersion, Kit: k, Progress: x.Progress, Village: x.Village}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}
