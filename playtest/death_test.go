//go:build playtest

package playtest

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestDeath is death screen v0 (23 Sep 2026): a hero at 0 health no longer
// walks the map. Five acts:
//
//  1. He earns something this session -- a level -- and it is on disk.
//  2. THE CONTROL: alive, no death screen. Then his health goes to 0 and the
//     screen is up, naming the day.
//  3. No panel opens and no verb reaches the game under it.
//  4. His kit-and-progress file is back to the bytes it held when he entered:
//     the level from act 1 is gone. That is what "last save" means (the 12 Sep
//     ruling): his .od2 is only written when he leaves alive.
//  5. Enter loads the last save: a living hero, no screen, no level.
func TestDeath(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Mortal", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	sidecar := str(game, "save_path") + ".strigoi.json"
	atEntry, entryErr := os.ReadFile(sidecar)

	// --- 1: something earned this session ------------------------------------
	setField(s, "progress", "grant_xp", 50.0)

	if mustNum(t, progressState(s), "level") != 2 {
		t.Fatalf("act 1: 50 experience is level 2: %v", progressState(s))
	}

	for i := 0; i < 30; i++ {
		if data, err := os.ReadFile(sidecar); err == nil && strings.Contains(string(data), `"xp": 50`) {
			break
		}

		s.call("strigoi_step", map[string]any{"frames": 2})
	}

	if data, _ := os.ReadFile(sidecar); !strings.Contains(string(data), `"xp": 50`) {
		t.Fatalf("act 1: a level is saved beside him as it happens; the file has %s", data)
	}

	// --- 2: the screen --------------------------------------------------------
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, uiState(s), "death_open") {
		t.Fatal("act 2 control: a living hero has no death screen")
	}

	setField(s, "meters", "health", 0.0)
	s.call("strigoi_step", map[string]any{"frames": 3})

	ui := uiState(s)
	if !flag(t, ui, "death_open") {
		t.Fatalf("act 2: at 0 health the death screen is up: %v", ui)
	}

	lines, _ := ui["death_lines"].([]any)
	if len(lines) == 0 || !strings.Contains(lines[0].(string), "17 June 1462") {
		t.Fatalf("act 2: the screen names the day he died; it reads %v", lines)
	}

	// --- 3: nothing reaches the game under it ----------------------------
	// (The world is NOT held -- D2's own death screen lets it run on; see
	// worldRunning.)
	s.call("strigoi_key", map[string]any{"key": "t"})
	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if ui := uiState(s); flag(t, ui, "talent_open") || flag(t, ui, "kit_open") {
		t.Fatalf("act 3: no panel opens under the death screen: %v", ui)
	}

	// And no verb reaches the game: L would light his torch (he carries one --
	// torch-and-blade is the harness default), and a dead man lights nothing.
	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, lightState(s), "carried_lit") {
		t.Fatal("act 3: L lit a dead man's torch -- a key reached the game under the death screen")
	}

	// --- 4: the file is back ---------------------------------------------------
	now, nowErr := os.ReadFile(sidecar)

	switch {
	case entryErr != nil && nowErr == nil:
		t.Fatalf("act 4: he entered with no kit file, so death removes the one this session wrote; it holds %s", now)
	case entryErr == nil && !bytes.Equal(now, atEntry):
		t.Fatalf("act 4: his file is back to how he entered.\n entered: %s\n now:     %s", atEntry, now)
	}

	// --- 5: Enter -------------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "enter"})

	alive, last := false, ""

	for i := 0; i < 120 && !alive; i++ {
		s.call("strigoi_step", map[string]any{"frames": 6})

		if last = s.callErr("strigoi_get_player", map[string]any{}); last != "" {
			continue
		}

		p := s.call("strigoi_get_player", map[string]any{})
		alive = num(sub(p, "state"), "health") > 0
		last = fmt.Sprint(p)
	}

	if !alive {
		t.Fatalf("act 5: Enter loads the last save, and he stands up in it; the last read was %s", last)
	}

	if flag(t, uiState(s), "death_open") {
		t.Fatal("act 5: the reloaded hero has no death screen")
	}

	if got := mustNum(t, progressState(s), "xp"); got != 0 {
		t.Fatalf("act 5: the level earned after his last save is gone; he has %.0f experience", got)
	}

	t.Logf("died with a level he had not saved; reloaded alive, level 1, file as he entered")
}
