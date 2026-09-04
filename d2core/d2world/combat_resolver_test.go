package d2world

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// THE RESOLVER'S UNIT TESTS. No game, no MPQs, every band and branch driven by
// a fake -- which is the only place several of these rules can be tested at
// all: "Shaken blocks Riposte" and "dark-into-light" are both unobservable in
// a real v0 build (see combat_resolver.go on each), so if they are not proved
// here they are not proved.
//
// HOW EXPECTATIONS ARE DERIVED, and it is a deliberate departure from the
// step-4 brief. The brief said to replay the seeded RNG in the test and
// predict each roll. That would pin this suite to the internals of
// rand.Shuffle -- the initiative tie-break draws from the same stream before
// the first blow -- and a Go release that changed the shuffle would break
// tests that are not about shuffling. Instead each test asserts the RELATION
// between the fields the system reports: score == clamp(roll+mod),
// damage == max(1, base*factor), mod == +/-AdvantageShift. The test still
// chooses the arithmetic and the system still reports the inputs, which is the
// property the brief was after, and determinism is proved directly by running
// two identically seeded fights and comparing their logs.

// tileIllumination is a light model with a level PER TILE, which the existing
// fakeIllumination (one level everywhere) cannot express -- and a shared level
// is exactly the case in which dark-into-light can never fire.
type tileIllumination struct {
	levels   map[[2]int]float64
	fallback float64
}

func (i *tileIllumination) Level(tileX, tileY int) float64 {
	if v, ok := i.levels[[2]int{tileX, tileY}]; ok {
		return v
	}

	return i.fallback
}

// fakeProfiles is the spawn tables' answer without the spawn tables.
type fakeProfiles struct{ byID map[string]Profile }

func (f *fakeProfiles) ProfileOf(id string) (Profile, bool) {
	p, ok := f.byID[id]

	return p, ok
}

// recordingAnimator records what the resolver asked to be drawn, in order.
type recordingAnimator struct{ acts []string }

func (a *recordingAnimator) Animate(id string, act CombatAct) {
	name := "?"

	switch act {
	case ActSwing:
		name = "swing"
	case ActHit:
		name = "hit"
	case ActDie:
		name = "die"
	}

	a.acts = append(a.acts, id+":"+name)
}

// resolverFight is a wired-up Combat: a player with a body, enemies with
// bodies and profiles, per-tile light, and a recording animator.
type resolverFight struct {
	c        *Combat
	notice   *Notice
	target   *fakeQuarry
	fitness  *fakeFitness
	bodies   *fakeBodies
	illum    *tileIllumination
	animator *recordingAnimator
	profiles *fakeProfiles
	watchers map[string]*fakeWatcher
}

func newResolverFight(t *testing.T, seed int64) *resolverFight {
	t.Helper()

	f := &resolverFight{
		notice:  NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials()),
		target:  &fakeQuarry{id: "p:1", x: 40, y: 40},
		fitness: &fakeFitness{reaction: true},
		// The player has a body HERE, which is the whole of step 4's
		// "you can lose": before it, Bodies answered for monsters only.
		bodies:   &fakeBodies{known: map[string]*fakeBody{"p:1": {health: 240, maxHealth: 240}}},
		illum:    &tileIllumination{levels: map[[2]int]float64{}},
		animator: &recordingAnimator{},
		profiles: &fakeProfiles{byID: map[string]Profile{}},
		watchers: map[string]*fakeWatcher{},
	}

	f.c = NewCombat(NewClock(DefaultClockDials()), f.notice, f.fitness, f.illum, f.bodies,
		f.profiles, f.animator, seed, DefaultCombatDials())

	t.Cleanup(f.c.Close)

	return f
}

