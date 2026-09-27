package d2world

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// M4.6 B1: every field the world save will carry is reported by its provider
// before anything saves it. The stream tests below are the ones that matter
// most: a provider that reported a draw count one off from where its stream
// really stands would restore a night one roll out of step, and nothing but a
// long playtest would ever show it.

// streamOf reads a provider's "rng" block.
func streamOf(t *testing.T, state map[string]interface{}) (seed int64, draws uint64) {
	t.Helper()

	block, ok := state["rng"].(map[string]interface{})
	require.True(t, ok, "the provider must report an rng block, got %v", state["rng"])

	seed, ok = block["seed"].(int64)
	require.True(t, ok, "rng.seed must be an int64, got %T", block["seed"])

	draws, ok = block["draws"].(uint64)
	require.True(t, ok, "rng.draws must be a uint64, got %T", block["draws"])

	return seed, draws
}

// nextAfter is the value a PLAIN stdlib stream at seed hands out after draws
// values: the reference every reported position is judged against, so a
// counter and a restore that are wrong together cannot agree with themselves.
func nextAfter(seed int64, draws uint64) int64 {
	r := rand.New(rand.NewSource(seed)) // nolint:gosec // test

	for i := uint64(0); i < draws; i++ {
		r.Int63()
	}

	return r.Int63()
}

// assertStreamAt checks that live, the system's own *rand.Rand, stands exactly
// where the provider says it does -- and that one draw short does not, which
// is what makes the first check mean anything.
func assertStreamAt(t *testing.T, live *rand.Rand, seed int64, draws uint64) {
	t.Helper()

	// A count of zero has no "one short" to compare against (and draws-1
	// would wrap to 2^64-1); every caller has drawn, so zero is a failure.
	require.Positive(t, draws, "the stream must have been drawn from")

	want := nextAfter(seed, draws)
	require.NotEqual(t, want, nextAfter(seed, draws-1), "the off-by-one control must differ")
	assert.Equal(t, want, live.Int63(), "the reported draw count is where the live stream stands")
}

func TestSpawnsReportWhereTheirStreamStands(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	seed, draws := streamOf(t, s.HarnessState())
	assert.Equal(t, int64(1462), seed, "the tables' stream is seeded from the run's seed")
	assert.Zero(t, draws, "nothing is drawn before the first check")

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))

	for i := 0; i < 6; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	state := s.HarnessState()
	require.Positive(t, state["groups"].(int), "the run must spawn to exercise the pack-size draw")

	_, draws = streamOf(t, state)
	assert.Greater(t, draws, uint64(state["rolls"].(int)),
		"a forced arrival draws its pack size on top of one roll per eligible row")

	assertStreamAt(t, s.rng.Rand, 1462, draws)
}

// At a chance of zero nothing spawns, so the only draws are the rolls -- one
// Float64 per eligible row per check. The system reports both numbers; they
// must agree exactly.
func TestSpawnsAtChanceZeroDrawOncePerRoll(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 0.0))

	before := s.HarnessState()
	_, d0 := streamOf(t, before)

	for i := 0; i < 10; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	after := s.HarnessState()
	_, d1 := streamOf(t, after)

	rolls := after["rolls"].(int) - before["rolls"].(int)
	require.Positive(t, rolls, "the night must have eligible rows, or this proves nothing")
	assert.Equal(t, uint64(rolls), d1-d0, "one draw per roll, no more and no fewer")
	assert.Equal(t, before["next_id"], after["next_id"], "nothing arrived, so no group number was spent")
}

func TestSpawnsReportSinceCheckAndNextID(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("check_minutes", 10.0))

	start := s.HarnessState()["since_check_minutes"].(float64)
	require.Less(t, start, 10.0)

	s.Advance(10 - start - 3) // three minutes short of the check
	assert.InDelta(t, 7.0, s.HarnessState()["since_check_minutes"].(float64), 1e-9)

	checks := s.HarnessState()["checks"].(int)

	s.Advance(3)
	assert.Equal(t, checks+1, s.HarnessState()["checks"].(int), "the tenth minute checks")
	assert.Zero(t, s.HarnessState()["since_check_minutes"].(float64), "and the count starts again")

	require.NoError(t, s.HarnessSet("chance", 100.0))

	nextBefore := s.HarnessState()["next_id"].(int)
	groupsBefore := s.Groups()

	s.Advance(10)

	made := s.Groups() - groupsBefore
	require.Positive(t, made)
	assert.Equal(t, nextBefore+made, s.HarnessState()["next_id"].(int), "one group number per group")
}

func TestCombatReportsWhereItsStreamStands(t *testing.T) {
	f := newResolverFight(t, 1462)

	seed, draws := streamOf(t, f.c.HarnessState())
	assert.Equal(t, int64(1462), seed)
	assert.Zero(t, draws, "no fight, no roll")
	assert.Equal(t, 1, f.c.HarnessState()["next_id"].(int))

	f.add(t, "n:1", 500, Profile{})
	f.add(t, "n:2", 500, Profile{})
	f.open(t)

	for i := 0; i < 4; i++ {
		f.round()
	}

	state := f.c.HarnessState()
	_, draws = streamOf(t, state)
	require.Positive(t, draws, "four rounds of blows must have rolled")
	assert.Equal(t, state["encounters"].(int)+1, state["next_id"].(int), "the next encounter's number")

	assertStreamAt(t, f.c.rng.Rand, 1462, draws)
}

