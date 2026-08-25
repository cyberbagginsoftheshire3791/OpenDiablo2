//go:build !harness

package d2app

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
)

// The Phase 3 playtest harness (P3 spec) compiles only with `-tags harness`.
// These no-ops keep the release build free of the MCP server, its flags, and
// its dependency (spec §3.2). The call sites in app.go are the only trace.

func (a *App) harnessEarlyInit() {}

func (a *App) harnessRegisterFlags() {}

func (a *App) harnessStart() {}

func (a *App) harnessDrainUpdate() {}

func (a *App) harnessDrainDraw(_ d2interface.Surface) {}

func (a *App) harnessNoteScreen(_ string) {}

func (a *App) harnessNoteGame(_ *d2client.GameClient, _ *d2gamescreen.Game) {}
