package d2gamescreen

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2progress"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// THE REVIEW OF COMBAT-STATUS (1 Oct 2026; strigoi-harness-runs\wt-rev-combat),
// its fixes, each decided on the coordinator's default (Josh can overturn):
// A1 a leave in combat writes no part of him; A2 / BUG-108 a forgotten chase
// ends, and a window's close in combat asks once; B1 the close's settle holds
// the world; B2 the moment after combat has its own words; the grace floor.

// csClose runs the close hook with the unit game's seams: unload is what the
// unload writes (OnUnload's hero half: his sidecar when unloadSavesHero),
// settle one settle frame.
func csClose(t *testing.T, v *Game, settle func(*Game) error) CloseReport {
	t.Helper()

	wasUnload, wasSettle, wasCut := unloadOnClose, settleFrame, cutWrites
	defer func() { unloadOnClose, settleFrame, cutWrites = wasUnload, wasSettle, wasCut }()

	unloadOnClose = func(v *Game) error {
		if v.unloadSavesHero() { // OnUnload's hero half, as it is written
			v.saveKit()
		}

		return nil
	}

	if settle != nil {
		settleFrame = settle
	}

	cutWrites = func(time.Duration) bool { return true }

	return v.CloseGame(5 * time.Second)
}

// csSidecarXP is the experience his sidecar holds.
func csSidecarXP(t *testing.T, save string) float64 {
	t.Helper()

	side, err := os.ReadFile(d2items.SidecarPath(save))
	require.NoError(t, err)

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(side, &doc))

	var progress map[string]interface{}
	require.NoError(t, json.Unmarshal(doc["progress"], &progress), "%s", doc["progress"])

	xp, ok := progress["xp"].(float64)
	require.True(t, ok, "progress.xp: %s", doc["progress"])

	return xp
}

// csSavedAt10 is a game saved with 10 experience, then 500 earned since.
func csSavedAt10(t *testing.T) (*Game, string, string) {
	t.Helper()

	v, save := csGame(t)
	if v.progress == nil {
		v.progress = &d2progress.Progress{}
	}

	v.progress.XP = 10

	_, err := v.SaveWorld(SaveOptions{})
	require.NoError(t, err)

	g1 := b5rSavedAt(t, save)
	require.Equal(t, 10.0, csSidecarXP(t, save))

	v.progress.XP = 500

	return v, save, g1
}

// A1: A LEAVE IN COMBAT WRITES NO PART OF HIM. The reviewer's
// TestReviewCloseInCombatKeepsTheHeroHalf, asserting the opposite: a close
// while a dog chases him is refused COMBAT, and the unload writes neither his
// .od2 nor his sidecar -- his sidecar is still the last save's (10 experience,
// not the 500 since), one moment with the world file. The same for the menu's
// EXIT WITHOUT SAVING in combat. THE CONTROL: the same close out of combat
// saves, and his sidecar holds the 500.
func TestALeaveInCombatWritesNoPartOfHim(t *testing.T) {
	v, save, g1 := csSavedAt10(t)
	require.True(t, v.Pursue(csDog(t, v), v.localPlayer))

	rep := csClose(t, v, nil)
	require.False(t, rep.Saved)
	require.Equal(t, SaveRefusedCombat, rep.Refused)
	require.Equal(t, g1, b5rSavedAt(t, save), "the world file is the last save's, one moment with his sidecar")
	require.Equal(t, 10.0, csSidecarXP(t, save), "his sidecar is the last save's: no part of the combat moment")

	// The menu: EXIT WITHOUT SAVING in combat leaves through the unload too.
	v, _, _ = csSavedAt10(t)
	require.True(t, v.Pursue(csDog(t, v), v.localPlayer))
	require.False(t, v.SaveFromMenu(true).Saved, "SAVE AND EXIT in combat is refused")
	require.False(t, v.unloadSavesHero(), "EXIT WITHOUT SAVING in combat writes no part of him")

	// Out of combat, EXIT WITHOUT SAVING keeps writing the hero half as it
	// always has (B5) -- the review's hole there is reported, not changed.
	for _, id := range v.pursuit.ChasersOf(v.localPlayer.ID()) {
		v.pursuit.Release(id)
	}

	require.Less(t, csFramesOut(v, 400), 400)
	require.True(t, v.unloadSavesHero(), "out of combat the unload writes him, as before")

	// The control: out of combat the close saves, and the sidecar has the 500.
	v, save, g1 = csSavedAt10(t)
	rep = csClose(t, v, nil)
	require.True(t, rep.Saved, "%s", rep)
	require.NotEqual(t, g1, b5rSavedAt(t, save))
	require.Equal(t, 500.0, csSidecarXP(t, save))
}

