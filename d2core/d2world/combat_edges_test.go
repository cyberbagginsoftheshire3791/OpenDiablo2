package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T3, his talents in the fight. Each edge is proved against the same fight
// with no edge -- the zero value is neutral, and that is half of every test.

type fakeEdges map[string]Edge

func (f fakeEdges) EdgeOf(id string) Edge { return f[id] }

func withEdge(f *resolverFight, e Edge) { f.c.SetEdges(fakeEdges{"p:1": e}) }

func rowsBy(t *testing.T, f *resolverFight, attacker string) []map[string]interface{} {
	t.Helper()

	var out []map[string]interface{}

	for _, row := range f.actions(t) {
		if row["attacker"] == attacker {
			out = append(out, row)
		}
	}

	return out
}

func TestEdgeRiposteDrillHitsHarder(t *testing.T) {
	riposte := func(e Edge) map[string]interface{} {
		f := newResolverFight(t, 1462)
		f.set(t, "forced_band", BandGraze)
		f.set(t, "player_action", PlayerActionHold)
		withEdge(f, e)
		f.add(t, "e:1", 5000, Profile{})
		f.open(t)
		f.round()

		for _, row := range rowsBy(t, f, "p:1") {
			if row["reaction"] == "riposte" {
				return row
			}
		}

		t.Fatal("no riposte")

		return nil
	}

	plain := riposte(Edge{})
	drilled := riposte(Edge{RiposteDamage: 1.5})

	require.Equal(t, plain["base"], drilled["base"], "same seed, same draw")
	assert.Equal(t, int(float64(plain["damage"].(int))*1.5), drilled["damage"], "half again as much")
}

func TestEdgeShieldWallTurnsTwo(t *testing.T) {
	blocked := func(e Edge) int {
		f := newResolverFight(t, 1462)
		f.set(t, "forced_band", BandCrit)
		f.set(t, "player_action", PlayerActionHold)
		f.c.SetKits(fakeKits{"p:1": shippedKit(t, "sword-and-board")})
		withEdge(f, e)

		pack := Profile{Group: "g:1", Row: "wolves", Speed: 5, DamageMin: 6, DamageMax: 12, DamageClass: "thrust"}
		f.add(t, "e:1", 400, pack)
		f.add(t, "e:2", 400, pack).y = 41
		f.add(t, "e:3", 400, pack).y = 39
		f.open(t)
		f.round()

		n := 0

		for _, row := range blowsOn(t, f, "p:1") {
			if row["blocked"] == true {
				n++
			}
		}

		return n
	}

	assert.Equal(t, 1, blocked(Edge{}), "one a round, plain")
	assert.Equal(t, 2, blocked(Edge{ExtraBlocks: 1}), "two with Shield Wall")
}

func TestEdgeKillingStrokeWidensHisCrit(t *testing.T) {
	bands := func(e Edge) map[string]int {
		f := newResolverFight(t, 1462)
		withEdge(f, e)
		f.add(t, "e:1", 100000, Profile{})
		f.open(t)

		out := map[string]int{}

		for i := 0; i < 30; i++ {
			f.round()

			for _, row := range rowsBy(t, f, "p:1") {
				out[row["band"].(string)]++
			}
		}

		return out
	}

	// A WIDER CRIT COMES OUT OF THE HIT BAND, never the graze: with the crit
	// band at 65 (15 + 50) and the graze at 35, nothing is left for a hit.
	wide := bands(Edge{CritBand: 50})
	require.NotZero(t, wide[BandCrit]+wide[BandGraze])
	assert.Zero(t, wide[BandHit], "every blow of his is a graze or a crit")
	assert.NotZero(t, wide[BandGraze], "and the grazes stay grazes")

	plain := bands(Edge{})
	assert.NotZero(t, plain[BandHit], "plain, he hits -- the control")

	// And the enemy's blows are untouched by HIS talent: the same seed with and
	// without the edge gives the enemy IDENTICAL bands, blow for blow (review
	// finding: "fewer than all crit" could not catch a leak).
	enemyBands := func(e Edge) []string {
		f := newResolverFight(t, 1462)
		f.set(t, "player_action", PlayerActionHold)
		withEdge(f, e)
		f.add(t, "e:1", 100000, Profile{})
		f.open(t)

		var out []string

		for i := 0; i < 30; i++ {
			f.round()

			for _, row := range rowsBy(t, f, "e:1") {
				out = append(out, row["band"].(string))
			}
		}

		return out
	}

	with, without := enemyBands(Edge{CritBand: 50}), enemyBands(Edge{})
	require.NotEmpty(t, without)
	assert.Equal(t, without, with, "his edge is his")
}

func TestEdgeSecondAnswerRipostesTwice(t *testing.T) {
	ripostes := func(e Edge) int {
		f := newResolverFight(t, 1462)
		f.set(t, "forced_band", BandGraze)
		f.set(t, "player_action", PlayerActionHold)
		withEdge(f, e)

		pack := Profile{Group: "g:1", Row: "wolves", Speed: 5, DamageMin: 1, DamageMax: 1}
		f.add(t, "e:1", 5000, pack)
		f.add(t, "e:2", 5000, pack).y = 41
		f.open(t)
		f.round()

		n := 0

		for _, row := range rowsBy(t, f, "p:1") {
			if row["reaction"] == "riposte" {
				n++
			}
		}

		return n
	}

	assert.Equal(t, 1, ripostes(Edge{}), "one Reaction a round (R2 §3 bullet 6)")
	assert.Equal(t, 2, ripostes(Edge{ExtraReactions: 1}), "two with Second Answer")
}

