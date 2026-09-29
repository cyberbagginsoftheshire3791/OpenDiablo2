package d2gamescreen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2saveref"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// THE WORLD SAVE'S LOAD (M4.6): BURST B4a, A QUIET EVENING RESUMES, AND
// BURST B4b, A HUNTED NIGHT RESUMES (29 Sep 2026). Josh's ruling is that the
// save "stops at that point then resumes at that point"; B3 wrote the file,
// B4a is the first code that read one back into a game, and B4b rebuilt every
// entity the file names -- the packs, the chases, the risen, the wounded, the
// deployed squad models -- so a night resumes as it was saved. It follows the
// one authoritative load order in docs/m4.6-world-save-notes.md ("The B4 load
// order"):
//
//  1. BEFORE Open (PrepareLoad, from App.ToCreateGame): undo a load a crash
//     cut off (recoverPreload); read the world file (unless this process
//     refused it and could not move it: B1); refuse it -- and fall back per
//     rule 7, the file set aside and he begins at dawn from his sidecar --
//     when its version is not d2save.Version or it is not a file a load could read
//     (d2save.Decode), the game is a network game (rule 9), it is another
//     hero's (World.CheckHeroFile) or a torn save (World.SameMoment). (B4a
//     also left a hunted night where it was, as B4b's -- HUNTED; B4b resumes
//     it, and the refusal is gone.) Then KEEP HIS OWN SIDECAR (in memory and
//     as .preload: preload.go) and write the embedded sidecar over his sidecar
//     file. The App hands the server the file's seed and his saved place
//     (SetNextGameSeed, SetNextStartPosition) and, in a harness build, the
//     uuid stream its seed from byte 0.
//  2. In CreateGame, right after NewClock: the clock, validated and restored
//     (trap 1 -- every system built after it samples it).
//  3. At the end of CreateGame, once the seed and the map are the file's
//     (B4b, rebuildEntities): THE ENTITIES. The villagers first -- the map
//     built them again in Open; each is matched to the file's native by
//     (name_key, born), given his saved id IN PLACE (MapEngine.RekeyEntity)
//     and his saved motion, and one the file lacks is taken off the map. Then
//     every other entity the file names, in the file's order, through the
//     one-shot next-id seam: SetNextEntityID(id), NewNPC or NewCreature by its
//     kind, its id checked (a constructor that spent the id refuses the
//     file), RestoreMotion (monsters, the risen and deployed squads keep their
//     walk), AddEntity. Nothing may be left waiting in the seam.
//  4. Then, still in CreateGame: VALIDATE EVERY BLOCK (D4) -- each system's
//     own Validate through one Resolver over the resumed map (the save's type,
//     worldResolver), the bodies against the rebuilt monsters, the deployed
//     squads' models against the map. Nothing has been restored but the
//     clock and the entities, and a refusal here closes the game before its
//     first frame (checkLoad).
//  5. On the first frame, in bindGameControls after bindKit (resumeLoad):
//     RESTORE EVERY BLOCK -- his health (after applyProgress), squads, light
//     whole with the kit torch's minutes zeroed (D1), corpses with no open-count
//     callback and placeTheDead skipped (trap 5), rising, spawns (their open
//     bodies from the file) and the spawner's arrival, notice, pursuit, combat
//     (and the ROUND line's key derived from its last round), the monsters'
//     bodies (B4b), the scene with lastStage and dawnPaidDay together (trap
//     1, BUG-17); then him: standing at hero.pos, facing hero.facing, with his
//     stamina and run toggle -- his walk does not continue (rule 4).
//  6. The world RNG, after everything that draws from it (trap 6: the map's
//     build, and step 3's NewNPC and NewCreature); in a harness build the
//     uuid stream's count and
//  7. the dials a script set (the resume hook).
//  8. ANY REFUSAL AT ANY STEP TEARS THE WHOLE GAME DOWN, PUTS HIS OWN SIDECAR
//     BACK (SetLoadAside: the B4a review, A1) and falls back per rule 7, on
//     the file's seed. A half-restored world never runs a frame: the first
//     frame of a loading game binds and restores before anything else in
//     Advance, and a game that fails to restore holds its world, writes
//     nothing on the way out, and is replaced by the dawn (abandonLoad).
//
// WHAT B4a LEFT TO B4b, AND B4b DID: every entity the map does not build
// (packs, the risen, deployed squad models) and everything that points at one
// -- the spawn groups, watches, chases, bodies; the villagers' re-keyed ids
// and their motion. A file B4a refused HUNTED is resumed now.

// ErrLoadRefused is what every refusal of a world file wraps.
var ErrLoadRefused = errors.New("the world save cannot be resumed")

