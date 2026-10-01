package d2world

import (
	"encoding/json"
	"testing"
)

// FOG OF WAR F2: THE NIGHT CLOSES IT (fog.go, light_view.go). Josh's rulings
// of 1 Oct 2026: Q2, dark sight 1.5 tiles rising to ~4 under a full moon; Q3,
// lit ground with a clear line is seen at any distance; Q4, the enemies of his
// own fight are shown while it lasts; Q5 (default), every one of his squads'
// models is an eye and villagers are not.
//
// Negative controls (1 Oct 2026): each test names the break it was run
// against; the logs are in strigoi-harness-runs\wt-fog2\nc\.

// fakeSky is a FogLight whose sky, moon and lit tiles a test sets.
type fakeSky struct {
	sky, moon float64
	lit       map[[2]int]bool
	discs     []LitDisc
}

func (s *fakeSky) SkyFraction() float64 { return s.sky }
func (s *fakeSky) Moon() float64        { return s.moon }
func (s *fakeSky) Lit(tx, ty int) bool  { return s.lit[[2]int{tx, ty}] }

func (s *fakeSky) LitDiscs(dst []LitDisc) []LitDisc { return append(dst, s.discs...) }

func nightFog(sky *fakeSky, sight TileSight) *Fog {
	f := NewFog(DefaultFogDials(), sight)
	f.SetLight(sky)

	return f
}

var him = []Eye{{ID: "s:1", X: 20.5, Y: 20.5}}

// TestNightShrinksSight: at deep night with no moon and nothing lit he sees
// the tiles within 1.5 of his tile's centre -- his own and the eight around
// it -- and a tile 4 away that he saw at noon is remembered, not seen.
//
// Negative control: drop the night term (UnlitReach returns DaySight) and
// this fails, "at deep night the tile 4 away (24,20) is visible"
// (nc-night-term.txt).
func TestNightShrinksSight(t *testing.T) {
	sky := &fakeSky{sky: 1}
	f := nightFog(sky, &openSight{})

	f.Update(48, 48, him)

	if st := f.At(24, 20); st != FogVisible {
		t.Fatalf("the control: at noon the tile 4 away (24,20) is %v", st)
	}

	sky.sky = 0
	f.Update(48, 48, him)

	if st := f.At(24, 20); st != FogExplored {
		t.Fatalf("at deep night the tile 4 away (24,20) is %v; want explored (seen at noon, not now)", st)
	}

	if _, visible := f.Counts(); visible != 9 {
		t.Fatalf("at deep night, no moon, nothing lit, he sees %d tiles; within 1.5 of his tile's centre are 9", visible)
	}

	if r := f.UnlitReach(); r != 1.5 {
		t.Fatalf("the unlit reach at deep night with no moon is %v, want the dark radius 1.5 (Q2)", r)
	}
}

// TestTheMoonWidensTheDark (Q2: "1.5 tiles, rising to about 4 under a full
// moon"): the dark radius is lerp(1.5, 4, moon).
//
// Negative control: ignore the moon (darkRadius = DarkRadius) and this fails,
// "under a full moon the tile 4 away (24,20) is unexplored" (nc-moon.txt).
func TestTheMoonWidensTheDark(t *testing.T) {
	if d := DefaultFogDials(); d.DarkRadius != 1.5 || d.MoonDarkRadius != 4 {
		t.Fatalf("fog ships dark %v, full-moon dark %v; Josh's Q2 is 1.5 rising to 4", d.DarkRadius, d.MoonDarkRadius)
	}

	for _, c := range []struct {
		moon     float64
		dark     float64
		in, past [2]int
	}{
		{1, 4, [2]int{24, 20}, [2]int{25, 20}},
		{0.5, 2.75, [2]int{22, 20}, [2]int{23, 20}},
		{0, 1.5, [2]int{21, 20}, [2]int{22, 20}},
	} {
		f := nightFog(&fakeSky{sky: 0, moon: c.moon}, &openSight{})
		f.Update(48, 48, him)

		if d := f.DarkRadius(); d != c.dark {
			t.Errorf("moon %v: the dark radius is %v, want %v", c.moon, d, c.dark)
		}

		if st := f.At(c.in[0], c.in[1]); st != FogVisible {
			t.Errorf("moon %v: the tile %v within the dark radius %v is %v", c.moon, c.in, c.dark, st)
		}

		if st := f.At(c.past[0], c.past[1]); st != FogUnexplored {
			t.Errorf("moon %v: the tile %v past the dark radius %v is %v", c.moon, c.past, c.dark, st)
		}
	}
}

