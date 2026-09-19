package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// M4.4c-2a, the seam: a round that STOPS at the player's slot and waits.
//
// The contract these pin is the one the 17 September A1 reconciliation
// settled, and it is the opposite of what the brief's §4.2 said before it:
// EXECUTING THE ACTION IS NOT FINISHING THE TURN. A commit resolves the
// player's slot; the turn closes only when AutoEndTurn's both-spent condition
// is met, or when the player says so with hold or end.
//
// Every assertion here has its pair, because a seam that always waits and a
// seam that never waits both pass a one-sided test:
//
//   - waits at the player's slot          <-> the policy walks straight through
//   - a strike leaves the turn open       <-> end closes it
//   - both spent auto-ends                <-> AutoEndTurn false keeps it open
//   - hold closes an unspent turn         <-> end is refused on one
//   - end closes a spent turn             <-> hold is refused on one

// humanFight is newResolverFight with the one dial that matters flipped. The
// dials are set at construction because PlayerControl is read inside the walk,
// and a fight opened under the policy has already resolved its round.
func humanFight(t *testing.T, seed int64, tune func(*CombatDials)) *resolverFight {
	t.Helper()

	f := newResolverFight(t, seed)

	dials := DefaultCombatDials()
	dials.PlayerControl = PlayerControlHuman

	if tune != nil {
		tune(&dials)
	}

	f.c.Close()
	f.c = NewCombat(NewClock(DefaultClockDials()), f.notice, f.fitness, f.illum, f.bodies,
		f.profiles, f.animator, f.morale, f.chases, seed, dials)

	t.Cleanup(f.c.Close)

	return f
}

// playerBlows counts rows in the round's log that the PLAYER struck.
func playerBlows(t *testing.T, f *resolverFight) int {
	t.Helper()

	n := 0

	for _, row := range f.actions(t) {
		if row["attacker"] == "p:1" {
			n++
		}
	}

	return n
}

func TestSeamHumanRoundWaitsAtThePlayerSlot(t *testing.T) {
	f := humanFight(t, 1462, nil)
	f.add(t, "e:1", 40, Profile{})
	f.open(t)

	require.False(t, f.c.Awaiting(), "the fight opens without a turn waiting -- tryStart resolves no round")

	f.round()

	assert.True(t, f.c.Awaiting(), "the round must stop at the player's slot")
	assert.Equal(t, 1, f.c.Round(), "a half-resolved round does not advance the round number")
	assert.Zero(t, playerBlows(t, f), "the policy must not have struck for him")

	// THE CONTROL. The same fight under the policy walks straight through and
	// the round closes, which is what makes the assertion above mean
	// something rather than measuring a fight that never starts.
	p := newResolverFight(t, 1462)
	p.add(t, "e:1", 40, Profile{})
	p.open(t)
	p.round()

	assert.False(t, p.c.Awaiting(), "the policy never waits")
	assert.Equal(t, 2, p.c.Round(), "and its round closes")
}

func TestSeamAwaitingFreezesTheEncounter(t *testing.T) {
	f := humanFight(t, 1462, nil)
	f.add(t, "e:1", 40, Profile{})
	f.open(t)
	f.round()
	require.True(t, f.c.Awaiting())

	round, health := f.c.Round(), f.health("p:1")

	// A hundred world minutes is a hundred rounds' worth. None of them buy
	// anything: the encounter is frozen mid-round by construction, and this is
	// the second lock behind Game.worldRunning()'s gate.
	f.c.Advance(100)

	assert.True(t, f.c.Awaiting(), "still waiting")
	assert.Equal(t, round, f.c.Round(), "no round resolved")
	assert.Equal(t, health, f.health("p:1"), "and nothing struck him")

	// AND THE BANKED TIME, which is the half only the early return prevents.
	//
	// The three assertions above pass with the guard REMOVED -- measured, when
	// this test's own negative control failed to go red -- because the loop
	// condition tests awaiting as well. What the guard ALONE stops is
	// sinceTurn accumulating behind an open turn. Without it a player who
	// thinks for a hundred world minutes leaves the encounter owing a hundred
	// minutes of rounds it never lived through, and they come due the moment
	// the world runs again: the fight catching up for having been made to wait.
	//
	// It is asserted on the field rather than through a round count on purpose.
	// Under human control the NEXT round stops at the player's slot too, so a
	// banked hundred minutes is invisible from outside until control changes
	// hands -- which is exactly the kind of assertion that cannot fail.
	assert.InDelta(t, 0, f.c.encounter.sinceTurn, 1e-9,
		"a hundred world minutes must not bank up behind an open turn")
}

