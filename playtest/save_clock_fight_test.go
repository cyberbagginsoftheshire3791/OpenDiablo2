//go:build playtest

package playtest

// The merge scout's playtests (29 Sep 2026): M4.6 B4b ("a hunted night
// resumes") merged with the raid's R1 ("fights he is not in"). Neither branch
// could run them: R1's Combat.Restore of a live clock fight was unit-tested
// only, because on its branch B4a still refused a hunted file, and B4b was
// built without R1. A fight he is not in never stops a save AS A FIGHT (the
// raid's Q5 (a)), so the save must resume one too.
//
// Brought into the real merge (merge-b4b-r1, 29 Sep 2026) against the FIXED
// branches, with two changes the fixes asked for:
//
//   - N1 AND N3 MUST BE REFUSED. combat.clock missing is refused FILE by
//     d2save's shape check (B3-8); the two clock fights' quarries swapped is
//     refused BLOCK by d2world.CheckClockWatches (BUG-73, the load's step 4),
//     where the scout's run found it resumed and divergent.
//   - AT THE SHIPPED ROUND, SAVED ON THE FIRST TRY (BUG-87 fixed, branch
//     save-held, 29 Sep 2026). The merge found that a clock fight's blows
//     play a swing and a flinch on its entities that outlast its round, and
//     B4b's review fixes refused a save while any was held (BUG-76) -- from a
//     clock fight's first blow to its last death, no frame took a save
//     (92-93% of its frames refused; strigoi-harness-runs\wt-merge\
//     held-summary.txt). So the merge's scripts slowed the round
//     (round_minutes 3 at night, 4 at dawn) and retried the save a frame at a
//     time. BUG-87's fix carries a held action in the file at its frame and
//     drops that refusal, so both steers are gone: the round is the shipped
//     one (scShippedRoundMinutes, required of the combat provider), the save
//     at T is asked once and must be made (scSaveAtT: no retry), and T's
//     file must hold at least one action still playing -- a swing, a blow
//     taken or a death, saved at its frame and resumed there.
//
//	TestSaveResumeWithAFightHeIsNotIn  the hunted night: a monster watching
//	                                   him (not yet seeing him: the combat
//	                                   status) and a clock fight live at T,
//	                                   two hours on, past the clock fight's end
//	TestSaveResumeAFightHeIsNotIn      the quick one: two clock fights in
//	                                   the first dawn, three rounds on, and
//	                                   the file-edit controls
//	TestSaveResumeMidAction            BUG-87's own: an NPC saved mid-swing,
//	                                   mid-flinch and mid-death, resumed there
//	TestSaveResumeAHeldActionOutOfReach the BUG-87 review's B1 and B2: a
//	                                   held action the load cannot resume --
//	                                   corrupt, refused; the art changed, ended
//
// The source-mutation controls, the conflicts of the merge and every run's
// log are in strigoi-harness-runs\wt-merge-scout\ (conflicts.md,
// merge-notes.md) for the scout's runs, and in
// strigoi-harness-runs\wt-merge\ for the real merge's.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
)

// scNightDials are the dials the hunted night leaves set at T: no table, the
// dead stay down, and his fights on the POLICY (start_game's own default for a
// script). His chaser is a zombie1, which no quick resolve finishes, and a
// human fight's open turn freezes the world clock (step_world refuses
// AWAITING_PLAYER), so his fight is played on world time as a clock fight is --
// both drivers live after T, and his dice are compared too. A dial is never
// saved (trap 7): a relaunch sets them again, and "load last save" has them
// re-applied by the load. The round is the shipped one: the merge slowed it
// to 3 minutes so a frame came on which no blow's action was held, and BUG-87
// made that unnecessary (the header).
var scNightDials = []dialWrite{
	{"spawns", "chance", 0},
	{"rising", "p", 0.0},
	{"rising", "edge_floor", 0},
	{"combat", "player_control", "policy"},
}

// scShippedRoundMinutes is the shipped combat round (the dial round_minutes;
// d2world's defaults): a round every world minute, 24 frames at night and 15
// at dawn. A round's blows hold a swing and a flinch 38 frames and a death 56
// (measured, BUG-87), so the holds overlap and a clock fight holds an action
// on nearly every frame from its first blow to its last death -- which is why
// T's file holds one. scShippedPace requires the provider to report it, so
// nothing steers the pace these scripts save at.
const scShippedRoundMinutes = 1.0

// scShippedPace requires the combat round to be the shipped one.
func scShippedPace(t *testing.T, s *session, act string) {
	t.Helper()

	if got := mustNum(t, combatState(s), "round_minutes"); got != scShippedRoundMinutes {
		t.Fatalf("%s: the round is %v world minutes, not the shipped %v: these scripts save at the shipped pace (BUG-87)",
			act, got, scShippedRoundMinutes)
	}
}

// The two hours after T, in the steps both sides of the comparison take from
// the same moment: at least scMinChunks steps of scChunk world minutes, and
// on (to scMaxChunks) until the clock fight has ended.
const (
	scChunk     = 10.0
	scMinChunks = 12
	scMaxChunks = 24
)

