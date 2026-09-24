//go:build playtest

package playtest

import (
	"image"
	"math"
	"strings"
	"testing"
)

// TestSquadsOnScreen is M4.4c-1's playtest (Constitution VI.2): the player
// commands SQUADS, and this asserts what a player sees through the "ui" and
// "meters" providers. The acts follow the brief's §10:
//
//  1. the shipped shape -- one squad; the provider's flat face, the squads[]
//     array and the sheet all report ONE number (the three faces agree);
//  2. selection -- a select-click on the player selects s:1, opens the sheet,
//     and is CONSUMED (his position and path are unchanged: it did not also
//     order a walk). This click lands on his OWN feet, so act 3 re-runs the
//     same assertion against a model two tiles away, where a fall-through
//     moves him a measurable distance, and proves the instrument can see a
//     walk at all;
//  3. the N-path -- squad_add places a second squad that is drawn, selectable
//     by a click on ITS model, and drains INDEPENDENTLY (s:1 is read across
//     both the write and the step, because "s:2's water fell" is equally true
//     of one shared meters object); its digest assertion is bodies_known,
//     never world_draws (hashed); squad_remove takes it back out;
//  4. daybreak -- a harness-placed second squad SURVIVES first light (its
//     models are standalone entities, never spawn-group members);
//  5. the water dial at 2.25 crosses Thirsty during night one, the drain
//     measured against the CLOCK's own delta, and the thirst cue appears;
//  6. the D5 contrast check -- the bar's fill differs from its surroundings by
//     a floor on a DAYLIGHT frame AND a deliberately dark one. Each frame
//     asserts its OWN background luminance first, so neither can quietly
//     become the other case;
//  8. a script can HOLD a mouse button at all -- strigoi_click's hold_frames,
//     which closes BUG-7's instrument half but NOT its guard assertion;
//  7. a TABLE-SPAWNED enemy gets a bar -- the gate is keyed on the spawn row,
//     not the stand-in monstats code the sprite comes from.
//
// At least one assertion compares a number the test chose against a number the
// system reported (the drain, and the three-faces 55).
// The numbers the c-1 review's sharpened assertions use (15 Sep 2026). Each
// says what it is measured against.
const (
	squadWalkEpsilon   = 0.01 // world tiles: a fall-through walk moves whole tiles
	squadSelectMinDist = 1.5  // world tiles the hero must be from s:2's model
	squadEmptyGroundDX = 220  // screen px from the hero, clear of both models
	// Luminance floors/ceilings for the two D5 frames. MEASURED on this script,
	// 15 Sep 2026: genuine daylight reads L 25.3 (§0 part 2 measured 25.9), the
	// pre-dawn moonlit frame this act used to be taken on reads L 12.0, and the
	// deliberately dark frame reads L 3.0. 20 and 10 separate all three.
	squadDaylightBgMin = 20.0 // luminance: day 25.3 passes, the old dawn frame (12.0) does not
	squadDarkBgMax     = 10.0 // luminance: night 3.0 passes, a dawn frame (12.0) does not
	squadMetersGapMin  = 10.0 // the two squads' water starts ~35 points apart
)

