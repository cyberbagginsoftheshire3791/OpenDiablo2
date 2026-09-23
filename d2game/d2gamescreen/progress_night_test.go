package d2gamescreen

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// A night lived through pays once, at the turn to dawn, and only to the living.
// The first case is the one the T3 review caught: a session opening AT dawn
// with lastStage still its zero value (night) must not pay -- bindProgress sets
// lastStage and dawnPaidDay from the clock, which is what the second case is.
func TestNightSurvived(t *testing.T) {
	night, dawn := d2world.StageNight, d2world.StageDawn

	cases := []struct {
		name          string
		last, now     d2world.Stage
		day, paid     int
		alive, expect bool
	}{
		{"the turn to dawn pays", night, dawn, 3, 2, true, true},
		{"a dawn already paid does not", night, dawn, 3, 3, true, false},
		{"the dead are not paid", night, dawn, 3, 2, false, false},
		{"dawn to dawn is no turn", dawn, dawn, 3, 2, true, false},
		{"night to night is no turn", night, night, 3, 2, true, false},
	}

	for _, c := range cases {
		if got := nightSurvived(c.last, c.now, c.day, c.paid, c.alive); got != c.expect {
			t.Errorf("%s: got %v", c.name, got)
		}
	}

	// The zero value of a Stage is night: the reason bindProgress must seed it.
	var zero d2world.Stage
	if zero != night {
		t.Fatal("the zero Stage is no longer night; revisit bindProgress's seeding comment")
	}
}
