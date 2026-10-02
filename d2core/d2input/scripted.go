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
// human at the keyboard is never locked out -- except the MOUSE of a game the
// harness owns (OwnMouse, BUG-111), which is the script's alone. The
// playtest harness drives it
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

	cursor               *[2]int // scripted cursor; cleared when the real cursor moves (unless owned)
	lastRealX, lastRealY int
	realSeen             bool

	// mouseOwned: the real mouse is not read at all -- its position, its
	// buttons and its wheel (BUG-111). See OwnMouse.
	mouseOwned bool
}

// ParkedCursorX and ParkedCursorY are where an owned mouse's cursor sits
// before a script has placed it: off the window, where nothing is hovered --
// the place an unfocused game's real cursor usually was, now the same in
// every run.
const (
	ParkedCursorX = -1
	ParkedCursorY = -1
)

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

// MoveCursor places the scripted cursor. It stays until the real mouse moves,
// or, with the mouse owned, until a script moves it.
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

// OwnMouse gives the script the whole mouse for the rest of the process
// (BUG-111): the real cursor's position, its buttons and its wheel are never
// read again, so a scripted cursor stays where the script put it however the
// human moves the real one, and before a script has placed it the cursor is
// parked off the window (ParkedCursorX, ParkedCursorY).
//
// The harness calls it for every game started with -harness, from the first
// frame, rather than at a script's first cursor verb: such a game is a
// scripted game launched without focus (30 Sep), and every reading in it
// must be the same in every run -- a hover, a click or a mouse-move event
// the real mouse makes before the script touches the cursor is as much a
// flake as one after. Josh works at the same laptop while the suite runs; his
// mouse was moving the scripted cursor (TestWatch, TestASlainManRises,
// TestTheDeadWalk hovered ""; BUG-15's scripted pointer reads). The keyboard
// still merges: an unfocused window gets no keys, and a focused one is a
// human who chose to type into it. A harness build run WITHOUT -harness never
// owns the mouse and plays as before.
func (s *ScriptedInputService) OwnMouse() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mouseOwned = true
}

// MouseOwned reports whether OwnMouse has been called.
func (s *ScriptedInputService) MouseOwned() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.mouseOwned
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
// mouse has not moved since; otherwise the real cursor. With the mouse owned
// the real cursor is never read: the scripted cursor, or the parked one.
func (s *ScriptedInputService) CursorPosition() (x, y int) {
	if s.MouseOwned() {
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.cursor != nil {
			return s.cursor[0], s.cursor[1]
		}

		return ParkedCursorX, ParkedCursorY
	}

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

// IsMouseButtonPressed merges real and scripted held state (scripted only
// with the mouse owned; so do the two edges below).
func (s *ScriptedInputService) IsMouseButtonPressed(m d2enum.MouseButton) bool {
	if !s.MouseOwned() && s.real.IsMouseButtonPressed(m) {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buttons[m]

	return ok && b.down
}

// IsMouseButtonJustPressed merges real and scripted press edges.
func (s *ScriptedInputService) IsMouseButtonJustPressed(m d2enum.MouseButton) bool {
	if !s.MouseOwned() && s.real.IsMouseButtonJustPressed(m) {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buttons[m]

	return ok && b.justPressed
}

// IsMouseButtonJustReleased merges real and scripted release edges.
func (s *ScriptedInputService) IsMouseButtonJustReleased(m d2enum.MouseButton) bool {
	if !s.MouseOwned() && s.real.IsMouseButtonJustReleased(m) {
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

// Wheel passes the real wheel through untouched.
//
// There is no scripted wheel: the playtest harness has no verb that rolls one,
// so overlaying state here would be a seam nothing drives. A human at the
// wheel is never locked out, which is the same rule the rest of this overlay
// follows -- but for an owned mouse, whose wheel is still: nothing scripted
// rolls it, and the real one is not read (BUG-111; the wheel zooms the map).
func (s *ScriptedInputService) Wheel() (xoff, yoff float64) {
	if s.MouseOwned() {
		return 0, 0
	}

	return s.real.Wheel()
}
