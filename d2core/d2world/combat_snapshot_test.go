package d2world

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// b2aCombatAdd puts an enemy at (x, y) with a body and a profile and steps the
// notice model until it has noticed the player.
func b2aCombatAdd(t *testing.T, f *resolverFight, id string, x, y float64, health int, p Profile) {
	t.Helper()

	w := &fakeWatcher{id: id, x: x, y: y}
	f.watchers[id] = w
	f.bodies.known[id] = &fakeBody{health: health, maxHealth: health}
	f.profiles.byID[id] = p

	aware(t, f.notice, w, f.target)
}

// b2aCombatFixture is a human-controlled fight fought to its end, the way the
// game fights one, leaving every record a fight leaves: a commit refused with
// no turn waiting, a far wolf declined at the start, a dog that joins a fight
// already running, a hold by the settable field, a strike and an end by the
// input path, the decision clock running while a turn waits, and the player's
// own blow ending it -- so the round's verb is never cleared. The game takes
// the fight's experience every frame; so does this, before the snapshot.
func b2aCombatFixture(t *testing.T) *resolverFight {
	t.Helper()

	f := humanFight(t, 1462, nil)

	require.Error(t, f.c.Commit(CommitStrike, ""), "no turn is waiting")

	b2aCombatAdd(t, f, "e:far", 46, 40, 30, Profile{Row: "wolves", Group: "g:far", Speed: 2, DamageMin: 2, DamageMax: 4})
	b2aCombatAdd(t, f, "e:1", 41, 40, 5, Profile{Row: "wolves", Group: "g:1", Speed: 2, DamageMin: 2, DamageMax: 4})
	f.open(t)

	b2aCombatAdd(t, f, "e:2", 39, 40, 5, Profile{Row: "dogs", Group: "g:2", Speed: 3, DamageMin: 1, DamageMax: 3})

	f.round()
	require.True(t, f.c.Awaiting())
	f.c.Wait(0.75)
	f.set(t, "commit", CommitHold)

	f.round()
	require.True(t, f.c.Awaiting())
	f.c.Wait(0.5)
	require.NoError(t, f.c.Commit(CommitStrike, "e:1"))
	require.NoError(t, f.c.Commit(CommitEnd, ""))

	for i := 0; i < 6 && f.c.Fighting(); i++ {
		f.round()

		if f.c.Awaiting() {
			f.c.Wait(0.25)
			require.NoError(t, f.c.Commit(CommitStrike, "e:2"))

			if f.c.Fighting() {
				require.NoError(t, f.c.Commit(CommitEnd, ""))
			}
		}
	}

	require.False(t, f.c.Fighting(), "the fixture's fight ends")
	require.Equal(t, "enemies_dead", f.c.endedReason)
	require.Equal(t, CommitStrike, f.c.lastActionVerb, "his own blow ended it, so the verb was never cleared")
	require.Positive(t, f.c.joined, "the dog joined the fight already running")
	require.NotEmpty(t, f.c.TakeXPEvents(), "the fight earned experience, taken as the game takes it")

	return f
}

// b2aCombatCopy is the same world with its combat model replaced by a FRESH
// one -- on another seed, so its dice can only be the snapshot's -- into which
// snap is restored.
func b2aCombatCopy(t *testing.T, snap CombatSnapshot) (*resolverFight, error) {
	t.Helper()

	f := b2aCombatFixture(t)
	f.c.Close()

	dials := DefaultCombatDials()
	dials.PlayerControl = PlayerControlHuman
	f.c = NewCombat(NewClock(DefaultClockDials()), f.notice, f.fitness, f.illum, f.bodies,
		f.profiles, f.animator, f.morale, f.chases, 99, dials)
	t.Cleanup(f.c.Close)

	return f, f.c.Restore(snap)
}

// b2aCombatSteps is the next fight, under the policy (the harness's
// player_control, set on both sides): a wolf arrives, the far one is declined
// again, and they fight it out. Every frame reads the provider, the overlay's
// view, what the game screen takes, and what the fight did to the bodies,
// the packs and the chases.
func b2aCombatSteps(t *testing.T, f *resolverFight) string {
	t.Helper()

	var trace strings.Builder

	observe := func() {
		trace.WriteString(b2aJSON(t, f.c.HarnessState()))
		trace.WriteString(b2aJSON(t, f.c.Tactical()))
		trace.WriteString(b2aJSON(t, f.c.TakeXPEvents()))
		trace.WriteString(b2aJSON(t, f.c.TakeRoundMinutes()))
		trace.WriteString(b2aJSON(t, []interface{}{f.animator.acts, f.morale.hurts, f.chases.released}))

		for _, id := range []string{"p:1", "e:1", "e:2", "e:3"} {
			if b := f.bodies.known[id]; b != nil {
				trace.WriteString(b2aJSON(t, []interface{}{id, b.health}))
			}
		}
	}

	observe() // the first frame after a load

	f.set(t, "player_control", PlayerControlPolicy)
	f.morale.morale["g:3"] = 60
	b2aCombatAdd(t, f, "e:3", 41, 41, 40, Profile{Row: "wolves", Group: "g:3", Speed: 2, DamageMin: 3, DamageMax: 6})

	for i := 0; i < 14; i++ {
		f.c.Advance(1.0)
		observe()
	}

	return trace.String()
}

