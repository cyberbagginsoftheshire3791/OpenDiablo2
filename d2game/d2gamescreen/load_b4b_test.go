package d2gamescreen

import (
	"errors"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2saveref"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2bestiary"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// M4.6 B4b: A HUNTED NIGHT RESUMES, IN A UNIT GAME. b4Game is B4a's quiet
// evening; b4bGame adds the bestiary and a villager -- a creature on the map
// before the screen captures its natives, as authored.go builds the village
// before CreateGame -- and b4bHunt fills the night with what B4a refused:
// a pack (a group on the tables), a wounded survivor and a slain beast lying
// as a corpse (bodies), a walk in progress, a watch and a chase, and a watch
// on the VILLAGER (so a record names him by the id the re-key must give
// back). THE CHASE IS OF THE VILLAGER since the combat status (Josh, 30 Sep
// 2026): a hostile chasing HIM is combat, and a save is refused in combat, so
// no file the game writes holds a chase of him any more -- B4b's was one. The
// chase of the villager names him by the id the re-key gives back, as the
// watch does; a file from before this burst holding a chase of him still
// resumes, and he is in combat after it (combat_status_test.go). Every creature is Strigoi's own PNG art, so no MPQs; an NPC cannot be
// built without them, and the NPC half -- a deployed squad model -- is the
// playtest's (TestSaveResume).

func b4bBestiary(t *testing.T) *d2bestiary.Catalog {
	t.Helper()

	data, err := b3Asset(t).LoadFile(bestiaryCatalogPath)
	require.NoError(t, err)

	b, err := d2bestiary.Load(data)
	require.NoError(t, err)

	return b
}

// b4bVillagerAt is where the unit map's villager stands, sub-tiles.
const b4bVillagerAt = 60

// b4bGame is b4Game with a bestiary and a villager the "map" built (its id a
// fresh uuid in every game, as the shipped game's crypto/rand ids are).
func b4bGame(t *testing.T) (*Game, string) {
	t.Helper()

	v, save := b4Game(t)
	v.bestiary = b4bBestiary(t)

	// His id is his connection's, new every launch: the file names him by
	// the word "player".
	b3SetField(t, v.localPlayer, "uuid", "0player-"+filepath.Base(t.TempDir()))

	b4bCreature(t, v, "feral-dog", b4bVillagerAt, b4bVillagerAt)
	v.natives = nativesOf(v.gameClient.MapEngine)
	require.Len(t, v.natives, 1)

	// A dial, set on every game alike (dials are never saved): nothing
	// notices him from further than half a tile, so the run on walks and
	// solves and no fight starts -- a save is refused mid-fight.
	require.NoError(t, v.spawns.HarnessSet("notice_radius", 0.5))

	return v, save
}

// b4bCreature builds a bestiary creature as the spawner does and puts it on
// the map.
func b4bCreature(t *testing.T, v *Game, id string, x, y int) *d2mapentity.Creature {
	t.Helper()

	entry, ok := v.bestiary.ByID(id)
	require.True(t, ok, id)

	c, err := v.gameClient.MapEngine.NewCreature(x, y, entry.Name, creatureAnimationPaths(entry), 0, nil)
	require.NoError(t, err)

	c.SetCreatureID(entry.ID)
	c.SetSpeed(entry.SpeedOr(5))
	v.gameClient.MapEngine.AddEntity(c)

	return c
}

// b4bVillager is the one native on v's map.
func b4bVillager(t *testing.T, v *Game) d2interface.MapEntity {
	t.Helper()

	for _, e := range v.natives {
		return e
	}

	t.Fatal("no villager")

	return nil
}

// b4bNight is what b4bHunt placed, by role.
type b4bNight struct {
	wounded, walker, slain *d2mapentity.Creature
	villager               d2interface.MapEntity
}

// b4bHunt fills v's night (b4Busy's evening first) with everything B4b
// rebuilds.
func b4bHunt(t *testing.T, v *Game) b4bNight {
	t.Helper()

	b4Busy(t, v)

	n := b4bNight{
		wounded:  b4bCreature(t, v, "wolf", 100, 100),
		walker:   b4bCreature(t, v, "wolf", 110, 90),
		slain:    b4bCreature(t, v, "feral-dog", 95, 105),
		villager: b4bVillager(t, v),
	}

	// The slain lies as a corpse, where he fell.
	require.NoError(t, n.slain.RestoreMotion(d2mapentity.Motion{
		Pos: [2]float64{95, 105}, Target: [2]float64{95, 105}, Path: [][2]float64{}, Mode: "dead", Corpse: true,
	}))

	// Their bodies: the survivor wounded, the slain at 0, the walker whole.
	v.adoptNPCBody(n.wounded.ID(), 96)
	v.adoptNPCBody(n.walker.ID(), 96)
	v.adoptNPCBody(n.slain.ID(), 72)
	v.bodies[n.wounded.ID()].health = 41
	v.bodies[n.slain.ID()].health = 0

	// A walk in progress: two waypoints ahead. And the villager walking too
	// (B4a refused a villager saved mid-walk; B4b restores his motion).
	n.walker.SetPath([]d2vector.Position{d2vector.NewPosition(120, 90), d2vector.NewPosition(130, 95)}, nil)
	n.villager.(*d2mapentity.Creature).SetPath([]d2vector.Position{d2vector.NewPosition(70, 60)}, nil)

	// A watch on him; a chase of the villager (the combat status: a chase of
	// HIM is combat, which refuses the save); a watch on the villager.
	require.True(t, v.Watch(n.walker, v.localPlayer))
	require.True(t, v.Pursue(n.walker, n.villager))
	require.True(t, v.Watch(n.wounded, n.villager))

	// The pack: the three as one group on the tables.
	r := worldResolver{v}
	snap, err := v.spawns.Snapshot(r)
	require.NoError(t, err)

	member := func(c *d2mapentity.Creature) d2world.SpawnMemberSnapshot {
		x, y := c.GetPositionF()

		return d2world.SpawnMemberSnapshot{ID: c.ID(), X: x, Y: y}
	}

	snap.NextID = 2
	snap.Groups = []d2world.SpawnGroupSnapshot{{
		ID: "g:1", Row: "wolves", Code: "zombie1", Morale: 66.7, BornAt: v.worldClock.WorldMinutes() - 10,
		Band: 0, Stage: "night", Weight: 1, Spawned: 3, BornWhere: [][2]float64{{20, 20}},
		Members: []d2world.SpawnMemberSnapshot{member(n.wounded), member(n.walker), member(n.slain)},
	}}

	// Restored into the tables themselves (they hold no group yet): the one
	// way to put a group on them without a table arrival, which needs the
	// MPQs' monstats.
	require.NoError(t, v.spawns.Restore(snap, r, v.gameClient.Seed))

	return n
}

// b4bIDs is every NPC and creature on v's map, by id, sorted.
func b4bIDs(v *Game) []string {
	out := []string{}

	for id, e := range v.gameClient.MapEngine.Entities() {
		if kindOf(e) != "" {
			out = append(out, id)
		}
	}

	sort.Strings(out)

	return out
}

// THE ACCEPTANCE TEST IN MINIATURE, ON A HUNTED NIGHT: a pack, a wounded
// survivor, a corpse, a walk, a watch on him and a chase of the villager (the
// combat status: never a chase of him), a watch on the villager -- saved, loaded into a fresh game whose villager has another id,
// and the loaded game saves the saved moment and, run on, stays the saved
// game. Every entity the file names is on the map with its saved id and
// motion; the villager is the SAME entity, re-keyed in place; the bodies are
// the file's; nothing is left waiting in the seam.
func TestAHuntedNightResumes(t *testing.T) {
	saved, save := b4bGame(t)
	night := b4bHunt(t, saved)

	w, atT := b4File(t, saved, save, "t.world.json")
	require.Len(t, w.Spawns.Groups, 1, "the pack is in the file")
	require.Len(t, w.Notice.Watches, 2, "the watches are in the file")
	require.Len(t, w.Pursuit.Chases, 1, "the chase is in the file")
	require.Len(t, w.Bodies, 3, "the bodies are in the file")
	require.Len(t, w.Entities, 4, "the villager and the pack are the file's entities")

	resumed, _ := b4bGame(t)
	villager := b4bVillager(t, resumed)
	require.NotEqual(t, night.villager.ID(), villager.ID(), "the fresh map's villager has another id (as every shipped launch does)")

	require.NoError(t, b4Load(t, resumed, w))

	require.Equal(t, []string{
		"clock", "natives", "entities", "validated", "health", "squads", "light", "torch", "corpses", "rising",
		"spawns", "spawner", "notice", "pursuit", "combat", "bodies", "scene", "hero", "world_rng",
	}, resumed.loadSteps, "the load order (docs/m4.6-world-save-notes.md)")

	// The villager: re-keyed IN PLACE -- the same entity, the saved id, the
	// map keyed by it, the old id gone -- and still the screen's native.
	require.Equal(t, night.villager.ID(), villager.ID(), "the villager answers to his saved id")
	require.Same(t, villager, resumed.gameClient.MapEngine.Entities()[villager.ID()])
	require.Same(t, villager, b4bVillager(t, resumed), "he is still the native the screen holds")

	// Every entity the file names, with its saved id; no other.
	require.Equal(t, b4bIDs(saved), b4bIDs(resumed))

	for _, c := range []*d2mapentity.Creature{night.wounded, night.walker, night.slain} {
		e, ok := resumed.gameClient.MapEngine.Entities()[c.ID()].(*d2mapentity.Creature)
		require.True(t, ok, "%s is a creature on the resumed map", c.ID())
		require.Equal(t, c.MotionSnapshot(), e.MotionSnapshot(), "%s: its walk and pose", c.ID())
		require.Equal(t, c.CreatureID(), e.CreatureID())
	}

	require.True(t, resumed.gameClient.MapEngine.Entities()[night.slain.ID()].(*d2mapentity.Creature).MotionSnapshot().Corpse,
		"the slain lies as a corpse")

	// The bodies are the file's, not adopted fresh.
	for _, b := range w.Bodies {
		got := resumed.bodies[b.ID]
		require.NotNil(t, got, b.ID)
		require.Equal(t, b.Health, got.health, b.ID)
		require.Equal(t, b.MaxHealth, got.maxHealth, b.ID)
	}

	_, waiting := resumed.gameClient.MapEngine.PendingEntityID()
	require.False(t, waiting, "nothing is left waiting in the seam")

	// S_R0 = S_T: the loaded game saves the saved moment.
	_, atR0 := b4File(t, resumed, save, "r0.world.json")
	require.Equal(t, b4Moment(t, atT), b4Moment(t, atR0), "S_R0 = S_T on a hunted night")

	// Run on, entities walking: the two stay one world.
	for i := 0; i < 120; i++ {
		saved.advanceWorld(0.5)
		saved.gameClient.MapEngine.Advance(0.5)
		resumed.advanceWorld(0.5)
		resumed.gameClient.MapEngine.Advance(0.5)
	}

	_, atU := b4File(t, saved, save, "u.world.json")
	_, atR := b4File(t, resumed, save, "r.world.json")
	require.Equal(t, b4Moment(t, atU), b4Moment(t, atR), "S_R = S_U: run on as far, the two are one world")
	require.NotEqual(t, b4Moment(t, atT), b4Moment(t, atU), "and the run on moved the world (the comparison has teeth)")
}

// A HUNTED NIGHT PASSES STEP 1 (B4b): everything B4a refused HUNTED at step 1
// -- an entity the map does not build, a monster's body, a pack, a watch, a
// chase, a deployed squad -- is now a file step 1 takes (the systems' own
// checks of it are step 4's, against the rebuilt map).
func TestAHuntedNightPassesStepOne(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	copyOf := func() *d2save.World {
		data, err := d2save.Encode(good, nil)
		require.NoError(t, err)

		w, err := d2save.Decode(data)
		require.NoError(t, err)

		return w
	}

	entity := d2save.Entity{ID: "0dog", Kind: d2save.KindNPC, Monstat: "fallen1", X: 10, Y: 10,
		Motion: d2mapentity.Motion{Pos: [2]float64{50, 50}, Target: [2]float64{50, 50}}}

	hunted := map[string]func(w *d2save.World){
		"a monster not the map's": func(w *d2save.World) { w.Entities = []d2save.Entity{entity} },
		"a monster's body": func(w *d2save.World) {
			w.Entities, w.Bodies = []d2save.Entity{entity}, []d2save.Body{{ID: "0dog", Health: 3, MaxHealth: 10}}
		},
		"a pack on the map": func(w *d2save.World) {
			w.Entities = []d2save.Entity{entity}
			w.Spawns.Groups = []d2world.SpawnGroupSnapshot{{ID: "g:1", Row: "wolves", Code: "fallen1",
				Members: []d2world.SpawnMemberSnapshot{{ID: "0dog", X: 10, Y: 10}}, Stage: "night"}}
		},
		"a watch": func(w *d2save.World) {
			w.Entities = []d2save.Entity{entity}
			w.Notice.Watches = []d2world.WatchSnapshot{{Watcher: "0dog", Target: d2saveref.Player}}
		},
		"a chase": func(w *d2save.World) {
			w.Entities = []d2save.Entity{entity}
			w.Pursuit.Chases = []d2world.ChaseSnapshot{{Hunter: "0dog", Quarry: d2saveref.Player}}
		},
		"a deployed squad (D3)": func(w *d2save.World) {
			w.Entities = []d2save.Entity{entity}
			w.Squads.NextID = 3
			w.Squads.Squads = append(w.Squads.Squads, d2world.SquadSnapshot{ID: "s:2", Owner: "player", Ordinal: 2,
				Members: []d2world.SquadMemberSnapshot{{Entity: "0dog", Health: 10, Max: 10}}})
		},
	}

	for name, fill := range hunted {
		w := copyOf()
		fill(w)

		s := b4Files(t, w, "Saver", b4Amazon)

		got, r := PrepareLoad(s, false)
		require.Nil(t, r, "%s: step 1 takes a hunted night", name)
		require.NotNil(t, got, name)
		require.Equal(t, len(w.Entities), len(got.Entities), name)

		forgetPreload(s)
	}
}

// STEP 3'S REFUSALS (B4b): a file naming an entity this build cannot rebuild
// as it was saved, or villagers the map does not build as saved, is refused
// before anything but the clock and the entities is restored -- ENTITY for a
// monster, NATIVES for a villager -- and nothing is left waiting in the seam.
func TestARebuildThatCannotBeExactIsRefused(t *testing.T) {
	saved, save := b4bGame(t)
	b4bHunt(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	decode := func() *d2save.World {
		data, err := d2save.Encode(good, nil)
		require.NoError(t, err)

		w, err := d2save.Decode(data)
		require.NoError(t, err)

		return w
	}

	first := func(w *d2save.World, native bool) *d2save.Entity {
		for i := range w.Entities {
			if w.Entities[i].Native == native {
				return &w.Entities[i]
			}
		}

		t.Fatal("no such entity")

		return nil
	}

	cases := map[string]struct {
		code   string
		break_ func(w *d2save.World)
	}{
		"a creature this build's bestiary lacks": {LoadRefusedEntity, func(w *d2save.World) { first(w, false).Creature = "manticore" }},
		"an npc of a monstat this build lacks": {LoadRefusedEntity, func(w *d2save.World) {
			e := first(w, false)
			e.Kind, e.Creature, e.Monstat = d2save.KindNPC, "", "nosuchmonster"
		}},
		"a kind no load rebuilds":         {LoadRefusedEntity, func(w *d2save.World) { first(w, false).Kind = "missile" }},
		"a motion a creature cannot have": {LoadRefusedEntity, func(w *d2save.World) { first(w, false).Motion.Mode = "WL" }},
		// "a name the entry does not give" was refused here until the B4b
		// review fixes (BUG-80): a name is a label, the entry is matched by
		// its creature_id, and a rename resumes with a note
		// (TestARenamedEntryResumesUnderItsNewName).
		"an id the villager takes first": {LoadRefusedEntity, func(w *d2save.World) {
			first(w, false).ID = first(w, true).ID
		}},
		"a villager the map does not build": {LoadRefusedNatives, func(w *d2save.World) {
			e := first(w, true)
			e.Born = &[2]float64{e.Born[0] + 3, e.Born[1]}
		}},
		"a villager of another kind": {LoadRefusedNatives, func(w *d2save.World) {
			e := first(w, true)
			e.Kind, e.Creature, e.Monstat = d2save.KindNPC, "", "fallen1"
		}},
		"a villager's motion no creature can have": {LoadRefusedNatives, func(w *d2save.World) { first(w, true).Motion.Mode = "NU" }},
		"a deployed squad whose model is not on the map": {LoadRefusedBlock, func(w *d2save.World) {
			w.Squads.NextID = 3
			w.Squads.Squads = append(w.Squads.Squads, d2world.SquadSnapshot{ID: "s:2", Owner: "player", Ordinal: 2,
				Morale: 100, Meters: w.Squads.Squads[0].Meters,
				Members: []d2world.SquadMemberSnapshot{{Entity: "0nobody", Health: 10, Max: 10}}})
		}},
	}

	for name, c := range cases {
		v, _ := b4bGame(t)
		w := decode()
		c.break_(w)

		r := v.restoreClock(w)
		require.Nil(t, r, name)

		r = v.checkLoad(w)
		require.NotNil(t, r, "%s: refused", name)
		require.Equal(t, c.code, r.Code, "%s: %v", name, r)

		if c.code == LoadRefusedBlock {
			require.Contains(t, r.Detail, "not on the map", "%s: refused by the map's check, not the snapshot's own", name)
		}
		require.ErrorIs(t, r, ErrLoadRefused)
		require.NotContains(t, v.loadSteps, "validated", "%s: the load stopped before its blocks were passed, let alone restored", name)

		_, waiting := v.gameClient.MapEngine.PendingEntityID()
		require.False(t, waiting, "%s: nothing is left waiting in the seam", name)
	}
}

// A VILLAGER THE FILE DOES NOT HAVE was taken off the map before the save
// (B3-6), and the load takes him off again: the resumed map is the saved
// one, and the next save marks no one native who was not.
func TestAVillagerTheFileLacksIsTakenOff(t *testing.T) {
	saved, save := b4bGame(t)
	b4Busy(t, saved)

	gone := b4bVillager(t, saved)
	saved.gameClient.MapEngine.RemoveEntity(gone)

	w, atT := b4File(t, saved, save, "t.world.json")
	require.Empty(t, w.Entities, "the file has no villager")

	resumed, _ := b4bGame(t)
	require.NoError(t, b4Load(t, resumed, w))
	require.Empty(t, b4bIDs(resumed), "the villager the map built is taken off")

	_, atR0 := b4File(t, resumed, save, "r0.world.json")
	require.Equal(t, b4Moment(t, atT), b4Moment(t, atR0))
}

// NOTHING IS RESTORED TWICE: a second load into the same game -- its bodies
// known, its entities on the map -- is refused by the first check it meets.
func TestNothingIsRestoredTwice(t *testing.T) {
	saved, save := b4bGame(t)
	b4bHunt(t, saved)

	w, _ := b4File(t, saved, save, "t.world.json")

	resumed, _ := b4bGame(t)
	require.NoError(t, b4Load(t, resumed, w))

	// The entities again: each id is on the map already.
	resumed.loadSteps = nil
	r := resumed.checkLoad(w)
	require.NotNil(t, r)
	require.Contains(t, []string{LoadRefusedEntity, LoadRefusedNatives}, r.Code, "%v", r)

	// The bodies again: the game knows them.
	require.Error(t, resumed.restoreBodies(w.Bodies))
	require.Error(t, resumed.checkBodies(nil), "not even an empty block restores over known bodies")
}

// The bodies' own refusals: a body with no monster on the map, and one past
// its maximum. (World.Check refuses the second in a file too; the load's
// check holds the block it restores whatever reached it.)
func TestTheBodiesBlockIsHeldToTheMap(t *testing.T) {
	v, _ := b4bGame(t)
	villager := b4bVillager(t, v)

	require.NoError(t, v.checkBodies([]d2save.Body{{ID: villager.ID(), Health: 3, MaxHealth: 10}}))
	require.Error(t, v.checkBodies([]d2save.Body{{ID: "nobody", Health: 3, MaxHealth: 10}}))
	require.Error(t, v.checkBodies([]d2save.Body{{ID: villager.ID(), Health: 11, MaxHealth: 10}}))
	require.Error(t, v.checkBodies([]d2save.Body{{ID: villager.ID(), Health: -1, MaxHealth: 10}}))
	require.Error(t, v.checkBodies([]d2save.Body{{ID: villager.ID(), Health: 0, MaxHealth: 0}}))
}

// A DEPLOYED SQUAD'S MODEL MUST BE AN NPC ON THE MAP (D3): the squads block's
// half Squads.Validate cannot check, having no map. (That a real deployed
// model -- an NPC, which needs the MPQs -- comes back with its saved id is
// TestSaveResume's.)
func TestADeployedModelIsHeldToTheMap(t *testing.T) {
	v, _ := b4bGame(t)
	villager := b4bVillager(t, v)

	squads := func(model string) d2world.SquadsSnapshot {
		return d2world.SquadsSnapshot{NextID: 3, Selected: "s:1", Squads: []d2world.SquadSnapshot{
			{ID: "s:1", Owner: "player", Ordinal: 1, Members: []d2world.SquadMemberSnapshot{{Entity: d2saveref.Player}}},
			{ID: "s:2", Owner: "player", Ordinal: 2, Members: []d2world.SquadMemberSnapshot{{Entity: model, Health: 5, Max: 5}}},
		}}
	}

	require.NoError(t, v.checkSquadModels(squads(d2saveref.Player)), "the player's squad alone names no model")

	err := v.checkSquadModels(squads("nobody"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not on the map")

	err = v.checkSquadModels(squads(villager.ID()))
	require.Error(t, err, "a creature is not a deployed model")
	require.Contains(t, err.Error(), "is a *d2mapentity.Creature")
}

// THE RE-KEY IS REFUSED WHEN IT CANNOT BE EXACT, changing nothing: the id
// another entity holds (the villager and a monster the file names both
// claim it), which the rebuild then refuses as ENTITY; covered by
// TestARebuildThatCannotBeExactIsRefused. Here, the verb itself on the game's
// engine: another entity's id, and the player's word.
func TestTheReKeyVerbOnTheGamesMap(t *testing.T) {
	v, _ := b4bGame(t)
	villager := b4bVillager(t, v)
	other := b4bCreature(t, v, "wolf", 80, 80)
	engine := v.gameClient.MapEngine
	was := villager.ID()

	for _, id := range []string{other.ID(), d2saveref.Player, ""} {
		err := engine.RekeyEntity(villager, id)
		require.True(t, errors.Is(err, d2mapentity.ErrEntityID), "%q: %v", id, err)
		require.Equal(t, was, villager.ID(), "%q: nothing changed", id)
		require.Same(t, villager, engine.Entities()[was])
	}

	require.NoError(t, engine.RekeyEntity(villager, "0saved-villager"))
	require.Equal(t, "0saved-villager", villager.ID())
	require.Same(t, villager, engine.Entities()["0saved-villager"])

	_, stillThere := engine.Entities()[was]
	require.False(t, stillThere, "the old id is off the map")
	require.True(t, errors.Is(engine.SetNextEntityID("0saved-villager"), d2mapentity.ErrEntityID),
		"the seam will not hand out an id a villager was re-keyed to")
}
