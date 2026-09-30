//go:build playtest

package playtest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestJanissaryArtLocomotion exercises the shipped hero selection, without a
// -hero flag or a harness hero_art override. Each bearing is reached by actual
// movement. The captures are native game frames; their sidecars retain the
// ground point and source sheet hashes for visual review. They do not pretend
// a correct direction number alone proves the painted figure faces that way.
func TestJanissaryArtLocomotion(t *testing.T) {
	s := janissaryStart(t, "torch-and-blade")
	janissaryCapture(t, s, "idle", "idle")
	x, y := janissaryClearGround(t, s)
	janissaryArrive(t, s, x, y)

	for _, run := range []bool{false, true} {
		if runToggled(t, s) != run {
			// The shipped R binding reaches the same toggle without the
			// known intermittent mouse-button event loss (BUG-15).
			s.call("strigoi_key", map[string]any{"key": "r"})
			s.call("strigoi_step", map[string]any{"frames": 4})
		}
		if runToggled(t, s) != run {
			t.Fatalf("R did not select running=%v", run)
		}
		mode := "walk"
		if run {
			mode = "run"
		}
		for _, bearing := range []struct {
			name   string
			dx, dy float64
			facing int
		}{
			{"SW", 0, 1, 8}, {"NW", -1, 0, 24},
			{"NE", 0, -1, 40}, {"SE", 1, 0, 56},
			{"S", 1, 1, 0}, {"W", -1, 1, 16},
			{"N", -1, -1, 32}, {"E", 1, -1, 48},
		} {
			toX, toY := x+2*bearing.dx, y+2*bearing.dy
			path := s.call("strigoi_find_path", map[string]any{"to_x": toX, "to_y": toY})
			if !flag(t, path, "straight_line_clear") || !flag(t, path, "reachable") {
				t.Fatalf("%s %s has no clear, reachable test route: %v", mode, bearing.name, path)
			}
			s.call("strigoi_move_player_to", map[string]any{"x": toX, "y": toY})
			s.call("strigoi_step", map[string]any{"frames": 8})
			p := s.call("strigoi_get_player", map[string]any{})
			state := sub(p, "state")
			got := int(mustNum(t, state, "direction"))
			// Sub-tile arrival rounding can quantize a cardinal route one of
			// 64 facing steps either side of its center, in the same art row.
			delta := math.Abs(float64(got - bearing.facing))
			delta = math.Min(delta, 64-delta)
			if got < 0 || got >= 64 || delta > 1 {
				t.Fatalf("%s %s faces %d, want %d +/- 1 circular step: %v", mode, bearing.name, got, bearing.facing, state)
			}
			if math.Hypot(mustNum(t, p, "x")-x, mustNum(t, p, "y")-y) < .05 {
				t.Fatalf("%s %s changed animation without moving", mode, bearing.name)
			}
			janissaryCapture(t, s, mode+"-"+bearing.name, mode)
			janissaryArrive(t, s, toX, toY)
			janissaryArrive(t, s, x, y)
		}
	}
	janissaryCapture(t, s, "idle-after-run", "idle")
}

