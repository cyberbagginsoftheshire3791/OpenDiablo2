package d2maprenderer

import (
	"fmt"
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2ds1"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
)

// FOG OF WAR, F1 (fog.go): the four render passes over a small map whose
// every tile has a floor, drawn onto a surface that records every push, pop
// and draw with the brightness and saturation it was drawn at.

// opSurface records the state-stack calls and draws made on it, in order.
type opSurface struct {
	log   []string
	cur   opState
	stack []opState
	draws []opDraw
}

type opState struct {
	x, y              int
	bright, sat       float64
	brightSet, satSet bool
}

type opDraw struct {
	src         d2interface.Surface
	x, y        int
	bright, sat float64
}

func newOpSurface() *opSurface { return &opSurface{cur: opState{bright: 1, sat: 1}} }

func (o *opSurface) push(op string) {
	o.stack = append(o.stack, o.cur)
	o.log = append(o.log, op)
}

func (o *opSurface) Renderer() d2interface.Renderer                     { return nil }
func (o *opSurface) Clear(color.Color)                                  {}
func (o *opSurface) DrawRect(int, int, color.Color)                     {}
func (o *opSurface) DrawLine(int, int, color.Color)                     {}
func (o *opSurface) DrawTextf(string, ...interface{})                   {}
func (o *opSurface) GetSize() (width, height int)                       { return 800, 600 }
func (o *opSurface) GetDepth() int                                      { return len(o.stack) }
func (o *opSurface) PushColor(color.Color)                              { o.push("C") }
func (o *opSurface) PushEffect(d2enum.DrawEffect)                       { o.push("E") }
func (o *opSurface) PushFilter(d2enum.Filter)                           { o.push("F") }
func (o *opSurface) PushSkew(float64, float64)                          { o.push("K") }
func (o *opSurface) PushScale(float64, float64)                         { o.push("Z") }
func (o *opSurface) ReplacePixels([]byte)                               {}
func (o *opSurface) Screenshot() *image.RGBA                            { return nil }
func (o *opSurface) RenderSection(d2interface.Surface, image.Rectangle) {}

func (o *opSurface) PushTranslation(x, y int) {
	o.push(fmt.Sprintf("T%d,%d", x, y))
	o.cur.x += x
	o.cur.y += y
}

func (o *opSurface) PushBrightness(b float64) {
	o.push(fmt.Sprintf("B%.4f", b))
	o.cur.bright = b
}

func (o *opSurface) PushSaturation(s float64) {
	o.push(fmt.Sprintf("S%.4f", s))
	o.cur.sat = s
}

func (o *opSurface) Pop() {
	o.cur = o.stack[len(o.stack)-1]
	o.stack = o.stack[:len(o.stack)-1]
	o.log = append(o.log, "P")
}

func (o *opSurface) PopN(n int) {
	for i := 0; i < n; i++ {
		o.Pop()
	}
}

func (o *opSurface) Render(src d2interface.Surface) {
	o.log = append(o.log, fmt.Sprintf("R%p", src))
	o.draws = append(o.draws, opDraw{src: src, x: o.cur.x, y: o.cur.y, bright: o.cur.bright, sat: o.cur.sat})
}

// fakeFog is a FogSampler over a fixed map of tile states; a tile it does not
// list is visible.
type fakeFog struct {
	explored map[[2]int]bool // listed: explored and not visible
	hidden   map[[2]int]bool // listed: unexplored
}

func (f *fakeFog) FogAt(tx, ty int) (explored, visible bool) {
	switch {
	case f.hidden[[2]int{tx, ty}]:
		return false, false
	case f.explored[[2]int{tx, ty}]:
		return true, false
	default:
		return true, true
	}
}

func (f *fakeFog) MemoryLook() (level, saturation float64) { return 0.45, 0.25 }

// flatLight is the same light on every tile.
type flatLight float64

func (l flatLight) Level(int, int) float64 { return float64(l) }

// fogEntity is a map entity that only stands on a tile and draws a picture.
type fogEntity struct {
	d2interface.MapEntity // nil: only what the passes call is answered

	id    string
	pos   d2vector.Position
	art   d2interface.Surface
	drawn int
}

func (e *fogEntity) ID() string                       { return e.id }
func (e *fogEntity) GetPosition() d2vector.Position   { return e.pos }
func (e *fogEntity) GetLayer() int                    { return 1 }
func (e *fogEntity) GetPositionF() (float64, float64) { w := e.pos.World(); return w.X(), w.Y() }
func (e *fogEntity) Render(target d2interface.Surface) {
	e.drawn++
	target.Render(e.art)
}

const fogMapSize = 8

