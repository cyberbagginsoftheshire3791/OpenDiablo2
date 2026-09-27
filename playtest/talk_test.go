//go:build playtest

package playtest

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

// TestTalk is T4 (23 Sep 2026): the village's talk and its one number. Acts:
//
//  1. A villager's hover label is his ROLE, not the D2 stand-in's name.
//  2. THE CONTROL: a click on him from across the camp opens nothing.
//  3. In reach, a click opens the talk, and the world is held while it lasts.
//     The HUD's own d2ui buttons are held too: the mini-panel, open before
//     the talk, is closed under it and its menu button opens nothing, and
//     the run button does not toggle running (the polish burst, 27 Sep 2026:
//     the journal's treatment, J1 review B1). When the talk ends (act 4) the
//     mini-panel comes back open and the run button works again.
//  4. Answers move the number and cost what they cost: the civil answer +3,
//     an hour's digging exactly 60 world minutes and +8 -- and that reaches
//     the first rung, water.
//  5. The first rung is real: at the well, a click on the trough answer fills
//     his water. Below the rung (the control) she will not let him near.
//  6. It is on disk beside his save.
func TestTalk(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Talker", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	if v := villageState(s); !flag(t, v, "bound") || mustNum(t, v, "rep") != 10 {
		t.Fatalf("a stranger at the gate stands at 10: %v", v)
	}

	headman := villager(t, s, "Warriv")

	// --- 1: the role ------------------------------------------------------------
	// On the default game's village he stands off the first screen: walk
	// toward him until he is on it (act 2 walks away again anyway).
	hx, hy := screenOf(t, s, headman)
	if hx < 0 || hy < 0 || hx >= 800 || hy >= 600 {
		walkNear(t, s, headman)
		hx, hy = screenOf(t, s, headman)

		if hx < 0 || hy < 0 || hx >= 800 || hy >= 600 {
			t.Fatalf("act 1: walked to the headman and he is still off the screen at %d,%d", hx, hy)
		}
	}
	s.call("strigoi_move_cursor", map[string]any{"x": hx, "y": hy})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := str(uiState(s), "hover_label"); got != "The headman" {
		t.Fatalf("act 1: the hover label is the role; it reads %q", got)
	}

	// --- 2: out of reach (the control) -----------------------------------------
	// He spawns beside the headman, so walk off first: the control must run.
	walkAway(t, s, headman, TalkReach+0.4)

	far := distTo(t, s, headman)
	hx, hy = screenOf(t, s, headman)
	s.call("strigoi_click", map[string]any{"x": hx, "y": hy, "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, uiState(s), "talk_open") || flag(t, villageState(s), "talking") {
		t.Fatalf("act 2: %.1f tiles away is too far to talk", far)
	}

	// --- 3: in reach -------------------------------------------------------------
	walkNear(t, s, headman)

	// Not through another panel: with the talent panel open over him, the
	// click is the panel's (review finding -- the talk check runs first).
	s.call("strigoi_key", map[string]any{"key": "t"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if !flag(t, uiState(s), "talent_open") {
		t.Fatal("act 3: T opens the talent panel (the setup for the click-through check)")
	}

	hx, hy = screenOf(t, s, headman)
	s.call("strigoi_click", map[string]any{"x": hx, "y": hy, "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, uiState(s), "talk_open") {
		t.Fatal("act 3: a click on a villager under the talent panel opened a talk through it")
	}

	s.call("strigoi_key", map[string]any{"key": "t"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// The mini-panel open before the talk, so that it is the talk that
	// closes it (below) and the talk's end that brings it back (act 4).
	if !flag(t, uiState(s), "mini_panel_open") {
		clickMiniButton(t, s, "open_close")
	}

	if !flag(t, uiState(s), "mini_panel_open") {
		t.Fatal("act 3: the mini-panel's menu button opens it (the setup for the talk's hold on the HUD)")
	}

	openTalkWith(t, s, headman)

	if node := str(villageState(s), "node"); node != "headman_first" {
		t.Fatalf("act 3: a first meeting opens on the first-meeting node: %q", node)
	}

	before := worldMinutes(t, s)
	s.call("strigoi_step", map[string]any{"frames": 60})

	if after := worldMinutes(t, s); after != before {
		t.Fatalf("act 3: the world is held while he talks; the clock ran %.2f -> %.2f", before, after)
	}

	// The hold is named, and step_world refuses it rather than spinning to its
	// tick cap (history item 121). Act 4's hour is measured from `before`, so
	// it also proves the refusal moved nothing.
	if held := mustStr(t, uiState(s), "world_held_by"); held != "talk" {
		t.Fatalf("act 3: world_held_by %q while he talks, want \"talk\"", held)
	}

	if msg := s.callErr("strigoi_step_world", map[string]any{"world_minutes": 10.0}); !strings.Contains(msg, "WORLD_HELD") || !strings.Contains(msg, `"talk"`) {
		t.Fatalf("act 3: step_world during a talk: got %q, want WORLD_HELD naming \"talk\"", msg)
	}

	// The HUD's own buttons are d2ui widgets and take a click whatever the
	// controls answer; under the talk they must do nothing (the polish burst,
	// the journal's treatment). The talk closed the open mini-panel...
	if flag(t, uiState(s), "mini_panel_open") {
		t.Fatal("act 3: the mini-panel is still open under the talk; the talk closes it, as the journal does")
	}

	// ...its menu button opens nothing...
	clickMiniButton(t, s, "open_close")

	if ui := uiState(s); flag(t, ui, "mini_panel_open") || !flag(t, ui, "talk_open") {
		t.Fatalf("act 3: the HUD's menu button worked under the talk: mini-panel %v, talk %v",
			flag(t, ui, "mini_panel_open"), flag(t, ui, "talk_open"))
	}

	// ...and the run button does not toggle running.
	running := runToggled(t, s)
	clickRunButton(t, s)

	if ui := uiState(s); runToggled(t, s) != running || !flag(t, ui, "talk_open") {
		t.Fatalf("act 3: the run button worked under the talk: run toggled %v -> %v, talk %v",
			running, runToggled(t, s), flag(t, ui, "talk_open"))
	}

	// --- 4: answers ------------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "1"}) // "Only to live..."
	s.call("strigoi_step", map[string]any{"frames": 2})

	if v := villageState(s); mustNum(t, v, "rep") != 13 || str(v, "node") != "headman_work" {
		t.Fatalf("act 4: the civil answer is worth 3 and leads to work: %v", v)
	}

	s.call("strigoi_key", map[string]any{"key": "1"}) // dig
	s.call("strigoi_step", map[string]any{"frames": 2})

	v := villageState(s)
	if mustNum(t, v, "rep") != 21 || str(v, "rung") != "water" {
		t.Fatalf("act 4: the ditch is worth 8, and 21 is the first rung: %v", v)
	}

	if spent := worldMinutes(t, s) - before; math.Abs(spent-60) > 0.5 {
		t.Fatalf("act 4: an hour's digging is 60 world minutes; the clock moved %.2f", spent)
	}

	// Saved as it happens, not only on leaving.
	sidecar := str(game, "save_path") + ".strigoi.json"
	if rep := villageOnDisk(t, s, sidecar, 21); rep != 21 {
		t.Fatalf("act 4: an answer is saved as it is given; the file has %d", rep)
	}

	s.call("strigoi_key", map[string]any{"key": "1"}) // "Leave." -- the thanks has no answer
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, uiState(s), "talk_open") {
		t.Fatal("act 4: a node with nothing to answer ends on its one way out")
	}

	// The talk over, the HUD is his again: the mini-panel as it was before
	// the talk (open), and the run button toggles -- twice, to leave it be.
	if !flag(t, uiState(s), "mini_panel_open") {
		t.Fatal("act 4: the mini-panel, open before the talk, is not back when it ends")
	}

	running = runToggled(t, s)
	clickRunButton(t, s)

	if runToggled(t, s) == running {
		t.Fatal("act 4: the run button does nothing after the talk (the control for act 3's refusal)")
	}

	clickRunButton(t, s)

	if runToggled(t, s) != running {
		t.Fatal("act 4: a second click on the run button does not toggle it back")
	}

	// --- 5: the well -----------------------------------------------------------
	well := villager(t, s, "Kashya")
	setField(s, "meters", "water", 20.0)

	walkNear(t, s, well)
	openTalkWith(t, s, well)

	if node := str(villageState(s), "node"); node != "well" {
		t.Fatalf("act 5: at the water rung she lets him to the trough: %q", node)
	}

	view := sub(uiState(s), "talk_view")
	ys := asList(view["answer_y"])

	if len(ys) == 0 {
		t.Fatalf("act 5: the panel lists its answers: %v", view)
	}

	s.call("strigoi_click", map[string]any{"x": 200, "y": int(ys[0].(float64)) + 6, "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// Two frames of the world running again after the talk closed drain a
	// sliver; 20 -> ~100 is the fill.
	if water := mustNum(t, metersState(s), "water"); water < 99 {
		t.Fatalf("act 5: the trough fills his water; it reads %.1f", water)
	}

	// THE CONTROL: below the rung, the well is guarded.
	setField(s, "village", "rep", 10.0)
	openTalkWith(t, s, well)

	if node := str(villageState(s), "node"); node != "well_wary" {
		t.Fatalf("act 5 control: below the water rung she stands between him and the rope: %q", node)
	}

	s.call("strigoi_key", map[string]any{"key": "escape"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, uiState(s), "talk_open") {
		t.Fatal("act 5: Escape walks away")
	}

	// --- 6: on disk, and read back -------------------------------------------
	// A value no path produces by itself -- not the start, not an answer's.
	setField(s, "village", "rep", 33.0)
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})

	if rep := villageOnDisk(t, s, sidecar, 33); rep != 33 {
		t.Fatalf("act 6: leaving saves the village; the file has %d", rep)
	}

	s.call("strigoi_start_game", map[string]any{"save_path": str(game, "save_path"), "seed": 1462, "wait_seconds": 90})

	v = villageState(s)
	flags := map[string]bool{}

	for _, f := range asList(v["flags"]) {
		flags[f.(string)] = true
	}

	if mustNum(t, v, "rep") != 33 || !flags["ditch_mended"] || !flags["met_headman"] {
		t.Fatalf("act 6: a second session reads the village back: %v", v)
	}

	t.Logf("talked to the headman and the well; the village read back: %v", v)
}

// villageOnDisk polls the sidecar until its village rep is want, or gives up
// and returns what it last read.
func villageOnDisk(t *testing.T, s *session, path string, want int) int {
	t.Helper()

	var saved struct {
		Village struct {
			Rep int `json:"rep"`
		} `json:"village"`
	}

	for i := 0; i < 50; i++ {
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &saved) == nil && saved.Village.Rep == want {
			break
		}

		s.call("strigoi_step", map[string]any{"frames": 6})
	}

	return saved.Village.Rep
}

func villageState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "village"}), "state")
}

