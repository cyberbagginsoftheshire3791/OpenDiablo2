package d2world

// ONE SOLVE, THROUGH THE GAME'S ROUTE LOOP, ON THE REAL VILLAGE (BUG-115, 2
// Oct 2026). The other pursuit benches route with the game's routeExact
// alone; the game's mapRouter.Route (d2game/d2gamescreen/game.go) tries the
// open tiles beside the quarry nearest first and then the quarry's own, so
// one counted solve can be up to nine PathFinds. This bench is that loop, on
// two placements:
//
//   - open: the quarry anywhere standable inside the fence, the hunter
//     anywhere standable (the R2 benches' placement);
//   - walledin: the quarry a villager in a 3x3-tile yard walled round
//     (its ring of tiles laid with no floor), the hunter anywhere outside --
//     the case where every candidate is out of reach.
//
// Routers: attempts is the loop as the game had it before BUG-115, one
// PathFind an attempt (reports PathFinds per solve); the query loop is
// registered by route_solve_query_bench_test.go (reports searches per solve).
// Each op is one solve, over a fixed cycle of 64 placements. Reported: ns/op
// (the mean), the 99th percentile and the worst solve; and, per placement,
// the fastest of its repeats -- its cost without the machine's interruptions
// -- their mean and the dearest placement's.
//
// And WalledInPack: a pack of eight notices the walled-in villager at once,
// at the shipped SolvesPerFrame, until every member has its route: the frame
// mean, p99 and worst (as BenchmarkPursuitPackNoticesAtOnce).
//
//	go test ./d2core/d2world -run XXX -bench 'VillageRouteSolve|VillageWalledInPack' -benchmem -count 3

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// villageRouteKind is a router under test: a name and a maker that also
// returns the counter it advances (attempts or searches) and its unit.
type villageRouteKind struct {
	name string
	make func(e *d2mapengine.MapEngine) (Router, *int, string)
}

var villageRouteKinds = []villageRouteKind{
	{"attempts", func(e *d2mapengine.MapEngine) (Router, *int, string) {
		n := 0
		return besideAttemptsRouter{e: e, attempts: &n}, &n, "pathfinds/solve"
	}},
}

// benchNeighboursNearest is game.go's neighboursNearest, exactly.
func benchNeighboursNearest(fromX, fromY, toX, toY float64) [8][2]float64 {
	table := [8][2]float64{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}
	idx := [8]int{0, 1, 2, 3, 4, 5, 6, 7}

	dist := func(k int) float64 {
		dx := toX + table[k][0] - fromX
		dy := toY + table[k][1] - fromY

		return dx*dx + dy*dy
	}

	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && (dist(idx[j]) < dist(idx[j-1]) ||
			(dist(idx[j]) == dist(idx[j-1]) && idx[j] < idx[j-1])); j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}

	var out [8][2]float64
	for i, k := range idx {
		out[i] = table[k]
	}

	return out
}

// benchUnblockedNeighbours is game.go's unblockedNeighbours over the engine
// (mapRouter.blockedTile).
func benchUnblockedNeighbours(e *d2mapengine.MapEngine, fromX, fromY, toX, toY float64) [][2]float64 {
	out := make([][2]float64, 0, 8)

	for _, n := range benchNeighboursNearest(fromX, fromY, toX, toY) {
		if e.BlockedAt(int(math.Floor((toX+n[0])*5)), int(math.Floor((toY+n[1])*5))) {
			continue
		}

		out = append(out, n)
	}

	return out
}

// besideAttemptsRouter is mapRouter.Route as it was before BUG-115.
type besideAttemptsRouter struct {
	e        *d2mapengine.MapEngine
	attempts *int
}

func (r besideAttemptsRouter) Route(fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	exact := rechaseBenchRouter{e: r.e}

	for _, n := range benchUnblockedNeighbours(r.e, fromX, fromY, toX, toY) {
		*r.attempts++

		if beside, ok := exact.Route(fromX, fromY, toX+n[0], toY+n[1]); ok {
			return beside, true
		}
	}

	*r.attempts++

	return exact.Route(fromX, fromY, toX, toY)
}

