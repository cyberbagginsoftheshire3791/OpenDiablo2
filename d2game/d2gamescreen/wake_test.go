package d2gamescreen

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// BUG-114: a game that resumes no world save wakes at dawn of the day after
// the last day his journal knows. THE CONTROL is a journal with no date: the
// clock stays at the epoch, as a new hero's does.
func TestHeWakesOnTheDayAfterHisJournal(t *testing.T) {
	for _, c := range []struct {
		journal string
		want    float64
		ok      bool
	}{
		{`{"last_date": "1462-06-19"}`, 3 * 1440, true},
		{`{"last_date": "1462-06-17"}`, 1440, true},
		{`{"last_date": "1462-06-23"}`, 7 * 1440, true},
		{`{"seq": 4}`, 0, false},
		{``, 0, false},
		{`{"last_date": "the nineteenth"}`, 0, false},
		{`{"last_date": "1462-06-10"}`, 0, false},
	} {
		got, ok := wakeMinutes(json.RawMessage(c.journal))
		require.Equal(t, c.ok, ok, c.journal)
		require.Equal(t, c.want, got, c.journal)
	}

	v, _ := b4Game(t)
	v.pendingLoad = nil
	require.NoError(t, v.worldClock.Restore(d2world.ClockSnapshot{Elapsed: 0}))

	// The control: no date, no move.
	v.wakeOnTheDayAfterHisJournal(json.RawMessage(`{"seq": 4}`))
	require.Equal(t, 0.0, v.worldClock.WorldMinutes())

	v.wakeOnTheDayAfterHisJournal(json.RawMessage(`{"last_date": "1462-06-19"}`))

	y, m, d := v.worldClock.Date()
	require.Equal(t, [3]int{1462, 6, 20}, [3]int{y, m, d}, "the day after the 19th")
	require.Equal(t, d2world.StageDawn, v.worldClock.Stage(), "at dawn")
	require.Equal(t, "02:45", v.worldClock.TimeOfDay())

	// Never backwards.
	v.wakeOnTheDayAfterHisJournal(json.RawMessage(`{"last_date": "1462-06-17"}`))
	require.Equal(t, 3.0*1440, v.worldClock.WorldMinutes())

	// A game that resumes a world save keeps the save's clock.
	require.NoError(t, v.worldClock.Restore(d2world.ClockSnapshot{Elapsed: 10}))
	v.pendingLoad = &d2save.World{}
	v.wakeOnTheDayAfterHisJournal(json.RawMessage(`{"last_date": "1462-06-19"}`))
	require.Equal(t, 10.0, v.worldClock.WorldMinutes())
}
