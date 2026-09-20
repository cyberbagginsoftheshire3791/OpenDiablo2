//go:build playtest

package playtest

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestTheHands* are M4.4c-2a's playtest, the seventeenth script: the player's
// hands. The seam is a round that STOPS at the player's slot and holds the
// world frozen with it, four verbs on keys (F strike, L torch, E end), and the
// pace instrument (ROUND / PACE / WISH lines). Each act is its own top-level
// function so it gets a clean game AND a clean log ring -- the ROUND/PACE/WISH
// greps below read the ring (strigoi_read_log), and a shared session would mix
// two fights' lines into one grep.
//
// Acts 1, 2, 3, 4, 5, 9 and 10 are c-2a's and live here; act 11 (the class pin)
// is a unit test in d2game/d2gamescreen because strigoi_start_game never opens
// the select screen. Acts 6, 7 and 8 -- break-away, click-to-strike, the
// ambush -- are c-2b's and are present below as skips so the count is honest.
//
// The measure-first ruling and the units guard both apply: at least one act
// (act 1, the wait) compares a number the TEST chose -- 120 stepped frames --
// against a number the SYSTEM reported -- decision_seconds_round -- so a timer
// wired to a constant fails rather than passing quietly. Provider numbers are
// read through mustNum, never num, so a renamed field reddens instead of
// reading a real zero (A3).

const handsDT = 1.0 / 60 // the harness stepped tick, matching harness.timeDT

// handsStart pauses, starts the amazon at seed 1462, and silences the spawn
// table so a wandering pack cannot open a fight the act did not arrange. It
// leaves player_control at the value every script starts on -- policy, which
// the launcher sets after start_game precisely because the SHIPPED screen now
// asks for human (shippedCombatDials, ask 5). Acts that want a human turn set
// it themselves, below.
func handsStart(t *testing.T) (s *session, playerID, playerHandle string, px, py float64) {
	t.Helper()

	s = start(t)
	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Hands", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	s.call("strigoi_set_system_field", map[string]any{"system": "spawns", "field": "chance", "value": 0})

	p := s.call("strigoi_get_player", map[string]any{})

	return s, str(p, "id"), str(p, "handle"), num(p, "x"), num(p, "y")
}

// openTurn steps frames -- never world minutes, because the FIRST round has to
// be caught and step_world resolves whole rounds at a time -- until the combat
// provider reports awaiting. awaiting, not fighting: fighting goes true at
// tryStart, a round before the player's slot opens.
func openTurn(t *testing.T, s *session) {
	t.Helper()

	for i := 0; i < 400; i++ {
		if flag(t, combatState(s), "awaiting") {
			return
		}

		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	t.Fatalf("no player turn opened in 1600 frames: %v", combatState(s))
}

// lightState reads the light provider's block through the "state" sub-map, the
// way combatState does for combat -- reading a field off the top level yields
// zero for every one of them.
func lightState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state")
}

// worldMinutes is the clock's own count, read strictly.
func worldMinutes(t *testing.T, s *session) float64 {
	t.Helper()

	return mustNum(t, clockState(s), "world_minutes")
}

// lineField pulls one key=value number out of a ROUND or PACE log line.
//
// The LINE is the instrument -- it is what Josh reads out of strigoi.log and
// what a later human playtest is scored from -- so an assertion about it has to
// read the line, not the provider record the line was formatted from. It fails
// on an absent key rather than returning zero, for mustNum's reason (A3): a
// missing field would otherwise read as a real 0.0 and prove nothing.
func lineField(t *testing.T, line, key string) float64 {
	t.Helper()

	for _, f := range strings.Fields(line) {
		name, value, found := strings.Cut(f, "=")
		if !found || name != key {
			continue
		}

		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			t.Fatalf("the %q field is %q, not a number: %v\n%s", key, value, err, line)
		}

		return n
	}

	t.Fatalf("the line carries no %q field -- absent is not zero (A3):\n%s", key, line)

	return 0
}

// ringLines returns every log-ring line matching pattern that carries text.
// The instrument writes its ROUND/PACE/WISH lines through the game screen's
// d2util.Logger, whose writer includes the harness ring (log.SetOutput tees it
// in harnessEarlyInit, before CreateGame builds the logger), so the lines land
// here as "[Game Screen][INFO] ROUND ..." with the ANSI stripped. Proven in
// section 0's TestS0Instrument before this script was written.
func ringLines(s *session, pattern string) []string {
	out := s.call("strigoi_read_log", map[string]any{"pattern": pattern, "limit": 200})

	var lines []string

	if raw, ok := out["lines"].([]any); ok {
		for _, l := range raw {
			m, _ := l.(map[string]any)
			if text := str(m, "text"); text != "" {
				lines = append(lines, text)
			}
		}
	}

	return lines
}

// stepToNight sets a new moon and steps the clock to the night stage, so the
// torch and the dark-into-light acts run in the dark the design is about.
func stepToNight(t *testing.T, s *session) {
	t.Helper()

	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "moon", "value": 0})

	for i := 0; i < 60; i++ {
		if str(clockState(s), "stage") == "night" {
			return
		}

		s.call("strigoi_step_world", map[string]any{"world_minutes": 30.0})
	}

	t.Fatalf("could not reach the night stage: %v", clockState(s))
}

// --- act 1: the wait ---------------------------------------------------------

