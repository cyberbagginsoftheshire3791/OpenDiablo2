//go:build playtest

package playtest

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestMainMenuLabelsRead is BUG-27 (28 Sep 2026): the first screen a player
// sees has words on its buttons. In the default game every main-menu button
// was a blank grey bar -- the labels WERE drawn, but a Strigoi font's warm-white
// ink under Diablo II's grey button tint came out at the stone's own grey
// (d2ui.buttonLabelColor). A harness field could not have seen it: the text
// was set and the label drawn. So this reads PIXELS.
//
// It boots to the menu (strigoi_navigate's "main_menu" is the trademark
// splash, which draws no buttons -- the finder's instrument warning), clicks
// past the splash off every button, and screenshots the menu with the cursor
// clear of them. The main menu's "ui" provider reports each button's rect and
// the rect its label was drawn in; within the label rect, on the button's
// face, the letters must stand out from the stone (labelInk).
//
// Control first: -classic, Diablo II's fonts, whose labels a player has always
// read -- so the instrument is shown to see letters before it is asked to.
func TestMainMenuLabelsRead(t *testing.T) {
	classic := startWith(t, "-classic") // the control is Diablo II's game, whatever start does
	if bad := mainMenuLabelInk(t, classic, "classic"); len(bad) > 0 {
		t.Fatalf("control: under -classic, Diablo II's own labels measured as unreadable -- the instrument, not the game: %s",
			strings.Join(bad, "; "))
	}

	classic.stop()

	s := startWith(t) // no switches: Strigoi's fonts and words
	if bad := mainMenuLabelInk(t, s, "strigoi"); len(bad) > 0 {
		t.Errorf("in the default game these main-menu labels cannot be read (BUG-27): %s", strings.Join(bad, "; "))
	}
}

// What counts as a label's ink, and how much of it a label needs.
//
// The first version measured against the label rect's own median and went
// GREEN on BUG-27's blank CREDITS and CINEMATICS (0.156, 0.177): Strigoi's
// Uncial face is nearly as tall as those short buttons, so the rect took in
// their dark border and gold trim, and the border was counted as letters.
// So the rect is cut to the button's FACE (labelFaceInsetX/Y in from its
// edges, clear of the end ornaments and the trim) and each row is compared
// with the stone of that row across the face. Measured with this instrument
// on 28 Sep 2026 (logs in strigoi-harness-runs\wt-menu\): BUG-27, the fix
// reverted, 0.000-0.007 on all eight; Diablo II's labels under -classic
// 0.059 (its small CINEMATICS) to 0.225; Strigoi's with the fix 0.158-0.176.
// The bar sits between the blank and the faintest label anyone has read.
const (
	labelInkContrast = 60   // luma steps from the face's row median that count as ink
	minLabelInk      = 0.03 // share of the label's face pixels that must be ink
	labelFaceInsetX  = 24   // the end ornaments (jewels) of Diablo II's button art
	labelFaceInsetY  = 3    // the gold trim along the top and bottom edges
)

// mainMenuButtons are the eight buttons the main menu's first page shows.
var mainMenuButtons = []string{
	"single_player", "other_multiplayer", "project_website", "map_engine_test",
	"world_editor", "credits", "cinematics", "exit",
}

