package d2gamescreen

import (
	"image/color"
	"math"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2screen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

const strigoiMenuBackground = "/data/strigoi/ui/menu/village-dusk-background-v1.png"

// Cache complete labels so fitting scales their spacing as well as glyphs.
type strigoiMenuLabel struct {
	*d2ui.Label
	surface d2interface.Surface
	text    string
	ink     [4]uint32
}

type strigoiMenu struct {
	background                               *d2ui.Sprite
	title, subtitle, heading, detail, footer *strigoiMenuLabel
	errorText                                *strigoiMenuLabel
	tools, back                              *d2ui.Button
	buttons                                  map[string]*d2ui.Button
	owned                                    []d2ui.Widget
}

func (v *MainMenu) loadStrigoiMenu(loading d2screen.LoadingState) {
	m := &strigoiMenu{buttons: map[string]*d2ui.Button{}}
	v.nativeMenu = m
	// Silence the inherited title music for this original visual menu. Game
	// ambience resumes through the destination screen's normal audio path.
	v.audioProvider.PlayBGM("")
	var err error
	m.background, err = v.uiManager.NewSprite(strigoiMenuBackground, "")
	if err != nil {
		v.Errorf("Strigoi menu background: %v; drawing the native dark fallback", err)
	}
	label := func(font, text string, ink color.Color) *strigoiMenuLabel {
		l := v.uiManager.NewLabel(font, d2resource.PaletteUnits)
		l.SetText(text)
		l.Color[0] = ink
		m.owned = append(m.owned, l)
		return &strigoiMenuLabel{Label: l}
	}
	m.title = label(d2resource.Font42, "S T R I G O I", color.RGBA{R: 219, G: 194, B: 144, A: 255})
	m.subtitle = label(d2resource.FontFormal12, "WALLACHIA  /  JUNE 1462", color.RGBA{R: 165, G: 162, B: 140, A: 255})
	m.heading = label(d2resource.Font24, "", color.RGBA{R: 222, G: 208, B: 176, A: 255})
	m.detail = label(d2resource.FontFormal12, "", color.RGBA{R: 195, G: 198, B: 180, A: 255})
	m.footer = label(d2resource.FontFormal10, "Development build", color.RGBA{R: 128, G: 134, B: 121, A: 255})
	button := func(name, text string, y int, action func()) *d2ui.Button {
		b := v.uiManager.NewMenuButton(text, 250, 40)
		b.SetPosition(64, y)
		b.OnActivated(action)
		m.buttons[name] = b
		m.owned = append(m.owned, b)
		return b
	}
	v.singlePlayerButton = button("single_player", "PLAY", 224, v.onSinglePlayerClicked)
	v.multiplayerButton = button("other_multiplayer", "MULTIPLAYER", 273, v.onMultiplayerClicked)
	v.creditsButton = button("credits", "CREDITS", 322, func() { v.SetScreenMode(ScreenModeProjectCredits) })
	m.tools = button("tools", "TOOLS", 371, func() { v.SetScreenMode(ScreenModeTools) })
	v.exitDiabloButton = button("exit", "QUIT", 450, v.onExitButtonClicked)
	v.editorButton = button("world_editor", "WORLD EDITOR", 224, v.onWorldEditorClicked)
	v.mapTestButton = button("map_engine_test", "MAP ENGINE TEST", 273, v.onMapTestClicked)
	v.githubButton = button("project_website", "ENGINE SOURCE", 322, v.onGithubButtonClicked)
	m.back = button("back", "BACK", 450, func() { v.SetScreenMode(ScreenModeMainMenu) })
	// Keep the working LAN flow; do not imply a new co-op feature.
	v.networkTCPIPButton = button("tcp_ip", "LOCAL NETWORK", 224, v.onNetworkTCPIPClicked)
	v.networkCancelButton = button("network_back", "BACK", 450, v.onNetworkCancelClicked)
	v.btnTCPIPHostGame = button("host", "HOST GAME", 224, v.onTCPIPHostGameClicked)
	v.btnTCPIPJoinGame = button("join", "JOIN GAME", 273, v.onTCPIPJoinGameClicked)
	v.btnTCPIPCancel = button("tcp_back", "BACK", 450, v.onTCPIPCancelClicked)
	v.btnServerIPOk = button("join_confirm", "JOIN", 322, v.onBtnTCPIPOkClicked)
	v.btnServerIPCancel = button("join_back", "BACK", 450, v.onBtnTCPIPCancelClicked)
	v.tcpJoinGameEntry = v.uiManager.NewMenuTextbox()
	v.tcpJoinGameEntry.SetPosition(64, 272)
	v.tcpJoinGameEntry.SetFilter(joinGameCharacterFilter)
	m.owned = append(m.owned, v.tcpJoinGameEntry)
	if v.errorLabel != nil {
		// Preserve the complete diagnostic for the log and harness; a bounded
		// on-screen summary stays readable instead of shrinking a long path.
		display := []rune(v.errorLabel.GetText())
		if len(display) > 170 {
			display = append(display[:167], '.', '.', '.')
		}
		m.errorText = label(d2resource.FontFormal12,
			strings.Join(d2util.SplitIntoLinesWithMaxWidth(string(display), 85), "\n"),
			color.RGBA{R: 233, G: 172, B: 138, A: 255})
		m.owned = append(m.owned, v.errorLabel)
	}
	v.SetScreenMode(ScreenModeMainMenu)
	if err = v.inputManager.BindHandler(v); err != nil {
		v.Errorf("binding Strigoi menu: %v", err)
	}
	d2harness.Register(mainMenuProvider{v})
	loading.Progress(ninetyPercent)
}

func (v *MainMenu) unloadStrigoiMenu() {
	for _, w := range v.nativeMenu.owned {
		w.SetVisible(false)
		if err := v.inputManager.UnbindHandler(w); err != nil {
			v.Errorf("unbinding Strigoi menu widget: %v", err)
		}
	}
}

func (v *MainMenu) setStrigoiMenuMode(mode mainMenuScreenMode) {
	v.screenMode = mode
	m := v.nativeMenu
	for _, b := range m.buttons {
		b.SetVisible(false)
	}
	v.tcpJoinGameEntry.SetVisible(false)
	m.heading.SetText("")
	m.detail.SetText("")
	show := func(names ...string) {
		for _, name := range names {
			m.buttons[name].SetVisible(true)
		}
	}
	switch mode {
	case ScreenModeMainMenu:
		show("single_player", "other_multiplayer", "credits", "tools", "exit")
	case ScreenModeTools:
		show("world_editor", "map_engine_test", "project_website", "back")
		m.heading.SetText("Tools")
	case ScreenModeProjectCredits:
		show("back")
		m.heading.SetText("Credits")
		m.detail.SetText("A game by Josh\n\nBuilt on OpenDiablo2 (GPL v3)\nProject art: Josh, with Codex assistance\nMenu background: OpenAI image generation\nFonts: IM FELL and Uncial Antiqua (OFL)\n\nFull asset sources and licenses: CREDITS.md")
	case ScreenModeMultiplayer:
		show("tcp_ip", "network_back")
		m.heading.SetText("Multiplayer")
		m.detail.SetText("Existing local-network engine test mode.")
	case ScreenModeTCPIP:
		show("host", "join", "tcp_back")
		m.heading.SetText("Local network")
		m.detail.SetText("Your address: " + v.getLocalIP())
	case ScreenModeServerIP:
		show("join_confirm", "join_back")
		m.heading.SetText("Join a game")
		v.tcpJoinGameEntry.SetVisible(true)
		v.tcpJoinGameEntry.Activate()
		m.detail.SetText("Enter the host's address.")
	}
	// SetText restores the label's default ink; apply our tint afterwards.
	m.heading.Color[0] = color.RGBA{R: 222, G: 208, B: 176, A: 255}
	m.detail.Color[0] = color.RGBA{R: 195, G: 198, B: 180, A: 255}
}

func renderMenuLabel(screen d2interface.Surface, l *strigoiMenuLabel, x, y, maxW, maxH int, preferred float64) {
	w, h := l.GetSize()
	if w <= 0 || h <= 0 {
		return
	}
	fit := math.Min(preferred, math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h)))
	r, g, b, a := l.Color[0].RGBA()
	ink := [4]uint32{r, g, b, a}
	if l.surface == nil || l.text != l.GetText() || l.ink != ink {
		l.surface = screen.Renderer().NewSurface(w, h)
		l.SetPosition(0, 0)
		l.Label.Render(l.surface)
		l.text, l.ink = l.GetText(), ink
	}
	screen.PushTranslation(x, y)
	screen.PushFilter(d2enum.FilterLinear)
	screen.PushScale(fit, fit)
	screen.Render(l.surface)
	screen.PopN(3)
}

