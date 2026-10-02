//go:build harness

package d2app

import (
	"strings"
	"testing"
)

func TestStrigoiIsTheDefault(t *testing.T) {
	own := launchChoice{Map: defaultMap, Hero: defaultHero, Fonts: defaultFonts, Strings: defaultStrings}

	for name, c := range map[string]struct {
		classic bool
		given   map[string]string
		want    launchChoice
	}{
		"no switches: Strigoi's own": {false, nil, own},
		"-classic: Diablo II's":      {true, nil, launchChoice{}},
		"-classic -fonts ours": {true, map[string]string{"fonts": "data/strigoi/fonts/fonts.json"},
			launchChoice{Fonts: "data/strigoi/fonts/fonts.json"}},
		"-map diablo, the rest ours": {false, map[string]string{"map": "Diablo"},
			launchChoice{Hero: defaultHero, Fonts: defaultFonts, Strings: defaultStrings}},
		"the harness's old words": {false, map[string]string{"map": "generated", "hero": "composite"},
			launchChoice{Fonts: defaultFonts, Strings: defaultStrings}},
		"an explicit empty value is Diablo II's": {false, map[string]string{"strings": " "},
			launchChoice{Map: defaultMap, Hero: defaultHero, Fonts: defaultFonts}},
		"another map of ours": {false, map[string]string{"map": "data/strigoi/maps/other.tmj"},
			launchChoice{Map: "data/strigoi/maps/other.tmj", Hero: defaultHero, Fonts: defaultFonts, Strings: defaultStrings}},
	} {
		if got := resolveLaunch(c.classic, c.given); got != c.want {
			t.Errorf("%s: %+v, want %+v", name, got, c.want)
		}
	}
}

// -classic's help says what the flag has meant since the tables burst: not
// only Diablo II's map, art, fonts and words but its rules and tables -- and
// that the right button casts there, where Strigoi's is the torch (the
// review, 27 Sep 2026).
//
// Negative control (27 Sep 2026): give the flag its old help ("Diablo II's
// generated Act 1, class art, fonts and words ...") and this fails.
func TestClassicHelpSaysItIsDiabloIIsRules(t *testing.T) {
	for _, want := range []string{"generated Act 1", "rules and tables", "experience", "right button", "torch"} {
		if !strings.Contains(classicFlagHelp, want) {
			t.Errorf("-classic's help never says %q: %s", want, classicFlagHelp)
		}
	}
}

// TestTheShippedViewIsHalfZoomWithFog (F5, 2 Oct 2026): with no -zoom the
// game starts at 0.5, and at 1.0 under -classic (Diablo II's art and UI); a
// -zoom given explicitly wins either way, -zoom 1 being the opt-out; and -fog
// defaults on (-fog=false is its opt-out; -classic's fence is the game
// screen's, TestTheClassicFenceKeepsFogOff).
//
// Negative controls (2 Oct 2026): defaultZoom back to 1.0 and the first case
// fails; resolveZoom ignoring -classic and the second fails; defaultFog false
// and the last check fails (wt-fog5\nc\).
func TestTheShippedViewIsHalfZoomWithFog(t *testing.T) {
	for name, c := range map[string]struct {
		classic, given bool
		zoom, want     float64
	}{
		"no switches: Strigoi's 0.5":     {false, false, defaultZoom, 0.5},
		"-classic: Diablo II's 1.0":      {true, false, defaultZoom, 1},
		"-zoom 1: the opt-out":           {false, true, 1, 1},
		"-classic -zoom 0.5: given wins": {true, true, 0.5, 0.5},
		"-zoom 0.8":                      {false, true, 0.8, 0.8},
	} {
		if got := resolveZoom(c.classic, c.given, c.zoom); got != c.want {
			t.Errorf("%s: zoom %v, want %v", name, got, c.want)
		}
	}

	if !defaultFog {
		t.Error("-fog defaults off; since F5 the shipped game has fog (-fog=false is the opt-out)")
	}
}
