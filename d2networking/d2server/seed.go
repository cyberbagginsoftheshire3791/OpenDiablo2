package d2server

import (
	"sync/atomic"
	"time"
)

// The next-game seed override (P3 spec E3, signed ask 8.7 processing): the
// playtest harness sets a seed immediately before the game client constructs
// the in-process GameServer, so map generation is reproducible. One-shot —
// consumed by the next NewGameServer and then cleared — and when no override
// is set the behaviour is exactly the old one (wall-clock seed).

// nolint:gochecknoglobals // deliberate one-shot cross-package handoff
var nextGameSeed int64

// SetNextGameSeed sets the seed the next NewGameServer call will use.
// Passing 0 clears the override (0 is not a usable seed value: it means
// "wall clock" everywhere in this seam).
func SetNextGameSeed(seed int64) {
	atomic.StoreInt64(&nextGameSeed, seed)
}

// ArmedForNextGame reports whether a seed and a start position are armed for
// the next game server -- handed over by a load (or a script) and not yet
// taken. Observability for the B4a review's C5: a game that never opened must
// leave neither behind.
func ArmedForNextGame() (seed, start bool) {
	nextStart.Lock()
	start = nextStart.set
	nextStart.Unlock()

	return atomic.LoadInt64(&nextGameSeed) != 0, start
}

// takeNextGameSeed consumes the override, or mints the default wall-clock
// seed when none is set.
func takeNextGameSeed() int64 {
	if s := atomic.SwapInt64(&nextGameSeed, 0); s != 0 {
		return s
	}

	return time.Now().UnixNano()
}
