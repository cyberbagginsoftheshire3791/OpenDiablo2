//go:build playtest

package playtest

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
)

// M4.6 B5: THE SAVE REACHES THE PLAYER (29 Sep 2026). Josh, 25 Sep: the save
// "stops at that point then resumes at that point". B1-B4b built the file,
// the verb and the load, and only the harness could make a save; these
// scripts make it the way he does -- the escape menu, the window's close, the
// dawn -- and relaunch, load, and require the game to be the moment it was
// saved at (the resume digest, as TestSaveResume compares it).
//
//	TestTheMenuSaves             Esc, SAVE GAME: "Game saved.", the three files,
//	                             the moment unmoved; a relaunch resumes it. In a
//	                             fight the menu says it cannot save and offers EXIT WITHOUT SAVING, which
//	                             writes no world file, and he comes back to his
//	                             last save. SAVE AND EXIT GAME saves and leaves.
//	                             (Since the combat status, 30 Sep 2026, the
//	                             fight's refusal is COMBAT: "You can't save in
//	                             combat.")
//	                             His .od2 read-only: SAVE GAME fails, puts the
//	                             world file back, and says his last save stands
//	                             -- which the next load shows is true (A2).
//	TestTheDawnAutosave          the first dawn he lives to see saves the game on
//	                             its own frame; a relaunch resumes that moment,
//	                             and the resumed dawn does not save again. A save
//	                             before the dawn, resumed, crosses the dawn with
//	                             its own in-frame autosave and is the original
//	                             game a second after dawn (B3).
//	TestTheDawnAutosaveWaitsOutAFight
//	                             a fight across first light: the autosave is
//	                             PENDING through every refused frame, and taken
//	                             the first frame after the fight has settled; a
//	                             relaunch resumes the moment it was taken.
//	TestTheCloseHook             the graceful quit (the window's close) at 20:00
//	                             saves and a relaunch resumes it; closed in a
//	                             fight, it leaves unsaved, promptly, and he
//	                             comes back to his last save. Closed with a talk
//	                             open, with his journal open, and in the moment
//	                             after a fight, it saves (A1).
//	TestTheLoadNotice            a world file this build cannot read is set aside
//	                             (kept, never deleted) and he wakes at dawn, told
//	                             so; a save with a villager gone says so; a save
//	                             cut off between its files resumes the save
//	                             before it, the .bak, and says so (A2).
//
// Every script's hero lives in the test's own %APPDATA% (the launcher's
// testHome), never Josh's saves.

// The words he reads (d2player's strigoi_strings.go), as he reads them.
const (
	b5GameSaved = "Game saved."
	// The combat status (30 Sep 2026) took FIGHTING's "his fight is running"
	// case: in his fight he is in combat, and the save is refused COMBAT.
	b5CantInAFight  = "You can't save in combat."
	b5ExitUnsaved   = "EXIT WITHOUT SAVING"
	b5SaveAndExit   = "SAVE AND EXIT GAME"
	b5SaveGame      = "SAVE GAME"
	b5DawnSaved     = "Dawn. The game is saved."
	b5DawnSavedLate = "The dawn's save is made."
	b5WakeAtDawn    = "You wake at dawn. The save is kept, set aside -- not deleted."
	b5OtherVersion  = "Your save is from another version of the game."
	b5VillagerGone  = "Your save is restored. A villager who was gone when you saved is gone again."

	// The B5 review fixes.
	b5SaveFailed      = "The game could not be saved: its files could not be written.\nYour last save stands."
	b5LeaveToLastSave = "Leave now and you come back to your last save."
	b5CutOff          = "Your save was cut off while it was being written."
	b5BakResumed      = "The save before it is restored. The cut-off save is kept, set aside -- not deleted."
)

// b5Dials are the dials every B5 script sets as its game begins, and again
// after every load (a dial is never saved): no pack, no risen man.
var b5Dials = []dialWrite{
	{"spawns", "chance", 0},
	{"rising", "edge_floor", 0},
	{"rising", "p", 0.0},
}

func b5SetDials(s *session, dials []dialWrite) {
	for _, d := range dials {
		setField(s, d.System, d.Field, d.Value)
	}
}

// b5Start begins a hero on seed 1462, paused, with the dials, and returns his
// .od2's path.
func b5Start(t *testing.T, s *session, name string) string {
	t.Helper()

	s.call("strigoi_pause", map[string]any{})
	g := s.call("strigoi_start_game", map[string]any{
		"hero_name": name, "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	b5SetDials(s, b5Dials)

	save := str(g, "save_path")
	if save == "" {
		t.Fatalf("start_game returned no save_path: %v", g)
	}

	return save
}

// b5Resume starts his save in the game s is attached to, requires the load to
// have resumed the world file of savedAt, and sets dials again.
func b5Resume(t *testing.T, s *session, act, save, savedAt string, dials []dialWrite) {
	t.Helper()

	s.call("strigoi_pause", map[string]any{})
	g := s.call("strigoi_start_game", map[string]any{"save_path": save, "wait_seconds": 90})

	load := sub(g, "load")
	if !flag(t, load, "resumed") || str(load, "saved_at") != savedAt {
		t.Fatalf("%s: start_game resumes the world file saved at %s: %v", act, savedAt, load)
	}

	b5SetDials(s, dials)
}

func saveState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "save"}), "state")
}

