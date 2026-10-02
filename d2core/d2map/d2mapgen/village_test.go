package d2mapgen

import (
	"image"
	"reflect"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// TestTheEngineKeepsTheMapsVillage (the raid's R3a review, B3): the village
// the engine is handed is the map's -- each household with exactly the npcs
// that name it, in map order, its door, roles, stock and church; the hotar;
// the posts -- with no coordinate its own transpose, so a swap is seen.
func TestTheEngineKeepsTheMapsVillage(t *testing.T) {
	m := &d2maptiled.Map{
		NPCs: []d2maptiled.NPC{
			{Monstat: "charsi", Household: "the smith's"},
			{Monstat: "kashya"},
			{Monstat: "akara", Household: "the church"},
			{Monstat: "warriv1", Household: "the smith's"},
		},
		Households: []d2maptiled.Household{
			{Name: "the church", Door: image.Pt(16, 17), Incense: 4, Stakes: 1, Church: true},
			{Name: "the smith's", Door: image.Pt(20, 31), Members: []string{"woman", "child"}, Incense: 2, Stakes: 3},
		},
		Hotar: image.Pt(22, 40), HasHotar: true,
		Posts: []d2maptiled.Post{{At: image.Pt(24, 34), Post: d2maptiled.PostGate}, {At: image.Pt(33, 13), Post: d2maptiled.PostCorner}},
	}

	want := d2mapengine.AuthoredVillage{
		Households: []d2mapengine.AuthoredHousehold{
			{Name: "the church", Door: image.Pt(16, 17), Speakers: []string{"akara"}, Incense: 4, Stakes: 1, Church: true},
			{Name: "the smith's", Door: image.Pt(20, 31), Members: []string{"woman", "child"}, Speakers: []string{"charsi", "warriv1"},
				Incense: 2, Stakes: 3},
		},
		Posts:    []d2mapengine.AuthoredPost{{At: image.Pt(24, 34), Post: "gate"}, {At: image.Pt(33, 13), Post: "corner"}},
		Hotar:    image.Pt(22, 40),
		HasHotar: true,
	}

	if got := authoredVillage(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("the engine's village\n %+v\nwant\n %+v", got, want)
	}

	if got := authoredVillage(&d2maptiled.Map{}); !reflect.DeepEqual(got, d2mapengine.AuthoredVillage{}) {
		t.Fatalf("a map with no village gives %+v", got)
	}
}
