package d2mapentity

import (
	"math/rand"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2path"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// NPC is a passive complex entity with which the player can interact.
// For example, Deckard Cain.
type NPC struct {
	mapEntity
	Paths         []d2path.Path
	name          string
	composite     *d2asset.Composite
	action        int
	path          int
	repetitions   int
	rng           *rand.Rand // per-entity behaviour RNG, seeded by the factory (P3 E4)
	monstatRecord *d2records.MonStatRecord
	monstatEx     *d2records.MonStat2Record
	HasPaths      bool
	isDone        bool

	// held is a mode set from OUTSIDE that must survive until it has played
	// through, and heldMode is which one. StartAction sets both; Advance
	// clears them when the composite reports a full play.
	//
	// WITHOUT THIS THERE IS NO WAY TO SEE A BLOW. SetAnimationMode does not
	// hold: rotate() re-derives the mode from the entity's own state every
	// time setTarget runs -- which is every waypoint of every route pursuit
	// hands over -- so an externally set A1 lasts only until the thing is
	// next given somewhere to walk.
	held           bool
	heldMode       d2enum.MonsterAnimationMode
	onHeldFinished func()

	// corpse is DD, held for the rest of the run. It is a separate flag from
	// held because a corpse never finishes: the resolver despawns nothing, so
	// a dead monster stays on the map with a dead monster's sprite until
	// something else (M4.7's corpse machine) takes it away.
	corpse bool
}

const (
	magicOffsetX            = 5
	magicOffsetScalarX      = 8
	magicOffsetScalarY      = 16
	minAnimationRepetitions = 3
	maxAnimationRepetitions = 5
)

// selectEquip picks an equipment variant with the world RNG (falling back to
// the global generator before the factory is seeded).
func (f *MapEntityFactory) selectEquip(slice []string) string {
	if len(slice) != 0 {
		return slice[f.randIntn(len(slice))]
	}

	return ""
}

func (v *NPC) randIntn(n int) int {
	if v.rng != nil {
		return v.rng.Intn(n)
	}

	// nolint:gosec // not cryptographic; pre-seed fallback only
	return rand.Intn(n)
}

// ID returns the NPC uuid
func (v *NPC) ID() string {
	return v.mapEntity.uuid
}

// Render renders this entity's animated composite.
func (v *NPC) Render(target d2interface.Surface) {
	renderOffset := v.Position.RenderOffset()
	target.PushTranslation(
		int((renderOffset.X()-renderOffset.Y())*magicOffsetScalarY),
		int(((renderOffset.X()+renderOffset.Y())*magicOffsetScalarX)-magicOffsetX),
	)

	defer target.Pop()

	if v.composite.Render(target) != nil {
		return
	}
}

// Path returns the current part of the entity's path.
func (v *NPC) Path() d2path.Path {
	return v.Paths[v.path]
}

// NextPath returns the next part of the entity's path.
func (v *NPC) NextPath() d2path.Path {
	v.path++
	if v.path == len(v.Paths) {
		v.path = 0
	}

	return v.Paths[v.path]
}

// SetPaths sets the entity's paths to the given slice. It also sets flags
// on the entity indicating that it has paths and has completed the
// previous none.
func (v *NPC) SetPaths(paths []d2path.Path) {
	v.Paths = paths
	v.HasPaths = len(paths) > 0
	v.isDone = true
}

// Advance is called once per frame and processes a
// single game tick.
func (v *NPC) Advance(tickTime float64) {
	// A CORPSE DOES NOT WALK, and this guard has to be above Step rather than
	// below it. Step carries the entity along whatever route it was last
	// given, and Pursuit.Release is deferred to M4.5 step 5 -- so a dead dog
	// still has a live chase handing it waypoints. Guarded only below, the
	// corpse glides after the player in the Dead pose, which reads as
	// deliberate rather than as a bug.
	//
	// The composite still advances, so the death animation finishes on screen.
	if !v.corpse {
		v.Step(tickTime)
	}

	if err := v.composite.Advance(tickTime); err != nil {
		return
	}

	// Nothing below may re-path a corpse or return it to Neutral.
	if v.corpse {
		return
	}

	// A held action ends when the composite has played it through once. The
	// animations loop by default (dc6_animation.go / dcc_animation.go both
	// construct with playLoop: true), so nothing returns a swing to Neutral
	// on its own -- this is what does.
	if v.held && v.composite.GetPlayedCount() >= 1 {
		v.finishHeldAction()
	}

	if !v.held && !v.corpse && v.HasPaths && v.wait() {
		// If at the target, set target to the next path.
		v.isDone = false
		path := v.NextPath()
		v.setTarget(
			path.Position,
			v.next,
		)

		v.action = path.Action
	}
}

// If an npc has a path to pause at each location.
// Waits for animation to end and all repetitions to be exhausted.
func (v *NPC) wait() bool {
	return v.isDone && v.composite.GetPlayedCount() > v.repetitions
}

func (v *NPC) next() {
	var newAnimationMode d2enum.MonsterAnimationMode

	v.isDone = true

	v.repetitions = minAnimationRepetitions + v.randIntn(maxAnimationRepetitions)

	switch d2enum.NPCActionType(v.action) {
	case d2enum.NPCActionSkill1:
		newAnimationMode = d2enum.MonsterAnimationModeSkill1
		v.repetitions = 0
	case d2enum.NPCActionInvalid, d2enum.NPCAction1, d2enum.NPCAction2, d2enum.NPCAction3:
		newAnimationMode = d2enum.MonsterAnimationModeNeutral
		v.repetitions = 0
	default:
		newAnimationMode = d2enum.MonsterAnimationModeNeutral
		v.repetitions = 0
	}

	if v.composite.GetAnimationMode() != newAnimationMode.String() {
		if err := v.composite.SetMode(newAnimationMode, v.composite.GetWeaponClass()); err != nil {
			return
		}
	}
}

// rotate sets direction and changes animation
func (v *NPC) rotate(direction int) {
	// A HELD MODE IS NOT OVERRIDDEN, and this guard is the whole reason
	// StartAction can promise anything.
	//
	// rotate is this entity's `directioner` (factory.go), and mapEntity calls
	// a directioner from setTarget -- so it fires on every waypoint of every
	// route pursuit hands over. Without the guard, a monster re-routed in the
	// middle of a swing has that swing replaced by Walk.
	//
	// MEASURED 3 Sep 2026: a SETTLED fight never re-routes -- the pursuer
	// ends its route on the quarry's own tile, reports arrived, and stops
	// asking -- but a fight the player is walking away from re-solves every
	// MinRepathMinutes and walks. That is the common case, not the rare one.
	//
	// The direction still turns. Which way a thing faces is not what it is
	// doing, and a corpse that spins is worse than one that does not.
	if v.held || v.corpse {
		if v.composite.GetDirection() != direction {
			v.composite.SetDirection(direction)
		}

		return
	}

	var newMode d2enum.MonsterAnimationMode
	if !v.atTarget() {
		newMode = d2enum.MonsterAnimationModeWalk
	} else {
		newMode = d2enum.MonsterAnimationModeNeutral
	}

	if newMode.String() != v.composite.GetAnimationMode() {
		if err := v.composite.SetMode(newMode, v.composite.GetWeaponClass()); err != nil {
			return
		}
	}

	if v.composite.GetDirection() != direction {
		v.composite.SetDirection(direction)
	}
}

// Selectable returns true if the object can be highlighted/selected.
func (v *NPC) Selectable() bool {
	// is there something handy that determines selectable npc's?
	return v.name != ""
}

// Label returns the NPC's in-game name (e.g. "Deckard Cain") or an empty string if it does not have a name.
func (v *NPC) Label() string {
	return v.name
}

// GetPosition returns the NPC's position
func (v *NPC) GetPosition() d2vector.Position {
	return v.mapEntity.Position
}

// GetVelocity returns the NPC's velocity vector
func (v *NPC) GetVelocity() d2vector.Vector {
	return v.mapEntity.velocity
}

// GetSize returns the current frame size
func (v *NPC) GetSize() (width, height int) {
	return v.composite.GetSize()
}

// MonStat returns the monstats record this NPC was built from, or nil.
//
// The record is already held (the factory sets it at construction) and 252 of
// its fields are decoded; until M4.5 step 3 nothing outside this package could
// read any of them, so the game screen could not learn a monster's hit-point
// band without going through HarnessState -- which is the mistake Pursuit's
// unexported `arrived` made, answering a Go question out of a JSON map.
//
// Read-only, and it is NOT health: it is the record the entity was stamped
// from. Where a monster's CURRENT health lives, and why it lives there, is
// npcBody in d2game/d2gamescreen.
func (v *NPC) MonStat() *d2records.MonStatRecord {
	return v.monstatRecord
}

// SetAnimationMode tells the composite which mode to play, keeping the
// current weapon class. It mirrors Player.SetAnimationMode (player.go).
//
// THIS IS THE FIRST EXPORTED WAY TO TELL A MONSTER TO DO ANYTHING. Before it,
// an NPC's whole animation vocabulary was the three modes next() and rotate()
// set for themselves -- Neutral, Walk and Skill1 -- and a census of Attack1,
// Attack2, GetHit, Death, Dead and Block outside d2enum returned empty for
// every one: no monster in this codebase had ever been told to attack, be hit
// or die. animation_mode is already reported per entity, so the day a
// resolver calls this, "the monster swung" becomes assertable.
//
// MEASURED (tools/animcensus, 31 Aug 2026): fallen1, zombie1 and skeleton1 --
// the three codes the signed spawn tables use -- all have A1, A2, GH, DT and
// DD, so those calls will find an animation rather than an error.
//
// TWO THINGS THIS DOES NOT DO, and the second is a live constraint on step 4.
// It does not hold the mode: next() and rotate() set the mode from the NPC's
// own state whenever they run, so an externally set A1 lasts only until the
// entity next decides otherwise. And it does not tell the caller when the
// swing lands; the precedent for that is Player.StartCasting(mode, onFinished).
// A resolver that needs either will need a held-mode path, which is step 4's
// to build and is deliberately not invented here.
func (v *NPC) SetAnimationMode(mode d2enum.MonsterAnimationMode) error {
	return v.composite.SetMode(mode, v.composite.GetWeaponClass())
}

// StartAction plays one animation and HOLDS it until it has played through,
// then returns the monster to Neutral and calls onFinished.
//
// THIS IS THE HELD-MODE PATH M4.5 STEP 3 NAMED AND DID NOT BUILD, and it is
// what makes "the monster swung" an assertable fact: animation_mode is already
// reported per entity, so a playtest reads A1 / GH / DT / DD out of the entity
// provider at some frame after the blow. Its shape is Player.StartCasting's --
// a mode plus a completion callback -- because that precedent already exists
// on the other side of the map.
//
// DEATH IS THE EXCEPTION AND IT IS DELIBERATE: MonsterAnimationModeDeath is
// followed by Dead, which is then held for the rest of the run. The resolver
// despawns nothing (its fence), so a dead monster stays on the map and must
// keep a dead monster's sprite; what a corpse eventually BECOMES is M4.7's.
//
// onFinished may be nil. A corpse refuses every further action.
func (v *NPC) StartAction(mode d2enum.MonsterAnimationMode, onFinished func()) error {
	if v.corpse {
		return nil
	}

	// Composite.SetMode SHORT-CIRCUITS on the mode it is already in and so
	// does not reset the played count (composite.go). A second swing in a row
	// would then be a no-op on an animation already past its end, and would
	// "finish" on the very next tick with nothing seen. Cycling through
	// Neutral is what resets it.
	if v.composite.GetAnimationMode() == mode.String() {
		if err := v.SetAnimationMode(d2enum.MonsterAnimationModeNeutral); err != nil {
			return err
		}
	}

	if err := v.SetAnimationMode(mode); err != nil {
		return err
	}

	v.held = true
	v.heldMode = mode
	v.onHeldFinished = onFinished

	return nil
}

// finishHeldAction ends a held action. Advance calls it when the composite has
// played the mode through once.
func (v *NPC) finishHeldAction() {
	done, mode := v.onHeldFinished, v.heldMode

	v.held = false
	v.onHeldFinished = nil

	// The callback runs WHATEVER the mode change does. A caller waiting to
	// hear that the swing landed must not be silenced by a missing animation
	// in the MPQs, and the early returns below used to do exactly that.
	defer func() {
		if done != nil {
			done()
		}
	}()

	if mode != d2enum.MonsterAnimationModeDeath {
		_ = v.SetAnimationMode(d2enum.MonsterAnimationModeNeutral)

		return
	}

	// DT is over. Hold DD for the rest of the run -- and only call it a corpse
	// once that mode has actually taken, or a failed SetMode would leave a
	// thing that is frozen in the death animation and can never act again.
	if err := v.SetAnimationMode(d2enum.MonsterAnimationModeDead); err != nil {
		return
	}

	v.corpse = true

	// Drop the route pursuit last handed it. rotate is already guarded by the
	// corpse flag set above, so this turns nothing and changes no mode.
	v.StopMoving()
}