func TestRisingReportsItsStreamAndLastStage(t *testing.T) {
	r, corpses, night := newTestRising(t, DefaultRisingDials())

	for _, id := range []string{"a", "b", "c"} {
		corpses.FallHuman(id, 0, 0)
	}

	corpses.FallHuman("staked", 0, 0)
	corpses.Close("staked")

	seed, draws := streamOf(t, r.HarnessState())
	assert.Equal(t, int64(1462), seed)
	assert.Zero(t, draws)
	assert.Equal(t, StageDay.String(), r.HarnessState()["last_stage"])

	night.set(0, StageNight)
	r.Advance()

	state := r.HarnessState()
	_, draws = streamOf(t, state)

	// Three open men draw once each; the staked one is no door and draws
	// nothing. The test laid the bodies, so it knows the answer.
	assert.Equal(t, uint64(3), draws, "one draw per door per band")
	assert.Equal(t, StageNight.String(), state["last_stage"])
	assert.Equal(t, 0, state["band"], "band is the band last rolled")

	assertStreamAt(t, r.rng.Rand, 1462, draws)
}

func TestLightReportsNextID(t *testing.T) {
	l := NewLight(NewClock(DefaultClockDials()), DefaultLightDials())
	t.Cleanup(l.Close)

	assert.Equal(t, 1, l.HarnessState()["next_id"])

	l.Add(SourceTorch, true, 0, 0)
	l.Add(SourceHearth, false, 3, 3)
	assert.Equal(t, 3, l.HarnessState()["next_id"], "two sources, two ids spent")

	l.Remove(1)
	assert.Equal(t, 3, l.HarnessState()["next_id"], "putting one out does not give its id back")
}

func TestNoticeReportsMinutesSinceCheck(t *testing.T) {
	n, _, _ := newTestNotice(true, 0)
	n.Watch(&fakeWatcher{id: "n:1"}, &fakeQuarry{id: "p:1", x: 5})

	row := func() map[string]interface{} { return n.Report()[0] }
	assert.Zero(t, row()["minutes_since_check"], "Watch evaluates at once")

	step := n.Dials().ReEvaluateMinutes * 0.25
	n.Advance(step)
	assert.InDelta(t, step, row()["minutes_since_check"].(float64), 1e-12)

	n.Advance(n.Dials().ReEvaluateMinutes)
	assert.Zero(t, row()["minutes_since_check"], "a re-evaluation starts it again")
}

func TestPursuitReportsTheLastSolve(t *testing.T) {
	p, _ := newTestPursuit(true)
	t.Cleanup(p.Close)

	p.Chase(&fakeHunter{id: "n:1", x: 0, y: 0}, &fakeQuarry{id: "p:1", x: 3, y: 4})

	row := p.HarnessState()["chase_list"].([]map[string]interface{})[0]
	assert.Equal(t, 3.0, row["solved_at_x"], "where the quarry stood at the solve")
	assert.Equal(t, 4.0, row["solved_at_y"])
	assert.Equal(t, 5.0, row["solved_distance"], "how far the hunter was: a 3-4-5 triangle")
}

func TestCorpsesReportTheirMapsAndDownedAt(t *testing.T) {
	minute := 100.0
	c := NewCorpses(nil, nil)
	c.SetClock(func() float64 { return minute })

	c.FallHuman("dead:1", 1, 1)
	require.True(t, c.Rise("dead:1"))
	c.Raised("dead:1", "m:7")

	state := c.HarnessState()
	assert.Equal(t, map[string]string{"m:7": "dead:1"}, state["risen_as"])
	assert.Equal(t, map[string]string{"dead:1": "m:7"}, state["walker"])
	assert.Equal(t, map[string]string{"dead:1": "m:7"}, state["last"])

	minute = 142.5
	c.Fall("m:7", RisenRow, 4, 4) // cut down: the same body, Downed

	state = c.HarnessState()
	body := state["bodies"].([]map[string]interface{})[0]
	assert.Equal(t, "downed", body["state"])
	assert.Equal(t, 142.5, body["downed_at"], "the minute he went down")
	assert.Empty(t, state["walker"], "he walks as no one now")
	assert.Equal(t, map[string]string{"dead:1": "m:7"}, state["last"], "but last remembers who he was")

	// The maps are copies: a reader cannot reach the registry through them.
	state["walker"].(map[string]string)["dead:1"] = "intruder"
	assert.Empty(t, c.HarnessState()["walker"])

	_, err := json.Marshal(state)
	require.NoError(t, err)
}

