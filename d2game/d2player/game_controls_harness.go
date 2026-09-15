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

	return map[string]interface{}{
		"inventory_open":    g.inventory.IsOpen(),
		"skilltree_open":    g.skilltree.IsOpen(),
		"hero_stats_open":   g.heroStatsPanel.IsOpen(),
		"quest_log_open":    g.questLog.IsOpen(),
		"party_open":        partyOpen,
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
