//go:build playtest

package playtest

import (
	"math"
	"strings"
	"testing"
)

// TestClockHUD is M4.4a's playtest script (Constitution VI.2): it asserts what
// the player READS on the always-visible clock strip, through the "ui" harness
// provider, rather than trusting that the HUD wired the clock. The strip shows
// the Julian date with weekday, the Orthodox feast/fast name, time-to-sunset in
// world hours to one decimal, and the moon as phase-name text (S1 §3.4; ruled
// 11 Sep, meters HELD to M4.4c).
//
// The clock opens frozen at the dawn epoch (02:45, 17 June 1462), so the
// number-vs-number check has a known answer: time-to-sunset is (19:45 - 02:45)
// = exactly 17.0 world hours to the clock's DuskStart. The test then steps a
// chosen span and asserts the readout FELL by that span (measured from the
// clock's own minute delta, because strigoi_step_world overshoots ~2.3% and an
// accumulated float must never be compared exactly), and finally crosses a
// midnight to prove the generated day table tracks the date.
func TestClockHUD(t *testing.T) {
	// The dials this asserts against (d2core/d2world clock.go). If the build
	// changes them, this list changes with it -- the point of a [DIAL].
	const (
		duskStartMinute = 19*60 + 45 // 19:45, the clock's DuskStart
		dawnMinute      = 2*60 + 45  // 02:45, the epoch
		epochToDuskH    = float64(duskStartMinute-dawnMinute) / 60.0
		minutesPerDay   = 24 * 60
		// The readout is world hours to one decimal, and the strip's stored
		// value trails the clock read by up to a fraction of a world minute,
		// so allow a tenth of an hour on the units cross-checks.
		hourSlack = 0.15
	)

	expectHoursToDusk := func(minuteOfDay float64) float64 {
		d := math.Mod(duskStartMinute-minuteOfDay, minutesPerDay)
		if d < 0 {
			d += minutesPerDay
		}

		return d / 60.0
	}

	s := start(t)

	// Born frozen (the M3.3 recipe): world time is still while the map loads,
	// so the opening strip is identical every launch.
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Eyes", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	// The controls register as the "ui" provider while a game screen lives.
	systems := s.call("strigoi_list_systems", map[string]any{})
	if !hasSystem(systems, "ui") {
		t.Fatalf("list_systems: want the ui provider, got %v", systems["systems"])
	}

	// Freeze the clock at the epoch, then step frames so HUD.Advance runs and
	// populates the strip WITHOUT the world drifting off 02:45. The born-frozen
	// state ends when start_game returns, so plain frames would advance the
	// clock (D7 §4 freeze; only the harness sets it in this build).
	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "frozen", "value": true})
	s.call("strigoi_step", map[string]any{"frames": 30})

	// ---- the opening strip: 17 June, the feast, the moon, and 17.0h to dusk --
	ui := sub(s.call("strigoi_get_system_state", map[string]any{"system": "ui"}), "state")
	clock := sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")

	date := mustStr(t, ui, "clock_strip_date")
	if !strings.Contains(date, "Thursday") || !strings.Contains(date, "17 June 1462") {
		t.Fatalf("strip date %q, want it to contain \"Thursday\" and \"17 June 1462\"", date)
	}

	// Cross-check the strip's weekday/date against the clock provider's own.
	if str(clock, "weekday") != "Thursday" || str(clock, "date") != "1462-06-17" {
		t.Fatalf("clock provider says %s %s, strip says %q", str(clock, "date"), str(clock, "weekday"), date)
	}

	if feast := mustStr(t, ui, "clock_strip_feast"); feast != "9th Thursday after Easter" {
		t.Fatalf("opening feast %q, want %q (C3 §3.6, 17 Jun)", feast, "9th Thursday after Easter")
	}

	if moon := mustStr(t, ui, "clock_strip_moon"); moon != "waning gibbous" {
		t.Fatalf("opening moon %q, want %q (last quarter is 20 Jun)", moon, "waning gibbous")
	}

	// The full strip line the player actually reads carries the sunset readout.
	if text := mustStr(t, ui, "clock_strip_text"); !strings.Contains(text, "Sunset in 17.0h") {
		t.Fatalf("strip text %q, want it to contain \"Sunset in 17.0h\"", text)
	}

	// NUMBER-VS-NUMBER (the UNITS guard): the test's own 17.0h -- computed from
	// the known epoch and DuskStart, not read from the system -- against the
	// number the strip reports at the frozen epoch.
	m0 := num(clock, "minute_of_day")
	h0 := mustNum(t, ui, "clock_strip_hours_to_dusk")

	if math.Abs(m0-dawnMinute) > 1 {
		t.Fatalf("the world did not open at the dawn epoch: minute_of_day %v, want ~%v", m0, dawnMinute)
	}

	if math.Abs(h0-epochToDuskH) > 0.05 {
		t.Fatalf("opening time-to-sunset %.3fh, want %.1fh (19:45 - 02:45)", h0, epochToDuskH)
	}

	if math.Abs(h0-expectHoursToDusk(m0)) > hourSlack {
		t.Fatalf("strip hours-to-dusk %.3f disagrees with the clock's minute %v (want %.3f)",
			h0, m0, expectHoursToDusk(m0))
	}

	// ---- step a chosen span and assert the readout FELL by that span ----
	// Unfreeze first: a frozen clock ignores the step (worldClock.Advance
	// returns 0), the same rule night_light §11 turns on.
	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "frozen", "value": false})

	const stepMinutes = 5 * 60 // the test chooses to advance five world hours
	s.call("strigoi_step_world", map[string]any{"world_minutes": stepMinutes})

	ui = sub(s.call("strigoi_get_system_state", map[string]any{"system": "ui"}), "state")
	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")

	m1 := num(clock, "minute_of_day")
	h1 := mustNum(t, ui, "clock_strip_hours_to_dusk")

	if h1 >= h0 {
		t.Fatalf("time-to-sunset did not fall toward dusk: %.2fh -> %.2fh", h0, h1)
	}

	// The clock's own minute delta is the ACTUAL advance (overshoot included);
	// the readout must have dropped by exactly that, within the display's slack.
	actualAdvanceH := (m1 - m0) / 60.0
	if math.Abs((h0-h1)-actualAdvanceH) > hourSlack {
		t.Fatalf("readout fell %.3fh but the clock advanced %.3fh", h0-h1, actualAdvanceH)
	}

	if math.Abs(h1-expectHoursToDusk(m1)) > hourSlack {
		t.Fatalf("after stepping: strip hours-to-dusk %.3f disagrees with minute %v (want %.3f)",
			h1, m1, expectHoursToDusk(m1))
	}

	// Still 17 June (five hours from 02:45 is ~07:52), so the feast is unchanged.
	if feast := mustStr(t, ui, "clock_strip_feast"); feast != "9th Thursday after Easter" {
		t.Fatalf("five hours in, feast %q, want it still 17 Jun's", feast)
	}

	// ---- cross a midnight: the generated day table must track the date ----
	// Step to ~noon on 18 June (overshoot cannot skip a whole civil day).
	toNextNoon := (float64(minutesPerDay) - m1) + 12*60
	s.call("strigoi_step_world", map[string]any{"world_minutes": toNextNoon})

	ui = sub(s.call("strigoi_get_system_state", map[string]any{"system": "ui"}), "state")
	clock = sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")

	if str(clock, "date") != "1462-06-18" || str(clock, "weekday") != "Friday" {
		t.Fatalf("a night later want Friday 18 June, clock says %s %s", str(clock, "date"), str(clock, "weekday"))
	}

	if date := mustStr(t, ui, "clock_strip_date"); !strings.Contains(date, "Friday") || !strings.Contains(date, "18 June 1462") {
		t.Fatalf("next-day strip date %q, want \"Friday\" and \"18 June 1462\"", date)
	}

	// 18 June is inside the Apostles' Fast with no movable feast, and still
	// waning gibbous -- the table lookup moved to the new row.
	if feast := mustStr(t, ui, "clock_strip_feast"); feast != "Apostles' Fast" {
		t.Fatalf("18 Jun feast %q, want %q", feast, "Apostles' Fast")
	}

	if moon := mustStr(t, ui, "clock_strip_moon"); moon != "waning gibbous" {
		t.Fatalf("18 Jun moon %q, want %q", moon, "waning gibbous")
	}

	// No unexpected error lines (the known frame-index warning is allowlisted).
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