// TestTheHandsWait is the seam's first fact: a human turn opens on round one,
// the world clock FREEZES while it is open, and the decision timer counts the
// frames it stays open. The policy control at the end proves the freeze is the
// human turn's doing and not the fight's.
func TestTheHandsWait(t *testing.T) {
	s, _, playerHandle, px, py := handsStart(t)

	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "human"})

	dog := spawnNPC(t, s, "fallen1", px+1, py)
	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": playerHandle})

	openTurn(t, s)

	c := combatState(s)
	if got := mustNum(t, c, "round"); got != 1 {
		t.Fatalf("act 1: opening the fight frame by frame must catch it on round 1; round=%.0f", got)
	}

	// THE WORLD STOPS WITH THE TURN. worldRunning() reads Awaiting(), so
	// advanceWorld is gated off and the clock does not move for the 120 frames
	// the turn is open. NEGATIVE CONTROL (row 1): drop the !awaiting term from
	// worldRunning() and this delta stops being zero.
	w0 := worldMinutes(t, s)
	d0 := mustNum(t, c, "decision_seconds_round")

	s.call("strigoi_step", map[string]any{"frames": 120})

	c = combatState(s)
	w1 := worldMinutes(t, s)
	d1 := mustNum(t, c, "decision_seconds_round")

	if w1 != w0 {
		t.Fatalf("act 1: the world clock moved during an open turn (%.6f -> %.6f); "+
			"worldRunning() must freeze on awaiting", w0, w1)
	}

	// THE TEST CHOSE THE NUMBER: 120 frames at 1/60 s is 2.0 s of thinking, and
	// Wait() adds the frame's elapsed on every awaiting frame. NEGATIVE CONTROL
	// (row 2): make Wait() add world minutes instead of elapsed and this reads
	// 8.0 by day / 5.0 at night instead of 2.0.
	const wantDecide = 120 * handsDT

	if got := d1 - d0; math.Abs(got-wantDecide) > 2*handsDT {
		t.Fatalf("act 1: 120 open frames must add %.4f s of decision time; decision_seconds_round moved %.4f", wantDecide, got)
	}

	t.Logf("act 1: turn open on round 1, world frozen (%.4f), decision_seconds_round +%.4f s over 120 frames",
		w0, d1-d0)

	// --- the policy control: no human turn, no freeze ---
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "policy"})
	// Close the open human turn with a hold (its Action is unspent), so the
	// fight can run on under the policy from here.
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "commit", "value": "hold"})

	if flag(t, combatState(s), "awaiting") {
		t.Fatalf("act 1: a hold must close the open turn: %v", combatState(s))
	}

	pw0 := worldMinutes(t, s)
	s.call("strigoi_step_world", map[string]any{"world_minutes": 2.0})
	pw1 := worldMinutes(t, s)

	if flag(t, combatState(s), "awaiting") {
		t.Fatalf("act 1 control: a policy fight must not open a turn: %v", combatState(s))
	}

	if pw1-pw0 < 1.5 {
		t.Fatalf("act 1 control: under policy the world must run through the fight; clock moved only %.4f of 2.0", pw1-pw0)
	}

	t.Logf("act 1 control: under policy no turn opened and the clock advanced %.4f world minutes", pw1-pw0)
}

// --- act 2: the strike by key, and the seam's controls -----------------------

