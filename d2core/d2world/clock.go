// Package d2world holds the simulated world's own systems — the ones that are
// neither rendering nor networking: the world clock (M4.1), and the light
// model that hangs off it. Everything here is plain arithmetic over the
// simulation delta the game screen already receives, so it is deterministic
// by construction: nothing in this package reads the wall clock, and nothing
// imports the renderer.
//
// Each system registers a d2harness.Provider at construction (P3 spec §3.5):
// a system is not done until its provider exposes every value its S1 §12
// playtest assertion needs.
package d2world

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// Stage is the part of the day the world is in. The stage decides the
// compression rate and the ambient light, and it is deliberately the only
// thing that does — D7 §7: tie the rate to a legible state and never change
// it mid-phase.
type Stage int

// The stages of a day (S1 §3.1).
const (
	StageNight Stage = iota
	StageDawn
	StageDay
	StageDusk
)

// String names the stage for provider output and logs.
func (s Stage) String() string {
	switch s {
	case StageDawn:
		return "dawn"
	case StageDay:
		return "day"
	case StageDusk:
		return "dusk"
	case StageNight:
		return "night"
	default:
		return fmt.Sprintf("stage-%d", int(s))
	}
}

// IsDark reports whether the stage is the deep night — the stage the light
// model floors out in and the one the compression dilates.
func (s Stage) IsDark() bool { return s == StageNight }

const (
	minutesPerHour = 60.0
	minutesPerDay  = 24 * minutesPerHour
	daysPerWeek    = 7
)

// ClockDials are the tunable numbers of the world clock. Every one is a
// [DIAL] the playtest answers; the defaults come from S1 §2/§3.4 as confirmed
// by D7 §6, and the stage boundaries from S1 Appendix A's computed June sky
// for ~44.93°N (sunrise ≈ 04:15, sunset ≈ 19:45 → ~15½ h of daylight;
// ~5½ h of true dark). The real solar-position algorithm and the C2/C4
// precision passes belong to M4.4 and later, not here.
type ClockDials struct {
	// DawnStart .. NightStart are minutes-of-day boundaries, [0, 1440).
	DawnStart  float64 // twilight begins; the sky starts to lift
	DayStart   float64 // sunrise
	DuskStart  float64 // sunset; the sky starts to fall
	NightStart float64 // true dark begins

	// Compression: world minutes advanced per second of simulated time.
	DayRate   float64 // dawn, day and dusk all run at this rate (D7 §7: legible states)
	NightRate float64 // the night is dilated on purpose — it carries the fear

	// MoonStart is the moon's illuminated fraction on the opening night and
	// MoonPerNight is how much it thins each night (S1 §3.2: "thinning every
	// night of the run"). VERIFY against S1 Appendix A's computed phase —
	// the astronomy pass (C2) owns the true value; these are placeholders
	// with the right shape.
	MoonStart    float64
	MoonPerNight float64
}

// DefaultClockDials returns the M4.1 starting dials.
//
// Cycle arithmetic these produce, for auditing: dawn+day+dusk = 18.5 world
// hours = 1110 world min ÷ 4 = 277.5 sim seconds ≈ 4.6 real min; night =
// 5.5 world hours = 330 world min ÷ 2.5 = 132 sim seconds ≈ 2.2 real min;
// a full cycle ≈ 6.8 real min, inside D7 §6's [7].
func DefaultClockDials() ClockDials {
	return ClockDials{
		DawnStart:    2*minutesPerHour + 45,  // 02:45
		DayStart:     4*minutesPerHour + 15,  // 04:15 sunrise
		DuskStart:    19*minutesPerHour + 45, // 19:45 sunset
		NightStart:   21*minutesPerHour + 15, // 21:15 true dark
		DayRate:      4.0,
		NightRate:    2.5,
		MoonStart:    0.55,
		MoonPerNight: 0.09,
	}
}

// Epoch is the world's opening moment: 17 June 1462 (Julian), at dawn.
// ATTESTED R1 §5 (the morning after the Night Attack); the weekday is
// Thursday, computed and pinned by the tests.
const (
	EpochYear   = 1462
	EpochMonth  = 6
	EpochDay    = 17
	epochMinute = 2*minutesPerHour + 45 // dawn — the run opens as the sky lifts
)

