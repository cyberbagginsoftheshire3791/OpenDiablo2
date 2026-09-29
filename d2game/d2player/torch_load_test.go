package d2player

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// torchControls is the least a GameControls needs to answer the L key: a
// closed escape menu, a light model, and one unlit torch to take out (the
// no-kit path, which a harness or unit build uses).
func torchControls(l *d2world.Light) *GameControls {
	return &GameControls{escapeMenu: &EscapeMenu{}, light: l, torchesCarried: 1}
}

// TestALoadedTorchIsNotRelit is D1's controls half (28 Sep 2026): THE LIGHT
// MODEL IS THE TRUTH ON LOAD. The load restores the light snapshot -- his
// carried torch with its id, next_id, lit and minutes -- into the resumed
// game's model. It does NOT press L. So the load counts no torch verb
// (ui.torch_verbs, which is not saved: the resumed controls start at zero and
// stay there until he presses L), and the controls over the restored model do
// exactly what they did over the saved one: the next L douses a lit torch and
// relights a doused one, keeping the same source, one verb each.
//
// The plan's first trap-4 text said "re-light through the L path". Over a
// DOUSED torch that path lights it -- a verb the saved game never made -- and
// over a lit one it would douse it; either way torch_verbs moves and the torch
// is no longer the one that was saved.
func TestALoadedTorchIsNotRelit(t *testing.T) {
	for _, lit := range []bool{true, false} {
		clock := d2world.NewClock(d2world.DefaultClockDials())
		saved := d2world.NewLight(clock, d2world.DefaultLightDials())

		g := torchControls(saved)
		g.combatTorch() // light: one verb

		if !lit {
			g.combatTorch() // douse: a second verb; the source stays, with its minutes
		}

		saved.Advance(clock.Advance(4))

		verbs := g.torchVerbs
		before := *saved.Carried()
		snap := saved.Snapshot()

		// The load: a fresh model restored from the snapshot and handed to
		// the game's controls. Nothing here presses L.
		loaded := d2world.NewLight(clock, d2world.DefaultLightDials())
		if err := loaded.Restore(snap); err != nil {
			t.Fatalf("lit=%v: %v", lit, err)
		}

		// The resumed game's controls, fresh as a relaunch builds them. The
		// load pressed nothing, so they have counted nothing; a load that
		// re-lit through L would already read one here.
		resumed := torchControls(loaded)

		if got := *loaded.Carried(); got != before {
			t.Fatalf("lit=%v: the loaded torch is %+v, saved %+v", lit, got, before)
		}

		if resumed.torchVerbs != 0 || g.torchVerbs != verbs {
			t.Fatalf("lit=%v: the load counted a torch verb (resumed %d, saved %d -> %d)",
				lit, resumed.torchVerbs, verbs, g.torchVerbs)
		}

		// The next L on each does the same thing to the same source.
		g.combatTorch()
		resumed.combatTorch()

		a, b := *saved.Carried(), *loaded.Carried()
		if a != b || a.ID != before.ID || a.Lit == before.Lit {
			t.Fatalf("lit=%v: the next L must toggle the SAME torch on both: saved %+v, loaded %+v (was %+v)",
				lit, a, b, before)
		}

		if g.torchVerbs != verbs+1 || resumed.torchVerbs != 1 {
			t.Fatalf("lit=%v: one L is one verb on each: saved %d -> %d, resumed 0 -> %d",
				lit, verbs, g.torchVerbs, resumed.torchVerbs)
		}

		saved.Close()
		loaded.Close()
		clock.Close()
	}
}
