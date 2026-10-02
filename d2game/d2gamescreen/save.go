package d2gamescreen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// THE WORLD SAVE'S VERB (M4.6 B3). Josh, 25 Sep 2026: the save "stops at that
// point then resumes at that point". Game.SaveWorld is the one verb every save
// goes through -- the harness's strigoi_save_game now; the escape menu, the
// window's close button and the dawn autosave in B5 -- and it writes three
// files that are one moment:
//
//  1. N.od2.world.json, the world (d2save): every system's B2 snapshot, the
//     entities, the bodies, the hero, the scene, the map, every stream, and a
//     copy of the sidecar;
//  2. N.od2, the hero, through the server's save packet as it always was
//     (made atomic, with a .bak, in d2hero);
//  3. N.od2.strigoi.json, the sidecar: the SAME bytes the world file embeds.
//
// and then re-takes the death screen's "as he entered" copy of the sidecar,
// so that "load last save" after a death means this moment in all three.
//
// IT REFUSES, and a refusal touches no file, while:
//
//   - the game is a network game (NETWORK, rule 9);
//   - there is no hero in the world yet (NOT_READY);
//   - he is dead (DEAD, the 12 Sep ruling: a dead hero is never written);
//   - he is in combat (COMBAT, Josh's ruling of 30 Sep 2026: "you can't save
//     while in combat"): his fight live, a hostile chasing him, his own swing
//     or hit or block reaction playing, or the few seconds after the last of
//     these (combat_status.go);
//   - a fight has not settled (FIGHTING, rule 2): the combat model's own
//     refusal (Combat.Snapshot), the screen's fight-only state -- a strike
//     waiting on a walk, a tactical walk still out, the fight edge not yet
//     taken back. (A monster's or villager's held action -- a swing, a blow
//     taken, a death -- is NOT refused since BUG-87: the file carries it at
//     its frame, and the load resumes it there. The B4b review fixes had
//     refused it, BUG-76. His fight running and his last swing playing were
//     FIGHTING's until the combat status took them: they are COMBAT now.)
//   - he is talking (TALKING);
//   - his journal is open (JOURNAL);
//   - he is choosing his loadout (LOADOUT).
//
// AND IT IS STRICT: before a byte is written, every block it took must pass
// the load's own check of it -- each system's Validate (or, where Validate
// also refuses a system in use, CheckSnapshot) -- and the file's (World.Check,
// then Decode of the bytes). A disagreement is an error, not a refusal: the
// live world holds something no load would take (the B3 review, B2).
//
// IT CHANGES NOTHING IN THE WORLD. No stream is drawn, no minute passes, no
// system is advanced: every snapshot is a read. The one thing a save moves is
// the death screen's copy of his sidecar, which is the point. The rule is
// Rule 10's: the dawn autosave (B5) calls this from inside a frame, so a save
// that moved the world would make a saved night differ from an unsaved one.
// TestSaveResume act 7 holds it to the state digest, before and after.
//
// IT IS NOT IN OnUnload, and cannot be: OnUnload nils the monster bodies and
// closes every world system before it writes the .od2. SAVE AND EXIT (B5)
// calls SaveWorld while the world stands, then leaves.

// The save's refusal codes, the harness's error codes for strigoi_save_game.
const (
	SaveRefusedNetwork  = "NETWORK"
	SaveRefusedNotReady = "NOT_READY"
	SaveRefusedDead     = "DEAD"
	SaveRefusedCombat   = "COMBAT"
	SaveRefusedFighting = "FIGHTING"
	SaveRefusedTalking  = "TALKING"
	SaveRefusedJournal  = "JOURNAL"
	SaveRefusedLoadout  = "LOADOUT"
)

// Two NOT_READY reasons the player is told in words of their own (the B5
// review, C4): the rest of NOT_READY is a game still starting.
const (
	saveReasonNoHeroSave = "this game opened no hero save"
	saveReasonNoKit      = "he has no kit file to save beside him"
)

// ErrSaveRefused is what every refusal wraps: errors.Is(err, ErrSaveRefused).
var ErrSaveRefused = errors.New("the game cannot be saved now")

// ErrBadSaveArgument is what a save refuses its ARGUMENTS with, whatever the
// game is doing: the harness's BAD_ARGUMENT.
var ErrBadSaveArgument = errors.New("the save cannot be made with these arguments")

