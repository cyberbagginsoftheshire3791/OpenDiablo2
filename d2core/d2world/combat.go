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
	fitness  FitnessSource
	illum    Illumination
	bodies   Bodies
	profiles Profiles
	animator Animator
	morale   Morale
	chases   Chases

	// kits is what each combatant carries (T2). Nil is legal: no gear.
	kits Kits

	// edges are his talents as numbers (T3). Nil is legal: no talents.
	edges Edges

	// corpses is where the dead lie (M4.7).
	corpses *Corpses

	// xpEvents is what fights did that earns experience, until the game
	// screen takes it (T3).
	xpEvents []XPEvent

	// killerIsPlayer is whether the blow being resolved is his, so a death
	// can charge the pack his What Breaks Them nerve (T3).
	killerIsPlayer bool

	// stepper moves a body on its turn in a paced fight (T1). Nil is legal:
	// nothing steps, and an enemy out of reach waits where it stands.
	stepper Stepper

	// owedMinutes is the world time closed paced rounds have not yet been
	// paid for; the game screen takes it with TakeRoundMinutes.
	owedMinutes float64

	// blowLog is the HUD's rolling log of the last few blows, fed by
	// resolveBlow. It is separate from lastActions, which is one round's.
	blowLog []BlowLine

	// stepsOrdered counts the walks a paced fight ordered -- the evidence that
	// a pack actually moved on its turn rather than waiting to be reached.
	stepsOrdered int

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
	started int
	ended   int
	rounds  int

	// commitsRefused counts commits that arrived with no turn waiting, or with
	// the wrong choice for the turn's state. It is reported, because a script
	// or a game screen sending the wrong verb should be visible rather than
	// silently absorbed.
	commitsRefused int

	// The decision timer. decisionSecondsRound resets at each turn-open;
	// decisionSeconds is the fight's total.
	//
	// IT TAKES A DELTA AND NEVER READS A WALL CLOCK. A time.Now() anywhere in
	// a provider would break determinism_test.go, which JSON-marshals every
	// provider and compares two launches of one build. Under a script the
	// delta is the stepped dt, so N frames read exactly N*dt; in a live build
	// it is the loop's elapsed, which IS wall time -- clamped at 0.25 s per
	// frame and timescale-scaled (gameclock.go:21-35). The clamp is
	// deliberate: a closed laptop lid is not a decision.
	decisionSeconds      float64
	decisionSecondsRound float64

	// Which path a commit arrived by. The playtest's primary path is the real
	// one -- strigoi_key through the input overlay, exactly what a friend
	// presses -- and the settable field exists for scripts that are not
	// testing input. Counting both is what stops one standing in for the
	// other.
	commitsByInput int
	commitsByField int

	// The pace window. It opens at the FIRST turn-open of a fight -- not at
	// tryStart -- because a fight the player never acted in measured nothing
	// about a person, and closes at end().
	paceOpen       bool
	wallSeconds    float64
	paceHealthOpen int
	paceCommits    int
	// The turn's ACTION, not the verb that closed it. Measured: reporting the
	// closing verb made every ROUND line read action=end, because the end
	// commit overwrote the strike a moment before finishRound captured it.
	// What the line has to answer is "what did he DO this round": strike,
	// light and douse set it; hold sets it to hold (the Action went unspent,
	// which IS the answer); end leaves it alone, because end is only
	// reachable once an Action is already recorded.
	lastActionVerb string
	lastRound      RoundRow
	lastPace       PaceRow
	declines       int
	actions        int

	// endedReason is why the LAST encounter ended, and it persists after the
	// encounter is gone -- an encounter that begins and ends between two
	// harness reads is otherwise invisible. The three counters beside it are
	// what make it countable rather than merely last-seen.
	endedReason      string
	endedEnemiesDead int
	endedDawn        int // M4.7 step 3: fights the dead left at first light
	endedPlayerDead  int
	endedDisengaged  int
	endedRouted      int

	// joined is how many enemies have arrived into a fight already running.
	// Reinforcements are otherwise invisible: a script can watch the
	// participant list grow, but only if it happens to read on the right
	// tick, and end() would report the fight identically either way.
	joined int

	// quickResolved counts the fights finished in one action, and
	// lastQuickAdvantage is the advantage figure that triggered the last one.
	// The dial is R2's to answer, so the build's job is to make what it did
	// legible rather than to defend the number.
	quickResolved      int
	lastQuickAdvantage float64
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

// Morale is what the combat model may do to a pack's nerve, and what it may
// ask about it. Two methods, and the split between them is the milestone
// boundary: M4.3b owns the morale STATE and the rout THRESHOLD, this owns the
// behaviour that moves the number and the behaviour that reads it.
//
// IT IS A FOURTH NARROW INTERFACE rather than a *Spawns, for the reason
// Fitness, Bodies and Profiles are: the fence is in the type system instead of
// in a comment, and the model can be tested with no spawn tables at all. It is
// deliberately NOT three methods -- the earlier draft asked for a GroupOf, and
// that symbol was already refused by name at step 4 because ProfileOf answers
// it (spawns.go), and Profile.Group is how every caller here already gets a
// pack key.
//
// A nil Morale is legal and means a fight against something with no pack
// behind it: nothing routs, and nothing quick-resolves. Reported as
// has_morale, on the has_notice precedent, so a playtest can tell "the wiring
// is missing" from "the rule did not fire".
type Morale interface {
	// Hurt takes morale off a group. It is the decrement, not a write.
	Hurt(groupID string, amount float64) bool

	// Morale is the number and whether the group is known at all. The second
	// return is load-bearing: an enemy the tables never placed is not an
	// enemy with zero morale, and quick-resolve's mundane test turns on
	// exactly that distinction.
	Morale(groupID string) (morale float64, known bool)

	// Routing is M4.3b's rout THRESHOLD, read rather than recomputed.
	//
	// THE BRIEF ASKED FOR TWO METHODS AND THIS IS THE THIRD, deliberately.
	// The threshold is a spawn dial (RoutAt, settable at runtime), so testing
	// morale <= 25 on this side would be a second home for one truth and
	// would silently stop agreeing the first time a script tuned the dial.
	// The ask that was signed refused a GroupOf, because ProfileOf already
	// answers it, and that refusal stands -- this is not that symbol. It is
	// also the clause M4.5 §3.9 needs: Spawns.Routing is one of the seven
	// rows the milestone signs must all flip to wire, and a flag nothing
	// reads cannot flip.
	Routing(groupID string) (routing, known bool)
}

