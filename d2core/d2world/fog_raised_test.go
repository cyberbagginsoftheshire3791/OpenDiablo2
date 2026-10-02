package d2world

import (
	"image"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2geom"
)

// FOG OF WAR F4, RAISED SIGHT (fog.go; Josh's ruling 4, 1 Oct 2026: talents,
// structures, gear / squad type and height all raise sight). Q6-Q9 are on the
// plan's recommended defaults, decided while Josh was away (his to overturn):
// a tower needs no garrison and sees at night as a squad does (Q6), Night
// Eyes widens his dark radius by 1 (Q8), an equipped composite bow gives +2
// (Q9), and height gives HeightTiles (2) a level.
//
// Negative controls (2 Oct 2026): each test names the break it was run
// against; the logs are in strigoi-harness-runs\wt-fog4\nc\ (nc.sh, every
// one red, then restored).

// raisedSight is an open map with ground heights and towers (Heights,
// Towered): every line is clear but those through a blocked tile on a row.
type raisedSight struct {
	openSight
	heights map[[2]int]int
	towers  []image.Rectangle
	sight   []float64
}

func (r *raisedSight) HeightAt(tx, ty int) int { return r.heights[[2]int{tx, ty}] }

func (r *raisedSight) TowerSights() ([]image.Rectangle, []float64) { return r.towers, r.sight }

// TestEachTermRaisesSight (the plan's F4 table): each term raises one eye's
// sight by a number the test chose, on open ground from (30.5, 30.5), where
// the tile (30+d, 30) is exactly d tiles off. Plain: 12 seen, 13 not. Gear
// +2 (Q9's bow): 14 seen, 15 not. Height 1 (HeightTiles 2): the same. A base
// of 16 (a tower's own): 16 seen, 17 not. And the eye reports each term.
//
// Negative controls: drop the gear term from the sight -- "gear +2: the tile
// 14 off (44,30) is unexplored" (nc-drop-gear.txt); drop the height term --
// "height 1: the tile 14 off ..." (nc-drop-height.txt); ignore Base --
// "base 16: the tile 16 off (46,30) is unexplored" (nc-drop-base.txt).
func TestEachTermRaisesSight(t *testing.T) {
	cases := []struct {
		name         string
		eye          Eye
		height       int
		seen, unseen int // tiles off, east
		sight        float64
	}{
		{"plain", Eye{ID: "s:1"}, 0, 12, 13, 12},
		{"gear +2", Eye{ID: "s:1", Gear: 2}, 0, 14, 15, 14},
		{"height 1", Eye{ID: "s:1"}, 1, 14, 15, 14},
		{"base 16", Eye{ID: "s:1", Base: 16}, 0, 16, 17, 16},
		{"all of them", Eye{ID: "s:1", Base: 16, Gear: 2}, 1, 20, 21, 20},
	}

	for _, c := range cases {
		sight := &raisedSight{heights: map[[2]int]int{{30, 30}: c.height}}
		f := NewFog(DefaultFogDials(), sight)

		e := c.eye
		e.X, e.Y = 30.5, 30.5
		f.Update(64, 64, []Eye{e})

		if st := f.At(30+c.seen, 30); st != FogVisible {
			t.Errorf("%s: the tile %d off (%d,30) is %v, want visible", c.name, c.seen, 30+c.seen, st)
		}

		if st := f.At(30+c.unseen, 30); st != FogUnexplored {
			t.Errorf("%s: the tile %d off (%d,30) is %v, want unexplored", c.name, c.unseen, 30+c.unseen, st)
		}

		es := f.EyeSights()
		if len(es) != 1 || es[0].Sight != c.sight || es[0].UnlitReach != c.sight || es[0].Height != c.height ||
			es[0].HeightTerm != 2*float64(c.height) || es[0].Gear != c.eye.Gear {
			t.Errorf("%s: the eye reports %+v; want sight %v (height %d)", c.name, es, c.sight, c.height)
		}
	}
}

// TestNightEyesWidensTheDark (Q8 on its default): at deep night under a new
// moon he sees 1.5 tiles; with Night Eyes' +1, 2.5 -- the tile (2, 1) off,
// 2.24 away, is seen with it and not without -- and a talent never makes the
// dark wider than his day sight.
//
// Negative control: drop DarkTalent from the dark radius and this fails,
// "with Night Eyes the tile 2.24 off (32,31) is explored" (nc-drop-dark-talent.txt).
func TestNightEyesWidensTheDark(t *testing.T) {
	sky := &fakeSky{sky: 1}
	f := nightFog(sky, &openSight{})

	f.Update(64, 64, []Eye{{ID: "s:1", X: 30.5, Y: 30.5}}) // noon: explore it all
	sky.sky = 0                                            // deep night, new moon
	f.Update(64, 64, []Eye{{ID: "s:1", X: 30.5, Y: 30.5}})

	if st := f.At(32, 31); st != FogExplored {
		t.Fatalf("without Night Eyes the tile 2.24 off (32,31) is %v at deep night; his dark radius is 1.5", st)
	}

	f.Update(64, 64, []Eye{{ID: "s:1", X: 30.5, Y: 30.5, DarkTalent: 1}})

	if st := f.At(32, 31); st != FogVisible {
		t.Fatalf("with Night Eyes the tile 2.24 off (32,31) is %v; his dark radius is 2.5", st)
	}

	if st := f.At(33, 30); st != FogExplored {
		t.Fatalf("with Night Eyes the tile 3 off (33,30) is %v; 2.5 does not reach it", st)
	}

	if es := f.EyeSights(); es[0].Dark != 2.5 || es[0].UnlitReach != 2.5 {
		t.Fatalf("the eye reports dark %v, unlit reach %v; want 2.5 and 2.5", es[0].Dark, es[0].UnlitReach)
	}

	f.Update(64, 64, []Eye{{ID: "s:1", X: 30.5, Y: 30.5, DarkTalent: 40}})

	if es := f.EyeSights(); es[0].UnlitReach != 12 {
		t.Fatalf("a dark radius of 41.5 reaches %v; never past his day sight, 12", es[0].UnlitReach)
	}
}

