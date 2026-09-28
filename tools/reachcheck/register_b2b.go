package main

// M4.6 B2b's rows: the entity-keyed snapshots, the next-id seam and the motion
// snapshot (27 Sep 2026). They sit in a file of their own, appended to
// Register at init, so the parallel save bursts (B2a beside this one) each add
// rows without editing the one shared list -- the merge is two new files, not
// two edits to the same closing brace.
//
// All of them are DEFERRED and expected DEAD: B2b builds the verbs and tests
// them in their packages, and nothing in either build calls them yet. The
// snapshots are for B3's Game.SaveWorld and the restores for B4b's hunted-night
// load. The day B3 or B4b calls one, its row flips to live and the gate goes
// red until the row is moved to wire -- which is the point: a save that exists
// only in a unit test is the hollow class this register was built for.
func init() {
	Register = append(Register, registerB2b...)
}

var registerB2b = []Entry{
	{sym(pkgWorld, "Spawns.Snapshot"), BucketDefer, VerdictDead,
		"The spawn tables' state for the world file, members by entity id with the dead marked gone. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgWorld, "Spawns.Restore"), BucketDefer, VerdictDead,
		"Puts the tables back through a Resolver, after the entities are rebuilt with their ids and motion. The hunted-night load calls it.", "M4.6 B4b"},
	{sym(pkgWorld, "Notice.Snapshot"), BucketDefer, VerdictDead,
		"Every watch, watcher by entity id and the player as the word player. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgWorld, "Notice.Restore"), BucketDefer, VerdictDead,
		"Puts every watch back through a Resolver; an unresolved watch is an error, never a drop. The hunted-night load calls it.", "M4.6 B4b"},
	{sym(pkgWorld, "Pursuit.Snapshot"), BucketDefer, VerdictDead,
		"Every chase, hunter by entity id and the player as the word player. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgWorld, "Pursuit.Restore"), BucketDefer, VerdictDead,
		"Puts every chase back through a Resolver, solving nothing: the walk is the entity's motion. The hunted-night load calls it.", "M4.6 B4b"},
	{sym(pkgMapEngine, "MapEngine.SetNextEntityID"), BucketDefer, VerdictDead,
		"The next-id seam through the engine: the next NewNPC or NewCreature takes a saved id; an id already on the map is refused. The load rebuilds every saved entity through it.", "M4.6 B4b"},
	{sym(pkgEntity, "MapEntityFactory.PendingEntityID"), BucketDefer, VerdictDead,
		"Whether a set id is still waiting. A load that has rebuilt everything asserts nothing is.", "M4.6 B4b"},
	{sym(pkgEntity, "Creature.MotionSnapshot"), BucketDefer, VerdictDead,
		"A creature's walk and pose for the world file's entity list. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgEntity, "Creature.RestoreMotion"), BucketDefer, VerdictDead,
		"Puts a rebuilt creature back mid-stride. The load calls it before Spawns.Restore, which checks each member stands where he was saved.", "M4.6 B4b"},
	{sym(pkgEntity, "NPC.MotionSnapshot"), BucketDefer, VerdictDead,
		"An inherited monster's walk and pose for the world file. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgEntity, "NPC.RestoreMotion"), BucketDefer, VerdictDead,
		"Puts a rebuilt inherited monster back mid-stride. The load calls it.", "M4.6 B4b"},
	{sym(pkgScreen, "gameSpawner.Snapshot"), BucketDefer, VerdictDead,
		"The arrival count, which sets where the next pack comes from. Game.SaveWorld calls it.", "M4.6 B3"},
	{sym(pkgScreen, "gameSpawner.Restore"), BucketDefer, VerdictDead,
		"Puts the arrival count back. The load calls it with the spawns.", "M4.6 B4b"},
}
