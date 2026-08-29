package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSpawner records what the tables asked for and hands back watchers. Like
// every other fake in this package it exists so the system under test can be
// exercised with no map, no MPQs and no ebiten.
type fakeSpawner struct {
	calls            int
	fail             bool
	lastCode         string
	lastCount        int
	lastMin, lastMax float64
	made             int
	removed          int
}

func (s *fakeSpawner) Despawn(members []Watcher) {
	s.removed += len(members)
}

func (s *fakeSpawner) Spawn(code string, count int, aroundX, aroundY, minTiles, maxTiles float64) []Watcher {
	s.calls++
	s.lastCode, s.lastCount = code, count
	s.lastMin, s.lastMax = minTiles, maxTiles

	if s.fail {
		return nil
	}

	out := make([]Watcher, 0, count)

	for i := 0; i < count; i++ {
		s.made++
		out = append(out, &fakeWatcher{
			id: code + ":" + itoa(s.made),
			x:  aroundX + minTiles,
			y:  aroundY,
		})
	}

	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var b []byte

	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}

	return string(b)
}

// advanceClockToStage steps the clock until it reaches the wanted stage,
// stepping in simulated seconds because the clock is stepped and never set.
func advanceClockToStage(t *testing.T, c *Clock, want Stage) {
	t.Helper()

	for i := 0; i < 100000; i++ {
		if c.Stage() == want {
			return
		}

		c.Advance(1)
	}

	t.Fatalf("clock never reached stage %v", want)
}

// advanceClockToBand steps the clock until the spawns system reports the band
// wanted.
//
// It asks the system rather than doing minute arithmetic on purpose: the deep
// night CROSSES MIDNIGHT (21:15 to 02:45), so "night start plus two thirds" is
// minute 1496 of a 1440-minute day and a helper that compares minutes-of-day
// directly can never reach it. The first draft of this file did exactly that
// and hung until its own iteration cap. Band() handles the wrap with a modulo;
// this walks the clock and trusts it.
func advanceClockToBand(t *testing.T, c *Clock, s *Spawns, want int) {
	t.Helper()

	for i := 0; i < 200000; i++ {
		if s.Band() == want {
			return
		}

		c.Advance(0.25)
	}

	t.Fatalf("clock never reached deep-night band %d", want)
}

func newTestSpawns(t *testing.T) (*Spawns, *Clock, *Notice, *fakeSpawner, *fakeQuarry) {
	t.Helper()

	clock := NewClock(DefaultClockDials())
	notice := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	spawner := &fakeSpawner{}
	target := &fakeQuarry{id: "p:1", x: 40, y: 40}

	s := NewSpawns(clock, notice, spawner, &fakeIllumination{}, 1462, DefaultSpawnDials())
	s.SetTarget(target)

	t.Cleanup(s.Close)

	return s, clock, notice, spawner, target
}

// The bands are the difficulty curve S1 §4 asks for, and they live here rather
// than on the clock by signature: the clock is the calendar.
func TestSpawnsBandsDivideTheNight(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	require.Equal(t, -1, s.Band(), "dawn is not the deep night")

	advanceClockToStage(t, clock, StageNight)
	assert.Equal(t, 0, s.Band(), "the night opens in band 0")

	advanceClockToBand(t, clock, s, 1)
	assert.Equal(t, 1, s.Band())

	// Band 2 lies PAST MIDNIGHT -- the deep night runs 21:15 to 02:45, so the
	// last third begins at about 00:56 and any arithmetic that does not wrap
	// the day will look for minute 1496 of 1440 and never find it.
	advanceClockToBand(t, clock, s, 2)
	assert.Equal(t, 2, s.Band(), "the last third is the worst one")
	assert.Less(t, clock.MinuteOfDay(), DefaultClockDials().DawnStart,
		"and it is after midnight when we get there")

	advanceClockToStage(t, clock, StageDawn)
	assert.Equal(t, -1, s.Band(), "the band is gone with the night")
}

