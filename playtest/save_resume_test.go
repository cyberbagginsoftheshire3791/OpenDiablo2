//go:build playtest

package playtest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
)

// TestSaveResume is M4.6's acceptance test (build plan section 4): the save
// "stops at that point then resumes at that point" (Josh, 25 Sep 2026).
//
// ACTS 1-6 ARE THE TEST THE WHOLE MILESTONE WAS DESIGNED AROUND: save at T,
// step 120 world minutes, relaunch, load, and the relaunched game must equal
// the original at T and again at T+120. B4a built them on a quiet evening (29
// Sep 2026); B4b (the same day) made the evening a HUNTED NIGHT -- the save
// "stops at that point then resumes at that point" only if it does so with a
// pack after him, and every dusk, night and dawn save measured is hunted.
// Act 7 is B3's (the save verb and its refusals), run first in a process of
// its own (saveVerbActs, below). Act 8 is rule 4 (a save mid-walk, B4b). The
// omit sweep of act 9 is TestSaveResumeNegatives (B4b, opt-in; B6's to
// finish).
//
//	1  Seed 99, stepped, the default game. A night that fills every block:
//	   the kit (the default loadout); a fight by day, a dog slain and LEFT
//	   LYING where he fell (a corpse entity, his body at 0; B4a took him off
//	   the map, which the quiet evening needed); a second fight, a second dog
//	   grazed and let go -- a wounded survivor of an ended fight (B4b); two
//	   forages and three stakes whittled, one of Night 1's dead staked and one
//	   in a hasty grave, the other two risen in the first night and staked by
//	   day where first light laid them down (a risen body Closed), the watch
//	   promised in a talk and stood at the headman's post from true dark, a
//	   second squad deployed (its model an NPC on the map: B4b), his torch
//	   lit; and, last, a pack forced from the table that NOTICES him and gives
//	   chase -- T is the first frames of the chase, the pack walking in (B4b).
//	   The precondition names every block and requires it non-empty
//	   (huntedPrecondition for B4b's) -- and (the B4a review, A3) every field
//	   the load restores ON HIM differs from what a fresh game gives him: his
//	   run toggle on, his wind below its maximum, a talent taken, a fraction
//	   of a health point owed to neglect, his facing not the fresh one.
//	2  At T -- about 01:40 of the third day, standing still at the post -- the
//	   save, which moves nothing (the digest before and after is one digest),
//	   and the file holds the packs, watches, chases, bodies, the entities the
//	   map does not build and the deployed squad.
//	3  120 world minutes more: the pack walks in and its fight is played
//	   BLOW BY BLOW (the B4b review fixes, BUG-77: B4b quick-resolved it
//	   before its first blow, so no roll or blow after T was ever compared;
//	   now the quick resolve is off and every blow a graze both ways -- dials
//	   at T -- so he lives, and act 3 requires blows and rounds after T and no
//	   quick resolve), dawn comes (the night paid, the watch kept, the
//	   rite, the soul pressure's count) and the torch burns out; S_U. Then he
//	   is wounded and leaves through the menu, which writes his .od2 and his
//	   sidecar and not the world file (B5's): the files a load finds beside
//	   the world file are two hours and a wound later than it.
//	4  A new process in the same home: start_game{save_path} -- NO seed, as
//	   the shipped game has none to give (the B4a review, C1: the seed the
//	   script asked for is this process's, not the world's) -- RESUMES the file
//	   on its own seed (the load reports it, and its steps), and S_R0 = S_T:
//	   the resume digest -- world, entities, rng and every system's world
//	   state -- and each named system. The world file won over the later .od2
//	   (his health) and the later sidecar (a kit whose torch had burnt out).
//	   Every entity the file names was rebuilt with its saved id and motion
//	   (B4b); the deployed squad's model -- an NPC, which no unit test can
//	   build -- answers to its saved id (B2b's review asked a playtest for
//	   that proof).
//	5  Saved again at once: the file is T's file byte for byte, but saved_at
//	   and the sidecar's generation (which is saved_at).
//	6  120 world minutes: S_R = S_U.
//	6b THE IN-PROCESS PATH: he dies, and the death screen's "load last save"
//	   (Enter; App.ReloadGame) resumes the saved moment -- not the dawn -- in
//	   this process: S = S_T again, the uuid stream and the script's dials
//	   put back by the load.
//	6c-6e THE REFUSALS, each falling back to dawn with the file set aside
//	   (rule 7) and his own sidecar, byte for byte, beside it: another hero's
//	   world file (6c) -- started on seed 7, which the file was not saved on:
//	   a file step 1 refuses anyway holds the script to no seed (the review's
//	   C2) -- a torn save, the sidecar of another generation (6d), and a
//	   changed map (6e, D5), refused after the game opened and so tearing that
//	   game down: THE REVIEW'S A1 PROBE -- the files as act 3 left them, the
//	   sidecar two hours later than the file, and after the teardown his
//	   sidecar is still act 3's, not the file's copy (BUG-60).
//	6f THE REVIEW'S A2 PROBE (BUG-61): a session that did not resume the file
//	   -- a network game's, rebuilt here by hiding the file, as the review did
//	   -- earns experience; the next single-player load must not write the
//	   file's older copy over it. The file reads TORN (that session's saves
//	   carry no generation of it) and is set aside; his experience stays.
//	6g THE REVIEW'S B1 PROBE (BUG-62; Windows): a file refused after its game
//	   opened, held open so it cannot be set aside. One teardown, one dawn --
//	   not a reload loop -- and his own sidecar beside it.
//	6h THE RE-KEY AS THE SHIPPED GAME NEEDS IT (B4b). T's file with its
//	   villagers' ids made another launch's (crypto/rand gives the shipped
//	   game's villagers new ids every launch; the harness's uuid stream gives
//	   them the file's): the load re-keys each villager in place to the file's
//	   id, and the resumed game saves that file, byte for byte but its stamp.
//	8  RULE 4 (B4b): he is walked, saved mid-stride, and loaded; he stands
//	   where he was saved, and every system, entity and part is the saved
//	   moment's but his walk (sameWorldExcept, hisWalk).
//	6i A SAVE IN THE DEATH WINDOW (the B4b review fixes, BUG-75, and BUG-87;
//	   the review's real-game probe, run after act 8): T resumed with B4b's
//	   quick-resolving dials, the pack's fight quick-resolved; each member it
//	   slew lies where his fall was recorded; the save is made at once, on
//	   the first try, while the slain still play their deaths, each saved at
//	   its frame; and that moment resumes exactly -- the deaths at their
//	   frames -- runs on the same through the deaths' ends, and each slain
//	   lies where the saved game's lies. (BUG-76 had refused the save until
//	   the last lay; BUG-87 carries the deaths instead.)
//
// THE COMPARISON (BUG-58 fixed, 29 Sep 2026): the digest's resume_digest is
// every part a resumed game must reproduce -- not sim (the harness's clock)
// and not process (this process's history: the files it loaded, the games it
// began, the torch verbs it counted) -- and screen coordinates are in no part.
//
// FOR THE NEGATIVE CONTROLS, two knobs, read here only: STRIGOI_SAVE_RESUME_
// ONLY=b4a skips act 7, and STRIGOI_SAVE_RESUME_FROM=<dir> takes acts 1-3
// from an evening a green run kept there (the kit's hero files at T and S_T,
// S_U) and runs acts 4-8 against it, so a control that breaks the LOAD is
// seen in seconds (TestSaveResumeNegatives needs the same knob). The green run keeps its evening at
// <run dir>/pt/TestSaveResume/evening.
func TestSaveResume(t *testing.T) {
	if os.Getenv("STRIGOI_SAVE_RESUME_ONLY") != "b4a" {
		saveVerbActs(t)
	}

	ev := eveningActs1to3(t)
	eveningActs4to6(t, ev)
}

// TestSaveResumeInTheFirstSecond is BUG-86 (the merge scout's; the B4b review
// fixes): a save made in a new game's first second resumes as the saved
// moment. A new game first reads its region a second into play, and a load
// reads it at once, so the resumed game had the village's sound environment
// and song where the saved one had neither -- in the digest's world part,
// which a resume compares: S_R0 differed from S_T on village.sound_env and
// village.music alone. They are this process's presentation now (the village
// provider's process part, as BUG-63 moved the ui's), and the premise is
// asserted both ways: at T the region is not read yet, and the resumed game
// has read it.
func TestSaveResumeInTheFirstSecond(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Early", "hero_class": "amazon", "seed": 99, "wait_seconds": 90,
	})
	save := str(game, "save_path")

	s.call("strigoi_step", map[string]any{"frames": 30}) // half a second

	if v := villageState(s); str(v, "music") != "" || mustNum(t, v, "sound_env") != 0 {
		t.Fatalf("the premise: half a second in, the region is not read yet (no song, no sound environment): %v", v)
	}

	sT := snapWorld(t, s)
	s.call("strigoi_save_game", map[string]any{})
	sameWorld(t, "the save moved nothing", sT, snapWorld(t, s))

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	g := s.call("strigoi_start_game", map[string]any{"save_path": save, "wait_seconds": 90})
	if load := sub(g, "load"); !flag(t, load, "resumed") {
		t.Fatalf("the save made half a second in resumes: %v", load)
	}

	sameWorld(t, "S_R0 = S_T (a save in the first second)", sT, snapWorld(t, s))

	if v := villageState(s); str(v, "music") == "" {
		t.Fatalf("the premise's other half: the load read the region at once, so the resumed game has the song the saved one had not yet: %v", v)
	}

	t.Logf("PASS: saved half a second into a new game, resumed as the saved moment (the village's sound differs and is this process's)")
}

// evening is what acts 1-3 hand acts 4-6: the hero's files at T and the two
// states the resumed game must reproduce.
type evening struct {
	save  string // his .od2 in this test's home
	fileT []byte // the world file written at T

	// od2T and sidecarT are his .od2 and sidecar as the save at T wrote
	// them; od2Left and sidecarLeft as he LEFT, through the menu, two hours
	// and a wound later -- the files a load finds beside the world file,
	// which must lose to it.
	od2T, sidecarT       []byte
	od2Left, sidecarLeft []byte

	sT, sU    worldSnap
	savedDial []dialWrite

	// squadModel is the deployed squad's model as the file at T names it
	// (B4b): an NPC the load must rebuild wearing that id.
	squadModel string
}

type dialWrite struct {
	System, Field string
	Value         any
}

// startDials are the dials act 1 sets as the game begins: no pack, nothing
// notices him from further than half a tile, and the dead rise only when the
// script says.
var startDials = []dialWrite{
	{"spawns", "chance", 0},
	{"spawns", "notice_radius", 0.5},
	{"rising", "edge_floor", 0},
	{"rising", "hasty_weight", 0.0},
	{"rising", "p", 0.0},
}

// The hunted night's dials (B4b): the notice radius opened so the pack sees
// him from where it arrives, room on the tables for its group; and (the B4b
// review fixes, BUG-77) the fight after T fought blow by blow, every blow a
// graze, so he lives through it.
const (
	huntedNoticeRadius = 24.0
	huntedMaxGroups    = 4

	// quickResolveOff: above 1 the quick resolve can never fire (Combat's
	// own dial, "how quick-resolve is turned OFF").
	quickResolveOff = 2.0
)

// huntedDials are the dials acts 1-3 leave set at T. A dial is never saved
// (trap 7): a relaunched game has the defaults until the script sets them
// again, and "load last save" in one process has them re-applied by the load.
//
// THE FIGHT AFTER T LANDS BLOWS (the B4b review fixes, BUG-77; the review's
// B2). B4b finished it before its first blow (quickResolvedDials, decision
// B4b-10), so after T no combat roll was drawn, no rebuilt monster swung or
// was struck, no restored speed was paced and no restored pack profile read:
// a divergence only a blow shows passed acts 4-8 green. Now the quick resolve
// is off and every blow is a graze, both ways: measured on evening-2
// (wt-b4b-fix\measure-1.txt), the two hours after T hold two fights of 13
// rounds and 45 blows, ending enemies_routed, and he lives at 214 of 237 --
// in the same 9 s of wall time, and two resumes of T agree to the digest.
var huntedDials = []dialWrite{
	{"spawns", "chance", 0},
	{"spawns", "notice_radius", huntedNoticeRadius},
	{"spawns", "max_groups", huntedMaxGroups},
	{"rising", "edge_floor", 0},
	{"rising", "hasty_weight", 0.0},
	{"rising", "p", 0.0},
	{"combat", "forced_band", "graze"},
	{"combat", "quick_resolve_advantage", quickResolveOff},
}

// quickResolvedDials are B4b's dials at T (decision B4b-10): the fight after T
// finished before its first blow. An evening kept before the review fixes
// (evening-2) was made with them -- keptEvening falls back to them when the
// evening names none -- and act 6i uses them: a quick resolve is where a save
// finds the slain still falling.
var quickResolvedDials = []dialWrite{
	{"spawns", "chance", 0},
	{"spawns", "notice_radius", huntedNoticeRadius},
	{"spawns", "max_groups", huntedMaxGroups},
	{"rising", "edge_floor", 0},
	{"rising", "hasty_weight", 0.0},
	{"rising", "p", 0.0},
	{"combat", "forced_band", ""},
	{"combat", "quick_resolve_advantage", 0.0},
}

// worldSnap is the state a resumed game must reproduce, as the harness reports
// it: the resume digest and its parts, each system's world-state hash, each
// system's whole state (for the diff a failure prints), and every entity
// without its per-process handle.
type worldSnap struct {
	Resume   string                     `json:"resume"`
	Parts    map[string]string          `json:"parts"`
	Systems  map[string]string          `json:"systems"`
	States   map[string]json.RawMessage `json:"states"`
	Entities map[string]json.RawMessage `json:"entities"`
}

