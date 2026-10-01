package d2world

import (
	"image"
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

// TestFogSeesItsRadius: day sight is Josh's 12 tiles (Q1), measured from the
// centre of his tile to the centre of the other (the review's B1 and B3, 1
// Oct 2026). He stands at (20.3,20.9), on tile (20,20), so he sees from
// (20.5,20.5): tile (32,20) is 12.0 off and visible, (32,21) is 12.04 off and
// unexplored, and so on the west and south; the disc is the 441 tiles whose
// centre offsets (dx, dy) have dx^2 + dy^2 <= 144.
//
// Negative controls (1 Oct 2026, strigoi-harness-runs\wt-fog\nc\): the
// radius DaySight+1 -- "the tile 12.04 tiles off (32,21) is visible, want
// unexplored" (nc18-radius-plus-one-v2.txt; nc8 before the snap); the
// distance measured to the tile's CORNER (the reviewer's m04, the B3 pin) --
// the same tile, 11.5 off its corner, visible, and (8,20) unexplored
// (nc19-distance-to-corner.txt).
func TestFogSeesItsRadius(t *testing.T) {
	f := newTestFog(&openSight{})

	if f.Dials().DaySight != 12 {
		t.Fatalf("day sight ships at %v; Josh's Q1 is 12", f.Dials().DaySight)
	}

	f.Update(48, 48, []Eye{{ID: "s:1", X: 20.3, Y: 20.9}})

	for _, c := range []struct {
		x, y int
		d    string
		want FogTile
	}{
		{32, 20, "12.0", FogVisible}, {8, 20, "12.0", FogVisible}, {20, 32, "12.0", FogVisible},
		{32, 21, "12.04", FogUnexplored}, {8, 21, "12.04", FogUnexplored}, {21, 32, "12.04", FogUnexplored},
		{20, 20, "0 (his own tile)", FogVisible},
	} {
		if st := f.At(c.x, c.y); st != c.want {
			t.Errorf("the tile %s tiles off (%d,%d) is %v, want %v; day sight is 12", c.d, c.x, c.y, st, c.want)
		}
	}

	if explored, visible := f.Counts(); explored != 441 || visible != 441 {
		t.Errorf("a 12-tile disc on open ground: %d visible, %d explored; want 441 of each", visible, explored)
	}
}

// TestAnEyeSeesFromTheCentreOfItsTile (the review's B1, folding its probe
// TestRevStaleEyeWithinTile): fog skips every update while he stays on his
// tile, so what he sees is a function of his tile alone. Entering the tile at
// its top-left and walking to its bottom-right sees exactly what arriving
// straight at the bottom-right sees, and the eye is reported at the centre.
//
// Negative control (1 Oct 2026): keep the eye at his exact point (no snap)
// and this fails, "standing at the same point, what he sees depends on where
// he entered the tile: 66 tiles differ" (nc20-eye-not-snapped.txt).
func TestAnEyeSeesFromTheCentreOfItsTile(t *testing.T) {
	a := newTestFog(&openSight{})
	a.Update(48, 48, []Eye{{ID: "s:1", X: 20.01, Y: 20.01}}) // entered at the top-left
	a.Update(48, 48, []Eye{{ID: "s:1", X: 20.99, Y: 20.99}}) // walked to the bottom-right, same tile

	b := newTestFog(&openSight{})
	b.Update(48, 48, []Eye{{ID: "s:1", X: 20.99, Y: 20.99}}) // arrived straight there

	diff := 0

	for ty := 0; ty < 48; ty++ {
		for tx := 0; tx < 48; tx++ {
			if a.At(tx, ty) != b.At(tx, ty) {
				diff++
			}
		}
	}

	if diff != 0 {
		t.Fatalf("standing at the same point, what he sees depends on where he entered the tile: %d tiles differ", diff)
	}

	if e := a.Eyes(); len(e) != 1 || e[0].X != 20.5 || e[0].Y != 20.5 {
		t.Fatalf("the eye is reported at %+v; it sees from the centre of its tile, (20.5,20.5)", e)
	}
}

// TestForgetThenTheSameEyeSeesAgain (the review's C1): forget clears every
// explored tile, and the next update -- with the eye on the same tile, which
// would otherwise be skipped -- sees again, so the ground under him is drawn.
//
// Negative control (1 Oct 2026): drop Forget's dirty mark (the reviewer's
// m05) and this fails, "after forget and an update: 0 explored, 266 visible"
// -- the update was skipped, so nothing under him is explored and the
// renderer draws nothing there (nc21-forget-not-dirty.txt).
func TestForgetThenTheSameEyeSeesAgain(t *testing.T) {
	f := newTestFog(&openSight{})
	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 5.5}})
	f.Explore(40.5, 40.5, 2)
	f.Forget()

	if e, _ := f.Counts(); e != 0 || f.isExplored(40, 40) || f.isExplored(5, 5) {
		t.Fatalf("after forget %d tiles are explored (40,40 %v, 5,5 %v)", e, f.isExplored(40, 40), f.isExplored(5, 5))
	}

	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 5.5}})

	if st := f.At(5, 5); st != FogVisible {
		t.Fatalf("after forget and an update on the same tile, his own tile (5,5) is %v", st)
	}

	if e, v := f.Counts(); e != v || v < 100 {
		t.Fatalf("after forget and an update: %d explored, %d visible; the disc again and nothing else", e, v)
	}

	if f.isExplored(40, 40) {
		t.Fatal("the forgotten (40,40) came back")
	}
}

