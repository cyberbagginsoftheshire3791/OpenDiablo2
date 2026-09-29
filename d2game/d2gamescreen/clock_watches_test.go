package d2gamescreen

// BUG-73 in the game screen (the raid-r1 follow-up, 29 Sep 2026; the merge
// scout's N3): the load's step 4 (checkLoad) and the save's own check
// (validateSnapshots) hold the file's live clock fights against its watches
// and chases (d2world.CheckClockWatches). A unit game with two clock fights on
// its map: the save takes it -- a live model with two clock fights is never
// refused -- and step 4 takes the file as saved; with the two fights'
// quarries swapped, which every other check of step 4 takes, step 4 refuses
// it BLOCK, and a save that assembled such a block writes nothing.

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
)

// cwDogs are the four dogs of the two clock fights, in sub-tiles: a quarry
// and the dog one tile east that fights it, twenty tiles from the next pair.
var cwDogs = [][2]int{{60, 60}, {65, 60}, {60, 160}, {65, 160}}

// cwPlace puts the four dogs on v's map with the bestiary id the save writes
// each by, as natives (the map had them when the screen was made: B4a's step
// 4 holds every creature on the map to the file's natives), and, given ids,
// re-keys each to its saved id (B4b's re-key by hand: B4a rebuilds nothing).
func cwPlace(t *testing.T, v *Game, ids []string) []*d2mapentity.Creature {
	t.Helper()

	engine := v.gameClient.MapEngine
	out := make([]*d2mapentity.Creature, 0, len(cwDogs))

	for i, at := range cwDogs {
		d := b3Dog(t, engine, at[0], at[1])
		d.SetCreatureID("feral-dog")

		if ids != nil {
			engine.RemoveEntity(d)
			b3SetField(t, d, "uuid", ids[i])
			engine.AddEntity(d)
		}

		out = append(out, d)
	}

	v.natives = nativesOf(engine)

	return out
}

// cwGame is a savable unit game (b4Game) with him named and the combat
// model's Resolver attached, as CreateGame attaches it.
func cwGame(t *testing.T, ids []string) (*Game, string, []*d2mapentity.Creature) {
	t.Helper()

	v, save := b4Game(t)
	b3SetField(t, v.localPlayer, "uuid", "p-him")

	dogs := cwPlace(t, v, ids)
	v.combat.SetResolver(worldResolver{v})

	return v, save, dogs
}

// cwTwoFights is the saved game: dog 1 aware of dog 0, dog 3 of dog 2, and
// the two clock fights they open.
func cwTwoFights(t *testing.T) (*Game, string, []string) {
	t.Helper()

	v, save, dogs := cwGame(t, nil)
	v.combat.SetPlayer(v.localPlayer.ID())

	v.notice.Watch(chaser{entity: dogs[1]}, prey{entity: dogs[0]})
	v.notice.Watch(chaser{entity: dogs[3]}, prey{entity: dogs[2]})

	for _, d := range []*d2mapentity.Creature{dogs[1], dogs[3]} {
		noticed, watching := v.notice.Noticed(d.ID())
		require.True(t, noticed && watching, "%s sees the dog beside it", d.ID())
	}

	v.combat.Advance(0.5)

	live := v.combat.HarnessState()["clock"].(map[string]interface{})["live"].([]map[string]interface{})
	require.Len(t, live, 2, "two clock fights live")

	ids := make([]string, 0, len(dogs))
	for _, d := range dogs {
		ids = append(ids, d.ID())
	}

	return v, save, ids
}

// cwSwap exchanges the first two live clock fights' quarries: the scout's N3.
func cwSwap(w *d2save.World) {
	live := w.Combat.Clock.Live
	live[0].Quarry, live[1].Quarry = live[1].Quarry, live[0].Quarry
}

// TestAClockFightsSwappedQuarryIsRefused: the file of a live model with two
// clock fights is written (the save's own check takes it) and passes the
// load's step 4; the same file with the fights' quarries swapped passes the
// combat block's own Validate and is refused BLOCK by step 4's cross-check.
// The control: swapped back, taken again.
func TestAClockFightsSwappedQuarryIsRefused(t *testing.T) {
	saved, save, ids := cwTwoFights(t)
	_, data := b4File(t, saved, save, "t.world.json")

	step4 := func(w *d2save.World) *LoadRefusal {
		t.Helper()

		g, _, _ := cwGame(t, ids)
		if r := g.restoreClock(w); r != nil {
			return r
		}

		return g.checkLoad(w)
	}

	w, err := d2save.Decode(data)
	require.NoError(t, err)
	require.Len(t, w.Combat.Clock.Live, 2)
	require.Len(t, w.Notice.Watches, 2)

	r := step4(w)
	require.Nil(t, r, "the file as saved passes step 4: %v", r)

	cwSwap(w)

	g, _, _ := cwGame(t, ids)
	require.NoError(t, g.combat.Validate(w.Combat, g.gameClient.Seed), "the gap: the combat block's own check takes the swapped file")

	r = step4(w)
	require.NotNil(t, r, "the swapped file passed step 4")
	require.Equal(t, LoadRefusedBlock, r.Code)
	require.Contains(t, r.Detail, "combat: combat snapshot: clock.live[0] c:1")
	require.Contains(t, r.Detail, "is aware of")

	cwSwap(w)
	require.Nil(t, step4(w), "swapped back, it is the saved file")
}

// TestASaveThatTookASwappedClockBlockWritesNothing: the save runs the load's
// cross-check on the file it assembled (validateSnapshots), so a block no
// load would take is an error (INTERNAL) and nothing is written; the probe
// swaps the quarries after the snapshots are taken, as a bug in a snapshot
// would. The control is the same game saved with no probe: written.
func TestASaveThatTookASwappedClockBlockWritesNothing(t *testing.T) {
	v, save, _ := cwTwoFights(t)

	t.Cleanup(func() { saveProbe = nil })
	saveProbe = cwSwap

	_, to, err := b3SaveTo(t, v, save)
	saveProbe = nil

	require.Error(t, err, "a swapped clock block was written")

	var refusal *SaveRefusal
	require.False(t, errors.As(err, &refusal), "a disagreement is an error (INTERNAL), not a refusal: %v", err)
	require.Contains(t, err.Error(), "the combat block this save took is one the load would refuse: combat snapshot: clock.live[0] c:1")

	_, serr := os.Stat(to)
	require.True(t, os.IsNotExist(serr), "nothing is written")

	_, _, err = b3SaveTo(t, v, save)
	require.NoError(t, err, "the control: the live model with two clock fights saves")
}
