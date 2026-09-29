package d2hero

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// M4.6 B3 review, A1: deleting a hero removes EVERY file of his -- the world
// file, every .bak and .tmp, every world file set aside unread -- and nothing
// of a neighbour whose number starts with his. The hero screen's Delete used
// to remove only N.od2 and the sidecar, and the next hero made got the same N
// and the dead man's world file.

func b3Touch(t *testing.T, dir string, names ...string) {
	t.Helper()

	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func b3Names(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}

	sort.Strings(out)

	return out
}

func TestDeleteHeroRemovesEveryFileOfHis(t *testing.T) {
	dir := t.TempDir()
	his := []string{
		"1.od2", "1.od2.bak", "1.od2.tmp", "1.od2.strigoi.json", "1.od2.strigoi.json.tmp",
		"1.od2.world.json", "1.od2.world.json.bak", "1.od2.world.json.tmp",
		"1.od2.world.json.v2.unread", "1.od2.world.json.unread.1", "1.OD2.world.json.unread",
	}
	neighbours := []string{"0.od2", "10.od2", "10.od2.world.json", "11.od2.strigoi.json", "1.od2x", "21.od2.world.json"}

	b3Touch(t, dir, his...)
	b3Touch(t, dir, neighbours...)

	removed, err := DeleteHero(filepath.Join(dir, "1.od2"))
	if err != nil {
		t.Fatal(err)
	}

	if len(removed) != len(his) {
		t.Fatalf("removed %d files, want every one of his %d: %v", len(removed), len(his), removed)
	}

	if last := filepath.Base(removed[len(removed)-1]); last != "1.od2" {
		t.Fatalf("his .od2 goes last, so a failure part-way leaves him listed; the last removed was %s", last)
	}

	sort.Strings(neighbours)

	got := b3Names(t, dir)
	if len(got) != len(neighbours) {
		t.Fatalf("left %v, want exactly the neighbours %v", got, neighbours)
	}

	for i := range got {
		if got[i] != neighbours[i] {
			t.Fatalf("left %v, want exactly the neighbours %v", got, neighbours)
		}
	}

	// Nothing to delete is not an error.
	if removed, err := DeleteHero(filepath.Join(dir, "7.od2")); err != nil || len(removed) != 0 {
		t.Fatalf("a hero with no files: removed %v, %v", removed, err)
	}
}

// A file of his that cannot be removed keeps his .od2: he stays listed, a
// second delete can finish, and his number is not given away meanwhile.
func TestDeleteHeroKeepsHisSaveWhileAFileOfHisRemains(t *testing.T) {
	dir := t.TempDir()
	b3Touch(t, dir, "3.od2", "3.od2.strigoi.json")

	// A directory with something in it cannot be removed by os.Remove.
	stuck := filepath.Join(dir, "3.od2.world.json")
	if err := os.MkdirAll(filepath.Join(stuck, "inside"), 0o750); err != nil {
		t.Fatal(err)
	}

	if _, err := DeleteHero(filepath.Join(dir, "3.od2")); err == nil {
		t.Fatal("a file of his could not be removed, and the delete said nothing")
	}

	if _, err := os.Stat(filepath.Join(dir, "3.od2")); err != nil {
		t.Fatalf("his .od2 must stay while a file of his remains: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "3.od2.strigoi.json")); !os.IsNotExist(err) {
		t.Fatalf("what could be removed is removed: %v", err)
	}

	if got := firstFreeFileName(dir); filepath.Base(got) == "3.od2" {
		t.Fatal("his number was handed out while his world file remains")
	}
}

// The next hero gets the first number NOTHING is named after.
func TestTheNextHeroGetsANumberNothingIsNamedAfter(t *testing.T) {
	cases := []struct {
		files []string
		want  string
	}{
		{nil, "0.od2"},
		{[]string{"0.od2"}, "1.od2"},
		{[]string{"0.od2.world.json"}, "1.od2"},
		{[]string{"0.od2", "1.od2.world.json.bak"}, "2.od2"},
		{[]string{"0.od2.strigoi.json", "1.od2.bak", "2.od2.world.json.v2.unread", "4.od2"}, "3.od2"},
		{[]string{"10.od2", "0.od2x"}, "0.od2"},
	}

	for _, tc := range cases {
		dir := t.TempDir()
		b3Touch(t, dir, tc.files...)

		if got := filepath.Base(firstFreeFileName(dir)); got != tc.want {
			t.Errorf("with %v the next hero is %s, want %s", tc.files, got, tc.want)
		}
	}

	if got := filepath.Base(firstFreeFileName(filepath.Join(t.TempDir(), "no-such-folder"))); got != "0.od2" {
		t.Errorf("no saves folder yet: %s", got)
	}
}
