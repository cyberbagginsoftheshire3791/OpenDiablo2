package d2world

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
)

// T1, THE TACTICAL LAYER (23 Sep 2026).
//
// Josh played the build on 19-20 September and his verdict on combat was one
// line: "it isn't even turn based dude". He was right about what he SAW, and
// the resolver was not the reason. The rounds were real -- D8's order, Move +
// Action, graze/hit/crit, the riposte -- but nothing about them reached the
// screen:
//
//   - a fight opened only once an aware enemy was already ADJACENT, after a
//     real-time chase, so the approach was never part of the fight;
//   - rounds were paced by WORLD time, so the enemy half of a round resolved in
//     one frame the instant the player committed, and every blow of a pack
//     landed at once;
//   - enemies never moved on their turn -- a pack member out of reach simply
//     did nothing -- so the only movement in a fight was D2's real-time walk;
//   - the player could walk anywhere, at any speed, at any time.
//
// This file is the answer, and it is a SECOND WAY TO RUN THE SAME ROUNDS rather
// than a replacement for the first. When the Paced dial is on and a person has
// control, the encounter is driven by Tick -- real seconds, one beat per visible
// thing -- instead of by world minutes, and each closed round charges exactly
// RoundMinutes to the world (R2 §2A's "1 round = 1 world minute", which the
// world-time path only approximated). Every blow still goes through
// resolveBlow; nothing lands by a path the policy could not have taken.
//
// THE OLD PATH IS UNTOUCHED AND STILL THE DEFAULT, on purpose: DefaultCombatDials
// leaves Paced off and EngageTiles at adjacency, so the resolver's ~40 unit
// tests describe exactly what they described before. The game screen turns the
// tactical layer on in shippedCombatDials().

// Stepper is how the combat model moves a body on its turn. d2world cannot
// import the map, so the game screen implements it -- the Animator and Chases
// precedent -- and a nil Stepper means nothing steps (an enemy out of reach
// then waits where it is, which is what the world-time path always did).
type Stepper interface {
	// StepToward orders id to walk at most `tiles` tiles toward (x, y),
	// stopping BESIDE that point rather than on it. It reports whether a walk
	// was ordered; false means no route, or nowhere free to stand.
	StepToward(id string, x, y float64, tiles int) bool

	// Moving reports whether id is still walking an order.
	Moving(id string) bool

	// Halt stops id where it stands. A released chase does NOT stop the path
	// already in flight -- Pursuit.Release only forgets the chase -- so without
	// this a participant keeps closing in real time after the fight has
	// opened, and "moves only on its turn" is false for its first turn.
	Halt(id string)
}

// SetStepper attaches the body-mover. It is a setter rather than another
// constructor argument because NewCombat already takes ten, at four
// construction sites, and a nil stepper is a legal and tested configuration.
func (c *Combat) SetStepper(s Stepper) { c.stepper = s }

// The tactical defaults. Each one is a [DIAL]; the numbers are a first
// guess made to be moved by Josh's first fight, not a ruling.
const (
	// TacticalEngageTiles: a fight opens when an aware enemy is this close.
	// Five tiles is roughly the on-screen radius (state.md: only ~5 tiles in
	// any direction are visible), so the approach happens where he can see it.
	TacticalEngageTiles = 5

	// TacticalDisengageTiles: the fight ends when nothing alive is this close.
	// Wider than engage so a fight does not flicker open and shut at the edge.
	TacticalDisengageTiles = 8

	// TacticalEnemyMoveTiles is how far a pack walks on its turn. More than the
	// player's two (the c-2 ruling, MoveTiles 2): outrunning wolves is not a
	// plan, and avoidance-first means not being caught in the first place.
	TacticalEnemyMoveTiles = 3

	// TacticalBeatSeconds is the pause after each visible thing -- a blow, the
	// end of a round -- so a person can see what happened.
	TacticalBeatSeconds = 0.45

	// TacticalStepTimeout caps how long a pack's walk is waited on. A body
	// that cannot finish its path (blocked, or an entity with no walk cycle)
	// must not hang the fight.
	TacticalStepTimeout = 2.5

	// blowLogLength is how many blows the HUD's log keeps.
	blowLogLength = 5
)

// paced reports whether THIS fight runs as the tactical layer: the dial is on
// and a person is at the controls. Under the policy there is nobody to wait
// for and nothing to watch, so the world-time path stands.
func (c *Combat) paced() bool {
	return c.dials.Paced && c.dials.PlayerControl == PlayerControlHuman
}

// Paced reports whether a paced fight would run now: the dial is on and a
// person is at the controls. The game screen gates its tactical input and the
// overlay on it, so an unpaced or policy fight keeps the old click behaviour.
func (c *Combat) Paced() bool { return c.paced() }

