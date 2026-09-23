package d2world

import "testing"

func TestCorpsesFallCloseAndCount(t *testing.T) {
	delta := 0
	c := NewCorpses(func(row string) bool { return row == "opportunists" }, func(d int) { delta += d })

	man := c.Fall("n:1", "opportunists", 10, 10)
	dog := c.Fall("n:2", "dogs", 12, 10)
	placed := c.FallHuman("dead:1", 20, 20)

	if man.Class != CorpseHuman || dog.Class != CorpseBeast || placed.Class != CorpseHuman {
		t.Fatalf("classes by row (Q1a): %s %s %s", man.Class, dog.Class, placed.Class)
	}

	if c.Open() != 3 || delta != 3 {
		t.Fatalf("three open bodies, a carcass among them: %d (delta %d)", c.Open(), delta)
	}

	c.Fall("n:1", "opportunists", 0, 0) // twice
	if c.Open() != 3 || delta != 3 {
		t.Fatal("a body falls once")
	}

	if !c.Close("n:1") || c.Open() != 2 || delta != 2 {
		t.Fatalf("the stake closes one: %d (delta %d)", c.Open(), delta)
	}

	// THE CONTROL: a closed body does not close again, and nothing unknown does.
	if c.Close("n:1") || c.Close("n:9") || delta != 2 {
		t.Fatal("close is once, and only for a body that fell")
	}
}

func TestCorpsesNearest(t *testing.T) {
	c := NewCorpses(nil, nil)
	c.FallHuman("a", 0, 0)
	c.FallHuman("b", 1, 0)
	c.Fall("dog", "dogs", 0.2, 0)

	human := func(b *Corpse) bool { return b.Class == CorpseHuman && b.State == CorpseFresh }

	if got := c.Nearest(0.9, 0, 1.5, human); got == nil || got.ID != "b" {
		t.Fatalf("nearest open man: %v", got)
	}

	if got := c.Nearest(0.1, 0, 1.5, human); got == nil || got.ID != "a" {
		t.Fatalf("the carcass is nearer but not a man: %v", got)
	}

	if got := c.Nearest(5, 5, 1.5, human); got != nil {
		t.Fatalf("nothing within reach: %v", got)
	}
}
