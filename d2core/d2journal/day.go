package d2journal

import (
	"fmt"
	"strings"
)

// RAMAZAN, BY HIS COUNT (C5 P1, P4). He keeps the days of Ramazan, not the
// village's Julian dates: the page header carries their date and his text
// his count. 1 Ramazan 866 AH is taken as 31 May 1462 (Julian), the day after
// the crescent was SEEN -- C5's observed start, an astronomical ESTIMATE; the
// tabular calendar has 30 May. No Ottoman chronicle dates it, and the journal
// never says one does (content map §5).

// ramazanFirst is 1 Ramazan 866 as a Julian day number.
var ramazanFirst = julianDayNumber(1462, 5, 31)

// ramazanDays is the month's length as counted here. 866's Ramazan is taken
// as 30 days; the slice ends on the 24th, so the choice does not show.
const ramazanDays = 30

// RamazanDay is the day of Ramazan 866 a Julian date falls on, 1..30, or 0
// outside the month.
func RamazanDay(y, m, d int) int {
	n := julianDayNumber(y, m, d) - ramazanFirst + 1
	if n < 1 || n > ramazanDays {
		return 0
	}

	return n
}

// julianDayNumber is the day number of a date in the JULIAN calendar (the
// village's, and the game clock's).
func julianDayNumber(y, m, d int) int {
	a := (14 - m) / 12
	yy := y + 4800 - a
	mm := m + 12*a - 3

	return d + (153*mm+2)/5 + 365*yy + yy/4 - 32083
}

var (
	ones = []string{"", "first", "second", "third", "fourth", "fifth", "sixth", "seventh", "eighth", "ninth",
		"tenth", "eleventh", "twelfth", "thirteenth", "fourteenth", "fifteenth", "sixteenth", "seventeenth",
		"eighteenth", "nineteenth"}
	monthNames = []string{"", "January", "February", "March", "April", "May", "June", "July", "August",
		"September", "October", "November", "December"}
)

// Ordinal says 1..30 as he would write it: "eighteenth", "twenty-first".
func Ordinal(n int) string {
	switch {
	case n <= 0 || n > 30:
		return fmt.Sprint(n)
	case n < 20:
		return ones[n]
	case n == 20:
		return "twentieth"
	case n < 30:
		return "twenty-" + ones[n-20]
	default:
		return "thirtieth"
	}
}

// clock says a minute-of-day as HH:MM. The sky line is the one place the
// journal prints a time (ruled 24 Sep); he has no clock, and the prose never
// uses one.
func clock(minute int) string {
	minute = ((minute % 1440) + 1440) % 1440
	return fmt.Sprintf("%02d:%02d", minute/60, minute%60)
}

// moonless is the stretch of true dark before the moon is up, in hours, or
// "" when the moon rises before the dark. A moonrise after midnight is on the
// clock's next day.
func moonless(d Day) string {
	if d.Moonrise < 0 {
		return fmt.Sprintf("%.1f h", float64(1440-d.DarkStart+d.DarkEnd)/60)
	}

	rise := d.Moonrise
	if rise < 720 {
		rise += 1440
	}

	if rise <= d.DarkStart {
		return ""
	}

	// A moon that rises after the dark ends leaves the whole of it moonless.
	rise = min(rise, 1440+d.DarkEnd)

	return fmt.Sprintf("%.1f h", float64(rise-d.DarkStart)/60)
}

// Render is a day's page: its title and text.
func (p DayPage) Render(d Day) (title, text string) {
	fast := d.Fast
	if w, ok := p.FastWords[fast]; ok {
		fast = w
	}

	saint := d.Saint
	if w, ok := p.SaintWords[saint]; ok {
		saint = w
	}

	rise := "none before dawn"
	if d.Moonrise >= 0 {
		rise = clock(d.Moonrise)
	}

	dark := moonless(d)
	if dark == "" {
		dark = "none"
	}

	month := ""
	if d.Month >= 1 && d.Month <= 12 {
		month = monthNames[d.Month]
	}

	fields := map[string]string{
		"weekday": d.Weekday, "day": fmt.Sprint(d.Dom), "month": month, "year": fmt.Sprint(d.Year),
		"ramazan": Ordinal(RamazanDay(d.Year, d.Month, d.Dom)),
		"feast":   d.Feast, "saint": saint, "fast": fast, "event": p.Events[d.Date],
		"sunset": clock(d.Sunset), "dark_start": clock(d.DarkStart), "dark_end": clock(d.DarkEnd),
		"moonrise": rise, "lit": fmt.Sprintf("%.0f%%", d.Lit), "moonless": dark,
	}

	fill := func(s string) string {
		return fieldPattern.ReplaceAllStringFunc(s, func(m string) string {
			return fields[m[1:len(m)-1]]
		})
	}

	var lines []string

	for _, l := range p.Lines {
		// A line whose every field is empty (no event today) is left out.
		if out := fill(l); strings.TrimSpace(out) != "" && !(strings.Contains(l, "{event}") && fields["event"] == "") {
			lines = append(lines, out)
		}
	}

	lines = append(lines, "", fill(p.Data))

	return fill(p.Title), strings.Join(lines, "\n")
}
