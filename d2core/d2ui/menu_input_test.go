package d2ui

import (
	"image/color"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2font"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2input"
)

// menuInput drives the same input manager as the game, without a renderer,
// audio device, asset files or window. Explicit edges let it reproduce a
// release delivered after a menu has changed without a new local press.
type menuInput struct {
	backspace        bool
	leftDown, leftUp bool
	leftHeld         bool
	x, y             int
}

func (s *menuInput) CursorPosition() (int, int) { return s.x, s.y }
func (s *menuInput) InputChars() []rune         { return nil }
func (s *menuInput) IsKeyPressed(k d2enum.Key) bool {
	return s.backspace && k == d2enum.KeyBackspace
}
func (*menuInput) IsKeyJustPressed(d2enum.Key) bool  { return false }
func (*menuInput) IsKeyJustReleased(d2enum.Key) bool { return false }
func (*menuInput) KeyPressDuration(d2enum.Key) int   { return 1 }
func (s *menuInput) IsMouseButtonPressed(b d2enum.MouseButton) bool {
	return b == d2enum.MouseButtonLeft && s.leftHeld
}
func (s *menuInput) IsMouseButtonJustPressed(b d2enum.MouseButton) bool {
	return b == d2enum.MouseButtonLeft && s.leftDown
}
func (s *menuInput) IsMouseButtonJustReleased(b d2enum.MouseButton) bool {
	return b == d2enum.MouseButtonLeft && s.leftUp
}
func (*menuInput) Wheel() (float64, float64) { return 0, 0 }

var _ d2interface.InputService = (*menuInput)(nil)

// Negative control (4 Oct 2026): omitting OnKeyRepeat's active-input guard
// makes hidden, unfocused and disabled fail ("12" becomes "1"); active passes.
func TestMenuTextboxBackspaceRequiresActiveInput(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		visible, focused, enabled bool
		want                      string
	}{
		{"active", true, true, true, "1"},
		{"hidden", false, true, true, "12"},
		{"unfocused", true, false, true, "12"},
		{"disabled", true, true, false, "12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &menuInput{x: -1, y: -1}
			im := d2input.NewInputManagerWithService(service)
			ui := &UIManager{inputManager: im}
			label := func() *Label {
				return &Label{BaseWidget: NewBaseWidget(ui), font: d2font.New(nil), Color: map[int]color.Color{}}
			}
			tb := &TextBox{
				BaseWidget: NewBaseWidget(ui), text: "12", filter: "0123456789",
				textLabel: label(), lineBar: label(), enabled: tc.enabled, isFocused: tc.focused,
			}
			tb.SetVisible(tc.visible)
			ui.addWidget(tb)
			if err := im.Advance(0, 0); err != nil {
				t.Fatal(err)
			}
			if got := tb.GetText(); got != "12" {
				t.Fatalf("no-key control: text %q, want 12", got)
			}
			service.backspace = true
			if err := im.Advance(0, 0); err != nil {
				t.Fatal(err)
			}
			if got := tb.GetText(); got != tc.want {
				t.Fatalf("Backspace: text %q, want %q", got, tc.want)
			}
		})
	}
}

type menuClickSound struct{ plays int }

func (s *menuClickSound) Play()           { s.plays++ }
func (*menuClickSound) Stop()             {}
func (*menuClickSound) SetPan(float64)    {}
func (*menuClickSound) IsPlaying() bool   { return false }
func (*menuClickSound) SetVolume(float64) {}

// Negative control (4 Oct 2026): omitting OnMouseButtonUp's press-owner clear
// fails at the bare second release (two activations instead of one).
func TestMenuReleaseConsumesPressOwner(t *testing.T) {
	service := &menuInput{x: 10, y: 10}
	im := d2input.NewInputManagerWithService(service)
	sound := &menuClickSound{}
	ui := &UIManager{inputManager: im, clickSfx: sound}
	if err := im.BindHandler(ui); err != nil {
		t.Fatal(err)
	}
	b := &Button{BaseWidget: NewBaseWidget(ui), enabled: true}
	b.width, b.height = 40, 40
	activations := 0
	b.OnActivated(func() { activations++ })
	ui.addWidget(b)
	poll := func() {
		t.Helper()
		if err := im.Advance(0, 0); err != nil {
			t.Fatal(err)
		}
	}
	// A genuine down/up remains a click: this is the positive control.
	service.leftDown, service.leftHeld = true, true
	poll()
	if !b.GetPressed() || sound.plays != 1 || activations != 0 {
		t.Fatalf("down: pressed %v, sounds %d, activations %d", b.GetPressed(), sound.plays, activations)
	}
	service.leftDown, service.leftHeld, service.leftUp = false, false, true
	poll()
	if b.GetPressed() || activations != 1 {
		t.Fatalf("up: pressed %v, activations %d, want false and 1", b.GetPressed(), activations)
	}
	// Leaving and returning to a menu must not revive its previous press.
	b.SetVisible(false)
	b.SetVisible(true)
	poll() // another bare up, with no new down
	if activations != 1 {
		t.Fatalf("bare second release activated the old button: %d activations, want 1", activations)
	}
	service.leftUp, service.leftDown, service.leftHeld = false, true, true
	poll()
	service.leftDown, service.leftHeld, service.leftUp = false, false, true
	poll()
	if activations != 2 || sound.plays != 2 {
		t.Fatalf("fresh click: activations %d, sounds %d, want 2 and 2", activations, sound.plays)
	}
}