// TestSaveResumeWithAFightHeIsNotIn is the merge scout's integration test: A
// HUNTED NIGHT WITH A LIVE CLOCK FIGHT, SAVED AND RESUMED.
//
//	V0  Seed 1462, a new game, stepped an hour at a time to true dark (fed
//	    and watered each hour, as the sleeper is).
//	V1  A zombie1 seven to ten tiles off on a clear line watches him; a
//	    fallen1 on a fallen1 fifteen tiles off the other way (both 61 HP). T
//	    is the moment before the zombie sees him, with the clock fight live:
//	    its quarry hurt and alive, its monster alive, the combat-clock stream
//	    drawn from, and none of it his. THE COMBAT STATUS MOVED T (30 Sep
//	    2026): it was the first frames of the zombie's chase of him, and a
//	    hostile chasing him is combat, in which a save is refused. So until T
//	    the notice radius is two tiles -- the clock fight's monster, a tile
//	    from its quarry, sees it; the zombie, seven or more off, does not --
//	    and at T it is the shipped one again (before the save, no frame
//	    between), so the zombie sees him and gives chase in the frames after
//	    T, in both games alike. The clock fight's two bodies watch
//	    and chase nothing but each other (a monster in both fights is the R1
//	    review's A1, fixed on its own; this script keeps out of it). He
//	    stands still; the round is the shipped one; the save is never
//	    refused as a fight (Q5 (a)), nor while a blow's action plays
//	    (BUG-87): it is made on the first try (scSaveAtT, no retry) and
//	    moves nothing; S_T; the file is hunted (a watch on him and no chase of
//	    him, the entities the map does not build), holds the clock fight in
//	    combat.clock.live, rng.combat_clock = combat.clock.rng, and holds at
//	    least one action still playing, at its frame (scHeldInFile).
//	V2  Two hours in ten-minute steps, and on until the clock fight has
//	    ended (at least past it): his chaser comes and his fight is played
//	    (on the policy), the clock fight ends; S_U, the ending.
//	V3  A new process, the same home: start_game{save_path}, no seed --
//	    resumed; S_R0 = S_T (the resume digest, and the clock block whole:
//	    the fight's c:<n> id and round, next_id, the combat-clock draws).
//	V4  The same steps: S_R = S_U, the clock block U's -- it ended the same
//	    way, on the same round, on the same draws.
//	V5  THE IN-PROCESS PATH: he dies, and "load last save" resumes T: S =
//	    S_T, and the same steps on, S_U.
//	V6  THE NEGATIVE CONTROL in the script: T's file with combat.clock
//	    dropped (strigoi_save_game's omit takes only top-level blocks, so the
//	    file is edited), his .od2 and sidecar of T beside it: REFUSED, FILE
//	    (d2save's shape check, B3-8) -- never resumed.
//
// The source-mutation control (the combat-clock stream not restored on load:
// red) is run and reverted by hand; its logs are in
// strigoi-harness-runs\wt-merge-scout\ and strigoi-harness-runs\wt-merge\.
func TestSaveResumeWithAFightHeIsNotIn(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Hunted", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	save := str(game, "save_path")

	for _, d := range scNightDials {
		setField(s, d.System, d.Field, d.Value)
	}

	scShippedPace(t, s, "V0")

	// The combat status: the notice radius two tiles until T (V1), the shipped
	// one after.
	shippedRadius := mustNum(t, spawnsState(s), "notice_radius")

	// --- V0: to true dark -----------------------------------------------------
	for i := 0; i < 24 && str(clockState(s), "stage") != "night"; i++ {
		if e := afhStepWorld(s, 60.0); e != "" {
			t.Fatalf("V0: step_world refused on the way to the night: %s", e)
		}

		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
	}

	if str(clockState(s), "stage") != "night" {
		t.Fatalf("V0: no night: %v", clockState(s))
	}

	p := s.call("strigoi_get_player", map[string]any{})
	playerID, px, py := str(p, "id"), num(p, "x"), num(p, "y")

	// --- V1: his chaser, and the fight he is not in ---------------------------
	// The chaser a zombie1 ten tiles or less from him on a clear line (the
	// table's pack was the first form: it landed where it never saw him, S1's
	// first run, pt-sc-1); the clock fight a fallen1 on a fallen1 fifteen tiles
	// off the other way (61 HP each: the fight lasts some six rounds, measured).
	// The clock fight's monster watches its quarry and nothing else.
	setField(s, "spawns", "notice_radius", 2.0)

	spot := scClearSpot(t, s, px, py)
	chaser := spawnNPC(t, s, "zombie1", spot[0], spot[1])
	chaserID := entityID(t, s, chaser)
	s.call("strigoi_watch", map[string]any{"watcher": chaser, "target": str(p, "handle")})

	quarry, monster := spawnNPC(t, s, "fallen1", px+15, py), spawnNPC(t, s, "fallen1", px+16, py)
	quarryID, monsterID := entityID(t, s, quarry), entityID(t, s, monster)
	s.call("strigoi_watch", map[string]any{"watcher": monster, "target": quarry})

	frames := 0
	for ; frames < 900 && !scReadyAtT(t, s, chaserID, playerID, quarryID); frames += 2 {
		if flag(t, combatState(s), "fighting") {
			t.Fatalf("V1: the chaser %s reached him after %d frames, before the clock fight had drawn blood: clock %s",
				chaserID, frames, cut(afhClockJSON(t, s)))
		}

		afhFrames(s, 2)
	}

	f := afhFightFor(s, quarryID)
	if f == nil || !scReadyAtT(t, s, chaserID, playerID, quarryID) || flag(t, combatState(s), "fighting") {
		t.Fatalf("V1: no T in %d frames -- a clock fight live with its quarry hurt, the chaser %s watching him and not chasing him, no fight of his: fighting %v, chases %v, clock %s",
			frames, chaserID, combatState(s)["fighting"], scChases(s), cut(afhClockJSON(t, s)))
	}

	// The shipped radius again: the zombie will see him in the frames after T.
	setField(s, "spawns", "notice_radius", shippedRadius)

	// --- V1: the save at T ------------------------------------------------------
	// Asked once, and made (BUG-87): a blow's action still playing is saved
	// at its frame, so nothing refuses it and nothing is retried.
	out := scSaveAtT(t, s, "V1", func() string {
		if flag(t, combatState(s), "fighting") {
			return "his own fight began"
		}

		if !scReadyAtT(t, s, chaserID, playerID, quarryID) {
			return fmt.Sprintf("T's condition failed: chases %v, clock %s", scChases(s), cut(afhClockJSON(t, s)))
		}

		return ""
	})

	f = afhFightFor(s, quarryID)
	if f == nil {
		t.Fatalf("V1: the clock fight on %s is live at T: %s", quarryID, cut(afhClockJSON(t, s)))
	}

	scOnlyEachOther(t, s, "V1", playerID, quarryID, monsterID)

	if c := combatState(s); str(c, "encounter") != "" || flag(t, c, "world_held") {
		t.Fatalf("V1: a fight he is not in took his slot: %v", c)
	}

	for _, raw := range asList(f["enemies"]) {
		if en, _ := raw.(map[string]any); str(en, "id") != monsterID || num(en, "health") <= 0 {
			t.Fatalf("V1: the clock fight's enemy is the living monster %s: %v", monsterID, f["enemies"])
		}
	}

	clockAtT := afhClock(s)
	fightT := str(f, "id")

	// S_T is taken after the save: scSaveAtT held the whole digest before and
	// after it to one value, so the world it reads is the saved one.
	sT, clockT := snapWorld(t, s), afhClockJSON(t, s)

	worldPath := str(out, "world_path")
	fileT, od2T, sidecarT := mustRead(t, worldPath), mustRead(t, save), mustRead(t, save+".strigoi.json")
	file := worldFile(t, "V1", fileT)

	for _, block := range []string{"notice.watches", "pursuit.chases", "entities"} {
		if len(listAt(t, file, block)) == 0 {
			t.Fatalf("V1: a hunted night's file has %s: none", block)
		}
	}

	// A watch or chase on him names him "player" in the file (B2b's Resolver).
	onHim := func(id string) bool { return id == "player" || id == playerID }
	hisWatchers, hisChases := 0, 0

	for _, raw := range listAt(t, file, "notice.watches") {
		if w, _ := raw.(map[string]any); onHim(str(w, "target")) {
			hisWatchers++
		}
	}

	for _, raw := range listAt(t, file, "pursuit.chases") {
		if c, _ := raw.(map[string]any); onHim(str(c, "quarry")) {
			hisChases++
		}
	}

	live := asList(sub(sub(file, "combat"), "clock")["live"])
	if len(live) != 1 {
		t.Fatalf("V1: the file holds %d live clock fights, want 1: %v", len(live), live)
	}

	if l, _ := live[0].(map[string]any); str(l, "id") != fightT || str(l, "quarry") != quarryID {
		t.Fatalf("V1: the saved clock fight is %v, want %s on %s", l, fightT, quarryID)
	}

	w, err := d2save.Decode(fileT)
	if err != nil {
		t.Fatalf("V1: the file does not decode: %v", err)
	}

	if w.Version != d2save.Version || w.RNG.CombatClock != w.Combat.Clock.RNG || w.RNG.CombatClock.Draws == 0 {
		t.Fatalf("V1: version %d, rng.combat_clock %+v, combat.clock.rng %+v", w.Version, w.RNG.CombatClock, w.Combat.Clock.RNG)
	}

	if hisWatchers == 0 || hisChases != 0 {
		t.Fatalf("V1: the file is hunted -- a watch on him -- and holds no chase of him (the combat status: a save is refused while one runs): %d watch(es) and %d chase(s) on him (%s): %v; %v",
			hisWatchers, hisChases, playerID, listAt(t, file, "notice.watches"), listAt(t, file, "pursuit.chases"))
	}

	heldT := scHeldInFile(t, "V1", w)
	t.Logf("V1: T's file holds %d action(s) still playing, each at its frame: %v", len(heldT), heldT)

	t.Logf("V1 PASS: saved at %s (%s) after %d frames, on the first try, with %s watching him, not yet seeing him (%d watch(es) and %d chase(s) on him in the file) and the clock fight %s live on %s (round %v, quarry %v of %v); clock next_id %v, combat-clock %+v; the digest unmoved",
		str(clockState(s), "time_of_day"), str(clockState(s), "stage"), frames, chaserID, hisWatchers, hisChases, fightT, quarryID,
		f["round"], f["quarry_health"], f["quarry_max_health"], clockAtT["next_id"], w.RNG.CombatClock)

	// --- V2: two hours, and past the clock fight's end; S_U; quit --------------
	chunks := scStepPast(t, s, "V2", quarryID, playerID, monsterID)

	if afhFightFor(s, quarryID) != nil {
		t.Fatalf("V2: the clock fight outlived %d minutes: %s", chunks*int(scChunk), cut(afhClockJSON(t, s)))
	}

	if flag(t, uiState(s), "death_open") || mustNum(t, metersState(s), "health") <= 0 {
		t.Fatalf("V2: he lives through the hours after T: %v", combatState(s))
	}

	sU, clockU := snapWorld(t, s), afhClockJSON(t, s)
	endU := scEnding(afhClock(s))

	if sU.Resume == sT.Resume || clockU == clockT {
		t.Fatal("V2: the hours moved nothing, so V4 would compare nothing")
	}

	t.Logf("V2: %d steps of %.0f minutes, to %s; his fights %v (the last %q); the clock fight %s ended: %s",
		chunks, scChunk, str(clockState(s), "time_of_day"), combatState(s)["encounters"], str(combatState(s), "ended_reason"), fightT, endU)
	s.stop()

	// --- V3: relaunch, load, S_R0 = S_T -------------------------------------------
	s = start(t)
	s.call("strigoi_pause", map[string]any{})

	g := s.call("strigoi_start_game", map[string]any{"save_path": save, "wait_seconds": 90})
	if load := sub(g, "load"); !flag(t, load, "resumed") {
		t.Fatalf("V3: start_game resumes the hunted night with a clock fight live: %v", load)
	}

	for _, d := range scNightDials {
		setField(s, d.System, d.Field, d.Value)
	}

	sameClock(t, "V3 (the clock block at R0 = T)", clockT, afhClockJSON(t, s))
	scSameFight(t, "V3", clockAtT, afhClock(s), quarryID)
	sameWorld(t, "V3 (S_R0 = S_T)", sT, snapWorld(t, s))
	t.Logf("V3 PASS: relaunched and resumed; S_R0 = S_T (resume digest %.12s); %s on %s at round %v, next_id %v, combat-clock draws %v",
		sT.Resume, fightT, quarryID, f["round"], clockAtT["next_id"], sub(clockAtT, "rng")["draws"])

	// --- V4: the same steps: S_R = S_U --------------------------------------------
	scStepChunks(t, s, "V4", chunks)
	sameClock(t, "V4 (the clock block at R = U)", clockU, afhClockJSON(t, s))

	if end := scEnding(afhClock(s)); end != endU {
		t.Errorf("RED V4: the clock fight ended otherwise on the resumed side: %s, want %s", end, endU)
	}

	sameWorld(t, "V4 (S_R = S_U)", sU, snapWorld(t, s))
	t.Logf("V4 PASS: S_R = S_U (resume digest %.12s); the clock fight ended the same way: %s", sU.Resume, endU)

	// --- V5: in process: he dies, and "load last save" resumes T ---------------
	setField(s, "meters", "health", 0.0)
	s.call("strigoi_step", map[string]any{"frames": 3})

	if !flag(t, uiState(s), "death_open") {
		t.Fatalf("V5: at 0 health the death screen is up: %v", uiState(s))
	}

	s.call("strigoi_key", map[string]any{"key": "enter"})

	if load := awaitGame(t, s, "V5"); !flag(t, load, "resumed") {
		t.Fatalf("V5: load last save resumes the world file, not the dawn: %v", load)
	}

	sameClock(t, "V5 (in process: the clock block T's)", clockT, afhClockJSON(t, s))
	sameWorld(t, "V5 (in process: S = S_T)", sT, snapWorld(t, s))

	scStepChunks(t, s, "V5", chunks)
	sameClock(t, "V5 (in process: the clock block U's)", clockU, afhClockJSON(t, s))
	sameWorld(t, "V5 (in process: the same steps on, S_U)", sU, snapWorld(t, s))
	t.Log("V5 PASS: load last save resumed T in this process, and the same steps on it is U")

	// --- V6: the negative control: combat.clock dropped from T's file ----------
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	awaitMenu(t, s)

	mutated := reindent(t, fileT, encodeFields(editField(t, decodeFields(t, fileT), "combat", func(combat json.RawMessage) json.RawMessage {
		return encodeFields(dropField(t, decodeFields(t, combat), "clock"))
	})))

	for path, data := range map[string][]byte{worldPath: mutated, save: od2T, save + ".strigoi.json": sidecarT} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("V6: %v", err)
		}
	}

	s.call("strigoi_start_game", map[string]any{"save_path": save, "seed": 1462, "wait_seconds": 90})
	load := awaitGame(t, s, "V6")

	if code, why := str(load, "refused"), str(load, "reason"); code != "FILE" || !strings.Contains(why, "combat.clock") {
		t.Errorf("RED V6 (N1, combat.clock dropped): the load must be refused FILE naming combat.clock; got %q (%s), resumed %v",
			code, cut(why), load["resumed"])
	} else {
		t.Logf("V6 PASS (N1, combat.clock dropped): refused %s (%s)", code, cut(why))
	}
}

