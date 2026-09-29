package d2world

// THE R1 REVIEW'S FIXES, IN UNIT FORM (the raid's R0.5 and R1, independent
// review of 29 Sep 2026). The review found the book swap separated the fights
// by RECORD and not by COMBATANT (A1): one monster could be an enemy in his
// fight and in a clock fight at once, and when the village killed it, a
// corpse struck him on and then paid him the kill. The reviewer's throwaway
// probes P1 (double membership) and P2 (a clock quarry that is his enemy) are
// real tests here, beside the tests of each part of the fix and of C1 (a
// decline is the named quarry's), C2 (every reader of his fight is his, mid
// clock step) and B2 (Rejoin says whose fight). B1 (a dead enemy off the map
// stops no save) is combat_clock_snapshot_test.go's. Each has its negative
// control: a source mutation, applied, run and reverted by a script, listed
// in the R1 build note's "Review fixes (29 Sep)" with what went red; the
// logs are strigoi-harness-runs\wt-raid-r1-fix\ctl-<name>.txt.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// clockHolds reports whether a live clock fight holds id as a living enemy.
func clockHolds(f *resolverFight, id string) bool {
	for _, l := range clockLive(f) {
		for _, e := range l["enemies"].([]map[string]interface{}) {
			if e["id"] == id && e["dead"] == false {
				return true
			}
		}
	}

	return false
}

// totals is how many blows his fight and the clock's have struck.
func totals(f *resolverFight) [2]interface{} {
	return [2]interface{}{f.c.HarnessState()["actions_total"], clockBlock(f)["actions_total"]}
}

// blowsBy counts the blows struck by id since the totals were before: the last
// round's rows of each side that has struck since (a round's rows outlive it,
// so a side that struck nothing new is not read).
func blowsBy(f *resolverFight, id string, before [2]interface{}) int {
	n := 0
	now := totals(f)

	for i, rows := range [][]map[string]interface{}{
		f.c.HarnessState()["actions"].([]map[string]interface{}),
		clockBlock(f)["actions"].([]map[string]interface{}),
	} {
		if now[i] == before[i] {
			continue
		}

		for _, r := range rows {
			if r["attacker"] == id {
				n++
			}
		}
	}

	return n
}

func actsOf(f *resolverFight, id, act string) int {
	n := 0

	for _, a := range f.animator.acts {
		if a == id+":"+act {
			n++
		}
	}

	return n
}

// villageAndHim is the review's P1 set-up: him bound, holding and with no
// Reaction (he never strikes, not even a riposte, so only the village could
// kill m:2), a villager v:1 two tiles east of him, and a wolf m:2 between
// them, watching v:1 -- a clock fight, c:1.
func villageAndHim(t *testing.T) (*resolverFight, *fakeWatcher, *fakeQuarry) {
	t.Helper()

	f := newResolverFight(t, 1462)
	f.c.SetPlayer("p:1")
	f.c.SetCorpses(NewCorpses(nil, nil, nil))
	f.set(t, "player_action", PlayerActionHold)
	f.fitness.reaction = false

	v := &fakeQuarry{id: "v:1", x: 42, y: 40}
	f.bodies.known["v:1"] = &fakeBody{health: 5000, maxHealth: 5000}

	m := &fakeWatcher{id: "m:2", x: 41, y: 40}
	f.watchers["m:2"] = m
	f.bodies.known["m:2"] = &fakeBody{health: 30, maxHealth: 30}
	f.profiles.byID["m:2"] = Profile{Row: "wolves", Group: "g:2", Speed: 2, DamageMin: 1, DamageMax: 2}

	aware(t, f.notice, m, v)
	f.c.Advance(0.25)
	require.NotNil(t, clockFightFor(f, "v:1"), "c:1 opens on v:1")
	require.False(t, f.c.Fighting(), "and it is not his")

	return f, m, v
}

// --- A1, the review's P1: the watch moves from a villager to him -------------

