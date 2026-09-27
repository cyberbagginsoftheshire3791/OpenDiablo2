package d2gamescreen

import (
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// The "scene" harness provider (M4.6 B1): the game screen's own bookkeeping,
// which no world system owns and nothing reported until the world save needed
// it. The save plan's rule is observability before serialisation -- a field
// the save writes but no provider reports is a field whose loss no test can
// see -- so everything here is something the save will carry (or check), read
// the moment it exists:
//
//   - field_dead: Night 1's placed dead IN THE ORDER THEY WERE LAID. The order
//     is state, not presentation: searching the i-th of them finds the i-th
//     writing (search.go), so a resume that shuffled them would hand the
//     Sultan's paper to a different body.
//   - dawn_paid_day and last_stage: which dawn has paid its night's experience,
//     and the stage the last frame saw. Restored wrong, the first frame after
//     a load pays a night's experience again (the plan's trap 1).
//   - watch_clock / watch_clock_set: the watch's last-frame clock. watch_stood,
//     the minutes stood, is on the "village" provider already.
//   - spawner_arrival: how many arrivals the spawner has placed. It sets the
//     next pack's bearing and range (packSpots), so a resume that reset it
//     would bring the next pack in from a different side.
//   - bodies: every monster's health, by entity id -- the only place a wolf's
//     wounds live (npc_body.go).
//   - world_rng: the map engine's world RNG, seed and draws. The draws are the
//     digest's rng part, which is a hash; this is the same number, readable
//     -- the same counter read twice, so agreeing with the digest proves the
//     two read one number, not that the number is right. The count is checked
//     against the stream itself in d2mapengine's unit tests, and across two
//     launches by TestTownWalkDeterministic.
//
// Read-only, like "progress": a settable field here would be a harness path
// into the screen's bookkeeping that the game never takes.
type sceneProvider struct{ v *Game }

func (p sceneProvider) HarnessName() string { return "scene" }

func (p sceneProvider) HarnessState() map[string]interface{} {
	v := p.v

	fieldDead := append([]string{}, v.fieldDead...)

	arrival := 0
	if v.spawner != nil {
		arrival = v.spawner.arrival
	}

	ids := make([]string, 0, len(v.bodies))
	for id := range v.bodies {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	bodies := make([]map[string]interface{}, 0, len(ids))

	for _, id := range ids {
		b := v.bodies[id]
		if b == nil {
			continue
		}

		bodies = append(bodies, map[string]interface{}{
			"id": id, "health": b.health, "max_health": b.maxHealth,
		})
	}

	// The same shape as every stream's report: present, seed, seed_str (the
	// exact seed -- a wall-clock one does not survive a float64 reader) and
	// draws; absent when there is no engine, never a zero that reads as one.
	world := d2rand.Absent()
	if v.gameClient != nil && v.gameClient.MapEngine != nil {
		world = d2rand.ReportOf(v.gameClient.MapEngine.RandSeed(), v.gameClient.MapEngine.RandDraws())
	}

	return map[string]interface{}{
		"field_dead":      fieldDead,
		"dawn_paid_day":   v.dawnPaidDay,
		"last_stage":      v.lastStage.String(),
		"watch_clock":     v.watchClock,
		"watch_clock_set": v.watchClockSet,
		"spawner_arrival": arrival,
		"bodies":          bodies,
		"world_rng":       world,
	}
}
