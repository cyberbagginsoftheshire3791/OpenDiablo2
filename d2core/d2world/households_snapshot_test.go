package d2world

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// THE RAID'S R3a (households.go, households_snapshot.go): the houses the map
// places, their stock saved and put back, five ways (B2's conventions).

// hhPlaces is a village of three households -- a house of a man, a woman and
// a child with the smith as its speaker, an old couple's, and the church with
// the priest -- a hotar and two posts.
func hhPlaces() VillagePlaces {
	return VillagePlaces{
		Households: []HouseholdPlace{
			{Name: "the smith's house", DoorX: 20, DoorY: 31, Members: []string{"man", "woman", "child"},
				Speakers: []string{"charsi"}, Incense: 3, Stakes: 2},
			{Name: "the old couple's", DoorX: 30, DoorY: 16, Members: []string{"old", "old"}, Incense: 1},
			{Name: "the church", DoorX: 16, DoorY: 16, Speakers: []string{"akara"}, Incense: 5, Stakes: 4, Church: true},
		},
		HasHotar: true, HotarX: 22, HotarY: 40,
		Posts: []PostPlace{{X: 24, Y: 34, Post: "gate"}, {X: 33, Y: 13, Post: "corner"}},
	}
}

func hhNew(t *testing.T) *Households {
	t.Helper()

	h, err := NewHouseholds(hhPlaces())
	require.NoError(t, err)
	t.Cleanup(h.Close)

	return h
}

// hhWorld is a village whose stock has moved off what the map authored: the
// smith's house down to one incense and no stakes, the old couple's up to
// three stakes.
func hhWorld(t *testing.T) *Households {
	t.Helper()

	h := hhNew(t)
	hhSet(t, h, "incense", "h:1", 1)
	hhSet(t, h, "stakes", "h:1", 0)
	hhSet(t, h, "stakes", "h:2", 3)

	return h
}

func hhSet(t *testing.T, h *Households, field, id string, n float64) {
	t.Helper()
	require.NoError(t, h.HarnessSet(field, map[string]interface{}{"house": id, "value": n}))
}

// hhSteps runs the houses on: three writes, the provider read before the
// first and after each (the first read is a load's first frame).
func hhSteps(t *testing.T, h *Households) string {
	t.Helper()

	out := b2aJSON(t, h.HarnessState()) + "\n"

	for _, w := range []struct {
		field, id string
		n         float64
	}{{"incense", "h:3", 2}, {"stakes", "h:2", 1}, {"incense", "h:2", 4}} {
		hhSet(t, h, w.field, w.id, w.n)
		out += b2aJSON(t, h.HarnessState()) + "\n"
	}

	return out
}

func hhClasses() []b2aClass {
	return []b2aClass{
		{Households{}, map[string]string{
			"places": "D: the map's -- doors, roles, speakers, church, hotar and posts; a file of another map is refused before this block (D5)",
			"houses": "S:houses",
		}},
		{house{}, map[string]string{
			"incense": "S:houses[0].incense",
			"stakes":  "S:houses[0].stakes",
		}},
	}
}

func TestHouseholdsSnapshotFieldsClassified(t *testing.T) {
	h := hhWorld(t)

	for _, c := range hhClasses() {
		b2aClassified(t, c.kind, h.Snapshot(), c.fields)
	}
}

// TestHouseholdsSnapshotRoundTrips: the stock survives JSON exactly, and a
// village restored from it -- a fresh one, at the map's own stock -- reports
// as the original did and stays the same village as both run on.
func TestHouseholdsSnapshotRoundTrips(t *testing.T) {
	h := hhWorld(t)

	snap := b2aThroughJSON(t, h.Snapshot())
	require.Equal(t, h.Snapshot(), snap, "the snapshot survives JSON exactly")
	require.Equal(t, []HouseSnapshot{{"h:1", 1, 0}, {"h:2", 1, 3}, {"h:3", 5, 4}}, snap.Houses)

	g := hhNew(t)
	require.NotEqual(t, h.HarnessState(), g.HarnessState(), "the fresh village is at the map's stock, not the saved one")
	require.NoError(t, g.Restore(snap))
	require.Equal(t, h.HarnessState(), g.HarnessState(), "restored, it reports what the original does")
	require.Equal(t, snap, g.Snapshot())
	require.Equal(t, hhSteps(t, h), hhSteps(t, g), "and runs on as the original does")
}

