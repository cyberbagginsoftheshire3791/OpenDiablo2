package d2player

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2progress"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// T3, the talent panel (23 Sep 2026), on the skill-tree key in place of D2's
// skill tree. Three columns -- Endurance, Drill, Lore -- of five, taken top
// down. A talent is for good (U1 §6.1: no respec), so a click on an open
// talent SELECTS it and shows what it does, and a second click on the same
// talent takes it. A stray click never spends a level.

// ProgressHolder is what the controls ask about his standing.
type ProgressHolder interface {
	Progress() (*d2progress.Progress, *d2progress.Tree)
	PickTalent(id string) error
}

const (
	talentPanelX      = 40
	talentPanelY      = 36
	talentPanelWidth  = 720
	talentPanelHeight = 300
	talentColWidth    = 236
	talentRowHeight   = 34
	talentTop         = 70
)

type talentCell struct {
	id, name   string
	x, y       int
	taken      bool
	open       bool
	branchName string
	text       string
}

type talentOverlay struct {
	open     bool
	selected string
	notice   string
	cells    []talentCell
	header   *d2ui.Label
	footer   *d2ui.Label
	labels   []*d2ui.Label
	branches [3]*d2ui.Label
	texts    []string
}

func newTalentOverlay(ui *d2ui.UIManager) *talentOverlay {
	t := &talentOverlay{}

	mk := func() *d2ui.Label {
		l := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
		l.Alignment = d2ui.HorizontalAlignLeft

		return l
	}

	t.header, t.footer = mk(), mk()
	for i := range t.branches {
		t.branches[i] = mk()
	}

	for i := 0; i < 15; i++ {
		t.labels = append(t.labels, mk())
		t.texts = append(t.texts, "")
	}

	return t
}

// SetProgressHolder attaches the game screen's owner of his standing.
func (g *GameControls) SetProgressHolder(h ProgressHolder) { g.progressHolder = h }

// toggleTalentPanel is the skill-tree key.
func (g *GameControls) toggleTalentPanel() {
	if g.hud == nil || g.hud.talents == nil || g.progressHolder == nil {
		return
	}

	if p, _ := g.progressHolder.Progress(); p == nil {
		return
	}

	t := g.hud.talents
	t.open = !t.open
	t.selected, t.notice = "", ""

	if t.open {
		g.clearScreen()

		// The kit panel shares the right of the screen; one at a time.
		if g.hud.kit != nil {
			g.hud.kit.open = false
		}
	}
}

// overTalentPanel reports a point on the open panel.
func (g *GameControls) overTalentPanel(mx, my int) bool {
	return g.hud != nil && g.hud.talents != nil && g.hud.talents.open &&
		inRect(mx, my, talentPanelX, talentPanelY, talentPanelWidth, talentPanelHeight)
}

// talentClick selects, then takes.
func (g *GameControls) talentClick(mx, my int) bool {
	if !g.overTalentPanel(mx, my) {
		return false
	}

	t := g.hud.talents

	for _, c := range t.cells {
		if !inRect(mx, my, c.x, c.y, talentColWidth-12, talentRowHeight-4) {
			continue
		}

		if t.selected != c.id {
			t.selected, t.notice = c.id, ""
			return true
		}

		if err := g.progressHolder.PickTalent(c.id); err != nil {
			t.notice = err.Error()
			return true
		}

		t.notice = fmt.Sprintf(ProgressTaken, c.name)

		return true
	}

	return true
}

// refreshTalents rebuilds the cells.
func (h *HUD) refreshTalents() {
	t := h.talents
	if t == nil || !t.open || h.gameControls == nil || h.gameControls.progressHolder == nil {
		return
	}

	p, tree := h.gameControls.progressHolder.Progress()
	if p == nil || tree == nil {
		return
	}

	next := tree.NextAt(p.XP)
	nextText := ProgressTop
	if next >= 0 {
		nextText = fmt.Sprintf("%d", next)
	}

	t.header.SetText(d2ui.ColorTokenize(fmt.Sprintf(ProgressHeader,
		p.Level(tree), p.XP, nextText, p.PicksWaiting(tree)), d2ui.ColorTokenGold))

	t.cells = t.cells[:0]
	i := 0

	for b, column := range p.Board(tree) {
		t.branches[b].SetText(d2ui.ColorTokenize(tree.Branches[b].Name, d2ui.ColorTokenGold))

		for r, cell := range column {
			x := talentPanelX + 14 + b*talentColWidth
			y := talentPanelY + talentTop + r*talentRowHeight

			t.cells = append(t.cells, talentCell{
				id: cell.Node.ID, name: cell.Node.Name, x: x, y: y,
				taken: cell.Taken, open: cell.Open, text: cell.Node.Text,
				branchName: tree.Branches[b].Name,
			})

			token := d2ui.ColorTokenGrey

			switch {
			case cell.Taken:
				token = d2ui.ColorTokenGreen
			case cell.Open:
				token = d2ui.ColorTokenWhite
			}

			name := fmt.Sprintf("%d  %s", r+1, cell.Node.Name)
			if t.selected == cell.Node.ID {
				name = "> " + name
			}

			text := d2ui.ColorTokenize(name, token)
			if i < len(t.labels) && t.texts[i] != text {
				t.texts[i] = text
				t.labels[i].SetText(text)
			}

			i++
		}
	}

	footer := ProgressHint

	for _, c := range t.cells {
		if c.id != t.selected {
			continue
		}

		footer = c.text

		switch {
		case c.taken:
			footer += "  " + ProgressAlreadyTaken
		case c.open:
			footer += "  " + ProgressClickAgain
		}
	}

	if t.notice != "" {
		footer = t.notice
	}

	t.footer.SetText(d2ui.ColorTokenize(footer, d2ui.ColorTokenYellow))
}

// renderTalents draws the panel.
func (h *HUD) renderTalents(target d2interface.Surface) {
	t := h.talents
	if t == nil || !t.open {
		return
	}

	fillRect(target, talentPanelX-1, talentPanelY-1, talentPanelWidth+2, talentPanelHeight+2, kitPanelEdge)
	fillRect(target, talentPanelX, talentPanelY, talentPanelWidth, talentPanelHeight, kitPanelColor)

	t.header.SetPosition(talentPanelX+14, talentPanelY+10)
	t.header.Render(target)

	for b, l := range t.branches {
		l.SetPosition(talentPanelX+14+b*talentColWidth, talentPanelY+42)
		l.Render(target)
	}

	for i, c := range t.cells {
		if c.id == t.selected {
			fillRect(target, c.x-4, c.y-3, talentColWidth-12, talentRowHeight-6, kitRowHover)
		}

		if i < len(t.labels) {
			t.labels[i].SetPosition(c.x, c.y)
			t.labels[i].Render(target)
		}
	}

	t.footer.SetPosition(talentPanelX+14, talentPanelY+talentPanelHeight-24)
	t.footer.Render(target)
}
