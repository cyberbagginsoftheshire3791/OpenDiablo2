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
	"sort"
	"strings"
	"testing"
	"time"
)

// TestSaveResume is M4.6's acceptance test (build plan section 4): the save
// "stops at that point then resumes at that point" (Josh, 25 Sep 2026).
//
// ACTS 1-6 ARE BURST B4a's -- A QUIET EVENING RESUMES (29 Sep 2026) -- and
// they are the test the whole milestone was designed around: save at T, step
// 120 world minutes, relaunch, load, and the relaunched game must equal the
// original at T and again at T+120. Act 7 is B3's (the save verb and its
// refusals), run first in a process of its own (saveVerbActs, below). Act 8
// (rule 4, a save mid-walk) and B4b's hunted night are not here yet; act 9's
// omit sweep is B6's.
//
//	1  Seed 99, stepped, the default game. An evening that fills every block
//	   B4a resumes, with no pack arriving: the kit (the default loadout), a
//	   fight by day (a dog slain, then taken off the map with his body), two
//	   forages and three stakes whittled, one of Night 1's dead staked and one
//	   in a hasty grave, the other two risen in the first night and staked by
//	   day where first light laid them down (a risen body Closed), the watch
//	   promised in a talk and stood at the headman's post from true dark, and
//	   his torch lit. The precondition names every block and requires it
//	   non-empty.
//	2  At T -- 01:30 of the third day, standing still at the post -- the save,
//	   which moves nothing (the digest before and after is one digest).
//	3  120 world minutes more: dawn comes (the night paid, the watch kept, the
//	   rite, the soul pressure's count) and the torch burns out; S_U. Then he
//	   is wounded and leaves through the menu, which writes his .od2 and his
//	   sidecar and not the world file (B5's): the files a load finds beside
//	   the world file are two hours and a wound later than it.
//	4  A new process in the same home: start_game{save_path, seed 99} RESUMES
//	   the file (the load reports it, and its steps), and S_R0 = S_T: the
//	   resume digest -- world, entities, rng and every system's world state --
//	   and each named system. The world file won over the later .od2 (his
//	   health) and the later sidecar (a kit whose torch had burnt out).
//	5  Saved again at once: the file is T's file byte for byte, but saved_at
//	   and the sidecar's generation (which is saved_at).
//	6  120 world minutes: S_R = S_U.
//	6b THE IN-PROCESS PATH: he dies, and the death screen's "load last save"
//	   (Enter; App.ReloadGame) resumes the saved moment -- not the dawn -- in
//	   this process: S = S_T again, the uuid stream and the script's dials
//	   put back by the load.
//	6c-6e THE REFUSALS, each falling back to dawn with the file set aside
//	   (rule 7): another hero's world file (6c), a torn save -- the sidecar of
//	   another generation (6d) -- and a changed map (6e, D5), which is refused
//	   after the game opened and so tears that game down.
//
// THE COMPARISON (BUG-58 fixed, 29 Sep 2026): the digest's resume_digest is
// every part a resumed game must reproduce -- not sim (the harness's clock)
// and not process (this process's history: the files it loaded, the games it
// began, the torch verbs it counted) -- and screen coordinates are in no part.
//
// FOR THE NEGATIVE CONTROLS, two knobs, read here only: STRIGOI_SAVE_RESUME_
// ONLY=b4a skips act 7, and STRIGOI_SAVE_RESUME_FROM=<dir> takes acts 1-3
// from an evening a green run kept there (the kit's hero files at T and S_T,
// S_U) and runs acts 4-6e against it, so a control that breaks the LOAD is
// seen in minutes. The green run keeps its evening at
// <run dir>/pt/TestSaveResume/evening.
func TestSaveResume(t *testing.T) {
	if os.Getenv("STRIGOI_SAVE_RESUME_ONLY") != "b4a" {
		saveVerbActs(t)
	}

	ev := eveningActs1to3(t)
	eveningActs4to6(t, ev)
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
}

type dialWrite struct {
	System, Field string
	Value         any
}

