package d2mapengine

import (
	"container/heap"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine/testdata/masterref"
)

// BUG-115's references. THE ANSWERS come from master 05ba3666's own
// d2mapengine, frozen verbatim in testdata/masterref (the BUG-115 review's
// copy: only the package line differs, plus export_rev.go's grid builder).
// It lives under testdata so the go tool's ./... patterns -- go build, go vet,
// the reach gate's deadcode runs -- never see it; only these tests import it.
// The copies below (coarsePathReference, corridorRouteReference, the counting
// half of pathFindReference) are kept to COUNT the old code's searches and
// corridors, which master does not report; pathFindReference checks the
// counting copy against master on every call.

// coarseReferenceRuns counts the reference's corridor tile searches.
var coarseReferenceRuns int

func (m *MapEngine) coarsePathReference(start, goal subTile) []subTile {
	coarseReferenceRuns++

	if m.tileCoordinateToIndex(goal.x, goal.y) < 0 || m.tileCoordinateToIndex(start.x, start.y) < 0 {
		return nil
	}

	cameFrom := make(map[subTile]subTile)
	bestCost := map[subTile]int{start: 0}

	open := &nodeQueue{{x: start.x, y: start.y, g: 0, h: octileDistance(start.x, start.y, goal.x, goal.y)}}
	heap.Init(open)

	for expanded := 0; open.Len() > 0 && expanded < maxCoarseExpanded; {
		current, ok := heap.Pop(open).(*pathNode)
		if !ok {
			return nil
		}

		here := subTile{current.x, current.y}

		if cost, seen := bestCost[here]; seen && current.g > cost {
			continue
		}

		if here == goal {
			return chainReference(cameFrom, start, goal)
		}

		expanded++

		for _, offset := range neighbourOffsets {
			next := subTile{here.x + offset.x, here.y + offset.y}

			if m.tileCoordinateToIndex(next.x, next.y) < 0 {
				continue
			}

			if !m.coarseStep(here, next, goal) {
				continue
			}

			step := costOrthogonal
			if offset.x != 0 && offset.y != 0 {
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

	return nil
}

func chainReference(cameFrom map[subTile]subTile, start, goal subTile) []subTile {
	reversed := []subTile{goal}

	for node := goal; node != start; {
		prev, ok := cameFrom[node]
		if !ok {
			return nil
		}

		reversed = append(reversed, prev)
		node = prev
	}

	out := make([]subTile, len(reversed))
	for i, node := range reversed {
		out[len(reversed)-1-i] = node
	}

	return out
}

// corridorRouteReference also counts its searches (the legs).
func (m *MapEngine) corridorRouteReference(from, goal subTile) ([]subTile, bool, int) {
	if from.x < 0 || from.y < 0 || goal.x < 0 || goal.y < 0 {
		return nil, false, 0
	}

	if m.blockedAtReference(goal.x, goal.y) {
		return nil, false, 0
	}

	tiles := m.coarsePathReference(tileOf(from), tileOf(goal))
	if len(tiles)-1 <= corridorLeg {
		return nil, false, 0
	}

	var steps []subTile

	spent, legs := 0, 0
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

		leg := m.searchReference(here, target)
		legs++

		spent += leg.expanded
		if !leg.exact || spent > maxCorridorExpanded {
			return nil, false, legs
		}

		steps = append(steps, leg.route(here)...)
		here, at = target, next
	}

	return steps, len(steps) > 0, legs
}

// masterOf is master's engine over the same walls as m: every subtile m
// blocks (read the old way, blockedAtReference) blocked, and the same tile
// slice length. Cached per engine; forgetMaster after editing m's walls.
func masterOf(m *MapEngine) *masterref.MapEngine {
	mastersMu.Lock()
	defer mastersMu.Unlock()

	if g, ok := masters[m]; ok {
		return g
	}

	w, h := m.size.Width, m.size.Height
	g := masterref.NewGrid(w, h)

	for y := 0; y < h*subtilesPerTile; y++ {
		for x := 0; x < w*subtilesPerTile; x++ {
			if m.blockedAtReference(x, y) {
				g.BlockSub(x, y)
			}
		}
	}

	if short := w*h - len(m.tiles); short > 0 {
		g.TruncateTiles(short)
	}

	masters[m] = g

	return g
}

func forgetMaster(m *MapEngine) {
	mastersMu.Lock()
	delete(masters, m)
	mastersMu.Unlock()
}

var (
	mastersMu sync.Mutex
	masters   = map[*MapEngine]*masterref.MapEngine{}
)

// pathFindReference is master's PathFind (the answer) and how many searches
// the old code ran for it (the counting copy, which must agree with master).
func (m *MapEngine) pathFindReference(start, dest d2vector.Position) ([]d2vector.Position, int) {
	want := masterOf(m).PathFind(start, dest)

	counted, searches := m.pathFindCounted(start, dest)
	if !reflect.DeepEqual(want, counted) {
		panic(fmt.Sprintf("the counting copy of the old PathFind is not master's: %v -> %v", start, dest))
	}

	return want, searches
}

// pathFindCounted is PathFind before BUG-115 (a copy), and how many searches
// it ran.
func (m *MapEngine) pathFindCounted(start, dest d2vector.Position) ([]d2vector.Position, int) {
	from := subTile{int(math.Floor(start.X())), int(math.Floor(start.Y()))}
	goal := subTile{int(math.Floor(dest.X())), int(math.Floor(dest.Y()))}

	if from == goal {
		return []d2vector.Position{dest}, 0
	}

	result := m.searchReference(from, goal)
	searches := 1

	steps := result.route(from)
	exact := result.exact

	if !exact && result.exhausted {
		long, ok, legs := m.corridorRouteReference(from, goal)
		searches += legs

		if ok {
			steps, exact = long, true
		}
	}

	if len(steps) == 0 {
		return []d2vector.Position{}, searches
	}

	return waypoints(from, steps, dest, exact), searches
}

// masterCoarse is master's coarsePath in subTiles.
func masterCoarse(g *masterref.MapEngine, a, b subTile) []subTile {
	p := g.CoarsePathRef(a.x, a.y, b.x, b.y)
	if p == nil {
		return nil
	}

	out := make([]subTile, len(p))
	for i, t := range p {
		out[i] = subTile{t[0], t[1]}
	}

	return out
}

// endsIn is mapRouter.routeExact's "reachable": the route's last waypoint is
// in tile (tx, ty).
func endsIn(path []d2vector.Position, tx, ty float64) bool {
	if len(path) == 0 {
		return false
	}

	last := path[len(path)-1]
	w := last.World()

	return math.Floor(w.X()) == math.Floor(tx) && math.Floor(w.Y()) == math.Floor(ty)
}

// besideOffsets is mapRouter's neighbour table (game.go), in its fixed order.
var besideOffsets = [8][2]float64{
	{0, -1}, {1, -1}, {1, 0}, {1, 1},
	{0, 1}, {-1, 1}, {-1, 0}, {-1, -1},
}

// routeBeside is mapRouter.Route's loop over a RouteQuery (the order here is
// the table's rather than nearest-first: the proofs do not depend on it), and
// routeBesideReference is the same loop over pathFindReference. Both return
// the route, whether it reached, and the searches spent.
func (m *MapEngine) routeBeside(hx, hy, qx, qy float64) ([]d2vector.Position, bool, int) {
	q := m.NewRouteQuery(d2vector.NewPositionTile(hx, hy))
	defer q.Close()

	for _, n := range besideOffsets {
		gx, gy := qx+n[0], qy+n[1]
		if m.blockedAt(int(math.Floor(gx*5)), int(math.Floor(gy*5))) {
			continue
		}

		dest := d2vector.NewPositionTile(gx, gy)
		if q.ProvenUnreachable(dest, int(math.Floor(gx)), int(math.Floor(gy))) {
			continue
		}

		if path := q.PathFind(dest); endsIn(path, gx, gy) {
			return path, true, q.Searches()
		}
	}

	path := q.PathFind(d2vector.NewPositionTile(qx, qy))

	return path, endsIn(path, qx, qy), q.Searches()
}

func (m *MapEngine) routeBesideReference(hx, hy, qx, qy float64) ([]d2vector.Position, bool, int) {
	from := d2vector.NewPositionTile(hx, hy)
	searches := 0

	for _, n := range besideOffsets {
		gx, gy := qx+n[0], qy+n[1]
		if masterOf(m).BlockedAt(int(math.Floor(gx*5)), int(math.Floor(gy*5))) {
			continue
		}

		path, s := m.pathFindReference(from, d2vector.NewPositionTile(gx, gy))
		searches += s

		if endsIn(path, gx, gy) {
			return path, true, searches
		}
	}

	path, s := m.pathFindReference(from, d2vector.NewPositionTile(qx, qy))

	return path, endsIn(path, qx, qy), searches + s
}

// blockedAt is SubTileAt's answer for every subtile on the map and a margin
// off it -- including a map whose tile slice is shorter than its size says.
// Control (a source mutation): the slot as offsetY*5+offsetX, the table read
// upside down, goes red at the first wall.
func TestBlockedAtIsSubTileAt(t *testing.T) {
	short := referenceMaze(2)
	short.tiles = short.tiles[:len(short.tiles)-45] // the last rows missing

	for _, m := range []*MapEngine{referenceMaze(1), short} {
		walls := 0

		for y := -12; y < 212; y++ {
			for x := -12; x < 212; x++ {
				want := m.blockedAtReference(x, y)
				require.Equal(t, want, m.blockedAt(x, y), "(%d,%d)", x, y)

				if want && x >= 0 && y >= 0 && x < 200 && y < 200 {
					walls++
				}
			}
		}

		require.Positive(t, walls, "the fixture has walls to get wrong")
	}
}

// The corridor's tile search is the reference's, corridor for corridor, on
// the mazes and the long wall, one scratch reused across every pair.
func TestCoarsePathMatchesTheReference(t *testing.T) {
	found := 0

	for seed := int64(1); seed <= 6; seed++ {
		m := referenceMaze(seed)
		rng := rand.New(rand.NewSource(seed * 104729)) // nolint:gosec // a test's pairs

		for i := 0; i < 120; i++ {
			a := subTile{rng.Intn(44) - 2, rng.Intn(44) - 2}
			b := subTile{rng.Intn(44) - 2, rng.Intn(44) - 2}

			want := m.coarsePathReference(a, b)
			require.Equal(t, want, m.coarsePath(a, b), "seed %d pair %d: %v -> %v", seed, i, a, b)
			require.Equal(t, masterCoarse(masterOf(m), a, b), want, "the copy is master's")

			if want != nil {
				found++
			}
		}
	}

	w := longWall()
	for _, pair := range [][2]subTile{{{20, 4}, {40, 4}}, {{28, 54}, {32, 2}}, {{2, 2}, {58, 58}}} {
		require.Equal(t, w.coarsePathReference(pair[0], pair[1]), w.coarsePath(pair[0], pair[1]), "%v", pair)
	}

	t.Logf("%d corridors found", found)
	require.Positive(t, found)
}

// THE ROUTE QUERY IS THE OLD ROUTER. On the mazes, from hunters anywhere and
// inside the sealed pockets to quarries anywhere and inside them, the Route
// loop over a RouteQuery returns the same route and the same reachable as the
// loop over the old PathFind -- and every PathFind it asks, on its own, is the
// old PathFind's answer. The proofs fire (some candidates skipped, some
// corridors spared) and save searches. Controls (source mutations): a flood
// that calls a region sealed without checking the start's first steps, and
// learn marking a search's visited cells sealed rather than reachable, both
// go red.
func TestARouteQueryAnswersAsTheReferenceDoes(t *testing.T) {
	pairs, reached, saved, gotSearches, refSearches := 0, 0, 0, 0, 0

	for seed := int64(1); seed <= 6; seed++ {
		m := referenceMaze(seed)
		rng := rand.New(rand.NewSource(seed * 6151)) // nolint:gosec // a test's pairs

		pocket := func() (float64, float64) {
			p := referencePockets[rng.Intn(len(referencePockets))]
			return (float64(p.x) + 1 + rng.Float64()*9) / 5, (float64(p.y) + 1 + rng.Float64()*9) / 5
		}

		for i := 0; i < 60; i++ {
			hx, hy := rng.Float64()*40, rng.Float64()*40
			qx, qy := rng.Float64()*40, rng.Float64()*40

			switch i % 4 {
			case 1:
				qx, qy = pocket() // a quarry in a sealed yard
			case 2:
				hx, hy = pocket() // a hunter in one
			case 3:
				qx, qy = hx+rng.Float64()*8-4, hy+rng.Float64()*8-4 // near
			}

			want, wantOK, ws := m.routeBesideReference(hx, hy, qx, qy)
			got, gotOK, gs := m.routeBeside(hx, hy, qx, qy)

			require.Equal(t, wantOK, gotOK, "seed %d pair %d reachable", seed, i)
			require.Equal(t, want, got, "seed %d pair %d: (%.2f,%.2f) -> (%.2f,%.2f) route", seed, i, hx, hy, qx, qy)
			require.LessOrEqual(t, gs, ws, "seed %d pair %d: never more searches", seed, i)

			// Each question on its own, in a fresh query: the old answer.
			for _, n := range besideOffsets {
				dest := d2vector.NewPositionTile(qx+n[0], qy+n[1])
				refPath, _ := m.pathFindReference(d2vector.NewPositionTile(hx, hy), dest)
				require.Equal(t, refPath, m.PathFind(d2vector.NewPositionTile(hx, hy), dest), "seed %d pair %d PathFind", seed, i)
			}

			pairs++
			gotSearches += gs
			refSearches += ws

			if gotOK {
				reached++
			}

			if gs < ws {
				saved++
			}
		}
	}

	t.Logf("%d routes (%d reached); searches %d, the reference %d; %d routes spent fewer", pairs, reached, gotSearches, refSearches, saved)
	require.Positive(t, reached)
	require.Less(t, reached, pairs, "some quarries are out of reach")
	require.Positive(t, saved, "the proofs fire")
}

// sealedYard is a 60x60-tile open map with a 4x4-tile yard walled in at
// tiles 30-33 (its wall one subtile thick, on the yard's outer ring).
func sealedYard() *MapEngine {
	m := testEngine(60, 60)

	for k := 149; k <= 170; k++ {
		block(m, k, 149)
		block(m, k, 170)
		block(m, 149, k)
		block(m, 170, k)
	}

	return m
}

// THE NINE SEARCHES, MEASURED (the budget review's D). A hunter far out on
// the open map and a quarry in a sealed yard: the old loop paid nine searches
// to the cap and nine corridors; the query pays the first candidate's search
// (whose flood proves the yard sealed and spares its corridor), skips the
// other seven, and pays the quarry's own search for the partial route the
// hunter walks. And a hunter sealed IN the yard with his quarry outside: the
// first search runs dry and proves everything else. Both with the identical
// route. Control (a source mutation): ProvenUnreachable always false -- the
// counts go back to the reference's and the test is red.
func TestASealedYardCostsTwoSearchesNotNine(t *testing.T) {
	m := sealedYard()

	for _, c := range []struct {
		name           string
		hx, hy, qx, qy float64
	}{
		{"quarry in the yard", 10.5, 10.5, 32.5, 31.5},
		{"hunter in the yard", 31.5, 31.5, 50.5, 12.5},
	} {
		coarseReferenceRuns = 0
		want, wantOK, ws := m.routeBesideReference(c.hx, c.hy, c.qx, c.qy)
		wantCoarse := coarseReferenceRuns

		s := m.acquireScratch()
		c0 := s.coarseRuns
		m.releaseScratch(s)

		got, gotOK, gs := m.routeBeside(c.hx, c.hy, c.qx, c.qy)

		s = m.acquireScratch()
		gotCoarse := s.coarseRuns - c0
		m.releaseScratch(s)

		require.False(t, wantOK, "%s: out of reach", c.name)
		require.Equal(t, wantOK, gotOK, c.name)
		require.Equal(t, want, got, "%s: the same partial route", c.name)
		require.NotEmpty(t, got, "%s: a partial route to walk", c.name)
		require.GreaterOrEqual(t, ws, 9, "%s: the reference pays nine", c.name)
		require.Equal(t, 2, gs, "%s: the query pays two", c.name)
		require.Zero(t, gotCoarse, "%s: and no corridor", c.name)
		t.Logf("%s: %d searches and %d corridors, the reference %d and %d", c.name, gs, gotCoarse, ws, wantCoarse)
	}
}

// A TILE HALF SEALED IS NOT PROVEN. The goal subtile (the tile's corner, the
// one routeExact aims at) is walled into a pocket of one, but the rest of its
// tile is open: the old route toward it ends on the tile's open side, which
// routeExact counts as reaching. So the query must not call the tile
// unreachable, though its goal subtile is. Control (a source mutation):
// ProvenUnreachable checking the goal subtile alone goes red here.
func TestATileHalfSealedIsNotProven(t *testing.T) {
	m := testEngine(20, 20)

	for _, d := range neighbourOffsets {
		block(m, 50+d.x, 50+d.y)
	}

	start, dest := d2vector.NewPosition(80.5, 80.5), d2vector.NewPositionTile(10, 10)

	want, _ := m.pathFindReference(start, dest)
	require.True(t, endsIn(want, 10, 10), "the old route ends in the tile: routeExact's reachable")

	q := m.NewRouteQuery(start)
	defer q.Close()

	require.Equal(t, want, q.PathFind(dest), "the same route")
	require.True(t, q.unreachable(subTile{50, 50}), "the goal subtile is proven sealed")
	require.False(t, q.ProvenUnreachable(dest, 10, 10), "but its tile is not")
}

// Four goroutines routing on one engine at once each get the reference's
// answers: a caller that finds the engine's scratch taken works in one of its
// own. Control (a source mutation): acquireScratch leaving the scratch in
// place, so all four share it, goes red.
func TestConcurrentPathFindsAgree(t *testing.T) {
	m := referenceMaze(3)
	rng := rand.New(rand.NewSource(77)) // nolint:gosec // a test's pairs

	type pair struct{ a, b d2vector.Position }

	pairs := make([]pair, 60)
	want := make([][]d2vector.Position, len(pairs))

	for i := range pairs {
		a := d2vector.NewPosition(rng.Float64()*200, rng.Float64()*200)
		b := d2vector.NewPosition(a.X()+rng.Float64()*60-30, a.Y()+rng.Float64()*60-30)
		pairs[i] = pair{a, b}
		want[i], _ = m.pathFindReference(pairs[i].a, pairs[i].b)
	}

	var wg sync.WaitGroup

	errs := make(chan int, 4*3*len(pairs))
	gate := make(chan struct{})

	for g := 0; g < 4; g++ {
		wg.Add(1)

		go func(g int) {
			defer wg.Done()

			<-gate

			for k := 0; k < 3*len(pairs); k++ {
				i := (k + g*15) % len(pairs)
				if got := m.PathFind(pairs[i].a, pairs[i].b); !reflect.DeepEqual(got, want[i]) {
					errs <- i
				}
			}
		}(g)
	}

	close(gate)
	wg.Wait()
	close(errs)

	for i := range errs {
		t.Fatalf("a concurrent PathFind disagreed with the reference (pair %d)", i)
	}
}

func BenchmarkRouteBesideSealedYard(b *testing.B) {
	m := sealedYard()

	b.Run("query", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m.routeBeside(10.5, 10.5, 32.5, 31.5)
		}
	})
	b.Run("reference", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m.routeBesideReference(10.5, 10.5, 32.5, 31.5)
		}
	})
}
