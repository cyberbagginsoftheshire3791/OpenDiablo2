package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BUG-11, and these are the tests whose absence let it ship twice.
//
// The group cap exists so a forced-chance script cannot fill the map, and it
// was implemented as len(groups) -- every group the tables have ever made and
// not yet sent home, whether or not it can still do anything. Nothing retires a
// pack the player BEAT: Despawn has two callers, the harness and
// clearAtDaybreak, and daybreak is hours away and skips any group that has
// noticed the player. So a beaten pack held a slot until sunrise.
//
// In August that produced a PERMANENT spawn stall at eight groups and
// clearAtDaybreak was written for it. On 19 September 2026 the cap moved 8 -> 2
// on a measured dial pass and the stall came straight back, this time bounded
// by the night rather than the life of the screen -- two packs killed at dusk
// and the tables never fired again. TestCombatRout is what caught it, by
// reporting two groups at `morale:0 routing:true notice:[]` after forty
// five-minute steps produced no arrival at all.
//
// WHAT THE OLD TESTS ASSERTED, AND WHY IT WAS NOT ENOUGH. Every half was
// covered. TestSpawnsRollEveryEligibleRowEvenAtTheCap pins that the cap holds
// and that the rolls still happen. TestSpawnsDespawnUnwatchesItsMembers pins
// that a despawn stops the watching. The resolver's tests pin that a death and
// a rout both withdraw their members. Nothing anywhere asked what the cap does
// with a group the resolver has finished with -- the composition, again, which
// is the shape of the whole 19 September audit.

// TestSpawnsTheCapCountsLiveGroupsNotSpentOnes is the unit half, and it is
// two-sided on purpose: the first arm is the CONTROL, and without it this test
// would pass against a build with no cap at all.
func TestSpawnsTheCapCountsLiveGroupsNotSpentOnes(t *testing.T) {
	s, clock, notice, _, _ := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))

	s.dials.MaxGroups = 2

	for i := 0; i < 4; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	require.Equal(t, 2, s.Groups(), "the cap holds while every group is live")
	require.Equal(t, 2, s.HarnessState()["live_groups"].(int))
	require.Zero(t, s.HarnessState()["spent_groups"].(int))

	// THE CONTROL. Four more checks at a certainty, and nothing arrives,
	// because nothing has happened to either pack.
	for i := 0; i < 4; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	require.Equal(t, 2, s.Groups(),
		"a full cap of live groups is a full cap -- if this fails the cap is gone, not fixed")

	// Now beat one of them, the way the game beats one: Combat.withdraw drops
	// every member of a pack that died or broke out of the notice model. This
	// calls the same verb withdraw calls, one member at a time.
	first := s.HarnessState()["group_list"].([]map[string]interface{})[0]["group"].(string)

	for _, m := range s.groups[first].members {
		require.True(t, notice.Unwatch(m.WatcherID()),
			"every member starts out watched, or the premise of this test is wrong")
	}

	assert.Equal(t, 1, s.HarnessState()["live_groups"].(int), "one pack is over")
	assert.Equal(t, 1, s.HarnessState()["spent_groups"].(int))
	assert.Equal(t, 2, s.Groups(), "and it is still on the books -- its corpses are still out there")

	s.Advance(s.dials.CheckMinutes)

	assert.Equal(t, 3, s.Groups(), "the freed slot let the night carry on")
	assert.Equal(t, 2, s.HarnessState()["live_groups"].(int),
		"and the cap still binds: two live, not three")
}

// TestSpawnsTheCapStillBindsWithNoNoticeModel is the nil arm. liveGroups cannot
// answer the question without a notice model, and the answer it gives then is
// the pre-19-September one -- every group counts, forever. Untagged tests that
// pass nil notice depend on that, and a build that treated "cannot tell" as
// "spent" would have no cap at all in those tests.
func TestSpawnsTheCapStillBindsWithNoNoticeModel(t *testing.T) {
	clock := NewClock(DefaultClockDials())
	spawner := &fakeSpawner{}

	s := NewSpawns(clock, nil, spawner, nil, &fakeIllumination{}, 1462, DefaultSpawnDials())
	defer s.Close()

	s.SetTarget(&fakeQuarry{id: "p:1", x: 40, y: 40})
	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))

	s.dials.MaxGroups = 1

	for i := 0; i < 6; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	assert.Equal(t, 1, s.Groups(), "with nothing watching, nothing can be shown to be spent")
	assert.Equal(t, 1, s.HarnessState()["live_groups"].(int))
}

