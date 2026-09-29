package d2world

// THE CLOCK BLOCK, THE FIVE WAYS (the raid's R1 assertion 6, Q5 default (a):
// "a fight he is not in never stops a save; it is saved with the world").
// B2a's five -- round trip, run on, the sweep, classification, the probes --
// over a world where the village is fighting: a clock fight live at the save,
// with a dead enemy, a routed one, one broken off at first light and one that
// joined late, and a second clock fight ended with its quarry dead. The
// existing combat snapshot tests cover his half and are unedited; the only
// rows they gained are TestCombatSnapshotFieldsClassified's for the new
// fields.

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// clockWorld is a resolver fight with quarries that are not him, the corpse
// registry they fall into, and a Resolver over both.
type clockWorld struct {
	*resolverFight
	quarries map[string]*fakeQuarry
	corpses  *Corpses
}

// clockResolver is the game's worldResolver over the fake world: an entity id
// is the watcher or quarry of that id, and PlayerRef is he.
type clockResolver struct{ w *clockWorld }

func (r clockResolver) Watcher(id string) (Watcher, bool) {
	w, ok := r.w.watchers[id]
	if !ok || w == nil {
		return nil, false
	}

	return w, true
}

func (r clockResolver) Quarry(ref string) (Quarry, bool) {
	if ref == PlayerRef {
		return r.w.target, true
	}

	q, ok := r.w.quarries[ref]
	if !ok || q == nil {
		return nil, false
	}

	return q, true
}

func (w *clockWorld) quarry(id string, x, y float64, hp int) *fakeQuarry {
	q := &fakeQuarry{id: id, x: x, y: y}
	w.quarries[id] = q
	w.bodies.known[id] = &fakeBody{health: hp, maxHealth: hp}

	return q
}

func (w *clockWorld) monster(t *testing.T, id string, x, y float64, hp int, p Profile, q *fakeQuarry) {
	t.Helper()

	m := &fakeWatcher{id: id, x: x, y: y}
	w.watchers[id] = m
	w.bodies.known[id] = &fakeBody{health: hp, maxHealth: hp}
	w.profiles.byID[id] = p
	w.chases.chasing[id] = true

	aware(t, w.notice, m, q)
}

// clockFixture is b2aCombatFixture's world -- his fight fought to its end,
// every record it leaves -- with him bound and the village fighting:
//   - c:1, live: v:1 against a dog pack (d:1 killed by v:1's first blow, so
//     d:2 routs), a wolf m:3, a risen r:4 broken off at first light, and a
//     wolf m:7 that joined late; stopped half a round into its current round;
//   - c:2, ended: v:5 cut down by m:6, its watch released.
func clockFixture(t *testing.T) *clockWorld {
	t.Helper()

	w := &clockWorld{resolverFight: b2aCombatFixture(t), quarries: map[string]*fakeQuarry{}, corpses: NewCorpses(nil, nil, nil)}
	w.c.SetPlayer("p:1")
	w.c.SetResolver(clockResolver{w})
	w.c.SetCorpses(w.corpses)

	v1 := w.quarry("v:1", 80, 40, 5000)
	w.morale.morale["g:d"] = 50
	w.monster(t, "d:1", 81, 40, 1, Profile{Row: "dogs", Group: "g:d", Speed: 3, Count: 2, DamageMin: 1, DamageMax: 2}, v1)
	w.monster(t, "d:2", 79, 40, 40, Profile{Row: "dogs", Group: "g:d", Speed: 3, Count: 2, DamageMin: 1, DamageMax: 2}, v1)
	w.monster(t, "m:3", 80, 41, 5000, Profile{Row: "wolves", Group: "g:3", Speed: 1, DamageMin: 1, DamageMax: 3}, v1)
	w.monster(t, "r:4", 80, 39, 5000, Profile{Row: RisenRow, Dead: true}, v1)

	w.c.Advance(0.25)
	require.Len(t, w.c.clockFights, 1, "c:1 opens")

	w.c.Advance(1.0)
	e := w.c.clockFights[0]
	require.True(t, e.dead["d:1"] && e.routed["d:2"], "v:1's first blow killed d:1 and the pack broke: %v %v", e.dead, e.routed)

	w.monster(t, "m:7", 81, 41, 5000, Profile{Row: "wolves", Group: "g:7", Speed: 2, DamageMin: 1, DamageMax: 3}, v1)
	w.c.Advance(1.0)
	require.Equal(t, 1, w.c.clockBook.joined, "m:7 joined c:1")

	require.Equal(t, 1, w.c.BreakOff(func(id string) bool { return w.profiles.byID[id].Dead }), "first light takes r:4 off")

	v5 := w.quarry("v:5", 80, 60, 2)
	w.monster(t, "m:6", 81, 60, 5000, Profile{Row: "wolves", Group: "g:6", Speed: 2, DamageMin: 2, DamageMax: 4}, v5)

	for i := 0; i < 20 && w.c.clockBook.quarryDead == 0; i++ {
		w.c.Advance(1.0)
	}

	require.Equal(t, 1, w.c.clockBook.quarryDead, "v:5 died in c:2")
	w.c.Advance(0.5)

	require.Len(t, w.c.clockFights, 1)
	require.Equal(t, "c:1", w.c.clockFights[0].id)

	return w
}

