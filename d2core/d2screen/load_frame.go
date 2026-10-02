package d2screen

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

// BUG-109: NO UI OR INPUT FRAME WHILE A SCREEN LOADS.
//
// The screen manager runs a new screen's OnLoad on a goroutine of its own
// (ScreenManager.Advance) and, while it runs, takes one progress update per
// frame from it. OnLoad builds the screen: every widget it makes is appended
// to the UI manager's list (UIManager.addWidget, which also re-sorts it in
// place) and bound as an input handler (the input manager's handler list,
// appended and re-sorted the same way). The frame went on, on the update
// goroutine, to UIManager.Advance and the input manager's Advance -- ranging
// over those same two lists while the loader wrote them. On 1 Oct a harness
// game going back to the main menu read the widget list's header
// half-written, a nil array with the new length, and died at
// ui_manager.go:172 on address 0x0 (game-20261001-165716.113-TestSaveResume
// .log). Every screen change since the fork has run that race; it was lost
// once.
//
// While a screen loads nothing on the update goroutine touches what the
// loader builds: the UI manager is neither advanced nor drawn and the input
// manager does not poll -- a scripted press waits, unconsumed, for the first
// frame after the load (the load screen takes no input anyway). The loader's
// last send (LoadingState.Done) is received before IsLoading goes false, so
// everything OnLoad wrote happens before the first frame that reads it.
//
// It lives here, not in d2app, so its tests run untagged and under -race on
// CI: d2app links ebiten's UI, and a test binary there cannot run headless.

// Loading is the one question the frame asks of the screen manager.
type Loading interface {
	IsLoading() bool
}

// UIFrame is the UI manager's per-frame half (*d2ui.UIManager).
type UIFrame interface {
	Advance(elapsed float64)
	Render(target d2interface.Surface)
}

// AdvanceUIAndInput is the frame's UI and input step, skipped while a screen
// loads.
func AdvanceUIAndInput(screen Loading, ui UIFrame, input d2interface.InputManager, elapsed, current float64) error {
	if screen.IsLoading() {
		return nil
	}

	ui.Advance(elapsed)

	return input.Advance(elapsed, current)
}

// RenderUI draws the UI manager's widgets, except while a screen loads.
func RenderUI(screen Loading, ui UIFrame, target d2interface.Surface) {
	if screen.IsLoading() {
		return
	}

	ui.Render(target)
}