func TestSpawnsWeightsFollowTheClock(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	rows := map[string]SpawnRow{}
	for _, r := range DefaultSpawnDials().Rows {
		rows[r.Name] = r
	}

	advanceClockToStage(t, clock, StageDay)
	assert.Zero(t, s.Weight(rows["wolves"]), "wolves are a deep-night row")
	assert.Zero(t, s.Weight(rows["dogs"]), "and dogs do not hunt at noon")

	advanceClockToStage(t, clock, StageNight)
	assert.Positive(t, s.Weight(rows["wolves"]), "at night they can come")
	assert.Zero(t, s.Weight(rows["boar"]), "boar are a dawn and dusk row")
}

// The wolves' band curve runs the other way from the dogs': deeper is worse
// for one and better for the other, which is what makes 2 a.m. its own thing.
func TestSpawnsWolvesGrowBolderAsTheNightDeepens(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	var wolves SpawnRow

	for _, r := range DefaultSpawnDials().Rows {
		if r.Name == "wolves" {
			wolves = r
		}
	}

	advanceClockToStage(t, clock, StageNight)
	require.Equal(t, 0, s.Band())

	first := s.Weight(wolves)

	advanceClockToBand(t, clock, s, 2)

	assert.Greater(t, s.Weight(wolves), first,
		"the last band must weigh more than the first")
}

func TestSpawnsCarrionWeightRisesAndCaps(t *testing.T) {
	s, _, _, _, _ := newTestSpawns(t)

	assert.InDelta(t, 1.0, s.CarrionWeight(), 0.001, "no bodies, no pull")

	require.True(t, s.SetOpenBodies(4))
	assert.InDelta(t, 2.0, s.CarrionWeight(), 0.001, "four bodies at +25% each")

	require.True(t, s.SetOpenBodies(100))
	assert.InDelta(t, 3.0, s.CarrionWeight(), 0.001,
		"a field of bodies is dangerous, not arithmetically absurd")

	assert.False(t, s.SetOpenBodies(-1))
	assert.Equal(t, 100, s.OpenBodies(), "a refused write must not land")
}

// The other half of R2 §3's trade: a torch does not only make you easier to
// see, it makes you worth coming to look at.
func TestSpawnsLightWeightDrawsTheHumanRow(t *testing.T) {
	clock := NewClock(DefaultClockDials())
	illum := &fakeIllumination{level: 0}
	notice := NewNotice(&fakeSight{clear: true}, illum, DefaultNoticeDials())
	s := NewSpawns(clock, notice, &fakeSpawner{}, illum, 1462, DefaultSpawnDials())

	defer s.Close()

	s.SetTarget(&fakeQuarry{id: "p:1"})

	assert.InDelta(t, 1.0, s.LightWeight(), 0.001, "in the dark you are not worth the walk")

	illum.level = 1
	assert.InDelta(t, DefaultSpawnDials().LightWeight, s.LightWeight(), 0.001)

	var human, beast SpawnRow

	for _, r := range DefaultSpawnDials().Rows {
		if r.LightDrawn {
			human = r
		}

		if r.Name == "wolves" {
			beast = r
		}
	}

	advanceClockToStage(t, clock, StageNight)

	illum.level = 0
	darkHuman, darkBeast := s.Weight(human), s.Weight(beast)

	illum.level = 1
	assert.Greater(t, s.Weight(human), darkHuman, "light draws the human row")
	assert.InDelta(t, darkBeast, s.Weight(beast), 0.001,
		"and leaves the beast rows alone -- only LightDrawn rows are drawn")
}

