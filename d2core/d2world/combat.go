package d2world

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// Combat is M4.5's encounter model: it decides that a fight is happening, who
// is in it, and whose turn it is. It does NOT yet decide what a blow does --
// that is the resolver, and it comes next.
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
	dials   CombatDials
	clock   *Clock
	notice  *Notice
	fitness Fitness
	illum   Illumination

	rng *rand.Rand

	encounter *encounter
	nextID    int

	// Counters, all reported. An encounter that started and ended between two
	// harness reads is invisible in the state and obvious in the counters.
	started  int
	ended    int
	rounds   int
	declines int
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
	AdjacentTiles int
}

// DefaultCombatDials returns the signed starting values.
func DefaultCombatDials() CombatDials {
	return CombatDials{
		RoundMinutes:  1.0,
		AdjacentTiles: 1,
	}
}

// encounter is one fight.
type encounter struct {
	id      string
	target  Quarry
	enemies []Combatant

	// order is the activation sequence, and it is PROVISIONAL.
	//
	// R2 §3 signs that activations are initiative-ordered and says nothing
	// whatever about what determines the order; initiative is absent from R2
	// §5's open-dials list, so it is unspecified rather than undialled, and
	// D8 -- combat initiation and initiative order -- is an open Research
	// Topic that has not started.
	//
	// Josh ruled on 29 Aug: ship a deliberately dull, seeded order and REPORT
	// it, so D8 can replace it without touching anything else. A reported
	// order is replaceable; an unreported one is a decision in hiding. The
	// shuffle is deliberately arbitrary rather than clever, because a clever
	// order would look like a rule somebody had decided.
	order []string

	round     int
	sinceTurn float64
}

// NewCombat builds the encounter model and registers the "combat" provider.
//
// seed is the run's seed: the provisional activation order is drawn from this
// system's own RNG so that two launches of one build at one seed produce the
// same order, which is R2 §3's determinism clause applied to the one part of
// this milestone that is arbitrary by design.
func NewCombat(clock *Clock, notice *Notice, fitness Fitness, illum Illumination,
	seed int64, dials CombatDials) *Combat {
	c := &Combat{
		dials:   dials,
		clock:   clock,
		notice:  notice,
		fitness: fitness,
		illum:   illum,
		rng:     rand.New(rand.NewSource(seed)), // nolint:gosec // gameplay RNG, seeded for reproducibility
		nextID:  1,
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
	for c.encounter.sinceTurn >= c.dials.RoundMinutes {
		c.encounter.sinceTurn -= c.dials.RoundMinutes
		c.encounter.round++
		c.rounds++
	}

	c.pruneOrEnd()
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

	c.encounter = &encounter{
		id:      fmt.Sprintf("e:%d", c.nextID),
		target:  target,
		enemies: enemies,
		round:   1,
		order:   c.provisionalOrder(enemies),
	}

	c.nextID++
	c.started++
	c.rounds++
}

// provisionalOrder is the seeded shuffle described on encounter.order. It is
// arbitrary on purpose and its arbitrariness is the point: D8 has not started,
// and a plausible-looking order would be mistaken for a decided one.
func (c *Combat) provisionalOrder(enemies []Combatant) []string {
	ids := make([]string, 0, len(enemies))
	for _, e := range enemies {
		ids = append(ids, e.WatcherID())
	}

	// Sort first so the shuffle's input does not depend on map iteration
	// order upstream, then shuffle from this system's own seeded RNG.
	sort.Strings(ids)
	c.rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })

	return ids
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

	kept := e.enemies[:0]

	for _, enemy := range e.enemies {
		if c.inReach(enemy, e.target) {
			kept = append(kept, enemy)
		}
	}

	e.enemies = kept

	if len(e.enemies) == 0 {
		c.encounter = nil
		c.ended++

		return
	}

	// Keep the reported order honest: an id that left the fight must leave the
	// order with it, or a script reading `order` sees a participant that is
	// no longer there.
	live := make(map[string]bool, len(e.enemies))
	for _, enemy := range e.enemies {
		live[enemy.WatcherID()] = true
	}

	order := e.order[:0]

	for _, id := range e.order {
		if live[id] {
			order = append(order, id)
		}
	}

	e.order = order
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

// Order reports the provisional activation sequence, or nil.
func (c *Combat) Order() []string {
	if c.encounter == nil {
		return nil
	}

	out := make([]string, len(c.encounter.order))
	copy(out, c.encounter.order)

	return out
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
	state["order"] = append([]string{}, e.order...)
	state["minutes_into_round"] = e.sinceTurn

	parts := make([]map[string]interface{}, 0, len(e.enemies)+1)

	// The player side first, and it carries the two facts M4.2 built for this
	// milestone to read. They are reported here as well as on the meters
	// provider deliberately: an assertion about a FIGHT should be readable
	// from the fight, not assembled from two providers by the script.
	if e.target != nil {
		px, py := e.target.QuarryAt()

		row := map[string]interface{}{
			"id":       e.target.QuarryID(),
			"side":     "player",
			"x":        px,
			"y":        py,
			"adjacent": true,
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

		row := map[string]interface{}{
			"id":       enemy.WatcherID(),
			"side":     "enemy",
			"x":        ex,
			"y":        ey,
			"adjacent": c.inReach(enemy, e.target),
		}

		if c.illum != nil {
			row["light_here"] = c.illum.Level(int(math.Floor(ex)), int(math.Floor(ey)))
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
func (c *Combat) HarnessSettableFields() []string {
	return []string{"adjacent_tiles", "disengage", "round", "round_minutes"}
}

// HarnessSet writes one allow-listed field.
func (c *Combat) HarnessSet(field string, value interface{}) error {
	switch field {
	case "round_minutes":
		v, ok := value.(float64)
		if !ok || v <= 0 {
			return fmt.Errorf("round_minutes wants a positive number, got %v", value)
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

		c.encounter = nil
		c.ended++

	default:
		return fmt.Errorf("combat has no settable field %q", field)
	}

	return nil
}