// groupReport finds one group's block in a spawns report by id.
func groupReport(t *testing.T, state map[string]interface{}, id string) map[string]interface{} {
	t.Helper()

	for _, g := range state["group_list"].([]map[string]interface{}) {
		if g["group"] == id {
			return g
		}
	}

	t.Fatalf("no group %q in the report", id)

	return nil
}

// M4.6 B1 review, B2: a group reports every member it was born with, by id,
// and a death does not take one off the list. Combat's withdraw unwatches the
// dead (so his notice row goes) but never shortens g.members, and the save must
// carry the pack as the game holds it. C14 rides along: born_where is a copy.
func TestSpawnsReportEveryMemberEvenTheDead(t *testing.T) {
	s, clock, notice, _, _ := newTestSpawns(t)

	advanceClockToStage(t, clock, StageNight)
	require.NoError(t, s.HarnessSet("chance", 100.0))

	for i := 0; i < 6 && s.Groups() == 0; i++ {
		s.Advance(s.dials.CheckMinutes)
	}

	require.Positive(t, s.Groups(), "a certainty at night must place a group")

	gid := s.HarnessState()["group_list"].([]map[string]interface{})[0]["group"].(string)

	// The test's answer comes from the group itself, not from the report.
	want := make([]string, 0, len(s.groups[gid].members))
	for _, m := range s.groups[gid].members {
		want = append(want, m.WatcherID())
	}

	require.NotEmpty(t, want)

	g := groupReport(t, s.HarnessState(), gid)
	assert.Equal(t, want, g["member_ids"], "every member, in the order placed")
	assert.Len(t, g["notice"], len(want), "all alive: one notice row each")

	// What Combat.withdraw does to a member that dies.
	notice.Unwatch(want[0])

	g = groupReport(t, s.HarnessState(), gid)
	assert.Equal(t, want, g["member_ids"], "the dead member is still a member of his pack")
	assert.Equal(t, len(want), g["members"])

	rows := g["notice"].([]map[string]interface{})
	assert.Len(t, rows, len(want)-1, "but his notice row is gone")

	for _, row := range rows {
		assert.NotEqual(t, want[0], row["watcher"])
	}

	// Copies: a reader can move neither a member nor a birthplace.
	g["member_ids"].([]string)[0] = "intruder"
	assert.Equal(t, want[0], s.groups[gid].members[0].WatcherID())
	assert.Equal(t, want, groupReport(t, s.HarnessState(), gid)["member_ids"])

	born := g["born_where"].([][2]float64)
	require.NotEmpty(t, born)

	first := born[0]
	born[0] = [2]float64{-1, -1}
	assert.Equal(t, first, groupReport(t, s.HarnessState(), gid)["born_where"].([][2]float64)[0],
		"born_where is a copy")

	_, err := json.Marshal(s.HarnessState())
	require.NoError(t, err)
}

// fakeDeployer places squad models with made-up ids.
type fakeDeployer struct{ n int }

func (d *fakeDeployer) Deploy(_, _ float64) (id string, maxHealth int, ok bool) {
	d.n++

	return "m:" + itoa(d.n), 50, true
}

func (d *fakeDeployer) Recall(string) bool { return true }

// M4.6 B1 review, C5: the squads report the number the next squad will take,
// which recalling a squad does not give back.
func TestSquadsReportNextSquadID(t *testing.T) {
	clock := NewClock(DefaultClockDials())
	t.Cleanup(clock.Close)

	sq := NewSquads(clock, DefaultMeterDials(), &fakeDeployer{})
	t.Cleanup(sq.Close)

	assert.Equal(t, 2, sq.HarnessState()["next_squad_id"], "s:1 is born with the owner")

	require.NoError(t, sq.HarnessSet("squad_add", map[string]interface{}{}))
	assert.Equal(t, 3, sq.HarnessState()["next_squad_id"], "s:2 spent 2")

	require.NoError(t, sq.HarnessSet("squad_remove", "s:2"))
	assert.Equal(t, 3, sq.HarnessState()["next_squad_id"], "recalling s:2 does not give its number back")

	require.NoError(t, sq.HarnessSet("squad_add", map[string]interface{}{}))
	assert.Equal(t, 4, sq.HarnessState()["next_squad_id"])

	ids := []string{}
	for _, row := range sq.HarnessState()["squads"].([]map[string]interface{}) {
		ids = append(ids, row["squad"].(string))
	}

	assert.Equal(t, []string{"s:1", "s:3"}, ids, "the new squad took the reported number")
}

// M4.6 B1 review, C8: a provider built with no stream reports the stream as
// absent, never as a real-looking stream at draw 0 of seed 0.
func TestAbsentStreamReportsAbsent(t *testing.T) {
	s := &Spawns{}
	block := s.HarnessState()["rng"].(map[string]interface{})

	assert.Equal(t, false, block["present"])
	assert.NotContains(t, block, "seed")
	assert.NotContains(t, block, "draws")

	live, _, _, _, _ := newTestSpawns(t)
	assert.Equal(t, true, live.HarnessState()["rng"].(map[string]interface{})["present"])
	assert.Equal(t, "1462", live.HarnessState()["rng"].(map[string]interface{})["seed_str"])
}