// retargetedToHim runs P1 on the design: m:2's watch moves to him (strigoi_watch
// today; Seek's retarget from R2). At no step is m:2 in two fights; c:1 lets
// it go and ends; his fight takes it; nothing but him ever strikes it, so the
// village kills nothing and pays nobody; no dead body strikes; and his own
// kill, at the end, pays him once.
func retargetedToHim(t *testing.T) clockFailures {
	t.Helper()

	f, m, _ := villageAndHim(t)
	aware(t, f.notice, m, f.target)

	var out clockFailures

	both, deadBlows := 0, 0

	step := func(minutes float64) {
		dead, before := f.health("m:2") <= 0, totals(f)
		f.c.Advance(minutes)

		if f.c.Participates("m:2") && clockHolds(f, "m:2") {
			both++
		}

		if dead {
			deadBlows += blowsBy(f, "m:2", before)
		}
	}

	step(0.25) // the step its watch moved: c:1 lets it go

	for i := 0; i < 20; i++ {
		step(1.0)
	}

	cb := clockBlock(f)
	out.expect(both == 0, "m:2 was in his fight and a clock fight at once on %d step(s)", both)
	out.expect(f.c.Participates("m:2"), "his fight never took m:2 (order %v)", f.c.Order())
	out.expect(clockFightFor(f, "v:1") == nil && !clockHolds(f, "m:2"), "a clock fight still holds m:2: %v", clockLive(f))
	out.expect(cb["ended_disengaged"] == 1, "c:1 did not end when m:2 left it: ended_disengaged %v", cb["ended_disengaged"])
	out.expect(f.health("m:2") == 30, "something struck m:2 while he held: health %d", f.health("m:2"))
	out.expect(cb["ended_enemies_dead"] == 0 && cb["xp_suppressed"] == 0, "the village killed m:2: enemies_dead %v xp_suppressed %v",
		cb["ended_enemies_dead"], cb["xp_suppressed"])
	out.expect(f.health("p:1") < 240, "m:2 never struck him in his fight")

	xp := f.c.TakeXPEvents()
	out.expect(len(xp) == 0, "his XP events for a kill he did not make: %v", xp)

	// He strikes: HIS kill is his, once.
	f.set(t, "player_action", PlayerActionAttack)
	f.fitness.reaction = true

	for i := 0; i < 20 && f.c.Fighting(); i++ {
		step(1.0)
	}

	xp = f.c.TakeXPEvents()
	out.expect(len(xp) == 1 && xp[0].Kind == "slain", "his own kill paid him %v, want one slain", xp)
	out.expect(clockBlock(f)["xp_suppressed"] == 0, "the clock counted his kill: xp_suppressed %v", clockBlock(f)["xp_suppressed"])
	out.expect(deadBlows == 0, "a dead m:2 struck %d time(s)", deadBlows)
	out.expect(actsOf(f, "m:2", "die") == 1, "m:2 played its death %d times", actsOf(f, "m:2", "die"))

	return out
}

func TestAMonsterRetargetedToHimLeavesTheVillagesFight(t *testing.T) {
	got := retargetedToHim(t)
	require.Empty(t, got, "P1, the watch moved to him:\n  %s", strings.Join(got, "\n  "))
}

// --- A1, the other direction: his fight wins a tie ---------------------------

