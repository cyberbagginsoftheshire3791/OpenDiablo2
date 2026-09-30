package d2gamescreen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
)

// HIS SIDECAR BEFORE A LOAD WRITES OVER IT (the B4a review, 29 Sep 2026: A1,
// BUG-60). A load's step 1 writes the world file's copy of his sidecar over
// his own before the game opens -- the kit, progress, standing, land and
// journal of the saved moment -- and a load can still be refused after that
// (the seed, the map, the villagers, a block). A refusal used to leave the
// world file's copy in place, so the dawn he fell back to had lost everything
// he had earned since the save, and his .od2 was of the later moment. So step
// 1 KEEPS HIS OWN FIRST:
//
//   - in memory (held, below), for the refusal in this process to put back
//     byte for byte (SetLoadAside calls restorePreload for every refusal; the
//     App calls RestorePreload when the game never opened at all);
//   - as N.od2.strigoi.json.preload, flushed to disk, for a crash between the
//     write and the load's end: the next start of that hero undoes the load
//     before anything reads his sidecar (recoverPreload, from PrepareLoad).
//
// A load that resumes lets the copy go (forgetPreload, from loadResumed).

// preloadSuffix is what the copy adds to his sidecar's path.
const preloadSuffix = ".preload"

// preloadPath is where step 1 keeps his sidecar: N.od2.strigoi.json.preload.
func preloadPath(savePath string) string {
	return d2items.SidecarPath(savePath) + preloadSuffix
}

// nolint:gochecknoglobals // one load at a time, as lastLoad
var held struct {
	sync.Mutex
	savePath string
	data     []byte
	kept     bool
}

// keepPreload keeps his sidecar as step 1 read it, before step 1 writes over
// it: in memory, and flushed to disk beside it (d2items.WriteFileAtomic).
func keepPreload(savePath string, sidecar []byte) error {
	if err := d2items.WriteFileAtomic(preloadPath(savePath), sidecar); err != nil {
		return err
	}

	held.Lock()
	defer held.Unlock()

	held.savePath, held.data, held.kept = savePath, append([]byte{}, sidecar...), true

	return nil
}

// restorePreload puts back the sidecar step 1 kept for savePath, byte for
// byte, and removes the copy on disk. It reports whether it put one back; a
// load that kept none (a refusal at step 1 before the write, a network game,
// another hero's load) restores nothing. A write that fails leaves the copy on
// disk and in memory, and says so.
func restorePreload(savePath string) (bool, error) {
	held.Lock()
	defer held.Unlock()

	if !held.kept || held.savePath != savePath || savePath == "" {
		return false, nil
	}

	if err := d2items.WriteHero(d2items.SidecarPath(savePath), held.data); err != nil {
		return false, err
	}

	held.savePath, held.data, held.kept = "", nil, false

	// His sidecar is back. A copy that will not go is judged at the next
	// start (recoverPreload) and never put over a sidecar that has moved on.
	_ = removeRetrying(preloadPath(savePath))

	return true, nil
}

// RestorePreload is restorePreload for the App: a load prepared and never
// opened -- the client could not be made, the game could not be joined, or
// CreateGame failed for a reason that is not a load's refusal -- puts his own
// sidecar back, as a refusal does (A1).
func RestorePreload(savePath string) {
	restored, err := restorePreload(savePath)

	updateLastLoad(func(r *LoadReport) {
		if restored {
			r.Preload = "restored"
		}

		if err != nil {
			r.Reason += fmt.Sprintf(" (and his own sidecar could not be put back: %v -- it is kept as %s)", err, preloadPath(savePath))
		}
	})
}

// forgetPreload lets the copy go: the load resumed, and the world file's
// moment is his now.
func forgetPreload(savePath string) {
	held.Lock()
	defer held.Unlock()

	if held.savePath == savePath {
		held.savePath, held.data, held.kept = "", nil, false
	}

	if savePath != "" {
		_ = removeRetrying(preloadPath(savePath))
	}
}

