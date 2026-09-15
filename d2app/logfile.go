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

// OpenLogFile opens (creating the directory) the append-mode log the game tees
// its output into. main.go sets it up BEFORE the engine starts, so a panic's
// stack lands on disk even when a double-clicked Windows exe has had its own
// console freed by ebiten's hideconsole -- the exe is console-subsystem, so
// Windows allocates a console that hideconsole frees, and stderr (and a crash's
// stack) then goes nowhere (audit B6, 12 Sep 2026; §0(b) measured the subsystem).
// It returns nil on any failure -- a missing log must never stop the game.
func OpenLogFile() *os.File {
	if err := os.MkdirAll(DataDir(), 0o750); err != nil {
		return nil
	}

	f, err := os.OpenFile(LogFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}

	return f
}
