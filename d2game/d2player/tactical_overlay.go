package d2player

import (
	"fmt"
	"math"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// T1, the tactical layer's face (23 Sep 2026). While a paced fight runs the
// player sees WHOSE TURN it is, what he has left to spend, where he can walk,
// what he can strike, and what just happened -- every one of which he had to
// infer before, from a Diablo II screen with a calendar on top.
//
// Two parts, both drawn by ONE uncached full-viewport CustomWidget at
// foreground priority (the overhead bars' pattern):
//
//   - tile diamonds on the map: the Move's reachable tiles, each enemy's tile
//     (brighter while its pack is acting, gold while it can be struck), and his
//     own;
//   - a panel at the bottom centre, in the y 470-524 band the c-2 ruling kept
//     for the strip: the round and whose turn, three pips, the keys, and the
//     last blows or a refusal.
//
// Everything is computed in Advance (refreshTactical) and only drawn in the
// callback -- the clock strip's compute-then-draw split.

const (
	tacticalPanelX      = 180
	tacticalPanelY      = 450
	tacticalPanelWidth  = 440
	tacticalPanelHeight = 82
	tacticalLineHeight  = 15
	tacticalLines       = 5
	tacticalPadX        = 10
	tacticalPadY        = 4

	tacticalNoticeSeconds = 2.5

	// Tile diamonds, 0xRRGGBBAA. Mid-luminance like the overhead bars (their
	// §0 measurement is the precedent): a bright outline over a night map
	// reads as a bug, a dim one reads as a grid.
	tacticalMoveColor    = 0x3c8c8cff // dim teal: where the Move can go
	tacticalEnemyColor   = 0x962828ff // crimson: an enemy's tile
	tacticalActingColor  = 0xd26e1eff // orange: its pack is acting now
	tacticalStrikeColor  = 0xc8a032ff // gold: in reach, Action unspent
	tacticalPlayerColor  = 0x8c7832ff // amber: his own tile
	tacticalPanelColor   = 0x0a0a0ac8 // near-black, mostly opaque
	tacticalPanelEdge    = 0x5a4628ff
	tacticalDiamondInset = 0.08
)

// tacticalDiamond is one projected tile outline.
type tacticalDiamond struct {
	pts   [4][2]int
	color uint32
	thick bool
}

// tacticalOverlay is the HUD's cache for the fight.
type tacticalOverlay struct {
	labels   [tacticalLines]*d2ui.Label
	texts    [tacticalLines]string
	diamonds []tacticalDiamond
	visible  bool

	notice     string
	noticeLeft float64
}

func newTacticalOverlay(ui *d2ui.UIManager) *tacticalOverlay {
	t := &tacticalOverlay{}

	for i := range t.labels {
		l := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
		l.Alignment = d2ui.HorizontalAlignLeft
		t.labels[i] = l
	}

	return t
}

// TacticalNotice puts a refusal or a hint on the combat panel for a moment.
func (g *GameControls) TacticalNotice(msg string) {
	if g.hud == nil || g.hud.tactical == nil {
		return
	}

	g.hud.tactical.notice = msg
	g.hud.tactical.noticeLeft = tacticalNoticeSeconds
}

// refreshTactical recomputes the overlay from the combat model's view.
func (h *HUD) refreshTactical(elapsed float64) {
	t := h.tactical
	if t == nil {
		return
	}

	if t.noticeLeft > 0 {
		t.noticeLeft -= elapsed
		if t.noticeLeft <= 0 {
			t.notice = ""
		}
	}

	var combat *d2world.Combat
	if h.gameControls != nil {
		combat = h.gameControls.combat
	}

	if combat == nil || h.mapEngine == nil || h.mapRenderer == nil {
		t.visible = false
		return
	}

	view := combat.Tactical()
	if !view.Fighting || !view.Paced {
		t.visible = false
		t.diamonds = t.diamonds[:0]

		return
	}

	t.visible = true
	t.diamonds = h.tacticalDiamonds(view, t.diamonds[:0])

	texts := h.tacticalTexts(view, t.notice)
	for i := range texts {
		if texts[i] != t.texts[i] {
			t.texts[i] = texts[i]
			t.labels[i].SetText(texts[i])
		}
	}
}

func (h *HUD) tacticalDiamonds(view d2world.TacticalView, out []tacticalDiamond) []tacticalDiamond {
	entities := h.mapEngine.Entities()

	player, ok := entities[view.PlayerID]
	if !ok {
		return out
	}

	// THE GRID IS THE PLAYER'S OWN LATTICE, centred on his feet. Measured on
	// the first real frame: bodies stand on whole-number world coordinates,
	// which the renderer draws at a tile's TOP CORNER, so a diamond drawn on
	// the floored tile sat half a tile down-right of everything it marked. The
	// tactical Move lands on this same lattice (Game.tacticalMove), so a click
	// inside a diamond walks to its centre.
	px, py := player.GetPositionF()

	enemyAt := map[[2]int]bool{}

	for _, e := range view.Enemies {
		if e.Dead || e.Routed {
			continue
		}

		enemyAt[[2]int{int(math.Floor(e.X)), int(math.Floor(e.Y))}] = true
	}

	// The Move's reach, only while it is his turn and the Move is unspent.
	if view.Phase == "player" && !view.MoveSpent && view.MoveTiles > 0 {
		for dy := -view.MoveTiles; dy <= view.MoveTiles; dy++ {
			for dx := -view.MoveTiles; dx <= view.MoveTiles; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}

				tx, ty := px+float64(dx), py+float64(dy)
				if enemyAt[[2]int{int(math.Floor(tx)), int(math.Floor(ty))}] || h.tileBlocked(tx, ty) {
					continue
				}

				out = append(out, h.diamond(tx, ty, tacticalMoveColor, false))
			}
		}
	}

	out = append(out, h.diamond(px, py, tacticalPlayerColor, true))

	for _, e := range view.Enemies {
		if e.Dead || e.Routed {
			continue
		}

		color := uint32(tacticalEnemyColor)

		switch {
		case e.Acting:
			color = tacticalActingColor
		case view.Phase == "player" && e.Adjacent && !view.ActionSpent:
			color = tacticalStrikeColor
		}

		out = append(out, h.diamond(e.X, e.Y, color, true))
	}

	return out
}

