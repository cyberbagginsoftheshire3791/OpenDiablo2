package d2world

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// Combat is M4.5's encounter model: it decides that a fight is happening, who
// is in it, whose turn it is, and -- since step 4 -- what a blow does. The
// resolver itself lives in combat_resolver.go, on this same struct.
//
// THE PROVIDER IS BUILT BEFORE THE RESOLVER, DELIBERATELY, and the note that
// signed this milestone argued the inversion at length. Four milestones of
// evidence: M4.1 shipped a placed light nothing could assert, M4.3a signed an
// assertion nothing in the harness could write, M4.3b needed two provider
// fields discovered mid-build. A fight that happens is not evidence the fight
// worked, because a fight can end correctly for the wrong reason and look
// identical from outside.
//
// WHAT IT IS ALLOWED TO KNOW is the fence, and it is the ask hardest to move
// later (note §4.2). The round, the participants, where they are, whether they
// are in reach, and the facts other systems already compute about them. NOT
// the inventory, NOT the map, and it spawns and despawns nothing.
//
// IT READS RATHER THAN RECOMPUTES, which is three signed deferrals arriving at
// once: the meters' ReactionAvailable and Shaken (M4.2 built them for this),
// the spawn tables' morale and routing (M4.3b owns the state, this owns the
// behaviour), and the light model's level at a tile (M4.1: "so that 'I can't
// see it' and 'I can't hit it' are one fact").
type Combat struct {
	dials    CombatDials
	clock    *Clock
	notice   *Notice
	fitness  Fitness
	illum    Illumination
	bodies   Bodies
	profiles Profiles
	animator Animator

	rng *rand.Rand

	encounter *encounter
	nextID    int

	// lastActions is every blow of the most recently RESOLVED ROUND, and
	// actionsRound is which round that was.
	//
	// IT IS CLEARED WHEN A ROUND STARTS RESOLVING, NOT WHEN Advance IS
	// CALLED, and the difference is the whole reason a script can read it at
	// all. Advance runs once per FRAME -- advanceWorld is driven by the
	// screen's own delta, so one stepped "world minute" is 15 to 24 calls --
	// and a round boundary falls inside one of them. A log cleared per call
	// is therefore empty on fourteen reads out of fifteen, which is exactly
	// what the first run of the thirteenth playtest found: `actions: []` on a
	// tick that had plainly just resolved two rounds.
	//
	// Retained this way, a script that steps and then reads always sees the
	// last round that actually happened, whichever frame it happened on. A
	// script that needs to know whether it is looking at a NEW round compares
	// actions_round, which is what makes counting over several steps exact.
	lastActions  []action
	actionsRound int

	// Counters, all reported. An encounter that started and ended between two
	// harness reads is invisible in the state and obvious in the counters.
	started  int
	ended    int
	rounds   int
	declines int
	actions  int

	// endedReason is why the LAST encounter ended, and it persists after the
	// encounter is gone -- an encounter that begins and ends between two
	// harness reads is otherwise invisible. The three counters beside it are
	// what make it countable rather than merely last-seen.
	endedReason      string
	endedEnemiesDead int
	endedPlayerDead  int
	endedDisengaged  int
}

// Fitness is what the combat model may know about the player's body beyond its
// health. Both values are R2 rules stated as facts about the body: §1's
// fatigue rule and §3's Shaken. M4.2 computes them and this READS them, which
// was signed as ask 1 of that milestone -- "M4.5's resolver then reads them
// rather than recomputing them".
//
// It is an interface rather than a *Meters so that the fence is in the type
// system rather than in a comment, and so this system can be tested without a
// body, a clock or a set of dials it does not use.
type Fitness interface {
	ReactionAvailable() bool
	Shaken() bool

	// Activity is what the body was doing at the moment something reached it,
	// and D8 §9's caught-head-down branch is stated entirely in terms of it:
	// a player caught foraging or labouring loses round one's initiative and
	// his Reaction with it, and one who was idle or on watch does not.
	//
	// IT COSTS *Meters NOTHING -- Meters.Activity already exists (meters.go),
	// so widening this interface widens no implementation. It is read ONCE,
	// in tryStart, BEFORE the game sets `labour` for the fight, which the
	// order of advanceWorld's call list guarantees.
	Activity() Activity
}

// Bodies is how the combat model asks what an enemy's body is, by the same id
// Notice and Pursuit already know it by. It is the monster half of what
// Fitness is for the player: the narrowest thing that answers the question,
// rather than a handle on the thing that owns the answer.
//
// Nil means no body is known, and the model must cope rather than assume:
// the game screen adopts a body for every monster the spawn tables send, but
// the harness and the debug terminal can both put an NPC on the map without
// going through it, and a fight against one of those is a fight against
// something with no body. That is reported as has_body:false rather than as
// zero health, because "I do not know" and "it is dead" are different facts
// and A3's fail-open lesson is what happens when they share a value.
//
// An implementation MUST return an untyped nil for an unknown id. Returning a
// nil *T here yields a non-nil interface, which would report has_body:true
// for a body that is not there.
type Bodies interface {
	BodyOf(id string) Body

	// BodiesKnown is how many bodies the registry holds.
	//
	// It exists so that ADOPTION IS OBSERVABLE, and that is not decoration.
	// The game screen adopts a body when the spawn tables place a monster,
	// and BodyOf also adopts one on demand for anything the tables did not
	// place -- so without a count, a playtest cannot tell the two apart, and
	// deleting the eager path would break nothing any test could see. That is
	// the shape of the M4.1 and M4.3b failures exactly: a thing the game is
	// supposed to do, which only the harness ever actually does, passing
	// because no assertion could distinguish them.
	//
	// With the count, forcing a real arrival and watching this rise BEFORE
	// any fight exists is evidence about the game.
	BodiesKnown() int
}

