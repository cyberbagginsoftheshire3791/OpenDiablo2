package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zombie is a strong striker with no group, so no nerve and no rout: the
// TestKit shape (BUG-112) -- a fight that only a death or a removal ends.
func zombie() Profile {
	return Profile{Row: "zombies", Speed: 3, DamageMin: 4, DamageMax: 8, Count: 1}
}

// TestARemovedEnemyLeavesHisFight is BUG-112: strigoi_remove_entity took a
// zombie off the map and the fight ran on against it, rounds 33 to 67, until
// he died. The removed monster's Combatant still stands where it stood (its
// position still reads after the map lets it go), so only Combat.Remove can
// take it out.
func TestARemovedEnemyLeavesHisFight(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "z:1", 5000, zombie())
	f.chases.chasing["z:1"] = true
	f.open(t)

	// The fight is real: a few rounds, and the zombie strikes him.
	for i := 0; i < 5; i++ {
		f.round()
	}

	require.True(t, f.c.Fighting())
	require.Less(t, f.health("p:1"), 240, "the zombie struck him before it was removed")

	left := f.c.Remove("z:1")

	assert.Equal(t, 1, left, "it left one fight, his")
	assert.False(t, f.c.Fighting(), "a fight with nothing left in it is over")
	assert.Equal(t, "disengaged", f.c.EndedReason(), "nobody won: it was taken away, not killed and not routed")
	assert.Equal(t, 1, f.c.HarnessState()["ended_disengaged"])
	assert.Equal(t, 0, f.c.HarnessState()["ended_enemies_dead"], "no kill is claimed")

	_, watching := f.notice.Noticed("z:1")
	assert.False(t, watching, "nothing watches for it any more (withdraw's Unwatch)")
	assert.Contains(t, f.chases.released, "z:1", "its chase was released (withdraw's Release)")

	// And the fight does not come back: forty rounds of world time, and not
	// one blow lands on him from a monster that is not there.
	hp := f.health("p:1")

	for i := 0; i < 40; i++ {
		f.round()
	}

	assert.False(t, f.c.Fighting(), "no fight opened again against the removed zombie")
	assert.Equal(t, hp, f.health("p:1"), "no blow from a removed monster")
	assert.Zero(t, f.c.Remove("z:1"), "a second removal leaves nothing")
}

// TestARemovedEnemyLeavesTheRestFighting: one of two taken away, the other
// fights on, and the removed one neither strikes nor stands in the order.
func TestARemovedEnemyLeavesTheRestFighting(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "z:1", 5000, zombie())
	other := f.add(t, "z:2", 5000, zombie())
	other.x, other.y = 39, 40
	f.open(t)

	f.round()
	require.ElementsMatch(t, []string{"p:1", "z:1", "z:2"}, f.c.Order())

	assert.Equal(t, 1, f.c.Remove("z:1"))
	assert.True(t, f.c.Fighting(), "the other zombie fights on")
	assert.Equal(t, []string{"p:1", "z:2"}, f.c.Order(), "the removed one is out of the order")

	for i := 0; i < 10; i++ {
		f.round()

		for _, row := range f.actions(t) {
			assert.NotEqual(t, "z:1", row["attacker"], "round %d: a removed monster struck", i)
			assert.NotEqual(t, "z:1", row["target"], "round %d: he struck a removed monster", i)
		}
	}

	for _, p := range f.c.HarnessState()["participants"].([]map[string]interface{}) {
		assert.NotEqual(t, "z:1", p["id"], "the removed one has no participant row")
	}
}

// TestARemovedEnemyLeavesAClockFight: the village's fights too -- an enemy
// taken away leaves the clock fight it is in, and a quarry taken away ends
// its clock fight and lets go every watch on it, as its death would.
func TestARemovedEnemyLeavesAClockFight(t *testing.T) {
	f := humanFight(t, 1462, nil)
	f.c.SetPlayer("p:1")

	clockQuarry(t, f, "v:1", 80, 40, 5000, "m:1", 5000)
	f.profiles.byID["m:1"] = zombie()

	f.c.Advance(0.5)
	require.NotNil(t, clockFightFor(f, "v:1"), "the village's fight opened")

	assert.Equal(t, 1, f.c.Remove("m:1"), "it left the clock fight")
	assert.Nil(t, clockFightFor(f, "v:1"), "the clock fight with nothing left in it is over")
	assert.Equal(t, "disengaged", clockBlock(f)["ended_reason"])
	assert.Equal(t, "", f.c.EndedReason(), "his book did not move")

	f.c.Advance(5)
	assert.Nil(t, clockFightFor(f, "v:1"), "and it does not open again")

	// The quarry taken away.
	clockQuarry(t, f, "v:2", 80, 60, 5000, "m:2", 5000)
	f.profiles.byID["m:2"] = zombie()

	f.c.Advance(0.5)
	require.NotNil(t, clockFightFor(f, "v:2"))

	assert.Equal(t, 1, f.c.Remove("v:2"))
	assert.Nil(t, clockFightFor(f, "v:2"), "a fight over a quarry taken away is over")

	_, watching := f.notice.Noticed("m:2")
	assert.False(t, watching, "every watch on the quarry taken away is let go, as at its death")
}

