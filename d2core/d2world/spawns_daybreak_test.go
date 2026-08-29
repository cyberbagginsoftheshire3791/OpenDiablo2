package d2world

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The regression guards for the SPAWN STALL, found by the reachability
// register on 29 Aug 2026 and ruled a burst of its own by Josh.
//
// M4.3b shipped with a one-way door. check() refuses to spawn once
// len(groups) reaches MaxGroups (8), and NOTHING in a shipped build ever
// called Despawn -- its only caller was HarnessSet, reachable only from
// harness-tagged code. So the eighth pack was the last one the game would
// ever produce. Not a leak: a permanent stall, with the night ceasing to
// happen for the life of the screen.
//
// The first test below fails on the pre-fix build at its third assertion,
// which is the point of writing it this way round: it demonstrates the stall
// before it demonstrates the cure.

func TestSpawnsClearAtDaybreakEndsTheStall(t *testing.T) {
	s, clock, _, spawner, target := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)

	// Force every eligible row, the way a script does, so the cap is reached
	// in a test rather than in three real nights.
	s.dials.Chance = 1000

	for i := 0; i < 60 && s.Groups() < s.dials.MaxGroups; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	require.Equal(t, s.dials.MaxGroups, s.Groups(), "forced chance should reach the group cap")

	// THE STALL ITSELF. At the cap, more night buys nothing at all: every
	// later roll hits the `continue` at the MaxGroups guard.
	atCap := spawner.made

	for i := 0; i < 10; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	require.Equal(t, atCap, spawner.made, "at the cap nothing new can arrive -- this is the stall")

	// Walk the quarry out of sight and wait out the memory window, so nothing
	// is still aware of it. The aware branch is deliberately left alone by
	// daybreak and has its own test below; this one is about the stall.
	//
	// This step is not scaffolding -- the first run of this test found five
	// of the eight packs still aware, because fakeSpawner places members
	// eight tiles out and the notice radius is twelve. The aware branch was
	// working; the test's premise was wrong.
	target.x, target.y = target.x+500, target.y+500

	for i := 0; i < 40; i++ {
		s.Advance(1.0)
	}

	// Daylight sends the unaware home, and takes them off the map. The second
	// half matters as much as the first: before this burst Despawn dropped
	// the bookkeeping and left the creatures standing there.
	advanceClockToStage(t, clock, StageDay)
	s.Advance(0.1)

	require.Zero(t, s.Groups(), "daylight should send every unaware group home")
	require.Equal(t, s.dials.MaxGroups, s.cleared, "every cleared group should be counted")
	require.Equal(t, spawner.made, spawner.removed,
		"every member the spawner made must be taken back out of the world, not merely forgotten")
	require.Equal(t, spawner.made, s.despawned)

	// And now the assertion the stall failed: a later night can still spawn.
	advanceClockToStage(t, clock, StageNight)

	for i := 0; i < 60 && s.Groups() == 0; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	require.NotZero(t, s.Groups(), "a later night must be able to spawn again")
	require.Greater(t, spawner.made, atCap, "the tables must have produced something new")
}

// A pack that has noticed you is not sent home by the sunrise. Daylight is a
// reason to go home, not a reason to forget the thing already coming for you.
func TestSpawnsDaybreakLeavesAnAwareGroupAlone(t *testing.T) {
	s, clock, _, _, target := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	s.dials.Chance = 1000

	for i := 0; i < 60 && s.Groups() == 0; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	require.NotZero(t, s.Groups(), "the test needs at least one group to work with")

	// Walk the quarry onto the pack so the sight test cannot fail to notice
	// it: fakeSpawner places members at aroundX+minTiles on the x axis.
	id := s.groupIDs()[0]
	g := s.groups[id]
	require.NotEmpty(t, g.members)

	mx, my := g.members[0].WatcherAt()
	target.x, target.y = mx, my

	// Long enough for the re-evaluation window to come round.
	for i := 0; i < 20 && !s.aware(id); i++ {
		s.Advance(1.0)
	}

	require.True(t, s.aware(id), "a watcher standing on the quarry with a clear line should notice it")

	before := s.Groups()

	advanceClockToStage(t, clock, StageDay)
	s.Advance(0.1)

	require.True(t, s.aware(id), "the aware group should still be watching")
	require.Contains(t, s.groupIDs(), id, "daylight must not send home a pack that has noticed the player")
	require.LessOrEqual(t, s.Groups(), before)
}
