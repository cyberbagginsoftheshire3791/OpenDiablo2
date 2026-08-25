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

// HarnessLocalPlayerID returns the local player's entity ID, or "" before the
// player exists.
func (v *Game) HarnessLocalPlayerID() string {
	if v.gameClient == nil {
		return ""
	}

	return v.gameClient.PlayerID
}
