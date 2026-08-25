package d2maprenderer

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// HarnessTileImage is one cached tile surface with its cache key, exposed for
// the Phase 3 playtest harness's dump_surface diagnostic (P3 spec §5.3 — the
// black-floor experiment: dump what generateFloorCache built and see whether
// the diamond is in the cache or lost in compositing).
type HarnessTileImage struct {
	Style       int
	Sequence    int
	TileType    int
	RandomIndex int
	Surface     d2interface.Surface
}

// HarnessCachedFloorTiles returns up to max distinct cached floor-tile
// surfaces for the current map, in tile-iteration order. It reads the same
// cache records generateFloorCache writes. Call on the game goroutine.
func (mr *MapRenderer) HarnessCachedFloorTiles(max int) []HarnessTileImage {
	var out []HarnessTileImage

	if mr.mapEngine == nil || max <= 0 {
		return out
	}

	seen := make(map[uint32]bool)
	tiles := *mr.mapEngine.Tiles()

	for idx := range tiles {
		tile := &tiles[idx]

		for i := range tile.Components.Floors {
			floor := &tile.Components.Floors[i]
			if floor.Hidden() || floor.Prop1 == 0 {
				continue
			}

			key := uint32(floor.Style)<<24 | uint32(floor.Sequence)<<16 | uint32(floor.RandomIndex)
			if seen[key] {
				continue
			}

			seen[key] = true

			sfc := mr.getImageCacheRecord(floor.Style, floor.Sequence, 0, floor.RandomIndex)
			if sfc == nil {
				continue
			}

			out = append(out, HarnessTileImage{
				Style:       int(floor.Style),
				Sequence:    int(floor.Sequence),
				TileType:    0,
				RandomIndex: int(floor.RandomIndex),
				Surface:     sfc,
			})

			if len(out) >= max {
				return out
			}
		}
	}

	return out
}
