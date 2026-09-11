package d2player

import "fmt"

// Strigoi-owned display strings. This is the one place the game's own
// hand-authored English lives — the strings that are NOT keys in a D2 string
// table and cannot be, because they are ours: the clock strip's month names
// and its "time to sunset" label. Hardcoded English has precedent
// (game.go:404, "Getting Zone Information"), and the localisation ratchet
// (Article V.2) has a single file to move these from when a Strigoi string
// table exists.
//
// The feast and moon-phase names the strip also shows are NOT here: they are
// GENERATED into the day table in d2core/d2world (Article II.5), so they live
// in the system that produces them rather than being retyped as UI strings.

// strigoiStripSeparator joins the clock strip's fields. ASCII on purpose:
// certain of Font16 glyph coverage, and it measured narrower than a middle dot
// (§0: 686px vs 730px for the widest line).
const strigoiStripSeparator = " - "

// strigoiMonthNames indexes the Julian calendar's months from 1 (January);
// index 0 is unused so month numbers map directly.
//
// nolint:gochecknoglobals // a constant lookup table
var strigoiMonthNames = [...]string{
	"", "January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}

// strigoiMonthName returns the English month name, or "" for a month out of
// range [1,12].
func strigoiMonthName(month int) string {
	if month < 1 || month >= len(strigoiMonthNames) {
		return ""
	}

	return strigoiMonthNames[month]
}

// strigoiSunsetLabel formats the time-to-sunset readout (S1 §3.4: world hours
// to one decimal). The clock counts to its own DuskStart, so this never shows
// a negative (see Clock.HoursToDusk).
func strigoiSunsetLabel(hoursToDusk float64) string {
	return fmt.Sprintf("Sunset in %.1fh", hoursToDusk)
}
