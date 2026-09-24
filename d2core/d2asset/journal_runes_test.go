package d2asset

import (
	"encoding/json"
	"os"
	"testing"
)

// TestTheJournalIsInTheFont is B8 (J1 attack, 24 Sep 2026): every character
// the journal's table writes is one a Strigoi font draws. A glyph the font
// lacks is skipped silently at draw time -- "kılıç" would read "kl" -- so the
// only place to catch it is here. The walk covers every string in the file,
// keys included, so a new field cannot slip past it.
func TestTheJournalIsInTheFont(t *testing.T) {
	data, err := os.ReadFile("../../data/strigoi/journal.json")
	if err != nil {
		t.Fatal(err)
	}

	var doc interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}

	drawn := map[rune]bool{}
	for _, r := range fontRunes() {
		drawn[r] = true
	}

	checked := 0

	var walk func(v interface{})
	walk = func(v interface{}) {
		switch x := v.(type) {
		case string:
			checked++

			for _, r := range x {
				if r != '\n' && !drawn[r] {
					t.Errorf("the journal writes %q (U+%04X), which no Strigoi font draws: %q", r, r, x)
				}
			}
		case []interface{}:
			for _, e := range x {
				walk(e)
			}
		case map[string]interface{}:
			for k, e := range x {
				walk(k)
				walk(e)
			}
		}
	}

	walk(doc)

	if checked < 300 {
		t.Fatalf("checked %d strings; the walk did not reach the entries", checked)
	}
}
