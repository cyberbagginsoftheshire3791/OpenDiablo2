//go:build playtest

package playtest

import (
	"testing"
)

// TestTheUnaidedChain is the eighteenth script, and it exists because of what
// 19 September 2026 measured: the gate was 39/0, the whole reachability register
// was green, all seventeen scripts passed, CI was green on master -- and Josh
// launched the game and could do nothing but walk around and die.
//
// The reason none of those instruments could see it is that ALL of them tested
// the parts. Counted at a97ed5c3: 21 of the 23 monsters appearing in any
// playtest assertion were PLACED by the script, and 21 awareness relationships
// were GRANTED by strigoi_watch rather than worked out by the notice model. The
// untagged corpus tests notice against a fakeSight, pursuit against a fakeRouter
// and the spawn tables against a fakeSpawner; no untagged test anywhere
// constructs a Game, and the three adapters that join those fakes to reality --
// mapSight.Clear, mapRouter.Route, gameSpawner.Spawn -- have no untagged test at
// all. The composition of the whole chain was exercised exactly once in the
// corpus, in combat_rout_test.go, at adjacent_tiles 10 (shipped: 1) and
// notice_radius 24 (shipped: 12) with chance forced to 100. At SHIPPED dials:
// never.
//
// So this script asserts the one thing nothing asserted: that with NOTHING
// arranged, the shipped world gets from "a night begins" to "a fight happened"
// on its own. The five links are spawn -> place -> see -> route -> close, and
// every one of them fails as a silent `continue`, which is why a build that
// produces no fight looks exactly like a build that has not got lucky yet.
//
// WHAT IT MAY NOT DO, and this is the whole point: it must never call
// strigoi_spawn_entity, strigoi_watch or strigoi_pursue, and it must not touch
// spawns.chance, spawns.notice_radius or combat.adjacent_tiles. Doing any of
// those substitutes for a link and the script stops measuring the game. The one
// deviation is player_control=policy, which launcher.go sets after every
// start_game; it is downstream of all five links, and without it the first
// opened turn would freeze the clock and the run would never finish.
func TestTheUnaidedChain(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Unaided", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	// The dials this script must NOT have changed, read back and pinned. If a
	// later edit "helps" the chain along by forcing the table or widening the
	// notice radius, this fails and says why, rather than passing on a world
	// that was arranged for it.
	sp := spawnsState(s)

	if got := mustNum(t, sp, "chance"); got != 0.35 {
		t.Fatalf("this script measures the SHIPPED chain: spawns.chance is %.2f, want the shipped 0.35 -- if a dial was forced, the run proves nothing", got)
	}

	if got := mustNum(t, combatState(s), "adjacent_tiles"); got != 1 {
		t.Fatalf("combat.adjacent_tiles is %.0f, want the shipped 1 -- reach 10 is how combat_rout_test.go opens a fight, and it is not what a player gets", got)
	}

	if !flag(t, sp, "has_spawner") || !flag(t, sp, "has_target") {
		t.Fatalf("the spawn tables are not wired to a spawner and a target: %v", sp)
	}

	var (
		firstSpawn, firstAware, firstChase string
		peakAware, peakChases              float64
		encounters                         float64
	)

	// Dusk is where the rows have weight: every authored row's StageDay weight
	// is 0, so from DayStart 04:15 to DuskStart 19:45 nothing can spawn at all.
	// 100 samples of 15 world minutes carries dawn -> day -> dusk -> night ->
	// dawn, which is one full cycle plus the shoulder.
	const (
		samples   = 100
		requested = 15.0
	)

	for i := 0; i < samples; i++ {
		before := mustNum(t, clockState(s), "world_minutes")

		s.call("strigoi_step_world", map[string]any{"world_minutes": requested})

		c := clockState(s)
		after := mustNum(t, c, "world_minutes")

		// INSTRUMENT CONTROL, and it is not optional: without it "no fight" and
		// "never stepped" are the same log, which is how the winnability run
		// could have reported a night nobody walked through. The step verb
		// overshoots ~2.3%, so the floor is loose and the ceiling is generous.
		if moved := after - before; moved < requested-1 || moved > requested*2 {
			t.Fatalf("sample %d: the clock moved %.2f world minutes, not ~%.0f -- this run measures nothing",
				i, moved, requested)
		}

		at := mustStr(t, c, "time_of_day")
		sp = spawnsState(s)

		if num(sp, "spawned") > 0 && firstSpawn == "" {
			firstSpawn = at
		}

		if aware := float64(len(awareList(sp))); aware > 0 {
			if firstAware == "" {
				firstAware = at
			}

			if aware > peakAware {
				peakAware = aware
			}
		}

		pu := sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")

		if chases := mustNum(t, pu, "chases"); chases > 0 {
			if firstChase == "" {
				firstChase = at
			}

			if chases > peakChases {
				peakChases = chases
			}
		}

		if n := mustNum(t, combatState(s), "encounters"); n > encounters {
			encounters = n
		}

		// Health is on the METERS provider. Reading it off combat with fail-open
		// num() returns 0 for an absent key and fakes a death on sample 0 --
		// measured, the first time this probe ran (A3).
		if hp := mustNum(t, metersState(s), "health"); hp <= 0 {
			t.Logf("the player died at %s (sample %d); Meters.Advance returns early for a dead body, so everything after this is frozen", at, i)

			break
		}
	}

	t.Logf("unaided chain: first spawn %q · first aware %q (peak %.0f) · first chase %q (peak %.0f) · encounters %.0f",
		firstSpawn, firstAware, peakAware, firstChase, peakChases, encounters)

	// The five links, each asserted where it can actually fail. They are ordered
	// so the FIRST failure names the link that broke, instead of one assertion
	// at the end that says only "no fight".
	if firstSpawn == "" {
		t.Fatalf("LINK 1-2: the spawn tables never placed anything in a full cycle -- rolls %v, checks %v, failures %v",
			sp["rolls"], sp["checks"], sp["spawn_failures"])
	}

	if firstAware == "" {
		t.Fatalf("LINK 3: %v members were placed and NOTHING ever noticed the player. The notice model or the raycast behind it is refusing every ray -- this is the shape BUG-9 and BUG-8 live in, and it is invisible to every other test in this repo because they all call strigoi_watch instead",
			sp["spawned"])
	}

	if firstChase == "" {
		t.Fatalf("LINK 4: something noticed the player at %s and no chase ever started -- startChasesForTheAware or the router is dropping every aware pair", firstAware)
	}

	if encounters == 0 {
		t.Fatalf("LINK 5: %.0f hunters noticed and chased and not one ever got within reach. tryStart's declines are the place to look; at adjacent_tiles 1 a pack that arrives strung out arrives one at a time",
			peakChases)
	}
}