// scSaveAtT is the save at T. Since BUG-87 no action a monster or villager
// is still playing refuses it -- a swing, a blow taken or a death is written
// at its frame and resumed there -- so at the shipped round it is made on the
// FIRST try, and any refusal is RED, here: it is asked once and never
// retried. (The merge's scSaveSettled retried a frame at a time while one was
// held, and needed 23 retries in each script with the round slowed.) stillT
// is T's own condition, asked first ("" while it holds). The save must move
// nothing: the whole digest before and after is one value (saveUnmoved's
// rule). It returns the save's result.
//
// (It returned a retry count too, which the scripts required to be 0; it was
// 0 by construction, so that check could not go red -- the BUG-87 review, its
// C2. The teeth are the Fatalf on a refusal, below.)
func scSaveAtT(t *testing.T, s *session, act string, stillT func() string) map[string]any {
	t.Helper()

	if why := stillT(); why != "" {
		t.Fatalf("%s: T does not hold: %s", act, why)
	}

	d0, parts0 := digest(s)

	out, e := scCall(s, "strigoi_save_game", map[string]any{})
	if e != "" {
		t.Fatalf("RED %s: the save at T was refused -- at the shipped round it is made on the first try, and never retried (BUG-87): %s", act, e)
	}

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

	t.Logf("%s: the save was made on the first try (no retry)", act)

	return out
}

// scHeldInFile is every action the file holds still playing, "id action at
// frame f + e s", and requires at least one: T is saved mid-action (BUG-87).
func scHeldInFile(t *testing.T, act string, w *d2save.World) []string {
	t.Helper()

	var held []string

	for _, e := range w.Entities {
		if e.Motion.Action == "" {
			continue
		}

		if e.Motion.ActionAt == nil {
			t.Fatalf("%s: %s holds %s with no frame", act, e.ID, e.Motion.Action)
		}

		held = append(held, fmt.Sprintf("%s %s at frame %d + %.4g s", cut8(e.ID), e.Motion.Action, e.Motion.ActionAt.Frame, e.Motion.ActionAt.Elapsed))
	}

	if len(held) == 0 {
		t.Fatalf("%s: T's file holds no action still playing, so the save at the shipped round proved nothing BUG-87 is about", act)
	}

	return held
}

// cut8 is an id's first eight characters.
func cut8(id string) string {
	if len(id) > 8 {
		return id[:8]
	}

	return id
}

// scCall invokes a tool and returns its result, or the tool error's text (the
// structured result of s.call and the refusal of s.callErr in one call: the
// save's world_path when it is made, its reason when it is refused).
func scCall(s *session, name string, args map[string]any) (map[string]any, string) {
	s.t.Helper()
	s.refuseRealSaves(name, args)

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	res, err := s.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		s.t.Fatalf("%s: transport error: %v\n--- game output (tail) ---\n%s", name, err, s.gameTail(40))
	}

	if res.IsError {
		return nil, contentText(res)
	}

	out := map[string]any{}

	if res.StructuredContent != nil {
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			s.t.Fatalf("%s: structured content: %v", name, err)
		}

		if err := json.Unmarshal(raw, &out); err != nil {
			s.t.Fatalf("%s: structured content decode: %v", name, err)
		}
	}

	s.t.Logf("%s -> %s", name, contentText(res))

	return out, ""
}