// errOmitNeedsTo refuses a save that would write a file with a block missing
// over his real save.
var errOmitNeedsTo = fmt.Errorf("%w: a save that omits a block writes a file no load reads, so it must be written somewhere else (to)",
	ErrBadSaveArgument)

// SaveRefusal is one refusal: its code and why.
type SaveRefusal struct {
	Code   string
	Reason string
}

func (r *SaveRefusal) Error() string { return r.Code + ": " + r.Reason }

// Unwrap makes errors.Is(err, ErrSaveRefused) true.
func (r *SaveRefusal) Unwrap() error { return ErrSaveRefused }

// SaveOptions are the harness's (burst B6's negative controls). The zero value
// is the save every caller in the game makes.
type SaveOptions struct {
	// Omit drops these top-level blocks from the world file. It needs To: a
	// file with a block missing is one no load reads, and it must never
	// replace his real save.
	Omit []string

	// To writes the world file here, and ONLY the world file: his .od2, his
	// sidecar, their .bak generations and the death screen's copy are not
	// touched. The world file is self-sufficient -- it embeds the sidecar --
	// so a file written here is a whole save of this moment, beside the real
	// one. A relative path is made absolute (from the working directory) and
	// the result's world_path says where it went; a path that is one of HIS
	// files -- named after his save, in his folder -- is refused
	// (ErrBadSaveArgument): To is "somewhere else" (the B3 review, B5). The
	// harness also keeps it out of the source tree (d2app).
	To string
}

// SaveResult is what a save wrote.
type SaveResult struct {
	WorldPath   string   `json:"world_path"`
	SavePath    string   `json:"save_path"`
	SidecarPath string   `json:"sidecar_path"`
	Written     []string `json:"written"`
	Kept        []string `json:"kept"`
	SetAside    string   `json:"set_aside,omitempty"`
	Omitted     []string `json:"omitted"`

	// PutBack is the world file a save wrote and then put back as it was,
	// because a later file of the same save could not be written (the M4.6
	// B5 review, A2): it is not in Written, and the last save stands. ""
	// when there was nothing to put back.
	PutBack string   `json:"put_back,omitempty"`
	Blocks  []string `json:"blocks"`
	Bytes   int      `json:"bytes"`
}

// saveBuild is the build the world file says wrote it (SetSaveBuild).
//
// nolint:gochecknoglobals // a process-wide fact, set once at launch
var saveBuild struct {
	sync.Mutex
	name string
}

// SetSaveBuild names the build in every world file this process writes: the
// branch and commit the App was made with.
func SetSaveBuild(name string) {
	saveBuild.Lock()
	defer saveBuild.Unlock()

	saveBuild.name = name
}

func buildName() string {
	saveBuild.Lock()
	defer saveBuild.Unlock()

	if saveBuild.name == "" {
		return "unknown"
	}

	return saveBuild.name
}

// uuidStream reports the harness's seeded uuid stream (SetUUIDStream).
//
// nolint:gochecknoglobals // the uuid package's reader is process-global too
var uuidStream struct {
	sync.Mutex
	report func() (seed int64, bytes uint64, ok bool)
}

// SetUUIDStream hands the save a way to read where the harness's seeded uuid
// stream stands (d2app/harness_uuid.go). The shipped game never sets it: its
// ids come from crypto/rand, and the world file's rng.uuid is absent.
func SetUUIDStream(report func() (seed int64, bytes uint64, ok bool)) {
	uuidStream.Lock()
	defer uuidStream.Unlock()

	uuidStream.report = report
}

func readUUIDStream() *d2save.UUIDStream {
	uuidStream.Lock()
	report := uuidStream.report
	uuidStream.Unlock()

	if report == nil {
		return nil
	}

	seed, bytes, ok := report()
	if !ok {
		return nil
	}

	return &d2save.UUIDStream{Seed: seed, Bytes: bytes}
}

// saveProbe, when a test sets it, changes the file a save has assembled just
// before the file is held to the load's checks: the B3 review's B2 probe,
// which corrupts one block at a time and requires the save to refuse it with
// no file touched (TestTheSaveRunsTheLoadsOwnChecks). Nil in every game.
//
// nolint:gochecknoglobals // a test's probe; nothing in the program sets it
var saveProbe func(w *d2save.World)

// writeHeroSave is step 2, his .od2 through the server (Game.OnPlayerSave). A
// unit test swaps it: a unit game has no server, and the B5 review's A2 is a
// step 2 that fails after the world file is written.
//
// nolint:gochecknoglobals // a seam for a unit test, as setAsideWorld is
var writeHeroSave = func(v *Game) error { return v.OnPlayerSave() }

