package d2player

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
)

// M4.4c-1 overhead bars: one thin bar over the player's squad(s) and over
// beasts and men, never the dead (ruled ask 4/7). Drawn by ONE uncached
// CustomWidget in panelGroup at foreground priority (the M4.4a clock-strip
// pattern) -- one widget, not one per unit; its callback iterates and issues
// DrawRects. DrawRect ignores the brightness/effect stack, so a rect is not
// dimmed by tile light: the bar's readability is decided by its colour, chosen
// in §0 from the contrast floor and the night spread at once.

const (
	overheadBarWidth  = 40 // [DIAL] §0 part 3 measured the spread at this size
	overheadBarHeight = 5
	overheadCueSize   = 3 // [DIAL] cue marks: SMALL and mid-luminance -- see below
	overheadCueGap    = 1

	// Colours are 0xRRGGBBAA. §0 part 3 chose crimson from BOTH measurements at
	// once (ruled ask 2d): the best night-spread margin (0.178, vs white's
	// 0.058) while clearing the daylight contrast floor (50.8) and the dark one
	// (73.7). A near-black frame carries boundary legibility the dark track
	// alone does not (empty-vs-daylight contrast is only 14).
	//
	// The selection outline and the cue marks (cueColor below) are held to the
	// SAME mid-luminance discipline as the fill (~L77-90) and the cues are
	// small: §0 measured the fill alone, and night_render_test.go caught the
	// first draft's bright gold outline (L147) + full-size cues pushing the
	// unlit-night spread to 0.28 over the 0.25 threshold. Dimmed and shrunk,
	// the whole ensemble sits back inside the envelope -- the shipped test is
	// the gate, and it is green.
	overheadBarFill   = 0xaa1e1eff // crimson (170,30,30), L=77
	overheadBarTrack  = 0x282828ff // dark track (40,40,40)
	overheadBarFrame  = 0x0a0a0aff // near-black 1px frame
	overheadBarSelect = 0x7d5a28ff // dim amber (125,90,40), L=85, the selected squad's outline [DIAL]
)

// cueColor is the colour of a stage-cue mark beside the bar. Mid-luminance and
// small so a screen of active cues stays inside §0 part 3's night-spread
// envelope; night_render_test.go is the backstop.
func cueColor(cue string) uint32 {
	switch cue {
	case cueHungry:
		return 0x785a28ff // dim amber, L=83
	case cueThirsty:
		return 0x2d5578ff // dim blue, L=83
	case cueNoReaction:
		return 0x505050ff // dim grey, L=80
	case cueShaken:
		return 0x5a3278ff // dim purple, L=87
	case cueDying:
		return 0x8c2d2dff // dim red, L=77
	}

	return 0x808080ff
}

// The stage-cue keys (S1 §5 conditions). They are machine keys reported through
// the "ui" provider and drawn as coloured marks; the sheet's display strings
// live in strigoi_strings.go (Article V.2). Beside the bar: hungry, thirsty,
// no-reaction, shaken, dying (brief clause 8).
const (
	cueHungry     = "hungry"
	cueThirsty    = "thirsty"
	cueNoReaction = "no_reaction"
	cueShaken     = "shaken"
	cueDying      = "dying"
)

// OverheadBar is one bar the HUD draws over a map entity (M4.4c-1). The game
// screen assembles the list -- it has the squads and the beast/man body
// registry -- and the HUD projects each entity's feet to the screen, anchors
// the bar over the sprite (the hover-loop arithmetic), draws it, and caches the
// projected rect for the "ui" provider (clause 6).
type OverheadBar struct {
	Entity   string
	Cur, Max int
	Cues     []string
	Selected bool
	Enemy    bool
}

// BarSource is what the HUD asks for the overhead bars each frame. The read is
// READ-ONLY and never adopts a body (never Bodies.BodyOf, which mutates the
// registry and would break the eager-adoption test): a player squad's bar is
// its pool from the Squads owner, a beast or man's is a non-adopting peek at
// the body registry, and the dead never get one (ShowsBar).
type BarSource interface {
	OverheadBars() []OverheadBar
}

