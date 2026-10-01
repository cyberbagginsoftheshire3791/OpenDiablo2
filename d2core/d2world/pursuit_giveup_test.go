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