func TestSpawnsForcedChanceMakesAGroupAndWatchesIt(t *testing.T) {
	s, clock, notice, spawner, target := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))

	s.Advance(s.dials.CheckMinutes)

	require.Positive(t, s.Groups(), "a certainty must actually fire")
	require.Positive(t, spawner.calls)

	state := s.HarnessState()
	list := state["group_list"].([]map[string]interface{})
	require.NotEmpty(t, list)

	// group_list[0] is the first row that fired, and check() walks the rows in
	// declaration order -- so at night that is "dogs", not the wolves one
	// reaches for. Assert the invariant (a group starts at its own row's
	// morale) rather than hard-coding which row wins the race.
	g := list[0]
	assert.Positive(t, g["members"].(int))
	assert.False(t, g["routing"].(bool))

	var wantMorale float64

	for _, r := range DefaultSpawnDials().Rows {
		if r.Name == g["row"].(string) {
			wantMorale = r.Morale
		}
	}

	require.Positive(t, wantMorale, "the group must name a real row")
	assert.InDelta(t, wantMorale, g["morale"].(float64), 0.001,
		"a group starts at its row's morale")

	// Every member is watching the target, and the block ask 6 names is here.
	assert.Equal(t, notice.Count(), state["notice_watching"].(int))
	assert.Positive(t, notice.Count())

	seen := g["notice"].([]map[string]interface{})
	require.NotEmpty(t, seen, "the notice block rides on the group, per ask 6")

	for _, field := range []string{"sees", "distance", "light_at_quarry", "noticed"} {
		_, ok := seen[0][field]
		assert.True(t, ok, "ask 6 names %q", field)
	}

	assert.Equal(t, target.id, seen[0]["quarry"])
}

// The two-launch determinism proof is the whole reason the harness exists.
// Same build, same seed, same arrivals.
func TestSpawnsAreDeterministicUnderASeed(t *testing.T) {
	run := func() []string {
		clock := NewClock(DefaultClockDials())
		notice := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
		spawner := &fakeSpawner{}
		dials := DefaultSpawnDials()
		dials.Chance = 3

		s := NewSpawns(clock, notice, spawner, &fakeIllumination{}, 1462, dials)
		defer s.Close()

		s.SetTarget(&fakeQuarry{id: "p:1", x: 40, y: 40})
		advanceClockToStage(t, clock, StageNight)

		for i := 0; i < 40; i++ {
			s.Advance(dials.CheckMinutes)
		}

		out := []string{}

		for _, row := range s.HarnessState()["group_list"].([]map[string]interface{}) {
			out = append(out, row["group"].(string)+"/"+row["row"].(string)+"/"+itoa(row["members"].(int)))
		}

		return out
	}

	first, second := run(), run()

	require.NotEmpty(t, first, "the run must actually spawn something to prove anything")
	assert.Equal(t, first, second, "same seed, same build, same arrivals")
}

// If the draw sequence depended on how many groups were alive, despawning one
// would silently change every later arrival -- and the two-launch proof would
// fail for a reason nobody could find.
func TestSpawnsRollEveryEligibleRowEvenAtTheCap(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))

	s.dials.MaxGroups = 0

	s.Advance(s.dials.CheckMinutes)

	assert.Zero(t, s.Groups(), "the cap holds")
	assert.Positive(t, s.HarnessState()["rolls"].(int),
		"but the rolls still happened, so the RNG sequence does not depend on the cap")
}

func TestSpawnsDespawnUnwatchesItsMembers(t *testing.T) {
	s, clock, notice, _, _ := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))
	s.Advance(s.dials.CheckMinutes)

	require.Positive(t, s.Groups())
	require.Positive(t, notice.Count())

	id := s.HarnessState()["group_list"].([]map[string]interface{})[0]["group"].(string)

	require.NoError(t, s.HarnessSet("despawn", id))
	assert.False(t, s.Despawn(id), "despawning twice is false, not an error")

	if s.Groups() == 0 {
		assert.Zero(t, notice.Count(), "the last group's members stop being watched")
	}
}

// The morale STATE is this milestone's; the rout BEHAVIOUR is M4.5's. The
// third provider rule says a reported value needs a verb that moves it.
func TestSpawnsMoraleIsWritableAndRoutingFollowsIt(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))
	s.Advance(s.dials.CheckMinutes)

	id := s.HarnessState()["group_list"].([]map[string]interface{})[0]["group"].(string)

	routing, known := s.Routing(id)
	require.True(t, known)
	require.False(t, routing)

	require.NoError(t, s.HarnessSet("morale", map[string]interface{}{"group": id, "value": 10.0}))

	routing, _ = s.Routing(id)
	assert.True(t, routing, "at or below RoutAt the group reports routing")

	// ...and back up again, because a value that only falls is the M4.2 trap.
	require.NoError(t, s.HarnessSet("morale", map[string]interface{}{"group": id, "value": 80.0}))

	routing, _ = s.Routing(id)
	assert.False(t, routing)

	_, known = s.Routing("g:999")
	assert.False(t, known)
}

