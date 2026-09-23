package d2items

import (
	"errors"
	"fmt"
	"math"
)

// Make is who made a thing (U1 §5.3). Strigoi has no affixes and no rarity;
// a lord's blade is better than a village one, and that is the whole ladder.
type Make string

// The three makes.
const (
	MakeVillage Make = "village"
	MakeIssue   Make = "issue"
	MakeLord    Make = "lord"
)

// Condition is how worn a thing is.
type Condition string

// The three conditions.
const (
	Sound   Condition = "sound"
	Damaged Condition = "damaged"
	Ruined  Condition = "ruined"
)

// makeFactor scales a weapon's bite and an armour's points. [DIAL]
func makeFactor(m Make) float64 {
	switch m {
	case MakeVillage:
		return 0.8
	case MakeLord:
		return 1.2
	}

	return 1.0
}

// conditionFactor scales a weapon's bite. [DIAL] Weapons do not wear in v0,
// but a found one can be damaged.
func conditionFactor(c Condition) float64 {
	switch c {
	case Damaged:
		return 0.75
	case Ruined:
		return 0.5
	}

	return 1.0
}

// Instance is one particular object: this sabre, this coat of mail.
type Instance struct {
	Item      string    `json:"item"`
	Make      Make      `json:"make"`
	Condition Condition `json:"condition"`

	// Points is what is left of an armour's protection. It is the wear, and
	// Condition follows from it for armour (armourCondition).
	Points int `json:"points,omitempty"`

	// BurnLeft is a torch's minutes while it is NOT lit and out of the light
	// model's hands.
	BurnLeft float64 `json:"burn_left,omitempty"`

	Count int `json:"count,omitempty"`
}

// Kit is one man's gear: six worn slots and a pack that is a list, not a grid.
type Kit struct {
	Loadout string             `json:"loadout"`
	Worn    map[Slot]*Instance `json:"worn"`
	Pack    []Instance         `json:"pack"`

	cat *Catalog
}

// Refusals, returned as errors so the panel can show them verbatim.
var (
	ErrInFight      = errors.New("not in a fight")
	ErrTwoHanded    = errors.New("the main hand holds a two-handed weapon")
	ErrLitTorch     = errors.New("douse the torch first")
	ErrRanged       = errors.New("shooting is not built yet")
	ErrStory        = errors.New("that stays with him")
	ErrNoSuchItem   = errors.New("nothing there")
	ErrDoesNotFit   = errors.New("it does not go there")
	ErrNothingWorn  = errors.New("nothing is worn there")
	ErrUnboundKit   = errors.New("kit has no catalogue")
	ErrUnknownKitID = errors.New("unknown loadout")
)

// NewKit builds a starting kit from a named loadout. Every starting item is
// soldier's issue in sound condition.
func (c *Catalog) NewKit(loadout string) (*Kit, error) {
	l, ok := c.loadouts[loadout]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownKitID, loadout)
	}

	k := &Kit{Loadout: loadout, Worn: map[Slot]*Instance{}, cat: c}

	for slot, id := range map[Slot]string{
		SlotMain: l.Main, SlotOff: l.Off, SlotBody: l.Body,
		SlotHead: l.Head, SlotBelt1: l.Belt1, SlotBelt2: l.Belt2,
	} {
		if id != "" {
			inst := c.fresh(id, 1)
			k.Worn[slot] = &inst
		}
	}

	for _, entry := range l.Pack {
		id, n := splitStack(entry)
		k.Pack = append(k.Pack, c.fresh(id, n))
	}

	return k, nil
}

func (c *Catalog) fresh(id string, count int) Instance {
	inst := Instance{Item: id, Make: MakeIssue, Condition: Sound}

	it := c.byID[id]

	if it.Armour != nil {
		inst.Points = int(math.Round(float64(it.Armour.Points) * makeFactor(inst.Make)))
	}

	if it.Light != nil {
		inst.BurnLeft = it.Light.BurnMinutes
	}

	if it.Stack > 0 || count > 1 {
		inst.Count = count
	}

	return inst
}

// Bind attaches the catalogue to a kit read back from a save, and drops
// anything the catalogue no longer knows rather than failing the load.
func (k *Kit) Bind(c *Catalog) {
	k.cat = c

	if k.Worn == nil {
		k.Worn = map[Slot]*Instance{}
	}

	for slot, inst := range k.Worn {
		if inst == nil {
			delete(k.Worn, slot)
			continue
		}

		// Unknown items, unknown slots, and items in a slot they do not fit
		// (a hand-edited file) are dropped to the pack or out entirely.
		it, ok := c.byID[inst.Item]
		if !ok {
			delete(k.Worn, slot)
			continue
		}

		if !fitsSlot(it, slot) {
			k.Pack = append(k.Pack, *inst)
			delete(k.Worn, slot)
		}
	}

	kept := k.Pack[:0]

	for _, inst := range k.Pack {
		if _, ok := c.byID[inst.Item]; ok {
			kept = append(kept, inst)
		}
	}

	k.Pack = kept
}

