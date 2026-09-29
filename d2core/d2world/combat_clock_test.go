package d2world

// THE FIGHTS HE IS NOT IN, IN UNIT FORM (the raid's R1, 29 Sep 2026): the
// brief's R1 closing assertions 2-5 and 7-10 over the resolver fixtures, each
// with its negative control. Assertion 6 (the snapshot) is
// combat_clock_snapshot_test.go's, and assertion 1 is the 288+ tests that were
// already here, unedited. The real game is playtest/fight_he_is_not_in_test.go.
//
// Where a control breaks the TEST'S side -- a clock driver fed on frames, a
// callback that reads his fight -- it runs here, beside the assertion, and
// must go red. Where it breaks the CODE (no separation, a shared stream, the
// earn guard gone, ...) it is a source mutation, applied, run and reverted
// by a script: "The Village at Night - R1 Build Note - 29 Sep 2026.md" lists
// each with what went red, and the logs are strigoi-harness-runs\wt-raid-r1\
// ctl-<name>.txt.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// clockFailures lets one check run twice -- on the design, where it must come
// back empty, and under a control, where it must not.
type clockFailures []string

func (f *clockFailures) expect(ok bool, format string, args ...interface{}) {
	if !ok {
		*f = append(*f, fmt.Sprintf(format, args...))
	}
}

const (
	clockDT        = 1.0 / 60 // the harness tick
	clockNightRate = 2.5      // DefaultClockDials' NightRate: 24 frames a world minute
)

// clockFrame is one game frame in miniature, as advanceWorld and
// tacticalAdvance run it: the world advances while nothing holds it
// (combat.Advance), then Tick, and each closed paced round's minutes are paid
// back through Advance. feedWhileHeld is the lockstep instrument's control: a
// clock driver fed on frames rather than on the world's minutes.
func clockFrame(f *resolverFight, s *fakeStepper, feedWhileHeld bool) {
	if s != nil {
		s.settle()
	}

	if !f.c.WorldHeld() {
		f.c.Advance(clockDT * clockNightRate)
	} else if feedWhileHeld {
		f.c.advanceClock(clockDT * clockNightRate)
	}

	f.c.Tick(clockDT)

	if owed := f.c.TakeRoundMinutes(); owed > 0 {
		f.c.Advance(owed)
	}
}

// clockFrames runs n frames.
func clockFrames(f *resolverFight, s *fakeStepper, n int) {
	for i := 0; i < n; i++ {
		clockFrame(f, s, false)
	}
}

// clockQuarry puts a quarry id at (x, y) with health hp, and a monster mid one
// tile east of it with health mhp, aware of it.
func clockQuarry(t *testing.T, f *resolverFight, id string, x, y float64, hp int, mid string, mhp int) *fakeQuarry {
	t.Helper()

	q := &fakeQuarry{id: id, x: x, y: y}
	f.bodies.known[id] = &fakeBody{health: hp, maxHealth: hp}

	w := &fakeWatcher{id: mid, x: x + 1, y: y}
	f.watchers[mid] = w
	f.bodies.known[mid] = &fakeBody{health: mhp, maxHealth: mhp}

	aware(t, f.notice, w, q)

	return q
}

func clockBlock(f *resolverFight) map[string]interface{} {
	return f.c.HarnessState()["clock"].(map[string]interface{})
}

func clockLive(f *resolverFight) []map[string]interface{} {
	return clockBlock(f)["live"].([]map[string]interface{})
}

// clockFightFor is the live clock fight after quarry, or nil.
func clockFightFor(f *resolverFight, quarry string) map[string]interface{} {
	for _, l := range clockLive(f) {
		if l["quarry"] == quarry {
			return l
		}
	}

	return nil
}

// clockRound is the round of the first live clock fight, -1 with none.
func clockRound(f *resolverFight) int {
	live := clockLive(f)
	if len(live) == 0 {
		return -1
	}

	return live[0]["round"].(int)
}

// --- R1 assertion 2 (and TestAFightHeIsNotIn acts 1-2, unit-sized) ---------

