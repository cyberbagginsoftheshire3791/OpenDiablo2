//go:build playtest

package playtest

import (
	"math"
	"testing"
)

// TestWatch is T8 (23 Sep 2026): the watch is STOOD, not survived.
//
//  1. THE CONTROL: he promises the watch by day, spends the night away from
//     the headman's post, and is alive at dawn -- the promise is broken and
//     costs standing (under T4 it paid).
//  2. He promises again and spends the night at the post: he stands in the
//     WATCH stance there, the minutes count, and dawn pays the watch.
//  3. BUG-17: he promises a third time and stands the watch into the small
//     hours, then takes three hours at the smith's anvil across dawn -- from
//     the night into full day between two frames. The watch he stood is still
//     settled and paid, and so is the night's experience. (The smith mends only
//     worn mail, so a fight by day wears it first.)
func TestWatch(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Watchman", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	// M4.7 step 3: Night 1's dead stay down -- this script's subject is the watch,
	// and a risen man coming for him would be a different script.
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	setField(s, "village", "rep", 30.0)

	home := s.call("strigoi_get_player", map[string]any{})
	headman := villager(t, s, "Warriv")
	walkNear(t, s, headman)

	promise := func(act string) {
		t.Helper()

		openTalkWith(t, s, headman)

		if str(villageState(s), "node") == "headman_first" {
			answer(t, s, 3) // say nothing, and go -- met
			openTalkWith(t, s, headman)
		}

		answer(t, s, answerIndex(t, s, "stand the watch"))
		answer(t, s, 1) // promise

		if !hasFlag(t, s, "watch_promised") {
			t.Fatalf("%s: the promise is made: %v", act, villageState(s))
		}
	}

	// --- 1: promised, and away all night (the control) ---------------------------
	promise("act 1")
	walkFrom(t, s, headman, 17) // the post is 15 tiles; the camp's walls allow ~20

	rep0 := mustNum(t, villageState(s), "rep")
	_, stood := throughTheNight(t, s)

	if stood != 0 {
		t.Fatalf("act 1: away from the post, no minute counts: %.1f", stood)
	}

	v := villageState(s)
	if hasFlag(t, s, "watch_promised") || mustNum(t, v, "rep") != rep0-3 {
		t.Fatalf("act 1: alive at dawn but never at the ditch -- the promise is broken, -3: %.0f -> %.0f (%v)", rep0, mustNum(t, v, "rep"), v)
	}

	// --- 2: promised, and at the post --------------------------------------------------
	walkNear(t, s, headman)
	promise("act 2")

	// T9: the kit panel shows the promise and its minutes.
	if !kitSays(t, s, "Watch: 0/180 min") {
		t.Fatal("act 2: the kit panel's status line shows the promised watch")
	}

	rep1 := mustNum(t, villageState(s), "rep")
	sawWatch, stood := throughTheNight(t, s)

	if !sawWatch {
		t.Fatal("act 2: at the post by night he stands in the WATCH stance")
	}

	if stood < 180 {
		t.Fatalf("act 2: a night at the post counts at least the 180 minutes needed: %.1f", stood)
	}

	if after := mustNum(t, villageState(s), "watch_stood"); after != 0 {
		t.Fatalf("act 2: dawn spends the night's minutes: %.1f left", after)
	}

	if got := mustNum(t, villageState(s), "rep"); got != rep1+8 {
		t.Fatalf("act 2: a watch stood pays 8 at dawn: %.0f -> %.0f", rep1, got)
	}

	// --- 3: BUG-17 -- a labour from the deep night into full day ---------------------
	// The fight that wears his mail, on open ground where he started (beside the
	// forge a beast spawns against its walls and the fight disengages).
	s.call("strigoi_move_player_to", map[string]any{"x": num(home, "x"), "y": num(home, "y")})

	for i := 0; i < 80; i++ {
		p := s.call("strigoi_get_player", map[string]any{})
		if math.Hypot(num(p, "x")-num(home, "x"), num(p, "y")-num(home, "y")) < 1 {
			break
		}

		s.call("strigoi_step", map[string]any{"frames": 6})
	}

	fightForBlows(t, s)
	setField(s, "combat", "player_action", "attack")

	for i := 0; i < 120 && flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 12})
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatal("act 3: the fight never ended")
	}

	if worn := mailPoints(t, s); worn >= 14 {
		t.Fatalf("act 3: the fight must wear his mail, or the smith has no work for him: %d", worn)
	}

	// The forge's rung, and the row's reproduction ("standing 38 or more").
	setField(s, "village", "rep", 40.0)
	walkNear(t, s, headman)
	promise("act 3")

	// The dead are not this act's subject: two dawns with Night 1's four bodies
	// open have raised soul pressure, which adds to the rising odds this script
	// set to 0 -- measured 27 Sep, a risen man came for him at 21:35 of this
	// third night and killed him at 22:42, 87 minutes into the watch.
	setField(s, "rising", "pressure", 0.0)

	rep2 := mustNum(t, villageState(s), "rep")
	stood = untilTheSmallHours(t, s)

	t.Logf("act 3: %.1f minutes stood by %s", stood, str(clockState(s), "time_of_day"))

	if stood < 180 {
		t.Fatalf("act 3: by the small hours he has stood the 180 minutes: %.1f", stood)
	}

	smith := villager(t, s, "Charsi")
	walkNear(t, s, smith)
	openTalkWith(t, s, smith)

	// THE PRECONDITION: the work must begin in the night and end past 04:15,
	// so no frame between can see dawn. 01:15 < start < 02:45.
	c := clockState(s)
	if m := mustNum(t, c, "minute_of_day"); str(c, "stage") != "night" || m <= 75 || m >= 165 {
		t.Fatalf("act 3: the anvil must be taken between 01:15 and 02:45 to cross dawn: %s (%s)",
			str(c, "time_of_day"), str(c, "stage"))
	}

	from := str(c, "time_of_day")
	xp := mustNum(t, progressState(s), "xp")

	answer(t, s, answerIndex(t, s, "mends your mail"))

	if st := str(clockState(s), "stage"); st != "day" {
		t.Fatalf("act 3: three hours from %s end in full day: %s at %s", from, st, str(clockState(s), "time_of_day"))
	}

	v = villageState(s)
	if hasFlag(t, s, "watch_promised") || mustNum(t, v, "watch_stood") != 0 || mustNum(t, v, "rep") != rep2+8 {
		t.Fatalf("act 3: the night he worked across still settles the watch he stood, +8: rep %.0f -> %.0f, %v",
			rep2, mustNum(t, v, "rep"), v)
	}

	if got := mustNum(t, progressState(s), "xp"); got != xp+50 {
		t.Fatalf("act 3: and pays the night's 50 experience: %.0f -> %.0f", xp, got)
	}

	t.Logf("broken watch -3, stood watch +8; stood %.0f minutes, then the anvil from %s to %s: +8 and +50 experience",
		stood, from, str(clockState(s), "time_of_day"))
}