// add puts an enemy one tile east of the player, gives it a body and a
// profile, and steps the notice model until it has noticed.
func (f *resolverFight) add(t *testing.T, id string, health int, p Profile) *fakeWatcher {
	t.Helper()

	w := &fakeWatcher{id: id, x: 41, y: 40}
	f.watchers[id] = w
	f.bodies.known[id] = &fakeBody{health: health, maxHealth: health}

	if p.Row != "" || p.Group != "" {
		f.profiles.byID[id] = p
	}

	aware(t, f.notice, w, f.target)

	return w
}

// open starts the fight. A single Advance either starts an encounter or ticks
// one, never both, so this resolves no round.
func (f *resolverFight) open(t *testing.T) {
	t.Helper()

	f.c.Advance(1.0)

	require.True(t, f.c.Fighting(), "the fight should have opened")
	require.Equal(t, 1, f.c.Round())
}

// round resolves exactly one round.
func (f *resolverFight) round() { f.c.Advance(1.0) }

func (f *resolverFight) set(t *testing.T, field string, value interface{}) {
	t.Helper()

	require.NoError(t, f.c.HarnessSet(field, value))
}

func (f *resolverFight) health(id string) int { return f.bodies.known[id].health }

// actions reads the blows of the most recent Advance.
func (f *resolverFight) actions(t *testing.T) []map[string]interface{} {
	t.Helper()

	rows, ok := f.c.HarnessState()["actions"].([]map[string]interface{})
	require.True(t, ok, "the provider must report an actions log")

	return rows
}

// expectedDamage is the arithmetic the TEST chooses, against the base and band
// the SYSTEM reports.
func expectedDamage(base int, factor float64) int {
	if d := int(float64(base) * factor); d > 1 {
		return d
	}

	return 1
}

// A BLOW DOES SOMETHING, AND WHAT IT DOES IS BOUNDED. R2 §3 bullet 5 refuses a
// binary miss, so every band -- graze included -- takes at least 1.
func TestResolverEveryBandDoesBoundedDamage(t *testing.T) {
	for _, tc := range []struct {
		band   string
		factor float64
	}{
		{BandGraze, 0.5},
		{BandHit, 1.0},
		{BandCrit, 1.5},
	} {
		t.Run(tc.band, func(t *testing.T) {
			f := newResolverFight(t, 1462)
			f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
			f.open(t)

			f.set(t, "forced_band", tc.band)

			before := f.health("w:1")

			f.round()

			rows := f.actions(t)
			require.NotEmpty(t, rows, "a round must resolve at least one blow")

			total := 0

			for _, row := range rows {
				require.Equal(t, tc.band, row["band"], "the forced band must apply to every blow")

				base, _ := row["base"].(int)
				damage, _ := row["damage"].(int)

				require.Equal(t, expectedDamage(base, tc.factor), damage,
					"damage must be max(1, base*factor); base=%d band=%s", base, tc.band)
				require.GreaterOrEqual(t, damage, 1, "no band does nothing")

				if row["target"] == "w:1" {
					total += damage
				}
			}

			require.Equal(t, before-total, f.health("w:1"),
				"the wolf's health must fall by exactly the damage dealt to it")
		})
	}
}

// THE FLOOR OF 1 IS WHAT MAKES "BOUNDED" TRUE AT THE BOTTOM. Drive the factor
// to zero and a graze still wounds.
func TestResolverGrazeNeverDoesNothing(t *testing.T) {
	f := newResolverFight(t, 7)
	f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
	f.open(t)

	f.set(t, "forced_band", BandGraze)
	f.set(t, "graze_factor", 0.0)

	f.round()

	for _, row := range f.actions(t) {
		require.Equal(t, 1, row["damage"], "a graze at factor 0 is still a wound, never nothing")
	}
}

