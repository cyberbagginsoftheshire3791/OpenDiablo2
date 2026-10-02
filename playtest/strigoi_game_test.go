//go:build playtest

package playtest

import (
	"strings"
	"testing"
)

// TestStrigoiIsTheGame (23 Sep 2026: "Our version can officially be Strigoi"):
// the game launched with NO switches is Strigoi's own -- the authored
// village, the Janissary's sheets, Strigoi's fonts and words -- and a first
// hour of play reads none of Diablo II's tiles, class art, fonts or string
// tables. (Straight into a game: the character-select screen, which still
// shows Diablo II's classes, is not visited.) Control first: -classic, as
// every other script launches, reads Diablo II's tiles and fonts.
func TestStrigoiIsTheGame(t *testing.T) {
	mpqIn := func(a map[string]any, area string) float64 {
		m, ok := sub(a, "by_area")[area].(map[string]any)
		if !ok {
			return 0
		}

		return num(m, "mpq")
	}

	classic := startWith(t, "-classic") // explicitly: this script's control is Diablo II's game, whatever start does
	classic.call("strigoi_pause", map[string]any{})
	classic.call("strigoi_start_game", map[string]any{
		"hero_name": "Classic", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	// THE SHIPPED VIEW (F5, 2 Oct 2026): -classic keeps Diablo II's -- the
	// close camera (1.0) and no fog -- with no -zoom or -fog given.
	if v, f := shippedView(classic); v != 1 || flag(t, f, "enabled") || str(f, "off_reason") != "classic" {
		t.Fatalf("control: -classic starts at zoom %v with fog enabled=%v (off_reason %q); want 1, false, \"classic\"",
			v, f["enabled"], str(f, "off_reason"))
	}

	c := sub(classic.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")
	if mpqIn(c, "data/global/tiles") == 0 || mpqIn(c, "data/local/font") == 0 || str(c, "font_set") != "" {
		t.Fatalf("control: -classic read %.0f Diablo II tile and %.0f font files, font set %q; want some, some, none",
			mpqIn(c, "data/global/tiles"), mpqIn(c, "data/local/font"), str(c, "font_set"))
	}

	// The ten tables Strigoi's game no longer reads (M5.3's tables burst,
	// 26 Sep 2026) are still Diablo II's game's: -classic loads every one.
	for _, table := range diabloGameTables {
		if !censusHas(c, table) {
			t.Errorf("control: -classic did not load %s", table)
		}
	}

	// And its right button still casts Diablo II's right skill: the cast's
	// missile and overlay tables load with it.
	cx, cy := pair(classic.call("strigoi_get_player", map[string]any{}), "screen")
	classic.call("strigoi_click", map[string]any{"x": int(cx) + 60, "y": int(cy), "button": "right"})
	classic.call("strigoi_step", map[string]any{"frames": 30})

	if c = sub(classic.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state"); !censusHas(c, "/data/global/excel/missiles.txt") {
		t.Errorf("control: a -classic right-click beside him (screen %.0f,%.0f) cast nothing -- missiles.txt unread", cx+60, cy)
	}

	classic.stop()

	s := startWith(t) // no switches
	s.call("strigoi_pause", map[string]any{})

	g := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Janissary", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	if !strings.HasSuffix(str(g, "map_built"), "village.tmj") || str(g, "map_error") != "" {
		t.Fatalf("the world is %q (error %q), want the village", str(g, "map_built"), str(g, "map_error"))
	}

	if !strings.Contains(str(g, "hero_used"), "data/strigoi/hero/") || str(g, "hero_error") != "" {
		t.Fatalf("the hero is drawn from %q (error %q), want Strigoi's sheets", str(g, "hero_used"), str(g, "hero_error"))
	}

	if body := str(sub(s.call("strigoi_get_player", map[string]any{}), "state"), "body"); body != "png" {
		t.Fatalf("the player is drawn by %q, want png", body)
	}

	// THE SHIPPED VIEW (F5, 2 Oct 2026; Josh's rulings of 1 Oct): with no
	// switches the camera is at 0.5 and fog of war is on. The control above
	// is -classic's 1.0 and no fog, read through the same two providers.
	// Negative control (2 Oct 2026): -fog's default put back to false, this
	// fails "with no switches fog is enabled=false" (wt-fog5\nc\).
	if v, f := shippedView(s); v != 0.5 || !flag(t, f, "enabled") {
		t.Fatalf("with no switches the game starts at zoom %v with fog enabled=%v (off_reason %q); want 0.5 and fog on",
			v, f["enabled"], str(f, "off_reason"))
	}

	for _, k := range []string{"i", "i", "t", "t", "h", "h"} {
		s.call("strigoi_key", map[string]any{"key": k})
		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	// The HUD's button menu opens Strigoi's panels, as the keys do: its
	// inventory button is the kit and its skill button the talents. Until
	// 23 Sep 2026 they opened Diablo II's grid (with OpenDiablo2's test items
	// in it) and Diablo II's skill tree.
	miniButton := func(name string) {
		t.Helper()

		b := sub(sub(uiState(s), "mini_panel_buttons"), name)
		if len(b) == 0 {
			t.Fatalf("no mini-panel button %q: %v", name, uiState(s)["mini_panel_buttons"])
		}

		if !flag(t, b, "visible") {
			t.Fatalf("the mini-panel's %q button is not drawn: %v", name, b)
		}

		s.call("strigoi_click", map[string]any{
			"x": int(mustNum(t, b, "x") + mustNum(t, b, "w")/2), "y": int(mustNum(t, b, "y") + mustNum(t, b, "h")/2),
			"button": "left",
		})
		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	for _, c := range []struct{ button, strigoi, diablo, key string }{
		{"inventory", "kit_open", "inventory_open", "i"},
		{"skills", "talent_open", "skilltree_open", "t"},
	} {
		if !flag(t, uiState(s), "mini_panel_open") {
			miniButton("open_close")
		}

		if !flag(t, uiState(s), "mini_panel_open") {
			t.Fatalf("the HUD's menu button did not open the mini-panel: %v", uiState(s)["mini_panel_buttons"])
		}

		miniButton(c.button)

		ui := uiState(s)
		if !flag(t, ui, c.strigoi) || flag(t, ui, c.diablo) {
			t.Fatalf("the mini-panel's %s button opened %s=%v, Diablo II's %s=%v; want Strigoi's",
				c.button, c.strigoi, ui[c.strigoi], c.diablo, ui[c.diablo])
		}

		s.call("strigoi_screenshot", map[string]any{"name": "mini-panel-" + c.button})
		s.call("strigoi_key", map[string]any{"key": c.key}) // closed as the key opens it
		s.call("strigoi_step", map[string]any{"frames": 4})

		if flag(t, uiState(s), c.strigoi) {
			t.Fatalf("the %s key did not close what the %s button opened", c.key, c.button)
		}
	}

	for i := 0; i < 4; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 15.0})
	}

	// THE VILLAGE'S SOUND IS ITS OWN (M5.3's tables burst; the review's B4,
	// 27 Sep 2026): village.tmj says sound_env 1, the game sets it from the
	// map -- not from levels.txt, which Strigoi's game does not load -- and
	// that environment's song plays: Diablo II's Act 1 town music (sounds.txt
	// names it act1/town1.wav, under data/global/music) until Strigoi has its
	// own. Nothing asserted this until the review.
	village := sub(s.call("strigoi_get_system_state", map[string]any{"system": "village"}), "state")
	music := strings.ToLower(strings.ReplaceAll(str(village, "music"), `\`, "/"))

	if env := mustNum(t, village, "sound_env"); env != 1 || music != "act1/town1.wav" {
		t.Errorf("in the village the sound environment is %.0f playing %q; want 1, the map's own, playing act1/town1.wav", env, music)
	}

	s.call("strigoi_screenshot", map[string]any{"name": "strigoi"})

	a := sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")

	if str(a, "font_set") != "/data/strigoi/fonts/fonts.json" || str(a, "string_set") != "/data/strigoi/strings/strings.json" {
		t.Fatalf("font set %q, string table %q: want Strigoi's", str(a, "font_set"), str(a, "string_set"))
	}

	// Diablo II's tables: exactly 4 of its 83 (history items 111, 113, 116,
	// and M5.3's tables burst) -- sounds, soundenviron, monstats, monstats2.
	// The generated world's six load only when one is built, the missiles and
	// cast overlays only when a -classic skill is cast, and ten are Diablo
	// II's game's alone: the hero's body is his manifest's, his loadout his
	// kit, his right hand the torch, the village's sound and name its own.
	// Exact since the review (27 Sep 2026): a bound of "at most 4" could not
	// see a table that left by accident.
	if n := mpqIn(a, "data/global/excel"); n != 4 {
		t.Errorf("data/global/excel: %.0f table(s) from Diablo II's MPQs, want exactly 4", n)
	}

	for _, table := range diabloGameTables {
		if censusHas(a, table) {
			t.Errorf("%s was read: it is Diablo II's game's, not Strigoi's", table)
		}
	}

	// The census's area names are what this reads: an area it still reads
	// from the MPQs (the UI) must show, or the zeros below prove nothing.
	if mpqIn(a, "data/global/ui") == 0 {
		t.Fatalf("the census shows no Diablo II UI file; its areas are not what this script reads: %v", sub(a, "by_area"))
	}

	// data/local is Diablo II's language file (data/local/use), unread when
	// the words and fonts are Strigoi's.
	for _, area := range []string{"data/global/tiles", "data/global/chars", "data/local/font", "data/local/lng", "data/local"} {
		if n := mpqIn(a, area); n != 0 {
			t.Errorf("%s: %.0f file(s) from Diablo II's MPQs, want none", area, n)
		}
	}

	// Every word the game asked for is in Strigoi's table: a missing key is
	// drawn as the key itself ("panelcmini" on the mini-panel's close tooltip
	// was the first found this way, 23 Sep 2026).
	if mustNum(t, a, "strings_missing") != 0 {
		var missing []string

		for _, raw := range asList(a["strings"]) {
			if k, ok := raw.(map[string]any); !ok {
				t.Fatalf("a string census row is not an object: %v", raw)
			} else if k["found"] == false {
				missing = append(missing, str(k, "key"))
			}
		}

		t.Errorf("%.0f key(s) the game asked for are not in Strigoi's string table: %v", mustNum(t, a, "strings_missing"), missing)
	}

	// THE RIGHT BUTTON IS THE TORCH (Josh, 25 Sep 2026): a right-click beside
	// him lights the torch in his hand, as L does, and casts nothing -- the
	// cast's tables and the skill tables stay unread (history item 116 made
	// them load with the first cast; that cast is -classic's now, above). The
	// HUD's two skill icons are his hands, drawn as the keys' letters until
	// their art lands, and Diablo II's skill-icon sheet is not read.
	cast := []string{"/data/global/excel/missiles.txt", "/data/global/excel/overlay.txt",
		"/data/global/excel/skills.txt", "/data/global/excel/skilldesc.txt", "/data/global/ui/spells/skillicon.dc6"}

	if hands := sub(uiState(s), "hand_icons"); str(hands, "left") != "F" || str(hands, "right") != "L" {
		t.Errorf("the HUD's skill icons draw %q and %q; want his hands' keys, F and L", str(hands, "left"), str(hands, "right"))
	}

	sx, sy := pair(s.call("strigoi_get_player", map[string]any{}), "screen")
	if sx == 0 && sy == 0 {
		t.Fatal("strigoi_get_player reported no screen position; the right-click would miss him")
	}

	lit := flag(t, sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state"), "carried_lit")

	s.call("strigoi_click", map[string]any{"x": int(sx) + 60, "y": int(sy), "button": "right"})
	s.call("strigoi_step", map[string]any{"frames": 30})

	if now := flag(t, sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state"), "carried_lit"); now == lit {
		t.Errorf("a right-click beside him (screen %.0f,%.0f) did not light or douse his torch (lit %v -> %v)", sx+60, sy, lit, now)
	}

	after := sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")
	for _, file := range cast {
		if censusHas(after, file) {
			t.Errorf("a right-click beside him read %s: Strigoi's right hand casts nothing", file)
		}
	}

	// J1 (24 Sep 2026): Q opens his journal now, not Diablo II's quest log --
	// so the quest log's art is never read, before Q or after it.
	const questArt = "/data/global/ui/menu/a1q1.dc6"

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 4})

	if ui := uiState(s); !flag(t, ui, "journal_open") || flag(t, ui, "quest_log_open") {
		t.Fatalf("Q: journal_open %v, quest_log_open %v; want the journal and not the quest log",
			flag(t, ui, "journal_open"), flag(t, ui, "quest_log_open"))
	}

	if censusHas(sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state"), questArt) {
		t.Errorf("Q read %s: Diablo II's quest log was opened behind the journal", questArt)
	}

	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 4})

	if flag(t, uiState(s), "journal_open") {
		t.Fatal("Q did not close the journal")
	}

	// Diablo II's character panel loads its art when first opened (history
	// item 114): none of it was read above, it still opens and closes on its
	// key, and opening it does read its art -- so the absence above was a
	// measurement.
	for _, c := range []struct{ key, open, art string }{
		{"c", "hero_stats_open", "/data/global/ui/panel/invchar6.dc6"},
	} {
		if censusHas(a, c.art) {
			t.Errorf("%s was read before its panel (%s) was opened", c.art, c.open)
		}

		s.call("strigoi_key", map[string]any{"key": c.key})
		s.call("strigoi_step", map[string]any{"frames": 4})

		if !flag(t, uiState(s), c.open) {
			t.Fatalf("%s did not open its panel (%s)", c.key, c.open)
		}

		if !censusHas(sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state"), c.art) {
			t.Errorf("opening %s did not read %s; the census cannot see the panel's art", c.open, c.art)
		}

		s.call("strigoi_key", map[string]any{"key": c.key})
		s.call("strigoi_step", map[string]any{"frames": 4})

		if flag(t, uiState(s), c.open) {
			t.Fatalf("%s did not close its panel (%s)", c.key, c.open)
		}
	}

	// Diablo II's item tables are not gone, only late: the debug console's
	// spawnitem and the harness's item spawn still make a Diablo II item, and
	// all three tables load with the first one (M5.3's tables burst) -- misc
	// among them, which nothing else reads.
	p := s.call("strigoi_get_player", map[string]any{})
	s.call("strigoi_spawn_entity", map[string]any{"kind": "item", "code": "hax", "x": num(p, "x") + 1, "y": num(p, "y")})

	items := sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")
	for _, table := range []string{"/data/global/excel/weapons.txt", "/data/global/excel/armor.txt", "/data/global/excel/misc.txt"} {
		if !censusHas(items, table) {
			t.Errorf("a Diablo II item was spawned and %s was not loaded for it", table)
		}
	}

	t.Logf("Strigoi, as launched: %.0f MPQ files, %.0f of ours, %.0f string keys asked (%.0f missing)",
		num(a, "mpq_files"), num(a, "native_files"), num(a, "strings_asked"), num(a, "strings_missing"))
}

// shippedView is the scale a game is drawn at (ui.view_scale) and its fog
// provider's state.
func shippedView(s *session) (float64, map[string]any) {
	return num(uiState(s), "view_scale"), sub(s.call("strigoi_get_system_state", map[string]any{"system": "fog"}), "state")
}

// diabloGameTables are the ten Diablo II tables M5.3's tables burst (26 Sep
// 2026) took out of Strigoi's game and left in -classic's: the experience
// bar's, the item tables and the grid's layout, the class rows and skills,
// the level types and levels.
var diabloGameTables = []string{
	"/data/global/excel/experience.txt",
	"/data/global/excel/misc.txt", "/data/global/excel/weapons.txt", "/data/global/excel/armor.txt",
	"/data/global/excel/inventory.txt",
	"/data/global/excel/charstats.txt", "/data/global/excel/skills.txt", "/data/global/excel/skilldesc.txt",
	"/data/global/excel/lvltypes.txt", "/data/global/excel/levels.txt",
}

// censusHas reports whether the assets census lists a file (paths compared
// case-blind and slash-normalised, as the loader records them).
func censusHas(assets map[string]any, path string) bool {
	want := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))

	for _, raw := range asList(assets["files"]) {
		if f, ok := raw.(map[string]any); ok && strings.ToLower(strings.ReplaceAll(str(f, "path"), `\`, "/")) == want {
			return true
		}
	}

	return false
}