// TestJanissaryArtCombat observes the actual player combat animation seam:
// F commits a strike; a shield blocks a real enemy blow; a second enemy's
// blow hurts him; finally an enemy kills him. No animation setter is used.
// This acceptance is deliberately red while the player ignores ActHit/ActDie
// or loses A1 on the following tick. A loader-only test cannot catch that.
func TestJanissaryArtCombat(t *testing.T) {
	s := janissaryStart(t, "sword-and-board")
	setField(s, "combat", "player_control", "human")
	setField(s, "combat", "forced_band", "crit")
	p := s.call("strigoi_get_player", map[string]any{})
	playerID := mustStr(t, p, "id")
	spot := clearNeighbour(t, s, mustNum(t, p, "x"), mustNum(t, p, "y"))
	enemy := spawnNPC(t, s, "zombie1", spot[0], spot[1])
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": mustStr(t, p, "handle")})
	second := spawnNPC(t, s, "zombie1", spot[0], spot[1])
	s.call("strigoi_watch", map[string]any{"watcher": second, "target": mustStr(t, p, "handle")})
	openTurn(t, s)
	s.call("strigoi_key", map[string]any{"key": "f"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	if !flag(t, combatState(s), "action_spent") {
		t.Fatal("F did not commit a real strike")
	}
	janissaryCapture(t, s, "attack", "attack")
	// A second sample catches the former one-frame A1 -> SC overwrite.
	s.call("strigoi_step", map[string]any{"frames": 6})
	janissaryCapture(t, s, "attack-held", "attack")
	if got := str(sub(s.call("strigoi_get_player", map[string]any{}), "state"), "animation_mode"); got != "A1" {
		t.Errorf("the strike's requested A1 was replaced by %q; attack-sheet fallback alone cannot detect this", got)
	}
	s.call("strigoi_key", map[string]any{"key": "e"})
	janissaryAwaitBlow(t, s, playerID, true)
	janissaryCapture(t, s, "block", "block")

	// The shield only blocks once per round. The second paced enemy blow
	// supplies the unblocked control without changing gear during combat.
	janissaryAwaitBlow(t, s, playerID, false)
	janissaryCapture(t, s, "hit", "hit")

	openTurn(t, s)
	// Health is setup; the fatal event must still be a resolver blow, not a
	// direct animation request or a direct write of zero health.
	setField(s, "meters", "health", 1.0)
	s.call("strigoi_key", map[string]any{"key": "e"})
	for frame := 0; frame < 480; frame++ {
		s.call("strigoi_step", map[string]any{"frames": 1})
		if flag(t, metersState(s), "dead") {
			if mustStr(t, combatState(s), "ended_reason") != "player_dead" {
				t.Fatal("death was not the arranged combat blow")
			}
			janissaryCapture(t, s, "death", "death")
			s.call("strigoi_step", map[string]any{"frames": 180})
			janissaryCapture(t, s, "dead", "dead")
			return
		}
	}
	t.Fatal("the enemy did not deliver the fatal blow in 480 frames")
}

func janissaryStart(t *testing.T, loadout string) *session {
	t.Helper()
	if os.Getenv("STRIGOI_HARNESS_ADDR") != "" {
		t.Skip("art acceptance launches its own default game and measures its private asset snapshot")
	}
	s := startWith(t) // deliberately the shipped default, even in a classic sweep
	s.call("strigoi_pause", map[string]any{})
	g := s.call("strigoi_start_game", map[string]any{
		"hero_name": "JanissaryArt", "hero_class": "amazon", "seed": 1462,
		"wait_seconds": 90, "loadout": loadout,
	})
	if str(g, "hero_used") != "/data/strigoi/hero/janissary/hero.json" || str(g, "hero_error") != "" {
		t.Fatalf("default hero is not the Janissary: %v", g)
	}
	if str(g, "map_built") != "/data/strigoi/maps/village.tmj" || str(g, "map_error") != "" {
		t.Fatalf("the default authored village failed to load: %v", g)
	}
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	// Advance the real clock to daylight before judging painted details.
	// Restore setup meters afterward so time spent waiting for noon does
	// not change the movement/combat conditions being exercised below.
	before := metersState(s)
	minutes := math.Mod(720-mustNum(t, clockState(s), "minute_of_day")+1440, 1440)
	if minutes > 0 {
		s.call("strigoi_step_world", map[string]any{"world_minutes": minutes})
	}
	for _, field := range []string{"health", "food", "water", "fatigue"} {
		setField(s, "meters", field, mustNum(t, before, field))
	}
	return s
}

func janissaryClearGround(t *testing.T, s *session) (float64, float64) {
	t.Helper()
	m := parseShippedVillage(t)
	for y := 3; y < m.Height-3; y++ {
		for x := 3; x < m.Width-3; x++ {
			clear := true
			for yy := y - 2; yy <= y+2; yy++ {
				for xx := x - 2; xx <= x+2; xx++ {
					clear = clear && !m.Blocked(xx, yy)
				}
			}
			if clear {
				px, py := float64(x)+.5, float64(y)+.5
				if flag(t, s.call("strigoi_find_path", map[string]any{"to_x": px, "to_y": py}), "reachable") {
					return px, py
				}
			}
		}
	}
	t.Fatal("no reachable 5x5 clear area in the shipped village for directional review")
	return 0, 0
}

func janissaryArrive(t *testing.T, s *session, x, y float64) {
	t.Helper()
	s.call("strigoi_move_player_to", map[string]any{"x": x, "y": y})
	for frame := 0; frame < 1800; frame += 12 {
		s.call("strigoi_step", map[string]any{"frames": 12})
		p := s.call("strigoi_get_player", map[string]any{})
		if math.Hypot(mustNum(t, p, "x")-x, mustNum(t, p, "y")-y) < .04 && str(sub(p, "state"), "body_sheet") == "idle" {
			return
		}
	}
	t.Fatalf("did not arrive and stand idle at %.2f,%.2f", x, y)
}

func janissaryAwaitBlow(t *testing.T, s *session, playerID string, blocked bool) {
	t.Helper()
	before := mustNum(t, combatState(s), "actions_total")
	for frame := 0; frame < 480; frame++ {
		s.call("strigoi_step", map[string]any{"frames": 1})
		c := combatState(s)
		if mustNum(t, c, "actions_total") == before {
			continue
		}
		before = mustNum(t, c, "actions_total")
		for _, row := range actionRows(t, c) {
			if str(row, "target") == playerID && flag(t, row, "blocked") == blocked && mustNum(t, row, "damage") > 0 {
				return
			}
		}
	}
	t.Fatalf("no real enemy blow with blocked=%v in 480 frames", blocked)
}

func janissaryCapture(t *testing.T, s *session, name, want string) {
	t.Helper()
	p := s.call("strigoi_get_player", map[string]any{})
	state := sub(p, "state")
	point, ok := p["screen"].([]any)
	if !ok || len(point) != 2 {
		t.Fatalf("%s: missing measured player ground screen point: %v", name, p)
	}
	sx, sy := pair(p, "screen")
	if sx <= 0 || sx >= 800 || sy <= 0 || sy >= 600 {
		t.Fatalf("%s: ground point %.1f,%.1f is outside the native frame", name, sx, sy)
	}
	shot := s.call("strigoi_screenshot", map[string]any{"name": "janissary-" + name})
	path := mustStr(t, shot, "path")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Fatalf("native capture is %v, want 800x600", b)
	}
	files := map[string]any{}
	actual := str(state, "body_sheet")
	for _, file := range []string{"hero.json", actual + ".png", actual + ".png.json"} {
		// startWith's executable carries an immutable asset mirror. Hash that
		// snapshot, not the checkout another art task may replace mid-run.
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(s.cmd.Path), "data", "strigoi", "hero", "janissary", file))
		if err != nil {
			t.Fatal(err)
		}
		files[file] = fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	evidence, err := json.MarshalIndent(map[string]any{"capture": path, "player": p, "art_sha256": files, "expected_sheet": want,
		"note": "screen is the world-projected actor position; at native scale the legacy player render origin is y+11 (RenderOffset adds two subtiles, then Player.Render subtracts 5px). Review feet relative to that origin, silhouette, scale and painted facing."}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".json", evidence, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: ground=(%.1f,%.1f), direction=%v, sheet=%s, native capture=%s", name, sx, sy, state["direction"], want, path)
	if str(state, "body") != "png" || actual != want {
		t.Errorf("%s: body=%q sheet=%q mode=%q, want the Janissary %s sheet (capture retained)", name, str(state, "body"), actual, str(state, "animation_mode"), want)
	}
}
