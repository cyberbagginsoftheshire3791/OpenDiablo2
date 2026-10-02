package d2mapengine

import "image"

// AuthoredHousehold is one household an authored map places (the raid's R3a;
// d2maptiled.Household): its name, door tile, people by role, the npcs that
// name it (by monstat, in map order), its stock and its church mark.
type AuthoredHousehold struct {
	Name            string
	Door            image.Point
	Members         []string
	Speakers        []string
	Incense, Stakes int
	Church          bool
}

// AuthoredPost is one watch post an authored map places: its tile and the
// way in it watches ("gate" or "corner").
type AuthoredPost struct {
	At   image.Point
	Post string
}

// AuthoredVillage is what an authored map says about its village (the raid's
// R3a): its households and watch posts in map order, and its hotar -- the
// boundary its dead are carried out to -- if it has one. A generated map has
// none of it.
type AuthoredVillage struct {
	Households []AuthoredHousehold
	Posts      []AuthoredPost
	Hotar      image.Point
	HasHotar   bool
}

// SetVillage records an authored map's village. ResetMap clears it.
func (m *MapEngine) SetVillage(v AuthoredVillage) {
	m.village = v
}

// Village is the authored map's village; the zero value on a generated map.
func (m *MapEngine) Village() AuthoredVillage {
	return m.village
}
