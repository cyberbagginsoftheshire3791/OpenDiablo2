package d2world

// THE FIGHTS HE IS NOT IN: the combat model's second driver (the raid
// milestone "the village at night", burst R1, 29 Sep 2026).
//
// Ruling 1 of 28 Sep: "fights they are in resolve without the player: ONE
// RESOLVER, TWO DRIVERS (the player's commit; the world clock, at R2's one
// round = one world minute)". c.encounter and Fighting() keep meaning HIS
// fight -- or, with no player bound, the one fight, exactly as before (the
// legacy rule every unit fixture runs on). A fight whose quarry is not him is
// a CLOCK FIGHT: it lives in c.clockFights, is never paced and never waits,
// and resolves one round per world minute on the world-time loop the
// resolver has always had (Advance's, as the policy runs it).
//
// ONE RESOLVER, BY A BOOK SWAP (measured on the throwaway spike raid-s0-spike,
// aad5f4ee: the 288 d2world tests green with no assertion edited). Every
// per-fight record the resolver writes on Combat -- the encounter pointer,
// the dice stream, the dials, the next id, experience, owed minutes, the
// logs, the round and pace rows, the timers and every counter -- is
// exchanged with the clock's book for the length of one clock fight's step,
// and exchanged back. The resolver's code is untouched: it cannot tell which
// driver it runs for except through encounter.driver, which three guards
// read (reachedZero, earn and the provider). HIS RECORDS STAY HIS BY
// CONSTRUCTION: while a clock fight steps, they are not on the struct.
//
// What a clock fight is, fixed here:
//   - its own id sequence, c:<n> (the raid's S0-3, default (a)), so his
//     e:<n>, next_id and B1's next_id == started + 1 never move with the
//     village's fights;
//   - its own stream, combat-clock, derived from the one seed NewCombat is
//     handed (d2rand.Rederive), so his dice never move either (M0.1d);
//   - its quarry's death is not his: a man's body falls, every watch on the
//     dead is released in the same frame (the wedge, BUG-66), and the fight
//     ends quarry_dead -- never player_dead;
//   - it earns him nothing: earn is suppressed and counted (xp_suppressed);
//   - ONE LIVE FIGHT PER COMBATANT (the raid R1 review's A1, BUG-67): the
//     separation is by combatant as well as by record. A monster or a
//     quarry already in a live fight -- his or the village's -- joins no
//     second one (scanAware); a clock fight lets go of a living enemy whose
//     watch has moved to someone else (pruneOrEnd), and his fight wins that
//     tie; and a body at 0 is gone in every fight, whichever fight killed it
//     (the activations, stillIn, and stepClock's dead quarry);
//   - it is saved with the world (the raid's Q5, default (a): a fight he is
//     not in never stops a save) -- combat_snapshot.go's clock block.

