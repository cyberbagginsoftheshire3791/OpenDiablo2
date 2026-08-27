package d2world

import (
	"encoding/json"
	"math"
	"testing"
)

// deepNightNewMoon builds the exact world S1 §4's playtest assertion names:
// the deep night, with a new moon. The clock is stepped there, never set.
func deepNightNewMoon(t *testing.T) (*Clock, *Light) {
	t.Helper()

	c := NewClock(DefaultClockDials())
	l := NewLight(c, DefaultLightDials())

	advanceToMinuteOfDay(t, c, DefaultClockDials().NightStart+30)

	if c.Stage() != StageNight {
		t.Fatalf("wanted the deep night, got %s at %s", c.Stage(), c.TimeOfDay())
	}

	if err := c.HarnessSet("moon", 0.0); err != nil {
		t.Fatal(err)
	}

	return c, l
}

// TestNightLightAssertion is S1 §4's playtest assertion, run as a unit test
// so the contract is pinned even without the game: at deep night with no
// light source and a new moon the visible radius equals the floor; lighting a
// torch restores radius R and decrements burn by 1 per world minute; the
// torch is extinguished at 0 and the radius returns to the floor.
func TestNightLightAssertion(t *testing.T) {
	dials := DefaultLightDials()
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	if got := l.Radius(); math.Abs(got-dials.FloorRadius) > 1e-9 {
		t.Fatalf("deep night, no light, new moon: radius %v, want the floor %v", got, dials.FloorRadius)
	}

	torch := l.Add(SourceTorch, true, 0, 0)

	if got := l.Radius(); math.Abs(got-dials.TorchRadius) > 1e-9 {
		t.Fatalf("torch lit: radius %v, want %v", got, dials.TorchRadius)
	}

	// One world minute of burn per world minute of clock.
	l.Advance(10)

	if want := dials.TorchBurn - 10; math.Abs(torch.Burn-want) > 1e-9 {
		t.Fatalf("after 10 world minutes: burn %v, want %v", torch.Burn, want)
	}

	// Burn it out.
	l.Advance(dials.TorchBurn)

	if torch.Lit {
		t.Fatal("the torch must go out at zero burn")
	}

	if torch.Burn != 0 {
		t.Fatalf("burn floored at %v, want 0", torch.Burn)
	}

	if got := l.Radius(); math.Abs(got-dials.FloorRadius) > 1e-9 {
		t.Fatalf("after the torch dies: radius %v, want the floor %v", got, dials.FloorRadius)
	}
}

func TestAmbientFallsFromDayToTheNightFloor(t *testing.T) {
	c := NewClock(DefaultClockDials())
	l := NewLight(c, DefaultLightDials())

	defer c.Close()
	defer l.Close()

	d := DefaultClockDials()

	advanceToMinuteOfDay(t, c, d.DayStart+120)

	if got := l.Ambient(); got != 1 {
		t.Fatalf("daylight ambient %v, want 1 (the day must look exactly as it does today)", got)
	}

	advanceToMinuteOfDay(t, c, d.DuskStart+45)
	dusk := l.Ambient()

	if dusk >= 1 || dusk <= l.nightFloor() {
		t.Fatalf("dusk ambient %v must sit between the floor %v and 1", dusk, l.nightFloor())
	}

	advanceToMinuteOfDay(t, c, d.NightStart+30)

	if got, want := l.Ambient(), l.nightFloor(); math.Abs(got-want) > 1e-9 {
		t.Fatalf("night ambient %v, want the floor %v", got, want)
	}
}

func TestMoonLiftsTheNightFloor(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	dark := l.Ambient()

	if err := c.HarnessSet("moon", 1.0); err != nil {
		t.Fatal(err)
	}

	if lit := l.Ambient(); lit <= dark {
		t.Fatalf("a full moon (%v) must be brighter than a new one (%v)", lit, dark)
	}
}

func TestLevelFallsOffFromTheSource(t *testing.T) {
	dials := DefaultLightDials()
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.SetPlayer(10.5, 10.5)
	l.Add(SourceTorch, true, 0, 0)

	at := l.Level(10, 10)
	if at < 0.9 {
		t.Fatalf("under the torch: level %v, want ~1", at)
	}

	// Level is quantised; Ambient is not. Compare like with like.
	ambient := l.quantise(l.Ambient())

	mid := l.Level(10+int(dials.TorchRadius)-1, 10)
	if mid >= at || mid <= ambient {
		t.Fatalf("mid-radius level %v must sit between the ambient %v and %v", mid, ambient, at)
	}

	far := l.Level(10+int(dials.TorchRadius)+3, 10)
	if math.Abs(far-ambient) > 1e-9 {
		t.Fatalf("beyond the radius: level %v, want the quantised ambient %v", far, ambient)
	}
}

