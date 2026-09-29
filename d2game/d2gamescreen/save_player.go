package d2gamescreen

import (
	"errors"
	"fmt"
	"time"

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
//   - THE CLOSE HOOK (rule 3): the window's close button, Alt-F4, and the
//     harness's graceful quit do what SAVE AND EXIT does -- the world file,
//     his .od2 and his sidecar, in B3's order and generation, then the unload
//     that writes his .od2 and sidecar as it always did. Refused (a fight), it
//     leaves without saving and he comes back to his last save. It never
//     blocks the close: the save runs under a limit (CloseGame).
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
// EVERY REFUSAL IS "NOT NOW". Nothing here branches on a refusal's code to
// decide whether to retry: the pending autosave retries on any *SaveRefusal,
// whatever it names -- a fight, a talk, a monster's held action (BUG-87, while
// that refusal exists) -- and gives up only when the day is over. The codes
// pick the WORDS a person reads (saveRefusalWords), nothing else. So a
// refusal the save gains or loses (the held action's, on save-held) changes
// no line of the logic.

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

// SaveRefusedNow is why the game cannot be saved now, in his words, or "".
// It is the save's own refusal (saveRefusal), asked and not acted on: a read.
func (v *Game) SaveRefusedNow() string {
	if r := v.saveRefusal(); r != nil {
		return saveRefusalWords(r.Code, v.combat != nil && v.combat.Fighting())
	}

	return ""
}

// SaveFromMenu is SAVE GAME (exit false) and SAVE AND EXIT GAME (exit true).
// SAVE GAME puts "Game saved." up itself; SAVE AND EXIT leaves, and the main
// menu is the notice.
func (v *Game) SaveFromMenu(exit bool) d2player.MenuSaveResult {
	by := SaveByMenu
	if exit {
		by = SaveByExit
	}

	_, err := v.SaveWorldAs(by, SaveOptions{})
	if err == nil {
		v.Infof("SAVE from the menu (%s)", by)

		if !exit && v.gameControls != nil {
			v.gameControls.SaveNotice(d2player.MenuSavedNotice, d2player.SaveNoticeSeconds)
		}

		return d2player.MenuSaveResult{Saved: true}
	}

	words := v.saveWords(err)
	v.lastSave.Words = words
	v.Infof("SAVE from the menu (%s) not made: %v", by, err)

	return d2player.MenuSaveResult{Words: words}
}

// saveWords is what a save's error says to him: a refusal in plain words, or
// that the files could not be written.
func (v *Game) saveWords(err error) string {
	var refusal *SaveRefusal
	if !errors.As(err, &refusal) {
		return d2player.MenuSaveFailed
	}

	return saveRefusalWords(refusal.Code, v.combat != nil && v.combat.Fighting())
}

// saveRefusalWords is a refusal, by its code, in his words (rule 2: "The menu
// says so"). hisFight is whether HIS fight is running: a FIGHTING refusal
// without one is the moment after a fight -- its end not yet applied, a blow
// or a death still playing (his swing, or a monster's held action: BUG-76 and
// BUG-87) -- which a moment cures. A code this function does not know is
// "not now" in general words, never a code on the screen.
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

	// Unloaded: the screen's own unload ran after the save -- his .od2 and
	// sidecar written as SAVE AND EXIT's unload writes them, the client closed.
	Unloaded    bool   `json:"unloaded"`
	UnloadError string `json:"unload_error,omitempty"`

	// TimedOut: the hook had not finished within its limit, and the close went
	// on without it (the files are as the save left them: B3's order means a
	// save cut off between them is refused TORN at the next load and set aside,
	// never half-resumed).
	TimedOut bool `json:"timed_out,omitempty"`
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
// generation in both), then the screen's unload, which writes his .od2 and
// sidecar as it always has (a dead hero's never) and closes the client -- run
// under limit, so the close never hangs on it. A refused save (a fight) still
// unloads: he leaves without saving and comes back to his last save.
//
// The hook runs on its own goroutine while the caller -- the game goroutine,
// in ebiten's Update or the harness's queue -- waits for it and does nothing
// else; past the limit the caller stops waiting and the process leaves. The
// App calls it once and runs no frame after it (App.closeTheGame).
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

// waitForClose is the close hook's limit: the hook's report, or TimedOut.
func waitForClose(done <-chan CloseReport, limit time.Duration) CloseReport {
	timer := time.NewTimer(limit)
	defer timer.Stop()

	select {
	case rep := <-done:
		return rep
	case <-timer.C:
		return CloseReport{TimedOut: true}
	}
}

// unloadOnClose is the close hook's unload: the screen's own OnUnload. A test
// swaps it -- a unit game has no controls or client to unbind -- to see the
// hook reach it, and to make it hang.
//
// nolint:gochecknoglobals // a seam for a unit test, as setAsideWorld is
var unloadOnClose = func(v *Game) error { return v.OnUnload() }

// closeNow is the hook's work: the save, then the unload.
func (v *Game) closeNow(rep *CloseReport) {
	_, err := v.SaveWorldAs(SaveByClose, SaveOptions{})

	var refusal *SaveRefusal

	switch {
	case err == nil:
		rep.Saved = true
	case errors.As(err, &refusal):
		rep.Refused, rep.Reason = refusal.Code, refusal.Reason
		rep.Words = saveRefusalWords(refusal.Code, v.combat != nil && v.combat.Fighting())
	default:
		rep.Error = err.Error()
	}

	v.Infof("CLOSE the save: %s", *rep)

	if err := unloadOnClose(v); err != nil {
		rep.UnloadError = err.Error()
	}

	rep.Unloaded = true
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
	if v.autosave.State != AutosavePending || v.worldClock == nil {
		return
	}

	a := &v.autosave
	now := v.worldClock.WorldMinutes()

	switch a.step(v.worldClock.Stage(), now, func() error {
		_, err := v.SaveWorldAs(SaveByDawn, SaveOptions{})
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
		v.saveNotice(d2player.AutosaveFailed)
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
func loadNoticeText(r LoadReport) string {
	switch {
	case r.Refused != "":
		wake := d2player.LoadWakeAtDawn
		if r.SetAside == "" {
			wake = d2player.LoadWakeAtDawnInPlace
		}

		return loadRefusalWords(r.Refused) + "\n" + wake
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
		wordsNow = saveRefusalWords(r.Code, v.combat != nil && v.combat.Fighting())
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
