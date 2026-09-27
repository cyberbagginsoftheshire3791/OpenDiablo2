// Package d2rand holds the counted random streams the world save rests on
// (M4.6 burst B1).
//
// Go's math/rand keeps its generator state private, so a stream cannot be
// written to a file and read back. What CAN be written is where the stream
// started and how far it has gone: a seed and a draw count. Restore rebuilds
// the stream from the seed and throws away exactly that many values, and the
// next value it hands out is the one the saved game would have drawn next.
//
// THE COUNT IS THE WHOLE CONTRACT, AND IT IS EASY TO GET WRONG BY ONE. A
// counter that misses one draw -- a method path that reaches the generator
// without passing through the counter -- restores a stream one value behind,
// and a resumed night then rolls a different pack from the night that was
// saved. So the count lives at the only place every *rand.Rand method passes
// through: the Source. rand.Rand's Int63, Uint32, Int31, Int, Int63n, Int31n,
// Intn, Float64, Float32, Perm, Shuffle, NormFloat64 and ExpFloat64 all reach
// the generator through Source.Int63 (or Source64.Uint64), and each call of
// either advances the underlying generator by exactly one step. A method that
// needs more than one value (a rejection loop, Perm, Shuffle) makes more than
// one call, and each is counted.
//
// ONE METHOD IS OUTSIDE THE CONTRACT: rand.Rand.Read. It keeps up to seven
// unread bytes of its last value inside the *rand.Rand, where no Source can
// see them, so a stream that has been Read from cannot be restored from its
// draw count. No gameplay stream calls Read. The harness's uuid stream is
// the one that does, and it is counted in BYTES by Reader, below, for exactly
// this reason.
//
// The design is the map engine's world RNG (d2mapengine/rand.go), which has
// counted its draws for the determinism digest since M3; this package is that
// wrapper made shared, with the restore it lacked.
package d2rand

import (
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"sync"
)

// Source is a rand.Source64 that remembers its seed and counts every value
// drawn from it. Hand it to rand.New; read Seeded and Draws to save it;
// rebuild it with Restore.
//
// Not safe for concurrent use, exactly like the *rand.Rand it sits under.
type Source struct {
	seed  int64
	draws uint64
	src   rand.Source64
}

var _ rand.Source64 = (*Source)(nil)

// NewSource returns a counting source seeded with seed, at draw 0.
func NewSource(seed int64) *Source {
	return &Source{seed: seed, src: newSource64(seed)}
}

// New returns a *rand.Rand drawing from a fresh counting source, and the
// source, so the caller can report and save where the stream stands.
func New(seed int64) (*rand.Rand, *Source) {
	src := NewSource(seed)

	return rand.New(src), src // nolint:gosec // gameplay RNG, seeded for reproducibility
}

// Restore returns a counting source seeded with seed and advanced past its
// first draws values: the value it hands out next is the one an unbroken
// stream would have handed out as its (draws+1)th. Draws reports draws.
//
// The skip costs one generator step per draw (nanoseconds each), so a night's
// few thousand draws restore instantly; it is not meant for counts in the
// billions.
func Restore(seed int64, draws uint64) *Source {
	s := NewSource(seed)

	for i := uint64(0); i < draws; i++ {
		// Uint64 and Int63 advance the generator by the same single step
		// (rngSource.Int63 is Uint64 masked), so skipping with either is
		// skipping with both; the test pins it by mixing them.
		s.src.Uint64()
	}

	s.draws = draws

	return s
}

// Int63 draws one value. Counted.
func (s *Source) Int63() int64 {
	s.draws++

	return s.src.Int63()
}

// Uint64 draws one value. Counted.
func (s *Source) Uint64() uint64 {
	s.draws++

	return s.src.Uint64()
}

// Seed reseeds the source and starts its count again from zero, so Seeded and
// Draws always describe the stream as it now runs.
func (s *Source) Seed(seed int64) {
	s.seed = seed
	s.draws = 0
	s.src.Seed(seed)
}

// Seeded is the seed the stream was last seeded with.
func (s *Source) Seeded() int64 { return s.seed }

// Draws is how many values have been drawn since the stream was last seeded.
func (s *Source) Draws() uint64 { return s.draws }

// Report is the stream's position in the shape every harness provider uses:
// {"present": true, "seed": .., "seed_str": "..", "draws": ..}. A nil source
// reports {"present": false} and NO seed or draws -- never a zero that reads
// like a stream at draw 0 of seed 0, which a save would write and a load
// would restore as if it were real.
func (s *Source) Report() map[string]interface{} {
	if s == nil {
		return Absent()
	}

	return ReportOf(s.seed, s.draws)
}

// ReportOf is the report of a stream at (seed, draws), for a stream whose
// position is read some other way (the map engine's RandSeed/RandDraws).
//
// THE SEED IS ALSO WRITTEN AS A DECIMAL STRING, seed_str, and that is the one
// a reader of JSON must use. A wall-clock seed (UnixNano, ~1.8e18) is far past
// 2^53, where float64 stops holding every integer; the playtests decode JSON
// into float64, so "seed" read that way is a DIFFERENT seed, and a stream
// restored from it is a different stream from its first draw. seed_str
// survives any decoder. The world file (M4.6) decodes seeds as int64.
func ReportOf(seed int64, draws uint64) map[string]interface{} {
	return map[string]interface{}{
		"present":  true,
		"seed":     seed,
		"seed_str": strconv.FormatInt(seed, 10),
		"draws":    draws,
	}
}

