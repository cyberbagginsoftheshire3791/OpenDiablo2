package d2world

import (
	"fmt"
)

// CorpsesSnapshot is the registry's saved state (M4.6 B2a): every body in the
// order it fell, and the three maps that tie a body to the members it has
// walked as. A body is saved whole -- row, class, state, where it lies, when it
// went down and what the man was in life -- because the machine is per body:
// a count could not say which door stands next or what the hover calls him.
//
// risen_as keeps EVERY member a body ever walked as (B1 review C12): a man
// who stood again is still the same body, and his first member's fall must
// still find it.
type CorpsesSnapshot struct {
	Bodies  []CorpseSnapshot  `json:"bodies"`
	RisenAs map[string]string `json:"risen_as"`
	Walker  map[string]string `json:"walker"`
	Last    map[string]string `json:"last"`
}

// CorpseSnapshot is one body.
type CorpseSnapshot struct {
	ID       string      `json:"id"`
	Row      string      `json:"row"`
	Class    CorpseClass `json:"class"`
	State    CorpseState `json:"state"`
	X        float64     `json:"x"`
	Y        float64     `json:"y"`
	Was      string      `json:"was"`
	DownedAt float64     `json:"downed_at"`
}

// Snapshot is the registry as it stands.
func (c *Corpses) Snapshot() CorpsesSnapshot {
	out := CorpsesSnapshot{
		Bodies:  make([]CorpseSnapshot, 0, len(c.order)),
		RisenAs: copyStrings(c.risenAs),
		Walker:  copyStrings(c.walker),
		Last:    copyStrings(c.last),
	}

	for _, id := range c.order {
		b := c.byID[id]
		out.Bodies = append(out.Bodies, CorpseSnapshot{
			ID: b.ID, Row: b.Row, Class: b.Class, State: b.State, X: b.X, Y: b.Y, Was: b.Was, DownedAt: b.DownedAt,
		})
	}

	return out
}

// Restore puts the saved bodies back WITHOUT CALLING changed (plan section 5,
// trap 5). The callback moves the spawn tables' open-body count by a delta,
// and on a load that count is set from the file; a restore that fired it would
// count every open body twice. For the same reason it restores only into an
// EMPTY registry: bodies already in it (Night 1's placed dead, if the load
// forgot to skip placing them) have already moved the count, and replacing
// them would leave the count describing bodies that are gone.
//
// It is checked whole first (Validate), so a refused snapshot changes nothing.
func (c *Corpses) Restore(s CorpsesSnapshot) error {
	if err := c.Validate(s); err != nil {
		return err
	}

	byID := make(map[string]*Corpse, len(s.Bodies))
	order := make([]string, 0, len(s.Bodies))

	for _, b := range s.Bodies {
		byID[b.ID] = &Corpse{
			ID: b.ID, Row: b.Row, Class: b.Class, State: b.State, X: b.X, Y: b.Y, Was: b.Was, DownedAt: b.DownedAt,
		}
		order = append(order, b.ID)
	}

	c.byID, c.order = byID, order
	c.risenAs, c.walker, c.last = copyStrings(s.RisenAs), copyStrings(s.Walker), copyStrings(s.Last)

	return nil
}

