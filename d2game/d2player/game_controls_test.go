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