// Combatant is a participant. It is deliberately the same shape as Watcher --
// the thing that notices you is the thing that comes for you is the thing you
// end up fighting, and inventing a third identity for it would mean three
// adapters where the game screen already provides one.
type Combatant interface {
	WatcherID() string
	WatcherAt() (x, y float64)
}

// CombatDials are the numbers M4.5 ships with. Every one is a [DIAL].
type CombatDials struct {
	// RoundMinutes is how much world time a round costs. SIGNED: R2 §2A puts
	// it at [1 round = 1 world minute], which is what makes a long fight a
	// costly fight -- torches burn per round and the night's stages advance
	// mid-fight.
	//
	// At the harness's default tick this is ~15 stepped frames by day and ~24
	// at night: the clock COMPRESSES, and a world-minute budget is at its
	// tightest in daylight, which is the opposite of the intuition.
	RoundMinutes float64

	// AdjacentTiles is how close counts as in reach, in whole tiles, measured
	// on the Chebyshev distance between floored tile positions -- so 1 means
	// the eight neighbours and the tile itself.
	//
	// It is SEPARATE from Pursuit's ArriveWithin (1.0, a Euclidean distance)
	// on purpose, and the note's ask 6 says why: the router already targets
	// the eight neighbour tiles because a quarry's own footprint is not a
	// place another thing can path to, so "arrived" and "in reach" are two
	// facts that happen to agree today. One overloaded dial would hide the
	// day they stop agreeing.
	//
	// MEASURED AT STEP 4, and the sentence above is wrong about the map: a
	// quarry's footprint IS routable -- entities do not block the search --
	// so a pursuer walks onto the player's own tile and stops there at
	// distance 0.000 rather than beside him. Every participant in a settled
	// fight therefore floors to ONE tile. That is why dark-into-light cannot
	// fire in a v0 build (§4.3 of the step-4 brief, and the resolver's
	// advantage() comment), and making a pursuer stop on an ADJACENT tile is
	// the one change that would make light, flank and facing real. Step 5 /
	// M4.3a's, not this dial's.
	AdjacentTiles int

	// --- step 4: the resolver's numbers. Every one is a [DIAL], every one is
	// settable, and they are DELIBERATELY VISIBLE rather than balanced: the
	// prototype is what answers them (M4.5 note ask 7). ---

	// GrazeBand and CritBand are the low and high slices of the d100. What is
	// left in the middle is a hit; it is reported rather than stored, so the
	// three can never disagree.
	//
	// THERE IS NO BINARY MISS (R2 §3 bullet 5: "whiffing five times isn't
	// tension, it's static"), so the worst a blow does is a small wound.
	GrazeBand, CritBand int

	// GrazeFactor, HitFactor and CritFactor multiply the damage draw. A graze
	// still does at least 1 -- the floor is what makes "bounded" true at the
	// bottom of the range.
	GrazeFactor, HitFactor, CritFactor float64

	// AdvantageShift is what dark-into-light is worth on the roll, and its
	// mirror is the light-into-dark penalty (R2 §3 bullet 7 signs the rule
	// and gives NO magnitude; R2 §5 does not list it as a dial, so ask 7's
	// "treat it as a dial and start it visible" is what this is).
	AdvantageShift int

	// ShakenPenalty is what Shaken costs the player's accuracy (R2 §3 bullet
	// 8 signs the condition and not the number).
	ShakenPenalty int

	// LitLevel is the line between dark and lit, and it is DELIBERATELY THE
	// SAME NUMBER the notice model calls lit (notice.go): "a wolf can see
	// you" and "you are lit for the wolf's blow" should be one fact, which is
	// S1 §4's one-source-of-truth applied to light.
	LitLevel float64

	// PlayerAction is the player's stand-in Action policy: "attack" strikes
	// the first adjacent living enemy in the order, "hold" does nothing.
	//
	// IT IS A STAND-IN AND IT IS REPORTED AS ONE. There is no player attack
	// verb in the engine, no turn UI until M4.4, and no blow tool in the
	// harness by design -- so without a policy a real build's fight has only
	// one side acting and every fight ends player_dead, which is "you can
	// lose" only in the sense that a wall can lose. The turn UI replaces the
	// policy with a choice; until then a script reading actions[] can see
	// that the player's blow came from the policy rather than from a person.
	PlayerAction string

	// ForcedBand pins every blow's band to "graze", "hit" or "crit" ("" is
	// off). BOTH RNG DRAWS STILL HAPPEN when it is set, so forcing one blow
	// does not shift the sequence the next one sees.
	//
	// It is the ONLY way a script can steer an outcome, and that is the whole
	// argument of §5.3: a script that could set health, land a blow or set an
	// animation would prove something the game never does.
	ForcedBand string
}

