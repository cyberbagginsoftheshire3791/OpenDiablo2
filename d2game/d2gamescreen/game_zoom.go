package d2gamescreen

import "github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"

// gameZoom is the view scale every new game's map renderer starts at: -zoom's
// value, clamped to the game's range (d2player.ClampGameZoom), 1.0 when the
// flag is not given. 1.0 is the shipped view and the default (Josh, 1 Oct
// 2026: the game zooms out; the art and the world's distances do not change).
//
// nolint:gochecknoglobals // set once from the command line, before any game
var gameZoom = 1.0

// SetGameZoom sets the zoom new games start at. d2app calls it with -zoom.
func SetGameZoom(z float64) { gameZoom = d2player.ClampGameZoom(z) }

// GameZoom is the zoom new games start at.
func GameZoom() float64 { return gameZoom }

// applyGameZoom gives a new game's map renderer the starting zoom. At 1.0 it
// does nothing at all -- the renderer is already unzoomed, and the shipped game
// must be the game it was before the zoom, call for call.
func (v *Game) applyGameZoom() {
	if gameZoom != 1.0 {
		v.mapRenderer.SetScale(gameZoom)
	}
}