func snapWorld(t *testing.T, s *session) worldSnap {
	t.Helper()

	d := s.call("strigoi_get_state_digest", map[string]any{})
	w := worldSnap{
		Resume: str(d, "resume_digest"), Parts: map[string]string{}, Systems: map[string]string{},
		States: map[string]json.RawMessage{}, Entities: map[string]json.RawMessage{},
	}

	if w.Resume == "" {
		t.Fatalf("the digest reports no resume_digest: %v", d)
	}

	for k, v := range sub(d, "parts") {
		w.Parts[k] = fmt.Sprint(v)
	}

	for k, v := range sub(d, "systems") {
		w.Systems[k] = fmt.Sprint(v)

		raw, _ := json.Marshal(sub(s.call("strigoi_get_system_state", map[string]any{"system": k}), "state"))
		w.States[k] = raw
	}

	for _, raw := range asList(s.call("strigoi_get_entities", map[string]any{"limit": 500})["items"]) {
		e, _ := raw.(map[string]any)
		full := s.call("strigoi_get_entity", map[string]any{"handle": str(e, "handle")})
		delete(full, "handle")
		delete(full, "screen")

		line, _ := json.Marshal(full)
		w.Entities[str(full, "id")] = line
	}

	return w
}

// sameWorld requires b to be the world a was, naming every part, system and
// entity that differs and, for a system, every field (the whole state, screen
// coordinates and all, which the digest itself leaves out).
func sameWorld(t *testing.T, act string, a, b worldSnap) {
	t.Helper()

	if a.Resume == b.Resume {
		return
	}

	var why []string

	for _, p := range []string{"world", "entities", "rng", "systems"} {
		if a.Parts[p] != b.Parts[p] {
			why = append(why, "part "+p)
		}
	}

	names := keysOfSnap(a.Systems, b.Systems)

	for _, n := range names {
		if a.Systems[n] == b.Systems[n] {
			continue
		}

		why = append(why, "system "+n)

		var ma, mb map[string]any
		_ = json.Unmarshal(a.States[n], &ma)
		_ = json.Unmarshal(b.States[n], &mb)

		for _, k := range keysOfAny(ma, mb) {
			ja, _ := json.Marshal(ma[k])
			jb, _ := json.Marshal(mb[k])

			if string(ja) != string(jb) {
				why = append(why, fmt.Sprintf("  %s.%s\n      was: %s\n      now: %s", n, k, cut(string(ja)), cut(string(jb))))
			}
		}
	}

	for _, id := range keysOfRaw(a.Entities, b.Entities) {
		if ea, eb := compact(a.Entities[id]), compact(b.Entities[id]); ea != eb {
			why = append(why, fmt.Sprintf("  entity %s\n      was: %s\n      now: %s", id, cut(ea), cut(eb)))
		}
	}

	t.Fatalf("%s: the resumed world is not the saved one (resume digest %.12s, want %.12s):\n%s",
		act, b.Resume, a.Resume, strings.Join(why, "\n"))
}

func keysOfSnap(a, b map[string]string) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}

	for k := range b {
		seen[k] = true
	}

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

func keysOfAny(a, b map[string]any) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}

	for k := range b {
		seen[k] = true
	}

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

func keysOfRaw(a, b map[string]json.RawMessage) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}

	for k := range b {
		seen[k] = true
	}

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

// compact is a JSON value without its whitespace (a kept evening's
// states.json is written indented).
func compact(raw json.RawMessage) string {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return string(raw)
	}

	return b.String()
}

func cut(s string) string {
	if len(s) > 900 {
		return s[:900] + "..."
	}

	return s
}

// eveningEpoch is where the clock starts, in minutes of day 0 (02:45).
const eveningEpoch = 2*60 + 45

// stepTo steps the world to minute m of day d, a few minutes at a time,
// keeping him fed and watered and rested while it does (act 1's evening is
// two days long, and water empties by 18:09; the writes are state, and none
// is made after T).
func stepTo(t *testing.T, s *session, day int, m float64) {
	t.Helper()

	target := float64(day*1440) + m - eveningEpoch

	for i := 0; i < 400; i++ {
		now := mustNum(t, clockState(s), "world_minutes")
		if now >= target {
			return
		}

		s.call("strigoi_step_world", map[string]any{"world_minutes": math.Min(20, target-now)})
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
	}

	t.Fatalf("the clock never reached day %d minute %.0f: %v", day, m, clockState(s))
}

// bodyAt is one body of the corpse registry by id.
func bodyAt(t *testing.T, s *session, id string) map[string]any {
	t.Helper()

	for _, raw := range asList(corpsesState(s)["bodies"]) {
		b, _ := raw.(map[string]any)
		if str(b, "id") == id {
			return b
		}
	}

	t.Fatalf("no body %s: %v", id, corpsesState(s))

	return nil
}

// eveningActs1to3 fills the evening, saves at T and steps on to T+120 (acts
// 1-3), or takes a kept one (STRIGOI_SAVE_RESUME_FROM, the controls' knob).
func eveningActs1to3(t *testing.T) evening {
	t.Helper()

	if from := os.Getenv("STRIGOI_SAVE_RESUME_FROM"); from != "" {
		return keptEvening(t, from)
	}

	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Evening", "hero_class": "amazon", "seed": 99, "wait_seconds": 90,
	})

	ev := evening{save: str(game, "save_path")}

	// What a fresh game gives him, for act 1's precondition on rule 4 (A3).
	fresh := heroOf(t, s)

	for _, d := range startDials {
		setField(s, d.System, d.Field, d.Value)
	}

	// The night's dead rise in the first night, and nothing notices him.
	setField(s, "rising", "p", 1.0)
	setField(s, "village", "rep", 30.0) // state: the headman offers the watch

	// --- 1: a fight by day: a dog slain, left lying where he fell ---------
	// (B4a took him off the map with his body: a monster on the map was B4b's.
	// B4b keeps him -- a corpse entity, held in the Dead pose, his body at 0.)
	pl := s.call("strigoi_get_player", map[string]any{})
	spot := clearNeighbour(t, s, num(pl, "x"), num(pl, "y"))
	dog := spawnNPC(t, s, "fallen1", spot[0], spot[1])

	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": str(pl, "handle")})
	fightNow(t, s)

	for i := 0; i < 8 && flag(t, combatState(s), "fighting"); i++ {
		setField(s, "combat", "forced_band", "crit")
		stepToNewRound(t, s)
	}

	setField(s, "combat", "forced_band", "")

	if got := str(combatState(s), "ended_reason"); got != "enemies_dead" {
		t.Fatalf("act 1: the dog must die for the file to carry a fight's counters; ended_reason %q", got)
	}

	s.call("strigoi_step", map[string]any{"frames": 4})

	// --- 1 (B4b): a fight with a wounded survivor, ended long before T ------
	// A second dog at his side, noticed (by day: in the dark at half a tile's
	// notice radius nothing sees him); a round of grazes each way; then he
	// disengages, and the dog is let go (unwatched), wounded, alive -- he
	// stands where he was until T and after.
	pl = s.call("strigoi_get_player", map[string]any{})
	beside := otherNeighbour(t, s, num(pl, "x"), num(pl, "y"), spot)
	survivor := spawnNPC(t, s, "fallen1", beside[0], beside[1])

	s.call("strigoi_watch", map[string]any{"watcher": survivor, "target": str(pl, "handle")})
	fightNow(t, s)
	setField(s, "combat", "forced_band", "graze")
	stepToNewRound(t, s)
	setField(s, "combat", "forced_band", "")

	if !flag(t, combatState(s), "fighting") {
		t.Fatalf("act 1 (B4b): a round of grazes leaves the fight running: %v", combatState(s))
	}

	setField(s, "combat", "disengage", true)
	s.call("strigoi_watch", map[string]any{"watcher": survivor, "target": str(pl, "handle"), "release": true})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, combatState(s), "fighting") || str(combatState(s), "ended_reason") != "disengaged" {
		t.Fatalf("act 1 (B4b): the fight ended, disengaged: %v", combatState(s))
	}

	// --- 1: two forages, three stakes -------------------------------------
	for i := 0; i < 2; i++ {
		s.call("strigoi_key", map[string]any{"key": "k"})
		s.call("strigoi_step", map[string]any{"frames": 2})
	}

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	for i := 0; i < 3; i++ {
		clickRecipe(t, s, "whittle-stake")
	}

	s.call("strigoi_key", map[string]any{"key": "i"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- 1: Night 1's dead: one staked, one in a hasty grave --------------
	for _, v := range []struct{ id, key, state string }{{"dead:1", "x", "closed"}, {"dead:2", "d", "hasty"}} {
		b := bodyAt(t, s, v.id)
		walkTo(t, s, num(b, "x"), num(b, "y"))
		s.call("strigoi_key", map[string]any{"key": v.key})
		s.call("strigoi_step", map[string]any{"frames": 2})

		if got := str(bodyAt(t, s, v.id), "state"); got != v.state {
			t.Fatalf("act 1: %s is %s after %s: %v", v.id, got, v.key, corpsesState(s))
		}
	}

	// --- 1: the first night: the two left open rise ------------------------
	stepTo(t, s, 0, 21*60+30)

	for _, id := range []string{"dead:3", "dead:4"} {
		if got := str(bodyAt(t, s, id), "state"); got != "risen" {
			t.Fatalf("act 1: at certain odds %s rises in the first band: %s", id, got)
		}
	}

	setField(s, "rising", "p", 0.0)

	// --- 1: first light lays them down; he stakes both by day -------------
	stepTo(t, s, 1, 4*60)

	for _, id := range []string{"dead:3", "dead:4"} {
		b := bodyAt(t, s, id)
		if str(b, "state") != "downed" {
			t.Fatalf("act 1: first light lays %s down: %v", id, b)
		}

		walkTo(t, s, num(b, "x"), num(b, "y"))
		s.call("strigoi_key", map[string]any{"key": "x"})
		s.call("strigoi_step", map[string]any{"frames": 2})

		if got := str(bodyAt(t, s, id), "state"); got != "closed" {
			t.Fatalf("act 1: the risen %s, staked by day, is Closed: %s", id, got)
		}
	}

	// --- 1: the talk: the watch promised ----------------------------------
	headman := villager(t, s, "Warriv")
	walkNear(t, s, headman)
	openTalkWith(t, s, headman)

	if str(villageState(s), "node") == "headman_first" {
		answer(t, s, 3)
		openTalkWith(t, s, headman)
	}

	answer(t, s, answerIndex(t, s, "stand the watch"))
	answer(t, s, 1)

	if !hasFlag(t, s, "watch_promised") {
		t.Fatalf("act 1: the watch is promised: %v", villageState(s))
	}

	// --- 1: the second night, at the post ----------------------------------
	stepTo(t, s, 1, 21*60+20)
	walkNear(t, s, headman)

	stepTo(t, s, 1, 22*60+30)

	// B4b: a deployed squad -- a second squad whose model is an NPC on the
	// map (squadDeployer) -- which the load must rebuild wearing its saved id.
	s.call("strigoi_set_system_field", map[string]any{"system": "meters", "field": "squad_add", "value": map[string]any{}})

	if n := len(asList(metersState(s)["squads"])); n != 2 {
		t.Fatalf("act 1 (B4b): squad_add deploys a second squad: %d squads", n)
	}

	// --- 1: the torch lit late ---------------------------------------------
	stepTo(t, s, 2, 60+15)

	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if !flag(t, lightState(s), "carried_lit") {
		t.Fatalf("act 1: L lights his torch: %v", lightState(s))
	}

	stepTo(t, s, 2, 90)
	setField(s, "meters", "food", 90.0)
	setField(s, "meters", "water", 90.0)
	setField(s, "meters", "fatigue", 10.0)
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- 1: every field the load restores on him, not a fresh game's (A3) --
	// Seven minutes parched: a fraction of a health point owed to neglect
	// (2 an hour: 0.23, no whole point taken), then water again.
	setField(s, "meters", "water", 0.0)
	s.call("strigoi_step_world", map[string]any{"world_minutes": 7})
	setField(s, "meters", "water", 90.0)

	// A talent taken with the pick his level gave him (Long Marches: it
	// changes no health).
	s.call("strigoi_key", map[string]any{"key": "t"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	clickCell(t, s, "long-marches")
	clickCell(t, s, "long-marches")
	s.call("strigoi_key", map[string]any{"key": "t"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// His run toggle on, by the HUD's button.
	clickRunButton(t, s)

	// --- 1 (B4b): a pack arrives and notices him; the chase begins ----------
	// The table forced (certain odds), then shut; then the notice radius
	// opened so the pack sees him. T is the first frames of the chase: the
	// pack is walking. The two hours after T hold its fight -- the walk in,
	// the engage, the rounds, the rout -- which the resumed game must play
	// the same, on the same dice. The fight is fought blow by blow (the B4b
	// review fixes, BUG-77: B4b quick-resolved it before its first blow, and
	// no roll or blow after T was ever compared), every blow a graze both
	// ways, so he lives through it whatever the pack: a death there would end
	// the evening's later acts.
	groupsBefore := map[string]bool{}
	for _, raw := range asList(spawnsState(s)["group_list"]) {
		g, _ := raw.(map[string]any)
		groupsBefore[str(g, "group")] = true
	}

	setField(s, "spawns", "max_groups", huntedMaxGroups)
	setField(s, "spawns", "chance", 100)

	pack := ""
	for i := 0; i < 30 && pack == ""; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1})

		for _, raw := range asList(spawnsState(s)["group_list"]) {
			g, _ := raw.(map[string]any)
			if !groupsBefore[str(g, "group")] && str(g, "row") != "risen" {
				pack = str(g, "group")
			}
		}
	}

	setField(s, "spawns", "chance", 0)

	if pack == "" {
		t.Fatalf("act 1 (B4b): a certain table never placed a pack: %v", spawnsState(s))
	}

	setField(s, "combat", "quick_resolve_advantage", quickResolveOff)
	setField(s, "combat", "forced_band", "graze")
	setField(s, "spawns", "notice_radius", huntedNoticeRadius)

	for i := 0; i < 200 && !packChasing(t, s, pack); i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})
	}

	if !packChasing(t, s, pack) || flag(t, combatState(s), "fighting") {
		t.Fatalf("act 1 (B4b): %s notices him and gives chase, no fight yet: fighting %v, %v",
			pack, combatState(s)["fighting"], sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")["chase_list"])
	}

	// His wind below its maximum (the village's tiles are a town's, where
	// running never spends it, so the meters' test-setup field stands him at
	// a wind), last: it comes back while he stands.
	setField(s, "meters", "stamina", 31.25)
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- 1: THE PRECONDITION: every block is non-empty ---------------------
	eveningPrecondition(t, s)
	huntedPrecondition(t, s, pack, entityID(t, s, dog), entityID(t, s, survivor))
	heroNotFresh(t, s, fresh)

	// --- 2: the save at T --------------------------------------------------
	atT := combatState(s)
	ev.sT = snapWorld(t, s)
	out := s.call("strigoi_save_game", map[string]any{})
	sameWorld(t, "act 2 (the save moved nothing)", ev.sT, snapWorld(t, s))

	world := str(out, "world_path")
	ev.fileT, ev.od2T, ev.sidecarT = mustRead(t, world), mustRead(t, ev.save), mustRead(t, ev.save+".strigoi.json")

	// B4b: the file holds the hunted night -- every block B4a refused.
	file := worldFile(t, "act 2", ev.fileT)
	for _, block := range []string{"spawns.groups", "notice.watches", "pursuit.chases", "bodies", "entities"} {
		if n := len(listAt(t, file, block)); n == 0 {
			t.Fatalf("act 2: a hunted night's file has %s: none", block)
		}
	}

	nonNative := 0
	for _, raw := range listAt(t, file, "entities") {
		if e, _ := raw.(map[string]any); !flag(t, e, "native") {
			nonNative++
		}
	}

	if deployed := len(listAt(t, file, "squads.squads")); nonNative == 0 || deployed < 2 {
		t.Fatalf("act 2: a hunted night's file names entities the map does not build (%d) and a deployed squad (%d squads)", nonNative, deployed)
	}

	ev.squadModel = deployedModel(t, file)

	t.Logf("act 2 PASS: saved at %s (%s, day %v), %d bytes, the digest unmoved",
		str(clockState(s), "time_of_day"), str(clockState(s), "stage"), clockState(s)["day_index"], len(ev.fileT))

	// --- 3: 120 world minutes on; quit -------------------------------------
	s.call("strigoi_step_world", map[string]any{"world_minutes": 120})
	ev.sU = snapWorld(t, s)

	if ev.sU.Resume == ev.sT.Resume {
		t.Fatal("act 3: 120 minutes moved nothing, so act 6 would compare nothing")
	}

	// B4b: the pack walked in and its fight was played (and quick-resolved,
	// the dial at T) -- he lived through the two hours.
	if flag(t, uiState(s), "death_open") || mustNum(t, sub(s.call("strigoi_get_player", map[string]any{}), "state"), "health") <= 0 {
		t.Fatalf("act 3: he lives through the two hours after T: %v", combatState(s))
	}

	t.Logf("act 3: in the two hours the fights ran to %v encounters (%q the last's end)",
		combatState(s)["encounters"], str(combatState(s), "ended_reason"))

	// The B4b review fixes (BUG-77): the fight after T landed blows -- rolls
	// drawn, rebuilt monsters swinging and struck -- and none was
	// quick-resolved, so act 6 compares a fight, not its absence.
	atU := combatState(s)
	if blows := mustNum(t, atU, "actions_total") - mustNum(t, atT, "actions_total"); blows <= 0 ||
		mustNum(t, atU, "rounds") <= mustNum(t, atT, "rounds") ||
		mustNum(t, atU, "quick_resolved") != mustNum(t, atT, "quick_resolved") {
		t.Fatalf("act 3: the fight after T is fought blow by blow: %v blows, rounds %v -> %v, quick_resolved %v -> %v",
			blows, atT["rounds"], atU["rounds"], atT["quick_resolved"], atU["quick_resolved"])
	}

	t.Logf("act 3: after T, %v rounds and %v blows, none quick-resolved",
		mustNum(t, atU, "rounds")-mustNum(t, atT, "rounds"), mustNum(t, atU, "actions_total")-mustNum(t, atT, "actions_total"))

	c := clockState(s)
	if str(c, "stage") == "night" || flag(t, lightState(s), "carried_lit") {
		t.Fatalf("act 3: the two hours cross dawn and outlast the torch (act 6's teeth): %s %s, light %v",
			str(c, "time_of_day"), str(c, "stage"), lightState(s))
	}

	// He leaves through the menu, wounded: SAVE AND EXIT writes his .od2 and
	// his sidecar (OnUnload) and not the world file (B5's) -- so the load in
	// act 4 finds an .od2 of another health and a sidecar with no torch (it
	// burnt out) beside the world file of T, and the world file must win.
	hurt := mustNum(t, sub(s.call("strigoi_get_player", map[string]any{}), "state"), "health") - 37
	setField(s, "meters", "health", hurt)
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	ev.od2Left, ev.sidecarLeft = mustRead(t, ev.save), mustRead(t, ev.save+".strigoi.json")

	if bytes.Equal(ev.od2Left, ev.od2T) || bytes.Equal(ev.sidecarLeft, ev.sidecarT) {
		t.Fatal("act 3: leaving through the menu rewrites his .od2 and sidecar (the files the world file must beat in act 4)")
	}

	t.Logf("act 3 PASS: at %s (%s): dawn came, the torch burnt out; S_U taken; he left through the menu at %.0f health",
		str(c, "time_of_day"), str(c, "stage"), hurt)

	ev.savedDial = huntedDials
	keepEvening(t, s, ev)
	s.stop()

	return ev
}

