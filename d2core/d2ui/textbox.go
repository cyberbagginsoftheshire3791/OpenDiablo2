package d2ui

import (
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
)

// static check that TextBox implements clickable widget
var _ ClickableWidget = &TextBox{}

// TextBox represents a text input box
type TextBox struct {
	*BaseWidget
	textLabel                         *Label
	lineBar                           *Label
	text                              string
	filter                            string
	bgSprite                          *Sprite
	menuSurface                       d2interface.Surface
	menuTextSurface, menuCaretSurface d2interface.Surface
	menuText                          string
	menuTextFit                       float64
	menuTextWidth                     int
	enabled                           bool
	isFocused                         bool
	isNumberOnly                      bool
	maxValue                          int

	// A wide box (NewMenuTextboxWide, the F8 note, 5 Oct 2026): any printable
	// character, up to maxLen of them (0 is the original 15), the line's
	// tail shown when it outgrows the face.
	maxLen       int
	anyPrintable bool

	*d2util.Logger
}

// NewTextbox creates a new instance of a text box
func (ui *UIManager) NewTextbox() *TextBox {
	bgSprite, err := ui.NewSprite(d2resource.TextBox2, d2resource.PaletteUnits)
	if err != nil {
		ui.Error(err.Error())
		return nil
	}
	return ui.newTextbox(bgSprite, nil)
}

// NewMenuTextbox keeps the text input behavior and draws an original plain face.
func (ui *UIManager) NewMenuTextbox() *TextBox {
	tb := ui.newTextbox(nil, ui.menuFace(180, 28))
	// The textbox owns input; its two hidden labels only supply ink.
	for _, l := range []*Label{tb.textLabel, tb.lineBar} {
		if err := ui.inputManager.UnbindHandler(l); err != nil {
			ui.Error(err.Error())
		}
	}
	return tb
}

// menuFace is the native menu's plain text-box face: a dark field with a
// brass rule along its foot.
func (ui *UIManager) menuFace(w, h int) d2interface.Surface {
	s := ui.renderer.NewSurface(w, h)
	s.DrawRect(w, h, color.RGBA{R: 15, G: 19, B: 18, A: 255})
	s.PushTranslation(0, h-1)
	s.DrawRect(w, 1, color.RGBA{R: 170, G: 143, B: 90, A: 255})
	s.Pop()

	return s
}

// NewMenuTextboxWide is the native menu's text box at another width, for a
// line of free text: it takes any printable character, holds up to maxLen of
// them, and shows the tail of the line once it outgrows the face (F8's note,
// 5 Oct 2026).
//
// IT IS DETACHED: bound to no input and never drawn by the UI manager (it
// stays invisible to it). Its owner draws it with Draw and feeds it with
// TypeChars and Backspace -- the F8 box is drawn after the frame it freezes
// is read back, and takes its keys ahead of everything, which a box the UI
// manager drew and the input manager fed could not be.
func (ui *UIManager) NewMenuTextboxWide(width, maxLen int) *TextBox {
	tb := ui.newTextbox(nil, ui.menuFace(width, 28))
	tb.maxLen = maxLen
	tb.anyPrintable = true

	for _, h := range []d2interface.InputEventHandler{tb, tb.textLabel, tb.lineBar} {
		if err := ui.inputManager.UnbindHandler(h); err != nil {
			ui.Error(err.Error())
		}
	}

	tb.SetVisible(false)

	return tb
}

// Draw draws the box where its owner wants it, visible or not (a detached
// box, NewMenuTextboxWide).
func (v *TextBox) Draw(target d2interface.Surface) {
	if v.menuSurface == nil {
		return
	}

	target.PushTranslation(v.x, v.y)
	target.Render(v.menuSurface)
	target.Pop()
	v.renderMenuText(target)
}

// TypeChars adds typed characters, as a key-chars event would.
func (v *TextBox) TypeChars(chars string) {
	if !v.enabled {
		return
	}

	v.SetText(v.text + chars)
}

