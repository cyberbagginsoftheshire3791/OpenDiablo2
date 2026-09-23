package d2world

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T1, the tactical layer. Every assertion here is paired with the control that
// makes it mean something -- a fight that never paces and a fight that always
// paces both pass a one-sided test.

// fakeStepper walks a watcher toward a point one Chebyshev tile per call to
// settle(), stopping beside the point. Until settle() has run `frames` times
// after an order, Moving reports true -- so a test can see the fight WAIT for
// a walk rather than trusting that it would.
type fakeStepper struct {
	f       *resolverFight
	frames  int
	pending map[string]int
	orders  []string
	halted  []string
	refuse  bool
}

func newFakeStepper(f *resolverFight, frames int) *fakeStepper {
	return &fakeStepper{f: f, frames: frames, pending: map[string]int{}}
}

func (s *fakeStepper) StepToward(id string, x, y float64, tiles int) bool {
	if s.refuse {
		return false
	}

	w, ok := s.f.watchers[id]
	if !ok {
		return false
	}

	s.orders = append(s.orders, id)

	for i := 0; i < tiles; i++ {
		dx, dy := x-w.x, y-w.y
		if math.Abs(dx) <= 1 && math.Abs(dy) <= 1 {
			break
		}

		w.x += sign(dx)
		w.y += sign(dy)
	}

	s.pending[id] = s.frames

	return true
}

func (s *fakeStepper) Moving(id string) bool { return s.pending[id] > 0 }

func (s *fakeStepper) Halt(id string) {
	s.halted = append(s.halted, id)
	s.pending[id] = 0
}

// settle is one frame of walking for everything ordered.
func (s *fakeStepper) settle() {
	for id, n := range s.pending {
		if n > 0 {
			s.pending[id] = n - 1
		}
	}
}

func sign(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}

	return 0
}

// pacedFight is a human-controlled fight with the tactical layer on, a stepper
// attached, and an enemy placed `tiles` east of the player.
func pacedFight(t *testing.T, tune func(*CombatDials)) (*resolverFight, *fakeStepper) {
	t.Helper()

	f := humanFight(t, 1462, func(d *CombatDials) {
		d.Paced = true
		d.EngageTiles = TacticalEngageTiles
		d.DisengageTiles = TacticalDisengageTiles
		d.EnemyMoveTiles = TacticalEnemyMoveTiles

		if tune != nil {
			tune(d)
		}
	})

	s := newFakeStepper(f, 3)
	f.c.SetStepper(s)

	return f, s
}

// tick runs n frames of 1/60 s, settling the stepper each frame the way the
// map engine finishes walks while the screen keeps drawing.
func tick(f *resolverFight, s *fakeStepper, n int) {
	for i := 0; i < n; i++ {
		s.settle()
		f.c.Tick(1.0 / 60)
	}
}

// untilAwaiting ticks until the player's turn opens, or fails.
func untilAwaiting(t *testing.T, f *resolverFight, s *fakeStepper) {
	t.Helper()

	for i := 0; i < 600; i++ {
		if f.c.Awaiting() || !f.c.Fighting() {
			return
		}

		tick(f, s, 1)
	}

	t.Fatal("the player's turn never opened")
}

func TestPacedFightOpensAtTheEngageRadius(t *testing.T) {
	f, _ := pacedFight(t, nil)

	w := f.add(t, "e:1", 40, Profile{})
	w.x = 44 // four tiles east: inside engage (5), well outside reach (1)

	f.c.Advance(1.0)
	assert.True(t, f.c.Fighting(), "an aware enemy four tiles off opens a paced fight")

	// THE CONTROL: the same enemy at the same distance under the default dials,
	// where the engage radius is adjacency, is a chase and not a fight.
	g := newResolverFight(t, 1462)
	v := g.add(t, "e:1", 40, Profile{})
	v.x = 44

	g.c.Advance(1.0)
	assert.False(t, g.c.Fighting(), "at the defaults four tiles is out of reach and nothing opens")
}

