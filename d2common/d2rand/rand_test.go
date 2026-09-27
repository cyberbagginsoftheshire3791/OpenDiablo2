package d2rand

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"testing"
)

// The seeds every test sweeps: the shipped playtest seeds, zero, a negative,
// and one past rngSource's int32max fold.
var testSeeds = []int64{1462, 99, 0, -7, 1<<31 + 5}

// next draws k values from r, Int63 for the even ones and Uint64 for the odd,
// so every comparison below exercises both of the source's counted methods.
func next(r *rand.Rand, k int) []uint64 {
	out := make([]uint64, k)

	for i := range out {
		if i%2 == 0 {
			out[i] = uint64(r.Int63())
		} else {
			out[i] = r.Uint64()
		}
	}

	return out
}

// reference is the uninterrupted stream: plain stdlib, no wrapper, no counter,
// n values thrown away and the following k returned. Every restore is judged
// against THIS, never against another counted stream, so a bug shared by the
// counter and Restore cannot agree with itself.
func reference(seed int64, n uint64, k int) []uint64 {
	r := rand.New(rand.NewSource(seed)) // nolint:gosec // test

	for i := uint64(0); i < n; i++ {
		if i%3 == 0 {
			r.Uint64()
		} else {
			r.Int63()
		}
	}

	return next(r, k)
}

