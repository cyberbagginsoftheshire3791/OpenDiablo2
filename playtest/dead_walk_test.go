//go:build playtest

package playtest

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTheDeadWalk is M4.7 step 3 (23 Sep 2026): the risen stand up in the
// world, come for him, and break off at first light.
//
//  1. With the odds certain and him well off (and nothing able to notice him:
//     the notice model is not this script's subject), the deep night's first
//     band stands Night 1's four dead up where they lay: four bodies risen,
//     four groups of the risen row on the map -- and no fight (the control
//     for the fight in act 3).
//  2. They are still standing at a quarter past two: the dead do not leave
//     before first light.
//  3. The notice radius restored, they find him and come: a fight opens in
//     which every enemy is the risen row, speed 0 (last in every round), and
//     he is not surprised.
//  4. He holds; the rounds carry the clock past first light, and the fight
//     ends "dawn". Each risen lies down where he stood: four open bodies of
//     men again, and no risen group on the map.
//  5. At odds 0 the four Downed stand certainly the next deep night.
//  6. In the third band a fifth, nameless, stands up at the edge of the night
//     with a body. Before the tale the hover calls him "A stranger" -- and
//     one of the four, standing a second time, still "A fallen soldier".
func TestTheDeadWalk(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Vigil", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 1.0)
	setField(s, "rising", "edge_floor", 0) // until act 6: the four are the subject

	health := mustNum(t, metersState(s), "health")
	keepAlive := func() {
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
		setField(s, "meters", "health", health)
	}

	c := corpsesState(s)
	if mustNum(t, c, "fresh_human") != 4 {
		t.Fatalf("four of Night 1's dead: %v", c)
	}

	body := asList(c["bodies"])[0].(map[string]any)
	bx, by := num(body, "x"), num(body, "y")

	// --- 1: they rise, and nothing has found him ------------------------------------------------
	// The notice radius is taken down until act 3 so the distance is not what
	// the control rests on.
	noticeRadius := mustNum(t, spawnsState(s), "notice_radius")
	setField(s, "spawns", "notice_radius", 0.05)
	walkAwayFrom(t, s, bx, by, 8)

	for i := 0; i < 400 && mustNum(t, risingState(s), "rolls") < 1; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		keepAlive()
	}

	c = corpsesState(s)
	if mustNum(t, c, "risen_human") != 4 {
		t.Fatalf("act 1: at odds 1 the first band stands all four up: %v", c)
	}

	if got := risenGroups(s); got != 4 {
		t.Fatalf("act 1: four of the risen row on the map, got %d", got)
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatal("act 1: nothing has found him yet")
	}

	// Act 1's walk leaves him wherever the map let him go. On the village
	// (the suite's game since 26 Sep) that is outside the east fence, and the
	// one risen man inside the notice radius there is behind a house: the
	// village is shelter (Josh, 24 Sep), and sight is not this script's
	// subject. So he walks back to where the first body's place can see him
	// while the radius is still too small for anything to notice him.
	standInSightOf(t, s, bx, by, 8)

	// --- 2: still standing before first light -----------------------------------------------------
	for i := 0; i < 400; i++ {
		if m := mustNum(t, clockState(s), "minute_of_day"); m >= 2*60+10 && m < 2*60+45 {
			break
		}

		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		keepAlive()
	}

	if st := str(clockState(s), "stage"); st != "night" {
		t.Fatalf("act 2: wanted the last of the night, got %q", st)
	}

	if got := risenGroups(s); got != 4 {
		t.Fatalf("act 2: the dead do not leave before first light: %d risen groups", got)
	}

	// --- 3: he walks back, and they come -----------------------------------------------------------
	setField(s, "spawns", "notice_radius", noticeRadius)
	setField(s, "combat", "player_action", "hold")
	setField(s, "combat", "round_minutes", 5.0)

	// He stands where he is: a walk still in flight when the fight opens
	// carries him out of it (measured: "disengaged" one round in). The dead
	// come to him.
	for i := 0; i < 200 && !flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 6})
	}

	fight := combatState(s)
	if !flag(t, fight, "fighting") {
		p := s.call("strigoi_get_player", map[string]any{})
		t.Fatalf("act 3: at the bodies' place, the dead find him: %v\nplayer at %.1f,%.1f; notice_list=%v",
			fight, num(p, "x"), num(p, "y"), spawnsState(s)["notice_list"])
	}

	if flag(t, fight, "surprised") {
		t.Fatalf("act 3: the dead take neither surprise branch: %v", fight)
	}

	enemies := 0

	for _, raw := range asList(fight["participants"]) {
		row := raw.(map[string]any)
		if str(row, "side") != "enemy" {
			continue
		}

		enemies++

		if str(row, "profile") != "risen" || num(row, "speed") != 0 {
			t.Fatalf("act 3: every enemy is the risen row at speed 0: %v", row)
		}

		drawnAsStrigoi(t, s, row)
	}

	if enemies == 0 {
		t.Fatalf("act 3: a fight with no enemy in it: %v", fight)
	}

	t.Logf("act 3: the fight opened at %s with %d of the dead: %v", str(clockState(s), "time_of_day"), enemies, fight["participants"])

	// --- 4: first light ------------------------------------------------------------------------------
	for i := 0; i < 400 && flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 12})
		keepAlive()
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatalf("act 4: the fight outlasted the night: %v", clockState(s))
	}

	if why := str(combatState(s), "ended_reason"); why != "dawn" {
		t.Fatalf("act 4: the dead break off at first light: ended %q at %s (%s); combat %v",
			why, str(clockState(s), "time_of_day"), str(clockState(s), "stage"), combatState(s))
	}

	s.call("strigoi_step", map[string]any{"frames": 2})

	c = corpsesState(s)
	if mustNum(t, c, "downed_human") != 4 {
		t.Fatalf("act 4: each risen lies down where he stood, Downed: %v", c)
	}

	if _, ok := c["risen_human"]; ok {
		t.Fatalf("act 4: none left standing: %v", c)
	}

	if got := risenGroups(s); got != 0 {
		t.Fatalf("act 4: no risen group on the map: %d", got)
	}

	if got := mustNum(t, combatState(s), "ended_dawn"); got != 1 {
		t.Fatalf("act 4: one fight left at first light: %.0f", got)
	}

	// --- 5: Q7a -- the Downed stand certainly the next night ------------------------------------
	// THE CONTROL IS THE ODDS: at p 0 no open body could rise, so four risen
	// at the first band are the Downed standing, not the roll.
	setField(s, "rising", "p", 0.0)
	setField(s, "spawns", "notice_radius", 0.05)
	walkAwayFrom(t, s, bx, by, 8)

	for i := 0; i < 400 && risenGroups(s) == 0; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		keepAlive()
	}

	if got := risenGroups(s); got != 4 || mustNum(t, corpsesState(s), "risen_human") != 4 {
		t.Fatalf("act 5: at odds 0 the four Downed stand in the next deep night: %d groups %v", got, corpsesState(s))
	}

	// --- 6: the edge floor -----------------------------------------------------------------------
	// Still at odds 0: in the third band one nameless dead man stands up at
	// the edge of the night anyway (S1 §6.3), and he has a body.
	setField(s, "rising", "edge_floor", 1)

	for i := 0; i < 400 && mustNum(t, risingState(s), "wandered") < 1; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		keepAlive()
	}

	if got := risenGroups(s); got != 5 || mustNum(t, corpsesState(s), "total") != 5 {
		t.Fatalf("act 6: a fifth, nameless, at the edge of the third band: %d groups %v", got, corpsesState(s))
	}

	// Before the priest's tale he is called what he was: a man he cannot know.
	// And one of Night 1's dead, standing a second time as a new member, is
	// still the comrade his body was (Corpses.BodyOf).
	wanderer, again := bodyNamed(t, s, "wanderer:1"), bodyNamed(t, s, str(body, "id"))
	if got := str(wanderer, "was"); got != "A stranger" {
		t.Fatalf("act 6: the wanderer's body says he was %q, want \"A stranger\": %v", got, wanderer)
	}

	if got := hoverOn(t, s, str(wanderer, "walks_as")); got != "A stranger" {
		t.Fatalf("act 6: before the tale the wanderer is called \"A stranger\": hover %q", got)
	}

	if got := hoverOn(t, s, str(again, "walks_as")); got != "A fallen soldier" {
		t.Fatalf("act 6: %s, standing again, is still called \"A fallen soldier\": hover %q", str(again, "id"), got)
	}

	t.Logf("four rose, came for him at 02:15, lay down Downed at first light (%d in the fight), stood again the next night at odds 0, and a fifth came from the edge", enemies)
}

