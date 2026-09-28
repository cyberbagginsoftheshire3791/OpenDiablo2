package main

// M4.6 B2a's rows: the snapshots with no entity ids (clock, light, squads,
// corpses, rising, combat) and C6's per-stream seed (28 Sep 2026). Like B2b's,
// they sit in a file of their own, appended to Register at init, so the two
// parallel save bursts each add rows without editing the one shared list.
//
// Derive is WIRED: CreateGame seeds the spawn tables, combat and the rising
// through it. If it went harness-only, the three streams would be back on one
// seed in the shipped game and every restore that swapped two of them would be
// invisible again -- C6 hollow, with its unit test still green.
//
// The twelve verbs are DEFERRED and expected DEAD: nothing in either build
// calls a Snapshot or a Restore yet. Game.SaveWorld (B3) takes the snapshots;
// the quiet-evening load (B4a) restores these six. The day either calls one,
// its row flips to live and the gate goes red until the row moves to wire.
func init() {
	Register = append(Register, registerB2a...)
}

var registerB2a = []Entry{
	{sym("d2common/d2rand", "Derive"), BucketWire, VerdictLive,
		"C6: CreateGame seeds the spawn tables, combat and the rising each on its own stream of the run's seed (the world keeps the seed itself), so no two gameplay streams hand out one sequence.", ""},

	{sym(pkgWorld, "Clock.Snapshot"), BucketDefer, VerdictDead,
		"The world minutes since the epoch, the clock's whole state. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgWorld, "Clock.Restore"), BucketDefer, VerdictDead,
		"Puts the clock at the saved minute, right after NewClock and before bindProgress reads it (trap 1). The load calls it.", "M4.6 B4a"},
	{sym(pkgWorld, "Light.Snapshot"), BucketDefer, VerdictDead,
		"Every light source and the next id. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgWorld, "Light.Restore"), BucketDefer, VerdictDead,
		"Puts the sources back. The load re-lights his torch through the L path and restores the rest without the carried source, never both (trap 4).", "M4.6 B4a"},
	{sym(pkgWorld, "Squads.Snapshot"), BucketDefer, VerdictDead,
		"Every squad with its members and meters, s:1's member written as the player. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgWorld, "Squads.Restore"), BucketDefer, VerdictDead,
		"Puts the squads back into a fresh owner, s:1 in place so the game's meters pointer and his conditioning survive. The load calls it.", "M4.6 B4a"},
	{sym(pkgWorld, "Corpses.Snapshot"), BucketDefer, VerdictDead,
		"Every body in fall order and the three member maps. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgWorld, "Corpses.Restore"), BucketDefer, VerdictDead,
		"Puts the bodies back into an empty registry WITHOUT the open-count callback (trap 5); the load skips placeTheDead and sets openBodies from the file.", "M4.6 B4a"},
	{sym(pkgWorld, "Rising.Snapshot"), BucketDefer, VerdictDead,
		"Soul pressure, the band and stage last seen, the counters and the stream. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgWorld, "Rising.Restore"), BucketDefer, VerdictDead,
		"Puts the rising back after the corpses, stream through d2rand.Stream.Restore. The load calls it.", "M4.6 B4a"},
	{sym(pkgWorld, "Combat.Snapshot"), BucketDefer, VerdictDead,
		"The model between fights: counters, records and the stream. Refuses during a fight (ErrCombatFighting) and while experience or paced minutes wait to be taken. Game.SaveWorld calls it, and its refusal is the save's FIGHTING.", "M4.6 B3"},
	{sym(pkgWorld, "Combat.Restore"), BucketDefer, VerdictDead,
		"Puts the model back between fights, with the other systems' counters. The load calls it.", "M4.6 B4a"},
}
