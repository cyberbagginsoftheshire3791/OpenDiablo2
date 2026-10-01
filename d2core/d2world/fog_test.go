package d2world

import (
	"strings"
	"testing"
)

// FOG OF WAR, F1 (fog.go). The rule's distance and memory, on a fake line of
// sight; the line of sight itself is the map's, tested where it lives
// (d2mapengine/tile_sight_test.go).

// openSight is a map with nothing on it that blocks: every line is clear, and
// each read costs one cell. blocked lists tiles a line may not pass THROUGH.
type openSight struct {
	calls   int
	blocked map[[2]int]bool
}

func (o *openSight) TileSightClear(fx, fy float64, tx, ty int) (bool, int) {
	o.calls++

	// A straight row only, which is all these tests ask: the tiles strictly
	// between the eye's tile and the target on row ty.
	ex := tileOf(fx)
	if tileOf(fy) == ty {
		for x := ex + 1; x < tx; x++ {
			if o.blocked[[2]int{x, ty}] {
				return false, 1
			}
		}
	}

	return true, 1
}

func newTestFog(sight TileSight) *Fog {
	return NewFog(DefaultFogDials(), sight)
}

// TestFogSeesItsRadius: day sight is Josh's 12 tiles (Q1). A tile whose
// centre is 11.9 tiles from the eye is visible; one 12.1 away is not, nor
// explored.
//
// Negative control (1 Oct 2026, strigoi-harness-runs\wt-fog\nc\): make the
// rule's radius DaySight+1 and this fails, "the tile 12.1 tiles off (8,10) is
// visible; day sight is 12" (nc8-radius-plus-one.txt).
func TestFogSeesItsRadius(t *testing.T) {
	f := newTestFog(&openSight{})

	if f.Dials().DaySight != 12 {
		t.Fatalf("day sight ships at %v; Josh's Q1 is 12", f.Dials().DaySight)
	}

	// The eye at x 20.6 on row 10: tile 32's centre is 11.9 east, tile 8's
	// 12.1 west.
	f.Update(48, 48, []Eye{{ID: "s:1", X: 20.6, Y: 10.5}})

	if st := f.At(32, 10); st != FogVisible {
		t.Errorf("the tile 11.9 tiles off (32,10) is %v; day sight is 12", st)
	}

	if st := f.At(8, 10); st != FogUnexplored {
		t.Errorf("the tile 12.1 tiles off (8,10) is %v; day sight is 12", st)
	}

	if st := f.At(20, 10); st != FogVisible {
		t.Errorf("the eye's own tile is %v", st)
	}

	explored, visible := f.Counts()
	if explored != visible || visible < 400 || visible > 470 {
		t.Errorf("a 12-tile disc on open ground: %d visible, %d explored; want ~452 of each", visible, explored)
	}
}

// TestFogBlockedLineHidesTheTile: the rule asks the line of sight, and a
// blocked line hides a tile inside the radius.
func TestFogBlockedLineHidesTheTile(t *testing.T) {
	f := newTestFog(&openSight{blocked: map[[2]int]bool{{25, 10}: true}})
	f.Update(48, 48, []Eye{{ID: "s:1", X: 20.5, Y: 10.5}})

	if st := f.At(25, 10); st != FogVisible {
		t.Errorf("the blocker's own face (25,10) is %v", st)
	}

	if st := f.At(27, 10); st != FogUnexplored {
		t.Errorf("the tile behind the blocker (27,10) is %v", st)
	}
}

// TestExploredIsKept: he walks away; the ground he saw stays explored and
// stops being visible.
//
// Negative control (1 Oct 2026): clear the explored set at the start of every
// recompute and this fails, "after the walk the start tile (5,5) is
// unexplored; seen ground is remembered" (nc9-explored-cleared.txt).
func TestExploredIsKept(t *testing.T) {
	f := newTestFog(&openSight{})
	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 5.5}})

	if st := f.At(5, 5); st != FogVisible {
		t.Fatalf("the start tile is %v before the walk", st)
	}

	f.Update(48, 48, []Eye{{ID: "s:1", X: 40.5, Y: 40.5}})

	if st := f.At(5, 5); st != FogExplored {
		t.Fatalf("after the walk the start tile (5,5) is %v; seen ground is remembered", st)
	}

	if st := f.At(40, 40); st != FogVisible {
		t.Fatalf("where he stands now is %v", st)
	}

	explored, visible := f.Counts()
	if explored <= visible {
		t.Fatalf("explored %d is not more than visible %d after a walk", explored, visible)
	}
}

