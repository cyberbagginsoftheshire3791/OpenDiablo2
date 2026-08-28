package d2world

import (
	"math"
	"sort"
)

// Notice is M4.3b's awareness model: the spawn tables decide WHAT arrives and
// WHEN, and this decides whether it has seen you.
//
// It exists as its own type rather than as a lump inside the spawn tables
// because the thing that made a pack is not the only thing that will ever want
// to ask whether something can see the player -- M4.5 can ask the same
// question about a creature no table spawned. M4.3a's Pursuit says the other
// half of this in its own doc comment: "It is deliberately NOT awareness.
// Nothing here decides WHEN a hunter should start chasing -- that is M4.3b's
// notice model." This is that model, and the three systems compose in one
// line: the tables make it, Notice decides it has seen you, Pursuit keeps the
// chase honest afterwards.
//
// WHAT IT IS ALLOWED TO KNOW is a signed fence, not an implementation detail
// (M4.3b ask 5, 28 Aug): LINE OF SIGHT, DISTANCE, and THE LIGHT LEVEL AT THE
// TARGET. Nothing else -- not the meters, not the inventory, not reputation.
// Adding the NOISE value was considered and refused at signature even though
// S1 §4 signs noise as something beasts and humans react to; that is the
// recorded case against the decision, and it is an additive change to the
// two interfaces below if the playtest disagrees. DO NOT WIDEN THIS QUIETLY:
// the fence is what makes a torch a trade -- light against being seen --
// rather than a free upgrade.
//
// Like every other system in this package it steps on the world clock, never
// the wall clock, and it talks to the map and the light model through
// interfaces so d2world keeps importing no map engine and no ebiten. That is
// also what makes it testable: the tests run on fakes with no MPQs.
type Notice struct {
	dials NoticeDials
	sight Sight
	illum Illumination

	watches map[string]*watch

	// checks counts sight evaluations since construction and notices counts
	// transitions from unaware to aware. Both are reported, for the reason
	// Pursuit.solves is: M4.3a's re-path bug was invisible to every unit test
	// and obvious the moment a counter was put in front of the running game.
	checks  int
	notices int
}

// Sight is the only thing Notice needs from the map: whether the straight line
// between two points is unobstructed. MapEngine.LineOfSight answers exactly
// this -- it is the raycast PathFind used to be before M4.3a replaced it with
// a real search, kept because whether a thing can SEE you is not whether it
// can WALK to you. This interface is why d2world does not have to know that.
type Sight interface {
	Clear(fromX, fromY, toX, toY float64) bool
}

// Illumination is the only thing Notice needs from the light model: how lit a
// tile is. *Light satisfies it as it stands, with no adapter, the same trick
// d2maprenderer.LightSampler uses in the other direction.
type Illumination interface {
	Level(tileX, tileY int) float64
}

// Watcher is a thing that might notice something. It only has to say where it
// is; everything else it is allowed to know comes from the two interfaces
// above, which is the fence.
type Watcher interface {
	WatcherID() string
	WatcherAt() (x, y float64)
}

// The thing being watched is a Quarry -- the same interface Pursuit chases,
// deliberately, because the thing worth noticing and the thing worth chasing
// are the same thing, and one adapter in the game screen should satisfy both.

// NoticeDials are the numbers M4.3b ships with. Every one is a [DIAL].
//
// EVERY RATE HERE IS IN WORLD MINUTES AND CARRIES ITS FRAME COST IN THE
// COMMENT, because the frame count is the number that actually gets paid and
// the conversion is counter-intuitive: the clock COMPRESSES, and DAY is the
// tight case (ClockDials.DayRate 4.0 against NightRate 2.5), so a
// world-minute budget buys FEWER frames in daylight than at night. At the
// harness's default tick of 1/60 s, one world minute is ~15 stepped frames by
// day and ~24 at night. M4.3a shipped a 0.25-world-minute floor reasoning it
// was "plenty of headroom" and the playtest measured 218 route solves across
// 600 stepped frames.
type NoticeDials struct {
	// Radius is how far a watcher can notice an unlit target, in world tiles.
	//
	// 12 IS DELIBERATELY LARGER THAN THE VIEWPORT. Only about five tiles in
	// any direction are on screen (screen offset for a tile offset (dx,dy) is
	// (80(dx-dy), 40(dx+dy)), so five tiles already spans 800x600), which
	// means a watcher can decide to come for the player while off-camera.
	// That was flagged twice as the weakest-sourced number in the build note
	// -- N1 gives pack sizes and stages but says nothing about detection --
	// and Josh ruled on it on 28 Aug: "12 tile aggro will keep it scary."
	// DO NOT TUNE IT DOWN AS AN OVERSIGHT. If a playtest makes it look
	// unfair, report the measurement and let him re-rule.
	Radius float64

	// LitMultiplier scales Radius when the target's light level is at or above
	// LitLevel: carrying a light multiplies the distance at which you are
	// seen. R2 §3 signs the dark-into-light advantage for combat; this is its
	// detection half, and it is the whole reason a torch is a trade.
	//
	// Expressed as a multiplier rather than a second radius so that tuning
	// Radius keeps the relationship intact.
	LitMultiplier float64

	// LitLevel is the quantised light level at which a target counts as lit.
	// Light.Level is quantised into 16 bands, so this lands on a band edge
	// rather than between two.
	LitLevel float64

	// ReEvaluateMinutes floors how often one watcher re-runs the sight test.
	// ~15 stepped frames by day, ~24 at night. Deliberately the tightest rate
	// in this milestone: awareness that lags is worse than awareness that
	// costs, and a sight test is one raycast rather than a whole search.
	ReEvaluateMinutes float64

	// MemoryMinutes is how long a watcher keeps coming after losing sight.
	// ~30 stepped frames by day, ~48 at night. Without it, breaking line of
	// sight cancels awareness on the very next tick and cover stops being a
	// tactic and becomes a switch.
	MemoryMinutes float64
}

