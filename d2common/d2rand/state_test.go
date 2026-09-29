package d2rand

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"
)

// The one shape every saved stream takes, round-tripped at a seed past 2^53:
// exact through a typed decode AND through a reader that decodes numbers into
// float64 (B1 notes, section 2). The world stream is written this way at the
// game seed, which a wall clock puts past 2^53.
func TestStreamStateSeedSurvivesJSON(t *testing.T) {
	const seed = int64(1)<<62 + 12345

	if seed == int64(float64(seed)) {
		t.Fatal("the control: float64 must lose this seed")
	}

	st := NewStream(seed)
	_ = st.Int63()

	raw, err := json.Marshal(StateOf(st))
	if err != nil {
		t.Fatal(err)
	}

	var typed StreamState
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatal(err)
	}

	if typed != (StreamState{Seed: seed, Draws: 1}) {
		t.Fatalf("a typed decode must be exact: got %+v from %s", typed, raw)
	}

	var loose map[string]interface{}
	if err := json.Unmarshal(raw, &loose); err != nil {
		t.Fatal(err)
	}

	if got, ok := loose["seed"].(string); !ok || got != strconv.FormatInt(seed, 10) {
		t.Fatalf("the seed must be a decimal string a float64 reader keeps exactly; got %T %v", loose["seed"], loose["seed"])
	}

	cp := NewStream(1)
	typed.RestoreInto(cp)

	if st.Int63() != cp.Int63() {
		t.Fatal("the restored stream must hand out the saved stream's next value")
	}
}

// Check takes exactly the seed SeedFor gives the named stream, and a draw
// count up to MaxDraws.
func TestStreamStateCheck(t *testing.T) {
	const world = int64(1462)

	for _, name := range append([]string{StreamWorld}, Derived...) {
		seed, err := SeedFor(world, name)
		if err != nil {
			t.Fatal(err)
		}

		if err := (StreamState{Seed: seed, Draws: MaxDraws}).Check(world, name); err != nil {
			t.Fatalf("%s at its own seed and MaxDraws must pass: %v", name, err)
		}

		if err := (StreamState{Seed: seed, Draws: MaxDraws + 1}).Check(world, name); !errors.Is(err, ErrStreamState) {
			t.Fatalf("%s one draw past MaxDraws must be refused, got %v", name, err)
		}

		if err := (StreamState{Seed: seed + 1, Draws: 3}).Check(world, name); !errors.Is(err, ErrStreamState) {
			t.Fatalf("%s one seed off must be refused, got %v", name, err)
		}

		if err := (StreamState{Seed: seed, Draws: 3}).Check(world+1, name); !errors.Is(err, ErrStreamState) {
			t.Fatalf("%s from another game's seed must be refused, got %v", name, err)
		}
	}

	if seed, _ := SeedFor(world, StreamWorld); seed != world {
		t.Fatalf("the world stream runs on the game seed itself; SeedFor says %d", seed)
	}

	if _, err := SeedFor(world, "weather"); !errors.Is(err, ErrStreamState) {
		t.Fatalf("a stream that does not exist must be refused, not derived; got %v", err)
	}
}

// THE MUTATION THE REVIEW NAMED: two blocks swapped. Every derived stream's
// saved state, offered as any OTHER stream's, is refused -- for every seed the
// Derive tests use, so it is the ring and not one lucky seed.
func TestStreamStateRefusesASwappedBlock(t *testing.T) {
	for _, world := range deriveSeeds {
		for _, saved := range Derived {
			state := StreamState{Seed: Derive(world, saved), Draws: 12}

			for _, as := range append([]string{StreamWorld}, Derived...) {
				err := state.Check(world, as)

				switch {
				case as == saved && err != nil:
					t.Fatalf("seed %d: %s's own block must pass as %s: %v", world, saved, as, err)
				case as != saved && !errors.Is(err, ErrStreamState):
					t.Fatalf("seed %d: %s's block restored as %s must be refused, got %v", world, saved, as, err)
				}
			}
		}
	}
}

// BenchmarkRestoreSkip is the cost of the replay Restore makes, per draw: the
// number MaxDraws is justified against.
func BenchmarkRestoreSkip(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Restore(1462, 1_000_000)
	}
}
