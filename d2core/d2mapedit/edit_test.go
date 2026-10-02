package d2mapedit

import (
	"image"
	"strings"
	"testing"
)

// PLACE A HOUSE. The whole point of the package, and the thing the village
// guard test used to make red: a 3x3 peasant-house dropped on open ground, and
// the game still takes the file.
//
// The spot is MEASURED rather than remembered: the test walks the village for a
// 3x3 of floored, wall-free, structure-free, unreachable-by-nobody ground. A
// hard-coded tile would be a test that starts lying the first time somebody
// moves a fence.
func TestPlacingAHouseOnOpenGroundLeavesAMapTheGameTakes(t *testing.T) {
	d := openVillage(t)

	const peasantHouse = 17 // gid: the 3x3 ../structures/peasant-house/intact.png

	k, ok := d.Kind(peasantHouse)
	if !ok || k.Footprint != image.Pt(3, 3) {
		t.Fatalf("gid %d is %+v; this test wants the 3x3 peasant-house", peasantHouse, k)
	}

	corner, found := openCorner(d, 3)
	if !found {
		t.Fatal("no clear 3x3 of ground anywhere on the village")
	}

	c, err := d.PlaceStructure(peasantHouse, corner.X, corner.Y)
	if err != nil {
		t.Fatalf("placing a house at %v: %v", corner, err)
	}

	s := NewStack(d)
	if err := s.Do(c); err != nil {
		t.Fatalf("placing a house at %v: %v", corner, err)
	}

	if problems := d.Validate(DirArt(villageDir())); len(problems) > 0 {
		for _, p := range problems {
			t.Errorf("after placing a house at %v: %v", corner, p)
		}

		t.FailNow()
	}

	saved, err := d.Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	m := parseVillage(t, saved)

	if len(m.Structures) != 9 {
		t.Errorf("the loader reads %d structures, want the village's 8 (its houses and, since fog of war F4, the gate's tower) plus the new one", len(m.Structures))
	}

	// And the house is solid where it stands, which is what the loader does with
	// a footprint (tiled.go:886-890).
	for y := corner.Y - 3; y < corner.Y; y++ {
		for x := corner.X - 3; x < corner.X; x++ {
			if !m.Blocked(x, y) {
				t.Errorf("tile %d,%d is under the new house and is not blocked", x, y)
			}
		}
	}
}

// openCorner finds the bottom corner of a clear side x side block: floored, no
// walls, no structure, and not standing on anybody.
func openCorner(d *Doc, side int) (image.Point, bool) {
	size := d.Size()

	occupied := map[image.Point]bool{}

	for _, o := range d.Objects() {
		if !o.IsStructure() {
			occupied[o.Tile()] = true
		}
	}

	for y := side; y <= size.Y; y++ {
		for x := side; x <= size.X; x++ {
			clear := true

			for dy := 1; dy <= side && clear; dy++ {
				for dx := 1; dx <= side && clear; dx++ {
					p := image.Pt(x-dx, y-dy)
					_, onStructure := d.StructureOn(p.X, p.Y)
					clear = d.FloorTile(p.X, p.Y) != 0 && d.WallTile(p.X, p.Y) == 0 &&
						!onStructure && !occupied[p]
				}
			}

			if clear {
				return image.Pt(x, y), true
			}
		}
	}

	return image.Point{}, false
}

