package d2gamescreen

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2saveref"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// THE COMBAT STATUS (Josh, 30 Sep 2026: "you can't save while in combat";
// combat_status.go), in a unit game with no MPQs: each trigger alone puts him
// in combat and the save is refused COMBAT; the grace holds, then clears; a
// clock fight alone does not; every save path -- the menu, the dawn autosave
// (pending, then taken), the close (left unsaved while chased; the grace
// waited out) -- refuses; an accepted save is never in combat; and a file
// from before this burst holding a chase of him resumes with him in combat.
// The playtest is TestTheCombatStatus (playtest/combat_status_test.go).

// csFrame is one frame's end, as Advance runs it: the combat status, then the
// dawn autosave.
const csFrame = closeFrameSeconds

// csGame is a savable unit game whose saves write all three files (b5rGame),
// with him named and standing in the open, the combat model's Resolver
// attached as CreateGame attaches it, and the shipped grace.
func csGame(t *testing.T) (*Game, string) {
	t.Helper()

	v, save := b5rGame(t)
	b3SetField(t, v.localPlayer, "uuid", "p-him")
	v.localPlayer.StandAt(100, 100, 0)
	v.combat.SetResolver(worldResolver{v})
	v.combatGraceSeconds = DefaultCombatGraceSeconds

	return v, save
}

// csTo is a path beside his save for a save that must not touch his files.
func csTo(save, name string) SaveOptions {
	return SaveOptions{To: filepath.Join(filepath.Dir(save), name)}
}

// csDog is a feral dog one tile east of him, with a body.
func csDog(t *testing.T, v *Game) *d2mapentity.Creature {
	t.Helper()

	dog := b3Dog(t, v.gameClient.MapEngine, 105, 100)
	dog.SetCreatureID("feral-dog")
	v.adoptNPCBody(dog.ID(), 20)

	return dog
}

// csFramesOut runs frame ends until he is out of combat, up to limit, and
// returns how many it took (limit+1 when he never came out).
func csFramesOut(v *Game, limit int) int {
	for i := 1; i <= limit; i++ {
		v.advanceCombatStatus(csFrame)

		if !v.InCombat() {
			return i
		}
	}

	return limit + 1
}

// csSaves is whether a save to a path beside his is made now.
func csSaves(t *testing.T, v *Game, save, name string) bool {
	t.Helper()

	_, err := v.SaveWorld(csTo(save, name))
	if err == nil {
		return true
	}

	var r *SaveRefusal
	require.True(t, errors.As(err, &r), "a save not made is refused, not failed: %v", err)

	return false
}

