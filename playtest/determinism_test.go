//go:build playtest

package playtest

import (
	"math"
	"testing"
)

// checkpoints of one seeded, stepped run of the town walk (P3 spec §5.2).
type determinismRun struct {
	spawnX, spawnY float64
	direction      [2]float64
	digests        [3]string // A: after load · B: after the walk · C: after 600 idle ticks
	parts          [3]map[string]any

	// M4.6 B1 review (C2): the world stream's count and the uuid stream's
	// bytes at each checkpoint, as numbers -- the digest compares them only as
	// hashes, and a mismatch there names the part, not the count.
	worldDraws [3]float64
	uuidBytes  [3]float64
}

// TestTownWalkDeterministic is the M3.3 proof (P3 spec §3.3, §5.2): the same
// script and seed, in two SEPARATE PROCESS LAUNCHES, must produce identical
// state digests at three checkpoints. The clock is paused before the world is
// created, so zero simulated time passes outside the explicit steps; the map,
// the world RNG, per-NPC behaviour seeds, and entity IDs all derive from the
// seed. A mismatch names its part (sim/world/entities/rng/systems) — that
// name goes straight into docs/harness.md's leak register.
func TestTownWalkDeterministic(t *testing.T) {
	const seed = 1462

	first := deterministicRun(t, seed)
	second := deterministicRun(t, seed)

	if first.spawnX != second.spawnX || first.spawnY != second.spawnY {
		t.Fatalf("seeded map generation diverged: spawn %.4f,%.4f vs %.4f,%.4f",
			first.spawnX, first.spawnY, second.spawnX, second.spawnY)
	}

	if first.direction != second.direction {
		t.Fatalf("the adaptive walk chose different directions (%v vs %v) — the map itself diverged",
			first.direction, second.direction)
	}

	names := [3]string{"A after load", "B after walk", "C after 600 idle ticks"}

	// M4.6 B1 review (C2): the world stream's count, compared across the two
	// launches as a NUMBER at each checkpoint. Independent of act 6c's check,
	// which reads the count twice in one process: here two processes that took
	// the same steps must have drawn the same number of values, and a count
	// that moves with anything but the simulation (a wall-clock consumer, a
	// presentation draw on the world stream) reads differently. The count's
	// agreement with the stream itself is d2mapengine's unit tests' job.
	for i := 0; i < 3; i++ {
		if first.worldDraws[i] != second.worldDraws[i] || first.uuidBytes[i] != second.uuidBytes[i] {
			t.Fatalf("checkpoint %s: world draws %.0f vs %.0f, uuid bytes %.0f vs %.0f -- the same steps drew "+
				"different amounts", names[i], first.worldDraws[i], second.worldDraws[i],
				first.uuidBytes[i], second.uuidBytes[i])
		}

		t.Logf("checkpoint %s: %.0f world draw(s), %.0f uuid byte(s) in both launches",
			names[i], first.worldDraws[i], first.uuidBytes[i])
	}

	for i := 0; i < 3; i++ {
		if first.digests[i] == second.digests[i] {
			t.Logf("checkpoint %s: digests match (%s…)", names[i], first.digests[i][:12])
			continue
		}

		// Name the leaking part for the register.
		for part, sum1 := range first.parts[i] {
			if sum2, ok := second.parts[i][part]; ok && sum1 != sum2 {
				t.Errorf("checkpoint %s: part %q diverged", names[i], part)
			}
		}

		t.Fatalf("checkpoint %s: digest mismatch %s vs %s — record the diverging part(s) above in docs/harness.md's leak register",
			names[i], first.digests[i][:12], second.digests[i][:12])
	}
}

