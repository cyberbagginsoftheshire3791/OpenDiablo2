package d2app

import (
	"os"
	"path/filepath"
)

// DataDir is the per-user directory Strigoi writes into: %LOCALAPPDATA%\Strigoi
// on Windows, the user cache dir elsewhere. The harness run directories and the
// crash/diagnostic log both live under it. It is the one place the root is
// derived -- the harness (harness.go, build-tagged) reuses it rather than
// repeating the os.Getenv("LOCALAPPDATA") logic, and main.go (untagged) cannot
// see the tagged code, so the helper lives here in an untagged file.
func DataDir() string {
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		root, _ = os.UserCacheDir()
	}

	return filepath.Join(root, "Strigoi")
}

// LogFilePath is where a friend's build writes its log. The pre-flight README
// names it so a crash report has somewhere to come from.
func LogFilePath() string {
	return filepath.Join(DataDir(), "strigoi.log")
}

// PreviousLogFilePath is the one generation of log kept behind the live one.
func PreviousLogFilePath() string {
	return LogFilePath() + ".1"
}

// logMaxBytes is how large the log may be at the moment a launch starts.
//
// [DIAL], and it exists because the file was UNBOUNDED. Measured 19 Sep 2026 at
// about 20 MB and still growing, and it is shared: the game appends to it and so
// does every harness launch, so a playtest suite adds twenty launches' worth in
// one run. That is the reason it grows faster than a person's play would explain,
// and the reason the handoff has to warn Josh to check the timestamp before
// reading a launch failure out of its tail.
//
// 8 MiB with ONE generation behind it bounds the pair at ~16 MiB plus whatever
// the current session writes. Small enough to open in an editor, large enough to
// hold several launches, which is what makes "the crash is in the log" a usable
// sentence.
const logMaxBytes = 8 << 20

// OpenLogFile opens (creating the directory) the append-mode log the game tees
// its output into. main.go sets it up BEFORE the engine starts, so a panic's
// stack lands on disk even when a double-clicked Windows exe has had its own
// console freed by ebiten's hideconsole -- the exe is console-subsystem, so
// Windows allocates a console that hideconsole frees, and stderr (and a crash's
// stack) then goes nowhere (audit B6, 12 Sep 2026; §0(b) measured the subsystem).
// It returns nil on any failure -- a missing log must never stop the game.
//
// ROTATION HAPPENS HERE, at the moment of a launch, and nowhere else. A size
// check on every write would need a wrapper around the writer main.go tees
// through, and the writer is also what d2term's BindLogger wraps -- one more
// thing between a panic and the disk, on the path whose whole job is to survive
// a panic. A launch is a natural boundary, and because the harness launches a
// fresh process per script the check runs often enough to bound the file.
func OpenLogFile() *os.File {
	if err := os.MkdirAll(DataDir(), 0o750); err != nil {
		return nil
	}

	rotateLogIfLarge()

	f, err := os.OpenFile(LogFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}

	return f
}

// rotateLogIfLarge moves an oversize log aside, keeping exactly one generation.
//
// EVERY FAILURE IS IGNORED ON PURPOSE. This runs before the game has a log to
// complain into, and the one thing it must never do is stop a launch. The
// realistic failure is Windows refusing to rename a file another process still
// has open -- the game and a harness launch share this path -- and the right
// answer there is to append to the oversize file and rotate on some later
// launch, not to refuse to start.
func rotateLogIfLarge() {
	info, err := os.Stat(LogFilePath())
	if err != nil || info.Size() < logMaxBytes {
		return
	}

	// Remove the older generation first: os.Rename replaces the destination on
	// Unix but not on every Windows configuration, and a failed rename here
	// would leave the live log oversize forever.
	_ = os.Remove(PreviousLogFilePath())
	_ = os.Rename(LogFilePath(), PreviousLogFilePath())
}