// eveningPrecondition is act 1's: every block B4a resumes holds something.
func eveningPrecondition(t *testing.T, s *session) {
	t.Helper()

	scene := sub(s.call("strigoi_get_system_state", map[string]any{"system": "scene"}), "state")
	c := corpsesState(s)
	light := lightState(s)
	village := villageState(s)
	combat := combatState(s)

	checks := []struct {
		what string
		ok   bool
	}{
		{"the clock at night", str(clockState(s), "stage") == "night"},
		{"his torch lit and carried", flag(t, light, "carried_lit")},
		{"the kit: his loadout chosen", !flag(t, uiState(s), "choosing_loadout")},
		{"forage: the land gathered from", mustNum(t, village, "land_left") < 12},
		{"a talk: the watch promised", hasFlag(t, s, "watch_promised")},
		{"part of a watch stood", mustNum(t, village, "watch_stood") > 0},
		{"a stake and a dig", mustNum(t, c, "hasty_human") == 1 && mustNum(t, c, "closed_human") == 3},
		{"a risen body Closed", len(saveBlock(t, c, "risen_as")) >= 2 && len(saveBlock(t, c, "last")) >= 2},
		{"a fight's counters", mustNum(t, combat, "encounters") >= 1},
		{"the rising rolled", mustNum(t, saveBlock(t, risingState(s), "rng"), "draws") > 0},
		{"the scene's dead laid", len(asList(scene["field_dead"])) == 4},
	}

	for _, c := range checks {
		if !c.ok {
			t.Fatalf("act 1: the precondition fails: %s", c.what)
		}
	}
}

// heroFields is what the load restores on him (rule 4, step 5) and what a
// fresh game gives him instead.
type heroFields struct {
	Run       bool
	Stamina   float64
	Max       float64
	Facing    float64
	Talents   int
	OwedHurts float64
}

func heroOf(t *testing.T, s *session) heroFields {
	t.Helper()

	p := sub(s.call("strigoi_get_player", map[string]any{}), "state")

	return heroFields{
		Run: flag(t, p, "run_toggled"), Stamina: mustNum(t, p, "stamina"), Max: mustNum(t, p, "max_stamina"),
		Facing: mustNum(t, p, "direction"), Talents: len(asList(progressState(s)["talents"])),
		OwedHurts: mustNum(t, metersState(s), "damage_owed"),
	}
}

// heroNotFresh is act 1's precondition on rule 4 (the B4a review, A3): every
// field the load restores on him differs from what a fresh game gives him, so
// a restore left out cannot pass for one made. Before, the run toggle was off
// and his wind full at T -- both a fresh game's -- and the controls that took
// out their restores stayed green.
func heroNotFresh(t *testing.T, s *session, fresh heroFields) {
	t.Helper()

	at := heroOf(t, s)

	checks := []struct {
		what string
		ok   bool
	}{
		{"his run toggle on (a fresh game's is off)", at.Run && !fresh.Run},
		{"his wind below its maximum (a fresh game's is full)", at.Stamina < at.Max && fresh.Stamina == fresh.Max},
		{"a talent taken (a fresh hero has none)", at.Talents > 0 && fresh.Talents == 0},
		{"a fraction of a health point owed to neglect (a fresh body owes none)", at.OwedHurts > 0 && fresh.OwedHurts == 0},
		{"his facing not a fresh game's", at.Facing != fresh.Facing},
	}

	for _, c := range checks {
		if !c.ok {
			t.Fatalf("act 1: the precondition on rule 4 fails: %s (at T %+v; fresh %+v)", c.what, at, fresh)
		}
	}

	t.Logf("act 1: at T he differs from a fresh game in every restored field: %+v (fresh %+v)", at, fresh)
}

// packChasing is whether the pack's group is aware of him and one of its
// members is chasing.
func packChasing(t *testing.T, s *session, pack string) bool {
	t.Helper()

	g := groupNamed(t, s, pack)
	if g == nil || mustNum(t, g, "aware") < 1 {
		return false
	}

	members := stringsOf(g["member_ids"])

	for _, raw := range asList(sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")["chase_list"]) {
		c, _ := raw.(map[string]any)
		if hasString(members, str(c, "hunter")) {
			return true
		}
	}

	return false
}

// huntedPrecondition is act 1's for the hunted night (B4b): every block B4a
// refused holds something at T -- a pack aware of him, one of it walking a
// chase (a walk in progress: his motion is saved and must be restored), no
// fight, a slain monster lying on the map with his body at 0, a wounded
// survivor of an ended fight, and a deployed squad whose model is an NPC on
// the map.
func huntedPrecondition(t *testing.T, s *session, pack, slain, survivor string) {
	t.Helper()

	walking := false

	for _, id := range stringsOf(groupNamed(t, s, pack)["member_ids"]) {
		if h := handleOfID(t, s, id); h != "" {
			st := sub(s.call("strigoi_get_entity", map[string]any{"handle": h}), "state")
			if len(asList(st["waypoints"])) > 0 {
				walking = true
			}
		}
	}

	bodies := map[string]map[string]any{}
	for _, raw := range asList(sub(s.call("strigoi_get_system_state", map[string]any{"system": "scene"}), "state")["bodies"]) {
		b, _ := raw.(map[string]any)
		bodies[str(b, "id")] = b
	}

	squads := asList(metersState(s)["squads"])
	model := ""

	if len(squads) == 2 {
		sq, _ := squads[1].(map[string]any)
		if models := asList(sq["models"]); len(models) == 1 {
			m, _ := models[0].(map[string]any)
			model = str(m, "entity")
		}
	}

	checks := []struct {
		what string
		ok   bool
	}{
		{"a pack aware of him", mustNum(t, groupNamed(t, s, pack), "aware") >= 1},
		{"one of the pack chasing him", packChasing(t, s, pack)},
		{"a chaser mid-walk (his waypoints ahead of him)", walking},
		{"no fight (a save is refused mid-fight)", !flag(t, combatState(s), "fighting")},
		{"the slain dog lying on the map, his body at 0", onTheMap(s, slain) && bodies[slain] != nil && num(bodies[slain], "health") == 0},
		{"the wounded survivor: alive, below his maximum", bodies[survivor] != nil &&
			num(bodies[survivor], "health") > 0 && num(bodies[survivor], "health") < num(bodies[survivor], "max_health")},
		{"a deployed squad, its model on the map", model != "" && onTheMap(s, model)},
	}

	for _, c := range checks {
		if !c.ok {
			t.Fatalf("act 1 (B4b): the precondition fails: %s (pack %v; bodies %v; squads %d)", c.what, groupNamed(t, s, pack), bodies, len(squads))
		}
	}

	t.Logf("act 1 (B4b): a hunted night at T: %s aware and chasing, a chaser mid-walk, the slain %s at 0, the survivor %s at %v of %v, the deployed model %s",
		pack, slain, survivor, bodies[survivor]["health"], bodies[survivor]["max_health"], model)
}

