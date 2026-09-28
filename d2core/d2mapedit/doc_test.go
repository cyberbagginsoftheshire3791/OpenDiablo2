package d2mapedit

import (
	"bytes"
	"encoding/json"
	"image"
	"reflect"
	"strconv"
	"testing"
)

// parsedValues reads a .tmj the way a test compares two of them: every value,
// numbers kept as their exact literal, indentation and key order thrown away.
func parsedValues(t *testing.T, data []byte) any {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("re-parsing: %v", err)
	}

	return v
}

// THE ROUND TRIP IS THE FOUNDATION. If a save loses nextobjectid, or turns
// compressionlevel -1 into -1.0, or drops a key this package has never heard
// of, then every other guarantee is written on sand: Tiled opens a file it did
// not write, and the next hand-edit is against something else again.
func TestTheVillageRoundTripsAsParsedValues(t *testing.T) {
	original := villageBytes(t)

	saved, err := openVillage(t).Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	if want, got := parsedValues(t, original), parsedValues(t, saved); !reflect.DeepEqual(want, got) {
		t.Errorf("the saved village is not the one that was opened.\nkeys differ somewhere under the top level; first difference:\n%s",
			firstDifference(want, got, ""))
	}
}

// And the bytes, which is more than the brief asked for and worth having: the
// designer's git diff after "move one house" should be the house. Tiled and
// tools/villagemap both write json.MarshalIndent(m, "", " ") plus a newline
// (tools/villagemap/main.go:558-562), and writeTree reproduces it exactly,
// including the file's own key order.
func TestTheVillageRoundTripsByteForByte(t *testing.T) {
	original := villageBytes(t)

	saved, err := openVillage(t).Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	if !bytes.Equal(original, saved) {
		t.Errorf("%d bytes in, %d bytes out; first difference at %d", len(original), len(saved), firstByteDifference(original, saved))
	}
}

// And the game must still take it. A round trip that parses as the same values
// and is then refused would mean the refusal is in something a value comparison
// cannot see.
func TestTheSavedVillageStillLoadsInTheGame(t *testing.T) {
	saved, err := openVillage(t).Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	parseVillage(t, saved) // fatals if the loader refuses
}

// An EDITED document must round-trip too, and to its own tree: everything the
// edits write is a json.Number, so opening what was just saved gives the tree
// back unchanged. Without this, "save, reopen, save again" could keep changing
// the file.
func TestAnEditedDocumentRoundTripsToItsOwnTree(t *testing.T) {
	d := openVillage(t)
	s := NewStack(d)

	// gid 17 is the 3x3 peasant-house; 22,42 is bare road south of the fence,
	// so this is a house on open ground outside the village.
	place, err := d.PlaceStructure(17, 22, 42)
	if err != nil {
		t.Fatalf("placing a house: %v", err)
	}

	if err := s.Do(place); err != nil {
		t.Fatalf("placing a house: %v", err)
	}

	move, err := d.MoveObject(10, 30.5, 30.5)
	if err != nil {
		t.Fatalf("moving the headman: %v", err)
	}

	if err := s.Do(move); err != nil {
		t.Fatalf("moving the headman: %v", err)
	}

	tile, err := d.SetFloorTile(0, 0, 4)
	if err != nil {
		t.Fatalf("setting a floor tile: %v", err)
	}

	if err := s.Do(tile); err != nil {
		t.Fatalf("setting a floor tile: %v", err)
	}

	group, err := d.SetGroup(Group{Name: "the south farm", Members: []int{10}})
	if err != nil {
		t.Fatalf("grouping: %v", err)
	}

	if err := s.Do(group); err != nil {
		t.Fatalf("grouping: %v", err)
	}

	once, err := d.Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	again, err := mustOpen(t, once).Bytes()
	if err != nil {
		t.Fatalf("saving again: %v", err)
	}

	if !bytes.Equal(once, again) {
		t.Errorf("saving an edited map twice gives different bytes; first difference at %d", firstByteDifference(once, again))
	}
}

