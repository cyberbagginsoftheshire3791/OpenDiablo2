package d2loader

import "testing"

// M5.2: the census tells an MPQ load from Strigoi's own, counts a file once
// however often it loads, and groups by area.
func TestCensusRecordsBySource(t *testing.T) {
	c := NewCensus()

	c.Record(`\data\global\ui\CURSOR\ohand.DC6`, `C:\Diablo II\d2data.mpq`)
	c.Record(`/data/global/ui/cursor/ohand.dc6`, `C:\Diablo II\d2data.mpq`)
	c.Record(`/data/strigoi/creatures/wolf/idle.png`, `C:\Temp\strigoi-harness`)

	e := c.Entries()
	if len(e) != 2 {
		t.Fatalf("two files, one loaded twice: %v", e)
	}

	if e[0].Source != CensusMPQ || e[0].Loads != 2 || e[0].From != "d2data.mpq" {
		t.Errorf("the cursor came from the MPQ, twice: %+v", e[0])
	}

	if e[1].Source != CensusNative {
		t.Errorf("the wolf is ours: %+v", e[1])
	}

	for path, want := range map[string]string{
		`\data\global\ui\CURSOR\ohand.DC6`:      "data/global/ui",
		`/data/global/monsters/fa/cof/fanu.cof`: "data/global/monsters",
		`data/local/font/latin/font16.tbl`:      "data/local/font",
		`/data/strigoi/creatures/wolf/idle.png`: "data/strigoi/creatures",
		`/data/global/excel/monstats.txt`:       "data/global/excel",
		`pal.dat`:                               ".",
	} {
		if got := Area(path); got != want {
			t.Errorf("Area(%q) = %q, want %q", path, got, want)
		}
	}

	var nilCensus *Census
	nilCensus.Record("x", "y.mpq") // must not panic
}
