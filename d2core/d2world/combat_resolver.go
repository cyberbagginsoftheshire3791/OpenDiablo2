package d2world

import (
	"math"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
)

// THE RESOLVER. Everything up to the blow existed before this file; nothing
// decided what a blow did, and so a fight could not be lost.
//
// It is a set of methods on Combat and a few fields on encounter rather than a
// new system, and that is the fence holding: it reads the light model, the
// meters' two derived facts and the spawn tables' authored numbers, it writes
// health through the same Body adapter the meters already use, and it creates,
// destroys and moves nothing.
//
// WHAT A BLOW IS, in one line each, with the rule each answers:
//
//   - a d100 lands in a BAND -- graze, hit or crit. There is no binary miss
//     (R2 §3 bullet 5), so the worst outcome of a blow is a small wound.
//   - the roll is shifted by ADVANTAGE and by SHAKEN, and the reason is
//     reported beside the number (R2 §3 bullets 7 and 8). A milestone whose
//     signed rules are unassertable has not shipped them.
//   - damage is a second draw from the attacker's PROFILE times the band's
//     factor, floored at 1.
//   - a graze on the player is the "miss" that offers a RIPOSTE (R2 §3
//     bullet 6, read against bullet 5's refusal of a binary miss).
//   - the player's side is run by a stand-in POLICY, because there is no
//     player attack verb in this engine yet (CombatDials.PlayerAction).
//
// TWO RNG DRAWS PER BLOW, ALWAYS AND IN THAT ORDER -- the d100 then the damage
// -- and both happen even when a script has forced the band, so forcing one
// blow does not shift the sequence the next one sees. Both come from Combat's
// own stream, so every other system's draws, and every seed-1462 measurement
// outside a fight, are exactly where they were.

// The bands a blow can land in. `hit` is what is left between the other two
// and is derived rather than stored, so the three can never disagree.
const (
	BandGraze = "graze"
	BandHit   = "hit"
	BandCrit  = "crit"
)

// The player's stand-in Action policy. See CombatDials.PlayerAction for why a
// policy exists at all and why it is reported.
const (
	PlayerActionAttack = "attack"
	PlayerActionHold   = "hold"
)

// PlayerControl decides whether a round WAITS at the player's slot or resolves
// it with the stand-in policy. The default stays "policy" in
// DefaultCombatDials so the ~40 resolver unit tests at four construction sites
// stand as written; the game screen sets "human", so the shipped build waits.
const (
	PlayerControlPolicy = "policy"
	PlayerControlHuman  = "human"
)

// The player's Commit choices. FIVE of them, and `end` is the one that took a
// ruling (Josh, 17 Sep 2026): `hold` is signed as "end the turn with the
// Action UNSPENT", and Commit is refused unless a turn is waiting, so before
// `end` existed nothing could close a turn AFTER a strike or a torch.
//
// EXECUTING THE ACTION IS NOT FINISHING THE TURN. A commit resolves the
// player's slot; the turn closes only when AutoEndTurn's both-spent condition
// is met, or when the player says so with hold or end.
const (
	CommitStrike = "strike"
	CommitLight  = "light"
	CommitDouse  = "douse"
	CommitHold   = "hold"
	CommitEnd    = "end"
)

// The reasons a blow's roll was shifted, reported per blow. Both can apply at
// once, comma-joined: "dark-into-light,shaken".
const (
	whyDarkIntoLight = "dark-into-light"
	whyLightIntoDark = "light-into-dark"
	whyShaken        = "shaken"
)

// Profile is what a combatant fights as: which PACK it belongs to (R2 §3
// bullet 2's activation unit), the row it was authored on, its initiative
// Speed and the bite it rolls.
type Profile struct {
	// Group is the pack key. Two live groups can share one row -- two dog
	// packs are two packs -- so the key is the group, never the row name.
	Group string

	// Row is the design's name for the table row, or a placeholder label. It
	// is for REPORTING: a script reading profile:"placeholder" knows it is
	// fighting something the spawn tables never placed.
	Row string

	Speed                int
	DamageMin, DamageMax int

	// DamageClass is how its blow hurts (cut, thrust, blunt), which is what
	// armour is rated against (T2). "" reads as cut.
	DamageClass string

	// Count is how many the PACK started with, and step 5 needs it twice: the
	// rout decrement scales with it (a loss out of two is half the pack; a
	// loss out of six is not), and quick-resolve's advantage is measured
	// against it. It comes from the group rather than from the row, because
	// the row gives a RANGE and what a fight is against is the pack that
	// actually arrived.
	//
	// Zero means unknown -- the placeholder profile has no pack behind it --
	// and both callers fall back to the participant list rather than dividing
	// by it.
	Count int
}

// Profiles is how the resolver asks what an enemy fights as. *Spawns
// satisfies it; the fence stays an interface exactly as Fitness and Bodies
// do, so the model can be tested with no tables at all.
type Profiles interface {
	ProfileOf(id string) (Profile, bool)
}

// CombatAct is one thing the resolver asks a sprite to show.
type CombatAct int

// The three acts a v0 fight has. Death is DT then a held DD; there is no
// corpse state beyond that, because what a dead beast BECOMES is M4.7's.
const (
	ActSwing CombatAct = iota
	ActHit
	ActDie
)

