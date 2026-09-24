// Package d2dialogue is Strigoi's talk (T4, 23 Sep 2026): data-driven
// conversations with the village and the one number the village keeps about
// him.
//
// THE SHAPE IS SIGNED, THE CONTENT IS NOT. S1 §8.2 fixes one integer --
// reputation -- that starts low-wary, has a fear FLOOR it cannot fall below and
// a CEILING the Occupier's Shadow holds down, and a ladder of what it unlocks:
// water at the well at a distance -> trade at the gate -> shelter inside the
// palisade -> the hearth -> the priest's rite. It names what moves it (their
// dead buried, the watch stood, steel and labour traded; theft and violence
// down) and that terrorising the village is possible and priced: the number
// drops to the floor and the gate closes for the run. Every threshold, every
// line and every villager is SILENT in the documents, and S1 §8.1 keeps all
// names placeholders by standing decision -- so the speakers here are ROLES
// ("the headman"), each drawn by a D2 stand-in sprite, and every number is a
// [DIAL] in data/strigoi/dialogue.json.
//
// It is a LEAF package: no ebiten, no world, no RNG. The game applies what a
// choice returns (food, water, time) to the systems that own them.
package d2dialogue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Rung is one step of the ladder: what reputation At or above unlocks.
type Rung struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	At   int    `json:"at"`
}

// Village is the number's rules.
type Village struct {
	Start   int    `json:"start"`
	Floor   int    `json:"floor"`
	Ceiling int    `json:"ceiling"`
	Ladder  []Rung `json:"ladder"`

	// Watch is what a watch promised by day and STOOD is worth (S1 §8.2:
	// "standing the palisade watch").
	Watch int `json:"watch"`

	// T8: a watch is stood, not survived. WatchMinutes is how much of the
	// night he must spend within WatchRadius tiles of the headman; a promise
	// broken costs BrokenWatch. All [DIAL]s.
	WatchMinutes float64 `json:"watch_minutes"`
	WatchRadius  float64 `json:"watch_radius"`
	BrokenWatch  int     `json:"broken_watch"`

	// M4.7 step 4. RiteRadius is how near the church (the priest's sprite
	// until there is one) a hasty grave must lie for the granted rite to
	// close it at first light (Q4a); a staking by day within SeenRadius of
	// any villager costs SeenCost standing, the first time (S1 §8.2). [DIAL]s.
	RiteRadius float64 `json:"rite_radius"`
	SeenRadius float64 `json:"seen_radius"`
	SeenCost   int     `json:"seen_cost"`
}

// Requires gates an opening or a choice. Every field set must hold.
type Requires struct {
	Rung    string `json:"rung,omitempty"`     // this rung reached
	Below   string `json:"below,omitempty"`    // this rung NOT reached
	Flag    string `json:"flag,omitempty"`     // this flag set
	NotFlag string `json:"not_flag,omitempty"` // this flag not set

	// NotFlags is every flag that must NOT be set, when one is not enough
	// (T6: no second sleep in a night, and none on a promised watch).
	NotFlags []string `json:"not_flags,omitempty"`
	Time     string   `json:"time,omitempty"` // "day" or "night"
}

// none reports a requirement that always holds.
func (q Requires) none() bool {
	return q.Rung == "" && q.Below == "" && q.Flag == "" && q.NotFlag == "" && len(q.NotFlags) == 0 && q.Time == ""
}

// Effects is what a choice does. Rep, Set, Clear and ToFloor act on the
// standing here; Food, Water and Minutes are returned for the game to apply.
type Effects struct {
	Rep     int      `json:"rep,omitempty"`
	ToFloor bool     `json:"to_floor,omitempty"`
	Set     []string `json:"set,omitempty"`
	Clear   []string `json:"clear,omitempty"`
	Food    float64  `json:"food,omitempty"`
	Water   float64  `json:"water,omitempty"`
	Minutes float64  `json:"minutes,omitempty"`

	// T5: barter in goods (S1 §8.3 -- no coin). Give puts items in his pack;
	// Mend has the smith restore the armour worn in a slot to new. The game
	// checks both against the item catalogue at load.
	Give map[string]int `json:"give,omitempty"`
	Mend string         `json:"mend,omitempty"`

	// Rest takes fatigue off (the meters' "rest"), and Shelter says the
	// answer's minutes pass inside the palisade: no pack arrives, nothing new
	// notices him. Together they are what the shelter rung MEANS.
	Rest    float64 `json:"rest,omitempty"`
	Shelter bool    `json:"shelter,omitempty"`
}