func TestSquadsOnScreen(t *testing.T) {
	const (
		waterDrain    = 2.25 // [DIAL] S15, hardcoded so this script says so if it moves
		contrastFloor = 30.0 // §0 part 2: crimson clears 50.8 by day, 73.7 at night
		tolerance     = 1.0
	)

	s := start(t)
	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Squad", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	// M4.7 step 3: Night 1's dead stay down -- this script's subject is the squads on screen,
	// and a risen man coming for him would be a different script.
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)

	// Step frames so advanceWorld's metersBodied latch flips: it binds s:1 to
	// the player's body AND its model's entity id (Game.advanceWorld). Without a
	// stepped frame the player exists but s:1's model has no entity yet, so the
	// selection hit test (act 2) would find nothing.
	s.call("strigoi_step", map[string]any{"frames": 60})

	player := s.call("strigoi_get_player", map[string]any{})
	px, py := pair(player, "screen")
	if px == 0 && py == 0 {
		t.Fatalf("no player screen position: %v", player)
	}

	t.Logf("player screen (%.0f, %.0f)", px, py)

	// --- ACT 1: the shipped shape, and the three faces agree -----------------
	m := metersState(s)

	if got := mustNum(t, m, "squad_count"); got != 1 {
		t.Fatalf("act 1: a shipped build has ONE squad, got squad_count %.0f", got)
	}

	if got := mustStr(t, m, "selected"); got != "s:1" {
		t.Fatalf("act 1: s:1 is selected at bind time, got %q", got)
	}

	squads := squadList(t, m)
	if len(squads) != 1 {
		t.Fatalf("act 1: want one squad in squads[], got %d", len(squads))
	}

	if got := mustStr(t, squads[0], "squad"); got != "s:1" {
		t.Fatalf("act 1: squads[0] is s:1, got %q", got)
	}

	// A number the TEST chose (55), read back through all THREE faces of the
	// one provider: the flat scalar, the squads[] array, and the sheet card.
	// If any diverges, the flat face is not preserved and M4.2/M4.5's whole
	// regression surface is a lie (brief §4.3).
	s.call("strigoi_set_system_field", map[string]any{
		"system": "meters", "field": "squad",
		"value": map[string]any{"squad": "s:1", "field": "water", "value": 55.0},
	})

	m = metersState(s)
	ui := uiState(s)

	flatWater := mustNum(t, m, "water")
	poolWater := mustNum(t, squadList(t, m)[0], "water")
	cards := cardList(t, ui)
	sheetWater := mustNum(t, cards[0], "water")

	if flatWater != 55 || poolWater != 55 || sheetWater != 55 {
		t.Fatalf("act 1: the three faces disagree on the water the test set to 55 -- "+
			"flat %.1f, squads[0] %.1f, sheet %.1f", flatWater, poolWater, sheetWater)
	}

	t.Logf("act 1: one squad; flat/squads[]/sheet all report the water the test set (55)")

	// --- ACT 2: selection consumes the click; the sheet opens ----------------
	if flag(t, ui, "sheet_open") {
		t.Fatalf("act 2: the sheet must be closed before a squad is selected")
	}

	before := s.call("strigoi_get_player", map[string]any{})
	x0, y0 := mustNum(t, before, "x"), mustNum(t, before, "y")

	// Click on the player's feet -- the centre of the sprite's hit rect, so the
	// select branch fires and CONSUMES it (opens the sheet, orders no walk):
	// brief §4.5, clause 10. The early return is what stops the walk.
	s.call("strigoi_click", map[string]any{"x": int(px), "y": int(py), "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 30})

	ui = uiState(s)
	if !flag(t, ui, "sheet_open") {
		t.Fatalf("act 2: a select-click on the player did not open the sheet")
	}

	if got := mustStr(t, ui, "selected_squad"); got != "s:1" {
		t.Fatalf("act 2: the select-click did not select s:1, got %q", got)
	}

	assertNoWalk(t, "act 2", s.call("strigoi_get_player", map[string]any{}), x0, y0)

	t.Logf("act 2: the select-click selected s:1, opened the sheet, and ordered no walk")

	// --- ACT 3: the N-path -- squad_add, drawn/selectable/independent, remove -
	bodiesBefore := mustNum(t, combatState(s), "bodies_known")

	s.call("strigoi_set_system_field", map[string]any{
		"system": "meters", "field": "squad_add", "value": map[string]any{},
	})

	m = metersState(s)
	if got := mustNum(t, m, "squad_count"); got != 2 {
		t.Fatalf("act 3: squad_add did not raise the count to 2, got %.0f", got)
	}

	// The digest assertion is bodies_known, NOT world_draws (hashed, and a
	// deployed model draws the world RNG by construction -- ruled ask 10(b)).
	// A number the test chose (+1) against the number the system reported.
	if got := mustNum(t, combatState(s), "bodies_known"); got != bodiesBefore+1 {
		t.Fatalf("act 3: deploying one squad model should adopt one body; bodies_known %.0f -> %.0f",
			bodiesBefore, got)
	}

	// squads[] is emitted through the ordinal sort, so element 0 is always s:1
	// and element 1 s:2 -- an assertion that reads element i must read the same
	// one on every run (brief §6.6), or determinism_test.go diverges at N>1.
	sq := squadList(t, m)
	if len(sq) < 2 {
		t.Fatalf("act 3: want two squads in squads[], got %d", len(sq))
	}

	if mustStr(t, sq[0], "squad") != "s:1" || mustStr(t, sq[1], "squad") != "s:2" {
		t.Fatalf("act 3: squads[] is not in ordinal order (element 0 must be s:1, element 1 s:2): %v", m["squads"])
	}

	s2 := sq[1]
	s2entity := mustStr(t, modelList(t, s2)[0], "entity")

	// Drawn: the bar widget has a bar over the second squad's model.
	if barFor(t, uiState(s), s2entity, true) == nil {
		t.Fatalf("act 3: the second squad's model %q has no overhead bar", s2entity)
	}

	// Selectable: the cycle key moves the selection s:1 -> s:2 -> s:1.
	s.call("strigoi_key", map[string]any{"key": "g", "action": "tap"})
	if got := mustStr(t, uiState(s), "selected_squad"); got != "s:2" {
		t.Fatalf("act 3: the cycle key did not move the selection to s:2, got %q", got)
	}

	s.call("strigoi_key", map[string]any{"key": "g", "action": "tap"})
	if got := mustStr(t, uiState(s), "selected_squad"); got != "s:1" {
		t.Fatalf("act 3: the cycle key did not wrap the selection back to s:1, got %q", got)
	}

	// A SELECT-CLICK ON A MODEL THAT IS NOT UNDER THE HERO. Act 2's click
	// landed on his own feet, where "he did not move" is a weak signal: a
	// fall-through to OnPlayerMove would have ordered a walk to the tile he
	// already stands on. s:2's model is deployed two tiles east of him
	// (squadDeployer.Deploy), so HERE a fall-through moves him a measurable
	// distance and the assertion can actually fail. The c-1 review, 15 Sep
	// 2026, found that the act 2 form could not.
	s2info := s.call("strigoi_get_entity", map[string]any{"handle": handleFor(t, s, s2entity)})
	s2sx, s2sy := pair(s2info, "screen")

	hero := s.call("strigoi_get_player", map[string]any{})
	hx, hy := mustNum(t, hero, "x"), mustNum(t, hero, "y")
	away := math.Hypot(mustNum(t, s2info, "x")-hx, mustNum(t, s2info, "y")-hy)

	if away < squadSelectMinDist {
		t.Fatalf("act 3: s:2's model is only %.2f tiles from the hero -- too close for the no-walk "+
			"assertion below to mean anything (it needs at least %.2f)", away, squadSelectMinDist)
	}

	s.call("strigoi_click", map[string]any{"x": int(s2sx), "y": int(s2sy), "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 30})

	ui = uiState(s)
	if got := mustStr(t, ui, "selected_squad"); got != "s:2" {
		t.Fatalf("act 3: a select-click on s:2's model, %.2f tiles away, did not select it, got %q", away, got)
	}

	if !flag(t, ui, "sheet_open") {
		t.Fatalf("act 3: the select-click on s:2's model did not open the sheet")
	}

	assertNoWalk(t, "act 3", s.call("strigoi_get_player", map[string]any{}), hx, hy)

	t.Logf("act 3: a select-click on s:2's model %.2f tiles away selected it and ordered no walk", away)

	// POSITIVE CONTROL for that assertion (Constitution VI.4: an instrument
	// nobody has seen fail is not an instrument). The same click, on a pixel
	// where NO model stands, must take the OTHER branch -- the one that closes
	// the sheet and calls OnPlayerMove. If the sheet does not close, the
	// clicks above never reached the branch the no-walk assertions are about
	// and those assertions proved nothing.
	ctlX := int(px) - squadEmptyGroundDX
	if ctlX < squadEmptyGroundDX {
		ctlX = int(px) + squadEmptyGroundDX
	}

	s.call("strigoi_click", map[string]any{"x": ctlX, "y": int(py), "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 30})

	if flag(t, uiState(s), "sheet_open") {
		t.Fatalf("act 3 control: a left click on empty ground at (%d,%.0f) did not close the sheet -- "+
			"the empty-ground branch was never reached, so the no-walk assertions above prove nothing",
			ctlX, py)
	}

	t.Logf("act 3 control: a click on empty ground took the walk branch (the sheet closed)")

	// DRAINS INDEPENDENTLY. "s:2's water fell" is not that claim -- one shared
	// *Meters behind two rows passes it unchanged -- so the OTHER squad is in
	// the assertion: s:1 is read across the write (no world time passes
	// between those two reads, so it is one stored value read twice, not an
	// accumulated float) and again across the step.
	s1Water0 := mustNum(t, squadWater(t, s, "s:1"), "water")

	s.call("strigoi_set_system_field", map[string]any{
		"system": "meters", "field": "squad",
		"value": map[string]any{"squad": "s:2", "field": "water", "value": 90.0},
	})

	if got := mustNum(t, squadWater(t, s, "s:1"), "water"); math.Abs(got-s1Water0) > 0.001 {
		t.Fatalf("act 3: writing s:2's water moved s:1's (%.3f -> %.3f) -- the two squads share one meters object",
			s1Water0, got)
	}

	s2Water0 := mustNum(t, squadWater(t, s, "s:2"), "water")
	if math.Abs(s2Water0-90) > 0.001 {
		t.Fatalf("act 3: the write did not land on s:2's own water (got %.3f, wanted 90)", s2Water0)
	}

	s.call("strigoi_step_world", map[string]any{"world_minutes": 120.0})

	s1Water1 := mustNum(t, squadWater(t, s, "s:1"), "water")
	s2Water1 := mustNum(t, squadWater(t, s, "s:2"), "water")

	if s2Water1 >= s2Water0 {
		t.Fatalf("act 3: the second squad did not drain: water %.1f -> %.1f", s2Water0, s2Water1)
	}

	if s1Water1 >= s1Water0 {
		t.Fatalf("act 3: the first squad did not drain across the same step: water %.1f -> %.1f",
			s1Water0, s1Water1)
	}

	if gap := math.Abs(s2Water1 - s1Water1); gap < squadMetersGapMin {
		t.Fatalf("act 3: the two squads' water converged to %.1f and %.1f (gap %.1f, under %.1f) -- "+
			"they are drinking from one pool", s1Water1, s2Water1, gap, squadMetersGapMin)
	}

	t.Logf("act 3: squad_add drew a selectable second squad that drained on its OWN meters "+
		"(s:1 %.1f -> %.1f while s:2 %.1f -> %.1f; bodies_known +1)", s1Water0, s1Water1, s2Water0, s2Water1)

	// --- ACT 4: daybreak -- the harness-placed squad survives first light -----
	// It is a standalone entity, never a spawn-group member, so clearAtDaybreak
	// (which despawns spawn-group members) does not touch it. Step to the next
	// StageDay and assert the model, and its body, are still there. This is the
	// assertion that proves §7's fence rather than asserting it in prose.
	stepToStage(s, "day")

	if got := mustNum(t, metersState(s), "squad_count"); got != 2 {
		t.Fatalf("act 4: the second squad vanished across daybreak (squad_count %.0f) -- "+
			"a squad model must never enter a spawn group", got)
	}

	// Its bar is only produced when its entity is still in the map engine
	// (refreshOverheadBars looks the entity up), so a present bar is a present
	// model -- the same data path act 3's "drawn" check used.
	if barFor(t, uiState(s), s2entity, true) == nil {
		t.Fatalf("act 4: the second squad's model %q lost its bar after first light -- "+
			"its entity was despawned, so a squad model must never enter a spawn group", s2entity)
	}

	t.Logf("act 4: the harness-placed squad survived first light (its model is not a spawn-group member)")

	// squad_remove takes it back out; the count returns and its body is released.
	bodiesBeforeRemove := mustNum(t, combatState(s), "bodies_known")
	s.call("strigoi_set_system_field", map[string]any{
		"system": "meters", "field": "squad_remove", "value": "s:2",
	})

	if got := mustNum(t, metersState(s), "squad_count"); got != 1 {
		t.Fatalf("act 3/4: squad_remove did not return the count to 1, got %.0f", got)
	}

	if got := mustNum(t, combatState(s), "bodies_known"); got != bodiesBeforeRemove-1 {
		t.Fatalf("act 3/4: squad_remove did not release the model's body; bodies_known %.0f -> %.0f",
			bodiesBeforeRemove, got)
	}

	t.Logf("act 3/4: squad_remove returned the count to 1 and released the body")

	// --- ACT 6a: the DAYLIGHT contrast frame, taken in ACTUAL daylight -------
	// It has to be taken HERE, after act 4 has stepped through daybreak. At
	// the top of the script the world has not reached sunrise, so the frame
	// captured there was a second DARK frame wearing the word "daylight" and
	// the tighter daylight background -- the one that set the 30-point floor
	// in §0 part 3 -- was never exercised. The c-1 review found it, 15 Sep
	// 2026. So: four world hours into the day, the clock has to agree, and the
	// BACKGROUND ITSELF is asserted bright before its contrast number is
	// trusted. The world time is not spent twice: act 5's stepToStage(night)
	// has four fewer hours to walk.
	s.call("strigoi_step_world", map[string]any{"world_minutes": 240.0})

	if c := clockState(s); str(c, "stage") != "day" {
		t.Fatalf("act 6a: wanted a daylight frame, but the clock says stage %s at %s",
			str(c, "stage"), str(c, "time_of_day"))
	}

	dayBar := barFor(t, uiState(s), str(player, "id"), false)
	dayFrame := s.frame(t, "squads-day")
	_, dayBg := assertBarContrast(t, "daylight", dayFrame, dayBar, contrastFloor)

	if dayBg < squadDaylightBgMin {
		t.Fatalf("act 6a: the daylight frame's background is L %.1f, under the %.1f floor -- "+
			"this is a dark frame, so it does not exercise the daylight case at all", dayBg, squadDaylightBgMin)
	}

	t.Logf("act 6a: the daylight frame is genuinely light (background L %.1f)", dayBg)

	// --- ACT 5: the water dial at 2.25 crosses Thirsty during night one -------
	stepToStage(s, "night")

	c := clockState(s)
	if str(c, "stage") != "night" {
		t.Fatalf("act 5: wanted night one, got stage %s at %s", str(c, "stage"), str(c, "time_of_day"))
	}

	// Stand water just above Thirsty (33) with the body idle, so the ONLY drain
	// is the base water rate (no daylight factor at night, no labour).
	s.call("strigoi_set_system_field", map[string]any{"system": "meters", "field": "activity", "value": "idle"})
	s.call("strigoi_set_system_field", map[string]any{"system": "meters", "field": "water", "value": 40.0})

	c = clockState(s)
	wm0 := num(c, "world_minutes")
	w0 := mustNum(t, metersState(s), "water")

	if flag(t, metersState(s), "thirsty") {
		t.Fatalf("act 5: water 40 should be above Thirsty (33), but thirsty is already set")
	}

	// Four world hours at 2.25/hour drains ~9 points, from 40 past 33 to ~31.
	s.call("strigoi_step_world", map[string]any{"world_minutes": 240.0})

	c = clockState(s)
	wm1 := num(c, "world_minutes")
	m = metersState(s)
	w1 := mustNum(t, m, "water")

	if str(c, "stage") != "night" {
		t.Fatalf("act 5: the drain window left night one (stage %s at %s)", str(c, "stage"), str(c, "time_of_day"))
	}

	// The drain measured against the CLOCK's OWN delta (world_minutes is
	// monotonic, so no midnight wrap and the ~2.3%% step overshoot is absorbed).
	hours := (wm1 - wm0) / 60
	predicted := waterDrain * hours

	if math.Abs((w0-w1)-predicted) > tolerance {
		t.Fatalf("act 5: water fell %.2f over %.3f world hours; at WaterDrain %.2f it should fall about %.2f "+
			"(if this drifts, the dial moved -- say so)", w0-w1, hours, waterDrain, predicted)
	}

	if !flag(t, m, "thirsty") {
		t.Fatalf("act 5: water fell to %.1f (<=33) during night one but thirsty did not flip", w1)
	}

	// The thirst CUE appears at the crossing, on the player's model.
	if !cueHas(uiState(s), str(player, "id"), "thirsty") {
		t.Fatalf("act 5: the thirsty cue is not shown over the player after the crossing")
	}

	t.Logf("act 5: water crossed Thirsty during night one (%.1f -> %.1f), drain matched the clock's delta, cue shown", w0, w1)

	// --- ACT 6b: the deliberately DARK contrast frame -------------------------
	// A night screenshot is not evidence on its own (the black-floor trap), so
	// the dark background is produced deliberately (moon 0), and the ASSERTION
	// is the bar's contrast, not the frame's darkness.
	s.call("strigoi_set_system_field", map[string]any{"system": "clock", "field": "moon", "value": 0})
	s.call("strigoi_step", map[string]any{"frames": 5})

	darkBar := barFor(t, uiState(s), str(player, "id"), false)
	darkFrame := s.frame(t, "squads-night")
	_, darkBg := assertBarContrast(t, "deliberately dark", darkFrame, darkBar, contrastFloor)

	// The pair is only a pair if the two frames are different cases.
	if darkBg > squadDarkBgMax {
		t.Fatalf("act 6b: the deliberately dark frame's background is L %.1f, over the %.1f ceiling -- "+
			"it is not dark, so the two frames are one case twice", darkBg, squadDarkBgMax)
	}

	t.Logf("act 6: the bar's fill cleared the contrast floor on a frame of background L %.1f "+
		"AND one of L %.1f", dayBg, darkBg)

	// --- ACT 7: a TABLE-SPAWNED enemy gets a bar -----------------------------
	// The bar gate is keyed on the SPAWN ROW, not the monstats code, because
	// N1 §5's codes are stand-in SPRITES: the wolves wear "zombie1" and the boar
	// wears "skeleton1". Until the c-1 review (15 Sep 2026) the gate read
	// MonStatRecord.MonsterGroup, which would have taken the bar off night
	// one's main threat while leaving the dogs (fallen1) barred -- so nothing
	// on screen would have looked broken. The assertion is the DELTA in enemy
	// bars across a forced arrival, which is robust to whatever else is on the
	// map, and it names the row it got.
	//
	// Clear the field first: night one has been running for hours by now and
	// the tables have had their own chances to fire, so the baseline has to be
	// a known one. Despawning takes those groups' bars with them. The chance
	// goes to 0 first: the night's own checks keep rolling, and one that fell
	// in the frames below put a fresh group on the cleared field (history item
	// 117, when step_world's batching changed and moved the checks by a tick).
	s.call("strigoi_set_system_field", map[string]any{"system": "spawns", "field": "chance", "value": 0})

	for _, g := range asList(spawnsState(s)["group_list"]) {
		if row, ok := g.(map[string]any); ok {
			s.call("strigoi_set_system_field", map[string]any{
				"system": "spawns", "field": "despawn", "value": str(row, "group"),
			})
		}
	}

	s.call("strigoi_step", map[string]any{"frames": 5})

	if got := num(spawnsState(s), "groups"); got != 0 {
		t.Fatalf("act 7: the field did not clear -- %.0f group(s) still live: %v", got, spawnsState(s)["group_list"])
	}

	enemyBefore := enemyBars(t, uiState(s))

	// The chance dial is raised rather than a spawn verb being called: there
	// is no spawn verb by design, and forcing the real table is evidence about
	// the game (spawns_test.go act 6 makes the same argument).
	s.call("strigoi_set_system_field", map[string]any{"system": "spawns", "field": "chance", "value": 100})

	for i := 0; i < 12 && num(spawnsState(s), "groups") == 0; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 6})
	}

	arrived := spawnsState(s)
	if num(arrived, "groups") == 0 {
		t.Fatalf("act 7: a certainty must actually fire; %.0f check(s), %.0f roll(s), %.0f failure(s)",
			num(arrived, "checks"), num(arrived, "rolls"), num(arrived, "spawn_failures"))
	}

	s.call("strigoi_step", map[string]any{"frames": 5})

	rows := []string{}
	members := 0

	for _, g := range asList(spawnsState(s)["group_list"]) {
		if row, ok := g.(map[string]any); ok {
			rows = append(rows, str(row, "row")+"/"+str(row, "code"))
			members += int(num(row, "members"))
		}
	}

	if got := enemyBars(t, uiState(s)) - enemyBefore; got != members {
		t.Fatalf("act 7: %v arrived with %d member(s) but the enemy bars rose by %d. Every LIVING "+
			"table-spawned enemy gets a bar, and the gate is keyed on the spawn ROW for exactly this "+
			"reason -- a gate keyed on the monstats group asks what the sprite is, not what the thing "+
			"is, and takes the bar off the rows wearing undead stand-ins.", rows, members, got)
	}

	t.Logf("act 7: %v arrived with %d member(s) and every one got a bar (enemy bars %d -> %d)",
		rows, members, enemyBefore, enemyBefore+members)

	// --- ACT 8: a script can HOLD a mouse button (BUG-7's instrument half) -----
	//
	// WHAT THIS ACT CLAIMS, AND WHAT IT DOES NOT. Until 19 Sep 2026 no playtest
	// could exercise a held mouse button at all: strigoi_click pressed AND
	// released inside one frame, so GameControls.OnMouseButtonRepeat -- reached
	// while a button is DOWN -- was unreachable from any script by construction.
	// That is BUG-7's instrument half, strigoi_click grew hold_frames, and this
	// act asserts it: a held click is delivered as a hold, the controls act on
	// it, and the button does not stay stuck down.
	//
	// IT DOES NOT ASSERT c-1'S SQUAD GUARD IN THAT HANDLER, and the reason is
	// measured rather than assumed. A draft of this act held a click on a squad
	// model and asserted no walk. It passed -- and it passed WITH THE GUARD
	// DISABLED, twice: once against the hero's own model, where a walk order is a
	// no-op (the same weakness the c-1 review caught in act 2), and again against
	// a squad deployed 2.00 tiles away. So something upstream of the guard
	// already stops that walk. The suspect is isInActiveMenusRect: the
	// select-click opens the sheet on the same press, and a point inside an open
	// panel's rect is excluded from the walk before the guard is consulted --
	// which would make the guard unreachable for a PLAYER too, not just for a
	// script. NOT ESTABLISHED: a probe found a held click on open ground at x+70
	// walks whether the sheet was open or not, but that click closed the sheet,
	// so it never tested the geometry that matters. The panel rects are c-2b's
	// own territory (the strip at y 470-524).
	//
	// A GREEN ASSERTION WHOSE CONTROL PASSES IS WORSE THAN NO ASSERTION, so the
	// claim is left out and docs/bugs.md BUG-7 carries what is still owed.
	s.call("strigoi_click", map[string]any{"x": 5, "y": 5, "button": "left"}) // dismiss the sheet
	s.call("strigoi_step", map[string]any{"frames": 10})

	player = s.call("strigoi_get_player", map[string]any{})
	holdScreenX, holdScreenY := pair(player, "screen")
	holdX, holdY := int(holdScreenX)+90, int(holdScreenY)
	holdFromX, holdFromY := mustNum(t, player, "x"), mustNum(t, player, "y")

	out := s.call("strigoi_click", map[string]any{
		"x": holdX, "y": holdY, "button": "left", "hold_frames": 40,
	})

	// The verb reports what it did, and the report has to say HELD -- a
	// hold_frames that quietly fell through to the tap path would move the hero
	// just the same, and this assertion is the whole difference.
	if applied := mustStr(t, out, "applied"); !strings.Contains(applied, "held 40 frame(s)") {
		t.Fatalf("act 8: strigoi_click did not report a held press: %q", applied)
	}

	s.call("strigoi_step", map[string]any{"frames": 30})

	moved := s.call("strigoi_get_player", map[string]any{})
	if math.Abs(mustNum(t, moved, "x")-holdFromX) <= squadWalkEpsilon &&
		math.Abs(mustNum(t, moved, "y")-holdFromY) <= squadWalkEpsilon {
		t.Fatalf("act 8: a 40-frame held click on open ground moved nothing from (%.3f,%.3f) -- "+
			"a hold the controls never act on is not an instrument", holdFromX, holdFromY)
	}

	// AND THE BUTTON IS RELEASED AFTERWARDS. A hold that left it stuck down would
	// pass the check above and break every later script, so the release is
	// asserted through its effect: an ordinary tap still orders a walk.
	restX, restY := mustNum(t, moved, "x"), mustNum(t, moved, "y")

	s.call("strigoi_click", map[string]any{"x": holdX, "y": holdY - 60, "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 30})

	again := s.call("strigoi_get_player", map[string]any{})
	if math.Abs(mustNum(t, again, "x")-restX) <= squadWalkEpsilon &&
		math.Abs(mustNum(t, again, "y")-restY) <= squadWalkEpsilon {
		t.Fatalf("act 8: an ordinary tap after a held click did nothing -- the hold left the " +
			"button stuck down")
	}

	t.Logf("act 8 PASS: a 40-frame held click is delivered AS a hold, the controls act on it "+
		"(%.2f,%.2f -> %.2f,%.2f), and a tap afterwards still works. BUG-7's instrument half is "+
		"closed; its guard assertion is NOT, and the row says why",
		holdFromX, holdFromY, restX, restY)
}

// enemyBars counts the overhead bars the "ui" provider marks as an enemy's.
func enemyBars(t *testing.T, ui map[string]any) int {
	t.Helper()

	n := 0

	for _, raw := range asList(ui["bars"]) {
		if row, ok := raw.(map[string]any); ok && flag(t, row, "enemy") {
			n++
		}
	}

	return n
}

// --- helpers ----------------------------------------------------------------

func uiState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "ui"}), "state")
}

