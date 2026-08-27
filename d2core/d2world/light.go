package d2world

import (
	"fmt"
	"math"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// LightDials are the light model's tunable numbers. The defaults come from
// S1 §4's source table as grounded by E6: light in 1462 was brief, dim and
// fat-fuelled, and E6 §2 asks that it stay "short and precious, not
// generous". D5 (campaign) owns how dark an isometric night can go and stay
// readable; the playtest answers these.
type LightDials struct {
	// FloorLevel is the ambient at the deep-night floor with no moon: not
	// quite black, because a screen the player cannot read at all is a bug
	// and not a mood (D5).
	FloorLevel float64
	// MoonLevel is how much a full moon lifts the night floor.
	MoonLevel float64

	// FloorRadius is what the player can see at the floor, in world tiles —
	// S1 §4: "the tile they stand on and little else".
	FloorRadius float64
	// DayRadius is the visible radius in full daylight, in world tiles. It
	// only has to exceed the screen.
	DayRadius float64

	// TorchRadius and TorchBurn are S1 §4's carried light: [medium] radius,
	// [60] world minutes. E6 §1 notes the real torch burned ~30 minutes and
	// reached ~20 feet; 60 is the game-dial S1 set and E6 endorsed, on the
	// condition it stays a ration rather than a supply.
	TorchRadius float64
	TorchBurn   float64

	// HearthRadius is the placed fire's [large] radius. Its fuel is a
	// daytime verb the slice's inventory work owns, so in M4.1 a hearth
	// burns until something puts it out.
	HearthRadius float64

	// FalloffStart is the fraction of a source's radius that is fully lit
	// before the light begins to fall off to nothing at the edge.
	FalloffStart float64

	// Steps quantises the level the renderer sees, so the night reads as
	// deliberate bands rather than mush.
	Steps int
}

// DefaultLightDials returns the M4.1 starting dials.
func DefaultLightDials() LightDials {
	return LightDials{
		FloorLevel:   0.10,
		MoonLevel:    0.14,
		FloorRadius:  1.5,
		DayRadius:    64,
		TorchRadius:  5,
		TorchBurn:    60,
		HearthRadius: 8,
		FalloffStart: 0.6,
		Steps:        16,
	}
}

// SourceKind names a kind of light. E6 §1's roster is wider than this; the
// slice's v0 is a carried light and a placed fire (S1 §4).
type SourceKind string

// The light sources of the slice's v0.
const (
	SourceTorch  SourceKind = "torch"
	SourceHearth SourceKind = "hearth"
)

// Source is one light in the world.
type Source struct {
	ID     int        // stable, assigned in creation order
	Kind   SourceKind //
	Radius float64    // world tiles
	Burn   float64    // world minutes remaining; negative means it does not burn down
	Lit    bool       //
	// Carried lights follow the player and occupy the off-hand (S1 §4). The
	// off-hand *slot* is inventory work Phase 6 owns; M4.1 models the light.
	Carried bool
	X, Y    float64 // world tiles, for placed lights
}

// burns reports whether this source consumes fuel.
func (s *Source) burns() bool { return s.Burn >= 0 }

// Light is the world's light model (S1 §4). Ambient falls with the clock to a
// floor set by the moon; sources restore a radius around themselves and burn
// down per world minute.
//
// It is the single source of truth S1 §4 requires: the renderer reads Level
// to darken the world, and — when M4.5 arrives — the combat resolver reads
// the same values, so that "I can't see it" and "I can't hit it" are one
// fact.
//
// Not safe for concurrent use; it lives on the game goroutine.
type Light struct {
	dials   LightDials
	clock   *Clock
	sources []*Source
	nextID  int

	playerX, playerY float64
}

// NewLight returns a light model reading the given clock and registers it as
// the harness "light" provider.
func NewLight(clock *Clock, dials LightDials) *Light {
	l := &Light{dials: dials, clock: clock, nextID: 1}
	d2harness.Register(l)

	return l
}

// Close unregisters the provider.
func (l *Light) Close() { d2harness.Unregister(l) }

// SetPlayer tells the model where the player is, in world tiles. Carried
// lights are there.
func (l *Light) SetPlayer(x, y float64) { l.playerX, l.playerY = x, y }

// Advance burns the lit sources down by the world minutes that just passed
// and extinguishes the ones that reach zero.
func (l *Light) Advance(worldMinutes float64) {
	if worldMinutes <= 0 {
		return
	}

	for _, s := range l.sources {
		if !s.Lit || !s.burns() {
			continue
		}

		s.Burn -= worldMinutes
		if s.Burn <= 0 {
			s.Burn = 0
			s.Lit = false
		}
	}
}

// Add puts a source in the world and returns it.
func (l *Light) Add(kind SourceKind, carried bool, x, y float64) *Source {
	s := &Source{ID: l.nextID, Kind: kind, Carried: carried, X: x, Y: y, Lit: true}
	l.nextID++

	switch kind {
	case SourceHearth:
		s.Radius, s.Burn = l.dials.HearthRadius, -1
	case SourceTorch:
		s.Radius, s.Burn = l.dials.TorchRadius, l.dials.TorchBurn
	default:
		s.Radius, s.Burn = l.dials.TorchRadius, l.dials.TorchBurn
	}

	l.sources = append(l.sources, s)

	return s
}

// Remove drops a source by ID.
func (l *Light) Remove(id int) bool {
	for i, s := range l.sources {
		if s.ID != id {
			continue
		}

		l.sources = append(l.sources[:i], l.sources[i+1:]...)

		return true
	}

	return false
}

// Carried returns the player's carried source, or nil.
func (l *Light) Carried() *Source {
	for _, s := range l.sources {
		if s.Carried {
			return s
		}
	}

	return nil
}

// Ambient is the light the sky gives, [FloorLevel+moon .. 1]. Daylight is
// full; dawn and dusk ramp; the deep night floors out at the moon.
func (l *Light) Ambient() float64 {
	floor := l.nightFloor()
	d := l.clock.dials
	m := l.clock.MinuteOfDay()

	switch l.clock.Stage() {
	case StageDay:
		return 1
	case StageNight:
		return floor
	case StageDawn:
		return lerp(floor, 1, ratio(m, d.DawnStart, d.DayStart))
	case StageDusk:
		return lerp(1, floor, ratio(m, d.DuskStart, d.NightStart))
	default:
		return floor
	}
}

func (l *Light) nightFloor() float64 {
	return clamp01(l.dials.FloorLevel + l.dials.MoonLevel*l.clock.Moon())
}

// Level returns the light at a world tile, 0 (pitch) to 1 (full daylight) —
// the one function the renderer and the combat resolver both call.
//
// The result is quantised into Steps so the night reads as deliberate bands
// rather than mush, which means Level never returns exactly Ambient(): the
// ambient is the continuous sky, Level is what a tile is drawn at. Radius()
// is computed from the continuous value, so the dials stay exact.
func (l *Light) Level(tileX, tileY int) float64 {
	best := l.Ambient()

	for _, s := range l.sources {
		if !s.Lit {
			continue
		}

		x, y := l.at(s)

		if c := l.contribution(float64(tileX)+0.5, float64(tileY)+0.5, x, y, s.Radius); c > best {
			best = c
		}
	}

	return l.quantise(clamp01(best))
}

// at is where a source actually shines from: a carried light is wherever the
// player is now, not where it was lit.
func (l *Light) at(s *Source) (x, y float64) {
	if s.Carried {
		return l.playerX, l.playerY
	}

	return s.X, s.Y
}

// tileOf is the index of the tile containing a world-tile coordinate.
func tileOf(v float64) int { return int(math.Floor(v)) }

// contribution is a source's light at a point: full inside FalloffStart of
// the radius, falling linearly to nothing at the edge.
func (l *Light) contribution(px, py, sx, sy, radius float64) float64 {
	if radius <= 0 {
		return 0
	}

	dist := math.Hypot(px-sx, py-sy)
	if dist >= radius {
		return 0
	}

	inner := radius * l.dials.FalloffStart
	if dist <= inner {
		return 1
	}

	return 1 - (dist-inner)/(radius-inner)
}

func (l *Light) quantise(v float64) float64 {
	if l.dials.Steps <= 1 {
		return v
	}

	steps := float64(l.dials.Steps)

	return math.Round(v*steps) / steps
}

// Radius is what the player can currently see, in world tiles — the value
// S1 §4's playtest assertion is written in. It is the larger of what the sky
// gives and what the brightest light on the player gives.
func (l *Light) Radius() float64 {
	r := lerp(l.dials.FloorRadius, l.dials.DayRadius, l.skyFraction())

	for _, s := range l.sources {
		if !s.Lit {
			continue
		}

		x, y := l.at(s)

		// A placed light only helps if the player is inside it.
		if !s.Carried && math.Hypot(l.playerX-x, l.playerY-y) > s.Radius {
			continue
		}

		if s.Radius > r {
			r = s.Radius
		}
	}

	return r
}

// skyFraction maps the ambient from the night floor (0) to full day (1).
func (l *Light) skyFraction() float64 {
	floor := l.nightFloor()
	if floor >= 1 {
		return 1
	}

	return clamp01((l.Ambient() - floor) / (1 - floor))
}

func ratio(v, lo, hi float64) float64 {
	if hi <= lo {
		return 1
	}

	return clamp01((v - lo) / (hi - lo))
}

func lerp(a, b, t float64) float64 { return a + (b-a)*clamp01(t) }

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// ------------------------------------------------------------- the provider --

// HarnessName identifies the provider (P3 §3.5).
func (l *Light) HarnessName() string { return "light" }

// HarnessState exposes everything S1 §4's assertion needs: the visible
// radius, the carried source and its remaining fuel, and the ambient the
// radius came from.
//
// It also reports Level where it matters, because Radius() is player-centric
// by construction and a light the player is standing outside of moves none of
// it: without player_level and each source's level_here, a placed source is
// unassertable in the model and only the pixels can see it. That was half of
// what M4.1 shipped wrong (reopening 3c9ff9f3-d21e-81e2-aed3-e78a47cd9a40).
func (l *Light) HarnessState() map[string]interface{} {
	state := map[string]interface{}{
		"radius":         l.Radius(),
		"ambient":        l.Ambient(),
		"night_floor":    l.nightFloor(),
		"player_level":   l.Level(tileOf(l.playerX), tileOf(l.playerY)),
		"stage":          l.clock.Stage().String(),
		"moon":           l.clock.Moon(),
		"sources":        len(l.sources),
		"lit_sources":    l.litCount(),
		"carried_source": "",
		"carried_burn":   0.0,
		"carried_lit":    false,
	}

	if c := l.Carried(); c != nil {
		state["carried_source"] = string(c.Kind)
		state["carried_burn"] = c.Burn
		state["carried_lit"] = c.Lit
	}

	// Sorted by ID so the digest is stable.
	sorted := make([]*Source, len(l.sources))
	copy(sorted, l.sources)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	list := make([]map[string]interface{}, 0, len(sorted))

	for _, s := range sorted {
		// x/y is where the source shines from, not where it was created: a
		// carried light reports the player's position, which is the only
		// position it has ever had.
		x, y := l.at(s)

		list = append(list, map[string]interface{}{
			"id": s.ID, "kind": string(s.Kind), "lit": s.Lit,
			"burn": s.Burn, "radius": s.Radius, "carried": s.Carried,
			"x": x, "y": y,
			"level_here": l.Level(tileOf(x), tileOf(y)),
		})
	}

	state["source_list"] = list

	return state
}

func (l *Light) litCount() int {
	n := 0

	for _, s := range l.sources {
		if s.Lit {
			n++
		}
	}

	return n
}

// HarnessSettableFields lists the test-setup writes the light system allows.
func (l *Light) HarnessSettableFields() []string {
	return []string{"carried_burn", "carried_lit", "carried_source", "place_source"}
}

// HarnessSet writes one allow-listed field. "carried_source" is the harness's
// give-the-player-a-torch verb (P3 §1 names it as a light requirement);
// setting it to "" takes the light away.
func (l *Light) HarnessSet(field string, value interface{}) error {
	switch field {
	case "carried_source":
		kind, ok := value.(string)
		if !ok {
			return fmt.Errorf("carried_source wants a string, got %T", value)
		}

		if c := l.Carried(); c != nil {
			l.Remove(c.ID)
		}

		if kind == "" {
			return nil
		}

		if kind != string(SourceTorch) && kind != string(SourceHearth) {
			return fmt.Errorf("no light source %q (torch, hearth)", kind)
		}

		l.Add(SourceKind(kind), true, l.playerX, l.playerY)

		return nil

	case "carried_burn":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("carried_burn wants a number, got %T", value)
		}

		c := l.Carried()
		if c == nil {
			return fmt.Errorf("the player carries no light; set carried_source first")
		}

		c.Burn = f
		c.Lit = c.Lit && f > 0

		return nil

	case "carried_lit":
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("carried_lit wants a bool, got %T", value)
		}

		c := l.Carried()
		if c == nil {
			return fmt.Errorf("the player carries no light; set carried_source first")
		}

		c.Lit = b && (!c.burns() || c.Burn > 0)

		return nil

	case "place_source":
		return l.placeSource(value)

	default:
		return fmt.Errorf("no settable field %q", field)
	}
}

