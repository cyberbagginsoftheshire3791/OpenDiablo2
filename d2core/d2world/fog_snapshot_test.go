package d2world

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FOG OF WAR F3, "KEPT" (fog_snapshot.go): the explored grid saved and put
// back, keyed on its map; what he sees now and the dials are not saved.

// b2aFogMap is the map the fixture's grid is explored on (a SHA-256 in hex,
// as the world file's map.sha is).
var b2aFogMap = strings.Repeat("ab", 32)

const b2aFogW, b2aFogH = 30, 20

// b2aFogSight is the fixture's map: one house, footprint (25,9)-(28,12),
// which hides its own back (houseSight).
func b2aFogSight() houseSight {
	return houseSight{footprints: []image.Rectangle{image.Rect(25, 9, 28, 12)}}
}

// b2aFogWorld is a fog that has walked: he looked from (5,5), walked to
// (14,10) -- the house's west face in sight, so the house is explored whole
// -- and a script explored a patch in the south-west corner by hand. Day
// sight is narrowed to 6 so the grid holds explored, unexplored and visible
// ground all at once.
func b2aFogWorld(t *testing.T) *Fog {
	t.Helper()

	f := NewFog(b2aFogDials(), b2aFogSight())
	f.Update(b2aFogW, b2aFogH, []Eye{{ID: "s:1", X: 5.5, Y: 5.5}})
	f.Update(b2aFogW, b2aFogH, []Eye{{ID: "s:1", X: 14.5, Y: 10.5}})
	f.Update(b2aFogW, b2aFogH, []Eye{{ID: "s:1", X: 20.5, Y: 10.5}})
	f.Explore(2.5, 17.5, 2)

	return f
}

func b2aFogDials() FogDials {
	d := DefaultFogDials()
	d.DaySight = 6

	return d
}

// b2aFogSteps runs the fog on: he walks on east and back west, and a second
// squad's model looks from the north-east; each step reads every tile's state
// (unexplored, explored, visible), the counts and the snapshot. The first
// step moves him, so a restored fog (marked to recompute) and the original
// (which last recomputed where he stood) are both read after a recompute.
func b2aFogSteps(t *testing.T, f *Fog) string {
	t.Helper()

	var trace strings.Builder

	for _, eyes := range [][]Eye{
		{{ID: "s:1", X: 22.5, Y: 12.5}},
		{{ID: "s:1", X: 22.5, Y: 12.5}, {ID: "s:2/m", X: 27.5, Y: 2.5}},
		{{ID: "s:1", X: 8.5, Y: 14.5}},
	} {
		f.Update(b2aFogW, b2aFogH, eyes)

		explored, visible := f.Counts()
		fmt.Fprintf(&trace, "%d %d %s\n%s\n", explored, visible, b2aJSON(t, f.Snapshot(b2aFogMap)), strings.Join(f.Rows(), "\n"))
	}

	return trace.String()
}

// b2aFogClasses is every field of Fog, labelled.
func b2aFogClasses() []b2aClass {
	return []b2aClass{{Fog{}, map[string]string{
		"dials":         "W: the dials; the game builds fog with the defaults and the harness tunes them -- never saved (D2)",
		"sight":         "W: the map's line of sight, given at construction",
		"light":         "W: the light it sees the night by, given by the game screen",
		"key":           "D: the light and the eyes the last recompute saw; Restore marks the fog dirty, so the next Update recomputes",
		"discs":         "D: scratch for the next key, refilled every Update",
		"sky":           "D: the quantised sky of the last recompute, read again from the light",
		"darkRadius":    "D: tonight's dark radius, from the dials and the moon at the next recompute",
		"litTried":      "D: per-recompute scratch, sized with the grid",
		"litSeen":       "D: a count of the last recompute's lit term",
		"w":             "S:w",
		"h":             "S:h",
		"explored":      "S:explored",
		"stamp":         "D: what is visible now -- derived from the eyes at the next recompute, never saved",
		"epoch":         "D: the visible set's epoch, reset with the grid",
		"exploredCount": "D: the explored bits counted at Restore",
		"visibleCount":  "D: what is visible now, counted at the next recompute",
		"eyes":          "D: the eyes the game passes every Update",
		"eyeKeys":       "D: the eyes' tiles at the last recompute; Restore marks the fog dirty",
		"structures":    "D: the map's footprints, read from the map when the grid is sized",
		"dirty":         "D: set by Restore, so the first Update recomputes",
		"recomputes":    "D: this process's cost counter (the digest's process part), never the world's",
		"skipped":       "D: this process's frame counter, never the world's",
		"cellsRead":     "D: this process's cost counter, never the world's",
	}}}
}

