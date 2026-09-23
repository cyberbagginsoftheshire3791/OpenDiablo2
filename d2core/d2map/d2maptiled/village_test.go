package d2maptiled

import (
	"image"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// The shipped village (data/strigoi/maps/village.tmj, written by
// tools/villagemap and edited in Tiled from then on) must load, and must keep
// the few things the slice's systems lean on. Everything else about its shape
// is free to change in the editor; these are the lines a hand-edit must not
// cross without somebody noticing.
func TestShippedVillageLoads(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data", "strigoi", "maps")

	data, err := os.ReadFile(filepath.Join(root, "village.tmj"))
	if err != nil {
		t.Fatalf("reading the shipped village: %v", err)
	}

	// The map's own folder is /data/strigoi/maps, as the game loads it, so
	// art beside it (../structures) resolves as it does in the game.
	strigoi := filepath.Dir(root)

	m, err := Parse(data, "/data/strigoi/maps", func(p string) ([]byte, error) {
		return os.ReadFile(filepath.Join(strigoi, filepath.FromSlash(strings.TrimPrefix(path.Clean(p), "/data/strigoi"))))
	})
	if err != nil {
		t.Fatalf("the shipped village is refused: %v", err)
	}

	if m.Blocked(int(m.StartX), int(m.StartY)) {
		t.Fatalf("the player starts on a blocked tile %.1f,%.1f", m.StartX, m.StartY)
	}

	// The four speakers' stand-ins (data/strigoi/dialogue.json).
	want := map[string]bool{"warriv1": false, "kashya": false, "charsi": false, "akara": false}
	for _, n := range m.NPCs {
		if _, ok := want[n.Monstat]; ok {
			want[n.Monstat] = true
		}
	}

	for id, found := range want {
		if !found {
			t.Errorf("no npc stands in as %s", id)
		}
	}

	// The fence stops a man and not his eyes (a wattle fence is chest-high):
	// the watch sees over it, the dead cannot walk through it.
	fence := m.At(12, 20)
	if fence.Wall < 0 || m.Kinds[fence.Wall].Name != "village-placeholder#6" {
		t.Fatalf("tile 12,20 = %+v, want the west fence", fence)
	}

	if !m.Blocked(12, 20) || m.BlocksSight(12, 20) {
		t.Errorf("the fence: blocked %v, blocks sight %v; want true, false", m.Blocked(12, 20), m.BlocksSight(12, 20))
	}

	// The inside area is the enclosure, fence ring included: the player
	// starts in it and the road beyond the gate is not in it.
	if len(m.Inside) != 1 || m.Inside[0] != image.Rect(12, 12, 36, 36) {
		t.Errorf("inside %v, want the fence ring 12..35", m.Inside)
	}

	if !m.IsInside(int(m.StartX), int(m.StartY)) || m.IsInside(23, 40) {
		t.Errorf("inside: start %v, road %v; want true, false", m.IsInside(int(m.StartX), int(m.StartY)), m.IsInside(23, 40))
	}

	// The strigoi-art houses stand on the map as structures: seven 3x3
	// footprints, every one inside the fence.
	if len(m.Structures) != 7 {
		t.Errorf("%d structures, want 7 houses", len(m.Structures))
	}

	for _, st := range m.Structures {
		if st.Footprint.Dx() != 3 || st.Footprint.Dy() != 3 || !st.Footprint.In(m.Inside[0]) {
			t.Errorf("structure %s (%s) is not a 3x3 inside the fence", st.Footprint, m.Kinds[st.Kind].Name)
		}
	}

	// The enclosure is closed except where it is meant to be open. Walk the
	// flood fill from the start and count how the village can be left.
	reach := flood(m, int(m.StartX), int(m.StartY))

	// The road south of the gate is reachable...
	road := 23 + (m.Height-1)*m.Width
	if !reach[road] {
		t.Error("the road off the south edge cannot be reached from the start: the gate is shut")
	}

	// ...but only through its openings. Stop up exactly those -- the gate
	// (fence ring y=35, x=23..24) and the unfinished north-east run (fence
	// ring y=12, x=31..35) -- with ditch, and the start must be shut in. A
	// fence or ditch that does not block leaks here.
	ditch := -1

	for i := range m.Kinds {
		if m.Kinds[i].Name == "village-placeholder#5" && m.Kinds[i].Layer == LayerFloor {
			ditch = i
		}
	}

	if ditch < 0 || !m.Kinds[ditch].Blocked || m.Kinds[ditch].BlocksSight {
		t.Fatalf("the ditch (tile id 5) is missing or wrong: blocked and seen across is the rule")
	}

	shut := *m
	shut.Cells = append([]Cell(nil), m.Cells...)

	for _, p := range [][2]int{{23, 35}, {24, 35}, {31, 12}, {32, 12}, {33, 12}, {34, 12}, {35, 12}} {
		shut.Cells[p[0]+p[1]*m.Width] = Cell{Floor: ditch, Wall: -1}
	}

	if r := flood(&shut, int(m.StartX), int(m.StartY)); r[road] {
		t.Error("with the gate and the unfinished corner stopped the village still leaks: the fence or ditch does not block")
	}

	// And the gate alone is an opening: stopping only the corner still lets
	// the start out, so the check above is not passing because the start is
	// walled in regardless.
	gateOnly := *m
	gateOnly.Cells = append([]Cell(nil), m.Cells...)

	for x := 31; x <= 35; x++ {
		gateOnly.Cells[x+12*m.Width] = Cell{Floor: ditch, Wall: -1}
	}

	if r := flood(&gateOnly, int(m.StartX), int(m.StartY)); !r[road] {
		t.Error("with only the corner stopped the start cannot reach the road: the gate is not open")
	}
}

func flood(m *Map, sx, sy int) []bool {
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