// placeSource is the harness's put-a-fire-over-there verb: the value is an
// object, {"kind": "hearth", "x": 22, "y": 14}, in world tiles.
//
// It exists because M4.1 shipped without it. Add has always taken a position
// and light_test.go has always exercised placed sources, but the only
// non-test caller was carried_source, which hardcodes the player's — so the
// source_list this provider reports could never hold anything but the
// player's own torch, and the camp's campfire stayed dark. A provider that
// reports a collection needs a verb that can put something in it, or the
// reporting is decoration (reopening 3c9ff9f3-d21e-81e2-aed3-e78a47cd9a40).
//
// Placed lights are not carried: they stay where they are put, and they help
// the player only when he walks into them.
func (l *Light) placeSource(value interface{}) error {
	fields, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`place_source wants an object like {"kind": "hearth", "x": 22, "y": 14} `+
			`with x and y in world tiles, got %T`, value)
	}

	kind, ok := fields["kind"].(string)
	if !ok {
		return fmt.Errorf(`place_source: "kind" wants a string (torch, hearth), got %T`, fields["kind"])
	}

	if kind != string(SourceTorch) && kind != string(SourceHearth) {
		return fmt.Errorf("no light source %q (torch, hearth)", kind)
	}

	x, ok := toFloat(fields["x"])
	if !ok {
		return fmt.Errorf(`place_source: "x" wants a number of world tiles, got %T`, fields["x"])
	}

	y, ok := toFloat(fields["y"])
	if !ok {
		return fmt.Errorf(`place_source: "y" wants a number of world tiles, got %T`, fields["y"])
	}

	l.Add(SourceKind(kind), false, x, y)

	return nil
}
