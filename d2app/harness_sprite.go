//go:build harness

package d2app

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// strigoi_describe_sprite (M5.3): what a sprite is, as the game draws it --
// its directions, frames and every frame's size, and whether a Strigoi PNG
// stands in for it (d2asset/sprite_override.go). It is how art for a
// Diablo II sprite gets its dimensions: describe the sprite, draw to fit, drop
// the PNG at the override path the answer names.

type harnessSpriteIn struct {
	Path    string `json:"path" jsonschema:"the sprite's game path as the game asks for it, e.g. /data/global/ui/CURSOR/ohand.DC6"`
	Palette string `json:"palette,omitempty" jsonschema:"palette for a Diablo II sprite; default the units palette"`
}

type harnessSpriteOut struct {
	Path         string   `json:"path"`
	Override     string   `json:"override,omitempty"` // the Strigoi PNG drawn instead, if any
	OverridePath string   `json:"override_path"`      // where a Strigoi PNG for it goes
	Directions   int      `json:"directions"`
	Frames       int      `json:"frames_per_direction"`
	Sizes        [][2]int `json:"frame_sizes"` // direction 0's frames, [w, h] each
	MaxW         int      `json:"max_w"`
	MaxH         int      `json:"max_h"`
}

func (a *App) harnessAddSpriteTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "strigoi_describe_sprite",
		Description: "A sprite's directions, frames and frame sizes as the game draws it, whether a Strigoi PNG stands in " +
			"for it, and where one would go (data/strigoi/override/<its path, lower-cased>.png). Loading it counts in the census.",
		Annotations: harnessAnnRO(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessSpriteIn) (*mcp.CallToolResult, harnessSpriteOut, error) {
		harnessLogCall("strigoi_describe_sprite")

		var (
			out     harnessSpriteOut
			loadErr error
		)

		if in.Path == "" {
			return nil, out, harnessErr("BAD_ARGUMENT", "no sprite path", "e.g. /data/global/ui/CURSOR/ohand.DC6")
		}

		err := harnessOnUpdate(func() {
			var info d2asset.SpriteInfo

			info, loadErr = a.asset.DescribeSprite(in.Path, in.Palette)
			out = harnessSpriteOut{
				Path: info.Path, Override: info.Override, OverridePath: d2asset.SpriteOverridePath(in.Path),
				Directions: info.Directions, Frames: info.Frames, Sizes: info.Sizes, MaxW: info.MaxW, MaxH: info.MaxH,
			}
		})
		if err != nil {
			return nil, out, err
		}

		if loadErr != nil {
			return nil, out, harnessErr("BAD_ARGUMENT", loadErr.Error(), "a .dc6, .dcc or .png the game can load")
		}

		return harnessText("%s: %d direction(s) x %d frame(s), up to %dx%d%s", in.Path, out.Directions, out.Frames,
			out.MaxW, out.MaxH, overrideNote(out.Override)), out, nil
	})
}

func overrideNote(override string) string {
	if override == "" {
		return ""
	}

	return fmt.Sprintf(" -- drawn from %s", override)
}