// Clock is the world's time base (S1 §3). It accumulates the simulation
// delta — the same delta the harness controls when stepping — into world
// minutes, and derives the Julian civil date, the weekday, the stage of the
// day and the moon from it. It never reads the wall clock.
//
// Not safe for concurrent use: like every other simulation system it lives on
// the game goroutine, and the harness marshals its calls onto it.
type Clock struct {
	dials ClockDials

	// elapsed is world minutes since the epoch. It is the whole state.
	elapsed float64

	// frozen holds the clock still (D7 §4's hearth time-freeze). Nothing
	// sets it in M4.1 except the harness: there are no safe zones until
	// there are houses.
	frozen bool
}

// NewClock returns a clock at the epoch and registers it as the harness
// "clock" provider.
func NewClock(dials ClockDials) *Clock {
	c := &Clock{dials: dials, elapsed: 0}
	d2harness.Register(c)

	return c
}

// Close unregisters the provider. The game screen calls it on unload.
func (c *Clock) Close() { d2harness.Unregister(c) }

// Advance moves the clock by one simulation tick and returns how many world
// minutes passed (zero while frozen) — the quantity every clock-driven system
// consumes.
func (c *Clock) Advance(simSeconds float64) float64 {
	if c.frozen || simSeconds <= 0 {
		return 0
	}

	delta := simSeconds * c.Rate()
	c.elapsed += delta

	return delta
}

// Rate is the current compression: world minutes per second of simulated
// time. It depends only on the stage (D7 §7).
func (c *Clock) Rate() float64 {
	if c.Stage().IsDark() {
		return c.dials.NightRate
	}

	return c.dials.DayRate
}

// Frozen(), the getter, used to sit here and had zero call sites: HarnessState
// publishes the flag from the field. Deleted 28 Aug 2026, reachability register.

// SetFrozen holds or releases the clock (D7 §4). In M4.1 only the harness
// calls it; the hearth and its safe zone arrive with the houses.
func (c *Clock) SetFrozen(frozen bool) { c.frozen = frozen }

// WorldMinutes returns world minutes elapsed since the epoch.
func (c *Clock) WorldMinutes() float64 { return c.elapsed }

// MinuteOfDay returns the minutes since local midnight, [0, 1440).
func (c *Clock) MinuteOfDay() float64 {
	m := math.Mod(epochMinute+c.elapsed, minutesPerDay)
	if m < 0 {
		m += minutesPerDay
	}

	return m
}

// DayIndex returns how many midnights have passed since the epoch — 0 on the
// opening day. The run is six of these (S1 §2).
func (c *Clock) DayIndex() int {
	return int(math.Floor((epochMinute + c.elapsed) / minutesPerDay))
}

// Stage returns the part of the day.
func (c *Clock) Stage() Stage {
	m := c.MinuteOfDay()
	d := c.dials

	switch {
	case m < d.DawnStart:
		return StageNight
	case m < d.DayStart:
		return StageDawn
	case m < d.DuskStart:
		return StageDay
	case m < d.NightStart:
		return StageDusk
	default:
		return StageNight
	}
}

// Date returns the Julian civil date (R1 §4: the calendar everyone in the
// fiction keeps).
func (c *Clock) Date() (year, month, day int) {
	return julianFromDayNumber(julianDayNumber(EpochYear, EpochMonth, EpochDay) + c.DayIndex())
}

// Weekday returns the day's name. The Saturday and the Tuesday of the run
// are load-bearing (S1 §3), so this is pinned by tests against two known
// answers rather than trusted to a formula.
func (c *Clock) Weekday() string {
	jdn := julianDayNumber(EpochYear, EpochMonth, EpochDay) + c.DayIndex()

	return weekdayNames[((jdn%daysPerWeek)+daysPerWeek)%daysPerWeek]
}

// nolint:gochecknoglobals // a constant table; Monday is JDN mod 7 == 0
var weekdayNames = [daysPerWeek]string{
	"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday",
}

