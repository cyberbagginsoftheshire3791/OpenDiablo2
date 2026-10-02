package d2geom

import "math"

// THE TILE WALK (fog of war: F1's line of sight, moved here in F4 so the map
// engine and fog's own cached grid walk the SAME code; 2 Oct 2026).
//
// Fog asks a different question from the beasts' checkLos: not "can this point
// see that point" but "can an eye see THAT TILE" -- and a tile that blocks
// sight is still seen, by its face. So the walk is tile-stepped, reads one
// cell per tile it crosses, and never reads the destination.

// tileWalkCellCap bounds the walk: a ray from one corner of the largest map
// to the other crosses fewer tiles than its width plus its height, so a walk
// that reaches this many has gone wrong (NaN coordinates) and answers blocked.
const tileWalkCellCap = 1 << 16

// tieEpsilon is how close the walk's two next boundary crossings must be to
// count as one: the ray passes through a tile corner.
const tieEpsilon = 1e-9

// TileLineClear reports whether the straight line from the point (fx, fy), in
// tiles, to the CENTRE of tile (tx, ty) crosses no tile that blocks STRICTLY
// BETWEEN the two: the start's own tile and the destination are never read,
// so a wall is seen by its face and only what stands behind it is hidden.
// cells is how many tiles the walk read. blocks answers for any tile,
// including ones off the map (which should block).
//
// It is a grid traversal (Amanatides & Woo): every tile the segment passes
// through, in order, and nothing it does not.
//
// WHERE THE RAY PASSES EXACTLY THROUGH A CORNER it steps diagonally, and is
// stopped only if BOTH tiles beside the corner block: a diagonal run of wall
// tiles touching at their corners is opaque, and one post beside the diagonal
// is not. Without this, whichever axis the tie broke toward would decide, and
// the answer would differ from the reverse ray's.
func TileLineClear(fx, fy float64, tx, ty int, blocks func(x, y int) bool) (clear bool, cells int) {
	return TileLineReads(fx, fy, tx, ty, func(x1, y1, x2, y2 int, corner bool) bool {
		if corner {
			return blocks(x1, y1) && blocks(x2, y2)
		}

		return blocks(x1, y1)
	})
}

// TileLineReads is TileLineClear's walk, telling its reader every read it
// makes, in order: one tile (corner false), or at a corner the two tiles
// beside it (corner true; the walk stops only if both block). read answers
// whether the walk is stopped there. A read of one tile counts one cell, a
// corner's two. It is the one walk: TileLineClear asks blocks through it, and
// fog of war (F4) records it once per offset -- from a tile's centre to
// another's the walk depends only on the offset between them -- and replays
// the record against its own grid.
func TileLineReads(fx, fy float64, tx, ty int, read func(x1, y1, x2, y2 int, corner bool) (stop bool)) (clear bool, cells int) {
	x, y := int(math.Floor(fx)), int(math.Floor(fy))
	if x == tx && y == ty {
		return true, 0
	}

	dx := float64(tx) + 0.5 - fx
	dy := float64(ty) + 0.5 - fy

	stepX, tMaxX, tDeltaX := axisWalk(fx, dx)
	stepY, tMaxY, tDeltaY := axisWalk(fy, dy)

	for cells < tileWalkCellCap {
		switch {
		case math.Abs(tMaxX-tMaxY) <= tieEpsilon:
			// Through a corner: the two tiles beside it, then the diagonal one.
			cells += 2

			if read(x+stepX, y, x, y+stepY, true) {
				return false, cells
			}

			x += stepX
			y += stepY
			tMaxX += tDeltaX
			tMaxY += tDeltaY
		case tMaxX < tMaxY:
			x += stepX
			tMaxX += tDeltaX
		default:
			y += stepY
			tMaxY += tDeltaY
		}

		if x == tx && y == ty {
			return true, cells
		}

		cells++

		if read(x, y, 0, 0, false) {
			return false, cells
		}
	}

	return false, cells
}

// axisWalk is one axis of the grid traversal from p along d: the step (+1, -1
// or 0), the ray parameter at which it first crosses a tile boundary, and the
// parameter between crossings. The segment runs over parameters 0..1.
func axisWalk(p, d float64) (step int, tMax, tDelta float64) {
	switch {
	case d > 0:
		return 1, (math.Floor(p) + 1 - p) / d, 1 / d
	case d < 0:
		return -1, (p - math.Floor(p)) / -d, 1 / -d
	default:
		return 0, math.Inf(1), math.Inf(1)
	}
}
