package d2world

import (
	"encoding/json"
	"math"
	"testing"
)

// TestJulianWeekdayAnchors pins the calendar to answers we know from outside
// the code. The fall of Constantinople is the method's validation anchor and
// the run's own week is load-bearing design: the Saturday is the only day the
// haunted grave opens and the Tuesday is the day the army goes south (S1 §3).
// If this test ever fails, every dated event in the slice has moved.
func TestJulianWeekdayAnchors(t *testing.T) {
	cases := []struct {
		y, m, d int
		want    string
	}{
		{1453, 5, 29, "Tuesday"},   // Constantinople falls — the validation anchor
		{1462, 6, 17, "Thursday"},  // the run opens (S1 §3)
		{1462, 6, 19, "Saturday"},  // the one Saturday: the exhumation day
		{1462, 6, 20, "Sunday"},    // liturgy; the priest is at the church
		{1462, 6, 22, "Tuesday"},   // Mehmed withdraws
		{1462, 6, 23, "Wednesday"}, // dawn here ends a six-night run
	}

	for _, c := range cases {
		jdn := julianDayNumber(c.y, c.m, c.d)

		got := weekdayNames[((jdn%daysPerWeek)+daysPerWeek)%daysPerWeek]
		if got != c.want {
			t.Errorf("%04d-%02d-%02d (Julian): weekday %s, want %s", c.y, c.m, c.d, got, c.want)
		}
	}
}

func TestJulianDayNumberRoundTrip(t *testing.T) {
	start := julianDayNumber(1462, 1, 1)
	for i := 0; i < 800; i++ {
		y, m, d := julianFromDayNumber(start + i)
		if got := julianDayNumber(y, m, d); got != start+i {
			t.Fatalf("round trip broke at +%d: %04d-%02d-%02d -> %d, want %d", i, y, m, d, got, start+i)
		}
	}
}