// scReadyAtT is T's condition: the clock fight on quarry live and its quarry
// hurt and alive, the combat-clock stream drawn from, and the chaser watching
// him and nothing chasing him (it was "the chaser chasing him" until the
// combat status, 30 Sep 2026, made that moment one no save can be made at).
func scReadyAtT(t *testing.T, s *session, chaser, player, quarry string) bool {
	t.Helper()

	f := afhFightFor(s, quarry)
	if f == nil {
		return false
	}

	h, m := num(f, "quarry_health"), num(f, "quarry_max_health")
	if h <= 0 || h >= m || num(sub(afhClock(s), "rng"), "draws") <= 0 {
		return false
	}

	// The combat status (30 Sep 2026): the chaser watches him and does not
	// yet chase him -- a chase of him is combat, and T must be savable.
	for _, raw := range scChases(s) {
		if c, _ := raw.(map[string]any); str(c, "quarry") == player {
			return false
		}
	}

	for _, raw := range asList(spawnsState(s)["notice_list"]) {
		if w, _ := raw.(map[string]any); str(w, "watcher") == chaser && str(w, "quarry") == player {
			return true
		}
	}

	return false
}

// scChases is the pursuit provider's chase list.
func scChases(s *session) []any {
	return asList(sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")["chase_list"])
}

// scClearSpot is where his chaser stands: seven to ten tiles from him, away
// from the clock fight (which is east), on a clear straight line.
func scClearSpot(t *testing.T, s *session, px, py float64) [2]float64 {
	t.Helper()

	for _, d := range [][2]float64{{-10, 0}, {0, 10}, {0, -10}, {-7, -7}, {-7, 7}, {-8, 0}, {0, 8}, {0, -8}, {-5, -5}, {-5, 5}} {
		x, y := px+d[0], py+d[1]
		if flag(t, s.call("strigoi_find_path", map[string]any{"to_x": x, "to_y": y}), "straight_line_clear") {
			return [2]float64{x, y}
		}
	}

	t.Fatalf("no spot seven to ten tiles from %.2f,%.2f on a clear line for his chaser", px, py)

	return [2]float64{}
}

// scOnlyEachOther is the A1 guard: the clock fight's two bodies watch and
// chase nothing but each other -- never him.
func scOnlyEachOther(t *testing.T, s *session, act, player string, pair ...string) {
	t.Helper()

	in := map[string]bool{}
	for _, id := range pair {
		in[id] = true
	}

	for _, raw := range asList(spawnsState(s)["notice_list"]) {
		if r, _ := raw.(map[string]any); in[str(r, "watcher")] && str(r, "quarry") == player {
			t.Fatalf("%s: the clock fight's %s watches him -- the R1 review's A1, which this script must stay out of: %v", act, str(r, "watcher"), r)
		}
	}

	for _, raw := range asList(sub(s.call("strigoi_get_system_state", map[string]any{"system": "pursuit"}), "state")["chase_list"]) {
		if c, _ := raw.(map[string]any); in[str(c, "hunter")] && str(c, "quarry") == player {
			t.Fatalf("%s: the clock fight's %s chases him -- the R1 review's A1: %v", act, str(c, "hunter"), c)
		}
	}
}

// scStepPast is V2's steps: scMinChunks of them, and on until the clock fight
// on quarry has ended. It returns how many it took, which the other side
// steps exactly.
func scStepPast(t *testing.T, s *session, act, quarry, player, monster string) int {
	t.Helper()

	n := 0

	for ; n < scMaxChunks && (n < scMinChunks || afhFightFor(s, quarry) != nil); n++ {
		if e := afhStepWorld(s, scChunk); e != "" {
			t.Fatalf("%s: step %d of %.0f minutes refused: %s (his fight %v)", act, n+1, scChunk, e, combatState(s))
		}

		scOnlyEachOther(t, s, act, player, quarry, monster)

		ck, c := afhClock(s), combatState(s)
		state := "ended"

		if f := afhFightFor(s, quarry); f != nil {
			state = fmt.Sprintf("round %v, quarry %v of %v, enemies %v", f["round"], f["quarry_health"], f["quarry_max_health"], f["enemies"])
		}

		t.Logf("%s step %d (%s): clock fight %s; clock ended %v (%q), draws %v; his fights %v, fighting %v",
			act, n+1, str(clockState(s), "time_of_day"), state, ck["ended"], str(ck, "ended_reason"),
			sub(ck, "rng")["draws"], c["encounters"], c["fighting"])
	}

	return n
}

// scStepChunks steps n of V2's steps.
func scStepChunks(t *testing.T, s *session, act string, n int) {
	t.Helper()

	for i := 0; i < n; i++ {
		if e := afhStepWorld(s, scChunk); e != "" {
			t.Fatalf("%s: step %d of %d refused: %s", act, i+1, n, e)
		}
	}
}

// scEnding is how the clock's fights have ended, in one line.
func scEnding(ck map[string]any) string {
	return fmt.Sprintf("started %v ended %v (last %q): quarry_dead %v, enemies_dead %v, routed %v, disengaged %v, dawn %v; rounds %v, next_id %v, combat-clock draws %v",
		ck["started"], ck["ended"], str(ck, "ended_reason"), ck["ended_quarry_dead"], ck["ended_enemies_dead"], ck["ended_routed"],
		ck["ended_disengaged"], ck["ended_dawn"], ck["rounds"], ck["next_id"], sub(ck, "rng")["draws"])
}

// scSameFight names what the brief asks of the resumed clock fight, beside
// the whole block's comparison: its c:<n> id and round, the clock's next_id,
// and the combat-clock stream's draws.
func scSameFight(t *testing.T, act string, want, got map[string]any, quarry string) {
	t.Helper()

	fw, fg := scFightIn(want, quarry), scFightIn(got, quarry)
	if fg == nil {
		t.Errorf("RED %s: no live clock fight on %s after the load", act, quarry)

		return
	}

	for _, k := range []string{"id", "round", "minutes_into_round", "quarry_health"} {
		if fmt.Sprint(fw[k]) != fmt.Sprint(fg[k]) {
			t.Errorf("RED %s: the clock fight's %s is %v, want %v", act, k, fg[k], fw[k])
		}
	}

	if fmt.Sprint(want["next_id"]) != fmt.Sprint(got["next_id"]) {
		t.Errorf("RED %s: the clock's next_id is %v, want %v", act, got["next_id"], want["next_id"])
	}

	if fmt.Sprint(sub(want, "rng")) != fmt.Sprint(sub(got, "rng")) {
		t.Errorf("RED %s: the combat-clock stream is %v, want %v", act, sub(got, "rng"), sub(want, "rng"))
	}
}

// scFightIn is the live clock fight on quarry in a clock block, or nil.
func scFightIn(ck map[string]any, quarry string) map[string]any {
	for _, l := range asList(ck["live"]) {
		if m, ok := l.(map[string]any); ok && str(m, "quarry") == quarry {
			return m
		}
	}

	return nil
}

// TestSaveResumeAFightHeIsNotIn is the merge scout's first, quick script
// (29 Sep 2026): two LIVE clock fights in the first dawn, saved and resumed,
// with the file-edit controls. (TestSaveResumeWithAFightHeIsNotIn, above, is
// the hunted night.) Neither branch could run it -- R1's Combat.Restore of
// the clock block was unit-tested only, because on its branch B4a still
// refused a hunted file, and B4b was built without R1. A fight he is not in
// never stops a save (the raid's Q5 (a)), so the save must also resume it.
//
//	V1  Seed 1462, the shipped combat dials (player_control human), the
//	    round among them (BUG-87). Two clock fights opened -- a
//	    zombie1 on a fallen1 fifteen tiles off and a second pair three tiles
//	    from it (c:1, c:2) -- and run a world minute at a time until both
//	    quarries are hurt: both live, both quarries hurt and alive, and
//	    neither his.
//	    The save, made on the first try (scSaveAtT, no retry; BUG-87),
//	    moves nothing; S_T; the file holds both in combat.clock.live,
//	    rng.combat_clock is combat.clock.rng, the combat-clock stream has
//	    been drawn from, and at least one action is still playing, saved at
//	    its frame (scHeldInFile).
//	V2  Three rounds (three step_world of scShippedRoundMinutes): S_U; he
//	    quits.
//	V3  A new process, the same home: start_game{save_path}, no seed --
//	    resumed; S_R0 = S_T. The resume digest's systems part hashes the
//	    combat provider's whole state (it has no process part), so the
//	    clock block -- the live fights, next_id, the combat-clock stream and
//	    every counter -- is in it; and the clock block is also compared
//	    whole, as JSON, so a failure names it.
//	V4  The same three rounds: S_R = S_U, the clock block U's.
//	V5  THE IN-PROCESS PATH: he dies, and "load last save" resumes T: S =
//	    S_T, and the same three rounds on, S_U.
//	V6  THE NEGATIVE CONTROLS, each from T's three files put back, each
//	    REFUSED:
//	    N1  combat.clock omitted from the file: FILE (d2save's shape check,
//	        B3-8);
//	    N2  a clock fight's quarry made an id no entity has: BLOCK (the
//	        combat block's Validate: the id does not resolve);
//	    N3  the two clock fights' quarries swapped: BLOCK (BUG-73,
//	        d2world.CheckClockWatches at the load's step 4: each fight's
//	        living enemy is aware of the other's quarry). The scout's run,
//	        before BUG-73, found it resumed and divergent.
//
// The source-mutation controls (the clock's restore moved, the stream not
// restored) are in strigoi-harness-runs\wt-merge-scout\merge-notes.md.
// Moved here from save_resume_test.go, where the scout's first commit put it,
// so the real merge takes the scout's scripts as one file.
func TestSaveResumeAFightHeIsNotIn(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Village", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	save := str(game, "save_path")

	for _, d := range villageFightDials {
		setField(s, d.System, d.Field, d.Value)
	}

	scShippedPace(t, s, "V1")

	// A second of play first, so the region check has run once and the
	// village's sound environment and music are set (Game.checkRegion runs
	// once a second of play; a load runs it at once, the sound being derived,
	// never saved). The first run of this script saved at 0.52 s: T had no
	// music and R0 had town1's, the only part that differed (pt-village-2).
	afhFrames(s, 70)

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")

	// --- V1: two fights he is not in, live ---------------------------------
	a, b := spawnNPC(t, s, "fallen1", px+15, py), spawnNPC(t, s, "zombie1", px+16, py)
	a2, b2 := spawnNPC(t, s, "fallen1", px+15, py+3), spawnNPC(t, s, "zombie1", px+16, py+3)
	aID, a2ID := entityID(t, s, a), entityID(t, s, a2)

	s.call("strigoi_watch", map[string]any{"watcher": b, "target": a})
	s.call("strigoi_watch", map[string]any{"watcher": b2, "target": a2})

	for i := 0; i < 60 && (afhFightFor(s, aID) == nil || afhFightFor(s, a2ID) == nil); i++ {
		afhFrames(s, 1)
	}

	hurt := func() bool {
		for _, q := range []string{aID, a2ID} {
			f := afhFightFor(s, q)
			if f == nil || num(f, "quarry_health") >= num(f, "quarry_max_health") {
				return false
			}
		}

		return true
	}

	minutes := 0
	for ; minutes < 3*int(scShippedRoundMinutes) && !hurt(); minutes++ {
		if e := afhStepWorld(s, 1.0); e != "" {
			t.Fatalf("V1: step_world refused while only the village fought: %s", e)
		}
	}

	t.Logf("V1: %d world minute(s) to both quarries hurt (a round every %.0f)", minutes, scShippedRoundMinutes)

	// The save, asked once and made (BUG-87): both fights live and neither
	// his, their blows' actions saved at their frames.
	out := scSaveAtT(t, s, "V1", func() string {
		for _, q := range []string{aID, a2ID} {
			f := afhFightFor(s, q)
			if f == nil {
				return "no live clock fight on " + q
			}

			if h, m := num(f, "quarry_health"), num(f, "quarry_max_health"); h <= 0 || h >= m {
				return fmt.Sprintf("%s's quarry is not hurt and alive (%v of %v)", str(f, "id"), h, m)
			}
		}

		if c := combatState(s); flag(t, c, "fighting") || str(c, "encounter") != "" {
			return "a fight he is not in took his slot"
		}

		return ""
	})

	for _, q := range []string{aID, a2ID} {
		f := afhFightFor(s, q)
		if f == nil {
			t.Fatalf("V1: no live clock fight on %s at T: %v", q, afhClock(s))
		}

		if h, m := num(f, "quarry_health"), num(f, "quarry_max_health"); h <= 0 || h >= m {
			t.Fatalf("V1: %s's quarry is hurt and alive at T (%v of %v): %v", str(f, "id"), h, m, f)
		}
	}

	if c := combatState(s); flag(t, c, "fighting") || str(c, "encounter") != "" {
		t.Fatalf("V1: a fight he is not in took his slot: %v", c)
	}

	clockAtT := afhClock(s)
	if draws := num(sub(clockAtT, "rng"), "draws"); draws <= 0 {
		t.Fatalf("V1: the combat-clock stream was never drawn from: %v", clockAtT["rng"])
	}

	// S_T after the save: scSaveAtT held the whole digest to one value.
	sT, clockT := snapWorld(t, s), afhClockJSON(t, s)

	worldPath := str(out, "world_path")
	fileT, od2T, sidecarT := mustRead(t, worldPath), mustRead(t, save), mustRead(t, save+".strigoi.json")
	file := worldFile(t, "V1", fileT)

	live := asList(sub(sub(file, "combat"), "clock")["live"])
	if len(live) != 2 {
		t.Fatalf("V1: the file holds %d live clock fights, want 2: %v", len(live), live)
	}

	w, err := d2save.Decode(fileT)
	if err != nil {
		t.Fatalf("V1: the file does not decode: %v", err)
	}

	if w.RNG.CombatClock != w.Combat.Clock.RNG || w.RNG.CombatClock.Draws == 0 {
		t.Fatalf("V1: rng.combat_clock %+v, combat.clock.rng %+v", w.RNG.CombatClock, w.Combat.Clock.RNG)
	}

	heldT := scHeldInFile(t, "V1", w)
	t.Logf("V1: T's file holds %d action(s) still playing, each at its frame: %v", len(heldT), heldT)

	t.Logf("V1 PASS: saved at %s on the first try with %d live clock fights (%s), clock next_id %v, combat-clock %+v; the digest unmoved",
		str(clockState(s), "time_of_day"), len(live), cut(fmt.Sprint(live)), clockAtT["next_id"], w.RNG.CombatClock)

	// --- V2: three rounds on; S_U; quit ------------------------------------
	for i := 0; i < 3; i++ {
		afhStepWorld(s, scShippedRoundMinutes)
	}

	sU, clockU := snapWorld(t, s), afhClockJSON(t, s)
	if sU.Resume == sT.Resume || clockU == clockT {
		t.Fatal("V2: three rounds moved nothing, so V4 would compare nothing")
	}

	t.Logf("V2: S_U taken, the clock block at U: %s", cut(clockU))
	s.stop()

	// --- V3: relaunch, load, S_R0 = S_T --------------------------------------
	s = start(t)
	s.call("strigoi_pause", map[string]any{})

	g := s.call("strigoi_start_game", map[string]any{"save_path": save, "wait_seconds": 90})
	if load := sub(g, "load"); !flag(t, load, "resumed") {
		t.Fatalf("V3: start_game resumes the world file with a clock fight live: %v", load)
	}

	for _, d := range villageFightDials {
		setField(s, d.System, d.Field, d.Value)
	}

	sameClock(t, "V3 (the clock block at R0 = T)", clockT, afhClockJSON(t, s))
	sameWorld(t, "V3 (S_R0 = S_T)", sT, snapWorld(t, s))
	t.Logf("V3 PASS: relaunched and resumed; S_R0 = S_T (resume digest %.12s), the clock block T's", sT.Resume)

	// --- V4: the same three rounds: S_R = S_U --------------------------------
	for i := 0; i < 3; i++ {
		afhStepWorld(s, scShippedRoundMinutes)
	}

	sameClock(t, "V4 (the clock block at R = U)", clockU, afhClockJSON(t, s))
	sameWorld(t, "V4 (S_R = S_U)", sU, snapWorld(t, s))
	t.Logf("V4 PASS: S_R = S_U (resume digest %.12s)", sU.Resume)

	// --- V5: in process: he dies, and "load last save" resumes T --------------
	setField(s, "meters", "health", 0.0)
	s.call("strigoi_step", map[string]any{"frames": 3})

	if !flag(t, uiState(s), "death_open") {
		t.Fatalf("V5: at 0 health the death screen is up: %v", uiState(s))
	}

	s.call("strigoi_key", map[string]any{"key": "enter"})

	if load := awaitGame(t, s, "V5"); !flag(t, load, "resumed") {
		t.Fatalf("V5: load last save resumes the world file, not the dawn: %v", load)
	}

	sameClock(t, "V5 (in process: the clock block T's)", clockT, afhClockJSON(t, s))
	sameWorld(t, "V5 (in process: S = S_T)", sT, snapWorld(t, s))

	for i := 0; i < 3; i++ {
		afhStepWorld(s, scShippedRoundMinutes)
	}

	sameClock(t, "V5 (in process: the clock block U's)", clockU, afhClockJSON(t, s))
	sameWorld(t, "V5 (in process: three rounds on, S_U)", sU, snapWorld(t, s))
	t.Log("V5 PASS: load last save resumed T in this process, and three rounds on it is U")

	// --- V6: the negative controls -------------------------------------------
	controls := []struct {
		name   string
		mutate func(t *testing.T, top []jsonField) []jsonField
		code   string // the refusal it must meet
		reason string // a phrase its reason must hold
	}{
		{"N1 (combat.clock omitted)", func(t *testing.T, top []jsonField) []jsonField {
			return editField(t, top, "combat", func(combat json.RawMessage) json.RawMessage {
				return encodeFields(dropField(t, decodeFields(t, combat), "clock"))
			})
		}, "FILE", "combat.clock"},
		{"N2 (a clock fight's quarry an id no entity has)", func(t *testing.T, top []jsonField) []jsonField {
			return editClockLive(t, top, func(live []json.RawMessage) {
				f := decodeFields(t, live[0])
				f = editField(t, f, "quarry", func(json.RawMessage) json.RawMessage {
					return json.RawMessage(`"ffffffff-no-such-entity"`)
				})
				live[0] = encodeFields(f)
			})
		}, "BLOCK", "does not resolve"},
		{"N3 (the two clock fights' quarries swapped)", func(t *testing.T, top []jsonField) []jsonField {
			return editClockLive(t, top, func(live []json.RawMessage) {
				f0, f1 := decodeFields(t, live[0]), decodeFields(t, live[1])
				q0, q1 := fieldOf(t, f0, "quarry"), fieldOf(t, f1, "quarry")
				f0 = editField(t, f0, "quarry", func(json.RawMessage) json.RawMessage { return q1 })
				f1 = editField(t, f1, "quarry", func(json.RawMessage) json.RawMessage { return q0 })
				live[0], live[1] = encodeFields(f0), encodeFields(f1)
			})
		}, "BLOCK", "is aware of"},
	}

	for _, c := range controls {
		s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
		awaitMenu(t, s)

		mutated := reindent(t, fileT, encodeFields(c.mutate(t, decodeFields(t, fileT))))
		if bytes.Equal(mutated, fileT) {
			t.Fatalf("%s: the mutation changed nothing", c.name)
		}

		for path, data := range map[string][]byte{worldPath: mutated, save: od2T, save + ".strigoi.json": sidecarT} {
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
		}

		g := s.call("strigoi_start_game", map[string]any{"save_path": save, "seed": 1462, "wait_seconds": 90})
		load := awaitGame(t, s, c.name)

		code, why := str(load, "refused"), str(load, "reason")

		switch {
		case code == c.code && strings.Contains(why, c.reason):
			t.Logf("%s PASS: refused %s (%s)", c.name, code, cut(why))
		case code != "":
			t.Errorf("RED %s: refused %s (%s), want %s naming %q", c.name, code, cut(why), c.code, c.reason)
		case flag(t, load, "resumed"):
			for _, d := range villageFightDials {
				setField(s, d.System, d.Field, d.Value)
			}

			t.Errorf("RED %s: the load was not refused (want %s): resumed, resume digest %.12s (S_T %.12s)",
				c.name, c.code, snapWorld(t, s).Resume, sT.Resume)
		default:
			t.Errorf("RED %s: neither refused nor resumed: %v (start %v)", c.name, load, sub(g, "load"))
		}
	}
}

// villageFightDials are the dials the village-fight script sets on every
// game it begins: no pack, the dead stay down, and his fights at the shipped
// dials (the launcher drops every script to the policy), the round among them
// (the merge slowed it to 4 minutes so a save could be made while the village
// fought; BUG-87 made that unnecessary). A dial is never saved (trap 7), so a
// relaunch sets them again.
var villageFightDials = []dialWrite{
	{"spawns", "chance", 0},
	{"rising", "p", 0.0},
	{"rising", "edge_floor", 0},
	{"combat", "player_control", "human"},
}

// afhClockJSON is the combat provider's clock block, as one JSON line.
func afhClockJSON(t *testing.T, s *session) string {
	t.Helper()

	raw, err := json.Marshal(afhClock(s))
	if err != nil {
		t.Fatalf("the clock block: %v", err)
	}

	return string(raw)
}

// sameClock requires the clock block b to be a, naming the fields that differ.
func sameClock(t *testing.T, act, a, b string) {
	t.Helper()

	if a == b {
		return
	}

	var ma, mb map[string]any
	_ = json.Unmarshal([]byte(a), &ma)
	_ = json.Unmarshal([]byte(b), &mb)

	var why []string

	for _, k := range keysOfAny(ma, mb) {
		ja, _ := json.Marshal(ma[k])
		jb, _ := json.Marshal(mb[k])

		if string(ja) != string(jb) {
			why = append(why, fmt.Sprintf("  clock.%s\n      was: %s\n      now: %s", k, cut(string(ja)), cut(string(jb))))
		}
	}

	t.Errorf("RED %s: the clock block differs:\n%s", act, strings.Join(why, "\n"))
}

// jsonField is one member of a JSON object, in the order the file has it:
// the controls edit the world file without re-ordering a single key.
type jsonField struct {
	Key string
	Val json.RawMessage
}

func decodeFields(t *testing.T, raw []byte) []jsonField {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("not a JSON object: %v %v", tok, err)
	}

	var out []jsonField

	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("a key: %v", err)
		}

		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("the value of %v: %v", tok, err)
		}

		out = append(out, jsonField{Key: tok.(string), Val: v})
	}

	return out
}

