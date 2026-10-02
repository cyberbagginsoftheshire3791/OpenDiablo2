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

// THE SHIPPED VIEW (F5, 2 Oct 2026): fog of war on and the camera at 0.5.
// Josh's rulings of 1 Oct (claude/rulings-2026-10-01-camera-scale-and-fog.md):
// the game zooms out to about half size on today's 800x600 so a squad does not
// fill the screen -- the art and the world's distances unchanged -- and fog
// flips on together with the zoom, so the suite's screenshot checks are
// re-baselined once. The opt-outs are the flags' own: -fog=false and -zoom 1.
// -classic is Diablo II's game and keeps Diablo II's view: zoom 1.0 unless
// -zoom names one (fog is off there whatever -fog says: the game screen's own
// fence, as in a network game and the World Editor).
const (
	defaultZoom = 0.5  // Strigoi's view: a man ~36 px tall, ~10 tiles each way on screen
	classicZoom = 1.0  // Diablo II's view, for its own art and UI
	defaultFog  = true // Strigoi's game: black until explored
)

// resolveZoom is the zoom a launch starts at: -zoom when it was given, else
// Strigoi's 0.5, or 1.0 under -classic.
func resolveZoom(classic, given bool, zoom float64) float64 {
	switch {
	case given:
		return zoom
	case classic:
		return classicZoom
	default:
		return defaultZoom
	}
}
