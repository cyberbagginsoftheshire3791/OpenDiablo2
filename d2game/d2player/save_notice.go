package d2player

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// THE SAVE'S NOTICES (M4.6 B5). What the save and the load tell him, on the
// HUD, for a few seconds: "Game saved." after the menu's SAVE GAME; the dawn
// autosave's quiet line; and, at the start of play, a load that could not
// resume his save ("...You wake at dawn. The save is kept, set aside -- not
// deleted.", the build plan's rule 7) or resumed it with a villager gone.
//
// Its own line, above the journal's (journalNoticeY), so a dawn that writes in
// his journal and saves the game says both. Gold on a dark band, the journal
// panel's colours, in Font16 -- Strigoi's IM Fell under its font set. A
// notice of two lines ("\n") draws each line centred.

const (
	saveNoticeY    = 438
	saveNoticePadX = 14
	saveNoticePadY = 4
	saveNoticeBand = 0x0c0a08c8
	saveNoticeEdge = 0x5a4628ff

	// SaveNoticeSeconds is how long "Game saved." and the dawn's line stay;
	// LoadNoticeSeconds the load's, which say more.
	SaveNoticeSeconds = 3.0
	LoadNoticeSeconds = 8.0
)

type saveNoticeOverlay struct {
	text  string
	left  float64
	label *d2ui.Label
}

func newSaveNoticeOverlay(ui *d2ui.UIManager) *saveNoticeOverlay {
	l := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
	l.Alignment = d2ui.HorizontalAlignCenter

	return &saveNoticeOverlay{label: l}
}

// SaveNotice shows a line of the save's (or the load's) on the HUD for
// seconds of play. A new notice replaces the one showing.
func (g *GameControls) SaveNotice(text string, seconds float64) {
	if g.hud == nil || g.hud.saveNotice == nil {
		return
	}

	g.hud.saveNotice.text, g.hud.saveNotice.left = text, seconds
}

// saveNoticeText is the notice while it shows, "" when none (the ui provider).
func (g *GameControls) saveNoticeText() string {
	if g.hud == nil || g.hud.saveNotice == nil {
		return ""
	}

	return g.hud.saveNotice.text
}

// advanceSaveNotice counts the notice down in the screen's own seconds.
func (h *HUD) advanceSaveNotice(elapsed float64) {
	n := h.saveNotice
	if n == nil || n.left <= 0 {
		return
	}

	if n.left -= elapsed; n.left <= 0 {
		n.text, n.left = "", 0
	}
}

// renderSaveNotice draws the notice: a band as wide as its widest line, and
// each line centred on the screen (d2ui.Label centres each line of a text).
func (h *HUD) renderSaveNotice(target d2interface.Surface) {
	n := h.saveNotice
	if n == nil || n.text == "" || n.label == nil {
		return
	}

	n.label.SetText(d2ui.ColorTokenize(n.text, d2ui.ColorTokenGold))

	tw, th := n.label.GetSize()
	x := screenWidth/2 - tw/2 - saveNoticePadX

	fillRect(target, x-1, saveNoticeY-saveNoticePadY-1, tw+2*saveNoticePadX+2, th+2*saveNoticePadY+2, saveNoticeEdge)
	fillRect(target, x, saveNoticeY-saveNoticePadY, tw+2*saveNoticePadX, th+2*saveNoticePadY, saveNoticeBand)

	n.label.SetPosition(screenWidth/2, saveNoticeY)
	n.label.Render(target)
}
