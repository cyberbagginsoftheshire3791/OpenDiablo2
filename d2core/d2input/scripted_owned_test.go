package d2input

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// restlessReal is a human working at the laptop while the suite runs: every
// read of the real cursor finds it somewhere new, the left button is held and
// the wheel rolls (BUG-111).
type restlessReal struct {
	fakeReal
	reads int
}

func (r *restlessReal) CursorPosition() (int, int) {
	r.reads++

	return 100 + 37*r.reads, 50 + 11*r.reads
}

func (r *restlessReal) IsMouseButtonPressed(d2enum.MouseButton) bool      { return true }
func (r *restlessReal) IsMouseButtonJustPressed(d2enum.MouseButton) bool  { return true }
func (r *restlessReal) IsMouseButtonJustReleased(d2enum.MouseButton) bool { return true }
func (r *restlessReal) Wheel() (float64, float64)                         { return 0, 1 }

// moveRecorder counts the mouse-move events the input manager delivers and
// where they said the cursor was.
type moveRecorder struct {
	moves  int
	lastX  int
	lastY  int
	clicks int
}

func (m *moveRecorder) OnMouseMove(e d2interface.MouseMoveEvent) bool {
	m.moves++
	m.lastX, m.lastY = e.X(), e.Y()

	return false
}

func (m *moveRecorder) OnMouseButtonDown(d2interface.MouseEvent) bool {
	m.clicks++

	return false
}

// TestAnOwnedMouseIgnoresTheRealOne is BUG-111: a harness game's scripted
// cursor stays where the script put it while the real mouse moves, and the
// real buttons and wheel reach nothing. Five readings of a hover used to give
// five positions 0.7 s apart.
func TestAnOwnedMouseIgnoresTheRealOne(t *testing.T) {
	real := &restlessReal{}
	s := NewScriptedInputService(real)
	s.OwnMouse()

	if !s.MouseOwned() {
		t.Fatal("OwnMouse did not take")
	}

	// Before a script has placed it: parked off the window, every poll -- the
	// literal -1,-1 (the review's C6: comparing against the constants let a
	// cursor parked ON the window, at 0,0, pass), where nothing is hovered.
	for i := 0; i < 3; i++ {
		if x, y := s.CursorPosition(); x != -1 || y != -1 {
			t.Errorf("poll %d before any scripted cursor: %d,%d, want parked off the window at -1,-1", i, x, y)

			break
		}
	}

	if ParkedCursorX != -1 || ParkedCursorY != -1 {
		t.Errorf("the parked cursor is %d,%d; the docs and the harness say -1,-1", ParkedCursorX, ParkedCursorY)
	}

	s.MoveCursor(300, 200)

	for i := 0; i < 5; i++ {
		if x, y := s.CursorPosition(); x != 300 || y != 200 {
			t.Errorf("reading %d while the real mouse moves: %d,%d, want the scripted 300,200", i, x, y)

			break
		}

		s.EndTick()
	}

	for _, b := range []d2enum.MouseButton{d2enum.MouseButtonLeft, d2enum.MouseButtonRight, d2enum.MouseButtonMiddle} {
		if s.IsMouseButtonPressed(b) || s.IsMouseButtonJustPressed(b) || s.IsMouseButtonJustReleased(b) {
			t.Fatalf("button %v: the real button reached the game", b)
		}
	}

	if x, y := s.Wheel(); x != 0 || y != 0 {
		t.Fatalf("wheel %v,%v: the real wheel reached the game", x, y)
	}

	// A scripted click still lands, and only it.
	s.Click(400, 250, d2enum.MouseButtonLeft)

	if !s.IsMouseButtonJustPressed(d2enum.MouseButtonLeft) || s.IsMouseButtonJustPressed(d2enum.MouseButtonRight) {
		t.Fatal("the scripted click is the only press")
	}

	// Through the real input manager: the script's one move is the only
	// move event, however often the real cursor moved.
	s2 := NewScriptedInputService(&restlessReal{})
	s2.OwnMouse()

	im := NewInputManagerWithService(s2)
	rec := &moveRecorder{}

	if err := im.BindHandler(rec); err != nil {
		t.Fatal(err)
	}

	s2.MoveCursor(320, 240)

	for i := 0; i < 6; i++ {
		if err := im.Advance(0, 0); err != nil {
			t.Fatal(err)
		}
	}

	if rec.moves != 1 || rec.lastX != 320 || rec.lastY != 240 {
		t.Fatalf("move events %d, last at %d,%d; want exactly one, at 320,240", rec.moves, rec.lastX, rec.lastY)
	}

	if rec.clicks != 0 {
		t.Fatalf("%d button-down events from the real mouse, want 0", rec.clicks)
	}
}

// TestAnUnownedMouseStillYieldsToTheHuman is the other side of the line: a
// harness build run by hand (no -harness) keeps the real mouse.
func TestAnUnownedMouseStillYieldsToTheHuman(t *testing.T) {
	real := &restlessReal{}
	s := NewScriptedInputService(real)

	s.MoveCursor(300, 200)
	s.CursorPosition() // the first reading only records where the real one is

	if x, y := s.CursorPosition(); x == 300 && y == 200 {
		t.Fatal("an unowned scripted cursor must yield when the real mouse moves")
	}

	if !s.IsMouseButtonPressed(d2enum.MouseButtonLeft) {
		t.Fatal("an unowned mouse's real button must show through")
	}

	if _, y := s.Wheel(); y != 1 {
		t.Fatal("an unowned mouse's real wheel must show through")
	}
}

// typingReal is the same restless human, typing as well: the I key held
// (pressed this poll, for a while), a character on the line.
type typingReal struct {
	restlessReal
}

func (r *typingReal) IsKeyPressed(k d2enum.Key) bool      { return k == d2enum.KeyI }
func (r *typingReal) IsKeyJustPressed(k d2enum.Key) bool  { return k == d2enum.KeyI }
func (r *typingReal) IsKeyJustReleased(k d2enum.Key) bool { return k == d2enum.KeyJ }
func (r *typingReal) KeyPressDuration(k d2enum.Key) int {
	if k == d2enum.KeyI {
		return 7
	}

	return 0
}
func (r *typingReal) InputChars() []rune { return []rune("x") }

// TestAnOwnedMouseLeavesTheKeyboardAlone is the line's other edge (the
// review's R3): owning the mouse takes the mouse only. A real key -- held,
// pressed, released, typed -- still reaches the game, as it does in a game
// the harness does not own.
func TestAnOwnedMouseLeavesTheKeyboardAlone(t *testing.T) {
	s := NewScriptedInputService(&typingReal{})
	s.OwnMouse()

	if !s.IsKeyPressed(d2enum.KeyI) || !s.IsKeyJustPressed(d2enum.KeyI) {
		t.Fatal("the real I key, held and just pressed, must reach a game whose mouse is owned")
	}

	if !s.IsKeyJustReleased(d2enum.KeyJ) {
		t.Fatal("the real J key's release must reach a game whose mouse is owned")
	}

	if d := s.KeyPressDuration(d2enum.KeyI); d != 7 {
		t.Fatalf("the real I key held 7 polls reads %d with the mouse owned", d)
	}

	if c := s.InputChars(); string(c) != "x" {
		t.Fatalf("the real typed character reads %q with the mouse owned, want \"x\"", string(c))
	}

	// And the mouse is still owned: the control that this is an owned game.
	if x, y := s.CursorPosition(); x != -1 || y != -1 {
		t.Fatalf("the cursor %d,%d: this game must own the mouse", x, y)
	}
}
