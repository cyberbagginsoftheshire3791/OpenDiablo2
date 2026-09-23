package d2mapengine

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2dt1"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// PathFind finds a walkable route between the given start and dest positions
// and returns its waypoints, in travel order. Positions are in SUBTILE space:
// d2vector.Position stores subtiles, and NewPositionTile multiplies by five.
//
// Until M4.3a this was not a pathfinder. It was one call to checkLos, and it
// returned a ONE-ELEMENT slice: the destination when nothing blocked the
// straight line, and otherwise the last walkable point before the first
// blocker. Nothing routed around anything, which is why clicking across town
// stopped at the first fence and why nothing in the world could approach
// anything else. The repo had filed that as normal -- "'stuck' is a normal
// outcome, not an error".
//
// It now runs a bounded, deterministic A* (astar.go) and returns the corners
// of the route it finds. When the goal cannot be reached, within the map or
// within the expansion budget, it returns the best partial route toward it
// rather than nothing -- the old walk-as-far-as-you-can behaviour, kept
// deliberately as the failure mode.
func (m *MapEngine) PathFind(start, dest d2vector.Position) []d2vector.Position {
	from := subTile{int(math.Floor(start.X())), int(math.Floor(start.Y()))}
	goal := subTile{int(math.Floor(dest.X())), int(math.Floor(dest.Y()))}

	if from == goal {
		return []d2vector.Position{dest}
	}

	result := m.search(from, goal)

	steps := result.route(from)
	exact := result.exact

	// The long way round (corridor.go): tried only when the ordinary search
	// ran out of BUDGET -- not when it proved the goal walled off -- and used
	// only if the whole corridor arrives. Every route the search above finds
	// is returned exactly as before.
	if !exact && result.exhausted {
		if long, ok := m.corridorRoute(from, goal); ok {
			steps, exact = long, true
		}
	}

	if len(steps) == 0 {
		return []d2vector.Position{}
	}

	return waypoints(from, steps, dest, exact)
}

// waypoints turns a subtile-by-subtile route into the corners of that route.
//
// The search works one subtile at a time, so a walk across town is hundreds of
// steps in a straight line. Keeping only the points where the direction
// changes gives the mover the same path with a fraction of the waypoints, and
// collapsing on an exact direction match keeps it deterministic. When the goal
// was actually reached the last waypoint is the caller's own dest, so the
// mover still lands exactly where it was asked to rather than on the centre of
// the nearest subtile.
func waypoints(from subTile, steps []subTile, dest d2vector.Position, exact bool) []d2vector.Position {
	points := make([]d2vector.Position, 0, len(steps))
	last := len(steps) - 1

	for i := range steps {
		if i < last {
			previous := from
			if i > 0 {
				previous = steps[i-1]
			}

			inX, inY := steps[i].x-previous.x, steps[i].y-previous.y
			outX, outY := steps[i+1].x-steps[i].x, steps[i+1].y-steps[i].y

			if inX == outX && inY == outY {
				continue // still travelling the same direction
			}
		}

		if i == last && exact {
			points = append(points, dest)

			continue
		}

		points = append(points, d2vector.NewPosition(
			float64(steps[i].x)+subTileCentre,
			float64(steps[i].y)+subTileCentre,
		))
	}

	return points
}

// LineOfSight reports whether the straight line between two positions is
// unobstructed. It is precisely what PathFind used to answer before M4.3a
// replaced it with a real search, and it is still a real question -- just a
// different one: whether a thing can SEE you is not whether it can WALK to
// you, and M4.3b's awareness model needs the first.
//
// It also has an immediate use as a negative control. A path assertion that
// only shows the mover arriving cannot tell a genuine detour from a walk that
// never needed one; asking whether the straight line was clear is what makes
// "it went around something" evidence rather than a hope. strigoi_find_path
// reports both for exactly that reason.
func (m *MapEngine) LineOfSight(start, end d2vector.Position) bool {
	clear, _ := m.checkLos(start, end)

	return clear
}