// TestHouseholdsSnapshotOfANewGame: a fresh village snapshots the map's own
// stock; a village of no households (a generated world, or a map with none)
// is an empty list -- never null -- and restores into a village of none.
func TestHouseholdsSnapshotOfANewGame(t *testing.T) {
	h := hhNew(t)
	require.Equal(t, []HouseSnapshot{{"h:1", 3, 2}, {"h:2", 1, 0}, {"h:3", 5, 4}}, h.Snapshot().Houses)

	none, err := NewHouseholds(VillagePlaces{})
	require.NoError(t, err)
	t.Cleanup(none.Close)

	raw, err := json.Marshal(none.Snapshot())
	require.NoError(t, err)
	require.JSONEq(t, `{"houses": []}`, string(raw), "no households is an empty list")

	again, err := NewHouseholds(VillagePlaces{})
	require.NoError(t, err)
	t.Cleanup(again.Close)
	require.NoError(t, again.Restore(b2aThroughJSON(t, none.Snapshot())))
	require.Equal(t, none.HarnessState(), again.HarnessState())
	require.Error(t, again.Restore(h.Snapshot()), "three houses do not restore into a village of none")
}

// The sweep: every leaf zeroed (or set), every house dropped -- each refused,
// or seen; and the probes: valid but wrong, each accepted and seen.
func TestHouseholdsSnapshotEveryFieldIsSeen(t *testing.T) {
	h := hhWorld(t)
	ref := hhSteps(t, hhWorld(t))

	try := func(raw []byte) (string, error) {
		var snap HouseholdsSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			return "", err
		}

		g := hhNew(t)
		if err := g.Restore(snap); err != nil {
			return "", err
		}

		return hhSteps(t, g), nil
	}

	b2aExercised(t, b2aSweep(t, h.Snapshot(), ref, nil, try), hhClasses()...)

	b2aMustDiverge(t, h.Snapshot(), ref, map[string]func(s *HouseholdsSnapshot){
		"two houses' stock swapped": func(s *HouseholdsSnapshot) {
			s.Houses[0].Incense, s.Houses[1].Incense = s.Houses[1].Incense+1, s.Houses[0].Incense
			s.Houses[0].Stakes, s.Houses[1].Stakes = s.Houses[1].Stakes, s.Houses[0].Stakes
		},
		"the map's own stock (a save that kept nothing)": func(s *HouseholdsSnapshot) {
			*s = hhNew(t).Snapshot()
		},
	}, try)
}

// What the restore refuses, one at a time, of an otherwise good snapshot --
// and a refused restore changes nothing.
func TestHouseholdsSnapshotRefusals(t *testing.T) {
	for name, bad := range map[string]func(s *HouseholdsSnapshot){
		"a house too few":             func(s *HouseholdsSnapshot) { s.Houses = s.Houses[:2] },
		"a house too many":            func(s *HouseholdsSnapshot) { s.Houses = append(s.Houses, HouseSnapshot{ID: "h:4"}) },
		"houses out of order":         func(s *HouseholdsSnapshot) { s.Houses[0], s.Houses[1] = s.Houses[1], s.Houses[0] },
		"a house of no id":            func(s *HouseholdsSnapshot) { s.Houses[2].ID = "" },
		"incense below zero":          func(s *HouseholdsSnapshot) { s.Houses[0].Incense = -1 },
		"stakes past the stock":       func(s *HouseholdsSnapshot) { s.Houses[1].Stakes = HouseholdStockMax + 1 },
		"incense past the stock":      func(s *HouseholdsSnapshot) { s.Houses[2].Incense = HouseholdStockMax + 1 },
		"no houses where there are 3": func(s *HouseholdsSnapshot) { s.Houses = []HouseSnapshot{} },
	} {
		h := hhWorld(t)
		snap := b2aThroughJSON(t, h.Snapshot())
		bad(&snap)

		before := h.HarnessState()
		require.Error(t, h.Validate(snap), name)
		require.Error(t, h.Restore(snap), name)
		require.Equal(t, before, h.HarnessState(), "%s: a refused restore changes nothing", name)
	}
}

