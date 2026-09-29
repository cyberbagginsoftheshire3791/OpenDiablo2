package d2world

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// M4.6 B2b: the spawn tables' snapshot, and the three closing tests for all
// three entity-keyed systems together -- round trip, advance in step, and
// every field lost or altered is seen. The world they run in is in
// resolver_test.go.

// A snapshot restored into a RELAUNCH -- another seed, another player id --
// reads back exactly: the resumed world shows what the saved one showed, and
// snapshots to the same bytes.
func TestEntitySnapshotsRoundTripThroughARelaunch(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)

	require.NotEqual(t, a.player.id, b.player.id, "a relaunch has a new player id")
	require.Equal(t, a.observe(t), b.observe(t), "the resumed world must show what the saved one showed")

	want, err := json.Marshal(sv.snap)
	require.NoError(t, err)

	got, err := json.Marshal(b.snapshot(t))
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got), "saving the resumed world must write what was loaded")

	// The resumed tables draw from the saved stream, not the relaunch's seed.
	require.Equal(t, d2rand.Derive(sv.seed, d2rand.StreamSpawns), b.spawns.rng.Seeded())
	require.Equal(t, a.spawns.rng.Draws(), b.spawns.rng.Draws())
}

// The saved world and the resumed one, run side by side, stay identical frame
// by frame -- and the run is long enough that the hidden clocks and the
// stream all had to do something.
func TestEntitySnapshotsAdvanceInStep(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)

	checks, rolls, draws := a.spawns.checks, a.spawns.rolls, a.spawns.rng.Draws()
	sights, solves := a.notice.checks, a.pursuit.solves

	want := b2bRecord(t, a, b2bAdvanceTicks)
	got := b2bRecord(t, b, b2bAdvanceTicks)

	require.Equal(t, -1, b2bFirstDifference(want, got), "the resumed world left the saved one's path")

	// A world that stood still proves nothing.
	require.Greater(t, a.spawns.checks, checks, "table checks")
	require.Greater(t, a.spawns.rolls, rolls, "table rolls")
	require.Greater(t, a.spawns.rng.Draws(), draws, "the tables' stream drew")
	require.Greater(t, a.notice.checks, sights, "sight tests")
	require.Greater(t, a.pursuit.solves, solves, "route solves")
	require.NotEqual(t, want[0], want[len(want)-1])
}

// b2bGroupWith finds the first saved group with a member of the given kind
// (gone or on the map), and that member's index.
func b2bGroupWith(t *testing.T, s *b2bSnap, gone bool) (int, int) {
	t.Helper()

	for gi, g := range s.Spawns.Groups {
		for mi, m := range g.Members {
			if m.Gone == gone {
				return gi, mi
			}
		}
	}

	t.Fatalf("no saved member with gone=%v", gone)

	return 0, 0
}

// b2bOtherMember is a member id on the map from a DIFFERENT group than gi.
func b2bOtherMember(t *testing.T, s *b2bSnap, gi int) string {
	t.Helper()

	for i, g := range s.Spawns.Groups {
		for _, m := range g.Members {
			if i != gi && !m.Gone {
				return m.ID
			}
		}
	}

	t.Fatal("no member on the map in another group")

	return ""
}

// b2bFreeGroupID is a group id below next_id that no saved group uses.
func b2bFreeGroupID(t *testing.T, s *b2bSnap) string {
	t.Helper()

	used := map[string]bool{}
	for _, g := range s.Spawns.Groups {
		used[g.ID] = true
	}

	for n := 1; n < s.Spawns.NextID; n++ {
		if id := "g:" + itoa(n); !used[id] {
			return id
		}
	}

	t.Fatal("no free group id below next_id")

	return ""
}

