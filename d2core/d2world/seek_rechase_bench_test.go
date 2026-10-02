package d2world

// THE RE-CHASE, MEASURED WITH THE QUARRIES MOVING (the R2 review's B2, 1 Oct
// 2026). BenchmarkNoticeSeekWorstFrame's quarries stand still, so after its
// warm-up no watch moves and it measures none of a retarget's real cost: Seek
// casts rays only, but the game restarts the chase on a moved watch in the
// same frame (Game.startChasesForTheAware) and Pursuit.Chase solves an A* at
// once. Here every frame is the game's night frame, in its order -- the
// notice model, Seek, the re-chase, Pursuit -- over the real village, the
// real LineOfSight and the real PathFind (the game's mapRouter, its exact
// route), with M quarries inside the fence each walking three tiles back and
// forth (about 0.08 tile a frame, a villager's walk) and N hunters standing
// where they were placed (stationary, so every solve is the quarries' doing).
//
// Two variants: sticky (the review's B1 fix, the shipped dials) and strict
// (no margin, no dwell: R2 as first built). Reported per world minute (24
// frames): retargets, rechase solves (Pursuit's count of chases restarted on
// another quarry -- the cost B2 names), and all solves; the frame: the mean
// (ns/op), the 99th percentile and the worst single frame; and the
// re-chase itself: the mean and worst wall time of one restarted chase (its
// solve) and the most restarted in one frame.
//
//	go test ./d2core/d2world -run XXX -bench SeekRechaseMovingQuarries -benchmem -count 3

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
)

// rechaseBenchRouter is the game's mapRouter.routeExact over the real engine.
type rechaseBenchRouter struct{ e *d2mapengine.MapEngine }

func (r rechaseBenchRouter) Route(fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	path := r.e.PathFind(d2vector.NewPositionTile(fromX, fromY), d2vector.NewPositionTile(toX, toY))
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

// rechaseBenchHunter watches and hunts, and stands where it was put.
type rechaseBenchHunter struct {
	id   string
	x, y float64
}

func (h *rechaseBenchHunter) WatcherID() string             { return h.id }
func (h *rechaseBenchHunter) WatcherAt() (float64, float64) { return h.x, h.y }
func (h *rechaseBenchHunter) HunterID() string              { return h.id }
func (h *rechaseBenchHunter) HunterAt() (float64, float64)  { return h.x, h.y }
func (h *rechaseBenchHunter) Following() bool               { return false }
func (h *rechaseBenchHunter) Follow(_ [][2]float64)         {}

// rechaseBenchQuarry walks three tiles back and forth along its own bearing.
type rechaseBenchQuarry struct {
	fakeQuarry
	x0, y0, dx, dy, phase float64
}

func (q *rechaseBenchQuarry) step(frame int) {
	s := 3 * math.Sin(float64(frame)*2*math.Pi/240+q.phase)
	q.x, q.y = q.x0+s*q.dx, q.y0+s*q.dy
}

type rechaseBenchNight struct {
	quarries []*rechaseBenchQuarry
	notice   *Notice
	seek     *Seek
	pursuit  *Pursuit
	frame    int

	// rechaseTimes is the wall time of each Chase that restarted a chase on
	// another quarry; busiest is the most such in one frame.
	rechaseTimes []time.Duration
	busiest      int
}

func newRechaseBenchNight(tb testing.TB, n, mq int, sticky bool) *rechaseBenchNight {
	tb.Helper()

	e, m := seekBenchVillage(tb)
	rng := rand.New(rand.NewSource(1462)) // nolint:gosec // a benchmark's placement, the S0 instrument's seed

	var open, inside [][2]int

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if m.Blocked(x, y) {
				continue
			}

			open = append(open, [2]int{x, y})

			if x > 15 && x < 32 && y > 15 && y < 32 {
				inside = append(inside, [2]int{x, y})
			}
		}
	}

	f := &rechaseBenchNight{}
	f.notice = NewNotice(&seekBenchSight{e: e}, seekBenchNight{}, DefaultNoticeDials())

	dials := DefaultSeekDials()
	if !sticky {
		dials.SwitchMarginTiles, dials.DwellMinutes = 0, 0
	}

	f.seek = NewSeek(f.notice, nil, nil, dials)
	f.pursuit = NewPursuit(rechaseBenchRouter{e: e}, DefaultPursuitDials())

	tb.Cleanup(f.seek.Close)
	tb.Cleanup(f.pursuit.Close)

	living := make([]Quarry, 0, mq)

	for i := 0; i < mq; i++ {
		p := inside[rng.Intn(len(inside))]
		a := rng.Float64() * 2 * math.Pi
		q := &rechaseBenchQuarry{
			fakeQuarry: fakeQuarry{id: fmt.Sprintf("q:%03d", i)},
			x0:         float64(p[0]) + 0.5, y0: float64(p[1]) + 0.5,
			dx: math.Cos(a), dy: math.Sin(a), phase: rng.Float64() * 2 * math.Pi,
		}
		q.step(0)
		f.quarries = append(f.quarries, q)
		living = append(living, &q.fakeQuarry)
	}

	f.seek.SetQuarries(func() []Quarry { return living })

	for i := 0; i < n; i++ {
		p := open[rng.Intn(len(open))]
		h := &rechaseBenchHunter{id: fmt.Sprintf("w:%03d", i), x: float64(p[0]) + 0.5, y: float64(p[1]) + 0.5}
		f.notice.Watch(h, living[i%mq])
	}

	// Two minutes to settle: rows on their phases, chases begun.
	for i := 0; i < 48; i++ {
		f.step()
	}

	return f
}