// EACH TRIGGER ALONE PUTS HIM IN COMBAT, and each is refused COMBAT: a hostile
// chasing him, his swing, his hit reaction, his block. A frame's end with the
// trigger on holds the grace full; with it off he stays in combat for the
// grace -- 3 s, 180 frames of a sixtieth -- and then saves. THE CONTROL: out of
// combat before, the same game saves.
func TestEachTriggerAlonePutsHimInCombat(t *testing.T) {
	cases := []struct {
		name, code string
		on, off    func(t *testing.T, v *Game)
	}{
		{"a hostile chasing him", CombatChased,
			func(t *testing.T, v *Game) { require.True(t, v.Pursue(csDog(t, v), v.localPlayer)) },
			func(t *testing.T, v *Game) {
				for _, id := range v.pursuit.ChasersOf(v.localPlayer.ID()) {
					require.True(t, v.pursuit.Release(id))
				}
			}},
		{"his swing", CombatSwing,
			func(t *testing.T, v *Game) { b3SetField(t, v.localPlayer, "isCasting", true) },
			func(t *testing.T, v *Game) { b3SetField(t, v.localPlayer, "isCasting", false) }},
		{"his hit reaction (BUG-105)", CombatReaction,
			func(t *testing.T, v *Game) {
				b3SetField(t, v.localPlayer, "actionHeld", true)
				b3SetField(t, v.localPlayer, "actionMode", d2enum.PlayerAnimationModeGetHit)
			},
			func(t *testing.T, v *Game) { b3SetField(t, v.localPlayer, "actionHeld", false) }},
		{"his block (BUG-105)", CombatReaction,
			func(t *testing.T, v *Game) {
				b3SetField(t, v.localPlayer, "actionHeld", true)
				b3SetField(t, v.localPlayer, "actionMode", d2enum.PlayerAnimationModeBlock)
			},
			func(t *testing.T, v *Game) { b3SetField(t, v.localPlayer, "actionHeld", false) }},
	}

	for _, c := range cases {
		v, save := csGame(t)

		// The control: out of combat, the game saves.
		require.False(t, v.InCombat(), c.name)
		require.True(t, csSaves(t, v, save, "before.world.json"), "%s: out of combat, a save is made", c.name)

		c.on(t, v)
		code, detail := v.CombatReason()
		require.Equal(t, c.code, code, "%s: %s", c.name, detail)
		require.True(t, v.InCombat(), c.name)
		b3Refused(t, v, SaveRefusedCombat, csTo(save, "in.world.json"))

		v.advanceCombatStatus(csFrame)
		require.Equal(t, DefaultCombatGraceSeconds, v.combatGrace, "%s: a frame with the trigger on holds the grace full", c.name)

		c.off(t, v)
		code, _ = v.CombatReason()
		require.Equal(t, CombatGrace, code, "%s: the trigger gone, the grace holds him", c.name)
		b3Refused(t, v, SaveRefusedCombat, csTo(save, "grace.world.json"))

		n := csFramesOut(v, 400)
		require.InDelta(t, 180, n, 1, "%s: out of combat 3 s of game time after the trigger ended", c.name)
		require.Zero(t, v.combatGrace, "%s: the grace ends at 0 exactly (the floor)", c.name)
		require.Empty(t, v.combatLast, c.name)
		require.True(t, csSaves(t, v, save, "after.world.json"), "%s: out of combat, the save is made", c.name)
	}

	// His death is not a reaction: a dead man is refused DEAD, before combat.
	v, save := csGame(t)
	b3SetField(t, v.localPlayer, "actionHeld", true)
	b3SetField(t, v.localPlayer, "actionMode", d2enum.PlayerAnimationModeDeath)
	require.False(t, v.localPlayer.Reacting(), "his death is not a reaction")
	require.False(t, v.InCombat())

	v.localPlayer.Stats.Health = 0
	b3Refused(t, v, SaveRefusedDead, csTo(save, "dead.world.json"))
}

// HIS FIGHT ALONE PUTS HIM IN COMBAT: a dog beside him, aware of him, opens
// his fight (the game's own chain: the watch, the chase, the combat model), and
// while it is live -- his turn open (Awaiting), the world held -- the save is
// refused COMBAT for the fight. Ended (disengaged) and let go, he stays in
// combat for the grace and then saves.
func TestHisFightPutsHimInCombat(t *testing.T) {
	v, save := csGame(t)
	require.True(t, csSaves(t, v, save, "before.world.json"), "the control: out of combat, a save is made")

	dog := csDog(t, v)
	require.True(t, v.Watch(dog, v.localPlayer))

	for i := 0; i < 120 && !v.combat.Fighting(); i++ {
		v.advanceWorld(csFrame)
	}

	require.True(t, v.combat.Fighting(), "a dog beside him, aware, opens his fight")

	code, detail := v.CombatReason()
	require.Equal(t, CombatFight, code, detail)
	require.Contains(t, detail, v.combat.Encounter())
	b3Refused(t, v, SaveRefusedCombat, csTo(save, "fight.world.json"))

	// An open turn is his fight's, and the status says fight.
	if v.combat.Awaiting() {
		code, _ = v.CombatReason()
		require.Equal(t, CombatFight, code)
	}

	v.advanceCombatStatus(csFrame)

	// Ended, and the dog let go (unwatched, its chase released).
	require.NoError(t, v.combat.HarnessSet("disengage", true))
	v.notice.Unwatch(dog.ID())
	v.pursuit.Release(dog.ID())
	require.False(t, v.combat.Fighting())

	code, _ = v.CombatReason()
	require.Equal(t, CombatGrace, code, "his fight over, the grace holds him")

	frames := 0
	for ; frames < 400 && v.InCombat(); frames++ {
		v.advanceWorld(csFrame)
		v.earnExperience()
		v.advanceCombatStatus(csFrame)
	}

	require.InDelta(t, 180, frames, 1, "out of combat 3 s after his fight ended")
	require.True(t, csSaves(t, v, save, "after.world.json"), "out of combat after his fight, the save is made")
}