// DefaultNoticeDials returns the signed §5 starting values.
func DefaultNoticeDials() NoticeDials {
	return NoticeDials{
		Radius:            12,
		LitMultiplier:     2,
		LitLevel:          0.30,
		ReEvaluateMinutes: 1.0,
		MemoryMinutes:     2.0,
	}
}

// watch is one watcher's awareness of one target.
type watch struct {
	watcher Watcher
	target  Quarry

	// noticed is the answer the rest of the game acts on. sees is the raw
	// sight test at the last evaluation, and the two differ on purpose for
	// MemoryMinutes after a watcher loses sight of something it had found.
	noticed bool
	sees    bool

	distance      float64
	lightAtTarget float64
	reach         float64 // the effective radius at the last evaluation

	sinceCheck float64 // world minutes since the last sight test
	sinceSeen  float64 // world minutes since the target was last actually seen

	checks  int
	notices int
}

// NewNotice builds the model. A nil sight or a nil illumination is allowed and
// means NOTHING EVER NOTICES ANYTHING -- the safe direction for a wiring bug,
// because the alternative default (everything notices through walls) would
// read as a deliberate design choice in play while this reads as a failure the
// playtest catches immediately. Wired() reports which it is.
func NewNotice(sight Sight, illum Illumination, dials NoticeDials) *Notice {
	return &Notice{
		dials:   dials,
		sight:   sight,
		illum:   illum,
		watches: make(map[string]*watch),
	}
}

// Wired reports whether the model has both of the things it is allowed to
// know. It is reported rather than merely asserted at construction so a
// script can tell "nothing noticed you" from "nothing could have".
func (n *Notice) Wired() bool { return n.sight != nil && n.illum != nil }

// Watch starts (or replaces) a watcher's awareness of a target and evaluates
// it once immediately, so a caller never has to step the world to find out
// whether something can already see the player.
//
// One watcher watches one thing. A creature aware of two targets at once is a
// question about target selection, which is M4.5's, not this milestone's.
func (n *Notice) Watch(watcher Watcher, target Quarry) {
	if watcher == nil || target == nil {
		return
	}

	w := &watch{watcher: watcher, target: target}
	n.watches[watcher.WatcherID()] = w
	n.evaluate(w)
}

// Unwatch drops a watcher. A provider that reports a collection needs a verb
// that can put something in it AND one that can take it back out -- the first
// provider rule, applied here from the start.
func (n *Notice) Unwatch(watcherID string) bool {
	if _, ok := n.watches[watcherID]; !ok {
		return false
	}

	delete(n.watches, watcherID)

	return true
}

// Count is how many watchers are live.
func (n *Notice) Count() int { return len(n.watches) }

// Checks is how many sight evaluations have run since construction.
func (n *Notice) Checks() int { return n.checks }

// Notices is how many times a watcher has gone from unaware to aware.
func (n *Notice) Notices() int { return n.notices }

// Noticed reports whether one watcher is currently aware of its target, and
// whether that watcher is being watched at all.
func (n *Notice) Noticed(watcherID string) (noticed, watching bool) {
	w, ok := n.watches[watcherID]
	if !ok {
		return false, false
	}

	return w.noticed, true
}

