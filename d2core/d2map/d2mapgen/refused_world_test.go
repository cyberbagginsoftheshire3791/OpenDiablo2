package d2mapgen

import (
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
)

// Diablo II's generated world reads its tables when it is built. When they
// cannot load, GenerateAct1Overworld and GenerateHostWorld return the reason
// -- the server refuses the game with it (NewGameServer), a client says it is
// not the host's world -- where a Fatalf ended the whole process (the tables
// burst's review, B3, 27 Sep 2026). No MPQ is mounted here and no authored
// map is set, so the generated world is asked for and cannot be built.
//
// Negative control (27 Sep 2026): put generateAct1World's Fatalf back and
// this test binary exits (status 1) before it reports anything.
func TestAWorldThatCannotBeBuiltIsAnError(t *testing.T) {
	SetAuthoredMap("")

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, asset)

	g, err := NewMapGenerator(asset, d2util.LogLevelNone, engine)
	if err != nil {
		t.Fatal(err)
	}

	if err := g.GenerateAct1Overworld(); err == nil || !strings.Contains(err.Error(), "its tables did not load") {
		t.Fatalf("GenerateAct1Overworld with no tables returned %v, want the tables' failure", err)
	}

	// A client asked for a host's generated world fails the same way.
	if err := g.GenerateHostWorld("", ""); err == nil || !strings.Contains(err.Error(), "its tables did not load") {
		t.Fatalf("GenerateHostWorld with no tables returned %v, want the tables' failure", err)
	}
}