// TestFogRecomputesOnlyWhenSomethingChanged: an Update with the eyes on the
// same tiles reads nothing and counts as skipped; a step onto the next tile,
// a dial or a forget recomputes.
//
// Negative control (1 Oct 2026): make Update recompute on every call and this
// fails, "a step inside his tile recomputed (2 recomputes, 0 skipped, ...)"
// (nc10-recompute-every-frame.txt).
func TestFogRecomputesOnlyWhenSomethingChanged(t *testing.T) {
	sight := &openSight{}
	f := newTestFog(sight)

	f.Update(48, 48, []Eye{{ID: "s:1", X: 10.1, Y: 10.1}})
	calls := sight.calls

	f.Update(48, 48, []Eye{{ID: "s:1", X: 10.9, Y: 10.7}})

	if r, s, _ := f.Counters(); r != 1 || s != 1 || sight.calls != calls {
		t.Fatalf("a step inside his tile recomputed (%d recomputes, %d skipped, %d new reads)", r, s, sight.calls-calls)
	}

	f.Update(48, 48, []Eye{{ID: "s:1", X: 11.1, Y: 10.7}})

	if r, _, _ := f.Counters(); r != 2 {
		t.Fatalf("a step onto the next tile did not recompute (%d recomputes)", r)
	}

	d := f.Dials()
	d.DaySight = 5
	f.SetDials(d)
	f.Update(48, 48, []Eye{{ID: "s:1", X: 11.1, Y: 10.7}})

	if r, _, _ := f.Counters(); r != 3 {
		t.Fatalf("a dial change did not recompute (%d recomputes)", r)
	}

	if _, v := f.Counts(); v > 100 {
		t.Fatalf("at day sight 5 he sees %d tiles; a 5-tile disc is ~80", v)
	}
}

// TestFogVerbsGoBothWays: explore, forget and reveal_all each move the grid
// and can be undone (the harness's state verbs, rule 3).
func TestFogVerbsGoBothWays(t *testing.T) {
	f := newTestFog(&openSight{})
	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 5.5}})

	if n := f.Explore(40.5, 40.5, 3); n < 25 || f.At(40, 40) != FogExplored {
		t.Fatalf("explore 3 at (40,40) marked %d tiles, (40,40) is %v", n, f.At(40, 40))
	}

	f.Forget()

	if e, _ := f.Counts(); e != 0 || f.At(40, 40) != FogUnexplored {
		t.Fatalf("after forget %d tiles are explored, (40,40) is %v", e, f.At(40, 40))
	}

	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 5.5}})

	if f.At(5, 5) != FogVisible {
		t.Fatalf("the Update after forget does not see where he stands: %v", f.At(5, 5))
	}

	f.RevealAll()

	if e, _ := f.Counts(); e != 48*48 || f.At(47, 47) != FogExplored {
		t.Fatalf("reveal_all: %d explored, (47,47) is %v", e, f.At(47, 47))
	}

	rows := f.Rows()
	if len(rows) != 48 || rows[5][5] != '2' || rows[47][47] != '1' || strings.ContainsRune(strings.Join(rows, ""), '0') {
		t.Fatalf("the rows after reveal_all are not 1s and 2s: row 5 %q, row 47 %q", rows[5], rows[47])
	}
}

// TestFogOffTheGridIsNothing: a tile off the grid is neither explored nor
// visible, and fog over no map (before the first Update) answers so for every
// tile.
func TestFogOffTheGridIsNothing(t *testing.T) {
	f := newTestFog(&openSight{})

	if e, v := f.FogAt(0, 0); e || v {
		t.Fatal("fog over no map says (0,0) is seen")
	}

	if n := f.Explore(1, 1, 5); n != 0 {
		t.Fatalf("explore over no map marked %d tiles", n)
	}

	f.Update(10, 10, []Eye{{ID: "s:1", X: 0.5, Y: 0.5}})

	for _, p := range [][2]int{{-1, 0}, {0, -1}, {10, 0}, {0, 10}} {
		if e, v := f.FogAt(p[0], p[1]); e || v {
			t.Errorf("off the grid (%d,%d) is explored %v visible %v", p[0], p[1], e, v)
		}
	}
}