// Object ids come out of nextobjectid and bump it, the way Tiled does, so the
// two never hand out the same id.
func TestPlaceStructureTakesTheNextObjectIDAndBumpsIt(t *testing.T) {
	d := openVillage(t)
	before := d.NextObjectID()

	if before != 14 {
		t.Logf("the village's nextobjectid is %d (it was 14 when this test was written)", before)
	}

	s := NewStack(d)
	corner, _ := openCorner(d, 3)

	c, err := d.PlaceStructure(17, corner.X, corner.Y)
	if err != nil {
		t.Fatalf("placing: %v", err)
	}

	if err := s.Do(c); err != nil {
		t.Fatalf("placing: %v", err)
	}

	if got := d.NextObjectID(); got != before+1 {
		t.Errorf("nextobjectid %d after one placement, want %d", got, before+1)
	}

	placed, ok := d.Object(before)
	if !ok {
		t.Fatalf("no object %d after placing one", before)
	}

	if placed.GID != 17 || placed.Class != ClassStructure || placed.Footprint != image.Rect(corner.X-3, corner.Y-3, corner.X, corner.Y) {
		t.Errorf("placed %+v", placed)
	}

	// A second placement takes the next id, not the same one.
	c2, err := d.PlaceStructure(17, corner.X, corner.Y)
	if err != nil {
		t.Fatalf("placing again: %v", err)
	}

	if err := s.Do(c2); err != nil {
		t.Fatalf("placing again: %v", err)
	}

	if _, ok := d.Object(before + 1); !ok {
		t.Errorf("the second house did not take id %d", before+1)
	}

	// And the undo gives the id back, so an undone placement does not leak one.
	if err := s.Undo(); err != nil {
		t.Fatalf("undoing: %v", err)
	}

	if err := s.Undo(); err != nil {
		t.Fatalf("undoing: %v", err)
	}

	if got := d.NextObjectID(); got != before {
		t.Errorf("nextobjectid %d after undoing both placements, want %d back", got, before)
	}

	// But only when the undone placement was the last to take an id. Undone out
	// of order -- which a Stack will not do and a direct caller can -- the id
	// stays spent, because handing the same id out twice is worse than spending
	// one.
	first, err := d.PlaceStructure(17, corner.X, corner.Y)
	if err != nil {
		t.Fatal(err)
	}

	second, err := d.PlaceStructure(17, corner.X, corner.Y)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []Cmd{first, second} {
		if err := c.Do(d); err != nil {
			t.Fatal(err)
		}
	}

	if err := first.Undo(d); err != nil {
		t.Fatal(err)
	}

	if got := d.NextObjectID(); got != before+2 {
		t.Errorf("nextobjectid %d after an out-of-order undo, want %d left spent", got, before+2)
	}

	if _, taken := d.Object(before + 1); !taken {
		t.Error("the second placement's object went with the first one's undo")
	}
}

