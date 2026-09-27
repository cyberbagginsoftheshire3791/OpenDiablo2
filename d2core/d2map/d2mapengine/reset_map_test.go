package d2mapengine

import (
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// A generated map's level type is read from lvltypes.txt, which loads with
// that world (M5.3's tables burst). When it cannot load, ResetMap returns the
// reason and the caller refuses the map; it used to Fatalf, which ended the
// whole process -- a friend's game, or the harness and every script in it
// (the tables burst's review, B3, 27 Sep 2026). No MPQ is mounted here, so
// the read fails; GenerateMap, the one caller that is not the generator,
// logs it and leaves the map empty.
//
// Negative control (27 Sep 2026): put the Fatalf back and this test binary
// exits (status 1) before it reports anything.
func TestResetMapRefusesALevelTypeThatDidNotLoad(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	m := CreateMapEngine(d2util.LogLevelNone, asset)

	err = m.ResetMap(d2enum.RegionAct1Town, 4, 3)
	if err == nil || !strings.Contains(err.Error(), "lvltypes.txt did not load") {
		t.Fatalf("ResetMap with no level table returned %v, want the table's failure", err)
	}

	m.GenerateMap(d2enum.RegionAct1Wilderness, 2, 0)

	if len(m.Entities()) != 0 {
		t.Fatal("a refused map placed entities")
	}
}