// retargetedAway: m:2 is in HIS fight when its watch moves to the villager.
// His fight keeps it while it is in reach of him (his fight wins the tie);
// no clock fight opens on v:1 with it; nobody strikes v:1 or m:2 but him and
// m:2's own blows on him.
func retargetedAway(t *testing.T) clockFailures {
	t.Helper()

	f := newResolverFight(t, 1462)
	f.c.SetPlayer("p:1")
	f.c.SetCorpses(NewCorpses(nil, nil, nil))
	f.set(t, "player_action", PlayerActionHold)
	f.fitness.reaction = false

	v := &fakeQuarry{id: "v:1", x: 42, y: 40}
	f.bodies.known["v:1"] = &fakeBody{health: 5000, maxHealth: 5000}

	m := f.add(t, "m:2", 30, Profile{Row: "wolves", Group: "g:2", Speed: 2, DamageMin: 1, DamageMax: 2})
	f.c.Advance(0.25)
	require.True(t, f.c.Participates("m:2"), "his fight opens on m:2")

	aware(t, f.notice, m, v)

	var out clockFailures

	both := 0

	for i := 0; i < 20; i++ {
		f.c.Advance(1.0)

		if f.c.Participates("m:2") && clockHolds(f, "m:2") {
			both++
		}
	}

	cb := clockBlock(f)
	out.expect(both == 0, "m:2 was in his fight and a clock fight at once on %d step(s)", both)
	out.expect(f.c.Participates("m:2"), "his fight let m:2 go: his fight must win the tie")
	out.expect(cb["started"] == 0, "a clock fight opened on v:1 with his enemy in it (started %v)", cb["started"])
	out.expect(f.health("v:1") == 5000 && f.health("m:2") == 30, "v:1 and m:2 fought: v:1 %d, m:2 %d", f.health("v:1"), f.health("m:2"))
	out.expect(len(f.c.TakeXPEvents()) == 0 && cb["xp_suppressed"] == 0, "a kill was paid or suppressed")

	return out
}

func TestHisFightWinsATie(t *testing.T) {
	got := retargetedAway(t)
	require.Empty(t, got, "the watch moved away from him:\n  %s", strings.Join(got, "\n  "))
}

// --- A1, the review's P2: his enemy is no village quarry ---------------------

// hisEnemyAsQuarry: a:1 is his enemy, and b:2 watches a:1. No clock fight
// opens on a:1 while it is in his fight; he kills it, and is paid once; it
// dies once (one death played, one body -- a beast's, where it fell), never
// strikes b:2 dead, and the clock counts nothing of it.
func hisEnemyAsQuarry(t *testing.T) clockFailures {
	t.Helper()

	f := newResolverFight(t, 1462)
	f.c.SetPlayer("p:1")

	k := NewCorpses(nil, nil, nil)
	f.c.SetCorpses(k)

	aw := &fakeWatcher{id: "a:1", x: 41, y: 40}
	aq := &fakeQuarry{id: "a:1", x: 41, y: 40}
	f.watchers["a:1"] = aw
	f.bodies.known["a:1"] = &fakeBody{health: 5, maxHealth: 5}
	f.profiles.byID["a:1"] = Profile{Row: "wolves", Group: "g:a", Speed: 2, DamageMin: 1, DamageMax: 2}
	aware(t, f.notice, aw, f.target)

	b := &fakeWatcher{id: "b:2", x: 42, y: 40}
	f.watchers["b:2"] = b
	f.bodies.known["b:2"] = &fakeBody{health: 5000, maxHealth: 5000}
	aware(t, f.notice, b, aq)

	var out clockFailures

	f.c.Advance(0.25)
	out.expect(f.c.Participates("a:1"), "his fight did not open on a:1")

	quarried := 0

	for i := 0; i < 10 && f.health("a:1") > 0; i++ {
		f.c.Advance(1.0)

		if clockFightFor(f, "a:1") != nil {
			quarried++
		}
	}

	out.expect(f.health("a:1") == 0, "he did not kill a:1 (health %d)", f.health("a:1"))

	xp := f.c.TakeXPEvents()
	out.expect(len(xp) == 1 && xp[0].Kind == "slain", "his kill paid him %v, want one slain", xp)

	for i := 0; i < 3; i++ {
		f.c.Advance(1.0)

		if clockFightFor(f, "a:1") != nil {
			quarried++
		}
	}

	cb := clockBlock(f)
	out.expect(quarried == 0 && cb["started"] == 0, "a clock fight opened on a:1, his enemy (%d step(s), started %v)", quarried, cb["started"])
	out.expect(cb["ended_quarry_dead"] == 0, "the clock counted his kill as its quarry's death: %v", cb["ended_quarry_dead"])
	out.expect(actsOf(f, "a:1", "die") == 1, "a:1 played its death %d times", actsOf(f, "a:1", "die"))
	out.expect(f.health("b:2") == 5000, "b:2 was struck (health %d): the dead a:1 took an activation", f.health("b:2"))

	if body, ok := k.Get("a:1"); ok {
		out.expect(body.Class != CorpseHuman, "a:1 fell as a man, a second time, over its own body: %+v", body)
	} else {
		out.expect(false, "no body fell for a:1")
	}

	return out
}