func TestLevelIsQuantised(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.SetPlayer(0, 0)
	l.Add(SourceTorch, true, 0, 0)

	steps := float64(DefaultLightDials().Steps)

	for x := 0; x < 8; x++ {
		v := l.Level(x, 0)
		if math.Abs(v*steps-math.Round(v*steps)) > 1e-9 {
			t.Fatalf("level %v at x=%d is not on a %v-step grid", v, x, steps)
		}
	}
}

func TestPlacedLightOnlyHelpsWhenThePlayerIsInIt(t *testing.T) {
	dials := DefaultLightDials()
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.SetPlayer(0, 0)
	l.Add(SourceHearth, false, 40, 40)

	if got := l.Radius(); math.Abs(got-dials.FloorRadius) > 1e-9 {
		t.Fatalf("a hearth across the map gave the player radius %v, want the floor", got)
	}

	l.SetPlayer(40, 41)

	if got := l.Radius(); math.Abs(got-dials.HearthRadius) > 1e-9 {
		t.Fatalf("standing at the hearth: radius %v, want %v", got, dials.HearthRadius)
	}
}

func TestHearthDoesNotBurnDown(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	hearth := l.Add(SourceHearth, false, 0, 0)

	l.Advance(10000)

	if !hearth.Lit {
		t.Fatal("the hearth is fuel-fed, not timed: it must not burn out on the clock in M4.1")
	}
}

func TestLightHarnessSetGivesAndTakesTheTorch(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	if err := l.HarnessSet("carried_burn", 5); err == nil {
		t.Error("setting burn with no carried light must fail")
	}

	if err := l.HarnessSet("carried_source", "torch"); err != nil {
		t.Fatal(err)
	}

	state := l.HarnessState()
	if state["carried_source"] != "torch" || state["carried_lit"] != true {
		t.Fatalf("after giving a torch: %v", state)
	}

	if err := l.HarnessSet("carried_burn", 1.0); err != nil {
		t.Fatal(err)
	}

	l.Advance(2)

	if l.Carried().Lit {
		t.Fatal("a one-minute torch must be out after two minutes")
	}

	if err := l.HarnessSet("carried_source", ""); err != nil {
		t.Fatal(err)
	}

	if l.Carried() != nil {
		t.Fatal("setting carried_source to empty must take the light away")
	}

	if err := l.HarnessSet("carried_source", "bonfire"); err == nil {
		t.Error("an unknown source must fail")
	}
}

func TestLightHarnessStateIsEncodableAndOrdered(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.Add(SourceHearth, false, 3, 3)
	l.Add(SourceTorch, true, 0, 0)

	state := l.HarnessState()

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("light state is not JSON-encodable: %v", err)
	}

	if len(raw) == 0 {
		t.Fatal("empty state")
	}

	list, ok := state["source_list"].([]map[string]interface{})
	if !ok || len(list) != 2 {
		t.Fatalf("source_list: %v", state["source_list"])
	}

	if list[0]["id"].(int) >= list[1]["id"].(int) {
		t.Fatal("source_list must be ordered by id so the digest is stable")
	}
}

// TestPlaceSourceLightsWhereItStandsAndNotOnThePlayer is the model half of
// the assertion M4.1 shipped without: at deep night with no carried light, a
// hearth placed away from the player lights its own ground and leaves his at
// the night floor.
//
// Nine tiles, not three. The hearth's radius is 8, so a hearth three tiles
// away engulfs the player, and a placed source that engulfs the player is
// indistinguishable from a carried one — which is exactly the failure this
// test exists to catch. Nine is the first whole tile past the radius.
func TestPlaceSourceLightsWhereItStandsAndNotOnThePlayer(t *testing.T) {
	dials := DefaultLightDials()
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.SetPlayer(31.5, 14.5)

	if err := l.HarnessSet("place_source", map[string]interface{}{
		"kind": "hearth", "x": 22.5, "y": 14.5,
	}); err != nil {
		t.Fatal(err)
	}

	floor := l.quantise(l.Ambient())

	if got := l.Level(22, 14); got < 0.9 {
		t.Fatalf("on the hearth's own tile: level %v, want ~1 — a placed light must light where it stands", got)
	}

	if got := l.Level(31, 14); math.Abs(got-floor) > 1e-9 {
		t.Fatalf("on the player's tile: level %v, want the night floor %v — the light is following the player",
			got, floor)
	}

	if got := l.Radius(); math.Abs(got-dials.FloorRadius) > 1e-9 {
		t.Fatalf("a hearth nine tiles away gave the player radius %v, want the floor %v", got, dials.FloorRadius)
	}

	state := l.HarnessState()

	if got, ok := state["player_level"].(float64); !ok || math.Abs(got-floor) > 1e-9 {
		t.Fatalf("player_level %v, want the night floor %v", state["player_level"], floor)
	}

	list, ok := state["source_list"].([]map[string]interface{})
	if !ok || len(list) != 1 {
		t.Fatalf("source_list: %v", state["source_list"])
	}

	src := list[0]
	if src["carried"] != false || src["x"] != 22.5 || src["y"] != 14.5 {
		t.Fatalf("the placed hearth reports itself as %v", src)
	}

	if lvl, _ := src["level_here"].(float64); lvl < 0.9 {
		t.Fatalf("level_here at the hearth is %v, want ~1", lvl)
	}
}

