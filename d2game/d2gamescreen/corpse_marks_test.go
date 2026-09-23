package d2gamescreen

import (
	"math"
	"testing"
)

// M4.7 review: the HUD's marks must not light what the night hides. A body is
// marked when its tile is lit to markLight, or when he stands within markNear.
func TestMarkVisible(t *testing.T) {
	cases := []struct {
		name        string
		level, dist float64
		want        bool
	}{
		{"by day, far off", 1, 30, true},
		{"at the edge of the light", markLight, 30, true},
		{"in the dark, far off", markLight - 0.1, 30, false},
		{"in the dark, no player", 0, math.Inf(1), false},
		{"in the dark, at his feet", 0, markNear, true},
		{"in the dark, a step too far", 0, markNear + 0.5, false},
	}

	for _, c := range cases {
		if got := markVisible(c.level, c.dist); got != c.want {
			t.Errorf("%s: markVisible(%.2f, %.1f) = %v, want %v", c.name, c.level, c.dist, got, c.want)
		}
	}
}
