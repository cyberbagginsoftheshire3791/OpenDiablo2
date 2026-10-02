package d2maprenderer

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// FOG OF WAR, F1 (1 Oct 2026; claude/fog-of-war-build-plan.md §2.5).
//
//	Unexplored         the tile and everything on it is not drawn: the screen
//	                   is cleared black every frame, so it is exact black.
//	Explored, unseen   the tile is drawn greyed -- saturation MemorySaturation,
//	                   brightness min(its light, MemoryLevel), a min and never a
//	                   product, so nothing darkens twice -- and NOTHING standing
//	                   on it is drawn (no creatures or people: ruling 2).
//	Visible            as it was before fog.
//
// A NIL SAMPLER IS THE UNFOGGED DRAW, CALL FOR CALL: no tile skipped, no push
// added, the one PushBrightness and one Pop per tile the passes made before
// (TestNoFogSamplerIsTheUnfoggedDraw). The World Editor never sets one.

// FogSampler answers, per world tile, whether the player has ever seen it and
// whether he sees it now, and how remembered ground is drawn. d2world.Fog is
// one; d2maprenderer imports no world code (as LightSampler).
type FogSampler interface {
	FogAt(tileX, tileY int) (explored, visible bool)
	MemoryLook() (level, saturation float64)
}

// SetFogSampler gives the renderer the fog to draw by; nil draws without fog.
func (mr *MapRenderer) SetFogSampler(sampler FogSampler) {
	mr.fogSampler = sampler
}

// HasFog says whether the renderer draws with fog (a sampler is set).
func (mr *MapRenderer) HasFog() bool {
	return mr.fogSampler != nil
}

// tileDrawn is whether a tile is drawn at all: always without fog; with fog,
// once explored -- or while it is visible without being explored, which is a
// contact's tile (an enemy of his fight on ground he never saw, F2 review B2:
// shown while the fight lasts, black again after it).
func (mr *MapRenderer) tileDrawn(tileX, tileY int) bool {
	if mr.fogSampler == nil {
		return true
	}

	explored, visible := mr.fogSampler.FogAt(tileX, tileY)

	return explored || visible
}

// pushTileView pushes how a drawn tile looks -- its light, and for remembered
// ground the memory's grey -- and returns how many states it pushed, for
// popTileView. Without fog, or on a visible tile, it is pushTileLight: one push.
func (mr *MapRenderer) pushTileView(target d2interface.Surface, tileX, tileY int) int {
	if mr.fogSampler == nil {
		mr.pushTileLight(target, tileX, tileY)

		return 1
	}

	if _, visible := mr.fogSampler.FogAt(tileX, tileY); visible {
		mr.pushTileLight(target, tileX, tileY)

		return 1
	}

	level, saturation := mr.fogSampler.MemoryLook()

	target.PushSaturation(clamp01(saturation))
	target.PushBrightness(math.Min(mr.tileLight(tileX, tileY), clamp01(level)))

	return 2 //nolint:gomnd // the saturation and the brightness
}

// popTileView pops what pushTileView pushed, one Pop per push (without fog,
// exactly the one Pop the passes made).
func popTileView(target d2interface.Surface, pushed int) {
	for ; pushed > 0; pushed-- {
		target.Pop()
	}
}

// Shows says whether something standing at world (x, y) is drawn: always
// without fog; with fog, only on a tile he sees now. It buckets the point to
// its tile as the render passes bucket entities (int of the world position).
//
// IT IS THE ONE VISIBILITY PREDICATE (F2; BUG-107): the entity draw, the
// overhead bars and the game's enemy-bar list, the hover and talk label and
// their hit test, the corpse marks, the tactical diamonds and click-to-strike
// all ask it, so nothing the player cannot see is named, barred, marked or
// struck at. "He sees it" already holds the night's three ways to be seen:
// within his reach, lit with a clear line, or an enemy of his own fight (fog's
// contact eyes, Q4).
func (mr *MapRenderer) Shows(x, y float64) bool {
	if mr == nil || mr.fogSampler == nil {
		return true
	}

	_, visible := mr.fogSampler.FogAt(int(x), int(y))

	return visible
}

// ShowsEntity is Shows for an entity, at its position as the passes read it.
func (mr *MapRenderer) ShowsEntity(e d2interface.MapEntity) bool {
	if mr == nil || mr.fogSampler == nil {
		return true
	}

	pos := e.GetPosition()
	w := pos.World()

	return mr.Shows(w.X(), w.Y())
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 1
	}

	return math.Max(0, math.Min(1, v))
}
