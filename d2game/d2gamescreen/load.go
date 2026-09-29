package d2gamescreen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2saveref"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// THE WORLD SAVE'S LOAD, BURST B4a: A QUIET EVENING RESUMES (M4.6, 29 Sep
// 2026). Josh's ruling is that the save "stops at that point then resumes at
// that point"; B3 wrote the file, and this is the first code that reads one
// back into a game. It follows the one authoritative load order in
// docs/m4.6-world-save-notes.md ("The B4 load order"), and does every step of
// it that names no hostile entity:
//
//  1. BEFORE Open (PrepareLoad, from App.ToCreateGame): read the world file;
//     refuse it -- and fall back per rule 7, the file set aside and he begins
//     at dawn from his sidecar -- when its version is not 1 or it is not a file
//     a load could read (d2save.Decode), the game is a network game (rule 9),
//     it is another hero's (World.CheckHeroFile) or a torn save
//     (World.SameMoment), or it holds a hunted night (B4a's own limit: packs,
//     watches, chases, an entity the map does not build, a monster's body, a
//     deployed squad -- all B4b's). Then write the embedded sidecar over his
//     sidecar file. The App hands the server the file's seed and his saved
//     place (SetNextGameSeed, SetNextStartPosition) and, in a harness build,
//     the uuid stream its seed from byte 0.
//  2. In CreateGame, right after NewClock: the clock, validated and restored
//     (trap 1 -- every system built after it samples it).
//  3. (B4b: rebuild the entities. B4a rebuilds none, and checks instead that
//     the villagers the map built are the file's natives, by name_key and
//     born, standing as saved.)
//  4. At the end of CreateGame: VALIDATE EVERY BLOCK (D4) -- the map's SHA
//     (D5), the seed the server ran on, the natives, and each system's own
//     Validate through one Resolver over the resumed map. Nothing has been
//     restored but the clock, and a refusal here closes the game before its
//     first frame (checkLoad).
//  5. On the first frame, in bindGameControls after bindKit (resumeLoad):
//     RESTORE EVERY BLOCK -- his health (after applyProgress), squads, light
//     whole with the kit torch's minutes zeroed (D1), corpses with no open-count
//     callback and placeTheDead skipped (trap 5), rising, spawns (their open
//     bodies from the file) and the spawner's arrival, notice, pursuit, combat
//     (and the ROUND line's key derived from its last round), the scene with
//     lastStage and dawnPaidDay together (trap 1, BUG-17); then him: standing
//     at hero.pos, facing hero.facing, with his stamina and run toggle.
//  6. The world RNG, after everything that draws from it (trap 6); in a
//     harness build the uuid stream's count and
//  7. the dials a script set (the resume hook).
//  8. ANY REFUSAL AT ANY STEP TEARS THE WHOLE GAME DOWN and falls back per
//     rule 7. A half-restored world never runs a frame: the first frame of a
//     loading game binds and restores before anything else in Advance, and a
//     game that fails to restore holds its world, writes nothing on the way
//     out, and is replaced by the dawn (abandonLoad).
//
// WHAT B4a LEAVES TO B4b: every entity the map does not build (packs, the
// risen, deployed squad models) and everything that points at one -- the
// spawn groups, watches, chases, bodies; the villagers' re-keyed ids and their
// motion. A file holding any of them is refused with a clear reason, and the
// player begins at dawn with his hero, kit and progress.

// ErrLoadRefused is what every refusal of a world file wraps.
var ErrLoadRefused = errors.New("the world save cannot be resumed")

// The load's refusal codes (LoadRefusal.Code), for the log and the harness.
const (
	LoadRefusedVersion = "VERSION" // a version other than 1 (a newer build's file)
	LoadRefusedFile    = "FILE"    // version 1, and not a file a load could read (d2save.Decode)
	LoadRefusedNetwork = "NETWORK" // rule 9: a network game loads no world file
	LoadRefusedHero    = "HERO"    // another hero's file (World.CheckHeroFile)
	LoadRefusedTorn    = "TORN"    // a save cut off between its files (World.SameMoment)
	LoadRefusedHunted  = "HUNTED"  // B4a's limit: a hunted night, B4b's
	LoadRefusedSidecar = "SIDECAR" // his sidecar could not be written
	LoadRefusedSeed    = "SEED"    // the game did not run on the file's seed
	LoadRefusedMap     = "MAP"     // D5: another map
	LoadRefusedNatives = "NATIVES" // the villagers are not the file's
	LoadRefusedBlock   = "BLOCK"   // a system's own Validate refused its block
)