// The reported roll, mod and score must agree with each other, or a script
// that reads one of them is reading a number the resolver did not use.
func TestResolverScoreIsTheClampedRollPlusMod(t *testing.T) {
	f := newResolverFight(t, 99)
	f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
	f.open(t)

	for i := 0; i < 8; i++ {
		f.round()

		for _, row := range f.actions(t) {
			roll, _ := row["roll"].(int)
			mod, _ := row["mod"].(int)
			score, _ := row["score"].(int)

			require.GreaterOrEqual(t, roll, 1)
			require.LessOrEqual(t, roll, 100)
			require.Equal(t, clampScore(roll+mod), score)
		}
	}
}

// DARK INTO LIGHT, AND THE REASON REPORTED BESIDE THE NUMBER (R2 §3 bullet 7).
// This is one of the two rules that can only ever be tested here: in a real
// build every participant stands on one tile and samples one light level.
func TestResolverAdvantageFiresForTheRightReason(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})

	// The player stands lit; the wolf stands in the dark one tile east.
	f.illum.levels[[2]int{40, 40}] = 0.50
	f.illum.levels[[2]int{41, 40}] = 0.10

	// No Reaction, so the round is exactly two blows. Left available, a graze
	// on the player buys a Riposte and the round is three -- which is the
	// resolver working, and noise in a test about light.
	f.fitness.reaction = false

	f.open(t)
	f.round()

	rows := f.actions(t)
	require.Len(t, rows, 2, "the player's blow and the wolf's")

	byAttacker := map[string]map[string]interface{}{}
	for _, row := range rows {
		byAttacker[row["attacker"].(string)] = row
	}

	wolf := byAttacker["w:1"]
	require.Equal(t, whyDarkIntoLight, wolf["advantage_why"], "the wolf strikes out of the dark at a lit man")
	require.Equal(t, 20, wolf["mod"])

	player := byAttacker["p:1"]
	require.Equal(t, whyLightIntoDark, player["advantage_why"], "and he strikes back into the dark")
	require.Equal(t, -20, player["mod"])
}

// THE NEGATIVE CONTROL, AND IT IS THE ONE THAT MATTERS: move the dial across
// the same two light levels and the verdict must flip off. Without this the
// test above would pass against a rule that always fires.
func TestResolverAdvantageIsTheDialAndNotAConstant(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})

	f.illum.levels[[2]int{40, 40}] = 0.50
	f.illum.levels[[2]int{41, 40}] = 0.10

	f.open(t)

	// Below both levels: nobody is in the dark, so there is no contrast.
	f.set(t, "lit_level", 0.01)
	f.round()

	for _, row := range f.actions(t) {
		require.Equal(t, "", row["advantage_why"], "at lit_level 0.01 everyone is lit: %v", row)
		require.Equal(t, 0, row["mod"])
	}

	// Above both levels: everyone is in the dark, and there is still none.
	f.set(t, "lit_level", 0.99)
	f.round()

	for _, row := range f.actions(t) {
		require.Equal(t, "", row["advantage_why"], "at lit_level 0.99 everyone is dark: %v", row)
		require.Equal(t, 0, row["mod"])
	}

	// Back between them and it fires again -- so the branch is live and the
	// two runs above were the dial talking, not a broken rule.
	f.set(t, "lit_level", 0.30)
	f.round()

	found := false

	for _, row := range f.actions(t) {
		if row["advantage_why"] == whyDarkIntoLight {
			found = true
		}
	}

	require.True(t, found, "with the dial back between the two levels the rule must fire again")
}

// SHAKEN COSTS THE PLAYER ACCURACY (R2 §3 bullet 8), and both reasons can
// apply to one blow.
func TestResolverShakenCostsThePlayerAccuracy(t *testing.T) {
	f := newResolverFight(t, 5)
	f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
	f.open(t)

	f.fitness.shaken = true

	f.round()

	rows := f.actions(t)

	var sawPlayer, sawEnemy bool

	for _, row := range rows {
		if row["attacker"] == "p:1" {
			sawPlayer = true

			require.Equal(t, -15, row["mod"], "Shaken is the player's penalty: %v", row)
			require.Equal(t, whyShaken, row["advantage_why"])
		}

		if row["attacker"] == "w:1" {
			sawEnemy = true

			require.Equal(t, 0, row["mod"], "a beast is never Shaken in v0: %v", row)
		}
	}

	require.True(t, sawPlayer && sawEnemy)
}