// A CLOCK FIGHT ALONE DOES NOT PUT HIM IN COMBAT (the raid's R1: a fight he is
// not in never stops a save, Q5 (a)). Two village fights live on the map, and
// he is out of combat, the provider says so, the save is made, and frames on
// he stays out. THE CONTROL: the fights are live.
func TestAClockFightAloneDoesNotPutHimInCombat(t *testing.T) {
	v, save, _ := cwTwoFights(t)
	v.combatGraceSeconds = DefaultCombatGraceSeconds

	live := v.combat.HarnessState()["clock"].(map[string]interface{})["live"].([]map[string]interface{})
	require.Len(t, live, 2, "the control: two clock fights live")
	require.False(t, v.combat.Fighting(), "neither is his")

	require.False(t, v.InCombat())
	require.Equal(t, false, saveProvider{v}.HarnessState()["in_combat"])

	for i := 0; i < 10; i++ {
		v.advanceCombatStatus(csFrame)
		require.False(t, v.InCombat(), "frame %d", i)
	}

	require.True(t, csSaves(t, v, save, "clock.world.json"), "a save is made while the village fights")
}

// THE GRACE HOLDS, THEN CLEARS, at the dial: 3 s (180 frames of a sixtieth),
// 1.5 s (90), and 0 -- out on the first frame the trigger is gone. A trigger
// that comes back inside the grace fills it again. The dial is the harness's
// save.combat_grace; it refuses what is not seconds.
func TestTheGraceHoldsThenClears(t *testing.T) {
	for _, c := range []struct {
		dial   float64
		frames int
	}{{3, 180}, {1.5, 90}, {0, 1}} {
		v, _ := csGame(t)
		require.NoError(t, saveProvider{v}.HarnessSet("combat_grace", c.dial))
		require.Equal(t, c.dial, saveProvider{v}.HarnessState()["combat_grace"])

		b3SetField(t, v.localPlayer, "isCasting", true)
		v.advanceCombatStatus(csFrame)
		b3SetField(t, v.localPlayer, "isCasting", false)

		require.InDelta(t, c.frames, csFramesOut(v, 400), 1, "dial %v", c.dial)
	}

	// A trigger inside the grace fills it again.
	v, _ := csGame(t)
	b3SetField(t, v.localPlayer, "isCasting", true)
	v.advanceCombatStatus(csFrame)
	b3SetField(t, v.localPlayer, "isCasting", false)

	for i := 0; i < 100; i++ {
		v.advanceCombatStatus(csFrame)
	}

	require.Less(t, v.combatGrace, 2.0)

	b3SetField(t, v.localPlayer, "isCasting", true)
	v.advanceCombatStatus(csFrame)
	b3SetField(t, v.localPlayer, "isCasting", false)
	require.Equal(t, DefaultCombatGraceSeconds, v.combatGrace, "the trigger again: the grace is full again")
	require.InDelta(t, 180, csFramesOut(v, 400), 1)

	// The dial's refusals.
	for _, bad := range []interface{}{-1.0, "3", 4000.0} {
		require.Error(t, saveProvider{v}.HarnessSet("combat_grace", bad), "%v", bad)
	}

	require.NoError(t, saveProvider{v}.HarnessSet("combat_grace", 2))
	require.Equal(t, 2.0, v.combatGraceSeconds, "a whole number of seconds is seconds")
}

