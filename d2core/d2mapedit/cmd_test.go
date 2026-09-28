package d2mapedit

import (
	"errors"
	"image"
	"testing"
)

// UNDO AFTER A GROUPED MOVE. The named acceptance test for this burst.
//
// A group is a whole farm or a monastery: moving it touches several objects and
// the group's own origin, and it has to come back in ONE undo. The failure this
// guards is the easy one to ship -- a grouped move that undoes one member, or
// leaves the origin where it was dragged to, so the next drag starts from a
// handle that is not where the buildings are.
func TestUndoAfterAGroupedMove(t *testing.T) {
	d := openVillage(t)
	s := NewStack(d)

	// The four speakers and the house one of them stands by, as one group.
	members := []int{3, 10, 11, 12, 13}
	origin := image.Pt(24, 24)

	group, err := d.SetGroup(Group{Name: "the square", Origin: origin, Members: members})
	if err != nil {
		t.Fatalf("grouping: %v", err)
	}

	if err := s.Do(group); err != nil {
		t.Fatalf("grouping: %v", err)
	}

	was := map[int][2]float64{}
	for _, id := range members {
		o, ok := d.Object(id)
		if !ok {
			t.Fatalf("no object %d", id)
		}

		was[id] = [2]float64{o.X, o.Y}
	}

	const dx, dy = 2, -3

	move, err := d.MoveGroup("the square", dx, dy)
	if err != nil {
		t.Fatalf("moving the group: %v", err)
	}

	// ONE command, however many members it holds.
	batch, isBatch := move.(*Batch)
	if !isBatch {
		t.Fatalf("MoveGroup returned %T, want one composite command", move)
	}

	if batch.Len() != len(members)+1 {
		t.Errorf("the batch holds %d commands, want one per member plus the origin", batch.Len())
	}

	depth := s.Depth()

	if err := s.Do(move); err != nil {
		t.Fatalf("moving the group: %v", err)
	}

	if got := s.Depth() - depth; got != 1 {
		t.Errorf("a grouped move put %d commands on the stack; a group is one edit", got)
	}

	// Everything moved, sub-tile offsets kept.
	for _, id := range members {
		o, _ := d.Object(id)
		wantX, wantY := was[id][0]+dx, was[id][1]+dy

		if o.X != wantX || o.Y != wantY {
			t.Errorf("object %d is at %.2f,%.2f after the move, want %.2f,%.2f", id, o.X, o.Y, wantX, wantY)
		}
	}

	if g, _ := d.Group("the square"); g.Origin != origin.Add(image.Pt(dx, dy)) {
		t.Errorf("the group's origin is %v after the move, want %v", g.Origin, origin.Add(image.Pt(dx, dy)))
	}

	// ONE undo puts all of it back.
	if err := s.Undo(); err != nil {
		t.Fatalf("undoing the grouped move: %v", err)
	}

	for _, id := range members {
		o, _ := d.Object(id)
		if o.X != was[id][0] || o.Y != was[id][1] {
			t.Errorf("after one undo object %d is at %.2f,%.2f, want %.2f,%.2f back",
				id, o.X, o.Y, was[id][0], was[id][1])
		}
	}

	if g, ok := d.Group("the square"); !ok || g.Origin != origin {
		t.Errorf("after one undo the group's origin is %v, want %v back", g.Origin, origin)
	}

	// And redo does the whole thing again.
	if err := s.Redo(); err != nil {
		t.Fatalf("redoing the grouped move: %v", err)
	}

	for _, id := range members {
		o, _ := d.Object(id)
		if o.X != was[id][0]+dx || o.Y != was[id][1]+dy {
			t.Errorf("after redo object %d is at %.2f,%.2f", id, o.X, o.Y)
		}
	}
}

