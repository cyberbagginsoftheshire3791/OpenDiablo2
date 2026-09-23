package d2mapgen

import (
	"fmt"
	"image"
	"math"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2ds1"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2dt1"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// The authored map (M5.4): a Tiled map the player's world is built from in
// place of the generated Act 1 town and wilderness.
//
// It is process-wide rather than per-generator because both halves of a
// local game generate the world -- the in-process server builds its map, then
// the client rebuilds its own from the GenerateMap packet -- and both must
// build the same one. A package setting read by both is the one place that
// cannot disagree with itself. It is NOT one-shot, unlike the harness seed,
// for the same reason: the client's generation runs after the server's.
//
// A map that fails to load is refused WHOLE and the generated world is built
// instead, with the reason logged as an error and kept for AuthoredMapReport.
// That is a choice: the alternative is a game that will not start, and a
// designer iterating on a map is better served by a world plus a loud error
// than by nothing. What is never done is to build PART of a map.

// nolint:gochecknoglobals // deliberate process-wide setting, see above
var authored struct {
	sync.Mutex
	path  string // what was asked for; "" = the generated world
	built string // the path last built successfully
	err   error  // why the last attempt was refused
}

// SetAuthoredMap makes every later world an authored one, built from the .tmj
// at p (a game-relative path such as data/strigoi/maps/village.tmj; the
// leading slash is optional and backslashes are accepted). "" returns to the
// generated Act 1 world. It also forgets the last attempt's outcome.
func SetAuthoredMap(p string) {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, "/"))
	if p != "" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	authored.Lock()
	defer authored.Unlock()

	authored.path, authored.built, authored.err = p, "", nil
}

// AuthoredMapReport returns the map asked for, the map last built from it
// ("" if none was), and why the last attempt was refused (nil if it was not).
func AuthoredMapReport() (asked, built string, err error) {
	authored.Lock()
	defer authored.Unlock()

	return authored.path, authored.built, authored.err
}

func authoredMapPath() string {
	authored.Lock()
	defer authored.Unlock()

	return authored.path
}

func recordAuthored(p string, err error) {
	authored.Lock()
	defer authored.Unlock()

	// Both halves of a local game build the world, server first. The report
	// must not let the second half's success paper over the first half's
	// refusal: the first error stands until SetAuthoredMap, and "built" means
	// every build since then succeeded.
	if err != nil {
		if authored.err == nil {
			authored.err = err
		}

		authored.built = ""

		return
	}

	if authored.err == nil {
		authored.built = p
	}
}

// generateAuthored builds the world from the Tiled map at p. Every refusal
// that can be foreseen -- the file, its format, a monstat it names -- comes
// before the engine is touched; one that cannot (NewNPC failing) leaves a
// half-built map, which the caller's fallback resets along with everything
// else when it generates the Act 1 world.
func (g *MapGenerator) generateAuthored(p string) error {
	data, err := g.asset.LoadFile(p)
	if err != nil {
		return fmt.Errorf("loading %s: %w", p, err)
	}

	m, err := d2maptiled.Parse(data, path.Dir(p), g.asset.LoadFile)
	if err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}

	stats := make([]*d2records.MonStatRecord, len(m.NPCs))

	for i, n := range m.NPCs {
		if stats[i] = findMonstat(g.asset.Records.Monster.Stats, n.Monstat); stats[i] == nil {
			return fmt.Errorf("%s: npc %q names no monstats record", p, n.Monstat)
		}
	}

	// Act 1 town: the region every Strigoi system and the palette are keyed
	// to (the renderer builds its tile cache only for a level type with a
	// nonzero ID, and the client refuses a region it does not know).
	g.engine.ResetAuthoredMap(d2enum.RegionAct1Town, m.Width, m.Height)

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			*g.engine.Tile(x, y) = authoredTile(m, x, y)
		}
	}

	for i, n := range m.NPCs {
		npc, err := g.engine.NewNPC(subtile(n.X), subtile(n.Y), stats[i], 0)
		if err != nil {
			return fmt.Errorf("%s: npc %q: %w", p, n.Monstat, err)
		}

		g.engine.AddEntity(npc)
	}

	// The start by whole tiles. The server places a player at
	// int(start*5) + the middle-of-tile offset in sub-tiles, which assumes a
	// whole-tile start: handed the x.5 of a tile's centre it lands him on the
	// corner of the NEXT tile (the generated town's DS1 start does exactly
	// that -- 105.5 becomes 106.0), a tile the map never checked. The floor of
	// the start puts him inside the tile the map put its start in.
	g.engine.SetAuthored(authoredImages(m), math.Floor(m.StartX), math.Floor(m.StartY))
	g.engine.SetInside(m.Inside)
	g.Infof("authored map %s: %dx%d tiles, %d tile kinds, %d npc(s)", p, m.Width, m.Height, len(m.Kinds), len(m.NPCs))

	return nil
}

