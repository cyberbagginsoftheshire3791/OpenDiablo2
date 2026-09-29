package d2items

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// M4.6 B3: KeepGeneration copies what a save is about to replace to .bak, and
// is not an error when there is nothing yet.
func TestKeepGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "0.od2")

	if err := KeepGeneration(path); err != nil {
		t.Fatalf("nothing to keep is not an error: %v", err)
	}

	if _, err := os.Stat(BakPath(path)); !os.IsNotExist(err) {
		t.Fatalf("nothing to keep, so no .bak: %v", err)
	}

	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := KeepGeneration(path); err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(BakPath(path)); string(got) != "first" {
		t.Fatalf("the .bak holds %q", got)
	}

	if BakPath(path) != path+".bak" {
		t.Fatalf("BakPath is %q", BakPath(path))
	}
}

// HeroBytes is exactly what SaveHero writes, so the world file's embedded
// sidecar and the sidecar file are one document (M4.6 B3).
func TestHeroBytesIsWhatSaveHeroWrites(t *testing.T) {
	k := kitFor(t, "torch-and-blade")
	x := Extras{Progress: []byte(`{"xp":120}`), Village: []byte(`{"rep":21}`), Land: []byte(`{"gathered":4}`), Journal: []byte(`{"read":[]}`)}
	path := filepath.Join(t.TempDir(), "0.od2.strigoi.json")

	if err := SaveHero(path, k, x); err != nil {
		t.Fatal(err)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	data, err := HeroBytes(k, x)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(onDisk, data) {
		t.Fatalf("HeroBytes:\n%s\nSaveHero wrote:\n%s", data, onDisk)
	}

	if _, err := HeroBytes(nil, x); err == nil {
		t.Fatal("no kit, no document")
	}

	if err := WriteHero(filepath.Join(t.TempDir(), "deeper", "1.od2.strigoi.json"), data); err != nil {
		t.Fatalf("WriteHero makes the directory: %v", err)
	}
}