func clockState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "clock"}), "state")
}

// squadWater returns the squads[] row for a squad id, failing clearly if it is
// gone -- so a despawned squad reddens with a message rather than an index
// panic, and the read is by id rather than position.
func squadWater(t *testing.T, s *session, id string) map[string]any {
	t.Helper()

	for _, sq := range squadList(t, metersState(s)) {
		if str(sq, "squad") == id {
			return sq
		}
	}

	t.Fatalf("squad %q is not in squads[] -- it vanished", id)

	return nil
}

// stepToStage steps whole days of world time until the clock reaches the wanted
// stage, so a test can reach the next night or the next daybreak without
// arithmetic on the day table.
func stepToStage(s *session, want string) {
	for i := 0; i < 48; i++ {
		if str(clockState(s), "stage") == want {
			return
		}

		s.call("strigoi_step_world", map[string]any{"world_minutes": 30.0})
	}
}

func squadList(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	return objList(t, m, "squads")
}

func modelList(t *testing.T, sq map[string]any) []map[string]any {
	t.Helper()
	return objList(t, sq, "models")
}

func cardList(t *testing.T, ui map[string]any) []map[string]any {
	t.Helper()
	return objList(t, ui, "sheet_cards")
}

func objList(t *testing.T, m map[string]any, key string) []map[string]any {
	t.Helper()

	raw, ok := m[key].([]any)
	if !ok {
		t.Fatalf("field %q is not a list: %T", key, m[key])
	}

	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if row, ok := e.(map[string]any); ok {
			out = append(out, row)
		}
	}

	return out
}

