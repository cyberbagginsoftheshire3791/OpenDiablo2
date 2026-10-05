//go:build !windows

package d2report

import (
	"os"
	"syscall"
)

// claim takes a marker whose owner is gone. Unix lets a file another process
// holds open be renamed, so the pid is asked instead (signal 0). A reused pid
// keeps a dead run's marker until that process ends: a report late, never a
// game's marker taken from under it.
func claim(path string, m Marker) (string, bool) {
	if m.PID > 0 && m.PID != os.Getpid() && syscall.Kill(m.PID, 0) == nil {
		return "", false
	}

	if m.PID == os.Getpid() && runningSelf(path) {
		return "", false
	}

	claimed := path + ".claimed"
	if err := os.Rename(path, claimed); err != nil {
		return "", false
	}

	return claimed, true
}

// runningSelf is this process's own live marker.
func runningSelf(path string) bool {
	runMu.Lock()
	defer runMu.Unlock()

	return runFile != nil && runPath == path
}
