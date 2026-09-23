package d2gamescreen

import (
	"math"
	"testing"
)

// A 20x20-tile inside area at 10..29, the quarry at its middle.
func villageInside(x, y int) bool { return x >= 10 && x < 30 && y >= 10 && y < 30 }

func TestAnArrivalInsideIsCarriedOut(t *testing.T) {
	// Due east of the quarry, 6 tiles: inside. Carried east until outside.
	x, y := outsideAlong(20, 20, 26, 20, villageInside)
	if villageInside(int(math.Floor(x)), int(math.Floor(y))) {
		t.Fatalf("still inside at %.2f,%.2f", x, y)
	}

	if y != 20 || x < 30 || x > 30.5 {
		t.Fatalf("carried to %.2f,%.2f; want just past the east edge on the same bearing (30..30.5, 20)", x, y)
	}

	// On a diagonal the bearing is kept.
	x, y = outsideAlong(20, 20, 23, 23, villageInside)
	if math.Abs((x-20)-(y-20)) > 1e-9 || villageInside(int(math.Floor(x)), int(math.Floor(y))) {
		t.Fatalf("diagonal arrival carried to %.2f,%.2f; want outside on the same diagonal", x, y)
	}
}

func TestAnArrivalOutsideIsUntouched(t *testing.T) {
	if x, y := outsideAlong(20, 20, 35, 20, villageInside); x != 35 || y != 20 {
		t.Fatalf("an outside spot moved to %.2f,%.2f", x, y)
	}

	// No inside areas at all (a generated world): nothing moves.
	none := func(int, int) bool { return false }
	if x, y := outsideAlong(20, 20, 26, 20, none); x != 26 || y != 20 {
		t.Fatalf("with no inside, a spot moved to %.2f,%.2f", x, y)
	}

	if x, y := outsideAlong(20, 20, 26, 20, nil); x != 26 || y != 20 {
		t.Fatalf("with a nil inside, a spot moved to %.2f,%.2f", x, y)
	}
}

func TestASpotOnTheQuarryStillGetsOut(t *testing.T) {
	x, y := outsideAlong(20, 20, 20, 20, villageInside)
	if villageInside(int(math.Floor(x)), int(math.Floor(y))) {
		t.Fatalf("a spot on the quarry stayed inside at %.2f,%.2f", x, y)
	}
}

func TestNowhereOutsideLeavesTheSpot(t *testing.T) {
	everywhere := func(int, int) bool { return true }
	if x, y := outsideAlong(20, 20, 26, 20, everywhere); x != 26 || y != 20 {
		t.Fatalf("with no outside in reach the spot moved to %.2f,%.2f", x, y)
	}
}

// With the quarry outside the village and the spot fallen inside it beyond
// him, the arrival comes out on HIS side -- not thrown on through the
// village and out the far fence.
func TestAQuarryOutsideDrawsTheNearerWayOut(t *testing.T) {
	// Quarry at 20,40 (outside, south), spot at 20,25 (inside, 5 tiles past
	// the south edge at y=30; the north edge is 15 tiles on).
	x, y := outsideAlong(20, 40, 20, 25, villageInside)
	if villageInside(int(math.Floor(x)), int(math.Floor(y))) {
		t.Fatalf("still inside at %.2f,%.2f", x, y)
	}

	if y < 30 || y > 30.5 || x != 20 {
		t.Fatalf("came out at %.2f,%.2f; want the south edge, the quarry's side (20, 30..30.5)", x, y)
	}

	// Control: a spot nearer the FAR edge still goes out the far side.
	x, y = outsideAlong(20, 40, 20, 11, villageInside)
	if y >= 10 || villageInside(int(math.Floor(x)), int(math.Floor(y))) {
		t.Fatalf("a spot one tile inside the north edge came out at %.2f,%.2f; want just north of 10", x, y)
	}
}