// villager finds the town NPC a D2 name draws.
func villager(t *testing.T, s *session, name string) string {
	t.Helper()

	entities := s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200})

	for _, raw := range asList(entities["items"]) {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		// By his label ("Warriv" under -classic) or by who stands in for him
		// (name_key): on the default game Strigoi's words label him "The
		// headman", and the stand-in is still Warriv.
		e := s.call("strigoi_get_entity", map[string]any{"handle": str(row, "handle")})
		if st := sub(e, "state"); str(st, "name") == name || str(st, "name_key") == name {
			return str(row, "handle")
		}
	}

	t.Fatalf("no villager drawn by %q in the town", name)

	return ""
}

func screenOf(t *testing.T, s *session, handle string) (int, int) {
	t.Helper()

	e := s.call("strigoi_get_entity", map[string]any{"handle": handle})
	xy := asList(e["screen"])

	if len(xy) != 2 {
		t.Fatalf("no screen position for %s: %v", handle, e)
	}

	return int(xy[0].(float64)), int(xy[1].(float64))
}

func distTo(t *testing.T, s *session, handle string) float64 {
	t.Helper()

	e := s.call("strigoi_get_entity", map[string]any{"handle": handle})
	p := s.call("strigoi_get_player", map[string]any{})

	return math.Hypot(num(e, "x")-num(p, "x"), num(e, "y")-num(p, "y"))
}

