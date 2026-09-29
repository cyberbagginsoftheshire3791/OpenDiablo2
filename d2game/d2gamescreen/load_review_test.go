package d2gamescreen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
)

// THE B4a REVIEW'S FIXES (29 Sep 2026), IN A UNIT GAME. Each test names the
// finding it closes; each was seen red with its fix taken out (the controls
// are in docs/m4.6-world-save-notes.md, "B4a review fixes").

// dawnNavigator records the App's two ways back: the dawn after a refusal and
// "load last save".
type dawnNavigator struct {
	menuNavigator
	dawns   []int64
	reloads []string
}

func (n *dawnNavigator) FallBackToDawn(_ string, seed int64, _ string) {
	n.dawns = append(n.dawns, seed)
}
func (n *dawnNavigator) ReloadGame(filePath string) { n.reloads = append(n.reloads, filePath) }

// b4Later is his sidecar as act 3 of TestSaveResume leaves it: of the world
// file's generation (every kit save of the resumed game carries it) and of a
// later moment -- more experience than the file's copy holds.
func b4Later(t *testing.T, w *d2save.World) []byte {
	t.Helper()

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Sidecar, &doc))

	doc["progress"] = map[string]interface{}{"xp": 105}

	out, err := json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)

	return out
}

// b4Prepared is a hero whose world file is good and whose sidecar is later
// than it (b4Later), after step 1: the world file's copy written over his, his
// own kept. It returns the save, his sidecar before step 1, and the file.
func b4Prepared(t *testing.T, good *d2save.World) (string, []byte, *d2save.World) {
	t.Helper()

	s := b4Files(t, good, "Saver", b4Amazon)
	before := b4Later(t, good)
	require.NoError(t, os.WriteFile(d2items.SidecarPath(s), before, 0o600))

	w, r := PrepareLoad(s, false)
	require.Nil(t, r)
	require.NotNil(t, w)

	written, err := os.ReadFile(d2items.SidecarPath(s))
	require.NoError(t, err)
	require.NotEqual(t, string(before), string(written), "step 1 wrote the world file's copy over his")

	kept, err := os.ReadFile(preloadPath(s))
	require.NoError(t, err)
	require.Equal(t, string(before), string(kept), "step 1 kept his own first, byte for byte")

	return s, before, w
}

// A1 (BUG-60): A LOAD REFUSED AFTER STEP 1 PUTS HIS OWN SIDECAR BACK, byte for
// byte, whichever refusal it is -- the seed, the map, the villagers, the clock
// (step 2), a block (step 4), the checks on the first frame (step 5) -- and
// the dawn he falls back to is his own. Before the fix every one of them left
// the world file's copy: his experience and standing of the saved moment, and
// everything since gone.
func TestARefusalAfterStepOnePutsHisSidecarBack(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")
	unread := fmt.Sprintf(".v%d.unread", d2save.Version)

	// The App's path: CreateGame's refusals (steps 2 and 4), which
	// App.ToCreateGame hands to SetLoadAside and then the dawn.
	opened := map[string]struct {
		code   string
		break_ func(w *d2save.World)
	}{
		"the seed (step 4)": {LoadRefusedSeed, func(w *d2save.World) { w.Seed++ }},
		"the map (step 4, D5)": {LoadRefusedMap, func(w *d2save.World) {
			w.Map = d2save.Map{Path: "village.tmj", SHA: "0000000000000000000000000000000000000000000000000000000000000000"}
		}},
		"the villagers (step 4)": {LoadRefusedNatives, func(w *d2save.World) {
			w.Entities = append(w.Entities, d2save.Entity{ID: "0ak", Kind: d2save.KindNPC, Monstat: "Akara",
				Native: true, NameKey: "Akara", Born: &[2]float64{10, 10}, X: 10, Y: 10,
				Motion: d2mapentity.Motion{Pos: [2]float64{50, 50}}})
		}},
		"the clock (step 2)": {LoadRefusedBlock, func(w *d2save.World) { w.Clock.Elapsed = -1 }},
		"a block (step 4)":   {LoadRefusedBlock, func(w *d2save.World) { w.Light.NextID = 1 }},
	}

	for name, c := range opened {
		s, before, w := b4Prepared(t, good)
		c.break_(w)

		v, _ := b4Game(t)

		r := v.restoreClock(w)
		if r == nil {
			r = v.checkLoad(w)
		}

		require.NotNil(t, r, name)
		require.Equal(t, c.code, r.Code, "%s: %v", name, r)

		SetLoadAside(s, r, true)

		now, err := os.ReadFile(d2items.SidecarPath(s))
		require.NoError(t, err)
		require.Equal(t, string(before), string(now), "%s: his own sidecar is back, byte for byte", name)

		_, err = os.Stat(preloadPath(s))
		require.True(t, os.IsNotExist(err), "%s: and the copy is gone", name)
		require.Equal(t, "restored", LastLoad().Preload, name)

		_, err = os.Stat(d2save.WorldPath(s) + unread)
		require.NoError(t, err, "%s: the file is set aside", name)
	}

	// The game's own path: a refusal on the first frame (checkBound) tears
	// the game down through abandonLoad.
	s, before, w := b4Prepared(t, good)

	v, _ := b4Game(t)
	nav := &dawnNavigator{}
	v.navigator, v.gameClient.SaveFilePath = nav, s

	require.Nil(t, v.restoreClock(w))
	require.Nil(t, v.checkLoad(w))

	v.pendingLoad = w
	v.kit.SpendOffHand() // a carried torch, and none in his off-hand

	v.loadFailed = v.resumeLoad(w)
	require.Error(t, v.loadFailed)

	v.failPendingLoad()
	require.True(t, v.loadAbandoned)
	require.Len(t, nav.dawns, 1, "the dawn opens in its place")

	now, err := os.ReadFile(d2items.SidecarPath(s))
	require.NoError(t, err)
	require.Equal(t, string(before), string(now), "the first frame's refusal: his own sidecar is back, byte for byte")

	// And a game that never opened at all (the App's RestorePreload: the
	// client not made, the host not reached).
	s, before, _ = b4Prepared(t, good)
	RestorePreload(s)

	now, err = os.ReadFile(d2items.SidecarPath(s))
	require.NoError(t, err)
	require.Equal(t, string(before), string(now), "a game that never opened: his own sidecar is back")

	_, err = os.Stat(preloadPath(s))
	require.True(t, os.IsNotExist(err))
}