// csSettle is one frame of the close's settle as Advance runs it in a unit
// game: the world, or its hold; the experience; the combat status.
func csSettle(v *Game) error {
	v.advanceWorldOrHold(csFrame)
	v.earnExperience()
	v.advanceCombatStatus(csFrame)

	return nil
}

// B1: THE CLOSE'S SETTLE HOLDS THE WORLD. The reviewer's
// TestReviewCloseInGraceRunsTheUnseenWorld, asserting the fix: a close in the
// grace with a dog beside him, aware, runs the grace's 180 frames with the
// world held ("closing") -- no world minute passes, no chase, no fight -- and
// saves. THE CONTROL, in the test: the same frames without the close's hold
// let the dog open his fight.
func TestTheClosesSettleHoldsTheWorld(t *testing.T) {
	inGraceWithADog := func(t *testing.T) *Game {
		t.Helper()

		v, _ := csGame(t)
		b3SetField(t, v.localPlayer, "isCasting", true)
		v.advanceCombatStatus(csFrame)
		b3SetField(t, v.localPlayer, "isCasting", false)

		code, _ := v.CombatReason()
		require.Equal(t, CombatGrace, code)
		require.True(t, v.Watch(csDog(t, v), v.localPlayer))

		return v
	}

	v := inGraceWithADog(t)
	before := v.worldClock.WorldMinutes()

	var heldBy string

	rep := csClose(t, v, func(v *Game) error {
		heldBy = v.WorldHeldBy()

		return csSettle(v)
	})
	require.True(t, rep.Saved, "%s", rep)
	require.InDelta(t, 180, rep.SettleFrames, 1, "the grace's remainder, settled")
	require.Equal(t, d2player.WorldHeldByClose, heldBy)
	require.Equal(t, before, v.worldClock.WorldMinutes(), "no world minute passed in the frames he did not see")
	require.Zero(t, v.pursuit.Count(), "nothing chased him in them")

	// The control: the same frames with the world running.
	v = inGraceWithADog(t)

	fight := false
	for i := 0; i < 180 && !fight; i++ {
		require.NoError(t, csSettle(v))

		code, _ := v.combatTrigger()
		fight = code == CombatFight || code == CombatChased
	}

	require.True(t, fight, "the control: unheld, the dog comes for him in those frames")
}

// B2: THE MOMENT AFTER COMBAT HAS ITS OWN WORDS. In the grace (or his swing or
// reaction), with nothing after him, the refusal says "Return to the game;
// you can save a moment after the fighting stops." -- under the menu that
// moment never ends on its own. THE CONTROL: chased, it is "You can't save in
// combat.".
func TestTheMomentAfterCombatHasItsOwnWords(t *testing.T) {
	v, _ := csGame(t)
	b3SetField(t, v.localPlayer, "isCasting", true)
	v.advanceCombatStatus(csFrame)

	r := v.saveRefusal()
	require.Equal(t, SaveRefusedCombat, r.Code)
	require.Equal(t, d2player.SaveRefusedCombatMomentWords, v.refusalWords(r), "his swing")

	b3SetField(t, v.localPlayer, "isCasting", false)

	r = v.saveRefusal()
	require.Equal(t, d2player.SaveRefusedCombatMomentWords, v.refusalWords(r), "the grace")
	require.Contains(t, v.SaveRefusedNow(), d2player.SaveRefusedCombatMomentWords)

	require.True(t, v.Pursue(csDog(t, v), v.localPlayer))

	r = v.saveRefusal()
	require.Equal(t, d2player.SaveRefusedCombatWords, v.refusalWords(r), "the control: chased")
}

// A2: A WINDOW'S CLOSE IN COMBAT ASKS ONCE. Chased, the first close asks (and
// is not taken); a second within closeAskWindow is taken; one after the
// window asks again. THE CONTROLS: in the grace -- which the close settles,
// then saves -- and out of combat, a close is taken at once.
func TestAWindowsCloseInCombatAsksOnce(t *testing.T) {
	v, _ := csGame(t)
	t0 := time.Now()

	require.False(t, v.AskBeforeClose(t0), "out of combat: taken")

	require.True(t, v.Pursue(csDog(t, v), v.localPlayer))
	v.advanceCombatStatus(csFrame)
	require.True(t, v.AskBeforeClose(t0), "chased: the first close asks")
	require.False(t, v.AskBeforeClose(t0.Add(2*time.Second)), "a second close within the window leaves")
	require.True(t, v.AskBeforeClose(t0.Add(closeAskWindow+3*time.Second)), "after the window it asks again")

	for _, id := range v.pursuit.ChasersOf(v.localPlayer.ID()) {
		v.pursuit.Release(id)
	}

	v.advanceCombatStatus(csFrame)

	code, _ := v.CombatReason()
	require.Equal(t, CombatGrace, code)
	require.False(t, v.AskBeforeClose(t0.Add(time.Minute)), "in the grace the close settles and saves: taken")
	require.True(t, v.closeAskedAt.IsZero(), "and the question is cleared")
}