// clockCopy is the same world with its combat model replaced by a FRESH one,
// on another game's streams, into which snap is restored as the saved game's.
// The watchers named in off are not in that world's map (B1: remains that
// left it).
func clockCopy(t *testing.T, snap CombatSnapshot, withResolver bool, off ...string) (*clockWorld, error) {
	t.Helper()

	w := clockFixture(t)
	w.c.Close()

	for _, id := range off {
		delete(w.watchers, id)
	}

	dials := DefaultCombatDials()
	dials.PlayerControl = PlayerControlHuman
	clock := NewClock(DefaultClockDials())
	t.Cleanup(clock.Close)

	w.c = NewCombat(clock, w.notice, w.fitness, w.illum, w.bodies,
		w.profiles, w.animator, w.morale, w.chases, d2rand.Derive(99, d2rand.StreamCombat), dials)
	t.Cleanup(w.c.Close)

	w.c.SetPlayer("p:1")
	w.c.SetCorpses(w.corpses)

	if withResolver {
		w.c.SetResolver(clockResolver{w})
	}

	return w, w.c.Restore(snap, b2aWorldSeed)
}

// clockSteps is the world run on after a save: the first frame read as a load
// shows it, eight world minutes of the fights, then a new quarry and its
// killer (c:3 opens on the clock's own sequence and stream), four more. Every
// frame reads the provider (his keys and the clock block), what the game takes,
// and what the fights did to the bodies, the packs, the chases, the watches
// and the dead.
func clockSteps(t *testing.T, w *clockWorld) string {
	t.Helper()

	var trace strings.Builder

	ids := make([]string, 0, len(w.bodies.known))
	for id := range w.bodies.known {
		ids = append(ids, id)
	}

	observe := func() {
		trace.WriteString(b2aJSON(t, w.c.HarnessState()))
		trace.WriteString(b2aJSON(t, w.c.Tactical()))
		trace.WriteString(b2aJSON(t, w.c.TakeXPEvents()))
		trace.WriteString(b2aJSON(t, w.c.TakeRoundMinutes()))
		trace.WriteString(b2aJSON(t, []interface{}{w.animator.acts, w.morale.hurts, w.chases.released, w.notice.Aware()}))
		trace.WriteString(b2aJSON(t, w.corpses.HarnessState()))

		sort.Strings(ids)

		for _, id := range ids {
			if b := w.bodies.known[id]; b != nil {
				trace.WriteString(b2aJSON(t, []interface{}{id, b.health}))
			}
		}
	}

	observe()

	for i := 0; i < 8; i++ {
		w.c.Advance(1.0)
		observe()
	}

	v8 := w.quarry("v:8", 80, 80, 3)
	w.monster(t, "m:9", 81, 80, 5000, Profile{Row: "wolves", Group: "g:9", Speed: 2, DamageMin: 2, DamageMax: 4}, v8)
	ids = append(ids, "v:8", "m:9")

	for i := 0; i < 4; i++ {
		w.c.Advance(1.0)
		observe()
	}

	return trace.String()
}

