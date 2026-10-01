package d2mapgen

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// villageEngine is the shipped village laid into an engine, as the game lays
// it (LayAuthoredMap), without its people.
func villageEngine(tb testing.TB) (*d2mapengine.MapEngine, *d2maptiled.Map) {
	tb.Helper()

	root := filepath.Join("..", "..", "..", "data", "strigoi", "maps")
	strigoi := filepath.Dir(root)

	data, err := os.ReadFile(filepath.Join(root, "village.tmj"))
	if err != nil {
		tb.Fatalf("reading the shipped village: %v", err)
	}

	m, err := d2maptiled.Parse(data, "/data/strigoi/maps", func(p string) ([]byte, error) {
		return os.ReadFile(filepath.Join(strigoi, filepath.FromSlash(strings.TrimPrefix(path.Clean(p), "/data/strigoi"))))
	})
	if err != nil {
		tb.Fatalf("the shipped village is refused: %v", err)
	}

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		tb.Fatal(err)
	}

	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, asset)
	LayAuthoredMap(engine, m)

	return engine, m
}

// BenchmarkFogRecompute: one recompute of the fog on the authored village,
// with 1, 6 and 24 eyes about the start, at day sight 12 (Josh's Q1) and 16
// (F4's raised sight). The budget is a worst frame of 0.5 ms (plan §3.8).
// Every iteration recomputes (a dial write makes it dirty): this is the cost
// of a frame on which he steps onto a new tile, not of a skipped one.
func BenchmarkFogRecompute(b *testing.B) {
	engine, m := villageEngine(b)
	size := engine.Size()

	for _, sight := range []float64{12, 16} {
		for _, n := range []int{1, 6, 24} {
			eyes := make([]d2world.Eye, n)
			for i := range eyes {
				a := 2 * math.Pi * float64(i) / float64(n)
				r := 3.0 * float64(i%3)
				eyes[i] = d2world.Eye{ID: fmt.Sprintf("e%d", i), X: m.StartX + 0.5 + r*math.Cos(a), Y: m.StartY + 0.5 + r*math.Sin(a)}
			}

			b.Run(fmt.Sprintf("sight%.0f/eyes%d", sight, n), func(b *testing.B) {
				dials := d2world.DefaultFogDials()
				dials.DaySight = sight
				f := d2world.NewFog(dials, engine)

				for i := 0; i < b.N; i++ {
					f.SetDials(dials)
					f.Update(size.Width, size.Height, eyes)
				}

				_, visible := f.Counts()
				_, _, cells := f.Counters()
				b.ReportMetric(float64(visible), "visible")
				b.ReportMetric(float64(cells)/float64(b.N), "cells/op")
			})
		}
	}
}

// BenchmarkFogRecomputeAtNight (F2): one recompute at deep night (new moon)
// on the authored village with 1, 6 and 24 eyes and 0, 4 or 16 lit hearths
// in a ring about the start (14 tiles out), plus his lit torch: the unlit
// discs shrink to the dark radius, and the lit term (Q3: lit ground at any
// distance) tries a line from each eye to every lit tile of every source.
// With a lit torch every frame of a walk recomputes (the torch's disc moves),
// so this is the cost of a walking frame at night.
func BenchmarkFogRecomputeAtNight(b *testing.B) {
	engine, m := villageEngine(b)
	size := engine.Size()

	for _, hearths := range []int{0, 4, 16} {
		for _, n := range []int{1, 6, 24} {
			eyes := make([]d2world.Eye, n)
			for i := range eyes {
				a := 2 * math.Pi * float64(i) / float64(n)
				r := 3.0 * float64(i%3)
				eyes[i] = d2world.Eye{ID: fmt.Sprintf("e%d", i), X: m.StartX + 0.5 + r*math.Cos(a), Y: m.StartY + 0.5 + r*math.Sin(a)}
			}

			b.Run(fmt.Sprintf("hearths%d/eyes%d", hearths, n), func(b *testing.B) {
				clock := d2world.NewClock(d2world.DefaultClockDials()) // the epoch: the night floor
				defer clock.Close()

				clock.SetMoon(0)

				light := d2world.NewLight(clock, d2world.DefaultLightDials())
				defer light.Close()

				light.Add(d2world.SourceTorch, true, 0, 0)

				for i := 0; i < hearths; i++ {
					a := 2 * math.Pi * float64(i) / float64(hearths)
					light.Add(d2world.SourceHearth, false, m.StartX+0.5+14*math.Cos(a), m.StartY+0.5+14*math.Sin(a))
				}

				view := d2world.NewLightView(light)
				view.SetCarriedAt(m.StartX+0.5, m.StartY+0.5)

				dials := d2world.DefaultFogDials()
				f := d2world.NewFog(dials, engine)
				f.SetLight(view)

				for i := 0; i < b.N; i++ {
					f.SetDials(dials)
					f.Update(size.Width, size.Height, eyes)
				}

				_, visible := f.Counts()
				_, _, cells := f.Counters()
				b.ReportMetric(float64(visible), "visible")
				b.ReportMetric(float64(f.LitSeen()), "lit_seen")
				b.ReportMetric(float64(cells)/float64(b.N), "cells/op")
			})
		}
	}
}