// BUG-108: A FORGOTTEN CHASE ENDS, AND HE CAN SAVE. A dog that noticed him and
// gave chase loses him (he is gone far past its sight); past the notice's
// memory it gives up, the chase ends, the grace runs, and the save is made.
// THE CONTROL, in the test: while the dog still remembers him the chase
// stands and the save is refused COMBAT.
func TestAForgottenChaseEndsAndHeCanSave(t *testing.T) {
	v, save := csGame(t)
	dog := b3Dog(t, v.gameClient.MapEngine, 150, 100)
	dog.SetCreatureID("feral-dog")
	v.adoptNPCBody(dog.ID(), 20)
	require.True(t, v.Watch(dog, v.localPlayer))

	for i := 0; i < 60 && !v.pursuit.Chasing(dog.ID()); i++ {
		v.advanceWorld(csFrame)
	}

	require.True(t, v.pursuit.Chasing(dog.ID()), "aware, the dog gives chase")

	v.localPlayer.StandAt(400, 400, 0) // sixty tiles off: out of every sight
	v.advanceWorld(csFrame)
	v.advanceCombatStatus(csFrame)

	noticed, _ := v.notice.Noticed(dog.ID())
	require.True(t, noticed, "the control: lost, but still remembered")
	require.True(t, v.pursuit.Chasing(dog.ID()), "the control: the chase stands while it remembers")
	b3Refused(t, v, SaveRefusedCombat, csTo(save, "remembered.world.json"))

	frames := 0
	for ; frames < 600 && v.pursuit.Chasing(dog.ID()); frames++ {
		v.advanceWorld(csFrame)
		v.advanceCombatStatus(csFrame)
	}

	require.False(t, v.pursuit.Chasing(dog.ID()), "forgotten, the chase ends (%d frames)", frames)
	require.False(t, v.combat.Fighting(), "no fight opened")
	require.Less(t, csFramesOut(v, 400), 400, "the grace runs out")
	require.True(t, csSaves(t, v, save, "forgotten.world.json"), "and the save is made")
}

// THE GRACE'S FLOOR: a remainder below a billionth of a second ends the grace
// on the frame that leaves it (combatGraceFloor). Without the floor, frame
// slices that do not sum exactly keep him in combat a frame too long.
func TestARemainderUnderTheFloorEndsTheGrace(t *testing.T) {
	v, _ := csGame(t)
	v.combatGrace, v.combatLast = csFrame+1e-12, "his swing is playing"

	v.advanceCombatStatus(csFrame)
	require.False(t, v.InCombat(), "a 1e-12 s remainder is no grace")
	require.Zero(t, v.combatGrace)

	// The control: a remainder above the floor holds him.
	v.combatGrace, v.combatLast = csFrame+1e-6, "his swing is playing"
	v.advanceCombatStatus(csFrame)
	require.True(t, v.InCombat())
}

// THE COMBAT OUT LINE SAYS THE TIME SPENT (integrate-1oct, the combat-status
// review's C3): two seconds of his swing and a half-second grace log "2.5 s
// in combat, 0.5 s after" -- the seconds that passed, where the line had
// printed the grace's dial. THE CONTROL, in the test: a second combat of one
// second logs its own 1.5 s, so the counts start again at COMBAT in.
func TestTheCombatOutLineSaysTheTimeSpent(t *testing.T) {
	v, _ := csGame(t)

	var log bytes.Buffer

	v.Logger = d2util.NewLogger()
	v.Logger.SetColorEnabled(false)
	v.Logger.Writer = &log
	v.combatGraceSeconds = 0.5

	fight := func(frames int) int {
		b3SetField(t, v.localPlayer, "isCasting", true)

		for i := 0; i < frames; i++ {
			v.advanceCombatStatus(csFrame)
		}

		b3SetField(t, v.localPlayer, "isCasting", false)

		return csFramesOut(v, 400)
	}

	require.InDelta(t, 30, fight(120), 1, "a half-second grace")
	require.Contains(t, log.String(), "COMBAT out: 2.5 s in combat, 0.5 s after his swing")

	log.Reset()
	require.InDelta(t, 30, fight(60), 1)
	require.Contains(t, log.String(), "COMBAT out: 1.5 s in combat, 0.5 s after his swing")
	require.Zero(t, v.combatSpent)
	require.Zero(t, v.combatAfter)
}