func encodeFields(fields []jsonField) json.RawMessage {
	var buf bytes.Buffer

	buf.WriteByte('{')

	for i, f := range fields {
		if i > 0 {
			buf.WriteByte(',')
		}

		k, _ := json.Marshal(f.Key)
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(f.Val)
	}

	buf.WriteByte('}')

	return buf.Bytes()
}

func fieldOf(t *testing.T, fields []jsonField, key string) json.RawMessage {
	t.Helper()

	for _, f := range fields {
		if f.Key == key {
			return f.Val
		}
	}

	t.Fatalf("no %q among %d fields", key, len(fields))

	return nil
}

func editField(t *testing.T, fields []jsonField, key string, edit func(json.RawMessage) json.RawMessage) []jsonField {
	t.Helper()

	for i, f := range fields {
		if f.Key == key {
			fields[i].Val = edit(f.Val)
			return fields
		}
	}

	t.Fatalf("no %q to edit", key)

	return nil
}

func dropField(t *testing.T, fields []jsonField, key string) []jsonField {
	t.Helper()

	for i, f := range fields {
		if f.Key == key {
			return append(fields[:i:i], fields[i+1:]...)
		}
	}

	t.Fatalf("no %q to drop", key)

	return nil
}

// editClockLive edits combat.clock.live, a list of clock fights, in place.
func editClockLive(t *testing.T, top []jsonField, edit func(live []json.RawMessage)) []jsonField {
	t.Helper()

	return editField(t, top, "combat", func(combat json.RawMessage) json.RawMessage {
		return encodeFields(editField(t, decodeFields(t, combat), "clock", func(clock json.RawMessage) json.RawMessage {
			return encodeFields(editField(t, decodeFields(t, clock), "live", func(raw json.RawMessage) json.RawMessage {
				var live []json.RawMessage
				if err := json.Unmarshal(raw, &live); err != nil {
					t.Fatalf("combat.clock.live: %v", err)
				}

				edit(live)

				out, _ := json.Marshal(live)

				return out
			}))
		}))
	})
}

