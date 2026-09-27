//go:build playtest

package playtest

import (
	"os"
	"testing"
)

// TestMain removes the one harness build a run made (launcher.go,
// harnessBinary) once every test is done with it. Best effort: Windows can
// hold an exe a moment after its last game exits, and a leftover temp dir
// costs nothing.
func TestMain(m *testing.M) {
	code := m.Run()

	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}

	os.Exit(code)
}