// LoadRefusal is a world file the load would not resume, and why.
type LoadRefusal struct {
	Code   string
	Detail string
}

func (r *LoadRefusal) Error() string { return r.Code + ": " + r.Detail }

// Unwrap makes errors.Is(err, ErrLoadRefused) true.
func (r *LoadRefusal) Unwrap() error { return ErrLoadRefused }

func refuseLoad(code, format string, args ...interface{}) *LoadRefusal {
	return &LoadRefusal{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// setsAside reports whether a refusal moves the file out of the way (rule 7).
// A network game's refusal does not: the file is a good single-player save,
// and the next single-player load resumes it (decision B4a-2). Nor does a
// sidecar that could not be written: the fault is the disk's, not the file's.
func (r *LoadRefusal) setsAside() bool {
	return r.Code != LoadRefusedNetwork && r.Code != LoadRefusedSidecar
}

// LoadReport is what the last load did, for the harness (strigoi_start_game,
// strigoi_get_game_info) and the log. It is this process's, never the
// world's: nothing hashed into the state digest reads it.
type LoadReport struct {
	// WorldPath is the world file the load looked for; Found whether one was
	// there.
	WorldPath string `json:"world_path"`
	Found     bool   `json:"found"`

	// Resumed: the game resumed the file's moment. SavedAt is the file's.
	Resumed bool   `json:"resumed"`
	SavedAt string `json:"saved_at,omitempty"`

	// Refused is the refusal's code and Reason its detail; SetAside where the
	// file went (rule 7).
	Refused  string `json:"refused,omitempty"`
	Reason   string `json:"reason,omitempty"`
	SetAside string `json:"set_aside,omitempty"`

	// Steps are the load's steps in the order they ran: the load order as it
	// happened ("clock", then "health", "squads", "light", ...).
	Steps []string `json:"steps"`

	// FellBack: the refusal came after the game was opened, which was torn
	// down and the same hero opened again at dawn (App.FallBackToDawn). The
	// dawn game keeps this report -- it finds no world file, having set it
	// aside -- so the refusal it replaced is still what the harness reads.
	FellBack bool `json:"fell_back,omitempty"`
}

// nolint:gochecknoglobals // one load at a time, reported to the harness
var lastLoad struct {
	sync.Mutex
	r LoadReport
}

// LastLoad is what the last load did.
func LastLoad() LoadReport {
	lastLoad.Lock()
	defer lastLoad.Unlock()

	r := lastLoad.r
	r.Steps = append([]string{}, r.Steps...)

	return r
}

func setLastLoad(r LoadReport) {
	lastLoad.Lock()
	defer lastLoad.Unlock()

	lastLoad.r = r
}

func updateLastLoad(f func(r *LoadReport)) {
	lastLoad.Lock()
	defer lastLoad.Unlock()

	f(&lastLoad.r)
}

// SetLoadAside records a refusal of the world file at savePath's side and,
// when the refusal says so, sets the file aside (d2save.SetAside, rule 7).
// It returns where the file went. afterOpen is a refusal that tears an opened
// game down (FellBack). The App calls it for a refusal CreateGame returned;
// PrepareLoad and abandonLoad call it for their own.
func SetLoadAside(savePath string, refusal *LoadRefusal, afterOpen bool) string {
	worldPath := d2save.WorldPath(savePath)
	aside := ""

	var asideErr error

	if refusal.setsAside() {
		aside, asideErr = d2save.SetAside(worldPath)
	}

	updateLastLoad(func(r *LoadReport) {
		r.WorldPath, r.Found, r.Resumed = worldPath, true, false
		r.Refused, r.Reason, r.SetAside, r.FellBack = refusal.Code, refusal.Detail, aside, afterOpen

		if asideErr != nil {
			r.Reason += fmt.Sprintf(" (and it could not be set aside: %v)", asideErr)
		}
	})

	return aside
}

// PrepareLoad is the load's step 1, before the game client opens: the world
// file beside the hero save at savePath, read and checked by everything that
// needs no game, and -- when it will be resumed -- his sidecar file written
// from the copy the world file carries. It returns the file to resume, or nil:
// with no refusal when there is no world file (a fresh hero, a save made
// before the world save existed), or with the refusal that set it aside, in
// which case he begins at dawn from his sidecar (rule 7).
//
// network is a network game (a LAN host or a client): rule 9 refuses loads as
// it refuses saves, because the TCP handlers race the counted world stream.
func PrepareLoad(savePath string, network bool) (*d2save.World, *LoadRefusal) {
	worldPath := d2save.WorldPath(savePath)
	prev := LastLoad()

	setLastLoad(LoadReport{WorldPath: worldPath, Steps: []string{}})

	if savePath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(worldPath) // nolint:gosec // the hero's own save
	switch {
	case errors.Is(err, os.ErrNotExist):
		// The dawn that replaces a load refused after its game opened: the
		// refusal is still the report (FellBack).
		if prev.FellBack && prev.WorldPath == worldPath {
			setLastLoad(prev)
		}

		return nil, nil
	case err != nil:
		// Unreadable is not "not there": say so, and begin at dawn. It is
		// not set aside -- a file that cannot be read cannot be moved either.
		r := refuseLoad(LoadRefusedFile, "reading %s: %v", worldPath, err)
		updateLastLoad(func(rep *LoadReport) { rep.Found, rep.Refused, rep.Reason = true, r.Code, r.Detail })

		return nil, r
	}

	w, refusal := checkWorldFile(savePath, data, network)
	if refusal != nil {
		SetLoadAside(savePath, refusal, false)

		return nil, refusal
	}

	// The world file's sidecar is his sidecar now: it is the kit, progress,
	// village, land and journal of the saved moment, and every kit save from
	// here carries its generation (bindKit reads it). Written byte for byte
	// as the save wrote it (re-indented from the world file's nesting), so the
	// death screen's copy -- taken by bindKit from this file -- is the save's.
	// The torch's minutes in it are NOT zeroed here (decision B4a-1): the load
	// zeroes them in the kit it binds (D1), so a fall back to dawn keeps them.
	var doc bytes.Buffer
	if err := json.Indent(&doc, w.Sidecar, "", "  "); err != nil {
		r := refuseLoad(LoadRefusedFile, "the world file's sidecar: %v", err)
		SetLoadAside(savePath, r, false)

		return nil, r
	}

	if err := d2items.WriteHero(d2items.SidecarPath(savePath), doc.Bytes()); err != nil {
		r := refuseLoad(LoadRefusedSidecar, "writing his sidecar from the world file: %v", err)
		SetLoadAside(savePath, r, false)

		return nil, r
	}

	updateLastLoad(func(rep *LoadReport) { rep.Found, rep.SavedAt = true, w.SavedAt })

	return w, nil
}

// checkWorldFile is step 1's refusals, in order: rule 9, the file itself, the
// hero it belongs to, the moment of his sidecar, and B4a's own limit.
func checkWorldFile(savePath string, data []byte, network bool) (*d2save.World, *LoadRefusal) {
	if network {
		return nil, refuseLoad(LoadRefusedNetwork, "a network game resumes no world save (rule 9); he begins at dawn, and the file waits for a single-player game")
	}

	w, err := d2save.Decode(data)
	if err != nil {
		if errors.Is(err, d2save.ErrWorldVersion) {
			return nil, refuseLoad(LoadRefusedVersion, "%v", err)
		}

		return nil, refuseLoad(LoadRefusedFile, "%v", err)
	}

	od2, err := os.ReadFile(savePath) // nolint:gosec // the hero's own save
	if err != nil {
		return nil, refuseLoad(LoadRefusedHero, "reading his .od2: %v", err)
	}

	if err := w.CheckHeroFile(od2); err != nil {
		return nil, refuseLoad(LoadRefusedHero, "%v", err)
	}

	sidecar, err := os.ReadFile(d2items.SidecarPath(savePath)) // nolint:gosec // the hero's own save
	if err != nil {
		return nil, refuseLoad(LoadRefusedTorn, "his sidecar: %v -- the world file's moment has no sidecar beside it", err)
	}

	if err := w.SameMoment(sidecar); err != nil {
		return nil, refuseLoad(LoadRefusedTorn, "%v", err)
	}

	if why := huntedNight(w); why != "" {
		return nil, refuseLoad(LoadRefusedHunted, "%s -- a hunted night is burst B4b's to resume", why)
	}

	return w, nil
}

// huntedNight is why a file is not a quiet evening, or "": B4a resumes a world
// that names no entity but the villagers the map builds and the player (the
// word "player"). Everything else here is an entity B4a cannot rebuild, or a
// record pointing at one.
func huntedNight(w *d2save.World) string {
	switch {
	case len(w.Spawns.Groups) > 0:
		return fmt.Sprintf("the spawn tables hold %d pack(s) on the map", len(w.Spawns.Groups))
	case len(w.Notice.Watches) > 0:
		return fmt.Sprintf("%d watch(es) are kept on him or another", len(w.Notice.Watches))
	case len(w.Pursuit.Chases) > 0:
		return fmt.Sprintf("%d chase(s) are running", len(w.Pursuit.Chases))
	case len(w.Bodies) > 0:
		return fmt.Sprintf("%d monster(s) have a body", len(w.Bodies))
	}

	for _, sq := range w.Squads.Squads {
		for _, m := range sq.Members {
			if m.Entity != d2saveref.Player {
				return fmt.Sprintf("squad %s has a deployed model, %s (D3)", sq.ID, m.Entity)
			}
		}
	}

	for _, e := range w.Entities {
		if !e.Native {
			return fmt.Sprintf("entity %s (%s %s%s) is not one the map builds", e.ID, e.Kind, e.Monstat, e.Creature)
		}
	}

	return ""
}

// loadStep records one step of the load as it runs (LoadReport.Steps).
func (v *Game) loadStep(name string) {
	v.loadSteps = append(v.loadSteps, name)

	steps := append([]string{}, v.loadSteps...)
	updateLastLoad(func(r *LoadReport) { r.Steps = steps })
}

// restoreClock is the load's step 2: right after NewClock, the clock checked
// and put at the saved minute, before anything is built that samples it.
func (v *Game) restoreClock(w *d2save.World) *LoadRefusal {
	if err := v.worldClock.Validate(w.Clock); err != nil {
		return refuseLoad(LoadRefusedBlock, "clock: %v", err)
	}

	if err := v.worldClock.Restore(w.Clock); err != nil {
		return refuseLoad(LoadRefusedBlock, "clock: %v", err)
	}

	v.loadStep("clock")

	return nil
}

// checkLoad is the load's step 4, at the end of CreateGame: every block the
// load will restore, checked against the game the file is being resumed into,
// before any is restored (D4). The clock alone is already restored (step 2).
func (v *Game) checkLoad(w *d2save.World) *LoadRefusal {
	if v.gameClient.Seed != w.Seed {
		return refuseLoad(LoadRefusedSeed, "the game runs on seed %d and the file was saved on %d", v.gameClient.Seed, w.Seed)
	}

	// D5: a changed map sets the file aside; he begins at dawn on it.
	path, sha := d2mapgen.HostMap()
	if generated := path == ""; generated != w.Map.Generated || (!generated && sha != w.Map.SHA) {
		return refuseLoad(LoadRefusedMap, "the map is %q (sha %q, generated %v) and the file was saved on %q (sha %q, generated %v) (D5)",
			path, sha, path == "", w.Map.Path, w.Map.SHA, w.Map.Generated)
	}

	if r := v.checkNatives(w); r != nil {
		return r
	}

	seed, r := v.gameClient.Seed, worldResolver{v}

	checks := []struct {
		block string
		err   error
	}{
		{"light", v.light.Validate(w.Light)},
		{"squads", v.squads.Validate(w.Squads)},
		{"corpses", v.corpses.Validate(w.Corpses)},
		{"rising", v.rising.Validate(w.Rising, seed, w.Corpses)},
		{"spawns", v.spawns.Validate(w.Spawns, r, seed)},
		{"spawner", v.spawner.validate(SpawnerSnapshot{Arrival: w.Spawner.Arrival})},
		{"notice", v.notice.Validate(w.Notice, r)},
		{"pursuit", v.pursuit.Validate(w.Pursuit, r)},
		{"combat", v.combat.Validate(w.Combat, seed)},
		{"world rng", w.RNG.World.Check(seed, d2rand.StreamWorld)},
		{"scene", checkStage(w.Scene.LastStage)},
	}

	for _, c := range checks {
		if c.err != nil {
			return refuseLoad(LoadRefusedBlock, "%s: %v", c.block, c.err)
		}
	}

	v.loadStep("validated")

	return nil
}

// checkNatives: the villagers the map built are the file's natives, one for
// one by (name_key, born), each standing as the file saved him. B4a rebuilds
// no entity and re-keys none (B4b's), so a villager missing, added, or moved
// from where the map puts him is a world B4a cannot resume.
func (v *Game) checkNatives(w *d2save.World) *LoadRefusal {
	live := nativesOf(v.gameClient.MapEngine)
	saved := map[nativeKey]d2save.Entity{}

	for _, e := range w.Entities {
		if e.Native {
			saved[nativeKey{nameKey: e.NameKey, x: e.Born[0], y: e.Born[1]}] = e
		}
	}

	for k, e := range live {
		if e == nil {
			return refuseLoad(LoadRefusedNatives, "two villagers the map built are both %q at %v,%v", k.nameKey, k.x, k.y)
		}

		se, ok := saved[k]
		if !ok {
			return refuseLoad(LoadRefusedNatives, "the map builds %q at %v,%v and the file has no such villager (a villager's removal is B4b's)",
				k.nameKey, k.x, k.y)
		}

		if motion, ok := motionOf(e); !ok || !sameMotion(motion, se.Motion) {
			return refuseLoad(LoadRefusedNatives, "%q (born %v,%v) was saved mid-walk or turned (%+v; the map builds him %+v): a villager's motion is B4b's to restore",
				k.nameKey, k.x, k.y, se.Motion, motion)
		}
	}

	if len(saved) != len(live) {
		return refuseLoad(LoadRefusedNatives, "the file has %d villager(s) and the map builds %d", len(saved), len(live))
	}

	return nil
}

func motionOf(e interface{}) (d2mapentity.Motion, bool) {
	switch t := e.(type) {
	case *d2mapentity.NPC:
		return t.MotionSnapshot(), true
	case *d2mapentity.Creature:
		return t.MotionSnapshot(), true
	}

	return d2mapentity.Motion{}, false
}

// sameMotion compares two motions through their JSON, the form the file keeps.
func sameMotion(a, b d2mapentity.Motion) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)

	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}