func TestCombatSnapshotFieldsClassified(t *testing.T) {
	f := b2aCombatFixture(t)
	snap, err := f.c.Snapshot()
	require.NoError(t, err)

	const fightOnly = "T: reset by end(); Snapshot refuses unless it is"

	b2aClassified(t, Combat{}, snap, map[string]string{
		"dials":    "W: the shipped dials, set by the game at construction; harness writes are test setup",
		"clock":    "W: the clock it reads; restored on its own",
		"notice":   "W: the notice model it asks who is aware",
		"fitness":  "W: the squads, looked up by id",
		"illum":    "W: the light model it samples",
		"bodies":   "W: the body registry (B2b/B4 restore the bodies)",
		"profiles": "W: the spawn tables' profiles",
		"animator": "W: the game screen's sprites",
		"morale":   "W: the spawn tables' morale",
		"chases":   "W: pursuit, which a death releases",
		"kits":     "W: what each combatant carries (the sidecar kit)",
		"edges":    "W: his talents as numbers, applied again from his progress",
		"corpses":  "W: the registry the dead fall into, restored on its own",
		"stepper":  "W: the game screen's walk for a paced fight",

		"xpEvents":       "T: the game takes them every frame (TakeXPEvents); Snapshot refuses unless empty",
		"killerIsPlayer": "T: set and cleared inside one blow's resolution; Snapshot refuses unless clear",
		"owedMinutes":    "T: the game takes them every frame (TakeRoundMinutes); Snapshot refuses unless zero",
		"encounter":      "T: the fight itself; Snapshot refuses during one (ErrCombatFighting)",

		"decisionSeconds":      fightOnly,
		"decisionSecondsRound": fightOnly,
		"paceOpen":             fightOnly,
		"wallSeconds":          fightOnly,
		"paceHealthOpen":       fightOnly,
		"paceCommits":          fightOnly,

		"blowLog":            "S:blow_log",
		"stepsOrdered":       "S:steps_ordered",
		"rng":                "S:rng",
		"nextID":             "S:next_id",
		"lastActions":        "S:last_actions",
		"actionsRound":       "S:actions_round",
		"started":            "S:started",
		"ended":              "S:ended",
		"rounds":             "S:rounds",
		"commitsRefused":     "S:commits_refused",
		"commitsByInput":     "S:commits_by_input",
		"commitsByField":     "S:commits_by_field",
		"lastActionVerb":     "S:last_action_verb",
		"lastRound":          "S:last_round",
		"lastPace":           "S:last_pace",
		"declines":           "S:declines",
		"actions":            "S:actions",
		"endedReason":        "S:ended_reason",
		"endedEnemiesDead":   "S:ended_enemies_dead",
		"endedDawn":          "S:ended_dawn",
		"endedPlayerDead":    "S:ended_player_dead",
		"endedDisengaged":    "S:ended_disengaged",
		"endedRouted":        "S:ended_routed",
		"joined":             "S:joined",
		"quickResolved":      "S:quick_resolved",
		"lastQuickAdvantage": "S:last_quick_advantage",
	})

	b2aClassified(t, action{}, snap, map[string]string{
		"round": "S:last_actions[0].round", "attacker": "S:last_actions[0].attacker",
		"target": "S:last_actions[0].target", "roll": "S:last_actions[0].roll", "mod": "S:last_actions[0].mod",
		"score": "S:last_actions[0].score", "band": "S:last_actions[0].band", "base": "S:last_actions[0].base",
		"damage": "S:last_actions[0].damage", "targetHealthAfter": "S:last_actions[0].target_health_after",
		"targetHasBody": "S:last_actions[0].target_has_body", "advantageWhy": "S:last_actions[0].advantage_why",
		"reaction": "S:last_actions[0].reaction", "bandRolled": "S:last_actions[0].band_rolled",
		"blocked": "S:last_actions[0].blocked", "absorbed": "S:last_actions[0].absorbed",
		"class": "S:last_actions[0].class", "weapon": "S:last_actions[0].weapon",
	})

	b2aClassified(t, RoundRow{}, snap, map[string]string{
		"Encounter": "S:last_round.encounter", "Round": "S:last_round.round",
		"DecideSeconds": "S:last_round.decide_seconds", "Action": "S:last_round.action", "Move": "S:last_round.move",
	})

	b2aClassified(t, PaceRow{}, snap, map[string]string{
		"Encounter": "S:last_pace.encounter", "Rounds": "S:last_pace.rounds",
		"WallSeconds": "S:last_pace.wall_seconds", "DecideSeconds": "S:last_pace.decide_seconds",
		"HealthOpen": "S:last_pace.health_open", "HealthClose": "S:last_pace.health_close",
		"EndReason": "S:last_pace.end_reason", "Enemies": "S:last_pace.enemies", "Kinds": "S:last_pace.kinds",
		"Initiator": "S:last_pace.initiator", "Surprised": "S:last_pace.surprised",
		"Control": "S:last_pace.control", "Commits": "S:last_pace.commits",
	})

	b2aClassified(t, BlowLine{}, snap, map[string]string{
		"Round": "S:blow_log[0].round", "Attacker": "S:blow_log[0].attacker", "Target": "S:blow_log[0].target",
		"Band": "S:blow_log[0].band", "Damage": "S:blow_log[0].damage", "Reaction": "S:blow_log[0].reaction",
		"Killed": "S:blow_log[0].killed",
	})
}

