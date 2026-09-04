package d2world

import (
	"math"
	"sort"
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

// resolveRound runs one round: every activation in order, until the round is
// done or a blow has ended the fight.
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

	playerID := e.target.QuarryID()

	// The sequence is taken once, at the top of the round. Something that
	// dies mid-round is skipped by the liveness test in enemyActivation
	// rather than by rebuilding the list underneath the loop.
	for _, id := range e.activation() {
		// A blow can end the encounter (the player died, or that was the last
		// of them). Nothing further happens in a fight that is over.
		if c.encounter == nil {
			return
		}

		if id == playerID {
			c.playerActivation()

			continue
		}

		c.enemyActivation(id)
	}
}

// playerActivation is the stand-in policy: strike the first adjacent living
// enemy in the order, or do nothing on "hold".
func (c *Combat) playerActivation() {
	e := c.encounter
	if e == nil || e.target == nil || c.dials.PlayerAction != PlayerActionAttack {
		return
	}

	for _, id := range e.enemyOrder {
		if e.dead[id] {
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
	if e == nil || e.target == nil || e.dead[id] {
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
	c.encounter.reactionUsedInRound = c.encounter.round

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

	if c.lastActions[idx].band != BandGraze {
		return false
	}

	// A Riposte cannot itself trigger a Riposte: the enemy side has no
	// reactions in v0.
	if c.lastActions[idx].reaction != "" {
		return false
	}

	if c.fitness == nil || !c.fitness.ReactionAvailable() || c.fitness.Shaken() {
		return false
	}

	// One per round (R2 §3 bullet 6's own cap), and none at all in a
	// surprised round one (D8 §9: the caught-head-down player's Reaction is
	// unavailable that round).
	if e.reactionUsedInRound == e.round || (e.round == 1 && e.surprised) {
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

	// Shaken costs the PLAYER accuracy. An enemy is never Shaken in v0 -- the
	// condition is a fact about the player's body and nothing computes one
	// for a beast.
	if attackerIsPlayer && c.fitness != nil && c.fitness.Shaken() {
		mod -= c.dials.ShakenPenalty
		why = joinWhy(why, whyShaken)
	}

	score := clampScore(roll + mod)

	band := c.bandFor(score)
	if c.dials.ForcedBand != "" {
		band = c.dials.ForcedBand
	}

	// The floor of 1 is what makes "bounded" true at the bottom: a graze is a
	// small wound, never nothing.
	damage := int(float64(base) * c.factorFor(band))
	if damage < 1 {
		damage = 1
	}

	a := action{
		round: round, attacker: attackerID, target: targetID,
		roll: roll, mod: mod, score: score, band: band,
		base: base, damage: damage,
		advantageWhy: why, reaction: reaction,
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

	idx := len(c.lastActions) - 1

	switch {
	case a.targetHasBody && a.targetHealthAfter <= 0:
		c.reachedZero(targetID)
	case a.targetHasBody:
		c.animate(targetID, ActHit)
	}

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
// activation, and it STAYS on the map, in its spawn group, with its chase
// still running and its participant row reporting dead:true. The resolver
// despawns nothing (the fence). N1's "dead is dead" is honoured and nothing
// pretends it is a corpse yet; the corpse machine is M4.7's, and Notice.Unwatch
// and Pursuit.Release are step 5's.
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

	c.animate(id, ActDie)

	kept := e.enemyOrder[:0]

	for _, x := range e.enemyOrder {
		if x != id {
			kept = append(kept, x)
		}
	}

	e.enemyOrder = kept

	for _, enemy := range e.enemies {
		if enemy != nil && !e.dead[enemy.WatcherID()] {
			return
		}
	}

	c.end("enemies_dead")
}

// advantage is R2 §3 bullet 7, on ABSOLUTE levels against one threshold --
// deliberately the same threshold the notice model calls lit, so that "a wolf
// can see you" and "you are lit for the wolf's blow" are one fact (S1 §4).
//
// IT CANNOT FIRE IN A v0 BUILD, AND THAT IS MEASURED RATHER THAN SUSPECTED.
// A pursuer's route ends on the quarry's OWN tile -- entities do not block the
// search -- so it walks to distance 0.000 and stops there, and every
// participant in a settled fight floors to one tile and therefore samples one
// light level. Measured 3 Sep 2026 at 696edbf2: twelve world minutes, two
// monsters, every light_here reading identical to the player's.
//
// The rule stays, for the same reason "Shaken blocks Riposte" stays: it is
// right, it is asserted in the unit tests with fakes, and the day a pursuer
// stops on an ADJACENT tile it becomes visible with no change here. That one
// change -- step 5's, or M4.3a's -- is also what flanking and facing need.
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
