package d2mapedit

import (
	"reflect"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// The raid's R3a: the village's objects placed and an npc's household set as
// edits, which the game then reads; undone, the file is byte for byte what it
// was.
func TestTheVillagesObjectsArePlacedAsEdits(t *testing.T) {
	t.Parallel()

	m, files := fixture(t)
	data := m.bytes(t)

	d, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}

	s := NewStack(d)

	do := func(c Cmd, err error) {
		t.Helper()

		if err != nil {
			t.Fatal(err)
		}

		if err := s.Do(c); err != nil {
			t.Fatal(err)
		}
	}

	do(d.PlacePoint(ClassHousehold, "home", 1.5, 3.5,
		Property{"members", "string", "man,woman,child"}, Property{"incense", "int", 3},
		Property{"stakes", "int", 2}, Property{"church", "bool", false}))
	do(d.PlacePoint(ClassHotar, "the boundary", 3.5, 1.5))
	do(d.PlacePoint(ClassWatchPost, "the gate", 0.5, 3.5, Property{"post", "string", "gate"}))
	do(d.SetProperty(4, Property{"household", "string", "home"}))

	out, err := d.Checked(files.size(), EngineParse("", files.loader()))
	if err != nil {
		t.Fatalf("the placed village is refused: %v", err)
	}

	parsed, err := d2maptiled.Parse(out, "", files.loader())
	if err != nil {
		t.Fatal(err)
	}

	if len(parsed.Households) != 1 || parsed.Households[0].Incense != 3 || !parsed.HasHotar || len(parsed.Posts) != 1 ||
		parsed.NPCs[0].Household != "home" {
		t.Fatalf("the game reads %+v %v %+v %+v", parsed.Households, parsed.HasHotar, parsed.Posts, parsed.NPCs)
	}

	// The game reads the edits exactly as it reads withVillage's hand-written
	// records.
	want, _ := fixture(t)
	withVillage(want)

	byHand, err := d2maptiled.Parse(want.bytes(t), "", files.loader())
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual([]any{parsed.Households, parsed.Hotar, parsed.Posts, parsed.NPCs},
		[]any{byHand.Households, byHand.Hotar, byHand.Posts, byHand.NPCs}) {
		t.Errorf("the edits read %+v %+v %+v; by hand %+v %+v %+v", parsed.Households, parsed.Posts, parsed.NPCs,
			byHand.Households, byHand.Posts, byHand.NPCs)
	}

	for s.CanUndo() {
		if err := s.Undo(); err != nil {
			t.Fatal(err)
		}
	}

	if back, _ := d.Bytes(); string(back) != string(data) {
		t.Fatalf("undone, the map is not what it was:\n%s", back)
	}

	// What the edits refuse: a class that is not the village's, off the map,
	// a value of the wrong type, a property on a structure.
	if _, err := d.PlacePoint(ClassNPC, "x", 1, 1); err == nil {
		t.Error("an npc is placed with its monstat, not as a village object")
	}

	if _, err := d.PlacePoint(ClassHotar, "x", 9, 1); err == nil {
		t.Error("a point off the map")
	}

	if _, err := d.PlacePoint(ClassHousehold, "x", 1, 1, Property{"incense", "int", "3"}); err == nil {
		t.Error("an int property given a string")
	}

	if _, err := d.SetProperty(3, Property{"household", "string", "home"}); err == nil {
		t.Error("a property on a structure")
	}
}
