package d2report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reports' tests point %LOCALAPPDATA% (the log's home and, with
// STRIGOI_REPORT_HOME empty, the reports') at a temp directory: the same
// no-seam choice d2logfile's tests make, for the same reason -- a seam would
// be one more thing on the path whose job is to survive a panic.
func reportHome(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv(HomeEnv, "")

	require.Equal(t, filepath.Join(dir, "Strigoi"), Root(), "the test must write into its own temp directory")
	require.NoError(t, os.MkdirAll(Root(), 0o750))

	t.Cleanup(EndRun)

	return Root()
}

// writeLog puts n numbered lines in the log the reports tail.
func writeLog(t *testing.T, n int) {
	t.Helper()

	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "line %d\r\n", i)
	}

	require.NoError(t, os.WriteFile(filepath.Join(Root(), "strigoi.log"), []byte(b.String()), 0o600))
}

func readJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &m), "%s is JSON", path)

	return m
}

func TestReportHomeOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(HomeEnv, dir)
	assert.Equal(t, dir, Root())
	assert.Equal(t, filepath.Join(dir, "feedback"), FeedbackDir())
	assert.Equal(t, filepath.Join(dir, "crashes"), CrashesDir())
}

func TestTailTakesTheLastLines(t *testing.T) {
	reportHome(t)
	writeLog(t, 250)

	lines := Tail(filepath.Join(Root(), "strigoi.log"), 200)
	require.Len(t, lines, 200)
	assert.Equal(t, "line 50", lines[0])
	assert.Equal(t, "line 249", lines[199], "the newest line last, its \\r gone")

	// Just rotated: the live log is short, the rest is in .1.
	require.NoError(t, os.Rename(filepath.Join(Root(), "strigoi.log"), filepath.Join(Root(), "strigoi.log.1")))
	require.NoError(t, os.WriteFile(filepath.Join(Root(), "strigoi.log"), []byte("new 0\nnew 1\n"), 0o600))

	lines = Tail(filepath.Join(Root(), "strigoi.log"), 5)
	assert.Equal(t, []string{"line 247", "line 248", "line 249", "new 0", "new 1"}, lines)

	assert.Nil(t, Tail(filepath.Join(Root(), "missing.log"), 5))
}

func TestWriteFeedbackWritesAllThree(t *testing.T) {
	reportHome(t)
	writeLog(t, 250)

	shot := []byte("\x89PNG fake frame")
	when := time.Date(2026, 10, 5, 14, 3, 9, 0, time.Local)

	dir, err := WriteFeedback(Feedback{
		Note: "the torch went out and I could not see why",
		Shot: shot,
		State: map[string]interface{}{
			"screen": "game",
			"game":   map[string]interface{}{"day": 2, "clock": "21:15"},
		},
		When: when,
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(FeedbackDir(), "20261005-140309"), dir)

	note, err := os.ReadFile(filepath.Join(dir, NoteFile))
	require.NoError(t, err)
	assert.Equal(t, "the torch went out and I could not see why\n", string(note))

	got, err := os.ReadFile(filepath.Join(dir, ShotFile))
	require.NoError(t, err)
	assert.Equal(t, shot, got)

	state := readJSON(t, filepath.Join(dir, StateFile))
	assert.Equal(t, "game", state["screen"])
	assert.Equal(t, 2.0, state["game"].(map[string]interface{})["day"])
	assert.Equal(t, "the torch went out and I could not see why", state["note"])
	assert.Equal(t, true, state["shot"])
	assert.Equal(t, when.Format(time.RFC3339), state["time"])

	tail, ok := state["log_tail"].([]interface{})
	require.True(t, ok, "log_tail is a list: %v", state["log_tail"])
	require.Len(t, tail, FeedbackLogLines)
	assert.Equal(t, "line 249", tail[len(tail)-1])

	// A second note in the same second gets its own folder, not this one.
	dir2, err := WriteFeedback(Feedback{Note: "again", When: when})
	require.NoError(t, err)
	assert.Equal(t, dir+"-2", dir2)

	_, err = os.Stat(filepath.Join(dir2, ShotFile))
	assert.True(t, os.IsNotExist(err), "no frame, no shot.png")
	assert.Equal(t, false, readJSON(t, filepath.Join(dir2, StateFile))["shot"])

	note, _ = os.ReadFile(filepath.Join(dir, NoteFile))
	assert.Equal(t, "the torch went out and I could not see why\n", string(note), "the first note is untouched")
}

// forcedCrashHere is a named frame the crash's stack.txt must carry.
func forcedCrashHere(t *testing.T) (dir string) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			dir = HandlePanic(r, "update", nil)
		}
	}()

	panic("forced for the test")
}

