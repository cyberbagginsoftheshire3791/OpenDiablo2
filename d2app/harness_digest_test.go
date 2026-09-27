//go:build harness

package d2app

import (
	"encoding/json"
	"testing"
)

// M4.6 B1: the digest's entity lines carry the entity by its id and NOT the
// per-process handle, so the part can compare across a save made in one
// process and resumed in another. Everything else the tools report about the
// entity -- except the presentation-only screen position -- stays in.
func TestDigestLineDropsTheHandle(t *testing.T) {
	screen := [2]int{400, 300}
	info := harnessEntityInfo{
		Handle: "e:7", ID: "id-7", Kind: "npc", Label: "Wolf",
		X: 12.5, Y: 3.25, Tile: [2]int{12, 3}, Layer: 1,
		State:  map[string]interface{}{"path_len": 2},
		Screen: &screen,
	}

	raw, err := json.Marshal(harnessDigestLine(info))
	if err != nil {
		t.Fatal(err)
	}

	var line map[string]interface{}
	if err := json.Unmarshal(raw, &line); err != nil {
		t.Fatal(err)
	}

	for _, gone := range []string{"handle", "screen"} {
		if _, ok := line[gone]; ok {
			t.Fatalf("the digest line must not carry %q: %s", gone, raw)
		}
	}

	for _, kept := range []string{"id", "kind", "label", "x", "y", "tile", "layer", "state"} {
		if _, ok := line[kept]; !ok {
			t.Fatalf("the digest line must still carry %q: %s", kept, raw)
		}
	}

	// Two processes that numbered the same entity differently write the same
	// line -- the property the change exists for.
	other := info
	other.Handle = "e:1"

	raw2, _ := json.Marshal(harnessDigestLine(other))
	if string(raw) != string(raw2) {
		t.Fatalf("one entity under two handles must digest identically:\n%s\n%s", raw, raw2)
	}
}