// Animator is how the resolver reaches sprites, and it exists for the same
// reason Bodies does: d2world cannot import d2mapentity, so it names the
// narrowest thing that answers the question and the game screen implements it.
//
// A nil Animator is legal and means nothing is drawn -- every unit test below
// runs that way, and has_animator reports it so a playtest can tell "the
// wiring is missing" from "the swing did not happen".
type Animator interface {
	Animate(id string, act CombatAct)
}

// action is one blow, and every field of it is reported. Without band and
// advantage_why the milestone's two signed rules are unassertable; without
// base a script can only range-check the damage instead of recomputing it.
type action struct {
	round             int
	attacker          string
	target            string
	roll              int
	mod               int
	score             int
	band              string
	base              int
	damage            int
	targetHealthAfter int
	targetHasBody     bool
	advantageWhy      string
	reaction          string

	// T2, the kit: the band before a shield stepped it down, whether one
	// did, what armour took off, the damage class and the weapon that struck.
	bandRolled string
	blocked    bool
	absorbed   int
	class      string
	weapon     string
}

// playerProfile is the M4.5 note's "one clearly-labelled placeholder melee
// profile" (ask 1, signed as a LABEL -- the numbers are nobody's until E3).
// It is CONTENT rather than a dial and has no setter: a script that wants a
// different blow forces a band.
func playerProfile() Profile {
	return Profile{Row: "placeholder-melee", DamageMin: 12, DamageMax: 20}
}

// defaultEnemyProfile is what something the spawn tables never placed fights
// as. It is not defensive: d2app/harness_spawn.go and the debug terminal both
// put NPCs on the map without going through the tables, and EVERY playtest
// fight so far has been against one of those. Without this the thirteenth
// script would fight something with no damage.
func defaultEnemyProfile() Profile {
	return Profile{Row: "placeholder", Speed: 1, DamageMin: 2, DamageMax: 5}
}

// profileOf answers for any enemy, table-placed or not. An enemy with no row
// is its own pack of one, keyed by its id, so a handful of harness-spawned
// monsters activate as separate packs rather than collapsing into one.
func (c *Combat) profileOf(id string) Profile {
	if c.profiles != nil {
		if p, ok := c.profiles.ProfileOf(id); ok {
			return p
		}
	}

	p := defaultEnemyProfile()
	p.Group = id

	return p
}

// hitBand is the middle slice of the d100, derived from the other two.
func (c *Combat) hitBand() int {
	if h := 100 - c.dials.GrazeBand - c.dials.CritBand; h > 0 {
		return h
	}

	return 0
}

// activation is the sequence for the CURRENT round, the player included.
//
// D8 §9: the player's side goes first each round, EXCEPT in a surprised round
// one -- caught head-down -- where the pack goes first and he goes last.
func (e *encounter) activation() []string {
	playerID := ""
	if e.target != nil {
		playerID = e.target.QuarryID()
	}

	out := make([]string, 0, len(e.enemyOrder)+1)

	if e.round == 1 && e.surprised {
		out = append(out, e.enemyOrder...)

		if playerID != "" {
			out = append(out, playerID)
		}

		return out
	}

	if playerID != "" {
		out = append(out, playerID)
	}

	return append(out, e.enemyOrder...)
}

// firstSide reports whose side acts first this round, which is the readable
// form of the same rule.
func (e *encounter) firstSide() string {
	if e.round == 1 && e.surprised {
		return "enemy"
	}

	return "player"
}

// enemyByID finds a participant by id.
func (e *encounter) enemyByID(id string) Combatant {
	for _, w := range e.enemies {
		if w != nil && w.WatcherID() == id {
			return w
		}
	}

	return nil
}

// d8Order is D8 §9's initiative, and it replaces step 2's provisional shuffle.
//
// Packs activate in DESCENDING authored Speed; ties are broken ONCE, at fight
// start, by the seeded RNG rather than re-rolled every round; members inside a
// pack go in sorted id order. The shuffle runs before the sort and the sort is
// STABLE, so equal speeds keep the shuffled order and unequal ones do not care
// what the shuffle did.
//
// THE SHUFFLE DRAWS FROM c.rng BEFORE THE FIRST BLOW, so a unit test that
// replays a seed must replay this draw too.
func (c *Combat) d8Order(enemies []Combatant) []string {
	type pack struct {
		speed   int
		members []string
	}

	byKey := map[string]*pack{}
	keys := make([]string, 0, len(enemies))

	for _, e := range enemies {
		if e == nil {
			continue
		}

		id := e.WatcherID()

		p := c.profileOf(id)
		if p.Group == "" {
			p.Group = id
		}

		if _, seen := byKey[p.Group]; !seen {
			byKey[p.Group] = &pack{speed: p.Speed}
			keys = append(keys, p.Group)
		}

		byKey[p.Group].members = append(byKey[p.Group].members, id)
	}

	// Sort the keys first so the shuffle's input never depends on the order
	// AwarePairs happened to hand them over in -- the A*'s lesson, and the
	// same reason provisionalOrder sorted before it shuffled.
	sort.Strings(keys)

	packs := make([]*pack, 0, len(keys))

	for _, k := range keys {
		sort.Strings(byKey[k].members)

		packs = append(packs, byKey[k])
	}

	c.rng.Shuffle(len(packs), func(i, j int) { packs[i], packs[j] = packs[j], packs[i] })
	sort.SliceStable(packs, func(i, j int) bool { return packs[i].speed > packs[j].speed })

	out := make([]string, 0, len(enemies))
	for _, p := range packs {
		out = append(out, p.members...)
	}

	return out
}