// otherNeighbour is an orthogonal neighbour of px, py with a clear line to
// it that is not taken (where the slain dog lies).
func otherNeighbour(t *testing.T, s *session, px, py float64, taken [2]float64) [2]float64 {
	t.Helper()

	for _, d := range [][2]float64{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
		spot := [2]float64{px + d[0], py + d[1]}
		if spot != taken && flag(t, s.call("strigoi_find_path", map[string]any{"to_x": spot[0], "to_y": spot[1]}), "straight_line_clear") {
			return spot
		}
	}

	t.Fatalf("no second clear neighbour of %.2f,%.2f", px, py)

	return [2]float64{}
}

// handleOfID is the harness handle of the entity with this id, or "".
func handleOfID(t *testing.T, s *session, id string) string {
	t.Helper()

	for _, raw := range asList(s.call("strigoi_get_entities", map[string]any{"limit": 500})["items"]) {
		e, _ := raw.(map[string]any)
		if str(e, "id") == id {
			return str(e, "handle")
		}
	}

	return ""
}

// deployedModel is the deployed squad's model the file names (B4b).
func deployedModel(t *testing.T, file map[string]any) string {
	t.Helper()

	for _, raw := range listAt(t, file, "squads.squads") {
		sq, _ := raw.(map[string]any)
		if str(sq, "id") == "s:1" {
			continue
		}

		for _, rm := range asList(sq["members"]) {
			if m, _ := rm.(map[string]any); str(m, "entity") != "" {
				return str(m, "entity")
			}
		}
	}

	t.Fatalf("the file names no deployed squad model: %v", sub(file, "squads"))

	return ""
}

// modelAnswers is B2b's review's runtime proof (B4b): the deployed squad's
// model -- an NPC, which no unit test can build -- is on the resumed map
// under the id the file saved it as, an npc of the deployer's monstat, and
// the squad names it by that id.
func modelAnswers(t *testing.T, s *session, act, model string) {
	t.Helper()

	h := handleOfID(t, s, model)
	if h == "" {
		t.Fatalf("%s: the deployed model %s is not on the resumed map under its saved id", act, model)
	}

	e := s.call("strigoi_get_entity", map[string]any{"handle": h})
	if str(e, "kind") != "npc" || str(sub(e, "state"), "monstat") != "fallen1" {
		t.Fatalf("%s: the deployed model %s is an npc of monstat fallen1: %v", act, model, e)
	}

	named := false

	for _, raw := range asList(metersState(s)["squads"]) {
		sq, _ := raw.(map[string]any)
		for _, rm := range asList(sq["models"]) {
			if m, _ := rm.(map[string]any); str(m, "entity") == model {
				named = true
			}
		}
	}

	if !named {
		t.Fatalf("%s: no squad names the model %s: %v", act, model, metersState(s)["squads"])
	}

	t.Logf("%s: the deployed squad's model answers to its saved id %s (an npc, fallen1)", act, model)
}

// keepEvening writes acts 1-3's evening beside the run's logs, for the
// controls (STRIGOI_SAVE_RESUME_FROM).
func keepEvening(t *testing.T, s *session, ev evening) {
	t.Helper()

	if s.RunBase == "" {
		return
	}

	dir := filepath.Join(s.RunBase, "pt", safeName(t.Name()), "evening")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Logf("keeping the evening: %v", err)
		return
	}

	states, _ := json.MarshalIndent(map[string]worldSnap{"t": ev.sT, "u": ev.sU}, "", " ")
	dials, _ := json.MarshalIndent(ev.savedDial, "", " ")

	for name, data := range map[string][]byte{
		"hero.od2": ev.od2Left, "hero.od2.strigoi.json": ev.sidecarLeft, "hero.od2.world.json": ev.fileT,
		"hero-at-t.od2": ev.od2T, "hero-at-t.od2.strigoi.json": ev.sidecarT, "states.json": states,
		"dials.json": dials,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Logf("keeping the evening: %v", err)
		}
	}

	t.Logf("the evening is kept at %s", dir)
}

// keptEvening is an evening a green run kept: its hero's files are copied
// into this test's home, where a relaunch finds them. Its dials are the ones
// it was made with (dials.json, since the B4b review fixes); an evening kept
// before then names none, and was made with B4b's quickResolvedDials.
func keptEvening(t *testing.T, from string) evening {
	t.Helper()

	// Read into a slice of its own: unmarshalled into one that shares
	// quickResolvedDials' array, the evening's dials overwrote act 6i's (the
	// first kept run of act 6i found the pack never quick-resolved).
	ev := evening{savedDial: quickResolvedDials}

	if data, err := os.ReadFile(filepath.Join(from, "dials.json")); err == nil {
		var dials []dialWrite
		if err := json.Unmarshal(data, &dials); err != nil {
			t.Fatalf("the kept evening's dials: %v", err)
		}

		ev.savedDial = dials
	}
	ev.od2Left, ev.sidecarLeft, ev.fileT = mustRead(t, filepath.Join(from, "hero.od2")),
		mustRead(t, filepath.Join(from, "hero.od2.strigoi.json")), mustRead(t, filepath.Join(from, "hero.od2.world.json"))
	ev.od2T, ev.sidecarT = mustRead(t, filepath.Join(from, "hero-at-t.od2")), mustRead(t, filepath.Join(from, "hero-at-t.od2.strigoi.json"))

	var states map[string]worldSnap
	if err := json.Unmarshal(mustRead(t, filepath.Join(from, "states.json")), &states); err != nil {
		t.Fatalf("the kept evening's states: %v", err)
	}

	ev.sT, ev.sU = states["t"], states["u"]
	ev.squadModel = deployedModel(t, worldFile(t, "the kept evening", ev.fileT))

	home, err := testHome(t)
	if err != nil {
		t.Fatal(err)
	}

	saves := filepath.Join(home, "OpenDiablo2", "Saves")
	if err := os.MkdirAll(saves, 0o750); err != nil {
		t.Fatal(err)
	}

	ev.save = filepath.Join(saves, "90.od2")

	for path, data := range map[string][]byte{ev.save: ev.od2Left, ev.save + ".strigoi.json": ev.sidecarLeft, ev.save + ".world.json": ev.fileT} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Logf("acts 1-3 taken from the evening kept at %s", from)

	return ev
}

// eveningActs4to6 relaunches and resumes (acts 4-6), then takes the in-process
// path (6b) and the refusals (6c-6e).
func eveningActs4to6(t *testing.T, ev evening) {
	t.Helper()

	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	// --- 4: relaunch, load, S_R0 = S_T --------------------------------------
	// No seed: the shipped game has none to give, and the load resumes the
	// file's. The digest's world part is the world's seed, not the one the
	// script asked for (the B4a review, C1: start_seed moved to the process
	// part; with it in the world part this act was red on part world alone).
	g := s.call("strigoi_start_game", map[string]any{"save_path": ev.save, "wait_seconds": 90})

	load := sub(g, "load")
	if !flag(t, load, "resumed") || str(load, "saved_at") != str(worldFile(t, "act 4", ev.fileT), "saved_at") {
		t.Fatalf("act 4: start_game resumes the world file: %v", load)
	}

	// The file keeps its seeds as strings (int64, B1 notes section 2).
	if got, want := fmt.Sprint(int64(num(g, "seed"))), str(worldFile(t, "act 4", ev.fileT), "seed"); got != want {
		t.Fatalf("act 4: a load with no seed runs on the file's (%s); the game runs on %s", want, got)
	}

	for _, d := range ev.savedDial {
		setField(s, d.System, d.Field, d.Value)
	}

	sameWorld(t, "act 4 (S_R0 = S_T)", ev.sT, snapWorld(t, s))
	modelAnswers(t, s, "act 4", ev.squadModel)
	t.Logf("act 4 PASS: resumed (%v); S_R0 = S_T, resume digest %.12s", asList(load["steps"]), ev.sT.Resume)

	// --- 5: saved again: T's file but for its stamp -------------------------
	out := s.call("strigoi_save_game", map[string]any{})
	fileR := mustRead(t, str(out, "world_path"))

	if !bytes.Equal(withoutSavedAt(t, fileR), withoutSavedAt(t, ev.fileT)) {
		t.Fatalf("act 5: the resumed game saves T's file, byte for byte but saved_at:\n T: %s\n R: %s",
			cut(string(withoutSavedAt(t, ev.fileT))), cut(string(withoutSavedAt(t, fileR))))
	}

	if bak := mustRead(t, str(out, "world_path")+".bak"); !bytes.Equal(bak, ev.fileT) {
		t.Fatal("act 5: the file loaded is kept as the .bak (rule 5)")
	}

	t.Logf("act 5 PASS: %d bytes, T's file but saved_at and the generation", len(fileR))

	// --- 6: 120 world minutes: S_R = S_U ------------------------------------
	s.call("strigoi_step_world", map[string]any{"world_minutes": 120})
	sameWorld(t, "act 6 (S_R = S_U)", ev.sU, snapWorld(t, s))
	t.Logf("act 6 PASS: S_R = S_U, resume digest %.12s", ev.sU.Resume)

	// --- 6b: he dies; "load last save" resumes T in this process -------------
	setField(s, "meters", "health", 0.0)
	s.call("strigoi_step", map[string]any{"frames": 3})

	if !flag(t, uiState(s), "death_open") {
		t.Fatalf("act 6b: at 0 health the death screen is up: %v", uiState(s))
	}

	s.call("strigoi_key", map[string]any{"key": "enter"})
	resumedLoad := awaitGame(t, s, "act 6b")

	if !flag(t, resumedLoad, "resumed") {
		t.Fatalf("act 6b: load last save resumes the world file, not the dawn: %v", resumedLoad)
	}

	sameWorld(t, "act 6b (in process: S = S_T)", ev.sT, snapWorld(t, s))
	modelAnswers(t, s, "act 6b", ev.squadModel)

	steps := fmt.Sprint(asList(resumedLoad["steps"]))
	if !strings.Contains(steps, "uuid") || !strings.Contains(steps, "dials") {
		t.Fatalf("act 6b: the in-process load puts the uuid stream and the script's dials back: %s", steps)
	}
	t.Logf("act 6b PASS: load last save resumed T in this process (%s)", steps)

	// --- 6c: another hero's world file ---------------------------------------
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	o := s.call("strigoi_start_game", map[string]any{"hero_name": "Other", "hero_class": "amazon", "seed": 99, "wait_seconds": 90})
	otherSave := str(o, "save_path")

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	if err := os.WriteFile(otherSave+".world.json", ev.fileT, 0o600); err != nil {
		t.Fatal(err)
	}

	// On seed 7, which the file was not saved on: step 1 refuses another
	// hero's file anyway, so the seed is no reason to refuse the start, and
	// his dawn runs on it (the B4a review, C2).
	refusedToDawn(t, s, "act 6c (another hero's)", otherSave, "HERO", 7, ev.fileT, mustRead(t, otherSave+".strigoi.json"), false)

	// --- 6d: a torn save: his sidecar of another generation -----------------
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	torn := regexp.MustCompile(`"generation": "[^"]*"`).ReplaceAll(mustRead(t, ev.save+".strigoi.json"), []byte(`"generation": "a save cut off"`))
	if err := os.WriteFile(ev.save+".strigoi.json", torn, 0o600); err != nil {
		t.Fatal(err)
	}

	refusedToDawn(t, s, "act 6d (a torn save)", ev.save, "TORN", 99, fileR, torn, false)

	// --- 6e: a changed map (D5), refused after the game opened: A1's probe ---
	// His files as act 3 left them -- the .od2 and the sidecar two hours and
	// a wound later than T, the sidecar of T's generation -- and T's file
	// with its map changed. Step 1 writes the file's copy of his sidecar
	// before the map is checked; the refusal must put his own back.
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	moved := regexp.MustCompile(`"sha": "[0-9a-f]{64}"`).ReplaceAll(ev.fileT, []byte(`"sha": "`+strings.Repeat("0", 64)+`"`))
	if bytes.Equal(moved, ev.fileT) {
		t.Fatal("act 6e: the file names no map sha to change (the default game builds the authored village)")
	}

	actThree(t, ev, moved)

	fell := refusedToDawn(t, s, "act 6e (a changed map)", ev.save, "MAP", 99, moved, ev.sidecarLeft, true)
	if str(fell, "preload") != "restored" {
		t.Fatalf("act 6e: the report says his own sidecar was put back: %v", fell)
	}

	if got, want := mustNum(t, progressState(s), "xp"), xpOf(t, ev.sidecarLeft); got != want {
		t.Fatalf("act 6e: his dawn is his own -- act 3's %v experience, not the file's copy; he has %v", want, got)
	}

	// --- 6f: a session that did not resume the file: A2's probe ------------
	eveningActs6fg(t, s, ev, moved)

	// --- 6h and 8 (B4b): the re-key, and rule 4 -----------------------------
	huntedActs6h8(t, s, ev)

	// --- 6i (the B4b review fixes): a save in the death window --------------
	deathWindowAct6i(t, s, ev)
}

