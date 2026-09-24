package d2gamescreen

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// T5, making and mending (23 Sep 2026): the game screen's half. The kit panel
// lists the recipes under his pack; a click makes one from what he carries and
// charges the world its minutes, the way labour for the village does.

// recipesPath is the shipped recipe table.
const recipesPath = "/data/strigoi/recipes.json"

var (
	errCraftInFight = errors.New("not in a fight")
	errCraftDead    = errors.New("the dead make nothing")
	errCraftBusy    = errors.New("not now")
)

// Recipes is the kit panel's list: every recipe, in the table's order, with
// what it costs and whether he could make it now.
func (v *Game) Recipes() []d2player.RecipeRow {
	if v.recipes == nil || v.kit == nil {
		return nil
	}

	rows := make([]d2player.RecipeRow, 0, len(v.recipes.Recipes))

	for i := range v.recipes.Recipes {
		r := &v.recipes.Recipes[i]

		var cost []string

		ids := make([]string, 0, len(r.Inputs))
		for id := range r.Inputs {
			ids = append(ids, id)
		}

		sort.Strings(ids)

		// The item ids are the short nouns (branches, feathers, arrowheads,
		// wire); the full names ran the line off the panel (T9, seen in the kit
		// playtest's screenshot).
		for _, id := range ids {
			cost = append(cost, fmt.Sprintf(d2player.KitRecipeInput, id, r.Inputs[id]))
		}

		rows = append(rows, d2player.RecipeRow{
			ID:    r.ID,
			Text:  fmt.Sprintf(d2player.KitRecipeLine, r.Name, r.Minutes),
			Cost:  strings.Join(cost, ", "),
			Ready: v.recipes.Check(v.kit, r.ID) == nil && !v.inFight() && v.talk == nil,
		})
	}

	return rows
}

// Craft makes or mends a recipe from his kit and pays its minutes.
func (v *Game) Craft(id string) error {
	switch {
	case v.recipes == nil || v.kit == nil:
		return errors.New("nothing to make")
	case v.died || !v.alive():
		return errCraftDead
	case v.inFight():
		return errCraftInFight
	case v.talk != nil || v.choosingLoadout:
		return errCraftBusy
	}

	r, err := v.recipes.Make(v.kit, id)
	if err != nil {
		return err
	}

	// The work takes the time it takes; the world moves by exactly that, and
	// he is labouring while it does (T7: caught at the bench is caught).
	v.spendMinutes(r.Minutes, 0, d2world.ActivityLabour)

	v.note("crafted:" + r.ID)
	v.saveKit()

	return nil
}

// barter applies what a villager hands over or mends (T5's half of a talk).
func (v *Game) barter(give map[string]int, mend string) {
	if v.kit == nil {
		return
	}

	for id, n := range give {
		if err := v.kit.Give(id, n); err != nil {
			v.Errorf("barter: %v", err)
		}
	}

	if mend != "" {
		if _, err := v.kit.Mend(d2items.Slot(mend), 1<<20, 1); err != nil {
			v.Errorf("barter: %v", err)
		}
	}
}

// canBarter refuses, before anything moves, an answer the kit cannot honour:
// a smith's mend with nothing to mend.
func (v *Game) canBarter(mend string) error {
	if mend == "" {
		return nil
	}

	if v.kit == nil {
		return d2items.ErrNothingToMend
	}

	_, err := v.kit.CanMend(d2items.Slot(mend), 1)

	return err
}