// TestATowerHoldsItsArea (Q6 on its default: no garrison): a 1x1 tower at
// (10, 10) seeing 16 is an eye of its own. By day it holds its disc while he
// walks away; at deep night, like a squad, it sees its dark radius and no
// further; a beacon lit on it (a hearth's disc about its tile) lets it see the
// lit ground by night.
//
// Negative control: drop the towers from the eyes (Update) and this fails,
// "by day the tile 14 off the tower (24,10) is unexplored" (nc-no-towers.txt).
func TestATowerHoldsItsArea(t *testing.T) {
	// He stands on the tower's row with a wall at (40,10) between: his own
	// lines to the tower's ground are blocked, so what is seen there is the
	// tower's.
	sight := &raisedSight{towers: []image.Rectangle{image.Rect(10, 10, 11, 11)}, sight: []float64{16}}
	sight.blocked = map[[2]int]bool{{40, 10}: true}
	sky := &fakeSky{sky: 1, band: 1}
	f := nightFog(sky, sight)

	f.Update(64, 64, []Eye{{ID: "s:1", X: 50.5, Y: 10.5}})

	if st := f.At(24, 10); st != FogVisible {
		t.Fatalf("by day the tile 14 off the tower (24,10) is %v; the tower sees 16", st)
	}

	if st := f.At(27, 10); st != FogUnexplored {
		t.Fatalf("by day the tile 17 off the tower (27,10) is %v; the tower sees 16", st)
	}

	towers := 0

	for _, e := range f.EyeSights() {
		if e.Tower {
			towers++

			if e.ID != "tower/10,10" || e.Sight != 16 || e.BaseTerm != 16 {
				t.Fatalf("the tower's eye is %+v", e)
			}
		}
	}

	if towers != 1 {
		t.Fatalf("%d tower eyes; the map has one tower", towers)
	}

	// He walks away; the tower holds its area.
	f.Update(64, 64, []Eye{{ID: "s:1", X: 60.5, Y: 10.5}})

	if st := f.At(24, 10); st != FogVisible {
		t.Fatalf("with him gone the tile 14 off the tower is %v; the tower holds it", st)
	}

	// Deep night: the tower sees its dark radius only.
	sky.sky, sky.band = 0, 0.1
	f.Update(64, 64, []Eye{{ID: "s:1", X: 60.5, Y: 10.5}})

	if st := f.At(11, 10); st != FogVisible {
		t.Fatalf("at night the tile beside the tower (11,10) is %v; its dark radius is 1.5", st)
	}

	if st := f.At(14, 10); st != FogExplored {
		t.Fatalf("at night the tile 4 off the tower (14,10) is %v; the tower sees no further than a squad", st)
	}

	// A beacon on it: a hearth's disc of 5 about its tile.
	disc, lit := litDisc(1, 10.5, 10.5, 5)
	sky.discs, sky.lit = []LitDisc{disc}, lit
	f.Update(64, 64, []Eye{{ID: "s:1", X: 60.5, Y: 10.5}})

	if st := f.At(14, 10); st != FogVisible {
		t.Fatalf("with its beacon lit the tile 4 off the tower (14,10) is %v; lit ground is seen", st)
	}

	if st := f.At(16, 10); st != FogExplored {
		t.Fatalf("with its beacon lit the dark tile 6 off (16,10) is %v", st)
	}
}

