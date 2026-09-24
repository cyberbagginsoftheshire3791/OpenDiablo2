package d2gamescreen

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// T4, talk (23 Sep 2026): the game screen's half. It owns the village's
// standing (saved beside his save, rolled back with it on death), opens talks
// with villagers in reach, applies what an answer costs or gives to the
// systems that own it -- the meters for water and bread, the clock for the
// time labour takes -- and settles a promised watch at dawn.

// dialoguePath is the shipped dialogue table.
const dialoguePath = "/data/strigoi/dialogue.json"

// TalkReachTiles is how close he must stand to talk [DIAL]: close enough to be
// heard without shouting across the ditch.
const TalkReachTiles = 4.0

// Refusals the talk verb gives.
var (
	errTalkTooFar   = errors.New("too far to talk -- walk closer")
	errTalkInFight  = errors.New("not in a fight")
	errTalkNobody   = errors.New("no one here to talk to")
	errTalkNotAlive = errors.New("the dead do not talk")
)

// bindStanding reads his standing from the sidecar's village block, or makes
// him a stranger at the gate.
func (v *Game) bindStanding(raw json.RawMessage) {
	if v.dialogue == nil {
		return
	}

	st := v.dialogue.NewStanding()

	if len(raw) > 0 {
		if err := json.Unmarshal(raw, st); err != nil {
			v.Errorf("village: %v -- a stranger again", err)

			st = v.dialogue.NewStanding()
		}

	}

	// Sorted, unique, inside today's floor and ceiling: a hand-edited or older
	// file must not reopen a closed gate (review finding).
	v.dialogue.Normalise(st)

	v.standing = st

	d2harness.Register(villageProvider{v})
}

// standingJSON is what the sidecar carries.
func (v *Game) standingJSON() json.RawMessage {
	if v.standing == nil {
		return nil
	}

	data, err := json.Marshal(v.standing)
	if err != nil {
		v.Errorf("village: %v", err)
		return nil
	}

	return data
}

// RoleFor is the role a stand-in sprite plays, or "".
func (v *Game) RoleFor(label string) string {
	if v.dialogue == nil {
		return ""
	}

	if s := v.dialogue.SpeakerFor(label); s != nil {
		return s.Role
	}

	return ""
}

func (v *Game) night() bool {
	return v.worldClock != nil && v.worldClock.Stage() == d2world.StageNight
}

// TalkTo opens a talk with the villager a stand-in sprite at (x, y) plays.
func (v *Game) TalkTo(label string, x, y float64) error {
	if v.dialogue == nil || v.standing == nil {
		return errTalkNobody
	}

	sp := v.dialogue.SpeakerFor(label)
	if sp == nil {
		return errTalkNobody
	}

	return v.openTalk(sp.ID, x, y)
}

func (v *Game) openTalk(speakerID string, x, y float64) error {
	switch {
	case v.died || !v.alive():
		return errTalkNotAlive
	case v.combat != nil && v.combat.Fighting():
		return errTalkInFight
	}

	if v.localPlayer != nil {
		px, py := v.localPlayer.GetPositionF()
		if math.Hypot(px-x, py-y) > TalkReachTiles {
			return errTalkTooFar
		}
	}

	talk, err := v.dialogue.Open(v.standing, speakerID, v.night())
	if err != nil {
		return err
	}

	// A talk over as it opens is never kept: v.talk holds the world.
	if talk.Done() {
		return errTalkNobody
	}

	v.talk = talk

	// J1: whom he spoke to, and where the talk began.
	v.note("talked:" + speakerID)
	v.note("node:" + talk.NodeID)

	return nil
}

