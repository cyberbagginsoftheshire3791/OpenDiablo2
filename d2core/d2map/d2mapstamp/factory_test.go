package d2mapstamp

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// A stamp reads the generated world's tables, which load when it is first
// stamped. When they cannot load, the stamp is refused -- LoadStamp's nil, as
// for a stamp whose tiles or DS1 will not read, and no preset names -- with
// the reason logged; it used to Fatalf the whole process (the tables burst's
// review, B3, 27 Sep 2026). No MPQ is mounted here, so the read fails.
//
// Negative control (27 Sep 2026): put ensureTables' Fatalf back and this test
// binary exits (status 1) before it reports anything.
func TestAStampWhoseTablesDoNotLoadIsRefused(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	f := NewStampFactory(asset, d2util.LogLevelNone, nil)

	if names := f.PresetFileNames(2); names != nil {
		t.Fatalf("preset names %v from tables that did not load", names)
	}

	if stamp := f.LoadStamp(d2enum.RegionAct1Town, 2, 0); stamp != nil {
		t.Fatalf("a stamp %+v from tables that did not load", stamp)
	}
}