// A1: A FAILED WRITE AT STEP 1 PUTS HIS OWN BACK TOO (SIDECAR). The write can
// fail half done (WriteFileAtomic falls back to writing in place while
// Windows holds the target); what was his is restored from the copy.
func TestAFailedStepOneWritePutsHisSidecarBack(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")
	s := b4Files(t, good, "Saver", b4Amazon)
	before := b4Later(t, good)
	require.NoError(t, os.WriteFile(d2items.SidecarPath(s), before, 0o600))

	was := writeStepOne
	writeStepOne = func(path string, data []byte) error {
		_ = os.WriteFile(path, data[:len(data)/2], 0o600) // torn

		return errors.New("the disk refused the rest")
	}

	defer func() { writeStepOne = was }()

	w, r := PrepareLoad(s, false)
	require.Nil(t, w)
	require.Equal(t, LoadRefusedSidecar, r.Code)

	now, err := os.ReadFile(d2items.SidecarPath(s))
	require.NoError(t, err)
	require.Equal(t, string(before), string(now), "his own sidecar is back, not the torn half")
}

// A1: A LOAD A CRASH CUT OFF IS UNDONE AT THE NEXT START. Step 1 wrote the
// world file's copy and the game never resumed or refused; the next start
// puts his own back before anything reads it -- so a refusal after that start
// still restores HIS sidecar, not the world file's copy step 1 left.
func TestACrashedLoadIsUndoneAtTheNextStart(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	// The crash: step 1 done, nothing after it.
	s, before, _ := b4Prepared(t, good)

	// The next start takes the file again ...
	w, r := PrepareLoad(s, false)
	require.Nil(t, r)
	require.NotNil(t, w)
	require.Equal(t, "recovered", LastLoad().Preload)

	kept, err := os.ReadFile(preloadPath(s))
	require.NoError(t, err)
	require.Equal(t, string(before), string(kept), "the copy is still his own, not the world file's")

	// ... and is refused after it opened: his own comes back.
	SetLoadAside(s, refuseLoad(LoadRefusedMap, "the map moved"), true)

	now, err := os.ReadFile(d2items.SidecarPath(s))
	require.NoError(t, err)
	require.Equal(t, string(before), string(now))

	// A copy a later world save has overtaken is discarded: the file beside
	// it is of another moment than the copy's generation.
	s, _, _ = b4Prepared(t, good)
	newer := d2save.WorldPath(s)
	data, err := os.ReadFile(newer)
	require.NoError(t, err)

	later, err := d2save.Decode(data)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(later.Sidecar, &doc))

	later.SavedAt, doc["generation"] = "2026-09-29T23:00:00Z", "2026-09-29T23:00:00Z"
	later.Sidecar, err = json.Marshal(doc)
	require.NoError(t, err)

	written, err := d2save.Encode(later, nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(newer, written, 0o600))

	require.Equal(t, "discarded", recoverPreload(s))

	_, err = os.Stat(preloadPath(s))
	require.True(t, os.IsNotExist(err))

	// A copy beside a sidecar that moved on after step 1 (a resumed game's
	// saves) is never put over it: it is kept aside for a person.
	s, _, _ = b4Prepared(t, good)
	moved := []byte(`{"version": 1, "generation": "` + good.SavedAt + `", "kit": {}, "moved": true}`)
	require.NoError(t, os.WriteFile(d2items.SidecarPath(s), moved, 0o600))

	require.Equal(t, "kept", recoverPreload(s))

	now, err = os.ReadFile(d2items.SidecarPath(s))
	require.NoError(t, err)
	require.Equal(t, string(moved), string(now), "a sidecar that moved on is not overwritten")

	_, err = os.Stat(preloadPath(s) + ".kept")
	require.NoError(t, err, "the copy is kept aside")
}

