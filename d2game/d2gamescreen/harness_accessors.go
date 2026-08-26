package d2gamescreen

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
)

// Accessors added for the Phase 3 playtest harness (P3 spec §4). Read-only;
// compiled in every build configuration.

// HarnessMapRenderer returns the game's map renderer, or nil before the game
// screen has loaded. Used by the harness's dump_surface diagnostic.
func (v *Game) HarnessMapRenderer() *d2maprenderer.MapRenderer {
	return v.mapRenderer
}

// HarnessControlsBound reports whether the game controls exist — they are
// created at the end of the first Advance that finds the local player, so
// true means the world has run at least one frame with the player in it
// (first-frame initialisations such as the player's animation mode are done).
func (v *Game) HarnessControlsBound() bool {
	return v.gameControls != nil
}

// HarnessLocalPlayerID returns the local player's entity ID, or "" before the
// player exists.
func (v *Game) HarnessLocalPlayerID() string {
	if v.gameClient == nil {
		return ""
	}

	return v.gameClient.PlayerID
}
