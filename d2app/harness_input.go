//go:build harness

package d2app

// Phase 3 playtest harness — low-level input (M3.4, P3 spec §3.7, §4.4):
// strigoi_key, strigoi_click, strigoi_move_cursor, strigoi_type_text over the
// d2input.ScriptedInputService overlay (E6). Coordinates here are SCREEN
// PIXELS (800x600), the one place in the harness that is not world tiles.
//
// Timing: a scripted action is queued onto the game goroutine, applied at the
// top of a frame, and polled by the input manager later in that same frame.
// Every tool then waits for that frame to finish before returning, so the
// next tool call — a state read, a screenshot, a step — observes the effect.
// Under the paused clock, polls still happen every rendered frame, so input
// works whether the simulation is live, paused, or being stepped.

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2input"
)

type harnessKeyIn struct {
	Key    string `json:"key" jsonschema:"key name: a-z, 0-9, escape, enter, space, tab, backspace, f1-f12, up/down/left/right, shift, control, alt, graveaccent, ..."`
	Action string `json:"action,omitempty" jsonschema:"tap (default: press for exactly one input poll), down (hold until up), up (release)"`
}

type harnessClickIn struct {
	X      int      `json:"x" jsonschema:"screen pixel x (0..799)"`
	Y      int      `json:"y" jsonschema:"screen pixel y (0..599)"`
	Button string   `json:"button,omitempty" jsonschema:"left (default), right, middle"`
	Mods   []string `json:"mods,omitempty" jsonschema:"modifier keys held for the click: shift, control, alt"`
}

type harnessCursorIn struct {
	X int `json:"x" jsonschema:"screen pixel x"`
	Y int `json:"y" jsonschema:"screen pixel y"`
}

type harnessTypeTextIn struct {
	Text string `json:"text" jsonschema:"printable characters delivered on one input poll (the terminal and text boxes read them); use strigoi_key for enter/backspace"`
}

type harnessInputOut struct {
	Applied  string `json:"applied"`
	Tick     int64  `json:"tick_applied"`
	Mode     string `json:"mode"`
	CursorX  int    `json:"cursor_x"`
	CursorY  int    `json:"cursor_y"`
	Scripted bool   `json:"cursor_scripted"`
}

// harnessWaitFrameAfter blocks until the frame that ran the queued command
// has completed (its input poll included). harness.tick increments at the
// top of each frame before the queue drains, so tick > applied means the
// applied frame is over.
func harnessWaitFrameAfter(applied int64) error {
	deadline := time.Now().Add(harnessToolTimeout)

	for atomic.LoadInt64(&harness.tick) <= applied {
		if time.Now().After(deadline) {
			return errGameNotTicking
		}

		time.Sleep(time.Millisecond)
	}

	return nil
}

// harnessApplyInput runs fn on the game goroutine, waits out that frame, and
// reports the tick it landed on plus the cursor the game now sees.
func harnessApplyInput(what string, fn func()) (harnessInputOut, error) {
	out := harnessInputOut{Applied: what}

	if harness.input == nil {
		return out, harnessErr("INTERNAL", "the scripted input overlay is not installed", "")
	}

	err := harnessOnUpdate(func() {
		fn()
		out.Tick = atomic.LoadInt64(&harness.tick)
	})
	if err != nil {
		return out, err
	}

	if err := harnessWaitFrameAfter(out.Tick); err != nil {
		return out, err
	}

	err = harnessOnUpdate(func() {
		out.CursorX, out.CursorY = harness.input.Cursor()
		out.Scripted = harness.input.CursorScripted()
	})
	if err != nil {
		return out, err
	}

	out.Mode = harnessTimeSnapshot().Mode

	return out, nil
}

