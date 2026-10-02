package d2mapgen

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// villageWithoutTheTower is the shipped village with its watchtower object
// taken off the objects layer: the village as master had it, for walking and
// sight (fog of war F4 added the tower; the high ground changes neither).
func villageWithoutTheTower(tb testing.TB) (*d2mapengine.MapEngine, *d2maptiled.Map) {
	tb.Helper()

	root := filepath.Join("..", "..", "..", "data", "strigoi", "maps")
	strigoi := filepath.Dir(root)

	data, err := os.ReadFile(filepath.Join(root, "village.tmj"))
	if err != nil {
		tb.Fatal(err)
	}

	var tree map[string]any
	if err := json.Unmarshal(data, &tree); err != nil {
		tb.Fatal(err)
	}

	removed := 0

	for _, raw := range tree["layers"].([]any) {
		l := raw.(map[string]any)
		if l["name"] != "objects" {
			continue
		}

		kept := []any{}

		for _, o := range l["objects"].([]any) {
			if o.(map[string]any)["name"] == "placeholder-watchtower" {
				removed++

				continue
			}

			kept = append(kept, o)
		}

		l["objects"] = kept
	}

	if removed != 1 {
		tb.Fatalf("the village has %d watchtowers; want the one at the gate", removed)
	}

	if data, err = json.Marshal(tree); err != nil {
		tb.Fatal(err)
	}

	m, err := d2maptiled.Parse(data, "/data/strigoi/maps", func(p string) ([]byte, error) {
		return os.ReadFile(filepath.Join(strigoi, filepath.FromSlash(strings.TrimPrefix(path.Clean(p), "/data/strigoi"))))
	})
	if err != nil {
		tb.Fatalf("the village without its tower is refused: %v", err)
	}

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		tb.Fatal(err)
	}

	engine := d2mapengine.CreateMapEngine(d2util.LogLevelNone, asset)
	LayAuthoredMap(engine, m)

	return engine, m
}

// TestTheTowerChangesNoBeastsSight (the F4 review's A, 2 Oct 2026; decided on
// a default Josh can overturn): the gate's watchtower is an eye for HIS fog
// and nothing else sees by it -- it is "blocks_sight": false, so the beasts'
// line of sight (checkLos, LineOfSight) is the village's without the tower,
// for every pair of standable tiles about the gate (the review's region: one
// end within x 9..41, y 18..47, the other within x 17..33, y 26..42). What the
// tower does change for everyone is one tile of walking: its tile is solid,
// as every structure's is (a route through the gate may run a few subtiles
// longer; docs/fog.md).
//
// Negative control (2 Oct 2026, strigoi-harness-runs\wt-fog4\nc\): the
// tower's tile without "blocks_sight": false (it defaults to true for a
// structure) and this fails, "tile (25,34) blocks sight true with the tower,
// false without" and "5182 of 178846 beast sight answers moved"
// (nc-tower-blocks-sight.txt; the reviewer counted 5,182 too).
func TestTheTowerChangesNoBeastsSight(t *testing.T) {
	with, mw := villageEngine(t)
	without, mo := villageWithoutTheTower(t)

	size := with.Size()

	for y := 0; y < size.Height; y++ {
		for x := 0; x < size.Width; x++ {
			tower := x == 25 && y == 34

			if mw.BlocksSight(x, y) != mo.BlocksSight(x, y) {
				t.Errorf("tile (%d,%d) blocks sight %v with the tower, %v without", x, y, mw.BlocksSight(x, y), mo.BlocksSight(x, y))
			}

			if mw.Blocked(x, y) != mo.Blocked(x, y) && !tower {
				t.Errorf("tile (%d,%d) is blocked %v with the tower, %v without; only the tower's own tile may differ", x, y, mw.Blocked(x, y), mo.Blocked(x, y))
			}
		}
	}

	if !mw.Blocked(25, 34) {
		t.Error("the tower's tile (25,34) is walkable; a structure is solid")
	}

	moved, total := 0, 0

	for ay := 18; ay <= 47; ay++ {
		for ax := 9; ax <= 41; ax++ {
			if mw.Blocked(ax, ay) {
				continue
			}

			p := d2vector.NewPosition(float64(ax)*5+2.5, float64(ay)*5+2.5)

			for by := 26; by <= 42; by++ {
				for bx := 17; bx <= 33; bx++ {
					if mw.Blocked(bx, by) {
						continue
					}

					q := d2vector.NewPosition(float64(bx)*5+2.5, float64(by)*5+2.5)
					total++

					if with.LineOfSight(p, q) != without.LineOfSight(p, q) {
						moved++
					}
				}
			}
		}
	}

	t.Logf("%d of %d beast sight answers about the gate moved with the tower", moved, total)

	if moved != 0 || total < 100000 {
		t.Fatalf("%d of %d beast sight answers moved; the tower is an eye for his fog, not a wall to theirs", moved, total)
	}
}
