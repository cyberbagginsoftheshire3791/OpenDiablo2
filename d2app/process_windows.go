//go:build windows

package d2app

import (
	"errors"
	"syscall"
)

// processAlive says whether a process with this id is running. It is asked
// about the process a throwaway playtest hero's folder names (playtest.go), and
// errs towards "running": a folder kept one start-up too long costs nothing, a
// folder taken from a game still playing in it costs that game its hero.
//
// Opening the process is not enough on Windows: a process that has exited can
// still be opened while anyone holds a handle to it, so its exit code is read,
// and only STILL_ACTIVE counts.
func processAlive(pid int) bool {
	const (
		processQueryLimitedInformation = 0x1000
		stillActive                    = 259
	)

	if pid <= 0 {
		return false
	}

	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		// Access denied: a process is there, and not ours to look into.
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}

	defer func() { _ = syscall.CloseHandle(h) }()

	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return true
	}

	return code == stillActive
}
