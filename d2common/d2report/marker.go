package d2report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2logfile"
)

// Marker is a run in progress: RunningDir()\<pid>.json, written at start-up,
// held open for the whole run and removed on a clean exit. Found at a later
// start-up, its run ended some other way.
//
// ONE MARKER PER PROCESS, AND THE OPEN FILE IS THE LIVENESS TEST. The folder
// is shared by every game this user starts -- his own, and a hand-started
// harness game beside it -- so a single running.json would be overwritten by
// the second launch and removed by the first clean exit. Each run keeps its
// own marker OPEN; on Windows a file another process holds open (Go opens
// without FILE_SHARE_DELETE) cannot be renamed, so a start-up that can rename
// a marker has proved its owner is gone -- whatever ended it -- and one that
// cannot leaves it alone. No pid is trusted on Windows, so a reused pid cannot
// fool it. Elsewhere the pid is asked (alive_other.go).
type Marker struct {
	PID     int    `json:"pid"`
	Started string `json:"started"`
	Version string `json:"version"`
	Exe     string `json:"exe,omitempty"`

	// Harness is a -harness run. A harness start-up reports only harness
	// markers and a player's only his own: a playtest killed by its script
	// is not his game ending badly.
	Harness bool `json:"harness"`

	// CrashOutput is the file the Go runtime writes a FATAL error into
	// (runtime/debug.SetCrashOutput): a panic on a goroutine no recover
	// covers, a concurrent map write, a stack overflow. Empty unless one
	// happened.
	CrashOutput string `json:"crash_output,omitempty"`

	// Crashed is the crash folder HandlePanic wrote, set as the run died.
	Crashed string `json:"crashed,omitempty"`

	// Panicked is a caught panic whose own folder could not be written whole
	// (QA-002, 5 Oct 2026): the next launch reports the run as a crash from
	// the log tail, which holds the panic's stack (crashGuard logs it first).
	Panicked string `json:"panicked,omitempty"`
}

// Previous is a run found ended without a clean exit.
type Previous struct {
	Marker  Marker `json:"marker"`
	Dir     string `json:"dir"`     // its report folder
	Crashed bool   `json:"crashed"` // a crash (caught or fatal), not just an unclean end
}

// UncleanSuffix ends the folder of a run that did not close cleanly;
// HarnessSuffix follows it for a -harness run's.
const (
	UncleanSuffix = "-unclean"
	HarnessSuffix = "-harness"
)

//nolint:gochecknoglobals // one process, one run
var (
	runMu       sync.Mutex
	runMarker   *Marker
	runFile     *os.File
	runPath     string
	runCrashOut string
)

// StartRun looks for runs of the same kind (harness or not) that ended
// without a clean exit, writes a report for each, then writes this run's own
// marker and points the runtime's fatal-error output at a file beside it. It
// returns the earlier runs it found, newest first. Call EndRun on every clean
// exit (Exit does).
func StartRun(version string, harness bool) []Previous {
	_ = os.MkdirAll(RunningDir(), 0o750)

	prev := scanPrevious(harness, time.Now())

	pid := os.Getpid()
	m := &Marker{
		PID:     pid,
		Started: time.Now().Format(time.RFC3339),
		Version: version,
		Harness: harness,
	}

	if exe, err := os.Executable(); err == nil {
		m.Exe = exe
	}

	crashPath := filepath.Join(RunningDir(), strconv.Itoa(pid)+".crash")
	if f, err := os.Create(crashPath); err == nil {
		if debug.SetCrashOutput(f, debug.CrashOptions{}) == nil {
			m.CrashOutput = crashPath
		}

		_ = f.Close()
	}

	path := filepath.Join(RunningDir(), strconv.Itoa(pid)+".json")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return prev
	}

	runMu.Lock()
	runMarker, runFile, runPath, runCrashOut = m, f, path, m.CrashOutput
	runMu.Unlock()

	writeMarker()

	return prev
}

