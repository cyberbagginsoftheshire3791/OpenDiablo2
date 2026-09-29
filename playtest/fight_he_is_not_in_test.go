//go:build playtest

package playtest

// TestAFightHeIsNotIn is the raid milestone's R1 script (the village at night,
// 29 Sep 2026): a fight he is not in, in the real game at the SHIPPED combat
// dials (human, paced -- player_control is set back to human after
// start_game, whose launcher drops every script to the policy).
//
// IT IS THE SEPARATION'S ONLY GUARD. Every other strigoi_watch in playtest/
// targets him, and the raid's S0 measured four scripts that touch combat
// (TestTheHandsWait, TestCombatBody, TestTacticalFight, TestTheDeadWalk)
// staying green with the separation removed. So each act asserts what the
// separation promises -- his fight, his PACE and ROUND lines, his XP, his
// dice, his world held -- stay his while the village fights.
//
// Its assertions are t.Errorf, so a negative-control run (a source mutation;
// the raid's R1 build note lists each, and the logs are
// strigoi-harness-runs\wt-raid-r1\pt-ctl-<name>.txt) reports every act that
// goes red, not only the first; setup failures are t.Fatalf.
//
//   TestAFightHeIsNotIn             acts 1-4 (+ C1-C4), the save (Q5 (a)),
//                                   the four speakers (S0-1 (a)), the lines,
//                                   his ids, the re-entrancy probe in the game
//   TestAFightHeIsNotInWhileHeSleeps act 5 (+ C5 by mutation)
//   TestAFightHeIsNotInReplays       act 6 (+ C6): two launches, one seed

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
)

// afhClock is the combat provider's clock block: the fights he is not in.
func afhClock(s *session) map[string]any { return sub(combatState(s), "clock") }

// afhFightFor is the live clock fight after quarry, or nil.
func afhFightFor(s *session, quarry string) map[string]any {
	for _, l := range asList(afhClock(s)["live"]) {
		if m, ok := l.(map[string]any); ok && str(m, "quarry") == quarry {
			return m
		}
	}

	return nil
}

func afhStepWorld(s *session, minutes float64) string {
	return s.callErr("strigoi_step_world", map[string]any{"world_minutes": minutes})
}

func afhFrames(s *session, n int) {
	s.call("strigoi_step", map[string]any{"frames": n})
}

// afhCheck is an assertion that lets the script go on, so a control run
// reports every act it turns red.
func afhCheck(t *testing.T, ok bool, format string, args ...any) {
	t.Helper()

	if !ok {
		t.Errorf("RED "+format, args...)
	}
}

// afhBody is the corpse registry's body of id, or nil.
func afhBody(s *session, id string) map[string]any {
	for _, raw := range asList(corpsesState(s)["bodies"]) {
		if b, _ := raw.(map[string]any); str(b, "id") == id {
			return b
		}
	}

	return nil
}

// afhWatching reports whether the watcher still has a notice row.
func afhWatching(s *session, watcherID string) bool {
	for _, r := range asList(spawnsState(s)["notice_list"]) {
		if m, _ := r.(map[string]any); str(m, "watcher") == watcherID {
			return true
		}
	}

	return false
}

func afhSlain(s *session) float64 { return num(sub(journalState(s), "events"), "slain") }

