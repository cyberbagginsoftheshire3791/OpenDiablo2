package d2interface

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
)

// Navigator is used for transitioning between game screens
type Navigator interface {
	ToMainMenu(errorMessageOptional ...string)
	ToSelectHero(connType d2clientconnectiontype.ClientConnectionType, connHost string)
	ToCreateGame(filePath string, connType d2clientconnectiontype.ClientConnectionType, connHost string)
	ToCharacterSelect(connType d2clientconnectiontype.ClientConnectionType, connHost string)
	ToMapEngineTest(region int, level int)
	ToCredits()
	ToCinematics()
	// ToWorldEditor opens the World Editor on a map. "" means the default map,
	// the village -- the only .tmj there is.
	ToWorldEditor(mapPath string)
	// ToPlaytest starts a real game on a map the editor has just written, the
	// normal way in: the authored map is set and the player picks his hero. It
	// is a separate verb from ToCreateGame because the editor names a FILE and
	// has no save path to hand over.
	ToPlaytest(mapPath string)
}
