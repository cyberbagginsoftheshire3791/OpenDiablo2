package d2player

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// M4.7: where the bodies lie, the stake and the hasty grave. The marks are
// PLACEHOLDER glyphs -- a pale-edged bar on the body's tile, dark red for a
// man's open body, brown for a carcass, dark earth for a hasty grave, grey
// once staked -- until there is art for the dead. X drives a stake through
// the body at his feet; D digs it a hasty grave.

// CorpseMark is one body as the HUD marks it.
type CorpseMark struct {
	X, Y  float64
	Open  bool
	Grave bool
	Human bool
}

// CorpseHolder is what the HUD and controls ask about the dead.
type CorpseHolder interface {
	CorpseMarks() []CorpseMark
	Stake() error
	Dig() error
}

const (
	// stakeKey is X; D2's default map leaves it unbound.
	stakeKey = d2enum.KeyX
	// digKey is D; D2's default map leaves it unbound too.
	digKey = d2enum.KeyD
)

const (
	corpseMarkW     = 16 // a body lying down, not a dot: 6 px squares vanished on night ground
	corpseMarkH     = 6
	corpseMarkEdge  = 0xc8b89ac0
	corpseMarkHuman = 0x8a1c14e0
	corpseMarkBeast = 0x6a4a28c0
	corpseMarkGrave = 0x3a2e20e0
	corpseMarkShut  = 0x606060a0
)

// SetCorpseHolder attaches the game screen's owner of the dead.
func (g *GameControls) SetCorpseHolder(h CorpseHolder) { g.corpseHolder = h }

// markColour is a mark's fill.
func markColour(m CorpseMark) uint32 {
	switch {
	case m.Open && m.Human:
		return corpseMarkHuman
	case m.Open:
		return corpseMarkBeast
	case m.Grave:
		return corpseMarkGrave
	}

	return corpseMarkShut
}

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

		fillRect(target, x-1, y-1, corpseMarkW+2, corpseMarkH+2, corpseMarkEdge)
		fillRect(target, x, y, corpseMarkW, corpseMarkH, markColour(m))
	}
}
