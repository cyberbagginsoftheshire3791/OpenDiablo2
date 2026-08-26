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

	run.digests[0], run.parts[0] = digest(s)

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

	// Idle under NPC behaviour: 600 stepped ticks exercise the per-entity
	// RNGs (idle repetitions, waypoint walking).
	s.call("strigoi_step", map[string]any{"frames": 600})

	run.digests[2], run.parts[2] = digest(s)

	// This process's game is done; the second launch needs the port.
	s.stop()

	return run
}

func digest(s *session) (string, map[string]any) {
	out := s.call("strigoi_get_state_digest", map[string]any{})
	return str(out, "digest"), sub(out, "parts")
}
