package d2world

import (
	"math"
	"testing"
)

// M4.4a's clock half shipped with zero CI-run tests: the only _test.go touching
// the day table was the playtest script, which CI cannot run (Article V). This
// file gives daytable.go and HoursToDusk tests that run on `go test ./...` --
// d2world links no ebiten, so they run headless. (audit B5, 12 Sep 2026.)

// TestDayTableRowsAgreeWithTheClock is the Zeller cross-check daytable.go:23
// promises but nothing performed: for every generated row, dayFor finds it and a
// NewClock stepped to that day reports the same civil date and weekday.
//
// Negative control: change any row's Weekday in daytable_gen.go and this fails.
func TestDayTableRowsAgreeWithTheClock(t *testing.T) {
	if len(sliceDayTable) == 0 {
		t.Fatal("the generated day table is empty")
	}

	epoch := julianDayNumber(EpochYear, EpochMonth, EpochDay)

	for _, row := range sliceDayTable {
		got, ok := dayFor(row.Year, row.Month, row.Day)
		if !ok {
			t.Fatalf("dayFor(%04d-%02d-%02d) is not ok, but it is a table row", row.Year, row.Month, row.Day)
		}

		if got != row {
			t.Fatalf("dayFor(%04d-%02d-%02d) returned a different row", row.Year, row.Month, row.Day)
		}

		// Step a fresh clock to this day and read what it computes.
		targetIndex := julianDayNumber(row.Year, row.Month, row.Day) - epoch

		c := NewClock(DefaultClockDials())
		for i := 0; c.DayIndex() < targetIndex && i < 100000; i++ {
			c.Advance(60)
		}

		gotIndex := c.DayIndex()
		y, m, d := c.Date()
		weekday := c.Weekday()
		c.Close()

		if gotIndex != targetIndex {
			t.Fatalf("could not step the clock to day index %d (stuck at %d)", targetIndex, gotIndex)
		}

		if y != row.Year || m != row.Month || d != row.Day {
			t.Fatalf("clock at index %d is %04d-%02d-%02d, table row is %04d-%02d-%02d",
				targetIndex, y, m, d, row.Year, row.Month, row.Day)
		}

		if weekday != row.Weekday {
			t.Fatalf("%04d-%02d-%02d: clock weekday %q, table says %q", row.Year, row.Month, row.Day, weekday, row.Weekday)
		}
	}
}

// TestDayForOutsideTheSliceIsNotOk pins the outside-the-slice branch: a date the
// run never reaches (1 Jan 1462) is not in the table, so the HUD shows the date
// and time-to-sunset but no feast or moon name.
func TestDayForOutsideTheSliceIsNotOk(t *testing.T) {
	if _, ok := dayFor(1462, 1, 1); ok {
		t.Fatal("1 Jan 1462 is outside the six-night slice; dayFor must be false")
	}

	// The negative control the generator itself passed at generation time.
	if _, ok := dayFor(EpochYear, EpochMonth, EpochDay); !ok {
		t.Fatal("the epoch day must be in the slice")
	}
}

// TestHoursToDusk pins Clock.HoursToDusk's "always in [0,24)" arithmetic at the
// three points the audit named. The rate is DayRate (4.0) from dawn through
// dusk and is sampled once per Advance, so 255 sim seconds is exactly 1020 world
// minutes -- 02:45 to 19:45 -- and the clock lands on the boundary without the
// step-world overshoot.
func TestHoursToDusk(t *testing.T) {
	c := NewClock(DefaultClockDials())
	defer c.Close()

	// Opens at the dawn epoch, 02:45: 19:45 - 02:45 = 17.0 world hours to dusk.
	if got := c.HoursToDusk(); math.Abs(got-17.0) > 0.01 {
		t.Fatalf("at 02:45 HoursToDusk = %.4f, want 17.0", got)
	}

	// Exactly to 19:45, the clock's DuskStart.
	c.Advance(255)

	if got := c.MinuteOfDay(); math.Abs(got-(19*60+45)) > 0.01 {
		t.Fatalf("stepped to minute-of-day %.4f, want 1185 (19:45)", got)
	}

	if got := c.HoursToDusk(); math.Abs(got) > 0.01 {
		t.Fatalf("at 19:45 HoursToDusk = %.4f, want 0", got)
	}

	// One world minute past dusk: time to the NEXT dusk wraps to ~23.98h and is
	// never negative. Advance 0.25 s x DayRate 4.0 = exactly one world minute.
	c.Advance(0.25)

	if got := c.HoursToDusk(); math.Abs(got-1439.0/60.0) > 0.01 {
		t.Fatalf("at 19:46 HoursToDusk = %.4f, want ~23.98", got)
	}
}