// TestDuskBlends: from deep night to day the unlit reach and the tiles seen
// rise with the sky fraction and never fall, from the 9 of the dark radius to
// the 441 of day sight 12; a change of sky alone recomputes.
func TestDuskBlends(t *testing.T) {
	sky := &fakeSky{}
	f := nightFog(sky, &openSight{})
	last, lastReach := -1, -1.0

	for i := 0; i <= 8; i++ {
		sky.sky = float64(i) / 8

		if !f.Update(48, 48, him) && i > 0 {
			t.Fatalf("the sky moved to %v and fog did not recompute", sky.sky)
		}

		_, visible := f.Counts()
		reach := f.UnlitReach()
		t.Logf("sky %.3f: reach %.3f, %d visible", sky.sky, reach, visible)

		if visible < last || reach < lastReach {
			t.Fatalf("at sky %v he sees %d tiles (reach %v), fewer than %d (reach %v) at the darker sky before",
				sky.sky, visible, reach, last, lastReach)
		}

		last, lastReach = visible, reach
	}

	sky.sky = 0
	f.Update(48, 48, him)

	if _, v := f.Counts(); v != 9 {
		t.Fatalf("back at deep night he sees %d, want 9", v)
	}

	if last != 441 {
		t.Fatalf("at full day he saw %d, want 441", last)
	}
}

// litDisc lights every tile whose centre is within r of (x, y).
func litDisc(id int, x, y, r float64) (LitDisc, map[[2]int]bool) {
	lit := map[[2]int]bool{}

	for ty := 0; ty < 48; ty++ {
		for tx := 0; tx < 48; tx++ {
			if dx, dy := float64(tx)+0.5-x, float64(ty)+0.5-y; dx*dx+dy*dy < r*r {
				lit[[2]int{tx, ty}] = true
			}
		}
	}

	return LitDisc{ID: id, X: x, Y: y, Radius: r}, lit
}

// TestFarFireIsSeenAtAnyDistance (Q3, "lit ground with a clear line is seen
// at any distance"): a fire 24 tiles off -- twice his day sight -- is seen
// at night, and the dark ground between is not; a wall on the line hides it.
//
// Negative control: cap the lit term at the day sight (skip a lit tile past
// DaySight) and this fails, "the fire 24 tiles off (44,20) is unexplored"
// (nc-far-fire-capped.txt).
func TestFarFireIsSeenAtAnyDistance(t *testing.T) {
	disc, lit := litDisc(1, 44.5, 20.5, 3)
	sight := &openSight{}
	f := nightFog(&fakeSky{sky: 0, lit: lit, discs: []LitDisc{disc}}, sight)

	f.Update(48, 48, him)

	if st := f.At(44, 20); st != FogVisible {
		t.Fatalf("the fire 24 tiles off (44,20) is %v; lit ground is seen at any distance (Q3)", st)
	}

	if st := f.At(32, 20); st != FogUnexplored {
		t.Fatalf("the dark ground between, (32,20), is %v", st)
	}

	if f.LitSeen() == 0 {
		t.Fatal("the provider's lit_seen counts no tile seen by the lit term")
	}

	sight.blocked = map[[2]int]bool{{35, 20}: true}
	f.Forget()
	f.Update(48, 48, him)

	if st := f.At(44, 20); st != FogUnexplored {
		t.Fatalf("with a wall at (35,20) on the line the fire's tile (44,20) is %v; the line must be clear", st)
	}
}