// TestTheHandsStrike is the F key and the four things it must and must not do.
// Executing the Action is NOT finishing the turn (the 17 September
// reconciliation): after F the turn is still open because the Move is unspent.
func TestTheHandsStrike(t *testing.T) {
	s, playerID, playerHandle, px, py := handsStart(t)

	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "human"})

	// CONTROL, the not-awaiting one: a commit with no turn waiting resolves
	// nothing and is counted. Done before any fight so the state is unambiguous.
	refused0 := mustNum(t, combatState(s), "commits_refused")
	s.call("strigoi_key", map[string]any{"key": "e"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := mustNum(t, combatState(s), "commits_refused"); got != refused0+1 {
		t.Fatalf("act 2: E with no turn waiting must be refused; commits_refused %.0f -> %.0f", refused0, got)
	}

	// A zombie1 (181 HP) so the fight survives every strike the controls throw.
	enemy := spawnNPC(t, s, "zombie1", px+1, py)
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": playerHandle})

	// Read the player's screen pixel now, for the ground click the both-spent
	// control needs later. The camera centres on the player, so this is stable.
	pl := s.call("strigoi_get_player", map[string]any{})
	sx, sy := pair(pl, "screen")

	openTurn(t, s)

	// --- the strike lands, and the Action is spent but the turn stays open ---
	s.call("strigoi_key", map[string]any{"key": "f"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	c := combatState(s)
	if !flag(t, c, "action_spent") {
		t.Fatalf("act 2: F must spend the Action: %v", c)
	}

	if !flag(t, c, "awaiting") || flag(t, c, "move_spent") {
		t.Fatalf("act 2: after F the turn stays open with the Move unspent (executing the Action is not finishing the turn): %v", c)
	}

	// A BLOW WENT THROUGH resolveBlow: the player's activation is in the log with
	// a band. NEGATIVE CONTROL (row 3): make Commit(strike) skip resolveBlow and
	// there is no such row.
	sawBand := false

	for _, row := range actionRows(t, c) {
		if str(row, "attacker") == playerID && str(row, "band") != "" {
			sawBand = true

			break
		}
	}

	if !sawBand {
		t.Fatalf("act 2: F must resolve a blow with a band; actions=%v", c["actions"])
	}

	// The world is still frozen because the turn is still open.
	w0 := worldMinutes(t, s)
	s.call("strigoi_step", map[string]any{"frames": 60})

	if w1 := worldMinutes(t, s); w1 != w0 {
		t.Fatalf("act 2: the turn is still open after F, so the world must stay frozen (%.6f -> %.6f)", w0, w1)
	}

	// --- end vs hold: E after F is Commit(end), and the ROUND row keeps strike -
	s.call("strigoi_key", map[string]any{"key": "e"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, combatState(s), "awaiting") {
		t.Fatalf("act 2: E must close a spent turn: %v", combatState(s))
	}

	if got := str(sub(combatState(s), "round_row"), "action"); got != "strike" {
		t.Fatalf("act 2: E after F closes the round as end and the ROUND row keeps action=strike; got %q", got)
	}

	// A fresh turn, and E on it is Commit(hold): the ROUND row reads hold.
	openTurn(t, s)
	s.call("strigoi_key", map[string]any{"key": "e"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := str(sub(combatState(s), "round_row"), "action"); got != "hold" {
		t.Fatalf("act 2: E on a fresh turn is a hold and the ROUND row reads action=hold; got %q", got)
	}

	t.Logf("act 2: E is one key, two verbs -- end kept the strike, hold read as hold")

	// --- the menu control (the replacement for the unfirable row 12) ---
	// A key never reaches GameControls while the escape menu is open (measured,
	// section 0 item 6), so it moves NEITHER counter and leaves the turn open.
	openTurn(t, s)
	pre := combatState(s)
	inp0, ref0 := mustNum(t, pre, "commits_by_input"), mustNum(t, pre, "commits_refused")

	s.call("strigoi_key", map[string]any{"key": "escape"})
	s.call("strigoi_key", map[string]any{"key": "f"})
	s.call("strigoi_step", map[string]any{"frames": 4})

	post := combatState(s)
	if mustNum(t, post, "commits_by_input") != inp0 || mustNum(t, post, "commits_refused") != ref0 {
		t.Fatalf("act 2: F under the open menu must reach neither counter; commits_by_input %.0f->%.0f refused %.0f->%.0f",
			inp0, mustNum(t, post, "commits_by_input"), ref0, mustNum(t, post, "commits_refused"))
	}

	if !flag(t, post, "awaiting") {
		t.Fatalf("act 2: the turn must still be open after a menu-blocked key: %v", post)
	}

	s.call("strigoi_key", map[string]any{"key": "escape"}) // close the menu
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, uiState(s), "escape_menu_open") {
		t.Fatalf("act 2: the escape menu must be closed before the ground click: %v", uiState(s))
	}

	t.Logf("act 2: a key under the open menu committed nothing and refused nothing -- consumed upstream")

	// --- the both-spent auto-end: a Move then the Action closes the turn ---
	// The turn is still open (action unspent, move unspent) from the menu
	// control. Spend the Move FIRST, with a ground click, while there is no
	// swing in flight to gate it -- OnPlayerMove is SpendMove's one caller. Then
	// F spends the Action and AutoEndTurn closes the round with no E at all.
	//
	// The click aims well WEST and up of the player's own sprite (a click on a
	// squad model selects it instead of walking, game_controls.go:620) and above
	// the bottom HUD rect (isInActiveMenusRect), with the enemy to the EAST.
	if flag(t, combatState(s), "move_spent") || flag(t, combatState(s), "action_spent") {
		t.Fatalf("act 2: the both-spent control needs a fresh turn: %v", combatState(s))
	}

	clickX, clickY := int(sx-160), int(sy-40)
	if clickX < 5 {
		clickX = 5
	}

	if clickY < 5 {
		clickY = 5
	}

	// A ground click is DROPPED while a swing is in flight (IsCasting gates it,
	// game_controls.go:612), and the earlier strike casts for ~123 frames. Wait
	// it out first, or the click reads as a range-guard failure that is really a
	// timing one -- section 0 item 5 measured exactly this window.
	for i := 0; i < 240 && flag(t, uiState(s), "casting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})
	}

	t.Logf("act 2: player screen ~(%.0f,%.0f); ground click at (%d,%d)", sx, sy, clickX, clickY)
	s.call("strigoi_click", map[string]any{"button": "left", "x": clickX, "y": clickY})
	s.call("strigoi_step", map[string]any{"frames": 4})

	if !flag(t, combatState(s), "move_spent") {
		t.Fatalf("act 2: a ground click must spend the Move (SpendMove); move_spent still false: %v", combatState(s))
	}

	if !flag(t, combatState(s), "awaiting") {
		t.Fatalf("act 2: the Move alone must not close the turn: %v", combatState(s))
	}

	s.call("strigoi_key", map[string]any{"key": "f"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// The Move + the Action auto-ends the turn with NO E. Assert against the
	// closed round's row, which persists: the walk carried the player out of
	// reach, so the fight itself may disengage a frame later -- but the round
	// that just closed carries both a strike and a move, which is the both-spent
	// signature, and awaiting is false.
	c = combatState(s)
	if flag(t, c, "awaiting") {
		t.Fatalf("act 2: Move + Action must auto-end the turn with no E; still awaiting: %v", c)
	}

	rr := sub(c, "round_row")
	if str(rr, "action") != "strike" || !flag(t, rr, "move") {
		t.Fatalf("act 2: the both-spent round must carry a strike AND a move; round_row=%v", rr)
	}

	t.Logf("act 2: a Move then a strike auto-ended the turn -- finishRound's third entry point")
}

// --- act 3: the torch by key -------------------------------------------------

// TestTheHandsTorch is L at night: it lights a carried torch AND spends the
// Action (the NEW RULE -- a torch costs the turn's one action in a fight), the
// burn drops by one per lit round, dousing keeps the burn, and once the one
// torch is spent L lights nothing.
func TestTheHandsTorch(t *testing.T) {
	s, _, playerHandle, px, py := handsStart(t)

	stepToNight(t, s)
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "human"})

	enemy := spawnNPC(t, s, "zombie1", px+1, py)
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": playerHandle})

	openTurn(t, s)

	// Nothing carried, Action unspent.
	if mustStr(t, lightState(s), "carried_source") != "" {
		t.Fatalf("act 3: the player should carry no light yet: %v", lightState(s))
	}

	if flag(t, combatState(s), "action_spent") {
		t.Fatalf("act 3: a fresh turn has an unspent Action: %v", combatState(s))
	}

	// L lights the torch AND spends the Action.
	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	light := lightState(s)
	if !flag(t, light, "carried_lit") {
		t.Fatalf("act 3: L must light a carried torch: %v", light)
	}

	if !flag(t, combatState(s), "action_spent") {
		t.Fatalf("act 3: L in a fight must spend the Action (the NEW RULE): %v", combatState(s))
	}

	burnLit := mustNum(t, light, "carried_burn")
	if burnLit < 55 {
		t.Fatalf("act 3: a fresh torch should carry about 60 world minutes; got %.1f", burnLit)
	}

	t.Logf("act 3: L lit a torch (burn %.1f) and spent the Action", burnLit)

	// --- burn drops by one per lit round ---
	round0 := mustNum(t, combatState(s), "round")
	litRounds := 0

	for k := 0; k < 6; k++ {
		s.call("strigoi_key", map[string]any{"key": "e"}) // end the spent turn
		s.call("strigoi_step", map[string]any{"frames": 2})

		// Step to the next open turn, letting the world (and the torch) burn.
		opened := false

		for i := 0; i < 200; i++ {
			if flag(t, combatState(s), "awaiting") {
				opened = true

				break
			}

			if !flag(t, combatState(s), "fighting") {
				break
			}

			s.call("strigoi_step", map[string]any{"frames": 4})
		}

		if !opened {
			break
		}

		litRounds++
	}

	roundsElapsed := mustNum(t, combatState(s), "round") - round0
	burnNow := mustNum(t, lightState(s), "carried_burn")

	// -1.0 per lit round, within a couple of frame slices and the stepper's
	// small overshoot. The test chose the round count; the system reported burn.
	if drop := burnLit - burnNow; math.Abs(drop-roundsElapsed) > 2.0 {
		t.Fatalf("act 3: burn must fall ~1 per lit round; %.1f rounds elapsed but burn dropped %.1f (%.1f -> %.1f)",
			roundsElapsed, drop, burnLit, burnNow)
	}

	t.Logf("act 3: %.0f lit rounds burned %.1f torch-minutes", roundsElapsed, burnLit-burnNow)

	// --- douse keeps the burn (row 4's assertion) ---
	// A fresh turn, so L reads the lit torch and douses it.
	if !flag(t, combatState(s), "awaiting") {
		openTurn(t, s)
	}

	burnBeforeDouse := mustNum(t, lightState(s), "carried_burn")
	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	light = lightState(s)
	if flag(t, light, "carried_lit") {
		t.Fatalf("act 3: L on a lit torch must douse it: %v", light)
	}

	// NEGATIVE CONTROL (row 4): make douse call Light.Remove and this reads 0.
	if got := mustNum(t, light, "carried_burn"); math.Abs(got-burnBeforeDouse) > 1.0 {
		t.Fatalf("act 3: douse keeps the burn (S1:102); it was %.1f and is now %.1f", burnBeforeDouse, got)
	}

	t.Logf("act 3: douse kept the burn at %.1f", mustNum(t, lightState(s), "carried_burn"))

	// --- TorchesCarried 0: the one torch is spent, so L lights nothing ---
	// Remove the carried source (as a burn-out would), then, on a fresh turn, L
	// finds no torch left to take out. The source id is a NUMBER from
	// source_list, not a string.
	srcID := -1.0

	if list, ok := light["source_list"].([]any); ok {
		for _, raw := range list {
			m, _ := raw.(map[string]any)
			if flag(t, m, "carried") {
				srcID = mustNum(t, m, "id")
			}
		}
	}

	if srcID < 0 {
		t.Fatalf("act 3: could not find the carried torch's id in source_list: %v", light)
	}

	s.call("strigoi_key", map[string]any{"key": "e"}) // close the doused turn
	s.call("strigoi_step", map[string]any{"frames": 2})
	s.call("strigoi_set_system_field", map[string]any{"system": "light", "field": "remove_source", "value": srcID})

	if mustStr(t, lightState(s), "carried_source") != "" {
		t.Fatalf("act 3: remove_source should leave nothing carried: %v", lightState(s))
	}

	openTurn(t, s)
	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := mustStr(t, lightState(s), "carried_source"); got != "" {
		t.Fatalf("act 3: with the one torch spent (TorchesCarried 0), L must light nothing; carried_source=%q", got)
	}

	t.Logf("act 3: the one torch spent, L lit nothing")
}

// --- act 3b: the torch burns out, and says so --------------------------------

// TestTheHandsTorchBurnsOut is the half of clause 3 that shipped a day after
// the rest of it: a torch at 0 minutes is REMOVED, and that removal is
// Light.Remove's first game caller -- the symbol M4.1 was reopened over.
//
// It gets its own game rather than a section of act 3, because the burn-out
// really does remove the carried source, which is the very thing act 3's last
// section removes by hand to prove TorchesCarried 0. Sharing a session would
// leave that section looking for a torch this one had already consumed.
//
// It exists because the telemetry had no test: `git grep TORCH_OUT -- playtest`
// found nothing, so the line Josh would read a burnt-out fight from, and the
// PACE row's torch_out beside it, were both written and never asserted.
func TestTheHandsTorchBurnsOut(t *testing.T) {
	s, _, playerHandle, px, py := handsStart(t)

	stepToNight(t, s)
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "human"})

	enemy := spawnNPC(t, s, "fallen1", px+1, py)
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": playerHandle})

	openTurn(t, s)

	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if !flag(t, lightState(s), "carried_lit") {
		t.Fatalf("act 3b: L must light the torch: %v", lightState(s))
	}

	// One minute of burn is the number the TEST chose; the removal, the line and
	// the provider's zeros are what the SYSTEM reports.
	s.call("strigoi_set_system_field", map[string]any{"system": "light", "field": "carried_burn", "value": 1.0})

	burntOut := false

	for i := 0; i < 200; i++ {
		if !flag(t, lightState(s), "carried_lit") {
			burntOut = true

			break
		}

		if !flag(t, combatState(s), "fighting") {
			break
		}

		if flag(t, combatState(s), "awaiting") {
			s.call("strigoi_key", map[string]any{"key": "e"}) // close the turn, spend the minute
		}

		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	if !burntOut {
		t.Fatalf("act 3b: a torch with one minute left must burn out inside a round: %v", lightState(s))
	}

	// The REMOVAL, not merely an unlit flag. The provider reports carried_burn
	// 0.0 and carried_lit false from its DEFAULTS once nothing is carried, which
	// is exactly why night_light_test.go's post-burn-out read still holds: the
	// fields stay present rather than going absent.
	light := lightState(s)

	if got := mustStr(t, light, "carried_source"); got != "" {
		t.Fatalf("act 3b: a burnt-out torch is removed, not just doused; carried_source=%q", got)
	}

	if got := mustNum(t, light, "carried_burn"); got != 0 {
		t.Fatalf("act 3b: a removed torch reports 0.0 burn, present and not absent; got %.2f", got)
	}

	outLines := ringLines(s, "TORCH_OUT")
	if len(outLines) == 0 {
		t.Fatalf("act 3b: the burnt-out torch must write a TORCH_OUT line; the ring carried none")
	}

	t.Logf("act 3b: %s", outLines[len(outLines)-1])
}

// --- act 4: the ROUND and PACE lines -----------------------------------------

// TestTheHandsPace runs one human fight to its close and reads the instrument
// two ways at once: off the log ring (what a friend greps) and off the combat
// provider's pace{} block (what a script asserts). They MUST agree, because the
// game screen formats the line from the same lastPace the provider reports --
// the fourth provider rule, one source and two readers. The policy control at
// the end proves the decision timer measures people, not fights.
func TestTheHandsPace(t *testing.T) {
	s, _, playerHandle, px, py := handsStart(t)

	stepToNight(t, s)
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "human"})
	// A crit band shortens the fight so it CLOSES inside the act -- a PACE line
	// exists only at end().
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "forced_band", "value": "crit"})

	enemy := spawnNPC(t, s, "fallen1", px+1, py)
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": playerHandle})

	openTurn(t, s)

	// Light a torch so the PACE line's torch_lit_rounds is a real number.
	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// The burn the torch starts the fight with: the PACE line's burn_after is
	// measured against it below, which is how the lit-round count gets a second,
	// independent side.
	burnLit := mustNum(t, lightState(s), "carried_burn")
	closed := false

	for turn := 0; turn < 12; turn++ {
		// Wait a known span so the decision timer has something to measure.
		s.call("strigoi_step", map[string]any{"frames": 12})

		if flag(t, combatState(s), "awaiting") {
			s.call("strigoi_key", map[string]any{"key": "f"}) // strike
			s.call("strigoi_step", map[string]any{"frames": 2})
		}

		if flag(t, combatState(s), "awaiting") {
			s.call("strigoi_key", map[string]any{"key": "e"}) // close the turn
			s.call("strigoi_step", map[string]any{"frames": 2})
		}

		// Step to the next open turn or the fight's end.
		for i := 0; i < 200; i++ {
			c := combatState(s)
			if !flag(t, c, "fighting") {
				closed = true

				break
			}

			if flag(t, c, "awaiting") {
				break
			}

			s.call("strigoi_step", map[string]any{"frames": 4})
		}

		if closed {
			break
		}
	}

	if !closed {
		t.Fatalf("act 4: the fight never closed: %v", combatState(s))
	}

	c := combatState(s)
	pace := sub(c, "pace")

	if str(pace, "encounter") == "" {
		t.Fatalf("act 4: a fight the player fought must write a pace row: %v", c)
	}

	if got := str(pace, "control"); got != "human" {
		t.Fatalf("act 4: a fought fight is control=human; got %q", got)
	}

	paceRounds := mustNum(t, pace, "rounds")

	// The log ring and the provider must agree, and the ROUND line count is the
	// fight's round count. NEGATIVE CONTROL (row 5): read pace.rounds after
	// end() has cleared the encounter and this stops matching.
	roundLines := ringLines(s, "ROUND ")
	if float64(len(roundLines)) != paceRounds {
		t.Fatalf("act 4: the ring carried %d ROUND lines but the pace row says %.0f rounds -- log and provider disagree",
			len(roundLines), paceRounds)
	}

	paceLines := ringLines(s, "PACE ")
	if len(paceLines) == 0 {
		t.Fatalf("act 4: the ring carried no PACE line")
	}

	encTag := "encounter=" + str(pace, "encounter")
	if !strings.Contains(paceLines[len(paceLines)-1], encTag) {
		t.Fatalf("act 4: the PACE line must name the fight (%q): %q", encTag, paceLines[len(paceLines)-1])
	}

	last := paceLines[len(paceLines)-1]
	paceLit := lineField(t, last, "torch_lit_rounds")

	// torch_lit_rounds, TWICE -- and the presence check that stood here was
	// hiding a real off-by-one. Two readings of this act disagreed for a day.
	// The first (19 Sep, morning) read 4 stamped lines against a counted 3, took
	// the BURN as the arbiter (60.0 -> 57.0, three torch-minutes) and concluded
	// the counter was right and the tally wrong. The second (19 Sep, evening)
	// found the ordering: writeRoundLine ran AFTER writePaceLine on the closing
	// frame, so the row could not have seen the last round. The second reading
	// was correct and the first was the burn's one-round slack being mistaken
	// for agreement. Fixed 20 Sep in applyFightingActivity's close branch.
	// The tally is gone; these two stand in its place, and neither can be
	// satisfied by a constant.
	//
	// (a) LOG against PROVIDER. writeRoundLine increments the counter exactly
	// when it stamps a line torch=lit, so the two readers of that one source
	// must agree -- and this is the assertion that reddens if the increment ever
	// drifts from the stamp.
	litLines := 0

	for _, line := range ringLines(s, "ROUND ") {
		if strings.Contains(line, "torch=lit") {
			litLines++
		}
	}

	// EQUALITY, and it was a range until 20 Sep. MEASURED 19 Sep: 4 stamped
	// lines against a counted 3, and the counter CANNOT drift from the stamp --
	// writeRoundLine increments and stamps in the same breath -- so the gap was
	// never the counter, it was the ORDER. The fight-close edge lives in the
	// meters switch (!fighting && wasFighting) and end() captures the final
	// RoundRow, but writeRoundLine ran later in the frame, so the row was
	// formatted before the last round's line existed. applyFightingActivity now
	// calls writeRoundLine immediately before writePaceLine; the later call is
	// keyed on encounter#round and no-ops. MEASURED AFTER, 20 Sep: 4 lines,
	// torch_lit_rounds=4, gap 0.
	//
	// The one-line slack is GONE ON PURPOSE. It was the defect's tolerance, and
	// leaving it would let the defect return green.
	if float64(litLines) != paceLit {
		t.Fatalf("act 4: %d ROUND lines say torch=lit and the PACE line counts %.0f -- these read one source and must agree exactly\n%s",
			litLines, paceLit, last)
	}

	// (b) THE RULE, with each side computed by a different subsystem: a lit
	// torch spends one world minute per round (E6/S1:102), the game screen
	// counts the rounds and the light model spends the burn. An off-by-one in
	// either path breaks this while both stay green on their own.
	//
	// THE SLACK IS ONE ROUND AND IT IS DIRECTIONAL, which is stricter than the
	// symmetric abs() that stood here. The round the torch is LIT IN is stamped
	// torch=lit but burns only the part of the minute after the press, so the
	// count may lead the burn by up to one. It can never LAG it: a minute cannot
	// be spent by a round that was not stamped lit. MEASURED 20 Sep, after the
	// ordering fix: 4 lit rounds, 3.0 torch-minutes -- lead of exactly 1, the
	// whole of the allowance, and the symmetric form was passing on its boundary
	// while admitting a -1 that the mechanism forbids.
	spent := burnLit - lineField(t, last, "burn_after")
	if lead := paceLit - spent; lead < 0 || lead > 1 {
		t.Fatalf("act 4: %.0f lit rounds must spend %.0f or %.0f torch-minutes (the lighting round burns a part minute); burn fell %.1f (%.1f -> %.1f)\n%s",
			paceLit, paceLit-1, paceLit, spent, burnLit, lineField(t, last, "burn_after"), last)
	}

	// decide_s: the FIGHT total must be the sum of the per-round decide_s values
	// the ROUND lines carry. Act 1 already pins one round against a number the
	// test chose (120 frames -> 2.0 s); this pins the total against its own
	// parts, so a total wired to a constant, double-counted, or reset between
	// rounds reddens. The slack is the lines' one decimal place.
	roundDecide := 0.0

	for _, line := range ringLines(s, "ROUND ") {
		roundDecide += lineField(t, line, "decide_s")
	}

	paceDecide := lineField(t, last, "decide_s")

	if math.Abs(paceDecide-roundDecide) > 0.05*float64(len(roundLines))+0.05 {
		t.Fatalf("act 4: the fight's decide_s is %.1f but its rounds sum to %.1f -- the total is not its parts\n%s",
			paceDecide, roundDecide, last)
	}

	if decide := mustNum(t, pace, "decide_seconds"); decide <= 0 {
		t.Fatalf("act 4: a human fight must accrue decision time; decide_s=%.4f", decide)
	}

	t.Logf("act 4: %d ROUND lines == pace.rounds %.0f; %d torch=lit lines == torch_lit_rounds %.0f == %.1f torch-minutes burned; decide_s %.1f == %.1f summed over its rounds\n%s",
		len(roundLines), paceRounds, litLines, paceLit, burnLit-lineField(t, last, "burn_after"),
		paceDecide, roundDecide, last)

	// --- the policy control: a policy fight accrues no decision time ---
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "policy"})
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "forced_band", "value": ""})

	enemy2 := spawnNPC(t, s, "fallen1", px+1, py)
	s.call("strigoi_watch", map[string]any{"watcher": enemy2, "target": playerHandle})

	sawAwaiting := false

	for i := 0; i < 40; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1.0})

		if flag(t, combatState(s), "awaiting") {
			sawAwaiting = true

			break
		}

		if flag(t, combatState(s), "fighting") && mustNum(t, combatState(s), "decision_seconds") > 0 {
			t.Fatalf("act 4 control: a policy fight must not accrue decision time; decision_seconds=%.4f",
				mustNum(t, combatState(s), "decision_seconds"))
		}
	}

	if sawAwaiting {
		t.Fatalf("act 4 control: a policy fight must never open a turn")
	}

	t.Logf("act 4 control: a policy fight opened no turn and accrued no decision time")
}