// mainMenuLabelInk boots s to the main menu, screenshots it, and returns one
// line per button whose label does not read; it fails the test outright if
// the menu or its report is not what it should be, so an empty result is a
// measurement and never a menu that showed nothing.
func mainMenuLabelInk(t *testing.T, s *session, game string) []string {
	t.Helper()

	// The menu loads asynchronously; its provider registers when it has.
	var ui map[string]any

	uiErr := "not read"
	for i := 0; i < 120; i++ {
		if uiErr = s.callErr("strigoi_get_system_state", map[string]any{"system": "ui"}); uiErr == "" {
			if ui = uiState(s); str(ui, "screen") == "main_menu" {
				break
			}
		}

		s.call("strigoi_step", map[string]any{"frames": 10})
	}

	if str(ui, "screen") != "main_menu" {
		t.Fatalf("%s: the main menu never reported itself as the ui system (last error %q, state %v)", game, uiErr, ui)
	}

	// Past the trademark splash with a click OFF every button (a release over
	// SINGLE PLAYER would start a game), which also parks the cursor there.
	const parkX, parkY = 700, 150

	for i := 0; i < 10 && str(ui, "main_menu_page") != "main_menu"; i++ {
		s.call("strigoi_click", map[string]any{"x": parkX, "y": parkY, "button": "left"})
		s.call("strigoi_step", map[string]any{"frames": 10})
		ui = uiState(s)
	}

	if page := str(ui, "main_menu_page"); page != "main_menu" {
		t.Fatalf("%s: the menu is on page %q after clicking past the splash, want main_menu", game, page)
	}

	s.call("strigoi_move_cursor", map[string]any{"x": parkX, "y": parkY})
	s.call("strigoi_step", map[string]any{"frames": 30})

	ui = uiState(s)
	buttons := sub(ui, "main_menu_buttons")

	shot := s.call("strigoi_screenshot", map[string]any{"name": "main-menu-labels-" + game})
	shotPath := str(shot, "path")

	f, err := os.Open(shotPath)
	if err != nil {
		t.Fatalf("%s: the screenshot is not on disk: %v", game, err)
	}

	img, err := png.Decode(f)
	_ = f.Close()

	if err != nil {
		t.Fatalf("%s: decoding the screenshot: %v", game, err)
	}

	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Fatalf("%s: the screenshot is %dx%d, want the 800x600 frame the rects are in", game, b.Dx(), b.Dy())
	}

	var bad, measured []string

	for _, name := range mainMenuButtons {
		b := sub(buttons, name)
		if len(b) == 0 {
			t.Fatalf("%s: the menu reports no %q button: %v", game, name, keysOf(buttons))
		}

		if !flag(t, b, "visible") || str(b, "text") == "" {
			t.Fatalf("%s: the %s button is not drawn with a label (visible %v, text %q)", game, name, b["visible"], str(b, "text"))
		}

		button := image.Rect(int(mustNum(t, b, "x")), int(mustNum(t, b, "y")),
			int(mustNum(t, b, "x")+mustNum(t, b, "w")), int(mustNum(t, b, "y")+mustNum(t, b, "h")))
		label := image.Rect(int(mustNum(t, b, "label_x")), int(mustNum(t, b, "label_y")),
			int(mustNum(t, b, "label_x")+mustNum(t, b, "label_w")), int(mustNum(t, b, "label_y")+mustNum(t, b, "label_h")))

		if label.Intersect(button.Inset(labelFaceInsetY)).Empty() {
			t.Fatalf("%s: the %s button's label rect %v is empty or off the button %v", game, name, label, button)
		}

		ink := labelInk(img, label, button)
		measured = append(measured, fmt.Sprintf("%s %q %.3f in %v on %v", name, str(b, "text"), ink, label, button))

		if ink < minLabelInk {
			bad = append(bad, fmt.Sprintf("%s %q: %.3f of its label rect %v is ink, want at least %.2f",
				name, str(b, "text"), ink, label, minLabelInk))
		}
	}

	sort.Strings(measured)
	t.Logf("%s main menu, %s -- ink per label: %s", game, shotPath, strings.Join(measured, ", "))

	return bad
}

// labelInk is the share of the label's pixels on the button's face whose
// luma is more than labelInkContrast from the stone of their row -- the
// median luma of that row across the whole face: the letters, light or dark,
// against what they are drawn on. A row a label covers is mostly stone even
// where letters are, so its median is the stone's.
func labelInk(img image.Image, label, button image.Rectangle) float64 {
	face := image.Rect(button.Min.X+labelFaceInsetX, button.Min.Y+labelFaceInsetY,
		button.Max.X-labelFaceInsetX, button.Max.Y-labelFaceInsetY)
	measured := label.Intersect(face)

	luma := func(x, y int) int {
		cr, cg, cb, _ := img.At(x, y).RGBA()

		return int((299*cr+587*cg+114*cb)/1000) >> 8
	}

	ink, n := 0, 0

	for y := measured.Min.Y; y < measured.Max.Y; y++ {
		row := make([]int, 0, face.Dx())
		for x := face.Min.X; x < face.Max.X; x++ {
			row = append(row, luma(x, y))
		}

		sort.Ints(row)
		stone := row[len(row)/2]

		for x := measured.Min.X; x < measured.Max.X; x++ {
			if l := luma(x, y); l-stone > labelInkContrast || stone-l > labelInkContrast {
				ink++
			}

			n++
		}
	}

	if n == 0 {
		return 0
	}

	return float64(ink) / float64(n)
}
