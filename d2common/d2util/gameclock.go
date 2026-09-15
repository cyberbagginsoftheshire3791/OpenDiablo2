package d2util

// FrameDeltas computes the game loop's per-frame time deltas from a clock
// reading. Extracted from d2app.advance (P3 spec E1) so the live path's
// arithmetic is pinned by a test while the playtest harness substitutes fixed
// deltas when stepping. All values are in seconds.
//
// It returns: the unscaled delta since lastTime (consumed by the terminal),
// the timeScale-scaled delta (the simulation tick), and the scaled delta
// since lastScreenAdvance (consumed by the screen manager).
//
// The raw delta is CLAMPED at maxDelta before scaling. ebiten delivers exactly
// one Update per frame with no catch-up (SyncWithFPS: TicksPerSecond -1), so an
// unbounded delta -- a closed laptop lid, a debugger break, a long synchronous
// load in a frame -- would arrive as one giant tick and drive the whole world
// forward at once: a 10-minute sleep is 2,400 world minutes and about -160 HP of
// neglect in a single frame (audit B1, 12 Sep 2026). The clamp bounds both the
// simulation tick and the screen tick; the playtest harness substitutes its own
// fixed deltas (harness_time.go) and never calls this, so it is unaffected.
func FrameDeltas(now, lastTime, lastScreenAdvance, timeScale float64) (elapsedUnscaled, elapsed, elapsedScreen float64) {
	const maxDelta = 0.25 // seconds

	elapsedUnscaled = now - lastTime
	if elapsedUnscaled > maxDelta {
		elapsedUnscaled = maxDelta
	}

	elapsed = elapsedUnscaled * timeScale

	screen := now - lastScreenAdvance
	if screen > maxDelta {
		screen = maxDelta
	}

	elapsedScreen = screen * timeScale

	return elapsedUnscaled, elapsed, elapsedScreen
}