// writeMarker rewrites the open marker in place. Caller need not hold runMu.
func writeMarker() {
	runMu.Lock()
	defer runMu.Unlock()

	if runFile == nil || runMarker == nil {
		return
	}

	data, err := json.MarshalIndent(runMarker, "", "  ")
	if err != nil {
		return
	}

	_ = runFile.Truncate(0)
	_, _ = runFile.WriteAt(append(data, '\n'), 0)
	_ = runFile.Sync()
}

// markCrashed records this run's crash folder in its marker, which stays.
// A folder that could not be written whole is not named (QA-002): the marker
// says the run panicked instead, and the next launch reports it as a crash
// from the log tail rather than pointing at a folder that lacks the stack.
func markCrashed(dir string, whole bool, reason interface{}) {
	runMu.Lock()
	if runMarker != nil {
		if whole {
			runMarker.Crashed = dir
		} else {
			runMarker.Panicked = fmt.Sprint(reason)
		}
	}
	runMu.Unlock()

	writeMarker()
}

func currentMarker() *Marker {
	runMu.Lock()
	defer runMu.Unlock()

	if runMarker == nil {
		return nil
	}

	m := *runMarker

	return &m
}

// EndRun is a clean exit: the marker and the fatal-error file go. Safe to call
// more than once, and before StartRun.
func EndRun() {
	runMu.Lock()
	defer runMu.Unlock()

	if runFile == nil {
		return
	}

	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	_ = runFile.Close()
	_ = os.Remove(runPath)

	if runCrashOut != "" {
		_ = os.Remove(runCrashOut)
	}

	runFile, runMarker, runPath, runCrashOut = nil, nil, "", ""
}

// Exit is a clean exit with this code: EndRun, then os.Exit.
func Exit(code int) {
	EndRun()
	os.Exit(code)
}

// scanPrevious reports every marker of this kind whose owner is gone.
func scanPrevious(harness bool, now time.Time) []Previous {
	entries, err := os.ReadDir(RunningDir())
	if err != nil {
		return nil
	}

	var found []Previous

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}

		path := filepath.Join(RunningDir(), name)

		data, err := os.ReadFile(path) //nolint:gosec // our own folder
		if err != nil {
			continue
		}

		var m Marker
		if json.Unmarshal(data, &m) != nil {
			// Torn (the power went mid-write): unreadable is not "not ours".
			// Its pid is its name; its kind is unknown, so it is claimed only
			// by the kind that cannot be fooled by it -- a player's start-up.
			if harness {
				continue
			}

			m.PID, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(name, ".json"), retryInfix))
		}

		if m.Harness != harness {
			continue
		}

		claimed, ok := claim(path, m)
		if !ok {
			continue // its owner is still running
		}

		// THE EVIDENCE GOES ONLY WITH A WHOLE REPORT (QA-001, 5 Oct 2026). A
		// report that could not be written -- its folder refused, a file in it
		// refused -- keeps the marker and the runtime's fatal output for the
		// next start-up to try again. Before, both were deleted whatever the
		// report did, and a crashes folder that could not be made lost the
		// crash for good.
		p, whole := report(m, data, now)
		if whole {
			_ = os.Remove(claimed)

			if m.CrashOutput != "" {
				_ = os.Remove(m.CrashOutput)
			}
		} else {
			keepForRetry(claimed, path, m)
		}

		found = append(found, p)
	}

	sort.Slice(found, func(i, j int) bool { return found[i].Marker.Started > found[j].Marker.Started })

	return found
}

// retryInfix names a marker (and its fatal output) kept for the next start-up
// when its own names are about to be taken: a dead run whose pid is this
// run's (pids are reused) would have <pid>.json and <pid>.crash overwritten by
// this run's own files, so it is kept as <pid>.retry.json and .retry.crash.
const retryInfix = ".retry"

