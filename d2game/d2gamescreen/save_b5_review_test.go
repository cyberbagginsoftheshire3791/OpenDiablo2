package d2gamescreen

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// THE M4.6 B5 REVIEW FIXES (29 Sep 2026): the close ends what only holds the
// screen and lets the moment after a fight settle (A1, BUG-97); a save that
// fails after its world file landed puts it back, and a torn world file whose
// .bak is his sidecar's moment is resumed from the .bak (A2, BUG-98); a file
// the load refused is never written over (B2, BUG-100); a SAVE AND EXIT
// writes his .od2 once (C2, BUG-101); and the words (C1, C4). No MPQs: a unit
// game's step 2 writes a stand-in .od2 (writeHeroSave), as the server would.

// b5rStandInOd2 is what the stand-in step 2 writes: his name and class, which
// is all a load's step 1 reads of an .od2 (CheckHeroFile).
const b5rStandInOd2 = `{"heroName":"Saver","heroType":6}`

// b5rHeroSave swaps step 2 for a stand-in that keeps the .bak and writes the
// .od2 through the same atomic write the server's does -- after fail, which
// may fail it (a read-only .od2, a file held open).
func b5rHeroSave(t *testing.T, fail func(v *Game) error) {
	t.Helper()

	was := writeHeroSave
	t.Cleanup(func() { writeHeroSave = was })

	writeHeroSave = func(v *Game) error {
		if fail != nil {
			if err := fail(v); err != nil {
				return err
			}
		}

		if err := d2items.KeepGeneration(v.gameClient.SaveFilePath); err != nil {
			return err
		}

		return d2items.WriteFileAtomic(v.gameClient.SaveFilePath, []byte(b5rStandInOd2))
	}
}

// b5rGame is a savable game whose saves write all three files.
func b5rGame(t *testing.T) (*Game, string) {
	t.Helper()

	v, save := b4Game(t)
	b5rHeroSave(t, nil)

	return v, save
}