func TestTheStackWalksBackAndForwards(t *testing.T) {
	d := openVillage(t)
	s := NewStack(d)

	if s.CanUndo() || s.CanRedo() {
		t.Error("a fresh stack can already undo or redo")
	}

	if !errors.Is(s.Undo(), ErrNothingToUndo) {
		t.Error("undoing an empty stack did not say there was nothing to undo")
	}

	if !errors.Is(s.Redo(), ErrNothingToRedo) {
		t.Error("redoing an empty stack did not say there was nothing to redo")
	}

	first, err := d.SetFloorTile(0, 0, 4)
	if err != nil {
		t.Fatal(err)
	}

	second, err := d.SetFloorTile(0, 1, 4)
	if err != nil {
		t.Fatal(err)
	}

	was0, was1 := d.FloorTile(0, 0), d.FloorTile(0, 1)

	for _, c := range []Cmd{first, second} {
		if err := s.Do(c); err != nil {
			t.Fatal(err)
		}
	}

	if !s.CanUndo() || s.CanRedo() {
		t.Error("after two edits the stack should undo and not redo")
	}

	if s.UndoLabel() != second.Label() {
		t.Errorf("UndoLabel %q, want %q", s.UndoLabel(), second.Label())
	}

	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}

	if d.FloorTile(0, 1) != was1 {
		t.Error("the undo did not put the second tile back")
	}

	if s.RedoLabel() != second.Label() {
		t.Errorf("RedoLabel %q, want %q", s.RedoLabel(), second.Label())
	}

	if err := s.Redo(); err != nil {
		t.Fatal(err)
	}

	if d.FloorTile(0, 1) != 4 {
		t.Error("the redo did not do the second edit again")
	}

	// A new edit after an undo drops the redo history: it described a future
	// that no longer follows.
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}

	third, err := d.SetFloorTile(0, 2, 4)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Do(third); err != nil {
		t.Fatal(err)
	}

	if s.CanRedo() {
		t.Error("the redo history survived a new edit")
	}

	// Back to the start.
	for s.CanUndo() {
		if err := s.Undo(); err != nil {
			t.Fatal(err)
		}
	}

	if d.FloorTile(0, 0) != was0 || d.FloorTile(0, 1) != was1 {
		t.Error("undoing everything did not put both tiles back")
	}
}

// A command whose Do fails must not go on the stack: an undo that replays an
// edit which never happened is worse than no undo.
func TestAFailedCommandDoesNotJoinTheHistory(t *testing.T) {
	d := openVillage(t)
	s := NewStack(d)

	if err := s.Do(&failing{}); err == nil {
		t.Fatal("a failing command was accepted")
	}

	if s.CanUndo() {
		t.Error("a failing command went on the undo stack")
	}

	if err := s.Do(nil); err == nil {
		t.Error("a nil command was accepted")
	}
}

type failing struct{}

func (*failing) Label() string   { return "a command that fails" }
func (*failing) Do(*Doc) error   { return errors.New("no") }
func (*failing) Undo(*Doc) error { return nil }

// A batch undoes in reverse, and a batch whose member fails halfway rolls back
// the members that succeeded rather than leaving half an edit behind.
func TestABatchIsAllOrNothing(t *testing.T) {
	d := openVillage(t)

	good, err := d.SetFloorTile(0, 0, 4)
	if err != nil {
		t.Fatal(err)
	}

	was := d.FloorTile(0, 0)
	order := []string{}

	batch := NewBatch("a broken group move",
		&recording{name: "first", into: &order},
		good,
		&recording{name: "second", into: &order},
		&failing{},
	)

	if err := batch.Do(d); err == nil {
		t.Fatal("a batch with a failing member succeeded")
	}

	if got := d.FloorTile(0, 0); got != was {
		t.Errorf("tile 0,0 is %d after the rolled-back batch, want %d", got, was)
	}

	// The two recorders were done in order and undone in reverse.
	want := []string{"do first", "do second", "undo second", "undo first"}
	if len(order) != len(want) {
		t.Fatalf("the batch did %v, want %v", order, want)
	}

	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("the batch did %v, want %v", order, want)
		}
	}
}

type recording struct {
	name string
	into *[]string
}

func (r *recording) Label() string { return r.name }

func (r *recording) Do(*Doc) error {
	*r.into = append(*r.into, "do "+r.name)

	return nil
}

func (r *recording) Undo(*Doc) error {
	*r.into = append(*r.into, "undo "+r.name)

	return nil
}
