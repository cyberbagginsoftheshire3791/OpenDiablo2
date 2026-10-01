package d2mapengine

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// FOG OF WAR, F1: TileSightClear, the fog's line of sight (tile_sight.go).

// blockTile marks every subtile of a tile the way the authored village's
// blocks_sight does (d2mapgen/authored.go): sight and walk, all 25.
func blockTile(t *testing.T, m *MapEngine, tx, ty int, walk, los bool) {
	t.Helper()

	tile := m.TileAt(tx, ty)
	if tile == nil {
		t.Fatalf("test setup: tile (%d,%d) is off the map", tx, ty)
	}

	for i := range tile.SubTiles {
		tile.SubTiles[i].BlockWalk = walk
		tile.SubTiles[i].BlockLOS = los
	}
}

// TestFogWallBlocksBehindButShowsItsFace: a wall 3 tiles from the eye is seen
// (its face); the tiles behind it on the same line are not; the open tile in
// front of it is.
//
// Negative controls (1 Oct 2026, strigoi-harness-runs\wt-fog\nc\): make
// tileBlocksSight answer false (the blocker ignored) and this fails, "the tile
// behind the wall (9,5) is seen through it" (nc4-blocker-ignored.txt); read
// the destination tile too (a blocked destination refused, as checkLos does)
// and it fails, "the wall's own face (8,5) is hidden" (nc5-destination-refused.txt).
func TestFogWallBlocksBehindButShowsItsFace(t *testing.T) {
	m := testEngine(20, 20)
	blockTile(t, m, 8, 5, true, true)

	const ex, ey = 5.5, 5.5

	for _, c := range []struct {
		tx, ty int
		want   bool
		what   string
	}{
		{7, 5, true, "the open tile in front of the wall"},
		{8, 5, true, "the wall's own face"},
		{9, 5, false, "the tile behind the wall"},
		{12, 5, false, "a tile far behind the wall"},
		{8, 7, true, "a tile beside the wall's line"},
		{5, 5, true, "the eye's own tile"},
	} {
		got, _ := m.TileSightClear(ex, ey, c.tx, c.ty)
		if got != c.want {
			if c.want {
				t.Errorf("%s (%d,%d) is hidden", c.what, c.tx, c.ty)
			} else {
				t.Errorf("%s (%d,%d) is seen through it", c.what, c.tx, c.ty)
			}
		}
	}

	// The cost counter: a ray of three tiles reads the two strictly between.
	if _, cells := m.TileSightClear(ex, ey, 8, 5); cells != 2 {
		t.Errorf("the ray to (8,5) read %d cells; the two strictly between are (6,5) and (7,5)", cells)
	}
}

// TestTileSightClearSharesTheSightRule: the fog obeys the map's ONE sight rule.
// Under the shipped rule (the sight bit) a walk-only tile does not block it;
// under SightBlockedByWalkFlag it does -- as it does for checkLos.
//
// Negative control (1 Oct 2026): make tileBlocksSight read BlockLOS directly
// instead of sightBlocked and this fails, "under the walk-flag rule a walk-only
// tile does not block the fog" (nc6-own-sight-rule.txt).
func TestTileSightClearSharesTheSightRule(t *testing.T) {
	m := testEngine(20, 20)
	blockTile(t, m, 7, 5, true, false) // a fence: not walkable, not opaque

	if clear, _ := m.TileSightClear(5.5, 5.5, 9, 5); !clear {
		t.Fatal("under the shipped rule (the sight bit) a walk-only tile blocks the fog")
	}

	m.setSightRule(SightBlockedByWalkFlag)

	if clear, _ := m.TileSightClear(5.5, 5.5, 9, 5); clear {
		t.Fatal("under the walk-flag rule a walk-only tile does not block the fog; it blocks checkLos")
	}

	if clear, _ := m.checkLos(d2vector.NewPosition(5.5*5, 5.5*5), d2vector.NewPosition(9.5*5, 5.5*5)); clear {
		t.Fatal("the control: checkLos under the walk-flag rule is not blocked either -- the setup is wrong")
	}
}

// TestTileSightThroughACornerNeedsBothSides: a ray along the exact diagonal
// passes through tile corners. One post beside the corner does not stop it; a
// diagonal wall (both tiles beside the corner) does. The answer is the same
// in both directions.
//
// Negative control (1 Oct 2026): break corner ties toward x (plain
// Amanatides-Woo, no tie case) and this fails, "with one post at (6,5) the
// diagonal (5,5)->(8,8) is blocked" (nc7-corner-ties-to-x.txt).
func TestTileSightThroughACornerNeedsBothSides(t *testing.T) {
	m := testEngine(20, 20)
	blockTile(t, m, 6, 5, true, true)

	if clear, _ := m.TileSightClear(5.5, 5.5, 8, 8); !clear {
		t.Fatal("with one post at (6,5) the diagonal (5,5)->(8,8) is blocked")
	}

	if clear, _ := m.TileSightClear(8.5, 8.5, 5, 5); !clear {
		t.Fatal("with one post at (6,5) the reverse diagonal (8,8)->(5,5) is blocked")
	}

	blockTile(t, m, 5, 6, true, true)

	if clear, _ := m.TileSightClear(5.5, 5.5, 8, 8); clear {
		t.Fatal("a diagonal wall, (6,5) and (5,6), lets the diagonal through its corner")
	}

	if clear, _ := m.TileSightClear(8.5, 8.5, 5, 5); clear {
		t.Fatal("the reverse ray goes through the diagonal wall")
	}
}

// TestTileSightOffTheMapBlocks: a ray that leaves the map is blocked, as
// checkLos's is; a tile on the map's edge is still seen.
func TestTileSightOffTheMapBlocks(t *testing.T) {
	m := testEngine(10, 10)

	if clear, _ := m.TileSightClear(1.5, 1.5, 0, 0); !clear {
		t.Fatal("the corner tile (0,0) next to the eye is hidden")
	}

	if clear, _ := m.TileSightClear(1.5, 1.5, -3, 1); clear {
		t.Fatal("a ray through tiles off the map is clear")
	}
}