func TestWriteCrashSurvivesAPanicInsideItself(t *testing.T) {
	reportHome(t)
	writeLog(t, 400)

	// The state snapshot is the likeliest thing to break in a crash -- it reads
	// the game that just broke. Its panic must cost state.json's game fields
	// and nothing else.
	SetStateFunc(func() map[string]interface{} { panic("the snapshot broke too") })
	t.Cleanup(func() { SetStateFunc(nil) })

	dir := forcedCrashHere(t)
	require.NotEmpty(t, dir, "the crash folder was written")
	assert.Equal(t, CrashesDir(), filepath.Dir(dir))

	stack, err := os.ReadFile(filepath.Join(dir, StackFile))
	require.NoError(t, err)
	assert.Contains(t, string(stack), "panic: forced for the test")
	assert.Contains(t, string(stack), "goroutine: update")
	assert.Contains(t, string(stack), "forcedCrashHere", "every goroutine's stack, the panicking one included")
	assert.Contains(t, string(stack), "--- all goroutines ---")

	tail, err := os.ReadFile(filepath.Join(dir, LogTailFile))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(tail)), "\n")
	require.Len(t, lines, CrashLogLines)
	assert.Equal(t, "line 399", lines[len(lines)-1])

	state := readJSON(t, filepath.Join(dir, StateFile))
	assert.Equal(t, "forced for the test", state["panic"])
	assert.Contains(t, state["state_error"], "the snapshot broke too")
}

func TestWriteCrashKeepsStateItCannotEncode(t *testing.T) {
	reportHome(t)

	SetStateFunc(func() map[string]interface{} {
		return map[string]interface{}{"screen": "game", "game": func() {}}
	})
	t.Cleanup(func() { SetStateFunc(nil) })

	dir := WriteCrash("x", "draw", []byte("goroutine 7 [running]:\nsomewhere.drawing()"))
	require.NotEmpty(t, dir)

	state := readJSON(t, filepath.Join(dir, StateFile))
	assert.Equal(t, "draw", state["goroutine"])
	assert.Contains(t, state["state_error"], "did not encode")

	stack, _ := os.ReadFile(filepath.Join(dir, StackFile))
	assert.Contains(t, string(stack), "somewhere.drawing()", "the recovered goroutine's own stack leads")
}

// deadMarker writes a marker for a run that is not running: no process holds
// it open (Windows), and its pid is not alive (elsewhere).
func deadMarker(t *testing.T, m Marker) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(RunningDir(), 0o750))

	if m.PID == 0 {
		m.PID = 999999937
	}

	data, err := json.Marshal(m)
	require.NoError(t, err)

	path := filepath.Join(RunningDir(), fmt.Sprintf("%d.json", m.PID))
	require.NoError(t, os.WriteFile(path, data, 0o600))

	return path
}

func TestARunThatDidNotCloseIsReported(t *testing.T) {
	reportHome(t)
	writeLog(t, 50)

	path := deadMarker(t, Marker{Started: "2026-10-05T01:00:00-05:00", Version: "local build"})

	prev := StartRun("local test", false)
	require.Len(t, prev, 1, "the dead run is found")
	assert.False(t, prev[0].Crashed)
	assert.True(t, strings.HasSuffix(prev[0].Dir, UncleanSuffix), "folder %s", prev[0].Dir)

	for _, f := range []string{MarkerFile, LogTailFile, StateFile} {
		_, err := os.Stat(filepath.Join(prev[0].Dir, f))
		assert.NoError(t, err, "%s in the unclean report", f)
	}

	_, err := os.Stat(filepath.Join(prev[0].Dir, StackFile))
	assert.True(t, os.IsNotExist(err), "no stack for a run with no fatal error")

	assert.Equal(t, "The last run did not close cleanly -- a report was saved to "+prev[0].Dir, Notice(prev))

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "the old marker is taken")

	// Reported once: the next launch finds nothing.
	EndRun()
	assert.Empty(t, StartRun("local test", false))
}

