//go:build playtest

package playtest

import (
	"testing"
)

// TestSpawns is M4.3b's Constitution VI.2 script: the ninth.
//
// The milestone's assertion is "a spawned thing notices the player under
// stated conditions and does not otherwise", and the second half is the half
// that matters. A watcher that notices proves nothing on its own -- it might
// have noticed for the wrong reason, or noticed everything always. So every
// positive here is paired with a negative taken from the same map, the same
// distance and the same tick, and the notice block reports the INPUTS (sees,
// distance, light_at_quarry) beside the verdict so the two cases can be told
// apart. That reporting is M4.3b ask 6, and the ask exists because M4.3a's
// section 3.2 was signed with an assertion nothing in the harness could write.
//
// Six acts:
//  1. find two tiles the same distance away: one with a clear line to the
//     player, one without;
//  2. POSITIVE -- a watcher in the open notices;
//  3. NEGATIVE -- a watcher behind cover does not, at the same distance;
//  4. the torch trade: a watcher beyond the radius does not notice until the
//     player lights up, and then does (R2 section 3's dark-into-light
//     advantage, seen from the other side);
//  5. the tables move with the clock and with the carrion count;
//  6. a forced arrival really arrives, carries morale both directions, and
//     can be taken back out again.
func TestSpawns(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gaunt", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")
	playerHandle := str(p, "handle")
	t.Logf("player at %.2f,%.2f (%s)", px, py, playerHandle)

	spawns := spawnsState(s)
	if !flag(t, spawns, "notice_wired") {
		t.Fatalf("the notice model is not wired: sight and light must both be attached, got %v", spawns)
	}

	t.Logf("spawns provider live: radius %.1f, lit x%.1f at %.2f, re-evaluate %.2f world-min",
		num(spawns, "notice_radius"), num(spawns, "notice_lit_multiplier"),
		num(spawns, "notice_lit_level"), num(spawns, "notice_re_evaluate_minutes"))

	// --- act 1: one tile in the open, one behind cover, same distance --------
	//
	// The map is generated rather than authored, so no fixed vector is
	// guaranteed to be blocked. Sweep a ring and take the first pair that
	// disagree about line of sight at the SAME radius -- holding distance
	// equal is what makes act 3 a control rather than a second variable.
	const ring = 6.0

	var clearX, clearY, blockedX, blockedY float64

	var haveClear, haveBlocked bool

	for _, d := range [][2]float64{
		{1, 0}, {0, 1}, {-1, 0}, {0, -1},
		{0.7, 0.7}, {-0.7, 0.7}, {0.7, -0.7}, {-0.7, -0.7},
	} {
		x, y := px+d[0]*ring, py+d[1]*ring

		path := s.call("strigoi_find_path", map[string]any{"to_x": x, "to_y": y})

		clear := flag(t, path, "straight_line_clear")
		if clear && !haveClear {
			clearX, clearY, haveClear = x, y, true
		}

		if !clear && !haveBlocked {
			blockedX, blockedY, haveBlocked = x, y, true
		}
	}

	if !haveClear || !haveBlocked {
		t.Fatalf("need one clear and one blocked bearing at %.0f tiles; clear=%t blocked=%t",
			ring, haveClear, haveBlocked)
	}

	t.Logf("act 1: clear line to %.1f,%.1f · blocked line to %.1f,%.1f (both %.0f tiles out)",
		clearX, clearY, blockedX, blockedY, ring)

	// --- act 2: POSITIVE -- in the open, it notices --------------------------
	seer := spawnNPC(t, s, "fallen1", clearX, clearY)

	s.call("strigoi_watch", map[string]any{"watcher": seer, "target": playerHandle})

	// Read the block before asserting anything, so a failure prints the inputs
	// that produced the verdict rather than only the verdict. That is what the
	// block is for.
	seerRow := noticeRowFor(t, s, seer)
	t.Logf("act 2 inputs: sees=%v distance=%.2f reach=%.1f light=%.3f",
		seerRow["sees"], num(seerRow, "distance"), num(seerRow, "reach"),
		num(seerRow, "light_at_quarry"))

	// THE UNITS ASSERTION, and it is here because its absence cost M4.3a a
	// false closeout number. The adapters feed d2world world tiles; if anything
	// ever divides by the subtile factor again, this is the line that says so
	// in one number instead of leaving a system quietly working at one fifth
	// scale. A watcher placed 6 tiles out must report about 6.
	if d := num(seerRow, "distance"); d < ring*0.8 || d > ring*1.25 {
		t.Fatalf("act 2: a watcher placed %.0f tiles away reports distance %.2f -- "+
			"the adapters are not in world tiles", ring, d)
	}

	if !flag(t, seerRow, "sees") {
		t.Fatalf("act 2: a watcher %.0f tiles away with a clear line must SEE the player; got %v",
			ring, seerRow)
	}

	if !flag(t, seerRow, "noticed") {
		t.Fatalf("act 2: seeing it, the watcher must notice it; got %v", seerRow)
	}

	t.Logf("act 2 PASS: %s sees the player at %.2f tiles, light %.3f, reach %.1f",
		seer, num(seerRow, "distance"), num(seerRow, "light_at_quarry"), num(seerRow, "reach"))

	// --- act 3: NEGATIVE -- behind cover at the same distance, it does not ---
	//
	// This is the assertion the milestone exists for. Same map, same tick,
	// same radius; the only thing that differs is the line.
	blind := spawnNPC(t, s, "fallen1", blockedX, blockedY)

	s.call("strigoi_watch", map[string]any{"watcher": blind, "target": playerHandle})

	blindRow := noticeRowFor(t, s, blind)
	t.Logf("act 3 inputs: sees=%v distance=%.2f reach=%.1f",
		blindRow["sees"], num(blindRow, "distance"), num(blindRow, "reach"))

	if flag(t, blindRow, "noticed") {
		t.Fatalf("act 3: a watcher behind cover must NOT notice, however close; got %v", blindRow)
	}
	if flag(t, blindRow, "sees") {
		t.Fatalf("act 3: the blocked watcher must report sees=false, got %v", blindRow)
	}

	if d := num(blindRow, "distance"); d > num(seerRow, "distance")+1.5 {
		t.Fatalf("act 3: the control must be about the same distance away, got %.2f vs %.2f",
			d, num(seerRow, "distance"))
	}

	t.Logf("act 3 PASS: %s is %.2f tiles away -- closer than the radius -- and still does not see the player",
		blind, num(blindRow, "distance"))

	// --- act 4: the torch trade ---------------------------------------------
	//
	// Shrink the radius rather than hunting for a clear line at 15 tiles: the
	// dial is settable precisely so a script can put a watcher in the gap
	// between dark reach and lit reach without depending on map luck. It also
	// proves the dial moves the VERDICT and not just the reported number.
	s.call("strigoi_watch", map[string]any{"watcher": blind, "release": true})

	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "notice_radius", "value": ring / 2,
	})
	s.call("strigoi_step_world", map[string]any{"world_minutes": 2})

	// One re-evaluation in: it has stopped SEEING, and it has not yet stopped
	// coming. That gap is MemoryMinutes doing its job -- without it, stepping
	// behind cover would cancel awareness on the very next tick and cover would
	// be a switch rather than a tactic. The first draft of this act asserted
	// the verdict here and failed on its own timing, which is the memory window
	// proving itself.
	seerRow = noticeRowFor(t, s, seer)
	if flag(t, seerRow, "sees") {
		t.Fatalf("act 4: with the reach at %.1f, a player %.2f tiles away must be out of sight; got %v",
			num(seerRow, "reach"), num(seerRow, "distance"), seerRow)
	}

	if !flag(t, seerRow, "noticed") {
		t.Fatalf("act 4: %.2f world minutes after losing sight, memory must still hold; got %v",
			num(seerRow, "minutes_unseen"), seerRow)
	}

	t.Logf("act 4a PASS: sight lost at reach %.1f, still coming %.2f world-min later (memory holds)",
		num(seerRow, "reach"), num(seerRow, "minutes_unseen"))

	// Past the memory window it gives up.
	s.call("strigoi_step_world", map[string]any{"world_minutes": 5})

	seerRow = noticeRowFor(t, s, seer)
	if flag(t, seerRow, "noticed") {
		t.Fatalf("act 4: past the memory window the watcher must forget; got %v", seerRow)
	}

	t.Logf("act 4b PASS: memory expired and the watcher lost the player")

	s.call("strigoi_set_system_field", map[string]any{
		"system": "light", "field": "carried_source", "value": "torch",
	})
	s.call("strigoi_step_world", map[string]any{"world_minutes": 2})

	seerRow = noticeRowFor(t, s, seer)
	if !flag(t, seerRow, "noticed") {
		t.Fatalf("act 4: a lit player at %.0f tiles is inside the doubled reach and must be noticed; got %v",
			ring, seerRow)
	}

	if r := num(seerRow, "reach"); r <= ring/2 {
		t.Fatalf("act 4: the lit multiplier must widen the reach past %.1f, got %.1f", ring/2, r)
	}

	t.Logf("act 4c PASS: light %.3f took reach from %.1f to %.1f and the verdict with it -- "+
		"carrying a torch is what got the player seen again",
		num(seerRow, "light_at_quarry"), ring/2, num(seerRow, "reach"))

	s.call("strigoi_set_system_field", map[string]any{
		"system": "light", "field": "carried_source", "value": "",
	})
	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "notice_radius", "value": 12,
	})

	// --- act 5: the tables move with the clock and with the carrion ----------
	before := spawnsState(s)
	beforeStage := str(before, "stage")
	beforeWolves := rowWeight(t, before, "wolves")

	// Walk to the deep night. The clock compresses, so measure against its own
	// elapsed minutes rather than the hours asked for.
	for i := 0; i < 40 && str(spawnsState(s), "stage") != "night"; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 60})
	}

	night := spawnsState(s)
	if str(night, "stage") != "night" {
		t.Fatalf("act 5: never reached the deep night, stage=%q", str(night, "stage"))
	}

	if band := num(night, "band"); band < 0 {
		t.Fatalf("act 5: the deep night must report a band, got %v", band)
	}

	nightWolves := rowWeight(t, night, "wolves")
	if nightWolves <= beforeWolves {
		t.Fatalf("act 5: wolves are a deep-night row; weight was %.3f in %q and %.3f at night",
			beforeWolves, beforeStage, nightWolves)
	}

	t.Logf("act 5a PASS: stage %q -> night band %.0f took the wolves row from %.3f to %.3f",
		beforeStage, num(night, "band"), beforeWolves, nightWolves)

	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "open_bodies", "value": 4,
	})

	carrion := spawnsState(s)
	if w := num(carrion, "carrion_weight"); w <= 1.0 {
		t.Fatalf("act 5: four open bodies must weigh more than none, got %.3f", w)
	}

	if w := rowWeight(t, carrion, "wolves"); w <= nightWolves {
		t.Fatalf("act 5: carrion must lift a beast row; %.3f -> %.3f", nightWolves, w)
	}

	t.Logf("act 5b PASS: 4 open bodies -> carrion weight %.2f, wolves %.3f -> %.3f",
		num(carrion, "carrion_weight"), nightWolves, rowWeight(t, carrion, "wolves"))

	// --- act 6: a forced arrival, its morale, and taking it back out ---------
	//
	// The chance dial is raised rather than a spawn verb being called, because
	// there is no spawn verb by design: forcing the real table is evidence
	// about the game, and a bypass would be evidence about the bypass.
	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "chance", "value": 100,
	})

	for i := 0; i < 12 && num(spawnsState(s), "groups") == 0; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 6})
	}

	arrived := spawnsState(s)
	if num(arrived, "groups") == 0 {
		t.Fatalf("act 6: a certainty must actually fire; %d check(s), %d roll(s), %d failure(s)",
			int(num(arrived, "checks")), int(num(arrived, "rolls")),
			int(num(arrived, "spawn_failures")))
	}

	group := firstGroup(t, arrived)
	groupID := str(group, "group")

	t.Logf("act 6: %s (%s, code %s) arrived with %d member(s), morale %.0f, %d aware",
		groupID, str(group, "row"), str(group, "code"),
		int(num(group, "members")), num(group, "morale"), int(num(group, "aware")))

	if blocks, ok := group["notice"].([]any); !ok || len(blocks) == 0 {
		t.Fatalf("act 6: every group must carry a notice block per ask 6, got %v", group["notice"])
	}

	if flag(t, group, "routing") {
		t.Fatalf("act 6: a fresh group must not already be routing, morale %.0f", num(group, "morale"))
	}

	// Morale in both directions -- the third provider rule. A value a script
	// can only watch fall is a value it cannot test.
	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "morale",
		"value": map[string]any{"group": groupID, "value": 5},
	})

	if !flag(t, firstGroup(t, spawnsState(s)), "routing") {
		t.Fatalf("act 6: morale 5 is under the rout threshold; the group must report routing")
	}

	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "morale",
		"value": map[string]any{"group": groupID, "value": 90},
	})

	if flag(t, firstGroup(t, spawnsState(s)), "routing") {
		t.Fatalf("act 6: morale 90 is well above the threshold; routing must clear again")
	}

	t.Logf("act 6a PASS: %s routed at morale 5 and stopped routing at 90 -- the state M4.5 will read", groupID)

	// And out again: a collection needs a verb that empties it.
	watchingBefore := int(num(spawnsState(s), "notice_watching"))

	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "despawn", "value": groupID,
	})

	after := spawnsState(s)
	if int(num(after, "notice_watching")) >= watchingBefore {
		t.Fatalf("act 6: despawning a group must stop its members being watched; %d -> %d",
			watchingBefore, int(num(after, "notice_watching")))
	}

	if msg := s.callErr("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "despawn", "value": groupID,
	}); msg == "" {
		t.Fatalf("act 6: despawning the same group twice must be an error, not a silent success")
	}

	t.Logf("act 6b PASS: despawn took watchers %d -> %d and the second despawn was refused",
		watchingBefore, int(num(after, "notice_watching")))

	// --- act 7: awareness must START A CHASE, with nobody asking -----------
	//
	// THIS IS THE ACT THE MILESTONE SHIPPED WITHOUT, and an audit found the
	// hole rather than a test doing it. Notice worked out awareness and
	// Pursuit could route a chase, and in any non-harness build nothing joined
	// them -- so a wolf saw the player and stood there, while this very script
	// passed because act 5 of the pathfinding run called strigoi_pursue by
	// hand. A feature reachable only from the harness is not a feature.
	//
	// So: NOTHING BELOW CALLS strigoi_pursue. The chase has to appear on its
	// own or the assertion fails.
	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "notice_radius", "value": 40,
	})
	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "chance", "value": 100,
	})

	for i := 0; i < 10 && len(awareList(spawnsState(s))) == 0; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 6})
	}

	aware := awareList(spawnsState(s))
	if len(aware) == 0 {
		t.Fatalf("act 7: with a 40-tile notice radius and a forced table, something must "+
			"end up aware of the player; state=%v", spawnsState(s))
	}

	chases := sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")
	if n := num(chases, "chases"); n == 0 {
		t.Fatalf("act 7: %d watcher(s) are aware of the player and NOTHING is chasing. "+
			"Awareness that nothing acts on is a diorama -- this is the M4.3b hole. aware=%v",
			len(aware), aware)
	}

	t.Logf("act 7 PASS: %d aware -> %.0f chase(s) started with nothing calling strigoi_pursue",
		len(aware), num(chases, "chases"))

	t.Logf("M4.3b: %d table check(s), %d roll(s), %d spawned, %d failure(s); "+
		"%d notice check(s), %d notice(s)",
		int(num(after, "checks")), int(num(after, "rolls")), int(num(after, "spawned")),
		int(num(after, "spawn_failures")), int(num(after, "notice_checks")),
		int(num(after, "notices")))
}