// roundEpsilon absorbs the floating-point error in an ACCUMULATED world
// minute, and it is a measured fix rather than a defensive one.
//
// Advance is called once per frame with a fractional slice of a minute, so a
// stepped world minute arrives as fifteen additions that sum to
// 0.9999999999999999 -- just under RoundMinutes. Compared exactly, that
// resolves NO round; the shortfall then carries, and the next step resolves
// two. The step-4 brief listed this as unverified ("whether a one-minute step
// always yields exactly one round++") and the thirteenth playtest measured it
// on its first run: a fight plainly under way reported `actions: []` and
// `minutes_into_round: 0.9999999999999999`.
//
// A world minute is O(1) and a frame's slice is O(0.05), so 1e-9 is far below
// anything meaningful and far above the error being absorbed.
const roundEpsilon = 1e-9

// minRoundMinutes is the floor on the RoundMinutes dial. See the setter.
const minRoundMinutes = 1e-3

// DefaultCombatDials returns the signed starting values.
func DefaultCombatDials() CombatDials {
	return CombatDials{
		RoundMinutes:  1.0,
		AdjacentTiles: 1,

		GrazeBand:      35,
		CritBand:       15,
		GrazeFactor:    0.5,
		HitFactor:      1.0,
		CritFactor:     1.5,
		AdvantageShift: 20,
		ShakenPenalty:  15,
		// READ FROM THE NOTICE MODEL'S OWN DIAL rather than restated as
		// 0.30, so the two cannot drift apart in a later edit. The comment
		// on LitLevel is the reason; this line is the enforcement.
		LitLevel:     DefaultNoticeDials().LitLevel,
		PlayerAction: PlayerActionAttack,
		ForcedBand:   "",
	}
}

// encounter is one fight.
type encounter struct {
	id      string
	target  Quarry
	enemies []Combatant

	// enemyOrder is the ENEMY half of the activation sequence: packs in
	// descending authored Speed, ties broken once at fight start by the
	// seeded RNG, members within a pack in sorted id order.
	//
	// D8 IS SIGNED AND THIS IS THE FIRST STEP TO BUILD IT (D8 §9, signed
	// 1 Sep 2026). It replaces the provisional shuffle M4.5 step 2 shipped
	// under Josh's 29 Aug ruling -- "ship a deliberately dull, seeded order
	// and REPORT it, so D8 can replace it without touching anything else."
	// That is exactly what happened: a reported order was replaceable.
	//
	// The PLAYER's id is not in here. Where the player activates depends on
	// the round (§4.3's surprised round one puts him last), so the sequence a
	// script reads is computed by activation() rather than stored twice.
	enemyOrder []string

	// dead is every enemy whose body has reached 0. A dead enemy LEAVES the
	// order and takes no activation, and STAYS a participant with dead:true
	// for the encounter's life -- the rows are built from enemies, so it
	// cannot both leave that slice and keep a row.
	dead map[string]bool

	// initiator, surprised and surpriseWhy are D8 §9's initiation facts, read
	// ONCE at tryStart and then fixed for the fight. initiator is always
	// "enemy" in v0: the only path into tryStart is AwarePairs(), i.e.
	// something noticed the player. The ambush branch needs a player attack
	// verb, which does not exist in the game and, by design, not in the
	// harness either -- so it is a named deferral (M4.4) rather than code for
	// a branch nothing can reach.
	initiator   string
	surprised   bool
	surpriseWhy string

	// reactionUsedInRound is the round in which the player's one Reaction was
	// spent. R2 §3 bullet 6 caps reactions at one per round.
	reactionUsedInRound int

	round     int
	sinceTurn float64
}

// NewCombat builds the encounter model and registers the "combat" provider.
//
// seed is the run's seed: every roll the resolver makes -- the initiative
// tie-break, each blow's d100 and each blow's damage draw -- comes from this
// system's own RNG, so two launches of one build at one seed fight the same
// fight. That is R2 §3 bullet 12's determinism clause, and keeping the stream
// private to combat is what leaves every other system's draw sequence, and so
// every seed-1462 measurement outside a fight, exactly where it was.
//
// profiles and animator may both be nil, and the model copes rather than
// assuming: a nil Profiles means every enemy fights on the placeholder
// profile, and a nil Animator means nothing swings on screen. Both are
// reported (has_profiles, has_animator) on the has_notice precedent, so a
// playtest can tell "the wiring is missing" from "the rule did not fire".
func NewCombat(clock *Clock, notice *Notice, fitness Fitness, illum Illumination,
	bodies Bodies, profiles Profiles, animator Animator, seed int64, dials CombatDials) *Combat {
	c := &Combat{
		dials:    dials,
		clock:    clock,
		notice:   notice,
		fitness:  fitness,
		illum:    illum,
		bodies:   bodies,
		profiles: profiles,
		animator: animator,
		rng:      rand.New(rand.NewSource(seed)), // nolint:gosec // gameplay RNG, seeded for reproducibility
		nextID:   1,
	}

	d2harness.Register(c)

	return c
}