func checkStage(name string) error {
	if _, ok := stageNamed(name); !ok {
		return fmt.Errorf("last_stage %q is no stage", name)
	}

	return nil
}

func stageNamed(name string) (d2world.Stage, bool) {
	for _, s := range []d2world.Stage{d2world.StageNight, d2world.StageDawn, d2world.StageDay, d2world.StageDusk} {
		if s.String() == name {
			return s, true
		}
	}

	return 0, false
}

// resumeHook is the harness's steps 6 and 7 (SetResumeHook), or nil.
//
// nolint:gochecknoglobals // set once by the harness build, as SetUUIDStream
var resumeHook struct {
	sync.Mutex
	fn func(uuid *d2save.UUIDStream) []string
}

// SetResumeHook hands the load the harness's part of it, called once every
// block is restored: the uuid stream's count put back (step 6, after the
// entities -- B4a rebuilds none, B4b will) and the dials a script set
// re-applied (step 7), naming the steps it ran. The shipped game never sets
// it: its ids come from crypto/rand, and it has no script.
func SetResumeHook(fn func(uuid *d2save.UUIDStream) []string) {
	resumeHook.Lock()
	defer resumeHook.Unlock()

	resumeHook.fn = fn
}

// resumeLoad is steps 5 to 7, on the first frame, once his kit and progress
// are bound (bindGameControls, after bindKit): the checks that need them,
// then every block restored, then him, then the world RNG, then the harness's
// part. A refusal before the first restore leaves the world as CreateGame
// made it; one after it cannot happen unless a Restore disagrees with its own
// Validate, and either way the caller tears the game down (abandonLoad).
func (v *Game) resumeLoad(w *d2save.World) error {
	if err := v.checkBound(w); err != nil {
		return err
	}

	seed, r := v.gameClient.Seed, worldResolver{v}

	// His health, after applyProgress set his maximum from the sidecar's
	// progress (checkBound held it to that maximum).
	v.localPlayer.Stats.Health = w.Hero.Health
	v.loadStep("health")

	if err := v.squads.Restore(w.Squads); err != nil {
		return refuseLoad(LoadRefusedBlock, "squads: %v", err)
	}

	v.loadStep("squads")

	// D1: THE LIGHT MODEL IS THE TRUTH. The snapshot whole, his carried torch
	// included (its id, lit or doused, its minutes, next_id); and the kit's
	// torch holds zero, the state the L key leaves it in, so its minutes are
	// not counted twice. Nothing is pressed: L would give the torch a new id,
	// light a doused one and count a verb the saved game never made.
	if err := v.light.Restore(w.Light); err != nil {
		return refuseLoad(LoadRefusedBlock, "light: %v", err)
	}

	v.loadStep("light")

	if v.light.Carried() != nil {
		if _, torch, ok := v.kit.OffHandTorch(); ok {
			torch.BurnLeft = 0
		}

		v.loadStep("torch")
	}

	// Trap 5: the corpses' Restore never calls the open-count callback, and
	// placeTheDead was not run -- the spawn tables' open bodies come from the
	// file, below.
	if err := v.corpses.Restore(w.Corpses); err != nil {
		return refuseLoad(LoadRefusedBlock, "corpses: %v", err)
	}

	v.loadStep("corpses")

	if err := v.rising.Restore(w.Rising, seed); err != nil {
		return refuseLoad(LoadRefusedBlock, "rising: %v", err)
	}

	v.loadStep("rising")

	if err := v.spawns.Restore(w.Spawns, r, seed); err != nil {
		return refuseLoad(LoadRefusedBlock, "spawns: %v", err)
	}

	v.loadStep("spawns")

	if err := v.spawner.Restore(SpawnerSnapshot{Arrival: w.Spawner.Arrival}); err != nil {
		return refuseLoad(LoadRefusedBlock, "spawner: %v", err)
	}

	v.loadStep("spawner")

	if err := v.notice.Restore(w.Notice, r); err != nil {
		return refuseLoad(LoadRefusedBlock, "notice: %v", err)
	}

	v.loadStep("notice")

	if err := v.pursuit.Restore(w.Pursuit, r); err != nil {
		return refuseLoad(LoadRefusedBlock, "pursuit: %v", err)
	}

	v.loadStep("pursuit")

	if err := v.combat.Restore(w.Combat, seed); err != nil {
		return refuseLoad(LoadRefusedBlock, "combat: %v", err)
	}

	// The ROUND line's edge: the last round the saved game logged is logged
	// already, so the first frame writes no second line for it (the game
	// screen's field classification: paceRoundKey is D, derived here).
	if row := v.combat.LastRound(); row.Encounter != "" {
		v.paceRoundKey = fmt.Sprintf("%s#%d", row.Encounter, row.Round)
	}

	v.loadStep("combat")

	v.restoreScene(w.Scene)
	v.loadStep("scene")

	// Rule 4: he stands where he was saved, facing as he faced; his walk, if
	// he had one, does not continue. His wind and his run toggle with him.
	v.localPlayer.StandAt(w.Hero.Pos[0], w.Hero.Pos[1], w.Hero.Facing)
	v.localPlayer.Stats.Stamina = w.Hero.Stamina

	if v.gameControls != nil {
		v.gameControls.RestoreRun(w.Hero.Run)
	} else if v.localPlayer.IsRunToggled() != w.Hero.Run {
		v.localPlayer.ToggleRunWalk()
		v.localPlayer.SetIsRunning(v.localPlayer.IsRunToggled())
	}

	v.loadStep("hero")

	// The region he stands in, read at once rather than a second into play:
	// its sound environment is derived, and the saved game had long read it.
	v.checkRegion()

	// Trap 6: last, after everything that draws from it -- the map's
	// generation and the villagers' construction drew in Open.
	if err := v.gameClient.MapEngine.RestoreRand(w.RNG.World.Seed, w.RNG.World.Draws); err != nil {
		return refuseLoad(LoadRefusedBlock, "world rng: %v", err)
	}

	v.loadStep("world_rng")

	resumeHook.Lock()
	hook := resumeHook.fn
	resumeHook.Unlock()

	if hook != nil {
		for _, step := range hook(w.RNG.UUID) {
			v.loadStep(step)
		}
	}

	return nil
}

