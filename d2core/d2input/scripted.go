package d2input

import (
	"sync"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// TickEnder is optionally implemented by an InputService that keeps per-poll
// edge state (a scripted "just pressed" that must be visible for exactly one
// poll). The input manager calls EndTick after each full poll cycle.
type TickEnder interface {
	EndTick()
}

// scriptedButton is one scripted key or mouse button.
type scriptedButton struct {
	down         bool
	justPressed  bool
	justReleased bool
	frames       int  // polls the button has been down (KeyPressDuration)
	releaseNext  bool // a tap: release at the end of the poll that saw the press
}

// ScriptedInputService overlays scripted keyboard and mouse state on a real
// InputService (P3 spec §3.7, engine change E6). A scripted press is "just
// pressed" for exactly one poll and "pressed" until released; a tap releases
// after one poll. Scripted state merges with the real device state, so a
// human at the keyboard is never locked out. The playtest harness drives it
// through strigoi_key / strigoi_click / strigoi_move_cursor /
// strigoi_type_text; with nothing scripted it is a transparent pass-through.
//
// Mutations and polls both happen on the game goroutine (the harness queues
// them); the mutex only makes the type safe to test and to extend.
type ScriptedInputService struct {
	real d2interface.InputService

	mu      sync.Mutex
	keys    map[d2enum.Key]*scriptedButton
	buttons map[d2enum.MouseButton]*scriptedButton
	chars   []rune // delivered by the next InputChars poll

	cursor               *[2]int // scripted cursor; cleared when the real cursor moves
	lastRealX, lastRealY int
	realSeen             bool
}

// NewScriptedInputService wraps a real input service.
func NewScriptedInputService(real d2interface.InputService) *ScriptedInputService {
	return &ScriptedInputService{
		real:    real,
		keys:    map[d2enum.Key]*scriptedButton{},
		buttons: map[d2enum.MouseButton]*scriptedButton{},
	}
}

// ---------------------------------------------------------------- scripting --

func pressButton(b *scriptedButton, tap bool) {
	if !b.down {
		b.justPressed = true
		b.frames = 0
	}

	b.down = true
	b.justReleased = false
	b.releaseNext = tap
}

func releaseButton(b *scriptedButton) {
	if b.down {
		b.justReleased = true
	}

	b.down = false
	b.releaseNext = false
}

// KeyDown holds a key until KeyUp.
func (s *ScriptedInputService) KeyDown(k d2enum.Key) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pressButton(s.key(k), false)
}

// KeyUp releases a scripted key.
func (s *ScriptedInputService) KeyUp(k d2enum.Key) {
	s.mu.Lock()
	defer s.mu.Unlock()

	releaseButton(s.key(k))
}

// KeyTap presses a key for exactly one poll.
func (s *ScriptedInputService) KeyTap(k d2enum.Key) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pressButton(s.key(k), true)
}

// MouseDown holds a mouse button until MouseUp.
func (s *ScriptedInputService) MouseDown(b d2enum.MouseButton) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pressButton(s.button(b), false)
}

// MouseUp releases a scripted mouse button.
func (s *ScriptedInputService) MouseUp(b d2enum.MouseButton) {
	s.mu.Lock()
	defer s.mu.Unlock()

	releaseButton(s.button(b))
}

// Click moves the scripted cursor to x,y and taps a mouse button there.
func (s *ScriptedInputService) Click(x, y int, b d2enum.MouseButton) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cursor = &[2]int{x, y}

	pressButton(s.button(b), true)
}

// MoveCursor places the scripted cursor. It stays until the real mouse moves.
func (s *ScriptedInputService) MoveCursor(x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cursor = &[2]int{x, y}
}

// TypeText delivers printable runes on the next InputChars poll.
func (s *ScriptedInputService) TypeText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.chars = append(s.chars, []rune(text)...)
}

// Cursor reports the cursor the game currently sees (scripted or real).
func (s *ScriptedInputService) Cursor() (x, y int) {
	return s.CursorPosition()
}

// CursorScripted reports whether a scripted cursor position is in effect.
func (s *ScriptedInputService) CursorScripted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.cursor != nil
}

// ScriptedDown reports whether the scripted (not real) state holds a key.
func (s *ScriptedInputService) ScriptedDown(k d2enum.Key) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.keys[k]

	return ok && b.down
}