// ItemIn is the catalogue entry worn in a slot, and the instance.
func (k *Kit) ItemIn(slot Slot) (*Item, *Instance, bool) {
	if k == nil || k.cat == nil {
		return nil, nil, false
	}

	inst := k.Worn[slot]
	if inst == nil {
		return nil, nil, false
	}

	it, ok := k.cat.byID[inst.Item]

	return it, inst, ok
}

// PackItem is the catalogue entry for a pack row.
func (k *Kit) PackItem(i int) (*Item, *Instance, bool) {
	if k == nil || k.cat == nil || i < 0 || i >= len(k.Pack) {
		return nil, nil, false
	}

	it, ok := k.cat.byID[k.Pack[i].Item]

	return it, &k.Pack[i], ok
}

// Equip moves pack row i onto the body. offLit says whether a torch in the
// off-hand is lit right now -- the light model knows, the kit does not.
//
// v0 RULES (U1 §5.2, E3 §4): no changing gear inside a fight; a two-hander
// takes the off-hand, so the off-hand item goes to the pack, and never a lit
// torch; nothing goes in the off-hand beside a two-hander; a bow is carried,
// not wielded (shooting is not built); the belt takes belt things.
func (k *Kit) Equip(i int, inFight, offLit bool) error {
	if k == nil || k.cat == nil {
		return ErrUnboundKit
	}

	if inFight {
		return ErrInFight
	}

	it, inst, ok := k.PackItem(i)
	if !ok {
		return ErrNoSuchItem
	}

	slot, err := k.slotFor(it)
	if err != nil {
		return err
	}

	if it.Weapon != nil && it.Weapon.Ranged {
		return ErrRanged
	}

	if slot == SlotOff {
		if main, _, ok := k.ItemIn(SlotMain); ok && main.Weapon != nil && main.Weapon.Hands == 2 {
			return ErrTwoHanded
		}
	}

	// A LIT TORCH IS NEVER DISPLACED, by anything that would take the
	// off-hand: another off-hand item or a two-hander. Review finding: the
	// first draft checked only the two-hander, so a shield went on beside a
	// burning torch and he had light and a block at once -- the dilemma the
	// loadout exists to force, gone.
	takesOff := slot == SlotOff || (it.Weapon != nil && it.Weapon.Hands == 2)
	if takesOff && offLit {
		if off, _, _ := k.ItemIn(SlotOff); off != nil && off.Light != nil {
			return ErrLitTorch
		}
	}

	if old, _, _ := k.ItemIn(slot); old != nil && old.Kind == KindStory {
		return ErrStory
	}

	taken := *inst
	k.Pack = append(k.Pack[:i], k.Pack[i+1:]...)

	if it.Weapon != nil && it.Weapon.Hands == 2 {
		if off := k.Worn[SlotOff]; off != nil {
			k.Pack = append(k.Pack, *off)
			delete(k.Worn, SlotOff)
		}
	}

	if old := k.Worn[slot]; old != nil {
		k.Pack = append(k.Pack, *old)
	}

	k.Worn[slot] = &taken

	return nil
}

// slotFor picks the worn slot an item goes to; a belt item takes the first
// free belt slot, then the first.
func (k *Kit) slotFor(it *Item) (Slot, error) {
	switch it.Fits {
	case "main":
		return SlotMain, nil
	case "off":
		return SlotOff, nil
	case "body":
		return SlotBody, nil
	case "head":
		return SlotHead, nil
	case "belt":
		if k.Worn[SlotBelt1] == nil {
			return SlotBelt1, nil
		}

		if k.Worn[SlotBelt2] == nil {
			return SlotBelt2, nil
		}

		return SlotBelt1, nil
	}

	return "", ErrDoesNotFit
}

// Unequip moves a worn item to the pack.
func (k *Kit) Unequip(slot Slot, inFight, offLit bool) error {
	if k == nil || k.cat == nil {
		return ErrUnboundKit
	}

	if inFight {
		return ErrInFight
	}

	it, inst, ok := k.ItemIn(slot)
	if !ok {
		return ErrNothingWorn
	}

	if it.Kind == KindStory {
		return ErrStory
	}

	if slot == SlotOff && it.Light != nil && offLit {
		return ErrLitTorch
	}

	k.Pack = append(k.Pack, *inst)
	delete(k.Worn, slot)

	return nil
}

// --- what the resolver asks -------------------------------------------------

