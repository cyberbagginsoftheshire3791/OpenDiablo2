package d2gamescreen

import "github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"

// The zoom a new game's map renderer starts at lives in d2player
// (processGameZoom), because the game's wheel moves it too: -zoom sets it
// once, clamped to the game's range, and a death's load, "load last save" or a
// new game then keep the view the player last chose (the zoom review, C2).
// The shipped view is 0.5 since F5 (2 Oct 2026; Josh, 1 Oct: the game zooms
// out; the art and the world's distances do not change): d2app's -zoom
// defaults to it, and to 1.0 under -classic. 1.0 is still the renderer's own
// unzoomed draw, call for call.

// SetGameZoom sets the zoom new games start at. d2app calls it with -zoom.
func SetGameZoom(z float64) { d2player.SetGameZoom(z) }

// GameZoom is the zoom new games start at.
func GameZoom() float64 { return d2player.GameZoom() }

// applyGameZoom gives a new game's map renderer the starting zoom. At 1.0 it
// does nothing at all -- the renderer is already unzoomed, and the shipped game
// must be the game it was before the zoom, call for call.
func (v *Game) applyGameZoom() {
	if z := d2player.GameZoom(); z != 1.0 {
		v.mapRenderer.SetScale(z)
	}
}
