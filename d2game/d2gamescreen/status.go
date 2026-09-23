package d2gamescreen

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// T9, the status lines (23 Sep 2026): what the village thinks of him, what
// the land still holds, and a promised watch's minutes -- on the kit panel,
// where a player already looks for "what do I have". Before this they were
// visible only in a talk's header and to the harness.

// Status is the kit panel's status lines.
func (v *Game) Status() []string {
	if v.standing == nil || v.dialogue == nil {
		return nil
	}

	rung := v.dialogue.Rung(v.standing).Name
	if rung == "" {
		rung = d2player.TalkStandingNone
	}

	lines := []string{fmt.Sprintf(d2player.StatusVillage, rung, v.standing.Rep)}

	second := fmt.Sprintf(d2player.StatusLand, v.LandLeft())
	if v.standing.Has(d2dialogue.FlagWatch) {
		second += fmt.Sprintf(d2player.StatusWatch, v.watchStood, v.dialogue.Village.WatchMinutes)
	}

	return append(lines, second)
}