// Choice is one thing he can say.
type Choice struct {
	Text     string   `json:"text"`
	Next     string   `json:"next,omitempty"` // "" ends the talk
	Requires Requires `json:"requires,omitempty"`
	Effects  Effects  `json:"effects,omitempty"`
}

// Node is one thing said to him, and his answers.
type Node struct {
	Text    string   `json:"text"`
	Choices []Choice `json:"choices"`
}

// Opening is where a talk with a speaker starts: the first whose
// requirements hold.
type Opening struct {
	Requires Requires `json:"requires,omitempty"`
	Node     string   `json:"node"`
}

// Speaker is one villager -- a ROLE, drawn by a stand-in sprite.
type Speaker struct {
	ID       string    `json:"id"`
	Role     string    `json:"role"`
	StandIn  string    `json:"stand_in"` // the D2 NPC's label that draws him
	Openings []Opening `json:"openings"`
}

// MaxAnswers is the most choices a node may offer: the talk panel draws this
// many, and keys 1-9 must never reach one it did not draw.
const MaxAnswers = 6

// Book is the validated dialogue table.
type Book struct {
	Village  Village          `json:"village"`
	Speakers []Speaker        `json:"speakers"`
	Nodes    map[string]*Node `json:"nodes"`

	rungs   map[string]int
	byStand map[string]*Speaker
	byID    map[string]*Speaker
}

