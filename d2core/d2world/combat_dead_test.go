package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deadProfile(group string) Profile {
	return Profile{Group: group, Row: RisenRow, Speed: 0, DamageMin: 5, DamageMax: 10, Count: 1, Dead: true}
}

// M4.7 step 3, R2 §2A: at first light the dead break off. A fight with only
// the dead in it ends "dawn"; a dog in the same fight fights on.
func TestBreakOffAtFirstLight(t *testing.T) {
	isDead := func(f *resolverFight) func(string) bool {
		return func(id string) bool { return f.profiles.byID[id].Dead }
	}

	f := newResolverFight(t, 1)
	f.add(t, "d:1", 60, deadProfile("g:1"))
	f.open(t)

	assert.Equal(t, 1, f.c.BreakOff(isDead(f)))
	assert.False(t, f.c.Fighting())
	assert.Equal(t, "dawn", f.c.EndedReason())

	g := newResolverFight(t, 1)
	g.add(t, "d:1", 60, deadProfile("g:1"))
	g.add(t, "dog:1", 20, Profile{Group: "g:2", Row: "dogs", Speed: 3, DamageMin: 3, DamageMax: 6, Count: 1})
	g.morale.morale["g:2"] = 50
	g.open(t)

	assert.Equal(t, 1, g.c.BreakOff(isDead(g)))
	assert.True(t, g.c.Fighting(), "the dog fights on")
	assert.Zero(t, g.c.BreakOff(isDead(g)), "nothing left to break off")

	// He kills the dog after first light: that fight was won, not left.
	for i := 0; i < 200 && g.c.Fighting(); i++ {
		g.round()
	}

	require.False(t, g.c.Fighting())
	assert.Equal(t, "enemies_dead", g.c.EndedReason())
	assert.Equal(t, 1, f.c.HarnessState()["ended_dawn"])
}

// D8 §9 / M4.4c-2 ask 2a: the dead take neither surprise branch -- caught
// head-down by the dead, he is not surprised; by a dog, he is (the control).
func TestTheDeadNeverSurprise(t *testing.T) {
	for _, c := range []struct {
		name      string
		p         Profile
		surprised bool
	}{
		{"dead", deadProfile("g:1"), false},
		{"dog", Profile{Group: "g:1", Row: "dogs", Speed: 3, DamageMin: 3, DamageMax: 6, Count: 1}, true},
	} {
		f := newResolverFight(t, 1)
		f.fitness.activity = ActivityForage
		f.add(t, "e:1", 60, c.p)
		f.open(t)

		require.NotNil(t, f.c.encounter)
		assert.Equal(t, c.surprised, f.c.encounter.surprised, c.name)
	}
}

// R2 §2B: quick-resolve never fires against the dead, even at a forced
// advantage and with morale reported -- the Dead flag is the fence.
func TestNeverQuickResolveTheDead(t *testing.T) {
	f := newResolverFight(t, 1)
	f.add(t, "d:1", 60, deadProfile("g:1"))
	f.morale.morale["g:1"] = 50 // even if something reported nerve

	assert.False(t, f.c.mundane("d:1"))
}
