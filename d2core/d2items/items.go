// Package d2items is Strigoi's own item model (U1 §5): what a man carries,
// what it does in a fight, and what wears. It adopts neither d2inventory nor
// diablo2item -- ruling A12 -- because a D2 item is an affix roll on a grid and
// a Strigoi item is a period object with a make and a condition.
//
// It is a LEAF package: no ebiten, no logging, no RNG, no world. The resolver
// asks it arithmetic questions (how much does this blade bite, how much does
// this mail take off a cut) and it answers them; who holds the kit, when it is
// saved and when a torch burns belong to other packages.
package d2items

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Kind is what an item is for.
type Kind string

// The four kinds U1 §5.1 signs, and story items (the ak börk), which have no
// stat and cannot be taken off.
const (
	KindWeapon Kind = "weapon"
	KindArmour Kind = "armour"
	KindLight  Kind = "light"
	KindTool   Kind = "tool"
	KindStory  Kind = "story"
)

// DamageClass is how a weapon hurts, and what armour is rated against.
type DamageClass string

// E3 §4's three classes.
const (
	Cut    DamageClass = "cut"
	Thrust DamageClass = "thrust"
	Blunt  DamageClass = "blunt"
)

// Slot is where an item is worn (U1 §5.2). The pack is a list, not a slot.
type Slot string

// The worn slots, in the order the kit panel lists them.
const (
	SlotMain  Slot = "main"
	SlotOff   Slot = "off"
	SlotBody  Slot = "body"
	SlotHead  Slot = "head"
	SlotBelt1 Slot = "belt1"
	SlotBelt2 Slot = "belt2"
)

// WornSlots is every worn slot in display order.
func WornSlots() []Slot {
	return []Slot{SlotMain, SlotOff, SlotBody, SlotHead, SlotBelt1, SlotBelt2}
}

// Reactions a weapon implies (R2 §3 bullet 6; E3 §4's column).
const (
	ReactionRiposte     = "riposte"
	ReactionBrace       = "brace"
	ReactionOpportunity = "opportunity"
	ReactionNone        = "none"
)

// The two starting loadouts (Squads on Screen §8, ruled 11 Sep 2026): the one
// off-hand commitment, chosen once at start.
const (
	LoadoutSwordAndBoard = "sword-and-board"
	LoadoutTorchAndBlade = "torch-and-blade"
)

// Item is one catalogue entry: the object, not a particular one of it.
type Item struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Kind     Kind    `json:"kind"`
	Fits     string  `json:"slot"` // main | off | body | head | belt | pack
	WeightKg float64 `json:"weight_kg"`
	Stack    int     `json:"stack,omitempty"`

	Weapon *Weapon    `json:"weapon,omitempty"`
	Armour *Armour    `json:"armour,omitempty"`
	Light  *LightSpec `json:"light,omitempty"`
	Tool   *Tool      `json:"tool,omitempty"`
}

// Weapon is how a weapon fights. Every number is a [DIAL] (E3 §4 gives the
// relations, never the numbers).
type Weapon struct {
	Hands    int         `json:"hands"`
	Reach    int         `json:"reach"`
	Class    DamageClass `json:"class"`
	Min      int         `json:"min"`
	Max      int         `json:"max"`
	Reaction string      `json:"reaction"`

	// VsMail is the fraction of a mail coat's reduction this weapon ignores:
	// a mace's 0.5 is E3's "heavy vs mail -- blunt ignores part of it".
	VsMail float64 `json:"vs_mail,omitempty"`

	// Ranged weapons exist as items; shooting is a v0 non-goal (R2 §3).
	Ranged bool `json:"ranged,omitempty"`
}

// Armour is how a worn piece protects.
type Armour struct {
	Reduction map[DamageClass]int `json:"reduction,omitempty"`
	Points    int                 `json:"points"`

	// CritOnly armour (the helmet) protects only against a crit: E3's
	// "reduces head crits".
	CritOnly bool `json:"crit_only,omitempty"`

	// Block is the shield: it steps one blow's band down a step, once a round.
	Block bool `json:"block,omitempty"`

	Repair string `json:"repair,omitempty"` // smith | field | none
}

// LightSpec is a carried light. The light MODEL (d2world.Light) owns burn
// while a torch is lit; the kit keeps what is left while it is not.
type LightSpec struct {
	Radius      float64 `json:"radius"`
	BurnMinutes float64 `json:"burn_minutes"`
}

// Tool is the verb an item enables.
type Tool struct {
	Verb string `json:"verb"`
}

// Catalog is the validated item table.
type Catalog struct {
	byID     map[string]*Item
	loadouts map[string]Loadout
	defaultL string
}

// Loadout is a starting kit by slot: item ids, and "id:count" for a stack.
type Loadout struct {
	Main  string   `json:"main"`
	Off   string   `json:"off"`
	Body  string   `json:"body"`
	Head  string   `json:"head"`
	Belt1 string   `json:"belt1"`
	Belt2 string   `json:"belt2"`
	Pack  []string `json:"pack"`
}