// fogFixture is an 8x8 map whose every tile has a floor of style 1 except
// the tile marked, whose floor is style 2 (its own art, so its draw can be
// told apart), with the camera on the middle.
type fogFixture struct {
	mr           *MapRenderer
	art, special *paintSurface
}

func newFogFixture(t *testing.T, special [2]int) *fogFixture {
	t.Helper()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, asset)
	engine.ResetAuthoredMap(d2enum.RegionAct1Town, fogMapSize, fogMapSize)

	for y := 0; y < fogMapSize; y++ {
		for x := 0; x < fogMapSize; x++ {
			var floor d2ds1.Tile

			floor.Prop1 = 1
			floor.Style = 1

			if x == special[0] && y == special[1] {
				floor.Style = 2
			}

			engine.Tile(x, y).Components.Floors = []d2ds1.Tile{floor}
		}
	}

	fx := &fogFixture{art: blockArt(160, 80), special: blockArt(160, 80)}
	fx.mr = &MapRenderer{viewport: newTestViewport(0, 0), mapEngine: engine}
	fx.mr.setImageCacheRecord(1, 0, 0, 0, fx.art)
	fx.mr.setImageCacheRecord(2, 0, 0, 0, fx.special)

	cx, cy := fx.mr.viewport.WorldToOrtho(fogMapSize/2, fogMapSize/2)
	moveTestCamera(fx.mr.viewport, cx, cy)

	return fx
}

func (fx *fogFixture) render() *opSurface {
	target := newOpSurface()
	fx.mr.Render(target)

	return target
}

// floorDraws counts the draws of the ordinary floor art and returns the
// special tile's draws.
func (fx *fogFixture) floorDraws(o *opSurface) (ordinary int, special []opDraw) {
	for _, d := range o.draws {
		switch d.src {
		case fx.art:
			ordinary++
		case fx.special:
			special = append(special, d)
		}
	}

	return ordinary, special
}

// TestNoFogSamplerIsTheUnfoggedDraw: with no fog sampler the four passes make
// exactly the calls they made before fog -- one PushBrightness and one Pop
// per tile per pass, no saturation, every tile drawn -- and a sampler that
// says every tile is visible makes the identical sequence, call for call.
//
// Negative control (1 Oct 2026, strigoi-harness-runs\wt-fog\nc\): make
// pushTileView push saturation 1 before the light whenever there is no
// sampler and this fails, "with no sampler the passes pushed a saturation"
// (nc11-nil-pushes-saturation.txt).
func TestNoFogSamplerIsTheUnfoggedDraw(t *testing.T) {
	fx := newFogFixture(t, [2]int{3, 3})
	fx.mr.SetLightSampler(flatLight(0.5))

	off := fx.render()

	for _, op := range off.log {
		if op[0] == 'S' {
			t.Fatal("with no sampler the passes pushed a saturation")
		}
	}

	ordinary, special := fx.floorDraws(off)
	if ordinary+len(special) != fogMapSize*fogMapSize || len(special) != 1 {
		t.Fatalf("with no sampler %d+%d floors were drawn; the map has %d", ordinary, len(special), fogMapSize*fogMapSize)
	}

	brights := 0

	for _, op := range off.log {
		if op[0] == 'B' {
			brights++
		}
	}

	if brights != 4*fogMapSize*fogMapSize {
		t.Fatalf("with no sampler %d brightness pushes; one per tile per pass is %d", brights, 4*fogMapSize*fogMapSize)
	}

	fx.mr.SetFogSampler(&fakeFog{})
	allSeen := fx.render()

	if !reflect.DeepEqual(off.log, allSeen.log) {
		t.Fatalf("a sampler that sees every tile drew a different sequence: %d calls against %d", len(allSeen.log), len(off.log))
	}

	if off.GetDepth() != 0 || allSeen.GetDepth() != 0 {
		t.Fatalf("the passes left states pushed: %d, %d", off.GetDepth(), allSeen.GetDepth())
	}
}

// TestUnexploredTileIsNotDrawn: an unexplored tile's floor is not drawn and
// nothing at all is pushed for it; every other floor is.
//
// Negative control (1 Oct 2026): make tileDrawn answer true for every tile
// and this fails, "the unexplored tile (3,3) was drawn 1 time(s)"
// (nc12-unexplored-drawn.txt).
func TestUnexploredTileIsNotDrawn(t *testing.T) {
	fx := newFogFixture(t, [2]int{3, 3})
	fx.mr.SetFogSampler(&fakeFog{hidden: map[[2]int]bool{{3, 3}: true}})

	o := fx.render()

	ordinary, special := fx.floorDraws(o)
	if len(special) != 0 {
		t.Fatalf("the unexplored tile (3,3) was drawn %d time(s)", len(special))
	}

	if ordinary != fogMapSize*fogMapSize-1 {
		t.Fatalf("%d other floors drawn, want %d", ordinary, fogMapSize*fogMapSize-1)
	}

	if o.GetDepth() != 0 {
		t.Fatalf("the passes left %d states pushed", o.GetDepth())
	}
}

