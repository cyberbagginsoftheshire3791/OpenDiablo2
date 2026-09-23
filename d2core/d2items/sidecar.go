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

// LoadSidecar reads a hero's kit and binds it to the catalogue.
func LoadSidecar(path string, c *Catalog) (*Kit, error) {
	if path == "" {
		return nil, ErrNoSidecar
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoSidecar
	}

	if err != nil {
		return nil, err
	}

	var sc sidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("kit file %s: %w", path, err)
	}

	if sc.Version != sidecarVersion || sc.Kit == nil {
		return nil, fmt.Errorf("kit file %s: version %d, want %d", path, sc.Version, sidecarVersion)
	}

	sc.Kit.Bind(c)

	return sc.Kit, nil
}

// SaveSidecar writes a hero's kit, atomically: a half-written kit file would
// cost him his gear.
func SaveSidecar(path string, k *Kit) error {
	if path == "" || k == nil {
		return nil
	}

	data, err := json.MarshalIndent(sidecar{Version: sidecarVersion, Kit: k}, "", "  ")
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
