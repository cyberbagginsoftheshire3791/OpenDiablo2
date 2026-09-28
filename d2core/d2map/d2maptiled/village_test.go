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
//
// # WHAT CHANGED AT M5.4's EDITOR BURST (27 Sep 2026), AND WHY
//
// This test used to pin the village's CENSUS as well as its rules, and the
// census was the stale part. It was written when the map was tool-generated and
// nothing could edit it, so "7 structures" and "the fence kind is
// village-placeholder#6 at tile 12,20" were free to assert. Now that
// d2core/d2mapedit can place a house, those two lines made "place a house" a
// RED TEST -- the editor's own acceptance test would have had to break this one
// to pass. A guard that goes red when the thing it guards is used correctly gets
// deleted by the next person in a hurry, so it is fixed here instead.
//
// Three assertions were replaced, and NOTHING was weakened:
//
//   - "exactly 7 structures" became "at least one, every one square, every one
//     inside the enclosure, and none overlapping another". The count said
//     nothing the rules do not; the non-overlap check is new, and is a check on
//     the loader as well as the map.
//   - "tile 12,20 is village-placeholder#6" became a SEARCH for a wall kind that
//     is blocked and not sight-blocking, which is the property the watch leans
//     on (a wattle fence is chest-high: the dead cannot walk through it, the
//     watch sees over it). The name and the tile were never the point.
//   - "the ditch is village-placeholder#5" became a search for a FLOOR kind that
//     is blocked and not sight-blocking, which is what the flood fill below
//     needs and what a ditch is.
//   - The inside area's exact rectangle (12,12)-(36,36) is no longer pinned, so
//     the enclosure can be extended. What it was there to prove is asserted
//     directly instead: there is one enclosure, the start is in it, the road
//     beyond the gate is not, and every structure is inside it.
//
// THE FLOOD FILL IS UNTOUCHED. It is the check that proves the enclosure, and it
// is the only thing here that can tell a fence that blocks from a fence that
// does not. Its openings -- the gate at 23..24,35 and the unfinished north-east
// run at 31..35,12 -- are still named by tile, on purpose: they are the map's
// two ways out, and the test's whole value is that stopping BOTH shuts the
// village while stopping only the corner does not.
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

	// Its sound environment and name are its own (M5.3's tables burst): the
	// game no longer reads levels.txt for them. 1 is the environment
	// levels.txt gave the Act 1 town the village stands in for, measured at
	// a690dfc4 -- the song, the day ambience and the day sounds it has always
	// played.
	if m.SoundEnv != 1 || m.DisplayName == "" {
		t.Errorf("sound_env %d, display_name %q; want 1 and a name", m.SoundEnv, m.DisplayName)
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
	// the watch sees over it, the dead cannot walk through it. Found by that
	// property rather than by name and tile, so moving a fence is an edit and
	// not a failure.
	fx, fy, found := findWall(m, func(k Kind) bool { return k.Blocked && !k.BlocksSight })
	if !found {
		t.Fatal("no wall on the village is blocked and seen across; the fence is the one thing the watch needs to see over")
	}

	if !m.Blocked(fx, fy) || m.BlocksSight(fx, fy) {
		t.Errorf("the fence at %d,%d: blocked %v, blocks sight %v; want true, false",
			fx, fy, m.Blocked(fx, fy), m.BlocksSight(fx, fy))
	}

	// One enclosure, fence ring included: the night does not arrive on it
	// (arrivals are placed outside and come in by the gate). The player starts
	// in it and the road beyond the gate does not.
	if len(m.Inside) != 1 {
		t.Fatalf("%d inside areas, want the one enclosure", len(m.Inside))
	}

	if !m.IsInside(int(m.StartX), int(m.StartY)) || m.IsInside(23, 40) {
		t.Errorf("inside: start %v, road %v; want true, false", m.IsInside(int(m.StartX), int(m.StartY)), m.IsInside(23, 40))
	}

	// The strigoi-art houses stand on the map as structures: square footprints,
	// every one inside the enclosure, and none over another. The COUNT is the
	// editor's business (d2core/d2mapedit places them); these are the rules.
	if len(m.Structures) == 0 {
		t.Error("no structures on the village; the houses are what the slice is walked around")
	}

	taken := map[image.Point]int{}

	for i, st := range m.Structures {
		if st.Footprint.Dx() != st.Footprint.Dy() {
			t.Errorf("structure %s (%s) is %dx%d; footprints are square",
				st.Footprint, m.Kinds[st.Kind].Name, st.Footprint.Dx(), st.Footprint.Dy())
		}

		if !st.Footprint.In(m.Inside[0]) {
			t.Errorf("structure %s (%s) is not inside the enclosure %s",
				st.Footprint, m.Kinds[st.Kind].Name, m.Inside[0])
		}

		for y := st.Footprint.Min.Y; y < st.Footprint.Max.Y; y++ {
			for x := st.Footprint.Min.X; x < st.Footprint.Max.X; x++ {
				p := image.Pt(x, y)
				if other, clash := taken[p]; clash {
					t.Errorf("structures %d and %d both stand on tile %d,%d", other, i, x, y)
				}

				taken[p] = i
			}
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
	//
	// The ditch is the map's blocked-but-seen-across FLOOR: blocked so nobody
	// walks it, seen across so the watch sees what is standing in it.
	ditch := -1

	for i := range m.Kinds {
		if m.Kinds[i].Layer == LayerFloor && m.Kinds[i].Blocked && !m.Kinds[i].BlocksSight {
			ditch = i
		}
	}

	if ditch < 0 {
		t.Fatal("the village has no blocked, seen-across floor kind: the ditch is missing, and blocked-and-seen-across is the rule")
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

// findWall is the first tile carrying a wall whose kind answers want.
func findWall(m *Map, want func(Kind) bool) (x, y int, found bool) {
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if w := m.At(x, y).Wall; w >= 0 && want(m.Kinds[w]) {
				return x, y, true
			}
		}
	}

	return 0, 0, false
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
