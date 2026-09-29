package d2world

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// M4.6 B2b: the fake world the three entity-keyed snapshots are tested in.
//
// It is the game's advanceWorld in miniature -- a clock, the spawn tables,
// the notice model and pursuit, joined the way the game joins them -- over
// fake entities that walk the routes pursuit hands them. Nothing in it needs
// a map, MPQs or ebiten.
//
// A RESUME IS A RELAUNCH. The resumed world is built fresh with its own seed
// and its own player id (the connection's uuid is new every launch), its
// entities are rebuilt with their saved ids and motion (what the d2mapentity
// seam and Motion do for the game), and then the three snapshots are restored
// through a Resolver over the NEW world. So a snapshot that wrote the old
// player id, or leaned on the old seed, cannot pass.

// b2bTickSeconds is one tick of the fake world, in simulated seconds: half a
// world minute at night, 0.8 by day. It must be finer than the notice
// model's one-minute look and pursuit's two-minute re-path, or every watch
// looks and every chase re-solves on every tick, their saved clocks are
// always zero, and losing them could never show.
const b2bTickSeconds = 0.2

// b2bWall is the fake map's one wall: a line of x. Sight across it is blocked,
// so a watcher can lose sight of the player and remember him.
const b2bWall = 38.5

// b2bEntity is a fake map entity: a Watcher and a Hunter that walks the route
// it is given, at a speed in tiles per world minute.
type b2bEntity struct {
	id    string
	x, y  float64
	route [][2]float64
	speed float64
}

func (e *b2bEntity) WatcherID() string             { return e.id }
func (e *b2bEntity) WatcherAt() (float64, float64) { return e.x, e.y }
func (e *b2bEntity) HunterID() string              { return e.id }
func (e *b2bEntity) HunterAt() (float64, float64)  { return e.x, e.y }
func (e *b2bEntity) Following() bool               { return len(e.route) > 0 }

func (e *b2bEntity) Follow(waypoints [][2]float64) {
	e.route = append([][2]float64(nil), waypoints...)
}

// step walks the route for the minutes that passed.
func (e *b2bEntity) step(minutes float64) {
	budget := e.speed * minutes

	for budget > 0 && len(e.route) > 0 {
		dx, dy := e.route[0][0]-e.x, e.route[0][1]-e.y

		d := math.Hypot(dx, dy)
		if d <= budget {
			e.x, e.y = e.route[0][0], e.route[0][1]
			e.route = e.route[1:]
			budget -= d

			continue
		}

		e.x += dx / d * budget
		e.y += dy / d * budget
		budget = 0
	}
}

// clone is the entity rebuilt with its saved id and motion.
func (e *b2bEntity) clone() *b2bEntity {
	c := *e
	c.route = append([][2]float64(nil), e.route...)

	return &c
}

// b2bEntityQuarry is an entity as a quarry: what a harness chase of one
// entity by another hands pursuit.
type b2bEntityQuarry struct{ e *b2bEntity }

func (q b2bEntityQuarry) QuarryID() string             { return q.e.id }
func (q b2bEntityQuarry) QuarryAt() (float64, float64) { return q.e.x, q.e.y }

// b2bPlayer is the player: a quarry whose id is new every launch.
type b2bPlayer struct {
	id   string
	x, y float64
}

func (p *b2bPlayer) QuarryID() string             { return p.id }
func (p *b2bPlayer) QuarryAt() (float64, float64) { return p.x, p.y }

// b2bPlayerAt is where the player stands on a tick: a slow wander that
// crosses the wall, so the watchers east of it lose him and find him again.
func b2bPlayerAt(tick int) (x, y float64) {
	k := float64(tick)

	return 40 + 6*math.Sin(k/15), 40 + 2*math.Cos(k/23)
}

type b2bSight struct{}

func (b2bSight) Clear(fromX, _, toX, _ float64) bool {
	return !(math.Min(fromX, toX) < b2bWall && b2bWall < math.Max(fromX, toX))
}

type b2bLight struct{ level float64 }

func (l b2bLight) Level(_, _ int) float64 { return l.level }

// b2bRouter routes on the straight line and stops a tile short, the way the
// game's router stops beside the quarry.
type b2bRouter struct{}

