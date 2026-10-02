package d2player

import (
	"image/color"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// rectSurface records the rectangles drawn on it, at their translation; any
// other call on it panics (the embedded interface is nil), so a test sees
// exactly what the marker draws.
type rectSurface struct {
	d2interface.Surface
	x, y  int
	stack [][2]int
	rects [][5]int // x, y, w, h, rgba
}

func (s *rectSurface) PushTranslation(x, y int) {
	s.stack = append(s.stack, [2]int{s.x, s.y})
	s.x, s.y = s.x+x, s.y+y
}

func (s *rectSurface) Pop() {
	last := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	s.x, s.y = last[0], last[1]
}

func (s *rectSurface) DrawRect(w, h int, c color.Color) {
	r, g, b, a := c.RGBA()
	rgba := int(r>>8)<<24 | int(g>>8)<<16 | int(b>>8)<<8 | int(a>>8)
	s.rects = append(s.rects, [5]int{s.x, s.y, w, h, rgba})
}

type combatAt bool

func (c combatAt) InCombat() bool { return bool(c) }

// THE COMBAT MARKER DRAWS ONLY IN COMBAT (the review of combat-status, C5):
// out of combat renderCombatMarker draws nothing and reports ""; in combat it
// draws its red-edged square at its place over the health globe and reports
// "!". THE CONTROL: the same HUD with the status flipped draws.
func TestTheCombatMarkerDrawsOnlyInCombat(t *testing.T) {
	g := &GameControls{}
	h := &HUD{gameControls: g, combat: &combatMarker{drew: "stale"}}
	g.hud = h

	g.SetCombatHolder(combatAt(false))

	out := &rectSurface{}
	h.renderCombatMarker(combatMarkerX, combatMarkerY, out)

	if len(out.rects) != 0 || h.combat.drew != "" {
		t.Fatalf("out of combat the marker drew %v (reported %q)", out.rects, h.combat.drew)
	}

	g.SetCombatHolder(combatAt(true))

	in := &rectSurface{}
	h.renderCombatMarker(combatMarkerX, combatMarkerY, in)

	if h.combat.drew != CombatMarkerLetter || len(in.rects) != 2 {
		t.Fatalf("in combat the marker drew %v (reported %q)", in.rects, h.combat.drew)
	}

	edge := [5]int{combatMarkerX, combatMarkerY, combatMarkerSize, combatMarkerSize, combatMarkerEdge}
	if in.rects[0] != edge {
		t.Fatalf("the marker's edge is %v, want %v (over the health globe)", in.rects[0], edge)
	}

	if g.combatMarkerReport()["drew"] != CombatMarkerLetter {
		t.Fatalf("the ui provider's combat_marker: %v", g.combatMarkerReport())
	}
}