// The load's refusal codes (LoadRefusal.Code), for the log and the harness.
const (
	LoadRefusedVersion = "VERSION" // a version other than d2save.Version (a newer or an older build's file)
	LoadRefusedFile    = "FILE"    // this build's version, and not a file a load could read (d2save.Decode)
	LoadRefusedNetwork = "NETWORK" // rule 9: a network game loads no world file
	LoadRefusedHero    = "HERO"    // another hero's file (World.CheckHeroFile)
	LoadRefusedTorn    = "TORN"    // a save cut off between its files (World.SameMoment)
	LoadRefusedSidecar = "SIDECAR" // his sidecar could not be written
	LoadRefusedSeed    = "SEED"    // the game did not run on the file's seed
	LoadRefusedMap     = "MAP"     // D5: another map
	LoadRefusedNatives = "NATIVES" // the villagers are not the file's
	LoadRefusedEntity  = "ENTITY"  // B4b: an entity the file names could not be rebuilt as it was saved
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

// EVERY REFUSAL SETS THE FILE ASIDE (rule 7). Until B4b one did not: B4a left
// a hunted night where it was, HUNTED, for B4b to resume (decision B4a-R3),
// and B4b resumes it -- the refusal and its exception are gone. A network
// game's refusal and a sidecar that could not be written used to leave the
// file too (decision B4a-2, OVERTURNED by decision B4a-R1): a session that did
// not resume the file carried its generation in every kit save, so the next
// single-player load resumed the old moment over everything that session had
// earned (BUG-61). The one file a load still leaves is one that could not be
// moved (B1), and it is made harmless the other way: a game that did not
// resume it never writes its generation (bindKit), so the file reads TORN the
// moment his sidecar moves on.

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

	// Preload is what became of the copy of his sidecar step 1 keeps before
	// it writes the world file's over it (the B4a review, A1; preload.go):
	// "restored" -- the load was refused after that write and his own was put
	// back, byte for byte; "recovered" -- a load a crash cut off was undone at
	// this start; "discarded" -- a copy a crash left behind was older than the
	// world save beside it; "kept" -- a copy a crash left behind could not be
	// judged and was moved to .preload.kept, never offered and never deleted.
	Preload string `json:"preload,omitempty"`

	// Notes are what the load did or found that is not a refusal and that a
	// player could ask about (the B4b review fixes): a villager the file
	// lacks taken off the map (BUG-79), an entity whose name this build gives
	// otherwise than the save did (BUG-80). Each is in the log too.
	Notes []string `json:"notes,omitempty"`

	// Dropped are the villagers (their name_key) the map built and the file
	// lacks, taken off the map again (one of Notes' kinds, BUG-79), for the
	// notice a player sees at the start of play (M4.6 B5).
	Dropped []string `json:"dropped,omitempty"`

	// Ignored: the file was refused earlier in this process and could not be
	// set aside, so no load reads it again until it changes (the B4a review,
	// B1: without this the dawn that replaced it found it again, refused it
	// again, and reloaded -- 51 teardowns in 25 s).
	Ignored bool `json:"ignored,omitempty"`

	// carried: this report was kept by the dawn that replaced its game, and
	// the load after that one starts its own (a script's next start_game of
	// the same hero used to read the old refusal as its own).
	carried bool
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

	if r.Notes != nil {
		r.Notes = append([]string{}, r.Notes...)
	}

	if r.Dropped != nil {
		r.Dropped = append([]string{}, r.Dropped...)
	}

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
//
// FIRST IT PUTS HIS SIDECAR BACK (the B4a review, A1; BUG-60). Step 1 writes
// the world file's copy of his sidecar over his own before the game opens; a
// refusal after that -- the seed, the map, the villagers or a block (steps 2,
// 4 and 5), or the sidecar's own write -- used to leave the world file's copy
// there, so the dawn he fell back to had the saved moment's experience and
// standing, everything he had earned since was gone, and his .od2 (not
// rewritten by a load) was of the later moment: two moments mixed. Now every
// refusal of a prepared load restores the copy step 1 kept, byte for byte,
// before anything else, and the dawn is his own last-entered dawn -- the same
// dawn a refusal at step 1 gives. A refusal with nothing kept restores
// nothing.
//
// A FILE THAT CANNOT BE SET ASIDE IS IGNORED FOR THE REST OF THIS PROCESS (the
// B4a review, B1; BUG-62). The move is retried for half a second
// (d2items.RenameRetrying, inside d2save.SetAside); if the file is still held
// it stays, and until it changes no load in this process reads it again --
// the dawn that replaces a refused game used to find it, refuse it, tear down
// and reload, for ever.
func SetLoadAside(savePath string, refusal *LoadRefusal, afterOpen bool) string {
	worldPath := d2save.WorldPath(savePath)

	restored, restoreErr := restorePreload(savePath)

	aside, asideErr := setAsideWorld(worldPath)
	if asideErr != nil {
		ignoreFromNowOn(worldPath, refusal)
	}

	updateLastLoad(func(r *LoadReport) {
		r.WorldPath, r.Found, r.Resumed = worldPath, true, false
		r.Refused, r.Reason, r.SetAside, r.FellBack = refusal.Code, refusal.Detail, aside, afterOpen

		if restored {
			r.Preload = "restored"
		}

		if restoreErr != nil {
			r.Reason += fmt.Sprintf(" (and his own sidecar could not be put back: %v -- it is kept as %s)",
				restoreErr, preloadPath(savePath))
		}

		if asideErr != nil {
			r.Reason += fmt.Sprintf(" (and it could not be set aside: %v -- no load reads it again in this run)", asideErr)
		}
	})

	return aside
}

// setAsideWorld is d2save.SetAside, and writeStepOne the sidecar write of
// step 1 (d2items.WriteHero); a test swaps them for the failures it cannot
// cause portably -- a file held open, a write the disk refuses half done.
//
// nolint:gochecknoglobals // seams for failures a unit test cannot cause portably
var (
	setAsideWorld = d2save.SetAside
	writeStepOne  = d2items.WriteHero
)

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
// Since the B4a review (A2, decision B4a-R1) the refused file is SET ASIDE, as
// every other refusal's is: his hero, kit and progress carry on into the
// network game; the saved night does not.
func PrepareLoad(savePath string, network bool) (*d2save.World, *LoadRefusal) {
	worldPath := d2save.WorldPath(savePath)
	prev := LastLoad()

	setLastLoad(LoadReport{WorldPath: worldPath, Steps: []string{}})

	if savePath == "" {
		return nil, nil
	}

	// A1: a load a crash cut off between step 1's write and its end is undone
	// before anything reads his sidecar (preload.go).
	recovered := recoverPreload(savePath)
	if recovered != "" {
		updateLastLoad(func(rep *LoadReport) { rep.Preload = recovered })
	}

	// B1: a file refused earlier in this process that could not be set aside
	// is not read again until it changes. The dawn that replaces a torn-down
	// load keeps that refusal as its report.
	if r := ignoredRefusal(worldPath); r != nil {
		if prev.WorldPath == worldPath && prev.Refused != "" && !prev.carried {
			prev.Steps, prev.carried = []string{}, true
			setLastLoad(prev)
		}

		updateLastLoad(func(rep *LoadReport) {
			rep.Found, rep.Refused, rep.Ignored = true, r.Code, true
			if rep.Reason == "" {
				rep.Reason = r.Detail
			}

			if recovered != "" {
				rep.Preload = recovered
			}
		})

		return nil, r
	}

	data, err := os.ReadFile(worldPath) // nolint:gosec // the hero's own save
	switch {
	case errors.Is(err, os.ErrNotExist):
		// The dawn that replaces a load refused after its game opened: the
		// refusal is still the report (FellBack) -- for that dawn only.
		if prev.FellBack && prev.WorldPath == worldPath && !prev.carried {
			prev.carried = true
			setLastLoad(prev)
		}

		return nil, nil
	case err != nil:
		// Unreadable is not "not there": say so, and begin at dawn. It is
		// set aside like every other refusal (the B4a review, A2) -- a file
		// held open cannot be moved either, and is then ignored for the rest
		// of this run (B1).
		r := refuseLoad(LoadRefusedFile, "reading %s: %v", worldPath, err)
		SetLoadAside(savePath, r, false)

		return nil, r
	}

	w, sidecar, refusal := checkWorldFile(savePath, data, network)
	if refusal != nil {
		SetLoadAside(savePath, refusal, false)

		return nil, refusal
	}

	// A1: HIS OWN SIDECAR IS KEPT BEFORE THE WORLD FILE'S IS WRITTEN OVER IT,
	// in memory and as N.od2.strigoi.json.preload, so any refusal from here
	// on can put it back (SetLoadAside) and a crash before the load ends is
	// undone at the next start (recoverPreload). A copy that cannot be kept
	// is a load that could not be undone: refused, before anything is
	// written.
	if err := keepPreload(savePath, sidecar); err != nil {
		r := refuseLoad(LoadRefusedSidecar, "keeping his sidecar before the load: %v", err)
		SetLoadAside(savePath, r, false)

		return nil, r
	}

	// The world file's sidecar is his sidecar now: it is the kit, progress,
	// village, land and journal of the saved moment, and every kit save from
	// here carries its generation (bindKit reads it). Written byte for byte
	// as the save wrote it (re-indented from the world file's nesting), so the
	// death screen's copy -- taken by bindKit from this file -- is the save's.
	// The torch's minutes in it are NOT zeroed here (decision B4a-1): the load
	// zeroes them in the kit it binds (D1), so a fall back to dawn keeps them.
	doc, err := stepOneSidecar(w)
	if err != nil {
		r := refuseLoad(LoadRefusedFile, "the world file's sidecar: %v", err)
		SetLoadAside(savePath, r, false)

		return nil, r
	}

	if err := writeStepOne(d2items.SidecarPath(savePath), doc); err != nil {
		r := refuseLoad(LoadRefusedSidecar, "writing his sidecar from the world file: %v", err)
		SetLoadAside(savePath, r, false)

		return nil, r
	}

	updateLastLoad(func(rep *LoadReport) { rep.Found, rep.SavedAt = true, w.SavedAt })

	return w, nil
}

// stepOneSidecar is the document step 1 writes over his sidecar: the world
// file's embedded sidecar, re-indented as d2items.HeroBytes writes it.
func stepOneSidecar(w *d2save.World) ([]byte, error) {
	var doc bytes.Buffer
	if err := json.Indent(&doc, w.Sidecar, "", "  "); err != nil {
		return nil, err
	}

	return doc.Bytes(), nil
}

// PeekLoad is step 1's checks and nothing else: the world file beside the
// save at savePath, and whether a single-player load would take it -- no
// file set aside, nothing written, no report. The harness's start_game asks
// it before refusing a seed the file was not saved on (the B4a review, C2):
// a file the load refuses anyway begins at dawn on the seed the script asked
// for, so that seed is no reason to refuse the start. nil, nil: no file.
func PeekLoad(savePath string) (*d2save.World, *LoadRefusal) {
	worldPath := d2save.WorldPath(savePath)
	if savePath == "" {
		return nil, nil
	}

	if r := ignoredRefusal(worldPath); r != nil {
		return nil, r
	}

	data, err := os.ReadFile(worldPath) // nolint:gosec // the hero's own save
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, refuseLoad(LoadRefusedFile, "reading %s: %v", worldPath, err)
	}

	w, _, r := checkWorldFile(savePath, data, false)

	return w, r
}

