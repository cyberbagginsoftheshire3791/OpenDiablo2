package d2mapengine

// The BUG-115 review's differential test, adopted: RouteQuery, PathFind and
// coarsePath against master 05ba3666's code verbatim (testdata/masterref,
// test-only), on random maps of six kinds, with the scratch's stamps and the
// marks' base pushed forward to the edge of wrapping between queries.
// REV_SEEDS / REV_CSEEDS / REV_BIG / REV_ONLY widen it (the review ran 420k+
// PathFinds this way); the default is the gate's size.

import (
	"math"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2geom"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine/testdata/masterref"
)

type revMap struct {
	w, h    int // tiles
	blocked map[[2]int]bool
	short   int // tiles dropped off the end of the slice
	kind    string
	yards   [][4]int // interiors: x, y, w, h in subtiles
}

func (r *revMap) inYard(rng *rand.Rand) (d2vector.Position, bool) {
	if len(r.yards) == 0 {
		return d2vector.Position{}, false
	}

	y := r.yards[rng.Intn(len(r.yards))]

	return d2vector.NewPosition(float64(y[0]+rng.Intn(y[2]))+rng.Float64()*0.9, float64(y[1]+rng.Intn(y[3]))+rng.Float64()*0.9), true
}

func (r *revMap) b(x, y int) {
	if x < 0 || y < 0 || x >= r.w*5 || y >= r.h*5 {
		return
	}

	r.blocked[[2]int{x, y}] = true
}

func (r *revMap) box(x0, y0, x1, y1 int) { // hollow box, inclusive
	for x := x0; x <= x1; x++ {
		r.b(x, y0)
		r.b(x, y1)
	}

	for y := y0; y <= y1; y++ {
		r.b(x0, y)
		r.b(x1, y)
	}
}

