package d2player

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
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
//
// THE ART DROPS IN (27 Sep 2026): no code when it lands. A hand draws its art
// when the file is there and fits, and its letter otherwise:
//
//   - the blade, left: BladeArtPath, one 48x48 frame;
//   - the torch, right: TorchArtPath, a 96x48 sheet of two 48x48 frames --
//     unlit, then lit -- with the usual .png.json beside it
//     ({"directions": 1, "frames_per_direction": 2}); the frame follows the
//     carried torch.
//
// The PNG loads through the creatures' and the hero's sheet loader
// (AssetManager.LoadAnimation). Art that is there and does not fit is
// reported in the log and the letter drawn: a wrong size would draw over the
// globes. The ui provider's hand_icons says which each hand drew ("F", or
// "art:blade") and the torch's frame. docs/art-spec.md has the paths.

const (
	// BladeArtPath is the left hand's icon art.
	BladeArtPath = "/data/strigoi/ui/hands/blade.png"
	// TorchArtPath is the right hand's: frame 0 unlit, frame 1 lit.
	TorchArtPath = "/data/strigoi/ui/hands/torch.png"

	torchFrameUnlit = 0
	torchFrameLit   = 1

	// handArtPrefix begins what a hand reports it drew when it drew art.
	handArtPrefix = "art:"
)

// handIcon is one hand's icon: the key it names and the label drawing it,
// or its art when there is some.
type handIcon struct {
	event d2enum.GameEvent
	label *d2ui.Label
	text  string // the letter on the label

	name  string                // the art's name: "blade", "torch"
	art   d2interface.Animation // nil: no art, the letter is drawn
	frame func() int            // the art's frame now; nil is frame 0

	drew      string // what it last drew: the letter, "art:<name>", or ""
	drewFrame int    // and the art's frame
}

// loadHands makes the two hands' labels and loads their art. Like the clock
// strip's the labels stay invisible to the UIManager: the skill widgets'
// render functions draw them.
func (h *HUD) loadHands() {
	newHand := func(event d2enum.GameEvent, name, path string, frames int) *handIcon {
		label := h.uiManager.NewLabel(d2resource.Font24, d2resource.PaletteStatic)
		label.Alignment = d2ui.HorizontalAlignCenter

		art, err := handArt(h.asset, path, frames)
		if err != nil {
			h.Errorf("hand art refused, drawing the key's letter: %v", err)
		}

		return &handIcon{event: event, label: label, name: name, art: art}
	}

	h.leftHand = newHand(d2enum.CombatStrike, "blade", BladeArtPath, 1)
	h.rightHand = newHand(d2enum.CombatTorch, "torch", TorchArtPath, 2)
	h.rightHand.frame = func() int {
		if h.torchLit() {
			return torchFrameLit
		}

		return torchFrameUnlit
	}
}

// handArt loads a hand's art from path: nil and no error when there is no
// file (the letter is drawn), nil and an error when there is one that is not
// one direction of `frames` frames, each 48x48 (iconArt, which the combat
// marker's art shares).
func handArt(asset *d2asset.AssetManager, path string, frames int) (d2interface.Animation, error) {
	return iconArt(asset, path, frames, skillIconWidth, skillIconHeight)
}

// choose is what the hand draws now: its art and the art's frame when it has
// art, else the letter it is given.
func (hand *handIcon) choose(letter string) (drew string, frame int) {
	if hand.art == nil {
		return letter, 0
	}

	if hand.frame != nil {
		frame = hand.frame()
	}

	return handArtPrefix + hand.name, frame
}

// torchLit reports a carried torch that is lit: the right hand's frame.
func (h *HUD) torchLit() bool {
	if h.gameControls == nil || h.gameControls.light == nil {
		return false
	}

	carried := h.gameControls.light.Carried()

	return carried != nil && carried.Lit
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

// renderHand draws a hand's art, or its key letter centred, in the 48x48
// icon whose bottom-left corner is x, bottom (where Diablo II's icon was
// drawn).
func (h *HUD) renderHand(hand *handIcon, x, bottom int, target d2interface.Surface) {
	if hand == nil {
		return
	}

	if text := h.handKeyText(hand.event); text != hand.text {
		hand.text = text
		hand.label.SetText(d2ui.ColorTokenize(text, d2ui.ColorTokenGold))
	}

	drew, frame := hand.choose(hand.text)
	hand.drew, hand.drewFrame = drew, frame

	if hand.art != nil {
		if err := hand.art.SetCurrentFrame(frame); err != nil {
			h.Error(err.Error())
			return
		}

		target.PushTranslation(x, bottom-skillIconHeight)
		hand.art.Render(target)
		target.Pop()

		return
	}

	if hand.text == "" {
		return
	}

	_, labelHeight := hand.label.GetSize()
	hand.label.SetPosition(x+skillIconWidth/2, bottom-skillIconHeight+(skillIconHeight-labelHeight)/2)
	hand.label.Render(target)
}

// HandsReport is what the two icons last drew: the key letter, "art:blade"
// or "art:torch" when the art is there, "" for none -- and for both under
// -classic, which draws Diablo II's icons instead -- and the torch art's frame
// (0 unlit, 1 lit; 0 while the right hand draws its letter).
func (h *HUD) HandsReport() (left, right string, rightFrame int) {
	if h.leftHand != nil {
		left = h.leftHand.drew
	}

	if h.rightHand != nil {
		right, rightFrame = h.rightHand.drew, h.rightHand.drewFrame
	}

	return left, right, rightFrame
}