func (a *App) harnessAddInputTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_key",
		Description: "Scripted keyboard input at the engine's input seam: tap (one poll: OnKeyDown fires once, then a release), down (hold), up (release). Merged with the real keyboard. Returns after the frame that consumed it, so the next call sees the effect (e.g. strigoi_get_system_state ui after tapping i).",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessKeyIn) (*mcp.CallToolResult, harnessInputOut, error) {
		harnessLogCall("strigoi_key")

		key, err := d2input.KeyByName(in.Key)
		if err != nil {
			return nil, harnessInputOut{}, harnessErr("BAD_ARGUMENT", err.Error(), "names follow d2enum.Key without the prefix: i, escape, enter, f5, kp7, graveaccent")
		}

		action := strings.ToLower(strings.TrimSpace(in.Action))
		if action == "" {
			action = "tap"
		}

		var fn func()

		switch action {
		case "tap":
			fn = func() { harness.input.KeyTap(key) }
		case "down":
			fn = func() { harness.input.KeyDown(key) }
		case "up":
			fn = func() { harness.input.KeyUp(key) }
		default:
			return nil, harnessInputOut{}, harnessErr("BAD_ARGUMENT", fmt.Sprintf("unknown action %q", in.Action), "use tap, down, or up")
		}

		out, err := harnessApplyInput(fmt.Sprintf("key %s %s", d2input.KeyName(key), action), fn)
		if err != nil {
			return nil, out, err
		}

		return harnessText("%s at tick %d", out.Applied, out.Tick), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_click",
		Description: "Scripted mouse click at SCREEN PIXELS x,y (800x600): the cursor moves there and the button is pressed for one poll and released the next, with optional held modifiers (shift-click casts the left skill). A click on open ground walks the player there through the normal controls. Use strigoi_get_player's screen field to aim at the player.",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessClickIn) (*mcp.CallToolResult, harnessInputOut, error) {
		harnessLogCall("strigoi_click")

		button, err := d2input.MouseButtonByName(in.Button)
		if err != nil {
			return nil, harnessInputOut{}, harnessErr("BAD_ARGUMENT", err.Error(), "")
		}

		mods := make([]d2enum.Key, 0, len(in.Mods))

		for _, m := range in.Mods {
			key, err := d2input.KeyByName(m)
			if err != nil || (key != d2enum.KeyShift && key != d2enum.KeyControl && key != d2enum.KeyAlt) {
				return nil, harnessInputOut{}, harnessErr("BAD_ARGUMENT", fmt.Sprintf("unknown modifier %q", m), "use shift, control, or alt")
			}

			mods = append(mods, key)
		}

		if in.X < 0 || in.Y < 0 || in.X >= 800 || in.Y >= 600 {
			return nil, harnessInputOut{}, harnessErr("OUT_OF_BOUNDS", fmt.Sprintf("%d,%d is outside the 800x600 frame", in.X, in.Y), "")
		}

		out, err := harnessApplyInput(fmt.Sprintf("%s click at %d,%d", in.Button, in.X, in.Y), func() {
			for _, m := range mods {
				harness.input.KeyTap(m) // held for the same poll as the click
			}

			harness.input.Click(in.X, in.Y, button)
		})
		if err != nil {
			return nil, out, err
		}

		return harnessText("%s at tick %d", out.Applied, out.Tick), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_move_cursor",
		Description: "Place the scripted cursor at SCREEN PIXELS x,y. It stays until the real mouse moves (hover highlights, tooltips, and the next strigoi_click without coordinates all see it).",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessCursorIn) (*mcp.CallToolResult, harnessInputOut, error) {
		harnessLogCall("strigoi_move_cursor")

		if in.X < 0 || in.Y < 0 || in.X >= 800 || in.Y >= 600 {
			return nil, harnessInputOut{}, harnessErr("OUT_OF_BOUNDS", fmt.Sprintf("%d,%d is outside the 800x600 frame", in.X, in.Y), "")
		}

		out, err := harnessApplyInput(fmt.Sprintf("cursor to %d,%d", in.X, in.Y), func() {
			harness.input.MoveCursor(in.X, in.Y)
		})
		if err != nil {
			return nil, out, err
		}

		return harnessText("%s at tick %d", out.Applied, out.Tick), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "strigoi_type_text",
		Description: "Deliver printable characters on one input poll, as if typed (the in-game terminal and text boxes read them). Non-printable keys go through strigoi_key.",
		Annotations: harnessAnnMut(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in harnessTypeTextIn) (*mcp.CallToolResult, harnessInputOut, error) {
		harnessLogCall("strigoi_type_text")

		if in.Text == "" {
			return nil, harnessInputOut{}, harnessErr("BAD_ARGUMENT", "text is empty", "")
		}

		out, err := harnessApplyInput(fmt.Sprintf("typed %d character(s)", len([]rune(in.Text))), func() {
			harness.input.TypeText(in.Text)
		})
		if err != nil {
			return nil, out, err
		}

		return harnessText("%s at tick %d", out.Applied, out.Tick), out, nil
	})
}
