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
