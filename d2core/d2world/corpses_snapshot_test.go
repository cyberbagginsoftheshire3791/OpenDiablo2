package d2world

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// b2aCorpsesWorld is a registry wired the way the game wires it -- a row
// classes a body and names what he was, a callback hears the open count move,
// a clock stamps the Downed -- holding one body in every state: open men and
// an open carcass, a hasty grave, a staked man, a risen man walking, a man cut
// down (Downed, with his minute), an edge wanderer walking, and a man who has
// stood twice (two members that walk back to one body).
type b2aCorpsesWorld struct {
	c      *Corpses
	minute float64
	deltas []int
}

func newB2aCorpses() *b2aCorpsesWorld {
	w := &b2aCorpsesWorld{}
	w.c = NewCorpses(
		func(row string) bool { return row == "men" },
		func(row string) string { return "a man of the " + row },
		func(d int) { w.deltas = append(w.deltas, d) },
	)
	w.c.SetClock(func() float64 { return w.minute })

	return w
}

func b2aCorpsesFixture(t *testing.T) *b2aCorpsesWorld {
	t.Helper()

	w := newB2aCorpses()
	c := w.c

	c.Fall("b:1", "men", 1, 1.5)
	c.Fall("b:2", "dogs", 2, 2.5)
	c.Fall("b:3", "men", 3, 3.5)
	require.True(t, c.Bury("b:3"))
	c.Fall("b:4", "men", 4, 4.5)
	require.True(t, c.Close("b:4"))

	c.Fall("b:5", "men", 5, 5.5)
	require.True(t, c.Rise("b:5"))
	c.Raised("b:5", "r:1")

	c.Fall("b:6", "men", 6, 6.5)
	require.True(t, c.Rise("b:6"))
	c.Raised("b:6", "r:2")

	w.minute = 37
	c.Fall("r:2", "", 6.25, 6.75) // cut down: b:6 lies Downed

	c.FallHuman("wanderer:1", "a stranger", 7, 7.5)
	require.True(t, c.Rise("wanderer:1"))
	c.Raised("wanderer:1", "r:3")

	c.Fall("b:7", "men", 8, 8.5)
	require.True(t, c.Rise("b:7"))
	c.Raised("b:7", "r:4")

	w.minute = 41
	c.Fall("r:4", "", 8.25, 8.75)
	require.True(t, c.Rise("b:7")) // he stands again
	c.Raised("b:7", "r:5")

	c.Fall("b:8", "men", 9, 9.5)

	return w
}

// b2aCorpsesSteps runs the machine on: walkers fall, bodies are staked, buried
// and risen, a stale member falls again, a carcass falls; then every question
// the game asks the registry is asked of every id.
func b2aCorpsesSteps(t *testing.T, w *b2aCorpsesWorld) string {
	t.Helper()

	c := w.c

	var trace strings.Builder

	// The first frame after a load, before anything moves a body.
	trace.WriteString(b2aJSON(t, c.HarnessState()))

	w.minute = 50

	c.Fall("r:1", "", 5.25, 5.75)
	c.Fall("r:3", "", 7.25, 7.75)
	c.Fall("r:4", "", 1, 1) // his older member: the body walks as r:5 now
	c.Close("b:1")
	c.Bury("b:2")
	c.Rise("b:3")
	c.Raised("b:3", "r:6")
	c.Close("b:6")
	c.Fall("b:9", "dogs", 9, 9)

	trace.WriteString(b2aJSON(t, c.HarnessState()))
	trace.WriteString(b2aJSON(t, w.deltas))

	for i := 1; i <= 7; i++ {
		m := fmt.Sprintf("r:%d", i)
		body, ok := c.BodyOf(m)
		trace.WriteString(b2aJSON(t, []interface{}{m, body, ok, c.DownedMember(m)}))
	}

	for _, b := range c.All() {
		got, ok := c.Get(b.ID)
		trace.WriteString(b2aJSON(t, []interface{}{b.ID, c.LastWalker(b.ID), c.Has(b.ID), got, ok, b.Door()}))
	}

	for _, at := range [][3]float64{{5, 5, 3}, {1, 1, 20}, {6.5, 6.5, 1.5}, {9, 9, 0.8}} {
		trace.WriteString(b2aJSON(t, c.Nearest(at[0], at[1], at[2], nil)))
		trace.WriteString(b2aJSON(t, c.Nearest(at[0], at[1], at[2], func(b *Corpse) bool { return b.Door() })))
	}

	trace.WriteString(b2aJSON(t, c.Open()))

	return trace.String()
}

func TestCorpsesSnapshotFieldsClassified(t *testing.T) {
	w := b2aCorpsesFixture(t)
	snap := w.c.Snapshot()

	b2aClassified(t, Corpses{}, snap, map[string]string{
		"byID":    "S:bodies",
		"order":   "S:bodies",
		"risenAs": "S:risen_as",
		"walker":  "S:walker",
		"last":    "S:last",
		"now":     "W: the world minutes a Downed man is stamped with (SetClock)",
		"isHuman": "W: the spawn tables' row classifier, given at construction",
		"was":     "W: the row's live name, given at construction",
		"changed": "W: the open-count callback; Restore never calls it (trap 5)",
	})

	b2aClassified(t, Corpse{}, snap, map[string]string{
		"ID":       "S:bodies[0].id",
		"Row":      "S:bodies[0].row",
		"Class":    "S:bodies[0].class",
		"State":    "S:bodies[0].state",
		"X":        "S:bodies[0].x",
		"Y":        "S:bodies[0].y",
		"Was":      "S:bodies[0].was",
		"DownedAt": "S:bodies[0].downed_at",
	})
}

