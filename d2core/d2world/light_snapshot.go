package d2world

import (
	"fmt"
	"math"
)

// LightSnapshot is the light model's saved state (M4.6 B2a): every source as
// it stands, in creation order, and the id the next source will take.
//
// Not saved, because they are derived: the carried burn rate (his talents,
// applied again when the load applies his progress), the player's position
// (the game moves it every frame with SetPlayer), and each source's RADIUS,
// which is a dial by kind (TorchRadius, HearthRadius) and is set from the
// current dials at restore (D2, 28 Sep 2026: a value derived from dials is
// never saved; a save that carried it would carry this build's tuning into the
// next). A carried source's X and Y are saved as they are but read by nothing
// -- it shines from wherever the player stands (Light.at).
type LightSnapshot struct {
	Sources []LightSourceSnapshot `json:"sources"`
	NextID  int                   `json:"next_id"`
}

// LightSourceSnapshot is one source.
type LightSourceSnapshot struct {
	ID      int        `json:"id"`
	Kind    SourceKind `json:"kind"`
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
			ID: s.ID, Kind: s.Kind, Burn: s.Burn, Lit: s.Lit, Carried: s.Carried, X: s.X, Y: s.Y,
		})
	}

	return out
}

// Restore replaces every source with the saved ones, the carried torch and
// its remaining minutes included, and restores the next id. Each source's
// radius is set from the model's own dials by its kind, exactly as Add sets
// it. It is checked whole first (Validate), so a refused snapshot leaves the
// model as it was.
//
// THE LIGHT MODEL IS THE TRUTH ON LOAD (D1, 28 Sep 2026; plan section 5, trap
// 4). A lit torch's minutes live HERE while it burns; the kit's torch holds
// zero (the L key moves them). But saveKit writes the carried source's burn
// back into the kit's torch for the sidecar -- lit or DOUSED, since a doused
// torch keeps its burn -- so the world file holds those minutes twice: in
// this snapshot's carried source and in the embedded kit. The load takes them
// from HERE: it restores this snapshot whole, carried source included, and
// ignores the kit torch's minutes whenever a world file is loaded (B4 zeroes
// the off-hand torch's BurnLeft in the kit it binds while this snapshot holds
// a carried source, as the L key would have). It does NOT re-light through
// the L path: that would give the torch a new id and move next_id, light a
// doused torch (L toggles), and count a torch verb (ui.torch_verbs) the saved
// game never made. TestLightRestoresTheCarriedTorchExactly holds the model's
// half; d2player's TestALoadedTorchIsNotRelit holds the controls' half.
func (l *Light) Restore(s LightSnapshot) error {
	if err := l.Validate(s); err != nil {
		return err
	}

	sources := make([]*Source, 0, len(s.Sources))
	for _, src := range s.Sources {
		sources = append(sources, &Source{
			ID: src.ID, Kind: src.Kind, Radius: l.radiusOf(src.Kind), Burn: src.Burn,
			Lit: src.Lit, Carried: src.Carried, X: src.X, Y: src.Y,
		})
	}

	l.sources = sources
	l.nextID = s.NextID

	return nil
}

// Validate is Restore's check and nothing else (D4): every refusal Restore
// would make, made without changing the model, so a load can check every
// block of a world file before it restores any.
//
// It accepts only what Snapshot could have written (the B2a review's B2):
// ids in creation order below next_id, a known kind, at most one carried
// source, and a burn that fits the kind -- a torch burns down from its dial
// and goes out at zero (Advance), so its burn is never negative and a lit one
// always has minutes left; a hearth never burns down, which Add marks with a
// negative burn (-1).
func (l *Light) Validate(s LightSnapshot) error {
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
		case !b2aFinite(src.Burn, src.X, src.Y):
			return fmt.Errorf("light snapshot: source %d has a burn or position that is not a number", src.ID)
		case src.Kind == SourceTorch && src.Burn < 0:
			return fmt.Errorf("light snapshot: torch %d burns %v; a torch burns down from its dial and is never negative", src.ID, src.Burn)
		case src.Kind == SourceTorch && src.Lit && src.Burn == 0:
			return fmt.Errorf("light snapshot: torch %d is lit with no minutes left; a torch goes out at zero", src.ID)
		case src.Kind == SourceHearth && src.Burn >= 0:
			return fmt.Errorf("light snapshot: hearth %d burns %v; a hearth is fuel-fed and never burns down (negative)", src.ID, src.Burn)
		}

		if src.Carried {
			carried++
		}

		last = src.ID
	}

	if carried > 1 {
		return fmt.Errorf("light snapshot: %d carried sources; he has one off-hand", carried)
	}

	return nil
}

// radiusOf is a kind's radius from the model's dials, as Add sets it.
func (l *Light) radiusOf(kind SourceKind) float64 {
	if kind == SourceHearth {
		return l.dials.HearthRadius
	}

	return l.dials.TorchRadius
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