// spawnsState reads the spawns provider. Read through "state" or every field
// silently reads zero -- get_system_state returns {system, settable, state}.
func spawnsState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "spawns"}), "state")
}

// spawnNPC places one npc in world tiles and returns its handle.
func spawnNPC(t *testing.T, s *session, code string, x, y float64) string {
	t.Helper()

	out := s.call("strigoi_spawn_entity", map[string]any{
		"kind": "npc", "code": code, "x": x, "y": y,
	})

	h := str(out, "handle")
	if h == "" {
		t.Fatalf("spawning %q at %.1f,%.1f returned no handle: %v", code, x, y, out)
	}

	return h
}

// noticeRowFor finds one watcher's notice block by the entity's own id.
//
// It reads notice_list rather than the per-group blocks, because a watcher
// started by strigoi_watch belongs to no group -- and placing a watcher
// exactly where it has to be is the whole mechanism behind the negative
// control in act 3.
func noticeRowFor(t *testing.T, s *session, watcher string) map[string]any {
	t.Helper()

	id := entityID(t, s, watcher)

	state := spawnsState(s)

	rows, ok := state["notice_list"].([]any)
	if !ok {
		t.Fatalf("the provider must report notice_list; got %v", state["notice_list"])
	}

	for _, r := range rows {
		rm, ok := r.(map[string]any)
		if ok && str(rm, "watcher") == id {
			return rm
		}
	}

	t.Fatalf("no notice block for %q (%s) in %v", watcher, id, rows)

	return nil
}

