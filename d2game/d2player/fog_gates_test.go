package d2player

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// FOG OF WAR F2: THE HUD'S FIVE LEAKS ASK ONE PREDICATE (BUG-107). With fog,
// MapRenderer.Shows says whether he sees the tile something stands on; the
// hover and talk hit test, the overhead bars, the corpse marks, the tactical
// diamonds and click-to-strike each ask it. Each test below runs the call site
// with the man's tile SEEN (the control: the site finds him) and then HIDDEN.
//
// Negative controls (1 Oct 2026, strigoi-harness-runs\wt-fog2\nc\): each gate
// removed in turn makes its test fail on the hidden case (nc-gate-*.txt).

// hidingFog is a fog that sees every tile but the ones it hides.
type hidingFog struct{ hidden map[[2]int]bool }

func (f hidingFog) FogAt(tx, ty int) (explored, visible bool) {
	return true, !f.hidden[[2]int{tx, ty}]
}

func (hidingFog) MemoryLook() (level, saturation float64) { return 0.45, 0.25 }

func hiding(tiles ...[2]int) hidingFog {
	h := map[[2]int]bool{}
	for _, t := range tiles {
		h[t] = true
	}

	return hidingFog{hidden: h}
}

var siteTile = [2]int{siteTileX, siteTileY}

// TestTheHoverAsksTheFog: a man on ground he does not see is not hovered --
// so not named, not talked to, and not highlighted (the F1 review's C3).
func TestTheHoverAsksTheFog(t *testing.T) {
	mr, engine, man, fx, fy := siteAt05(t)
	h := &HUD{mapEngine: engine, mapRenderer: mr}

	mr.SetFogSampler(hiding())

	if got := h.hoveredEntity(fx, fy-10); got != man {
		t.Fatalf("the control: on a tile he sees, the man is not hovered (%v)", got)
	}

	mr.SetFogSampler(hiding(siteTile))

	if got := h.hoveredEntity(fx, fy-10); got != nil {
		t.Fatalf("the man on a tile he does not see is hovered (%s): BUG-107's hover leak", got.ID())
	}

	if got := h.hoveredEntityWhere(fx, fy-10, nil); got != nil {
		t.Fatalf("the talk panel's hit test finds the hidden man (%s)", got.ID())
	}
}

// TestNoBarOverABodyHeDoesNotSee: the overhead bar is not projected over a
// hidden body.
func TestNoBarOverABodyHeDoesNotSee(t *testing.T) {
	mr, engine, _, _, _ := siteAt05(t)
	h := &HUD{mapEngine: engine, mapRenderer: mr, bars: oneBar{}}

	mr.SetFogSampler(hiding())
	h.refreshOverheadBars()

	if len(h.overheadBars) != 1 {
		t.Fatalf("the control: %d bars over a man he sees, want 1", len(h.overheadBars))
	}

	mr.SetFogSampler(hiding(siteTile))
	h.refreshOverheadBars()

	if len(h.overheadBars) != 0 {
		t.Fatalf("%d bars over a man on a tile he does not see: BUG-107's bar leak", len(h.overheadBars))
	}
}

type marksAt struct {
	CorpseHolder

	marks []CorpseMark
}

func (m marksAt) CorpseMarks() []CorpseMark { return m.marks }

// TestNoCorpseMarkInTheFog: a corpse mark on ground he does not see is not
// drawn; one beside it, on ground he sees, is.
func TestNoCorpseMarkInTheFog(t *testing.T) {
	mr, engine, _, _, _ := siteAt05(t)
	marks := marksAt{marks: []CorpseMark{
		{X: siteTileX + 0.5, Y: siteTileY + 0.5, Open: true},
		{X: siteTileX + 2.5, Y: siteTileY + 0.5, Grave: true},
	}}
	h := &HUD{mapEngine: engine, mapRenderer: mr, gameControls: &GameControls{corpseHolder: marks}}

	mr.SetFogSampler(hiding())

	if got := len(h.corpseMarksShown()); got != 2 {
		t.Fatalf("the control: %d corpse marks shown on ground he sees, want 2", got)
	}

	mr.SetFogSampler(hiding(siteTile))

	got := h.corpseMarksShown()
	if len(got) != 1 || got[0].X != siteTileX+2.5 {
		t.Fatalf("with the first body's tile hidden the marks shown are %+v; want only the one he sees", got)
	}
}

// fightAt05 is the site with a second man, the wolf, two tiles east of the
// first (the player), and his fight's view with the wolf in it.
func fightAt05(t *testing.T) (*HUD, *GameControls, d2world.TacticalView, int, int, [2]int) {
	t.Helper()

	mr, engine, _, _, _ := siteAt05(t)
	wolf := &standingMan{id: "wolf", pos: d2vector.NewPositionTile(siteTileX+2, siteTileY)}
	engine.AddEntity(wolf)

	h := &HUD{mapEngine: engine, mapRenderer: mr}
	g := &GameControls{mapRenderer: mr, hud: h}
	view := d2world.TacticalView{
		Fighting: true, Paced: true, PlayerID: "man",
		Enemies: []d2world.TacticalEnemy{{ID: "wolf", X: siteTileX + 2, Y: siteTileY}},
	}
	wx, wy := mr.WorldToScreen(siteTileX+2, siteTileY)

	return h, g, view, wx, wy, [2]int{siteTileX + 2, siteTileY}
}