// drawnAsStrigoi is M5.1b: a risen man in the fight is drawn from the
// bestiary's strigoi -- the sheet the game reports for him is one of
// data/strigoi/creatures/strigoi's, and the game loads it as 96-pixel frames in
// eight directions -- and he fights at the numbers the dead had before the art
// changed: the men's max health, 84.
func drawnAsStrigoi(t *testing.T, s *session, participant map[string]any) {
	t.Helper()

	e := s.call("strigoi_get_entity", map[string]any{"handle": handleFor(t, s, str(participant, "id"))})
	sheet := str(sub(e, "state"), "sheet")

	t.Logf("act 3: risen %s drawn from %q, max_health %v", str(participant, "id"), sheet, participant["max_health"])

	if !strings.HasPrefix(sheet, "/data/strigoi/creatures/strigoi/") {
		t.Fatalf("act 3: a risen man is drawn from %q, not the strigoi's sheets: %v", sheet, e)
	}

	if got := mustNum(t, participant, "max_health"); got != 84 {
		t.Fatalf("act 3: a risen man keeps the men's max health 84: %v", participant)
	}

	// Review B2 (27 Sep 2026): he walks at the bestiary strigoi's speed. The
	// natural-spawn path (gameSpawner.Spawn -> Entry.SpeedOr) had no test, and
	// the authored 5 equals the stand-in's, so a revert to the stand-in's
	// speed changed nothing a script saw -- hence the number is read from the
	// data, and the control authored 6.
	if got, want := mustNum(t, sub(e, "state"), "speed"), bestiarySpeed(t, "strigoi"); got != want {
		t.Fatalf("act 3: a risen man walks at %v, want the bestiary strigoi's %v: %v", got, want, e)
	}

	sprite := s.call("strigoi_describe_sprite", map[string]any{"path": sheet})
	if num(sprite, "directions") != 8 || num(sprite, "max_w") != 96 || num(sprite, "max_h") != 96 {
		t.Fatalf("act 3: the strigoi's %s is not the 8-direction 96-pixel sheet: %v", sheet, sprite)
	}
}

