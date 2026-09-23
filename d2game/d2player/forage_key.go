package d2player

import "github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"

// T7: K forages -- half an hour head-down gathering branches. The game screen
// owns the land and refuses the verb in a fight, a talk or the loadout choice.

// forageKey is K. D2's default map leaves it unbound.
const forageKey = d2enum.KeyK

// ForageHolder gathers from the land.
type ForageHolder interface {
	Forage() error
}

// SetForageHolder attaches the game screen's owner of the land.
func (g *GameControls) SetForageHolder(h ForageHolder) { g.forageHolder = h }
