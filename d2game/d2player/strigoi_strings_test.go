package d2player

import "testing"

// TestStrigoiMonthName pins the out-of-range guard the clock strip depends on
// (item 7 / audit B5, 12 Sep 2026): a month outside [1,12] names nothing rather
// than indexing the lookup table out of range. d2player links no ebiten, so this
// runs on CI. Zeller's month arithmetic feeds this, so the guard is not academic.
//
// Negative control: drop the `month < 1 || month >= len(...)` guard and the 13
// and -1 cases index out of the 13-element table and panic. (Month 0 is the
// sentinel empty string at index 0, so it is covered by the table itself, not
// the guard -- the guard's real job is the out-of-range ends, which 13 and -1
// exercise; the June case is the positive control.)
func TestStrigoiMonthName(t *testing.T) {
	if got := strigoiMonthName(0); got != "" {
		t.Fatalf("month 0 must name nothing, got %q", got)
	}

	if got := strigoiMonthName(13); got != "" {
		t.Fatalf("month 13 must name nothing, got %q", got)
	}

	if got := strigoiMonthName(-1); got != "" {
		t.Fatalf("a negative month must name nothing, got %q", got)
	}

	// Positive control, so "" is not simply always returned.
	if got := strigoiMonthName(6); got != "June" {
		t.Fatalf("month 6 must be June, got %q", got)
	}
}