// risenGroups counts the risen row's groups on the map.
func risenGroups(s *session) int {
	n := 0

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		if g, ok := raw.(map[string]any); ok && str(g, "row") == "risen" {
			n++
		}
	}

	return n
}

// standInSightOf walks the player toward (x, y) until he is within near tiles
// AND the sight ray FROM (x, y) to him is clear -- the direction the notice
// model casts, watcher to target -- then stops him where he stands. There is
// no stop verb: re-targeting his own tile is the stop, and the check after it
// proves he stopped rather than assuming it.
func standInSightOf(t *testing.T, s *session, x, y, near float64) {
	t.Helper()

	// find_path's from_x/from_y are omitempty: a zero would silently cast the
	// ray from the player to himself, which is always clear.
	if x == 0 || y == 0 {
		t.Fatalf("standInSightOf(%.1f,%.1f): a zero coordinate cannot be sent as the ray's origin", x, y)
	}

	s.call("strigoi_move_player_to", map[string]any{"x": x, "y": y})

	for i := 0; i < 300; i++ {
		p := s.call("strigoi_get_player", map[string]any{})
		px, py := mustNum(t, p, "x"), mustNum(t, p, "y")

		if math.Hypot(px-x, py-y) <= near &&
			flag(t, s.call("strigoi_find_path", map[string]any{"from_x": x, "from_y": y, "to_x": px, "to_y": py}), "straight_line_clear") {
			s.call("strigoi_move_player_to", map[string]any{"x": px, "y": py})
			s.call("strigoi_step", map[string]any{"frames": 30})

			a := s.call("strigoi_get_player", map[string]any{})
			s.call("strigoi_step", map[string]any{"frames": 30})
			b := s.call("strigoi_get_player", map[string]any{})

			if math.Hypot(mustNum(t, b, "x")-mustNum(t, a, "x"), mustNum(t, b, "y")-mustNum(t, a, "y")) > 0.05 {
				t.Fatalf("stood in sight of %.1f,%.1f at %.1f,%.1f but he is still walking (%.2f,%.2f -> %.2f,%.2f)",
					x, y, px, py, num(a, "x"), num(a, "y"), num(b, "x"), num(b, "y"))
			}

			t.Logf("stands at %.1f,%.1f (the line was cleared at %.1f,%.1f), %.1f tiles from %.1f,%.1f", num(b, "x"), num(b, "y"),
				px, py, math.Hypot(num(b, "x")-x, num(b, "y")-y), x, y)

			return
		}

		s.call("strigoi_step", map[string]any{"frames": 6})
	}

	t.Fatalf("walked toward %.1f,%.1f and never stood within %.0f tiles on a clear line", x, y, near)
}

