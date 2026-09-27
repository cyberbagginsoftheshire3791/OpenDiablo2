package d2app

import "strings"

// STRIGOI IS THE GAME (23 Sep 2026). Josh: "I think its time. Our version can
// officially be Strigoi." With no switches the game is built from its own
// files: the authored village, the Janissary's project sheets, Strigoi's fonts
// and Strigoi's words. Diablo II's generated Act 1,
// class art, fonts and string tables are still there, behind -classic -- the
// playtest suite runs that way, so its record stays comparable -- and any
// one of the four can be named either way on its own.
const (
	defaultMap     = "data/strigoi/maps/village.tmj"
	defaultHero    = "data/strigoi/hero/janissary/hero.json"
	defaultFonts   = "data/strigoi/fonts/fonts.json"
	defaultStrings = "data/strigoi/strings/strings.json"

	// diabloValue names Diablo II's own for any of the four flags.
	diabloValue = "diablo"
)

// launchChoice is what a launch is built from; "" is Diablo II's.
type launchChoice struct {
	Map, Hero, Fonts, Strings string
}

// classicFlagHelp is -classic's line in -h. It is more than the four
// settings below: since M5.3's tables burst (26 Sep 2026) -classic is also
// Diablo II's rules and the tables they read -- the hero's body from
// charstats.txt, his items, the experience table, the level tables, and the
// right button casting his right skill (shift-click the left) -- where
// Strigoi's game has its own and the right button is the torch. The help said
// only the four until the tables burst's review (27 Sep 2026).
const classicFlagHelp = "Diablo II's game: its generated Act 1, class art, fonts and words (each of -map, -hero, -fonts, -strings can still name Strigoi's), " +
	"and its rules and tables -- the hero's body and items, experience, the level tables, and skills cast with the right button and shift-click " +
	"(in Strigoi's game the right button is the torch)"

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