// bar is one entry of the "ui" provider's bars[] (id, x, y, w, h, fill).
type bar struct {
	x, y, w, h int
	fill       float64
}

// barFor returns the overhead bar for an entity id, or fails (or returns nil,
// per mustFind) if there is none.
func barFor(t *testing.T, ui map[string]any, id string, allowMissing bool) *bar {
	t.Helper()

	raw, _ := ui["bars"].([]any)
	for _, e := range raw {
		row, ok := e.(map[string]any)
		if !ok {
			continue
		}

		if str(row, "id") != id {
			continue
		}

		return &bar{
			x:    int(num(row, "x")),
			y:    int(num(row, "y")),
			w:    int(num(row, "w")),
			h:    int(num(row, "h")),
			fill: num(row, "fill"),
		}
	}

	if !allowMissing {
		t.Fatalf("no overhead bar for entity %q in %v", id, ui["bars"])
	}

	return nil
}

// cueHas reports whether the entity's stage cues include the given cue key.
func cueHas(ui map[string]any, id, cue string) bool {
	raw, _ := ui["cues"].([]any)
	for _, e := range raw {
		row, ok := e.(map[string]any)
		if !ok || str(row, "id") != id {
			continue
		}

		cues, _ := row["cues"].([]any)
		for _, c := range cues {
			if s, _ := c.(string); s == cue {
				return true
			}
		}
	}

	return false
}

