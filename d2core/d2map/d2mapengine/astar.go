package d2mapengine

// A* over the subtile grid. This is the search behind PathFind (M4.3a); the
// thing that follows the list it produces -- mapEntity.SetPath, Step,
// nextPath -- is untouched.
//
// Three properties this has to hold, in the order they matter:
//
//  1. DETERMINISM. Every entity position is inside the harness state digest,
//     so two launches of one build must produce a byte-identical path for the
//     same start and goal. The queue therefore orders on a TOTAL key and the
//     search never iterates a map: Go randomises map order, and one such loop
//     anywhere in here would make the digest drift for no visible reason.
//  2. BOUNDEDNESS. An unbounded search over a 150x150 map at subtile
//     resolution is 562,500 cells and a frame-time hazard. Expansion is capped,
//     and on exhaustion the search returns the best partial route toward the
//     goal rather than nothing -- so failure degrades to "walk as far as you
//     can", which is what the raycast it replaces effectively did.
//  3. SAFETY. Every read goes through SubTileAt, which returns nil off the map;
//     off the map is treated as blocked. Before M4.3a fixed those accessors, a
//     search that stepped past the map edge crashed the process.
const (
	// Integer step costs, the classic octile approximation of 1 and sqrt(2).
	// Integers rather than floats on purpose: f, g and h then compare exactly,
	// so the ordering below cannot drift with floating-point rounding on a
	// different machine, and neither can the digest.
	costOrthogonal = 10
	costDiagonal   = 14

	// maxExpandedNodes bounds one search. [DIAL] -- roughly 160 world tiles of
	// area, which covers any journey inside the current 150x150 map with room
	// to spare while keeping the worst frame bounded.
	maxExpandedNodes = 4000

	// subTileCentre places a waypoint in the middle of its subtile rather than
	// on a boundary, so a mover never sits exactly on the edge between two.
	subTileCentre = 0.5
)

// subTile is a whole-number position on the subtile grid.
type subTile struct {
	x, y int
}

// neighbourOffsets is walked in a FIXED compass order, N first, clockwise.
// The order is part of the determinism contract: equal-cost routes must be
// discovered in the same sequence on every run.
var neighbourOffsets = [8]subTile{
	{0, -1},  // N
	{1, -1},  // NE
	{1, 0},   // E
	{1, 1},   // SE
	{0, 1},   // S
	{-1, 1},  // SW
	{-1, 0},  // W
	{-1, -1}, // NW
}

// blockedAt reports whether a subtile cannot be walked. Off the map counts as
// blocked: there is no tile there, and the map edge is as impassable as a
// wall.
//
// It is SubTileAt(x, y).BlockWalk with the steps written out (BUG-115): the
// tile by floored division, refused off the map exactly as TileAt refuses it,
// and the subtile's slot as GetSubTileFlags' lookup table maps it -- row y of
// the table is the slots 20-5y .. 24-5y. A search asks this up to 24 times a
// node, and the call chain through SubTileAt, TileAt and the table was about
// 15% of a search. TestBlockedAtIsSubTileAt holds it to SubTileAt cell for
// cell, on and off the map.
func (m *MapEngine) blockedAt(x, y int) bool {
	tileX, offsetX := floorDivMod(x, subtilesPerTile)
	tileY, offsetY := floorDivMod(y, subtilesPerTile)

	if tileX < 0 || tileX >= m.size.Width || tileY < 0 || tileY >= m.size.Height {
		return true
	}

	idx := tileX + tileY*m.size.Width
	if idx >= len(m.tiles) {
		return true
	}

	return m.tiles[idx].SubTiles[(subtilesPerTile-1-offsetY)*subtilesPerTile+offsetX].BlockWalk
}

// BlockedAt reports whether a subtile cannot be walked, in the same terms the A*
// uses. Exported so mapRouter.Route (d2game/d2gamescreen) can skip a neighbour
// candidate tile that is itself blocked BEFORE paying for a full search toward
// it: a blocked goal never enters the open set, so an unguarded routeExact there
// expands to the whole budget and returns nothing usable (audit B3, 12 Sep 2026).
func (m *MapEngine) BlockedAt(x, y int) bool { return m.blockedAt(x, y) }

// octileDistance is the cost of an unobstructed 8-way walk between two
// subtiles. It never overestimates -- a real route cannot beat a straight one
// -- so it is admissible and the paths this search returns are shortest under
// the step costs above.
func octileDistance(x, y, goalX, goalY int) int {
	dx, dy := abs(x-goalX), abs(y-goalY)
	if dx < dy {
		dx, dy = dy, dx
	}

	return costOrthogonal*(dx-dy) + costDiagonal*dy
}

