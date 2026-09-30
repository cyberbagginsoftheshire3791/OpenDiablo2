package d2asset

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2cof"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

const (
	hardcodedFPS     = 25.0
	hardcodedDivisor = 1.0 / 256.0
	speedUnit        = hardcodedFPS * hardcodedDivisor
)

// Composite is a composite entity animation
type Composite struct {
	*AssetManager
	baseType    d2enum.ObjectType
	basePath    string
	token       string
	palettePath string
	direction   int
	equipment   [d2enum.CompositeTypeMax]string
	mode        *compositeMode
	size        *size
}

type size struct {
	Width  int
	Height int
}

// Advance moves the composite animation forward for a given elapsed time in nanoseconds.
func (c *Composite) Advance(elapsed float64) error {
	if c.mode == nil {
		return nil
	}

	c.mode.lastFrameTime += elapsed
	framesToAdd := int(c.mode.lastFrameTime / c.mode.animationSpeed)
	c.mode.lastFrameTime -= float64(framesToAdd) * c.mode.animationSpeed
	c.mode.frameIndex += framesToAdd
	c.mode.playedCount += c.mode.frameIndex / c.mode.frameCount
	c.mode.frameIndex %= c.mode.frameCount

	for _, layer := range c.mode.layers {
		if layer != nil {
			if err := layer.Advance(elapsed); err != nil {
				return err
			}
		}
	}

	return nil
}

// Render performs drawing of the Composite on the rendered d2interface.Surface.
func (c *Composite) Render(target d2interface.Surface) error {
	if c.mode == nil {
		return nil
	}

	direction := d2cof.Dir64ToCof(c.direction, c.mode.cof.NumberOfDirections)

	for _, layerIndex := range c.mode.cof.Priority[direction][c.mode.frameIndex] {
		layer := c.mode.layers[layerIndex]

		if layer != nil {
			layer.RenderFromOrigin(target, true)
		}
	}

	for _, layerIndex := range c.mode.cof.Priority[direction][c.mode.frameIndex] {
		layer := c.mode.layers[layerIndex]
		if layer != nil {
			layer.RenderFromOrigin(target, false)
		}
	}

	return nil
}

// ObjectAnimationMode returns the object animation mode
func (c *Composite) ObjectAnimationMode() d2enum.ObjectAnimationMode {
	return c.mode.animationMode.(d2enum.ObjectAnimationMode)
}

// GetAnimationMode returns the animation mode the Composite should render with.
func (c *Composite) GetAnimationMode() string {
	return c.mode.animationMode.String()
}

// GetCurrentFrame returns the frame index in the current animation mode.
func (c *Composite) GetCurrentFrame() int {
	return c.mode.frameIndex
}

// GetFrameCount returns the number of frames in the current animation mode.
func (c *Composite) GetFrameCount() int {
	return c.mode.frameCount
}

// GetWeaponClass returns the currently loaded weapon class
func (c *Composite) GetWeaponClass() string {
	return c.mode.weaponClass
}

// SetMode sets the Composite's animation mode weapon class and direction
func (c *Composite) SetMode(animationMode animationMode, weaponClass string) error {
	if c.mode != nil && c.mode.animationMode.String() == animationMode.String() && c.mode.weaponClass == weaponClass {
		return nil
	}

	mode, err := c.createMode(animationMode, weaponClass)
	if err != nil {
		return err
	}

	c.resetPlayedCount()
	c.mode = mode

	return nil
}

// Equip changes the current layer configuration
func (c *Composite) Equip(equipment *[d2enum.CompositeTypeMax]string) error {
	c.equipment = *equipment
	if c.mode == nil {
		return nil
	}

	mode, err := c.createMode(c.mode.animationMode, c.mode.weaponClass)

	if err != nil {
		return err
	}

	c.mode = mode

	return nil
}

// SetAnimSpeed sets the speed at which the Composite's animation should advance through its frames
func (c *Composite) SetAnimSpeed(speed int) {
	c.mode.animationSpeed = 1.0 / (float64(speed) * speedUnit) //nolint:gomnd // taking inverse
	for layerIdx := range c.mode.layers {
		layer := c.mode.layers[layerIdx]
		if layer != nil {
			layer.SetPlaySpeed(c.mode.animationSpeed)
		}
	}
}

