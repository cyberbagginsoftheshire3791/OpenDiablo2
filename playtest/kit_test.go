//go:build playtest

package playtest

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestKit is T2's playtest (23 Sep 2026): the hero's gear in a real launch.
//
//  1. a new hero opens on the LOADOUT CHOICE and the world is held until he
//     makes it; 1 chooses sword-and-board, and the kit is written beside his
//     save;
//  2. I opens the Strigoi kit panel (not the D2 grid); clicking his hand row
//     takes the sabre off and clicking it in the pack puts it back;
//  3. L is refused -- sword-and-board carries no torch, which is the dilemma;
//  4. in a fight, the SHIELD turns a crit into a hit once a round and the MAIL
//     takes its share and wears -- and the wear is on disk when he leaves.
//
// Its control is the torch-and-blade run at the end: L lights, and no blow is
// ever blocked.
func TestKit(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Kit", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
		"loadout": "ask",
	})
	setField(s, "spawns", "chance", 0)

	sidecar := str(game, "save_path") + ".strigoi.json"
	if str(game, "save_path") == "" {
		t.Fatalf("start_game must report the save it created: %v", game)
	}

	// --- 1: the choice holds the world -----------------------------------------
	if !flag(t, uiState(s), "choosing_loadout") {
		t.Fatalf("act 1: a new hero with no kit must open on the loadout choice: %v", uiState(s))
	}

	if _, err := os.Stat(sidecar); err == nil {
		t.Fatal("act 1: no kit may be written before he chooses")
	}

	w0 := worldMinutes(t, s)
	s.call("strigoi_step", map[string]any{"frames": 60})

	if w1 := worldMinutes(t, s); w1 != w0 {
		t.Fatalf("act 1: the world moved while he was choosing (%.4f -> %.4f)", w0, w1)
	}

	// The hold is named, and step_world refuses it (history item 121).
	if held := mustStr(t, uiState(s), "world_held_by"); held != "loadout" {
		t.Fatalf("act 1: world_held_by %q while he chooses, want \"loadout\"", held)
	}

	if msg := s.callErr("strigoi_step_world", map[string]any{"world_minutes": 10.0}); !strings.Contains(msg, "WORLD_HELD") || !strings.Contains(msg, `"loadout"`) {
		t.Fatalf("act 1: step_world during the loadout choice: got %q, want WORLD_HELD naming \"loadout\"", msg)
	}

	s.call("strigoi_key", map[string]any{"key": "1"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, uiState(s), "choosing_loadout") {
		t.Fatal("act 1: 1 must make the choice")
	}

	kit := readKit(t, sidecar)
	if kit.Loadout != "sword-and-board" {
		t.Fatalf("act 1: 1 is sword-and-board; the kit file says %q", kit.Loadout)
	}

	s.call("strigoi_step", map[string]any{"frames": 60})

	if w2 := worldMinutes(t, s); w2 == w0 {
		t.Fatal("act 1: the world must run once he has chosen")
	}

	// --- 2: the kit panel ----------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	ui := uiState(s)
	if !flag(t, ui, "kit_open") || flag(t, ui, "inventory_open") {
		t.Fatalf("act 2: I opens the Strigoi kit panel and not the D2 grid: %v", ui)
	}

	hand := kitRow(t, ui, func(r map[string]any) bool { return str(r, "slot") == "main" })
	if !strings.Contains(str(hand, "text"), "Kilic") {
		t.Fatalf("act 2: his hand holds the kılıç: %v", hand)
	}

	s.call("strigoi_click", map[string]any{"button": "left", "x": int(num(hand, "x")), "y": int(num(hand, "y"))})
	s.call("strigoi_step", map[string]any{"frames": 2})

	ui = uiState(s)
	hand = kitRow(t, ui, func(r map[string]any) bool { return str(r, "slot") == "main" })

	if strings.Contains(str(hand, "text"), "Kilic") {
		t.Fatalf("act 2: clicking his hand must take the sabre off: %v (notice %q)", hand, str(ui, "kit_notice"))
	}

	sabre := kitRow(t, ui, func(r map[string]any) bool {
		return num(r, "pack") >= 0 && strings.Contains(str(r, "text"), "Kilic")
	})

	s.call("strigoi_click", map[string]any{"button": "left", "x": int(num(sabre, "x")), "y": int(num(sabre, "y"))})
	s.call("strigoi_step", map[string]any{"frames": 2})

	hand = kitRow(t, uiState(s), func(r map[string]any) bool { return str(r, "slot") == "main" })
	if !strings.Contains(str(hand, "text"), "Kilic") {
		t.Fatalf("act 2: clicking the sabre in the pack must put it back in his hand: %v", hand)
	}

	shot := s.call("strigoi_screenshot", map[string]any{"name": "t2-kit-panel"})
	t.Logf("act 2: the kit panel, %s", str(shot, "path"))

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- 3: no torch ---------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, lightState(s), "carried_lit") {
		t.Fatal("act 3: sword-and-board has no torch to light")
	}

	if notice := str(uiState(s), "tactical_notice"); !strings.Contains(notice, "torch") {
		t.Fatalf("act 3: L must say why it did nothing; the panel says %q", notice)
	}

	// --- 4: shield and mail in a fight ----------------------------------------
	mailBefore := kit.Worn["body"].Points

	blocked, absorbed := fightForBlows(t, s)
	if blocked == 0 {
		t.Fatal("act 4: over a forced-crit fight the shield must turn at least one blow")
	}

	if absorbed == 0 {
		t.Fatal("act 4: the mail must take something off a blow")
	}

	// The wear is written when the fight closes, and again when he leaves.
	// navigate returns before the screen has unloaded, so the file is polled
	// rather than read once (measured: a single read beat the write).
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})

	after := readKit(t, sidecar)
	for i := 0; i < 50 && after.Worn["body"].Points >= mailBefore; i++ {
		s.call("strigoi_step", map[string]any{"frames": 6})
		after = readKit(t, sidecar)
	}
	if after.Worn["body"].Points >= mailBefore {
		t.Fatalf("act 4: the mail's wear must be saved: %d -> %d points", mailBefore, after.Worn["body"].Points)
	}

	t.Logf("act 4: %d blow(s) blocked, %d absorbed in total; mail %d -> %d points on disk",
		blocked, absorbed, mailBefore, after.Worn["body"].Points)
}

