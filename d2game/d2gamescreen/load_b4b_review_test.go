package d2gamescreen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2saveref"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// THE B4b REVIEW FIXES (29 Sep 2026), in a unit game: the review's B1 (a
// death playing at the save; since BUG-87 saved and resumed at its frame), C2 (a villager the file lacks), C3 (a renamed
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

// B1 (BUG-75) AND BUG-87: THE REVIEW'S UNIT PROBE, TestZZRevDeathMidWalk, AS A
// TEST. A wolf walking in is slain (his body at 0, the death animated, as the
// quick resolve leaves him) and the game is run 0, 3 and 8 frames into his
// death. The review found him walking on through it, and a save made then
// resumed him falling again from the first frame, his corpse 0.6 and 1.6
// sub-tiles further on than the saved game's. The B4b review fixes stopped
// the walk (BUG-75) and refused the save while the death played (BUG-76);
// since BUG-87 the save is made, and carries the death at its frame. Now:
//   - his walk ends where his death begins (BUG-75): no route, no target
//     ahead, no velocity, and he does not move through the death;
//   - the save is made while his death plays (BUG-87), and the file holds
//     the death at the frame and the time into it that his sheet shows;
//   - the resumed game is the saved one at once, the death at that frame,
//     and on every frame after: the two deaths end on the same frame, and
//     each corpse lies where his death began; and run on, it is the same.
func TestADeathSavedHalfPlayedResumesAtItsFrame(t *testing.T) {
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

		// BUG-87: saved while the death plays, at its frame.
		wf, atT := b4File(t, saved, save, "t.world.json")

		atFrame := w.MotionSnapshot().ActionAt
		require.NotNil(t, atFrame, "[before=%d] he is still dying at the save", before)

		inFile := b87Entity(t, wf, w.ID())
		require.Equal(t, "death", inFile.Motion.Action, "[before=%d] the file holds his death", before)
		require.Equal(t, atFrame, inFile.Motion.ActionAt, "[before=%d] at its frame", before)

		resumed, _ := b4bGame(t)
		require.NoError(t, b4Load(t, resumed, wf))

		_, atR0 := b4File(t, resumed, save, "r0.world.json")
		require.Equal(t, b4Moment(t, atT), b4Moment(t, atR0), "[before=%d] S_R0 = S_T", before)

		e := resumed.gameClient.MapEngine.Entities()[w.ID()].(*d2mapentity.Creature)
		require.Equal(t, w.MotionSnapshot(), e.MotionSnapshot(), "[before=%d] his death resumed at its frame", before)

		frames := 0
		for ; frames < 200 && !w.MotionSnapshot().Corpse; frames++ {
			b4bfixStep(saved, 1)
			b4bfixStep(resumed, 1)
			require.Equal(t, w.MotionSnapshot(), e.MotionSnapshot(), "[before=%d] %d frames after the save", before, frames+1)
		}

		require.True(t, w.MotionSnapshot().Corpse, "[before=%d] the death played through", before)
		require.True(t, e.MotionSnapshot().Corpse, "[before=%d] and the resumed one on the same frame", before)
		require.Equal(t, fell, w.MotionSnapshot().Pos, "[before=%d] his corpse lies where his death began", before)
		require.Equal(t, fell, e.MotionSnapshot().Pos, "[before=%d] and so does the resumed one", before)

		b4bfixStep(saved, 100)
		b4bfixStep(resumed, 100)

		_, atU := b4File(t, saved, save, "u.world.json")
		_, atR := b4File(t, resumed, save, "r.world.json")
		require.Equal(t, b4Moment(t, atU), b4Moment(t, atR), "[before=%d] S_R = S_U", before)

		t.Logf("[before=%d] saved at frame %d of his death (%v s into it); both lay %d frames after the save, at %v", before,
			atFrame.Frame, atFrame.Elapsed, frames, fell)
	}
}

// b87Entity is the file's entity id.
func b87Entity(t *testing.T, w *d2save.World, id string) d2save.Entity {
	t.Helper()

	for _, e := range w.Entities {
		if e.ID == id {
			return e
		}
	}

	t.Fatalf("%s is not in the file", id)

	return d2save.Entity{}
}