// fightHeIsNotIn: the shipped dials (human, paced), his player bound, a clock
// fight forty tiles off, nothing aware of him.
func fightHeIsNotIn(t *testing.T, minutes int) clockFailures {
	t.Helper()

	f, s := pacedFight(t, nil)
	f.c.SetPlayer("p:1")
	clockQuarry(t, f, "v:1", 80, 40, 5000, "m:2", 5000)

	var out clockFailures

	clockFrame(f, s, false) // the fight opens on this frame

	live := clockLive(f)
	out.expect(len(live) == 1, "clock.live has %d fights, want 1", len(live))

	if len(live) == 1 {
		out.expect(live[0]["quarry"] == "v:1", "the clock fight's quarry is %v, want v:1", live[0]["quarry"])
		out.expect(live[0]["id"] == "c:1", "the clock fight is %v, want c:1", live[0]["id"])
	}

	out.expect(!f.c.Fighting(), "Fighting() is true: a fight he is not in took his slot")
	out.expect(!f.c.WorldHeld(), "WorldHeld() is true: a fight he is not in holds the world")
	out.expect(!f.c.Awaiting(), "Awaiting() is true: his key would act for the villager")
	out.expect(f.c.Encounter() == "", "Encounter() is %q", f.c.Encounter())

	r0, vh0 := clockRound(f), f.health("v:1")

	clockFrames(f, s, minutes*24)

	out.expect(clockRound(f)-r0 == minutes, "the clock fight advanced %d rounds in %d world minutes, want %d",
		clockRound(f)-r0, minutes, minutes)
	out.expect(f.health("v:1") < vh0, "the quarry's health did not fall (%d -> %d)", vh0, f.health("v:1"))
	out.expect(f.health("p:1") == 240, "his health moved: %d", f.health("p:1"))

	st := f.c.HarnessState()
	out.expect(st["fighting"] == false, "his fighting = %v", st["fighting"])
	out.expect(st["encounters"] == 0, "his encounters = %v, want 0", st["encounters"])
	out.expect(st["actions_total"] == 0, "his actions_total = %v, want 0", st["actions_total"])
	out.expect(st["next_id"] == 1, "his next_id = %v, want 1: a clock fight spent his id", st["next_id"])
	out.expect(st["round_row"].(map[string]interface{})["encounter"] == "", "his round_row names %v",
		st["round_row"].(map[string]interface{})["encounter"])
	out.expect(f.c.LastRound().Encounter == "" && f.c.LastPace().Encounter == "",
		"his ROUND or PACE row names a fight: %+v %+v", f.c.LastRound(), f.c.LastPace())
	out.expect(len(f.c.Tactical().Blows) == 0, "his HUD blow log holds %d blows of a fight he was not in", len(f.c.Tactical().Blows))

	rows := clockBlock(f)["actions"].([]map[string]interface{})
	out.expect(len(rows) > 0, "the clock fight's last round has no blows")

	for _, row := range rows {
		out.expect(row["target"] == "v:1" || row["attacker"] == "v:1", "a clock blow not about v:1: %v", row)
	}

	return out
}

func TestClockFightIsNotHis(t *testing.T) {
	got := fightHeIsNotIn(t, 10)
	require.Empty(t, got, "a fight he is not in:\n  %s", strings.Join(got, "\n  "))
}

// --- R1 assertion 2 (M0.1c): lockstep with his paced fight ------------------

type lockstep struct {
	clockRoundsBefore   int
	hisRoundsClosed     int
	clockRoundsDuring   int
	clockDuringOpenTurn int
	heldOnlyByHis       bool
	hisRows             []string
}

