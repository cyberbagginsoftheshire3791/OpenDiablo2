package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// M4.7 step 3b review: a Downed man who stands again is back in the fight he
// fell in at once -- no notice, no engage gate -- and the fallen row goes; a
// member who was never in the fight is not taken in (the control).
func TestRejoinTheFightHeFellIn(t *testing.T) {
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

	require.True(t, f.c.Fighting())

	// Far off and unnoticed: an arrival would not join. He is not arriving.
	stood := &fakeWatcher{id: "d:2", x: 90, y: 90}
	f.profiles.byID["d:2"] = deadProfile("g:9")

	require.True(t, f.c.Rejoin("d:1", stood))

	ids := map[string]bool{}
	for _, en := range f.c.encounter.enemies {
		ids[en.WatcherID()] = true
	}

	assert.True(t, ids["d:2"], "he is in the fight")
	assert.False(t, ids["d:1"], "the fallen row went with the remains")
	assert.Contains(t, f.c.encounter.enemyOrder, "d:2")

	assert.False(t, f.c.Rejoin("stranger", &fakeWatcher{id: "x:1"}), "only a man who fell in this fight rejoins it")
}

// Quick-resolve never finishes a fight while a Downed man lies in it.
func TestNoQuickResolveOverTheDowned(t *testing.T) {
	corpses := NewCorpses(nil, nil)
	corpses.FallHuman("body", 41, 40)
	require.True(t, corpses.Rise("body"))
	corpses.Raised("body", "d:1")

	f := newResolverFight(t, 1)
	f.c.SetCorpses(corpses)
	f.set(t, "quick_resolve_advantage", 0.0)
	f.add(t, "d:1", 1, deadProfile("g:1"))
	f.add(t, "dog:1", 500, Profile{Group: "g:2", Row: "dogs", Speed: 3, DamageMin: 1, DamageMax: 1, Count: 1})
	f.morale.morale["g:2"] = 50
	f.open(t)

	for i := 0; i < 30 && !f.c.encounter.dead["d:1"]; i++ {
		f.round()
	}

	require.True(t, f.c.encounter.dead["d:1"])
	assert.False(t, f.c.tryQuickResolve(), "not while he may stand")

	require.True(t, corpses.Close("body"))
	assert.True(t, f.c.tryQuickResolve(), "staked, the dog alone may be finished (the control)")
}

// The band's roll does not cut a Downed man's window short (the review):
// cut down a minute before the band turns, he still lies when it does.
func TestBandEdgeKeepsTheWindow(t *testing.T) {
	dials := DefaultRisingDials()
	dials.P = 0

	r, corpses, night := newTestRising(t, dials)

	now := 0.0
	corpses.SetClock(func() float64 { return now })
	r.SetClock(func() float64 { return now })

	night.set(0, StageNight)
	r.Advance()

	downedAt(t, corpses, &now, "edge", 100)

	now = 101
	night.set(1, StageNight)
	r.Advance()

	b, _ := corpses.Get("edge")
	assert.Equal(t, CorpseDowned, b.State, "the band turned inside his window")

	now = 100 + dials.DownedMinutes
	r.Advance()

	b, _ = corpses.Get("edge")
	assert.Equal(t, CorpseRisen, b.State, "his window up, he stands")
}
