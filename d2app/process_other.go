//go:build !windows && !unix

package d2app

// processAlive cannot ask on this platform, so every process is taken to be
// running and no other game's playtest folder is ever cleared. See
// process_windows.go.
func processAlive(pid int) bool { return pid > 0 }