// walkAwayFrom walks him at least min tiles from a point, in whichever
// direction the map allows.
func walkAwayFrom(t *testing.T, s *session, x, y, min float64) {
	t.Helper()

	d := min + 4

	for _, off := range [][2]float64{{d, 0}, {-d, 0}, {0, d}, {0, -d}, {d, d}, {-d, -d}, {d, -d}, {-d, d}} {
		s.call("strigoi_move_player_to", map[string]any{"x": x + off[0], "y": y + off[1]})

		for i := 0; i < 120; i++ {
			p := s.call("strigoi_get_player", map[string]any{})
			if math.Hypot(num(p, "x")-x, num(p, "y")-y) >= min {
				return
			}

			s.call("strigoi_step", map[string]any{"frames": 6})
		}
	}

	t.Fatalf("could not walk %.0f tiles from %.1f,%.1f", min, x, y)
}

// TestASlainManRises is the 27 Sep 2026 ruling's third case and review C1: a
// man he killed rises where he lay.
//
//  1. Dusk, and the men come: at certainty the tables send the men's row
//     (fourth in declaration order, so three groups are let in), and every
//     other arrival is sent home.
//  2. He kills one of them: a man's open body of the men's row, and the body
//     says what he was, "Opportunist".
//  3. THE CONTROL for act 4: the slain man's remains lie on the map -- the
//     entity list can see them.
//  4. At odds 1 the next deep-night band stands the body up as a new member,
//     and the slain man's remains are gone: not lying under the man who stood
//     up from them (C1: they were left when the body had never stood before).
//  5. Before the priest's tale the hover calls the risen man what he was,
//     "Opportunist" -- never the dead's own name.
//  6. And so does the combat log. At the shipped human control (the combat
//     panel is drawn only in a paced fight) the dead are let notice him; he
//     ends each turn and the risen man strikes. The log calls him
//     "Opportunist", and no blow line names the dead before the tale.
func TestASlainManRises(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Reaper", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)

	health := mustNum(t, metersState(s), "health")
	keepAlive := func() {
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
		setField(s, "meters", "health", health)
	}

	// --- 1: dusk, and the men come ----------------------------------------------------------------
	for i := 0; i < 80 && str(clockState(s), "stage") != "dusk"; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 20.0})
		keepAlive()
	}

	if st := str(clockState(s), "stage"); st != "dusk" {
		t.Fatalf("act 1: never reached dusk: %s at %s", st, str(clockState(s), "time_of_day"))
	}

	setField(s, "spawns", "max_groups", 3)
	setField(s, "spawns", "chance", 100)

	menID := ""

	for i := 0; i < 12 && menID == ""; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		keepAlive()

		for _, raw := range asList(spawnsState(s)["group_list"]) {
			if g, ok := raw.(map[string]any); ok && str(g, "row") == "opportunists" {
				menID = str(g, "group")
			}
		}
	}

	setField(s, "spawns", "chance", 0)

	if menID == "" {
		t.Fatalf("act 1: at certainty the men never came at dusk: %v", spawnsState(s)["group_list"])
	}

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		if g, ok := raw.(map[string]any); ok && str(g, "group") != menID {
			setField(s, "spawns", "despawn", str(g, "group"))
		}
	}

	// --- 2: he kills one ------------------------------------------------------------------------------
	// The way TestCombatRout brings a real pack to a real fight: seen by
	// torchlight from far off, and every blow a graze of one point while he
	// walks to them and holds -- then crits, and he strikes.
	setField(s, "spawns", "notice_radius", 24)
	setField(s, "light", "carried_source", "torch")
	setField(s, "combat", "player_action", "hold")
	setField(s, "combat", "forced_band", "graze")
	setField(s, "combat", "graze_factor", 0.01)

	_, packID, _ := walkToAPack(t, s, 1)
	if packID != menID {
		t.Fatalf("act 2: the fight is with %s, not the men (%s)", packID, menID)
	}

	keepAlive()
	setField(s, "combat", "forced_band", "crit")
	setField(s, "combat", "player_action", "attack")

	for i := 0; i < 400 && flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 4})
		keepAlive()
	}

	if flag(t, combatState(s), "fighting") {
		t.Fatalf("act 2: the fight never ended: %v", combatState(s))
	}

	setField(s, "spawns", "notice_radius", 0.05) // the dead stand; they never come
	setField(s, "combat", "forced_band", "")

	var slain map[string]any

	for _, raw := range asList(corpsesState(s)["bodies"]) {
		if b, ok := raw.(map[string]any); ok && str(b, "row") == "opportunists" && str(b, "state") == "fresh" {
			slain = b

			break
		}
	}

	if slain == nil {
		t.Fatalf("act 2: he killed none of the men: %v", corpsesState(s))
	}

	if str(slain, "class") != "human" || str(slain, "was") != "Opportunist" {
		t.Fatalf("act 2: a man's body, and it says he was an Opportunist: %v", slain)
	}

	id := str(slain, "id")

	// --- 3: the control -- his remains lie there ---------------------------------------------------
	if !onTheMap(s, id) {
		t.Fatalf("act 3: the slain man's remains are not in the entity list, so their absence would prove nothing")
	}

	// --- 4: he stands up, and nothing lies under him ------------------------------------------------
	setField(s, "rising", "p", 1.0)

	rolls := mustNum(t, risingState(s), "rolls")
	for i := 0; i < 60 && mustNum(t, risingState(s), "rolls") == rolls; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 10.0})
		keepAlive()
	}

	s.call("strigoi_step", map[string]any{"frames": 2})

	risen := bodyNamed(t, s, id)
	member := str(risen, "walks_as")

	t.Logf("act 4: %s rose at %s as %s: %v", id, str(clockState(s), "time_of_day"), member, risen)

	if str(risen, "state") != "risen" || member == "" || member == id {
		t.Fatalf("act 4: at odds 1 the slain man's body stands up as a new member: %v", risen)
	}

	if !onTheMap(s, member) {
		t.Fatalf("act 4: the risen man %s is not on the map", member)
	}

	if onTheMap(s, id) {
		t.Fatalf("act 4 (C1): the slain man's remains %s still lie on the map under the man who stood up from them", id)
	}

	// --- 5: before the tale, what he was ---------------------------------------------------------------
	if got := hoverOn(t, s, member); got != "Opportunist" {
		t.Fatalf("act 5: before the tale a slain opportunist risen is called \"Opportunist\": hover %q", got)
	}

	// --- 6: the combat log calls him the same --------------------------------------------------------
	// The launcher drops every script to policy; the shipped screen is human,
	// and only a paced fight draws the panel the log is on.
	setField(s, "combat", "player_control", "human")
	setField(s, "spawns", "notice_radius", 24)

	for i := 0; i < 300 && !flag(t, combatState(s), "fighting"); i++ {
		s.call("strigoi_step", map[string]any{"frames": 4})
		keepAlive()
	}

	if !flag(t, combatState(s), "fighting") {
		t.Fatalf("act 6: the risen man never came for him: %v", combatState(s))
	}

	blows, named, dead := map[string]bool{}, 0, bestiaryName(t, "strigoi")

	for round := 0; round < 12 && named == 0 && flag(t, combatState(s), "fighting"); round++ {
		openTurn(t, s)
		keepAlive()

		for _, line := range blowLines(s) {
			blows[line] = true

			if strings.Contains(line, "Opportunist") {
				named++
			}
		}

		s.call("strigoi_key", map[string]any{"key": "e"})
		s.call("strigoi_step", map[string]any{"frames": 2})
	}

	t.Logf("act 6: %d distinct blow lines: %v", len(blows), blows)

	for line := range blows {
		if strings.Contains(line, dead) {
			t.Fatalf("act 6: before the tale the combat log names one of the dead %q: %q", dead, line)
		}
	}

	if named == 0 {
		t.Fatalf("act 6: the combat log never called the risen man \"Opportunist\" as the hover does: %v", blows)
	}

	t.Logf("slain at dusk, risen at %s where he lay, his remains taken off, and called what he was on the hover and in the log",
		str(clockState(s), "time_of_day"))
}

