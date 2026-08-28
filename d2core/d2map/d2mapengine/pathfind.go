package d2mapengine

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// PathFind finds a walkable route between the given start and dest positions
// and returns its waypoints, in travel order. Positions are in SUBTILE space:
// d2vector.Position stores subtiles, and NewPositionTile multiplies by five.
//
// Until M4.3a this was not a pathfinder. It was one call to checkLos, and it
// returned a ONE-ELEMENT slice: the destination when nothing blocked the
// straight line, and otherwise the last walkable point before the first
// blocker. Nothing routed around anything, which is why clicking across town
// stopped at the first fence and why nothing in the world could approach
// anything else. The repo had filed that as normal -- "'stuck' is a normal
// outcome, not an error".
//
// It now runs a bounded, deterministic A* (astar.go) and returns the corners
// of the route it finds. When the goal cannot be reached, within the map or
// within the expansion budget, it returns the best partial route toward it
// rather than nothing -- the old walk-as-far-as-you-can behaviour, kept
// deliberately as the failure mode.
func (m *MapEngine) PathFind(start, dest d2vector.Position) []d2vector.Position {
	from := subTile{int(math.Floor(start.X())), int(math.Floor(start.Y()))}
	goal := subTile{int(math.Floor(dest.X())), int(math.Floor(dest.Y()))}

	if from == goal {
		return []d2vector.Position{dest}
	}

	result := m.search(from, goal)

	steps := result.route(from)
	if len(steps) == 0 {
		return []d2vector.Position{}
	}

	return waypoints(from, steps, dest, result.exact)
}

// waypoints turns a subtile-by-subtile route into the corners of that route.
//
// The search works one subtile at a time, so a walk across town is hundreds of
// steps in a straight line. Keeping only the points where the direction
// changes gives the mover the same path with a fraction of the waypoints, and
// collapsing on an exact direction match keeps it deterministic. When the goal
// was actually reached the last waypoint is the caller's own dest, so the
// mover still lands exactly where it was asked to rather than on the centre of
// the nearest subtile.
func waypoints(from subTile, steps []subTile, dest d2vector.Position, exact bool) []d2vector.Position {
	points := make([]d2vector.Position, 0, len(steps))
	last := len(steps) - 1

	for i := range steps {
		if i < last {
			previous := from
			if i > 0 {
				previous = steps[i-1]
			}

			inX, inY := steps[i].x-previous.x, steps[i].y-previous.y
			outX, outY := steps[i+1].x-steps[i].x, steps[i+1].y-steps[i].y

			if inX == outX && inY == outY {
				continue // still travelling the same direction
			}
		}

		if i == last && exact {
			points = append(points, dest)

			continue
		}

		points = append(points, d2vector.NewPosition(
			float64(steps[i].x)+subTileCentre,
			float64(steps[i].y)+subTileCentre,
		))
	}

	return points
}

// checkLos finds out if there is a clear line of sight between two points.
//
// PathFind no longer uses it -- M4.3a replaced the raycast with a real search
// -- but line of sight is a different question from reachability, and M4.3b's
// awareness model is the intended caller: whether a thing can SEE you is not
// whether it can WALK to you. If M4.3b ends up not needing it, delete it
// rather than leaving an exported-looking helper with no caller, which is the
// shape of defect the M4.1 reopening was.
func (m *MapEngine) checkLos(start, end d2vector.Position) (bool, d2vector.Position) {
	dv := d2vector.Position{Vector: *end.Clone()}
	dv.Subtract(&start.Vector)
	dx := dv.X()
	dy := dv.Y()
	N := math.Max(math.Abs(dx), math.Abs(dy))

	var divN float64
	if N == 0 {
		divN = 0.0
	} else {
		divN = 1.0 / N // nolint:gomnd // we're just taking inverse...
	}

	xstep := dx * divN
	ystep := dy * divN
	x := start.X()
	y := start.Y()

	for i := 0; i <= int(N); i++ {
		x += xstep
		y += ystep

		// SubTileAt returns nil off the map, and off the map is not walkable:
		// the same answer as a wall, and the walk stops at the last good point
		// instead of dereferencing nil. Before SubTileAt was guarded this loop
		// crashed the process on a move target past the map edge, because it
		// does no bounds checking of its own.
		flags := m.SubTileAt(int(math.Floor(x)), int(math.Floor(y)))
		if flags == nil || flags.BlockWalk {
			return false, d2vector.NewPosition(x-xstep, y-ystep)
		}
	}

	return true, end
}
