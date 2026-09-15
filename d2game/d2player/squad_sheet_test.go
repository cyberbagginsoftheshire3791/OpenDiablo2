package d2player

import (
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// TestSquadSheetLinesOneLinePerCue is the regression test for the c-1 review's
// fourth finding (15 Sep 2026): the cue labels were joined with ", " onto one
// line, against §0 part 1's measurement. The joined night-one cue string
// measured 281px against a card CONTENT width of 140px (152 less two 6px
// pads), and nothing in renderSquadSheet clips, so the overflow would have run
// off the card unannounced.
//
// It lives here rather than in the playtest because the "ui" provider reports
// a card's CUES, not the lines the sheet draws from them: the joining happened
// below the provider, where no script could see it. d2game/d2player is
// ebiten-free, so this runs on CI, which the playtest scripts never do.
func TestSquadSheetLinesOneLinePerCue(t *testing.T) {
	const fields = 7 // squad, health, food, water, fatigue, stance, members

	if bare := squadSheetLines(d2world.SheetCard{Squad: "s:1"}); len(bare) != fields {
		t.Fatalf("a card with no cues wants %d lines, got %d: %q", fields, len(bare), bare)
	}

	card := d2world.SheetCard{Squad: "s:1", Cues: []string{cueHungry, cueThirsty, cueShaken}}

	lines := squadSheetLines(card)
	if len(lines) != fields+len(card.Cues) {
		t.Fatalf("each cue gets its OWN line: wanted %d lines for %d cue(s), got %d: %q",
			fields+len(card.Cues), len(card.Cues), len(lines), lines)
	}

	cueLines := lines[fields:]

	for _, line := range cueLines {
		if strings.Contains(line, ", ") {
			t.Fatalf("the cue lines are joined: %q -- §0 ruled them short SEPARATE marks", line)
		}
	}

	for _, cue := range card.Cues {
		label := strigoiCueLabel(cue)
		found := false

		for _, line := range cueLines {
			if line == label {
				found = true
			}
		}

		if !found {
			t.Fatalf("cue %q (%q) is not a line of its own: %q", cue, label, cueLines)
		}
	}
}
