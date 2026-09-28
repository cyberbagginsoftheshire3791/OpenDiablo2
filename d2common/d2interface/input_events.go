package d2interface

import "github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"

// HandlerEvent holds the qualifiers for a key or mouse event
type HandlerEvent interface {
	KeyMod() d2enum.KeyMod
	ButtonMod() d2enum.MouseButtonMod
	X() int
	Y() int
}

// KeyEvent represents an event associated with a keyboard key
type KeyEvent interface {
	HandlerEvent
	Key() d2enum.Key
	// Duration represents the number of frames this key has been pressed for
	Duration() int
}

// KeyCharsEvent represents an event associated with a keyboard character being pressed
type KeyCharsEvent interface {
	HandlerEvent
	Chars() []rune
}

// MouseEvent represents a mouse event
type MouseEvent interface {
	HandlerEvent
	Button() d2enum.MouseButton
}

// MouseMoveEvent represents a mouse movement event
type MouseMoveEvent interface {
	HandlerEvent
}

// MouseWheelEvent represents a roll of the mouse wheel (or a touchpad scroll).
// X and Y, inherited from HandlerEvent, are where the cursor was when it
// rolled; ScrollX and ScrollY are how far it rolled, in whatever sign the
// platform reported (nothing in the engine normalises it).
type MouseWheelEvent interface {
	HandlerEvent
	ScrollX() float64
	ScrollY() float64
}