// checkWorldFile is step 1's refusals, in order: rule 9, the file itself, the
// hero it belongs to and the moment of his sidecar. (B4a's own limit, a hunted
// night, was the fifth; B4b resumes one.) It returns his sidecar file as it
// read it, which step 1 keeps (A1).
func checkWorldFile(savePath string, data []byte, network bool) (*d2save.World, []byte, *LoadRefusal) {
	if network {
		return nil, nil, refuseLoad(LoadRefusedNetwork,
			"a network game resumes no world save (rule 9); he begins with his hero, kit and progress, and the saved night is set aside")
	}

	w, err := d2save.Decode(data)
	if err != nil {
		if errors.Is(err, d2save.ErrWorldVersion) {
			return nil, nil, refuseLoad(LoadRefusedVersion, "%v", err)
		}

		return nil, nil, refuseLoad(LoadRefusedFile, "%v", err)
	}

	od2, err := os.ReadFile(savePath) // nolint:gosec // the hero's own save
	if err != nil {
		return nil, nil, refuseLoad(LoadRefusedHero, "reading his .od2: %v", err)
	}

	if err := w.CheckHeroFile(od2); err != nil {
		return nil, nil, refuseLoad(LoadRefusedHero, "%v", err)
	}

	sidecar, err := os.ReadFile(d2items.SidecarPath(savePath)) // nolint:gosec // the hero's own save
	if err != nil {
		return nil, nil, refuseLoad(LoadRefusedTorn, "his sidecar: %v -- the world file's moment has no sidecar beside it", err)
	}

	if err := w.SameMoment(sidecar); err != nil {
		return nil, nil, refuseLoad(LoadRefusedTorn, "%v", err)
	}

	return w, sidecar, nil
}

