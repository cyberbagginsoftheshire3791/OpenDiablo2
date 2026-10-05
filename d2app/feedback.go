package d2app

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"log"
	"os"
	"runtime/debug"
	"strings"

	ebitenlib "github.com/hajimehoshi/ebiten/v2"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2report"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
)

// F8 IS FEEDBACK, ANYWHERE (Level 1, ruled by Josh 5 Oct 2026). The key
// freezes the frame he is looking at, holds the world as the escape menu does,
// and opens one line: what happened, or what did he want? Enter saves
// %LOCALAPPDATA%\Strigoi\feedback\<stamp>\{note.txt, shot.png, state.json}
// (d2report.WriteFeedback) and a FEEDBACK line in the log; Escape cancels and
// writes nothing.
//
// IT IS THE APP'S, NOT A SCREEN'S, so the same box opens on the native menu,
// -classic's menu, the World Editor and in a game, and the frame it saves is
// the whole frame -- every screen, the UI and the gui manager drawn, the box
// not yet (App.render).
//
// THE KEY. F8 sat in D2's key table as UseSkill8, which nothing in this game
// handles (GameControls.OnKeyDown has no case for any UseSkill); the default
// binding is taken off it (key_map.go) so the table and the game agree. The
// World Editor uses no F-key.
const feedbackKey = d2enum.KeyF8

// Words on the box.
const (
	feedbackPrompt   = "What happened, or what did you want?"
	feedbackHint     = "Enter saves it, with a picture of this moment.    Esc cancels."
	feedbackSaved    = "Feedback saved"
	feedbackMaxRunes = 400
)

// How long the "saved" line stays up, in real seconds.
const feedbackNoticeSeconds = 3.0

// The box, in the 800x600 frame.
const (
	fbPanelX, fbPanelY, fbPanelW, fbPanelH = 100, 228, 600, 124
	fbBoxX, fbBoxY, fbBoxW                 = 120, 272, 560
)

// fittedLabel is a label drawn shrunk to a width when its text is longer, the
// way the native menu fits its own (renderMenuLabel): rendered once to a
// surface, re-rendered when its text changes.
type fittedLabel struct {
	*d2ui.Label
	surface d2interface.Surface
	text    string
}

func (l *fittedLabel) draw(target d2interface.Surface, x, y, maxW int) {
	if l == nil || l.Label == nil || l.GetText() == "" {
		return
	}

	w, h := l.GetSize()
	if w <= 0 || h <= 0 {
		return
	}

	if w <= maxW {
		l.SetPosition(x, y)
		l.Render(target)

		return
	}

	if l.surface == nil || l.text != l.GetText() {
		l.surface = target.Renderer().NewSurface(w, h)
		l.SetPosition(0, 0)
		l.Render(l.surface)
		l.text = l.GetText()
	}

	fit := float64(maxW) / float64(w)

	target.PushTranslation(x, y)
	target.PushFilter(d2enum.FilterLinear)
	target.PushScale(fit, fit)
	target.Render(l.surface)
	target.PopN(3)
}

// feedbackOverlay is F8's box, the notices it leaves, and the main menu's
// line about a run that did not close cleanly.
type feedbackOverlay struct {
	app *App

	box                  *d2ui.TextBox
	prompt, hint, notice *fittedLabel
	startLine            *fittedLabel

	open        bool
	pendingShot bool
	shot        []byte // the frozen frame, PNG
	shotW       int
	shotH       int
	heldGame    *d2gamescreen.Game

	noticeText  string
	noticeUntil float64

	// startNotice is the line the menu shows for earlier runs StartRun found
	// ended badly (d2report.Notice); startDir the newest one's folder.
	startNotice string
	startDir    string
	startDrawn  bool

	// What the provider reports.
	opened, saved, cancelled int
	lastDir, lastNote        string
	lastErr                  string
}