// actThree puts his files back as act 3 left them -- his .od2 and sidecar two
// hours and a wound later than T, the sidecar of T's generation -- beside the
// world file given.
func actThree(t *testing.T, ev evening, world []byte) {
	t.Helper()

	for path, data := range map[string][]byte{ev.save: ev.od2Left, ev.save + ".strigoi.json": ev.sidecarLeft, ev.save + ".world.json": world} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// xpOf is a sidecar document's experience.
func xpOf(t *testing.T, sidecar []byte) float64 {
	t.Helper()

	var sc struct {
		Progress struct {
			XP float64 `json:"xp"`
		} `json:"progress"`
	}

	if err := json.Unmarshal(sidecar, &sc); err != nil {
		t.Fatalf("the sidecar is not JSON: %v", err)
	}

	return sc.Progress.XP
}

// eveningActs6fg are the review's A2 and B1 probes (the B4a review, 29 Sep
// 2026): a session that did not resume the world file must not lose what it
// earned to the next load (6f), and a file that cannot be set aside must not
// loop (6g).
func eveningActs6fg(t *testing.T, s *session, ev evening, moved []byte) {
	t.Helper()

	// --- 6f: T's file, resumable beside his act-3 files, and a session that
	// never read it: a network game's (rule 9), rebuilt here as the review
	// rebuilt it, by hiding the file for one session. He earns 20.
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	world := ev.save + ".world.json"
	actThree(t, ev, ev.fileT)

	if err := os.Rename(world, world+".hidden"); err != nil {
		t.Fatal(err)
	}

	g := s.call("strigoi_start_game", map[string]any{"save_path": ev.save, "seed": 99, "wait_seconds": 90})
	if l := sub(g, "load"); flag(t, l, "found") || flag(t, l, "resumed") {
		t.Fatalf("act 6f: the session does not see the file: %v", l)
	}

	earned := mustNum(t, progressState(s), "xp") + 20
	setField(s, "progress", "grant_xp", 20.0)
	s.call("strigoi_step", map[string]any{"frames": 2})
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	after := mustRead(t, ev.save+".strigoi.json")
	if xpOf(t, after) != earned {
		t.Fatalf("act 6f: the session's experience is on disk: %v, want %v", xpOf(t, after), earned)
	}

	if err := os.Rename(world+".hidden", world); err != nil {
		t.Fatal(err)
	}

	// The next single-player load: the file does not pair with a sidecar that
	// session wrote -- TORN, set aside -- and his 20 stay. Before the fix the
	// session's kit saves carried the file's generation, the load resumed T,
	// and his sidecar went back to T's experience (the review: 125 -> 55).
	refusedToDawn(t, s, "act 6f (a session that did not resume it)", ev.save, "TORN", 99, ev.fileT, after, false)

	if got := mustNum(t, progressState(s), "xp"); got != earned {
		t.Fatalf("act 6f: his experience is the session's %v; he has %v", earned, got)
	}

	// --- 6g: held open, so it cannot be set aside: one dawn, not a loop ----
	if runtime.GOOS != "windows" {
		t.Logf("act 6g skipped: only Windows refuses to move a file held open")
		return
	}

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)
	actThree(t, ev, moved)

	held, err := os.Open(world)
	if err != nil {
		t.Fatal(err)
	}

	torn := tornDowns(t, s)
	g = s.call("strigoi_start_game", map[string]any{"save_path": ev.save, "seed": 99, "wait_seconds": 90})
	load := sub(g, "load")

	fellBack, _ := load["fell_back"].(bool)
	ignored, _ := load["ignored"].(bool)

	if str(load, "refused") != "MAP" || !fellBack || !ignored || str(load, "set_aside") != "" {
		_ = held.Close()
		t.Fatalf("act 6g: refused MAP after the game opened, not set aside, and ignored by the dawn: %v", load)
	}

	// Give a loop the time it had in the review (51 teardowns in 25 s).
	time.Sleep(3 * time.Second)

	n := tornDowns(t, s) - torn
	_ = held.Close()

	if n != 1 {
		t.Fatalf("act 6g: one teardown and one dawn; the log has %d teardowns", n)
	}

	if got := mustRead(t, ev.save+".strigoi.json"); !bytes.Equal(got, ev.sidecarLeft) {
		t.Fatal("act 6g: his own sidecar is back beside the file that could not be moved")
	}

	if got := mustRead(t, world); !bytes.Equal(got, moved) {
		t.Fatal("act 6g: the file stays whole where it was")
	}

	c := clockState(s)
	if !flag(t, s.call("strigoi_get_game_info", map[string]any{}), "in_game") || str(c, "stage") != "dawn" {
		t.Fatalf("act 6g: he is in a game at dawn: %v", c)
	}

	t.Logf("act 6g PASS: refused MAP, the file held and not moved, one teardown, his own dawn (%s)", str(load, "reason"))
}

// huntedActs6h8 are B4b's last two acts, in the same process as 6b-6g.
//
// 6h, THE RE-KEY AS THE SHIPPED GAME NEEDS IT. In a harness build the map's
// villagers draw their ids from the uuid stream reseeded from the file's
// seed, so they are the file's already and the load's re-key is the
// identity; the shipped game's ids come from crypto/rand, new every launch.
// So T's file is given its villagers' ids as another launch would have made
// them -- each native's id, everywhere in the file, made another (a suffix
// keeps the entity list's order) -- and loaded: each villager must answer to
// the file's id, the map's own gone, and the resumed game must save that
// file, byte for byte but its stamp.
//
// 8, RULE 4: "a walk you were in the middle of does not continue". He is
// walked, saved mid-stride, and loaded: he stands where he was saved, and
// every other system, entity and part is the saved moment's.
func huntedActs6h8(t *testing.T, s *session, ev evening) {
	t.Helper()

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	file := worldFile(t, "act 6h", ev.fileT)
	rekeyed := ev.fileT
	renamed := map[string]string{}

	for _, raw := range listAt(t, file, "entities") {
		e, _ := raw.(map[string]any)
		if !flag(t, e, "native") {
			continue
		}

		id := str(e, "id")
		renamed[id] = id + "-another-launch"
		rekeyed = bytes.ReplaceAll(rekeyed, []byte(`"`+id+`"`), []byte(`"`+renamed[id]+`"`))
	}

	if len(renamed) == 0 {
		t.Fatal("act 6h: the file names no villager to re-key")
	}

	actThree(t, ev, rekeyed)

	g := s.call("strigoi_start_game", map[string]any{"save_path": ev.save, "wait_seconds": 90})
	if load := sub(g, "load"); !flag(t, load, "resumed") {
		t.Fatalf("act 6h: a file whose villagers have another launch's ids resumes: %v", load)
	}

	for old, now := range renamed {
		if handleOfID(t, s, now) == "" || handleOfID(t, s, old) != "" {
			t.Fatalf("act 6h: the villager saved as %s answers to it, and the map's own id %s is gone", now, old)
		}
	}

	to := filepath.Join(t.TempDir(), "rekeyed.world.json")
	s.call("strigoi_save_game", map[string]any{"to": to})

	if !bytes.Equal(withoutSavedAt(t, mustRead(t, to)), withoutSavedAt(t, rekeyed)) {
		t.Fatalf("act 6h: the resumed game saves the re-keyed file, byte for byte but its stamp:\n file: %s\n saved: %s",
			cut(string(withoutSavedAt(t, rekeyed))), cut(string(withoutSavedAt(t, mustRead(t, to)))))
	}

	t.Logf("act 6h PASS: %d villager(s) re-keyed in place to the file's ids; the resumed game saves the file", len(renamed))

	// --- 8: rule 4, a save mid-walk ------------------------------------------
	for _, d := range ev.savedDial {
		setField(s, d.System, d.Field, d.Value)
	}

	p := s.call("strigoi_get_player", map[string]any{})
	playerID := str(p, "id")
	s.call("strigoi_move_player_to", map[string]any{"x": num(p, "x") - 4, "y": num(p, "y")})
	s.call("strigoi_step", map[string]any{"frames": 3})

	// Mid-walk: stepping toward a point that is not where he stands (a
	// click's walk is a target, and a path when it turns a corner).
	mid := s.call("strigoi_get_player", map[string]any{})
	if tg := asList(sub(mid, "state")["target"]); len(tg) != 2 || (tg[0].(float64) == num(mid, "x") && tg[1].(float64) == num(mid, "y")) {
		t.Fatalf("act 8: he is mid-walk when he saves: at %v,%v, %v", mid["x"], mid["y"], sub(mid, "state"))
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatalf("act 8: no fight while he walks: %v", combatState(s))
	}

	s.call("strigoi_save_game", map[string]any{})
	walking := snapWorld(t, s)

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	g = s.call("strigoi_start_game", map[string]any{"save_path": ev.save, "wait_seconds": 90})
	if load := sub(g, "load"); !flag(t, load, "resumed") {
		t.Fatalf("act 8: the save made mid-walk resumes: %v", load)
	}

	for _, d := range ev.savedDial {
		setField(s, d.System, d.Field, d.Value)
	}

	standing := snapWorld(t, s)

	// He stands where he was saved: no path, no waypoints, his target the
	// place he stands.
	st := sub(s.call("strigoi_get_player", map[string]any{}), "state")
	pl := s.call("strigoi_get_player", map[string]any{})

	if mustNum(t, st, "path_len") != 0 || len(asList(st["waypoints"])) != 0 {
		t.Fatalf("act 8: his walk does not continue (rule 4): %v", st)
	}

	if tg := asList(st["target"]); len(tg) != 2 || tg[0].(float64) != num(pl, "x") || tg[1].(float64) != num(pl, "y") {
		t.Fatalf("act 8: he stands at his own place: target %v, at %v,%v", st["target"], pl["x"], pl["y"])
	}

	// Every other system, entity and part is the saved moment's: his walk,
	// and only it, is left out of the comparison.
	sameWorldExcept(t, "act 8 (rule 4: all but his walk)", walking, standing, hisWalk(playerID))

	t.Logf("act 8 PASS: saved mid-walk, he resumes standing where he was, and the world is the saved one but his walk")
}

// deathWindowAct6i is THE REVIEW'S REAL-GAME B1 PROBE AS AN ACT (the B4b
// review fixes, BUG-75 and BUG-76; the probe was TestZZRevMidDeath, on
// evening-2), made a save mid-death by BUG-87. The review resumed T with the
// fight after it quick-resolved, stepped to the frame the quick resolve fired
// and saved there, while the slain still played their deaths: sixty frames
// later two opportunists lay dead in the saved game and were still falling in
// the resumed one. And a member quick-resolved while he walked in walked on
// through his death, his corpse coming to rest away from where the resolver
// recorded his fall. The B4b review fixes stopped the walk (BUG-75) and
// refused the save until the last lay (BUG-76); BUG-87 saves the deaths at
// their frames instead.
//
// Here, on the same T (the evening's file beside his act-3 files, as act 4
// loads it) with B4b's dials (quickResolvedDials): the quick resolve fires;
// every member it slew lies where his fall was recorded (BUG-75); the save is
// made at once, on the first try, with the slain still playing their deaths
// -- each in the file at its frame; and that moment T' resumes exactly (S_R0
// = S_T', the deaths at their frames), is the same world 20 frames on (S_R =
// S_M', the deaths still playing) and 120 frames on (S_R = S_U', every slain
// a corpse), and each resumed corpse lies where the saved game's lies.
func deathWindowAct6i(t *testing.T, s *session, ev evening) {
	t.Helper()

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)
	actThree(t, ev, ev.fileT)

	g := s.call("strigoi_start_game", map[string]any{"save_path": ev.save, "wait_seconds": 90})
	if load := sub(g, "load"); !flag(t, load, "resumed") {
		t.Fatalf("act 6i: T resumes: %v", load)
	}

	for _, d := range quickResolvedDials {
		setField(s, d.System, d.Field, d.Value)
	}

	t.Logf("act 6i: the combat dials after the resume: %v", sub(combatState(s), "dials"))

	fallen := map[string]bool{}
	for _, raw := range asList(corpsesState(s)["bodies"]) {
		b, _ := raw.(map[string]any)
		fallen[str(b, "id")] = true
	}

	q0 := mustNum(t, combatState(s), "quick_resolved")
	frames := 0

	for frames < 6000 && mustNum(t, combatState(s), "quick_resolved") == q0 {
		s.call("strigoi_step", map[string]any{"frames": 10})
		frames += 10
	}

	if mustNum(t, combatState(s), "quick_resolved") == q0 {
		t.Fatalf("act 6i: the pack walks in and its fight is quick-resolved within %d frames (dials %v): %v",
			frames, sub(combatState(s), "dials"), combatState(s))
	}

	// The slain: every body the quick resolve laid, and where it lay.
	slain := map[string][2]float64{}
	for _, raw := range asList(corpsesState(s)["bodies"]) {
		b, _ := raw.(map[string]any)
		if id := str(b, "id"); !fallen[id] {
			slain[id] = [2]float64{mustNum(t, b, "x"), mustNum(t, b, "y")}
		}
	}

	if len(slain) == 0 {
		t.Fatalf("act 6i: the quick resolve slew someone: %v", corpsesState(s))
	}

	lieWhereTheyFell(t, s, "act 6i (the first look after the quick resolve)", slain)

	// BUG-87: the save is made at once, with the slain still falling.
	if e := s.callErr("strigoi_save_game", map[string]any{}); e != "" {
		t.Fatalf("RED act 6i: the save in the death window is made on the first try (BUG-87): refused %q", e)
	}

	sT := snapWorld(t, s)

	dying := map[string]string{}

	for id := range slain {
		var e map[string]any
		_ = json.Unmarshal(sT.Entities[id], &e)

		if st := sub(e, "state"); str(st, "held") != "" {
			if _, ok := st["held_frame"]; !ok {
				t.Fatalf("act 6i: at T' the slain %s holds %s and reports no frame: %v", id, str(st, "held"), st)
			}

			dying[id] = fmt.Sprintf("%s at frame %v + %v s", str(st, "held"), st["held_frame"], st["held_elapsed"])
		}
	}

	if len(dying) == 0 {
		t.Fatalf("act 6i: at T' at least one slain is still playing his death, so the save is made mid-death (%d slain, none holding an action)", len(slain))
	}

	lieWhereTheyFell(t, s, "act 6i (at T')", slain)

	s.call("strigoi_step", map[string]any{"frames": 20})
	sM := snapWorld(t, s)

	s.call("strigoi_step", map[string]any{"frames": 100})
	sU := snapWorld(t, s)

	corpses := map[string][2]float64{}

	for id := range slain {
		var e map[string]any
		_ = json.Unmarshal(sU.Entities[id], &e)

		if st := sub(e, "state"); str(st, "held") != "" || !flag(t, st, "corpse") {
			t.Fatalf("act 6i: 120 frames after T' the slain %s lies dead, holding no action: %v", id, st)
		}

		corpses[id] = [2]float64{num(e, "x"), num(e, "y")}
	}

	lieWhereTheyFell(t, s, "act 6i (T'+120, the saved game)", slain)

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	g = s.call("strigoi_start_game", map[string]any{"save_path": ev.save, "wait_seconds": 90})
	if load := sub(g, "load"); !flag(t, load, "resumed") {
		t.Fatalf("act 6i: T' resumes: %v", load)
	}

	for _, d := range quickResolvedDials {
		setField(s, d.System, d.Field, d.Value)
	}

	sameWorld(t, "act 6i (S_R0 = S_T', saved mid-death: each death at its frame)", sT, snapWorld(t, s))

	s.call("strigoi_step", map[string]any{"frames": 20})
	sameWorld(t, "act 6i (S_R = S_M', twenty frames on, the deaths still playing)", sM, snapWorld(t, s))

	s.call("strigoi_step", map[string]any{"frames": 100})
	sameWorld(t, "act 6i (S_R = S_U', 120 frames on, every slain a corpse)", sU, snapWorld(t, s))
	lieWhereTheyFell(t, s, "act 6i (resumed: where the saved game's lie)", corpses)

	t.Logf("act 6i PASS: the quick resolve after %d frames slew %d; the save was made at once, on the first try, with %d still falling (%v); "+
		"S_R0 = S_T', S_R = S_M' twenty frames on and S_R = S_U' 120 frames on; each lies where he fell, in both games", frames, len(slain), len(dying), dying)
}

