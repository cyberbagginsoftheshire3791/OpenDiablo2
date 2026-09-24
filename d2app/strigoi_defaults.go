package d2app

import "strings"

// STRIGOI IS THE GAME (23 Sep 2026). Josh: "I think its time. Our version can
// officially be Strigoi." With no switches the game is built from its own
// files: the authored village, the Janissary's sheets (a placeholder until the
// real art), Strigoi's fonts and Strigoi's words. Diablo II's generated Act 1,
// class art, fonts and string tables are still there, behind -classic -- the
// playtest suite runs that way, so its record stays comparable -- and any
// one of the four can be named either way on its own.
const (
	defaultMap     = "data/strigoi/maps/village.tmj"
	defaultHero    = "data/strigoi/hero/placeholder/hero.json"
	defaultFonts   = "data/strigoi/fonts/fonts.json"
	defaultStrings = "data/strigoi/strings/strings.json"

	// diabloValue names Diablo II's own for any of the four flags.
	diabloValue = "diablo"
)

// launchChoice is what a launch is built from; "" is Diablo II's.
type launchChoice struct {
	Map, Hero, Fonts, Strings string
}

// resolveLaunch picks each of the four: Strigoi's own by default, Diablo II's
// with classic; a flag given explicitly wins either way, and its value
// "diablo" (or "" -- and, as the harness has always said them, "generated"
// for the map and "composite" for the hero) is Diablo II's.
func resolveLaunch(classic bool, given map[string]string) launchChoice {
	pick := func(name, strigoi string, diabloNames ...string) string {
		v, set := given[name]
		if !set {
			if classic {
				return ""
			}

			return strigoi
		}

		v = strings.TrimSpace(v)

		for _, d := range append(diabloNames, diabloValue, "") {
			if strings.EqualFold(v, d) {
				return ""
			}
		}

		return v
	}

	return launchChoice{
		Map:     pick("map", defaultMap, "generated"),
		Hero:    pick("hero", defaultHero, "composite"),
		Fonts:   pick("fonts", defaultFonts),
		Strings: pick("strings", defaultStrings),
	}
}