func newFeedbackOverlay(a *App) *feedbackOverlay {
	f := &feedbackOverlay{app: a}

	label := func(font string, ink color.Color) *fittedLabel {
		l := a.ui.NewLabel(font, d2resource.PaletteUnits)
		if l == nil {
			return nil
		}

		// Drawn by this overlay alone, never by the UI manager (it stays
		// invisible to it) and bound to no input.
		_ = a.inputManager.UnbindHandler(l)
		l.Color[0] = ink

		return &fittedLabel{Label: l}
	}

	f.prompt = label(d2resource.FontFormal12, color.RGBA{R: 222, G: 208, B: 176, A: 255})
	f.hint = label(d2resource.FontFormal10, color.RGBA{R: 160, G: 162, B: 146, A: 255})
	f.notice = label(d2resource.FontFormal12, color.RGBA{R: 222, G: 208, B: 176, A: 255})
	f.startLine = label(d2resource.FontFormal12, color.RGBA{R: 233, G: 172, B: 138, A: 255})

	if f.prompt != nil {
		f.prompt.SetText(feedbackPrompt)
		f.prompt.Color[0] = color.RGBA{R: 222, G: 208, B: 176, A: 255}
	}

	if f.hint != nil {
		f.hint.SetText(feedbackHint)
		f.hint.Color[0] = color.RGBA{R: 160, G: 162, B: 146, A: 255}
	}

	f.box = a.ui.NewMenuTextboxWide(fbBoxW, feedbackMaxRunes)
	if f.box != nil {
		f.box.SetPosition(fbBoxX, fbBoxY)
	}

	if err := a.inputManager.BindHandlerWithPriority(f, d2enum.PriorityTop); err != nil {
		a.Errorf("binding F8 feedback: %v", err)
	}

	d2harness.Register(feedbackProvider{f})

	return f
}

// setStartNotice is the main menu's line for runs that ended badly.
func (f *feedbackOverlay) setStartNotice(text, dir string) {
	f.startNotice, f.startDir = text, dir

	if f.startLine != nil {
		f.startLine.SetText(text)
		f.startLine.Color[0] = color.RGBA{R: 233, G: 172, B: 138, A: 255}
	}
}

// OnKeyDown opens the box on F8; open, it takes every key.
func (f *feedbackOverlay) OnKeyDown(event d2interface.KeyEvent) bool {
	if !f.open {
		if event.Key() != feedbackKey || f.box == nil {
			return false
		}

		f.openBox()

		return true
	}

	switch event.Key() {
	case d2enum.KeyEnter, d2enum.KeyKPEnter:
		f.save()
	case d2enum.KeyEscape:
		f.cancel()
	case d2enum.KeyBackspace:
		f.box.Backspace()
	}

	return true
}

// OnKeyRepeat repeats Backspace as the menu's box does; open, nothing else
// reaches the game.
func (f *feedbackOverlay) OnKeyRepeat(event d2interface.KeyEvent) bool {
	if !f.open {
		return false
	}

	if event.Key() == d2enum.KeyBackspace && event.Duration() > 1 && feedbackRepeat(event.Duration()) {
		f.box.Backspace()
	}

	return true
}

// feedbackRepeat is the menu box's own pacing (d2ui debounceEvents), the
// first press being OnKeyDown's.
func feedbackRepeat(frames int) bool {
	const delay, interval = 30, 3

	return frames >= delay && (frames-delay)%interval == 0
}

// OnKeyUp swallows releases while open.
func (f *feedbackOverlay) OnKeyUp(d2interface.KeyEvent) bool { return f.open }

// OnKeyChars types into the box.
func (f *feedbackOverlay) OnKeyChars(event d2interface.KeyCharsEvent) bool {
	if !f.open {
		return false
	}

	f.box.TypeChars(string(event.Chars()))

	return true
}

// The mouse does nothing under the box: a click must not walk him, press a
// menu button or pick a tile in the editor.
func (f *feedbackOverlay) OnMouseButtonDown(d2interface.MouseEvent) bool   { return f.open }
func (f *feedbackOverlay) OnMouseButtonUp(d2interface.MouseEvent) bool     { return f.open }
func (f *feedbackOverlay) OnMouseButtonRepeat(d2interface.MouseEvent) bool { return f.open }
func (f *feedbackOverlay) OnMouseWheel(d2interface.MouseWheelEvent) bool   { return f.open }