// tryQuickResolve is R2 §2B: "quick-resolve exists for mundane animals at
// overwhelming advantage (never for the dead)". It reports whether it fired.
//
// IT FIRES. An earlier draft had it report an offer and do nothing, and that
// contradicts a SIGNED playtest assertion -- N1 §5: "quick-resolve fires
// against a lone dog at advantage and never against the dead", which the
// signed M4.5 note re-quotes and grades buildable in its first half.
//
// THE SECOND HALF IS A NAMED DEFERRAL TO M4.7 RATHER THAN A FALSE ASSERTION.
// There are no dead in this engine: the rows are dogs, wolves, boar and
// opportunists. The signed note says so itself -- "SECOND HALF UNASSERTABLE --
// there are no dead until M4.7" -- so a test claiming to prove the prohibition
// would be proving nothing about the rule and something false about the build.
// What IS asserted instead is that an enemy with no known group never
// quick-resolves, which is a true statement about this build.
//
// The threshold is R2 §5's open dial, not this milestone's number.
func (c *Combat) tryQuickResolve() bool {
	e := c.encounter
	if e == nil || e.target == nil || c.morale == nil {
		return false
	}

	if c.deadByBody(e.target.QuarryID()) {
		return false
	}

	// THE ADVANTAGE IS MEASURED AGAINST THE FIGHT, NOT THE TABLE, and the
	// first version of this got it wrong in a way only a real build showed.
	//
	// It divided by the pack's AUTHORED size, and in a shipped game a pack
	// does not arrive together -- Spawns.spawn scatters its members
	// independently between the row's MinTiles and MaxTiles, and only the
	// ones that see the player ever join. So a fight against two dogs of an
	// authored four already read as 50% advantage before a single blow
	// landed, and with two packs represented it crossed the threshold at
	// once: the fourteenth playtest caught quick-resolve MASSACRING fights in
	// which nothing had died. What the rule is about is how much of the enemy
	// side in front of you is already down, so the denominator is the
	// participant list -- which never shrinks for the dead or the routed,
	// because their rows last the encounter's life.
	//
	// Profile.Count is still exactly right for the ROUT decrement: morale is
	// the GROUP's, and a group that lost one of four has lost one of four
	// wherever the other three are standing.
	living := make([]string, 0, len(e.enemies))
	starting := 0

	for _, enemy := range e.enemies {
		if enemy == nil {
			continue
		}

		starting++

		id := enemy.WatcherID()

		if e.gone(id) {
			continue
		}

		// EVERY LIVING ENEMY MUST BE MUNDANE. One thing that is not is enough
		// to refuse, because the prohibition is about what is in the fight,
		// not about the majority of it.
		if !c.mundane(id) {
			return false
		}

		living = append(living, id)
	}

	// Nothing left to finish, and nothing to be at an advantage over.
	if len(living) == 0 || starting <= 0 {
		return false
	}

	advantage := 1 - float64(len(living))/float64(starting)
	if advantage < c.dials.QuickResolveAdvantage {
		return false
	}

	for _, id := range living {
		if body := c.bodyOf(id); body != nil {
			body.SetHealth(0)
		}

		e.dead[id] = true
		c.earn("slain", id)
		c.fallCorpse(id)

		c.animate(id, ActDie)

		c.withdraw(id)
	}

	e.enemyOrder = e.enemyOrder[:0]

	c.quickResolved++
	c.lastQuickAdvantage = advantage

	// THE ENDING IS enemies_dead EVEN IF SOMETHING ROUTED EARLIER, and the
	// precedence is deliberate: a fight the player finished is a fight the
	// player finished. endingReason() is for the case where the last thing
	// standing walked away, which is not this one.
	c.end("enemies_dead")

	return true
}

// mundane is "a beast, and one this world placed".
//
// IT CANNOT BE morale > 0 ALONE, because "no morale" and "no group" are
// different facts and the engine returns the second. Anything the tables never
// placed -- and every playtest fight so far has been against exactly such a
// stand-in -- gets the placeholder profile, whose group is its own id and
// which the spawn tables have never heard of. Reading unknown as zero would
// classify every harness-spawned NPC as the dead and refuse always; reading it
// as mundane would offer against everything, including the day something
// undead arrives through a path the tables do not own.
//
// So the test is: the group is KNOWN, and its morale is positive. M4.3b signs
// the dead's morale as "none, ever", which makes the second clause the
// prohibition stated in the only terms this engine has.
func (c *Combat) mundane(id string) bool {
	if c.morale == nil {
		return false
	}

	group := c.profileOf(id).Group
	if group == "" {
		return false
	}

	morale, known := c.morale.Morale(group)

	return known && morale > 0
}

