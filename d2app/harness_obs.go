//go:build harness

package d2app

// Phase 3 playtest harness — observation tools (M3.2, P3 spec §4.3):
// entities, tiles, map windows, the log ring, screenshots, and the
// dump_surface diagnostic (§5.3, the black-floor experiment).

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
)

func harnessSystemNames() []string {
	return d2harness.Names()
}

// harnessEntityFor resolves a handle to the live entity behind it, including
// the p:1 shortcut for the local player. Pulled out of getEntity so the tools
// that need the ENTITY rather than a rendering of it (strigoi_pursue) resolve
// handles exactly the same way the observers do.
func harnessEntityFor(client *d2client.GameClient, handle string) (d2interface.MapEntity, bool) {
	if client == nil {
		return nil, false
	}

	id, ok := harnessIDForHandle(handle)
	if !ok {
		if handle != "p:1" || client.PlayerID == "" {
			return nil, false
		}

		id = client.PlayerID
	}

	e, ok := client.MapEngine.Entities()[id]

	return e, ok
}

// ---------------------------------------------------------------- entities --

type harnessEntityInfo struct {
	Handle  string                 `json:"handle"`
	ID      string                 `json:"id"`
	Kind    string                 `json:"kind"`
	Label   string                 `json:"label,omitempty"`
	X       float64                `json:"x"`
	Y       float64                `json:"y"`
	Tile    [2]int                 `json:"tile"`
	Layer   int                    `json:"layer"`
	Target  *[2]float64            `json:"target,omitempty"`
	PathLen int                    `json:"path_len,omitempty"`
	State   map[string]interface{} `json:"state,omitempty"`

	// Screen is the entity's current screen-pixel position (camera-relative;
	// presentation, so it is filled only by get_entity/get_player and never
	// by the digest). Use it to aim strigoi_click at an entity.
	Screen *[2]int `json:"screen,omitempty"`
}

type harnessGetEntitiesIn struct {
	Kind   string      `json:"kind,omitempty" jsonschema:"filter: player, npc, object, item, missile, other"`
	Near   *[3]float64 `json:"near,omitempty" jsonschema:"[x, y, radius] in world tiles"`
	Limit  int         `json:"limit,omitempty" jsonschema:"default 50"`
	Offset int         `json:"offset,omitempty"`
}

type harnessGetEntitiesOut struct {
	Items      []harnessEntityInfo `json:"items"`
	Total      int                 `json:"total"`
	HasMore    bool                `json:"has_more"`
	NextOffset int                 `json:"next_offset,omitempty"`
}

type harnessGetEntityIn struct {
	Handle string `json:"handle" jsonschema:"an entity handle from strigoi_get_entities, e.g. p:1 or e:3"`
}

func harnessEntityKind(e interface{}) string {
	switch e.(type) {
	case *d2mapentity.Player:
		return "player"
	case *d2mapentity.NPC:
		return "npc"
	case *d2mapentity.Object:
		return "object"
	case *d2mapentity.Item:
		return "item"
	case *d2mapentity.Missile:
		return "missile"
	default:
		return "other"
	}
}

// harnessEntityInfoFor builds the wire shape for one entity. Runs on the game
// goroutine. deep adds kind-specific state (get_entity).
func harnessEntityInfoFor(id string, e interface{}, localPlayerID string, deep bool) harnessEntityInfo {
	info := harnessEntityInfo{
		Handle: harnessHandleFor(id, localPlayerID),
		ID:     id,
		Kind:   harnessEntityKind(e),
	}

	if me, ok := e.(interface {
		Label() string
		GetLayer() int
	}); ok {
		info.Label = me.Label()
		info.Layer = me.GetLayer()
	}

	// Kind-specific state comes from the entity itself (d2harness.Stateful,
	// P3 spec §3.5): the entity decides what is observable, the harness only
	// relays it. The player's class is relayed as its name.
	if deep {
		if st, ok := e.(d2harness.Stateful); ok {
			info.State = st.HarnessState()
		}
	}

	if pl, ok := e.(*d2mapentity.Player); ok {
		world := pl.Position.World()
		info.X, info.Y = world.X(), world.Y()
		tile := pl.Position.Tile()
		info.Tile = [2]int{int(tile.X()), int(tile.Y())}

		if info.State != nil {
			info.State["class"] = harnessHeroName(pl.Class)
		}

		return info
	}

	if npc, ok := e.(*d2mapentity.NPC); ok {
		world := npc.Position.World()
		info.X, info.Y = world.X(), world.Y()
		tile := npc.Position.Tile()
		info.Tile = [2]int{int(tile.X()), int(tile.Y())}

		return info
	}

	if me, ok := e.(interface{ GetPositionF() (float64, float64) }); ok {
		// GetPositionF returns sub-tile coordinates; world tiles are /5
		sx, sy := me.GetPositionF()
		info.X, info.Y = sx/5, sy/5
		info.Tile = [2]int{int(info.X), int(info.Y)}
	}

	return info
}

