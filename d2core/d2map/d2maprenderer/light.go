package d2maprenderer

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// fullyLit is daylight: the brightness at which the surface stack does
// nothing at all and a tile draws exactly as it did before M4.1.
const fullyLit = 1.0

// LightSampler answers one question — how lit is this world tile, from 0
// (pitch dark) to 1 (full daylight).
//
// The map renderer holds one so that night can dim the world without the
// renderer knowing anything about clocks, moons or torches. d2world.Light
// satisfies this interface by having the method the light model already
// needed, so d2maprenderer never imports d2world: no import cycle, and the
// renderer stays buildable and testable on its own (it links no ebiten, and
// the headless-CI rule depends on that staying true).
//
// Tile coordinates are world tiles, the same integers the render passes
// iterate over.
type LightSampler interface {
	Level(tileX, tileY int) float64
}

// SetLightSampler gives the renderer a light model to ask about each tile.
// Passing nil restores the flat, fully-lit rendering the engine had before
// M4.1 — which is also what an unset sampler does, so nothing that builds a
// MapRenderer is obliged to know about light at all.
func (mr *MapRenderer) SetLightSampler(sampler LightSampler) {
	mr.lightSampler = sampler
}

// tileLight is the brightness a tile is drawn at.
//
// With no sampler it is always daylight, and the surface's brightness guard
// (`brightness != 1`) means the colour matrix is never touched — the drawn
// pixels are identical to the pre-M4.1 renderer, not merely similar.
func (mr *MapRenderer) tileLight(tileX, tileY int) float64 {
	if mr.lightSampler == nil {
		return fullyLit
	}

	level := mr.lightSampler.Level(tileX, tileY)

	// A sampler that answers with nonsense dims nothing rather than
	// poisoning the frame: NaN reaches ebiten's colour matrix intact and
	// takes the whole draw with it.
	if math.IsNaN(level) {
		return fullyLit
	}

	return math.Max(0, math.Min(fullyLit, level))
}

// pushTileLight pushes a tile's brightness onto the draw target. Every call
// must be matched by target.Pop(); the render passes do that around the
// same body the viewport translation already wraps.
func (mr *MapRenderer) pushTileLight(target d2interface.Surface, tileX, tileY int) {
	target.PushBrightness(mr.tileLight(tileX, tileY))
}
