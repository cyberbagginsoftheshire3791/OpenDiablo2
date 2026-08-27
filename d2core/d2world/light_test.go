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
