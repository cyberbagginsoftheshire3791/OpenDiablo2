package d2world

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// Survival meters (S1 §5, M4.2). Three meters drain with the world clock and
// are replenished by consuming; neglect kills. Light is the fourth resource
// and deliberately not a meter — M4.1 built it as burn time (S1 §4).
//
// A NOTE ON DIRECTION, because it is the easiest thing here to get backwards:
// Food and Water run 100 (full) down to 0 (empty). **Fatigue runs the other
// way**, 0 (rested) up to 100 (exhausted), because S1 §5's thresholds are
// written as "at fatigue ≥ 75%" and "at food = 0". Resting lowers fatigue.

// MeterDials are the survival model's tunable numbers. Every one is a [DIAL]
// the playtest and the Phase 6 checkpoint answer. E2's instruction shapes
// them: food scarcity in the slice is meant to come from the authored world —
// scorched stores, June spoilage, two overlapping fasts — and not from a
// punishing drain rate, so these are gentle on purpose.
type MeterDials struct {
	// Drain per world hour, in meter points.
	FoodDrain    float64
	WaterDrain   float64
	FatigueDrain float64

	// DaylightWaterFactor multiplies thirst while the sun is up: S1 §5's
	// "faster in daylight heat (it is late June)".
	DaylightWaterFactor float64

	// LabourFoodFactor and LabourWaterFactor multiply the drain while
	// digging, fighting or carrying (S1 §5). WatchFatigueFactor does the
	// same for the night watch and hearth-tales.
	LabourFoodFactor   float64
	LabourWaterFactor  float64
	WatchFatigueFactor float64

	// WarnLevel is where Food/Water read as Hungry/Thirsty.
	WarnLevel float64

	// HungryFatigueFactor is S1 §5's "Hungry: fatigue drains faster".
	HungryFatigueFactor float64

	// NoReactionFatigue is R2 §1's fatigue rule: at or above it the body has
	// no Reaction to give. ShakenFatigue is where fights start Shaken, and
	// ThirstyShakenFatigue is that threshold lowered by thirst (S1 §5).
	NoReactionFatigue    float64
	ShakenFatigue        float64
	ThirstyShakenFatigue float64

	// NeglectDamage is health lost per world hour per empty meter.
	NeglectDamage float64

	// RestRate is the fatigue removed per world hour of rest.
	RestRate float64
}

// DefaultMeterDials returns the M4.2 starting dials (build note §4).
func DefaultMeterDials() MeterDials {
	return MeterDials{
		FoodDrain:            3.0,
		WaterDrain:           4.5,
		FatigueDrain:         3.5,
		DaylightWaterFactor:  1.5,
		LabourFoodFactor:     2.0,
		LabourWaterFactor:    1.5,
		WatchFatigueFactor:   1.5,
		WarnLevel:            33,
		HungryFatigueFactor:  1.5,
		NoReactionFatigue:    75,
		ShakenFatigue:        90,
		ThirstyShakenFatigue: 80,
		NeglectDamage:        2.0,
		RestRate:             12.5,
	}
}

// Activity is what the body is doing, which changes what it spends. The
// verbs that would set it (digging, fighting, carrying, standing watch) do
// not exist yet — M4.3 and M4.5 bring them — so in M4.2 the harness is the
// only caller. A dial with no way to exercise it is dead wiring, which is
// the mistake M4.1 was reopened for.
type Activity string

// The activities of the slice's v0.
const (
	ActivityIdle   Activity = "idle"
	ActivityLabour Activity = "labour" // digging, fighting, carrying (S1 §5)
	ActivityWatch  Activity = "watch"  // the night watch and hearth-tales
)

// meterFull is the top of every meter's range.
const meterFull = 100.0

// Body is what the meters need from the thing they are keeping alive: a
// health value they can read and decrement. The game screen adapts the
// player's hero stats to it, so this package imports no entity or hero code
// and stays what Clock and Light are — plain arithmetic over a stepped
// delta, linking zero ebiten.
//
// A nil Body means neglect is still modelled and reported; nothing takes
// damage. That is the state before a game screen exists.
type Body interface {
	CurrentHealth() int
	MaxHealth() int
	SetHealth(int)
}

// Meters is the survival model (S1 §5). It drains on the world clock, kills
// by neglect, and exposes the states R2's combat rules are written in —
// ReactionAvailable and Shaken — so that M4.5's resolver reads the same
// fact the body reports rather than recomputing it. One source of truth, the
// rule S1 §4 set for light and M4.1 delivered.
//
// Not safe for concurrent use; it lives on the game goroutine.
type Meters struct {
	dials MeterDials
	clock *Clock
	body  Body

	food     float64 // 100 full → 0 empty
	water    float64 // 100 full → 0 empty
	fatigue  float64 // 0 rested → 100 exhausted
	activity Activity

	// damage carries the fraction of a health point owed between steps, so
	// neglect is smooth in world minutes rather than lumpy per call.
	damage float64
}

