package d2gamescreen

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

type clearSight struct{}

func (clearSight) TileSightClear(float64, float64, int, int) (bool, int) { return true, 1 }

// TestFogProviderWritesAndRefuses: the fog provider takes its dials in range,
// its verbs and a probe, and refuses a dial out of range, a malformed verb and
// an unknown field without changing anything. A game with no map is off and
// says why.
func TestFogProviderWritesAndRefuses(t *testing.T) {
	v := &Game{fog: newGameFog(clearSight{}, true)}
	p := fogProvider{v}

	if st := p.HarnessState(); st["enabled"] != false || st["off_reason"] != fogOffNoMap || st["wanted"] != true {
		t.Fatalf("a fogged game with no map reports enabled %v, off_reason %v, wanted %v",
			st["enabled"], st["off_reason"], st["wanted"])
	}

	for _, ok := range []struct {
		field string
		value interface{}
	}{
		{"day_sight", 10.0}, {"memory_level", 0.3}, {"memory_saturation", 0.0},
		{"explore", map[string]interface{}{"x": 3.0, "y": 3.0, "r": 2.0}},
		{"forget", true}, {"reveal_all", true},
		{"probe", map[string]interface{}{"x": 4.0, "y": 5.5}},
		{"enabled", false},
	} {
		if err := p.HarnessSet(ok.field, ok.value); err != nil {
			t.Errorf("%s %v was refused: %v", ok.field, ok.value, err)
		}
	}

	if d := v.fog.fog.Dials(); d.DaySight != 10 || d.MemoryLevel != 0.3 || d.MemorySaturation != 0 {
		t.Fatalf("the dials written are %+v", d)
	}

	if v.fog.probe == nil || *v.fog.probe != [2]int{4, 5} {
		t.Fatalf("the probe is %v, want the tile (4,5)", v.fog.probe)
	}

	if v.fog.wanted {
		t.Fatal("enabled false left the fog wanted")
	}

	for _, bad := range []struct {
		field string
		value interface{}
	}{
		{"day_sight", 0.0}, {"day_sight", 65.0}, {"day_sight", "12"},
		{"memory_level", 1.5}, {"memory_saturation", -0.1},
		{"explore", map[string]interface{}{"x": 1.0, "y": 1.0}},
		{"probe", 3.0}, {"enabled", "yes"}, {"rows", 1.0},
	} {
		if err := p.HarnessSet(bad.field, bad.value); err == nil {
			t.Errorf("%s %v was taken", bad.field, bad.value)
		}
	}

	if d := v.fog.fog.Dials(); d.DaySight != 10 {
		t.Fatalf("a refused write changed day_sight to %v", d.DaySight)
	}
}

// TestFogDefaultsAreJoshs: the shipped dials are Q1's 12 tiles and the plan's
// look (0.45, 0.25), and fog is off unless -fog says otherwise.
func TestFogDefaultsAreJoshs(t *testing.T) {
	if d := d2world.DefaultFogDials(); d.DaySight != 12 || d.MemoryLevel != 0.45 || d.MemorySaturation != 0.25 {
		t.Fatalf("fog ships with %+v", d)
	}

	if processGameFog {
		t.Fatal("fog is on by default; F1 is opt-in behind -fog")
	}
}