// resolveRound opens a round and walks it. Under the policy it runs straight
// through, exactly as it did before the seam existed. Under human control it
// STOPS at the player's slot with awaiting set, and the world stops with it.
func (c *Combat) resolveRound() {
	e := c.encounter
	if e == nil || e.target == nil {
		return
	}

	// A fresh log for THIS round, kept until the next one resolves. See
	// Combat.lastActions: clearing it per Advance call would empty it on
	// every frame that is not a round boundary, which is most of them.
	c.lastActions = c.lastActions[:0]
	c.actionsRound = e.round

	// R2 §2B's quick-resolve, offered BEFORE the round rather than inside it:
	// a fight already won against mundane animals is finished in one action
	// instead of ground out a blow at a time. It runs at the TOP of the round
	// and therefore never during a wait -- Advance is not called while the
	// encounter is awaiting, so a waiting turn cannot be quick-resolved out
	// from under the player.
	if c.tryQuickResolve() {
		return
	}

	// THE SEQUENCE IS FROZEN HERE, once, and walked from a cursor. v1.0 of
	// this note re-read e.activation() on resume, which is unsafe: the
	// player's slot MOVES between rounds (a surprised round one puts him
	// last), so a resumed walk would have indexed a different list than the
	// one it stopped in.
	e.sequence = e.activation()
	e.cursor = 0
	e.moveSpent = false
	e.actionSpent = false

	c.walkSequence()
}

// walkSequence runs the frozen sequence from the cursor, and is the ONLY place
// a slot is resolved. It returns true when the round's sequence is exhausted.
//
// It is entered from two lines: resolveRound at the top of a round, and
// endTurn when the player closes a turn that stopped part-way through.
func (c *Combat) walkSequence() bool {
	e := c.encounter
	if e == nil || e.target == nil {
		return false
	}

	playerID := e.target.QuarryID()

	for e.cursor < len(e.sequence) {
		// A blow can end the encounter (the player died, or that was the last
		// of them). Nothing further happens in a fight that is over.
		if c.encounter == nil {
			return false
		}

		id := e.sequence[e.cursor]

		if id == playerID {
			if c.dials.PlayerControl == PlayerControlHuman {
				// Frozen mid-round, by construction: the cursor still points
				// AT the player's slot, so the commit that follows resolves
				// this slot and the walk resumes from the next one.
				e.awaiting = true

				// The per-turn timer starts at the turn-OPEN, which is here
				// and nowhere else. Resetting it at the close instead would
				// leave the last turn's number readable for exactly as long
				// as nobody looked.
				c.decisionSecondsRound = 0

				return false
			}

			c.playerActivation()
		} else {
			c.enemyActivation(id)
		}

		e.cursor++
	}

	e.sequence = nil
	e.cursor = 0

	return true
}

// playerActivation is the stand-in policy: strike the first adjacent living
// enemy in the order, or do nothing on "hold".
func (c *Combat) playerActivation() {
	e := c.encounter
	if e == nil || e.target == nil || c.dials.PlayerAction != PlayerActionAttack {
		return
	}

	for _, id := range e.enemyOrder {
		if e.gone(id) {
			continue
		}

		enemy := e.enemyByID(id)
		if enemy == nil || !c.inReach(enemy, e.target) {
			continue
		}

		c.resolveBlow(e.round, e.target.QuarryID(), id, true, "")

		return
	}
}

// commitStrike is the player's chosen blow. It goes through resolveBlow
// unchanged -- the roll, the bands, the light advantage, the animation, the
// body write -- so NO BLOW LANDS BY ANY PATH THE POLICY COULD NOT HAVE TAKEN.
//
// An empty or unreachable target falls through to playerActivation, which is
// the policy's own choice: the first living adjacent enemy in D8 order. That
// is deliberate and it is what makes the strike KEY equal to the policy's
// target exactly; choosing a different one is the click, and the click is
// c-2b's.
func (c *Combat) commitStrike(targetID string) {
	e := c.encounter
	if e == nil || e.target == nil {
		return
	}

	if targetID != "" && !e.gone(targetID) {
		if enemy := e.enemyByID(targetID); enemy != nil && c.inReach(enemy, e.target) {
			c.resolveBlow(e.round, e.target.QuarryID(), targetID, true, "")

			return
		}
	}

	c.playerActivation()
}

// enemyActivation is one enemy's blow on the player, and the Riposte it may
// buy him.
//
// A PACK IS ONE ACTIVATION and every living, adjacent member of it strikes
// inside that activation (R2 §3 bullet 2 for the unit, and R2 §1 for why a
// six-wolf pack landing six blows is the design rather than a bug -- "multi
// enemy fairness through tools, rather than stat walls"). The dial that tames
// a pack is MaxCount on its row, not a fudge here.
func (c *Combat) enemyActivation(id string) {
	e := c.encounter
	if e == nil || e.target == nil || e.gone(id) {
		return
	}

	enemy := e.enemyByID(id)
	if enemy == nil || !c.inReach(enemy, e.target) {
		// Still chasing. Being in the encounter and being in reach are two
		// facts, and pruneOrEnd owns the second one.
		return
	}

	idx, ok := c.resolveBlow(e.round, id, e.target.QuarryID(), false, "")
	if !ok {
		return
	}

	if !c.riposteAllowed(idx, id) {
		return
	}

	// Reported on BOTH rows: on the enemy's blow, as the thing that triggered
	// the answer, and on the player's, as what it was.
	c.lastActions[idx].reaction = "riposte"
	c.spendReaction()

	c.resolveBlow(c.encounter.round, e.target.QuarryID(), id, true, "riposte")
}