// step is one night frame in the game's order (advanceWorld).
func (f *rechaseBenchNight) step() {
	f.frame++

	for _, q := range f.quarries {
		q.step(f.frame)
	}

	const dt = 1.0 / 24

	f.notice.Advance(dt)
	f.seek.Advance(dt)

	// Game.startChasesForTheAware.
	inFrame := 0

	for _, pair := range f.notice.AwarePairs() {
		h, ok := pair.Watcher.(Hunter)
		if !ok {
			continue
		}

		if whom, chasing := f.pursuit.ChasingWhom(h.HunterID()); chasing && (pair.Target == nil || whom == pair.Target.QuarryID()) {
			continue
		}

		r0, t0 := f.pursuit.rechases, time.Now()
		f.pursuit.Rechase(h, pair.Target)

		if f.pursuit.rechases > r0 {
			f.rechaseTimes = append(f.rechaseTimes, time.Since(t0))
			inFrame++
		}
	}

	if inFrame > f.busiest {
		f.busiest = inFrame
	}

	f.pursuit.Advance(dt)
}

func BenchmarkSeekRechaseMovingQuarries(b *testing.B) {
	for _, c := range []struct{ n, m int }{
		{24, 30}, // the brief's N and M
		{48, 60}, // 2x
	} {
		for _, sticky := range []bool{true, false} {
			name := "sticky"
			if !sticky {
				name = "strict"
			}

			b.Run(fmt.Sprintf("N%d_M%d/%s", c.n, c.m, name), func(b *testing.B) {
				f := newRechaseBenchNight(b, c.n, c.m, sticky)
				r0, s0, t0 := f.pursuit.rechases, f.pursuit.Solves(), f.seek.retargets
				times := make([]time.Duration, 0, b.N)
				f.rechaseTimes, f.busiest = nil, 0

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					start := time.Now()
					f.step()
					times = append(times, time.Since(start))
				}

				b.StopTimer()

				sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

				minutes := float64(b.N) / 24
				b.ReportMetric(float64(f.seek.retargets-t0)/minutes, "retargets/min")
				b.ReportMetric(float64(f.pursuit.rechases-r0)/minutes, "rechase-solves/min")
				b.ReportMetric(float64(f.pursuit.Solves()-s0)/minutes, "solves/min")
				b.ReportMetric(float64(times[len(times)*99/100].Nanoseconds()), "p99-frame-ns")
				b.ReportMetric(float64(times[len(times)-1].Nanoseconds()), "worst-frame-ns")

				// What one retarget costs the frame it is made in.
				var sum, worst time.Duration

				for _, d := range f.rechaseTimes {
					sum += d
					if d > worst {
						worst = d
					}
				}

				if len(f.rechaseTimes) > 0 {
					b.ReportMetric(float64(sum.Nanoseconds())/float64(len(f.rechaseTimes)), "rechase-mean-ns")
				}

				b.ReportMetric(float64(worst.Nanoseconds()), "rechase-worst-ns")
				b.ReportMetric(float64(f.busiest), "rechases-in-busiest-frame")
			})
		}
	}
}
