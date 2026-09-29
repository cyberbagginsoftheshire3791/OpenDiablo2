package d2rand

import "testing"

// REDERIVE NAMES THE SIBLING STREAM (the raid's R1, 29 Sep 2026): from any
// stream's derived seed, the seed Derive gives any other stream of the same
// game -- over every edge seed Effective has a branch for. The combat model
// seeds its clock-driven fights this way from the one seed NewCombat is
// handed, so a wrong Rederive would put them on a stream no save could check.
func TestRederiveNamesTheSiblingStream(t *testing.T) {
	for _, g := range deriveSeeds {
		for _, from := range Derived {
			for _, to := range Derived {
				if got, want := Rederive(Derive(g, from), from, to), Derive(g, to); got != want {
					t.Fatalf("game seed %d: Rederive(Derive(g, %q), %q, %q) = %d, Derive(g, %q) = %d",
						g, from, from, to, got, to, want)
				}
			}
		}
	}

	// The control: from the wrong stream's seed, it names another game's.
	if Rederive(Derive(1462, StreamSpawns), StreamCombat, StreamCombatClock) == Derive(1462, StreamCombatClock) {
		t.Fatal("control: Rederive from a seed that is not the named stream's must not land on the sibling")
	}
}

// The clock fights' stream is pinned like the others (TestDeriveIsFixed): it
// seeds every fight the village has without him, so a change to it moves
// every measured clock fight and must be a decision. 179919329 is combat's
// pinned seed at 1462, so the Rederive the combat model makes is pinned too.
func TestDeriveCombatClockIsFixed(t *testing.T) {
	const want = 1070640403

	if got := Derive(1462, StreamCombatClock); got != want {
		t.Fatalf("Derive(1462, %q) = %d, pinned %d", StreamCombatClock, got, want)
	}

	if got := Rederive(179919329, StreamCombat, StreamCombatClock); got != want {
		t.Fatalf("Rederive(combat's 1462 seed) = %d, pinned %d", got, want)
	}
}