// ErrRefusedFileHeld is a save refused because the world file beside his save
// is one the load refused and could not set aside (the M4.6 B5 review, B2;
// BUG-100): rule 7 says such a file is never overwritten, and it is still
// held, so it cannot be moved out of the way first. Not a refusal (a moment
// cannot cure it): the save fails, and says so.
var ErrRefusedFileHeld = errors.New("the save beside his that could not be read is still held open and cannot be set aside, so it is not written over")

// SaveWorld saves the game: the world file, then his .od2, then his sidecar,
// then the death screen's copy (see the top of this file). A refusal is a
// *SaveRefusal and touches no file.
func (v *Game) SaveWorld(opts SaveOptions) (SaveResult, error) {
	res := SaveResult{Omitted: append([]string{}, opts.Omit...)}

	// A bad call is refused whatever the game is doing.
	if len(opts.Omit) > 0 && opts.To == "" {
		return res, errOmitNeedsTo
	}

	to := ""

	if opts.To != "" {
		abs, err := filepath.Abs(opts.To)
		if err != nil {
			return res, fmt.Errorf("%w: to %q: %v", ErrBadSaveArgument, opts.To, err)
		}

		to = abs
	}

	if r := v.saveRefusal(); r != nil {
		return res, r
	}

	if to != "" && d2hero.IsHeroFile(v.gameClient.SaveFilePath, to) {
		return res, fmt.Errorf("%w: to %s is one of his own files (his save is %s); to writes a world file somewhere else",
			ErrBadSaveArgument, to, v.gameClient.SaveFilePath)
	}

	world, sidecar, err := v.worldFile()
	if err != nil {
		return res, err
	}

	data, err := d2save.Encode(world, opts.Omit)
	if err != nil {
		return res, err
	}

	// STRICT AT SAVE: a whole file is read back as a load would read it
	// before anything is written, so a file no load could read is never one a
	// player is left holding. (A file with a block omitted is meant to be
	// unreadable, and goes only where To says.)
	if len(opts.Omit) == 0 {
		if _, err := d2save.Decode(data); err != nil {
			return res, fmt.Errorf("the world file this save would write is not one a load could read: %w", err)
		}
	}

	res.Bytes = len(data)

	for _, b := range d2save.Blocks {
		if !contains(opts.Omit, b) {
			res.Blocks = append(res.Blocks, b)
		}
	}

	if to != "" {
		w, err := d2save.WriteWorld(to, data)
		res.WorldPath, res.SetAside = to, w.SetAside

		if err != nil {
			return res, err
		}

		res.Written = []string{to}
		res.Kept = nonEmpty(w.Bak)

		v.Infof("SAVE world=%s (to; his save untouched) omitted=%v bytes=%d", to, opts.Omit, len(data))

		return res, nil
	}

	res.SavePath = v.gameClient.SaveFilePath
	res.WorldPath = d2save.WorldPath(res.SavePath)
	res.SidecarPath = v.kitPath

	// 0. A file the load refused and could not move is not written over
	// (rule 7; the B5 review, B2): WriteWorld would keep a readable one as
	// the .bak -- and the save after that would lose it. It is set aside now,
	// under the load's own name for it, or the save is not made.
	if aside, err := setAsideRefusedWorld(res.WorldPath); err != nil {
		return res, err
	} else if aside != "" {
		res.SetAside = aside
	}

	// 1. The world.
	w, err := d2save.WriteWorld(res.WorldPath, data)
	if w.SetAside != "" {
		res.SetAside = w.SetAside
	}

	if err != nil {
		return res, fmt.Errorf("writing the world file: %w", err)
	}

	res.Written = append(res.Written, res.WorldPath)
	res.Kept = append(res.Kept, nonEmpty(w.Bak)...)

	// 2. His .od2, through the server as it always was: a local client's
	// packet is a direct call, so the file is written when this returns, and
	// its error comes back (d2server, M4.6 B3).
	//
	// A STEP THAT FAILS AFTER THE WORLD FILE IS WRITTEN PUTS IT BACK (the B5
	// review, A2; BUG-98): the new world file beside the old sidecar is a
	// torn save, which the next load refuses TORN -- and his last save would
	// survive only as the .bak. So the world file goes back to what WriteWorld
	// replaced, and the three files are one moment again: the last save's.
	if err := writeHeroSave(v); err != nil {
		return v.putBackWorld(res, w, fmt.Errorf("the world file is written and his .od2 is not: %w", err))
	}

	res.Written = append(res.Written, res.SavePath)

	if _, err := os.Stat(d2items.BakPath(res.SavePath)); err == nil {
		res.Kept = append(res.Kept, d2items.BakPath(res.SavePath))
	}

	// 3. His sidecar: the same bytes the world file carries. Failed, the
	// world file goes back too (A2): his .od2 stays the newer moment, as it is
	// after every EXIT WITHOUT SAVING, and the world file carries the hero
	// the load restores.
	if err := d2items.WriteHero(v.kitPath, sidecar); err != nil {
		return v.putBackWorld(res, w, fmt.Errorf("the world file and his .od2 are written and his sidecar is not: %w", err))
	}

	res.Written = append(res.Written, v.kitPath)

	// Every kit save from here writes this save's generation, so the sidecar
	// and the world file agree until the next world save (the B3 review, B7).
	v.saveGeneration = world.SavedAt

	// 4. "Last save" is this moment now, for the death screen too.
	v.snapshotHero()

	v.Infof("SAVE world=%s od2=%s sidecar=%s bytes=%d", res.WorldPath, res.SavePath, v.kitPath, len(data))

	return res, nil
}

