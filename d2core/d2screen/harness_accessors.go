package d2screen

// Accessors added for the Phase 3 playtest harness (P3 spec §4). They expose
// read-only state and compile in every build configuration.

// CurrentScreen returns the active screen, or nil while a screen is loading
// or a transition is pending.
func (sm *ScreenManager) CurrentScreen() Screen {
	return sm.currentScreen
}

// IsLoading reports whether a screen is currently loading.
func (sm *ScreenManager) IsLoading() bool {
	return sm.loadingScreen != nil
}