// keepForRetry puts a claimed marker whose report failed back where a later
// scan claims it as any other: under its own name, beside its fatal output,
// which stays where it is. Only a dead run with this run's pid moves aside
// (retryInfix). If even that fails, the claimed file is left as it is -- the
// evidence is kept, if not retried.
func keepForRetry(claimed, path string, m Marker) {
	if m.PID != os.Getpid() {
		_ = os.Rename(claimed, path)
		return
	}

	base := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".json"), retryInfix) + retryInfix

	original := m.CrashOutput
	moved := ""

	if original != "" && filepath.Base(original) != base+".crash" {
		moved = filepath.Join(RunningDir(), base+".crash")
		if os.Rename(original, moved) == nil {
			m.CrashOutput = moved
		} else {
			moved = ""
		}
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err == nil {
		err = writeFile(filepath.Join(RunningDir(), base+".json"), append(data, '\n'), 0o600)
	}

	if err != nil {
		if moved != "" {
			_ = os.Rename(moved, original)
		}

		return
	}

	_ = os.Remove(claimed)
}

// report writes the folder for one ended run, and says whether every file of
// it was written. A report that is not whole leaves no folder behind (its
// retry will write one), so a recovered crash is reported once.
func report(m Marker, raw []byte, now time.Time) (Previous, bool) {
	var fatal []byte
	if m.CrashOutput != "" {
		fatal, _ = os.ReadFile(m.CrashOutput)
	}

	// A panic HandlePanic caught has its folder already; say so and add
	// nothing. A fatal error the runtime wrote has a stack and no folder.
	if m.Crashed != "" && len(fatal) == 0 {
		return Previous{Marker: m, Dir: m.Crashed, Crashed: true}, true
	}

	p := Previous{Marker: m, Crashed: len(fatal) > 0 || m.Crashed != "" || m.Panicked != ""}

	suffix := UncleanSuffix
	if m.Harness {
		suffix += HarnessSuffix
	}

	dir, err := newDir(CrashesDir(), now, suffix)
	if err != nil {
		return p, false
	}

	reason := "the run ended without a clean exit: a crash nothing caught, a freeze ended " +
		"from Task Manager, or the power going"

	switch {
	case len(fatal) > 0:
		reason = "the run crashed: the Go runtime wrote a fatal error (stack.txt)"
	case m.Panicked != "":
		reason = "the run crashed (" + m.Panicked + ") and its own crash folder could not be written whole; " +
			"the panic and its stack are in log-tail.txt"
	}

	errs := []error{
		writeFile(filepath.Join(dir, MarkerFile), raw, 0o600),
		writeLines(filepath.Join(dir, LogTailFile), Tail(d2logfile.LogFilePath(), CrashLogLines)),
	}

	if len(fatal) > 0 {
		errs = append(errs, writeFile(filepath.Join(dir, StackFile), fatal, 0o600))
	}

	errs = append(errs, writeJSON(filepath.Join(dir, StateFile), map[string]interface{}{
		"reason":          reason,
		"previous_run":    m,
		"detected_at":     now.Format(time.RFC3339),
		"detected_by_pid": os.Getpid(),
		"crashed":         p.Crashed,
	}))

	for _, e := range errs {
		if e != nil {
			_ = os.RemoveAll(dir)
			return p, false
		}
	}

	p.Dir = dir

	return p, true
}

// Notice is the one line the main menu shows for runs StartRun found, "" for
// none.
func Notice(prev []Previous) string {
	switch {
	case len(prev) == 0:
		return ""
	case len(prev) == 1 && prev[0].Dir == "":
		if prev[0].Crashed {
			return "The last run crashed -- its report could not be written; the next launch will try again"
		}

		return "The last run did not close cleanly -- its report could not be written; the next launch will try again"
	case len(prev) == 1 && prev[0].Crashed:
		return "The last run crashed -- a report was saved to " + prev[0].Dir
	case len(prev) == 1:
		return "The last run did not close cleanly -- a report was saved to " + prev[0].Dir
	}

	for _, p := range prev {
		if p.Dir == "" {
			return fmt.Sprintf("%d earlier runs did not close cleanly -- not every report could be written; "+
				"the next launch will try again", len(prev))
		}
	}

	return fmt.Sprintf("%d earlier runs did not close cleanly -- reports were saved to %s", len(prev), CrashesDir())
}