// entityID turns a harness handle into the entity id d2world stores.
func entityID(t *testing.T, s *session, handle string) string {
	t.Helper()

	entity := s.call("strigoi_get_entity", map[string]any{"handle": handle})

	id := str(entity, "id")
	if id == "" {
		t.Fatalf("no id for handle %q: %v", handle, entity)
	}

	return id
}

// rowWeight pulls one table row's current weight out of the provider.
func rowWeight(t *testing.T, state map[string]any, name string) float64 {
	t.Helper()

	rows, ok := state["rows"].([]any)
	if !ok {
		t.Fatalf("the provider must report its rows, got %v", state["rows"])
	}

	for _, r := range rows {
		rm, ok := r.(map[string]any)
		if ok && str(rm, "row") == name {
			return num(rm, "weight")
		}
	}

	t.Fatalf("no table row %q in %v", name, rows)

	return 0
}

// firstGroup returns group_list[0], which is ordered.
func firstGroup(t *testing.T, state map[string]any) map[string]any {
	t.Helper()

	groups, ok := state["group_list"].([]any)
	if !ok || len(groups) == 0 {
		t.Fatalf("expected at least one live group, got %v", state["group_list"])
	}

	g, ok := groups[0].(map[string]any)
	if !ok {
		t.Fatalf("group_list[0] is not an object: %v", groups[0])
	}

	return g
}

// awareList pulls the ids of every watcher currently aware of its target.
func awareList(state map[string]any) []string {
	raw, _ := state["notice_aware"].([]any)

	out := make([]string, 0, len(raw))

	for _, v := range raw {
		if id, ok := v.(string); ok {
			out = append(out, id)
		}
	}

	return out
}
