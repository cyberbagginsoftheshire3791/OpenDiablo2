package d2gamescreen

import (
	"fmt"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"

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

// fencedGame is a game with -fog wanted and a renderer, on a client holding a
// map engine and the given players (nil entries: only the count is read).
func fencedGame(t *testing.T, classic bool, players int) *Game {
	t.Helper()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	asset.SetClassic(classic)

	client := &d2client.GameClient{MapEngine: &d2mapengine.MapEngine{}, Players: map[string]*d2mapentity.Player{}}
	for i := 0; i < players; i++ {
		client.Players[fmt.Sprintf("p%d", i)] = nil
	}

	return &Game{
		asset: asset, gameClient: client,
		fog: newGameFog(clearSight{}, true), mapRenderer: d2maprenderer.NewViewOnlyMapRenderer(0, 0),
	}
}

// TestTheClassicFenceKeepsFogOff (the review's B2): under -classic a game
// wanting fog draws none -- the provider says why, the renderer holds no
// sampler, and the harness cannot turn it on.
//
// Negative control (1 Oct 2026): drop the classic case from fogOffReason (the
// reviewer's m01) and this fails, "under -classic the off reason is "no_map"
// and the renderer holds fog" (nc27-classic-not-fenced.txt).
func TestTheClassicFenceKeepsFogOff(t *testing.T) {
	v := fencedGame(t, true, 1)
	v.fogAdvance()

	if r := v.fogOffReason(); r != fogOffClassic || v.mapRenderer.HasFog() {
		t.Fatalf("under -classic the off reason is %q and the renderer holds fog %v", r, v.mapRenderer.HasFog())
	}

	if err := (fogProvider{v}).HarnessSet("enabled", true); err == nil || v.mapRenderer.HasFog() {
		t.Fatalf("enabled=true under -classic: err %v, renderer holds fog %v", err, v.mapRenderer.HasFog())
	}
}

// TestTheNetworkFenceKeepsFogOff (the review's B2): with two players fog is
// off, and a fog the renderer held is taken from it.
//
// Negative control (1 Oct 2026): drop the network case (the reviewer's m02)
// and this fails, "with two players the off reason is "no_map" and the
// renderer holds fog true" (nc28-network-not-fenced.txt).
func TestTheNetworkFenceKeepsFogOff(t *testing.T) {
	v := fencedGame(t, false, 0)
	v.fogAdvance() // a single-player game before its player joined: no_map, black

	if !v.mapRenderer.HasFog() {
		t.Fatal("the control: a -fog game before its player joined does not hold the fog")
	}

	v.gameClient.Players["a"], v.gameClient.Players["b"] = nil, nil
	v.fogAdvance()

	if r := v.fogOffReason(); r != fogOffNetwork || v.mapRenderer.HasFog() {
		t.Fatalf("with two players the off reason is %q and the renderer holds fog %v", r, v.mapRenderer.HasFog())
	}
}

// TestAFoggedGameIsBlackFromItsFirstFrame (the review's B5): CreateGame runs
// fogAdvance before there is a player, and with -fog that hands the renderer
// the fog at once -- a fog that has seen nothing, so every tile is
// unexplored and nothing is drawn. "no_map" never takes it away; turning fog
// off does (the reviewer's m03).
//
// Negative controls (1 Oct 2026): detach on "no_map" as on any other reason
// (the pre-review rule) and this fails, "a -fog game before its player is
// there leaves the renderer unfogged" (nc29-no-map-detaches.txt); keep the
// sampler when fog is turned off (m03) and it fails, "enabled=false left the
// renderer fogged" (nc30-off-keeps-sampler.txt).
func TestAFoggedGameIsBlackFromItsFirstFrame(t *testing.T) {
	v := fencedGame(t, false, 0)
	v.fogAdvance()

	if r := v.fogOffReason(); r != fogOffNoMap {
		t.Fatalf("a game with no player yet is off for %q, want no_map", r)
	}

	if !v.mapRenderer.HasFog() {
		t.Fatal("a -fog game before its player is there leaves the renderer unfogged")
	}

	if v.mapRenderer.Shows(3.5, 3.5) {
		t.Fatal("a fog that has seen nothing shows (3,3)")
	}

	v.fogAdvance()

	if !v.mapRenderer.HasFog() {
		t.Fatal("a second frame with no player took the fog away")
	}

	if err := (fogProvider{v}).HarnessSet("enabled", false); err != nil || v.mapRenderer.HasFog() {
		t.Fatalf("enabled=false left the renderer fogged (err %v)", err)
	}

	off := fencedGame(t, false, 0)
	off.fog.wanted = false
	off.fogAdvance()

	if off.mapRenderer.HasFog() {
		t.Fatal("without -fog the renderer holds fog")
	}
}
