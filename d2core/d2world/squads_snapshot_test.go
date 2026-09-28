package d2world

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// b2aSquadsWorld is the player's squad bound to his body and two deployed
// squads (a third deployed and recalled, so next_id has a gap), every one in a
// different state: s:1 starving on watch, s:2 parched at labour with two
// models (the second added by hand -- nothing in c-1 deploys a squad of two,
// and the snapshot must carry one), s:4 foraging and selected. Seventeen
// minutes of neglect leave a fraction of a point owed on s:1 and s:2.
func b2aSquadsWorld(t *testing.T) (*Clock, *Squads, *fakeBody) {
	t.Helper()

	c := NewClock(DefaultClockDials())
	t.Cleanup(c.Close)
	c.Advance(300)

	s := NewSquads(c, DefaultMeterDials(), &fakeDeployer{})
	t.Cleanup(s.Close)

	body := &fakeBody{health: 80, maxHealth: 100}
	s.BindPlayer(body, "p:1")

	for i := 0; i < 3; i++ {
		require.NoError(t, s.HarnessSet("squad_add", map[string]interface{}{"x": 3.0, "y": 4.0}))
	}

	require.NoError(t, s.HarnessSet("squad_remove", "s:3"))

	s.squads["s:2"].members = append(s.squads["s:2"].members, &model{entity: "m:9", health: 30, max: 50, order: 1})

	for _, w := range []struct {
		squad, field string
		value        interface{}
	}{
		{"s:1", "food", 0.0}, {"s:1", "water", 50.0}, {"s:1", "fatigue", 30.0}, {"s:1", "activity", "watch"},
		{"s:2", "food", 40.0}, {"s:2", "water", 0.0}, {"s:2", "fatigue", 80.0}, {"s:2", "activity", "labour"},
		{"s:4", "food", 20.0}, {"s:4", "water", 70.0}, {"s:4", "fatigue", 10.0}, {"s:4", "activity", "forage"},
	} {
		require.NoError(t, s.HarnessSet("squad", map[string]interface{}{"squad": w.squad, "field": w.field, "value": w.value}))
	}

	require.True(t, s.SetSelected("s:4"))

	s.Advance(c.Advance(4.25))

	return c, s, body
}

// b2aSquadsSteps runs the squads on through neglect, reading everything the
// game reads off them: the provider, the bars, the sheet, and each model's
// fitness as combat looks it up. It cycles the selection once.
func b2aSquadsSteps(t *testing.T, c *Clock, s *Squads) string {
	t.Helper()

	var trace strings.Builder

	observe := func() {
		trace.WriteString(b2aJSON(t, s.HarnessState()))
		trace.WriteString(b2aJSON(t, s.Bars()))
		trace.WriteString(b2aJSON(t, s.SheetCards()))

		for _, id := range []string{"p:1", "m:1", "m:9", "m:3", "m:2"} {
			if f := s.FitnessOf(id); f != nil {
				trace.WriteString(b2aJSON(t, []interface{}{id, s.SquadOf(id), f.ReactionAvailable(), f.Shaken(), f.Activity()}))
			}
		}
	}

	observe() // the first frame after a load

	for i := 0; i < 8; i++ {
		s.Advance(c.Advance(3.5))

		if i == 5 {
			s.Cycle()
		}

		observe()
	}

	return trace.String()
}

// b2aSquadsCopy is a fresh owner restored from snap and bound, as the load
// binds it, to a body in the state the original's stood in.
func b2aSquadsCopy(t *testing.T, clockSnap ClockSnapshot, snap SquadsSnapshot, health int, bindFirst bool) (*Clock, *Squads, error) {
	t.Helper()

	c := NewClock(DefaultClockDials())
	require.NoError(t, c.Restore(clockSnap))

	s := NewSquads(c, DefaultMeterDials(), &fakeDeployer{})
	body := &fakeBody{health: health, maxHealth: 100}

	if bindFirst {
		s.BindPlayer(body, "p:1")
	}

	if err := s.Restore(snap); err != nil {
		c.Close()
		s.Close()

		return nil, nil, err
	}

	if !bindFirst {
		s.BindPlayer(body, "p:1")
	}

	t.Cleanup(c.Close)
	t.Cleanup(s.Close)

	return c, s, nil
}