// Talking reports an open talk and what it shows.
func (v *Game) Talking() (bool, d2player.TalkView) {
	if v.talk == nil || v.talk.Done() {
		return false, d2player.TalkView{}
	}

	v.talk.SetNight(v.night())

	view := d2player.TalkView{
		Role: v.talk.Speaker.Role,
		Text: v.talk.Text(),
		Rep:  v.standing.Rep,
		Rung: v.dialogue.Rung(v.standing).Name,
	}

	for _, c := range v.talk.Answers() {
		view.Answers = append(view.Answers, c.Text)
	}

	// A node with nothing to answer still needs a way out.
	if len(view.Answers) == 0 {
		view.Answers = []string{d2player.TalkLeave}
	}

	return true, view
}

// Answer takes answer i and applies what it does.
func (v *Game) Answer(i int) error {
	if v.talk == nil || v.talk.Done() {
		return d2dialogue.ErrOver
	}

	v.talk.SetNight(v.night())

	if len(v.talk.Answers()) == 0 {
		v.EndTalk()
		return nil
	}

	// T5: an answer the kit cannot honour is refused before the standing
	// moves or the minutes are spent (the smith has nothing to mend).
	peek, err := v.talk.Peek(i)
	if err != nil {
		return err
	}

	if err := v.canBarter(peek.Effects.Mend); err != nil {
		return err
	}

	effects, err := v.talk.Choose(i)
	if err != nil {
		return err
	}

	v.applyTalk(effects)

	// J1: where the answer led.
	if !v.talk.Done() {
		v.note("node:" + v.talk.NodeID)
	}

	// The minutes an answer costs run the world: he can come out of an hour's
	// labour dead of thirst or in a fight. Either ends the talk (review
	// finding); saveKit already refuses a dead hero.
	if v.talk.Done() || !v.alive() || (v.combat != nil && v.combat.Fighting()) {
		v.EndTalk()
	}

	v.saveKit()

	return nil
}

// applyTalk gives the meters and the clock what an answer did. Water from the
// trough and bread for labour are the first things in a shipped build that
// fill a meter back up -- S1 §8.2's ladder begins with "water at the well".
func (v *Game) applyTalk(e d2dialogue.Effects) {
	if v.meters != nil {
		for kind, amount := range map[string]float64{"food": e.Food, "water": e.Water} {
			if amount <= 0 {
				continue
			}

			if err := v.meters.Consume(kind, amount); err != nil {
				v.Errorf("talk: %v", err)
			}
		}
	}

	// T5: what he is handed, or mended, for the work.
	v.barter(e.Give, e.Mend)

	// T6: hours inside the palisade -- no pack arrives, nothing new sees him.
	if e.Shelter {
		if v.spawns != nil {
			v.spawns.SetSheltered(true)
			defer v.spawns.SetSheltered(false)
		}

		if v.notice != nil {
			v.notice.SetHidden(true)
			defer v.notice.SetHidden(false)
		}
	}

	// Labour takes the time it takes: the world moves by exactly those
	// minutes (clock, meters, light, spawns), paid the way a paced round is --
	// and he is LABOURING while it does (T7), so a pack that arrives mid-dig
	// catches him head-down (D8 §9). Asleep in the byre he is not at work.
	stance := d2world.ActivityLabour
	if e.Shelter {
		stance = ""
	}

	v.spendMinutes(e.Minutes, e.Rest, stance)

	if e.Rep != 0 || e.ToFloor {
		v.Infof("village: standing %d (%s)", v.standing.Rep, v.dialogue.Rung(v.standing).ID)
	}
}

