package d2gamescreen

import (
	"math"
	"testing"
)

// staircase is the shape the subtile A* returns for a diagonal: it is
// 8-connected (astar.go neighbourOffsets), so each step is one subtile in x AND
// y -- but starting off a tile's centre line, the x and y tile boundaries are
// crossed on DIFFERENT waypoints, which is what the first cost function
// double-charged.
func staircase(fx, fy float64, tiles int) [][2]float64 {
	var out [][2]float64

	x, y := fx, fy

	for i := 0; i < tiles*5; i++ {
		x += 0.2
		y += 0.2
		out = append(out, [2]float64{x, y})
	}

	return out
}

func TestRouteTilesChargesADiagonalAsChebyshev(t *testing.T) {
	t.Parallel()

	// Start mid-tile so every boundary crossing is a distinct waypoint.
	route := staircase(10.5, 10.3, 3)

	if got := routeTiles(10.5, 10.3, route); got != 3 {
		t.Fatalf("a three-tile diagonal staircase must cost 3 tiles; got %d", got)
	}

	// THE CONTROL, and the measured bug: counting each boundary crossing as a
	// step charges the same staircase as two steps per tile.
	crossings := 0
	cx, cy := math.Floor(10.5), math.Floor(10.3)

	for _, w := range route {
		wx, wy := math.Floor(w[0]), math.Floor(w[1])
		if wx != cx || wy != cy {
			crossings++ // the old counter: one step per waypoint that changed tile
		}

		cx, cy = wx, wy
	}

	if crossings <= 3 {
		t.Fatalf("the control must reproduce the overcount the first run measured; crossings=%d", crossings)
	}
}

func TestRouteTilesChargesADetour(t *testing.T) {
	t.Parallel()

	// Two tiles east as the crow flies, but walked round a wall: three tiles
	// north, two east, three south -- eight tiles of walking.
	route := [][2]float64{{10.5, 7.5}, {12.5, 7.5}, {12.5, 10.5}}

	if got := routeTiles(10.5, 10.5, route); got <= 2 {
		t.Fatalf("a detour must cost more than its crow-flies distance; got %d", got)
	}
}

func TestTruncateRouteStopsAtTheMove(t *testing.T) {
	t.Parallel()

	route := staircase(10.5, 10.3, 5)
	cut := truncateRoute(10.5, 10.3, route, 3)

	if len(cut) == 0 {
		t.Fatal("a three-tile budget must keep some of a five-tile walk")
	}

	end := cut[len(cut)-1]
	if got := chebyshevTiles(10.5, 10.3, end[0], end[1]); got != 3 {
		t.Fatalf("the cut walk must end three tiles out; ends %d out at %v", got, end)
	}

	if got := routeTiles(10.5, 10.3, cut); got > 3 {
		t.Fatalf("the cut walk must cost at most its budget; costs %d", got)
	}
}