// Close unregisters the provider.
func (c *Combat) Close() { d2harness.Unregister(c) }

// Advance runs the encounter on the world minutes that just passed.
//
// It is stepped from Game.advanceWorld after the spawn tables and after
// startChasesForTheAware, so a thing that noticed the player on this tick and
// is already in reach can open a fight on the same tick rather than standing
// blind for one. The order of that call list is the whole turn structure this
// engine has: there is no queue anywhere in the tree, and every system takes
// world minutes as a float.
func (c *Combat) Advance(worldMinutes float64) {
	if worldMinutes <= 0 {
		return
	}

	if c.encounter == nil {
		c.tryStart()

		return
	}

	c.encounter.sinceTurn += worldMinutes

	// Rounds consume world time (R2 §2A). Several rounds can fall inside one
	// step if the caller hands over a large slice of world minutes, and they
	// are counted rather than collapsed -- a script that steps an hour should
	// see an hour's worth of rounds, not one.
	for c.encounter != nil && c.encounter.sinceTurn >= c.dials.RoundMinutes-roundEpsilon {
		c.encounter.sinceTurn -= c.dials.RoundMinutes

		// RESOLVE THE ROUND THAT IS RUNNING, THEN MOVE ON TO THE NEXT ONE,
		// and the order of those two is not cosmetic. tryStart opens the
		// encounter ON round 1, so incrementing first would resolve round 2
		// as the first resolved round and round 1 would never run at all --
		// which would make D8 §9's caught-head-down branch, whose whole
		// condition is `round == 1`, unreachable code. The unit tests caught
		// it; nothing else would have, because a fight in which the player
		// simply goes first every round looks completely normal.
		//
		// The counters are unchanged by the swap: tryStart counts round 1 as
		// entered and each pass here counts the next one, so `rounds` still
		// means rounds entered and Round() still reports the round now
		// running, exactly as they did before the resolver existed.
		c.resolveRound()

		c.rounds++

		// A blow can end the encounter -- the player died, or that was the
		// last of them. The remaining world minutes of that step buy no
		// further rounds.
		if c.encounter == nil {
			break
		}

		c.encounter.round++
	}

	if c.encounter != nil {
		c.pruneOrEnd()
	}
}

// tryStart opens an encounter when something aware of the player is also in
// reach of them.
//
// AWARENESS ALONE IS NOT A FIGHT, and the distinction is the milestone's:
// M4.3b decided that a thing has SEEN you and M4.3a closes the distance, and
// both were deliberately silent about the last two tiles. A pack that has
// noticed you from twelve tiles away is a chase, not an encounter.
//
// ONE HALF OF R2 §3's TRIGGER IS MISSING AND IT IS NAMED RATHER THAN FUDGED.
// R2 says combat triggers on MUTUAL awareness -- sight, sound and light. The
// notice model reports whether the group has noticed the PLAYER and nothing
// anywhere reports whether the player has noticed the GROUP, and noise was
// fenced out of that model by M4.3b's signature. So v0 triggers on the
// group's awareness alone; the asymmetry is in the milestone's DoD, and the
// second direction is additive to a model that already exists.
func (c *Combat) tryStart() {
	if c.notice == nil {
		return
	}

	var (
		enemies []Combatant
		target  Quarry
	)

	for _, pair := range c.notice.AwarePairs() {
		if pair.Target == nil || pair.Watcher == nil {
			continue
		}

		if !c.inReach(pair.Watcher, pair.Target) {
			c.declines++

			continue
		}

		// A CORPSE DOES NOT START A FIGHT, and this filter is load-bearing
		// rather than tidy. tryStart runs on EVERY tick the encounter is nil
		// and takes every noticed watcher, and step 4 clears neither the
		// notice model nor the chase when something dies -- both are step 5's
		// (Notice.Unwatch, Pursuit.Release). So a dog killed this round is
		// still noticed and still in reach on the next tick, and without this
		// line the frame after "enemies_dead" opens a fresh encounter against
		// the corpse, the policy re-kills it, and every counter a playtest
		// asserts == 1 on climbs once a round for the rest of the night.
		//
		// A watcher with NO body is alive by this test, which is the same
		// answer has_body:false already gives: it cannot be hurt and cannot
		// die, and "I do not know" must not read as "it is dead" (A3).
		if c.deadByBody(pair.Watcher.WatcherID()) {
			continue
		}

		// One encounter, one quarry. A second target is the target-selection
		// question notice.go:186 hands to this milestone, and v0 answers it
		// the dullest way available: the first aware pair in the stable order
		// names the quarry, and anything aware of somebody else waits.
		if target == nil {
			target = pair.Target
		}

		if pair.Target.QuarryID() != target.QuarryID() {
			continue
		}

		enemies = append(enemies, pair.Watcher)
	}

	if len(enemies) == 0 {
		return
	}

	// The other half of the same rule: nothing starts a fight with a dead
	// quarry either. Without it the frame after "player_dead" opens an
	// encounter against a 0-HP player and the pack beats the corpse forever.
	if target == nil || c.deadByBody(target.QuarryID()) {
		return
	}

	// D8 §9's initiation facts, read ONCE and then fixed for the fight.
	//
	// THE STANCE MUST BE READ BEFORE THE GAME SETS `labour` FOR THE FIGHT
	// (§4.6.3), and advanceWorld's call order is what guarantees it: the
	// game's write happens after combat.Advance returns, so what this reads
	// is what the player was doing when the pack arrived. The nil guard is
	// not defensive -- the constructor allows a nil fitness and every unit
	// test above the resolver uses one.
	stance := ActivityIdle
	if c.fitness != nil {
		stance = c.fitness.Activity()
	}

	surprised, why := false, ""

	switch stance {
	case ActivityForage:
		surprised, why = true, "caught-foraging"
	case ActivityLabour:
		surprised, why = true, "caught-labouring"
	case ActivityIdle, ActivityWatch:
		// Vigilant. D8 §9 names both as the un-surprised stances.
	}

	c.encounter = &encounter{
		id:      fmt.Sprintf("e:%d", c.nextID),
		target:  target,
		enemies: enemies,
		dead:    map[string]bool{},
		round:   1,

		// v0 has exactly one initiator. See encounter.initiator for why the
		// ambush branch is a named deferral rather than an unreachable else.
		initiator:   "enemy",
		surprised:   surprised,
		surpriseWhy: why,

		enemyOrder: c.d8Order(enemies),
	}

	// A NEW FIGHT STARTS WITH AN EMPTY LOG. lastActions is cleared when a
	// round RESOLVES, and a fresh encounter has resolved none -- so without
	// this the provider reports the previous fight's blows against this one,
	// and actions_round cannot tell them apart because both read 1.
	c.lastActions = c.lastActions[:0]
	c.actionsRound = 0

	c.nextID++
	c.started++
	c.rounds++
}

