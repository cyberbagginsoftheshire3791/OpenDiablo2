package d2mapentity

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// Motion is a map entity's walk and pose as the world save carries it (M4.6
// B2b, build plan §1): enough to put it back mid-stride and have it take the
// same next step.
//
// Every position is in SUB-TILES, the engine's own units, so a round trip is
// bit-exact; the harness reports the same points divided by five.
//
// NOT CARRIED, and why:
//   - the arrival callback (done) is a function. A chase's walk is handed
//     over with none (the game's chaser passes nil to SetPath). A PACED
//     FIGHT'S walk is not: Game.StepToward passes borrowPace's return, which
//     hands the creature its own speed back on arrival (corrected at the B2b
//     review; this said every monster walk had none). That one needs no
//     carrying either: saves are refused during a fight, and whenever no
//     fight runs Game.tacticalAdvance calls returnAllPace, which gives every
//     borrowed pace back and empties the table the callback reads -- so at
//     any moment a save is allowed, a walk's callback is a no-op
//     (d2gamescreen TestNoPaceIsBorrowedOutsideAFight pins that). The
//     player's click-to-move has one, and the player's walk is not restored
//     (rule 4, below).
//   - drawLayer is never set on these kinds; highlight is a render flag
//     cleared every frame.
//   - the frame of an animation that is NOT a held action (idle, walk, an
//     NPC's NU or WL, a corpse's Dead pose): it restarts from its first
//     frame. Nothing in the world reads it -- it is what is drawn -- but a
//     villager's patrol pause, which only villagers take (below).
//   - a held action's callback (finished / onHeldFinished): the game starts
//     every action with a nil one (npc_body.go), so there is nothing to lose.
//   - an NPC's patrol (Paths, path index, repetitions): only villagers
//     patrol, and villagers are rebuilt by the map (build plan §1).
//
// A HELD ACTION IS CARRIED AT ITS FRAME (M4.6 BUG-87, 29 Sep 2026): Action
// is the mode being played through, and ActionAt how far it has played -- the
// frame and the time already spent on it -- so a swing, a blow taken or a
// death saved half-played resumes at that frame and ends on the frame the
// saved one would have. The history: B2b carried the mode and not the frame,
// and wrote that a creature saved while its DEATH was held "plays the death
// again from its first frame after a load and then lies as a corpse, as it
// would have". The B4b review measured otherwise (its B1): the death ended
// later than it would have, and a monster slain on its way in walked on
// through its death. So (the B4b review fixes, BUG-75 and BUG-76) a death
// ends the walk where it begins (mapEntity.halt), and the save refused while
// any monster or villager held an action. The B4b x R1 merge measured what
// that refusal cost (BUG-87): a fight he is not in -- a monster and a
// villager -- held an action on 92-93% of its frames, so the refusal lasted
// the whole fight.
// Carrying the frame made the refusal unnecessary, and it is gone.
//
// RULE 4 IS THE PLAYER'S ALONE: "a walk you were in the middle of does not
// continue". Monsters, the risen and deployed squads keep their motion, and
// the player's is simply not restored -- which is why there is no
// MotionSnapshot on *Player. That is B4's to wire.
type Motion struct {
	Pos      [2]float64   `json:"pos"`
	Target   [2]float64   `json:"target"`
	Velocity [2]float64   `json:"velocity"`
	Path     [][2]float64 `json:"path"`
	Speed    float64      `json:"speed"`

	// Dir is the facing; Mode the animation mode it is in (an NPC's
	// composite mode, "NU", "WL"...; a creature's, "idle", "walk"...).
	Dir  int    `json:"dir"`
	Mode string `json:"mode"`

	// Action is a mode being HELD until it has played through (StartAction),
	// "" when none. A death being played becomes a corpse when it ends.
	Action string `json:"action,omitempty"`

	// ActionAt is how far the held action has played (M4.6 BUG-87): present
	// exactly when Action is. The facing is Dir, above.
	ActionAt *ActionProgress `json:"action_at,omitempty"`

	// Corpse is the Dead pose, held for the rest of the run. A corpse does
	// not walk.
	Corpse bool `json:"corpse,omitempty"`
}

