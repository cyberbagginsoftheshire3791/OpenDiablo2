//go:build playtest

package playtest

import (
	"testing"
)

// TestStrigoiFirstNight: a first day and night on the DEFAULT game -- the
// village, Strigoi's hero, fonts and words -- as a player gets it. The loop
// (talk, forage, the tables, the dead) was built and proved on Diablo II's
// Act 1, where the rest of the suite still runs (-classic); this is its first
// night where it now lives. The hero is kept alive (meters topped up) so the
// night runs its whole length.
func TestStrigoiFirstNight(t *testing.T) {
	s := startWith(t) // no switches: Strigoi
	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Nightwatch", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	health := mustNum(t, metersState(s), "health")
	keepAlive := func() {
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
		setField(s, "meters", "health", health)
	}

	// The day's verbs, on the village.
	headman := villager(t, s, "The headman")
	walkNear(t, s, headman)
	openTalkWith(t, s, headman)
	s.call("strigoi_key", map[string]any{"key": "escape"})
	s.call("strigoi_step", map[string]any{"frames": 4})

	s.call("strigoi_key", map[string]any{"key": "k"})
	s.call("strigoi_step", map[string]any{"frames": 4})

	startDate := str(clockState(s), "date")
	sawNight, dayAgain := false, false

	// A day and a night, as shipped: the tables roll, the dead may come.
	for i := 0; i < 112 && !dayAgain; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 15.0})
		keepAlive()

		c := clockState(s)
		if str(c, "stage") == "night" {
			sawNight = true
		}

		dayAgain = sawNight && str(c, "stage") != "night" && str(c, "date") != startDate
	}

	s.call("strigoi_screenshot", map[string]any{"name": "strigoi-morning"})

	sp := spawnsState(s)
	t.Logf("the night on the village: %d table check(s), %d roll(s), %d spawned, %d spawn failure(s); %d notice(s); risen groups %d",
		int(num(sp, "checks")), int(num(sp, "rolls")), int(num(sp, "spawned")), int(num(sp, "spawn_failures")),
		int(num(sp, "notices")), risenGroups(s))

	if !sawNight || !dayAgain {
		t.Fatalf("the night never came and went: night seen %v, next day %v (%s -> %s)", sawNight, dayAgain, startDate, str(clockState(s), "date"))
	}

	if num(sp, "checks") == 0 {
		t.Fatalf("the spawn tables never ran on the village: %v", sp)
	}
}
