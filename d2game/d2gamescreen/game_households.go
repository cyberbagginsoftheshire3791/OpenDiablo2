package d2gamescreen

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// villagePlaces is the authored map's village as the households system takes
// it (the raid's R3a): every household with its door, roles, speakers, stock
// and church mark, the hotar and the watch posts. A generated map has none.
func villagePlaces(v d2mapengine.AuthoredVillage) d2world.VillagePlaces {
	places := d2world.VillagePlaces{HasHotar: v.HasHotar, HotarX: v.Hotar.X, HotarY: v.Hotar.Y}

	for _, h := range v.Households {
		places.Households = append(places.Households, d2world.HouseholdPlace{
			Name: h.Name, DoorX: h.Door.X, DoorY: h.Door.Y,
			Members:  append([]string(nil), h.Members...),
			Speakers: append([]string(nil), h.Speakers...),
			Incense:  h.Incense, Stakes: h.Stakes, Church: h.Church,
		})
	}

	for _, p := range v.Posts {
		places.Posts = append(places.Posts, d2world.PostPlace{X: p.At.X, Y: p.At.Y, Post: p.Post})
	}

	return places
}