// loadStep records one step of the load as it runs (LoadReport.Steps).
func (v *Game) loadStep(name string) {
	v.loadSteps = append(v.loadSteps, name)

	steps := append([]string{}, v.loadSteps...)
	updateLastLoad(func(r *LoadReport) { r.Steps = steps })
}

// loadNote records something the load did or found that is not a refusal
// (LoadReport.Notes), and logs it: the B4b review found a villager the file
// lacked taken off the map with no line anywhere (its C2).
func (v *Game) loadNote(format string, args ...interface{}) {
	note := fmt.Sprintf(format, args...)

	v.Warningf("LOAD %s", note)
	updateLastLoad(func(r *LoadReport) { r.Notes = append(r.Notes, note) })
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

// checkLoad is the load's steps 3 and 4, at the end of CreateGame: the game
// runs on the file's seed and map; then the entities are rebuilt (step 3, B4b:
// rebuildEntities); then every block the load will restore is checked against
// the game the file is being resumed into, through one Resolver over the
// resumed map, before any is restored (D4). The clock alone is already
// restored (step 2). A refusal anywhere closes this game before its first
// frame: the entities the rebuild placed go with its map.
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

	// Step 3 (B4b): every entity the file names, on the map with its saved id
	// and motion, before anything that resolves one is checked.
	if r := v.rebuildEntities(w); r != nil {
		return r
	}

	seed, r := v.gameClient.Seed, worldResolver{v}

	checks := []struct {
		block string
		err   error
	}{
		{"light", v.light.Validate(w.Light)},
		{"squads", v.squads.Validate(w.Squads)},
		{"squads", v.checkSquadModels(w.Squads)},
		{"corpses", v.corpses.Validate(w.Corpses)},
		{"rising", v.rising.Validate(w.Rising, seed, w.Corpses)},
		{"spawns", v.spawns.Validate(w.Spawns, r, seed)},
		{"spawner", v.spawner.validate(SpawnerSnapshot{Arrival: w.Spawner.Arrival})},
		{"notice", v.notice.Validate(w.Notice, r)},
		{"pursuit", v.pursuit.Validate(w.Pursuit, r)},
		{"combat", v.combat.Validate(w.Combat, seed)},
		// BUG-73: each live clock fight held against the FILE's watches and
		// chases. Only the file holds both halves here: the combat model's
		// notice is this new game's, which watches no one until step 5.
		{"combat", d2world.CheckClockWatches(w.Combat, w.Notice, w.Pursuit)},
		{"bodies", v.checkBodies(w.Bodies)},
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

// rebuiltEntity is what the load asks of an entity it re-keys or rebuilds:
// an NPC or a creature, whose walk and pose it puts back and reads again.
type rebuiltEntity interface {
	d2interface.MapEntity
	MotionSnapshot() d2mapentity.Motion
	RestoreMotion(d2mapentity.Motion) error
}

// rebuildEntities is the load's step 3 (M4.6 B4b), in the load order's
// order: the villagers first, then every other entity the file names; and
// nothing may be left waiting in the next-id seam.
func (v *Game) rebuildEntities(w *d2save.World) *LoadRefusal {
	if r := v.rekeyNatives(w); r != nil {
		return r
	}

	v.loadStep("natives")

	for _, se := range w.Entities {
		if se.Native {
			continue
		}

		if r := v.rebuildEntity(se); r != nil {
			return r
		}
	}

	if id, waiting := v.gameClient.MapEngine.PendingEntityID(); waiting {
		return refuseLoad(LoadRefusedEntity, "%q is still waiting in the next-id seam after the rebuild: the entity it was set for was never built", id)
	}

	v.loadStep("entities")

	return nil
}

// rekeyNatives is step 3's first half: the villagers. The map built them
// again in Open, and the game screen captured them, by the entity, before
// anything else could be placed (natives, in CreateGame). Each is matched to
// the file's native by (name_key, born) -- where the map put him, which
// World.Check holds unique in the file and saveEntities at the save -- and:
//   - given the id he was saved under, IN PLACE (MapEngine.RekeyEntity): he
//     stays the entity the screen holds, so he is still native at the next
//     save (the B3 review, B3). In a harness build the id is already his (the
//     uuid stream is reseeded from the file's seed); in the shipped game it is
//     a new crypto/rand id every launch, and this is what puts the file's back;
//   - given the walk and pose he was saved in (RestoreMotion), read back and
//     required equal. (B4a refused a villager saved mid-walk, NATIVES; B4b
//     restores him. What is NOT carried is a patrol's place -- path index,
//     repetitions, the arrival that starts his next leg, his own rng -- and
//     only the generated Act 1's villagers patrol: the authored village's
//     stand. B4b-2 said a patrolling villager resumed mid-leg "finishes his
//     leg and stands"; the B4b review read otherwise and it was right (its
//     C1; BUG-78): the map's SetPaths leaves him isDone, so on his first
//     animation loop Advance gives him the next path -- path[1] -- and he
//     abandons the restored leg. A -classic resume of a patrolling villager
//     is not exact.)
//
// A villager the map builds and the file lacks was taken off the map before
// the save (B3-6), and is taken off again here, noted in the load report and
// the log, and dropped from the screen's natives (BUG-79). A villager or an
// entity saved while it held an action is refused (BUG-76). A villager the file has and
// the map does not build, or two the map built on one key, or one of another
// kind or monstat than saved, is a world this build cannot resume: NATIVES.
func (v *Game) rekeyNatives(w *d2save.World) *LoadRefusal {
	engine := v.gameClient.MapEngine
	saved := map[nativeKey]d2save.Entity{}

	for _, e := range w.Entities {
		if e.Native && e.Born != nil {
			saved[nativeKey{nameKey: e.NameKey, x: e.Born[0], y: e.Born[1]}] = e
		}
	}

	keys := make([]nativeKey, 0, len(v.natives))
	for k := range v.natives {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.nameKey != b.nameKey {
			return a.nameKey < b.nameKey
		}

		if a.x != b.x {
			return a.x < b.x
		}

		return a.y < b.y
	})

	type pair struct {
		live  rebuiltEntity
		saved d2save.Entity
	}

	pairs := make([]pair, 0, len(saved))

	for _, k := range keys {
		e := v.natives[k]
		if e == nil {
			return refuseLoad(LoadRefusedNatives, "two villagers the map built are both %q at %v,%v", k.nameKey, k.x, k.y)
		}

		se, ok := saved[k]
		if !ok {
			// B3-6: taken off the map before the save, so taken off now --
			// and said so, and no longer held as a native (the B4b review
			// fixes, BUG-79: he was removed with no log line and no report,
			// and stayed in the screen's natives). A generated map has no
			// SHA for D5 to hold to the file, so a build whose -classic
			// village gained a villager would drop him here: the note is
			// how anyone would know.
			engine.RemoveEntity(e)
			delete(v.natives, k)
			v.loadNote("the villager %q born at %v,%v (%s here) is not in the file, which was saved after he was "+
				"taken off the map: taken off it again", k.nameKey, k.x, k.y, e.ID())
			updateLastLoad(func(r *LoadReport) { r.Dropped = append(r.Dropped, k.nameKey) })

			continue
		}

		live, ok := e.(rebuiltEntity)
		if !ok || kindOf(e) != se.Kind {
			return refuseLoad(LoadRefusedNatives, "%q (born %v,%v) is a %T on the map and was saved as a %s", k.nameKey, k.x, k.y, e, se.Kind)
		}

		if npc, isNPC := e.(*d2mapentity.NPC); isNPC && (npc.MonStat() == nil || npc.MonStat().Key != se.Monstat) {
			return refuseLoad(LoadRefusedNatives, "%q (born %v,%v) was saved as monstat %q and the map builds another", k.nameKey, k.x, k.y, se.Monstat)
		}

		if why := heldInFile(se); why != "" {
			return refuseLoad(LoadRefusedNatives, "%q (born %v,%v) %s", k.nameKey, k.x, k.y, why)
		}

		pairs = append(pairs, pair{live: live, saved: se})
	}

	if len(pairs) != len(saved) {
		for k, se := range saved {
			if _, ok := v.natives[k]; !ok {
				return refuseLoad(LoadRefusedNatives, "the file's villager %s, %q born at %v,%v, is not one the map builds",
					se.ID, k.nameKey, k.x, k.y)
			}
		}
	}

	for _, p := range pairs {
		if err := engine.RekeyEntity(p.live, p.saved.ID); err != nil {
			return refuseLoad(LoadRefusedNatives, "%q (born %v,%v) cannot take his saved id: %v", p.saved.NameKey, p.saved.Born[0], p.saved.Born[1], err)
		}
	}

	// Each villager answers to his saved id now, on the map and to himself:
	// what every record naming him -- a watch, a chase, the next save's
	// entity list -- will ask.
	for _, p := range pairs {
		if on := engine.Entities()[p.saved.ID]; on != d2interface.MapEntity(p.live) || p.live.ID() != p.saved.ID {
			return refuseLoad(LoadRefusedNatives, "%q (born %v,%v) answers to %q, not his saved id %q",
				p.saved.NameKey, p.saved.Born[0], p.saved.Born[1], p.live.ID(), p.saved.ID)
		}
	}

	for _, p := range pairs {
		if err := restoreMotionExactly(p.live, p.saved.Motion); err != nil {
			return refuseLoad(LoadRefusedNatives, "%s, %q: %v", p.saved.ID, p.saved.NameKey, err)
		}
	}

	return nil
}

// rebuildEntity is step 3's second half, for one entity the map does not
// build: the seam set to its saved id, the entity built by its kind, its id
// checked, its walk and pose put back and read back, and it placed on the
// map. The kind's record is found BEFORE the id is set, so a file naming a
// monster this build does not have leaves nothing waiting.
func (v *Game) rebuildEntity(se d2save.Entity) *LoadRefusal {
	engine := v.gameClient.MapEngine

	if why := heldInFile(se); why != "" {
		return refuseLoad(LoadRefusedEntity, "%s (%s) %s", se.ID, se.Kind, why)
	}

	build, r := v.entityBuilder(se)
	if r != nil {
		return r
	}

	if err := engine.SetNextEntityID(se.ID); err != nil {
		return refuseLoad(LoadRefusedEntity, "%s (%s): %v", se.ID, se.Kind, err)
	}

	e, err := build()
	if err != nil {
		return refuseLoad(LoadRefusedEntity, "%s (%s %s%s): rebuilding it: %v", se.ID, se.Kind, se.Monstat, se.Creature, err)
	}

	// The seam's own promise, checked where it matters: NewNPC and NewCreature
	// WEAR the id. One that spent it would put a monster on the map under a
	// fresh id, and every registry that names him -- his pack, his watch, his
	// chase, his body, his squad -- would name nobody.
	if got := e.ID(); got != se.ID {
		return refuseLoad(LoadRefusedEntity, "%s (%s) was rebuilt as %q: the constructor spent the saved id instead of wearing it", se.ID, se.Kind, got)
	}

	if err := restoreMotionExactly(e, se.Motion); err != nil {
		return refuseLoad(LoadRefusedEntity, "%s (%s): %v", se.ID, se.Kind, err)
	}

	// ITS NAME IS A LABEL, NOT ITS KEY (the B4b review fixes, BUG-80;
	// decision B4b-5 overturned). The entity was matched on its kind and its
	// record -- the bestiary entry by creature_id, the monstat by its key --
	// in entityBuilder. B4b refused a name other than the saved one, so a
	// bestiary label renamed ("Wolf" to "Grey wolf") threw away every save
	// with a wolf in it. A rename is noted and logged, and the pack resumes
	// under the new name.
	if got := d2mapentity.NameKey(e); got != se.NameKey {
		v.loadNote("%s (%s %s%s) was saved named %q and this build names it %q: resumed under the new name",
			se.ID, se.Kind, se.Monstat, se.Creature, se.NameKey, got)
	}

	engine.AddEntity(e)

	return nil
}

// entityBuilder is how the entity se is built, by its kind, with the record
// its kind names -- NewNPC from its monstat, NewCreature from its bestiary
// entry (and the entry's stand-in monstat, as the spawner builds one) -- or
// the refusal when this build has no such record.
func (v *Game) entityBuilder(se d2save.Entity) (func() (rebuiltEntity, error), *LoadRefusal) {
	engine := v.gameClient.MapEngine
	x, y := int(se.Motion.Pos[0]), int(se.Motion.Pos[1])

	switch se.Kind {
	case d2save.KindNPC:
		monstat := v.monstatNamed(se.Monstat)
		if monstat == nil {
			return nil, refuseLoad(LoadRefusedEntity, "%s is an npc of monstat %q, which this build does not have", se.ID, se.Monstat)
		}

		return func() (rebuiltEntity, error) {
			npc, err := engine.NewNPC(x, y, monstat, 0)
			if err != nil {
				return nil, err
			}

			return npc, nil
		}, nil
	case d2save.KindCreature:
		if v.bestiary == nil {
			return nil, refuseLoad(LoadRefusedEntity, "%s is a creature and the game has no bestiary", se.ID)
		}

		entry, ok := v.bestiary.ByID(se.Creature)
		if !ok {
			return nil, refuseLoad(LoadRefusedEntity, "%s is a creature of bestiary entry %q, which this build does not have", se.ID, se.Creature)
		}

		// The stand-in monstat, as gameSpawner.Spawn passes it: it only
		// decides what the construction draws from the world stream, which
		// step 6 puts back after the rebuild. (A unit test's game has no
		// monstats, and builds the creature with none.)
		standIn := v.monstatNamed(entry.StandIn)

		return func() (rebuiltEntity, error) {
			creature, err := engine.NewCreature(x, y, entry.Name, creatureAnimationPaths(entry), 0, standIn)
			if err != nil {
				return nil, err
			}

			creature.SetCreatureID(entry.ID)

			return creature, nil
		}, nil
	}

	return nil, refuseLoad(LoadRefusedEntity, "%s is of kind %q, which no load rebuilds", se.ID, se.Kind)
}

// monstatNamed is this build's monstats record by its Id, or nil.
func (v *Game) monstatNamed(key string) *d2records.MonStatRecord {
	if key == "" || v.asset == nil || v.asset.Records == nil {
		return nil
	}

	return v.asset.Records.Monster.Stats[key]
}

// kindOf is the entity list's kind of a map entity ("" for one it does not
// carry).
func kindOf(e d2interface.MapEntity) string {
	switch e.(type) {
	case *d2mapentity.NPC:
		return d2save.KindNPC
	case *d2mapentity.Creature:
		return d2save.KindCreature
	}

	return ""
}

// heldInFile is why a saved entity's motion cannot be resumed exactly because
// it holds an action, or "" (the B4b review fixes, BUG-76). The file carries
// the held mode and not its frame, so the action would play again from its
// first frame: a death saved half-played fell again after the load. The save
// refuses that moment (Game.heldAction), so no file this build writes holds
// one; a file that does is refused rather than resumed inexactly.
func heldInFile(se d2save.Entity) string {
	if se.Motion.Action == "" {
		return ""
	}

	return fmt.Sprintf("was saved while its %s played: the file carries no animation's frame, so it would play again "+
		"from its first, and a save refuses that moment", se.Motion.Action)
}

// restoreMotionExactly puts a walk and pose back and reads it back: a Motion
// no entity could have had is refused by RestoreMotion before anything
// changes, and one that comes back other than it went in -- a mode the
// sheets or the composite cannot hold, a facing the entity will not take --
// is a monster that would not take the saved step next.
func restoreMotionExactly(e rebuiltEntity, mo d2mapentity.Motion) error {
	if err := e.RestoreMotion(mo); err != nil {
		return err
	}

	if got := e.MotionSnapshot(); !sameMotion(got, mo) {
		return fmt.Errorf("its motion was restored as %+v and saved as %+v", got, mo)
	}

	return nil
}

// sameMotion compares two motions through their JSON, the form the file keeps.
func sameMotion(a, b d2mapentity.Motion) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)

	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}