// Chases is how the combat model ends a chase, and it exists because
// RELEASING A DEAD HUNTER IS NOT ENOUGH ON ITS OWN.
//
// The game screen calls startChasesForTheAware() every frame, immediately
// before Advance, and that function has no liveness filter of any kind: it
// walks AwarePairs() and chases anything not already chasing. So a Release on
// death is undone on the very next frame unless the same death also stops the
// watching. Both halves land together here -- Release through this interface,
// Unwatch through the notice model this system already holds.
//
// A nil Chases means nothing is released, which is exactly what every build
// before step 5 did.
type Chases interface {
	Release(hunterID string) bool
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
	// It is SEPARATE from Pursuit's ArriveWithin (1.5, a Euclidean distance)
	// on purpose, and the note's ask 6 says why: the router targets the eight
	// neighbour tiles, so "arrived" and "in reach" are two facts that happen
	// to agree today. One overloaded dial would hide the day they stop
	// agreeing.
	//
	// MEASURED AT STEP 4: ask 6's REASON for that agreement was wrong about
	// the map. A quarry's footprint IS routable -- entities do not block the
	// search -- so the router's exact-tile attempt succeeded every time, a
	// pursuer walked onto the player's own tile and stopped there at distance
	// 0.000 rather than beside him, and every participant in a settled fight
	// floored to ONE tile and sampled ONE light level.
	//
	// FIXED BY M4.5 ask 8 (mapRouter.Route, d2game/d2gamescreen/game.go): the
	// eight neighbours are now tried NEAREST FIRST and the quarry's own tile
	// is only the fallback, and ArriveWithin moved 1.0 -> 1.5 with it because
	// a diagonal stop sits at 1.414. Participants therefore stand on
	// DIFFERENT tiles and can sample different light levels. That does not on
	// its own make dark-into-light FIRE -- at night the level is uniform, so
	// observing it in a real build needs a PLACED source between two
	// participants -- but it is the one change that made light, flank and
	// facing possible to assert at all.
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
	// verb in the engine, no turn UI until M4.4c-2 (M4.4a shipped the clock
	// strip only; the hands are M4.4c-2), and no blow tool in the
	// harness by design -- so without a policy a real build's fight has only
	// one side acting and every fight ends player_dead, which is "you can
	// lose" only in the sense that a wall can lose. The turn UI replaces the
	// policy with a choice; until then a script reading actions[] can see
	// that the player's blow came from the policy rather than from a person.
	PlayerAction string

	// PlayerControl decides whether a round waits at the player's slot.
	// DefaultCombatDials keeps "policy" so the resolver's own unit tests stand
	// as written; the game screen sets "human" at construction, so the shipped
	// build is the one that waits. The harness reports which it was.
	PlayerControl string

	// AutoEndTurn ends the turn when BOTH Move and Action are spent. With it
	// false, only hold or end close a turn. It is a [DIAL] because the first
	// human night is what says whether an automatic close feels like losing
	// the turn or like being spared a keystroke.
	AutoEndTurn bool

	// ForcedBand pins every blow's band to "graze", "hit" or "crit" ("" is
	// off). BOTH RNG DRAWS STILL HAPPEN when it is set, so forcing one blow
	// does not shift the sequence the next one sees.
	//
	// It is the ONLY way a script can steer an outcome, and that is the whole
	// argument of §5.3: a script that could set health, land a blow or set an
	// animation would prove something the game never does.
	ForcedBand string

	// --- step 5: rout and quick-resolve. Both are [DIAL]s and both are
	// visible on purpose; R2 §5's header says the prototype answers these. ---

	// LossWeight scales what one death costs a pack's nerve. The decrement is
	// LossWeight * 100 / the pack's STARTING count, so losing one of two is
	// half your pack and losing one of six is not.
	//
	// A FLAT DECREMENT WAS TRIED ON PAPER AND IS REFUTED BY THE AUTHORED
	// ROWS. Dogs start at 50 and wolves at 60 against a rout threshold of 25,
	// so any flat value large enough to break a wolf pack breaks a dog pack
	// on the same loss -- the ten-point gap between the rows is smaller than
	// the decrement, so it can never move the answer. Worse, it makes a lone
	// boar (morale 70) unable to rout at ANY value that leaves dogs alive.
	//
	// At 1.0 the authored rows reproduce all three of N1 §5's phrasings: a
	// pack of two or four dogs breaks on loss one ("rout early"), three
	// wolves on loss two and six wolves on loss three ("rout at losses"), and
	// a SOLITARY boar never breaks at all -- which is N1's own "dangerous
	// cornered", the corpus answering rather than this dial inventing.
	LossWeight float64

	// QuickResolveAdvantage is how far ahead the player must be before a
	// fight against mundane animals can be finished in one action.
	//
	// THE DIAL IS R2 §5's, NOT THIS MILESTONE'S: its list of open dials names
	// "quick-resolve advantage threshold" outright, and the signed M4.5 note
	// already carries QuickResolveAdvantage in its own dial table. So the
	// build ships it with a visible starting value and the prototype answers
	// it; a brief that legislated the number would be answering a question
	// R2 reserved.
	//
	// It is measured as the fraction of the enemy side already gone: 0.75
	// means three of a four-dog pack are down. Never against the dead -- see
	// mundane(), and see the DoD for why that half is a named deferral.
	QuickResolveAdvantage float64

	// --- T1, the tactical layer (23 Sep 2026). See combat_tactical.go. ---

	// Paced runs a HUMAN-controlled fight on real seconds, one visible beat at
	// a time, and charges each closed round RoundMinutes of world time instead
	// of letting world time pace the rounds. Off in DefaultCombatDials, on in
	// the shipped game.
	Paced bool

	// EngageTiles is how close an aware enemy must be for a fight to OPEN. At
	// or below AdjacentTiles it is adjacency, which is what every build before
	// T1 did. DisengageTiles is how far away everything alive must be for a
	// fight to END; at or below the engage radius it is the engage radius.
	EngageTiles, DisengageTiles int

	// EnemyMoveTiles is how far an enemy walks on its own turn in a paced
	// fight. Zero means enemies do not step (the world-time path's behaviour).
	EnemyMoveTiles int

	// MoveTiles is the player's Move in tiles (the c-2 ruling: 2, and a click
	// beyond it is REFUSED, not clamped). The game screen enforces it -- the
	// route is the map's -- and reads it from here so there is one number.
	MoveTiles int
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

		// POLICY, deliberately: the world package's ~40 resolver tests at four
		// construction sites stand as written, and the game screen sets human.
		PlayerControl: PlayerControlPolicy,
		AutoEndTurn:   true,

		LossWeight:            1.0,
		QuickResolveAdvantage: 0.75,

		// T1 is OFF here: the tactical layer is the game screen's choice, and
		// the resolver's unit tests describe the world-time path.
		MoveTiles: 2,
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

	// routed is every enemy whose PACK broke. It is a third set beside dead
	// rather than a removal from enemies, and the review that caught the
	// alternative is the reason.
	//
	// REMOVING A ROUTER FROM enemies REPORTS EVERY ROUT AS A KILL. pruneOrEnd
	// opens by looping the slice to see whether everything is dead, and a
	// loop over an EMPTY slice leaves allDead true -- so in the single-pack
	// case, which is the commonest fight in the game, the list empties and
	// the encounter ends "enemies_dead". Removing only the LIVING routers
	// fails the same way: the slice then holds corpses only, all of them in
	// dead, and allDead is true again. The comment three lines into
	// pruneOrEnd names that exact scenario as the defect step 4 was fixed to
	// prevent.
	//
	// Keeping them and marking them is what works, and it costs one clause in
	// three places: the all-gone test, the disengagement test and the order.
	// A routed enemy takes no activation, keeps its participant row with
	// routed:true, and STAYS STANDING ON THE MAP -- making it flee needs a
	// destination, a router call per member per round and a rule for
	// re-noticing, which is a named deferral (ask 2) rather than something
	// half-built here. A wolf that stands still reads as a bug in a
	// screenshot; a wolf that evaporates is a worse lie.
	routed map[string]bool

	// broke is who left at first light (M4.7 step 3): the dead.
	broke map[string]bool

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

	// blockUsedInRound is the round his shield last turned a blow (T2): one a
	// round, like the Reaction.
	blockUsedInRound int

	// T3: how many Reactions and blocks the round has spent -- a talent can
	// raise both caps above one.
	reactionsInRound int
	blocksInRound    int

	round     int
	sinceTurn float64

	// --- M4.4c-2a: the seam. The round that stops and waits. ---------------
	//
	// sequence is the round's activation order, FROZEN at the top of the round
	// and walked from cursor. It is stored rather than recomputed because the
	// player's slot moves between rounds -- a surprised round one puts him
	// last -- so a resumed walk that re-read activation() would index a
	// different list than the one it stopped in.
	sequence []string
	cursor   int

	// awaiting means THE PLAYER'S TURN IS OPEN and the world sim is frozen
	// with it. It is not "waiting for an Action": it stays true after a strike
	// while the Move is still unspent, because the turn is not over and the
	// world must not run. Game.worldRunning() reads it.
	awaiting bool

	// The turn's economy, reset at the top of each round. AutoEndTurn ends the
	// turn when BOTH are spent -- the ruling's words, "after Move + Action" --
	// so a turn with the Action spent and the Move unspent WAITS for a Move
	// click or the End key, and Move-after-Action stays reachable.
	moveSpent   bool
	actionSpent bool

	// --- T1: the paced round's state. Tick walks it; see combat_tactical.go.
	// roundOpen is a paced round with a frozen sequence; acting is the pack
	// whose activation is running; stepping is the members ordered to walk
	// and not yet arrived; strikers is who strikes next, one per beat; beat
	// is the pause still owed; stepWait is how long the walk has taken.
	roundOpen  bool
	playerWait float64
	acting     []string
	stepping   []string
	strikers   []string
	beat       float64
	stepWait   float64
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
// profiles, animator, morale and chases may all be nil, and the model copes
// rather than assuming: a nil Profiles means every enemy fights on the
// placeholder profile, a nil Animator means nothing swings on screen, a nil
// Morale means nothing routs and nothing quick-resolves, and a nil Chases
// means a dead hunter keeps its chase. All four are reported (has_profiles,
// has_animator, has_morale, has_chases) on the has_notice precedent, so a
// playtest can tell "the wiring is missing" from "the rule did not fire".
func NewCombat(clock *Clock, notice *Notice, fitness FitnessSource, illum Illumination,
	bodies Bodies, profiles Profiles, animator Animator, morale Morale, chases Chases,
	seed int64, dials CombatDials) *Combat {
	c := &Combat{
		dials:    dials,
		clock:    clock,
		notice:   notice,
		fitness:  fitness,
		illum:    illum,
		bodies:   bodies,
		profiles: profiles,
		animator: animator,
		morale:   morale,
		chases:   chases,
		rng:      rand.New(rand.NewSource(seed)), // nolint:gosec // gameplay RNG, seeded for reproducibility
		nextID:   1,
	}

	d2harness.Register(c)

	return c
}

// Close unregisters the provider.
func (c *Combat) Close() { d2harness.Unregister(c) }

// fitnessOf looks up a combatant's Fitness by entity id (M4.4c-1 clause 4). It
// replaces the singular fitness field's five read sites: with squads on the
// map, "the player's fatigue" is a lookup, not a handle. It is nil-safe both
// ways -- a nil source (every unit test above the resolver still passes one)
// and an id no squad owns both return nil, which each site handles exactly as
// it handled a nil fitness before.
func (c *Combat) fitnessOf(id string) Fitness {
	if c.fitness == nil {
		return nil
	}

	return c.fitness.FitnessOf(id)
}

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

	// A PACED FIGHT IS NOT RUN BY WORLD TIME. Tick drives it and each closed
	// round is paid for through TakeRoundMinutes -- which is how this call
	// happens during a paced fight at all: the game advances the world by the
	// round's minute, and that minute must not also resolve rounds here.
	if c.paced() {
		return
	}

	// A waiting turn freezes the encounter: no world time accumulates, no
	// round resolves, no tail runs. The gate in Game.worldRunning() means
	// Advance is not normally reached at all while awaiting -- this is the
	// second lock, so a caller that steps combat directly cannot run the world
	// out from under an open turn either.
	if c.encounter.awaiting {
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

		// The round stopped at the player's slot. Nothing is counted and the
		// tail does not run: the round is half-resolved and stays that way
		// until a commit closes it. finishRound and the tail are reached from
		// endTurn instead -- the same code, entered from a different line.
		if c.encounter != nil && c.encounter.awaiting {
			return
		}

		c.finishRound()

		// A blow can end the encounter -- the player died, or that was the
		// last of them. The remaining world minutes of that step buy no
		// further rounds.
		if c.encounter == nil {
			break
		}
	}

	if c.encounter != nil {
		c.reinforce()
		c.pruneOrEnd()
	}
}