func TestAFightHeIsNotIn(t *testing.T) {
	s, playerID, playerHandle, px, py := handsStart(t)
	setField(s, "combat", "player_control", "human")

	xp0 := mustNum(t, progressState(s), "xp")
	hp0 := mustNum(t, metersState(s), "health")
	slain0 := afhSlain(s)
	pd0 := mustNum(t, combatState(s), "ended_player_dead")

	t.Logf("MEASURE start: player %s at %.2f,%.2f; his xp %.0f, health %.0f; clock block %v", playerID, px, py, xp0, hp0, afhClock(s))

	// --- C1: the same bodies with no watch -- no fight, and A untouched ------
	a := spawnNPC(t, s, "fallen1", px+15, py)
	b := spawnNPC(t, s, "zombie1", px+16, py)
	aID, bID := entityID(t, s, a), entityID(t, s, b)
	started0 := mustNum(t, afhClock(s), "started")

	for i := 0; i < 2; i++ {
		if e := afhStepWorld(s, 1.0); e != "" {
			t.Fatalf("C1: step_world refused with no fight anywhere: %s", e)
		}
	}

	afhCheck(t, afhFightFor(s, aID) == nil && mustNum(t, afhClock(s), "started") == started0,
		"C1: a clock fight opened with no watch: %v", afhClock(s))

	// --- act 1: B watches A; a clock fight opens, and it is not his --------
	s.call("strigoi_watch", map[string]any{"watcher": b, "target": a})

	frames := 0
	for ; frames < 60 && afhFightFor(s, aID) == nil && !flag(t, combatState(s), "fighting"); frames++ {
		afhFrames(s, 1)
	}

	c, f, ui := combatState(s), afhFightFor(s, aID), systemState(s, "ui")
	t.Logf("MEASURE act1: A=%s B=%s frames=%d clock_fight=%v fighting=%v world_held=%v awaiting=%v world_held_by=%q encounter=%q",
		aID, bID, frames, f, c["fighting"], c["world_held"], c["awaiting"], str(ui, "world_held_by"), str(c, "encounter"))

	if f == nil {
		t.Fatalf("RED act1: no clock fight for A (%s) after %d frames; combat %v", aID, frames, c)
	}

	afhCheck(t, strings.HasPrefix(str(f, "id"), "c:"), "act1: the clock fight's id %q is not of the clock's sequence", str(f, "id"))
	afhCheck(t, !flag(t, c, "fighting"), "act1: fighting=true -- a fight he is not in took his slot")
	afhCheck(t, !flag(t, c, "world_held"), "act1: world_held=true -- a fight he is not in holds the world")
	afhCheck(t, !flag(t, c, "awaiting"), "act1: awaiting=true -- his key would act for A")
	afhCheck(t, str(ui, "world_held_by") == "", "act1: ui.world_held_by=%q", str(ui, "world_held_by"))
	afhCheck(t, str(c, "encounter") == "", "act1: his encounter is %q", str(c, "encounter"))
	afhCheck(t, num(f, "quarry_health") == num(f, "quarry_max_health") && num(f, "quarry_max_health") > 1,
		"C1: A's health at the fight's opening %v of %v: something struck him before the watch", f["quarry_health"], f["quarry_max_health"])

	clockID := str(f, "id")

	// --- act 2: world minutes buy clock rounds, one each; C2 ------------------
	w0, r0, s0, qh0 := worldMinutes(t, s), num(f, "round"), num(f, "minutes_into_round"), num(f, "quarry_health")

	refused := ""
	for i := 0; i < 5 && refused == ""; i++ {
		refused = afhStepWorld(s, 1.0)
	}

	w1 := worldMinutes(t, s)
	f = afhFightFor(s, aID)
	c = combatState(s)

	if f == nil {
		t.Fatalf("RED act2: A's clock fight ended within five minutes: %v", afhClock(s))
	}

	want := math.Floor(s0 + (w1 - w0) + 1e-9)
	t.Logf("MEASURE act2: world minutes %.4f -> %.4f (+%.4f), into round %.4f; clock round %.0f -> %.0f (want +%.0f); A %.0f -> %.0f; his health %.0f -> %.0f; refused=%q",
		w0, w1, w1-w0, s0, r0, num(f, "round"), want, qh0, num(f, "quarry_health"), hp0, mustNum(t, metersState(s), "health"), refused)
	afhCheck(t, refused == "", "act2: step_world refused while only the village fought: %s", refused)
	afhCheck(t, num(f, "round")-r0 == want, "act2: %.0f clock rounds in %.4f world minutes (from %.4f into the round), want %.0f",
		num(f, "round")-r0, w1-w0, s0, want)
	afhCheck(t, num(f, "quarry_health") < qh0, "act2: A's health did not fall (%.0f -> %.0f)", qh0, num(f, "quarry_health"))
	afhCheck(t, mustNum(t, metersState(s), "health") == hp0, "act2: his health moved")
	afhCheck(t, mustNum(t, c, "encounters") == 0 && mustNum(t, c, "actions_total") == 0,
		"act2: his fight records moved: encounters=%v actions_total=%v", c["encounters"], c["actions_total"])

	for _, row := range asList(afhClock(s)["actions"]) {
		m, _ := row.(map[string]any)
		afhCheck(t, str(m, "target") == aID || str(m, "attacker") == aID, "act2: a clock blow not about A: %v", m)
	}

	setField(s, "clock", "frozen", true)
	rf0 := num(afhFightFor(s, aID), "round")
	afhFrames(s, 120)
	rf1 := num(afhFightFor(s, aID), "round")
	setField(s, "clock", "frozen", false)
	t.Logf("MEASURE C2: a frozen clock, 120 frames: clock round %.0f -> %.0f", rf0, rf1)
	afhCheck(t, rf1 == rf0, "C2: a frozen clock resolved %.0f rounds", rf1-rf0)

	// --- the save (Q5 (a)): a fight he is not in never stops a save ---------
	to := filepath.Join(t.TempDir(), "village-fighting.world.json")
	d0, _ := digest(s)
	e := s.callErr("strigoi_save_game", map[string]any{"to": to})
	d1, _ := digest(s)

	afhCheck(t, e == "", "save: refused while only the village fought (Q5 (a)): %s", e)
	afhCheck(t, d0 == d1, "save: saving moved the world")

	if e == "" {
		data, err := os.ReadFile(to) // nolint:gosec // the test's own temp file
		if err != nil {
			t.Fatalf("save: %v", err)
		}

		w, err := d2save.Decode(data)
		afhCheck(t, err == nil, "save: the file the save wrote does not decode: %v", err)

		var file map[string]any
		if err := json.Unmarshal(data, &file); err != nil {
			t.Fatalf("save: %v", err)
		}

		live := asList(sub(sub(file, "combat"), "clock")["live"])
		afhCheck(t, len(live) == 1, "save: the file holds %d live clock fights, want 1", len(live))

		if len(live) == 1 {
			l, _ := live[0].(map[string]any)
			afhCheck(t, str(l, "id") == clockID && str(l, "quarry") == aID, "save: the saved clock fight is %v", l)
		}

		if w != nil {
			afhCheck(t, w.Version == d2save.Version && w.RNG.CombatClock == w.Combat.Clock.RNG,
				"save: version %d, rng.combat_clock %+v, combat.clock.rng %+v", w.Version, w.RNG.CombatClock, w.Combat.Clock.RNG)
		}

		t.Logf("MEASURE save: %d bytes at %s, version %v, clock.live %v", len(data), to, file["version"], live)
	}

	// --- C4: the fight runs to its end; B's death pays him nothing -----------
	ck0 := afhClock(s)

	for i := 0; i < 60 && afhFightFor(s, aID) != nil; i++ {
		afhStepWorld(s, 1.0)
	}

	ck := afhClock(s)
	xp1 := mustNum(t, progressState(s), "xp")
	t.Logf("MEASURE C4: clock ended=%v reason=%q enemies_dead %v -> %v quarry_dead %v; xp_suppressed %v -> %v; his xp %.0f -> %.0f; journal slain %.0f -> %.0f",
		ck["ended"], str(ck, "ended_reason"), ck0["ended_enemies_dead"], ck["ended_enemies_dead"], ck["ended_quarry_dead"],
		ck0["xp_suppressed"], ck["xp_suppressed"], xp0, xp1, slain0, afhSlain(s))
	afhCheck(t, afhFightFor(s, aID) == nil, "C4: A's fight did not end in 60 world minutes")
	afhCheck(t, num(ck, "ended_enemies_dead") == num(ck0, "ended_enemies_dead")+1 || num(ck, "ended_quarry_dead") == num(ck0, "ended_quarry_dead")+1,
		"C4: the fight ended neither way: %v", ck)
	afhCheck(t, num(ck, "xp_suppressed") > num(ck0, "xp_suppressed") || num(ck, "ended_quarry_dead") > num(ck0, "ended_quarry_dead"),
		"C4: a kill in a clock fight was not counted as suppressed")
	afhCheck(t, xp1 == xp0, "C4: his XP moved %.0f -> %.0f on a clock fight's kill", xp0, xp1)
	afhCheck(t, afhSlain(s) == slain0, "C4: the journal noted a slaying he did not do")

	// --- act 3 (M0.1c): lockstep with his paced fight ------------------------
	a2 := spawnNPC(t, s, "fallen1", px+15, py+3)
	b2 := spawnNPC(t, s, "zombie1", px+16, py+3)
	a2ID := entityID(t, s, a2)
	s.call("strigoi_watch", map[string]any{"watcher": b2, "target": a2})

	for i := 0; i < 60 && afhFightFor(s, a2ID) == nil; i++ {
		afhFrames(s, 1)
	}

	if afhFightFor(s, a2ID) == nil {
		t.Fatalf("RED act3: no clock fight for A2")
	}

	foe := spawnNPC(t, s, "zombie1", px+1, py)
	s.call("strigoi_watch", map[string]any{"watcher": foe, "target": playerHandle})
	openTurn(t, s)

	c = combatState(s)
	afhCheck(t, str(c, "encounter") == "e:1", "act3: his first fight is %q, not e:1: the village's fights spent his ids", str(c, "encounter"))
	afhCheck(t, num(c, "next_id") == num(c, "encounters")+1, "act3: his next_id %v is not one past his %v fights (B1's rule)",
		c["next_id"], c["encounters"])

	cr := func() float64 {
		if f := afhFightFor(s, a2ID); f != nil {
			return num(f, "round")
		}

		return -1
	}

	hr0, cr0 := mustNum(t, combatState(s), "round"), cr()
	afhFrames(s, 600)
	cr1 := cr()
	t.Logf("MEASURE C3: his turn open 600 frames: clock round %.0f -> %.0f; awaiting=%v", cr0, cr1, flag(t, combatState(s), "awaiting"))
	afhCheck(t, cr1 == cr0 && cr0 > 0, "C3: the clock fight advanced %.0f rounds while his turn was open", cr1-cr0)

	for turn := 0; turn < 3; turn++ {
		s.call("strigoi_key", map[string]any{"key": "e"})
		afhFrames(s, 2)
		openTurn(t, s)
	}

	c = combatState(s)
	hr1, cr2 := mustNum(t, c, "round"), cr()
	t.Logf("MEASURE act3: his round %.0f -> %.0f; clock round %.0f -> %.0f; his encounter %q",
		hr0, hr1, cr0, cr2, str(c, "encounter"))
	afhCheck(t, hr1-hr0 == 3 && cr2-cr0 == hr1-hr0, "act3: the clock fight advanced %.0f rounds while his closed %.0f", cr2-cr0, hr1-hr0)

	// His fight ends under the policy, as hands_test ends one.
	setField(s, "combat", "player_control", "policy")

	if flag(t, combatState(s), "awaiting") {
		setField(s, "combat", "commit", "hold")
	}

	for i := 0; i < 60 && flag(t, combatState(s), "fighting"); i++ {
		afhStepWorld(s, 1.0)
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatalf("act3: his fight did not end: %v", combatState(s))
	}

	setField(s, "combat", "player_control", "human")

	// --- act 4: a clock quarry dies -------------------------------------------
	c = combatState(s)
	xp2, pd2, er2, slain2 := mustNum(t, progressState(s), "xp"), mustNum(t, c, "ended_player_dead"), str(c, "ended_reason"), afhSlain(s)
	q0 := mustNum(t, afhClock(s), "ended_quarry_dead")
	rel0 := mustNum(t, afhClock(s), "released")

	a3 := spawnNPC(t, s, "fallen1", px+15, py+6)
	a3ID := entityID(t, s, a3)
	a3e := s.call("strigoi_get_entity", map[string]any{"handle": a3})
	killers := []string{}

	for i, dx := range []float64{1, -1, 0} {
		dy := 0.0
		if i == 2 {
			dy = 1
		}

		k := spawnNPC(t, s, "zombie1", px+15+dx, py+6+dy)
		killers = append(killers, entityID(t, s, k))
		s.call("strigoi_watch", map[string]any{"watcher": k, "target": a3})
	}

	setField(s, "combat", "forced_band", "crit")

	for i := 0; i < 40 && mustNum(t, afhClock(s), "ended_quarry_dead") == q0; i++ {
		afhStepWorld(s, 1.0)
	}

	setField(s, "combat", "forced_band", "")
	afhFrames(s, 30)

	shot := s.call("strigoi_screenshot", map[string]any{"name": "raid-r1-quarry-dead"})
	body := afhBody(s, a3ID)
	ck = afhClock(s)
	c = combatState(s)
	mode := str(sub(s.call("strigoi_get_entity", map[string]any{"handle": a3}), "state"), "animation_mode")

	t.Logf("MEASURE act4: A3 %s; body %v; mode %q; clock quarry_dead %.0f -> %v, released %.0f -> %v; his ended_player_dead %.0f -> %v, ended_reason %q -> %q; xp %.0f -> %.0f; screenshot %s",
		a3ID, body, mode, q0, ck["ended_quarry_dead"], rel0, ck["released"], pd2, c["ended_player_dead"], er2, str(c, "ended_reason"),
		xp2, mustNum(t, progressState(s), "xp"), str(shot, "path"))

	afhCheck(t, num(ck, "ended_quarry_dead") == q0+1, "act4: clock ended_quarry_dead did not rise by one")
	afhCheck(t, body != nil && str(body, "class") == "human" && str(body, "state") == "fresh",
		"act4: no fresh human body for A3: %v", body)

	if body != nil {
		afhCheck(t, math.Floor(num(body, "x")) == math.Floor(num(a3e, "x")) && math.Floor(num(body, "y")) == math.Floor(num(a3e, "y")),
			"act4: A3's body at %v,%v, not on his tile %v,%v", body["x"], body["y"], a3e["x"], a3e["y"])
	}

	afhCheck(t, mode == "DT" || mode == "DD", "act4: A3 did not play his death: mode %q", mode)
	afhCheck(t, mustNum(t, c, "ended_player_dead") == pd2 && str(c, "ended_reason") == er2,
		"act4: his ending moved: ended_player_dead %v, ended_reason %q", c["ended_player_dead"], str(c, "ended_reason"))
	afhCheck(t, mustNum(t, progressState(s), "xp") == xp2, "act4: his XP moved")
	afhCheck(t, afhSlain(s) == slain2, "act4: the journal noted a slaying he did not do")
	afhCheck(t, num(ck, "released") >= rel0+3, "act4: the killers' watches were not all released (%v -> %v)", rel0, ck["released"])

	for _, k := range killers {
		afhCheck(t, !afhWatching(s, k), "act4: killer %s still watches the dead A3 (the wedge)", k)
	}

	// --- S0-1 (a): the four speakers are no quarry ----------------------------
	speakers := afhSpeakers(t, s)
	started1 := mustNum(t, afhClock(s), "started")

	for _, code := range []string{"kashya", "charsi", "warriv1", "akara"} {
		sp, ok := speakers[code]
		if !ok {
			t.Errorf("RED speakers: no %s on the map", code)
			continue
		}

		spID := str(sp, "id")
		sx, sy := num(sp, "x"), num(sp, "y")

		s.call("strigoi_move_player_to", map[string]any{"x": sx - 1.5, "y": sy + 1.5, "wait": true, "max_ticks": 3000})

		k := spawnNPC(t, s, "fallen1", sx+1, sy)
		kID := entityID(t, s, k)
		s.call("strigoi_watch", map[string]any{"watcher": k, "target": str(sp, "handle")})

		for i := 0; i < 3; i++ {
			afhStepWorld(s, 1.0)
		}

		t.Logf("MEASURE speaker %s (%s): watched by %s (watching=%v); clock fight %v; body %v",
			code, spID, kID, afhWatching(s, kID), afhFightFor(s, spID), afhBody(s, spID))
		afhCheck(t, afhFightFor(s, spID) == nil, "speakers: a fight opened on %s", code)
		afhCheck(t, afhBody(s, spID) == nil, "speakers: %s lies dead", code)
	}

	afhCheck(t, mustNum(t, afhClock(s), "started") == started1, "speakers: a clock fight opened on a speaker (started %.0f -> %v)",
		started1, afhClock(s)["started"])

	// --- the lines stay his; nothing re-entered; his ids -----------------------
	rounds, paces := ringLines(s, "ROUND encounter="), ringLines(s, "PACE ")

	for _, l := range append(append([]string{}, rounds...), paces...) {
		afhCheck(t, !strings.Contains(l, "encounter=c:"), "lines: a ROUND or PACE line for a fight he was not in: %s", l)
	}

	for _, l := range paces {
		t.Logf("MEASURE PACE: %s", l)
	}

	ck, c = afhClock(s), combatState(s)
	t.Logf("MEASURE end: %d ROUND and %d PACE lines; clock started %v ended %v rounds %v reentrant_reads %v; his encounters %v next_id %v",
		len(rounds), len(paces), ck["started"], ck["ended"], ck["rounds"], ck["reentrant_reads"], c["encounters"], c["next_id"])
	afhCheck(t, len(paces) >= 1, "lines: his own fight wrote no PACE line")
	afhCheck(t, num(ck, "reentrant_reads") == 0, "a game callback read Fighting, Encounter or Awaiting during a clock step (%v)", ck["reentrant_reads"])
	afhCheck(t, num(c, "next_id") == num(c, "encounters")+1, "his next_id %v is not one past his %v fights", c["next_id"], c["encounters"])
	afhCheck(t, mustNum(t, combatState(s), "ended_player_dead") == pd0, "his ended_player_dead moved over the whole script")
}

