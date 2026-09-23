package d2player

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// M4.7 step 1: where the bodies lie, and the stake. The marks are PLACEHOLDER
// glyphs -- a pale-edged bar on the body's tile, dark red for a man's open body,
// brown for a carcass, grey once staked -- until there is art for the dead.
// X drives a stake through the body at his feet.

// CorpseMark is one body as the HUD marks it.
type CorpseMark struct {
	X, Y  float64
	Open  bool
	Human bool
}

// CorpseHolder is what the HUD and controls ask about the dead.
type CorpseHolder interface {
	CorpseMarks() []CorpseMark
	Stake() error
}

// stakeKey is X; D2's default map leaves it unbound.
const stakeKey = d2enum.KeyX

const (
	corpseMarkW     = 16 // a body lying down, not a dot: 6 px squares vanished on night ground
	corpseMarkH     = 6
	corpseMarkEdge  = 0xc8b89ac0
	corpseMarkHuman = 0x8a1c14e0
	corpseMarkBeast = 0x6a4a28c0
	corpseMarkShut  = 0x606060a0
)

// SetCorpseHolder attaches the game screen's owner of the dead.
func (g *GameControls) SetCorpseHolder(h CorpseHolder) { g.corpseHolder = h }

// renderCorpses draws a mark on every body on screen.
func (h *HUD) renderCorpses(target d2interface.Surface) {
	if h.gameControls == nil || h.gameControls.corpseHolder == nil || h.mapRenderer == nil {
		return
	}

	for _, m := range h.gameControls.corpseHolder.CorpseMarks() {
		sx, sy := h.mapRenderer.WorldToScreenF(m.X, m.Y)
		x, y := int(math.Floor(sx))-corpseMarkW/2, int(math.Floor(sy))-corpseMarkH/2

		if x < -corpseMarkW || y < -corpseMarkH || x > screenWidth || y > screenHeight {
			continue
		}

		colour := uint32(corpseMarkShut)

		switch {
		case m.Open && m.Human:
			colour = corpseMarkHuman
		case m.Open:
			colour = corpseMarkBeast
		}

		fillRect(target, x-1, y-1, corpseMarkW+2, corpseMarkH+2, corpseMarkEdge)
		fillRect(target, x, y, corpseMarkW, corpseMarkH, colour)
	}
}
