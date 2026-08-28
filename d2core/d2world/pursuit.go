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
}

// DefaultPursuitDials are the signed §4 starting values.
func DefaultPursuitDials() PursuitDials {
	return PursuitDials{
		RepathTiles:      1.5,
		ArriveWithin:     1.0,
		MinRepathMinutes: 2.0,
		ProgressTiles:    0.5,
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
func (p *Pursuit) Chase(hunter Hunter, quarry Quarry) {
	if hunter == nil || quarry == nil {
		return
	}

	c := &chase{hunter: hunter, quarry: quarry}
	p.chases[hunter.HunterID()] = c
	p.solve(c)
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

// Count is how many chases are live.
func (p *Pursuit) Count() int { return len(p.chases) }

// Solves is how many routes have been computed since construction.
func (p *Pursuit) Solves() int { return p.solves }

// Advance steps every live chase by the world minutes that just passed.
func (p *Pursuit) Advance(worldMinutes float64) {
	if worldMinutes <= 0 || len(p.chases) == 0 {
		return
	}

	// Iterate in a fixed order. Ranging a map is randomised in Go, and these
	// solves feed entity positions, which are inside the state digest -- the
	// same reason the A* itself never ranges a map.
	for _, id := range p.hunterIDs() {
		c := p.chases[id]
		c.sinceSolve += worldMinutes

		hx, hy := c.hunter.HunterAt()
		qx, qy := c.quarry.QuarryAt()

		if distance(hx, hy, qx, qy) <= p.dials.ArriveWithin {
			// Caught up. Stand there; M4.5 decides what happens next.
			c.arrived = true

			continue
		}

		c.arrived = false

		if c.sinceSolve < p.dials.MinRepathMinutes {
			continue
		}

		if distance(qx, qy, c.solvedAtX, c.solvedAtY) >= p.dials.RepathTiles {
			p.solve(c)

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
			p.solve(c)
		}
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
		})
	}

	return map[string]interface{}{
		"chases":             len(p.chases),
		"solves":             p.solves,
		"chase_list":         list,
		"repath_tiles":       p.dials.RepathTiles,
		"arrive_within":      p.dials.ArriveWithin,
		"min_repath_minutes": p.dials.MinRepathMinutes,
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