// afhSpeakers finds the map's four speakers by where the village map places
// them (the raid's S0 scaffold's list).
func afhSpeakers(t *testing.T, s *session) map[string]map[string]any {
	t.Helper()

	want := map[string][2]float64{"warriv1": {28.5, 22.5}, "kashya": {25.5, 25.5}, "charsi": {25.5, 29.5}, "akara": {19.5, 16.5}}
	out := map[string]map[string]any{}

	for _, raw := range asList(s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200})["items"]) {
		e, _ := raw.(map[string]any)

		for code, at := range want {
			if math.Hypot(num(e, "x")-at[0], num(e, "y")-at[1]) < 0.6 {
				out[code] = e
			}
		}
	}

	return out
}

// TestAFightHeIsNotInWhileHeSleeps is act 5: he sleeps a shelter night while a
// clock fight runs a dozen tiles off, and the sleep runs its whole four hours
// -- talk.go's loop reads inFight, which is his fight. (C5, the pre-R1 reader,
// is the no-separation mutation: the sleep stops at its first minute.)
func TestAFightHeIsNotInWhileHeSleeps(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Sleeper", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	setField(s, "combat", "player_control", "human")

	for i := 0; i < 40 && str(clockState(s), "stage") != "night"; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 60.0})
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
	}

	if str(clockState(s), "stage") != "night" {
		t.Fatalf("act 5: no night: %v", clockState(s))
	}

	setField(s, "meters", "fatigue", 80.0)
	setField(s, "village", "rep", 40.0) // shelter

	headman := villager(t, s, "Warriv")
	walkNear(t, s, headman)

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")

	a := spawnNPC(t, s, "zombie1", px+12, py)
	b := spawnNPC(t, s, "fallen1", px+13, py)
	aID := entityID(t, s, a)
	s.call("strigoi_watch", map[string]any{"watcher": b, "target": a})

	for i := 0; i < 60 && afhFightFor(s, aID) == nil; i++ {
		afhFrames(s, 1)
	}

	afhCheck(t, afhFightFor(s, aID) != nil, "act5: no clock fight twelve tiles off")
	afhCheck(t, !flag(t, combatState(s), "fighting"), "act5: the clock fight is his")

	openTalkWith(t, s, headman)

	if str(villageState(s), "node") == "headman_first" {
		answer(t, s, 3) // say nothing, and go -- met
		openTalkWith(t, s, headman)
	}

	answer(t, s, answerIndex(t, s, "sleep inside"))

	before, rounds0 := worldMinutes(t, s), mustNum(t, afhClock(s), "rounds")

	answer(t, s, 1) // sleep four hours

	slept, rounds1 := worldMinutes(t, s)-before, mustNum(t, afhClock(s), "rounds")
	t.Logf("MEASURE act5: slept %.2f world minutes; clock rounds %.0f -> %.0f during it; clock %v", slept, rounds0, rounds1, afhClock(s))
	afhCheck(t, math.Abs(slept-240) <= 0.5, "act5: his sleep stopped after %.2f of 240 minutes: a fight he was not in woke him", slept)
	afhCheck(t, rounds1 > rounds0, "act5: the clock fight did not run while he slept")
	afhCheck(t, !flag(t, combatState(s), "fighting"), "act5: he is in a fight")
}

