//go:build playtest

package playtest

import (
	"math"
	"strings"
	"testing"
)

// TestSurvivalMeters is M4.2's playtest script (Constitution VI.2): S1 §5's
// signed assertion, as far as the signed build-shape note says M4.2 owns it.
//
// That sentence is three clauses and only one was buildable when the note was
// written — there is no combat state machine until M4.5, and until this
// milestone nothing in the codebase wrote Stats.Health and no player death
// existed. So the split, signed 27 Aug:
//
//  1. each meter reads its expected value after N world hours — M4.2 owns it
//     whole, and this script asserts it against the clock's own elapsed
//     minutes rather than against the number of hours it asked for;
//  2. "at fatigue ≥ 75% the combat state machine offers no Reaction" — M4.2
//     owns the BODY's half and exposes reaction_available / shaken for
//     M4.5's resolver to read, so this script asserts the flags flip at the
//     thresholds and that thirst lowers the Shaken one;
//  3. "at food = 0 health decrements per hour until death" — M4.2 owns it;
//     the death SCREEN is M4.6's (S1 §6.5, R2 §3), so the script asserts the
//     body reaches zero health and stops, and asserts nothing about a screen.
//
// And the fourth act is the rule this milestone added: a provider that
// reports a value needs a verb that can move it in BOTH directions, or a
// script can only ever watch the number fall.
func TestSurvivalMeters(t *testing.T) {
	const (
		// The signed dials (build note §4). Hardcoded on purpose: if a dial
		// moves, this script should say so.
		foodDrain    = 3.0
		waterDrain   = 4.5
		fatigueDrain = 3.5
		warnLevel    = 33.0
		noReaction   = 75.0
		shakenAt     = 90.0
		thirstyShake = 80.0
		neglect      = 2.0

		nightMinute = 1275.0 // 21:15, true dark
		drainHours  = 4.0    // stays inside the night, so no daylight thirst
		tolerance   = 0.05   // meter points, against float drift over ~6k ticks
	)

	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gaunt", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	// 0. the provider is registered, and no longer merely planned.
	systems := s.call("strigoi_list_systems", map[string]any{})
	if !hasSystem(systems, "meters") {
		t.Fatalf("the meters provider is not registered: %v", systems)
	}

	for _, p := range asList(systems["planned_not_yet_registered"]) {
		if strings.HasPrefix(str2(p), "meters") {
			t.Fatalf("meters is still listed as planned: %v", p)
		}
	}

	// --- 1. the meters drain on the world clock -------------------------
	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "moon", "value": 0})

	clock := sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	s.call("strigoi_step_world", map[string]any{
		"world_minutes": (24*60 - num(clock, "minute_of_day")) + nightMinute + 30,
	})

	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	if str(clock, "stage") != "night" {
		t.Fatalf("wanted the deep night, got stage %s at %s", str(clock, "stage"), str(clock, "time_of_day"))
	}

	// Getting to a deep night costs ~43 world hours, and at the shipped
	// dials that is long enough to empty every meter twice over — water
	// goes in 22 hours, food in 33. That is the design working (S1 §5 wants
	// eating and drinking to be a daily cost), but it means the drain
	// window has to start from a known body rather than from whatever the
	// walk to nightfall left. This is what the settable meters are for.
	setMeter(s, "food", 100)
	setMeter(s, "water", 100)

	before := setMeter(s, "fatigue", 0)
	startMinutes := num(clock, "world_minutes")

	t.Logf("at %s, topped up: food %.2f water %.2f fatigue %.2f",
		str(clock, "time_of_day"), num(before, "food"), num(before, "water"), num(before, "fatigue"))

	s.call("strigoi_step_world", map[string]any{"world_minutes": drainHours * 60})

	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	after := sub(s.call("strigoi_get_system_state", map[string]any{"system": "meters"}), "state")

	if str(clock, "stage") != "night" {
		t.Fatalf("the drain window left the night (%s) — the daylight thirst multiplier "+
			"is now in the arithmetic and these expectations are wrong", str(clock, "stage"))
	}

	hours := (num(clock, "world_minutes") - startMinutes) / 60

	for _, tc := range []struct {
		name string
		got  float64
		want float64
	}{
		{"food", num(after, "food"), num(before, "food") - foodDrain*hours},
		{"water", num(after, "water"), num(before, "water") - waterDrain*hours},
		{"fatigue", num(after, "fatigue"), num(before, "fatigue") + fatigueDrain*hours},
	} {
		if math.Abs(tc.got-tc.want) > tolerance {
			t.Fatalf("after %.4f world hours: %s reads %.4f, want %.4f", hours, tc.name, tc.got, tc.want)
		}
	}

	t.Logf("after %.2f world hours: food %.2f water %.2f fatigue %.2f — each on its dial",
		hours, num(after, "food"), num(after, "water"), num(after, "fatigue"))

	// --- 2. consuming moves them back the other way ----------------------
	fed := sub(s.call("strigoi_set_system_field", map[string]any{
		"system": "meters", "field": "consume",
		"value": map[string]any{"kind": "food", "amount": 5.0},
	}), "state")

	if want := num(after, "food") + 5; math.Abs(num(fed, "food")-want) > tolerance {
		t.Fatalf("eating 5 took food to %.3f, want %.3f", num(fed, "food"), want)
	}

	rested := sub(s.call("strigoi_set_system_field", map[string]any{
		"system": "meters", "field": "consume",
		"value": map[string]any{"kind": "rest", "amount": 12.5},
	}), "state")

	if want := num(fed, "fatigue") - 12.5; math.Abs(num(rested, "fatigue")-want) > tolerance {
		t.Fatalf("an hour's sleep took fatigue to %.3f, want %.3f", num(rested, "fatigue"), want)
	}

	t.Logf("consume moves them both ways: food %.2f -> %.2f, fatigue %.2f -> %.2f",
		num(after, "food"), num(fed, "food"), num(fed, "fatigue"), num(rested, "fatigue"))

	// --- 3. the thresholds M4.5 will read --------------------------------
	if !setMeter(s, "fatigue", noReaction-1)["reaction_available"].(bool) {
		t.Fatalf("at %.0f fatigue the Reaction is still there", noReaction-1)
	}

	gone := setMeter(s, "fatigue", noReaction)
	if gone["reaction_available"].(bool) {
		t.Fatalf("at %.0f fatigue the Reaction must be gone (S1 §5)", noReaction)
	}

	if gone["shaken"].(bool) {
		t.Fatalf("%.0f fatigue is not yet Shaken", noReaction)
	}

	if v := setMeter(s, "fatigue", shakenAt); !v["shaken"].(bool) {
		t.Fatalf("at %.0f fatigue fights start Shaken (S1 §5): %v", shakenAt, v)
	}

	// ...and thirst lowers the Shaken threshold (S1 §5's Water row).
	mid := setMeter(s, "fatigue", (thirstyShake+shakenAt)/2)
	if mid["shaken"].(bool) {
		t.Fatalf("%.0f fatigue with water in you is not Shaken", (thirstyShake+shakenAt)/2)
	}

	thirsty := setMeter(s, "water", warnLevel)
	if got := num(thirsty, "shaken_threshold"); math.Abs(got-thirstyShake) > tolerance {
		t.Fatalf("thirsty Shaken threshold %.2f, want %.2f", got, thirstyShake)
	}

	if !thirsty["shaken"].(bool) {
		t.Fatalf("the same fatigue, thirsty, starts the fight Shaken: %v", thirsty)
	}

	t.Logf("the thresholds hold: Reaction lost at %.0f, Shaken at %.0f — and at %.0f when thirsty",
		noReaction, shakenAt, thirstyShake)

	// --- 4. death by neglect, as far as M4.2 owns it ---------------------
	// Last, because it is the one act this session cannot undo.
	empty := setMeter(s, "water", 0)
	empty = setMeter(s, "food", 0)

	if !empty["dying"].(bool) {
		t.Fatalf("two empty meters is dying: %v", empty)
	}

	health0 := num(empty, "health")
	if health0 <= 0 {
		t.Fatalf("no health to spend: %v", empty)
	}

	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	deathStart := num(clock, "world_minutes")

	s.call("strigoi_step_world", map[string]any{"world_minutes": 2 * 60})

	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	hurt := sub(s.call("strigoi_get_system_state", map[string]any{"system": "meters"}), "state")

	// Two empty meters cost twice as much, and the model owes whole points
	// only, so the reading is the floor of the debt.
	spent := neglect * 2 * (num(clock, "world_minutes") - deathStart) / 60
	if want := health0 - math.Floor(spent); math.Abs(num(hurt, "health")-want) > 1 {
		t.Fatalf("after %.2f starving, parched hours: health %.0f, want about %.0f",
			(num(clock, "world_minutes")-deathStart)/60, num(hurt, "health"), want)
	}

	t.Logf("neglect is spending health: %.0f -> %.0f in %.2f world hours",
		health0, num(hurt, "health"), (num(clock, "world_minutes")-deathStart)/60)

	// Run it out. The step batch is bounded, so walk it in day-sized bites.
	dead := hurt

	for i := 0; i < 40 && !dead["dead"].(bool); i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 6 * 60})
		dead = sub(s.call("strigoi_get_system_state", map[string]any{"system": "meters"}), "state")
	}

	if !dead["dead"].(bool) {
		t.Fatalf("neglect must kill: %v", dead)
	}

	if got := num(dead, "health"); got != 0 {
		t.Fatalf("dead at health %.0f, want 0", got)
	}

	// The player entity reports the same zero — the meters and the entity
	// read one field, so they cannot disagree.
	player := s.call("strigoi_get_player", map[string]any{})
	if state, ok := player["state"].(map[string]any); ok {
		if got := num(state, "health"); got != 0 {
			t.Fatalf("the meters say dead but the player reports health %.0f", got)
		}
	}

	// A dead body stops spending. (What it does NOT do is show a death
	// screen — that is M4.6's, deliberately, per the signed build note.)
	food := num(dead, "food")
	fatigue := num(dead, "fatigue")

	s.call("strigoi_step_world", map[string]any{"world_minutes": 6 * 60})

	still := sub(s.call("strigoi_get_system_state", map[string]any{"system": "meters"}), "state")
	if num(still, "food") != food || num(still, "fatigue") != fatigue {
		t.Fatalf("a dead body kept draining: food %.2f->%.2f fatigue %.2f->%.2f",
			food, num(still, "food"), fatigue, num(still, "fatigue"))
	}

	t.Logf("death by neglect: health 0, the meters stopped, and no death screen was asserted " +
		"because M4.6 owns it")

	// --- 5. no unexpected error lines (the known one is allowlisted) -----
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

// setMeter writes one meter and returns the system state afterwards.
func setMeter(s *session, field string, value float64) map[string]any {
	s.t.Helper()

	return sub(s.call("strigoi_set_system_field", map[string]any{
		"system": "meters", "field": field, "value": value,
	}), "state")
}

// hasSystem lives in ui_inventory_test.go — same package, same question.

func asList(v any) []any {
	list, _ := v.([]any)
	return list
}

func str2(v any) string {
	s, _ := v.(string)
	return s
}
