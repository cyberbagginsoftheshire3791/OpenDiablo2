package d2world

import (
	"fmt"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// Squads is the owner M4.4c-1 adds: the player commands SQUADS, not a lone
// hero (11 Sep ruling; Squads-on-Screen brief). The unit of every meter and
// every overhead bar is the squad, DoW2-style, with a member count; a named
// man is a squad of one. This type holds N squads, each with its own *Meters,
// and IS the single registered "meters" harness provider -- its flat scalar
// face keeps reporting and setting the SELECTED squad (s:1 in every shipped
// build), so M4.2 and M4.5's regression surface is untouched, and a squads[]
// array is added beside it (brief section 4.3).
//
// It also answers Combat's Fitness lookups: with squads on the map, "the
// player's fatigue" is a lookup by entity id, not a field (clause 4).
//
// Not safe for concurrent use; it lives on the game goroutine, like Meters.
type Squads struct {
	clock    *Clock
	dials    MeterDials
	deployer Deployer

	squads   map[string]*squad
	nextID   int
	selected string
}

// FitnessSource is how Combat looks up a combatant's Fitness by id (clause 4).
// It replaces the singular Fitness the resolver held. The Squads owner
// implements it; for the player's squad of one, the id is the player's entity
// id and the answer is that squad's meters -- exactly what Combat read as its
// single Fitness before squads existed.
type FitnessSource interface {
	FitnessOf(id string) Fitness
}

// Deployer creates and removes the map entity a squad MODEL is (ruled ask
// 10(b): a model is a real thing on the map that can be drawn, positioned and
// killed individually). d2world cannot import the map engine, so the game
// screen provides this the way it provides Spawner -- the fence is in the type
// system. It is nil-legal: a Squads owner with no Deployer holds s:1 (whose
// model is the player, already on the map, no entity created) but cannot
// deploy a second squad.
type Deployer interface {
	// Deploy places one model entity near (x, y) in world tiles and returns
	// its entity id and full health. ok is false if nothing was placed.
	Deploy(x, y float64) (id string, maxHealth int, ok bool)
	// Recall removes a model entity previously deployed.
	Recall(id string) bool
}

// model is one member of a squad, and ruled ask 10(b) makes it a real thing on
// the map: it carries an ENTITY ID (localPlayer's for s:1's model). health/max
// are stored for a DEPLOYED model; for the player's squad they live in the
// bound body (playerBody, the sole writer of Stats.Health) and these are unread.
// order is the draw-down order -- neglect and combat spend the front model
// first. No named-man flag (ruled ask 8: a named man is never a model inside
// someone else's squad).
type model struct {
	entity string
	health int
	max    int
	order  int
}

// squad is one squad: an id, an owner, its own non-registering meters, its
// members, and the morale ruled ask 10 gives every player squad. Its stance is
// the meters' Activity. extBody is set for the player's squad (s:1): its health
// lives in the bound body, so the globe, the resolver and neglect all read one
// field. When nil (a deployed squad), the squad owns its models' health and the
// meters drain the front model through frontModelBody.
type squad struct {
	id      string
	owner   string
	ordinal int
	meters  *Meters
	extBody Body
	members []*model
	morale  float64
}

// defaultSquadMorale is where a player squad's morale starts. It is carried as
// STATE and reported; nothing drives it in c-1 (section 7 fences the player-side
// rout consequence out -- what the player loses control of when a squad breaks
// has no rule and is campaign-scope).
const defaultSquadMorale = 100.0

// NewSquads creates the owner with the player's squad s:1 bound and selected,
// and registers it as the single "meters" provider. s:1's one model is the
// player: no entity is created and no RNG is drawn (brief section 3.1), so a
// shipped build is byte-for-byte the old single-meters world with a squads[]
// view added. Every per-squad *Meters is built by the non-registering
// constructor; the owner holds the one Register call, and Close unregisters it.
func NewSquads(clock *Clock, dials MeterDials, deployer Deployer) *Squads {
	s := &Squads{
		clock:    clock,
		dials:    dials,
		deployer: deployer,
		squads:   map[string]*squad{},
		nextID:   1,
	}

	first := s.newSquad("player")
	s.selected = first.id // "s:1"

	d2harness.Register(s)

	return s
}

func (s *Squads) newSquad(owner string) *squad {
	id := fmt.Sprintf("s:%d", s.nextID)
	sq := &squad{
		id:      id,
		owner:   owner,
		ordinal: s.nextID,
		meters:  newMeters(s.clock, s.dials),
		morale:  defaultSquadMorale,
		members: []*model{{order: 0}},
	}

	s.squads[id] = sq
	s.nextID++

	return sq
}

// Close unregisters the provider.
func (s *Squads) Close() { d2harness.Unregister(s) }

// PlayerMeters returns s:1's meters, for the game screen's internal edges (the
// fighting-activity write) that address the player's own squad by name. Nil
// before construction of s:1, which cannot happen.
func (s *Squads) PlayerMeters() *Meters {
	if sq := s.squads["s:1"]; sq != nil {
		return sq.meters
	}

	return nil
}

// BindPlayer binds the player's squad s:1 to the player's body and entity id --
// the metersBodied latch's one-shot job (Game.advanceWorld), mirroring how the
// single meters took its body. The health lives in the bound body (playerBody,
// the sole writer of Stats.Health), so s:1 stores no health of its own and the
// globe, the resolver and neglect cannot drift.
func (s *Squads) BindPlayer(body Body, entityID string) {
	sq := s.squads["s:1"]
	if sq == nil {
		return
	}

	sq.extBody = body
	sq.meters.SetBody(body)

	if len(sq.members) > 0 {
		sq.members[0].entity = entityID
	}
}

// Advance drains every squad's meters by the world minutes that just passed,
// so a second squad drains INDEPENDENTLY of the player's (brief section 10's
// N-path). Ordered by ordinal so a stepped world is reproducible.
func (s *Squads) Advance(worldMinutes float64) {
	for _, id := range s.squadIDs() {
		s.squads[id].meters.Advance(worldMinutes)
	}
}

// squadIDs returns the squad ids in a deterministic ordinal order -- the
// spawns.groupIDs idiom (spawns.go). Every list is ordered: an assertion that
// reads element 0 must read the same one on every run, and determinism_test.go
// diverges on parts["systems"] otherwise (brief section 6.6).
func (s *Squads) squadIDs() []string {
	ids := make([]string, 0, len(s.squads))
	for id := range s.squads {
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		return s.squads[ids[i]].ordinal < s.squads[ids[j]].ordinal
	})

	return ids
}

