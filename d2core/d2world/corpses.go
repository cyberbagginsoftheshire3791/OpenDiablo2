package d2world

import (
	"fmt"
	"math"
	"sort"
)

// M4.7 step 1 (23 Sep 2026): the slain are open bodies, and the stake closes
// one. S1 §6.2 gives every dead body a state -- Fresh (open) / Hasty grave /
// Closed / Risen / Downed; step 1 built the first and the third, the registry
// that holds them, and the count the carrion weighting has read through a
// harness stand-in since M4.3b ("open_bodies ... the corpse machine drives it
// when it lands", ask 3). Step 2 adds the hasty grave and the rising (see
// rising.go); Downed is step 3's.
//
// ONE RULING TAKEN ON THE RECOMMENDED OPTION (M4.7 note, Q1a): a dead BEAST
// is carrion -- it counts toward the carrion weight and never rises. Only the
// bodies of men are doors. So a corpse has a class, from its spawn row.

// CorpseState is a body's place in S1 §6.2's machine.
type CorpseState string

// The states built so far (Downed is step 3's).
const (
	// CorpseFresh is an open body: carrion to beasts, a door to the dead.
	CorpseFresh CorpseState = "fresh"
	// CorpseHasty is a hasty grave: no longer open, but a man's still rolls
	// to rise at a reduced weight (S1 §6.2).
	CorpseHasty CorpseState = "hasty"
	// CorpseClosed is staked (or rited): it never rises.
	CorpseClosed CorpseState = "closed"
	// CorpseRisen has left where it lay. Step 3 stands it up to fight.
	CorpseRisen CorpseState = "risen"
	// CorpseDowned is a risen man cut down, or laid down at first light (S1
	// §6.2): not open -- no carrion -- and a door that stands again. At
	// night he stands once his window runs out; by day he waits, and stands
	// certainly in the next deep-night band (M4.7 Q7a). Only a stake keeps
	// him down.
	CorpseDowned CorpseState = "downed"
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

	// DownedAt is the world minute he went down (CorpseDowned only).
	DownedAt float64
}

// Door reports a body that can still rise: a man's, open or in a hasty grave.
func (b *Corpse) Door() bool {
	return b.Class == CorpseHuman && (b.State == CorpseFresh || b.State == CorpseHasty || b.State == CorpseDowned)
}

// Corpses is the registry. It is world state for one session: M4.6 owns the
// world save, and "load last save means a new dawn" (12 Sep), so bodies do not
// survive a reload.
type Corpses struct {
	byID    map[string]*Corpse
	order   []string
	risenAs map[string]string // every member a body has walked as -> the body
	walker  map[string]string // a risen body -> the member it walks as NOW
	last    map[string]string // a body -> the last member it walked as, standing or fallen
	now     func() float64    // world minutes, for DownedAt (nil reads 0)
	isHuman func(row string) bool
	changed func(openDelta int)
}

// NewCorpses makes a registry. isHuman classes a body by its spawn row;
// changed hears every change to the count of open bodies (the carrion
// weighting's input). Either may be nil.
func NewCorpses(isHuman func(row string) bool, changed func(openDelta int)) *Corpses {
	return &Corpses{byID: map[string]*Corpse{}, risenAs: map[string]string{}, walker: map[string]string{}, last: map[string]string{}, isHuman: isHuman, changed: changed}
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

	// A risen man who falls -- cut down, or laid down at first light -- is
	// the SAME body, Downed where he fell (S1 §6.2; M4.7 step 3b), not a new
	// one. Only once: a second fall of the same member finds him lying.
	if cid, ok := c.risenAs[id]; ok {
		b := c.byID[cid]
		if b != nil && b.State == CorpseRisen && c.walker[cid] == id {
			b.State, b.X, b.Y, b.DownedAt = CorpseDowned, x, y, c.minutes()
			delete(c.walker, cid)
		}

		return b
	}

	class := CorpseBeast
	if human || (c.isHuman != nil && c.isHuman(row)) {
		class = CorpseHuman
	}

	b := &Corpse{ID: id, Row: row, Class: class, State: CorpseFresh, X: x, Y: y}
	c.byID[id] = b
	c.order = append(c.order, id)

	c.open(1)

	return b
}

func (c *Corpses) open(delta int) {
	if c.changed != nil {
		c.changed(delta)
	}
}

// Has reports a registered body.
func (c *Corpses) Has(id string) bool { _, ok := c.byID[id]; return ok }

// Get is one body as it is now (a copy).
func (c *Corpses) Get(id string) (Corpse, bool) {
	b, ok := c.byID[id]
	if !ok {
		return Corpse{}, false
	}

	return *b, true
}

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

// Close closes a body: the stake through an open body or a hasty grave (R1
// §2, the stake pins the corpse in the grave). It reports whether it did.
func (c *Corpses) Close(id string) bool {
	b, ok := c.byID[id]
	if !ok || (b.State != CorpseFresh && b.State != CorpseHasty && b.State != CorpseDowned) {
		return false
	}

	wasOpen := b.State == CorpseFresh
	b.State = CorpseClosed

	if wasOpen {
		c.open(-1)
	}

	return true
}

// Bury puts an open body in a hasty grave (step 2). A carcass may be buried
// too: it stops being carrion, and it never rose anyway.
func (c *Corpses) Bury(id string) bool {
	b, ok := c.byID[id]
	if !ok || b.State != CorpseFresh {
		return false
	}

	b.State = CorpseHasty
	c.open(-1)

	return true
}

// Rise marks a door risen (step 2's roll). Closed bodies and carcasses never
// rise; it reports whether this one did.
func (c *Corpses) Rise(id string) bool {
	b, ok := c.byID[id]
	if !ok || !b.Door() {
		return false
	}

	wasOpen := b.State == CorpseFresh
	b.State = CorpseRisen

	if wasOpen {
		c.open(-1)
	}

	return true
}

// Raised records which standing member a risen body walks as (step 3), so
// its fall comes back to the same body.
func (c *Corpses) Raised(bodyID, memberID string) {
	if b, ok := c.byID[bodyID]; ok && b.State == CorpseRisen && memberID != "" {
		c.risenAs[memberID] = bodyID
		c.walker[bodyID] = memberID
		c.last[bodyID] = memberID
	}
}

// DownedMember reports a member whose body lies Downed now: he fell, and has
// neither stood again nor been staked (M4.7 step 3b).
func (c *Corpses) DownedMember(memberID string) bool {
	cid, ok := c.risenAs[memberID]
	if !ok {
		return false
	}

	b := c.byID[cid]

	return b != nil && b.State == CorpseDowned && c.last[cid] == memberID
}

// LastWalker is the last member a body walked as, standing or fallen, or "".
// When a Downed man stands again the fallen one's remains are taken off the
// map, so the body is never drawn twice.
func (c *Corpses) LastWalker(bodyID string) string { return c.last[bodyID] }

// SetClock attaches the world minutes a Downed body's window is measured on.
func (c *Corpses) SetClock(now func() float64) { c.now = now }

func (c *Corpses) minutes() float64 {
	if c.now == nil {
		return 0
	}

	return c.now()
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
