package d2world

import (
	"encoding/json"
	"math"
	"testing"
)

// testBody stands in for the player's hero stats: the meters only ever need
// to read health, read its ceiling, and set it.
type testBody struct{ health, max int }

func (b *testBody) CurrentHealth() int { return b.health }
func (b *testBody) MaxHealth() int     { return b.max }
func (b *testBody) SetHealth(h int)    { b.health = h }

// nightMeters builds a full, rested body at deep night — night so the
// daylight thirst multiplier is out of the arithmetic unless a test wants it.
func nightMeters(t *testing.T) (*Clock, *Meters) {
	t.Helper()

	c := NewClock(DefaultClockDials())
	advanceToMinuteOfDay(t, c, DefaultClockDials().NightStart+30)

	if c.Stage() != StageNight {
		t.Fatalf("wanted the deep night, got %s at %s", c.Stage(), c.TimeOfDay())
	}

	return c, NewMeters(c, DefaultMeterDials())
}

const tenHours = 10 * minutesPerHour

// TestMetersDrainOnTheWorldClock is the first clause of S1 §5's assertion,
// pinned as a unit test: with the clock advanced N world hours and nothing
// consumed, each meter reads its expected value.
func TestMetersDrainOnTheWorldClock(t *testing.T) {
	d := DefaultMeterDials()
	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	m.Advance(tenHours)

	for _, tc := range []struct {
		name string
		got  float64
		want float64
	}{
		{"food", m.Food(), meterFull - d.FoodDrain*10},
		{"water", m.Water(), meterFull - d.WaterDrain*10},
		{"fatigue", m.Fatigue(), d.FatigueDrain * 10},
	} {
		if math.Abs(tc.got-tc.want) > 1e-9 {
			t.Errorf("after 10 world hours: %s %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// TestFatigueTakesTheReactionThenShakes is R2 §1's coupling — the reason the
// meters exist in a horror game rather than as chores.
func TestFatigueTakesTheReactionThenShakes(t *testing.T) {
	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	if !m.ReactionAvailable() || m.Shaken() {
		t.Fatal("a rested body has its Reaction and is not Shaken")
	}

	if err := m.HarnessSet("fatigue", 74.0); err != nil {
		t.Fatal(err)
	}

	if !m.ReactionAvailable() {
		t.Error("at 74 fatigue the Reaction is still there")
	}

	if err := m.HarnessSet("fatigue", 75.0); err != nil {
		t.Fatal(err)
	}

	if m.ReactionAvailable() {
		t.Error("at 75 fatigue the Reaction is gone (S1 §5)")
	}

	if m.Shaken() {
		t.Error("75 is not yet Shaken")
	}

	if err := m.HarnessSet("fatigue", 90.0); err != nil {
		t.Fatal(err)
	}

	if !m.Shaken() {
		t.Error("at 90 fatigue fights start Shaken (S1 §5)")
	}
}

// TestThirstLowersTheShakenThreshold is S1 §5's Water row: "Thirsty: Shaken
// threshold lowers."
func TestThirstLowersTheShakenThreshold(t *testing.T) {
	d := DefaultMeterDials()
	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	if err := m.HarnessSet("fatigue", 85.0); err != nil {
		t.Fatal(err)
	}

	if m.Shaken() {
		t.Fatal("85 fatigue with water in you is not Shaken")
	}

	if got := m.ShakenThreshold(); got != d.ShakenFatigue {
		t.Fatalf("watered threshold %v, want %v", got, d.ShakenFatigue)
	}

	if err := m.HarnessSet("water", d.WarnLevel); err != nil {
		t.Fatal(err)
	}

	if got := m.ShakenThreshold(); got != d.ThirstyShakenFatigue {
		t.Fatalf("thirsty threshold %v, want %v", got, d.ThirstyShakenFatigue)
	}

	if !m.Shaken() {
		t.Error("the same 85 fatigue, thirsty, starts the fight Shaken")
	}
}

// TestHungerTiresYouFaster is S1 §5's Food row: "Hungry: fatigue drains
// faster." It is the coupling that makes food matter at night.
func TestHungerTiresYouFaster(t *testing.T) {
	d := DefaultMeterDials()

	fed, hungry := 0.0, 0.0

	for _, tc := range []struct {
		food float64
		out  *float64
	}{{meterFull, &fed}, {d.WarnLevel, &hungry}} {
		c, m := nightMeters(t)

		if err := m.HarnessSet("food", tc.food); err != nil {
			t.Fatal(err)
		}

		m.Advance(tenHours)
		*tc.out = m.Fatigue()

		c.Close()
		m.Close()
	}

	if want := fed * d.HungryFatigueFactor; math.Abs(hungry-want) > 1e-9 {
		t.Fatalf("hungry fatigue %v, want %v (fed %v x%v)", hungry, want, fed, d.HungryFatigueFactor)
	}
}

// TestDaylightCostsMoreWater is S1 §5's "faster in daylight heat (it is late
// June)" — the only meter the clock's stage touches.
func TestDaylightCostsMoreWater(t *testing.T) {
	d := DefaultMeterDials()

	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	m.Advance(tenHours)
	night := meterFull - m.Water()

	day := NewClock(DefaultClockDials())
	advanceToMinuteOfDay(t, day, DefaultClockDials().DayStart+120)

	if day.Stage() != StageDay {
		t.Fatalf("wanted daylight, got %s", day.Stage())
	}

	sun := NewMeters(day, DefaultMeterDials())

	defer day.Close()
	defer sun.Close()

	sun.Advance(tenHours)

	if want := night * d.DaylightWaterFactor; math.Abs((meterFull-sun.Water())-want) > 1e-9 {
		t.Fatalf("daylight thirst %v, want %v (night %v x%v)",
			meterFull-sun.Water(), want, night, d.DaylightWaterFactor)
	}
}

// TestLabourCostsMore exercises the activity multipliers. The verbs that
// would set the activity arrive with M4.3 and M4.5; until then the harness
// is the only caller, and a dial nothing can exercise is the dead wiring
// M4.1 was reopened over.
func TestLabourCostsMore(t *testing.T) {
	d := DefaultMeterDials()
	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	m.Advance(tenHours)
	idleFood := meterFull - m.Food()

	if err := m.HarnessSet("food", meterFull); err != nil {
		t.Fatal(err)
	}

	if err := m.HarnessSet("activity", "labour"); err != nil {
		t.Fatal(err)
	}

	m.Advance(tenHours)

	if want := idleFood * d.LabourFoodFactor; math.Abs((meterFull-m.Food())-want) > 1e-9 {
		t.Fatalf("labouring food cost %v, want %v", meterFull-m.Food(), want)
	}

	if err := m.HarnessSet("activity", "digging"); err == nil {
		t.Error("an unknown activity must fail")
	}
}

// TestNeglectKills is the third clause of S1 §5's assertion, as far as M4.2
// owns it (build note §3): at food = 0 health decrements per world hour
// until death. The death SCREEN is M4.6's, on S1 §6.5 and R2 §3.
func TestNeglectKills(t *testing.T) {
	d := DefaultMeterDials()
	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	body := &testBody{health: 40, max: 40}
	m.SetBody(body)

	if m.Dying() || m.Dead() {
		t.Fatal("a fed body is neither dying nor dead")
	}

	if err := m.HarnessSet("food", 0); err != nil {
		t.Fatal(err)
	}

	if !m.Dying() {
		t.Fatal("an empty stomach is dying")
	}

	m.Advance(tenHours)

	if want := 40 - int(d.NeglectDamage*10); body.health != want {
		t.Fatalf("after 10 starving hours: health %d, want %d", body.health, want)
	}

	// Empty the water too: two empty meters kill twice as fast.
	if err := m.HarnessSet("water", 0); err != nil {
		t.Fatal(err)
	}

	before := body.health
	m.Advance(minutesPerHour)

	if want := before - int(d.NeglectDamage*2); body.health != want {
		t.Fatalf("one hour starving AND parched: health %d, want %d", body.health, want)
	}

	// Run it to the end.
	for i := 0; i < 100 && !m.Dead(); i++ {
		m.Advance(tenHours)
	}

	if !m.Dead() {
		t.Fatal("neglect must kill")
	}

	if body.health != 0 {
		t.Fatalf("dead at health %d, want 0", body.health)
	}

	// A dead body stops spending.
	food := m.Food()
	m.Advance(tenHours)

	if m.Food() != food {
		t.Error("a dead body must not keep draining")
	}
}

// TestNoBodyMeansNothingDies: before a game screen exists the meters still
// drain and still report Dying — they just have nothing to damage.
func TestNoBodyMeansNothingDies(t *testing.T) {
	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	if err := m.HarnessSet("food", 0); err != nil {
		t.Fatal(err)
	}

	m.Advance(tenHours)

	if !m.Dying() {
		t.Error("an empty stomach is dying whether or not there is a body")
	}

	if m.Dead() {
		t.Error("with no body attached nothing can die")
	}
}

// TestConsumeMovesThemBothWays is the third provider rule, made mechanical:
// a provider that reports a value needs a verb that can move it in BOTH
// directions, or a script can only ever watch the number fall.
func TestConsumeMovesThemBothWays(t *testing.T) {
	d := DefaultMeterDials()
	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	m.Advance(tenHours)

	food, water, fatigue := m.Food(), m.Water(), m.Fatigue()

	if err := m.HarnessSet("consume", map[string]interface{}{"kind": "food", "amount": 10.0}); err != nil {
		t.Fatal(err)
	}

	if want := food + 10; math.Abs(m.Food()-want) > 1e-9 {
		t.Errorf("after eating: food %v, want %v", m.Food(), want)
	}

	if err := m.Consume("water", 5); err != nil {
		t.Fatal(err)
	}

	if want := water + 5; math.Abs(m.Water()-want) > 1e-9 {
		t.Errorf("after drinking: water %v, want %v", m.Water(), want)
	}

	// An hour of sleep is RestRate of rest.
	if err := m.Consume("rest", d.RestRate); err != nil {
		t.Fatal(err)
	}

	if want := fatigue - d.RestRate; math.Abs(m.Fatigue()-want) > 1e-9 {
		t.Errorf("after an hour's sleep: fatigue %v, want %v", m.Fatigue(), want)
	}

	// Nothing overfills.
	if err := m.Consume("food", 1000); err != nil {
		t.Fatal(err)
	}

	if m.Food() != meterFull {
		t.Errorf("food %v, want the meter capped at %v", m.Food(), meterFull)
	}
}

func TestMetersHarnessSetRejectsBadShapes(t *testing.T) {
	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	cases := []struct {
		field string
		value interface{}
	}{
		{"food", "full"},
		{"food", -1.0},
		{"water", 101.0},
		{"activity", 3},
		{"consume", "bread"},
		{"consume", map[string]interface{}{"kind": "bread", "amount": 1.0}},
		{"consume", map[string]interface{}{"kind": "food"}},
		{"consume", map[string]interface{}{"kind": "food", "amount": -5.0}},
		{"morale", 50.0},
	}

	for _, tc := range cases {
		if err := m.HarnessSet(tc.field, tc.value); err == nil {
			t.Errorf("%s=%v: want an error, got none", tc.field, tc.value)
		}
	}

	if m.Food() != meterFull || m.Water() != meterFull {
		t.Fatalf("a rejected write changed a meter: food %v water %v", m.Food(), m.Water())
	}
}

func TestMetersHarnessStateIsEncodable(t *testing.T) {
	c, m := nightMeters(t)

	defer c.Close()
	defer m.Close()

	state := m.HarnessState()

	if _, ok := state["health"]; ok {
		t.Error("with no body attached the state must not claim a health")
	}

	m.SetBody(&testBody{health: 30, max: 55})

	state = m.HarnessState()
	if state["health"] != 30 || state["max_health"] != 55 {
		t.Fatalf("state health: %v / %v", state["health"], state["max_health"])
	}

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("meters state is not JSON-encodable: %v", err)
	}

	if len(raw) == 0 {
		t.Fatal("empty state")
	}

	for _, key := range []string{"food", "water", "fatigue", "reaction_available", "shaken", "dying", "dead"} {
		if _, ok := state[key]; !ok {
			t.Errorf("the assertion needs %q and the provider does not report it", key)
		}
	}
}