// TestSpawnsAPackKilledThroughTheResolverFreesItsSlot is the COMPOSITION test,
// and it is the one that would have caught this.
//
// It wires a real Spawns to a real Combat through the two interfaces the game
// screen wires them with -- Profiles and Morale are both *Spawns -- over one
// real Notice, and then does nothing by hand except swing: the tables place the
// pack, the notice model makes it aware, the resolver kills it, the resolver
// withdraws it, and the tables are asked for another one. No test in this
// package had ever joined those two systems, which is exactly how a cap that
// counted corpses survived two milestones.
func TestSpawnsAPackKilledThroughTheResolverFreesItsSlot(t *testing.T) {
	clock := NewClock(DefaultClockDials())
	notice := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	spawner := &fakeSpawner{}
	chases := &fakeChases{chasing: map[string]bool{}}
	target := &fakeQuarry{id: "p:1", x: 40, y: 40}

	dials := DefaultSpawnDials()
	dials.MaxGroups = 1 // one pack at a time, so "another one arrived" is unambiguous

	spawns := NewSpawns(clock, notice, spawner, chases, &fakeIllumination{}, 1462, dials)
	defer spawns.Close()

	spawns.SetTarget(target)

	bodies := &fakeBodies{known: map[string]*fakeBody{"p:1": {health: 240, maxHealth: 240}}}

	combat := NewCombat(clock, notice, &fakeFitness{reaction: true}, &fakeIllumination{},
		bodies, spawns, nil, spawns, chases, 1462, DefaultCombatDials())
	defer combat.Close()

	// Combat learns who the player is from the notice model's aware pairs --
	// the Quarry each watch names -- so nothing here tells it. The tables set
	// the same target on the spawns system, and that is the whole join.
	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, spawns.HarnessSet("chance", 100.0))

	spawns.Advance(spawns.dials.CheckMinutes)

	require.Equal(t, 1, spawns.Groups(), "the tables must place a pack for this to be about anything")

	groupID := spawns.HarnessState()["group_list"].([]map[string]interface{})[0]["group"].(string)
	members := spawns.groups[groupID].members
	require.NotEmpty(t, members)

	// A body each, one hit from death, and a chase each -- which is what the
	// game screen gives them. Nothing else is arranged.
	for _, m := range members {
		bodies.known[m.WatcherID()] = &fakeBody{health: 1, maxHealth: 1}
		chases.chasing[m.WatcherID()] = true
	}

	// The fakeSpawner puts a pack MinTiles away, so reach has to cover it. It
	// is a dial, and widening it is how a unit test gets a pack into reach
	// without moving anything by hand.
	require.NoError(t, combat.HarnessSet("adjacent_tiles", 32.0))
	require.NoError(t, combat.HarnessSet("forced_band", "crit"))
	require.NoError(t, combat.HarnessSet("player_action", "attack"))

	for i := 0; i < 40 && spawns.HarnessState()["live_groups"].(int) > 0; i++ {
		combat.Advance(1.0)
	}

	state := spawns.HarnessState()
	require.Zero(t, state["live_groups"].(int),
		"the resolver must finish the pack off for this test to be testing anything: %v", state)
	require.Equal(t, 1, state["spent_groups"].(int))
	require.Equal(t, 1, spawns.Groups(), "and it is still on the books until daybreak")

	// THE ASSERTION. One check, and the night carries on.
	spawns.Advance(spawns.dials.CheckMinutes)

	assert.Equal(t, 2, spawns.Groups(), "a beaten pack does not hold the cap")
	assert.Equal(t, 1, spawns.HarnessState()["live_groups"].(int),
		"and the replacement is the only live one, so the cap still means what it says")
}

// TestSpawnsMaxGroupsIsWritable is the third provider rule, paid late.
// max_groups was reported from the day it existed and no verb moved it, so the
// 19 September dial sweep had to edit the source between runs -- four rebuilds
// for one question, and a sweep that cannot be re-run from a script is a sweep
// nobody re-runs.
func TestSpawnsMaxGroupsIsWritable(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	assert.Contains(t, s.HarnessSettableFields(), "max_groups")

	require.Error(t, s.HarnessSet("max_groups", "two"), "it wants a number")
	require.Error(t, s.HarnessSet("max_groups", 0.0), "a cap of nothing is a stalled world, not a dial")

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))
	require.NoError(t, s.HarnessSet("max_groups", 1.0))

	for i := 0; i < 4; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	require.Equal(t, 1, s.Groups(), "the written cap binds")
	assert.Equal(t, 1, s.HarnessState()["max_groups"].(int), "and the provider reports what was written")

	require.NoError(t, s.HarnessSet("max_groups", 3.0))

	for i := 0; i < 4; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	assert.Equal(t, 3, s.Groups(), "raising it lets the night carry on -- the write moves the cap in both directions")
}
