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
	X, Y   float64
	Open   bool
	Grave  bool
	Human  bool
	Downed bool // a risen man down, and not for long (M4.7 step 3b)
}

// CorpseHolder is what the HUD and controls ask about the dead.
type CorpseHolder interface {
	CorpseMarks() []CorpseMark
	Stake() error
	Dig() error
	// Search goes through the dead man at his feet (J2b).
	Search() error
	// DeadName is what one of the dead is called, or "" (M4.7 Q6a; before
	// the priest's tale, what he was in life -- 27 Sep 2026).
	DeadName(id string) string
}

// nameFor is what the player is shown for an entity, wherever he is shown a
// name -- the hover and the combat log alike: a villager's ROLE (T4), one of
// the dead's name (DeadName: what he was in life until the priest's tale, the
// dead after), or its own label. EVERY place that names a creature to the
// player goes through here; the combat log once read Label and called a risen
// man by the name the hover withheld. The HUD and the harness both read it, so
// what a script asserts is what is drawn.
func (g *GameControls) nameFor(e d2interface.MapEntity) string {
	if g.talkHolder != nil {
		if role := g.talkHolder.RoleFor(nameKey(e)); role != "" {
			return role
		}
	}

	if g.corpseHolder != nil {
		if name := g.corpseHolder.DeadName(e.ID()); name != "" {
			return name
		}
	}

	return e.Label()
}

const (
	// stakeKey is X; D2's default map leaves it unbound.
	stakeKey = d2enum.KeyX
	// digKey is D; D2's default map leaves it unbound too.
	digKey = d2enum.KeyD
	// searchKey is U (J2b); D2's default map leaves it unbound
	// (key_map.go ResetToDefault).
	searchKey = d2enum.KeyU
)

const (
	corpseMarkW     = 16 // a body lying down, not a dot: 6 px squares vanished on night ground
	corpseMarkH     = 6
	corpseMarkEdge  = 0xc8b89ac0
	corpseMarkHuman = 0x8a1c14e0
	corpseMarkBeast = 0x6a4a28c0
	corpseMarkGrave = 0x3a2e20e0
	corpseMarkDown  = 0xd8301cf0
	corpseMarkShut  = 0x606060a0
)

// SetCorpseHolder attaches the game screen's owner of the dead.
func (g *GameControls) SetCorpseHolder(h CorpseHolder) { g.corpseHolder = h }

// markColour is a mark's fill.
func markColour(m CorpseMark) uint32 {
	switch {
	case m.Downed:
		return corpseMarkDown
	case m.Open && m.Human:
		return corpseMarkHuman
	case m.Open:
		return corpseMarkBeast
	case m.Grave:
		return corpseMarkGrave
	}

	return corpseMarkShut
}

// corpseMarksShown are the corpse marks on ground he sees now: fog of war F2
// (BUG-107) -- no mark where he cannot see the body; all of them without fog.
func (h *HUD) corpseMarksShown() []CorpseMark {
	all := h.gameControls.corpseHolder.CorpseMarks()
	out := all[:0:0]

	for _, m := range all {
		if h.mapRenderer.Shows(m.X, m.Y) {
			out = append(out, m)
		}
	}

	return out
}

// renderCorpses draws a mark on every body on screen.
func (h *HUD) renderCorpses(target d2interface.Surface) {
	if h.gameControls == nil || h.gameControls.corpseHolder == nil || h.mapRenderer == nil {
		return
	}

	for _, m := range h.corpseMarksShown() {
		sx, sy := h.mapRenderer.WorldToScreenF(m.X, m.Y)
		x, y := int(math.Floor(sx))-corpseMarkW/2, int(math.Floor(sy))-corpseMarkH/2

		if x < -corpseMarkW || y < -corpseMarkH || x > screenWidth || y > screenHeight {
			continue
		}

		fillRect(target, x-1, y-1, corpseMarkW+2, corpseMarkH+2, corpseMarkEdge)
		fillRect(target, x, y, corpseMarkW, corpseMarkH, markColour(m))
	}
}
