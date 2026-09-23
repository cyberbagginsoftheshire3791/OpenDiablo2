//go:build playtest

package playtest

import (
	"image"
	"image/png"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// TestAuthoredMap is M5.4's playtest: the world built from a Tiled map
// (data/strigoi/maps/village.tmj) instead of from Diablo II's DS1 stamps.
//
// It holds the game against the map FILE, parsed here by the same package the
// game uses, so every expectation is a number the map's author chose rather
// than one the engine reported:
//
//  1. the village is built: reported as built, the player in its
//     player_start tile, the world its size and no bigger, and not one
//     Diablo II tile file read to build it;
//  2. its collision: the fence is solid on every sub-tile and the road is
//     open, a route out through the fence has to go round by the gate, a goal
//     outside the WEST fence is reached the long way round (out of the south
//     gate and up the outside -- the corridor), and a church tile cannot be
//     reached at all;
//  3. its art: the renderer's cached floor surface is the PNG in the repo,
//     pixel for pixel;
//  4. its people: the four speakers' stand-ins stand where the map put them,
//     under the labels the dialogue binds them by;
//  5. the player walks out of the gate on the authored collision;
//  6. control -- a map that does not exist is refused whole: the generated
//     Act 1 world is built instead, the refusal is reported, nothing of the
//     reserved tile style is in the renderer's cache, and the census now DOES
//     list tile files (so the zero in act 1 was a measurement).
func TestAuthoredMap(t *testing.T) {
	const (
		village = "/data/strigoi/maps/village.tmj"
		missing = "/data/strigoi/maps/missing.tmj"
	)

	m := parseShippedVillage(t)

	s := start(t)

	// --- act 1: the village is built --------------------------------------------
	// Asked without the leading slash, as the -map flag would be typed.
	g := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Mapper", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
		"map": strings.TrimPrefix(village, "/"),
	})

	if str(g, "map_built") != village || str(g, "map_error") != "" {
		t.Fatalf("the village: built %q error %q; want %q built cleanly", str(g, "map_built"), str(g, "map_error"), village)
	}

	// The same TILE: the engine places a player by whole tiles (d2mapgen's
	// SetAuthored call says why), so the start's fraction is not kept -- but
	// the tile must be the one the map chose and checked.
	sx, sy := pair(g, "spawn_tile")
	if int(sx) != int(m.StartX) || int(sy) != int(m.StartY) {
		t.Fatalf("spawned at %.2f,%.2f; the map's player_start is %.2f,%.2f", sx, sy, m.StartX, m.StartY)
	}

	if tile := s.call("strigoi_get_tile", map[string]any{"x": 120, "y": 120}); flag(t, tile, "exists") {
		t.Fatal("tile 120,120 exists: the generated world is still here around a 48-tile village")
	}

	// No Diablo II tile set was read for it: the village's tiles are all its
	// own, and the census counts every file the game has loaded since launch.
	if dt := tileFilesLoaded(t, s); len(dt) != 0 {
		t.Fatalf("building the village read %d Diablo II tile file(s), e.g. %s", len(dt), dt[0])
	}

	// --- act 2: collision ---------------------------------------------------------
	fence := s.call("strigoi_get_tile", map[string]any{"x": 12, "y": 20})
	if int(num(fence, "walls")) != 1 || walkable(fence) != 0 {
		t.Fatalf("the west fence at 12,20: %d walls, %d of 25 sub-tiles walkable; want 1 and 0",
			int(num(fence, "walls")), walkable(fence))
	}

	road := s.call("strigoi_get_tile", map[string]any{"x": 23, "y": 40})
	if int(num(road, "walls")) != 0 || walkable(road) != 25 {
		t.Fatalf("the road at 23,40: %d walls, %d of 25 walkable; want 0 and 25", int(num(road, "walls")), walkable(road))
	}

	// Out through the fence: the straight line from the spawn to just outside
	// the south fence, west of the gate, crosses the fence and the ditch; the
	// route that reaches it must step on no blocked tile, so it went round by
	// the gate. (Not straight_line_clear: that is the SIGHT ray, and a wattle
	// fence and a ditch are authored not to block sight.)
	ox, oy := 20.5, 37.5
	if !crossesBlocked(m, sx, sy, ox, oy) {
		t.Fatalf("the straight line %.1f,%.1f -> %.1f,%.1f crosses nothing blocked; the map has moved, pick another goal", sx, sy, ox, oy)
	}

	out := s.call("strigoi_find_path", map[string]any{"to_x": ox, "to_y": oy})
	if !flag(t, out, "reachable") {
		t.Fatalf("%.1f,%.1f outside the south fence is unreachable from the spawn: the gate does not open", ox, oy)
	}

	for _, raw := range out["waypoints"].([]any) {
		w := raw.(map[string]any)
		if x, y := int(num(w, "x")), int(num(w, "y")); m.Blocked(x, y) {
			t.Fatalf("the route steps on blocked tile %d,%d", x, y)
		}
	}

	// The long way round (d2mapengine/corridor.go): a goal just outside the
	// WEST fence, from the spawn, is a walk out of the south gate and back up
	// the outside -- far past one bounded search's budget, and unreachable
	// before the corridor.
	wx, wy := westOfTheFence(t, m, int(m.StartY))

	west := s.call("strigoi_find_path", map[string]any{"to_x": wx, "to_y": wy})
	if !flag(t, west, "reachable") {
		t.Fatalf("%.1f,%.1f outside the west fence is unreachable from the spawn: the long way round by the gate was not found", wx, wy)
	}

	// Every segment between corners is clear, and the route passes the gate
	// (fence ring y=35, x=23..24): it really went the long way round.
	px, py, viaGate := num(west, "from_x"), num(west, "from_y"), false

	for _, raw := range west["waypoints"].([]any) {
		w := raw.(map[string]any)
		x, y := num(w, "x"), num(w, "y")

		if crossesBlocked(m, px, py, x, y) {
			t.Fatalf("the long route's segment %.1f,%.1f -> %.1f,%.1f crosses a blocked tile", px, py, x, y)
		}

		if crossesRow(py, y, 35.5) {
			if cx := px + (x-px)*(35.5-py)/(y-py); cx >= 23 && cx < 25 {
				viaGate = true
			}
		}

		px, py = x, y
	}

	if !viaGate {
		t.Fatal("the long route to the west never crossed the gate row between x 23 and 25")
	}

	if church := s.call("strigoi_find_path", map[string]any{"to_x": 16.5, "to_y": 14.5}); flag(t, church, "reachable") {
		t.Fatal("a church tile (16,14) is reachable: its walls do not block")
	}

	// --- act 3: the art -------------------------------------------------------------
	items := floorDump(t, s)
	if len(items) == 0 || int(num(items[0], "style")) != authoredStyle {
		t.Fatalf("the floor cache starts %v; want the authored style %d", items, authoredStyle)
	}

	for _, it := range items {
		if int(num(it, "style")) != authoredStyle {
			continue
		}

		kind := m.Kinds[int(num(it, "sequence"))]
		dumped := readPNG(t, str(it, "path"))

		// Opaque pixels inside the diamond and a transparent one outside it:
		// a premultiplication or stride error shows in one or the other.
		for _, p := range [][2]int{{80, 40}, {40, 40}, {120, 40}, {80, 20}, {1, 1}, {158, 78}} {
			want := kind.Pixels.RGBAAt(p[0], p[1])

			r, gg, b, a := dumped.At(p[0], p[1]).RGBA()
			got := [4]uint8{uint8(r >> 8), uint8(gg >> 8), uint8(b >> 8), uint8(a >> 8)}

			if got != [4]uint8{want.R, want.G, want.B, want.A} {
				t.Fatalf("cached %s at %v is %v; the PNG in the repo has %v", kind.Name, p, got, want)
			}
		}
	}

	// --- act 4: the people ------------------------------------------------------------
	npcs := s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 50})
	labels := map[string][2]float64{}

	for _, raw := range npcs["items"].([]any) {
		e := raw.(map[string]any)
		labels[str(e, "label")] = [2]float64{num(e, "x"), num(e, "y")}
	}

	standIns := map[string]string{"warriv1": "Warriv", "kashya": "Kashya", "charsi": "Charsi", "akara": "Akara"}

	for _, n := range m.NPCs {
		label, ok := standIns[n.Monstat]
		if !ok {
			continue
		}

		at, found := labels[label]
		if !found {
			t.Fatalf("no npc labelled %q (the dialogue's stand-in for %s); labels: %v", label, n.Monstat, labels)
		}

		if math.Hypot(at[0]-n.X, at[1]-n.Y) > 0.5 {
			t.Fatalf("%s stands at %.2f,%.2f; the map put him at %.2f,%.2f", label, at[0], at[1], n.X, n.Y)
		}
	}

	shot := s.call("strigoi_screenshot", map[string]any{"name": "authored-village"})
	t.Logf("the village from its player_start: %s", str(shot, "path"))

	// --- act 5: out of the gate on the authored collision ---------------------------
	gx, gy := 23.5, 40.5
	move := s.call("strigoi_move_player_to", map[string]any{"x": gx, "y": gy, "wait": true, "max_ticks": 3000})

	p := s.call("strigoi_get_player", map[string]any{})
	if d := math.Hypot(num(p, "x")-gx, num(p, "y")-gy); d > 1.5 {
		t.Fatalf("walked for the road at %.1f,%.1f and stopped at %.2f,%.2f (%.2f off) [%v after %v ticks]",
			gx, gy, num(p, "x"), num(p, "y"), d, move["outcome"], move["ticks"])
	}

	// Evidence, not assertion: the gate by daylight, for a human to look at.
	// The game opens before dawn, when the village is all but black.
	s.call("strigoi_step_world", map[string]any{"world_minutes": 8 * 60})
	noon := s.call("strigoi_screenshot", map[string]any{"name": "authored-village-gate-day"})
	t.Logf("the gate by day: %s", str(noon, "path"))

	// --- act 6: the control -------------------------------------------------
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	s.call("strigoi_step", map[string]any{"frames": 30})

	bad := s.call("strigoi_start_game", map[string]any{
		"save_path": str(g, "save_path"), "seed": 1462, "wait_seconds": 90,
		"map": missing,
	})

	if str(bad, "map_asked") != missing || str(bad, "map_built") != "" || !strings.Contains(str(bad, "map_error"), "missing.tmj") {
		t.Fatalf("a missing map: asked %q built %q error %q; want it asked, not built, and refused by name",
			str(bad, "map_asked"), str(bad, "map_built"), str(bad, "map_error"))
	}

	if tile := s.call("strigoi_get_tile", map[string]any{"x": 120, "y": 120}); !flag(t, tile, "exists") {
		t.Fatal("control: the refused map did not fall back to the 150-tile generated world")
	}

	if items := floorDump(t, s); len(items) == 0 || int(num(items[0], "style")) == authoredStyle {
		t.Fatalf("control: the generated world's floor cache starts %v; want Diablo II tiles, not the authored style", items)
	}

	// And the census does see tile files when they are read: the generated
	// world reads the Act 1 town's, so the village's zero above is a real zero.
	if dt := tileFilesLoaded(t, s); len(dt) == 0 {
		t.Fatal("control: the generated world read no tile files by the census; the census is not seeing them")
	}
}