func TestPacedPackWalksOnItsTurnThenStrikes(t *testing.T) {
	f, s := pacedFight(t, func(d *CombatDials) { d.ForcedBand = BandHit })

	w := f.add(t, "e:1", 40, Profile{})
	w.x = 44

	f.c.Advance(1.0)
	require.True(t, f.c.Fighting())

	untilAwaiting(t, f, s)
	require.True(t, f.c.Awaiting(), "the player goes first in an unsurprised round")
	require.Equal(t, 44.0, w.x, "nothing moved before the player's turn")

	before := f.health("p:1")

	require.NoError(t, f.c.Commit(CommitHold, ""))
	untilAwaiting(t, f, s)

	assert.Equal(t, []string{"e:1"}, s.orders, "the enemy was ordered to walk on its own turn")
	assert.Equal(t, 41.0, w.x, "three tiles west, ending beside the player")
	assert.Less(t, f.health("p:1"), before, "and having arrived, it struck")
	assert.Equal(t, 2, f.c.Round(), "the round closed and the next one is waiting on him")

	// THE CONTROL: with EnemyMoveTiles at zero nothing walks, nothing reaches
	// him, and his health does not move -- so the strike above was bought by
	// the walk and not by something else in the round.
	g, gs := pacedFight(t, func(d *CombatDials) {
		d.ForcedBand = BandHit
		d.EnemyMoveTiles = 0
	})

	v := g.add(t, "e:1", 40, Profile{})
	v.x = 44

	g.c.Advance(1.0)
	untilAwaiting(t, g, gs)

	gBefore := g.health("p:1")

	require.NoError(t, g.c.Commit(CommitHold, ""))
	untilAwaiting(t, g, gs)

	assert.Empty(t, gs.orders)
	assert.Equal(t, 44.0, v.x)
	assert.Equal(t, gBefore, g.health("p:1"))
}

func TestPacedFightWaitsForTheWalkToFinish(t *testing.T) {
	f, s := pacedFight(t, nil)
	s.frames = 30 // half a second of walking

	w := f.add(t, "e:1", 40, Profile{})
	w.x = 44

	f.c.Advance(1.0)
	untilAwaiting(t, f, s)

	before := f.health("p:1")

	require.NoError(t, f.c.Commit(CommitHold, ""))

	// The end-of-turn beat, then the order, then fewer frames than the walk.
	tick(f, s, 40)
	require.Equal(t, []string{"e:1"}, s.orders, "the walk was ordered")
	require.True(t, s.Moving("e:1"), "and is still under way")
	assert.Equal(t, before, f.health("p:1"), "nothing strikes while its walk is still playing")

	tick(f, s, 40)
	assert.Less(t, f.health("p:1"), before, "the strike lands once the walk has finished")
}

func TestPacedPackBlowsLandABeatApart(t *testing.T) {
	f, s := pacedFight(t, func(d *CombatDials) { d.ForcedBand = BandHit })

	pack := Profile{Group: "g:1", Row: "wolves", Speed: 5, DamageMin: 3, DamageMax: 3}
	f.add(t, "e:1", 40, pack)
	f.add(t, "e:2", 40, pack).y = 41

	f.c.Advance(1.0)
	untilAwaiting(t, f, s)

	require.NoError(t, f.c.Commit(CommitHold, ""))

	enemyBlows := func() int {
		n := 0

		for _, b := range f.c.Tactical().Blows {
			if b.Target == "p:1" {
				n++
			}
		}

		return n
	}

	// The end-of-turn beat is 0.45 s = 27 frames. One frame past it, exactly
	// one of the pair has struck.
	tick(f, s, 28)
	assert.Equal(t, 1, enemyBlows(), "the first blow, alone")

	tick(f, s, 28)
	assert.Equal(t, 2, enemyBlows(), "the second, a beat later")

	// THE CONTROL: the world-time path lands both in the one Advance that
	// closes the turn -- which is exactly what Josh saw and called not turn
	// based.
	g := humanFight(t, 1462, func(d *CombatDials) { d.ForcedBand = BandHit })
	g.add(t, "e:1", 40, pack)
	g.add(t, "e:2", 40, pack).y = 41
	g.open(t)
	g.round()
	require.True(t, g.c.Awaiting())
	require.NoError(t, g.c.Commit(CommitHold, ""))

	n := 0

	for _, b := range g.c.Tactical().Blows {
		if b.Target == "p:1" {
			n++
		}
	}

	assert.Equal(t, 2, n, "unpaced, both blows land in one call")
}

