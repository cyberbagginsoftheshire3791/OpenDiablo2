package d2mapgen

import (
	"errors"
	"image"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// A 3x1 authored map: open grass, a blocked house wall on grass, and a hole.
func authoredFixture() *d2maptiled.Map {
	floor := image.NewRGBA(image.Rect(0, 0, 160, 80))
	wall := image.NewRGBA(image.Rect(0, 0, 160, 230))

	return &d2maptiled.Map{
		Width: 3, Height: 1,
		Kinds: []d2maptiled.Kind{
			{Name: "v#0", Layer: d2maptiled.LayerFloor, Pixels: floor},
			{Name: "v#1", Layer: d2maptiled.LayerWall, Blocked: true, BlocksSight: true, Pixels: wall},
		},
		Cells: []d2maptiled.Cell{{Floor: 0, Wall: -1}, {Floor: 0, Wall: 1}, {Floor: -1, Wall: -1}},
	}
}

func TestAuthoredTileBuildsTheEngineTile(t *testing.T) {
	m := authoredFixture()

	open := authoredTile(m, 0, 0, nil)
	if len(open.Components.Floors) != 1 || len(open.Components.Walls) != 0 {
		t.Fatalf("open tile components %+v", open.Components)
	}

	f := open.Components.Floors[0]
	if f.Style != d2mapengine.AuthoredStyle || f.Sequence != 0 || f.Prop1 == 0 || f.Type != d2enum.TileFloor {
		t.Fatalf("floor %+v: want the reserved style, sequence 0, Prop1 set", f)
	}

	if !d2mapengine.IsAuthoredTile(&f) {
		t.Fatal("IsAuthoredTile false on an authored floor")
	}

	if open.RegionType != d2enum.RegionAct1Town {
		t.Fatalf("region %v, want Act 1 town", open.RegionType)
	}

	for i, s := range open.SubTiles {
		if s.BlockWalk || s.BlockLOS {
			t.Fatalf("open tile sub-tile %d blocked: %+v", i, s)
		}
	}

	house := authoredTile(m, 1, 0, nil)
	if len(house.Components.Walls) != 1 {
		t.Fatalf("house tile has %d walls", len(house.Components.Walls))
	}

	w := house.Components.Walls[0]
	if w.Type != d2mapengine.AuthoredWallType || !w.Type.UpperWall() || w.Type == d2enum.TileRoof {
		t.Fatalf("wall type %v: want an upper wall that is not a roof", w.Type)
	}

	// 230 tall: drawn 150 above the diamond's top so its bottom 80 sit on it.
	if w.YAdjust != 80-230 {
		t.Fatalf("wall YAdjust %d, want %d", w.YAdjust, 80-230)
	}

	for i, s := range house.SubTiles {
		if !s.BlockWalk || !s.BlockPlayerWalk || !s.BlockLOS {
			t.Fatalf("house sub-tile %d not blocked: %+v", i, s)
		}
	}

	hole := authoredTile(m, 2, 0, nil)
	if len(hole.Components.Floors) != 0 || !hole.SubTiles[12].BlockWalk {
		t.Fatalf("hole: floors %d, centre %+v; want no floor and blocked", len(hole.Components.Floors), hole.SubTiles[12])
	}

	// Control: a DT1 tile is not authored.
	var d2 = f
	d2.Style = 30
	if d2mapengine.IsAuthoredTile(&d2) {
		t.Fatal("IsAuthoredTile true on a style-30 tile")
	}
}

func TestAuthoredImagesKeyLikeTheRenderer(t *testing.T) {
	m := authoredFixture()
	images := authoredImages(m)

	if len(images) != 2 {
		t.Fatalf("%d images, want 2", len(images))
	}

	if images[d2mapengine.AuthoredKey{Sequence: 0, Type: d2enum.TileFloor}] != m.Kinds[0].Pixels {
		t.Fatal("floor art not under (0, floor)")
	}

	if images[d2mapengine.AuthoredKey{Sequence: 1, Type: d2mapengine.AuthoredWallType}] != m.Kinds[1].Pixels {
		t.Fatal("wall art not under (1, wall type)")
	}

	// Control: the wall is not also filed as a floor.
	if images[d2mapengine.AuthoredKey{Sequence: 1, Type: d2enum.TileFloor}] != nil {
		t.Fatal("wall art filed as a floor")
	}
}

func TestFindMonstat(t *testing.T) {
	a, b := &d2records.MonStatRecord{}, &d2records.MonStatRecord{}
	stats := d2records.MonStats{"warriv1": a, "Kashya": b}

	if findMonstat(stats, "warriv1") != a || findMonstat(stats, "Warriv1") != a || findMonstat(stats, "kashya") != b {
		t.Fatal("exact or case-folded lookup missed")
	}

	if findMonstat(stats, "gheed") != nil {
		t.Fatal("an absent id was found")
	}
}

func TestSubtile(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want int
	}{{0, 0}, {0.5, 2}, {3.2, 16}, {12.99, 64}} {
		if got := subtile(c.in); got != c.want {
			t.Errorf("subtile(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestSetAuthoredMapNormalisesAndForgets(t *testing.T) {
	t.Cleanup(func() { SetAuthoredMap("") })

	SetAuthoredMap(`data\strigoi\maps\village.tmj`)
	recordAuthored("/data/strigoi/maps/village.tmj", nil)

	asked, built, err := AuthoredMapReport()
	if asked != "/data/strigoi/maps/village.tmj" || built != asked || err != nil {
		t.Fatalf("report %q %q %v", asked, built, err)
	}

	recordAuthored(asked, errors.New("bad"))

	if _, built, err = AuthoredMapReport(); built != "" || err == nil {
		t.Fatalf("a refused build left built=%q err=%v", built, err)
	}

	// The client's success after the server's refusal does not erase it.
	recordAuthored(asked, nil)

	if _, built, err = AuthoredMapReport(); built != "" || err == nil || err.Error() != "bad" {
		t.Fatalf("a later success papered over the refusal: built=%q err=%v", built, err)
	}

	SetAuthoredMap("")

	if asked, built, err = AuthoredMapReport(); asked != "" || built != "" || err != nil {
		t.Fatalf("clearing left %q %q %v", asked, built, err)
	}
}

// A 2x2 structure is cut into four 80-pixel strips along its two front
// faces, each on its own tile, each drawn from the right height, and each
// carrying the right columns of the art; a fully transparent strip is left
// out.
func TestAStructureIsCutIntoStrips(t *testing.T) {
	art := image.NewRGBA(image.Rect(0, 0, 320, 200))
	for x := 0; x < 320; x++ {
		for y := 100; y < 200; y++ { // the top half stays transparent
			art.Pix[(y*320+x)*4], art.Pix[(y*320+x)*4+3] = byte(x/80+1), 255
		}
	}

	m := &d2maptiled.Map{
		Width: 4, Height: 4,
		Kinds:      []d2maptiled.Kind{{Name: "barn", Layer: d2maptiled.LayerStructure, Blocked: true, Footprint: image.Pt(2, 2), Pixels: art}},
		Structures: []d2maptiled.Structure{{Kind: 0, Footprint: image.Rect(1, 1, 3, 3)}},
	}

	walls, images := structureStrips(m)

	// Front tile 2,2. Left face: strip 0 on 1,2 (one step back, 40 px
	// higher), strip 1 on 2,2. Right face: strip 2 on 2,2, strip 3 on 2,1.
	want := []struct {
		at      image.Point
		typ     d2enum.TileType
		index   byte
		yAdjust int
	}{
		{image.Pt(1, 2), d2mapengine.AuthoredStripLeft, 0, 80 + 40 - 200},
		{image.Pt(2, 2), d2mapengine.AuthoredStripLeft, 1, 80 - 200},
		{image.Pt(2, 2), d2mapengine.AuthoredStripRight, 2, 80 - 200},
		{image.Pt(2, 1), d2mapengine.AuthoredStripRight, 3, 80 + 40 - 200},
	}

	count := 0
	for _, w := range walls {
		count += len(w)
	}

	if count != len(want) || len(images) != len(want) {
		t.Fatalf("%d strip walls and %d images, want %d each", count, len(images), len(want))
	}

	for _, c := range want {
		found := false

		for _, tile := range walls[c.at] {
			if tile.Type == c.typ && tile.RandomIndex == c.index {
				found = true

				if tile.YAdjust != c.yAdjust || tile.Style != d2mapengine.AuthoredStyle || tile.Sequence != 0 {
					t.Errorf("strip %d on %v: YAdjust %d style %d seq %d; want %d, authored, 0", c.index, c.at, tile.YAdjust, tile.Style, tile.Sequence, c.yAdjust)
				}
			}
		}

		if !found {
			t.Errorf("no strip %d (%v) on tile %v", c.index, c.typ, c.at)
			continue
		}

		img := images[d2mapengine.AuthoredKey{Sequence: 0, Type: c.typ, Index: c.index}]
		if img == nil || img.Bounds().Dx() != 80 || img.Bounds().Dy() != 200 {
			t.Fatalf("strip %d image %v", c.index, img)
		}

		// The strip holds art columns 80j..80j+79: their marker is j+1.
		if got := img.Pix[(150*80+40)*4]; got != c.index+1 {
			t.Errorf("strip %d carries column marker %d, want %d", c.index, got, c.index+1)
		}
	}

	// A transparent strip is not drawn at all.
	clear := image.NewRGBA(image.Rect(0, 0, 320, 200))
	m.Kinds[0].Pixels = clear

	if walls, images := structureStrips(m); len(walls) != 0 || len(images) != 0 {
		t.Fatalf("a transparent structure produced %d walls, %d images", len(walls), len(images))
	}
}

// A structure's whole art is never registered under its own key: it is drawn
// only as strips.
func TestAStructureKindIsNotAWholeImage(t *testing.T) {
	m := &d2maptiled.Map{Kinds: []d2maptiled.Kind{
		{Layer: d2maptiled.LayerStructure, Footprint: image.Pt(2, 2), Pixels: image.NewRGBA(image.Rect(0, 0, 320, 200))},
		{Layer: d2maptiled.LayerWall, Pixels: image.NewRGBA(image.Rect(0, 0, 160, 120))},
	}}

	images := authoredImages(m)
	if len(images) != 1 || images[d2mapengine.AuthoredKey{Sequence: 1, Type: d2mapengine.AuthoredWallType}] == nil {
		t.Fatalf("images %v; want only the wall", images)
	}
}