// checkBound is step 4's half that needs his kit and progress bound: the
// world file's sidecar gave him a kit (so the loadout choice is not holding
// the world), his saved health fits the maximum his progress gives him, and
// a carried torch is the one in his off-hand.
func (v *Game) checkBound(w *d2save.World) error {
	switch {
	case v.localPlayer == nil || v.localPlayer.Stats == nil:
		return refuseLoad(LoadRefusedBlock, "hero: he is not in the world")
	case v.kit == nil || v.choosingLoadout:
		return refuseLoad(LoadRefusedBlock, "sidecar: the world file's sidecar gave him no kit")
	case w.Hero.Health > v.localPlayer.Stats.MaxHealth:
		return refuseLoad(LoadRefusedBlock, "hero: health %d is past the %d his progress gives him",
			w.Hero.Health, v.localPlayer.Stats.MaxHealth)
	}

	for _, src := range w.Light.Sources {
		if src.Carried {
			if _, _, ok := v.kit.OffHandTorch(); !ok {
				return refuseLoad(LoadRefusedBlock, "light: a carried torch, and no torch in his off-hand")
			}
		}
	}

	return nil
}

// restoreScene puts the game screen's own bookkeeping back. lastStage and
// dawnPaidDay TOGETHER, from the file (trap 1, BUG-17): bindProgress set both
// from the restored clock, and a night lived through but not yet paid would
// otherwise be paid twice or never.
func (v *Game) restoreScene(s d2save.Scene) {
	v.lastStage, _ = stageNamed(s.LastStage)
	v.dawnPaidDay = s.DawnPaidDay
	v.watchStood = s.WatchStood
	v.fieldDead = append([]string{}, s.FieldDead...)
	v.watchClock, v.watchClockSet = s.WatchClock, s.WatchClockSet
}