// RIPOSTE: A GRAZE ON THE PLAYER IS THE MISS (R2 §3 bullet 6, read against
// bullet 5's refusal of a binary miss). Every branch, in both directions --
// clause 5 of the DoD becomes assertable exactly here.
func TestResolverRiposte(t *testing.T) {
	// build makes a fight in which the enemy's blow is guaranteed to graze
	// and the player takes no action of his own, so the ONLY thing a Riposte
	// can be is the reaction.
	build := func(t *testing.T, tune func(f *resolverFight)) *resolverFight {
		t.Helper()

		f := newResolverFight(t, 1462)
		f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
		f.open(t)
		f.set(t, "forced_band", BandGraze)
		f.set(t, "player_action", PlayerActionHold)

		tune(f)

		f.round()

		return f
	}

	ripostes := func(t *testing.T, f *resolverFight) []map[string]interface{} {
		t.Helper()

		out := []map[string]interface{}{}

		for _, row := range f.actions(t) {
			if row["attacker"] == "p:1" && row["reaction"] == "riposte" {
				out = append(out, row)
			}
		}

		return out
	}

	t.Run("a graze on the player is answered", func(t *testing.T) {
		f := build(t, func(f *resolverFight) { f.fitness.reaction = true })

		got := ripostes(t, f)
		require.Len(t, got, 1, "one graze, one answer")
		require.Equal(t, "w:1", got[0]["target"], "the answer goes to the thing that grazed him")

		// It is reported on the enemy's blow too, as what triggered it.
		var tagged bool

		for _, row := range f.actions(t) {
			if row["attacker"] == "w:1" && row["reaction"] == "riposte" {
				tagged = true
			}
		}

		require.True(t, tagged, "the triggering blow must carry the reaction too")
	})

	// CLAUSE 5 OF THE DoD: at fatigue >= 75 the resolver offers no Reaction.
	// It is READ from the meters, never recomputed (M4.2 ask 1).
	t.Run("no reaction available means no riposte", func(t *testing.T) {
		f := build(t, func(f *resolverFight) { f.fitness.reaction = false })
		require.Empty(t, ripostes(t, f))
	})

	// The rule that cannot be seen in a real build, tested in the only place
	// it can be: Shaken blocks the Reaction independently of clause 5.
	t.Run("shaken blocks the riposte even with a reaction available", func(t *testing.T) {
		f := build(t, func(f *resolverFight) {
			f.fitness.reaction = true
			f.fitness.shaken = true
		})
		require.Empty(t, ripostes(t, f))
	})

	// One per round (R2 §3 bullet 6's own cap): two grazing wolves, one
	// answer.
	t.Run("one riposte per round however many graze", func(t *testing.T) {
		f := newResolverFight(t, 1462)
		f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})

		second := f.add(t, "w:2", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
		second.x, second.y = 39, 40

		f.open(t)
		f.set(t, "forced_band", BandGraze)
		f.set(t, "player_action", PlayerActionHold)
		f.round()

		require.Len(t, ripostes(t, f), 1, "two grazes, still one Reaction")
	})

	// D8 §9: a player caught head-down loses his Reaction for round one.
	t.Run("no riposte in a surprised round one", func(t *testing.T) {
		f := newResolverFight(t, 1462)
		f.fitness.activity = ActivityForage
		f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
		f.open(t)
		f.set(t, "forced_band", BandGraze)
		f.set(t, "player_action", PlayerActionHold)

		f.round()
		require.Empty(t, ripostes(t, f), "round one was surprised")

		// And it comes back in round two, so the block was the branch and not
		// a Reaction that never worked.
		f.round()
		require.Len(t, ripostes(t, f), 1, "the Reaction returns in round two")
	})
}

