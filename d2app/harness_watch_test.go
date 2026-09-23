//go:build harness

package d2app

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stallFixture points the stack dumps at a fresh directory, forgets any
// earlier dump, and captures the log.
func stallFixture(t *testing.T) (dir string, logged *bytes.Buffer) {
	t.Helper()

	dir = t.TempDir()
	logged = &bytes.Buffer{}

	prevDir, prevLog := harness.runDir, log.Writer()
	harness.runDir = dir
	log.SetOutput(logged)

	harnessStall.Lock()
	harnessStall.tick, harnessStall.file = 0, ""
	harnessStall.Unlock()

	t.Cleanup(func() {
		harness.runDir = prevDir
		log.SetOutput(prevLog)
	})

	return dir, logged
}

func stallFiles(t *testing.T, dir string) []string {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "stall-*.txt"))
	if err != nil {
		t.Fatal(err)
	}

	return files
}

// A request the loop never services comes back as GAME_NOT_TICKING after the
// timeout -- and leaves, in a file the error and the log both name, where every
// goroutine was, including the one that asked. The log gets one line, not the
// stacks: a dump in the ring would evict the lines that show what the game was
// doing.
func TestNotTickingLeavesTheStacksInAFile(t *testing.T) {
	dir, logged := stallFixture(t)

	nobody := make(chan harnessCmd) // no loop drains it

	began := time.Now()
	err := harnessRunOn(nobody, func() {})

	if !errors.Is(err, errGameNotTicking) {
		t.Fatalf("an unserviced request returned %v, want GAME_NOT_TICKING", err)
	}

	if waited := time.Since(began); waited < harnessToolTimeout {
		t.Fatalf("gave up after %v, before the %v timeout", waited, harnessToolTimeout)
	}

	if !strings.Contains(err.Error(), "0 update ticks") {
		t.Errorf("the error does not say the loop stood still: %v", err)
	}

	files := stallFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("%d stack files in the run directory, want 1", len(files))
	}

	if !strings.Contains(err.Error(), files[0]) || !strings.Contains(logged.String(), files[0]) {
		t.Errorf("the stack file %s is not named by the error (%v) and the log (%q)", files[0], err, logged.String())
	}

	dump, readErr := os.ReadFile(files[0])
	if readErr != nil {
		t.Fatal(readErr)
	}

	if !strings.Contains(string(dump), "goroutine ") || !strings.Contains(string(dump), "TestNotTickingLeavesTheStacksInAFile") {
		t.Fatalf("the file does not hold the goroutine stacks (the asking test's own frame among them):\n%.400s", dump)
	}

	if strings.Contains(logged.String(), "goroutine ") {
		t.Fatalf("the stacks went into the log as well:\n%.400s", logged.String())
	}
}

// A loop stuck on one tick is dumped once, however many calls wait on it; a
// loop that has moved on and stalled again is dumped again.
func TestOneDumpPerStall(t *testing.T) {
	dir, _ := stallFixture(t)

	before := atomic.LoadInt64(&harness.tick)

	first := harnessNotTicking(before)
	second := harnessNotTicking(before)

	if n := len(stallFiles(t, dir)); n != 1 {
		t.Fatalf("two calls on one stalled tick wrote %d stack files, want 1", n)
	}

	if first.Error() != second.Error() {
		t.Errorf("the second call names a different dump:\n%v\n%v", first, second)
	}

	atomic.AddInt64(&harness.tick, 1)
	time.Sleep(time.Millisecond) // the file name has millisecond resolution

	_ = harnessNotTicking(before)

	if n := len(stallFiles(t, dir)); n != 2 {
		t.Fatalf("a new stall after the loop moved wrote %d files in all, want 2", n)
	}
}

// The tick count separates a stopped loop from a slow one.
func TestNotTickingCountsTheTicksThatRan(t *testing.T) {
	stallFixture(t)

	before := atomic.LoadInt64(&harness.tick)
	atomic.AddInt64(&harness.tick, 7)

	if err := harnessNotTicking(before); !strings.Contains(err.Error(), "7 update ticks") {
		t.Fatalf("seven ticks ran and the error says: %v", err)
	}
}