// SetDirection sets the direction of the composite and its layers.
//
// A TURN KEEPS EVERY LAYER'S FRAME (M4.6 BUG-90, fixed by the BUG-87 review
// fixes with BUG-89's rule: a turn turns the art, it does not restart it). A
// DCC or PNG layer's own SetDirection puts its frame back to 0, so an NPC
// that turned during a swing drew its layers from their first frame while
// its mode's frame -- which decides when the swing ends -- ran on; a load,
// which sets each layer to the mode's frame (SetProgress), then drew another
// frame than the saved game did. Each layer's frame and the time into it are
// put back after the turn, so the layers stay in step with the mode through
// it. (A DC6 layer already kept its frame.)
func (c *Composite) SetDirection(direction int) {
	if c.mode == nil {
		return
	}

	wg := sync.WaitGroup{}

	c.direction = direction
	wg.Add(len(c.mode.layers))

	for layerIdx := range c.mode.layers {
		go func(idx int) {
			defer wg.Done()

			layer := c.mode.layers[idx]

			if layer != nil {
				frame, elapsed := layer.Progress()

				if err := layer.SetDirection(c.direction); err != nil {
					fmt.Printf("failed to set direction of layer: %d, err: %v\n", idx, err)
				}

				keepProgress(layer, frame, elapsed)
			}
		}(layerIdx)
	}

	wg.Wait()
}

// progressSetter is this package's animations' unchecked restore
// (Animation.setProgress), which every layer a composite loads has: DCC, DC6
// and PNG animations embed Animation.
type progressSetter interface {
	setProgress(frame int, elapsed float64)
}

// keepProgress puts a layer back at a frame and time it held, when it is one
// of this package's animations and its current facing has the frame.
func keepProgress(layer d2interface.Animation, frame int, elapsed float64) {
	if ps, ok := layer.(progressSetter); ok && frame >= 0 && frame < layer.GetFrameCount() {
		ps.setProgress(frame, elapsed)
	}
}

// GetDirection returns the current direction the composite is facing
func (c *Composite) GetDirection() int {
	return c.direction
}

// GetPlayedCount returns the number of times the current animation mode has completed all its distinct frames
func (c *Composite) GetPlayedCount() int {
	if c.mode == nil {
		return 0
	}

	return c.mode.playedCount
}

// SetPlayLoop turns on or off animation looping
func (c *Composite) SetPlayLoop(loop bool) {
	for layerIdx := range c.mode.layers {
		layer := c.mode.layers[layerIdx]
		if layer != nil {
			layer.SetPlayLoop(loop)
		}
	}
}

// SetSubLoop sets a loop to be between the specified frame indices
func (c *Composite) SetSubLoop(startFrame, endFrame int) {
	for layerIdx := range c.mode.layers {
		layer := c.mode.layers[layerIdx]
		if layer != nil {
			layer.SetSubLoop(startFrame, endFrame)
		}
	}
}

// SetCurrentFrame sets the current frame index of the animation
func (c *Composite) SetCurrentFrame(frame int) {
	for layerIdx := range c.mode.layers {
		layer := c.mode.layers[layerIdx]
		if layer != nil {
			if err := layer.SetCurrentFrame(frame); err != nil {
				fmt.Printf("failed to set current frame of layer: %d, err: %v\n", layerIdx, err)
			}
		}
	}
}

// Progress is how far the current mode has played: the frame it is on, the
// time already spent on that frame (the sub-frame progress Advance carries),
// and how many times it has played through. It is what decides the mode's
// next frame and when it has played through -- an NPC's held action ends on
// the first Advance that makes played 1 (M4.6 BUG-87: a held action is saved
// at its frame). (0, 0, 0) with no mode.
//
// The layers keep their own frame and time: they are what is DRAWN, not what
// decides anything. Since the BUG-87 review fixes (BUG-90) a turn keeps them
// (SetDirection), so they stay in step with the mode's frame; before, a turn
// put every DCC and PNG layer back to its first frame while the mode's frame
// ran on.
func (c *Composite) Progress() (frame int, elapsed float64, played int) {
	if c.mode == nil {
		return 0, 0, 0
	}

	return c.mode.frameIndex, c.mode.lastFrameTime, c.mode.playedCount
}