func autosaveOf(s *session) map[string]any { return sub(saveState(s), "autosave") }

func lastSaveOf(s *session) map[string]any { return sub(saveState(s), "last_save") }

func escapeMenuOf(s *session) map[string]any { return sub(uiState(s), "escape_menu") }

// b5Files are his three files: the world file, his .od2, his sidecar.
func b5Files(save string) []string {
	return []string{d2save.WorldPath(save), save, save + ".strigoi.json"}
}

// b5SavedAt is a world file's saved_at, and requires his sidecar to be of its
// generation (B3's pairing: one moment in both).
func b5SavedAt(t *testing.T, act, save string) string {
	t.Helper()

	file := worldFile(t, act, mustRead(t, d2save.WorldPath(save)))
	at := str(file, "saved_at")

	if at == "" {
		t.Fatalf("%s: the world file has no saved_at", act)
	}

	if gen := generationOf(t, mustRead(t, save+".strigoi.json")); gen != at {
		t.Fatalf("%s: his sidecar is of generation %q, the world file saved at %q: a torn save", act, gen, at)
	}

	return at
}

// menuPick opens the escape menu (when it is not open), moves the keys to the
// entry and presses Enter.
func menuPick(t *testing.T, s *session, act, entry string) {
	t.Helper()

	if !flag(t, uiState(s), "escape_menu_open") {
		s.call("strigoi_key", map[string]any{"key": "escape"})
	}

	for i := 0; i < 8; i++ {
		m := escapeMenuOf(s)
		if !flag(t, m, "open") || str(m, "layout") != "main" {
			t.Fatalf("%s: the escape menu is not open on its main entries: %v", act, m)
		}

		entries := stringsOf(m["entries"])
		at, want := -1, -1

		for j, e := range entries {
			if e == str(m, "selected") {
				at = j
			}

			if e == entry {
				want = j
			}
		}

		switch {
		case want < 0:
			t.Fatalf("%s: the menu has no %q: %v", act, entry, entries)
		case at == want:
			s.call("strigoi_key", map[string]any{"key": "enter"})
			return
		case at > want:
			s.call("strigoi_key", map[string]any{"key": "up"})
		default:
			s.call("strigoi_key", map[string]any{"key": "down"})
		}
	}

	t.Fatalf("%s: the keys never reached %q: %v", act, entry, escapeMenuOf(s))
}

// b5Fight lights his torch -- an unlit man is not seen at night or at dusk
// (B4b's measurement: a dog one tile away never noticed him) -- puts a dog one
// tile from him, and steps frames until the fight opens.
func b5Fight(t *testing.T, s *session) {
	t.Helper()

	if !flag(t, lightState(s), "carried_lit") {
		s.call("strigoi_key", map[string]any{"key": "l"})

		if !flag(t, lightState(s), "carried_lit") {
			t.Fatalf("L lights his torch (the default loadout carries one): %v", lightState(s))
		}
	}

	p := s.call("strigoi_get_player", map[string]any{})
	spot := clearNeighbour(t, s, num(p, "x"), num(p, "y"))
	dog := spawnNPC(t, s, "fallen1", spot[0], spot[1])

	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": str(p, "handle")})
	fightNow(t, s)
}

// b5Relaunch ends this process and starts another in the same home.
func b5Relaunch(t *testing.T, s *session) *session {
	t.Helper()

	s.stop()

	return start(t)
}