import (
	"sort"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// The two drivers. driverPlayer is the zero value, so every encounter tryStart
// makes is his until the clock driver marks it.
const (
	driverPlayer = ""
	driverClock  = "clock"
)

// clockIDPrefix is a clock fight's id prefix (S0-3 (a)): "c:<n>", from the
// clock book's own sequence. tryStart writes "e:<n>" from whichever next id
// is on the struct; the clock driver relabels what it opened.
const clockIDPrefix = "c:"

// fightBook is every per-fight record the resolver writes on the Combat
// struct, for the clock fights. His live on the struct; the clock's live
// here; swapBook exchanges them around a clock step.
//
// The fields after the marker are the clock driver's own and are NEVER
// swapped: counters only the clock keeps, and the step flag his accessors
// read.
type fightBook struct {
	encounter      *encounter
	rng            *d2rand.Stream
	dials          CombatDials
	nextID         int
	xpEvents       []XPEvent
	killerIsPlayer bool
	owedMinutes    float64
	blowLog        []BlowLine
	stepsOrdered   int
	lastActions    []action
	actionsRound   int

	started, ended, rounds int

	decisionSeconds, decisionSecondsRound float64
	paceOpen                              bool
	wallSeconds                           float64
	paceHealthOpen, paceCommits           int

	lastActionVerb string
	lastRound      RoundRow
	lastPace       PaceRow
	declines       int
	actions        int

	endedReason                                                   string
	endedEnemiesDead, endedDawn, endedPlayerDead, endedDisengaged int
	endedRouted                                                   int
	joined, quickResolved                                         int
	lastQuickAdvantage                                            float64

	// --- the clock driver's own; never swapped ---------------------------

	// stepping is true for exactly the length of one clock step (withClock).
	// Fighting, Encounter and Awaiting read it: see Combat.his.
	stepping bool

	// quarryDead counts clock fights that ended with their quarry dead,
	// xpSuppressed the kills and routs in clock fights that earned him
	// nothing, released the watches let go on a dead clock quarry, and
	// reentrantReads the reads of Fighting, Encounter or Awaiting made while
	// a clock fight stepped (R1 assertion 9 holds it at zero).
	quarryDead, xpSuppressed, released, reentrantReads int
}

// newFightBook is the clock's book for a combat model handed seed, the
// StreamCombat seed: the clock fights draw from the combat-clock stream of
// the same game (d2rand.Rederive), so NewCombat takes no new argument.
func newFightBook(seed int64) *fightBook {
	return &fightBook{
		rng:    d2rand.NewStream(d2rand.Rederive(seed, d2rand.StreamCombat, d2rand.StreamCombatClock)),
		nextID: 1,
	}
}

// swapBook exchanges the struct's per-fight records with b. It is its own
// inverse: swap, run, swap.
func (c *Combat) swapBook(b *fightBook) {
	c.encounter, b.encounter = b.encounter, c.encounter
	c.rng, b.rng = b.rng, c.rng
	c.dials, b.dials = b.dials, c.dials
	c.nextID, b.nextID = b.nextID, c.nextID
	c.xpEvents, b.xpEvents = b.xpEvents, c.xpEvents
	c.killerIsPlayer, b.killerIsPlayer = b.killerIsPlayer, c.killerIsPlayer
	c.owedMinutes, b.owedMinutes = b.owedMinutes, c.owedMinutes
	c.blowLog, b.blowLog = b.blowLog, c.blowLog
	c.stepsOrdered, b.stepsOrdered = b.stepsOrdered, c.stepsOrdered
	c.lastActions, b.lastActions = b.lastActions, c.lastActions
	c.actionsRound, b.actionsRound = b.actionsRound, c.actionsRound
	c.started, b.started = b.started, c.started
	c.ended, b.ended = b.ended, c.ended
	c.rounds, b.rounds = b.rounds, c.rounds
	c.decisionSeconds, b.decisionSeconds = b.decisionSeconds, c.decisionSeconds
	c.decisionSecondsRound, b.decisionSecondsRound = b.decisionSecondsRound, c.decisionSecondsRound
	c.paceOpen, b.paceOpen = b.paceOpen, c.paceOpen
	c.wallSeconds, b.wallSeconds = b.wallSeconds, c.wallSeconds
	c.paceHealthOpen, b.paceHealthOpen = b.paceHealthOpen, c.paceHealthOpen
	c.paceCommits, b.paceCommits = b.paceCommits, c.paceCommits
	c.lastActionVerb, b.lastActionVerb = b.lastActionVerb, c.lastActionVerb
	c.lastRound, b.lastRound = b.lastRound, c.lastRound
	c.lastPace, b.lastPace = b.lastPace, c.lastPace
	c.declines, b.declines = b.declines, c.declines
	c.actions, b.actions = b.actions, c.actions
	c.endedReason, b.endedReason = b.endedReason, c.endedReason
	c.endedEnemiesDead, b.endedEnemiesDead = b.endedEnemiesDead, c.endedEnemiesDead
	c.endedDawn, b.endedDawn = b.endedDawn, c.endedDawn
	c.endedPlayerDead, b.endedPlayerDead = b.endedPlayerDead, c.endedPlayerDead
	c.endedDisengaged, b.endedDisengaged = b.endedDisengaged, c.endedDisengaged
	c.endedRouted, b.endedRouted = b.endedRouted, c.endedRouted
	c.joined, b.joined = b.joined, c.joined
	c.quickResolved, b.quickResolved = b.quickResolved, c.quickResolved
	c.lastQuickAdvantage, b.lastQuickAdvantage = b.lastQuickAdvantage, c.lastQuickAdvantage
}

// clockDials is what a clock fight runs under: his dials, with the driver's
// three switched to the world-time policy -- never paced, never waiting for a
// person, the living side striking the first adjacent enemy. Recomputed at
// every step, so a harness write to round_minutes or forced_band reaches both
// drivers alike.
func clockDials(d CombatDials) CombatDials {
	d.Paced = false
	d.PlayerControl = PlayerControlPolicy
	d.PlayerAction = PlayerActionAttack

	return d
}

// withClock runs fn with e as c.encounter under the clock's book, and returns
// the encounter as fn left it (nil if fn ended it, or opened none). The swap
// back is deferred, so nothing fn does can leave his records off the struct.
func (c *Combat) withClock(e *encounter, fn func()) (out *encounter) {
	b := c.clockBook
	b.encounter, b.dials = e, clockDials(c.dials)

	c.swapBook(b)
	b.stepping = true

	defer func() {
		b.stepping = false
		c.swapBook(b)

		out, b.encounter = b.encounter, nil
	}()

	fn()

	return nil
}

// his is the player's encounter -- even in the middle of a clock step, when
// the book swap has put a clock fight on c.encounter. Every reader of his
// fight the game has goes through it or through midStep, so a reader reached
// from a callback a clock step makes (Animate, Chases.Release,
// Notice.Unwatch, Corpses.Fall/FallHuman, Morale.Hurt, and whatever the game
// hangs on them) still gets HIS answer: Fighting, Encounter, Awaiting, Round,
// Order, Participates, ActionSpent and MoveSpent read his encounter here;
// WorldHeld, Paced, LastRound, LastPace and EndedReason read his records from
// the book (midStep); Tactical reads them with the swap undone (asHis). Every
// such read is counted (clock.reentrant_reads): none is made today, R1
// assertion 9 holds it at zero, and a count that moves names a new reader to
// look at. (The R1 review's C2, BUG-71: before it, three of those readers were
// guarded.)
//
// THE LIMIT, NAMED: what is not a read is not guarded. His verbs (Commit,
// SpendMove, Wait, Tick), the frame's takes (TakeXPEvents, TakeRoundMinutes)
// and the harness provider (HarnessState) act on whatever is on the struct.
// Each is called by the game's frame or by a script between frames, never
// from a callback of a clock step; one reached from such a callback would act
// on the clock fight's records.
func (c *Combat) his() *encounter {
	if c.midStep() {
		return c.clockBook.encounter
	}

	return c.encounter
}

// midStep reports that a clock fight is stepping -- his records are in the
// clock's book and the clock fight's on the struct -- and counts the read
// (clock.reentrant_reads). Every guarded reader asks it once.
func (c *Combat) midStep() bool {
	if b := c.clockBook; b != nil && b.stepping {
		b.reentrantReads++

		return true
	}

	return false
}

// asHis runs read with HIS records on the struct: between clock steps they are
// there already; mid-step the book swap is undone for the length of the read
// and redone after it, and the read is counted as midStep counts one. read
// must only read.
func (c *Combat) asHis(read func()) {
	b := c.clockBook
	if !c.midStep() {
		read()

		return
	}

	b.stepping = false
	c.swapBook(b)

	defer func() {
		c.swapBook(b)
		b.stepping = true
	}()

	read()
}

// fightingElsewhere is every combatant of a live fight other than the one on
// the struct: each fight's quarry, and each of its enemies that has not left
// it (dead, routed or broken off). scanAware lets none of them into a second
// fight (the raid R1 review's A1, BUG-67). The fight on the struct is the one
// scanning -- his, or a clock fight under its book -- and its own are its
// own; his fight, mid clock step, is in the book. Nil when there is no other
// live fight, which is always on the legacy path.
func (c *Combat) fightingElsewhere() map[string]bool {
	his := c.encounter
	if b := c.clockBook; b != nil && b.stepping {
		his = b.encounter
	}

	var out map[string]bool

	add := func(e *encounter) {
		if e == nil || e == c.encounter {
			return
		}

		if out == nil {
			out = map[string]bool{}
		}

		if e.target != nil {
			out[e.target.QuarryID()] = true
		}

		for _, en := range e.enemies {
			if en != nil && !e.gone(en.WatcherID()) {
				out[en.WatcherID()] = true
			}
		}
	}

	add(his)

	for _, e := range c.clockFights {
		add(e)
	}

	return out
}

// engaged reports that id is a living enemy of a live fight -- his, or one he
// is not in -- that has not left it (dead, routed or broken off): the raid's
// R2, "a fighter keeps its target". Seek asks it between frames and never
// retargets such a watcher, so a monster at his throat stays at his throat
// (J-R1-3, default (a)) and a monster killing a villager finishes the job.
// A nil model has no fights.
func (c *Combat) engaged(id string) bool {
	if c == nil {
		return false
	}

	in := func(e *encounter) bool {
		if e == nil || e.gone(id) {
			return false
		}

		for _, en := range e.enemies {
			if en != nil && en.WatcherID() == id {
				return true
			}
		}

		return false
	}

	// His fight, mid clock step, is in the book -- read without counting,
	// as fightingElsewhere reads it: Seek is no callback of a clock step.
	his := c.encounter
	if b := c.clockBook; b != nil && b.stepping {
		his = b.encounter
	}

	if in(his) {
		return true
	}

	for _, e := range c.clockFights {
		if in(e) {
			return true
		}
	}

	return false
}

// awareOfAnother reports a watcher whose noticed watch names someone other
// than q: a clock fight's enemy whose watch has moved on (pruneOrEnd).
func (c *Combat) awareOfAnother(id string, q Quarry) bool {
	if c.notice == nil || q == nil {
		return false
	}

	w := c.notice.watches[id]

	return w != nil && w.noticed && w.target != nil && w.target.QuarryID() != q.QuarryID()
}

// SetPlayer binds the player's entity id, as Squads.BindPlayer is bound: the
// game screen binds it every frame, beside Advance. With a player bound, a
// fight opens as his only on a pair whose target is he, and a hostile aware
// of anyone else opens a clock fight. With none bound -- every unit fixture
// -- nothing changes: the legacy rule, at most one encounter, driven as the
// dials say.
func (c *Combat) SetPlayer(id string) { c.player = id }

// SetResolver attaches what turns a saved clock fight's ids back into live
// things (M4.6 B2b's Resolver): the game screen's worldResolver, over the live
// map at a save and the rebuilt one at a load. Validate and Restore need it
// only for a snapshot that holds live clock fights; without one, such a
// snapshot is refused (ErrUnresolvedRef), never half restored.
func (c *Combat) SetResolver(r Resolver) { c.resolver = r }

// SetProtected attaches the rule for who is no quarry (the raid's S0-1,
// default (a)): the four speakers are 1-HP stand-ins with no death to show
// -- two die standing and two vanish (M0.5) -- so until their death art (A5)
// lands no fight opens on them. The scan skips a pair whose target the rule
// names, as it skips a dead target; the speakers stay on the map, watched
// or not, and are full entities in every other way. Nil protects nobody.
func (c *Combat) SetProtected(protected func(id string) bool) { c.protected = protected }

// quarryNamed finds the Quarry value an aware pair holds for id, or nil.
func (c *Combat) quarryNamed(id string) Quarry {
	if c.notice == nil {
		return nil
	}

	for _, p := range c.notice.AwarePairs() {
		if p.Target != nil && p.Target.QuarryID() == id {
			return p.Target
		}
	}

	return nil
}

// advanceClock is the second driver: every clock fight's rounds on the world
// minutes that passed, then a fight opened for every aware quarry that is
// neither him nor already fought over.
//
// LOCKSTEP WITH HIS PACED FIGHT COMES FREE: the game screen does not advance
// the world while his fight holds it, and pays each of his closed rounds back
// as one round's world minutes (TakeRoundMinutes), so a clock fight takes
// exactly one round per round of his that closes, and none while his turn is
// open (M0.1c, measured).
//
// A fight that ends is taken out of the list in its own slot, at once, and the
// list closed up after the loop (dropEndedClock), so a later fight's scan in
// the same frame never takes an ended fight's enemies for a live fight's
// (fightingElsewhere).
func (c *Combat) advanceClock(worldMinutes float64) {
	for i, e := range c.clockFights {
		c.clockFights[i] = c.withClock(e, func() { c.stepClock(worldMinutes) })
	}

	c.dropEndedClock()

	c.openClockFights()
}

// openClockFights opens a clock fight for each quarry an aware pair names that
// is neither him nor already in a clock fight -- one quarry per fight, as his
// fight has one (v0's named limit: a villager fights what came for him, and
// two villagers do not yet gang up on one risen). A reinforcement to a quarry
// already fought over is its fight's own reinforce, as his is.
func (c *Combat) openClockFights() {
	if c.notice == nil {
		return
	}

	taken := map[string]bool{c.player: true}

	for _, e := range c.clockFights {
		if e.target != nil {
			taken[e.target.QuarryID()] = true
		}
	}

	for _, p := range c.notice.AwarePairs() {
		if p.Target == nil || taken[p.Target.QuarryID()] {
			continue
		}

		q := p.Target
		taken[q.QuarryID()] = true

		if out := c.withClock(nil, func() { c.tryStartFor(q) }); out != nil {
			out.driver = driverClock
			out.id = clockIDPrefix + strings.TrimPrefix(out.id, "e:")
			c.clockFights = append(c.clockFights, out)
		}
	}
}

// stepClock is the world-time loop of Advance, for the encounter the clock
// book holds: the same code, under dials that are never paced and never
// human, so the loop never waits.
func (c *Combat) stepClock(worldMinutes float64) {
	e := c.encounter
	if e == nil {
		return
	}

	// A QUARRY ALREADY AT 0 ENDS ITS FIGHT BEFORE ANY ROUND (the raid R1
	// review's A1, BUG-67). Something else killed him -- another fight, or a
	// path that is no fight's -- so this one has nothing left to fight: it
	// ends quarry_dead, and the watches on him go in this frame as they go
	// when a clock fight kills him. No body falls and no death plays: he fell
	// where he died, by what killed him. Before it, the dead quarry took his
	// activation and struck, and the blow that answered him fell a second
	// body and played a second death on the corpse.
	if e.target != nil && c.deadByBody(e.target.QuarryID()) {
		c.clockBook.released += c.releaseWatchesOn(e.target.QuarryID())
		c.clockBook.quarryDead++
		c.end("quarry_dead")

		return
	}

	e.sinceTurn += worldMinutes

	for c.encounter != nil && c.encounter.sinceTurn >= c.dials.RoundMinutes-roundEpsilon {
		c.encounter.sinceTurn -= c.dials.RoundMinutes

		c.resolveRound()
		c.finishRound()
	}

	if c.encounter != nil {
		c.reinforce()
		c.pruneOrEnd()
	}
}

// quarryDead is a clock fight's quarry at 0 (reachedZero's clock branch): a
// man's body falls where he stood and he plays his death, every watch on him
// is let go in the same frame -- B2's wedge fixed at its source, BUG-66 -- and
// the fight ends quarry_dead. Never player_dead: that is the death screen's
// word, and his book is not on the struct to take it.
func (c *Combat) quarryDead(e *encounter) {
	id := e.target.QuarryID()

	if c.corpses != nil {
		x, y := e.target.QuarryAt()
		c.corpses.FallHuman(id, "villager", x, y)
	}

	c.animate(id, ActDie)

	c.clockBook.released += c.releaseWatchesOn(id)
	c.clockBook.quarryDead++

	c.end("quarry_dead")
}

// releaseWatchesOn withdraws every watcher whose watch is on id -- its watch
// and its chase, withdraw's both halves -- and says how many. In watcher-id
// order, so two launches let them go alike.
func (c *Combat) releaseWatchesOn(id string) int {
	if c.notice == nil {
		return 0
	}

	n := 0

	for _, wid := range c.notice.watcherIDs() {
		if w := c.notice.watches[wid]; w != nil && w.target != nil && w.target.QuarryID() == id {
			c.withdraw(wid)
			n++
		}
	}

	return n
}

// dropEndedClock takes the fights a BreakOff or a Rejoin ended out of the list.
func (c *Combat) dropEndedClock() {
	live := c.clockFights[:0]

	for _, e := range c.clockFights {
		if e != nil {
			live = append(live, e)
		}
	}

	for i := len(live); i < len(c.clockFights); i++ {
		c.clockFights[i] = nil
	}

	c.clockFights = live
}

// clockState is the provider's clock block: the clock fights, and the clock
// book's records. EVERY TOP-LEVEL KEY OF THE PROVIDER KEEPS MEANING HIS FIGHT
// (the rule the 18 playtest files that read it rely on); this block is where
// the fights he is not in are reported.
func (c *Combat) clockState() map[string]interface{} {
	live := make([]map[string]interface{}, 0, len(c.clockFights))

	for _, e := range c.clockFights {
		enemies := make([]map[string]interface{}, 0, len(e.enemies))

		for _, en := range e.enemies {
			if en == nil {
				continue
			}

			id := en.WatcherID()
			row := map[string]interface{}{"id": id, "dead": e.dead[id], "routed": e.routed[id], "broke": e.broke[id]}

			if body := c.bodyOf(id); body != nil {
				row["health"] = body.CurrentHealth()
			}

			enemies = append(enemies, row)
		}

		quarry, quarryHealth, quarryMax := "", -1, -1

		if e.target != nil {
			quarry = e.target.QuarryID()

			if body := c.bodyOf(quarry); body != nil {
				quarryHealth, quarryMax = body.CurrentHealth(), body.MaxHealth()
			}
		}

		live = append(live, map[string]interface{}{
			"id": e.id, "quarry": quarry, "quarry_health": quarryHealth, "quarry_max_health": quarryMax,
			"round": e.round, "minutes_into_round": e.sinceTurn,
			"enemies": enemies, "enemy_order": append([]string{}, e.enemyOrder...), "order": e.activation(),
			"dead": sortedTrue(e.dead), "routed": sortedTrue(e.routed), "broke": sortedTrue(e.broke),
			"initiator": e.initiator, "surprised": e.surprised, "surprise_why": e.surpriseWhy,
		})
	}

	b := c.clockBook

	// The clock's last round's blows, read the way his are (actionRows), with
	// its book on the struct for the length of the read.
	c.swapBook(b)
	rows := c.actionRows()
	c.swapBook(b)

	return map[string]interface{}{
		"live":                 live,
		"next_id":              b.nextID,
		"started":              b.started,
		"ended":                b.ended,
		"rounds":               b.rounds,
		"declined_reach":       b.declines,
		"actions_total":        b.actions,
		"actions_round":        b.actionsRound,
		"actions":              rows,
		"joined":               b.joined,
		"quick_resolved":       b.quickResolved,
		"last_quick_advantage": b.lastQuickAdvantage,
		"ended_reason":         b.endedReason,
		"ended_quarry_dead":    b.quarryDead,
		"ended_enemies_dead":   b.endedEnemiesDead,
		"ended_player_dead":    b.endedPlayerDead,
		"ended_routed":         b.endedRouted,
		"ended_disengaged":     b.endedDisengaged,
		"ended_dawn":           b.endedDawn,
		"xp_suppressed":        b.xpSuppressed,
		"released":             b.released,
		"reentrant_reads":      b.reentrantReads,
		"rng":                  b.rng.Report(),
	}
}

// sortedTrue is the keys of a set whose value is true, sorted: how a clock
// fight's dead, routed and broke are written.
func sortedTrue(set map[string]bool) []string {
	var out []string

	for id, ok := range set {
		if ok {
			out = append(out, id)
		}
	}

	sort.Strings(out)

	return out
}