// TestCarriedSourceReportsWhereThePlayerIsNow pins the other half of the
// reporting fix: a carried light's stored X/Y is the position it was lit at
// and never moves, so the provider must report the player's instead.
func TestCarriedSourceReportsWhereThePlayerIsNow(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.SetPlayer(10.5, 10.5)

	if err := l.HarnessSet("carried_source", "torch"); err != nil {
		t.Fatal(err)
	}

	l.SetPlayer(20.5, 20.5)

	list, _ := l.HarnessState()["source_list"].([]map[string]interface{})
	if len(list) != 1 {
		t.Fatalf("source_list: %v", list)
	}

	if list[0]["x"] != 20.5 || list[0]["y"] != 20.5 {
		t.Fatalf("a carried torch must report where the player is now, not where it was lit: %v", list[0])
	}
}

// TestPlaceSourceRejectsBadShapes: the verb takes an object, so its errors
// have to name the shape, and a rejected placement must leave nothing behind.
func TestPlaceSourceRejectsBadShapes(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	cases := []struct {
		name  string
		value interface{}
	}{
		{"not an object", "hearth"},
		{"no kind", map[string]interface{}{"x": 1.0, "y": 2.0}},
		{"unknown kind", map[string]interface{}{"kind": "bonfire", "x": 1.0, "y": 2.0}},
		{"no x", map[string]interface{}{"kind": "hearth", "y": 2.0}},
		{"y is a string", map[string]interface{}{"kind": "hearth", "x": 1.0, "y": "over there"}},
	}

	for _, tc := range cases {
		if err := l.HarnessSet("place_source", tc.value); err == nil {
			t.Errorf("%s: want an error, got none", tc.name)
		}
	}

	if n := len(l.sources); n != 0 {
		t.Fatalf("a rejected placement left %d source(s) behind", n)
	}
}

// TestRemoveSourcePutsTheFireOut is the other half of place_source: a hearth
// is fuel-fed and never burns down (S1 §4), so without a verb that puts it
// out there is no way to show that the dark comes back.
func TestRemoveSourcePutsTheFireOut(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.SetPlayer(31.5, 14.5)

	if err := l.HarnessSet("place_source", map[string]interface{}{
		"kind": "hearth", "x": 22.5, "y": 14.5,
	}); err != nil {
		t.Fatal(err)
	}

	floor := l.quantise(l.Ambient())

	if got := l.Level(22, 14); got < 0.9 {
		t.Fatalf("the hearth should be lit before we put it out: level %v", got)
	}

	list, _ := l.HarnessState()["source_list"].([]map[string]interface{})
	if len(list) != 1 {
		t.Fatalf("source_list: %v", list)
	}

	id, _ := list[0]["id"].(int)

	if err := l.HarnessSet("remove_source", float64(id)); err != nil {
		t.Fatal(err)
	}

	if got := l.Level(22, 14); math.Abs(got-floor) > 1e-9 {
		t.Fatalf("after the fire went out the hearth's tile is at %v, want the night floor %v", got, floor)
	}

	state := l.HarnessState()
	if state["sources"] != 0 || state["lit_sources"] != 0 {
		t.Fatalf("removing the only source should leave none: %v", state)
	}

	if err := l.HarnessSet("remove_source", float64(id)); err == nil {
		t.Error("removing the same source twice must fail")
	}

	if err := l.HarnessSet("remove_source", "the hearth"); err == nil {
		t.Error("remove_source must reject a non-numeric id")
	}

	if err := l.HarnessSet("remove_source", 1.5); err == nil {
		t.Error("remove_source must reject a fractional id")
	}
}
