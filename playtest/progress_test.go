//go:build playtest

package playtest

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestProgress is T3's playtest (23 Sep 2026): experience, a level, and a
// talent that changes his body, in a real launch.
//
//  1. a kill EARNS experience, and exactly what data/strigoi/talents.json says
//     it is worth -- a number the test knows, against one the game reports;
//  2. a level opens a pick; T opens the talent panel in place of D2's tree;
//  3. a click SELECTS and only a second click TAKES (there is no respec); a
//     talent whose rank above is untaken is refused;
//  4. Hard Flesh raises his maximum health by exactly its +30 -- the control
//     is the same hero one click earlier;
//  5. his experience and talents are on disk beside his save when he leaves.
//
// Reaching a level by fights alone would take minutes of them, so after the
// kill proves the earning path the script uses grant_xp -- the one arranging
// verb, named for what it is.
func TestProgress(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Progress", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	prog := progressState(s)
	if !flag(t, prog, "bound") || mustNum(t, prog, "level") != 1 || mustNum(t, prog, "xp") != 0 {
		t.Fatalf("a fresh hero is level 1 with no experience: %v", prog)
	}

	// --- 1: a kill earns what the table says ---------------------------------
	p := s.call("strigoi_get_player", map[string]any{})
	enemy := spawnNPC(t, s, "fallen1", num(p, "x")+1, num(p, "y"))
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": str(p, "handle")})

	setField(s, "combat", "forced_band", "crit")
	fightNow(t, s)

	for i := 0; i < 60 && flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 12})
	}

	setField(s, "combat", "forced_band", "")

	// A harness-spawned fallen1 is on no spawn row, so it is worth the table's
	// fallback: slain[""] = 5.
	if got := mustNum(t, progressState(s), "xp"); got != 5 {
		t.Fatalf("act 1: one kill of an unplaced beast is worth 5 experience; he has %.0f", got)
	}

	// --- 2: a level, and the panel -----------------------------------------
	setField(s, "progress", "grant_xp", 45.0)

	prog = progressState(s)
	if mustNum(t, prog, "level") != 2 || mustNum(t, prog, "picks_waiting") != 1 {
		t.Fatalf("act 2: 50 experience is level 2 with one pick: %v", prog)
	}

	s.call("strigoi_key", map[string]any{"key": "t"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	ui := uiState(s)
	if !flag(t, ui, "talent_open") {
		t.Fatalf("act 2: T opens the talent panel: %v", ui)
	}

	if cells, _ := ui["talent_cells"].([]any); len(cells) != 15 {
		t.Fatalf("act 2: three branches of five; the panel shows %d", len(cells))
	}

	// --- 3: select, then take; the locked refuse ----------------------------
	clickCell(t, s, "hard-flesh")
	clickCell(t, s, "hard-flesh")

	if note := str(uiState(s), "talent_note"); !strings.Contains(note, "above") {
		t.Fatalf("act 3: rank 2 before rank 1 must be refused; the panel says %q", note)
	}

	maxBefore := mustNum(t, metersState(s), "max_health")

	clickCell(t, s, "long-marches")

	if talents(t, s)["long-marches"] {
		t.Fatal("act 3: ONE click selects and must not take -- a talent is for good")
	}

	clickCell(t, s, "long-marches")

	if !talents(t, s)["long-marches"] || mustNum(t, progressState(s), "picks_waiting") != 0 {
		t.Fatalf("act 3: the second click takes it and spends the pick: %v", progressState(s))
	}

	// --- 4: Hard Flesh is felt ------------------------------------------------
	if got := mustNum(t, metersState(s), "max_health"); got != maxBefore {
		t.Fatalf("act 4 control: Long Marches changes no health; max %.0f -> %.0f", maxBefore, got)
	}

	setField(s, "progress", "grant_xp", 70.0) // level 3

	clickCell(t, s, "hard-flesh")
	clickCell(t, s, "hard-flesh")

	if got := mustNum(t, metersState(s), "max_health"); got != maxBefore+30 {
		t.Fatalf("act 4: Hard Flesh is +30 maximum health: %.0f -> %.0f", maxBefore, got)
	}

	// --- 5: on disk ------------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "t"})
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})

	sidecar := str(game, "save_path") + ".strigoi.json"

	var saved struct {
		Progress struct {
			XP      int      `json:"xp"`
			Talents []string `json:"talents"`
		} `json:"progress"`
	}

	for i := 0; i < 50; i++ {
		if data, err := os.ReadFile(sidecar); err == nil && json.Unmarshal(data, &saved) == nil && saved.Progress.XP == 120 {
			break
		}

		s.call("strigoi_step", map[string]any{"frames": 6})
	}

	if saved.Progress.XP != 120 || len(saved.Progress.Talents) != 2 {
		t.Fatalf("act 5: 120 experience and two talents on disk; the file has %+v", saved.Progress)
	}

	t.Logf("act 5: level 3, %v, max health %.0f -> %.0f, on disk", saved.Progress.Talents, maxBefore, maxBefore+30)
}

func progressState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "progress"}), "state")
}

func talents(t *testing.T, s *session) map[string]bool {
	t.Helper()

	out := map[string]bool{}

	if list, ok := progressState(s)["talents"].([]any); ok {
		for _, raw := range list {
			if id, ok := raw.(string); ok {
				out[id] = true
			}
		}
	}

	return out
}

func clickCell(t *testing.T, s *session, id string) {
	t.Helper()

	cells, _ := uiState(s)["talent_cells"].([]any)
	for _, raw := range cells {
		c, _ := raw.(map[string]any)
		if str(c, "id") == id {
			s.call("strigoi_click", map[string]any{"button": "left", "x": int(num(c, "x")), "y": int(num(c, "y"))})
			s.call("strigoi_step", map[string]any{"frames": 2})

			return
		}
	}

	t.Fatalf("no talent cell %q on the panel", id)
}
