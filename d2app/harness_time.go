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
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// harnessMinuteEpsilon is the tolerance on "did the clock advance as far as I
// asked". A world minute arrives as a sum of per-tick slices, so it lands a
// few ulps short; without this an honest full step reads as a stall.
const harnessMinuteEpsilon = 1e-9

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
	WorldMin   float64 `json:"world_minutes,omitempty" jsonschema:"world minutes to advance; steps until the clock provider reports the target (needs a running game)"`
}

type harnessStepOut struct {
	Ticks        int     `json:"ticks"`
	SimSeconds   float64 `json:"sim_seconds"`
	WorldMinutes float64 `json:"world_minutes,omitempty"`
	Digest       string  `json:"digest"`
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
		Description: "The simulation clock: live (wall clock) or paused (frozen; advanced only by strigoi_step, strigoi_step_world and a held strigoi_click). sim_seconds counts stepped time only.",
		Annotations: harnessAnnRO(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessTimeModeOut, error) {
		harnessLogCall("strigoi_get_time_mode")
		out := harnessTimeSnapshot()

		return harnessText("mode=%s dt=%s sim_seconds=%s", out.Mode, harnessFmtFloat(out.DT), harnessFmtFloat(out.SimSeconds)), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_pause",
		Description: "Freeze the simulation clock: frames keep rendering, deltas are zero, and time moves only by strigoi_step, strigoi_step_world or a held strigoi_click (one dt tick per held frame). Idempotent.",
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
		Description: "Advance by simulated seconds, or by world_minutes — which steps until the world clock reports the target (the clock's own compression decides how many ticks that takes, and it differs between day and night).",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessStepWorldIn) (*mcp.CallToolResult, harnessStepOut, error) {
		harnessLogCall("strigoi_step_world")

		var out harnessStepOut

		if in.WorldMin > 0 {
			return a.harnessStepWorldMinutes(in.WorldMin)
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

// harnessStepWorldMinutes advances the simulation until the world clock says
// the requested number of world minutes has passed (P3 spec §3.4). It never
// sets the clock — it steps it, so what a script asserts about a date is the
// clock's arithmetic and not the test's (§4.5). The compression differs
// between day and night, so each batch is sized at the clock's FASTEST rate
// (max_rate) and the remainder re-measured after it: a batch can only fall
// short, never run past the target across dawn (history item 117).
func (a *App) harnessStepWorldMinutes(worldMinutes float64) (*mcp.CallToolResult, harnessStepOut, error) {
	var out harnessStepOut

	const (
		maxTicks = 200000
		minBatch = 10
		// Aim slightly SHORT and converge from below: sized at the fastest
		// rate, each pass closes 98% of what is left by day and at least 61% by
		// night, so a 1110-minute step lands in a handful of passes and the
		// final one overshoots by at most minBatch ticks' worth.
		fudge = 0.98
	)

	dt := harnessTimeSnapshot().DT

	start, ok := harnessClockMinutes()
	if !ok {
		return nil, out, harnessErr("NOT_IMPLEMENTED",
			"no clock provider is registered — the world clock lives on the game screen (M4.1)",
			"call strigoi_start_game first, or use sim_seconds")
	}

	target := start + worldMinutes

	for out.Ticks < maxTicks {
		// A frozen clock never reaches the target: every tick advances it by
		// 0, and this loop spun to maxTicks -- past the playtest client's 60 s
		// timeout -- before saying so (history item 120). Checked every pass,
		// like the open turn below, so a clock frozen mid-step is caught too.
		frozen, err := harnessClockFrozen()
		if err != nil {
			return nil, out, err
		}

		if frozen {
			out.SimSeconds = harnessTimeSnapshot().SimSeconds

			return nil, out, harnessErr("CLOCK_FROZEN",
				fmt.Sprintf("stepped %d ticks and the world clock is frozen -- no number of ticks moves it", out.Ticks),
				"release it with strigoi_set_system_field clock.frozen=false, or step frames with strigoi_step")
		}

		// The game's own holds wedge it the same way (history item 121): the
		// escape menu, an open talk, the loadout choice. A paced fight is the
		// one hold that still moves the clock, a round's minutes at a time; its
		// open turn is AWAITING_PLAYER's, below. Anything else -- including a
		// hold added later -- is refused by name.
		held, err := harnessWorldHeldBy()
		if err != nil {
			return nil, out, err
		}

		if held != "" && held != "fight" {
			out.SimSeconds = harnessTimeSnapshot().SimSeconds

			return nil, out, harnessErr("WORLD_HELD",
				fmt.Sprintf("stepped %d ticks and the world is held by %q -- no number of ticks moves it", out.Ticks, held),
				"close what holds it (strigoi_key escape closes the menu or the journal and walks away from a talk; answer the loadout with strigoi_key 1 or 2), or step frames with strigoi_step")
		}

		// M4.4c-2a: an open player turn freezes the world clock exactly as the
		// escape menu does (Game.WorldHeldBy reads Combat.WorldHeld, which is
		// Awaiting widened to a paced fight's whole length), so a
		// step_world into a waiting turn would never converge -- it spins to
		// maxTicks while the playtest client's 60s callTimeout fires first,
		// Fatalf's the script and leaves harness.stepping true, poisoning the
		// next call (S0 item 1: the gated call had not returned at 64.76s).
		// The guard turns that wedge into a one-frame error the moment the turn
		// is open, whether it was open on entry or a fight opened mid-step.
		awaiting, err := harnessCombatAwaiting()
		if err != nil {
			return nil, out, harnessErr("INTERNAL",
				fmt.Sprintf("the AWAITING_PLAYER guard cannot read the combat provider: %v", err),
				"the `awaiting` field was renamed or changed type -- fix the provider or this guard, "+
					"because a silent false here restores the 60-second step_world wedge")
		}

		if awaiting {
			out.SimSeconds = harnessTimeSnapshot().SimSeconds

			return nil, out, harnessErr("AWAITING_PLAYER",
				fmt.Sprintf("stepped %d ticks and a player turn is now open -- the world clock is frozen with it", out.Ticks),
				"commit (strigoi_key f/l/e, or set combat.commit), or set combat.player_control=policy")
		}

		now, ok := harnessClockMinutes()
		if !ok {
			return nil, out, harnessErr("INTERNAL", "the clock provider vanished mid-step", "")
		}

		remaining := target - now
		if remaining <= 0 {
			break
		}

		rate, _ := harnessClockRate()
		if rate <= 0 {
			return nil, out, harnessErr("INTERNAL", "the clock reports a zero rate",
				"a rate is a dial (DayRate, NightRate) and is never zero in a shipped build -- check the clock's dials")
		}

		// Sized at the clock's FASTEST rate, not the one in force: the rate
		// is the stage's (the night runs at 2.5 world minutes a second, dawn
		// and day at 4), and a batch sized at the night's rate that crossed
		// dawn overshot the target by up to half again -- 60 minutes from
		// 02:00 stepped 67 (history item 117). At the fastest rate a batch
		// can only fall short, and the next pass covers the rest.
		maxRate, ok := harnessClockField("max_rate")
		if !ok || maxRate < rate {
			return nil, out, harnessErr("INTERNAL",
				fmt.Sprintf("the clock reports max_rate %v (ok=%v) against rate %v", maxRate, ok, rate),
				"the clock provider's max_rate was renamed or is wrong -- step_world sizes its batches by it")
		}

		batch := int(remaining/(maxRate*dt)*fudge) + 1
		if batch < minBatch {
			batch = minBatch
		}

		if out.Ticks+batch > maxTicks {
			batch = maxTicks - out.Ticks
		}

		if err := a.harnessStep(batch, dt); err != nil {
			return nil, out, err
		}

		out.Ticks += batch
	}

	end, _ := harnessClockMinutes()
	out.WorldMinutes = end - start
	out.SimSeconds = harnessTimeSnapshot().SimSeconds

	parts, err := a.harnessDigestParts()
	if err != nil {
		return nil, out, err
	}

	out.Digest = harnessTotalDigest(parts)

	// The same floating-point accumulation the resolver's roundEpsilon
	// absorbs, one level up: the clock advances a fractional slice per tick,
	// so fifteen ticks of a "one minute" step sum to 0.9999999999999998 and
	// an exact comparison calls that a stall. Found by the thirteenth
	// playtest, which steps frames before it steps minutes and so starts from
	// a sub-tick offset that earlier scripts never had.
	if out.WorldMinutes < worldMinutes-harnessMinuteEpsilon {
		return nil, out, harnessErr("TIMEOUT_LOADING",
			fmt.Sprintf("stepped %d ticks and the clock advanced only %s of %s world minutes",
				out.Ticks, harnessFmtFloat(out.WorldMinutes), harnessFmtFloat(worldMinutes)),
			"was the step batch capped?")
	}

	return harnessText("stepped %d ticks · %s world minutes · digest %s",
		out.Ticks, harnessFmtFloat(out.WorldMinutes), out.Digest[:12]), out, nil
}

// harnessClockMinutes reads world_minutes off the registered clock provider,
// on the game goroutine. ok is false when no clock is registered.
func harnessClockMinutes() (minutes float64, ok bool) {
	return harnessClockField("world_minutes")
}

// harnessClockFrozen reads the clock provider's `frozen` flag on the game
// goroutine. It reads STRICTLY, for harnessCombatAwaiting's reason: no clock,
// an absent field or one of another type is an error, never "not frozen"; and
// a game that is not ticking is its own error (GAME_NOT_TICKING), passed
// through rather than misread as a rename.
func harnessClockFrozen() (bool, error) {
	var (
		frozen, found, present bool
		got                    interface{}
	)

	if err := harnessOnUpdate(func() {
		p, ok := d2harness.Lookup("clock")
		if !ok {
			return
		}

		found = true
		got, present = p.HarnessState()["frozen"]
		frozen, _ = got.(bool)
	}); err != nil {
		return false, err
	}

	switch _, isBool := got.(bool); {
	case !found:
		return false, harnessErr("INTERNAL", "no clock provider is registered mid-step", "")
	case !present:
		return false, harnessErr("INTERNAL", "the clock provider does not report `frozen`",
			"the field was renamed -- fix the provider or the CLOCK_FROZEN guard")
	case !isBool:
		return false, harnessErr("INTERNAL", fmt.Sprintf("the clock provider reports `frozen` as %T, not a bool", got),
			"the field changed type -- fix the provider or the CLOCK_FROZEN guard")
	}

	return frozen, nil
}

// harnessWorldHeldBy reads the ui provider's world_held_by (Game.WorldHeldBy)
// on the game goroutine, strictly: no ui provider, an absent or non-string
// field, or "unknown" (the game screen never attached) is an error, never a
// running world.
func harnessWorldHeldBy() (string, error) {
	var (
		found, present bool
		got            interface{}
	)

	if err := harnessOnUpdate(func() {
		p, ok := d2harness.Lookup("ui")
		if !ok {
			return
		}

		found = true
		got, present = p.HarnessState()["world_held_by"]
	}); err != nil {
		return "", err
	}

	held, isString := got.(string)

	switch {
	case !found:
		return "", harnessErr("INTERNAL", "no ui provider is registered mid-step", "")
	case !present:
		return "", harnessErr("INTERNAL", "the ui provider does not report `world_held_by`",
			"the field was renamed -- fix the provider or the WORLD_HELD guard")
	case !isString:
		return "", harnessErr("INTERNAL", fmt.Sprintf("the ui provider reports `world_held_by` as %T, not a string", got),
			"the field changed type -- fix the provider or the WORLD_HELD guard")
	case held == "unknown":
		return "", harnessErr("INTERNAL", "the game screen has not told the controls what holds the world",
			"GameControls.SetWorldHolder was not called -- a shipped game calls it in Game.OnLoad")
	}

	return held, nil
}

// harnessClockRate reads the clock's current compression (world minutes per
// simulated second).
func harnessClockRate() (rate float64, ok bool) {
	return harnessClockField("rate")
}

// harnessCombatAwaiting reports whether the combat provider has an open player
// turn (M4.4c-2a). It reads through Lookup rather than a typed accessor for the
// same reason the clock helpers do: the harness package must not import
// d2world, and the provider registry is the seam that keeps it out. False when
// no combat provider is registered, so a world with no fight steps freely.
// It reads the field STRICTLY: a registered combat provider that does not
// report `awaiting`, or reports it as something other than a bool, is a rename
// or a type change, and returning false there would silently restore the
// 60-second wedge this guard exists to remove -- the absent-reads-as-zero trap
// (A3) one layer up from mustNum. No provider at all is the honest false.
func harnessCombatAwaiting() (awaiting bool, err error) {
	_ = harnessOnUpdate(func() {
		p, found := d2harness.Lookup("combat")
		if !found {
			return
		}

		v, present := p.HarnessState()["awaiting"]
		if !present {
			err = errors.New("the combat provider reports no `awaiting` field")

			return
		}

		b, isBool := v.(bool)
		if !isBool {
			err = fmt.Errorf("the combat provider's `awaiting` is %T, not a bool", v)

			return
		}

		awaiting = b
	})

	return awaiting, err
}

func harnessClockField(field string) (value float64, ok bool) {
	_ = harnessOnUpdate(func() {
		p, found := d2harness.Lookup("clock")
		if !found {
			return
		}

		v, present := p.HarnessState()[field]
		if !present {
			return
		}

		f, isFloat := v.(float64)
		if !isFloat {
			return
		}

		value, ok = f, true
	})

	return value, ok
}
