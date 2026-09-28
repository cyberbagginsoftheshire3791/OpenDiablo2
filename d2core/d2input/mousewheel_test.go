package d2input

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// wheelService is a minimal InputService that reports a cursor position and a
// wheel roll and nothing else. It is deliberately separate from fakeReal in
// scripted_test.go: that one exists to feed the scripted overlay, this one
// exists to drive inputManager.Advance.
type wheelService struct {
	cursorX, cursorY int
	wheelX, wheelY   float64
	// wheelReads counts Wheel() calls, so a test can prove the manager reads
	// the wheel exactly once per poll.
	wheelReads int
	// keysDown reports these keys as pressed and just-pressed, which is how a
	// test proves the wheel path does not travel as a key.
	keysDown map[d2enum.Key]bool
}

func (w *wheelService) CursorPosition() (int, int) { return w.cursorX, w.cursorY }
func (w *wheelService) InputChars() []rune         { return nil }
func (w *wheelService) IsKeyPressed(k d2enum.Key) bool {
	return w.keysDown[k]
}
func (w *wheelService) IsKeyJustPressed(k d2enum.Key) bool {
	return w.keysDown[k]
}
func (w *wheelService) IsKeyJustReleased(d2enum.Key) bool            { return false }
func (w *wheelService) IsMouseButtonPressed(d2enum.MouseButton) bool { return false }
func (w *wheelService) IsMouseButtonJustPressed(d2enum.MouseButton) bool {
	return false
}
func (w *wheelService) IsMouseButtonJustReleased(d2enum.MouseButton) bool {
	return false
}
func (w *wheelService) KeyPressDuration(d2enum.Key) int { return 0 }
func (w *wheelService) Wheel() (float64, float64) {
	w.wheelReads++
	return w.wheelX, w.wheelY
}

// static check: the fake really is the interface the manager polls.
var _ d2interface.InputService = &wheelService{}

// wheelSpy records every wheel event it is handed.
type wheelSpy struct {
	events []d2interface.MouseWheelEvent
	// prevent is what OnMouseWheel returns, i.e. "I handled it".
	prevent bool
}

func (s *wheelSpy) OnMouseWheel(event d2interface.MouseWheelEvent) bool {
	s.events = append(s.events, event)
	return s.prevent
}

// keySpy records key-down events. It exists only so the wheel tests can show
// the wheel does not arrive as one.
type keySpy struct {
	keys []d2enum.Key
}

func (s *keySpy) OnKeyDown(event d2interface.KeyEvent) bool {
	s.keys = append(s.keys, event.Key())
	return false
}

func advanceOnce(t *testing.T, im d2interface.InputManager) {
	t.Helper()

	if err := im.Advance(0, 0); err != nil {
		t.Fatalf("Advance: %v", err)
	}
}

// TestWheelRollReachesAWheelHandler is the end-to-end plumbing assertion: the
// manager reads the service's wheel once a poll and hands a bound
// MouseWheelHandler the amounts and the cursor position.
func TestWheelRollReachesAWheelHandler(t *testing.T) {
	svc := &wheelService{cursorX: 321, cursorY: 123, wheelX: -2, wheelY: 3}
	im := NewInputManagerWithService(svc)
	spy := &wheelSpy{}

	if err := im.BindHandler(spy); err != nil {
		t.Fatalf("BindHandler: %v", err)
	}

	advanceOnce(t, im)

	if svc.wheelReads != 1 {
		t.Errorf("the manager read the wheel %d times in one poll, want 1", svc.wheelReads)
	}

	if len(spy.events) != 1 {
		t.Fatalf("handler got %d wheel events, want 1", len(spy.events))
	}

	e := spy.events[0]

	if got, want := e.ScrollX(), -2.0; got != want {
		t.Errorf("ScrollX = %v, want %v", got, want)
	}

	if got, want := e.ScrollY(), 3.0; got != want {
		t.Errorf("ScrollY = %v, want %v", got, want)
	}

	if got, want := e.X(), 321; got != want {
		t.Errorf("X = %d, want %d -- a zoom about the cursor needs the cursor", got, want)
	}

	if got, want := e.Y(), 123; got != want {
		t.Errorf("Y = %d, want %d", got, want)
	}
}

// TestWheelEventCarriesTheKeyAndButtonMods pins the half of the event that
// comes from the shared HandlerEvent base, so a wheel event is as qualified as
// a click: ctrl-wheel is a different gesture from wheel.
func TestWheelEventCarriesTheKeyAndButtonMods(t *testing.T) {
	svc := &wheelService{
		wheelY:   1,
		keysDown: map[d2enum.Key]bool{d2enum.KeyControl: true},
	}
	im := NewInputManagerWithService(svc)
	spy := &wheelSpy{}

	if err := im.BindHandler(spy); err != nil {
		t.Fatalf("BindHandler: %v", err)
	}

	advanceOnce(t, im)

	if len(spy.events) != 1 {
		t.Fatalf("handler got %d wheel events, want 1", len(spy.events))
	}

	if mod := spy.events[0].KeyMod(); mod&d2enum.KeyModControl == 0 {
		t.Errorf("KeyMod = %d, want the control bit (%d) set", mod, d2enum.KeyModControl)
	}
}