func equal(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// TestRestoreThenDrawIsTheUninterruptedStream is the save's contract: restore
// at n, draw k, and the k values are draws n+1..n+k of a stream nobody
// stopped.
func TestRestoreThenDrawIsTheUninterruptedStream(t *testing.T) {
	const k = 64

	for _, seed := range testSeeds {
		for _, n := range []uint64{0, 1, 2, 7, 63, 1000, 4096} {
			want := reference(seed, n, k)

			src := Restore(seed, n)
			if src.Draws() != n || src.Seeded() != seed {
				t.Fatalf("Restore(%d, %d) reports seed %d draws %d", seed, n, src.Seeded(), src.Draws())
			}

			got := next(rand.New(src), k) // nolint:gosec // test

			if !equal(got, want) {
				t.Fatalf("seed %d: restore at %d then %d draws is not the uninterrupted stream\n got %v\nwant %v",
					seed, n, k, got[:4], want[:4])
			}

			if src.Draws() != n+k {
				t.Fatalf("seed %d: after restoring at %d and drawing %d the count is %d, want %d",
					seed, n, k, src.Draws(), n+k)
			}
		}
	}
}

// TestRestoreOffByOneDiverges is the negative control that makes the test
// above mean something: one draw short or one draw long must NOT reproduce
// the stream. If it did, the count could be wrong by one and every restore
// test would still pass.
func TestRestoreOffByOneDiverges(t *testing.T) {
	const k = 8

	for _, seed := range testSeeds {
		for _, n := range []uint64{1, 2, 7, 1000} {
			want := reference(seed, n, k)

			if got := next(rand.New(Restore(seed, n-1)), k); equal(got, want) { // nolint:gosec // test
				t.Fatalf("seed %d: restoring at %d-1 reproduced the stream at %d; an off-by-one count would go unseen", seed, n, n)
			}

			if got := next(rand.New(Restore(seed, n+1)), k); equal(got, want) { // nolint:gosec // test
				t.Fatalf("seed %d: restoring at %d+1 reproduced the stream at %d; an off-by-one count would go unseen", seed, n, n)
			}
		}
	}
}

// methodPath is one way a *rand.Rand consumer reaches the source.
type methodPath struct {
	name string
	call func(r *rand.Rand)
	// exact is how many source values ONE call consumes when that is fixed;
	// atLeast when a rejection loop can take more. One of the two is set.
	exact, atLeast uint64
}

func methodPaths() []methodPath {
	return []methodPath{
		{name: "Int63", call: func(r *rand.Rand) { r.Int63() }, exact: 1},
		// Uint64 is ONE value, not two, because Source implements Source64:
		// rand.New then calls Source64.Uint64 directly. Without that, Rand
		// would build it from two Int63 calls.
		{name: "Uint64", call: func(r *rand.Rand) { r.Uint64() }, exact: 1},
		{name: "Uint32", call: func(r *rand.Rand) { r.Uint32() }, exact: 1},
		{name: "Int31", call: func(r *rand.Rand) { r.Int31() }, exact: 1},
		{name: "Int", call: func(r *rand.Rand) { r.Int() }, exact: 1},
		{name: "Float64", call: func(r *rand.Rand) { r.Float64() }, atLeast: 1}, // resamples 1 in 2^53
		{name: "Float32", call: func(r *rand.Rand) { r.Float32() }, atLeast: 1},
		{name: "Int63n(2^40)", call: func(r *rand.Rand) { r.Int63n(1 << 40) }, exact: 1}, // a power of two masks
		{name: "Int63n(1000)", call: func(r *rand.Rand) { r.Int63n(1000) }, atLeast: 1},
		{name: "Int31n(7)", call: func(r *rand.Rand) { r.Int31n(7) }, atLeast: 1},
		{name: "Intn(1)", call: func(r *rand.Rand) { r.Intn(1) }, exact: 1},             // still draws, masks to 0
		{name: "Intn(100)", call: func(r *rand.Rand) { r.Intn(100) }, atLeast: 1},       // combat's d100
		{name: "Intn(3)", call: func(r *rand.Rand) { r.Intn(3) }, atLeast: 1},           // a pack size
		{name: "Intn(3<<30)", call: func(r *rand.Rand) { r.Intn(3 << 30) }, atLeast: 1}, // rejects often
		{name: "Perm(10)", call: func(r *rand.Rand) { r.Perm(10) }, atLeast: 10},        // one Intn per element
		{name: "Shuffle(10)", call: func(r *rand.Rand) { // combat's initiative tie-break
			r.Shuffle(10, func(i, j int) {})
		}, atLeast: 9},
		{name: "Shuffle(1)", call: func(r *rand.Rand) { r.Shuffle(1, func(i, j int) {}) }, exact: 0},
		{name: "NormFloat64", call: func(r *rand.Rand) { r.NormFloat64() }, atLeast: 1},
		{name: "ExpFloat64", call: func(r *rand.Rand) { r.ExpFloat64() }, atLeast: 1},
	}
}

// TestEveryMethodPathIsCounted drives each way a consumer can reach the
// source, then restores at the count it reported and checks the next values
// against the stream that kept going. An uncounted path would leave the
// restore behind; a double-counted one would leave it ahead. Either fails
// here -- the off-by-one control above is what guarantees that.
func TestEveryMethodPathIsCounted(t *testing.T) {
	const (
		calls = 200
		k     = 16
	)

	for _, seed := range testSeeds {
		for _, mp := range methodPaths() {
			r, src := New(seed)

			// A warm-up of the other methods first, so the path under test
			// starts mid-stream rather than at draw 0.
			next(r, 5)
			before := src.Draws()

			var minPerCall, maxPerCall uint64 = ^uint64(0), 0

			for i := 0; i < calls; i++ {
				at := src.Draws()
				mp.call(r)

				used := src.Draws() - at
				if used < minPerCall {
					minPerCall = used
				}

				if used > maxPerCall {
					maxPerCall = used
				}
			}

			switch {
			case mp.atLeast == 0 && (minPerCall != mp.exact || maxPerCall != mp.exact):
				t.Fatalf("seed %d %s: one call consumed %d..%d values, want exactly %d",
					seed, mp.name, minPerCall, maxPerCall, mp.exact)
			case mp.atLeast > 0 && minPerCall < mp.atLeast:
				t.Fatalf("seed %d %s: one call consumed as few as %d values, want at least %d",
					seed, mp.name, minPerCall, mp.atLeast)
			}

			drawn := src.Draws()

			want := next(r, k)
			got := next(rand.New(Restore(seed, drawn)), k) // nolint:gosec // test

			if !equal(got, want) {
				t.Fatalf("seed %d %s: after %d calls (%d values counted) a restore is not the stream that kept going",
					seed, mp.name, calls, drawn-before)
			}
		}
	}
}

// TestManyPathsAtOnce is the shape a night actually has: Float64 rolls, Intn
// pack sizes and Shuffle tie-breaks interleaved on one stream.
func TestManyPathsAtOnce(t *testing.T) {
	paths := methodPaths()

	for _, seed := range testSeeds {
		r, src := New(seed)

		for i := 0; i < 500; i++ {
			paths[(i*7+int(uint64(seed)%5))%len(paths)].call(r)
		}

		drawn := src.Draws()
		want := next(r, 32)

		if got := next(rand.New(Restore(seed, drawn)), 32); !equal(got, want) { // nolint:gosec // test
			t.Fatalf("seed %d: a mixed stream of %d values does not restore", seed, drawn)
		}

		if got := next(rand.New(Restore(seed, drawn-1)), 32); equal(got, want) { // nolint:gosec // test
			t.Fatalf("seed %d: the mixed stream restored one short still matched", seed)
		}
	}
}

// TestCountedStreamDrawsTheStdlibValues: wrapping must not change a single
// value, or moving spawns, combat and rising onto it would move every
// seeded measurement in the project.
func TestCountedStreamDrawsTheStdlibValues(t *testing.T) {
	for _, seed := range testSeeds {
		plain := rand.New(rand.NewSource(seed)) // nolint:gosec // test
		counted, _ := New(seed)

		for i := 0; i < 256; i++ {
			a, b := plain.Float64(), counted.Float64()
			c, d := plain.Intn(100), counted.Intn(100)

			if a != b || c != d {
				t.Fatalf("seed %d draw %d: counted stream drew %v/%d, stdlib %v/%d", seed, i, b, d, a, c)
			}
		}

		plain.Shuffle(9, func(i, j int) {})
		counted.Shuffle(9, func(i, j int) {})

		if plain.Int63() != counted.Int63() {
			t.Fatalf("seed %d: diverged after a Shuffle", seed)
		}
	}
}

// TestSeedStartsTheCountAgain: a reseed is a new stream, and the source must
// say so -- a count carried across a reseed would restore to the wrong place.
func TestSeedStartsTheCountAgain(t *testing.T) {
	r, src := New(1462)
	next(r, 10)

	r.Seed(99)

	if src.Seeded() != 99 || src.Draws() != 0 {
		t.Fatalf("after Seed(99): seed %d draws %d, want 99 and 0", src.Seeded(), src.Draws())
	}

	if got, want := next(r, 8), reference(99, 0, 8); !equal(got, want) {
		t.Fatal("a reseeded source does not draw the new seed's stream")
	}
}

// TestReportEncodes: providers put Report straight into HarnessState, which
// the digest JSON-encodes.
func TestReportEncodes(t *testing.T) {
	var nilSrc *Source

	for _, src := range []*Source{nilSrc, Restore(1462, 3)} {
		b, err := json.Marshal(src.Report())
		if err != nil {
			t.Fatalf("report does not encode: %v", err)
		}

		t.Logf("%s", b)
	}

	if got := Restore(1462, 3).Report(); got["seed"] != int64(1462) || got["draws"] != uint64(3) {
		t.Fatalf("report = %v", got)
	}
}

// TestReadIsOutsideTheDrawCount pins the exception the package comment names:
// after a Read that stops mid-value, the draw count alone cannot restore the
// byte stream. If this ever passes the other way round, the stdlib changed
// and Reader's reason for existing should be re-checked.
func TestReadIsOutsideTheDrawCount(t *testing.T) {
	r, src := New(1462)

	head := make([]byte, 3)
	_, _ = r.Read(head) // one value drawn, four of its bytes left inside r

	want := make([]byte, 4)
	_, _ = r.Read(want)

	// Neither place the count could point at -- before the value Read opened,
	// or after it -- hands out the four bytes that were still inside r.
	for _, at := range []uint64{src.Draws() - 1, src.Draws()} {
		got := make([]byte, 4)
		_, _ = rand.New(Restore(1462, at)).Read(got) // nolint:gosec // test

		if bytes.Equal(got, want) {
			t.Fatalf("a draw-count restore at %d reproduced a mid-value Read; the package comment is out of date", at)
		}
	}
}

// TestReaderIsTheStdlibByteStream: moving the uuid stream onto Reader must not
// move one entity id, so its bytes must be rand.Rand.Read's, in uuid-sized
// reads.
func TestReaderIsTheStdlibByteStream(t *testing.T) {
	for _, seed := range testSeeds {
		plain := rand.New(rand.NewSource(seed)) // nolint:gosec // test
		rd := NewReader(seed)

		for i := 0; i < 50; i++ {
			a, b := make([]byte, 16), make([]byte, 16)
			_, _ = plain.Read(a)

			if n, err := rd.Read(b); n != 16 || err != nil {
				t.Fatalf("Read = %d, %v", n, err)
			}

			if !bytes.Equal(a, b) {
				t.Fatalf("seed %d read %d: Reader %x, stdlib %x", seed, i, b, a)
			}
		}

		if rd.Bytes() != 50*16 || rd.Seeded() != seed {
			t.Fatalf("seed %d: Bytes %d Seeded %d", seed, rd.Bytes(), rd.Seeded())
		}
	}
}

// TestReaderRestores: restore at b bytes, read on, and it is the stream that
// kept going -- at every offset inside a 7-byte value, which is exactly where
// a draw count fails. One byte short is the negative control.
func TestReaderRestores(t *testing.T) {
	for _, seed := range testSeeds {
		for _, b := range []uint64{0, 1, 3, 6, 7, 8, 16, 48, 49, 50, 1000} {
			rd := NewReader(seed)
			skip := make([]byte, b)
			_, _ = rd.Read(skip)

			want := make([]byte, 23)
			_, _ = rd.Read(want)

			back := RestoreReader(seed, b)
			if back.Bytes() != b {
				t.Fatalf("RestoreReader(%d, %d).Bytes() = %d", seed, b, back.Bytes())
			}

			got := make([]byte, 23)
			_, _ = back.Read(got)

			if !bytes.Equal(got, want) {
				t.Fatalf("seed %d: restore at byte %d then 23 bytes is not the stream that kept going", seed, b)
			}

			if b == 0 {
				continue
			}

			short := make([]byte, 23)
			_, _ = RestoreReader(seed, b-1).Read(short)

			if bytes.Equal(short, want) {
				t.Fatalf("seed %d: restoring one byte short of %d still matched", seed, b)
			}
		}
	}
}
