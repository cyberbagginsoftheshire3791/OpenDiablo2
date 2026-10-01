//go:build playtest

package playtest

// THE RAID'S R2 SCRIPTS (the village at night, 29 Sep 2026): who the night's
// hunters choose among the living, in the real game.
//
//   TestTheNearestLiving      act 1: a watcher with two visible quarries takes
//                             the nearer (a stand-in villager, not him);
//                             act 2, the brief's control: the distances
//                             swapped, and the choice flips; act 3: a
//                             villager's own watch (side:living) sees a
//                             monster and never chases it or fights it;
//                             act 4 (S0-1 (a)): a hostile with only a speaker
//                             in view keeps its target.
//   TestHisSleepHidesOnlyHim  his shelter sleep hides him, not the village: a
//                             hostile watching him turns, while he sleeps, to
//                             the stand-in villager it can see, and comes for
//                             her.
//
// The source-mutation controls (the brief's) are in "The Village at Night -
// R2 Build Note - 29 Sep 2026.md": the global hidden put back (TestHisSleep-
// HidesOnlyHim red), the chases reading every side (act 3 red), the speakers'
// protection lifted (act 4 red). Each act's assertions are t.Errorf through
// nlCheck, so a control run reports every act it turns red.

import (
	"math"
	"testing"
)

// seekState reads the seek provider.
func seekState(s *session) map[string]any { return systemState(s, "seek") }

// seekRowOf is the seek provider's row for a watcher id, or nil.
func seekRowOf(s *session, watcherID string) map[string]any {
	for _, raw := range asList(seekState(s)["rows"]) {
		if m, _ := raw.(map[string]any); str(m, "watcher") == watcherID {
			return m
		}
	}

	return nil
}

// noticeOf is the notice row for a watcher id (notice_list), or nil.
func noticeOf(s *session, watcherID string) map[string]any {
	for _, raw := range asList(spawnsState(s)["notice_list"]) {
		if m, _ := raw.(map[string]any); str(m, "watcher") == watcherID {
			return m
		}
	}

	return nil
}

// chaseOf is the pursuit row for a hunter id, or nil.
func chaseOf(s *session, hunterID string) map[string]any {
	for _, raw := range asList(systemState(s, "pursuit")["chase_list"]) {
		if m, _ := raw.(map[string]any); str(m, "hunter") == hunterID {
			return m
		}
	}

	return nil
}

func nlCheck(t *testing.T, ok bool, format string, args ...any) {
	t.Helper()

	if !ok {
		t.Errorf("RED "+format, args...)
	}
}

// nlClear is find_path's straight_line_clear between two points.
func nlClear(t *testing.T, s *session, fx, fy, tx, ty float64) bool {
	t.Helper()

	return flag(t, s.call("strigoi_find_path", map[string]any{"from_x": fx, "from_y": fy, "to_x": tx, "to_y": ty}),
		"straight_line_clear")
}

// nlBearings are the eight directions nlPlace tries, in order.
var nlBearings = [][2]float64{
	{1, 0}, {0, 1}, {-1, 0}, {0, -1},
	{0.7071, 0.7071}, {-0.7071, 0.7071}, {0.7071, -0.7071}, {-0.7071, -0.7071},
}

// nlPlace finds a bearing from (px, py) -- trying them from the from-th on,
// so two acts do not stand their casts on one another -- on which a monster
// can stand near tiles off, and a villager side tiles to the monster's side,
// with clear lines him-monster and monster-villager, inside the map. It
// returns the monster's and the villager's points.
func nlPlace(t *testing.T, s *session, from int, px, py, near, side float64) (mx, my, vx, vy float64) {
	t.Helper()

	for i := range nlBearings {
		d := nlBearings[(from+i)%len(nlBearings)]
		mx, my = px+near*d[0], py+near*d[1]
		vx, vy = mx-side*d[1], my+side*d[0]

		if mx < 2 || my < 2 || vx < 2 || vy < 2 || mx > 45 || my > 45 || vx > 45 || vy > 45 {
			continue
		}

		if nlClear(t, s, px, py, mx, my) && nlClear(t, s, mx, my, vx, vy) {
			return mx, my, vx, vy
		}
	}

	t.Fatalf("no bearing from %.1f,%.1f with a monster %.0f off and a villager %.0f to its side, both in clear lines", px, py, near, side)

	return 0, 0, 0, 0
}