// riposteAllowed is R2 §3 bullet 6 with bullet 5's refusal of a binary miss
// folded in: A GRAZE ON THE PLAYER IS THE MISS. The enemy's blow was poor
// enough to be answered.
//
// Clause 5 of the milestone's DoD -- at fatigue >= 75 the resolver offers no
// Reaction -- becomes assertable exactly here, and it is READ from the meters
// rather than recomputed (M4.2 ask 1).
//
// "SHAKEN BLOCKS RIPOSTE" IS UNOBSERVABLE IN A REAL BUILD and the rule stays
// anyway: ShakenFatigue (90) and ThirstyShakenFatigue (80) both exceed
// NoReactionFatigue (75), so in every shipped build Shaken already implies no
// Reaction, and a Riposte withheld at fatigue 95 was withheld by clause 5. The
// model must still be right for the day the dials cross, so the condition is
// here and it is asserted in the unit tests with fakes, never in a playtest.
func (c *Combat) riposteAllowed(idx int, attackerID string) bool {
	e := c.encounter
	if e == nil || idx < 0 || idx >= len(c.lastActions) {
		return false
	}

	// THE ROLLED BAND, not the one a shield turned it into: a blow the shield
	// stepped down to a graze was a good blow well met, not a poor one to
	// answer. And the answer needs a blade that ripostes (E3 §4's column):
	// an empty hand, a spear or a mace gives none.
	if c.lastActions[idx].bandRolled != BandGraze {
		return false
	}

	if k := c.kitOf(e.target.QuarryID()); k != nil {
		if b, ok := k.MainBite(); !ok || b.Reaction != d2items.ReactionRiposte {
			return false
		}
	}

	// A Riposte cannot itself trigger a Riposte: the enemy side has no
	// reactions in v0.
	if c.lastActions[idx].reaction != "" {
		return false
	}

	if f := c.fitnessOf(e.target.QuarryID()); f == nil || !f.ReactionAvailable() || f.Shaken() {
		return false
	}

	// One per round (R2 §3 bullet 6's own cap), and none at all in a
	// surprised round one (D8 §9: the caught-head-down player's Reaction is
	// unavailable that round).
	if !c.reactionsLeft() || (e.round == 1 && e.surprised) {
		return false
	}

	// Nothing to answer if the thing that grazed you is already dead.
	return !e.dead[attackerID]
}