// assertBarContrast samples the bar's fill and a point just outside it and
// asserts they differ in luminance by at least the floor -- the D5 gate's
// instrument (ruled ask 8), on whichever frame it is handed. Luminance is
// night_render's own formula (the mean of the 8-bit channels).
//
// It RETURNS both luminances, because the contrast number alone does not say
// which frame it came from: the caller has to be able to assert that a frame
// called "daylight" is light and one called "dark" is dark, or the pair can
// quietly become the same case twice (the c-1 review, 15 Sep 2026).
func assertBarContrast(t *testing.T, frame string, img image.Image, b *bar, floor float64) (fillL, bgL float64) {
	t.Helper()

	if b.w < 4 || b.h < 2 {
		t.Fatalf("%s: the bar rect is too small to sample: %+v", frame, b)
	}

	// The fill sits at the left of the bar; sample its centre.
	fillL = lumAt(img, b.x+b.w/4, b.y+b.h/2)
	// The background: a few pixels above the bar, clear of the frame and cues.
	bgL = lumAt(img, b.x+b.w/2, b.y-6)

	if diff := math.Abs(fillL - bgL); diff < floor {
		t.Fatalf("%s: the bar's fill (L %.1f) and its surroundings (L %.1f) differ by only %.1f, "+
			"below the D5 floor %.1f -- the bar is not legible here", frame, fillL, bgL, diff, floor)
	}

	return fillL, bgL
}