// finishRound closes the round that just resolved: it counts the round entered
// and moves the encounter to the next one. It is the second half of what the
// Advance loop used to do inline, lifted out because it is now reached from
// TWO lines -- Advance, when a policy round walks straight through, and
// endTurn, when the player closes a turn that stopped part-way.
//
// The counters mean exactly what they meant before the seam: rounds is rounds
// ENTERED (tryStart counts round 1 and each close counts the next), and it is
// incremented even when the round that just ran ended the fight.
func (c *Combat) finishRound() {
	// Captured BEFORE the counters move, so the row names the round that just
	// closed rather than the one about to run.
	if c.encounter != nil {
		c.lastRound = RoundRow{
			Encounter:     c.encounter.id,
			Round:         c.encounter.round,
			DecideSeconds: c.decisionSecondsRound,
			Action:        c.lastActionVerb,
			Move:          c.encounter.moveSpent,
		}
	}

	c.lastActionVerb = ""

	c.rounds++

	if c.encounter != nil {
		c.encounter.round++
	}
}

// ActionSpent reports whether the open turn's Action is gone. It is what lets
// ONE key mean two things: E sends hold while this is false and end once it is
// true, so the player never learns there are two verbs.
func (c *Combat) ActionSpent() bool {
	return c.encounter != nil && c.encounter.actionSpent
}

// MoveSpent reports whether the open turn's Move is gone.
func (c *Combat) MoveSpent() bool {
	return c.encounter != nil && c.encounter.moveSpent
}

