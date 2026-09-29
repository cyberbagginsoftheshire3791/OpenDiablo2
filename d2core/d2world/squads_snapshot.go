package d2world

import (
	"fmt"
)

// SquadsSnapshot is the squads' saved state (M4.6 B2a): every squad in ordinal
// order, each with its members and its meters, plus the number the next squad
// will take and which squad is selected.
//
// Not saved, because they are derived: each meters' conditioning (his
// talents, applied again when the load applies his progress) and the body the
// meters spend (s:1's is the player's, bound by BindPlayer; a deployed squad's
// is its front model, bound here).
type SquadsSnapshot struct {
	NextID   int             `json:"next_id"`
	Selected string          `json:"selected"`
	Squads   []SquadSnapshot `json:"squads"`
}

// SquadSnapshot is one squad.
//
// s:1 is the player's squad, and its one member is the player: its entity is
// written as the word "player" (plan section 2: the player's id is his
// connection's and new every launch) and its health and max as 0, because the
// player's health lives in his body, not here. A deployed squad's members are
// map entities, written by their plain ids: B2b rebuilds every entity with its
// saved id, so no resolver stands between the two.
type SquadSnapshot struct {
	ID      string                `json:"id"`
	Owner   string                `json:"owner"`
	Ordinal int                   `json:"ordinal"`
	Morale  float64               `json:"morale"`
	Members []SquadMemberSnapshot `json:"members"`
	Meters  MetersSnapshot        `json:"meters"`
}

// SquadMemberSnapshot is one model.
type SquadMemberSnapshot struct {
	Entity string `json:"entity"`
	Health int    `json:"health"`
	Max    int    `json:"max"`
	Order  int    `json:"order"`
}

// MetersSnapshot is one squad's survival meters. Damage is the fraction of a
// health point neglect owes between steps: dropped, a starving man bleeds his
// next point late.
type MetersSnapshot struct {
	Food     float64  `json:"food"`
	Water    float64  `json:"water"`
	Fatigue  float64  `json:"fatigue"`
	Activity Activity `json:"activity"`
	Damage   float64  `json:"damage"`
}

// playerSquad is the player's squad: born with the owner, never removed.
const playerSquad = "s:1"

// Snapshot is the squads as they stand.
func (s *Squads) Snapshot() SquadsSnapshot {
	out := SquadsSnapshot{NextID: s.nextID, Selected: s.selected, Squads: make([]SquadSnapshot, 0, len(s.squads))}

	for _, id := range s.squadIDs() {
		sq := s.squads[id]
		snap := SquadSnapshot{
			ID: sq.id, Owner: sq.owner, Ordinal: sq.ordinal, Morale: sq.morale,
			Members: make([]SquadMemberSnapshot, 0, len(sq.members)),
			Meters:  sq.meters.snapshot(),
		}

		for _, m := range sq.members {
			member := SquadMemberSnapshot{Entity: m.entity, Health: m.health, Max: m.max, Order: m.order}
			if sq.id == playerSquad {
				member.Entity = PlayerRef
			}

			snap.Members = append(snap.Members, member)
		}

		out.Squads = append(out.Squads, snap)
	}

	return out
}

func (m *Meters) snapshot() MetersSnapshot {
	return MetersSnapshot{Food: m.food, Water: m.water, Fatigue: m.fatigue, Activity: m.activity, Damage: m.damage}
}

// Restore puts the saved squads back. It is checked whole before anything
// changes, so a refused snapshot leaves the squads as they were.
//
// IT RESTORES INTO A FRESH OWNER: one holding only s:1, as NewSquads builds it.
// A deployed squad's models are map entities, and replacing a squad that has
// them would leave them standing with no squad; so a Restore into an owner
// with a second squad is refused rather than guessed at.
//
// s:1 IS RESTORED IN PLACE. The game holds s:1's *Meters as its own meters
// (game.meters = PlayerMeters()), and that meters carries his conditioning,
// so its values are written into the same object rather than replaced. Its
// binding is kept too: if BindPlayer has already run, s:1 keeps the body and
// the entity it was bound to; if not, it waits for BindPlayer (the game's
// first frame with a player), which is how a new game binds it as well.
//
// Every deployed squad gets new meters spending its front model, exactly as
// addSquad builds one; its models' entities must be on the map with their
// saved ids (B2b's seam, B4's order).
//
// DEPLOYED SQUADS ARE B4b's (D3, 28 Sep 2026). A deployed squad's models are
// map entities, rebuilt with their saved ids through the next-id seam, which
// is the hunted-night load's. The quiet-evening load (B4a) restores s:1 only:
// it refuses a snapshot holding any deployed squad -- a model it cannot
// rebuild -- until B4b brings the seam to the load. This Restore does not
// check that the models' entities exist (it has no Resolver); B4b's order
// rebuilds them before it.
func (s *Squads) Restore(snap SquadsSnapshot) error {
	if err := s.Validate(snap); err != nil {
		return err
	}

	player := s.squads[playerSquad]
	squads := make(map[string]*squad, len(snap.Squads))

	for _, ss := range snap.Squads {
		if ss.ID == playerSquad {
			player.owner, player.ordinal, player.morale = ss.Owner, ss.Ordinal, ss.Morale
			player.meters.restore(ss.Meters)

			m := ss.Members[0]
			player.members[0].health, player.members[0].max, player.members[0].order = m.Health, m.Max, m.Order

			squads[ss.ID] = player

			continue
		}

		sq := &squad{
			id: ss.ID, owner: ss.Owner, ordinal: ss.Ordinal, morale: ss.Morale,
			meters:  newMeters(s.clock, s.dials),
			members: make([]*model, 0, len(ss.Members)),
		}

		for _, m := range ss.Members {
			sq.members = append(sq.members, &model{entity: m.Entity, health: m.Health, max: m.Max, order: m.Order})
		}

		sq.meters.restore(ss.Meters)
		sq.meters.SetBody(frontModelBody{sq: sq})
		squads[ss.ID] = sq
	}

	s.squads, s.nextID, s.selected = squads, snap.NextID, snap.Selected

	return nil
}

