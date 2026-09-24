package d2journal

import "sort"

// THE GAME'S VOCABULARY. What the game promises to raise and report, in one
// place, so the game screen and the tests build the same lists: a name here
// that no call site raises is an entry that never appears. Each has a call
// site in d2game/d2gamescreen (grep "note(\""); the playtest (journal_test.go)
// raises node:, beast:, risen_seen_untold, dawn_breakoff, night_survived and
// watch_broken, and the rest are raised by the verbs other scripts drive.

// Events the game raises by name (Note), beside the families in GameEvents.
var baseEvents = []string{
	"rose",              // a body rose (Game.raiseTheDead)
	"reraised",          // a Downed dead man stood again (raiseTheDead's Downed branch)
	"staked",            // a stake driven, in a fight or out of one
	"graved",            // a hasty grave dug
	"dawn_breakoff",     // first light broke the dead off or laid them down (Game.firstLight)
	"risen_seen_untold", // a risen man in a fight before the priest's tale
	"slain",             // anything slain in a fight
	"routed",            // a pack routed
	"foraged",           // branches gathered
	"torch_out",         // the torch in his hand burnt out
	"night_survived",    // a night lived through (the dawn experience edge)
	"watch_kept",        // a promised watch stood (Game.dawnWatch)
	"watch_broken",      // a promised watch not stood
}

// States the game reports every frame (Facts.State).
var gameStates = []string{
	"night",    // the deep night
	"fighting", // a fight is open
	"hungry",   // the meters' warning bands
	"thirsty",
	"starving", // where neglect takes health
	"parched",
	"shaken",        // fatigue past the Shaken threshold
	"torch_lit",     // his carried torch burns
	"carries_torch", // a torch is worn or packed
	"shield",        // a kalkan is worn or packed
}

// Vocabulary is what the families of events are built from.
type Vocabulary struct {
	Rows     []string // spawn rows: beast:<row>, slain:<row>
	Recipes  []string // crafted:<recipe>
	Speakers []string // talked:<speaker> (any opening, even a refusal: key words on node:)
	Nodes    []string // node:<node>, a dialogue node reached
}

// GameEvents is every event the game can raise, sorted.
func GameEvents(v Vocabulary) []string {
	out := append([]string{}, baseEvents...)

	for _, r := range v.Rows {
		out = append(out, "beast:"+r, "slain:"+r)
	}

	for _, r := range v.Recipes {
		out = append(out, "crafted:"+r)
	}

	for _, s := range v.Speakers {
		out = append(out, "talked:"+s)
	}

	for _, n := range v.Nodes {
		out = append(out, "node:"+n)
	}

	sort.Strings(out)

	return out
}

// GameStates is every state the game reports.
func GameStates() []string { return append([]string{}, gameStates...) }
