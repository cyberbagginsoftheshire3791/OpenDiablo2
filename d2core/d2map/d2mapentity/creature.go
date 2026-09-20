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
}

var _ d2interface.MapEntity = (*Creature)(nil)

func newCreature(x, y int, name string, idle d2interface.Animation, direction int) (*Creature, error) {
	if idle == nil {
		return nil, fmt.Errorf("creature %q has no idle animation", name)
	}

	c := &Creature{
		mapEntity: newMapEntity(x, y),
		name:      name,
		animations: map[creatureMode]d2interface.Animation{
			creatureIdle: idle,
		},
		direction: direction,
	}
	c.mapEntity.directioner = c.rotate

	if err := c.setMode(creatureIdle); err != nil {
		return nil, err
	}

	return c, nil
}

// ID returns the creature's stable map id.
func (c *Creature) ID() string { return c.mapEntity.uuid }

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
func (c *Creature) Advance(elapsed float64) {
	if !c.corpse {
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

func (c *Creature) rotate(direction int) {
	c.direction = direction
	if c.animation != nil {
		_ = c.animation.SetDirection(direction)
	}
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
	return nil
}

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

// HarnessState exposes the same facts scripts use to inspect inherited NPCs.
func (c *Creature) HarnessState() map[string]interface{} {
	x, y := c.GetPositionF()
	return map[string]interface{}{
		"animation_mode": string(c.mode),
		"creature":       c.name,
		"direction":      c.direction,
		"moving":         c.IsMoving(),
		"world_x":        x,
		"world_y":        y,
	}
}
