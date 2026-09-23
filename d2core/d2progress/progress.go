// Package d2progress is Strigoi's progression (T3, 23 Sep 2026): experience,
// levels, and the talent tree.
//
// THE SHAPE IS SIGNED, THE CONTENT IS NOT. U1 §6.1 and the Squads on Screen
// brief §8 fix three branches -- Endurance, Drill, Lore -- of five, [8-10]
// picks, no respec, and one rule: picks DEEPEN what he can already do rather
// than grant it (a Riposte is available from the first fight; Drill makes it
// better). Every node's effect, every XP value and every level threshold is
// SILENT in the documents, so each one here is invented, is a [DIAL], and is
// written in data/strigoi/talents.json where it can be changed without code.
//
// Each effect is wired to a system that already runs -- the meters, his body,
// the light, the notice model, the resolver -- so no talent is decorative.
//
// It is a LEAF package: no ebiten, no world, no RNG.
package d2progress

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Effect keys a node may carry. MULTIPLIERS combine by product and default to
// 1; everything else adds and defaults to 0.
const (
	FatigueRate       = "fatigue_rate"        // x: fatigue gained per hour
	FoodWaterRate     = "food_water_rate"     // x: food and water spent per hour
	MaxHealth         = "max_health"          // +: to his maximum health
	ShakenFatigue     = "shaken_fatigue"      // +: fatigue before Shaken sets in
	NoReactionFatigue = "no_reaction_fatigue" // +: fatigue before the Reaction is lost
	RiposteDamage     = "riposte_damage"      // x: a riposte's damage
	BlocksPerRound    = "blocks_per_round"    // +: blows the shield turns a round, beyond one
	MoveTiles         = "move_tiles"          // +: tiles in his Move
	CritBand          = "crit_band"           // +: his blows' crit band
	Reactions         = "reactions"           // +: Reactions a round, beyond one
	AdvantageBonus    = "advantage_bonus"     // +: to his dark-into-light advantage
	TorchBurnRate     = "torch_burn_rate"     // x: his carried torch's burn
	KillNerve         = "kill_nerve"          // x: nerve a pack loses for a death he dealt
	NoticeRadius      = "notice_radius"       // +: the radius beasts notice him at
	LitNerve          = "lit_nerve"           // +: nerve a beast's pack loses to his hit while his torch is lit
)

// multipliers are the keys that combine by product.
var multipliers = map[string]bool{
	FatigueRate: true, FoodWaterRate: true, RiposteDamage: true,
	TorchBurnRate: true, KillNerve: true,
}

// knownEffects is every key the game wires. A node naming anything else is
// refused at load: an effect nothing reads is a talent that does nothing.
var knownEffects = map[string]bool{
	FatigueRate: true, FoodWaterRate: true, MaxHealth: true, ShakenFatigue: true,
	NoReactionFatigue: true, RiposteDamage: true, BlocksPerRound: true, MoveTiles: true,
	CritBand: true, Reactions: true, AdvantageBonus: true, TorchBurnRate: true,
	KillNerve: true, NoticeRadius: true, LitNerve: true,
}

// Node is one talent.
type Node struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Text    string             `json:"text"`
	Effects map[string]float64 `json:"effects"`
}

// Branch is one column of the tree. Nodes are taken in order: a rank needs the
// rank above it.
type Branch struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Nodes []Node `json:"nodes"`
}

// XPTable is what earns experience.
type XPTable struct {
	Slain        map[string]int `json:"slain"` // by spawn row; "" is anything else
	RoutedFactor float64        `json:"routed_factor"`
	Night        int            `json:"night"`
}

// Tree is the validated progression table.
type Tree struct {
	XP       XPTable  `json:"xp"`
	Levels   []int    `json:"levels"` // cumulative XP to reach level i+1
	Branches []Branch `json:"branches"`

	byID   map[string]*Node
	branch map[string]int // node id -> branch index
	rank   map[string]int // node id -> rank in its branch
}

// Load parses and validates a progression table.
func Load(data []byte) (*Tree, error) {
	var t Tree

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("decode talents: %w", err)
	}

	if len(t.Levels) < 2 || t.Levels[0] != 0 {
		return nil, fmt.Errorf("levels must start at 0 and have at least two entries")
	}

	for i := 1; i < len(t.Levels); i++ {
		if t.Levels[i] <= t.Levels[i-1] {
			return nil, fmt.Errorf("levels must rise: %d then %d", t.Levels[i-1], t.Levels[i])
		}
	}

	// THE SIGNED SHAPE IS ENFORCED, not merely expected: three branches of at
	// most five (U1 §6.1). The talent panel is drawn for exactly that, and a
	// fourth branch in the data would otherwise crash it (review finding).
	if len(t.Branches) != 3 {
		return nil, fmt.Errorf("the tree has %d branches; the signed shape is three", len(t.Branches))
	}

	for _, br := range t.Branches {
		if len(br.Nodes) > 5 {
			return nil, fmt.Errorf("branch %q has %d nodes; the signed shape is five at most", br.ID, len(br.Nodes))
		}
	}

	t.byID, t.branch, t.rank = map[string]*Node{}, map[string]int{}, map[string]int{}

	for b := range t.Branches {
		br := &t.Branches[b]
		if br.ID == "" || br.Name == "" || len(br.Nodes) == 0 {
			return nil, fmt.Errorf("branch %d needs an id, a name and nodes", b)
		}

		for r := range br.Nodes {
			n := &br.Nodes[r]
			n.ID = strings.ToLower(strings.TrimSpace(n.ID))

			if n.ID == "" || n.Name == "" || len(n.Effects) == 0 {
				return nil, fmt.Errorf("%s rank %d needs an id, a name and an effect", br.ID, r+1)
			}

			if _, dup := t.byID[n.ID]; dup {
				return nil, fmt.Errorf("duplicate talent %q", n.ID)
			}

			for k := range n.Effects {
				if !knownEffects[k] {
					return nil, fmt.Errorf("talent %q: nothing reads the effect %q", n.ID, k)
				}
			}

			t.byID[n.ID] = n
			t.branch[n.ID] = b
			t.rank[n.ID] = r
		}
	}

	return &t, nil
}

