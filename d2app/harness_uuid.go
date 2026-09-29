//go:build harness

package d2app

import (
	"strconv"
	"sync"

	"github.com/google/uuid"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
)

// The harness's uuid stream, counted (M4.6 B1).
//
// A seeded start_game makes entity ids reproducible by seeding the uuid
// package's reader (P3 E5). That reader is the one stream in the game that
// draws through rand.Rand.Read, whose unread bytes live inside the *rand.Rand
// where no draw counter can see them, so it is counted in BYTES by
// d2rand.Reader -- whose bytes are rand.Rand.Read's exactly, so no id moves.
// The "uuid" provider reports the count; the world save's load (burst B4b)
// will restore it with d2rand.RestoreReader after the entities are rebuilt.
//
// It is PROCESS state, not game state: the reader outlives a game, and a
// second seeded start_game replaces it. The provider is registered once, the
// first time a start_game seeds or unseeds the stream or a game begins, and
// lives with the process.
//
// ONLY start_game RESEEDS IT (M4.6 B1 review, B1). Every other way into a
// game -- the death screen's "load last save" (App.ReloadGame, which ends in
// ToCreateGame), a menu's new game -- inherits the stream exactly where the
// last game left it: a seeded process continues from the dead game's byte
// count, and the new game's player id (the connection's uuid) and every
// entity id come from there, not from the start of the seed. Such a game is
// deterministic for one script in one process and matches no fresh launch.
// The provider says which kind of game this is: seeded_for_this_game is true
// only when start_game seeded the stream for the game now running, and
// bytes_at_game_start is where the stream stood when that game began (0 for a
// fresh seed). Restoring the stream on a load is burst B4b's
// (docs/m4.6-world-save-notes.md).

// nolint:gochecknoglobals // the uuid package's reader is process-global too
var harnessUUID struct {
	mu   sync.Mutex
	r    *d2rand.Reader
	once sync.Once

	games      uint64 // games this process has begun (ToCreateGame calls)
	seededFor  uint64 // the game start_game last seeded the stream for; 0 = none
	startBytes uint64 // the stream's byte count when the current game began
}

func harnessRegisterUUID() {
	harnessUUID.once.Do(func() {
		d2harness.Register(harnessUUIDProvider{})

		// M4.6 B3: the world save carries where the stream stands (rng.uuid),
		// so a load can put it back after the entities (B4b, trap 6).
		d2gamescreen.SetUUIDStream(harnessUUIDStream)
	})
}

// harnessUUIDStream is where the seeded uuid stream stands, for the world
// save: its seed and byte count, and false when start_game left it unseeded
// (crypto/rand, which no save can carry).
func harnessUUIDStream() (seed int64, bytes uint64, ok bool) {
	harnessUUID.mu.Lock()
	defer harnessUUID.mu.Unlock()

	if harnessUUID.r == nil {
		return 0, 0, false
	}

	return harnessUUID.r.Seeded(), harnessUUID.r.Bytes(), true
}

// harnessSeedUUID seeds the uuid stream (seed != 0) or hands it back to
// crypto/rand (seed == 0), for the game start_game is about to begin.
func harnessSeedUUID(seed int64) {
	harnessRegisterUUID()

	harnessUUID.mu.Lock()
	defer harnessUUID.mu.Unlock()

	harnessUUID.seededFor = harnessUUID.games + 1 // ToCreateGame follows

	if seed == 0 {
		harnessUUID.r = nil

		uuid.SetRand(nil)

		return
	}

	harnessUUID.r = d2rand.NewReader(seed)

	uuid.SetRand(harnessUUID.r)
}

// harnessUUIDGameBegins marks a new game, before it draws a single id.
func harnessUUIDGameBegins() {
	harnessRegisterUUID()

	harnessUUID.mu.Lock()
	defer harnessUUID.mu.Unlock()

	harnessUUID.games++
	harnessUUID.startBytes = 0

	if harnessUUID.r != nil {
		harnessUUID.startBytes = harnessUUID.r.Bytes()
	}
}

// harnessGameBegins is ToCreateGame's first line in a harness build: every
// game, whichever way it was entered, is counted before it draws an id.
func (a *App) harnessGameBegins() { harnessUUIDGameBegins() }

// harnessUUIDProvider reports where the uuid stream stands.
type harnessUUIDProvider struct{}

func (harnessUUIDProvider) HarnessName() string { return "uuid" }

func (harnessUUIDProvider) HarnessState() map[string]interface{} {
	harnessUUID.mu.Lock()
	r := harnessUUID.r
	games, seededFor, startBytes := harnessUUID.games, harnessUUID.seededFor, harnessUUID.startBytes
	harnessUUID.mu.Unlock()

	state := map[string]interface{}{
		// Which game this is, and whether start_game seeded the stream FOR it.
		// A reload's game is games = seeded_game + 1 and not seeded for.
		"games":                games,
		"seeded_game":          seededFor,
		"seeded_for_this_game": r != nil && games > 0 && seededFor == games,
		"bytes_at_game_start":  startBytes,
	}

	if r == nil {
		state["seeded"], state["seed"], state["seed_str"] = false, int64(0), "0"
		state["bytes"], state["uuids"] = uint64(0), uint64(0)

		return state
	}

	bytes := r.Bytes()
	seed := r.Seeded()

	state["seeded"] = true
	state["seed"] = seed
	state["seed_str"] = strconv.FormatInt(seed, 10) // exact past 2^53 (d2rand.ReportOf)
	state["bytes"] = bytes
	state["uuids"] = bytes / 16 // a v4 uuid reads 16 bytes

	return state
}