// tileBlocked asks the map about the subtile a body standing at (x, y) would
// occupy -- the same subtile the Move's route targets.
func (h *HUD) tileBlocked(x, y float64) bool {
	const subTiles = 5

	return h.mapEngine.BlockedAt(int(math.Floor(x*subTiles)), int(math.Floor(y*subTiles)))
}

// diamond projects a one-tile cell CENTRED on (cx, cy) to the screen, inset a
// little so neighbouring cells read as separate.
func (h *HUD) diamond(cx, cy float64, color uint32, thick bool) tacticalDiamond {
	lo, hi := -0.5+tacticalDiamondInset, 0.5-tacticalDiamondInset
	corners := [4][2]float64{
		{cx + lo, cy + lo}, {cx + hi, cy + lo},
		{cx + hi, cy + hi}, {cx + lo, cy + hi},
	}

	d := tacticalDiamond{color: color, thick: thick}

	for i, c := range corners {
		sx, sy := h.mapRenderer.WorldToScreenF(c[0], c[1])
		d.pts[i] = [2]int{int(math.Floor(sx)), int(math.Floor(sy))}
	}

	return d
}

// tacticalTexts is the panel's five lines.
func (h *HUD) tacticalTexts(view d2world.TacticalView, notice string) [tacticalLines]string {
	var out [tacticalLines]string

	switch view.Phase {
	case "player":
		out[0] = d2ui.ColorTokenize(fmt.Sprintf(TacticalRoundYours, view.Round), d2ui.ColorTokenGold)
	default:
		out[0] = d2ui.ColorTokenize(fmt.Sprintf(TacticalRoundTheirs, view.Round), d2ui.ColorTokenRed)
	}

	out[1] = tacticalPip(TacticalPipMove, !view.MoveSpent) + "   " +
		tacticalPip(TacticalPipAction, !view.ActionSpent) + "   " +
		tacticalPip(TacticalPipReaction, view.ReactionAvailable)

	out[2] = d2ui.ColorTokenize(TacticalKeys, d2ui.ColorTokenGrey)

	blows := view.Blows
	lines := make([]string, 0, 2)

	for i := len(blows) - 1; i >= 0 && len(lines) < 2; i-- {
		lines = append(lines, h.blowText(blows[i], view.PlayerID))
	}

	if notice != "" {
		out[3] = d2ui.ColorTokenize(notice, d2ui.ColorTokenYellow)

		if len(lines) > 0 {
			out[4] = lines[0]
		}

		return out
	}

	for i, l := range lines {
		out[3+i] = l
	}

	return out
}

func tacticalPip(name string, ready bool) string {
	if ready {
		return d2ui.ColorTokenize(name+" "+TacticalPipReady, d2ui.ColorTokenGreen)
	}

	return d2ui.ColorTokenize(name+" "+TacticalPipSpent, d2ui.ColorTokenGrey)
}