// --- act 5: the wish note ----------------------------------------------------

// TestTheHandsWish is the console verb that lands a friend's own words beside
// the numbers. It reads the fight LIVE: outside a fight the encounter is "-",
// inside one it is the fight's id. NEGATIVE CONTROL (row 9): read the fight from
// a source that survives end() and the in-fight line reports encounter=- while a
// fight is running.
func TestTheHandsWish(t *testing.T) {
	s, _, playerHandle, px, py := handsStart(t)

	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "human"})

	// Outside any fight: the variadic marker lets the words arrive unquoted.
	if msg := s.callErr("strigoi_run_console", map[string]any{"command": "wish out of the fight I wanted to run"}); msg != "" {
		t.Fatalf("act 5: an unquoted wish must run (the variadic marker); got %q", msg)
	}

	outside := findLine(ringLines(s, "WISH "), "out of the fight")
	if outside == "" {
		t.Fatalf("act 5: no WISH line for the outside wish: %v", ringLines(s, "WISH "))
	}

	if !strings.Contains(outside, "encounter=-") {
		t.Fatalf("act 5: a wish outside a fight reads encounter=-; got %q", outside)
	}

	// Inside a fight: the encounter is the live one.
	enemy := spawnNPC(t, s, "zombie1", px+1, py)
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": playerHandle})

	openTurn(t, s)

	encounter := str(combatState(s), "encounter")
	if encounter == "" {
		t.Fatalf("act 5: the fight has no encounter id: %v", combatState(s))
	}

	if msg := s.callErr("strigoi_run_console", map[string]any{"command": "wish inside the fight I wanted to hide"}); msg != "" {
		t.Fatalf("act 5: the in-fight wish must run; got %q", msg)
	}

	inside := findLine(ringLines(s, "WISH "), "inside the fight")
	if inside == "" {
		t.Fatalf("act 5: no WISH line for the in-fight wish: %v", ringLines(s, "WISH "))
	}

	if !strings.Contains(inside, "encounter="+encounter) {
		t.Fatalf("act 5: a wish inside the fight must name the live encounter %q; got %q", encounter, inside)
	}

	if strings.Contains(inside, "encounter=-") {
		t.Fatalf("act 5: the in-fight wish read encounter=- while a fight was running: %q", inside)
	}

	t.Logf("act 5: outside %q ; inside %q", outside, inside)
}