func (b2bRouter) Route(fromX, fromY, toX, toY float64) ([][2]float64, bool) {
	d := math.Hypot(toX-fromX, toY-fromY)
	if d < 1 {
		return nil, true
	}

	endX, endY := toX-(toX-fromX)/d, toY-(toY-fromY)/d

	return [][2]float64{{(fromX + endX) / 2, (fromY + endY) / 2}, {endX, endY}}, d < 40
}

// b2bSpawner places fake entities and names them in creation order -- the
// stand-in for the uuid stream and gameSpawner.arrival, both of which the
// game restores beside these snapshots. Some calls are scripted: the first
// places nothing (a failure), the second places one short (a dropped
// member), and the third places its pack far out of sight (a pack that
// never notices him, which daybreak sends home). The last of a pack of three
// or more is a straggler who stands too far off to see him, and the rest
// walk at different paces, so a pack strings out along its chase.
type b2bSpawner struct {
	w     *b2bWorld
	calls int
	made  int
}

func (s *b2bSpawner) Spawn(_, _ string, count int, aroundX, aroundY, minTiles, maxTiles float64) []Watcher {
	s.calls++

	switch s.calls {
	case 1:
		return nil
	case 2:
		if count > 1 {
			count--
		}
	}

	reach := minTiles
	if s.calls == 3 {
		reach = maxTiles + 30
	}

	out := make([]Watcher, 0, count)

	for i := 0; i < count; i++ {
		s.made++

		off := reach + 0.5*float64(i)
		if count >= 3 && i == count-1 {
			off += 25 // the straggler
		}

		e := &b2bEntity{
			id: fmt.Sprintf("e:%d", s.made),
			x:  aroundX + off,
			y:  aroundY + 0.25*float64(i),
			// Slower than he wanders, so a pack trails him and some of it
			// loses him at the wall while the rest still see him.
			speed: 0.08 + 0.06*float64(s.made%3),
		}

		s.w.entities[e.id] = e
		out = append(out, e)
	}

	return out
}

func (s *b2bSpawner) Despawn(members []Watcher) {
	for _, m := range members {
		delete(s.w.entities, m.WatcherID())
	}
}

// b2bWorld is the miniature advanceWorld.
type b2bWorld struct {
	seed     int64 // the game seed; the tables' stream is derived from it, as CreateGame derives it
	clock    *Clock
	notice   *Notice
	spawns   *Spawns
	pursuit  *Pursuit
	player   *b2bPlayer
	spawner  *b2bSpawner
	entities map[string]*b2bEntity
	ticks    int
}

func b2bNewWorld(t *testing.T, seed int64, playerID string) *b2bWorld {
	t.Helper()

	w := &b2bWorld{
		seed:     seed,
		player:   &b2bPlayer{id: playerID},
		entities: make(map[string]*b2bEntity),
	}
	w.player.x, w.player.y = b2bPlayerAt(0)
	w.spawner = &b2bSpawner{w: w}

	dials := DefaultSpawnDials()
	dials.Chance = 50 // every eligible row fires at every check, up to the cap
	dials.MaxGroups = 3

	w.clock = NewClock(DefaultClockDials())
	// A long memory, so a watcher that has lost him at the wall is still
	// coming for him while others see him.
	notice := DefaultNoticeDials()
	notice.MemoryMinutes = 30

	w.notice = NewNotice(b2bSight{}, b2bLight{0.2}, notice)
	w.pursuit = NewPursuit(b2bRouter{}, DefaultPursuitDials())
	w.spawns = NewSpawns(w.clock, w.notice, w.spawner, w.pursuit, b2bLight{0.2}, d2rand.Derive(seed, d2rand.StreamSpawns), dials)
	w.spawns.SetTarget(w.player)

	t.Cleanup(w.clock.Close)
	t.Cleanup(w.pursuit.Close)
	t.Cleanup(w.spawns.Close)

	return w
}

