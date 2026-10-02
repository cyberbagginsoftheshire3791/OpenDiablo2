package d2world

import (
	"fmt"
	"math"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// Pursuit is the fourth d2world system, beside Clock, Light and Meters, and
// the second half of M4.3a: the A* answers "what is the route", and this
// answers "keep taking it while the thing you want keeps moving".
//
// It is deliberately NOT awareness. Nothing here decides WHEN a hunter should
// start chasing -- that is M4.3b's notice model, which belongs with the tables
// that made the thing. A chase is started by whoever knows it should start,
// and this keeps it honest afterwards.
//
// Like every other system here it steps on the world clock, never the wall
// clock, and it talks to the map through interfaces so d2world keeps importing
// no map engine and no ebiten. That is also what makes it testable: the tests
// below run on a hand-built grid with no MPQs.
type Pursuit struct {
	dials  PursuitDials
	router Router
	chases map[string]*chase
	// solves counts route solutions since construction. It is reported, so a
	// script can assert that a chase re-pathed rather than merely arrived --
	// re-pathing is the behaviour, arriving is only its consequence.
	solves int

	// rechases counts the solves Chase made replacing a chase on ANOTHER
	// quarry (the raid R2 review's B2): the cost of a retarget, which Seek
	// does not pay itself -- the game restarts the chase on the moved watch
	// in the same frame, and Chase solves at once. Included in solves.
	rechases int

	// rechasedThisFrame counts the rechases since the last Advance, against
	// RechasesPerFrame (the R2 review B's B2). Advance zeroes it before the
	// frame's chases are started again (Game.advanceWorld steps Pursuit
	// before startChasesForTheAware), so its value between two frames is
	// never read: not saved.
	rechasedThisFrame int

	// rechasesDeferred counts the Rechase calls the budget turned away (each
	// asked again the next frame). Reported for the benchmark and the tests;
	// not in the provider, so not saved.
	rechasesDeferred int

	// solvedThisFrame counts the A* solves (Router.Route calls) since the
	// last Advance, against SolvesPerFrame (the per-frame solve budget, 2
	// Oct 2026). Like rechasedThisFrame, Advance zeroes it before it serves
	// anything, so its value between two frames is never read: not saved.
	solvedThisFrame int

	// advancedThisFrame is whether the last Advance stepped the world (its
	// minutes were more than zero). ServeQueued serves nothing in a frame
	// whose world stood still -- a held fight's frame -- as Advance does not.
	// Set by every Advance before anything reads it: not saved.
	advancedThisFrame bool

	// queued counts, over the pursuit's life, each time the budget left a
	// wanted solve for a later frame: a re-path or a first route Advance
	// could not serve, and a world chase start (Rechase on a hunter with no
	// chase) left without its route. Reported for the benchmark and the
	// tests; not in the provider, so not saved.
	queued int
}

// PursuitDials are the numbers M4.3a ships with. Every one is a [DIAL].
type PursuitDials struct {
	// RepathTiles is how far the quarry must move from where it stood at the
	// last solve before the route is recomputed. Too small and every jitter
	// costs a search; too large and the hunter walks confidently at where you
	// used to be.
	RepathTiles float64

	// ArriveWithin is how close counts as caught. A pursuer that arrives
	// simply stands next to its quarry: M4.3a has no combat, and pretending
	// otherwise would be the diorama this milestone exists to avoid.
	//
	// IT IS 1.5 BECAUSE A DIAGONAL NEIGHBOUR IS 1.414 AWAY. Until the
	// pursuer-adjacent burst the router walked a hunter onto the quarry's own
	// tile, so every arrival was at distance 0.000 and 1.0 was never tested.
	// Now that it stops BESIDE the quarry, an orthogonal stop is 1.000 and a
	// diagonal one is 1.414: at 1.0 the diagonal case would never report
	// arrived and would re-solve on the MinRepathMinutes cadence forever,
	// which is exactly the churn M4.3a's re-path clock was added to stop.
	//
	// It stays SEPARATE from Combat's AdjacentTiles (a Chebyshev 1, so the
	// eight neighbours) on purpose, and the M4.5 note's ask 6 says why: they
	// are two facts that happen to agree, and one overloaded dial would hide
	// the day they stop agreeing.
	ArriveWithin float64

	// MinRepathMinutes floors the re-solve rate in WORLD minutes, so a quarry
	// oscillating across the RepathTiles boundary cannot make a hunter solve
	// on every single tick.
	//
	// World minutes, not real ones, and the difference matters more than it
	// looks: the clock compresses (D7), so a quarter of a world minute can
	// pass in about three frames. The first value here was 0.25 and the
	// playtest measured 218 solves across 600 stepped frames because of it.
	MinRepathMinutes float64

	// ProgressTiles is how much ground a hunter must gain before an
	// unreachable route is worth re-solving. It is what keeps a pursuer from
	// stalling several tiles short of a quarry it cannot quite path to, and
	// what stops one that has genuinely run out of options from asking
	// forever.
	ProgressTiles float64

	// RechasesPerFrame caps the chases Rechase restarts on ANOTHER quarry
	// between two Advances (the R2 review B's B2): each costs an A* at once (mean
	// 1.5-3.0 ms on the village, worst 13-42 ms), and a long step -- a
	// sleep's or a labour's hour -- lets every Seek row look in ONE frame,
	// so a pack could retarget whole in it. Past the cap a chase keeps its
	// old quarry for now; the next frame's startChasesForTheAware asks again,
	// so the rest follow one a frame. A first chase is never capped. 1.
	RechasesPerFrame int

	// SolvesPerFrame is the per-frame A* budget (2 Oct 2026): how many
	// routes Pursuit solves between two Advances, shared by the re-paths
	// and first routes Advance serves, the world's chase starts and its
	// re-chases (Rechase). One solve on the village costs 1.5-2.6 ms on the
	// mean and up to 13-42 ms at worst (the R2 review), against a 2 ms
	// frame, so a burst -- a pack noticing him at once, the chases begun
	// together falling due together, a long step making every chase due --
	// must not land in one frame. It counts SOLVES, never wall time, so
	// the sim stays deterministic and a resume exact.
	//
	// Advance serves the wanted solves oldest first and, at a budget of 2
	// or more, keeps max(1, RechasesPerFrame) of it back (never all of it)
	// for the re-chases and starts that follow it in the frame
	// (Game.startChasesForTheAware runs after it), so neither side can
	// starve the other: a re-chase finds its unit every frame -- with the
	// re-chase cap switched off (RechasesPerFrame 0) too -- and Advance
	// serves at least one solve every frame. What the world does not use of
	// the kept-back share goes to the chases still owing a first route
	// (ServeQueued, after the world's starts). AT A BUDGET OF 1 THE ONE SLOT
	// IS SHARED, not split: Advance takes it whenever it wants a solve, and a
	// re-chase or a start gets it only on a frame Advance did not -- so at 1
	// a re-chase can wait as long as re-paths keep falling due (the second
	// review's dial corners). Past the budget a chase keeps the route it is
	// walking (or stands, if it has none yet) and is served on a later
	// frame. 0 switches the budget off (every solve at once, as before).
	// [DIAL] 2.
	SolvesPerFrame int
}

// DefaultPursuitDials are the signed §4 starting values.
func DefaultPursuitDials() PursuitDials {
	return PursuitDials{
		RepathTiles:      1.5,
		ArriveWithin:     1.5,
		MinRepathMinutes: 2.0,
		ProgressTiles:    0.5,
		RechasesPerFrame: 1,
		SolvesPerFrame:   2,
	}
}

// Router is the only thing Pursuit needs from the map: a route between two
// points in world tiles, and whether it actually reaches. d2world does not
// know what a MapEngine is, and this interface is why it does not have to.
type Router interface {
	Route(fromX, fromY, toX, toY float64) (waypoints [][2]float64, reachable bool)
}

// Quarry is a thing worth chasing. It only has to say where it is.
type Quarry interface {
	QuarryID() string
	QuarryAt() (x, y float64)
}

// Hunter is a thing that chases. It says where it is, whether it is still
// walking a route, and takes a new one.
type Hunter interface {
	HunterID() string
	HunterAt() (x, y float64)
	Following() bool
	Follow(waypoints [][2]float64)
}

// chase is one hunter's pursuit of one quarry.
type chase struct {
	hunter Hunter
	quarry Quarry

	// solvedAtX/Y is where the QUARRY stood when the route was last computed.
	// The re-path test is against this rather than against the hunter, because
	// what invalidates a route is the goal moving, not the walker.
	solvedAtX, solvedAtY float64

	// solvedDistance is how far the hunter was from the quarry at the last
	// solve. An unreachable route is only worth re-asking from meaningfully
	// closer ground, and this is what "closer" is measured against.
	solvedDistance float64

	sinceSolve float64 // world minutes
	reachable  bool
	solves     int
	arrived    bool

	// fromWatch marks a chase the world started or moved from a watch's
	// awareness (Rechase, Game.startChasesForTheAware), as against a script's
	// strigoi_pursue (Chase alone). GiveUpOnTheForgotten reads it
	// (integrate-1oct): R2's Seek moves a WATCH onto another quarry and the
	// chase follows only under the re-chase budget, so a world chase can stand
	// on the quarry its watch has just left -- and if the watch then forgets,
	// that chase must end too, or BUG-108 is back (him "chased" for good, and
	// the save refused COMBAT). IN THE WORLD FILE since version 5
	// (pursuit.chases[].from_watch, written only when true; the per-frame
	// budget's second review, A): until then a load read it false, and a
	// world chase saved while its re-chase was deferred, whose watch then
	// forgot, stood in the resumed game and was released in the game that
	// ran on -- the resume diverged. A script's chase is saved false and
	// read false, so it stands after a resume exactly as before it
	// (d2gamescreen TestAHuntedNightResumes holds one).
	fromWatch bool
}

// NewPursuit builds the system and registers it as a harness provider.
func NewPursuit(router Router, dials PursuitDials) *Pursuit {
	p := &Pursuit{
		dials:  dials,
		router: router,
		chases: make(map[string]*chase),
	}

	d2harness.Register(p)

	return p
}

// Close unregisters the provider.
func (p *Pursuit) Close() { d2harness.Unregister(p) }

// Chase starts or replaces a hunter's pursuit of a quarry, and solves the
// first route immediately so the hunter is moving before the next tick.
//
// One hunter chases one thing: starting a second chase replaces the first,
// because a creature running at two targets at once is a bug wearing a
// feature's clothes.
//
// A script's chase (Game.Pursue, strigoi_pursue) comes here and is OUTSIDE the
// solve budget: a script that starts a chase reads its route at once. (Its
// solve is counted in solvedThisFrame, but in the game the harness's verbs run
// before the frame's Advance, which zeroes the count -- so in the game it never
// shrinks the world's share; the second review's C.) The world's chases come
// through Rechase.
func (p *Pursuit) Chase(hunter Hunter, quarry Quarry) {
	if hunter == nil || quarry == nil {
		return
	}

	if old, ok := p.chases[hunter.HunterID()]; ok && old.quarry != nil && old.quarry.QuarryID() != quarry.QuarryID() {
		p.rechasedThisFrame++
		p.rechases++
	}

	c := &chase{hunter: hunter, quarry: quarry}
	p.chases[hunter.HunterID()] = c
	p.solve(c)
}

// Rechase is Chase under the frame's re-chase budget (RechasesPerFrame; the
// R2 review B's B2): the game's way to move a chase onto the quarry Seek moved
// its watch to (Game.startChasesForTheAware). A chase on another quarry is
// restarted only while the budget since the last Advance lasts; past it the
// chase keeps its old quarry and Rechase reports false -- the next frame asks
// again. Anything else (no chase yet, or one on this quarry) is Chase. A
// script's strigoi_pursue is Chase, never capped.
//
// The per-frame solve budget (SolvesPerFrame, 2 Oct 2026) holds it too. A
// re-chase past the budget is deferred exactly as past the cap: the chase
// keeps its old quarry and its route, Rechase reports false, and the next
// frame asks again (Advance keeps max(1, RechasesPerFrame) of each frame's
// budget back for it at a budget of 2 or more, so it is never starved). A START past the budget -- a hunter with no
// chase yet -- is begun without its route: the chase is live (ChasersOf names
// it, so the combat status reads him chased from the frame he is noticed) but
// owes its first solve, and the hunter stands until Advance serves it, first
// of everything it serves (see Advance). Such a chase is the one with no solve
// yet (chase.solves 0), which the world file already carries, so the queue is
// derived from the saved chases and adds nothing to the file.
func (p *Pursuit) Rechase(hunter Hunter, quarry Quarry) bool {
	if hunter == nil || quarry == nil {
		return false
	}

	old, has := p.chases[hunter.HunterID()]

	if has && old.quarry != nil && old.quarry.QuarryID() != quarry.QuarryID() &&
		p.dials.RechasesPerFrame > 0 && p.rechasedThisFrame >= p.dials.RechasesPerFrame {
		p.rechasesDeferred++

		return false
	}

	if !p.budgetLeft() {
		if has {
			p.rechasesDeferred++

			return false
		}

		// A start: live now, its route owed.
		p.chases[hunter.HunterID()] = &chase{hunter: hunter, quarry: quarry, fromWatch: true}
		p.queued++

		return true
	}

	p.Chase(hunter, quarry)
	p.chases[hunter.HunterID()].fromWatch = true

	return true
}

// budgetLeft is whether the frame's solve budget has a solve left.
func (p *Pursuit) budgetLeft() bool {
	return p.dials.SolvesPerFrame <= 0 || p.solvedThisFrame < p.dials.SolvesPerFrame
}

// advanceBudget is how many solves Advance may make itself: the frame's
// budget less what it keeps back for the re-chases and starts after it --
// max(1, RechasesPerFrame), so switching the re-chase cap off does not switch
// the reserve off (the second review's B) -- and never less than one, so the
// queue always drains. At a budget of 1 the one slot is shared (nothing is
// kept back; see SolvesPerFrame). 0 is no limit.
func (p *Pursuit) advanceBudget() int {
	b := p.dials.SolvesPerFrame
	if b <= 0 {
		return 0
	}

	keep := p.dials.RechasesPerFrame
	if keep < 1 {
		keep = 1
	}

	if keep > b-1 {
		keep = b - 1
	}

	return b - keep
}

// ServeQueued spends what is left of the frame's budget on the chases that
// still owe their first route, oldest first (as Advance orders them), and
// returns how many it served. The game calls it once a frame, after the
// world's starts and re-chases (Game.startChasesForTheAware): the share
// Advance kept back for them is theirs first, and what they did not use is no
// longer wasted on a frame with a pack still standing (the second review's C:
// a pack of 8 is routed in 4 frames, not 7). Re-paths are Advance's alone.
// Nothing is served in a frame whose world stood still (a held fight's), as
// Advance serves nothing then.
func (p *Pursuit) ServeQueued() int {
	if !p.advancedThisFrame {
		return 0
	}

	var owed []*chase

	for _, id := range p.hunterIDs() {
		if c := p.chases[id]; c.solves == 0 {
			owed = append(owed, c)
		}
	}

	sort.SliceStable(owed, func(i, j int) bool { return owed[i].sinceSolve > owed[j].sinceSolve })

	n := 0

	for _, c := range owed {
		if !p.budgetLeft() {
			break
		}

		p.solve(c)
		n++
	}

	return n
}

// Queued is how many chases owe their first route now (started past the
// budget, not yet served). Derived from the chases: a chase with no solve.
func (p *Pursuit) Queued() int {
	n := 0

	for _, c := range p.chases {
		if c.solves == 0 {
			n++
		}
	}

	return n
}

// Release ends a chase. A provider that reports a collection needs a verb
// that can put something in it AND one that can take it back out -- the rule
// the M4.1 reopening produced, applied here from the start rather than after
// someone notices the list can only grow.
func (p *Pursuit) Release(hunterID string) bool {
	if _, ok := p.chases[hunterID]; !ok {
		return false
	}

	delete(p.chases, hunterID)

	return true
}

// Chasing reports whether a hunter already has a chase running. The caller
// that turns awareness into pursuit asks this every tick, and asking is
// cheaper than restarting a chase that is already honest -- Chase() replaces,
// which would reset the re-path clock on every frame.
func (p *Pursuit) Chasing(hunterID string) bool {
	_, ok := p.chases[hunterID]

	return ok
}

// ChasingWhom is the quarry a hunter's chase is on, and whether it has one
// (the raid's R2). The caller that turns awareness into pursuit asks it every
// tick: a watch Seek moved onto a nearer villager must move the chase too, or
// the wolf would keep walking at the man it no longer wants.
func (p *Pursuit) ChasingWhom(hunterID string) (string, bool) {
	c, ok := p.chases[hunterID]
	if !ok || c.quarry == nil {
		return "", ok
	}

	return c.quarry.QuarryID(), true
}

// Count is how many chases are live.
func (p *Pursuit) Count() int { return len(p.chases) }

// ChasersOf is every hunter chasing quarryID, in the stable order the rest of
// the system uses (sorted ids), or nil. The game screen's combat status asks
// it every frame whether anything is chasing HIM (Josh, 30 Sep 2026: "Fight,
// action, or chased"): a chase lives here from Chase to Release, whatever its
// route does -- a re-path replaces the route, never the chase -- so the answer
// does not flicker as a hunter re-paths.
func (p *Pursuit) ChasersOf(quarryID string) []string {
	var out []string

	for _, id := range p.hunterIDs() {
		if p.chases[id].quarry.QuarryID() == quarryID {
			out = append(out, id)
		}
	}

	return out
}

// GiveUpOnTheForgotten releases every chase whose hunter has forgotten its
// quarry, and returns the hunters released, in the stable order (BUG-108,
// fixed 1 Oct 2026 on the coordinator's default, option (a); Josh can
// overturn). A chase was started from a watch's awareness
// (Game.startChasesForTheAware) and nothing ended it when the awareness
// lapsed: a dog that lost him kept re-routing to where he was until it died
// or dawn came -- and since the combat status a hostile chasing him is combat,
// so he could not save. Now the chase ends when its hunter's watch of the
// same quarry is no longer noticed: MemoryMinutes [DIAL] of the notice model
// (2.0 world minutes shipped) unseen. The number is the notice's own on
// purpose: MemoryMinutes is signed as "how long a watcher keeps coming after
// losing sight", and the watch's minutes unseen stop counting once it
// forgets, so a longer give-up would need a second clock in the world file
// for no designed difference. A chase with no watch behind it (the harness's
// strigoi_pursue), or a script's chase of another quarry than its watch's, is
// left alone. A hunter that sees its quarry again notices it again, and the
// next frame's startChasesForTheAware chases again.
//
// With the raid's R2 (integrate-1oct): a WORLD chase (fromWatch: started or
// moved by Rechase) ends when its hunter's watch forgets, whatever quarry the
// watch is on now. Seek can move the watch from him to a villager while the
// re-chase budget (RechasesPerFrame) keeps the chase on him for a frame or
// more; a watch that then forgets the villager is behind no chase of anyone,
// and the chase of him it left must end with it. A world chase whose watch is
// still aware of another quarry stands: startChasesForTheAware moves it.
func (p *Pursuit) GiveUpOnTheForgotten(n *Notice) []string {
	if n == nil {
		return nil
	}

	var out []string

	for _, id := range p.hunterIDs() {
		w, ok := n.watches[id]
		if !ok || w.noticed || w.target == nil {
			continue
		}

		if c := p.chases[id]; !c.fromWatch && w.target.QuarryID() != c.quarry.QuarryID() {
			continue
		}

		delete(p.chases, id)

		out = append(out, id)
	}

	return out
}

// Solves is how many routes have been computed since construction.
func (p *Pursuit) Solves() int { return p.solves }

// Advance steps every live chase by the world minutes that just passed.
//
// It first finds every chase that wants a solve -- one that owes its first
// route (begun past the budget: Rechase), and the re-paths M4.3a's rules
// call for -- and then serves them under the frame's solve budget
// (SolvesPerFrame less the RechasesPerFrame it keeps back), in a stable
// order: first routes before re-paths (a hunter with no route stands; one
// with a stale route is still walking), then the longest since its last solve
// (sinceSolve: a chase passed over keeps counting while a served one starts
// again from zero, so the oldest is served first and every wait is bounded),
// then the hunter id. A chase passed over keeps the route it is walking and
// is asked about again next frame from its own state, which the world file
// carries: the queue is derived, never saved.
//
// Oldest first, NOT his own chasers first: Pursuit names no player, and a
// priority class would let one side's re-paths starve the other's; with the
// budget's numbers a chaser waits a few frames at most (the burst benchmark),
// which is not visible on a walk.
//
// With no budget (SolvesPerFrame 0) every wanted solve is served, which is
// exactly the old rule: the router is a pure function of two points on a
// fixed map, so solving after the scan rather than during it changes no route.
func (p *Pursuit) Advance(worldMinutes float64) {
	p.rechasedThisFrame = 0
	p.solvedThisFrame = 0
	p.advancedThisFrame = worldMinutes > 0

	if worldMinutes <= 0 || len(p.chases) == 0 {
		return
	}

	var want []*chase

	// Iterate in a fixed order. Ranging a map is randomised in Go, and these
	// solves feed entity positions, which are inside the state digest -- the
	// same reason the A* itself never ranges a map.
	for _, id := range p.hunterIDs() {
		c := p.chases[id]
		c.sinceSolve += worldMinutes

		hx, hy := c.hunter.HunterAt()
		qx, qy := c.quarry.QuarryAt()

		c.arrived = distance(hx, hy, qx, qy) <= p.dials.ArriveWithin

		// Owed its first route (begun past the budget): served whatever
		// else is true, even beside its quarry -- the hunter is still walking
		// whatever it walked before the chase began.
		if c.solves == 0 {
			want = append(want, c)

			continue
		}

		if c.arrived {
			// Caught up. Stand there; M4.5 decides what happens next.
			continue
		}

		if c.sinceSolve < p.dials.MinRepathMinutes {
			continue
		}

		if distance(qx, qy, c.solvedAtX, c.solvedAtY) >= p.dials.RepathTiles {
			want = append(want, c)

			continue
		}

		// The quarry has not moved far enough to invalidate the route, so the
		// only reason left to solve is that the hunter has run out of one.
		if c.hunter.Following() {
			continue
		}

		// It has. Whether asking again is worth anything depends on what the
		// last answer was.
		//
		// If the route reached, the hunter is simply due a fresh one. If it
		// did NOT reach, the same question from the same place gets the same
		// answer, and asking it every tick is how a hunter that cannot reach
		// its quarry burns a search for the rest of the night -- the playtest
		// measured 218 solves across 600 stepped frames doing exactly that.
		//
		// But refusing outright is wrong too: a partial route still carries
		// the hunter closer, and from somewhere closer the question is a
		// genuinely new one. So the condition is PROGRESS. Each re-solve has
		// to be paid for by ground actually gained, which both lets a hunter
		// close in and bounds the loop, because a hunter that has stopped
		// making progress stops asking.
		if c.reachable || distance(hx, hy, qx, qy) < c.solvedDistance-p.dials.ProgressTiles {
			want = append(want, c)
		}
	}

	// want is in hunter-id order; a stable sort keeps it as the last key.
	sort.SliceStable(want, func(i, j int) bool {
		a, b := want[i], want[j]
		if (a.solves == 0) != (b.solves == 0) {
			return a.solves == 0
		}

		return a.sinceSolve > b.sinceSolve
	})

	limit := p.advanceBudget()

	for i, c := range want {
		if limit > 0 && i >= limit {
			p.queued += len(want) - i

			break
		}

		p.solve(c)
	}
}

// solve computes one route and hands it to the hunter.
func (p *Pursuit) solve(c *chase) {
	hx, hy := c.hunter.HunterAt()
	qx, qy := c.quarry.QuarryAt()

	waypoints, reachable := p.router.Route(hx, hy, qx, qy)

	c.solvedAtX, c.solvedAtY = qx, qy
	c.solvedDistance = distance(hx, hy, qx, qy)
	c.sinceSolve = 0
	c.reachable = reachable
	c.solves++
	p.solves++
	p.solvedThisFrame++

	// A partial route is still worth walking: the bounded search returns the
	// best approach it managed, which is how "cannot reach you" stays "gets as
	// close as it can" rather than "stands still".
	c.hunter.Follow(waypoints)
}

// hunterIDs returns the live hunter ids in a stable order.
func (p *Pursuit) hunterIDs() []string {
	ids := make([]string, 0, len(p.chases))
	for id := range p.chases {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}

func distance(ax, ay, bx, by float64) float64 {
	dx, dy := ax-bx, ay-by

	return math.Sqrt(dx*dx + dy*dy)
}

// ------------------------------------------------------------ harness ------

// HarnessName identifies the provider (P3 §3.5).
func (p *Pursuit) HarnessName() string { return "pursuit" }

// HarnessState reports the chases and the dials behind them. chase_list is
// ordered, because an assertion that reads element 0 must read the same one
// on every run.
func (p *Pursuit) HarnessState() map[string]interface{} {
	list := make([]map[string]interface{}, 0, len(p.chases))

	for _, id := range p.hunterIDs() {
		c := p.chases[id]
		hx, hy := c.hunter.HunterAt()
		qx, qy := c.quarry.QuarryAt()

		list = append(list, map[string]interface{}{
			"hunter":            id,
			"quarry":            c.quarry.QuarryID(),
			"hunter_x":          hx,
			"hunter_y":          hy,
			"quarry_x":          qx,
			"quarry_y":          qy,
			"distance":          distance(hx, hy, qx, qy),
			"quarry_moved":      distance(qx, qy, c.solvedAtX, c.solvedAtY),
			"minutes_since":     c.sinceSolve,
			"reachable":         c.reachable,
			"solves":            c.solves,
			"arrived":           c.arrived,
			"following":         c.hunter.Following(),
			"repath_tiles_dial": p.dials.RepathTiles,

			// M4.6 B1: what the re-path tests read -- where the quarry
			// stood and how far the hunter was at the last solve.
			// quarry_moved above is derived from the first; these are the
			// state itself, which is what a save carries.
			"solved_at_x":     c.solvedAtX,
			"solved_at_y":     c.solvedAtY,
			"solved_distance": c.solvedDistance,

			// Version 5 (pursuit-budget): a WORLD chase, which
			// GiveUpOnTheForgotten ends when its watch forgets. Saved and
			// restored, so reported (integrate-3: the B6 review's rule,
			// BUG-117 -- a field the file restores and no provider reports
			// is one a resume can lose unseen).
			"from_watch": c.fromWatch,
		})
	}

	return map[string]interface{}{
		"chases":             len(p.chases),
		"solves":             p.solves,
		"rechase_solves":     p.rechases,
		"chase_list":         list,
		"repath_tiles":       p.dials.RepathTiles,
		"arrive_within":      p.dials.ArriveWithin,
		"min_repath_minutes": p.dials.MinRepathMinutes,
		"rechases_per_frame": p.dials.RechasesPerFrame,

		// The per-frame solve budget (2 Oct 2026): the dial, and how many
		// chases owe their first route now (derived from the chases, so a
		// resume reports the same).
		"solves_per_frame": p.dials.SolvesPerFrame,
		"queued":           p.Queued(),
	}
}

// HarnessSettableFields lists the writes the system allows.
func (p *Pursuit) HarnessSettableFields() []string {
	return []string{"arrive_within", "release", "repath_tiles"}
}

// HarnessSet writes one allow-listed field.
//
// There is no "chase" field here, and the omission is deliberate rather than
// an oversight: starting a chase needs two live entities, which d2world cannot
// look up by handle -- it does not know what an entity is. The harness tool
// that owns entity handles starts chases; this side can only stop them and
// move the dials. Saying so here is cheaper than a future session rediscovering
// it.
func (p *Pursuit) HarnessSet(field string, value interface{}) error {
	switch field {
	case "repath_tiles", "arrive_within":
		f, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("%s wants a number in world tiles, got %T", field, value)
		}

		if f <= 0 {
			return fmt.Errorf("%s wants a positive number of world tiles, got %v", field, f)
		}

		if field == "repath_tiles" {
			p.dials.RepathTiles = f
		} else {
			p.dials.ArriveWithin = f
		}

		return nil

	case "release":
		id, ok := value.(string)
		if !ok {
			return fmt.Errorf(`release wants a hunter id like "n:12", got %T`, value)
		}

		if !p.Release(id) {
			return fmt.Errorf("no chase is running for hunter %q", id)
		}

		return nil
	}

	return fmt.Errorf("pursuit has no settable field %q", field)
}