// TestALineIsWalkedOnceATile (F4's cost answer): a recompute for anything but
// an eye's tile -- the sky, a dial -- reads every line from the cache, and an
// eye that changes tile walks its own again; a tower walks its lines once.
//
// Negative control (2 Oct 2026, strigoi-harness-runs\wt-fog4\nc\): never
// read the cache (lineClear) and this fails, "a dial's recompute walked 993
// lines; every one was cached" (nc-cache-ignored.txt). Never resetting an
// eye's cache when it changes tile is TestTheLineCacheSeesWhatAFreshFogSees's
// control (d2mapgen, the real village).
func TestALineIsWalkedOnceATile(t *testing.T) {
	sight := &raisedSight{towers: []image.Rectangle{image.Rect(10, 10, 11, 11)}, sight: []float64{16}}
	f := NewFog(DefaultFogDials(), sight)

	f.Update(64, 64, []Eye{{ID: "s:1", X: 40.5, Y: 40.5}})
	first := sight.calls

	if first == 0 {
		t.Fatal("the first recompute walked no line")
	}

	d := f.Dials()
	d.DaySight = 11
	f.SetDials(d)
	f.Update(64, 64, []Eye{{ID: "s:1", X: 40.5, Y: 40.5}})

	if sight.calls != first {
		t.Fatalf("a dial's recompute walked %d lines; every one was cached", sight.calls-first)
	}

	if f.LinesCached() == 0 {
		t.Fatal("no line was read from the cache")
	}

	f.Update(64, 64, []Eye{{ID: "s:1", X: 41.5, Y: 40.5}})

	walked := sight.calls - first
	if walked == 0 || walked >= first {
		t.Fatalf("his step walked %d lines (the first recompute %d): his own again, not the tower's", walked, first)
	}
}

// pillarSight is a map whose every seventh tile on both axes blocks, walked
// by the map's own d2geom walk; with opaque it also says its tiles
// (OpaqueTiles), so fog replays its recorded walk on its own grid.
type pillarSight struct{ w, h int }

func (p pillarSight) TileBlocksSight(tx, ty int) bool {
	return tx < 0 || ty < 0 || tx >= p.w || ty >= p.h || (tx%7 == 3 && ty%7 == 3)
}

func (p pillarSight) TileSightClear(fx, fy float64, tx, ty int) (bool, int) {
	return d2geom.TileLineClear(fx, fy, tx, ty, p.TileBlocksSight)
}

// walkOnly is pillarSight without TileBlocksSight: fog walks the map's lines.
type walkOnly struct{ p pillarSight }

func (o walkOnly) TileSightClear(fx, fy float64, tx, ty int) (bool, int) {
	return o.p.TileSightClear(fx, fy, tx, ty)
}

// TestTheRecordedWalkOnAnyGrid (F4's cost answer, its edges): fog's recorded
// walk sees what the map's own walk sees on a grid too large for the ray
// table (600 x 600: rays kept in a map), and an eye standing off the grid's
// last row -- whose offsets the table cannot hold -- is answered by the map's
// walk rather than panicking.
//
// Negative control (2 Oct 2026, strigoi-harness-runs\wt-fog4\nc\): drop the
// off-grid fallback (lineClear's f.in(ex, ey)) and this panics, "index out of
// range" (nc-off-grid-eye.txt).
func TestTheRecordedWalkOnAnyGrid(t *testing.T) {
	p := pillarSight{600, 600}
	eyes := []Eye{{ID: "s:1", X: 100.5, Y: 200.5}, {ID: "s:2/m", X: 300.5, Y: 600.4}}

	grid, walked := NewFog(DefaultFogDials(), p), NewFog(DefaultFogDials(), walkOnly{p})
	grid.Update(600, 600, eyes)
	walked.Update(600, 600, eyes)

	if grid.rays != nil || grid.rayMap == nil {
		t.Fatalf("a 600 x 600 grid keeps its rays in a table of %d; want the map", len(grid.rays))
	}

	for ty := 180; ty < 600; ty++ {
		for tx := 80; tx < 320; tx++ {
			if a, b := grid.At(tx, ty), walked.At(tx, ty); a != b {
				t.Fatalf("tile (%d,%d) is %v to the recorded walk and %v to the map's", tx, ty, a, b)
			}
		}
	}

	_, ga := grid.Counts()
	_, gb := walked.Counts()
	_, _, ca := grid.Counters()
	_, _, cb := walked.Counters()

	if ga != gb || ca != cb || ga < 300 {
		t.Fatalf("visible %d / %d, cells %d / %d; one walk, one answer", ga, gb, ca, cb)
	}

	if st := grid.At(300, 599); st != FogVisible {
		t.Fatalf("the tile beside the off-grid eye, (300,599), is %v", st)
	}

	// The table's own path: a 48 x 48 grid, an eye standing off its last row
	// and seeing 60 -- to row 0, 48 rows up, an offset the table cannot hold.
	small := pillarSight{48, 48}
	off := []Eye{{ID: "s:1", X: 20.5, Y: 48.3}}
	far := DefaultFogDials()
	far.DaySight = 60
	grid, walked = NewFog(far, small), NewFog(far, walkOnly{small})
	grid.Update(48, 48, off)
	walked.Update(48, 48, off)

	if grid.rays == nil {
		t.Fatal("a 48 x 48 grid keeps no ray table")
	}

	for ty := 0; ty < 48; ty++ {
		for tx := 0; tx < 48; tx++ {
			if a, b := grid.At(tx, ty), walked.At(tx, ty); a != b {
				t.Fatalf("48 x 48, the eye off its last row: tile (%d,%d) is %v to fog and %v to the map's walk", tx, ty, a, b)
			}
		}
	}
}
