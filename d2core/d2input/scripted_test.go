package d2input

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// fakeReal stands in for the ebiten input service: nothing is pressed unless
// the test says so.
type fakeReal struct {
	x, y    int
	pressed map[d2enum.Key]bool
	chars   []rune
}

func (f *fakeReal) CursorPosition() (int, int)                   { return f.x, f.y }
func (f *fakeReal) InputChars() []rune                           { return f.chars }
func (f *fakeReal) IsKeyPressed(k d2enum.Key) bool               { return f.pressed[k] }
func (f *fakeReal) IsKeyJustPressed(d2enum.Key) bool             { return false }
func (f *fakeReal) IsKeyJustReleased(d2enum.Key) bool            { return false }
func (f *fakeReal) IsMouseButtonPressed(d2enum.MouseButton) bool { return false }
func (f *fakeReal) IsMouseButtonJustPressed(d2enum.MouseButton) bool {
	return false
}
func (f *fakeReal) IsMouseButtonJustReleased(d2enum.MouseButton) bool {
	return false
}
func (f *fakeReal) KeyPressDuration(d2enum.Key) int { return 0 }

// TestScriptedTapIsJustPressedForOneTick is engine change E6's test (P3 spec
// §6.2): a scripted tap is "just pressed" for exactly one poll, "pressed"
// until released, and "just released" on the following poll.
func TestScriptedTapIsJustPressedForOneTick(t *testing.T) {
	s := NewScriptedInputService(&fakeReal{})

	s.KeyTap(d2enum.KeyI)

	// poll 1: the press edge
	if !s.IsKeyJustPressed(d2enum.KeyI) || !s.IsKeyPressed(d2enum.KeyI) || s.IsKeyJustReleased(d2enum.KeyI) {
		t.Fatal("poll 1: want just-pressed + pressed")
	}

	if d := s.KeyPressDuration(d2enum.KeyI); d != 1 {
		t.Fatalf("poll 1: duration %d, want 1", d)
	}

	s.EndTick()

	// poll 2: released
	if s.IsKeyJustPressed(d2enum.KeyI) || s.IsKeyPressed(d2enum.KeyI) || !s.IsKeyJustReleased(d2enum.KeyI) {
		t.Fatal("poll 2: want just-released only")
	}

	s.EndTick()

	// poll 3: idle and forgotten
	if s.IsKeyJustPressed(d2enum.KeyI) || s.IsKeyPressed(d2enum.KeyI) || s.IsKeyJustReleased(d2enum.KeyI) {
		t.Fatal("poll 3: want nothing")
	}

	if len(s.keys) != 0 {
		t.Fatalf("idle keys must be forgotten, have %d", len(s.keys))
	}
}

func TestScriptedHoldAndRelease(t *testing.T) {
	s := NewScriptedInputService(&fakeReal{})

	s.KeyDown(d2enum.KeyShift)

	for poll := 1; poll <= 3; poll++ {
		if !s.IsKeyPressed(d2enum.KeyShift) {
			t.Fatalf("poll %d: shift must stay pressed", poll)
		}

		if got := s.IsKeyJustPressed(d2enum.KeyShift); got != (poll == 1) {
			t.Fatalf("poll %d: just-pressed %v", poll, got)
		}

		if d := s.KeyPressDuration(d2enum.KeyShift); d != poll {
			t.Fatalf("poll %d: duration %d", poll, d)
		}

		s.EndTick()
	}

	s.KeyUp(d2enum.KeyShift)

	if s.IsKeyPressed(d2enum.KeyShift) || !s.IsKeyJustReleased(d2enum.KeyShift) {
		t.Fatal("after KeyUp: want just-released, not pressed")
	}

	s.EndTick()

	if s.IsKeyJustReleased(d2enum.KeyShift) {
		t.Fatal("the release edge lasts one poll")
	}
}

func TestScriptedMergesWithReal(t *testing.T) {
	real := &fakeReal{pressed: map[d2enum.Key]bool{d2enum.KeyA: true}, chars: []rune("r")}
	s := NewScriptedInputService(real)

	if !s.IsKeyPressed(d2enum.KeyA) {
		t.Fatal("a really pressed key must show through")
	}

	s.TypeText("xy")

	if got := string(s.InputChars()); got != "rxy" {
		t.Fatalf("chars: %q, want real then scripted", got)
	}

	s.EndTick()

	if got := string(s.InputChars()); got != "r" {
		t.Fatalf("scripted chars last one poll, got %q", got)
	}
}

