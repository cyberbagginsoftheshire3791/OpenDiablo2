package d2gamescreen

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// T1, the tactical layer's game-screen half (23 Sep 2026). d2world decides
// whose turn it is and what a blow does; this file is everything that needs the
// map: walking a pack on its turn, refusing a Move that is too long, turning a
// click on an enemy into a strike, and paying the world for each round.
//
// See d2core/d2world/combat_tactical.go for why the layer exists at all.

// tacticalStrikeSettle is how many frames the hero must have stood still after
// a move-to-strike before the strike is sent, so the blow does not resolve
// against the position he is leaving.
const tacticalStrikeSettle = 2

// tacticalWalkSpeed is the slowest a body walks on its turn, in subtiles per
// second (5 to a tile). [DIAL] Measured on the first real run: a zombie1 at
// D2's own Velocity covered half a tile in the 2.5 s the fight waits for a
// walk, so a pack's turn read as a stall. At 12 a three-tile move takes about
// 1.25 s. A body already faster keeps its own pace, and the borrowed pace is
// handed back when the walk ends (the path's done callback), so a chase after
// the fight runs at the creature's authored speed.
const tacticalWalkSpeed = 12.0

// StepToward is the d2world.Stepper half that walks an enemy on its turn: to a
// free tile beside (x, y), cut to at most `tiles` tiles of travel. It never
// walks onto a tile another body stands on, which is what keeps a pack from
// stacking on one square around the player.
func (v *Game) StepToward(id string, x, y float64, tiles int) bool {
	walker, ok := v.walkerByID(id)
	if !ok || tiles <= 0 {
		return false
	}

	fx, fy := walker.GetPositionF()

	path, _, ok := v.tacticalRoute(id, fx, fy, x, y)
	if !ok {
		return false
	}

	// THE WALK MUST END ON A FREE TILE, and the goal being free is not enough:
	// a cut walk ends on an intermediate waypoint, and a packmate ordered a
	// moment earlier has already claimed its destination without standing on
	// it yet. A leg cut short stops short of a taken tile; a whole waypoint
	// on one is backed off (review findings).
	taken := v.occupiedTiles(id)
	free := func(x, y float64) bool { return !taken[[2]int{int(math.Floor(x)), int(math.Floor(y))}] }

	path = truncateRoute(fx, fy, path, tiles, free)

	for len(path) > 0 {
		end := path[len(path)-1]
		if !taken[[2]int{int(math.Floor(end[0])), int(math.Floor(end[1]))}] {
			break
		}

		path = path[:len(path)-1]
	}

	if len(path) == 0 {
		return false
	}

	end := path[len(path)-1]
	v.reserve(id, end[0], end[1])

	positions := make([]d2vector.Position, 0, len(path))
	for _, w := range path {
		positions = append(positions, d2vector.NewPositionTile(w[0], w[1]))
	}

	walker.SetPath(positions, v.borrowPace(id, walker))

	return true
}

// borrowPace raises a slow body to tacticalWalkSpeed for one walk and returns
// the callback that gives its own speed back. The original is kept by id, so a
// second order before the first walk finished cannot capture the borrowed pace
// as the original.
func (v *Game) borrowPace(id string, walker pathWalker) func() {
	paced, ok := walker.(interface {
		GetSpeed() float64
		SetSpeed(float64)
	})
	if !ok {
		return nil
	}

	if v.tacticalPace == nil {
		v.tacticalPace = map[string]float64{}
	}

	if _, held := v.tacticalPace[id]; !held {
		if paced.GetSpeed() >= tacticalWalkSpeed {
			return nil
		}

		v.tacticalPace[id] = paced.GetSpeed()
	}

	paced.SetSpeed(tacticalWalkSpeed)

	return func() {
		if orig, held := v.tacticalPace[id]; held {
			paced.SetSpeed(orig)
			delete(v.tacticalPace, id)
		}
	}
}