// putBackWorld is a save that failed after its world file was written (the B5
// review, A2): the world file is put back as it was (d2save.RestoreWorld), so
// the last save stands, and res says so -- the world file out of Written and
// in PutBack. If it cannot be put back, Written keeps it and the error says
// so: the words a player reads are chosen from Written (saveFailedWords), and
// never say his last save stands when it may not.
func (v *Game) putBackWorld(res SaveResult, w d2save.Written, cause error) (SaveResult, error) {
	if err := d2save.RestoreWorld(w); err != nil {
		v.Errorf("SAVE failed after the world file was written (%v), and it could not be put back: %v", cause, err)

		return res, fmt.Errorf("%w; and the world file could not be put back: %v", cause, err)
	}

	kept := res.Written[:0]

	for _, p := range res.Written {
		if p != res.WorldPath {
			kept = append(kept, p)
		}
	}

	res.Written, res.PutBack = kept, res.WorldPath

	v.Warningf("SAVE failed after the world file was written (%v); the world file is put back as it was, and the last save stands", cause)

	return res, cause
}

// setAsideRefusedWorld is step 0 of a save (the B5 review, B2; BUG-100): the
// world file at worldPath, if the load refused it earlier in this run and
// could not set it aside (ignoredRefusal), is set aside now, under the name
// the load gives that refusal -- it is never kept as the .bak, which the save
// after this one would overwrite. It returns where it went, or "" for any
// other file; still held, the save fails (ErrRefusedFileHeld).
func setAsideRefusedWorld(worldPath string) (string, error) {
	r := ignoredRefusal(worldPath)
	if r == nil {
		return "", nil
	}

	aside, err := setAsideWorld(worldPath, r.Code)
	if err != nil {
		return "", fmt.Errorf("%w (%s, refused %s at the load: %v)", ErrRefusedFileHeld, worldPath, r.Code, err)
	}

	forgetIgnored(worldPath)

	return aside, nil
}