// blowText is one blow in words: who, how well, how much, and whether it
// killed. A riposte says so first, because it is the thing he earned.
func (h *HUD) blowText(b d2world.BlowLine, playerID string) string {
	attacker, target := h.nameOf(b.Attacker, playerID), h.nameOf(b.Target, playerID)
	verb := tacticalVerb(b.Band, b.Attacker == playerID)

	text := fmt.Sprintf("%s %s %s: %d", attacker, verb, target, b.Damage)
	if b.Reaction != "" && b.Attacker == playerID {
		text = TacticalRiposte + " " + text
	}

	if b.Killed {
		text += " - " + TacticalSlain
	}

	token := d2ui.ColorTokenWhite

	switch {
	case b.Target == playerID:
		token = d2ui.ColorTokenRed
	case b.Killed:
		token = d2ui.ColorTokenGold
	}

	return d2ui.ColorTokenize(text, token)
}

func (h *HUD) nameOf(id, playerID string) string {
	if id == playerID {
		return TacticalYou
	}

	if e, ok := h.mapEngine.Entities()[id]; ok {
		if name := strings.TrimSpace(e.Label()); name != "" {
			return name
		}
	}

	return TacticalSomething
}

func tacticalVerb(band string, byPlayer bool) string {
	verbs := map[string][2]string{
		d2world.BandGraze: {"grazes", "graze"},
		d2world.BandHit:   {"hits", "hit"},
		d2world.BandCrit:  {"crits", "crit"},
	}

	v, ok := verbs[band]
	if !ok {
		v = [2]string{"strikes", "strike"}
	}

	if byPlayer {
		return v[1]
	}

	return v[0]
}

// renderTactical draws the cache. Absolute screen coordinates, as every
// uncached CustomWidget callback must.
func (h *HUD) renderTactical(target d2interface.Surface) {
	t := h.tactical
	if t == nil || !t.visible {
		return
	}

	for _, d := range t.diamonds {
		drawDiamond(target, d)
	}

	fillRect(target, tacticalPanelX-1, tacticalPanelY-1, tacticalPanelWidth+2, tacticalPanelHeight+2, tacticalPanelEdge)
	fillRect(target, tacticalPanelX, tacticalPanelY, tacticalPanelWidth, tacticalPanelHeight, tacticalPanelColor)

	for i, l := range t.labels {
		if t.texts[i] == "" {
			continue
		}

		l.SetPosition(tacticalPanelX+tacticalPadX, tacticalPanelY+tacticalPadY+i*tacticalLineHeight)
		l.Render(target)
	}
}

func drawDiamond(target d2interface.Surface, d tacticalDiamond) {
	col := d2util.Color(d.color)

	for i := 0; i < 4; i++ {
		a, b := d.pts[i], d.pts[(i+1)%4]

		target.PushTranslation(a[0], a[1])
		target.DrawLine(b[0]-a[0], b[1]-a[1], col)
		target.Pop()

		if d.thick {
			target.PushTranslation(a[0], a[1]+1)
			target.DrawLine(b[0]-a[0], b[1]-a[1], col)
			target.Pop()
		}
	}
}

// inTacticalPanel reports a screen point on the combat panel while it shows.
func (g *GameControls) inTacticalPanel(mx, my int) bool {
	if g.hud == nil || g.hud.tactical == nil || !g.hud.tactical.visible {
		return false
	}

	return mx >= tacticalPanelX && mx < tacticalPanelX+tacticalPanelWidth &&
		my >= tacticalPanelY && my < tacticalPanelY+tacticalPanelHeight
}

// tacticalEnemyAt is the enemy under a screen point in a paced fight, or "".
// It hit-tests each living participant's sprite rectangle, feet-anchored the
// way a PNG creature renders, and falls back to the enemy's own tile.
func (g *GameControls) tacticalEnemyAt(mx, my int) string {
	if g.combat == nil || g.mapRenderer == nil || g.hud == nil || g.hud.mapEngine == nil {
		return ""
	}

	view := g.combat.Tactical()
	if !view.Fighting || !view.Paced {
		return ""
	}

	wx, wy := g.mapRenderer.ScreenToWorld(mx, my)
	entities := g.hud.mapEngine.Entities()

	for _, e := range view.Enemies {
		if e.Dead || e.Routed {
			continue
		}

		// Inside the diamond drawn for it, which is centred on its feet.
		if math.Abs(wx-e.X) <= 0.5 && math.Abs(wy-e.Y) <= 0.5 {
			return e.ID
		}

		ent, ok := entities[e.ID]
		if !ok {
			continue
		}

		sx, sy := g.mapRenderer.WorldToScreenF(ent.GetPositionF())
		w, hgt := ent.GetSize()

		if float64(mx) >= sx-float64(w)/2 && float64(mx) <= sx+float64(w)/2 &&
			float64(my) >= sy-float64(hgt) && float64(my) <= sy {
			return e.ID
		}
	}

	return ""
}
