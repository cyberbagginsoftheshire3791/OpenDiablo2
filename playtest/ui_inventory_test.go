//go:build playtest

package playtest

import (
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

// TestUIInventory is the M3.4 UI script (P3 spec §6.1): scripted input at the
// engine's input seam, asserted through the "ui" provider. It exercises every
// M3.4 tool once — providers (list/get/set), key/click/move_cursor/type_text,
// spawn/remove — under the paused clock, so the whole thing is stepped, never
// wall-clock timed.
func TestUIInventory(t *testing.T) {
	s := start(t)

	// The world is born frozen (the M3.3 recipe); frames still render and the
	// input manager still polls every frame, so scripted input works paused.
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name":    "UI",
		"hero_class":   "amazon",
		"seed":         1462,
		"wait_seconds": 90,
	})

	spawnX, spawnY := pair(game, "spawn_tile")
	t.Logf("spawned at %.2f, %.2f", spawnX, spawnY)

	// 1. providers: the controls register as "ui" once the player exists
	systems := s.call("strigoi_list_systems", map[string]any{})
	if !hasSystem(systems, "ui") {
		t.Fatalf("list_systems: want the ui provider, got %v", systems["systems"])
	}

	ui := sub(s.call("strigoi_get_system_state", map[string]any{"system": "ui"}), "state")
	if ui["inventory_open"] != false || ui["escape_menu_open"] != false {
		t.Fatalf("fresh game: want all panels closed, got %v", ui)
	}

	// A planned system answers with its milestone, not a bare unknown. This
	// used to ask about "meters" and passed until M4.2 shipped them, which
	// is the assertion doing its job: it pins the ABSENCE of a system, so
	// it must move to one still absent as each milestone lands. Next after
	// spawns: dead (M4.3 / M4.6), then combat (M4.5).
	if msg := s.callErr("strigoi_get_system_state", map[string]any{"system": "spawns"}); !strings.Contains(msg, "NOT_IMPLEMENTED") || !strings.Contains(msg, "M4.3") {
		t.Fatalf("spawns: want NOT_IMPLEMENTED naming M4.3, got %q", msg)
	}

	// ...and a system that has landed answers with its state.
	if !hasSystem(systems, "meters") {
		t.Fatalf("meters registered at M4.2; list_systems does not show it: %v", systems["systems"])
	}

	if msg := s.callErr("strigoi_set_system_field", map[string]any{"system": "ui", "field": "inventory_open", "value": true}); !strings.Contains(msg, "FIELD_NOT_SETTABLE") {
		t.Fatalf("ui is read-only: want FIELD_NOT_SETTABLE, got %q", msg)
	}

	// let the camera settle on the player (it converges per frame, not per sim second)
	s.call("strigoi_step", map[string]any{"frames": 30})

	// 2. tap i: the inventory opens (the default key map binds B and I)
	key := s.call("strigoi_key", map[string]any{"key": "i", "action": "tap"})
	if num(key, "tick_applied") <= 0 {
		t.Fatalf("key: no tick_applied in %v", key)
	}

	ui = sub(s.call("strigoi_get_system_state", map[string]any{"system": "ui"}), "state")
	if ui["inventory_open"] != true || ui["right_panel_open"] != true {
		t.Fatalf("after tapping i: want inventory_open, got %v", ui)
	}

	// 3. screenshot: the inventory panel (right half, above the HUD) is drawn
	shot := s.call("strigoi_screenshot", map[string]any{"name": "inventory"})
	lit, total := litFraction(t, str(shot, "path"), 420, 40, 780, 520)
	t.Logf("inventory panel region: %d/%d sampled pixels lit (%.0f%%)", lit, total, 100*float64(lit)/float64(total))

	if float64(lit) < 0.25*float64(total) {
		t.Fatalf("the inventory panel region is mostly black (%d/%d lit) — the panel did not render", lit, total)
	}

	// 4. tap i again: it closes
	s.call("strigoi_key", map[string]any{"key": "i"})

	ui = sub(s.call("strigoi_get_system_state", map[string]any{"system": "ui"}), "state")
	if ui["inventory_open"] != false {
		t.Fatalf("after the second tap: want inventory closed, got %v", ui)
	}

	// 5. escape opens the escape menu; escape again closes it
	s.call("strigoi_key", map[string]any{"key": "escape"})

	ui = sub(s.call("strigoi_get_system_state", map[string]any{"system": "ui"}), "state")
	if ui["escape_menu_open"] != true {
		t.Fatalf("after escape: want escape_menu_open, got %v", ui)
	}

	s.call("strigoi_key", map[string]any{"key": "escape"})

	ui = sub(s.call("strigoi_get_system_state", map[string]any{"system": "ui"}), "state")
	if ui["escape_menu_open"] != false {
		t.Fatalf("after the second escape: want the menu closed, got %v", ui)
	}

	// 6. a scripted click on open ground walks the player through the normal
	//    controls. One world tile east is (+80, +40) screen pixels from the
	//    player (the isometric projection); east is clear at seed 1462.
	before := s.call("strigoi_get_player", map[string]any{})
	sx, sy := pair(before, "screen")
	fromX, fromY := num(before, "x"), num(before, "y")

	if sx == 0 && sy == 0 {
		t.Fatal("get_player: no screen position")
	}

	cursor := s.call("strigoi_move_cursor", map[string]any{"x": int(sx) + 120, "y": int(sy) + 60})
	if cursor["cursor_scripted"] != true {
		t.Fatalf("move_cursor: want a scripted cursor, got %v", cursor)
	}

	s.call("strigoi_click", map[string]any{"x": int(sx) + 120, "y": int(sy) + 60, "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 180})

	after := s.call("strigoi_get_player", map[string]any{})
	dx, dy := num(after, "x")-fromX, num(after, "y")-fromY
	t.Logf("click walk: moved %.2f, %.2f tiles (want mostly +x)", dx, dy)

	if math.Hypot(dx, dy) < 0.5 || dx < math.Abs(dy) {
		t.Fatalf("the click did not walk the player east: moved %.2f, %.2f", dx, dy)
	}

	// 7. type_text reaches the input chars poll without disturbing the game
	typed := s.call("strigoi_type_text", map[string]any{"text": "ok"})
	if num(typed, "tick_applied") <= 0 {
		t.Fatalf("type_text: %v", typed)
	}

	// 8. spawn an npc two tiles north of the player, read it, remove it
	p := s.call("strigoi_get_player", map[string]any{})
	count := int(num(s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200}), "total"))

	spawned := s.call("strigoi_spawn_entity", map[string]any{
		"kind": "npc", "code": "fallen1", "x": num(p, "x"), "y": num(p, "y") - 2,
	})

	handle := str(spawned, "handle")
	if !strings.HasPrefix(handle, "e:") {
		t.Fatalf("spawn_entity: want an e:N handle, got %v", spawned)
	}

	if got := int(num(s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200}), "total")); got != count+1 {
		t.Fatalf("npc count after spawn: want %d, got %d", count+1, got)
	}

	npc := s.call("strigoi_get_entity", map[string]any{"handle": handle})
	if st := sub(npc, "state"); st["monstat"] != "fallen1" {
		t.Fatalf("spawned npc state: want monstat fallen1, got %v", st)
	}

	// spawned entities live in the digest: the world changed
	s.call("strigoi_step", map[string]any{"frames": 60})

	removed := s.call("strigoi_remove_entity", map[string]any{"handle": handle})
	if removed["removed"] != true {
		t.Fatalf("remove_entity: %v", removed)
	}

	if msg := s.callErr("strigoi_get_entity", map[string]any{"handle": handle}); !strings.Contains(msg, "UNKNOWN_HANDLE") {
		t.Fatalf("after removal: want UNKNOWN_HANDLE, got %q", msg)
	}

	if msg := s.callErr("strigoi_remove_entity", map[string]any{"handle": "p:1"}); !strings.Contains(msg, "BAD_ARGUMENT") {
		t.Fatalf("removing the player must be refused, got %q", msg)
	}

	// 9. no unexpected error lines (the known "invalid frame index" is allowed)
	logs := s.call("strigoi_read_log", map[string]any{"pattern": `\[(ERROR|FATAL)\]`, "limit": 50})

	if lines, ok := logs["lines"].([]any); ok {
		for _, l := range lines {
			m, _ := l.(map[string]any)
			if !strings.Contains(str(m, "text"), "invalid frame index") {
				t.Fatalf("unexpected error log line: %v", m)
			}
		}
	}
}

func hasSystem(out map[string]any, name string) bool {
	systems, _ := out["systems"].([]any)

	for _, sys := range systems {
		m, _ := sys.(map[string]any)
		if str(m, "name") == name {
			return true
		}
	}

	return false
}

// litFraction samples every 4th pixel of a screenshot region and counts the
// ones that are not near-black.
func litFraction(t *testing.T, path string, x0, y0, x1, y1 int) (lit, total int) {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("screenshot missing on disk: %v", err)
	}

	img, err := png.Decode(f)
	_ = f.Close()

	if err != nil {
		t.Fatalf("screenshot decode: %v", err)
	}

	for y := y0; y < y1; y += 4 {
		for x := x0; x < x1; x += 4 {
			r, g, b, _ := img.At(x, y).RGBA()
			total++

			if r>>8+g>>8+b>>8 > 30 {
				lit++
			}
		}
	}

	return lit, total
}
