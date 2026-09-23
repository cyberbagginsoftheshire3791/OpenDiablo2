package d2mapengine

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
)

// Where an authored wall image hangs from its tile's top corner: a strip
// over the left or right half of the diamond, anything else centred by its
// own width -- which for a one-tile wall is where a DT1 wall hangs.
func TestAuthoredWallLeft(t *testing.T) {
	for _, c := range []struct {
		typ   d2enum.TileType
		width int
		want  float64
	}{
		{AuthoredStripLeft, 80, -80},
		{AuthoredStripRight, 80, 0},
		{AuthoredWallType, 160, -80},
		{AuthoredWallType, 480, -240},
	} {
		if got := AuthoredWallLeft(c.typ, c.width); got != c.want {
			t.Errorf("AuthoredWallLeft(%v, %d) = %v, want %v", c.typ, c.width, got, c.want)
		}
	}

	// Both strips are upper walls (drawn with the people around them), not
	// roofs, specials or lower walls.
	for _, typ := range []d2enum.TileType{AuthoredStripLeft, AuthoredStripRight, AuthoredWallType} {
		if !typ.UpperWall() || typ.LowerWall() || typ.Special() || typ == d2enum.TileRoof {
			t.Errorf("%v is not a plain upper wall", typ)
		}
	}
}