// b5rRead is a file's bytes, as a string.
func b5rRead(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

// b5rSavedAt is the world file's saved_at, and requires his sidecar to be of
// its generation: one moment in both.
func b5rSavedAt(t *testing.T, save string) string {
	t.Helper()

	w, err := d2save.Decode([]byte(b5rRead(t, d2save.WorldPath(save))))
	require.NoError(t, err)
	require.NoError(t, w.SameMoment([]byte(b5rRead(t, d2items.SidecarPath(save)))), "the world file and his sidecar are one moment")

	return w.SavedAt
}

// b5rClose runs the close hook on v with the unload and the cut swapped out (a
// unit game has no client to unbind, and a real cut would stop every later
// test's writes), and settle as the settle's frame.
func b5rClose(t *testing.T, v *Game, settle func(v *Game) error) (CloseReport, int) {
	t.Helper()

	unloads := 0

	wasUnload, wasSettle, wasCut := unloadOnClose, settleFrame, cutWrites

	defer func() { unloadOnClose, settleFrame, cutWrites = wasUnload, wasSettle, wasCut }()

	unloadOnClose = func(*Game) error {
		unloads++
		return nil
	}

	settleFrame = func(*Game) error {
		t.Errorf("the close ran a settle frame this test did not expect")
		return errors.New("no frame was expected")
	}
	if settle != nil {
		settleFrame = settle
	}

	cutWrites = func(time.Duration) bool { return true }

	return v.CloseGame(5 * time.Second), unloads
}

// A1 (BUG-97): A CLOSE WITH A TALK OR THE JOURNAL OPEN SAVES. They hold the
// world and refuse the save, and neither is anything the world file keeps, so
// the close ends them first -- as a death does -- and then saves. Before, the
// close was refused TALKING or JOURNAL and left: an hour of play lost. THE
// CONTROLS: each open alone refuses a save made without the close.
func TestTheCloseEndsATalkAndTheJournalAndSaves(t *testing.T) {
	cases := []struct {
		name, code, ended string
		open              func(v *Game)
		gone              func(v *Game) bool
	}{
		{"a talk open", SaveRefusedTalking, "talk",
			func(v *Game) { v.talk = &d2dialogue.Talk{NodeID: "greet"} }, func(v *Game) bool { return v.talk == nil }},
		{"his journal open", SaveRefusedJournal, "journal",
			func(v *Game) { v.journalOpen = true }, func(v *Game) bool { return !v.journalOpen }},
	}

	for _, c := range cases {
		v, save := b5rGame(t)
		c.open(v)

		// The control: open, the save is refused.
		_, err := v.SaveWorld(SaveOptions{})

		var r *SaveRefusal
		require.True(t, errors.As(err, &r), c.name)
		require.Equal(t, c.code, r.Code, c.name)

		rep, unloads := b5rClose(t, v, nil)
		require.True(t, rep.Saved, "%s: the close saves: %+v", c.name, rep)
		require.Equal(t, []string{c.ended}, rep.Ended, c.name)
		require.True(t, c.gone(v), "%s: the close ended it", c.name)
		require.Zero(t, rep.SettleFrames, "%s: nothing to settle", c.name)
		require.Equal(t, 1, unloads, c.name)
		require.NotEmpty(t, b5rSavedAt(t, save), c.name)
		require.Equal(t, SaveByClose, v.lastSave.By, c.name)
	}
}

// A1 (BUG-97): THE MOMENT AFTER A FIGHT SETTLES, AND THEN THE CLOSE SAVES.
// FIGHTING with no fight of his -- the fight's end not yet applied, his last
// swing, a blow or a death still playing -- is cured by frames, so the close
// runs them until the save is no longer refused, and saves. A refusal frames
// never cure is given up at closeSettleFrames and leaves unsaved; one no
// frame is run for (his death) runs none.
func TestTheCloseLetsTheMomentAfterAFightSettle(t *testing.T) {
	v, save := b5rGame(t)
	v.wasFighting = true

	frames := 0
	rep, _ := b5rClose(t, v, func(v *Game) error {
		frames++
		if frames == 5 {
			v.wasFighting = false // the fight's end applied, on its fifth frame
		}

		return nil
	})

	require.True(t, rep.Saved, "settled, the close saves: %+v", rep)
	require.Equal(t, 5, rep.SettleFrames)
	require.NotEmpty(t, b5rSavedAt(t, save))

	// Frames that never cure it: given up at the cap, unsaved.
	v2, save2 := b5rGame(t)
	v2.wasFighting = true

	rep, unloads := b5rClose(t, v2, func(*Game) error { return nil })
	require.False(t, rep.Saved)
	require.Equal(t, SaveRefusedFighting, rep.Refused)
	require.Equal(t, closeSettleFrames, rep.SettleFrames)
	require.Equal(t, 1, unloads, "unsaved, the close still unloads")

	_, err := os.Stat(d2save.WorldPath(save2))
	require.True(t, errors.Is(err, os.ErrNotExist), "a refused close writes no world file")

	// His death: no frame is run for it, and nothing is saved.
	v3, _ := b5rGame(t)
	v3.died = true

	rep, _ = b5rClose(t, v3, nil)
	require.False(t, rep.Saved)
	require.Equal(t, SaveRefusedDead, rep.Refused)
	require.Equal(t, d2player.SaveRefusedDeadWords, rep.Words)
	require.Zero(t, rep.SettleFrames)
}

// A2 (BUG-98): A SAVE THAT FAILS AFTER ITS WORLD FILE LANDED PUTS IT BACK. His
// .od2 cannot be written (read-only, held open: step 2) -- or his sidecar
// (step 3) -- after the new world file is in place; the world file goes back
// to the last save's, so the three files are one moment again, and the words
// say his last save stands because it does. The next load resumes it.
func TestAFailedSavePutsTheWorldFileBack(t *testing.T) {
	for _, step := range []string{"his .od2", "his sidecar"} {
		v, save := b5rGame(t)
		world := d2save.WorldPath(save)

		_, err := v.SaveWorld(SaveOptions{})
		require.NoError(t, err, "the last save")

		last, lastSidecar, lastAt := b5rRead(t, world), b5rRead(t, d2items.SidecarPath(save)), b5rSavedAt(t, save)

		v.advanceWorld(30)

		switch step {
		case "his .od2":
			b5rHeroSave(t, func(*Game) error { return errors.New("Access is denied") })
		case "his sidecar":
			// A directory where the sidecar's temporary file goes: its write
			// fails, on every system.
			require.NoError(t, os.Mkdir(d2items.SidecarPath(save)+".tmp", 0o750))
		}

		res, err := v.SaveWorld(SaveOptions{})
		require.Error(t, err, step)
		require.Equal(t, world, res.PutBack, "%s: the world file was put back", step)
		require.NotContains(t, res.Written, world, step)

		require.Equal(t, last, b5rRead(t, world), "%s: the world file is the last save's again", step)
		require.Equal(t, lastSidecar, b5rRead(t, d2items.SidecarPath(save)), "%s: his sidecar is the last save's", step)
		require.Equal(t, lastAt, b5rSavedAt(t, save), step)

		words, stands := v.saveWords(res, err)
		require.True(t, stands, step)
		require.Equal(t, d2player.MenuSaveFailed, words, step)
		require.Contains(t, words, "Your last save stands", step)

		w, r := PrepareLoad(save, false)
		require.Nil(t, r, "%s: the next load is not refused (TORN before the fix)", step)
		require.Equal(t, lastAt, w.SavedAt, "%s: the next load resumes the last save", step)
	}
}

// A2 (BUG-98): A PUT-BACK THAT FAILS TOO leaves the .bak untouched and says so
// -- never "your last save stands" -- and THE NEXT LOAD RESUMES THE .bak: the
// world file is newer than his sidecar (TORN), and its .bak is his sidecar's
// moment, so the torn file is set aside as .torn.unread (kept) and the .bak
// resumed in its place, with a note. THE CONTROL: a .bak that is not his
// sidecar's moment is not resumed, and the load is refused TORN as before.
func TestATornSaveResumesTheBak(t *testing.T) {
	v, save := b5rGame(t)
	world := d2save.WorldPath(save)

	_, err := v.SaveWorld(SaveOptions{})
	require.NoError(t, err, "the last save")

	last, lastAt := b5rRead(t, world), b5rSavedAt(t, save)

	v.advanceWorld(30)

	// Step 2 fails, and a directory where the world file's temporary file
	// goes makes the put-back fail too.
	b5rHeroSave(t, func(*Game) error {
		if err := os.Mkdir(world+".tmp", 0o750); err != nil {
			return err
		}

		return errors.New("Access is denied")
	})

	res, err := v.SaveWorld(SaveOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not be put back")
	require.Contains(t, res.Written, world, "the torn world file is still written")
	require.Empty(t, res.PutBack)

	torn := b5rRead(t, world)
	require.NotEqual(t, last, torn, "the premise: the world file is the new, torn one")
	require.Equal(t, last, b5rRead(t, d2items.BakPath(world)), "the .bak is untouched: the last save")

	words, stands := v.saveWords(res, err)
	require.False(t, stands)
	require.Equal(t, d2player.MenuSaveFailedNotWhole, words)
	require.NotContains(t, words, "stands")
	require.Equal(t, d2player.MenuLeaveNotWhole, v.leaveWords(stands))

	// PeekLoad sees the .bak, and moves nothing.
	pw, pr := PeekLoad(save)
	require.Nil(t, pr)
	require.Equal(t, lastAt, pw.SavedAt)
	require.Equal(t, torn, b5rRead(t, world), "a peek moves nothing")

	// The next load: TORN, and the .bak resumed.
	w, r := PrepareLoad(save, false)
	require.Nil(t, r, "the torn file's .bak is his sidecar's moment: resumed")
	require.Equal(t, lastAt, w.SavedAt)

	rep := LastLoad()
	require.True(t, rep.FromBak)
	require.Equal(t, d2save.TornPath(world), rep.SetAside)
	require.Equal(t, torn, b5rRead(t, rep.SetAside), "the torn file is kept, set aside -- never lost")
	require.Equal(t, last, b5rRead(t, world), "the .bak is in the world file's place")
	require.NotEmpty(t, rep.Notes)

	rep.Resumed = true
	require.Equal(t, d2player.LoadRefusedTornWords+"\n"+d2player.LoadTornResumedBak, loadNoticeText(rep))

	// THE CONTROL: a .bak that is not his sidecar's moment is not resumed.
	// His sidecar of a first save, the world file of a second -- torn -- and
	// a .bak that is the second too.
	v2, save2 := b5rGame(t)
	world2 := d2save.WorldPath(save2)

	_, err = v2.SaveWorld(SaveOptions{})
	require.NoError(t, err)

	firstSidecar := b5rRead(t, d2items.SidecarPath(save2))

	v2.advanceWorld(10)

	_, err = v2.SaveWorld(SaveOptions{})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(d2items.SidecarPath(save2), []byte(firstSidecar), 0o600))
	require.NoError(t, os.WriteFile(d2items.BakPath(world2), []byte(b5rRead(t, world2)), 0o600))

	_, r = PrepareLoad(save2, false)
	require.NotNil(t, r, "a .bak that is not his sidecar's moment is not resumed")
	require.Equal(t, LoadRefusedTorn, r.Code)
	require.False(t, LastLoad().FromBak)
}

// B2 (BUG-100): A FILE THE LOAD REFUSED AND COULD NOT SET ASIDE IS NEVER WRITTEN
// OVER. Rule 7: set aside, never lost. The next save sets it aside first --
// under the load's own name for its refusal -- and never keeps it as the .bak
// (where the save after would overwrite it). Still held, the save is not made,
// and says why. THE CONTROL: with no refusal remembered, the save keeps the
// file as the .bak, as every save does.
func TestASaveSetsAsideAFileTheLoadRefused(t *testing.T) {
	v, save := b5rGame(t)
	world := d2save.WorldPath(save)

	_, err := v.SaveWorld(SaveOptions{})
	require.NoError(t, err)

	v.advanceWorld(5)

	_, err = v.SaveWorld(SaveOptions{})
	require.NoError(t, err)

	good, refused := b5rRead(t, d2items.BakPath(world)), b5rRead(t, world)

	// The load refused the world file (the map moved) and could not move it.
	ignoreFromNowOn(world, refuseLoad(LoadRefusedMap, "the map moved"))
	t.Cleanup(func() { forgetIgnored(world) })

	// Still held: the save is not made, and nothing is written.
	was := setAsideWorld
	setAsideWorld = func(string, string) (string, error) { return "", errors.New("the process cannot access the file") }

	res, err := v.SaveWorld(SaveOptions{})
	setAsideWorld = was

	require.ErrorIs(t, err, ErrRefusedFileHeld)
	require.Empty(t, res.Written)
	require.Equal(t, refused, b5rRead(t, world))
	require.Equal(t, good, b5rRead(t, d2items.BakPath(world)))

	words, stands := v.saveWords(res, err)
	require.Equal(t, d2player.MenuSaveFailedHeld, words)
	require.True(t, stands)

	// Let go: set aside first, then written; the .bak is still the good save.
	v.advanceWorld(5)

	res, err = v.SaveWorld(SaveOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, res.SetAside)
	require.Equal(t, refused, b5rRead(t, res.SetAside), "the refused file is kept, set aside")
	require.Equal(t, good, b5rRead(t, d2items.BakPath(world)), "the refused file was never kept as the .bak")
	require.Nil(t, ignoredRefusal(world), "nothing at its path is that file any more")

	// And the save after it keeps the new save as the .bak; the refused file
	// is still where it was set aside.
	aside := res.SetAside
	saved := b5rRead(t, world)

	v.advanceWorld(5)

	_, err = v.SaveWorld(SaveOptions{})
	require.NoError(t, err)
	require.Equal(t, saved, b5rRead(t, d2items.BakPath(world)))
	require.Equal(t, refused, b5rRead(t, aside))
}

// C2 (BUG-101): A SAVE AND EXIT, OR A CLOSE, THAT SAVED WRITES HIS .od2 ONCE.
// The unload that follows does not write it and his sidecar again -- the
// second write kept the first as the .od2's .bak, so the .bak was this moment
// and not the save before it (rule 5). SAVE GAME and a refused close leave the
// unload's write as it was.
func TestTheExitsSaveWritesHisOd2Once(t *testing.T) {
	v, _ := b5rGame(t)
	require.True(t, v.unloadSavesHero(), "the control: a game that has not saved writes his files on the way out")

	require.True(t, v.SaveFromMenu(false).Saved)
	require.True(t, v.unloadSavesHero(), "SAVE GAME plays on: the way out writes as ever")

	require.True(t, v.SaveFromMenu(true).Saved)
	require.False(t, v.unloadSavesHero(), "SAVE AND EXIT GAME wrote them: the unload does not again")

	v2, _ := b5rGame(t)
	rep, _ := b5rClose(t, v2, nil)
	require.True(t, rep.Saved)
	require.False(t, v2.unloadSavesHero(), "a close that saved wrote them")

	v3, _ := b5rGame(t)
	v3.died = true
	_, _ = b5rClose(t, v3, nil)
	require.False(t, v3.exitSavedHero, "a refused close saved nothing")
}

// C1 and C4: the words. A network game's menu says what leaving keeps (his
// hero and gear, not the night), and its load notice does not say he wakes at
// dawn; NOT_READY's reasons that are not a game still starting have words of
// their own; a failed save says his last save stands only when it does.
func TestTheWordsAreTrue(t *testing.T) {
	v, _ := b5rGame(t)

	require.Equal(t, d2player.MenuExitWithoutSaved, v.leaveWords(true))
	require.Equal(t, d2player.MenuLeaveNotWhole, v.leaveWords(false))

	for _, c := range []struct{ reason, want string }{
		{saveReasonNoHeroSave, d2player.SaveRefusedNoSaveWords},
		{saveReasonNoKit, d2player.SaveRefusedNoKitWords},
		{"he is not in the world yet", d2player.SaveRefusedNotYetWords},
	} {
		require.Equal(t, c.want, v.refusalWords(&SaveRefusal{Code: SaveRefusedNotReady, Reason: c.reason}), c.reason)
	}

	require.NotEqual(t, d2player.SaveRefusedNoSaveWords, d2player.SaveRefusedNotYetWords)
	require.NotEqual(t, d2player.SaveRefusedNoKitWords, d2player.SaveRefusedNotYetWords)

	// A real refusal: no hero save.
	v.gameClient.SaveFilePath = ""
	require.True(t, strings.HasPrefix(v.SaveRefusedNow(), d2player.SaveRefusedNoSaveWords+"\n"), v.SaveRefusedNow())

	for _, r := range []LoadReport{
		{Found: true, Refused: LoadRefusedNetwork, SetAside: "x"},
		{Found: true, Refused: LoadRefusedNetwork},
	} {
		got := loadNoticeText(r)
		require.NotContains(t, got, "dawn", "a LAN client joins the host's hour: %q", got)
		require.True(t, strings.HasPrefix(got, d2player.LoadRefusedNetworkWords+"\n"), got)
	}

	require.Equal(t, "Your last save could not be restored.", d2player.LoadRefusedOtherWords, "worded by no time of day")
	require.NotContains(t, d2player.MenuLeaveNetwork, "last save", "a network game has no last save to come back to")
	require.NotContains(t, d2player.AutosaveFailedNotWhole, "stands")
}
