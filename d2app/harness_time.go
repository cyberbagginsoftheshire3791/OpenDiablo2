//go:build harness

package d2app

// Phase 3 playtest harness — M3.3 determinism (P3 spec §3.3–§3.6, §4.2):
// the paused/stepped clock, the seed plumbing, and the state digest.
//
// The model: the simulation is either "live" (the wall clock drives
// advanceOnce, exactly as before the harness existed) or "paused" (every
// ebiten frame runs advanceOnce with zero deltas and a frozen clock).
// Stepping happens only inside queued harness commands, which call
// advanceOnce directly with a fixed dt — the wall clock is never involved,
// so the same script and seed replay the same simulation.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

const harnessStepBatch = 600 // ticks executed per queued command [DIAL] P3 §3.4

// harnessStepDeltas reports whether the harness holds the clock. While
// paused, frames advance with zero deltas and the frozen simNow. Runs on the
// game goroutine (called from advance).
func (a *App) harnessStepDeltas() (elapsedUnscaled, elapsed, elapsedScreen, current float64, held bool) {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	if harness.timeMode != "paused" {
		return 0, 0, 0, 0, false
	}

	return 0, 0, 0, harness.simNow, true
}

// harnessPauseOnGoroutine freezes the clock. Must run on the game goroutine.
func (a *App) harnessPauseOnGoroutine() {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	if harness.timeMode == "paused" {
		return
	}

	harness.timeMode = "paused"

	if harness.simNow == 0 {
		harness.simNow = d2util.Now()
	}
}

// harnessResumeOnGoroutine hands the clock back to the wall, resetting the
// live timestamps so the paused span does not arrive as one giant delta.
func (a *App) harnessResumeOnGoroutine() {
	harness.mu.Lock()
	harness.timeMode = "live"
	harness.mu.Unlock()

	now := d2util.Now()
	a.lastTime = now
	a.lastScreenAdvance = now
}

// harnessStepTicks runs n fixed-dt simulation ticks directly. Must run on the
// game goroutine; the caller batches to keep single frames responsive.
func (a *App) harnessStepTicks(n int, dt float64) error {
	for i := 0; i < n; i++ {
		harness.mu.Lock()
		harness.simNow += dt
		harness.simSeconds += dt
		now := harness.simNow
		harness.mu.Unlock()

		if err := a.advanceOnce(dt, dt, dt, now); err != nil {
			return err
		}
	}

	return nil
}

// harnessStep pauses if needed and advances exactly frames ticks in batches.
func (a *App) harnessStep(frames int, dt float64) error {
	harness.mu.Lock()
	if harness.stepping {
		harness.mu.Unlock()
		return harnessErr("BAD_ARGUMENT", "a step is already executing", "wait for it to return")
	}

	harness.stepping = true
	harness.timeDT = dt
	harness.mu.Unlock()

	defer func() {
		harness.mu.Lock()
		harness.stepping = false
		harness.mu.Unlock()
	}()

	if err := harnessOnUpdate(func() { a.harnessPauseOnGoroutine() }); err != nil {
		return err
	}

	remaining := frames

	for remaining > 0 {
		batch := remaining
		if batch > harnessStepBatch {
			batch = harnessStepBatch
		}

		var stepErr error

		if err := harnessOnUpdate(func() { stepErr = a.harnessStepTicks(batch, dt) }); err != nil {
			return err
		}

		if stepErr != nil {
			return harnessErr("INTERNAL", fmt.Sprintf("advance failed mid-step: %v", stepErr), "")
		}

		remaining -= batch
	}

	return nil
}

// ------------------------------------------------------------------ digest --

func harnessFmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// harnessDigestParts builds the per-part canonical strings on the game
// goroutine (P3 spec §3.6). Pixels, animation frames, audio, log text, and
// the raw frame tick are deliberately excluded: parts must be comparable
// across process launches, and boot-frame counts differ per launch.
func (a *App) harnessDigestParts() (map[string]string, error) {
	raw := map[string]string{}

	var buildErr error

	err := harnessOnUpdate(func() {
		harness.mu.Lock()
		raw["sim"] = fmt.Sprintf("mode=%s dt=%s sim_seconds=%s",
			harness.timeMode, harnessFmtFloat(harness.timeDT), harnessFmtFloat(harness.simSeconds))
		seed := harness.currentSeed
		harness.mu.Unlock()

		client, _ := harnessGame()
		if client == nil {
			raw["world"] = "no-game"
			raw["entities"] = ""
			raw["rng"] = ""
		} else {
			entities := client.MapEngine.Entities()
			raw["world"] = fmt.Sprintf("seed=%d start_seed=%d entities=%d", client.Seed, seed, len(entities))
			raw["rng"] = fmt.Sprintf("world_draws=%d", client.MapEngine.RandDraws())

			ids := make([]string, 0, len(entities))
			for id := range entities {
				ids = append(ids, id)
			}

			sort.Strings(ids)

			infos := make([]harnessEntityInfo, 0, len(ids))
			for _, id := range ids {
				infos = append(infos, harnessEntityInfoFor(id, entities[id], client.PlayerID, true))
			}

			sort.Slice(infos, func(i, j int) bool { return harnessHandleLess(infos[i].Handle, infos[j].Handle) })

			var canon []byte

			for i := range infos {
				line, err := json.Marshal(infos[i])
				if err != nil {
					buildErr = err
					return
				}

				canon = append(canon, line...)
				canon = append(canon, '\n')
			}

			raw["entities"] = string(canon)
		}

		var sys []byte

		providers := d2harness.Providers()
		names := make([]string, 0, len(providers))
		byName := map[string]d2harness.Provider{}

		for _, p := range providers {
			names = append(names, p.HarnessName())
			byName[p.HarnessName()] = p // duplicates: newest wins, matching Lookup
		}

		sort.Strings(names)

		seen := map[string]bool{}

		for _, n := range names {
			if seen[n] {
				continue
			}

			seen[n] = true

			state, err := json.Marshal(byName[n].HarnessState())
			if err != nil {
				buildErr = err
				return
			}

			sys = append(sys, []byte(n+"=")...)
			sys = append(sys, state...)
			sys = append(sys, '\n')
		}

		raw["systems"] = string(sys)
	})
	if err != nil {
		return nil, err
	}

	if buildErr != nil {
		return nil, harnessErr("INTERNAL", fmt.Sprintf("digest: %v", buildErr), "")
	}

	parts := make(map[string]string, len(raw))

	for name, content := range raw {
		sum := sha256.Sum256([]byte(content))
		parts[name] = hex.EncodeToString(sum[:])
	}

	return parts, nil
}

