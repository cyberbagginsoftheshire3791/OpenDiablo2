package d2util

// FrameDeltas computes the game loop's per-frame time deltas from a clock
// reading. Extracted from d2app.advance (P3 spec E1) so the live path's
// arithmetic is pinned by a test while the playtest harness substitutes fixed
// deltas when stepping. All values are in seconds.
//
// It returns: the unscaled delta since lastTime (consumed by the terminal),
// the timeScale-scaled delta (the simulation tick), and the scaled delta
// since lastScreenAdvance (consumed by the screen manager).
func FrameDeltas(now, lastTime, lastScreenAdvance, timeScale float64) (elapsedUnscaled, elapsed, elapsedScreen float64) {
	elapsedUnscaled = now - lastTime
	elapsed = elapsedUnscaled * timeScale
	elapsedScreen = (now - lastScreenAdvance) * timeScale

	return elapsedUnscaled, elapsed, elapsedScreen
}
