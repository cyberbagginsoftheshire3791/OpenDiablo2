package d2items

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The M4.6 B5 review, B1 (BUG-99): a save is never written in place, and the
// close's limit falls between files. Seams, not Windows: a rename refused (a
// file held open, which only Windows refuses a rename for) and one that hangs
// (a disk that does) are swapped in for os.Rename, so the tests run the same
// on the gate's machine and in CI.

// b5rShortBackoff makes RenameRetrying's waits a millisecond each, for a test.
func b5rShortBackoff(t *testing.T) {
	t.Helper()

	was := renameBackoff
	renameBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

	t.Cleanup(func() { renameBackoff = was })
}

// b5rRename swaps os.Rename for f, for a test.
func b5rRename(t *testing.T, f func(from, to string) error) {
	t.Helper()

	was := renameFile
	renameFile = f

	t.Cleanup(func() { renameFile = was })
}

// A RENAME THAT STAYS REFUSED FAILS THE WRITE, AND THE FILE IS AS IT WAS. Before
// the review it was written in place (os.WriteFile), and a process that left
// in the middle of that left half a file where his save was. The control: a
// rename refused twice and then let through lands, retried.
func TestAWriteIsNeverMadeInPlace(t *testing.T) {
	b5rShortBackoff(t)

	path := filepath.Join(t.TempDir(), "0.od2")
	if err := os.WriteFile(path, []byte("the last save"), 0o600); err != nil {
		t.Fatal(err)
	}

	tries := 0

	b5rRename(t, func(from, to string) error {
		tries++
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: errors.New("Access is denied")}
	})

	err := WriteFileAtomic(path, []byte("half of a new save"))
	if err == nil {
		t.Fatal("a rename refused on every try must fail the write, not write the file in place")
	}

	if got, _ := os.ReadFile(path); string(got) != "the last save" {
		t.Fatalf("a refused write changed the file: %q", got)
	}

	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused write left its temporary file behind (%v)", err)
	}

	if want := 1 + len(renameBackoff); tries != want {
		t.Fatalf("the rename was tried %d times, want %d (once, then once after each wait)", tries, want)
	}

	// THE CONTROL: refused twice, then let through -- the retry lands it.
	tries = 0

	b5rRename(t, func(from, to string) error {
		tries++
		if tries <= 2 {
			return errors.New("Access is denied")
		}

		return os.Rename(from, to)
	})

	if err := WriteFileAtomic(path, []byte("the new save")); err != nil {
		t.Fatalf("a rename refused twice and then let through must land: %v", err)
	}

	if got, _ := os.ReadFile(path); string(got) != "the new save" {
		t.Fatalf("the retried write holds %q", got)
	}
}

// THE CLOSE'S LIMIT FALLS BETWEEN FILES. At the cut no write begins -- one asked
// for after it fails (ErrWritesCut) and touches nothing -- and the one in
// flight is waited for, up to the grace, so it ends whole; a write that hangs
// past the grace is reported (finished false), and the file it would have
// replaced is whole, because nothing is ever written in place.
func TestTheCloseCutsWritesBetweenFiles(t *testing.T) {
	b5rShortBackoff(t)
	t.Cleanup(uncutWrites)

	dir := t.TempDir()
	first, second := filepath.Join(dir, "0.od2.world.json"), filepath.Join(dir, "0.od2")

	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("the last save"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// A write in flight whose rename is slow (a disk that is slow, not hung).
	entered, release := make(chan struct{}), make(chan struct{})

	var enter sync.Once

	b5rRename(t, func(from, to string) error {
		enter.Do(func() { close(entered) })
		<-release

		return os.Rename(from, to)
	})

	done := make(chan error, 1)

	go func() { done <- WriteFileAtomic(first, []byte("the new world file")) }()

	<-entered

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(release)
	}()

	began := time.Now()
	if !CutWrites(5 * time.Second) {
		t.Fatal("the cut must wait for the write in flight to end")
	}

	if took := time.Since(began); took < 40*time.Millisecond {
		t.Fatalf("the cut returned in %v, before the write in flight could have ended", took)
	}

	if err := <-done; err != nil {
		t.Fatalf("the write in flight at the cut ends whole: %v", err)
	}

	if got, _ := os.ReadFile(first); string(got) != "the new world file" {
		t.Fatalf("the write in flight at the cut holds %q", got)
	}

	// After the cut no write begins.
	b5rRename(t, os.Rename)

	err := WriteFileAtomic(second, []byte("the new .od2"))
	if !errors.Is(err, ErrWritesCut) {
		t.Fatalf("a write asked for after the cut is refused ErrWritesCut, got %v", err)
	}

	if got, _ := os.ReadFile(second); string(got) != "the last save" {
		t.Fatalf("a write refused at the cut touched the file: %q", got)
	}

	if _, err := os.Stat(second + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a write refused at the cut began its temporary file (%v)", err)
	}

	// A write that hangs past the grace: reported, and the file whole.
	uncutWrites()

	hung, unhang := make(chan struct{}), make(chan struct{})

	var hang sync.Once

	b5rRename(t, func(from, to string) error {
		hang.Do(func() { close(hung) })
		<-unhang

		return errors.New("the disk never answered")
	})

	go func() { done <- WriteFileAtomic(second, []byte("the new .od2")) }()

	<-hung

	if CutWrites(30 * time.Millisecond) {
		t.Fatal("a write that hangs past the grace is reported (finished false)")
	}

	if got, _ := os.ReadFile(second); string(got) != "the last save" {
		t.Fatalf("the file a hung write would replace is whole: %q", got)
	}

	close(unhang)
	<-done
}