// A CHASE THAT FLICKERS STAYS COMBAT (Josh, 30 Sep 2026: "chased" must latch
// for a few seconds; the R2 review's B1: with a second quarry near, a chase on
// him can drop for 0.4-0.8 s and come back). The grace is the latch: a chase
// released for 0.8 s and taken again keeps him in combat on every frame
// between, with no COMBAT out edge (combatLast is never cleared), and the save
// is refused COMBAT throughout. THE CONTROL, in the test: the same flicker
// with a grace shorter than it (0.3 s) does take him out of combat -- so it is
// the latch, and nothing else, that holds him.
func TestAChaseThatFlickersStaysCombat(t *testing.T) {
	const flickerFrames = 48 // 0.8 s at a sixtieth: the longest B1 measured

	flicker := func(t *testing.T, grace float64) (outFrames int) {
		t.Helper()

		v, save := csGame(t)
		require.NoError(t, saveProvider{v}.HarnessSet("combat_grace", grace))

		dog := csDog(t, v)
		require.True(t, v.Pursue(dog, v.localPlayer))

		for i := 0; i < 30; i++ {
			v.advanceCombatStatus(csFrame)
		}

		code, _ := v.CombatReason()
		require.Equal(t, CombatChased, code)

		require.True(t, v.pursuit.Release(dog.ID()), "the chase drops (the flicker)")

		for i := 0; i < flickerFrames; i++ {
			v.advanceCombatStatus(csFrame)

			if !v.InCombat() || v.combatLast == "" {
				outFrames++

				continue
			}

			b3Refused(t, v, SaveRefusedCombat, csTo(save, "flicker.world.json"))
		}

		if outFrames > 0 {
			return outFrames // out of combat in the gap: the caller judges it
		}

		require.True(t, v.Pursue(dog, v.localPlayer), "the chase comes back")
		v.advanceCombatStatus(csFrame)

		code, _ = v.CombatReason()
		require.Equal(t, CombatChased, code, "chased again")
		require.Equal(t, grace, v.combatGrace, "the chase back fills the latch again")

		return outFrames
	}

	require.Zero(t, flicker(t, DefaultCombatGraceSeconds),
		"a 0.8 s gap in the chase never takes him out of combat at the shipped grace")
	require.Positive(t, flicker(t, 0.3),
		"the control: a latch shorter than the gap lets him out -- the latch is what holds him")
}

// EVERY SAVE PATH REFUSES COMBAT, and each says so in his words: the menu's
// SAVE GAME and SAVE AND EXIT GAME (nothing written, the exit is EXIT WITHOUT
// SAVING's), the dawn autosave (pending while he is chased and through the
// grace, then taken on the first frame out), and the close (chased: it leaves
// without saving at once, no frame run for a chase; in the grace: it waits
// out the grace's remainder, and saves).
func TestEverySavePathRefusesCombat(t *testing.T) {
	chase := func(t *testing.T, v *Game) *d2mapentity.Creature {
		t.Helper()

		dog := csDog(t, v)
		require.True(t, v.Pursue(dog, v.localPlayer))

		return dog
	}

	// The menu.
	v, save := csGame(t)
	chase(t, v)

	note := v.SaveRefusedNow()
	require.Equal(t, d2player.SaveRefusedCombatWords+"\n"+d2player.MenuExitWithoutSaved, note)
	require.Equal(t, "You can't save in combat.", strings.Split(note, "\n")[0])

	for _, exit := range []bool{false, true} {
		res := v.SaveFromMenu(exit)
		require.False(t, res.Saved, "exit %v", exit)
		require.True(t, strings.HasPrefix(res.Words, d2player.SaveRefusedCombatWords), "exit %v: %q", exit, res.Words)
		require.Equal(t, SaveRefusedCombat, v.lastSave.Code)
		require.Equal(t, d2player.SaveRefusedCombatWords, v.lastSave.Words)
	}

	_, err := os.Stat(d2save.WorldPath(save))
	require.True(t, errors.Is(err, os.ErrNotExist), "the menu wrote no world file in combat")

	// The dawn autosave: pending while he is chased, and through the grace;
	// taken on the first frame he is out of combat.
	v, save = csGame(t)
	dog := chase(t, v)

	v.armDawnAutosave(1)

	for i := 0; i < 30; i++ {
		v.advanceCombatStatus(csFrame)
		v.advanceAutosave()
		require.Equal(t, AutosavePending, v.autosave.State, "frame %d", i)
	}

	require.Equal(t, SaveRefusedCombat, v.autosave.LastCode)
	require.Equal(t, 30, v.autosave.Tries)

	require.True(t, v.pursuit.Release(dog.ID()))

	frames := 0
	for ; frames < 400 && v.autosave.State == AutosavePending; frames++ {
		v.advanceCombatStatus(csFrame)
		v.advanceAutosave()
	}

	require.Equal(t, AutosaveTaken, v.autosave.State)
	require.Zero(t, v.combatGrace, "the autosave is taken with no grace left")
	require.Equal(t, SaveByDawn, v.autosave.By)
	require.InDelta(t, 180, frames, 1, "taken on the first frame out of combat, 3 s after the chase ended")
	require.NotEmpty(t, b5rSavedAt(t, save))

	// The close, chased: no frame is run for a chase, and he leaves unsaved.
	v, save = csGame(t)
	chase(t, v)

	rep, unloads := b5rClose(t, v, nil)
	require.False(t, rep.Saved, "%+v", rep)
	require.Equal(t, SaveRefusedCombat, rep.Refused)
	require.Equal(t, d2player.SaveRefusedCombatWords, rep.Words)
	require.Zero(t, rep.SettleFrames, "a chase does not end on its own: the close does not wait for it")
	require.Equal(t, 1, unloads, "unsaved, the close still unloads")

	_, err = os.Stat(d2save.WorldPath(save))
	require.True(t, errors.Is(err, os.ErrNotExist), "a close in combat writes no world file")

	// The close in the grace: it waits out the remainder, and saves.
	v, save = csGame(t)
	dog = chase(t, v)
	v.advanceCombatStatus(csFrame)
	require.True(t, v.pursuit.Release(dog.ID()))

	for i := 0; i < 60; i++ { // a second of the grace spent before the close
		v.advanceCombatStatus(csFrame)
	}

	rep, _ = b5rClose(t, v, func(v *Game) error {
		v.advanceCombatStatus(csFrame)
		return nil
	})
	require.True(t, rep.Saved, "the grace waited out, the close saves: %+v", rep)
	require.InDelta(t, 120, rep.SettleFrames, 1, "the grace's remainder, and no more")
	require.NotEmpty(t, b5rSavedAt(t, save))
}