func (v *MainMenu) renderStrigoiMenu(screen d2interface.Surface) {
	m := v.nativeMenu
	screen.DrawRect(800, 600, color.RGBA{R: 11, G: 16, B: 15, A: 255})
	if m.background != nil {
		w, h := m.background.GetSize()
		screen.PushFilter(d2enum.FilterLinear)
		screen.PushScale(800/float64(w), 600/float64(h))
		screen.Render(m.background.GetSurface())
		screen.PopN(2)
	}
	// A subtle shade preserves quiet title space even on bright displays.
	screen.DrawRect(350, 600, color.RGBA{R: 7, G: 12, B: 10, A: 68})
	renderMenuLabel(screen, m.title, 64, 87, 264, 65, 1.55)
	renderMenuLabel(screen, m.subtitle, 66, 162, 260, 20, 1)
	screen.PushTranslation(64, 194)
	screen.DrawRect(250, 1, color.RGBA{R: 120, G: 101, B: 64, A: 255})
	screen.Pop()
	renderMenuLabel(screen, m.heading, 64, 200, 250, 24, 1)
	if v.screenMode == ScreenModeProjectCredits {
		screen.PushTranslation(350, 190)
		screen.DrawRect(426, 290, color.RGBA{R: 11, G: 17, B: 15, A: 235})
		screen.Pop()
		renderMenuLabel(screen, m.detail, 372, 218, 384, 235, 1)
	} else {
		renderMenuLabel(screen, m.detail, 64, 380, 265, 52, 1)
	}
	renderMenuLabel(screen, m.footer, 64, 565, 250, 18, 1)
	if v.errorLabel != nil && v.errorLabel.GetText() != "" {
		screen.PushTranslation(20, 512)
		screen.DrawRect(760, 48, color.RGBA{R: 13, G: 17, B: 15, A: 244})
		screen.Pop()
		// Errors are visible on return to the native main menu, not on a splash.
		renderMenuLabel(screen, m.errorText, 32, 518, 736, 36, 1)
	}
}

func (v *MainMenu) strigoiMenuKey(event d2interface.KeyEvent) bool {
	if event.Key() != d2enum.KeyEscape {
		return false
	}
	switch v.screenMode {
	case ScreenModeMainMenu:
		// An accidental Escape from a returned editor does not quit the process.
		return true
	case ScreenModeTCPIP:
		v.SetScreenMode(ScreenModeMultiplayer)
	case ScreenModeServerIP:
		v.SetScreenMode(ScreenModeTCPIP)
	default:
		v.SetScreenMode(ScreenModeMainMenu)
	}
	return true
}
