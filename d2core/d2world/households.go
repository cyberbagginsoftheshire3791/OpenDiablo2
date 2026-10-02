package d2world

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// HOUSEHOLDS: THE VILLAGE AS HOUSES (the raid's R3a, 2 Oct 2026; the raid
// brief's P1). The tenth world system. The rulings of 28 Sep: "The house is a
// place" -- "members, door, kept state, hearth dark / lit / tale" -- and ruling
// 7, "Keeping a house is work someone does each dusk, and it lapses ... incense
// and the priest's time are what is short".
//
// WHERE R3a STOPS. This burst builds the houses from the map -- each
// household's door, its people by role, its church mark and the stock it
// keeps (incense and stakes) -- the village's boundary (the hotar) and its
// watch posts, the stock in the world file, and the provider. Nothing in the
// world spends or renews the stock yet, and nothing reads the houses: the
// members as bodies on the map (R3b), the keeping at 21:15 and the breach (R5),
// the watch and the village's rite (R6) and the carry-out to the hotar (R8)
// are later bursts, each of which adds its own state (and bumps the file).
// A household's members are its ROLES as authored, and its speakers the map's
// npcs that name it; no member has an entity id until R3b makes the bodies.
//
// The map is the truth for everything but the stock (D: a world file of
// another map is refused before this block is read, D5); the stock is the
// house's own (S).

// HouseholdStockMax bounds a house's incense and stakes. It is the map
// format's bound (d2maptiled.MaxHouseholdStock): a file cannot hold more than
// a map could author, and a typo of 300 is refused, not obeyed.
const HouseholdStockMax = 99

// HouseholdPlace is one household as the map places it.
type HouseholdPlace struct {
	// Name is the map's name for the household.
	Name string
	// DoorX, DoorY is the door tile.
	DoorX, DoorY int
	// Members is its people by role (man, woman, old, youth, child), as
	// authored; Speakers is the npcs (by their stand-in's monstat) that name
	// the household, in map order.
	Members  []string
	Speakers []string
	// Incense and Stakes are what the house starts with.
	Incense, Stakes int
	// Church marks the church, the one house that is always kept.
	Church bool
}

// PostPlace is one watch post: its tile and the way in it watches ("gate" or
// "corner").
type PostPlace struct {
	X, Y int
	Post string
}

// VillagePlaces is what the map says about the village: its households in map
// order, its boundary (HasHotar false for none) and its watch posts in map
// order. A generated world has none of it.
type VillagePlaces struct {
	Households     []HouseholdPlace
	HasHotar       bool
	HotarX, HotarY int
	Posts          []PostPlace
}

// house is one household's own state: its stock.
type house struct {
	incense, stakes int
}

// Households is the village's houses.
type Households struct {
	places VillagePlaces
	houses []house
}

// NewHouseholds builds the houses the map places, each with the stock it
// starts with, and registers the "households" provider. A stock past
// HouseholdStockMax or below 0 is refused (the map format already refuses it).
func NewHouseholds(places VillagePlaces) (*Households, error) {
	h := &Households{places: places, houses: make([]house, len(places.Households))}

	for i, p := range places.Households {
		if err := checkStock(p.Name, p.Incense, p.Stakes); err != nil {
			return nil, err
		}

		h.houses[i] = house{incense: p.Incense, stakes: p.Stakes}
	}

	d2harness.Register(h)

	return h, nil
}

// Close unregisters the provider.
func (h *Households) Close() { d2harness.Unregister(h) }

func checkStock(what string, incense, stakes int) error {
	switch {
	case incense < 0 || incense > HouseholdStockMax:
		return fmt.Errorf("household %s: incense %d is not from 0 to %d", what, incense, HouseholdStockMax)
	case stakes < 0 || stakes > HouseholdStockMax:
		return fmt.Errorf("household %s: stakes %d is not from 0 to %d", what, stakes, HouseholdStockMax)
	}

	return nil
}

// HouseholdID is the id of the i-th household (from 0) in map order: "h:1",
// "h:2", ...
func HouseholdID(i int) string { return fmt.Sprintf("h:%d", i+1) }

func (h *Households) index(id string) int {
	for i := range h.houses {
		if HouseholdID(i) == id {
			return i
		}
	}

	return -1
}

// HarnessName names the provider. ("village" is the reputation provider's.)
func (h *Households) HarnessName() string { return "households" }

// HarnessState reports every house in map order -- its id, name, door, roles,
// speakers, church mark, the stock it started with and the stock it holds --
// the hotar, the watch posts and the counts.
func (h *Households) HarnessState() map[string]interface{} {
	houses := make([]map[string]interface{}, 0, len(h.houses))
	churches, members := 0, 0

	for i, p := range h.places.Households {
		if p.Church {
			churches++
		}

		members += len(p.Members) + len(p.Speakers)

		houses = append(houses, map[string]interface{}{
			"id":            HouseholdID(i),
			"name":          p.Name,
			"door":          []int{p.DoorX, p.DoorY},
			"members":       append([]string{}, p.Members...),
			"speakers":      append([]string{}, p.Speakers...),
			"church":        p.Church,
			"incense":       h.houses[i].incense,
			"stakes":        h.houses[i].stakes,
			"incense_start": p.Incense,
			"stakes_start":  p.Stakes,
		})
	}

	posts := make([]map[string]interface{}, 0, len(h.places.Posts))
	for _, p := range h.places.Posts {
		posts = append(posts, map[string]interface{}{"post": p.Post, "at": []int{p.X, p.Y}})
	}

	var hotar interface{}
	if h.places.HasHotar {
		hotar = []int{h.places.HotarX, h.places.HotarY}
	}

	return map[string]interface{}{
		"houses":     houses,
		"households": len(houses),
		"churches":   churches,
		"members":    members,
		"hotar":      hotar,
		"posts":      posts,
	}
}

// HarnessSettableFields lists the writes the system allows: one house's
// incense or stakes, as {"house": "h:<n>", "value": n} -- test set-up, the way
// the village provider's rep is, so a script can run a house down.
func (h *Households) HarnessSettableFields() []string { return []string{"incense", "stakes"} }

// HarnessSet applies one write.
func (h *Households) HarnessSet(field string, value interface{}) error {
	if field != "incense" && field != "stakes" {
		return fmt.Errorf("households has no settable field %q", field)
	}

	m, ok := value.(map[string]interface{})
	if !ok || len(m) != 2 {
		return fmt.Errorf("%s wants {\"house\": \"h:<n>\", \"value\": n}, got %v", field, value)
	}

	id, _ := m["house"].(string)

	i := h.index(id)
	if i < 0 {
		return fmt.Errorf("%s: %q is not a household (the map places %d)", field, m["house"], len(h.houses))
	}

	n, ok := m["value"].(float64)
	if !ok || n != math.Trunc(n) || n < 0 || n > HouseholdStockMax {
		return fmt.Errorf("%s wants a whole number from 0 to %d, got %v", field, HouseholdStockMax, m["value"])
	}

	if field == "incense" {
		h.houses[i].incense = int(n)
	} else {
		h.houses[i].stakes = int(n)
	}

	return nil
}
