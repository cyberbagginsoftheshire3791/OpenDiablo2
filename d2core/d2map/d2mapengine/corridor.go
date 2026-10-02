package d2mapengine

// The corridor: a route too long for one bounded subtile search.
//
// search() caps itself at maxExpandedNodes subtiles -- about a 12x12-tile patch
// of open ground -- which is plenty for walking round a cart and nowhere near
// enough to walk round a village. On the authored village (M5.4) a goal just
// outside the west fence, seen from the green, is a 45-tile walk by way of the
// south gate, and the search spent its whole budget flooding the green and
// came back "unreachable". Anything that has to go the long way round a wall
// -- the night coming for the gate, the player walking out to the field --
// hit that ceiling.
//
// The fix is the usual one, and it is only ever reached AFTER the ordinary
// search has run out of budget, so every route that search already finds is
// found exactly as before (same nodes, same order, same waypoints):
//
//  1. A coarse A* over whole TILES finds the corridor: a tile is open when its
//     centre subtile is, and a step between neighbouring tiles is open only
//     when the subtiles on the straight line between their centres are -- so a
//     wall one subtile thick between two open tiles still stops it. Diagonal
//     steps need both L-shaped detours open (no corner cutting, the same rule
//     the fine search keeps).
//  2. The ordinary subtile search then walks that corridor a few tiles at a
//     time, each leg well inside its budget, and the legs are joined.
//
// It returns a route only if EVERY leg reached its goal exactly. Anything less
// -- no corridor, or a leg the coarse grid misjudged -- and PathFind keeps the
// ordinary search's best partial route, which is today's behaviour. So the
// corridor can find routes that were not found before; it cannot make a route
// that was found worse, and it cannot make a failure different.
//
// Deterministic for the same reasons as search(): a total queue order, a fixed
// neighbour order, no map iteration.
//
// KNOWN LIMIT, pinned by TestAnOffCentreGapIsNotACorridor: the coarse grid
// sees a gap only where it crosses the straight line between two tile
// centres. A doorway a subtile or two wide at a tile's edge -- common in
// Diablo II's own DT1 walls, impossible in an authored map, which blocks
// whole tiles -- is invisible to it, and the corridor then fails safe: the
// ordinary partial route comes back unchanged.
//
// COST, measured on the laptop (BenchmarkPathFind*, 23 Sep 2026): a 56-tile
// wall walked round in about 7 ms; a search toward a goal sealed behind a
// complete wall about 7 ms, against 3.8 ms for the ordinary search alone. The
// bounds below keep the worst case finite: the coarse search, and the fine
// legs' total expansions.
const (
	// maxCoarseExpanded bounds the tile search. [DIAL] -- 3,000 tiles is a
	// 55x55 patch, room for a detour round a village-sized obstacle; an
	// enclosed goal costs at most this many tile steps.
	maxCoarseExpanded = 3000

	// corridorLeg is how many corridor tiles one fine leg covers. [DIAL] -- 6
	// tiles is 30 subtiles, a leg the fine search finishes in a few hundred
	// expansions on open ground.
	corridorLeg = 6

	// maxCorridorExpanded bounds the legs together. [DIAL] -- five ordinary
	// budgets; a corridor whose legs cost more than that is abandoned.
	maxCorridorExpanded = 5 * maxExpandedNodes

	tileCentreOffset = subtilesPerTile / 2
)

// tileCentre is the centre subtile of a tile.
func tileCentre(t subTile) subTile {
	return subTile{t.x*subtilesPerTile + tileCentreOffset, t.y*subtilesPerTile + tileCentreOffset}
}

// tileOf is the tile a subtile lies in. Coordinates here are never negative:
// a negative subtile is off the map and never reaches the corridor.
func tileOf(s subTile) subTile {
	return subTile{s.x / subtilesPerTile, s.y / subtilesPerTile}
}