// clockOnly is the clock block alone, at the path it has in the combat
// snapshot, for the sweep: his half is TestCombatSnapshotEveryFieldIsLoadBearing's.
type clockOnly struct {
	Clock CombatClockSnapshot `json:"clock"`
}

// clockClasses is every field of the clock book and of a clock fight, labelled.
func clockClasses() []b2aClass {
	const never = "T: a clock fight never paces, never waits and never takes a turn; Snapshot refuses a clock fight holding it"

	const unread = "D: the clock's copy of a record only his fight's readers read (the ROUND and PACE lines, the HUD log); " +
		"nothing reads the clock's, so a load starts it empty"

	const stale = "D: a per-round cap keyed by the round that spent it; between two steps a clock fight has moved past " +
		"that round, so a load restores 0 and reads the same (Snapshot refuses one keyed by the running round)"

	return []b2aClass{
		{fightBook{}, map[string]string{
			"encounter":      "T: the clock fight on the struct for one step (withClock); Snapshot refuses unless nil",
			"rng":            "S:clock.rng",
			"dials":          "W: recomputed from his dials at every clock step (clockDials)",
			"nextID":         "S:clock.next_id",
			"xpEvents":       "T: earn is suppressed in a clock fight; Snapshot refuses unless empty",
			"killerIsPlayer": "T: set and cleared inside one blow's resolution; Snapshot refuses unless clear",
			"owedMinutes":    "T: a clock fight is never paced and owes no minutes; Snapshot refuses unless zero",
			"blowLog":        unread,
			"stepsOrdered":   "T: a clock fight never paces, so orders no walk; Snapshot refuses unless zero",
			"lastActions":    "S:clock.last_actions",
			"actionsRound":   "S:clock.actions_round",
			"started":        "S:clock.started",
			"ended":          "S:clock.ended",
			"rounds":         "S:clock.rounds",

			"decisionSeconds":      "T: a clock fight never opens a turn; Snapshot refuses unless zero",
			"decisionSecondsRound": "T: a clock fight never opens a turn; Snapshot refuses unless zero",
			"paceOpen":             "T: a clock fight never opens the pace window; Snapshot refuses unless shut",
			"wallSeconds":          "T: a clock fight never opens the pace window; Snapshot refuses unless zero",
			"paceHealthOpen":       "T: a clock fight never opens the pace window; Snapshot refuses unless zero",
			"paceCommits":          "T: a clock fight never opens the pace window; Snapshot refuses unless zero",
			"lastActionVerb":       "T: only his commit sets a verb; Snapshot refuses a clock book holding one",

			"lastRound":          unread,
			"lastPace":           unread,
			"declines":           "S:clock.declines",
			"actions":            "S:clock.actions",
			"endedReason":        "S:clock.ended_reason",
			"endedEnemiesDead":   "S:clock.ended_enemies_dead",
			"endedDawn":          "S:clock.ended_dawn",
			"endedPlayerDead":    "S:clock.ended_player_dead",
			"endedDisengaged":    "S:clock.ended_disengaged",
			"endedRouted":        "S:clock.ended_routed",
			"joined":             "S:clock.joined",
			"quickResolved":      "S:clock.quick_resolved",
			"lastQuickAdvantage": "S:clock.last_quick_advantage",

			"stepping":       "T: true only inside a clock step (withClock); Snapshot refuses while set",
			"quarryDead":     "S:clock.ended_quarry_dead",
			"xpSuppressed":   "S:clock.xp_suppressed",
			"released":       "S:clock.released",
			"reentrantReads": "S:clock.reentrant_reads",
		}},
		{encounter{}, map[string]string{
			"id":          "S:clock.live[0].id",
			"target":      "S:clock.live[0].quarry",
			"enemies":     "S:clock.live[0].enemies",
			"driver":      "D: every saved clock fight is clock-driven; restored as driverClock",
			"enemyOrder":  "S:clock.live[0].enemy_order",
			"dead":        "S:clock.live[0].dead",
			"routed":      "S:clock.live[0].routed",
			"broke":       "S:clock.live[0].broke",
			"initiator":   "S:clock.live[0].initiator",
			"surprised":   "S:clock.live[0].surprised",
			"surpriseWhy": "S:clock.live[0].surprise_why",
			"round":       "S:clock.live[0].round",
			"sinceTurn":   "S:clock.live[0].since_turn",

			"reactionUsedInRound": stale,
			"blockUsedInRound":    stale,
			"reactionsInRound":    stale,
			"blocksInRound":       stale,

			"sequence":    "T: the round's frozen walk; walkSequence empties it before a clock step returns; Snapshot refuses unless empty",
			"cursor":      "T: the frozen walk's place; zero once the walk is done; Snapshot refuses unless zero",
			"awaiting":    never,
			"moveSpent":   never,
			"actionSpent": never,
			"roundOpen":   never,
			"playerWait":  never,
			"acting":      never,
			"stepping":    never,
			"strikers":    never,
			"beat":        never,
			"stepWait":    never,
		}},
	}
}

