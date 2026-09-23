// Package d2craft is Strigoi's making and mending (T5, 23 Sep 2026).
//
// THE SHAPE IS SIGNED, THE RECIPES ARE NOT. S1 §8.3: arrows do not restock
// (Cut Off), so FLETCHING is the first crafting verb; the burial kit (stake,
// needle, briar, millet, spade, mallet) is craftable or obtainable at the
// village; armour is repaired by the village smith (by reputation) or a field
// kit, "slower, worse"; and there is no coin -- barter in goods. Period craft
// capability is UNKNOWN -> E5, so "the slice uses two abstract recipes"; this
// has three, every input, output and minute a [DIAL] in
// data/strigoi/recipes.json.
//
// A recipe's materials come from the village (d2dialogue's give), not from a
// gathering verb -- there is none yet.
package d2craft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
)

// Mend is a recipe that repairs worn armour instead of making a thing.
type Mend struct {
	Slot   d2items.Slot `json:"slot"`
	Points int          `json:"points"`
	Cap    float64      `json:"cap"` // fraction of full points it can reach
}

// Recipe is one thing he can make or mend.
type Recipe struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Inputs  map[string]int `json:"inputs"`
	Tools   []string       `json:"tools,omitempty"`
	Outputs map[string]int `json:"outputs,omitempty"`
	Mend    *Mend          `json:"mend,omitempty"`
	Minutes float64        `json:"minutes"`
}

// Book is the validated recipe table.
type Book struct {
	Recipes []Recipe `json:"recipes"`

	byID map[string]*Recipe
}

// Load parses and validates the recipes against the item catalogue.
func Load(data []byte, cat *d2items.Catalog) (*Book, error) {
	var b Book

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("decode recipes: %w", err)
	}

	b.byID = map[string]*Recipe{}

	known := func(where, id string, n int) error {
		if !cat.Has(id) {
			return fmt.Errorf("%s: no item %q", where, id)
		}

		if n <= 0 {
			return fmt.Errorf("%s: %s x%d -- counts are positive", where, id, n)
		}

		return nil
	}

	for i := range b.Recipes {
		r := &b.Recipes[i]
		if r.ID == "" || r.Name == "" || r.Minutes <= 0 {
			return nil, fmt.Errorf("recipe %d needs an id, a name and minutes > 0", i)
		}

		if _, dup := b.byID[r.ID]; dup {
			return nil, fmt.Errorf("two recipes named %q", r.ID)
		}

		// A recipe makes something or mends something -- exactly one.
		if (len(r.Outputs) == 0) == (r.Mend == nil) {
			return nil, fmt.Errorf("recipe %s must make something or mend something, not both or neither", r.ID)
		}

		// Something in, and only MATERIALS in: a recipe must never eat a story
		// item, a torch's burn, a worn-out coat -- or the knife it needs.
		if len(r.Inputs) == 0 {
			return nil, fmt.Errorf("recipe %s takes nothing: a recipe is not a free source of goods", r.ID)
		}

		for id, n := range r.Inputs {
			if err := known(r.ID+" input", id, n); err != nil {
				return nil, err
			}

			if it := cat.Item(id); it.Kind != d2items.KindMaterial {
				return nil, fmt.Errorf("recipe %s: %q is not a material", r.ID, id)
			}
		}

		for id, n := range r.Outputs {
			if err := known(r.ID+" output", id, n); err != nil {
				return nil, err
			}
		}

		for _, id := range r.Tools {
			if err := known(r.ID+" tool", id, 1); err != nil {
				return nil, err
			}

			if _, eaten := r.Inputs[id]; eaten {
				return nil, fmt.Errorf("recipe %s both needs and eats %q", r.ID, id)
			}
		}

		if m := r.Mend; m != nil && (m.Points <= 0 || m.Cap <= 0 || m.Cap > 1 || !d2items.IsArmourSlot(m.Slot)) {
			return nil, fmt.Errorf("recipe %s: a mend needs a slot, points > 0 and a cap in (0, 1]", r.ID)
		}

		b.byID[r.ID] = r
	}

	return &b, nil
}

// Recipe is one recipe by id, or nil.
func (b *Book) Recipe(id string) *Recipe { return b.byID[id] }

// Refusals.
var (
	ErrUnknown = errors.New("no such recipe")
	ErrNoTool  = errors.New("needs a tool he does not carry")
)

// sortedKeys keeps refusals and costs in a stable order.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// Check reports why a recipe cannot be made from this kit now, or nil.
func (b *Book) Check(k *d2items.Kit, id string) error {
	r := b.byID[id]
	if r == nil {
		return ErrUnknown
	}

	// An unbound kit could take the inputs and then fail to give the outputs.
	if !k.Bound() {
		return d2items.ErrUnboundKit
	}

	for _, t := range r.Tools {
		if !k.Carries(t) {
			return fmt.Errorf("%w: %s", ErrNoTool, t)
		}
	}

	for _, item := range sortedKeys(r.Inputs) {
		if have, need := k.Count(item), r.Inputs[item]; have < need {
			return fmt.Errorf("%w: %s %d of %d", d2items.ErrNotEnough, item, have, need)
		}
	}

	if m := r.Mend; m != nil {
		// A mend that would restore nothing is refused BEFORE it eats
		// anything: dry-run it on the points it would reach.
		if _, err := k.CanMend(m.Slot, m.Cap); err != nil {
			return err
		}
	}

	return nil
}

// Make makes (or mends) a recipe: it takes the inputs, gives the outputs or
// restores the points, and returns the recipe for the caller to charge its
// minutes. It changes nothing if Check refuses.
func (b *Book) Make(k *d2items.Kit, id string) (*Recipe, error) {
	if err := b.Check(k, id); err != nil {
		return nil, err
	}

	r := b.byID[id]

	for _, item := range sortedKeys(r.Inputs) {
		if err := k.Take(item, r.Inputs[item]); err != nil {
			return nil, err // unreachable after Check
		}
	}

	for _, item := range sortedKeys(r.Outputs) {
		if err := k.Give(item, r.Outputs[item]); err != nil {
			return nil, err
		}
	}

	if m := r.Mend; m != nil {
		if _, err := k.Mend(m.Slot, m.Points, m.Cap); err != nil {
			return nil, err
		}
	}

	return r, nil
}