func TestSquadsSnapshotFieldsClassified(t *testing.T) {
	_, s, _ := b2aSquadsWorld(t)
	snap := s.Snapshot()

	b2aClassified(t, Squads{}, snap, map[string]string{
		"clock":    "W: the clock the meters read; restored on its own",
		"dials":    "W: construction dials; the game builds the squads with the defaults",
		"deployer": "W: how a model becomes a map entity; the load rebuilds entities through B2b's seam",
		"squads":   "S:squads",
		"nextID":   "S:next_id",
		"selected": "S:selected",
	})

	b2aClassified(t, squad{}, snap, map[string]string{
		"id":      "S:squads[0].id",
		"owner":   "S:squads[0].owner",
		"ordinal": "S:squads[0].ordinal",
		"meters":  "S:squads[0].meters",
		"extBody": "D: s:1's body is the player's, bound by BindPlayer on the game's first frame with a player",
		"members": "S:squads[0].members",
		"morale":  "S:squads[0].morale",
	})

	b2aClassified(t, model{}, snap, map[string]string{
		"entity": "S:squads[1].members[0].entity",
		"health": "S:squads[1].members[0].health",
		"max":    "S:squads[1].members[0].max",
		"order":  "S:squads[1].members[0].order",
	})

	b2aClassified(t, Meters{}, snap, map[string]string{
		"cond":     "D: his talents (T3), set again when the load applies his progress; s:1's meters keep theirs",
		"dials":    "W: construction dials; the squads build every meters with their own",
		"clock":    "W: the clock the meters read; restored on its own",
		"body":     "D: the health neglect spends -- s:1's by BindPlayer, a deployed squad's front model by Restore",
		"food":     "S:squads[0].meters.food",
		"water":    "S:squads[0].meters.water",
		"fatigue":  "S:squads[0].meters.fatigue",
		"activity": "S:squads[0].meters.activity",
		"damage":   "S:squads[0].meters.damage",
	})
}

func TestSquadsSnapshotRoundTrip(t *testing.T) {
	c, s, body := b2aSquadsWorld(t)

	snap := b2aThroughJSON(t, s.Snapshot())
	require.Equal(t, s.Snapshot(), snap, "the snapshot survives JSON exactly")
	require.Equal(t, playerEntity, snap.Squads[0].Members[0].Entity, "s:1's member is written as the player")
	require.Positive(t, snap.Squads[0].Meters.Damage, "the fixture owes a fraction of a point on s:1")
	require.Positive(t, snap.Squads[1].Meters.Damage, "and on s:2")

	playerMeters := s.PlayerMeters()
	playerMeters.SetConditioning(Conditioning{FatigueRate: 0.5})

	for _, bindFirst := range []bool{false, true} {
		c2, s2, err := b2aSquadsCopy(t, c.Snapshot(), snap, body.health, bindFirst)
		require.NoError(t, err)
		s2.PlayerMeters().SetConditioning(Conditioning{FatigueRate: 0.5})

		require.Equal(t, b2aJSON(t, s.HarnessState()), b2aJSON(t, s2.HarnessState()), "bindFirst=%v: the same squads", bindFirst)

		if bindFirst {
			require.Equal(t, b2aSquadsSteps(t, c, s), b2aSquadsSteps(t, c2, s2), "and they stay the same run on")
		}
	}

	// s:1 is restored IN PLACE: the pointer the game holds as its meters is
	// still the player's squad's afterwards, and keeps his conditioning.
	c3 := NewClock(DefaultClockDials())
	t.Cleanup(c3.Close)
	require.NoError(t, c3.Restore(c.Snapshot()))

	s3 := NewSquads(c3, DefaultMeterDials(), &fakeDeployer{})
	t.Cleanup(s3.Close)

	held := s3.PlayerMeters()
	held.SetConditioning(Conditioning{FatigueRate: 0.25})
	require.NoError(t, s3.Restore(snap))
	require.Same(t, held, s3.PlayerMeters(), "the game's meters pointer survives a restore")
	require.Equal(t, 0.25, held.cond.FatigueRate, "and so does his conditioning")
	require.Equal(t, snap.Squads[0].Meters.Fatigue, held.Fatigue(), "with the saved values written into it")
}