// Load parses and validates a dialogue table. It refuses anything a talk
// could trip over mid-conversation: a missing node, an unknown rung, a
// speaker with no way in, a ladder above the ceiling.
func Load(data []byte) (*Book, error) {
	var b Book

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("decode dialogue: %w", err)
	}

	v := b.Village
	if v.Floor > v.Start || v.Start > v.Ceiling {
		return nil, fmt.Errorf("the village needs floor <= start <= ceiling; got %d, %d, %d", v.Floor, v.Start, v.Ceiling)
	}

	if v.Watch < 0 || v.BrokenWatch < 0 {
		return nil, fmt.Errorf("watch and broken_watch cannot be negative")
	}

	// Missing, these load as 0 -- and a zero watch_minutes pays every promise
	// (T4's old rule) while a zero radius keeps none (review finding).
	if v.WatchMinutes <= 0 || v.WatchRadius <= 0 {
		return nil, fmt.Errorf("watch_minutes and watch_radius must be > 0")
	}

	// Missing, these load as 0: a rite that closes nothing and a staking no
	// one ever sees -- both silent, so both refused.
	if v.RiteRadius <= 0 || v.SeenRadius <= 0 || v.SeenCost < 0 {
		return nil, fmt.Errorf("rite_radius and seen_radius must be > 0 and seen_cost >= 0")
	}

	b.rungs = map[string]int{}

	for i, r := range v.Ladder {
		if r.ID == "" || r.Name == "" {
			return nil, fmt.Errorf("rung %d needs an id and a name", i)
		}

		if _, dup := b.rungs[r.ID]; dup {
			return nil, fmt.Errorf("two rungs named %q", r.ID)
		}

		if i > 0 && r.At <= v.Ladder[i-1].At {
			return nil, fmt.Errorf("the ladder must rise: %s at %d after %d", r.ID, r.At, v.Ladder[i-1].At)
		}

		// A rung above the ceiling could never be reached: the Occupier's
		// Shadow would be a wall, not a ceiling.
		if r.At > v.Ceiling {
			return nil, fmt.Errorf("rung %s at %d is above the ceiling %d", r.ID, r.At, v.Ceiling)
		}

		b.rungs[r.ID] = r.At
	}

	checkReq := func(where string, q Requires) error {
		for _, id := range []string{q.Rung, q.Below} {
			if _, ok := b.rungs[id]; id != "" && !ok {
				return fmt.Errorf("%s: no rung %q", where, id)
			}
		}

		if q.Time != "" && q.Time != "day" && q.Time != "night" {
			return fmt.Errorf("%s: time is day or night, not %q", where, q.Time)
		}

		return nil
	}

	// Every flag a requirement reads must be one some choice sets (or the one
	// the code sets): a misspelt flag is a door that never opens.
	setBy := map[string]bool{FlagWatch: true, FlagSleptInside: true}

	for _, n := range b.Nodes {
		if n == nil {
			continue
		}

		for _, c := range n.Choices {
			for _, f := range c.Effects.Set {
				setBy[f] = true
			}
		}
	}

	checkFlags := func(where string, q Requires) error {
		for _, f := range append([]string{q.Flag, q.NotFlag}, q.NotFlags...) {
			if f != "" && !setBy[f] {
				return fmt.Errorf("%s: no choice ever sets the flag %q", where, f)
			}
		}

		return nil
	}

	for id, n := range b.Nodes {
		// "" is how a choice says END; a node by that name would be a talk
		// that is over the moment it opens -- and holds the world.
		if id == "" {
			return nil, fmt.Errorf("a node may not have an empty id")
		}

		if n == nil || n.Text == "" {
			return nil, fmt.Errorf("node %q has no text", id)
		}

		if len(n.Choices) > MaxAnswers {
			return nil, fmt.Errorf("node %q offers %d answers; the panel draws %d", id, len(n.Choices), MaxAnswers)
		}

		for i, c := range n.Choices {
			where := fmt.Sprintf("node %q choice %d", id, i+1)

			if c.Text == "" {
				return nil, fmt.Errorf("%s has no text", where)
			}

			if e := c.Effects; e.Food < 0 || e.Water < 0 || e.Minutes < 0 || e.Rest < 0 {
				return nil, fmt.Errorf("%s: food, water, rest and minutes cannot be negative", where)
			}

			if e := c.Effects; (e.Shelter || e.Rest > 0) && e.Minutes <= 0 {
				return nil, fmt.Errorf("%s: shelter or rest take time; minutes must be > 0", where)
			}

			for item, n := range c.Effects.Give {
				if item == "" || n <= 0 {
					return nil, fmt.Errorf("%s: gives %q x%d -- counts are positive", where, item, n)
				}
			}

			if err := checkFlags(where, c.Requires); err != nil {
				return nil, err
			}

			if _, ok := b.Nodes[c.Next]; c.Next != "" && !ok {
				return nil, fmt.Errorf("%s leads to no node %q", where, c.Next)
			}

			if err := checkReq(where, c.Requires); err != nil {
				return nil, err
			}
		}
	}

	b.byStand, b.byID = map[string]*Speaker{}, map[string]*Speaker{}

	for i := range b.Speakers {
		s := &b.Speakers[i]
		if s.ID == "" || s.Role == "" || s.StandIn == "" || len(s.Openings) == 0 {
			return nil, fmt.Errorf("speaker %d needs an id, a role, a stand-in and an opening", i)
		}

		if _, dup := b.byStand[s.StandIn]; dup {
			return nil, fmt.Errorf("two speakers drawn by %q", s.StandIn)
		}

		if _, dup := b.byID[s.ID]; dup {
			return nil, fmt.Errorf("two speakers named %q", s.ID)
		}

		for j, o := range s.Openings {
			if _, ok := b.Nodes[o.Node]; !ok {
				return nil, fmt.Errorf("speaker %s opening %d: no node %q", s.ID, j+1, o.Node)
			}

			where := fmt.Sprintf("speaker %s opening %d", s.ID, j+1)

			if err := checkReq(where, o.Requires); err != nil {
				return nil, err
			}

			if err := checkFlags(where, o.Requires); err != nil {
				return nil, err
			}
		}

		// The last opening must always hold, or a talk could find no way in.
		if last := s.Openings[len(s.Openings)-1].Requires; !last.none() {
			return nil, fmt.Errorf("speaker %s: the last opening must have no requirements", s.ID)
		}

		b.byStand[s.StandIn] = s
		b.byID[s.ID] = s
	}

	return &b, nil
}

// ItemsNamed is every item a choice gives, for the game to check against its
// catalogue: a villager must not hand over something that does not exist.
func (b *Book) ItemsNamed() []string {
	seen := map[string]bool{}

	for _, n := range b.Nodes {
		for _, c := range n.Choices {
			for item := range c.Effects.Give {
				seen[item] = true
			}
		}
	}

	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}

	sort.Strings(out)

	return out
}

// SlotsNamed is every armour slot a choice mends, for the same check.
func (b *Book) SlotsNamed() []string {
	seen := map[string]bool{}

	for _, n := range b.Nodes {
		for _, c := range n.Choices {
			if c.Effects.Mend != "" {
				seen[c.Effects.Mend] = true
			}
		}
	}

	out := make([]string, 0, len(seen))
	for slot := range seen {
		out = append(out, slot)
	}

	sort.Strings(out)

	return out
}

