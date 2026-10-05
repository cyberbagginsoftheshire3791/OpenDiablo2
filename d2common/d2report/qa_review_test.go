package d2report

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// QA-001 and QA-002 (independent QA pass, 5 Oct 2026). A report that could not
// be written must not take the evidence it was written from: the dead run's
// marker and the runtime's fatal output stay for a later start-up to report,
// and that report is written once.
//
// TestQAReportFailurePreservesEvidence is the QA pass's own reproduction
// (qa-feedback-6ec2b41f, qa_review_test.go), ported: a synthetic dead run with
// a fatal stack, and a FILE where the crashes folder should be, so the report
// cannot make its folder. Its blocked case failed at 6ec2b41f-7679ab00 ("failed
// crash reporting deleted both original marker and fatal stack").
func TestQAReportFailurePreservesEvidence(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "writable_control", true: "blocked_crash_destination"}[blocked], func(t *testing.T) {
			reportHome(t)
			require.NoError(t, os.MkdirAll(RunningDir(), 0o700))

			fatal := filepath.Join(RunningDir(), "999999.crash")
			require.NoError(t, os.WriteFile(fatal, []byte("fatal QA evidence"), 0o600))

			marker := filepath.Join(RunningDir(), "999999.json")
			raw, _ := json.Marshal(Marker{PID: 999999, Started: "2026-10-05T00:00:00Z", CrashOutput: fatal})
			require.NoError(t, os.WriteFile(marker, raw, 0o600))

			if blocked {
				require.NoError(t, os.WriteFile(CrashesDir(), []byte("blocks directory creation"), 0o600))
			}

			previous := scanPrevious(false, time.Now())
			require.Len(t, previous, 1, "expected one previous run")

			if !blocked {
				require.NotEmpty(t, previous[0].Dir, "positive control must produce a folder")
				_, err := os.Stat(filepath.Join(previous[0].Dir, StackFile))
				require.NoError(t, err)

				return
			}

			require.Empty(t, previous[0].Dir, "blocked destination unexpectedly wrote a report")
			assert.True(t, previous[0].Crashed)
			assert.Contains(t, Notice(previous), "could not be written; the next launch will try again")

			// (2) the evidence is kept where it was: the marker under its own
			// name, the fatal output beside it.
			var kept Marker

			data, err := os.ReadFile(marker)
			require.NoError(t, err, "failed crash reporting deleted the marker; the next launch cannot retry")
			require.NoError(t, json.Unmarshal(data, &kept))
			assert.Equal(t, 999999, kept.PID)
			assert.Equal(t, fatal, kept.CrashOutput)

			stack, err := os.ReadFile(fatal)
			require.NoError(t, err, "failed crash reporting deleted the fatal stack")
			assert.Equal(t, "fatal QA evidence", string(stack))

			// (3) the obstruction gone, the next scan reports them ...
			require.NoError(t, os.Remove(CrashesDir()))

			again := scanPrevious(false, time.Now())
			require.Len(t, again, 1)
			require.NotEmpty(t, again[0].Dir)
			assert.True(t, again[0].Crashed)

			got, err := os.ReadFile(filepath.Join(again[0].Dir, StackFile))
			require.NoError(t, err)
			assert.Equal(t, "fatal QA evidence", string(got), "the recovered report carries the original stack")

			// ... and (4) once: the evidence is taken with the whole report.
			assert.Empty(t, scanPrevious(false, time.Now()), "a recovered run is reported once")

			dirs, _ := os.ReadDir(CrashesDir())
			assert.Len(t, dirs, 1, "one report, not two")

			left, _ := os.ReadDir(RunningDir())
			assert.Empty(t, left, "nothing of the dead run is left in running\\")
		})
	}
}

// refuseFile makes the report seam refuse one file name, for the test.
func refuseFile(t *testing.T, base string) {
	t.Helper()

	writeFile = func(name string, data []byte, perm os.FileMode) error {
		if filepath.Base(name) == base {
			return errors.New("refused for the test: " + base)
		}

		return os.WriteFile(name, data, perm)
	}

	t.Cleanup(func() { writeFile = os.WriteFile })
}

// QA-002: a file of the report refused AFTER its folder was made. Before, the
// report went on, the evidence was deleted, and the menu said "a report was
// saved" to a folder without the stack.
func TestAPartialReportKeepsTheEvidence(t *testing.T) {
	reportHome(t)

	fatal := filepath.Join(Root(), "running", "999999961.crash")
	require.NoError(t, os.MkdirAll(filepath.Dir(fatal), 0o750))
	require.NoError(t, os.WriteFile(fatal, []byte("fatal: the partial-report test"), 0o600))
	deadMarker(t, Marker{PID: 999999961, CrashOutput: fatal})

	refuseFile(t, StackFile)

	prev := scanPrevious(false, time.Now())
	require.Len(t, prev, 1)
	assert.Empty(t, prev[0].Dir, "a report short of its stack is not a saved report")
	assert.NotContains(t, Notice(prev), "a report was saved")

	dirs, _ := os.ReadDir(CrashesDir())
	assert.Empty(t, dirs, "the partial folder is not left behind to be reported twice")

	_, err := os.Stat(filepath.Join(RunningDir(), "999999961.json"))
	require.NoError(t, err, "the marker is kept")

	_, err = os.Stat(fatal)
	require.NoError(t, err, "the fatal output is kept")

	// The write allowed again: reported, whole, once.
	writeFile = os.WriteFile

	again := scanPrevious(false, time.Now())
	require.Len(t, again, 1)
	require.NotEmpty(t, again[0].Dir)
	assert.Equal(t, "The last run crashed -- a report was saved to "+again[0].Dir, Notice(again))
	assert.Empty(t, scanPrevious(false, time.Now()))
}