// checkSquadModels is the squads block's half that needs the map (D3, B4b):
// every deployed squad's model is an NPC on the resumed map -- squadDeployer
// builds each with NewNPC, and step 3 rebuilt it with its saved id. (Squads'
// own Validate has no map to look at: the B2 review's "What the review fixes
// did NOT do".)
func (v *Game) checkSquadModels(snap d2world.SquadsSnapshot) error {
	for _, sq := range snap.Squads {
		for _, m := range sq.Members {
			if m.Entity == d2saveref.Player {
				continue
			}

			e, ok := v.gameClient.MapEngine.Entities()[m.Entity]
			if !ok {
				return fmt.Errorf("%s's model %s is not on the map", sq.ID, m.Entity)
			}

			if _, isNPC := e.(*d2mapentity.NPC); !isNPC {
				return fmt.Errorf("%s's model %s is a %T, and a deployed model is an NPC", sq.ID, m.Entity, e)
			}
		}
	}

	return nil
}

// checkBodies is the bodies block against the resumed map (B4b): the game
// knows no body yet -- a body restored twice is refused -- and every body is
// a monster on the map, an NPC or a creature, at 0 to its maximum.
func (v *Game) checkBodies(bodies []d2save.Body) error {
	if len(v.bodies) != 0 {
		return fmt.Errorf("the game already knows %d monster bodies; a load restores into none", len(v.bodies))
	}

	for _, b := range bodies {
		e, ok := v.gameClient.MapEngine.Entities()[b.ID]
		if !ok {
			return fmt.Errorf("%s has a body and is not on the map", b.ID)
		}

		if kindOf(e) == "" {
			return fmt.Errorf("%s has a body and is a %T", b.ID, e)
		}

		if b.MaxHealth < 1 || b.Health < 0 || b.Health > b.MaxHealth {
			return fmt.Errorf("%s's body is at %d of %d", b.ID, b.Health, b.MaxHealth)
		}
	}

	return nil
}