func genRevMap(rng *rand.Rand, kind int) *revMap {
	r := &revMap{blocked: map[[2]int]bool{}}

	switch kind {
	case 0: // open field, any size, many ties
		r.kind = "open"
		r.w, r.h = 1+rng.Intn(30), 1+rng.Intn(30)
	case 1: // noise
		r.kind = "noise"
		r.w, r.h = 1+rng.Intn(25), 1+rng.Intn(25)
		p := []float64{0.05, 0.2, 0.35, 0.45, 0.6}[rng.Intn(5)]

		for y := 0; y < r.h*5; y++ {
			for x := 0; x < r.w*5; x++ {
				if rng.Float64() < p {
					r.b(x, y)
				}
			}
		}
	case 2: // pockets: sealed boxes, some leaking only diagonally, some with a door
		r.kind = "pockets"
		r.w, r.h = 8+rng.Intn(30), 8+rng.Intn(30)

		for n := 0; n < 1+rng.Intn(8); n++ {
			bw, bh := 3+rng.Intn(60), 3+rng.Intn(60)
			x0, y0 := rng.Intn(r.w*5), rng.Intn(r.h*5)
			x1, y1 := x0+bw, y0+bh
			r.box(x0, y0, x1, y1)

			switch rng.Intn(5) {
			case 0: // a diagonal-only leak at a corner: inner corner open to an outer diagonal
				// The corner opens to the outside; the inside meets it only
				// diagonally, across two wall subtiles.
				delete(r.blocked, [2]int{x0, y0})
			case 1: // a one-subtile door
				delete(r.blocked, [2]int{x0 + bw/2, y0})
			case 2: // a door that is a diagonal pair through a thick wall
				r.box(x0+1, y0+1, x1-1, y1-1)
				delete(r.blocked, [2]int{x0 + bw/2, y0})
				delete(r.blocked, [2]int{x0 + bw/2 + 1, y0 + 1})
			}
		}
	case 3: // long wall with a far gap, on a bigger map (corridor territory)
		r.kind = "longwall"
		r.w, r.h = 30+rng.Intn(25), 30+rng.Intn(25)
		wx := 20 + rng.Intn(r.w*5-40)
		gap := rng.Intn(r.h * 5)

		for y := 0; y < r.h*5; y++ {
			if y < gap || y > gap+rng.Intn(3) {
				r.b(wx, y)
			}
		}

		if rng.Intn(2) == 0 { // a sealed yard too
			x0, y0 := rng.Intn(r.w*5-30), rng.Intn(r.h*5-30)
			r.box(x0, y0, x0+5+rng.Intn(40), y0+5+rng.Intn(40))
		}
	case 4: // tile-blocks: whole tiles walled, authored-map style
		r.kind = "tiles"
		r.w, r.h = 5+rng.Intn(25), 5+rng.Intn(25)
		p := []float64{0.1, 0.3, 0.45}[rng.Intn(3)]

		for ty := 0; ty < r.h; ty++ {
			for tx := 0; tx < r.w; tx++ {
				if rng.Float64() < p {
					for k := 0; k < 25; k++ {
						r.b(tx*5+k%5, ty*5+k/5)
					}
				}
			}
		}
	}

	if kind == 5 { // yards on a big open map: the flood's territory
		r.kind = "yards"
		r.w, r.h = 30+rng.Intn(16), 30+rng.Intn(16)

		for n := 0; n < 3+rng.Intn(10); n++ {
			side := []int{4, 8, 15, 30, 44, 45, 46, 47, 60}[rng.Intn(9)] // interior ~ side^2 vs floodCap 2048
			bw, bh := side, side+rng.Intn(5)-2
			x0, y0 := rng.Intn(r.w*5-bw-2), rng.Intn(r.h*5-bh-2)
			r.box(x0, y0, x0+bw+1, y0+bh+1)

			switch rng.Intn(4) {
			case 0:
				delete(r.blocked, [2]int{x0, y0}) // corner open: diagonal-only, so still sealed
			case 1:
				delete(r.blocked, [2]int{x0 + 1 + rng.Intn(bw), y0}) // a door
			}

			r.yards = append(r.yards, [4]int{x0 + 1, y0 + 1, bw, bh})
		}
	}

	if revEnv("REV_BIG", 0) > 0 { // the 150x150 map's size, same features in its corner
		r.w, r.h = revEnv("REV_BIG", 0), revEnv("REV_BIG", 0)
	}

	if r.kind != "open" && rng.Intn(10) == 0 && r.h > 2 {
		r.short = 1 + rng.Intn(r.w)
	}

	return r
}

// install puts the map into an existing engine (keeping its scratch) and
// builds master's engine for it.
func (r *revMap) install(m *MapEngine) *masterref.MapEngine {
	m.size = d2geom.Size{Width: r.w, Height: r.h}
	m.tiles = make([]MapTile, r.w*r.h)
	ref := masterref.NewGrid(r.w, r.h)

	for k := range r.blocked {
		block(m, k[0], k[1])
		ref.BlockSub(k[0], k[1])
	}

	if r.short > 0 {
		m.tiles = m.tiles[:len(m.tiles)-r.short]
		ref.TruncateTiles(r.short)
	}

	return ref
}

func revEnv(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}

	return def
}

func revPos(rng *rand.Rand, r *revMap) d2vector.Position {
	// subtile position, sometimes off the map, never exactly a subtile centre
	fx := rng.Float64()
	if fx == 0.5 {
		fx = 0.25
	}

	fy := rng.Float64()
	if fy == 0.5 {
		fy = 0.75
	}

	x := rng.Intn(r.w*5+8) - 4
	y := rng.Intn(r.h*5+8) - 4

	return d2vector.NewPosition(float64(x)+fx, float64(y)+fy)
}

func revTile(p d2vector.Position) (int, int) {
	w := p.World()
	return int(math.Floor(w.X())), int(math.Floor(w.Y()))
}