func TestFogSnapshotFieldsClassified(t *testing.T) {
	f := b2aFogWorld(t)

	for _, c := range b2aFogClasses() {
		b2aClassified(t, c.kind, f.Snapshot(b2aFogMap), c.fields)
	}
}

// TestFogSnapshotRoundTrips: the grid survives JSON exactly, and a fog
// restored from it -- one that had explored other ground first -- remembers
// exactly the ground the original did, and stays the same fog as both run on.
// Nothing is visible until the restored fog's first Update (visible is
// derived, never saved).
func TestFogSnapshotRoundTrips(t *testing.T) {
	f := b2aFogWorld(t)

	snap := b2aThroughJSON(t, f.Snapshot(b2aFogMap))
	require.Equal(t, f.Snapshot(b2aFogMap), snap, "the snapshot survives JSON exactly")
	require.Equal(t, b2aFogMap, snap.Map)
	require.Equal(t, [2]int{b2aFogW, b2aFogH}, [2]int{snap.W, snap.H})
	require.Len(t, snap.Explored, 4*(((b2aFogW*b2aFogH+7)/8+2)/3), "75 bytes of bits, padded base64: 100 characters")

	explored, _ := f.Counts()
	require.Greater(t, explored, 100, "the fixture explored ground")
	require.Less(t, explored, b2aFogW*b2aFogH-100, "and left ground unexplored")

	g := NewFog(b2aFogDials(), b2aFogSight())
	g.Update(b2aFogW, b2aFogH, []Eye{{ID: "s:1", X: 25.5, Y: 17.5}}) // ground the original never saw
	require.NoError(t, g.Restore(snap, b2aFogW, b2aFogH, b2aFogMap))

	gExplored, gVisible := g.Counts()
	require.Equal(t, explored, gExplored, "the restored fog remembers as much")
	require.Zero(t, gVisible, "and sees nothing until its first Update: visible is not saved")

	for ty := 0; ty < b2aFogH; ty++ {
		for tx := 0; tx < b2aFogW; tx++ {
			fe, _ := f.FogAt(tx, ty)
			ge, gv := g.FogAt(tx, ty)
			require.Equal(t, fe, ge, "tile (%d,%d): explored", tx, ty)
			require.False(t, gv, "tile (%d,%d) visible before any Update", tx, ty)
		}
	}

	require.Equal(t, snap, g.Snapshot(b2aFogMap), "restored, it snapshots to the same grid")
	require.Equal(t, b2aFogSteps(t, f), b2aFogSteps(t, g), "and stays the same fog when both run on")
}

// TestFogSnapshotOfANewGame: a fog that has never looked (fog off, or saved
// before its first look) is the empty snapshot -- no map, no grid -- and
// restoring it leaves a fog that remembers nothing: its next Update sizes it
// and sees only what the eyes see, as a new game does.
func TestFogSnapshotOfANewGame(t *testing.T) {
	fresh := NewFog(b2aFogDials(), b2aFogSight())
	snap := b2aThroughJSON(t, fresh.Snapshot(b2aFogMap))
	require.Equal(t, FogSnapshot{}, snap, "a fog that never looked names no map and holds no grid")
	require.True(t, snap.Empty())
	require.NoError(t, snap.Check())

	g := b2aFogWorld(t)
	require.NoError(t, g.Restore(snap, b2aFogW, b2aFogH, b2aFogMap), "an empty grid fits any map")

	explored, visible := g.Counts()
	w, h := g.Size()
	require.Equal(t, [4]int{0, 0, 0, 0}, [4]int{explored, visible, w, h}, "restored empty: unsized, nothing remembered")

	other := NewFog(b2aFogDials(), b2aFogSight())

	require.Equal(t, b2aFogSteps(t, other), b2aFogSteps(t, g), "and from there it is a new game's fog")
}

