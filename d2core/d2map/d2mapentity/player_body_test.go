package d2mapentity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

func placeholderHero(t *testing.T) *pngBody {
	t.Helper()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	if err := asset.AddSource(filepath.Join("..", "..", ".."), types.AssetSourceFileSystem); err != nil {
		t.Fatal(err)
	}

	f := &MapEntityFactory{asset: asset}

	body, err := f.loadHeroBody("/data/strigoi/hero/placeholder/hero.json", "hth", 0)
	if err != nil {
		t.Fatalf("the shipped placeholder hero does not load: %v", err)
	}

	return body
}

// Every player animation mode lands on a sheet, falling back when the hero
// has none of its own -- and reports the MODE asked for, as the composite
// does, so the player's own bookkeeping cannot tell the bodies apart.
func TestAPNGHeroDrawsEveryMode(t *testing.T) {
	b := placeholderHero(t)

	// The placeholder's sheets have distinct frame counts: idle 4, walk 8,
	// attack 6, hit 3, death 6, dead 1.
	for _, c := range []struct {
		mode   d2enum.PlayerAnimationMode
		sheet  string
		frames int
	}{
		{d2enum.PlayerAnimationModeTownNeutral, "idle", 4},
		{d2enum.PlayerAnimationModeNeutral, "idle", 4},
		{d2enum.PlayerAnimationModeTownWalk, "walk", 8},
		{d2enum.PlayerAnimationModeRun, "walk", 8}, // no run sheet: walk
		{d2enum.PlayerAnimationModeAttack1, "attack", 6},
		{d2enum.PlayerAnimationModeCast, "attack", 6},
		{d2enum.PlayerAnimationModeGetHit, "hit", 3},
		{d2enum.PlayerAnimationModeBlock, "hit", 3},    // no block sheet: hit
		{d2enum.PlayerAnimationModeSequence, "hit", 3}, // "GH" to the composite too
		{d2enum.PlayerAnimationModeDeath, "death", 6},
		{d2enum.PlayerAnimationModeDead, "dead", 1},
		{d2enum.PlayerAnimationModeNone, "idle", 4}, // anything else: idle
	} {
		if err := b.SetMode(c.mode, "hth"); err != nil {
			t.Fatalf("%v: %v", c.mode, err)
		}

		if b.Sheet() != c.sheet || b.GetFrameCount() != c.frames {
			t.Errorf("%v draws the %q sheet (%d frames), want %q (%d)", c.mode, b.Sheet(), b.GetFrameCount(), c.sheet, c.frames)
		}

		if b.GetAnimationMode() != c.mode.String() {
			t.Errorf("%v reports mode %q", c.mode, b.GetAnimationMode())
		}
	}

	if w, h := b.GetSize(); w != 96 || h != 90 {
		t.Fatalf("size %dx%d, want 96x90 (the cell's width, the manifest's height)", w, h)
	}
}

// A hero with no dead sheet lies still on the last of his death.
func TestDeadFallsBackToDeath(t *testing.T) {
	full := placeholderHero(t)

	sheets := map[string]d2interface.Animation{}
	for name, s := range full.sheets {
		if name != "dead" {
			sheets[name] = s
		}
	}

	b, err := newPNGBody(sheets, full.lengths, "hth", 0)
	if err != nil {
		t.Fatal(err)
	}

	_ = b.SetMode(d2enum.PlayerAnimationModeDead, "hth")

	if b.Sheet() != "death" {
		t.Fatalf("dead with no dead sheet draws %q, want death", b.Sheet())
	}
}

// The body faces the way the player does: the player's 64 facings land on the
// sheet's rows in Diablo II's 8-direction order (SW NW NE SE S W N E) -- and
// keep facing that way across a change of sheet.
func TestAPNGHeroFacesTheRightRow(t *testing.T) {
	b := placeholderHero(t)

	for _, c := range []struct {
		dir64 int
		row   int
		name  string
	}{
		{0, 4, "S"}, {8, 0, "SW"}, {16, 5, "W"}, {24, 1, "NW"},
		{32, 6, "N"}, {40, 2, "NE"}, {48, 7, "E"}, {56, 3, "SE"},
	} {
		b.SetDirection(c.dir64)

		if got := b.anim.GetDirection(); got != c.row {
			t.Errorf("facing %d (%s) draws row %d, want %d", c.dir64, c.name, got, c.row)
		}
	}

	b.SetDirection(56)
	_ = b.SetMode(d2enum.PlayerAnimationModeTownWalk, "hth")

	if b.anim.GetDirection() != 3 || b.GetDirection() != 56 {
		t.Fatalf("after a change of sheet he faces row %d (facing %d), want row 3 (56)", b.anim.GetDirection(), b.GetDirection())
	}
}