// Moon returns the moon's illuminated fraction tonight, [0, 1] — the ambient
// floor of the deep night (S1 §3.2). It thins across the run, so the last
// night is darker than the first.
func (c *Clock) Moon() float64 {
	f := c.dials.MoonStart - c.dials.MoonPerNight*float64(c.DayIndex())

	return math.Max(0, math.Min(1, f))
}

// SetMoon overrides the moon's fraction. This is world state, not the
// clock's arithmetic, so unlike the time itself it is settable from a test
// (P3 §4.5 forbids setting the time; S1 §4's assertion requires a new moon).
func (c *Clock) SetMoon(fraction float64) {
	c.dials.MoonStart = math.Max(0, math.Min(1, fraction)) + c.dials.MoonPerNight*float64(c.DayIndex())
}

// Clock returns the wall-clock-free time of day as HH:MM.
func (c *Clock) TimeOfDay() string {
	m := int(c.MinuteOfDay())

	return fmt.Sprintf("%02d:%02d", m/int(minutesPerHour), m%int(minutesPerHour))
}

// ---------------------------------------------------------- Julian calendar --

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}

	return q
}

// julianDayNumber returns the Julian Day Number of a date in the JULIAN
// calendar. Pinned by tests to two known answers: 29 May 1453 (the fall of
// Constantinople) is a Tuesday, and 17 June 1462 is a Thursday (S1 App. A).
func julianDayNumber(year, month, day int) int {
	a := floorDiv(14-month, 12)
	y := year + 4800 - a
	m := month + 12*a - 3

	return day + floorDiv(153*m+2, 5) + 365*y + floorDiv(y, 4) - 32083
}

// julianFromDayNumber inverts julianDayNumber.
func julianFromDayNumber(jdn int) (year, month, day int) {
	c := jdn + 32082
	d := floorDiv(4*c+3, 1461)
	e := c - floorDiv(1461*d, 4)
	m := floorDiv(5*e+2, 153)

	day = e - floorDiv(153*m+2, 5) + 1
	month = m + 3 - 12*floorDiv(m, 10)
	year = d - 4800 + floorDiv(m, 10)

	return year, month, day
}

// ------------------------------------------------------------- the provider --

// HarnessName identifies the provider (P3 §3.5).
func (c *Clock) HarnessName() string { return "clock" }

// HarnessState exposes everything S1 §12's clock assertion needs: the date
// and weekday after N world hours, the stage, and the moon.
func (c *Clock) HarnessState() map[string]interface{} {
	year, month, day := c.Date()

	return map[string]interface{}{
		"world_minutes": c.elapsed,
		"minute_of_day": c.MinuteOfDay(),
		"time_of_day":   c.TimeOfDay(),
		"date":          fmt.Sprintf("%04d-%02d-%02d", year, month, day),
		"year":          year,
		"month":         month,
		"day":           day,
		"weekday":       c.Weekday(),
		"day_index":     c.DayIndex(),
		"stage":         c.Stage().String(),
		"rate":          c.Rate(),
		"moon":          c.Moon(),
		"frozen":        c.frozen,
	}
}

// HarnessSettableFields lists what a test may write. The time is deliberately
// absent: the clock is stepped, never set (P3 §4.5), so an assertion about a
// date proves the clock's arithmetic and not the test's.
func (c *Clock) HarnessSettableFields() []string { return []string{"frozen", "moon"} }

// HarnessSet writes one allow-listed field.
func (c *Clock) HarnessSet(field string, value interface{}) error {
	switch field {
	case "frozen":
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("frozen wants a bool, got %T", value)
		}

		c.SetFrozen(b)

		return nil
	case "moon":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("moon wants a number in [0,1], got %T", value)
		}

		c.SetMoon(f)

		return nil
	case "world_minutes", "date", "time_of_day", "day_index":
		return fmt.Errorf("the clock is stepped, never set — advance it with strigoi_step_world")
	default:
		return fmt.Errorf("no settable field %q", field)
	}
}

// toFloat accepts the numeric shapes JSON decoding produces.
func toFloat(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}