// Halt stops a body where it stands, for a paced fight opening: releasing a
// chase forgets it but leaves the path in flight, and the hero may be halfway
// through an ordinary walk.
func (v *Game) Halt(id string) {
	walker, ok := v.walkerByID(id)
	if !ok {
		return
	}

	if s, ok := walker.(interface{ StopMoving() }); ok {
		s.StopMoving()
	}

	// StopMoving drops the path's done callback, so the borrowed pace is
	// handed back here rather than trusted to it (review finding).
	v.returnPace(id, walker)
	delete(v.tacticalReserved, id)
}

// returnPace gives a body back its own walking speed, if a tactical walk
// borrowed one.
func (v *Game) returnPace(id string, walker pathWalker) {
	orig, held := v.tacticalPace[id]
	if !held {
		return
	}

	if paced, ok := walker.(interface{ SetSpeed(float64) }); ok {
		paced.SetSpeed(orig)
	}

	delete(v.tacticalPace, id)
}

// returnAllPace hands every borrowed pace back once no paced fight is running,
// so no creature leaves a fight faster than it was authored.
func (v *Game) returnAllPace() {
	for id := range v.tacticalPace {
		if walker, ok := v.walkerByID(id); ok {
			v.returnPace(id, walker)
		} else {
			delete(v.tacticalPace, id)
		}
	}

	for id := range v.tacticalReserved {
		delete(v.tacticalReserved, id)
	}
}

// Moving is the Stepper half the fight waits on: a pack's walk, or the hero's.
func (v *Game) Moving(id string) bool {
	walker, ok := v.walkerByID(id)

	return ok && walker.IsMoving()
}

func (v *Game) walkerByID(id string) (pathWalker, bool) {
	if v.gameClient == nil || v.gameClient.MapEngine == nil {
		return nil, false
	}

	entity, ok := v.gameClient.MapEngine.Entities()[id]
	if !ok {
		return nil, false
	}

	walker, ok := entity.(pathWalker)

	return walker, ok
}

// tacticalRoute finds a walk from (fx, fy) to the nearest free, routable tile
// beside (tx, ty), and reports its cost in tiles. "Free" is the map's walk bit
// AND no other body standing there -- the pursuit router deliberately ignores
// bodies (entities do not block the A*), which is right for a chase and wrong
// for a turn, where two wolves on one square read as a bug.
func (v *Game) tacticalRoute(selfID string, fx, fy, tx, ty float64) (path [][2]float64, cost int, ok bool) {
	r := mapRouter{engine: v.gameClient.MapEngine}
	taken := v.occupiedTiles(selfID)

	blocked := func(tileX, tileY float64) bool {
		if r.blockedTile(tileX, tileY) {
			return true
		}

		return taken[[2]int{int(math.Floor(tileX)), int(math.Floor(tileY))}]
	}

	for _, n := range unblockedNeighbours(fx, fy, tx, ty, blocked) {
		gx, gy := tx+n[0], ty+n[1]

		// Already standing there.
		if math.Floor(gx) == math.Floor(fx) && math.Floor(gy) == math.Floor(fy) {
			return nil, 0, true
		}

		route, reachable := r.routeExact(fx, fy, gx, gy)
		if !reachable {
			continue
		}

		return route, routeTiles(fx, fy, route), true
	}

	return nil, 0, false
}

// occupiedTiles is every tile a living body stands on, except selfID's.
func (v *Game) occupiedTiles(selfID string) map[[2]int]bool {
	out := map[[2]int]bool{}

	for id, e := range v.gameClient.MapEngine.Entities() {
		if id == selfID {
			continue
		}

		switch e.(type) {
		case *d2mapentity.Player, *d2mapentity.NPC, *d2mapentity.Creature:
		default:
			continue
		}

		if v.bodyDead(id) {
			continue
		}

		x, y := e.GetPositionF()
		out[[2]int{int(math.Floor(x)), int(math.Floor(y))}] = true
	}

	// A destination already ordered is taken, until its walker arrives (or is
	// halted) -- then its body marks the tile itself.
	for rid, tile := range v.tacticalReserved {
		if rid == selfID {
			continue
		}

		if !v.Moving(rid) {
			delete(v.tacticalReserved, rid)
			continue
		}

		out[tile] = true
	}

	return out
}

