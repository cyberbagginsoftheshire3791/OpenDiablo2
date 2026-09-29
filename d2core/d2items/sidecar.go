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

// SidecarVersion is bumped when the file's shape changes incompatibly. The
// world file checks the sidecar it embeds against this constant (d2save's
// checkSidecar), not a literal of its own (the B3 review, 29 Sep 2026).
const SidecarVersion = 1

type sidecar struct {
	Version int `json:"version"`

	// Generation is the world save this document belongs to: the saved_at of
	// the last N.od2.world.json written with it (M4.6 B3 review, B7). A world
	// save writes its own saved_at here and into the world file; every kit
	// save between world saves carries the same generation forward (the game
	// screen keeps it), so the sidecar and the world file agree until the
	// next world save. After a crash between the world file's write and the
	// sidecar's, they do not, and the load can tell (d2save.World.SameMoment).
	// Empty on a hero no world save has touched.
	Generation string `json:"generation,omitempty"`

	Kit *Kit `json:"kit"`

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

	// Journal is his diary (J1), raw for the same reason. Optional: without
	// it the start entries are written again at once.
	Journal json.RawMessage `json:"journal,omitempty"`
}

// Extras is what rides in the file beside the kit, raw, so this package
// imports neither the progression nor the dialogue package.
type Extras struct {
	Progress json.RawMessage
	Village  json.RawMessage
	Land     json.RawMessage
	Journal  json.RawMessage

	// Generation is the world save this document belongs to (sidecar's
	// Generation): read back by LoadHero, written by HeroBytes.
	Generation string
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

	if sc.Version != SidecarVersion || sc.Kit == nil {
		return nil, Extras{}, fmt.Errorf("kit file %s: version %d, want %d", path, sc.Version, SidecarVersion)
	}

	sc.Kit.Bind(c)

	return sc.Kit, Extras{
		Progress: sc.Progress, Village: sc.Village, Land: sc.Land, Journal: sc.Journal, Generation: sc.Generation,
	}, nil
}

// SaveHero writes a hero's kit and what rides beside it,
// atomically: a half-written file would cost him his gear and his levels.
func SaveHero(path string, k *Kit, x Extras) error {
	if path == "" || k == nil {
		return nil
	}

	data, err := HeroBytes(k, x)
	if err != nil {
		return err
	}

	return WriteHero(path, data)
}

// HeroBytes is the document SaveHero writes: the kit and what rides beside it,
// indented. The world save (M4.6 B3) takes it once and writes the same bytes
// twice -- into the sidecar and, embedded, into the world file -- so the two
// are one moment by construction.
func HeroBytes(k *Kit, x Extras) ([]byte, error) {
	if k == nil {
		return nil, errors.New("no kit to write")
	}

	return json.MarshalIndent(sidecar{
		Version: SidecarVersion, Generation: x.Generation, Kit: k,
		Progress: x.Progress, Village: x.Village, Land: x.Land, Journal: x.Journal,
	}, "", "  ")
}

// WriteHero writes a document HeroBytes made to the sidecar at path,
// atomically.
func WriteHero(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}

	return WriteFileAtomic(path, data)
}

// BakPath is where a save keeps the generation it replaces: path + ".bak"
// (rule 5 of the world save: one save per hero, the last one kept and not
// offered in game). The World Editor's map save keeps its own the same way
// (d2mapedit.BackupPath).
func BakPath(path string) string {
	return path + ".bak"
}

// KeepGeneration copies the file at path, if there is one, to BakPath(path)
// through WriteFileAtomic, before a save replaces it (M4.6 B3: the hero's
// .od2 and the world file). Nothing at path yet is not an error: there is no
// generation to keep.
func KeepGeneration(path string) error {
	old, err := os.ReadFile(path) // nolint:gosec // the hero's own save
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("reading the generation about to be replaced: %w", err)
	}

	if err := WriteFileAtomic(BakPath(path), old); err != nil {
		return fmt.Errorf("keeping the previous generation: %w", err)
	}

	return nil
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
// retried for half a second (RenameRetrying), and if the target stays locked
// the data is written in place: a save that is not atomic beats a save that is
// not made.
//
// THE TEMPORARY FILE IS FLUSHED TO DISK BEFORE THE RENAME (M4.6 B3 review,
// 29 Sep 2026): without it a power cut soon after the rename can leave the
// new name pointing at blocks the disk never received -- an empty or torn
// save where the old one was. One Sync per save is cheap beside the save.
func WriteFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := writeSynced(tmp, data); err != nil {
		return err
	}

	err := RenameRetrying(tmp, path)
	if err == nil {
		return nil
	}

	if werr := os.WriteFile(path, data, 0o600); werr != nil {
		return fmt.Errorf("save %s: rename: %v; direct write: %w", path, err, werr)
	}

	_ = os.Remove(tmp)

	return nil
}

// writeSynced writes data to a new file at path and flushes it to disk
// (File.Sync) before closing it.
func writeSynced(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // nolint:gosec // a save beside the hero's own
	if err != nil {
		return err
	}

	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}

	if cerr := f.Close(); err == nil {
		err = cerr
	}

	return err
}

// RenameRetrying renames from to to, retrying for half a second while the
// rename is refused -- on Windows, while anything else holds either file open
// (WriteFileAtomic's lesson). The world save's setting aside of a file it
// cannot read goes through it too (d2save, the B3 review's C item): the file
// being moved is exactly the kind a reader or a scanner has open.
func RenameRetrying(from, to string) error {
	var err error

	for i := 0; i < renameTries; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}

		time.Sleep(renamePause)
	}

	return err
}
