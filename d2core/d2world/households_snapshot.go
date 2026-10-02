package d2world

import "fmt"

// THE HOUSEHOLDS' SNAPSHOT (the raid's R3a; B2's conventions, D2: a value
// derived from the map or from dials is never saved). Saved: each house's
// stock, by id in map order. Not saved: the doors, roles, speakers, church
// mark, hotar and posts -- the map's (D), guarded by the map's SHA (D5: a file
// of another map is refused before this block is read).

// HouseholdsSnapshot is the households' saved state.
type HouseholdsSnapshot struct {
	Houses []HouseSnapshot `json:"houses"`
}

// HouseSnapshot is one house's stock.
type HouseSnapshot struct {
	ID      string `json:"id"`
	Incense int    `json:"incense"`
	Stakes  int    `json:"stakes"`
}

// Snapshot is every house's stock, in map order. A village of no households
// is an empty list, never null.
func (h *Households) Snapshot() HouseholdsSnapshot {
	houses := make([]HouseSnapshot, len(h.houses))

	for i, s := range h.houses {
		houses[i] = HouseSnapshot{ID: HouseholdID(i), Incense: s.incense, Stakes: s.stakes}
	}

	return HouseholdsSnapshot{Houses: houses}
}

// Check is every refusal the block can make with no map to hand: ids h:1,
// h:2, ... in order, each stock from 0 to HouseholdStockMax. The world file's
// own check (d2save) asks it, and Validate does.
func (s HouseholdsSnapshot) Check() error {
	for i, house := range s.Houses {
		if want := HouseholdID(i); house.ID != want {
			return fmt.Errorf("households snapshot: house %d is %q; the houses are h:1, h:2, ... in map order, so it is %q", i, house.ID, want)
		}

		if err := checkStock(house.ID, house.Incense, house.Stakes); err != nil {
			return fmt.Errorf("households snapshot: %v", err)
		}
	}

	return nil
}

// Validate is Restore's check and nothing else (D4): the block is one Check
// takes, of exactly the households this map places.
func (h *Households) Validate(s HouseholdsSnapshot) error {
	if err := s.Check(); err != nil {
		return err
	}

	if len(s.Houses) != len(h.houses) {
		return fmt.Errorf("households snapshot: %d houses, and this map places %d", len(s.Houses), len(h.houses))
	}

	return nil
}

// Restore puts every house's stock back (after Validate; a refused snapshot
// leaves the houses as they were).
func (h *Households) Restore(s HouseholdsSnapshot) error {
	if err := h.Validate(s); err != nil {
		return err
	}

	for i, saved := range s.Houses {
		h.houses[i] = house{incense: saved.Incense, stakes: saved.Stakes}
	}

	return nil
}