func TestCombatClockSnapshotFieldsClassified(t *testing.T) {
	w := clockFixture(t)

	snap, err := w.c.Snapshot()
	require.NoError(t, err)

	for _, c := range clockClasses() {
		b2aClassified(t, c.kind, snap, c.fields)
	}
}

func TestCombatClockSnapshotRefusesEveryTransient(t *testing.T) {
	classes := clockClasses()

	b2aTransientsRefused(t, classes[0], func() (interface{}, func() error) {
		w := clockFixture(t)

		return w.c.clockBook, func() error { _, err := w.c.Snapshot(); return err }
	})

	b2aTransientsRefused(t, classes[1], func() (interface{}, func() error) {
		w := clockFixture(t)

		return w.c.clockFights[0], func() error { _, err := w.c.Snapshot(); return err }
	})
}

// A FIGHT HE IS NOT IN NEVER STOPS A SAVE (Q5 (a)), and it is in the save:
// round trip through JSON, into a fresh model on another game's streams, and
// the copy fights on as the original does.
func TestCombatClockSnapshotRoundTrip(t *testing.T) {
	orig := clockFixture(t)

	s, err := orig.c.Snapshot()
	require.NoError(t, err, "the village is fighting and the save is not refused")

	snap := b2aThroughJSON(t, s)
	require.Equal(t, s, snap, "the snapshot survives JSON exactly")
	require.Len(t, snap.Clock.Live, 1, "the live clock fight is in it")
	require.Equal(t, "c:1", snap.Clock.Live[0].ID)
	require.Equal(t, []string{"d:1"}, snap.Clock.Live[0].Dead)
	require.Equal(t, []string{"d:2"}, snap.Clock.Live[0].Routed)
	require.Equal(t, []string{"r:4"}, snap.Clock.Live[0].Broke)
	require.NotEmpty(t, snap.Clock.LastActions)
	require.Positive(t, snap.Clock.RNG.Draws, "the clock fights drew")
	require.Equal(t, 3, snap.Clock.NextID)

	// Observability first: what the save carries, the provider reports.
	live := clockBlockOf(orig.c)["live"].([]map[string]interface{})
	require.Equal(t, snap.Clock.Live[0].ID, live[0]["id"])
	require.Equal(t, snap.Clock.Live[0].Round, live[0]["round"])

	cp, err := clockCopy(t, snap, true)
	require.NoError(t, err)

	again, err := cp.c.Snapshot()
	require.NoError(t, err)
	require.Equal(t, snap, again, "restored is the same model, the clock's dice included")
	require.Equal(t, clockSteps(t, orig), clockSteps(t, cp), "and it fights on the same")
}