// A new game saved at once: s:1 alone, before and after it is bound.
func TestSquadsSnapshotOfANewGame(t *testing.T) {
	c := NewClock(DefaultClockDials())
	t.Cleanup(c.Close)

	for _, bound := range []bool{false, true} {
		orig := NewSquads(c, DefaultMeterDials(), &fakeDeployer{})
		t.Cleanup(orig.Close)

		cp := NewSquads(c, DefaultMeterDials(), &fakeDeployer{})
		t.Cleanup(cp.Close)

		if bound {
			orig.BindPlayer(&fakeBody{health: 100, maxHealth: 100}, "p:1")
			cp.BindPlayer(&fakeBody{health: 100, maxHealth: 100}, "p:1")
		}

		require.NoError(t, cp.Restore(b2aThroughJSON(t, orig.Snapshot())))
		require.Equal(t, b2aJSON(t, orig.HarnessState()), b2aJSON(t, cp.HarnessState()), "bound=%v", bound)
	}
}

func TestSquadsSnapshotRefusesWhatCannotBe(t *testing.T) {
	_, s, _ := b2aSquadsWorld(t)
	good := s.Snapshot()

	for name, bad := range map[string]func(s *SquadsSnapshot){
		"no s:1":                   func(s *SquadsSnapshot) { s.Squads = s.Squads[1:] },
		"s:1 named for an entity":  func(s *SquadsSnapshot) { s.Squads[0].Members[0].Entity = "p:1" },
		"s:1 with a second member": func(s *SquadsSnapshot) { s.Squads[0].Members = append(s.Squads[0].Members, s.Squads[0].Members[0]) },
		"an ordinal at next_id":    func(s *SquadsSnapshot) { s.NextID = 4 },
		"an id off its ordinal":    func(s *SquadsSnapshot) { s.Squads[1].ID = "s:7" },
		"a squad twice":            func(s *SquadsSnapshot) { s.Squads[2] = s.Squads[1] },
		"selected nowhere":         func(s *SquadsSnapshot) { s.Selected = "s:3" },
		"food past full":           func(s *SquadsSnapshot) { s.Squads[1].Meters.Food = 101 },
		"a whole point owed":       func(s *SquadsSnapshot) { s.Squads[1].Meters.Damage = 1 },
		"no activity":              func(s *SquadsSnapshot) { s.Squads[1].Meters.Activity = "" },
		"a model past its max":     func(s *SquadsSnapshot) { s.Squads[1].Members[0].Health = 51 },
		"a model with no entity":   func(s *SquadsSnapshot) { s.Squads[1].Members[0].Entity = "" },
	} {
		var snap SquadsSnapshot

		raw, _ := json.Marshal(good)
		require.NoError(t, json.Unmarshal(raw, &snap))
		bad(&snap)

		_, _, err := b2aSquadsCopy(t, ClockSnapshot{}, snap, 80, true)
		require.Error(t, err, name)
	}

	// And into an owner that already has a deployed squad: its model would be
	// left standing with no squad.
	require.Error(t, s.Restore(good), "a restore into a used owner is refused")
}

func TestSquadsSnapshotEveryFieldIsLoadBearing(t *testing.T) {
	c, s, body := b2aSquadsWorld(t)
	clockSnap, snap, health := c.Snapshot(), s.Snapshot(), body.health
	ref := b2aSquadsSteps(t, c, s)

	b2aSweep(t, snap, ref, nil, func(raw []byte) (string, error) {
		var v SquadsSnapshot
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", err
		}

		c2, s2, err := b2aSquadsCopy(t, clockSnap, v, health, false)
		if err != nil {
			return "", err
		}

		return b2aSquadsSteps(t, c2, s2), nil
	})
}
