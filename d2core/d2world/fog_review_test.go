package d2world

import (
	"image"
	"testing"
)

// FOG OF WAR F2, THE REVIEW'S FIXES (fog-f2 @ fd3c2e6d, 1 Oct 2026). Each test
// names the finding it holds and the break it was run red against; the logs
// are in strigoi-harness-runs\wt-fog2\nc\ (nc-r-*.txt).

var reviewHouse = houseSight{footprints: []image.Rectangle{image.Rect(25, 9, 28, 12)}}

// TestAContactInAHouseShowsOnlyItsTile (B1; the reviewer's
// TestProbeContactInHouseRevealsWholeHouse): an enemy of his fight standing
// inside a house far off in the dark shows its own tile -- not the house.
//
// Negative control: show contacts before the whole-structure reveal (as
// fd3c2e6d did) and this fails, "a contact inside a house shows 9 house
// tiles" (nc-r-contact-before-structures.txt).
func TestAContactInAHouseShowsOnlyItsTile(t *testing.T) {
	f := nightFog(&fakeSky{sky: 0}, reviewHouse)
	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 10.5}, {ID: "fight/wolf", X: 26.5, Y: 10.5, Contact: true}})

	n := 0

	for ty := 9; ty < 12; ty++ {
		for tx := 25; tx < 28; tx++ {
			if f.At(tx, ty) == FogVisible {
				n++
			}
		}
	}

	if n != 1 || f.At(26, 10) != FogVisible {
		t.Fatalf("a contact inside a house shows %d house tiles (its own (26,10) is %v); want only its own", n, f.At(26, 10))
	}
}

// TestAContactLeavesNoMemory (B2; the reviewer's TestProbeContactLeavesMemory):
// while the fight lasts the contact's tile is shown (visible, not explored);
// after it, ground he never saw is black again, not remembered.
//
// Negative control: show a contact's tile with see() (explore it) and this
// fails, "after the fight the contact's tile (30,30) is explored"
// (nc-r-contact-explores.txt).
func TestAContactLeavesNoMemory(t *testing.T) {
	f := nightFog(&fakeSky{sky: 0}, &openSight{})
	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 10.5}, {ID: "fight/wolf", X: 30.5, Y: 30.5, Contact: true}})

	if explored, visible := f.FogAt(30, 30); !visible || explored {
		t.Fatalf("during the fight the contact's tile (30,30) is explored %v, visible %v; want shown, not explored", explored, visible)
	}

	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 10.5}})

	if st := f.At(30, 30); st != FogUnexplored {
		t.Fatalf("after the fight the contact's tile (30,30) is %v; he never saw that ground", st)
	}
}

// TestADeadModelIsNoEye (B3): a deployed squad's model at 0 health is not
// among the living models fog takes its eyes from, nor is his own model when
// his body is at 0; ModelEntities (the selection hit test's) still lists all.
//
// Negative control: LivingModelEntities returns ModelEntities and this fails,
// "the living models are [s:1 s:2 s:3]" (nc-r-dead-eye.txt).
func TestADeadModelIsNoEye(t *testing.T) {
	c := NewClock(DefaultClockDials())
	t.Cleanup(c.Close)

	s := NewSquads(c, DefaultMeterDials(), &fakeDeployer{})
	t.Cleanup(s.Close)

	body := &fakeBody{health: 80, maxHealth: 100}
	s.BindPlayer(body, "p:1")

	for i := 0; i < 2; i++ {
		if err := s.HarnessSet("squad_add", map[string]interface{}{"x": 3.0, "y": 4.0}); err != nil {
			t.Fatal(err)
		}
	}

	squadsOf := func(ms []SquadModel) []string {
		out := []string{}
		for _, m := range ms {
			out = append(out, m.Squad)
		}

		return out
	}

	if got := squadsOf(s.LivingModelEntities()); len(got) != 3 {
		t.Fatalf("the control: three living models, got %v", got)
	}

	s.squads["s:2"].members[0].health = 0

	if got := squadsOf(s.LivingModelEntities()); len(got) != 2 || got[0] != "s:1" || got[1] != "s:3" {
		t.Fatalf("with s:2's model dead the living models are %v; want [s:1 s:3]", got)
	}

	body.health = 0

	if got := squadsOf(s.LivingModelEntities()); len(got) != 1 || got[0] != "s:3" {
		t.Fatalf("with his body at 0 the living models are %v; want [s:3]", got)
	}

	if got := len(s.ModelEntities()); got != 3 {
		t.Fatalf("ModelEntities lists %d models; the selection hit test still sees all 3", got)
	}
}

// TestATorchWalkRecomputesPerTileStep (B4): his lit torch walks with him
// across three tiles in thirty frames; fog recomputes as he enters each tile,
// not each frame -- the carried disc is keyed by its tile.
//
// Negative control: key a carried disc exactly (keyOf returns the disc) and
// this fails, "a torch-lit walk of 30 frames over 3 tiles recomputed 30
// times" (nc-r-carried-exact.txt).
func TestATorchWalkRecomputesPerTileStep(t *testing.T) {
	sky := &fakeSky{sky: 0}
	f := nightFog(sky, &openSight{})
	n := 0

	for i := 0; i < 30; i++ {
		x := 10.05 + 0.1*float64(i)
		sky.discs = []LitDisc{{ID: 1, X: x, Y: 20.5, Radius: 5, Carried: true}}

		if f.Update(48, 48, []Eye{{ID: "s:1", X: x, Y: 20.5}}) {
			n++
		}
	}

	if n != 3 {
		t.Fatalf("a torch-lit walk of 30 frames over 3 tiles recomputed %d times; want 3, one a tile", n)
	}

	// A FIXED source that moves is a new fire: keyed exactly.
	sky.discs = []LitDisc{{ID: 2, X: 30.5, Y: 20.5, Radius: 5}}
	f.Update(48, 48, []Eye{{ID: "s:1", X: 12.95, Y: 20.5}})
	sky.discs[0].X = 30.6

	if !f.Update(48, 48, []Eye{{ID: "s:1", X: 12.95, Y: 20.5}}) {
		t.Fatal("a fixed source moved and fog skipped")
	}
}

