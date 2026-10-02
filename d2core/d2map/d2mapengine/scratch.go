package d2mapengine

// The A*'s working memory, and the proofs that spare a route its wasted
// searches (BUG-115, 2 Oct 2026).
//
// THE SCRATCH. search() kept what it knew of each subtile in a map keyed by
// the packed subtile (since the per-frame budget's burst), and coarsePath in
// two maps keyed by the subTile struct. A map lookup is the dominant cost of a
// search that runs to its budget. The scratch is a dense grid the size of the
// map, one cell per subtile (and one per tile for the corridor), stamped with
// the search that wrote it: a new search bumps the stamp instead of clearing
// the grid, so starting one costs nothing and reading a cell is one index.
// The village (48x48 tiles) is a 57,600-cell grid, 12 bytes a cell.
//
// NO ROUTE CHANGES. The grid holds exactly what the map held -- the cheapest
// cost found and the step it was reached by -- and is only ever read by the
// subtile's own index, never iterated; the open set's order is total, so the
// same pops happen in the same sequence. TestSearchMatchesTheReferenceSearch
// and TestCoarsePathMatchesTheReference hold both searches to the forms they
// replaced, route for route.
//
// THE PROOFS (RouteQuery). The game's router (mapRouter.Route) asks up to nine
// routes from ONE start -- the eight tiles beside a quarry, nearest first, and
// then its own -- and each was a full PathFind: a search to its 4,000-node cap
// and then the corridor's coarse search. A quarry in a sealed yard cost nine
// of each to learn, nine times over, that the yard is sealed. A RouteQuery
// keeps what its searches learnt about the start's reach, and answers
// "provably unreachable" only when it is a fact of the map:
//
//   - a search from the start that runs its open set dry has enumerated
//     everything the start can reach (its component is that search's
//     visited cells), so any other subtile is unreachable;
//   - a flood from a goal over the walkable subtiles (the A*'s own step rule,
//     which is symmetric between two walkable subtiles) that runs dry inside
//     floodCap cells, and contains neither the start nor any subtile the
//     start can step to, is a sealed region: nothing in it is reachable.
//
// A proven-unreachable goal skips the corridor (it could only fail: its last
// leg must arrive exactly), and a proven-unreachable neighbour tile skips its
// whole PathFind (it could only come back not reaching). Either way the
// answer the caller sees is the one the searches would have given.
// TestARouteQueryAnswersAsTheReferenceDoes holds it to the old PathFind.
//
// Deterministic: nothing here reads a clock or iterates a map, and a proof is
// a pure function of the map and the start, so what a query skips never
// depends on the order its questions came in.