func TestFloorDiv(t *testing.T) {
	cases := []struct{ a, b, want int }{
		{7, 2, 3}, {-7, 2, -4}, {7, -2, -4}, {-7, -2, 3}, {8, 2, 4}, {-8, 2, -4}, {0, 5, 0},
	}
	for _, c := range cases {
		if got := floorDiv(c.a, c.b); got != c.want {
			t.Errorf("floorDiv(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// advanceToMinuteOfDay steps the clock in small ticks until it reaches the
// given minute of day, the way the game does. It never sets the time —
// the clock is stepped, never set (P3 §4.5).
func advanceToMinuteOfDay(t *testing.T, c *Clock, target float64) {
	t.Helper()

	for i := 0; i < 200000; i++ {
		if c.MinuteOfDay() >= target && c.MinuteOfDay() < target+5 {
			return
		}

		c.Advance(0.1)
	}

	t.Fatalf("never reached minute %v (stuck at %v)", target, c.MinuteOfDay())
}

func TestClockOpensAtDawnOn17June(t *testing.T) {
	c := NewClock(DefaultClockDials())
	defer c.Close()

	y, m, d := c.Date()
	if y != 1462 || m != 6 || d != 17 {
		t.Fatalf("opening date %04d-%02d-%02d, want 1462-06-17", y, m, d)
	}

	if c.Weekday() != "Thursday" {
		t.Fatalf("opening weekday %s, want Thursday", c.Weekday())
	}

	if c.Stage() != StageDawn {
		t.Fatalf("opening stage %s, want dawn", c.Stage())
	}

	if c.TimeOfDay() != "02:45" {
		t.Fatalf("opening time %s, want 02:45", c.TimeOfDay())
	}

	if c.DayIndex() != 0 {
		t.Fatalf("opening day index %d, want 0", c.DayIndex())
	}
}

func TestClockStagesAndRates(t *testing.T) {
	c := NewClock(DefaultClockDials())
	defer c.Close()

	d := DefaultClockDials()

	cases := []struct {
		minute float64
		stage  Stage
		rate   float64
	}{
		{d.DayStart + 60, StageDay, d.DayRate},
		{d.DuskStart + 10, StageDusk, d.DayRate},
		{d.NightStart + 10, StageNight, d.NightRate},
	}

	for _, tc := range cases {
		advanceToMinuteOfDay(t, c, tc.minute)

		if c.Stage() != tc.stage {
			t.Fatalf("at %s: stage %s, want %s", c.TimeOfDay(), c.Stage(), tc.stage)
		}

		if c.Rate() != tc.rate {
			t.Fatalf("at %s: rate %v, want %v", c.TimeOfDay(), c.Rate(), tc.rate)
		}
	}
}

// TestClockIsAPureFunctionOfItsDeltas is the determinism contract: the clock
// reads nothing but the simulation delta, so the same deltas always produce
// the same world. This is what keeps M4.1 inside the digest.
func TestClockIsAPureFunctionOfItsDeltas(t *testing.T) {
	deltas := []float64{1.0 / 60, 1.0 / 60, 0.5, 3, 17, 0.25}

	a, b := NewClock(DefaultClockDials()), NewClock(DefaultClockDials())
	defer a.Close()
	defer b.Close()

	for i := 0; i < 400; i++ {
		for _, d := range deltas {
			a.Advance(d)
			b.Advance(d)
		}
	}

	if a.WorldMinutes() != b.WorldMinutes() {
		t.Fatalf("same deltas diverged: %v vs %v", a.WorldMinutes(), b.WorldMinutes())
	}

	if a.TimeOfDay() != b.TimeOfDay() || a.Weekday() != b.Weekday() {
		t.Fatalf("derived state diverged: %s %s vs %s %s", a.TimeOfDay(), a.Weekday(), b.TimeOfDay(), b.Weekday())
	}
}

func TestClockFreezeHoldsTheWorld(t *testing.T) {
	c := NewClock(DefaultClockDials())
	defer c.Close()

	c.Advance(10)
	before := c.WorldMinutes()

	c.SetFrozen(true)

	if got := c.Advance(600); got != 0 {
		t.Fatalf("frozen clock advanced by %v, want 0", got)
	}

	if c.WorldMinutes() != before {
		t.Fatalf("frozen clock moved: %v -> %v", before, c.WorldMinutes())
	}

	c.SetFrozen(false)

	if got := c.Advance(1); got <= 0 {
		t.Fatalf("released clock advanced by %v, want > 0", got)
	}
}

func TestMoonThinsAcrossTheRun(t *testing.T) {
	c := NewClock(DefaultClockDials())
	defer c.Close()

	first := c.Moon()

	// Six nights on: the last night must be darker than the first (S1 §3.2).
	for c.DayIndex() < 5 {
		c.Advance(60)
	}

	last := c.Moon()
	if last >= first {
		t.Fatalf("moon did not thin: night 1 %v, night 6 %v", first, last)
	}

	if last < 0 {
		t.Fatalf("moon went negative: %v", last)
	}
}

func TestClockRefusesToBeSet(t *testing.T) {
	c := NewClock(DefaultClockDials())
	defer c.Close()

	for _, field := range []string{"world_minutes", "date", "time_of_day", "day_index"} {
		if err := c.HarnessSet(field, 100); err == nil {
			t.Errorf("setting %q was allowed; the clock is stepped, never set (P3 §4.5)", field)
		}
	}

	if err := c.HarnessSet("frozen", true); err != nil {
		t.Errorf("frozen must be settable: %v", err)
	}

	if err := c.HarnessSet("moon", 0.0); err != nil {
		t.Errorf("moon must be settable: %v", err)
	}

	if c.Moon() != 0 {
		t.Errorf("moon after set: %v, want 0", c.Moon())
	}

	if err := c.HarnessSet("bogus", 1); err == nil {
		t.Error("unknown field must fail")
	}
}

func TestClockHarnessStateIsEncodable(t *testing.T) {
	c := NewClock(DefaultClockDials())
	defer c.Close()

	c.Advance(500)

	state := c.HarnessState()
	if state["weekday"] == "" || state["date"] == "" {
		t.Fatalf("state missing date/weekday: %v", state)
	}

	if _, err := json.Marshal(state); err != nil {
		t.Fatalf("clock state is not JSON-encodable: %v", err)
	}

	if math.Abs(state["world_minutes"].(float64)-c.WorldMinutes()) > 1e-9 {
		t.Fatal("world_minutes disagrees with the clock")
	}
}
