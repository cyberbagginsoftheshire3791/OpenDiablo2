package d2gamescreen

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
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

// BodiesKnown is how many monsters currently have a body. NPC BODIES ONLY --
// the player's body is answered by BodyOf but is not in this registry and
// never counts here, so a script that reads bodies_known across step 4 must
// not expect it to have gone up by one.
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
	// THE PLAYER ANSWERS HERE TOO, AND THAT IS WHAT MAKES LOSING POSSIBLE.
	//
	// Before step 4 the resolver could reach a wolf's health and not the
	// player's -- Fitness was fenced to two booleans and playerBody was handed
	// privately to the meters -- so a fight could only ever go one way. One
	// seam for every participant is the narrowest fix, and it has a second
	// virtue: the resolver now writes the player's health through the SAME
	// adapter the meters' neglect path uses (game.go's playerBody, the only
	// writer of Stats.Health in the tree), so the two readings cannot drift.
	//
	// A second interface for the player -- PlayerBody -- was the alternative,
	// and it is the second-health-abstraction disease this project keeps
	// treating.
	if v.localPlayer != nil && v.localPlayer.Stats != nil && v.localPlayer.ID() == id {
		return playerBody{player: v.localPlayer}
	}

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

// Animate satisfies d2world.Animator: it is how the resolver reaches sprites.
//
// THE SEAM IS THE SAME ONE BODIES USES, for the same reason -- d2world cannot
// import d2mapentity, so it names three acts and this side decides what they
// look like. A swing is A1, a blow taken is GH, a death is DT and then a held
// DD; the monster's held-mode path (NPC.StartAction) is what makes any of them
// survive the tick after they were set.
//
// THE PLAYER'S SIDE RENDERS WRONG AND IT IS COSMETIC, NOT A DEFERRAL WORTH
// PRETENDING AWAY: Player.Advance re-applies GetAnimationMode() every tick and
// that returns Cast while IsCasting(), so a StartCasting(Attack1) shows the SC
// animation from the second tick onward. The fix is one field on Player
// (castMode, returned from GetAnimationMode) and it belongs in its own commit
// with a screenshot; the milestone's DoD says nothing about the player's
// sprite, and inventing a second animation path here to dodge it would be
// worse than the flaw.
func (v *Game) Animate(id string, act d2world.CombatAct) {
	if id == "" {
		return
	}

	if v.localPlayer != nil && v.localPlayer.ID() == id {
		if act == d2world.ActSwing {
			v.localPlayer.StartCasting(d2enum.PlayerAnimationModeAttack1, nil)
		}

		// Nothing for a blow taken and nothing for death: the player's death
		// screen, and what a dead player looks like, are M4.6's.
		return
	}

	if v.gameClient == nil || v.gameClient.MapEngine == nil {
		return
	}

	entity, ok := v.gameClient.MapEngine.Entities()[id]
	if !ok {
		return
	}

	npc, ok := entity.(*d2mapentity.NPC)
	if !ok {
		return
	}

	// Errors are swallowed on purpose. A missing animation is a content
	// problem in the MPQs, not a reason to stop resolving a fight -- and
	// tools/animcensus measured on 31 Aug that fallen1, zombie1 and skeleton1
	// all carry A1, GH, DT and DD, so a failure here means something the
	// spawn tables did not place.
	switch act {
	case d2world.ActSwing:
		_ = npc.StartAction(d2enum.MonsterAnimationModeAttack1, nil)
	case d2world.ActHit:
		_ = npc.StartAction(d2enum.MonsterAnimationModeGetHit, nil)
	case d2world.ActDie:
		_ = npc.StartAction(d2enum.MonsterAnimationModeDeath, nil)
	}
}
