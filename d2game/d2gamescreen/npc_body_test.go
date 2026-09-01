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