// runLockstep opens a clock fight (unless withClock is false: the A of the dice
// A/B), lets it run 10 world minutes, then opens his paced fight and commits n
// turns, holding each open for hold frames.
func runLockstep(t *testing.T, feedWhileHeld, withClock bool, n, hold int) lockstep {
	t.Helper()

	f, s := pacedFight(t, nil)
	f.c.SetPlayer("p:1")

	if withClock {
		clockQuarry(t, f, "v:1", 80, 40, 5000, "m:2", 5000)
	}

	var out lockstep

	clockFrame(f, s, feedWhileHeld)

	r0 := clockRound(f)
	clockFrames(f, s, 10*24)
	out.clockRoundsBefore = clockRound(f) - r0

	f.add(t, "e:1", 5000, Profile{})
	clockFrame(f, s, feedWhileHeld)
	require.True(t, f.c.Fighting(), "his paced fight opens")
	require.Equal(t, "e:1", f.c.Encounter())

	hisR0, clockR0 := f.c.Round(), clockRound(f)
	rowsByRound := map[int]string{}
	out.heldOnlyByHis = true

	frame := func() {
		clockFrame(f, s, feedWhileHeld)

		if f.c.WorldHeld() && !f.c.Fighting() {
			out.heldOnlyByHis = false
		}

		st := f.c.HarnessState()
		if rows := st["actions"].([]map[string]interface{}); len(rows) > 0 {
			rowsByRound[st["actions_round"].(int)] = fmt.Sprintf("%v", rows)
		}
	}

	for turn := 0; turn < n; turn++ {
		for i := 0; i < 2000 && !f.c.Awaiting(); i++ {
			frame()
		}

		require.True(t, f.c.Awaiting(), "his turn opens")

		before := clockRound(f)

		for i := 0; i < hold; i++ {
			frame()
		}

		out.clockDuringOpenTurn += clockRound(f) - before

		require.NoError(t, f.c.Commit(CommitStrike, ""))

		if f.c.Awaiting() {
			require.NoError(t, f.c.Commit(CommitEnd, ""))
		}
	}

	for i := 0; i < 2000 && !f.c.Awaiting(); i++ {
		frame()
	}

	// His rows, one per round: the fullest log each round showed before the
	// next cleared it. (A paced round clears its log at its open, so a read at
	// the turn-open is always empty: the S0 instrument's first version
	// compared six empty logs. Its own control is below: blows >= 5.)
	for r := hisR0; r < f.c.Round(); r++ {
		out.hisRows = append(out.hisRows, fmt.Sprintf("r%d %v", r, rowsByRound[r]))
	}

	out.hisRows = append(out.hisRows, fmt.Sprintf("health p:1=%d e:1=%d", f.health("p:1"), f.health("e:1")))
	out.hisRoundsClosed = f.c.Round() - hisR0
	out.clockRoundsDuring = clockRound(f) - clockR0

	return out
}

func TestAClockFightKeepsLockstepWithHisPacedFight(t *testing.T) {
	got := runLockstep(t, false, true, 5, 600)
	t.Logf("MEASURED clock rounds in 10 world minutes before his fight: %d", got.clockRoundsBefore)
	t.Logf("MEASURED his rounds closed %d; clock rounds in the same span %d; clock rounds across 5 x 600 open-turn frames %d",
		got.hisRoundsClosed, got.clockRoundsDuring, got.clockDuringOpenTurn)

	require.Equal(t, 10, got.clockRoundsBefore, "one clock round per world minute")
	require.Equal(t, 5, got.hisRoundsClosed, "his five turns closed five rounds")
	require.Equal(t, got.hisRoundsClosed, got.clockRoundsDuring, "one clock round per round of his that closes")
	require.Zero(t, got.clockDuringOpenTurn, "no clock round while his turn is open")
	require.True(t, got.heldOnlyByHis, "only his fight holds the world")

	// THE CONTROL: the clock driver fed on every frame, held or not. The count
	// must see the drift, or it could not see a lockstep lost either.
	red := runLockstep(t, true, true, 5, 600)
	t.Logf("CONTROL frame-fed clock driver: his rounds %d, clock rounds %d, during open turns %d",
		red.hisRoundsClosed, red.clockRoundsDuring, red.clockDuringOpenTurn)
	require.NotZero(t, red.clockDuringOpenTurn, "the control must go red")
}

// --- R1 assertion 3 (M0.1d): his dice do not depend on the village ----------

func TestHisDiceDoNotDependOnAFightHeIsNotIn(t *testing.T) {
	a := runLockstep(t, false, false, 5, 10)
	b := runLockstep(t, false, true, 5, 10)

	// The instrument's own control: what is compared holds blows.
	blows := 0
	for _, r := range a.hisRows {
		blows += strings.Count(r, "attacker:")
	}

	t.Logf("MEASURED instrument: %d rounds compared, %d blows in them", len(a.hisRows)-1, blows)
	require.GreaterOrEqual(t, blows, 5, "the comparison must compare blows, not empty logs")

	require.Equal(t, a.hisRows, b.hisRows, "his rows must not depend on a fight across the village "+
		"(the share-stream control, a source mutation, is N-R1b)")
}

