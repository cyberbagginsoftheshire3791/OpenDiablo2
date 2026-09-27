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