// benchVillageMap parses the village as seekBenchVillage does, unlaid.
func benchVillageMap(tb testing.TB) *d2maptiled.Map {
	tb.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", "data", "strigoi", "maps"))
	if err != nil {
		tb.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "village.tmj")) // nolint:gosec // the repo's own map
	if err != nil {
		tb.Fatal(err)
	}

	strigoi := filepath.Dir(root)

	m, err := d2maptiled.Parse(data, "/data/strigoi/maps", func(p string) ([]byte, error) {
		return os.ReadFile(filepath.Join(strigoi, filepath.FromSlash(strings.TrimPrefix(path.Clean(p), "/data/strigoi")))) // nolint:gosec // the repo's own art
	})
	if err != nil {
		tb.Fatal(err)
	}

	return m
}

// walledInVillage is the village with a yard walled round: the first tile
// inside the fence (scanning rows, then columns, from (15,15)) whose 7x7
// neighbourhood is all standable becomes the yard's centre, and the ring of
// tiles two out from it is laid with no floor. Returns the engine, the map
// as laid, and the centre.
func walledInVillage(tb testing.TB) (*d2mapengine.MapEngine, *d2maptiled.Map, [2]int) {
	tb.Helper()

	m := benchVillageMap(tb)

	clear7 := func(cx, cy int) bool {
		for y := cy - 3; y <= cy+3; y++ {
			for x := cx - 3; x <= cx+3; x++ {
				if m.Blocked(x, y) {
					return false
				}
			}
		}

		return true
	}

	centre := [2]int{-1, -1}

	for y := 15; y < 33 && centre[0] < 0; y++ {
		for x := 15; x < 33; x++ {
			if clear7(x, y) {
				centre = [2]int{x, y}

				break
			}
		}
	}

	if centre[0] < 0 {
		tb.Fatal("no open 7x7 inside the fence for the yard")
	}

	for y := centre[1] - 2; y <= centre[1]+2; y++ {
		for x := centre[0] - 2; x <= centre[0]+2; x++ {
			if abs2(x-centre[0]) == 2 || abs2(y-centre[1]) == 2 {
				m.Cells[x+y*m.Width].Floor = -1
				m.Cells[x+y*m.Width].Wall = -1
			}
		}
	}

	e := &d2mapengine.MapEngine{}
	d2mapgen.LayAuthoredMap(e, m)

	return e, m, centre
}

func abs2(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

// villageSolvePairs is the bench's fixed cycle of (hunter, quarry) placements.
func villageSolvePairs(m *d2maptiled.Map, walled bool, centre [2]int) [][4]float64 {
	rng := rand.New(rand.NewSource(1462)) // nolint:gosec // a benchmark's placement

	var outside, inside [][2]int

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if m.Blocked(x, y) {
				continue
			}

			if walled && abs2(x-centre[0]) <= 2 && abs2(y-centre[1]) <= 2 {
				continue // the yard and its ring
			}

			outside = append(outside, [2]int{x, y})

			if x > 15 && x < 32 && y > 15 && y < 32 {
				inside = append(inside, [2]int{x, y})
			}
		}
	}

	pairs := make([][4]float64, 64)

	for i := range pairs {
		h := outside[rng.Intn(len(outside))]

		var qx, qy float64

		if walled { // the villager somewhere in his 3x3 yard
			qx = float64(centre[0]-1) + rng.Float64()*2.99
			qy = float64(centre[1]-1) + rng.Float64()*2.99
		} else {
			q := inside[rng.Intn(len(inside))]
			qx, qy = float64(q[0])+rng.Float64(), float64(q[1])+rng.Float64()
		}

		pairs[i] = [4]float64{float64(h[0]) + 0.5, float64(h[1]) + 0.5, qx, qy}
	}

	return pairs
}