// lieWhereTheyFell requires each slain entity to stand exactly where his fall
// was recorded (the corpses block's x, y; Combat.fallCorpse): BUG-75.
func lieWhereTheyFell(t *testing.T, s *session, act string, slain map[string][2]float64) {
	t.Helper()

	at := map[string][2]float64{}

	for _, raw := range asList(s.call("strigoi_get_entities", map[string]any{"limit": 500})["items"]) {
		e, _ := raw.(map[string]any)
		if _, ok := slain[str(e, "id")]; ok {
			at[str(e, "id")] = [2]float64{num(e, "x"), num(e, "y")}
		}
	}

	for id, fell := range slain {
		got, ok := at[id]
		if !ok || got != fell {
			t.Fatalf("%s: the slain %s lies where his fall was recorded, %v; he is at %v (on the map: %v)", act, id, fell, got, ok)
		}
	}
}

// hisWalk is rule 4's exemption: the player's own walk, which a load does not
// continue -- the point he was stepping to, the waypoints ahead, the path's
// length, the walk's animation (the mode, and the PNG hero's sheet it draws:
// "run" saved, "idle" resumed; measured, the only field beyond the first four
// that differed), and, since the entities report it (the B4b review fixes,
// BUG-82), the velocity he was stepping with.
func hisWalk(playerID string) func(where string) bool {
	return func(where string) bool {
		switch where {
		case "entity " + playerID + ".target", "entity " + playerID + ".waypoints",
			"entity " + playerID + ".path_len", "entity " + playerID + ".animation_mode",
			"entity " + playerID + ".body_sheet", "entity " + playerID + ".velocity":
			return true
		}

		return false
	}
}

// sameWorldExcept is sameWorld with named differences allowed: "system
// <name>.<field>" or "entity <id>.<key>". Any other difference fails, named;
// so does an allowed one that is not there (the exemption must be needed).
//
// AND A HASH THAT DIFFERS WITH NOTHING TO EXPLAIN IT FAILS (the B4b review
// fixes, BUG-81; the review's C4). The systems and entities are compared field
// by field, but their hashes are the digest's: a system whose hash differed
// while every field of its report agreed -- a field the digest hashes and
// the report does not show -- passed silently, where sameWorld would have
// failed. So each system whose hash differs must show a differing field, and
// the systems and entities parts must each show a differing system or
// entity line.
func sameWorldExcept(t testing.TB, act string, a, b worldSnap, allowed func(where string) bool) {
	t.Helper()

	var why []string

	// The parts no entity or system line covers: the world (the seed, the
	// map) and every stream's position. The entities and systems parts are
	// compared line by line and field by field below.
	for _, p := range []string{"world", "rng"} {
		if a.Parts[p] != b.Parts[p] {
			why = append(why, "part "+p)
		}
	}

	systemsDiffer := 0

	for _, n := range keysOfSnap(a.Systems, b.Systems) {
		var ma, mb map[string]any
		_ = json.Unmarshal(a.States[n], &ma)
		_ = json.Unmarshal(b.States[n], &mb)

		if a.Systems[n] == b.Systems[n] {
			continue
		}

		systemsDiffer++
		explained := false

		for _, k := range keysOfAny(ma, mb) {
			ja, _ := json.Marshal(ma[k])
			jb, _ := json.Marshal(mb[k])

			if string(ja) == string(jb) {
				continue
			}

			explained = true

			if !allowed("system " + n + "." + k) {
				why = append(why, fmt.Sprintf("  %s.%s\n      was: %s\n      now: %s", n, k, cut(string(ja)), cut(string(jb))))
			}
		}

		if !explained {
			why = append(why, fmt.Sprintf("  system %s: its hash differs (%s, %s) and no field of its report does -- "+
				"the difference is in something the comparison cannot see", n, a.Systems[n], b.Systems[n]))
		}
	}

	if a.Parts["systems"] != b.Parts["systems"] && systemsDiffer == 0 {
		why = append(why, "part systems: it differs and no system's hash does")
	}

	exempted, entitiesDiffer := 0, 0

	for _, id := range keysOfRaw(a.Entities, b.Entities) {
		var ea, eb map[string]any
		_ = json.Unmarshal(a.Entities[id], &ea)
		_ = json.Unmarshal(b.Entities[id], &eb)

		sa, sb := sub(ea, "state"), sub(eb, "state")
		delete(ea, "state")
		delete(eb, "state")

		for _, k := range keysOfAny(ea, eb) {
			ja, _ := json.Marshal(ea[k])
			jb, _ := json.Marshal(eb[k])

			if string(ja) != string(jb) {
				entitiesDiffer++

				why = append(why, fmt.Sprintf("  entity %s.%s\n      was: %s\n      now: %s", id, k, cut(string(ja)), cut(string(jb))))
			}
		}

		for _, k := range keysOfAny(sa, sb) {
			ja, _ := json.Marshal(sa[k])
			jb, _ := json.Marshal(sb[k])

			if string(ja) == string(jb) {
				continue
			}

			entitiesDiffer++

			if allowed("entity " + id + "." + k) {
				exempted++

				continue
			}

			why = append(why, fmt.Sprintf("  entity %s.%s\n      was: %s\n      now: %s", id, k, cut(string(ja)), cut(string(jb))))
		}
	}

	if a.Parts["entities"] != b.Parts["entities"] && entitiesDiffer == 0 {
		why = append(why, "part entities: it differs and no entity line does")
	}

	if len(why) > 0 {
		t.Fatalf("%s: the resumed world differs from the saved one beyond what is exempt:\n%s", act, strings.Join(why, "\n"))
	}

	if exempted == 0 {
		t.Fatalf("%s: nothing the exemption names differed -- the act compared nothing it was written for", act)
	}

	t.Logf("%s: every system and entity equal; %d exempt field(s) differ", act, exempted)
}

// THE CONTROL FOR sameWorldExcept (the B4b review fixes, BUG-81): no game.
// Rule 4's comparison allows named fields to differ, and compared the systems
// and entities field by field -- so a system whose hash differed while its
// report agreed passed, where sameWorld would have failed (the review's C4).
// Three pairs: a walk that differs only where the exemption allows (passes:
// the control of the control), the same with a system's hash differing and
// nothing in its report to say why (fails, naming the system), and with the
// entities part differing and no entity line (fails).
func TestSameWorldExceptFailsAnUnexplainedHash(t *testing.T) {
	saved := worldSnap{
		Resume:  "r-saved",
		Parts:   map[string]string{"world": "w", "rng": "g", "systems": "s1", "entities": "e1"},
		Systems: map[string]string{"combat": "c1"},
		States:  map[string]json.RawMessage{"combat": json.RawMessage(`{"round":3}`)},
		Entities: map[string]json.RawMessage{
			"0him": json.RawMessage(`{"id":"0him","kind":"player","state":{"target":[3,4]}}`),
		},
	}

	walked := saved
	walked.Resume, walked.Parts = "r-walked", map[string]string{"world": "w", "rng": "g", "systems": "s1", "entities": "e2"}
	walked.Entities = map[string]json.RawMessage{
		"0him": json.RawMessage(`{"id":"0him","kind":"player","state":{"target":[5,4]}}`),
	}

	allowed := func(where string) bool { return where == "entity 0him.target" }

	f := &sameWorldTB{}
	sameWorldExcept(f, "the control", saved, walked, allowed)

	if f.failed {
		t.Fatalf("an exempt difference alone must pass: %s", f.msg)
	}

	hidden := walked
	hidden.Parts = map[string]string{"world": "w", "rng": "g", "systems": "s2", "entities": "e2"}
	hidden.Systems = map[string]string{"combat": "c2"}

	f = &sameWorldTB{}
	sameWorldExcept(f, "a hidden system difference", saved, hidden, allowed)

	if !f.failed || !contains(f.msg, "combat") {
		t.Fatalf("a system whose hash differs with no field to explain it must fail, naming it: failed %v, %q", f.failed, f.msg)
	}

	part := walked
	part.Entities = saved.Entities

	f = &sameWorldTB{}
	sameWorldExcept(f, "a hidden entity difference", saved, part, allowed)

	if !f.failed || !contains(f.msg, "part entities") {
		t.Fatalf("an entities part that differs with no entity line to explain it must fail: failed %v, %q", f.failed, f.msg)
	}
}

// sameWorldTB is flag_test's fakeTB with Logf: sameWorldExcept logs on a pass.
type sameWorldTB struct {
	testing.TB

	failed bool
	msg    string
}

func (f *sameWorldTB) Helper() {}

func (f *sameWorldTB) Fatalf(format string, args ...any) {
	if !f.failed {
		f.failed, f.msg = true, fmt.Sprintf(format, args...)
	}
}

func (f *sameWorldTB) Logf(string, ...any) {}

// tornDowns counts the teardowns of refused loads in the game's log.
func tornDowns(t *testing.T, s *session) int {
	t.Helper()

	data, err := os.ReadFile(s.LogPath)
	if err != nil {
		t.Fatalf("reading the game's log: %v", err)
	}

	return strings.Count(string(data), "LOAD torn down")
}

// awaitGame waits -- in wall time, stepping nothing, so the resumed world is
// not moved by the wait -- for a new game to be in play with its controls
// bound, and returns its load report.
func awaitGame(t *testing.T, s *session, act string) map[string]any {
	t.Helper()

	last := ""

	for i := 0; i < 300; i++ {
		time.Sleep(100 * time.Millisecond)

		info := s.call("strigoi_get_game_info", map[string]any{})
		if !flag(t, info, "in_game") || flag(t, info, "loading") {
			continue
		}

		if last = s.callErr("strigoi_get_system_state", map[string]any{"system": "ui"}); last != "" {
			continue
		}

		if flag(t, uiState(s), "death_open") {
			continue
		}

		// The load report as it stands NOW, and only once it is settled --
		// resumed, refused, or no file found (M4.6 B4b: the report read at
		// the top of this loop could be a frame old, taken between CreateGame's
		// steps and the first frame's restore, and a rebuild of the hunted
		// night's entities made that window wide enough to be hit under a
		// parallel run: "resumed false, steps [clock natives entities
		// validated]").
		load := sub(s.call("strigoi_get_game_info", map[string]any{}), "load")
		if flag(t, load, "found") && !flag(t, load, "resumed") && str(load, "refused") == "" {
			continue
		}

		return load
	}

	t.Fatalf("%s: no game came up in 30 s (last: %s)", act, last)

	return nil
}

