package d2mapedit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The 28 Sep review of World Editor v0 found three defects that live in this
// package: a save that trusted the validator alone (B2), a placed structure
// named for the loader's tile id rather than its kind (C), and an unsaved
// marker that could not tell the saved state when undo came back to it (C).
// docs/editor.md, "27-28 Sep review and fixes", has the whole list.

// B2. THE SAVE ASKS THE ENGINE, NOT ONLY THE VALIDATOR. The reviewer's case: a
// tile PNG cut off just after its header. The validator reads the size out of
// the first 24 bytes (DirArt, PNGSize) and is satisfied; the loader decodes the
// picture and refuses the map -- and a refused map is not an error in the game,
// it is Diablo II's Act 1 built in its place. So the save must refuse it.
//
// Negative control (28 Sep 2026): make Checked skip the engine call and this
// fails -- "a map the engine refuses was written: <path>" -- with the file on
// disk and the validator still reporting 0 problems.
func TestSaveRefusesArtTheEngineCannotDecode(t *testing.T) {
	m, files := fixture(t)

	// The floor PNG, truncated a few bytes past its IHDR: a real signature and
	// a real size, and no picture.
	whole := files["floor.png"]
	files["floor.png"] = whole[:pngHeaderLen+8]

	d := mustOpen(t, m.bytes(t))

	// The instrument's own control: the validator really is satisfied, so it
	// is the ENGINE check below that refuses, not the validator.
	if problems := d.Validate(files.size()); len(problems) != 0 {
		t.Fatalf("the truncated PNG was meant to pass the header-only validator, got %d problem(s): %v",
			len(problems), problems[0])
	}

	path := filepath.Join(t.TempDir(), "truncated.tmj")

	err := d.Save(path, files.size(), EngineParse("", files.loader()))

	var refused *EngineRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("Save returned %v (%T), want an *EngineRefusedError", err, err)
	}

	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a map the engine refuses was written: %s", path)
	}

	// And the same document with the whole PNG back saves: the refusal was
	// about the art, not about asking.
	files["floor.png"] = whole

	if err := d.Save(path, files.size(), EngineParse("", files.loader())); err != nil {
		t.Fatalf("the intact map was refused: %v", err)
	}

	t.Logf("refused as it should be: %v", refused)
}

// Checked with no engine to ask is a refusal, never a silent pass -- a nil
// check is exactly the hole B2 closed.
func TestCheckedWithoutAnEngineRefuses(t *testing.T) {
	m, files := fixture(t)
	d := mustOpen(t, m.bytes(t))

	if _, err := d.Checked(files.size(), nil); !errors.Is(err, errNoEngine) {
		t.Fatalf("Checked with a nil engine = %v, want errNoEngine", err)
	}
}

// Checked hands back EXACTLY the bytes the engine was asked about, which are the
// bytes a save writes and a playtest runs.
func TestCheckedReturnsTheBytesTheEngineSaw(t *testing.T) {
	m, files := fixture(t)
	d := mustOpen(t, m.bytes(t))

	var seen []byte

	data, err := d.Checked(files.size(), func(b []byte) error {
		seen = append([]byte(nil), b...)
		return EngineParse("", files.loader())(b)
	})
	if err != nil {
		t.Fatal(err)
	}

	if string(seen) != string(data) {
		t.Fatalf("Checked returned %d bytes, the engine was shown %d different ones", len(data), len(seen))
	}
}