// WorldHeld reports whether the fight is holding the world still. It replaces
// Awaiting as the game screen's gate: a paced fight holds the world for its
// WHOLE length -- the player's think and the packs' visible turns alike -- and
// pays for each round with TakeRoundMinutes instead.
//
// Awaiting keeps its meaning (the player's turn is open) and its other
// readers: the harness's step_world guard and the decision timer.
func (c *Combat) WorldHeld() bool {
	if c.encounter == nil {
		return false
	}

	return c.encounter.awaiting || c.paced()
}

// TakeRoundMinutes hands over the world minutes owed by rounds closed since the
// last call, and zeroes the debt. The game screen advances the world by exactly
// this much, so a paced round costs RoundMinutes and not a frame more.
func (c *Combat) TakeRoundMinutes() float64 {
	m := c.owedMinutes
	c.owedMinutes = 0

	return m
}

// Participates reports whether id is in the live fight. The game screen asks it
// before starting a chase: inside a paced fight a participant moves only on its
// turn, and a chase re-pathing it every tick would walk it in real time.
func (c *Combat) Participates(id string) bool {
	e := c.encounter
	if e == nil || id == "" {
		return false
	}

	if e.target != nil && e.target.QuarryID() == id {
		return true
	}

	return e.enemyByID(id) != nil
}

// engageTiles and disengageTiles never fall below adjacency: a fight must be
// able to open on anything it could strike, and must not close on it.
//
// BOTH ARE THE TACTICAL LAYER'S AND ONLY APPLY TO A PACED FIGHT. Under the
// policy -- which every playtest script except the hands acts runs on (the
// launcher's dropToPolicy) -- a fight opens and closes at adjacency exactly as
// it did before T1, so a script that measures the world-time path is not
// quietly measuring a fight that opened four tiles earlier. One switch.
func (c *Combat) engageTiles() int {
	if !c.paced() {
		return c.dials.AdjacentTiles
	}

	if c.dials.EngageTiles > c.dials.AdjacentTiles {
		return c.dials.EngageTiles
	}

	return c.dials.AdjacentTiles
}

func (c *Combat) disengageTiles() int {
	// Fenced like engage -- the review caught the first draft leaving this one
	// out, which kept a policy fight open until the enemy was 8 tiles off.
	if !c.paced() {
		return c.dials.AdjacentTiles
	}

	if c.dials.DisengageTiles > c.engageTiles() {
		return c.dials.DisengageTiles
	}

	return c.engageTiles()
}

// within is the Chebyshev test on floored tiles that inReach has always been,
// at an arbitrary radius.
func within(w Combatant, q Quarry, tiles int) bool {
	if w == nil || q == nil {
		return false
	}

	wx, wy := w.WatcherAt()
	qx, qy := q.QuarryAt()

	dx := int(math.Abs(math.Floor(wx) - math.Floor(qx)))
	dy := int(math.Abs(math.Floor(wy) - math.Floor(qy)))

	return dx <= tiles && dy <= tiles
}

// holdParticipants takes the chase off every enemy in a paced fight and stops
// every body in it where it stands -- the player's too -- so the only thing
// that moves anyone is his own turn. Their WATCH stays: they are still aware of
// the player, and when the fight ends (disengaged) the game's
// startChasesForTheAware picks them straight back up.
func (c *Combat) holdParticipants() {
	e := c.encounter
	if e == nil || !c.paced() {
		return
	}

	for _, enemy := range e.enemies {
		if enemy != nil {
			c.holdOne(enemy.WatcherID())
		}
	}

	if c.stepper != nil && e.target != nil {
		c.stepper.Halt(e.target.QuarryID())
	}
}

// holdOne takes one enemy's chase and stops its walk.
func (c *Combat) holdOne(id string) {
	if c.chases != nil {
		c.chases.Release(id)
	}

	if c.stepper != nil {
		c.stepper.Halt(id)
	}
}