// Validate is Restore's check and nothing else (D4): the registry must be
// empty, every body a man or a beast in one of the machine's states, ids
// unique, and every map naming bodies that are saved -- a walker's body is
// risen, and its member walks back to it.
//
// It accepts only what the machine could have written (the B2a review's B2):
//   - a beast never rises (Rise takes only a door, and a door is a man's), so
//     it is never risen or Downed. A beast in a hasty grave IS possible -- the
//     dig verb buries any open body, a carcass included (Corpses.Bury; the
//     review listed hasty with the other two, and the game says otherwise);
//   - a risen body walks as someone: the game stands a body up only when it
//     has a member to walk as (raise), and Raised records him in walker;
//   - downed_at is written when a man goes Downed and never cleared, so a
//     body that stood again (risen) or was staked (closed) after it keeps its
//     minute. What never went Downed -- an open body, a hasty grave, any
//     beast -- has none. (The review said "a downed_at on a body that is not
//     Downed"; the corpses fixture's b:7, cut down and standing again, is the
//     machine writing exactly that.)
func (c *Corpses) Validate(s CorpsesSnapshot) error {
	if len(c.byID) != 0 || len(c.order) != 0 || len(c.risenAs) != 0 || len(c.walker) != 0 || len(c.last) != 0 {
		return fmt.Errorf("corpses snapshot: restore into an empty registry; this one holds %d bodies", len(c.order))
	}

	return c.CheckSnapshot(s)
}

// CheckSnapshot is every check Validate makes of the snapshot itself --
// everything but its refusal of a registry already in use. Game.SaveWorld runs it on
// the snapshot it has just taken from this live registry, so a save never writes a
// block the load's own Validate would refuse (the M4.6 B3 review, B2: "strict
// at save" stopped at the file's own checks). Validate is the in-use refusal
// and this, so the two cannot disagree.
func (c *Corpses) CheckSnapshot(s CorpsesSnapshot) error {
	return checkCorpsesSnapshot(s)
}

func checkCorpsesSnapshot(s CorpsesSnapshot) error {
	state := make(map[string]CorpseState, len(s.Bodies))

	for _, b := range s.Bodies {
		switch {
		case b.ID == "":
			return fmt.Errorf("corpses snapshot: a body with no id")
		case state[b.ID] != "":
			return fmt.Errorf("corpses snapshot: body %q is saved twice", b.ID)
		case b.Class != CorpseHuman && b.Class != CorpseBeast:
			return fmt.Errorf("corpses snapshot: body %q is class %q, neither human nor beast", b.ID, b.Class)
		case !b2aFinite(b.X, b.Y, b.DownedAt):
			return fmt.Errorf("corpses snapshot: body %q lies at a position or a minute that is not a number", b.ID)
		}

		switch b.State {
		case CorpseFresh, CorpseHasty, CorpseClosed, CorpseRisen, CorpseDowned:
		default:
			return fmt.Errorf("corpses snapshot: body %q is in no state the machine has (%q)", b.ID, b.State)
		}

		switch {
		case b.Class == CorpseBeast && (b.State == CorpseRisen || b.State == CorpseDowned):
			return fmt.Errorf("corpses snapshot: body %q is a beast %s; a beast never rises", b.ID, b.State)
		case b.DownedAt != 0 && (b.Class == CorpseBeast || b.State == CorpseFresh || b.State == CorpseHasty):
			return fmt.Errorf("corpses snapshot: body %q (%s %s) went down at minute %v; it never went Downed",
				b.ID, b.Class, b.State, b.DownedAt)
		}

		state[b.ID] = b.State
	}

	for member, body := range s.RisenAs {
		if member == "" || state[body] == "" {
			return fmt.Errorf("corpses snapshot: member %q walked as body %q, which is not saved", member, body)
		}
	}

	for body, member := range s.Walker {
		if state[body] != CorpseRisen || s.RisenAs[member] != body {
			return fmt.Errorf("corpses snapshot: body %q walks as %q, but it is not risen or %q does not walk back to it",
				body, member, member)
		}
	}

	for body, member := range s.Last {
		if state[body] == "" || s.RisenAs[member] != body {
			return fmt.Errorf("corpses snapshot: body %q last walked as %q, which does not walk back to it", body, member)
		}
	}

	for _, b := range s.Bodies {
		if b.State == CorpseRisen && s.Walker[b.ID] == "" {
			return fmt.Errorf("corpses snapshot: body %q is risen and walks as no one; the game stands a body up only "+
				"as a member", b.ID)
		}
	}

	return nil
}
