package d2mapentity

import (
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// creatureMode is Strigoi's animation vocabulary. It deliberately does not use
// D2's two-letter composite modes: a creature is one rendered sheet, not a COF
// assembling equipment layers.
type creatureMode string

const (
	creatureIdle   creatureMode = "idle"
	creatureWalk   creatureMode = "walk"
	creatureAttack creatureMode = "attack"
	creatureHit    creatureMode = "hit"
	creatureDeath  creatureMode = "death"
	creatureDead   creatureMode = "dead"
)

// Creature is a map entity drawn from ordinary PNG animations. The first asset
// may contain only an idle still; missing modes fall back to idle so art can be
// tested in the game before the full animation set exists.
type Creature struct {
	mapEntity
	name       string
	animations map[creatureMode]d2interface.Animation
	animation  d2interface.Animation
	mode       creatureMode
	direction  int
	held       bool
	heldMode   creatureMode
	finished   func()
	corpse     bool

	// sheets is the path each mode was loaded from (M5.1b), so the harness
	// can say which art a creature is drawn from rather than only its name.
	sheets map[creatureMode]string

	// creatureID is the bestiary entry the creature was built from (M4.6
	// B3): the world save's entity list names it, and a load rebuilds the
	// creature from that entry. The name alone is not a key -- it is a label,
	// and two entries may share one.
	creatureID string
}

var _ d2interface.MapEntity = (*Creature)(nil)

// newCreature builds a creature; an empty id means a fresh one.
func newCreature(x, y int, id, name string, animations map[creatureMode]d2interface.Animation, direction int) (*Creature, error) {
	if animations[creatureIdle] == nil {
		return nil, fmt.Errorf("creature %q has no idle animation", name)
	}

	c := &Creature{
		mapEntity:  newMapEntityWithID(x, y, id),
		name:       name,
		animations: animations,
		direction:  direction,
	}
	c.mapEntity.directioner = c.rotate

	if err := c.setMode(creatureIdle); err != nil {
		return nil, err
	}

	return c, nil
}

// ID returns the creature's stable map id.
func (c *Creature) ID() string { return c.mapEntity.uuid }

// SetCreatureID records the bestiary entry the creature was built from. The
// game calls it beside every NewCreature (the spawner's and the terminal's);
// NewCreature does not take it, because the factory knows art and a stand-in,
// not the bestiary.
func (c *Creature) SetCreatureID(id string) { c.creatureID = id }

// CreatureID is the bestiary entry the creature was built from, or "" for one
// built without it (a unit test's).
func (c *Creature) CreatureID() string { return c.creatureID }

// Label returns the authored creature name.
func (c *Creature) Label() string { return c.name }

// Selectable keeps the creature available to map hit-testing and HUD systems.
func (c *Creature) Selectable() bool { return c.name != "" }

// GetPosition returns the creature's subtile position for the map renderer.
func (c *Creature) GetPosition() d2vector.Position { return c.mapEntity.Position }

// GetVelocity returns its current movement vector.
func (c *Creature) GetVelocity() d2vector.Vector { return c.mapEntity.velocity }

// GetSize returns the active sprite frame size.
func (c *Creature) GetSize() (width, height int) {
	if c.animation == nil {
		return minHitboxSize, minHitboxSize
	}

	return c.animation.GetCurrentFrameSize()
}

// Render anchors the bottom of a PNG frame on the entity's map position.
func (c *Creature) Render(target d2interface.Surface) {
	if c.animation == nil {
		return
	}

	renderOffset := c.Position.RenderOffset()
	target.PushTranslation(
		int((renderOffset.X()-renderOffset.Y())*magicOffsetScalarY),
		int(((renderOffset.X()+renderOffset.Y())*magicOffsetScalarX)-magicOffsetX),
	)
	defer target.Pop()

	if c.highlight {
		target.PushBrightness(highlightBrightness)
		defer target.Pop()
		c.highlight = false
	}

	c.animation.RenderFromOrigin(target, false)
}

// Advance moves the creature, chooses idle or walk, and advances its sprite.
//
// A CORPSE DOES NOT WALK, AND NEITHER DOES A MONSTER PLAYING ITS DEATH (the
// B4b review fixes, BUG-75). StartAction ends the walk when the death begins
// (halt); this guard is what keeps it ended if anything hands a dying
// monster a route before its corpse lies, so the corpse lies where the death
// began -- where Combat.fallCorpse recorded the fall.
func (c *Creature) Advance(elapsed float64) {
	if !c.corpse && !c.dying() {
		c.Step(elapsed)
	}

	if c.animation == nil {
		return
	}

	if err := c.animation.Advance(elapsed); err != nil {
		return
	}

	if c.corpse {
		return
	}

	if c.held && c.animation.GetPlayedCount() >= 1 {
		c.finishAction()
		return
	}
	if c.held {
		return
	}

	wanted := creatureIdle
	if c.IsMoving() {
		wanted = creatureWalk
	}
	if c.mode != wanted {
		_ = c.setMode(wanted)
	}
}

func (c *Creature) setMode(mode creatureMode) error {
	animation := c.animations[mode]
	actual := mode
	if animation == nil {
		animation = c.animations[creatureIdle]
		actual = creatureIdle
	}
	if animation == nil {
		return fmt.Errorf("creature %q has no animation for %q and no idle fallback", c.name, mode)
	}

	c.animation = animation
	c.mode = actual
	c.animation.Rewind()
	c.animation.ResetPlayedCount()
	return c.animation.SetDirection(c.direction)
}

// rotate is the creature's directioner, which mapEntity.setTarget calls at
// every waypoint of every route, whether the facing changes or not.
//
// A TURN TURNS THE SHEET; IT DOES NOT RESTART IT (M4.6 BUG-89, fixed by the
// BUG-87 review fixes -- Josh's default (a)). The sheet's SetDirection puts
// its frame back to 0, so a creature biting or flinching while it walked
// started the action again at every waypoint: a feral dog's bite held 61
// ticks at 1/60 s standing and 121 when it turned 20 ticks in. Now a waypoint
// on the same facing is no turn at all (the guard NPC.rotate has always had),
// and a real turn keeps the frame and the time into it, as an NPC's composite
// mode does (and, since BUG-90, its layers). An action ends when its sheet
// has played through once, turned or not. Nothing the fight resolves reads
// it: blows are the combat model's (Game.Animate passes no callback), and a
// death does not turn (halt) -- only how long a swing or a flinch is drawn.
func (c *Creature) rotate(direction int) {
	if direction == c.direction {
		return
	}

	c.direction = direction
	if c.animation == nil {
		return
	}

	frame, elapsed := c.animation.Progress()
	_ = c.animation.SetDirection(direction)

	// Every facing of a sheet has the same frames, and elapsed is the time
	// Advance left, so this is a restore of the sheet's own point.
	_ = c.animation.SetProgress(frame, elapsed)
}

func creatureModeForMonsterMode(mode d2enum.MonsterAnimationMode) creatureMode {
	switch mode {
	case d2enum.MonsterAnimationModeWalk:
		return creatureWalk
	case d2enum.MonsterAnimationModeAttack1, d2enum.MonsterAnimationModeAttack2:
		return creatureAttack
	case d2enum.MonsterAnimationModeGetHit, d2enum.MonsterAnimationModeBlock:
		return creatureHit
	case d2enum.MonsterAnimationModeDeath:
		return creatureDeath
	case d2enum.MonsterAnimationModeDead:
		return creatureDead
	default:
		return creatureIdle
	}
}

// StartAction adapts the combat resolver's existing monster acts to Strigoi's
// mode names. A missing first-pass animation uses the idle still and still lets
// combat finish; art availability never stalls simulation.
func (c *Creature) StartAction(mode d2enum.MonsterAnimationMode, onFinished func()) error {
	if c.corpse {
		return nil
	}

	wanted := creatureModeForMonsterMode(mode)
	if err := c.setMode(wanted); err != nil {
		return err
	}

	c.held = true
	c.heldMode = wanted
	c.finished = onFinished

	// A death ends the walk where it begins (BUG-75; mapEntity.halt).
	if wanted == creatureDeath {
		c.halt()
	}

	return nil
}

// dying is a death being played: held, and not yet a corpse.
func (c *Creature) dying() bool { return c.held && c.heldMode == creatureDeath }

func (c *Creature) finishAction() {
	done, mode := c.finished, c.heldMode
	c.held = false
	c.finished = nil

	defer func() {
		if done != nil {
			done()
		}
	}()

	if mode != creatureDeath {
		_ = c.setMode(creatureIdle)
		return
	}

	_ = c.setMode(creatureDead)
	c.corpse = true
	c.StopMoving()
}

// HarnessState exposes the same facts scripts use to inspect inherited NPCs,
// and since M5.1b the sheet the current mode is drawn from (a mode with no
// sheet of its own falls back to idle, and reports idle's) and the speed it
// walks at.
func (c *Creature) HarnessState() map[string]interface{} {
	x, y := c.GetPositionF()
	state := map[string]interface{}{
		"animation_mode": string(c.mode),
		"creature":       c.name,
		"creature_id":    c.creatureID, // M4.6 B3: what the world save names it by
		"direction":      c.direction,
		"moving":         c.IsMoving(),
		"sheet":          c.sheets[c.mode],
		"speed":          c.Speed,
		"world_x":        x,
		"world_y":        y,

		// The pose flags the world save carries (the B4b review fixes,
		// BUG-82): the action held until it has played through ("" when
		// none), and the Dead pose. animation_mode alone cannot tell a held
		// attack from one a missing sheet fell back from, nor a corpse from a
		// monster posed dead.
		"held":   string(c.heldMode),
		"corpse": c.corpse,
	}

	if !c.held {
		state["held"] = ""
	}

	c.harnessHeldAt(state)
	c.harnessMotion(state)

	return state
}
