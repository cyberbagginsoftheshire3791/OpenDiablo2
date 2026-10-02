package d2mapgen

import (
	"fmt"
	"image"
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
// Every iteration every eye steps one tile (east, then back): this is the
// cost of a frame on which they all step onto a new tile, not of a skipped
// one. (Until F4 a dial write made each iteration dirty; since F4's line
// cache that is a cached frame -- BenchmarkFogRaisedSight's "sky".)
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
				stepped := steppedEyes(eyes)

				for i := 0; i < b.N; i++ {
					if i%2 == 0 {
						f.Update(size.Width, size.Height, eyes)
					} else {
						f.Update(size.Width, size.Height, stepped)
					}
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
				stepped := steppedEyes(eyes)

				// Every eye steps a tile each iteration (F4: a dial write is a
				// cached frame since the line cache).
				for i := 0; i < b.N; i++ {
					if i%2 == 0 {
						f.Update(size.Width, size.Height, eyes)
					} else {
						f.Update(size.Width, size.Height, stepped)
					}
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

// BenchmarkFogWalkWithATorch (the F2 review's B4): one FRAME of a walk at
// deep night with his torch lit -- he moves 0.07 tiles a frame (about 4 tiles
// a second at 60 frames) -- with 0, 4 or 16 hearths about the start. The
// carried disc is keyed by its tile, so a frame recomputes only when he steps
// onto another tile (recomputes/op is the share of frames that do).
func BenchmarkFogWalkWithATorch(b *testing.B) {
	engine, m := villageEngine(b)
	size := engine.Size()

	for _, hearths := range []int{0, 4, 16} {
		b.Run(fmt.Sprintf("hearths%d", hearths), func(b *testing.B) {
			clock := d2world.NewClock(d2world.DefaultClockDials())
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
			f := d2world.NewFog(d2world.DefaultFogDials(), engine)
			f.SetLight(view)

			r0, _, _ := f.Counters()

			for i := 0; i < b.N; i++ {
				// Back and forth over 6 tiles east of the start.
				step := float64(i % 85)
				if (i/85)%2 == 1 {
					step = 85 - step
				}

				x, y := m.StartX+0.5+0.07*step, m.StartY+0.5
				view.SetCarriedAt(x, y)
				f.Update(size.Width, size.Height, []d2world.Eye{{ID: "s:1", X: x, Y: y}})
			}

			r1, _, _ := f.Counters()
			b.ReportMetric(float64(r1-r0)/float64(b.N), "recomputes/op")
		})
	}
}

// TestFogOnTheVillageSeesHisSurroundings: the fog over the real village from
// the start, at day sight 12, sees a good share of the 12-tile disc (452
// tiles) -- the houses hide some -- and the start tile; and somewhere within
// 11 tiles a house or wall hides what stands behind it (the map really blocks).
func TestFogOnTheVillageSeesHisSurroundings(t *testing.T) {
	engine, m := villageEngine(t)
	size := engine.Size()

	// His eye alone: since F4 the gate's tower is an eye too, and sees 16.
	f := d2world.NewFog(d2world.DefaultFogDials(), hisOwnSight{engine})
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

// hisOwnSight is the village's line of sight and footprints without its
// towers or heights: what his eye sees alone (F4 made the gate's tower an eye).
type hisOwnSight struct{ e *d2mapengine.MapEngine }

func (s hisOwnSight) TileSightClear(fx, fy float64, tx, ty int) (bool, int) {
	return s.e.TileSightClear(fx, fy, tx, ty)
}

func (s hisOwnSight) Structures() []image.Rectangle { return s.e.Structures() }

// steppedEyes are the eyes one tile east: a frame on which every eye steps.
func steppedEyes(eyes []d2world.Eye) []d2world.Eye {
	out := append([]d2world.Eye(nil), eyes...)
	for i := range out {
		out[i].X++
	}

	return out
}

// raisedEyes are n eyes about the village's start as F4 raises them: every
// third carries a bow (+2) and every fourth stands on the churchyard's high
// ground (height 1, +2), at day sight 12 or 16.
func raisedEyes(m *d2maptiled.Map, n int) []d2world.Eye {
	eyes := make([]d2world.Eye, n)

	for i := range eyes {
		a := 2 * math.Pi * float64(i) / float64(n)
		r := 3.0 * float64(i%3)
		eyes[i] = d2world.Eye{ID: fmt.Sprintf("e%d", i), X: m.StartX + 0.5 + r*math.Cos(a), Y: m.StartY + 0.5 + r*math.Sin(a)}

		if i%3 == 0 {
			eyes[i].Gear = 2
		}

		if i%4 == 0 {
			eyes[i].X, eyes[i].Y = 14.5+float64(i%6), 15.5+float64(i%3) // the churchyard
		}
	}

	return eyes
}

// BenchmarkFogRaisedSight (F4): the real village with its tower at the gate
// (sight 16) and its high ground, 1, 6 and 24 eyes raised as raisedEyes says,
// at day sight 12 and 16. Two frames:
//
//   - step: every eye steps a tile (east, then back) -- the worst frame, every
//     eye's lines walked afresh (the tower's stay cached);
//   - sky: nothing moves but the recompute is forced (a dial; at dusk, the
//     sky's 1/64 step) -- every line read from the cache.
//
// The budget is a worst frame of 0.5 ms (plan §3.8).
func BenchmarkFogRaisedSight(b *testing.B) {
	engine, m := villageEngine(b)
	size := engine.Size()

	for _, sight := range []float64{12, 16} {
		for _, n := range []int{1, 6, 24} {
			eyes := raisedEyes(m, n)
			stepped := steppedEyes(eyes)

			for _, frame := range []string{"step", "sky"} {
				b.Run(fmt.Sprintf("%s/sight%.0f/eyes%d", frame, sight, n), func(b *testing.B) {
					dials := d2world.DefaultFogDials()
					dials.DaySight = sight
					f := d2world.NewFog(dials, engine)
					f.Update(size.Width, size.Height, eyes)

					for i := 0; i < b.N; i++ {
						switch {
						case frame == "sky":
							f.SetDials(dials)
							f.Update(size.Width, size.Height, eyes)
						case i%2 == 0:
							f.Update(size.Width, size.Height, stepped)
						default:
							f.Update(size.Width, size.Height, eyes)
						}
					}

					_, visible := f.Counts()
					_, _, cells := f.Counters()
					b.ReportMetric(float64(visible), "visible")
					b.ReportMetric(float64(cells)/float64(b.N+1), "cells/op")
				})
			}
		}
	}
}

// TestTheVillageHasItsTowerAndHighGround (F4, Q7 on its default): the shipped
// village lays a tower at the gate -- tile (25, 34), seeing 16 -- and the
// church and cemetery's high ground at height 1 (G4's "cemetery on the high
// ground": every churchyard floor tile), into the engine, and fog makes the
// tower an eye.
//
// Negative control (2 Oct 2026, strigoi-harness-runs\wt-fog4\nc\): drop
// LayAuthoredMap's SetTowers and SetHeights and this fails, "the engine has
// 0 towers" (nc-no-towers-laid.txt).
func TestTheVillageHasItsTowerAndHighGround(t *testing.T) {
	engine, m := villageEngine(t)
	size := engine.Size()

	footprints, sight := engine.TowerSights()
	if len(footprints) != 1 || len(sight) != 1 {
		t.Fatalf("the engine has %d towers; the village has one, at the gate", len(footprints))
	}

	if footprints[0] != image.Rect(25, 34, 26, 35) || sight[0] != 16 {
		t.Fatalf("the tower is %v seeing %v; want (25,34) seeing 16", footprints[0], sight[0])
	}

	high, churchyard := 0, 0

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if m.Kinds[m.At(x, y).Floor].Name == "village-placeholder#4" {
				churchyard++

				if engine.HeightAt(x, y) != 1 {
					t.Errorf("the churchyard tile (%d,%d) is at height %d", x, y, engine.HeightAt(x, y))
				}
			}

			if engine.HeightAt(x, y) > 0 {
				high++
			}
		}
	}

	if churchyard == 0 || high != churchyard {
		t.Fatalf("%d tiles are high ground and %d are churchyard; want the churchyard, all of it", high, churchyard)
	}

	if h := engine.HeightAt(int(m.StartX), int(m.StartY)); h != 0 {
		t.Fatalf("the start is at height %d", h)
	}

	f := d2world.NewFog(d2world.DefaultFogDials(), engine)
	f.Update(size.Width, size.Height, []d2world.Eye{{ID: "s:1", X: 15.5, Y: 17.5}})

	towers, him := 0, d2world.EyeSight{}

	for _, e := range f.EyeSights() {
		if e.Tower {
			towers++
		} else {
			him = e
		}
	}

	if towers != 1 || him.Height != 1 || him.Sight != 14 {
		t.Fatalf("%d tower eyes; on the churchyard he is at height %d seeing %v; want 1, 1, 14", towers, him.Height, him.Sight)
	}
}

// TestTheLineCacheSeesWhatAFreshFogSees (F4's cost answer, its correctness):
// on the real village, six eyes and the tower walk and the day sight moves
// (12, 16, 9) over 60 recomputes; after each, the cached fog's visible set is
// exactly a fresh fog's -- one that has walked every line anew.
//
// Negative control (2 Oct 2026, strigoi-harness-runs\wt-fog4\nc\): never
// reset an eye's cache when it changes tile (linesFor) and this fails, "after
// step 1 tile (12,0) is visible to the cached fog false, to a fresh one true"
// (nc-cache-never-reset.txt).
func TestTheLineCacheSeesWhatAFreshFogSees(t *testing.T) {
	engine, m := villageEngine(t)
	size := engine.Size()
	eyes := raisedEyes(m, 6)
	dials := d2world.DefaultFogDials()
	cached := d2world.NewFog(dials, engine)

	for step := 0; step < 60; step++ {
		for i := range eyes {
			a := float64(step*7+i*13) * 0.37
			eyes[i].X += 0.6 * math.Cos(a)
			eyes[i].Y += 0.6 * math.Sin(a)
			eyes[i].X = math.Max(1, math.Min(46, eyes[i].X))
			eyes[i].Y = math.Max(1, math.Min(46, eyes[i].Y))
		}

		dials.DaySight = []float64{12, 16, 9}[step%3]
		cached.SetDials(dials)
		cached.Update(size.Width, size.Height, eyes)

		fresh := d2world.NewFog(dials, engine)
		fresh.Update(size.Width, size.Height, eyes)

		for ty := 0; ty < size.Height; ty++ {
			for tx := 0; tx < size.Width; tx++ {
				_, vc := cached.FogAt(tx, ty)
				_, vf := fresh.FogAt(tx, ty)

				if vc != vf {
					t.Fatalf("after step %d tile (%d,%d) is visible to the cached fog %v, to a fresh one %v", step, tx, ty, vc, vf)
				}
			}
		}
	}

	if cached.LinesCached() == 0 {
		t.Fatal("the cached fog read no line from its cache")
	}

	_, _, cachedCells := cached.Counters()
	t.Logf("60 recomputes: %d lines read from the cache, %d cells walked", cached.LinesCached(), cachedCells)
}

// walkedSight is the village's sight, towers and heights WITHOUT its
// TileBlocksSight: a fog over it walks the map's own TileSightClear for every
// line, as fog did before F4 read the map's opacity into a grid of its own.
type walkedSight struct{ e *d2mapengine.MapEngine }

func (s walkedSight) TileSightClear(fx, fy float64, tx, ty int) (bool, int) {
	return s.e.TileSightClear(fx, fy, tx, ty)
}

func (s walkedSight) Structures() []image.Rectangle { return s.e.Structures() }
func (s walkedSight) HeightAt(tx, ty int) int       { return s.e.HeightAt(tx, ty) }

func (s walkedSight) TowerSights() ([]image.Rectangle, []float64) { return s.e.TowerSights() }

// TestFogsGridWalksTheMapsLines (F4's cost answer, its correctness): fog's
// own opacity grid, walked by d2geom.TileLineClear, sees exactly what the
// map's TileSightClear sees -- on the real village, 24 eyes stepping over 40
// recomputes at day sights 16 and 9 and at deep night with four hearths lit,
// every tile's state equal and every line's cell count equal.
//
// Negative control (2 Oct 2026, strigoi-harness-runs\wt-fog4\nc\): read
// fog's grid one row off (opaque[((y+1)%h)*w+x]) and this fails, "night
// false, step 0: tile (12,0) is unexplored to the grid's fog and visible to
// the map's" (nc-opacity-row-off.txt).
func TestFogsGridWalksTheMapsLines(t *testing.T) {
	engine, m := villageEngine(t)
	size := engine.Size()
	eyes := raisedEyes(m, 24)

	clock := d2world.NewClock(d2world.DefaultClockDials())
	defer clock.Close()

	clock.SetMoon(0)

	light := d2world.NewLight(clock, d2world.DefaultLightDials())
	defer light.Close()

	for i := 0; i < 4; i++ {
		a := 2 * math.Pi * float64(i) / 4
		light.Add(d2world.SourceHearth, true, m.StartX+0.5+11*math.Cos(a), m.StartY+0.5+11*math.Sin(a))
	}

	for _, night := range []bool{false, true} {
		dials := d2world.DefaultFogDials()
		grid, walked := d2world.NewFog(dials, engine), d2world.NewFog(dials, walkedSight{engine})

		if night {
			view := d2world.NewLightView(light)
			grid.SetLight(view)
			walked.SetLight(view)
		}

		for step := 0; step < 40; step++ {
			for i := range eyes {
				a := float64(step*5+i*11) * 0.53
				eyes[i].X = math.Max(1, math.Min(46, eyes[i].X+0.7*math.Cos(a)))
				eyes[i].Y = math.Max(1, math.Min(46, eyes[i].Y+0.7*math.Sin(a)))
			}

			dials.DaySight = []float64{16, 9}[step%2]
			grid.SetDials(dials)
			walked.SetDials(dials)
			grid.Update(size.Width, size.Height, eyes)
			walked.Update(size.Width, size.Height, eyes)

			for ty := 0; ty < size.Height; ty++ {
				for tx := 0; tx < size.Width; tx++ {
					if a, b := grid.At(tx, ty), walked.At(tx, ty); a != b {
						t.Fatalf("night %v, step %d: tile (%d,%d) is %v to the grid's fog and %v to the map's", night, step, tx, ty, a, b)
					}
				}
			}
		}

		_, _, gc := grid.Counters()
		_, _, wc := walked.Counters()

		if gc != wc || gc == 0 {
			t.Fatalf("night %v: the grid's walks read %d cells, the map's %d; one walk, one count", night, gc, wc)
		}

		t.Logf("night %v: 40 recomputes, %d cells walked by each, equal", night, gc)
	}
}