// Every saved spawns field, lost or altered, is SEEN: it either changes what
// the resumed world shows (at the load or within the run), or Restore refuses
// it. A field that can go missing without a trace is a hole in observability,
// not a pass.
func TestSpawnsSnapshotEveryFieldIsSeen(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)
	want := b2bRecord(t, a, b2bAdvanceTicks)

	live := func(t *testing.T, s *b2bSnap) (*SpawnGroupSnapshot, *SpawnMemberSnapshot) {
		gi, mi := b2bGroupWith(t, s, false)

		return &s.Spawns.Groups[gi], &s.Spawns.Groups[gi].Members[mi]
	}

	gone := func(t *testing.T, s *b2bSnap) (*SpawnGroupSnapshot, *SpawnMemberSnapshot) {
		gi, mi := b2bGroupWith(t, s, true)

		return &s.Spawns.Groups[gi], &s.Spawns.Groups[gi].Members[mi]
	}

	top := func(name string, f func(sp *SpawnsSnapshot)) b2bMutation {
		return b2bMutation{"spawns." + name + " lost", b2bDiverge, func(_ *testing.T, s *b2bSnap) { f(&s.Spawns) }}
	}

	grp := func(name string, expect b2bExpect, f func(g *SpawnGroupSnapshot)) b2bMutation {
		return b2bMutation{"spawns.group." + name, expect, func(t *testing.T, s *b2bSnap) {
			g, _ := live(t, s)
			f(g)
		}}
	}

	outcomes := b2bSweep(t, sv, want, []b2bMutation{
		top("next_id", func(sp *SpawnsSnapshot) { sp.NextID++ }),
		{"spawns.next_id zero", b2bRefuse, func(_ *testing.T, s *b2bSnap) { s.Spawns.NextID = 0 }},
		top("since_check_minutes", func(sp *SpawnsSnapshot) { sp.SinceCheck = 0 }),
		top("open_bodies", func(sp *SpawnsSnapshot) { sp.OpenBodies = 0 }),
		top("checks", func(sp *SpawnsSnapshot) { sp.Checks = 0 }),
		top("rolls", func(sp *SpawnsSnapshot) { sp.Rolls = 0 }),
		top("spawned", func(sp *SpawnsSnapshot) { sp.Spawned = 0 }),
		top("failures", func(sp *SpawnsSnapshot) { sp.Failures = 0 }),
		top("dropped", func(sp *SpawnsSnapshot) { sp.Dropped = 0 }),
		top("despawned", func(sp *SpawnsSnapshot) { sp.Despawned = 0 }),
		top("cleared", func(sp *SpawnsSnapshot) { sp.Cleared = 0 }),
		top("released", func(sp *SpawnsSnapshot) { sp.Released = 0 }),
		// The seed is checked against the game's (the B2a review's B1): lost,
		// or another stream's, it is refused rather than rolled on.
		{"spawns.rng.seed lost", b2bRefuse, func(_ *testing.T, s *b2bSnap) { s.Spawns.RNG.Seed = 0 }},
		{"spawns.rng.seed the combat stream's", b2bRefuse, func(_ *testing.T, s *b2bSnap) {
			s.Spawns.RNG.Seed = d2rand.Derive(sv.seed, d2rand.StreamCombat)
		}},
		{"spawns.rng.draws past the cap", b2bRefuse, func(_ *testing.T, s *b2bSnap) { s.Spawns.RNG.Draws = d2rand.MaxDraws + 1 }},
		top("rng.draws", func(sp *SpawnsSnapshot) { sp.RNG.Draws = 0 }),
		{"spawns.groups lost", b2bDiverge, func(_ *testing.T, s *b2bSnap) { s.Spawns.Groups = nil }},

		{"spawns.group.id moved to a free number", b2bDiverge, func(t *testing.T, s *b2bSnap) {
			g, _ := live(t, s)
			g.ID = b2bFreeGroupID(t, s)
		}},
		grp("id empty", b2bRefuse, func(g *SpawnGroupSnapshot) { g.ID = "" }),
		{"spawns.group.id at next_id", b2bRefuse, func(t *testing.T, s *b2bSnap) {
			g, _ := live(t, s)
			g.ID = "g:" + itoa(s.Spawns.NextID)
		}},
		grp("row lost", b2bRefuse, func(g *SpawnGroupSnapshot) { g.Row = "" }),
		grp("row another", b2bDiverge, func(g *SpawnGroupSnapshot) {
			if g.Row == "wolves" {
				g.Row = "dogs"
			} else {
				g.Row = "wolves"
			}
		}),
		grp("code lost", b2bDiverge, func(g *SpawnGroupSnapshot) { g.Code = "" }),
		grp("members lost", b2bRefuse, func(g *SpawnGroupSnapshot) { g.Members = nil }),
		grp("a member dropped", b2bDiverge, func(g *SpawnGroupSnapshot) { g.Members = g.Members[:len(g.Members)-1] }),
		grp("morale lost", b2bDiverge, func(g *SpawnGroupSnapshot) { g.Morale = 0 }),
		grp("born_at lost", b2bDiverge, func(g *SpawnGroupSnapshot) { g.BornAt = 0 }),
		grp("band altered", b2bDiverge, func(g *SpawnGroupSnapshot) { g.Band = 1 - g.Band }),
		grp("band impossible", b2bRefuse, func(g *SpawnGroupSnapshot) { g.Band = 3 }),
		grp("stage lost", b2bRefuse, func(g *SpawnGroupSnapshot) { g.Stage = "" }),
		grp("stage altered", b2bDiverge, func(g *SpawnGroupSnapshot) {
			if g.Stage == "night" {
				g.Stage = "dusk"
			} else {
				g.Stage = "night"
			}
		}),
		grp("weight lost", b2bDiverge, func(g *SpawnGroupSnapshot) { g.Weight = 0 }),
		grp("spawned lost", b2bDiverge, func(g *SpawnGroupSnapshot) { g.Spawned = 0 }),
		grp("born_where lost", b2bDiverge, func(g *SpawnGroupSnapshot) { g.BornWhere = nil }),

		// Identity. Every one of these is refused: a member who resolves to
		// the wrong entity, or who is saved twice, or is on the map when the
		// save said he was not (and the reverse), is a pack hunting with the
		// wrong man.
		{"spawns.member id empty", b2bRefuse, func(t *testing.T, s *b2bSnap) { _, m := live(t, s); m.ID = "" }},
		{"spawns.member id unknown", b2bRefuse, func(t *testing.T, s *b2bSnap) { _, m := live(t, s); m.ID = "e:999" }},
		{"spawns.member id of another pack's man", b2bRefuse, func(t *testing.T, s *b2bSnap) {
			gi, mi := b2bGroupWith(t, s, false)
			s.Spawns.Groups[gi].Members[mi].ID = b2bOtherMember(t, s, gi)
		}},
		{"spawns.member x moved", b2bRefuse, func(t *testing.T, s *b2bSnap) { _, m := live(t, s); m.X += 0.5 }},
		{"spawns.member y moved", b2bRefuse, func(t *testing.T, s *b2bSnap) { _, m := live(t, s); m.Y -= 0.5 }},
		{"spawns.member on the map saved gone", b2bRefuse, func(t *testing.T, s *b2bSnap) { _, m := live(t, s); m.Gone = true }},
		{"spawns.member gone saved on the map", b2bRefuse, func(t *testing.T, s *b2bSnap) { _, m := gone(t, s); m.Gone = false }},
		{"spawns.gone member id altered", b2bDiverge, func(t *testing.T, s *b2bSnap) { _, m := gone(t, s); m.ID = "e:998" }},
		// Carried, never read: nothing in the game asks where a man who is
		// no longer on the map stood. Game.takeOffTheMap only happens to a
		// pack member (a slain man whose body stood up), Spawns.Member's one
		// caller is the risen rejoining a fight, and the layDead hook reads
		// positions of the dead's row alone. It is saved so the stand-in
		// answers WatcherAt as the saved chaser did.
		{"spawns.gone member x moved", b2bCarried, func(t *testing.T, s *b2bSnap) { _, m := gone(t, s); m.X += 0.5 }},
		{"spawns.gone member y moved", b2bCarried, func(t *testing.T, s *b2bSnap) { _, m := gone(t, s); m.Y += 0.5 }},
	})

	b2aExercised(t, outcomes, b2bSpawnsClasses()...)
}