// Bite is the weapon in the main hand as the resolver rolls it: its damage
// range after make and condition, its class, what it ignores of mail, and the
// reaction it implies. ok is false when the hand is empty or holds nothing
// that fights -- the caller keeps its own fallback.
type Bite struct {
	Item     string
	Min, Max int
	Class    DamageClass
	VsMail   float64
	Reaction string
}

// MainBite reports the main-hand weapon.
func (k *Kit) MainBite() (Bite, bool) {
	it, inst, ok := k.ItemIn(SlotMain)
	if !ok || it.Weapon == nil || it.Weapon.Ranged {
		return Bite{}, false
	}

	f := makeFactor(inst.Make) * conditionFactor(inst.Condition)

	lo := int(math.Round(float64(it.Weapon.Min) * f))
	hi := int(math.Round(float64(it.Weapon.Max) * f))

	if lo < 1 {
		lo = 1
	}

	if hi < lo {
		hi = lo
	}

	return Bite{
		Item: it.ID, Min: lo, Max: hi, Class: it.Weapon.Class,
		VsMail: it.Weapon.VsMail, Reaction: it.Weapon.Reaction,
	}, true
}

// armourCondition is what the points left make of a worn piece: above half is
// sound, half or less is damaged (its reduction halves), none is ruined.
func armourCondition(inst *Instance, max int) Condition {
	switch {
	case max <= 0:
		return Sound
	case inst.Points <= 0:
		return Ruined
	case inst.Points*2 <= max:
		return Damaged
	}

	return Sound
}

// maxPoints is a worn armour piece's full points at its make.
func (k *Kit) maxPoints(slot Slot) int {
	it, inst, ok := k.ItemIn(slot)
	if !ok || it.Armour == nil {
		return 0
	}

	return int(math.Round(float64(it.Armour.Points) * makeFactor(inst.Make)))
}

// ArmourCondition reports a worn piece's condition by its points.
func (k *Kit) ArmourCondition(slot Slot) Condition {
	_, inst, ok := k.ItemIn(slot)
	if !ok {
		return Sound
	}

	return armourCondition(inst, k.maxPoints(slot))
}

// Absorb is what the body armour (and, on a crit, the helmet) takes off one
// blow of the given class, and it WEARS them: a hit costs a point, a crit two,
// a graze nothing [DIAL]. vsMail is the attacker's share of the reduction
// ignored. The caller floors the damage; this only subtracts.
func (k *Kit) Absorb(class DamageClass, vsMail float64, band string) int {
	if k == nil {
		return 0
	}

	total := 0
	crit := band == "crit"

	for _, slot := range []Slot{SlotBody, SlotHead} {
		it, inst, ok := k.ItemIn(slot)
		if !ok || it.Armour == nil || it.Armour.Block {
			continue
		}

		if it.Armour.CritOnly && !crit {
			continue
		}

		cond := armourCondition(inst, k.maxPoints(slot))
		if cond == Ruined && it.Armour.Points > 0 {
			continue
		}

		r := float64(it.Armour.Reduction[class]) * (1 - clamp01(vsMail))
		if cond == Damaged {
			r /= 2
		}

		total += int(math.Round(r))

		if it.Armour.Points > 0 {
			inst.Points -= wearFor(band)
			if inst.Points < 0 {
				inst.Points = 0
			}

			inst.Condition = armourCondition(inst, k.maxPoints(slot))
		}
	}

	return total
}

func wearFor(band string) int {
	switch band {
	case "hit":
		return 1
	case "crit":
		return 2
	}

	return 0
}

// CanBlock reports an unruined shield in the off-hand.
func (k *Kit) CanBlock() bool {
	it, inst, ok := k.ItemIn(SlotOff)
	if !ok || it.Armour == nil || !it.Armour.Block {
		return false
	}

	return armourCondition(inst, k.maxPoints(SlotOff)) != Ruined
}

// SpendBlock takes a point off the shield for a block it made.
func (k *Kit) SpendBlock() {
	_, inst, ok := k.ItemIn(SlotOff)
	if !ok {
		return
	}

	if inst.Points > 0 {
		inst.Points--
	}

	inst.Condition = armourCondition(inst, k.maxPoints(SlotOff))
}

// OffHandTorch reports a torch in the off-hand, and its instance.
func (k *Kit) OffHandTorch() (*Item, *Instance, bool) {
	it, inst, ok := k.ItemIn(SlotOff)
	if !ok || it.Light == nil {
		return nil, nil, false
	}

	return it, inst, true
}

// SpendOffHand removes the off-hand item entirely -- a torch burnt to nothing.
func (k *Kit) SpendOffHand() {
	if k != nil {
		delete(k.Worn, SlotOff)
	}
}