// C. A PLACED STRUCTURE IS NAMED FOR ITS KIND. The village names its houses
// "peasant-house" and "burned-house"; the editor used to write
// "village-placeholder#16" beside them.
//
// Negative control (28 Sep 2026): write k.Name in placeStructureCmd.Do again
// and this fails -- `the placed structure is named "village-placeholder#16",
// want "peasant-house"`.
func TestAPlacedStructureIsNamedForItsKind(t *testing.T) {
	d := openVillage(t)
	s := NewStack(d)

	corner, ok := openCorner(d, 3)
	if !ok {
		t.Fatal("no clear 3x3 in the village")
	}

	for _, want := range []struct {
		gid  int
		name string
	}{{17, "peasant-house"}, {18, "burned-house"}} {
		// Every existing structure of that kind in the shipped village is the
		// record the name must match.
		for _, o := range d.Structures() {
			if o.GID == want.gid && o.Name != want.name {
				t.Fatalf("the village names gid %d %q, not %q -- the test's premise is wrong", want.gid, o.Name, want.name)
			}
		}

		c, err := d.PlaceStructure(want.gid, corner.X, corner.Y)
		if err != nil {
			t.Fatal(err)
		}

		if err := s.Do(c); err != nil {
			t.Fatal(err)
		}

		placed, ok := d.StructureOn(corner.X-1, corner.Y-1)
		if !ok {
			t.Fatalf("nothing stands on the placed footprint's front tile %d,%d", corner.X-1, corner.Y-1)
		}

		if placed.Name != want.name {
			t.Errorf("the placed structure is named %q, want %q", placed.Name, want.name)
		}

		if err := s.Undo(); err != nil {
			t.Fatal(err)
		}
	}

	// The shapes that are not structures/<kind>/<state>.png.
	for _, c := range []struct {
		k    Kind
		want string
	}{
		{Kind{Name: "t#1", Image: "tiles/placeholder-house.png"}, "placeholder-house"},
		{Kind{Name: "t#2", Image: `..\structures\village-well\intact.png`}, "village-well"},
		{Kind{Name: "t#3", Image: "house.png"}, "house"},
		{Kind{Name: "t#4"}, "t#4"},
	} {
		if got := StructureName(c.k); got != c.want {
			t.Errorf("StructureName(%q) = %q, want %q", c.k.Image, got, c.want)
		}
	}
}

// C. THE UNSAVED MARK FOLLOWS THE HISTORY. Undoing back to the saved map is a
// saved map; an edit made after undoing below the save throws the saved state's
// branch away, and no amount of undo or redo reaches it again.
//
// Negative control (28 Sep 2026): make Dirty answer the way the old flag did --
// true after ANY edit, undo or redo until the next save (len(done)+len(redo) > 0
// unless the history sits exactly on the save with nothing to redo) -- and this
// fails: "after undoing the only edit the map is the file again, and the stack
// says it is dirty".
func TestTheUnsavedMarkFollowsTheHistory(t *testing.T) {
	d := openVillage(t)
	s := NewStack(d)

	if s.Dirty() {
		t.Fatal("a document just opened is dirty")
	}

	corner, ok := openCorner(d, 3)
	if !ok {
		t.Fatal("no clear 3x3 in the village")
	}

	place := func() {
		t.Helper()

		c, err := d.PlaceStructure(17, corner.X, corner.Y)
		if err != nil {
			t.Fatal(err)
		}

		if err := s.Do(c); err != nil {
			t.Fatal(err)
		}
	}

	place()

	if !s.Dirty() {
		t.Fatal("an edit left the document clean")
	}

	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}

	if s.Dirty() {
		t.Fatal("after undoing the only edit the map is the file again, and the stack says it is dirty")
	}

	if err := s.Redo(); err != nil {
		t.Fatal(err)
	}

	s.MarkSaved() // Ctrl+S with the house down

	if s.Dirty() {
		t.Fatal("dirty straight after a save")
	}

	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}

	if !s.Dirty() {
		t.Fatal("undoing past the save left the document clean")
	}

	if err := s.Redo(); err != nil {
		t.Fatal(err)
	}

	if s.Dirty() {
		t.Fatal("redoing back to the save left the document dirty")
	}

	// Below the save, a NEW edit drops the branch the save was on.
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}

	del, err := d.DeleteObject(10)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Do(del); err != nil {
		t.Fatal(err)
	}

	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}

	if !s.Dirty() {
		t.Fatal("the saved map is not reachable any more (its redo branch is gone), yet the stack says clean")
	}

	if s.CanRedo() {
		if err := s.Redo(); err != nil {
			t.Fatal(err)
		}
	}

	if !s.Dirty() {
		t.Fatal("redoing the new edit is not the saved map either")
	}
}