// TestLitGroundIsSeenAtNight, on the REAL light model: a hearth 10 tiles off
// at deep night, new moon. Its lit tiles are seen; the dark tiles between
// are not. "Lit" is the drawn level above the QUANTISED sky.
//
// Negative control: compare against the raw Ambient (Lit: Level > Ambient())
// (either path: nc-raw-ambient, nc-raw-ambient-live) and this fails, "(23,23),
// the hearth's faintest edge ... is visible" --
// Level is quantised, so a tile the hearth lifts only from 0.10 to 0.12 is
// DRAWN at the night's own band, 0.125, and a raw "0.125 > 0.10" calls it lit
// (nc-raw-ambient.txt).
func TestLitGroundIsSeenAtNight(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.Add(SourceHearth, false, 30.5, 20.5)

	// Both of the view's paths: before the game has placed him this frame (the
	// light model's own sky, read per tile) and after (the sky read once).
	for _, live := range []bool{false, true} {
		view := NewLightView(l)
		if live {
			view.SetCarriedAt(20.5, 20.5)
		}

		f := NewFog(DefaultFogDials(), &openSight{})
		f.SetLight(view)
		f.Update(48, 48, him)

		for _, c := range []struct {
			x, y int
			want FogTile
			why  string
		}{
			{30, 20, FogVisible, "the hearth's own tile, 10 off"},
			{26, 20, FogVisible, "lit ground 4 from the hearth"},
			{22, 20, FogUnexplored, "the dark tile between, 2 from him and 8 from the hearth"},
			{21, 20, FogVisible, "within his dark radius"},
			{30, 30, FogUnexplored, "past the hearth's light"},
			{23, 23, FogUnexplored, "the hearth's faintest edge, drawn in the night's own band (0.125)"},
		} {
			if st := f.At(c.x, c.y); st != c.want {
				t.Errorf("view live %v: (%d,%d), %s, is %v; want %v (level %.3f, ambient %.3f)",
					live, c.x, c.y, c.why, st, c.want, l.Level(c.x, c.y), l.Ambient())
			}
		}
	}
}

// TestEachSquadIsAnEye (Q5's default): a second eye sees its own ground --
// at night, its own dark radius -- and a tile beside it is unexplored with
// him alone.
func TestEachSquadIsAnEye(t *testing.T) {
	sky := &fakeSky{sky: 0}
	alone := nightFog(sky, &openSight{})
	alone.Update(48, 48, him)

	if st := alone.At(36, 36); st != FogUnexplored {
		t.Fatalf("the control: with him alone, (36,36) is %v", st)
	}

	both := nightFog(sky, &openSight{})
	both.Update(48, 48, append([]Eye{}, him[0], Eye{ID: "s:2/e7", X: 35.5, Y: 35.5}))

	if st := both.At(36, 36); st != FogVisible {
		t.Fatalf("the second squad's model at (35,35) does not see (36,36): %v", st)
	}

	if _, v := both.Counts(); v != 18 {
		t.Fatalf("two eyes at deep night see %d tiles, want 9 each", v)
	}
}

// TestAContactShowsItsOwnTileOnly (Q4): an enemy of his fight 6 tiles off in
// the dark is shown -- its tile is visible -- and nothing around it is.
//
// Negative control: treat a contact as a full eye and this fails, "the tile
// beside the contact (27,20) is visible" (nc-contact-is-an-eye.txt).
func TestAContactShowsItsOwnTileOnly(t *testing.T) {
	f := nightFog(&fakeSky{sky: 0}, &openSight{})
	f.Update(48, 48, append([]Eye{}, him[0], Eye{ID: "fight/wolf", X: 26.2, Y: 20.7, Contact: true}))

	if st := f.At(26, 20); st != FogVisible {
		t.Fatalf("the enemy of his fight on (26,20) is on a tile that is %v; Q4 shows it", st)
	}

	for _, p := range [][2]int{{27, 20}, {25, 20}, {26, 21}} {
		if st := f.At(p[0], p[1]); st != FogUnexplored {
			t.Fatalf("the tile beside the contact %v is %v; a contact shows itself, not its ground", p, st)
		}
	}

	if cf := f.ClearFrom(26, 20); len(cf) != 1 {
		t.Fatalf("the probe asks lines of %v; a contact has none", cf)
	}
}

// TestTheTorchFollowsHimThroughAHeldTurn (BUG-108): the turn opens with him
// at (10.5,10.5) and his torch lit; the world is held, so the light model is
// not told he moves; he walks his Move two tiles east. The tile 4 ahead of
// where he now stands is lit and seen -- and the light model itself still
// has him where the turn opened (the sim reads it; fog does not write it).
//
// Negative control: never SetCarriedAt (the view reads the light model's
// stale position, master's behaviour) and this fails, "the tile 4 ahead
// (16,10) is unexplored" (nc-torch-lag.txt).
func TestTheTorchFollowsHimThroughAHeldTurn(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.Add(SourceTorch, true, 0, 0)
	l.SetPlayer(10.5, 10.5) // the turn opens

	view := NewLightView(l)
	f := NewFog(DefaultFogDials(), &openSight{})
	f.SetLight(view)
	f.Update(48, 48, []Eye{{ID: "s:1", X: 10.5, Y: 10.5}})

	// His Move: two tiles east, the world held.
	view.SetCarriedAt(12.5, 10.5)
	f.Update(48, 48, []Eye{{ID: "s:1", X: 12.5, Y: 10.5}})

	t.Logf("the tile 4 ahead (16,10): the light model draws it %.3f (torch where the turn opened), the view %.3f",
		l.Level(16, 10), view.Level(16, 10))

	if st := f.At(16, 10); st != FogVisible {
		t.Fatalf("the tile 4 ahead (16,10) is %v; his torch is where he stands", st)
	}

	if l.playerX != 10.5 || l.playerY != 10.5 {
		t.Fatalf("fog moved the light model's player to (%v,%v); the sim's light is not fog's to write", l.playerX, l.playerY)
	}
}

