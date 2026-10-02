package d2gamescreen

import (
	"image"
	"reflect"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// TestTheHouseholdsAreTheEnginesVillage (the raid's R3a review, B3): the
// households system is given the engine's village field for field -- doors and
// posts by x then y, none its own transpose -- and a village of none is none.
func TestTheHouseholdsAreTheEnginesVillage(t *testing.T) {
	v := d2mapengine.AuthoredVillage{
		Households: []d2mapengine.AuthoredHousehold{
			{Name: "the church", Door: image.Pt(16, 17), Speakers: []string{"akara"}, Incense: 4, Stakes: 1, Church: true},
			{Name: "the smith's", Door: image.Pt(20, 31), Members: []string{"woman", "child"}, Speakers: []string{"charsi"}, Incense: 2, Stakes: 3},
		},
		Posts:    []d2mapengine.AuthoredPost{{At: image.Pt(24, 34), Post: "gate"}, {At: image.Pt(33, 13), Post: "corner"}},
		Hotar:    image.Pt(22, 40),
		HasHotar: true,
	}

	want := d2world.VillagePlaces{
		Households: []d2world.HouseholdPlace{
			{Name: "the church", DoorX: 16, DoorY: 17, Speakers: []string{"akara"}, Incense: 4, Stakes: 1, Church: true},
			{Name: "the smith's", DoorX: 20, DoorY: 31, Members: []string{"woman", "child"}, Speakers: []string{"charsi"}, Incense: 2, Stakes: 3},
		},
		HasHotar: true, HotarX: 22, HotarY: 40,
		Posts: []d2world.PostPlace{{X: 24, Y: 34, Post: "gate"}, {X: 33, Y: 13, Post: "corner"}},
	}

	if got := villagePlaces(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("the households are given\n %+v\nwant\n %+v", got, want)
	}

	if got := villagePlaces(d2mapengine.AuthoredVillage{}); !reflect.DeepEqual(got, d2world.VillagePlaces{}) {
		t.Fatalf("no village gives %+v", got)
	}
}

// TestTheStockBoundIsTheMapsBound (the review's B4): the world file cannot
// hold more than a map can author, nor refuse what it can.
func TestTheStockBoundIsTheMapsBound(t *testing.T) {
	if d2world.HouseholdStockMax != d2maptiled.MaxHouseholdStock {
		t.Fatalf("the households' stock bound is %d and the map format's %d", d2world.HouseholdStockMax, d2maptiled.MaxHouseholdStock)
	}
}