// deadByBody reports that the registry knows this id's body AND that body has
// run out. An unknown id is alive: see the call site in tryStart.
func (c *Combat) deadByBody(id string) bool {
	if c.bodies == nil {
		return false
	}

	body := c.bodies.BodyOf(id)

	return body != nil && body.CurrentHealth() <= 0
}

// pruneOrEnd drops enemies that are no longer in reach and ends the encounter
// when none are left.
//
// This is R2 §3's "escape and sanctuary" seen from the other side --
// disengagement is real and supported, and walking out of reach is the whole
// of it in v0. What ENDS a chase is Pursuit.Release, which is still deferred:
// a pack that loses its fight keeps following, and that is correct until the
// resolver can kill something.
func (c *Combat) pruneOrEnd() {
	e := c.encounter

	// EVERY SURVIVOR IS A CORPSE, TESTED BEFORE THE REACH FILTER AND NOT
	// AFTER, and the order is a bug the review caught rather than taste.
	//
	// Filtering first turns "the last living one walked away" into
	// "everything is dead": drop the living straggler for being out of reach
	// and only corpses are left to look at. A fight the player survived
	// because a wolf lost its nerve would then be reported enemies_dead, and
	// ended_enemies_dead -- which the thirteenth playtest asserts on -- would
	// be counting the wrong thing.
	allDead := true

	for _, enemy := range e.enemies {
		if enemy != nil && !e.dead[enemy.WatcherID()] {
			allDead = false

			break
		}
	}

	if allDead {
		c.end("enemies_dead")

		return
	}

	// A CORPSE KEEPS ITS ROW EVEN IF IT DRIFTS OUT OF REACH. encounter.dead
	// promises the row lasts the encounter's life, and the resolver despawns
	// nothing -- a dead dog still has a live chase, so it can still be moved.
	kept := e.enemies[:0]

	for _, enemy := range e.enemies {
		if e.dead[enemy.WatcherID()] || c.inReach(enemy, e.target) {
			kept = append(kept, enemy)
		}
	}

	e.enemies = kept

	// Disengagement is now "nothing ALIVE is still in reach" -- corpses do not
	// keep a fight open, and allDead above has already ruled out the case
	// where there was never anything alive to leave.
	living := false

	for _, enemy := range e.enemies {
		if !e.dead[enemy.WatcherID()] {
			living = true

			break
		}
	}

	if !living {
		c.end("disengaged")

		return
	}

	// Keep the reported order honest: an id that left the fight must leave the
	// order with it, or a script reading `order` sees a participant that is
	// no longer there.
	live := make(map[string]bool, len(e.enemies))
	for _, enemy := range e.enemies {
		live[enemy.WatcherID()] = true
	}

	order := e.enemyOrder[:0]

	for _, id := range e.enemyOrder {
		if live[id] && !e.dead[id] {
			order = append(order, id)
		}
	}

	e.enemyOrder = order
}