// nlLooked steps frames until the watcher's seek row has looked (its phase
// has come), and returns the row.
func nlLooked(t *testing.T, s *session, watcherID string) map[string]any {
	t.Helper()

	for i := 0; i < 60; i++ {
		s.call("strigoi_step", map[string]any{"frames": 1})

		if row := seekRowOf(s, watcherID); row != nil && str(row, "reason") != "pending" {
			return row
		}
	}

	t.Errorf("RED the watcher %s never looked in 60 frames: %v", watcherID, seekState(s))

	return nil
}

func TestTheNearestLiving(t *testing.T) {
	s, playerID, playerHandle, px, py := handsStart(t)
	r0 := mustNum(t, seekState(s), "retargets")

	t.Logf("MEASURE start: him %s at %.2f,%.2f; seek %v", playerID, px, py, seekState(s))

	// --- act 1: two visible quarries, the villager the nearer ------------------
	mx, my, vx, vy := nlPlace(t, s, 0, px, py, 6, 1.5)
	v := spawnNPC(t, s, "fallen1", vx, vy)
	m := spawnNPC(t, s, "zombie1", mx, my)
	vID, mID := entityID(t, s, v), entityID(t, s, m)

	setField(s, "seek", "stand_in", vID)
	s.call("strigoi_watch", map[string]any{"watcher": m, "target": playerHandle})

	n0 := noticeOf(s, mID)
	row := nlLooked(t, s, mID)
	n1 := noticeOf(s, mID)

	t.Logf("MEASURE act1: monster %s at %.1f,%.1f watching him (sees=%v distance=%v reach=%v); villager %s at %.1f,%.1f; row %v; notice %v",
		mID, mx, my, n0["sees"], n0["distance"], n0["reach"], vID, vx, vy, row, n1)
	nlCheck(t, n0 != nil && flag(t, n0, "sees"), "act1 (setup): the monster does not see him: %v", n0)
	nlCheck(t, str(row, "target") == vID && str(row, "reason") == "living",
		"act1: the monster did not take the nearer villager: row %v", row)
	nlCheck(t, str(n1, "quarry") == vID && str(n1, "side") == "hostile", "act1: its watch is not on her: %v", n1)
	nlCheck(t, mustNum(t, seekState(s), "retargets") == r0+1, "act1: retargets %v, want %v", seekState(s)["retargets"], r0+1)

	for i := 0; i < 5 && str(chaseOf(s, mID), "quarry") != vID; i++ {
		afhFrames(s, 1)
	}

	nlCheck(t, str(chaseOf(s, mID), "quarry") == vID, "act1: its chase is not on her: %v", chaseOf(s, mID))

	for i := 0; i < 240 && afhFightFor(s, vID) == nil; i++ {
		afhFrames(s, 1)
	}

	f := afhFightFor(s, vID)
	t.Logf("MEASURE act1: the village's fight %v; his fighting=%v", f, combatState(s)["fighting"])
	nlCheck(t, f != nil, "act1: no clock fight on the villager: %v", afhClock(s))
	nlCheck(t, !flag(t, combatState(s), "fighting"), "act1: a fight on him opened")

	// --- act 2, THE CONTROL: the distances swapped; the choice flips ------------
	r1 := mustNum(t, seekState(s), "retargets")
	mx2, my2, vx2, vy2 := nlPlace(t, s, 2, px, py, 4, 7)
	v2 := spawnNPC(t, s, "fallen1", vx2, vy2)
	m2 := spawnNPC(t, s, "zombie1", mx2, my2)
	v2ID, m2ID := entityID(t, s, v2), entityID(t, s, m2)

	setField(s, "seek", "stand_in", v2ID)
	s.call("strigoi_watch", map[string]any{"watcher": m2, "target": playerHandle})

	row2 := nlLooked(t, s, m2ID)
	t.Logf("MEASURE act2: monster %s at %.1f,%.1f (him 4 off), villager %s at %.1f,%.1f (7 off): row %v; notice %v",
		m2ID, mx2, my2, v2ID, vx2, vy2, row2, noticeOf(s, m2ID))
	nlCheck(t, str(row2, "target") == "player" && str(row2, "reason") == "living",
		"act2 (control): he is the nearer, and the monster did not keep him: %v", row2)
	nlCheck(t, mustNum(t, seekState(s), "retargets") == r1, "act2 (control): a watch moved (%v -> %v)", r1, seekState(s)["retargets"])

	s.call("strigoi_watch", map[string]any{"watcher": m2, "release": true})
	s.call("strigoi_remove_entity", map[string]any{"handle": m2})

	// --- act 3: a villager's own watch never makes a chase or a fight ---------
	wx, wy, xx, xy := nlPlace(t, s, 5, px, py, 5, 1)
	w := spawnNPC(t, s, "fallen1", wx, wy)
	x := spawnNPC(t, s, "zombie1", xx, xy)
	wID, xID := entityID(t, s, w), entityID(t, s, x)

	setField(s, "seek", "stand_in", wID)
	s.call("strigoi_watch", map[string]any{"watcher": w, "target": x, "side": "living"})
	afhFrames(s, 60)

	nw := noticeOf(s, wID)
	living := asList(spawnsState(s)["notice_aware_living"])
	t.Logf("MEASURE act3: villager %s watching monster %s: notice %v; notice_aware_living %v; chase %v; fight on the monster %v",
		wID, xID, nw, living, chaseOf(s, wID), afhFightFor(s, xID))
	nlCheck(t, str(nw, "side") == "living" && flag(t, nw, "noticed"), "act3 (setup): her watch does not see it: %v", nw)
	sawIt := false
	for _, id := range living {
		sawIt = sawIt || id == wID
	}

	nlCheck(t, sawIt, "act3: the villager is not in notice_aware_living: %v", living)
	nlCheck(t, chaseOf(s, wID) == nil, "act3: the villager chases the monster: %v", chaseOf(s, wID))
	nlCheck(t, afhFightFor(s, xID) == nil, "act3: a fight opened on the monster: %v", afhFightFor(s, xID))
	nlCheck(t, seekRowOf(s, wID) == nil, "act3: Seek chose for the village's own side: %v", seekRowOf(s, wID))

	s.call("strigoi_watch", map[string]any{"watcher": w, "release": true})

	// The earlier acts' stand-ins are taken back out of the living before act
	// 4, so a speaker is the only living in the monster's view. (R2's first
	// run, 29 Sep: act 3's stand-in, 4.1 tiles from Charsi, was still a
	// stand-in, and Seek rightly turned act 4's monster onto her -- a slip in
	// the act's setup, not in Seek.)
	for _, id := range []string{vID, v2ID, wID} {
		setField(s, "seek", "stand_in_remove", id)
	}

	// --- act 4 (S0-1 (a)): only a speaker in view -- the target is kept --------
	speakers := afhSpeakers(t, s)
	charsi, ok := speakers["charsi"]

	if !ok {
		t.Fatalf("act4: no charsi on the map")
	}

	cID, cx, cy := str(charsi, "id"), num(charsi, "x"), num(charsi, "y")

	// A short notice radius (a dial, set back at the end), so he can stand
	// out of the monster's reach inside the village.
	setField(s, "spawns", "notice_radius", 4.0)

	var far [2]float64

	for _, p := range [][2]float64{{14.5, 14.5}, {33.5, 14.5}, {14.5, 33.5}, {33.5, 33.5}, {23.5, 14.5}} {
		if math.Hypot(p[0]-cx, p[1]-cy) > 12 {
			far = p

			break
		}
	}

	s.call("strigoi_move_player_to", map[string]any{"x": far[0], "y": far[1], "wait": true, "max_ticks": 3000})

	k := spawnNPC(t, s, "fallen1", cx+1.5, cy)
	kID := entityID(t, s, k)
	s.call("strigoi_watch", map[string]any{"watcher": k, "target": playerHandle})

	for i := 0; i < 3; i++ {
		afhStepWorld(s, 1.0)
	}

	nk, rk := noticeOf(s, kID), seekRowOf(s, kID)
	t.Logf("MEASURE act4: charsi %s at %.1f,%.1f; monster %s beside her watching him: notice %v; row %v; fight on her %v; her body %v",
		cID, cx, cy, kID, nk, rk, afhFightFor(s, cID), afhBody(s, cID))
	nlCheck(t, num(nk, "distance") > num(nk, "reach"), "act4 (setup): he is in the monster's reach: %v", nk)
	nlCheck(t, str(nk, "quarry") == playerID, "act4: the monster's watch left him: %v", nk)
	nlCheck(t, str(rk, "target") == "player", "act4: Seek turned the monster: %v", rk)
	nlCheck(t, afhFightFor(s, cID) == nil, "act4: a fight opened on the speaker")
	nlCheck(t, afhBody(s, cID) == nil, "act4: the speaker lies dead")

	setField(s, "spawns", "notice_radius", 12.0)
}