// RoundRow is what one closed round was, captured at the close. The game
// screen writes a ROUND line from it: the round edge is Round() changing, and
// this is the row that goes with the edge.
type RoundRow struct {
	Encounter     string
	Round         int
	DecideSeconds float64
	Action        string
	Move          bool
}

// PaceRow is what one whole fight was, captured at end(). It is the row R2 §4
// is measured against -- the night's total is the sum of WallSeconds over one
// day -- and it is built here rather than on the game screen because every
// number in it except the clock stamps is this package's own fact.
//
// WallSeconds runs from the FIRST TURN-OPEN to end(), not from tryStart: a
// fight the player never got a turn in measured nothing about a person.
type PaceRow struct {
	Encounter     string
	Rounds        int
	WallSeconds   float64
	DecideSeconds float64
	HealthOpen    int
	HealthClose   int
	EndReason     string
	Enemies       int
	Kinds         []string
	Initiator     string
	Surprised     bool
	Control       string
	Commits       int
}

// LastRound and LastPace are the two records, read by the game screen at the
// edges it already sees.
func (c *Combat) LastRound() RoundRow { return c.lastRound }

// LastPace is the closed fight's record; its Encounter is "" until one closes.
func (c *Combat) LastPace() PaceRow { return c.lastPace }

// Wait accumulates the player's thinking time. The game screen hands it the
// frame's elapsed while a turn is open; it does nothing at any other time, so
// a caller that forgets to check cannot poison the number.
func (c *Combat) Wait(elapsed float64) {
	if elapsed <= 0 || c.encounter == nil {
		return
	}

	if c.encounter.awaiting {
		c.decisionSeconds += elapsed
		c.decisionSecondsRound += elapsed

		// The first turn-open starts the fight's wall clock and pins the
		// health it opened on.
		if !c.paceOpen {
			c.paceOpen = true
			c.paceHealthOpen = c.playerHealth()
		}
	}

	// Wall time runs on EVERY frame once the window is open, not only the
	// awaiting ones: it is how long the fight took a person, and the frames
	// between rounds are part of that.
	if c.paceOpen {
		c.wallSeconds += elapsed
	}
}

// playerHealth reads the quarry's body, or -1 when there is nothing to read.
// The player's health lives on the meters and is NOT duplicated in the combat
// provider; this is the pace row's own snapshot, taken at two instants.
func (c *Combat) playerHealth() int {
	if c.encounter == nil || c.encounter.target == nil || c.bodies == nil {
		return -1
	}

	body := c.bodies.BodyOf(c.encounter.target.QuarryID())
	if body == nil {
		return -1
	}

	return body.CurrentHealth()
}

// DecisionSeconds and DecisionSecondsRound were DELETED on 18 September 2026,
// hours after they were written, because the reach gate measured them DEAD:
// nothing called them in either build. HarnessState reads decisionSeconds and
// decisionSecondsRound straight off the struct, in the same package, and the
// PACE line reads the totals off the pace row. Two exported accessors with no
// reader is the hollow-class shape this project has been caught by twice, and
// the honest fix for a symbol nobody calls is to remove it, not to find it a
// caller. If a reader ever appears outside d2world, they come back with it.

// Awaiting reports whether the player's turn is open. Game.worldRunning() is
// its one live reader, and the harness reports it.
func (c *Combat) Awaiting() bool {
	return c.encounter != nil && c.encounter.awaiting
}

// CommitsRefused counts commits that arrived with no turn waiting, or with the
// wrong choice for the turn's state. A refused commit resolves nothing.
//
// HarnessState reads the count THROUGH this method, as it does Awaiting and
// MoveSpent, so there is one reading path rather than two. That is also what
// keeps the row honest at harness-only: five seam tests assert on it, and a
// method whose only callers are tests is dead in both builds.
func (c *Combat) CommitsRefused() int { return c.commitsRefused }

// Commit is the player's choice, and the verb the whole seam exists for.
//
// EXECUTING THE ACTION IS NOT FINISHING THE TURN. strike, light and douse
// spend the Action and leave the turn OPEN when the Move is unspent -- the
// world stays frozen, the player can still walk, and either AutoEndTurn or an
// explicit end closes the round. hold and end close it outright.
//
// hold and end are two names for one key: E sends hold when the Action is
// unspent (its signed meaning -- "end the turn with the Action unspent") and
// end when it is spent. They are checked rather than aliased so that a game
// screen sending the wrong one is loud instead of silently fine.
// Commit is the INPUT path: a key press through the game controls. It counts
// separately from the settable field so that one cannot quietly stand in for
// the other -- the playtest's primary path is the real one a friend presses.
func (c *Combat) Commit(choice, targetID string) error {
	if err := c.commit(choice, targetID); err != nil {
		return err
	}

	c.commitsByInput++

	return nil
}

// commit is the one code path both callers share.
func (c *Combat) commit(choice, targetID string) error {
	e := c.encounter
	if e == nil || !e.awaiting {
		c.commitsRefused++

		return fmt.Errorf("commit %q refused: no turn is waiting", choice)
	}

	c.paceCommits++

	switch choice {
	case CommitStrike, CommitLight, CommitDouse, CommitStake, CommitHold:
		c.lastActionVerb = choice
	case CommitEnd:
		// end closes a turn whose Action is already spent, so the Action verb
		// is already recorded and must not be overwritten.
	}

	switch choice {
	case CommitStrike, CommitLight, CommitDouse, CommitStake:
		if e.actionSpent {
			c.commitsRefused++

			return fmt.Errorf("commit %q refused: the Action is already spent this turn", choice)
		}

		if choice == CommitStrike {
			c.commitStrike(targetID)
		}

		// light and douse resolve in the light model, which lives on the game
		// screen; what they spend is the same Action, and that is this
		// package's half of the verb.
		e.actionSpent = true

		// The encounter can end on the player's own blow.
		if c.encounter == nil {
			return nil
		}

		c.maybeEndTurn()

	case CommitHold:
		if e.actionSpent {
			c.commitsRefused++

			return fmt.Errorf("commit %q refused: the Action is spent -- %q closes a spent turn", choice, CommitEnd)
		}

		c.endTurn()

	case CommitEnd:
		if !e.actionSpent {
			c.commitsRefused++

			return fmt.Errorf("commit %q refused: the Action is unspent -- %q closes an unspent turn", choice, CommitHold)
		}

		c.endTurn()

	default:
		c.commitsRefused++

		return fmt.Errorf("commit %q refused: want one of %q, %q, %q, %q, %q, %q",
			choice, CommitStrike, CommitLight, CommitDouse, CommitStake, CommitHold, CommitEnd)
	}

	return nil
}

// SpendMove marks the player's Move spent. The game screen calls it when a
// walk is ordered during the player's own turn.
//
// It exists so that the turn-over check runs on the MOVE path too. Putting
// that check only inside Commit is the defect this seam was reconciled to
// avoid: every sentence in the design says a Move click can close a both-spent
// turn, and a Commit-only check cannot deliver it.
func (c *Combat) SpendMove() {
	e := c.encounter
	if e == nil || !e.awaiting {
		return
	}

	e.moveSpent = true

	c.maybeEndTurn()
}

