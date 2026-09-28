package d2world

import (
	"fmt"
	"math"
)

// LightSnapshot is the light model's saved state (M4.6 B2a): every source as
// it stands, in creation order, and the id the next source will take.
//
// Not saved, because they are derived: the carried burn rate (his talents,
// applied again when the load applies his progress), and the player's
// position (the game moves it every frame with SetPlayer). A carried source's
// X and Y are saved as they are but read by nothing -- it shines from wherever
// the player stands (Light.at).
type LightSnapshot struct {
	Sources []LightSourceSnapshot `json:"sources"`
	NextID  int                   `json:"next_id"`
}

// LightSourceSnapshot is one source.
type LightSourceSnapshot struct {
	ID      int        `json:"id"`
	Kind    SourceKind `json:"kind"`
	Radius  float64    `json:"radius"`
	Burn    float64    `json:"burn"`
	Lit     bool       `json:"lit"`
	Carried bool       `json:"carried"`
	X       float64    `json:"x"`
	Y       float64    `json:"y"`
}

// Snapshot is the model as it stands.
func (l *Light) Snapshot() LightSnapshot {
	out := LightSnapshot{Sources: make([]LightSourceSnapshot, 0, len(l.sources)), NextID: l.nextID}

	for _, s := range l.sources {
		out.Sources = append(out.Sources, LightSourceSnapshot{
			ID: s.ID, Kind: s.Kind, Radius: s.Radius, Burn: s.Burn,
			Lit: s.Lit, Carried: s.Carried, X: s.X, Y: s.Y,
		})
	}

	return out
}

// Restore replaces every source with the saved ones, the carried torch and
// its remaining minutes included, and restores the next id. It is checked
// whole before anything changes, so a refused snapshot leaves the model as it
// was.
//
// THIS IS THE MODEL, NOT THE LOAD, and the carried torch is where the two
// differ (plan section 5, trap 4). A lit torch's minutes live HERE while it
// burns and are zero in the kit (the L key moves them); saveKit writes them
// back into the kit's torch for the sidecar. So the world file holds a lit
// torch's minutes twice -- in this snapshot's carried source and in the
// embedded kit -- and the load must take them from ONE place: B4 re-lights
// the torch through the L path, which moves the kit's minutes into the model,
// and restores this snapshot without its carried source. Restoring both
// lights one torch with two torches' minutes. Which one, and when, is B4's;
// this restores exactly what it is given.
func (l *Light) Restore(s LightSnapshot) error {
	if s.NextID < 1 {
		return fmt.Errorf("light snapshot: next_id %d; ids start at 1", s.NextID)
	}

	carried := 0
	last := 0

	for i, src := range s.Sources {
		switch {
		case src.ID <= last:
			return fmt.Errorf("light snapshot: source %d has id %d after id %d; sources are kept in creation order", i, src.ID, last)
		case src.ID >= s.NextID:
			return fmt.Errorf("light snapshot: source id %d is not below next_id %d; a new source would reuse it", src.ID, s.NextID)
		case src.Kind != SourceTorch && src.Kind != SourceHearth:
			return fmt.Errorf("light snapshot: source %d has no kind %q (torch, hearth)", src.ID, src.Kind)
		case !b2aFinite(src.Radius, src.Burn, src.X, src.Y) || src.Radius < 0:
			return fmt.Errorf("light snapshot: source %d has a radius, burn or position that is not a number", src.ID)
		}

		if src.Carried {
			carried++
		}

		last = src.ID
	}

	if carried > 1 {
		return fmt.Errorf("light snapshot: %d carried sources; he has one off-hand", carried)
	}

	sources := make([]*Source, 0, len(s.Sources))
	for _, src := range s.Sources {
		sources = append(sources, &Source{
			ID: src.ID, Kind: src.Kind, Radius: src.Radius, Burn: src.Burn,
			Lit: src.Lit, Carried: src.Carried, X: src.X, Y: src.Y,
		})
	}

	l.sources = sources
	l.nextID = s.NextID

	return nil
}

// b2aFinite reports that every value is a number: not NaN, not infinite.
// JSON cannot carry either, and a snapshot built in code should not either.
func b2aFinite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}

	return true
}
