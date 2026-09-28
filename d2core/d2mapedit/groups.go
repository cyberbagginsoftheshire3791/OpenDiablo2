package d2mapedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strings"
)

// GROUPS LIVE IN THE "note" MAP PROPERTY, AND THERE IS NOWHERE ELSE.
//
// A group is editor-only data: a name, an origin and the object ids that move
// together, so a whole farm or a monastery is dragged as one thing. The .tmj has
// exactly one place that will carry it and still load:
//
//   - "note" is READ AND IGNORED by the loader -- mapProperties' case "note"
//     has an empty body and does not even check its type (tiled.go:325). Any
//     other map property is a hard refusal (:335).
//   - There is no per-object place at all. MEASURED by reading the loader: a
//     player_start takes no properties (tiled.go:932), an npc takes ONLY
//     "monstat" and refuses any other name (:1084), an inside area takes no
//     properties (:1048), and -- the one the brief asked about -- structure()
//     refuses a structure object that carries ANY property or any rotation
//     (tiled.go:987-989). So a group id cannot ride on its members.
//   - A tileset tile's properties are refused outside the four the loader reads
//     (tiled.go:857), so the palette is no good either.
//
// So: the human's note text is kept exactly as it was, and the editor's data
// goes after a marker line in a clearly-machine-readable block. A .tmj with no
// groups gets no block at all, so an untouched map's note is untouched and its
// bytes do not move.

// NoteMarker separates a designer's note from the editor's own data inside the
// "note" map property. Everything before it is the note; everything after it is
// one line of JSON the editor wrote.
const NoteMarker = "--- strigoi world editor data below: machine-written, edit in the editor ---"

// noteVersion is bumped when the editor block's shape changes incompatibly. An
// older editor reading a newer block keeps the block (it is in the note) and
// reports the error rather than silently dropping somebody's groups.
const noteVersion = 1

// Group is a named set of objects that move together, with the origin the
// editor drags them by.
type Group struct {
	Name string `json:"name"`
	// Origin is in whole tiles: where the group's handle sits.
	Origin image.Point `json:"origin"`
	// Members are object ids, in the order the designer added them.
	Members []int `json:"members"`
}

// noteBlock is the JSON the editor writes after NoteMarker.
type noteBlock struct {
	Version int     `json:"version"`
	Groups  []Group `json:"groups"`
}

// Note is the designer's own note text, with the editor's block taken off.
func (d *Doc) Note() string {
	return d.m.note
}

// Groups is every group, by name.
func (d *Doc) Groups() []Group {
	out := make([]Group, len(d.m.groups))

	for i, g := range d.m.groups {
		out[i] = g
		out[i].Members = append([]int(nil), g.Members...)
	}

	return out
}

// Group is the group with this name.
func (d *Doc) Group(name string) (Group, bool) {
	for _, g := range d.Groups() {
		if g.Name == name {
			return g, true
		}
	}

	return Group{}, false
}

// GroupsError is what went wrong reading the editor's block out of the "note"
// property, or nil. A note whose block cannot be read is kept WHOLE as the
// note -- nobody's groups and nobody's sentences are thrown away to make the
// document open -- and this says so.
func (d *Doc) GroupsError() error {
	return d.m.groupsErr
}

// decodeNote splits a "note" property into the human's text and the editor's
// groups.
func decodeNote(note string) (human string, groups []Group, err error) {
	i := strings.Index(note, NoteMarker)
	if i < 0 {
		return note, nil, nil
	}

	human = strings.TrimRight(note[:i], "\n")
	rest := strings.TrimSpace(note[i+len(NoteMarker):])

	var block noteBlock
	if jerr := json.Unmarshal([]byte(rest), &block); jerr != nil {
		return note, nil, fmt.Errorf("the editor's block in the \"note\" property is not readable: %w", jerr)
	}

	if block.Version != noteVersion {
		return note, nil, fmt.Errorf("the editor's block in the \"note\" property is version %d, this editor writes %d", block.Version, noteVersion)
	}

	return human, block.Groups, nil
}

// encodeNote puts a note and its groups back together. With no groups the
// marker is left off entirely, so a map nobody grouped keeps the note it had.
func encodeNote(human string, groups []Group) (string, error) {
	if len(groups) == 0 {
		return human, nil
	}

	raw, err := json.Marshal(noteBlock{Version: noteVersion, Groups: groups})
	if err != nil {
		return "", err
	}

	if human == "" {
		return NoteMarker + "\n" + string(raw), nil
	}

	return human + "\n\n" + NoteMarker + "\n" + string(raw), nil
}

// ---- the group edits ------------------------------------------------------

// groupsCmd replaces the whole group list, which is the simplest thing that is
// correct: the list lives in one string, so every group edit is one write of
// that string and one restore of the old one.
type groupsCmd struct {
	label string
	want  []Group
	had   []Group
	first bool
}

func (c *groupsCmd) Label() string {
	return c.label
}

func (c *groupsCmd) Do(d *Doc) error {
	if !c.first {
		c.had = d.Groups()
		c.first = true
	}

	return d.writeGroups(c.want)
}

func (c *groupsCmd) Undo(d *Doc) error {
	return d.writeGroups(c.had)
}