// spendMinutes moves the world by minutes, ten at a time, taking rest off
// fatigue as they pass rather than all at the end -- so a tired man is not
// Shaken or hurt by neglect while he sleeps (T6 review finding) -- and
// stopping if he dies or a fight opens partway (T7: a fight is not waited
// out). stance, when set, is what he is doing for those minutes; it is put
// back afterwards unless the fight has since set its own.
func (v *Game) spendMinutes(minutes, rest float64, stance d2world.Activity) {
	if minutes <= 0 || v.worldClock == nil || v.worldClock.Rate() <= 0 {
		return
	}

	if stance != "" && v.meters != nil {
		before := v.meters.Activity()
		v.meters.SetActivity(stance)

		defer func() {
			// A FIGHT THAT OPENED PARTWAY OWNS THE STANCE NOW. It remembered
			// ours (activityBeforeFight) and will put THAT back when it ends --
			// so hand it what he was doing before the work instead, and leave the
			// fight's own labour alone. The review of T7's first version traced
			// both failures of the naive restore: after one caught forage every
			// later fight opened "caught-foraging", and a fight that opened
			// mid-dig was reset to idle cost while it ran.
			if v.inFight() {
				if v.activityBeforeFight == stance {
					v.activityBeforeFight = before
				}

				return
			}

			if v.meters.Activity() == stance {
				v.meters.SetActivity(before)
			}
		}()
	}

	for left := minutes; left > 0 && v.alive() && !v.inFight(); {
		// The rate is read each step: it is the stage's (the night runs
		// slower), so a sleep or a labour that crosses dawn or nightfall pays
		// each step at the rate in force when it is taken -- exactly step
		// minutes -- not at the rate it began with (history item 117).
		step := math.Min(spendStepMinutes, left)

		rate := v.worldClock.Rate()
		if rate <= 0 {
			return
		}

		v.advanceWorld(step / rate)
		left -= step

		if rest > 0 && v.meters != nil && v.alive() {
			if err := v.meters.Consume("rest", rest*step/minutes); err != nil {
				v.Errorf("talk: %v", err)
			}
		}
	}
}

// EndTalk walks away.
func (v *Game) EndTalk() {
	if v.talk != nil {
		v.talk.Leave()
	}

	v.talk = nil
}

// villageProvider is the "village" harness system.
type villageProvider struct{ v *Game }

func (p villageProvider) HarnessName() string { return "village" }

func (p villageProvider) HarnessState() map[string]interface{} {
	v := p.v
	state := map[string]interface{}{"bound": v.standing != nil}

	if v.standing == nil {
		return state
	}

	state["rep"] = v.standing.Rep
	state["rung"] = v.dialogue.Rung(v.standing).ID
	state["flags"] = append([]string{}, v.standing.Flags...)
	state["talking"] = v.talk != nil
	state["watch_stood"] = v.watchStood
	state["land_gathered"] = v.land.Gathered
	state["land_left"] = v.LandLeft()

	if v.talk != nil {
		state["speaker"] = v.talk.Speaker.ID
		state["node"] = v.talk.NodeID
	}

	return state
}

func (p villageProvider) HarnessSettableFields() []string {
	return []string{"rep", "rite_radius", "seen_radius", "watch_radius"}
}

// HarnessSet stands the village at a number, the same test-setup shape as
// grant_xp: a script reaches a rung without playing the days to it.
func (p villageProvider) HarnessSet(field string, value interface{}) error {
	v := p.v

	// M4.7 step 4's radii, so a script can stand the church and the watching
	// village where the map put the bodies -- and J1's watch radius, so a
	// script can break a promised watch wherever the night finds him.
	if field == "rite_radius" || field == "seen_radius" || field == "watch_radius" {
		f, ok := value.(float64)
		if !ok || f <= 0 || v.dialogue == nil {
			return fmt.Errorf("%s wants a positive number, got %v", field, value)
		}

		switch field {
		case "rite_radius":
			v.dialogue.Village.RiteRadius = f
		case "seen_radius":
			v.dialogue.Village.SeenRadius = f
		default:
			v.dialogue.Village.WatchRadius = f
		}

		return nil
	}

	if field != "rep" {
		return fmt.Errorf("village has no settable field %q", field)
	}

	f, ok := value.(float64)
	if !ok || v.standing == nil {
		return fmt.Errorf("rep wants a number and a bound village, got %v", value)
	}

	v.standing.Rep = 0
	v.dialogue.Move(v.standing, int(f))

	return nil
}

// spendStepMinutes is how finely spent minutes are paid out: fine enough that a
// fight opening partway stops the rest of them.
const spendStepMinutes = 10.0