// A sheet plays at its manifest fps; without one, in a second; and a run drawn
// with the walk sheet steps faster than the walk, by the run's speed over the
// walk's, so his feet keep pace with the ground.
func TestAPNGHeroKeepsTime(t *testing.T) {
	b := placeholderHero(t)

	frameAfter := func(mode d2enum.PlayerAnimationMode, seconds float64) int {
		_ = b.SetMode(d2enum.PlayerAnimationModeNone, "hth") // a different mode first: start from the top
		_ = b.SetMode(mode, "hth")
		_ = b.Advance(seconds)

		return b.GetCurrentFrame()
	}

	// attack: 6 frames at the placeholder's 10 fps -- frame 3 at 0.35 s (the
	// engine's default second would be on frame 2).
	if f := frameAfter(d2enum.PlayerAnimationModeAttack1, 0.35); f != 3 {
		t.Errorf("the attack at 0.35 s is on frame %d, want 3 (10 fps)", f)
	}

	// walk: 8 frames in the default second -- frame 4 at 0.5 s.
	if f := frameAfter(d2enum.PlayerAnimationModeTownWalk, 0.5); f != 4 {
		t.Errorf("the walk at 0.5 s is on frame %d, want 4", f)
	}

	// run on the walk sheet: the second scaled by 9/13 -- frame 5 at 0.5 s.
	if f := frameAfter(d2enum.PlayerAnimationModeRun, 0.5); f != 5 {
		t.Errorf("the run at 0.5 s is on frame %d, want 5 (the walk sheet played 13/9 as fast)", f)
	}
}

// Asking again for the mode already set changes nothing: the player asks
// every tick, and a rewind each time would freeze him on frame 0.
func TestSetModeAgainIsANoOp(t *testing.T) {
	b := placeholderHero(t)
	_ = b.SetMode(d2enum.PlayerAnimationModeTownWalk, "hth")

	_ = b.Advance(0.3)
	frame := b.GetCurrentFrame()

	if frame == 0 {
		t.Fatal("control: the walk did not advance")
	}

	_ = b.SetMode(d2enum.PlayerAnimationModeTownWalk, "hth")

	if b.GetCurrentFrame() != frame {
		t.Fatalf("setting the same mode rewound the walk from frame %d to %d", frame, b.GetCurrentFrame())
	}

	// A different mode does start from the top.
	_ = b.SetMode(d2enum.PlayerAnimationModeTownNeutral, "hth")

	if b.GetCurrentFrame() != 0 {
		t.Fatal("a new mode did not start at frame 0")
	}
}

func TestHeroManifest(t *testing.T) {
	m, err := parseHeroManifest([]byte(`{"name":"x","animations":{"idle":"idle.png","walk":"/abs/walk.png","hit":"sub\\hit.png"},"fps":{"walk":12}}`),
		"/data/strigoi/hero/x/hero.json")
	if err != nil {
		t.Fatal(err)
	}

	if m.Animations.Idle != "/data/strigoi/hero/x/idle.png" || m.Animations.Walk != "/abs/walk.png" ||
		m.Animations.Hit != "/data/strigoi/hero/x/sub/hit.png" {
		t.Fatalf("paths %q %q %q; want relative joined, absolute kept, backslashes turned", m.Animations.Idle, m.Animations.Walk, m.Animations.Hit)
	}

	if m.FPS["walk"] != 12 {
		t.Fatalf("fps %v", m.FPS)
	}

	for name, c := range map[string]struct{ manifest, says string }{
		"typo":        {`{"animations":{"idle":"i.png","atack":"a.png"}}`, "atack"},
		"no idle":     {`{"animations":{"walk":"w.png"}}`, "idle"},
		"not json":    {`idle`, ""},
		"trailing":    {`{"animations":{"idle":"i.png"}} {"animations":{}}`, "after"},
		"fps typo":    {`{"animations":{"idle":"i.png"},"fps":{"wlak":10}}`, "wlak"},
		"fps not > 0": {`{"animations":{"idle":"i.png"},"fps":{"walk":0}}`, "above zero"},
	} {
		_, err := parseHeroManifest([]byte(c.manifest), "/h.json")
		if err == nil {
			t.Errorf("%s accepted", name)
		} else if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s refused without saying %q: %v", name, c.says, err)
		}
	}
}

