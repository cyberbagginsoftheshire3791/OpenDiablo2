package d2player

type inputCallbackListener interface {
	OnPlayerMove(x, y float64)
	OnPlayerCast(skillID int, x, y float64)

	// OnTacticalTarget is a click on an enemy inside a paced fight (T1): the
	// game screen strikes it, or walks beside it and strikes on arrival.
	OnTacticalTarget(entityID string)
}