func TestTheMenuSaves(t *testing.T) {
	s := start(t)
	save := b5Start(t, s, "Menu")
	files := b5Files(save)

	s.call("strigoi_step_world", map[string]any{"world_minutes": 30})

	// --- act 1: Esc, SAVE GAME ------------------------------------------------
	// The moment, before the menu opens (the menu itself is in the digest's
	// world part as escape_menu_open and world_held_by, and it is closed again
	// once SAVE GAME has saved).
	sBefore := snapWorld(t, s)

	s.call("strigoi_key", map[string]any{"key": "escape"})
	m := escapeMenuOf(s)

	if got := strings.Join(stringsOf(m["entries"]), "|"); got != "OPTIONS|"+b5SaveGame+"|"+b5SaveAndExit+"|RETURN TO GAME" {
		t.Fatalf("act 1: the menu's entries are %q", got)
	}

	if str(m, "note") != "" || flag(t, m, "exit_refused") {
		t.Fatalf("act 1: a savable moment: no refusal line, and SAVE AND EXIT GAME: %v", m)
	}

	shot := s.call("strigoi_screenshot", map[string]any{"name": "b5-menu-savable"})
	t.Logf("act 1: the menu, savable: %v", shot["path"])

	menuPick(t, s, "act 1", b5SaveGame)

	ui := uiState(s)
	if flag(t, ui, "escape_menu_open") {
		t.Fatalf("act 1: SAVE GAME saves and returns him to the game: %v", escapeMenuOf(s))
	}

	if got := str(ui, "save_notice"); got != b5GameSaved {
		t.Fatalf("act 1: the notice is %q, want %q", got, b5GameSaved)
	}

	if ls := lastSaveOf(s); str(ls, "by") != "menu" || str(ls, "result") != "saved" || num(saveState(s), "saves") != 1 {
		t.Fatalf("act 1: the save provider's last save: %v (saves %v)", ls, saveState(s)["saves"])
	}

	savedT := b5SavedAt(t, "act 1", save)
	sT := snapWorld(t, s)
	sameWorld(t, "act 1: the menu's save moved nothing", sBefore, sT)
	t.Logf("act 1 PASS: SAVE GAME wrote %s (saved_at %s), \"Game saved.\" shown, the world unmoved", files[0], savedT)

	// --- act 2: relaunch, load: S_R0 = S_T -------------------------------------
	s = b5Relaunch(t, s)
	b5Resume(t, s, "act 2", save, savedT, b5Dials)
	sameWorld(t, "act 2 (S_R0 = S_T, the menu's save)", sT, snapWorld(t, s))
	t.Logf("act 2 PASS: relaunched, resumed; the menu's moment, resume digest %.12s", sT.Resume)

	// --- act 3: in a fight the menu says so, and offers EXIT WITHOUT SAVING -----
	b5Fight(t, s)

	s.call("strigoi_key", map[string]any{"key": "escape"})
	m = escapeMenuOf(s)

	if !strings.HasPrefix(str(m, "note"), b5CantInAFight) || !flag(t, m, "exit_refused") {
		t.Fatalf("act 3: in a fight the menu says %q and its exit leaves unsaved: %v", b5CantInAFight, m)
	}

	if got := strings.Join(stringsOf(m["entries"]), "|"); !strings.Contains(got, b5ExitUnsaved) || strings.Contains(got, b5SaveAndExit) {
		t.Fatalf("act 3: the exit entry reads %q in a fight: %q", b5ExitUnsaved, got)
	}

	shot = s.call("strigoi_screenshot", map[string]any{"name": "b5-menu-refused"})
	t.Logf("act 3: the menu, refused: %v; note %q", shot["path"], str(m, "note"))

	before := hashFiles(t, files)
	menuPick(t, s, "act 3", b5SaveGame)

	if !flag(t, uiState(s), "escape_menu_open") {
		t.Fatal("act 3: a refused SAVE GAME keeps the menu up")
	}

	if ls := lastSaveOf(s); str(ls, "result") != "refused" || str(ls, "code") != "COMBAT" || str(ls, "words") != b5CantInAFight {
		t.Fatalf("act 3: the refused save is recorded with its words: %v", ls)
	}

	if !equalHashes(before, hashFiles(t, files)) {
		t.Fatal("act 3: a refused save touched a file")
	}

	menuPick(t, s, "act 3", b5ExitUnsaved)
	awaitMenu(t, s)

	if got := hashFiles(t, files[:1]); !equalHashes(got, map[string]string{files[0]: before[files[0]]}) {
		t.Fatal("act 3: EXIT WITHOUT SAVING wrote the world file")
	}

	t.Logf("act 3 PASS: in a fight the menu said %q, SAVE GAME wrote nothing, EXIT WITHOUT SAVING left the world file as it was", b5CantInAFight)

	// --- act 4: ...and he comes back to his last save --------------------------
	b5Resume(t, s, "act 4", save, savedT, b5Dials)
	sameWorld(t, "act 4 (back to his last save)", sT, snapWorld(t, s))
	t.Log("act 4 PASS: after leaving unsaved he resumes his last save, S = S_T")

	// --- act 5: SAVE AND EXIT GAME ---------------------------------------------
	s.call("strigoi_step_world", map[string]any{"world_minutes": 20})
	sX := snapWorld(t, s)
	menuPick(t, s, "act 5", b5SaveAndExit)
	awaitMenu(t, s)

	savedX := b5SavedAt(t, "act 5", save)
	if savedX == savedT {
		t.Fatal("act 5: SAVE AND EXIT GAME wrote no new save")
	}

	b5Resume(t, s, "act 5", save, savedX, b5Dials)
	sameWorld(t, "act 5 (SAVE AND EXIT GAME resumes)", sX, snapWorld(t, s))
	t.Logf("act 5 PASS: SAVE AND EXIT GAME saved (saved_at %s) and left; the load is that moment", savedX)

	// --- act 6: his .od2 cannot be written: the world file is put back ---------
	// The B5 review, A2 (BUG-98). His .od2 read-only (a sync client or a
	// scanner holding it does the same): SAVE GAME writes the world file,
	// fails at his .od2, and puts the world file back as it was, so the three
	// files are still his last save's -- and "Your last save stands" is true.
	// Before the fix the world file stayed the new one beside the old sidecar,
	// the next load was refused TORN, and he woke at dawn, his last save only
	// in the .bak (the review's probe B).
	s.call("strigoi_step_world", map[string]any{"world_minutes": 20})

	before = hashFiles(t, files)

	if err := os.Chmod(save, 0o444); err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.Chmod(save, 0o644) }()

	menuPick(t, s, "act 6", b5SaveGame)
	m = escapeMenuOf(s)

	if ls := lastSaveOf(s); str(ls, "result") != "failed" {
		t.Fatalf("act 6: the save failed: %v", ls)
	}

	if !equalHashes(before, hashFiles(t, files)) {
		t.Fatalf("act 6: the failed save left his files changed -- the world file must be put back as it was:\n before %v\n after  %v",
			before, hashFiles(t, files))
	}

	if got := b5SavedAt(t, "act 6", save); got != savedX {
		t.Fatalf("act 6: the world file is still his last save (%s), not %s", savedX, got)
	}

	if note := str(m, "note"); !flag(t, m, "open") || !flag(t, m, "exit_refused") ||
		!strings.HasPrefix(note, b5SaveFailed) || !strings.HasSuffix(note, b5LeaveToLastSave) {
		t.Fatalf("act 6: a failed save keeps the menu up and says his last save stands -- true, since it was put back: %v", m)
	}

	menuPick(t, s, "act 6", b5ExitUnsaved)
	awaitMenu(t, s)

	if err := os.Chmod(save, 0o644); err != nil {
		t.Fatal(err)
	}

	b5Resume(t, s, "act 6", save, savedX, b5Dials)
	sameWorld(t, "act 6 (a failed save: his last save, not TORN)", sX, snapWorld(t, s))
	t.Logf("act 6 PASS: his .od2 read-only, SAVE GAME failed and put the world file back (%q); the next load resumed his last save", str(m, "note"))
}

