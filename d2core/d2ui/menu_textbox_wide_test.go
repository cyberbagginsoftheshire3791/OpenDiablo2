package d2ui

import (
	"image"
	"strings"
	"testing"
)

// The F8 note's box (5 Oct 2026): the native menu's text box, wide, taking a
// sentence. Its whole line is kept; its face shows the end he is typing at,
// at full size, inside the face.
func TestWideMenuTextboxKeepsTheNoteAndShowsItsTail(t *testing.T) {
	ui := menuRenderUI(t, false)

	tb := ui.NewMenuTextboxWide(560, 200)
	tb.SetPosition(120, 300)
	tb.Activate()

	if tb.GetVisible() {
		t.Fatal("a detached box must stay invisible to the UI manager, which would draw it into the frozen frame")
	}

	if err := ui.inputManager.UnbindHandler(tb); err == nil {
		t.Fatal("a detached box was bound to the input manager: it would take keys behind the owner's back")
	}

	note := "The torch went out; was it the rain? 100% sure it was lit (day 2, 21:15)."
	tb.TypeChars(note)

	if tb.GetText() != note {
		t.Fatalf("stored %q, want the whole note %q (any printable character)", tb.GetText(), note)
	}

	long := strings.Repeat("abcdefghij ", 30)
	tb.SetText("")
	tb.TypeChars(long)

	if got := len([]rune(tb.GetText())); got != 200 {
		t.Fatalf("stored %d characters, want the 200 the box holds", got)
	}

	shown := tb.textLabel.GetText()
	if shown == "" || len(shown) >= 200 || !strings.HasSuffix(tb.GetText(), shown) {
		t.Fatalf("shown %q: want a non-empty strict tail of the stored line", shown)
	}

	tw, _ := tb.textLabel.GetTextMetrics(shown)
	cw, _ := tb.lineBar.GetSize()

	if tw+cw > 560-12 {
		t.Fatalf("the shown tail is %d+%d px wide, past the face's %d", tw, cw, 560-12)
	}

	// And it is the LONGEST tail that fits: one more character would not.
	stored := []rune(tb.GetText())
	more := string(stored[len(stored)-len([]rune(shown))-1:])

	if mw, _ := tb.textLabel.GetTextMetrics(more); mw+cw <= 560-12 {
		t.Fatalf("the shown tail %q could show one more character (%d+%d px fits %d)", shown, mw, cw, 560-12)
	}

	frame := ui.renderer.NewSurface(800, 600).(*menuRenderSurface)
	tb.Draw(frame)

	face := image.Rect(120, 300, 680, 328)
	for _, d := range frame.draws {
		if !d.rect.In(face) {
			t.Fatalf("draw %v outside the face %v", d.rect, face)
		}
	}

	if tb.menuTextFit != 1 {
		t.Fatalf("the tail is drawn at %.2f, want full size (1)", tb.menuTextFit)
	}

	tb.Backspace()

	if got := len([]rune(tb.GetText())); got != 199 {
		t.Fatalf("after a backspace %d characters, want 199", got)
	}

	// The 15-character box the menu already had is unchanged.
	old := ui.NewMenuTextbox()
	old.SetFilter("abc")
	old.SetText("abcabcabcabcabcabc d")

	if old.GetText() != "abcabcabcabcabc" {
		t.Fatalf("the original box stored %q, want 15 filtered characters", old.GetText())
	}
}
