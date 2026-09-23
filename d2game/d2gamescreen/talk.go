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

	// Labour takes the time it takes: the world moves by exactly those
	// minutes (clock, meters, light, spawns), paid the way a paced round is.
	if e.Minutes > 0 && v.worldClock != nil {
		if rate := v.worldClock.Rate(); rate > 0 {
			v.advanceWorld(e.Minutes / rate)
		}
	}

	if e.Rep != 0 || e.ToFloor {
		v.Infof("village: standing %d (%s)", v.standing.Rep, v.dialogue.Rung(v.standing).ID)
	}
}

// EndTalk walks away.
func (v *Game) EndTalk() {
	if v.talk != nil {
		v.talk.Leave()
	}

	v.talk = nil
}

// dawnWatch settles a promised watch; called on the night-to-dawn edge he
// lived through.
func (v *Game) dawnWatch() {
	if v.dialogue == nil || v.standing == nil {
		return
	}

	if gained := v.dialogue.DawnWatch(v.standing); gained != 0 {
		if v.gameControls != nil {
			v.gameControls.SetZoneChangeText(fmt.Sprintf(d2player.TalkWatchKept, gained))
			v.gameControls.ShowZoneChangeText()
			v.gameControls.HideZoneChangeTextAfter(levelNoticeSeconds)
		}

		v.saveKit()
	}
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

	if v.talk != nil {
		state["speaker"] = v.talk.Speaker.ID
		state["node"] = v.talk.NodeID
	}

	return state
}

func (p villageProvider) HarnessSettableFields() []string { return []string{"rep"} }

// HarnessSet stands the village at a number, the same test-setup shape as
// grant_xp: a script reaches a rung without playing the days to it.
func (p villageProvider) HarnessSet(field string, value interface{}) error {
	v := p.v
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