// eveningDials are the dials acts 1-3 leave set at T. A dial is never saved
// (trap 7): a relaunched game has the defaults until the script sets them
// again, and "load last save" in one process has them re-applied by the load.
var eveningDials = []dialWrite{
	{"spawns", "chance", 0},
	{"spawns", "notice_radius", 0.5},
	{"rising", "edge_floor", 0},
	{"rising", "hasty_weight", 0.0},
	{"rising", "p", 0.0},
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

	for _, d := range eveningDials {
		setField(s, d.System, d.Field, d.Value)
	}

	// The night's dead rise in the first night, and nothing notices him.
	setField(s, "rising", "p", 1.0)
	setField(s, "village", "rep", 30.0) // state: the headman offers the watch

	// --- 1: a fight by day: a dog slain, then taken off the map ------------
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

	if removed := s.call("strigoi_remove_entity", map[string]any{"handle": dog}); removed["body_dropped"] != true {
		t.Fatalf("act 1: the slain dog leaves the map with his body: %v", removed)
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

	// --- 1: the second night, at the post; the torch lit late -------------
	stepTo(t, s, 1, 21*60+20)
	walkNear(t, s, headman)
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

	// --- 1: THE PRECONDITION: every block B4a resumes is non-empty ---------
	eveningPrecondition(t, s)

	// --- 2: the save at T --------------------------------------------------
	ev.sT = snapWorld(t, s)
	out := s.call("strigoi_save_game", map[string]any{})
	sameWorld(t, "act 2 (the save moved nothing)", ev.sT, snapWorld(t, s))

	world := str(out, "world_path")
	ev.fileT, ev.od2T, ev.sidecarT = mustRead(t, world), mustRead(t, ev.save), mustRead(t, ev.save+".strigoi.json")

	file := worldFile(t, "act 2", ev.fileT)
	for _, block := range []string{"spawns.groups", "notice.watches", "pursuit.chases", "bodies"} {
		if n := len(listAt(t, file, block)); n != 0 {
			t.Fatalf("act 2: a quiet evening has no %s: %d", block, n)
		}
	}

	for _, raw := range listAt(t, file, "entities") {
		if e, _ := raw.(map[string]any); !flag(t, e, "native") {
			t.Fatalf("act 2: a quiet evening has only the villagers on the map: %v", e)
		}
	}

	t.Logf("act 2 PASS: saved at %s (%s, day %v), %d bytes, the digest unmoved",
		str(clockState(s), "time_of_day"), str(clockState(s), "stage"), clockState(s)["day_index"], len(ev.fileT))

	// --- 3: 120 world minutes on; quit -------------------------------------
	s.call("strigoi_step_world", map[string]any{"world_minutes": 120})
	ev.sU = snapWorld(t, s)

	if ev.sU.Resume == ev.sT.Resume {
		t.Fatal("act 3: 120 minutes moved nothing, so act 6 would compare nothing")
	}

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

	ev.savedDial = eveningDials
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
		{"no noticed pack", mustNum(t, spawnsState(s), "groups") == 0},
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

	for name, data := range map[string][]byte{
		"hero.od2": ev.od2Left, "hero.od2.strigoi.json": ev.sidecarLeft, "hero.od2.world.json": ev.fileT,
		"hero-at-t.od2": ev.od2T, "hero-at-t.od2.strigoi.json": ev.sidecarT, "states.json": states,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Logf("keeping the evening: %v", err)
		}
	}

	t.Logf("the evening is kept at %s", dir)
}

// keptEvening is an evening a green run kept: its hero's files are copied
// into this test's home, where a relaunch finds them.
func keptEvening(t *testing.T, from string) evening {
	t.Helper()

	ev := evening{savedDial: eveningDials}
	ev.od2Left, ev.sidecarLeft, ev.fileT = mustRead(t, filepath.Join(from, "hero.od2")),
		mustRead(t, filepath.Join(from, "hero.od2.strigoi.json")), mustRead(t, filepath.Join(from, "hero.od2.world.json"))
	ev.od2T, ev.sidecarT = mustRead(t, filepath.Join(from, "hero-at-t.od2")), mustRead(t, filepath.Join(from, "hero-at-t.od2.strigoi.json"))

	var states map[string]worldSnap
	if err := json.Unmarshal(mustRead(t, filepath.Join(from, "states.json")), &states); err != nil {
		t.Fatalf("the kept evening's states: %v", err)
	}

	ev.sT, ev.sU = states["t"], states["u"]

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
	g := s.call("strigoi_start_game", map[string]any{"save_path": ev.save, "seed": 99, "wait_seconds": 90})

	load := sub(g, "load")
	if !flag(t, load, "resumed") || str(load, "saved_at") != str(worldFile(t, "act 4", ev.fileT), "saved_at") {
		t.Fatalf("act 4: start_game resumes the world file: %v", load)
	}

	for _, d := range ev.savedDial {
		setField(s, d.System, d.Field, d.Value)
	}

	sameWorld(t, "act 4 (S_R0 = S_T)", ev.sT, snapWorld(t, s))
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

	refusedToDawn(t, s, "act 6c (another hero's)", otherSave, "HERO", ev.fileT, false)

	// --- 6d: a torn save: his sidecar of another generation -----------------
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	torn := regexp.MustCompile(`"generation": "[^"]*"`).ReplaceAll(mustRead(t, ev.save+".strigoi.json"), []byte(`"generation": "a save cut off"`))
	if err := os.WriteFile(ev.save+".strigoi.json", torn, 0o600); err != nil {
		t.Fatal(err)
	}

	refusedToDawn(t, s, "act 6d (a torn save)", ev.save, "TORN", fileR, false)

	// --- 6e: a changed map (D5), refused after the game opened --------------
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	moved := regexp.MustCompile(`"sha": "[0-9a-f]{64}"`).ReplaceAll(fileR, []byte(`"sha": "`+strings.Repeat("0", 64)+`"`))
	if bytes.Equal(moved, fileR) {
		t.Fatal("act 6e: the file names no map sha to change (the default game builds the authored village)")
	}

	if err := os.WriteFile(ev.save+".world.json", moved, 0o600); err != nil {
		t.Fatal(err)
	}

	sidecar := worldFile(t, "act 6e", fileR)["sidecar"]
	doc, _ := json.MarshalIndent(sidecar, "", "  ")

	if err := os.WriteFile(ev.save+".strigoi.json", doc, 0o600); err != nil {
		t.Fatal(err)
	}

	refusedToDawn(t, s, "act 6e (a changed map)", ev.save, "MAP", moved, true)
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

		return sub(info, "load")
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

// refusedToDawn starts the hero at save and requires the load refused with
// code, the file set aside whole (rule 7), and him at dawn in a living game.
// afterOpen is a refusal that tears an opened game down (fell_back).
func refusedToDawn(t *testing.T, s *session, act, save, code string, file []byte, afterOpen bool) {
	t.Helper()

	g := s.call("strigoi_start_game", map[string]any{"save_path": save, "seed": 99, "wait_seconds": 90})
	load := sub(g, "load")

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

	t.Logf("%s PASS: refused %s (%s), set aside as %s; he begins at dawn", act, code, str(load, "reason"), filepath.Base(aside))
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

// worldFile decodes a world file and requires every block, in order,
// version 1.
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

	if num(file, "version") != 1 {
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
