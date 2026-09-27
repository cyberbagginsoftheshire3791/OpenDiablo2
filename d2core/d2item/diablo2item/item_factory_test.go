package diablo2item

import (
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// Diablo II's item tables load with the first item made, not at boot (M5.3's
// tables burst). When they cannot load, NewItem says so -- an error naming the
// tables, no panic, no item -- and marks none of them loaded, so the next
// NewItem reads them again (EnsureRecords; the tables burst's review, B3, 27
// Sep 2026). Here no MPQ is mounted, so every read fails.
//
// Negative control (27 Sep 2026): drop NewItem's early return on the failed
// load and this fails -- the error is "cannot create item", which says
// nothing of the tables that were never read.
func TestNewItemRefusesWhenItsTablesDoNotLoad(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	f, err := NewItemFactory(asset)
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 2; i++ {
		item, err := f.NewItem("hax")
		if item != nil || err == nil || !strings.Contains(err.Error(), "its tables did not load") {
			t.Fatalf("make %d: item %v, error %v; want no item and the tables' failure", i, item, err)
		}
	}

	for _, table := range d2resource.ItemRecords {
		if asset.RecordsLoaded(table) {
			t.Errorf("%s is marked loaded after a failed read", table)
		}
	}
}
