//go:build playtest

package playtest

import (
	"image"
	"strings"
	"testing"
)

// THE COMBAT STATUS (Josh, 30 Sep 2026): "I think we should have a combat
// status for the player, and you can't save while in combat." He is in combat
// while his fight is live, his own swing or hit or block reaction plays, or a
// hostile chases him -- and for three seconds of game time after the last of
// these ends (the save provider's combat_grace). A small marker sits over the
// health globe while he is (a red "!" until its art lands), and every save is
// refused COMBAT: "You can't save in combat." (d2gamescreen/combat_status.go)
//
//	0  Out of combat: the provider says so, and the marker's square is the
//	   map's (the pixel control).
//	1  A HOSTILE CHASES HIM: a fallen eight to ten tiles off, set to watch
//	   him, notices him and gives chase -- the game's own chain, no fight yet.
//	   In combat, "chased"; the marker is drawn -- by the ui provider, and in
//	   the screenshot's pixels at its square; the escape menu says "You can't
//	   save in combat." and offers EXIT WITHOUT SAVING; SAVE GAME is refused
//	   COMBAT with those words and writes nothing.
//	2  THE CHASE ENDS (the fallen let go: unwatched, its chase released): in
//	   combat for the grace, the marker still drawn; out after 3 s (180
//	   frames, one either way), the marker gone -- provider and pixels -- and
//	   SAVE GAME saves ("Game saved.").
//	3  A VILLAGE FIGHT ELSEWHERE (a clock fight: a zombie on a fallen fifteen
//	   tiles off): live, and he is not in combat; no marker; a save is made.
//	4  BUG-105: HIS HIT REACTION AFTER HIS FIGHT ENDS. A dog beside him; he
//	   holds and every blow is a hit (so no swing or riposte of his hides the
//	   reaction); the frame the dog's blow lands and his get-hit plays, his
//	   fight is ended (disengaged, the dog let go). The save during the reaction is refused COMBAT, "reaction"; after
//	   the reaction and the grace, made.
//	5  THE DAWN AUTOSAVE WHILE HE IS CHASED AT DAWN: day 1's first light finds
//	   a zombie on his trail (sent after him, and watching nobody, so it never
//	   fights): the autosave is armed and PENDING, refused COMBAT, every frame;
//	   the chase released, it is taken on the first frame out of combat -- 3 s
//	   later -- by the dawn, and the world file is that moment's.
//
// No pack, and the dead stay down (b5Dials): every hostile here is placed by
// the script, and each act's is let go before the next.

const (
	csCombatWords = "You can't save in combat."

	// The moment after combat, under the menu (the review's B2): its first line.
	csMomentWords = "Return to the game; you can save"

	// The marker's square (d2player: combatMarkerX, combatMarkerY, 32 square)
	// and its edge's colour, 0x8c1c14.
	csMarkerX, csMarkerY, csMarkerSize = 54, 468, 32
)

// csStatus is the save provider's combat status.
func csStatus(t *testing.T, s *session) (inCombat bool, reason string, left float64) {
	t.Helper()

	st := saveState(s)

	return flag(t, st, "in_combat"), str(st, "combat_reason"), mustNum(t, st, "combat_grace_left")
}

// csMarker is the ui provider's combat_marker: what the marker last drew.
func csMarker(s *session) map[string]any { return sub(uiState(s), "combat_marker") }

// csShot takes a screenshot and reports whether the marker's red edge is at
// its square: the left edge's middle pixel and the top edge's, each within a
// few levels of 0x8c1c14.
func csShot(t *testing.T, s *session, name string) (drawn bool, path string) {
	t.Helper()

	img, path := screenshot(t, s, name)
	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Fatalf("the screenshot is %v, not 800x600: %s", b, path)
	}

	edge := func(x, y int) bool {
		r, g, b := csRGB(img, x, y)
		return abs(r-0x8c) <= 6 && abs(g-0x1c) <= 6 && abs(b-0x14) <= 6
	}

	return edge(csMarkerX, csMarkerY+csMarkerSize/2) && edge(csMarkerX+csMarkerSize/2, csMarkerY), path
}

func csRGB(img image.Image, x, y int) (r, g, b int) {
	rr, gg, bb, _ := img.At(x, y).RGBA()

	return int(rr >> 8), int(gg >> 8), int(bb >> 8)
}

