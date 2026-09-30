package d2gamescreen

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// M4.6 B5: the save reaches the player (save_player.go). The state machine of
// the dawn autosave, the words every refusal is said in, and the close hook's
// limit -- each with no MPQs; the playtests (playtest/save_player_test.go) are
// the same things in a real game.

// b5Refusal is a refusal with a code, as SaveWorld returns one.
func b5Refusal(code string) error {
	return &SaveRefusal{Code: code, Reason: "b5: " + code}
}

// b5Saver answers from a script: each call takes the next answer, and the last
// repeats. calls counts them.
type b5Saver struct {
	answers []error
	calls   int
}

func (s *b5Saver) save() error {
	i := s.calls
	if i >= len(s.answers) {
		i = len(s.answers) - 1
	}

	s.calls++

	return s.answers[i]
}

// refusedThenSaved is n refusals (cycling the codes) and then a save.
func refusedThenSaved(n int, codes ...string) *b5Saver {
	s := &b5Saver{}
	for i := 0; i < n; i++ {
		s.answers = append(s.answers, b5Refusal(codes[i%len(codes)]))
	}

	s.answers = append(s.answers, nil)

	return s
}

// TestTheDawnAutosaveStateMachine: armed at the dawn edge; taken on the first
// frame that saves; PENDING through any number of refused frames, whatever the
// refusal's code -- including one this build has never heard of, as a held
// action's became when the save-held merge dropped it; dropped when night falls; failed
// (and not retried) on an error that is not a refusal; and taken by his own
// save while it waits.
func TestTheDawnAutosaveStateMachine(t *testing.T) {
	var a dawnAutosave

	// Nothing armed: a step asks nothing.
	never := &b5Saver{answers: []error{nil}}
	require.Equal(t, autosaveNothing, a.step(d2world.StageDawn, 0, never.save))
	require.Zero(t, never.calls)

	// Taken on the frame it was armed.
	a.arm(1)
	require.Equal(t, AutosavePending, a.State)
	require.Equal(t, 1, a.Day)

	now := &b5Saver{answers: []error{nil}}
	require.Equal(t, autosaveSaved, a.step(d2world.StageDawn, 1590, now.save))
	require.Equal(t, AutosaveTaken, a.State)
	require.Equal(t, SaveByDawn, a.By)
	require.Equal(t, 1590.0, a.TakenAt)
	require.Zero(t, a.Tries)

	// Once taken, never again that dawn.
	require.Equal(t, autosaveNothing, a.step(d2world.StageDawn, 1591, now.save))
	require.Equal(t, 1, now.calls)

	// BUG-87, measured on master: a clock fight refuses 92-93% of its frames,
	// in runs of up to 392 frames (6.53 s). A pending autosave must outlast
	// such a run -- and one of several: 1000 refused frames, four codes, one
	// of them unknown to this build, then a save.
	for _, stage := range []d2world.Stage{d2world.StageDawn, d2world.StageDay, d2world.StageDusk} {
		a.arm(2)

		s := refusedThenSaved(1000, SaveRefusedFighting, SaveRefusedTalking, "HELD", SaveRefusedJournal)

		for i := 0; i < 1000; i++ {
			require.Equal(t, autosaveWaiting, a.step(stage, float64(i), s.save), "frame %d", i)
			require.Equal(t, AutosavePending, a.State)
		}

		require.Equal(t, 1000, a.Tries)
		require.Equal(t, SaveRefusedJournal, a.LastCode)
		require.Equal(t, autosaveSavedLate, a.step(stage, 1000, s.save), "stage %v", stage)
		require.Equal(t, AutosaveTaken, a.State)
		require.Equal(t, 1001, s.calls, "tried on every frame, and on no frame after it took")
	}

	// Dropped when night falls, with nothing asked of the save that frame.
	a.arm(3)

	s := refusedThenSaved(5, SaveRefusedFighting)
	require.Equal(t, autosaveWaiting, a.step(d2world.StageDusk, 0, s.save))
	require.Equal(t, autosaveGaveUp, a.step(d2world.StageNight, 1, s.save))
	require.Equal(t, AutosaveDropped, a.State)
	require.Equal(t, 1, s.calls)
	require.Equal(t, autosaveNothing, a.step(d2world.StageDawn, 2, s.save), "a dropped autosave waits for the next dawn's arm")

	// A save that failed is not a refusal: failed, and never retried (a disk
	// that refused a write would otherwise be written at every frame).
	a.arm(4)

	broke := &b5Saver{answers: []error{errors.New("writing the world file: the disk is full")}}
	require.Equal(t, autosaveBroke, a.step(d2world.StageDay, 0, broke.save))
	require.Equal(t, AutosaveFailed, a.State)
	require.Contains(t, a.Error, "disk is full")
	require.Equal(t, autosaveNothing, a.step(d2world.StageDay, 1, broke.save))
	require.Equal(t, 1, broke.calls)

	// His own save while it waits is the day's save: taken by it.
	a.arm(5)
	require.Equal(t, autosaveWaiting, a.step(d2world.StageDawn, 0, refusedThenSaved(1, SaveRefusedFighting).save))
	require.True(t, a.cover(SaveByMenu, 12))
	require.Equal(t, AutosaveTaken, a.State)
	require.Equal(t, SaveByMenu, a.By)
	require.False(t, a.cover(SaveByMenu, 13), "nothing waits once it is taken")
}

