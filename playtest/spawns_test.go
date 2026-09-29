//go:build playtest

package playtest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
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
//
// M4.6 B1 (26 Sep 2026) rides along, because this script already forces the
// arrivals and steps the tables the world save's fields describe. Act 6c reads
// every field B1 added -- each stream's seed against the run's, the scene's
// world draws beside the digest's rng part (one number read two ways, which is
// not a check of the count -- see the act), the tables' clock toward their next check
// against the minutes stepped, one draw per roll at a chance of zero, a forced
// arrival moving the spawner's arrival count, the next group number and the
// stream by amounts the other numbers bound, one light id per torch, the
// screen's bookkeeping against the systems it shadows -- and after act 7 the
// chase reports its last solve. The review of B1 added act 6's member ids (a
// group names every member), values rather than types for the walkers'
// targets and the chase's solve, and the walk at the end, whose destination
// the script chooses.
func TestSpawns(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Gaunt", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	// M4.7 step 3: this script is the TABLES' -- their rows, their rolls,
	// their groups' morale -- and a risen man is none of those (the newest
	// group act 6 inspects was one of the dead). The dead stay down.
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)

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
	//
	// THE RING IS SWEPT WITH THIRTY-TWO BEARINGS, AND THE NUMBER IS MEASURED
	// (19 Sep 2026, BUG-9). This act used ONE ring of 6 tiles and EIGHT
	// bearings, and it went red the day sight started reading the bit D2
	// authored for it, reporting "need one clear and one blocked bearing" -- which
	// names the symptom and hides the fact.
	//
	// The fact, from a throwaway cover census over 32 bearings at nine radii
	// (kept at strigoi-harness-runs\bug11-keep\zz_cover_test.go.keep):
	//
	//	r= 4: 2 blocked of 32 · r= 6: 1 · r= 8: 1 · r=10: 2 · r=12: 2
	//	r=14: 3 · r=16: 3 · r=20: 4 · r=24: 9
	//
	// So cover under the shipped rule is real but SPARSE -- 3% to 9% of bearings
	// inside the notice radius, against roughly a quarter of them under the old
	// walk-flag rule. Eight bearings is a coin toss at that density; thirty-two
	// finds one at every radius measured. The sweep over radii stays as the
	// fallback, and the pair still has to come from a SINGLE radius, which is the
	// control this act rests on.
	//
	// 12 is deliberately NOT in the radius list: the shipped notice radius IS 12,
	// and act 2 needs its seer comfortably INSIDE the radius rather than sitting
	// on the boundary where `distance <= reach` turns on a float.
	rings := []float64{6, 8, 4, 10}

	const bearings = 32

	var ring, clearX, clearY, blockedX, blockedY float64

	var haveClear, haveBlocked bool

	for _, r := range rings {
		haveClear, haveBlocked = false, false

		for i := 0; i < bearings; i++ {
			a := 2 * math.Pi * float64(i) / float64(bearings)
			x, y := px+math.Cos(a)*r, py+math.Sin(a)*r

			path := s.call("strigoi_find_path", map[string]any{"to_x": x, "to_y": y})

			clear := flag(t, path, "straight_line_clear")
			if clear && !haveClear {
				clearX, clearY, haveClear = x, y, true
			}

			if !clear && !haveBlocked {
				blockedX, blockedY, haveBlocked = x, y, true
			}
		}

		if haveClear && haveBlocked {
			ring = r

			break
		}
	}

	if !haveClear || !haveBlocked {
		t.Fatalf("no radius in %v carries BOTH a clear and a sight-blocked bearing out of %d from "+
			"%.1f,%.1f -- under the shipped sight rule cover is sparse (3-9%% of bearings), so suspect "+
			"the spawn point's neighbourhood before the sight model (clear=%t blocked=%t)",
			rings, bearings, px, py, haveClear, haveBlocked)
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

	if d := mustNum(t, blindRow, "distance"); d > mustNum(t, seerRow, "distance")+1.5 {
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

	// M4.7: Night 1's dead lie open from the first dawn, so the carrion
	// multiplier is already lifted when this act begins. Zero it, so the
	// band's lift is measured alone and act 5b's four bodies lift from none.
	setField(s, "spawns", "open_bodies", 0)

	night := spawnsState(s)
	if str(night, "stage") != "night" {
		t.Fatalf("act 5: never reached the deep night, stage=%q", str(night, "stage"))
	}

	if band := mustNum(t, night, "band"); band < 0 {
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

	// THE NEWEST GROUP, NOT group_list[0], AND STEP 5 IS WHY. Until M4.5 step
	// 5 nothing in the game could move a group's morale, so any live group
	// still sat at its authored value and the first one would do. The rout
	// trigger changed that: a group that has lost a member in a fight can be
	// at zero, and this act read one and failed on its own premise. What it
	// is about is a FRESHLY ARRIVED group, so it now asks for one by name.
	group := newestGroup(t, arrived)
	groupID := str(group, "group")

	t.Logf("act 6: %s (%s, code %s) arrived with %d member(s), morale %.0f, %d aware",
		groupID, str(group, "row"), str(group, "code"),
		int(num(group, "members")), num(group, "morale"), int(num(group, "aware")))

	if blocks, ok := group["notice"].([]any); !ok || len(blocks) == 0 {
		t.Fatalf("act 6: every group must carry a notice block per ask 6, got %v", group["notice"])
	}

	// M4.6 B1 review (B2): the group names its members. A fresh arrival is all
	// alive and all watched, so its member ids are exactly the watchers of its
	// notice rows: one row each, no stranger, nobody missing. (TestCombatRout
	// act C is the other half: a member that dies stays on the list.)
	ids := memberIDs(t, group)
	watchers := noticeWatchers(t, group)

	if len(watchers) != len(ids) {
		t.Fatalf("act 6: %s has %d member id(s) %v and %d notice row(s) %v -- a fresh pack is watched whole",
			groupID, len(ids), ids, len(watchers), watchers)
	}

	for _, w := range watchers {
		if !hasString(ids, w) {
			t.Fatalf("act 6: %s's notice row for %s names no member of it: member_ids %v", groupID, w, ids)
		}
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

	if !flag(t, groupByID(t, spawnsState(s), groupID), "routing") {
		t.Fatalf("act 6: morale 5 is under the rout threshold; the group must report routing")
	}

	s.call("strigoi_set_system_field", map[string]any{
		"system": "spawns", "field": "morale",
		"value": map[string]any{"group": groupID, "value": 90},
	})

	if flag(t, groupByID(t, spawnsState(s), groupID), "routing") {
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

	// --- act 6c (M4.6 B1): the save's fields are reported, and move -------
	saveFieldsAct(t, s, seer)

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

		// On the village the arrival stands outside the fence and a building
		// can stand on the line (seed 1462: the smithy hides his start tile
		// from the dogs). Sight is acts 2-4's subject, not this one's, so he
		// steps into the open. He WALKS; nothing below calls strigoi_pursue.
		if len(awareList(spawnsState(s))) == 0 {
			if x, y, ok := firstArrival(spawnsState(s)); ok {
				standInSightOf(t, s, x, y, 40)
			}
		}
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

	checkChaseSolves(t, s) // M4.6 B1

	checkWalkTarget(t, s) // M4.6 B1 review, C3

	// --- act 8 (M4.6 B2b review): a harness removal forgets the man ------
	removeAChasingWatcher(t, s)

	t.Logf("M4.3b: %d table check(s), %d roll(s), %d spawned, %d failure(s); "+
		"%d notice check(s), %d notice(s)",
		int(num(after, "checks")), int(num(after, "rolls")), int(num(after, "spawned")),
		int(num(after, "spawn_failures")), int(num(after, "notice_checks")),
		int(num(after, "notices")))
}

// removeAChasingWatcher is act 8 (M4.6 B2b review): strigoi_remove_entity
// takes a pack member that is watching and chasing the player off the map,
// and he is unwatched and his chase released with him -- as a death does. His
// group keeps him as a member, as a pack keeps its dead.
//
// Before the fix the removal took only the entity. The notice model went on
// watching a man the map no longer had, and every save from then on would be
// refused (Spawns.Snapshot takes a watched member the live world cannot find
// for a living wolf the Resolver lost), as would pursuit's snapshot (a hunter
// that does not resolve).
func removeAChasingWatcher(t *testing.T, s *session) {
	t.Helper()

	pursuit := sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")

	hunter := ""

	for _, raw := range asList(pursuit["chase_list"]) {
		if row, ok := raw.(map[string]any); ok && str(row, "hunter") != "" {
			hunter = str(row, "hunter")

			break
		}
	}

	if hunter == "" {
		t.Fatalf("act 8: act 7 left a chase, so there must be a hunter to remove: %v", pursuit)
	}

	isWatched := func() bool {
		for _, raw := range asList(spawnsState(s)["notice_list"]) {
			if row, ok := raw.(map[string]any); ok && str(row, "watcher") == hunter {
				return true
			}
		}

		return false
	}

	isChasing := func() bool {
		p := sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")
		for _, raw := range asList(p["chase_list"]) {
			if row, ok := raw.(map[string]any); ok && str(row, "hunter") == hunter {
				return true
			}
		}

		return false
	}

	if !isWatched() || !isChasing() {
		t.Fatalf("act 8: the premise -- %s is watched (%v) and chasing (%v)", hunter, isWatched(), isChasing())
	}

	group := ""

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		if g, ok := raw.(map[string]any); ok && hasString(memberIDs(t, g), hunter) {
			group = str(g, "group")
		}
	}

	out := s.call("strigoi_remove_entity", map[string]any{"handle": handleFor(t, s, hunter)})
	if out["removed"] != true || out["unwatched"] != true || out["released"] != true {
		t.Fatalf("act 8: removing a watcher that hunts must unwatch him and release his chase: %v", out)
	}

	// And his body goes with him (M4.6 B3 review, B4): the pack's arrival
	// gave him one, and a body with no entity refuses every save after it.
	if out["body_dropped"] != true {
		t.Fatalf("act 8: removing a pack member must drop the body his arrival gave him: %v", out)
	}

	if isWatched() || isChasing() {
		t.Fatalf("act 8: %s is off the map and still watched (%v) or chasing (%v) -- every save would be refused",
			hunter, isWatched(), isChasing())
	}

	if group != "" && !hasString(memberIDs(t, groupByID(t, spawnsState(s), group)), hunter) {
		t.Fatalf("act 8: his group %s must keep him as a member, as a pack keeps its dead", group)
	}

	t.Logf("act 8 PASS: removed %s -- unwatched, his chase released, still a member of %q", hunter, group)
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

// newestGroup returns the group that arrived most recently, by born_at.
//
// Since M4.5 step 5 a group's morale can be moved by the game, so "any live
// group" is no longer the same thing as "a group at its authored morale".
func newestGroup(t *testing.T, state map[string]any) map[string]any {
	t.Helper()

	groups, ok := state["group_list"].([]any)
	if !ok || len(groups) == 0 {
		t.Fatalf("expected at least one live group, got %v", state["group_list"])
	}

	var best map[string]any

	bornAt := -1.0

	for _, raw := range groups {
		g, ok := raw.(map[string]any)
		if ok && num(g, "born_at") > bornAt {
			best, bornAt = g, num(g, "born_at")
		}
	}

	if best == nil {
		t.Fatalf("no group carried a born_at: %v", groups)
	}

	return best
}

// groupByID finds one group block by its id, so an assertion about a group
// keeps reading the SAME group rather than whichever one sorts first.
func groupByID(t *testing.T, state map[string]any, id string) map[string]any {
	t.Helper()

	groups, _ := state["group_list"].([]any)

	for _, raw := range groups {
		g, ok := raw.(map[string]any)
		if ok && str(g, "group") == id {
			return g
		}
	}

	t.Fatalf("no group %q in %v", id, groups)

	return nil
}

// firstArrival is the birthplace of the first member of the first live group
// (group_list[k].born_where[0]), in world tiles.
func firstArrival(state map[string]any) (x, y float64, ok bool) {
	for _, raw := range asList(state["group_list"]) {
		grp, _ := raw.(map[string]any)

		for _, w := range asList(grp["born_where"]) {
			if at, _ := w.([]any); len(at) == 2 {
				x, _ = at[0].(float64)
				y, _ = at[1].(float64)

				return x, y, true
			}
		}
	}

	return 0, 0, false
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

// --- M4.6 B1: the save's fields, observed ------------------------------------
//
// The world save (M4.6) writes, for every stream, a seed and a draw count, and
// for every system the counters and clocks it cannot resume without. Burst B1
// puts each of them on a provider BEFORE anything saves it, and these helpers
// are where the script proves they are there and move when they should. Every
// assertion compares a number the test chose -- a seed, a chance of zero, a
// number of minutes, one torch -- or a number a second system reports, against
// what the provider says. A field that is present but never moves is a field a
// broken save would not disturb.

// saveBlock reads a nested object strictly: an absent block is a failure that
// names what IS there, never an empty map every read then passes on.
func saveBlock(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()

	v, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("block %q is absent or not an object (%T) -- present: %v", key, m[key], keysOf(m))
	}

	return v
}

func systemState(s *session, name string) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": name}), "state")
}

// arrivalMark is what act 6c reads before a forced arrival.
type arrivalMark struct {
	arrival, nextID, failures, draws, rolls float64
}

func markArrivals(t *testing.T, s *session) arrivalMark {
	t.Helper()

	spawns := spawnsState(s)

	return arrivalMark{
		arrival:  mustNum(t, systemState(s, "scene"), "spawner_arrival"),
		nextID:   mustNum(t, spawns, "next_id"),
		failures: mustNum(t, spawns, "spawn_failures"),
		draws:    mustNum(t, saveBlock(t, spawns, "rng"), "draws"),
		rolls:    mustNum(t, spawns, "rolls"),
	}
}

// checkArrivals: across a forced arrival, the spawner's arrival count, the
// tables' next group number and the stream's draw count all move, and by
// amounts the other numbers bound. Each group the tables adopt spends one
// group number and one arrival; an arrival that placed nobody spends an
// arrival and counts a failure instead. The stream draws one value per roll,
// plus a pack size for every arrival whose row has a range.
func checkArrivals(t *testing.T, s *session, before arrivalMark) {
	t.Helper()

	after := markArrivals(t, s)

	groups := after.nextID - before.nextID
	arrivals := after.arrival - before.arrival
	failures := after.failures - before.failures

	if groups < 1 {
		t.Fatalf("act 6c: a forced arrival must spend a group number; next_id %.0f -> %.0f", before.nextID, after.nextID)
	}

	if arrivals < groups || arrivals > groups+failures {
		t.Fatalf("act 6c: %.0f group(s) and %.0f failure(s) must mean %.0f..%.0f arrival(s); "+
			"the spawner reports %.0f", groups, failures, groups, groups+failures, arrivals)
	}

	if draws, rolls := after.draws-before.draws, after.rolls-before.rolls; draws < rolls || rolls < 1 {
		t.Fatalf("act 6c: %.0f roll(s) must draw at least %.0f value(s); the stream moved %.0f",
			rolls, rolls, draws)
	}

	scene := systemState(s, "scene")
	if n, known := len(asList(scene["bodies"])), mustNum(t, combatState(s), "bodies_known"); float64(n) != known || n == 0 {
		t.Fatalf("act 6c: the arrival's bodies must be on the scene, one per body combat knows; "+
			"scene lists %d, combat knows %.0f", n, known)
	}

	for _, raw := range asList(scene["bodies"]) {
		b, _ := raw.(map[string]any)
		if h, m := mustNum(t, b, "health"), mustNum(t, b, "max_health"); m < 1 || h > m {
			t.Fatalf("act 6c: a body reads %.0f of %.0f health: %v", h, m, b)
		}
	}

	t.Logf("act 6c PASS: %.0f group(s), %.0f arrival(s), %.0f failure(s); %.0f draw(s) for %.0f roll(s); "+
		"%d bodies with their health on the scene",
		groups, arrivals, failures, after.draws-before.draws, after.rolls-before.rolls, len(asList(scene["bodies"])))
}

// saveFieldsAct is act 6c: every field B1 added, present and moving.
func saveFieldsAct(t *testing.T, s *session, seer string) {
	t.Helper()

	// --- the seeds: the number the test chose, and one stream each ---------
	//
	// C6 (M4.6 B2a): the world stream runs on the run's seed itself, and the
	// spawn tables, combat and the rising each on a stream DERIVED from it
	// (d2rand.Derive) -- four different streams, where three used to share
	// one sequence and a restore that swapped two could not be seen.
	const seed = 1462

	spawns := spawnsState(s)
	scene := systemState(s, "scene")
	rising := systemState(s, "rising")

	streams := map[string]struct {
		block map[string]any
		seed  int64
	}{
		"spawns.rng":      {saveBlock(t, spawns, "rng"), d2rand.Derive(seed, d2rand.StreamSpawns)},
		"combat.rng":      {saveBlock(t, combatState(s), "rng"), d2rand.Derive(seed, d2rand.StreamCombat)},
		"scene.world_rng": {saveBlock(t, scene, "world_rng"), seed},
		"rising.rng":      {saveBlock(t, rising, "rng"), d2rand.Derive(seed, d2rand.StreamRising)},
	}

	distinct := map[int64]string{}

	for name, want := range streams {
		// Exactly, as a string: a wall-clock seed does not survive the float64
		// every mustNum goes through (review C9), so a save reads seed_str.
		// These four are below 2^31, so the float reads exactly too.
		if got := mustNum(t, want.block, "seed"); got != float64(want.seed) {
			t.Fatalf("act 6c: %s must be seeded %d from the run's seed %d, reports %.0f", name, want.seed, seed, got)
		}

		if !flag(t, want.block, "present") || mustStr(t, want.block, "seed_str") != strconv.FormatInt(want.seed, 10) {
			t.Fatalf("act 6c: %s must be present with seed_str %q: %v", name, strconv.FormatInt(want.seed, 10), want.block)
		}

		if other, dup := distinct[want.seed]; dup {
			t.Fatalf("act 6c: %s and %s run on one seed %d; every stream has its own", name, other, want.seed)
		}

		distinct[want.seed] = name
	}

	// --- the world stream: the scene reports the digest's number ----------
	//
	// The digest's rng part is sha256("world_draws=N"), a hash no script can
	// read a number from, and the scene reports N readably. Hashing the scene's
	// N must give the digest's part exactly. THIS DOES NOT CHECK THE COUNT
	// (review C2): both read MapEngine.RandDraws, so a miscount agrees with
	// itself here. It checks only that the readable number is the hashed one.
	// The count is checked against the stream itself by d2mapengine's unit
	// tests (TestRestoreRand*: draws through the engine and its factories,
	// judged against a plain stdlib stream), and across two fresh launches at
	// the same steps by TestTownWalkDeterministic.
	draws := mustNum(t, saveBlock(t, systemState(s, "scene"), "world_rng"), "draws")
	sum := sha256.Sum256([]byte(fmt.Sprintf("world_draws=%d", int64(draws))))

	if part := str(sub(s.call("strigoi_get_state_digest", map[string]any{}), "parts"), "rng"); part != hex.EncodeToString(sum[:]) {
		t.Fatalf("act 6c: the scene reports %.0f world draws, but the digest's rng part is not their hash (%.12s)",
			draws, part)
	}

	// --- the tables' clock toward the next check -------------------------
	chance, checkMinutes := mustNum(t, spawns, "chance"), mustNum(t, spawns, "check_minutes")

	setField(s, "spawns", "chance", 0)
	setField(s, "spawns", "check_minutes", 1000)

	before, clockBefore := spawnsState(s), mustNum(t, clockState(s), "world_minutes")
	s.call("strigoi_step_world", map[string]any{"world_minutes": 7})
	after, clockAfter := spawnsState(s), mustNum(t, clockState(s), "world_minutes")

	stepped := clockAfter - clockBefore
	if stepped < 7 {
		t.Fatalf("act 6c: step_world 7 moved the clock %.3f world minutes", stepped)
	}

	if moved := mustNum(t, after, "since_check_minutes") - mustNum(t, before, "since_check_minutes"); math.Abs(moved-stepped) > 1e-6 {
		t.Fatalf("act 6c: the clock ran %.6f world minutes and since_check_minutes moved %.6f", stepped, moved)
	}

	if mustNum(t, after, "checks") != mustNum(t, before, "checks") ||
		mustNum(t, saveBlock(t, after, "rng"), "draws") != mustNum(t, saveBlock(t, before, "rng"), "draws") {
		t.Fatalf("act 6c: short of a 1000-minute check nothing is checked or drawn; checks %.0f -> %.0f, draws %.0f -> %.0f",
			num(before, "checks"), num(after, "checks"),
			num(saveBlock(t, before, "rng"), "draws"), num(saveBlock(t, after, "rng"), "draws"))
	}

	// --- at a chance of zero, one draw per roll, no more and no fewer ------
	setField(s, "spawns", "check_minutes", 1)

	before = spawnsState(s)
	arrivalBefore := mustNum(t, systemState(s, "scene"), "spawner_arrival")
	s.call("strigoi_step_world", map[string]any{"world_minutes": 5})
	after = spawnsState(s)

	rolls := mustNum(t, after, "rolls") - mustNum(t, before, "rolls")
	drawn := mustNum(t, saveBlock(t, after, "rng"), "draws") - mustNum(t, saveBlock(t, before, "rng"), "draws")

	if mustNum(t, after, "checks") <= mustNum(t, before, "checks") || rolls < 1 {
		t.Fatalf("act 6c: five minutes of one-minute checks at night must roll; checks %.0f -> %.0f, rolls +%.0f",
			num(before, "checks"), num(after, "checks"), rolls)
	}

	if drawn != rolls {
		t.Fatalf("act 6c: at a chance of zero every draw is a roll: %.0f roll(s), %.0f draw(s)", rolls, drawn)
	}

	if mustNum(t, after, "next_id") != mustNum(t, before, "next_id") ||
		mustNum(t, systemState(s, "scene"), "spawner_arrival") != arrivalBefore {
		t.Fatalf("act 6c: nothing arrived, so no group number and no arrival may be spent")
	}

	if since := mustNum(t, after, "since_check_minutes"); since < 0 || since >= 1 {
		t.Fatalf("act 6c: with one-minute checks the tables are never a minute from the last: %.4f", since)
	}

	// --- a forced arrival: the arrival count, the group number, the stream --
	//
	// ITS OWN ARRIVAL, NOT ACT 6'S. The first run of this act read across act
	// 6's loop and failed on its premise: that loop only steps while no group
	// is alive, and on seed 1462 two were already out after act 5, so nothing
	// arrived and next_id sat at 3. Here the arrival is forced and waited for,
	// under a raised cap so a full map cannot starve it, and sent home after.
	maxGroups := mustNum(t, after, "max_groups")

	setField(s, "spawns", "max_groups", mustNum(t, after, "groups")+4)
	setField(s, "spawns", "chance", 100)

	mark := markArrivals(t, s)

	for i := 0; i < 10 && mustNum(t, spawnsState(s), "next_id") == mark.nextID; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1})
	}

	checkArrivals(t, s, mark)

	for _, raw := range asList(spawnsState(s)["group_list"]) {
		g, _ := raw.(map[string]any)

		var n float64
		if _, err := fmt.Sscanf(str(g, "group"), "g:%f", &n); err == nil && n >= mark.nextID {
			setField(s, "spawns", "despawn", str(g, "group"))
		}
	}

	setField(s, "spawns", "max_groups", maxGroups)
	setField(s, "spawns", "chance", chance)
	setField(s, "spawns", "check_minutes", checkMinutes)

	t.Logf("act 6c PASS: world seed 1462, spawns %d, combat %d, rising %d (derived); world draws %.0f hash to "+
		"the digest's rng part; since_check moved %.4f in %.4f stepped; %.0f roll(s) drew %.0f",
		streams["spawns.rng"].seed, streams["combat.rng"].seed, streams["rising.rng"].seed,
		draws, stepped, stepped, rolls, drawn)

	// --- one torch, one id -------------------------------------------------
	lit := mustNum(t, lightState(s), "next_id")

	setField(s, "light", "carried_source", "torch")

	if got := mustNum(t, lightState(s), "next_id"); got != lit+1 {
		t.Fatalf("act 6c: lighting one torch spends one light id: %.0f -> %.0f", lit, got)
	}

	setField(s, "light", "carried_source", "")

	if got := mustNum(t, lightState(s), "next_id"); got != lit+1 {
		t.Fatalf("act 6c: putting it out gives no id back: %.0f", got)
	}

	// --- the scene agrees with the systems it shadows ---------------------
	scene = systemState(s, "scene")
	clock := clockState(s)

	if got, want := mustStr(t, scene, "last_stage"), mustStr(t, clock, "stage"); got != want {
		t.Fatalf("act 6c: the screen's last_stage %q must be the clock's stage %q after a frame", got, want)
	}

	if got, want := mustStr(t, systemState(s, "rising"), "last_stage"), mustStr(t, clock, "stage"); got != want {
		t.Fatalf("act 6c: the rising's last_stage %q must be the clock's stage %q", got, want)
	}

	if paid, day := mustNum(t, scene, "dawn_paid_day"), mustNum(t, clock, "day_index"); paid < 0 || paid > day {
		t.Fatalf("act 6c: the dawn paid for (%.0f) cannot be later than today (%.0f)", paid, day)
	}

	flag(t, scene, "watch_clock_set")
	mustNum(t, scene, "watch_clock")

	// Night 1's dead, in the order they were laid: each is a man's body the
	// corpse registry holds, and the registry holds them in the same order.
	laid := asList(scene["field_dead"])
	if len(laid) == 0 {
		t.Fatalf("act 6c: Night 1's dead are laid at the first dawn; field_dead is empty")
	}

	corpses := corpsesState(s)
	order := map[string]int{}

	for i, raw := range asList(corpses["bodies"]) {
		b, _ := raw.(map[string]any)
		order[mustStr(t, b, "id")] = i
		mustNum(t, b, "downed_at")
	}

	last := -1

	for _, raw := range laid {
		id, _ := raw.(string)

		at, ok := order[id]
		if !ok || at <= last {
			t.Fatalf("act 6c: field_dead %v must be bodies the registry holds, in the order they fell", laid)
		}

		last = at
	}

	for _, key := range []string{"risen_as", "walker", "last"} {
		if _, ok := corpses[key].(map[string]any); !ok {
			t.Fatalf("act 6c: corpses must report %q as an object, got %T", key, corpses[key])
		}
	}

	// --- every watch says when it next looks ------------------------------
	reEvaluate := mustNum(t, spawnsState(s), "notice_re_evaluate_minutes")

	// NOT A LOOP OVER NOTHING (review C3), and the first run under the review
	// found it was one: by here every pack has been sent home and nothing is
	// watched, so the loop below passed on an empty list. So the act watches
	// one of its own -- with the radius shrunk so it cannot see the player,
	// which means it notices nothing and starts no chase for act 7 to find --
	// and times its look against the clock.
	radius := mustNum(t, spawnsState(s), "notice_radius")
	setField(s, "spawns", "notice_radius", 0.1)

	pp := s.call("strigoi_get_player", map[string]any{})
	spot := clearNeighbour(t, s, num(pp, "x"), num(pp, "y"))
	looker := spawnNPC(t, s, "fallen1", spot[0], spot[1])

	s.call("strigoi_watch", map[string]any{"watcher": looker, "target": "p:1"})

	row := noticeRowFor(t, s, looker)
	if flag(t, row, "noticed") || mustNum(t, row, "minutes_since_check") != 0 {
		t.Fatalf("act 6c: a watch looks the moment it starts (minutes_since_check 0) and at reach 0.1 sees nothing: %v", row)
	}

	before6c := worldMinutes(t, s)
	s.call("strigoi_step_world", map[string]any{"world_minutes": reEvaluate / 4})
	stepped6c := worldMinutes(t, s) - before6c

	row = noticeRowFor(t, s, looker)
	if since := mustNum(t, row, "minutes_since_check"); stepped6c < reEvaluate && math.Abs(since-stepped6c) > 1e-6 {
		t.Fatalf("act 6c: %.6f world minutes after its look, a watch must say %.6f since its check; says %.6f",
			stepped6c, stepped6c, since)
	}

	// And past one re-evaluation, so the count the loop below bounds has
	// started again: a report of the minutes since it last SAW (which keeps
	// growing for a watch that never sees) would be past the bound here.
	s.call("strigoi_step_world", map[string]any{"world_minutes": reEvaluate})

	notices := asList(spawnsState(s)["notice_list"])
	if len(notices) == 0 {
		t.Fatalf("act 6c: a watch was just started, so notice_list cannot be empty")
	}

	defer func() {
		s.call("strigoi_watch", map[string]any{"watcher": looker, "release": true})
		setField(s, "spawns", "notice_radius", radius)
	}()

	for _, raw := range notices {
		row, _ := raw.(map[string]any)
		if m := mustNum(t, row, "minutes_since_check"); m < 0 || m >= reEvaluate {
			t.Fatalf("act 6c: a watch is always within one re-evaluation (%.2f) of its last look: %v", reEvaluate, row)
		}
	}

	// --- entities say where they are walking -------------------------------
	//
	// Values, not types (review C3): every point is two finite numbers, and the
	// waypoints are as many as path_len, which the entity counts separately.
	// What each point IS is asserted at the end of the script, on a walk whose
	// destination the script chooses (checkWalkTarget).
	for _, handle := range []string{"p:1", seer} {
		state := sub(s.call("strigoi_get_entity", map[string]any{"handle": handle}), "state")

		motionPoint(t, handle+" target", state["target"])

		waypoints, ok := state["waypoints"].([]any)
		if !ok {
			t.Fatalf("act 6c: %s must report its waypoints, got %v", handle, state["waypoints"])
		}

		if n := mustNum(t, state, "path_len"); float64(len(waypoints)) != n {
			t.Fatalf("act 6c: %s reports %d waypoint(s) and path_len %.0f -- one list", handle, len(waypoints), n)
		}

		for i, w := range waypoints {
			motionPoint(t, fmt.Sprintf("%s waypoint %d", handle, i), w)
		}
	}

	t.Logf("act 6c PASS: light ids, the scene's stage and dawn, %d laid dead in fall order, every watch's "+
		"next look, and the walkers' targets are all reported", len(laid))
}

