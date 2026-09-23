package d2player

// The "ui" system for the Phase 3 playtest harness (P3 spec §3.5): the game
// controls register as a d2harness.Provider while a game screen is live, so a
// script can assert which panels and menus are open after scripted input
// (the M3.4 inventory script) without a hand-written tool per panel.
// Compiled in every build; d2gamescreen registers and unregisters it.

// HarnessName identifies the provider.
func (g *GameControls) HarnessName() string { return "ui" }

// HarnessState reports which panels, menus, and overlays are open.
func (g *GameControls) HarnessState() map[string]interface{} {
	partyOpen := false
	if g.PartyPanel != nil {
		partyOpen = g.PartyPanel.IsOpen()
	}

	// The overhead bars the HUD last projected (M4.4c-1, clauses 6/8): the same
	// rects the player sees, so a playtest asserts on them without re-deriving
	// the offset (the harness reports the entity's feet, not the bar). Ordered
	// as the HUD drew them (squads by ordinal, enemies by id), so the digest is
	// deterministic.
	bars := make([]map[string]interface{}, 0, len(g.hud.overheadBars))
	cues := make([]map[string]interface{}, 0)
	selectedSquad := ""

	sheetCards := make([]map[string]interface{}, 0)

	if g.squads != nil {
		selectedSquad = g.squads.Selected()

		for _, c := range g.squads.SheetCards() {
			sheetCards = append(sheetCards, map[string]interface{}{
				"squad":      c.Squad,
				"food":       c.Food,
				"water":      c.Water,
				"fatigue":    c.Fatigue,
				"stance":     c.Stance,
				"members":    c.Members,
				"health":     c.Health,
				"max_health": c.MaxHealth,
				"cues":       c.Cues,
				"selected":   c.Selected,
			})
		}
	}

	for _, b := range g.hud.overheadBars {
		bars = append(bars, map[string]interface{}{
			"id":       b.entity,
			"x":        b.x,
			"y":        b.y,
			"w":        b.w,
			"h":        b.h,
			"fill":     b.fill,
			"selected": b.selected,
			"enemy":    b.enemy,
		})

		if len(b.cues) > 0 {
			cues = append(cues, map[string]interface{}{
				"id":   b.entity,
				"cues": b.cues,
			})
		}
	}

	// casting is the swing in flight (M4.4c-2a, section 0 item 5). It gates
	// the click handlers already (three call sites in this file) but nothing
	// reported it, so "how many stepped frames until IsCasting clears" -- the
	// number c-2b's break-away act must wait -- was unmeasurable from a
	// script. It is one bool and it answers that.
	casting := g.hero != nil && g.hero.IsCasting()

	return map[string]interface{}{
		"casting":         casting,
		"inventory_open":  g.inventory.IsOpen(),
		"skilltree_open":  g.skilltree.IsOpen(),
		"hero_stats_open": g.heroStatsPanel.IsOpen(),
		"quest_log_open":  g.questLog.IsOpen(),
		"party_open":      partyOpen,

		// T1: the refusal or hint the combat panel is showing, so a script can
		// assert WHY a tactical click did nothing rather than only that it did.
		"tactical_notice": g.tacticalNoticeText(),

		// T2: the kit panel and the loadout choice.
		"kit_open":         g.hud != nil && g.hud.kit != nil && g.hud.kit.open,
		"choosing_loadout": g.kitHolder != nil && g.kitHolder.ChoosingLoadout(),
		"kit_notice":       g.kitNoticeText(),
		"kit_rows":         g.kitRowsReport(),

		// T3: the talent panel and its cells, clickable where drawn.
		"talent_open":  g.hud != nil && g.hud.talents != nil && g.hud.talents.open,
		"talent_cells": g.talentCellsReport(),
		"talent_note":  g.talentNoteText(),

		// T4: the talk panel, as drawn.
		"talk_open":   g.talking(),
		"talk_view":   g.talkViewReport(),
		"hover_label": g.hoverLabelReport(),

		// Death screen v0.
		"death_open":        g.dead(),
		"death_lines":       g.deathLinesReport(),
		"help_open":         g.HelpOverlay.IsOpen(),
		"escape_menu_open":  g.escapeMenu.IsOpen(),
		"skill_select_open": g.hud.skillSelectMenu.IsOpen(),
		"left_panel_open":   g.isLeftPanelOpen(),
		"right_panel_open":  g.isRightPanelOpen(),
		"free_cam":          g.FreeCam,
		"clock":             g.clock,

		// The M4.4a clock strip -- exactly what the player reads at the top of
		// the screen, so a playtest can assert the eyes work. These are the
		// strings the HUD last computed (refreshed once a world minute), plus
		// the raw time-to-sunset number the "Sunset in X.Xh" readout is
		// formatted from. Empty until the HUD has advanced a frame with a clock.
		"clock_strip_date":          g.hud.stripDate,
		"clock_strip_feast":         g.hud.stripFeast,
		"clock_strip_moon":          g.hud.stripMoon,
		"clock_strip_hours_to_dusk": g.hud.stripHoursToDusk,
		"clock_strip_text":          g.hud.stripText,

		// The overhead bars and stage cues, the selected squad, and the sheet
		// (M4.4c-1, clauses 6/8/9/11).
		"bars":           bars,
		"cues":           cues,
		"selected_squad": selectedSquad,
		"sheet_open":     g.hud.sheetOpen,
		"sheet_cards":    sheetCards,
	}
}

