//go:build harness

package d2app

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// loadingFlag is the screen manager's one answer the frame asks.
type loadingFlag struct{ loading bool }

func (l *loadingFlag) IsLoading() bool { return l.loading }

// frameSpy is the UI manager and the input manager in one: it records each
// read of what a loading screen's OnLoad writes, and whether that read came
// while the screen was loading.
type frameSpy struct {
	screen *loadingFlag

	uiAdvances, uiRenders, inputAdvances int
	whileLoading                         []string
}

func (f *frameSpy) note(what string) {
	if f.screen.loading {
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
		if err := advanceUIAndInput(screen, spy, input, 1.0/60, 0); err != nil {
			t.Fatal(err)
		}

		renderUI(screen, spy, nil)
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
