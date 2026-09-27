//go:build harness

package d2app

import (
	mrand "math/rand"
	"testing"

	"github.com/google/uuid"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// M4.6 B1: the seeded uuid stream is counted, and counting it moves no id.
func TestUUIDStreamIsCountedAndUnchanged(t *testing.T) {
	t.Cleanup(func() { harnessSeedUUID(0) })

	// What a seeded start_game handed out before B1: the stdlib reader.
	uuid.SetRand(mrand.New(mrand.NewSource(1462))) // nolint:gosec // test

	before := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}

	harnessSeedUUID(1462)

	for i := 0; i < 3; i++ {
		if got := uuid.New(); got != before[i] {
			t.Fatalf("uuid %d: counted stream %s, stdlib %s -- counting moved an entity id", i, got, before[i])
		}
	}

	p, ok := d2harness.Lookup("uuid")
	if !ok {
		t.Fatal("seeding the stream must register the uuid provider")
	}

	state := p.HarnessState()
	if state["seeded"] != true || state["seed"] != int64(1462) || state["bytes"] != uint64(48) || state["uuids"] != uint64(3) {
		t.Fatalf("three uuids are 48 bytes from seed 1462: %v", state)
	}

	// The restore B4b will make: back at byte 48, the next id is the 4th.
	next, err := uuid.NewRandomFromReader(d2rand.RestoreReader(1462, 48))
	if err != nil || next != before[3] {
		t.Fatalf("restoring at byte 48 must hand out the 4th id %s, got %s (%v)", before[3], next, err)
	}

	short, _ := uuid.NewRandomFromReader(d2rand.RestoreReader(1462, 47))
	if short == before[3] {
		t.Fatal("one byte short must not hand out the same id")
	}

	harnessSeedUUID(0)

	if state := p.HarnessState(); state["seeded"] != false || state["bytes"] != uint64(0) {
		t.Fatalf("an unseeded start_game reports no stream: %v", state)
	}
}

// M4.6 B1 review, B1: what the uuid stream does on each way into a game,
// pinned as it is today. The fresh launch (start_game seeds, then the game
// begins) starts the seed's stream at byte 0 for this game; "load last save"
// (App.ReloadGame -> ToCreateGame, which nothing reseeds) CONTINUES the dead
// game's stream, so its ids are the seed's later ones and no fresh launch
// draws them. The provider must tell the two apart. Restoring the stream on a
// load is B4b's; if B4b changes the second half, this test changes with it.
func TestUUIDStreamOnAFreshLaunchAndOnAReload(t *testing.T) {
	t.Cleanup(func() { harnessSeedUUID(0) })

	// Plain stdlib: the seed's ids in order, the reference for both halves.
	ref := mrand.New(mrand.NewSource(1462)) // nolint:gosec // test
	seedIDs := make([]uuid.UUID, 3)

	for i := range seedIDs {
		seedIDs[i], _ = uuid.NewRandomFromReader(ref)
	}

	// --- the fresh launch: start_game seeds, ToCreateGame begins the game ---
	harnessSeedUUID(1462)
	harnessUUIDGameBegins()

	p, ok := d2harness.Lookup("uuid")
	if !ok {
		t.Fatal("the uuid provider must be registered")
	}

	fresh := p.HarnessState()
	if fresh["seeded_for_this_game"] != true || fresh["bytes_at_game_start"] != uint64(0) ||
		fresh["seed_str"] != "1462" || fresh["games"] != fresh["seeded_game"] {
		t.Fatalf("a fresh launch is seeded for its game from byte 0: %v", fresh)
	}

	for i := 0; i < 2; i++ {
		if got := uuid.New(); got != seedIDs[i] {
			t.Fatalf("fresh launch id %d: %s, the seed's is %s", i, got, seedIDs[i])
		}
	}

	// --- the reload: ToCreateGame again, and nothing reseeds -------------
	harnessUUIDGameBegins()

	reload := p.HarnessState()
	if reload["seeded_for_this_game"] != false {
		t.Fatalf("a reloaded game was not seeded for: %v", reload)
	}

	if reload["bytes_at_game_start"] != uint64(32) || reload["bytes"] != uint64(32) || reload["seeded"] != true {
		t.Fatalf("the reloaded game begins where the dead one stopped, 2 ids = 32 bytes in: %v", reload)
	}

	if reload["games"].(uint64) != reload["seeded_game"].(uint64)+1 {
		t.Fatalf("the reload is the game after the seeded one: %v", reload)
	}

	// TODAY'S BEHAVIOUR, and the limitation B4b owns: the reloaded game's
	// first id is the seed's THIRD, not its first.
	if got := uuid.New(); got != seedIDs[2] || got == seedIDs[0] {
		t.Fatalf("the reloaded game's first id is %s; the dead game's next was %s", got, seedIDs[2])
	}
}