// FrameLength is how long one frame of the current mode lasts, in seconds
// (its animation speed); 0 with no mode.
func (c *Composite) FrameLength() float64 {
	if c.mode == nil {
		return 0
	}

	return c.mode.animationSpeed
}

// SetProgress puts the current mode at frame, with elapsed seconds already
// spent on it, ON ITS FIRST PLAY (played 0): Progress's restore, for a held
// action, which is always on its first play -- it ends the Advance its play
// count reaches 1 (M4.6 BUG-87). Each layer is put at the same point of its
// own sheet (the frame modulo its own count), which is where a layer that has
// advanced in step with the mode since it began stands. It refuses a frame
// the mode does not have, or no mode, changing nothing.
//
// AND A TIME NO PLAY COULD HAVE (the BUG-87 review's B1, BUG-91): below
// ElapsedFloor, at or past the mode's frame length, or not a number. Advance
// keeps the mode's time in [0, FrameLength) -- a hair below zero at most. A
// file holding -1.0 s made the first Advance take whole frames OFF the mode's
// frame, and Render indexed Priority[dir][-8]: the game panicked (the review's
// playtest, and 1e17 s or more did the same through the frame's int
// overflow). The layers are then set unchecked: this has checked the mode's
// time, and a layer's frame length (the mode's speed times its frames, over
// its frames) can be an ulp shorter than the mode's.
func (c *Composite) SetProgress(frame int, elapsed float64) error {
	if c.mode == nil {
		return errors.New("composite: no mode to set the progress of")
	}

	if frame < 0 || frame >= c.mode.frameCount {
		return fmt.Errorf("composite: frame %d of a mode of %d", frame, c.mode.frameCount)
	}

	if length := c.mode.animationSpeed; elapsed < ElapsedFloor || !(elapsed < length) {
		return fmt.Errorf("composite: %v s into a frame of %v s is no point of a play", elapsed, length)
	}

	c.mode.frameIndex = frame
	c.mode.lastFrameTime = elapsed
	c.mode.playedCount = 0

	for _, layer := range c.mode.layers {
		if layer == nil {
			continue
		}

		n := layer.GetFrameCount()
		if n <= 0 {
			continue
		}

		layer.ResetPlayedCount()
		keepProgress(layer, frame%n, elapsed)
	}

	return nil
}

func (c *Composite) resetPlayedCount() {
	if c.mode != nil {
		c.mode.playedCount = 0
	}
}

type animationMode interface {
	String() string
}

type compositeMode struct {
	cof           *d2cof.COF
	animationMode animationMode
	weaponClass   string
	playedCount   int

	layers []d2interface.Animation

	frameCount     int
	frameIndex     int
	animationSpeed float64
	lastFrameTime  float64
}

func (c *Composite) createMode(animationMode animationMode, weaponClass string) (*compositeMode, error) {
	cofPath := fmt.Sprintf("%s/%s/COF/%s%s%s.COF", c.basePath, c.token, c.token, animationMode, weaponClass)
	if exists, err := c.FileExists(cofPath); !exists {
		return nil, fmt.Errorf("composite not found at path '%s': %v", cofPath, err)
	}

	cof, err := c.LoadCOF(cofPath)
	if err != nil {
		return nil, err
	}

	animationKey := strings.ToUpper(c.token + animationMode.String() + weaponClass)

	animationData := c.Records.Animation.Data.GetRecords(animationKey)
	if len(animationData) == 0 {
		return nil, errors.New("could not find Animation data")
	}

	mode := &compositeMode{
		cof:            cof,
		animationMode:  animationMode,
		weaponClass:    weaponClass,
		layers:         make([]d2interface.Animation, d2enum.CompositeTypeMax),
		frameCount:     animationData[0].FramesPerDirection(),
		animationSpeed: 1.0 / (float64(animationData[0].Speed()) * speedUnit), //nolint:gomnd // taking inverse
	}

	for _, cofLayer := range cof.CofLayers {
		layerValue := c.equipment[cofLayer.Type]
		if layerValue == "" {
			layerValue = "lit"
		}

		drawEffect := d2enum.DrawEffectNone

		if cofLayer.Transparent {
			drawEffect = cofLayer.DrawEffect
		}

		layer, err := c.loadCompositeLayer(cofLayer.Type.String(), layerValue, animationMode.String(),
			cofLayer.WeaponClass.String(), c.palettePath, drawEffect)
		if err == nil {
			layer.SetPlaySpeed(mode.animationSpeed)
			layer.PlayForward()
			layer.SetShadow(cofLayer.Shadow != 0)

			if err := layer.SetDirection(c.direction); err != nil {
				return nil, err
			}

			mode.layers[cofLayer.Type] = layer
		}
	}

	return mode, nil
}

