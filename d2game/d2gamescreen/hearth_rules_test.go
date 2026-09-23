package d2gamescreen

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// M4.7 step 4: the priest closes a man's hasty grave within the radius, and
// nothing else -- a fresh body, a carcass's grave, a closed one, or a grave a
// step beyond the church's reach.
func TestRiteCloses(t *testing.T) {
	grave := d2world.Corpse{State: d2world.CorpseHasty, Class: d2world.CorpseHuman}

	cases := []struct {
		name string
		b    d2world.Corpse
		dist float64
		want bool
	}{
		{"a man's grave in reach", grave, 12, true},
		{"a man's grave a step too far", grave, 12.5, false},
		{"an open body", d2world.Corpse{State: d2world.CorpseFresh, Class: d2world.CorpseHuman}, 1, false},
		{"a carcass in a grave", d2world.Corpse{State: d2world.CorpseHasty, Class: d2world.CorpseBeast}, 1, false},
		{"already closed", d2world.Corpse{State: d2world.CorpseClosed, Class: d2world.CorpseHuman}, 1, false},
	}

	for _, c := range cases {
		if got := riteCloses(c.b, c.dist, 12); got != c.want {
			t.Errorf("%s: riteCloses = %v, want %v", c.name, got, c.want)
		}
	}
}

// A staking is seen by day within the radius; not at night, not out of it.
func TestSeenBy(t *testing.T) {
	cases := []struct {
		name  string
		night bool
		dist  float64
		want  bool
	}{
		{"by day, near", false, 8, true},
		{"by day, a step too far", false, 8.5, false},
		{"at night, near", true, 1, false},
	}

	for _, c := range cases {
		if got := seenBy(c.night, c.dist, 8); got != c.want {
			t.Errorf("%s: seenBy = %v, want %v", c.name, got, c.want)
		}
	}
}
