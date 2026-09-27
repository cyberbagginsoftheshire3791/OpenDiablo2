package d2player

import (
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
)

// T10: the help names every Strigoi verb by the key the controls actually
// read, and fills exactly the overlay's bullet slots.
func TestStrigoiHelpNamesTheKeys(t *testing.T) {
	lines := strigoiHelp()

	if len(lines) != bullets {
		t.Fatalf("the overlay has %d bullet slots; the help has %d lines", bullets, len(lines))
	}

	all := strings.Join(lines, "\n")

	for key, phrase := range map[d2enum.Key]string{
		forageKey: "K forages",
		stakeKey:  "X stakes",
		digKey:    "D digs",
		searchKey: "U searches",
	} {
		letter := string(rune('A' + int(key-d2enum.KeyA)))
		if !strings.HasPrefix(phrase, letter+" ") || !strings.Contains(all, phrase) {
			t.Errorf("the help must say %q for key %s", phrase, letter)
		}
	}

	// The right button is the torch, as L is (Josh, 25 Sep 2026); the help
	// said only L until the tables burst's review (27 Sep 2026).
	for _, verb := range []string{"strike", "torch", "right button lights or douses", "kit", "talents", "talk", "Esc", "Q opens your journal"} {
		if !strings.Contains(all, verb) {
			t.Errorf("the help never mentions %q", verb)
		}
	}
}