// NewMeters returns a full, rested body reading the given clock, and
// registers it as the harness "meters" provider.
func NewMeters(clock *Clock, dials MeterDials) *Meters {
	m := &Meters{
		dials:    dials,
		clock:    clock,
		food:     meterFull,
		water:    meterFull,
		fatigue:  0,
		activity: ActivityIdle,
	}
	d2harness.Register(m)

	return m
}

// Close unregisters the provider.
func (m *Meters) Close() { d2harness.Unregister(m) }

// SetBody gives the meters the health that neglect damages. Passing nil
// takes it away again.
func (m *Meters) SetBody(body Body) { m.body = body }

// Advance drains the meters by the world minutes that just passed and
// applies neglect damage. A dead body stops spending.
func (m *Meters) Advance(worldMinutes float64) {
	if worldMinutes <= 0 || m.Dead() {
		return
	}

	hours := worldMinutes / minutesPerHour

	food := m.dials.FoodDrain * hours
	water := m.dials.WaterDrain * hours
	fatigue := m.dials.FatigueDrain * hours

	switch m.activity {
	case ActivityLabour:
		food *= m.dials.LabourFoodFactor
		water *= m.dials.LabourWaterFactor
	case ActivityWatch:
		fatigue *= m.dials.WatchFatigueFactor
	case ActivityIdle:
	}

	if m.clock != nil && m.clock.Stage() == StageDay {
		water *= m.dials.DaylightWaterFactor
	}

	// Hunger is charged before the meal is: an empty stomach tires you over
	// the hour you spent emptying it (S1 §5).
	if m.Hungry() {
		fatigue *= m.dials.HungryFatigueFactor
	}

	m.food = clampMeter(m.food - food)
	m.water = clampMeter(m.water - water)
	m.fatigue = clampMeter(m.fatigue + fatigue)

	m.applyNeglect(hours)
}

// applyNeglect takes health for each empty meter. Two empty meters kill
// twice as fast, which is the reading S1 §5 wants.
func (m *Meters) applyNeglect(hours float64) {
	empty := 0
	if m.Starving() {
		empty++
	}

	if m.Parched() {
		empty++
	}

	if empty == 0 || m.body == nil {
		return
	}

	m.damage += m.dials.NeglectDamage * float64(empty) * hours

	whole := int(m.damage)
	if whole <= 0 {
		return
	}

	m.damage -= float64(whole)

	health := m.body.CurrentHealth() - whole
	if health < 0 {
		health = 0
	}

	m.body.SetHealth(health)
}

// Consume replenishes a meter: "food" and "water" raise theirs, "rest"
// lowers fatigue. Amount is in meter points — an hour of sleep is RestRate
// of rest. It is the other direction of the same arithmetic Advance runs,
// and it exists because a provider that reports a value needs a verb that
// can move it BOTH ways, or a script can only ever watch the number fall.
func (m *Meters) Consume(kind string, amount float64) error {
	if amount < 0 {
		return fmt.Errorf("consume wants a positive amount, got %v", amount)
	}

	switch kind {
	case "food":
		m.food = clampMeter(m.food + amount)
	case "water":
		m.water = clampMeter(m.water + amount)
	case "rest":
		m.fatigue = clampMeter(m.fatigue - amount)
	default:
		return fmt.Errorf("no consumable %q (food, water, rest)", kind)
	}

	return nil
}

// Food, Water and Fatigue are the raw meter values.
func (m *Meters) Food() float64    { return m.food }
func (m *Meters) Water() float64   { return m.water }
func (m *Meters) Fatigue() float64 { return m.fatigue }

// Activity reports what the body is currently doing.
func (m *Meters) Activity() Activity { return m.activity }

// SetActivity changes what the body is doing.
func (m *Meters) SetActivity(a Activity) { m.activity = a }

// Hungry and Thirsty are the warning bands; Starving and Parched are where
// neglect starts taking health.
func (m *Meters) Hungry() bool   { return m.food <= m.dials.WarnLevel }
func (m *Meters) Starving() bool { return m.food <= 0 }
func (m *Meters) Thirsty() bool  { return m.water <= m.dials.WarnLevel }
func (m *Meters) Parched() bool  { return m.water <= 0 }