// awaitMenu waits in wall time for the game to be gone.
func awaitMenu(t *testing.T, s *session) {
	t.Helper()

	for i := 0; i < 100; i++ {
		if !flag(t, s.call("strigoi_get_game_info", map[string]any{}), "in_game") {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatal("the game did not leave for the menu")
}

// refusedToDawn starts the hero at save on seed and requires the load refused
// with code, the file set aside whole (rule 7), his sidecar byte for byte the
// one given -- his own, never the world file's copy (the B4a review, A1) --
// and him at dawn in a living game on that seed. afterOpen is a refusal that
// tears an opened game down (fell_back). It returns the load's report.
func refusedToDawn(t *testing.T, s *session, act, save, code string, seed int, file, sidecar []byte, afterOpen bool) map[string]any {
	t.Helper()

	g := s.call("strigoi_start_game", map[string]any{"save_path": save, "seed": seed, "wait_seconds": 90})
	load := sub(g, "load")

	if got := num(g, "seed"); got != float64(seed) {
		t.Fatalf("%s: his dawn runs on seed %d; the game runs on %v", act, seed, got)
	}

	if got := mustRead(t, save+".strigoi.json"); !bytes.Equal(got, sidecar) {
		t.Fatalf("%s: his sidecar is his own, byte for byte, after the refusal:\n want %s\n  got %s", act, cut(string(sidecar)), cut(string(got)))
	}

	// fell_back is written only when true (omitempty).
	fellBack, _ := load["fell_back"].(bool)
	if str(load, "refused") != code || flag(t, load, "resumed") || fellBack != afterOpen {
		t.Fatalf("%s: the load is refused %s (fell back %v): %v", act, code, afterOpen, load)
	}

	aside := str(load, "set_aside")
	if got, err := readSaved(aside); err != nil || !bytes.Equal(got, file) {
		t.Fatalf("%s: the file is set aside whole at %q (%v)", act, aside, err)
	}

	if _, err := os.Stat(save + ".world.json"); !os.IsNotExist(err) {
		t.Fatalf("%s: the world file is out of the way (%v)", act, err)
	}

	c := clockState(s)
	if str(c, "stage") != "dawn" || mustNum(t, c, "world_minutes") > 1 || mustNum(t, sub(s.call("strigoi_get_player", map[string]any{}), "state"), "health") <= 0 {
		t.Fatalf("%s: he begins at dawn, alive: %v", act, c)
	}

	t.Logf("%s PASS: refused %s (%s), set aside as %s; his own sidecar beside it; he begins at dawn", act, code, str(load, "reason"), filepath.Base(aside))

	return load
}

// saveVerbActs is act 7, burst B3's: the save verb and its refusals, in a
// process of its own (its hero, Saver, is not the evening's).
//
// Act 7, as the plan has it, and what B3 adds to it:
//
//	7a  a save outside a fight writes N.od2.world.json with every block, and
//	    the .od2 and the sidecar with it -- the sidecar byte for byte the
//	    document the world file embeds -- and SAVING CHANGES NOTHING: the
//	    state digest before and after is one digest (rule 10's autosave runs
//	    inside a frame; a save that drew a number would move the night).
//	7b  mid-fight: FIGHTING, and every file untouched (hashed before and
//	    after), a save to another path included.
//	7c  a busy world -- a lit torch, a pack on the map, a chase, the placed
//	    dead, a slain dog with his wounds, a fight's counters -- saves with
//	    every list block non-empty and every live pack member in the entity
//	    list where his pack says he stands; the previous save is the .bak;
//	    and the digest is still unmoved.
//	7d  to writes only the world file, where it is told, the same file but
//	    for saved_at; omit leaves a block out, and needs to.
//	7e  dead: DEAD, every file untouched -- and the death put his sidecar
//	    back to the LAST SAVE's bytes, not to the moment he entered, because
//	    the save re-takes the death screen's copy.
//
// The B3 review (29 Sep 2026) adds, in the same acts:
//
//	7a  the file names its hero -- the .od2's heroName and heroType -- and
//	    carries his facing and stamina, each what the player reports; he is
//	    walked a step first, so he faces somewhere other than where he
//	    started. The sidecar file is of the world file's generation.
//	7c  before the save, the sidecar a fight's end rewrote still carries
//	    7a's generation; at least one pack member is checked in the entity
//	    list; the .od2's .bak is 7a's .od2, byte for byte.
//	7d  to is fenced: relative, inside the repository, or one of his own
//	    files is BAD_ARGUMENT and writes nothing; world_path is absolute.
//	7f  (run before 7e, which kills him) the slain dog taken off the map by
//	    the harness takes his body with him (body_dropped), and the next save
//	    is made -- it used to fail INTERNAL on a body with no entity.
func saveVerbActs(t *testing.T) {
	t.Helper()

	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Saver", "hero_class": "amazon", "seed": 99, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	save := str(game, "save_path")
	world := save + ".world.json"
	sidecar := save + ".strigoi.json"

	if save == "" {
		t.Fatalf("start_game reported no save path: %v", game)
	}

	// --- 7a: a quiet save writes all three, and moves nothing --------------
	// A step first, so the facing the file must carry is not the one he was
	// made with (the B3 review, B6): one neighbour, and back if that one left
	// him facing 0.
	faceSomewhere(t, s)

	out := saveUnmoved(t, s, "7a", map[string]any{})

	if got := str(out, "world_path"); got != world {
		t.Fatalf("7a: world_path %q, want %q beside his save", got, world)
	}

	for _, p := range []string{world, save, sidecar} {
		if !hasString(stringsOf(out["written"]), p) {
			t.Fatalf("7a: the save writes %s; written %v", p, out["written"])
		}
	}

	first := mustRead(t, world)
	file := worldFile(t, "7a", first)

	// One moment in both files (B7): the sidecar file is of this save's
	// generation, before it is compared whole.
	gen7a := str(file, "saved_at")
	if got := generationOf(t, mustRead(t, sidecar)); got != gen7a || gen7a == "" {
		t.Fatalf("7a: the sidecar is of this save's generation %q; it carries %q", gen7a, got)
	}

	sameSidecar(t, "7a", file, mustRead(t, sidecar))

	if got := str(file, "seed"); got != "99" {
		t.Fatalf("7a: the seed is written as the exact string \"99\", got %v", file["seed"])
	}

	p := s.call("strigoi_get_player", map[string]any{})
	hero := sub(file, "hero")

	if num(hero, "x") != num(p, "x") || num(hero, "y") != num(p, "y") {
		t.Fatalf("7a: the hero is saved where he stands: file %v,%v, player %v,%v", hero["x"], hero["y"], p["x"], p["y"])
	}

	if num(hero, "health") != num(sub(p, "state"), "health") || num(hero, "health") <= 0 {
		t.Fatalf("7a: the hero's health is saved: file %v, player %v", hero["health"], sub(p, "state")["health"])
	}

	// Who he is, which way he faces, how much wind he has (A1, B6).
	firstOD2 := mustRead(t, save)
	sameHero(t, "7a", hero, firstOD2)

	if num(hero, "facing") != num(sub(p, "state"), "direction") || num(hero, "facing") == 0 {
		t.Fatalf("7a: his facing is saved as he faces, and he was walked to face somewhere: file %v, player %v",
			hero["facing"], sub(p, "state")["direction"])
	}

	if num(hero, "stamina") != num(sub(p, "state"), "stamina") {
		t.Fatalf("7a: his stamina is saved: file %v, player %v", hero["stamina"], sub(p, "state")["stamina"])
	}

	t.Logf("7a PASS: %d bytes, %d blocks, the sidecar embedded byte for byte, digest unmoved; %s the %s facing %v with %v stamina, generation %s",
		len(first), len(file), str(hero, "name"), str(hero, "class"), hero["facing"], hero["stamina"], gen7a)

	// --- 7b: mid-fight, FIGHTING, and nothing is touched --------------------
	pl := s.call("strigoi_get_player", map[string]any{})
	spot := clearNeighbour(t, s, num(pl, "x"), num(pl, "y"))
	dog := spawnNPC(t, s, "fallen1", spot[0], spot[1])
	dogID := entityID(t, s, dog)

	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": str(pl, "handle")})
	fightNow(t, s)

	files := []string{world, world + ".bak", save, save + ".bak", sidecar}
	before := hashFiles(t, files)
	elsewhere := filepath.Join(t.TempDir(), "mid-fight.world.json")

	refusedWith(t, s, "7b", "FIGHTING", map[string]any{})
	refusedWith(t, s, "7b (to)", "FIGHTING", map[string]any{"to": elsewhere, "omit": []any{"corpses"}})

	if after := hashFiles(t, files); !equalHashes(before, after) {
		t.Fatalf("7b: a refused save touched a file:\n before %v\n after  %v", before, after)
	}

	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Fatalf("7b: a refused save wrote %s (%v)", elsewhere, err)
	}

	t.Logf("7b PASS: mid-fight FIGHTING, %d files untouched, nothing written elsewhere", len(files))

	// --- 7c: a busy world saves whole ---------------------------------------
	// The dog dies (forced crits, as TestCombatResolver finishes its own), so
	// the file carries a fight's counters and a body with its wounds.
	for i := 0; i < 8 && flag(t, combatState(s), "fighting"); i++ {
		setField(s, "combat", "forced_band", "crit")
		stepToNewRound(t, s)
	}

	setField(s, "combat", "forced_band", "")

	if got := str(combatState(s), "ended_reason"); got != "enemies_dead" {
		t.Fatalf("7c: the dog must die for the file to carry a fight's end; ended_reason %q", got)
	}

	s.call("strigoi_step", map[string]any{"frames": 2})

	// His torch, lit.
	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if !flag(t, lightState(s), "carried_lit") {
		t.Fatalf("7c: L lights his torch: %v", lightState(s))
	}

	// A pack on the map: the table forced, and the save taken the moment one
	// arrives -- before it can walk in and make a fight.
	setField(s, "spawns", "chance", 100)

	for i := 0; i < 60 && num(spawnsState(s), "groups") == 0; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1})
	}

	setField(s, "spawns", "chance", 0)

	if num(spawnsState(s), "groups") == 0 {
		t.Fatalf("7c: a certain table never placed a pack: %v", spawnsState(s))
	}

	// A chase, of one entity after another: a second fallen, far off and
	// watching nobody, sent after a villager.
	chaser := spawnNPC(t, s, "fallen1", num(pl, "x")+9, num(pl, "y")+9)
	villager := nativeHandle(t, s)

	s.call("strigoi_pursue", map[string]any{"hunter": chaser, "quarry": villager})
	s.call("strigoi_step", map[string]any{"frames": 1})

	if flag(t, combatState(s), "fighting") {
		t.Fatalf("7c: the arranged world must not be in a fight when it saves: %v", combatState(s))
	}

	// The fight's end rewrote his sidecar (saveKit); it carries the last
	// world save's generation forward, so the two files still agree (B7).
	if got := generationOf(t, mustRead(t, sidecar)); got != gen7a {
		t.Fatalf("7c: a kit save between world saves carries 7a's generation %q; the sidecar carries %q", gen7a, got)
	}

	out = saveUnmoved(t, s, "7c", map[string]any{})

	second := mustRead(t, world)
	file = worldFile(t, "7c", second)

	// The file, kept beside the run's logs for a person to read.
	if s.RunBase != "" {
		keep := filepath.Join(s.RunBase, "pt", safeName(t.Name()), "save-resume-7c.world.json")
		if err := os.WriteFile(keep, second, 0o600); err == nil {
			t.Logf("7c: the world file is kept at %s", keep)
		}
	}

	if bak := mustRead(t, world+".bak"); !bytes.Equal(bak, first) {
		t.Fatalf("7c: the previous save is kept as .bak, byte for byte (rule 5)")
	}

	if bak := mustRead(t, save+".bak"); !bytes.Equal(bak, firstOD2) {
		t.Fatalf("7c: the .od2 keeps its previous generation too -- 7a's .od2, byte for byte (rule 5)")
	}

	sameSidecar(t, "7c", file, mustRead(t, sidecar))
	sameHero(t, "7c", sub(file, "hero"), mustRead(t, save))

	if got, want := generationOf(t, mustRead(t, sidecar)), str(file, "saved_at"); got != want {
		t.Fatalf("7c: the sidecar is of 7c's generation %q; it carries %q", want, got)
	}

	savedSidecar := mustRead(t, sidecar)

	for _, path := range []string{"light.sources", "squads.squads", "spawns.groups", "notice.watches",
		"pursuit.chases", "corpses.bodies", "bodies", "entities", "scene.field_dead"} {
		if n := len(listAt(t, file, path)); n == 0 {
			t.Fatalf("7c: %s is empty in a world arranged to fill it", path)
		}
	}

	if num(sub(file, "combat"), "started") < 1 || num(sub(file, "clock"), "elapsed") <= 0 {
		t.Fatalf("7c: a fight's counters and the clock are saved: combat %v, clock %v", file["combat"], file["clock"])
	}

	rng := sub(file, "rng")
	if num(sub(rng, "world"), "draws") <= 0 || sub(rng, "uuid") == nil || num(sub(rng, "uuid"), "bytes") <= 0 {
		t.Fatalf("7c: the world and uuid streams are saved where they stand: %v", rng)
	}

	for _, stream := range []string{"spawns", "combat", "rising"} {
		if fmt.Sprint(sub(rng, stream)) != fmt.Sprint(sub(sub(file, stream), "rng")) {
			t.Fatalf("7c: rng.%s %v is the %s block's own stream %v", stream, sub(rng, stream), stream, sub(sub(file, stream), "rng"))
		}
	}

	checkMap(t, sub(file, "map"))

	entities := map[string]map[string]any{}
	for _, raw := range listAt(t, file, "entities") {
		e, _ := raw.(map[string]any)
		entities[str(e, "id")] = e
	}

	if dogE := entities[dogID]; dogE == nil || str(dogE, "kind") != "npc" || str(dogE, "monstat") != "fallen1" {
		t.Fatalf("7c: the slain dog is an npc entity of monstat fallen1: %v", dogE)
	}

	members := 0

	for _, raw := range listAt(t, file, "spawns.groups") {
		g, _ := raw.(map[string]any)

		for _, rm := range asList(g["members"]) {
			// gone is written only when true (omitempty).
			m, _ := rm.(map[string]any)
			if gone, _ := m["gone"].(bool); gone {
				continue
			}

			e := entities[str(m, "id")]
			if e == nil || num(e, "x") != num(m, "x") || num(e, "y") != num(m, "y") {
				t.Fatalf("7c: %s's member %s stands at %v,%v and the entity list has %v", str(g, "id"), str(m, "id"), m["x"], m["y"], e)
			}

			members++
		}
	}

	natives := 0

	for _, e := range entities {
		if flag(t, e, "native") {
			natives++

			if len(asList(e["born"])) != 2 {
				t.Fatalf("7c: a native entity carries where the map put it: %v", e)
			}
		}
	}

	if natives == 0 {
		t.Fatalf("7c: the map's villagers are marked native")
	}

	// The loop above checks every member on the map against the entity list;
	// with none, it checked nothing (the review's C item).
	if members == 0 {
		t.Fatalf("7c: no pack member was on the map to check against the entity list")
	}

	t.Logf("7c PASS: %d entities (%d native), %d pack member(s) where their pack says, every list block non-empty, .bak = 7a's file",
		len(entities), natives, members)

	// --- 7d: to and omit -------------------------------------------------
	ours := hashFiles(t, files)
	to := filepath.Join(t.TempDir(), "copy.world.json")

	out = s.call("strigoi_save_game", map[string]any{"to": to})
	if str(out, "world_path") != to || !filepath.IsAbs(str(out, "world_path")) || len(stringsOf(out["written"])) != 1 {
		t.Fatalf("7d: to writes the world file there and nothing else, and says where, absolute: %v", out)
	}

	// The fence (B5): relative (the game's working directory is the
	// repository), inside the repository, or one of his own files.
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	inRepo := filepath.Join(repo, "docs", "b3-to.world.json")

	for _, bad := range []map[string]any{
		{"to": "b3-relative.world.json"},
		{"to": inRepo},
		{"to": world},
		{"to": world, "omit": []any{"corpses"}},
		{"to": sidecar},
	} {
		if e := s.callErr("strigoi_save_game", bad); !strings.Contains(e, "BAD_ARGUMENT") {
			t.Fatalf("7d: %v must be BAD_ARGUMENT, got %q", bad, e)
		}
	}

	for _, p := range []string{inRepo, filepath.Join(repo, "b3-relative.world.json")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("7d: a refused to wrote %s (%v)", p, err)
		}
	}

	if !equalHashes(ours, hashFiles(t, files)) {
		t.Fatalf("7d: a save to another path touched his save")
	}

	if !bytes.Equal(withoutSavedAt(t, mustRead(t, to)), withoutSavedAt(t, mustRead(t, world))) {
		t.Fatalf("7d: the same moment written elsewhere is the same file, but for saved_at")
	}

	omitted := filepath.Join(t.TempDir(), "no-corpses.world.json")
	s.call("strigoi_save_game", map[string]any{"to": omitted, "omit": []any{"corpses", "scene"}})

	var top map[string]json.RawMessage
	if err := json.Unmarshal(mustRead(t, omitted), &top); err != nil {
		t.Fatal(err)
	}

	if _, ok := top["corpses"]; ok || len(top) != len(worldBlocks)-2 {
		t.Fatalf("7d: omit leaves out corpses and scene and nothing else: %d blocks", len(top))
	}

	for _, bad := range []map[string]any{{"omit": []any{"corpses"}}, {"to": omitted, "omit": []any{"weather"}}} {
		if e := s.callErr("strigoi_save_game", bad); !strings.Contains(e, "BAD_ARGUMENT") {
			t.Fatalf("7d: %v must be BAD_ARGUMENT, got %q", bad, e)
		}
	}

	if !equalHashes(ours, hashFiles(t, files)) {
		t.Fatalf("7d: omit and its refusals touched his save")
	}

	t.Logf("7d PASS: to writes only there (the same file but saved_at); omit drops exactly its blocks; omit without to is refused; to is fenced")

	// --- 7f: a monster taken off the map takes his body with him ------------
	// (Run before 7e, which kills him.) The slain dog's body is in the file
	// at 0; the harness's removal used to leave it, with no entity, and every
	// save after it failed INTERNAL "a body with no entity in the file".
	removed := s.call("strigoi_remove_entity", map[string]any{"handle": dog})
	if removed["removed"] != true || removed["body_dropped"] != true {
		t.Fatalf("7f: removing the slain dog drops his body: %v", removed)
	}

	saveUnmoved(t, s, "7f", map[string]any{})

	after := worldFile(t, "7f", mustRead(t, world))

	for _, raw := range listAt(t, after, "bodies") {
		if b, _ := raw.(map[string]any); str(b, "id") == dogID {
			t.Fatalf("7f: the removed dog's body is still saved: %v", b)
		}
	}

	for _, raw := range listAt(t, after, "entities") {
		if e, _ := raw.(map[string]any); str(e, "id") == dogID {
			t.Fatalf("7f: the removed dog is still an entity: %v", e)
		}
	}

	// 7f's save is the last save now: the death below puts ITS sidecar back.
	savedSidecar = mustRead(t, sidecar)

	t.Logf("7f PASS: the slain dog removed with his body, and the next save made")

	// --- 7e: dead, DEAD, and the death put the LAST SAVE back ---------------
	setField(s, "meters", "health", 0.0)
	s.call("strigoi_step", map[string]any{"frames": 3})

	if !flag(t, uiState(s), "death_open") {
		t.Fatalf("7e: at 0 health the death screen is up: %v", uiState(s))
	}

	if now := mustRead(t, sidecar); !bytes.Equal(now, savedSidecar) {
		t.Fatalf("7e: the death puts his sidecar back to the LAST SAVE's bytes (the save re-took the death screen's copy):\n saved: %s\n now:   %s",
			savedSidecar, now)
	}

	dead := hashFiles(t, files)

	refusedWith(t, s, "7e", "DEAD", map[string]any{})

	if !equalHashes(dead, hashFiles(t, files)) {
		t.Fatalf("7e: a refused save touched a file")
	}

	t.Logf("7e PASS: DEAD, every file untouched; the death restored the last save's sidecar")

	// The evening's processes follow: this one's work is done.
	s.stop()
}