// saveRefusal is why a save cannot be made now, or nil.
func (v *Game) saveRefusal() *SaveRefusal {
	refuse := func(code, format string, args ...interface{}) *SaveRefusal {
		return &SaveRefusal{Code: code, Reason: fmt.Sprintf(format, args...)}
	}

	switch {
	case v.gameClient == nil || v.gameClient.MapEngine == nil:
		return refuse(SaveRefusedNotReady, "no game is running")
	case !v.gameClient.IsSinglePlayer():
		// Rule 9: the TCP handlers race the counted world stream (C13).
		return refuse(SaveRefusedNetwork, "a network game is not saved")
	case v.gameClient.SaveFilePath == "":
		return refuse(SaveRefusedNotReady, saveReasonNoHeroSave)
	case v.localPlayer == nil || v.localPlayer.Stats == nil:
		return refuse(SaveRefusedNotReady, "he is not in the world yet")
	case v.pendingLoad != nil || v.loadAbandoned:
		// M4.6 B4a: a world save still being resumed, or one refused on its
		// first frame, is not a moment anyone could save.
		return refuse(SaveRefusedNotReady, "the world save is still being resumed, or was refused")
	case v.died || v.heroDead():
		return refuse(SaveRefusedDead, "he is dead, and a dead hero is never saved")
	}

	// The combat status (Josh, 30 Sep 2026): his fight, a hostile after him,
	// his own swing or reaction, and the grace after them (combat_status.go).
	if code, detail := v.CombatReason(); code != "" {
		return refuse(SaveRefusedCombat, "he is in combat (%s): %s", code, detail)
	}

	if why := v.fightUnsettled(); why != "" {
		return refuse(SaveRefusedFighting, "%s", why)
	}

	switch {
	case v.talk != nil:
		return refuse(SaveRefusedTalking, "he is talking")
	case v.journalOpen:
		return refuse(SaveRefusedJournal, "his journal is open")
	case v.choosingLoadout:
		return refuse(SaveRefusedLoadout, "he is choosing his loadout")
	case v.kit == nil || v.kitPath == "":
		return refuse(SaveRefusedNotReady, saveReasonNoKit)
	case v.worldClock == nil || v.light == nil || v.squads == nil || v.spawns == nil || v.spawner == nil ||
		v.notice == nil || v.pursuit == nil || v.corpses == nil || v.rising == nil || v.combat == nil ||
		v.seek == nil:
		return refuse(SaveRefusedNotReady, "the world's systems are not all built")
	}

	return nil
}

// fightUnsettled is why the fight is not over as far as a save is concerned,
// or "". Every clause is state that exists only in a fight and is empty
// whenever one is not running and its close has been applied: the save
// refuses rather than carry it (the plan's section 1, "UI and fight state").
//
// THE COMBAT STATUS TOOK TWO OF ITS CLAUSES (30 Sep 2026): "a fight is
// running" (Combat.Fighting) and "his last swing is still playing"
// (Player.IsCasting) were the same thing as being in combat, which COMBAT
// refuses before this is asked -- and for the grace after them, so both are
// COMBAT now, never FIGHTING. What is left here is the fight's own
// bookkeeping, which a frame or two after his fight clears; the grace is
// longer than any of it measured, so in the game FIGHTING is a backstop (and
// the whole of it with the harness's save.combat_grace at 0).
//
// The combat model's own refusal is here, with the screen's (the B3 review:
// it used to come after TALKING, JOURNAL and LOADOUT, from inside the
// snapshots, so a talk opened on a fight's closing frame was refused TALKING
// while the fight's experience waited). Combat.WorldHeld was here too, and
// could never fire: it is false whenever no encounter runs, which Fighting,
// the clause before it, already refused.
func (v *Game) fightUnsettled() string {
	unsettled := ""
	if v.combat != nil {
		unsettled = v.combatUnsettled()
	}

	switch {
	case unsettled != "":
		return unsettled
	case v.wasFighting:
		return "the fight's end has not been applied yet"
	case v.pendingStrike != "":
		return "a strike is waiting on his walk to " + v.pendingStrike
	case len(v.tacticalPace) > 0 || len(v.tacticalReserved) > 0:
		return "a tactical walk is still out"
	case v.journalFight != "":
		return "the journal is still reading a fight"
	}

	return ""
}

// combatUnsettled is the combat model's own word that a fight is not over --
// its experience or paced minutes not yet taken, the pace window open -- or
// "". Combat.Snapshot is a read; its refusal is the only thing asked of it.
func (v *Game) combatUnsettled() string {
	if _, err := v.combat.Snapshot(); err != nil {
		return err.Error()
	}

	return ""
}