// ------------------------------------------------------------------- tiles --

type harnessGetTileIn struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type harnessGetTileOut struct {
	Exists    bool   `json:"exists"`
	Region    int    `json:"region"`
	LevelName string `json:"level_name,omitempty"`
	Walkable  []bool `json:"walkable_subtiles,omitempty"`
	Floors    int    `json:"floors"`
	Walls     int    `json:"walls"`
	Shadows   int    `json:"shadows"`
}

// harnessFindPathIn asks for a route WITHOUT walking it. Coordinates are world
// tiles, matching strigoi_move_player_to and every other tool a script writes;
// the search itself works in subtiles and the conversion happens here, so a
// script never has to remember which space it is in.
type harnessFindPathIn struct {
	// omitempty is what makes these OPTIONAL in the generated JSON schema: the
	// go-sdk marks every field without it as required, and a start point that
	// must always be supplied would defeat the point of defaulting it.
	FromX float64 `json:"from_x,omitempty" jsonschema:"start x in world tiles; defaults to the player's position"`
	FromY float64 `json:"from_y,omitempty" jsonschema:"start y in world tiles; defaults to the player's position"`
	ToX   float64 `json:"to_x" jsonschema:"goal x in world tiles"`
	ToY   float64 `json:"to_y" jsonschema:"goal y in world tiles"`
}

// harnessPathPoint is one waypoint, reported in both spaces because assertions
// want different ones: world tiles read naturally, subtiles are what the search
// actually computed and are therefore what a byte-identical comparison should
// be made on.
type harnessPathPoint struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	SubX  float64 `json:"sub_x"`
	SubY  float64 `json:"sub_y"`
	Index int     `json:"index"`
}

type harnessFindPathOut struct {
	FromX     float64            `json:"from_x"`
	FromY     float64            `json:"from_y"`
	ToX       float64            `json:"to_x"`
	ToY       float64            `json:"to_y"`
	Reachable bool               `json:"reachable"`
	Waypoints []harnessPathPoint `json:"waypoints"`
	Count     int                `json:"waypoint_count"`
	// StraightLineClear reports whether the OLD raycast would have made it:
	// the negative control for "this route goes around something". Without it
	// a passing path assertion cannot tell a real detour from an unobstructed
	// walk that never needed one.
	StraightLineClear bool `json:"straight_line_clear"`
}

type harnessDumpMapIn struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	W      int    `json:"w" jsonschema:"window width in tiles, max 64"`
	H      int    `json:"h" jsonschema:"window height in tiles, max 64"`
	Layer  string `json:"layer,omitempty" jsonschema:"walk (default, sub-tile resolution), entities, region"`
	Format string `json:"format,omitempty" jsonschema:"ascii (default) or json"`
}

type harnessDumpMapOut struct {
	Layer  string `json:"layer"`
	Format string `json:"format"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	Text   string `json:"text"`
}

type harnessReadLogIn struct {
	Cursor  int    `json:"cursor,omitempty" jsonschema:"return lines with seq greater than this; 0 = from the beginning of the ring"`
	Pattern string `json:"pattern,omitempty" jsonschema:"optional RE2 filter"`
	Limit   int    `json:"limit,omitempty" jsonschema:"default 200"`
}

type harnessReadLogOut struct {
	Lines      []harnessLogLine `json:"lines"`
	NextCursor int              `json:"next_cursor"`
	Dropped    int              `json:"dropped"`
}

// -------------------------------------------------------------- screenshots --

type harnessScreenshotIn struct {
	Name   string  `json:"name,omitempty" jsonschema:"base file name; default shot"`
	Region *[4]int `json:"region,omitempty" jsonschema:"[x, y, w, h] crop in screen pixels"`
	Inline bool    `json:"inline,omitempty" jsonschema:"also return the PNG as an image content block"`
}

type harnessScreenshotOut struct {
	Path string `json:"path"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	Tick int64  `json:"tick"`
}