// D8 §9's ORDER: packs by descending authored Speed, the player first.
func TestResolverOrderIsPacksBySpeedDescending(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "slow", 5000, Profile{Group: "g:slow", Row: "boar", Speed: 1, DamageMin: 8, DamageMax: 14})
	f.add(t, "fast", 5000, Profile{Group: "g:fast", Row: "dogs", Speed: 3, DamageMin: 3, DamageMax: 6})
	f.add(t, "mid", 5000, Profile{Group: "g:mid", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
	f.open(t)

	require.Equal(t, []string{"p:1", "fast", "mid", "slow"}, f.c.Order(),
		"the player first, then packs in descending Speed")
}

// Two live groups can share a row -- two dog packs are two packs -- so the
// pack key must be the GROUP. Members inside one pack go in sorted id order.
func TestResolverPacksAreGroupsNotRows(t *testing.T) {
	f := newResolverFight(t, 1462)

	for _, m := range []string{"dogB2", "dogB1"} {
		w := f.add(t, m, 5000, Profile{Group: "g:2", Row: "dogs", Speed: 3, DamageMin: 3, DamageMax: 6})
		w.x, w.y = 39, 40
	}

	for _, m := range []string{"dogA2", "dogA1"} {
		f.add(t, m, 5000, Profile{Group: "g:1", Row: "dogs", Speed: 3, DamageMin: 3, DamageMax: 6})
	}

	f.open(t)

	order := f.c.Order()
	require.Equal(t, "p:1", order[0])

	// Both packs share the row "dogs" and the same Speed. Whichever pack the
	// tie-break puts first, its members must be adjacent and sorted: a
	// collapsed lookup keyed on the row name would interleave them.
	rest := order[1:]
	require.Len(t, rest, 4)
	require.True(t,
		(rest[0] == "dogA1" && rest[1] == "dogA2" && rest[2] == "dogB1" && rest[3] == "dogB2") ||
			(rest[0] == "dogB1" && rest[1] == "dogB2" && rest[2] == "dogA1" && rest[3] == "dogA2"),
		"packs must stay whole and their members sorted, got %v", rest)
}

// D8 §9's CAUGHT HEAD-DOWN branch, all four stances.
func TestResolverSurpriseIsTheStanceAtTheMomentOfArrival(t *testing.T) {
	for _, tc := range []struct {
		stance    Activity
		surprised bool
		why       string
	}{
		{ActivityForage, true, "caught-foraging"},
		{ActivityLabour, true, "caught-labouring"},
		{ActivityIdle, false, ""},
		{ActivityWatch, false, ""},
	} {
		t.Run(string(tc.stance), func(t *testing.T) {
			f := newResolverFight(t, 1462)
			f.fitness.activity = tc.stance
			f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
			f.open(t)

			state := f.c.HarnessState()
			require.Equal(t, tc.surprised, state["surprised"])
			require.Equal(t, tc.why, state["surprise_why"])
			require.Equal(t, "enemy", state["initiator"], "v0 has exactly one initiator")

			if tc.surprised {
				require.Equal(t, "enemy", state["first_side"])
				require.Equal(t, []string{"w:1", "p:1"}, f.c.Order(), "caught head-down, the player goes last")

				// Round two puts him back in front, so the branch is round
				// one's alone.
				f.round()
				require.Equal(t, "player", f.c.HarnessState()["first_side"])
				require.Equal(t, []string{"p:1", "w:1"}, f.c.Order())
			} else {
				require.Equal(t, "player", state["first_side"])
				require.Equal(t, []string{"p:1", "w:1"}, f.c.Order())
			}
		})
	}
}

// AN ENEMY AT ZERO IS DEAD: it leaves the order, keeps its participant row,
// and the fight ends when it was the last of them.
func TestResolverEnemyAtZeroLeavesTheOrderAndKeepsItsRow(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "w:1", 20, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})

	second := f.add(t, "w:2", 5000, Profile{Group: "g:2", Row: "boar", Speed: 1, DamageMin: 8, DamageMax: 14})
	second.x, second.y = 39, 40

	f.open(t)
	f.set(t, "forced_band", BandCrit)

	for i := 0; i < 6 && f.health("w:1") > 0; i++ {
		f.round()
	}

	require.Equal(t, 0, f.health("w:1"), "a 20-HP wolf under crits must die inside six rounds")
	require.True(t, f.c.Fighting(), "the boar is still standing, so the fight goes on")
	require.NotContains(t, f.c.Order(), "w:1", "a corpse takes no activation")

	// It KEEPS its row. The rows are built from `enemies`, so it cannot both
	// leave that slice and stay reported -- the first draft of the brief
	// asked for both.
	parts, ok := f.c.HarnessState()["participants"].([]map[string]interface{})
	require.True(t, ok)

	var found map[string]interface{}

	for _, row := range parts {
		if row["id"] == "w:1" {
			found = row
		}
	}

	require.NotNil(t, found, "the dead wolf keeps its participant row")
	require.Equal(t, true, found["dead"])
	require.Equal(t, 0, found["health"])
}