// TestFogNeverTouchesTheSim: fog's updates and its light view's moves leave
// the light model's state exactly as they found it -- fog is display only
// (plan §3.3). The game-level digest comparison is fog_test.go's act 10.
//
// Negative control: make SetCarriedAt also call the light model's SetPlayer
// (the plan's one-liner) and this fails, "fog changed the light model's
// state" (nc-view-writes-light.txt).
func TestFogNeverTouchesTheSim(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.Add(SourceTorch, true, 0, 0)
	l.Add(SourceHearth, false, 30.5, 20.5)
	l.SetPlayer(20.5, 20.5)

	before, _ := json.Marshal(l.HarnessState())

	view := NewLightView(l)
	f := NewFog(DefaultFogDials(), &openSight{})
	f.SetLight(view)

	for i := 0; i < 20; i++ {
		x := 20.5 + float64(i)*0.3
		view.SetCarriedAt(x, 20.5)
		f.Update(48, 48, []Eye{{ID: "s:1", X: x, Y: 20.5}})
	}

	after, _ := json.Marshal(l.HarnessState())

	if string(before) != string(after) {
		t.Fatalf("fog changed the light model's state:\nbefore %s\nafter  %s", before, after)
	}
}

// TestTheLightKeyRecomputes: with his tile unchanged, fog recomputes when the
// sky, the moon or a lit source changes, and only then.
//
// Negative control: leave the light out of the key (sameEyes alone) and
// this fails, "the moon rose and fog skipped" (nc-light-key.txt).
func TestTheLightKeyRecomputes(t *testing.T) {
	disc, lit := litDisc(1, 30.5, 20.5, 3)
	sky := &fakeSky{sky: 0, lit: lit, discs: []LitDisc{disc}}
	f := nightFog(sky, &openSight{})

	f.Update(48, 48, him)

	if f.Update(48, 48, him) {
		t.Fatal("nothing changed and fog recomputed")
	}

	for _, step := range []struct {
		what   string
		change func()
	}{
		{"the moon rose", func() { sky.moon = 0.5 }},
		{"the sky lifted", func() { sky.sky = 0.25 }},
		{"the fire moved", func() { sky.discs[0].X = 31.5 }},
		{"the fire went out", func() { sky.discs = nil }},
	} {
		step.change()

		if !f.Update(48, 48, him) {
			t.Fatalf("%s and fog skipped", step.what)
		}

		if f.Update(48, 48, him) {
			t.Fatalf("after %s, nothing changed and fog recomputed", step.what)
		}
	}
}

// TestTheViewIsTheLightWhereHeStands: with his torch where the light model
// last put him, the view's level is the light model's to the bit, on every
// tile, at deep night, at dusk and by day -- so with fog on the renderer
// draws exactly the light master draws whenever the world has just run.
//
// Negative control: forget the carried torch in the view's level (shine it
// from where it was lit, (0,0)) and this fails, "at the night floor the view draws
// (0,0) at 1.000, the light model at 0.125" (nc-view-not-the-light.txt).
func TestTheViewIsTheLightWhereHeStands(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.Add(SourceTorch, true, 0, 0)
	l.Add(SourceHearth, false, 30.5, 20.5)
	l.SetPlayer(12.3, 10.8)

	view := NewLightView(l)

	d := DefaultClockDials()

	for _, step := range []struct {
		name   string
		minute float64
	}{{"the night floor", d.NightStart + 30}, {"dawn", d.DawnStart + 30}, {"day", 12 * 60}} {
		advanceToMinuteOfDay(t, c, step.minute)
		view.SetCarriedAt(12.3, 10.8)

		for ty := 0; ty < 40; ty++ {
			for tx := 0; tx < 40; tx++ {
				if a, b := view.Level(tx, ty), l.Level(tx, ty); a != b {
					t.Fatalf("at %s the view draws (%d,%d) at %.3f, the light model at %.3f", step.name, tx, ty, a, b)
				}
			}
		}
	}
}