// resolveBlow is the blow itself. It returns the index of the action it
// logged, so a caller can annotate it (the Riposte).
func (c *Combat) resolveBlow(round int, attackerID, targetID string, attackerIsPlayer bool,
	reaction string) (int, bool) {
	if c.encounter == nil {
		return -1, false
	}

	profile := playerProfile()
	if !attackerIsPlayer {
		profile = c.profileOf(attackerID)
	}

	// T2: the player strikes with what is in his hand. The kılıç bites 12-20,
	// the placeholder's own range, so the starting kit moves no number; a
	// knife or an axe does. No kit, or an empty hand, keeps the placeholder.
	class, vsMail, weapon := profile.DamageClass, 0.0, ""
	if attackerIsPlayer {
		if k := c.kitOf(attackerID); k != nil {
			if b, ok := k.MainBite(); ok {
				profile.DamageMin, profile.DamageMax = b.Min, b.Max
				class, vsMail, weapon = string(b.Class), b.VsMail, b.Item
			}
		}
	}

	if class == "" {
		class = string(d2items.Cut)
	}

	// DRAW ONE: the d100. DRAW TWO: the damage. Both always, in this order,
	// forced band or not -- see this file's header.
	roll := c.rng.Intn(100) + 1

	span := profile.DamageMax - profile.DamageMin
	if span < 0 {
		span = 0
	}

	base := c.rng.Intn(span+1) + profile.DamageMin
	if base < 1 {
		base = 1
	}

	mod, why := c.advantage(attackerID, targetID)

	// T3: his edges. Zero is neutral, so a hero with no talents -- and every
	// resolver test, which binds none -- rolls exactly as before.
	edge := Edge{}
	if attackerIsPlayer {
		edge = c.edgeOf(attackerID)

		if mod > 0 {
			mod += edge.AdvantageBonus
		}
	}

	// Shaken costs the PLAYER accuracy. An enemy is never Shaken in v0 -- the
	// condition is a fact about the player's body and nothing computes one
	// for a beast.
	if f := c.fitnessOf(attackerID); attackerIsPlayer && f != nil && f.Shaken() {
		mod -= c.dials.ShakenPenalty
		why = joinWhy(why, whyShaken)
	}

	score := clampScore(roll + mod)

	band := c.bandForCrit(score, c.dials.CritBand+edge.CritBand)
	if c.dials.ForcedBand != "" {
		band = c.dials.ForcedBand
	}

	// T2: THE SHIELD STEPS ONE BLOW A ROUND DOWN A BAND -- crit to hit, hit to
	// graze [DIAL: once a round]. Enough to blunt a pack, not to erase one. It
	// draws nothing from the RNG, so the fight's sequence is the same with a
	// shield or without.
	rolled := band
	targetKit := c.kitOf(targetID)
	blocked := false

	if targetKit != nil && !attackerIsPlayer && (band == BandHit || band == BandCrit) &&
		targetKit.CanBlock() && c.blocksLeft(round, targetID) {
		if band == BandCrit {
			band = BandHit
		} else {
			band = BandGraze
		}

		targetKit.SpendBlock()
		c.spendBlock(round)
		blocked = true
	}

	// The floor of 1 is what makes "bounded" true at the bottom: a graze is a
	// small wound, never nothing.
	damage := int(float64(base) * c.factorFor(band))

	// T3: Riposte Drill -- his answer lands harder.
	if attackerIsPlayer && reaction == "riposte" {
		damage = int(float64(damage) * orOne(edge.RiposteDamage))
	}

	// T2: his mail takes its share off a blow of its class, and wears for it.
	absorbed := 0
	if targetKit != nil && !attackerIsPlayer {
		absorbed = targetKit.Absorb(d2items.DamageClass(class), vsMail, band)
		damage -= absorbed
	}

	if damage < 1 {
		damage = 1
	}

	a := action{
		round: round, attacker: attackerID, target: targetID,
		roll: roll, mod: mod, score: score, band: band,
		base: base, damage: damage,
		advantageWhy: why, reaction: reaction,
		bandRolled: rolled, blocked: blocked, absorbed: absorbed,
		class: class, weapon: weapon,
	}

	c.animate(attackerID, ActSwing)

	// A target with no body cannot be hurt and cannot die, which is what
	// has_body:false has meant since step 3. The blow is still reported -- it
	// happened -- and target_health_after is ABSENT rather than zero, because
	// "I do not know" and "it is dead" must not share a value (A3).
	if body := c.bodyOf(targetID); body != nil {
		health := body.CurrentHealth() - damage
		if health < 0 {
			health = 0
		}

		body.SetHealth(health)

		a.targetHasBody = true
		a.targetHealthAfter = health
	}

	c.lastActions = append(c.lastActions, a)
	c.actions++

	c.logBlow(a)

	idx := len(c.lastActions) - 1

	c.killerIsPlayer = attackerIsPlayer

	switch {
	case a.targetHasBody && a.targetHealthAfter <= 0:
		c.reachedZero(targetID)
	case a.targetHasBody:
		c.animate(targetID, ActHit)

		// T3: Fire and Iron -- while his torch burns, a hit that lands
		// shakes a beast's pack. Never the dead: mundane() is the fence.
		if attackerIsPlayer && edge.LitNerve > 0 && band != BandGraze && c.morale != nil && c.mundane(targetID) {
			if group := c.profileOf(targetID).Group; group != "" {
				c.morale.Hurt(group, edge.LitNerve)
				c.routeIfBroken(group)
			}
		}
	}

	c.killerIsPlayer = false

	return idx, true
}

// reachedZero is what 0 health means, and it means two different things.
//
// THE PLAYER AT 0: the encounter ends, reported as player_dead. Meters.Dead()
// is already true by the same field, so there is no second death fact. There
// is no screen, no reload and no animation -- M4.6 owns all three -- and the
// player keeps walking. That is grotesque and it is correct for this step.
//
// AN ENEMY AT 0: it is dead. It leaves the order and takes no further
// activation, and it STAYS on the map, in its spawn group, with its participant
// row reporting dead:true. The resolver despawns nothing (the fence). Its chase
// and its watch are both dropped -- step 5 added withdraw(), and Notice.Unwatch
// without Pursuit.Release would be undone on the next frame by
// startChasesForTheAware. N1's "dead is dead" is honoured and nothing pretends
// it is a corpse yet; the corpse machine is M4.7's.
func (c *Combat) reachedZero(id string) {
	e := c.encounter
	if e == nil {
		return
	}

	if e.target != nil && e.target.QuarryID() == id {
		c.end("player_dead")

		return
	}

	if e.dead[id] {
		return
	}

	e.dead[id] = true
	c.earn("slain", id)
	c.fallCorpse(id)

	c.animate(id, ActDie)

	kept := e.enemyOrder[:0]

	for _, x := range e.enemyOrder {
		if x != id {
			kept = append(kept, x)
		}
	}

	e.enemyOrder = kept

	// The world stops paying attention to it. Before step 5 a dead dog kept
	// its chase AND kept being noticed, so the next tick opened a fresh
	// encounter against the corpse.
	c.withdraw(id)

	// And its pack feels it. This is the trigger M4.3b's signature left
	// belonging to neither milestone: the state was built, the threshold was
	// built, the report was built, and nothing could move the number.
	c.loseNerve(id)

	for _, enemy := range e.enemies {
		if enemy != nil && !e.gone(enemy.WatcherID()) {
			return
		}
	}

	c.end(e.endingReason())
}

