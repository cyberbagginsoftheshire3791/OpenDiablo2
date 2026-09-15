package d2gamescreen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// npcBody is arithmetic and nothing else, so these are short. What they are
// really defending is the two decisions in it: full health at adoption, and a
// floor of 1 so that a body is never born dead.
func TestNPCBodyStartsFullAndIsWritable(t *testing.T) {
	b := newNPCBody(181) // zombie1's MaxHPNormal, measured by tools/animcensus

	require.Equal(t, 181, b.MaxHealth())
	require.Equal(t, 181, b.CurrentHealth(), "a body is adopted at full health")

	b.SetHealth(40)
	require.Equal(t, 40, b.CurrentHealth())
	require.Equal(t, 181, b.MaxHealth(), "the band does not move when health does")
}

// A record with a nonsense maximum must not produce a monster that dies to
// the first blow -- at step 4 that would look exactly like a resolver bug,
// which is the whole class of confusion this milestone is trying to avoid.
func TestNPCBodyIsNeverBornDead(t *testing.T) {
	for _, maxHealth := range []int{0, -1} {
		b := newNPCBody(maxHealth)
		require.Equal(t, 1, b.MaxHealth())
		require.Equal(t, 1, b.CurrentHealth())
	}
}

// THE NIL-INTERFACE TRAP, ON THE REAL IMPLEMENTATION.
//
// `return v.bodies[id]` would be the obvious way to write BodyOf and it would
// be wrong: a missing map entry yields a nil *npcBody, and a nil *npcBody
// returned as a d2world.Body is an interface that is NOT nil. The combat
// model's `if body != nil` would pass, it would call CurrentHealth on it, and
// the game would panic in a system that had just reported has_body:true.
//
// A zero-value Game is enough to reach the early return, which is the point:
// no map engine, no client, no screen.
func TestGameBodyOfReturnsAnUntypedNil(t *testing.T) {
	v := &Game{}

	body := v.BodyOf("nobody")

	require.Nil(t, body)
	require.True(t, body == nil, "must be an UNTYPED nil, or has_body lies and the next call panics")

	// And Game really does satisfy the interface d2world declared, which is
	// what lets NewCombat take it.
	var lookup d2world.Bodies = v
	require.True(t, lookup.BodyOf("nobody") == nil)
}

// Adoption is idempotent and release forgets. The idempotence matters because
// BodyOf adopts on demand as well as the spawner adopting eagerly, so the two
// paths can race for the same id across frames; a second adoption that reset
// health to full would heal a monster mid-fight at step 4.
func TestAdoptAndReleaseNPCBody(t *testing.T) {
	v := &Game{}

	v.adoptNPCBody("w:1", 181)
	require.NotNil(t, v.BodyOf("w:1"))
	require.Equal(t, 181, v.BodyOf("w:1").CurrentHealth())

	v.BodyOf("w:1").SetHealth(12)
	v.adoptNPCBody("w:1", 181)
	require.Equal(t, 12, v.BodyOf("w:1").CurrentHealth(), "adopting twice must not heal it")

	v.releaseNPCBody("w:1")
	require.True(t, v.BodyOf("w:1") == nil, "released, and no map engine to re-adopt from")

	// Releasing something that was never adopted is a no-op rather than a
	// panic: Despawn runs for members the screen may never have adopted.
	v.releaseNPCBody("never-existed")

	// An empty id is refused rather than stored under "".
	v.adoptNPCBody("", 50)
	require.True(t, v.BodyOf("") == nil)
}

