package d2player

import "testing"

// The click-repeat throttle used to read the wall clock (d2util.Now); it now
// reads the controls' own accumulated clock (P3 spec §2.2). These pin the
// semantics that matter: the first held-click action fires immediately, and
// later ones wait exactly the threshold.
func TestRepeatDue(t *testing.T) {
	cases := []struct {
		name      string
		now, last float64
		want      bool
	}{
		{"first action fires at clock zero", 0, -mouseBtnActionsThreshold, true},
		{"too soon after the last action", 0.1, 0, false},
		{"exactly the threshold", mouseBtnActionsThreshold, 0, true},
		{"well after", 3.0, 1.0, true},
		{"just under the threshold", 1.0 + mouseBtnActionsThreshold - 1e-9, 1.0, false},
	}

	for _, c := range cases {
		if got := repeatDue(c.now, c.last); got != c.want {
			t.Errorf("%s: repeatDue(%v, %v) = %v, want %v", c.name, c.now, c.last, got, c.want)
		}
	}
}

// TestHeldSelectStands pins BUG-120's latch (decision A of its review, 2 Oct
// 2026): a hold whose press selected a squad stays that select while the
// cursor is within heldSelectSlop of the press point on both axes, and a drag
// further off is a walk. No playtest can drag during a hold -- strigoi_click
// holds the button at one point -- so the drag half is asserted here.
func TestHeldSelectStands(t *testing.T) {
	const px, py = 440, 358

	cases := []struct {
		name    string
		latched bool
		x, y    int
		want    bool
	}{
		{"a still cursor on a select", true, px, py, true},
		{"the slop's edge, both axes", true, px + heldSelectSlop, py - heldSelectSlop, true},
		{"one pixel past the slop in x", true, px + heldSelectSlop + 1, py, false},
		{"one pixel past the slop in y", true, px, py - heldSelectSlop - 1, false},
		{"a drag well off the model", true, px - 40, py + 12, false},
		{"a ground press never latches", false, px, py, false},
	}

	for _, c := range cases {
		if got := heldSelectStands(c.latched, px, py, c.x, c.y); got != c.want {
			t.Errorf("%s: heldSelectStands(%v, %d,%d, %d,%d) = %v, want %v",
				c.name, c.latched, px, py, c.x, c.y, got, c.want)
		}
	}
}