// writeGroups puts a group list into the "note" map property and rebuilds the
// read model.
func (d *Doc) writeGroups(groups []Group) error {
	// A note whose editor block cannot be read is kept whole as the note, so
	// writing a new block would leave two markers in it and lose the old one.
	// Refuse instead: somebody must look at that note by hand.
	if d.m.groupsErr != nil {
		return fmt.Errorf("cannot write groups: %w", d.m.groupsErr)
	}

	note, err := encodeNote(d.m.note, groups)
	if err != nil {
		return err
	}

	if err := d.setMapProperty("note", "string", note); err != nil {
		return err
	}

	d.derive()

	return nil
}

// setMapProperty writes one map property, adding the "properties" array if the
// map has none. Only "note", "sound_env" and "display_name" are legal
// (tiled.go:322-340), so this refuses anything else rather than writing a file
// the game will refuse whole.
func (d *Doc) setMapProperty(name, typ string, value any) error {
	switch name {
	case "note", "sound_env", "display_name":
	default:
		return fmt.Errorf("map property %q would be refused by the game; it reads \"sound_env\" and \"display_name\" and ignores \"note\"", name)
	}

	// An empty note is no note: dropping the property is what keeps a map
	// nobody has annotated byte-identical to the one Tiled wrote.
	if name == "note" && value == "" {
		d.deleteMapProperty(name)

		return nil
	}

	props := fieldArray(d.tree, "properties")

	for _, raw := range props {
		p, ok := asObject(raw)
		if !ok {
			continue
		}

		if fieldString(p, "name") == name {
			p.Set("type", typ)
			p.Set("value", value)

			return nil
		}
	}

	p := newJSONObject()
	p.Set("name", name)
	p.Set("type", typ)
	p.Set("value", value)

	// Appended, NOT sorted. village.tmj's properties are in the order
	// tools/villagemap wrote them (note, sound_env, display_name), which is not
	// alphabetical, and reordering somebody else's lines to insert one of ours
	// is the diff this package exists to avoid.
	d.tree.Set("properties", append(props, p))

	return nil
}

func (d *Doc) deleteMapProperty(name string) {
	props := fieldArray(d.tree, "properties")
	out := props[:0]

	for _, raw := range props {
		if p, ok := asObject(raw); ok && fieldString(p, "name") == name {
			continue
		}

		out = append(out, raw)
	}

	if len(out) == 0 {
		d.tree.Delete("properties")

		return
	}

	d.tree.Set("properties", out)
}

// SetGroup adds a group or replaces the one with the same name. Every member
// must be an object that exists, because a group naming an object nobody can
// see is a move that silently does less than it says.
func (d *Doc) SetGroup(g Group) (Cmd, error) {
	if strings.TrimSpace(g.Name) == "" {
		return nil, errors.New("a group needs a name")
	}

	seen := map[int]bool{}

	for _, id := range g.Members {
		if _, ok := d.Object(id); !ok {
			return nil, fmt.Errorf("group %q names object %d, which the map does not hold", g.Name, id)
		}

		if seen[id] {
			return nil, fmt.Errorf("group %q names object %d twice", g.Name, id)
		}

		seen[id] = true
	}

	g.Members = append([]int(nil), g.Members...)
	want := d.Groups()
	replaced := false

	for i := range want {
		if want[i].Name == g.Name {
			want[i], replaced = g, true
		}
	}

	if !replaced {
		want = append(want, g)
	}

	return &groupsCmd{label: fmt.Sprintf("group %q", g.Name), want: want}, nil
}

// DeleteGroup forgets a group. Its members stay where they are: a group is a
// handle, not a container.
func (d *Doc) DeleteGroup(name string) (Cmd, error) {
	if _, ok := d.Group(name); !ok {
		return nil, fmt.Errorf("no group named %q", name)
	}

	var want []Group

	for _, g := range d.Groups() {
		if g.Name != name {
			want = append(want, g)
		}
	}

	return &groupsCmd{label: fmt.Sprintf("delete group %q", name), want: want}, nil
}

// MoveGroup shifts a whole group by dx, dy WHOLE TILES: every member and the
// group's own origin, as ONE command. The members keep their sub-tile offsets,
// so a villager standing in the middle of his tile still is.
func (d *Doc) MoveGroup(name string, dx, dy int) (Cmd, error) {
	g, ok := d.Group(name)
	if !ok {
		return nil, fmt.Errorf("no group named %q", name)
	}

	cmds := make([]Cmd, 0, len(g.Members)+1)

	for _, id := range g.Members {
		o, found := d.Object(id)
		if !found {
			return nil, fmt.Errorf("group %q names object %d, which the map does not hold", name, id)
		}

		c, err := d.MoveObject(id, o.X+float64(dx), o.Y+float64(dy))
		if err != nil {
			return nil, err
		}

		cmds = append(cmds, c)
	}

	moved := g
	moved.Origin = g.Origin.Add(image.Pt(dx, dy))

	want := d.Groups()
	for i := range want {
		if want[i].Name == name {
			want[i] = moved
		}
	}

	cmds = append(cmds, &groupsCmd{label: "group origin", want: want})

	return NewBatch(fmt.Sprintf("move group %q by %d,%d", name, dx, dy), cmds...), nil
}
