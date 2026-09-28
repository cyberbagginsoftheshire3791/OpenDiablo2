package d2mapedit

import (
	"image"
	"strings"
	"testing"
)

// A GROUP MUST SURVIVE SAVE AND REOPEN, AND THE GAME MUST STILL LOAD THE FILE.
//
// That is the whole bargain of putting editor data in the "note" map property:
// the loader reads note and ignores it (tiled.go:325), so the file still loads,
// and nothing else in a .tmj is free. This test is the bargain, checked both
// ways in one pass.
func TestAGroupSurvivesSaveAndReopenAndTheGameStillLoadsIt(t *testing.T) {
	d := openVillage(t)
	humanNote := d.Note()

	if humanNote == "" {
		t.Fatal("the village's note is empty, so this test cannot show the note surviving")
	}

	if len(d.Groups()) != 0 {
		t.Fatalf("the village already has groups: %v", d.Groups())
	}

	s := NewStack(d)

	for _, g := range []Group{
		{Name: "the south farm", Origin: image.Pt(17, 30), Members: []int{7, 12}},
		{Name: "the churchyard", Origin: image.Pt(23, 13), Members: []int{9, 13}},
	} {
		c, err := d.SetGroup(g)
		if err != nil {
			t.Fatalf("grouping %q: %v", g.Name, err)
		}

		if err := s.Do(c); err != nil {
			t.Fatalf("grouping %q: %v", g.Name, err)
		}
	}

	saved, err := d.Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	// The game still takes it.
	parseVillage(t, saved)

	// And so does the editor, with the groups and the note intact.
	again := mustOpen(t, saved)

	if err := again.GroupsError(); err != nil {
		t.Fatalf("reopening the groups: %v", err)
	}

	if got := again.Note(); got != humanNote {
		t.Errorf("the note came back as %q, want %q", got, humanNote)
	}

	groups := again.Groups()
	if len(groups) != 2 {
		t.Fatalf("%d groups came back, want 2: %v", len(groups), groups)
	}

	farm, ok := again.Group("the south farm")
	if !ok {
		t.Fatal("the south farm did not come back")
	}

	if farm.Origin != image.Pt(17, 30) || len(farm.Members) != 2 || farm.Members[0] != 7 || farm.Members[1] != 12 {
		t.Errorf("the south farm came back as %+v", farm)
	}

	// The note the designer reads still reads: the marker and the JSON are after
	// his text, not instead of it.
	note := noteProperty(t, saved)
	if !strings.HasPrefix(note, humanNote) {
		t.Errorf("the saved note does not begin with the designer's own text:\n%s", note)
	}

	if !strings.Contains(note, NoteMarker) {
		t.Errorf("the saved note has no marker line:\n%s", note)
	}

	// Deleting every group takes the block out again, so a map nobody grouped is
	// the map Tiled wrote.
	for _, name := range []string{"the south farm", "the churchyard"} {
		c, err := again.DeleteGroup(name)
		if err != nil {
			t.Fatalf("ungrouping %q: %v", name, err)
		}

		if err := NewStack(again).Do(c); err != nil {
			t.Fatalf("ungrouping %q: %v", name, err)
		}
	}

	bare, err := again.Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	if got := noteProperty(t, bare); got != humanNote {
		t.Errorf("after ungrouping the note is %q, want the designer's text back", got)
	}

	if string(bare) != string(villageBytes(t)) {
		t.Errorf("grouping and ungrouping did not give the file back; first difference at %d",
			firstByteDifference(villageBytes(t), bare))
	}
}

// noteProperty reads the raw "note" map property out of saved bytes, block and
// all, without going through decodeNote.
func noteProperty(t *testing.T, data []byte) string {
	t.Helper()

	tree, err := parseTree(data)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	for _, raw := range fieldArray(tree, "properties") {
		if p, ok := asObject(raw); ok && fieldString(p, "name") == "note" {
			return fieldString(p, "value")
		}
	}

	return ""
}

