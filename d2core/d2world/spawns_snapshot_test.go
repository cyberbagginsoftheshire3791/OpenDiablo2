package d2world

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, int64(1462), b.spawns.rng.Seeded())
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

	b2bSweep(t, sv, want, []b2bMutation{
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
		top("rng_seed", func(sp *SpawnsSnapshot) { sp.RNGSeed = 0 }),
		top("rng_draws", func(sp *SpawnsSnapshot) { sp.RNGDraws = 0 }),
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
	})
}

// A refused restore changes nothing: all or nothing.
func TestSpawnsRestoreIsAllOrNothing(t *testing.T) {
	a := b2bFilledWorld(t)
	sv := a.save(t)

	b, err := b2bResume(t, sv, sv.snap)
	require.NoError(t, err)

	before := b.observe(t)

	bad := b2bThroughJSON(t, sv.snap)
	bad.Spawns.NextID++
	bad.Spawns.Checks += 100
	last := &bad.Spawns.Groups[len(bad.Spawns.Groups)-1]
	last.Members[0].ID, last.Members[0].Gone = "e:999", false // refused only after the first groups were read

	err = b.spawns.Restore(bad.Spawns, b2bResolver{b})
	require.True(t, errors.Is(err, ErrUnresolvedRef), "%v", err)
	require.Equal(t, before, b.observe(t), "a refused restore must leave the tables exactly as they were")
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

// The seed is written as a string, exact past 2^53, and the restored stream
// hands out the value the saved one would have.
func TestSpawnsSnapshotSeedIsExactPast2to53(t *testing.T) {
	const seed = int64(1)<<62 + 1

	b, err := json.Marshal(SpawnsSnapshot{NextID: 1, RNGSeed: seed, RNGDraws: 5})
	require.NoError(t, err)

	var loose map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &loose))
	require.Equal(t, "4611686018427387905", loose["rng_seed"], "a reader decoding into interface{} still gets it exactly")

	var back SpawnsSnapshot
	require.NoError(t, json.Unmarshal(b, &back))
	require.Equal(t, seed, back.RNGSeed)

	// The control: the same field without ",string" is a JSON number, and a
	// float64 reader gets a different seed.
	type plain struct {
		RNGSeed int64 `json:"rng_seed"`
	}

	pb, err := json.Marshal(plain{seed})
	require.NoError(t, err)

	var pl map[string]interface{}
	require.NoError(t, json.Unmarshal(pb, &pl))
	require.NotEqual(t, seed, int64(pl["rng_seed"].(float64)), "the control: a number past 2^53 does not survive float64")

	s, _, _, _, _ := newTestSpawns(t)
	require.NoError(t, s.Restore(back, b2bResolver{&b2bWorld{entities: map[string]*b2bEntity{}}}))
	require.Equal(t, seed, s.rng.Seeded())
	require.Equal(t, uint64(5), s.rng.Draws())
	require.Equal(t, nextAfter(seed, 5), s.rng.Int63(), "the next value is the saved stream's sixth")
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