func BenchmarkVillageRouteSolve(b *testing.B) {
	for _, walled := range []bool{false, true} {
		for _, kind := range villageRouteKinds {
			place := "open"
			if walled {
				place = "walledin"
			}

			b.Run(place+"/"+kind.name, func(b *testing.B) {
				var (
					e      *d2mapengine.MapEngine
					m      *d2maptiled.Map
					centre [2]int
				)

				if walled {
					e, m, centre = walledInVillage(b)
				} else {
					e, m = seekBenchVillage(b)
				}

				pairs := villageSolvePairs(m, walled, centre)
				router, count, unit := kind.make(e)
				times := make([]time.Duration, 0, b.N)
				reached := 0

				// The fastest of each placement's repeats is its cost with the
				// machine's interruptions taken out (a loaded laptop preempts a
				// thread for ~10 ms now and then, which sets every p99 here);
				// the dearest placement by that measure is the worst solve the
				// code itself makes.
				fastest := make([]time.Duration, len(pairs))

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					p := pairs[i%len(pairs)]
					start := time.Now()

					if _, ok := router.Route(p[0], p[1], p[2], p[3]); ok {
						reached++
					}

					d := time.Since(start)
					times = append(times, d)

					if k := i % len(pairs); fastest[k] == 0 || d < fastest[k] {
						fastest[k] = d
					}
				}

				b.StopTimer()

				var dearest, sum time.Duration

				for _, d := range fastest {
					sum += d
					if d > dearest {
						dearest = d
					}
				}

				b.ReportMetric(float64(dearest.Nanoseconds()), "dearest-placement-ns")
				b.ReportMetric(float64(sum.Nanoseconds())/float64(len(fastest)), "placement-mean-ns")

				sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
				b.ReportMetric(float64(times[len(times)*99/100].Nanoseconds()), "p99-solve-ns")
				b.ReportMetric(float64(times[len(times)-1].Nanoseconds()), "worst-solve-ns")
				b.ReportMetric(float64(*count)/float64(b.N), unit)
				b.ReportMetric(float64(reached)/float64(b.N), "reached/solve")
			})
		}
	}
}

func BenchmarkVillageWalledInPack(b *testing.B) {
	for _, kind := range villageRouteKinds {
		b.Run(fmt.Sprintf("pack8/budget%d/%s", DefaultPursuitDials().SolvesPerFrame, kind.name), func(b *testing.B) {
			e, m, centre := walledInVillage(b)
			pairs := villageSolvePairs(m, true, centre)
			router, _, _ := kind.make(e)

			p := NewPursuit(router, DefaultPursuitDials())
			b.Cleanup(p.Close)

			villager := &fakeQuarry{id: "v:1", x: float64(centre[0]) + 0.5, y: float64(centre[1]) + 0.5}

			var times []time.Duration

			busiest, next := 0, 0

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				for _, id := range p.hunterIDs() {
					p.Release(id)
				}

				pack := make([]*rechaseBenchHunter, 8)
				for k := range pack {
					o := pairs[next%len(pairs)]
					next++
					pack[k] = &rechaseBenchHunter{id: fmt.Sprintf("w:%d", k), x: o[0], y: o[1]}
				}

				b.StartTimer()

				for f := 0; f == 0 || p.Queued() > 0; f++ {
					s0 := p.Solves()
					start := time.Now()

					p.Advance(1.0 / 24)

					if f == 0 {
						for _, h := range pack {
							p.Rechase(h, villager)
						}
					}

					p.ServeQueued()

					times = append(times, time.Since(start))

					if n := p.Solves() - s0; n > busiest {
						busiest = n
					}
				}
			}

			b.StopTimer()

			benchFrameStats(b, times, "")
			b.ReportMetric(float64(busiest), "solves-in-busiest-frame")
		})
	}
}
