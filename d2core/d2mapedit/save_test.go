package d2mapedit

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// THE SAVE REFUSES A MAP THE GAME WOULD REFUSE. This is the one that matters:
// d2mapgen silently builds Diablo II's Act 1 when a .tmj is refused
// (d2mapgen/authored.go:34-38), so a save that goes through is a designer
// looking at somebody else's town.
func TestSaveRefusesAMapTheGameWouldRefuse(t *testing.T) {
	m, files := fixture(t)

	// Two player_starts: a refusal the loader makes and a file Tiled writes
	// happily.
	m.addObject(tmj{"id": 9, "name": "another start", "type": ClassPlayerStart,
		"x": 2.5 * tileH, "y": 0.5 * tileH, "width": 0, "height": 0,
		"rotation": 0, "visible": true, "point": true})

	d := mustOpen(t, m.bytes(t))
	path := filepath.Join(t.TempDir(), "broken.tmj")

	err := d.Save(path, files.size())
	if err == nil {
		t.Fatal("a map the game refuses was written")
	}

	var refused *RefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("Save returned %T (%v), want a *RefusedError", err, err)
	}

	if len(refused.Problems) == 0 {
		t.Error("the refusal names no problem")
	}

	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the file exists after a refused save (%v)", statErr)
	}

	if _, statErr := os.Stat(path + ".tmp"); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a .tmp was left behind by a refused save")
	}
}

// A save keeps the generation it overwrites, and the file it writes is the one
// the document holds.
func TestSaveKeepsThePreviousGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "village.tmj")
	original := villageBytes(t)

	// The art lives beside the real map, so the copy is validated against the
	// real directory rather than a copy of every PNG.
	artReader := DirArt(villageDir())

	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	d := openVillage(t)
	s := NewStack(d)
	corner, _ := openCorner(d, 3)

	c, err := d.PlaceStructure(17, corner.X, corner.Y)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Do(c); err != nil {
		t.Fatal(err)
	}

	if err := d.Save(path, artReader); err != nil {
		t.Fatalf("saving: %v", err)
	}

	want, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != string(want) {
		t.Errorf("the file on disk is not the document; first difference at %d", firstByteDifference(want, got))
	}

	kept, err := os.ReadFile(BackupPath(path))
	if err != nil {
		t.Fatalf("reading the kept generation: %v", err)
	}

	if string(kept) != string(original) {
		t.Errorf("%s is not the generation that was overwritten", BackupPath(path))
	}

	// Nothing in flight is left behind.
	for _, leftover := range []string{path + ".tmp", BackupPath(path) + ".tmp"} {
		if _, err := os.Stat(leftover); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s was left behind", leftover)
		}
	}

	// And the map the game reads back is the edited one.
	saved := parseVillage(t, got)
	if len(saved.Structures) != 8 {
		t.Errorf("the saved map holds %d structures, want 8", len(saved.Structures))
	}
}

// Saving where there was nothing keeps no generation, and that is not an error.
func TestSavingSomewhereNewKeepsNoGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "village.tmj")

	if err := openVillage(t).Save(path, DirArt(villageDir())); err != nil {
		t.Fatalf("saving: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("the file was not written: %v", err)
	}

	if _, err := os.Stat(BackupPath(path)); !errors.Is(err, os.ErrNotExist) {
		t.Error("a .bak appeared for a file that did not exist")
	}
}

// %APPDATA%\OpenDiablo2\Saves is the player's namespace: the hero screen lists
// and rewrites what it finds there (d2hero/hero_state_factory.go:287-297), so a
// map must not land in it.
func TestSaveRefusesThePlayersSaveDirectory(t *testing.T) {
	home := t.TempDir()

	// os.UserConfigDir reads %APPDATA% on Windows, XDG_CONFIG_HOME (then
	// $HOME/.config) elsewhere; playtest/launcher.go:170 sets the same pair to
	// give a test its own save namespace.
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)

	saves, err := playerSavesDir()
	if err != nil {
		t.Skipf("this machine has no user config directory: %v", err)
	}

	// Checked WITHOUT under(): a test must not ask the function it is testing
	// whether it should run.
	if !strings.HasPrefix(saves, filepath.Clean(home)+string(filepath.Separator)) {
		t.Skipf("os.UserConfigDir is %s, which this test cannot redirect on %s", saves, runtime.GOOS)
	}

	d := openVillage(t)
	artReader := DirArt(villageDir())

	for _, name := range []string{
		filepath.Join(saves, "village.tmj"),
		filepath.Join(saves, "maps", "village.tmj"),
		saves,
	} {
		if err := d.Save(name, artReader); !errors.Is(err, ErrPlayerSaves) {
			t.Errorf("Save(%s) = %v, want ErrPlayerSaves", name, err)
		}

		if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s exists after a refused save", name)
		}
	}

	// And a map next to the saves directory rather than in it is fine.
	fine := filepath.Join(home, "OpenDiablo2", "village.tmj")
	if err := d.Save(fine, artReader); err != nil {
		t.Errorf("Save(%s) = %v, want it allowed", fine, err)
	}
}

// A file that cannot be parsed is set aside on request, under the name the kit
// screen already uses -- and OpenFile does NOT do it on its own, because a map
// is somebody's week of work.
func TestSetAsidePutsAnUnparseableFileAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "village.tmj")
	rubbish := []byte("{this was a map once")

	if err := os.WriteFile(path, rubbish, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := OpenFile(path); err == nil {
		t.Fatal("an unparseable file opened")
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("OpenFile moved the file on its own: %v", err)
	}

	aside, err := SetAside(path)
	if err != nil {
		t.Fatalf("setting aside: %v", err)
	}

	if aside != CorruptPath(path) {
		t.Errorf("set aside to %s, want %s", aside, CorruptPath(path))
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("the original is still there after being set aside")
	}

	kept, err := os.ReadFile(aside)
	if err != nil {
		t.Fatal(err)
	}

	if string(kept) != string(rubbish) {
		t.Error("the set-aside file is not the bytes that could not be parsed")
	}
}

// OpenFile round-trips the real village off disk and remembers where it came
// from.
func TestOpenFileReadsTheShippedVillage(t *testing.T) {
	path := filepath.Join(villageDir(), "village.tmj")

	d, err := OpenFile(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}

	if d.Path() != path {
		t.Errorf("Path %q, want %q", d.Path(), path)
	}

	if problems := d.Validate(DirArt(villageDir())); len(problems) > 0 {
		t.Errorf("%v", problems)
	}
}