func TestAMonsterInHisFightIsNoVillageQuarry(t *testing.T) {
	got := hisEnemyAsQuarry(t)
	require.Empty(t, got, "P2, his enemy as a clock quarry:\n  %s", strings.Join(got, "\n  "))
}

// --- A1 part 3: a body at 0 is gone in every fight ---------------------------

// A monster in his fight whose body reaches 0 by something that is not his
// fight's (another fight, or no fight's) never strikes again, is never struck
// again, and pays nothing: his fight ends with nothing left in it.
func TestABodyAtZeroIsGoneInEveryFight(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.c.SetPlayer("p:1")
	f.c.SetCorpses(NewCorpses(nil, nil, nil))
	f.add(t, "m:2", 5000, Profile{Row: "wolves", Group: "g:2", Speed: 2, DamageMin: 1, DamageMax: 2})

	f.c.Advance(0.25)
	require.True(t, f.c.Participates("m:2"), "his fight opens on m:2")

	f.bodies.known["m:2"].health = 0 // killed elsewhere: nothing in HIS fight's record says so

	hp, before := f.health("p:1"), totals(f)
	f.c.Advance(1.0)

	var out clockFailures

	out.expect(blowsBy(f, "m:2", before) == 0, "the dead m:2 struck him")
	out.expect(f.health("p:1") == hp, "his health moved %d -> %d", hp, f.health("p:1"))

	for _, r := range f.c.HarnessState()["actions"].([]map[string]interface{}) {
		out.expect(r["target"] != "m:2", "he struck the dead m:2: %v", r)
	}

	xp := f.c.TakeXPEvents()
	out.expect(len(xp) == 0, "a kill he did not make paid him: %v", xp)
	out.expect(!f.c.Fighting() && f.c.EndedReason() == "enemies_dead", "his fight did not end on its dead enemy: fighting %v, ended %q",
		f.c.Fighting(), f.c.EndedReason())
	out.expect(actsOf(f, "m:2", "die") == 0, "m:2 played a death in a fight that did not kill it")

	require.Empty(t, out, "a body at 0 in his fight:\n  %s", strings.Join(out, "\n  "))
}

// A clock fight whose quarry is at 0 before its round -- killed by something
// that is not this fight -- ends at once: no blow, no second body, no second
// death; quarry_dead, and the watches on him go in the frame.
func TestAClockFightWhoseQuarryIsAlreadyDeadEndsBeforeItsRound(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.c.SetPlayer("p:1")

	k := NewCorpses(nil, nil, nil)
	f.c.SetCorpses(k)
	clockQuarry(t, f, "v:1", 80, 40, 5000, "m:2", 5000)

	f.c.Advance(0.25)
	require.NotNil(t, clockFightFor(f, "v:1"), "c:1 opens")

	f.bodies.known["v:1"].health = 0

	actions0, acts0 := clockBlock(f)["actions_total"], len(f.animator.acts)
	f.c.Advance(1.0)

	cb := clockBlock(f)

	var out clockFailures

	out.expect(len(clockLive(f)) == 0, "c:1 is still live")
	out.expect(cb["ended_reason"] == "quarry_dead" && cb["ended_quarry_dead"] == 1, "c:1 ended %v (quarry_dead %v)",
		cb["ended_reason"], cb["ended_quarry_dead"])
	out.expect(cb["actions_total"] == actions0, "blows were struck over a dead quarry: actions %v -> %v", actions0, cb["actions_total"])
	out.expect(len(f.animator.acts) == acts0, "something played over the dead: %v", f.animator.acts[acts0:])
	out.expect(cb["released"] == 1, "m:2's watch was not let go (released %v)", cb["released"])

	_, fell := k.Get("v:1")
	out.expect(!fell, "a body fell for v:1: the fight that ended did not kill him")
	out.expect(f.health("m:2") == 5000, "the dead v:1 struck m:2")

	require.Empty(t, out, "a clock fight over a dead quarry:\n  %s", strings.Join(out, "\n  "))
}

