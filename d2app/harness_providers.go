//go:build harness

package d2app

// Phase 3 playtest harness — the provider tools (M3.4, P3 spec §3.5, §4.3,
// §4.4): list_systems / get_system_state / set_system_field over the
// d2harness registry. These three tools are the only ones that know about
// providers, and they never change as systems are added: a Phase 4 system
// registers at construction and is observable the day it exists.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// harnessPlannedSystems names the milestone that adds each system the S1 §12
// assertions will need, so an early call fails with the milestone in the
// message instead of a bare "unknown" (P3 spec §3.10).
var harnessPlannedSystems = map[string]string{
	// "clock" and "light" registered at M4.1, "meters" at M4.2 — they are
	// live whenever a game screen exists, so they are absent from this map
	// on purpose.
	"spawns":        "M4.3",
	"dead":          "M4.3 / M4.6",
	"combat":        "M4.5",
	"reputation":    "Phase 6",
	"inventory":     "Phase 6",
	"region":        "Phase 6",
	"soul_pressure": "Phase 4 (dashboard simulation)",
}

type harnessSystemInfo struct {
	Name           string   `json:"name"`
	Settable       bool     `json:"settable"`
	SettableFields []string `json:"settable_fields"`
}

type harnessListSystemsOut struct {
	Systems []harnessSystemInfo `json:"systems"`
	Total   int                 `json:"total"`
	Planned []string            `json:"planned_not_yet_registered"`
}

type harnessSystemIn struct {
	System string `json:"system" jsonschema:"a registered system name from strigoi_list_systems, e.g. ui"`
}

type harnessSystemStateOut struct {
	System   string                 `json:"system"`
	Settable bool                   `json:"settable"`
	State    map[string]interface{} `json:"state"`
}

type harnessSetFieldIn struct {
	System string      `json:"system" jsonschema:"a registered system name"`
	Field  string      `json:"field" jsonschema:"an allow-listed field of that system (see settable_fields)"`
	Value  interface{} `json:"value" jsonschema:"the new value; the system validates type and range"`
}

func harnessSystemInfoFor(p d2harness.Provider) harnessSystemInfo {
	info := harnessSystemInfo{Name: p.HarnessName(), SettableFields: []string{}}

	if _, ok := p.(d2harness.Settable); ok {
		info.Settable = true
	}

	if fl, ok := p.(d2harness.FieldLister); ok && fl.HarnessSettableFields() != nil {
		info.SettableFields = append(info.SettableFields, fl.HarnessSettableFields()...)
		sort.Strings(info.SettableFields)
	}

	return info
}

// harnessSystemErr turns a missing provider into the spec's error shape: a
// planned system names its milestone; anything else is UNKNOWN_SYSTEM.
func harnessSystemErr(name string) error {
	if ms, ok := harnessPlannedSystems[name]; ok {
		return harnessErr("NOT_IMPLEMENTED",
			fmt.Sprintf("system %q has no provider yet (arrives at %s)", name, ms),
			"strigoi_list_systems shows what is registered now")
	}

	return harnessErr("UNKNOWN_SYSTEM", fmt.Sprintf("no system named %q", name),
		"strigoi_list_systems shows the registered names")
}

func (a *App) harnessAddProviderTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_list_systems",
		Description: "The gameplay systems registered as harness providers (P3 spec §3.5), with the fields each allows strigoi_set_system_field to write. Also lists the planned systems that have no provider yet. Reads on the game goroutine.",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, harnessListSystemsOut, error) {
		harnessLogCall("strigoi_list_systems")

		out := harnessListSystemsOut{Systems: []harnessSystemInfo{}}

		err := harnessOnUpdate(func() {
			for _, name := range d2harness.Names() {
				p, ok := d2harness.Lookup(name)
				if !ok {
					continue
				}

				out.Systems = append(out.Systems, harnessSystemInfoFor(p))
			}
		})
		if err != nil {
			return nil, out, err
		}

		out.Total = len(out.Systems)

		registered := map[string]bool{}
		for _, s := range out.Systems {
			registered[s.Name] = true
		}

		out.Planned = []string{}

		for name, ms := range harnessPlannedSystems {
			if !registered[name] {
				out.Planned = append(out.Planned, name+" ("+ms+")")
			}
		}

		sort.Strings(out.Planned)

		names := make([]string, 0, len(out.Systems))
		for _, s := range out.Systems {
			names = append(names, s.Name)
		}

		return harnessText("%d registered system(s): %s · planned, not yet registered: %d", out.Total, strings.Join(names, ", "), len(out.Planned)), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_get_system_state",
		Description: "A registered system's observable state as the system itself exposes it (P3 spec §3.5). A planned system with no provider yet answers NOT_IMPLEMENTED naming its milestone. Reads on the game goroutine; pause first for a stable read.",
		Annotations: harnessAnnRO(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessSystemIn) (*mcp.CallToolResult, harnessSystemStateOut, error) {
		harnessLogCall("strigoi_get_system_state")

		name := strings.ToLower(strings.TrimSpace(in.System))
		out := harnessSystemStateOut{System: name}

		var toolErr error

		err := harnessOnUpdate(func() {
			p, ok := d2harness.Lookup(name)
			if !ok {
				toolErr = harnessSystemErr(name)
				return
			}

			_, out.Settable = p.(d2harness.Settable)
			out.State = p.HarnessState()
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		return harnessText("%s: %d field(s)", name, len(out.State)), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_set_system_field",
		Description: "Test setup: write one allow-listed field of a registered system (e.g. meters.fatigue at M4.2) and return the system's state afterwards. The system's own allow-list decides what is settable — FIELD_NOT_SETTABLE otherwise. Runs on the game goroutine.",
		Annotations: harnessAnnMut(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessSetFieldIn) (*mcp.CallToolResult, harnessSystemStateOut, error) {
		harnessLogCall("strigoi_set_system_field")

		name := strings.ToLower(strings.TrimSpace(in.System))
		out := harnessSystemStateOut{System: name}

		if strings.TrimSpace(in.Field) == "" {
			return nil, out, harnessErr("BAD_ARGUMENT", "field is required", "strigoi_list_systems shows settable_fields")
		}

		var toolErr error

		err := harnessOnUpdate(func() {
			p, ok := d2harness.Lookup(name)
			if !ok {
				toolErr = harnessSystemErr(name)
				return
			}

			s, ok := p.(d2harness.Settable)
			if !ok {
				toolErr = harnessErr("FIELD_NOT_SETTABLE",
					fmt.Sprintf("system %q exposes no settable fields", name),
					"it is read-only; drive it through game verbs or input instead")
				return
			}

			out.Settable = true

			if err := s.HarnessSet(in.Field, in.Value); err != nil {
				toolErr = harnessErr("FIELD_NOT_SETTABLE", fmt.Sprintf("%s.%s: %v", name, in.Field, err),
					"strigoi_list_systems shows settable_fields")
				return
			}

			out.State = p.HarnessState()
		})
		if err != nil {
			return nil, out, err
		}

		if toolErr != nil {
			return nil, out, toolErr
		}

		return harnessText("%s.%s set", name, in.Field), out, nil
	})
}
