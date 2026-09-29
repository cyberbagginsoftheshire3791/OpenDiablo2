package d2hero

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// M4.6 B3: the .od2 is written atomically and keeps its last generation as
// .bak (rule 5). Save's temporary file is not left behind, and the .bak is the
// save before the last, not the first.
func TestSaveKeepsTheLastGeneration(t *testing.T) {
	f := &HeroStateFactory{}
	path := filepath.Join(t.TempDir(), "Saves", "0.od2")
	state := &HeroState{HeroName: "Saver", FilePath: path, Stats: &HeroStatsState{Health: 10, MaxHealth: 20}}

	read := func(p string) HeroState {
		t.Helper()

		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}

		var s HeroState
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}

		return s
	}

	for health := 10; health <= 12; health++ {
		state.Stats.Health = health
		if err := f.Save(state); err != nil {
			t.Fatal(err)
		}
	}

	if got := read(path).Stats.Health; got != 12 {
		t.Fatalf("the save holds health %d, want 12", got)
	}

	if got := read(path + ".bak").Stats.Health; got != 11 {
		t.Fatalf("the .bak holds health %d, want 11 (the save before the last)", got)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("the temporary file is left behind: %v", err)
	}
}

// THE WRITE GOES THROUGH A TEMPORARY FILE AND A RENAME (M4.6 B3). With
// N.od2.tmp made unwritable -- a directory stands where the temporary file
// would go -- the save must fail and leave the .od2 it would have replaced
// exactly as it was: a write straight into N.od2 would succeed here, and a
// crash in the middle of one would leave half a hero.
func TestSaveGoesThroughATemporaryFile(t *testing.T) {
	f := &HeroStateFactory{}
	path := filepath.Join(t.TempDir(), "0.od2")
	state := &HeroState{HeroName: "Saver", FilePath: path, Stats: &HeroStatsState{Health: 10, MaxHealth: 20}}

	if err := f.Save(state); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(path+".tmp", 0o750); err != nil {
		t.Fatal(err)
	}

	state.Stats.Health = 3

	if err := f.Save(state); err == nil {
		t.Fatal("with its temporary file blocked the save must fail, not write the .od2 in place")
	}

	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatalf("a failed save changed the .od2:\n before %s\n after  %s", before, after)
	}
}
