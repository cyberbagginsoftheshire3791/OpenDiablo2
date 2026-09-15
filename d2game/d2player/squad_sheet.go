package d2player

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// M4.4c-1 sheet: the "tiles" are a UI panel of CARDS (ruled ask 2), one per
// squad, shown after selecting a squad (ruled ask 6). Sized from §0 part 1's
// Font16 measurement: line height 16px, the widest field line 132px, so a card
// box ~152 wide. Build #1 shows one card (the player's squad); several stack.
// It opens on selection (a select-click or the cycle key) and closes on a key
// or on deselect (a left click on empty ground). No eat/drink buttons -- ask 5
// ruled the verb out.

const (
	sheetX          = 8
	sheetY          = 26 // just below the clock strip (y 2-20)
	sheetWidth      = 152
	sheetPadding    = 6
	sheetLineHeight = 16
	sheetCardGap    = 6

	sheetBgColor     = 0x0a0a0ad2 // near-black, ~82% alpha, so the map reads through faintly
	sheetBorderColor = 0xd2a03cff // the muted gold the selected squad's bar uses
)

// setSheetOpen shows or hides the selected-squad sheet.
func (h *HUD) setSheetOpen(open bool) { h.sheetOpen = open }

// squadSheetLines is a card's text, sized in §0 part 1. Numbers are rounded to
// whole meter points, the sheet's own convention.
func squadSheetLines(card d2world.SheetCard) []string {
	lines := []string{
		fmt.Sprintf("Squad %s", card.Squad),
		fmt.Sprintf("Health %d/%d", card.Health, card.MaxHealth),
		fmt.Sprintf("Food %d/100", int(card.Food+0.5)),
		fmt.Sprintf("Water %d/100", int(card.Water+0.5)),
		fmt.Sprintf("Fatigue %d/100", int(card.Fatigue+0.5)),
		fmt.Sprintf("Stance: %s", strigoiStanceLabel(card.Stance)),
		fmt.Sprintf("Members: %d", card.Members),
	}

	// ONE LINE PER CUE. §0 part 1 measured the joined cue string at 281px
	// against a card CONTENT width of 140px (152 less two 6px pads), and
	// nothing here clips -- two cues joined with ", " already overflow the
	// card. §0 ruled the cues render as short separate marks; joining them was
	// the c-1 review's finding, 15 Sep 2026.
	for _, c := range card.Cues {
		lines = append(lines, strigoiCueLabel(c))
	}

	return lines
}

// renderSquadSheet draws the sheet when it is open: one card per squad, stacked,
// each a faint near-black box (the map reads through) with a gold top edge on
// the selected card and a Font16 label per line. The label is invisible to the
// UIManager (like the clock strip), so this is the only thing that paints it.
func (h *HUD) renderSquadSheet(target d2interface.Surface) {
	if !h.sheetOpen || h.gameControls == nil || h.gameControls.squads == nil || h.sheetLabel == nil {
		return
	}

	cards := h.gameControls.squads.SheetCards()
	if len(cards) == 0 {
		return
	}

	y := sheetY

	for _, card := range cards {
		lines := squadSheetLines(card)
		boxH := len(lines)*sheetLineHeight + 2*sheetPadding

		fillRect(target, sheetX, y, sheetWidth, boxH, sheetBgColor)

		if card.Selected {
			fillRect(target, sheetX, y, sheetWidth, 2, sheetBorderColor)
		}

		for i, line := range lines {
			h.sheetLabel.SetText(line)
			h.sheetLabel.SetPosition(sheetX+sheetPadding, y+sheetPadding+i*sheetLineHeight)
			h.sheetLabel.Render(target)
		}

		y += boxH + sheetCardGap
	}
}
