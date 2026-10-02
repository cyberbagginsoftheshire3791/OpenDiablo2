package masterref

// TEST-ONLY: master 05ba3666's d2mapengine, verbatim (only the package line
// differs), with this file's grid builder -- the BUG-115 review's frozen
// reference. It sits under testdata so ./... (go build, go vet, the reach
// gate) never builds it; d2mapengine's and d2gamescreen's tests import it to
// hold the BUG-115 router to master's answers. Check it is still verbatim:
//
//	for f in astar authored corridor doc engine entity_id map_tile pathfind rand tile_sight; do
//	  git show 05ba3666:d2core/d2map/d2mapengine/$f.go | sed '0,/^package d2mapengine/s//package masterref/' |
//	    diff -q - d2core/d2map/d2mapengine/testdata/masterref/$f.go; done

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2geom"
)

// NewGrid is an empty w x h-tile engine.
func NewGrid(w, h int) *MapEngine {
	m := &MapEngine{}
	m.size = d2geom.Size{Width: w, Height: h}
	m.tiles = make([]MapTile, w*h)

	return m
}

// BlockSub sets BlockWalk on one subtile.
func (m *MapEngine) BlockSub(x, y int) {
	if f := m.SubTileAt(x, y); f != nil {
		f.BlockWalk = true
		f.BlockLOS = true
	}
}

// TruncateTiles drops the last n tiles of the slice (a short map).
func (m *MapEngine) TruncateTiles(n int) { m.tiles = m.tiles[:len(m.tiles)-n] }

// CoarsePathRef is master's coarsePath.
func (m *MapEngine) CoarsePathRef(ax, ay, bx, by int) [][2]int {
	p := m.coarsePath(subTile{ax, ay}, subTile{bx, by})
	if p == nil {
		return nil
	}

	out := make([][2]int, len(p))
	for i, s := range p {
		out[i] = [2]int{s.x, s.y}
	}

	return out
}
