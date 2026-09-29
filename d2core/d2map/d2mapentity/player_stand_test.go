package d2mapentity

import (
	"testing"
)

// M4.6 B4a, rule 4: a load stands him at the saved point, bit-exact, and a
// walk he was on does not continue -- stepping moves him nowhere and calls no
// arrival.
func TestStandAtStandsHimStill(t *testing.T) {
	p := &Player{mapEntity: movingEntity()}
	arrived := false
	p.done = func() { arrived = true }

	x, y := 143.40000000000003, 112.19999999999999
	p.StandAt(x, y, 56) // no body drawn: the facing is the body's, and there is none to turn

	for i := 0; i < 20; i++ {
		p.Step(normalTickTime)
	}

	if p.Position.X() != x || p.Position.Y() != y {
		t.Fatalf("he stands at the saved point exactly: %v,%v, want %v,%v", p.Position.X(), p.Position.Y(), x, y)
	}

	if p.IsMoving() || p.hasPath() || arrived || !p.velocity.IsZero() {
		t.Fatalf("no walk continues: moving %v, path %v, arrival called %v, velocity %v",
			p.IsMoving(), p.hasPath(), arrived, p.velocity)
	}
}