// TestKitTorchAndBladeControl is the other loadout: the torch lights and the
// shield never turns anything, because there is none.
func TestKitTorchAndBladeControl(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Torch", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
		"loadout": "torch-and-blade",
	})
	setField(s, "spawns", "chance", 0)

	if flag(t, uiState(s), "choosing_loadout") {
		t.Fatal("a hero whose kit was written does not choose again")
	}

	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if !flag(t, lightState(s), "carried_lit") {
		t.Fatal("torch-and-blade: L lights the torch in his off-hand")
	}

	if blocked, _ := fightForBlows(t, s); blocked != 0 {
		t.Fatalf("torch-and-blade has no shield, and %d blow(s) were blocked", blocked)
	}
}

// fightForBlows arranges a fight under the policy with every band forced to a
// crit, lets it run a few world minutes, and counts blocked blows and armour
// absorption across every round it can see.
func fightForBlows(t *testing.T, s *session) (blocked, absorbed int) {
	t.Helper()

	p := s.call("strigoi_get_player", map[string]any{})
	enemy := spawnNPC(t, s, "zombie1", num(p, "x")+1, num(p, "y"))
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": str(p, "handle")})

	setField(s, "combat", "forced_band", "crit")
	setField(s, "combat", "player_action", "hold") // he never kills it

	fightNow(t, s)

	seen := map[float64]bool{}

	for i := 0; i < 40 && flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 12})

		c := combatState(s)
		round := mustNum(t, c, "actions_round")

		if seen[round] {
			continue
		}

		seen[round] = true

		for _, row := range actionRows(t, c) {
			if str(row, "target") != str(p, "id") {
				continue
			}

			if flag(t, row, "blocked") {
				blocked++
			}

			absorbed += int(mustNum(t, row, "absorbed"))
		}
	}

	setField(s, "combat", "forced_band", "")

	return blocked, absorbed
}

type kitFile struct {
	Loadout string `json:"loadout"`
	Worn    map[string]struct {
		Item   string `json:"item"`
		Points int    `json:"points"`
	} `json:"worn"`
}

func readKit(t *testing.T, path string) kitFile {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the kit file %s: %v", path, err)
	}

	var sc struct {
		Kit kitFile `json:"kit"`
	}

	if err := json.Unmarshal(data, &sc); err != nil {
		t.Fatalf("the kit file %s: %v", path, err)
	}

	return sc.Kit
}

func kitRow(t *testing.T, ui map[string]any, match func(map[string]any) bool) map[string]any {
	t.Helper()

	rows, _ := ui["kit_rows"].([]any)
	for _, raw := range rows {
		if r, ok := raw.(map[string]any); ok && match(r) {
			return r
		}
	}

	t.Fatalf("no kit row matches in %v", rows)

	return nil
}
