package d2items

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// shipped loads the real data/strigoi/items.json the game ships.
func shipped(t *testing.T) *Catalog {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "data", "strigoi", "items.json"))
	if err != nil {
		t.Fatal(err)
	}

	c, err := Load(data)
	if err != nil {
		t.Fatalf("the shipped item table must load: %v", err)
	}

	return c
}

func kitFor(t *testing.T, loadout string) *Kit {
	t.Helper()

	k, err := shipped(t).NewKit(loadout)
	if err != nil {
		t.Fatal(err)
	}

	return k
}

func packIndex(t *testing.T, k *Kit, id string) int {
	t.Helper()

	for i, inst := range k.Pack {
		if inst.Item == id {
			return i
		}
	}

	t.Fatalf("%q is not in the pack: %+v", id, k.Pack)

	return -1
}

func TestLoadRefusesWhatItCannotTrust(t *testing.T) {
	for name, doc := range map[string]string{
		"unknown field": `{"items":[{"id":"a","name":"A","kind":"story","slot":"head","weight_kg":1,"sparkle":1}],"loadouts":{"x":{}},"default_loadout":"x"}`,
		"duplicate id":  `{"items":[{"id":"a","name":"A","kind":"story","slot":"head"},{"id":"A","name":"B","kind":"story","slot":"head"}],"loadouts":{"x":{}},"default_loadout":"x"}`,
		"bad slot":      `{"items":[{"id":"a","name":"A","kind":"story","slot":"hat"}],"loadouts":{"x":{}},"default_loadout":"x"}`,
		"bad weapon":    `{"items":[{"id":"a","name":"A","kind":"weapon","slot":"main","weapon":{"hands":3,"reach":1,"class":"cut","min":1,"max":2,"reaction":"none"}}],"loadouts":{"x":{}},"default_loadout":"x"}`,
		"loadout miss":  `{"items":[{"id":"a","name":"A","kind":"story","slot":"head"}],"loadouts":{"x":{"main":"nope"}},"default_loadout":"x"}`,
		"misfit":        `{"items":[{"id":"a","name":"A","kind":"story","slot":"head"}],"loadouts":{"x":{"main":"a"}},"default_loadout":"x"}`,
		"no default":    `{"items":[{"id":"a","name":"A","kind":"story","slot":"head"}],"loadouts":{"x":{}},"default_loadout":"y"}`,
	} {
		if _, err := Load([]byte(doc)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	// THE CONTROL: the shipped table loads, so the refusals above are about
	// what each one broke and not about the loader refusing everything.
	shipped(t)
}

func TestTheTwoLoadoutsAreTheDilemma(t *testing.T) {
	board := kitFor(t, "sword-and-board")
	torch := kitFor(t, "torch-and-blade")

	if !board.CanBlock() {
		t.Fatal("sword-and-board carries a shield that blocks")
	}

	if _, _, ok := board.OffHandTorch(); ok {
		t.Fatal("and no torch: one off-hand, the genre's dilemma (R2 §3)")
	}

	if torch.CanBlock() {
		t.Fatal("torch-and-blade has no block")
	}

	it, inst, ok := torch.OffHandTorch()
	if !ok || inst.BurnLeft != it.Light.BurnMinutes {
		t.Fatalf("torch-and-blade carries a fresh torch: %+v %+v", it, inst)
	}

	// Both carry the same kılıç, which copies the placeholder profile's 12-20
	// so the player's side of the winnability basis does not move.
	for _, k := range []*Kit{board, torch} {
		b, ok := k.MainBite()
		if !ok || b.Min != 12 || b.Max != 20 || b.Class != Cut || b.Reaction != ReactionRiposte {
			t.Fatalf("the starting blade must bite 12-20 cut with a riposte: %+v", b)
		}
	}
}

func TestEquipRules(t *testing.T) {
	c := shipped(t)

	// A fight refuses any change -- and its mirror, out of a fight, allows it.
	k := kitFor(t, "torch-and-blade")
	if err := k.Unequip(SlotMain, true, false); !errors.Is(err, ErrInFight) {
		t.Fatalf("unequip in a fight: %v", err)
	}

	if err := k.Unequip(SlotMain, false, false); err != nil {
		t.Fatalf("unequip out of a fight: %v", err)
	}

	if _, ok := k.MainBite(); ok {
		t.Fatal("an empty main hand has no bite")
	}

	// The bow is carried, not wielded -- and its mirror, a sabre goes back.
	if err := k.Equip(packIndex(t, k, "composite-bow"), false, false); !errors.Is(err, ErrRanged) {
		t.Fatalf("equip the bow: %v", err)
	}

	if err := k.Equip(packIndex(t, k, "kilic"), false, false); err != nil {
		t.Fatalf("equip the sabre: %v", err)
	}

	// A two-hander will not go past a LIT torch -- and with it unlit, it takes
	// the off-hand and the torch goes to the pack.
	k.Pack = append(k.Pack, c.fresh("spear", 1))

	if err := k.Equip(packIndex(t, k, "spear"), false, true); !errors.Is(err, ErrLitTorch) {
		t.Fatalf("a spear past a lit torch: %v", err)
	}

	if err := k.Equip(packIndex(t, k, "spear"), false, false); err != nil {
		t.Fatalf("a spear past an unlit torch: %v", err)
	}

	if k.Worn[SlotOff] != nil {
		t.Fatal("the two-hander took the off-hand")
	}

	packIndex(t, k, "torch") // and the torch is in the pack

	// Nothing goes in the off-hand beside a two-hander.
	if err := k.Equip(packIndex(t, k, "torch"), false, false); !errors.Is(err, ErrTwoHanded) {
		t.Fatalf("a torch beside a spear: %v", err)
	}

	// The ak börk stays -- and its mirror, the mail comes off.
	if err := k.Unequip(SlotHead, false, false); !errors.Is(err, ErrStory) {
		t.Fatalf("take off the ak börk: %v", err)
	}

	if err := k.Unequip(SlotBody, false, false); err != nil {
		t.Fatalf("take off the mail: %v", err)
	}
}

func TestMailAbsorbsAndWears(t *testing.T) {
	k := kitFor(t, "torch-and-blade")

	full := k.maxPoints(SlotBody)
	if full <= 0 {
		t.Fatal("the mail has points")
	}

	// A cut hit loses the mail's cut reduction and costs one point.
	if got := k.Absorb(Cut, 0, "hit"); got != 4 {
		t.Fatalf("mail takes 4 off a cut; took %d", got)
	}

	if got := k.Worn[SlotBody].Points; got != full-1 {
		t.Fatalf("a hit wears one point: %d of %d", got, full)
	}

	// A graze takes the reduction and wears nothing.
	before := k.Worn[SlotBody].Points
	k.Absorb(Cut, 0, "graze")

	if k.Worn[SlotBody].Points != before {
		t.Fatal("a graze wears nothing")
	}

	// A mace ignores half of it.
	if got := k.Absorb(Blunt, 0.5, "graze"); got != 1 {
		// blunt 1 * 0.5 = 0.5, rounds to 1 -- the mace's point is against CUT
		// mail values, which the next line proves.
		t.Fatalf("blunt vs mail: %d", got)
	}

	if got := k.Absorb(Cut, 0.5, "graze"); got != 2 {
		t.Fatalf("a mace's share against the cut reduction: got %d, want 2", got)
	}

	// Worn to half, the reduction halves; worn out, it is gone.
	k.Worn[SlotBody].Points = full / 2
	if got := k.Absorb(Cut, 0, "graze"); got != 2 {
		t.Fatalf("damaged mail halves its reduction: %d", got)
	}

	k.Worn[SlotBody].Points = 0
	if got := k.Absorb(Cut, 0, "graze"); got != 0 {
		t.Fatalf("ruined mail takes nothing: %d", got)
	}

	// THE CONTROL: with the mail off, nothing absorbs at all.
	bare := kitFor(t, "torch-and-blade")
	_ = bare.Unequip(SlotBody, false, false)

	if got := bare.Absorb(Cut, 0, "hit"); got != 0 {
		t.Fatalf("no mail, no reduction: %d", got)
	}
}

func TestTheHelmetOnlyAnswersACrit(t *testing.T) {
	c := shipped(t)
	k := kitFor(t, "torch-and-blade")

	helm := c.fresh("helmet", 1)
	k.Worn[SlotHead] = &helm
	_ = k.Unequip(SlotBody, false, false)

	if got := k.Absorb(Cut, 0, "hit"); got != 0 {
		t.Fatalf("a helmet does not answer a hit: %d", got)
	}

	if got := k.Absorb(Cut, 0, "crit"); got != 3 {
		t.Fatalf("a helmet answers a crit: %d", got)
	}
}

func TestTheShieldBlocksUntilItIsRuined(t *testing.T) {
	k := kitFor(t, "sword-and-board")

	for i := 0; i < k.maxPoints(SlotOff); i++ {
		if !k.CanBlock() {
			t.Fatalf("the shield failed after %d blocks", i)
		}

		k.SpendBlock()
	}

	if k.CanBlock() {
		t.Fatal("a ruined shield blocks nothing")
	}
}

func TestSaveRoundTripBindsAndDropsTheUnknown(t *testing.T) {
	c := shipped(t)
	k := kitFor(t, "sword-and-board")
	k.Worn[SlotBody].Points = 3
	k.Pack = append(k.Pack, Instance{Item: "a-thing-since-removed"})

	var back Kit

	data := mustJSON(t, k)
	mustUnJSON(t, data, &back)
	back.Bind(c)

	if back.Worn[SlotBody].Points != 3 {
		t.Fatal("wear survives a save")
	}

	for _, inst := range back.Pack {
		if inst.Item == "a-thing-since-removed" {
			t.Fatal("an item the catalogue no longer knows is dropped, not kept")
		}
	}

	if !back.CanBlock() {
		t.Fatal("the bound kit answers questions")
	}
}

func TestLoadKgCountsStacks(t *testing.T) {
	k := kitFor(t, "torch-and-blade")
	base := k.LoadKg()

	k.Pack[packIndex(t, k, "arrows")].Count += 10

	if got := k.LoadKg() - base; got < 0.49 || got > 0.51 {
		t.Fatalf("ten more arrows weigh half a kilo: %.3f", got)
	}
}

func TestSidecarRoundTrip(t *testing.T) {
	c := shipped(t)
	dir := t.TempDir()
	save := filepath.Join(dir, "3.od2")

	if _, _, err := LoadHero(SidecarPath(save), c); !errors.Is(err, ErrNoSidecar) {
		t.Fatalf("a hero who never chose has no kit: %v", err)
	}

	k := kitFor(t, "sword-and-board")
	k.Worn[SlotBody].Points = 5

	if err := SaveHero(SidecarPath(save), k, nil); err != nil {
		t.Fatal(err)
	}

	back, _, err := LoadHero(SidecarPath(save), c)
	if err != nil {
		t.Fatal(err)
	}

	if back.Loadout != "sword-and-board" || back.Worn[SlotBody].Points != 5 || !back.CanBlock() {
		t.Fatalf("the kit came back different: %+v", back)
	}

	// Beside THIS save and no other: a second hero has no kit.
	if _, _, err := LoadHero(SidecarPath(filepath.Join(dir, "4.od2")), c); !errors.Is(err, ErrNoSidecar) {
		t.Fatalf("another hero's save shares nothing: %v", err)
	}
}

// A LIT TORCH IS NEVER DISPLACED (review finding): not by a shield, not by a
// two-hander -- and with the torch doused, both go on and the torch goes to the
// pack.
func TestALitTorchStaysInHand(t *testing.T) {
	c := shipped(t)

	for _, id := range []string{"kalkan", "spear"} {
		k := kitFor(t, "torch-and-blade")
		k.Pack = append(k.Pack, c.fresh(id, 1))

		if err := k.Equip(packIndex(t, k, id), false, true); !errors.Is(err, ErrLitTorch) {
			t.Fatalf("%s beside a LIT torch must be refused: %v", id, err)
		}

		if _, _, ok := k.OffHandTorch(); !ok {
			t.Fatalf("%s: the torch must still be in hand", id)
		}

		if err := k.Equip(packIndex(t, k, id), false, false); err != nil {
			t.Fatalf("%s beside a doused torch: %v", id, err)
		}

		packIndex(t, k, "torch")
	}
}

// Bind puts an item worn where it does not fit back in the pack.
func TestBindDropsAMisfitToThePack(t *testing.T) {
	c := shipped(t)
	k := kitFor(t, "torch-and-blade")

	mail := *k.Worn[SlotBody]
	k.Worn[SlotMain] = &mail
	k.Worn["foo"] = &Instance{Item: "knife"}

	k.Bind(c)

	if it, _, _ := k.ItemIn(SlotMain); it != nil {
		t.Fatalf("mail in the hand must come out: %+v", it)
	}

	if _, ok := k.Worn["foo"]; ok {
		t.Fatal("an unknown slot must not survive Bind")
	}
}

// Progress rides beside the kit, and a file without it still loads.
func TestSidecarCarriesProgress(t *testing.T) {
	c := shipped(t)
	path := SidecarPath(filepath.Join(t.TempDir(), "5.od2"))
	k := kitFor(t, "torch-and-blade")

	if err := SaveHero(path, k, []byte(`{"xp":120}`)); err != nil {
		t.Fatal(err)
	}

	_, prog, err := LoadHero(path, c)
	if err != nil {
		t.Fatal(err)
	}

	var back struct{ XP int }
	if err := json.Unmarshal(prog, &back); err != nil || back.XP != 120 {
		t.Fatalf("progress round trip: %q %v", prog, err)
	}

	// A T2 file -- kit only -- has no progress and is not an error.
	if err := SaveHero(path, k, nil); err != nil {
		t.Fatal(err)
	}

	if _, prog, err := LoadHero(path, c); err != nil || prog != nil {
		t.Fatalf("a kit-only file loads with no progress: %q %v", prog, err)
	}
}
