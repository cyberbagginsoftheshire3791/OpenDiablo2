package d2ui

import "testing"

// TestButtonLabelColor is BUG-27's rule: with a Strigoi font set a button's
// label is drawn in the font's own ink, not under Diablo II's grey tint (which
// turns that ink into the stone's own grey); every other case keeps its
// layout's colour -- including the zero colour the arrow and add-skill
// buttons are made with, whose "text" is a palette path.
func TestButtonLabelColor(t *testing.T) {
	for _, c := range []struct {
		layout  uint32
		fontSet bool
		want    uint32
	}{
		{greyAlpha100, true, whiteAlpha100},
		{greyAlpha100, false, greyAlpha100},
		{whiteAlpha100, true, whiteAlpha100},
		{whiteAlpha100, false, whiteAlpha100},
		{0, true, 0},
		{0, false, 0},
	} {
		if got := buttonLabelColor(c.layout, c.fontSet); got != c.want {
			t.Errorf("buttonLabelColor(%#08x, font set %v) = %#08x, want %#08x", c.layout, c.fontSet, got, c.want)
		}
	}
}
