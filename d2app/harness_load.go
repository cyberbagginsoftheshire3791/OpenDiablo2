//go:build harness

package d2app

import (
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
)

// THE HARNESS'S PART OF THE WORLD SAVE'S LOAD (M4.6 B4a): the uuid stream and
// the script's dials. Neither exists in the shipped game -- its ids come from
// crypto/rand and it has no script -- so both live here, behind the tag.
//
// THE UUID STREAM ON BOTH PATHS (B1 notes section 1). A world file written by
// a seeded harness game carries where the stream stood (rng.uuid). A load --
// start_game in a fresh process, or the death screen's "load last save" in
// this one (App.ReloadGame) -- reseeds the stream from BYTE 0 of the file's
// seed before the game opens (harnessLoadBegins), so the connection and the
// villagers the map builds draw the ids a fresh launch of that seed draws, as
// the saved game did; and once every block is restored the stream is put
// where the file says (harnessResume, d2rand.RestoreReader) -- after the
// entities, trap 6 (B4b's step 3 rebuilds them with their saved ids, which
// draws no uuid). Before B4a "load last save"
// continued the dead game's stream and matched no launch.
//
// THE DIALS (the load order's step 7). A dial is never saved (trap 7), so a
// script's dials are not in the file. strigoi_set_system_field records every
// write to a field this file lists as a dial, and a load re-applies the last
// value of each, in the order they were first written, once every block is
// restored: "load last save" resumes with the dials the script set. A fresh
// process has recorded nothing; its script sets its dials itself.

// harnessLoadBegins is the load's step 1 in a harness build: the uuid stream
// reseeded from byte 0 of the file's seed, for the game about to open.
func (a *App) harnessLoadBegins(w *d2save.World) {
	harnessRegisterUUID()

	if w == nil || w.RNG.UUID == nil {
		return
	}

	harnessUUID.mu.Lock()
	defer harnessUUID.mu.Unlock()

	harnessUUID.r = d2rand.NewReader(w.RNG.UUID.Seed)
	harnessUUID.seededFor = harnessUUID.games // harnessGameBegins has counted this game
	harnessUUID.startBytes = 0

	uuid.SetRand(harnessUUID.r)
}

// harnessFallBack is FallBackToDawn's harness half: the dawn game that
// replaces a refused load starts the stream the refused game started from, so
// it is the dawn a fresh launch of that seed would be.
func (a *App) harnessFallBack(_ int64) {
	harnessUUID.mu.Lock()
	r := harnessUUID.r
	harnessUUID.mu.Unlock()

	if r != nil {
		harnessSeedUUID(r.Seeded())
	}
}

// harnessResume is the load's steps 6 and 7 (d2gamescreen.SetResumeHook),
// on the game goroutine once every block is restored. It names the steps it
// ran.
func harnessResume(saved *d2save.UUIDStream) []string {
	var steps []string

	if saved != nil {
		harnessUUID.mu.Lock()
		harnessUUID.r = d2rand.RestoreReader(saved.Seed, saved.Bytes)
		uuid.SetRand(harnessUUID.r)
		harnessUUID.mu.Unlock()

		steps = append(steps, "uuid")
	}

	if harnessReapplyDials() > 0 {
		steps = append(steps, "dials")
	}

	return steps
}

// harnessDialFields are the settable fields that are DIALS -- a number the
// game is tuned by, never state the world file carries -- by system. Every
// other settable field is state (meters, light sources, open bodies, a
// group's morale) or a verb (commit, despawn, release, round), and a load
// must never write it again over what it restored: those are named in
// harnessNotDials, below, and EVERY settable field of every provider is in
// exactly one of the two lists (TestEverySettableFieldIsADialOrNamedState
// reads the providers' own HarnessSettableFields from the source and fails
// on a field in neither -- the B4a review, B3: the village's three radii were
// dials the list had missed, so "load last save" put the church, the
// watching village and the headman's post back at the data's radii).
//
// nolint:gochecknoglobals // a fixed table
var harnessDialFields = map[string][]string{
	"clock": {"frozen", "moon"},
	"combat": {
		"adjacent_tiles", "advantage_shift", "auto_end_turn", "crit_band", "crit_factor",
		"disengage_tiles", "enemy_move_tiles", "engage_tiles", "forced_band", "graze_band",
		"graze_factor", "hit_factor", "lit_level", "loss_weight", "move_tiles", "paced",
		"player_action", "player_control", "quick_resolve_advantage", "round_minutes", "shaken_penalty",
	},
	"pursuit": {"arrive_within", "repath_tiles"},
	"rising":  {"edge_floor", "hasty_weight", "p", "pressure"},
	// M4.6 B5: the dawn autosave switched off, for a script whose subject is
	// a save of its own it must load after a dawn (TestSaveResume). The combat
	// status (30 Sep 2026): its grace, in seconds of game time.
	"save":   {"autosave", "combat_grace"},
	"spawns": {"chance", "check_minutes", "max_groups", "notice_lit_level", "notice_radius", "rout_at"},
	// M4.7 step 4's and J1's radii: where the rite and the watching village
	// reach, and how far from the headman's post a watch is still kept. The
	// data's (dialogue.json's village block), never saved.
	"village": {"rite_radius", "seen_radius", "watch_radius"},
}