// onTheMap reports an entity id in the map's entity list.
func onTheMap(s *session, id string) bool {
	for _, raw := range asList(s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 500})["items"]) {
		if row, ok := raw.(map[string]any); ok && str(row, "id") == id {
			return true
		}
	}

	return false
}

// bodyNamed is one body of the corpses provider, by id.
func bodyNamed(t *testing.T, s *session, id string) map[string]any {
	t.Helper()

	for _, raw := range asList(corpsesState(s)["bodies"]) {
		if b, ok := raw.(map[string]any); ok && str(b, "id") == id {
			return b
		}
	}

	t.Fatalf("no body %q: %v", id, corpsesState(s))

	return nil
}

// hoverOn walks him near one entity, puts the cursor on it and reads the
// hover's label: what the game calls that entity, and no other.
func hoverOn(t *testing.T, s *session, id string) string {
	t.Helper()

	if id == "" {
		t.Fatal("hoverOn: no entity id")
	}

	handle := handleFor(t, s, id)
	e := s.call("strigoi_get_entity", map[string]any{"handle": handle})
	walkWithin(t, s, num(e, "x"), num(e, "y"), 3)

	sx, sy := screenOf(t, s, handle)
	if sx < 0 || sy < 0 || sx >= 800 || sy >= 600 {
		t.Fatalf("hoverOn %s: off the screen at %d,%d (%v)", id, sx, sy, e)
	}

	s.call("strigoi_move_cursor", map[string]any{"x": sx, "y": sy})
	s.call("strigoi_step", map[string]any{"frames": 2})

	return str(uiState(s), "hover_label")
}