// untilTheSmallHours steps the world, feeding him, to between 01:20 and 01:50
// of the coming night -- late enough that three hours' work ends past 04:15,
// early enough to walk somewhere first -- and reports the watch minutes stood.
func untilTheSmallHours(t *testing.T, s *session) float64 {
	t.Helper()

	for i := 0; i < 150; i++ {
		c := clockState(s)
		if m := mustNum(t, c, "minute_of_day"); str(c, "stage") == "night" && m >= 80 && m < 110 {
			return mustNum(t, villageState(s), "watch_stood")
		}

		s.call("strigoi_step_world", map[string]any{"world_minutes": 20.0})
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
	}

	t.Fatal("the small hours never came")

	return 0
}

// throughTheNight steps the world to the next dawn, feeding him as it goes,
// and reports whether he was ever seen in the watch stance.
func throughTheNight(t *testing.T, s *session) (bool, float64) {
	t.Helper()

	stage := func() string {
		return str(sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state"), "stage")
	}

	sawWatch, wasNight, stood := false, false, 0.0

	for i := 0; i < 120; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 20.0})
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)

		if str(metersState(s), "activity") == "watch" {
			sawWatch = true
		}

		if st := mustNum(t, villageState(s), "watch_stood"); st > stood {
			stood = st
		}

		st := stage()
		if st == "night" {
			wasNight = true
		}

		if wasNight && st == "dawn" {
			s.call("strigoi_step", map[string]any{"frames": 2})

			return sawWatch, stood
		}
	}

	t.Fatal("the night never ended")

	return false, 0
}

func hasFlag(t *testing.T, s *session, flag string) bool {
	t.Helper()

	for _, f := range asList(villageState(s)["flags"]) {
		if f.(string) == flag {
			return true
		}
	}

	return false
}

// walkFrom walks him at least min tiles from a villager, anywhere.
func walkFrom(t *testing.T, s *session, handle string, min float64) {
	t.Helper()

	e := s.call("strigoi_get_entity", map[string]any{"handle": handle})
	d := min + 7

	for _, off := range [][2]float64{{d, -d}, {d, 0}, {d, d}, {0, d}, {-d, 0}, {0, -d}, {-d, -d}, {-d, d}} {
		s.call("strigoi_move_player_to", map[string]any{"x": num(e, "x") + off[0], "y": num(e, "y") + off[1]})

		for i := 0; i < 120 && distTo(t, s, handle) < min; i++ {
			s.call("strigoi_step", map[string]any{"frames": 6})
		}

		if distTo(t, s, handle) >= min {
			return
		}
	}

	t.Fatalf("could not walk %.0f tiles from %s", min, handle)
}
