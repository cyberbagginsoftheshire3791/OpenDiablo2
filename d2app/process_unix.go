//go:build unix

package d2app

import (
	"errors"
	"syscall"
)

// processAlive says whether a process with this id is running: signal 0 checks
// without sending anything. EPERM means it is there and belongs to someone
// else. See process_windows.go.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := syscall.Kill(pid, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}