// checkLos finds out if there is a clear line of sight between two points.
func (m *MapEngine) checkLos(start, end d2vector.Position) (bool, d2vector.Position) {
	dv := d2vector.Position{Vector: *end.Clone()}
	dv.Subtract(&start.Vector)
	dx := dv.X()
	dy := dv.Y()
	N := math.Max(math.Abs(dx), math.Abs(dy))

	var divN float64
	if N == 0 {
		divN = 0.0
	} else {
		divN = 1.0 / N // nolint:gomnd // we're just taking inverse...
	}

	xstep := dx * divN
	ystep := dy * divN

	// BUG-8: the walk must REACH the destination and must not pass it. The old
	// loop ran i <= int(N) and incremented before sampling, so it took int(N)+1
	// unit steps along a segment only N long -- a full extra subtile whenever N
	// was whole, which is every axis-aligned and 45-degree ray between
	// grid-aligned entities. A blocker beyond the target blocked the ray while
	// the reverse ray stayed clear, and a legal endpoint on the map's last
	// subtile was rejected because the sample past it read nil.
	//
	// Ceil(N) steps with the final one clamped to N lands the last sample ON
	// the destination for both whole and fractional N. Plain i < int(N) is NOT
	// the fix: it reaches the endpoint only when N is whole, and for a
	// fractional N it stops short and never examines the destination subtile --
	// a watcher would see through a blocker standing on the player.
	//
	// The position is computed from start each step rather than accumulated, so
	// a long ray does not drift.
	steps := int(math.Ceil(N))

	for i := 1; i <= steps; i++ {
		travelled := math.Min(float64(i), N)
		x := start.X() + xstep*travelled
		y := start.Y() + ystep*travelled

		// SubTileAt returns nil off the map, and off the map is not walkable:
		// the same answer as a wall, and the walk stops at the last good point
		// instead of dereferencing nil. Before SubTileAt was guarded this loop
		// crashed the process on a move target past the map edge, because it
		// does no bounds checking of its own.
		flags := m.SubTileAt(int(math.Floor(x)), int(math.Floor(y)))
		if flags == nil || m.sightBlocked(flags) {
			previous := math.Min(float64(i-1), N)

			return false, d2vector.NewPosition(start.X()+xstep*previous, start.Y()+ystep*previous)
		}
	}

	return true, end
}

// SightRule names which authored bits stop a ray. It is a DIAL rather than a
// constant because measurement made it one, and because it took three
// measurements on 19 September 2026 to find out which rule the game needs.
type SightRule int

// The three rules, and THE ZERO VALUE IS THE SHIPPED ONE. That ordering is
// deliberate and was chosen after getting it wrong: the package's own tests
// build an engine as `&MapEngine{}`, so with anything else at zero the tests
// silently measure a different sight model from the game, and two tests written
// to pin the shipped rule pinned the helper instead. A field nobody sets must
// mean what the game means.
const (
	// SightBlockedBySightFlag is the SHIPPED rule: BlockLOS alone, the bit D2
	// authored for sight. On the inherited Act 1 art this is a large change and
	// the comment on sightBlocked below is the argument for it.
	SightBlockedBySightFlag SightRule = iota

	// SightBlockedByEither stops a ray on BlockWalk OR BlockLOS. It reads the
	// authored sight bit, which is what BUG-9 asked for in the narrow sense, and
	// because the census found BlockLOS to be a strict SUBSET of BlockWalk in
	// this art it is behaviourally identical to the pre-fork rule here -- which
	// is to say it reproduces the night that does not happen. Kept as the
	// conservative option and as the rule to switch to if the slice's own art
	// ever marks its walls opaque properly.
	SightBlockedByEither

	// SightBlockedByWalkFlag is the PRE-19-September behaviour: BlockWalk alone,
	// so the authored sight bit is never read. Kept because every number in this
	// project's record before that date was measured under it, and because it is
	// the negative control that names BUG-9.
	SightBlockedByWalkFlag
)