// worldFile takes every snapshot and assembles the file, and the sidecar
// document it embeds, then holds every block to the load's own checks. Every
// call in it is a read.
func (v *Game) worldFile() (*d2save.World, json.RawMessage, error) {
	r := worldResolver{v}

	// The file's moment, and the generation its sidecar carries (B7).
	savedAt := time.Now().UTC().Format(time.RFC3339Nano)

	combat, err := v.combat.Snapshot()
	if err != nil {
		// fightUnsettled asked first; this is its word again, should a fight
		// ever start between the two.
		return nil, nil, &SaveRefusal{Code: SaveRefusedFighting, Reason: err.Error()}
	}

	// Spawns and Notice also refuse while a sleep runs (sheltered, hidden),
	// but a sleep runs inside one talk answer's call and is over when it
	// returns, so no caller of SaveWorld -- between frames, or from the menu
	// -- can find one running: an error here is the snapshot's own.
	spawns, err := v.spawns.Snapshot(r)
	if err != nil {
		return nil, nil, fmt.Errorf("spawns: %w", err)
	}

	notice, err := v.notice.Snapshot(r)
	if err != nil {
		return nil, nil, fmt.Errorf("notice: %w", err)
	}

	pursuit, err := v.pursuit.Snapshot(r)
	if err != nil {
		return nil, nil, fmt.Errorf("pursuit: %w", err)
	}

	sidecar, err := v.heroBytes(savedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("his sidecar: %w", err)
	}

	entities, err := v.saveEntities()
	if err != nil {
		return nil, nil, err
	}

	engine := v.gameClient.MapEngine
	mapPath, mapSHA := d2mapgen.HostMap()
	x, y := v.localPlayer.GetPositionF()

	w := &d2save.World{
		Version: d2save.Version,
		Build:   buildName(),
		SavedAt: savedAt,
		Map:     d2save.Map{Path: mapPath, SHA: mapSHA, Generated: mapPath == ""},
		Seed:    v.gameClient.Seed,
		RNG: d2save.RNG{
			World:  d2rand.StreamState{Seed: engine.RandSeed(), Draws: engine.RandDraws()},
			Spawns: spawns.RNG,
			Combat: combat.RNG,
			// The raid's R1: the fights he is not in, combat's second stream.
			CombatClock: combat.Clock.RNG,
			UUID:        readUUIDStream(),
		},
		Hero: d2save.Hero{
			Name:  v.localPlayer.Name(),
			Class: v.localPlayer.Class.String(),
			X:     x, Y: y,
			Pos:     [2]float64{v.localPlayer.Position.X(), v.localPlayer.Position.Y()},
			Facing:  v.localPlayer.Facing(),
			Health:  v.localPlayer.Stats.Health,
			Stamina: v.localPlayer.Stats.Stamina,
			Run:     v.localPlayer.IsRunToggled(),
		},
		Sidecar:  sidecar,
		Clock:    v.worldClock.Snapshot(),
		Light:    v.light.Snapshot(),
		Squads:   v.squads.Snapshot(),
		Spawns:   spawns,
		Spawner:  d2save.Spawner{Arrival: v.spawner.Snapshot().Arrival},
		Notice:   notice,
		Pursuit:  pursuit,
		Seek:     v.seek.Snapshot(),
		Corpses:  v.corpses.Snapshot(),
		Rising:   v.rising.Snapshot(),
		Combat:   combat,
		Bodies:   v.saveBodies(),
		Entities: entities,
		Scene: d2save.Scene{
			WatchStood:    v.watchStood,
			FieldDead:     append([]string{}, v.fieldDead...),
			DawnPaidDay:   v.dawnPaidDay,
			LastStage:     v.lastStage.String(),
			WatchClock:    v.watchClock,
			WatchClockSet: v.watchClockSet,
		},
	}

	w.RNG.Rising = w.Rising.RNG

	if saveProbe != nil {
		saveProbe(w)
	}

	if err := v.validateSnapshots(w, r); err != nil {
		return nil, nil, err
	}

	if err := w.Check(); err != nil {
		return nil, nil, fmt.Errorf("the world as it stands would not make a file a load could read: %w", err)
	}

	return w, sidecar, nil
}