// TestANewMapSizeIsAFreshGrid (the review's C5, m10): an update on a map of
// another size starts a fresh, unexplored grid of that size.
//
// Negative control (1 Oct 2026): resize only when the grid is empty and this
// fails, "on a 20x20 map the grid is 48x48" (nc22-no-resize.txt).
func TestANewMapSizeIsAFreshGrid(t *testing.T) {
	f := newTestFog(&openSight{})
	f.Update(48, 48, []Eye{{ID: "s:1", X: 40.5, Y: 40.5}})
	f.Update(20, 20, []Eye{{ID: "s:1", X: 5.5, Y: 5.5}})

	if w, h := f.Size(); w != 20 || h != 20 {
		t.Fatalf("on a 20x20 map the grid is %dx%d", w, h)
	}

	if e, v := f.Counts(); e != v {
		t.Fatalf("the new map remembers %d tiles it never showed (%d explored, %d visible)", e-v, e, v)
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

// houseSight is a map with one house on it, footprint (25,9)-(28,12): an eye
// west of it sees the house's west column (its face) and nothing east of
// that column -- the house hides its own back, as a real footprint does.
type houseSight struct{ footprints []image.Rectangle }

func (h houseSight) TileSightClear(fx, fy float64, tx, ty int) (bool, int) {
	return tx <= 25 || tileOf(fx) > 27, 1
}

func (h houseSight) Structures() []image.Rectangle { return h.footprints }

// TestAHouseSeenFromBehindIsWhole: a house seen by one column of its footprint
// is visible whole (so all its art is drawn), and when he walks away it is
// remembered whole. A house he has never seen any of stays unexplored.
//
// Negative control (1 Oct 2026, strigoi-harness-runs\wt-fog\nc\): drop the
// footprint pass (wholeStructures does nothing) and this fails, "the house's
// far corner (26,9) is unexplored; a house seen at all is seen whole"
// (nc16-no-footprint-pass.txt).
func TestAHouseSeenFromBehindIsWhole(t *testing.T) {
	house := image.Rect(25, 9, 28, 12)
	far := image.Rect(40, 40, 43, 43)

	f := newTestFog(houseSight{footprints: []image.Rectangle{house, far}})
	f.Update(48, 48, []Eye{{ID: "s:1", X: 20.5, Y: 10.5}})

	if st := f.At(25, 10); st != FogVisible {
		t.Fatalf("the house's face (25,10) is %v", st)
	}

	for ty := house.Min.Y; ty < house.Max.Y; ty++ {
		for tx := house.Min.X; tx < house.Max.X; tx++ {
			if st := f.At(tx, ty); st != FogVisible {
				t.Fatalf("the house's far corner (%d,%d) is %v; a house seen at all is seen whole", tx, ty, st)
			}
		}
	}

	if st := f.At(26, 13); st != FogUnexplored {
		t.Fatalf("the ground behind the house (26,13) is %v; only the house is shown whole", st)
	}

	if st := f.At(41, 41); st != FogUnexplored {
		t.Fatalf("a house he never saw (41,41) is %v", st)
	}

	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 40.5}})

	for _, p := range [][2]int{{25, 10}, {27, 11}} {
		if st := f.At(p[0], p[1]); st != FogExplored {
			t.Fatalf("walked away, the house's tile (%d,%d) is %v; a house is remembered whole", p[0], p[1], st)
		}
	}

	if r, ok := f.StructureAt(27, 11); !ok || r != house {
		t.Fatalf("StructureAt(27,11) = %v %v, want the house", r, ok)
	}
}

// TestExploreRemembersAHouseWhole: the explore verb touching one tile of a
// house remembers it whole.
func TestExploreRemembersAHouseWhole(t *testing.T) {
	house := image.Rect(25, 9, 28, 12)

	f := newTestFog(houseSight{footprints: []image.Rectangle{house}})
	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 40.5}})
	f.Explore(25.5, 9.5, 0.1)

	if st := f.At(27, 11); st != FogExplored {
		t.Fatalf("explore on the house's corner left its far corner %v", st)
	}
}
