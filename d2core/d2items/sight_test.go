package d2items

import (
	"errors"
	"testing"
)

// TestTheBowIsHeldToLookAlong (fog of war F4; Josh's Q9 on its default: "an
// equipped composite bow: +2, the hunter's eye"): carried in the pack the bow
// gives nothing; taken up -- both hands, never past a lit torch, which it
// would displace -- it gives his eye +2; it does not shoot (no bite: in a
// fight he strikes as with an empty hand); put down, the +2 goes with it.
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

	if _, ok := k.MainBite(); ok {
		t.Fatal("the bow in his hands bites: shooting is not built, so it must not")
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
	ok := `{"items": [{"id": "x", "name": "X", "kind": "tool", "slot": "main", "weight_kg": 1, "sight": 2, "tool": {"verb": "v"}}],` +
		` "loadouts": {"a": {}}, "default_loadout": "a"}`
	if _, err := Load([]byte(ok)); err != nil {
		t.Errorf("held sight of 2 is refused: %v", err)
	}
}
