//go:build harness

package d2app

// Phase 3 playtest harness — spawn_entity / remove_entity (M3.4, P3 spec
// §4.4). Both wrap the engine's own entity factory (the paths the spawnmon /
// spawnitemat console commands use): the harness places what the game can
// already make and never invents a kind of its own. Phase 4 spawn tables add
// their kinds here when they exist.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

const harnessSubtilesPerTile = 5

type harnessSpawnIn struct {
	Kind string  `json:"kind" jsonschema:"npc (a monstats.txt Id, e.g. fallen1), item (space-separated item codes, e.g. 'hax' or 'cap'), object (an objects.txt index or name)"`
	Code string  `json:"code" jsonschema:"what to spawn, per kind"`
	X    float64 `json:"x" jsonschema:"world-tile x"`
	Y    float64 `json:"y" jsonschema:"world-tile y"`
}

type harnessSpawnOut struct {
	Handle string  `json:"handle"`
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Label  string  `json:"label,omitempty"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

type harnessRemoveIn struct {
	Handle string `json:"handle" jsonschema:"an entity handle (e:N); the local player p:1 cannot be removed"`
}

type harnessRemoveOut struct {
	Removed bool   `json:"removed"`
	Handle  string `json:"handle"`
	Kind    string `json:"kind,omitempty"`
}

func (a *App) harnessSpawn(kind, code string, x, y float64) (d2interface.MapEntity, error) {
	client, _ := harnessGame()
	if client == nil {
		return nil, harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
	}

	engine := client.MapEngine
	subX, subY := int(x*harnessSubtilesPerTile), int(y*harnessSubtilesPerTile)

	switch kind {
	case "npc":
		monstat := a.asset.Records.Monster.Stats[code]
		if monstat == nil {
			return nil, harnessErr("BAD_ARGUMENT", fmt.Sprintf("no monstats.txt entry %q", code), "use a monstats Id such as fallen1, zombie1, or skeleton1")
		}

		npc, err := engine.NewNPC(subX, subY, monstat, 0)
		if err != nil {
			return nil, harnessErr("INTERNAL", fmt.Sprintf("npc %q: %v", code, err), "")
		}

		engine.AddEntity(npc)

		return npc, nil

	case "item":
		codes := strings.Fields(code)
		if len(codes) == 0 {
			return nil, harnessErr("BAD_ARGUMENT", "item needs at least one item code", "e.g. 'hax' (hand axe) or 'cap'")
		}

		item, err := engine.NewItem(int(x), int(y), codes...)
		if err != nil {
			return nil, harnessErr("BAD_ARGUMENT", fmt.Sprintf("item %q: %v", code, err), "item codes come from weapons/armor/misc.txt")
		}

		engine.AddEntity(item)

		return item, nil

	case "object":
		rec := a.harnessObjectRecord(code)
		if rec == nil {
			return nil, harnessErr("BAD_ARGUMENT", fmt.Sprintf("no objects.txt entry %q", code), "pass the objects.txt index or exact name")
		}

		obj, err := engine.NewObject(subX, subY, rec, d2resource.PaletteUnits)
		if err != nil {
			return nil, harnessErr("INTERNAL", fmt.Sprintf("object %q: %v", code, err), "")
		}

		engine.AddEntity(obj)

		return obj, nil

	default:
		return nil, harnessErr("BAD_ARGUMENT", fmt.Sprintf("unknown kind %q", kind), "use npc, item, or object")
	}
}

// harnessObjectRecord resolves an objects.txt entry by index or by exact
// (case-insensitive) name; the lowest index wins a name tie.
func (a *App) harnessObjectRecord(code string) *d2records.ObjectDetailRecord {
	details := a.asset.Records.Object.Details

	if idx, err := strconv.Atoi(code); err == nil {
		return details[idx]
	}

	var best *d2records.ObjectDetailRecord

	for idx, rec := range details {
		if rec == nil || !strings.EqualFold(rec.Name, code) {
			continue
		}

		if best == nil || idx < best.Index {
			best = rec
		}
	}

	return best
}

func (a *App) harnessAddSpawnTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_spawn_entity",
		Description: "Put something in the world at a world-tile position through the engine's own factory: an npc (monstats Id), an item (item codes), or an object (objects.txt index or name). Returns a stable handle. Under a seeded run the new entity's ID and behaviour seed are reproducible.",
		Annotations: harnessAnnMut(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessSpawnIn) (*mcp.CallToolResult, harnessSpawnOut, error) {
		harnessLogCall("strigoi_spawn_entity")

		var out harnessSpawnOut

		kind := strings.ToLower(strings.TrimSpace(in.Kind))
		code := strings.TrimSpace(in.Code)

		if code == "" {
			return nil, out, harnessErr("BAD_ARGUMENT", "code is required", "")
		}

		var toolErr error

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			if client == nil {
				toolErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			if !client.MapEngine.TileExists(int(in.X), int(in.Y)) {
				toolErr = harnessErr("OUT_OF_BOUNDS", fmt.Sprintf("tile %d,%d is not on the map", int(in.X), int(in.Y)), "strigoi_get_tile reports what exists")
				return
			}

			e, err := a.harnessSpawn(kind, code, in.X, in.Y)
			if err != nil {
				toolErr = err
				return
			}

			info := harnessEntityInfoFor(e.ID(), e, client.PlayerID, false)
			out = harnessSpawnOut{Handle: info.Handle, ID: info.ID, Kind: info.Kind, Label: info.Label, X: info.X, Y: info.Y}
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		return harnessText("spawned %s %s (%s) at %.1f,%.1f", out.Kind, out.Handle, code, out.X, out.Y), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_remove_entity",
		Description: "Remove an entity by handle from the running map (npc, item, object, missile). Players cannot be removed. The handle stays known so a later get_entity reports it gone.",
		Annotations: harnessAnnMut(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessRemoveIn) (*mcp.CallToolResult, harnessRemoveOut, error) {
		harnessLogCall("strigoi_remove_entity")

		out := harnessRemoveOut{Handle: in.Handle}

		var toolErr error

		err := harnessOnUpdate(func() {
			client, _ := harnessGame()
			if client == nil {
				toolErr = harnessErr("NOT_IN_GAME", "no game is running", "call strigoi_start_game first")
				return
			}

			id, ok := harnessIDForHandle(in.Handle)
			if !ok {
				toolErr = harnessErr("UNKNOWN_HANDLE", fmt.Sprintf("no entity for handle %q", in.Handle), "list handles with strigoi_get_entities")
				return
			}

			e, ok := client.MapEngine.Entities()[id]
			if !ok {
				toolErr = harnessErr("UNKNOWN_HANDLE", fmt.Sprintf("entity %q is already gone", in.Handle), "")
				return
			}

			if _, isPlayer := e.(*d2mapentity.Player); isPlayer {
				toolErr = harnessErr("BAD_ARGUMENT", "players cannot be removed", "remove npcs, items, objects, or missiles")
				return
			}

			out.Kind = harnessEntityKind(e)

			client.MapEngine.RemoveEntity(e)

			out.Removed = true
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		return harnessText("removed %s %s", out.Kind, out.Handle), out, nil
	})
}