func (f *feedbackOverlay) openBox() {
	f.open = true
	f.pendingShot = true
	f.shot, f.shotW, f.shotH = nil, 0, 0
	f.opened++
	f.box.SetText("")
	f.box.Activate()

	if g, ok := f.app.screen.Current().(*d2gamescreen.Game); ok {
		g.SetFeedbackHold(true)
		f.heldGame = g
	}
}

func (f *feedbackOverlay) closeBox() {
	f.open = false
	f.pendingShot = false
	f.shot = nil

	if f.heldGame != nil {
		f.heldGame.SetFeedbackHold(false)
		f.heldGame = nil
	}
}

func (f *feedbackOverlay) cancel() {
	f.cancelled++
	f.closeBox()
}

func (f *feedbackOverlay) save() {
	note := strings.TrimSpace(f.box.GetText())

	state := f.app.reportState()
	if f.shot != nil {
		state["frame"] = map[string]interface{}{"w": f.shotW, "h": f.shotH}
	}

	dir, err := d2report.WriteFeedback(d2report.Feedback{Note: note, Shot: f.shot, State: state})

	log.Printf("FEEDBACK dir=%s text=%q", dir, note)

	f.lastDir, f.lastNote = dir, note

	if err != nil {
		f.lastErr = err.Error()
		f.showNotice("Feedback could not be saved: " + err.Error())
		log.Printf("FEEDBACK error: %v", err)
	} else {
		f.lastErr = ""
		f.saved++
		f.showNotice(feedbackSaved)
	}

	f.closeBox()
}

func (f *feedbackOverlay) showNotice(text string) {
	f.noticeText = text
	f.noticeUntil = d2util.Now() + feedbackNoticeSeconds

	if f.notice != nil {
		f.notice.SetText(text)
		f.notice.Color[0] = color.RGBA{R: 222, G: 208, B: 176, A: 255}
	}
}

// render runs after everything the frame draws but the console and the
// harness's reads: it first freezes the frame F8 asked for -- before the box
// is on it -- then draws the box and the notices.
func (f *feedbackOverlay) render(target d2interface.Surface) {
	if f.pendingShot {
		f.pendingShot = false
		f.freeze(target)
	}

	f.startDrawn = false

	// The menu's line is for the menu he starts at: once he is in a game it
	// has been read, and the menu he comes back to is quiet.
	if _, inGame := f.app.screenCurrent().(*d2gamescreen.Game); inGame && f.startNotice != "" {
		f.startNotice = ""
	}

	if f.startNotice != "" && f.onMainMenu() {
		target.DrawRect(800, 28, color.RGBA{R: 13, G: 17, B: 15, A: 236})
		f.startLine.draw(target, 10, 6, 780)
		f.startDrawn = true
	}

	if f.open {
		target.DrawRect(800, 600, color.RGBA{A: 150})
		target.PushTranslation(fbPanelX, fbPanelY)
		target.DrawRect(fbPanelW, fbPanelH, color.RGBA{R: 11, G: 16, B: 15, A: 244})
		target.DrawRect(fbPanelW, 1, color.RGBA{R: 170, G: 143, B: 90, A: 255})
		target.Pop()
		f.prompt.draw(target, fbBoxX, fbPanelY+16, fbBoxW)
		f.box.Draw(target)
		f.hint.draw(target, fbBoxX, fbBoxY+40, fbBoxW)
	}

	if f.noticeText != "" {
		if d2util.Now() > f.noticeUntil {
			f.noticeText = ""
			return
		}

		y := 40
		if f.startDrawn {
			y = 34
		}

		target.PushTranslation(250, y)
		target.DrawRect(300, 26, color.RGBA{R: 13, G: 17, B: 15, A: 236})
		target.Pop()
		f.notice.draw(target, 262, y+5, 276)
	}
}