// --- R1 assertion 4: a clock quarry at 0 --------------------------------------

func quarryDies(t *testing.T) clockFailures {
	t.Helper()

	f, s := pacedFight(t, func(d *CombatDials) { d.ForcedBand = BandCrit })
	f.c.SetPlayer("p:1")

	k := NewCorpses(nil, nil, nil)
	f.c.SetCorpses(k)
	f.chases.chasing["m:2"] = true
	clockQuarry(t, f, "v:1", 80, 40, 2, "m:2", 5000)

	clockFrames(f, s, 5*24)

	var out clockFailures

	body, ok := k.Get("v:1")
	out.expect(ok, "no body fell for v:1")

	if ok {
		out.expect(body.Class == CorpseHuman && body.State == CorpseFresh, "v:1's body is %s/%s, want human/fresh", body.Class, body.State)
		out.expect(body.X == 80 && body.Y == 40, "v:1's body at %v,%v, want 80,40", body.X, body.Y)
	}

	out.expect(strings.Contains(strings.Join(f.animator.acts, " "), "v:1:die"), "v:1 did not play its death: %v", f.animator.acts)

	cb := clockBlock(f)
	out.expect(cb["ended_quarry_dead"] == 1, "clock ended_quarry_dead = %v, want 1", cb["ended_quarry_dead"])
	out.expect(cb["ended_reason"] == "quarry_dead", "clock ended_reason = %v", cb["ended_reason"])
	out.expect(cb["ended_player_dead"] == 0, "clock ended_player_dead = %v: a villager's death read as a dead player", cb["ended_player_dead"])
	out.expect(cb["released"] == 1, "clock released = %v, want 1 (m:2's watch)", cb["released"])

	st := f.c.HarnessState()
	out.expect(st["ended_player_dead"] == 0, "his ended_player_dead = %v", st["ended_player_dead"])
	out.expect(f.c.EndedReason() == "", "his EndedReason (the death screen's read) = %q", f.c.EndedReason())
	out.expect(len(f.c.TakeXPEvents()) == 0, "XP events reached him")

	_, watching := f.notice.Noticed("m:2")
	out.expect(!watching, "m:2 still watches the dead v:1 (the wedge)")
	out.expect(len(f.chases.released) > 0 && f.chases.released[len(f.chases.released)-1] == "m:2",
		"m:2's chase was not let go: %v", f.chases.released)
	out.expect(len(clockLive(f)) == 0, "a clock fight is still live")

	return out
}

func TestAClockQuarryAtZeroIsNotHisDeath(t *testing.T) {
	got := quarryDies(t)
	require.Empty(t, got, "a clock quarry at 0:\n  %s", strings.Join(got, "\n  "))
}

// C4: the monster killed in a clock fight pays him nothing.
func killerDies(t *testing.T) clockFailures {
	t.Helper()

	f, s := pacedFight(t, func(d *CombatDials) { d.ForcedBand = BandCrit })
	f.c.SetPlayer("p:1")
	f.c.SetCorpses(NewCorpses(nil, nil, nil))
	clockQuarry(t, f, "v:1", 80, 40, 5000, "m:2", 3)

	clockFrames(f, s, 3*24)

	var out clockFailures

	cb := clockBlock(f)
	out.expect(cb["ended_enemies_dead"] == 1, "clock ended_enemies_dead = %v, want 1", cb["ended_enemies_dead"])
	out.expect(cb["xp_suppressed"] == 1, "clock xp_suppressed = %v, want 1", cb["xp_suppressed"])

	xp := f.c.TakeXPEvents()
	out.expect(len(xp) == 0, "his XP events: %v", xp)

	_, err := f.c.Snapshot()
	out.expect(err == nil, "a save after the village's kill is refused: %v", err)

	return out
}

func TestAClockKillPaysHimNothing(t *testing.T) {
	got := killerDies(t)
	require.Empty(t, got, "a clock fight's kill:\n  %s", strings.Join(got, "\n  "))
}