// AND WHEN THE LAST ONE FALLS, THE FIGHT ENDS AND SAYS SO.
func TestResolverEndsWhenEveryEnemyIsDead(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "w:1", 20, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
	f.open(t)
	f.set(t, "forced_band", BandCrit)

	for i := 0; i < 6 && f.c.Fighting(); i++ {
		f.round()
	}

	require.False(t, f.c.Fighting())

	state := f.c.HarnessState()
	require.Equal(t, "enemies_dead", state["ended_reason"])
	require.Equal(t, 1, state["ended_enemies_dead"])
	require.Equal(t, 0, state["ended_player_dead"])
}

// YOU CAN LOSE. This is the milestone's sentence, at the model level.
func TestResolverPlayerAtZeroEndsTheEncounter(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.bodies.known["p:1"] = &fakeBody{health: 12, maxHealth: 240}
	f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
	f.open(t)

	f.set(t, "player_action", PlayerActionHold)
	f.set(t, "forced_band", BandCrit)

	for i := 0; i < 6 && f.c.Fighting(); i++ {
		f.round()
	}

	require.False(t, f.c.Fighting(), "a dead player is not in a fight")
	require.Equal(t, 0, f.health("p:1"))

	state := f.c.HarnessState()
	require.Equal(t, "player_dead", state["ended_reason"])
	require.Equal(t, 1, state["ended_player_dead"])
}

// THE DEAD MUST NOT RESTART THE FIGHT.
//
// tryStart runs on every tick the encounter is nil and takes every noticed
// watcher, and step 4 clears neither the notice model nor the chase when
// something dies. So a corpse is still noticed and still in reach, and without
// the filter in tryStart the very next tick opens a fresh encounter against
// it. THIS IS THE TEST THAT WOULD HAVE CAUGHT THE FIRST DRAFT OF THE BRIEF:
// its negative control is to delete that filter and watch these counters
// climb once a round for the rest of the night.
func TestResolverTheDeadDoNotRestartTheFight(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "w:1", 20, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
	f.open(t)
	f.set(t, "forced_band", BandCrit)

	for i := 0; i < 6 && f.c.Fighting(); i++ {
		f.round()
	}

	require.False(t, f.c.Fighting())

	// The corpse is still aware and still one tile away: nothing cleared it.
	noticed, watching := f.notice.Noticed("w:1")
	require.True(t, watching && noticed, "the premise -- the corpse is still a live watcher")

	before := f.c.HarnessState()

	for i := 0; i < 10; i++ {
		f.c.Advance(1.0)
	}

	after := f.c.HarnessState()

	require.Equal(t, before["encounters"], after["encounters"], "no new fight against the corpse")
	require.Equal(t, before["ended_enemies_dead"], after["ended_enemies_dead"])
	require.Equal(t, before["rounds"], after["rounds"], "and no rounds resolved against it")
	require.Equal(t, false, after["fighting"])
}