// reserve claims the tile a walk will end on.
func (v *Game) reserve(id string, x, y float64) {
	if v.tacticalReserved == nil {
		v.tacticalReserved = map[string][2]int{}
	}

	v.tacticalReserved[id] = [2]int{int(math.Floor(x)), int(math.Floor(y))}
}

// bodyDead reports a known body at zero, without adopting one (npc_body.go's
// peek, the same read the overhead bars use).
func (v *Game) bodyDead(id string) bool {
	if b, ok := v.bodies[id]; ok && b != nil {
		return b.health <= 0
	}

	return false
}

// routeTiles is a route's cost in whole tiles, counted the way adjacency is:
// the Chebyshev distance from the start tile to the end tile, so a diagonal
// step is one tile -- matching the Move's refusal rule to the reach rule the
// player already sees on the grid -- and never less than the walk's length
// divided by a diagonal's, so a long detour round a wall still costs what it
// walks.
//
// MEASURED WRONG THE FIRST TIME, on the first real run: counting each tile
// boundary the subtile path crossed charged a zigzag diagonal as two steps per
// tile, so a three-tile Move read as five and was refused, and a pack's
// three-tile walk was cut to one.
func routeTiles(fx, fy float64, route [][2]float64) int {
	if len(route) == 0 {
		return 0
	}

	end := route[len(route)-1]
	cheb := chebyshevTiles(fx, fy, end[0], end[1])

	if walk := int(math.Ceil(routeLength(fx, fy, route)/math.Sqrt2 - 0.05)); walk > cheb {
		return walk
	}

	return cheb
}

// chebyshevTiles is the tile distance between two world points' floored tiles.
func chebyshevTiles(ax, ay, bx, by float64) int {
	dx := math.Abs(math.Floor(bx) - math.Floor(ax))
	dy := math.Abs(math.Floor(by) - math.Floor(ay))

	return int(math.Max(dx, dy))
}

// routeLength is the walk's Euclidean length in tiles from (fx, fy).
func routeLength(fx, fy float64, route [][2]float64) float64 {
	length, cx, cy := 0.0, fx, fy

	for _, w := range route {
		length += math.Hypot(w[0]-cx, w[1]-cy)
		cx, cy = w[0], w[1]
	}

	return length
}

// truncateRoute keeps the walk a `tiles`-tile move reaches, by the same
// measure routeTiles charges. The waypoint that would carry it past the move
// is not dropped but cut short: the path finder returns a straight run over
// open ground as ONE waypoint at its far end, and dropping that left a body
// more than a move away with nowhere to go -- a risen man six tiles off across
// the village green stood still for forty rounds (the default-game sweep,
// 23 Sep 2026). free, when given, is where a cut leg may END (not a tile
// another body stands on or has claimed): the leg is cut short of the first
// point that is not, so a body behind a packmate on its straight line stops
// behind him rather than not at all.
func truncateRoute(fx, fy float64, route [][2]float64, tiles int, free func(x, y float64) bool) [][2]float64 {
	limit := float64(tiles)*math.Sqrt2 + 0.05
	length, cx, cy := 0.0, fx, fy
	out := make([][2]float64, 0, len(route))

	within := func(x, y, walked float64) bool {
		return walked <= limit && chebyshevTiles(fx, fy, x, y) <= tiles
	}

	for _, w := range route {
		seg := math.Hypot(w[0]-cx, w[1]-cy)

		if within(w[0], w[1], length+seg) {
			length += seg
			cx, cy = w[0], w[1]
			out = append(out, w)

			continue
		}

		// Part of this leg fits: go as far along it as the move allows, and
		// no further than the last free point.
		cutWithin := within
		if free != nil {
			cutWithin = func(x, y, walked float64) bool { return within(x, y, walked) && free(x, y) }
		}

		if cut, ok := cutLeg(cx, cy, w, length, truncateStep, cutWithin); ok {
			out = append(out, cut)
		}

		break
	}

	return out
}

