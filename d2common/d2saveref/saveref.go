// Package d2saveref holds the words the world save writes in place of an id
// (M4.6). It is a leaf -- it imports nothing -- so that the world model
// (d2world) and the map entities (d2mapentity), which import neither each
// other nor anything in common, can agree on one spelling.
package d2saveref

// Player is the word a save writes wherever a record points at the player.
//
// The player's entity id is his CONNECTION's uuid, new on every launch, so a
// save that wrote it would name a man the resumed game does not have. Every
// other entity is rebuilt with its saved id (the next-id seam), so he alone
// needs a word -- and the seam must refuse the word as an id.
//
// It was three spellings until the B2b review (28 Sep 2026): d2world's
// PlayerRef, the squads snapshot's playerEntity, and a literal in the seam.
// d2world.PlayerRef is now this constant under the name d2world's code reads.
const Player = "player"