func clockBlockOf(c *Combat) map[string]interface{} {
	return c.HarnessState()["clock"].(map[string]interface{})
}

// His fight still refuses a save while the village's does not.
func TestOnlyHisFightStopsASave(t *testing.T) {
	w := clockFixture(t)

	_, err := w.c.Snapshot()
	require.NoError(t, err)

	b2aCombatAdd(t, w.resolverFight, "e:near", 41, 40, 50, Profile{Row: "wolves", Group: "g:near", Speed: 2, DamageMin: 2, DamageMax: 4})
	w.c.Advance(0.25)
	require.True(t, w.c.Fighting(), "his fight opens")

	_, err = w.c.Snapshot()
	require.True(t, errors.Is(err, ErrCombatFighting), "his fight refuses the save: %v", err)
}

func TestCombatClockSnapshotEveryFieldIsLoadBearing(t *testing.T) {
	orig := clockFixture(t)

	snap, err := orig.c.Snapshot()
	require.NoError(t, err)

	ref := clockSteps(t, orig)

	try := func(raw []byte) (string, error) {
		var v clockOnly
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", err
		}

		s := snap
		s.Clock = v.Clock

		cp, err := clockCopy(t, s, true)
		if err != nil {
			return "", err
		}

		return clockSteps(t, cp), nil
	}

	b2aExercised(t, b2aSweep(t, clockOnly{snap.Clock}, ref, nil, try), clockClasses()...)

	// The probes: valid, and not the saved world.
	tryWhole := func(raw []byte) (string, error) {
		var s CombatSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		cp, err := clockCopy(t, s, true)
		if err != nil {
			return "", err
		}

		return clockSteps(t, cp), nil
	}

	b2aMustDiverge(t, snap, ref, map[string]func(s *CombatSnapshot){
		"the clock's ids and counts one fight on": func(s *CombatSnapshot) {
			s.Clock.NextID, s.Clock.Started, s.Clock.Ended = s.Clock.NextID+1, s.Clock.Started+1, s.Clock.Ended+1
		},
		"the live fight under the ended one's id": func(s *CombatSnapshot) { s.Clock.Live[0].ID = "c:2" },
		"the live fight back at round one":        func(s *CombatSnapshot) { s.Clock.Live[0].Round = 1 },
	}, tryWhole)
}

