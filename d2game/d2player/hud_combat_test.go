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

// THE MARKER REPORTS WHERE IT DREW, THIS FRAME (integrate-1oct, the
// combat-status review's C2). Drawn at a place that is not the constants'
// (the widget moved), combat_marker reports that place; on the next frame,
// with the widget not rendered (HUD.Render's beginCombatMarkerFrame and no
// draw), it reports nothing drawn and no square -- not the last frame's "!".
// THE CONTROL, in the test: the same frame before it ends reports the draw.
func TestTheCombatMarkerReportsWhereItDrewThisFrame(t *testing.T) {
	g := &GameControls{}
	h := &HUD{gameControls: g, combat: &combatMarker{}}
	g.hud = h

	g.SetCombatHolder(combatAt(true))

	x, y := combatMarkerX+7, combatMarkerY-11

	h.beginCombatMarkerFrame()
	h.renderCombatMarker(x, y, &rectSurface{})

	r := g.combatMarkerReport()
	if r["drew"] != CombatMarkerLetter || r["x"] != x || r["y"] != y || r["w"] != combatMarkerSize || r["h"] != combatMarkerSize {
		t.Fatalf("drawn at %d,%d this frame, combat_marker reports %v", x, y, r)
	}

	h.beginCombatMarkerFrame() // the next frame: the widget is not rendered

	r = g.combatMarkerReport()
	if r["drew"] != "" || r["x"] != 0 || r["y"] != 0 || r["w"] != 0 || r["h"] != 0 {
		t.Fatalf("a frame the marker was not drawn on, combat_marker reports %v", r)
	}

	if r["in_combat"] != true {
		t.Fatalf("in_combat is the status, drawn or not: %v", r)
	}
}
