package d2interface

import "github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"

// InputService represents an interface offering Keyboard and Mouse interactions.
type InputService interface {
	// CursorPosition returns a position of a mouse cursor relative to the game screen (window).
	CursorPosition() (x int, y int)
	// InputChars return "printable" runes read from the keyboard at the time update is called.
	InputChars() []rune
	// IsKeyPressed checks if the provided key is down.
	IsKeyPressed(key d2enum.Key) bool
	// IsKeyJustPressed checks if the provided key is just transitioned from up to down.
	IsKeyJustPressed(key d2enum.Key) bool
	// IsKeyJustReleased checks if the provided key is just transitioned from down to up.
	IsKeyJustReleased(key d2enum.Key) bool
	// IsMouseButtonPressed checks if the provided mouse button is down.
	IsMouseButtonPressed(button d2enum.MouseButton) bool
	// IsMouseButtonJustPressed checks if the provided mouse button is just transitioned from up to down.
	IsMouseButtonJustPressed(button d2enum.MouseButton) bool
	// IsMouseButtonJustReleased checks if the provided mouse button is just transitioned from down to up.
	IsMouseButtonJustReleased(button d2enum.MouseButton) bool
	// KeyPressDuration returns how long the key is pressed in frames.
	KeyPressDuration(key d2enum.Key) int
	// Wheel returns the x and y scroll amounts of the mouse wheel or touchpad
	// for this poll, and 0, 0 when nothing is being scrolled. A wheel is not a
	// key -- it has a magnitude and no press, release or press duration -- so
	// it is read here rather than through d2enum.KeyMouseWheelUp/Down, which
	// the ebiten adapter's key map does not carry and which therefore alias
	// the A key. See d2core/d2input/mousewheel_event.go.
	Wheel() (xoff, yoff float64)
}