// withdraw takes one enemy out of the world's attention: its chase ends and
// nothing watches for it any more.
//
// BOTH HALVES OR NEITHER, and the reason is a frame long. The game screen runs
// startChasesForTheAware() every frame immediately before Advance, and that
// function has no liveness filter -- it walks AwarePairs() and chases anything
// not already chasing. So Release on its own is undone on the very next frame:
// read the chase list one frame later and the dead dog is chasing again, with
// no harness verb called anywhere. Unwatch is what makes the release stick.
//
// Both are tolerant of a miss. Release returns false for a hunter with no
// chase and Unwatch for a watcher already dropped, and neither is an error
// here: something can die without ever having chased.
func (c *Combat) withdraw(id string) {
	if c.chases != nil {
		c.chases.Release(id)
	}

	if c.notice != nil {
		c.notice.Unwatch(id)
	}
}

// loseNerve is what one death costs the pack that lost it, and it is the whole
// of the rout TRIGGER.
//
// THE DECREMENT SCALES WITH THE PACK'S STARTING SIZE -- LossWeight * 100 /
// count -- because a flat one is refuted by the authored rows: dogs at 50 and
// wolves at 60 against a threshold of 25 are ten points apart, and any flat
// decrement big enough to break the wolves breaks the dogs on the same loss.
// See CombatDials.LossWeight for the worked table.
//
// A pack with no group, or one the tables never placed, loses nothing. Neither
// does one with an unknown starting count: the placeholder profile has none,
// and the participant list is used instead rather than dividing by zero.
func (c *Combat) loseNerve(id string) {
	if c.morale == nil {
		return
	}

	profile := c.profileOf(id)
	if profile.Group == "" {
		return
	}

	if _, known := c.morale.Morale(profile.Group); !known {
		return
	}

	count := profile.Count
	if count <= 0 {
		count = c.packSize(profile.Group)
	}

	if count <= 0 {
		return
	}

	// T3: What Breaks Them -- a death HE dealt costs the pack more.
	weight := c.dials.LossWeight
	if c.killerIsPlayer && c.encounter != nil && c.encounter.target != nil {
		weight *= orOne(c.edgeOf(c.encounter.target.QuarryID()).KillNerve)
	}

	c.morale.Hurt(profile.Group, weight*100/float64(count))

	c.routeIfBroken(profile.Group)
}

// routeIfBroken marks a whole pack routed once its morale is at or below the
// threshold M4.3b signed.
//
// THE PACK BREAKS, NOT THE INDIVIDUAL. R2 §3 bullet 2 makes the pack the
// activation unit and N1 §5 writes every rout phrasing about packs, so one
// dog's nerve is not a thing this model has. Every LIVING member of the group
// still in this fight leaves it at once, and each of them is withdrawn for the
// same reason a dead one is -- otherwise the frame after the rout re-chases
// them and tryStart opens a fresh fight against the pack that just fled.
//
// A routed member keeps its participant row, with routed:true, and keeps
// standing where it stood. What a routing pack DOES on the map is a named
// deferral (ask 2): fleeing needs a destination, a router call per member per
// round and a rule for re-noticing.
func (c *Combat) routeIfBroken(groupID string) {
	e := c.encounter
	if e == nil || c.morale == nil {
		return
	}

	routing, known := c.morale.Routing(groupID)
	if !known || !routing {
		return
	}

	for _, enemy := range e.enemies {
		if enemy == nil {
			continue
		}

		id := enemy.WatcherID()
		if e.gone(id) || c.profileOf(id).Group != groupID {
			continue
		}

		e.routed[id] = true
		c.earn("routed", id)

		c.withdraw(id)
	}

	kept := e.enemyOrder[:0]

	for _, id := range e.enemyOrder {
		if !e.gone(id) {
			kept = append(kept, id)
		}
	}

	e.enemyOrder = kept
}

// packSize counts how many of one group are participants in this fight. It is
// the fallback for a profile with no authored count, and it is deliberately
// the fight's view rather than the table's: what the player is up against is
// what turned up.
func (c *Combat) packSize(groupID string) int {
	e := c.encounter
	if e == nil {
		return 0
	}

	n := 0

	for _, enemy := range e.enemies {
		if enemy != nil && c.profileOf(enemy.WatcherID()).Group == groupID {
			n++
		}
	}

	return n
}