// b2bSpawnsClasses is every field of the spawn tables and of a group,
// labelled at its path in the saved moment (b2bSnap) -- the B2b review's B5:
// B2b's systems had no reflection check, so a field added to one was saved or
// not by chance.
func b2bSpawnsClasses() []b2aClass {
	return []b2aClass{
		{Spawns{}, map[string]string{
			"sheltered":  "T: true only inside a sleep (talk.go sets it and defers it back); Snapshot refuses while set",
			"dials":      "D: DefaultSpawnDials; a dial a script moved is the script's, not the save's (trap 7)",
			"clock":      "W: the clock the tables read; restored on its own",
			"notice":     "W: the notice model the tables watch through; restored on its own",
			"illum":      "W: the light model a spawn position is judged by",
			"spawner":    "W: the game spawner that puts members on the map (its arrival is saved on its own)",
			"target":     "W: the player, set by SetTarget when the game is built",
			"chases":     "W: pursuit, which a despawn releases",
			"rng":        "S:spawns.rng",
			"groups":     "S:spawns.groups",
			"nextID":     "S:spawns.next_id",
			"sinceCk":    "S:spawns.since_check_minutes",
			"openBodies": "S:spawns.open_bodies",
			"layDead":    "W: the game screen's hook for a risen man lying down",
			"checks":     "S:spawns.checks",
			"rolls":      "S:spawns.rolls",
			"spawned":    "S:spawns.spawned",
			"failures":   "S:spawns.failures",
			"dropped":    "S:spawns.dropped",
			"despawned":  "S:spawns.despawned",
			"cleared":    "S:spawns.cleared",
			"released":   "S:spawns.released",
		}},
		{group{}, map[string]string{
			"id":        "S:spawns.groups[0].id",
			"row":       "S:spawns.groups[0].row",
			"code":      "S:spawns.groups[0].code",
			"members":   "S:spawns.groups[0].members",
			"morale":    "S:spawns.groups[0].morale",
			"bornAt":    "S:spawns.groups[0].born_at",
			"band":      "S:spawns.groups[0].band",
			"stage":     "S:spawns.groups[0].stage",
			"weight":    "S:spawns.groups[0].weight",
			"spawned":   "S:spawns.groups[0].spawned",
			"bornWhere": "S:spawns.groups[0].born_where",
		}},
	}
}

