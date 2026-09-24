package d2player

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// Death screen v0 (23 Sep 2026): "you died on the night of 17 June -- load
// last save / quit" (the 9 Sep order's step, M19). Before it, a hero at 0
// health kept walking the map.
//
// WHAT "LAST SAVE" MEANS is the 12 Sep ruling's, made concrete: his .od2 is
// written only when he leaves the world alive, and his kit-and-progress file
// is rolled back, at the moment of death, to what it held when he entered.
// So a death costs everything since he last left the road -- the experience,
// the talents, the wear -- and both files agree on the moment he returns to.
//
// The game screen owns the death and the files; this draws, and routes Enter
// and Escape.

// Death is what the screen says about how he died. The game screen fills it
// once, on the frame he dies; this package words it.
type Death struct {
	Weekday          string
	Year, Month, Day int
	Time             string // HH:MM
	Night            bool
	Cause            string // DeathCause*
}

// The causes the screen can name.
const (
	DeathCauseFight  = "fight"
	DeathCauseHunger = "hunger"
	DeathCauseThirst = "thirst"
)

// DeathHolder is what the controls ask about his death.
type DeathHolder interface {
	// Death reports whether he is dead, and how.
	Death() (dead bool, how Death)
	// LoadLastSave returns him to the moment he last entered the world.
	LoadLastSave()
	// QuitToMenu leaves to the main menu, saving nothing.
	QuitToMenu()
}

const (
	deathShade   = 0x000000b4
	deathX       = 130
	deathY       = 170
	deathWidth   = 540
	deathHeight  = 150
	deathEdge    = 0x6a1c14ff
	deathColor   = 0x0c0806f0
	deathLineGap = 20
	deathMaxLine = 3
)

type deathOverlay struct {
	open   bool
	lines  []string
	title  *d2ui.Label
	labels []*d2ui.Label
	footer *d2ui.Label
}

func newDeathOverlay(ui *d2ui.UIManager) *deathOverlay {
	d := &deathOverlay{}

	mk := func() *d2ui.Label {
		l := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
		l.Alignment = d2ui.HorizontalAlignCenter

		return l
	}

	d.title, d.footer = mk(), mk()
	for i := 0; i < deathMaxLine; i++ {
		d.labels = append(d.labels, mk())
	}

	return d
}

// SetDeathHolder attaches the game screen's owner of his death.
func (g *GameControls) SetDeathHolder(h DeathHolder) { g.deathHolder = h }

// dead reports whether the death screen is up. Every input path asks first:
// a dead man neither walks, nor fights, nor opens a panel.
func (g *GameControls) dead() bool {
	return g.hud != nil && g.hud.death != nil && g.hud.death.open
}

// deathKey handles a key while he is dead, and consumes every key.
func (g *GameControls) deathKey(key d2interface.KeyEvent) bool {
	if !g.dead() {
		return false
	}

	switch key.Key() {
	case d2enum.KeyEnter:
		g.deathHolder.LoadLastSave()
	case d2enum.KeyEscape:
		g.deathHolder.QuitToMenu()
	}

	return true
}

// refreshDeath reads the holder once a frame.
func (h *HUD) refreshDeath() {
	d := h.death
	if d == nil || h.gameControls == nil || h.gameControls.deathHolder == nil {
		return
	}

	var how Death

	d.open, how = h.gameControls.deathHolder.Death()
	d.lines = d.lines[:0]

	if d.open {
		d.lines = deathLines(how)

		// Nothing else is up over a dead man.
		if h.kit != nil {
			h.kit.open = false
		}

		if h.talents != nil {
			h.talents.open = false
		}

		// J1 (review B8): nor the journal, whose keys the death screen eats.
		if h.journal != nil && h.journal.open {
			h.gameControls.closeJournal()
		}
	}
}

func (h *HUD) renderDeath(target d2interface.Surface) {
	d := h.death
	if d == nil || !d.open {
		return
	}

	fillRect(target, 0, 0, screenWidth, screenHeight, deathShade)
	fillRect(target, deathX-1, deathY-1, deathWidth+2, deathHeight+2, deathEdge)
	fillRect(target, deathX, deathY, deathWidth, deathHeight, deathColor)

	cx := deathX + deathWidth/2

	d.title.SetText(d2ui.ColorTokenize(DeathTitle, d2ui.ColorTokenRed))
	d.title.SetPosition(cx, deathY+14)
	d.title.Render(target)

	for i, line := range d.lines {
		if i >= len(d.labels) {
			break
		}

		d.labels[i].SetText(line)
		d.labels[i].SetPosition(cx, deathY+44+i*deathLineGap)
		d.labels[i].Render(target)
	}

	d.footer.SetText(d2ui.ColorTokenize(DeathKeys, d2ui.ColorTokenGold))
	d.footer.SetPosition(cx, deathY+deathHeight-26)
	d.footer.Render(target)
}

// deathLines words a death: when, how, and what "last save" gives back.
func deathLines(how Death) []string {
	when := DeathOnTheDay
	if how.Night {
		when = DeathOnTheNight
	}

	lines := []string{fmt.Sprintf(when, how.Weekday, how.Day, strigoiMonthName(how.Month), how.Year, how.Time)}

	switch how.Cause {
	case DeathCauseFight:
		lines = append(lines, DeathByFight)
	case DeathCauseHunger:
		lines = append(lines, DeathByHunger)
	case DeathCauseThirst:
		lines = append(lines, DeathByThirst)
	}

	return append(lines, DeathWhatIsLost)
}