// restoreBodies puts every monster's health back (B4b): Game.bodies from the
// file, id by id -- the wounded survivor at his wounds, the slain at 0 -- and
// nothing adopted at full health in their place. Checked first, so it can
// never restore over bodies already known.
func (v *Game) restoreBodies(bodies []d2save.Body) error {
	if err := v.checkBodies(bodies); err != nil {
		return err
	}

	v.bodies = make(map[string]*npcBody, len(bodies))

	for _, b := range bodies {
		v.bodies[b.ID] = &npcBody{health: b.Health, maxHealth: b.MaxHealth}
	}

	return nil
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
// entities, which step 3 rebuilt in CreateGame) and the dials a script set
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

	// B4b: every monster's health, as the file has it -- a wounded survivor
	// at his wounds, the slain at 0 -- and none adopted fresh in its place.
	if err := v.restoreBodies(w.Bodies); err != nil {
		return refuseLoad(LoadRefusedBlock, "bodies: %v", err)
	}

	v.loadStep("bodies")

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
	// generation and the villagers' construction drew in Open, and step 3's
	// NewNPC and NewCreature in CreateGame (B4b).
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
// file set aside and his own sidecar put back, at dawn -- on the file's seed
// (the B4a review, C3; before it, the seed this game ran on).
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

	// C3 (the B4a review): the dawn that replaces a refused load runs on the
	// FILE's seed, wherever it was refused. The game ran on it too unless the
	// refusal is SEED, which is exactly the case the file's seed must win.
	seed := v.gameClient.Seed
	if v.pendingLoad != nil {
		seed = v.pendingLoad.Seed
	}

	v.loadAbandoned = true
	v.pendingLoad = nil

	var refusal *LoadRefusal
	if !errors.As(err, &refusal) {
		refusal = refuseLoad(LoadRefusedBlock, "%v", err)
	}

	// His own sidecar goes back first (A1, inside SetLoadAside), then the
	// file is set aside, then the dawn opens on what he had.
	save := v.gameClient.SaveFilePath
	aside := SetLoadAside(save, refusal, true)
	reason := fmt.Sprintf("the world save was not resumed: %v", refusal)

	if aside != "" {
		reason += "; set aside as " + aside
	}

	v.Errorf("LOAD refused, torn down: %s", reason)

	if fb, ok := v.navigator.(loadFallback); ok {
		fb.FallBackToDawn(save, seed, reason)

		return
	}

	if v.navigator != nil {
		v.navigator.ToMainMenu(reason)
	}
}

// loadResumed is a load that restored every block: the report says so, the
// log names the moment and the steps, and the copy of his sidecar step 1 kept
// is let go (A1): the world file's moment is his now.
func (v *Game) loadResumed() {
	forgetPreload(v.gameClient.SaveFilePath)

	savedAt := ""

	updateLastLoad(func(r *LoadReport) {
		r.Resumed = true
		savedAt = r.SavedAt
	})

	v.Infof("LOAD resumed the world saved at %s: %v", savedAt, v.loadSteps)
}

// refuseNetworkReload is rule 9 for the death screen's "load last save" out
// of a network game (the B4a review, C4): the world file beside his save, if
// one is there, is refused NETWORK and set aside before the App reopens him
// as a local game, so the reload begins at dawn with his hero, kit and
// progress -- never at a single-player night the network game did not play.
func refuseNetworkReload(savePath string) {
	if savePath == "" {
		return
	}

	if _, err := os.Stat(d2save.WorldPath(savePath)); err != nil {
		return
	}

	SetLoadAside(savePath, refuseLoad(LoadRefusedNetwork,
		"\"load last save\" out of a network game resumes no world save (rule 9); he begins at dawn, and the saved night is set aside"), false)
}