// LoadKg is the kit's total weight: worn and carried.
func (k *Kit) LoadKg() float64 {
	if k == nil || k.cat == nil {
		return 0
	}

	total := 0.0

	add := func(inst *Instance) {
		if it, ok := k.cat.byID[inst.Item]; ok {
			n := inst.Count
			if n < 1 {
				n = 1
			}

			total += it.WeightKg * float64(n)
		}
	}

	for _, inst := range k.Worn {
		if inst != nil {
			add(inst)
		}
	}

	for i := range k.Pack {
		add(&k.Pack[i])
	}

	return total
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

// ---- T5: what crafting and barter do to a kit ------------------------------

// ErrNotEnough is a take the pack cannot cover.
var ErrNotEnough = errors.New("not enough in the pack")

// countOf is how many one pack row holds: a stack's count, or one.
func countOf(inst *Instance) int {
	if inst.Count > 0 {
		return inst.Count
	}

	return 1
}

// Count is how many of an item are in the pack.
func (k *Kit) Count(id string) int {
	n := 0

	for i := range k.Pack {
		if k.Pack[i].Item == id {
			n += countOf(&k.Pack[i])
		}
	}

	return n
}

// Carries reports an item worn or in the pack -- what a recipe's TOOLS need.
func (k *Kit) Carries(id string) bool {
	for _, inst := range k.Worn {
		if inst != nil && inst.Item == id {
			return true
		}
	}

	return k.Count(id) > 0
}

// Take removes n of an item from the pack, all or nothing.
func (k *Kit) Take(id string, n int) error {
	if n <= 0 {
		return nil
	}

	if k.Count(id) < n {
		return fmt.Errorf("%w: %s", ErrNotEnough, id)
	}

	kept := k.Pack[:0]

	for _, inst := range k.Pack {
		if n > 0 && inst.Item == id {
			have := countOf(&inst)
			if have <= n {
				n -= have
				continue
			}

			inst.Count = have - n
			n = 0
		}

		kept = append(kept, inst)
	}

	k.Pack = kept

	return nil
}

// ErrUnknownItem is an item the catalogue does not know.
var ErrUnknownItem = errors.New("unknown item")

// Bound reports a kit attached to its catalogue (NewKit, or Bind after a load).
func (k *Kit) Bound() bool { return k != nil && k.cat != nil }

// Give puts n of an item in the pack: onto a stack of it while the stack has
// room, then in new rows. Items that do not stack arrive one to a row, new
// and sound.
func (k *Kit) Give(id string, n int) error {
	if !k.Bound() {
		return ErrUnboundKit
	}

	it, ok := k.cat.byID[id]
	if !ok {
		return fmt.Errorf("%w %q", ErrUnknownItem, id)
	}

	for n > 0 {
		if it.Stack > 0 {
			for i := range k.Pack {
				if room := it.Stack - countOf(&k.Pack[i]); k.Pack[i].Item == id && room > 0 {
					add := min(room, n)
					k.Pack[i].Count = countOf(&k.Pack[i]) + add
					n -= add
				}
			}

			if n <= 0 {
				break
			}

			add := min(it.Stack, n)
			k.Pack = append(k.Pack, k.cat.fresh(id, add))
			k.Pack[len(k.Pack)-1].Count = add
			n -= add

			continue
		}

		k.Pack = append(k.Pack, k.cat.fresh(id, 1))
		n--
	}

	return nil
}

// ErrNothingToMend is a repair of a piece already as good as this repair
// makes it.
var ErrNothingToMend = errors.New("nothing to mend")

// Mend restores up to points to the armour worn in a slot, but never past
// capFraction of its full points: a field repair is "slower, worse" than the
// smith's (S1 §8.3), so it stops short of new. It returns what it restored.
func (k *Kit) Mend(slot Slot, points int, capFraction float64) (int, error) {
	room, err := k.CanMend(slot, capFraction)
	if err != nil {
		return 0, err
	}

	_, inst, _ := k.ItemIn(slot)
	restored := min(points, room)
	inst.Points += restored
	inst.Condition = armourCondition(inst, k.maxPoints(slot))

	return restored, nil
}

// CanMend reports how far a mend up to capFraction could go, without doing it.
func (k *Kit) CanMend(slot Slot, capFraction float64) (int, error) {
	it, inst, ok := k.ItemIn(slot)
	if !ok || it.Armour == nil || it.Armour.Points <= 0 {
		return 0, fmt.Errorf("%w: no armour worn there", ErrNothingToMend)
	}

	limit := int(math.Floor(float64(k.maxPoints(slot)) * clamp01(capFraction)))
	if inst.Points >= limit {
		return 0, ErrNothingToMend
	}

	return limit - inst.Points, nil
}