// --- C1: a decline is the named quarry's ------------------------------------

// Out of reach of their quarries: a:1 watching him, b:2 watching a villager.
// With him bound his declines count a:1 alone and the clock's b:2 alone; the
// legacy rule (no player bound) counts both, as it always has.
func TestADeclineIsTheNamedQuarrys(t *testing.T) {
	declines := func(bound bool) (his, clock interface{}, saved int) {
		f := newResolverFight(t, 1462)

		if bound {
			f.c.SetPlayer("p:1")
		}

		a := &fakeWatcher{id: "a:1", x: 50, y: 40}
		f.watchers["a:1"] = a
		aware(t, f.notice, a, f.target)

		v := &fakeQuarry{id: "v:1", x: 80, y: 40}
		b := &fakeWatcher{id: "b:2", x: 90, y: 40}
		f.watchers["b:2"] = b
		aware(t, f.notice, b, v)

		f.c.Advance(0.25)

		snap, err := f.c.Snapshot()
		require.NoError(t, err)

		return f.c.HarnessState()["declined_reach"], clockBlock(f)["declined_reach"], snap.Declines
	}

	his, clock, saved := declines(true)
	require.Equal(t, 1, his, "his declines count the pair on him alone")
	require.Equal(t, 1, clock, "the clock's count the pair on the quarry it was opening alone")
	require.Equal(t, 1, saved, "and his saved declines are his")

	his, _, saved = declines(false)
	require.Equal(t, 2, his, "the legacy rule counts every pair, as it always has")
	require.Equal(t, 2, saved)
}

// --- C2: every reader of his fight is his, mid clock step --------------------

// readers is every reader of his fight the game has, as one line.
func readers(c *Combat) string {
	return fmt.Sprintf("Fighting=%v Encounter=%q Awaiting=%v Round=%d Order=%v Participates(e:1)=%v ActionSpent=%v MoveSpent=%v "+
		"WorldHeld=%v Paced=%v Tactical=%+v LastRound=%+v LastPace=%+v EndedReason=%q",
		c.Fighting(), c.Encounter(), c.Awaiting(), c.Round(), c.Order(), c.Participates("e:1"), c.ActionSpent(), c.MoveSpent(),
		c.WorldHeld(), c.Paced(), c.Tactical(), c.LastRound(), c.LastPace(), c.EndedReason())
}

// everyReaderAnimator reads every reader of his fight each time a clock step
// calls it, as a game reader hung on a callback would.
type everyReaderAnimator struct {
	recordingAnimator
	c     *Combat
	reads []string
}

func (a *everyReaderAnimator) Animate(id string, act CombatAct) {
	a.recordingAnimator.Animate(id, act)
	a.reads = append(a.reads, readers(a.c))
}

