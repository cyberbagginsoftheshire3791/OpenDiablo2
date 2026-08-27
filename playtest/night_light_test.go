//go:build playtest

package playtest

import (
	"math"
	"strings"
	"testing"
)

// TestNightAndLight is M4.1's playtest script (Constitution VI.2). It runs
// S1 §4's own assertion, verbatim, through the harness:
//
//	at deep night with no light source and a new moon, the visible radius
//	equals the floor; lighting a torch restores radius R and decrements burn
//	by 1 per world minute; the torch is extinguished at 0 and the radius
//	returns to the floor.
//
// It also pins the two rules M4.1 inherited: the clock is stepped and never
// set (P3 §4.5), and the world advances only on the harness's own clock.
func TestNightAndLight(t *testing.T) {
	// The dials this asserts against (d2core/d2world). If the build changes
	// them, this list changes with it — that is the point of a [DIAL].
	const (
		floorRadius = 1.5
		torchRadius = 5.0
		torchBurn   = 60.0
		dawnMinute  = 165.0  // 02:45, the epoch
		nightMinute = 1275.0 // 21:15, true dark
		// Overshoot allowance: the stepper closes the last gap in 10-tick
		// batches, so it can land a fraction of a world minute past target.
		slack = 1.0
	)

	s := start(t)

	// The world is born frozen (the M3.3 recipe), so no world time passes
	// while the map loads and the opening state is identical every launch.
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Night", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	// 1. both M4.1 systems are registered, and neither is still "planned"
	systems := s.call("strigoi_list_systems", map[string]any{})
	for _, name := range []string{"clock", "light"} {
		if !hasSystem(systems, name) {
			t.Fatalf("list_systems: %q is not registered: %v", name, systems["systems"])
		}
	}

	if planned, ok := systems["planned_not_yet_registered"].([]any); ok {
		for _, p := range planned {
			if s, _ := p.(string); strings.HasPrefix(s, "clock") || strings.HasPrefix(s, "light") {
				t.Fatalf("%q is registered but still listed as planned", s)
			}
		}
	}

	// 2. the world opens on the attested date, at dawn
	clock := sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	if str(clock, "date") != "1462-06-17" || str(clock, "weekday") != "Thursday" {
		t.Fatalf("opening date %s %s, want 1462-06-17 Thursday (R1 §5)", str(clock, "date"), str(clock, "weekday"))
	}

	if str(clock, "stage") != "dawn" || math.Abs(num(clock, "minute_of_day")-dawnMinute) > 1 {
		t.Fatalf("opening stage %s at %s, want dawn at 02:45", str(clock, "stage"), str(clock, "time_of_day"))
	}

	// 3. the clock is stepped, never set (P3 §4.5)
	if msg := s.callErr("strigoi_set_system_field", map[string]any{
		"system": "clock", "field": "world_minutes", "value": 999,
	}); !strings.Contains(msg, "FIELD_NOT_SETTABLE") {
		t.Fatalf("setting the clock must be refused, got %q", msg)
	}

	// 4. a new moon — world state, so this one IS settable
	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "moon", "value": 0})

	// 5. step to the deep night. This is the clock's own arithmetic: the
	//    stepper only advances ticks until the clock says we are there.
	stepped := s.call("strigoi_step_world", map[string]any{"world_minutes": nightMinute - dawnMinute + 30})
	t.Logf("stepped %v ticks for %v world minutes", stepped["ticks"], stepped["world_minutes"])

	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	if str(clock, "stage") != "night" {
		t.Fatalf("after stepping to %s: stage %s, want night", str(clock, "time_of_day"), str(clock, "stage"))
	}

	if got := num(clock, "moon"); got != 0 {
		t.Fatalf("moon %v, want 0 (new)", got)
	}

	if got := num(clock, "rate"); got >= 4 {
		t.Fatalf("night rate %v must be slower than the day's 4 world-min/s (D7 §6)", got)
	}

	// 6. THE ASSERTION, part 1 — no light, new moon, deep night: the floor
	light := sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state")
	if got := num(light, "radius"); math.Abs(got-floorRadius) > 1e-6 {
		t.Fatalf("deep night, no light, new moon: radius %v, want the floor %v", got, floorRadius)
	}

	if str(light, "carried_source") != "" {
		t.Fatalf("the player should carry no light yet: %v", light)
	}

	darkAmbient := num(light, "ambient")
	if darkAmbient >= 0.5 {
		t.Fatalf("the deep night is not dark: ambient %v", darkAmbient)
	}

	// 7. part 2 — lighting a torch restores radius R
	s.call("strigoi_set_system_field", map[string]any{"system": "light", "field": "carried_source", "value": "torch"})

	light = sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state")
	if got := num(light, "radius"); math.Abs(got-torchRadius) > 1e-6 {
		t.Fatalf("torch lit: radius %v, want %v", got, torchRadius)
	}

	if got := num(light, "carried_burn"); math.Abs(got-torchBurn) > 1e-6 {
		t.Fatalf("a fresh torch has %v world minutes, want %v", got, torchBurn)
	}

	// 8. part 3 — burn decrements by 1 per world minute
	s.call("strigoi_step_world", map[string]any{"world_minutes": 10})

	light = sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state")

	burn := num(light, "carried_burn")
	if want := torchBurn - 10; burn > want || burn < want-slack {
		t.Fatalf("after 10 world minutes: burn %v, want %v (±%v)", burn, want, slack)
	}

	if got := num(light, "radius"); math.Abs(got-torchRadius) > 1e-6 {
		t.Fatalf("a burning torch still lights: radius %v, want %v", got, torchRadius)
	}

	// 9. part 4 — it goes out at zero, and the radius returns to the floor
	s.call("strigoi_step_world", map[string]any{"world_minutes": torchBurn})

	light = sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state")
	if light["carried_lit"] != false || num(light, "carried_burn") != 0 {
		t.Fatalf("the torch must be out and empty: lit=%v burn=%v", light["carried_lit"], num(light, "carried_burn"))
	}

	if got := num(light, "radius"); math.Abs(got-floorRadius) > 1e-6 {
		t.Fatalf("after the torch dies: radius %v, want the floor %v", got, floorRadius)
	}

	// 10. the day comes back, and with it the light. Step to the next noon.
	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	toNoon := (24*60 - num(clock, "minute_of_day")) + 12*60
	s.call("strigoi_step_world", map[string]any{"world_minutes": toNoon})

	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
	light = sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state")

	if str(clock, "stage") != "day" {
		t.Fatalf("stepped to %s on %s: stage %s, want day", str(clock, "time_of_day"), str(clock, "date"), str(clock, "stage"))
	}

	if str(clock, "date") != "1462-06-18" || str(clock, "weekday") != "Friday" {
		t.Fatalf("a night later it should be Friday 18 June, got %s %s", str(clock, "date"), str(clock, "weekday"))
	}

	if got := num(light, "ambient"); got != 1 {
		t.Fatalf("daylight ambient %v, want 1 — the day must look exactly as it did before M4.1", got)
	}

	// 11. the hearth freeze holds the world still (D7 §4; nothing but the
	//     harness sets it in M4.1)
	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "frozen", "value": true})

	before := num(sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state"), "world_minutes")
	s.call("strigoi_step", map[string]any{"frames": 600})
	after := num(sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state"), "world_minutes")

	if after != before {
		t.Fatalf("a frozen clock moved: %v -> %v", before, after)
	}

	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "frozen", "value": false})

	// 12. no unexpected error lines (the known one is allowlisted)
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