// TestHouseholdsReportTheMap: the provider reports what the map placed and
// the stock; its writes refuse what is not a house or a stock.
func TestHouseholdsReportTheMap(t *testing.T) {
	h := hhNew(t)
	st := h.HarnessState()

	require.Equal(t, 3, st["households"])
	require.Equal(t, 1, st["churches"])
	require.Equal(t, 7, st["members"], "three roles and a speaker, two roles, a speaker")
	require.Equal(t, []int{22, 40}, st["hotar"])
	require.Equal(t, []map[string]interface{}{{"post": "gate", "at": []int{24, 34}}, {"post": "corner", "at": []int{33, 13}}}, st["posts"])

	first := st["houses"].([]map[string]interface{})[0]
	require.Equal(t, map[string]interface{}{
		"id": "h:1", "name": "the smith's house", "door": []int{20, 31}, "members": []string{"man", "woman", "child"},
		"speakers": []string{"charsi"}, "church": false, "incense": 3, "stakes": 2, "incense_start": 3, "stakes_start": 2,
	}, first)

	for _, bad := range []interface{}{
		map[string]interface{}{"house": "h:4", "value": 1.0},
		map[string]interface{}{"house": "h:1", "value": -1.0},
		map[string]interface{}{"house": "h:1", "value": 1.5},
		map[string]interface{}{"house": "h:1", "value": float64(HouseholdStockMax + 1)},
		map[string]interface{}{"house": "h:1"},
		map[string]interface{}{"house": "h:1", "value": 1.0, "and": 2.0},
		3.0,
	} {
		require.Error(t, h.HarnessSet("incense", bad), "%v", bad)
	}

	require.Error(t, h.HarnessSet("kept", map[string]interface{}{"house": "h:1", "value": 1.0}), "no field but incense and stakes")
	require.Equal(t, st, h.HarnessState(), "every refused write changed nothing")

	none, err := NewHouseholds(VillagePlaces{})
	require.NoError(t, err)
	t.Cleanup(none.Close)
	require.Nil(t, none.HarnessState()["hotar"], "no hotar reads null")

	_, err = NewHouseholds(VillagePlaces{Households: []HouseholdPlace{{Name: "x", Incense: HouseholdStockMax + 1}}})
	require.Error(t, err, "a map's stock past the bound is refused")
}

// TestTheStockBoundIsInclusive (the R3a review's B4): a house may hold
// exactly HouseholdStockMax, built from the map or restored from the file,
// and not one more.
func TestTheStockBoundIsInclusive(t *testing.T) {
	top := HouseholdStockMax

	h, err := NewHouseholds(VillagePlaces{Households: []HouseholdPlace{{Name: "x", Incense: top, Stakes: top}}})
	require.NoError(t, err, "a house at the bound is built")
	t.Cleanup(h.Close)

	require.NoError(t, HouseholdsSnapshot{Houses: []HouseSnapshot{{ID: "h:1", Incense: top, Stakes: top}}}.Check(), "and kept")
	require.NoError(t, h.HarnessSet("incense", map[string]interface{}{"house": "h:1", "value": float64(top)}), "and set")

	for _, bad := range []HouseSnapshot{{ID: "h:1", Incense: top + 1}, {ID: "h:1", Stakes: top + 1}} {
		require.Error(t, HouseholdsSnapshot{Houses: []HouseSnapshot{bad}}.Check(), "%+v", bad)
	}
}
