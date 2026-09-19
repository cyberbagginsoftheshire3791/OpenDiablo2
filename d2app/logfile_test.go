package d2app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The log was UNBOUNDED until 19 September 2026, measured at about 20 MB, and it
// is shared: the game appends to it and so does every harness launch, so one
// playtest suite adds twenty launches' worth. Rotation is c-2b's, ruled the same
// day, and these are its tests.
//
// They work by pointing DataDir at a temp directory through the one environment
// variable it reads, rather than by introducing a seam. A seam here would be a
// second thing to keep right on the path whose entire job is to survive a panic.
func logDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)

	// On a machine where LOCALAPPDATA is meaningful this is the whole story; on
	// one where it is not, assert the fallback did not win anyway, because a test
	// that silently measured the user's real cache directory would be worse than
	// no test.
	require.Equal(t, filepath.Join(dir, "Strigoi"), DataDir(),
		"the test must be writing into its own temp directory")

	return filepath.Join(dir, "Strigoi")
}

func TestOpenLogFileRotatesAnOversizeLog(t *testing.T) {
	logDir(t)

	require.NoError(t, os.MkdirAll(DataDir(), 0o750))

	old := strings.Repeat("x", logMaxBytes+1)
	require.NoError(t, os.WriteFile(LogFilePath(), []byte(old), 0o644))

	f := OpenLogFile()
	require.NotNil(t, f, "a rotation must never cost the caller its log")

	defer func() { _ = f.Close() }()

	_, err := f.WriteString("this launch\n")
	require.NoError(t, err)

	moved, err := os.ReadFile(PreviousLogFilePath())
	require.NoError(t, err, "the oversize log must be kept, not deleted")
	assert.Len(t, moved, len(old), "and kept whole")

	live, err := os.ReadFile(LogFilePath())
	require.NoError(t, err)
	assert.Equal(t, "this launch\n", string(live),
		"the live log starts again from empty -- this is the assertion the 20 MB file failed")
}

// THE CONTROL, and without it this file would pass against a build that rotated
// unconditionally, or truncated, or deleted. A log under the cap is left exactly
// as it was and appended to.
func TestOpenLogFileLeavesASmallLogAlone(t *testing.T) {
	logDir(t)

	require.NoError(t, os.MkdirAll(DataDir(), 0o750))
	require.NoError(t, os.WriteFile(LogFilePath(), []byte("last launch\n"), 0o644))

	f := OpenLogFile()
	require.NotNil(t, f)

	defer func() { _ = f.Close() }()

	_, err := f.WriteString("this launch\n")
	require.NoError(t, err)

	live, err := os.ReadFile(LogFilePath())
	require.NoError(t, err)
	assert.Equal(t, "last launch\nthis launch\n", string(live),
		"under the cap the log is appended to, not rotated")

	_, err = os.Stat(PreviousLogFilePath())
	assert.True(t, os.IsNotExist(err), "and nothing is moved aside")
}

// ONE generation, not a growing pile. Two rotations must leave two files, and
// the kept one must be the newer of the two old logs.
func TestOpenLogFileKeepsExactlyOnePreviousGeneration(t *testing.T) {
	logDir(t)

	require.NoError(t, os.MkdirAll(DataDir(), 0o750))

	for _, marker := range []string{"first", "second"} {
		body := marker + strings.Repeat("x", logMaxBytes)
		require.NoError(t, os.WriteFile(LogFilePath(), []byte(body), 0o644))

		f := OpenLogFile()
		require.NotNil(t, f)
		require.NoError(t, f.Close())
	}

	kept, err := os.ReadFile(PreviousLogFilePath())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(kept), "second"),
		"the generation kept is the most recent one that was moved aside")

	_, err = os.Stat(LogFilePath() + ".2")
	assert.True(t, os.IsNotExist(err), "rotation keeps one generation, not a pile")

	entries, err := os.ReadDir(DataDir())
	require.NoError(t, err)
	assert.Len(t, entries, 2, "the log and one generation behind it, and nothing else: %v", entries)
}

// A log that does not exist yet is the first-launch case, and it must not be a
// rotation, an error, or a nil file.
func TestOpenLogFileCreatesTheFirstLog(t *testing.T) {
	logDir(t)

	f := OpenLogFile()
	require.NotNil(t, f, "the first launch of a fresh install must still get a log")

	defer func() { _ = f.Close() }()

	_, err := os.Stat(LogFilePath())
	require.NoError(t, err, "and the directory is created for it")

	_, err = os.Stat(PreviousLogFilePath())
	assert.True(t, os.IsNotExist(err))
}
