package d2player

import (
	"fmt"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// T4, talk (23 Sep 2026). A left click on a villager within reach opens a
// conversation; the world is held while it lasts. What is said to him is at
// the top, his answers are numbered under it -- keys 1-9 or a click -- and
// Escape walks away. The villagers are ROLES drawn by D2 stand-in sprites
// (S1 §8.1 keeps every name a placeholder), so the hover label says the role,
// not the sprite's D2 name.

// TalkView is one frame of a conversation.
type TalkView struct {
	Role    string
	Text    string
	Answers []string
	Rep     int
	Rung    string // the rung he stands on, "" below the first
}

// TalkHolder is what the controls ask about talk.
type TalkHolder interface {
	// TalkTo opens a talk with the villager a stand-in sprite plays.
	TalkTo(label string, tileX, tileY float64) error
	// Talking reports an open talk and what it shows.
	Talking() (bool, TalkView)
	// Answer takes answer i (0-based).
	Answer(i int) error
	// EndTalk walks away.
	EndTalk()
	// RoleFor is the role a stand-in sprite plays, or "".
	RoleFor(label string) string
}

const (
	talkX        = 60
	talkY        = 250
	talkWidth    = 680
	talkPadX     = 14
	talkLineH    = 16
	talkWrap     = 90
	talkMaxText  = 6
	talkMaxAns   = d2dialogue.MaxAnswers
	talkAnswerH  = 18
	talkColor    = 0x0c0a08ee
	talkEdge     = 0x5a4628ff
	talkRowHover = 0x2a2218ff
)

type talkOverlay struct {
	open    bool
	view    TalkView
	notice  string
	text    []string
	answerY []int
	height  int

	role    *d2ui.Label
	footer  *d2ui.Label
	lines   []*d2ui.Label
	answers []*d2ui.Label
}

func newTalkOverlay(ui *d2ui.UIManager) *talkOverlay {
	t := &talkOverlay{}

	mk := func() *d2ui.Label {
		l := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
		l.Alignment = d2ui.HorizontalAlignLeft

		return l
	}

	t.role, t.footer = mk(), mk()

	for i := 0; i < talkMaxText; i++ {
		t.lines = append(t.lines, mk())
	}

	for i := 0; i < talkMaxAns; i++ {
		t.answers = append(t.answers, mk())
	}

	return t
}

// SetTalkHolder attaches the game screen's owner of the village.
func (g *GameControls) SetTalkHolder(h TalkHolder) { g.talkHolder = h }

// talking reports an open conversation.
func (g *GameControls) talking() bool {
	return g.hud != nil && g.hud.talk != nil && g.hud.talk.open
}

// talkKey handles a key during a conversation; it consumes every key.
func (g *GameControls) talkKey(key d2interface.KeyEvent) bool {
	if !g.talking() {
		return false
	}

	if key.Key() == d2enum.KeyEscape {
		g.talkHolder.EndTalk()
		g.hud.talk.open = false

		return true
	}

	if n := int(key.Key() - d2enum.Key1); n >= 0 && n < 9 {
		g.answer(n)
	}

	return true
}

func (g *GameControls) answer(i int) {
	g.hud.talk.notice = ""

	if err := g.talkHolder.Answer(i); err != nil {
		g.hud.talk.notice = err.Error()
	}

	g.hud.refreshTalk()
}

// talkClick routes a left click: on an answer during a talk, or on a
// villager to open one. It reports whether the click was consumed.
func (g *GameControls) talkClick(mx, my int) bool {
	if g.talkHolder == nil || g.hud == nil || g.hud.talk == nil {
		return false
	}

	t := g.hud.talk

	if t.open {
		for i, y := range t.answerY {
			if inRect(mx, my, talkX, y, talkWidth, talkAnswerH) {
				g.answer(i)
				break
			}
		}

		return true // the talk is modal
	}

	// A villager under another modal surface is not clicked through it: the
	// loadout choice, and the kit and talent panels, take their own clicks
	// (review finding -- this check runs before theirs).
	if (g.kitHolder != nil && g.kitHolder.ChoosingLoadout()) || g.overKitPanel(mx, my) || g.overTalentPanel(mx, my) {
		return false
	}

	e := g.hud.hoveredVillager(mx, my)
	if e == nil {
		return false
	}

	x, y := e.GetPositionF()
	t.notice = ""

	if err := g.talkHolder.TalkTo(nameKey(e), x, y); err != nil {
		// Out of reach is not an error to show over the world; the click
		// falls through and he walks toward the villager instead.
		return false
	}

	g.hud.refreshTalk()

	return true
}

// refreshTalk reads the holder and lays the panel out.
func (h *HUD) refreshTalk() {
	t := h.talk
	if t == nil || h.gameControls == nil || h.gameControls.talkHolder == nil {
		return
	}

	t.open, t.view = h.gameControls.talkHolder.Talking()
	if !t.open {
		t.text, t.answerY = t.text[:0], t.answerY[:0]
		return
	}

	t.text = wrapText(t.view.Text, talkWrap, talkMaxText)

	y := talkY + 10 + talkLineH*(len(t.text)+2)
	t.answerY = t.answerY[:0]

	answers := t.view.Answers
	if len(answers) > talkMaxAns {
		answers = answers[:talkMaxAns]
	}

	for range answers {
		t.answerY = append(t.answerY, y)
		y += talkAnswerH
	}

	t.height = y - talkY + talkLineH + 14
}

func (h *HUD) renderTalk(target d2interface.Surface) {
	t := h.talk
	if t == nil || !t.open {
		return
	}

	fillRect(target, talkX-1, talkY-1, talkWidth+2, t.height+2, talkEdge)
	fillRect(target, talkX, talkY, talkWidth, t.height, talkColor)

	standing := TalkStandingNone
	if t.view.Rung != "" {
		standing = t.view.Rung
	}

	t.role.SetText(d2ui.ColorTokenize(fmt.Sprintf(TalkHeader, t.view.Role, standing), d2ui.ColorTokenGold))
	t.role.SetPosition(talkX+talkPadX, talkY+8)
	t.role.Render(target)

	for i, line := range t.text {
		t.lines[i].SetText(line)
		t.lines[i].SetPosition(talkX+talkPadX, talkY+10+talkLineH*(i+1))
		t.lines[i].Render(target)
	}

	for i, y := range t.answerY {
		if inRect(h.lastMouseX, h.lastMouseY, talkX, y, talkWidth, talkAnswerH) {
			fillRect(target, talkX+2, y-1, talkWidth-4, talkAnswerH, talkRowHover)
		}

		t.answers[i].SetText(fmt.Sprintf("%d. %s", i+1, t.view.Answers[i]))
		t.answers[i].SetPosition(talkX+talkPadX*2, y)
		t.answers[i].Render(target)
	}

	foot := TalkKeys
	if t.notice != "" {
		foot = d2ui.ColorTokenize(t.notice, d2ui.ColorTokenRed)
	}

	t.footer.SetText(foot)
	t.footer.SetPosition(talkX+talkPadX, talkY+t.height-talkLineH-6)
	t.footer.Render(target)
}

// wrapText breaks text at spaces into at most max lines of width characters.
func wrapText(text string, width, max int) []string {
	var lines []string

	line := ""

	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}

	if line != "" {
		lines = append(lines, line)
	}

	if len(lines) > max {
		lines = lines[:max]
		lines[max-1] += " ..."
	}

	return lines
}

// hoveredVillager is the villager under a screen point, or nil.
func (h *HUD) hoveredVillager(mx, my int) d2interface.MapEntity {
	if h.gameControls == nil || h.gameControls.talkHolder == nil {
		return nil
	}

	holder := h.gameControls.talkHolder

	return h.hoveredEntityWhere(mx, my, func(e d2interface.MapEntity) bool {
		return holder.RoleFor(nameKey(e)) != ""
	})
}

// hoverTarget is what the hover label names and a left click acts on: a
// villager under the cursor first, then anything selectable -- so the label
// and the talk click never disagree (T6 review finding).
func (h *HUD) hoverTarget(mx, my int) d2interface.MapEntity {
	if v := h.hoveredVillager(mx, my); v != nil {
		return v
	}

	return h.hoveredEntity(mx, my)
}
