package d2gamescreen

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
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

// TestFogDigestLeavesOutFramesAndScreen: the digest's process part is what two
// launches of one script share, so fog's frame counter (skipped) and the
// probe's screen point stay out of it; the script still reads both.
//
// Negative control (1 Oct 2026): return HarnessState whole as the process part
// and this fails, "the digest carries skipped, a count of frames"
// (strigoi-harness-runs\wt-fog\nc\nc15-digest-whole-state.txt).
func TestFogDigestLeavesOutFramesAndScreen(t *testing.T) {
	v := &Game{fog: newGameFog(clearSight{}, true), mapRenderer: d2maprenderer.NewViewOnlyMapRenderer(0, 0)}
	v.fog.fog.Update(10, 10, []d2world.Eye{{ID: fogEyeID, X: 5.5, Y: 5.5}})
	v.fog.fog.Update(10, 10, []d2world.Eye{{ID: fogEyeID, X: 5.6, Y: 5.5}})
	v.fog.probe = &[2]int{1, 1}

	p := fogProvider{v}

	st := p.HarnessState()
	if st["skipped"] != 1 {
		t.Fatalf("the state reports skipped %v, want 1", st["skipped"])
	}

	if pr, _ := st["probe"].(map[string]interface{}); pr["screen"] == nil {
		t.Fatalf("the state's probe has no screen point: %v", pr)
	}

	world, process := p.HarnessDigest()
	if len(world) != 0 {
		t.Fatalf("fog put %v in the world part; F1 does not save it", world)
	}

	if _, ok := process["skipped"]; ok {
		t.Fatal("the digest carries skipped, a count of frames")
	}

	probe, _ := process["probe"].(map[string]interface{})
	if _, ok := probe["screen"]; ok || probe["state"] == nil {
		t.Fatalf("the digest's probe is %v; want its state and no screen point", probe)
	}

	if process["explored"] != st["explored"] || process["recomputes"] != 1 {
		t.Fatalf("the digest lost the world-tile counts: %v", process)
	}
}
