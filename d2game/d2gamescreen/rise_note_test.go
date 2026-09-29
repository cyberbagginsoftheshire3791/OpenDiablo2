package d2gamescreen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// The raid R1 review's B2 (BUG-69): a Downed man who stands again is
// "reraised" in his journal -- "I cut one down and turned my back on it" --
// only when the fight he stands again into is HIS. Before the fix Rejoin
// reached the village's fights too (R1's assertion 5) and the screen took its
// yes for his, so a man a villager cut down across the village wrote that he
// had cut him down. raiseTheDead's decision is rejoinHisFight's answer read
// by riseNote; both are exercised here on a real combat model with his fight
// and a village fight live, each holding a man who stands again.

// riseSight sees everything; riseLight lights everything.
type riseSight struct{}

func (riseSight) Clear(_, _, _, _ float64) bool { return true }

type riseLight struct{}

func (riseLight) Level(_, _ int) float64 { return 1 }

// riseAt is a thing at a place: a watcher, a quarry, a man standing again.
type riseAt struct {
	id   string
	x, y float64
}

func (r riseAt) WatcherID() string         { return r.id }
func (r riseAt) WatcherAt() (x, y float64) { return r.x, r.y }
func (r riseAt) QuarryID() string          { return r.id }
func (r riseAt) QuarryAt() (x, y float64)  { return r.x, r.y }

func TestAManWhoStandsIntoTheVillagesFightIsNotOneHeCutDown(t *testing.T) {
	clock := d2world.NewClock(d2world.DefaultClockDials())
	t.Cleanup(clock.Close)

	notice := d2world.NewNotice(riseSight{}, riseLight{}, d2world.DefaultNoticeDials())
	combat := d2world.NewCombat(clock, notice, nil, nil, nil, nil, nil, nil, nil, 1462, d2world.DefaultCombatDials())
	t.Cleanup(combat.Close)
	combat.SetPlayer("p:1")

	// h:1 comes for him; r:1 for a villager forty tiles off.
	notice.Watch(riseAt{"h:1", 41.5, 40.5}, riseAt{"p:1", 40.5, 40.5})
	notice.Watch(riseAt{"r:1", 81.5, 40.5}, riseAt{"v:1", 80.5, 40.5})

	for i := 0; i < 20; i++ {
		h, _ := notice.Noticed("h:1")
		r, _ := notice.Noticed("r:1")

		if h && r {
			break
		}

		notice.Advance(1.0)
	}

	combat.Advance(0.25)
	require.True(t, combat.Participates("h:1"), "his fight holds h:1")

	clockBlock := func() map[string]interface{} { return combat.HarnessState()["clock"].(map[string]interface{}) }
	require.Len(t, clockBlock()["live"], 1, "the village's fight holds r:1")

	v := &Game{combat: combat}

	// r:1, cut down by the villager, stands again as r:2: back into the
	// village's fight -- and not a man he cut down.
	require.Equal(t, "rose", riseNote("r:2", v.rejoinHisFight("r:1", riseAt{"r:2", 81.5, 40.5})),
		"a man standing again into a fight he is not in is not reraised")
	require.Equal(t, 1, clockBlock()["joined"], "he did rejoin the village's fight")
	require.Equal(t, 0, combat.HarnessState()["joined"], "and not his")

	// h:1, cut down by him, stands again as h:2: back into HIS fight.
	require.Equal(t, "reraised", riseNote("h:2", v.rejoinHisFight("h:1", riseAt{"h:2", 41.5, 40.5})),
		"a man standing again into his fight is one he cut down")
	require.Equal(t, 1, combat.HarnessState()["joined"])
	require.True(t, combat.Participates("h:2"))

	// A body that stood for the first time, or stood nobody up.
	require.Equal(t, "rose", riseNote("n:1", v.rejoinHisFight("n:0", riseAt{"n:1", 60, 60})), "a man in no fight rose")
	require.Equal(t, "", riseNote("", true), "nobody stood: nothing to note")
	require.False(t, (&Game{}).rejoinHisFight("h:2", riseAt{"h:3", 41.5, 40.5}), "no combat model: no fight")
}
