//go:build harness

package d2app

import (
	"sync"

	"github.com/google/uuid"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
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
// first time a start_game seeds or unseeds the stream, and lives with the
// process.

// nolint:gochecknoglobals // the uuid package's reader is process-global too
var harnessUUID struct {
	mu   sync.Mutex
	r    *d2rand.Reader
	once sync.Once
}

// harnessSeedUUID seeds the uuid stream (seed != 0) or hands it back to
// crypto/rand (seed == 0).
func harnessSeedUUID(seed int64) {
	harnessUUID.once.Do(func() { d2harness.Register(harnessUUIDProvider{}) })

	harnessUUID.mu.Lock()
	defer harnessUUID.mu.Unlock()

	if seed == 0 {
		harnessUUID.r = nil

		uuid.SetRand(nil)

		return
	}

	harnessUUID.r = d2rand.NewReader(seed)

	uuid.SetRand(harnessUUID.r)
}

// harnessUUIDProvider reports where the uuid stream stands.
type harnessUUIDProvider struct{}

func (harnessUUIDProvider) HarnessName() string { return "uuid" }

func (harnessUUIDProvider) HarnessState() map[string]interface{} {
	harnessUUID.mu.Lock()
	r := harnessUUID.r
	harnessUUID.mu.Unlock()

	if r == nil {
		return map[string]interface{}{"seeded": false, "seed": int64(0), "bytes": uint64(0), "uuids": uint64(0)}
	}

	bytes := r.Bytes()

	return map[string]interface{}{
		"seeded": true,
		"seed":   r.Seeded(),
		"bytes":  bytes,
		"uuids":  bytes / 16, // a v4 uuid reads 16 bytes
	}
}