// Tick drives a paced fight on real seconds. The game screen calls it every
// frame the screen is live (not under the escape menu); outside a paced fight,
// and during the player's own turn, it does nothing.
//
// It is a small state machine and every state waits for something a person can
// see: the player's walk to finish, a pack's walk to finish, a beat after each
// blow. The loop is bounded so a state that forgets to wait cannot spin.
func (c *Combat) Tick(elapsed float64) {
	e := c.encounter
	if e == nil || !c.paced() || e.awaiting {
		return
	}

	if elapsed > 0 {
		e.beat -= elapsed

		if len(e.stepping) > 0 {
			e.stepWait += elapsed
		}

		if c.stepper != nil && e.target != nil && c.stepper.Moving(e.target.QuarryID()) {
			e.playerWait += elapsed
		}
	}

	playerID := ""
	if e.target != nil {
		playerID = e.target.QuarryID()
	}

	const maxSteps = 256

	for i := 0; i < maxSteps; i++ {
		e = c.encounter
		if e == nil || e.awaiting || e.beat > 0 {
			return
		}

		// Nothing acts while the player is still walking his Move -- but not
		// forever: a walk that never settles is stopped where it is, exactly
		// as a pack's is, or the fight would stall on it.
		if c.stepper != nil && playerID != "" && c.stepper.Moving(playerID) {
			if e.playerWait < TacticalStepTimeout {
				return
			}

			c.stepper.Halt(playerID)
		}

		e.playerWait = 0

		switch {
		case len(e.stepping) > 0:
			if e.stepWait < TacticalStepTimeout && c.anyMoving(e.stepping) {
				return
			}

			// A walk the timeout cut short is STOPPED, not left running: a
			// body still walking when his turn opens is moving on his time.
			if c.stepper != nil {
				for _, id := range e.stepping {
					if c.stepper.Moving(id) {
						c.stepper.Halt(id)
					}
				}
			}

			e.stepping = nil
			e.strikers = c.strikersOf(e.acting)

		case len(e.strikers) > 0:
			id := e.strikers[0]
			e.strikers = e.strikers[1:]

			c.enemyActivation(id)

			if c.encounter == nil {
				return
			}

			e.beat = TacticalBeatSeconds

		case e.acting != nil:
			// The pack's activation is complete.
			e.acting = nil

		case !e.roundOpen:
			c.openPacedRound()

		case e.cursor >= len(e.sequence):
			c.closePacedRound()

			if c.encounter == nil {
				return
			}

			e.beat = TacticalBeatSeconds

		default:
			id := e.sequence[e.cursor]

			if id == playerID {
				e.awaiting = true
				c.decisionSecondsRound = 0

				return
			}

			c.beginPackActivation()
		}
	}
}

// openPacedRound is resolveRound's opening half without the walk: the log is
// reset, quick-resolve is offered, and the sequence is frozen.
func (c *Combat) openPacedRound() {
	e := c.encounter

	c.lastActions = c.lastActions[:0]
	c.actionsRound = e.round

	// Open BEFORE quick-resolve: a round that ends the fight is still a round
	// lived, and end() charges an open round's minutes (see payOpenRound).
	e.roundOpen = true

	if c.tryQuickResolve() {
		return
	}

	e.sequence = e.activation()
	e.cursor = 0
	e.moveSpent = false
	e.actionSpent = false
}

// closePacedRound is the tail the world-time path runs after a walk: count the
// round, pay for it in world minutes, let reinforcements in and let the
// disengaged out.
func (c *Combat) closePacedRound() {
	e := c.encounter

	e.sequence = nil
	e.cursor = 0
	e.roundOpen = false

	c.finishRound()

	c.owedMinutes += c.dials.RoundMinutes

	if c.encounter != nil {
		c.reinforce()
		c.pruneOrEnd()
	}
}

// beginPackActivation starts the activation at the cursor. A PACK IS ONE
// ACTIVATION (R2 §3 bullet 2): the consecutive members of one group in the
// frozen sequence move together -- "wolves move as one" -- and then each one
// that ended its walk in reach strikes, a beat apart.
func (c *Combat) beginPackActivation() {
	e := c.encounter

	first := e.sequence[e.cursor]
	group := c.profileOf(first).Group

	var members []string

	for e.cursor < len(e.sequence) {
		id := e.sequence[e.cursor]
		if id == e.target.QuarryID() || c.profileOf(id).Group != group {
			break
		}

		if !e.gone(id) {
			members = append(members, id)
		}

		e.cursor++

		// A member with no group is its own pack of one (profileOf keys it by
		// its id), so this loop takes exactly one of those.
		if group == "" {
			break
		}
	}

	if len(members) == 0 {
		return
	}

	e.acting = members
	e.stepWait = 0

	tx, ty := e.target.QuarryAt()

	for _, id := range members {
		enemy := e.enemyByID(id)
		if enemy == nil || c.inReach(enemy, e.target) {
			continue
		}

		if c.stepper == nil || c.dials.EnemyMoveTiles <= 0 {
			continue
		}

		if c.stepper.StepToward(id, tx, ty, c.dials.EnemyMoveTiles) {
			e.stepping = append(e.stepping, id)
			c.stepsOrdered++
		}
	}

	if len(e.stepping) == 0 {
		e.strikers = c.strikersOf(members)
	}
}

