package d2gamescreen

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mapedit"
)

// TestTheGhostWillNotBuryTheVillagesObjects (the raid's R3a review): a
// structure dropped on a household's door, the hotar or a watch post is
// refused by the ghost with who stands there -- the loader refuses all three
// on a blocked tile -- not by a refusal after the edit. Drawing them is R3c's.
func TestTheGhostWillNotBuryTheVillagesObjects(t *testing.T) {
	d, err := d2mapedit.OpenFile(filepath.Join("..", "..", "data", "strigoi", "maps", "village.tmj"))
	if err != nil {
		t.Fatal(err)
	}

	e := &Editor{doc: d}

	for _, c := range []struct {
		at   image.Point
		name string
	}{{image.Pt(16, 16), "the church"}, {image.Pt(22, 40), "the hotar"}, {image.Pt(24, 34), "the gate post"}} {
		ok, why := e.nobodyStandsIn(image.Rect(c.at.X-1, c.at.Y-1, c.at.X+1, c.at.Y+1))
		if ok || !strings.Contains(why, c.name) {
			t.Errorf("a footprint over %v: ok %v, %q; want refused naming %q", c.at, ok, why, c.name)
		}
	}

	// The control: open ground (the green east of the road) is free.
	if ok, why := e.nobodyStandsIn(image.Rect(19, 20, 21, 22)); !ok {
		t.Errorf("open ground at 19..20,20..21 refused: %s", why)
	}
}
