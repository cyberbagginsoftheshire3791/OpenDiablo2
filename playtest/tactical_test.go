//go:build playtest

package playtest

import (
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

// TestTacticalFight is T1's playtest (23 Sep 2026): the tactical layer in a
// real launch, at the SHIPPED combat dials. Josh played the build on 19-20 Sep
// and said "it isn't even turn based"; this script is the list of things that
// sentence says a real fight must now do, each against its control:
//
//  1. the fight OPENS while the enemy is still out of reach (engage radius),
//     and the enemy STOPS walking the moment it does;
//  2. the world is held for the player's whole turn;
//  3. a Move past the Move's range is REFUSED, not clamped;
//  4. on its own turn the enemy WALKS (steps_ordered) and ends closer;
//  5. a closed round costs EXACTLY one round's world minutes -- the world is
//     held through the enemy's visible turn and paid for per round;
//  6. the overlay is on screen (a real 800x600 frame of the player's turn).
//
// It arranges the fight (spawn + watch), and says so: the unaided script is the
// one that proves fights happen on their own. This one proves what a fight IS
// once it happens.
func TestTacticalFight(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Tactics", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	// The spawn tables are silenced so a wandering pack cannot join the fight
	// this script arranges (hands_test's reason, and its same line).
	setField(s, "spawns", "chance", 0)

	pl := s.call("strigoi_get_player", map[string]any{})
	playerID, playerHandle, px, py := str(pl, "id"), str(pl, "handle"), num(pl, "x"), num(pl, "y")

	// The launcher drops every script to policy after start_game; the SHIPPED
	// screen is human, so this puts back exactly what ships.
	setField(s, "combat", "player_control", "human")

	c := combatState(s)
	if !flag(t, c, "paced") {
		t.Fatalf("the shipped screen must run the tactical layer: paced=false: %v", c["paced"])
	}

	if !flag(t, c, "has_stepper") {
		t.Fatal("the screen must attach itself as the Stepper, or packs never walk on their turn")
	}

	engage := mustNum(t, c, "engage_radius")
	enemyMove := mustNum(t, c, "enemy_move_tiles")
	moveTiles := mustNum(t, c, "move_tiles")
	roundMinutes := mustNum(t, c, "round_minutes")

	if engage < 3 {
		t.Fatalf("a shipped fight must open before the enemy is adjacent: engage_radius=%.0f", engage)
	}

	// A zombie1 (181 HP) four tiles off, on a bearing the map lets it walk.
	spot := clearSpotAt(t, s, px, py, 4)
	enemy := spawnNPC(t, s, "zombie1", spot[0], spot[1])
	enemyID := entityID(t, s, enemy)
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": playerHandle})

	// --- 1: it opens out of reach, and the enemy stops -------------------------
	fightNow(t, s)

	c = combatState(s)
	row := participant(t, c, enemyID)

	if flag(t, row, "adjacent") {
		t.Fatalf("act 1: the fight must open BEFORE the enemy is adjacent -- that is the engage radius: %v", row)
	}

	openX, openY := mustNum(t, row, "x"), mustNum(t, row, "y")

	openTurn(t, s)

	c = combatState(s)
	row = participant(t, c, enemyID)

	if moved := math.Hypot(mustNum(t, row, "x")-openX, mustNum(t, row, "y")-openY); moved > 0.35 {
		t.Fatalf("act 1: the enemy kept walking after the fight opened (%.2f tiles) -- a released chase leaves its path in flight, and Halt is what stops it", moved)
	}

	if mustNum(t, c, "round") != 1 {
		t.Fatalf("act 1: the first turn must be round 1: %v", c["round"])
	}

	t.Logf("act 1: fight open with the enemy at %.2f,%.2f (not adjacent), engage %.0f", openX, openY, engage)

	// --- 2: the world is held for his turn --------------------------------------
	w0 := worldMinutes(t, s)
	s.call("strigoi_step", map[string]any{"frames": 90})

	if w1 := worldMinutes(t, s); w1 != w0 {
		t.Fatalf("act 2: the world moved during his turn (%.6f -> %.6f)", w0, w1)
	}

	// --- 6: the overlay is on screen --------------------------------------------
	shot := s.call("strigoi_screenshot", map[string]any{"name": "t1-tactical-player-turn"})
	f, err := os.Open(str(shot, "path"))
	if err != nil {
		t.Fatalf("act 6: screenshot missing on disk: %v", err)
	}

	img, err := png.Decode(f)
	_ = f.Close()

	if err != nil {
		t.Fatalf("act 6: screenshot decode: %v", err)
	}

	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Fatalf("act 6: screenshot is %dx%d", b.Dx(), b.Dy())
	}

	// The panel is a near-black box with a warm edge at (180..620, 450..532); a
	// pixel on its top edge must be that edge colour, which nothing in a D2 map
	// frame paints by accident at exactly that spot and hue.
	if r, g, b, _ := img.At(400, 449).RGBA(); r>>8 < 0x40 || r>>8 > 0x70 || g>>8 < 0x30 || g>>8 > 0x5a || b>>8 > 0x38 {
		t.Fatalf("act 6: no combat panel edge at (400,449): rgb=(%d,%d,%d)", r>>8, g>>8, b>>8)
	}

	t.Logf("act 6: frame %s", str(shot, "path"))

	// --- 3: a Move past the range is refused -----------------------------------
	p := s.call("strigoi_get_player", map[string]any{})
	sx, sy := pair(p, "screen")
	ppx, ppy := num(p, "x"), num(p, "y")

	// Away from the enemy, on a clear bearing, one tile past the Move -- a tile
	// he COULD walk to, so the refusal is about range and nothing else. The
	// control at the end of the script moves the dial and clicks it again.
	far := moveTiles + 1
	farSpot := clearSpotAway(t, s, ppx, ppy, far, spot)

	clickTile(s, sx, sy, ppx, ppy, farSpot)
	s.call("strigoi_step", map[string]any{"frames": 30})

	c = combatState(s)
	if flag(t, c, "move_spent") {
		t.Fatalf("act 3: a click %.0f tiles off must be REFUSED with a Move of %.0f, and it spent the Move", far, moveTiles)
	}

	// REFUSED FOR THE RIGHT REASON. A click that missed the map, landed on the
	// HUD or hit a blocked tile also leaves move_spent false, so the reason the
	// panel gives is the assertion, not the absence of a walk.
	if notice := str(uiState(s), "tactical_notice"); !strings.HasPrefix(notice, "Too far") {
		t.Fatalf("act 3: the refusal must be about range; the panel says %q", notice)
	}

	p = s.call("strigoi_get_player", map[string]any{})
	if moved := math.Hypot(num(p, "x")-ppx, num(p, "y")-ppy); moved > 0.2 {
		t.Fatalf("act 3: the refused click still walked him %.2f tiles (refuse, not clamp)", moved)
	}

	// --- 4 and 5: he holds; the enemy walks on its turn; one round's minute ----
	steps0 := mustNum(t, c, "steps_ordered")
	d0 := math.Max(math.Abs(openX-ppx), math.Abs(openY-ppy))
	w0 = worldMinutes(t, s)

	s.call("strigoi_key", map[string]any{"key": "e"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	c = combatState(s)
	if flag(t, c, "awaiting") {
		t.Fatalf("act 4: E must close his turn: %v", c)
	}

	if !flag(t, c, "world_held") {
		t.Fatalf("act 5: the world must stay held through the enemy's turn: %v", c)
	}

	openTurn(t, s)

	c = combatState(s)

	if got := mustNum(t, c, "steps_ordered"); got <= steps0 {
		t.Fatalf("act 4: the enemy took its turn without walking (steps_ordered %.0f -> %.0f)", steps0, got)
	}

	row = participant(t, c, enemyID)
	d1 := math.Max(math.Abs(mustNum(t, row, "x")-ppx), math.Abs(mustNum(t, row, "y")-ppy))

	if d1 >= d0 || d0-d1 > enemyMove+0.6 {
		t.Fatalf("act 4: the enemy must close by at most %.0f tiles on its turn: %.2f -> %.2f", enemyMove, d0, d1)
	}

	if got := mustNum(t, c, "round"); got != 2 {
		t.Fatalf("act 5: one hold must close exactly one round; round=%.0f", got)
	}

	// THE NUMBER THE TEST CHOSE against the number the system reported: one
	// round, one round's world minutes. Frames ran for the whole enemy turn --
	// dozens of them -- and none of them moved the clock.
	if got := worldMinutes(t, s) - w0; math.Abs(got-roundMinutes) > 1e-6 {
		t.Fatalf("act 5: a closed round must cost exactly %.3f world minutes; the clock moved %.6f", roundMinutes, got)
	}

	t.Logf("act 4/5: enemy %.2f -> %.2f tiles in one turn, round 2, clock +%.3f", d0, d1, roundMinutes)

	// --- 3's CONTROL: the same click with a Move long enough is ACCEPTED --------
	// Without this, act 3 passes for a click that missed the map, a blocked
	// tile, or a handler that refuses everything. Only the dial moves.
	setField(s, "combat", "move_tiles", far+1)

	p = s.call("strigoi_get_player", map[string]any{})
	sx, sy = pair(p, "screen")
	ppx, ppy = num(p, "x"), num(p, "y")

	clickTile(s, sx, sy, ppx, ppy, farSpot)
	s.call("strigoi_step", map[string]any{"frames": 30})

	if c = combatState(s); !flag(t, c, "move_spent") {
		t.Fatalf("act 3 control: the same tile with move_tiles=%.0f must be accepted; move_spent=false; panel says %q",
			far+1, str(uiState(s), "tactical_notice"))
	}

	_ = playerID
}

// clickTile clicks a point on the player's lattice (whole tiles from where he
// stands, which is where the overlay draws its diamonds), from his own screen
// point: +1 tile x is (+80,+40) px and +1 tile y is (-80,+40) (state.md).
func clickTile(s *session, sx, sy, px, py float64, tile [2]float64) {
	fx := tile[0] - px
	fy := tile[1] - py

	s.call("strigoi_click", map[string]any{"button": "left",
		"x": int(sx + 80*(fx-fy)), "y": int(sy + 40*(fx+fy))})
}

// clearSpotAway is clearSpotAt that will not choose the enemy's bearing.
func clearSpotAway(t *testing.T, s *session, px, py, dist float64, avoid [2]float64) [2]float64 {
	t.Helper()

	for _, d := range [][2]float64{{1, 0}, {0, 1}, {-1, 0}, {0, -1}, {1, 1}, {-1, -1}, {1, -1}, {-1, 1}} {
		x, y := px+d[0]*dist, py+d[1]*dist
		if (x-px)*(avoid[0]-px)+(y-py)*(avoid[1]-py) > 0 {
			continue // same side as the enemy
		}

		if flag(t, s.call("strigoi_find_path", map[string]any{"to_x": x, "to_y": y}), "straight_line_clear") {
			return [2]float64{x, y}
		}
	}

	t.Fatalf("no clear bearing %.0f tiles from %.2f,%.2f away from the enemy", dist, px, py)

	return [2]float64{}
}

// clearSpotAt is a tile `dist` tiles from the player on the first orthogonal
// bearing whose straight line is clear, so a spawned enemy has a walk to make.
// Seed 1462 draws a generated map, so the bearing is swept, never hardcoded.
func clearSpotAt(t *testing.T, s *session, px, py, dist float64) [2]float64 {
	t.Helper()

	for _, d := range [][2]float64{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
		x, y := px+d[0]*dist, py+d[1]*dist
		if flag(t, s.call("strigoi_find_path", map[string]any{"to_x": x, "to_y": y}), "straight_line_clear") {
			return [2]float64{x, y}
		}
	}

	t.Fatalf("no orthogonal bearing %.0f tiles from %.2f,%.2f is clear", dist, px, py)

	return [2]float64{}
}