// FitnessOf returns the Fitness of the squad whose model carries the given
// entity id, or nil if no squad owns it. For s:1 the player's entity id maps to
// the player's squad meters, which is what Combat read as its single Fitness
// before squads existed. A nil answer is handled by the resolver exactly as a
// nil fitness always was.
func (s *Squads) FitnessOf(id string) Fitness {
	if id == "" {
		return nil
	}

	for _, sq := range s.squads {
		for _, m := range sq.members {
			if m.entity == id {
				return sq.meters
			}
		}
	}

	return nil
}

// Selected returns the selected squad id.
func (s *Squads) Selected() string { return s.selected }

// SetSelected selects a squad by id, reporting whether it exists.
func (s *Squads) SetSelected(id string) bool {
	if _, ok := s.squads[id]; !ok {
		return false
	}

	s.selected = id

	return true
}

// SquadOf returns the squad id owning the given model entity id, or "" if none.
func (s *Squads) SquadOf(entityID string) string {
	if entityID == "" {
		return ""
	}

	for _, id := range s.squadIDs() {
		for _, m := range s.squads[id].members {
			if m.entity == entityID {
				return id
			}
		}
	}

	return ""
}

// front returns the front living model of a squad (lowest order with health
// left), the one neglect and combat draw down (brief section 4.3). For the
// player's squad the health is in the bound body, so its one model is always
// the front.
func (sq *squad) front() *model {
	if len(sq.members) == 0 {
		return nil
	}

	if sq.extBody != nil {
		return sq.members[0]
	}

	var best *model

	for _, m := range sq.members {
		if m.health <= 0 {
			continue
		}

		if best == nil || m.order < best.order {
			best = m
		}
	}

	return best
}

