package d2player

// FeedbackState is what F8's note (and a crash report) says of the HUD: the
// view's scale and what is open over the world, in the order Escape would
// close them. The names are the ui provider's (game_controls_harness.go), so a
// note and a playtest read the same words. 5 Oct 2026.
func (g *GameControls) FeedbackState() map[string]interface{} {
	open := []string{}

	add := func(name string, isOpen bool) {
		if isOpen {
			open = append(open, name)
		}
	}

	add("death", g.dead())
	add("journal", g.journalOpen())
	add("talk", g.talking())

	if g.hud != nil {
		add("kit", g.hud.kit != nil && g.hud.kit.open)
		add("talents", g.hud.talents != nil && g.hud.talents.open)
		add("sheet", g.hud.sheetOpen)
		add("skill_select", g.hud.skillSelectMenu != nil && g.hud.skillSelectMenu.IsOpen())
	}

	add("escape_menu", g.escapeMenu != nil && g.escapeMenu.IsOpen())
	add("help", g.HelpOverlay != nil && g.HelpOverlay.IsOpen())

	if g.inventory != nil && g.skilltree != nil {
		add("right_panel", g.isRightPanelOpen())
	}

	if g.heroStatsPanel != nil && g.questLog != nil && g.inventory != nil {
		add("left_panel", g.isLeftPanelOpen())
	}

	state := map[string]interface{}{
		"open":     open,
		"free_cam": g.FreeCam,
	}

	if g.mapRenderer != nil {
		state["view_scale"] = g.viewScale()
	}

	return state
}