// maybeEndTurn closes the turn if and only if AutoEndTurn's both-spent
// condition is met. It is the ONE place that decision is made, and it runs
// wherever a pip is spent -- at the end of Commit and on the Move path.
func (c *Combat) maybeEndTurn() {
	e := c.encounter
	if e == nil || !e.awaiting {
		return
	}

	if !c.dials.AutoEndTurn || !e.moveSpent || !e.actionSpent {
		return
	}

	c.endTurn()
}

// endTurn closes an open turn: it walks the rest of the frozen sequence -- the
// remaining enemy slots, liveness-tested -- then finishes the round and runs
// the tail. It is the same code Advance runs, entered from a different line.
func (c *Combat) endTurn() {
	e := c.encounter
	if e == nil || !e.awaiting {
		return
	}

	e.awaiting = false

	// Past the player's own slot, which the commit just resolved.
	e.cursor++

	// In a paced fight the rest of the round is Tick's, one visible beat at a
	// time, starting with a beat so his own blow is seen to land first.
	if c.paced() {
		e.beat = TacticalBeatSeconds

		return
	}

	c.walkSequence()
	c.finishRound()

	if c.encounter != nil {
		c.reinforce()
		c.pruneOrEnd()
	}
}

// scanAware is the aware-and-in-reach sweep, and it is a function rather than
// a loop inside tryStart because STEP 5 NEEDS IT TWICE: once to open a fight,
// and once per tick during a live one to find reinforcements.
//
// THE EXTRACTION STOPS EXACTLY HERE, AND THAT IS THE POINT. tryStart does four
// more things after the sweep -- it picks the quarry, reads D8's stance once
// and fixes it, ASSIGNS c.encounter, and resets the action log. Any refactor
// that let the reinforcement path reach the assignment would rebuild the fight
// every tick: round back to 1, dead and routed emptied, surprised recomputed,
// the activation order redrawn. So the sweep is lifted and the construction is
// not, and the reinforcement caller is handed the quarry rather than choosing
// one.
//
// A nil target means "choose one": the first aware pair in the stable order
// names the quarry and anything aware of somebody else waits, which is v0's
// dullest available answer to the target-selection question notice.go hands
// this milestone. A non-nil target means "these are the terms" -- the sweep
// may not repoint a fight that is already running at somebody.
//
// declined is returned rather than counted here so the caller decides whether
// it means anything; see tryStart.
func (c *Combat) scanAware(target Quarry) (enemies []Combatant, chosen Quarry, declined int) {
	if c.notice == nil {
		return nil, target, 0
	}

	for _, pair := range c.notice.AwarePairs() {
		if pair.Target == nil || pair.Watcher == nil {
			continue
		}

		// ENGAGE, not strike: at the defaults the two radii are the same
		// adjacency, and in a paced fight the approach is part of the fight.
		if !within(pair.Watcher, pair.Target, c.engageTiles()) {
			declined++

			continue
		}

		// A CORPSE DOES NOT START A FIGHT OR JOIN ONE, and this filter is
		// load-bearing rather than tidy. The sweep runs on every tick and
		// takes every noticed watcher. Step 5 now releases the chase and
		// stops the watching when something dies, so this is belt-and-braces
		// where it used to be the only thing standing -- AND IT MUST NOT BE
		// DELETED, because it also stops a fight restarting against a body
		// the registry never adopted.
		//
		// A watcher with NO body is alive by this test, which is the same
		// answer has_body:false already gives: it cannot be hurt and cannot
		// die, and "I do not know" must not read as "it is dead" (A3).
		if c.deadByBody(pair.Watcher.WatcherID()) {
			continue
		}

		if target == nil {
			target = pair.Target
		}

		if pair.Target.QuarryID() != target.QuarryID() {
			continue
		}

		enemies = append(enemies, pair.Watcher)
	}

	return enemies, target, declined
}

// reinforce is a monster arriving into a fight that is already running.
//
// UNTIL STEP 5 THE PARTICIPANT LIST COULD ONLY SHRINK: tryStart built it once
// and pruneOrEnd removed from it, so a second pack that noticed the player
// mid-fight stood outside the encounter doing nothing. docs/reachability.md
// FILED it under the deferrals the register cannot carry -- no symbol was dead,
// the list was built and pruned in a shipped build, and the gap was a design
// one -- and that entry is gone from the doc because this function closed it.
//
// IT RUNS AFTER THE ROUND LOOP, ONCE PER Advance, and both halves of that
// matter. Once per call, because the loop can resolve several rounds and a
// sweep inside it would let one arrival be counted repeatedly. After, because
// a reinforcement must not act in the round it arrives -- resolveRound takes
// its sequence once at the top, so an enemy inserted afterwards first acts in
// the round that follows.
//
// IT NEVER REPOINTS THE FIGHT. The sweep is handed e.target, so a watcher
// aware of somebody else is not a reinforcement; it is somebody else's
// problem, and v0 has no second encounter for it.
func (c *Combat) reinforce() {
	e := c.encounter
	if e == nil || e.target == nil {
		return
	}

	arrivals, _, _ := c.scanAware(e.target)
	if len(arrivals) == 0 {
		return
	}

	known := make(map[string]bool, len(e.enemies))
	for _, enemy := range e.enemies {
		if enemy != nil {
			known[enemy.WatcherID()] = true
		}
	}

	for _, arrival := range arrivals {
		id := arrival.WatcherID()
		if known[id] {
			continue
		}

		known[id] = true

		e.enemies = append(e.enemies, arrival)
		e.enemyOrder = insertBySpeed(e.enemyOrder, id, c.profileOf(id).Speed, c.speedOf)
		c.joined++

		if c.paced() {
			c.holdOne(id)
		}
	}
}

// speedOf is the authored initiative Speed of one enemy already in a fight.
func (c *Combat) speedOf(id string) int { return c.profileOf(id).Speed }

