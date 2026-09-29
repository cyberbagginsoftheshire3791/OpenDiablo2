//go:build harness

package d2app

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// dialProbe is a settable system that records what is written to it.
type dialProbe struct {
	name   string
	writes []string
}

func (p *dialProbe) HarnessName() string                  { return p.name }
func (p *dialProbe) HarnessState() map[string]interface{} { return map[string]interface{}{} }
func (p *dialProbe) HarnessSet(field string, value interface{}) error {
	p.writes = append(p.writes, field)
	return nil
}

// M4.6 B4a, the load's step 7: a script's DIALS are re-applied by a load --
// the last value of each, in the order each was first written -- and nothing
// that is state or a verb is ever written again over what the load restored.
func TestTheLoadReappliesTheScriptsDialsAndNothingElse(t *testing.T) {
	harnessDials.Lock()
	harnessDials.order, harnessDials.values = nil, nil
	harnessDials.Unlock()

	rising := &dialProbe{name: "rising"}
	meters := &dialProbe{name: "meters"}
	spawns := &dialProbe{name: "spawns"}

	for _, p := range []d2harness.Provider{rising, meters, spawns} {
		d2harness.Register(p)
		defer d2harness.Unregister(p)
	}

	harnessRecordDial("rising", "p", 1.0)
	harnessRecordDial("meters", "health", 0.0)      // state: never re-applied
	harnessRecordDial("spawns", "despawn", "g:1")   // a verb: never re-applied
	harnessRecordDial("spawns", "open_bodies", 3.0) // state: never re-applied
	harnessRecordDial("spawns", "chance", 0.0)
	harnessRecordDial("rising", "p", 0.0) // the last value of p, in p's first place

	if n := harnessReapplyDials(); n != 2 {
		t.Fatalf("two dials re-applied, got %d", n)
	}

	if len(meters.writes) != 0 || len(spawns.writes) != 1 || spawns.writes[0] != "chance" || len(rising.writes) != 1 {
		t.Fatalf("only the dials are written again: meters %v, spawns %v, rising %v", meters.writes, spawns.writes, rising.writes)
	}

	if got := harnessDialNames(); len(got) != 2 || got[0] != "rising.p" || got[1] != "spawns.chance" {
		t.Fatalf("the recorded dials: %v", got)
	}
}

// The uuid provider's report is split for the digest: the stream (seed,
// bytes) is the world file's rng.uuid, and the game counts are this process's.
func TestTheUUIDReportsGameCountsAsProcessState(t *testing.T) {
	t.Cleanup(func() { harnessSeedUUID(0) })

	harnessSeedUUID(1462)

	world, process := harnessUUIDProvider{}.HarnessDigest()

	for _, k := range harnessUUIDProcessKeys {
		if _, ok := world[k]; ok {
			t.Errorf("%s is this process's, and is in the world part", k)
		}

		if _, ok := process[k]; !ok {
			t.Errorf("%s is missing from the process part", k)
		}
	}

	for _, k := range []string{"seeded", "seed", "seed_str", "bytes", "uuids"} {
		if _, ok := world[k]; !ok {
			t.Errorf("%s is the stream's, and is missing from the world part", k)
		}
	}
}