// reindent lays edited out as the save lays a file out (two spaces), with
// the original's trailing newline.
func reindent(t *testing.T, original []byte, edited []byte) []byte {
	t.Helper()

	var compacted, out bytes.Buffer
	if err := json.Compact(&compacted, edited); err != nil {
		t.Fatalf("the edited file: %v", err)
	}

	if err := json.Indent(&out, compacted.Bytes(), "", "  "); err != nil {
		t.Fatalf("the edited file: %v", err)
	}

	if bytes.HasSuffix(original, []byte("\n")) {
		out.WriteByte('\n')
	}

	return out.Bytes()
}

// TestSaveResumeMidAction is BUG-87's own script (branch save-held, 29 Sep
// 2026): AN NPC SAVED MID-SWING, MID-FLINCH AND MID-DEATH RESUMES AT THE SAME
// FRAME. The unit tests carry a creature's sheet and a hand-built composite;
// an inherited monster's real composite needs the MPQs, so it is carried
// here, at the shipped round, in a clock fight -- a zombie1 on a fallen1
// fifteen tiles off, the two fighters the merge measured.
//
//	M1  Seed 1462, the shipped dials (villageFightDials, the round among
//	    them). The fight opens; the frames are stepped one at a time until a
//	    fighter holds A1 (a swing) past its first frame: the save is made on
//	    the first try (BUG-87), and the file holds the swing at the frame and
//	    the time into it that the entity reports (held_frame, held_elapsed).
//	    S_T; seven frames on, S_M; sixty, S_U.
//	M2  "Load last save" by the menu, in this process: resumed; the swing at
//	    its frame -- S_R0 = S_T, the entity's own report compared first;
//	    seven frames on S_M, sixty S_U.
//	M3  From the resumed game, the same for GH (a blow taken), and then DT
//	    (the loser's death, 56 frames): each saved on the first try at its
//	    frame, each resumed there and the same seven and sixty frames on --
//	    the death ending on the same frame, the corpse where the saved
//	    game's lies.
func TestSaveResumeMidAction(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Midswing", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	save := str(game, "save_path")

	for _, d := range villageFightDials {
		setField(s, d.System, d.Field, d.Value)
	}

	scShippedPace(t, s, "M1")
	afhFrames(s, 70)

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")

	quarry, monster := spawnNPC(t, s, "fallen1", px+15, py), spawnNPC(t, s, "zombie1", px+16, py)
	ids := []string{entityID(t, s, quarry), entityID(t, s, monster)}
	s.call("strigoi_watch", map[string]any{"watcher": monster, "target": quarry})

	for _, want := range []struct {
		mode string
		past int // the frame of the mode it must be past: mid-action, not its first
	}{{"A1", 1}, {"GH", 1}, {"DT", 3}} {
		act := "M-" + want.mode

		// Step to the frame a fighter holds the action past its first frame.
		who, st := "", map[string]any(nil)

		for f := 0; f < 3000 && who == ""; f++ {
			for _, id := range ids {
				e := scEntityByID(t, s, id)
				if e == nil {
					continue
				}

				if es := sub(e, "state"); str(es, "held") == want.mode && num(es, "held_frame") >= float64(want.past) {
					who, st = id, es
				}
			}

			if who == "" {
				afhFrames(s, 1)
			}
		}

		if who == "" {
			t.Fatalf("%s: no fighter held %s past frame %d in 3000 frames: clock %s", act, want.mode, want.past, cut(afhClockJSON(t, s)))
		}

		out, e := scCall(s, "strigoi_save_game", map[string]any{})
		if e != "" {
			t.Fatalf("RED %s: the save with %s holding %s at frame %v is made on the first try (BUG-87): refused %q",
				act, cut8(who), want.mode, st["held_frame"], e)
		}

		w, err := d2save.Decode(mustRead(t, str(out, "world_path")))
		if err != nil {
			t.Fatalf("%s: the file does not decode: %v", act, err)
		}

		var inFile *d2save.Entity

		for i := range w.Entities {
			if w.Entities[i].ID == who {
				inFile = &w.Entities[i]
			}
		}

		if inFile == nil || inFile.Motion.Action != want.mode || inFile.Motion.ActionAt == nil ||
			float64(inFile.Motion.ActionAt.Frame) != num(st, "held_frame") || inFile.Motion.ActionAt.Elapsed != num(st, "held_elapsed") {
			t.Fatalf("RED %s: the file holds %s's %s at the frame it reports (%v + %v s): %+v", act, cut8(who), want.mode,
				st["held_frame"], st["held_elapsed"], inFile)
		}

		sT := snapWorld(t, s)

		afhFrames(s, 7)
		sM := snapWorld(t, s)

		afhFrames(s, 53)
		sU := snapWorld(t, s)

		whoU := sub(scEntityByID(t, s, who), "state")

		// M2: the menu's load, in this process.
		s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
		awaitMenu(t, s)

		g := s.call("strigoi_start_game", map[string]any{"save_path": save, "wait_seconds": 90})
		if load := sub(g, "load"); !flag(t, load, "resumed") {
			t.Fatalf("%s: the save with %s held resumes: %v", act, want.mode, load)
		}

		for _, d := range villageFightDials {
			setField(s, d.System, d.Field, d.Value)
		}

		if got := sub(scEntityByID(t, s, who), "state"); str(got, "held") != want.mode ||
			fmt.Sprint(got["held_frame"], got["held_elapsed"]) != fmt.Sprint(st["held_frame"], st["held_elapsed"]) {
			t.Fatalf("RED %s: %s resumed holding %q at frame %v + %v s, saved holding %s at %v + %v s", act, cut8(who),
				str(got, "held"), got["held_frame"], got["held_elapsed"], want.mode, st["held_frame"], st["held_elapsed"])
		}

		sameWorld(t, act+" (S_R0 = S_T, the action at its frame)", sT, snapWorld(t, s))

		afhFrames(s, 7)
		sameWorld(t, act+" (seven frames on, S_M)", sM, snapWorld(t, s))

		afhFrames(s, 53)
		sameWorld(t, act+" (sixty frames on, S_U)", sU, snapWorld(t, s))

		t.Logf("%s PASS: %s held %s at frame %v + %v s; saved on the first try, resumed at that frame, and the same 7 and 60 frames on (then: held %q, corpse %v)",
			act, cut8(who), want.mode, st["held_frame"], st["held_elapsed"], str(whoU, "held"), whoU["corpse"])
	}
}