// tacticalNoticeText is the combat panel's live notice, "" when none is shown.
func (g *GameControls) tacticalNoticeText() string {
	if g.hud == nil || g.hud.tactical == nil {
		return ""
	}

	return g.hud.tactical.notice
}

// kitNoticeText is the kit panel's last refusal, "" when none.
func (g *GameControls) kitNoticeText() string {
	if g.hud == nil || g.hud.kit == nil {
		return ""
	}

	return g.hud.kit.notice
}

// kitRowsReport is the panel's clickable rows with their screen y, so a script
// clicks exactly what the player would.
func (g *GameControls) kitRowsReport() []map[string]interface{} {
	out := []map[string]interface{}{}

	if g.hud == nil || g.hud.kit == nil {
		return out
	}

	for _, r := range g.hud.kit.rows {
		out = append(out, map[string]interface{}{
			"text": r.text, "slot": string(r.slot), "pack": r.pack, "recipe": r.recipe,
			"x": kitPanelX + kitPadX + 4, "y": r.y + kitRowHeight/2,
		})
	}

	return out
}

// talentCellsReport is every talent cell with its state and a click point.
func (g *GameControls) talentCellsReport() []map[string]interface{} {
	out := []map[string]interface{}{}

	if g.hud == nil || g.hud.talents == nil {
		return out
	}

	for _, c := range g.hud.talents.cells {
		out = append(out, map[string]interface{}{
			"id": c.id, "taken": c.taken, "open": c.open,
			"x": c.x + 20, "y": c.y + 8,
		})
	}

	return out
}

// talentNoteText is the panel's last notice.
func (g *GameControls) talentNoteText() string {
	if g.hud == nil || g.hud.talents == nil {
		return ""
	}

	return g.hud.talents.notice
}

// deathLinesReport is the death screen's text, as drawn.
func (g *GameControls) deathLinesReport() []string {
	if !g.dead() {
		return []string{}
	}

	return append([]string(nil), g.hud.death.lines...)
}

// talkViewReport is the talk panel's content and where its answers are drawn.
func (g *GameControls) talkViewReport() map[string]interface{} {
	if !g.talking() {
		return map[string]interface{}{}
	}

	t := g.hud.talk

	return map[string]interface{}{
		"role":     t.view.Role,
		"text":     t.view.Text,
		"answers":  append([]string(nil), t.view.Answers...),
		"answer_y": append([]int(nil), t.answerY...),
		"rep":      t.view.Rep,
		"rung":     t.view.Rung,
		"notice":   t.notice,
	}
}

// hoverLabelReport is the label the hover shows under the cursor now.
func (g *GameControls) hoverLabelReport() string {
	if g.hud == nil {
		return ""
	}

	e := g.hud.hoverTarget(g.hud.lastMouseX, g.hud.lastMouseY)
	if e == nil {
		return ""
	}

	if g.talkHolder != nil {
		if role := g.talkHolder.RoleFor(e.Label()); role != "" {
			return role
		}
	}

	return e.Label()
}
