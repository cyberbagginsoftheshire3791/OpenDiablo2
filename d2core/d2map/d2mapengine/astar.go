package d2mapengine

import (
	"container/heap"
)

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

// pathNode is one entry in the open set.
type pathNode struct {
	x, y int
	g, h int
}

// nodeQueue is a min-heap of pathNodes.
type nodeQueue []*pathNode

func (q nodeQueue) Len() int { return len(q) }

// Less is a TOTAL order, and that is the point. Ordering on f alone leaves
// equal-f nodes to be separated by whatever the heap happens to do with them,
// which is stable within a process but not something to rely on across builds.
// Falling through f -> h -> y -> x leaves no ties at all: two distinct nodes
// can never compare equal, because no two share a coordinate pair.
func (q nodeQueue) Less(i, j int) bool {
	a, b := q[i], q[j]

	if af, bf := a.g+a.h, b.g+b.h; af != bf {
		return af < bf
	}

	if a.h != b.h {
		return a.h < b.h
	}

	if a.y != b.y {
		return a.y < b.y
	}

	return a.x < b.x
}

func (q nodeQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *nodeQueue) Push(x interface{}) { *q = append(*q, x.(*pathNode)) }

func (q *nodeQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]

	return item
}

// blockedAt reports whether a subtile cannot be walked. Off the map counts as
// blocked: SubTileAt returns nil there, and the map edge is as impassable as a
// wall.
func (m *MapEngine) blockedAt(x, y int) bool {
	flags := m.SubTileAt(x, y)

	return flags == nil || flags.BlockWalk
}

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

// searchResult is what one A* run produces: the chain of came-from links, the
// node the search actually reached, and whether that node is the goal.
type searchResult struct {
	cameFrom map[subTile]subTile
	reached  subTile
	exact    bool
}

// search runs the bounded A* and returns the route it found, or the closest
// approach it managed within the expansion budget.
func (m *MapEngine) search(start, goal subTile) searchResult {
	cameFrom := make(map[subTile]subTile)
	bestCost := map[subTile]int{start: 0}

	startH := octileDistance(start.x, start.y, goal.x, goal.y)

	open := &nodeQueue{{x: start.x, y: start.y, g: 0, h: startH}}
	heap.Init(open)

	closest, closestH := start, startH
	expanded := 0

	for open.Len() > 0 && expanded < maxExpandedNodes {
		current, ok := heap.Pop(open).(*pathNode)
		if !ok {
			break
		}

		here := subTile{current.x, current.y}

		// A cheaper route to this node was queued after this entry was; the
		// stale entry is skipped rather than removed, which is the usual way
		// to avoid a decrease-key operation.
		if cost, seen := bestCost[here]; seen && current.g > cost {
			continue
		}

		if here == goal {
			return searchResult{cameFrom: cameFrom, reached: here, exact: true}
		}

		if current.h < closestH {
			closest, closestH = here, current.h
		}

		expanded++

		for _, offset := range neighbourOffsets {
			next := subTile{current.x + offset.x, current.y + offset.y}

			if m.blockedAt(next.x, next.y) {
				continue
			}

			step := costOrthogonal

			if offset.x != 0 && offset.y != 0 {
				// No corner cutting: a diagonal needs both of its orthogonal
				// neighbours open, or the step clips the corner of a wall and
				// the mover walks through geometry it should have gone around.
				if m.blockedAt(current.x+offset.x, current.y) ||
					m.blockedAt(current.x, current.y+offset.y) {
					continue
				}

				step = costDiagonal
			}

			cost := current.g + step
			if prev, seen := bestCost[next]; seen && cost >= prev {
				continue
			}

			bestCost[next] = cost
			cameFrom[next] = here

			heap.Push(open, &pathNode{
				x: next.x,
				y: next.y,
				g: cost,
				h: octileDistance(next.x, next.y, goal.x, goal.y),
			})
		}
	}

	// Either the budget ran out or the goal is walled off. Head for the
	// closest approach instead of refusing to move.
	return searchResult{cameFrom: cameFrom, reached: closest, exact: false}
}

// route walks the came-from chain back from the reached node and returns the
// subtiles from the first step after start through to it, in travel order.
func (r searchResult) route(start subTile) []subTile {
	if r.reached == start {
		return nil
	}

	reversed := make([]subTile, 0, 16)

	for node := r.reached; node != start; {
		reversed = append(reversed, node)

		prev, ok := r.cameFrom[node]
		if !ok {
			// No chain back to the start. Nothing honest to return.
			return nil
		}

		node = prev
	}

	forward := make([]subTile, len(reversed))
	for i, node := range reversed {
		forward[len(reversed)-1-i] = node
	}

	return forward
}