// validateSnapshots holds every block this save took to the check the load
// will make of it (the B3 review, B2; D4's Validate twins): Clock, Light,
// Rising and Combat by their Validate, and Squads, Corpses, Spawns, Notice
// and Pursuit by CheckSnapshot -- Validate's checks without its refusal of a
// system in use, which the live system this save read always is -- Spawns,
// Notice and Pursuit through the save's own Resolver over the live map, as
// the load's goes over the resumed one. And the spawner's arrival, and the
// clock fights against the watches and chases beside them
// (d2world.CheckClockWatches, BUG-73), as the load's step 4 checks them.
//
// A refusal is the live world holding something no load would take -- a lit
// torch at no minutes, two carried sources, a source id past next_id -- and
// no file is touched: the harness reports it INTERNAL with the system and
// the reason. Called one by one, never through method values (the reach
// gate's lesson in World.Check).
func (v *Game) validateSnapshots(w *d2save.World, r d2world.Resolver) error {
	seed := v.gameClient.Seed

	refused := func(block string, err error) error {
		return fmt.Errorf("the %s block this save took is one the load would refuse: %w", block, err)
	}

	if err := v.worldClock.Validate(w.Clock); err != nil {
		return refused("clock", err)
	}

	if err := v.light.Validate(w.Light); err != nil {
		return refused("light", err)
	}

	if err := v.squads.CheckSnapshot(w.Squads); err != nil {
		return refused("squads", err)
	}

	if err := v.corpses.CheckSnapshot(w.Corpses); err != nil {
		return refused("corpses", err)
	}

	if err := v.rising.Validate(w.Rising, seed, w.Corpses); err != nil {
		return refused("rising", err)
	}

	if err := v.combat.Validate(w.Combat, seed); err != nil {
		return refused("combat", err)
	}

	if err := v.spawns.CheckSnapshot(w.Spawns, r, seed); err != nil {
		return refused("spawns", err)
	}

	if err := v.spawner.validate(SpawnerSnapshot{Arrival: w.Spawner.Arrival}); err != nil {
		return refused("spawner", err)
	}

	if err := v.notice.CheckSnapshot(w.Notice, r); err != nil {
		return refused("notice", err)
	}

	if err := v.pursuit.CheckSnapshot(w.Pursuit, r); err != nil {
		return refused("pursuit", err)
	}

	// The raid's R2: Seek's rows and stand-ins, which resolve nothing.
	if err := v.seek.CheckSnapshot(w.Seek); err != nil {
		return refused("seek", err)
	}

	// BUG-73: the load's cross-check of the combat block's clock fights
	// against the watches and chases beside them (checkLoad), made on the
	// file this save assembled. The live game never refuses it: it is the
	// rule pruneOrEnd keeps at the end of every clock step.
	if err := d2world.CheckClockWatches(w.Combat, w.Notice, w.Pursuit); err != nil {
		return refused("combat", err)
	}

	return nil
}

// heroBytes is his sidecar document as saveKit writes it, of the given world
// save generation (saveKit passes the last save's, SaveWorld its own: B7). A
// torch he holds lit keeps its minutes in the light model, and the kit's copy
// reads zero while it burns (the L key), so the minutes are written in for
// the file and taken back out: the live kit still owns nothing the light
// model owns.
func (v *Game) heroBytes(generation string) ([]byte, error) {
	if v.kit == nil {
		return nil, errors.New("he has no kit")
	}

	_, torch, isTorch := v.kit.OffHandTorch()
	held := 0.0

	if isTorch && v.light != nil {
		if carried := v.light.Carried(); carried != nil && torch.BurnLeft == 0 {
			held = carried.Burn
			torch.BurnLeft = held
		}
	}

	data, err := d2items.HeroBytes(v.kit, d2items.Extras{
		Progress: v.progressJSON(), Village: v.standingJSON(), Land: v.landJSON(), Journal: v.journalJSON(),
		Generation: generation,
	})

	if held > 0 {
		torch.BurnLeft = 0
	}

	return data, err
}

