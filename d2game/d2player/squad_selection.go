package d2player

import "math"

// M4.4c-1 selection: the player selects a squad by clicking one of its model
// sprites, or cycles with the cycle key; the selected squad's bar gets an
// outline and its sheet opens (ruled ask 6). Selection CANNOT reuse the hover
// loop's Selectable() guard -- Player.Selectable() is IsInTown(), so the
// janissary is unhoverable outside the town stamp (where night one happens),
// and a harness-placed NPC has name=="" and is never Selectable -- so it uses
// its own hit test over the squad's member entities (brief §4.5, §2).

// squadAtScreen returns the id of the squad whose model's sprite rect contains
// the screen point, or "" if none. It is the hover-loop hit test
// (renderForSelectableEntitiesHovered) with the Selectable() guard replaced by
// squad membership.
func (g *GameControls) squadAtScreen(mx, my int) string {
	if g.squads == nil || g.hud == nil || g.hud.mapEngine == nil || g.mapRenderer == nil {
		return ""
	}

	entities := g.hud.mapEngine.Entities()

	for _, sm := range g.squads.ModelEntities() {
		ent, ok := entities[sm.Entity]
		if !ok {
			continue
		}

		sxf, syf := g.mapRenderer.WorldToScreenF(ent.GetPositionF())
		ex, ey := int(math.Floor(sxf)), int(math.Floor(syf))

		w, h := ent.GetSize()
		halfW, halfH := w>>1, h>>1

		l, r := ex-halfW-hoverLabelOuterPad, ex+halfW+hoverLabelOuterPad
		t, b := ey-halfH-hoverLabelOuterPad, ey+halfH-hoverLabelOuterPad

		if l <= mx && r >= mx && t <= my && b >= my {
			return sm.Squad
		}
	}

	return ""
}

// selectSquad selects a squad and opens its sheet (ruled ask 6: the sheet opens
// on selection).
func (g *GameControls) selectSquad(id string) {
	if g.squads == nil {
		return
	}

	if g.squads.SetSelected(id) {
		g.hud.setSheetOpen(true)
	}
}

// cycleSquad selects the next squad (the cycle key) and opens its sheet. With
// one squad it re-selects s:1, harmless, and keeps the key live for the N-path.
func (g *GameControls) cycleSquad() {
	if g.squads == nil {
		return
	}

	g.squads.Cycle()
	g.hud.setSheetOpen(true)
}

// closeSquadSheet closes the selected-squad sheet (ruled ask 6: it closes on a
// key or on deselect -- a left click on empty ground).
func (g *GameControls) closeSquadSheet() {
	if g.hud != nil {
		g.hud.setSheetOpen(false)
	}
}
