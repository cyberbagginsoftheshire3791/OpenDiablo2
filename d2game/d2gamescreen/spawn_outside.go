package d2gamescreen

import "math"

// outsideSearchTiles is how far past its spot an arrival is carried looking
// for ground outside an inside area. [DIAL] -- more than any village is wide.
const outsideSearchTiles = 64

// outsideAlong moves an arrival's spot, along the line from the quarry
// through it, to the nearest point outside every authored inside area (M5.4:
// the village within its fence -- what comes from the dark comes from
// outside, and must come in by the gate).
//
// With the quarry INSIDE, that is outward: the first outside point past the
// spot. With the quarry OUTSIDE (the player on the road, the spot fallen
// inside the village beyond him), there are two ways out along the line --
// back toward the quarry, or on through the village and out the far side --
// and the nearer is taken, so an arrival is not thrown across the village.
//
// A spot already outside, or a map with no inside areas, is returned
// unchanged; so is one with no outside ground within outsideSearchTiles,
// which the spawner then refuses like any other inside spot.
//
// Pure, so it is tested without an engine: inside answers for whole tiles.
func outsideAlong(fromX, fromY, spotX, spotY float64, inside func(x, y int) bool) (float64, float64) {
	in := func(x, y float64) bool { return inside(int(math.Floor(x)), int(math.Floor(y))) }

	if inside == nil || !in(spotX, spotY) {
		return spotX, spotY
	}

	dx, dy := spotX-fromX, spotY-fromY

	d := math.Hypot(dx, dy)
	if d == 0 {
		dx, dy, d = 1, 0, 1
	}

	ux, uy := dx/d, dy/d

	fwdX, fwdY, fwdOK := 0.0, 0.0, false

	for t := d; t <= d+outsideSearchTiles; t += 0.5 {
		if x, y := fromX+ux*t, fromY+uy*t; !in(x, y) {
			fwdX, fwdY, fwdOK = x, y, true

			break
		}
	}

	if !in(fromX, fromY) {
		for t := d - 0.5; t >= 0; t -= 0.5 {
			if x, y := fromX+ux*t, fromY+uy*t; !in(x, y) {
				if !fwdOK || d-t < math.Hypot(fwdX-spotX, fwdY-spotY) {
					return x, y
				}

				break
			}
		}
	}

	if fwdOK {
		return fwdX, fwdY
	}

	return spotX, spotY
}