// ActionProgress is how far a held action has played: the frame of its sheet
// (a creature's) or of its mode (an NPC's composite) it is on, and the time,
// in seconds, already spent on that frame -- the sub-frame progress the
// animation carries from one Advance to the next. With the mode (Action) and
// the facing (Dir) it is the whole of a held action's state:
//
//   - the play count is not carried: a held action is always on its first
//     play -- it ends on the Advance that makes the count 1 (Creature.Advance,
//     NPC.Advance) -- so it is 0 at every moment a save can see;
//   - the speed, the frame count and whether the sheet loops are the art's,
//     rebuilt with the entity;
//   - what happens when it ends is the mode's: back to idle (a creature) or
//     Neutral (an NPC), or, for a death, the Dead pose held for the rest of
//     the run.
//
// Elapsed is kept as the animation holds it, bit for bit (a JSON float64
// round-trips exactly), and may be a hair below zero: Advance subtracts whole
// frames from a sum of frame times. It is always in [d2asset.ElapsedFloor,
// the frame's length) in a running game; a file holding another time is
// refused, or, when the art has changed since the save, the action ended as it
// would have ended (ResumeMotion).
type ActionProgress struct {
	Frame   int     `json:"frame"`
	Elapsed float64 `json:"elapsed"`
}

// check refuses a Motion no running entity could have had. d2vector panics
// on a NaN or infinite position, so this must run before anything is set.
func (mo Motion) check() error {
	nums := []float64{mo.Pos[0], mo.Pos[1], mo.Target[0], mo.Target[1], mo.Velocity[0], mo.Velocity[1], mo.Speed}
	for _, p := range mo.Path {
		nums = append(nums, p[0], p[1])
	}

	for _, v := range nums {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("d2mapentity: motion: %v is not a number a walk can have", v)
		}
	}

	if mo.Speed < 0 || mo.Dir < 0 {
		return fmt.Errorf("d2mapentity: motion: speed %v, facing %d", mo.Speed, mo.Dir)
	}

	if mo.Corpse && mo.Action != "" {
		return fmt.Errorf("d2mapentity: motion: a corpse holds no action (%q)", mo.Action)
	}

	// A held action is carried at its frame, and only a held action is.
	switch {
	case mo.Action != "" && mo.ActionAt == nil:
		return fmt.Errorf("d2mapentity: motion: the held %q carries no frame", mo.Action)
	case mo.Action == "" && mo.ActionAt != nil:
		return fmt.Errorf("d2mapentity: motion: a frame (%+v) and no held action", *mo.ActionAt)
	case mo.ActionAt != nil:
		if err := mo.ActionAt.Check(); err != nil {
			return fmt.Errorf("d2mapentity: motion: the held %q %w", mo.Action, err)
		}
	}

	return nil
}

// Check refuses a point no play of ANY art could have (the BUG-87 review's
// B1, BUG-91): a negative frame, a time that is not a number, or a time below
// d2asset.ElapsedFloor, the floor no play goes under -- a file holding an
// NPC's swing at -1.0 s made its composite draw a negative frame, and the
// game panicked. A corrupt file, refused wherever it is read: here (the
// restore) and by d2save's World.Check (the file). The CEILING -- one frame's
// length, and the whole play -- is the art's, which only the restore knows
// (heldActionMisfit; d2asset's SetProgress).
func (at ActionProgress) Check() error {
	if at.Frame < 0 || math.IsNaN(at.Elapsed) || math.IsInf(at.Elapsed, 0) || at.Elapsed < d2asset.ElapsedFloor {
		return fmt.Errorf("at %+v is not a point of a play", at)
	}

	return nil
}

// motion is the walk, without the pose.
func (m *mapEntity) motion() Motion {
	mo := Motion{
		Pos:      [2]float64{m.Position.X(), m.Position.Y()},
		Target:   [2]float64{m.Target.X(), m.Target.Y()},
		Velocity: [2]float64{m.velocity.X(), m.velocity.Y()},
		Path:     make([][2]float64, 0, len(m.path)),
		Speed:    m.Speed,
	}

	for i := range m.path {
		mo.Path = append(mo.Path, [2]float64{m.path[i].X(), m.path[i].Y()})
	}

	return mo
}

// setMotion puts the walk back, field by field. It does NOT go through
// setTarget, which would call the directioner and, for an NPC, re-derive its
// mode from the walk -- the pose is the caller's to restore, exactly.
func (m *mapEntity) setMotion(mo Motion) {
	m.Position = d2vector.NewPosition(mo.Pos[0], mo.Pos[1])
	m.Target = d2vector.NewPosition(mo.Target[0], mo.Target[1])
	m.velocity = *d2vector.NewVector(mo.Velocity[0], mo.Velocity[1])
	m.Speed = mo.Speed
	m.done = nil

	m.path = make([]d2vector.Position, 0, len(mo.Path))
	for _, p := range mo.Path {
		m.path = append(m.path, d2vector.NewPosition(p[0], p[1]))
	}
}