// walkWithin walks him to within near tiles of a point, in waited slices (he
// walks about two tiles a second, so a far wanderer is several seconds off),
// trying the point's four diagonal neighbours in turn, and fails if he never
// gets there.
func walkWithin(t *testing.T, s *session, x, y, near float64) {
	t.Helper()

	// Re-targeting his own tile is the stop: a move outlives the slice that
	// issued it, and the cursor must not chase a camera still sliding.
	stop := func(p map[string]any) {
		s.call("strigoi_move_player_to", map[string]any{"x": num(p, "x"), "y": num(p, "y")})
		s.call("strigoi_step", map[string]any{"frames": 12})
	}

	for _, off := range [][2]float64{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}} {
		for slice := 0; slice < 20; slice++ {
			p := s.call("strigoi_get_player", map[string]any{})
			if math.Hypot(num(p, "x")-x, num(p, "y")-y) <= near {
				stop(p)

				return
			}

			move := s.call("strigoi_move_player_to", map[string]any{
				"x": x + off[0], "y": y + off[1], "wait": true, "max_ticks": 150,
			})

			if str(move, "outcome") == "arrived" {
				break
			}
		}

		p := s.call("strigoi_get_player", map[string]any{})
		if math.Hypot(num(p, "x")-x, num(p, "y")-y) <= near {
			stop(p)

			return
		}
	}

	p := s.call("strigoi_get_player", map[string]any{})
	t.Fatalf("could not walk within %.0f tiles of %.1f,%.1f: he stands at %.1f,%.1f", near, x, y, num(p, "x"), num(p, "y"))
}

// blowPattern is a blow line of the combat log: "who verb whom: damage".
var blowPattern = regexp.MustCompile(`: \d+`)

// blowLines are the combat panel's blow lines now (the combat log).
func blowLines(s *session) []string {
	var out []string

	for _, raw := range asList(uiState(s)["tactical_lines"]) {
		if line, ok := raw.(string); ok && blowPattern.MatchString(line) {
			out = append(out, line)
		}
	}

	return out
}

// bestiaryCreature is one creature of the bestiary on disk -- the data the
// game under test was given (the launcher copies it beside the binary).
func bestiaryCreature(t *testing.T, id string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "data", "strigoi", "bestiary.json"))
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Creatures []map[string]any `json:"creatures"`
	}

	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}

	for _, c := range doc.Creatures {
		if c["id"] == id {
			return c
		}
	}

	t.Fatalf("no creature %q in the bestiary", id)

	return nil
}

// bestiarySpeed is a creature's authored speed.
func bestiarySpeed(t *testing.T, id string) float64 {
	t.Helper()

	return mustNum(t, bestiaryCreature(t, id), "speed")
}

// bestiaryName is a creature's name.
func bestiaryName(t *testing.T, id string) string {
	t.Helper()

	return mustStr(t, bestiaryCreature(t, id), "name")
}
