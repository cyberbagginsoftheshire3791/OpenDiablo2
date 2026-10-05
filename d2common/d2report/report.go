// Package d2report writes the reports a player's build leaves behind: the
// feedback F8 takes (a note, the frame and the state), the crash bundle a
// panic writes before the process goes, and the "running" marker that lets the
// NEXT launch say the last one did not close cleanly -- a crash nothing caught,
// a freeze ended from Task Manager, the power going.
//
// IT IS ITS OWN PACKAGE, AND EBITEN-FREE, FOR THE SAME REASON d2logfile IS
// (see that package's comment): its tests must run on a headless CI runner, and
// the crash path is the one place a second dependency between a panic and the
// disk would hurt. main.go (untagged), d2app and the screens all reach it.
//
// EVERY FUNCTION HERE IS BEST EFFORT. A report that cannot be written must
// never stop the game, and a crash handler that panics would lose the very
// stack it was called to save; the crash writer guards each step on its own.
package d2report

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2logfile"
)

// HomeEnv overrides the folder the reports go under. The playtest launcher
// points it at each test's private home, so a suite's killed games never
// leave markers or crash folders in the player's own %LOCALAPPDATA%\Strigoi --
// which the launched games otherwise share (only %APPDATA% is private).
const HomeEnv = "STRIGOI_REPORT_HOME"

// Tail sizes ruled with the design (5 Oct 2026).
const (
	FeedbackLogLines = 200
	CrashLogLines    = 300
)

// stampLayout is the folder name: <yyyyMMdd-HHmmss>, local time.
const stampLayout = "20060102-150405"

// Root is the folder the reports go under: $STRIGOI_REPORT_HOME when set,
// %LOCALAPPDATA%\Strigoi otherwise (d2logfile.DataDir).
func Root() string {
	if home := os.Getenv(HomeEnv); home != "" {
		return home
	}

	return d2logfile.DataDir()
}

// FeedbackDir holds one folder per F8 note.
func FeedbackDir() string { return filepath.Join(Root(), "feedback") }

// CrashesDir holds one folder per crash, and one per run that did not close
// cleanly (suffixed -unclean).
func CrashesDir() string { return filepath.Join(Root(), "crashes") }

// RunningDir holds the markers of the runs in progress, one per process.
func RunningDir() string { return filepath.Join(Root(), "running") }

// Stamp is a report folder's name for t.
func Stamp(t time.Time) string { return t.Format(stampLayout) }

// newDir makes parent\<stamp><suffix>, adding -2, -3... when a folder of that
// name exists already (two notes in one second must not share a folder).
func newDir(parent string, now time.Time, suffix string) (string, error) {
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return "", err
	}

	base := Stamp(now) + suffix

	for i := 1; i < 100; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}

		dir := filepath.Join(parent, name)

		err := os.Mkdir(dir, 0o750)
		if err == nil {
			return dir, nil
		}

		if !os.IsExist(err) {
			return "", err
		}
	}

	return "", fmt.Errorf("no free report folder for %s under %s", base, parent)
}

// tailBytes is how far back Tail reads: 300 lines of this game's log run well
// under it, and reading the whole 8 MiB log to find them would stall the frame
// F8 is pressed on.
const tailBytes = 256 << 10

// Tail is the last n lines of the file at path, oldest first; nil when it
// cannot be read. When the live log holds fewer than n lines -- it was just
// rotated -- the rest come from the generation before it (path + ".1").
func Tail(path string, n int) []string {
	lines := tailOf(path, n)
	if len(lines) < n {
		older := tailOf(path+".1", n-len(lines))
		lines = append(older, lines...)
	}

	return lines
}

func tailOf(path string, n int) []string {
	if n <= 0 {
		return nil
	}

	f, err := os.Open(path) //nolint:gosec // our own log
	if err != nil {
		return nil
	}

	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil
	}

	start := info.Size() - tailBytes
	if start < 0 {
		start = 0
	}

	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}

	if start > 0 {
		// The first line is probably cut; drop it.
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}

	var lines []string

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)

	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines
}