// overheadBarRender is one projected bar, cached each Advance and drawn from the
// cache in the render callback -- the same compute-then-draw split the clock
// strip uses, so the "ui" provider reports exactly what is drawn even on a
// stepped frame the harness never rendered.
type overheadBarRender struct {
	entity     string
	x, y, w, h int
	fill       float64
	cues       []string
	selected   bool
	enemy      bool
}

// refreshOverheadBars recomputes the projected bars from the bar source. Called
// from HUD.Advance so the cache is fresh every stepped frame. The anchor is the
// hover-loop arithmetic (hud.go:631-667): feet -> screen (WorldToScreenF,
// floored), minus the entity's RenderOffset, minus its GetSize() height, minus
// the pad -- GROUNDED, not a dial.
func (h *HUD) refreshOverheadBars() {
	if h.bars == nil || h.mapEngine == nil || h.mapRenderer == nil {
		h.overheadBars = nil
		return
	}

	src := h.bars.OverheadBars()
	entities := h.mapEngine.Entities()
	out := make([]overheadBarRender, 0, len(src))

	for _, b := range src {
		ent, ok := entities[b.Entity]
		if !ok {
			continue
		}

		sxf, syf := h.mapRenderer.WorldToScreenF(ent.GetPositionF())
		ex, ey := int(math.Floor(sxf)), int(math.Floor(syf))

		entPos := ent.GetPosition()
		offset := entPos.RenderOffset()
		xOff, yOff := int(offset.X()), int(offset.Y())

		_, entHeight := ent.GetSize()

		anchorX := ex - xOff
		anchorY := ey - yOff - entHeight - hoverLabelOuterPad

		fill := 0.0
		if b.Max > 0 {
			fill = float64(b.Cur) / float64(b.Max)
		}

		fill = math.Max(0, math.Min(1, fill))

		out = append(out, overheadBarRender{
			entity:   b.Entity,
			x:        anchorX - overheadBarWidth/2,
			y:        anchorY - overheadBarHeight,
			w:        overheadBarWidth,
			h:        overheadBarHeight,
			fill:     fill,
			cues:     b.Cues,
			selected: b.Selected,
			enemy:    b.Enemy,
		})
	}

	h.overheadBars = out
}

// renderOverheadBars draws the cached bars: for the selected squad a muted-gold
// 1px ring, then a near-black frame, the dark track, and the crimson fill
// proportion, then the stage-cue marks to the right. No effect pushed (DrawRect
// ignores it anyway); the bar's LENGTH encodes health, the colour is constant.
func (h *HUD) renderOverheadBars(target d2interface.Surface) {
	for _, b := range h.overheadBars {
		if b.selected {
			fillRect(target, b.x-2, b.y-2, b.w+4, 1, overheadBarSelect)
			fillRect(target, b.x-2, b.y+b.h+1, b.w+4, 1, overheadBarSelect)
			fillRect(target, b.x-2, b.y-2, 1, b.h+4, overheadBarSelect)
			fillRect(target, b.x+b.w+1, b.y-2, 1, b.h+4, overheadBarSelect)
		}

		fillRect(target, b.x-1, b.y-1, b.w+2, b.h+2, overheadBarFrame)
		fillRect(target, b.x, b.y, b.w, b.h, overheadBarTrack)
		fillRect(target, b.x, b.y, int(float64(b.w)*b.fill), b.h, overheadBarFill)

		cx := b.x + b.w + 3
		for _, cue := range b.cues {
			fillRect(target, cx, b.y, overheadCueSize, overheadCueSize, cueColor(cue))
			cx += overheadCueSize + overheadCueGap
		}
	}
}

// fillRect draws one opaque rect at absolute screen coordinates. The uncached
// CustomWidget applies no translation, so each rect sets its own (custom_widget
// .go), the clock-strip pattern.
func fillRect(target d2interface.Surface, x, y, w, h int, rgba uint32) {
	if w <= 0 || h <= 0 {
		return
	}

	target.PushTranslation(x, y)
	target.DrawRect(w, h, d2util.Color(rgba))
	target.Pop()
}
