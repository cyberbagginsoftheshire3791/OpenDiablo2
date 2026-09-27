package d2gamescreen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2bestiary"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// M4.7 step 4: the priest closes a man's hasty grave within the radius, and
// nothing else -- a fresh body, a carcass's grave, a closed one, or a grave a
// step beyond the church's reach.
func TestRiteCloses(t *testing.T) {
	grave := d2world.Corpse{State: d2world.CorpseHasty, Class: d2world.CorpseHuman}

	cases := []struct {
		name string
		b    d2world.Corpse
		dist float64
		want bool
	}{
		{"a man's grave in reach", grave, 12, true},
		{"a man's grave a step too far", grave, 12.5, false},
		{"an open body", d2world.Corpse{State: d2world.CorpseFresh, Class: d2world.CorpseHuman}, 1, false},
		{"a carcass in a grave", d2world.Corpse{State: d2world.CorpseHasty, Class: d2world.CorpseBeast}, 1, false},
		{"already closed", d2world.Corpse{State: d2world.CorpseClosed, Class: d2world.CorpseHuman}, 1, false},
	}

	for _, c := range cases {
		if got := riteCloses(c.b, c.dist, 12); got != c.want {
			t.Errorf("%s: riteCloses = %v, want %v", c.name, got, c.want)
		}
	}
}

// A staking is seen by day within the radius; not at night, not out of it.
func TestSeenBy(t *testing.T) {
	cases := []struct {
		name  string
		night bool
		dist  float64
		want  bool
	}{
		{"by day, near", false, 8, true},
		{"by day, a step too far", false, 8.5, false},
		{"at night, near", true, 1, false},
	}

	for _, c := range cases {
		if got := seenBy(c.night, c.dist, 8); got != c.want {
			t.Errorf("%s: seenBy = %v, want %v", c.name, got, c.want)
		}
	}
}

// testDeadBestiary is a bestiary with the men's row, the dead's row and the
// dead's words, none of them the shipped text -- the rule is under test here,
// not the data.
const testDeadBestiary = `{
  "creatures": [
    {"id":"man","name":"A man of the road","spawn_row":"opportunists","stand_in":"fallen1","animations":{"idle":"/m.png"},"max_health":1},
    {"id":"dead","name":"one of the dead","spawn_row":"risen","stand_in":"fallen1","animations":{"idle":"/d.png"},"max_health":1}
  ],
  "the_dead_were": {"placed_dead": "a comrade", "wanderer": "nobody he knows"}
}`

func testDeadNames(t *testing.T) deadNames {
	t.Helper()

	catalog, err := d2bestiary.Load([]byte(testDeadBestiary))
	if err != nil {
		t.Fatal(err)
	}

	names, err := deadNamesFrom(catalog)
	if err != nil {
		t.Fatal(err)
	}

	return names
}

// Josh's ruling of 27 Sep 2026, as DeadName's table: before the priest's tale
// a risen man is called what he was in life -- what his body says, else his
// row's live name, else a stranger -- and after it, the dead. The living are
// not the dead. And before the tale, nothing is ever called the dead's name.
func TestDeadName(t *testing.T) {
	n := testDeadNames(t)

	placed := d2world.Corpse{ID: "dead:4", Class: d2world.CorpseHuman, Was: "a comrade"}
	wanderer := d2world.Corpse{ID: "wanderer:1", Class: d2world.CorpseHuman, Was: "nobody he knows"}
	slain := d2world.Corpse{ID: "n:7", Row: "opportunists", Class: d2world.CorpseHuman, Was: "A man of the road"}
	unnamedMan := d2world.Corpse{ID: "n:8", Row: "opportunists", Class: d2world.CorpseHuman}
	unnamedDead := d2world.Corpse{ID: "n:9", Row: d2world.RisenRow, Class: d2world.CorpseHuman}

	cases := []struct {
		name    string
		risen   bool
		knows   bool
		body    d2world.Corpse
		hasBody bool
		want    string
	}{
		{"one of Night 1's dead, before the tale", true, false, placed, true, "a comrade"},
		{"a wanderer, before the tale", true, false, wanderer, true, "nobody he knows"},
		{"a man he slew, before the tale", true, false, slain, true, "A man of the road"},
		{"a slain man whose body kept no name: his row's", true, false, unnamedMan, true, "A man of the road"},
		{"a body of the dead's own row: never the dead's name", true, false, unnamedDead, true, "nobody he knows"},
		{"a risen man with no body: a stranger", true, false, d2world.Corpse{}, false, "nobody he knows"},
		{"Night 1's dead, after the tale", true, true, placed, true, "one of the dead"},
		{"a wanderer, after the tale", true, true, wanderer, true, "one of the dead"},
		{"a man he slew, after the tale", true, true, slain, true, "one of the dead"},
		{"the living, before", false, false, slain, true, ""},
		{"the living, after", false, true, slain, true, ""},
	}

	for _, c := range cases {
		got := n.name(c.risen, c.knows, c.body, c.hasBody)
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}

		if !c.knows && got == n.known {
			t.Errorf("%s: before the tale he is called %q, the dead's own name", c.name, got)
		}
	}
}

// The dead's words come from the bestiary, and a bestiary that cannot say
// what a risen man was before the tale -- or says it with the dead's own name
// -- stops the screen rather than show that name.
func TestDeadNamesFrom(t *testing.T) {
	n := testDeadNames(t)
	if n.known != "one of the dead" || n.were.PlacedDead != "a comrade" || n.were.Wanderer != "nobody he knows" {
		t.Fatalf("deadNamesFrom = %+v", n)
	}

	if got := n.rowName("OPPORTUNISTS"); got != "A man of the road" {
		t.Fatalf("a row's live name is its creature's: %q", got)
	}

	if got := n.rowName(d2world.RisenRow); got != "" {
		t.Fatalf("what a man became is not what he was: %q", got)
	}

	for name, doc := range map[string]string{
		"no the_dead_were": strings.Replace(testDeadBestiary,
			`,
  "the_dead_were": {"placed_dead": "a comrade", "wanderer": "nobody he knows"}`, "", 1),
		"no risen creature": strings.Replace(testDeadBestiary, `"spawn_row":"risen",`, "", 1),
		"the dead's name reused": strings.Replace(testDeadBestiary,
			`"wanderer": "nobody he knows"`, `"wanderer": "one of the dead"`, 1),
	} {
		if doc == testDeadBestiary {
			t.Fatalf("%s: the edit did not apply", name)
		}

		catalog, err := d2bestiary.Load([]byte(doc))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if _, err := deadNamesFrom(catalog); err == nil {
			t.Errorf("%s: deadNamesFrom accepted it", name)
		}
	}
}

// The shipped bestiary has the dead's words, and the game can start on it.
func TestShippedDeadNames(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "data", "strigoi", "bestiary.json"))
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := d2bestiary.Load(data)
	if err != nil {
		t.Fatal(err)
	}

	n, err := deadNamesFrom(catalog)
	if err != nil {
		t.Fatal(err)
	}

	if got := n.rowName("opportunists"); got != "Opportunist" {
		t.Fatalf("a slain opportunist was an Opportunist: %q", got)
	}
}