func harnessTotalDigest(parts map[string]string) string {
	names := make([]string, 0, len(parts))
	for n := range parts {
		names = append(names, n)
	}

	sort.Strings(names)

	h := sha256.New()

	for _, n := range names {
		h.Write([]byte(n))
		h.Write([]byte(parts[n]))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// ------------------------------------------------------------------- tools --

type harnessTimeModeOut struct {
	Mode       string  `json:"mode"`
	DT         float64 `json:"dt"`
	SimSeconds float64 `json:"sim_seconds"`
	Tick       int64   `json:"tick"`
	Stepping   bool    `json:"stepping"`
}

type harnessStepIn struct {
	Frames int     `json:"frames" jsonschema:"fixed ticks to advance, 1..100000"`
	DT     float64 `json:"dt,omitempty" jsonschema:"seconds per tick; default 1/60"`
}

type harnessStepWorldIn struct {
	SimSeconds float64 `json:"sim_seconds,omitempty" jsonschema:"simulated seconds to advance (converted to ticks at dt)"`
	WorldMin   float64 `json:"world_minutes,omitempty" jsonschema:"NOT_IMPLEMENTED until the M4.4 clock provider exists"`
}

type harnessStepOut struct {
	Ticks      int     `json:"ticks"`
	SimSeconds float64 `json:"sim_seconds"`
	Digest     string  `json:"digest"`
}

type harnessSeedIn struct {
	Seed int64 `json:"seed" jsonschema:"nonzero seed value"`
}

type harnessSeedOut struct {
	Seed int64 `json:"seed"`
}

type harnessDigestOut struct {
	Digest     string            `json:"digest"`
	Parts      map[string]string `json:"parts"`
	SimSeconds float64           `json:"sim_seconds"`
	Seed       int64             `json:"seed"`
}

func harnessTimeSnapshot() harnessTimeModeOut {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	return harnessTimeModeOut{
		Mode:       harness.timeMode,
		DT:         harness.timeDT,
		SimSeconds: harness.simSeconds,
		Tick:       atomic.LoadInt64(&harness.tick),
		Stepping:   harness.stepping,
	}
}

func (a *App) harnessAddTimeTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_get_time_mode",
		Description: "The simulation clock: live (wall clock) or paused (frozen; advanced only by strigoi_step). sim_seconds counts stepped time only.",
		Annotations: harnessAnnRO(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessTimeModeOut, error) {
		harnessLogCall("strigoi_get_time_mode")
		out := harnessTimeSnapshot()

		return harnessText("mode=%s dt=%s sim_seconds=%s", out.Mode, harnessFmtFloat(out.DT), harnessFmtFloat(out.SimSeconds)), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_pause",
		Description: "Freeze the simulation clock: frames keep rendering, deltas are zero, strigoi_step is the only way time moves. Idempotent.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: harnessBoolPtr(false), DestructiveHint: harnessBoolPtr(false)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessTimeModeOut, error) {
		harnessLogCall("strigoi_pause")

		if err := harnessOnUpdate(func() { a.harnessPauseOnGoroutine() }); err != nil {
			return nil, harnessTimeModeOut{}, err
		}

		out := harnessTimeSnapshot()

		return harnessText("paused at sim_seconds=%s", harnessFmtFloat(out.SimSeconds)), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_resume",
		Description: "Hand the simulation clock back to the wall clock (live mode). Idempotent.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: harnessBoolPtr(false), DestructiveHint: harnessBoolPtr(false)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessTimeModeOut, error) {
		harnessLogCall("strigoi_resume")

		if err := harnessOnUpdate(func() { a.harnessResumeOnGoroutine() }); err != nil {
			return nil, harnessTimeModeOut{}, err
		}

		out := harnessTimeSnapshot()

		return harnessText("live"), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_step",
		Description: "THE determinism primitive: advance exactly N fixed-dt simulation ticks (pausing first if live), then report the state digest. Runs up to 600 ticks per frame, so long steps stay responsive.",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessStepIn) (*mcp.CallToolResult, harnessStepOut, error) {
		harnessLogCall("strigoi_step")

		var out harnessStepOut

		if in.Frames < 1 || in.Frames > 100000 {
			return nil, out, harnessErr("BAD_ARGUMENT", "frames must be 1..100000", "")
		}

		dt := in.DT
		if dt <= 0 {
			dt = harnessTimeSnapshot().DT
		}

		if err := a.harnessStep(in.Frames, dt); err != nil {
			return nil, out, err
		}

		parts, err := a.harnessDigestParts()
		if err != nil {
			return nil, out, err
		}

		out.Ticks = in.Frames
		out.SimSeconds = harnessTimeSnapshot().SimSeconds
		out.Digest = harnessTotalDigest(parts)

		return harnessText("stepped %d ticks · sim_seconds=%s · digest %s", out.Ticks, harnessFmtFloat(out.SimSeconds), out.Digest[:12]), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_step_world",
		Description: "Advance by simulated seconds (converted to ticks at dt). world_minutes waits for the M4.4 clock provider.",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessStepWorldIn) (*mcp.CallToolResult, harnessStepOut, error) {
		harnessLogCall("strigoi_step_world")

		var out harnessStepOut

		if in.WorldMin > 0 {
			return nil, out, harnessErr("NOT_IMPLEMENTED", "world_minutes needs the clock provider (arrives at M4.4)", "use sim_seconds for now")
		}

		if in.SimSeconds <= 0 {
			return nil, out, harnessErr("BAD_ARGUMENT", "sim_seconds must be > 0", "")
		}

		dt := harnessTimeSnapshot().DT
		frames := int(in.SimSeconds/dt + 0.5)

		if frames < 1 {
			frames = 1
		}

		if frames > 100000 {
			return nil, out, harnessErr("BAD_ARGUMENT", fmt.Sprintf("%s sim seconds is %d ticks; the cap is 100000 per call", harnessFmtFloat(in.SimSeconds), frames), "")
		}

		if err := a.harnessStep(frames, dt); err != nil {
			return nil, out, err
		}

		parts, err := a.harnessDigestParts()
		if err != nil {
			return nil, out, err
		}

		out.Ticks = frames
		out.SimSeconds = harnessTimeSnapshot().SimSeconds
		out.Digest = harnessTotalDigest(parts)

		return harnessText("stepped %d ticks (%s sim s) · digest %s", frames, harnessFmtFloat(in.SimSeconds), out.Digest[:12]), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_set_seed",
		Description: "Seed for the NEXT strigoi_start_game (one-shot; start_game's own seed parameter overrides it). Zero clears.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: harnessBoolPtr(false), DestructiveHint: harnessBoolPtr(false)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessSeedIn) (*mcp.CallToolResult, harnessSeedOut, error) {
		harnessLogCall("strigoi_set_seed")

		harness.mu.Lock()
		harness.pendingSeed = in.Seed
		harness.mu.Unlock()

		return harnessText("pending seed %d", in.Seed), harnessSeedOut{Seed: in.Seed}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_reseed_world",
		Description: "Reseed the world RNG mid-game without regenerating the map — for repeated-roll tests (spawn tables, rise chances).",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessSeedIn) (*mcp.CallToolResult, harnessSeedOut, error) {
		harnessLogCall("strigoi_reseed_world")

		var toolErr error

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			if client == nil {
				toolErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			client.MapEngine.ReseedRand(in.Seed)
		})
		if err != nil {
			return nil, harnessSeedOut{}, err
		}

		if toolErr != nil {
			return nil, harnessSeedOut{}, toolErr
		}

		return harnessText("world RNG reseeded to %d", in.Seed), harnessSeedOut{Seed: in.Seed}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_get_state_digest",
		Description: "SHA-256 of the canonical simulation state, with per-part digests (sim, world, entities, rng, systems) so a mismatch points at the leaking part. Excludes pixels, animation, audio, logs, and raw frame ticks (P3 spec §3.6). Comparable across process launches when seeded and stepped.",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessDigestOut, error) {
		harnessLogCall("strigoi_get_state_digest")

		var out harnessDigestOut

		parts, err := a.harnessDigestParts()
		if err != nil {
			return nil, out, err
		}

		snap := harnessTimeSnapshot()
		out.Parts = parts
		out.Digest = harnessTotalDigest(parts)
		out.SimSeconds = snap.SimSeconds

		harness.mu.Lock()
		out.Seed = harness.currentSeed
		harness.mu.Unlock()

		return harnessText("digest %s · sim_seconds=%s", out.Digest[:12], harnessFmtFloat(out.SimSeconds)), out, nil
	})
}

// harnessConsumeStartSeed resolves the seed for a start_game call: an
// explicit parameter wins, else a pending set_seed, else unseeded. Clears the
// pending value either way.
func harnessConsumeStartSeed(explicit *int64) int64 {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	seed := harness.pendingSeed
	harness.pendingSeed = 0

	if explicit != nil && *explicit != 0 {
		seed = *explicit
	}

	harness.currentSeed = seed

	return seed
}

// harnessWaitDeadline is a small helper for live-mode waits.
func harnessSleep(d time.Duration) { time.Sleep(d) }
