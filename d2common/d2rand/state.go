package d2rand

import (
	"errors"
	"fmt"
)

// ErrStreamState is what StreamState.Check returns for a saved stream no game
// could have written.
var ErrStreamState = errors.New("d2rand: saved stream refused")

// MaxDraws is the most draws a saved stream may claim (M4.6 B2a review, B1).
//
// Restore replays every draw, one generator step each, so the count is also
// the time a load spends on the stream. It is capped rather than trusted,
// because a file is not trusted: a count near 2^64 would hang the load for
// centuries, and one merely wrong -- a corrupted digit -- would restore a
// different stream, silently.
//
// THE NUMBER. A night draws on the order of thousands of values per stream
// (a spawn roll per table check, a few per blow, one per open body per band;
// the plan's section 8 lists draws per night as an ESTIMATE, not measured).
// Ten to the ninth is ten thousand nights at a hundred thousand draws each --
// a margin of well over an order of magnitude on the estimate, times a longer
// campaign than anyone will play. BenchmarkRestoreSkip measured the replay at
// 1.5 ns a draw on Josh's laptop (28 Sep 2026: 1.48 ms per million), so a
// stream AT the cap restores in about a second and a half, and a load's four
// streams in about six: slow, not hung. Ten to the tenth, the review's example,
// would be fifteen seconds a stream. A save that ever nears the cap is a
// finding, not a load.
const MaxDraws uint64 = 1_000_000_000

// StreamWorld names the map engine's world stream. It is the one stream that
// runs on the game seed itself (SeedFor), so the map and the digest's world
// part never moved when the others were derived (C6).
const StreamWorld = "world"

// Derived is every gameplay stream seeded through Derive, in a fixed order. It
// is the list, not a copy of it: SeedFor refuses a name that is not on it, so a
// stream added to the game must be added here before its save can be checked,
// and TestDeriveStreamsNeverCoincide iterates exactly this.
var Derived = []string{StreamSpawns, StreamCombat, StreamRising, StreamCombatClock}

// SeedFor is the seed the stream name runs on in a game seeded with
// worldSeed: the game seed itself for the world stream, Derive for the rest.
// A name on neither list is an error, never a guess.
func SeedFor(worldSeed int64, name string) (int64, error) {
	if name == StreamWorld {
		return worldSeed, nil
	}

	for _, d := range Derived {
		if d == name {
			return Derive(worldSeed, name), nil
		}
	}

	return 0, fmt.Errorf("%w: no stream is named %q (%s, or one of %v)", ErrStreamState, name, StreamWorld, Derived)
}

// StreamState is a counted stream's position as the world file carries it:
// the seed it was made from and how many values it has handed out (M4.6 B2a
// and B2b, made one type at the B2 review on 28 Sep 2026 -- B2a's b2aStream
// and B2b's b2bStream had one shape and two names).
//
// The seed is written as a JSON STRING. A wall-clock game seed (UnixNano,
// ~1.8e18) is past 2^53, where float64 stops holding every integer, and a
// reader that decodes numbers into float64 would restore a different stream
// (B1 notes, section 2). A derived seed is below 2^31 and would survive
// either way; the world stream's is the game seed and would not, so every
// stream is written one way.
type StreamState struct {
	Seed  int64  `json:"seed,string"`
	Draws uint64 `json:"draws"`
}

// StateOf is where st stands now.
func StateOf(st *Stream) StreamState {
	return StreamState{Seed: st.Seeded(), Draws: st.Draws()}
}

// Check refuses a saved stream that the game seeded with worldSeed could not
// have written for the stream called name: a seed that is not SeedFor's (a
// stream saved under another's name -- two blocks swapped -- or from another
// game), or more draws than MaxDraws.
//
// Without it a Restore took any {seed, draws} it was handed: swapping the
// rising's block with combat's restored both, each running on the other's
// dice from its first roll, and nothing noticed (the B2a review's B1).
func (s StreamState) Check(worldSeed int64, name string) error {
	want, err := SeedFor(worldSeed, name)
	if err != nil {
		return err
	}

	if s.Seed != want {
		return fmt.Errorf("%w: the %s stream is saved on seed %d, and a game seeded %d runs it on %d",
			ErrStreamState, name, s.Seed, worldSeed, want)
	}

	if s.Draws > MaxDraws {
		return fmt.Errorf("%w: the %s stream is saved at %d draws, past the %d a save may claim (MaxDraws)",
			ErrStreamState, name, s.Draws, MaxDraws)
	}

	return nil
}

// RestoreInto puts st where s says, through Stream.Restore -- which replaces
// the rand and its counted source together -- and nothing else (B1 notes,
// section 3). It does not check s: the caller's Restore has already, before
// changing anything, so a refused snapshot changes nothing.
func (s StreamState) RestoreInto(st *Stream) { st.Restore(s.Seed, s.Draws) }