// Aware returns the ids of every watcher currently aware of its target, in a
// stable order. This is what the spawn tables read to decide that a group has
// found the player, and what M4.5 will read to decide a fight can start.
func (n *Notice) Aware() []string {
	out := make([]string, 0, len(n.watches))

	for _, id := range n.watcherIDs() {
		if n.watches[id].noticed {
			out = append(out, id)
		}
	}

	return out
}

// Advance steps every watcher by the world minutes that just passed.
//
// The split here is deliberate: the cheap bookkeeping (memory decay) runs
// every tick so MemoryMinutes means what it says, while the expensive part
// (the sight test) is floored by ReEvaluateMinutes. Rate-limiting the
// bookkeeping too would make the memory window a function of the check
// cadence, which is exactly the kind of unit confusion the 218-solve bug was.
func (n *Notice) Advance(worldMinutes float64) {
	if worldMinutes <= 0 || len(n.watches) == 0 {
		return
	}

	// Fixed iteration order. Ranging a map is randomised in Go, and what this
	// decides feeds entity movement, which is inside the state digest -- the
	// same reason the A* never ranges a map and Pursuit sorts its hunters.
	for _, id := range n.watcherIDs() {
		w := n.watches[id]
		w.sinceCheck += worldMinutes

		if w.noticed && !w.sees {
			w.sinceSeen += worldMinutes

			if w.sinceSeen >= n.dials.MemoryMinutes {
				w.noticed = false
			}
		}

		if w.sinceCheck < n.dials.ReEvaluateMinutes {
			continue
		}

		n.evaluate(w)
	}
}

// evaluate runs one sight test and updates one watch.
func (n *Notice) evaluate(w *watch) {
	w.sinceCheck = 0
	w.checks++
	n.checks++

	wx, wy := w.watcher.WatcherAt()
	tx, ty := w.target.QuarryAt()

	w.distance = distance(wx, wy, tx, ty)

	if !n.Wired() {
		w.sees = false
		w.lightAtTarget = 0
		w.reach = 0

		return
	}

	w.lightAtTarget = n.illum.Level(int(math.Floor(tx)), int(math.Floor(ty)))

	w.reach = n.dials.Radius
	if w.lightAtTarget >= n.dials.LitLevel {
		w.reach *= n.dials.LitMultiplier
	}

	// Distance is checked before the raycast on purpose: it is arithmetic and
	// the raycast walks the grid, so the cheap test gates the dear one.
	w.sees = w.distance <= w.reach && n.sight.Clear(wx, wy, tx, ty)

	if !w.sees {
		return
	}

	w.sinceSeen = 0

	if !w.noticed {
		w.noticed = true
		w.notices++
		n.notices++
	}
}

// watcherIDs returns the live watcher ids in a stable order.
func (n *Notice) watcherIDs() []string {
	ids := make([]string, 0, len(n.watches))
	for id := range n.watches {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}

// Report renders the per-watcher notice blocks the spawns provider embeds.
//
// It lives here rather than in the provider because these are Notice's facts,
// and it returns the four fields M4.3b ask 6 names -- sees, distance,
// light_at_quarry, noticed -- plus the reach that produced them, because an
// assertion that a lit target was seen from further away is unreadable
// without it. The list is ordered: an assertion that reads element 0 must
// read the same one on every run.
//
// ASK 6 EXISTS BECAUSE M4.3a's §3.2 DID NOT. A chase that starts is not
// evidence that noticing works, because a chase can start for the wrong
// reason and look identical from outside. Reporting the inputs and the
// verdict is what lets a script show a watcher that CAN see and does notice
// beside one that CANNOT and does not.
func (n *Notice) Report() []map[string]interface{} {
	list := make([]map[string]interface{}, 0, len(n.watches))

	for _, id := range n.watcherIDs() {
		w := n.watches[id]

		list = append(list, map[string]interface{}{
			"watcher":         id,
			"quarry":          w.target.QuarryID(),
			"sees":            w.sees,
			"noticed":         w.noticed,
			"distance":        w.distance,
			"light_at_quarry": w.lightAtTarget,
			"reach":           w.reach,
			"minutes_unseen":  w.sinceSeen,
			"checks":          w.checks,
			"notices":         w.notices,
		})
	}

	return list
}

// Dials exposes the current dials so the spawns provider can report them.
func (n *Notice) Dials() NoticeDials { return n.dials }

// SetRadius moves the base notice radius. Returns false for a value that is
// not a positive number of world tiles.
func (n *Notice) SetRadius(tiles float64) bool {
	if tiles <= 0 {
		return false
	}

	n.dials.Radius = tiles

	return true
}

// SetLitLevel moves the light level at which a target counts as lit.
func (n *Notice) SetLitLevel(level float64) bool {
	if level < 0 || level > 1 {
		return false
	}

	n.dials.LitLevel = level

	return true
}