// TestNeighboursNearestIsOrderedAndDeterministic pins the ordering the
// pursuer-adjacent burst introduced.
//
// Route tries the eight tiles around a quarry NEAREST FIRST, so a hunter
// approaching from the south stops on the south side rather than walking
// around to the table's first entry. The ordering has to be deterministic,
// because these routes move entities and entity positions are inside the state
// digest -- so the tie-break is the table index and nothing else.
func TestNeighboursNearestIsOrderedAndDeterministic(t *testing.T) {
	const qx, qy = 10.0, 10.0

	for _, tc := range []struct {
		name         string
		fromX, fromY float64
		wantFirst    [2]float64
	}{
		{"hunter due south", qx, qy + 6, [2]float64{0, 1}},
		{"hunter due north", qx, qy - 6, [2]float64{0, -1}},
		{"hunter due east", qx + 6, qy, [2]float64{1, 0}},
		{"hunter south-east", qx + 6, qy + 6, [2]float64{1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := neighboursNearest(tc.fromX, tc.fromY, qx, qy)
			if got[0] != tc.wantFirst {
				t.Fatalf("nearest offset should be %v, got %v (full order %v)",
					tc.wantFirst, got[0], got)
			}

			// Every offset appears exactly once: it is a permutation of the
			// table, not a filter. A dropped entry would silently stop a
			// hunter from ever considering one approach tile.
			seen := map[[2]float64]int{}
			for _, n := range got {
				seen[n]++
			}

			if len(seen) != len(routeNeighbours) {
				t.Fatalf("expected all %d offsets exactly once, got %v", len(routeNeighbours), seen)
			}

			// And the distances are non-decreasing, which is the ordering
			// claim itself rather than a proxy for it.
			prev := -1.0

			for _, n := range got {
				dx, dy := qx+n[0]-tc.fromX, qy+n[1]-tc.fromY
				d := dx*dx + dy*dy

				if d < prev {
					t.Fatalf("distances must not decrease; got %v in %v", d, got)
				}

				prev = d
			}
		})
	}

	// Determinism: the same inputs give the same order, every time. Two
	// launches of one build at one seed have to agree.
	a := neighboursNearest(3, 4, 10, 10)
	for i := 0; i < 5; i++ {
		if neighboursNearest(3, 4, 10, 10) != a {
			t.Fatalf("neighboursNearest is not deterministic")
		}
	}
}

// TestUnblockedNeighboursSkipsBlockedTiles pins item 5 (12 Sep 2026): Route drops
// a neighbour candidate tile that is itself blocked before paying for a full A*
// toward it, and keeps nearest-first order among the rest. A blocked goal never
// enters the search's open set and burns the whole budget (proved separately in
// d2mapengine.TestSearchExpansionIsVisibleAndBoundedByABlockedGoal); Route tries
// up to nine tiles per solve, so skipping the blocked ones is the fix (audit B3).
//
// Negative control: drop the `if blocked continue` in unblockedNeighbours and the
// blocked tile reappears -- Len and the NotEqual below both fail.
func TestUnblockedNeighboursSkipsBlockedTiles(t *testing.T) {
	const qx, qy = 10.0, 10.0

	// Hunter due south of the quarry, so the nearest candidate is the south tile
	// (0,+1) -- block exactly that one.
	blockedTile := [2]float64{0, 1}
	blocked := func(tileX, tileY float64) bool {
		return tileX == qx+blockedTile[0] && tileY == qy+blockedTile[1]
	}

	got := unblockedNeighbours(qx, qy+6, qx, qy, blocked)

	require.Len(t, got, len(routeNeighbours)-1, "exactly the one blocked tile is dropped")

	for _, n := range got {
		require.NotEqual(t, blockedTile, n, "the blocked tile must be skipped")
	}

	// Nearest-first is preserved: with due-south gone, the nearest survivor is
	// the south-east diagonal (equal distance to south-west, tie broken on the
	// lower table index).
	require.Equal(t, [2]float64{1, 1}, got[0], "the nearest surviving candidate is the SE diagonal")

	// Positive control: with nothing blocked, all eight survive, due-south first.
	all := unblockedNeighbours(qx, qy+6, qx, qy, func(float64, float64) bool { return false })
	require.Len(t, all, len(routeNeighbours))
	require.Equal(t, [2]float64{0, 1}, all[0], "unblocked, the nearest is due south")
}