// checkChaseSolves: the chase act 7 started reports its last solve -- where
// the quarry stood and how far the hunter was -- which is what the re-path
// rule reads and what a resumed chase needs.
func checkChaseSolves(t *testing.T, s *session) {
	t.Helper()

	rows := asList(systemState(s, "pursuit")["chase_list"])
	if len(rows) == 0 {
		t.Fatalf("after act 7 (M4.6 B1): a chase must be running to report its solve")
	}

	for _, raw := range rows {
		row, _ := raw.(map[string]any)

		// Values, not presence (review C3): quarry_moved is the provider's own
		// distance from where the quarry stands now to where it stood at the
		// solve, so the solve point the save would carry must be exactly that
		// far from the quarry. A zero, a swapped x and y, or a stale point
		// reads otherwise.
		sx, sy := mustNum(t, row, "solved_at_x"), mustNum(t, row, "solved_at_y")
		qx, qy := mustNum(t, row, "quarry_x"), mustNum(t, row, "quarry_y")

		if moved := mustNum(t, row, "quarry_moved"); math.Abs(math.Hypot(qx-sx, qy-sy)-moved) > 1e-6 {
			t.Fatalf("after act 7 (M4.6 B1): the solve point (%.3f,%.3f) is %.4f from the quarry at (%.3f,%.3f), "+
				"and quarry_moved says %.4f: %v", sx, sy, math.Hypot(qx-sx, qy-sy), qx, qy, moved, row)
		}

		if mustNum(t, row, "solves") >= 1 && mustNum(t, row, "solved_distance") <= 0 {
			t.Fatalf("after act 7 (M4.6 B1): a chase that solved from a distance reports solved_distance %.3f: %v",
				num(row, "solved_distance"), row)
		}
	}

	t.Logf("after act 7 (M4.6 B1) PASS: %d chase(s) report where and how far they last solved", len(rows))
}