// saveBodies is every monster's health, sorted by id.
func (v *Game) saveBodies() []d2save.Body {
	ids := make([]string, 0, len(v.bodies))
	for id := range v.bodies {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	out := make([]d2save.Body, 0, len(ids))

	for _, id := range ids {
		if b := v.bodies[id]; b != nil {
			out = append(out, d2save.Body{ID: id, Health: b.health, MaxHealth: b.maxHealth})
		}
	}

	return out
}

// saveEntities is every NPC and creature on the map, sorted by id, with its
// motion. The player is not among them (his id is his connection's), nor are
// items, missiles and objects, which the save does not carry.
func (v *Game) saveEntities() ([]d2save.Entity, error) {
	all := v.gameClient.MapEngine.Entities()

	ids := make([]string, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	// Which live entity is which native: by the entity itself, not its id,
	// which B4b's re-key changes (the B3 review, B3). A pair two entities
	// share could not be told apart on load: refused.
	nativeOf := make(map[d2interface.MapEntity]nativeKey, len(v.natives))

	for k, e := range v.natives {
		if e == nil {
			return nil, fmt.Errorf("two entities the map built are both %q at %v,%v: a load re-keys a native by its "+
				"name_key and where the map put it, and could not tell them apart", k.nameKey, k.x, k.y)
		}

		nativeOf[e] = k
	}

	out := make([]d2save.Entity, 0, len(ids))

	for _, id := range ids {
		e := all[id]

		var se d2save.Entity

		switch t := e.(type) {
		case *d2mapentity.NPC:
			monstat := t.MonStat()
			if monstat == nil {
				return nil, fmt.Errorf("entity %s is an NPC with no monstat, which no load could rebuild", id)
			}

			se = d2save.Entity{Kind: d2save.KindNPC, Monstat: monstat.Key, Motion: t.MotionSnapshot()}
		case *d2mapentity.Creature:
			if t.CreatureID() == "" {
				return nil, fmt.Errorf("entity %s is a creature built from no bestiary entry, which no load could rebuild", id)
			}

			se = d2save.Entity{Kind: d2save.KindCreature, Creature: t.CreatureID(), Motion: t.MotionSnapshot()}
		default:
			continue
		}

		se.ID = e.ID()
		se.NameKey = d2mapentity.NameKey(e)
		se.X, se.Y = e.GetPositionF()

		if k, ok := nativeOf[e]; ok {
			se.Native = true
			se.Born = &[2]float64{k.x, k.y}
		}

		out = append(out, se)
	}

	return out, nil
}

// nativeKey is how a native is known across a load: who stands in for him and
// where the map put him (world tiles). B4b re-keys a rebuilt map's villager to
// his saved id by it (B3-6).
type nativeKey struct {
	nameKey string
	x, y    float64
}

// nativesOf is every NPC and creature on the map as the game screen is made:
// the ones the map built -- the villagers -- which a load's map build makes
// again, and which it re-keys rather than rebuilds (B4b). Each is recorded by
// its key and held by the ENTITY, not its id (the B3 review, B3): B4b's
// re-key gives a villager his saved id, and a record by id would lose him --
// the next save would mark no villager native. A key two entities share maps
// to nil, and the save refuses it (saveEntities): the load could not tell the
// two apart. B4b must re-key in place -- the same entity, a new id -- or
// capture the natives again after it rebuilds them.
func nativesOf(engine *d2mapengine.MapEngine) map[nativeKey]d2interface.MapEntity {
	out := map[nativeKey]d2interface.MapEntity{}

	if engine == nil {
		return out
	}

	for _, e := range engine.Entities() {
		switch e.(type) {
		case *d2mapentity.NPC, *d2mapentity.Creature:
		default:
			continue
		}

		x, y := e.GetPositionF()
		k := nativeKey{nameKey: d2mapentity.NameKey(e), x: x, y: y}

		if _, dup := out[k]; dup {
			out[k] = nil

			continue
		}

		out[k] = e
	}

	return out
}

// worldResolver is the game's d2world.Resolver over the LIVE map: an entity id
// is the entity on the map with that id, as the chaser the live path wraps it
// in (startChasesForTheAware, Game.Watch); the word "player" is the local
// player, as the prey the spawn tables watch him as. The save uses it to name
// the player and to decide which pack members are gone; the load (B4b) must
// build its Resolver the same way over the resumed map (B2b notes).
type worldResolver struct{ v *Game }

func (r worldResolver) Watcher(id string) (d2world.Watcher, bool) {
	w, ok := r.walker(id)
	if !ok {
		return nil, false
	}

	return chaser{entity: w}, true
}

func (r worldResolver) Quarry(ref string) (d2world.Quarry, bool) {
	if ref == d2world.PlayerRef {
		p := r.v.thePlayer()
		if p == nil {
			return nil, false
		}

		return prey{entity: p}, true
	}

	w, ok := r.walker(ref)
	if !ok {
		return nil, false
	}

	return prey{entity: w}, true
}

// thePlayer is the local player: the one the controls are bound to, or,
// before they are (a load's step 4 runs at the end of CreateGame, and the
// controls bind on the first frame: M4.6 B4b), the client's own player -- the
// same entity bindGameControls will bind.
func (v *Game) thePlayer() *d2mapentity.Player {
	if v.localPlayer != nil {
		return v.localPlayer
	}

	if v.gameClient == nil {
		return nil
	}

	for _, p := range v.gameClient.Players {
		if p != nil && p.ID() == v.gameClient.PlayerID {
			return p
		}
	}

	return nil
}

func (r worldResolver) walker(id string) (pathWalker, bool) {
	if id == "" || id == d2world.PlayerRef || r.v.gameClient == nil || r.v.gameClient.MapEngine == nil {
		return nil, false
	}

	e, ok := r.v.gameClient.MapEngine.Entities()[id]
	if !ok {
		return nil, false
	}

	w, ok := e.(pathWalker)

	return w, ok
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}

	return false
}

func nonEmpty(s ...string) []string {
	out := []string{}

	for _, x := range s {
		if x != "" {
			out = append(out, x)
		}
	}

	return out
}