// --- R1 assertion 5: Rejoin and BreakOff reach a clock fight ----------------

func TestRejoinAndBreakOffReachAClockFight(t *testing.T) {
	f := humanFight(t, 1462, nil)
	f.c.SetPlayer("p:1")

	q := clockQuarry(t, f, "v:1", 80, 40, 5000, "r:1", 5000)
	f.profiles.byID["r:1"] = Profile{Row: RisenRow, Dead: true}

	beast := &fakeWatcher{id: "m:3", x: 79, y: 40}
	f.watchers["m:3"] = beast
	f.bodies.known["m:3"] = &fakeBody{health: 5000, maxHealth: 5000}
	aware(t, f.notice, beast, q)

	f.c.Advance(0.5)
	require.Len(t, clockLive(f), 1)
	require.Len(t, clockLive(f)[0]["enemies"], 2, "the risen and the beast are both in it")

	// Rejoin: the risen, Downed and standing again as a new member.
	stood := &fakeWatcher{id: "r:2", x: 81, y: 40}
	f.watchers["r:2"] = stood
	f.bodies.known["r:2"] = &fakeBody{health: 5000, maxHealth: 5000}
	f.profiles.byID["r:2"] = Profile{Row: RisenRow, Dead: true}

	hisJoined := f.c.HarnessState()["joined"]
	require.True(t, f.c.Rejoin("r:1", stood), "a Downed man in a clock fight stands again into it")

	// The game takes the fallen one's remains off the map, and his watch with
	// them (Game.raiseTheDead); without it the old member would join again.
	f.notice.Unwatch("r:1")
	require.Equal(t, hisJoined, f.c.HarnessState()["joined"], "his joined count did not move")
	require.Equal(t, 1, clockBlock(f)["joined"])

	var ids []string
	for _, e := range clockLive(f)[0]["enemies"].([]map[string]interface{}) {
		ids = append(ids, e["id"].(string))
	}

	require.Equal(t, []string{"m:3", "r:2"}, ids)
	require.False(t, f.c.Rejoin("r:9", stood), "a man in no fight rejoins none")

	// BreakOff: first light takes the dead out; the beast fights on.
	n := f.c.BreakOff(func(id string) bool { return f.profiles.byID[id].Dead })
	require.Equal(t, 1, n)
	require.Len(t, clockLive(f), 1, "a mixed fight goes on")

	// And a fight of the dead alone ends dawn, in the clock's book.
	q2 := clockQuarry(t, f, "v:5", 80, 60, 5000, "r:6", 5000)
	f.profiles.byID["r:6"] = Profile{Row: RisenRow, Dead: true}
	_ = q2

	f.c.Advance(0.5)
	require.Len(t, clockLive(f), 2)

	n = f.c.BreakOff(func(id string) bool { return f.profiles.byID[id].Dead })
	require.Equal(t, 1, n)
	require.Len(t, clockLive(f), 1, "the dead-only fight ended")
	require.Equal(t, 1, clockBlock(f)["ended_dawn"])
	require.Equal(t, "dawn", clockBlock(f)["ended_reason"])
	require.Equal(t, 0, f.c.HarnessState()["ended_dawn"], "his ended_dawn did not move")
}

// --- R1 assertion 7: the wedge ----------------------------------------------

// wedge, in the legacy (unbound) path, which is the only path it can happen
// on: with a player bound his scan takes only pairs on him and the clock
// driver scans per quarry. A stale watch "a:1", sorted first, is on a dead
// quarry; a live pair "b:2" stands beside the player.
func wedge(t *testing.T) bool {
	t.Helper()

	f := newResolverFight(t, 1462)

	dead := &fakeQuarry{id: "v:9", x: 60, y: 40}
	f.bodies.known["v:9"] = &fakeBody{health: 0, maxHealth: 1}

	stale := &fakeWatcher{id: "a:1", x: 61, y: 40}
	f.watchers["a:1"] = stale
	f.bodies.known["a:1"] = &fakeBody{health: 61, maxHealth: 61}
	aware(t, f.notice, stale, dead)

	live := &fakeWatcher{id: "b:2", x: 41, y: 40}
	f.watchers["b:2"] = live
	f.bodies.known["b:2"] = &fakeBody{health: 61, maxHealth: 61}
	aware(t, f.notice, live, f.target)

	f.c.Advance(1.0)

	return f.c.Fighting()
}