// authoredStyle is d2mapengine.AuthoredStyle, restated: the playtest package
// links no engine, and a test that imported the constant it is checking
// would agree with any value.
const authoredStyle = 250

func parseShippedVillage(t *testing.T) *d2maptiled.Map {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "data", "strigoi", "maps"))
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "village.tmj"))
	if err != nil {
		t.Fatalf("reading the village: %v", err)
	}

	m, err := d2maptiled.Parse(data, "/maps", func(p string) ([]byte, error) {
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path.Clean(p), "/maps"))))
	})
	if err != nil {
		t.Fatalf("the village does not parse outside the game either: %v", err)
	}

	return m
}

// crossesBlocked samples the straight line between two world points every
// tenth of a tile and reports whether any sample lands on a blocked tile.
func crossesBlocked(m *d2maptiled.Map, x0, y0, x1, y1 float64) bool {
	n := int(math.Hypot(x1-x0, y1-y0)*10) + 1

	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		if m.Blocked(int(x0+(x1-x0)*f), int(y0+(y1-y0)*f)) {
			return true
		}
	}

	return false
}

func walkable(tile map[string]any) int {
	n := 0

	if list, ok := tile["walkable_subtiles"].([]any); ok {
		for _, v := range list {
			if b, _ := v.(bool); b {
				n++
			}
		}
	}

	return n
}

