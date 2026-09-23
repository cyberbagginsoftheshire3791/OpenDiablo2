package d2world

// T3, the edges (23 Sep 2026): what his talents change about a fight, read by
// the resolver from the game screen through one narrow seam -- the Kits,
// Animator and Stepper precedent. d2world knows nothing of the talent tree;
// it knows only the numbers the tree adds up to.
//
// THE ZERO VALUE IS NEUTRAL, so a nil Edges, an unknown id, or a hero with no
// talents fights exactly as before T3: multipliers read 0 as 1, and every
// other field adds nothing.

// Edge is one combatant's talents, as numbers.
type Edge struct {
	RiposteDamage  float64 // x a riposte's damage (0 = 1)
	ExtraBlocks    int     // blows the shield turns a round beyond the first
	ExtraMove      int     // tiles added to his Move
	CritBand       int     // added to his blows' crit band
	ExtraReactions int     // Reactions a round beyond the first
	AdvantageBonus int     // added to his dark-into-light advantage
	KillNerve      float64 // x the nerve a pack loses to a death he dealt (0 = 1)
	LitNerve       float64 // nerve a beast's pack loses to his hit (set only while his torch burns)
}

// Edges is how the resolver reads them.
type Edges interface {
	EdgeOf(id string) Edge
}

// SetEdges attaches the edge source.
func (c *Combat) SetEdges(e Edges) { c.edges = e }

func (c *Combat) edgeOf(id string) Edge {
	if c.edges == nil || id == "" {
		return Edge{}
	}

	return c.edges.EdgeOf(id)
}

func orOne(v float64) float64 {
	if v <= 0 {
		return 1
	}

	return v
}

// reactionsLeft reports whether the player may still react this round: one
// Reaction a round, plus his Drill (R2 §3 bullet 6's cap, deepened).
func (c *Combat) reactionsLeft() bool {
	e := c.encounter
	if e == nil || e.target == nil {
		return false
	}

	if e.reactionUsedInRound != e.round {
		return true
	}

	return e.reactionsInRound < 1+c.edgeOf(e.target.QuarryID()).ExtraReactions
}

// spendReaction counts one Reaction against the round.
func (c *Combat) spendReaction() {
	e := c.encounter
	if e.reactionUsedInRound != e.round {
		e.reactionsInRound = 0
	}

	e.reactionsInRound++
	e.reactionUsedInRound = e.round
}

// blocksLeft and spendBlock are the shield's version of the same count.
func (c *Combat) blocksLeft(round int, targetID string) bool {
	e := c.encounter
	if e.blockUsedInRound != round {
		return true
	}

	return e.blocksInRound < 1+c.edgeOf(targetID).ExtraBlocks
}

func (c *Combat) spendBlock(round int) {
	e := c.encounter
	if e.blockUsedInRound != round {
		e.blocksInRound = 0
	}

	e.blocksInRound++
	e.blockUsedInRound = round
}

// --- experience ---------------------------------------------------------------

// XPEvent is one thing a fight did that earns experience. The game screen owns
// what each is worth (data/strigoi/talents.json); combat only says it happened.
type XPEvent struct {
	Kind string // "slain" or "routed"
	Row  string // the enemy's spawn row, "" for anything the tables never placed
}

// TakeXPEvents hands over what happened since the last call and forgets it.
func (c *Combat) TakeXPEvents() []XPEvent {
	out := c.xpEvents
	c.xpEvents = nil

	return out
}

func (c *Combat) earn(kind, enemyID string) {
	c.xpEvents = append(c.xpEvents, XPEvent{Kind: kind, Row: c.profileOf(enemyID).Row})
}