// FlagsNamed is every flag the table can set -- each choice's "set", and the
// two the code sets for it (FlagWatch, FlagSleptInside) -- sorted: what the
// journal's conditions may read (J1). The game adds the flags it marks itself.
func (b *Book) FlagsNamed() []string {
	seen := map[string]bool{FlagWatch: true, FlagSleptInside: true}

	for _, n := range b.Nodes {
		for _, c := range n.Choices {
			for _, f := range c.Effects.Set {
				seen[f] = true
			}
		}
	}

	return sortedSet(seen)
}

// NodeIDs is every node, sorted.
func (b *Book) NodeIDs() []string {
	seen := map[string]bool{}
	for id := range b.Nodes {
		seen[id] = true
	}

	return sortedSet(seen)
}

// SpeakerIDs is every villager, in table order.
func (b *Book) SpeakerIDs() []string {
	out := make([]string, 0, len(b.Speakers))
	for _, s := range b.Speakers {
		out = append(out, s.ID)
	}

	return out
}

// RungIDs is the ladder, bottom first.
func (b *Book) RungIDs() []string {
	out := make([]string, 0, len(b.Village.Ladder))
	for _, r := range b.Village.Ladder {
		out = append(out, r.ID)
	}

	return out
}

func sortedSet(seen map[string]bool) []string {
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}

	sort.Strings(out)

	return out
}

// SpeakerFor is the villager a stand-in sprite plays, or nil.
func (b *Book) SpeakerFor(standIn string) *Speaker { return b.byStand[standIn] }

// Speaker is a villager by id, or nil.
func (b *Book) Speaker(id string) *Speaker { return b.byID[id] }

// Standing is what the village thinks of him. It is saved beside his save.
type Standing struct {
	Rep   int      `json:"rep"`
	Flags []string `json:"flags"`
}

// NewStanding is a stranger at the gate.
func (b *Book) NewStanding() *Standing { return &Standing{Rep: b.Village.Start, Flags: []string{}} }

// Normalise makes a loaded standing safe to use: flags sorted and unique
// (Has searches them), the number inside today's floor and ceiling (a table
// may have lowered the ceiling since it was saved).
func (b *Book) Normalise(s *Standing) {
	if s.Flags == nil {
		s.Flags = []string{}
	}

	sort.Strings(s.Flags)

	out := s.Flags[:0]

	for i, f := range s.Flags {
		if i == 0 || f != s.Flags[i-1] {
			out = append(out, f)
		}
	}

	s.Flags = out
	b.Move(s, 0)
}

// Has reports a flag.
func (s *Standing) Has(flag string) bool {
	i := sort.SearchStrings(s.Flags, flag)
	return i < len(s.Flags) && s.Flags[i] == flag
}

// Mark sets a flag the game raises itself rather than a choice -- a staking
// the village saw (M4.7 step 4).
func (s *Standing) Mark(flag string) { s.set(flag) }

func (s *Standing) set(flag string) {
	if s.Has(flag) {
		return
	}

	s.Flags = append(s.Flags, flag)
	sort.Strings(s.Flags)
}

func (s *Standing) clear(flag string) {
	i := sort.SearchStrings(s.Flags, flag)
	if i < len(s.Flags) && s.Flags[i] == flag {
		s.Flags = append(s.Flags[:i], s.Flags[i+1:]...)
	}
}

// Move changes the number within the floor and the ceiling.
func (b *Book) Move(s *Standing, delta int) {
	s.Rep += delta

	if s.Rep < b.Village.Floor {
		s.Rep = b.Village.Floor
	}

	if s.Rep > b.Village.Ceiling {
		s.Rep = b.Village.Ceiling
	}
}

// Rung is the highest rung reached, or the zero Rung below the first.
func (b *Book) Rung(s *Standing) Rung {
	var got Rung

	for _, r := range b.Village.Ladder {
		if s.Rep >= r.At {
			got = r
		}
	}

	return got
}

// Reached reports a rung reached.
func (b *Book) Reached(s *Standing, rung string) bool {
	at, ok := b.rungs[rung]
	return ok && s.Rep >= at
}

// Meets reports whether a requirement holds.
func (b *Book) Meets(s *Standing, q Requires, night bool) bool {
	switch {
	case q.Rung != "" && !b.Reached(s, q.Rung):
		return false
	case q.Below != "" && b.Reached(s, q.Below):
		return false
	case q.Flag != "" && !s.Has(q.Flag):
		return false
	case q.NotFlag != "" && s.Has(q.NotFlag):
		return false
	case anyHeld(s, q.NotFlags):
		return false
	case q.Time == "day" && night, q.Time == "night" && !night:
		return false
	}

	return true
}