func floorDump(t *testing.T, s *session) []map[string]any {
	t.Helper()

	out := s.call("strigoi_dump_surface", map[string]any{"kind": "floor_tile", "max": 16})

	var items []map[string]any

	if list, ok := out["items"].([]any); ok {
		for _, v := range list {
			items = append(items, v.(map[string]any))
		}
	}

	return items
}

func readPNG(t *testing.T, p string) image.Image {
	t.Helper()

	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("opening the dumped tile: %v", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decoding the dumped tile: %v", err)
	}

	return img
}

// tileFilesLoaded lists the Diablo II tile files (.dt1, .ds1) the "assets"
// census has recorded since launch.
func tileFilesLoaded(t *testing.T, s *session) []string {
	t.Helper()

	st := sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")
	t.Logf("census so far: %v files from the MPQs, %v of our own", st["mpq_files"], st["native_files"])

	var out []string

	list, ok := st["files"].([]any)
	if !ok {
		t.Fatalf("the assets census has no file list: %v", st)
	}

	for _, raw := range list {
		p := strings.ToLower(str(raw.(map[string]any), "path"))
		if strings.HasSuffix(p, ".dt1") || strings.HasSuffix(p, ".ds1") {
			out = append(out, p)
		}
	}

	return out
}

// crossesRow reports whether a segment from y0 to y1 crosses row y.
func crossesRow(y0, y1, y float64) bool {
	return (y0 < y && y1 >= y) || (y1 < y && y0 >= y)
}

// westOfTheFence finds an open tile outside the west fence on row y.
func westOfTheFence(t *testing.T, m *d2maptiled.Map, y int) (float64, float64) {
	t.Helper()

	for x := 9; x >= 2; x-- {
		if !m.Blocked(x, y) {
			return float64(x) + 0.5, float64(y) + 0.5
		}
	}

	t.Fatalf("no open tile west of the fence on row %d", y)

	return 0, 0
}
