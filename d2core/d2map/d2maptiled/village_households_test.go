package d2maptiled

import (
	"encoding/json"
	"image"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestTheShippedVillagesHouseholds: the raid's R3a proposal on the shipped
// village (Josh's to move; data/strigoi/maps/README.md, "Josh decides the
// shape"): a household at the door of each of the six peasant houses and at
// the church's, the burned house empty of one, a hotar on the road south of
// the gate, a post at the gate and one at the north-east gap, and the four
// speakers members of their houses.
func TestTheShippedVillagesHouseholds(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data", "strigoi", "maps")

	data, err := os.ReadFile(filepath.Join(root, "village.tmj"))
	if err != nil {
		t.Fatal(err)
	}

	strigoi := filepath.Dir(root)

	m, err := Parse(data, "/data/strigoi/maps", func(p string) ([]byte, error) {
		return os.ReadFile(filepath.Join(strigoi, filepath.FromSlash(strings.TrimPrefix(path.Clean(p), "/data/strigoi"))))
	})
	if err != nil {
		t.Fatalf("the shipped village is refused: %v", err)
	}

	doors := map[string]image.Point{}
	churches := 0

	for _, h := range m.Households {
		doors[h.Name] = h.Door

		if h.Church {
			churches++
		}

		if h.Incense != 3 || h.Stakes != 2 {
			t.Errorf("%s starts with %d incense and %d stakes; the brief's D-H1 and D-H4 are 3 and 2", h.Name, h.Incense, h.Stakes)
		}
	}

	if want := map[string]image.Point{
		"the church": {16, 16}, "the headman's house": {28, 21}, "the well house": {27, 27}, "the west house": {17, 27},
		"the smith's house": {20, 31}, "the north-east house": {30, 16}, "the north house": {24, 16},
	}; !reflect.DeepEqual(doors, want) {
		t.Errorf("doors %v, want %v", doors, want)
	}

	if churches != 1 || !m.Households[0].Church {
		t.Errorf("%d churches; the church is the first household", churches)
	}

	// Every peasant house keeps a household (the six footprints below); the
	// burned house (31,30) and the gate's tower do not.
	var kept []image.Point

	for _, h := range m.Households {
		for _, d := range []image.Point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			if s := m.StructureOn(h.Door.X+d.X, h.Door.Y+d.Y); s >= 0 {
				kept = append(kept, m.Structures[s].Footprint.Min)
			}
		}
	}

	if want := []image.Point{{27, 18}, {28, 26}, {14, 26}, {17, 30}, {29, 13}, {23, 13}}; !reflect.DeepEqual(kept, want) {
		t.Errorf("the houses kept (by footprint) %v, want %v", kept, want)
	}

	if !m.HasHotar || m.Hotar != image.Pt(22, 40) || m.IsInside(m.Hotar.X, m.Hotar.Y) {
		t.Errorf("the hotar is at %v (has %v)", m.Hotar, m.HasHotar)
	}

	if want := []Post{{At: image.Pt(24, 34), Post: PostGate}, {At: image.Pt(33, 13), Post: PostCorner}}; !reflect.DeepEqual(m.Posts, want) {
		t.Errorf("posts %+v, want %+v", m.Posts, want)
	}

	speakers := map[string]string{}
	for _, n := range m.NPCs {
		speakers[n.Monstat] = n.Household
	}

	if want := map[string]string{"warriv1": "the headman's house", "kashya": "the well house", "charsi": "the smith's house",
		"akara": "the church"}; !reflect.DeepEqual(speakers, want) {
		t.Errorf("speakers %v, want %v", speakers, want)
	}
}

// TestTheShippedVillagesBuildingsAreMarked (the R3a review's B2): on the
// shipped village a door beside a tree or beside the gate's watchtower is
// beside no building -- the house, smithy and church walls are marked
// "building", the trees and the tower are not -- while a door beside the
// smithy's walls is taken.
func TestTheShippedVillagesBuildingsAreMarked(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data", "strigoi", "maps")

	data, err := os.ReadFile(filepath.Join(root, "village.tmj"))
	if err != nil {
		t.Fatal(err)
	}

	strigoi := filepath.Dir(root)
	load := func(p string) ([]byte, error) {
		return os.ReadFile(filepath.Join(strigoi, filepath.FromSlash(strings.TrimPrefix(path.Clean(p), "/data/strigoi"))))
	}

	with := func(x, y float64) error {
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}

		for _, raw := range m["layers"].([]any) {
			if l := raw.(map[string]any); l["name"] == "objects" {
				l["objects"] = append(l["objects"].([]any), map[string]any{
					"id": 900, "name": "probe", "type": "household", "x": x * 80, "y": y * 80, "point": true,
				})
			}
		}

		out, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}

		_, err = Parse(out, "/data/strigoi/maps", load)

		return err
	}

	for _, c := range []struct {
		what string
		x, y float64
		want string // "" for taken
	}{
		{"beside a tree (25,37)", 25.5, 38.5, "its door tile 25,38 is beside no building"},
		{"beside the gate's watchtower (25,34)", 25.5, 33.5, "its door tile 25,33 is beside no building"},
		{"beside the smithy's walls (26,31)", 25.5, 31.5, ""},
	} {
		err := with(c.x, c.y)

		switch {
		case c.want == "" && err != nil:
			t.Errorf("a door %s is refused: %v", c.what, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("a door %s: %v; want %q", c.what, err, c.want)
		}
	}
}
