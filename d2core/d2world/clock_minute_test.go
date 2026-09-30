package d2world

import (
	"fmt"
	"math"
	"strconv"
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

// THE BUG-87 REVIEW'S C6 (BUG-94): WHAT BUG-88'S FLOOR DID TO THE READOUT'S
// TEXT, over a whole day, sampled as the HUD samples it -- on the first frame
// of each new world minute (int(MinuteOfDay) changed), at the 1/60 s tick. The
// old reading counted from that frame's exact moment, a fraction of a minute
// in; the new one from the minute's start. The number moves by under a
// sixtieth of an hour; the TEXT ("%.1f") moves by exactly one tenth on the
// minutes where the two round apart, and on no other -- and the dusk minute,
// 19:45, reads "24.0" in both (BUG-94: the floor made it "0.0").
func TestTheReadoutsTextOverADay(t *testing.T) {
	c := NewClock(DefaultClockDials())
	defer c.Close()

	old := func(minuteOfDay float64) float64 {
		d := math.Mod(c.dials.DuskStart-minuteOfDay, minutesPerDay)
		if d < 0 {
			d += minutesPerDay
		}

		return d / minutesPerHour
	}

	last, minutes, moved := int(c.MinuteOfDay()), 0, 0

	for minutes < minutesPerDay {
		c.Advance(1.0 / 60)

		m := c.MinuteOfDay()
		if int(m) == last {
			continue
		}

		last = int(m)
		minutes++

		now := c.HoursToDusk()
		require.True(t, now > 0 && now <= 24, "%.4f: %v in (0, 24]", m, now)

		was, is := fmt.Sprintf("%.1f", old(m)), fmt.Sprintf("%.1f", now)
		if int(m) == int(c.dials.DuskStart) {
			require.Equal(t, "24.0", is, "the dusk minute reads a day away (%.4f)", m)
			require.Equal(t, was, is, "as it read before the floor (%.4f)", m)
		}

		if was != is {
			moved++

			a, _ := strconv.ParseFloat(was, 64)
			b, _ := strconv.ParseFloat(is, 64)
			require.InDelta(t, 0.1, math.Abs(a-b), 1e-9, "%.4f: %q to %q is one tenth", m, was, is)
		}
	}

	require.Positive(t, moved)
	t.Logf("over a day's %d minutes, the readout's text moved by one tenth on %d (x.x5 h rounds up from the minute's start); 19:45 reads \"24.0\"", minutes, moved)
}
