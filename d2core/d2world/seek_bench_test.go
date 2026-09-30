package d2world

// THE WORST FRAME, MEASURED ON THE REAL VILLAGE (the raid's R2; the brief's
// M0.2 (iii), "committed in R2, not thrown away"; D-B1's 2 ms budget for
// notice and Seek together at N 24 x M 30).
//
// The shipped village.tmj is parsed by d2maptiled.Parse and laid into a real
// MapEngine by d2mapgen.LayAuthoredMap, which is what the game does, and the
// real MapEngine.LineOfSight stands behind the notice model's Sight. The
// light is the measured night ambient, 0.5, above LitLevel 0.30: at night
// every quarry is lit, so reach is Radius x LitMultiplier = 24 tiles (S0).
// M quarries stand inside the fence and N watchers anywhere standable, each
// watching quarry i mod M; Seek's living are the M quarries.
//
// TWO FRAMES, EACH OP ONE OF THEM:
//   - all-due: every watch's sight test due at once (Notice.Advance of a
//     whole ReEvaluateMinutes) and every Seek row due at once (a whole
//     RetargetMinutes) -- the unstaggered worst frame, S0's.
//   - staggered: every watch's sight test due at once still (the notice
//     model's own cadence is not staggered: its cost is N rays, not N x M),
//     and Seek on one night frame (1/24 minute) under the stagger, so one
//     phase's rows look. ns/op is the mean over the minute's phases, and
//     worst-frame-ns is the costliest phase's mean: the frame the budget is
//     against.
//
// No A* runs in either (D-S2 = 0: the raid's S0-2 (a); Seek serves beasts and
// men, who choose by sight). rays/frame counts every sight test the op cast,
// the notice model's and Seek's.
//
//	go test ./d2core/d2world -run XXX -bench NoticeSeekWorstFrame -benchmem -count 3

import (
	"fmt"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// seekBenchVillage is the shipped village, laid as the game lays it.
func seekBenchVillage(tb testing.TB) (*d2mapengine.MapEngine, *d2maptiled.Map) {
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

	e := &d2mapengine.MapEngine{}
	d2mapgen.LayAuthoredMap(e, m)

	return e, m
}

// seekBenchSight is the game's mapSight (game.go), counted.
type seekBenchSight struct {
	e    *d2mapengine.MapEngine
	rays int
}

func (s *seekBenchSight) Clear(fx, fy, tx, ty float64) bool {
	s.rays++

	return s.e.LineOfSight(d2vector.NewPositionTile(fx, fy), d2vector.NewPositionTile(tx, ty))
}

// seekBenchNight is the measured night ambient (0.5), above LitLevel.
type seekBenchNight struct{}

func (seekBenchNight) Level(_, _ int) float64 { return 0.5 }

// seekBenchFrame is one village night of N watchers and M quarries.
type seekBenchFrame struct {
	sight  *seekBenchSight
	notice *Notice
	seek   *Seek
}

func newSeekBenchFrame(tb testing.TB, n, mq int, staggered bool) *seekBenchFrame {
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

			if x > 12 && x < 35 && y > 12 && y < 35 {
				inside = append(inside, [2]int{x, y})
			}
		}
	}

	f := &seekBenchFrame{sight: &seekBenchSight{e: e}}
	f.notice = NewNotice(f.sight, seekBenchNight{}, DefaultNoticeDials())

	dials := DefaultSeekDials()
	if !staggered {
		dials.StaggerSlots = 1
	}

	f.seek = NewSeek(f.notice, nil, nil, dials)
	tb.Cleanup(f.seek.Close)

	quarries := make([]Quarry, 0, mq)

	for i := 0; i < mq; i++ {
		p := inside[rng.Intn(len(inside))]
		quarries = append(quarries, &fakeQuarry{id: fmt.Sprintf("q:%03d", i), x: float64(p[0]) + 0.5, y: float64(p[1]) + 0.5})
	}

	f.seek.SetQuarries(func() []Quarry { return quarries })

	for i := 0; i < n; i++ {
		p := open[rng.Intn(len(open))]
		w := &fakeWatcher{id: fmt.Sprintf("w:%03d", i), x: float64(p[0]) + 0.5, y: float64(p[1]) + 0.5}
		f.notice.Watch(w, quarries[i%mq])
	}

	// Make every row, then run two whole minutes, so each op finds the
	// night in its steady state: rows on their phases, watches on their
	// nearest.
	for i := 0; i < 48; i++ {
		f.notice.Advance(1.0 / 24)
		f.seek.Advance(1.0 / 24)
	}

	return f
}

func BenchmarkNoticeSeekWorstFrame(b *testing.B) {
	for _, c := range []struct{ n, m int }{
		{24, 30},  // the brief's N and M
		{48, 60},  // 2x
		{96, 120}, // 4x
	} {
		for _, staggered := range []bool{false, true} {
			name := "all-due"
			if staggered {
				name = "staggered"
			}

			b.Run(fmt.Sprintf("N%d_M%d/%s", c.n, c.m, name), func(b *testing.B) {
				f := newSeekBenchFrame(b, c.n, c.m, staggered)
				retarget := f.seek.Dials().RetargetMinutes
				seekStep := retarget

				if staggered {
					seekStep = retarget / float64(f.seek.Dials().StaggerSlots)
				}

				slots := f.seek.Dials().StaggerSlots
				perSlot := make([]time.Duration, slots)
				perCount := make([]int, slots)
				r0 := f.sight.rays

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					t0 := time.Now()

					f.notice.Advance(f.notice.dials.ReEvaluateMinutes)
					f.seek.Advance(seekStep)

					perSlot[i%slots] += time.Since(t0)
					perCount[i%slots]++
				}

				b.StopTimer()

				worst := 0.0

				for s := range perSlot {
					if perCount[s] == 0 {
						continue
					}

					if mean := float64(perSlot[s].Nanoseconds()) / float64(perCount[s]); mean > worst {
						worst = mean
					}
				}

				b.ReportMetric(float64(f.sight.rays-r0)/float64(b.N), "rays/frame")
				b.ReportMetric(worst, "worst-frame-ns")
			})
		}
	}
}