func TestHisSleepHidesOnlyHim(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Sleeper", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)

	for i := 0; i < 40 && str(clockState(s), "stage") != "night"; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 60.0})
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
	}

	if str(clockState(s), "stage") != "night" {
		t.Fatalf("no night: %v", clockState(s))
	}

	setField(s, "meters", "fatigue", 80.0)
	setField(s, "village", "rep", 40.0) // shelter

	headman := villager(t, s, "Warriv")
	walkNear(t, s, headman)
	openTalkWith(t, s, headman)

	if str(villageState(s), "node") == "headman_first" {
		answer(t, s, 3) // say nothing, and go -- met
		openTalkWith(t, s, headman)
	}

	answer(t, s, answerIndex(t, s, "sleep inside"))

	// The night's cast, placed with the sleep's answer on screen -- no frame
	// runs between here and the sleep: a monster four tiles off watching him,
	// and a stand-in villager six tiles to its side. Awake, he is the nearer.
	p := s.call("strigoi_get_player", map[string]any{})
	px, py, playerHandle := num(p, "x"), num(p, "y"), str(p, "handle")
	hp0 := mustNum(t, metersState(s), "health")

	mx, my, vx, vy := nlPlace(t, s, 0, px, py, 4, 6)
	v := spawnNPC(t, s, "fallen1", vx, vy)
	m := spawnNPC(t, s, "zombie1", mx, my)
	vID, mID := entityID(t, s, v), entityID(t, s, m)

	setField(s, "seek", "stand_in", vID)
	s.call("strigoi_watch", map[string]any{"watcher": m, "target": playerHandle})

	n0 := noticeOf(s, mID)
	before, r0, c0 := worldMinutes(t, s), mustNum(t, seekState(s), "retargets"), mustNum(t, afhClock(s), "started")

	answer(t, s, 1) // sleep four hours

	slept := worldMinutes(t, s) - before
	r1, c1 := mustNum(t, seekState(s), "retargets"), mustNum(t, afhClock(s), "started")
	nm, ch := noticeOf(s, mID), chaseOf(s, mID)

	// NOTHING WALKS DURING A SLEEP (measured at R2's first run, 29 Sep): the
	// sleep is spendMinutes' ten-minute steps of advanceWorld, and an entity
	// moves only in MapEngine.Advance's frames, so the monster ends the four
	// hours exactly where it began, six tiles from her, and under the
	// launcher's policy a fight opens only at adjacency (declined_reach counts
	// each step). So the script reads what the sleep CAN show of the night
	// coming for her -- its watch AND its chase moved onto her -- and logs the
	// clock fights (0 -> 0 at that run) rather than asserting one opened: the
	// first build's "clock started rises" read the engine wrongly (the R2
	// build note, "Deviations").
	t.Logf("MEASURE sleep: monster %s at %.1f,%.1f watching him (at the watch: %v); villager %s at %.1f,%.1f; slept %.2f; retargets %.0f -> %.0f; clock fights %.0f -> %.0f; the monster's notice after %v; its chase %v; its row %v; clock %v",
		mID, mx, my, n0, vID, vx, vy, slept, r0, r1, c0, c1, nm, ch, seekRowOf(s, mID), afhClock(s))
	nlCheck(t, n0 != nil && flag(t, n0, "sees"), "(setup): at the watch the monster sees him: %v", n0)
	nlCheck(t, math.Abs(slept-240) <= 0.5, "his sleep stopped after %.2f of 240 minutes", slept)
	nlCheck(t, r1 > r0, "no hostile turned to the villager while he slept (retargets %.0f -> %.0f): his sleep hid her too", r0, r1)
	nlCheck(t, str(ch, "quarry") == vID, "the monster's chase after the sleep is not on the villager: %v -- the night did not come for her", ch)
	nlCheck(t, nm != nil && str(nm, "quarry") == vID, "the monster's watch after the sleep is not on the villager: %v", nm)
	nlCheck(t, !flag(t, combatState(s), "fighting"), "he is in a fight after his sleep")
	nlCheck(t, mustNum(t, metersState(s), "health") >= hp0, "something struck him while he slept (%.0f -> %v)", hp0, metersState(s)["health"])
}