// TestNoTacticalDiamondOnAHiddenEnemy: the enemy's diamond is not drawn
// where he does not see it. (Under fog an enemy of HIS fight is a contact --
// Q4 -- so in the game this holds while the fight lasts; the gate is the same
// predicate.)
func TestNoTacticalDiamondOnAHiddenEnemy(t *testing.T) {
	h, _, view, _, _, wolfTile := fightAt05(t)

	h.mapRenderer.SetFogSampler(hiding())

	seen := len(h.tacticalDiamonds(view, nil))
	if seen != 2 {
		t.Fatalf("the control: %d diamonds with the wolf seen, want 2 (his and the wolf's)", seen)
	}

	h.mapRenderer.SetFogSampler(hiding(wolfTile))

	if got := len(h.tacticalDiamonds(view, nil)); got != 1 {
		t.Fatalf("%d diamonds with the wolf on a tile he does not see, want 1 (his own): BUG-107's diamond leak", got)
	}
}

// TestNoStrikeAtAnEnemyHeDoesNotSee: click-to-strike finds no target where
// he does not see one.
func TestNoStrikeAtAnEnemyHeDoesNotSee(t *testing.T) {
	h, g, view, wx, wy, wolfTile := fightAt05(t)

	h.mapRenderer.SetFogSampler(hiding())

	if got := g.enemyUnder(view, wx, wy-10); got != "wolf" {
		t.Fatalf("the control: a click on the wolf he sees strikes %q", got)
	}

	h.mapRenderer.SetFogSampler(hiding(wolfTile))

	if got := g.enemyUnder(view, wx, wy-10); got != "" {
		t.Fatalf("a click on the wolf he does not see strikes %q: BUG-107's click leak", got)
	}
}

// TestNoFogShowsEverything: without fog every gate is open, as on master.
func TestNoFogShowsEverything(t *testing.T) {
	h, g, view, wx, wy, _ := fightAt05(t)

	if !h.mapRenderer.Shows(-5, -5) || len(h.tacticalDiamonds(view, nil)) != 2 || g.enemyUnder(view, wx, wy-10) != "wolf" {
		t.Fatal("with no fog sampler a gate is shut")
	}
}

// TestAnEnemyWithNoBodyAsksTheFogWhereTheFightHasIt (the review's C4, its M8):
// an enemy of his fight with no entity on the map is gated by the tile the
// fight has it on.
//
// Negative control: return true from enemyShown's fallback (the reviewer's
// M8) and this fails, "an enemy with no body on a hidden tile is shown"
// (nc-r-enemy-fallback-true.txt).
func TestAnEnemyWithNoBodyAsksTheFogWhereTheFightHasIt(t *testing.T) {
	h, _, _, _, _, _ := fightAt05(t)
	ghost := d2world.TacticalEnemy{ID: "ghost", X: siteTileX + 5.5, Y: siteTileY + 0.5}
	entities := h.mapEngine.Entities()

	h.mapRenderer.SetFogSampler(hiding())

	if !h.enemyShown(entities, ghost) {
		t.Fatal("the control: an enemy with no body on a tile he sees is hidden")
	}

	h.mapRenderer.SetFogSampler(hiding([2]int{siteTileX + 5, siteTileY}))

	if h.enemyShown(entities, ghost) {
		t.Fatal("an enemy with no body on a hidden tile is shown")
	}
}

// TestTheBarsAreInTheDigestsProcessPart (the review's B6): which bars are
// drawn depends, with fog on, on what he sees -- presentation -- so the bars
// are in the digest's process part (without their screen rects) and never in
// the world part, which fog must not reach.
//
// Negative control: leave bars in the world part (fd3c2e6d) and this fails,
// "the bars are in the digest's world part" (nc-r-bars-in-world.txt).
func TestTheBarsAreInTheDigestsProcessPart(t *testing.T) {
	world, process := splitUIDigest(map[string]interface{}{
		"bars":     []map[string]interface{}{{"entity": "wolf", "x": 3, "y": 4, "w": 30, "h": 4, "fill": 0.5}},
		"free_cam": false,
	})

	if _, ok := world["bars"]; ok {
		t.Fatal("the bars are in the digest's world part")
	}

	rows, _ := process["bars"].([]map[string]interface{})
	if len(rows) != 1 || rows[0]["entity"] != "wolf" || rows[0]["fill"] != 0.5 {
		t.Fatalf("the digest's process part has bars %v; want the wolf's", process["bars"])
	}

	if _, ok := rows[0]["x"]; ok {
		t.Fatal("a bar's screen rect is in the digest")
	}

	if _, ok := world["free_cam"]; !ok {
		t.Fatal("the control: free_cam, world state, left the world part")
	}
}