func TestScriptedCursorAndClick(t *testing.T) {
	real := &fakeReal{x: 10, y: 20}
	s := NewScriptedInputService(real)

	if x, y := s.CursorPosition(); x != 10 || y != 20 {
		t.Fatalf("pass-through cursor: %d,%d", x, y)
	}

	s.Click(300, 200, d2enum.MouseButtonLeft)

	if x, y := s.CursorPosition(); x != 300 || y != 200 {
		t.Fatalf("scripted cursor: %d,%d", x, y)
	}

	if !s.IsMouseButtonJustPressed(d2enum.MouseButtonLeft) || !s.IsMouseButtonPressed(d2enum.MouseButtonLeft) {
		t.Fatal("click: want just-pressed + pressed on the first poll")
	}

	s.EndTick()

	if s.IsMouseButtonPressed(d2enum.MouseButtonLeft) || !s.IsMouseButtonJustReleased(d2enum.MouseButtonLeft) {
		t.Fatal("click: want released on the second poll")
	}

	// the scripted cursor holds while the real mouse is still...
	if x, y := s.CursorPosition(); x != 300 || y != 200 {
		t.Fatalf("cursor must hold: %d,%d", x, y)
	}

	// ...and yields the moment the human moves the mouse
	real.x = 11

	if x, y := s.CursorPosition(); x != 11 || y != 20 {
		t.Fatalf("real mouse movement must win: %d,%d", x, y)
	}
}

// keyRecorder counts OnKeyDown calls through the input manager.
type keyRecorder struct {
	downs []d2enum.Key
	ups   []d2enum.Key
}

func (r *keyRecorder) OnKeyDown(e d2interface.KeyEvent) bool {
	r.downs = append(r.downs, e.Key())
	return true
}

func (r *keyRecorder) OnKeyUp(e d2interface.KeyEvent) bool {
	r.ups = append(r.ups, e.Key())
	return true
}

// TestInputManagerDeliversScriptedTapOnce drives the real input manager with
// the overlay: one tap, three advances, exactly one OnKeyDown and one OnKeyUp.
func TestInputManagerDeliversScriptedTapOnce(t *testing.T) {
	s := NewScriptedInputService(&fakeReal{})
	im := NewInputManagerWithService(s)
	rec := &keyRecorder{}

	if err := im.BindHandler(rec); err != nil {
		t.Fatal(err)
	}

	s.KeyTap(d2enum.KeyI)

	for i := 0; i < 3; i++ {
		if err := im.Advance(0, 0); err != nil {
			t.Fatal(err)
		}
	}

	if len(rec.downs) != 1 || rec.downs[0] != d2enum.KeyI {
		t.Fatalf("OnKeyDown calls: %v, want exactly one KeyI", rec.downs)
	}

	if len(rec.ups) != 1 || rec.ups[0] != d2enum.KeyI {
		t.Fatalf("OnKeyUp calls: %v, want exactly one KeyI", rec.ups)
	}
}

func TestKeyByName(t *testing.T) {
	cases := map[string]d2enum.Key{
		"i": d2enum.KeyI, "I": d2enum.KeyI, "KeyI": d2enum.KeyI, "escape": d2enum.KeyEscape, "esc": d2enum.KeyEscape,
		"Enter": d2enum.KeyEnter, "return": d2enum.KeyEnter, "f5": d2enum.KeyF5, "ctrl": d2enum.KeyControl,
		"`": d2enum.KeyGraveAccent, "space": d2enum.KeySpace, " ": d2enum.KeySpace, "kp7": d2enum.KeyKP7,
	}

	for name, want := range cases {
		got, err := KeyByName(name)
		if err != nil || got != want {
			t.Errorf("KeyByName(%q) = %v, %v; want %v", name, got, err, want)
		}
	}

	if _, err := KeyByName("bogus"); err == nil {
		t.Error("bogus must fail")
	}

	if KeyName(d2enum.KeyEscape) != "escape" || KeyName(d2enum.KeyI) != "i" {
		t.Error("KeyName round trip")
	}

	if b, err := MouseButtonByName("Right"); err != nil || b != d2enum.MouseButtonRight {
		t.Error("MouseButtonByName right")
	}

	if _, err := MouseButtonByName("x1"); err == nil {
		t.Error("unknown button must fail")
	}
}
