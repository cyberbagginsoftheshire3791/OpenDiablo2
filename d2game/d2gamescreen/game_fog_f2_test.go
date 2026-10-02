package d2gamescreen

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// FOG OF WAR F2 at the game screen (game_fog.go): whose eyes, which enemies
// are contacts, which light the renderer draws by, and the provider's night
// dials. Negative controls: strigoi-harness-runs\wt-fog2\nc\ (nc-game-*.txt).

func at(m map[string][2]float64) func(id string) (x, y float64, ok bool) {
	return func(id string) (float64, float64, bool) {
		p, ok := m[id]

		return p[0], p[1], ok
	}
}

// TestEachSquadIsAnEye (Q5's default): every model of his squads but his own
// (he is s:1, already an eye) is an eye at its entity's position; a model
// whose entity is off the map is none; nothing else -- no villager -- is.
//
// Negative control: keep his own model as a second eye and this fails, "the
// eyes are [s:1/p1 s:2/e7]" (nc-game-player-twice.txt).
func TestEachSquadIsAnEye(t *testing.T) {
	models := []d2world.SquadModel{
		{Squad: "s:1", Entity: "p1"}, {Squad: "s:2", Entity: "e7"}, {Squad: "s:3", Entity: "gone"},
	}
	pos := at(map[string][2]float64{"p1": {5, 5}, "e7": {30.2, 12.9}, "villager": {6, 6}})

	eyes := squadEyes(nil, "p1", models, pos)

	ids := make([]string, 0, len(eyes))
	for _, e := range eyes {
		ids = append(ids, e.ID)
	}

	if len(eyes) != 1 || eyes[0].ID != "s:2/e7" || eyes[0].X != 30.2 || eyes[0].Y != 12.9 || eyes[0].Contact {
		t.Fatalf("the eyes are %v (%+v); want the one other model, s:2/e7 at (30.2,12.9)", ids, eyes)
	}
}

// TestHisFightsEnemiesAreContacts (Q4): every enemy of his fight still in it
// is a contact at its body's position on the map (else the fight's); the
// gone -- dead, routed, or broke off at first light (the review's C3,
// TacticalEnemy.Gone) -- are not.
//
// Negative controls: keep the dead and the routed and this fails, "the
// contacts are [fight/wolf fight/dog fight/rat fight/ghost fight/risen]"
// (nc-game-dead-contacts.txt); read dead-or-routed only (the pre-review
// filter) and it fails on fight/risen (nc-game-broke-contact.txt).
func TestHisFightsEnemiesAreContacts(t *testing.T) {
	enemies := []d2world.TacticalEnemy{
		{ID: "wolf", X: 1, Y: 1}, {ID: "dog", X: 2, Y: 2, Dead: true},
		{ID: "rat", X: 3, Y: 3, Routed: true}, {ID: "ghost", X: 9.5, Y: 8.5},
		{ID: "risen", X: 4, Y: 4, Broke: true},
	}
	got := fightContacts(nil, enemies, at(map[string][2]float64{"wolf": {14.3, 7.6}}))

	ids := make([]string, 0, len(got))
	for _, e := range got {
		ids = append(ids, e.ID)
	}

	if len(got) != 2 || got[0].ID != "fight/wolf" || got[1].ID != "fight/ghost" {
		t.Fatalf("the contacts are %v; want the wolf and the ghost, not the dead dog, the routed rat or the risen man who broke off", ids)
	}

	if !got[0].Contact || got[0].X != 14.3 || got[0].Y != 7.6 || got[1].X != 9.5 {
		t.Fatalf("the contacts stand at %+v; want the wolf's body (14.3,7.6) and the ghost where the fight has it", got)
	}
}

// TestFogDrawsByTheLightHeSees (BUG-110): while fog is attached the renderer
// draws by the light view (his torch where he stands), and when fog is taken
// away it draws by the light model itself again -- master's draw.
//
// Negative control: leave the view as the sampler on detach and this fails,
// "with fog off the renderer draws by view" (nc-game-detach-keeps-view.txt).
func TestFogDrawsByTheLightHeSees(t *testing.T) {
	clock := d2world.NewClock(d2world.DefaultClockDials())
	defer clock.Close()

	light := d2world.NewLight(clock, d2world.DefaultLightDials())
	defer light.Close()

	v := fencedGame(t, false, 0)
	v.light = light
	v.mapRenderer.SetLightSampler(light)
	v.fog.seeByLight(light)

	if v.fogDrawsBy() != "light" {
		t.Fatalf("the control: before fog attaches the renderer draws by %s", v.fogDrawsBy())
	}

	v.fogAdvance()

	if !v.mapRenderer.HasFog() || v.fogDrawsBy() != "view" {
		t.Fatalf("with fog attached the renderer draws by %s (fog %v); want the view", v.fogDrawsBy(), v.mapRenderer.HasFog())
	}

	if err := (fogProvider{v}).HarnessSet("enabled", false); err != nil {
		t.Fatal(err)
	}

	if v.mapRenderer.HasFog() || v.mapRenderer.LightSampler() != d2maprenderer.LightSampler(light) {
		t.Fatalf("with fog off the renderer draws by %s (fog %v); want the light model, master's draw",
			v.fogDrawsBy(), v.mapRenderer.HasFog())
	}
}

// TestFogProviderTakesTheNightDials: dark_radius and moon_dark_radius are
// settable in 0..64 and reported with tonight's dark and the unlit reach.
func TestFogProviderTakesTheNightDials(t *testing.T) {
	v := &Game{fog: newGameFog(clearSight{}, true)}
	p := fogProvider{v}

	for _, ok := range []struct {
		field string
		value float64
	}{{"dark_radius", 2}, {"moon_dark_radius", 5}} {
		if err := p.HarnessSet(ok.field, ok.value); err != nil {
			t.Fatalf("%s %v was refused: %v", ok.field, ok.value, err)
		}
	}

	for _, bad := range []struct {
		field string
		value interface{}
	}{{"dark_radius", -1.0}, {"moon_dark_radius", 65.0}, {"dark_radius", "1.5"}} {
		if err := p.HarnessSet(bad.field, bad.value); err == nil {
			t.Errorf("%s %v was taken", bad.field, bad.value)
		}
	}

	st := p.HarnessState()
	if st["dark_radius"] != 2.0 || st["moon_dark_radius"] != 5.0 {
		t.Fatalf("the provider reports dark %v, full-moon dark %v; want 2 and 5", st["dark_radius"], st["moon_dark_radius"])
	}

	for _, k := range []string{"tonight_dark", "unlit_reach", "contacts", "lit_seen", "by_light", "draws_by"} {
		if _, ok := st[k]; !ok {
			t.Errorf("the provider does not report %s", k)
		}
	}
}
