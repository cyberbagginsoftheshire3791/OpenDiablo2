package d2items

import (
	"errors"
	"testing"
)

// TestTheBowIsHeldToLookAlong (fog of war F4; Josh's Q9 on its default: "an
// equipped composite bow: +2, the hunter's eye"): carried in the pack the bow
// gives nothing; taken up -- both hands, never past a lit torch, which it
// would displace -- it gives his eye +2; it does not shoot; put down, the +2
// goes with it.
//
// Negative control (2 Oct 2026, strigoi-harness-runs\wt-fog4\nc\): Kit.Sight
// reading the off hand only and this fails, "with the bow in his hands he
// sees +0" (nc-kit-sight-off-only.txt).
func TestTheBowIsHeldToLookAlong(t *testing.T) {
	k := kitFor(t, "torch-and-blade")

	if got := k.Sight(); got != 0 {
		t.Fatalf("with the bow in his pack he sees +%v; carried, it gives nothing", got)
	}

	if err := k.Equip(packIndex(t, k, "composite-bow"), false, true); !errors.Is(err, ErrLitTorch) {
		t.Fatalf("the bow past a lit torch: %v; a two-hander never displaces a lit torch", err)
	}

	if err := k.Equip(packIndex(t, k, "composite-bow"), true, false); !errors.Is(err, ErrInFight) {
		t.Fatalf("the bow taken up in a fight: %v", err)
	}

	if err := k.Equip(packIndex(t, k, "composite-bow"), false, false); err != nil {
		t.Fatalf("the bow taken up out of a fight with the torch unlit: %v", err)
	}

	if got := k.Sight(); got != 2 {
		t.Fatalf("with the bow in his hands he sees +%v; Q9 is +2", got)
	}

	if k.Worn[SlotOff] != nil {
		t.Fatal("the bow is two-handed: the torch goes to the pack")
	}

	// Swung, not shot (the F4 review's B2): its bite is its bash, 2-5 blunt
	// with no reaction -- below a knife's 6-11, never a sabre's 12-20.
	bite, ok := k.MainBite()
	if !ok || bite.Item != "composite-bow" || bite.Min != 2 || bite.Max != 5 || bite.Class != Blunt || bite.Reaction != ReactionNone {
		t.Fatalf("the bow in his hands bites %+v (ok %v); want its bash, 2-5 blunt, no reaction", bite, ok)
	}

	if err := k.Unequip(SlotMain, false, false); err != nil {
		t.Fatal(err)
	}

	if got := k.Sight(); got != 0 {
		t.Fatalf("with the bow put down he sees +%v", got)
	}

	// The table refuses sight that is not held, and sight past the bound.
	for _, doc := range []string{
		`{"items": [{"id": "x", "name": "X", "kind": "tool", "slot": "pack", "weight_kg": 1, "sight": 1, "tool": {"verb": "v"}}],` +
			` "loadouts": {"a": {}}, "default_loadout": "a"}`,
		`{"items": [{"id": "x", "name": "X", "kind": "tool", "slot": "main", "weight_kg": 1, "sight": 9, "tool": {"verb": "v"}}],` +
			` "loadouts": {"a": {}}, "default_loadout": "a"}`,
	} {
		if _, err := Load([]byte(doc)); err == nil {
			t.Errorf("the table took %s", doc)
		}
	}

	// Their mirror: held sight within the bound loads.
	held := `{"items": [{"id": "x", "name": "X", "kind": "tool", "slot": "main", "weight_kg": 1, "sight": 2, "tool": {"verb": "v"}}],` +
		` "loadouts": {"a": {}}, "default_loadout": "a"}`
	if _, err := Load([]byte(held)); err != nil {
		t.Errorf("held sight of 2 is refused: %v", err)
	}
}

// TestAHeldBowStrikesAsABash (the F4 review's B2, 2 Oct 2026): a bow held in
// a fight is swung -- the composite bow's bash, 2-5 blunt, scaled by its make
// and condition like any weapon -- and never strikes like the sabre it
// displaced. The bow is 0.9 kg of horn, wood and sinew built to flex: swung,
// it is a light stick that a hard blow would break, so its bash sits below
// the knife's 6-11 (a real edge) and far below the kılıç's 12-20. A bash is a
// ranged weapon's alone, and must have 1 <= min <= max.
//
// Negative control (2 Oct 2026, strigoi-harness-runs\wt-fog4\nc\): MainBite
// ignoring the bash (no bite for a ranged weapon, as at c06e4c3a) and this
// fails, "a held bow has no bite" (nc-bow-no-bash.txt).
func TestAHeldBowStrikesAsABash(t *testing.T) {
	k := kitFor(t, "sword-and-board")

	sabre, ok := k.MainBite()
	if !ok {
		t.Fatal("the sabre has no bite")
	}

	k.Pack = append(k.Pack, k.cat.fresh("composite-bow", 1))
	if err := k.Equip(len(k.Pack)-1, false, false); err != nil {
		t.Fatal(err)
	}

	bow, ok := k.MainBite()
	if !ok {
		t.Fatal("a held bow has no bite")
	}

	if bow.Max >= sabre.Min || bow.Class != Blunt || bow.Reaction != ReactionNone {
		t.Fatalf("the held bow bites %+v, the sabre %+v; a swung bow is a weak bash, never a sabre's blow", bow, sabre)
	}

	k.Worn[SlotMain].Condition = Ruined

	if worn, _ := k.MainBite(); worn.Max != 3 { // round(5 x 0.5)
		t.Fatalf("a ruined bow's bash tops at %d; scaled like any weapon, 3", worn.Max)
	}

	for _, bad := range []string{
		`"weapon": {"hands": 1, "reach": 1, "class": "cut", "min": 2, "max": 3, "reaction": "none", "bash": {"min": 1, "max": 2}}`,
		`"weapon": {"hands": 2, "reach": 6, "class": "thrust", "min": 2, "max": 3, "reaction": "none", "ranged": true, "bash": {"min": 3, "max": 2}}`,
	} {
		doc := `{"items": [{"id": "x", "name": "X", "kind": "weapon", "slot": "main", "weight_kg": 1, ` + bad + `}],` +
			` "loadouts": {"a": {}}, "default_loadout": "a"}`
		if _, err := Load([]byte(doc)); err == nil {
			t.Errorf("the table took %s", bad)
		}
	}
}
