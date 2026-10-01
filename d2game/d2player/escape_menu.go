package d2player

import (
	"strings"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2gui"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

type (
	layoutID int
	optionID int
)

const (
	// UI
	labelGutter    = 10
	sidePanelsSize = 80
	pentSize       = 54
	menuSize       = 500
	spacerWidth    = 10

	// layouts
	noLayoutID layoutID = iota - 2
	saveLayoutID
	mainLayoutID
	optionsLayoutID
	soundOptionsLayoutID
	videoOptionsLayoutID
	automapOptionsLayoutID
	configureControlsLayoutID

	// audio
	optAudioSoundVolume optionID = iota
	optAudioMusicVolume
	optAudio3dSound
	optAudioHardwareAcceleration
	optAudioEnvEffects
	optAudioNpcSpeech
	// video
	optVideoResolution
	optVideoLightingQuality
	optVideoBlendedShadows
	optVideoPerspective
	optVideoGamma
	optVideoContrast
	// automap
	optAutomapSize
	optAutomapFade
	optAutomapCenterWhenCleared
	optAutomapShowParty
	optAutomapShowNames
)

const (
	singleFrame = time.Millisecond * 16
)

// saveGameLayoutID is SAVE GAME's (M4.6 B5): like noLayoutID (RETURN TO GAME)
// and saveLayoutID (SAVE AND EXIT GAME) it names what an entry does, not a
// layout the menu shows.
const saveGameLayoutID layoutID = -3

// MenuSaver is the game's save as the escape menu asks for it (M4.6 B5; the
// build plan's rules 1-3). The game screen is the one saver; the menu knows
// only these two questions and the words that come back, never a refusal's
// code, so a refusal the save gains or loses changes nothing here.
type MenuSaver interface {
	// SaveRefusedNow is the note under the entries when the game cannot be
	// saved at this moment -- why, in the player's words, and what leaving
	// without saving does (the B5 review fixes: that depends on the game --
	// a network game has no last save to come back to) -- or "" when it can.
	// The menu asks it as it opens: the world is paused under the menu, so
	// the answer holds while it is up.
	SaveRefusedNow() string

	// SaveFromMenu saves the game. exit is SAVE AND EXIT GAME (the menu leaves
	// for the main menu once it is saved); otherwise SAVE GAME, which keeps
	// playing and puts the notice up itself.
	SaveFromMenu(exit bool) MenuSaveResult
}

// MenuSaveResult is what SaveFromMenu did: saved, or not -- and then Words is
// the whole note the menu shows: why, and what leaving does.
type MenuSaveResult struct {
	Saved bool
	Words string
}

// NewEscapeMenu creates a new escape menu
func NewEscapeMenu(navigator d2interface.Navigator,
	renderer d2interface.Renderer,
	audioProvider d2interface.AudioProvider,
	uiManager *d2ui.UIManager,
	guiManager *d2gui.GuiManager,
	assetManager *d2asset.AssetManager,
	l d2util.LogLevel,
	keyMap *KeyMap,
) *EscapeMenu {
	m := &EscapeMenu{
		audioProvider: audioProvider,
		renderer:      renderer,
		navigator:     navigator,
		guiManager:    guiManager,
		assetManager:  assetManager,
		keyMap:        keyMap,
	}

	keyBindingMenu := NewKeyBindingMenu(assetManager, renderer, uiManager, guiManager, keyMap, l, m)
	m.keyBindingMenu = keyBindingMenu

	m.layouts = make(map[layoutID]*layout)
	m.layouts[mainLayoutID] = m.newMainLayout()
	m.layouts[optionsLayoutID] = m.newOptionsLayout()
	m.layouts[soundOptionsLayoutID] = m.newSoundOptionsLayout()
	m.layouts[videoOptionsLayoutID] = m.newVideoOptionsLayout()
	m.layouts[automapOptionsLayoutID] = m.newAutomapOptionsLayout()

	m.Logger = d2util.NewLogger()
	m.Logger.SetLevel(l)
	m.Logger.SetPrefix(logPrefix)

	return m
}

// EscapeMenu represents the in-game menu that shows up when the esc key is pressed
type EscapeMenu struct {
	isOpen        bool
	selectSound   d2interface.SoundEffect
	currentLayout layoutID

	// leftPent and rightPent are generated once and shared between the layouts
	leftPent  *d2gui.AnimatedSprite
	rightPent *d2gui.AnimatedSprite
	layouts   map[layoutID]*layout

	renderer       d2interface.Renderer
	audioProvider  d2interface.AudioProvider
	navigator      d2interface.Navigator
	guiManager     *d2gui.GuiManager
	assetManager   *d2asset.AssetManager
	keyMap         *KeyMap
	keyBindingMenu *KeyBindingMenu

	onCloseCb func()

	// M4.6 B5: the game's save (SetSaver); the main layout's exit entry and
	// the line under the entries, which say whether and why a save is refused;
	// and whether the exit entry reads EXIT WITHOUT SAVING.
	saver       MenuSaver
	exitLabel   *d2gui.Label
	noteLabels  []*d2gui.Label
	exitRefused bool

	*d2util.Logger
}

type layout struct {
	*d2gui.Layout
	leftPent           *d2gui.AnimatedSprite
	rightPent          *d2gui.AnimatedSprite
	actionableElements []actionableElement
	currentEl          int
	rendered           bool
	isRaw              bool
}

func (l *layout) Trigger() {
	// noop
}

type showLayoutLabel struct {
	*d2gui.Label
	target     layoutID
	showLayout func(id layoutID)
}

func (l *showLayoutLabel) Trigger() {
	l.showLayout(l.target)
}

type enumLabel struct {
	*d2gui.Layout
	textChangingLabel *d2gui.Label
	optionID          optionID
	values            []string
	current           int
	playSound         func()
	updateValue       func(optID optionID, value string)
	*EscapeMenu
}

func (l *enumLabel) Trigger() {
	l.playSound()
	next := (l.current + 1) % len(l.values)
	l.current = next

	currentValue := l.values[l.current]
	if err := l.textChangingLabel.SetText(currentValue); err != nil {
		l.EscapeMenu.Errorf("could not change the label text to: %s", currentValue)
	}

	l.updateValue(l.optionID, currentValue)
}

type actionableElement interface {
	GetOffset() (int, int)
	Trigger()
}

// newMainLayout is the menu Esc opens. M4.6 B5 (the build plan's rule 1): SAVE
// GAME saves and keeps playing; SAVE AND EXIT GAME saves, then leaves for the
// main menu -- or, when the save is refused (rule 2: a fight, and the rest),
// reads EXIT WITHOUT SAVING, and the line under the entries says why.
func (m *EscapeMenu) newMainLayout() *layout {
	return m.wrapLayout(func(l *layout) {
		m.addBigSelectionLabel(l, "OPTIONS", optionsLayoutID)
		m.addBigSelectionLabel(l, MenuSaveGame, saveGameLayoutID)
		m.exitLabel = m.addBigSelectionLabel(l, MenuSaveAndExit, saveLayoutID)
		m.addBigSelectionLabel(l, "RETURN TO GAME", noLayoutID)

		for i := 0; i < menuNoteLines; i++ {
			m.noteLabels = append(m.noteLabels, m.addNoteLabel(l))
		}
	})
}

// menuNoteLines is how many lines the note under the entries has room for;
// menuNoteColor its ink, a pale gold (Strigoi's font set multiplies it over
// its near-white ink): measured on a screenshot, Diablo II's brown read too
// dark over the village at noon.
const (
	menuNoteLines = 3
	menuNoteColor = 0xf0dca0ff
)

// addNoteLabel is one line under the main menu's entries: empty until a save
// is refused. Not an entry -- the keys and the pentagrams pass it by. Each
// line is its own label so the base layout centres each (a label of two
// lines draws them left-aligned).
func (m *EscapeMenu) addNoteLabel(l *layout) *d2gui.Label {
	label, err := l.AddLabelWithColor("", d2gui.FontStyle16Units, d2util.Color(menuNoteColor))
	if err != nil {
		m.Error(err.Error())
		return nil
	}

	return label
}

// noteText is the note as the lines show it, joined by newlines.
func (m *EscapeMenu) noteText() string {
	lines := make([]string, 0, len(m.noteLabels))

	for _, l := range m.noteLabels {
		if l != nil && l.GetText() != "" {
			lines = append(lines, l.GetText())
		}
	}

	return strings.Join(lines, "\n")
}

func (m *EscapeMenu) newOptionsLayout() *layout {
	return m.wrapLayout(func(l *layout) {
		m.addBigSelectionLabel(l, "SOUND OPTIONS", soundOptionsLayoutID)
		m.addBigSelectionLabel(l, "VIDEO OPTIONS", videoOptionsLayoutID)
		m.addBigSelectionLabel(l, "AUTOMAP OPTIONS", automapOptionsLayoutID)
		m.addBigSelectionLabel(l, "CONFIGURE CONTROLS", configureControlsLayoutID)
		m.addBigSelectionLabel(l, "PREVIOUS MENU", mainLayoutID)
	})
}

func (m *EscapeMenu) newSoundOptionsLayout() *layout {
	return m.wrapLayout(func(l *layout) {
		m.addTitle(l, "SOUND OPTIONS")
		m.addEnumLabel(l, optAudioSoundVolume, "SOUND", []string{"TODO"})
		m.addEnumLabel(l, optAudioMusicVolume, "MUSIC", []string{"TODO"})
		m.addEnumLabel(l, optAudio3dSound, "3D BIAS", []string{"TODO"})
		m.addEnumLabel(l, optAudioHardwareAcceleration, "HARDWARE ACCELERATION", []string{"ON", "OFF"})
		m.addEnumLabel(l, optAudioEnvEffects, "ENVIRONMENTAL EFFECTS", []string{"ON", "OFF"})
		m.addEnumLabel(l, optAudioNpcSpeech, "NPC SPEECH", []string{"AUDIO AND TEXT", "AUDIO ONLY", "TEXT ONLY"})
		m.addPreviousMenuLabel(l)
	})
}

func (m *EscapeMenu) newVideoOptionsLayout() *layout {
	return m.wrapLayout(func(l *layout) {
		m.addTitle(l, "VIDEO OPTIONS")
		m.addEnumLabel(l, optVideoResolution, "VIDEO RESOLUTION", []string{"800X600", "1024X768"})
		m.addEnumLabel(l, optVideoLightingQuality, "LIGHTING QUALITY", []string{"LOW", "HIGH"})
		m.addEnumLabel(l, optVideoBlendedShadows, "BLENDED SHADOWS", []string{"ON", "OFF"})
		m.addEnumLabel(l, optVideoPerspective, "PERSPECTIVE", []string{"ON", "OFF"})
		m.addEnumLabel(l, optVideoGamma, "GAMMA", []string{"TODO"})
		m.addEnumLabel(l, optVideoContrast, "CONTRAST", []string{"TODO"})
		m.addPreviousMenuLabel(l)
	})
}

func (m *EscapeMenu) newAutomapOptionsLayout() *layout {
	return m.wrapLayout(func(l *layout) {
		m.addTitle(l, "AUTOMAP OPTIONS")
		m.addEnumLabel(l, optAutomapSize, "AUTOMAP SIZE", []string{"FULL SCREEN"})
		m.addEnumLabel(l, optAutomapFade, "FADE", []string{"YES", "NO"})
		m.addEnumLabel(l, optAutomapCenterWhenCleared, "CENTER WHEN CLEARED", []string{"YES", "NO"})
		m.addEnumLabel(l, optAutomapShowParty, "SHOW PARTY", []string{"YES", "NO"})
		m.addEnumLabel(l, optAutomapShowNames, "SHOW NAMES", []string{"YES", "NO"})
		m.addPreviousMenuLabel(l)
	})
}

func (m *EscapeMenu) newConfigureControlsLayout(keyBindingMenu *KeyBindingMenu) *layout {
	return &layout{
		Layout: keyBindingMenu.GetLayout(),
		isRaw:  true,
	}
}

func (m *EscapeMenu) wrapLayout(fn func(*layout)) *layout {
	wrapper := d2gui.CreateLayout(m.renderer, d2gui.PositionTypeHorizontal, m.assetManager)
	wrapper.SetVerticalAlign(d2gui.VerticalAlignMiddle)
	wrapper.AddSpacerDynamic()

	center := wrapper.AddLayout(d2gui.PositionTypeHorizontal)
	center.SetSize(menuSize, 0)

	left := center.AddLayout(d2gui.PositionTypeVertical)
	left.SetSize(sidePanelsSize, pentSize)
	leftPent, err := left.AddAnimatedSprite(d2resource.PentSpin, d2resource.PaletteUnits, d2gui.DirectionBackward)

	if err != nil {
		m.Error(err.Error())
		return nil
	}

	m.leftPent = leftPent

	// wrap the base layout so we can pass values around more easily
	base := &layout{}
	baseLayout := center.AddLayout(d2gui.PositionTypeVertical)
	baseLayout.SetHorizontalAlign(d2gui.HorizontalAlignCenter)
	base.Layout = baseLayout
	fn(base)

	right := center.AddLayout(d2gui.PositionTypeHorizontal)
	// For some reason, aligning the panel to the right won't align the pentagram, so we need to add a static spacer.
	right.AddSpacerStatic(sidePanelsSize-pentSize, 0)
	right.SetSize(sidePanelsSize, pentSize)
	rightPent, err := right.AddAnimatedSprite(d2resource.PentSpin, d2resource.PaletteUnits, d2gui.DirectionForward)

	if err != nil {
		m.Error(err.Error())
		return nil
	}

	m.rightPent = rightPent

	wrapper.AddSpacerDynamic()

	return &layout{
		Layout:             wrapper,
		leftPent:           leftPent,
		rightPent:          rightPent,
		actionableElements: base.actionableElements,
	}
}

func (m *EscapeMenu) addTitle(l *layout, text string) {
	_, err := l.AddLabel(text, d2gui.FontStyle42Units)
	if err != nil {
		m.Errorf("could not add label: %s to the escape menu", text)
	}

	l.AddSpacerStatic(spacerWidth, labelGutter)
}

func (m *EscapeMenu) addBigSelectionLabel(l *layout, text string, targetLayout layoutID) *d2gui.Label {
	guiLabel, err := l.AddLabel(text, d2gui.FontStyle42Units)
	if err != nil {
		m.Error(err.Error())
	}

	label := &showLayoutLabel{Label: guiLabel, target: targetLayout, showLayout: m.showLayout}
	label.SetMouseClickHandler(func(_ d2interface.MouseEvent) {
		label.Trigger()
	})

	elID := len(l.actionableElements)

	label.SetMouseEnterHandler(func(_ d2interface.MouseMoveEvent) {
		m.onHoverElement(elID)
	})

	l.AddSpacerStatic(spacerWidth, labelGutter)
	l.actionableElements = append(l.actionableElements, label)

	return guiLabel
}

func (m *EscapeMenu) addPreviousMenuLabel(l *layout) {
	l.AddSpacerStatic(spacerWidth, labelGutter)

	guiLabel, err := l.AddLabel("PREVIOUS MENU", d2gui.FontStyle30Units)
	if err != nil {
		m.Error(err.Error())
	}

	label := &showLayoutLabel{Label: guiLabel, target: optionsLayoutID, showLayout: m.showLayout}
	label.SetMouseClickHandler(func(_ d2interface.MouseEvent) {
		label.Trigger()
	})

	elID := len(l.actionableElements)

	label.SetMouseEnterHandler(func(_ d2interface.MouseMoveEvent) {
		m.onHoverElement(elID)
	})

	l.actionableElements = append(l.actionableElements, label)
}

func (m *EscapeMenu) addEnumLabel(l *layout, optID optionID, text string, values []string) {
	guiLayout := l.AddLayout(d2gui.PositionTypeHorizontal)
	layout := &layout{Layout: guiLayout}
	layout.SetSize(menuSize, 0)

	_, err := layout.AddLabel(text, d2gui.FontStyle30Units)
	if err != nil {
		m.Errorf("could not add label: %s to the escape menu", text)
	}

	elID := len(l.actionableElements)

	layout.SetMouseEnterHandler(func(_ d2interface.MouseMoveEvent) {
		m.onHoverElement(elID)
	})

	layout.AddSpacerDynamic()

	guiLabel, err := layout.AddLabel(values[0], d2gui.FontStyle30Units)
	if err != nil {
		m.Error(err.Error())
	}

	label := &enumLabel{
		Layout:            guiLayout,
		textChangingLabel: guiLabel,
		optionID:          optID,
		values:            values,
		current:           0,
		playSound:         m.playSound,
		updateValue:       m.onUpdateValue,
	}

	layout.SetMouseClickHandler(func(_ d2interface.MouseEvent) {
		label.Trigger()
	})
	l.AddSpacerStatic(spacerWidth, labelGutter)

	l.actionableElements = append(l.actionableElements, label)
}

// OnLoad loads the necessary files for the escape menu
func (m *EscapeMenu) OnLoad() {
	var err error

	err = m.keyBindingMenu.Load()
	if err != nil {
		m.Errorf("unable to load the configure controls window: %v", err.Error())
	}

	m.layouts[configureControlsLayoutID] = m.newConfigureControlsLayout(m.keyBindingMenu)

	m.selectSound, err = m.audioProvider.LoadSound(d2resource.SFXCursorSelect, false, false)
	if err != nil {
		m.Error(err.Error())
	}
}

// OnEscKey is called when the escape key is pressed
func (m *EscapeMenu) OnEscKey() {
	if m.currentLayout == configureControlsLayoutID {
		m.setLayout(optionsLayoutID)

		if err := m.keyBindingMenu.Close(); err != nil {
			m.Errorf("unable to close the configure controls menu: %v", err.Error())
		}
	} else {
		m.close()
	}
}

// SetOnCloseCb sets the callback that is run when close() is called
func (m *EscapeMenu) SetOnCloseCb(cb func()) {
	m.onCloseCb = cb
}

// SetSaver gives the menu the game's save (M4.6 B5). Without one, SAVE AND EXIT
// GAME leaves as it always did and SAVE GAME saves nothing.
func (m *EscapeMenu) SetSaver(s MenuSaver) {
	m.saver = s
}

func (m *EscapeMenu) close() {
	m.isOpen = false

	m.guiManager.SetLayout(nil)
	m.onCloseCb()
}

func (m *EscapeMenu) open() {
	m.isOpen = true

	// Before the layout is set, so the entries are placed around the line
	// they will show.
	m.askSave()
	m.setLayout(mainLayoutID)
}

// askSave asks the game, as the menu opens, whether it can be saved now, and
// shows the answer: nothing when it can; when it cannot, why, in plain words,
// and the exit entry reads EXIT WITHOUT SAVING (the build plan's rule 2).
func (m *EscapeMenu) askSave() {
	why := ""
	if m.saver != nil {
		why = m.saver.SaveRefusedNow()
	}

	if why == "" {
		m.showSave(false, "")
		return
	}

	m.showSave(true, why)
}

// showSave sets the exit entry's words and the line under the entries.
func (m *EscapeMenu) showSave(refused bool, note string) {
	m.exitRefused = refused

	exit := MenuSaveAndExit
	if refused {
		exit = MenuExitWithoutSave
	}

	if m.exitLabel != nil {
		if err := m.exitLabel.SetText(exit); err != nil {
			m.Errorf("could not set the exit entry's words: %v", err)
		}
	}

	lines := strings.SplitN(note, "\n", menuNoteLines)

	for i, l := range m.noteLabels {
		text := ""
		if i < len(lines) {
			text = lines[i]
		}

		if l == nil {
			continue
		}

		if err := l.SetText(text); err != nil {
			m.Errorf("could not set the save's line: %v", err)
		}
	}
}

// saveGame is SAVE GAME: saved, the menu closes and he plays on (the game puts
// the notice up); refused, or a save that failed, the menu stays and says why,
// and its exit entry leaves without saving.
func (m *EscapeMenu) saveGame() {
	if m.saver == nil {
		m.showSave(true, SaveRefusedOtherWords+"\n"+MenuExitWithoutSaved)
		return
	}

	if res := m.saver.SaveFromMenu(false); !res.Saved {
		m.showSave(true, res.Words)
		return
	}

	m.close()
}

// saveAndExit is the exit entry: SAVE AND EXIT GAME saves, then leaves for the
// main menu (whose unload does not write his .od2 and sidecar again: the save
// wrote them -- the B5 review, C2); EXIT WITHOUT SAVING leaves at once, and
// his last save stands (rule 2; in a network game, his hero and gear go with
// him). A save that fails keeps him here, the entry turned
// to EXIT WITHOUT SAVING, so leaving unsaved is always his choice, never the
// menu's.
func (m *EscapeMenu) saveAndExit() {
	if m.saver == nil || m.exitRefused {
		m.navigator.ToMainMenu()
		return
	}

	if res := m.saver.SaveFromMenu(true); !res.Saved {
		m.showSave(true, res.Words)
		return
	}

	m.navigator.ToMainMenu()
}

// Dismiss puts the menu away, whatever it shows, with no sound (the B5
// review, A1): the window's close runs the frames that let a fight's end
// settle before it saves, and the world is paused under the menu.
func (m *EscapeMenu) Dismiss() {
	if !m.isOpen {
		return
	}

	m.close()
}

func (m *EscapeMenu) playSound() {
	m.selectSound.Play()
}

func (m *EscapeMenu) showLayout(id layoutID) {
	m.playSound()

	if id == noLayoutID {
		m.close()
		return
	}

	if id == saveGameLayoutID {
		m.saveGame()
		return
	}

	if id == saveLayoutID {
		m.saveAndExit()
		return
	}

	m.setLayout(id)

	if id == configureControlsLayoutID {
		m.keyBindingMenu.Open()
	} else if err := m.keyBindingMenu.Close(); err != nil {
		m.Errorf("unable to close the configure controls menu: %v", err.Error())
	}
}

func (m *EscapeMenu) onHoverElement(id int) {
	_, y := m.layouts[m.currentLayout].actionableElements[id].GetOffset()
	m.layouts[m.currentLayout].currentEl = id

	x, _ := m.leftPent.GetPosition()
	m.leftPent.SetPosition(x, y+spacerWidth)

	x, _ = m.rightPent.GetPosition()
	m.rightPent.SetPosition(x, y+spacerWidth)
}

func (m *EscapeMenu) onUpdateValue(optID optionID, value string) {
	m.Infof("updating value %d with %s", optID, value)
}

func (m *EscapeMenu) setLayout(id layoutID) {
	layout := m.layouts[id]

	if layout.isRaw {
		m.guiManager.SetLayout(layout.Layout)
		m.layouts[id].rendered = true
		m.currentLayout = id

		return
	}

	m.leftPent = m.layouts[id].leftPent
	m.rightPent = m.layouts[id].rightPent
	m.currentLayout = id
	m.layouts[id].currentEl = len(m.layouts[id].actionableElements) - 1 // default to Previous Menu

	m.guiManager.SetLayout(m.layouts[id].Layout)

	// when first rendering a layout, widgets don't have offsets so we hide pentagrams for a frame
	if !m.layouts[id].rendered {
		m.layouts[id].rendered = true
		m.leftPent.SetVisible(false)
		m.rightPent.SetVisible(false)

		go func() {
			time.Sleep(singleFrame)
			m.onHoverElement(m.layouts[id].currentEl)
			m.leftPent.SetVisible(true)
			m.rightPent.SetVisible(true)
		}()
	} else {
		m.onHoverElement(m.layouts[id].currentEl)
	}
}

func (m *EscapeMenu) onUpKey() {
	if !m.isOpen {
		return
	}

	if m.layouts[m.currentLayout].currentEl == 0 {
		return
	}

	m.layouts[m.currentLayout].currentEl--
	m.onHoverElement(m.layouts[m.currentLayout].currentEl)
}

func (m *EscapeMenu) onDownKey() {
	if !m.isOpen {
		return
	}

	if m.layouts[m.currentLayout].currentEl == len(m.layouts[m.currentLayout].actionableElements)-1 {
		return
	}

	m.layouts[m.currentLayout].currentEl++
	m.onHoverElement(m.layouts[m.currentLayout].currentEl)
}

func (m *EscapeMenu) onEnterKey() {
	if !m.isOpen {
		return
	}

	m.layouts[m.currentLayout].actionableElements[m.layouts[m.currentLayout].currentEl].Trigger()
}

// IsOpen returns whether the escape menu is open (visible) or not
func (m *EscapeMenu) IsOpen() bool {
	return m.isOpen
}

// Advance computes the state of the elements of the menu overtime
func (m *EscapeMenu) Advance(elapsed float64) error {
	if m.keyBindingMenu != nil {
		if err := m.keyBindingMenu.Advance(elapsed); err != nil {
			return err
		}
	}

	return nil
}

// Render will render the escape menu on the target surface
func (m *EscapeMenu) Render(target d2interface.Surface) error {
	if m.isOpen {
		if err := m.keyBindingMenu.Render(target); err != nil {
			return err
		}
	}

	return nil
}

// OnMouseButtonDown triggers whnever a mous button is pressed
func (m *EscapeMenu) OnMouseButtonDown(event d2interface.MouseEvent) bool {
	if !m.isOpen {
		return false
	}

	if m.currentLayout == configureControlsLayoutID {
		if err := m.keyBindingMenu.onMouseButtonDown(event); err != nil {
			m.Errorf("unable to handle mouse down on configure controls menu: %v", err.Error())
		}
	}

	return false
}

// OnMouseButtonUp triggers whenever a mouse button is released
func (m *EscapeMenu) OnMouseButtonUp(event d2interface.MouseEvent) bool {
	if !m.isOpen {
		return false
	}

	if m.currentLayout == configureControlsLayoutID {
		m.keyBindingMenu.onMouseButtonUp()
	}

	return false
}

// OnMouseMove triggers whenever the mouse moves within the renderer
func (m *EscapeMenu) OnMouseMove(event d2interface.MouseMoveEvent) bool {
	if !m.isOpen {
		return false
	}

	if m.currentLayout == configureControlsLayoutID {
		m.keyBindingMenu.onMouseMove(event)
	}

	return false
}

// OnKeyDown defines the actions of the Escape Menu when a key is pressed
func (m *EscapeMenu) OnKeyDown(event d2interface.KeyEvent) bool {
	if m.keyBindingMenu.IsOpen() {
		if err := m.keyBindingMenu.OnKeyDown(event); err != nil {
			m.Errorf("unable to handle key down on configure controls menu: %v", err.Error())
		}

		return false
	}

	switch event.Key() {
	case d2enum.KeyUp:
		m.onUpKey()
	case d2enum.KeyDown:
		m.onDownKey()
	case d2enum.KeyEnter:
		m.onEnterKey()
	default:
		return false
	}

	return true
}

// layoutName is the layout up, for the harness: "main", or the other layouts
// by what they hold.
func (m *EscapeMenu) layoutName() string {
	switch m.currentLayout {
	case mainLayoutID:
		return "main"
	case optionsLayoutID:
		return "options"
	case soundOptionsLayoutID, videoOptionsLayoutID, automapOptionsLayoutID:
		return "an options page"
	case configureControlsLayoutID:
		return "controls"
	default:
		return "none"
	}
}

// HarnessReport is the escape menu as it would be drawn (M4.6 B5), for the ui
// provider: which layout is up, the main menu's entries in order, the one the
// keys are on, the line under them, and whether the exit entry leaves without
// saving. It is this process's presentation -- a resumed game's menu has never
// been opened -- so the ui provider puts it in the digest's process part.
func (m *EscapeMenu) HarnessReport() map[string]interface{} {
	report := map[string]interface{}{
		"open":         m.isOpen,
		"layout":       m.layoutName(),
		"entries":      []string{},
		"selected":     "",
		"note":         "",
		"exit_refused": m.exitRefused,
	}

	report["note"] = m.noteText()

	main, ok := m.layouts[mainLayoutID]
	if !ok || main == nil {
		return report
	}

	entries := make([]string, 0, len(main.actionableElements))

	for _, el := range main.actionableElements {
		if l, isLabel := el.(*showLayoutLabel); isLabel && l.Label != nil {
			entries = append(entries, l.GetText())
		}
	}

	report["entries"] = entries

	if m.isOpen && m.currentLayout == mainLayoutID && main.currentEl >= 0 && main.currentEl < len(entries) {
		report["selected"] = entries[main.currentEl]
	}

	return report
}
