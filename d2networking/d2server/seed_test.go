package d2server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
)

func TestNextGameSeedIsOneShot(t *testing.T) {
	SetNextGameSeed(1462)
	assert.Equal(t, int64(1462), takeNextGameSeed(), "the set seed is consumed")

	second := takeNextGameSeed()
	assert.NotEqual(t, int64(1462), second, "the override is one-shot")
	assert.NotZero(t, second, "with no override the wall-clock default applies")

	SetNextGameSeed(7)
	SetNextGameSeed(0) // clearing
	assert.NotEqual(t, int64(7), takeNextGameSeed(), "0 clears the override")
}

// M4.6 B4a: a load puts the local player where he was saved. The position is
// one-shot (the next server takes it), placed on the sub-tile the saved point
// lies in, spent by the first local client, and never given to a remote one.
func TestNextStartPositionIsOneShotAndLocal(t *testing.T) {
	SetNextStartPosition(143.4, 112.9)

	at := takeNextStartPosition()
	assert.True(t, at.set, "the set position is taken by the next server")
	assert.False(t, takeNextStartPosition().set, "and only by it: one-shot")

	// A remote player joins at the map's start, and does not spend it.
	x, y, wx, wy := at.placeOnConnect(d2clientconnectiontype.LANClient, 20, 30)
	assert.Equal(t, []int{103, 153}, []int{x, y}, "a remote player: the start tile, mid-tile, as ever")
	assert.Equal(t, []float64{20, 30}, []float64{wx, wy})

	// The local player is put on the sub-tile the saved point lies in.
	x, y, wx, wy = at.placeOnConnect(d2clientconnectiontype.Local, 20, 30)
	assert.Equal(t, []int{143, 112}, []int{x, y}, "the local player: the sub-tile his saved point lies in")
	assert.InDelta(t, 28.68, wx, 1e-9)
	assert.InDelta(t, 22.58, wy, 1e-9)

	// Spent: a second local connection (a reconnect) starts on the map's start.
	x, y, _, _ = at.placeOnConnect(d2clientconnectiontype.Local, 20, 30)
	assert.Equal(t, []int{103, 153}, []int{x, y}, "the position is spent by the first local client")

	// Cleared: a load refused before its game opened leaves nothing behind.
	SetNextStartPosition(1, 1)
	ClearNextStartPosition()
	assert.False(t, takeNextStartPosition().set, "a cleared position is not taken")
}

// The B4a review, C5: a game that never opened leaves nothing armed for the
// next -- the seed a load handed the server as well as his start position
// (before, only the position was dropped, and the next game of any hero began
// on the refused file's seed).
func TestClearNextGameClearsTheSeedAndThePosition(t *testing.T) {
	SetNextGameSeed(1462)
	SetNextStartPosition(143.4, 112.9)

	seed, start := ArmedForNextGame()
	assert.True(t, seed && start, "armed")

	ClearNextGame()

	seed, start = ArmedForNextGame()
	assert.False(t, seed, "the seed is cleared")
	assert.False(t, start, "the start position is cleared")
	assert.NotEqual(t, int64(1462), takeNextGameSeed(), "and no server takes it")
}
