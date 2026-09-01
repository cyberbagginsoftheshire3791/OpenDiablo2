package d2world

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeFitness stands in for the meters. M4.2 computes ReactionAvailable and
// Shaken and this milestone READS them, which was signed as that milestone's
// ask 1 -- so the combat model takes a two-method interface and can be tested
// with no meters, no body and no clock.
type fakeFitness struct {
	reaction bool
	shaken   bool
}

func (f *fakeFitness) ReactionAvailable() bool { return f.reaction }
func (f *fakeFitness) Shaken() bool            { return f.shaken }

// fakeBodies stands in for the game screen's body registry (M4.5 step 3).
//
// BodyOf returns an UNTYPED nil for an id it does not know, which is the
// contract the Bodies doc comment states and not a detail of the fake: a nil
// *T returned into an interface is not nil, so a lookup that got this wrong
// would report has_body:true for a body that is not there and then panic
// dereferencing it.
type fakeBodies struct {
	known map[string]*fakeBody
}

func (f *fakeBodies) BodiesKnown() int { return len(f.known) }

func (f *fakeBodies) BodyOf(id string) Body {
	if b, ok := f.known[id]; ok && b != nil {
		return b
	}

	return nil
}

type fakeBody struct {
	health    int
	maxHealth int
}

func (b *fakeBody) CurrentHealth() int { return b.health }
func (b *fakeBody) MaxHealth() int     { return b.maxHealth }
func (b *fakeBody) SetHealth(h int)    { b.health = h }

// newTestCombat wires a Notice with a clear line of sight, so a watcher's
// awareness is a function of distance alone and the tests can put a thing in
// reach or out of it by moving it.
func newTestCombat(t *testing.T) (*Combat, *Notice, *fakeQuarry, *fakeFitness) {
	t.Helper()

	// Nil bodies, deliberately: most of these tests are about whether a fight
	// happens, and a model that needs a body registry to decide that would
	// have the fence in the wrong place.
	return newTestCombatWith(t, nil)
}

// newTestCombatWith is newTestCombat plus a body lookup, for the tests that
// are about what the provider reports rather than about when a fight starts.
func newTestCombatWith(t *testing.T, bodies Bodies) (*Combat, *Notice, *fakeQuarry, *fakeFitness) {
	t.Helper()

	clock := NewClock(DefaultClockDials())
	notice := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	fitness := &fakeFitness{reaction: true}
	target := &fakeQuarry{id: "p:1", x: 40, y: 40}

	c := NewCombat(clock, notice, fitness, &fakeIllumination{}, bodies, 1462, DefaultCombatDials())

	t.Cleanup(c.Close)

	return c, notice, target, fitness
}

// aware puts a watcher on the notice model and steps it until it has noticed.
func aware(t *testing.T, n *Notice, w *fakeWatcher, q *fakeQuarry) {
	t.Helper()

	n.Watch(w, q)

	for i := 0; i < 20; i++ {
		if noticed, watching := n.Noticed(w.WatcherID()); watching && noticed {
			return
		}

		n.Advance(1.0)
	}

	t.Fatalf("watcher %s never noticed the target", w.WatcherID())
}

// AWARENESS ALONE IS NOT A FIGHT. This is the distinction the milestone is
// built on: M4.3b decided that a thing has SEEN you, M4.3a closes the
// distance, and both were deliberately silent about the last two tiles.
func TestCombatDoesNotStartOnAwarenessAlone(t *testing.T) {
	c, notice, target, _ := newTestCombat(t)

	// Six tiles away: well inside the 12-tile notice radius, nowhere near
	// reach.
	wolf := &fakeWatcher{id: "w:1", x: 46, y: 40}
	aware(t, notice, wolf, target)

	c.Advance(1.0)

	require.False(t, c.Fighting(), "a pack that has noticed you from six tiles away is a chase, not an encounter")
	require.Zero(t, c.Round())

	state := c.HarnessState()
	require.Equal(t, false, state["fighting"])
	require.Positive(t, state["declined_reach"], "the near miss should be counted, not silently dropped")
}

