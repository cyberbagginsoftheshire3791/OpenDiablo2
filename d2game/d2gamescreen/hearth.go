package d2gamescreen

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2bestiary"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// M4.7 step 4 (23 Sep 2026): the priest's rite and the hearth. On the build
// note's recommended options, each flagged for Josh:
//
//   - Q6a: the priest's tale (`heard_tale`, set by "Listen." at the hearth
//     rung) is how he learns the dead. Before it a risen man is a man: no
//     bar, and he is called by what he was in life -- Josh's ruling of 27 Sep
//     2026: "A fallen soldier" for Night 1's dead, "A stranger" for the edge
//     floor's, and a slain man by his row's name ("Opportunist"). After it
//     the dead carry a bar and are called by the risen creature's name, "the
//     dead" (PLACEHOLDER -- the bestiary's tells wait on M1). Every one of
//     those words is the bestiary's (deadNames), so a rename is a data edit.
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
)

// deadNames is every word the game has for the dead, all of it the
// bestiary's (Josh's ruling of 27 Sep 2026: he renames them in data).
type deadNames struct {
	// known is what one of the dead is called once the priest has told his
	// tale: the risen row's creature's name. PLACEHOLDER: R1 §1's tells (a
	// ruddy face, blood at the mouth) are the bestiary's, and M1 is not
	// verified.
	known string
	// were is what a risen man is called before the tale when no spawn row
	// names him: Night 1's dead, and the edge floor's wanderers.
	were d2bestiary.DeadWere

	bestiary *d2bestiary.Catalog
}

// deadNamesFrom takes the dead's words from the bestiary, and refuses a
// bestiary without them: the hover would then fall back on the risen
// creature's own name before the tale, which a player must not see then.
func deadNamesFrom(c *d2bestiary.Catalog) (deadNames, error) {
	risen, ok := c.ForSpawnRow(d2world.RisenRow)
	if !ok {
		return deadNames{}, fmt.Errorf("no creature for the %q row: the dead have no name", d2world.RisenRow)
	}

	were := c.DeadWere()
	if were.PlacedDead == "" || were.Wanderer == "" {
		return deadNames{}, errors.New("no the_dead_were: nothing to call a risen man before the priest's tale")
	}

	if were.PlacedDead == risen.Name || were.Wanderer == risen.Name {
		return deadNames{}, fmt.Errorf("the_dead_were repeats %q, the name he learns only from the priest", risen.Name)
	}

	return deadNames{known: risen.Name, were: were, bestiary: c}, nil
}

// rowName is what a man of a spawn row was in life: the row's creature's live
// name ("Opportunist"), or "" for a row with none. Never the dead's own row --
// what he became is not what he was. Corpses.Fall asks it for every body.
func (n deadNames) rowName(row string) string {
	if strings.EqualFold(strings.TrimSpace(row), d2world.RisenRow) {
		return ""
	}

	if e, ok := n.bestiary.ForSpawnRow(row); ok {
		return e.Name
	}

	return ""
}

// name is DeadName's rule on its own: nothing for the living; after the tale,
// the dead's own name; before it, what his body says he was -- or his row's
// name, or, for a man nothing names, a stranger. It never answers the known
// name before the tale.
func (n deadNames) name(risen, knows bool, body d2world.Corpse, hasBody bool) string {
	switch {
	case !risen:
		return ""
	case knows:
		return n.known
	case hasBody && body.Was != "":
		return body.Was
	case hasBody && n.rowName(body.Row) != "":
		return n.rowName(body.Row)
	default:
		return n.were.Wanderer
	}
}

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

// DeadName is what the player is shown for an entity that is one of the dead
// -- on the hover and in the combat log alike (d2player's nameFor) -- and ""
// for anything else. Before the priest's tale a risen man is called what he
// was in life, which his body remembers (Corpses.BodyOf); after it, the dead.
func (v *Game) DeadName(id string) string {
	risen := v.isRisen(id)

	var (
		body    d2world.Corpse
		hasBody bool
	)

	if risen && v.corpses != nil {
		body, hasBody = v.corpses.BodyOf(id)
	}

	return v.deadNames.name(risen, v.knowsTheDead(), body, hasBody)
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
		if _, npc := e.(*d2mapentity.NPC); npc && d2mapentity.NameKey(e) == sp.StandIn {
			return e
		}
	}

	return nil
}

// protectedQuarry is the raid's S0-1, default (a): the four speakers are no
// quarry until their death art (A5) lands. Their stand-ins are 1-HP bodies
// (MaxHPNormal 0) with no death to show -- Kashya and Charsi die standing,
// Warriv and Akara vanish (the raid's S0, M0.5) -- so combat's scans skip a
// pair whose target is one (Combat.SetProtected), and no fight opens on them.
// A speaker is an NPC drawn by a speaker's stand-in, as speakerEntity finds
// one. This departs, for as long as it lasts, from ruling 1's "Full entities"
// and "any living quarry" and from ruling 4's freedom to terrorise the
// village; the brief's S0-1 names it for Josh. Since the raid's R2 Seek reads
// it too (through Combat): a speaker is never chosen as a quarry, so a hostile
// with only a speaker in view keeps its target.
func (v *Game) protectedQuarry(id string) bool { return v.speakerNPC(id) }

// speakerNPC reports an NPC on the map drawn by one of the speakers'
// stand-ins: one of the map's villagers, as the raid's R2 counts them
// (seekQuarries). protectedQuarry is this rule today (S0-1 (a) protects every
// speaker); when their death art lands it lifts and this one stays.
func (v *Game) speakerNPC(id string) bool {
	if v.dialogue == nil || v.gameClient == nil || v.gameClient.MapEngine == nil {
		return false
	}

	e, ok := v.gameClient.MapEngine.Entities()[id]
	if !ok {
		return false
	}

	if _, npc := e.(*d2mapentity.NPC); !npc {
		return false
	}

	key := d2mapentity.NameKey(e)

	for i := range v.dialogue.Speakers {
		if v.dialogue.Speakers[i].StandIn == key {
			return true
		}
	}

	return false
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
