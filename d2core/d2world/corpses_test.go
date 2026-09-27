package d2world

import "testing"

func TestCorpsesFallCloseAndCount(t *testing.T) {
	delta := 0
	c := NewCorpses(func(row string) bool { return row == "opportunists" }, nil, func(d int) { delta += d })

	man := c.Fall("n:1", "opportunists", 10, 10)
	dog := c.Fall("n:2", "dogs", 12, 10)
	placed := c.FallHuman("dead:1", "", 20, 20)

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
	c := NewCorpses(nil, nil, nil)
	c.FallHuman("a", "", 0, 0)
	c.FallHuman("b", "", 1, 0)
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

// Josh's ruling of 27 Sep 2026: a body carries what the man was in life. A
// slain man's is his row's live name, which the game gives the registry; the
// placed dead and the wanderers are given theirs. A man who rises keeps it --
// his fall, his standing again -- and every member he walks as finds his body.
func TestCorpsesWasAndBodyOf(t *testing.T) {
	names := map[string]string{"opportunists": "Opportunist", "dogs": "Feral dog"}
	c := NewCorpses(func(row string) bool { return row == "opportunists" }, func(row string) string { return names[row] }, nil)

	man := c.Fall("n:1", "opportunists", 10, 10)
	placed := c.FallHuman("dead:1", "A fallen soldier", 20, 20)
	nameless := c.Fall("n:2", "no-such-row", 0, 0)

	if man.Was != "Opportunist" || placed.Was != "A fallen soldier" || nameless.Was != "" {
		t.Fatalf("what they were: %q %q %q", man.Was, placed.Was, nameless.Was)
	}

	if _, ok := c.BodyOf("n:1"); ok {
		t.Fatal("a body that never rose walks as no one")
	}

	// He rises, is cut down, and stands again as a new member.
	c.Rise("n:1")
	c.Raised("n:1", "risen:a")
	c.Fall("risen:a", RisenRow, 11, 11)
	c.Rise("n:1")
	c.Raised("n:1", "risen:b")

	for _, member := range []string{"risen:a", "risen:b"} {
		b, ok := c.BodyOf(member)
		if !ok || b.ID != "n:1" || b.Was != "Opportunist" {
			t.Fatalf("%s walks as the opportunist's body, still what he was: %+v %v", member, b, ok)
		}
	}

	for _, raw := range c.HarnessState()["bodies"].([]map[string]interface{}) {
		if raw["id"] == "n:1" && (raw["was"] != "Opportunist" || raw["walks_as"] != "risen:b") {
			t.Fatalf("the provider reports what he was and whom he walks as: %v", raw)
		}
	}

	// THE CONTROL: with no name given, a slain man's body says nothing -- the
	// name above came from the game's function, not from the row itself.
	bare := NewCorpses(nil, nil, nil)
	if b := bare.Fall("n:1", "opportunists", 0, 0); b.Was != "" {
		t.Fatalf("no function, no name: %q", b.Was)
	}
}