// The review's C3 (its probe TestReviewRemoveTheLastLivingWithTheDeadLeft):
// he kills one zombie, the other is taken off the map. The fight is over and
// it ends as it would have had the other died -- enemies_dead, his kill
// counted -- not "disengaged": the dead one's row is still in it.
func TestARemovalLeavingOnlyTheDeadEndsEnemiesDead(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "z:1", 1, zombie())
	other := f.add(t, "z:2", 5000, zombie())
	other.x, other.y = 39, 40
	f.open(t)

	for i := 0; i < 30 && f.health("z:1") > 0; i++ {
		f.round()
	}

	require.LessOrEqual(t, f.health("z:1"), 0, "z:1 died")
	require.True(t, f.c.Fighting(), "the control: z:2 still fights")

	assert.Equal(t, 1, f.c.Remove("z:2"))
	assert.False(t, f.c.Fighting(), "nothing left alive in it: over")
	assert.Equal(t, "enemies_dead", f.c.EndedReason(), "the dead one's row is still in it: it ends as pruneOrEnd ends it")
	assert.Equal(t, 1, f.c.HarnessState()["ended_enemies_dead"])
	assert.Equal(t, 0, f.c.HarnessState()["ended_disengaged"])
}

// The review's C4 (its mutant R5): a Downed man who may stand again keeps the
// fight going when the last living enemy is taken away -- the fight goes on
// until he stands or is staked, as pruneOrEnd keeps it (stillIn).
func TestARemovalLeavesTheDownedInTheFight(t *testing.T) {
	corpses := NewCorpses(nil, nil, nil)
	corpses.FallHuman("body", "", 41, 40)
	require.True(t, corpses.Rise("body"))
	corpses.Raised("body", "d:1")

	f := newResolverFight(t, 1)
	f.c.SetCorpses(corpses)
	f.add(t, "d:1", 1, deadProfile("g:1"))
	other := f.add(t, "z:2", 5000, zombie())
	other.x, other.y = 39, 40
	f.open(t)

	for i := 0; i < 30 && !f.c.encounter.dead["d:1"]; i++ {
		f.round()
	}

	require.True(t, f.c.encounter.dead["d:1"], "d:1 was cut down")
	require.True(t, corpses.DownedMember("d:1"), "and lies Downed: he may stand again")
	require.True(t, f.c.Fighting())

	assert.Equal(t, 1, f.c.Remove("z:2"))
	assert.True(t, f.c.Fighting(), "a Downed man who may stand again keeps the fight")

	// The control: staked, he keeps nothing, and the next round ends it.
	require.True(t, corpses.Close("body"))
	f.round()
	assert.False(t, f.c.Fighting(), "staked, nothing keeps the fight")
	assert.Equal(t, "enemies_dead", f.c.EndedReason())
}

// The reviewer's mutant R7: a dead enemy taken off the map leaves no dead
// mark behind -- its row, its place in the order and its dead mark all go
// (a mark with no row would be saved and counted as a body that is not
// there).
func TestARemovedDeadEnemyLeavesNoMark(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.add(t, "z:1", 1, zombie())
	other := f.add(t, "z:2", 5000, zombie())
	other.x, other.y = 39, 40
	f.open(t)

	for i := 0; i < 30 && !f.c.encounter.dead["z:1"]; i++ {
		f.round()
	}

	require.True(t, f.c.encounter.dead["z:1"], "the control: z:1 is marked dead")

	assert.Equal(t, 1, f.c.Remove("z:1"))
	assert.True(t, f.c.Fighting(), "z:2 fights on")
	assert.False(t, f.c.encounter.dead["z:1"], "the removed one's dead mark went with its row")
	assert.NotContains(t, f.c.encounter.enemyOrder, "z:1")
}
