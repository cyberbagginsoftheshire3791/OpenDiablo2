package d2mapentity

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
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

// THE B4b REVIEW FIXES (29 Sep 2026), BUG-82: the pose flags and the velocity
// the world save carries are reported, so a resume that lost one is visible
// in the digest (the review's C5: the digest had none of the three).
func TestHarnessStateReportsThePoseAndTheVelocity(t *testing.T) {
	c := b2bWalker(t)

	state := c.HarnessState()
	require.Equal(t, "", state["held"], "a walker holds no action")
	require.Equal(t, false, state["corpse"])

	v := c.GetVelocity()
	require.False(t, v.IsZero(), "the walker has a velocity")
	require.Equal(t, [2]float64{v.X() / subtilesPerTile, v.Y() / subtilesPerTile}, state["velocity"],
		"reported in world tiles, as target and waypoints are")

	require.NoError(t, c.StartAction(d2enum.MonsterAnimationModeAttack1, nil))
	require.Equal(t, "attack", c.HarnessState()["held"])

	for i := 0; i < 100 && c.held; i++ {
		c.Advance(b2bFrame)
	}

	require.Equal(t, "", c.HarnessState()["held"], "the bite played through")

	require.NoError(t, c.StartAction(d2enum.MonsterAnimationModeDeath, nil))
	require.Equal(t, "death", c.HarnessState()["held"])

	for i := 0; i < 100 && !c.corpse; i++ {
		c.Advance(b2bFrame)
	}

	require.Equal(t, true, c.HarnessState()["corpse"])
	require.Equal(t, "", c.HarnessState()["held"])

	// A standstill is reported as zero, whatever the sign the stop left
	// (the first whole run was red at act 4 on [0,-0] against [0,0]).
	still := &Creature{mapEntity: newMapEntity(5, 5)}
	still.velocity.Set(math.Copysign(0, -1), math.Copysign(0, -1))

	line, err := json.Marshal(still.HarnessState()["velocity"])
	require.NoError(t, err)
	require.Equal(t, "[0,0]", string(line))

	// An NPC (no composite: the snapshot half only, as TestNPCMotionCarriesItsPose).
	held := &NPC{mapEntity: newMapEntity(0, 0), held: true, heldMode: d2enum.MonsterAnimationModeGetHit}
	require.Equal(t, "GH", held.HarnessState()["held"])
	require.Equal(t, false, held.HarnessState()["corpse"])

	dead := &NPC{mapEntity: newMapEntity(0, 0), corpse: true}
	require.Equal(t, "", dead.HarnessState()["held"])
	require.Equal(t, true, dead.HarnessState()["corpse"])
	require.Equal(t, [2]float64{0, 0}, dead.HarnessState()["velocity"])

	if _, err := json.Marshal(held.HarnessState()); err != nil {
		t.Fatalf("not JSON-encodable: %v", err)
	}
}
