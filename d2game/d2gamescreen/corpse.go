package d2gamescreen

import (
	"errors"
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// M4.7 step 1 (23 Sep 2026): the game screen's half of the corpse machine --
// Night 1's dead, the stake, and the marks that show where bodies lie.
// See `M4.7 - The Corpse Machine Build-Shape Note.md`; this builds step 1 on
// the note's recommended options (Q1a: a beast is carrion and never rises;
// Q2a: a placeholder field of dead near the start), each flagged for Josh.

const (
	stakeItem    = "stake"
	stakeReach   = 1.5 // [DIAL] tiles: "at your feet"
	stakeMinutes = 5.0 // [DIAL]
	placedDead   = 4   // [DIAL] Night 1's dead (Q2a PLACEHOLDER)

	// markLight is how lit a body\'s tile must be for its mark to show, and
	// markNear how close he must stand to see one in the dark [DIALs].
	markLight = 0.3
	markNear  = 2.0
)

// placedDeadOffsets are where Night 1's dead lie around his first tile
// [DIAL, PLACEHOLDER until the map is authored]; the first walkable ones win.
var placedDeadOffsets = [][2]float64{
	{2, 1}, {-1, 3}, {3, -1}, {-2, -2}, {1, -3}, {4, 2}, {-3, 1}, {2, 4}, {-2, -4}, {4, -3},
}

var (
	errStakeNoBody  = errors.New("no man's body at your feet")
	errStakeCarcass = errors.New("a beast's carcass does not rise")
	errStakeNone    = errors.New("no stake -- whittle one from a branch")
)

// walkable reports a whole tile a body can lie on and he can reach -- the
// gameSpawner's own test: the tile exists and its centre does not block.
func (v *Game) walkable(tileX, tileY float64) bool {
	if v.gameClient == nil || v.gameClient.MapEngine == nil {
		return false
	}

	engine := v.gameClient.MapEngine
	if !engine.TileExists(int(tileX), int(tileY)) {
		return false
	}

	flags := engine.SubTileAt(int(tileX*subTilesPerTile)+2, int(tileY*subTilesPerTile)+2)

	return flags != nil && !flags.BlockWalk
}

// placeTheDead lays Night 1's dead near where he enters (M4.7 Q2a). The world
// begins again at the 17 June dawn every session, so they lie again every
// session -- the 12 Sep "load last save means a new dawn".
func (v *Game) placeTheDead() {
	if v.corpses == nil || v.localPlayer == nil {
		return
	}

	px, py := v.localPlayer.GetPositionF()
	placed := 0

	for i, off := range placedDeadOffsets {
		if placed >= placedDead {
			break
		}

		tx, ty := math.Floor(px+off[0]), math.Floor(py+off[1])
		if !v.walkable(tx, ty) {
			continue
		}

		v.corpses.FallHuman(fmt.Sprintf("dead:%d", i+1), tx+0.5, ty+0.5)
		placed++
	}
}

// Stake drives a stake through the open body of a man at his feet: five
// minutes' work, one stake spent, the body Closed -- it will never rise (S1
// §6.2; R1 §2, the stake pins the corpse). Refused in a fight, dead, talking,
// without a stake, and for a beast's carcass (Q1a).
func (v *Game) Stake() error {
	err := v.stake()

	if v.gameControls != nil {
		text := d2player.StakeDone
		if err != nil {
			text = fmt.Sprintf(d2player.StakeRefused, err)
		}

		v.gameControls.SetZoneChangeText(text)
		v.gameControls.ShowZoneChangeText()
		v.gameControls.HideZoneChangeTextAfter(levelNoticeSeconds)
	}

	return err
}

func (v *Game) stake() error {
	switch {
	case v.corpses == nil || v.kit == nil || v.localPlayer == nil:
		return errStakeNoBody
	case v.died || !v.alive():
		return errCraftDead
	case v.talk != nil || v.choosingLoadout:
		return errForageBusy
	case v.inFight():
		return errForageFight
	}

	px, py := v.localPlayer.GetPositionF()

	body := v.corpses.Nearest(px, py, stakeReach, func(b *d2world.Corpse) bool {
		return b.State == d2world.CorpseFresh && b.Class == d2world.CorpseHuman
	})
	if body == nil {
		if v.corpses.Nearest(px, py, stakeReach, func(b *d2world.Corpse) bool { return b.State == d2world.CorpseFresh }) != nil {
			return errStakeCarcass
		}

		return errStakeNoBody
	}

	if !v.kit.Carries(stakeItem) {
		return errStakeNone
	}

	v.spendMinutes(stakeMinutes, 0, d2world.ActivityLabour)

	// Caught at it: the work is not done and the stake not spent.
	if v.inFight() {
		return errCaughtAtWork
	}

	if !v.alive() {
		return errCraftDead
	}

	// The stake is spent first: a body is never closed for nothing.
	if !v.kit.Use(stakeItem) {
		return errStakeNone
	}

	v.corpses.Close(body.ID)
	v.saveKit()

	return nil
}

// CorpseMarks are the bodies as the HUD marks them (PLACEHOLDER glyphs until
// there is art for the dead). A body in the dark is not marked unless he
// stands over it: the marks must not light what the night hides (R2 §1).
func (v *Game) CorpseMarks() []d2player.CorpseMark {
	if v.corpses == nil {
		return nil
	}

	all := v.corpses.All()
	marks := make([]d2player.CorpseMark, 0, len(all))

	for _, b := range all {
		if !v.canSeeBody(b.X, b.Y) {
			continue
		}

		marks = append(marks, d2player.CorpseMark{
			X: b.X, Y: b.Y,
			Open:  b.State == d2world.CorpseFresh,
			Human: b.Class == d2world.CorpseHuman,
		})
	}

	return marks
}

// canSeeBody reports a body lit enough to see, or at his feet.
func (v *Game) canSeeBody(x, y float64) bool {
	if v.light == nil {
		return true
	}

	level := v.light.Level(int(math.Floor(x)), int(math.Floor(y)))
	dist := math.Inf(1)

	if v.localPlayer != nil {
		px, py := v.localPlayer.GetPositionF()
		dist = math.Hypot(x-px, y-py)
	}

	return markVisible(level, dist)
}

// markVisible is the rule on its own: lit to markLight, or within markNear.
func markVisible(level, dist float64) bool {
	return level >= markLight || dist <= markNear
}