// assertNoWalk fails unless the player is where he was and has no path: the
// select-click was CONSUMED (brief §4.5, clause 10).
//
// It reads path_len out of the player's STATE, not off the top level of
// strigoi_get_player, and through mustNum rather than num. The top level has
// no such field -- a never-assigned, omitempty PathLen used to sit in
// harnessEntityInfo, deleted in this commit -- so num() answered 0 for
// "absent" exactly as it would for a real zero, and this assertion could not
// fail on any run. mustNum reddens loudly if the field ever moves again.
func assertNoWalk(t *testing.T, act string, after map[string]any, x0, y0 float64) {
	t.Helper()

	x1, y1 := mustNum(t, after, "x"), mustNum(t, after, "y")

	if n := mustNum(t, sub(after, "state"), "path_len"); n != 0 {
		t.Fatalf("%s: the select-click ORDERED A WALK -- path_len %.0f. The click must be consumed "+
			"by an early return before OnPlayerMove.", act, n)
	}

	if math.Abs(x1-x0) > squadWalkEpsilon || math.Abs(y1-y0) > squadWalkEpsilon {
		t.Fatalf("%s: the select-click MOVED the player (%.3f,%.3f) -> (%.3f,%.3f). The click must be "+
			"consumed by an early return before OnPlayerMove.", act, x0, y0, x1, y1)
	}
}

// handleFor finds the harness handle for a raw entity id, so a script can aim
// strigoi_click at an entity it did not spawn itself: strigoi_get_entities
// reports handles but no screen position, strigoi_get_entity reports both.
func handleFor(t *testing.T, s *session, id string) string {
	t.Helper()

	entities := s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200})

	for _, raw := range asList(entities["items"]) {
		if row, ok := raw.(map[string]any); ok && str(row, "id") == id {
			return mustStr(t, row, "handle")
		}
	}

	t.Fatalf("no harness handle for entity %q -- it is not in the npc list", id)

	return ""
}

func lumAt(img image.Image, x, y int) float64 {
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return 0
	}

	r, g, bl, _ := img.At(x, y).RGBA()

	return float64(r>>8+g>>8+bl>>8) / 3
}