// MotionSnapshot is the creature's walk and pose now.
func (c *Creature) MotionSnapshot() Motion {
	mo := c.mapEntity.motion()
	mo.Dir = c.direction
	mo.Mode = string(c.mode)
	mo.Corpse = c.corpse

	if c.held {
		mo.Action = string(c.heldMode)
		mo.ActionAt = &ActionProgress{}

		if c.animation != nil {
			mo.ActionAt.Frame, mo.ActionAt.Elapsed = c.animation.Progress()
		}
	}

	return mo
}

// RestoreMotion puts the creature's walk and pose back, a held action at its
// saved frame: the strict restore, the unit tests'. It refuses, and changes
// nothing, when the Motion is not one a creature could have had with this
// art -- a held action at a frame its sheet does not have, or at a time no
// frame of it lasts, among them. A load resumes with ResumeMotion.
func (c *Creature) RestoreMotion(mo Motion) error {
	_, _, err := c.restoreMotion(mo, false)

	return err
}

// ResumeMotion is the load's restore (Game.rebuildEntity, Game.rekeyNatives),
// which takes this build's art as it finds it (the BUG-87 review's B2,
// BUG-92): a held action the art no longer fits (heldActionMisfit) is ENDED,
// as it would have ended -- a bite or a blow back to idle (walking on next
// frame if it still walks), a death to the Dead pose, a corpse where the file
// puts it -- rather than refused, and ended says which and why (nil when the
// action resumed at its saved point). A corrupt one is still refused,
// changing nothing. It returns the Motion the creature then has: mo, unless
// an action was ended.
func (c *Creature) ResumeMotion(mo Motion) (restored Motion, ended *HeldActionEnded, err error) {
	return c.restoreMotion(mo, true)
}

func (c *Creature) restoreMotion(mo Motion, fit bool) (Motion, *HeldActionEnded, error) {
	if err := mo.check(); err != nil {
		return mo, nil, err
	}

	mode, ok := creatureModeNamed(mo.Mode)
	if !ok {
		return mo, nil, fmt.Errorf("d2mapentity: motion: %q is not a creature mode", mo.Mode)
	}

	action := creatureMode("")

	var ended *HeldActionEnded

	if mo.Action != "" {
		if action, ok = creatureModeNamed(mo.Action); !ok {
			return mo, nil, fmt.Errorf("d2mapentity: motion: %q is not a creature mode", mo.Action)
		}

		// The sheet the action plays on (idle's when it has none of its
		// own, as setMode will land) must have the saved frame, and a
		// frame of it must last longer than the time already spent on it.
		lands, sheet := action, c.animations[action]
		if sheet == nil {
			lands, sheet = creatureIdle, c.animations[creatureIdle]
		}

		if sheet == nil {
			return mo, nil, fmt.Errorf("d2mapentity: motion: the held %q has no sheet to play on", mo.Action)
		}

		// A LOAD ENDS AN ACTION THE ART NO LONGER FITS (B2, BUG-92): put on
		// its first frame, where any sheet has one, and ended below.
		if fit {
			if why := heldActionMisfit(mo, string(lands), sheet.GetFrameCount(), sheet.FrameLength()); why != "" {
				ended = &HeldActionEnded{Action: mo.Action, Why: why}
				mo.Mode, mo.ActionAt = string(lands), &ActionProgress{}
			}
		}

		if mo.ActionAt.Frame >= sheet.GetFrameCount() {
			return mo, nil, fmt.Errorf("d2mapentity: motion: the held %q at frame %d: its sheet has no such frame", mo.Action, mo.ActionAt.Frame)
		}

		if length := sheet.FrameLength(); !(mo.ActionAt.Elapsed < length) {
			return mo, nil, fmt.Errorf("d2mapentity: motion: the held %q at %v s into a frame: a frame of its sheet lasts %v s", mo.Action, mo.ActionAt.Elapsed, length)
		}
	}

	c.mapEntity.setMotion(mo)
	c.direction = mo.Dir
	c.held, c.heldMode, c.finished = false, "", nil
	c.corpse = mo.Corpse

	// setMode lands on idle when the wanted sheet is missing, as it did when
	// the action was first started, so the mode it lands on is the one saved.
	wanted := mode
	if action != "" {
		wanted = action
	}

	if err := c.setMode(wanted); err != nil {
		return mo, nil, err
	}

	if action != "" {
		// AT ITS SAVED FRAME, NOT ITS FIRST (BUG-87): setMode rewound the
		// sheet, reset its play count and set its facing, which puts the
		// frame back to 0; the saved frame and the time already spent on it
		// go on after, so the action ends on the frame the saved one would
		// have.
		if err := c.animation.SetProgress(mo.ActionAt.Frame, mo.ActionAt.Elapsed); err != nil {
			return mo, nil, err
		}

		c.held, c.heldMode = true, action
	}

	if ended != nil {
		// ENDED AS IT WOULD HAVE ENDED: the Advance's own end of an action
		// (finishAction), with no callback to call -- the restore hands
		// none, as the game starts every action with none.
		c.finishAction()

		return c.MotionSnapshot(), ended, nil
	}

	return mo, nil, nil
}

