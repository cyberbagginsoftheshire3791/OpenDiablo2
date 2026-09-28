package d2mapedit

import (
	"image"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// The editor's flood fill must be the loader's flood fill. It is compared
// against one walked over d2maptiled's own Map with d2maptiled's own Blocked, so
// a difference in either rule shows up here rather than as a village somebody
// cannot walk out of.
func TestReachableIsTheSameFloodTheLoaderWouldWalk(t *testing.T) {
	data := villageBytes(t)
	d := openVillage(t)
	m := parseVillage(t, data)

	mine, err := d.Reachable()
	if err != nil {
		t.Fatalf("flooding: %v", err)
	}

	theirs := loaderFlood(m, int(m.StartX), int(m.StartY))

	if mine.Count() == 0 {
		t.Fatal("nothing is reachable from the start, so this test is measuring nothing")
	}

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if mine.At(x, y) != theirs[x+y*m.Width] {
				t.Fatalf("tile %d,%d: the editor says reachable=%v, a flood over the loader's map says %v",
					x, y, mine.At(x, y), theirs[x+y*m.Width])
			}
		}
	}

	t.Logf("%d of %d tiles are reachable from the start", mine.Count(), m.Width*m.Height)
}

// loaderFlood is the flood d2maptiled/village_test.go walks, over the loader's
// own Map and its own Blocked.
func loaderFlood(m *d2maptiled.Map, sx, sy int) []bool {
	seen := make([]bool, m.Width*m.Height)
	stack := [][2]int{{sx, sy}}

	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := p[0], p[1]

		if x < 0 || y < 0 || x >= m.Width || y >= m.Height || seen[x+y*m.Width] || m.Blocked(x, y) {
			continue
		}

		seen[x+y*m.Width] = true
		stack = append(stack, [2]int{x + 1, y}, [2]int{x - 1, y}, [2]int{x, y + 1}, [2]int{x, y - 1})
	}

	return seen
}

// THE WARNING THE EDITOR EXISTS TO GIVE: an edit that seals the village. The
// village's gate is a GAP, so filling it with a blocked floor shuts the map, and
// the editor must be able to see that before a designer plays half an hour
// wondering why nothing arrives.
func TestReachableSeesTheGateShut(t *testing.T) {
	d := openVillage(t)

	const ditch = 6 // gid: placeholder-ditch, a FLOOR tile that is blocked

	k, ok := d.Kind(ditch)
	if !ok || !k.Blocked || k.BlocksSight {
		t.Fatalf("gid %d is %+v; this test wants the ditch: blocked and seen across", ditch, k)
	}

	road := image.Pt(23, d.Size().Y-1)

	open, err := d.Reachable()
	if err != nil {
		t.Fatalf("flooding: %v", err)
	}

	if !open.At(road.X, road.Y) {
		t.Fatalf("the road at %v cannot be reached from the start before anything is edited", road)
	}

	// The two openings the village has: the gate in the south fence and the
	// unfinished north-east run. Stop both and the start is shut in.
	s := NewStack(d)
	for _, p := range []image.Point{{X: 23, Y: 35}, {X: 24, Y: 35}, {X: 31, Y: 12}, {X: 32, Y: 12}, {X: 33, Y: 12}, {X: 34, Y: 12}, {X: 35, Y: 12}} {
		c, err := d.SetFloorTile(p.X, p.Y, ditch)
		if err != nil {
			t.Fatalf("ditching %v: %v", p, err)
		}

		if err := s.Do(c); err != nil {
			t.Fatalf("ditching %v: %v", p, err)
		}

		// A ditched tile must also lose its wall, or a fence tile would be
		// doing the blocking instead.
		if d.WallTile(p.X, p.Y) != 0 {
			clear, err := d.SetWallTile(p.X, p.Y, 0)
			if err != nil {
				t.Fatalf("clearing the wall at %v: %v", p, err)
			}

			if err := s.Do(clear); err != nil {
				t.Fatalf("clearing the wall at %v: %v", p, err)
			}
		}
	}

	shut, err := d.Reachable()
	if err != nil {
		t.Fatalf("flooding: %v", err)
	}

	if shut.At(road.X, road.Y) {
		t.Error("with the gate and the unfinished corner ditched the road is still reachable: the editor cannot see a sealed village")
	}

	if shut.Count() >= open.Count() {
		t.Errorf("%d tiles reachable after sealing, %d before", shut.Count(), open.Count())
	}

	// Undoing it all opens the gate again, which shows the flood is reading the
	// document rather than remembering its first answer.
	for s.CanUndo() {
		if err := s.Undo(); err != nil {
			t.Fatalf("undoing: %v", err)
		}
	}

	back, err := d.Reachable()
	if err != nil {
		t.Fatalf("flooding: %v", err)
	}

	if !back.At(road.X, road.Y) {
		t.Error("after undoing the ditches the road is still unreachable")
	}
}