// --- M4.6 B1 review ----------------------------------------------------------

// memberIDs reads a group's member_ids strictly: a list of distinct non-empty
// strings, as many as the group reports members -- and as it was spawned,
// because a pack's list is never shortened (a death unwatches, it does not
// remove).
func memberIDs(t *testing.T, group map[string]any) []string {
	t.Helper()

	raw, ok := group["member_ids"].([]any)
	if !ok {
		t.Fatalf("group %s must report member_ids as a list, got %T -- present: %v",
			str(group, "group"), group["member_ids"], keysOf(group))
	}

	ids := make([]string, 0, len(raw))

	for _, v := range raw {
		id, _ := v.(string)
		if id == "" || hasString(ids, id) {
			t.Fatalf("group %s: member_ids %v holds an empty or repeated id", str(group, "group"), raw)
		}

		ids = append(ids, id)
	}

	if n, spawned := mustNum(t, group, "members"), mustNum(t, group, "spawned"); float64(len(ids)) != n || n != spawned {
		t.Fatalf("group %s: %d member id(s), members %.0f, spawned %.0f -- one list, never shortened",
			str(group, "group"), len(ids), n, spawned)
	}

	return ids
}

// noticeWatchers is the watcher of each of a group's notice rows.
func noticeWatchers(t *testing.T, group map[string]any) []string {
	t.Helper()

	out := []string{}

	for _, raw := range asList(group["notice"]) {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("group %s: a notice row is %T", str(group, "group"), raw)
		}

		out = append(out, mustStr(t, row, "watcher"))
	}

	return out
}

