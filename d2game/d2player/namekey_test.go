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