func TestAFatalErrorIsACrash(t *testing.T) {
	reportHome(t)

	fatal := filepath.Join(Root(), "running", "999999941.crash")
	require.NoError(t, os.MkdirAll(filepath.Dir(fatal), 0o750))
	require.NoError(t, os.WriteFile(fatal, []byte("fatal error: concurrent map writes\n\ngoroutine 12 [running]:\n"), 0o600))
	deadMarker(t, Marker{PID: 999999941, CrashOutput: fatal})

	prev := StartRun("local test", false)
	require.Len(t, prev, 1)
	assert.True(t, prev[0].Crashed)

	stack, err := os.ReadFile(filepath.Join(prev[0].Dir, StackFile))
	require.NoError(t, err)
	assert.Contains(t, string(stack), "concurrent map writes")
	assert.Equal(t, "The last run crashed -- a report was saved to "+prev[0].Dir, Notice(prev))

	_, err = os.Stat(fatal)
	assert.True(t, os.IsNotExist(err), "the fatal-error file is taken with its marker")
}

func TestACaughtCrashNamesItsOwnFolder(t *testing.T) {
	reportHome(t)

	deadMarker(t, Marker{Crashed: `C:\somewhere\crashes\20261005-010203`})

	prev := StartRun("local test", false)
	require.Len(t, prev, 1)
	assert.True(t, prev[0].Crashed)
	assert.Equal(t, `C:\somewhere\crashes\20261005-010203`, prev[0].Dir)

	entries, _ := os.ReadDir(CrashesDir())
	assert.Empty(t, entries, "no second report for a crash that wrote its own")
}

func TestALiveRunIsLeftAlone(t *testing.T) {
	reportHome(t)

	assert.Empty(t, StartRun("local test", false))

	own := filepath.Join(RunningDir(), fmt.Sprintf("%d.json", os.Getpid()))

	m := readJSON(t, own)
	assert.Equal(t, float64(os.Getpid()), m["pid"])
	assert.Equal(t, "local test", m["version"])
	assert.Equal(t, false, m["harness"])

	crashOut, _ := m["crash_output"].(string)
	assert.NotEmpty(t, crashOut, "the runtime's fatal output points beside the marker")

	// A second start-up while this run lives must not take its marker.
	assert.Empty(t, scanPrevious(false, time.Now()))

	_, err := os.Stat(own)
	require.NoError(t, err, "the live marker is still there")

	// A clean exit removes both files.
	EndRun()

	_, err = os.Stat(own)
	assert.True(t, os.IsNotExist(err), "a clean exit removes the marker")

	_, err = os.Stat(crashOut)
	assert.True(t, os.IsNotExist(err), "and the fatal-error file")
}

func TestACrashMarksTheLiveMarker(t *testing.T) {
	reportHome(t)

	StartRun("local test", false)

	dir := forcedCrashHere(t)
	require.NotEmpty(t, dir)

	m := readJSON(t, filepath.Join(RunningDir(), fmt.Sprintf("%d.json", os.Getpid())))
	assert.Equal(t, dir, m["crashed"], "the next launch reads the crash's folder from the marker")
}

func TestHarnessAndPlayerRunsAreKeptApart(t *testing.T) {
	reportHome(t)

	deadMarker(t, Marker{PID: 999999951, Harness: true})

	assert.Empty(t, StartRun("local test", false), "a player's launch does not report a killed playtest")
	EndRun()

	prev := StartRun("local test", true)
	require.Len(t, prev, 1)
	assert.True(t, strings.HasSuffix(prev[0].Dir, UncleanSuffix+HarnessSuffix), "folder %s", prev[0].Dir)
}

func TestNoticeWords(t *testing.T) {
	t.Setenv(HomeEnv, `C:\R`)
	assert.Equal(t, "", Notice(nil))
	assert.Equal(t, "2 earlier runs did not close cleanly -- reports were saved to "+filepath.Join(`C:\R`, "crashes"),
		Notice([]Previous{{Dir: "a"}, {Dir: "b", Crashed: true}}))
	assert.Equal(t, "The last run crashed -- no report could be written", Notice([]Previous{{Crashed: true}}))
}
