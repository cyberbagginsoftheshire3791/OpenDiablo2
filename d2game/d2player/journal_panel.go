package d2player

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2journal"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// J1, the journal (24 Sep 2026). Q -- and the mini-panel's quest button --
// open his diary in place of Diablo II's quest log whenever the game screen
// holds a journal. It is a centred page: the parts as tabs across the top,
// that part's titles down the left (newest first, unread in gold), and the
// chosen one's text on the right. While it is open it is MODAL -- every key
// and click is its own (A3) -- and the world is held (WorldHeldByJournal).
// It is refused while dead, talking, choosing a loadout, and in any fight.
// No art: fills and text, like the talk and kit panels.

// JournalHolder is what the controls ask about the journal.
type JournalHolder interface {
	// JournalParts are the tabs.
	JournalParts() []d2journal.Part
	// JournalRows is a part's rows, newest first.
	JournalRows(part string) []d2journal.View
	// JournalLeave marks a part read: he has turned away from it.
	JournalLeave(part string)
	// JournalAllowed is why the journal cannot open now, or nil.
	JournalAllowed() error
	// SetJournalOpen holds the world while he reads.
	SetJournalOpen(open bool)
}

const (
	journalX      = 40
	journalY      = 40
	journalWidth  = 720
	journalHeight = 520
	journalPadX   = 14

	journalTabY    = journalY + 30
	journalTabH    = 18
	journalTabGap  = 2
	journalTabSpan = journalWidth - 2*journalPadX

	journalListX    = journalX + journalPadX
	journalListY    = journalY + 60
	journalListW    = 224
	journalRowH     = 18
	journalListRows = 23

	journalTextX = journalX + 252
	journalTextY = journalY + 60
	journalLineH = 16

	// journalMaxParts is the most tabs that fit at the widest part title
	// (d2journal.MaxPartTitle, ten characters plus the unread mark).
	journalMaxParts = 7

	journalColor = 0x0c0a08f4
	journalEdge  = 0x5a4628ff
	journalRule  = 0x3a2e1cff
	journalSel   = 0x2a2218ff

	journalNoticeY = 470
)

type journalOverlay struct {
	open   bool
	part   int
	sel    int
	top    int
	parts  []d2journal.Part
	unread []bool // per part, as of the last turn of the page
	rows   []d2journal.View
	text   []string

	// The notice under the play area: "written in my journal", for a while.
	notice     string
	noticeLeft float64

	title  *d2ui.Label
	footer *d2ui.Label
	note   *d2ui.Label
	tabs   []*d2ui.Label
	list   []*d2ui.Label
	lines  []*d2ui.Label
}

func newJournalOverlay(ui *d2ui.UIManager) *journalOverlay {
	j := &journalOverlay{}

	mk := func() *d2ui.Label {
		l := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
		l.Alignment = d2ui.HorizontalAlignLeft

		return l
	}

	j.title, j.footer = mk(), mk()

	j.note = ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
	j.note.Alignment = d2ui.HorizontalAlignCenter

	for i := 0; i < journalMaxParts; i++ {
		j.tabs = append(j.tabs, mk())
	}

	for i := 0; i < journalListRows; i++ {
		j.list = append(j.list, mk())
	}

	// The title, a blank, a task's state line, and the text.
	for i := 0; i < d2journal.MaxLines+3; i++ {
		j.lines = append(j.lines, mk())
	}

	return j
}

// SetJournalHolder attaches the game screen's journal.
func (g *GameControls) SetJournalHolder(h JournalHolder) { g.journalHolder = h }

// journalOpen reports the journal open.
func (g *GameControls) journalOpen() bool {
	return g.hud != nil && g.hud.journal != nil && g.hud.journal.open
}

// JournalNotice shows a line under the play area for a while -- its own
// line, not the zone text a level-up or a stake uses (B12).
func (g *GameControls) JournalNotice(text string, seconds float64) {
	if g.hud == nil || g.hud.journal == nil {
		return
	}

	g.hud.journal.notice, g.hud.journal.noticeLeft = text, seconds
}

// questAction is the quest-log key and the mini-panel's quest button: the
// journal whenever the game screen holds one (J1 -- the default game and
// -classic alike, B13), Diablo II's quest log only with no holder at all.
func (g *GameControls) questAction() {
	if g.journalHolder == nil {
		g.toggleQuestLog()
		return
	}

	if g.journalOpen() {
		g.closeJournal()
		return
	}

	g.openJournal()
}