// lineOpen reports whether every subtile from a's centre to b's centre,
// excluding a's own and including b's unless skipLast, is walkable. a and b
// are orthogonal neighbours.
func (m *MapEngine) lineOpen(a, b subTile, skipLast bool) bool {
	from, to := tileCentre(a), tileCentre(b)
	dx, dy := sign(to.x-from.x), sign(to.y-from.y)

	for i := 1; i <= subtilesPerTile; i++ {
		if skipLast && i == subtilesPerTile {
			break
		}

		if m.blockedAt(from.x+dx*i, from.y+dy*i) {
			return false
		}
	}

	return true
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}

	return 0
}

// coarseStep reports whether the corridor may step from tile a to its
// neighbour b. goal is the corridor's target tile, whose own centre is allowed
// to be blocked: the fine search goes to the real goal subtile, not the centre.
func (m *MapEngine) coarseStep(a, b, goal subTile) bool {
	if a.x == b.x || a.y == b.y {
		return m.lineOpen(a, b, b == goal)
	}

	// Diagonal: both L-shaped detours through the orthogonal neighbours.
	for _, via := range [2]subTile{{b.x, a.y}, {a.x, b.y}} {
		if !m.lineOpen(a, via, false) || !m.lineOpen(via, b, b == goal) {
			return false
		}
	}

	return true
}

// tileEdges is one tile's cached lines (searchScratch.edges).
type tileEdges struct {
	stamp uint32
	known uint8
	open  uint8
}

// edgeOf is the direction bit of an orthogonal step: E, W, S, N.
func edgeOf(dx, dy int) uint8 {
	switch {
	case dx > 0:
		return 1
	case dx < 0:
		return 2
	case dy > 0:
		return 4
	}

	return 8
}

// lineOpen is MapEngine.lineOpen remembered for the corridor search in hand
// (BUG-115): a diagonal step walks four lines and every tile is asked about by
// up to eight neighbours, so the same line was walked again and again -- the
// tile search's whole cost. The line from a to b is a pure function of the
// map, so remembering it within one search changes no corridor. The goal's
// own line (skipLast) is walked fresh, as it differs.
func (s *searchScratch) lineOpen(m *MapEngine, a, b subTile, skipLast bool) bool {
	if skipLast {
		return m.lineOpen(a, b, true)
	}

	e := &s.edges[a.y*s.tileWidth+a.x]
	if e.stamp != s.tileStamp {
		*e = tileEdges{stamp: s.tileStamp}
	}

	bit := edgeOf(b.x-a.x, b.y-a.y)
	if e.known&bit == 0 {
		e.known |= bit

		if m.lineOpen(a, b, false) {
			e.open |= bit
		}
	}

	return e.open&bit != 0
}

// coarseStep is MapEngine.coarseStep over the remembered lines.
func (s *searchScratch) coarseStep(m *MapEngine, a, b, goal subTile) bool {
	if a.x == b.x || a.y == b.y {
		return s.lineOpen(m, a, b, b == goal)
	}

	for _, via := range [2]subTile{{b.x, a.y}, {a.x, b.y}} {
		if !s.lineOpen(m, a, via, false) || !s.lineOpen(m, via, b, b == goal) {
			return false
		}
	}

	return true
}

// coarsePath is the tile-level A*: the corridor's tiles from start to goal
// inclusive, or nil when there is none within the budget.
func (m *MapEngine) coarsePath(start, goal subTile) []subTile {
	s := m.acquireScratch()
	defer m.releaseScratch(s)

	return s.coarsePath(m, start, goal)
}