func TestSetHeroArtNamesOneHeroOnce(t *testing.T) {
	t.Cleanup(func() { SetHeroArt("") })

	for in, want := range map[string]string{
		`data\strigoi\hero\x\hero.json`:            "/data/strigoi/hero/x/hero.json",
		"data/strigoi/../strigoi/hero/x/hero.json": "/data/strigoi/hero/x/hero.json",
		"/../../etc/hero.json":                     "/etc/hero.json",
		"  ":                                       "",
	} {
		SetHeroArt(in)

		if asked, _, _ := HeroArtReport(); asked != want {
			t.Errorf("SetHeroArt(%q) asks for %q, want %q", in, asked, want)
		}
	}
}

func TestHeroArtReportKeepsTheFirstRefusal(t *testing.T) {
	t.Cleanup(func() { SetHeroArt("") })

	const x = "/data/strigoi/hero/x/hero.json"

	SetHeroArt(x)
	recordHeroArt(x, errFake)
	recordHeroArt(x, nil)

	if _, used, err := HeroArtReport(); used != "" || err != errFake {
		t.Fatalf("a later success hid a refusal: used %q err %v", used, err)
	}

	// A result for a setting since replaced is not this setting's.
	SetHeroArt("/data/strigoi/hero/y/hero.json")
	recordHeroArt(x, errFake)

	if _, used, err := HeroArtReport(); used != "" || err != nil {
		t.Fatalf("the old hero's refusal was recorded against the new one: used %q err %v", used, err)
	}

	recordHeroArt("/data/strigoi/hero/y/hero.json", nil)

	if _, used, _ := HeroArtReport(); used != "/data/strigoi/hero/y/hero.json" {
		t.Fatalf("used %q", used)
	}
}

var errFake = fakeErr("refused")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

// A hero's height is what the bar and label measure by, and it must fit its
// cell.
func TestHeroHeightFitsTheCell(t *testing.T) {
	dir, err := os.MkdirTemp("", "hero")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	if err := os.MkdirAll(filepath.Join(dir, "h"), 0o750); err != nil {
		t.Fatal(err)
	}

	write := func(name, extra string) {
		manifest := `{"name":"x","animations":{"idle":"/data/strigoi/hero/placeholder/idle.png",` +
			`"dead":"/data/strigoi/hero/placeholder/dead.png"}` + extra + `}`
		if err := os.WriteFile(filepath.Join(dir, "h", name), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("tall.json", `,"height":129`)
	write("below.json", `,"height":-1`)
	write("fits.json", `,"height":100`)
	write("none.json", ``)

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	for _, src := range []string{dir, filepath.Join("..", "..", "..")} {
		if err := asset.AddSource(src, types.AssetSourceFileSystem); err != nil {
			t.Fatal(err)
		}
	}

	f := &MapEntityFactory{asset: asset}

	if _, err := f.loadHeroBody("/h/tall.json", "hth", 0); err == nil || !strings.Contains(err.Error(), "taller") {
		t.Fatalf("a hero taller than his 128 px cell was accepted: %v", err)
	}

	if _, err := f.loadHeroBody("/h/below.json", "hth", 0); err == nil || !strings.Contains(err.Error(), "below zero") {
		t.Fatalf("a hero of height -1 was accepted: %v", err)
	}

	// The height given is the height reported, on every sheet.
	b, err := f.loadHeroBody("/h/fits.json", "hth", 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []d2enum.PlayerAnimationMode{d2enum.PlayerAnimationModeTownNeutral, d2enum.PlayerAnimationModeDead} {
		if err := b.SetMode(mode, "hth"); err != nil {
			t.Fatal(err)
		}

		if w, h := b.GetSize(); w != 96 || h != 100 {
			t.Fatalf("%v: %dx%d, want 96x100", mode, w, h)
		}
	}

	// No height: the whole cell, as before.
	b, err = f.loadHeroBody("/h/none.json", "hth", 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, h := b.GetSize(); h != 128 {
		t.Fatalf("no height given: %d, want the 128 px cell", h)
	}
}