// writeFile is os.WriteFile, the one way the reports write a file -- a seam
// so a test can refuse one file of a report and watch what the report does
// then (QA-002). Nothing else ever sets it.
//
//nolint:gochecknoglobals // a test seam
var writeFile = os.WriteFile

func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	return writeFile(path, append(data, '\n'), 0o600)
}

func writeLines(path string, lines []string) error {
	text := strings.Join(lines, "\n")
	if text != "" {
		text += "\n"
	}

	return writeFile(path, []byte(text), 0o600)
}

// Feedback is one F8 note.
type Feedback struct {
	Note  string
	Shot  []byte                 // the frame as PNG bytes; nil writes no shot.png
	State map[string]interface{} // the game's state at the moment; log_tail is added
	When  time.Time
}

// File names in a feedback folder.
const (
	NoteFile  = "note.txt"
	ShotFile  = "shot.png"
	StateFile = "state.json"
)

// WriteFeedback writes FeedbackDir()\<stamp>\{note.txt, shot.png, state.json}
// and returns the folder. state.json carries what the caller gave plus the
// last FeedbackLogLines lines of the log ("log_tail"), the note, the time and
// whether a shot was taken. The note is written first: if anything later
// fails, what he said is already on disk.
func WriteFeedback(fb Feedback) (string, error) {
	when := fb.When
	if when.IsZero() {
		when = time.Now()
	}

	dir, err := newDir(FeedbackDir(), when, "")
	if err != nil {
		return "", err
	}

	if err := writeFile(filepath.Join(dir, NoteFile), []byte(fb.Note+"\n"), 0o600); err != nil {
		return dir, err
	}

	var firstErr error

	if fb.Shot != nil {
		if err := writeFile(filepath.Join(dir, ShotFile), fb.Shot, 0o600); err != nil {
			firstErr = err
		}
	}

	state := map[string]interface{}{}
	for k, v := range fb.State {
		state[k] = v
	}

	state["note"] = fb.Note
	state["time"] = when.Format(time.RFC3339)
	state["shot"] = fb.Shot != nil
	state["log_tail"] = nonNil(Tail(d2logfile.LogFilePath(), FeedbackLogLines))

	if err := writeJSON(filepath.Join(dir, StateFile), state); err != nil && firstErr == nil {
		firstErr = err
	}

	return dir, firstErr
}

func nonNil(lines []string) []string {
	if lines == nil {
		return []string{}
	}

	return lines
}

// Crash file names.
const (
	StackFile   = "stack.txt"
	LogTailFile = "log-tail.txt"
	MarkerFile  = "marker.json"
)

// stateFn is the App's cheap snapshot of what was on screen, for a crash's
// state.json. It is called inside the crash handler, under its own recover.
//
//nolint:gochecknoglobals // one process, one crash handler
var (
	stateMu sync.Mutex
	stateFn func() map[string]interface{}
)

// SetStateFunc names the snapshot a crash's state.json is built from.
func SetStateFunc(f func() map[string]interface{}) {
	stateMu.Lock()
	stateFn = f
	stateMu.Unlock()
}

func callState() (state map[string]interface{}, perr interface{}) {
	defer func() {
		if r := recover(); r != nil {
			state, perr = nil, r
		}
	}()

	stateMu.Lock()
	f := stateFn
	stateMu.Unlock()

	if f == nil {
		return nil, nil
	}

	return f(), nil
}

// allStacks is every goroutine's stack, growing the buffer until it fits.
func allStacks() []byte {
	buf := make([]byte, 1<<20)

	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) || len(buf) >= 64<<20 {
			return buf[:n]
		}

		buf = make([]byte, 2*len(buf))
	}
}