import (
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// floodCap bounds one sealed-region flood. [DIAL] -- 2,048 subtiles is about
// 80 tiles of yard; a bigger region is not proven, only searched as before.
// The flood is a plain walk, several times cheaper a cell than the A*.
const floodCap = 2048

// noStep marks a cell with no predecessor (a search's start).
const noStep = 0xff

// searchCell is what a search knows of one subtile (or the corridor of one
// tile): the cheapest cost to it found so far and the index into
// neighbourOffsets of the step that reached it, valid while stamp is the
// current search's.
type searchCell struct {
	stamp uint32
	cost  int32
	step  uint8
}

// The marks a RouteQuery leaves on a subtile, offset by the query's base.
const (
	markReach  = 1 // reachable from the query's start
	markSealed = 2 // proven unreachable: a sealed region's
	markBig    = 3 // in a region too big to flood: not proven either way
	markSpan   = 4
)

// searchScratch is one search's working memory, reused.
type searchScratch struct {
	width, height int // in subtiles
	stamp         uint32
	cells         []searchCell
	visited       []int32 // the cells the current search (or flood) stamped
	open          openSet
	reversed      []subTile

	tileWidth, tileHeight int
	tileStamp             uint32
	tileCells             []searchCell

	// edges caches lineOpen for the current corridor search: per tile, which
	// of its four orthogonal lines (bit d of known) have been walked, and
	// which were open (bit d of open), valid while stamp is tileStamp.
	edges []tileEdges

	marks    []uint32 // RouteQuery's marks, allocated on first use
	markBase uint32

	// coarseRuns counts corridor tile searches, for the tests that hold a
	// proof to the corridors it spares.
	coarseRuns int
}

func newSearchScratch(tilesW, tilesH int) *searchScratch {
	w, h := tilesW*subtilesPerTile, tilesH*subtilesPerTile

	return &searchScratch{
		width:      w,
		height:     h,
		cells:      make([]searchCell, w*h),
		tileWidth:  tilesW,
		tileHeight: tilesH,
		tileCells:  make([]searchCell, tilesW*tilesH),
		edges:      make([]tileEdges, tilesW*tilesH),
	}
}

// acquireScratch takes the engine's scratch, or makes one when it is in use
// or the map has changed size since it was made.
func (m *MapEngine) acquireScratch() *searchScratch {
	m.scratchMu.Lock()
	s := m.scratch
	m.scratch = nil
	m.scratchMu.Unlock()

	if s == nil || s.tileWidth != m.size.Width || s.tileHeight != m.size.Height {
		s = newSearchScratch(m.size.Width, m.size.Height)
	}

	return s
}

// releaseScratch gives a scratch back for the next search.
func (m *MapEngine) releaseScratch(s *searchScratch) {
	m.scratchMu.Lock()
	m.scratch = s
	m.scratchMu.Unlock()
}

// begin starts a new search (or flood): a fresh stamp, so every cell reads
// unseen, and an empty visited list.
func (s *searchScratch) begin() {
	s.stamp++
	if s.stamp == 0 { // wrapped: the old stamps could collide, so clear them
		for i := range s.cells {
			s.cells[i].stamp = 0
		}

		s.stamp = 1
	}

	s.visited = s.visited[:0]
}

// beginTiles is begin for the corridor's tile grid.
func (s *searchScratch) beginTiles() {
	s.tileStamp++
	if s.tileStamp == 0 {
		for i := range s.tileCells {
			s.tileCells[i].stamp = 0
			s.edges[i].stamp = 0
		}

		s.tileStamp = 1
	}
}

// index is a subtile's cell, or false off the grid.
func (s *searchScratch) index(t subTile) (int, bool) {
	if t.x < 0 || t.y < 0 || t.x >= s.width || t.y >= s.height {
		return 0, false
	}

	return t.y*s.width + t.x, true
}

// walk follows the steps back from reached to start through cells and
// returns start-exclusive, reached-inclusive travel order; nil when reached is
// start, or when the chain is broken (it never is).
func (s *searchScratch) walk(cells []searchCell, stamp uint32, width int, start, reached subTile) []subTile {
	if reached == start {
		return nil
	}

	reversed := s.reversed[:0]

	for node := reached; node != start; {
		reversed = append(reversed, node)

		c := cells[node.y*width+node.x]
		if c.stamp != stamp || c.step == noStep {
			s.reversed = reversed

			return nil
		}

		off := neighbourOffsets[c.step]
		node = subTile{node.x - off.x, node.y - off.y}
	}

	s.reversed = reversed

	forward := make([]subTile, len(reversed))
	for i, node := range reversed {
		forward[len(reversed)-1-i] = node
	}

	return forward
}

// RouteQuery is several PathFinds from one start, which share what their
// searches learn (see the top of this file). It holds the engine's scratch
// until Close. Not safe for concurrent use; one per Route call.
type RouteQuery struct {
	m        *MapEngine
	s        *searchScratch
	from     subTile
	base     uint32
	failed   bool // a search from the start has come back not arriving
	sealedIn bool // a search from the start ran dry: its reach is all marked
	searches int
}

// NewRouteQuery starts a query from start (a SUBTILE position, as PathFind's).
func (m *MapEngine) NewRouteQuery(start d2vector.Position) *RouteQuery {
	return &RouteQuery{
		m:    m,
		s:    m.acquireScratch(),
		from: subTile{int(math.Floor(start.X())), int(math.Floor(start.Y()))},
	}
}

// Close gives the scratch back. The query must not be used after.
func (q *RouteQuery) Close() {
	if q.s != nil {
		q.m.releaseScratch(q.s)
		q.s = nil
	}
}

// Searches is how many A* searches the query has run: one a PathFind, plus a
// corridor's legs. A diagnostic for the benchmarks; nothing budgets on it.
func (q *RouteQuery) Searches() int { return q.searches }

// PathFind is MapEngine.PathFind from the query's start, identical in what it
// returns; it skips the corridor when the goal is proven unreachable.
func (q *RouteQuery) PathFind(dest d2vector.Position) []d2vector.Position {
	goal := subTile{int(math.Floor(dest.X())), int(math.Floor(dest.Y()))}

	if q.from == goal {
		return []d2vector.Position{dest}
	}

	result := q.s.search(q.m, q.from, goal)
	q.searches++
	q.learn(result)

	steps, exact := result.steps, result.exact

	// The long way round (corridor.go): tried only when the ordinary search
	// ran out of BUDGET -- not when it proved the goal walled off -- and used
	// only if the whole corridor arrives, which a proven-unreachable goal
	// cannot.
	if !exact && result.exhausted && !q.unreachable(goal) {
		long, ok, legs := q.s.corridorRoute(q.m, q.from, goal)
		q.searches += legs

		if ok {
			steps, exact = long, true
		}
	}

	if len(steps) == 0 {
		return []d2vector.Position{}
	}

	return waypoints(q.from, steps, dest, exact)
}

// ProvenUnreachable reports whether PathFind(dest) from the query's start is
// PROVEN to neither arrive nor end anywhere in tile (tileX, tileY) -- the two
// ways mapRouter.routeExact can call a route reachable. False means "not
// proven", never "reachable". It spends nothing until a search from the start
// has failed: the first candidate of a Route that routes pays no proof.
func (q *RouteQuery) ProvenUnreachable(dest d2vector.Position, tileX, tileY int) bool {
	if !q.failed {
		return false
	}

	if !q.unreachable(subTile{int(math.Floor(dest.X())), int(math.Floor(dest.Y()))}) {
		return false
	}

	for oy := 0; oy < subtilesPerTile; oy++ {
		for ox := 0; ox < subtilesPerTile; ox++ {
			if !q.unreachable(subTile{tileX*subtilesPerTile + ox, tileY*subtilesPerTile + oy}) {
				return false
			}
		}
	}

	return true
}

// ensureMarks allocates the marks grid and takes this query's three values.
func (q *RouteQuery) ensureMarks() {
	if q.base != 0 {
		return
	}

	s := q.s
	if len(s.marks) != len(s.cells) {
		s.marks = make([]uint32, len(s.cells))
		s.markBase = 0
	}

	if s.markBase >= math.MaxUint32-2*markSpan { // wrapped: clear
		for i := range s.marks {
			s.marks[i] = 0
		}

		s.markBase = 0
	}

	s.markBase += markSpan
	q.base = s.markBase
}

// learn records what one search from the start proved: every cell it
// stamped is reachable, and a search that ran dry stamped ALL of the start's
// reach. Only failures teach anything a later question needs.
func (q *RouteQuery) learn(r searchResult) {
	if r.exact {
		return
	}

	q.failed = true
	q.ensureMarks()

	for _, i := range q.s.visited {
		q.s.marks[i] = q.base + markReach
	}

	if !r.exhausted {
		q.sealedIn = true
	}
}

// unreachable reports whether subtile t is proven unreachable from the start.
func (q *RouteQuery) unreachable(t subTile) bool {
	if t == q.from {
		return false
	}

	if q.m.blockedAt(t.x, t.y) {
		// Never in an open set, so never reached: a blocked subtile is no
		// search's answer (the start excepted, above).
		return true
	}

	if !q.failed {
		return false
	}

	q.ensureMarks()

	i := t.y*q.s.width + t.x // walkable, so on the grid

	switch q.s.marks[i] {
	case q.base + markReach, q.base + markBig:
		return false
	case q.base + markSealed:
		return true
	}

	if q.sealedIn {
		return true // the start's whole reach is marked, and t is not in it
	}

	return q.floodSealed(t)
}

// stepOpen is the A*'s step rule: next walkable, and a diagonal needs both of
// its orthogonal neighbours walkable.
func (m *MapEngine) stepOpen(x, y, k int) bool {
	off := neighbourOffsets[k]

	if m.blockedAt(x+off.x, y+off.y) {
		return false
	}

	if off.x != 0 && off.y != 0 {
		return !m.blockedAt(x+off.x, y) && !m.blockedAt(x, y+off.y)
	}

	return true
}

// floodSealed floods t's region of walkable subtiles and marks it: reachable
// (it touches the start's known reach, the start, or a subtile the start can
// step to), sealed (none of those, and it ran dry), or too big (floodCap).
// Between two walkable subtiles the step rule is symmetric, so a dry flood is
// t's whole region and the start reaches it only through one of its own
// first steps -- the start itself may stand on a blocked subtile.
//
// Two arguments keep a sealed verdict honest, and either is enough. A flood
// runs only after a search from the start failed without running dry, so the
// start's region holds more than maxExpandedNodes subtiles and a flood inside
// it would hit floodCap (smaller) before it ran dry. And the flood stops at
// the start's known reach, and checks the start's first steps. The second is
// what holds at any cap: the control that raises floodCap and drops both
// checks goes red; raising floodCap alone stays green.
func (q *RouteQuery) floodSealed(t subTile) bool {
	s, m := q.s, q.m

	s.begin()

	start := t.y*s.width + t.x
	s.cells[start].stamp = s.stamp
	s.visited = append(s.visited, int32(start))

	verdict := uint32(markSealed)

flood:
	for head := 0; head < len(s.visited); head++ {
		if len(s.visited) >= floodCap {
			verdict = markBig

			break
		}

		i := int(s.visited[head])
		x, y := i%s.width, i/s.width

		for k := range neighbourOffsets {
			if !m.stepOpen(x, y, k) {
				continue
			}

			off := neighbourOffsets[k]
			ni := (y+off.y)*s.width + x + off.x

			if s.cells[ni].stamp == s.stamp {
				continue
			}

			s.cells[ni].stamp = s.stamp
			s.visited = append(s.visited, int32(ni))

			switch s.marks[ni] {
			case q.base + markReach: // t's region is the start's
				verdict = markReach

				break flood
			case q.base + markBig: // t's region is one already too big
				verdict = markBig

				break flood
			}
		}
	}

	// Belt and braces: learn has already marked the start's first steps
	// reachable (every failed search discovers them), so a region holding one
	// stopped at it above. Checked anyway, because a sealed verdict is the one
	// that skips work.
	if verdict == markSealed && q.reachesInto() {
		verdict = markReach
	}

	for _, i := range s.visited {
		s.marks[i] = q.base + verdict
	}

	return verdict == markSealed
}

// reachesInto reports whether the start, or a subtile the start can step to,
// is in the flood just run (stamped with the current stamp).
func (q *RouteQuery) reachesInto() bool {
	s := q.s

	if i, ok := s.index(q.from); ok && s.cells[i].stamp == s.stamp {
		return true
	}

	for k, off := range neighbourOffsets {
		if !q.m.stepOpen(q.from.x, q.from.y, k) {
			continue
		}

		if i, ok := s.index(subTile{q.from.x + off.x, q.from.y + off.y}); ok && s.cells[i].stamp == s.stamp {
			return true
		}
	}

	return false
}