// (5) A kept run keeps its kind and the live-run rule: a player's start-up
// leaves a harness run's retry marker alone, and a live marker is never kept
// or taken.
func TestAKeptRunKeepsItsKind(t *testing.T) {
	reportHome(t)

	deadMarker(t, Marker{PID: 999999971, Harness: true})
	require.NoError(t, os.WriteFile(CrashesDir(), []byte("blocks"), 0o600))

	prev := scanPrevious(true, time.Now())
	require.Len(t, prev, 1)
	require.Empty(t, prev[0].Dir)

	_, err := os.Stat(filepath.Join(RunningDir(), "999999971.json"))
	require.NoError(t, err)

	require.NoError(t, os.Remove(CrashesDir()))

	assert.Empty(t, StartRun("local test", false), "a player's start-up does not take a harness run's kept marker")
	assert.Empty(t, scanPrevious(false, time.Now()), "nor does a second player start-up take this live run's marker")

	EndRun()

	prev = scanPrevious(true, time.Now())
	require.Len(t, prev, 1, "the harness run is reported by a harness start-up")
	assert.True(t, strings.HasSuffix(prev[0].Dir, UncleanSuffix+HarnessSuffix), "folder %s", prev[0].Dir)
}

// QA-002, the crash's own folder: a caught panic whose folder is short of a
// file does not name that folder in the marker; it says the run panicked, and
// the next launch reports a crash from the log tail.
func TestAShortCrashFolderIsNotNamed(t *testing.T) {
	reportHome(t)
	writeLog(t, 20)

	StartRun("local test", false)
	refuseFile(t, StackFile)

	dir := forcedCrashHere(t)
	require.NotEmpty(t, dir, "the folder itself was made")

	m := readJSON(t, filepath.Join(RunningDir(), fmt.Sprintf("%d.json", os.Getpid())))
	assert.Nil(t, m["crashed"], "a folder without its stack is not named as the crash's report")
	assert.Equal(t, "forced for the test", m["panicked"])

	// What the next launch makes of such a marker.
	writeFile = os.WriteFile
	EndRun()
	deadMarker(t, Marker{PID: 999999981, Panicked: "forced for the test"})

	prev := StartRun("local test", false)
	require.Len(t, prev, 1)
	assert.True(t, prev[0].Crashed)
	assert.True(t, strings.HasSuffix(prev[0].Dir, UncleanSuffix))
	assert.Equal(t, "The last run crashed -- a report was saved to "+prev[0].Dir, Notice(prev))
	assert.Contains(t, readJSON(t, filepath.Join(prev[0].Dir, StateFile))["reason"], "log-tail.txt")
}

// A dead run with THIS run's pid (pids are reused) cannot be kept under its
// own names: this run's marker and fatal-output file are about to take them.
// It moves aside as <pid>.retry.json / .retry.crash, still claimable.
func TestAKeptRunWithOurPidMovesAside(t *testing.T) {
	reportHome(t)

	pid := os.Getpid()
	fatal := filepath.Join(Root(), "running", fmt.Sprintf("%d.crash", pid))
	require.NoError(t, os.MkdirAll(filepath.Dir(fatal), 0o750))
	require.NoError(t, os.WriteFile(fatal, []byte("fatal: an earlier run with our pid"), 0o600))
	deadMarker(t, Marker{PID: pid, CrashOutput: fatal})
	require.NoError(t, os.WriteFile(CrashesDir(), []byte("blocks"), 0o600))

	prev := StartRun("local test", false)
	require.Len(t, prev, 1)
	require.Empty(t, prev[0].Dir)

	retry := filepath.Join(RunningDir(), fmt.Sprintf("%d.retry.json", pid))
	kept := readJSON(t, retry)
	assert.Equal(t, filepath.Join(RunningDir(), fmt.Sprintf("%d.retry.crash", pid)), kept["crash_output"])

	stack, err := os.ReadFile(kept["crash_output"].(string))
	require.NoError(t, err)
	assert.Equal(t, "fatal: an earlier run with our pid", string(stack), "this run's own fatal-output file did not overwrite it")

	EndRun()
	require.NoError(t, os.Remove(CrashesDir()))

	prev = StartRun("local test", false)
	require.Len(t, prev, 1)
	require.NotEmpty(t, prev[0].Dir)

	got, err := os.ReadFile(filepath.Join(prev[0].Dir, StackFile))
	require.NoError(t, err)
	assert.Equal(t, "fatal: an earlier run with our pid", string(got))
}