// Undoing everything must give back the file that was opened, byte for byte.
// This is the strongest statement the undo stack can make, and it catches what
// a per-command test cannot: a command whose Undo is nearly right.
func TestUndoingEverythingGivesTheFileBack(t *testing.T) {
	original := villageBytes(t)
	d := openVillage(t)
	s := NewStack(d)

	for _, step := range []struct {
		what string
		make func() (Cmd, error)
	}{
		{"place a house", func() (Cmd, error) { return d.PlaceStructure(17, 22, 42) }},
		{"move the headman", func() (Cmd, error) { return d.MoveObject(10, 30.5, 30.5) }},
		{"delete a villager", func() (Cmd, error) { return d.DeleteObject(11) }},
		{"paint the floor", func() (Cmd, error) { return d.SetFloorTile(0, 0, 4) }},
		{"paint a wall", func() (Cmd, error) { return d.SetWallTile(0, 0, 7) }},
		{"group", func() (Cmd, error) { return d.SetGroup(Group{Name: "g", Origin: image.Pt(1, 2), Members: []int{10}}) }},
	} {
		c, err := step.make()
		if err != nil {
			t.Fatalf("%s: %v", step.what, err)
		}

		if err := s.Do(c); err != nil {
			t.Fatalf("%s: %v", step.what, err)
		}
	}

	for s.CanUndo() {
		label := s.UndoLabel()
		if err := s.Undo(); err != nil {
			t.Fatalf("undoing %q: %v", label, err)
		}
	}

	back, err := d.Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	if !bytes.Equal(original, back) {
		t.Errorf("after undoing six edits the file is not the one that was opened; first difference at %d", firstByteDifference(original, back))
	}
}

func TestOpenRefusesWhatItCannotEdit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		bad  func(m tmj)
	}{
		{"not a map", func(m tmj) { m["type"] = "tileset" }},
		{"orthogonal", func(m tmj) { m["orientation"] = "orthogonal" }},
		{"the wrong tile size", func(m tmj) { m["tilewidth"] = 64; m["tileheight"] = 32 }},
		{"infinite", func(m tmj) { m["infinite"] = true }},
		{"a side of zero", func(m tmj) { m["width"] = 0 }},
		{"a side past the limit", func(m tmj) { m["width"] = maxSide + 1 }},
		{"no floor layer", func(m tmj) { m.layer(LayerFloor)["name"] = "ground" }},
		{"a compressed layer", func(m tmj) { m.layer(LayerFloor)["compression"] = "zlib" }},
		{"a base64 layer", func(m tmj) { m.layer(LayerFloor)["encoding"] = "base64" }},
		{"a short layer", func(m tmj) { m.layer(LayerFloor)["data"] = fill(15, 1) }},
		{"an external tileset", func(m tmj) { m.tileset()["source"] = "t.tsx" }},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, _ := fixture(t)
			tc.bad(m)

			if _, err := Open(m.bytes(t)); err == nil {
				t.Error("opened a map the editor cannot represent")
			}
		})
	}
}

func TestOpenRefusesRubbish(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, data string }{
		{"empty", ""},
		{"not json", "this is not a map"},
		{"an array", "[1,2,3]"},
		{"half a map", `{"orientation":"isometric",`},
		{"two maps", `{"orientation":"isometric","width":1}{"width":2}`},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Open([]byte(tc.data)); err == nil {
				t.Errorf("opened %q", tc.data)
			}
		})
	}
}

// ---- small helpers --------------------------------------------------------

func mustOpen(t *testing.T, data []byte) *Doc {
	t.Helper()

	d, err := Open(data)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	return d
}

func firstByteDifference(a, b []byte) int {
	for i := range a {
		if i >= len(b) || a[i] != b[i] {
			return i
		}
	}

	if len(b) > len(a) {
		return len(a)
	}

	return -1
}

// firstDifference walks two parsed trees and names the first path that differs,
// so a failure says "layers[0].data[17]" rather than printing 41KB twice.
func firstDifference(want, got any, at string) string {
	if reflect.DeepEqual(want, got) {
		return ""
	}

	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			break
		}

		for k, wv := range w {
			gv, has := g[k]
			if !has {
				return at + "." + k + ": missing from the saved file"
			}

			if d := firstDifference(wv, gv, at+"."+k); d != "" {
				return d
			}
		}

		for k := range g {
			if _, has := w[k]; !has {
				return at + "." + k + ": the saved file invented it"
			}
		}
	case []any:
		g, ok := got.([]any)
		if !ok {
			break
		}

		if len(w) != len(g) {
			return at + ": " + itoa(len(w)) + " values in, " + itoa(len(g)) + " out"
		}

		for i := range w {
			if d := firstDifference(w[i], g[i], at+"["+itoa(i)+"]"); d != "" {
				return d
			}
		}
	}

	return at + ": " + valueString(want) + " became " + valueString(got)
}

func valueString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "?"
	}

	if len(b) > 80 {
		b = append(b[:80:80], '.', '.', '.')
	}

	return string(b)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