// The other half of the same rule: nothing starts a fight with a dead quarry.
func TestResolverNoFightStartsAgainstADeadPlayer(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.bodies.known["p:1"] = &fakeBody{health: 0, maxHealth: 240}
	f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})

	for i := 0; i < 10; i++ {
		f.c.Advance(1.0)
	}

	require.False(t, f.c.Fighting(), "a pack does not beat a corpse forever")
	require.Equal(t, 0, f.c.HarnessState()["encounters"])
}

// R2 §3 bullet 12: two launches of one build at one seed fight one fight.
func TestResolverIsDeterministic(t *testing.T) {
	run := func() []map[string]interface{} {
		f := newResolverFight(t, 1462)
		f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})

		second := f.add(t, "w:2", 5000, Profile{Group: "g:2", Row: "dogs", Speed: 3, DamageMin: 3, DamageMax: 6})
		second.x, second.y = 39, 40

		f.open(t)

		log := []map[string]interface{}{}

		for i := 0; i < 10; i++ {
			f.round()
			log = append(log, f.actions(t)...)
		}

		return log
	}

	first, second := run(), run()

	require.NotEmpty(t, first)
	require.Equal(t, first, second, "one seed, one fight -- every roll, band and damage")
}

// THE FENCE: the model still works with no tables and no sprites. Both are
// reported, so a playtest can tell missing wiring from a rule that did not
// fire -- the has_notice precedent.
func TestResolverCopesWithNoProfilesAndNoAnimator(t *testing.T) {
	notice := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	target := &fakeQuarry{id: "p:1", x: 40, y: 40}
	bodies := &fakeBodies{known: map[string]*fakeBody{
		"p:1": {health: 240, maxHealth: 240},
		"w:1": {health: 61, maxHealth: 61},
	}}

	c := NewCombat(NewClock(DefaultClockDials()), notice, &fakeFitness{reaction: true},
		&fakeIllumination{}, bodies, nil, nil, 1462, DefaultCombatDials())

	defer c.Close()

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40}
	aware(t, notice, wolf, target)

	c.Advance(1.0)
	c.Advance(1.0)

	state := c.HarnessState()
	require.Equal(t, false, state["has_profiles"])
	require.Equal(t, false, state["has_animator"])
	require.True(t, c.Fighting(), "no panic, and a fight still happens")

	parts, ok := state["participants"].([]map[string]interface{})
	require.True(t, ok)

	// Everything the tables did not place fights on the placeholder, and says
	// so, so a script can never mistake it for a table monster.
	require.Equal(t, "placeholder", parts[1]["profile"])
	require.Less(t, bodies.known["w:1"].health, 61, "the placeholder still bites")
}

// The three acts, in the order a fight produces them.
//
// The wolf carries 120 HP deliberately: at 20 it died to the player's very
// first forced crit (12-20 base times 1.5) and never swung at all, so the
// swing assertion below measured nothing. The player goes first every
// un-surprised round, which makes "the enemy also swung" a fact you have to
// keep something alive to see.
func TestResolverAsksForTheRightAnimations(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "w:1", 120, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
	f.open(t)
	f.set(t, "forced_band", BandCrit)

	f.round()
	require.Contains(t, f.animator.acts, "p:1:swing", "the player swung")
	require.Contains(t, f.animator.acts, "w:1:swing", "and so did the wolf")

	for i := 0; i < 6 && f.c.Fighting(); i++ {
		f.round()
	}

	require.Contains(t, f.animator.acts, "w:1:die", "the wolf died on screen")
	require.NotContains(t, f.animator.acts, "p:1:die", "the player's death is M4.6's, not step 4's")
}