// A map with NO note at all must still be groupable, and the property must
// appear as a legal one the loader accepts.
func TestAMapWithNoNoteCanStillCarryGroups(t *testing.T) {
	m, files := fixture(t)
	d := mustOpen(t, m.bytes(t))

	if d.Note() != "" {
		t.Fatalf("the fixture has a note: %q", d.Note())
	}

	c, err := d.SetGroup(Group{Name: "the yard", Origin: image.Pt(1, 1), Members: []int{3, 4}})
	if err != nil {
		t.Fatalf("grouping: %v", err)
	}

	if err := NewStack(d).Do(c); err != nil {
		t.Fatalf("grouping: %v", err)
	}

	saved, err := d.Bytes()
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	again := mustOpen(t, saved)

	if g, ok := again.Group("the yard"); !ok || g.Origin != image.Pt(1, 1) || len(g.Members) != 2 {
		t.Errorf("the group came back as %+v (found %v)", g, ok)
	}

	if again.Note() != "" {
		t.Errorf("a note appeared out of nowhere: %q", again.Note())
	}

	// Still a map the game takes, and one the validator passes.
	if problems := again.Validate(files.size()); len(problems) > 0 {
		t.Errorf("after grouping: %v", problems)
	}
}

// A note whose editor block cannot be read is kept WHOLE, and the editor says
// so instead of throwing somebody's sentences or somebody's groups away.
func TestACorruptEditorBlockKeepsTheNoteWhole(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, block string }{
		{"not json", "{this is not json"},
		{"a version from the future", `{"version":99,"groups":[]}`},
		{"nothing at all", ""},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, _ := fixture(t)
			note := "a real note.\n\n" + NoteMarker + "\n" + tc.block
			m["properties"] = append(m["properties"].([]any), tmj{"name": "note", "type": "string", "value": note})

			d := mustOpen(t, m.bytes(t))

			if d.GroupsError() == nil {
				t.Fatal("a broken editor block was read without complaint")
			}

			if d.Note() != note {
				t.Errorf("the note came back as %q, want the whole thing", d.Note())
			}

			if len(d.Groups()) != 0 {
				t.Errorf("groups came out of a broken block: %v", d.Groups())
			}

			// And a group edit is refused rather than writing a second marker.
			c, err := d.SetGroup(Group{Name: "g", Members: []int{3}})
			if err != nil {
				t.Fatalf("building the command: %v", err)
			}

			if err := NewStack(d).Do(c); err == nil {
				t.Error("a group was written over a note the editor cannot read")
			}
		})
	}
}

// The measured finding this design rests on: a structure object cannot carry a
// property either, so there is nowhere per-object to keep a group id. Measured
// against the loader, not read off a comment.
func TestNoObjectCanCarryEditorData(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		add  func(m tmj)
	}{
		{"a structure", func(m tmj) { m.object(3)["properties"] = editorProp() }},
		{"a player_start", func(m tmj) { m.object(1)["properties"] = editorProp() }},
		{"an inside area", func(m tmj) { m.object(2)["properties"] = editorProp() }},
		{"an npc", func(m tmj) {
			m.object(4)["properties"] = append(m.object(4)["properties"].([]any), editorProp()...)
		}},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, files := fixture(t)
			tc.add(m)

			if _, err := parseFixture(t, m, files); err == nil {
				t.Fatal("the loader ACCEPTS a group id on this object, so it could have gone there after all")
			}
		})
	}

	// And the one place that IS free: the note, with anything in it.
	t.Run("the note", func(t *testing.T) {
		t.Parallel()

		m, files := fixture(t)
		m["properties"] = append(m["properties"].([]any),
			tmj{"name": "note", "type": "string", "value": "anything at all " + NoteMarker + ` {"version":1,"groups":[]}`})

		if _, err := parseFixture(t, m, files); err != nil {
			t.Fatalf("the loader refuses a note: %v", err)
		}
	})
}

func editorProp() []any {
	return []any{tmj{"name": "strigoi_group", "type": "string", "value": "the south farm"}}
}
