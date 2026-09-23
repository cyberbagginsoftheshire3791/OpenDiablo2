package d2world

import (
	"fmt"
	"math"
	"sort"
)

// M4.7 step 1 (23 Sep 2026): the slain are open bodies, and the stake closes
// one. S1 §6.2 gives every dead body a state -- Fresh (open) / Hasty grave /
// Closed / Risen / Downed; this step builds the first and the third, the
// registry that holds them, and the count the carrion weighting has read
// through a harness stand-in since M4.3b ("open_bodies ... the corpse machine
// drives it when it lands", ask 3). The rising roll is step 2.
//
// ONE RULING TAKEN ON THE RECOMMENDED OPTION (M4.7 note, Q1a): a dead BEAST
// is carrion -- it counts toward the carrion weight and never rises. Only the
// bodies of men are doors. So a corpse has a class, from its spawn row.

// CorpseState is a body's place in S1 §6.2's machine (step 1: two of five).
type CorpseState string

// The states step 1 builds.
const (
	CorpseFresh  CorpseState = "fresh"
	CorpseClosed CorpseState = "closed"
)

// CorpseClass is whose body it is.
type CorpseClass string

// The classes (M4.7 Q1a).
const (
	CorpseHuman CorpseClass = "human"
	CorpseBeast CorpseClass = "beast"
)

// Corpse is one body where it fell.
type Corpse struct {
	ID    string
	Row   string
	Class CorpseClass
	State CorpseState
	X, Y  float64
}

// Corpses is the registry. It is world state for one session: M4.6 owns the
// world save, and "load last save means a new dawn" (12 Sep), so bodies do not
// survive a reload.
type Corpses struct {
	byID    map[string]*Corpse
	order   []string
	isHuman func(row string) bool
	changed func(openDelta int)
}

// NewCorpses makes a registry. isHuman classes a body by its spawn row;
// changed hears every change to the count of open bodies (the carrion
// weighting's input). Either may be nil.
func NewCorpses(isHuman func(row string) bool, changed func(openDelta int)) *Corpses {
	return &Corpses{byID: map[string]*Corpse{}, isHuman: isHuman, changed: changed}
}

// Fall records a body where it fell. A second fall of the same id is ignored.
func (c *Corpses) Fall(id, row string, x, y float64) *Corpse {
	return c.fall(id, row, x, y, false)
}

// FallHuman records a body that is a man's whatever its row says -- the
// placed dead of Night 1 (M4.7 Q2a), which no spawn row produced.
func (c *Corpses) FallHuman(id string, x, y float64) *Corpse {
	return c.fall(id, "", x, y, true)
}

func (c *Corpses) fall(id, row string, x, y float64, human bool) *Corpse {
	if b, ok := c.byID[id]; ok {
		return b
	}

	class := CorpseBeast
	if human || (c.isHuman != nil && c.isHuman(row)) {
		class = CorpseHuman
	}

	b := &Corpse{ID: id, Row: row, Class: class, State: CorpseFresh, X: x, Y: y}
	c.byID[id] = b
	c.order = append(c.order, id)

	if c.changed != nil {
		c.changed(1)
	}

	return b
}

// Has reports a registered body.
func (c *Corpses) Has(id string) bool { _, ok := c.byID[id]; return ok }

// Open is how many bodies lie open -- beasts included: a carcass is carrion.
func (c *Corpses) Open() int {
	n := 0

	for _, b := range c.byID {
		if b.State == CorpseFresh {
			n++
		}
	}

	return n
}

// Nearest is the nearest body within r tiles of (x, y) that keep accepts, or
// nil. Ties go to the earliest fallen, so the answer is the same every run.
func (c *Corpses) Nearest(x, y, r float64, keep func(*Corpse) bool) *Corpse {
	var (
		best  *Corpse
		bestD = math.Inf(1)
	)

	for _, id := range c.order {
		b := c.byID[id]
		if keep != nil && !keep(b) {
			continue
		}

		if d := math.Hypot(b.X-x, b.Y-y); d <= r && d < bestD {
			best, bestD = b, d
		}
	}

	return best
}

// Close closes an open body (the stake, step 1). It reports whether it did.
func (c *Corpses) Close(id string) bool {
	b, ok := c.byID[id]
	if !ok || b.State != CorpseFresh {
		return false
	}

	b.State = CorpseClosed

	if c.changed != nil {
		c.changed(-1)
	}

	return true
}

// All is every body, in the order they fell (copies).
func (c *Corpses) All() []Corpse {
	out := make([]Corpse, 0, len(c.order))
	for _, id := range c.order {
		out = append(out, *c.byID[id])
	}

	return out
}

// HarnessName is the "corpses" system.
func (c *Corpses) HarnessName() string { return "corpses" }

// HarnessState reports the bodies and their counts.
func (c *Corpses) HarnessState() map[string]interface{} {
	bodies := make([]map[string]interface{}, 0, len(c.order))
	counts := map[string]int{}

	for _, b := range c.All() {
		bodies = append(bodies, map[string]interface{}{
			"id": b.ID, "row": b.Row, "class": string(b.Class), "state": string(b.State), "x": b.X, "y": b.Y,
		})
		counts[string(b.State)+"_"+string(b.Class)]++
	}

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	state := map[string]interface{}{"bodies": bodies, "open": c.Open(), "total": len(c.order)}
	for _, k := range keys {
		state[k] = counts[k]
	}

	return state
}

// HarnessSettableFields: none. Bodies fall and close through the game.
func (c *Corpses) HarnessSettableFields() []string { return nil }

// HarnessSet refuses: there is nothing to set.
func (c *Corpses) HarnessSet(field string, _ interface{}) error {
	return fmt.Errorf("corpses has no settable field %q", field)
}