func (c *Composite) loadCompositeLayer(layerKey, layerValue, animationMode, weaponClass,
	palettePath string, drawEffect d2enum.DrawEffect) (d2interface.Animation, error) {
	// PNG FIRST, AND THAT ORDER IS THE RATCHET (Plan §5: D2-asset dependency only
	// ever shrinks). A layer with an original sprite beside it uses the original
	// sprite; everything else still falls back to D2's art, so the replacement of
	// one monster does not wait on the replacement of all of them.
	stem := fmt.Sprintf("%s/%s/%s/%s%s%s%s%s",
		c.basePath, c.token, layerKey, c.token, layerKey, layerValue, animationMode, weaponClass)

	animationPaths := []string{stem + ".png", stem + ".dcc", stem + ".dc6"}

	// THE LOOP USED TO `return` HERE INSTEAD OF `continue`, so the `.dc6`
	// alternative was never reached: a layer whose `.dcc` was missing failed
	// outright, and the second entry in this list had been dead since it was
	// written. Found 20 Sep 2026 while adding the `.png` entry, which would have
	// been equally dead. The error now reports every path that was tried, because
	// "animation not found" without the list is unactionable.
	var tried []string

	for idx := range animationPaths {
		tried = append(tried, animationPaths[idx])

		// A layer whose Diablo II file is gone but whose Strigoi override
		// is there (sprite_override.go) is still a layer.
		exists, err := c.FileExists(animationPaths[idx])
		if (err != nil || !exists) && c.spriteOverride(animationPaths[idx]) == "" {
			continue
		}

		animation, err := c.LoadAnimationWithEffect(animationPaths[idx], palettePath, drawEffect)
		if err == nil {
			return animation, nil
		}
	}

	return nil, fmt.Errorf("no animation for layer %s: tried %v", layerKey, tried)
}

// DirectionCount is how many facings the current mode's art actually holds.
//
// It exists for tools/spritescale, which measures D2's creature sprites so the
// art spec our own creatures are drawn to quotes real numbers instead of guessed
// ones. The count lives on the COF and the COF is unexported, so a caller
// outside this package had no way to ask -- and "how many rows does a
// spritesheet need" is the first question anyone drawing a replacement asks.
//
// Zero when no mode is set, which is the same answer GetFrameCount gives.
func (c *Composite) DirectionCount() int {
	if c.mode == nil || c.mode.cof == nil {
		return 0
	}

	return c.mode.cof.NumberOfDirections
}

// GetSize returns the size of the composite
func (c *Composite) GetSize() (w, h int) {
	c.updateSize()
	return c.size.Width, c.size.Height
}

func (c *Composite) updateSize() {
	if c.mode == nil {
		return
	}

	direction := d2cof.Dir64ToCof(c.direction, c.mode.cof.NumberOfDirections)

	biggestW, biggestH := 0, 0

	for _, layerIndex := range c.mode.cof.Priority[direction][c.mode.frameIndex] {
		layer := c.mode.layers[layerIndex]
		if layer != nil {
			w, h := layer.GetCurrentFrameSize()

			if biggestW < w {
				biggestW = w
			}

			if biggestH < h {
				biggestH = h
			}
		}
	}

	if c.size == nil {
		c.size = &size{}
	}

	c.size.Width = biggestW
	c.size.Height = biggestH
}

func baseString(baseType d2enum.ObjectType) string {
	switch baseType {
	case d2enum.ObjectTypePlayer:
		return "/data/global/chars"
	case d2enum.ObjectTypeCharacter:
		return "/data/global/monsters"
	case d2enum.ObjectTypeItem:
		return "/data/global/objects"
	default:
		return ""
	}
}