// A bad stand-in code must show up in the provider, not crash in the field.
func TestSpawnsFailedSpawnIsCountedNotFatal(t *testing.T) {
	clock := NewClock(DefaultClockDials())
	notice := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	spawner := &fakeSpawner{fail: true}

	s := NewSpawns(clock, notice, spawner, &fakeIllumination{}, 1462, DefaultSpawnDials())
	defer s.Close()

	s.SetTarget(&fakeQuarry{id: "p:1"})
	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))

	s.Advance(s.dials.CheckMinutes)

	assert.Zero(t, s.Groups())
	assert.Positive(t, s.HarnessState()["spawn_failures"].(int))
}

// "g:2" must sort before "g:10". A string sort would not.
func TestSpawnsGroupOrderIsNumeric(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))

	s.dials.MaxGroups = 100

	for i := 0; i < 12 && s.Groups() < 11; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	require.Greater(t, s.Groups(), 9, "need past g:9 for the ordering to bite")

	ids := s.groupIDs()
	for i := 1; i < len(ids); i++ {
		require.Less(t, groupOrdinal(ids[i-1]), groupOrdinal(ids[i]),
			"group ids must sort numerically: %v", ids)
	}
}

// An unwired model must be distinguishable from one that simply saw nothing.
func TestSpawnsReportsWhetherNoticeIsWired(t *testing.T) {
	clock := NewClock(DefaultClockDials())
	blind := NewNotice(nil, nil, DefaultNoticeDials())

	s := NewSpawns(clock, blind, &fakeSpawner{}, nil, 1462, DefaultSpawnDials())
	defer s.Close()

	assert.False(t, s.HarnessState()["notice_wired"].(bool))
}

func TestSpawnsHarnessSetValidates(t *testing.T) {
	s, _, _, _, _ := newTestSpawns(t)

	assert.Error(t, s.HarnessSet("open_bodies", "three"))
	assert.Error(t, s.HarnessSet("open_bodies", -2.0))
	assert.NoError(t, s.HarnessSet("open_bodies", 3.0))
	assert.Equal(t, 3, s.OpenBodies())

	assert.Error(t, s.HarnessSet("chance", -1.0))
	assert.Error(t, s.HarnessSet("check_minutes", 0.0))
	assert.NoError(t, s.HarnessSet("check_minutes", 2.0))

	assert.Error(t, s.HarnessSet("notice_radius", 0.0))
	assert.NoError(t, s.HarnessSet("notice_radius", 20.0))

	assert.Error(t, s.HarnessSet("notice_lit_level", 2.0))
	assert.NoError(t, s.HarnessSet("notice_lit_level", 0.5))

	assert.Error(t, s.HarnessSet("despawn", 7.0))
	assert.Error(t, s.HarnessSet("despawn", "g:999"))

	assert.Error(t, s.HarnessSet("morale", "g:1"))
	assert.Error(t, s.HarnessSet("morale", map[string]interface{}{"group": "g:1"}))

	assert.Error(t, s.HarnessSet("nonsense", 1.0))
}

// Advancing the tables must advance awareness too, or a group spawned this
// tick stands blind until the next one.
func TestSpawnsAdvanceStepsNotice(t *testing.T) {
	s, clock, notice, _, _ := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))
	s.Advance(s.dials.CheckMinutes)

	require.Positive(t, notice.Count())

	before := notice.Checks()
	s.Advance(DefaultNoticeDials().ReEvaluateMinutes + s.dials.CheckMinutes)

	assert.Greater(t, notice.Checks(), before)
}

func TestSpawnsWithoutATargetDoesNothing(t *testing.T) {
	clock := NewClock(DefaultClockDials())
	notice := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	spawner := &fakeSpawner{}

	s := NewSpawns(clock, notice, spawner, &fakeIllumination{}, 1462, DefaultSpawnDials())
	defer s.Close()

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))

	s.Advance(s.dials.CheckMinutes * 5)

	assert.Zero(t, spawner.calls, "no target, nothing to spawn around")
	assert.False(t, s.HarnessState()["has_target"].(bool))
}
