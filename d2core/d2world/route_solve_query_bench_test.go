package d2world

import (
	"math"
	"reflect"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
)

// The game's Route loop since BUG-115 (game.go mapRouter.Route): one
// RouteQuery for every attempt, a candidate proven out of reach skipped.
// Registered here, apart from route_solve_bench_test.go, so that file builds
// against the engine before BUG-115 for the "before" numbers.
func init() {
	villageRouteKinds = append(villageRouteKinds, villageRouteKind{"query", func(e *d2mapengine.MapEngine) (Router, *int, string) {
		n := 0
		return besideQueryRouter{e: e, searches: &n}, &n, "searches/solve"
	}})
}

type besideQueryRouter struct {
	e        *d2mapengine.MapEngine
	searches *int
}

func (r besideQueryRouter) Route(fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	q := r.e.NewRouteQuery(d2vector.NewPositionTile(fromX, fromY))

	defer func() {
		*r.searches += q.Searches()
		q.Close()
	}()

	for _, n := range benchUnblockedNeighbours(r.e, fromX, fromY, toX, toY) {
		gx, gy := toX+n[0], toY+n[1]

		if q.ProvenUnreachable(d2vector.NewPositionTile(gx, gy), int(math.Floor(gx)), int(math.Floor(gy))) {
			continue
		}

		if beside, ok := routeInBench(q, gx, gy); ok {
			return beside, true
		}
	}

	return routeInBench(q, toX, toY)
}

// routeInBench is game.go's routeIn: the query's PathFind, in world tiles,
// and whether it ends on the goal tile.
func routeInBench(q *d2mapengine.RouteQuery, toX, toY float64) ([][2]float64, bool) {
	path := q.PathFind(d2vector.NewPositionTile(toX, toY))
	out := make([][2]float64, 0, len(path))

	for i := range path {
		w := path[i].World()
		out = append(out, [2]float64{w.X(), w.Y()})
	}

	reachable := false
	if n := len(out); n > 0 {
		reachable = math.Floor(out[n-1][0]) == math.Floor(toX) && math.Floor(out[n-1][1]) == math.Floor(toY)
	}

	return out, reachable
}

// ON THE REAL VILLAGE, THE QUERY LOOP IS THE OLD LOOP. Every placement of the
// solve bench, open and walled in, plus the reverse (the hunter in the yard):
// the same route and the same reachable from both, and never more searches
// than the old loop's PathFinds (each at least one search). The walled-in
// placements are where the proofs fire. Control (a source mutation in
// d2mapengine/scratch.go): learn marking a search's cells sealed rather than
// reachable goes red here.
func TestTheRouteQueryIsTheOldLoopOnTheVillage(t *testing.T) {
	type run struct {
		name   string
		e      func() (*d2mapengine.MapEngine, [][4]float64)
		proven bool
	}

	runs := []run{
		{"open", func() (*d2mapengine.MapEngine, [][4]float64) {
			e, m := seekBenchVillage(t)
			return e, villageSolvePairs(m, false, [2]int{})
		}, false},
		{"walledin", func() (*d2mapengine.MapEngine, [][4]float64) {
			e, m, c := walledInVillage(t)
			return e, villageSolvePairs(m, true, c)
		}, true},
		{"hunter in the yard", func() (*d2mapengine.MapEngine, [][4]float64) {
			e, m, c := walledInVillage(t)
			pairs := villageSolvePairs(m, true, c)
			for i := range pairs { // swap: the villager hunts out of his yard
				pairs[i] = [4]float64{pairs[i][2], pairs[i][3], pairs[i][0], pairs[i][1]}
			}
			return e, pairs
		}, true},
		{"quarry on the yard's wall, hunter beyond it", func() (*d2mapengine.MapEngine, [][4]float64) {
			// The nearest candidates are inside the yard and fail first; the
			// ones outside it, in the hunter's own region, must still be
			// tried and reached.
			e, m, c := walledInVillage(t)
			var pairs [][4]float64
			for y := c[1] + 3; y < m.Height; y++ {
				for x := c[0] - 3; x <= c[0]+3; x++ {
					if !m.Blocked(x, y) {
						pairs = append(pairs, [4]float64{float64(x) + 0.5, float64(y) + 0.5, float64(c[0]) + 0.5, float64(c[1]-2) + 0.5})
					}
				}
			}
			return e, pairs
		}, false}, // mixed: searches include the corridors' legs, PathFinds do not
	}

	for _, r := range runs {
		e, pairs := r.e()
		attempts, searches := 0, 0
		old := besideAttemptsRouter{e: e, attempts: &attempts}
		now := besideQueryRouter{e: e, searches: &searches}

		for i, p := range pairs {
			wantPath, wantOK := old.Route(p[0], p[1], p[2], p[3])
			gotPath, gotOK := now.Route(p[0], p[1], p[2], p[3])

			if wantOK != gotOK || !reflect.DeepEqual(wantPath, gotPath) {
				t.Fatalf("%s pair %d %v: the query loop answered (%v, %v), the old loop (%v, %v)", r.name, i, p, gotOK, gotPath, wantOK, wantPath)
			}
		}

		t.Logf("%s: %d solves, %d searches against the old loop's %d PathFinds", r.name, len(pairs), searches, attempts)

		if r.proven && searches >= attempts {
			t.Fatalf("%s: the proofs saved nothing (%d searches, %d PathFinds)", r.name, searches, attempts)
		}
	}
}
