package d2player

import "fmt"

// Strigoi-owned display strings. This is the one place the game's own
// hand-authored English lives — the strings that are NOT keys in a D2 string
// table and cannot be, because they are ours: the clock strip's month names
// and its "time to sunset" label. Hardcoded English has precedent
// (game.go:404, "Getting Zone Information"), and the localisation ratchet
// (Article V.2) has a single file to move these from when a Strigoi string
// table exists.
//
// The feast and moon-phase names the strip also shows are NOT here: they are
// GENERATED into the day table in d2core/d2world (Article II.5), so they live
// in the system that produces them rather than being retyped as UI strings.

// strigoiStripSeparator joins the clock strip's fields. ASCII on purpose:
// certain of Font16 glyph coverage, and it measured narrower than a middle dot
// (§0: 686px vs 730px for the widest line).
const strigoiStripSeparator = " - "

// strigoiMonthNames indexes the Julian calendar's months from 1 (January);
// index 0 is unused so month numbers map directly.
//
// nolint:gochecknoglobals // a constant lookup table
var strigoiMonthNames = [...]string{
	"", "January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}

// strigoiMonthName returns the English month name, or "" for a month out of
// range [1,12].
func strigoiMonthName(month int) string {
	if month < 1 || month >= len(strigoiMonthNames) {
		return ""
	}

	return strigoiMonthNames[month]
}

// strigoiSunsetLabel formats the time-to-sunset readout (S1 §3.4: world hours
// to one decimal). The clock counts to its own DuskStart, so this never shows
// a negative (see Clock.HoursToDusk).
func strigoiSunsetLabel(hoursToDusk float64) string {
	return fmt.Sprintf("Sunset in %.1fh", hoursToDusk)
}

// The M4.4c-1 squad sheet and overhead cues. The stage names (S1 §5) and the
// sheet's labels are ours, so they live here beside the clock strip's month
// names (Article V.2). The cue KEYS ("hungry", ...) are the machine contract
// the Squads owner and the overhead marks share; these are their display
// strings. ASCII throughout, Font16 measured in §0 part 1.
//
// nolint:gochecknoglobals // constant lookup tables
var strigoiCueLabels = map[string]string{
	"hungry":      "Hungry",
	"thirsty":     "Thirsty",
	"no_reaction": "No reaction",
	"shaken":      "Shaken",
	"dying":       "Dying",
}

// strigoiCueLabel returns a stage cue's display string, or the key unchanged if
// it is one this file does not name.
func strigoiCueLabel(cue string) string {
	if s, ok := strigoiCueLabels[cue]; ok {
		return s
	}

	return cue
}

// nolint:gochecknoglobals // constant lookup table
var strigoiStanceLabels = map[string]string{
	"idle":   "Idle",
	"labour": "Labouring",
	"watch":  "On watch",
	"forage": "Foraging",
}

// strigoiStanceLabel returns a squad stance's display string.
func strigoiStanceLabel(stance string) string {
	if s, ok := strigoiStanceLabels[stance]; ok {
		return s
	}

	return stance
}

// T1, the tactical layer's words (23 Sep 2026). The panel's lines and the
// refusals a click in a paced fight can earn. Exported because the game screen
// raises the refusals and this file is the one home for Strigoi's English.
const (
	TacticalRoundYours  = "Round %d  -  YOUR TURN"
	TacticalRoundTheirs = "Round %d  -  they move"
	TacticalPipMove     = "MOVE"
	TacticalPipAction   = "ACTION"
	TacticalPipReaction = "REACTION"
	TacticalPipReady    = "ready"
	TacticalPipSpent    = "spent"
	TacticalKeys        = "click tile: move   click foe / F: strike   L: torch   E: end turn"
	TacticalYou         = "You"
	TacticalSomething   = "Something"
	TacticalRiposte     = "Riposte!"
	TacticalSlain       = "slain"

	TacticalNotYourTurn = "Not your turn."
	TacticalMoveSpent   = "Your Move is spent. Strike (F) or end the turn (E)."
	TacticalActionSpent = "Your Action is spent. End the turn (E)."
	TacticalTileTaken   = "Something is standing there."
	TacticalNoWay       = "No way through."
	TacticalTooFar      = "Too far: %d tiles, and your Move is %d."
	TacticalOutOfReach  = "Out of reach."
)
