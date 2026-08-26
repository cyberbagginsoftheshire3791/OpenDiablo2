package d2input

import (
	"fmt"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
)

// keyNames maps the canonical lower-case name of every keyboard key to its
// enum value. Letters and digits are their own names; the rest follow the
// d2enum constant names without the Key prefix, lower-cased. KeyByName also
// accepts a few aliases (esc, return, ctrl, and the punctuation characters).
//
//nolint:gochecknoglobals // a constant table
var keyNames = map[string]d2enum.Key{
	"0":            d2enum.Key0,
	"1":            d2enum.Key1,
	"2":            d2enum.Key2,
	"3":            d2enum.Key3,
	"4":            d2enum.Key4,
	"5":            d2enum.Key5,
	"6":            d2enum.Key6,
	"7":            d2enum.Key7,
	"8":            d2enum.Key8,
	"9":            d2enum.Key9,
	"a":            d2enum.KeyA,
	"b":            d2enum.KeyB,
	"c":            d2enum.KeyC,
	"d":            d2enum.KeyD,
	"e":            d2enum.KeyE,
	"f":            d2enum.KeyF,
	"g":            d2enum.KeyG,
	"h":            d2enum.KeyH,
	"i":            d2enum.KeyI,
	"j":            d2enum.KeyJ,
	"k":            d2enum.KeyK,
	"l":            d2enum.KeyL,
	"m":            d2enum.KeyM,
	"n":            d2enum.KeyN,
	"o":            d2enum.KeyO,
	"p":            d2enum.KeyP,
	"q":            d2enum.KeyQ,
	"r":            d2enum.KeyR,
	"s":            d2enum.KeyS,
	"t":            d2enum.KeyT,
	"u":            d2enum.KeyU,
	"v":            d2enum.KeyV,
	"w":            d2enum.KeyW,
	"x":            d2enum.KeyX,
	"y":            d2enum.KeyY,
	"z":            d2enum.KeyZ,
	"apostrophe":   d2enum.KeyApostrophe,
	"backslash":    d2enum.KeyBackslash,
	"backspace":    d2enum.KeyBackspace,
	"capslock":     d2enum.KeyCapsLock,
	"comma":        d2enum.KeyComma,
	"delete":       d2enum.KeyDelete,
	"down":         d2enum.KeyDown,
	"end":          d2enum.KeyEnd,
	"enter":        d2enum.KeyEnter,
	"equal":        d2enum.KeyEqual,
	"escape":       d2enum.KeyEscape,
	"f1":           d2enum.KeyF1,
	"f2":           d2enum.KeyF2,
	"f3":           d2enum.KeyF3,
	"f4":           d2enum.KeyF4,
	"f5":           d2enum.KeyF5,
	"f6":           d2enum.KeyF6,
	"f7":           d2enum.KeyF7,
	"f8":           d2enum.KeyF8,
	"f9":           d2enum.KeyF9,
	"f10":          d2enum.KeyF10,
	"f11":          d2enum.KeyF11,
	"f12":          d2enum.KeyF12,
	"graveaccent":  d2enum.KeyGraveAccent,
	"home":         d2enum.KeyHome,
	"insert":       d2enum.KeyInsert,
	"kp0":          d2enum.KeyKP0,
	"kp1":          d2enum.KeyKP1,
	"kp2":          d2enum.KeyKP2,
	"kp3":          d2enum.KeyKP3,
	"kp4":          d2enum.KeyKP4,
	"kp5":          d2enum.KeyKP5,
	"kp6":          d2enum.KeyKP6,
	"kp7":          d2enum.KeyKP7,
	"kp8":          d2enum.KeyKP8,
	"kp9":          d2enum.KeyKP9,
	"kpadd":        d2enum.KeyKPAdd,
	"kpdecimal":    d2enum.KeyKPDecimal,
	"kpdivide":     d2enum.KeyKPDivide,
	"kpenter":      d2enum.KeyKPEnter,
	"kpequal":      d2enum.KeyKPEqual,
	"kpmultiply":   d2enum.KeyKPMultiply,
	"kpsubtract":   d2enum.KeyKPSubtract,
	"left":         d2enum.KeyLeft,
	"leftbracket":  d2enum.KeyLeftBracket,
	"menu":         d2enum.KeyMenu,
	"minus":        d2enum.KeyMinus,
	"numlock":      d2enum.KeyNumLock,
	"pagedown":     d2enum.KeyPageDown,
	"pageup":       d2enum.KeyPageUp,
	"pause":        d2enum.KeyPause,
	"period":       d2enum.KeyPeriod,
	"printscreen":  d2enum.KeyPrintScreen,
	"right":        d2enum.KeyRight,
	"rightbracket": d2enum.KeyRightBracket,
	"scrolllock":   d2enum.KeyScrollLock,
	"semicolon":    d2enum.KeySemicolon,
	"slash":        d2enum.KeySlash,
	"space":        d2enum.KeySpace,
	"tab":          d2enum.KeyTab,
	"up":           d2enum.KeyUp,
	"alt":          d2enum.KeyAlt,
	"control":      d2enum.KeyControl,
	"shift":        d2enum.KeyShift,
	"tilde":        d2enum.KeyTilde,
}

//nolint:gochecknoglobals // a constant table
var keyAliases = map[string]string{
	"esc":    "escape",
	"return": "enter",
	"ctrl":   "control",
	"del":    "delete",
	"ins":    "insert",
	"grave":  "graveaccent",
	"`":      "graveaccent",
	"'":      "apostrophe",
	"\\":     "backslash",
	",":      "comma",
	".":      "period",
	"/":      "slash",
	";":      "semicolon",
	"-":      "minus",
	"=":      "equal",
	"[":      "leftbracket",
	"]":      "rightbracket",
	"~":      "tilde",
	"pgup":   "pageup",
	"pgdn":   "pagedown",
	" ":      "space",
}

// KeyByName resolves a key name ("i", "escape", "f5", "KeyEnter", "ctrl") to
// its enum value.
func KeyByName(name string) (d2enum.Key, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		n = strings.ToLower(name) // a lone space is the space key
	}

	if alias, ok := keyAliases[n]; ok {
		n = alias
	}

	if len(n) > 3 && strings.HasPrefix(n, "key") {
		if k, ok := keyNames[n[3:]]; ok {
			return k, nil
		}
	}

	if k, ok := keyNames[n]; ok {
		return k, nil
	}

	return 0, fmt.Errorf("unknown key %q", name)
}

// KeyName returns the canonical name of a key, or "key<N>" for one without.
func KeyName(k d2enum.Key) string {
	for name, v := range keyNames {
		if v == k {
			return name
		}
	}

	return fmt.Sprintf("key%d", int(k))
}

// MouseButtonByName resolves left, right, or middle.
func MouseButtonByName(name string) (d2enum.MouseButton, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "left":
		return d2enum.MouseButtonLeft, nil
	case "right":
		return d2enum.MouseButtonRight, nil
	case "middle":
		return d2enum.MouseButtonMiddle, nil
	default:
		return 0, fmt.Errorf("unknown mouse button %q (left, right, middle)", name)
	}
}