func TestSpawnsSnapshotFieldsClassified(t *testing.T) {
	a := b2bFilledWorld(t)
	snap := a.snapshot(t)

	for _, c := range b2bSpawnsClasses() {
		b2aClassified(t, c.kind, snap, c.fields)
	}

	// The one transient: set, the snapshot is refused (the B3 check).
	b2aTransientsRefused(t, b2bSpawnsClasses()[0], func() (interface{}, func() error) {
		w := b2bFilledWorld(t)

		return w.spawns, func() error { _, err := w.spawns.Snapshot(b2bResolver{w}); return err }
	})
}

// GONE IS NOT TAKEN ON THE RESOLVER'S WORD. A live-world Resolver that cannot
// find a member the notice model still watches has lost a living wolf, not
// found a dead man: saved as gone, he would come back a stand-in while his
// entity walked on beside the pack. Refused at save. The dead man the world
// really lost (unwatched at his death) is still saved as gone.
func TestSpawnsSnapshotRefusesAWatchedMemberTheResolverLost(t *testing.T) {
	a := b2bFilledWorld(t)

	_, err := a.spawns.Snapshot(b2bResolver{a})
	require.NoError(t, err, "the control: the filled world, with its gone member, saves")

	var lost string

	for _, id := range a.spawns.groupIDs() {
		for _, m := range a.spawns.groups[id].members {
			if _, watched := a.notice.Noticed(m.WatcherID()); watched && lost == "" {
				lost = m.WatcherID()
			}
		}
	}

	require.NotEmpty(t, lost, "a watched member")

	_, err = a.spawns.Snapshot(b2bForgetful{b2bResolver{a}, lost})
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)
}

// b2bForgetful is a live-world resolver that cannot find one entity.
type b2bForgetful struct {
	b2bResolver
	lost string
}

func (r b2bForgetful) Watcher(id string) (Watcher, bool) {
	if id == r.lost {
		return nil, false
	}

	return r.b2bResolver.Watcher(id)
}

// A refused restore changes nothing: all or nothing.
func TestSpawnsRestoreIsAllOrNothing(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	// A relaunch whose tables are still as built: the only tables a Restore
	// takes.
	b, err := b2bResumeSkipping(t, sv, sv.snap, "spawns")
	require.NoError(t, err)

	before := b.observe(t)

	bad := b2bThroughJSON(t, sv.snap)
	bad.Spawns.NextID++
	bad.Spawns.Checks += 100
	last := &bad.Spawns.Groups[len(bad.Spawns.Groups)-1]
	last.Members[0].ID, last.Members[0].Gone = "e:999", false // refused only after the first groups were read

	require.True(t, errors.Is(b.spawns.Validate(bad.Spawns, b2bResolver{b}, sv.seed), ErrUnresolvedRef), "Validate")

	err = b.spawns.Restore(bad.Spawns, b2bResolver{b}, sv.seed)
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)
	require.Equal(t, before, b.observe(t), "a refused restore must leave the tables exactly as they were")

	require.NoError(t, b.spawns.Validate(sv.snap.Spawns, b2bResolver{b}, sv.seed), "the control: the saved one validates")
	require.Equal(t, before, b.observe(t), "and Validate changes nothing")
}

// A restore into tables that already hold a group is refused (the B2b
// review's B4, as B2a's corpses and squads refuse): the live group's members
// would be left on the map, watched and chasing, in no pack.
func TestSpawnsRestoreRefusesTablesInUse(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)

	before := b.observe(t)

	require.Error(t, b.spawns.Validate(sv.snap.Spawns, b2bResolver{b}, sv.seed))
	require.Error(t, b.spawns.Restore(sv.snap.Spawns, b2bResolver{b}, sv.seed), "the very snapshot, over itself")
	require.Equal(t, before, b.observe(t))
}