func TestCombatSnapshotRoundTrip(t *testing.T) {
	orig := b2aCombatFixture(t)

	s, err := orig.c.Snapshot()
	require.NoError(t, err)

	snap := b2aThroughJSON(t, s)
	require.Equal(t, s, snap, "the snapshot survives JSON exactly")
	require.Positive(t, snap.RNG.Draws, "the fight drew")
	require.NotEmpty(t, snap.LastActions)
	require.NotEmpty(t, snap.BlowLog)

	cp, err := b2aCombatCopy(t, snap)
	require.NoError(t, err)

	again, err := cp.c.Snapshot()
	require.NoError(t, err)
	require.Equal(t, snap, again, "restored is the same model, dice included")
	require.Equal(t, b2aCombatSteps(t, orig), b2aCombatSteps(t, cp), "and it fights the next fight the same")
}

// A new game saved at once: no fight yet, so the pace row's kinds are nil and
// the provider writes null -- and a game whose last paced fight had no rowed
// enemy writes []. Both must come back as they went.
func TestCombatSnapshotOfANewGame(t *testing.T) {
	for _, kinds := range [][]string{nil, {}} {
		orig := newResolverFight(t, 1462)
		orig.c.lastPace.Kinds = kinds

		cp := newResolverFight(t, 7)

		s, err := orig.c.Snapshot()
		require.NoError(t, err)
		require.NoError(t, cp.c.Restore(b2aThroughJSON(t, s)))

		require.Equal(t, b2aJSON(t, orig.c.HarnessState()), b2aJSON(t, cp.c.HarnessState()), "kinds %#v", kinds)
		require.Equal(t, b2aJSON(t, orig.c.Tactical()), b2aJSON(t, cp.c.Tactical()))
	}
}

// Saving is refused during a fight, and whenever something a fight leaves for
// the game screen has not been taken -- a snapshot that dropped it would lose
// experience or world minutes.
func TestCombatSnapshotRefusals(t *testing.T) {
	f := humanFight(t, 1462, nil)
	b2aCombatAdd(t, f, "e:1", 41, 40, 50, Profile{Row: "wolves", Group: "g:1", Speed: 2, DamageMin: 2, DamageMax: 4})
	f.open(t)

	_, err := f.c.Snapshot()
	require.True(t, errors.Is(err, ErrCombatFighting), "mid-fight: %v", err)

	good, err := b2aCombatFixture(t).c.Snapshot()
	require.NoError(t, err)
	require.Error(t, f.c.Restore(good), "and a fight is never restored into")

	for name, dirty := range map[string]func(c *Combat){
		"experience not taken": func(c *Combat) { c.xpEvents = append(c.xpEvents, XPEvent{Kind: "slain"}) },
		"minutes owed":         func(c *Combat) { c.owedMinutes = 1 },
		"a blow mid-flight":    func(c *Combat) { c.killerIsPlayer = true },
		"the pace window open": func(c *Combat) { c.paceOpen = true },
		"decision seconds":     func(c *Combat) { c.decisionSeconds = 0.5 },
		"wall seconds":         func(c *Combat) { c.wallSeconds = 0.5 },
		"pace commits":         func(c *Combat) { c.paceCommits = 1 },
	} {
		cf := b2aCombatFixture(t)
		dirty(cf.c)

		_, err := cf.c.Snapshot()
		require.Error(t, err, name)
		require.False(t, errors.Is(err, ErrCombatFighting), "%s is not a fight", name)
	}

	for name, bad := range map[string]func(s *CombatSnapshot){
		"next_id not one past started": func(s *CombatSnapshot) { s.NextID++ },
		"a fight not ended":            func(s *CombatSnapshot) { s.Ended-- },
		"a negative count":             func(s *CombatSnapshot) { s.Joined = -1 },
	} {
		s := good
		bad(&s)

		_, err := b2aCombatCopy(t, s)
		require.Error(t, err, name)
	}
}

func TestCombatSnapshotEveryFieldIsLoadBearing(t *testing.T) {
	orig := b2aCombatFixture(t)
	snap, err := orig.c.Snapshot()
	require.NoError(t, err)

	ref := b2aCombatSteps(t, orig)

	b2aSweep(t, snap, ref, nil, func(raw []byte) (string, error) {
		var s CombatSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		cp, err := b2aCombatCopy(t, s)
		if err != nil {
			return "", err
		}

		return b2aCombatSteps(t, cp), nil
	})
}