// csFramesUntilOut steps a frame at a time until the provider says he is out
// of combat, up to limit, and returns how many frames it took.
func csFramesUntilOut(t *testing.T, s *session, limit int) int {
	t.Helper()

	for i := 1; i <= limit; i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})

		if in, _, _ := csStatus(t, s); !in {
			return i
		}
	}

	in, reason, left := csStatus(t, s)
	t.Fatalf("still in combat after %d frames: %v %q, %.3f s of grace left", limit, in, reason, left)

	return 0
}

// csLetGo takes a hostile out of his world: unwatched, and its chase released.
func csLetGo(t *testing.T, s *session, handle, target string) {
	t.Helper()

	s.call("strigoi_watch", map[string]any{"watcher": handle, "target": target, "release": true})

	id := entityID(t, s, handle)
	for _, raw := range scChases(s) {
		if c, _ := raw.(map[string]any); str(c, "hunter") == id {
			setField(s, "pursuit", "release", id)
		}
	}
}

// csChasedBy is whether the pursuit provider has hunter id chasing him.
func csChasedBy(s *session, hunter, him string) bool {
	for _, raw := range scChases(s) {
		if c, _ := raw.(map[string]any); str(c, "hunter") == hunter && str(c, "quarry") == him {
			return true
		}
	}

	return false
}

