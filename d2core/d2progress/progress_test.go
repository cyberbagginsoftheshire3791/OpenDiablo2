package d2progress

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func shipped(t *testing.T) *Tree {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "data", "strigoi", "talents.json"))
	if err != nil {
		t.Fatal(err)
	}

	tree, err := Load(data)
	if err != nil {
		t.Fatalf("the shipped talent table must load: %v", err)
	}

	return tree
}

func TestTheShippedTreeIsTheSignedShape(t *testing.T) {
	tree := shipped(t)

	if len(tree.Branches) != 3 {
		t.Fatalf("three branches (Endurance, Drill, Lore); got %d", len(tree.Branches))
	}

	for _, b := range tree.Branches {
		if len(b.Nodes) != 5 {
			t.Fatalf("%s: five nodes; got %d", b.Name, len(b.Nodes))
		}
	}

	// [8-10] picks: nine levels past the first, so nine picks from fifteen.
	if picks := tree.maxLevel() - 1; picks < 8 || picks > 10 {
		t.Fatalf("the ruled [8-10] picks; this table gives %d", picks)
	}
}

func TestLoadRefusesAnEffectNothingReads(t *testing.T) {
	doc := `{"xp":{"slain":{"":1},"routed_factor":0.5,"night":1},"levels":[0,10],
	"branches":[{"id":"a","name":"A","nodes":[{"id":"x","name":"X","text":"","effects":{"sparkle":1}}]},
	{"id":"b","name":"B","nodes":[{"id":"y","name":"Y","text":"","effects":{"move_tiles":1}}]},
	{"id":"c","name":"C","nodes":[{"id":"z","name":"Z","text":"","effects":{"move_tiles":1}}]}]}`

	if _, err := Load([]byte(doc)); err == nil {
		t.Fatal("a talent whose effect nothing reads must be refused")
	}

	// THE CONTROL: the same table with a wired effect loads.
	ok := `{"xp":{"slain":{"":1},"routed_factor":0.5,"night":1},"levels":[0,10],
	"branches":[{"id":"a","name":"A","nodes":[{"id":"x","name":"X","text":"","effects":{"move_tiles":1}}]},
	{"id":"b","name":"B","nodes":[{"id":"y","name":"Y","text":"","effects":{"move_tiles":1}}]},
	{"id":"c","name":"C","nodes":[{"id":"z","name":"Z","text":"","effects":{"move_tiles":1}}]}]}`

	if _, err := Load([]byte(ok)); err != nil {
		t.Fatalf("a wired effect loads: %v", err)
	}
}

func TestLevelsAndPicks(t *testing.T) {
	tree := shipped(t)
	p := &Progress{}

	if p.Level(tree) != 1 || p.PicksWaiting(tree) != 0 {
		t.Fatal("a new hero is level 1 with nothing to pick")
	}

	if up := p.Gain(tree, 49); up != 0 {
		t.Fatalf("49 XP crosses no level; crossed %d", up)
	}

	if up := p.Gain(tree, 1); up != 1 || p.PicksWaiting(tree) != 1 {
		t.Fatalf("50 XP is level 2 and one pick: crossed %d, waiting %d", up, p.PicksWaiting(tree))
	}

	// Past the top the level stops and so do the picks.
	p.Gain(tree, 100000)

	if p.Level(tree) != tree.maxLevel() || p.PicksWaiting(tree) != tree.maxLevel()-1 {
		t.Fatalf("capped at %d with %d picks; got %d and %d",
			tree.maxLevel(), tree.maxLevel()-1, p.Level(tree), p.PicksWaiting(tree))
	}

	if tree.NextAt(p.XP) != -1 {
		t.Fatal("no next level past the top")
	}
}

func TestPickRules(t *testing.T) {
	tree := shipped(t)
	p := &Progress{}

	if err := p.Pick(tree, "long-marches"); !errors.Is(err, ErrNoPick) {
		t.Fatalf("no pick before a level: %v", err)
	}

	p.Gain(tree, 120) // level 3: two picks

	if err := p.Pick(tree, "hard-flesh"); !errors.Is(err, ErrLocked) {
		t.Fatalf("rank 2 before rank 1: %v", err)
	}

	if err := p.Pick(tree, "long-marches"); err != nil {
		t.Fatal(err)
	}

	if err := p.Pick(tree, "long-marches"); !errors.Is(err, ErrTaken) {
		t.Fatalf("twice: %v", err)
	}

	if err := p.Pick(tree, "hard-flesh"); err != nil {
		t.Fatalf("rank 2 after rank 1: %v", err)
	}

	if err := p.Pick(tree, "night-eyes"); !errors.Is(err, ErrNoPick) {
		t.Fatalf("both picks spent: %v", err)
	}

	if err := p.Pick(tree, "no-such"); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestEffectsCombine(t *testing.T) {
	tree := shipped(t)
	p := &Progress{}

	if p.Effect(tree, FatigueRate) != 1 || p.Effect(tree, MaxHealth) != 0 {
		t.Fatal("with no talents, multipliers are 1 and sums are 0")
	}

	p.Gain(tree, 100000)

	for _, id := range []string{"long-marches", "hard-flesh", "iron-nerve", "lean-months", "unbroken"} {
		if err := p.Pick(tree, id); err != nil {
			t.Fatal(err)
		}
	}

	if got := p.Effect(tree, MaxHealth); got != 70 {
		t.Fatalf("Hard Flesh and Unbroken add 70 health; got %v", got)
	}

	if got := p.Effect(tree, FatigueRate); got != 0.85 {
		t.Fatalf("Long Marches: fatigue x0.85; got %v", got)
	}

	// A talent the tree has since dropped contributes nothing.
	p.Talents = append(p.Talents, "retired-talent")

	if got := p.Effect(tree, MaxHealth); got != 70 {
		t.Fatalf("an unknown talent adds nothing; got %v", got)
	}
}

func TestXPValues(t *testing.T) {
	tree := shipped(t)

	if tree.SlainXP("wolves") != 15 || tree.SlainXP("something-else") != 5 {
		t.Fatal("slain XP by row, with a fallback")
	}

	if tree.RoutedXP("boar") != 10 {
		t.Fatalf("routed is half: %d", tree.RoutedXP("boar"))
	}
}

func TestLoadRefusesAFourthBranch(t *testing.T) {
	branch := func(id string) string {
		return `{"id":"` + id + `","name":"` + id + `","nodes":[{"id":"` + id + `1","name":"N","text":"","effects":{"move_tiles":1}}]}`
	}

	head := `{"xp":{"slain":{"":1},"routed_factor":0.5,"night":1},"levels":[0,10],"branches":[`

	if _, err := Load([]byte(head + branch("a") + "," + branch("b") + "," + branch("c") + "," + branch("d") + `]}`)); err == nil {
		t.Fatal("four branches must be refused: the panel draws three")
	}

	if _, err := Load([]byte(head + branch("a") + "," + branch("b") + "," + branch("c") + `]}`)); err != nil {
		t.Fatalf("three load: %v", err)
	}
}