// Every rule the clock block is held to refuses a snapshot that breaks it, and
// a refused restore changes nothing.
func TestCombatClockSnapshotRefusals(t *testing.T) {
	good, err := clockFixture(t).c.Snapshot()
	require.NoError(t, err)

	_, err = clockCopy(t, good, true)
	require.NoError(t, err, "the control: the good snapshot restores")

	cases := map[string]func(s *CombatSnapshot){
		"clock.next_id not one past started":   func(s *CombatSnapshot) { s.Clock.NextID++ },
		"a clock fight neither ended nor live": func(s *CombatSnapshot) { s.Clock.Ended-- },
		"a negative clock count":               func(s *CombatSnapshot) { s.Clock.Joined = -1 },
		"a quick advantage that is no number":  func(s *CombatSnapshot) { s.Clock.LastQuickAdvantage = math.NaN() },
		"another game's clock stream": func(s *CombatSnapshot) {
			s.Clock.RNG.Seed = d2rand.Derive(7, d2rand.StreamCombatClock)
		},
		"his stream in the clock's place": func(s *CombatSnapshot) { s.Clock.RNG = s.RNG },
		"a live id of his sequence":       func(s *CombatSnapshot) { s.Clock.Live[0].ID = "e:1" },
		"a live id past the sequence":     func(s *CombatSnapshot) { s.Clock.Live[0].ID = "c:9" },
		"two fights over one quarry": func(s *CombatSnapshot) {
			twin := s.Clock.Live[0]
			twin.ID = "c:2"
			s.Clock.Live = append(s.Clock.Live, twin)
			s.Clock.Ended--
		},
		"a clock fight after him":          func(s *CombatSnapshot) { s.Clock.Live[0].Quarry = PlayerRef },
		"a quarry nobody is":               func(s *CombatSnapshot) { s.Clock.Live[0].Quarry = "v:404" },
		"an enemy nobody is":               func(s *CombatSnapshot) { s.Clock.Live[0].Enemies = append(s.Clock.Live[0].Enemies, "m:404") },
		"an enemy twice":                   func(s *CombatSnapshot) { s.Clock.Live[0].Enemies = append(s.Clock.Live[0].Enemies, "m:3") },
		"the quarry among its enemies":     func(s *CombatSnapshot) { s.Clock.Live[0].Enemies = append(s.Clock.Live[0].Enemies, "v:1") },
		"no enemies":                       func(s *CombatSnapshot) { s.Clock.Live[0].Enemies, s.Clock.Live[0].EnemyOrder = nil, nil },
		"a dead id twice":                  func(s *CombatSnapshot) { s.Clock.Live[0].Dead = append(s.Clock.Live[0].Dead, "d:1") },
		"the quarry routed from his fight": func(s *CombatSnapshot) { s.Clock.Live[0].Routed = append(s.Clock.Live[0].Routed, "v:1") },
		"an order naming him":              func(s *CombatSnapshot) { s.Clock.Live[0].EnemyOrder = append(s.Clock.Live[0].EnemyOrder, PlayerRef) },
		"round zero":                       func(s *CombatSnapshot) { s.Clock.Live[0].Round = 0 },
		"minutes before the round began":   func(s *CombatSnapshot) { s.Clock.Live[0].SinceTurn = -0.5 },
		"minutes that are no number":       func(s *CombatSnapshot) { s.Clock.Live[0].SinceTurn = math.Inf(1) },
		"a clock fight he started":         func(s *CombatSnapshot) { s.Clock.Live[0].Initiator = "player" },
		"a surprise's reason, no surprise": func(s *CombatSnapshot) { s.Clock.Live[0].SurpriseWhy = "caught-foraging" },
	}

	for name, bad := range cases {
		s := b2aThroughJSON(t, good)
		bad(&s)

		cp, err := clockCopy(t, s, true)
		require.Error(t, err, name)
		require.Equal(t, 1, cp.c.clockBook.nextID, "%s: a refused restore changes nothing", name)
		require.Empty(t, cp.c.clockFights, "%s: a refused restore changes nothing", name)
	}

	// No Resolver to find the live fight's quarry and enemies: refused, not
	// half restored.
	cp, err := clockCopy(t, good, false)
	require.ErrorIs(t, err, ErrUnresolvedRef)
	require.Empty(t, cp.c.clockFights)

	// And Snapshot refuses what a clock fight must never hold at a save.
	for name, dirty := range map[string]func(w *clockWorld){
		"a clock fight after him":                 func(w *clockWorld) { w.c.clockFights[0].target = w.target },
		"a clock fight not clock-driven":          func(w *clockWorld) { w.c.clockFights[0].driver = driverPlayer },
		"a reaction spent in the running round":   func(w *clockWorld) { w.c.clockFights[0].reactionUsedInRound = w.c.clockFights[0].round },
		"a block spent in the running round":      func(w *clockWorld) { w.c.clockFights[0].blockUsedInRound = w.c.clockFights[0].round },
		"experience the clock's book never takes": func(w *clockWorld) { w.c.clockBook.xpEvents = []XPEvent{{Kind: "slain"}} },
	} {
		w := clockFixture(t)
		dirty(w)

		_, err := w.c.Snapshot()
		require.Error(t, err, name)
		require.False(t, errors.Is(err, ErrCombatFighting), "%s is not his fight", name)
	}
}

// --- B1 (the raid R1 review; BUG-68): a dead enemy off the map --------------