func TestTheCombatStatus(t *testing.T) {
	s := start(t)
	save := b5Start(t, s, "Combat")
	files := b5Files(save)

	if got := mustNum(t, saveState(s), "combat_grace"); got != 3 {
		t.Fatalf("the grace is the shipped 3 s of game time: %v", got)
	}

	// To the day: a fallen sees a man in daylight from where it stands.
	for i := 0; i < 12 && str(clockState(s), "stage") != "day"; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 15})
	}

	p := s.call("strigoi_get_player", map[string]any{})
	him, handle, px, py := str(p, "id"), str(p, "handle"), num(p, "x"), num(p, "y")

	// --- act 0: out of combat --------------------------------------------------
	if in, reason, left := csStatus(t, s); in || reason != "" || left != 0 {
		t.Fatalf("act 0: out of combat at %s: in %v %q, %.3f s left", str(clockState(s), "time_of_day"), in, reason, left)
	}

	drawn, shot := csShot(t, s, "combat-0-out")
	if drawn || str(csMarker(s), "drew") != "" {
		t.Fatalf("act 0: no marker out of combat: pixels %v (%s), ui %v", drawn, shot, csMarker(s))
	}

	t.Logf("act 0 PASS: out of combat at %s; the marker's square is the map's (%s)", str(clockState(s), "time_of_day"), shot)

	// --- act 1: a hostile chases him ---------------------------------------------
	spot := scClearSpot(t, s, px, py)
	dog := spawnNPC(t, s, "fallen1", spot[0], spot[1])
	dogID := entityID(t, s, dog)
	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": handle})

	frames := 0
	for ; frames < 120 && !csChasedBy(s, dogID, him); frames++ {
		s.call("strigoi_step", map[string]any{"frames": 1})
	}

	if !csChasedBy(s, dogID, him) || flag(t, combatState(s), "fighting") {
		t.Fatalf("act 1: the fallen %s gives chase, no fight yet (%d frames): fighting %v, chases %v",
			dogID, frames, combatState(s)["fighting"], scChases(s))
	}

	if in, reason, left := csStatus(t, s); !in || reason != "chased" || left != 3 {
		t.Fatalf("act 1: chased, he is in combat: in %v %q, %.3f s of grace held", in, reason, left)
	}

	drawn, shot = csShot(t, s, "combat-1-chased")
	if m := csMarker(s); !drawn || str(m, "drew") != "!" {
		t.Fatalf("act 1: the marker over the health globe: pixels %v (%s), ui %v", drawn, shot, m)
	}

	s.call("strigoi_key", map[string]any{"key": "escape"})
	m := escapeMenuOf(s)

	if !strings.HasPrefix(str(m, "note"), csCombatWords) || !flag(t, m, "exit_refused") {
		t.Fatalf("act 1: chased, the menu says %q and its exit leaves unsaved: %v", csCombatWords, m)
	}

	if got := strings.Join(stringsOf(m["entries"]), "|"); !strings.Contains(got, b5ExitUnsaved) {
		t.Fatalf("act 1: the exit entry reads %q in combat: %q", b5ExitUnsaved, got)
	}

	csShot(t, s, "combat-1-menu") // the menu over the marker, for a person to look at
	before := hashFiles(t, files)
	menuPick(t, s, "act 1", b5SaveGame)

	if ls := lastSaveOf(s); str(ls, "result") != "refused" || str(ls, "code") != "COMBAT" || str(ls, "words") != csCombatWords {
		t.Fatalf("act 1: SAVE GAME is refused COMBAT, in his words: %v", ls)
	}

	if !equalHashes(before, hashFiles(t, files)) {
		t.Fatal("act 1: a refused save touched a file")
	}

	menuPick(t, s, "act 1", "RETURN TO GAME")
	t.Logf("act 1 PASS: %s chasing him after %d frames: in combat (chased), the marker drawn (pixels %v, %s), the menu said %q and SAVE GAME wrote nothing",
		dogID, frames, drawn, shot, csCombatWords)

	// --- act 2: the chase ends; the grace; out ----------------------------------------
	csLetGo(t, s, dog, handle)

	if csChasedBy(s, dogID, him) || flag(t, combatState(s), "fighting") {
		t.Fatalf("act 2: the fallen is let go: chases %v", scChases(s))
	}

	if in, reason, _ := csStatus(t, s); !in || reason != "grace" {
		t.Fatalf("act 2: the chase over, the grace holds him in combat: %v %q", in, reason)
	}

	s.call("strigoi_step", map[string]any{"frames": 170})

	if in, reason, left := csStatus(t, s); !in || reason != "grace" || left <= 0 || left > 0.2 {
		t.Fatalf("act 2: 170 frames into the grace he is still in combat, with about a sixth of a second left: %v %q %.3f", in, reason, left)
	}

	if drawn, shot = csShot(t, s, "combat-2-grace"); !drawn {
		t.Fatalf("act 2: the marker stays while the grace runs: %s", shot)
	}

	n := 170 + csFramesUntilOut(t, s, 30)
	if n < 179 || n > 181 {
		t.Fatalf("act 2: out of combat %d frames after the chase ended, want 180 (3 s of game time)", n)
	}

	if drawn, shot = csShot(t, s, "combat-2-out"); drawn || str(csMarker(s), "drew") != "" {
		t.Fatalf("act 2: out of combat, the marker is gone: pixels %v (%s), ui %v", drawn, shot, csMarker(s))
	}

	menuPick(t, s, "act 2", b5SaveGame)

	if ls := lastSaveOf(s); str(ls, "result") != "saved" || str(uiState(s), "save_notice") != b5GameSaved {
		t.Fatalf("act 2: out of combat, SAVE GAME saves: %v, notice %q", ls, str(uiState(s), "save_notice"))
	}

	t.Logf("act 2 PASS: the chase ended; in combat %d frames more (the grace), the marker gone after (%s), and SAVE GAME saved", n, shot)

	// --- act 2b: the grace under the escape menu (the review's B3 and B2) -------------
	// A chase (strigoi_pursue: no watch, so no fight) and its release put him
	// in the grace; under the menu, which pauses a single-player world, the
	// grace does not run -- 300 frames and not a hundredth of a second gone --
	// and the menu's line says what to do: "Return to the game; you can save
	// a moment after the fighting stops." (not "You can't save in combat."
	// with nothing in sight). Back in the game the grace runs out from where
	// it stood.
	p = s.call("strigoi_get_player", map[string]any{})
	spot = scClearSpot(t, s, num(p, "x"), num(p, "y"))
	runner := spawnNPC(t, s, "zombie1", spot[0], spot[1])
	runnerID := entityID(t, s, runner)
	s.call("strigoi_pursue", map[string]any{"hunter": runner, "quarry": handle})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if in, reason, _ := csStatus(t, s); !in || reason != "chased" {
		t.Fatalf("act 2b: %s sent after him: in combat (chased): %v %q", runnerID, in, reason)
	}

	setField(s, "pursuit", "release", runnerID)
	s.call("strigoi_step", map[string]any{"frames": 60})

	_, reason, leftBefore := csStatus(t, s)
	if reason != "grace" || leftBefore <= 1.5 || leftBefore >= 2.5 {
		t.Fatalf("act 2b: a second into the grace: %q, %.3f s left", reason, leftBefore)
	}

	s.call("strigoi_key", map[string]any{"key": "escape"})
	s.call("strigoi_step", map[string]any{"frames": 300})

	in, reasonUnder, leftUnder := csStatus(t, s)
	if !in || reasonUnder != "grace" || leftUnder != leftBefore {
		t.Fatalf("act 2b: 300 frames under the escape menu, the grace is paused: %.6f s before, %v %q %.6f s after",
			leftBefore, in, reasonUnder, leftUnder)
	}

	m = escapeMenuOf(s)
	if !strings.HasPrefix(str(m, "note"), csMomentWords) || !flag(t, m, "exit_refused") {
		t.Fatalf("act 2b: in the grace, the menu says %q: %v", csMomentWords, m)
	}

	csShot(t, s, "combat-2b-menu-grace")
	menuPick(t, s, "act 2b", "RETURN TO GAME")

	out := csFramesUntilOut(t, s, 200)
	if want := int(leftBefore*60 + 0.5); out < want-2 || out > want+2 {
		t.Fatalf("act 2b: back in the game, out %d frames later; the grace had %.3f s (%d frames) left", out, leftBefore, want)
	}

	t.Logf("act 2b PASS: the grace held at %.3f s through 300 frames under the menu, the menu said %q, and ran out %d frames after it closed",
		leftBefore, csMomentWords, out)

	// --- act 3: a village fight elsewhere --------------------------------------------
	quarry, monster := spawnNPC(t, s, "fallen1", px+15, py+3), spawnNPC(t, s, "zombie1", px+16, py+3)
	quarryID := entityID(t, s, quarry)
	s.call("strigoi_watch", map[string]any{"watcher": monster, "target": quarry})

	for i := 0; i < 120 && afhFightFor(s, quarryID) == nil; i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})
	}

	if afhFightFor(s, quarryID) == nil || flag(t, combatState(s), "fighting") {
		t.Fatalf("act 3: a clock fight on %s, not his: fighting %v, clock %v", quarryID, combatState(s)["fighting"], afhClock(s))
	}

	s.call("strigoi_step", map[string]any{"frames": 30})

	if in, reason, _ := csStatus(t, s); in || afhFightFor(s, quarryID) == nil {
		t.Fatalf("act 3: the village fights and he is not in combat: %v %q; live %v", in, reason, afhFightFor(s, quarryID) != nil)
	}

	if drawn, shot = csShot(t, s, "combat-3-village"); drawn {
		t.Fatalf("act 3: no marker for a fight he is not in: %s", shot)
	}

	if e := s.callErr("strigoi_save_game", map[string]any{}); e != "" {
		t.Fatalf("act 3: a save is made while the village fights: %s", e)
	}

	csLetGo(t, s, monster, quarry)
	t.Logf("act 3 PASS: a clock fight live on %s: not in combat, no marker, the save made", quarryID)

	// --- act 4: BUG-105: his reaction after his fight ends ---------------------------
	p = s.call("strigoi_get_player", map[string]any{})
	beside := clearNeighbour(t, s, num(p, "x"), num(p, "y"))
	biter := spawnNPC(t, s, "fallen1", beside[0], beside[1])
	// He holds, and every blow is a hit: a graze on him is answered by his
	// riposte -- a swing -- and a hit plays his get-hit (measured, the first
	// run: under grazes the dog died of ripostes before one blow flinched him).
	setField(s, "combat", "player_action", "hold")
	setField(s, "combat", "forced_band", "hit")
	s.call("strigoi_watch", map[string]any{"watcher": biter, "target": handle})
	fightNow(t, s)

	mode := ""
	for i := 0; i < 1200; i++ {
		ps := sub(s.call("strigoi_get_player", map[string]any{}), "state")
		if mode = str(ps, "animation_mode"); (mode == "GH" || mode == "BL") && !flag(t, ps, "casting") {
			break
		}

		s.call("strigoi_step", map[string]any{"frames": 1})
	}

	if mode != "GH" && mode != "BL" {
		t.Fatalf("act 4: no blow on him played its reaction in 1200 frames: %v", combatState(s))
	}

	setField(s, "combat", "disengage", true)
	csLetGo(t, s, biter, handle)
	setField(s, "combat", "forced_band", "")
	setField(s, "combat", "player_action", "attack")

	ps := sub(s.call("strigoi_get_player", map[string]any{}), "state")
	if flag(t, combatState(s), "fighting") || str(ps, "animation_mode") != mode {
		t.Fatalf("act 4: his fight is over and his %s still plays: fighting %v, mode %q", mode, combatState(s)["fighting"], str(ps, "animation_mode"))
	}

	if in, reason, _ := csStatus(t, s); !in || reason != "reaction" {
		t.Fatalf("act 4 (BUG-105): his %s playing after his fight, he is in combat for it: %v %q", mode, in, reason)
	}

	refusedWith(t, s, "act 4 (BUG-105)", "COMBAT", map[string]any{})

	n = csFramesUntilOut(t, s, 400)

	if e := s.callErr("strigoi_save_game", map[string]any{}); e != "" {
		t.Fatalf("act 4: out of combat after the reaction and the grace, the save is made: %s", e)
	}

	t.Logf("act 4 PASS (BUG-105): his %s played on after his fight ended; the save was refused COMBAT (reaction), and made %d frames later", mode, n)

	// --- act 5: the dawn autosave while he is chased at dawn ------------------------
	stepTo(t, s, 1, 2*60+36) // fed and watered on the way

	for i := 0; i < 10 && mustNum(t, clockState(s), "minute_of_day") < 2*60+40; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1})
	}

	if c := clockState(s); mustNum(t, c, "day_index") != 1 || str(c, "stage") != "night" {
		t.Fatalf("act 5: the last minutes of the first night: %v", c)
	}

	p = s.call("strigoi_get_player", map[string]any{})
	spot = scClearSpot(t, s, num(p, "x"), num(p, "y"))
	tracker := spawnNPC(t, s, "zombie1", spot[0], spot[1])
	trackerID := entityID(t, s, tracker)
	s.call("strigoi_pursue", map[string]any{"hunter": tracker, "quarry": handle})

	savedBefore := b5SavedAt(t, "act 5 (the last save)", save)

	// Until first light arms it: pending, or -- the control that lets the
	// autosave ignore combat -- taken at once.
	armed := 0
	for ; armed < 3000 && !hasString([]string{"pending", "taken"}, str(autosaveOf(s), "state")); armed++ {
		s.call("strigoi_step", map[string]any{"frames": 1})
	}

	a := autosaveOf(s)
	if str(a, "state") != "pending" || !csChasedBy(s, trackerID, him) {
		t.Fatalf("act 5: first light arms the autosave while he is chased: %v, chased %v", a, csChasedBy(s, trackerID, him))
	}

	s.call("strigoi_step", map[string]any{"frames": 120})

	if a = autosaveOf(s); str(a, "state") != "pending" || str(a, "last_refusal") != "COMBAT" || mustNum(t, a, "tries") < 100 {
		t.Fatalf("act 5: chased at dawn, the autosave waits, refused COMBAT every frame: %v", a)
	}

	setField(s, "pursuit", "release", trackerID)

	taken := 0
	for ; taken < 400 && str(autosaveOf(s), "state") == "pending"; taken++ {
		s.call("strigoi_step", map[string]any{"frames": 1})
	}

	a = autosaveOf(s)
	if str(a, "state") != "taken" || str(a, "by") != "dawn" || taken < 179 || taken > 182 {
		t.Fatalf("act 5: the chase released, the autosave is taken by the dawn on the first frame out of combat, 180 frames on: %v after %d frames", a, taken)
	}

	savedAt := b5SavedAt(t, "act 5", save)
	if savedAt == savedBefore {
		t.Fatal("act 5: the dawn's autosave wrote no new world file")
	}

	t.Logf("act 5 PASS: the dawn armed the autosave with %s on his trail; pending, refused COMBAT (%v tries); taken by the dawn %d frames after the chase ended (saved_at %s)",
		trackerID, a["tries"], taken, savedAt)
}