// authoredTile is one engine tile of an authored map: its floor and wall as
// tiles of the reserved style, and its walk and sight flags set directly on
// all 25 sub-tiles (v0 blocks whole tiles; a DT1 tile's per-sub-tile flags
// have no counterpart in a Tiled tileset yet).
func authoredTile(m *d2maptiled.Map, x, y int) d2mapengine.MapTile {
	var t d2mapengine.MapTile

	t.RegionType = d2enum.RegionAct1Town

	c := m.At(x, y)

	if c.Floor >= 0 {
		t.Components.Floors = []d2ds1.Tile{authoredDS1Tile(c.Floor, d2enum.TileFloor, 0)}
	}

	if c.Wall >= 0 {
		// Wall art stands on the floor diamond: its bottom 80 pixels are the
		// tile, so the image is drawn that much less its height above it.
		h := m.Kinds[c.Wall].Pixels.Bounds().Dy()
		t.Components.Walls = []d2ds1.Tile{authoredDS1Tile(c.Wall, d2mapengine.AuthoredWallType, d2maptiled.TileHeight-h)}
	}

	blocked, sight := m.Blocked(x, y), m.BlocksSight(x, y)

	for i := range t.SubTiles {
		t.SubTiles[i] = d2dt1.SubTileFlags{
			BlockWalk:       blocked,
			BlockPlayerWalk: blocked,
			BlockJump:       blocked,
			BlockLOS:        sight,
			BlockLight:      sight,
		}
	}

	return t
}

func authoredDS1Tile(kind int, typ d2enum.TileType, yAdjust int) d2ds1.Tile {
	var t d2ds1.Tile

	// Prop1 nonzero: the renderer skips a tile whose Prop1 is 0.
	t.Prop1 = 1
	t.Style = d2mapengine.AuthoredStyle
	t.Sequence = byte(kind)
	t.Type = typ
	t.YAdjust = yAdjust

	return t
}

// authoredImages keys each kind's art the way the renderer will look it up.
func authoredImages(m *d2maptiled.Map) map[d2mapengine.AuthoredKey]*image.RGBA {
	out := make(map[d2mapengine.AuthoredKey]*image.RGBA, len(m.Kinds))

	for i := range m.Kinds {
		typ := d2enum.TileFloor
		if m.Kinds[i].Layer == d2maptiled.LayerWall {
			typ = d2mapengine.AuthoredWallType
		}

		out[d2mapengine.AuthoredKey{Sequence: byte(i), Type: typ}] = m.Kinds[i].Pixels
	}

	return out
}

// findMonstat looks a monstats record up by id: exactly, then ignoring case
// (a designer typing "Warriv1" for "warriv1" means the same man). The
// case-folded pass walks the ids in sorted order so it cannot pick differently
// from run to run.
func findMonstat(stats d2records.MonStats, id string) *d2records.MonStatRecord {
	if rec := stats[id]; rec != nil {
		return rec
	}

	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		if strings.EqualFold(k, id) {
			return stats[k]
		}
	}

	return nil
}

// subtile converts a world-tile coordinate to the sub-tile NewNPC takes.
func subtile(v float64) int {
	return int(math.Floor(v * subtilesPerTile))
}

const subtilesPerTile = 5