func TestPacedRoundsArePaidInWorldMinutes(t *testing.T) {
	f, s := pacedFight(t, nil)

	f.add(t, "e:1", 400, Profile{})

	f.c.Advance(1.0)
	untilAwaiting(t, f, s)

	assert.True(t, f.c.WorldHeld(), "the player's turn holds the world")
	assert.Zero(t, f.c.TakeRoundMinutes(), "nothing is owed before a round closes")

	require.NoError(t, f.c.Commit(CommitHold, ""))
	require.False(t, f.c.Awaiting())
	assert.True(t, f.c.WorldHeld(), "the packs' turn holds it too -- that is the difference from the seam")

	untilAwaiting(t, f, s)

	assert.InDelta(t, f.c.dials.RoundMinutes, f.c.TakeRoundMinutes(), 1e-9, "one closed round owes one round's minutes")
	assert.Zero(t, f.c.TakeRoundMinutes(), "and taking it pays it")

	// World time handed back to Advance during a paced fight resolves nothing:
	// the round is Tick's.
	round, health := f.c.Round(), f.health("p:1")

	require.NoError(t, f.c.Commit(CommitHold, ""))
	f.c.Advance(100)
	assert.Equal(t, round, f.c.Round(), "a hundred world minutes buy no rounds in a paced fight")

	// The round number alone CANNOT catch world time resolving a paced round:
	// measured by this test's own negative control, the world-time path reopens
	// the round, walks straight back to the player's slot and stops there, so
	// the number never moves. What it does do is throw away the packs' pending
	// turn and hand the player a fresh one. So assert the half that moves.
	assert.False(t, f.c.Awaiting(), "the packs' turn is still pending -- world time did not skip it")
	assert.Equal(t, health, f.health("p:1"), "and nothing struck him out of turn")

	// THE CONTROL: the same non-awaiting state with the tactical layer off does
	// not hold the world, which is what makes the two assertions above about
	// pacing rather than about fighting.
	g := humanFight(t, 1462, nil)
	g.add(t, "e:1", 400, Profile{})
	g.open(t)
	assert.False(t, g.c.WorldHeld(), "unpaced, an open fight with no turn waiting does not hold the world")
}

func TestPacedFightTakesTheChaseOffItsParticipants(t *testing.T) {
	f, _ := pacedFight(t, nil)
	f.chases.chasing["e:1"] = true

	w := f.add(t, "e:1", 40, Profile{})
	w.x = 44

	f.c.Advance(1.0)
	require.True(t, f.c.Fighting())

	assert.Contains(t, f.chases.released, "e:1", "a participant moves on its turn, not on a chase")
	assert.Equal(t, []string{"e:1", "p:1"}, f.c.stepper.(*fakeStepper).halted,
		"and the walk already in flight stops -- a released chase alone keeps walking")
	assert.True(t, f.c.Participates("e:1"))
	assert.True(t, f.c.Participates("p:1"))
	assert.False(t, f.c.Participates("e:9"))

	// THE CONTROL: unpaced, opening a fight releases nothing.
	g := newResolverFight(t, 1462)
	g.chases.chasing["e:1"] = true
	g.add(t, "e:1", 40, Profile{})
	g.open(t)
	assert.Empty(t, g.chases.released)
}

func TestPacedFightEndsWhenHeWalksOutOfRange(t *testing.T) {
	f, s := pacedFight(t, func(d *CombatDials) { d.EnemyMoveTiles = 0 })

	w := f.add(t, "e:1", 400, Profile{})
	w.x = 44

	f.c.Advance(1.0)
	untilAwaiting(t, f, s)

	// Inside the disengage radius: the round closes and the fight goes on.
	require.NoError(t, f.c.Commit(CommitHold, ""))
	untilAwaiting(t, f, s)
	require.True(t, f.c.Fighting(), "four tiles is inside disengage (8)")

	// He walks well clear and ends his turn.
	f.target.x = 30

	require.NoError(t, f.c.Commit(CommitHold, ""))
	untilAwaiting(t, f, s)

	assert.False(t, f.c.Fighting(), "nothing alive within eight tiles ends it")
	assert.Equal(t, "disengaged", f.c.HarnessState()["ended_reason"])
}

func TestPacedFightIsOffUnderThePolicy(t *testing.T) {
	// Paced on, but nobody at the controls: there is nothing to wait for, so
	// the world-time path runs and Tick does nothing.
	f := newResolverFight(t, 1462)
	f.set(t, "paced", true)
	f.add(t, "e:1", 400, Profile{})
	f.open(t)

	f.c.Tick(10)
	assert.Equal(t, 1, f.c.Round(), "Tick does not drive a policy fight")

	f.round()
	assert.Equal(t, 2, f.c.Round(), "world time still does")
	assert.False(t, f.c.WorldHeld())

	// ONE SWITCH: the engage radius is the tactical layer's too. Under the
	// policy an enemy four tiles off is a chase, whatever the dial says, so a
	// script on the world-time path measures the fight it always measured.
	g := newResolverFight(t, 1462)
	g.set(t, "paced", true)
	g.set(t, "engage_tiles", 5.0)

	w := g.add(t, "e:1", 40, Profile{})
	w.x = 44

	g.c.Advance(1.0)
	assert.False(t, g.c.Fighting(), "under the policy, engage is adjacency")
}

