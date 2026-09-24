package d2gamescreen

import (
	"errors"
	"os"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
)

// T2, the kit (23 Sep 2026): the game screen owns the local hero's gear. It
// loads the catalogue with the bestiary, reads the hero's kit from the sidecar
// beside his save (d2items.SidecarPath), and -- when there is none, which is
// every new hero and every save from before T2 -- holds the world while he
// chooses his loadout, once. Every change is saved at once: equipping,
// choosing, a torch burning out, and the screen closing on a living hero.
//
// It answers three seams: d2world.Kits (the resolver reads his weapon, his
// mail and his shield), d2player.KitHolder (the panel and the torch key), and
// the harness through the "ui" provider.

// itemCatalogPath is the shipped item table.
const itemCatalogPath = "/data/strigoi/items.json"

// bindKit reads the hero's kit, or opens the loadout choice. Called once the
// local player exists.
func (v *Game) bindKit() {
	if v.items == nil {
		return
	}

	v.kitPath = d2items.SidecarPath(v.gameClient.SaveFilePath)

	// Death screen v0: the file as he enters, before anything here writes it.
	v.snapshotHero()

	kit, extras, err := d2items.LoadHero(v.kitPath, v.items)

	// T3: his progress rides in the same file; none yet is a fresh hero.
	// T4: and what the village thinks of him; none yet is a stranger.
	// Deferred calls run last-first, so the STANDING binds first and progress
	// after it: nothing progress does can save the file before the standing is
	// in hand (review finding: a save then would drop the village block).
	// J1: the journal reads the standing and progress, so it binds LAST --
	// deferred FIRST (A9).
	defer v.bindJournal(extras.Journal)
	defer v.bindProgress(extras.Progress)
	defer v.bindStanding(extras.Village)
	defer v.bindLand(extras.Land)

	switch {
	case err == nil:
		v.kit = kit
	case errors.Is(err, d2items.ErrNoSidecar):
		v.choosingLoadout = true
	default:
		// A kit file that cannot be read is not a reason to lose the run: say
		// so, SET THE FILE ASIDE rather than overwrite it -- it holds his
		// progress too, and the next save would erase it for good (review
		// finding) -- and let him choose again.
		v.Errorf("kit: %v -- set aside as .corrupt; choosing a loadout again", err)

		if rerr := os.Rename(v.kitPath, v.kitPath+".corrupt"); rerr != nil {
			v.Errorf("kit: %v", rerr)
		}

		v.choosingLoadout = true
	}
}

// KitOf is the d2world.Kits seam: the local hero's kit, and nobody else's --
// no other body carries gear in v0.
func (v *Game) KitOf(id string) *d2items.Kit {
	if v.kit == nil || v.localPlayer == nil || id != v.localPlayer.ID() {
		return nil
	}

	return v.kit
}

// Kit is the local hero's kit, or nil before he has chosen.
func (v *Game) Kit() *d2items.Kit { return v.kit }

// Catalog is the item table.
func (v *Game) Catalog() *d2items.Catalog { return v.items }

// ChoosingLoadout reports that the choice is open and the world is held.
func (v *Game) ChoosingLoadout() bool { return v.choosingLoadout }

// ChooseLoadout makes the one choice and saves it.
func (v *Game) ChooseLoadout(name string) error {
	if !v.choosingLoadout || v.items == nil {
		return nil
	}

	kit, err := v.items.NewKit(name)
	if err != nil {
		return err
	}

	v.kit = kit
	v.choosingLoadout = false
	v.saveKit()

	return nil
}

// EquipFromPack puts pack row i on him.
func (v *Game) EquipFromPack(i int) error {
	if v.kit == nil {
		return nil
	}

	// T6: a row he eats is eaten, not worn.
	if it, _, ok := v.kit.PackItem(i); ok && it.Tool != nil && it.Tool.Verb == d2items.VerbEat {
		return v.eatFromPack(i)
	}

	_, torch, hadTorch := v.kit.OffHandTorch()
	var displaced d2items.Instance

	if hadTorch {
		displaced = *torch
	}

	if err := v.kit.Equip(i, v.inFight(), v.torchLit()); err != nil {
		return err
	}

	// A DOUSED torch the equip pushed out of his hand takes its minutes out of
	// the light model with it -- the handoff UnequipSlot makes, made here too.
	// Review finding: without it the source stayed "carried", L relit a torch
	// he no longer held, and a fresh torch's minutes could be overwritten.
	if _, _, still := v.kit.OffHandTorch(); hadTorch && !still {
		v.returnTorchToPack(displaced.Item)
	}

	v.saveKit()

	return nil
}