// TestStillWheelSendsNoEvent is the guard's control. Wheel() returns 0, 0 on
// every frame nothing is being scrolled -- the overwhelming majority of frames
// in a running game -- and an event on each of those would put a zoom handler
// on the hot path of every frame forever.
func TestStillWheelSendsNoEvent(t *testing.T) {
	svc := &wheelService{cursorX: 10, cursorY: 10}
	im := NewInputManagerWithService(svc)
	spy := &wheelSpy{}

	if err := im.BindHandler(spy); err != nil {
		t.Fatalf("BindHandler: %v", err)
	}

	advanceOnce(t, im)
	advanceOnce(t, im)

	if len(spy.events) != 0 {
		t.Errorf("a still wheel produced %d events, want 0", len(spy.events))
	}
}

// TestWheelDoesNotTravelAsAKey is the landmine's guard.
//
// d2enum.KeyMouseWheelUp and KeyMouseWheelDown exist
// (d2common/d2enum/input_key.go:112-113) and are absent from the ebiten
// adapter's keyToEbiten map, so on a real keyboard they report the A key. This
// test holds the two apart from the other end: a service that reports those two
// constants as pressed raises key events and NOT a wheel event, and a service
// that reports a wheel roll raises a wheel event and no key event. If somebody
// later routes the wheel through those constants, one half of this goes red.
func TestWheelDoesNotTravelAsAKey(t *testing.T) {
	t.Run("the wheel keys are keys, not a wheel", func(t *testing.T) {
		svc := &wheelService{keysDown: map[d2enum.Key]bool{
			d2enum.KeyMouseWheelUp:   true,
			d2enum.KeyMouseWheelDown: true,
		}}
		im := NewInputManagerWithService(svc)
		wheel, keys := &wheelSpy{}, &keySpy{}

		if err := im.BindHandler(wheel); err != nil {
			t.Fatalf("BindHandler: %v", err)
		}

		if err := im.BindHandler(keys); err != nil {
			t.Fatalf("BindHandler: %v", err)
		}

		advanceOnce(t, im)

		if len(wheel.events) != 0 {
			t.Errorf("the wheel key constants raised %d wheel events, want 0", len(wheel.events))
		}

		if len(keys.keys) != 2 {
			t.Errorf("the wheel key constants raised %d key events, want 2", len(keys.keys))
		}
	})

	t.Run("a wheel roll is a wheel, not a key", func(t *testing.T) {
		svc := &wheelService{wheelY: 1}
		im := NewInputManagerWithService(svc)
		wheel, keys := &wheelSpy{}, &keySpy{}

		if err := im.BindHandler(wheel); err != nil {
			t.Fatalf("BindHandler: %v", err)
		}

		if err := im.BindHandler(keys); err != nil {
			t.Fatalf("BindHandler: %v", err)
		}

		advanceOnce(t, im)

		if len(wheel.events) != 1 {
			t.Errorf("a wheel roll raised %d wheel events, want 1", len(wheel.events))
		}

		if len(keys.keys) != 0 {
			t.Errorf("a wheel roll raised %d key events, want 0", len(keys.keys))
		}
	})
}

// TestWheelAmountSurvivesAsAnAmount is what a key could never carry. A trackpad
// reports fractions and a wheel notch is not always 1, so the event has to keep
// the number rather than reduce it to "up" or "down".
func TestWheelAmountSurvivesAsAnAmount(t *testing.T) {
	svc := &wheelService{wheelY: 0.25}
	im := NewInputManagerWithService(svc)
	spy := &wheelSpy{}

	if err := im.BindHandler(spy); err != nil {
		t.Fatalf("BindHandler: %v", err)
	}

	advanceOnce(t, im)

	if len(spy.events) != 1 {
		t.Fatalf("handler got %d wheel events, want 1", len(spy.events))
	}

	if got, want := spy.events[0].ScrollY(), 0.25; got != want {
		t.Errorf("ScrollY = %v, want %v", got, want)
	}
}

// TestScriptedServicePassesTheWheelThrough: the playtest overlay sits between
// the manager and the real device on every harness build, so a wheel that it
// swallowed would work in the shipped game and be dead under the harness.
func TestScriptedServicePassesTheWheelThrough(t *testing.T) {
	real := &fakeReal{wheelX: 1.5, wheelY: -4}
	s := NewScriptedInputService(real)

	x, y := s.Wheel()

	if x != 1.5 || y != -4 {
		t.Errorf("Wheel() = (%v, %v), want (1.5, -4)", x, y)
	}
}
