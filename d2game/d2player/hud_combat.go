package d2player

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// THE COMBAT MARKER (Josh, 30 Sep 2026: "Marker by the health globe"). While
// he is in combat -- his fight live, a hostile chasing him, his own swing or
// hit or block reaction playing, or the few seconds after the last of these
// (the game screen's combat status, d2gamescreen/combat_status.go) -- he
// cannot save, and a small marker sits over the health globe saying so. It is
// drawn only then.
//
// THE ART DROPS IN, as the hands' does (hud_hands.go): CombatArtPath, one
// frame of combatMarkerSize square, drawn when the file is there and fits;
// until then the marker is CombatMarkerLetter in the HUD's font (Strigoi's IM
// Fell; Diablo II's under -classic), red on a dark square with a red edge.
// It is a glyph and not a letter because every letter the HUD draws beside
// the globes is a KEY (F strikes, L lights, and C opens his character panel):
// a letter here would read as one more key. Art that is there and does not
// fit is reported in the log and the glyph drawn. The ui provider's
// combat_marker says what it last drew ("!", "art:combat", or "" when not in
// combat) and where. docs/art-spec.md has the path.
//
// WHERE: centred over the health globe (hpGlobeX 30, 80 wide: its middle is
// x 70), its foot at y 500, above the globe's stone frame (whose top is at
// about y 505), clear of the globe, of the left hand's icon (x 117-165, y
// 552-600) and of the life tooltip's line only while the globe is not
// hovered. Measured on the screenshot TestTheCombatStatus takes.

const (
	// CombatArtPath is the marker's art: one frame, combatMarkerSize square.
	CombatArtPath = "/data/strigoi/ui/combat.png"

	// CombatMarkerLetter is the marker until its art lands.
	CombatMarkerLetter = "!"

	// combatMarkerArt is what the marker reports it drew when it drew art.
	combatMarkerArt = handArtPrefix + "combat"

	combatMarkerSize = 32
	combatMarkerX    = hpGlobeX + hpGlobeWidth/2 - combatMarkerSize/2 // 54
	combatMarkerY    = 468                                            // its foot at 500
	combatMarkerBand = 0x0c0a08dc
	combatMarkerEdge = 0x8c1c14ff
)

// CombatHolder is the game screen's combat status, as the HUD asks it.
type CombatHolder interface {
	// InCombat is true while he is in combat, and so cannot save.
	InCombat() bool
}

// SetCombatHolder attaches the game screen's combat status (bindGameControls).
func (g *GameControls) SetCombatHolder(c CombatHolder) { g.combatHolder = c }

// inCombat is the status, false until the game screen attaches.
func (g *GameControls) inCombat() bool {
	return g != nil && g.combatHolder != nil && g.combatHolder.InCombat()
}

// combatMarker is the marker: its glyph's label, or its art when there is
// some, and what it last drew.
type combatMarker struct {
	label *d2ui.Label
	art   d2interface.Animation
	drew  string
}

// loadCombatMarker makes the marker's label and loads its art. The label
// stays invisible to the UIManager: the marker's widget draws it.
func (h *HUD) loadCombatMarker() {
	label := h.uiManager.NewLabel(d2resource.Font24, d2resource.PaletteStatic)
	label.Alignment = d2ui.HorizontalAlignCenter
	label.SetText(d2ui.ColorTokenize(CombatMarkerLetter, d2ui.ColorTokenRed))

	art, err := iconArt(h.asset, CombatArtPath, 1, combatMarkerSize, combatMarkerSize)
	if err != nil {
		h.Errorf("combat marker art refused, drawing %q: %v", CombatMarkerLetter, err)
	}

	h.combat = &combatMarker{label: label, art: art}
}

// renderCombatMarker draws the marker at x, y (its top-left) while he is in
// combat, and nothing otherwise.
func (h *HUD) renderCombatMarker(x, y int, target d2interface.Surface) {
	m := h.combat
	if m == nil {
		return
	}

	m.drew = ""

	if !h.gameControls.inCombat() {
		return
	}

	if m.art != nil {
		if err := m.art.SetCurrentFrame(0); err != nil {
			h.Error(err.Error())
			return
		}

		target.PushTranslation(x, y)
		m.art.Render(target)
		target.Pop()

		m.drew = combatMarkerArt

		return
	}

	fillRect(target, x, y, combatMarkerSize, combatMarkerSize, combatMarkerEdge)
	fillRect(target, x+1, y+1, combatMarkerSize-2, combatMarkerSize-2, combatMarkerBand)

	_, labelHeight := m.label.GetSize()
	m.label.SetPosition(x+combatMarkerSize/2, y+(combatMarkerSize-labelHeight)/2)
	m.label.Render(target)

	m.drew = CombatMarkerLetter
}

// combatMarkerReport is the ui provider's combat_marker: what the marker last
// drew -- CombatMarkerLetter, "art:combat", or "" (not in combat) -- and its
// square on the screen.
func (g *GameControls) combatMarkerReport() map[string]interface{} {
	drew := ""
	if g.hud != nil && g.hud.combat != nil {
		drew = g.hud.combat.drew
	}

	return map[string]interface{}{
		"drew": drew, "in_combat": g.inCombat(),
		"x": combatMarkerX, "y": combatMarkerY, "w": combatMarkerSize, "h": combatMarkerSize,
	}
}

// iconArt loads a HUD icon's art from path: nil and no error when there is no
// file (the icon's letter or glyph is drawn), nil and an error when there is
// one that is not one direction of `frames` frames, each w x h.
// (handArt is this at a hand's 48x48.)
func iconArt(asset *d2asset.AssetManager, path string, frames, w, h int) (d2interface.Animation, error) {
	if asset == nil {
		return nil, nil
	}

	if exists, err := asset.FileExists(path); err != nil || !exists {
		return nil, nil
	}

	art, err := asset.LoadAnimation(path, "")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	if dirs, n := art.GetDirectionCount(), art.GetFrameCount(); dirs != 1 || n != frames {
		return nil, fmt.Errorf("%s is %d direction(s) of %d frame(s); this icon is 1 of %d (a sheet needs its .png.json)",
			path, dirs, n, frames)
	}

	for i := 0; i < frames; i++ {
		fw, fh, err := art.GetFrameSize(i)
		if err != nil {
			return nil, fmt.Errorf("%s frame %d: %w", path, i, err)
		}

		if fw != w || fh != h {
			return nil, fmt.Errorf("%s frame %d is %dx%d; this icon's frame is %dx%d", path, i, fw, fh, w, h)
		}
	}

	return art, nil
}