type harnessDumpSurfaceIn struct {
	Kind string `json:"kind" jsonschema:"floor_tile is the only kind in M3.2 (the black-floor diagnostic)"`
	Max  int    `json:"max,omitempty" jsonschema:"how many distinct cached tiles to dump; default 4"`
}

type harnessDumpSurfaceItem struct {
	Style      int    `json:"style"`
	Sequence   int    `json:"sequence"`
	Index      int    `json:"random_index"`
	W          int    `json:"w"`
	H          int    `json:"h"`
	OpaquePx   int    `json:"opaque_px"`
	NonBlackPx int    `json:"non_black_px"`
	Path       string `json:"path"`
}

type harnessDumpSurfaceOut struct {
	Items []harnessDumpSurfaceItem `json:"items"`
}

func (a *App) harnessAddObservationTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_get_entities",
		Description: "List entities with stable handles (p:1 is the local player). Coordinates are world tiles. Paginated; filter by kind or near=[x,y,radius].",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessGetEntitiesIn) (*mcp.CallToolResult, harnessGetEntitiesOut, error) {
		harnessLogCall("strigoi_get_entities")

		var out harnessGetEntitiesOut

		var toolErr error

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			if client == nil {
				toolErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			entities := client.MapEngine.Entities()

			ids := make([]string, 0, len(entities))
			for id := range entities {
				ids = append(ids, id)
			}

			sort.Strings(ids)

			var all []harnessEntityInfo

			for _, id := range ids {
				info := harnessEntityInfoFor(id, entities[id], client.PlayerID, false)

				if in.Kind != "" && info.Kind != strings.ToLower(in.Kind) {
					continue
				}

				if in.Near != nil {
					dx, dy := info.X-in.Near[0], info.Y-in.Near[1]
					if dx*dx+dy*dy > in.Near[2]*in.Near[2] {
						continue
					}
				}

				all = append(all, info)
			}

			sort.Slice(all, func(i, j int) bool { return harnessHandleLess(all[i].Handle, all[j].Handle) })

			limit := in.Limit
			if limit <= 0 || limit > 200 {
				limit = 50
			}

			out.Total = len(all)

			if in.Offset < len(all) {
				end := in.Offset + limit
				if end > len(all) {
					end = len(all)
				}

				out.Items = all[in.Offset:end]
				out.HasMore = end < len(all)

				if out.HasMore {
					out.NextOffset = end
				}
			}
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		return harnessText("%d of %d entities", len(out.Items), out.Total), out, nil
	})

	getEntity := func(handle string) (*mcp.CallToolResult, harnessEntityInfo, error) {
		var out harnessEntityInfo

		var toolErr error

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			if client == nil {
				toolErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			id, ok := harnessIDForHandle(handle)
			if !ok {
				if handle == "p:1" && client.PlayerID != "" {
					id = client.PlayerID
				} else {
					toolErr = harnessErr("UNKNOWN_HANDLE", fmt.Sprintf("no entity for handle %q", handle), "list handles with strigoi_get_entities")
					return
				}
			}

			e, ok := client.MapEngine.Entities()[id]
			if !ok {
				toolErr = harnessErr("UNKNOWN_HANDLE", fmt.Sprintf("entity %q is gone", handle), "it may have been removed; re-list")
				return
			}

			out = harnessEntityInfoFor(id, e, client.PlayerID, true)

			// Presentation extra for input scripts (never part of the digest).
			if _, game := harnessGame(); game != nil {
				if mr := game.HarnessMapRenderer(); mr != nil {
					sx, sy := mr.WorldToScreen(out.X, out.Y)
					out.Screen = &[2]int{sx, sy}
				}
			}
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		return harnessText("%s %s at %.1f,%.1f", out.Kind, out.Handle, out.X, out.Y), out, nil
	}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_get_entity",
		Description: "One entity in full, including kind-specific state (player: stamina, gold, act, flags).",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessGetEntityIn) (*mcp.CallToolResult, harnessEntityInfo, error) {
		harnessLogCall("strigoi_get_entity")
		return getEntity(in.Handle)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_get_player",
		Description: "The local player in full (shortcut for strigoi_get_entity p:1).",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessEntityInfo, error) {
		harnessLogCall("strigoi_get_player")
		return getEntity("p:1")
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_get_tile",
		Description: "One map tile: region, level name, per-subtile walkability (5x5, true = walkable), and component counts.",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessGetTileIn) (*mcp.CallToolResult, harnessGetTileOut, error) {
		harnessLogCall("strigoi_get_tile")

		var out harnessGetTileOut

		var toolErr error

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			if client == nil {
				toolErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			engine := client.MapEngine
			if !engine.TileExists(in.X, in.Y) {
				return
			}

			tile := engine.TileAt(in.X, in.Y)
			if tile == nil {
				return
			}

			out.Exists = true
			out.Region = int(tile.RegionType)
			out.Floors = len(tile.Components.Floors)
			out.Walls = len(tile.Components.Walls)
			out.Shadows = len(tile.Components.Shadows)

			if rec := a.asset.Records.Level.Details[out.Region]; rec != nil {
				out.LevelName = rec.LevelDisplayName
			}

			out.Walkable = make([]bool, 0, 25)

			for sy := 0; sy < 5; sy++ {
				for sx := 0; sx < 5; sx++ {
					flags := engine.SubTileAt(in.X*5+sx, in.Y*5+sy)
					out.Walkable = append(out.Walkable, flags == nil || !flags.BlockWalk)
				}
			}
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		return harnessText("tile %d,%d exists=%v region=%d %s", in.X, in.Y, out.Exists, out.Region, out.LevelName), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "strigoi_find_path",
		Description: "The route the pathfinder would take between two points, WITHOUT walking it. " +
			"Coordinates are world tiles; from_x/from_y default to the player. Returns the waypoints in " +
			"travel order (world tiles and subtiles), whether the goal is reachable, and whether the straight " +
			"line between the two points is clear -- that last one is the negative control, because a route " +
			"that arrives proves nothing unless the straight line did not.",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessFindPathIn) (*mcp.CallToolResult, harnessFindPathOut, error) {
		harnessLogCall("strigoi_find_path")

		var out harnessFindPathOut

		var toolErr error

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			if client == nil {
				toolErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			fromX, fromY := in.FromX, in.FromY

			// Defaulting to the player is the common case by a wide margin,
			// and a script that has to read its own position first to ask a
			// question about it is a script with a race in it.
			if fromX == 0 && fromY == 0 {
				if p, ok := client.Players[client.PlayerID]; ok && p != nil {
					world := p.Position.World()
					fromX, fromY = world.X(), world.Y()
				}
			}

			start := d2vector.NewPositionTile(fromX, fromY)
			goal := d2vector.NewPositionTile(in.ToX, in.ToY)

			engine := client.MapEngine
			path := engine.PathFind(start, goal)

			out.FromX, out.FromY = fromX, fromY
			out.ToX, out.ToY = in.ToX, in.ToY
			out.StraightLineClear = engine.LineOfSight(start, goal)
			out.Waypoints = make([]harnessPathPoint, 0, len(path))

			for i := range path {
				world := path[i].World()
				out.Waypoints = append(out.Waypoints, harnessPathPoint{
					Index: i,
					X:     world.X(),
					Y:     world.Y(),
					SubX:  path[i].X(),
					SubY:  path[i].Y(),
				})
			}

			out.Count = len(out.Waypoints)

			// Reachable means the route actually ends at the goal tile rather
			// than at the best partial approach the bounded search managed.
			if out.Count > 0 {
				last := out.Waypoints[out.Count-1]
				out.Reachable = math.Floor(last.X) == math.Floor(in.ToX) &&
					math.Floor(last.Y) == math.Floor(in.ToY)
			}
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		return harnessText("path %.1f,%.1f -> %.1f,%.1f: %d waypoints, reachable=%v, straight line clear=%v",
			out.FromX, out.FromY, out.ToX, out.ToY, out.Count, out.Reachable, out.StraightLineClear), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_dump_map",
		Description: "A windowed map dump (max 64x64 tiles). Layers: walk (sub-tile resolution, '#' blocked '.' free), entities (P player, N npc, O object, I item, M missile), region (tile region ids).",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessDumpMapIn) (*mcp.CallToolResult, harnessDumpMapOut, error) {
		harnessLogCall("strigoi_dump_map")

		out := harnessDumpMapOut{Layer: in.Layer, Format: in.Format}

		if out.Layer == "" {
			out.Layer = "walk"
		}

		if out.Format == "" {
			out.Format = "ascii"
		}

		if in.W <= 0 || in.H <= 0 || in.W > 64 || in.H > 64 {
			return nil, out, harnessErr("BAD_ARGUMENT", "w and h must be 1..64 tiles", "")
		}

		var toolErr error

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			if client == nil {
				toolErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			engine := client.MapEngine

			var b strings.Builder

			switch out.Layer {
			case "walk":
				for sy := 0; sy < in.H*5; sy++ {
					for sx := 0; sx < in.W*5; sx++ {
						flags := engine.SubTileAt(in.X*5+sx, in.Y*5+sy)
						if flags != nil && flags.BlockWalk {
							b.WriteByte('#')
						} else {
							b.WriteByte('.')
						}
					}

					b.WriteByte('\n')
				}
			case "entities":
				grid := make([][]byte, in.H)
				for i := range grid {
					grid[i] = []byte(strings.Repeat(".", in.W))
				}

				for id, e := range client.MapEngine.Entities() {
					info := harnessEntityInfoFor(id, e, client.PlayerID, false)

					gx, gy := info.Tile[0]-in.X, info.Tile[1]-in.Y
					if gx < 0 || gy < 0 || gx >= in.W || gy >= in.H {
						continue
					}

					marks := map[string]byte{"player": 'P', "npc": 'N', "object": 'O', "item": 'I', "missile": 'M', "other": '?'}
					grid[gy][gx] = marks[info.Kind]
				}

				for _, row := range grid {
					b.Write(row)
					b.WriteByte('\n')
				}
			case "region":
				for ty := 0; ty < in.H; ty++ {
					for tx := 0; tx < in.W; tx++ {
						if !engine.TileExists(in.X+tx, in.Y+ty) {
							b.WriteString(" -")
							continue
						}

						tile := engine.TileAt(in.X+tx, in.Y+ty)
						b.WriteString(fmt.Sprintf("%2d", int(tile.RegionType)))
					}

					b.WriteByte('\n')
				}
			default:
				toolErr = harnessErr("BAD_ARGUMENT", fmt.Sprintf("unknown layer %q", out.Layer), "use walk, entities, or region")
				return
			}

			out.W, out.H = in.W, in.H
			out.Text = b.String()
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		return harnessText("%s layer, %dx%d tiles at %d,%d:\n%s", out.Layer, out.W, out.H, in.X, in.Y, out.Text), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_read_log",
		Description: "Logger output since a cursor (a ring of the last 5000 lines from every engine logger). Pass the returned next_cursor to poll incrementally; pattern is an RE2 filter.",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessReadLogIn) (*mcp.CallToolResult, harnessReadLogOut, error) {
		harnessLogCall("strigoi_read_log")

		var out harnessReadLogOut

		limit := in.Limit
		if limit <= 0 || limit > 1000 {
			limit = 200
		}

		var pattern *regexp.Regexp

		if in.Pattern != "" {
			var err error

			pattern, err = regexp.Compile(in.Pattern)
			if err != nil {
				return nil, out, harnessErr("BAD_ARGUMENT", fmt.Sprintf("bad pattern: %v", err), "RE2 syntax")
			}
		}

		out.Lines, out.NextCursor, out.Dropped = harness.ring.since(in.Cursor, pattern, limit)

		return harnessText("%d line(s), next_cursor %d", len(out.Lines), out.NextCursor), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_screenshot",
		Description: "PNG of the next rendered frame, written to the run directory (never the repo — the pixels are Blizzard's). Optional [x,y,w,h] crop; inline=true also returns the image so you can look at it.",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessScreenshotIn) (*mcp.CallToolResult, harnessScreenshotOut, error) {
		harnessLogCall("strigoi_screenshot")

		var out harnessScreenshotOut

		var img *image.RGBA

		var toolErr error

		err := harnessOnDraw(func() {
			if harness.drawTarget == nil {
				toolErr = harnessErr("INTERNAL", "no draw target", "")
				return
			}

			img = harness.drawTarget.Screenshot()
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		if in.Region != nil {
			r := image.Rect(in.Region[0], in.Region[1], in.Region[0]+in.Region[2], in.Region[1]+in.Region[3])
			r = r.Intersect(img.Bounds())

			if r.Empty() {
				return nil, out, harnessErr("OUT_OF_BOUNDS", "the crop region is outside the frame", "the frame is 800x600")
			}

			img = img.SubImage(r).(*image.RGBA)
		}

		name := filepath.Base(in.Name)
		if name == "" || name == "." || name == string(filepath.Separator) {
			name = "shot"
		}

		name = strings.TrimSuffix(name, ".png")
		out.Tick = atomic.LoadInt64(&harness.tick)
		out.Path = filepath.Join(harness.runDir, fmt.Sprintf("%s-%08d.png", name, out.Tick))
		out.W = img.Bounds().Dx()
		out.H = img.Bounds().Dy()

		f, err := os.Create(out.Path)
		if err != nil {
			return nil, out, harnessErr("INTERNAL", fmt.Sprintf("cannot write %q: %v", out.Path, err), "")
		}

		encErr := png.Encode(f, img)
		closeErr := f.Close()

		if encErr != nil || closeErr != nil {
			return nil, out, harnessErr("INTERNAL", fmt.Sprintf("png encode: %v / %v", encErr, closeErr), "")
		}

		result := harnessText("%dx%d frame at tick %d -> %s", out.W, out.H, out.Tick, out.Path)

		if in.Inline {
			data, err := os.ReadFile(out.Path)
			if err == nil {
				result.Content = append(result.Content, &mcp.ImageContent{Data: data, MIMEType: "image/png"})
			}
		}

		return result, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_dump_surface",
		Description: "The black-floor diagnostic (P3 spec §5.3): dump cached floor-tile surfaces to PNGs in the run directory and report opaque / non-black pixel counts. If the PNGs show the isometric diamond, the bug is compositing/positioning; if they are blank, it is the NewSurface/ReplacePixels path.",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessDumpSurfaceIn) (*mcp.CallToolResult, harnessDumpSurfaceOut, error) {
		harnessLogCall("strigoi_dump_surface")

		var out harnessDumpSurfaceOut

		if strings.ToLower(in.Kind) != "floor_tile" {
			return nil, out, harnessErr("BAD_ARGUMENT", fmt.Sprintf("unknown kind %q", in.Kind), "floor_tile is the only kind in M3.2")
		}

		maxTiles := in.Max
		if maxTiles <= 0 || maxTiles > 16 {
			maxTiles = 4
		}

		var toolErr error

		err := harnessOnDraw(func() {
			_, game := harnessGame()
			if game == nil {
				toolErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			mr := game.HarnessMapRenderer()
			if mr == nil {
				toolErr = harnessErr("NOT_IN_GAME", "the map renderer is not up yet", "wait for start_game to report ready")
				return
			}

			for _, item := range mr.HarnessCachedFloorTiles(maxTiles) {
				img := item.Surface.Screenshot()
				if img == nil {
					continue
				}

				opaque, nonBlack := 0, 0

				for i := 0; i+3 < len(img.Pix); i += 4 {
					if img.Pix[i+3] == 0 {
						continue
					}

					opaque++

					if img.Pix[i] > 8 || img.Pix[i+1] > 8 || img.Pix[i+2] > 8 {
						nonBlack++
					}
				}

				path := filepath.Join(harness.runDir,
					"floortile-"+strconv.Itoa(item.Style)+"-"+strconv.Itoa(item.Sequence)+"-"+strconv.Itoa(item.RandomIndex)+".png")

				if f, err := os.Create(path); err == nil {
					_ = png.Encode(f, img)
					_ = f.Close()
				}

				out.Items = append(out.Items, harnessDumpSurfaceItem{
					Style:      item.Style,
					Sequence:   item.Sequence,
					Index:      item.RandomIndex,
					W:          img.Bounds().Dx(),
					H:          img.Bounds().Dy(),
					OpaquePx:   opaque,
					NonBlackPx: nonBlack,
					Path:       path,
				})
			}
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		summary := make([]string, 0, len(out.Items))
		for _, it := range out.Items {
			summary = append(summary, fmt.Sprintf("%d-%d-%d: %dx%d opaque=%d nonblack=%d", it.Style, it.Sequence, it.Index, it.W, it.H, it.OpaquePx, it.NonBlackPx))
		}

		return harnessText("%d cached floor tile(s): %s", len(out.Items), strings.Join(summary, " · ")), out, nil
	})
}

// harnessHandleLess orders p:* before e:* and numerically within each.
func harnessHandleLess(a, b string) bool {
	pa, pb := strings.HasPrefix(a, "p:"), strings.HasPrefix(b, "p:")
	if pa != pb {
		return pa
	}

	na, _ := strconv.Atoi(a[strings.Index(a, ":")+1:])
	nb, _ := strconv.Atoi(b[strings.Index(b, ":")+1:])

	if na != nb {
		return na < nb
	}

	return a < b
}
