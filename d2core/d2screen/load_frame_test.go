package d2screen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2gui"
)

// loadingFlag is the screen manager's one answer the frame asks.
type loadingFlag struct{ loading bool }

func (l *loadingFlag) IsLoading() bool { return l.loading }

// frameSpy is the UI manager: it records each read of what a loading
// screen's OnLoad writes, and whether that read came while the screen was
// loading.
type frameSpy struct {
	screen Loading

	uiAdvances, uiRenders, inputAdvances int
	whileLoading                         []string
}

func (f *frameSpy) note(what string) {
	if f.screen.IsLoading() {
		f.whileLoading = append(f.whileLoading, what)
	}
}

func (f *frameSpy) Advance(float64) {
	f.uiAdvances++
	f.note("ui.Advance")
}

func (f *frameSpy) Render(d2interface.Surface) {
	f.uiRenders++
	f.note("ui.Render")
}

// inputSpy is the input manager's Advance, recorded on the same spy.
type inputSpy struct {
	d2interface.InputManager // nil: only Advance is called

	spy *frameSpy
}

func (i inputSpy) Advance(_, _ float64) error {
	i.spy.inputAdvances++
	i.spy.note("input.Advance")

	return nil
}

// TestNoUIOrInputFrameWhileAScreenLoads is BUG-109's ordering, frame by
// frame as the screen manager makes it: the frame that receives a loading
// screen's progress update runs while OnLoad, on its own goroutine, goes on
// appending widgets to the UI manager's list and handlers to the input
// manager's. That frame -- and every frame until the load's last update --
// must not range over either list. The panic (ui_manager.go:172, address
// 0x0) was the UI manager's Advance reading the widget list's header
// half-written by an append on the loader goroutine.
func TestNoUIOrInputFrameWhileAScreenLoads(t *testing.T) {
	screen := &loadingFlag{}
	spy := &frameSpy{screen: screen}
	input := inputSpy{spy: spy}

	frame := func() {
		if err := AdvanceUIAndInput(screen, spy, input, 1.0/60, 0); err != nil {
			t.Fatal(err)
		}

		RenderUI(screen, spy, nil)
	}

	// The control arm: a screen in play gets its UI and input every frame.
	frame()

	if spy.uiAdvances != 1 || spy.uiRenders != 1 || spy.inputAdvances != 1 {
		t.Fatalf("a screen in play: ui advanced %d, drawn %d, input %d; want 1, 1, 1 -- the gate is not the whole frame",
			spy.uiAdvances, spy.uiRenders, spy.inputAdvances)
	}

	// The screen changes: OnLoad runs, and three progress updates come in,
	// one a frame.
	screen.loading = true

	for i := 0; i < 3; i++ {
		frame()
	}

	if len(spy.whileLoading) > 0 {
		t.Fatalf("while a screen loaded the frame read what its OnLoad builds: %v (BUG-109)", spy.whileLoading)
	}

	// The load's last update is received: the first frame after it reads
	// everything OnLoad wrote.
	screen.loading = false

	frame()

	if spy.uiAdvances != 2 || spy.uiRenders != 2 || spy.inputAdvances != 2 {
		t.Fatalf("the first frame after the load: ui advanced %d, drawn %d, input %d in all; want 2, 2, 2",
			spy.uiAdvances, spy.uiRenders, spy.inputAdvances)
	}
}

// fakeUI and fakeGUI are the UI and GUI managers' halves the screen manager
// calls; they draw nothing.
type fakeUI struct{ resets int }

func (u *fakeUI) Reset() { u.resets++ }

type fakeGUI struct{}

func (fakeGUI) ShowLoadScreen(float64)  {}
func (fakeGUI) HideLoadScreen()         {}
func (fakeGUI) ShowCursor()             {}
func (fakeGUI) HideCursor()             {}
func (fakeGUI) SetLayout(*d2gui.Layout) {}

// buildingScreen is a screen whose OnLoad builds its widget list as the main
// menu's does: a progress update, then appends, a few times over. writing is
// true from the first line of OnLoad to its last -- set before the first
// send, cleared before LoadingState.Done's, so every frame that receives an
// update before the last sees it true, and every frame after the last sees
// it false (the channel orders both).
type buildingScreen struct {
	widgets []int
	writing atomic.Bool
}

func (b *buildingScreen) OnLoad(loading LoadingState) {
	b.writing.Store(true)

	for step := 0; step < 4; step++ {
		loading.Progress(float64(step) / 4)

		for i := 0; i < 64; i++ {
			b.widgets = append(b.widgets, i) // addWidget's append, off the update goroutine
		}
	}

	b.writing.Store(false)
}

