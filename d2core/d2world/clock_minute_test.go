package d2world

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// M4.6 BUG-88: HoursToDusk COUNTS FROM THE START OF THE CURRENT WORLD MINUTE.
// The HUD reads it on the frame the strip refreshes -- the first frame of a
// new minute in a running game, and part-way through the minute in a game
// resumed there -- so it must be one value for the whole minute, or a saved
// and a resumed game can show two texts (16.85 h, "Sunset in 16.9h", saved;
// 16.84 h, "16.8h", resumed: measured by the BUG-87 burst).
func TestHoursToDuskCountsFromTheMinutesStart(t *testing.T) {
	c := NewClock(DefaultClockDials())
	defer c.Close()

	// 02:45 to 02:54, whole minutes at DayRate 4.0: 9 world minutes.
	c.Advance(2.25)
	require.InDelta(t, 2*60+54, c.MinuteOfDay(), 1e-9)

	start := c.HoursToDusk()
	require.Equal(t, (19*60+45-(2*60+54))/60.0, start, "16.85 h at 02:54 exactly")

	// Every fifteenth of the minute (a dawn frame at 1/60 s) reads the same
	// value, and the next minute another.
	for i := 1; i < 15; i++ {
		c.Advance(1.0 / 60)
		require.Equal(t, start, c.HoursToDusk(), "%d frames into the minute (%.4f)", i, c.MinuteOfDay())
	}

	c.Advance(1.0 / 60)
	c.Advance(1e-6)
	require.InDelta(t, start-1.0/60, c.HoursToDusk(), 1e-12, "the next minute (%.4f)", c.MinuteOfDay())
}
