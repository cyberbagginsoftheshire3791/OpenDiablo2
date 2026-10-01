package d2player

import (
	"path/filepath"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
)

// THE CALL SITES AT 0.5 (the zoom review, B3/B4). view_scale_test.go pins the
// arithmetic; these drive the code that calls it -- the hover loop, the squad
// and tactical hit tests, the overhead bars -- through a real view-only map
// renderer at 0.5, with one 60 x 100 man standing at the middle of the screen.
//
// The probes: a point 40 px above his feet is inside his full-size sprite
// (which reaches 50 px up, and 55 with the pad) and outside his half-size one
// (25 px up, 30 with the pad), so a call site that measured him at 1.0 hits
// him there and a right one does not; a point 10 px above his feet is on him
// either way, the control that the probe is not simply missing everything.

const (
	siteW, siteH         = 60, 100
	siteTileX, siteTileY = 30, 20
)

// standingMan is a map entity on a whole tile, selectable, 60 x 100.
type standingMan struct {
	d2interface.MapEntity

	id  string
	pos d2vector.Position
}

func (m *standingMan) ID() string                     { return m.id }
func (m *standingMan) GetPosition() d2vector.Position { return m.pos }
func (m *standingMan) GetSize() (int, int)            { return siteW, siteH }
func (m *standingMan) Selectable() bool               { return true }
func (m *standingMan) Highlight()                     {}

func (m *standingMan) GetPositionF() (float64, float64) {
	w := m.pos.World()
	return w.X(), w.Y()
}

// siteAt05 is a map renderer at 0.5 with its camera on the man's feet, a map
// engine holding him, and his feet's screen point.
func siteAt05(t *testing.T) (*d2maprenderer.MapRenderer, *d2mapengine.MapEngine, *standingMan, int, int) {
	t.Helper()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	if err := asset.AddSource(filepath.Join("..", ".."), types.AssetSourceFileSystem); err != nil {
		t.Fatal(err)
	}

	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, asset)
	engine.ResetAuthoredMap(d2enum.RegionAct1Town, 64, 64)

	man := &standingMan{id: "man", pos: d2vector.NewPositionTile(siteTileX, siteTileY)}
	engine.AddEntity(man)

	mr := d2maprenderer.NewViewOnlyMapRenderer(0, 0)
	cx, cy := mr.WorldToOrtho(siteTileX, siteTileY)
	cam := d2vector.NewPosition(cx, cy)
	mr.SetCameraPosition(&cam)
	mr.SetScale(0.5)

	fx, fy := mr.WorldToScreen(siteTileX, siteTileY)

	return mr, engine, man, fx, fy
}

// TestTheHoverLoopHitsTheHalfSizeMan drives HUD.hoveredEntity.
//
// Negative control (1 Oct 2026, the review's M11/M12 -- the hover loop and the
// squad hit test share spriteUnder): measure the sprite at 1.0 in spriteUnder
// and this fails: "at 0.5 a point 40 px above his feet hovers man; his sprite
// is drawn 50 px tall" (nc21-spriteunder-scale1.txt).
func TestTheHoverLoopHitsTheHalfSizeMan(t *testing.T) {
	mr, engine, man, fx, fy := siteAt05(t)
	h := &HUD{mapEngine: engine, mapRenderer: mr}

	if got := h.hoveredEntity(fx, fy-10); got != d2interface.MapEntity(man) {
		t.Fatalf("the control: at 0.5 a point 10 px above his feet hovers %v, not him", got)
	}

	if got := h.hoveredEntity(fx, fy-40); got != nil {
		t.Fatalf("at 0.5 a point 40 px above his feet hovers %s; his sprite is drawn 50 px tall", got.ID())
	}

	// squadAtScreen's per-model test is the same function.
	if !spriteUnder(mr, man, fx, fy-10) || spriteUnder(mr, man, fx, fy-40) {
		t.Fatalf("spriteUnder, the squad hit test's: 10 px above %v, 40 px above %v; want true, false",
			spriteUnder(mr, man, fx, fy-10), spriteUnder(mr, man, fx, fy-40))
	}
}

// TestTheTacticalHitTestHitsTheHalfSizeMan drives tacticalSpriteHit: his
// sprite stands on his feet, 50 px tall at 0.5.
//
// Negative control (1 Oct 2026, the review's M9): drop its ScaleLength line and
// this fails: "at 0.5 a point 70 px above his feet hits his tactical sprite"
// (nc22-tactical-unscaled.txt).
func TestTheTacticalHitTestHitsTheHalfSizeMan(t *testing.T) {
	mr, _, man, fx, fy := siteAt05(t)

	if !tacticalSpriteHit(mr, man, fx, fy-40) {
		t.Fatal("the control: at 0.5 a point 40 px above his feet misses his tactical sprite (50 px tall)")
	}

	if tacticalSpriteHit(mr, man, fx, fy-70) {
		t.Fatal("at 0.5 a point 70 px above his feet hits his tactical sprite; it is drawn 50 px tall")
	}

	if tacticalSpriteHit(mr, man, fx+20, fy-10) {
		t.Fatal("at 0.5 a point 20 px right of his feet hits his tactical sprite; it is drawn 30 px wide")
	}
}

// TestTheOverheadBarHangsOverTheHalfSizeHead drives HUD.refreshOverheadBars:
// the bar's bottom sits the pad above his head as drawn -- 50 px up at 0.5,
// less his render offset (one sub-tile, scaled to 1 px) -- at its UI size.
//
// Negative control (1 Oct 2026, the review's M10): pass 1 for the scale at the
// bars' call site and this fails: "at 0.5 the bar is at x 379 y 189; over his
// drawn head it is at x 379 y 239" (nc23-bars-scale1.txt).
func TestTheOverheadBarHangsOverTheHalfSizeHead(t *testing.T) {
	mr, engine, _, fx, fy := siteAt05(t)
	h := &HUD{mapEngine: engine, mapRenderer: mr, bars: oneBar{}}

	h.refreshOverheadBars()

	if len(h.overheadBars) != 1 {
		t.Fatalf("%d bars projected, want 1", len(h.overheadBars))
	}

	b := h.overheadBars[0]
	wantY := fy - 1 - siteH/2 - hoverLabelOuterPad - overheadBarHeight
	wantX := fx - 1 - overheadBarWidth/2

	if b.y != wantY || b.x != wantX {
		t.Fatalf("at 0.5 the bar is at x %d y %d; over his drawn head it is at x %d y %d", b.x, b.y, wantX, wantY)
	}

	if b.w != overheadBarWidth || b.h != overheadBarHeight {
		t.Fatalf("at 0.5 the bar is %d x %d; it keeps its UI size %d x %d", b.w, b.h, overheadBarWidth, overheadBarHeight)
	}
}

type oneBar struct{}

func (oneBar) OverheadBars() []OverheadBar { return []OverheadBar{{Entity: "man", Cur: 5, Max: 10}} }