// westWall blocks every line from west of x=10 to east of it, and nothing
// else: an eye west of it is walled in; an eye east of it sees all.
type westWall struct{}

func (westWall) TileSightClear(fx, _ float64, tx, _ int) (bool, int) {
	return !(fx < 10 && tx > 10), 1
}

// TestAContactSeesNoLitGround (the review's M2): a contact shows its own tile
// and sees nothing else -- not even lit ground with a clear line from it. A
// fire beyond a wall he cannot see past stays unseen though an enemy of his
// fight stands in clear view of it.
//
// Negative control: let contacts try the lit term's lines (the reviewer's
// M2) and this fails, "the fire (30,20) behind the wall is visible"
// (nc-r-contact-sees-lit.txt).
func TestAContactSeesNoLitGround(t *testing.T) {
	disc, lit := litDisc(1, 30.5, 20.5, 2)
	f := nightFog(&fakeSky{sky: 0, lit: lit, discs: []LitDisc{disc}}, westWall{})
	f.Update(48, 48, []Eye{{ID: "s:1", X: 5.5, Y: 20.5}, {ID: "fight/wolf", X: 28.5, Y: 24.5, Contact: true}})

	if st := f.At(28, 24); st != FogVisible {
		t.Fatalf("the control: the contact's own tile is %v", st)
	}

	if st := f.At(30, 20); st != FogUnexplored {
		t.Fatalf("the fire (30,20) behind the wall is %v; a contact sees nothing for him", st)
	}
}

// TestTheDarkRadiusNeverPassesTheDaySight (C5): dials that put the dark
// radius past the day sight do not make the night see further than the day.
//
// Negative control: drop UnlitReach's cap (dark not min'd with DaySight) and
// this fails, "at deep night with a dark radius of 20 he sees 20 tiles"
// (nc-r-dark-uncapped.txt).
func TestTheDarkRadiusNeverPassesTheDaySight(t *testing.T) {
	d := DefaultFogDials()
	d.DarkRadius, d.MoonDarkRadius = 20, 20

	f := NewFog(d, &openSight{})
	f.SetLight(&fakeSky{sky: 0})
	f.Update(48, 48, him)

	if r := f.UnlitReach(); r != 12 {
		t.Fatalf("at deep night with a dark radius of 20 he sees %v tiles; never past the day sight 12", r)
	}

	if st := f.At(33, 20); st != FogUnexplored {
		t.Fatalf("the tile 13 off (33,20) is %v", st)
	}
}

// TestDuskKeepsTheLitSetAndItsCost (C1, the reviewer's
// TestProbeDuskLitBandDesync; and M5): through a whole dusk, frame by frame,
// with a hearth 25 tiles off, the tiles fog sees by the lit term are exactly
// the tiles the light draws lit on every frame -- the sky as drawn (the
// ambient's 1/16 band) is in the key -- and fog recomputes a bounded number
// of times (the sky's 1/64 steps and the bands), not once a frame.
//
// Negative controls: leave the band out of the key and this fails, "frames
// where fog's lit set is not the drawn one" (nc-r-band-not-keyed.txt); key
// the raw sky (the reviewer's M5) and it fails, "recomputed N times in M
// frames" (nc-r-sky-unquantised.txt).
func TestDuskKeepsTheLitSetAndItsCost(t *testing.T) {
	c := NewClock(DefaultClockDials())
	l := NewLight(c, DefaultLightDials())

	defer c.Close()
	defer l.Close()

	c.SetMoon(0)
	advanceToMinuteOfDay(t, c, DefaultClockDials().DuskStart)
	l.Add(SourceHearth, false, 30.5, 20.5)

	view := NewLightView(l)
	f := NewFog(DefaultFogDials(), &openSight{})
	f.SetLight(view)

	me := []Eye{{ID: "s:1", X: 5.5, Y: 20.5}}
	frames, bad, recomputes := 0, 0, 0

	// A small step, so the dusk is thousands of frames, as a real one is.
	for i := 0; i < 400000 && c.Stage() != StageNight; i++ {
		c.Advance(0.01)
		view.SetCarriedAt(5.5, 20.5)

		if f.Update(48, 48, me) {
			recomputes++
		}

		frames++

		if f.UnlitReach() >= 12 {
			continue // the day's reach covers nothing out here, but say so
		}

		for tx := 23; tx <= 38; tx++ {
			if view.Lit(tx, 20) != (f.At(tx, 20) == FogVisible) {
				bad++

				break
			}
		}
	}

	t.Logf("dusk: %d frames, %d recomputes, %d frames where fog's lit set is not the drawn one", frames, recomputes, bad)

	if frames < 1000 {
		t.Fatalf("the dusk ran %d frames; the test needs a real dusk", frames)
	}

	if bad != 0 {
		t.Fatalf("%d frames where fog's lit set is not the drawn one", bad)
	}

	if recomputes > 100 {
		t.Fatalf("recomputed %d times in %d frames; the sky's 64 steps and 16 bands bound it", recomputes, frames)
	}
}