// HeldActionEnded is a held action a load could not resume at its saved
// point, because this build's art no longer fits it, and ended instead (the
// BUG-87 review's B2, BUG-92; Josh's default, his to overturn): the action,
// and why (heldActionMisfit). The load puts it in its report (the load's
// notes and ended_actions), so a script can tell an entity that differs from
// the saved moment BY DESIGN from one that diverged.
type HeldActionEnded struct {
	Action string `json:"action"`
	Why    string `json:"why"`
}

// String is the load note's text.
func (e HeldActionEnded) String() string {
	return fmt.Sprintf("an animation could not be resumed exactly: its held %q was ended, as it would have ended, because %s (the art changed since the save)",
		e.Action, e.Why)
}

// heldActionMisfit is why this build's art cannot resume a held action at its
// saved point, or "" when it can -- or when the point is CORRUPT, which is
// the restore's to refuse (the BUG-87 review's B1 and B2, BUG-91 and
// BUG-92). lands is the mode the action is drawn in now (a creature's sheet
// falls back to idle's when the action has none), frames and length the
// frame count and one frame's length of the art it plays on.
//
// THE LINE, since a file cannot say which art it was saved with:
//   - CORRUPT, refused (no art could have written it): a negative frame, a
//     time that is not a number or is below d2asset.ElapsedFloor
//     (Motion.check, and World.Check at the file), an action no mode is
//     named (creatureModeNamed, monsterModeNamed), and a time at or past the
//     WHOLE play of the art -- frames x length, one second for every
//     creature sheet (a PNG sheet plays in its default second whatever its
//     frame count), so no re-export brings a saved time up to it. That is
//     B1's -1.0 s, 1e17 s and 1e18 s; SetProgress refuses them too.
//   - THE ART CHANGED, ended and noted: the action is drawn in another mode
//     than the one saved (a sheet added for an action that had fallen back
//     to idle, or one taken away), at a frame the art does not have (a sheet
//     re-exported with fewer frames), or at a time at or past one frame of
//     the art but short of its whole play (a sheet re-exported with MORE
//     frames has shorter ones, and a saved time can outlast them).
func heldActionMisfit(mo Motion, lands string, frames int, length float64) string {
	if mo.Action == "" || mo.ActionAt == nil || frames <= 0 || !(length > 0) {
		return ""
	}

	at := *mo.ActionAt

	switch {
	case at.Frame < 0 || at.Elapsed < d2asset.ElapsedFloor || !(at.Elapsed < float64(frames)*length):
		return "" // corrupt: the restore refuses it
	case mo.Mode != lands:
		return fmt.Sprintf("it was saved drawn as %q, and this build draws it as %q", mo.Mode, lands)
	case at.Frame >= frames:
		return fmt.Sprintf("it was saved at frame %d, and its art has %d", at.Frame, frames)
	case !(at.Elapsed < length):
		return fmt.Sprintf("it was saved %v s into a frame, and a frame of its art lasts %v s", at.Elapsed, length)
	}

	return ""
}

func creatureModeNamed(name string) (creatureMode, bool) {
	for _, m := range []creatureMode{creatureIdle, creatureWalk, creatureAttack, creatureHit, creatureDeath, creatureDead} {
		if string(m) == name {
			return m, true
		}
	}

	return "", false
}

// MotionSnapshot is the NPC's walk and pose now.
func (v *NPC) MotionSnapshot() Motion {
	mo := v.mapEntity.motion()
	mo.Corpse = v.corpse

	if v.composite != nil {
		mo.Dir = v.composite.GetDirection()
		mo.Mode = v.composite.GetAnimationMode()
	}

	if v.held {
		mo.Action = v.heldMode.String()
		mo.ActionAt = &ActionProgress{}

		if v.composite != nil {
			mo.ActionAt.Frame, mo.ActionAt.Elapsed, _ = v.composite.Progress()
		}
	}

	return mo
}