// end closes the encounter and records WHY, both as the last reason and as a
// counter. An encounter that starts and ends between two harness reads is
// invisible in the state and obvious in the counters -- the same argument the
// four counters at the top of this file were added for.
func (c *Combat) end(reason string) {
	c.encounter = nil
	c.ended++
	c.endedReason = reason

	switch reason {
	case "enemies_dead":
		c.endedEnemiesDead++
	case "player_dead":
		c.endedPlayerDead++
	case "disengaged":
		c.endedDisengaged++
	}
}

// inReach is the adjacency test, on floored tile positions.
//
// Chebyshev rather than Euclidean, and the difference is deliberate: R2 §3's
// zone-of-control is stated in adjacency terms, the map is a tile grid, and a
// diagonal neighbour is as adjacent as an orthogonal one. Pursuit's
// ArriveWithin is a Euclidean 1.0 and answers a different question -- see the
// AdjacentTiles dial.
func (c *Combat) inReach(w Combatant, q Quarry) bool {
	if w == nil || q == nil {
		return false
	}

	wx, wy := w.WatcherAt()
	qx, qy := q.QuarryAt()

	dx := int(math.Abs(math.Floor(wx) - math.Floor(qx)))
	dy := int(math.Abs(math.Floor(wy) - math.Floor(qy)))

	return dx <= c.dials.AdjacentTiles && dy <= c.dials.AdjacentTiles
}

// Fighting reports whether an encounter is live. It is the seam a resolver,
// a HUD or a script asks about, and it exists so that the answer does not
// have to be read out of the harness state -- the mistake Pursuit's `arrived`
// made, where the fact is reported and no Go caller can reach it.
func (c *Combat) Fighting() bool { return c.encounter != nil }

// Round reports the current round, or 0 when nothing is happening.
func (c *Combat) Round() int {
	if c.encounter == nil {
		return 0
	}

	return c.encounter.round
}

// Order reports the activation sequence for the CURRENT round, the player's
// own id included.
//
// ONE ORDER, NOT TWO, and the format CHANGED at step 4. Step 2 reported the
// enemies alone because there was nothing else to activate; D8 §9 puts the
// player in the sequence, and keeping a second list for "and then the player"
// is the two-copies disease this codebase keeps treating. The assertions in
// combat_test.go that pinned the enemy-only format moved in the same commit.
func (c *Combat) Order() []string {
	if c.encounter == nil {
		return nil
	}

	return c.encounter.activation()
}

// --- the harness provider -------------------------------------------------
//
// Built before the resolver, and this is the part the note argued for. What it
// must report is fixed by what the milestone has to prove: R2 §3's resolution
// bands and its dark-into-light rule are UNASSERTABLE unless the band a roll
// landed in and the reason an attack had advantage are reported, because
// otherwise a script can see that damage happened and not that it happened for
// the signed reason. Those two fields arrive with the resolver; the encounter
// shape is here so they have somewhere to land.

// HarnessName is the provider's name. d2app/harness_providers.go already
// reserved "combat" against M4.5, so the entry is removed from the planned
// map in the same commit this registers.
func (c *Combat) HarnessName() string { return "combat" }