// loadFallback is the App's way back from a load refused after its game was
// opened: the game is closed, and the same hero opened again -- with the world
// file set aside, at dawn -- on the seed this game ran on.
type loadFallback interface {
	FallBackToDawn(savePath string, seed int64, reason string)
}

// failPendingLoad tears down a load its first frame refused (Advance calls
// it once the controls are bound, so the teardown unbinds what was bound).
func (v *Game) failPendingLoad() {
	if v.loadFailed == nil {
		return
	}

	err := v.loadFailed
	v.loadFailed = nil
	v.abandonLoad(err)
}

// abandonLoad is step 8 for a refusal on the first frame: nothing more of the
// world runs (Advance returns at once), nothing is written on the way out
// (OnUnload saves nothing for an abandoned load), the file is set aside, and
// the game is replaced by the dawn.
func (v *Game) abandonLoad(err error) {
	if v.loadAbandoned {
		return
	}

	v.loadAbandoned = true
	v.pendingLoad = nil

	var refusal *LoadRefusal
	if !errors.As(err, &refusal) {
		refusal = refuseLoad(LoadRefusedBlock, "%v", err)
	}

	save := v.gameClient.SaveFilePath
	aside := SetLoadAside(save, refusal, true)
	reason := fmt.Sprintf("the world save was not resumed: %v", refusal)

	if aside != "" {
		reason += "; set aside as " + aside
	}

	v.Errorf("LOAD refused, torn down: %s", reason)

	if fb, ok := v.navigator.(loadFallback); ok {
		fb.FallBackToDawn(save, v.gameClient.Seed, reason)

		return
	}

	if v.navigator != nil {
		v.navigator.ToMainMenu(reason)
	}
}

// loadResumed is a load that restored every block: the report says so, and
// the log names the moment and the steps.
func (v *Game) loadResumed() {
	savedAt := ""

	updateLastLoad(func(r *LoadReport) {
		r.Resumed = true
		savedAt = r.SavedAt
	})

	v.Infof("LOAD resumed the world saved at %s: %v", savedAt, v.loadSteps)
}