func TestEdgeNightEyesSharpenTheAdvantage(t *testing.T) {
	mod := func(e Edge) int {
		f := newResolverFight(t, 1462)
		f.fitness.reaction = false
		withEdge(f, e)
		f.add(t, "w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, DamageMin: 6, DamageMax: 12})

		// HE stands in the dark and the wolf in the light: dark-into-light is his.
		f.illum.levels[[2]int{40, 40}] = 0.10
		f.illum.levels[[2]int{41, 40}] = 0.50

		f.open(t)
		f.round()

		rows := rowsBy(t, f, "p:1")
		require.NotEmpty(t, rows)

		return rows[0]["mod"].(int)
	}

	plain := mod(Edge{})
	require.Positive(t, plain, "the setup must give him the advantage")
	assert.Equal(t, plain+10, mod(Edge{AdvantageBonus: 10}))
}

func TestEdgeWhatBreaksThemAndFireAndIron(t *testing.T) {
	// A kill he deals costs the pack more nerve.
	hurtOnKill := func(e Edge) string {
		f := newResolverFight(t, 1462)
		f.set(t, "forced_band", BandCrit)
		withEdge(f, e)
		dogPack(t, f, "g:1", 4, 100)
		f.open(t)
		f.round()

		require.NotEmpty(t, f.morale.hurts)

		return f.morale.hurts[0]
	}

	assert.Equal(t, "g:1:25.0000", hurtOnKill(Edge{}), "a dog of four is a quarter of the pack")
	assert.Equal(t, "g:1:37.5000", hurtOnKill(Edge{KillNerve: 1.5}), "half again as much")

	// A hit that does not kill shakes the pack while his torch burns -- and
	// without the edge, a hit that does not kill costs nothing.
	hurtOnHit := func(e Edge) []string {
		f := newResolverFight(t, 1462)
		f.set(t, "forced_band", BandHit)
		f.set(t, "player_action", PlayerActionAttack)
		withEdge(f, e)
		f.morale.morale["g:1"] = 100
		f.add(t, "g:1.w:1", 5000, Profile{Group: "g:1", Row: "wolves", Speed: 2, Count: 3, DamageMin: 1, DamageMax: 1})
		f.open(t)
		f.round()

		return f.morale.hurts
	}

	assert.Empty(t, hurtOnHit(Edge{}), "no edge: a hit is not a death")
	assert.Equal(t, []string{"g:1:10.0000"}, hurtOnHit(Edge{LitNerve: 10}))
}

func TestXPEventsFollowTheFight(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.set(t, "forced_band", BandCrit)
	dogPack(t, f, "g:1", 2, 50) // two dogs at 50: the first death routs the second
	f.open(t)
	f.round()

	events := f.c.TakeXPEvents()
	require.Len(t, events, 2)
	assert.Equal(t, XPEvent{Kind: "slain", Row: "dogs"}, events[0])
	assert.Equal(t, XPEvent{Kind: "routed", Row: "dogs"}, events[1])
	assert.Empty(t, f.c.TakeXPEvents(), "taken once")
}

func TestEdgeQuickFeetLengthensHisMove(t *testing.T) {
	f, s := pacedFight(t, nil)
	f.add(t, "e:1", 400, Profile{})
	f.c.Advance(1.0)
	untilAwaiting(t, f, s)

	plain := f.c.Tactical().MoveTiles

	withEdge(f, Edge{ExtraMove: 1})
	assert.Equal(t, plain+1, f.c.Tactical().MoveTiles)
}

func TestConditioningSlowsTheMetersAndRaisesTheThresholds(t *testing.T) {
	_, plain := nightMeters(t)
	_, hard := nightMeters(t)
	hard.SetConditioning(Conditioning{FatigueRate: 0.5, FoodWaterRate: 0.5, ShakenFatigue: 10, NoReactionFatigue: 10})

	plain.Advance(tenHours)
	hard.Advance(tenHours)

	assert.InDelta(t, plain.Fatigue()/2, hard.Fatigue(), 1e-9, "half the fatigue")
	assert.InDelta(t, 100-(100-plain.Food())/2, hard.Food(), 1e-9, "half the hunger")
	assert.Equal(t, plain.ShakenThreshold()+10, hard.ShakenThreshold())

	plainFatigue := plain.Fatigue()

	// Unbroken's half: the Reaction holds past the signed fatigue.
	edge := DefaultMeterDials().NoReactionFatigue
	require.NoError(t, plain.HarnessSet("fatigue", edge+5))
	require.NoError(t, hard.HarnessSet("fatigue", edge+5))
	assert.False(t, plain.ReactionAvailable(), "plain: gone at the signed threshold")
	assert.True(t, hard.ReactionAvailable(), "conditioned: still there")

	// THE CONTROL: a zero Conditioning is the signed body.
	_, zero := nightMeters(t)
	zero.SetConditioning(Conditioning{})
	zero.Advance(tenHours)
	assert.Equal(t, plainFatigue, zero.Fatigue())
}

func TestTallowAndPitchSlowsOnlyHisTorch(t *testing.T) {
	l := NewLight(NewClock(DefaultClockDials()), DefaultLightDials())
	t.Cleanup(l.Close)

	carried := l.Add(SourceTorch, true, 0, 0)
	placed := l.Add(SourceTorch, false, 3, 3)

	l.SetCarriedBurnRate(0.75)
	l.Advance(20)

	assert.InDelta(t, 60-15, carried.Burn, 1e-9, "his torch: a quarter slower")
	assert.InDelta(t, 60-20, placed.Burn, 1e-9, "a placed one: as signed")
}
