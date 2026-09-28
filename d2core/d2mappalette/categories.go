package d2mappalette

import (
	"image"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// Category is a palette tab.
//
// IT HAS NO ENGINE MEANING. The loader knows a layer, a footprint and two
// booleans; nothing in d2maptiled or anywhere else calls a thing a building. A
// Category is this package's grouping for the editor's tabs, derived by
// [CategoryOf], and it is the only field on an [Entry] that is a convention
// rather than a measurement.
type Category string

// The five tabs. Buildings and Props hold art in v0; Creatures, People and
// Terrain are shown with a reason and nothing in them -- see [Tabs].
const (
	CategoryBuildings Category = "buildings"
	CategoryProps     Category = "props"
	CategoryCreatures Category = "creatures"
	CategoryPeople    Category = "people"
	CategoryTerrain   Category = "terrain"
)

func categoryOrder(c Category) int {
	for i, t := range tabDefs {
		if t.Category == c {
			return i
		}
	}

	return len(tabDefs)
}

// buildingWords and propWords are the keyword tables [CategoryOf] uses when the
// footprint cannot tell a building from a prop: a single-tile wall image may be
// a house or a tree, and only its name says which.
//
// PROP WORDS ARE CHECKED FIRST, and that ordering was earned. The Dealu
// monastery's yard well is "Dealu monastery well", which contains "monastery";
// building-words-first filed a well as a building. A prop word is the more
// specific of the two when both appear, because the prop is the object and the
// building is only its setting -- the monastery well is a well.
//
// BOUNDARY KIT -- fence, palisade, wall, gate -- is filed under Buildings, not
// Props: laying out a village perimeter is construction, and the fence ring is
// what the village's "inside" rectangle follows (data/strigoi/maps/README.md).
//
// Both tables are matched as substrings against the entry's ID and display name,
// lowercased. They are written down here rather than hidden in a switch so the
// next person can add a word without guessing the rule.
var buildingWords = []string{
	"house", "smithy", "forge", "church", "chapel", "tower", "barn", "byre",
	"mill", "stable", "granary", "tavern", "hall", "keep", "gate", "refectory",
	"cells", "cloister", "monastery", "cottage", "hut", "shed", "wall", "fence",
	"palisade",
}

var propWords = []string{
	"well", "tree", "grave", "hearth", "cart", "barrel", "trough", "bench",
	"grindstone", "stump", "rock", "bush", "log", "crate", "sack", "corpse",
	"debris", "rubble",
}

// CategoryOf groups one piece of art into a tab.
//
// The rule, in order:
//
//  1. Anything whose default layer is the FLOOR layer is Terrain. A floor tile
//     is ground to paint, and painting is the Terrain tab -- which is not in v0.
//  2. A prop word in the name wins: see [propWords] for why it goes first.
//  3. Then a building word.
//  4. Then the footprint: something covering more than one tile, and named
//     nothing either table knows, is a building by default.
//  5. Everything left is a Prop.
func CategoryOf(layer d2maptiled.Layer, footprint image.Point, id, name string) Category {
	if layer == d2maptiled.LayerFloor {
		return CategoryTerrain
	}

	hay := strings.ToLower(id + " " + name)

	for _, w := range propWords {
		if strings.Contains(hay, w) {
			return CategoryProps
		}
	}

	for _, w := range buildingWords {
		if strings.Contains(hay, w) {
			return CategoryBuildings
		}
	}

	if footprint.X > 1 || footprint.Y > 1 {
		return CategoryBuildings
	}

	return CategoryProps
}

// Tab is one tab of the palette: its title, whether it has anything in it in
// v0, and if not, exactly why.
type Tab struct {
	Category Category
	// Title is the tab's label.
	Title string
	// Available is false for a tab the editor must show and cannot fill.
	Available bool
	// Why is empty for an available tab, and for an unavailable one is the
	// measured reason, with the file and line a reader can check it against.
	Why string
	// Count is how many entries of this category the catalog holds. An
	// unavailable tab may still have a non-zero count -- Terrain does, because
	// the floor tiles are catalogued and only the painting tool is missing --
	// and the editor shows the count greyed out.
	Count int
}

// The unavailable reasons. Each was verified against HEAD d1d07434 and carries
// its citations; they are exported so a test can assert the editor is shown the
// same sentence this package documents.
const (
	// WhyNoCreatures is the Creatures tab's reason. TWO independent walls, and
	// the second is the one that matters.
	//
	// First: a .tmj cannot say "a wolf stands here". objectLayer's class switch
	// (tiled.go:930-964) accepts player_start, npc and inside, tile objects are
	// routed to structure() first (tiled.go:905-913), and the default arm
	// refuses anything else outright -- "object %d has class %q; the game reads
	// player_start, npc and inside" (tiled.go:963). There is no creature class
	// to add a property to.
	//
	// Second, and the reason this is not a five-line loader change: the only
	// place in the shipped build where a creature is given the adapter that
	// makes it hunt is gameSpawner.Spawn, which wraps each arrival in
	// chaser{entity: entity} (game.go:1740) and hands it back to d2world, whose
	// spawn tables then register it with the awareness model
	// (d2world/spawns.go:899). The other two chaser constructions, Game.Pursue
	// (game.go:1870) and Game.Watch (game.go:1894), are called from
	// d2app/harness_tools.go alone, and that file is //go:build harness -- so
	// they do not exist in a shipped binary. The debug terminal's spawnmon
	// (game.go:2318) builds a creature and never wraps it, so even a
	// hand-spawned creature is never registered. An authored creature would
	// stand inert for ever: nothing would make it notice the player, and
	// startChasesForTheAware (game.go:1283) only walks pairs the notice model
	// already holds.
	WhyNoCreatures = "A map cannot place a creature: the loader's object switch takes only " +
		"player_start, npc and inside, and refuses anything else (d2maptiled/tiled.go:963). " +
		"The deeper reason is that gameSpawner.Spawn is the only place in a shipped build that " +
		"wraps a creature in the chaser adapter (d2gamescreen/game.go:1740) for the spawn tables " +
		"to register with the awareness model (d2world/spawns.go:899) -- the other two chaser " +
		"constructions are reached only from //go:build harness code -- so an authored creature " +
		"would stand inert for ever, noticing nothing and chasing nobody."

	// WhyNoPeople is the People tab's reason.
	//
	// An npc object takes ONE property and it is a string: npcMonstat
	// (tiled.go:1079-1098) refuses any other name and refuses a non-string. The
	// value is a monstats.txt row, looked up as g.asset.Records.Monster.Stats
	// in the game. The four people in the village are Diablo II stand-ins --
	// warriv1, kashya, charsi, akara (data/strigoi/maps/village.tmj objects
	// 10-13, pinned by d2maptiled/village_test.go:50) -- because S1 section 8.1
	// keeps every villager's name a placeholder by standing decision, so
	// d2dialogue's speakers are ROLES each drawn by a D2 stand-in sprite
	// (d2core/d2dialogue/dialogue.go:13-16). Placing a new person is therefore
	// not a map edit: it needs a stand-in sprite chosen, a monstats row that
	// exists, and a role and lines in data/strigoi/dialogue.json.
	WhyNoPeople = "An npc object accepts exactly one property, the string \"monstat\" " +
		"(d2maptiled/tiled.go:1079-1098), which names a monstats.txt row. The four speakers in the " +
		"village are Diablo II stand-ins -- warriv1, kashya, charsi, akara -- because S1 section 8.1 " +
		"keeps every villager's name a placeholder by standing decision, so the speakers are ROLES " +
		"each drawn by a stand-in sprite (d2core/d2dialogue/dialogue.go:13-16). Adding a person " +
		"means a new stand-in and new dialogue in data/strigoi/dialogue.json, not just a map edit."

	// WhyNoTerrain is the Terrain tab's reason.
	//
	// Terrain painting is mechanically the smallest of the three. A tile layer
	// is a plain array of gids, never base64 and never compressed
	// (tiled.go:615-618), so painting one tile is a single write of 1+tile-id into
	// the floor layer's data array at x+y*width -- the same index the loader
	// itself reads (tileLayer, tiled.go:602-649). The floor tiles are already
	// catalogued here, so the Terrain tab has a count and no tool. It is out of
	// v0 because the editor's v0 is placing structures, and a paint tool needs
	// its own undo, brush and flood-fill before it is worth having; the
	// catalogue is ready for it.
	WhyNoTerrain = "Painting the ground is v2. The mechanism is already simple -- a tile layer is a " +
		"plain gid array, never base64 (d2maptiled/tiled.go:615-618), so painting one tile is a single " +
		"write of 1+tile-id into the floor layer's data at x+y*width, the same index the loader " +
		"reads (tiled.go:602-649). It is not in v0 because a paint tool needs its own brush, undo " +
		"and flood-fill; the floor tiles are catalogued here already and are counted in this tab."
)

var tabDefs = []Tab{
	{Category: CategoryBuildings, Title: "Buildings", Available: true},
	{Category: CategoryProps, Title: "Props", Available: true},
	{Category: CategoryTerrain, Title: "Terrain", Available: false, Why: WhyNoTerrain},
	{Category: CategoryCreatures, Title: "Creatures", Available: false, Why: WhyNoCreatures},
	{Category: CategoryPeople, Title: "People", Available: false, Why: WhyNoPeople},
}

// Tabs is the tab strip with no catalog behind it: the five tabs in display
// order, with the three unavailable ones carrying their reasons. Use
// [Catalog.Tabs] to get the same strip with counts filled in.
func Tabs() []Tab {
	out := make([]Tab, len(tabDefs))
	copy(out, tabDefs)

	return out
}
