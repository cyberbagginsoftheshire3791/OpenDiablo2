package d2world

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
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

	return b2aCombatFixtureOn(t, d2rand.Derive(b2aWorldSeed, d2rand.StreamCombat))
}

func b2aCombatFixtureOn(t *testing.T, seed int64) *resolverFight {
	t.Helper()

	f := humanFight(t, seed, nil)

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
// one -- on another game's stream, so its dice can only be the snapshot's --
// into which snap is restored as the saved game's (b2aWorldSeed).
func b2aCombatCopy(t *testing.T, snap CombatSnapshot) (*resolverFight, error) {
	t.Helper()

	f := b2aCombatFixture(t)
	f.c.Close()

	dials := DefaultCombatDials()
	dials.PlayerControl = PlayerControlHuman
	clock := NewClock(DefaultClockDials())
	t.Cleanup(clock.Close)

	f.c = NewCombat(clock, f.notice, f.fitness, f.illum, f.bodies,
		f.profiles, f.animator, f.morale, f.chases, d2rand.Derive(99, d2rand.StreamCombat), dials)
	t.Cleanup(f.c.Close)

	return f, f.c.Restore(snap, b2aWorldSeed)
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

// b2aCombatClasses is every field of the combat model and of the records it
// keeps, labelled.
func b2aCombatClasses() []b2aClass {
	const fightOnly = "T: reset by end(); Snapshot refuses unless it is"

	return []b2aClass{
		{Combat{}, map[string]string{
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
		}},
		{action{}, map[string]string{
			"round": "S:last_actions[0].round", "attacker": "S:last_actions[0].attacker",
			"target": "S:last_actions[0].target", "roll": "S:last_actions[0].roll", "mod": "S:last_actions[0].mod",
			"score": "S:last_actions[0].score", "band": "S:last_actions[0].band", "base": "S:last_actions[0].base",
			"damage": "S:last_actions[0].damage", "targetHealthAfter": "S:last_actions[0].target_health_after",
			"targetHasBody": "S:last_actions[0].target_has_body", "advantageWhy": "S:last_actions[0].advantage_why",
			"reaction": "S:last_actions[0].reaction", "bandRolled": "S:last_actions[0].band_rolled",
			"blocked": "S:last_actions[0].blocked", "absorbed": "S:last_actions[0].absorbed",
			"class": "S:last_actions[0].class", "weapon": "S:last_actions[0].weapon",
		}},
		{RoundRow{}, map[string]string{
			"Encounter": "S:last_round.encounter", "Round": "S:last_round.round",
			"DecideSeconds": "S:last_round.decide_seconds", "Action": "S:last_round.action", "Move": "S:last_round.move",
		}},
		{PaceRow{}, map[string]string{
			"Encounter": "S:last_pace.encounter", "Rounds": "S:last_pace.rounds",
			"WallSeconds": "S:last_pace.wall_seconds", "DecideSeconds": "S:last_pace.decide_seconds",
			"HealthOpen": "S:last_pace.health_open", "HealthClose": "S:last_pace.health_close",
			"EndReason": "S:last_pace.end_reason", "Enemies": "S:last_pace.enemies", "Kinds": "S:last_pace.kinds",
			"Initiator": "S:last_pace.initiator", "Surprised": "S:last_pace.surprised",
			"Control": "S:last_pace.control", "Commits": "S:last_pace.commits",
		}},
		{BlowLine{}, map[string]string{
			"Round": "S:blow_log[0].round", "Attacker": "S:blow_log[0].attacker", "Target": "S:blow_log[0].target",
			"Band": "S:blow_log[0].band", "Damage": "S:blow_log[0].damage", "Reaction": "S:blow_log[0].reaction",
			"Killed": "S:blow_log[0].killed",
		}},
	}
}

func TestCombatSnapshotFieldsClassified(t *testing.T) {
	f := b2aCombatFixture(t)
	snap, err := f.c.Snapshot()
	require.NoError(t, err)

	for _, c := range b2aCombatClasses() {
		b2aClassified(t, c.kind, snap, c.fields)
	}
}

// Every field labelled T (transient: empty at every save) makes Snapshot
// refuse while it is set -- checked by setting each on its own, on a model
// between fights that saves (the B2a review's B3). A field labelled T that
// checkBetweenFights (or Fighting) does not look at would be dropped by a save
// in silence, and fails here.
func TestCombatSnapshotRefusesEveryTransient(t *testing.T) {
	b2aTransientsRefused(t, b2aCombatClasses()[0], func() (interface{}, func() error) {
		f := b2aCombatFixture(t)

		return f.c, func() error { _, err := f.c.Snapshot(); return err }
	})
}

func TestCombatSnapshotRoundTrip(t *testing.T) {
	orig := b2aCombatFixture(t)

	s, err := orig.c.Snapshot()
	require.NoError(t, err)

	snap := b2aThroughJSON(t, s)
	require.Equal(t, s, snap, "the snapshot survives JSON exactly")
	require.Positive(t, snap.RNG.Draws, "the fight drew")

	// Observability first (the B2a review's C7): what the save carries, the
	// provider reports.
	require.Equal(t, CommitStrike, snap.LastActionVerb)
	require.Equal(t, snap.LastActionVerb, orig.c.HarnessState()["last_action_verb"], "the provider reports the saved verb")
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
		orig := newResolverFight(t, d2rand.Derive(b2aWorldSeed, d2rand.StreamCombat))
		orig.c.lastPace.Kinds = kinds

		cp := newResolverFight(t, d2rand.Derive(7, d2rand.StreamCombat))

		s, err := orig.c.Snapshot()
		require.NoError(t, err)
		require.NoError(t, cp.c.Restore(b2aThroughJSON(t, s), b2aWorldSeed))

		require.Equal(t, b2aJSON(t, orig.c.HarnessState()), b2aJSON(t, cp.c.HarnessState()), "kinds %#v", kinds)
		require.Equal(t, b2aJSON(t, orig.c.Tactical()), b2aJSON(t, cp.c.Tactical()))
	}
}

// A wall-clock GAME seed is past 2^53. Combat's stream runs on the seed Derive
// gives it; what must survive is the check against the game seed, read as an
// int64 (B1 notes, section 2), with the stream drawn part way so the count
// matters too. The stream state's own trip through JSON at a seed past 2^53
// is d2rand's TestStreamStateSeedSurvivesJSON.
func TestCombatSnapshotSeedSurvivesJSON(t *testing.T) {
	const world = int64(1)<<62 + 54321

	require.NotEqual(t, world, int64(float64(world)), "the control: float64 loses this seed")

	orig := newResolverFight(t, d2rand.Derive(world, d2rand.StreamCombat))
	for i := 0; i < 5; i++ {
		orig.c.rng.Float64()
	}

	s, err := orig.c.Snapshot()
	require.NoError(t, err)

	raw, err := json.Marshal(s)
	require.NoError(t, err)

	var loose map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &loose))
	require.IsType(t, "", loose["rng"].(map[string]interface{})["seed"], "the seed is written as a string")

	var typed CombatSnapshot
	require.NoError(t, json.Unmarshal(raw, &typed))
	require.Equal(t, s, typed, "and read back exactly")

	cp := newResolverFight(t, d2rand.Derive(7, d2rand.StreamCombat))
	require.NoError(t, cp.c.Restore(typed, world))
	require.Equal(t, orig.c.rng.Float64(), cp.c.rng.Float64(), "the restored stream is the game's, at its sixth value")

	lost := newResolverFight(t, d2rand.Derive(7, d2rand.StreamCombat))
	require.Error(t, lost.c.Restore(typed, int64(float64(world))), "the game seed read through float64 is another game")
}

