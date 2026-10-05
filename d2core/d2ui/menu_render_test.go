package d2ui

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2input"
)

// This renderer clamps empty surfaces, as Renderer.NewSurface does, and mirrors ebiten's
// additive translation and assigned scale. Its glyph draws come from real
// font assets, so a reported fitted rectangle cannot conceal unscaled spacing.
type menuRenderState struct {
	x, y   int
	sx, sy float64
}
type menuRenderDraw struct {
	source d2interface.Surface
	rect   image.Rectangle
}
type menuRenderSurface struct {
	d2interface.Surface
	renderer      *menuRenderRenderer
	width, height int
	state         menuRenderState
	stack         []menuRenderState
	draws         []menuRenderDraw
}
type menuRenderRenderer struct{ d2interface.Renderer }

func (r *menuRenderRenderer) NewSurface(w, h int) d2interface.Surface {
	w, h = max(1, w), max(1, h)
	return &menuRenderSurface{renderer: r, width: w, height: h, state: menuRenderState{sx: 1, sy: 1}}
}
func (s *menuRenderSurface) Renderer() d2interface.Renderer { return s.renderer }
func (s *menuRenderSurface) GetSize() (int, int)            { return s.width, s.height }
func (s *menuRenderSurface) GetDepth() int                  { return len(s.stack) }
func (s *menuRenderSurface) save()                          { s.stack = append(s.stack, s.state) }
func (s *menuRenderSurface) Pop() {
	s.state = s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
}
func (s *menuRenderSurface) PopN(n int) {
	for range n {
		s.Pop()
	}
}
func (s *menuRenderSurface) PushTranslation(x, y int)     { s.save(); s.state.x += x; s.state.y += y }
func (s *menuRenderSurface) PushScale(x, y float64)       { s.save(); s.state.sx = x; s.state.sy = y }
func (s *menuRenderSurface) PushColor(color.Color)        { s.save() }
func (s *menuRenderSurface) PushEffect(d2enum.DrawEffect) { s.save() }
func (s *menuRenderSurface) PushFilter(d2enum.Filter)     { s.save() }
func (*menuRenderSurface) ReplacePixels([]byte)           {}
func (*menuRenderSurface) DrawRect(int, int, color.Color) {}
func (s *menuRenderSurface) Render(src d2interface.Surface) {
	w, h := src.GetSize()
	s.draws = append(s.draws, menuRenderDraw{src, image.Rect(s.state.x, s.state.y, s.state.x+int(math.Ceil(float64(w)*s.state.sx)), s.state.y+int(math.Ceil(float64(h)*s.state.sy)))})
}

// Negative controls (4 Oct 2026): dropping the fake's clamp fails at 0x0;
// scaling its translations fails at x=13 rather than the renderer's x=16.
func TestMenuRenderInstrument(t *testing.T) {
	r := &menuRenderRenderer{}
	empty := r.NewSurface(0, 0)
	if w, h := empty.GetSize(); w != 1 || h != 1 {
		t.Fatalf("empty clamp %dx%d, want 1x1", w, h)
	}
	s := r.NewSurface(20, 20).(*menuRenderSurface)
	s.PushTranslation(10, 20)
	s.PushScale(.5, .5)
	s.PushTranslation(6, 8)
	s.Render(r.NewSurface(4, 6))
	s.PopN(3)
	if got := s.draws[0].rect; got != image.Rect(16, 28, 18, 31) {
		t.Fatalf("additive translation/assigned scale: %v", got)
	}
	if s.GetDepth() != 0 {
		t.Fatal("instrument stack does not balance")
	}
}

func menuRenderUI(t *testing.T, large bool) *UIManager {
	t.Helper()
	am, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}
	if large {
		dir := t.TempDir()
		if err = os.WriteFile(filepath.Join(dir, "fonts.json"), []byte(`{"default":{"face":"gofont:regular","size":128}}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err = am.AddSource(dir, types.AssetSourceFileSystem); err != nil {
			t.Fatal(err)
		}
		if err = am.UseFontSet("fonts.json"); err != nil {
			t.Fatal(err)
		}
	} else {
		if err = am.AddSource(filepath.Join("..", ".."), types.AssetSourceFileSystem); err != nil {
			t.Fatal(err)
		}
		if err = am.UseFontSet("data/strigoi/fonts/fonts.json"); err != nil {
			t.Fatal(err)
		}
	}
	return &UIManager{asset: am, renderer: &menuRenderRenderer{}, inputManager: d2input.NewInputManagerWithService(&menuInput{}), Logger: d2util.NewLogger()}
}

// Negative controls (4 Oct 2026): disabling fit overflows the 128px-font
// caret; disabling cache invalidation keeps the empty text; dropping the
// native SetText branch displays only "55"; removing the caret's text-end
// offset puts it at x=70 instead of x=143/default or x=209/128px.
func TestMenuTextboxRendersEmptyAndFittedText(t *testing.T) {
	for _, large := range []bool{false, true} {
		name := "default"
		if large {
			name = "128px"
		}
		t.Run(name, func(t *testing.T) {
			ui := menuRenderUI(t, large)
			tb := ui.NewMenuTextbox()
			tb.SetPosition(64, 272)
			tb.SetFilter("0123456789.")
			frame := ui.renderer.NewSurface(800, 600).(*menuRenderSurface)
			box := image.Rect(64, 272, 244, 300)
			for _, text := range []string{"", "255.255.255.255", "1", ""} {
				tb.SetText(text)
				tb.Activate()
				// Capture the caret's visible phase without assuming wall-clock phase.
				deadline := time.Now().Add(time.Second)
				caret := false
				for !caret && time.Now().Before(deadline) {
					frame.draws = nil
					tb.Render(frame)
					for _, draw := range frame.draws {
						if !draw.rect.In(box) {
							t.Fatalf("%q draw %v outside textbox %v", text, draw.rect, box)
						}
						if draw.source == tb.menuCaretSurface {
							caret = true
							wantX := 70 + int(math.Ceil(float64(tb.menuTextWidth)*tb.menuTextFit))
							if draw.rect.Min.X != wantX || draw.rect.Min.Y != 275 {
								t.Fatalf("caret %v, want origin %d,275", draw.rect, wantX)
							}
						}
					}
					if !caret {
						time.Sleep(time.Millisecond)
					}
				}
				if !caret {
					t.Fatalf("%q caret never drawn", text)
				}
				if tb.GetText() != text || tb.textLabel.GetText() != text {
					t.Fatalf("text truncated: stored %q displayed %q want %q", tb.GetText(), tb.textLabel.GetText(), text)
				}
				wantWidth, _ := tb.textLabel.GetTextMetrics(text)
				if tb.menuText != text || tb.menuTextWidth != wantWidth {
					t.Fatalf("stale rendered text cache: %q width %d, want %q width %d", tb.menuText, tb.menuTextWidth, text, wantWidth)
				}
				if frame.GetDepth() != 0 {
					t.Fatal("textbox leaked surface state")
				}
			}
		})
	}
}