// insertBySpeed puts one arrival into an activation order that already exists,
// WITHOUT re-running the tie-break shuffle.
//
// Re-drawing the order mid-fight is worse than a D8 violation. d8Order takes
// its tie-break from the combat RNG, so a second draw would move every
// subsequent damage roll in the fight -- two launches of one build at one seed
// would stop agreeing, which is R2 §3 bullet 12's determinism clause broken by
// a feature that has nothing to do with damage.
//
// THE TIE IS DECIDED HERE AND IT IS DECIDED DELIBERATELY: an arrival goes in
// front of the first member SLOWER than it, so at equal Speed it lands behind
// everything already fighting. Two live dog packs are two groups on one row at
// one Speed, and the tables can place them, so the case is reachable rather
// than theoretical. Last among its equals is the answer that costs the fight
// nothing: the pack that was already swinging keeps its place.
func insertBySpeed(order []string, id string, speed int, speedOf func(string) int) []string {
	at := len(order)

	for i, other := range order {
		if speedOf(other) < speed {
			at = i

			break
		}
	}

	order = append(order, "")
	copy(order[at+1:], order[at:])
	order[at] = id

	return order
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

	enemies, target, declined := c.scanAware(nil)

	// DECLINES STILL MEAN WHAT THEY MEANT. The scan is now run in two places
	// -- here, and once per tick during a live fight to find reinforcements
	// -- and only this one adds to the counter. Counting both would make
	// declines grow per tick per out-of-reach watcher for a whole fight's
	// duration, which is a different quantity wearing the same name.
	c.declines += declined

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
	if f := c.fitnessOf(target.QuarryID()); f != nil {
		stance = f.Activity()
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

	// M4.7 step 3 (D8 §9; M4.4c-2 ask 2a): the dead take neither surprise
	// branch. A fight with nothing but the dead in it is never a caught one.
	if c.onlyTheDead(enemies) {
		surprised, why = false, ""
	}

	c.encounter = &encounter{
		id:      fmt.Sprintf("e:%d", c.nextID),
		target:  target,
		enemies: enemies,
		dead:    map[string]bool{},
		routed:  map[string]bool{},
		broke:   map[string]bool{},
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
	c.blowLog = c.blowLog[:0]

	c.nextID++
	c.started++
	c.rounds++

	c.holdParticipants()
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
// of it in v0. What ENDS a chase is Pursuit.Release, wired since step 5
// (combat_resolver.go withdraw, on a death or a rout) and, since the hardening
// burst, also from Spawns.Despawn when a pack is sent home at daybreak. Ending
// the encounter here does not end the chase; that lives there.
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
	//
	// STEP 5 WIDENS "CORPSE" TO "GONE": dead OR routed. A pack that broke has
	// left the fight without dying, and it is the same sentence -- the fight
	// the player survived because a wolf lost its nerve -- that the paragraph
	// above was written about. endingReason() decides which of the two it
	// gets reported as.
	allGone := true

	for _, enemy := range e.enemies {
		if enemy != nil && c.stillIn(e, enemy.WatcherID()) {
			allGone = false

			break
		}
	}

	if allGone {
		c.end(e.endingReason())

		return
	}

	// A CORPSE KEEPS ITS ROW EVEN IF IT DRIFTS OUT OF REACH. encounter.dead
	// promises the row lasts the encounter's life, and the resolver despawns
	// nothing -- a dead dog still has a live chase, so it can still be moved.
	kept := e.enemies[:0]

	for _, enemy := range e.enemies {
		// The dead that broke off at first light have LEFT, and they leave
		// the rows too: LayDownDead has taken them off the map, so a row
		// kept for one would be drawn standing where nothing stands.
		if e.broke[enemy.WatcherID()] {
			continue
		}

		if e.gone(enemy.WatcherID()) || within(enemy, e.target, c.disengageTiles()) {
			kept = append(kept, enemy)
		}
	}

	e.enemies = kept

	// Disengagement is now "nothing ALIVE is still in reach" -- corpses do not
	// keep a fight open, and allDead above has already ruled out the case
	// where there was never anything alive to leave.
	living := false

	for _, enemy := range e.enemies {
		id := enemy.WatcherID()

		// A Downed man holds the fight only while the player stands near
		// him: walk away and it is over -- he will stand and come later.
		// Without this a man with no stake could never leave a fight with
		// one of the dead (measured: stood again eight times, and killed
		// him). Kept rows of the gone are kept at any distance, so the
		// distance is asked here.
		if c.stillIn(e, id) && (!e.gone(id) || within(enemy, e.target, c.disengageTiles())) {
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
		if live[id] && !e.gone(id) {
			order = append(order, id)
		}
	}

	e.enemyOrder = order
}

// gone is "takes no further part": dead, or its pack broke. Every place that
// used to ask e.dead about PARTICIPATION asks this instead, and the places
// that ask about DEATH -- the riposte's "nothing to answer if it is already
// dead", the participant row's dead:true -- deliberately still ask e.dead.
func (e *encounter) gone(id string) bool { return e.dead[id] || e.routed[id] || e.broke[id] }

// stillIn is "keeps the fight going": a participant not gone, or one of the
// dead cut down whose body lies Downed and may stand again inside his window
// (M4.7 step 3b; R2 §3, "possibly while the fight still runs"). The fight
// goes on, round by round, until he stands or is staked -- or first light.
func (c *Combat) stillIn(e *encounter, id string) bool {
	if !e.gone(id) {
		return true
	}

	return !e.broke[id] && c.corpses != nil && c.corpses.DownedMember(id)
}

// Rejoin puts a Downed man who stood again back into the fight he fell in
// (M4.7 step 3b; R2 §3, "possibly while the fight still runs"), as the new
// member he walks as. It is not an arrival -- no notice or engage gate: he
// stands where he lay, in the fight he never left -- and the fallen one's
// row goes with the remains the game takes off the map. It reports whether
// the old member was in the fight.
func (c *Combat) Rejoin(oldID string, stood Combatant) bool {
	e := c.encounter
	if e == nil || stood == nil || oldID == "" {
		return false
	}

	in := false
	kept := e.enemies[:0]

	for _, en := range e.enemies {
		if en != nil && en.WatcherID() == oldID {
			in = true

			continue
		}

		kept = append(kept, en)
	}

	e.enemies = kept

	if !in {
		return false
	}

	id := stood.WatcherID()
	e.enemies = append(e.enemies, stood)
	e.enemyOrder = insertBySpeed(e.enemyOrder, id, c.profileOf(id).Speed, c.speedOf)
	c.joined++

	if c.paced() {
		c.holdOne(id)
	}

	return true
}

// onlyTheDead reports a side made wholly of the dead's row.
func (c *Combat) onlyTheDead(enemies []Combatant) bool {
	if len(enemies) == 0 {
		return false
	}

	for _, en := range enemies {
		if en == nil || !c.profileOf(en.WatcherID()).Dead {
			return false
		}
	}

	return true
}

// BreakOff is first light (M4.7 step 3; R2 §2A, "the dead break off at first
// light"): every enemy leave accepts takes no further part, and a fight left
// with nothing in it ends "dawn". A beast in the same fight fights on. It
// reports how many broke off.
func (c *Combat) BreakOff(leave func(id string) bool) int {
	e := c.encounter
	if e == nil || leave == nil {
		return 0
	}

	n := 0

	for _, en := range e.enemies {
		// A Downed man leaves with the rest: by day he lies until dark.
		if en == nil || !c.stillIn(e, en.WatcherID()) || !leave(en.WatcherID()) {
			continue
		}

		e.broke[en.WatcherID()] = true
		n++
	}

	if n == 0 {
		return 0
	}

	for _, en := range e.enemies {
		if en != nil && c.stillIn(e, en.WatcherID()) {
			return n
		}
	}

	// "dawn" is BreakOff's own ending: a fight it emptied. A mixed fight the
	// beasts go on to lose ends however it ends (the review of step 3).
	c.end("dawn")

	return n
}

// endingReason names an ending in which nothing is left to fight.
//
// ROUT WINS WHEN ANYTHING ROUTED, and the precedence is a judgement rather
// than an accident: "you survived because they broke" is the more interesting
// fact about the night than "the rest of them happened to be dead", and a
// fight that ends with survivors walking away is exactly the outcome
// enemies_dead would misreport.
func (e *encounter) endingReason() string {
	if len(e.routed) > 0 {
		return "enemies_routed"
	}

	return "enemies_dead"
}

// SetCorpses attaches the corpse registry (M4.7): every enemy death the
// resolver leaves on the map becomes an open body there.
func (c *Combat) SetCorpses(k *Corpses) { c.corpses = k }

// fallCorpse records an enemy's body where it fell.
func (c *Combat) fallCorpse(id string) {
	if c.corpses == nil || c.encounter == nil {
		return
	}

	for _, en := range c.encounter.enemies {
		if en != nil && en.WatcherID() == id {
			x, y := en.WatcherAt()
			c.corpses.Fall(id, c.profileOf(id).Row, x, y)

			return
		}
	}
}

// EndedReason is why the last encounter ended ("" before any). The death
// screen reads it to say a fight killed him.
func (c *Combat) EndedReason() string { return c.endedReason }

// end closes the encounter and records WHY, both as the last reason and as a
// counter. An encounter that starts and ends between two harness reads is
// invisible in the state and obvious in the counters -- the same argument the
// four counters at the top of this file were added for.
func (c *Combat) end(reason string) {
	c.payOpenRound()

	// The pace row is built from the encounter that is about to go, so it is
	// captured HERE rather than by a caller reading a nil encounter a frame
	// later. A fight that opened and closed between two frames is otherwise
	// invisible, which is the same reason ended_reason persists.
	if c.encounter != nil && c.paceOpen {
		e := c.encounter

		// THE ROUND THAT DIED WITH THE FIGHT still happened, and it is the one
		// the player will want to read: the blow that ended it was his.
		//
		// Measured: a three-round fight wrote two ROUND lines. The last round
		// ends inside commit(), where the killing blow nils the encounter and
		// the commit returns before finishRound is ever reached -- so the row
		// was never captured and act 4's "one line per round" could not hold.
		// Captured here instead, on the only edge that sees it.
		if c.lastActionVerb != "" {
			c.lastRound = RoundRow{
				Encounter:     e.id,
				Round:         e.round,
				DecideSeconds: c.decisionSecondsRound,
				Action:        c.lastActionVerb,
				Move:          e.moveSpent,
			}
		}

		kinds := make([]string, 0, len(e.enemies))
		seen := map[string]bool{}

		for _, enemy := range e.enemies {
			kind := c.profileOf(enemy.WatcherID()).Row
			if kind != "" && !seen[kind] {
				seen[kind] = true

				kinds = append(kinds, kind)
			}
		}

		c.lastPace = PaceRow{
			Encounter:     e.id,
			Rounds:        e.round,
			WallSeconds:   c.wallSeconds,
			DecideSeconds: c.decisionSeconds,
			HealthOpen:    c.paceHealthOpen,
			HealthClose:   c.playerHealth(),
			EndReason:     reason,
			Enemies:       len(e.enemies),
			Kinds:         kinds,
			Initiator:     e.initiator,
			Surprised:     e.surprised,
			Control:       c.dials.PlayerControl,
			Commits:       c.paceCommits,
		}
	}

	c.paceOpen = false
	c.wallSeconds = 0
	c.decisionSeconds = 0
	c.decisionSecondsRound = 0
	c.paceCommits = 0
	c.paceHealthOpen = 0

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
	case "enemies_routed":
		c.endedRouted++
	case "dawn":
		c.endedDawn++
	}
}

// inReach is the adjacency test, on floored tile positions.
//
// Chebyshev rather than Euclidean, and the difference is deliberate: R2 §3's
// zone-of-control is stated in adjacency terms, the map is a tile grid, and a
// diagonal neighbour is as adjacent as an orthogonal one. Pursuit's
// ArriveWithin is a Euclidean 1.5 and answers a different question -- see the
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

// Encounter reports the LIVE encounter's id, or "" when nothing is happening.
//
// LastRound().Encounter cannot stand in for this and the difference is not
// cosmetic: that one names the last CLOSED round, so it is empty for the whole
// of round one and it goes on naming a fight that has already ended. The wish
// note needs the fight the player is IN at the moment he writes, which is the
// only fight he could be writing about.
func (c *Combat) Encounter() string {
	if c.encounter == nil {
		return ""
	}

	return c.encounter.id
}

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

		// T1. world_held is what the game screen's gate reads; owed_minutes is
		// the paced world time not yet paid; steps_ordered counts the walks a
		// paced fight ordered, the evidence a pack moved on its own turn.
		"paced":            c.dials.Paced,
		"engage_tiles":     c.dials.EngageTiles,
		"disengage_tiles":  c.dials.DisengageTiles,
		"enemy_move_tiles": c.dials.EnemyMoveTiles,
		"move_tiles":       c.dials.MoveTiles,
		"world_held":       c.WorldHeld(),
		"owed_minutes":     c.owedMinutes,
		"steps_ordered":    c.stepsOrdered,
		"has_stepper":      c.stepper != nil,

		// The radii IN FORCE, which differ from the dials above under the
		// policy: engage and disengage are the tactical layer's alone.
		"engage_radius":    c.engageTiles(),
		"disengage_radius": c.disengageTiles(),
		"has_notice":       c.notice != nil,
		"has_fitness":      c.fitness != nil,
		"has_bodies":       c.bodies != nil,
		"has_profiles":     c.profiles != nil,
		"has_animator":     c.animator != nil,
		"has_morale":       c.morale != nil,
		"has_chases":       c.chases != nil,
		"bodies_known":     0,

		// The resolver's facts about the LAST encounter, reported whether or
		// not one is running: a fight that begins and ends inside one step is
		// otherwise invisible.
		"ended_reason":       c.endedReason,
		"ended_enemies_dead": c.endedEnemiesDead,
		"ended_player_dead":  c.endedPlayerDead,
		"ended_disengaged":   c.endedDisengaged,
		"ended_routed":       c.endedRouted,
		"ended_dawn":         c.endedDawn,

		// Step 5's three facts about what the fight DID, all reported whether
		// or not one is running. joined is the only evidence a reinforcement
		// ever arrived -- the participant list grows and shrinks, and a
		// script that reads on the wrong tick sees neither.
		"joined":               c.joined,
		"quick_resolved":       c.quickResolved,
		"last_quick_advantage": c.lastQuickAdvantage,

		// M4.4c-2a, the seam. awaiting is the one a script steps frames toward
		// (fighting is true from tryStart, a round before the first slot
		// resolves), and it is what Game.worldRunning() reads.
		"awaiting":               c.Awaiting(),
		"action_spent":           c.ActionSpent(),
		"move_spent":             c.MoveSpent(),
		"decision_seconds":       c.decisionSeconds,
		"decision_seconds_round": c.decisionSecondsRound,
		"commits_refused":        c.CommitsRefused(),
		"commits_by_input":       c.commitsByInput,
		"commits_by_field":       c.commitsByField,
		"wall_seconds":           c.wallSeconds,

		// The two records the instrument writes its lines from, reported so a
		// script asserts against the SAME numbers the log carries rather than
		// re-deriving them from a grep. Fourth provider rule: two paths to one
		// observable is the trap, and this is the way out of it -- the log and
		// the provider read one source.
		"round_row": map[string]interface{}{
			"encounter":      c.lastRound.Encounter,
			"round":          c.lastRound.Round,
			"decide_seconds": c.lastRound.DecideSeconds,
			"action":         c.lastRound.Action,
			"move":           c.lastRound.Move,
		},
		"pace": map[string]interface{}{
			"encounter":      c.lastPace.Encounter,
			"rounds":         c.lastPace.Rounds,
			"wall_seconds":   c.lastPace.WallSeconds,
			"decide_seconds": c.lastPace.DecideSeconds,
			"health_open":    c.lastPace.HealthOpen,
			"health_close":   c.lastPace.HealthClose,
			"end_reason":     c.lastPace.EndReason,
			"enemies":        c.lastPace.Enemies,
			"kinds":          c.lastPace.Kinds,
			"initiator":      c.lastPace.Initiator,
			"surprised":      c.lastPace.Surprised,
			"control":        c.lastPace.Control,
			"commits":        c.lastPace.Commits,
		},

		"actions_total": c.actions,
		"actions_round": c.actionsRound,
		"actions":       c.actionRows(),

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
			"player_control":  c.dials.PlayerControl,
			"auto_end_turn":   c.dials.AutoEndTurn,
			"forced_band":     c.dials.ForcedBand,

			"loss_weight":             c.dials.LossWeight,
			"quick_resolve_advantage": c.dials.QuickResolveAdvantage,
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
	state["acting"] = append([]string{}, e.acting...)
	state["stepping"] = append([]string{}, e.stepping...)

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

			// NO health, deliberately: the player's health is the meters',
			// and one truth with two homes is the disease. A blow on the
			// player is evidenced by its action row's target_health_after,
			// which is the blow's own fact rather than a second copy of the
			// meter.
			//
			// Two assertions pin the absence: combat_test.go:401 (unit -- the
			// require.NotContains on parts[0]) and combat_body_test.go:226-228
			// (playtest -- the playerRow["health"] dup check). Both are NAMED
			// as well as numbered because this comment IS correction S6: it
			// cited :395 until M4.4c-1's own six added lines in
			// combat_test.go moved the assertion out from under it, which is
			// the drift S6 was filed for, recurring in the same burst that
			// was told to fix it.
		}

		if c.illum != nil {
			row["light_here"] = c.illum.Level(int(math.Floor(px)), int(math.Floor(py)))
		}

		if f := c.fitnessOf(e.target.QuarryID()); f != nil {
			row["reaction_available"] = f.ReactionAvailable()
			row["shaken"] = f.Shaken()
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
			// on the map in DD and stays in its spawn group -- but since step
			// 5 its chase is released and nothing watches for it.
			"dead": e.dead[enemy.WatcherID()],

			// A ROUTED ENEMY IS NOT A DEAD ONE, and reporting them in one
			// field would hide the whole point. It keeps its row, leaves the
			// order, takes no activation, and is still standing where it
			// stood: what a routing pack DOES on the map is a named deferral.
			"routed": e.routed[enemy.WatcherID()],
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
		// M4.4c-2a adds three, merged in alphabetically rather than appended,
		// because the list is sorted and TestCombatSettableFields compares it
		// element by element. auto_end_turn and player_control are dials;
		// commit is a VERB expressed as a settable field, which is c-1's
		// ruling and what keeps the tool count at 36.
		//
		// commit calls the SAME code path Combat.Commit calls -- two paths,
		// one implementation, which is the fourth provider rule's trap taken
		// seriously. commits_by_input and commits_by_field report which was
		// used, so a script cannot quietly prove the input path with the
		// field path.
		"adjacent_tiles", "advantage_shift", "auto_end_turn", "commit",
		"crit_band", "crit_factor", "disengage", "disengage_tiles",
		"enemy_move_tiles", "engage_tiles", "forced_band", "graze_band",
		"graze_factor", "hit_factor", "lit_level", "loss_weight",
		"move_tiles", "paced", "player_action", "player_control",
		"quick_resolve_advantage", "round", "round_minutes", "shaken_penalty",
	}
}