// Dying reports that at least one meter is empty and health is being spent.
func (m *Meters) Dying() bool { return m.Starving() || m.Parched() }

// Dead reports that the body ran out of health. With no Body attached
// nothing can die, so the meters keep draining and reporting.
func (m *Meters) Dead() bool { return m.body != nil && m.body.CurrentHealth() <= 0 }

// ShakenThreshold is the fatigue at which fights start Shaken. Thirst lowers
// it (S1 §5).
func (m *Meters) ShakenThreshold() float64 {
	if m.Thirsty() {
		return m.dials.ThirstyShakenFatigue
	}

	return m.dials.ShakenFatigue
}

// ReactionAvailable is R2 §1's fatigue rule stated as a fact about the body
// rather than a decision by the resolver: M4.5 reads it, M4.2 owns it.
func (m *Meters) ReactionAvailable() bool { return m.fatigue < m.dials.NoReactionFatigue }

// Shaken is R2 §3's condition, entered on exhaustion and lowered by thirst.
func (m *Meters) Shaken() bool { return m.fatigue >= m.ShakenThreshold() }

func clampMeter(v float64) float64 { return math.Max(0, math.Min(meterFull, v)) }

// ------------------------------------------------------------- the provider --

// HarnessName identifies the provider (P3 §3.5).
func (m *Meters) HarnessName() string { return "meters" }

// HarnessState exposes everything S1 §5's assertion needs. That includes
// health, which the player entity also reports: the duplication is
// deliberate, because the meters' own assertion ("at food = 0 health
// decrements per hour until death") is written in it, and both readings come
// from the same field so they cannot drift.
func (m *Meters) HarnessState() map[string]interface{} {
	state := map[string]interface{}{
		"food":               m.food,
		"water":              m.water,
		"fatigue":            m.fatigue,
		"activity":           string(m.activity),
		"hungry":             m.Hungry(),
		"starving":           m.Starving(),
		"thirsty":            m.Thirsty(),
		"parched":            m.Parched(),
		"reaction_available": m.ReactionAvailable(),
		"shaken":             m.Shaken(),
		"shaken_threshold":   m.ShakenThreshold(),
		"dying":              m.Dying(),
		"dead":               m.Dead(),
		"neglect_damage":     m.dials.NeglectDamage,
		"has_body":           m.body != nil,
	}

	if m.body != nil {
		state["health"] = m.body.CurrentHealth()
		state["max_health"] = m.body.MaxHealth()
	}

	return state
}

// HarnessSettableFields lists the test-setup writes the meters allow.
func (m *Meters) HarnessSettableFields() []string {
	return []string{"activity", "consume", "fatigue", "food", "water"}
}

// HarnessSet writes one allow-listed field. The three meters are directly
// settable because a script has to be able to stand the body at a threshold
// without stepping a day to get there; "consume" is the game-shaped verb
// that Phase 6's inventory item will drive.
func (m *Meters) HarnessSet(field string, value interface{}) error {
	switch field {
	case "food", "water", "fatigue":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("%s wants a number 0-100, got %T", field, value)
		}

		return m.setMeter(field, f)

	case "activity":
		kind, ok := value.(string)
		if !ok {
			return fmt.Errorf("activity wants a string (idle, labour, watch), got %T", value)
		}

		switch Activity(kind) {
		case ActivityIdle, ActivityLabour, ActivityWatch:
			m.activity = Activity(kind)
			return nil
		default:
			return fmt.Errorf("no activity %q (idle, labour, watch)", kind)
		}

	case "consume":
		return m.consumeField(value)

	default:
		return fmt.Errorf("no settable field %q", field)
	}
}

func (m *Meters) setMeter(field string, f float64) error {
	if f < 0 || f > meterFull {
		return fmt.Errorf("%s wants 0-100, got %v", field, f)
	}

	switch field {
	case "food":
		m.food = f
	case "water":
		m.water = f
	case "fatigue":
		m.fatigue = f
	}

	return nil
}

// consumeField is the object form of Consume: {"kind": "food", "amount": 40}.
func (m *Meters) consumeField(value interface{}) error {
	fields, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`consume wants an object like {"kind": "food", "amount": 40}, got %T`, value)
	}

	kind, ok := fields["kind"].(string)
	if !ok {
		return fmt.Errorf(`consume: "kind" wants a string (food, water, rest), got %T`, fields["kind"])
	}

	amount, ok := toFloat(fields["amount"])
	if !ok {
		return fmt.Errorf(`consume: "amount" wants a number of meter points, got %T`, fields["amount"])
	}

	return m.Consume(kind, amount)
}