// Talk is one conversation in progress.
type Talk struct {
	book    *Book
	st      *Standing
	night   bool
	Speaker *Speaker
	NodeID  string
}

// Refusals.
var (
	ErrNoSpeaker = errors.New("no one to talk to")
	ErrNoChoice  = errors.New("no such answer")
	ErrOver      = errors.New("the talk is over")
)

// Open starts a talk with a villager at his first opening that holds.
func (b *Book) Open(s *Standing, speakerID string, night bool) (*Talk, error) {
	sp := b.byID[speakerID]
	if sp == nil {
		return nil, ErrNoSpeaker
	}

	for _, o := range sp.Openings {
		if b.Meets(s, o.Requires, night) {
			return &Talk{book: b, st: s, night: night, Speaker: sp, NodeID: o.Node}, nil
		}
	}

	return nil, ErrNoSpeaker // unreachable: Load requires an open last opening
}

// SetNight tells a talk the stage has turned: a talk whose answers cost time
// can outlast the day it opened in.
func (t *Talk) SetNight(night bool) { t.night = night }

// Done reports a finished talk.
func (t *Talk) Done() bool { return t.NodeID == "" }

// Text is what is being said to him.
func (t *Talk) Text() string {
	if t.Done() {
		return ""
	}

	return t.book.Nodes[t.NodeID].Text
}

// Answers are the choices he can make now, in order: those whose
// requirements hold. A node with none offers only leaving.
func (t *Talk) Answers() []Choice {
	if t.Done() {
		return nil
	}

	var out []Choice

	for _, c := range t.book.Nodes[t.NodeID].Choices {
		if t.book.Meets(t.st, c.Requires, t.night) {
			out = append(out, c)
		}
	}

	return out
}

// Peek is answer i without taking it, so the game can refuse one it cannot
// honour (a mend with nothing to mend) before the standing moves.
func (t *Talk) Peek(i int) (Choice, error) {
	if t.Done() {
		return Choice{}, ErrOver
	}

	answers := t.Answers()
	if i < 0 || i >= len(answers) {
		return Choice{}, ErrNoChoice
	}

	return answers[i], nil
}

// Choose takes answer i (0-based, of Answers), applies what it does to the
// standing, moves the talk on, and returns the effects the game must apply.
func (t *Talk) Choose(i int) (Effects, error) {
	if t.Done() {
		return Effects{}, ErrOver
	}

	answers := t.Answers()
	if i < 0 || i >= len(answers) {
		return Effects{}, ErrNoChoice
	}

	c := answers[i]
	e := c.Effects

	if e.ToFloor {
		t.st.Rep = t.book.Village.Floor
	}

	t.book.Move(t.st, e.Rep)

	for _, f := range e.Set {
		t.st.set(f)
	}

	for _, f := range e.Clear {
		t.st.clear(f)
	}

	t.NodeID = c.Next

	return e, nil
}

// Leave ends the talk where it stands.
func (t *Talk) Leave() { t.NodeID = "" }

// FlagWatch is set by a talk in which he promises the night's watch; the game
// reads it at dawn (DawnWatch).
const FlagWatch = "watch_promised"

// FlagSleptInside is set by a night's sleep in the byre and cleared at dawn:
// one sleep a night, so shelter shortens a night and never skips it (T6
// review finding -- asked again and again, it skipped the whole night).
const FlagSleptInside = "slept_inside"

func anyHeld(s *Standing, flags []string) bool {
	for _, f := range flags {
		if s.Has(f) {
			return true
		}
	}

	return false
}

// DawnWatch settles the night at dawn: the byre is his to ask for again, and
// a promised watch is judged by the minutes he STOOD at the ditch -- kept, the
// village counts it; broken, it counts that too (T8: before, being alive at
// dawn was enough, wherever he had spent the night). It returns what the
// number moved by.
func (b *Book) DawnWatch(s *Standing, stood float64) int {
	s.clear(FlagSleptInside)

	if !s.Has(FlagWatch) {
		return 0
	}

	s.clear(FlagWatch)

	before := s.Rep

	if stood >= b.Village.WatchMinutes {
		b.Move(s, b.Village.Watch)
	} else {
		b.Move(s, -b.Village.BrokenWatch)
	}

	return s.Rep - before
}