// THE MUTATION THE B2a REVIEW NAMED (B1): the rising's and combat's stream
// blocks swapped. Each restored into the other's system is refused -- before
// this, both restored and each ran on the other's dice from its first roll.
func TestCombatAndRisingRefuseEachOthersStream(t *testing.T) {
	good, err := b2aCombatFixture(t).c.Snapshot()
	require.NoError(t, err)

	rw := b2aRisingFixture(t)
	rising, corpses := rw.r.Snapshot(), rw.w.c.Snapshot()

	good.RNG, rising.RNG = rising.RNG, good.RNG

	_, err = b2aCombatCopy(t, good)
	require.ErrorIs(t, err, d2rand.ErrStreamState, "combat with the rising's stream")

	_, err = b2aRisingCopy(rw, corpses, rising)
	require.ErrorIs(t, err, d2rand.ErrStreamState, "the rising with combat's stream")

	// The control: swapped back, both restore.
	good.RNG, rising.RNG = rising.RNG, good.RNG

	_, err = b2aCombatCopy(t, good)
	require.NoError(t, err)

	_, err = b2aRisingCopy(rw, corpses, rising)
	require.NoError(t, err)
}

// Saving is refused during a fight, and whenever something a fight leaves for
// the game screen has not been taken -- a snapshot that dropped it would lose
// experience or world minutes.
func TestCombatSnapshotRefusals(t *testing.T) {
	f := humanFight(t, d2rand.Derive(b2aWorldSeed, d2rand.StreamCombat), nil)
	b2aCombatAdd(t, f, "e:1", 41, 40, 50, Profile{Row: "wolves", Group: "g:1", Speed: 2, DamageMin: 2, DamageMax: 4})
	f.open(t)

	_, err := f.c.Snapshot()
	require.True(t, errors.Is(err, ErrCombatFighting), "mid-fight: %v", err)

	good, err := b2aCombatFixture(t).c.Snapshot()
	require.NoError(t, err)
	require.Error(t, f.c.Restore(good, b2aWorldSeed), "and a fight is never restored into")
	require.Error(t, f.c.Validate(good, b2aWorldSeed), "nor validated into")

	for name, dirty := range map[string]func(c *Combat){
		"experience not taken": func(c *Combat) { c.xpEvents = append(c.xpEvents, XPEvent{Kind: "slain"}) },
		"minutes owed":         func(c *Combat) { c.owedMinutes = 1 },
		"a blow mid-flight":    func(c *Combat) { c.killerIsPlayer = true },
		"the pace window open": func(c *Combat) { c.paceOpen = true },
		"decision seconds":     func(c *Combat) { c.decisionSeconds = 0.5 },
		"wall seconds":         func(c *Combat) { c.wallSeconds = 0.5 },
		"pace commits":         func(c *Combat) { c.paceCommits = 1 },
		"round seconds":        func(c *Combat) { c.decisionSecondsRound = 0.5 },
		"the health it opened": func(c *Combat) { c.paceHealthOpen = 40 },
	} {
		cf := b2aCombatFixture(t)
		dirty(cf.c)

		_, err := cf.c.Snapshot()
		require.Error(t, err, name)
		require.False(t, errors.Is(err, ErrCombatFighting), "%s is not a fight", name)
	}

	// Nor into a model that holds what the snapshot never carries: restored
	// over, it would stay owed to a resumed game.
	for name, dirty := range map[string]func(c *Combat){
		"experience not taken": func(c *Combat) { c.xpEvents = append(c.xpEvents, XPEvent{Kind: "slain"}) },
		"minutes owed":         func(c *Combat) { c.owedMinutes = 1 },
		"the pace window open": func(c *Combat) { c.paceOpen = true },
	} {
		fresh := newResolverFight(t, d2rand.Derive(7, d2rand.StreamCombat))
		dirty(fresh.c)
		require.Error(t, fresh.c.Validate(good, b2aWorldSeed), "validate into a model with %s", name)
		require.Error(t, fresh.c.Restore(good, b2aWorldSeed), "restore into a model with %s", name)
		require.Equal(t, 1, fresh.c.nextID, "%s: a refused restore changes nothing", name)
	}

	for name, bad := range map[string]func(s *CombatSnapshot){
		"next_id not one past started": func(s *CombatSnapshot) { s.NextID++ },
		"a fight not ended":            func(s *CombatSnapshot) { s.Ended-- },
		"a negative count":             func(s *CombatSnapshot) { s.Joined = -1 },
		"another game's stream":        func(s *CombatSnapshot) { s.RNG.Seed = d2rand.Derive(7, d2rand.StreamCombat) },
		"more draws than a save holds": func(s *CombatSnapshot) { s.RNG.Draws = d2rand.MaxDraws + 1 },
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

	try := func(raw []byte) (string, error) {
		var s CombatSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		cp, err := b2aCombatCopy(t, s)
		if err != nil {
			return "", err
		}

		return b2aCombatSteps(t, cp), nil
	}

	b2aExercised(t, b2aSweep(t, snap, ref, nil, try), b2aCombatClasses()...)

	// next_id, started and ended are only ever refused alone (each is checked
	// against the others); lost together, consistently, they must still show.
	b2aMustDiverge(t, snap, ref, map[string]func(s *CombatSnapshot){
		"a fresh model's ids and counts": func(s *CombatSnapshot) { s.NextID, s.Started, s.Ended = 1, 0, 0 },
		"one fight more":                 func(s *CombatSnapshot) { s.NextID, s.Started, s.Ended = s.NextID+1, s.Started+1, s.Ended+1 },
	}, try)
}