// P4, the review's probe, as a test: c:1 keeps d:1's row (v:1 killed it) for
// the fight's life, and d:1's remains leave the map -- his body rises and the
// game takes what lay there off it, or his pack is sent home at daybreak.
// The save validates the live model's own snapshot (validateSnapshots ->
// Validate), and a fight he is not in must not stop it (Q5 (a)); a LIVING
// enemy off the map still does, because that is a world the save cannot
// describe.
func TestADeadEnemyOffTheMapStopsNoSave(t *testing.T) {
	w := clockFixture(t)

	snap, err := w.c.Snapshot()
	require.NoError(t, err)
	require.NoError(t, w.c.Validate(snap, b2aWorldSeed), "the control: every entity on the map")
	require.Equal(t, []string{"d:1"}, snap.Clock.Live[0].Dead)
	require.Contains(t, snap.Clock.Live[0].Enemies, "d:1", "c:1 keeps the dead d:1's row")

	delete(w.watchers, "d:1")

	snap, err = w.c.Snapshot()
	require.NoError(t, err)
	require.NoError(t, w.c.Validate(snap, b2aWorldSeed), "a dead enemy off the map stops no save")

	w.c.Advance(1.0)
	w.c.Advance(1.0)

	snap, err = w.c.Snapshot()
	require.NoError(t, err)
	require.Len(t, snap.Clock.Live, 1, "c:1 fights on")
	require.Contains(t, snap.Clock.Live[0].Enemies, "d:1", "its row with it")
	require.NoError(t, w.c.Validate(snap, b2aWorldSeed), "two minutes on, still no refusal")

	// A routed enemy off the map (his pack sent home) stops no save either.
	m3 := w.watchers["m:3"]
	delete(w.watchers, "d:2")
	require.Equal(t, []string{"d:2"}, snap.Clock.Live[0].Routed)
	require.NoError(t, w.c.Validate(snap, b2aWorldSeed), "a routed enemy off the map stops no save")

	// THE CONTROL beside it: a living enemy off the map is still refused.
	delete(w.watchers, "m:3")
	require.ErrorIs(t, w.c.Validate(snap, b2aWorldSeed), ErrUnresolvedRef, "a living enemy off the map is refused")
	w.watchers["m:3"] = m3
	require.NoError(t, w.c.Validate(snap, b2aWorldSeed))
}

// The sweep case: the same world with d:1 off the map, saved, restored into a
// fresh model whose map has no d:1 either -- its row an id-only one (goneRow)
// -- round trips, fights on exactly as the live model does, and every field of
// the clock block is still load-bearing (B2a's sweep over it: each mutation
// refused or diverging).
func TestCombatClockSnapshotWithADeadEnemyOffTheMap(t *testing.T) {
	orig := clockFixture(t)
	delete(orig.watchers, "d:1")

	snap, err := orig.c.Snapshot()
	require.NoError(t, err)

	snap = b2aThroughJSON(t, snap)
	ref := clockSteps(t, orig)

	cp, err := clockCopy(t, snap, true, "d:1")
	require.NoError(t, err, "a dead enemy off the map restores")

	again, err := cp.c.Snapshot()
	require.NoError(t, err)
	require.Equal(t, snap, again, "restored is the same model")

	var row Combatant

	for _, en := range cp.c.clockFights[0].enemies {
		if en.WatcherID() == "d:1" {
			row = en
		}
	}

	require.Equal(t, goneRow("d:1"), row, "d:1 comes back as its id alone")
	require.Equal(t, ref, clockSteps(t, cp), "and the copy fights on as the live model does")

	try := func(raw []byte) (string, error) {
		var v clockOnly
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", err
		}

		s := snap
		s.Clock = v.Clock

		cp, err := clockCopy(t, s, true, "d:1")
		if err != nil {
			return "", err
		}

		return clockSteps(t, cp), nil
	}

	b2aExercised(t, b2aSweep(t, clockOnly{snap.Clock}, ref, nil, try), clockClasses()...)
}