// poolHealth is the squad's living health -- the pool the overhead bar
// displays. For the player's squad it is the bound body (one field); for a
// deployed squad it is the sum of the living models.
func (sq *squad) poolHealth() int {
	if sq.extBody != nil {
		return sq.extBody.CurrentHealth()
	}

	total := 0

	for _, m := range sq.members {
		if m.health > 0 {
			total += m.health
		}
	}

	return total
}

// rosterMax is the roster maximum, which DOES NOT SHRINK when a model dies
// (brief section 4): the bar falls as health is lost and never jumps up.
func (sq *squad) rosterMax() int {
	if sq.extBody != nil {
		return sq.extBody.MaxHealth()
	}

	total := 0

	for _, m := range sq.members {
		total += m.max
	}

	return total
}

// frontModelBody adapts a deployed squad's front living model to the meters
// Body interface: neglect and the resolver spend whole points from the front
// model first (brief section 4.3). A squad of N bleeds one model at a time
// rather than dividing a point N ways, which would round every model to zero
// and never bleed.
type frontModelBody struct{ sq *squad }

func (b frontModelBody) CurrentHealth() int {
	if m := b.sq.front(); m != nil {
		return m.health
	}

	return 0
}

func (b frontModelBody) MaxHealth() int {
	if m := b.sq.front(); m != nil {
		return m.max
	}

	return 0
}

func (b frontModelBody) SetHealth(h int) {
	if m := b.sq.front(); m != nil {
		m.health = h
	}
}

// addSquad deploys a new squad with one model near (x, y). Ruled ask 10(b): the
// model is a real map entity, so this draws the world RNG by construction --
// which is why the N-path asserts on bodies_known, never on world_draws
// (unreadable, hashed). Returns the new squad id.
func (s *Squads) addSquad(x, y float64) (string, error) {
	if s.deployer == nil {
		return "", fmt.Errorf("no deployer wired; a squad model cannot be placed on the map")
	}

	id, max, ok := s.deployer.Deploy(x, y)
	if !ok || id == "" {
		return "", fmt.Errorf("deploy of a squad model failed at (%v, %v)", x, y)
	}

	sq := s.newSquad("player")
	sq.members[0].entity = id
	sq.members[0].health = max
	sq.members[0].max = max
	sq.meters.SetBody(frontModelBody{sq: sq}) // a deployed squad owns its health

	return sq.id, nil
}

// removeSquad recalls a squad's models and drops it. s:1 (the player) cannot be
// removed: it is the bound hero.
func (s *Squads) removeSquad(id string) error {
	if id == "s:1" {
		return fmt.Errorf("cannot remove the player's squad %q", id)
	}

	sq, ok := s.squads[id]
	if !ok {
		return fmt.Errorf("no live squad %q", id)
	}

	if s.deployer != nil {
		for _, m := range sq.members {
			if m.entity != "" {
				s.deployer.Recall(m.entity)
			}
		}
	}

	delete(s.squads, id)

	if s.selected == id {
		s.selected = "s:1"
	}

	return nil
}

// ------------------------------------------------- the bars and the selection --

// SquadBar is one player squad's overhead-bar data: the model entity ids to
// draw over, the POOLED health over the roster maximum, the active stage cues,
// and whether it is the selected squad. The game screen turns this into the
// HUD's bar list; the HUD projects each entity and draws it.
type SquadBar struct {
	Squad    string
	Entities []string
	Cur, Max int
	Cues     []string
	Selected bool
}

// Bars returns one SquadBar per squad, ordered, for the overhead bars.
func (s *Squads) Bars() []SquadBar {
	out := make([]SquadBar, 0, len(s.squads))

	for _, id := range s.squadIDs() {
		sq := s.squads[id]

		ents := make([]string, 0, len(sq.members))
		for _, m := range sq.members {
			if m.entity != "" {
				ents = append(ents, m.entity)
			}
		}

		out = append(out, SquadBar{
			Squad:    id,
			Entities: ents,
			Cur:      sq.poolHealth(),
			Max:      sq.rosterMax(),
			Cues:     sq.cues(),
			Selected: id == s.selected,
		})
	}

	return out
}