func TestCorpsesSnapshotRoundTrip(t *testing.T) {
	w := b2aCorpsesFixture(t)

	snap := b2aThroughJSON(t, w.c.Snapshot())
	require.Equal(t, w.c.Snapshot(), snap, "the snapshot survives JSON exactly")
	require.Len(t, snap.Bodies, 9)
	require.Equal(t, "b:7", snap.RisenAs["r:4"], "a man who stood twice keeps both members")
	require.Equal(t, "b:7", snap.RisenAs["r:5"])

	w2 := newB2aCorpses()
	require.NoError(t, w2.c.Restore(snap))
	require.Empty(t, w2.deltas, "RESTORE NEVER CALLS changed (trap 5): the open count comes from the file")

	require.Equal(t, b2aJSON(t, w.c.HarnessState()), b2aJSON(t, w2.c.HarnessState()), "restored is the same registry")

	// The copy's callback log starts empty; the original's holds the fixture's
	// deltas, so the steps compare from here.
	w.deltas = nil
	require.Equal(t, b2aCorpsesSteps(t, w), b2aCorpsesSteps(t, w2), "and stays the same registry run on")

	// Snapshot copies its maps, so a reader cannot reach the registry.
	snap2 := w2.c.Snapshot()
	snap2.RisenAs["r:1"] = "b:1"
	require.Equal(t, "b:5", w2.c.Snapshot().RisenAs["r:1"])
}

// A new game saved at once: no bodies.
func TestCorpsesSnapshotOfANewGame(t *testing.T) {
	orig, cp := newB2aCorpses(), newB2aCorpses()

	require.NoError(t, cp.c.Restore(b2aThroughJSON(t, orig.c.Snapshot())))
	require.Equal(t, b2aJSON(t, orig.c.HarnessState()), b2aJSON(t, cp.c.HarnessState()))
	require.Empty(t, cp.deltas)
}

func TestCorpsesSnapshotRefusesWhatCannotBe(t *testing.T) {
	good := b2aCorpsesFixture(t).c.Snapshot()

	for name, bad := range map[string]func(s *CorpsesSnapshot){
		"a body twice":                 func(s *CorpsesSnapshot) { s.Bodies[1].ID = s.Bodies[0].ID },
		"no id":                        func(s *CorpsesSnapshot) { s.Bodies[1].ID = "" },
		"no class":                     func(s *CorpsesSnapshot) { s.Bodies[1].Class = "" },
		"no state":                     func(s *CorpsesSnapshot) { s.Bodies[1].State = "sleeping" },
		"a member of no body":          func(s *CorpsesSnapshot) { s.RisenAs["r:1"] = "b:99" },
		"a walker of a body not risen": func(s *CorpsesSnapshot) { s.Walker["b:1"] = "r:1" },
		"a walker not walking back":    func(s *CorpsesSnapshot) { s.Walker["b:5"] = "r:2" },
		"a last not walking back":      func(s *CorpsesSnapshot) { s.Last["b:6"] = "r:1" },
	} {
		var s CorpsesSnapshot

		raw, _ := json.Marshal(good)
		require.NoError(t, json.Unmarshal(raw, &s))
		bad(&s)

		w := newB2aCorpses()
		require.Error(t, w.c.Restore(s), name)
		require.Empty(t, w.c.All(), "%s: a refused restore changes nothing", name)
	}

	// Into a registry that already holds a body -- whose fall already moved
	// the open count -- it is refused.
	w := newB2aCorpses()
	w.c.Fall("placed:1", "men", 0, 0)
	require.Error(t, w.c.Restore(good))
}

func TestCorpsesSnapshotEveryFieldIsLoadBearing(t *testing.T) {
	w := b2aCorpsesFixture(t)
	snap := w.c.Snapshot()
	w.deltas = nil
	ref := b2aCorpsesSteps(t, w)

	try := func(raw []byte) (string, error) {
		var s CorpsesSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		w2 := newB2aCorpses()
		if err := w2.c.Restore(s); err != nil {
			return "", err
		}

		return b2aCorpsesSteps(t, w2), nil
	}

	b2aSweep(t, snap, ref, nil, try)

	// A body's id, class and state are only ever refused when zeroed (no id,
	// no class, no state); a valid other one must show.
	b2aMustDiverge(t, snap, ref, map[string]func(s *CorpsesSnapshot){
		"a man's body a beast's": func(s *CorpsesSnapshot) { s.Bodies[0].Class = CorpseBeast },
		"an open body staked":    func(s *CorpsesSnapshot) { s.Bodies[0].State = CorpseClosed },
		"a body renamed":         func(s *CorpsesSnapshot) { s.Bodies[8].ID = "b:88" },
	}, try)
}
