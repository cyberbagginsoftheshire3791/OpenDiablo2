package d2input

import "github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"

// MouseWheelEvent represents a roll of the mouse wheel, or a touchpad scroll.
//
// It is shaped like MouseEvent -- an embedded HandlerEvent carrying the
// modifiers and the cursor position, plus what makes this event its own kind.
// For the wheel that is a pair of amounts rather than a button, because a wheel
// roll has a magnitude and no press, release or press duration.
//
// It deliberately does NOT travel as a KeyEvent with d2enum.KeyMouseWheelUp or
// KeyMouseWheelDown. Those two constants exist (d2common/d2enum/input_key.go
// :112-113) and are absent from the ebiten adapter's keyToEbiten map
// (d2core/d2input/ebiten/ebiten_input.go:13-114), so
// keyToEbiten[KeyMouseWheelUp] is the zero ebiten.Key, which is ebiten.KeyA.
// They alias the A key, and there is no ebiten key they could honestly map to,
// because ebiten reports the wheel as two float64 offsets (ebiten.Wheel).
type MouseWheelEvent struct {
	HandlerEvent
	scrollX float64
	scrollY float64
}

// KeyMod returns the key mod
func (e *MouseWheelEvent) KeyMod() d2enum.KeyMod {
	return e.HandlerEvent.keyMod
}

// ButtonMod represents a button mod
func (e *MouseWheelEvent) ButtonMod() d2enum.MouseButtonMod {
	return e.HandlerEvent.buttonMod
}

// X returns the cursor's X position when the wheel rolled
func (e *MouseWheelEvent) X() int {
	return e.HandlerEvent.x
}

// Y returns the cursor's Y position when the wheel rolled
func (e *MouseWheelEvent) Y() int {
	return e.HandlerEvent.y
}

// ScrollX returns how far the wheel rolled horizontally.
//
// The sign is whatever the platform reported: ebiten adds GLFW's scroll offsets
// unchanged (ebiten v2.9.10 internal/ui/input_glfw.go:89-95), so a desktop's
// "natural scrolling" setting flips it. Nothing in the engine normalises it, so
// a handler that cares about direction must decide for itself.
func (e *MouseWheelEvent) ScrollX() float64 {
	return e.scrollX
}

// ScrollY returns how far the wheel rolled vertically. See ScrollX on the sign.
func (e *MouseWheelEvent) ScrollY() float64 {
	return e.scrollY
}