func hasString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}

	return false
}

// motionPoint reads one [x, y] of an entity's motion strictly: two finite
// numbers, in world tiles.
func motionPoint(t *testing.T, what string, v any) (x, y float64) {
	t.Helper()

	pt, _ := v.([]any)
	if len(pt) != 2 {
		t.Fatalf("%s must be [x, y], got %v", what, v)
	}

	x, okX := pt[0].(float64)
	y, okY := pt[1].(float64)

	if !okX || !okY || math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		t.Fatalf("%s must be two finite numbers, got %v", what, v)
	}

	return x, y
}

// checkWalkTarget (review C3): an entity's target and waypoints are VALUES a
// script can predict, on a walk whose destination it chooses. Every frame of
// the walk: the waypoints are as many as path_len, the target is ahead of him
// while any are left, and the walk's last point (its last waypoint, or its
// target when none are left) is the destination. Arrived, he stands on his
// target with nothing ahead. At the end of the script, so the walk moves
// nothing an act reads.
//
// THE DESTINATION IS CHOSEN SO THE PATH HAS WAYPOINTS, and the second run of
// this check is why. It first walked four tiles in a straight line, which the
// pathfinder answers with the destination as the target and NO waypoints --
// so a build that reported every waypoint list empty passed it (the dropped-
// waypoints negative control, 27 Sep). So it asks strigoi_find_path for a
// reachable point whose route bends (three or more points), walks there, and
// fails unless some frame of some walk had a waypoint ahead to check.
func checkWalkTarget(t *testing.T, s *session) {
	t.Helper()

	p := s.call("strigoi_get_player", map[string]any{})
	px0, py0 := num(p, "x"), num(p, "y")

	type goal struct{ x, y float64 }

	var goals []goal

	for _, r := range []float64{5, 7, 4, 9} {
		for i := 0; i < 16 && len(goals) < 6; i++ {
			a := 2 * math.Pi * float64(i) / 16
			x, y := px0+math.Cos(a)*r, py0+math.Sin(a)*r

			route := s.call("strigoi_find_path", map[string]any{"to_x": x, "to_y": y})
			if flag(t, route, "reachable") && mustNum(t, route, "waypoint_count") >= 3 {
				goals = append(goals, goal{x, y})
			}
		}
	}

	if len(goals) == 0 {
		t.Fatalf("walk (M4.6 B1 review): no reachable point 4-9 tiles from (%.2f,%.2f) has a route that bends", px0, py0)
	}

	sawWaypoints, arrived := 0, false

	for _, g := range goals {
		s.call("strigoi_move_player_to", map[string]any{"x": g.x, "y": g.y})

		frames, mostAhead := 0, 0

		for i := 0; i < 240; i++ {
			s.call("strigoi_step", map[string]any{"frames": 1})

			ent := s.call("strigoi_get_entity", map[string]any{"handle": "p:1"})
			state := sub(ent, "state")
			tx, ty := motionPoint(t, "p:1 target", state["target"])
			px, py := num(ent, "x"), num(ent, "y")
			pathLen := mustNum(t, state, "path_len")

			if pathLen == 0 && math.Hypot(tx-px, ty-py) <= 0.01 {
				if frames > 0 || i >= 20 {
					break // standing: arrived, or never started
				}

				continue // the move has not reached him yet
			}

			frames++

			waypoints := asList(state["waypoints"])
			if float64(len(waypoints)) != pathLen {
				t.Fatalf("walk (M4.6 B1 review) frame %d: %d waypoint(s) and path_len %.0f -- one list",
					i, len(waypoints), pathLen)
			}

			lastX, lastY := tx, ty

			if len(waypoints) > 0 {
				mostAhead = max(mostAhead, len(waypoints))

				if math.Hypot(tx-px, ty-py) <= 0.01 {
					t.Fatalf("walk (M4.6 B1 review) frame %d: %d waypoint(s) ahead and his target (%.3f,%.3f) "+
						"is where he stands -- the target is the point he steps toward", i, len(waypoints), tx, ty)
				}

				for k, w := range waypoints {
					motionPoint(t, fmt.Sprintf("p:1 waypoint %d", k), w)
				}

				lastX, lastY = motionPoint(t, "p:1 last waypoint", waypoints[len(waypoints)-1])
			}

			if dist := math.Hypot(lastX-g.x, lastY-g.y); dist > 1.5 {
				t.Fatalf("walk (M4.6 B1 review) frame %d: sent to (%.2f,%.2f), the walk ends at (%.2f,%.2f), "+
					"%.2f tiles off; target %v waypoints %v", i, g.x, g.y, lastX, lastY, dist, state["target"], waypoints)
			}
		}

		if mostAhead > 0 {
			sawWaypoints++
		}

		res := s.call("strigoi_move_player_to", map[string]any{"x": g.x, "y": g.y, "wait": true, "max_ticks": 900})
		if str(res, "outcome") != "arrived" {
			t.Logf("walk (M4.6 B1 review): to (%.2f,%.2f) ended %q after %d walking frame(s)", g.x, g.y,
				str(res, "outcome"), frames)

			continue
		}

		ent := s.call("strigoi_get_entity", map[string]any{"handle": "p:1"})
		state := sub(ent, "state")
		ax, ay := motionPoint(t, "p:1 target (arrived)", state["target"])

		if math.Hypot(ax-num(ent, "x"), ay-num(ent, "y")) > 0.01 || len(asList(state["waypoints"])) != 0 ||
			mustNum(t, state, "path_len") != 0 {
			t.Fatalf("walk (M4.6 B1 review): arrived at (%.2f,%.2f), he must stand on his target with nothing "+
				"ahead; target (%.2f,%.2f) waypoints %v", num(ent, "x"), num(ent, "y"), ax, ay, state["waypoints"])
		}

		arrived = true

		t.Logf("walk (M4.6 B1 review): to (%.2f,%.2f), %d walking frame(s) with up to %d waypoint(s) ahead, "+
			"every one ending at the destination; arrived on his target", g.x, g.y, frames, mostAhead)

		if sawWaypoints > 0 {
			break
		}
	}

	if sawWaypoints == 0 || !arrived {
		t.Fatalf("walk (M4.6 B1 review): of %d walk(s), %d had a waypoint ahead and arrived=%t -- both halves "+
			"must have been checked", len(goals), sawWaypoints, arrived)
	}

	t.Logf("walk (M4.6 B1 review) PASS: waypoints checked on %d walk(s), and an arrival", sawWaypoints)
}