func TestRevQueryIsMaster(t *testing.T) {
	seeds := revEnv("REV_SEEDS", 300)
	base := int64(revEnv("REV_BASE", 1))
	m := &MapEngine{}

	var paths, proven, provenTile, reached, wraps, byBlocked, bySealedIn, byFlood, bigs int

	defer func() {
		t.Logf("dest-tile proofs: goal blocked %d, start sealed in %d, by flood/marks %d; queries with a too-big flood %d", byBlocked, bySealedIn, byFlood, bigs)
	}()

	kinds := map[string]int{}

	for seed := base; seed < base+int64(seeds); seed++ {
		rng := rand.New(rand.NewSource(seed * 7919))
		kind := int(seed % int64(revEnv("REV_KINDS", 6)))
		if o := revEnv("REV_ONLY", -1); o >= 0 {
			kind = o
		}
		r := genRevMap(rng, kind)
		ref := r.install(m)
		kinds[r.kind]++

		queries := 2 + rng.Intn(4)
		if r.kind == "longwall" {
			queries = 2
		}

		for qi := 0; qi < queries; qi++ {
			// After the scratch has been used on this map, push its stamps
			// to the edge of wrapping, so the low stamps already written
			// would collide if a wrap failed to clear them.
			if qi > 0 && rng.Intn(3) == 0 {
				// Only ever FORWARD, as the code moves them: a stamp or a
				// mark base moved backward collides by construction.
				s := m.acquireScratch()
				if v := math.MaxUint32 - uint32(rng.Intn(3)); v > s.stamp {
					s.stamp = v
				}
				if v := math.MaxUint32 - uint32(rng.Intn(2)); v > s.tileStamp {
					s.tileStamp = v
				}
				if v := math.MaxUint32 - 2*markSpan - uint32(rng.Intn(markSpan)); s.marks != nil && v > s.markBase {
					s.markBase = v
				}
				m.releaseScratch(s)
				wraps++
			}

			start := revPos(rng, r)
			if rng.Intn(4) == 0 && len(r.blocked) > 0 { // start ON a wall
				for k := range r.blocked {
					start = d2vector.NewPosition(float64(k[0])+0.3, float64(k[1])+0.6)
					break
				}
			}

			if p, ok := r.inYard(rng); ok && rng.Intn(5) == 0 {
				start = p // a hunter in a yard
			}

			q := m.NewRouteQuery(start)
			nd := 3 + rng.Intn(10)
			if r.kind == "longwall" || r.kind == "yards" {
				nd = 2 + rng.Intn(4)
			}

			// a quarry, and its eight neighbours in a random order, plus strays
			qp := revPos(rng, r)
			if p, ok := r.inYard(rng); ok && rng.Intn(5) < 3 {
				qp = p
			}
			dests := []d2vector.Position{}
			for _, n := range rng.Perm(8) {
				o := besideOffsets[n]
				dests = append(dests, d2vector.NewPosition(qp.X()+o[0]*5, qp.Y()+o[1]*5))
			}
			dests = append(dests, qp)
			for i := 0; i < nd; i++ {
				switch rng.Intn(5) {
				case 0:
					dests = append(dests, start) // start == goal
				case 1:
					dests = append(dests, d2vector.NewPosition(start.X()+rng.Float64()*4-2, start.Y()+rng.Float64()*4-2))
				default:
					dests = append(dests, revPos(rng, r))
				}
			}

			for di, dest := range dests {
				tx, ty := revTile(dest)
				// Also an arbitrary tile near the dest, which the proof must
				// also only call when true.
				ax, ay := tx+rng.Intn(3)-1, ty+rng.Intn(3)-1

				pu := q.ProvenUnreachable(dest, tx, ty)
				pa := q.ProvenUnreachable(dest, ax, ay)

				if pu {
					g := subTile{int(math.Floor(dest.X())), int(math.Floor(dest.Y()))}
					switch {
					case m.blockedAt(g.x, g.y):
						byBlocked++
					case q.sealedIn:
						bySealedIn++
					default:
						byFlood++
					}
				}

				if q.base != 0 {
					for _, v := range q.s.marks[:0] {
						_ = v
					}
				}

				want := ref.PathFind(start, dest)
				got := q.PathFind(dest)
				paths++

				if !reflect.DeepEqual(want, got) {
					t.Fatalf("seed %d (%s %dx%d short %d) q%d dest %d: start %v dest %v\nwant %v\ngot  %v",
						seed, r.kind, r.w, r.h, r.short, qi, di, start, dest, want, got)
				}

				// A fresh one-shot PathFind is master's too.
				if one := m.PathFind(start, dest); !reflect.DeepEqual(want, one) {
					t.Fatalf("seed %d one-shot PathFind differs", seed)
				}

				arrived := len(want) > 0 && want[len(want)-1] == dest

				for _, c := range []struct {
					proven bool
					x, y   int
				}{{pu, tx, ty}, {pa, ax, ay}} {
					if !c.proven {
						continue
					}

					proven++
					if c.x == tx && c.y == ty {
						provenTile++
					}

					if arrived {
						g := subTile{int(math.Floor(dest.X())), int(math.Floor(dest.Y()))}
						gi := g.y*q.s.width + g.x
						t.Logf("diag: sealedIn %v failed %v base %d markBase %d mark(goal)-base %d stamp %d tileStamp %d wraps-so-far %d searches %d",
							q.sealedIn, q.failed, q.base, q.s.markBase, int64(q.s.marks[gi])-int64(q.base), q.s.stamp, q.s.tileStamp, wraps, q.searches)
						q2 := m.NewRouteQuery(start)
						_ = q2.PathFind(dests[0])
						t.Logf("diag: a fresh query (after one PathFind) says %v", q2.ProvenUnreachable(dest, c.x, c.y))
						q2.Close()
						t.Fatalf("seed %d q%d dest %d: proven unreachable but master ARRIVES: start %v dest %v route %v", seed, qi, di, start, dest, want)
					}

					if len(want) > 0 {
						lx, ly := revTile(want[len(want)-1])
						if lx == c.x && ly == c.y {
							t.Fatalf("seed %d q%d dest %d: tile (%d,%d) proven unreachable but master's route ends in it: start %v dest %v route %v", seed, qi, di, c.x, c.y, start, dest, want)
						}
					}
				}

				if endsIn(want, dest.World().X(), dest.World().Y()) {
					reached++
				}
			}

			if q.base != 0 {
				for _, v := range q.s.marks {
					if v == q.base+markBig {
						bigs++
						break
					}
				}
			}

			q.Close()
		}
	}

	t.Logf("kinds %v; %d PathFinds identical (%d reached their tile); %d proofs (%d on the dest tile); %d stamp-wrap setups", kinds, paths, reached, proven, provenTile, wraps)
}

