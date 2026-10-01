package d2gamescreen

import (
	"fmt"
	"image"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// FOG OF WAR, F1: black until explored, by day (1 Oct 2026; Josh: "Black
// until explored with line of sight that can be upgraded and extended";
// claude/fog-of-war-build-plan.md §4 F1, with Josh's Q1: day sight 12).
//
// OPT-IN. -fog turns it on for every game this process starts; it is off by
// default, off under -classic (Diablo II's game), off in a network game (more
// than one player: shared sight has no rule yet), and the World Editor never
// has it (its renderer gets no sampler). With fog off the renderer holds no
// FogSampler, and a nil sampler is the unfogged draw, call for call
// (d2maprenderer/fog.go).
//
// DISPLAY ONLY: nothing here feeds Notice, Combat, Seek or Pursuit.
//
// NOT SAVED (F3 saves it): a load is a new game, and starts black.

// processGameFog is -fog: whether the games this process starts have fog.
//
// nolint:gochecknoglobals // one value per process, set from the command line
var processGameFog bool

// SetGameFog sets whether new games have fog. d2app calls it with -fog.
func SetGameFog(on bool) { processGameFog = on }

// fogEyeID is the one eye of F1: squad s:1, the player.
const fogEyeID = "s:1"

// Why fog is off, as the provider reports it.
const (
	fogOffFlag    = "off"     // -fog not given (or the harness turned it off)
	fogOffClassic = "classic" // -classic: Diablo II's game has no fog
	fogOffNetwork = "network" // more than one player
	fogOffNoMap   = "no_map"  // the map or the player is not there yet
)

// gameFog is one game's fog: the grids, and whether they are wanted and drawn.
type gameFog struct {
	fog      *d2world.Fog
	wanted   bool // -fog at creation, then the harness's "enabled"
	attached bool // the map renderer holds the fog as its sampler
	probe    *[2]int
}

// newGameFog is a game's fog, reading the map's line of sight.
func newGameFog(sight d2world.TileSight, wanted bool) *gameFog {
	return &gameFog{fog: d2world.NewFog(d2world.DefaultFogDials(), sight), wanted: wanted}
}

// mapTileSight is fog's line of sight: the game's map engine's
// TileSightClear, asked of whichever engine the client holds now.
type mapTileSight struct{ v *Game }

func (s mapTileSight) TileSightClear(fx, fy float64, tx, ty int) (clear bool, cells int) {
	if s.v.gameClient == nil || s.v.gameClient.MapEngine == nil {
		return false, 0
	}

	return s.v.gameClient.MapEngine.TileSightClear(fx, fy, tx, ty)
}

// Structures are the map's structure footprints, which fog shows whole.
func (s mapTileSight) Structures() []image.Rectangle {
	if s.v.gameClient == nil || s.v.gameClient.MapEngine == nil {
		return nil
	}

	return s.v.gameClient.MapEngine.Structures()
}

// fogOffReason is why this game draws no fog now, "" when it does.
func (v *Game) fogOffReason() string {
	switch {
	case v.fog == nil:
		return fogOffFlag
	case v.asset != nil && v.asset.Classic():
		return fogOffClassic
	case !v.fog.wanted:
		return fogOffFlag
	case v.gameClient == nil || v.gameClient.MapEngine == nil || v.gameClient.MapEngine.IsLoading:
		return fogOffNoMap
	case len(v.gameClient.Players) > 1:
		return fogOffNetwork
	case v.localPlayer == nil || len(v.gameClient.Players) == 0:
		return fogOffNoMap // not joined yet
	}

	return ""
}

// fogAdvance is fog's update, every frame AFTER MapEngine.Advance (plan §2.4):
// a held turn stops the world clock but not his Move, and the ground he walks
// onto must open as he walks. The Update itself does nothing unless his tile
// changed (d2world.Fog.Update), so a frame where he stands still costs a
// comparison.
//
// THE RENDERER HOLDS THE FOG FROM THE GAME'S FIRST FRAME (the review's B5, 1
// Oct 2026): CreateGame calls this too, before there is a map or a player
// ("no_map"), and a fog that has seen nothing answers every tile unexplored,
// so the first frames of a -fog game are black rather than the whole village
// drawn unfogged for a frame. "no_map" never takes the fog away; -classic, a
// network game and fog turned off do, the frame they are seen.
func (v *Game) fogAdvance() {
	if v.fog == nil {
		return
	}

	switch v.fogOffReason() {
	case "":
	case fogOffNoMap:
		v.fogAttach()

		return
	default:
		v.fogDetach()

		return
	}

	size := v.gameClient.MapEngine.Size()
	at := v.localPlayer.Position.World()

	v.fog.fog.Update(size.Width, size.Height, []d2world.Eye{{ID: fogEyeID, X: at.X(), Y: at.Y()}})
	v.fogAttach()
}

// fogAttach gives the renderer the fog, once.
func (v *Game) fogAttach() {
	if !v.fog.attached && v.mapRenderer != nil {
		v.mapRenderer.SetFogSampler(v.fog.fog)
		v.fog.attached = true
	}
}

// fogDetach takes the fog from the renderer: the next frame is the unfogged
// draw.
func (v *Game) fogDetach() {
	if v.fog != nil && v.fog.attached && v.mapRenderer != nil {
		v.mapRenderer.SetFogSampler(nil)
	}

	if v.fog != nil {
		v.fog.attached = false
	}
}

// fogProvider is the harness's "fog" system (docs/harness.md).
type fogProvider struct{ v *Game }

func (p fogProvider) HarnessName() string { return "fog" }

// HarnessState is everything a script asserts fog on: whether it is on and
// why not, the grid's counts (and, for a map of at most 64 x 64, its rows:
// '0' unexplored, '1' explored, '2' visible), the eyes, the dials, the cost
// counters, and the probe's answer at the tile a script named.
func (p fogProvider) HarnessState() map[string]interface{} {
	v := p.v
	if v.fog == nil {
		return map[string]interface{}{"enabled": false, "off_reason": fogOffFlag}
	}

	f := v.fog.fog
	w, h := f.Size()
	explored, visible := f.Counts()
	recomputes, skipped, cells := f.Counters()
	dials := f.Dials()
	reason := v.fogOffReason()

	eyes := make([]interface{}, 0, 1)
	for _, e := range f.Eyes() {
		eyes = append(eyes, map[string]interface{}{
			"id": e.ID, "x": e.X, "y": e.Y, "sight": dials.DaySight,
			"terms": map[string]interface{}{"base": dials.DaySight},
		})
	}

	st := map[string]interface{}{
		"enabled":           reason == "",
		"wanted":            v.fog.wanted,
		"off_reason":        reason,
		"drawn":             v.mapRenderer != nil && v.mapRenderer.HasFog(),
		"w":                 w,
		"h":                 h,
		"explored":          explored,
		"visible":           visible,
		"recomputes":        recomputes,
		"skipped":           skipped,
		"cells_read":        cells,
		"day_sight":         dials.DaySight,
		"memory_level":      dials.MemoryLevel,
		"memory_saturation": dials.MemorySaturation,
		"eyes":              eyes,
		"saved":             false, // F3
	}

	if w > 0 && h > 0 && w <= 64 && h <= 64 {
		st["rows"] = f.Rows()
	}

	if pr := v.fog.probe; pr != nil {
		probe := map[string]interface{}{
			"x": pr[0], "y": pr[1], "state": f.At(pr[0], pr[1]).String(),
			"clear_from": f.ClearFrom(pr[0], pr[1]),
		}

		if r, ok := f.StructureAt(pr[0], pr[1]); ok {
			probe["structure"] = []int{r.Min.X, r.Min.Y, r.Max.X, r.Max.Y} // the footprint, Max exclusive
		}

		if v.mapRenderer != nil {
			sx, sy := v.mapRenderer.WorldToScreen(float64(pr[0])+0.5, float64(pr[1])+0.5)
			probe["screen"] = []int{sx, sy} // the tile's centre on screen now
		}

		st["probe"] = probe
	}

	return st
}

// HarnessDigest: fog is not saved in F1 (F3 saves it), so a game resumed from
// a world save starts black and does not reproduce it -- the whole of it is
// this process's. Two things are in neither part: `skipped`, which counts
// frames (boot frames differ from launch to launch, so two launches of one
// script would disagree), and the probe's `screen` point, presentation that
// moves while the camera eases onto him (BUG-58's rule).
func (p fogProvider) HarnessDigest() (world, process map[string]interface{}) {
	process = p.HarnessState()
	delete(process, "skipped")

	if probe, ok := process["probe"].(map[string]interface{}); ok {
		kept := make(map[string]interface{}, len(probe))

		for k, v := range probe {
			if k != "screen" {
				kept[k] = v
			}
		}

		process["probe"] = kept
	}

	return map[string]interface{}{}, process
}

// HarnessSettableFields are fog's writes: whether it is on (the game's view,
// like ui.zoom), its three dials, the three state verbs and the probe.
func (p fogProvider) HarnessSettableFields() []string {
	return []string{
		"enabled", "day_sight", "memory_level", "memory_saturation",
		"explore", "forget", "reveal_all", "probe",
	}
}

// HarnessSet writes one of fog's fields, then brings the fog up to date at
// once, so a paused game's next frame draws what was written.
func (p fogProvider) HarnessSet(field string, value interface{}) error {
	v := p.v
	if v.fog == nil {
		return fmt.Errorf("this game has no fog")
	}

	f := v.fog.fog

	switch field {
	case "enabled":
		on, ok := value.(bool)
		if !ok {
			return fmt.Errorf("enabled wants true or false, got %T", value)
		}

		if on && v.asset != nil && v.asset.Classic() {
			return fmt.Errorf("fog is off under -classic: Diablo II's game has no fog")
		}

		v.fog.wanted = on
	case "day_sight", "memory_level", "memory_saturation":
		n, ok := value.(float64)
		if !ok || math.IsNaN(n) {
			return fmt.Errorf("%s wants a number, got %v", field, value)
		}

		d := f.Dials()

		switch field {
		case "day_sight":
			if n <= 0 || n > 64 {
				return fmt.Errorf("day_sight %v is outside 0..64 tiles", n)
			}

			d.DaySight = n
		case "memory_level":
			if n < 0 || n > 1 {
				return fmt.Errorf("memory_level %v is outside 0..1", n)
			}

			d.MemoryLevel = n
		default:
			if n < 0 || n > 1 {
				return fmt.Errorf("memory_saturation %v is outside 0..1", n)
			}

			d.MemorySaturation = n
		}

		f.SetDials(d)
	case "explore":
		m, ok := value.(map[string]interface{})
		x, okx := m["x"].(float64)
		y, oky := m["y"].(float64)
		r, okr := m["r"].(float64)

		if !ok || !okx || !oky || !okr || r < 0 {
			return fmt.Errorf("explore wants {x, y, r} in world tiles, got %v", value)
		}

		f.Explore(x, y, r)
	case "forget":
		f.Forget()
	case "reveal_all":
		f.RevealAll()
	case "probe":
		m, ok := value.(map[string]interface{})
		x, okx := m["x"].(float64)
		y, oky := m["y"].(float64)

		if !ok || !okx || !oky {
			return fmt.Errorf("probe wants {x, y}, a tile, got %v", value)
		}

		v.fog.probe = &[2]int{int(math.Floor(x)), int(math.Floor(y))}

		return nil // a question: nothing to bring up to date
	default:
		return fmt.Errorf("fog has no settable field %q", field)
	}

	v.fogAdvance()

	return nil
}
