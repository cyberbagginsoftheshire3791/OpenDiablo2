package d2gamescreen

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// The seam only exists for a real player if the SHIPPED screen asks for it.
//
// M4.4c-2a ask 5 ruled the split: DefaultCombatDials keeps "policy" so the
// resolver's own unit tests stand, and the game screen sets "human". The second
// half was written a day late, and nothing in the build could see the gap --
// the keys still call Combat.Commit from OnKeyDown, so `deadcode` and every
// reachability row stayed green while F/L/E answered "refused: no turn is
// waiting" on every press in a real launch.
//
// This test is the mechanical check that closes that hole, and it pins BOTH
// halves: flipping the default instead of the screen reddens the second
// assertion, and losing the screen's line reddens the first.
func TestTheShippedScreenTakesTheTurn(t *testing.T) {
	t.Parallel()

	if got, want := shippedCombatDials().PlayerControl, d2world.PlayerControlHuman; got != want {
		t.Fatalf("a real launch must stop the round at the player: PlayerControl = %q, want %q", got, want)
	}

	if got, want := d2world.DefaultCombatDials().PlayerControl, d2world.PlayerControlPolicy; got != want {
		t.Fatalf("DefaultCombatDials must keep policy, or the screen's line is untested and ~40 resolver unit tests have quietly changed meaning: PlayerControl = %q, want %q", got, want)
	}
}

// And the flip must be the ONLY thing the screen changes: if a dial drifts
// between the signed defaults and what ships, the numbers every playtest
// asserts against stop describing the game Josh launches.
//
// T1 (23 Sep 2026) widens "the flip" to the tactical layer's four dials, and
// NAMES them here rather than loosening the comparison: the screen changes
// exactly the human flip and the tactical layer, and nothing else.
func TestTheShippedDialsChangeNothingElse(t *testing.T) {
	t.Parallel()

	want := d2world.DefaultCombatDials()
	want.PlayerControl = d2world.PlayerControlHuman
	want.Paced = true
	want.EngageTiles = d2world.TacticalEngageTiles
	want.DisengageTiles = d2world.TacticalDisengageTiles
	want.EnemyMoveTiles = d2world.TacticalEnemyMoveTiles

	if got := shippedCombatDials(); got != want {
		t.Fatalf("the shipped dials must be the signed defaults plus the human flip and the tactical layer, nothing else:\n got  %+v\n want %+v", got, want)
	}
}

// The tactical layer ships ON, and it stays OFF in the defaults so the
// resolver's unit tests keep describing the world-time path. Both halves,
// for the same reason TestTheShippedScreenTakesTheTurn pins both of its own.
func TestTheShippedScreenIsPaced(t *testing.T) {
	t.Parallel()

	if !shippedCombatDials().Paced {
		t.Fatal("a real launch must run the tactical layer: Paced = false")
	}

	if d2world.DefaultCombatDials().Paced {
		t.Fatal("DefaultCombatDials must keep Paced off")
	}

	if got := shippedCombatDials().EngageTiles; got <= shippedCombatDials().AdjacentTiles {
		t.Fatalf("a shipped fight must open before the enemy is adjacent: EngageTiles = %d", got)
	}
}