func TestTheEditsRefuseWhatTheyCannotRepresent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		try  func(d *Doc) (Cmd, error)
		want string
	}{
		{"a structure whose tile is in no tileset", func(d *Doc) (Cmd, error) { return d.PlaceStructure(999, 10, 10) }, "in this map's tilesets"},
		{"a structure that is not one", func(d *Doc) (Cmd, error) { return d.PlaceStructure(7, 10, 10) }, "no footprint"},
		{"a structure off the map", func(d *Doc) (Cmd, error) { return d.PlaceStructure(17, 1, 1) }, "reaches off"},
		{"a floor tile off the map", func(d *Doc) (Cmd, error) { return d.SetFloorTile(48, 0, 1) }, "off the 48x48 map"},
		{"a structure painted on the floor", func(d *Doc) (Cmd, error) { return d.SetFloorTile(0, 0, 17) }, "is a structure"},
		{"a wall painted on the floor", func(d *Doc) (Cmd, error) { return d.SetFloorTile(0, 0, 7) }, "floor art is exactly"},
		// A 160x80 tile is legal on BOTH layers -- wall art is 160 wide and
		// 80..512 tall, so a floor tile fits it (tiled.go:828). Only art that
		// is not 160x80 is refused on the floor.
		{"a tile id in no tileset", func(d *Doc) (Cmd, error) { return d.SetFloorTile(0, 0, 999) }, "in this map's tilesets"},
		{"moving an object nobody has", func(d *Doc) (Cmd, error) { return d.MoveObject(999, 1, 1) }, "no object 999"},
		{"deleting an object nobody has", func(d *Doc) (Cmd, error) { return d.DeleteObject(999) }, "no object 999"},
		{"a group with no name", func(d *Doc) (Cmd, error) { return d.SetGroup(Group{}) }, "needs a name"},
		{"a group naming a stranger", func(d *Doc) (Cmd, error) {
			return d.SetGroup(Group{Name: "g", Members: []int{999}})
		}, "does not hold"},
		{"a group naming one object twice", func(d *Doc) (Cmd, error) {
			return d.SetGroup(Group{Name: "g", Members: []int{10, 10}})
		}, "twice"},
		{"moving a group nobody has", func(d *Doc) (Cmd, error) { return d.MoveGroup("nobody", 1, 1) }, "no group"},
		{"deleting a group nobody has", func(d *Doc) (Cmd, error) { return d.DeleteGroup("nobody") }, "no group"},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := tc.try(openVillage(t))
			if err == nil {
				t.Fatal("the edit was allowed")
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// A map with no "walls" layer refuses a wall rather than inventing the layer.
func TestSetWallTileRefusesAMapWithNoWallsLayer(t *testing.T) {
	m, _ := fixture(t)

	layers := m["layers"].([]any)
	m["layers"] = []any{layers[0], layers[2]} // floor and objects only

	d := mustOpen(t, m.bytes(t))
	if d.HasWalls() {
		t.Fatal("the fixture still has a walls layer")
	}

	if _, err := d.SetWallTile(0, 0, 2); err == nil {
		t.Error("a wall was painted onto a map with no walls layer")
	}
}

// Deleting an object and undoing it must give back EVERYTHING it had, including
// keys this package does not understand and the place it had in the file -- a
// structure's file order decides which of two overlapping ones the loader keeps.
func TestDeletingAnObjectAndUndoingItGivesBackEverything(t *testing.T) {
	m, files := fixture(t)

	// A key the editor has never heard of, on the object in the middle.
	m.object(2)["visible"] = true
	m.object(2)["tiled-only-nonsense"] = []any{tmj{"deep": 1}}

	d := mustOpen(t, m.bytes(t))
	before, err := d.Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	s := NewStack(d)

	c, err := d.DeleteObject(2)
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}

	if err := s.Do(c); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	if _, ok := d.Object(2); ok {
		t.Fatal("object 2 is still there after deleting it")
	}

	if got := len(d.Objects()); got != 3 {
		t.Errorf("%d objects after deleting one of four", got)
	}

	if err := s.Undo(); err != nil {
		t.Fatalf("undoing the delete: %v", err)
	}

	after, err := d.Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	if string(before) != string(after) {
		t.Errorf("the undone delete did not give the file back; first difference at %d",
			firstByteDifference(before, after))
	}

	if problems := d.Validate(files.size()); len(problems) > 0 {
		t.Errorf("after the round trip: %v", problems)
	}
}

// Painting a tile and undoing it, on both layers.
func TestPaintingATileAndUndoingIt(t *testing.T) {
	d := openVillage(t)
	s := NewStack(d)

	const (
		x, y       = 23, 40 // the road, outside the fence
		road       = 4      // gid: placeholder-road
		fence      = 7      // gid: placeholder-fence, a wall
		ditchFloor = 6      // gid: placeholder-ditch, a blocked FLOOR
	)

	wasFloor, wasWall := d.FloorTile(x, y), d.WallTile(x, y)

	for _, step := range []struct {
		what string
		make func() (Cmd, error)
		read func() int
		want int
	}{
		{"a road tile", func() (Cmd, error) { return d.SetFloorTile(x, y, road) }, func() int { return d.FloorTile(x, y) }, road},
		{"a ditch", func() (Cmd, error) { return d.SetFloorTile(x, y, ditchFloor) }, func() int { return d.FloorTile(x, y) }, ditchFloor},
		{"a fence", func() (Cmd, error) { return d.SetWallTile(x, y, fence) }, func() int { return d.WallTile(x, y) }, fence},
		{"no fence", func() (Cmd, error) { return d.SetWallTile(x, y, 0) }, func() int { return d.WallTile(x, y) }, 0},
	} {
		c, err := step.make()
		if err != nil {
			t.Fatalf("%s: %v", step.what, err)
		}

		if err := s.Do(c); err != nil {
			t.Fatalf("%s: %v", step.what, err)
		}

		if got := step.read(); got != step.want {
			t.Fatalf("%s: tile %d,%d reads %d, want %d", step.what, x, y, got, step.want)
		}
	}

	// A ditch is a blocked FLOOR: the one tile kind that proves Blocked is not
	// just "is there a wall here" (tiled.go:246).
	if !d.Blocked(x, y) {
		t.Error("a ditch floor is not blocked")
	}

	if d.BlocksSight(x, y) {
		t.Error("a ditch stops sight; the watch sees across it")
	}

	for s.CanUndo() {
		if err := s.Undo(); err != nil {
			t.Fatalf("undoing: %v", err)
		}
	}

	if d.FloorTile(x, y) != wasFloor || d.WallTile(x, y) != wasWall {
		t.Errorf("after undoing everything tile %d,%d is floor %d wall %d, want %d and %d",
			x, y, d.FloorTile(x, y), d.WallTile(x, y), wasFloor, wasWall)
	}
}