func TestCombatStartsWhenAnAwareThingIsInReach(t *testing.T) {
	c, notice, target, _ := newTestCombat(t)

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40} // one tile east
	aware(t, notice, wolf, target)

	c.Advance(1.0)

	require.True(t, c.Fighting(), "an aware thing standing next to you is a fight")
	require.Equal(t, 1, c.Round(), "an encounter opens on round 1")
	require.Equal(t, []string{"w:1"}, c.Order())
}

// A diagonal neighbour is as adjacent as an orthogonal one: the reach test is
// Chebyshev on floored tiles, not Euclidean, because R2's zone-of-control is
// stated in adjacency terms and the map is a tile grid.
func TestCombatReachIsChebyshevNotEuclidean(t *testing.T) {
	c, notice, target, _ := newTestCombat(t)

	// (41,41) is 1.41 tiles away by Euclid -- outside Pursuit's ArriveWithin
	// of 1.0 -- and one tile away on the grid.
	wolf := &fakeWatcher{id: "w:1", x: 41, y: 41}
	aware(t, notice, wolf, target)

	c.Advance(1.0)

	require.True(t, c.Fighting(),
		"a diagonal neighbour is in reach; if this fails the test is Euclidean and the dial is being read as a distance")
}

// Rounds consume world time (R2 section 2A). A script that steps an hour
// should see an hour's worth of rounds rather than one.
func TestCombatRoundsConsumeWorldTime(t *testing.T) {
	c, notice, target, _ := newTestCombat(t)

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40}
	aware(t, notice, wolf, target)

	c.Advance(1.0)
	require.Equal(t, 1, c.Round())

	// Half a round buys nothing.
	c.Advance(0.5)
	require.Equal(t, 1, c.Round(), "half a round is not a round")

	// The other half completes it.
	c.Advance(0.5)
	require.Equal(t, 2, c.Round())

	// Ten world minutes at one minute a round.
	c.Advance(10.0)
	require.Equal(t, 12, c.Round(), "ten world minutes should buy ten rounds, not one")
}

// Disengagement is real and supported (R2 section 3). Walking out of reach is
// the whole of it in v0.
func TestCombatEndsWhenNothingIsInReach(t *testing.T) {
	c, notice, target, _ := newTestCombat(t)

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40}
	aware(t, notice, wolf, target)

	c.Advance(1.0)
	require.True(t, c.Fighting())

	// The player walks away.
	target.x, target.y = 60, 60

	c.Advance(1.0)

	require.False(t, c.Fighting(), "nothing in reach is the end of the encounter")
	require.Equal(t, 1, c.HarnessState()["ended"])
}

// A pack fights as a pack, and an id that leaves the fight leaves the reported
// order with it -- otherwise a script reading `order` sees a participant that
// is no longer there.
func TestCombatOrderTracksTheLivingParticipants(t *testing.T) {
	c, notice, target, _ := newTestCombat(t)

	near := &fakeWatcher{id: "w:1", x: 41, y: 40}
	alsoNear := &fakeWatcher{id: "w:2", x: 39, y: 40}

	aware(t, notice, near, target)
	aware(t, notice, alsoNear, target)

	c.Advance(1.0)

	require.True(t, c.Fighting())
	require.ElementsMatch(t, []string{"w:1", "w:2"}, c.Order(), "both packs are in the fight")

	// One backs off.
	alsoNear.x = 50

	c.Advance(1.0)

	require.Equal(t, []string{"w:1"}, c.Order(), "the one that left must leave the order too")
	require.True(t, c.Fighting(), "the other is still there")
}

