package d2mpq

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"testing"
)

// d2dataPath is the retail archive this test reads when it is installed. The
// repository ships no Blizzard data (Article V), and the fixture archive is
// too small to race, so on a machine without the game the test skips.
func d2dataPath() string {
	if p := os.Getenv("STRIGOI_D2DATA_MPQ"); p != "" {
		return p
	}

	return `C:\Program Files (x86)\Diablo II\d2data.mpq`
}

// Files the main menu loads -- the BUG-5 crash sites among them (a font
// table that ended early, a DC6 whose runs overran its frame).
var concurrentReadNames = []string{
	`data\global\ui\FrontEnd\D2logoFireLeft.DC6`,
	`data\global\ui\FrontEnd\PopUpOKCancel.dc6`,
	`data\global\ui\Loading\loadingscreen.dc6`,
	`data\global\ui\CURSOR\ohand.DC6`,
	`data\local\FONT\LATIN\font16.dc6`,
	`data\local\FONT\LATIN\font16.tbl`,
	`data\local\FONT\LATIN\font42.dc6`,
	`data\local\FONT\LATIN\fontformal12.dc6`,
	`data\global\tiles\act1\town\floor.dt1`,
	`data\global\palette\units\pal.dat`,
}

// BUG-5 (23 Sep 2026): the archive's one file handle is shared by every
// stream, and the game reads assets from more than one goroutine (a screen's
// OnLoad runs on its own). Reads must be positional, or one goroutine's Seek
// moves another's Read and a stream decodes someone else's bytes. Sixteen
// goroutines read the main menu's files over and over; every read must match
// the file read alone.
func TestConcurrentReadsDoNotCrossStreams(t *testing.T) {
	path := d2dataPath()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no retail archive at %s (set STRIGOI_D2DATA_MPQ): %v", path, err)
	}

	archive, err := FromFile(path)
	if err != nil {
		t.Fatalf("the archive opens: %v", err)
	}

	defer func() { _ = archive.Close() }()

	want := map[string][]byte{}

	for _, name := range concurrentReadNames {
		if data, err := archive.ReadFile(name); err == nil && len(data) > 0 {
			want[name] = data
		}
	}

	if len(want) < 3 {
		t.Fatalf("only %d of the main menu's files read alone -- the name list is wrong", len(want))
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		fail []string
	)

	for g := 0; g < 16; g++ {
		wg.Add(1)

		go func(g int) {
			defer wg.Done()

			for i := 0; i < 40; i++ {
				for name, data := range want {
					got, err := archive.ReadFile(name)
					if err != nil || !bytes.Equal(got, data) {
						mu.Lock()
						fail = append(fail, fmt.Sprintf("goroutine %d, pass %d, %s: %v (%d bytes, want %d)",
							g, i, name, err, len(got), len(data)))
						mu.Unlock()

						return
					}
				}
			}
		}(g)
	}

	wg.Wait()

	if len(fail) > 0 {
		t.Fatalf("%d concurrent reads crossed streams; first: %s", len(fail), fail[0])
	}
}
