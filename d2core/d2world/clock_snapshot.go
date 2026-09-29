package d2world

import (
	"fmt"
	"math"
)

// M4.6 B2a: the world save's snapshot of each system with no entity ids --
// clock, light, squads, corpses, rising and combat (B2b's spawns, notice and
// pursuit have their own files and tests). Each of the six follows one
// contract, and each system's test holds it to it:
//
//  1. Snapshot then Restore into a fresh system is the same system: its
//     provider and its behaviour cannot tell the two apart.
//  2. Advanced by the same steps afterwards, the two stay the same.
//  3. Any one field of the snapshot zeroed (or dropped) and restored makes the
//     copy diverge from the original, or is refused by Restore. A field whose
//     loss nothing can see is a hole in observability or dead weight, and the
//     sweep fails on it rather than letting it through. The sweep first
//     restores the untouched snapshot the same way and requires it to match,
//     so a divergence it counts is the field's and not the harness's.
//
// Every field of every system's struct is also classified by a test (saved,
// derived on load, transient, or wiring), so a field added later cannot be
// silently left out of the save: the classification test fails until someone
// decides which it is.
//
// A snapshot is data, not the load. The ORDER things are restored in is B4's
// (plan section 5, and the one authoritative list in
// docs/m4.6-world-save-notes.md), and each Restore's doc comment names the part
// of that order it depends on.
//
// EVERY RESTORE HAS A VALIDATE TWIN (D4, 28 Sep 2026), on all nine systems --
// these six and B2b's spawns, notice and pursuit: the same checks, run without
// changing anything, so a load checks every block of the file before it
// restores any, and a refusal anywhere sets the whole file aside (rule 7)
// rather than leaving a world half restored. Restore calls Validate first, so
// the two can never disagree.

// ClockSnapshot is the clock's saved state: the world minutes since the epoch,
// which is the whole of it. The date, weekday, stage, rate and moon are
// derived from it. frozen and the moon override are the harness's settings,
// not the world's, so a resumed game does not inherit them.
type ClockSnapshot struct {
	Elapsed float64 `json:"elapsed"`
}

// Snapshot is the clock as it stands.
func (c *Clock) Snapshot() ClockSnapshot { return ClockSnapshot{Elapsed: c.elapsed} }

// Restore puts the clock at a saved moment. It is the only setter the time
// has, and it exists for the load alone: the harness still cannot set the
// time (HarnessSet refuses world_minutes), because a script that could would
// prove its own arithmetic rather than the clock's.
//
// Load order (B4, plan section 5 trap 1): restore the clock right after
// NewClock, before bindProgress seeds lastStage and dawnPaidDay from it --
// restored later, the first frame reads a stage change and pays a night.
func (c *Clock) Restore(s ClockSnapshot) error {
	if err := c.Validate(s); err != nil {
		return err
	}

	c.elapsed = s.Elapsed

	return nil
}

// Validate is Restore's check and nothing else (D4): elapsed is a number of
// world minutes, not before the epoch.
func (c *Clock) Validate(s ClockSnapshot) error {
	if math.IsNaN(s.Elapsed) || math.IsInf(s.Elapsed, 0) || s.Elapsed < 0 {
		return fmt.Errorf("clock snapshot: elapsed %v is not a number of world minutes since the epoch", s.Elapsed)
	}

	return nil
}
