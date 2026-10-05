//go:build windows

package d2report

import "os"

// claim takes a marker whose owner is gone, returning the path it was moved
// to. On Windows the rename IS the test: the owner holds its marker open for
// its whole run, and Go opens files without FILE_SHARE_DELETE, so the rename
// fails while it lives -- however it was ended, and whatever pid it had.
func claim(path string, _ Marker) (string, bool) {
	claimed := path + ".claimed"
	if err := os.Rename(path, claimed); err != nil {
		return "", false
	}

	return claimed, true
}