func (m *Meters) restore(s MetersSnapshot) {
	m.food, m.water, m.fatigue, m.activity, m.damage = s.Food, s.Water, s.Fatigue, s.Activity, s.Damage
}

// Validate is Restore's check and nothing else (D4): a fresh owner, s:1
// first and the player's alone, squads in ordinal order below next_id, a
// selected squad that is saved, meters in range -- and every model's entity in
// ONE squad (the B2a review's B2): a map entity is one man, and a model is in
// one squad at a time.
func (s *Squads) Validate(snap SquadsSnapshot) error {
	player, ok := s.squads[playerSquad]
	if !ok || len(s.squads) != 1 || len(player.members) != 1 {
		return fmt.Errorf("squads snapshot: restore into a fresh owner holding only %s; this one holds %d squad(s)",
			playerSquad, len(s.squads))
	}

	return s.CheckSnapshot(snap)
}

// CheckSnapshot is every check Validate makes of the snapshot itself --
// everything but its refusal of a owner already in use. Game.SaveWorld runs it on
// the snapshot it has just taken from this live owner, so a save never writes a
// block the load's own Validate would refuse (the M4.6 B3 review, B2: "strict
// at save" stopped at the file's own checks). Validate is the in-use refusal
// and this, so the two cannot disagree.
func (s *Squads) CheckSnapshot(snap SquadsSnapshot) error {
	if len(snap.Squads) == 0 || snap.Squads[0].ID != playerSquad {
		return fmt.Errorf("squads snapshot: the first squad must be %s, the player's", playerSquad)
	}

	seen := map[string]bool{}
	models := map[string]string{}
	last := 0

	for _, ss := range snap.Squads {
		if err := checkSquadSnapshot(ss, last, snap.NextID); err != nil {
			return err
		}

		if ss.ID != playerSquad {
			for _, m := range ss.Members {
				if other, dup := models[m.Entity]; dup {
					return fmt.Errorf("squads snapshot: entity %s is a model of %s and of %s; a man is in one squad",
						m.Entity, other, ss.ID)
				}

				models[m.Entity] = ss.ID
			}
		}

		seen[ss.ID] = true
		last = ss.Ordinal
	}

	if !seen[snap.Selected] {
		return fmt.Errorf("squads snapshot: selected %q is not a saved squad", snap.Selected)
	}

	return nil
}

func checkSquadSnapshot(ss SquadSnapshot, lastOrdinal, nextID int) error {
	switch {
	case ss.Ordinal <= lastOrdinal:
		return fmt.Errorf("squads snapshot: %s has ordinal %d after %d; squads are kept in ordinal order", ss.ID, ss.Ordinal, lastOrdinal)
	case ss.Ordinal >= nextID:
		return fmt.Errorf("squads snapshot: %s's ordinal %d is not below next_id %d; a new squad would reuse it", ss.ID, ss.Ordinal, nextID)
	case ss.ID != fmt.Sprintf("s:%d", ss.Ordinal):
		return fmt.Errorf("squads snapshot: squad %q has ordinal %d; its id is s:<ordinal>", ss.ID, ss.Ordinal)
	case ss.Owner == "":
		return fmt.Errorf("squads snapshot: %s has no owner", ss.ID)
	case len(ss.Members) == 0:
		return fmt.Errorf("squads snapshot: %s has no members", ss.ID)
	case !b2aFinite(ss.Morale):
		return fmt.Errorf("squads snapshot: %s's morale is not a number", ss.ID)
	}

	if err := checkMetersSnapshot(ss.ID, ss.Meters); err != nil {
		return err
	}

	if ss.ID == playerSquad {
		m := ss.Members
		if len(m) != 1 || m[0].Entity != PlayerRef || m[0].Health != 0 || m[0].Max != 0 || m[0].Order != 0 {
			return fmt.Errorf("squads snapshot: %s is the player alone -- one member, entity %q, health and max 0 "+
				"(his health is his body's), order 0; got %+v", playerSquad, PlayerRef, m)
		}

		return nil
	}

	for _, m := range ss.Members {
		if m.Entity == "" || m.Entity == PlayerRef || m.Max < 1 || m.Health < 0 || m.Health > m.Max {
			return fmt.Errorf("squads snapshot: %s has a model %+v that is not a map entity with health in [0, max]", ss.ID, m)
		}
	}

	return nil
}

func checkMetersSnapshot(id string, m MetersSnapshot) error {
	for name, v := range map[string]float64{"food": m.Food, "water": m.Water, "fatigue": m.Fatigue} {
		if !b2aFinite(v) || v < 0 || v > meterFull {
			return fmt.Errorf("squads snapshot: %s's %s is %v, outside 0-100", id, name, v)
		}
	}

	switch m.Activity {
	case ActivityIdle, ActivityLabour, ActivityWatch, ActivityForage:
	default:
		return fmt.Errorf("squads snapshot: %s's activity %q is none of idle, labour, watch, forage", id, m.Activity)
	}

	if !b2aFinite(m.Damage) || m.Damage < 0 || m.Damage >= 1 {
		return fmt.Errorf("squads snapshot: %s owes %v of a health point; neglect carries a fraction in [0, 1)", id, m.Damage)
	}

	return nil
}
