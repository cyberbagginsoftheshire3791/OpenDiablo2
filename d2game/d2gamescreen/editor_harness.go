package d2gamescreen

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mapedit"
)

// --- the World Editor's "editor" harness provider ---------------------------

// editorProvider is the harness's "editor" system while the World Editor is the
// screen (28 Sep review, B1). The acceptance script drives the REAL screen --
// it picks a palette row and clicks the map with the mouse, presses Ctrl+S and
// P on the keyboard -- and it needs three things from here to do that honestly:
// where things are on the screen (the palette's rows, the map's transform, the
// people), what the editor thinks of the tile under the cursor (the ghost's own
// verdict), and the document's state (dirty, undo depth, the engine's verdict).
// It reports; the only thing it writes is the zoom.
//
// THE ZOOM FIELD. The harness has no wheel verb (37 tools), so a script cannot
// turn one. Setting "zoom" calls Editor.zoomAbout -- the function the wheel
// handler calls -- about the middle of the map's view, so a scripted zoom goes
// down the same line a turned wheel does. It is the project's pattern: a
// settable provider field, not a new tool.
//
// OnLoad registers it last; OnUnload removes it, so a playtest's game never
// sees a stale editor.
type editorProvider struct{ e *Editor }

func (p editorProvider) HarnessName() string { return "editor" }

// HarnessSettableFields lists the one field a script may write.
func (p editorProvider) HarnessSettableFields() []string { return []string{"zoom"} }

// HarnessSet writes the zoom, about the middle of the map's view.
func (p editorProvider) HarnessSet(field string, value interface{}) error {
	if field != "zoom" {
		return fmt.Errorf("only zoom is settable")
	}

	var z float64

	switch v := value.(type) {
	case float64:
		z = v
	case int:
		z = float64(v)
	default:
		return fmt.Errorf("zoom wants a number, got %T", value)
	}

	if math.IsNaN(z) || z < edMinScale || z > edMaxScale {
		return fmt.Errorf("zoom %v is outside the editor's range %v..%v", z, edMinScale, edMaxScale)
	}

	if p.e.mapRenderer == nil {
		return fmt.Errorf("the editor has not loaded")
	}

	view := mapViewRect()
	p.e.zoomAbout(view.Min.X+view.Dx()/2, view.Min.Y+view.Dy()/2, z)

	return nil
}

func (p editorProvider) HarnessState() map[string]interface{} {
	e := p.e
	view := mapViewRect()

	tool := "select"
	if e.tool == toolPlace {
		tool = "place"
	}

	picked := ""
	if e.picked != nil {
		picked = e.picked.ID
	}

	firstProblem := ""
	if len(e.problems) > 0 {
		firstProblem = e.problems[0].Error()
	}

	engineError := ""
	if e.engineErr != nil {
		engineError = e.engineErr.Error()
	}

	tab := ""
	if e.tab >= 0 && e.tab < len(e.tabs) {
		tab = e.tabs[e.tab].Title
	}

	state := map[string]interface{}{
		"screen":        "world_editor",
		"map_path":      e.mapPath,
		"disk_path":     e.diskPath,
		"asset_root":    e.assetRoot,
		"dirty":         e.dirty(),
		"undo_depth":    e.stack.Depth(),
		"undo_label":    e.stack.UndoLabel(),
		"can_redo":      e.stack.CanRedo(),
		"message":       e.message,
		"problems":      len(e.problems),
		"first_problem": firstProblem,
		"engine_error":  engineError,
		"sealed":        e.sealed,
		"tool":          tool,
		"picked":        picked,
		"selected":      e.selected,
		"tab":           tab,
		"map_size":      []int{e.doc.Size().X, e.doc.Size().Y},
		"view":          []int{view.Min.X, view.Min.Y, view.Max.X, view.Max.Y},
		"grid":          e.showGrid,
		"label_zoom":    edLabelZoom,
		"tabs":          p.tabs(),
		"rows":          p.rows(),
		"structures":    p.structures(),
		"people":        p.people(),
	}

	if e.mapRenderer == nil {
		return state
	}

	state["zoom"] = e.mapRenderer.Scale()
	state["fit_zoom"] = e.fitScale(view)

	// The map's transform, as three vectors: world (x, y) is on screen at
	// origin + x*x_axis + y*y_axis. The viewport is affine, so a script can
	// aim at the middle of any tile from this without asking again.
	ox, oy := e.mapRenderer.WorldToScreenF(0, 0)
	ax, ay := e.mapRenderer.WorldToScreenF(1, 0)
	bx, by := e.mapRenderer.WorldToScreenF(0, 1)

	state["world_to_screen"] = map[string]interface{}{
		"origin": []float64{ox, oy},
		"x_axis": []float64{ax - ox, ay - oy},
		"y_axis": []float64{bx - ox, by - oy},
	}

	w, h := float64(e.doc.Size().X), float64(e.doc.Size().Y)
	corner := func(x, y float64) []float64 {
		sx, sy := e.mapRenderer.WorldToScreenF(x, y)
		return []float64{sx, sy}
	}

	// The map's own diamond on screen: top, right, bottom and left corners.
	state["map_corners"] = [][]float64{corner(0, 0), corner(w, 0), corner(w, h), corner(0, h)}

	if x, y, ok := e.hoverTile(); ok {
		hover := map[string]interface{}{"tile": []int{x, y}}

		if e.picked != nil {
			okHere, why := e.canPlace(*e.picked, x, y)
			hover["can_place"], hover["why"] = okHere, why
		}

		state["hover"] = hover
	}

	return state
}