// walkNear walks him to within two tiles of a villager.
func walkNear(t *testing.T, s *session, handle string) {
	t.Helper()

	for tries := 0; tries < 6 && distTo(t, s, handle) > 2.5; tries++ {
		e := s.call("strigoi_get_entity", map[string]any{"handle": handle})
		s.call("strigoi_move_player_to", map[string]any{"x": num(e, "x") + 1, "y": num(e, "y") + 1})

		for i := 0; i < 60 && distTo(t, s, handle) > 2.5; i++ {
			s.call("strigoi_step", map[string]any{"frames": 6})
		}
	}

	if d := distTo(t, s, handle); d > TalkReach {
		t.Fatalf("could not walk within reach of %s: %.1f tiles", handle, d)
	}
}

// walkAway walks him at least min tiles from a villager, somewhere the
// villager is still on screen to be clicked.
func walkAway(t *testing.T, s *session, handle string, min float64) {
	t.Helper()

	e := s.call("strigoi_get_entity", map[string]any{"handle": handle})
	p := s.call("strigoi_get_player", map[string]any{})

	onScreen := func() bool {
		x, y := screenOf(t, s, handle)
		return x > 10 && x < 790 && y > 10 && y < 520
	}

	d := min + 1

	for _, off := range [][2]float64{{d, d}, {-d, -d}, {d, -d}, {-d, d}, {d, 0}, {-d, 0}, {0, d}, {0, -d}} {
		s.call("strigoi_move_player_to", map[string]any{"x": num(e, "x") + off[0], "y": num(e, "y") + off[1]})

		for i := 0; i < 60 && distTo(t, s, handle) < min; i++ {
			s.call("strigoi_step", map[string]any{"frames": 6})
		}

		s.call("strigoi_step", map[string]any{"frames": 30})

		if distTo(t, s, handle) >= min && onScreen() {
			return
		}
	}

	t.Fatalf("could not walk %.0f tiles from %s (from %.0f,%.0f)", min, handle, num(p, "x"), num(p, "y"))
}

