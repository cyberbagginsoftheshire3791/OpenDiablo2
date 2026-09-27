package d2mapengine

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// An authored map is built without lvltypes.txt or levels.txt (M5.3's tables
// burst): its level type is the region enum it is given -- the one field the
// authored path ever read -- and its sound environment and name are its own.
// The asset manager here has no MPQ and no table, which is what Strigoi's
// game now builds the village with.
//
// Negative control: read the level type from the table again in
// ResetAuthoredMap and this panics on the empty table (index out of range);
// drop the ResetMap clear and the generated world after it keeps the
// village's region.
func TestAnAuthoredMapReadsNoLevelTable(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	m := CreateMapEngine(d2util.LogLevelError, asset)

	m.ResetAuthoredMap(d2enum.RegionAct1Town, 4, 3)

	if got := m.LevelType().ID; got != int(d2enum.RegionAct1Town) {
		t.Fatalf("level type %d, want the region's %d", got, d2enum.RegionAct1Town)
	}

	if asset.RecordsLoaded(d2resource.LevelType) || asset.RecordsLoaded(d2resource.LevelDetails) {
		t.Fatal("building an authored map loaded a level table")
	}

	if _, _, ok := m.AuthoredRegion(); ok {
		t.Fatal("a fresh map claims an authored region before the map says one")
	}

	m.SetAuthoredRegion(1, "Village")

	if env, name, ok := m.AuthoredRegion(); !ok || env != 1 || name != "Village" {
		t.Fatalf("authored region %d %q %v, want 1 Village true", env, name, ok)
	}

	// A later reset forgets it, as it forgets the art and the start.
	m.ResetAuthoredMap(d2enum.RegionAct1Town, 4, 3)

	if _, _, ok := m.AuthoredRegion(); ok {
		t.Fatal("a reset kept the last map's region")
	}
}