// b5ToDawn steps frames, one at a time, until the autosave leaves idle (the
// dawn armed it), and returns how many.
func b5ToDawn(t *testing.T, s *session, act string) int {
	t.Helper()

	for i := 1; i <= 600; i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})

		if str(autosaveOf(s), "state") != "idle" {
			return i
		}
	}

	t.Fatalf("%s: no dawn armed the autosave in 600 frames: clock %v, autosave %v", act, clockState(s), autosaveOf(s))

	return 0
}

func TestTheDawnAutosave(t *testing.T) {
	s := start(t)
	save := b5Start(t, s, "Dawn")

	if a := autosaveOf(s); str(a, "state") != "idle" || !flag(t, a, "enabled") {
		t.Fatalf("the premise: the game begins at dawn and no dawn has armed an autosave: %v", a)
	}

	// The first dawn he lives to see is day 1's (the game begins at day 0's).
	stepTo(t, s, 1, 2*60+38)

	if c := clockState(s); str(c, "stage") != "night" {
		t.Fatalf("the premise: 02:38 of day 1 is night: %v", c)
	}

	if _, err := os.Stat(d2save.WorldPath(save)); !os.IsNotExist(err) {
		t.Fatalf("the premise: no world file before the dawn (%v)", err)
	}

	// --- act 0: a save before the dawn, kept for act 3 (B3) -------------------
	s.call("strigoi_save_game", map[string]any{})

	savedT0 := b5SavedAt(t, "act 0", save)
	filesT0 := map[string][]byte{}

	for _, f := range b5Files(save) {
		filesT0[f] = mustRead(t, f)
	}

	if a := autosaveOf(s); str(a, "state") != "idle" {
		t.Fatalf("act 0: a save at night leaves the autosave idle: %v", a)
	}

	frames := b5ToDawn(t, s, "act 1")

	a := autosaveOf(s)
	if str(a, "state") != "taken" || str(a, "by") != "dawn" || num(a, "tries") != 0 || num(a, "day") != 1 {
		t.Fatalf("act 1: the dawn's autosave is taken on its own frame: %v", a)
	}

	if c := clockState(s); str(c, "stage") != "dawn" {
		t.Fatalf("act 1: taken at dawn: %v", c)
	}

	if got := str(uiState(s), "save_notice"); got != b5DawnSaved {
		t.Fatalf("act 1: the notice is %q, want %q", got, b5DawnSaved)
	}

	savedA := b5SavedAt(t, "act 1", save)
	sA := snapWorld(t, s)
	t.Logf("act 1 PASS: %d frames from 02:38 the dawn armed and took the autosave (saved_at %s)", frames, savedA)

	// A second of the day after the dawn's save, for act 3.
	s.call("strigoi_step", map[string]any{"frames": 60})
	sDay := snapWorld(t, s)

	// --- act 2: relaunch, load: the dawn's moment --------------------------
	s = b5Relaunch(t, s)
	b5Resume(t, s, "act 2", save, savedA, b5Dials)
	sameWorld(t, "act 2 (S_R0 = the dawn's moment)", sA, snapWorld(t, s))

	// At most one per dawn: the resumed dawn is paid (dawn_paid_day is in the
	// file), so it arms nothing.
	s.call("strigoi_step_world", map[string]any{"world_minutes": 15})

	if a := autosaveOf(s); str(a, "state") != "idle" || num(saveState(s), "saves") != 0 {
		t.Fatalf("act 2: the resumed dawn does not save again: %v (saves %v)", a, saveState(s)["saves"])
	}

	if got := b5SavedAt(t, "act 2", save); got != savedA {
		t.Fatalf("act 2: the world file is still the dawn's (%s), not %s", savedA, got)
	}

	t.Logf("act 2 PASS: relaunched, resumed the dawn's moment; the resumed dawn armed nothing")

	// --- act 3: the save before the dawn, resumed, crosses the dawn (B3) ------
	// The B5 review's B3: no resume comparison crossed a dawn with the
	// in-frame autosave on both sides. His files as act 0 left them; the
	// resumed game steps to the dawn -- its own autosave taken on its own
	// frame, the same frame as the original's -- and a second on, and is the
	// original game a second after its dawn. The autosave, taken inside a
	// frame on both sides, moved nothing either would not.
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	for f, data := range filesT0 {
		if err := os.WriteFile(f, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	b5Resume(t, s, "act 3", save, savedT0, b5Dials)

	if again := b5ToDawn(t, s, "act 3"); again != frames {
		t.Fatalf("act 3: the resumed game's dawn armed %d frames from the save, the original's %d", again, frames)
	}

	if a := autosaveOf(s); str(a, "state") != "taken" || str(a, "by") != "dawn" {
		t.Fatalf("act 3: the resumed game's dawn took its own autosave: %v", a)
	}

	s.call("strigoi_step", map[string]any{"frames": 60})
	sameWorld(t, "act 3 (a save before the dawn, resumed across it: the autosave on both sides)", sDay, snapWorld(t, s))
	t.Logf("act 3 PASS: resumed the save of 02:38, crossed the dawn (%d frames, its own autosave), and a second on it is the original game", frames)

	// --- act 4: ...and with no autosave at all, the same world ---------------
	// The in-frame autosave MOVES NOTHING (rule 10; B3): until now that rested
	// on act 7a of TestSaveResume, a save between frames. The same resume and
	// the same frames with the autosave off (the harness's dial) are the
	// original game too -- so the dawn's save, taken inside the dawn's frame,
	// changed nothing the game would not have been without it.
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	for f, data := range filesT0 {
		if err := os.WriteFile(f, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	b5Resume(t, s, "act 4", save, savedT0, append(append([]dialWrite{}, b5Dials...), autosaveOffDial))

	if again := b5ToDawn(t, s, "act 4"); again != frames {
		t.Fatalf("act 4: the dawn came %d frames from the save, the original's %d", again, frames)
	}

	if a := autosaveOf(s); str(a, "state") != "off" {
		t.Fatalf("act 4: with the dial off the dawn saves nothing: %v", a)
	}

	s.call("strigoi_step", map[string]any{"frames": 60})
	sameWorld(t, "act 4 (the same frames with no autosave: the in-frame save moved nothing)", sDay, snapWorld(t, s))

	if got := b5SavedAt(t, "act 4", save); got != savedT0 {
		t.Fatalf("act 4: with the dial off no save was written: the world file is %s, not act 0's %s", got, savedT0)
	}

	t.Log("act 4 PASS: the same resume and frames with the autosave off is the same world: the dawn's in-frame save moved nothing")
}

func TestTheDawnAutosaveWaitsOutAFight(t *testing.T) {
	s := start(t)
	save := b5Start(t, s, "Wait")

	// Every blow a graze both ways, and a graze does nothing, so the fight
	// outlasts first light and he lives through it (measured, the first run:
	// graze_factor's default let the dog die in four rounds, before 02:45).
	dials := append(append([]dialWrite{}, b5Dials...),
		dialWrite{"combat", "graze_factor", 0.0}, dialWrite{"combat", "forced_band", "graze"})
	b5SetDials(s, dials)

	stepTo(t, s, 1, 2*60+41)
	b5Fight(t, s)

	if c := clockState(s); str(c, "stage") != "night" {
		t.Fatalf("the premise: the fight opens before first light: %v", c)
	}

	// --- act 1: first light in the fight: PENDING --------------------------
	frames := b5ToDawn(t, s, "act 1")

	// COMBAT since the combat status (30 Sep 2026): in his fight he is in
	// combat, and the save's refusal is that (it was FIGHTING's).
	a := autosaveOf(s)
	if str(a, "state") != "pending" || str(a, "last_refusal") != "COMBAT" || num(a, "tries") < 1 {
		t.Fatalf("act 1: first light in a fight: the autosave is pending on COMBAT: %v (combat fighting %v)",
			a, combatState(s)["fighting"])
	}

	if !flag(t, combatState(s), "fighting") {
		t.Fatalf("act 1: the fight is still running at first light: %v", combatState(s))
	}

	if _, err := os.Stat(d2save.WorldPath(save)); !os.IsNotExist(err) {
		t.Fatalf("act 1: a pending autosave has written nothing (%v)", err)
	}

	// Still pending a second of frames on: tried, and refused, every frame.
	s.call("strigoi_step", map[string]any{"frames": 60})

	a = autosaveOf(s)
	if str(a, "state") != "pending" || num(a, "tries") < 60 {
		t.Fatalf("act 1: a second later it is still pending, tried every frame: %v", a)
	}

	t.Logf("act 1 PASS: first light %d frames in, in the fight: pending, %v refused frames", frames, a["tries"])

	// --- act 2: the fight ends; the first frame that saves takes it --------
	dials[len(dials)-1] = dialWrite{"combat", "forced_band", "crit"}
	setField(s, "combat", "forced_band", "crit")

	ended, taken := 0, 0

	for i := 1; i <= 3000 && taken == 0; i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})

		if ended == 0 && !flag(t, combatState(s), "fighting") {
			ended = i
		}

		if str(autosaveOf(s), "state") != "pending" {
			taken = i
		}
	}

	a = autosaveOf(s)
	if str(a, "state") != "taken" || str(a, "by") != "dawn" || ended == 0 {
		t.Fatalf("act 2: the fight ended (frame %d) and the autosave was taken: %v", ended, a)
	}

	if got := str(uiState(s), "save_notice"); got != b5DawnSavedLate {
		t.Fatalf("act 2: the notice is %q, want %q", got, b5DawnSavedLate)
	}

	if c := clockState(s); str(c, "stage") == "night" {
		t.Fatalf("act 2: taken that day, not the next night: %v", c)
	}

	savedA := b5SavedAt(t, "act 2", save)
	sA := snapWorld(t, s)
	t.Logf("act 2 PASS: the fight ended %d frames on and the autosave was taken %d frames after that, after %v refused frames (the last %v)",
		ended, taken-ended, a["tries"], a["last_refusal"])

	// --- act 3: relaunch, load: the moment it was taken --------------------
	s = b5Relaunch(t, s)
	b5Resume(t, s, "act 3", save, savedA, dials)
	sameWorld(t, "act 3 (S_R0 = the moment the autosave was taken)", sA, snapWorld(t, s))
	t.Log("act 3 PASS: relaunched, resumed the moment the pending autosave was taken")
}

func TestTheCloseHook(t *testing.T) {
	s := start(t)
	save := b5Start(t, s, "Close")
	files := b5Files(save)

	// Mid-evening: 20:00 of the first day, dusk.
	stepTo(t, s, 0, 20*60)

	sC := snapWorld(t, s)
	out := s.call("strigoi_quit", map[string]any{"confirm": true, "graceful": true})

	closed := sub(out, "close")
	if !flag(t, closed, "saved") || !flag(t, closed, "unloaded") {
		t.Fatalf("act 1: the close saves and unloads: %v", out)
	}

	s.stop()

	savedC := b5SavedAt(t, "act 1", save)
	t.Logf("act 1 PASS: the graceful quit saved at 20:00 (saved_at %s) and unloaded", savedC)

	// --- act 2: relaunch, load: it resumes ----------------------------------
	s = start(t)
	b5Resume(t, s, "act 2", save, savedC, b5Dials)
	sameWorld(t, "act 2 (the close resumes)", sC, snapWorld(t, s))
	t.Log("act 2 PASS: relaunched, resumed the moment the window closed")

	// --- act 3: closed in a fight: unsaved, promptly, and his last save stands
	b5Fight(t, s)

	before := hashFiles(t, files[:1])
	began := time.Now()
	out = s.call("strigoi_quit", map[string]any{"confirm": true, "graceful": true})
	took := time.Since(began)

	// COMBAT since the combat status (30 Sep 2026; it was FIGHTING): rule 3,
	// "In combat it leaves without saving".
	closed = sub(out, "close")
	if flag(t, closed, "saved") || str(closed, "refused") != "COMBAT" || !flag(t, closed, "unloaded") {
		t.Fatalf("act 3: closed in a fight, the save is refused COMBAT and the close goes on: %v", out)
	}

	if took > 5*time.Second {
		t.Fatalf("act 3: the refused close took %v", took)
	}

	s.stop()

	if !equalHashes(before, hashFiles(t, files[:1])) {
		t.Fatal("act 3: a refused close wrote the world file")
	}

	// His .od2 and sidecar were written on the way out (the unload, as ever),
	// of the last save's generation, so the world file still pairs with them.
	if gen := generationOf(t, mustRead(t, files[2])); gen != savedC {
		t.Fatalf("act 3: his sidecar's generation is %q, the last save's %q", gen, savedC)
	}

	s = start(t)
	b5Resume(t, s, "act 3", save, savedC, b5Dials)
	sameWorld(t, "act 3 (a refused close: back to his last save)", sC, snapWorld(t, s))
	t.Logf("act 3 PASS: closed in a fight in %v, unsaved; he comes back to his last save", took)

	// THE B5 REVIEW, A1 (BUG-97): a close refused for what only holds the
	// screen left without saving -- the journal (Q) or a talk open, or the
	// second after a fight -- and nothing said so. The close ends them, lets
	// the fight's end settle, and saves.

	// --- act 4: closed with a talk open -----------------------------------------
	talkTo(t, s, nativeHandle(t, s))

	if st := saveState(s); str(st, "refused_now") != "TALKING" {
		t.Fatalf("act 4: the premise: a talk is open and a save is refused TALKING: %v", st)
	}

	atClose := mustNum(t, clockState(s), "world_minutes")
	out = s.call("strigoi_quit", map[string]any{"confirm": true, "graceful": true})

	closed = sub(out, "close")
	if !flag(t, closed, "saved") || strings.Join(stringsOf(closed["ended"]), ",") != "talk" {
		t.Fatalf("act 4: closed with a talk open, the close ends the talk and saves: %v", out)
	}

	s.stop()

	savedK := b5SavedAt(t, "act 4", save)
	if savedK == savedC {
		t.Fatal("act 4: the close wrote no new save")
	}

	s = start(t)
	b5Resume(t, s, "act 4", save, savedK, b5Dials)

	if got := mustNum(t, clockState(s), "world_minutes"); got != atClose || flag(t, uiState(s), "talk_open") {
		t.Fatalf("act 4: the resumed game is the moment of the close (minute %v, no talk open): minute %v, talk open %v",
			atClose, got, uiState(s)["talk_open"])
	}

	t.Logf("act 4 PASS: closed with a talk open: ended, saved (saved_at %s), resumed at the moment of the close", savedK)

	// --- act 5: closed with his journal open, an hour after his last save -------
	// The moment is taken with the journal open (opening it reads what it
	// shows, which the save keeps), and the resumed game is compared with it
	// open again: the journal is the one thing the close ended.
	s.call("strigoi_step_world", map[string]any{"world_minutes": 60})
	s.call("strigoi_key", map[string]any{"key": "q"})

	if st := saveState(s); !flag(t, uiState(s), "journal_open") || str(st, "refused_now") != "JOURNAL" {
		t.Fatalf("act 5: the premise: his journal is open and a save is refused JOURNAL: %v", st)
	}

	sJ := snapWorld(t, s)

	out = s.call("strigoi_quit", map[string]any{"confirm": true, "graceful": true})

	closed = sub(out, "close")
	if !flag(t, closed, "saved") || strings.Join(stringsOf(closed["ended"]), ",") != "journal" {
		t.Fatalf("act 5: closed with his journal open, the close ends it and saves: %v", out)
	}

	s.stop()

	savedJ := b5SavedAt(t, "act 5", save)
	if savedJ == savedK {
		t.Fatal("act 5: the close wrote no new save")
	}

	s = start(t)
	b5Resume(t, s, "act 5", save, savedJ, b5Dials)

	if flag(t, uiState(s), "journal_open") {
		t.Fatal("act 5: the resumed game opens with his journal closed")
	}

	s.call("strigoi_key", map[string]any{"key": "q"})
	sameWorld(t, "act 5 (closed with the journal open: the hour is kept)", sJ, snapWorld(t, s))

	// And he closes it again (it takes his keys while it is open).
	s.call("strigoi_key", map[string]any{"key": "q"})

	if flag(t, uiState(s), "journal_open") {
		t.Fatal("act 5: Q closes the journal again")
	}
	t.Logf("act 5 PASS: closed an hour after his last save with his journal open: ended, saved (saved_at %s), the hour kept", savedJ)

	// --- act 6: closed in the moment after a fight ------------------------------
	// The dog dies (every blow a crit); the fight is over, and the save is
	// still refused while its end settles -- the dog's death playing out, his
	// last swing -- which frames cure: the close runs them, then saves. Since
	// the combat status (30 Sep 2026) the refusal is COMBAT for the few seconds
	// after his fight (the grace), which the close waits out (up to 180
	// frames), and saves.
	b5Fight(t, s)
	setField(s, "combat", "forced_band", "crit")

	for i := 0; flag(t, combatState(s), "fighting"); i++ {
		if i > 3000 {
			t.Fatalf("act 6: the fight never ended: %v", combatState(s))
		}

		s.call("strigoi_step", map[string]any{"frames": 1})
	}

	st := saveState(s)
	if str(st, "refused_now") != "COMBAT" || !hasString([]string{"grace", "swing", "reaction"}, str(st, "combat_reason")) {
		t.Fatalf("act 6: the premise: the fight is over and a save is still refused in the moment after it (COMBAT, its grace): %v", st)
	}

	reasonAtClose, graceAtClose := str(st, "combat_reason"), mustNum(t, st, "combat_grace_left")

	xpAtClose := mustNum(t, progressState(s), "xp")
	atClose = mustNum(t, clockState(s), "world_minutes")
	out = s.call("strigoi_quit", map[string]any{"confirm": true, "graceful": true})

	// The settle waits out what is left: his swing or reaction, if one still
	// plays, and then the whole grace (180 frames); or, in the grace alone,
	// its remainder. Never more than the close's bound (480).
	closed = sub(out, "close")
	settled := num(closed, "settle_frames")

	if !flag(t, closed, "saved") || settled < 1 || settled > 480 ||
		(reasonAtClose == "grace" && math.Abs(settled-graceAtClose*60) > 1) || (reasonAtClose != "grace" && settled < 180) {
		t.Fatalf("act 6: closed in the moment after a fight (%s, %.3f s of grace left), the close waits out what is left of it and saves: %v",
			reasonAtClose, graceAtClose, out)
	}

	s.stop()

	savedF := b5SavedAt(t, "act 6", save)
	if savedF == savedJ {
		t.Fatal("act 6: the close wrote no new save")
	}

	s = start(t)
	b5Resume(t, s, "act 6", save, savedF, b5Dials)

	if got := mustNum(t, clockState(s), "world_minutes"); got < atClose || got > atClose+20 {
		t.Fatalf("act 6: the resumed game is the close's moment, a few settled frames on: minute %v, the close at %v", got, atClose)
	}

	if xp := mustNum(t, progressState(s), "xp"); xp < xpAtClose || flag(t, combatState(s), "fighting") {
		t.Fatalf("act 6: the fight's end is kept: xp %v (at the close %v), fighting %v", xp, xpAtClose, combatState(s)["fighting"])
	}

	t.Logf("act 6 PASS: closed in the moment after a fight (%s): %v settle frames (%v ms), saved (saved_at %s), resumed",
		reasonAtClose, closed["settle_frames"], closed["settle_ms"], savedF)
}

func TestTheLoadNotice(t *testing.T) {
	s := start(t)
	save := b5Start(t, s, "Notice")

	s.call("strigoi_step_world", map[string]any{"world_minutes": 10})
	s.call("strigoi_save_game", map[string]any{})

	// --- act 1: a file this build cannot read: set aside, never lost ---------
	world := d2save.WorldPath(save)
	data := mustRead(t, world)
	was := []byte(fmt.Sprintf("\"version\": %d,", d2save.Version))

	if !bytes.Contains(data, was) {
		t.Fatalf("act 1: the world file has no %s", was)
	}

	edited := bytes.Replace(data, was, []byte(`"version": 99,`), 1)
	if err := os.WriteFile(world, edited, 0o600); err != nil {
		t.Fatal(err)
	}

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	g := s.call("strigoi_start_game", map[string]any{"save_path": save, "seed": 1462, "wait_seconds": 90})
	load := sub(g, "load")

	if str(load, "refused") != "VERSION" || str(load, "set_aside") == "" {
		t.Fatalf("act 1: a version this build cannot read is refused and set aside: %v", load)
	}

	aside := str(load, "set_aside")
	if !filepath.IsAbs(aside) {
		aside = filepath.Join(filepath.Dir(save), aside)
	}

	if kept := mustRead(t, aside); !bytes.Equal(kept, edited) {
		t.Fatalf("act 1: the file set aside at %s is the file, byte for byte (never lost)", aside)
	}

	want := b5OtherVersion + "\n" + b5WakeAtDawn
	if got := str(uiState(s), "save_notice"); got != want {
		t.Fatalf("act 1: the notice at the start of play is %q, want %q", got, want)
	}

	if got := str(saveState(s), "load_notice"); got != want {
		t.Fatalf("act 1: the save provider's load_notice is %q", got)
	}

	shot := s.call("strigoi_screenshot", map[string]any{"name": "b5-load-notice"})
	t.Logf("act 1 PASS: refused VERSION, set aside as %s, and he was told: %q (%v)", aside, want, shot["path"])

	// --- act 2: a save with a villager gone ---------------------------------
	b5SetDials(s, b5Dials)

	gone := nativeHandle(t, s)
	s.call("strigoi_remove_entity", map[string]any{"handle": gone})
	s.call("strigoi_save_game", map[string]any{})
	savedV := b5SavedAt(t, "act 2", save)

	fileV := mustRead(t, world)

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)
	b5Resume(t, s, "act 2", save, savedV, b5Dials)

	sV := snapWorld(t, s)
	load = sub(s.call("strigoi_get_game_info", map[string]any{}), "load")
	if len(stringsOf(load["dropped"])) != 1 {
		t.Fatalf("act 2: the load took the one villager the file lacks off the map: %v", load)
	}

	if got := str(uiState(s), "save_notice"); got != b5VillagerGone {
		t.Fatalf("act 2: the notice is %q, want %q", got, b5VillagerGone)
	}

	t.Logf("act 2 PASS: resumed with %v gone, and he was told", load["dropped"])

	// --- act 3: a save cut off between its files: the save before it -------
	// The B5 review, A2 (BUG-98). What a crash, or a close past its limit,
	// leaves when it falls after the world file landed and before his .od2
	// and sidecar: the new world file beside the last save's .od2 and
	// sidecar, and the last save itself kept as the world file's .bak. The
	// world file is TORN; its .bak is his sidecar's moment -- the last whole
	// save -- so the load resumes the .bak, sets the torn file aside (kept,
	// never lost) and tells him. Before the fix he woke at dawn.
	od2V, sidecarV := mustRead(t, save), mustRead(t, save+".strigoi.json")
	if gen := generationOf(t, sidecarV); gen != savedV {
		t.Fatalf("act 3: the premise: his sidecar is the last save's (%s): %s", savedV, gen)
	}

	s.call("strigoi_step_world", map[string]any{"world_minutes": 15})
	s.call("strigoi_save_game", map[string]any{})

	cutAt := b5SavedAt(t, "act 3", save)
	cutFile := mustRead(t, world)

	if bak := mustRead(t, world+".bak"); !bytes.Equal(bak, fileV) {
		t.Fatal("act 3: the premise: the new save kept the last one as the world file's .bak")
	}

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	// The cut: his .od2 and sidecar as the last save left them.
	if err := os.WriteFile(save, od2V, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(save+".strigoi.json", sidecarV, 0o600); err != nil {
		t.Fatal(err)
	}

	b5Resume(t, s, "act 3", save, savedV, b5Dials)

	load = sub(s.call("strigoi_get_game_info", map[string]any{}), "load")
	aside = str(load, "set_aside")

	if !flag(t, load, "from_bak") || !strings.HasSuffix(aside, ".torn.unread") {
		t.Fatalf("act 3: the torn world file (saved %s) was set aside as .torn.unread and its .bak resumed: %v", cutAt, load)
	}

	if !filepath.IsAbs(aside) {
		aside = filepath.Join(filepath.Dir(save), aside)
	}

	if kept := mustRead(t, aside); !bytes.Equal(kept, cutFile) {
		t.Fatalf("act 3: the file set aside at %s is the cut-off save, byte for byte (never lost)", aside)
	}

	want = b5CutOff + "\n" + b5BakResumed
	if got := str(uiState(s), "save_notice"); got != want {
		t.Fatalf("act 3: the notice is %q, want %q", got, want)
	}

	sameWorld(t, "act 3 (a save cut off: the save before it, resumed)", sV, snapWorld(t, s))
	t.Logf("act 3 PASS: the world file cut off at %s was set aside as %s, the .bak (%s) resumed, and he was told", cutAt, filepath.Base(aside), savedV)
}