// sightBlocked decides which authored bits stop a ray, and choosing between
// them is the largest single lever in this build.
//
// D2 authors two bits. BlockWalk marks anything you cannot walk through --
// walls, but also water, low fences, carts, ledges. BlockLOS is meant to mark
// what is genuinely opaque. checkLos consulted BlockWalk alone from the fork
// onwards, so the sight bit was decoded and read nowhere: docs/bugs.md BUG-9.
//
// THE CENSUS FIRST, because it is what makes this a design decision rather than
// a typo. tools/subtilecensus counted every DT1 of Act 1 Town, Wilderness and
// Cave: 38,425 subtiles, BlockWalk 10,252 (26.7%), BlockLOS 1,194 (3.1%), both
// bits 1,194, **BlockLOS-only ZERO**. The sight bit is a strict SUBSET of the
// walk bit here, so honouring it alone REMOVES 9,058 blockers and adds none --
// an 88.4% cut in sight blocking. It is not a neutral correction and this
// comment does not pretend it is.
//
// SO IT WAS DECIDED BY MEASURING BOTH WORLDS, at shipped dials, one full cycle
// each, on the same seed, with nothing spawned and nothing watched:
//
//	walls as cover (walk bit, or the union -- identical on this art):
//	    1 encounter · peak hunters aware 0-1 · health 240 -> 232
//	    and at the old MaxGroups 8 the record's own six-night run called it
//	    "one squall at dusk then a long empty walk" (9 Sep, F6)
//	darkness as cover (sight bit):
//	    12 encounters · peak aware 5 · health 240 -> 13, survived
//
// The first is not a harder game, it is an ABSENT one: on this art the town's
// clutter means a ray of eight to twelve tiles is almost never clear, so
// nothing ever notices the player and the night does not happen. That is the
// build Josh launched on 19 September and could do nothing in but walk around
// and die of thirst. playtest/spawns_test.go measured the same fact from the
// other side: under the sight bit, sixty-four rays from the spawn across radii
// 4, 6, 8 and 10 are ALL clear -- there is no geometric cover near where the
// game starts, at all.
//
// AND THAT IS WHAT THE DESIGN ASKS FOR ANYWAY. Cover in this game is darkness
// and distance, not geometry: the notice radius is 12 tiles and DOUBLES for a
// lit target (NoticeDials.LitMultiplier), R2 makes light a combat system with
// advantage for striking out of the dark, and the whole slice is "the night is
// the enemy". Hiding behind a cart was never the mechanic. Walls-as-cover at
// 26.7% density is an accident of inherited art, not a rule anyone chose.
//
// WHAT IT COSTS, stated rather than buried: water, low fences, carts and ledges
// no longer stop the eye, and neither does anything else the art failed to mark
// BlockLOS -- which on these DT1s is most things. If the slice's own authored
// maps (M5.4) mark their walls properly, this rule becomes exactly right; until
// then it is the better of two wrong worlds, chosen on the numbers above.
//
// Pathfinding is NOT affected. The A* reads flags.BlockWalk directly
// (astar.go:117); checkLos feeds sight and the harness's straight-line control
// and nothing else, so no route smooths itself through a wall.
//
// The difficulty this unlocks is carried by the spawn dials, deliberately, and
// MaxGroups is where it lives (d2world/spawns.go) -- measured 8 -> 2 in the
// same commit.
func (m *MapEngine) sightBlocked(flags *d2dt1.SubTileFlags) bool {
	switch m.sightRule {
	case SightBlockedByWalkFlag:
		return flags.BlockWalk
	case SightBlockedByEither:
		return flags.BlockWalk || flags.BlockLOS
	default:
		return flags.BlockLOS
	}
}

// setSightRule chooses which authored bits stop a ray. The zero value is the
// shipped rule, so an engine nobody configures sees what the game sees;
// CreateMapEngine sets it anyway, because the intent should be readable at the
// construction site rather than inferred from a constant's position.
//
// UNEXPORTED, AND THAT IS THE POINT. An exported setter with no caller outside
// its own tests is the hollow-symbol class this project's reachability register
// exists to find, and adding two of them to claim a seam would have been the
// sixth costume of it. Re-running the A/B means changing the one line in
// CreateMapEngine, which is what the 19 September measurements did. The shipped
// rule is pinned by TestSightShipsTheSightFlagAndTheZeroValueIsThatRule, in CI,
// so a silent flip fails a test rather than quietly invalidating a night's
// numbers -- which is the assurance an exported getter would have been for.
func (m *MapEngine) setSightRule(rule SightRule) {
	m.sightRule = rule
}

// sightRuleOf reports the rule sight obeys.
func (m *MapEngine) sightRuleOf() SightRule {
	return m.sightRule
}
