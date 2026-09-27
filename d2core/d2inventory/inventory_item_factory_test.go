package d2inventory

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// Strigoi's hero carries no Diablo II item (M5.3's tables burst): his loadout
// is his kit, so the factory reads neither weapons.txt nor armor.txt and every
// class's equipment is empty -- which a Diablo II composite draws as an empty
// hand, in the constant hand-to-hand weapon class.
//
// Negative control: build DefaultHeroItems whatever the launch and this fails
// with "could not find armor entry for code 'buc'" (no table is loaded here).
func TestStrigoisHeroCarriesNoDiabloItem(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	f, err := NewInventoryItemFactory(asset)
	if err != nil {
		t.Fatalf("the factory needed a table: %v", err)
	}

	for _, table := range []string{d2resource.Weapons, d2resource.Armor, d2resource.Misc} {
		if asset.RecordsLoaded(table) {
			t.Errorf("%s was loaded", table)
		}
	}

	equipment := f.DefaultHeroItems[d2enum.HeroAmazon]
	if equipment.RightHand != nil || equipment.Shield != nil {
		t.Fatalf("the amazon's equipment %+v, want empty hands", equipment)
	}

	if got := equipment.RightHand.GetWeaponClass(); got != "hth" {
		t.Fatalf("an empty hand's weapon class is %q, want the constant hth", got)
	}
}
