package d2player

import (
	"strings"
	"testing"
)

// The death screen words when and how, and what a reload gives back.
func TestDeathLines(t *testing.T) {
	night := deathLines(Death{Weekday: "Thursday", Year: 1462, Month: 6, Day: 17, Time: "21:14", Night: true, Cause: DeathCauseFight})

	if len(night) != 3 || night[0] != "Died on the night of Thursday 17 June 1462, at 21:14." || night[1] != DeathByFight {
		t.Fatalf("a night death in a fight: %q", night)
	}

	// THE CONTROL: by day, and of no cause the screen can name, it says only
	// when -- and still what is lost.
	day := deathLines(Death{Weekday: "Friday", Year: 1462, Month: 6, Day: 18, Time: "14:02"})

	if len(day) != 2 || strings.Contains(day[0], "night") || day[1] != DeathWhatIsLost {
		t.Fatalf("a day death, no cause: %q", day)
	}

	if got := deathLines(Death{Cause: DeathCauseThirst})[1]; got != DeathByThirst {
		t.Fatalf("thirst: %q", got)
	}
}
