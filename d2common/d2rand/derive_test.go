package d2rand

import (
	"math"
	"math/rand"
	"testing"
)

// deriveSeeds are game seeds chosen to reach every branch of Effective: zero,
// negatives, exact multiples of the modulus (residue zero), one past it, the
// seeds the playtests use, a wall-clock-sized seed above 2^53, and both ends
// of int64.
var deriveSeeds = []int64{
	0, 1, -1, 99, 1462, 2026,
	int32max, 2 * int32max, -int32max, int32max + 1, int32max - 1,
	1<<53 + 1, 1790000000123456789,
	math.MaxInt64, math.MinInt64, math.MinInt64 + 1,
}

var streamNames = []string{StreamSpawns, StreamCombat, StreamRising}

// firstValues is what a plain stdlib stream at seed hands out first.
func firstValues(seed int64) [3]int64 {
	r := rand.New(rand.NewSource(seed)) // nolint:gosec // test

	return [3]int64{r.Int63(), r.Int63(), r.Int63()}
}

// Effective is only worth anything if it IS the stdlib's reduction: a seed and
// its Effective value must be one stream, judged against the installed
// math/rand rather than against this package's reading of it.
func TestDeriveEffectiveIsTheStdlibsSeed(t *testing.T) {
	for _, seed := range deriveSeeds {
		eff := Effective(seed)
		if eff < 1 || eff > int32max-1 {
			t.Fatalf("Effective(%d) = %d, outside [1, 2^31-2]", seed, eff)
		}

		if firstValues(seed) != firstValues(eff) {
			t.Fatalf("seed %d and its Effective %d must be one stream", seed, eff)
		}
	}

	// The control: two seeds a modulus apart are one stream, so Effective must
	// map them together -- and a seed one apart is a different stream.
	if firstValues(1462) != firstValues(1462+int32max) || Effective(1462) != Effective(1462+int32max) {
		t.Fatal("seeds 2^31-1 apart are one math/rand stream and one Effective seed")
	}

	if firstValues(1462) == firstValues(1463) {
		t.Fatal("the control: adjacent seeds must be different streams")
	}
}

// C6, the point of Derive: for every game seed, the world stream (the game
// seed itself) and the three derived streams are four DIFFERENT streams --
// different effective seeds, so a restore that swapped two of them would show.
func TestDeriveStreamsNeverCoincide(t *testing.T) {
	offsets := map[int64]string{}

	for _, name := range streamNames {
		off := streamOffset(name)
		if off < 1 || off > int32max-2 {
			t.Fatalf("offset of %q is %d, outside [1, 2^31-3]", name, off)
		}

		if other, dup := offsets[off]; dup {
			t.Fatalf("streams %q and %q share offset %d, so they would share every seed", name, other, off)
		}

		offsets[off] = name
	}

	for _, seed := range deriveSeeds {
		seen := map[int64]string{Effective(seed): "world"}
		firsts := map[[3]int64]string{firstValues(seed): "world"}

		for _, name := range streamNames {
			d := Derive(seed, name)
			if d < 1 || d > int32max-1 || Effective(d) != d {
				t.Fatalf("Derive(%d, %q) = %d is not its own effective seed in [1, 2^31-2]", seed, name, d)
			}

			if other, dup := seen[d]; dup {
				t.Fatalf("game seed %d: %q and %q run on one effective seed %d", seed, name, other, d)
			}

			seen[d] = name

			v := firstValues(d)
			if other, dup := firsts[v]; dup {
				t.Fatalf("game seed %d: %q and %q hand out the same first values", seed, name, other)
			}

			firsts[v] = name
		}
	}
}

// The derived seeds are part of every seeded fight and night, so they are
// pinned: a change to the hash or the ring moves every measured outcome, and
// must be a decision rather than an accident.
func TestDeriveIsFixed(t *testing.T) {
	for _, c := range []struct {
		seed   int64
		stream string
		want   int64
	}{
		{1462, StreamSpawns, 443254809},
		{1462, StreamCombat, 179919329},
		{1462, StreamRising, 1368529044},
		{99, StreamSpawns, 443253446},
	} {
		if got := Derive(c.seed, c.stream); got != c.want {
			t.Errorf("Derive(%d, %q) = %d, pinned %d", c.seed, c.stream, got, c.want)
		}
	}
}
