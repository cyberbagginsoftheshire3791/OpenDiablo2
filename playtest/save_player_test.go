//go:build playtest

package playtest

import (
	"bytes"
	"fmt"
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
//	                             fight the menu says "You can't save during a
//	                             fight." and offers EXIT WITHOUT SAVING, which
//	                             writes no world file, and he comes back to his
//	                             last save. SAVE AND EXIT GAME saves and leaves.
//	TestTheDawnAutosave          the first dawn he lives to see saves the game on
//	                             its own frame; a relaunch resumes that moment,
//	                             and the resumed dawn does not save again.
//	TestTheDawnAutosaveWaitsOutAFight
//	                             a fight across first light: the autosave is
//	                             PENDING through every refused frame, and taken
//	                             the first frame after the fight has settled; a
//	                             relaunch resumes the moment it was taken.
//	TestTheCloseHook             the graceful quit (the window's close) at 20:00
//	                             saves and a relaunch resumes it; closed in a
//	                             fight, it leaves unsaved, promptly, and he
//	                             comes back to his last save.
//	TestTheLoadNotice            a world file this build cannot read is set aside
//	                             (kept, never deleted) and he wakes at dawn, told
//	                             so; a save with a villager gone says so.
//
// Every script's hero lives in the test's own %APPDATA% (the launcher's
// testHome), never Josh's saves.

// The words he reads (d2player's strigoi_strings.go), as he reads them.
const (
	b5GameSaved     = "Game saved."
	b5CantInAFight  = "You can't save during a fight."
	b5ExitUnsaved   = "EXIT WITHOUT SAVING"
	b5SaveAndExit   = "SAVE AND EXIT GAME"
	b5SaveGame      = "SAVE GAME"
	b5DawnSaved     = "Dawn. The game is saved."
	b5DawnSavedLate = "The dawn's save is made."
	b5WakeAtDawn    = "You wake at dawn. The save is kept, set aside -- not deleted."
	b5OtherVersion  = "Your save is from another version of the game."
	b5VillagerGone  = "Your save is restored. A villager who was gone when you saved is gone again."
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

	if ls := lastSaveOf(s); str(ls, "result") != "refused" || str(ls, "code") != "FIGHTING" || str(ls, "words") != b5CantInAFight {
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

	a := autosaveOf(s)
	if str(a, "state") != "pending" || str(a, "last_refusal") != "FIGHTING" || num(a, "tries") < 1 {
		t.Fatalf("act 1: first light in a fight: the autosave is pending on FIGHTING: %v (combat fighting %v)",
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

	closed = sub(out, "close")
	if flag(t, closed, "saved") || str(closed, "refused") != "FIGHTING" || !flag(t, closed, "unloaded") {
		t.Fatalf("act 3: closed in a fight, the save is refused and the close goes on: %v", out)
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

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)
	b5Resume(t, s, "act 2", save, savedV, b5Dials)

	load = sub(s.call("strigoi_get_game_info", map[string]any{}), "load")
	if len(stringsOf(load["dropped"])) != 1 {
		t.Fatalf("act 2: the load took the one villager the file lacks off the map: %v", load)
	}

	if got := str(uiState(s), "save_notice"); got != b5VillagerGone {
		t.Fatalf("act 2: the notice is %q, want %q", got, b5VillagerGone)
	}

	t.Logf("act 2 PASS: resumed with %v gone, and he was told", load["dropped"])
}