// tabs is the palette's tab strip with each tab's rectangle on screen.
func (p editorProvider) tabs() []interface{} {
	e := p.e
	out := make([]interface{}, 0, len(e.tabs))

	for i, tab := range e.tabs {
		out = append(out, map[string]interface{}{
			"title": tab.Title, "category": string(tab.Category), "available": tab.Available, "count": tab.Count,
			"x": editorScreenW - edPaletteW, "y": e.paletteTabsTop() + i*edTabH, "w": edPaletteW, "h": edTabH,
		})
	}

	return out
}

// rows is the palette rows on show, each with its rectangle on screen -- the
// same geometry editorPaletteHit reads, so a click in the middle of one picks it.
func (p editorProvider) rows() []interface{} {
	e := p.e
	top := e.paletteRowsTop()
	visible := e.paletteVisibleRows()
	out := []interface{}{}

	for i := 0; i < visible; i++ {
		idx := e.rowTop + i
		if idx >= len(e.rows) {
			break
		}

		ent := e.rows[idx]
		usable, why := e.canPickable(ent)

		out = append(out, map[string]interface{}{
			"id": ent.ID, "name": ent.DisplayName, "category": string(ent.Category), "layer": fmt.Sprint(ent.Layer),
			"footprint": []int{ent.Footprint.X, ent.Footprint.Y}, "pickable": usable, "why": why,
			"x": editorScreenW - edPaletteW, "y": top + i*edRowH, "w": edPaletteW, "h": edRowH,
		})
	}

	return out
}

// structures is every structure in the document with its footprint, Max
// exclusive, as the loader lays it.
func (p editorProvider) structures() []interface{} {
	out := []interface{}{}

	for _, o := range p.e.doc.Structures() {
		fp := o.Footprint
		out = append(out, map[string]interface{}{
			"id": o.ID, "name": o.Name, "gid": o.GID,
			"footprint": []int{fp.Min.X, fp.Min.Y, fp.Max.X, fp.Max.Y},
		})
	}

	return out
}

// people is every person and the player_start, with where each is drawn -- the
// markers B5 added -- and, from the last frame drawn, the box his mark covers
// and where his name went ("label", absent when it was not drawn, and
// "label_text", what was drawn there: the second 28 Sep review's C).
func (p editorProvider) people() []interface{} {
	e := p.e
	out := []interface{}{}

	for _, o := range e.doc.Objects() {
		if o.Class != d2mapedit.ClassNPC && o.Class != d2mapedit.ClassPlayerStart {
			continue
		}

		person := map[string]interface{}{
			"id": o.ID, "name": editorObjectName(o), "class": o.Class,
			"tile": []int{o.Tile().X, o.Tile().Y},
		}

		if e.mapRenderer != nil {
			x, y := e.mapRenderer.WorldToScreen(o.X, o.Y)
			person["screen"] = []int{x, y}
		}

		if d, ok := e.peopleDrawn[o.ID]; ok {
			person["mark"] = []int{d.mark.Min.X, d.mark.Min.Y, d.mark.Max.X, d.mark.Max.Y}

			if !d.label.Empty() {
				person["label"] = []int{d.label.Min.X, d.label.Min.Y, d.label.Max.X, d.label.Max.Y}
				person["label_text"] = d.text
			}
		}

		out = append(out, person)
	}

	return out
}

// editorLayerName keeps d2maptiled's layer type spelled once in this file.
var _ = d2maptiled.LayerStructure