// A LIVING ENEMY WALKING AWAY IS DISENGAGED, NOT "EVERYTHING IS DEAD".
//
// Found by the step-4 review. pruneOrEnd used to drop out-of-reach enemies
// FIRST and then ask whether everything left was dead -- so killing one wolf
// and letting the other lose its nerve left only a corpse to look at, and a
// fight the player survived was reported enemies_dead. ended_enemies_dead is
// what the thirteenth playtest asserts on, so the count lied too.
func TestResolverALivingEnemyLeavingIsDisengagement(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "w:1", 20, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})

	runner := f.add(t, "w:2", 5000, Profile{Group: "g:2", Row: "boar", Speed: 1, DamageMin: 8, DamageMax: 14})
	runner.x, runner.y = 39, 40

	f.open(t)
	f.set(t, "forced_band", BandCrit)

	for i := 0; i < 6 && f.health("w:1") > 0; i++ {
		f.round()
	}

	require.Equal(t, 0, f.health("w:1"), "the 20-HP wolf must die first")
	require.True(t, f.c.Fighting(), "the boar is still there")

	// Now the living one walks out of reach.
	runner.x, runner.y = 60, 60

	f.round()

	require.False(t, f.c.Fighting())

	state := f.c.HarnessState()
	require.Equal(t, "disengaged", state["ended_reason"],
		"one corpse and one runaway is a fight that was WALKED OUT OF, not one that was won")
	require.Equal(t, 1, state["ended_disengaged"])
	require.Equal(t, 0, state["ended_enemies_dead"])
}

// A NEW FIGHT STARTS WITH AN EMPTY ACTION LOG.
//
// Also the review's. The log is cleared when a round RESOLVES, and a fresh
// encounter has resolved none -- so without a clear in tryStart the provider
// reported the PREVIOUS fight's blows against this one, and actions_round
// could not tell them apart because both read 1.
func TestResolverANewFightStartsWithNoActions(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "w:1", 20, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})
	f.open(t)
	f.set(t, "forced_band", BandCrit)

	for i := 0; i < 6 && f.c.Fighting(); i++ {
		f.round()
	}

	require.False(t, f.c.Fighting())
	require.NotEmpty(t, f.actions(t), "the fight that just ended left its blows on the board, as it should")

	// A second, live enemy opens a second encounter.
	next := f.add(t, "w:2", 5000, Profile{Group: "g:2", Row: "boar", Speed: 1, DamageMin: 8, DamageMax: 14})
	next.x, next.y = 39, 40

	f.open(t)

	require.Empty(t, f.actions(t), "a fight that has resolved nothing has struck no blows")
	require.Equal(t, 0, f.c.HarnessState()["actions_round"])
}

// THE ROUND DIAL HAS A FLOOR, and the loop that spends it is why.
//
// Advance subtracts RoundMinutes from an accumulator until it runs out. Below
// the accumulator's precision the subtraction is a no-op and the loop cannot
// terminate: the review hung a test at 1e-17 after twenty-three million
// rounds. Well above that it is still nonsense -- 1e-4 resolves ten thousand
// rounds inside one stepped world minute -- so the setter refuses both.
func TestResolverRoundMinutesIsFloored(t *testing.T) {
	f := newResolverFight(t, 1462)

	require.Error(t, f.c.HarnessSet("round_minutes", 1e-17), "a round cannot cost nothing")
	require.Error(t, f.c.HarnessSet("round_minutes", 0.0))
	require.Error(t, f.c.HarnessSet("round_minutes", -1.0))
	require.NoError(t, f.c.HarnessSet("round_minutes", minRoundMinutes), "the floor itself is allowed")
	require.NoError(t, f.c.HarnessSet("round_minutes", 2.0))
	require.Equal(t, 2.0, f.c.HarnessState()["round_minutes"])
}