func (s *ScriptedInputService) key(k d2enum.Key) *scriptedButton {
	b, ok := s.keys[k]
	if !ok {
		b = &scriptedButton{}
		s.keys[k] = b
	}

	return b
}

func (s *ScriptedInputService) button(m d2enum.MouseButton) *scriptedButton {
	b, ok := s.buttons[m]
	if !ok {
		b = &scriptedButton{}
		s.buttons[m] = b
	}

	return b
}

// EndTick closes one poll cycle: press edges are consumed, taps release,
// release edges from the previous cycle are dropped, hold durations grow.
func (s *ScriptedInputService) EndTick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, b := range s.keys {
		if endButtonTick(b) {
			delete(s.keys, k)
		}
	}

	for m, b := range s.buttons {
		if endButtonTick(b) {
			delete(s.buttons, m)
		}
	}

	s.chars = nil
}

// endButtonTick advances one button past a poll; true means it is idle and
// can be forgotten.
func endButtonTick(b *scriptedButton) bool {
	b.justPressed = false

	if b.justReleased {
		b.justReleased = false
		return !b.down
	}

	if b.down {
		b.frames++

		if b.releaseNext {
			b.down = false
			b.releaseNext = false
			b.justReleased = true
		}

		return false
	}

	return true
}

// -------------------------------------------------- d2interface.InputService --

// CursorPosition returns the scripted cursor while one is set and the real
// mouse has not moved since; otherwise the real cursor.
func (s *ScriptedInputService) CursorPosition() (x, y int) {
	rx, ry := s.real.CursorPosition()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.realSeen && (rx != s.lastRealX || ry != s.lastRealY) {
		s.cursor = nil // the human moved the mouse: the real cursor wins
	}

	s.lastRealX, s.lastRealY, s.realSeen = rx, ry, true

	if s.cursor != nil {
		return s.cursor[0], s.cursor[1]
	}

	return rx, ry
}

// InputChars returns the real typed runes followed by the scripted ones.
func (s *ScriptedInputService) InputChars() []rune {
	real := s.real.InputChars()

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.chars) == 0 {
		return real
	}

	out := make([]rune, 0, len(real)+len(s.chars))
	out = append(out, real...)
	out = append(out, s.chars...)

	return out
}

// IsKeyPressed merges real and scripted held state.
func (s *ScriptedInputService) IsKeyPressed(k d2enum.Key) bool {
	if s.real.IsKeyPressed(k) {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.keys[k]

	return ok && b.down
}

// IsKeyJustPressed merges real and scripted press edges.
func (s *ScriptedInputService) IsKeyJustPressed(k d2enum.Key) bool {
	if s.real.IsKeyJustPressed(k) {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.keys[k]

	return ok && b.justPressed
}

// IsKeyJustReleased merges real and scripted release edges.
func (s *ScriptedInputService) IsKeyJustReleased(k d2enum.Key) bool {
	if s.real.IsKeyJustReleased(k) {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.keys[k]

	return ok && b.justReleased
}

// IsMouseButtonPressed merges real and scripted held state.
func (s *ScriptedInputService) IsMouseButtonPressed(m d2enum.MouseButton) bool {
	if s.real.IsMouseButtonPressed(m) {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buttons[m]

	return ok && b.down
}

// IsMouseButtonJustPressed merges real and scripted press edges.
func (s *ScriptedInputService) IsMouseButtonJustPressed(m d2enum.MouseButton) bool {
	if s.real.IsMouseButtonJustPressed(m) {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buttons[m]

	return ok && b.justPressed
}

// IsMouseButtonJustReleased merges real and scripted release edges.
func (s *ScriptedInputService) IsMouseButtonJustReleased(m d2enum.MouseButton) bool {
	if s.real.IsMouseButtonJustReleased(m) {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buttons[m]

	return ok && b.justReleased
}

// KeyPressDuration returns the real duration, or the scripted hold length in
// polls (1 on the poll that sees the press, like ebiten's inpututil).
func (s *ScriptedInputService) KeyPressDuration(k d2enum.Key) int {
	if d := s.real.KeyPressDuration(k); d > 0 {
		return d
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if b, ok := s.keys[k]; ok && b.down {
		return b.frames + 1
	}

	return 0
}