// HarnessState reports the encounter.
func (c *Combat) HarnessState() map[string]interface{} {
	state := map[string]interface{}{
		"fighting":       c.encounter != nil,
		"encounter":      "",
		"round":          0,
		"order":          []string{},
		"participants":   []map[string]interface{}{},
		"encounters":     c.started,
		"ended":          c.ended,
		"rounds":         c.rounds,
		"declined_reach": c.declines,
		"round_minutes":  c.dials.RoundMinutes,
		"adjacent_tiles": c.dials.AdjacentTiles,
		"has_notice":     c.notice != nil,
		"has_fitness":    c.fitness != nil,
		"has_bodies":     c.bodies != nil,
		"has_profiles":   c.profiles != nil,
		"has_animator":   c.animator != nil,
		"bodies_known":   0,

		// The resolver's facts about the LAST encounter, reported whether or
		// not one is running: a fight that begins and ends inside one step is
		// otherwise invisible.
		"ended_reason":       c.endedReason,
		"ended_enemies_dead": c.endedEnemiesDead,
		"ended_player_dead":  c.endedPlayerDead,
		"ended_disengaged":   c.endedDisengaged,
		"actions_total":      c.actions,
		"actions_round":      c.actionsRound,
		"actions":            c.actionRows(),

		// Empty until an encounter fills them in below, so that a script
		// never has to tell "absent" from "no fight" (A3).
		"initiator":    "",
		"surprised":    false,
		"surprise_why": "",
		"first_side":   "",

		// Every dial a script can set, read back, so an assertion can prove
		// the write took rather than trusting it. Third provider rule.
		"dials": map[string]interface{}{
			"graze_band":      c.dials.GrazeBand,
			"crit_band":       c.dials.CritBand,
			"hit_band":        c.hitBand(),
			"graze_factor":    c.dials.GrazeFactor,
			"hit_factor":      c.dials.HitFactor,
			"crit_factor":     c.dials.CritFactor,
			"advantage_shift": c.dials.AdvantageShift,
			"shaken_penalty":  c.dials.ShakenPenalty,
			"lit_level":       c.dials.LitLevel,
			"player_action":   c.dials.PlayerAction,
			"forced_band":     c.dials.ForcedBand,
		},
	}

	if c.bodies != nil {
		state["bodies_known"] = c.bodies.BodiesKnown()
	}

	if c.clock != nil {
		state["stage"] = c.clock.Stage().String()
	}

	if c.encounter == nil {
		return state
	}

	e := c.encounter

	state["encounter"] = e.id
	state["round"] = e.round
	state["order"] = e.activation()
	state["minutes_into_round"] = e.sinceTurn
	state["initiator"] = e.initiator
	state["surprised"] = e.surprised
	state["surprise_why"] = e.surpriseWhy
	state["first_side"] = e.firstSide()

	parts := make([]map[string]interface{}, 0, len(e.enemies)+1)

	// The player side first, and it carries the two facts M4.2 built for this
	// milestone to read. They are reported here as well as on the meters
	// provider deliberately: an assertion about a FIGHT should be readable
	// from the fight, not assembled from two providers by the script.
	if e.target != nil {
		px, py := e.target.QuarryAt()

		melee := playerProfile()

		row := map[string]interface{}{
			"id":       e.target.QuarryID(),
			"side":     "player",
			"x":        px,
			"y":        py,
			"adjacent": true,

			// The note's "one clearly-labelled placeholder melee profile"
			// (M4.5 ask 1, signed as a LABEL; E3 fills in the numbers). It is
			// reported so that a script asserting on a damage draw checks it
			// against the profile the system says it used, rather than
			// against a constant the script also chose.
			"profile":    melee.Row,
			"damage_min": melee.DamageMin,
			"damage_max": melee.DamageMax,

			// NO health, deliberately, and combat_body_test.go:208-209 pins
			// the absence: the player's health is the meters', and one truth
			// with two homes is the disease. A blow on the player is
			// evidenced by its action row's target_health_after, which is the
			// blow's own fact rather than a second copy of the meter.
		}

		if c.illum != nil {
			row["light_here"] = c.illum.Level(int(math.Floor(px)), int(math.Floor(py)))
		}

		if c.fitness != nil {
			row["reaction_available"] = c.fitness.ReactionAvailable()
			row["shaken"] = c.fitness.Shaken()
		}

		parts = append(parts, row)
	}

	for _, enemy := range e.enemies {
		ex, ey := enemy.WatcherAt()

		profile := c.profileOf(enemy.WatcherID())

		row := map[string]interface{}{
			"id":       enemy.WatcherID(),
			"side":     "enemy",
			"x":        ex,
			"y":        ey,
			"adjacent": c.inReach(enemy, e.target),

			// What it fights as. `profile` reads "placeholder" for anything
			// the spawn tables did not place -- a harness- or terminal-spawned
			// NPC has no row -- so a script can never mistake the fallback for
			// a table monster.
			"profile":    profile.Row,
			"pack":       profile.Group,
			"speed":      profile.Speed,
			"damage_min": profile.DamageMin,
			"damage_max": profile.DamageMax,

			// A dead enemy KEEPS its row for the encounter's life and leaves
			// the order. The resolver despawns nothing (the fence): it stays
			// on the map in DD, stays in its spawn group, and its chase still
			// exists. Clearing all three is step 5's.
			"dead": e.dead[enemy.WatcherID()],
		}

		if c.illum != nil {
			row["light_here"] = c.illum.Level(int(math.Floor(ex)), int(math.Floor(ey)))
		}

		// M4.5 step 3: the enemy has a body, and a script can now assert on
		// it. has_body is reported separately and always, so that a monster
		// the game screen never adopted reads as "no body known" rather than
		// as a monster on zero health -- see the Bodies doc comment.
		row["has_body"] = false

		if c.bodies != nil {
			if body := c.bodies.BodyOf(enemy.WatcherID()); body != nil {
				row["has_body"] = true
				row["health"] = body.CurrentHealth()
				row["max_health"] = body.MaxHealth()
			}
		}

		parts = append(parts, row)
	}

	state["participants"] = parts

	return state
}

// HarnessSettableFields lists what a script may write.
//
// There is no "start" verb, and its absence is the same call Spawns made about
// spawning: a script that could conjure an encounter would prove something the
// game never does. A fight starts because something aware of you got within
// reach, and a script arranges that by spawning and stepping the clock, which
// exercises the real path.
// Every resolver dial is settable, and that is the third provider rule rather
// than generosity: an assertion that can only be made in one direction is not
// an assertion. A script proves dark-into-light by moving lit_level until the
// verdict flips, and proves the band arithmetic by forcing a band -- which is
// the ONLY outcome-steering verb there is, for the reasons on ForcedBand.
//
// STILL NOT SETTABLE, AND THE OMISSIONS ARE THE ARGUMENT: no health, no blow,
// no animation, no start. A script that could set a wolf to 1 HP would prove
// nothing about the resolver; one that could land a blow would prove nothing
// about the game; one that could set A1 would prove nothing about the
// held-mode path. Nor are the rows' Speed and damage or the two placeholder
// profiles settable -- they are CONTENT, and a script that wants a different
// bite spawns a different row.
func (c *Combat) HarnessSettableFields() []string {
	return []string{
		"adjacent_tiles", "advantage_shift", "crit_band", "crit_factor",
		"disengage", "forced_band", "graze_band", "graze_factor", "hit_factor",
		"lit_level", "player_action", "round", "round_minutes", "shaken_penalty",
	}
}