// RestoreMotion puts the NPC's walk and pose back, a held action at its saved
// frame: the strict restore, the unit tests'. A Motion no NPC could have had
// is refused before anything changes; a composite that cannot load the saved
// mode, or whose mode has no such frame as the held action's, or no frame
// lasting the time spent on it, is reported after the walk is set (the mode's
// frame count and speed are known only once it is loaded). An NPC with no
// composite (a test's) takes the walk and has no animation to pose. A load
// resumes with ResumeMotion.
func (v *NPC) RestoreMotion(mo Motion) error {
	_, _, err := v.restoreMotion(mo, false)

	return err
}

// ResumeMotion is the load's restore (Game.rebuildEntity, Game.rekeyNatives;
// the BUG-87 review's B2, BUG-92): a held action the composite's mode no
// longer fits (heldActionMisfit: its frames, its frame length) is ENDED, as
// it would have ended -- a swing or a blow back to Neutral, a death to Dead,
// a corpse where the file puts it -- and ended says which and why; a corrupt
// one is still refused. It returns the Motion the NPC then has. (An NPC's
// frames and speed are the animdata's, not a sheet an art pass re-exports,
// so this is the rarer case; a mode the composite cannot load at all is still
// refused.)
func (v *NPC) ResumeMotion(mo Motion) (restored Motion, ended *HeldActionEnded, err error) {
	return v.restoreMotion(mo, true)
}

func (v *NPC) restoreMotion(mo Motion, fit bool) (Motion, *HeldActionEnded, error) {
	if err := mo.check(); err != nil {
		return mo, nil, err
	}

	var mode, action d2enum.MonsterAnimationMode

	var ok bool

	if v.composite != nil {
		if mode, ok = monsterModeNamed(mo.Mode); !ok {
			return mo, nil, fmt.Errorf("d2mapentity: motion: %q is not a monster mode", mo.Mode)
		}
	}

	if mo.Action != "" {
		if action, ok = monsterModeNamed(mo.Action); !ok {
			return mo, nil, fmt.Errorf("d2mapentity: motion: %q is not a monster mode", mo.Action)
		}
	}

	v.mapEntity.setMotion(mo)
	v.held, v.onHeldFinished = false, nil
	v.corpse = mo.Corpse

	var ended *HeldActionEnded

	if v.composite != nil {
		wanted := mode
		if mo.Action != "" {
			wanted = action
		}

		if err := v.composite.SetMode(wanted, v.composite.GetWeaponClass()); err != nil {
			return mo, nil, err
		}

		v.composite.SetDirection(mo.Dir)

		// A held action AT ITS SAVED FRAME, NOT ITS FIRST (BUG-87), set after
		// the facing, on its first play: SetMode short-circuits on the mode
		// the composite is already in, so its play count is set here, not
		// trusted. A load's is fitted to the mode's art first -- its frames
		// and its frame length -- and one the art no longer fits is put on
		// its first frame and ended below (B2, BUG-92). Not to its mode: a
		// composite lands on the mode asked or fails, so a saved mode other
		// than the action's is a corrupt file, which the load's read-back
		// refuses.
		if mo.Action != "" {
			if fit {
				if why := heldActionMisfit(mo, mo.Mode, v.composite.GetFrameCount(), v.composite.FrameLength()); why != "" {
					ended = &HeldActionEnded{Action: mo.Action, Why: why}
					mo.ActionAt = &ActionProgress{}
				}
			}

			if err := v.composite.SetProgress(mo.ActionAt.Frame, mo.ActionAt.Elapsed); err != nil {
				return mo, nil, err
			}
		}
	}

	if mo.Action != "" {
		v.held, v.heldMode = true, action
	}

	if ended != nil {
		// ENDED AS IT WOULD HAVE ENDED: the Advance's own end of a held
		// action (finishHeldAction), with no callback to call.
		v.finishHeldAction()

		return v.MotionSnapshot(), ended, nil
	}

	return mo, nil, nil
}

// monsterModeNamed is MonsterAnimationMode.String read backwards. "GH" is
// both GetHit and Knockback; either draws the same composite mode, and the
// first is returned.
func monsterModeNamed(name string) (d2enum.MonsterAnimationMode, bool) {
	for m := d2enum.MonsterAnimationModeDeath; m <= d2enum.MonsterAnimationModeRun; m++ {
		if m.String() == name {
			return m, true
		}
	}

	return 0, false
}
