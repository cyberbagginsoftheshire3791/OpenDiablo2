package d2gamescreen

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// T7, forage (23 Sep 2026): the gathering verb. K sends him head-down into the
// scrub for half an hour; he comes back with branches, from a stock the land
// does not renew (S1 §8.3: "finite local resources... the map's stocks do not
// regenerate within the run").
//
// IT ALSO MAKES A BUILT RULE LIVE. D8 §9's caught-head-down branch -- a player
// caught foraging goes LAST, surprised, with no Reaction in round one -- has
// been in the resolver since M4.5 step 4 and reachable only from the harness,
// because nothing in the game ever set the forage stance
// (docs/reachability.md). Foraging sets it for exactly the minutes it takes,
// so a pack that arrives while he is bent over the brush catches him that way.

const (
	forageMinutes = 30.0       // [DIAL] one forage
	forageYield   = 2          // [DIAL] branches it brings back
	forageStock   = 12         // [DIAL] branches the land holds for the run
	forageItem    = "branches" // what it gathers
)

// land is what the country around has given him (saved beside his save).
type land struct {
	Gathered int `json:"gathered"`
}

var (
	errForageBusy   = errors.New("not now")
	errForageFight  = errors.New("not in a fight")
	errLandBare     = errors.New("nothing left to gather near here")
	errCaughtAtWork = errors.New("caught head-down")
)

// bindLand reads what he has gathered; a hero without it has gathered nothing.
func (v *Game) bindLand(raw json.RawMessage) {
	v.land = land{}

	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &v.land); err != nil {
			v.Errorf("land: %v", err)

			v.land = land{}
		}
	}
}

func (v *Game) landJSON() json.RawMessage {
	data, err := json.Marshal(v.land)
	if err != nil {
		return nil
	}

	return data
}

// LandLeft is what the land still holds.
func (v *Game) LandLeft() int {
	if left := forageStock - v.land.Gathered; left > 0 {
		return left
	}

	return 0
}

// Forage spends half an hour head-down gathering. A fight that finds him at it
// opens with him caught (the resolver reads the stance) and he gathers nothing.
func (v *Game) Forage() error {
	n, err := v.forage()

	if v.gameControls != nil {
		text := fmt.Sprintf(d2player.ForageGathered, n, v.LandLeft())
		if err != nil {
			text = fmt.Sprintf(d2player.ForageRefused, err)
		}

		v.gameControls.SetZoneChangeText(text)
		v.gameControls.ShowZoneChangeText()
		v.gameControls.HideZoneChangeTextAfter(levelNoticeSeconds)
	}

	return err
}

func (v *Game) forage() (int, error) {
	switch {
	case v.kit == nil || v.meters == nil:
		return 0, errForageBusy
	case v.died || !v.alive():
		return 0, errCraftDead
	case v.talk != nil || v.choosingLoadout:
		return 0, errForageBusy
	case v.inFight():
		return 0, errForageFight
	case v.LandLeft() <= 0:
		return 0, errLandBare
	}

	v.spendMinutes(forageMinutes, 0, d2world.ActivityForage)

	if v.inFight() {
		return 0, errCaughtAtWork
	}

	if !v.alive() {
		return 0, errCraftDead
	}

	n := forageYield
	if left := v.LandLeft(); n > left {
		n = left
	}

	if err := v.kit.Give(forageItem, n); err != nil {
		return 0, err
	}

	v.land.Gathered += n
	v.saveKit()

	return n, nil
}
