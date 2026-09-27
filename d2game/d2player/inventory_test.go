package d2player

import (
	"bytes"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// Diablo II's grid reads its layout (inventory.txt) the first time it is
// opened. A read that fails leaves the grid shut -- cleanly: no panel is built
// on a missing layout -- and the NEXT open reads it again, which is
// EnsureRecords' contract for a failed table (the tables burst's review, B3,
// 27 Sep 2026). Here no MPQ is mounted, so every read fails.
//
// Negative control (27 Sep 2026): set loaded before the read, as the burst
// shipped it, and this fails -- the second open reads nothing (one attempt
// logged, not two), and the grid stays shut for the rest of the game.
func TestTheGridRetriesALayoutThatDidNotLoad(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer

	logger := d2util.NewLogger()
	logger.Writer = &log
	logger.SetColorEnabled(false)
	logger.SetLevel(d2util.LogLevelError)

	// No UI manager: a Load that went on to build the panel on a missing
	// layout would fall over here.
	inv := &Inventory{asset: asset, recordKey: "Amazon", Logger: logger}

	for i := 1; i <= 2; i++ {
		inv.Open()

		if inv.IsOpen() || inv.grid != nil || inv.loaded {
			t.Fatalf("open %d: a grid whose layout did not load opened (open %v, grid %v, loaded %v)",
				i, inv.IsOpen(), inv.grid != nil, inv.loaded)
		}

		if n := strings.Count(log.String(), "the grid cannot open: its layout did not load"); n != i {
			t.Fatalf("after open %d the layout was read %d time(s), want %d -- a failed table is tried again:\n%s",
				i, n, i, log.String())
		}
	}
}
