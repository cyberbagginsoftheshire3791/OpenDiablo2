package d2mapedit

import (
	"errors"
	"fmt"
)

// Cmd is one undoable edit. Every edit operation returns one instead of
// changing the document, so the caller decides whether it goes on a stack.
//
// Do and Undo take the document rather than closing over it, so a command is a
// value the screen can hold, log and label without owning a Doc.
type Cmd interface {
	Do(*Doc) error
	Undo(*Doc) error
	Label() string
}

// Batch is several commands as ONE edit: one label, one undo. A grouped move --
// a whole farm, a monastery -- is a Batch of moves, and undoing it puts every
// member back in one step, which is the behaviour a designer expects and the
// thing that is easiest to get wrong.
type Batch struct {
	label string
	cmds  []Cmd
}

// NewBatch makes a composite command. Nothing is run until Do.
func NewBatch(label string, cmds ...Cmd) *Batch {
	return &Batch{label: label, cmds: cmds}
}

// Len is how many commands the batch holds.
func (b *Batch) Len() int {
	return len(b.cmds)
}

// Label is the batch's own label.
func (b *Batch) Label() string {
	return b.label
}

// Do runs the members in order. IF ONE FAILS, THE ONES THAT SUCCEEDED ARE
// UNDONE before the error is returned: half an edit on the undo stack is worse
// than no edit, because the next undo would only put half of it back.
func (b *Batch) Do(d *Doc) error {
	for i, c := range b.cmds {
		if err := c.Do(d); err != nil {
			for j := i - 1; j >= 0; j-- {
				_ = b.cmds[j].Undo(d)
			}

			return fmt.Errorf("%s: %w", b.label, err)
		}
	}

	return nil
}

// Undo undoes the members in REVERSE order, which is the only order that is
// correct in general: a later member may depend on an earlier one.
func (b *Batch) Undo(d *Doc) error {
	for i := len(b.cmds) - 1; i >= 0; i-- {
		if err := b.cmds[i].Undo(d); err != nil {
			return fmt.Errorf("%s: %w", b.label, err)
		}
	}

	return nil
}

// ErrNothingToUndo and ErrNothingToRedo are what an empty stack answers.
var (
	ErrNothingToUndo = errors.New("nothing to undo")
	ErrNothingToRedo = errors.New("nothing to redo")
)

// Stack is the editor's undo/redo history over one document.
//
// IT ALSO KNOWS WHICH POINT IN THE HISTORY IS ON DISK (28 Sep review, C): the
// editor's unsaved marker used to be a flag set by every edit and cleared only by
// a save, so undoing back to the saved map still said *UNSAVED*. saved is the
// depth of the history when the document last matched the file -- 0 when it was
// opened -- and Dirty is simply "the history is not at that depth". A new edit
// made below it throws away the redo branch the saved state was on, so the file
// can no longer be reached by undo or redo and saved becomes unreachable (-1).
type Stack struct {
	doc   *Doc
	done  []Cmd
	redo  []Cmd
	saved int
}

// NewStack starts a history over a document.
func NewStack(d *Doc) *Stack {
	return &Stack{doc: d}
}

// Doc is the document the stack edits.
func (s *Stack) Doc() *Doc {
	return s.doc
}

// Do runs a command and pushes it. A command that FAILS is not pushed -- an
// undo must never replay an edit that did not happen -- and the redo history is
// dropped, because it described a future that no longer follows.
func (s *Stack) Do(c Cmd) error {
	if c == nil {
		return errors.New("no command")
	}

	if err := c.Do(s.doc); err != nil {
		return err
	}

	// Below the saved depth, the saved state lives only in the redo branch
	// this edit is about to drop.
	if len(s.done) < s.saved {
		s.saved = -1
	}

	s.done = append(s.done, c)
	s.redo = nil

	return nil
}

// MarkSaved records that the document as it now stands is what is on disk: the
// editor calls it after a save succeeds.
func (s *Stack) MarkSaved() {
	s.saved = len(s.done)
}

// Dirty reports whether the document differs from the last save (or from the
// file it was opened from, before any save): whether undo and redo have left
// the history anywhere but the point MarkSaved recorded.
func (s *Stack) Dirty() bool {
	return len(s.done) != s.saved
}

// CanUndo reports whether there is anything to undo.
func (s *Stack) CanUndo() bool {
	return len(s.done) > 0
}

// CanRedo reports whether there is anything to redo.
func (s *Stack) CanRedo() bool {
	return len(s.redo) > 0
}

// UndoLabel is what the next undo would undo, "" for nothing.
func (s *Stack) UndoLabel() string {
	if !s.CanUndo() {
		return ""
	}

	return s.done[len(s.done)-1].Label()
}

// RedoLabel is what the next redo would do again, "" for nothing.
func (s *Stack) RedoLabel() string {
	if !s.CanRedo() {
		return ""
	}

	return s.redo[len(s.redo)-1].Label()
}

// Undo takes the last command back.
func (s *Stack) Undo() error {
	if !s.CanUndo() {
		return ErrNothingToUndo
	}

	c := s.done[len(s.done)-1]
	if err := c.Undo(s.doc); err != nil {
		return err
	}

	s.done = s.done[:len(s.done)-1]
	s.redo = append(s.redo, c)

	return nil
}

// Redo does the last undone command again.
func (s *Stack) Redo() error {
	if !s.CanRedo() {
		return ErrNothingToRedo
	}

	c := s.redo[len(s.redo)-1]
	if err := c.Do(s.doc); err != nil {
		return err
	}

	s.redo = s.redo[:len(s.redo)-1]
	s.done = append(s.done, c)

	return nil
}

// Depth is how many commands could be undone.
func (s *Stack) Depth() int {
	return len(s.done)
}
