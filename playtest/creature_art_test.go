//go:build playtest

package playtest

import (
	"image/png"
	"os"
	"testing"
)

// TestFeralDogStillInGame is the first art acceptance test. It uses the same
// terminal verb a player can use, then proves the placed entity is the PNG
// creature and records the actual 800x600 game frame for visual review.
func TestFeralDogStillInGame(t *testing.T) {
	testCreatureStillInGame(t, "feral-dog", "Feral dog", 72, "feral-dog-still")
}

func TestWolfStillInGame(t *testing.T) {
	testCreatureStillInGame(t, "wolf", "Wolf", 96, "wolf-v1-night")
}

func testCreatureStillInGame(t *testing.T, creatureID, creatureName string, maxHealth float64, screenshotName string) {
	t.Helper()
	s := start(t)
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "ArtCheck", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	if msg := s.callErr("strigoi_run_console", map[string]any{"command": "spawnmon " + creatureID}); msg != "" {
		t.Fatalf("spawnmon %s: %s", creatureID, msg)
	}

	entities := s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 100})
	items, ok := entities["items"].([]any)
	if !ok {
		t.Fatalf("entities.items missing or wrong type: %v", entities)
	}

	var creature map[string]any
	for _, raw := range items {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		deep := s.call("strigoi_get_entity", map[string]any{"handle": str(row, "handle")})
		state := sub(deep, "state")
		if str(state, "creature") == creatureName {
			creature = deep
			break
		}
	}

	if creature == nil {
		t.Fatalf("spawnmon created no inspectable %s; entities=%v", creatureName, entities)
	}
	if got := str(sub(creature, "state"), "animation_mode"); got != "idle" {
		t.Fatalf("%s animation_mode = %q, want idle", creatureID, got)
	}

	shot := s.call("strigoi_screenshot", map[string]any{"name": screenshotName})
	shotPath := str(shot, "path")
	f, err := os.Open(shotPath)
	if err != nil {
		t.Fatalf("screenshot missing on disk: %v", err)
	}
	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("screenshot decode: %v", err)
	}
	if bounds := img.Bounds(); bounds.Dx() != 800 || bounds.Dy() != 600 {
		t.Fatalf("screenshot = %dx%d, want 800x600", bounds.Dx(), bounds.Dy())
	}

	player := s.call("strigoi_get_player", map[string]any{})
	s.call("strigoi_watch", map[string]any{
		"watcher": str(creature, "handle"), "target": str(player, "handle"),
	})
	fightNow(t, s)
	body := participant(t, combatState(s), str(creature, "id"))
	if got := mustNum(t, body, "max_health"); got != maxHealth {
		t.Fatalf("%s max_health = %.0f, want authored bestiary value %.0f: %v", creatureID, got, maxHealth, body)
	}

	t.Logf("%s %s at %.2f, %.2f screen=%v health=%.0f; screenshot %s",
		creatureID, str(creature, "handle"), num(creature, "x"), num(creature, "y"), creature["screen"], num(body, "max_health"), shotPath)
}
