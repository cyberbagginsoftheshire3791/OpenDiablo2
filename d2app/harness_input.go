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
	Hold   int      `json:"hold_frames,omitempty" jsonschema:"hold the button DOWN for this many frames before releasing (default 1, a tap). 2 or more is what reaches the click-and-hold path: GameControls.OnMouseButtonRepeat and its 0.25 s threshold, which a tap cannot reach by construction (BUG-7). PAUSED, each held frame is one dt tick, so the simulation (sim_seconds, world time) moves N ticks"`
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
	start := atomic.LoadInt64(&harness.tick)

	for atomic.LoadInt64(&harness.tick) <= applied {
		if time.Now().After(deadline) {
			return harnessNotTicking(start)
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

// harnessHoldFramesMax bounds hold_frames so a script cannot wedge the tool for
// longer than the tool timeout allows. At 60 fps 600 frames is ten seconds,
// which is two orders of magnitude past the 0.25 s threshold it exists to cross.
const harnessHoldFramesMax = 600

// harnessApplyHeldClick presses a mouse button, holds it down across whole
// frames, and releases it.
//
// IT EXISTS BECAUSE NO PLAYTEST COULD EXERCISE A HELD BUTTON (BUG-7, filed 15
// Sep 2026). strigoi_click pressed and released inside ONE poll, so
// repeatDue(now, now) was false by construction and GameControls'
// OnMouseButtonRepeat -- the click-and-hold path that walks the hero, casts the
// left skill, and carries c-1's squad guard -- was unreachable from any script.
// The guard was verified by READING, and the gap was named in the build note
// rather than papered over. This is the verb that closes it.
//
// The modifiers stay a one-poll tap, as they are for a tap click: a held
// shift-click is a different question (a repeating cast) and it needs its own
// assertion before it gets a verb.
//
// stepFrame runs one frame with the button down. PAUSED, it must be a real
// tick (dt of simulated time), not a wait for the next frame: a paused frame
// has zero deltas, so the controls' clock never reached the 0.25 s repeat
// threshold and every OnMouseButtonRepeat saw repeatDue false -- the hold was
// delivered and did nothing, and a script's "the hold walked him" was the
// PRESS's walk (history item 123, found by instrumenting the handler).
func harnessApplyHeldClick(what string, x, y int, button d2enum.MouseButton,
	mods []d2enum.Key, frames int, stepFrame func() error) (harnessInputOut, error) {
	out := harnessInputOut{Applied: what}

	if harness.input == nil {
		return out, harnessErr("INTERNAL", "the scripted input overlay is not installed", "")
	}

	err := harnessOnUpdate(func() {
		for _, m := range mods {
			harness.input.KeyTap(m)
		}

		harness.input.MoveCursor(x, y)
		harness.input.MouseDown(button)

		out.Tick = atomic.LoadInt64(&harness.tick)
	})
	if err != nil {
		return out, err
	}

	// The press is polled on a frame of its own before any held frame runs,
	// so OnMouseButtonDown always fires at the same point in the hold: a
	// paused step queued behind it could otherwise drain in the same frame
	// and land the press a tick late (the 0.12.3 review).
	if err := harnessWaitFrameAfter(out.Tick); err != nil {
		return out, err
	}

	// Whole frames, each one an input poll the game sees with the button still
	// down. The controls' clock accrues per frame, which is what carries it past
	// the 0.25 s threshold -- so the count is deterministic rather than a sleep.
	for i := 0; i < frames; i++ {
		if err := stepFrame(); err != nil {
			// Best effort: never leave the button stuck down for the next call.
			_ = harnessOnUpdate(func() { harness.input.MouseUp(button) })

			return out, err
		}
	}

	err = harnessOnUpdate(func() {
		harness.input.MouseUp(button)
	})
	if err != nil {
		return out, err
	}

	if err := harnessWaitFrameAfter(atomic.LoadInt64(&harness.tick)); err != nil {
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
		Description: "Scripted mouse click at SCREEN PIXELS x,y (800x600): the cursor moves there and the button is pressed for one poll and released the next, with optional held modifiers (shift-click casts the left skill). A click on open ground walks the player there through the normal controls. Use strigoi_get_player's screen field to aim at the player. hold_frames 2 or more HOLDS the button down for that many frames instead, which is the only way to reach the click-and-hold path (OnMouseButtonRepeat); paused, each held frame is one dt tick, so the simulation moves.",
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

		if in.Hold < 0 || in.Hold > harnessHoldFramesMax {
			return nil, harnessInputOut{}, harnessErr("BAD_ARGUMENT",
				fmt.Sprintf("hold_frames %d is outside 0..%d", in.Hold, harnessHoldFramesMax),
				"1 or 0 is a tap; 2 or more holds the button that many frames")
		}

		var out harnessInputOut

		if in.Hold > 1 {
			// Live, a frame carries wall time; paused, each held frame is one
			// dt tick, as strigoi_step's are, so the hold lasts in simulated
			// time what it says (see harnessApplyHeldClick).
			stepFrame := func() error { return harnessWaitFrameAfter(atomic.LoadInt64(&harness.tick)) }

			if snap := harnessTimeSnapshot(); snap.Mode == "paused" {
				// A paused hold steps the simulation, so it takes the stepping
				// flag as strigoi_step does: nothing else steps while it holds.
				harness.mu.Lock()
				if harness.stepping {
					harness.mu.Unlock()

					return nil, harnessInputOut{}, harnessErr("BAD_ARGUMENT", "a step is already executing", "wait for it to return")
				}

				harness.stepping = true
				harness.mu.Unlock()

				defer func() {
					harness.mu.Lock()
					harness.stepping = false
					harness.mu.Unlock()
				}()

				dt := snap.DT
				stepFrame = func() error {
					var stepErr error
					if err := harnessOnUpdate(func() { stepErr = a.harnessStepTicks(1, dt) }); err != nil {
						return err
					}

					if stepErr != nil {
						return harnessErr("INTERNAL", fmt.Sprintf("advance failed mid-hold: %v", stepErr), "")
					}

					return nil
				}
			}

			out, err = harnessApplyHeldClick(
				fmt.Sprintf("%s click at %d,%d held %d frame(s)", in.Button, in.X, in.Y, in.Hold),
				in.X, in.Y, button, mods, in.Hold, stepFrame)
		} else {
			out, err = harnessApplyInput(fmt.Sprintf("%s click at %d,%d", in.Button, in.X, in.Y), func() {
				for _, m := range mods {
					harness.input.KeyTap(m) // held for the same poll as the click
				}

				harness.input.Click(in.X, in.Y, button)
			})
		}

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
