package d2player

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
)

type labelled struct {
	d2interface.MapEntity
	label, key string
}

func (l labelled) Label() string   { return l.label }
func (l labelled) NameKey() string { return l.key }

type plain struct {
	d2interface.MapEntity
	label string
}

func (p plain) Label() string { return p.label }

// A villager is known by his untranslated name, whatever his label reads.
func TestNameKeyIsTheUntranslatedName(t *testing.T) {
	if got := nameKey(labelled{label: "The headman", key: "Warriv"}); got != "Warriv" {
		t.Fatalf("an NPC translated to %q is known as %q, want Warriv", "The headman", got)
	}

	if got := nameKey(plain{label: "Chest"}); got != "Chest" {
		t.Fatalf("an entity with no key is known by its label, got %q", got)
	}
}

// identified is an entity with an id and a label, and nothing else.
type identified struct {
	d2interface.MapEntity
	id, label string
}

func (e identified) ID() string    { return e.id }
func (e identified) Label() string { return e.label }

// deadNamer answers DeadName from a table; the rest of CorpseHolder is unused.
type deadNamer struct {
	CorpseHolder
	names map[string]string
}

func (d deadNamer) DeadName(id string) string { return d.names[id] }

// 27 Sep 2026: every place that names a creature to the player uses ONE
// function. The combat log once read Label, so a risen man the hover called
// "A fallen soldier" struck as "the dead" in the log; here the log's name is
// the hover's for the dead, the label for anything else, and "Something" for
// a creature with neither.
func TestTheCombatLogNamesAsTheHoverDoes(t *testing.T) {
	g := &GameControls{corpseHolder: deadNamer{names: map[string]string{"risen:1": "A fallen soldier"}}}
	h := &HUD{gameControls: g}

	risen := identified{id: "risen:1", label: "the dead"}
	wolf := identified{id: "wolf:1", label: "Wolf"}
	blank := identified{id: "x:1", label: "  "}

	for _, c := range []struct {
		e    identified
		want string
	}{
		{risen, "A fallen soldier"},
		{wolf, "Wolf"},
		{blank, TacticalSomething},
	} {
		if got := h.logName(c.e); got != c.want {
			t.Errorf("the log calls %s %q, want %q", c.e.id, got, c.want)
		}
	}

	if hover, log := g.nameFor(risen), h.logName(risen); hover != log {
		t.Fatalf("the hover says %q and the log %q", hover, log)
	}

	// Before the controls are attached the HUD has only the label.
	if got := (&HUD{}).logName(risen); got != "the dead" {
		t.Fatalf("no controls, the label: %q", got)
	}
}
