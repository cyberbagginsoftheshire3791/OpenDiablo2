package d2gamescreen

import (
	"errors"
	"fmt"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// THE SAVE REACHES THE PLAYER (M4.6 B5, 29 Sep 2026). Josh, 25 Sep: the save
// "stops at that point then resumes at that point". B1-B4b built the file, the
// verb and the load; until this burst only the harness could make a save.
// This file gives Game.SaveWorld its callers in the game, and says what the
// save and the load did in the player's words:
//
//   - THE MENU (the build plan's rules 1 and 2): SAVE GAME saves and keeps
//     playing ("Game saved."); SAVE AND EXIT GAME saves and leaves for the
//     main menu. A refused save is said in plain words ("You can't save during
//     a fight.") and the exit entry reads EXIT WITHOUT SAVING. The menu is
//     d2player's; it asks through d2player.MenuSaver, which Game implements
//     here (SaveRefusedNow, SaveFromMenu).
//   - THE CLOSE HOOK (rule 3): the window's close button, Alt-F4, the
//     console's quit and the harness's graceful quit do what SAVE AND EXIT
//     does -- the world file, his .od2 and his sidecar, in B3's order and
//     generation, then the unload. What only holds the screen is ended first
//     -- a talk, the journal, as a death ends them -- and the moment after a
//     fight is let settle, frames run until it clears (the B5 review, A1).
//     ONLY HIS LIVE FIGHT, HIS DEATH OR A NETWORK GAME makes a close leave
//     without saving what he played (rule 3: "In a fight it leaves without
//     saving, and you come back to your last save"). A game still starting,
//     or his gear not yet chosen, has played nothing -- the world has not
//     moved since it opened -- and is left as it is. It never blocks the
//     close: the save runs under a limit (CloseGame), and every file it
//     writes is a temporary file and a rename (the B5 review, B1).
//   - THE DAWN AUTOSAVE (rule 10, ruled 27 Sep: "every dawn he lives to see"):
//     armed on the frame the night is paid (earnExperience, where
//     nightSurvived and dawnWatch settle the night), taken at the end of that
//     frame -- or, if the save refuses then, PENDING and tried again at the end
//     of every frame until one takes it, and dropped, with a log line and a
//     quiet notice, if night falls first. At most one per dawn.
//   - THE LOAD'S NOTICES (rule 7): a load refused and set aside, or resumed
//     with a villager the file lacks taken off the map, says so on the HUD at
//     the start of play.
//
// EVERY REFUSAL IS "NOT NOW" TO THE AUTOSAVE. Nothing in its machine branches
// on a refusal's code to decide whether to retry: the pending autosave
// retries on any *SaveRefusal, whatever it names -- a fight, a talk, his last
// swing still playing, whatever the save refuses in the build this is part of
// -- and gives up only when the day is over. The codes pick the WORDS a person
// reads (saveRefusalWords), and the close hook's settle (the one refusal a few
// frames cure: FIGHTING with no fight of his), nothing else. So a refusal the
// save gains or loses changes no line of the autosave's logic: the merge with
// save-held (29 Sep 2026) dropped the refusal of a monster's or villager's
// held action (BUG-87 carries it in the file at its frame), and nothing here
// changed -- the autosave is simply taken sooner after a fight.

// The game's callers of the world save (saveRecord.By, the "save" provider).
const (
	SaveByMenu    = "menu"      // SAVE GAME
	SaveByExit    = "menu_exit" // SAVE AND EXIT GAME
	SaveByClose   = "close"     // the window's close, and the harness's graceful quit
	SaveByDawn    = "dawn"      // the dawn autosave
	SaveByHarness = "harness"   // strigoi_save_game
)

// The dawn autosave's states (the "save" provider's autosave.state).
const (
	AutosaveIdle    = "idle"    // no dawn has armed one in this game
	AutosavePending = "pending" // armed and refused so far; tried every frame
	AutosaveTaken   = "taken"   // saved (by the dawn, or by his own save while it waited)
	AutosaveDropped = "dropped" // night fell before any frame would save
	AutosaveFailed  = "failed"  // a save that was not refused could not be written
	AutosaveOff     = "off"     // the harness's dial (save.autosave false): never armed
)

// dawnAutosave is rule 10's state machine. One per game screen; the world
// file does not carry it (a save made while it waits takes it -- covered --
// so it is never pending in a file, and a resumed game's dawnPaidDay, which the
// file does carry, keeps the resumed day from arming it again).
type dawnAutosave struct {
	State string // one of the Autosave* states; "" is idle

	// Day is the day whose dawn armed it (Clock.DayIndex).
	Day int

	// Tries counts the frames it was refused while pending; LastCode and
	// LastReason are the last refusal's.
	Tries      int
	LastCode   string
	LastReason string

	// By is who took it: the dawn, or a save of his (the menu) that covered
	// it while it waited. TakenAt is the world minute it was taken at.
	By      string
	TakenAt float64

	// Error is why a save that was not a refusal failed.
	Error string
}

// The step's outcomes: what the frame did with the autosave.
type autosaveEvent int

const (
	autosaveNothing   autosaveEvent = iota // not pending
	autosaveSaved                          // taken on the frame it was armed
	autosaveSavedLate                      // taken after waiting
	autosaveWaiting                        // refused this frame; still pending
	autosaveGaveUp                         // night fell first: dropped
	autosaveBroke                          // a save that was not a refusal failed
)

// arm is the dawn edge of day: a new autosave, pending, replacing whatever
// the last dawn's became.
func (a *dawnAutosave) arm(day int) {
	*a = dawnAutosave{State: AutosavePending, Day: day, TakenAt: -1}
}

// autosaveWindowOpen is "that day" (the build plan's section 7: "it saves at
// the first moment that day when saving is allowed. It does not skip the
// day"): dawn, day and dusk. Night -- true dark, NightStart -- ends it.
func autosaveWindowOpen(stage d2world.Stage) bool {
	return stage != d2world.StageNight
}

// step is one frame of a pending autosave: the day over, it is dropped;
// otherwise save is tried, and taken, or still pending on a refusal -- ANY
// refusal: the code is not read -- or failed on any other error. now is the
// world minute, for TakenAt.
func (a *dawnAutosave) step(stage d2world.Stage, now float64, save func() error) autosaveEvent {
	if a.State != AutosavePending {
		return autosaveNothing
	}

	if !autosaveWindowOpen(stage) {
		a.State = AutosaveDropped

		return autosaveGaveUp
	}

	err := save()

	var refusal *SaveRefusal

	switch {
	case err == nil:
		a.State, a.By, a.TakenAt = AutosaveTaken, SaveByDawn, now

		if a.Tries > 0 {
			return autosaveSavedLate
		}

		return autosaveSaved
	case errors.As(err, &refusal):
		a.Tries++
		a.LastCode, a.LastReason = refusal.Code, refusal.Reason

		return autosaveWaiting
	default:
		a.State, a.Error = AutosaveFailed, err.Error()

		return autosaveBroke
	}
}

// cover is a save of his, made while the dawn's waited: it is the day's save,
// and the autosave is taken by it rather than writing the same moment again a
// frame later. It reports whether there was one waiting.
func (a *dawnAutosave) cover(by string, now float64) bool {
	if a.State != AutosavePending {
		return false
	}

	a.State, a.By, a.TakenAt = AutosaveTaken, by, now

	return true
}

// saveRecord is the last save the game or the harness asked for, whatever
// came of it, and how many were written.
type saveRecord struct {
	By        string
	Result    string // "saved", "refused" or "failed"
	Code      string // a refusal's code
	Reason    string // a refusal's reason, or the failure
	Words     string // what the player was told, if anything
	At        float64
	Day       int
	WorldPath string
	SavedAt   string
	Count     int // saves written by this game screen, any caller but To
}

// SaveWorldAs is SaveWorld, for one of its callers: the menu, the close hook,
// the dawn and the harness all come through here, so a real save (not To)
// is recorded for the "save" provider and takes a dawn autosave that is
// waiting (cover). SaveWorld itself is untouched: its refusals, its order,
// its strictness are B3's.
func (v *Game) SaveWorldAs(by string, opts SaveOptions) (SaveResult, error) {
	res, err := v.SaveWorld(opts)
	if opts.To != "" {
		return res, err
	}

	now, day := 0.0, 0
	if v.worldClock != nil {
		now, day = v.worldClock.WorldMinutes(), v.worldClock.DayIndex()
	}

	rec := saveRecord{By: by, At: now, Day: day, Count: v.lastSave.Count}

	var refusal *SaveRefusal

	switch {
	case err == nil:
		rec.Result, rec.WorldPath, rec.Count = "saved", res.WorldPath, rec.Count+1
		rec.SavedAt = v.saveGeneration

		if by != SaveByDawn && v.autosave.cover(by, now) {
			v.Infof("AUTOSAVE the dawn of day %d is taken by the %s save", v.autosave.Day, by)
		}
	case errors.As(err, &refusal):
		rec.Result, rec.Code, rec.Reason = "refused", refusal.Code, refusal.Reason
	default:
		rec.Result, rec.Reason = "failed", err.Error()
	}

	// The dawn's retries are the autosave's own record (Tries); only its
	// outcome is a save anyone asked for.
	if by != SaveByDawn || rec.Result != "refused" {
		v.lastSave = rec
	}

	return res, err
}

// --- the menu (d2player.MenuSaver) ------------------------------------------

// SaveRefusedNow is the menu's note when the game cannot be saved now -- why,
// in his words, and what leaving does -- or "" when it can. It is the save's
// own refusal (saveRefusal), asked and not acted on: a read.
func (v *Game) SaveRefusedNow() string {
	if r := v.saveRefusal(); r != nil {
		return v.refusalWords(r) + "\n" + v.leaveWords(true)
	}

	return ""
}

// hisFight is whether HIS fight is running (the combat model's encounter): a
// FIGHTING refusal without one is the moment after a fight, which a moment
// cures.
func (v *Game) hisFight() bool {
	return v.combat != nil && v.combat.Fighting()
}

// leaveWords is the menu's last line when a save is not made: what EXIT
// WITHOUT SAVING does (the B5 review, C1 and A2). In a network game leaving
// keeps his hero and gear and not the night -- there is no "last save" of a
// network game to come back to. Otherwise he comes back to his last save --
// when it stands: a save that failed and could not put the world file back
// may have left it torn, and then the line says what he can do.
func (v *Game) leaveWords(lastSaveStands bool) string {
	switch {
	case v.gameClient != nil && !v.gameClient.IsSinglePlayer():
		return d2player.MenuLeaveNetwork
	case !lastSaveStands:
		return d2player.MenuLeaveNotWhole
	}

	return d2player.MenuExitWithoutSaved
}

// SaveFromMenu is SAVE GAME (exit false) and SAVE AND EXIT GAME (exit true).
// SAVE GAME puts "Game saved." up itself; SAVE AND EXIT leaves, and the main
// menu is the notice.
func (v *Game) SaveFromMenu(exit bool) d2player.MenuSaveResult {
	by := SaveByMenu
	if exit {
		by = SaveByExit
	}

	res, err := v.SaveWorldAs(by, SaveOptions{})
	if err == nil {
		v.Infof("SAVE from the menu (%s)", by)

		if exit {
			// SAVE AND EXIT's unload need not write his .od2 and sidecar
			// again: this save just did (the B5 review, C2).
			v.exitSavedHero = true
		} else if v.gameControls != nil {
			v.gameControls.SaveNotice(d2player.MenuSavedNotice, d2player.SaveNoticeSeconds)
		}

		return d2player.MenuSaveResult{Saved: true}
	}

	words, stands := v.saveWords(res, err)
	v.lastSave.Words = words
	v.Infof("SAVE from the menu (%s) not made: %v", by, err)

	return d2player.MenuSaveResult{Words: words + "\n" + v.leaveWords(stands)}
}

// saveWords is what a save's error says to him -- a refusal in plain words,
// or that the files could not be written -- and whether his last save stands.
// A failed save's words are chosen from what it left written (the B5 review,
// A2; BUG-98): "Your last save stands" only when the world file is not among
// res.Written -- never written, or put back as it was.
func (v *Game) saveWords(res SaveResult, err error) (words string, lastSaveStands bool) {
	var refusal *SaveRefusal
	if errors.As(err, &refusal) {
		return v.refusalWords(refusal), true
	}

	if worldFileLeftWritten(res) {
		return d2player.MenuSaveFailedNotWhole, false
	}

	if errors.Is(err, ErrRefusedFileHeld) {
		return d2player.MenuSaveFailedHeld, true
	}

	return d2player.MenuSaveFailed, true
}

// worldFileLeftWritten is a failed save that wrote the world file and could
// not put it back: the one failure after which his last save may not stand.
func worldFileLeftWritten(res SaveResult) bool {
	for _, p := range res.Written {
		if p != "" && p == res.WorldPath {
			return true
		}
	}

	return false
}

// refusalWords is a refusal in his words: saveRefusalWords, and NOT_READY's
// two reasons that are not a game still starting -- a game with no hero save,
// and a hero with no kit file -- in words of their own (the B5 review, C4).
func (v *Game) refusalWords(r *SaveRefusal) string {
	if r.Code == SaveRefusedNotReady {
		switch r.Reason {
		case saveReasonNoHeroSave:
			return d2player.SaveRefusedNoSaveWords
		case saveReasonNoKit:
			return d2player.SaveRefusedNoKitWords
		}
	}

	return saveRefusalWords(r.Code, v.hisFight())
}

// saveRefusalWords is a refusal, by its code, in his words (rule 2: "The menu
// says so"). hisFight is whether HIS fight is running: a FIGHTING refusal
// without one is the moment after a fight -- its end not yet applied, its
// experience or pace not yet taken, his last swing still playing -- which a
// moment cures. (A monster's or villager's held action -- a blow or a death
// still playing, BUG-76's refusal -- was one more until the merge with
// save-held, 29 Sep 2026: BUG-87 carries it in the file at its frame, and the
// save no longer refuses it. The case is still reached without it; the words
// stand.) A code this function does not know is "not now" in general words,
// never a code on the screen.
func saveRefusalWords(code string, hisFight bool) string {
	switch code {
	case SaveRefusedFighting:
		if hisFight {
			return d2player.SaveRefusedFightWords
		}

		return d2player.SaveRefusedSettleWords
	case SaveRefusedTalking:
		return d2player.SaveRefusedTalkWords
	case SaveRefusedJournal:
		return d2player.SaveRefusedJournalWords
	case SaveRefusedLoadout:
		return d2player.SaveRefusedLoadoutWords
	case SaveRefusedDead:
		return d2player.SaveRefusedDeadWords
	case SaveRefusedNetwork:
		return d2player.SaveRefusedNetworkWords
	case SaveRefusedNotReady:
		return d2player.SaveRefusedNotYetWords
	default:
		return d2player.SaveRefusedOtherWords
	}
}

// --- the close hook (rule 3) -------------------------------------------------

// CloseReport is what the close hook did.
type CloseReport struct {
	// Saved: the world file, his .od2 and his sidecar were written, one
	// moment in all three (B3's order and generation).
	Saved bool `json:"saved"`

	// Refused is the save's refusal code and Reason its reason; Words what a
	// player would have been told. A refused close still closes.
	Refused string `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Words   string `json:"words,omitempty"`

	// Error is a save that was neither made nor refused (a write that failed),
	// or a panic inside the hook.
	Error string `json:"error,omitempty"`

	// Ended is what the close ended before it saved, because it only held
	// the screen and ends safely (the B5 review, A1): "talk", "journal", and
	// "menu" (the escape menu, which pauses the world a fight's end must
	// settle in).
	Ended []string `json:"ended,omitempty"`

	// SettleFrames is how many frames the close ran for the moment after a
	// fight to settle before it saved (the B5 review, A1): 0 when nothing
	// needed it, at most closeSettleFrames; SettleMillis the wall time they
	// took (at most closeSettleBudget).
	SettleFrames int   `json:"settle_frames,omitempty"`
	SettleMillis int64 `json:"settle_ms,omitempty"`

	// Unloaded: the screen's own unload ran after the save -- his .od2 and
	// sidecar written as SAVE AND EXIT's unload writes them, the client closed.
	Unloaded    bool   `json:"unloaded"`
	UnloadError string `json:"unload_error,omitempty"`

	// TimedOut: the hook had not finished within its limit, and the close went
	// on without it. The limit falls between files (the B5 review, B1): from
	// then no write begins, and the one in flight is waited for (closeGrace);
	// every file the close writes is a temporary file and a rename, so each
	// is whole, old or new. A save cut off between its files is refused TORN
	// at the next load, which resumes the .bak -- the last save -- when it is
	// his sidecar's moment.
	TimedOut bool `json:"timed_out,omitempty"`

	// CutMidWrite: timed out, and a file's write was still running at the
	// end of the grace (a disk that hangs past it). That file is a temporary
	// one -- no write is ever made in place (d2items.WriteFileAtomic) -- and
	// the file it would replace is whole.
	CutMidWrite bool `json:"cut_mid_write,omitempty"`
}

func (r CloseReport) String() string {
	switch {
	case r.TimedOut:
		return "timed out; closed without waiting"
	case r.Saved:
		return fmt.Sprintf("saved; unloaded %v %s", r.Unloaded, r.UnloadError)
	case r.Refused != "":
		return fmt.Sprintf("not saved (%s: %s); unloaded %v %s", r.Refused, r.Reason, r.Unloaded, r.UnloadError)
	default:
		return fmt.Sprintf("not saved (%s); unloaded %v %s", r.Error, r.Unloaded, r.UnloadError)
	}
}

// CloseGame is the close hook's game half: what SAVE AND EXIT GAME does --
// the save (SaveWorld: the world file, then his .od2, then his sidecar, the
// generation in both), then the screen's unload, which closes the client
// (and writes his .od2 and sidecar only when the save did not: the B5
// review, C2) -- run under limit, so the close never hangs on it. First a
// talk and the journal are ended and the moment after a fight is let settle
// (closeNow). A save refused for a reason no moment cures (his fight, his
// death, a network game) still unloads: he leaves without saving and comes
// back to his last save.
//
// The hook runs on its own goroutine while the caller -- the game goroutine,
// in ebiten's Update or the harness's queue -- waits for it and does nothing
// else; past the limit the caller stops waiting, lets the write in flight
// finish, and the process leaves. The App calls it once and runs no frame
// after it (App.closeTheGame).
//
// EVERY FILE IS WRITTEN WHOLE OR NOT AT ALL (the B5 review, B1; BUG-99): no
// write is ever made in place (d2items.WriteFileAtomic), so a process that
// leaves at any instant leaves no half file; and at the limit no write
// begins and the one in flight is let finish (waitForClose), so the close
// gives up between files, never inside one.
func (v *Game) CloseGame(limit time.Duration) CloseReport {
	done := make(chan CloseReport, 1)

	go func() {
		rep := CloseReport{}

		defer func() {
			if p := recover(); p != nil {
				rep.Error = fmt.Sprintf("panic in the close hook: %v", p)
			}

			done <- rep
		}()

		v.closeNow(&rep)
	}()

	return waitForClose(done, limit)
}

// closeGrace is how long the close waits, past its limit, for a file whose
// write has begun (the B5 review, B1): longer than one write's retries
// (d2items.RenameRetrying waits two seconds in all for a refused rename), so a
// write that has begun ends -- written, or refused whole -- before the process
// does. A write still running after it is a disk that hangs.
const closeGrace = 3 * time.Second

// waitForClose is the close hook's limit: the hook's report, or TimedOut. At
// the limit the writes are cut (d2items.CutWrites): no file begins from then,
// and the one in flight is waited for, up to closeGrace, so the limit falls
// between files.
func waitForClose(done <-chan CloseReport, limit time.Duration) CloseReport {
	timer := time.NewTimer(limit)
	defer timer.Stop()

	select {
	case rep := <-done:
		return rep
	case <-timer.C:
		finished := cutWrites(closeGrace)

		return CloseReport{TimedOut: true, CutMidWrite: !finished}
	}
}

// cutWrites is d2items.CutWrites; a unit test swaps it to see the limit reach
// it without cutting the package's other tests' writes.
//
// nolint:gochecknoglobals // a seam for a unit test, as unloadOnClose is
var cutWrites = d2items.CutWrites

// unloadOnClose is the close hook's unload: the screen's own OnUnload. A test
// swaps it -- a unit game has no controls or client to unbind -- to see the
// hook reach it, and to make it hang.
//
// nolint:gochecknoglobals // a seam for a unit test, as setAsideWorld is
var unloadOnClose = func(v *Game) error { return v.OnUnload() }

// The close's settle (the B5 review, A1): the moment after a fight -- FIGHTING
// with no fight of his: its end not yet applied, its experience or pace not
// yet taken, his last swing still playing out -- and the close runs frames
// until it clears, then saves. Those are a frame or two. The eight seconds
// were sized for a monster's held action (BUG-76's refusal: BUG-87 measured a
// clock fight's refusal at up to 6.53 s at the shipped round), which the merge
// with save-held (29 Sep 2026) dropped -- the file carries a held action at
// its frame. The bound is KEPT: his own swing still refuses, and a settle
// that ends early costs nothing, while one cut short leaves unsaved. The
// settle runs up to eight seconds of game time -- and never more than
// closeSettleBudget of the close's wall time (closeLimit is ten), so the save
// and the unload always have time left.
const (
	closeFrameSeconds = 1.0 / 60
	closeSettleFrames = 8 * 60
	closeSettleBudget = 4 * time.Second
)

// settleFrame is one frame of the close's settle: the screen's own Advance,
// run on the hook's goroutine while the game goroutine waits for it. A unit
// test swaps it -- a unit game has no client to advance.
//
// nolint:gochecknoglobals // a seam for a unit test, as unloadOnClose is
var settleFrame = func(v *Game) error { return v.Advance(closeFrameSeconds) }

// closeNow is the hook's work: what only holds the screen ended, the moment
// after a fight settled, the save, then the unload (the B5 review, A1; BUG-97).
//
//   - A TALK AND THE JOURNAL END, as a death ends them (noticeDeath): they
//     hold the world and refuse the save (TALKING, JOURNAL), and neither is
//     anything the world file keeps. Before, a close with the journal open
//     left without saving and nothing said so.
//   - THE MOMENT AFTER A FIGHT SETTLES (settleForClose): frames run until the
//     save is no longer refused for it.
//   - ONLY HIS LIVE FIGHT, HIS DEATH OR A NETWORK GAME LEAVES UNSAVED what he
//     played: his own fight (rule 3: "In a fight it leaves without saving,
//     and you come back to your last save"), his death (a dead hero is never
//     saved), a network game (rule 9). A game not ready to be saved (still
//     starting) or a loadout not yet chosen is refused too, and loses
//     nothing: the world has been held, or not yet built, since the game
//     opened. (Until the merge with save-held, 29 Sep 2026, one more: a
//     village's fight whose held actions, BUG-87, outlast the settle's eight
//     seconds. The save no longer refuses a held action.)
func (v *Game) closeNow(rep *CloseReport) {
	v.closing = true

	if v.talk != nil {
		rep.Ended = append(rep.Ended, "talk")
	}

	if v.journalOpen {
		rep.Ended = append(rep.Ended, "journal")
	}

	v.EndTalk()
	v.journalOpen = false

	v.settleForClose(rep)

	res, err := v.SaveWorldAs(SaveByClose, SaveOptions{})

	var refusal *SaveRefusal

	switch {
	case err == nil:
		rep.Saved = true

		// The unload need not write his .od2 and sidecar again: this save
		// just did (the B5 review, C2).
		v.exitSavedHero = true
	case errors.As(err, &refusal):
		rep.Refused, rep.Reason = refusal.Code, refusal.Reason
		rep.Words = v.refusalWords(refusal)
	default:
		rep.Error = err.Error()
		rep.Words, _ = v.saveWords(res, err)
	}

	v.Infof("CLOSE the save: %s", *rep)

	if err := unloadOnClose(v); err != nil {
		rep.UnloadError = err.Error()
	}

	rep.Unloaded = true
}

// settleForClose runs frames while the save is refused FIGHTING with no fight
// of his, up to closeSettleFrames and closeSettleBudget, and records how many
// in rep. It stops at once on any other refusal -- his own fight among them,
// which a frame may open -- and on none. The escape menu, which pauses the
// world a fight's end must settle in, is put away first.
func (v *Game) settleForClose(rep *CloseReport) {
	began := time.Now()

	defer func() { rep.SettleMillis = time.Since(began).Milliseconds() }()

	for rep.SettleFrames < closeSettleFrames && time.Since(began) < closeSettleBudget {
		r := v.saveRefusal()
		if r == nil || r.Code != SaveRefusedFighting || v.hisFight() {
			return
		}

		if rep.SettleFrames == 0 && v.escapeMenu != nil && v.escapeMenu.IsOpen() {
			v.escapeMenu.Dismiss()
			rep.Ended = append(rep.Ended, "menu")
		}

		if err := settleFrame(v); err != nil {
			v.Errorf("CLOSE a frame of the settle failed: %v", err)
			return
		}

		rep.SettleFrames++
	}
}

// --- the dawn autosave (rule 10) ---------------------------------------------

// armDawnAutosave is the dawn edge (earnExperience, on the frame nightSurvived
// pays and dawnWatch settles the night). A network game is never saved (rule
// 9), so none is armed there; nor under the harness's dial.
func (v *Game) armDawnAutosave(day int) {
	switch {
	case v.autosaveOff:
		v.autosave = dawnAutosave{State: AutosaveOff, Day: day, TakenAt: -1}
		return
	case v.gameClient == nil || !v.gameClient.IsSinglePlayer():
		return
	}

	v.autosave.arm(day)
	v.Infof("AUTOSAVE armed: the dawn of day %d", day)
}

// advanceAutosave is the end of every frame (Advance): a pending autosave is
// tried -- at the end of the frame the dawn armed it, after everything that
// frame did (the night's experience, the watch, the journal), and at the end
// of every frame after while it is refused.
func (v *Game) advanceAutosave() {
	// The close's settle frames save nothing of their own: the close's save
	// comes after them, and covers a waiting autosave (the B5 review, A1).
	if v.autosave.State != AutosavePending || v.worldClock == nil || v.closing {
		return
	}

	a := &v.autosave
	now := v.worldClock.WorldMinutes()

	var res SaveResult

	switch a.step(v.worldClock.Stage(), now, func() error {
		r, err := v.SaveWorldAs(SaveByDawn, SaveOptions{})
		res = r

		return err
	}) {
	case autosaveSaved:
		v.Infof("AUTOSAVE the dawn of day %d: saved", a.Day)
		v.saveNotice(d2player.AutosaveTaken)
	case autosaveSavedLate:
		v.Infof("AUTOSAVE the dawn of day %d: saved after %d refused frames (the last %s: %s)",
			a.Day, a.Tries, a.LastCode, a.LastReason)
		v.saveNotice(d2player.AutosaveTakenLate)
	case autosaveWaiting:
		// The first refusal, and every sixtieth after (a second of frames),
		// so a long wait is visible in the log without a line a frame.
		if a.Tries == 1 || a.Tries%60 == 0 {
			v.Infof("AUTOSAVE the dawn of day %d: pending (%d refused frames; %s: %s)", a.Day, a.Tries, a.LastCode, a.LastReason)
		}
	case autosaveGaveUp:
		v.Warningf("AUTOSAVE the dawn of day %d: dropped -- night fell after %d refused frames (the last %s: %s); the last save stands",
			a.Day, a.Tries, a.LastCode, a.LastReason)
		v.saveNotice(d2player.AutosaveDropped)
	case autosaveBroke:
		v.Errorf("AUTOSAVE the dawn of day %d: the save failed: %s", a.Day, a.Error)

		// Chosen from what the failed save left written (the B5 review, A2).
		if worldFileLeftWritten(res) {
			v.saveNotice(d2player.AutosaveFailedNotWhole)
		} else {
			v.saveNotice(d2player.AutosaveFailed)
		}
	case autosaveNothing:
	}
}

// saveNotice puts a save's line on the HUD, when the controls are up.
func (v *Game) saveNotice(text string) {
	if v.gameControls != nil {
		v.gameControls.SaveNotice(text, d2player.SaveNoticeSeconds)
	}
}

// --- the load's notices (rule 7) --------------------------------------------

// noticeTheLoad is the start of play (bindGameControls, once the controls
// are bound): what the load that opened this game did, if a player should
// hear it. A game whose own load is about to be torn down says nothing -- the
// dawn that replaces it keeps the report, and says it.
func (v *Game) noticeTheLoad() {
	if v.loadFailed != nil || v.loadAbandoned {
		return
	}

	text := loadNoticeText(LastLoad())
	if text == "" {
		return
	}

	v.loadNotice = text
	v.Infof("LOAD notice: %q", text)

	if v.gameControls != nil {
		v.gameControls.SaveNotice(text, d2player.LoadNoticeSeconds)
	}
}

// loadNoticeText is what a load's report says to him, or "": a refusal in
// plain words and that he wakes at dawn with the file kept (rule 7: "You are
// told why, it is set aside, and you begin at dawn"); or a resume that took a
// villager the file lacks off the map (BUG-79's note). Its other notes (a
// label this build words otherwise, BUG-80) change nothing he could see.
//
// A NETWORK GAME'S refusal does not say he wakes at dawn (the B5 review, C1): a
// LAN client joins the host's world at the host's hour. It says what comes with
// him and that the saved night is kept.
//
// A TORN FILE WHOSE .bak WAS RESUMED (the B5 review, A2) says so: the save
// that was cut off is kept, set aside, and the save before it is where he is.
func loadNoticeText(r LoadReport) string {
	switch {
	case r.Refused == LoadRefusedNetwork:
		kept := d2player.LoadNetworkKept
		if r.SetAside == "" {
			kept = d2player.LoadNetworkKeptInPlace
		}

		return loadRefusalWords(r.Refused) + "\n" + kept
	case r.Refused != "":
		wake := d2player.LoadWakeAtDawn
		if r.SetAside == "" {
			wake = d2player.LoadWakeAtDawnInPlace
		}

		return loadRefusalWords(r.Refused) + "\n" + wake
	case r.Resumed && r.FromBak:
		return d2player.LoadRefusedTornWords + "\n" + d2player.LoadTornResumedBak
	case r.Resumed && len(r.Dropped) == 1:
		return d2player.LoadVillagerGone
	case r.Resumed && len(r.Dropped) > 1:
		return fmt.Sprintf(d2player.LoadVillagersGone, len(r.Dropped))
	}

	return ""
}

// loadRefusalWords is a load's refusal, by its code, in his words.
func loadRefusalWords(code string) string {
	switch code {
	case LoadRefusedVersion:
		return d2player.LoadRefusedVersionWords
	case LoadRefusedFile:
		return d2player.LoadRefusedFileWords
	case LoadRefusedNetwork:
		return d2player.LoadRefusedNetworkWords
	case LoadRefusedHero:
		return d2player.LoadRefusedHeroWords
	case LoadRefusedTorn:
		return d2player.LoadRefusedTornWords
	case LoadRefusedSidecar:
		return d2player.LoadRefusedSidecarWords
	case LoadRefusedMap:
		return d2player.LoadRefusedMapWords
	case LoadRefusedNatives:
		return d2player.LoadRefusedNativesWords
	default:
		return d2player.LoadRefusedOtherWords
	}
}

// --- the "save" harness provider ---------------------------------------------

// saveProvider reports the save as the player meets it: the dawn autosave's
// state, the last save asked for and what came of it, the notice the load
// gave, and whether a save would be refused now. EVERYTHING in it is this
// process's history, not the world's: a game resumed from a save has taken no
// autosave and made no save, where the game that saved has (HarnessDigest).
type saveProvider struct{ v *Game }

func (p saveProvider) HarnessName() string { return "save" }

func (p saveProvider) HarnessState() map[string]interface{} {
	v := p.v
	a := v.autosave

	state := a.State
	if state == "" {
		state = AutosaveIdle
	}

	refusedNow, reasonNow, wordsNow := "", "", ""
	if r := v.saveRefusal(); r != nil {
		refusedNow, reasonNow = r.Code, r.Reason
		wordsNow = v.refusalWords(r)
	}

	ls := v.lastSave

	return map[string]interface{}{
		"autosave": map[string]interface{}{
			"state": state, "day": a.Day, "tries": a.Tries,
			"last_refusal": a.LastCode, "last_reason": a.LastReason,
			"by": a.By, "taken_at": a.TakenAt, "error": a.Error,
			"enabled": !v.autosaveOff,
		},
		"last_save": map[string]interface{}{
			"by": ls.By, "result": ls.Result, "code": ls.Code, "reason": ls.Reason, "words": ls.Words,
			"at": ls.At, "day": ls.Day, "world_path": ls.WorldPath, "saved_at": ls.SavedAt,
		},
		"saves":         ls.Count,
		"load_notice":   v.loadNotice,
		"refused_now":   refusedNow,
		"reason_now":    reasonNow,
		"words_now":     wordsNow,
		"autosave_dial": !v.autosaveOff,
	}
}

// HarnessDigest: none of it is the world's (see the type). The process part
// is the autosave's state, the load's notice and whether a save would be
// refused now; the RECORD of saves asked for -- last_save and saves -- is in
// neither part: it is a log of the calls a script made, like the harness's
// own call list, and a save that "moves nothing" (TestSaveResume act 7a
// compares every part before and after one) must not move the digest by
// being recorded. (Measured: with last_save in the process part, 7a went
// red on [process], the first full suite of B5.)
func (p saveProvider) HarnessDigest() (world, process map[string]interface{}) {
	process = p.HarnessState()

	delete(process, "last_save")
	delete(process, "saves")

	return map[string]interface{}{}, process
}

// HarnessSettableFields: one dial. autosave false keeps the dawn from saving
// by itself, for a script whose subject is a save it makes (TestSaveResume
// steps past dawn after T and must load T); the shipped game has no way to set
// it.
func (p saveProvider) HarnessSettableFields() []string { return []string{"autosave"} }

func (p saveProvider) HarnessSet(field string, value interface{}) error {
	if field != "autosave" {
		return fmt.Errorf("save has no settable field %q", field)
	}

	on, ok := value.(bool)
	if !ok {
		return fmt.Errorf("autosave wants a bool, got %T", value)
	}

	p.v.autosaveOff = !on

	if !on && p.v.autosave.State == AutosavePending {
		p.v.autosave.State = AutosaveOff
	}

	return nil
}
