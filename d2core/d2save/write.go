package d2save

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
)

// worldSuffix is what the world file adds to the hero's save path, as the
// sidecar adds ".strigoi.json" (d2items.SidecarPath).
const worldSuffix = ".world.json"

// WorldPath is the world file beside a hero save: N.od2 -> N.od2.world.json.
// "" for "" (a remote client has no save).
func WorldPath(savePath string) string {
	if savePath == "" {
		return ""
	}

	return savePath + worldSuffix
}

// UnreadPath is where a world file this build cannot read is set aside: the
// version it holds in the name when it holds a whole number (N.od2.world.json
// .v2.unread), and plain ".unread" when it holds none this build can name
// (a string, a float, no version, not JSON at all).
func UnreadPath(path, version string) string {
	if wholeNumber.MatchString(version) {
		return path + ".v" + version + ".unread"
	}

	return path + ".unread"
}

var wholeNumber = regexp.MustCompile(`^-?[0-9]+$`)

// Written says what WriteWorld did.
type Written struct {
	// Path is the file written.
	Path string `json:"path"`

	// Bak is where the previous generation was kept (Path + ".bak"), or ""
	// when there was none -- no file yet, or one this build could not read.
	Bak string `json:"bak,omitempty"`

	// SetAside is where a file this build cannot read was moved before the
	// write (rule 7), or "".
	SetAside string `json:"set_aside,omitempty"`
}

// WriteWorld writes data to path so that a crash never leaves half a file,
// keeping what was there:
//
//   - a version-1 file already at path that this build READS (Decode takes
//     it) is the previous save: it is copied to path + ".bak" first (rule 5:
//     one save per hero, the last kept as .bak and not offered in game);
//   - a file of any other version, one that is not a world file at all, or a
//     version-1 file Decode refuses, is one this build cannot read, and rule 7
//     says it is never overwritten: it is moved aside to UnreadPath -- under a
//     name nothing else holds, so a second such file never overwrites the
//     first -- and .bak is left alone. (A bad version-1 file used to be
//     copied over the good .bak, so the one save a load could fall back on
//     was replaced by one it could not read: the B3 review's C item.)
//   - then data goes in through d2items.WriteFileAtomic (a temporary file,
//     flushed, and a rename, retried while Windows holds the target open).
//
// It writes the bytes it is given and checks nothing about them: SaveWorld
// decodes what it is about to write, and a negative control's file with a
// block omitted is meant to be one no load reads.
func WriteWorld(path string, data []byte) (Written, error) {
	if path == "" {
		return Written{}, errors.New("d2save: no path to write the world file to")
	}

	out := Written{Path: path}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return out, err
	}

	old, err := os.ReadFile(path) // nolint:gosec // the hero's own save
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return out, fmt.Errorf("d2save: reading the file about to be replaced: %w", err)
	case VersionOf(old) == strconv.Itoa(Version) && readable(old):
		if err := d2items.KeepGeneration(path); err != nil {
			return out, err
		}

		out.Bak = d2items.BakPath(path)
	default:
		aside, err := setAside(path, VersionOf(old))
		if err != nil {
			return out, err
		}

		out.SetAside = aside
	}

	if err := d2items.WriteFileAtomic(path, data); err != nil {
		return out, err
	}

	return out, nil
}

// readable reports whether a version-1 file is one this build reads: the
// only kind WriteWorld keeps as the .bak.
func readable(data []byte) bool {
	_, err := Decode(data)

	return err == nil
}

// SetAside moves a world file the LOAD refuses out of the way (rule 7: "a save
// this build cannot read ... is neither opened nor overwritten. You are told
// why, it is set aside, and you begin at dawn"), and says where it went. The
// name carries the version the file holds, as WriteWorld's setting aside does
// -- N.od2.world.json.v1.unread for a version-1 file the load refused (another
// hero's, a torn save, a changed map), .v2.unread for a newer build's -- under
// a numbered suffix if that name is taken, so nothing set aside is ever
// overwritten (M4.6 B4a). The load's step 1 and its teardown call it.
//
// A FILE THAT CANNOT BE READ IS STILL MOVED (the B4a review, A2): the load
// refuses it FILE and sets it aside like any other, under plain ".unread" --
// its version cannot be named. Only a file that is not there is an error
// before the move.
func SetAside(path string) (string, error) {
	data, err := os.ReadFile(path) // nolint:gosec // the hero's own save
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("d2save: reading the world file to set it aside: %w", err)
	case err != nil:
		return setAside(path, "")
	}

	return setAside(path, VersionOf(data))
}

// setAside moves the file at path to UnreadPath, or to the first of
// UnreadPath + ".1", ".2", ... that nothing holds. The move is retried while
// Windows refuses it (d2items.RenameRetrying): a file being set aside is
// exactly the kind a reader or a scanner has open (the B3 review's C item).
func setAside(path, version string) (string, error) {
	base := UnreadPath(path, version)
	aside := base

	for n := 1; ; n++ {
		if _, err := os.Stat(aside); errors.Is(err, os.ErrNotExist) {
			break
		} else if err != nil {
			return "", err
		}

		aside = base + "." + strconv.Itoa(n)
	}

	if err := d2items.RenameRetrying(path, aside); err != nil {
		return "", fmt.Errorf("d2save: setting aside %s, which this build cannot read: %w", path, err)
	}

	return aside, nil
}