func TestTacticalViewReportsTheTurn(t *testing.T) {
	f, s := pacedFight(t, func(d *CombatDials) { d.ForcedBand = BandHit })

	f.add(t, "e:1", 400, Profile{})

	v := f.c.Tactical()
	assert.False(t, v.Fighting)
	assert.Equal(t, "", v.Phase)

	f.c.Advance(1.0)
	untilAwaiting(t, f, s)

	v = f.c.Tactical()
	assert.True(t, v.Fighting)
	assert.Equal(t, "player", v.Phase)
	assert.Equal(t, "p:1", v.PlayerID)
	assert.Equal(t, 2, v.MoveTiles)
	require.Len(t, v.Enemies, 1)
	assert.True(t, v.Enemies[0].Adjacent)

	require.NoError(t, f.c.Commit(CommitStrike, "e:1"))

	v = f.c.Tactical()
	assert.True(t, v.ActionSpent)
	assert.False(t, v.MoveSpent)
	require.NotEmpty(t, v.Blows)
	assert.Equal(t, "p:1", v.Blows[len(v.Blows)-1].Attacker, "his blow is in the log")

	require.NoError(t, f.c.Commit(CommitEnd, ""))
	assert.Equal(t, "enemy", f.c.Tactical().Phase)
}

func TestTacticalSettableDialsRoundTrip(t *testing.T) {
	f := newResolverFight(t, 1462)

	for field, want := range map[string]float64{
		"engage_tiles": 6, "disengage_tiles": 9, "enemy_move_tiles": 4, "move_tiles": 3,
	} {
		f.set(t, field, want)
		assert.Equal(t, want, float64(f.c.HarnessState()[field].(int)), field)
	}

	f.set(t, "paced", true)
	assert.Equal(t, true, f.c.HarnessState()["paced"])

	require.Error(t, f.c.HarnessSet("engage_tiles", -1.0))
	require.Error(t, f.c.HarnessSet("paced", "yes"))
}

// THE ROUND THAT ENDS A FIGHT IS STILL PAID FOR (review finding, 23 Sep).
// closePacedRound is not reached when his blow kills the last enemy, so
// without payOpenRound the decisive round cost no world time at all.
func TestPacedKillingRoundIsCharged(t *testing.T) {
	f, s := pacedFight(t, func(d *CombatDials) { d.ForcedBand = BandCrit })

	f.add(t, "e:1", 1, Profile{}) // one blow kills it

	f.c.Advance(1.0)
	untilAwaiting(t, f, s)
	require.Zero(t, f.c.TakeRoundMinutes())

	require.NoError(t, f.c.Commit(CommitStrike, "e:1"))
	require.False(t, f.c.Fighting(), "the blow ended the fight")

	assert.InDelta(t, f.c.dials.RoundMinutes, f.c.TakeRoundMinutes(), 1e-9,
		"the round the fight ended in owes its minute")

	// THE CONTROL: unpaced, the same fight owes nothing through this path --
	// world time paced its rounds as they ran.
	g := humanFight(t, 1462, func(d *CombatDials) { d.ForcedBand = BandCrit })
	g.add(t, "e:1", 1, Profile{})
	g.open(t)
	g.round()
	require.True(t, g.c.Awaiting())
	require.NoError(t, g.c.Commit(CommitStrike, "e:1"))
	assert.Zero(t, g.c.TakeRoundMinutes())
}

// ONE SWITCH, the disengage half (review finding): under the policy an enemy
// that steps two tiles off leaves the fight, as it always did.
func TestDisengageIsAdjacencyUnderThePolicy(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.set(t, "paced", true)
	f.set(t, "disengage_tiles", 8.0)

	w := f.add(t, "e:1", 400, Profile{})
	f.open(t)

	w.x = 43 // three tiles off: inside 8, outside adjacency
	f.round()

	assert.False(t, f.c.Fighting(), "under the policy the fight ends at adjacency")
	assert.Equal(t, "disengaged", f.c.HarnessState()["ended_reason"])
}

// A walk that never settles cannot stall the fight: the player's own walk is
// halted at the timeout, as a pack's is.
func TestPacedPlayerWalkTimesOut(t *testing.T) {
	f, s := pacedFight(t, nil)

	f.add(t, "e:1", 400, Profile{})
	f.c.Advance(1.0)
	untilAwaiting(t, f, s)

	require.NoError(t, f.c.Commit(CommitHold, ""))

	s.pending["p:1"] = 1 << 30 // a walk that will never finish on its own

	untilAwaiting(t, f, s)
	assert.Contains(t, s.halted, "p:1", "the stuck walk was stopped")
	assert.True(t, f.c.Awaiting(), "and the round went on to his next turn")
}
