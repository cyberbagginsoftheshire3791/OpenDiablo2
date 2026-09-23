package d2gamescreen

import (
	"errors"
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// M4.7 (23 Sep 2026): the game screen's half of the corpse machine --
// Night 1's dead, the stake, the hasty grave, and the marks that show where
// bodies lie. See `M4.7 - The Corpse Machine Build-Shape Note.md`; this builds
// steps 1-2 on the note's recommended options (Q1a: a beast is carrion and
// never rises; Q2a: a placeholder field of dead near the start; Q3a: stake and
// hasty grave), each flagged for Josh.

const (
	stakeItem    = "stake"
	stakeReach   = 1.5  // [DIAL] tiles: "at your feet"
	stakeMinutes = 5.0  // [DIAL]
	digMinutes   = 30.0 // [DIAL] a hasty grave (M4.7 note step 2)
	placedDead   = 4    // [DIAL] Night 1's dead (Q2a PLACEHOLDER)

	// risingSeedOffset keeps the rising's draws off the spawn tables' stream.
	risingSeedOffset = 4707

	// markLight is how lit a body's tile must be for its mark to show, and
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
	errDigNoBody    = errors.New("no open body at your feet")
	errBodyGone     = errors.New("it is gone -- it rose while you worked")
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

// corpseNotice shows a verb's outcome in the zone-change line.
func (v *Game) corpseNotice(done, refused string, err error) {
	if v.gameControls == nil {
		return
	}

	text := done
	if err != nil {
		text = fmt.Sprintf(refused, err)
	}

	v.gameControls.SetZoneChangeText(text)
	v.gameControls.ShowZoneChangeText()
	v.gameControls.HideZoneChangeTextAfter(levelNoticeSeconds)
}

// Stake drives a stake through the body of a man at his feet, open or in a
// hasty grave: five minutes' work, one stake spent, the body Closed -- it
// will never rise (S1 §6.2; R1 §2, the stake pins the corpse in the grave).
// Refused in a fight, dead, talking, without a stake, and for a beast's
// carcass (Q1a).
func (v *Game) Stake() error {
	err := v.stake()
	v.corpseNotice(d2player.StakeDone, d2player.StakeRefused, err)

	return err
}

// Dig lays an open body in a hasty grave: half an hour head-down with no
// tool (PLACEHOLDER: the note's digging tool waits on M6), and the body is no
// longer carrion -- but a man's still rolls to rise, at a quarter the odds
// (S1 §6.2's "hasty vs closed").
func (v *Game) Dig() error {
	err := v.dig()
	v.corpseNotice(d2player.DigDone, d2player.DigRefused, err)

	return err
}

// fieldWorkRefused is what stops any work on a body.
func (v *Game) fieldWorkRefused() error {
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

	return nil
}

// work spends a verb's minutes as labour and reports why it did not finish.
func (v *Game) work(minutes float64) error {
	v.spendMinutes(minutes, 0, d2world.ActivityLabour)

	// Caught at it: the work is not done.
	if v.inFight() {
		return errCaughtAtWork
	}

	if !v.alive() {
		return errCraftDead
	}

	return nil
}

func (v *Game) stake() error {
	if err := v.fieldWorkRefused(); err != nil {
		return err
	}

	px, py := v.localPlayer.GetPositionF()

	body := v.corpses.Nearest(px, py, stakeReach, func(b *d2world.Corpse) bool { return b.Door() })
	if body == nil {
		if v.corpses.Nearest(px, py, stakeReach, func(b *d2world.Corpse) bool { return b.State == d2world.CorpseFresh }) != nil {
			return errStakeCarcass
		}

		return errStakeNoBody
	}

	if !v.kit.Carries(stakeItem) {
		return errStakeNone
	}

	if err := v.work(stakeMinutes); err != nil {
		return err
	}

	// The world ran while he worked: a band can turn in those minutes, and
	// the body he knelt at can rise under his hands. Then there is nothing
	// to stake, and the stake stays in the pack.
	if now, ok := v.corpses.Get(body.ID); !ok || !now.Door() {
		return errBodyGone
	}

	// The stake is spent first: a body is never closed for nothing.
	if !v.kit.Use(stakeItem) {
		return errStakeNone
	}

	if !v.corpses.Close(body.ID) {
		return errBodyGone
	}

	if v.rising != nil {
		v.rising.Rite()
	}

	v.saveKit()

	return nil
}

func (v *Game) dig() error {
	if err := v.fieldWorkRefused(); err != nil {
		return err
	}

	px, py := v.localPlayer.GetPositionF()

	body := v.corpses.Nearest(px, py, stakeReach, func(b *d2world.Corpse) bool { return b.State == d2world.CorpseFresh })
	if body == nil {
		return errDigNoBody
	}

	if err := v.work(digMinutes); err != nil {
		return err
	}

	// Half an hour is long enough for a band to turn: the body may be gone.
	if !v.corpses.Bury(body.ID) {
		return errBodyGone
	}

	return nil
}

// CorpseMarks are the bodies as the HUD marks them (PLACEHOLDER glyphs until
// there is art for the dead). A body in the dark is not marked unless he
// stands over it: the marks must not light what the night hides (R2 §1). A
// risen body is not marked at all -- the place is empty.
func (v *Game) CorpseMarks() []d2player.CorpseMark {
	if v.corpses == nil {
		return nil
	}

	all := v.corpses.All()
	marks := make([]d2player.CorpseMark, 0, len(all))

	for _, b := range all {
		if b.State == d2world.CorpseRisen || !v.canSeeBody(b.X, b.Y) {
			continue
		}

		marks = append(marks, d2player.CorpseMark{
			X: b.X, Y: b.Y,
			Open:  b.State == d2world.CorpseFresh,
			Grave: b.State == d2world.CorpseHasty,
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

// raiseTheDead stands a risen body up where it lay (M4.7 step 3; Q5a: as
// himself -- a man's body and a man's art, so he looks alive until he acts).
// It names the member he walks as, or "" if he could not stand.
func (v *Game) raiseTheDead(b d2world.Corpse) string {
	if v.spawns == nil {
		return ""
	}

	return v.spawns.Raise(b.X, b.Y)
}

// firstLight is R2 §2A: the dead break off. A fight they are in loses them
// (and ends "dawn" if nothing else is in it), and each risen man lies down
// where he stands, open again -- a door that rolls again tomorrow night.
// (Q7 PLACEHOLDER: the note recommends he stand CERTAINLY in the next deep
// night; until Downed exists, step 3 leaves him a door at P.)
func (v *Game) firstLight() {
	if v.spawns == nil {
		return
	}

	if v.combat != nil {
		v.combat.BreakOff(func(id string) bool {
			p, ok := v.spawns.ProfileOf(id)

			return ok && p.Dead
		})
	}

	v.spawns.LayDownDead()
}
