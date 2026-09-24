package d2world

// DayEntry is one date of the Strigoi day table: the historical Orthodox
// calendar and computed sky for that day of the slice. The rows are GENERATED
// from the sky tooling in docs/sky (Article II.5: derived facts live in the
// systems that produce them) and live in daytable_gen.go -- do not hand-edit
// them; edit the generator and re-run it. The clock keys into this table with
// its own computed Julian date, so the HUD reads the feast name and moon-phase
// name for "today" without recomputing either.
//
// Feast is the name the HUD shows (C3 s3.6: the movable feast when there is
// one -- "9th Thursday after Easter" on 17 Jun, "2nd Sunday after Pentecost"
// on 20 Jun -- otherwise "Apostles' Fast", which the whole slice sits inside).
// FeastDetail and FastRule are the fixed commemoration and the Typikon fast
// rule: grounding carried from C3, not shown on the strip. MoonPhase is the
// phase NAME the HUD shows (S1 s3.4 downgrade from a glyph until there is art).
// The minute fields are minute-of-day in Targoviste local mean time, kept for
// provenance and later light/scheduling work; the HUD does not read them, and
// the clock's own DuskStart -- not SunsetMinute -- drives time-to-sunset.
type DayEntry struct {
	Year, Month, Day int
	Weekday          string // Zeller cross-check of Clock.Weekday()
	Feast            string // the displayed feast/fast name (C3 s3.6)
	FeastDetail      string // the fixed commemoration (grounding, not displayed)
	FastRule         string // the Typikon fast rule (grounding, not displayed)
	MoonPhase        string // the displayed moon phase name (Meeus)
	SunriseMinute    int    // LMT minute-of-day (Meeus); grounding
	SunsetMinute     int    // LMT minute-of-day (Meeus); grounding
	MoonriseMinute   int    // LMT minute-of-day (PyEphem); -1 if none before sunrise
	MoonLitPercent   float64
}

// dayFor returns the generated table row for a Julian civil date, and false
// when the date is outside the six-night slice (17-23 June 1462). Outside the
// slice the HUD shows the date and time-to-sunset but no feast or moon name.
func dayFor(year, month, day int) (DayEntry, bool) {
	for _, e := range sliceDayTable {
		if e.Year == year && e.Month == month && e.Day == day {
			return e, true
		}
	}

	return DayEntry{}, false
}

// SliceDays is every row of the day table, in date order: the journal's day
// pages read the whole slice, not only today (J1).
func SliceDays() []DayEntry { return append([]DayEntry{}, sliceDayTable...) }

// Today returns the day-table row for the clock's current Julian date. The
// second result is false outside the slice; the HUD then omits the feast and
// moon text rather than guessing.
func (c *Clock) Today() (DayEntry, bool) {
	return dayFor(c.Date())
}