// HarnessSet writes one allow-listed field.
func (c *Combat) HarnessSet(field string, value interface{}) error {
	switch field {
	case "round_minutes":
		// FLOORED, NOT MERELY POSITIVE. Advance resolves rounds in a loop
		// that subtracts this from an accumulator, so a value below the
		// accumulator's precision never shortens it and the loop cannot
		// terminate -- the review hung a test at 1e-17 after 23 million
		// rounds. Well short of that it is still nonsense rather than a dial:
		// 1e-4 resolves ten thousand rounds inside one stepped world minute,
		// so every per-round assertion a script makes measures noise.
		v, ok := value.(float64)
		if !ok || v < minRoundMinutes {
			return fmt.Errorf("round_minutes wants a number >= %v, got %v", minRoundMinutes, value)
		}

		c.dials.RoundMinutes = v

	case "adjacent_tiles":
		v, ok := value.(float64)
		if !ok || v < 0 {
			return fmt.Errorf("adjacent_tiles wants a non-negative number, got %v", value)
		}

		c.dials.AdjacentTiles = int(v)

	case "round":
		// A value a script can only ever watch rise is a value it cannot
		// assert against cheaply -- the third provider rule. Setting the
		// round lets a torch-burn-per-round assertion reach round twenty
		// without stepping twenty world minutes of everything else.
		v, ok := value.(float64)
		if !ok || v < 1 {
			return fmt.Errorf("round wants a number >= 1, got %v", value)
		}

		if c.encounter == nil {
			return fmt.Errorf("no encounter is running")
		}

		c.encounter.round = int(v)

	case "disengage":
		// The emptying half of the first provider rule. A collection needs a
		// verb that can put something in it and one that can take it back out,
		// and here the filling half is deliberately the game's alone.
		if c.encounter == nil {
			return fmt.Errorf("no encounter is running")
		}

		c.end("disengaged")

	case "graze_band", "crit_band":
		v, ok := toFloat(value)
		if !ok || v < 0 || v > 100 {
			return fmt.Errorf("%s wants a number 0-100, got %v", field, value)
		}

		graze, crit := c.dials.GrazeBand, c.dials.CritBand
		if field == "graze_band" {
			graze = int(v)
		} else {
			crit = int(v)
		}

		// The middle band is derived, so the two ends must leave one. A pair
		// that sums past 100 would make `hit` negative and every roll a crit,
		// which is a silently wrong fight rather than a refused write.
		if graze+crit > 100 {
			return fmt.Errorf("graze_band %d + crit_band %d leaves no room for a hit", graze, crit)
		}

		c.dials.GrazeBand, c.dials.CritBand = graze, crit

	case "graze_factor", "hit_factor", "crit_factor":
		v, ok := toFloat(value)
		if !ok || v < 0 {
			return fmt.Errorf("%s wants a non-negative number, got %v", field, value)
		}

		switch field {
		case "graze_factor":
			c.dials.GrazeFactor = v
		case "hit_factor":
			c.dials.HitFactor = v
		case "crit_factor":
			c.dials.CritFactor = v
		}

	case "advantage_shift", "shaken_penalty":
		v, ok := toFloat(value)
		if !ok || v < 0 || v > 100 {
			return fmt.Errorf("%s wants a magnitude 0-100, got %v", field, value)
		}

		if field == "advantage_shift" {
			c.dials.AdvantageShift = int(v)
		} else {
			c.dials.ShakenPenalty = int(v)
		}

	case "lit_level":
		v, ok := toFloat(value)
		if !ok || v < 0 || v > 1 {
			return fmt.Errorf("lit_level wants a light level 0-1, got %v", value)
		}

		c.dials.LitLevel = v

	case "player_action":
		kind, ok := value.(string)
		if !ok || (kind != PlayerActionAttack && kind != PlayerActionHold) {
			return fmt.Errorf("player_action wants %q or %q, got %v", PlayerActionAttack, PlayerActionHold, value)
		}

		c.dials.PlayerAction = kind

	case "forced_band":
		kind, ok := value.(string)
		if !ok {
			return fmt.Errorf("forced_band wants a string (graze, hit, crit, or empty), got %T", value)
		}

		switch kind {
		case "", BandGraze, BandHit, BandCrit:
			c.dials.ForcedBand = kind
		default:
			return fmt.Errorf("no band %q (graze, hit, crit, or empty to clear)", kind)
		}

	default:
		return fmt.Errorf("combat has no settable field %q", field)
	}

	return nil
}