// afhReplay is one launch of act 6: a clock fight on seed, six world minutes,
// its blows round by round with the two entity ids written A and B.
func afhReplay(t *testing.T, seed int64) []string {
	t.Helper()

	s := start(t)
	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Replay", "hero_class": "amazon", "seed": seed, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	setField(s, "combat", "player_control", "human")

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")

	a := spawnNPC(t, s, "fallen1", px+15, py)
	b := spawnNPC(t, s, "zombie1", px+16, py)
	aID, bID := entityID(t, s, a), entityID(t, s, b)
	s.call("strigoi_watch", map[string]any{"watcher": b, "target": a})

	for i := 0; i < 60 && afhFightFor(s, aID) == nil; i++ {
		afhFrames(s, 1)
	}

	if afhFightFor(s, aID) == nil {
		t.Fatalf("act6: no clock fight on seed %d", seed)
	}

	name := strings.NewReplacer(aID, "A", bID, "B")
	byRound := map[float64]string{}

	for i := 0; i < 6; i++ {
		afhStepWorld(s, 1.0)

		ck := afhClock(s)
		rows := []string{}

		for _, raw := range asList(ck["actions"]) {
			m, _ := raw.(map[string]any)
			rows = append(rows, name.Replace(fmt.Sprintf("%s>%s roll=%v band=%v dmg=%v after=%v",
				str(m, "attacker"), str(m, "target"), m["roll"], m["band"], m["damage"], m["target_health_after"])))
		}

		byRound[num(ck, "actions_round")] = strings.Join(rows, "; ")
	}

	out := []string{}
	for r := 1.0; r <= 8; r++ {
		if rows, ok := byRound[r]; ok {
			out = append(out, fmt.Sprintf("round %.0f: %s", r, rows))
		}
	}

	s.stop()

	return out
}

// TestAFightHeIsNotInReplays is act 6: two launches on one seed fight the
// village's fight blow for blow (TestTownWalkDeterministic's shape), and C6,
// a third on another seed, does not.
func TestAFightHeIsNotInReplays(t *testing.T) {
	first, second := afhReplay(t, 1462), afhReplay(t, 1462)

	for _, r := range first {
		t.Logf("MEASURE act6 seed 1462: %s", r)
	}

	afhCheck(t, len(first) >= 4, "act6: only %d clock rounds recorded", len(first))
	afhCheck(t, strings.Join(first, "\n") == strings.Join(second, "\n"), "act6: one seed, two launches, two fights:\n%s\n---\n%s",
		strings.Join(first, "\n"), strings.Join(second, "\n"))

	other := afhReplay(t, 7)
	for _, r := range other {
		t.Logf("MEASURE C6 seed 7: %s", r)
	}

	afhCheck(t, strings.Join(first, "\n") != strings.Join(other, "\n"), "C6: another seed fought the same fight: the instrument cannot see dice")
}
