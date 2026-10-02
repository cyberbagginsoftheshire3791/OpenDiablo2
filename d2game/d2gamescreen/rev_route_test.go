package d2gamescreen

// The BUG-115 review's differential test, adopted: the game's own
// mapRouter.Route and tacticalRoute against master 05ba3666's loops over
// master's PathFind (d2mapengine/testdata/masterref, master's code verbatim;
// test-only, see its export_rev.go). Controls G1 and G2 (the proof asked of
// the quarry's tile instead of the candidate's, in Route and in
// tacticalRoute) go red here.

import (
	"math"
	"math/rand"
	"reflect"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine/testdata/masterref"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
)

func revRouteExactRef(ref *masterref.MapEngine, fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	path := ref.PathFind(d2vector.NewPositionTile(fromX, fromY), d2vector.NewPositionTile(toX, toY))
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

func revBlockedRef(ref *masterref.MapEngine) func(x, y float64) bool {
	return func(x, y float64) bool {
		return ref.BlockedAt(int(math.Floor(x*subTilesPerTile)), int(math.Floor(y*subTilesPerTile)))
	}
}

// master's mapRouter.Route
func revRouteRef(ref *masterref.MapEngine, fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	for _, n := range unblockedNeighbours(fromX, fromY, toX, toY, revBlockedRef(ref)) {
		if beside, ok := revRouteExactRef(ref, fromX, fromY, toX+n[0], toY+n[1]); ok {
			return beside, true
		}
	}

	return revRouteExactRef(ref, fromX, fromY, toX, toY)
}

// master's tacticalRoute with no other bodies
func revTacticalRef(ref *masterref.MapEngine, fx, fy, tx, ty float64) ([][2]float64, int, bool) {
	for _, n := range unblockedNeighbours(fx, fy, tx, ty, revBlockedRef(ref)) {
		gx, gy := tx+n[0], ty+n[1]
		if math.Floor(gx) == math.Floor(fx) && math.Floor(gy) == math.Floor(fy) {
			return nil, 0, true
		}

		route, reachable := revRouteExactRef(ref, fx, fy, gx, gy)
		if !reachable {
			continue
		}

		return route, routeTiles(fx, fy, route), true
	}

	return nil, 0, false
}

func TestRevGameRouteIsMaster(t *testing.T) {
	e := &d2mapengine.MapEngine{}
	v := &Game{gameClient: &d2client.GameClient{MapEngine: e}}
	solves, reached := 0, 0

	for seed := int64(1); seed <= 400; seed++ {
		rng := rand.New(rand.NewSource(seed * 977))
		w, h := 6+rng.Intn(40), 6+rng.Intn(40)
		e.ResetAuthoredMap(d2enum.RegionAct1Town, w, h)
		ref := &masterref.MapEngine{}
		ref.ResetAuthoredMap(d2enum.RegionAct1Town, w, h)

		blk := func(x, y int) {
			if f := e.SubTileAt(x, y); f != nil {
				f.BlockWalk = true
				ref.SubTileAt(x, y).BlockWalk = true
			}
		}

		var yards [][4]int

		switch seed % 3 {
		case 0: // noise
			p := rng.Float64() * 0.45
			for y := 0; y < h*5; y++ {
				for x := 0; x < w*5; x++ {
					if rng.Float64() < p {
						blk(x, y)
					}
				}
			}
		default: // yards, sealed or with a door
			for n := 0; n < 2+rng.Intn(6); n++ {
				s := 5 + rng.Intn(50)
				if s+3 >= w*5 || s+3 >= h*5 {
					continue
				}
				x0, y0 := rng.Intn(w*5-s-2), rng.Intn(h*5-s-2)
				door := rng.Intn(3) == 0
				for k := 0; k <= s+1; k++ {
					if !(door && k == s/2) {
						blk(x0+k, y0)
					}
					blk(x0+k, y0+s+1)
					blk(x0, y0+k)
					blk(x0+s+1, y0+k)
				}
				yards = append(yards, [4]int{x0 + 1, y0 + 1, s, s})
			}
		}

		pos := func() (float64, float64) {
			if len(yards) > 0 && rng.Intn(2) == 0 {
				y := yards[rng.Intn(len(yards))]
				return (float64(y[0]+rng.Intn(y[2])) + rng.Float64()) / 5, (float64(y[1]+rng.Intn(y[3])) + rng.Float64()) / 5
			}
			return rng.Float64() * float64(w), rng.Float64() * float64(h)
		}

		for i := 0; i < 12; i++ {
			fx, fy := pos()
			tx, ty := pos()
			if rng.Intn(4) == 0 {
				tx, ty = fx+rng.Float64()*4-2, fy+rng.Float64()*4-2
			}

			want, wantOK := revRouteRef(ref, fx, fy, tx, ty)
			got, gotOK := mapRouter{engine: e}.Route(fx, fy, tx, ty)
			if wantOK != gotOK || !reflect.DeepEqual(want, got) {
				t.Fatalf("seed %d pair %d Route (%.3f,%.3f)->(%.3f,%.3f): want %v %v, got %v %v", seed, i, fx, fy, tx, ty, wantOK, want, gotOK, got)
			}

			wp, wc, wok := revTacticalRef(ref, fx, fy, tx, ty)
			gp, gc, gok := v.tacticalRoute("self", fx, fy, tx, ty)
			if wok != gok || wc != gc || !reflect.DeepEqual(wp, gp) {
				t.Fatalf("seed %d pair %d tacticalRoute: want %v %d %v, got %v %d %v", seed, i, wok, wc, wp, gok, gc, gp)
			}

			solves++
			if gotOK {
				reached++
			}
		}
	}

	t.Logf("%d Route and tacticalRoute solves identical to master (%d Routes reached)", solves, reached)
}
