package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// M4.7 step 3: the dead are a row the tables never roll. With the chance
// forced and the cap wide, a whole night of checks makes beasts and men and
// never the dead -- and the risen row's weight is zero in every stage.
func TestRisenNeverTabled(t *testing.T) {
	s, clock, _, _, _ := newTestSpawns(t)

	s.dials.Chance = 1000
	s.dials.MaxGroups = 1000

	risen, ok := s.rowNamed(RisenRow)
	require.True(t, ok, "the dead have a row")

	for _, stage := range []Stage{StageDawn, StageDay, StageDusk, StageNight} {
		advanceClockToStage(t, clock, stage)
		assert.Zero(t, s.Weight(risen), stage.String())

		for i := 0; i < 5; i++ {
			s.check()
		}
	}

	require.NotZero(t, len(s.groups), "the forced tables made something")

	for _, g := range s.groups {
		assert.NotEqual(t, RisenRow, g.row, "the tables raised the dead")
	}
}

// Raise stands ONE of the dead up exactly where the body lay, drawn as a man
// (Q5a), watching the target; it fights last, never routs, and the cap does
// not count it.
func TestRaiseStandsOneWhereTheBodyLay(t *testing.T) {
	s, _, notice, spawner, _ := newTestSpawns(t)

	id := s.Raise(12.5, 30.5)
	require.NotEmpty(t, id)

	assert.Equal(t, "opportunists", spawner.lastKind, "a risen man is drawn as a man")
	assert.Equal(t, 1, spawner.lastCount)
	assert.Zero(t, spawner.lastMin)
	assert.Zero(t, spawner.lastMax, "where the body lay, not a ring around it")

	p, ok := s.ProfileOf(id)
	require.True(t, ok)
	assert.True(t, p.Dead)
	assert.Equal(t, RisenRow, p.Row)
	assert.Zero(t, p.Speed, "the dead go last")

	routing, known := s.Routing(p.Group)
	assert.True(t, known)
	assert.False(t, routing, "morale 0 is none, not broken")

	_, watching := notice.watches[id]
	assert.True(t, watching, "it watches him like any arrival")

	assert.Zero(t, s.liveGroups(), "the cap counts what the tables send, not the dead")

	spawner.fail = true
	assert.Empty(t, s.Raise(1, 1), "nowhere to stand: nothing stands")
}

// First light lays every risen down where it stands and sends its group away;
// a beast group is left to daybreak.
func TestLayDownDead(t *testing.T) {
	s, _, _, spawner, _ := newTestSpawns(t)

	a, b := s.Raise(10, 10), s.Raise(20, 20)
	s.spawn(DefaultSpawnDials().Rows[0], 1) // a dog pack

	laid := map[string][2]float64{}
	s.SetLayDead(func(id string, x, y float64) { laid[id] = [2]float64{x, y} })

	n := s.LayDownDead()

	assert.Equal(t, 2, n)
	assert.Equal(t, [2]float64{10, 10}, laid[a])
	assert.Equal(t, [2]float64{20, 20}, laid[b])
	assert.Len(t, s.groups, 1, "the dogs stay")
	assert.Equal(t, 2, spawner.removed)
}

// A risen man who falls is the same body, open again where he fell -- once.
func TestCorpsesRisenFallsAsHimself(t *testing.T) {
	open := 0
	c := NewCorpses(nil, func(d int) { open += d })

	c.FallHuman("dead:1", 1, 1)
	require.True(t, c.Rise("dead:1"))
	c.Raised("dead:1", "m:7")
	require.Equal(t, 0, open)

	c.Fall("m:7", RisenRow, 5, 6)

	b, _ := c.Get("dead:1")
	assert.Equal(t, CorpseFresh, b.State)
	assert.Equal(t, [2]float64{5, 6}, [2]float64{b.X, b.Y})
	assert.Equal(t, 1, open)
	assert.False(t, c.Has("m:7"), "not a second body")

	c.Fall("m:7", RisenRow, 9, 9)
	assert.Equal(t, 1, open, "a second fall of the same man is nothing")

	c.Raised("dead:1", "m:8")
	assert.Empty(t, c.risenAs["m:8"], "only a risen body walks")

	// Risen again as m:9 after m:7 was cut down: m:7's stale lay (an older
	// group still holding its dead member) must not move the body.
	require.True(t, c.Rise("dead:1"))
	c.Raised("dead:1", "m:9")
	c.Fall("m:7", RisenRow, 1, 1)

	b, _ = c.Get("dead:1")
	assert.Equal(t, CorpseRisen, b.State, "a stale member does not lay the body")

	c.Fall("m:9", RisenRow, 3, 4)

	b, _ = c.Get("dead:1")
	assert.Equal(t, [2]float64{3, 4}, [2]float64{b.X, b.Y}, "its walker does")
}

// A harness or daybreak despawn of a risen group lays its bodies too.
func TestDespawnLaysTheDead(t *testing.T) {
	s, _, _, _, _ := newTestSpawns(t)

	id := s.Raise(4, 5)

	var laid []string

	s.SetLayDead(func(member string, _, _ float64) { laid = append(laid, member) })

	// Standing, it is neither spent nor routing in the harness's report.
	st := s.HarnessState()
	assert.Equal(t, 0, st["spent_groups"], "the dead are not the tables' spent")

	groups, ok := st["group_list"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, groups, 1)
	assert.Equal(t, false, groups[0]["routing"], "morale 0 is not broken")

	p, _ := s.ProfileOf(id)
	require.True(t, s.Despawn(p.Group))
	assert.Equal(t, []string{id}, laid)
}

// The rising stands a body up through its raise hook, and keeps it lying when
// the hook cannot; first light is heard once, after the dawn count.
func TestRisingRaisesAndHearsFirstLight(t *testing.T) {
	dials := DefaultRisingDials()
	dials.P = 1

	r, corpses, night := newTestRising(t, dials)
	corpses.FallHuman("stands", 0, 0)
	corpses.FallHuman("cannot", 0, 0)

	var order []string

	r.SetRaise(func(b Corpse) string {
		if b.ID == "cannot" {
			return ""
		}

		return "m:" + b.ID
	})
	r.SetFirstLight(func() { order = append(order, "light") })

	night.set(0, StageNight)
	r.Advance()

	stood, _ := corpses.Get("stands")
	lay, _ := corpses.Get("cannot")

	assert.Equal(t, CorpseRisen, stood.State)
	assert.Equal(t, CorpseFresh, lay.State, "no tile to stand on: it lies")
	assert.Equal(t, "stands", corpses.risenAs["m:stands"])

	night.set(-1, StageDawn)
	r.Advance()
	r.Advance()

	assert.Equal(t, []string{"light"}, order)
	assert.InDelta(t, dials.PerOpenAtDawn, r.Pressure(), 1e-9, "the one left lying counts at dawn")
}