// TestRememberedTileIsGreyedByMin: an explored-but-unseen tile is drawn at
// saturation 0.25 and brightness min(its light, 0.45): by day 0.45, at night
// its own night level (0.2) -- not 0.2 x 0.45, which would darken it twice. A
// visible tile is drawn at its light and full colour.
//
// Negative control (1 Oct 2026): multiply the light by the memory level
// instead of taking the min and this fails at night, "at night the
// remembered tile is drawn at brightness 0.09000000000000001, want 0.2 (min,
// not product)"
// (nc13-multiply-not-min.txt).
func TestRememberedTileIsGreyedByMin(t *testing.T) {
	for _, c := range []struct {
		name  string
		light float64
		want  float64
	}{
		{"by day", 1, 0.45},
		{"at night", 0.2, 0.2},
	} {
		fx := newFogFixture(t, [2]int{3, 3})
		fx.mr.SetLightSampler(flatLight(c.light))
		fx.mr.SetFogSampler(&fakeFog{explored: map[[2]int]bool{{3, 3}: true}})

		o := fx.render()

		_, special := fx.floorDraws(o)
		if len(special) != 1 {
			t.Fatalf("%s the remembered tile was drawn %d times", c.name, len(special))
		}

		d := special[0]
		if d.bright != c.want {
			t.Errorf("%s the remembered tile is drawn at brightness %v, want %v (min, not product)", c.name, d.bright, c.want)
		}

		if d.sat != 0.25 {
			t.Errorf("%s the remembered tile is drawn at saturation %v, want 0.25", c.name, d.sat)
		}

		for _, other := range o.draws {
			if other.src == fx.art && (other.bright != c.light || other.sat != 1) {
				t.Fatalf("%s a visible tile is drawn at brightness %v saturation %v, want %v and 1",
					c.name, other.bright, other.sat, c.light)
			}
		}
	}
}

// TestNoEntityOnAnUnseenTile: with fog, a man on a visible tile is drawn; one
// on remembered ground and one on unexplored ground are not. Without fog all
// three are.
//
// Negative control (1 Oct 2026): make entityShown answer true and this fails,
// "the man on remembered ground (2,5) was drawn 1 time(s)"
// (nc14-entity-in-memory-drawn.txt).
func TestNoEntityOnAnUnseenTile(t *testing.T) {
	fx := newFogFixture(t, [2]int{-1, -1})

	art := blockArt(10, 20)
	seen := &fogEntity{id: "seen", pos: d2vector.NewPositionTile(4.5, 4.5), art: art}
	remembered := &fogEntity{id: "remembered", pos: d2vector.NewPositionTile(2.5, 5.5), art: art}
	unexplored := &fogEntity{id: "unexplored", pos: d2vector.NewPositionTile(6.5, 2.5), art: art}

	for _, e := range []*fogEntity{seen, remembered, unexplored} {
		fx.mr.mapEngine.AddEntity(e)
	}

	fx.render()

	if seen.drawn != 1 || remembered.drawn != 1 || unexplored.drawn != 1 {
		t.Fatalf("without fog the three men were drawn %d, %d, %d times; want 1 each",
			seen.drawn, remembered.drawn, unexplored.drawn)
	}

	fx.mr.SetFogSampler(&fakeFog{
		explored: map[[2]int]bool{{2, 5}: true},
		hidden:   map[[2]int]bool{{6, 2}: true},
	})

	fx.render()

	if seen.drawn != 2 {
		t.Errorf("the man on a visible tile (4,4) was not drawn")
	}

	if remembered.drawn != 1 {
		t.Errorf("the man on remembered ground (2,5) was drawn %d time(s)", remembered.drawn-1)
	}

	if unexplored.drawn != 1 {
		t.Errorf("the man on unexplored ground (6,2) was drawn %d time(s)", unexplored.drawn-1)
	}

	if !fx.mr.Shows(4.5, 4.5) || fx.mr.Shows(2.5, 5.5) || fx.mr.Shows(6.5, 2.5) {
		t.Error("Shows disagrees with what was drawn")
	}
}

// TestShowsWithoutFogIsAlwaysTrue: no sampler, everything is shown.
func TestShowsWithoutFogIsAlwaysTrue(t *testing.T) {
	mr := &MapRenderer{viewport: newTestViewport(0, 0)}

	if !mr.Shows(-100, 3.5) || mr.HasFog() {
		t.Fatal("a renderer with no fog hides something")
	}
}