// R2 section 3's determinism clause applied to the one part of this milestone
// that is arbitrary by design: the provisional order is seeded, so two runs of
// one build at one seed agree.
func TestCombatProvisionalOrderIsDeterministic(t *testing.T) {
	orderFor := func() []string {
		clock := NewClock(DefaultClockDials())
		notice := NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
		target := &fakeQuarry{id: "p:1", x: 40, y: 40}
		c := NewCombat(clock, notice, &fakeFitness{}, &fakeIllumination{}, nil, 1462, DefaultCombatDials())

		defer c.Close()

		for _, id := range []string{"w:1", "w:2", "w:3", "w:4"} {
			w := &fakeWatcher{id: id, x: 41, y: 40}
			notice.Watch(w, target)
		}

		for i := 0; i < 20; i++ {
			notice.Advance(1.0)
		}

		c.Advance(1.0)

		return c.Order()
	}

	first := orderFor()
	second := orderFor()

	require.Len(t, first, 4)
	require.Equal(t, first, second, "one build at one seed must produce one order")
}

func TestCombatProviderReportsTheFight(t *testing.T) {
	c, notice, target, fitness := newTestCombat(t)

	fitness.reaction = false
	fitness.shaken = true

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40}
	aware(t, notice, wolf, target)

	c.Advance(1.0)

	state := c.HarnessState()
	require.Equal(t, true, state["fighting"])
	require.Equal(t, "e:1", state["encounter"])
	require.Equal(t, 1, state["round"])
	require.Equal(t, 1.0, state["round_minutes"])

	parts, ok := state["participants"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, parts, 2, "the player and the wolf")

	// The player side carries the two facts M4.2 built for this milestone to
	// read. Reporting them HERE as well as on the meters provider is the
	// point: an assertion about a fight should be readable from the fight.
	require.Equal(t, "player", parts[0]["side"])
	require.Equal(t, false, parts[0]["reaction_available"])
	require.Equal(t, true, parts[0]["shaken"])
	require.Contains(t, parts[0], "light_here")

	require.Equal(t, "enemy", parts[1]["side"])
	require.Equal(t, "w:1", parts[1]["id"])
	require.Equal(t, true, parts[1]["adjacent"])
}

func TestCombatSettableFields(t *testing.T) {
	c, notice, target, _ := newTestCombat(t)

	require.Equal(t, []string{"adjacent_tiles", "disengage", "round", "round_minutes"}, c.HarnessSettableFields())

	// Setting a value on nothing is an error rather than a silent no-op.
	require.Error(t, c.HarnessSet("round", 5.0), "there is no encounter to set a round on")
	require.Error(t, c.HarnessSet("disengage", true))
	require.Error(t, c.HarnessSet("no_such_field", 1.0))

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40}
	aware(t, notice, wolf, target)
	c.Advance(1.0)

	// A value a script can only ever watch rise is a value it cannot assert
	// against cheaply -- the third provider rule.
	require.NoError(t, c.HarnessSet("round", 20.0))
	require.Equal(t, 20, c.Round())

	require.NoError(t, c.HarnessSet("disengage", true))
	require.False(t, c.Fighting(), "the emptying half of the first provider rule")

	require.Error(t, c.HarnessSet("round_minutes", -1.0), "a round cannot cost negative time")
	require.NoError(t, c.HarnessSet("round_minutes", 2.0))
	require.Equal(t, 2.0, c.HarnessState()["round_minutes"])
}

// The reach dial has to be able to move, or a script cannot tell a reach
// failure from a design that never engages.
func TestCombatAdjacentTilesIsADial(t *testing.T) {
	c, notice, target, _ := newTestCombat(t)

	wolf := &fakeWatcher{id: "w:1", x: 44, y: 40} // four tiles east: out of reach
	aware(t, notice, wolf, target)

	c.Advance(1.0)
	require.False(t, c.Fighting())

	require.NoError(t, c.HarnessSet("adjacent_tiles", 5.0))

	c.Advance(1.0)
	require.True(t, c.Fighting(), "widening the reach should bring the same wolf into the fight")
}

