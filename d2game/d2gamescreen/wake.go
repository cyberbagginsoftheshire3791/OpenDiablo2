package d2gamescreen

import (
	"encoding/json"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// BUG-114 (the J1 review's B6; M4.6 B6 review, Josh's default (a), 2 Oct
// 2026): A GAME THAT DOES NOT RESUME A WORLD SAVE WAKES AT DAWN OF THE DAY
// AFTER THE LAST DAY HIS SIDECAR KNOWS -- not on 17 June, the clock's epoch.
//
// Every game that begins without the world file begins at dawn: rule 7's fall
// back from a refused file, rule 9's network game, a hero saved before the
// world save existed. Each started a new clock at the epoch, so a hero whose
// journal held the 19th's page woke on the 17th (measured in act 9: "fallen
// back to dawn, the clock reads 1462-06-17 02:45; his journal's pages are
// [1462-06-17 1462-06-18 1462-06-19]").
//
// The last day his sidecar knows is his journal's last_date -- the newest
// slice day he has seen, which the journal already saves (no change to the
// sidecar's shape). He wakes at 02:45 of the day after it: the night he was
// in is lost, as rule 10's autosave promises at most one night is, and the
// journal stays true. The clock never goes backwards, and a hero with no
// journal (a new one) wakes on the epoch as before.
//
// It runs in bindKit, before bindProgress samples the clock for lastStage and
// dawnPaidDay, so the dawn he wakes on is not paid as a night survived; and on
// the first frame, before any system has advanced.
func (v *Game) wakeOnTheDayAfterHisJournal(journal json.RawMessage) {
	if v.worldClock == nil || v.pendingLoad != nil {
		return
	}

	elapsed, ok := wakeMinutes(journal)
	if !ok || elapsed <= v.worldClock.WorldMinutes() {
		return
	}

	if err := v.worldClock.Restore(d2world.ClockSnapshot{Elapsed: elapsed}); err != nil {
		v.Errorf("waking on the day after his journal: %v", err)

		return
	}

	y, m, d := v.worldClock.Date()
	v.Infof("WAKE no world save resumed: he wakes at dawn of %04d-%02d-%02d, the day after the last his journal knows", y, m, d)
}

// wakeMinutes is the clock's elapsed minutes at 02:45 of the day after the
// journal block's last_date (each day's dawn is a whole number of days past
// the epoch's), and false when the block names no date this calendar reads.
func wakeMinutes(journal json.RawMessage) (float64, bool) {
	if len(journal) == 0 {
		return 0, false
	}

	var j struct {
		LastDate string `json:"last_date"`
	}

	if err := json.Unmarshal(journal, &j); err != nil || j.LastDate == "" {
		return 0, false
	}

	last, err := time.Parse("2006-01-02", j.LastDate)
	if err != nil {
		return 0, false
	}

	// A difference in days: the proleptic calendar's count between two June
	// 1462 dates is the Julian one's.
	epoch := time.Date(d2world.EpochYear, time.Month(d2world.EpochMonth), d2world.EpochDay, 0, 0, 0, 0, time.UTC)

	days := int(last.Sub(epoch).Hours()/24) + 1
	if days < 1 {
		return 0, false
	}

	return float64(days) * minutesPerDay, true
}

// minutesPerDay is a day of world minutes.
const minutesPerDay = 24 * 60