// scEntityByID is the entity whose id is id (strigoi_get_entity's report), or
// nil: a handle is per process, and a resumed game's are new.
func scEntityByID(t *testing.T, s *session, id string) map[string]any {
	t.Helper()

	for _, raw := range asList(s.call("strigoi_get_entities", map[string]any{"limit": 500})["items"]) {
		if e, _ := raw.(map[string]any); str(e, "id") == id {
			return s.call("strigoi_get_entity", map[string]any{"handle": str(e, "handle")})
		}
	}

	return nil
}

// TestSaveResumeAHeldActionOutOfReach is the BUG-87 review's B1 and B2 in the
// game (the review fixes, 29 Sep 2026; BUG-91, BUG-92): a held action the
// load cannot resume at its saved point. The reviewer's probe
// (TestZZRevNegativeElapsedNPC) is its act N2: a file whose NPC holds a swing
// a second BEFORE its frame -- a time no running game writes -- was taken by
// every check, and the resumed game panicked drawing a negative frame
// (composite.go:70, index out of range [-8]). Now:
//
//	N1  As TestSaveResumeMidAction's M1: seed 1462, the shipped dials, a
//	    zombie1 on a fallen1; the frames stepped until a fighter holds A1
//	    past its first frame; the save made; his files at T kept.
//	N2  The swing edited to -1.0 s into its frame (the probe's file): REFUSED,
//	    not crashed -- by the file's own check (World.Check, FILE: a point no
//	    play of any art can have). He begins at dawn, and the game draws on
//	    (5 frames stepped, 2 s of drawing) and answers.
//	N3  The swing at 1e18 s: a finite time the file cannot judge (it knows no
//	    art), so the load's restore refuses it (the composite's SetProgress:
//	    at or past one frame; 1e17 s and more wrapped the frame negative and
//	    crashed the game too): ENTITY, fallen back to dawn; it draws on.
//	N4  The swing at frame 99 -- A1 has 16: the ART no longer fits it (a
//	    sheet with fewer frames), so the night is RESUMED, the swing ended as
//	    it would have ended (the fighter in Neutral, where the file puts it),
//	    and the load report names it (ended_actions, and a note "an animation
//	    could not be resumed exactly"). The rest of the world is the saved
//	    moment -- S_R0 = S_T but for that entity's pose (sameWorldExcept) --
//	    and it plays on 60 frames.
func TestSaveResumeAHeldActionOutOfReach(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Outofreach", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	save := str(game, "save_path")

	for _, d := range villageFightDials {
		setField(s, d.System, d.Field, d.Value)
	}

	scShippedPace(t, s, "N1")
	afhFrames(s, 70)

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")

	quarry, monster := spawnNPC(t, s, "fallen1", px+15, py), spawnNPC(t, s, "zombie1", px+16, py)
	ids := []string{entityID(t, s, quarry), entityID(t, s, monster)}
	s.call("strigoi_watch", map[string]any{"watcher": monster, "target": quarry})

	// --- N1: a swing held past its first frame, saved -----------------------
	who := ""

	for f := 0; f < 3000 && who == ""; f++ {
		for _, id := range ids {
			if e := scEntityByID(t, s, id); e != nil {
				if es := sub(e, "state"); str(es, "held") == "A1" && num(es, "held_frame") >= 1 {
					who = id
				}
			}
		}

		if who == "" {
			afhFrames(s, 1)
		}
	}

	if who == "" {
		t.Fatalf("N1: no fighter held A1 past its first frame in 3000 frames: clock %s", cut(afhClockJSON(t, s)))
	}

	out, e := scCall(s, "strigoi_save_game", map[string]any{})
	if e != "" {
		t.Fatalf("N1: the save with %s mid-swing is made (BUG-87): refused %q", cut8(who), e)
	}

	sT := snapWorld(t, s)
	atT := evening{save: save, od2Left: mustRead(t, save), sidecarLeft: mustRead(t, save+".strigoi.json")}
	fileT := mustRead(t, str(out, "world_path"))

	stT := sub(scEntityByID(t, s, who), "state")
	t.Logf("N1: saved with %s holding A1 at frame %v + %v s", cut8(who), stT["held_frame"], stT["held_elapsed"])

	// The file at T with the swing's point edited.
	withSwingAt := func(frame, elapsed string) []byte {
		file := decodeNumbers(t, fileT)
		edited := false

		for _, raw := range asList(file["entities"]) {
			if ent, _ := raw.(map[string]any); str(ent, "id") == who {
				at := sub(sub(ent, "motion"), "action_at")
				at["frame"], at["elapsed"] = json.Number(frame), json.Number(elapsed)
				edited = true
			}
		}

		if !edited {
			t.Fatalf("N1: the file does not hold %s", who)
		}

		data, err := json.MarshalIndent(file, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		return append(data, '\n')
	}

	// load puts his files at T back, the world file edited, and loads them by
	// the menu, in this process; it returns the load report.
	load := func(act string, world []byte) map[string]any {
		if flag(t, s.call("strigoi_get_game_info", map[string]any{}), "in_game") {
			s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
			awaitMenu(t, s)
		}

		actThree(t, atT, world)

		g := s.call("strigoi_start_game", map[string]any{"save_path": save, "wait_seconds": 90})
		rep := sub(g, "load")
		t.Logf("%s: load %v", act, rep)

		return rep
	}

	// drawsOn is the game alive after a load: five frames stepped, two
	// seconds of real drawing (the probe's panic was in Composite.Render),
	// and the harness answering.
	drawsOn := func(act string) {
		afhFrames(s, 5)
		time.Sleep(2 * time.Second)

		if !flag(t, s.call("strigoi_get_game_info", map[string]any{}), "in_game") {
			t.Fatalf("%s: the game is not in a game after the load", act)
		}
	}

	// --- N2 and N3: corrupt, refused, and the game draws on (B1) ------------
	for _, c := range []struct {
		act, elapsed, code, why string
	}{
		{"N2 (-1.0 s, the probe's file)", "-1.0", "FILE", "not a point of a play"},
		{"N3 (1e18 s)", "1e18", "ENTITY", "s into a frame of"},
	} {
		rep := load(c.act, withSwingAt("1", c.elapsed))

		if flag(t, rep, "resumed") || str(rep, "refused") != c.code || !strings.Contains(str(rep, "reason"), c.why) {
			t.Fatalf("RED %s: a swing %s s into its frame is refused %s (%q): %v", c.act, c.elapsed, c.code, c.why, rep)
		}

		drawsOn(c.act)
		t.Logf("%s PASS: refused %s (%s), he began at dawn, and the game drew on", c.act, c.code, cut(str(rep, "reason")))
	}

	// --- N4: the art no longer fits -- resumed, the swing ended (B2) --------
	rep := load("N4 (frame 99)", withSwingAt("99", "0"))
	if !flag(t, rep, "resumed") {
		t.Fatalf("RED N4: a swing at a frame its art lacks is resumed, not refused (BUG-92): %v", rep)
	}

	for _, d := range villageFightDials {
		setField(s, d.System, d.Field, d.Value)
	}

	ended := asList(rep["ended_actions"])
	if len(ended) != 1 {
		t.Fatalf("RED N4: the load report names the one action it ended: %v", rep)
	}

	if e, _ := ended[0].(map[string]any); str(e, "id") != who || str(e, "action") != "A1" || !strings.Contains(str(e, "why"), "frame 99") {
		t.Fatalf("RED N4: ended_actions names %s's A1 at frame 99: %v", who, ended)
	}

	noted := false
	for _, n := range asList(rep["notes"]) {
		if note, _ := n.(string); strings.Contains(note, who) && strings.Contains(note, "an animation could not be resumed exactly") {
			noted = true
		}
	}

	if !noted {
		t.Fatalf("RED N4: the load's notes say an animation could not be resumed exactly: %v", rep["notes"])
	}

	st := sub(scEntityByID(t, s, who), "state")
	if str(st, "held") != "" || str(st, "animation_mode") != "NU" || flag(t, st, "corpse") {
		t.Fatalf("RED N4: %s resumed with its swing ended, in Neutral: %v", cut8(who), st)
	}

	sameWorldExcept(t, "N4 (S_R0 = S_T but for the fighter whose swing was ended)", sT, snapWorld(t, s),
		func(where string) bool { return strings.HasPrefix(where, "entity "+who+".") })

	afhFrames(s, 60)
	drawsOn("N4")

	t.Logf("N4 PASS: resumed; %s's A1 at frame 99 ended as it would have ended (%s, held %q); the report: %v; the rest the saved moment; 60 frames on",
		cut8(who), str(st, "animation_mode"), str(st, "held"), ended[0])
}