// THE ENEMY HAS A BODY, AND A SCRIPT CAN SEE IT (M4.5 step 3).
//
// This is the milestone's own assertion at the model level: before step 3 an
// NPC had no health at all, and the provider could report that a fight was
// happening without being able to say what was in it.
func TestCombatReportsAnEnemyBody(t *testing.T) {
	bodies := &fakeBodies{known: map[string]*fakeBody{
		"w:1": {health: 181, maxHealth: 181},
	}}

	c, notice, target, _ := newTestCombatWith(t, bodies)

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40}
	aware(t, notice, wolf, target)

	c.Advance(1.0)

	state := c.HarnessState()
	require.Equal(t, true, state["has_bodies"])

	parts, ok := state["participants"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, parts, 2)

	require.Equal(t, "enemy", parts[1]["side"])
	require.Equal(t, true, parts[1]["has_body"])
	require.Equal(t, 181, parts[1]["health"])
	require.Equal(t, 181, parts[1]["max_health"])

	// The PLAYER's body is deliberately not reported here. It is already
	// meters.health / max_health, and Fitness was fenced to two booleans on
	// purpose; duplicating it would give one truth two homes, which is the
	// disease this project keeps treating.
	require.NotContains(t, parts[0], "health")
}

// A MONSTER WITH NO BODY READS AS "NO BODY", NOT AS ZERO HEALTH.
//
// The harness and the debug terminal can both put an NPC on the map without
// the game screen adopting it, so this case is real rather than defensive.
// A3's lesson in one sentence: a missing value that reads as a legal value is
// an assertion that passes while measuring nothing -- health:0 would satisfy
// "the wolf is nearly dead" for a wolf nobody ever gave a body to.
func TestCombatReportsNoBodyRatherThanZeroHealth(t *testing.T) {
	// Knows a different monster, so the lookup works and this id is genuinely
	// absent rather than the whole registry being empty.
	bodies := &fakeBodies{known: map[string]*fakeBody{
		"someone-else": {health: 61, maxHealth: 61},
	}}

	c, notice, target, _ := newTestCombatWith(t, bodies)

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40}
	aware(t, notice, wolf, target)

	c.Advance(1.0)

	parts, ok := c.HarnessState()["participants"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, parts, 2)

	require.Equal(t, false, parts[1]["has_body"], "the registry does not know this id")
	require.NotContains(t, parts[1], "health", "absent, not zero")
	require.NotContains(t, parts[1], "max_health")
}

// The model still works with no body registry at all, which is what every
// unit test above it relies on and what a Combat built before the game screen
// exists would see.
func TestCombatWithoutABodyRegistryStillReportsTheFight(t *testing.T) {
	c, notice, target, _ := newTestCombat(t)

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40}
	aware(t, notice, wolf, target)

	c.Advance(1.0)

	state := c.HarnessState()
	require.Equal(t, true, state["fighting"])
	require.Equal(t, false, state["has_bodies"])

	parts, ok := state["participants"].([]map[string]interface{})
	require.True(t, ok)
	require.Equal(t, false, parts[1]["has_body"])
}

// A LOOKUP THAT RETURNS A TYPED NIL WOULD PANIC, AND THIS IS THE CONTROL FOR
// IT. Go's nil-interface trap: a (*T)(nil) returned as an interface is not
// nil, so "if body != nil" passes and the next call dereferences nothing.
// The Bodies doc comment requires an untyped nil; this proves the model
// believes a lookup that honours it, and d2gamescreen's own test proves the
// real implementation does.
func TestCombatBelievesAnUntypedNilFromTheLookup(t *testing.T) {
	bodies := &fakeBodies{known: map[string]*fakeBody{"w:1": nil}}

	// The fake maps a known id to a nil body, which its BodyOf must convert
	// into an untyped nil rather than passing along.
	require.Nil(t, bodies.BodyOf("w:1"))
	require.True(t, bodies.BodyOf("w:1") == nil, "must be an UNTYPED nil")

	c, notice, target, _ := newTestCombatWith(t, bodies)

	wolf := &fakeWatcher{id: "w:1", x: 41, y: 40}
	aware(t, notice, wolf, target)

	c.Advance(1.0)

	parts, ok := c.HarnessState()["participants"].([]map[string]interface{})
	require.True(t, ok)
	require.Equal(t, false, parts[1]["has_body"])
}