func TestSeamStrikeLeavesTheTurnOpenAndEndClosesIt(t *testing.T) {
	f := humanFight(t, 1462, nil)
	f.add(t, "e:1", 40, Profile{})
	f.open(t)
	f.round()
	require.True(t, f.c.Awaiting())

	require.NoError(t, f.c.Commit(CommitStrike, ""))

	// THE RECONCILED CONTRACT. The blow landed and the turn is STILL open,
	// because the Move is unspent. Before 17 September the brief said this
	// closed the round, and act 2 asserted it -- while act 3, four lines
	// below, asserted the opposite on the same rule.
	assert.Equal(t, 1, playerBlows(t, f), "the strike resolved through resolveBlow")
	assert.True(t, f.c.Awaiting(), "the Action is spent and the Move is not: the turn WAITS")
	assert.Equal(t, 1, f.c.Round(), "and the round has not closed")

	require.NoError(t, f.c.Commit(CommitEnd, ""))

	assert.False(t, f.c.Awaiting(), "end closes the turn")
	assert.Equal(t, 2, f.c.Round(), "the round closed and the next one is running")
}

func TestSeamBothSpentAutoEndsWithoutAnEndKey(t *testing.T) {
	// THE MECHANISM THE RECONCILIATION NEARLY MISSED. Every sentence in the
	// design says a Move click can close a both-spent turn; a turn-over check
	// that lives only inside Commit cannot deliver it, because a Move is not a
	// commit. maybeEndTurn runs wherever a pip is spent, so finishRound has
	// THREE entry points: Advance, Commit, and the Move that spends the last.
	strikeThenMove := humanFight(t, 1462, nil)
	strikeThenMove.add(t, "e:1", 40, Profile{})
	strikeThenMove.open(t)
	strikeThenMove.round()

	require.NoError(t, strikeThenMove.c.Commit(CommitStrike, ""))
	require.True(t, strikeThenMove.c.Awaiting(), "still open on one pip")

	strikeThenMove.c.SpendMove()

	assert.False(t, strikeThenMove.c.Awaiting(), "both spent: the turn ends itself")
	assert.Equal(t, 2, strikeThenMove.c.Round(), "and the round closed with no end key")

	// The other order is the same turn. R2 gives no ordering, and
	// strike-then-step-back is a legal round.
	moveThenStrike := humanFight(t, 1462, nil)
	moveThenStrike.add(t, "e:1", 40, Profile{})
	moveThenStrike.open(t)
	moveThenStrike.round()

	moveThenStrike.c.SpendMove()
	require.True(t, moveThenStrike.c.Awaiting(), "a Move alone does not end a turn")

	require.NoError(t, moveThenStrike.c.Commit(CommitStrike, ""))

	assert.False(t, moveThenStrike.c.Awaiting(), "both spent, either order")
	assert.Equal(t, 2, moveThenStrike.c.Round())
}

func TestSeamAutoEndTurnFalseKeepsABothSpentTurnOpen(t *testing.T) {
	// The control for the test above: with the dial off, both pips spent is
	// not enough and only an explicit end closes the turn. Without this, a
	// seam that closed on ANY commit would pass the auto-end test.
	f := humanFight(t, 1462, func(d *CombatDials) { d.AutoEndTurn = false })
	f.add(t, "e:1", 40, Profile{})
	f.open(t)
	f.round()

	require.NoError(t, f.c.Commit(CommitStrike, ""))
	f.c.SpendMove()

	assert.True(t, f.c.Awaiting(), "AutoEndTurn false: both spent still waits")
	assert.Equal(t, 1, f.c.Round())

	require.NoError(t, f.c.Commit(CommitEnd, ""))
	assert.False(t, f.c.Awaiting(), "and end still closes it")
}