func abs(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

// openNode is one entry of the open set, as a value: f (g + h) kept so the
// order needs no addition, and 32-bit fields so an entry is 20 bytes, not a
// pathNode's 32 (BUG-115).
type openNode struct {
	f, h, x, y, g int32
}

// openSet is the search's min-heap of openNode VALUES, ordered by less.
type openSet []openNode

// less is nodeQueue.Less on values: f, then h, then y, then x -- a total order
// over the entries the search ever holds at once (two entries for one subtile
// differ in g, so in f).
func (q openSet) less(i, j int) bool {
	a, b := &q[i], &q[j]

	if a.f != b.f {
		return a.f < b.f
	}

	if a.h != b.h {
		return a.h < b.h
	}

	if a.y != b.y {
		return a.y < b.y
	}

	return a.x < b.x
}

// node is an open-set entry for subtile (x, y) at cost g with heuristic h.
func node(x, y, g, h int) openNode {
	return openNode{f: int32(g + h), h: int32(h), x: int32(x), y: int32(y), g: int32(g)}
}

// The open set is a 4-ary heap (BUG-115): half the levels of a binary one,
// and a node's four children share a cache line or two. The order is total,
// so any correct heap pops the same sequence.
const heapArity = 4

func (q *openSet) push(n openNode) {
	*q = append(*q, n)
	h := *q

	for i := len(h) - 1; i > 0; {
		parent := (i - 1) / heapArity
		if !h.less(i, parent) {
			break
		}

		h[i], h[parent] = h[parent], h[i]
		i = parent
	}
}

func (q *openSet) pop() openNode {
	h := *q
	top := h[0]
	last := len(h) - 1
	h[0] = h[last]
	h = h[:last]

	for i := 0; ; {
		first := heapArity*i + 1
		if first >= len(h) {
			break
		}

		c := first

		end := first + heapArity
		if end > len(h) {
			end = len(h)
		}

		for k := first + 1; k < end; k++ {
			if h.less(k, c) {
				c = k
			}
		}

		if !h.less(c, i) {
			break
		}

		h[i], h[c] = h[c], h[i]
		i = c
	}

	*q = h

	return top
}

// searchResult is what one A* run produces: the route it found (or its
// closest approach), the node the search actually reached, and whether that
// node is the goal.
type searchResult struct {
	// steps is the route from the first step after the start through
	// reached, in travel order; nil when reached is the start. Walked out of
	// the scratch before the search returns (BUG-115), so a result stays
	// good after the scratch has moved on to another search.
	steps   []subTile
	reached subTile
	exact   bool
	// expanded is how many nodes this search popped and expanded. It is exposed
	// so a test can prove that routing toward a blocked/walled goal burns the
	// whole budget -- the cost mapRouter.Route's blocked-neighbour skip avoids
	// (audit B3, 12 Sep 2026).
	expanded int
	// exhausted is true when the search stopped because it hit the budget
	// with nodes still open -- the route may exist, it was too far to find.
	// False when the open set ran dry: the goal is walled off, and the
	// corridor (corridor.go) has nothing to add.
	exhausted bool
}

// search runs the bounded A* and returns the route it found, or the closest
// approach it managed within the expansion budget.
func (m *MapEngine) search(start, goal subTile) searchResult {
	s := m.acquireScratch()
	defer m.releaseScratch(s)

	return s.search(m, start, goal)
}

// search is the bounded A* in the scratch's grid (scratch.go). What it knows
// of a subtile -- the cheapest cost so far and the step that reached it -- is
// the cell at that subtile's index, stamped with this search; the open set is
// a heap of values ordered by openSet.less. The start's own cell is written
// when the start is on the grid; off it (a walker past the edge), nothing
// ever steps back onto it, because off the map is blocked.
func (s *searchScratch) search(m *MapEngine, start, goal subTile) searchResult {
	s.begin()

	if i, ok := s.index(start); ok {
		s.cells[i] = searchCell{stamp: s.stamp, cost: 0, step: noStep}
		s.visited = append(s.visited, int32(i))
	}

	startH := octileDistance(start.x, start.y, goal.x, goal.y)

	open := s.open[:0]
	open.push(node(start.x, start.y, 0, startH))

	closest, closestH := start, startH
	expanded := 0

	for len(open) > 0 && expanded < maxExpandedNodes {
		current := open.pop()

		here := subTile{int(current.x), int(current.y)}
		g := int(current.g)

		// A cheaper route to this node was queued after this entry was; the
		// stale entry is skipped rather than removed, which is the usual way
		// to avoid a decrease-key operation.
		if i, ok := s.index(here); ok && s.cells[i].stamp == s.stamp && g > int(s.cells[i].cost) {
			continue
		}

		if here == goal {
			s.open = open

			return searchResult{steps: s.walk(s.cells, s.stamp, s.width, start, here), reached: here, exact: true, expanded: expanded}
		}

		if h := int(current.h); h < closestH {
			closest, closestH = here, h
		}

		expanded++

		// Each neighbour's walk bit once (BUG-115): a diagonal's two
		// orthogonal neighbours, which the no-corner-cutting rule reads, are
		// the steps either side of it in the compass order.
		var walkable [8]bool
		for k, offset := range neighbourOffsets {
			walkable[k] = !m.blockedAt(here.x+offset.x, here.y+offset.y)
		}

		for k, offset := range neighbourOffsets {
			if !walkable[k] {
				continue
			}

			step := costOrthogonal

			if k&1 == 1 {
				// No corner cutting: a diagonal needs both of its orthogonal
				// neighbours open, or the step clips the corner of a wall and
				// the mover walks through geometry it should have gone around.
				if !walkable[k-1] || !walkable[(k+1)&7] {
					continue
				}

				step = costDiagonal
			}

			next := subTile{here.x + offset.x, here.y + offset.y}
			cost := g + step

			// Walkable, so on the map and on the grid.
			i := next.y*s.width + next.x
			c := &s.cells[i]

			if c.stamp == s.stamp {
				if cost >= int(c.cost) {
					continue
				}
			} else {
				s.visited = append(s.visited, int32(i))
			}

			*c = searchCell{stamp: s.stamp, cost: int32(cost), step: uint8(k)}

			open.push(node(next.x, next.y, cost, octileDistance(next.x, next.y, goal.x, goal.y)))
		}
	}

	s.open = open

	// Either the budget ran out or the goal is walled off. Head for the
	// closest approach instead of refusing to move.
	return searchResult{
		steps:     s.walk(s.cells, s.stamp, s.width, start, closest),
		reached:   closest,
		exact:     false,
		expanded:  expanded,
		exhausted: len(open) > 0,
	}
}

// route is the subtiles from the first step after start through to the node
// reached, in travel order (start is the search's own; kept for the tests'
// symmetry with the reference search).
func (r searchResult) route(_ subTile) []subTile {
	return r.steps
}
