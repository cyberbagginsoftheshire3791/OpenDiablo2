package d2mapedit

import (
	"errors"
	"image"
)

// Blocked reports whether nobody may walk on x, y.
//
// This mirrors Map.Blocked (tiled.go:240-247) exactly, and it is worth spelling
// out because the two rules are INDEPENDENT: a tile with no floor is blocked, a
// tile a structure stands on is blocked, and a tile whose floor OR wall kind is
// marked blocked is blocked. Whether sight stops there is a different question
// with a different answer (BlocksSight) -- the village's fence and ditch are
// both blocked and both seen across.
//
// A gid the tilesets do not hold contributes nothing here; Validate reports it.
func (d *Doc) Blocked(x, y int) bool {
	if !d.m.onMap(x, y) {
		return true
	}

	floor := d.FloorTile(x, y)
	if floor == 0 {
		return true
	}

	if _, on := d.StructureOn(x, y); on {
		return true
	}

	if k, ok := d.m.kind(floor); ok && k.Blocked {
		return true
	}

	if wall := d.WallTile(x, y); wall != 0 {
		if k, ok := d.m.kind(wall); ok && k.Blocked {
			return true
		}
	}

	return false
}

// BlocksSight reports whether a line of sight stops at x, y, mirroring
// Map.BlocksSight (tiled.go:251-258). A tile with no floor does not stop
// sight: there is nothing there to stop it.
func (d *Doc) BlocksSight(x, y int) bool {
	if !d.m.onMap(x, y) {
		return false
	}

	if st, on := d.StructureOn(x, y); on {
		if k, ok := d.m.kind(st.GID); ok && k.BlocksSight {
			return true
		}
	}

	if floor := d.FloorTile(x, y); floor != 0 {
		if k, ok := d.m.kind(floor); ok && k.BlocksSight {
			return true
		}
	}

	if wall := d.WallTile(x, y); wall != 0 {
		if k, ok := d.m.kind(wall); ok && k.BlocksSight {
			return true
		}
	}

	return false
}

// ErrNoStart means the map has no single player_start to flood out from.
var ErrNoStart = errors.New("the map has no one player_start to walk out from")

// Reach is the answer to "what can be walked to": one bool per tile, row-major.
type Reach struct {
	Width, Height int
	Seen          []bool
}

// At reports whether x, y can be walked to. Off the map is false.
func (r Reach) At(x, y int) bool {
	if x < 0 || y < 0 || x >= r.Width || y >= r.Height {
		return false
	}

	return r.Seen[x+y*r.Width]
}

// Count is how many tiles can be walked to.
func (r Reach) Count() int {
	n := 0

	for _, seen := range r.Seen {
		if seen {
			n++
		}
	}

	return n
}

// Reachable floods out from the player_start over the tiles Blocked lets
// through, so the editor can warn that an edit has sealed the map -- a house
// across the only gap, a ditch closing the gate. It is the check that proves an
// enclosure, and it is the one an editor most needs, because a sealed village
// looks perfectly right on screen.
func (d *Doc) Reachable() (Reach, error) {
	start, ok := d.Start()
	if !ok {
		return Reach{}, ErrNoStart
	}

	t := start.Tile()
	if !d.m.onMap(t.X, t.Y) {
		return Reach{}, ErrNoStart
	}

	return d.ReachableFrom(t.X, t.Y), nil
}

// ReachableFrom floods out from one tile. Four-way, like the flood the shipped
// village's guard test walks (d2maptiled/village_test.go).
func (d *Doc) ReachableFrom(x, y int) Reach {
	r := Reach{Width: d.m.width, Height: d.m.height, Seen: make([]bool, d.m.width*d.m.height)}
	stack := []image.Point{image.Pt(x, y)}

	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if !d.m.onMap(p.X, p.Y) || r.Seen[d.m.at(p.X, p.Y)] || d.Blocked(p.X, p.Y) {
			continue
		}

		r.Seen[d.m.at(p.X, p.Y)] = true
		stack = append(stack,
			image.Pt(p.X+1, p.Y), image.Pt(p.X-1, p.Y),
			image.Pt(p.X, p.Y+1), image.Pt(p.X, p.Y-1))
	}

	return r
}

// HeightAt is the ground's height at x, y in levels: its floor tile's
// "height" (fog of war F4: an eye standing there sees further), 0 off the map
// or with no floor -- d2maptiled's Map.HeightAt.
func (d *Doc) HeightAt(x, y int) int {
	if !d.m.onMap(x, y) {
		return 0
	}

	if floor := d.FloorTile(x, y); floor != 0 {
		if k, ok := d.m.kind(floor); ok {
			return k.Height
		}
	}

	return 0
}

// TowerSightAt is the sight radius of the tower standing on x, y (fog of war
// F4: a structure with "sight_radius"), 0 when none does.
func (d *Doc) TowerSightAt(x, y int) int {
	if st, on := d.StructureOn(x, y); on {
		if k, ok := d.m.kind(st.GID); ok {
			return k.SightRadius
		}
	}

	return 0
}
