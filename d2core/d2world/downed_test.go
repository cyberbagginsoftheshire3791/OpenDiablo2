package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// downedAt lays a risen man Downed at a world minute: fallen, risen, cut down.
func downedAt(t *testing.T, c *Corpses, clock *float64, id string, at float64) {
	t.Helper()

	c.FallHuman(id, 0, 0)
	require.True(t, c.Rise(id))
	c.Raised(id, "m:"+id)

	*clock = at
	c.Fall("m:"+id, RisenRow, 1, 1)

	b, _ := c.Get(id)
	require.Equal(t, CorpseDowned, b.State)
}

// M4.7 step 3b, S1 §6.2 / R2 §3: at night a Downed man stands again once his
// window is up, and not a minute before; by day he waits.
func TestDownedWindow(t *testing.T) {
	dials := DefaultRisingDials()
	dials.P = 0

	r, corpses, night := newTestRising(t, dials)

	now := 0.0
	corpses.SetClock(func() float64 { return now })
	r.SetClock(func() float64 { return now })

	var stood []string

	r.SetRaise(func(b Corpse) string {
		stood = append(stood, b.ID)

		return "again:" + b.ID
	})

	night.set(0, StageNight)
	r.Advance() // the band's roll, with nothing yet to roll

	downedAt(t, corpses, &now, "night", 100)

	now = 100 + dials.DownedMinutes - 0.5
	r.Advance()

	b, _ := corpses.Get("night")
	assert.Equal(t, CorpseDowned, b.State, "inside his window he lies")

	now = 100 + dials.DownedMinutes
	r.Advance()

	b, _ = corpses.Get("night")
	assert.Equal(t, CorpseRisen, b.State, "his window up, he stands again")
	assert.Equal(t, []string{"night"}, stood)
	assert.Equal(t, 1, r.stoodAgain)
	assert.Equal(t, "again:night", corpses.LastWalker("night"))

	// By day the window does not stand him: he waits for the dark.
	night.set(-1, StageDawn)
	r.Advance()
	night.set(-1, StageDay)
	downedAt(t, corpses, &now, "day", 500)

	now = 900
	r.Advance()

	b, _ = corpses.Get("day")
	assert.Equal(t, CorpseDowned, b.State, "by day he waits")
}

// Q7a: a Downed man stands CERTAINLY in the next deep-night band -- at P 0,
// through the roll, not the window (no clock here).
func TestDownedStandsCertainlyNextBand(t *testing.T) {
	dials := DefaultRisingDials()
	dials.P = 0

	r, corpses, night := newTestRising(t, dials)

	now := 0.0
	corpses.SetClock(func() float64 { return now })
	downedAt(t, corpses, &now, "lay", 10)
	corpses.FallHuman("open", 0, 0)

	night.set(0, StageNight)
	r.Advance()

	lay, _ := corpses.Get("lay")
	open, _ := corpses.Get("open")

	assert.Equal(t, CorpseRisen, lay.State, "the Downed stand")
	assert.Equal(t, CorpseFresh, open.State, "the open do not, at P 0 (the control)")
	assert.Equal(t, 1, r.risen)
	assert.Zero(t, r.stoodAgain)
}

// A Downed man at dawn is an unrited body: soul pressure counts him.
func TestDawnCountsTheDowned(t *testing.T) {
	dials := DefaultRisingDials()
	dials.P = 0

	r, corpses, night := newTestRising(t, dials)

	now := 0.0
	corpses.SetClock(func() float64 { return now })

	// Into the LAST band first, so every roll of the night is spent; he goes
	// down after it, and with no clock on the rising nothing stands him
	// before the dawn count.
	night.set(risingBands-1, StageNight)
	r.Advance()
	downedAt(t, corpses, &now, "d", 1)

	night.set(-1, StageDawn)
	r.Advance()

	assert.InDelta(t, dials.PerOpenAtDawn, r.Pressure(), 1e-9)
}

// The stake in a fight is an Action: it spends the Action and cannot be
// committed twice in a turn (R2 §3; M4.7 step 3b).
func TestCommitStakeIsAnAction(t *testing.T) {
	f := newResolverFight(t, 1)
	f.set(t, "player_control", "human")
	f.add(t, "d:1", 60, deadProfile("g:1"))
	f.open(t)

	for i := 0; i < 20 && !f.c.Awaiting(); i++ {
		f.round()
	}

	require.True(t, f.c.Awaiting(), "his turn opens")
	require.NoError(t, f.c.Commit(CommitStake, "body"))
	assert.True(t, f.c.ActionSpent())
	assert.Error(t, f.c.Commit(CommitStake, "body"), "one Action a turn")
}

// R2 §3, "possibly while the fight still runs": when the last of the dead is
// cut down the fight does not end -- he lies Downed and may stand; it ends
// when he is staked. A dog's death ends its fight at once (the control).
func TestTheFightHoldsForTheDowned(t *testing.T) {
	corpses := NewCorpses(nil, nil)
	corpses.FallHuman("body", 41, 40)
	require.True(t, corpses.Rise("body"))
	corpses.Raised("body", "d:1")

	f := newResolverFight(t, 1)
	f.c.SetCorpses(corpses)
	f.add(t, "d:1", 1, deadProfile("g:1"))
	f.open(t)

	for i := 0; i < 20 && !f.c.encounter.dead["d:1"]; i++ {
		f.round()
	}

	b, _ := corpses.Get("body")
	require.Equal(t, CorpseDowned, b.State, "cut down, he lies Downed")
	assert.True(t, f.c.Fighting(), "and the fight holds while he may stand")

	require.True(t, corpses.Close("body"))
	f.round()

	assert.False(t, f.c.Fighting())
	assert.Equal(t, "enemies_dead", f.c.EndedReason(), "staked, the fight is won")

	// The control: a dog cut down ends the fight on the blow.
	g := newResolverFight(t, 1)
	g.c.SetCorpses(NewCorpses(nil, nil))
	g.add(t, "dog:1", 1, Profile{Group: "g:2", Row: "dogs", Speed: 3, DamageMin: 3, DamageMax: 6, Count: 1})
	g.morale.morale["g:2"] = 50
	g.open(t)

	for i := 0; i < 20 && g.c.Fighting(); i++ {
		g.round()
	}

	assert.False(t, g.c.Fighting())
}