// truncateStep is how finely a leg is cut, in tiles: a twentieth of a
// subtile's width is far below anything the walk or the tile checks see.
const truncateStep = 0.01

// cutLeg is the furthest point along the leg from (cx, cy) to w, in steps of
// step tiles, that within accepts (having walked `walked` tiles before the
// leg). ok is false when not even the first step fits.
func cutLeg(cx, cy float64, w [2]float64, walked, step float64,
	within func(x, y, walked float64) bool) (end [2]float64, ok bool) {
	seg := math.Hypot(w[0]-cx, w[1]-cy)
	if seg == 0 {
		return end, false
	}

	ux, uy := (w[0]-cx)/seg, (w[1]-cy)/seg

	for d := step; d < seg; d += step {
		x, y := cx+ux*d, cy+uy*d
		if !within(x, y, walked+d) {
			break
		}

		end, ok = [2]float64{x, y}, true
	}

	return end, ok
}

// tacticalAdvance runs once per live frame: the fight's own clock, the world
// time its rounds owe, and a move-to-strike waiting for the hero to arrive.
func (v *Game) tacticalAdvance(elapsed float64) {
	if v.combat == nil {
		return
	}

	v.combat.Tick(elapsed)

	// A paced round is paid for here, with exactly the world minutes it cost:
	// the clock's own rate turns them back into the simulated seconds
	// advanceWorld takes, so the clock, light, meters, spawns and pursuit all
	// see one round's minute and not a frame of wall time more.
	if owed := v.combat.TakeRoundMinutes(); owed > 0 && v.worldClock != nil {
		if rate := v.worldClock.Rate(); rate > 0 {
			v.advanceWorld(owed / rate)
		}
	}

	v.settlePendingStrike()

	if !v.combat.Fighting() && (len(v.tacticalPace) > 0 || len(v.tacticalReserved) > 0) {
		v.returnAllPace()
	}
}

// settlePendingStrike sends the strike a click on a distant enemy asked for,
// once the hero has finished the walk that brought him into reach.
func (v *Game) settlePendingStrike() {
	if v.pendingStrike == "" {
		return
	}

	if !v.combat.Awaiting() || v.localPlayer == nil {
		v.pendingStrike = ""
		return
	}

	if v.localPlayer.IsMoving() {
		v.pendingStrikeStill = 0
		return
	}

	v.pendingStrikeStill++
	if v.pendingStrikeStill < tacticalStrikeSettle {
		return
	}

	target := v.pendingStrike
	v.pendingStrike = ""

	// ONLY THE ENEMY HE CLICKED. Commit's empty-or-unreachable fallback would
	// strike the first adjacent enemy in D8 order instead, which is not what a
	// click on one wolf asked for (review finding).
	adjacent := false

	for _, e := range v.combat.Tactical().Enemies {
		if e.ID == target && e.Adjacent && !e.Dead && !e.Routed {
			adjacent = true
		}
	}

	if !adjacent {
		v.tacticalNotice(d2player.TacticalOutOfReach)
		return
	}

	if err := v.combat.Commit(d2world.CommitStrike, target); err != nil {
		v.tacticalNotice(err.Error())
	}
}