// AN ACCEPTED SAVE IS NEVER IN COMBAT (the world file carries no grace, and
// needs none). Triggers on and off over 600 frames -- a chase, his swing, his
// reaction, each for a while and then gone -- and a save tried at every
// frame's end: every save made finds the grace at 0 and no trigger; every one
// refused while either is set. THE CONTROL: saves were made, and saves were
// refused.
func TestAnAcceptedSaveIsOutOfCombat(t *testing.T) {
	v, save := csGame(t)
	dog := csDog(t, v)

	made, refused := 0, 0

	for f := 0; f < 600; f++ {
		switch f {
		case 50:
			require.True(t, v.Pursue(dog, v.localPlayer))
		case 120:
			v.pursuit.Release(dog.ID())
		case 350:
			b3SetField(t, v.localPlayer, "isCasting", true)
		case 360:
			b3SetField(t, v.localPlayer, "isCasting", false)
			b3SetField(t, v.localPlayer, "actionHeld", true)
			b3SetField(t, v.localPlayer, "actionMode", d2enum.PlayerAnimationModeGetHit)
		case 390:
			b3SetField(t, v.localPlayer, "actionHeld", false)
		}

		v.advanceCombatStatus(csFrame)

		code, _ := v.combatTrigger()

		if csSaves(t, v, save, "every.world.json") {
			made++

			require.Zero(t, v.combatGrace, "frame %d: a save was made with the grace running", f)
			require.Empty(t, code, "frame %d: a save was made with a trigger on", f)
		} else {
			refused++

			require.True(t, v.combatGrace > 0 || code != "", "frame %d: refused out of combat", f)
		}
	}

	require.Positive(t, made)
	require.Positive(t, refused)
	t.Logf("%d saves made, %d refused in combat", made, refused)
}

// A FILE FROM BEFORE THE COMBAT STATUS -- saved while a hostile chased him, as
// B4b's hunted night was -- still resumes, and he is in combat the moment it
// does: the chase is his again, so he cannot save until it ends. (The game no
// longer writes such a file; Josh's saves of 29 Sep may hold one.)
func TestAFileHoldingAChaseOfHimResumesInCombat(t *testing.T) {
	saved, save := b4bGame(t)
	night := b4bHunt(t, saved)

	w, _ := b4File(t, saved, save, "t.world.json")
	require.Len(t, w.Pursuit.Chases, 1)
	require.Equal(t, night.walker.ID(), w.Pursuit.Chases[0].Hunter)

	// As a file of 29 Sep: the chase of him.
	w.Pursuit.Chases[0].Quarry = d2saveref.Player

	resumed, _ := b4bGame(t)
	resumed.combatGraceSeconds = DefaultCombatGraceSeconds
	require.NoError(t, b4Load(t, resumed, w))

	code, detail := resumed.CombatReason()
	require.Equal(t, CombatChased, code, detail)
	require.Contains(t, detail, night.walker.ID())
	b3Refused(t, resumed, SaveRefusedCombat, csTo(save, "r0.world.json"))
}
