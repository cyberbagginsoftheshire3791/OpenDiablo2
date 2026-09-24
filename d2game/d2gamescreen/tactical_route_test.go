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

// A straight run over open ground comes back from the path finder as ONE
// waypoint at its far end. Six tiles away with a three-tile move, the walk
// must go three tiles along it, not nowhere: dropping the whole leg froze a
// risen man six tiles off across the village green for forty rounds.
func TestTruncateRouteCutsALongStraightLeg(t *testing.T) {
	t.Parallel()

	for _, leg := range [][2]float64{{16.5, 10.5}, {16.5, 16.5}, {10.5, 4.2}} {
		cut := truncateRoute(10.5, 10.5, [][2]float64{leg}, 3, nil)

		if len(cut) != 1 {
			t.Fatalf("to %v: a three-tile move along a six-tile straight leg must go somewhere; got %v", leg, cut)
		}

		end := cut[0]
		if got := chebyshevTiles(10.5, 10.5, end[0], end[1]); got != 3 {
			t.Fatalf("to %v: the cut walk must end three tiles out; ends %d out at %v", leg, got, end)
		}

		if got := routeTiles(10.5, 10.5, cut); got > 3 {
			t.Fatalf("to %v: the cut walk must cost at most its move; costs %d", leg, got)
		}

		// On the leg, not beside it.
		dx, dy := leg[0]-10.5, leg[1]-10.5
		if cross := dx*(end[1]-10.5) - dy*(end[0]-10.5); math.Abs(cross) > 1e-6 {
			t.Fatalf("to %v: the cut end %v is off the straight leg", leg, end)
		}
	}

	// A leg the move covers whole is kept whole.
	if cut := truncateRoute(10.5, 10.5, [][2]float64{{12.5, 10.5}}, 3, nil); len(cut) != 1 || cut[0] != [2]float64{12.5, 10.5} {
		t.Fatalf("a two-tile leg inside a three-tile move must be kept as it is: %v", cut)
	}
}

// A packmate standing where the cut would end: the walk stops short of his
// tile, behind him, rather than dropping the leg and not moving at all.
func TestTruncateRouteStopsBehindATakenTile(t *testing.T) {
	t.Parallel()

	taken := func(x, y float64) bool { return !(math.Floor(x) == 13 && math.Floor(y) == 10) }

	cut := truncateRoute(10.5, 10.5, [][2]float64{{16.5, 10.5}}, 3, taken)
	if len(cut) != 1 {
		t.Fatalf("a packmate three tiles out must not freeze the walk: %v", cut)
	}

	if got := chebyshevTiles(10.5, 10.5, cut[0][0], cut[0][1]); got != 2 {
		t.Fatalf("the walk must stop on the tile before the taken one (two out); ends %d out at %v", got, cut[0])
	}

	// Every tile ahead taken: he never steps onto one.
	blocked := func(x, y float64) bool { return math.Floor(x) == 10 }
	for _, w := range truncateRoute(10.5, 10.5, [][2]float64{{16.5, 10.5}}, 3, blocked) {
		if math.Floor(w[0]) != 10 {
			t.Fatalf("with every tile ahead taken the walk entered one: %v", w)
		}
	}
}

func TestTruncateRouteStopsAtTheMove(t *testing.T) {
	t.Parallel()

	route := staircase(10.5, 10.3, 5)
	cut := truncateRoute(10.5, 10.3, route, 3, nil)

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