// B1 (BUG-62): A FILE REFUSED AFTER ITS GAME OPENED THAT CANNOT BE SET ASIDE
// IS NOT READ AGAIN IN THIS PROCESS -- the dawn that replaces the game does
// not find it, refuse it and reload for ever (the review held the file open:
// 51 teardowns in 25 s). A file changed since is another file, and is read.
func TestAFileThatCannotBeSetAsideIsNotReadAgain(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")
	s, before, _ := b4Prepared(t, good)

	was := setAsideWorld
	setAsideWorld = func(string) (string, error) { return "", errors.New("the process cannot access the file") }

	defer func() { setAsideWorld = was }()

	SetLoadAside(s, refuseLoad(LoadRefusedMap, "the map moved"), true)

	_, err := os.Stat(d2save.WorldPath(s))
	require.NoError(t, err, "the file could not be moved")

	// The dawn's load: the file is ignored, his sidecar is his.
	w, r := PrepareLoad(s, false)
	require.Nil(t, w, "the dawn does not resume the file it just refused")
	require.NotNil(t, r)
	require.Equal(t, LoadRefusedMap, r.Code)

	rep := LastLoad()
	require.True(t, rep.Ignored && rep.FellBack, "the report is the refusal the dawn replaced: %+v", rep)

	now, err := os.ReadFile(d2items.SidecarPath(s))
	require.NoError(t, err)
	require.Equal(t, string(before), string(now))

	_, err = os.Stat(preloadPath(s))
	require.True(t, os.IsNotExist(err), "nothing kept: step 1 never ran")

	// Nor does PeekLoad (the harness's seed check) read it.
	w, r = PeekLoad(s)
	require.Nil(t, w)
	require.NotNil(t, r)

	// The file written again (a save, a copy put back): read again.
	later := time.Now().Add(time.Minute)
	require.NoError(t, os.Chtimes(d2save.WorldPath(s), later, later))

	w, r = PrepareLoad(s, false)
	require.Nil(t, r)
	require.NotNil(t, w, "a file changed since is another file")
}

// A2 (BUG-61): ONLY A GAME THAT RESUMED A WORLD FILE CARRIES ITS GENERATION.
// A game that began without it -- a network game, the dawn after a refusal, a
// file hidden or held -- writes none, so the file reads TORN at the next load
// once his sidecar has moved on, instead of pairing with it and writing its
// older copy over everything that game earned (the review: a LAN evening's
// 125 experience back to 55).
func TestOnlyAResumedGameCarriesTheWorldFilesGeneration(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	for _, resumed := range []bool{false, true} {
		s := b4Files(t, good, "Saver", b4Amazon)

		v, _ := b4Game(t)
		v.items = b4Catalog(t)
		v.gameClient.SaveFilePath = s

		if resumed {
			v.pendingLoad = good
		}

		v.bindKit()
		require.NotNil(t, v.kit)

		v.saveKit()

		data, err := os.ReadFile(d2items.SidecarPath(s))
		require.NoError(t, err)

		if resumed {
			require.NoError(t, good.SameMoment(data), "a resumed game's kit saves carry the file's generation (B7)")

			continue
		}

		require.Error(t, good.SameMoment(data), "a game that did not resume the file does not pair with it")
	}
}

