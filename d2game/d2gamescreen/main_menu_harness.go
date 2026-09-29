package d2gamescreen

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// --- the main menu's "ui" harness provider ---------------------------------

// mainMenuProvider is the harness's "ui" system while the main menu is the
// screen, as the game controls are while a game is (BUG-27, 28 Sep 2026). It
// reports which of the menu's pages is showing and where each of its buttons
// is -- and where on it the button drew its label -- so a playtest can read
// a screenshot of the first screen a player sees and measure each label's
// ink, the way mini_panel_buttons lets one click the HUD's buttons. OnLoad
// registers it once every button exists; OnUnload removes it.
type mainMenuProvider struct{ v *MainMenu }

func (p mainMenuProvider) HarnessName() string { return "ui" }

func (p mainMenuProvider) HarnessState() map[string]interface{} {
	v := p.v
	buttons := map[string]interface{}{}

	for name, b := range map[string]*d2ui.Button{
		"single_player":     v.singlePlayerButton,
		"other_multiplayer": v.multiplayerButton,
		"project_website":   v.githubButton,
		"map_engine_test":   v.mapTestButton,
		"world_editor":      v.editorButton,
		"credits":           v.creditsButton,
		"cinematics":        v.cinematicsButton,
		"exit":              v.exitDiabloButton,
	} {
		if b == nil {
			continue
		}

		x, y := b.GetPosition()
		w, h := b.GetSize()
		label := b.LabelRect()

		buttons[name] = map[string]interface{}{
			"x": x, "y": y, "w": w, "h": h,
			"visible": b.GetVisible(),
			"text":    b.Text(),
			// The label's rect on SCREEN, as drawn when the button is up.
			"label_x": x + label.Min.X, "label_y": y + label.Min.Y,
			"label_w": label.Dx(), "label_h": label.Dy(),
		}
	}

	// The line the menu was opened with, if any: why the last thing that sent
	// the game back here failed -- a game that could not start, or a map the
	// World Editor refused (the second 28 Sep review's B: a script must tell
	// the harness's refusal from any other way back to the menu).
	menuError := ""
	if v.errorLabel != nil {
		menuError = v.errorLabel.GetText()
	}

	return map[string]interface{}{
		"screen":            "main_menu",
		"main_menu_page":    mainMenuPageName(v.screenMode),
		"main_menu_buttons": buttons,
		"main_menu_error":   menuError,
	}
}

// mainMenuPageName names the menu's page: "trademark" (the splash a click
// leaves, which draws no buttons), "main_menu", "multiplayer", "tcp_ip",
// "server_ip" or "unknown".
func mainMenuPageName(mode mainMenuScreenMode) string {
	switch mode {
	case ScreenModeTrademark:
		return "trademark"
	case ScreenModeMainMenu:
		return "main_menu"
	case ScreenModeMultiplayer:
		return "multiplayer"
	case ScreenModeTCPIP:
		return "tcp_ip"
	case ScreenModeServerIP:
		return "server_ip"
	default:
		return "unknown"
	}
}