// WriteCrash writes CrashesDir()\<stamp>\{stack.txt, log-tail.txt,
// state.json} for a panic and returns the folder ("" when not even the folder
// could be made). where names the goroutine that panicked (main, update,
// draw); stack is that goroutine's stack as the recover saw it (may be nil).
//
// IT NEVER PANICS. Each file is written under its own recover, so a fault in
// the state snapshot -- the likeliest, since the game it reads is the thing
// that just broke -- costs state.json and nothing else; state.json then says
// why it is short ("state_error").
func WriteCrash(reason interface{}, where string, stack []byte) (dir string) {
	dir, _ = writeCrash(reason, where, stack)

	return dir
}

// writeCrash is WriteCrash, saying also whether every file was written: a
// folder short of one is not "a report was saved" (QA-002).
func writeCrash(reason interface{}, where string, stack []byte) (dir string, whole bool) {
	defer func() {
		if r := recover(); r != nil {
			whole = false
		}
	}()

	now := time.Now()

	d, err := newDir(CrashesDir(), now, "")
	if err != nil {
		return "", false
	}

	dir = d
	whole = true

	// guard runs one file's write; a write refused or a panic in it makes the
	// folder short.
	guard := func(f func() error) {
		defer func() {
			if r := recover(); r != nil {
				whole = false
			}
		}()

		if f() != nil {
			whole = false
		}
	}

	guard(func() error {
		var b strings.Builder

		fmt.Fprintf(&b, "panic: %v\ngoroutine: %s\ntime: %s\npid: %d\n\n", reason, where, now.Format(time.RFC3339), os.Getpid())

		if len(stack) > 0 {
			b.WriteString("--- the goroutine that panicked ---\n")
			b.Write(stack)
			b.WriteString("\n\n")
		}

		b.WriteString("--- all goroutines ---\n")
		b.Write(allStacks())

		return writeFile(filepath.Join(d, StackFile), []byte(b.String()), 0o600)
	})

	guard(func() error {
		return writeLines(filepath.Join(d, LogTailFile), Tail(d2logfile.LogFilePath(), CrashLogLines))
	})

	guard(func() error {
		state := map[string]interface{}{
			"panic":     fmt.Sprint(reason),
			"goroutine": where,
			"time":      now.Format(time.RFC3339),
			"pid":       os.Getpid(),
		}

		if m := currentMarker(); m != nil {
			state["run"] = m
		}

		snap, perr := callState()
		if perr != nil {
			state["state_error"] = fmt.Sprintf("the state snapshot panicked too: %v", perr)
		}

		for k, v := range snap {
			if _, taken := state[k]; !taken {
				state[k] = v
			}
		}

		if err := writeJSON(filepath.Join(d, StateFile), state); err != nil {
			// A value json cannot take (a func, a NaN): keep the plain fields.
			delete(state, "game")
			state["state_error"] = fmt.Sprintf("state did not encode: %v", err)

			return writeJSON(filepath.Join(d, StateFile), map[string]interface{}{
				"panic": state["panic"], "goroutine": where, "time": state["time"],
				"pid": state["pid"], "state_error": state["state_error"],
			})
		}

		return nil
	})

	return dir, whole
}

// HandlePanic is what a recover() hands a panic to: the crash folder, the
// marker told this run crashed (so the next launch says so and names the
// folder instead of writing a second, emptier report), and the folder named in
// the log. It returns the folder. The caller exits afterwards.
//
// A folder that could not be written whole is not named in the marker
// (QA-002): the next launch reports the run as a crash from the log tail --
// which holds the panic and its stack, logged before this is called -- instead
// of saying a report was saved to a folder without its stack.
func HandlePanic(reason interface{}, where string, stack []byte) string {
	dir, whole := writeCrash(reason, where, stack)

	func() {
		defer func() { _ = recover() }()
		markCrashed(dir, whole && dir != "", reason)
	}()

	return dir
}