// TestTheDawnAutosaveOnAGame: the glue on a game screen that saves -- armed at
// the dawn edge, refused by a real refusal (the fight's edge), pending with the
// refusal's code, and never armed under the harness's dial.
func TestTheDawnAutosaveOnAGame(t *testing.T) {
	v, save := b3SavableGame(t)
	require.Equal(t, d2world.StageDawn, v.worldClock.Stage(), "the premise: a new game is at dawn")

	v.wasFighting = true

	v.armDawnAutosave(1)
	require.Equal(t, AutosavePending, v.autosave.State)

	for i := 1; i <= 3; i++ {
		v.advanceAutosave()
		require.Equal(t, AutosavePending, v.autosave.State)
		require.Equal(t, i, v.autosave.Tries)
		require.Equal(t, SaveRefusedFighting, v.autosave.LastCode)
	}

	_, err := os.Stat(d2save.WorldPath(save))
	require.True(t, errors.Is(err, os.ErrNotExist), "a refused autosave writes nothing")
	require.Empty(t, v.lastSave.By, "the dawn's refusals are the autosave's record, not a save asked for")

	// The dial: off, nothing is armed, and a pending one stops.
	require.NoError(t, saveProvider{v}.HarnessSet("autosave", false))
	require.Equal(t, AutosaveOff, v.autosave.State)

	v.armDawnAutosave(2)
	require.Equal(t, AutosaveOff, v.autosave.State)

	require.NoError(t, saveProvider{v}.HarnessSet("autosave", true))

	v.armDawnAutosave(3)
	require.Equal(t, AutosavePending, v.autosave.State)
	require.Error(t, saveProvider{v}.HarnessSet("autosave", "yes"))

	// The provider says it, in the digest's process part only.
	world, process := saveProvider{v}.HarnessDigest()
	require.Empty(t, world)
	require.Equal(t, AutosavePending, process["autosave"].(map[string]interface{})["state"])
	require.Equal(t, SaveRefusedFighting, process["refused_now"])
	require.Equal(t, d2player.SaveRefusedSettleWords, process["words_now"], "the fight's edge, with no fight of his running")
}