func TestSeamHoldAndEndAreTheTwoHalvesOfOneKey(t *testing.T) {
	// hold is SIGNED as "end the turn with the Action unspent". end closes a
	// turn whose Action is already spent -- the gap that had no verb until
	// Josh ruled the fifth choice on 17 September. E sends whichever fits, and
	// they are checked rather than aliased so a game screen sending the wrong
	// one is loud instead of silently fine.
	hold := humanFight(t, 1462, nil)
	hold.add(t, "e:1", 40, Profile{})
	hold.open(t)
	hold.round()

	require.NoError(t, hold.c.Commit(CommitHold, ""))

	assert.False(t, hold.c.Awaiting(), "hold closes an unspent turn")
	assert.Zero(t, playerBlows(t, hold), "and spends no Action doing it")
	assert.Equal(t, 2, hold.c.Round())

	// end on an unspent turn is the wrong half, and is refused.
	wrongEnd := humanFight(t, 1462, nil)
	wrongEnd.add(t, "e:1", 40, Profile{})
	wrongEnd.open(t)
	wrongEnd.round()

	require.Error(t, wrongEnd.c.Commit(CommitEnd, ""), "end wants a spent Action")
	assert.True(t, wrongEnd.c.Awaiting(), "and the turn is untouched")
	assert.Equal(t, 1, wrongEnd.c.CommitsRefused())

	// hold on a spent turn is the other wrong half.
	wrongHold := humanFight(t, 1462, nil)
	wrongHold.add(t, "e:1", 40, Profile{})
	wrongHold.open(t)
	wrongHold.round()

	require.NoError(t, wrongHold.c.Commit(CommitStrike, ""))
	require.Error(t, wrongHold.c.Commit(CommitHold, ""), "hold wants an unspent Action")
	assert.True(t, wrongHold.c.Awaiting(), "and the turn is untouched")
	assert.Equal(t, 1, wrongHold.c.CommitsRefused())
}

func TestSeamCommitIsRefusedWithNoTurnWaiting(t *testing.T) {
	f := humanFight(t, 1462, nil)
	f.add(t, "e:1", 40, Profile{})
	f.open(t)

	// No round has resolved, so no turn is open.
	require.Error(t, f.c.Commit(CommitStrike, ""), "a commit with no turn waiting is refused")
	assert.Equal(t, 1, f.c.CommitsRefused())
	assert.Zero(t, playerBlows(t, f), "and resolves nothing")

	f.round()
	require.True(t, f.c.Awaiting())

	// A second Action in one turn is refused too.
	require.NoError(t, f.c.Commit(CommitStrike, ""))
	require.Error(t, f.c.Commit(CommitLight, ""), "the Action is already spent")
	assert.Equal(t, 2, f.c.CommitsRefused())
	assert.Equal(t, 1, playerBlows(t, f), "still one blow")

	// And an unknown choice names the five.
	require.Error(t, f.c.Commit("shout", ""))
	assert.Equal(t, 3, f.c.CommitsRefused())
}

func TestSeamTheTorchSpendsTheActionAndNoBlow(t *testing.T) {
	// light and douse resolve in the light model, which lives on the game
	// screen. This package's half of the verb is the Action it spends -- and
	// it must NOT produce a blow, which is act 3's assertion.
	f := humanFight(t, 1462, nil)
	f.add(t, "e:1", 40, Profile{})
	f.open(t)
	f.round()

	require.NoError(t, f.c.Commit(CommitLight, ""))

	assert.Zero(t, playerBlows(t, f), "the torch is not a blow")
	assert.True(t, f.c.Awaiting(), "and the turn is still open on the unspent Move")

	f.c.SpendMove()
	assert.False(t, f.c.Awaiting(), "both spent closes it")
}
