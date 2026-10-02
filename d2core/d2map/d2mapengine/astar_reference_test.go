package d2mapengine

import (
	"container/heap"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// pathNode is one entry in the reference searches' open set.
type pathNode struct {
	x, y int
	g, h int
}

// nodeQueue is the reference searches' container/heap of *pathNode (the
// production searches' was until BUG-115; they now use openSet, the same
// order on values).
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

// blockedAtReference is blockedAt as it stood before BUG-115: through
// SubTileAt and GetSubTileFlags' lookup table.
func (m *MapEngine) blockedAtReference(x, y int) bool {
	flags := m.SubTileAt(x, y)

	return flags == nil || flags.BlockWalk
}

// THE REFERENCE SEARCH (2 Oct 2026). searchReference is search() exactly as it
// stood before the per-frame A* budget's burst made it cheaper -- two maps
// keyed by the subTile struct, container/heap over *pathNode -- kept here so
// the cheaper search can be held to it route for route. Every entity position
// is inside the state digest, so "faster" is only allowed to mean "the same
// answer, sooner".
type referenceResult struct {
	cameFrom  map[subTile]subTile
	reached   subTile
	exact     bool
	expanded  int
	exhausted bool
}

func (m *MapEngine) searchReference(start, goal subTile) referenceResult {
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

		if cost, seen := bestCost[here]; seen && current.g > cost {
			continue
		}

		if here == goal {
			return referenceResult{cameFrom: cameFrom, reached: here, exact: true, expanded: expanded}
		}

		if current.h < closestH {
			closest, closestH = here, current.h
		}

		expanded++

		for _, offset := range neighbourOffsets {
			next := subTile{current.x + offset.x, current.y + offset.y}

			if m.blockedAtReference(next.x, next.y) {
				continue
			}

			step := costOrthogonal

			if offset.x != 0 && offset.y != 0 {
				if m.blockedAtReference(current.x+offset.x, current.y) ||
					m.blockedAtReference(current.x, current.y+offset.y) {
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

			heap.Push(open, &pathNode{x: next.x, y: next.y, g: cost, h: octileDistance(next.x, next.y, goal.x, goal.y)})
		}
	}

	return referenceResult{cameFrom: cameFrom, reached: closest, exact: false, expanded: expanded, exhausted: open.Len() > 0}
}

func (r referenceResult) route(start subTile) []subTile {
	if r.reached == start {
		return nil
	}

	reversed := make([]subTile, 0, 16)

	for node := r.reached; node != start; {
		reversed = append(reversed, node)

		prev, ok := r.cameFrom[node]
		if !ok {
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

// referenceMaze is a 40x40-tile map (200x200 subtiles) with random walls --
// long runs both ways and scattered blocks -- so searches meet corners, dead
// ends, sealed pockets and the expansion budget.
func referenceMaze(seed int64) *MapEngine {
	m := testEngine(40, 40)
	rng := rand.New(rand.NewSource(seed)) // nolint:gosec // a test's walls

	for w := 0; w < 40; w++ {
		x, y := rng.Intn(200), rng.Intn(200)
		dx, dy := 1, 0

		if rng.Intn(2) == 0 {
			dx, dy = 0, 1
		}

		for k := 0; k < 10+rng.Intn(120); k++ {
			if x+k*dx < 200 && y+k*dy < 200 {
				block(m, x+k*dx, y+k*dy)
			}
		}
	}

	for k := 0; k < 1500; k++ {
		block(m, rng.Intn(200), rng.Intn(200))
	}

	// Sealed pockets (referencePockets): a search that starts inside one
	// runs its open set dry.
	for _, p := range referencePockets {
		for k := 0; k <= 10; k++ {
			block(m, p.x+k, p.y)
			block(m, p.x+k, p.y+10)
			block(m, p.x, p.y+k)
			block(m, p.x+10, p.y+k)
		}
	}

	return m
}

// referencePockets are the corners of the maze's sealed 11x11 boxes.
var referencePockets = []subTile{{20, 20}, {120, 40}, {60, 150}, {170, 170}}

// The cheaper search is the reference search: on random mazes, from random
// starts to random goals (near, far, blocked, off the map), every field of the
// result and the route itself are identical. Control (a source mutation):
// openSet.less breaking f ties on x before y -- the same set of shortest
// routes, a different one chosen -- goes red.
func TestSearchMatchesTheReferenceSearch(t *testing.T) {
	pairs, exhausted, walled, exact := 0, 0, 0, 0

	for seed := int64(1); seed <= 6; seed++ {
		m := referenceMaze(seed)
		rng := rand.New(rand.NewSource(seed * 7919)) // nolint:gosec // a test's pairs

		for i := 0; i < 150; i++ {
			start := subTile{rng.Intn(200), rng.Intn(200)}

			var goal subTile

			if i%5 == 4 { // inside a sealed pocket
				p := referencePockets[rng.Intn(len(referencePockets))]
				start = subTile{p.x + 1 + rng.Intn(9), p.y + 1 + rng.Intn(9)}

				if m.blockedAt(start.x, start.y) {
					start = subTile{p.x + 5, p.y + 5}
				}
			}

			switch i % 3 {
			case 0: // near
				goal = subTile{start.x + rng.Intn(41) - 20, start.y + rng.Intn(41) - 20}
			case 1: // anywhere
				goal = subTile{rng.Intn(200), rng.Intn(200)}
			default: // anywhere, off the map now and then
				goal = subTile{rng.Intn(220) - 10, rng.Intn(220) - 10}
			}

			got, want := m.search(start, goal), m.searchReference(start, goal)

			require.Equal(t, want.reached, got.reached, "seed %d pair %d: %v -> %v reached", seed, i, start, goal)
			require.Equal(t, want.exact, got.exact, "seed %d pair %d exact", seed, i)
			require.Equal(t, want.expanded, got.expanded, "seed %d pair %d expanded", seed, i)
			require.Equal(t, want.exhausted, got.exhausted, "seed %d pair %d exhausted", seed, i)
			require.Equal(t, want.route(start), got.route(start), "seed %d pair %d: %v -> %v route", seed, i, start, goal)

			pairs++

			switch {
			case got.exact:
				exact++
			case got.exhausted:
				exhausted++
			default:
				walled++
			}
		}
	}

	// The fixture reaches every kind of answer, or it proves less than it says.
	t.Logf("%d pairs: %d exact, %d out of budget, %d walled off", pairs, exact, exhausted, walled)
	require.Positive(t, exact)
	require.Positive(t, exhausted)
	require.Positive(t, walled)
}

// The same on the corridor's long wall: the leg searches agree too.
func TestTheLongWayRoundMatchesTheReference(t *testing.T) {
	m := longWall()

	for _, pair := range [][2]subTile{
		{{100, 20}, {200, 20}},
		{{140, 270}, {160, 10}},
		{{10, 10}, {290, 290}},
	} {
		got, want := m.search(pair[0], pair[1]), m.searchReference(pair[0], pair[1])
		require.Equal(t, want.route(pair[0]), got.route(pair[0]), "%v", pair)
		require.Equal(t, want.expanded, got.expanded, "%v", pair)
	}
}

func BenchmarkSearchReferenceWalledOff(b *testing.B) {
	m := testEngine(60, 60)
	for y := 0; y < 300; y++ {
		block(m, 150, y)
	}

	for i := 0; i < b.N; i++ {
		m.searchReference(subTile{100, 20}, subTile{200, 20})
	}
}