// maxLevel is the highest level the table defines.
func (t *Tree) maxLevel() int { return len(t.Levels) }

// LevelFor is the level a total of XP buys.
func (t *Tree) LevelFor(xp int) int {
	level := 1

	for i := 1; i < len(t.Levels); i++ {
		if xp >= t.Levels[i] {
			level = i + 1
		}
	}

	return level
}

// NextAt is the XP the next level needs, or -1 at the top.
func (t *Tree) NextAt(xp int) int {
	level := t.LevelFor(xp)
	if level >= len(t.Levels) {
		return -1
	}

	return t.Levels[level]
}

// SlainXP is what one kill of a row is worth.
func (t *Tree) SlainXP(row string) int {
	if v, ok := t.XP.Slain[row]; ok {
		return v
	}

	return t.XP.Slain[""]
}

// RoutedXP is what one routed enemy of a row is worth.
func (t *Tree) RoutedXP(row string) int {
	return int(float64(t.SlainXP(row)) * t.XP.RoutedFactor)
}

// Progress is one hero's standing. It is saved in his sidecar.
type Progress struct {
	XP      int      `json:"xp"`
	Talents []string `json:"talents"`

	// BaseMaxHealth is his maximum health BEFORE talents, recorded the first
	// time progress is kept for him. His .od2 stores whatever the maximum was
	// when it was saved, talents included, so recomputing from here -- never
	// adding to the stored value -- is what stops a bonus compounding on
	// every load.
	BaseMaxHealth int `json:"base_max_health"`
}

// Refusals the talent panel shows.
var (
	ErrNoPick      = errors.New("no talent pick is waiting")
	ErrTaken       = errors.New("already taken")
	ErrLocked      = errors.New("take the talent above it first")
	ErrUnknownNode = errors.New("no such talent")
)

// Level is his level.
func (p *Progress) Level(t *Tree) int { return t.LevelFor(p.XP) }

// PicksWaiting is how many talents he has earned and not yet chosen: one per
// level after the first.
func (p *Progress) PicksWaiting(t *Tree) int {
	n := p.Level(t) - 1 - len(p.Talents)
	if n < 0 {
		return 0
	}

	return n
}

// Has reports a taken talent.
func (p *Progress) Has(id string) bool {
	for _, have := range p.Talents {
		if have == id {
			return true
		}
	}

	return false
}

// CanPick reports whether a node could be taken now, and why not.
func (p *Progress) CanPick(t *Tree, id string) error {
	if _, ok := t.byID[id]; !ok {
		return ErrUnknownNode
	}

	if p.Has(id) {
		return ErrTaken
	}

	if r := t.rank[id]; r > 0 {
		above := t.Branches[t.branch[id]].Nodes[r-1].ID
		if !p.Has(above) {
			return ErrLocked
		}
	}

	if p.PicksWaiting(t) <= 0 {
		return ErrNoPick
	}

	return nil
}

// Pick takes a talent. There is no respec (U1 §6.1).
func (p *Progress) Pick(t *Tree, id string) error {
	if err := p.CanPick(t, id); err != nil {
		return err
	}

	p.Talents = append(p.Talents, id)

	return nil
}

// Gain adds experience and reports how many levels it crossed.
func (p *Progress) Gain(t *Tree, xp int) int {
	if xp <= 0 {
		return 0
	}

	before := p.Level(t)
	p.XP += xp

	return p.Level(t) - before
}

// Effect is the combined value of one effect key over every talent taken:
// a product for multipliers (1 with none), a sum for the rest (0 with none).
// Talents the tree no longer knows contribute nothing.
func (p *Progress) Effect(t *Tree, key string) float64 {
	total := 0.0
	if multipliers[key] {
		total = 1.0
	}

	for _, id := range p.Talents {
		n, ok := t.byID[id]
		if !ok {
			continue
		}

		v, ok := n.Effects[key]
		if !ok {
			continue
		}

		if multipliers[key] {
			total *= v
		} else {
			total += v
		}
	}

	return total
}

// NodeState is one node as the talent panel draws it.
type NodeState struct {
	Node   *Node
	Taken  bool
	Open   bool // can be picked now
	Branch int
	Rank   int
}

// Board is every node with its state, branch by branch.
func (p *Progress) Board(t *Tree) [][]NodeState {
	out := make([][]NodeState, len(t.Branches))

	for b := range t.Branches {
		for r := range t.Branches[b].Nodes {
			n := &t.Branches[b].Nodes[r]
			out[b] = append(out[b], NodeState{
				Node: n, Taken: p.Has(n.ID), Open: p.CanPick(t, n.ID) == nil,
				Branch: b, Rank: r,
			})
		}
	}

	return out
}