// cues lists the active stage cues of a squad (S1 §5, brief clause 8): hungry,
// thirsty, no-reaction, shaken, dying. These are machine keys, not display
// strings; the sheet's strings live in strigoi_strings.go and the overhead
// marks are coloured rects keyed off these.
func (sq *squad) cues() []string {
	m := sq.meters
	out := []string{}

	if m.Hungry() {
		out = append(out, "hungry")
	}

	if m.Thirsty() {
		out = append(out, "thirsty")
	}

	if !m.ReactionAvailable() {
		out = append(out, "no_reaction")
	}

	if m.Shaken() {
		out = append(out, "shaken")
	}

	if m.Dying() {
		out = append(out, "dying")
	}

	return out
}

// SquadModel pairs a model's entity id with its squad, for the selection hit
// test (which squad a clicked entity belongs to).
type SquadModel struct {
	Squad  string
	Entity string
}

// ModelEntities lists every squad model's entity id with its squad, ordered,
// for the selection hit test.
func (s *Squads) ModelEntities() []SquadModel {
	out := []SquadModel{}

	for _, id := range s.squadIDs() {
		for _, m := range s.squads[id].members {
			if m.entity != "" {
				out = append(out, SquadModel{Squad: id, Entity: m.entity})
			}
		}
	}

	return out
}

// Cycle selects the next squad in order after the current selection, wrapping.
// The cycle key (§4.5) calls it; with one squad it is a no-op.
func (s *Squads) Cycle() string {
	ids := s.squadIDs()
	if len(ids) == 0 {
		return s.selected
	}

	idx := 0

	for i, id := range ids {
		if id == s.selected {
			idx = i
			break
		}
	}

	s.selected = ids[(idx+1)%len(ids)]

	return s.selected
}

// SheetCard is one card of the selected-squad sheet (ruled ask 2/6): the
// numbers a player reads after selecting a squad -- food, water and fatigue as
// numbers, the stance, the member count, the pooled health over the roster
// maximum, and the active stage cues.
type SheetCard struct {
	Squad             string
	Food, Water       float64
	Fatigue           float64
	Stance            string
	Members           int
	Health, MaxHealth int
	Cues              []string
	Selected          bool
}

// SheetCards returns one card per squad, ordered, for the sheet.
func (s *Squads) SheetCards() []SheetCard {
	out := make([]SheetCard, 0, len(s.squads))

	for _, id := range s.squadIDs() {
		sq := s.squads[id]
		out = append(out, SheetCard{
			Squad:     id,
			Food:      sq.meters.Food(),
			Water:     sq.meters.Water(),
			Fatigue:   sq.meters.Fatigue(),
			Stance:    string(sq.meters.Activity()),
			Members:   len(sq.members),
			Health:    sq.poolHealth(),
			MaxHealth: sq.rosterMax(),
			Cues:      sq.cues(),
			Selected:  id == s.selected,
		})
	}

	return out
}

// ------------------------------------------------------------- the provider --

// HarnessName identifies the provider. It is "meters", the SAME name the single
// Meters registered under, so the flat scalar face (below) keeps every M4.2 and
// M4.5 assertion honest without a rename (brief section 4.3).
func (s *Squads) HarnessName() string { return "meters" }

// HarnessState reports the flat scalar face of the SELECTED squad (unchanged in
// meaning -- every regression test leaves selection at s:1, the player's), and
// adds the collection face beside it: squads[] (ordered), squad_count, selected
// (clauses 1, 3, 5).
func (s *Squads) HarnessState() map[string]interface{} {
	sel := s.squads[s.selected]
	if sel == nil {
		sel = s.squads["s:1"]
	}

	// The flat face: the selected squad's meters, reported exactly as the
	// single meters reported. flat health/max_health are the bound body
	// (ruled ask 1a); the pool lives in squads[i].health.
	state := sel.meters.HarnessState()

	squads := make([]map[string]interface{}, 0, len(s.squads))
	for _, id := range s.squadIDs() {
		squads = append(squads, s.squads[id].report())
	}

	state["squads"] = squads
	state["squad_count"] = len(s.squads)
	state["selected"] = s.selected

	return state
}