// returnTorchToPack moves a carried source's minutes onto the torch that just
// went into the pack (the newest one of that item), and removes the source.
func (v *Game) returnTorchToPack(item string) {
	if v.light == nil {
		return
	}

	carried := v.light.Carried()
	if carried == nil {
		return
	}

	burn := carried.Burn
	v.light.Remove(carried.ID)

	for i := len(v.kit.Pack) - 1; i >= 0; i-- {
		if v.kit.Pack[i].Item == item {
			v.kit.Pack[i].BurnLeft = burn
			return
		}
	}
}

// UnequipSlot takes a worn item off. A torch put away takes its remaining
// minutes with it: the light model gives the source up and the kit keeps the
// burn, so the same torch lit again later picks up where it stopped.
func (v *Game) UnequipSlot(slot d2items.Slot) error {
	if v.kit == nil {
		return nil
	}

	_, torch, isTorch := v.kit.OffHandTorch()

	if err := v.kit.Unequip(slot, v.inFight(), v.torchLit()); err != nil {
		return err
	}

	if slot == d2items.SlotOff && isTorch {
		v.returnTorchToPack(torch.Item)
	}

	v.saveKit()

	return nil
}

// spendBurntTorch is the kit's half of a torch burning out: it is gone from
// his hand, not carried as an empty stick (the 12 Sep ruling on burn-out).
func (v *Game) spendBurntTorch() {
	if v.kit == nil {
		return
	}

	if _, _, ok := v.kit.OffHandTorch(); ok {
		v.kit.SpendOffHand()
		v.note("torch_out")
		v.saveKit()
	}
}

func (v *Game) inFight() bool { return v.combat != nil && v.combat.Fighting() }

func (v *Game) torchLit() bool {
	if v.light == nil {
		return false
	}

	carried := v.light.Carried()

	return carried != nil && carried.Lit
}

// saveKit writes the sidecar. A failure is logged, not fatal: losing a write
// costs the last change, and stopping the game costs the run.
func (v *Game) saveKit() {
	if v.kit == nil || v.kitPath == "" {
		return
	}

	// NEVER FOR A DEAD HERO, the 12 Sep ruling the .od2 already obeys
	// (shouldSaveOnUnload). Review finding: the fight-close save wrote a dead
	// man's wear beside his last living save.
	if v.localPlayer != nil && !shouldSaveOnUnload(v.localPlayer) {
		return
	}

	// A torch lit when he leaves keeps what it had left: the light model holds
	// the minutes while it burns (the kit holds zero, see the L key), so they
	// are written for the save and taken back out, and the live kit still
	// owns nothing the light model owns.
	_, torch, isTorch := v.kit.OffHandTorch()
	held := 0.0

	if isTorch && v.light != nil {
		if carried := v.light.Carried(); carried != nil && torch.BurnLeft == 0 {
			held = carried.Burn
			torch.BurnLeft = held
		}
	}

	if err := d2items.SaveHero(v.kitPath, v.kit, d2items.Extras{Progress: v.progressJSON(), Village: v.standingJSON(), Land: v.landJSON(), Journal: v.journalJSON()}); err != nil {
		v.Errorf("kit: %v", err)
	}

	if held > 0 {
		torch.BurnLeft = 0
	}
}

// Refusals of the eat verb (T6).
var (
	errEatInFight = errors.New("not in a fight")
	errNotHungry  = errors.New("not hungry")
)

// eatFromPack eats one piece of pack row i: the first verb that feeds him from
// his own kit. The 28 Aug ruling deferred eating to Phase 6's inventory, which
// T2 built; the peksimet has carried the verb since.
func (v *Game) eatFromPack(i int) error {
	it, _, ok := v.kit.PackItem(i)
	if !ok || it.Tool == nil {
		return d2items.ErrNotFood
	}

	switch {
	case v.meters == nil:
		return errNotHungry
	case v.died || !v.alive():
		return errCraftDead
	case v.talk != nil || v.choosingLoadout:
		return errCraftBusy
	case v.inFight():
		return errEatInFight
	case v.meters.Food()+it.Tool.Food > 100+eatSlack:
		// A piece is not wasted on a stomach it would mostly overflow.
		return errNotHungry
	}

	food, err := v.kit.Eat(i)
	if err != nil {
		return err
	}

	if err := v.meters.Consume("food", food); err != nil {
		return err
	}

	v.saveKit()

	return nil
}

// eatSlack is how much of a piece may overflow the food meter [DIAL].
const eatSlack = 5.0
