package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSourceExists pins the S7 fix (14 Sep 2026): a loose file that is on disk
// must report Exists()==true, and one that is not must report false. The bug was
// os.IsExist(err) after os.Stat -- true only for an "already exists" error -- so
// the loose-file source answered false for every file that existed, and
// FileExists (LoadSound, composite COF/animation checks) could not find a single
// loose asset. No MPQ is needed for this.
//
// Negative control: restore os.IsExist(err) and the "present" case flips to
// false -- the test goes red.
func TestSourceExists(t *testing.T) {
	dir := t.TempDir()

	present := filepath.Join(dir, "here.txt")
	if err := os.WriteFile(present, []byte("loose asset"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	s := &Source{Root: dir}

	if !s.Exists("here.txt") {
		t.Fatalf("Exists must be true for a file that is on disk")
	}

	if s.Exists("missing.txt") {
		t.Fatalf("Exists must be false for a file that is not on disk")
	}
}