// report is one squad's entry in squads[]: the same scalar meter keys, plus the
// squad's id/owner/morale/member count, the POOLED health over the roster
// maximum, the stance, and the per-model array (each with its entity id).
func (sq *squad) report() map[string]interface{} {
	r := sq.meters.HarnessState()
	r["squad"] = sq.id
	r["owner"] = sq.owner
	r["morale"] = sq.morale
	r["members"] = len(sq.members)
	r["health"] = sq.poolHealth()
	r["max_health"] = sq.rosterMax()
	r["stance"] = string(sq.meters.Activity())

	models := make([]map[string]interface{}, 0, len(sq.members))
	for _, m := range sq.members {
		models = append(models, sq.modelReport(m))
	}

	r["models"] = models

	return r
}

func (sq *squad) modelReport(m *model) map[string]interface{} {
	health, max := m.health, m.max
	if sq.extBody != nil {
		// s:1's one model reads the bound body -- one field, no divergence.
		health, max = sq.extBody.CurrentHealth(), sq.extBody.MaxHealth()
	}

	return map[string]interface{}{
		"entity":     m.entity,
		"health":     health,
		"max_health": max,
		"order":      m.order,
	}
}

// HarnessSettableFields lists the writes the provider allows. The five flat
// meter fields keep writing to the SELECTED squad, so all six M4.2/M4.5 write
// sites keep working; selected/squad/squad_add/squad_remove are the collection
// verbs (ruled ask 4; clause 2's provider-rule-1 add/remove). They are settable
// FIELDS, not new harness tools, so the tool count stays 36.
func (s *Squads) HarnessSettableFields() []string {
	return []string{
		"activity", "consume", "fatigue", "food", "health", "water",
		"selected", "squad", "squad_add", "squad_remove",
	}
}

// HarnessSet writes one allow-listed field. The flat fields delegate to the
// selected squad's meters; the collection verbs act on the owner.
func (s *Squads) HarnessSet(field string, value interface{}) error {
	switch field {
	case "activity", "consume", "fatigue", "food", "health", "water":
		sel := s.squads[s.selected]
		if sel == nil {
			return fmt.Errorf("no selected squad %q", s.selected)
		}

		return sel.meters.HarnessSet(field, value)

	case "selected":
		id, ok := value.(string)
		if !ok {
			return fmt.Errorf(`selected wants a squad id string like "s:1", got %T`, value)
		}

		if !s.SetSelected(id) {
			return fmt.Errorf("no live squad %q", id)
		}

		return nil

	case "squad":
		return s.setSquadField(value)

	case "squad_add":
		x, y := deployXY(value)

		_, err := s.addSquad(x, y)

		return err

	case "squad_remove":
		id, ok := value.(string)
		if !ok {
			return fmt.Errorf(`squad_remove wants a squad id string like "s:2", got %T`, value)
		}

		return s.removeSquad(id)

	default:
		return fmt.Errorf("no settable field %q", field)
	}
}

// setSquadField is the per-squad object verb, on the spawns.setMorale pattern:
// {"squad": "s:1", "field": "water", "value": 12}. It writes one meter of one
// named squad, so a script can drain a squad that is not selected.
func (s *Squads) setSquadField(value interface{}) error {
	obj, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf(`squad wants an object {"squad":"s:1","field":"water","value":12}, got %T`, value)
	}

	id, ok := obj["squad"].(string)
	if !ok {
		return fmt.Errorf(`squad wants a "squad" id string like "s:1"`)
	}

	sq, ok := s.squads[id]
	if !ok {
		return fmt.Errorf("no live squad %q", id)
	}

	f, ok := obj["field"].(string)
	if !ok {
		return fmt.Errorf(`squad wants a "field" string (food, water, fatigue, activity, consume)`)
	}

	return sq.meters.HarnessSet(f, obj["value"])
}

// deployXY reads an optional {"x": .., "y": ..} in world tiles from a squad_add
// value; a missing position deploys at the origin, which the Deployer resolves
// relative to the player.
func deployXY(value interface{}) (x, y float64) {
	obj, _ := value.(map[string]interface{})
	x, _ = toFloat(obj["x"])
	y, _ = toFloat(obj["y"])

	return x, y
}