// coarsePath against master's, any sizes, same reused engine and scratch.
func TestRevCoarseIsMaster(t *testing.T) {
	m := &MapEngine{}
	n := 0

	for seed := int64(1); seed <= int64(revEnv("REV_CSEEDS", 200)); seed++ {
		rng := rand.New(rand.NewSource(seed * 31))
		r := genRevMap(rng, int(seed%int64(revEnv("REV_KINDS", 6))))
		ref := r.install(m)

		for i := 0; i < 30; i++ {
			a := subTile{rng.Intn(r.w+4) - 2, rng.Intn(r.h+4) - 2}
			b := subTile{rng.Intn(r.w+4) - 2, rng.Intn(r.h+4) - 2}

			want := ref.CoarsePathRef(a.x, a.y, b.x, b.y)
			gotT := m.coarsePath(a, b)

			var got [][2]int
			if gotT != nil {
				got = make([][2]int, len(gotT))
				for k, s := range gotT {
					got[k] = [2]int{s.x, s.y}
				}
			}

			if !reflect.DeepEqual(want, got) {
				t.Fatalf("seed %d %s %dx%d: %v -> %v\nwant %v\ngot  %v", seed, r.kind, r.w, r.h, a, b, want, got)
			}

			n++
		}
	}

	t.Logf("%d corridors identical", n)
}