func (g *GameControls) openJournal() {
	if g.hud == nil || g.hud.journal == nil || (g.escapeMenu != nil && g.escapeMenu.IsOpen()) {
		return
	}

	if err := g.journalHolder.JournalAllowed(); err != nil {
		g.JournalNotice(fmt.Sprintf(JournalRefused, err), 3)
		return
	}

	j := g.hud.journal

	j.parts = g.journalHolder.JournalParts()
	if len(j.parts) == 0 {
		return
	}

	// One surface at a time: Diablo II's panels, the kit and the talents
	// close under it (A3).
	g.clearScreen()
	g.updateLayout()

	if g.hud.kit != nil {
		g.hud.kit.open = false
	}

	if g.hud.talents != nil {
		g.hud.talents.open = false
	}

	if j.part >= len(j.parts) {
		j.part = 0
	}

	// The HUD's own buttons are d2ui widgets, which take a click whatever
	// the controls answer: the mini-panel is closed and disabled while the
	// journal is up, as under the escape menu, and the run button refuses
	// (HUD.onToggleRunButton) -- review B1.
	g.hud.miniPanel.closeDisabled()

	j.open, j.notice, j.noticeLeft = true, "", 0
	g.journalHolder.SetJournalOpen(true)
	g.showJournalPart(j.part)
}

func (g *GameControls) closeJournal() {
	j := g.hud.journal
	if !j.open {
		return
	}

	g.journalHolder.JournalLeave(j.parts[j.part].ID)
	j.open = false
	g.journalHolder.SetJournalOpen(false)

	g.hud.miniPanel.restoreDisabled()
}

// showJournalPart turns to a part, leaving the one he was on read.
func (g *GameControls) showJournalPart(i int) {
	j := g.hud.journal

	if i != j.part && j.part < len(j.parts) {
		g.journalHolder.JournalLeave(j.parts[j.part].ID)
	}

	j.part, j.sel, j.top = i, 0, 0
	j.rows = g.journalHolder.JournalRows(j.parts[i].ID)

	j.unread = j.unread[:0]
	for _, p := range j.parts {
		j.unread = append(j.unread, journalHasUnread(g.journalHolder.JournalRows(p.ID)))
	}

	g.selectJournalRow(0)
}

// selectJournalRow chooses a row and lays its text out.
func (g *GameControls) selectJournalRow(i int) {
	j := g.hud.journal

	if len(j.rows) == 0 {
		j.sel, j.top = 0, 0
		j.text = []string{JournalEmpty}

		return
	}

	i = max(0, min(i, len(j.rows)-1))
	j.sel = i

	if i < j.top {
		j.top = i
	}

	if i >= j.top+journalListRows {
		j.top = i - journalListRows + 1
	}

	r := j.rows[i]
	text := r.Text

	switch r.Mark {
	case d2journal.TaskOpen:
		text = JournalStateOpen + "\n" + text
	case d2journal.TaskDone:
		text = JournalStateDone + "\n" + text
	case d2journal.TaskFailed:
		text = JournalStateFailed + "\n" + text
	}

	j.text = append([]string{r.Title, ""}, d2journal.Wrap(text, d2journal.WrapWidth)...)
	if len(j.text) > len(j.lines) {
		j.text = j.text[:len(j.lines)]
	}
}

// journalKey handles every key while the journal is open (A3).
func (g *GameControls) journalKey(key d2interface.KeyEvent) bool {
	if !g.journalOpen() {
		return false
	}

	j := g.hud.journal

	switch {
	case key.Key() == d2enum.KeyEscape, g.keyMap.getGameEvent(key.Key()) == d2enum.ToggleQuestLog:
		g.closeJournal()
	case key.Key() == d2enum.KeyLeft:
		g.showJournalPart((j.part + len(j.parts) - 1) % len(j.parts))
	case key.Key() == d2enum.KeyRight:
		g.showJournalPart((j.part + 1) % len(j.parts))
	case key.Key() == d2enum.KeyUp:
		g.selectJournalRow(j.sel - 1)
	case key.Key() == d2enum.KeyDown:
		g.selectJournalRow(j.sel + 1)
	}

	return true
}

// journalClick handles a click while the journal is open: a tab, a row, or
// nothing -- but always consumed (A3).
func (g *GameControls) journalClick(mx, my int) {
	j := g.hud.journal

	for i := range j.parts {
		if i < journalMaxParts && inRect(mx, my, journalTabX(i, len(j.parts)), journalTabY, journalTabW(len(j.parts)), journalTabH) {
			g.showJournalPart(i)
			return
		}
	}

	for k := 0; k < journalListRows && j.top+k < len(j.rows); k++ {
		if inRect(mx, my, journalListX, journalListY+k*journalRowH, journalListW, journalRowH) {
			g.selectJournalRow(j.top + k)
			return
		}
	}
}