// payOpenRound charges the round a paced fight ENDS in. closePacedRound is the
// only other place a round is paid, and a fight that ends mid-round -- his
// killing blow, his death in the packs' turn, a quick-resolve -- never reaches
// it: without this every decisive fight was one round cheaper than it was, and
// torches, meters and the clock undercounted it (review finding, 23 Sep).
func (c *Combat) payOpenRound() {
	if e := c.encounter; e != nil && c.paced() && e.roundOpen {
		e.roundOpen = false
		c.owedMinutes += c.dials.RoundMinutes
	}
}

// strikersOf is the members still standing, still in the fight and in reach.
func (c *Combat) strikersOf(members []string) []string {
	e := c.encounter
	if e == nil {
		return nil
	}

	out := make([]string, 0, len(members))

	for _, id := range members {
		if e.gone(id) {
			continue
		}

		if enemy := e.enemyByID(id); enemy != nil && c.inReach(enemy, e.target) {
			out = append(out, id)
		}
	}

	return out
}

func (c *Combat) anyMoving(ids []string) bool {
	if c.stepper == nil {
		return false
	}

	for _, id := range ids {
		if c.stepper.Moving(id) {
			return true
		}
	}

	return false
}

// --- what the HUD reads ----------------------------------------------------

// BlowLine is one blow as the combat log shows it.
type BlowLine struct {
	Round    int
	Attacker string
	Target   string
	Band     string
	Damage   int
	Reaction string
	Killed   bool
}

// TacticalEnemy is one enemy participant as the overlay draws it.
type TacticalEnemy struct {
	ID       string
	X, Y     float64
	Adjacent bool
	Dead     bool
	Routed   bool
	Acting   bool
}

// TacticalView is everything the tactical overlay draws, read in one call so
// the HUD never assembles a fight out of pieces that could disagree.
type TacticalView struct {
	Fighting bool
	Paced    bool
	Round    int

	// Phase is "player" while his turn is open, "enemy" while a pack is
	// acting or the round is closing, and "" out of a fight.
	Phase string

	PlayerID          string
	MoveSpent         bool
	ActionSpent       bool
	ReactionAvailable bool
	MoveTiles         int

	Enemies []TacticalEnemy
	Blows   []BlowLine
}

// Tactical reports the live fight for the overlay. Out of a fight it still
// carries the last blows, so the log does not vanish the instant a fight ends.
func (c *Combat) Tactical() TacticalView {
	v := TacticalView{
		Paced:     c.paced(),
		MoveTiles: c.dials.MoveTiles,
		Blows:     append([]BlowLine(nil), c.blowLog...),
	}

	e := c.encounter
	if e == nil {
		return v
	}

	v.Fighting = true
	v.Round = e.round
	v.MoveSpent = e.moveSpent
	v.ActionSpent = e.actionSpent

	v.Phase = "enemy"
	if e.awaiting {
		v.Phase = "player"
	}

	if e.target != nil {
		v.PlayerID = e.target.QuarryID()

		// T3: Quick Feet.
		v.MoveTiles += c.edgeOf(v.PlayerID).ExtraMove

		if f := c.fitnessOf(v.PlayerID); f != nil {
			v.ReactionAvailable = f.ReactionAvailable() && !f.Shaken() &&
				c.reactionsLeft() && !(e.round == 1 && e.surprised)
		}

		// T2: and a blade that ripostes -- the pip must not offer what the
		// resolver will refuse (riposteAllowed reads the same weapon).
		if k := c.kitOf(v.PlayerID); k != nil {
			if b, ok := k.MainBite(); !ok || b.Reaction != d2items.ReactionRiposte {
				v.ReactionAvailable = false
			}
		}
	}

	acting := make(map[string]bool, len(e.acting))
	for _, id := range e.acting {
		acting[id] = true
	}

	for _, enemy := range e.enemies {
		if enemy == nil {
			continue
		}

		id := enemy.WatcherID()
		x, y := enemy.WatcherAt()

		v.Enemies = append(v.Enemies, TacticalEnemy{
			ID: id, X: x, Y: y,
			Adjacent: c.inReach(enemy, e.target),
			Dead:     e.dead[id],
			Routed:   e.routed[id],
			Acting:   acting[id],
		})
	}

	return v
}

// logBlow keeps the HUD's rolling log. It is fed from resolveBlow, the one
// place every blow passes, so the log cannot show a blow that did not happen.
func (c *Combat) logBlow(a action) {
	line := BlowLine{
		Round: a.round, Attacker: a.attacker, Target: a.target,
		Band: a.band, Damage: a.damage, Reaction: a.reaction,
		Killed: a.targetHasBody && a.targetHealthAfter <= 0,
	}

	c.blowLog = append(c.blowLog, line)
	if n := len(c.blowLog); n > blowLogLength {
		c.blowLog = append(c.blowLog[:0], c.blowLog[n-blowLogLength:]...)
	}
}