func (w *b2bWorld) entityIDs() []string {
	ids := make([]string, 0, len(w.entities))
	for id := range w.entities {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}

// tick is one frame of advanceWorld, in the game's order: the entities walk,
// pursuit re-paths, the tables run (and step Notice), then the aware start
// their chases.
func (w *b2bWorld) tick() {
	w.ticks++
	minutes := w.clock.Advance(b2bTickSeconds)
	w.player.x, w.player.y = b2bPlayerAt(w.ticks)

	for _, id := range w.entityIDs() {
		w.entities[id].step(minutes)
	}

	w.pursuit.Advance(minutes)
	w.spawns.Advance(minutes)

	for _, pair := range w.notice.AwarePairs() {
		h, ok := pair.Watcher.(Hunter)
		if !ok || w.pursuit.Chasing(h.HunterID()) {
			continue
		}

		w.pursuit.Chase(h, pair.Target)
	}
}

// kill is Combat.withdraw and the rising after it: the member stops being
// watched and chased, and his remains are taken off the map.
func (w *b2bWorld) kill(id string) {
	w.notice.Unwatch(id)
	w.pursuit.Release(id)
	delete(w.entities, id)
}

// observe is everything a script or the digest can see of the three systems
// and the entities they drive, as one exact string. The player's id is
// written as "player": it is new every launch, and nothing may depend on it.
func (w *b2bWorld) observe(t *testing.T) string {
	t.Helper()

	type ent struct {
		X, Y  float64
		Route [][2]float64
		Speed float64
	}

	ents := make(map[string]ent, len(w.entities))
	for id, e := range w.entities {
		ents[id] = ent{e.x, e.y, e.route, e.speed}
	}

	b, err := json.Marshal(map[string]interface{}{
		"spawns":   w.spawns.HarnessState(),
		"pursuit":  w.pursuit.HarnessState(),
		"entities": ents,
		"player":   [2]float64{w.player.x, w.player.y},
		"clock":    w.clock.WorldMinutes(),
	})
	require.NoError(t, err)

	return strings.ReplaceAll(string(b), strconv.Quote(w.player.id), strconv.Quote(PlayerRef))
}

// b2bResolver is the game's resolver, over one fake world.
type b2bResolver struct{ w *b2bWorld }

func (r b2bResolver) Watcher(id string) (Watcher, bool) {
	e, ok := r.w.entities[id]
	if !ok {
		return nil, false
	}

	return e, true
}

func (r b2bResolver) Quarry(ref string) (Quarry, bool) {
	if ref == PlayerRef {
		return r.w.player, true
	}

	e, ok := r.w.entities[ref]
	if !ok {
		return nil, false
	}

	return b2bEntityQuarry{e}, true
}

// b2bSnap is the three snapshots, taken together at one moment.
type b2bSnap struct {
	Spawns  SpawnsSnapshot  `json:"spawns"`
	Notice  NoticeSnapshot  `json:"notice"`
	Pursuit PursuitSnapshot `json:"pursuit"`
}

func (w *b2bWorld) snapshot(t *testing.T) b2bSnap {
	t.Helper()

	r := b2bResolver{w}

	sp, err := w.spawns.Snapshot(r)
	require.NoError(t, err)

	no, err := w.notice.Snapshot(r)
	require.NoError(t, err)

	pu, err := w.pursuit.Snapshot(r)
	require.NoError(t, err)

	return b2bSnap{sp, no, pu}
}

// b2bThroughJSON writes a snapshot out and reads it back, as the world file
// will: a field that does not survive JSON does not survive a save.
func b2bThroughJSON(t *testing.T, s b2bSnap) b2bSnap {
	t.Helper()

	b, err := json.Marshal(s)
	require.NoError(t, err)

	var out b2bSnap
	require.NoError(t, json.Unmarshal(b, &out))

	return out
}

// b2bSaved is one saved moment: the three snapshots, read back through JSON,
// and what the rest of the save carries beside them -- the clock, the player,
// the uuid stream and every entity's id and motion -- as the fake world has
// them.
type b2bSaved struct {
	seed     int64 // the saved game's seed, which every Restore checks its stream against
	snap     b2bSnap
	ticks    int
	px, py   float64
	calls    int
	made     int
	entities map[string]*b2bEntity
}

func (w *b2bWorld) save(t *testing.T) b2bSaved {
	t.Helper()

	sv := b2bSaved{
		seed:     w.seed,
		snap:     b2bThroughJSON(t, w.snapshot(t)),
		ticks:    w.ticks,
		px:       w.player.x,
		py:       w.player.y,
		calls:    w.spawner.calls,
		made:     w.spawner.made,
		entities: make(map[string]*b2bEntity, len(w.entities)),
	}

	for id, e := range w.entities {
		sv.entities[id] = e.clone()
	}

	return sv
}

// b2bResume builds a fresh world as a relaunch would -- another seed, another
// player id -- rebuilds the saved entities with their ids and motion, and
// restores the three snapshots into it through a Resolver over the new world.
func b2bResume(t *testing.T, sv b2bSaved, s b2bSnap) (*b2bWorld, error) {
	t.Helper()

	return b2bResumeSkipping(t, sv, s, "")
}

// b2bResumeSkipping is b2bResume with one system ("spawns", "notice" or
// "pursuit") left as a relaunch builds it -- empty -- so a test can restore
// into it by hand: every Restore refuses a system that already holds state
// (the B2b review's B4).
func b2bResumeSkipping(t *testing.T, sv b2bSaved, s b2bSnap, skip string) (*b2bWorld, error) {
	t.Helper()

	b := b2bNewWorld(t, 7, "p:resumed")

	// The clock is B2a's to restore; here it is rebuilt the long way, by the
	// same ticks, which reaches the same float.
	for i := 0; i < sv.ticks; i++ {
		b.clock.Advance(b2bTickSeconds)
	}

	b.ticks = sv.ticks
	b.player.x, b.player.y = sv.px, sv.py
	b.spawner.calls, b.spawner.made = sv.calls, sv.made

	for id, e := range sv.entities {
		b.entities[id] = e.clone()
	}

	r := b2bResolver{b}

	if skip != "spawns" {
		if err := b.spawns.Restore(s.Spawns, r, sv.seed); err != nil {
			return nil, err
		}
	}

	if skip != "notice" {
		if err := b.notice.Restore(s.Notice, r); err != nil {
			return nil, err
		}
	}

	if skip != "pursuit" {
		if err := b.pursuit.Restore(s.Pursuit, r); err != nil {
			return nil, err
		}
	}

	return b, nil
}

// b2bRecord is what a world shows now and after each of n ticks.
func b2bRecord(t *testing.T, w *b2bWorld, n int) []string {
	t.Helper()

	out := make([]string, 0, n+1)
	out = append(out, w.observe(t))

	for i := 0; i < n; i++ {
		w.tick()
		out = append(out, w.observe(t))
	}

	return out
}

// b2bFirstDifference is the first tick at which two recordings differ, or -1.
func b2bFirstDifference(a, b []string) int {
	for i := range a {
		if i >= len(b) || a[i] != b[i] {
			return i
		}
	}

	return -1
}

// b2bAdvanceTicks is how far every resumed world is run beside the original.
const b2bAdvanceTicks = 400

// b2bExpect is what losing one saved field must do.
type b2bExpect int

const (
	// b2bDiverge: it restores, and the world shows the difference at the
	// moment of the load or within b2bAdvanceTicks.
	b2bDiverge b2bExpect = iota
	// b2bRefuse: Restore refuses it, and the world is not resumed.
	b2bRefuse
	// b2bCarried: it restores and nothing observable reads it. Each one is
	// named with its reason, and pinned: the day something reads it, this
	// goes red and the reason has to be rewritten.
	b2bCarried
)

// b2bMutation is one saved field changed to another value it could hold.
type b2bMutation struct {
	name   string
	expect b2bExpect
	mutate func(t *testing.T, s *b2bSnap)
}

// b2bSweep resumes the saved moment once per mutation and holds each to its
// expectation against want, the unmutated world's recording.
//
// It returns what each mutation exercised, for the classification's check
// that every saved path is seen (b2aExercised, the B2 review's B3/B5): the
// paths at which the mutated snapshot's JSON differs from the saved one's, and
// whether it was seen (refused or diverged; a carried one is not).
func b2bSweep(t *testing.T, sv b2bSaved, want []string, muts []b2bMutation) []b2aOutcome {
	t.Helper()

	var outcomes []b2aOutcome

	base := b2aTree(t, sv.snap)

	for _, m := range muts {
		s := b2bThroughJSON(t, sv.snap)
		m.mutate(t, &s)

		var paths []string
		b2aTreeDiff(base, b2aTree(t, s), "", &paths)
		require.NotEmpty(t, paths, "%s: the mutation changed nothing in the snapshot", m.name)

		for _, p := range paths {
			outcomes = append(outcomes, b2aOutcome{path: p, seen: m.expect != b2bCarried})
		}

		w, err := b2bResume(t, sv, s)

		switch m.expect {
		case b2bRefuse:
			if err == nil {
				t.Errorf("%s: restored; it must be refused", m.name)

				continue
			}

			t.Logf("%-40s refused: %v", m.name, err)

		case b2bDiverge, b2bCarried:
			if err != nil {
				t.Errorf("%s: refused (%v); it must restore", m.name, err)

				continue
			}

			at := b2bFirstDifference(want, b2bRecord(t, w, len(want)-1))

			switch {
			case m.expect == b2bDiverge && at < 0:
				t.Errorf("%s: LOST WITHOUT A TRACE -- %d ticks and the world never differed. A hole in observability, not a pass",
					m.name, len(want)-1)
			case m.expect == b2bDiverge:
				t.Logf("%-40s diverged at tick %d", m.name, at)
			case at >= 0:
				t.Errorf("%s: pinned as carried-and-unread, but the world differed at tick %d: something reads it now", m.name, at)
			default:
				t.Logf("%-40s carried, never read (pinned)", m.name)
			}
		}
	}

	return outcomes
}

// b2bFull is a world in which every saved field has something to say: two
// kinds of group, a dead member off the map, a despawn with chases released,
// a pack sent home at daybreak, a watch that sees, one that remembers and one
// that has not noticed, and chases both arrived and still walking. It returns
// what is still missing, so a failure says which.
func b2bFull(w *b2bWorld) []string {
	var missing []string

	need := func(ok bool, what string) {
		if !ok {
			missing = append(missing, what)
		}
	}

	s := w.spawns
	need(len(s.groups) >= 2, "two groups")
	need(s.failures > 0 && s.dropped > 0, "a failure and a dropped member")
	need(s.despawned > 0 && s.released > 0 && s.cleared > 0, "a despawn, a release and a daybreak clear")
	need(s.sinceCk > 0 && s.openBodies > 0, "since_check and open bodies")

	// In the dark, so the run after the save rolls the tables.
	stage := w.clock.Stage()
	need(stage == StageDusk || stage == StageNight, "the dark")

	gone := false

	for _, g := range s.groups {
		for _, m := range g.members {
			if _, on := w.entities[m.WatcherID()]; !on {
				gone = true
			}
		}
	}

	need(gone, "a member off the map")

	var sees, remembers, unaware, waiting bool

	for _, wt := range w.notice.watches {
		sees = sees || (wt.noticed && wt.sees)
		remembers = remembers || (wt.noticed && !wt.sees && wt.sinceSeen > 0)
		unaware = unaware || !wt.noticed
		waiting = waiting || (wt.noticed && wt.sees && wt.sinceCheck > 0)
	}

	need(sees && remembers && unaware, "watches that see, remember and are unaware")
	need(waiting, "a seeing watch part-way to its next look")

	var arrived, walking bool

	for _, c := range w.pursuit.chases {
		arrived = arrived || c.arrived
		walking = walking || (!c.arrived && c.sinceSolve > 0)
	}

	need(arrived && walking, "chases arrived and walking")

	return missing
}

// b2bFilledWorld runs a world from the epoch's dawn, through the day and into
// the dusk after, scripting the two events the tables cannot make on their
// own -- a death whose remains leave the map, and a despawn -- and stops at
// the first tick at which b2bFull is satisfied.
func b2bFilledWorld(t *testing.T) *b2bWorld {
	t.Helper()

	w := b2bNewWorld(t, 1462, "p:0")
	require.True(t, w.spawns.SetOpenBodies(2))

	killedIn, despawned := "", false

	for i := 0; i < 20000; i++ {
		w.tick()

		// After the first daybreak (which sends home every pack that has
		// forgotten him), in the dark, once a chasing pack has two members:
		// one dies and his remains go.
		dark := w.clock.Stage() == StageDusk || w.clock.Stage() == StageNight
		if killedIn == "" && w.spawns.cleared > 0 && dark {
			for _, id := range w.spawns.groupIDs() {
				g := w.spawns.groups[id]
				if len(g.members) >= 2 && w.pursuit.Chasing(g.members[0].WatcherID()) {
					w.kill(g.members[0].WatcherID())
					killedIn = id

					break
				}
			}
		}

		// After that, the harness sends home ANOTHER pack that is chasing, so
		// the despawn releases chases.
		if killedIn != "" && !despawned {
			for _, id := range w.spawns.groupIDs() {
				if id != killedIn && w.b2bChasing(id) {
					require.True(t, w.spawns.Despawn(id))
					despawned = true

					break
				}
			}
		}

		if killedIn != "" && despawned && len(b2bFull(w)) == 0 {
			t.Logf("filled at tick %d, %.1f world minutes, stage %v", w.ticks, w.clock.WorldMinutes(), w.clock.Stage())

			return w
		}
	}

	t.Fatalf("the fake world never filled; missing: %v", b2bFull(w))

	return nil
}

// b2bChasing reports whether any member of a group has a chase running.
func (w *b2bWorld) b2bChasing(groupID string) bool {
	for _, m := range w.spawns.groups[groupID].members {
		if w.pursuit.Chasing(m.WatcherID()) {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------- tests ----

// The Resolver's word for the player, and its strictness: an id that names
// nothing, or names the wrong thing, is ErrUnresolvedRef -- never a nil
// handed back as if it were fine.
func TestResolverIsStrict(t *testing.T) {
	w := b2bNewWorld(t, 1, "p:live")
	e := &b2bEntity{id: "e:1", x: 3, y: 4}
	w.entities[e.id] = e
	r := b2bResolver{w}

	got, err := b2bResolveWatcher(r, "e:1")
	require.NoError(t, err)
	require.Same(t, e, got)

	for _, bad := range []string{"e:404", "", PlayerRef} {
		_, err := b2bResolveWatcher(r, bad)
		require.True(t, errors.Is(err, ErrUnresolvedRef), "watcher %q: %v", bad, err)
	}

	_, err = b2bResolveWatcher(nil, "e:1")
	require.True(t, errors.Is(err, ErrUnresolvedRef), "no resolver: %v", err)

	q, err := b2bResolveQuarry(r, PlayerRef)
	require.NoError(t, err)
	require.Equal(t, "p:live", q.QuarryID())

	q, err = b2bResolveQuarry(r, "e:1")
	require.NoError(t, err)
	require.Equal(t, "e:1", q.QuarryID())

	for _, bad := range []string{"e:404", "", "p:live"} {
		_, err := b2bResolveQuarry(r, bad)
		require.True(t, errors.Is(err, ErrUnresolvedRef), "quarry %q: %v", bad, err)
	}

	// The player is written as the word, anything else as its id.
	require.Equal(t, PlayerRef, b2bQuarryRef(w.player, "p:live"))
	require.Equal(t, "e:1", b2bQuarryRef(b2bEntityQuarry{e}, "p:live"))
	require.Equal(t, "p:live", b2bQuarryRef(w.player, ""), "no player named, no word")
}

// A resolver that hands back the wrong entity is caught: it must answer to
// the id it was asked for.
type b2bLyingResolver struct{ b2bResolver }

func (r b2bLyingResolver) Watcher(string) (Watcher, bool) { return &b2bEntity{id: "e:other"}, true }

func (r b2bLyingResolver) Quarry(ref string) (Quarry, bool) {
	if ref == PlayerRef {
		return r.w.player, true
	}

	return b2bEntityQuarry{&b2bEntity{id: "e:other"}}, true
}

func TestResolverThatLiesIsCaught(t *testing.T) {
	w := b2bNewWorld(t, 1, "p:live")
	r := b2bLyingResolver{b2bResolver{w}}

	_, err := b2bResolveWatcher(r, "e:1")
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)

	_, err = b2bResolveQuarry(r, "e:1")
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)
}

// The fake world itself: it must fill, or every test built on it proves less
// than it says.
func TestB2bWorldFills(t *testing.T) {
	w := b2bFilledWorld(t)
	require.Empty(t, b2bFull(w))
}