// HarnessSet writes one allow-listed field.
func (c *Combat) HarnessSet(field string, value interface{}) error {
	switch field {
	case "player_control":
		kind, ok := value.(string)
		if !ok || (kind != PlayerControlPolicy && kind != PlayerControlHuman) {
			return fmt.Errorf("player_control wants %q or %q, got %v", PlayerControlPolicy, PlayerControlHuman, value)
		}

		c.dials.PlayerControl = kind

		return nil

	case "paced":
		on, ok := value.(bool)
		if !ok {
			return fmt.Errorf("paced wants a bool, got %v", value)
		}

		c.dials.Paced = on

		return nil

	case "engage_tiles", "disengage_tiles", "enemy_move_tiles", "move_tiles":
		v, ok := toFloat(value)
		if !ok || v < 0 || v > 64 {
			return fmt.Errorf("%s wants a tile count 0-64, got %v", field, value)
		}

		switch field {
		case "engage_tiles":
			c.dials.EngageTiles = int(v)
		case "disengage_tiles":
			c.dials.DisengageTiles = int(v)
		case "enemy_move_tiles":
			c.dials.EnemyMoveTiles = int(v)
		case "move_tiles":
			c.dials.MoveTiles = int(v)
		}

		return nil

	case "auto_end_turn":
		on, ok := value.(bool)
		if !ok {
			return fmt.Errorf("auto_end_turn wants a bool, got %v", value)
		}

		c.dials.AutoEndTurn = on

		return nil

	case "commit":
		choice, ok := value.(string)
		if !ok {
			return fmt.Errorf("commit wants a string, got %v", value)
		}

		// THE SAME VERB THE KEYS CALL. A script that does not care about the
		// input overlay uses this; one that is testing the input path uses
		// strigoi_key, and the two counters say which happened.
		if err := c.commit(choice, ""); err != nil {
			return err
		}

		c.commitsByField++

		return nil

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

	case "loss_weight":
		// Settable so a script can prove the arithmetic in both directions:
		// drive it to zero and a pack that loses every member never routs,
		// which is the negative control for the whole trigger.
		v, ok := toFloat(value)
		if !ok || v < 0 {
			return fmt.Errorf("loss_weight wants a non-negative number, got %v", value)
		}

		c.dials.LossWeight = v

	case "quick_resolve_advantage":
		// R2 §5's own dial, and the prototype is what answers it. Above 1 it
		// can never fire, which is a legitimate thing for a script to set:
		// it is how quick-resolve is turned OFF for an assertion about the
		// blow-by-blow path.
		v, ok := toFloat(value)
		if !ok || v < 0 {
			return fmt.Errorf("quick_resolve_advantage wants a non-negative number, got %v", value)
		}

		c.dials.QuickResolveAdvantage = v

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
