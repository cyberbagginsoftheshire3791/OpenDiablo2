package d2mapedit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
)

// THE SAVE.
//
//   - It VALIDATES FIRST and refuses to write a file the game would throw away.
//     See validate.go for why that matters more than it sounds: a refused map
//     does not stop the game, it silently swaps in Diablo II's Act 1
//     (d2mapgen/authored.go:34-38).
//   - The previous generation is kept as <path>.bak before anything is
//     overwritten, so one bad save is one undo away even after the editor has
//     been closed.
//   - The write itself goes through d2items.WriteFileAtomic
//     (d2core/d2items/sidecar.go:129), which is REUSED rather than reinvented
//     because it carries a measured Windows lesson: the rename can be refused
//     while anything else holds the target open -- a polling reader, an
//     antivirus scan, the search indexer, Explorer's preview -- so it retries
//     for half a second and then writes in place, because a save that is not
//     atomic beats a save that is not made (sidecar.go:120-128). <path>.tmp is
//     the file in flight, which is the convention already in the tree.
//   - It refuses to write into the player's save namespace. See ErrPlayerSaves.

// ErrPlayerSaves means the path is inside the directory the game keeps heroes
// in (%APPDATA%\OpenDiablo2\Saves, d2hero/hero_state_factory.go:287-293). A map
// does not belong there: the hero screen lists and rewrites what it finds, so a
// stray file in that directory is a file somebody else owns.
var ErrPlayerSaves = errors.New("that is the player's save directory (OpenDiablo2/Saves); a map does not go there")

// Save validates the document and writes it to path.
func (d *Doc) Save(path string, art Art) error {
	if problems := d.Validate(art); len(problems) > 0 {
		return &RefusedError{Problems: problems}
	}

	data, err := d.Bytes()
	if err != nil {
		return err
	}

	if err := guardPlayerSaves(path); err != nil {
		return err
	}

	if err := backup(path); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}

	return d2items.WriteFileAtomic(path, data)
}

// BackupPath is where Save keeps the generation it is about to overwrite.
func BackupPath(path string) string {
	return path + ".bak"
}

// CorruptPath is where SetAside puts a file that cannot be parsed, the
// convention the kit screen already uses (d2game/d2gamescreen/kit.go:60-62).
func CorruptPath(path string) string {
	return path + ".corrupt"
}

// backup copies the file about to be overwritten to BackupPath. A path with
// nothing there yet has no generation to keep, which is not an error.
func backup(path string) error {
	old, err := os.ReadFile(path) // nolint:gosec // the designer names the file
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("reading the generation about to be overwritten: %w", err)
	}

	if err := d2items.WriteFileAtomic(BackupPath(path), old); err != nil {
		return fmt.Errorf("keeping the previous generation: %w", err)
	}

	return nil
}

// SetAside moves a file that cannot be parsed out of the way, to CorruptPath,
// and returns where it went. It is what an editor calls when the designer has
// been told his file is unreadable and has said to put it aside; OpenFile does
// not do it on its own, because a map is somebody's week of work and a kit is
// not.
func SetAside(path string) (string, error) {
	aside := CorruptPath(path)

	if err := guardPlayerSaves(path); err != nil {
		return "", err
	}

	if err := os.Rename(path, aside); err != nil {
		return "", err
	}

	return aside, nil
}

// guardPlayerSaves refuses a path in or under the hero save directory.
func guardPlayerSaves(p string) error {
	saves, err := playerSavesDir()
	if err != nil || saves == "" {
		// Nowhere to defend: a machine with no config directory has no save
		// directory to stray into either.
		return nil
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}

	if under(filepath.Clean(abs), filepath.Clean(saves)) {
		return fmt.Errorf("%s: %w", p, ErrPlayerSaves)
	}

	return nil
}

// under reports whether p is dir or sits inside it. Case is ignored on Windows,
// where %APPDATA% comes back in whatever case the environment holds and
// C:\Users\x\AppData and c:\users\X\appdata are the same directory -- a
// case-sensitive comparison would let a stray map into the save namespace by
// the back door.
func under(p, dir string) bool {
	if runtime.GOOS == "windows" {
		p, dir = strings.ToLower(p), strings.ToLower(dir)
	}

	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// playerSavesDir is the game's own save path
// (d2hero/hero_state_factory.go:287-293), read the same way so the two cannot
// point at different directories.
func playerSavesDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "OpenDiablo2", "Saves"), nil
}
