package d2gamescreen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2saveref"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// THE B4b REVIEW FIXES (29 Sep 2026), in a unit game: the review's B1 (a
// death playing at the save), C2 (a villager the file lacks), C3 (a renamed
// bestiary entry), C7 (a risen man walking at T) and the merge scout's
// BUG-86 (the village's sound in the digest's world part). The playtest
// halves are TestSaveResume's act 6i, TestASlainWalkerLiesWhereHeFell and
// TestSaveResumeInTheFirstSecond.

// b4bfixStep runs v n frames of 40 ms, the world and the map, as a frame does.
func b4bfixStep(v *Game, n int) {
	for i := 0; i < n; i++ {
		v.advanceWorld(0.04)
		v.gameClient.MapEngine.Advance(0.04)
	}
}

// B1 (BUG-75, BUG-76): THE REVIEW'S UNIT PROBE, TestZZRevDeathMidWalk, AS A
// TEST. A wolf walking in is slain (his body at 0, the death animated, as the
// quick resolve leaves him) and the game is run 0, 3 and 8 frames into his
// death. The review found him walking on through it, and a save made then
// resumed him falling again from the first frame, his corpse 0.6 and 1.6
// sub-tiles further on than the saved game's. Now:
//   - his walk ends where his death begins (BUG-75): no route, no target
//     ahead, no velocity, and he does not move through the death;
//   - no save is made while his death plays (BUG-76): refused FIGHTING,
//     naming it, and no file is touched;
//   - once he lies, the save is made, and the resumed game is the saved one
//     at once and run on -- his corpse where his death began, in both.
func TestADeathIsNeitherWalkedNorSavedHalfPlayed(t *testing.T) {
	for _, before := range []int{0, 3, 8} {
		saved, save := b4bGame(t)
		b4Busy(t, saved)

		w := b4bCreature(t, saved, "wolf", 100, 100)
		saved.adoptNPCBody(w.ID(), 96)
		w.SetPath([]d2vector.Position{d2vector.NewPosition(200, 100), d2vector.NewPosition(300, 100)}, nil)
		b4bfixStep(saved, 5)
		require.True(t, w.IsMoving(), "[before=%d] he is walking in", before)

		saved.bodies[w.ID()].health = 0
		saved.Animate(w.ID(), d2world.ActDie)

		fell := w.MotionSnapshot().Pos
		mo := w.MotionSnapshot()
		require.Equal(t, "death", mo.Action, "[before=%d]", before)
		require.Equal(t, fell, mo.Target, "[before=%d] BUG-75: no step ahead of a dying wolf", before)
		require.Empty(t, mo.Path, "[before=%d] BUG-75: and no route", before)
		require.Equal(t, [2]float64{}, mo.Velocity, "[before=%d] BUG-75: and no velocity", before)

		b4bfixStep(saved, before)
		require.Equal(t, fell, w.MotionSnapshot().Pos, "[before=%d] BUG-75: he does not walk while he dies", before)

		// BUG-76: no save while the death plays.
		refusedTo := filepath.Join(filepath.Dir(save), "refused.world.json")
		b3Refused(t, saved, SaveRefusedFighting, SaveOptions{To: refusedTo})

		_, err := saved.SaveWorld(SaveOptions{To: refusedTo})
		require.Contains(t, err.Error(), w.ID(), "[before=%d] the refusal names him", before)
		require.Contains(t, err.Error(), "death", "[before=%d] and his death", before)
		require.NoFileExists(t, refusedTo, "[before=%d] a refusal touches no file", before)

		frames := 0
		for ; frames < 200 && !w.MotionSnapshot().Corpse; frames++ {
			b4bfixStep(saved, 1)
		}

		require.True(t, w.MotionSnapshot().Corpse, "[before=%d] the death played through", before)
		require.Equal(t, fell, w.MotionSnapshot().Pos, "[before=%d] his corpse lies where his death began", before)

		wf, atT := b4File(t, saved, save, "t.world.json")

		resumed, _ := b4bGame(t)
		require.NoError(t, b4Load(t, resumed, wf))

		_, atR0 := b4File(t, resumed, save, "r0.world.json")
		require.Equal(t, b4Moment(t, atT), b4Moment(t, atR0), "[before=%d] S_R0 = S_T", before)

		e := resumed.gameClient.MapEngine.Entities()[w.ID()].(*d2mapentity.Creature)

		b4bfixStep(saved, 100)
		b4bfixStep(resumed, 100)

		_, atU := b4File(t, saved, save, "u.world.json")
		_, atR := b4File(t, resumed, save, "r.world.json")
		require.Equal(t, b4Moment(t, atU), b4Moment(t, atR), "[before=%d] S_R = S_U", before)
		require.Equal(t, fell, e.MotionSnapshot().Pos, "[before=%d] the resumed corpse lies where he fell", before)
		require.Equal(t, fell, w.MotionSnapshot().Pos, "[before=%d] and so does the saved one", before)

		t.Logf("[before=%d] refused for %d frames of his death; then saved and resumed, the corpse at %v in both", before, frames, fell)
	}
}