// tacticalMove is a ground click inside a paced fight. It reports whether it
// handled the click; false means no paced fight, and the walk is ordinary.
func (v *Game) tacticalMove(tx, ty float64) bool {
	if v.combat == nil || !v.combat.Fighting() || !v.combat.Paced() {
		return false
	}

	if !v.combat.Awaiting() {
		v.tacticalNotice(d2player.TacticalNotYourTurn)
		return true
	}

	if v.combat.MoveSpent() {
		v.tacticalNotice(d2player.TacticalMoveSpent)
		return true
	}

	if v.localPlayer == nil || v.localPlayer.IsMoving() {
		return true
	}

	world := v.localPlayer.Position.World()
	fx, fy := world.X(), world.Y()

	// Snap to HIS lattice: whole tiles from where he stands, which is where the
	// overlay centres its diamonds. A click anywhere inside a diamond walks to
	// that diamond's centre, and the Chebyshev count is whole tiles exactly.
	gx, gy := fx+math.Round(tx-fx), fy+math.Round(ty-fy)

	if v.occupiedTiles(v.localPlayer.ID())[[2]int{int(math.Floor(gx)), int(math.Floor(gy))}] {
		v.tacticalNotice(d2player.TacticalTileTaken)
		return true
	}

	route, reachable := mapRouter{engine: v.gameClient.MapEngine}.routeExact(fx, fy, gx, gy)
	if !reachable {
		v.tacticalNotice(d2player.TacticalNoWay)
		return true
	}

	moveTiles := v.combat.Tactical().MoveTiles

	// REFUSE, NOT CLAMP (the c-2 ruling on MoveTiles): a click past the Move
	// does nothing but say so, because a clamped walk ends somewhere he did not
	// choose, and the overlay already shows him where he can go.
	if cost := routeTiles(fx, fy, route); cost > moveTiles {
		v.tacticalNotice(fmt.Sprintf(d2player.TacticalTooFar, cost, moveTiles))
		return true
	}

	v.combat.SpendMove()
	v.sendMove(gx, gy)

	return true
}

// OnTacticalTarget is a click on an enemy inside a paced fight: strike it if it
// is in reach, or spend the Move walking beside it and strike on arrival.
func (v *Game) OnTacticalTarget(id string) {
	if v.combat == nil || !v.combat.Fighting() || !v.combat.Paced() {
		return
	}

	if !v.combat.Awaiting() {
		v.tacticalNotice(d2player.TacticalNotYourTurn)
		return
	}

	view := v.combat.Tactical()

	var target *d2world.TacticalEnemy

	for i := range view.Enemies {
		if view.Enemies[i].ID == id && !view.Enemies[i].Dead && !view.Enemies[i].Routed {
			target = &view.Enemies[i]
		}
	}

	if target == nil {
		return
	}

	if target.Adjacent {
		if view.ActionSpent {
			v.tacticalNotice(d2player.TacticalActionSpent)
			return
		}

		if err := v.combat.Commit(d2world.CommitStrike, id); err != nil {
			v.tacticalNotice(err.Error())
		}

		return
	}

	if view.MoveSpent || v.localPlayer == nil || v.localPlayer.IsMoving() {
		v.tacticalNotice(d2player.TacticalOutOfReach)
		return
	}

	world := v.localPlayer.Position.World()
	fx, fy := world.X(), world.Y()

	route, cost, ok := v.tacticalRoute(v.localPlayer.ID(), fx, fy, target.X, target.Y)
	if !ok || len(route) == 0 {
		v.tacticalNotice(d2player.TacticalNoWay)
		return
	}

	if cost > view.MoveTiles {
		v.tacticalNotice(fmt.Sprintf(d2player.TacticalTooFar, cost, view.MoveTiles))
		return
	}

	last := route[len(route)-1]

	v.combat.SpendMove()
	v.sendMove(last[0], last[1])

	if !view.ActionSpent {
		v.pendingStrike = id
		v.pendingStrikeStill = 0
	}
}

// tacticalNotice puts one line on the HUD's combat panel.
func (v *Game) tacticalNotice(msg string) {
	if v.gameControls != nil {
		v.gameControls.TacticalNotice(msg)
	}
}

// The interface assertions the stepper relies on, checked at compile time for
// the three kinds of body a fight can hold.
var (
	_ pathWalker            = (*d2mapentity.NPC)(nil)
	_ pathWalker            = (*d2mapentity.Creature)(nil)
	_ pathWalker            = (*d2mapentity.Player)(nil)
	_ d2interface.MapEntity = (*d2mapentity.Creature)(nil)
	_ d2world.Stepper       = (*Game)(nil)
)