// findLine returns the first line containing needle, or "".
func findLine(lines []string, needle string) string {
	for _, l := range lines {
		if strings.Contains(l, needle) {
			return l
		}
	}

	return ""
}

// --- act 10: dark into light against a real verb -----------------------------

// TestTheHandsDarkIntoLight proves R2 §3 bullet 7 in a real build for the first
// time: a placed hearth lights the enemy's ground and not the player's, and the
// player's own strike (F, a real verb) reads dark-into-light. The enemy is
// spawned far enough that the hearth on its tile leaves the player in the dark;
// lit_level is then calibrated between the two measured levels, which is the
// dial the resolver's own comment says proves this rule. The torch-lit control
// lights the player too, and the advantage stops firing.
func TestTheHandsDarkIntoLight(t *testing.T) {
	s, playerID, playerHandle, px, py := handsStart(t)

	stepToNight(t, s)
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "human"})
	// The enemy two tiles east, and a reach of two, so it floors to a tile of
	// its own (not the player's) while the strike still lands.
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "adjacent_tiles", "value": 2})

	enemy := spawnNPC(t, s, "zombie1", px+2, py)
	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": playerHandle})

	openTurn(t, s) // the world freezes here, so the enemy stops where it stands

	// A hearth five tiles EAST of the enemy: both participants sit in the
	// hearth's falloff, the enemy nearer and so brighter, the player dimmer --
	// the light GRADIENT dark-into-light needs, which at night's uniform ambient
	// no carried source can make (the resolver's own comment, combat_resolver.go).
	e := s.call("strigoi_get_entity", map[string]any{"handle": enemy})
	ex, ey := math.Floor(mustNum(t, e, "x")), math.Floor(mustNum(t, e, "y"))

	s.call("strigoi_set_system_field", map[string]any{
		"system": "light", "field": "place_source",
		"value": map[string]any{"kind": "hearth", "x": ex + 5, "y": ey},
	})

	// The two tiles' levels, read off the combat provider's own participant rows
	// -- the SAME illum.Level the resolver's advantage() reads.
	c := combatState(s)
	playerLevel := participantLight(t, c, "player")
	enemyLevel := participantLight(t, c, "enemy")

	if enemyLevel-playerLevel < 0.1 {
		t.Fatalf("act 10: the hearth must light the enemy's ground more than the player's; enemy %.3f player %.3f",
			enemyLevel, playerLevel)
	}

	// lit_level between the two, so the player stands in the dark and the enemy
	// in the light. This is the dial the resolver names as the way to prove the
	// rule; the hearth is what makes the two levels differ at all.
	litLevel := (playerLevel + enemyLevel) / 2
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "lit_level", "value": litLevel})

	// The player strikes from the dark into the light.
	s.call("strigoi_key", map[string]any{"key": "f"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if !playerBlowHasWhy(t, combatState(s), playerID, "dark-into-light") {
		t.Fatalf("act 10: the player's strike from the dark at a lit enemy must read dark-into-light; actions=%v",
			combatState(s)["actions"])
	}

	t.Logf("act 10: player at %.3f struck an enemy at %.3f (lit_level %.3f) -- dark-into-light",
		playerLevel, enemyLevel, litLevel)

	// --- the torch-lit control: light the player too, and it stops firing ---
	s.call("strigoi_key", map[string]any{"key": "e"}) // close the spent turn
	s.call("strigoi_step", map[string]any{"frames": 2})

	// Give the player a lit torch, so his own tile is now bright. Both lit means
	// no advantage either way (a lit torch also lights every adjacent enemy --
	// state.md -- which is exactly why the torch cannot MAKE dark-into-light).
	s.call("strigoi_set_system_field", map[string]any{"system": "light", "field": "carried_source", "value": "torch"})

	openTurn(t, s)
	s.call("strigoi_key", map[string]any{"key": "f"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if playerBlowHasWhy(t, combatState(s), playerID, "dark-into-light") {
		t.Fatalf("act 10 control: with the player's own torch lit, dark-into-light must not fire; actions=%v",
			combatState(s)["actions"])
	}

	t.Logf("act 10 control: with the player lit, the advantage stopped firing")
}

// participantLight reads the light_here of the first participant on a side.
func participantLight(t *testing.T, combat map[string]any, side string) float64 {
	t.Helper()

	parts, _ := combat["participants"].([]any)
	for _, raw := range parts {
		row, _ := raw.(map[string]any)
		if str(row, "side") == side {
			return mustNum(t, row, "light_here")
		}
	}

	t.Fatalf("act 10: no %s participant to read light_here from: %v", side, parts)

	return 0
}

// playerBlowHasWhy reports whether the player landed a blow this round whose
// advantage_why matches want.
func playerBlowHasWhy(t *testing.T, combat map[string]any, playerID, want string) bool {
	t.Helper()

	for _, row := range actionRows(t, combat) {
		if str(row, "attacker") == playerID && str(row, "advantage_why") == want {
			return true
		}
	}

	return false
}

// --- act 9: no turn for the dead ---------------------------------------------

// TestTheHandsPlayerDeath is the seam's last fact: a dead player takes no turn.
// The pack kills him under the policy (the fast path), the encounter ends
// player_dead with awaiting false, and then -- under human control, where a
// turn WOULD open -- the corpse is handed none. NEGATIVE CONTROL (row 11):
// tryStart's dead-quarry guard is what keeps the turn from re-opening; remove it
// and a turn opens against the corpse.
func TestTheHandsPlayerDeath(t *testing.T) {
	s, _, playerHandle, px, py := handsStart(t)

	// Policy and a pack of TOUGH enemies: the policy player strikes back one a
	// round, so weak dogs die faster than they can wear him down and he wins. A
	// wall of zombie1s (181 HP each) he cannot clear keeps every bite landing,
	// and their bodies wear him to zero -- the milestone's grim sentence.
	for i := 0; i < 8; i++ {
		z := spawnNPC(t, s, "zombie1", px+1, py)
		s.call("strigoi_watch", map[string]any{"watcher": z, "target": playerHandle})
	}

	// Open the fight first (disengage is a per-encounter flag and is refused
	// when none is running), then pull the whole pack into it so they all bite.
	for i := 0; i < 150 && !flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1.0})
	}

	if flag(t, combatState(s), "fighting") {
		s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "disengage", "value": true})
	}

	dead := false

	for i := 0; i < 200; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1.0})

		if mustNum(t, metersState(s), "health") <= 0 {
			dead = true

			break
		}
	}

	if !dead {
		t.Fatalf("act 9: the pack failed to kill the player: health %.0f", mustNum(t, metersState(s), "health"))
	}

	c := combatState(s)
	if got := str(c, "ended_reason"); got != "player_dead" {
		t.Fatalf("act 9: the fight must end player_dead; got %q", got)
	}

	if flag(t, c, "awaiting") {
		t.Fatalf("act 9: a dead player is not awaiting a turn: %v", c)
	}

	if !flag(t, metersState(s), "dead") {
		t.Fatalf("act 9: the meters must agree the player is dead: %v", metersState(s))
	}

	// The dead take no turn, even under human control, even with the pack still
	// aware and in reach: tryStart skips a dead quarry.
	s.call("strigoi_set_system_field", map[string]any{"system": "combat", "field": "player_control", "value": "human"})

	enc0 := mustNum(t, combatState(s), "encounters")

	for i := 0; i < 30; i++ {
		s.call("strigoi_step", map[string]any{"frames": 4})

		if flag(t, combatState(s), "awaiting") {
			t.Fatalf("act 9: a turn opened for the corpse -- the dead-quarry guard is gone")
		}
	}

	if got := mustNum(t, combatState(s), "encounters"); got != enc0 {
		t.Fatalf("act 9: no new fight may open against the corpse; encounters %.0f -> %.0f", enc0, got)
	}

	t.Logf("act 9: the player died (player_dead, not awaiting) and the corpse was handed no turn")
}

// --- acts 6, 7, 8: deferred to c-2b ------------------------------------------

// TestTheHandsDeferredToC2b names the three acts c-2b owns, so the count of what
// this milestone did NOT build is honest rather than silent. Each needs a piece
// c-2b ships: the break-away needs the MoveTiles range guard, click-to-strike
// needs the hit test over the strip, and the ambush needs StartByPlayer.
func TestTheHandsDeferredToC2b(t *testing.T) {
	t.Run("act6_break_away", func(t *testing.T) { t.Skip("c-2b: the MoveTiles range guard and the break-away act") })
	t.Run("act7_click_to_strike", func(t *testing.T) { t.Skip("c-2b: click-to-strike over the strip, and the swing wait") })
	t.Run("act8_ambush", func(t *testing.T) { t.Skip("c-2b: StartByPlayer and the ambush flags") })
}