// A FILE THAT HOLDS AN ACTION IS REFUSED (BUG-76): the save never writes one,
// so a file that does was not written by this build's save -- and resumed, the
// action would play again from its first frame. A monster's is refused ENTITY
// and a villager's NATIVES, before anything is rebuilt or re-keyed.
func TestAFileHoldingAnActionIsRefused(t *testing.T) {
	saved, save := b4bGame(t)
	b4bHunt(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	for _, c := range []struct {
		name   string
		native bool
		action string
		code   string
	}{
		{"a monster saved while his death played", false, "death", LoadRefusedEntity},
		{"a monster saved mid-bite", false, "attack", LoadRefusedEntity},
		{"a villager saved mid-blow", true, "hit", LoadRefusedNatives},
	} {
		data, err := d2save.Encode(good, nil)
		require.NoError(t, err)

		w, err := d2save.Decode(data)
		require.NoError(t, err)

		for i := range w.Entities {
			if w.Entities[i].Native == c.native && !w.Entities[i].Motion.Corpse {
				w.Entities[i].Motion.Action = c.action

				break
			}
		}

		v, _ := b4bGame(t)
		require.Nil(t, v.restoreClock(w), c.name)

		r := v.checkLoad(w)
		require.NotNil(t, r, "%s: refused", c.name)
		require.Equal(t, c.code, r.Code, "%s: %v", c.name, r)
		require.Contains(t, r.Detail, "played", c.name)

		_, waiting := v.gameClient.MapEngine.PendingEntityID()
		require.False(t, waiting, "%s: nothing is left waiting in the seam", c.name)
	}
}

// C3 (BUG-80; decision B4b-5 overturned): A RENAMED BESTIARY ENTRY RESUMES
// UNDER ITS NEW NAME. The entity is matched on its kind and its record -- the
// bestiary entry by creature_id -- and its name is a label: a file whose wolf
// was saved as "Wolf" loads in a build that calls him something else, and the
// load says so, in its report and its log. (B4b refused it ENTITY, so a
// label's rename threw away every save with a wolf in it.)
func TestARenamedEntryResumesUnderItsNewName(t *testing.T) {
	saved, save := b4bGame(t)
	night := b4bHunt(t, saved)

	w, _ := b4File(t, saved, save, "t.world.json")

	renamed := ""

	for i := range w.Entities {
		if w.Entities[i].ID == night.walker.ID() {
			w.Entities[i].NameKey = "A grey wolf"
			renamed = w.Entities[i].ID
		}
	}

	require.NotEmpty(t, renamed)

	setLastLoad(LoadReport{})

	resumed, _ := b4bGame(t)
	require.NoError(t, b4Load(t, resumed, w), "a renamed entry resumes")

	e, ok := resumed.gameClient.MapEngine.Entities()[renamed].(*d2mapentity.Creature)
	require.True(t, ok, "he is on the map under his saved id")
	require.Equal(t, night.walker.CreatureID(), e.CreatureID(), "rebuilt from the entry the file names")
	require.Equal(t, night.walker.Label(), e.Label(), "under the name this build gives it")

	notes := LastLoad().Notes
	require.Len(t, notes, 1, "%v", notes)
	require.Contains(t, notes[0], renamed)
	require.Contains(t, notes[0], `"A grey wolf"`)
	require.Contains(t, notes[0], `"`+e.Label()+`"`)
}

// C2 (BUG-79): A VILLAGER THE FILE LACKS IS TAKEN OFF THE MAP AND SAID TO BE.
// TestAVillagerTheFileLacksIsTakenOff (B4b) holds the map and the re-save;
// here the rest the review asked for: a line in the load's report (and its
// log), and he is no longer held as a native -- neither in the screen's
// natives nor in the scene provider's report of them.
func TestAVillagerTheFileLacksIsNotedAndForgotten(t *testing.T) {
	saved, save := b4bGame(t)
	b4Busy(t, saved)

	saved.gameClient.MapEngine.RemoveEntity(b4bVillager(t, saved))

	w, _ := b4File(t, saved, save, "t.world.json")

	resumed, _ := b4bGame(t)
	gone := b4bVillager(t, resumed)

	setLastLoad(LoadReport{})
	require.NoError(t, b4Load(t, resumed, w))

	require.Empty(t, resumed.natives, "he is no longer held as a native")

	notes := LastLoad().Notes
	require.Len(t, notes, 1, "%v", notes)
	require.Contains(t, notes[0], gone.ID())
	require.Contains(t, notes[0], "taken off")

	natives, _ := sceneProvider{resumed}.HarnessState()["natives"].([]map[string]interface{})
	require.Empty(t, natives, "the scene reports no native")

	// And the game that took him off reports him gone too: the provider
	// reports only the natives on the map, so the two games agree.
	natives, _ = sceneProvider{saved}.HarnessState()["natives"].([]map[string]interface{})
	require.Empty(t, natives, "the saved game reports no native either")
}

// C7: A RISEN MAN WALKING AT T RESUMES. The review read, and did not run, that
// the risen are rebuilt by the pack's path (Spawns.Raise -> gameSpawner.Spawn
// -> the bestiary's strigoi). Here one is on the map at T -- the strigoi
// entry, in a group of the risen row, a watch and a chase on him, walking in
// -- and the resumed game is the saved one at once and run on, the risen man
// still walking in both. (The unit spawner cannot stand one up itself: it has
// no stand-in monstat to place him with, so he is built as b4bHunt builds the
// pack, and his group put on the tables through Restore.)
func TestARisenManWalkingAtTResumes(t *testing.T) {
	saved, save := b4bGame(t)
	b4Busy(t, saved)

	risen := b4bCreature(t, saved, "strigoi", 120, 110)
	saved.adoptNPCBody(risen.ID(), 72)

	// Walking in: a route ahead, as a chase hands one (and the chase too).
	risen.SetPath([]d2vector.Position{d2vector.NewPosition(135, 112), d2vector.NewPosition(140, 112)}, nil)

	require.True(t, saved.Watch(risen, saved.localPlayer))
	require.True(t, saved.Pursue(risen, saved.localPlayer))

	r := worldResolver{saved}
	snap, err := saved.spawns.Snapshot(r)
	require.NoError(t, err)

	x, y := risen.GetPositionF()
	snap.NextID = 2
	snap.Groups = []d2world.SpawnGroupSnapshot{{
		ID: "g:1", Row: d2world.RisenRow, Code: "fallen1", Morale: 0, BornAt: saved.worldClock.WorldMinutes() - 1,
		Stage: "night", Weight: 0, Spawned: 1, BornWhere: [][2]float64{{x, y}},
		Members: []d2world.SpawnMemberSnapshot{{ID: risen.ID(), X: x, Y: y}},
	}}
	require.NoError(t, saved.spawns.Restore(snap, r, saved.gameClient.Seed))

	b4bfixStep(saved, 3)
	require.True(t, risen.IsMoving(), "the risen man walks in at T")

	w, atT := b4File(t, saved, save, "t.world.json")
	require.Len(t, w.Spawns.Groups, 1)
	require.Equal(t, d2world.RisenRow, w.Spawns.Groups[0].Row)
	require.Len(t, w.Pursuit.Chases, 1)
	require.Equal(t, risen.ID(), w.Pursuit.Chases[0].Hunter)
	require.Equal(t, d2saveref.Player, w.Pursuit.Chases[0].Quarry)

	resumed, _ := b4bGame(t)
	require.NoError(t, b4Load(t, resumed, w))

	e, ok := resumed.gameClient.MapEngine.Entities()[risen.ID()].(*d2mapentity.Creature)
	require.True(t, ok, "the risen man is on the resumed map under his saved id")
	require.Equal(t, "strigoi", e.CreatureID())
	require.Equal(t, risen.MotionSnapshot(), e.MotionSnapshot(), "mid-walk, as saved")

	_, atR0 := b4File(t, resumed, save, "r0.world.json")
	require.Equal(t, b4Moment(t, atT), b4Moment(t, atR0), "S_R0 = S_T with a risen man walking")

	b4bfixStep(saved, 60)
	b4bfixStep(resumed, 60)

	_, atU := b4File(t, saved, save, "u.world.json")
	_, atR := b4File(t, resumed, save, "r.world.json")
	require.Equal(t, b4Moment(t, atU), b4Moment(t, atR), "S_R = S_U")

	for _, se := range w.Entities {
		if se.ID == risen.ID() {
			require.NotEqual(t, se.Motion.Pos, e.MotionSnapshot().Pos, "and he walked on after T")
		}
	}
}

// BUG-86 (the merge scout's): THE VILLAGE'S SOUND IS THIS PROCESS'S
// PRESENTATION. A new game reads its region a second into play and a load at
// once, so a save in a new game's first second resumed with the sound set
// where the saved game had none. The village provider now puts the sound
// environment and its song in the digest's process part, as the ui provider's
// talent cells went (BUG-63), and keeps the rest in the world part.
func TestTheVillagesSoundIsThisProcesssPresentation(t *testing.T) {
	v, _ := b4bGame(t)

	all := villageProvider{v}.HarnessState()
	world, process := villageProvider{v}.HarnessDigest()

	for _, key := range []string{"sound_env", "music"} {
		require.Contains(t, all, key, "the provider still reports %s", key)
		require.NotContains(t, world, key, "%s is not the world's", key)
		require.Contains(t, process, key, "%s is this process's", key)
		require.Equal(t, all[key], process[key], key)
	}

	for key, value := range all {
		if key == "sound_env" || key == "music" {
			continue
		}

		require.Equal(t, value, world[key], "%s stays in the world part", key)
	}

	require.Len(t, process, 2, "nothing else moved: %v", process)
	require.Len(t, world, len(all)-2, "and nothing else left the world part: %v", world)
}
