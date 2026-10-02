package d2world

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// BUG-108 (fixed 1 Oct 2026): a chase ends when its hunter forgets its quarry
// (Pursuit.GiveUpOnTheForgotten). A dog that saw him, chased, and lost sight
// keeps the chase through the notice's memory and is released the moment the
// watch forgets; a chase with no watch behind it, and a watch of another
// quarry, are left alone. THE CONTROL, in the test: before the memory runs
// out the chase stands -- so it is the forgetting, and nothing else, that
// releases it.
func TestAChaseEndsWhenItsHunterForgets(t *testing.T) {
	n, sight, _ := newTestNotice(true, 0)
	p, _ := newTestPursuit(true)

	him := &fakeQuarry{id: "p:1", x: 5, y: 0}
	villager := &fakeQuarry{id: "v:1", x: 3, y: 0}

	dog := &fakeHunter{id: "n:1"}
	n.Watch(&fakeWatcher{id: "n:1"}, him)
	p.Chase(dog, him)

	// A chase with no watch (the harness's strigoi_pursue).
	p.Chase(&fakeHunter{id: "n:2"}, him)

	// A watch of him, but a chase of the villager.
	n.Watch(&fakeWatcher{id: "n:3"}, him)
	p.Chase(&fakeHunter{id: "n:3"}, villager)

	require.Empty(t, p.GiveUpOnTheForgotten(n), "all seen: nothing gives up")

	sight.clear = false
	n.Advance(DefaultNoticeDials().ReEvaluateMinutes)

	noticed, _ := n.Noticed("n:1")
	require.True(t, noticed, "lost sight, still remembered")
	require.Empty(t, p.GiveUpOnTheForgotten(n), "the control: inside the memory the chase stands")
	require.True(t, p.Chasing("n:1"))

	n.Advance(DefaultNoticeDials().MemoryMinutes)

	noticed, _ = n.Noticed("n:1")
	require.False(t, noticed, "past the memory it forgets")
	require.Equal(t, []string{"n:1"}, p.GiveUpOnTheForgotten(n), "the forgotten chase of him ends")
	require.False(t, p.Chasing("n:1"))
	require.Equal(t, []string{"n:2"}, p.ChasersOf("p:1"), "a chase with no watch behind it stands")
	require.True(t, p.Chasing("n:3"), "a chase of another quarry than the watch's stands")
	require.Empty(t, p.GiveUpOnTheForgotten(nil), "no notice model: nothing")
}

// integrate-1oct: the combat status's give-up (BUG-108) meeting the raid's R2
// (Seek's retarget and the re-chase budget). Seek moves two hunters' watches
// from him onto a villager in one frame; the budget (RechasesPerFrame, 1)
// moves one chase and defers the other, which stands on him. Both watches
// then forget the villager. BOTH chases must end -- the deferred one is a
// chase of him that no watch is behind, and left standing it would keep him
// "chased" (COMBAT) for good -- while the re-chase cap held in the frame it
// was asked. THE CONTROL, in the test: while the watches are still aware
// nothing gives up, and the deferred chase is still on him.
func TestAForgottenWatchEndsTheChaseSeekLeftBehind(t *testing.T) {
	n, sight, _ := newTestNotice(true, 0)
	p, _ := newTestPursuit(true)
	defer p.Close()

	require.Equal(t, 1, p.dials.RechasesPerFrame, "the shipped budget this test reads")

	him := &fakeQuarry{id: "p:1", x: 5, y: 0}
	villager := &fakeQuarry{id: "v:1", x: 3, y: 0}

	dog := &fakeHunter{id: "n:1"}
	wolf := &fakeHunter{id: "n:2"}

	n.Watch(&fakeWatcher{id: "n:1"}, him)
	n.Watch(&fakeWatcher{id: "n:2"}, him)
	require.True(t, p.Rechase(dog, him), "a first chase is not budgeted")
	require.True(t, p.Rechase(wolf, him), "a first chase is not budgeted")
	p.Advance(0.01)

	// Seek's retarget: both watches move to the villager, and see her.
	require.True(t, n.Retarget("n:1", villager))
	require.True(t, n.Retarget("n:2", villager))

	require.True(t, p.Rechase(dog, villager), "the frame's one re-chase")
	require.False(t, p.Rechase(wolf, villager), "the cap: the second is deferred")
	require.Equal(t, []string{"n:2"}, p.ChasersOf("p:1"), "the deferred chase stands on him")

	require.Empty(t, p.GiveUpOnTheForgotten(n), "the control: aware watches give nothing up")
	require.Equal(t, []string{"n:2"}, p.ChasersOf("p:1"))

	sight.clear = false
	n.Advance(DefaultNoticeDials().ReEvaluateMinutes)
	n.Advance(DefaultNoticeDials().MemoryMinutes)

	for _, id := range []string{"n:1", "n:2"} {
		noticed, watching := n.Noticed(id)
		require.True(t, watching)
		require.False(t, noticed, "%s has forgotten the villager", id)
	}

	require.Equal(t, []string{"n:1", "n:2"}, p.GiveUpOnTheForgotten(n),
		"both world chases end: the villager's, and the one Seek's budget left on him")
	require.Empty(t, p.ChasersOf("p:1"), "nothing chases him")
	require.Zero(t, p.Count())

	// A script's chase of another quarry than its hunter's (forgotten) watch
	// still stands, as the combat status shipped it.
	p.Chase(&fakeHunter{id: "n:1"}, him)
	require.Empty(t, p.GiveUpOnTheForgotten(n), "strigoi_pursue's chase of another quarry stands")
	require.Equal(t, []string{"n:1"}, p.ChasersOf("p:1"))
}