// Saving is refused while he shelters; the flag is transient.
func TestSpawnsSnapshotRefusedWhileSheltered(t *testing.T) {
	a := b2bFilledWorld(t)
	a.spawns.SetSheltered(true)

	_, err := a.spawns.Snapshot(b2bResolver{a})
	require.Error(t, err)

	a.spawns.SetSheltered(false)

	_, err = a.spawns.Snapshot(b2bResolver{a})
	require.NoError(t, err)
}

// The dead are saved with their pack, and a man whose remains are off the map
// comes back as a stand-in answering to his id and his last place -- so
// spent(), his profile and member_ids see what the saved game saw.
func TestSpawnsSnapshotKeepsTheDeadInThePack(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	gi, mi := b2bGroupWith(t, &sv.snap, true)
	g := sv.snap.Spawns.Groups[gi]
	dead := g.Members[mi]

	_, onMap := a.entities[dead.ID]
	require.False(t, onMap, "the gone member is off the map")

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)

	m, ok := b.spawns.Member(dead.ID)
	require.True(t, ok, "the dead man is still in his pack")
	require.IsType(t, &b2bGoneMember{}, m)

	x, y := m.WatcherAt()
	require.Equal(t, [2]float64{dead.X, dead.Y}, [2]float64{x, y})

	p, ok := b.spawns.ProfileOf(dead.ID)
	require.True(t, ok)
	require.Equal(t, g.ID, p.Group, "his profile still names his pack")

	// A despawn of his pack takes the living off the map and skips him.
	require.True(t, b.spawns.Despawn(g.ID))
}

// A wall-clock GAME seed is past 2^53. The tables' stream runs on the seed
// Derive gives it; the restore checks it against the game seed, read as an
// int64, and the restored stream hands out the value the saved one would have.
// (The stream state's own trip through JSON at a seed past 2^53 is d2rand's
// TestStreamStateSeedSurvivesJSON, which the three systems now share.)
func TestSpawnsSnapshotSeedIsExactPast2to53(t *testing.T) {
	const world = int64(1)<<62 + 1

	require.NotEqual(t, world, int64(float64(world)), "the control: float64 loses this seed")

	seed := d2rand.Derive(world, d2rand.StreamSpawns)

	b, err := json.Marshal(SpawnsSnapshot{NextID: 1, RNG: d2rand.StreamState{Seed: seed, Draws: 5}})
	require.NoError(t, err)

	var loose map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &loose))
	rng, ok := loose["rng"].(map[string]interface{})
	require.True(t, ok, "the stream is written as {seed, draws}: %s", b)
	require.IsType(t, "", rng["seed"], "the seed is a string")
	require.Equal(t, 5.0, rng["draws"])

	var back SpawnsSnapshot
	require.NoError(t, json.Unmarshal(b, &back))
	require.Equal(t, seed, back.RNG.Seed)

	s, _, _, _, _ := newTestSpawns(t)
	require.NoError(t, s.Restore(back, b2bResolver{&b2bWorld{entities: map[string]*b2bEntity{}}}, world))
	require.Equal(t, seed, s.rng.Seeded())
	require.Equal(t, uint64(5), s.rng.Draws())
	require.Equal(t, nextAfter(seed, 5), s.rng.Int63(), "the next value is the saved stream's sixth")

	lost, _, _, _, _ := newTestSpawns(t)
	require.Error(t, lost.Restore(back, b2bResolver{&b2bWorld{entities: map[string]*b2bEntity{}}}, int64(float64(world))),
		"the game seed read through float64 is another game, and its tables another stream")
}

// Losing the minutes run toward the next table check moves WHEN the tables
// look, not just a reported number: the resumed world checks on another
// minute.
func TestSpawnsSinceCheckIsBehaviour(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	lost := b2bThroughJSON(t, sv.snap)
	lost.Spawns.SinceCheck = 0

	at := b2bProbeDiverges(t, sv, lost, func(w *b2bWorld) float64 { return float64(w.spawns.checks) })
	require.GreaterOrEqual(t, at, 1, "the table checks must fall on another tick")
}

// b2bProbeDiverges resumes the saved moment twice -- once as saved, once with
// a changed snapshot -- runs both, and returns the first tick at which probe
// reads differently (-1 if never). Tick 0 is the load itself.
func b2bProbeDiverges(t *testing.T, sv b2bSaved, changed b2bSnap, probe func(*b2bWorld) float64) int {
	t.Helper()

	ref, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)

	alt, err := b2bResume(t, sv, changed)
	require.NoError(t, err)

	for i := 0; i <= b2bAdvanceTicks; i++ {
		if probe(ref) != probe(alt) {
			return i
		}

		ref.tick()
		alt.tick()
	}

	return -1
}
