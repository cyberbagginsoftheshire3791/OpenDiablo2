package d2gamescreen

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// M4.7 step 4 (23 Sep 2026): the priest's rite and the hearth. On the build
// note's recommended options, each flagged for Josh:
//
//   - Q6a: the priest's tale (`heard_tale`, set by "Listen." at the hearth
//     rung) is how he learns the dead. Before it a risen man is a man: no
//     bar, and the hover calls him what his body was. After it the dead
//     carry a bar and the hover names them (PLACEHOLDER label -- the
//     bestiary's tells wait on M1).
//   - Q4a: `rite_granted` (the rite rung's "Thank him.") has the priest close
//     every hasty grave of a man within rite_radius of the church at each
//     first light, while the village still stands at the rite rung and the
//     gate is open to him -- "the dead you buried", a widening of S1 §8.2's "your
//     own dead" that H7/M11 must bless. The priest's sprite stands in for
//     the church until there is one.
//   - S1 §8.2: a staking SEEN -- by day, within seen_radius of any villager --
//     costs seen_cost standing, the first time only.

const (
	flagHeardTale   = "heard_tale"
	flagRiteGranted = "rite_granted"
	flagSeenStaking = "seen_staking"
	flagGateClosed  = "gate_closed"
	riteRung        = "rite"

	priestSpeaker = "priest"

	// deadLabel is what the hover calls one of the dead once he knows them.
	// PLACEHOLDER: R1 §1's tells (a ruddy face, blood at the mouth) are the
	// bestiary's, and M1 is not verified.
	deadLabel = "the dead"
)

// knowsTheDead is the hearth unlock (Q6a).
func (v *Game) knowsTheDead() bool {
	return v.standing != nil && v.standing.Has(flagHeardTale)
}

// isRisen reports an entity of the dead's row.
func (v *Game) isRisen(id string) bool {
	if v.spawns == nil {
		return false
	}

	p, ok := v.spawns.ProfileOf(id)

	return ok && p.Dead
}

// DeadName is what the hover calls an entity that is one of the dead: "" --
// his body's own name -- until the priest's tale, deadLabel after.
func (v *Game) DeadName(id string) string {
	if v.knowsTheDead() && v.isRisen(id) {
		return deadLabel
	}

	return ""
}

// speakerEntity finds a villager's stand-in sprite on the map, looked up each
// call for the reason headmanEntity gives. Only an NPC can be a villager: a
// hero who shares a stand-in's name is not the priest (the step-4 review).
func (v *Game) speakerEntity(speaker string) d2interface.MapEntity {
	if v.dialogue == nil || v.gameClient == nil || v.gameClient.MapEngine == nil {
		return nil
	}

	sp := v.dialogue.Speaker(speaker)
	if sp == nil {
		return nil
	}

	for _, e := range v.gameClient.MapEngine.Entities() {
		if _, npc := e.(*d2mapentity.NPC); npc && e.Label() == sp.StandIn {
			return e
		}
	}

	return nil
}

// riteHolds is the priest still keeping his promise: the rite granted, the
// village still at the rite rung, and the gate not shut against him -- a man
// who took the bread at sword-point has no priest (the step-4 review; a
// reading taken for Josh to confirm).
func (v *Game) riteHolds() bool {
	return v.standing != nil && v.dialogue != nil &&
		v.standing.Has(flagRiteGranted) && !v.standing.Has(flagGateClosed) &&
		v.dialogue.Reached(v.standing, riteRung)
}

// riteCloses is the rule on its own: a man's hasty grave within the radius.
func riteCloses(b d2world.Corpse, dist, radius float64) bool {
	return b.State == d2world.CorpseHasty && b.Class == d2world.CorpseHuman && dist <= radius
}

// seenBy is the other rule on its own: by day, within the radius.
func seenBy(night bool, dist, radius float64) bool {
	return !night && dist <= radius
}

// riteAtDawn is Q4a: with the rite granted, the priest closes the hasty
// graves of men within rite_radius of the church. Each is a rite (soul
// pressure falls). It reports how many he closed.
func (v *Game) riteAtDawn() int {
	if !v.riteHolds() || v.corpses == nil {
		return 0
	}

	church := v.speakerEntity(priestSpeaker)
	if church == nil {
		return 0
	}

	cx, cy := church.GetPositionF()
	n := 0

	for _, b := range v.corpses.All() {
		if !riteCloses(b, math.Hypot(b.X-cx, b.Y-cy), v.dialogue.Village.RiteRadius) {
			continue
		}

		if v.corpses.Close(b.ID) {
			n++

			if v.rising != nil {
				v.rising.Rite()
			}
		}
	}

	if n > 0 && v.gameControls != nil {
		v.gameControls.SetZoneChangeText(fmt.Sprintf(d2player.RiteAtDawn, n))
		v.gameControls.ShowZoneChangeText()
		v.gameControls.HideZoneChangeTextAfter(levelNoticeSeconds)
	}

	return n
}

// seenStaking charges a staking the village saw: by day, within seen_radius
// of any villager, the first time only (S1 §8.2). It reports whether it did.
func (v *Game) seenStaking() bool {
	if v.standing == nil || v.dialogue == nil || v.localPlayer == nil || v.standing.Has(flagSeenStaking) {
		return false
	}

	px, py := v.localPlayer.GetPositionF()

	for i := range v.dialogue.Speakers {
		e := v.speakerEntity(v.dialogue.Speakers[i].ID)
		if e == nil {
			continue
		}

		ex, ey := e.GetPositionF()
		if !seenBy(v.night(), math.Hypot(px-ex, py-ey), v.dialogue.Village.SeenRadius) {
			continue
		}

		v.dialogue.Move(v.standing, -v.dialogue.Village.SeenCost)
		v.standing.Mark(flagSeenStaking)

		return true
	}

	return false
}