// harnessNotDials are the settable fields that are NOT dials, by system, and
// why: state a load restores, or a verb. A load never re-applies them. The
// list is here so that a new settable field has to be classified to pass
// TestEverySettableFieldIsADialOrNamedState -- a field in neither list is a
// question nobody answered.
//
// nolint:gochecknoglobals // a fixed table
var harnessNotDials = map[string][]string{
	// verbs: a round committed, a disengage taken, a round stepped.
	"combat": {"commit", "disengage", "round"},
	// the editor's view, not a game's.
	"editor": {"zoom"},
	// state: the light model's sources (the file's light block).
	"light": {"carried_burn", "carried_lit", "carried_source", "place_source", "remove_source"},
	// state: the meters (the file's squads block) and his body's health and
	// wind (hero.health, hero.stamina); verbs: consume, the squads' add and
	// remove. Two providers answer to "meters" (Meters alone, and Squads,
	// which the game registers); their fields are listed together.
	"meters": {
		"activity", "consume", "fatigue", "food", "health", "stamina", "water",
		"selected", "squad", "squad_add", "squad_remove",
	},
	// a verb: experience granted (the sidecar's progress).
	"progress": {"grant_xp"},
	// a verb: a chase released.
	"pursuit": {"release"},
	// state: a group's morale, the open bodies (the file's spawns block); a
	// verb: a group despawned.
	"spawns": {"despawn", "morale", "open_bodies"},
	// state: his standing (the sidecar's village block).
	"village": {"rep"},
}

// nolint:gochecknoglobals // the script's dials, for this process
var harnessDials struct {
	sync.Mutex
	order  []string // "system.field", first written first
	values map[string]harnessDialWrite
}

type harnessDialWrite struct {
	system, field string
	value         interface{}
}

func harnessIsDial(system, field string) bool {
	for _, f := range harnessDialFields[system] {
		if f == field {
			return true
		}
	}

	return false
}

// harnessRecordDial keeps a successful write to a dial.
func harnessRecordDial(system, field string, value interface{}) {
	if !harnessIsDial(system, field) {
		return
	}

	harnessDials.Lock()
	defer harnessDials.Unlock()

	if harnessDials.values == nil {
		harnessDials.values = map[string]harnessDialWrite{}
	}

	key := system + "." + field
	if _, ok := harnessDials.values[key]; !ok {
		harnessDials.order = append(harnessDials.order, key)
	}

	harnessDials.values[key] = harnessDialWrite{system: system, field: field, value: value}
}

// harnessReapplyDials writes every recorded dial again, on the game
// goroutine, and says how many it wrote. A write the system now refuses is
// skipped: the load is not failed for a script's tuning.
func harnessReapplyDials() int {
	harnessDials.Lock()
	writes := make([]harnessDialWrite, 0, len(harnessDials.order))

	for _, key := range harnessDials.order {
		writes = append(writes, harnessDials.values[key])
	}
	harnessDials.Unlock()

	n := 0

	for _, w := range writes {
		p, ok := d2harness.Lookup(w.system)
		if !ok {
			continue
		}

		if s, ok := p.(d2harness.Settable); ok && s.HarnessSet(w.field, w.value) == nil {
			n++
		}
	}

	return n
}

// harnessDialNames is the recorded dials, sorted, for a report.
func harnessDialNames() []string {
	harnessDials.Lock()
	defer harnessDials.Unlock()

	out := append([]string{}, harnessDials.order...)
	sort.Strings(out)

	return out
}