// TalkReach mirrors d2gamescreen.TalkReachTiles [DIAL].
const TalkReach = 4.0

// openTalkWith clicks a villager and requires the talk to open.
func openTalkWith(t *testing.T, s *session, handle string) {
	t.Helper()

	x, y := screenOf(t, s, handle)
	s.call("strigoi_click", map[string]any{"x": x, "y": y, "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if !flag(t, uiState(s), "talk_open") {
		ui := uiState(s)
		t.Fatalf("a click on %s in reach opens a talk: %v (clicked %d,%d; %.1f tiles; hover %q; fighting %v; paced %v)",
			handle, villageState(s), x, y, distTo(t, s, handle), str(ui, "hover_label"),
			flag(t, combatState(s), "fighting"), combatState(s)["encounter"])
	}
}

// runToggled is whether the run button is down: the player's run_toggled.
func runToggled(t *testing.T, s *session) bool {
	t.Helper()

	return flag(t, sub(s.call("strigoi_get_player", map[string]any{}), "state"), "run_toggled")
}

// clickRunButton clicks the HUD's run button where it is drawn (ui.run_button).
func clickRunButton(t *testing.T, s *session) {
	t.Helper()

	b := sub(uiState(s), "run_button")
	if len(b) == 0 {
		t.Fatalf("no run button in the ui state: %v", keysOf(uiState(s)))
	}

	s.call("strigoi_click", map[string]any{
		"x": int(mustNum(t, b, "x") + mustNum(t, b, "w")/2), "y": int(mustNum(t, b, "y") + mustNum(t, b, "h")/2),
		"button": "left",
	})
	s.call("strigoi_step", map[string]any{"frames": 4})
}