// C4: "LOAD LAST SAVE" OUT OF A NETWORK GAME RESUMES NO WORLD FILE. The App
// reopens him as a local game, so the file beside his save is refused under
// rule 9 and set aside first; a local game's reload leaves it for the load.
func TestLoadLastSaveOutOfANetworkGameSetsTheWorldFileAside(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	for _, conn := range []d2clientconnectiontype.ClientConnectionType{
		d2clientconnectiontype.LANServer, d2clientconnectiontype.LANClient, d2clientconnectiontype.Local,
	} {
		s := b4Files(t, good, "Saver", b4Amazon)
		sidecar, err := os.ReadFile(d2items.SidecarPath(s))
		require.NoError(t, err)

		client, err := d2client.Create(conn, b3Asset(t), d2util.LogLevelNone, nil)
		require.NoError(t, err)

		client.SaveFilePath = s
		nav := &dawnNavigator{}
		v := &Game{gameClient: client, navigator: nav}

		v.LoadLastSave()
		require.Equal(t, []string{s}, nav.reloads, "the reload is asked for")

		_, err = os.Stat(d2save.WorldPath(s))

		if conn == d2clientconnectiontype.Local {
			require.NoError(t, err, "a local game's reload leaves the file for its load")

			continue
		}

		require.True(t, os.IsNotExist(err), "a network game's reload sets the file aside (rule 9)")

		now, _ := os.ReadFile(d2items.SidecarPath(s))
		require.Equal(t, sidecar, now, "his sidecar untouched")
		require.Equal(t, LoadRefusedNetwork, LastLoad().Refused)
	}
}

// C3: THE DAWN THAT REPLACES A LOAD REFUSED ON ITS FIRST FRAME RUNS ON THE
// FILE'S SEED, not the one the refused game ran on (they differ exactly when
// the refusal is SEED; here the game's is moved after step 4 to tell them
// apart).
func TestTheDawnAfterATornDownLoadRunsOnTheFilesSeed(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")
	s, _, w := b4Prepared(t, good)

	v, _ := b4Game(t)
	nav := &dawnNavigator{}
	v.navigator, v.gameClient.SaveFilePath = nav, s

	require.Nil(t, v.restoreClock(w))
	require.Nil(t, v.checkLoad(w))

	v.pendingLoad = w
	v.gameClient.Seed = w.Seed + 1
	v.abandonLoad(refuseLoad(LoadRefusedBlock, "a restore disagreed with its own check"))

	require.Equal(t, []int64{w.Seed}, nav.dawns, "the dawn runs on the file's seed")
}

// C2: PEEKLOAD IS STEP 1'S CHECKS AND NOTHING ELSE. start_game asks it before
// refusing a seed: a file the load would refuse anyway is no reason to refuse
// the start. It writes nothing and sets nothing aside.
func TestPeekLoadChecksAndWritesNothing(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	s := b4Files(t, good, "Saver", b4Amazon)
	sidecar, _ := os.ReadFile(d2items.SidecarPath(s))

	w, r := PeekLoad(s)
	require.Nil(t, r)
	require.NotNil(t, w)
	require.Equal(t, good.Seed, w.Seed)

	other := b4Files(t, good, "Otherman", b4Amazon)

	w, r = PeekLoad(other)
	require.Nil(t, w)
	require.Equal(t, LoadRefusedHero, r.Code)

	for _, path := range []string{s, other} {
		_, err := os.Stat(d2save.WorldPath(path))
		require.NoError(t, err, "nothing set aside")

		_, err = os.Stat(preloadPath(path))
		require.True(t, os.IsNotExist(err), "nothing kept")
	}

	now, _ := os.ReadFile(d2items.SidecarPath(s))
	require.Equal(t, sidecar, now, "nothing written")
}

// A1: A LOAD THAT RESUMED LETS THE COPY GO. The world file's moment is his
// now: the .preload is removed, and nothing is held to be "put back" over
// what the resumed game goes on to write.
func TestAResumedLoadLetsTheCopyGo(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")
	s, _, _ := b4Prepared(t, good)

	v, _ := b4Game(t)
	v.gameClient.SaveFilePath = s
	v.loadResumed()

	_, err := os.Stat(preloadPath(s))
	require.True(t, os.IsNotExist(err), "the copy is removed")

	restored, err := restorePreload(s)
	require.NoError(t, err)
	require.False(t, restored, "and nothing is held to put back")
}

// A2: A WORLD FILE THAT CANNOT BE READ IS SET ASIDE, as every refusal's is
// (before, "a file that cannot be read cannot be moved either", and it
// stayed). A directory where the file should be is a file no
// read can take, on every system.
func TestAnUnreadableWorldFileIsSetAside(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")
	s := b4Files(t, good, "Saver", b4Amazon)
	world := d2save.WorldPath(s)

	require.NoError(t, os.Remove(world))
	require.NoError(t, os.Mkdir(world, 0o750))

	w, r := PrepareLoad(s, false)
	require.Nil(t, w)
	require.Equal(t, LoadRefusedFile, r.Code)

	_, err := os.Stat(world)
	require.True(t, os.IsNotExist(err), "set aside")

	info, err := os.Stat(world + ".unread")
	require.NoError(t, err)
	require.True(t, info.IsDir(), "the thing that was there, moved whole")
	require.Equal(t, world+".unread", LastLoad().SetAside)
}