// journalTabW is each tab's width when there are n: the tabs share the
// panel's width, so a seventh part (J2's writings) still fits on it.
func journalTabW(n int) int {
	n = max(1, min(n, journalMaxParts))
	return journalTabSpan/n - journalTabGap
}

func journalTabX(i, n int) int { return journalX + journalPadX + i*(journalTabW(n)+journalTabGap) }

// advanceJournal counts the notice down.
func (h *HUD) advanceJournal(elapsed float64) {
	if j := h.journal; j != nil && j.noticeLeft > 0 {
		if j.noticeLeft -= elapsed; j.noticeLeft <= 0 {
			j.notice, j.noticeLeft = "", 0
		}
	}
}

// journalRowTitle is a row as the list draws it: a task's mark, the title,
// and a bullet when unread.
func journalRowTitle(r d2journal.View) string {
	mark := ""

	switch r.Mark {
	case d2journal.TaskOpen:
		mark = JournalMarkOpen
	case d2journal.TaskDone:
		mark = JournalMarkDone
	case d2journal.TaskFailed:
		mark = JournalMarkFailed
	}

	return mark + r.Title
}

func (h *HUD) renderJournal(target d2interface.Surface) {
	j := h.journal
	if j == nil {
		return
	}

	if !j.open {
		if j.notice != "" {
			j.note.SetText(d2ui.ColorTokenize(j.notice, d2ui.ColorTokenGold))
			j.note.SetPosition(screenWidth/2, journalNoticeY)
			j.note.Render(target)
		}

		return
	}

	fillRect(target, journalX-1, journalY-1, journalWidth+2, journalHeight+2, journalEdge)
	fillRect(target, journalX, journalY, journalWidth, journalHeight, journalColor)
	fillRect(target, journalX+journalPadX, journalListY-6, journalWidth-2*journalPadX, 1, journalRule)
	fillRect(target, journalTextX-12, journalListY, 1, journalListRows*journalRowH, journalRule)

	j.title.SetText(d2ui.ColorTokenize(JournalTitle, d2ui.ColorTokenGold))
	j.title.SetPosition(journalX+journalPadX, journalY+8)
	j.title.Render(target)

	for i, p := range j.parts {
		if i >= journalMaxParts {
			break
		}

		label := p.Title
		if i != j.part && i < len(j.unread) && j.unread[i] {
			label += JournalUnread
		}

		color := d2ui.ColorTokenGrey
		if i == j.part {
			color = d2ui.ColorTokenGold
			fillRect(target, journalTabX(i, len(j.parts)), journalTabY-1, journalTabW(len(j.parts)), journalTabH, journalSel)
		}

		j.tabs[i].SetText(d2ui.ColorTokenize(label, color))
		j.tabs[i].SetPosition(journalTabX(i, len(j.parts))+4, journalTabY)
		j.tabs[i].Render(target)
	}

	for k := 0; k < journalListRows && j.top+k < len(j.rows); k++ {
		r := j.rows[j.top+k]
		y := journalListY + k*journalRowH

		if j.top+k == j.sel {
			fillRect(target, journalListX-2, y-1, journalListW, journalRowH, journalSel)
		}

		color := d2ui.ColorTokenWhite
		if r.Unread {
			color = d2ui.ColorTokenGold
		}

		// Titles are bounded at load (d2journal.MaxTitle) to fit the column
		// with a task's four-character mark.
		j.list[k].SetText(d2ui.ColorTokenize(journalRowTitle(r), color))
		j.list[k].SetPosition(journalListX, y)
		j.list[k].Render(target)
	}

	for i, line := range j.text {
		if i == 0 && len(j.rows) > 0 {
			line = d2ui.ColorTokenize(line, d2ui.ColorTokenGold)
		}

		j.lines[i].SetText(line)
		j.lines[i].SetPosition(journalTextX, journalTextY+i*journalLineH)
		j.lines[i].Render(target)
	}

	j.footer.SetText(d2ui.ColorTokenize(JournalKeys, d2ui.ColorTokenGrey))
	j.footer.SetPosition(journalX+journalPadX, journalY+journalHeight-22)
	j.footer.Render(target)
}

func journalHasUnread(rows []d2journal.View) bool {
	for _, r := range rows {
		if r.Unread {
			return true
		}
	}

	return false
}