type document struct {
	Items          []Item             `json:"items"`
	Loadouts       map[string]Loadout `json:"loadouts"`
	DefaultLoadout string             `json:"default_loadout"`
}

// Load parses and validates an item table.
func Load(data []byte) (*Catalog, error) {
	var doc document

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode items: %w", err)
	}

	if len(doc.Items) == 0 {
		return nil, fmt.Errorf("item table has no items")
	}

	c := &Catalog{byID: map[string]*Item{}, loadouts: map[string]Loadout{}}

	for i := range doc.Items {
		it := doc.Items[i]
		it.ID = strings.ToLower(strings.TrimSpace(it.ID))

		if err := validate(&it); err != nil {
			return nil, fmt.Errorf("item %d (%q): %w", i, it.ID, err)
		}

		if _, dup := c.byID[it.ID]; dup {
			return nil, fmt.Errorf("duplicate item id %q", it.ID)
		}

		c.byID[it.ID] = &it
	}

	for name, l := range doc.Loadouts {
		if err := c.checkLoadout(l); err != nil {
			return nil, fmt.Errorf("loadout %q: %w", name, err)
		}

		c.loadouts[name] = l
	}

	c.defaultL = doc.DefaultLoadout
	if _, ok := c.loadouts[c.defaultL]; !ok {
		return nil, fmt.Errorf("default_loadout %q is not a loadout", c.defaultL)
	}

	return c, nil
}

func validate(it *Item) error {
	if it.ID == "" || strings.TrimSpace(it.Name) == "" {
		return fmt.Errorf("an item needs an id and a name")
	}

	switch it.Fits {
	case "main", "off", "body", "head", "belt", "pack":
	default:
		return fmt.Errorf("slot %q is not main, off, body, head, belt or pack", it.Fits)
	}

	switch it.Kind {
	case KindWeapon:
		w := it.Weapon
		if w == nil || w.Min < 1 || w.Max < w.Min || (w.Hands != 1 && w.Hands != 2) || w.Reach < 1 {
			return fmt.Errorf("a weapon needs hands 1|2, reach >= 1 and 1 <= min <= max")
		}

		switch w.Class {
		case Cut, Thrust, Blunt:
		default:
			return fmt.Errorf("damage class %q", w.Class)
		}

		switch w.Reaction {
		case ReactionRiposte, ReactionBrace, ReactionOpportunity, ReactionNone:
		default:
			return fmt.Errorf("reaction %q", w.Reaction)
		}
	case KindArmour:
		if it.Armour == nil || it.Armour.Points < 0 {
			return fmt.Errorf("armour needs an armour block with points >= 0")
		}
	case KindLight:
		if it.Light == nil || it.Light.BurnMinutes <= 0 {
			return fmt.Errorf("a light needs burn_minutes > 0")
		}
	case KindTool:
		if it.Tool == nil || it.Tool.Verb == "" {
			return fmt.Errorf("a tool needs a verb")
		}
	case KindStory:
	default:
		return fmt.Errorf("kind %q", it.Kind)
	}

	return nil
}

func (c *Catalog) checkLoadout(l Loadout) error {
	worn := map[Slot]string{
		SlotMain: l.Main, SlotOff: l.Off, SlotBody: l.Body,
		SlotHead: l.Head, SlotBelt1: l.Belt1, SlotBelt2: l.Belt2,
	}

	for slot, id := range worn {
		if id == "" {
			continue
		}

		it, ok := c.byID[id]
		if !ok {
			return fmt.Errorf("%s: no item %q", slot, id)
		}

		if !fitsSlot(it, slot) {
			return fmt.Errorf("%s: %q does not fit there", slot, id)
		}
	}

	for _, entry := range l.Pack {
		id, _ := splitStack(entry)
		if _, ok := c.byID[id]; !ok {
			return fmt.Errorf("pack: no item %q", id)
		}
	}

	return nil
}

// DefaultLoadout is the loadout an old save or an unset choice gets.
func (c *Catalog) DefaultLoadout() string { return c.defaultL }

// fitsSlot is the one place the slot rule lives.
func fitsSlot(it *Item, slot Slot) bool {
	switch slot {
	case SlotMain:
		return it.Fits == "main"
	case SlotOff:
		return it.Fits == "off"
	case SlotBody:
		return it.Fits == "body"
	case SlotHead:
		return it.Fits == "head"
	case SlotBelt1, SlotBelt2:
		return it.Fits == "belt"
	}

	return false
}

func splitStack(entry string) (id string, count int) {
	id, n, found := strings.Cut(entry, ":")
	if !found {
		return id, 1
	}

	count = 0
	for _, r := range n {
		if r < '0' || r > '9' {
			return id, 1
		}

		count = count*10 + int(r-'0')
	}

	if count < 1 {
		count = 1
	}

	return id, count
}