// Absent is the report of a stream that does not exist.
func Absent() map[string]interface{} {
	return map[string]interface{}{"present": false}
}

func newSource64(seed int64) rand.Source64 {
	return source64(rand.NewSource(seed)) // nolint:gosec // gameplay RNG, seeded for reproducibility
}

// source64 insists that src is a Source64, and PANICS if it is not.
//
// rand.NewSource's result implements Source64 in every supported Go. The
// fallback the map engine's wrapper carried -- wrap it in a *rand.Rand, which
// is a Source64 -- would have broken the count silently: a *rand.Rand used as
// a source builds its Uint64 from TWO Int63 calls, so every counted draw
// would advance the generator two steps while Restore skips one, and a
// restored stream would be wrong from its first value. A stdlib that stopped
// returning a Source64 must stop the game, not skew it.
func source64(src rand.Source) rand.Source64 {
	s64, ok := src.(rand.Source64)
	if !ok {
		panic(fmt.Sprintf("d2rand: %T is not a rand.Source64; a counted stream cannot be restored from it", src))
	}

	return s64
}

// Stream is a counted stream held as ONE value: the *rand.Rand a system
// draws from and the Source under it (M4.6 B1 review, C8).
//
// A system that kept the two in separate fields had to replace both on a
// restore, and a restore that replaced one would leave the other describing a
// stream nothing draws from: a report of (seed, draws) for a source no roll
// uses, or rolls from a rand whose count no report reads. Restore replaces
// them together inside the one value every holder points at.
//
// The *rand.Rand is embedded, so a system draws exactly as it did
// (rng.Float64(), rng.Intn(n), rng.Shuffle(..)). HOLD THE *Stream, NEVER ITS
// Rand: a *rand.Rand taken out of it keeps drawing from the old source after
// a Restore, and no report counts those draws.
//
// Not safe for concurrent use, like the *rand.Rand it holds.
type Stream struct {
	*rand.Rand
	src *Source
}

// NewStream returns a counted stream seeded with seed, at draw 0.
func NewStream(seed int64) *Stream {
	st := &Stream{}
	st.use(NewSource(seed))

	return st
}

// Restore puts the stream where a saved game left it: seeded with seed and
// advanced past its first draws values (see the package func Restore). The
// rand and its source are replaced together.
func (st *Stream) Restore(seed int64, draws uint64) {
	st.use(Restore(seed, draws))
}

func (st *Stream) use(src *Source) {
	st.src = src
	st.Rand = rand.New(src) // nolint:gosec // gameplay RNG, seeded for reproducibility
}

// Seeded is the seed the stream was last seeded or restored with.
func (st *Stream) Seeded() int64 { return st.src.Seeded() }

// Draws is how many values have been drawn since then (counting the ones a
// Restore skipped).
func (st *Stream) Draws() uint64 { return st.src.Draws() }

// Report is the stream's position (Source.Report); a nil stream reports that
// it is absent.
func (st *Stream) Report() map[string]interface{} {
	if st == nil {
		return Absent()
	}

	return st.src.Report()
}

// Reader is a seeded byte stream -- an io.Reader over a *rand.Rand's Read --
// that counts the bytes it has handed out. It exists for the harness's uuid
// stream (uuid.SetRand takes an io.Reader), which is the one stream in the
// game that draws through rand.Rand.Read.
//
// It is counted in BYTES, not draws, because Read keeps up to seven unread
// bytes of its last value inside the *rand.Rand: the draw count cannot say
// where inside a value the next byte comes from, and the byte count can.
// Replaying the same number of bytes through Read leaves the *rand.Rand in
// exactly the state it was in, so RestoreReader is exact. The bytes it hands
// out are byte-identical to rand.New(rand.NewSource(seed)).Read's, so moving
// the uuid stream onto it moves no entity id.
//
// Safe for concurrent use: the uuid package may be called from the server's
// goroutine as well as the game's.
type Reader struct {
	mu    sync.Mutex
	seed  int64
	bytes uint64
	r     *rand.Rand
}

var _ io.Reader = (*Reader)(nil)

// NewReader returns a counting byte stream seeded with seed, at byte 0.
func NewReader(seed int64) *Reader {
	return &Reader{seed: seed, r: rand.New(rand.NewSource(seed))} // nolint:gosec // seeded for reproducibility
}

// RestoreReader returns a byte stream seeded with seed and advanced past its
// first bytes bytes.
func RestoreReader(seed int64, bytes uint64) *Reader {
	rd := NewReader(seed)

	var buf [512]byte

	for left := bytes; left > 0; {
		n := uint64(len(buf))
		if left < n {
			n = left
		}

		_, _ = rd.r.Read(buf[:n]) // rand.Rand.Read always fills p and never errs

		left -= n
	}

	rd.bytes = bytes

	return rd
}

// Read fills p from the stream. Counted in bytes.
func (r *Reader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n, err := r.r.Read(p)
	r.bytes += uint64(n)

	return n, err
}

// Seeded is the seed the stream started from.
func (r *Reader) Seeded() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.seed
}

// Bytes is how many bytes have been read since the stream was seeded.
func (r *Reader) Bytes() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.bytes
}
