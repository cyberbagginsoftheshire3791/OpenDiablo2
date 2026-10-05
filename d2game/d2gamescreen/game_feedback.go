package d2gamescreen

import (
	"math"
)

// F8's note box and the crash report read the game through these two (5 Oct
// 2026). The box itself is the App's (d2app/feedback.go): it is the same box on
// the menus, in the World Editor and in a game.

// SetFeedbackHold holds the world while F8's note box is open over this game,
// as the escape menu holds it (screenLive), and lets it go when the box closes.
func (v *Game) SetFeedbackHold(held bool) { v.feedbackHeld = held }

// FeedbackHeld is the hold SetFeedbackHold set.
func (v *Game) FeedbackHeld() bool { return v.feedbackHeld }

// FeedbackState is what a note's state.json says of the game: the day and the
// clock, where he stands, how the view is drawn, what is open, what holds the
// world. It reads only what the HUD reads, and every read is guarded against a
// half-built game -- a crash report calls it from inside a panic.
func (v *Game) FeedbackState() map[string]interface{} {
	state := map[string]interface{}{}

	if v.worldClock != nil {
		state["day"] = v.worldClock.DayIndex()
		state["clock"] = v.worldClock.TimeOfDay()
		state["world_minutes"] = round2(v.worldClock.WorldMinutes())
	}

	if v.localPlayer != nil && v.localPlayer.Position.World() != nil {
		tile := v.localPlayer.Position.Tile()
		state["player_tile"] = map[string]interface{}{"x": round2(tile.X()), "y": round2(tile.Y())}
		state["hero"] = v.localPlayer.Name()
	}

	reason := v.fogOffReason()
	state["fog"] = reason == ""

	if reason != "" {
		state["fog_off_reason"] = reason
	}

	if v.gameControls != nil {
		for k, val := range v.gameControls.FeedbackState() {
			state[k] = val
		}
	}

	if v.gameClient != nil {
		state["world_held_by"] = v.WorldHeldBy()
		state["players"] = len(v.gameClient.Players)
	}

	if v.combat != nil && v.combat.Fighting() {
		state["encounter"] = v.combat.Encounter()
		state["round"] = v.combat.Round()
	}

	return state
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
