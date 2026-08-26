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
	}
}
