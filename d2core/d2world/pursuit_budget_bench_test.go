package d2world

// THE PER-FRAME SOLVE BUDGET, MEASURED ON THE REAL VILLAGE (2 Oct 2026).
// Two bursts the R2 benchmark does not make:
//
//   - PackNoticesAtOnce: a pack of eight, placed anywhere standable, notices
//     him (inside the fence) in one frame and every member's chase starts
//     (Pursuit.Rechase, as Game.startChasesForTheAware calls it); then frames
//     run, Pursuit first, until no chase owes its route. Each op is one such
//     burst on a fresh placement (a fixed seed). Reported: the frame mean
//     (over every frame of every burst), its 99th percentile and the worst,
//     the most solves in a frame, and the frames until every member had its
//     route.
//   - LongStep: the R2 night (N hunters, M quarries walking) given one frame
//     of ten world minutes -- a sleep's or a labour's step -- which makes
//     every chase due at once, then a world minute of night frames. Each op is
//     one long frame and the 24 after it. Reported: the long frame's mean and
//     worst, the solves in it, and the p99 and worst of every frame.
//
// Each with no budget (budget0, as before) and the shipped SolvesPerFrame.
//
//	go test ./d2core/d2world -run XXX -bench 'PursuitPackNoticesAtOnce|PursuitLongStep' -benchmem -count 3

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"
)

func benchFrameStats(b *testing.B, times []time.Duration, prefix string) {
	b.Helper()

	if len(times) == 0 {
		return
	}

	sorted := append([]time.Duration(nil), times...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}

	b.ReportMetric(float64(sum.Nanoseconds())/float64(len(sorted)), prefix+"mean-frame-ns")
	b.ReportMetric(float64(sorted[len(sorted)*99/100].Nanoseconds()), prefix+"p99-frame-ns")
	b.ReportMetric(float64(sorted[len(sorted)-1].Nanoseconds()), prefix+"worst-frame-ns")
}

func BenchmarkPursuitPackNoticesAtOnce(b *testing.B) {
	for _, budget := range []int{0, DefaultPursuitDials().SolvesPerFrame} {
		b.Run(fmt.Sprintf("pack8/budget%d", budget), func(b *testing.B) {
			e, m := seekBenchVillage(b)
			rng := rand.New(rand.NewSource(1462)) // nolint:gosec // a benchmark's placement

			var open [][2]int

			for y := 0; y < m.Height; y++ {
				for x := 0; x < m.Width; x++ {
					if !m.Blocked(x, y) {
						open = append(open, [2]int{x, y})
					}
				}
			}

			dials := DefaultPursuitDials()
			dials.SolvesPerFrame = budget
			p := NewPursuit(rechaseBenchRouter{e: e}, dials)
			b.Cleanup(p.Close)

			him := &fakeQuarry{id: "p:1", x: 24.5, y: 24.5}

			var times []time.Duration

			frames, busiest := 0, 0

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				for _, id := range p.hunterIDs() {
					p.Release(id)
				}

				pack := make([]*rechaseBenchHunter, 8)
				for k := range pack {
					o := open[rng.Intn(len(open))]
					pack[k] = &rechaseBenchHunter{id: fmt.Sprintf("w:%d", k), x: float64(o[0]) + 0.5, y: float64(o[1]) + 0.5}
				}

				b.StartTimer()

				for f := 0; f == 0 || p.Queued() > 0; f++ {
					s0 := p.Solves()
					start := time.Now()

					p.Advance(1.0 / 24)

					if f == 0 {
						for _, h := range pack {
							p.Rechase(h, him)
						}
					}

					p.ServeQueued()

					times = append(times, time.Since(start))

					if n := p.Solves() - s0; n > busiest {
						busiest = n
					}

					frames++
				}
			}

			b.StopTimer()

			benchFrameStats(b, times, "")
			b.ReportMetric(float64(busiest), "solves-in-busiest-frame")
			b.ReportMetric(float64(frames)/float64(b.N), "frames-to-all-routed")
		})
	}
}

func BenchmarkPursuitLongStep(b *testing.B) {
	for _, c := range []struct{ n, m int }{{24, 30}, {48, 60}} {
		for _, budget := range []int{0, DefaultPursuitDials().SolvesPerFrame} {
			b.Run(fmt.Sprintf("N%d_M%d/budget%d", c.n, c.m, budget), func(b *testing.B) {
				f := newRechaseBenchNightBudget(b, c.n, c.m, true, budget)

				var long, all []time.Duration

				longSolves := 0

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					s0 := f.pursuit.Solves()
					start := time.Now()
					f.stepDt(10)
					d := time.Since(start)
					longSolves += f.pursuit.Solves() - s0

					long = append(long, d)
					all = append(all, d)

					for k := 0; k < 24; k++ {
						start := time.Now()
						f.step()
						all = append(all, time.Since(start))
					}
				}

				b.StopTimer()

				benchFrameStats(b, long, "long-")
				benchFrameStats(b, all, "all-")
				b.ReportMetric(float64(longSolves)/float64(b.N), "solves-in-long-frame")
				b.ReportMetric(float64(f.busiestSolves), "solves-in-busiest-frame")
			})
		}
	}
}