// coarsePath is the tile-level A* in the scratch's tile grid (BUG-115: it kept
// two maps keyed by the subTile struct and a container/heap of pointers; the
// same search, the same total order, so the same corridor --
// TestCoarsePathMatchesTheReference).
func (s *searchScratch) coarsePath(m *MapEngine, start, goal subTile) []subTile {
	if m.tileCoordinateToIndex(goal.x, goal.y) < 0 || m.tileCoordinateToIndex(start.x, start.y) < 0 {
		return nil
	}

	s.beginTiles()
	s.coarseRuns++

	stamp, width := s.tileStamp, s.tileWidth
	s.tileCells[start.y*width+start.x] = searchCell{stamp: stamp, cost: 0, step: noStep}

	open := s.open[:0]
	open.push(node(start.x, start.y, 0, octileDistance(start.x, start.y, goal.x, goal.y)))

	defer func() { s.open = open }()

	for expanded := 0; len(open) > 0 && expanded < maxCoarseExpanded; {
		current := open.pop()

		here := subTile{int(current.x), int(current.y)}
		g := int(current.g)

		if c := s.tileCells[here.y*width+here.x]; c.stamp == stamp && g > int(c.cost) {
			continue
		}

		if here == goal {
			return s.chain(start, goal)
		}

		expanded++

		for k, offset := range neighbourOffsets {
			next := subTile{here.x + offset.x, here.y + offset.y}

			if m.tileCoordinateToIndex(next.x, next.y) < 0 {
				continue
			}

			if !s.coarseStep(m, here, next, goal) {
				continue
			}

			step := costOrthogonal
			if offset.x != 0 && offset.y != 0 {
				step = costDiagonal
			}

			cost := g + step

			c := &s.tileCells[next.y*width+next.x]
			if c.stamp == stamp && cost >= int(c.cost) {
				continue
			}

			*c = searchCell{stamp: stamp, cost: int32(cost), step: uint8(k)}

			open.push(node(next.x, next.y, cost, octileDistance(next.x, next.y, goal.x, goal.y)))
		}
	}

	return nil
}

// chain walks the tile grid's steps back from goal to start and returns
// start..goal.
func (s *searchScratch) chain(start, goal subTile) []subTile {
	if start == goal {
		return []subTile{goal}
	}

	tail := s.walk(s.tileCells, s.tileStamp, s.tileWidth, start, goal)
	if tail == nil {
		return nil
	}

	return append([]subTile{start}, tail...)
}

// corridorRoute is the long way round: a coarse corridor, walked leg by leg
// with the ordinary search. It returns the joined subtile steps (the first
// step after from through goal) and true only when every leg arrived.
func (m *MapEngine) corridorRoute(from, goal subTile) ([]subTile, bool) {
	s := m.acquireScratch()
	defer m.releaseScratch(s)

	steps, ok, _ := s.corridorRoute(m, from, goal)

	return steps, ok
}

// corridorRoute is corridorRoute in the scratch, also counting the leg
// searches it ran.
func (s *searchScratch) corridorRoute(m *MapEngine, from, goal subTile) (steps []subTile, ok bool, legs int) {
	// Off the map on the negative side: integer division would fold -3 onto
	// tile 0, so refuse here rather than route to the wrong tile.
	if from.x < 0 || from.y < 0 || goal.x < 0 || goal.y < 0 {
		return nil, false, 0
	}

	// A blocked goal can never be reached exactly; the last leg would only
	// burn its budget finding that out again.
	if m.blockedAt(goal.x, goal.y) {
		return nil, false, 0
	}

	tiles := s.coarsePath(m, tileOf(from), tileOf(goal))

	// A corridor of one leg or less is a walk the ordinary search would have
	// finished well inside its budget had the fine route existed -- its only
	// leg would BE the search that just failed. Nothing to gain.
	if len(tiles)-1 <= corridorLeg {
		return nil, false, 0
	}

	spent := 0

	here, last := from, len(tiles)-1

	for at := 0; at < last; {
		next := at + corridorLeg
		if next > last {
			next = last
		}

		target := tileCentre(tiles[next])
		if next == last {
			target = goal
		}

		leg := s.search(m, here, target)
		legs++

		spent += leg.expanded
		if !leg.exact || spent > maxCorridorExpanded {
			return nil, false, legs
		}

		steps = append(steps, leg.steps...)
		here, at = target, next
	}

	return steps, len(steps) > 0, legs
}