func TestADeadQuarryWedgesNoFight(t *testing.T) {
	require.True(t, wedge(t), "with the per-pair skip, the live pair opens its fight (the no-skip control is N-R1f)")
}

// And bound: the quarry dies with its killer's watch on it; the watch goes in
// the same frame, and a second aware pair elsewhere opens its fight at the
// next scan.
func TestAClockQuarrysWatchesGoInTheFrameHeDies(t *testing.T) {
	f, s := pacedFight(t, func(d *CombatDials) { d.ForcedBand = BandCrit })
	f.c.SetPlayer("p:1")
	clockQuarry(t, f, "v:1", 80, 40, 2, "m:2", 5000)

	for i := 0; i < 5*24 && clockBlock(f)["ended_quarry_dead"] == 0; i++ {
		clockFrame(f, s, false)
	}

	require.Equal(t, 1, clockBlock(f)["ended_quarry_dead"])

	_, watching := f.notice.Noticed("m:2")
	require.False(t, watching, "the killer's watch went in the frame the quarry died")

	clockQuarry(t, f, "v:3", 80, 60, 5000, "m:4", 5000)
	clockFrame(f, s, false)
	require.NotNil(t, clockFightFor(f, "v:3"), "the next pair opens its fight at the next scan")
	require.Equal(t, "c:2", clockFightFor(f, "v:3")["id"])
}

// --- R1 assertion 8: the legacy rule -----------------------------------------

func TestTheLegacyRuleKeepsOneEncounter(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "e:1", 5000, Profile{})
	clockQuarry(t, f, "v:1", 80, 40, 5000, "m:2", 5000)

	for i := 0; i < 5; i++ {
		f.c.Advance(1.0)
	}

	require.True(t, f.c.Fighting())
	require.Empty(t, clockLive(f), "with no player bound, no clock fight opens")
	require.Equal(t, 1, f.c.HarnessState()["encounters"], "one encounter, as before the raid")
	require.Equal(t, 5000, f.health("v:1"), "and the second quarry's watchers wait")
}

// --- R1 assertion 9: no callback of a clock step reads his fight -------------

// readingAnimator is the control's callback: an Animator that reads the
// combat model's three accessors each time it is called, as a game reader
// hung on a callback would.
type readingAnimator struct {
	recordingAnimator
	c     *Combat
	reads []string
}

func (a *readingAnimator) Animate(id string, act CombatAct) {
	a.recordingAnimator.Animate(id, act)
	a.reads = append(a.reads, fmt.Sprintf("fighting=%v encounter=%q awaiting=%v", a.c.Fighting(), a.c.Encounter(), a.c.Awaiting()))
}

// clockStepCallbacks runs one clock fight to its quarry's death and another to
// its monster's -- every callback a clock step makes: Animate, Chases.Release,
// Notice.Unwatch (the real Notice), Corpses.FallHuman and Fall (the real
// Corpses), Morale.Hurt -- and returns the clock block.
func clockStepCallbacks(t *testing.T, reading bool) (map[string]interface{}, *resolverFight, *readingAnimator) {
	t.Helper()

	f, s := pacedFight(t, func(d *CombatDials) { d.ForcedBand = BandCrit })

	var ra *readingAnimator

	if reading {
		ra = &readingAnimator{c: f.c}
		f.c.animator = ra
	}

	f.c.SetPlayer("p:1")
	f.c.SetCorpses(NewCorpses(nil, nil, nil))
	f.chases.chasing["m:2"], f.chases.chasing["m:4"] = true, true
	clockQuarry(t, f, "v:1", 80, 40, 2, "m:2", 5000)
	clockQuarry(t, f, "v:3", 80, 60, 5000, "m:4", 3)
	f.profiles.byID["m:4"] = Profile{Row: "wolves", Group: "g:4", Speed: 2, DamageMin: 1, DamageMax: 1}
	f.morale.morale["g:4"] = 60

	clockFrames(f, s, 5*24)

	return clockBlock(f), f, ra
}

