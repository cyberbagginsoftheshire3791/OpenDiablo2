package d2items

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// renameBackoff is how long RenameRetrying waits after each refused rename:
// twenty short waits first (half a second: a reader's poll, Explorer's
// preview, a scanner's glance -- the 23 Sep measurement's holders), then four
// longer ones, doubling (a second and a half more: an antivirus scan of the
// whole file). Two seconds in all; then the refusal is the write's failure
// (the M4.6 B5 review, B1).
//
// nolint:gochecknoglobals // a unit test shortens it
var renameBackoff = func() []time.Duration {
	pauses := make([]time.Duration, 0, 24)

	for i := 0; i < 20; i++ {
		pauses = append(pauses, 25*time.Millisecond)
	}

	for d := 100 * time.Millisecond; d <= 800*time.Millisecond; d *= 2 {
		pauses = append(pauses, d)
	}

	return pauses
}()

// renameFile is os.Rename; a unit test swaps it for a rename that is refused
// (a file held open, which only Windows refuses a rename for) or one that
// blocks (a hung disk).
//
// nolint:gochecknoglobals // a seam for the failures a unit test cannot cause portably
var renameFile = os.Rename

// WriteFileAtomic writes data to path through a temporary file and a rename,
// so a crash mid-write never leaves half a file.
//
// ON WINDOWS THE RENAME CAN BE REFUSED ("Access is denied") while anything else
// has the target open -- measured 23 Sep 2026: the kit playtest's polling
// reader, and in the field an antivirus scan, the search indexer or Explorer's
// preview would do the same. The save was simply lost. So the rename is
// retried (RenameRetrying: half a second of short waits, then longer ones,
// two seconds in all).
//
// IT NEVER WRITES IN PLACE (the M4.6 B5 review, B1; BUG-99). Until the review
// a target that stayed locked was written in place with os.WriteFile -- "a
// save that is not atomic beats a save that is not made". The world save
// changed that trade: a save is three files that are one moment, and a
// process that leaves in the middle of an in-place write (the window's close
// under its limit, a crash, a power cut) leaves half a file where his save
// was -- a world file no load reads, or an .od2 that no longer opens. A
// refused rename is now the write's failure, and the save that asked for it
// deals with that whole: the world save puts its world file back as it was
// (Game.SaveWorld's rollback), so his last save stands; a kit save is made
// again by the next one. The retry is what the in-place write was for, and
// stays; only the fallback is gone.
//
// THE TEMPORARY FILE IS FLUSHED TO DISK BEFORE THE RENAME (M4.6 B3 review,
// 29 Sep 2026): without it a power cut soon after the rename can leave the
// new name pointing at blocks the disk never received -- an empty or torn
// save where the old one was. One Sync per save is cheap beside the save.
//
// Once the close's limit has passed (CutWrites) no write begins at all.
//
// A write that fails leaves no temporary file behind (the B5 review's probe
// B found a stray N.od2.tmp beside a read-only .od2).
func WriteFileAtomic(path string, data []byte) error {
	if err := beginWrite(path); err != nil {
		return err
	}

	defer endWrite()

	tmp := path + ".tmp"
	if err := writeSynced(tmp, data); err != nil {
		_ = os.Remove(tmp)

		return err
	}

	if err := RenameRetrying(tmp, path); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("save %s: the rename was refused for %v, and a save is never written in place: %w",
			path, renameBackoffTotal(), err)
	}

	return nil
}

// renameBackoffTotal is how long RenameRetrying waits in all.
func renameBackoffTotal() time.Duration {
	var total time.Duration

	for _, d := range renameBackoff {
		total += d
	}

	return total
}

// THE CLOSE'S WRITES (the M4.6 B5 review, B1; BUG-99). The window's close
// saves the game under a limit (d2app's closeLimit): a hung disk -- an
// antivirus scan, a sync client -- must not hang the close, so past the limit
// the process leaves without the hook. Two rules make that safe for the
// files:
//
//   - No write is ever made in place (WriteFileAtomic). Every file is a
//     temporary file and a rename, and a rename is whole or not at all, so a
//     process that leaves at ANY instant leaves each file as it was or as it
//     became -- never half of one.
//   - CutWrites: when the limit passes, no write BEGINS, and the close waits
//     (a grace longer than one write's retries) for the one in flight to
//     finish, so the close gives up between files, never inside one. The
//     save's order (the world file, his .od2, his sidecar) and the load's
//     fallback to the world file's .bak cover a cut between files
//     (d2gamescreen: SaveWorld's rollback, the load's TORN fallback).
//
// The cut is process-wide and one-way in the game: a closing process never
// writes again. uncutWrites is the unit tests' reset.

// ErrWritesCut is a write asked for after the close's limit passed.
var ErrWritesCut = errors.New("the game is closing and its time is up: no file is written from here")

// nolint:gochecknoglobals // one process, one close
var writes struct {
	sync.Mutex
	cut      bool
	inFlight int
}

// CutWrites is the close's limit reached: from now on no WriteFileAtomic
// begins (ErrWritesCut), and it waits up to grace for the writes in flight to
// end. finished is false when one was still running at the end of the grace
// (a disk that hangs past it: the process leaves in the middle of that
// write's temporary file, and the file it would have replaced is whole).
func CutWrites(grace time.Duration) (finished bool) {
	writes.Lock()
	writes.cut = true
	writes.Unlock()

	deadline := time.Now().Add(grace)

	for {
		writes.Lock()
		n := writes.inFlight
		writes.Unlock()

		if n == 0 {
			return true
		}

		if time.Now().After(deadline) {
			return false
		}

		time.Sleep(5 * time.Millisecond)
	}
}

// uncutWrites undoes CutWrites. The game never calls it (a closing process
// ends); a unit test does, so the next test can write.
func uncutWrites() {
	writes.Lock()
	defer writes.Unlock()

	writes.cut = false
}

// beginWrite counts a write in flight, or refuses it after the cut.
func beginWrite(path string) error {
	writes.Lock()
	defer writes.Unlock()

	if writes.cut {
		return fmt.Errorf("save %s: %w", path, ErrWritesCut)
	}

	writes.inFlight++

	return nil
}

// endWrite is a write done.
func endWrite() {
	writes.Lock()
	defer writes.Unlock()

	writes.inFlight--
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

// RenameRetrying renames from to to, retrying while the rename is refused --
// on Windows, while anything else holds either file open (WriteFileAtomic's
// lesson) -- for two seconds, the waits growing (renameBackoff). The world
// save's setting aside of a file it cannot read goes through it too (d2save,
// the B3 review's C item): the file being moved is exactly the kind a reader
// or a scanner has open.
func RenameRetrying(from, to string) error {
	err := renameFile(from, to)
	if err == nil {
		return nil
	}

	for _, pause := range renameBackoff {
		time.Sleep(pause)

		if err = renameFile(from, to); err == nil {
			return nil
		}
	}

	return err
}