// TestFogRefusesAGridOfTheWrongSize, and of the wrong map, and malformed: each
// is refused by Validate and by Restore, and a refused Restore changes
// nothing. The map is the F1 review's C6: a grid of the RIGHT size explored on
// another map is refused (D5's rule), not laid over this one.
//
// Negative controls (1 Oct 2026, strigoi-harness-runs\wt-fog3\nc\): drop
// Validate's size case and "another map's size" is restored (nc1); drop its
// map case and "the right size on another map" is (nc2); take the encoder's
// spelling check out of fogDecode and "a grid broken across lines" is (nc3);
// drop the padding-bits check and "a bit past the last tile" is (nc4).
func TestFogRefusesAGridOfTheWrongSize(t *testing.T) {
	good := b2aFogWorld(t).Snapshot(b2aFogMap)
	raw, err := base64.StdEncoding.DecodeString(good.Explored)
	require.NoError(t, err)

	encode := func(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

	cases := map[string]FogSnapshot{
		"another map's size":            {Map: b2aFogMap, W: b2aFogH, H: b2aFogW, Explored: good.Explored},
		"a grid one row short":          {Map: b2aFogMap, W: b2aFogW, H: b2aFogH - 1, Explored: encode(raw[:(b2aFogW*(b2aFogH-1)+7)/8])},
		"the right size on another map": {Map: strings.Repeat("cd", 32), W: b2aFogW, H: b2aFogH, Explored: good.Explored},
		"the generated map's grid":      {Map: "", W: b2aFogW, H: b2aFogH, Explored: good.Explored},
		"a grid a byte short":           {Map: b2aFogMap, W: b2aFogW, H: b2aFogH, Explored: encode(raw[:len(raw)-1])},
		"a grid a byte long":            {Map: b2aFogMap, W: b2aFogW, H: b2aFogH, Explored: encode(append(append([]byte(nil), raw...), 0))},
		"a grid that is not base64":     {Map: b2aFogMap, W: b2aFogW, H: b2aFogH, Explored: "*" + good.Explored[1:]},
		"a grid broken across lines":    {Map: b2aFogMap, W: b2aFogW, H: b2aFogH, Explored: good.Explored[:40] + "\n" + good.Explored[40:]},
		"a grid of one side":            {Map: b2aFogMap, W: b2aFogW, Explored: good.Explored},
		"a grid of negative size":       {Map: b2aFogMap, W: -b2aFogW, H: -b2aFogH, Explored: good.Explored},
		"a grid past any map":           {Map: b2aFogMap, W: 5000, H: b2aFogH, Explored: good.Explored},
		"an empty grid naming a map":    {Map: b2aFogMap},
		"an empty grid holding bits":    {Explored: good.Explored},
	}

	f := b2aFogWorld(t)

	for name, bad := range cases {
		require.Error(t, f.Validate(bad, b2aFogW, b2aFogH, b2aFogMap), name)

		before := strings.Join(f.Rows(), "\n") + b2aJSON(t, f.Snapshot(b2aFogMap))
		require.Error(t, f.Restore(bad, b2aFogW, b2aFogH, b2aFogMap), name)
		require.Equal(t, before, strings.Join(f.Rows(), "\n")+b2aJSON(t, f.Snapshot(b2aFogMap)),
			"%s: a refused restore changes nothing", name)
	}

	// The control: the good grid is taken, on its map and size.
	require.NoError(t, f.Validate(good, b2aFogW, b2aFogH, b2aFogMap))
	require.NoError(t, f.Restore(good, b2aFogW, b2aFogH, b2aFogMap))

	// A bit past the last tile: 600 tiles fill 75 bytes exactly, so a grid
	// of 599 tiles (599 x 1) has one bit past its last -- set, refused by the
	// snapshot's own check; clear, taken (so it is that bit that refuses).
	pastLast := append([]byte(nil), raw...)
	pastLast[len(pastLast)-1] |= 0x80
	require.Error(t, FogSnapshot{Map: b2aFogMap, W: b2aFogW*b2aFogH - 1, H: 1, Explored: encode(pastLast)}.Check(),
		"a bit past the last tile")

	pastLast[len(pastLast)-1] &^= 0x80
	require.NoError(t, FogSnapshot{Map: b2aFogMap, W: b2aFogW*b2aFogH - 1, H: 1, Explored: encode(pastLast)}.Check())
}

// TestFogSnapshotEveryFieldIsLoadBearing is the B-series sweep (b2aSweep):
// every field of the snapshot, changed or dropped, is refused or seen when the
// restored fog runs on; and a valid grid with ground forgotten, or ground
// remembered that was not seen, restores and is seen.
func TestFogSnapshotEveryFieldIsLoadBearing(t *testing.T) {
	f := b2aFogWorld(t)
	snap := f.Snapshot(b2aFogMap)
	ref := b2aFogSteps(t, f)

	try := func(raw []byte) (string, error) {
		var s FogSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		g := NewFog(b2aFogDials(), b2aFogSight())
		if err := g.Restore(s, b2aFogW, b2aFogH, b2aFogMap); err != nil {
			return "", err
		}

		return b2aFogSteps(t, g), nil
	}

	b2aExercised(t, b2aSweep(t, snap, ref, map[string]string{}, try), b2aFogClasses()...)

	flip := func(s *FogSnapshot, tile int) {
		raw, err := base64.StdEncoding.DecodeString(s.Explored)
		require.NoError(t, err)

		raw[tile/8] ^= 1 << (uint(tile) % 8)
		s.Explored = base64.StdEncoding.EncodeToString(raw)
	}

	b2aMustDiverge(t, snap, ref, map[string]func(s *FogSnapshot){
		// (2,17) the script explored by hand; (0,0) never seen.
		"a tile forgotten":            func(s *FogSnapshot) { flip(s, 17*b2aFogW+2) },
		"a tile remembered unseen":    func(s *FogSnapshot) { flip(s, 0) },
		"the last tile remembered":    func(s *FogSnapshot) { flip(s, b2aFogW*b2aFogH-1) },
		"nothing remembered (a grid)": func(s *FogSnapshot) { s.Explored = base64.StdEncoding.EncodeToString(make([]byte, 75)) },
		"the empty block":             func(s *FogSnapshot) { *s = FogSnapshot{} },
	}, try)
}

// TestFogDialsAreNotSaved (D2): the dials are this build's tuning. A fog
// restored into a build with another day sight keeps ITS day sight, and sees
// by it.
func TestFogDialsAreNotSaved(t *testing.T) {
	snap := b2aFogWorld(t).Snapshot(b2aFogMap)

	d := b2aFogDials()
	d.DaySight = 3
	g := NewFog(d, b2aFogSight())
	require.NoError(t, g.Restore(snap, b2aFogW, b2aFogH, b2aFogMap))
	require.Equal(t, d, g.Dials(), "the restored fog's dials are its own")

	g.Update(b2aFogW, b2aFogH, []Eye{{ID: "s:1", X: 8.5, Y: 14.5}})
	require.Equal(t, FogExplored, g.At(13, 14), "5 tiles off, past the restored fog's own day sight of 3: remembered, not seen")
}

// TestTheGridsLayoutIsPinned (the F3 review's B1): the documented layout,
// pinned to exact strings -- tile (x,y) at bit y*W+x, lowest bit first,
// padding zero. A 5 x 3 grid fully revealed is bytes FF 7F ("/38="); tiles
// (0,0), (4,0), (1,1), (4,2) alone are bits 0, 4, 6, 14: bytes 51 40
// ("UUA="). A round trip alone cannot see a layout that is wrong both ways.
//
// Negative controls (strigoi-harness-runs\wt-fog3\nc\): Snapshot drops the
// last tile (n := f.w*f.h - 1, the reviewer's m42) and this fails, "a
// revealed 5 x 3 grid is \"/38A\"" (ncb1-m42); Snapshot and Restore both
// transposed (the reviewer's m1) and it fails, "tile [4 0] is not explored"
// (ncb1-m1).
func TestTheGridsLayoutIsPinned(t *testing.T) {
	all := NewFog(DefaultFogDials(), &openSight{})
	all.Update(5, 3, nil)
	all.RevealAll()

	if got := all.Snapshot("m").Explored; got != "/38=" {
		t.Errorf("a revealed 5 x 3 grid is %q; want \"/38=\" (every tile, the last included)", got)
	}

	some := NewFog(DefaultFogDials(), &openSight{})
	some.Update(5, 3, nil)

	if err := some.Restore(FogSnapshot{Map: "m", W: 5, H: 3, Explored: "UUA="}, 5, 3, "m"); err != nil {
		t.Fatal(err)
	}

	want := map[[2]int]bool{{0, 0}: true, {4, 0}: true, {1, 1}: true, {4, 2}: true}

	for y := 0; y < 3; y++ {
		for x := 0; x < 5; x++ {
			if e, _ := some.FogAt(x, y); e != want[[2]int{x, y}] {
				t.Errorf("tile [%d %d] explored %v from \"UUA=\"; want %v", x, y, e, want[[2]int{x, y}])
			}
		}
	}

	if n, _ := some.Counts(); n != 4 {
		t.Errorf("\"UUA=\" restores %d tiles; want 4", n)
	}

	if got := some.Snapshot("m").Explored; got != "UUA=" {
		t.Errorf("the four tiles snapshot as %q; want \"UUA=\"", got)
	}
}

// TestAMidTileTorchSaveResumesToTheSameGrid (the F3 review's A1, its probe
// TestReviewProbeMidTileTorchResume): at deep night, his torch carried and
// lit, he walks east in small steps. At EVERY frame of the walk the game is
// saved and resumed: a new fog, the grid restored, one frame standing where
// he stands -- and the uninterrupted game runs one more frame standing there.
// The two grids are one (S_R = S_U). Fog sees by his torch from the centre of
// his tile, so what it lights for him is a function of his tile alone.
//
// Negative control (strigoi-harness-runs\wt-fog3\nc\): fog.go and
// light_view.go as 4984fde0 had them (the carried disc keyed by tile, lit
// from his exact point) and this fails, "at x=20.95 S_R != S_U: uninterrupted
// 101 explored, resumed 110" (nca1-before-the-fix).
func TestAMidTileTorchSaveResumesToTheSameGrid(t *testing.T) {
	c, l := deepNightNewMoon(t)

	defer c.Close()
	defer l.Close()

	l.Add(SourceTorch, true, 0, 0)

	const mapID = "abab"

	view := NewLightView(l)
	f := NewFog(DefaultFogDials(), &openSight{})
	f.SetLight(view)

	stand := func(g *Fog, v *LightView, x float64) {
		v.SetCarriedAt(x, 20.5)
		g.Update(48, 48, []Eye{{ID: "s:1", X: x, Y: 20.5}})
	}

	frames, bad := 0, 0

	// He walks east from (16.5,20.5) to (20.95,20.5) in steps of 0.05.
	for i := 0; i <= 89; i++ {
		x := 16.5 + 0.05*float64(i)
		stand(f, view, x)

		snap := f.Snapshot(mapID) // S_T

		view2 := NewLightView(l)
		g := NewFog(DefaultFogDials(), &openSight{})
		g.SetLight(view2)

		if err := g.Restore(snap, 48, 48, mapID); err != nil {
			t.Fatal(err)
		}

		stand(f, view, x)  // uninterrupted: one more frame standing there
		stand(g, view2, x) // resumed: the first frame, standing there

		frames++

		if u, r := f.Snapshot(mapID), g.Snapshot(mapID); u != r {
			bad++

			if bad <= 3 {
				ue, _ := f.Counts()
				re, _ := g.Counts()
				t.Errorf("at x=%.2f S_R != S_U: uninterrupted %d explored, resumed %d", x, ue, re)
			}
		}
	}

	if frames != 90 {
		t.Fatalf("the walk ran %d frames, want 90", frames)
	}

	if bad > 0 {
		t.Fatalf("%d of %d mid-walk resumes remembered other ground than the game that ran on", bad, frames)
	}

	if f.LitSeen() == 0 {
		t.Fatal("the control: his torch lit no ground past his dark radius; the test proves nothing")
	}
}