// TestFogOnTheVillageSeesHisSurroundings: the fog over the real village from
// the start, at day sight 12, sees a good share of the 12-tile disc (452
// tiles) -- the houses hide some -- and the start tile; and somewhere within
// 11 tiles a house or wall hides what stands behind it (the map really blocks).
func TestFogOnTheVillageSeesHisSurroundings(t *testing.T) {
	engine, m := villageEngine(t)
	size := engine.Size()

	f := d2world.NewFog(d2world.DefaultFogDials(), engine)
	f.Update(size.Width, size.Height, []d2world.Eye{{ID: "s:1", X: m.StartX + 0.5, Y: m.StartY + 0.5}})

	_, visible := f.Counts()
	t.Logf("from the start (%.1f,%.1f) at day sight 12 he sees %d tiles", m.StartX+0.5, m.StartY+0.5, visible)

	if visible < 150 || visible > 452 {
		t.Fatalf("from the start he sees %d tiles; a 12-tile disc is at most ~452 and the village is mostly open", visible)
	}

	if st := f.At(int(m.StartX), int(m.StartY)); st != d2world.FogVisible {
		t.Fatalf("the start tile is %v", st)
	}

	hidden := 0

	for ty := 0; ty < size.Height; ty++ {
		for tx := 0; tx < size.Width; tx++ {
			d := math.Hypot(float64(tx)+0.5-m.StartX-0.5, float64(ty)+0.5-m.StartY-0.5)
			if d <= 11 && f.At(tx, ty) == d2world.FogUnexplored {
				hidden++
			}
		}
	}

	t.Logf("%d tiles within 11 of him are hidden by the houses and walls", hidden)

	if hidden == 0 {
		t.Fatal("nothing within 11 tiles of the start is hidden: the village's walls block no sight")
	}
}

// TestTheVillagesHousesAreSeenWhole: on the real village, from the start, no
// structure is part-seen -- each is visible whole or not at all -- and at least
// one is seen. The footprints reach the fog through LayAuthoredMap's
// SetStructures.
//
// Negative control (1 Oct 2026, strigoi-harness-runs\wt-fog\nc\): drop
// LayAuthoredMap's SetStructures and this fails, "the engine has 0 structure
// footprints; the village has 7" (nc17-no-structures-laid.txt).
func TestTheVillagesHousesAreSeenWhole(t *testing.T) {
	engine, m := villageEngine(t)
	size := engine.Size()

	if got := len(engine.Structures()); got != len(m.Structures) || got == 0 {
		t.Fatalf("the engine has %d structure footprints; the village has %d", got, len(m.Structures))
	}

	f := d2world.NewFog(d2world.DefaultFogDials(), engine)
	f.Update(size.Width, size.Height, []d2world.Eye{{ID: "s:1", X: m.StartX + 0.5, Y: m.StartY + 0.5}})

	seen := 0

	for _, s := range m.Structures {
		r := s.Footprint
		visible, tiles := 0, 0

		for ty := r.Min.Y; ty < r.Max.Y; ty++ {
			for tx := r.Min.X; tx < r.Max.X; tx++ {
				tiles++

				if f.At(tx, ty) == d2world.FogVisible {
					visible++
				}
			}
		}

		t.Logf("structure %v: %d of %d tiles visible", r, visible, tiles)

		if visible > 0 && visible < tiles {
			t.Errorf("structure %v is part-seen: %d of %d tiles visible", r, visible, tiles)
		}

		if visible == tiles {
			seen++
		}
	}

	if seen == 0 {
		t.Fatal("no structure is seen from the start")
	}
}
