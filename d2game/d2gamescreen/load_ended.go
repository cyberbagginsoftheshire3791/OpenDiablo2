package d2gamescreen

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
)

// EndedAction is an entity whose held action the load ENDED rather than
// resumed at its saved point, because this build's art no longer fits it --
// a sheet re-exported with fewer or more frames, a sheet added or taken away
// since the save (the BUG-87 review's B2, BUG-92; Josh's default, his to
// overturn). The entity is resumed where the file puts it, in the state the
// action would have reached on its own end: a swing or a blow back to idle,
// a death to its corpse. It differs from the saved moment by design, and the
// load report says which and why so a script can hold the rest of the world
// to the saved moment. A CORRUPT held action (a negative frame, a time no
// play could have: d2mapentity.heldActionMisfit's line) is still refused.
type EndedAction struct {
	ID     string `json:"id"`
	Who    string `json:"who"`
	Action string `json:"action"`
	Why    string `json:"why"`
}

// noteEndedAction records a held action the load ended (nil: none) in the
// load report -- its ended_actions and a note, "an animation could not be
// resumed exactly: ..." -- and in the log. who names the entity as the load
// knows it: a villager's name key, another entity's kind and record.
func (v *Game) noteEndedAction(id, who string, ended *d2mapentity.HeldActionEnded) {
	if ended == nil {
		return
	}

	v.loadNote("%s (%s): %s", id, who, ended)
	updateLastLoad(func(r *LoadReport) {
		r.EndedActions = append(r.EndedActions, EndedAction{ID: id, Who: who, Action: ended.Action, Why: ended.Why})
	})
}