func deterministicRun(t *testing.T, seed int64) determinismRun {
	t.Helper()

	s := start(t)

	var run determinismRun

	// Freeze the clock BEFORE the world exists: loading still progresses
	// (screen transitions are not time-driven) but zero simulated time
	// passes, so checkpoint A is byte-identical across launches.
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name":    "Determ",
		"hero_class":   "amazon",
		"seed":         seed,
		"wait_seconds": 90,
	})

	if got := int64(num(game, "seed")); got != seed {
		t.Fatalf("start_game applied seed %d, want %d", got, seed)
	}

	run.spawnX, run.spawnY = pair(game, "spawn_tile")

	// M4.6 B1 review (B1): the fresh-launch path, pinned. start_game seeded
	// the uuid stream FOR THIS GAME, from byte 0; the provider says so, and
	// says the seed exactly. ("load last save" does neither -- it continues
	// the dead game's stream -- and that is B4b's, docs/m4.6-world-save-notes.md.)
	uuidState := sub(s.call("strigoi_get_system_state", map[string]any{"system": "uuid"}), "state")
	if !flag(t, uuidState, "seeded_for_this_game") || mustNum(t, uuidState, "bytes_at_game_start") != 0 ||
		mustStr(t, uuidState, "seed_str") != "1462" {
		t.Fatalf("a seeded start_game seeds the uuid stream for its own game from byte 0: %v", uuidState)
	}

	if b := mustNum(t, uuidState, "bytes"); b <= 0 || math.Mod(b, 16) != 0 {
		t.Fatalf("the new game's ids are whole v4 uuids read from the stream (16 bytes each): %.0f byte(s)", b)
	}

	run.digests[0], run.parts[0] = digest(s)
	run.worldDraws[0], run.uuidBytes[0] = streamCounts(t, s)

	// The walk: adaptive direction (the seeded map is fixed, so both runs
	// choose the same one — asserted by the caller), stepped, never wall-clock.
	moved := false

	for _, d := range [][2]float64{{6, 0}, {-6, 0}, {0, 6}, {0, -6}} {
		p := s.call("strigoi_get_player", map[string]any{})
		fromX, fromY := num(p, "x"), num(p, "y")

		res := s.call("strigoi_move_player_to", map[string]any{
			"x": fromX + d[0], "y": fromY + d[1], "wait": true, "max_ticks": 900,
		})

		px, py := pair(res, "position_tile")
		if math.Hypot(px-fromX, py-fromY) >= 2 {
			run.direction = d
			moved = true

			t.Logf("walk %+v: outcome=%s at %.2f,%.2f after %v ticks", d, str(res, "outcome"), px, py, res["ticks"])

			break
		}
	}

	if !moved {
		t.Fatal("the player could not move >= 2 tiles in any cardinal direction on the seeded map")
	}

	run.digests[1], run.parts[1] = digest(s)
	run.worldDraws[1], run.uuidBytes[1] = streamCounts(t, s)

	// Idle under NPC behaviour: 600 stepped ticks exercise the per-entity
	// RNGs (idle repetitions, waypoint walking).
	s.call("strigoi_step", map[string]any{"frames": 600})

	run.digests[2], run.parts[2] = digest(s)
	run.worldDraws[2], run.uuidBytes[2] = streamCounts(t, s)

	if run.worldDraws[0] <= 0 || run.worldDraws[1] < run.worldDraws[0] || run.worldDraws[2] < run.worldDraws[1] {
		t.Fatalf("the world stream is drawn by the map and never runs backwards: %v", run.worldDraws)
	}

	// This process's game is done; the second launch needs the port.
	s.stop()

	return run
}

// streamCounts reads the world stream's draw count (scene.world_rng, seeded
// 1462) and the uuid stream's byte count.
func streamCounts(t *testing.T, s *session) (worldDraws, uuidBytes float64) {
	t.Helper()

	scene := sub(s.call("strigoi_get_system_state", map[string]any{"system": "scene"}), "state")
	world := saveBlock(t, scene, "world_rng")

	if mustStr(t, world, "seed_str") != "1462" {
		t.Fatalf("the world stream is seeded 1462: %v", world)
	}

	uuidState := sub(s.call("strigoi_get_system_state", map[string]any{"system": "uuid"}), "state")

	return mustNum(t, world, "draws"), mustNum(t, uuidState, "bytes")
}

func digest(s *session) (string, map[string]any) {
	out := s.call("strigoi_get_state_digest", map[string]any{})
	return str(out, "digest"), sub(out, "parts")
}