// Backspace takes the last character off.
func (v *TextBox) Backspace() {
	r := []rune(v.text)
	if len(r) == 0 {
		return
	}

	v.SetText(string(r[:len(r)-1]))
}

func (ui *UIManager) newTextbox(bgSprite *Sprite, menuSurface d2interface.Surface) *TextBox {

	base := NewBaseWidget(ui)

	tb := &TextBox{
		BaseWidget:   base,
		filter:       "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
		bgSprite:     bgSprite,
		menuSurface:  menuSurface,
		textLabel:    ui.NewLabel(d2resource.FontFormal11, d2resource.PaletteUnits),
		lineBar:      ui.NewLabel(d2resource.FontFormal11, d2resource.PaletteUnits),
		enabled:      true,
		Logger:       ui.Logger,
		isNumberOnly: false, // (disabled)
		maxValue:     -1,    // (disabled)
	}
	tb.lineBar.SetText("_")

	ui.addWidget(tb)

	return tb
}

// SetFilter sets the text box filter
func (v *TextBox) SetFilter(filter string) {
	v.filter = filter
}

// Render renders the text box
func (v *TextBox) Render(target d2interface.Surface) {
	if !v.visible {
		return
	}

	if v.menuSurface != nil {
		target.PushTranslation(v.x, v.y)
		target.Render(v.menuSurface)
		target.Pop()
		v.renderMenuText(target)
		return
	} else {
		v.bgSprite.Render(target)
	}
	v.textLabel.Render(target)

	// nolint:gomnd // byte expressions
	if (time.Now().UnixNano()/1e6)&(1<<8) > 0 {
		v.lineBar.Render(target)
	}
}

func (v *TextBox) renderMenuText(target d2interface.Surface) {
	if v.menuTextSurface == nil || v.menuText != v.textLabel.GetText() {
		tw, th := v.textLabel.GetSize()
		cw, ch := v.lineBar.GetSize()
		v.menuTextSurface = target.Renderer().NewSurface(tw, th)
		v.menuCaretSurface = target.Renderer().NewSurface(cw, ch)
		v.textLabel.SetPosition(0, 0)
		v.lineBar.SetPosition(0, 0)
		v.textLabel.Render(v.menuTextSurface)
		v.lineBar.Render(v.menuCaretSurface)
		v.menuText = v.textLabel.GetText()
		v.menuTextWidth = tw
		fw, fh := v.menuSurface.GetSize()
		v.menuTextFit = math.Min(1, math.Min(float64(fw-12)/float64(tw+cw), float64(fh-6)/float64(max(th, ch))))
	}
	target.PushTranslation(v.x+6, v.y+3)
	target.PushScale(v.menuTextFit, v.menuTextFit)
	target.Render(v.menuTextSurface)
	target.PopN(2)
	// Keep the caret inside the same fitted line without changing stored text.
	if v.isFocused && (time.Now().UnixNano()/1e6)&(1<<8) > 0 {
		target.PushTranslation(v.x+6+int(math.Ceil(float64(v.menuTextWidth)*v.menuTextFit)), v.y+3)
		target.PushScale(v.menuTextFit, v.menuTextFit)
		target.Render(v.menuCaretSurface)
		target.PopN(2)
	}
}

// OnKeyChars handles key character events
func (v *TextBox) OnKeyChars(event d2interface.KeyCharsEvent) bool {
	if !v.isFocused || !v.visible || !v.enabled {
		return false
	}

	newText := string(event.Chars())
	if !(len(newText) > 0) {
		return false
	}

	if !v.isNumberOnly {
		v.text += newText
		v.SetText(v.text)

		return true
	}

	number, err := strconv.Atoi(v.text + newText)
	if err != nil {
		v.Debugf("Unable to convert string %s to intager: %s", v.text+newText, err)
		return false
	}

	if number <= v.maxValue {
		v.text += newText
	} else {
		v.text = strconv.Itoa(v.maxValue)
	}

	v.SetText(v.text)

	return true
}