// A structure seals the map just as well as a ditch, and that is the edit a
// designer actually makes: a house dropped across the gate.
//
// MEASURED while writing this test: the village has TWO ways out, not one -- the
// gate at 23..24,35 and the unfinished north-east run at 31..35,12 -- so a house
// over the gate alone leaves the road reachable round the top. The corner is
// stopped first, exactly as d2maptiled's village guard does, and the road is
// checked to be reachable THROUGH THE GATE before the house goes down.
func TestAHouseAcrossTheGateSealsTheVillage(t *testing.T) {
	d := openVillage(t)
	road := image.Pt(23, d.Size().Y-1)
	s := NewStack(d)

	const ditch = 6

	for x := 31; x <= 35; x++ {
		c, err := d.SetFloorTile(x, 12, ditch)
		if err != nil {
			t.Fatalf("ditching the corner at %d,12: %v", x, err)
		}

		if err := s.Do(c); err != nil {
			t.Fatalf("ditching the corner at %d,12: %v", x, err)
		}
	}

	before, err := d.Reachable()
	if err != nil {
		t.Fatalf("flooding: %v", err)
	}

	if !before.At(road.X, road.Y) {
		t.Fatal("with only the corner stopped the road is already unreachable, so the gate is not the way out this test thinks it is")
	}

	// The gate is the gap at 23..24 in the fence ring's south side (y=35). A 3x3
	// with its bottom corner at 26,36 covers x 23..25, y 33..35.
	c, err := d.PlaceStructure(17, 26, 36)
	if err != nil {
		t.Fatalf("placing a house across the gate: %v", err)
	}

	if err := s.Do(c); err != nil {
		t.Fatalf("placing a house across the gate: %v", err)
	}

	after, err := d.Reachable()
	if err != nil {
		t.Fatalf("flooding: %v", err)
	}

	if after.At(road.X, road.Y) {
		t.Error("a house across the gate did not shut the village; check the gate is still at 23..24,35")
	}

	// And taking the house away opens it again.
	if err := s.Undo(); err != nil {
		t.Fatalf("undoing the house: %v", err)
	}

	open, err := d.Reachable()
	if err != nil {
		t.Fatalf("flooding: %v", err)
	}

	if !open.At(road.X, road.Y) {
		t.Error("removing the house did not open the gate again")
	}
}

// Reachable needs the one player_start it floods from, and says so rather than
// answering about nothing.
func TestReachableRefusesAMapWithNoStart(t *testing.T) {
	m, _ := fixture(t)
	l := m.layer(LayerObjects)
	l["objects"] = m.objects()[1:] // drop the player_start

	d := mustOpen(t, m.bytes(t))

	if _, err := d.Reachable(); err == nil {
		t.Error("Reachable answered for a map with no player_start")
	}
}

// Blocked is four separate reasons and BlocksSight is a different question. The
// fence proves it: blocked, and seen over.
func TestBlockedAndBlocksSightAreIndependent(t *testing.T) {
	d := openVillage(t)
	size := d.Size()

	seenAcross, sightStopped := 0, 0

	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			switch {
			case d.Blocked(x, y) && !d.BlocksSight(x, y):
				seenAcross++
			case !d.Blocked(x, y) && d.BlocksSight(x, y):
				sightStopped++
			}
		}
	}

	if seenAcross == 0 {
		t.Error("no tile on the village is blocked and seen across; the fence and the ditch both are, so the two rules have been joined")
	}

	t.Logf("%d tiles blocked but seen across, %d walkable but sight-stopping", seenAcross, sightStopped)
}
