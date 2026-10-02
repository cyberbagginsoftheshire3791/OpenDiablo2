//go:build harness

package d2app

import (
	"testing"
)

// TestOnlyAHarnessedGameOwnsTheMouse is BUG-111's line, both ways: a harness
// build installs the scripted overlay in every launch (harnessInputService),
// and only a game started with -harness gives the script the mouse
// (harnessClaimMouse, from harnessStart). A harness build run by hand --
// Josh playing it -- keeps the real mouse.
func TestOnlyAHarnessedGameOwnsTheMouse(t *testing.T) {
	savedEnabled, savedInput := harness.enabled, harness.input
	t.Cleanup(func() { harness.enabled, harness.input = savedEnabled, savedInput })

	off, on := false, true

	for _, c := range []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{"no flags registered", nil, false},
		{"run by hand, without -harness", &off, false},
		{"started with -harness", &on, true},
	} {
		harness.enabled, harness.input = c.enabled, nil

		// Installing the overlay never owns the mouse by itself.
		(&App{}).harnessInputService(nil)

		if harness.input == nil {
			t.Fatalf("%s: the overlay was not installed", c.name)
		}

		if harness.input.MouseOwned() {
			t.Fatalf("%s: installing the scripted overlay took the mouse; only harnessStart's claim may", c.name)
		}

		if got := harnessClaimMouse(); got != c.want || harness.input.MouseOwned() != c.want {
			t.Fatalf("%s: claimed %v, owned %v; want %v", c.name, got, harness.input.MouseOwned(), c.want)
		}
	}
}
