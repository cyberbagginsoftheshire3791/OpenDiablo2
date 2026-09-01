package d2gamescreen

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// npcBody is the monster half of d2world.Body. The player half is playerBody,
// at the foot of game.go, and until this file existed there was no other:
// the player was a body and an NPC was a sprite on a path.
//
// THE HEALTH LIVES HERE, ON AN ADAPTER OWNED BY THE GAME SCREEN, AND NOT ON
// *NPC. Three reasons, and the M4.5 build note (section 4.4) names this as
// the weakest structural call in the milestone, so it is worth restating
// where the code is. d2world already owns a health abstraction and a second
// one is the thing this project keeps punishing. d2mapentity is
// engine-inherited code and growing struct state there is the expensive kind
// of change. And R2 section 1 signs NO health bar until folk-knowledge
// unlocks it, so nothing needs the entity to render its own health.
//
// The trade reverses the day a health bar is wanted: the entity would then
// need to reach its own health, and this would have to move onto it. Named
// rather than discovered.
type npcBody struct {
	health    int
	maxHealth int
}

// npcBody satisfies the interface the meters already declared for the player.
var _ d2world.Body = (*npcBody)(nil)

// CurrentHealth is what a resolver will subtract from at M4.5 step 4.
func (b *npcBody) CurrentHealth() int { return b.health }

// MaxHealth is the monstats MaxHPNormal the body was adopted with.
func (b *npcBody) MaxHealth() int { return b.maxHealth }

// SetHealth writes the body's health. NOTHING CALLS THIS YET, deliberately:
// a blow is what writes it, and the resolver that lands one is step 4. The
// reachability register carries that as a deferral rather than leaving it to
// be noticed.
func (b *npcBody) SetHealth(h int) { b.health = h }

// newNPCBody builds a body at full health from a monstats record's band.
//
// MaxHPNormal, NOT A ROLL BETWEEN MIN AND MAX, and the reason is determinism
// rather than taste: a roll needs an RNG stream, and every existing stream's
// draw sequence is what the two-launch determinism proof and every seed-1462
// measurement rest on. The worldgen fix of 28 August added ONE draw and moved
// the player from tile (31,14) to (106,128); a roll here would move every
// spawn position in every playtest for a cosmetic gain. Varying hit points
// is a [DIAL] deferred to step 4, where the resolver's own RNG is the honest
// place for it.
//
// MEASURED (tools/animcensus, 31 Aug): fallen1 21-61, zombie1 101-181,
// skeleton1 86-129. So a wolf ships with 181 and a dog with 61.
//
// A record with no positive maximum is clamped to 1. A body is never born
// dead, and a zero here would make a monster that dies to the first blow look
// like a resolver bug at step 4.
func newNPCBody(maxHealth int) *npcBody {
	if maxHealth < 1 {
		maxHealth = 1
	}

	return &npcBody{health: maxHealth, maxHealth: maxHealth}
}

// adoptNPCBody gives a monster a body, keyed by the entity id that
// chaser.WatcherID reports -- the only identity d2world has for it.
//
// THE GAME CALLS THIS, WHICH IS THE POINT. gameSpawner.Spawn calls it for
// every member of every pack the night sends, and Spawns.Advance drives that
// from advanceWorld, so a shipped build adopts bodies without the harness
// doing anything. Compare Light.Remove after M4.1, whose only caller was
// inside HarnessSet and which was therefore still harness-only one level
// down: this one is not.
func (v *Game) adoptNPCBody(id string, maxHealth int) {
	if id == "" {
		return
	}

	if v.bodies == nil {
		v.bodies = make(map[string]*npcBody)
	}

	if _, exists := v.bodies[id]; exists {
		return
	}

	v.bodies[id] = newNPCBody(maxHealth)
}

// releaseNPCBody forgets a body when its entity leaves the map.
func (v *Game) releaseNPCBody(id string) {
	delete(v.bodies, id)
}

// BodiesKnown is how many monsters currently have a body.
//
// It is what makes eager adoption testable. gameSpawner.Spawn adopts when the
// night places a pack; BodyOf adopts on demand for anything else. Both end up
// with a body, so a test that only ever looks at a body cannot tell which
// path put it there -- and a broken eager path would then be invisible, which
// is how M4.1 and M4.3b shipped hollow. A playtest can force a real table
// arrival and watch this number rise before any fight exists, which only the
// eager path can do.
func (v *Game) BodiesKnown() int { return len(v.bodies) }

// BodyOf satisfies d2world.Bodies.
//
// It falls back to adopting on demand, and that fallback is not tidiness --
// it is the fix for a real hole. THE HARNESS AND THE DEBUG TERMINAL BOTH PUT
// NPCs ON THE MAP WITHOUT PASSING THROUGH THIS SCREEN: d2app/harness_spawn.go
// calls engine.NewNPC directly for strigoi_spawn_entity, and d2app does not
// import d2core/d2world at all, so it could not build a body even if it
// wanted to; Game.commandSpawnMon does the same for the terminal. Without
// this fallback every script-placed monster would be bodiless, and step 4's
// first fight -- which a playtest necessarily arranges with the harness --
// would meet a nil and read as a broken resolver.
//
// The lookup is by id against the map engine's own entity table, so a body is
// only ever invented for something that is really on the map.
//
// Returns an UNTYPED nil when there is no body. Returning v.bodies[id]
// directly would hand back a nil *npcBody wrapped in a non-nil interface, and
// the caller's "if body != nil" would pass.
func (v *Game) BodyOf(id string) d2world.Body {
	if b, ok := v.bodies[id]; ok && b != nil {
		return b
	}

	if v.gameClient == nil || v.gameClient.MapEngine == nil {
		return nil
	}

	entity, ok := v.gameClient.MapEngine.Entities()[id]
	if !ok {
		return nil
	}

	npc, ok := entity.(*d2mapentity.NPC)
	if !ok {
		return nil
	}

	monstat := npc.MonStat()
	if monstat == nil {
		return nil
	}

	v.adoptNPCBody(id, monstat.MaxHPNormal)

	if b, ok := v.bodies[id]; ok && b != nil {
		return b
	}

	return nil
}