// advantage is R2 §3 bullet 7, on ABSOLUTE levels against one threshold --
// deliberately the same threshold the notice model calls lit, so that "a wolf
// can see you" and "you are lit for the wolf's blow" are one fact (S1 §4).
//
// IT COULD NOT FIRE IN A v0 BUILD UNTIL M4.5 ASK 8, AND THAT WAS MEASURED
// RATHER THAN SUSPECTED. A pursuer's route used to end on the quarry's OWN
// tile -- entities do not block the search -- so it walked to distance 0.000
// and stopped there, and every participant in a settled fight floored to one
// tile and therefore sampled one light level. Measured 3 Sep 2026 at 696edbf2:
// twelve world minutes, two monsters, every light_here reading identical to
// the player's.
//
// ASK 8 (187d52db) MADE IT POSSIBLE AND HAS NOT YET MADE IT OBSERVED. A
// pursuer now stops on the best free NEIGHBOUR of its quarry, so participants
// stand on DIFFERENT tiles and this function reads two independent levels.
// What is still missing is a level that DIFFERS: at night the illumination is
// uniform at 0.5000, so observing this rule in a real build needs a PLACED
// source between two participants -- its own act and its own burst.
//
// So the rule stays proved where it was, for the same reason "Shaken blocks
// Riposte" stays: it is right and it is asserted in the unit tests with fakes,
// which is also where flanking and facing will be proved when they arrive.
func (c *Combat) advantage(attackerID, targetID string) (int, string) {
	if c.illum == nil {
		return 0, ""
	}

	ax, ay, haveA := c.tileOf(attackerID)

	tx, ty, haveT := c.tileOf(targetID)
	if !haveA || !haveT {
		return 0, ""
	}

	attackerLit := c.illum.Level(ax, ay) >= c.dials.LitLevel
	targetLit := c.illum.Level(tx, ty) >= c.dials.LitLevel

	switch {
	case !attackerLit && targetLit:
		return c.dials.AdvantageShift, whyDarkIntoLight
	case attackerLit && !targetLit:
		return -c.dials.AdvantageShift, whyLightIntoDark
	}

	return 0, ""
}

// tileOf is a participant's floored tile, by id, on either side.
func (c *Combat) tileOf(id string) (x, y int, ok bool) {
	e := c.encounter
	if e == nil {
		return 0, 0, false
	}

	if e.target != nil && e.target.QuarryID() == id {
		fx, fy := e.target.QuarryAt()

		return int(math.Floor(fx)), int(math.Floor(fy)), true
	}

	if w := e.enemyByID(id); w != nil {
		fx, fy := w.WatcherAt()

		return int(math.Floor(fx)), int(math.Floor(fy)), true
	}

	return 0, 0, false
}

// bandFor places a score in its band.
func (c *Combat) bandFor(score int) string {
	switch {
	case score <= c.dials.GrazeBand:
		return BandGraze
	case score > 100-c.dials.CritBand:
		return BandCrit
	default:
		return BandHit
	}
}

// bandForCrit is bandFor with the crit band given -- his Killing Stroke widens
// it for his own blows. The graze band is untouched, so a wider crit comes out
// of the hit band.
func (c *Combat) bandForCrit(score, crit int) string {
	if crit == c.dials.CritBand {
		return c.bandFor(score)
	}

	switch {
	case score <= c.dials.GrazeBand:
		return BandGraze
	case score > 100-crit:
		return BandCrit
	}

	return BandHit
}

// factorFor is what the band multiplies the damage draw by.
func (c *Combat) factorFor(band string) float64 {
	switch band {
	case BandGraze:
		return c.dials.GrazeFactor
	case BandCrit:
		return c.dials.CritFactor
	default:
		return c.dials.HitFactor
	}
}

// bodyOf is the one place health is reached, on both sides. The player's body
// answers here too, through the game screen's own BodyOf -- so the resolver
// and the meters write the SAME field through the SAME adapter, and the two
// readings cannot drift.
func (c *Combat) bodyOf(id string) Body {
	if c.bodies == nil {
		return nil
	}

	return c.bodies.BodyOf(id)
}

// animate is the nil-safe way to ask for a sprite.
func (c *Combat) animate(id string, act CombatAct) {
	if c.animator == nil || id == "" {
		return
	}

	c.animator.Animate(id, act)
}

// actionRows reports the blows of the most recent Advance call.
func (c *Combat) actionRows() []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(c.lastActions))

	for i := range c.lastActions {
		a := c.lastActions[i]

		row := map[string]interface{}{
			"round":         a.round,
			"attacker":      a.attacker,
			"target":        a.target,
			"roll":          a.roll,
			"mod":           a.mod,
			"score":         a.score,
			"band":          a.band,
			"base":          a.base,
			"damage":        a.damage,
			"advantage_why": a.advantageWhy,
			"reaction":      a.reaction,
			"band_rolled":   a.bandRolled,
			"blocked":       a.blocked,
			"absorbed":      a.absorbed,
			"damage_class":  a.class,
			"weapon":        a.weapon,
		}

		if a.targetHasBody {
			row["target_health_after"] = a.targetHealthAfter
		}

		out = append(out, row)
	}

	return out
}

// joinWhy comma-joins the reasons a roll was shifted; both can apply at once.
func joinWhy(why, add string) string {
	if why == "" {
		return add
	}

	return why + "," + add
}

// clampScore keeps a shifted roll on the die.
func clampScore(score int) int {
	if score < 1 {
		return 1
	}

	if score > 100 {
		return 100
	}

	return score
}

// Kits is how the resolver reads what a combatant carries (T2). The game
// screen implements it; nil, or a nil kit for an id, means no gear -- the
// placeholder profile and no armour, which is every fight before T2 and every
// resolver unit test that does not ask for one.
type Kits interface {
	KitOf(id string) *d2items.Kit
}

// SetKits attaches the kit source.
func (c *Combat) SetKits(k Kits) { c.kits = k }

func (c *Combat) kitOf(id string) *d2items.Kit {
	if c.kits == nil || id == "" {
		return nil
	}

	return c.kits.KitOf(id)
}