// recoverPreload is the next start after a crash (A1): a copy left beside his
// sidecar means a load wrote over it and never ended. It says what it did:
//
//   - "recovered": his sidecar is still exactly what step 1 wrote from the
//     world file beside it, and the copy is of that file's moment (its
//     generation is the file's saved_at, which step 1 required) -- the load
//     was cut off, and is undone: the copy goes back, byte for byte. So it
//     does when his sidecar is not a document at all (a write cut off).
//   - "discarded": the world file beside it is of a later save than the copy
//     (a world save since has written both files again): the copy is older
//     than everything on disk, and is removed.
//   - "kept": anything else -- his sidecar moved on after the write (a game
//     resumed, and its saves), or no world file can be read to judge by. The
//     copy is never put over a sidecar that may be newer: it is moved to
//     .preload.kept, never offered, never deleted, for a person to judge.
//   - "": no copy.
func recoverPreload(savePath string) string {
	held.Lock()
	held.savePath, held.data, held.kept = "", nil, false
	held.Unlock()

	copyPath := preloadPath(savePath)

	kept, err := os.ReadFile(copyPath) // nolint:gosec // the hero's own save
	if err != nil {
		return ""
	}

	sidecarPath := d2items.SidecarPath(savePath)
	now, nowErr := os.ReadFile(sidecarPath) // nolint:gosec // the hero's own save

	put := func() string {
		if err := d2items.WriteHero(sidecarPath, kept); err != nil {
			return keepAside(copyPath)
		}

		_ = removeRetrying(copyPath)

		return "recovered"
	}

	if nowErr != nil || !json.Valid(now) {
		return put()
	}

	data, err := os.ReadFile(d2save.WorldPath(savePath)) // nolint:gosec // the hero's own save
	if err != nil {
		return keepAside(copyPath)
	}

	w, err := d2save.Decode(data)
	if err != nil {
		return keepAside(copyPath)
	}

	var gen struct {
		Generation string `json:"generation"`
	}

	if json.Unmarshal(kept, &gen) == nil && gen.Generation != w.SavedAt {
		_ = removeRetrying(copyPath)

		return "discarded"
	}

	if written, err := stepOneSidecar(w); err == nil && bytes.Equal(now, written) {
		return put()
	}

	return keepAside(copyPath)
}

// keepAside moves a copy that cannot be judged to .preload.kept (over an older
// one: the newest unjudged copy is the one worth a person's look).
func keepAside(copyPath string) string {
	// If even the move fails the copy stays where it is, which is also kept.
	_ = d2items.RenameRetrying(copyPath, copyPath+".kept")

	return "kept"
}

// removeRetrying removes a file, retrying for half a second while Windows
// refuses it (a reader or a scanner holding it, WriteFileAtomic's lesson).
func removeRetrying(path string) error {
	var err error

	for i := 0; i < 20; i++ {
		if err = os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		}

		time.Sleep(25 * time.Millisecond)
	}

	return err
}

// A FILE REFUSED AND NOT SET ASIDE IS NOT READ AGAIN IN THIS PROCESS (the B4a
// review, B1; BUG-62). A world file refused after its game opened is set aside
// and the dawn opened in its place; when the move failed (the file held open:
// an editor, a scanner, a backup tool), the dawn found the same file, refused
// it the same way, tore down and reloaded -- measured 51 teardowns in 25 s and
// never a game. Now the refusal is remembered by the file's path, size and
// modification time, and no load in this process reads that file again; a
// file written since (a save, a copy put back) is another file and is read.

type ignoredFile struct {
	size    int64
	modTime time.Time
	refusal LoadRefusal
}

// nolint:gochecknoglobals // this process's memory of files it could not move
var ignored struct {
	sync.Mutex
	files map[string]ignoredFile
}

// ignoreFromNowOn remembers a refused file that could not be set aside.
func ignoreFromNowOn(worldPath string, refusal *LoadRefusal) {
	info, err := os.Stat(worldPath)
	if err != nil {
		return
	}

	ignored.Lock()
	defer ignored.Unlock()

	if ignored.files == nil {
		ignored.files = map[string]ignoredFile{}
	}

	ignored.files[worldPath] = ignoredFile{size: info.Size(), modTime: info.ModTime(), refusal: *refusal}
}

// ignoredRefusal is the refusal a file at worldPath was given earlier in this
// process when it could not be set aside, if it is still that file; or nil.
func ignoredRefusal(worldPath string) *LoadRefusal {
	info, err := os.Stat(worldPath)
	if err != nil {
		return nil
	}

	ignored.Lock()
	defer ignored.Unlock()

	f, ok := ignored.files[worldPath]
	if !ok || f.size != info.Size() || !f.modTime.Equal(info.ModTime()) {
		return nil
	}

	r := f.refusal
	r.Detail = fmt.Sprintf("%s (refused earlier in this run and could not be set aside: not read again until it changes)", f.refusal.Detail)

	return &r
}

// forgetIgnored is a refused file that was set aside after all (a save's step
// 0, the M4.6 B5 review, B2): nothing at its path is that file any more.
func forgetIgnored(worldPath string) {
	ignored.Lock()
	defer ignored.Unlock()

	delete(ignored.files, worldPath)
}