// OnKeyRepeat handles key repeat events
func (v *TextBox) OnKeyRepeat(event d2interface.KeyEvent) bool {
	if !v.isFocused || !v.visible || !v.enabled {
		return false
	}
	if event.Key() == d2enum.KeyBackspace && debounceEvents(event.Duration()) {
		if len(v.text) >= 1 {
			v.text = v.text[:len(v.text)-1]
		}

		v.SetText(v.text)
	}

	return false
}

func debounceEvents(numFrames int) bool {
	const (
		delay    = 30
		interval = 3
	)

	if numFrames == 1 {
		return true
	}

	if numFrames >= delay && (numFrames-delay)%interval == 0 {
		return true
	}

	return false
}

// Advance updates the text box
func (v *TextBox) Advance(_ float64) error {
	return nil
}

// Update updates the textbox (not currently implemented)
func (v *TextBox) Update() {
}

// GetText returns the text box's text
func (v *TextBox) GetText() string {
	return v.text
}

// SetText sets the text box's text
//
//nolint:gomnd // Built-in values
func (v *TextBox) SetText(newText string) {
	result := ""

	for _, c := range newText {
		if !v.accepts(c) {
			continue
		}

		result += string(c)
	}

	limit := v.maxLen
	if limit <= 0 {
		limit = 15
	}

	if r := []rune(result); len(r) > limit {
		result = string(r[:limit])
	}

	v.text = result
	if v.menuSurface != nil {
		v.textLabel.SetText(v.shownTail(result))
		return
	}

	for {
		tw, _ := v.textLabel.GetTextMetrics(result)

		if tw > 150 {
			result = result[1:]
			continue
		}

		v.lineBar.SetPosition(v.x+6+tw, v.y+3)
		v.textLabel.SetText(result)

		break
	}
}

// accepts is the filter: the filter's characters, or any printable one for a
// wide box.
func (v *TextBox) accepts(c rune) bool {
	if v.anyPrintable {
		return unicode.IsPrint(c)
	}

	return strings.Contains(v.filter, string(c))
}

// shownTail is the part of a wide box's line that fits its face: the end,
// where he is typing. A 15-character box shows it all, fitted (renderMenuText).
func (v *TextBox) shownTail(text string) string {
	if v.maxLen <= 0 || v.menuSurface == nil {
		return text
	}

	fw, _ := v.menuSurface.GetSize()
	cw, _ := v.lineBar.GetSize()
	room := fw - 12 - cw

	r := []rune(text)
	for len(r) > 0 {
		if tw, _ := v.textLabel.GetTextMetrics(string(r)); tw <= room {
			break
		}

		r = r[1:]
	}

	return string(r)
}

// GetSize returns the size of the text box
func (v *TextBox) GetSize() (width, height int) {
	if v.menuSurface != nil {
		return v.menuSurface.GetSize()
	}
	return v.bgSprite.GetCurrentFrameSize()
}

// SetPosition sets the position of the text box
//
//nolint:gomnd // Built-in values
func (v *TextBox) SetPosition(x, y int) {
	lw, _ := v.textLabel.GetSize()

	v.x = x
	v.y = y

	v.textLabel.SetPosition(v.x+6, v.y+3)
	v.lineBar.SetPosition(v.x+6+lw, v.y+3)
	if v.bgSprite != nil {
		v.bgSprite.SetPosition(v.x, v.y+26)
	}
}

// GetEnabled returns the enabled state of the text box
func (v *TextBox) GetEnabled() bool {
	return v.enabled
}

// SetEnabled sets the enabled state of the text box
func (v *TextBox) SetEnabled(enabled bool) {
	v.enabled = enabled
}

// SetPressed does nothing for text boxes
func (v *TextBox) SetPressed(_ bool) {
	// no op
}

// GetPressed does nothing for text boxes
func (v *TextBox) GetPressed() bool {
	return false
}

// OnActivated handles activation events for the text box
func (v *TextBox) OnActivated(_ func()) {
	// no op
}

// Activate activates the text box
func (v *TextBox) Activate() {
	v.isFocused = true
}

// SetNumberOnly sets text box to support only numeric values
func (v *TextBox) SetNumberOnly(max int) {
	v.isNumberOnly = true
	v.maxValue = max
}
