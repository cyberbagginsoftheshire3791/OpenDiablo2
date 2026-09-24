package d2player

import "github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"

// nameKey is e's untranslated name when it has one -- an NPC's monstats
// NameString (d2mapentity.NPC.NameKey) -- else its Label. A villager is
// recognised by the sprite standing in for him, and with Strigoi's own words
// (-strings) his Label is translated away from that sprite's name.
// (d2player does not import d2mapentity; the method is asserted instead.)
func nameKey(e d2interface.MapEntity) string {
	if k, ok := e.(interface{ NameKey() string }); ok {
		return k.NameKey()
	}

	return e.Label()
}
