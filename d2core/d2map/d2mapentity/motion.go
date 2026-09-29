package d2mapentity

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
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
//   - an animation's frame. An action being played through (Action) restarts
//     from its first frame. B2b wrote that a creature saved while its DEATH is
//     held "plays the death again from its first frame after a load and then
//     lies as a corpse, as it would have". The B4b review measured otherwise
//     (its B1): the death ends later than it would have, and a monster slain
//     on its way in walked on through its death, so its corpse came to rest
//     further along its route. So (the B4b review fixes, BUG-75 and BUG-76)
//     a death now ends the walk where it begins (mapEntity.halt), and THE
//     GAME NEVER SAVES A HELD ACTION: Game.SaveWorld refuses while any
//     monster or villager holds one, as it refuses while his own swing
//     plays, and the load refuses a file that holds one. Restoring a held
//     action here stays exact about everything but the frame, and is
//     unit-tested; no save or load of the game's asks it to. Its callback
//     (finished / onHeldFinished) is not carried either: the game starts
//     every action with a nil one (npc_body.go), so there is nothing to lose.
//   - an NPC's patrol (Paths, path index, repetitions): only villagers
//     patrol, and villagers are rebuilt by the map (build plan §1).
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

	// Corpse is the Dead pose, held for the rest of the run. A corpse does
	// not walk.
	Corpse bool `json:"corpse,omitempty"`
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
	}

	return mo
}

// RestoreMotion puts the creature's walk and pose back. It refuses, and
// changes nothing, when the Motion is not one a creature could have had.
func (c *Creature) RestoreMotion(mo Motion) error {
	if err := mo.check(); err != nil {
		return err
	}

	mode, ok := creatureModeNamed(mo.Mode)
	if !ok {
		return fmt.Errorf("d2mapentity: motion: %q is not a creature mode", mo.Mode)
	}

	action := creatureMode("")

	if mo.Action != "" {
		if action, ok = creatureModeNamed(mo.Action); !ok {
			return fmt.Errorf("d2mapentity: motion: %q is not a creature mode", mo.Action)
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
		return err
	}

	if action != "" {
		c.held, c.heldMode = true, action
	}

	return nil
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
	}

	return mo
}

// RestoreMotion puts the NPC's walk and pose back. A Motion no NPC could have
// had is refused before anything changes; a composite that cannot load the
// saved mode is reported after the walk is set (it loaded that mode once, so
// that is a missing file, not a bad save). An NPC with no composite (a
// test's) takes the walk and has no animation to pose.
func (v *NPC) RestoreMotion(mo Motion) error {
	if err := mo.check(); err != nil {
		return err
	}

	var mode, action d2enum.MonsterAnimationMode

	var ok bool

	if v.composite != nil {
		if mode, ok = monsterModeNamed(mo.Mode); !ok {
			return fmt.Errorf("d2mapentity: motion: %q is not a monster mode", mo.Mode)
		}
	}

	if mo.Action != "" {
		if action, ok = monsterModeNamed(mo.Action); !ok {
			return fmt.Errorf("d2mapentity: motion: %q is not a monster mode", mo.Action)
		}
	}

	v.mapEntity.setMotion(mo)
	v.held, v.onHeldFinished = false, nil
	v.corpse = mo.Corpse

	if v.composite != nil {
		wanted := mode
		if mo.Action != "" {
			wanted = action
		}

		if err := v.composite.SetMode(wanted, v.composite.GetWeaponClass()); err != nil {
			return err
		}

		v.composite.SetDirection(mo.Dir)
	}

	if mo.Action != "" {
		v.held, v.heldMode = true, action
	}

	return nil
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
