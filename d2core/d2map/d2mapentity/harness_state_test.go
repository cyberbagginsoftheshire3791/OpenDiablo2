package d2mapentity

import (
	"encoding/json"
	"testing"
)

// The harness hashes HarnessState into the determinism digest, so it must be
// safe on partially constructed entities (nil stats, composite, monstat) and
// JSON-encodable in full.

func TestPlayerHarnessStateNilSafe(t *testing.T) {
	p := &Player{mapEntity: newMapEntity(155, 70), name: "Harness", Act: 1, Gold: 7}

	state := p.HarnessState()

	if state["name"] != "Harness" || state["act"] != 1 || state["gold"] != 7 {
		t.Fatalf("unexpected state: %v", state)
	}

	if _, ok := state["stamina"]; ok {
		t.Fatal("stamina must be absent when Stats is nil")
	}

	if _, ok := state["direction"]; ok {
		t.Fatal("direction must be absent when the composite is nil")
	}

	if _, err := json.Marshal(state); err != nil {
		t.Fatalf("player state is not JSON-encodable: %v", err)
	}
}

func TestNPCHarnessStateNilSafe(t *testing.T) {
	n := &NPC{mapEntity: newMapEntity(10, 20), name: "Cain", HasPaths: true, path: 2, repetitions: 3}

	state := n.HarnessState()

	if state["name"] != "Cain" || state["has_paths"] != true || state["path_index"] != 2 || state["repetitions"] != 3 {
		t.Fatalf("unexpected state: %v", state)
	}

	if _, ok := state["monstat"]; ok {
		t.Fatal("monstat must be absent when the record is nil")
	}

	if _, err := json.Marshal(state); err != nil {
		t.Fatalf("npc state is not JSON-encodable: %v", err)
	}
}