// TestEveryRefusalIsSaidInWords: every code SaveWorld refuses with is said in
// words of its own -- never the code, never the catch-all -- and a code this
// build has never heard of is "not now" in general words. The same for every
// code a load refuses with.
func TestEveryRefusalIsSaidInWords(t *testing.T) {
	saves := map[string]string{
		SaveRefusedNetwork:  d2player.SaveRefusedNetworkWords,
		SaveRefusedNotReady: d2player.SaveRefusedNotYetWords,
		SaveRefusedDead:     d2player.SaveRefusedDeadWords,
		SaveRefusedTalking:  d2player.SaveRefusedTalkWords,
		SaveRefusedJournal:  d2player.SaveRefusedJournalWords,
		SaveRefusedLoadout:  d2player.SaveRefusedLoadoutWords,
	}

	seen := map[string]string{}

	for code, want := range saves {
		for _, hisFight := range []bool{false, true} {
			got := saveRefusalWords(code, hisFight)
			require.Equal(t, want, got, "code %s", code)
			require.NotContains(t, got, code, "a code is the log's, not his")
		}

		require.NotEqual(t, d2player.SaveRefusedOtherWords, want)

		if other, dup := seen[want]; dup {
			t.Fatalf("%s and %s say the same words", code, other)
		}

		seen[want] = code
	}

	// Rule 2's own case: his fight; and the moment after one (its end not yet
	// applied, his last swing still playing -- and, until the save-held merge,
	// a monster's blow or death), which a moment cures.
	require.Equal(t, d2player.SaveRefusedFightWords, saveRefusalWords(SaveRefusedFighting, true))
	require.Equal(t, "You can't save during a fight.", d2player.SaveRefusedFightWords)
	require.Equal(t, d2player.SaveRefusedSettleWords, saveRefusalWords(SaveRefusedFighting, false))
	require.Equal(t, d2player.SaveRefusedOtherWords, saveRefusalWords("HELD", false))
	require.Equal(t, d2player.SaveRefusedOtherWords, saveRefusalWords("", true))

	loads := []string{
		LoadRefusedVersion, LoadRefusedFile, LoadRefusedNetwork, LoadRefusedHero, LoadRefusedTorn,
		LoadRefusedSidecar, LoadRefusedMap, LoadRefusedNatives,
	}

	said := map[string]string{}

	for _, code := range loads {
		got := loadRefusalWords(code)
		require.NotEqual(t, d2player.LoadRefusedOtherWords, got, "code %s", code)
		require.NotContains(t, got, code)

		if other, dup := said[got]; dup {
			t.Fatalf("%s and %s say the same words", code, other)
		}

		said[got] = code
	}

	for _, code := range []string{LoadRefusedSeed, LoadRefusedEntity, LoadRefusedBlock, "NEW"} {
		require.Equal(t, d2player.LoadRefusedOtherWords, loadRefusalWords(code), "code %s", code)
	}
}

// TestTheLoadsNotice: what a load's report says at the start of play -- rule
// 7's refusal (why, and that he wakes at dawn with the file kept), a file left
// in place when it could not be moved, a villager gone -- and nothing for a
// load that resumed with no villager gone, or for no file at all.
func TestTheLoadsNotice(t *testing.T) {
	cases := []struct {
		name string
		r    LoadReport
		want string
	}{
		{"no file", LoadReport{}, ""},
		{"resumed", LoadReport{Found: true, Resumed: true}, ""},
		{"resumed, a label renamed (BUG-80): nothing he could see", LoadReport{Found: true, Resumed: true, Notes: []string{"renamed"}}, ""},
		{"set aside", LoadReport{Found: true, Refused: LoadRefusedVersion, SetAside: "0.od2.world.json.v99.unread"},
			d2player.LoadRefusedVersionWords + "\n" + d2player.LoadWakeAtDawn},
		{"torn down and set aside", LoadReport{Found: true, Refused: LoadRefusedMap, SetAside: "x", FellBack: true},
			d2player.LoadRefusedMapWords + "\n" + d2player.LoadWakeAtDawn},
		{"could not be moved", LoadReport{Found: true, Refused: LoadRefusedFile, Ignored: true},
			d2player.LoadRefusedFileWords + "\n" + d2player.LoadWakeAtDawnInPlace},
		{"a villager gone", LoadReport{Found: true, Resumed: true, Dropped: []string{"Warriv"}}, d2player.LoadVillagerGone},
		{"two gone", LoadReport{Found: true, Resumed: true, Dropped: []string{"Warriv", "Akara"}},
			strings.Replace(d2player.LoadVillagersGone, "%d", "2", 1)},

		// The B5 review fixes: a network game does not wake at dawn (C1); a
		// torn file whose .bak was resumed says so (A2).
		{"a network game", LoadReport{Found: true, Refused: LoadRefusedNetwork, SetAside: "x"},
			d2player.LoadRefusedNetworkWords + "\n" + d2player.LoadNetworkKept},
		{"a network game, the file left in place", LoadReport{Found: true, Refused: LoadRefusedNetwork, Ignored: true},
			d2player.LoadRefusedNetworkWords + "\n" + d2player.LoadNetworkKeptInPlace},
		{"a torn file, its .bak resumed", LoadReport{Found: true, Resumed: true, FromBak: true, SetAside: "x.torn.unread"},
			d2player.LoadRefusedTornWords + "\n" + d2player.LoadTornResumedBak},
	}

	for _, c := range cases {
		require.Equal(t, c.want, loadNoticeText(c.r), c.name)
	}

	require.Contains(t, d2player.LoadWakeAtDawn, "not deleted", "rule 7: set aside, never lost -- and he is told so")
}