// A SWING AND A BLOW TAKEN, SAVED HALF-PLAYED, RESUME AT THEIR FRAME (BUG-87),
// on a monster walking in and on the villager (a native, re-keyed in place):
// the save is made, the file holds each at its frame, and the resumed game is
// the saved one at once and on every frame after -- each action ending on the
// same frame, back to its idle or its walk -- and run on, S_R = S_U.
func TestASwingAndABlowSavedHalfPlayedResumeAtTheirFrame(t *testing.T) {
	for _, in := range []int{0, 4, 11} {
		saved, save := b4bGame(t)
		night := b4bHunt(t, saved)

		saved.Animate(night.walker.ID(), d2world.ActSwing)
		saved.Animate(night.wounded.ID(), d2world.ActHit)
		saved.Animate(night.villager.ID(), d2world.ActHit)
		b4bfixStep(saved, in)

		held := map[string]string{night.walker.ID(): "attack", night.wounded.ID(): "hit", night.villager.ID(): "hit"}

		wf, atT := b4File(t, saved, save, "t.world.json")

		for id, action := range held {
			mo := saved.gameClient.MapEngine.Entities()[id].(*d2mapentity.Creature).MotionSnapshot()
			require.Equal(t, action, mo.Action, "[in=%d] %s holds its %s at the save", in, id, action)
			require.Equal(t, mo.ActionAt, b87Entity(t, wf, id).Motion.ActionAt, "[in=%d] %s: the file holds it at its frame", in, id)
		}

		resumed, _ := b4bGame(t)
		require.NoError(t, b4Load(t, resumed, wf))

		_, atR0 := b4File(t, resumed, save, "r0.world.json")
		require.Equal(t, b4Moment(t, atT), b4Moment(t, atR0), "[in=%d] S_R0 = S_T", in)

		ended := map[string]int{}

		for f := 1; f <= 60; f++ {
			b4bfixStep(saved, 1)
			b4bfixStep(resumed, 1)

			for id := range held {
				a := saved.gameClient.MapEngine.Entities()[id].(*d2mapentity.Creature).MotionSnapshot()
				b := resumed.gameClient.MapEngine.Entities()[id].(*d2mapentity.Creature).MotionSnapshot()
				require.Equal(t, a, b, "[in=%d] %s, %d frames after the save", in, id, f)

				if a.Action == "" && ended[id] == 0 {
					ended[id] = f
				}
			}
		}

		for id, action := range held {
			require.Positive(t, ended[id], "[in=%d] %s's %s ended within the run", in, id, action)
		}

		_, atU := b4File(t, saved, save, "u.world.json")
		_, atR := b4File(t, resumed, save, "r.world.json")
		require.Equal(t, b4Moment(t, atU), b4Moment(t, atR), "[in=%d] S_R = S_U", in)

		t.Logf("[in=%d] saved with a bite, a blow taken and the villager's blow held; each ended on the same frame in both games: %v", in, ended)
	}
}

// A FILE THAT HOLDS AN ACTION RESUMES IT AT ITS FRAME (BUG-87; the B4b review
// fixes refused such a file, BUG-76): a monster's death, a monster's bite and
// the villager's blow, each written into the file at a frame part-way through
// its sheet, are resumed there -- each entity's motion is the file's, read
// back equal. And a held action the file carries no frame for, or at a frame
// its sheet does not have, is refused where its motion is restored -- ENTITY
// for a monster, NATIVES for a villager -- nothing left waiting in the seam
// (and, as every refusal does, the whole game torn down: step 8).
func TestAFileHoldingAnActionResumesItAtItsFrame(t *testing.T) {
	saved, save := b4bGame(t)
	b4bHunt(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	edit := func(native bool, action string, at *d2mapentity.ActionProgress) (*d2save.World, string) {
		data, err := d2save.Encode(good, nil)
		require.NoError(t, err)

		w, err := d2save.Decode(data)
		require.NoError(t, err)

		for i := range w.Entities {
			if w.Entities[i].Native == native && !w.Entities[i].Motion.Corpse {
				w.Entities[i].Motion.Mode, w.Entities[i].Motion.Action, w.Entities[i].Motion.ActionAt = action, action, at

				return w, w.Entities[i].ID
			}
		}

		t.Fatal("no such entity in the file")

		return nil, ""
	}

	for _, c := range []struct {
		name   string
		native bool
		action string
		at     d2mapentity.ActionProgress
	}{
		{"a monster saved while his death played", false, "death", d2mapentity.ActionProgress{Frame: 5, Elapsed: 0.02}},
		{"a monster saved mid-bite", false, "attack", d2mapentity.ActionProgress{Frame: 3, Elapsed: 0.05}},
		{"a villager saved mid-blow", true, "hit", d2mapentity.ActionProgress{Frame: 2, Elapsed: 0.1}},
	} {
		at := c.at
		w, id := edit(c.native, c.action, &at)

		v, _ := b4bGame(t)
		require.NoError(t, b4Load(t, v, w), c.name)

		got := v.gameClient.MapEngine.Entities()[id].(*d2mapentity.Creature).MotionSnapshot()
		require.Equal(t, b87Entity(t, w, id).Motion, got, "%s: resumed at its frame", c.name)
	}

	for _, c := range []struct {
		name   string
		native bool
		at     *d2mapentity.ActionProgress
		code   string
		why    string
	}{
		{"a monster's death with no frame", false, nil, LoadRefusedEntity, "carries no frame"},
		{"a monster's death at a frame its sheet lacks", false, &d2mapentity.ActionProgress{Frame: 99}, LoadRefusedEntity, "no such frame"},
		{"a villager's blow with no frame", true, nil, LoadRefusedNatives, "carries no frame"},
		{"a villager's blow at a frame its sheet lacks", true, &d2mapentity.ActionProgress{Frame: 99}, LoadRefusedNatives, "no such frame"},
	} {
		action := "death"
		if c.native {
			action = "hit"
		}

		w, _ := edit(c.native, action, c.at)

		v, _ := b4bGame(t)
		require.Nil(t, v.restoreClock(w), c.name)

		r := v.checkLoad(w)
		require.NotNil(t, r, "%s: refused", c.name)
		require.Equal(t, c.code, r.Code, "%s: %v", c.name, r)
		require.Contains(t, r.Detail, c.why, c.name)

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