// freeze reads the frame back as F8's picture: one ReadPixels (~33 ms), on the
// draw goroutine, as strigoi_screenshot does.
func (f *feedbackOverlay) freeze(target d2interface.Surface) {
	img := target.Screenshot()
	if img == nil {
		return
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Printf("FEEDBACK frame not encoded: %v", err)
		return
	}

	f.shot = buf.Bytes()
	f.shotW, f.shotH = img.Bounds().Dx(), img.Bounds().Dy()
}

func (f *feedbackOverlay) onMainMenu() bool {
	if f.app.screen == nil {
		return false
	}

	_, ok := f.app.screen.Current().(*d2gamescreen.MainMenu)

	return ok
}

// feedbackProvider is the harness's "feedback" system: the box, what it last
// saved, and the menu's line. None of it is the world's, and none of it is in
// the digest (HarnessDigest): it is this process's presentation.
type feedbackProvider struct{ f *feedbackOverlay }

func (p feedbackProvider) HarnessName() string { return "feedback" }

func (p feedbackProvider) HarnessState() map[string]interface{} {
	f := p.f

	text := ""
	if f.box != nil {
		text = f.box.GetText()
	}

	return map[string]interface{}{
		"key":          "f8",
		"open":         f.open,
		"text":         text,
		"shot_taken":   f.shot != nil,
		"opened":       f.opened,
		"saved":        f.saved,
		"cancelled":    f.cancelled,
		"last_dir":     f.lastDir,
		"last_note":    f.lastNote,
		"last_error":   f.lastErr,
		"notice":       f.noticeText,
		"start_notice": f.startNotice,
		"start_dir":    f.startDir,
		"start_drawn":  f.startDrawn,
		"reports_root": d2report.Root(),
		"feedback_dir": d2report.FeedbackDir(),
		"crashes_dir":  d2report.CrashesDir(),
		"held_game":    f.heldGame != nil,
		"screen":       f.app.screenName(),
	}
}

func (p feedbackProvider) HarnessDigest() (world, process map[string]interface{}) { return nil, nil }

// screenName is the screen in front, by the harness's words for them.
func (a *App) screenName() string {
	if a.screen == nil {
		return "none"
	}

	switch s := a.screen.Current().(type) {
	case nil:
		return "none"
	case *d2gamescreen.MainMenu:
		return "main_menu"
	case *d2gamescreen.Game:
		return "game"
	case *d2gamescreen.Editor:
		return "world_editor"
	case *d2gamescreen.CharacterSelect:
		return "character_select"
	case *d2gamescreen.SelectHeroClass:
		return "select_hero"
	case *d2gamescreen.Credits:
		return "credits"
	case *d2gamescreen.MapEngineTest:
		return "map_engine_test"
	default:
		return strings.TrimPrefix(fmt.Sprintf("%T", s), "*d2gamescreen.")
	}
}

// reportState is what a note's and a crash's state.json say of this process:
// the build, the screen, how it was started, and the game's own state when a
// game is in front. A crash calls it under its own recover (d2report).
func (a *App) reportState() map[string]interface{} {
	version := map[string]interface{}{"branch": a.gitBranch, "commit": a.gitCommit}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision", "vcs.time", "vcs.modified":
				version[strings.TrimPrefix(s.Key, "vcs.")] = s.Value
			}
		}
	}

	state := map[string]interface{}{
		"version": version,
		"screen":  a.screenName(),
		"pid":     os.Getpid(),
		"args":    os.Args[1:],
		"harness": a.harnessEnabled(),
	}

	if a.Options != nil {
		state["classic"] = a.Options.classic
	}

	if a.screen != nil {
		state["loading"] = a.screen.IsLoading()
	}

	if a.renderer != nil {
		state["fullscreen"] = a.renderer.IsFullScreen()
		w, h := ebitenlib.WindowSize()
		state["window"] = map[string]interface{}{"w": w, "h": h}
	}

	if a.feedback != nil && a.feedback.open {
		state["feedback_open"] = true
	}

	if g, ok := a.screenCurrent().(*d2gamescreen.Game); ok {
		state["game"] = g.FeedbackState()
	}

	return state
}

func (a *App) screenCurrent() interface{} {
	if a.screen == nil {
		return nil
	}

	return a.screen.Current()
}
