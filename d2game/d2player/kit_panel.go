package d2player

import (
	"fmt"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// T2, the kit's face (23 Sep 2026). Two things drawn by one foreground widget:
//
//   - THE LOADOUT CHOICE, once per hero, the first time he enters the world
//     (Squads on Screen §8, ruled 11 Sep: shield OR torch, chosen once at
//     start). The world is held while it is open. Keys 1 and 2, or a click.
//   - THE KIT PANEL on I, replacing the D2 grid inventory (U1 §5.2: six worn
//     slots and a pack that is a list, not a grid). Click a worn row to take
//     it off, a pack row to put it on. The last refusal is shown under it.
//
// The game screen owns the kit and its save; this file only draws and routes
// clicks through KitHolder.

// KitHolder is what the controls ask about the local hero's gear.
type KitHolder interface {
	Kit() *d2items.Kit
	Catalog() *d2items.Catalog
	ChoosingLoadout() bool
	ChooseLoadout(name string) error
	EquipFromPack(i int) error
	UnequipSlot(slot d2items.Slot) error
}

const (
	kitPanelX      = 405
	kitPanelY      = 40
	kitPanelWidth  = 385
	kitRowHeight   = 15
	kitPadX        = 10
	kitPadY        = 6
	kitMaxRows     = 26
	kitPanelColor  = 0x0c0a08e6
	kitPanelEdge   = 0x5a4628ff
	kitRowHover    = 0x2a2218ff
	choiceX        = 140
	choiceY        = 150
	choiceWidth    = 520
	choiceHeight   = 210
	choiceOptionH  = 70
	choiceOptionY0 = 60
)

// kitRow is one clickable line on the panel.
type kitRow struct {
	text string
	slot d2items.Slot // a worn row
	pack int          // a pack row, or -1
	y    int
}

// kitOverlay is the HUD's cache for both panels.
type kitOverlay struct {
	open     bool
	rows     []kitRow
	labels   []*d2ui.Label
	texts    []string
	title    *d2ui.Label
	notice   string
	choosing bool
	choice   [4]*d2ui.Label
}

func newKitOverlay(ui *d2ui.UIManager) *kitOverlay {
	k := &kitOverlay{}

	mk := func() *d2ui.Label {
		l := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
		l.Alignment = d2ui.HorizontalAlignLeft

		return l
	}

	k.title = mk()
	for i := 0; i < kitMaxRows; i++ {
		k.labels = append(k.labels, mk())
		k.texts = append(k.texts, "")
	}

	for i := range k.choice {
		k.choice[i] = mk()
	}

	k.choice[0].SetText(d2ui.ColorTokenize(KitChooseTitle, d2ui.ColorTokenGold))
	k.choice[1].SetText(d2ui.ColorTokenize(KitChooseBoard, d2ui.ColorTokenWhite))
	k.choice[2].SetText(d2ui.ColorTokenize(KitChooseTorch, d2ui.ColorTokenWhite))
	k.choice[3].SetText(d2ui.ColorTokenize(KitChooseHint, d2ui.ColorTokenGrey))
	k.title.SetText(d2ui.ColorTokenize(KitTitle, d2ui.ColorTokenGold))

	return k
}

// SetKitHolder attaches the game screen's kit owner.
func (g *GameControls) SetKitHolder(h KitHolder) { g.kitHolder = h }

// toggleKitPanel is I: the Strigoi kit, not the D2 grid. It takes the right
// half of the screen, so a D2 right panel is closed first rather than drawn
// beneath it and fed the same click.
func (g *GameControls) toggleKitPanel() {
	if g.hud == nil || g.hud.kit == nil || g.kitHolder == nil || g.kitHolder.Kit() == nil {
		return
	}

	if !g.hud.kit.open {
		g.clearRightScreenSide()
	}

	g.hud.kit.open = !g.hud.kit.open
	g.hud.kit.notice = ""
}

// overKitPanel reports a screen point on the open kit panel.
func (g *GameControls) overKitPanel(mx, my int) bool {
	if g.hud == nil || g.hud.kit == nil || !g.hud.kit.open {
		return false
	}

	return inRect(mx, my, kitPanelX, kitPanelY, kitPanelWidth, kitPanelHeight(len(g.hud.kit.rows)))
}

// kitKey handles keys while the loadout choice is open. It consumes every key
// but Escape, because nothing else should happen before he has chosen.
func (g *GameControls) kitKey(key d2interface.KeyEvent) bool {
	if g.kitHolder == nil || !g.kitHolder.ChoosingLoadout() {
		return false
	}

	switch key.Key() {
	case kitKeyOne:
		g.chooseLoadout(d2items.LoadoutSwordAndBoard)
	case kitKeyTwo:
		g.chooseLoadout(d2items.LoadoutTorchAndBlade)
	}

	return true
}

func (g *GameControls) chooseLoadout(name string) {
	if err := g.kitHolder.ChooseLoadout(name); err != nil && g.hud != nil && g.hud.kit != nil {
		g.hud.kit.notice = err.Error()
	}
}

// kitClick routes a click on either panel; it reports whether it was consumed.
func (g *GameControls) kitClick(mx, my int) bool {
	if g.hud == nil || g.hud.kit == nil || g.kitHolder == nil {
		return false
	}

	k := g.hud.kit

	if g.kitHolder.ChoosingLoadout() {
		switch {
		case inRect(mx, my, choiceX, choiceY+choiceOptionY0, choiceWidth, choiceOptionH/2):
			g.chooseLoadout(d2items.LoadoutSwordAndBoard)
		case inRect(mx, my, choiceX, choiceY+choiceOptionY0+choiceOptionH/2, choiceWidth, choiceOptionH/2):
			g.chooseLoadout(d2items.LoadoutTorchAndBlade)
		}

		return true // the choice is modal
	}

	if !k.open || !inRect(mx, my, kitPanelX, kitPanelY, kitPanelWidth, kitPanelHeight(len(k.rows))) {
		return false
	}

	for _, r := range k.rows {
		if my < r.y || my >= r.y+kitRowHeight {
			continue
		}

		var err error

		switch {
		case r.pack >= 0:
			err = g.kitHolder.EquipFromPack(r.pack)
		case r.slot != "":
			err = g.kitHolder.UnequipSlot(r.slot)
		default:
			return true
		}

		k.notice = ""
		if err != nil {
			k.notice = err.Error()
		}

		return true
	}

	return true
}

func kitPanelHeight(rows int) int {
	return kitPadY*2 + kitRowHeight*(rows+3)
}

func inRect(mx, my, x, y, w, h int) bool {
	return mx >= x && mx < x+w && my >= y && my < y+h
}

// refreshKit recomputes the panel rows from the kit.
func (h *HUD) refreshKit() {
	k := h.kit
	if k == nil || h.gameControls == nil || h.gameControls.kitHolder == nil {
		return
	}

	holder := h.gameControls.kitHolder
	k.choosing = holder.ChoosingLoadout()

	kit := holder.Kit()
	if !k.open || kit == nil {
		k.rows = k.rows[:0]
		return
	}

	rows := k.rows[:0]
	y := kitPanelY + kitPadY + kitRowHeight

	add := func(r kitRow) {
		if len(rows) >= kitMaxRows {
			return
		}

		r.y = y
		y += kitRowHeight
		rows = append(rows, r)
	}

	for _, slot := range d2items.WornSlots() {
		name := d2ui.ColorTokenize(KitEmpty, d2ui.ColorTokenGrey)

		if it, inst, ok := kit.ItemIn(slot); ok {
			name = describe(kit, it, inst, slot)
		}

		add(kitRow{text: fmt.Sprintf("%-6s %s", kitSlotName(slot), name), slot: slot, pack: -1})
	}

	add(kitRow{text: d2ui.ColorTokenize(fmt.Sprintf(KitPackHeader, kit.LoadKg()), d2ui.ColorTokenGold), pack: -1})

	for i := range kit.Pack {
		if it, inst, ok := kit.PackItem(i); ok {
			add(kitRow{text: "  " + describe(kit, it, inst, ""), pack: i})
		}
	}

	k.rows = rows

	for i := range k.labels {
		text := ""
		if i < len(rows) {
			text = rows[i].text
		}

		if text != k.texts[i] {
			k.texts[i] = text
			k.labels[i].SetText(text)
		}
	}
}

// describe is one item in words: name, count, and what matters about it.
func describe(kit *d2items.Kit, it *d2items.Item, inst *d2items.Instance, slot d2items.Slot) string {
	parts := []string{it.Name}

	if inst.Count > 1 {
		parts[0] = fmt.Sprintf("%s x%d", it.Name, inst.Count)
	}

	switch {
	case it.Weapon != nil && it.Weapon.Ranged:
		parts = append(parts, KitRangedNote)
	case it.Weapon != nil:
		parts = append(parts, fmt.Sprintf("%d-%d %s", it.Weapon.Min, it.Weapon.Max, it.Weapon.Class))
	case it.Armour != nil && it.Armour.Block:
		parts = append(parts, fmt.Sprintf(KitShieldNote, inst.Points))
	case it.Armour != nil && it.Armour.Points > 0:
		parts = append(parts, fmt.Sprintf(KitArmourNote, inst.Points))
	case it.Light != nil:
		parts = append(parts, fmt.Sprintf(KitTorchNote, inst.BurnLeft))
	}

	if slot != "" && it.Armour != nil && it.Armour.Points > 0 {
		if c := kit.ArmourCondition(slot); c != d2items.Sound {
			parts = append(parts, string(c))
		}
	}

	return strings.Join(parts, "  ")
}

// renderKit draws the choice, or the panel, from the cache.
func (h *HUD) renderKit(target d2interface.Surface) {
	k := h.kit
	if k == nil {
		return
	}

	if k.choosing {
		fillRect(target, choiceX-1, choiceY-1, choiceWidth+2, choiceHeight+2, kitPanelEdge)
		fillRect(target, choiceX, choiceY, choiceWidth, choiceHeight, kitPanelColor)

		ys := []int{choiceY + 16, choiceY + choiceOptionY0 + 8, choiceY + choiceOptionY0 + choiceOptionH/2 + 8,
			choiceY + choiceHeight - 30}

		for i, l := range k.choice {
			l.SetPosition(choiceX+kitPadX*2, ys[i])
			l.Render(target)
		}

		if k.notice != "" {
			k.title.SetText(d2ui.ColorTokenize(k.notice, d2ui.ColorTokenRed))
			k.title.SetPosition(choiceX+kitPadX*2, choiceY+choiceHeight-14)
			k.title.Render(target)
		}

		return
	}

	if !k.open {
		return
	}

	height := kitPanelHeight(len(k.rows))
	fillRect(target, kitPanelX-1, kitPanelY-1, kitPanelWidth+2, height+2, kitPanelEdge)
	fillRect(target, kitPanelX, kitPanelY, kitPanelWidth, height, kitPanelColor)

	k.title.SetText(d2ui.ColorTokenize(KitTitle, d2ui.ColorTokenGold))
	k.title.SetPosition(kitPanelX+kitPadX, kitPanelY+kitPadY)
	k.title.Render(target)

	for i, r := range k.rows {
		k.labels[i].SetPosition(kitPanelX+kitPadX, r.y)
		k.labels[i].Render(target)
	}

	if k.notice != "" {
		k.title.SetText(d2ui.ColorTokenize(k.notice, d2ui.ColorTokenYellow))
		k.title.SetPosition(kitPanelX+kitPadX, kitPanelY+height-kitRowHeight-kitPadY)
		k.title.Render(target)
	}
}

func kitSlotName(s d2items.Slot) string {
	switch s {
	case d2items.SlotMain:
		return KitSlotMain
	case d2items.SlotOff:
		return KitSlotOff
	case d2items.SlotBody:
		return KitSlotBody
	case d2items.SlotHead:
		return KitSlotHead
	case d2items.SlotBelt1, d2items.SlotBelt2:
		return KitSlotBelt
	}

	return string(s)
}