// widgetReader is the UI manager's frame over the screen's widget list.
type widgetReader struct {
	screen *buildingScreen

	advances, renders int
	raced             []string
	seen              int
}

func (w *widgetReader) read(what string) {
	if w.screen.writing.Load() {
		w.raced = append(w.raced, what)
	}

	n := 0
	for range w.screen.widgets { // UIManager.Advance's range: a data race under -race if OnLoad still appends
		n++
	}

	w.seen = n
}

func (w *widgetReader) Advance(float64) {
	w.advances++
	w.read("ui.Advance")
}

func (w *widgetReader) Render(d2interface.Surface) {
	w.renders++
	w.read("ui.Render")
}

type nopInput struct{ d2interface.InputManager }

func (nopInput) Advance(_, _ float64) error { return nil }

// TestAScreensOnLoadNeverRacesTheFrame is BUG-109 on a real ScreenManager:
// its own goroutine runs the screen's OnLoad, its own channel hands the frame
// the progress updates, and the frame is the App's (AdvanceUIAndInput,
// RenderUI) over a UI that reads the widget list OnLoad appends to. No frame
// reads the list while OnLoad runs; the first frame after the load reads all
// of it. Removing the gate fails it deterministically (a frame reads while
// writing), and under -race (CI) as a data race on the list.
func TestAScreensOnLoadNeverRacesTheFrame(t *testing.T) {
	sm := NewScreenManager(nil, d2util.LogLevelNone, nil)
	ui := &fakeUI{}
	sm.uiManager = ui
	sm.guiManager = fakeGUI{}

	screen := &buildingScreen{}
	reader := &widgetReader{screen: screen}

	frame := func() {
		if err := sm.Advance(1.0 / 60); err != nil {
			t.Fatal(err)
		}

		if err := AdvanceUIAndInput(sm, reader, nopInput{}, 1.0/60, 0); err != nil {
			t.Fatal(err)
		}

		RenderUI(sm, reader, nil)
	}

	sm.SetNextScreen(screen)

	frames := 0
	for ; frames == 0 || (frames < 100 && sm.CurrentScreen() == nil); frames++ {
		frame()
	}

	if sm.CurrentScreen() != Screen(screen) {
		t.Fatalf("the screen never finished loading in %d frames", frames)
	}

	if ui.resets != 1 {
		t.Fatalf("the screen change reset the UI %d times, want 1", ui.resets)
	}

	if frames < 5 {
		t.Fatalf("the load took %d frames: want one per progress update (4) and the change -- the test is not exercising the load", frames)
	}

	if len(reader.raced) > 0 {
		t.Fatalf("a frame read the widget list while OnLoad was still building it: %v (BUG-109)", reader.raced)
	}

	// The control: the frame after the load reads everything OnLoad built.
	frame()

	if reader.advances == 0 || reader.renders == 0 || reader.seen != 4*64 {
		t.Fatalf("after the load: ui advanced %d, drawn %d, saw %d widgets; want >0, >0, %d",
			reader.advances, reader.renders, reader.seen, 4*64)
	}
}

// TestTheAppFrameGoesThroughTheGate is BUG-109's wiring: d2app's frame
// reaches the UI manager and the input manager only through the gate. The
// App cannot be built in a headless test (it links ebiten's UI), so this
// reads d2app's source: advanceOnce calls AdvanceUIAndInput, render calls
// RenderUI, and no file in d2app calls a.ui.Advance, a.ui.Render or
// a.inputManager.Advance directly.
func TestTheAppFrameGoesThroughTheGate(t *testing.T) {
	dir := filepath.Join("..", "..", "d2app")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	calledIn := map[string]map[string]bool{} // function -> selector calls in it
	var direct []string

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			calls := map[string]bool{}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel := selectorText(call.Fun)
				calls[sel] = true

				switch sel {
				case "a.ui.Advance", "a.ui.Render", "a.inputManager.Advance":
					direct = append(direct, fset.Position(call.Pos()).String()+": "+sel)
				}

				return true
			})

			calledIn[fn.Name.Name] = calls
		}
	}

	if len(direct) > 0 {
		t.Fatalf("d2app calls the UI or input manager's frame outside the BUG-109 gate: %v", direct)
	}

	if !calledIn["advanceOnce"]["d2screen.AdvanceUIAndInput"] {
		t.Fatal("App.advanceOnce must step the UI and input through d2screen.AdvanceUIAndInput (BUG-109)")
	}

	if !calledIn["render"]["d2screen.RenderUI"] {
		t.Fatal("App.render must draw the UI through d2screen.RenderUI (BUG-109)")
	}
}

// selectorText is a call's function as written: "a.ui.Advance".
func selectorText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return selectorText(x.X) + "." + x.Sel.Name
	default:
		return ""
	}
}