// worldBlocks is every top-level block of the world file, in order
// (d2save.Blocks): the script keeps its own copy, so a block dropped from the
// code is a red script, not a quietly shorter list.
var worldBlocks = []string{
	"version", "build", "saved_at",
	"map", "seed", "rng", "hero", "sidecar",
	"clock", "light", "squads", "spawns", "spawner", "notice", "pursuit",
	"corpses", "rising", "combat", "bodies", "entities", "scene",
}

// saveUnmoved saves and requires that the save moved nothing: the digest
// before and after is one digest (every part named when it is not).
func saveUnmoved(t *testing.T, s *session, act string, args map[string]any) map[string]any {
	t.Helper()

	d0, parts0 := digest(s)
	out := s.call("strigoi_save_game", args)
	d1, parts1 := digest(s)

	if d0 != d1 {
		var moved []string

		for k, v := range parts0 {
			if fmt.Sprint(parts1[k]) != fmt.Sprint(v) {
				moved = append(moved, k)
			}
		}

		t.Fatalf("%s: saving moved the world: the digest parts %v changed", act, moved)
	}

	return out
}

// refusedWith requires a save refused with code.
func refusedWith(t *testing.T, s *session, act, code string, args map[string]any) {
	t.Helper()

	if e := s.callErr("strigoi_save_game", args); !strings.HasPrefix(e, code+":") {
		t.Fatalf("%s: the save must be refused %s, got %q", act, code, e)
	}
}

// worldFile decodes a world file and requires every block, in order, and
// this build's version -- d2save.Version, never a literal (the raid's R0.5:
// the version follows Version, so the milestone's bump leaves this green).
func worldFile(t *testing.T, act string, data []byte) map[string]any {
	t.Helper()

	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("%s: the world file is not JSON: %v", act, err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	_, _ = dec.Token()

	var order []string

	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}

		order = append(order, tok.(string))

		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}

	if strings.Join(order, ",") != strings.Join(worldBlocks, ",") {
		t.Fatalf("%s: the world file's blocks are %v, want %v", act, order, worldBlocks)
	}

	if num(file, "version") != float64(d2save.Version) {
		t.Fatalf("%s: version %v", act, file["version"])
	}

	return file
}

// sameSidecar requires the world file's embedded sidecar to be the sidecar
// file's document (compared compact).
func sameSidecar(t *testing.T, act string, file map[string]any, onDisk []byte) {
	t.Helper()

	embedded, err := json.Marshal(file["sidecar"])
	if err != nil {
		t.Fatal(err)
	}

	var disk any
	if err := json.Unmarshal(onDisk, &disk); err != nil {
		t.Fatalf("%s: the sidecar is not JSON: %v", act, err)
	}

	want, _ := json.Marshal(disk)

	if !bytes.Equal(embedded, want) {
		t.Fatalf("%s: the world file's sidecar is not the sidecar file's document:\n embedded %s\n on disk  %s", act, embedded, want)
	}
}

// checkMap: the authored village names its .tmj and its SHA; the generated
// world (the -classic sweep) says it is generated.
func checkMap(t *testing.T, m map[string]any) {
	t.Helper()

	if flag(t, m, "generated") {
		if str(m, "path") != "" || str(m, "sha") != "" {
			t.Fatalf("the generated world has no path or sha: %v", m)
		}

		return
	}

	if !strings.HasSuffix(str(m, "path"), ".tmj") || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(str(m, "sha")) {
		t.Fatalf("an authored map names its .tmj and its sha256: %v", m)
	}
}

// listAt is the list at a dotted path of the file.
func listAt(t *testing.T, file map[string]any, path string) []any {
	t.Helper()

	parts := strings.Split(path, ".")
	cur := file

	for _, p := range parts[:len(parts)-1] {
		cur = sub(cur, p)
	}

	return asList(cur[parts[len(parts)-1]])
}

// nativeHandle is the handle of one of the map's villagers (scene.natives).
func nativeHandle(t *testing.T, s *session) string {
	t.Helper()

	scene := sub(s.call("strigoi_get_system_state", map[string]any{"system": "scene"}), "state")

	natives := map[string]bool{}
	for _, raw := range asList(scene["natives"]) {
		n, _ := raw.(map[string]any)
		natives[str(n, "id")] = true
	}

	list := s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200})

	for _, raw := range asList(list["items"]) {
		e, _ := raw.(map[string]any)
		if natives[str(e, "id")] {
			return str(e, "handle")
		}
	}

	t.Fatalf("no native entity among the npcs (scene natives %v)", scene["natives"])

	return ""
}

func stringsOf(v any) []string {
	var out []string

	for _, x := range asList(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}

	return out
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()

	data, err := readSaved(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	return data
}

// hashFiles is each file's SHA-256, or "absent".
func hashFiles(t *testing.T, paths []string) map[string]string {
	t.Helper()

	out := map[string]string{}

	for _, p := range paths {
		data, err := readSaved(p)

		switch {
		case os.IsNotExist(err):
			out[p] = "absent"
		case err != nil:
			t.Fatalf("reading %s: %v", p, err)
		default:
			sum := sha256.Sum256(data)
			out[p] = hex.EncodeToString(sum[:])
		}
	}

	return out
}

func equalHashes(a, b map[string]string) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// withoutSavedAt is a world file with its saved_at line blanked, and the
// embedded sidecar's generation, which is the same moment (B7).
func withoutSavedAt(t *testing.T, data []byte) []byte {
	t.Helper()

	re := regexp.MustCompile(`(?m)^  "saved_at": "[^"]*",$`)
	if !re.Match(data) {
		t.Fatalf("no saved_at line in the world file")
	}

	gen := regexp.MustCompile(`(?m)^    "generation": "[^"]*",$`)
	if !gen.Match(data) {
		t.Fatalf("no generation line in the world file's sidecar")
	}

	return gen.ReplaceAll(re.ReplaceAll(data, []byte(`  "saved_at": "",`)), []byte(`    "generation": "",`))
}

// generationOf is a sidecar document's generation.
func generationOf(t *testing.T, sidecar []byte) string {
	t.Helper()

	var sc struct {
		Generation string `json:"generation"`
	}

	if err := json.Unmarshal(sidecar, &sc); err != nil {
		t.Fatalf("the sidecar is not JSON: %v", err)
	}

	return sc.Generation
}

// sameHero requires the world file's hero to be the .od2's: its heroName, and
// its heroType by name (the B3 review, A1).
func sameHero(t *testing.T, act string, hero map[string]any, od2 []byte) {
	t.Helper()

	var h struct {
		Name string `json:"heroName"`
		Type int    `json:"heroType"`
	}

	if err := json.Unmarshal(od2, &h); err != nil {
		t.Fatalf("%s: the .od2 is not JSON: %v", act, err)
	}

	classes := []string{"", "Barbarian", "Necromancer", "Paladin", "Assassin", "Sorceress", "Amazon", "Druid"}

	if h.Type < 1 || h.Type >= len(classes) || str(hero, "name") != h.Name || str(hero, "class") != classes[h.Type] {
		t.Fatalf("%s: the world file names %q the %q; the .od2 is %q, heroType %d", act, hero["name"], hero["class"], h.Name, h.Type)
	}
}

// faceSomewhere walks him to a clear neighbour, and back if that left him
// facing direction 0, so a facing the save drops cannot pass for one it kept.
func faceSomewhere(t *testing.T, s *session) {
	t.Helper()

	p := s.call("strigoi_get_player", map[string]any{})
	x, y := num(p, "x"), num(p, "y")
	spot := clearNeighbour(t, s, x, y)

	for _, to := range [][2]float64{spot, {x, y}} {
		s.call("strigoi_move_player_to", map[string]any{"x": to[0], "y": to[1], "wait": true, "max_ticks": 600})
		s.call("strigoi_step", map[string]any{"frames": 2})

		if num(sub(s.call("strigoi_get_player", map[string]any{}), "state"), "direction") != 0 {
			return
		}
	}

	t.Fatalf("walked to a neighbour and back, and he faces direction 0 both ways")
}