func TestNoCallbackOfAClockStepReadsHisFight(t *testing.T) {
	cb, f, _ := clockStepCallbacks(t, false)

	// The callbacks ran: otherwise a zero below would prove nothing.
	require.Equal(t, 1, cb["ended_quarry_dead"])
	require.Equal(t, 1, cb["ended_enemies_dead"])
	require.NotEmpty(t, f.animator.acts)
	require.NotEmpty(t, f.morale.hurts)
	require.NotEmpty(t, f.chases.released)

	require.Equal(t, 0, cb["reentrant_reads"], "a callback made during a clock step read Fighting, Encounter or Awaiting")

	// THE CONTROL: a callback that reads them. The count must see it -- and
	// every read must have been answered with HIS fight (none), not the clock
	// fight the swap had on the struct.
	red, _, ra := clockStepCallbacks(t, true)
	t.Logf("CONTROL a reading Animator: reentrant_reads=%v; its reads: %v", red["reentrant_reads"], ra.reads)
	require.NotZero(t, red["reentrant_reads"], "the control must go red")

	for _, r := range ra.reads {
		require.Equal(t, `fighting=false encounter="" awaiting=false`, r, "a reentrant read answered with the clock fight")
	}
}

// --- R1 assertion 10 (S0-3 (a)): the clock fights' own ids -------------------

func TestClockFightsHaveTheirOwnIDs(t *testing.T) {
	f, s := pacedFight(t, nil)
	f.c.SetPlayer("p:1")
	clockQuarry(t, f, "v:1", 80, 40, 5000, "m:2", 5000)
	clockQuarry(t, f, "v:3", 80, 60, 5000, "m:4", 5000)

	clockFrame(f, s, false)
	require.Equal(t, "c:1", clockFightFor(f, "v:1")["id"])
	require.Equal(t, "c:2", clockFightFor(f, "v:3")["id"])
	require.Equal(t, 3, clockBlock(f)["next_id"])

	f.add(t, "e:1", 5000, Profile{})
	clockFrame(f, s, false)
	require.True(t, f.c.Fighting())
	require.Equal(t, "e:1", f.c.Encounter(), "two clock fights opened before his first, and his first is still e:1")

	st := f.c.HarnessState()
	require.Equal(t, "e:1", st["encounter"])
	require.Equal(t, st["encounters"].(int)+1, st["next_id"], "his next_id is one past his fights started (B1's rule)")
	require.Equal(t, 2, clockBlock(f)["started"])
}

// --- S0-1 (a): a protected quarry is no quarry --------------------------------

func TestAProtectedQuarryIsNoQuarry(t *testing.T) {
	f, s := pacedFight(t, nil)
	f.c.SetPlayer("p:1")
	f.c.SetProtected(func(id string) bool { return id == "v:1" })
	clockQuarry(t, f, "v:1", 80, 40, 1, "m:2", 5000)
	clockQuarry(t, f, "v:3", 80, 60, 5000, "m:4", 5000)

	clockFrames(f, s, 5*24)

	require.Nil(t, clockFightFor(f, "v:1"), "no fight opens on a protected quarry")
	require.Equal(t, 1, f.health("v:1"), "and nothing strikes him")
	require.NotNil(t, clockFightFor(f, "v:3"), "the control, beside it: an unprotected quarry's fight opens")

	// Unbound (the legacy path): a pair on a protected quarry sorted first
	// does not take his slot, nor wedge the pair on him.
	g := newResolverFight(t, 1462)
	g.c.SetProtected(func(id string) bool { return id == "a:0" })

	prot := &fakeQuarry{id: "a:0", x: 60, y: 40}
	g.bodies.known["a:0"] = &fakeBody{health: 1, maxHealth: 1}
	w := &fakeWatcher{id: "a:1", x: 61, y: 40}
	g.watchers["a:1"] = w
	g.bodies.known["a:1"] = &fakeBody{health: 61, maxHealth: 61}
	aware(t, g.notice, w, prot)
	g.add(t, "b:2", 61, Profile{})

	g.c.Advance(1.0)
	require.True(t, g.c.Fighting(), "the pair on him opens his fight")
	require.Equal(t, "p:1", g.c.Order()[0], "and he, not the protected quarry, is its quarry")
}