// TestTheMenuAsksTheSave: the menu's two questions on a game that saves --
// nothing refused, then a talk open: the words, and SAVE GAME not made, no
// file written, the refusal the last save's record.
func TestTheMenuAsksTheSave(t *testing.T) {
	v, save := b3SavableGame(t)

	require.Empty(t, v.SaveRefusedNow(), "the control: a savable game is not refused")

	v.talk = &d2dialogue.Talk{NodeID: "greet"}

	// Why, and what leaving does (the B5 review: the note is the game's, so a
	// network game can say its own).
	note := d2player.SaveRefusedTalkWords + "\n" + d2player.MenuExitWithoutSaved
	require.Equal(t, note, v.SaveRefusedNow())

	for _, exit := range []bool{false, true} {
		res := v.SaveFromMenu(exit)
		require.False(t, res.Saved)
		require.Equal(t, note, res.Words)
		require.Equal(t, "refused", v.lastSave.Result)
		require.Equal(t, SaveRefusedTalking, v.lastSave.Code)
	}

	_, err := os.Stat(d2save.WorldPath(save))
	require.True(t, errors.Is(err, os.ErrNotExist), "a refused menu save writes nothing")
}

// TestTheCloseHookNeverBlocks: a refused close (his death: since the B5
// review a fight's edge settles and saves, TestTheCloseLetsTheMomentAfterA
// FightSettle) is not saved, writes no world file, and still unloads; a close
// whose unload hangs is left behind at the limit, and the limit cuts the
// writes (between files: d2items.CutWrites); and the limit itself returns on
// time.
func TestTheCloseHookNeverBlocks(t *testing.T) {
	v, save := b3SavableGame(t)
	v.died = true

	unloads, cuts := 0, 0
	defer func(was func(*Game) error) { unloadOnClose = was }(unloadOnClose)
	defer func(was func(time.Duration) bool) { cutWrites = was }(cutWrites)

	// The limit's cut is the process's; here it is counted, so the package's
	// later tests can still write.
	cutWrites = func(grace time.Duration) bool {
		require.Equal(t, closeGrace, grace)
		cuts++

		return true
	}

	unloadOnClose = func(*Game) error {
		unloads++
		return nil
	}

	rep := v.CloseGame(5 * time.Second)
	require.False(t, rep.Saved)
	require.Equal(t, SaveRefusedDead, rep.Refused)
	require.Equal(t, d2player.SaveRefusedDeadWords, rep.Words)
	require.True(t, rep.Unloaded, "a refused close still closes")
	require.Equal(t, 1, unloads)
	require.False(t, rep.TimedOut)
	require.Zero(t, cuts, "a close within its limit cuts nothing")

	entries, err := os.ReadDir(filepath.Dir(save))
	require.NoError(t, err)

	for _, e := range entries {
		require.NotEqual(t, filepath.Base(d2save.WorldPath(save)), e.Name(), "a refused close writes no world file")
	}

	// A panic inside the hook is a report, not a crash of the close (a game
	// of its own: each hook below may leave its goroutine behind).
	v2, _ := b3SavableGame(t)
	v2.died = true
	unloadOnClose = func(*Game) error { panic("b5") }
	rep = v2.CloseGame(5 * time.Second)
	require.Contains(t, rep.Error, "panic in the close hook")

	// An unload that never returns: the hook is left behind at the limit, and
	// the close goes on.
	v3, _ := b3SavableGame(t)
	v3.died = true
	release, finished := make(chan struct{}), make(chan struct{})

	unloadOnClose = func(*Game) error {
		<-release
		close(finished)

		return nil
	}

	start := time.Now()
	rep = v3.CloseGame(150 * time.Millisecond)
	took := time.Since(start)

	require.True(t, rep.TimedOut)
	require.Less(t, took, 3*time.Second, "the close waited %v past a 150 ms limit", took)
	require.Equal(t, 1, cuts, "at the limit the writes are cut")

	// Let the hook go before the game's cleanup, so nothing races it.
	close(release)
	<-finished

	// The limit alone: a report that comes is returned, one that does not
	// times out.
	done := make(chan CloseReport, 1)
	done <- CloseReport{Saved: true}
	require.True(t, waitForClose(done, time.Second).Saved)
	require.True(t, waitForClose(make(chan CloseReport), 20*time.Millisecond).TimedOut)
}
