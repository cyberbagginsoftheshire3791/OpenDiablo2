package d2mapentity

import (
	"encoding/json"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// M4.6 B1: an entity reports where it is walking -- the point it steps toward
// and the waypoints ahead -- in world tiles, beside path_len. The save carries
// both, so both must be observable first.
func TestHarnessStateReportsMotion(t *testing.T) {
	n := &NPC{mapEntity: newMapEntity(50, 50)} // sub-tiles: world 10,10

	state := n.HarnessState()
	if got := state["target"]; got != [2]float64{10, 10} {
		t.Fatalf("a standing entity's target is where it stands, in world tiles; got %v", got)
	}

	if got := state["waypoints"].([][2]float64); len(got) != 0 {
		t.Fatalf("no walk, no waypoints; got %v", got)
	}

	// Three waypoints; SetPath consumes the first as the current target.
	n.SetPath([]d2vector.Position{
		d2vector.NewPosition(60, 50), d2vector.NewPosition(70, 55), d2vector.NewPosition(75, 75),
	}, nil)

	state = n.HarnessState()
	if got := state["target"]; got != [2]float64{12, 10} {
		t.Fatalf("the target is the first waypoint, in world tiles; got %v", got)
	}

	want := [][2]float64{{14, 11}, {15, 15}}

	got := state["waypoints"].([][2]float64)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("the waypoints still ahead, in world tiles: got %v want %v", got, want)
	}

	if state["path_len"] != len(want) {
		t.Fatalf("path_len and waypoints are one list: %v vs %d", state["path_len"], len(want))
	}

	if _, err := json.Marshal(state); err != nil {
		t.Fatalf("not JSON-encodable: %v", err)
	}

	p := &Player{mapEntity: newMapEntity(5, 5)}
	if _, ok := p.HarnessState()["waypoints"]; !ok {
		t.Fatal("the player reports waypoints too")
	}

	c := &Creature{mapEntity: newMapEntity(5, 5)}
	if _, ok := c.HarnessState()["target"]; !ok {
		t.Fatal("a creature reports its target too")
	}
}
