package d2player

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// THE HUD'S TWO SKILL ICONS ARE HIS TWO HANDS (Josh, 25 Sep 2026; M5.3's
// tables burst). The left button owns the blade hand and the right is the
// torch, so the icon beside each globe is the hand that button works: the
// left icon the blade (F strikes), the right icon the torch (L lights and
// douses). Their art is two 48x48 frames not yet drawn
// (claude/art-needs); until it lands each icon is the letter of the key that
// does the same thing, in the HUD's IM Fell, read from the key map so a
// rebound key shows its own letter. -classic draws Diablo II's skill icons.

// handIcon is one hand's icon: the key it names and the label drawing it.
type handIcon struct {
	event d2enum.GameEvent
	label *d2ui.Label
	text  string
}

// loadHands makes the two hands' labels. Like the clock strip's they stay
// invisible to the UIManager: the skill widgets' render functions draw them.
func (h *HUD) loadHands() {
	newHand := func(event d2enum.GameEvent) *handIcon {
		label := h.uiManager.NewLabel(d2resource.Font24, d2resource.PaletteStatic)
		label.Alignment = d2ui.HorizontalAlignCenter

		return &handIcon{event: event, label: label}
	}

	h.leftHand = newHand(d2enum.CombatStrike)
	h.rightHand = newHand(d2enum.CombatTorch)
}

// handKeyText is the letter of the key bound to a hand's verb (its primary
// binding, else its secondary), "" when neither is bound.
func (h *HUD) handKeyText(event d2enum.GameEvent) string {
	if h.gameControls == nil || h.gameControls.keyMap == nil {
		return ""
	}

	binding := h.gameControls.keyMap.GetKeysForGameEvent(event)
	if binding == nil {
		return ""
	}

	for _, key := range []d2enum.Key{binding.Primary, binding.Secondary} {
		if key != -1 {
			return h.gameControls.keyMap.KeyToString(key)
		}
	}

	return ""
}

// renderHand draws a hand's key letter centred in the 48x48 icon whose
// bottom-left corner is x, bottom (where Diablo II's icon was drawn).
func (h *HUD) renderHand(hand *handIcon, x, bottom int, target d2interface.Surface) {
	if hand == nil {
		return
	}

	if text := h.handKeyText(hand.event); text != hand.text {
		hand.text = text
		hand.label.SetText(d2ui.ColorTokenize(text, d2ui.ColorTokenGold))
	}

	if hand.text == "" {
		return
	}

	_, labelHeight := hand.label.GetSize()
	hand.label.SetPosition(x+skillIconWidth/2, bottom-skillIconHeight+(skillIconHeight-labelHeight)/2)
	hand.label.Render(target)
}

// HandsReport is what the two icons show: the key letter each draws, "" for
// none -- and for both under -classic, which draws Diablo II's icons instead.
func (h *HUD) HandsReport() (left, right string) {
	if h.leftHand != nil {
		left = h.leftHand.text
	}

	if h.rightHand != nil {
		right = h.rightHand.text
	}

	return left, right
}