// His paced turn is open, his Action spent, while the village fights: a clock
// fight c:1 many rounds in, another (c:2) ended quarry_dead. A callback of
// c:1's step reads every reader, and each must answer as it did before the
// step -- his fight, never the clock's -- and be counted.
func TestEveryReaderAnswersHisFightMidStep(t *testing.T) {
	f, s := pacedFight(t, nil)
	f.c.SetPlayer("p:1")
	f.c.SetCorpses(NewCorpses(nil, nil, nil))
	clockQuarry(t, f, "v:1", 80, 40, 5000, "m:2", 5000)
	clockQuarry(t, f, "v:3", 80, 60, 2, "m:4", 5000)

	for i := 0; i < 5*24 && clockBlock(f)["ended_quarry_dead"] == 0; i++ {
		clockFrame(f, s, false)
	}

	require.Equal(t, "quarry_dead", clockBlock(f)["ended_reason"], "the clock's last fight ended quarry_dead")

	f.add(t, "e:1", 5000, Profile{})

	for i := 0; i < 2000 && !f.c.Awaiting(); i++ {
		clockFrame(f, s, false)
	}

	require.True(t, f.c.Awaiting(), "his turn opens")
	require.NoError(t, f.c.Commit(CommitStrike, ""))
	require.True(t, f.c.Awaiting() && f.c.ActionSpent(), "his Action is spent and his turn still open")
	require.Greater(t, clockRound(f), f.c.Round(), "the clock fight is further on than his: a leaked Round would show")

	want := readers(f.c)
	reads0 := clockBlock(f)["reentrant_reads"].(int)

	ra := &everyReaderAnimator{c: f.c}
	f.c.animator = ra

	f.c.Advance(1.0) // his turn is open: only the clock fight steps

	require.NotEmpty(t, ra.reads, "the clock step made no callback: the test would prove nothing")

	var out clockFailures

	for i, got := range ra.reads {
		out.expect(got == want, "read %d mid clock step answered the clock's fight:\n    got  %s\n    want %s", i, got, want)
	}

	out.expect(readers(f.c) == want, "after the step his readers moved: %s", readers(f.c))

	counted := clockBlock(f)["reentrant_reads"].(int) - reads0
	out.expect(counted == 14*len(ra.reads), "reentrant_reads counted %d reads for %d callbacks of 14 readers", counted, len(ra.reads))

	require.Empty(t, out, "every reader of his fight, mid clock step:\n  %s", strings.Join(out, "\n  "))
}

// --- B2: RejoinFight says whose fight --------------------------------------

func TestRejoinFightSaysWhose(t *testing.T) {
	f := humanFight(t, 1462, nil)
	f.c.SetPlayer("p:1")

	clockQuarry(t, f, "v:1", 80, 40, 5000, "r:1", 5000)
	f.profiles.byID["r:1"] = Profile{Row: RisenRow, Dead: true}
	f.add(t, "h:1", 5000, Profile{Row: RisenRow, Dead: true})

	f.c.Advance(0.5)
	require.NotNil(t, clockFightFor(f, "v:1"))
	require.True(t, f.c.Participates("h:1"))

	stood := func(id string, x float64) *fakeWatcher {
		w := &fakeWatcher{id: id, x: x, y: 40}
		f.watchers[id] = w
		f.bodies.known[id] = &fakeBody{health: 5000, maxHealth: 5000}
		f.profiles.byID[id] = Profile{Row: RisenRow, Dead: true}

		return w
	}

	in, his := f.c.RejoinFight("r:1", stood("r:2", 81))
	require.True(t, in, "a man in the village's fight stands again into it")
	require.False(t, his, "and it is not his fight")

	in, his = f.c.RejoinFight("h:1", stood("h:2", 41))
	require.True(t, in && his, "a man in his fight stands again into HIS")

	in, his = f.c.RejoinFight("x:9", stood("x:10", 60))
	require.False(t, in || his, "a man in no fight rejoins none")

	in, his = f.c.RejoinFight("h:2", nil)
	require.False(t, in || his, "nobody standing rejoins nothing")

	// Unbound (the legacy rule): the one fight is his.
	g := humanFight(t, 1462, nil)
	g.add(t, "d:1", 5000, Profile{Row: RisenRow, Dead: true})
	g.c.Advance(0.5)

	in, his = g.c.RejoinFight("d:1", &fakeWatcher{id: "d:2", x: 41, y: 40})
	require.True(t, in && his)
}
