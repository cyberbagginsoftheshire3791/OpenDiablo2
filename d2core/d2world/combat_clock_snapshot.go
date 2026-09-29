package d2world

// The clock driver's half of the combat snapshot (the raid's R1, 29 Sep 2026;
// combat_clock.go is the driver). The raid's Q5, default (a): "a fight he is
// not in never stops a save; it is saved with the world". So the clock book's
// records and every live clock fight go into CombatSnapshot.Clock, and only
// his fight still refuses a save (ErrCombatFighting).

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// clockSnapshot is the clock block: the book's records and the live fights.
func (c *Combat) clockSnapshot() (CombatClockSnapshot, error) {
	b := c.clockBook

	if err := c.checkBook(b); err != nil {
		return CombatClockSnapshot{}, err
	}

	k := CombatClockSnapshot{
		NextID: b.nextID, RNG: d2rand.StateOf(b.rng),
		Started: b.started, Ended: b.ended, Rounds: b.rounds, Declines: b.declines, Actions: b.actions,
		Joined: b.joined, QuickResolved: b.quickResolved, LastQuickAdvantage: b.lastQuickAdvantage,
		EndedReason: b.endedReason, EndedQuarryDead: b.quarryDead, EndedEnemiesDead: b.endedEnemiesDead,
		EndedPlayerDead: b.endedPlayerDead, EndedDisengaged: b.endedDisengaged, EndedRouted: b.endedRouted,
		EndedDawn: b.endedDawn, XPSuppressed: b.xpSuppressed, Released: b.released,
		ReentrantReads: b.reentrantReads, ActionsRound: b.actionsRound, LastActions: actionSnapshots(b.lastActions),
	}

	for _, e := range c.clockFights {
		f, err := c.clockFightSnapshot(e)
		if err != nil {
			return CombatClockSnapshot{}, err
		}

		k.Live = append(k.Live, f)
	}

	return k, nil
}

// checkBook refuses a snapshot of a clock book that holds what no save may
// drop: a step in progress, and what a fight leaves for the game screen or its
// pace window -- the same refusals as his (checkBetweenFights), read with the
// book on the struct. A clock fight never paces, never waits and never earns,
// so every one of them is empty between steps.
func (c *Combat) checkBook(b *fightBook) error {
	switch {
	case b.stepping || b.encounter != nil:
		return fmt.Errorf("combat snapshot refused: a clock fight is mid-step")
	case b.stepsOrdered != 0:
		return fmt.Errorf("combat snapshot refused: the clock's book holds %d ordered step(s); a clock fight never paces", b.stepsOrdered)
	case b.lastActionVerb != "":
		return fmt.Errorf("combat snapshot refused: the clock's book holds a player's verb %q; a clock fight never takes one", b.lastActionVerb)
	}

	c.swapBook(b)
	err := c.checkBetweenFights()
	c.swapBook(b)

	if err != nil {
		return fmt.Errorf("the clock's book: %w", err)
	}

	return nil
}

// clockFightSnapshot is one live clock fight, refused if it holds anything a
// clock fight should never hold between two steps.
func (c *Combat) clockFightSnapshot(e *encounter) (CombatClockFightSnapshot, error) {
	switch {
	case e == nil || e.target == nil:
		return CombatClockFightSnapshot{}, fmt.Errorf("combat snapshot refused: a clock fight with no quarry")
	case e.driver != driverClock:
		return CombatClockFightSnapshot{}, fmt.Errorf("combat snapshot refused: %s is in the clock's list and not clock-driven", e.id)
	case c.player != "" && e.target.QuarryID() == c.player:
		return CombatClockFightSnapshot{}, fmt.Errorf("combat snapshot refused: clock fight %s is after him; his fight is never a clock fight", e.id)
	case e.awaiting || e.moveSpent || e.actionSpent || e.roundOpen || len(e.sequence) != 0 || e.cursor != 0 ||
		e.playerWait != 0 || len(e.acting) != 0 || len(e.stepping) != 0 || len(e.strikers) != 0 ||
		e.beat != 0 || e.stepWait != 0:
		return CombatClockFightSnapshot{}, fmt.Errorf("combat snapshot refused: clock fight %s holds a paced round's or a turn's state", e.id)
	case e.reactionUsedInRound >= e.round || e.blockUsedInRound >= e.round:
		// A cap is keyed by the round that spent it, and between two steps the
		// fight has always moved past that round (finishRound). A cap keyed by
		// the running round would be lost by the save: refuse it rather.
		return CombatClockFightSnapshot{}, fmt.Errorf("combat snapshot refused: clock fight %s has a reaction or block spent in its running round %d",
			e.id, e.round)
	}

	f := CombatClockFightSnapshot{
		ID: e.id, Quarry: e.target.QuarryID(),
		Enemies: make([]string, 0, len(e.enemies)), EnemyOrder: append([]string{}, e.enemyOrder...),
		Dead: sortedTrue(e.dead), Routed: sortedTrue(e.routed), Broke: sortedTrue(e.broke),
		Round: e.round, SinceTurn: e.sinceTurn,
		Initiator: e.initiator, Surprised: e.surprised, SurpriseWhy: e.surpriseWhy,
	}

	for _, en := range e.enemies {
		if en != nil {
			f.Enemies = append(f.Enemies, en.WatcherID())
		}
	}

	return f, nil
}

// checkClockSnapshot is the clock block's own consistency, with nothing live
// to resolve against: counts, the id sequence, the fights' shapes.
func checkClockSnapshot(k CombatClockSnapshot) error {
	counts := []struct {
		name string
		n    int
	}{
		{"started", k.Started}, {"ended", k.Ended}, {"rounds", k.Rounds}, {"declines", k.Declines},
		{"actions", k.Actions}, {"joined", k.Joined}, {"quick_resolved", k.QuickResolved},
		{"ended_quarry_dead", k.EndedQuarryDead}, {"ended_enemies_dead", k.EndedEnemiesDead},
		{"ended_player_dead", k.EndedPlayerDead}, {"ended_disengaged", k.EndedDisengaged},
		{"ended_routed", k.EndedRouted}, {"ended_dawn", k.EndedDawn}, {"xp_suppressed", k.XPSuppressed},
		{"released", k.Released}, {"reentrant_reads", k.ReentrantReads}, {"actions_round", k.ActionsRound},
	}

	for _, n := range counts {
		if n.n < 0 {
			return fmt.Errorf("combat snapshot: clock.%s is %d", n.name, n.n)
		}
	}

	switch {
	case k.NextID != k.Started+1:
		// The clock's tryStart spends one number per fight, as his does.
		return fmt.Errorf("combat snapshot: clock.next_id %d after %d clock fight(s) started; it is always one past", k.NextID, k.Started)
	case k.Ended+len(k.Live) != k.Started:
		return fmt.Errorf("combat snapshot: %d clock fight(s) started, %d ended and %d live; every one started has ended or is live",
			k.Started, k.Ended, len(k.Live))
	case !b2aFinite(k.LastQuickAdvantage):
		return fmt.Errorf("combat snapshot: clock.last_quick_advantage is not a number")
	}

	ids, quarries := map[string]bool{}, map[string]bool{}

	for i, f := range k.Live {
		if err := checkClockFight(f, k.Started); err != nil {
			return fmt.Errorf("combat snapshot: clock.live[%d]: %w", i, err)
		}

		if ids[f.ID] || quarries[f.Quarry] {
			return fmt.Errorf("combat snapshot: clock.live[%d]: %s (after %q) is a second fight of one id or one quarry", i, f.ID, f.Quarry)
		}

		ids[f.ID], quarries[f.Quarry] = true, true
	}

	return nil
}

// checkClockFight is one saved clock fight's own shape.
func checkClockFight(f CombatClockFightSnapshot, started int) error {
	n, err := strconv.Atoi(strings.TrimPrefix(f.ID, clockIDPrefix))

	switch {
	case !strings.HasPrefix(f.ID, clockIDPrefix) || err != nil || n < 1 || n > started:
		return fmt.Errorf("id %q is not c:<n> for n in 1..%d", f.ID, started)
	case f.Quarry == "" || f.Quarry == PlayerRef:
		return fmt.Errorf("quarry %q: a clock fight's quarry is an entity, and never he", f.Quarry)
	case len(f.Enemies) == 0:
		return fmt.Errorf("no enemies")
	case f.Round < 1:
		return fmt.Errorf("round %d", f.Round)
	case math.IsNaN(f.SinceTurn) || math.IsInf(f.SinceTurn, 0) || f.SinceTurn < -roundEpsilon:
		// Not below 0 -- below -roundEpsilon (BUG-85, found by the merge
		// scout's TestSaveResumeAFightHeIsNotIn). The round loop resolves a
		// round once the minutes reach RoundMinutes-roundEpsilon and then
		// takes a whole RoundMinutes off, so a round resolved on an
		// accumulated 0.9999999999999999 leaves since_turn a hair below zero
		// (-2.2e-16, measured) until the next slice: a state the model makes
		// on the frame a round resolves, which the save refused INTERNAL and
		// a load would have refused BLOCK.
		return fmt.Errorf("since_turn %v", f.SinceTurn)
	case f.Initiator != "enemy":
		return fmt.Errorf("initiator %q: a clock fight opens on its enemies' notice", f.Initiator)
	case !f.Surprised && f.SurpriseWhy != "":
		return fmt.Errorf("surprise_why %q in a fight that was not a surprise", f.SurpriseWhy)
	}

	// Each list is a set of entity ids. The order and the three sets may name
	// ids no longer among the enemies -- the dead that broke off at first light
	// leave the rows (pruneOrEnd), and a Downed man who stood again leaves his
	// old id behind (Rejoin) -- so only the enemies are resolved at a load,
	// and of them only the living must resolve (clockFightsOf).
	for name, list := range map[string][]string{
		"enemies": f.Enemies, "enemy_order": f.EnemyOrder, "dead": f.Dead, "routed": f.Routed, "broke": f.Broke,
	} {
		seen := map[string]bool{}

		for _, id := range list {
			if id == "" || id == PlayerRef || id == f.Quarry || seen[id] {
				return fmt.Errorf("%s names %q: empty, him, the quarry, or twice", name, id)
			}

			seen[id] = true
		}
	}

	return nil
}

// CheckClockWatches is the world file's cross-check of the combat block's
// live clock fights against the notice and pursuit blocks (BUG-73; the merge
// scout's N3, strigoi-harness-runs\wt-merge-scout\merge-notes.md). Neither
// Combat.Validate nor Notice.Validate can make it: at the load's step 4 the
// combat model's notice is the new game's, which watches no one (the file's
// watches are restored only at step 5, after every block's Validate has
// passed), so only the file holds both halves. Game.checkLoad calls it
// after the blocks' own checks, and Game.validateSnapshots calls it on the
// file a save has assembled, as B3 runs every check of the load at save.
//
// NOTHING ELSE TIES A CLOCK FIGHT TO ITS ENEMIES. Its quarry is an entity id,
// and Validate asks only that the id resolves. So a file whose two clock
// fights had their quarries swapped was accepted, and the resumed world
// diverged from the saved one: at the first step each fight's enemies were
// found aware of another fight's quarry and let go (pruneOrEnd), both fights
// ended, and new ones opened in their place.
//
// THE RULE IS pruneOrEnd's OWN, as the save found it. At the end of every
// clock step pruneOrEnd lets go of each living enemy (not dead, routed or
// broken off) whose NOTICED watch names someone other than its fight's
// quarry (awareOfAnother), and a frame's every watch and awareness change is
// made before the combat model steps (Game.advanceWorld: the tables step the
// notice model, the rising stands the Downed, and then combat). So no file a
// frame leaves holds such an enemy -- unless its chase still names the
// quarry: a harness strigoi_watch moves the watch and not the chase, and a
// save between that verb and the next frame is the game's own state. A
// living enemy of a live clock fight is therefore refused when its watch is
// noticed and names another AND it does not chase the fight's quarry.
//
// WHAT IS NOT REFUSED, because the game makes it: a gone enemy's row, with
// or without a watch or an entity (the review's B1, BUG-68); a living enemy
// whose watch names another and is NOT noticed (awareOfAnother lets it stay
// while it is in reach: a Downed man standing again into the village's fight
// is watching him, a spawn table's watch, until he sees him; a retarget that
// has not been noticed yet); a living enemy with no watch at all (a harness
// strigoi_unwatch keeps it in its fight while it is in reach); and any chase,
// or none (a chase started on an earlier target is never moved, and a
// watcher that cannot walk has none). The one-fight rule (the review's A1,
// BUG-67) is Combat's own and is not read here: this reads each fight's
// enemies against the watches alone.
//
// Only the ids are compared: a clock fight's quarry is never he, so a watch
// or chase on PlayerRef is always another quarry's.
func CheckClockWatches(combat CombatSnapshot, notice NoticeSnapshot, pursuit PursuitSnapshot) error {
	if len(combat.Clock.Live) == 0 {
		return nil
	}

	watches := make(map[string]WatchSnapshot, len(notice.Watches))
	for _, w := range notice.Watches {
		watches[w.Watcher] = w
	}

	chases := make(map[string]string, len(pursuit.Chases))
	for _, ch := range pursuit.Chases {
		chases[ch.Hunter] = ch.Quarry
	}

	for i, f := range combat.Clock.Live {
		gone := map[string]bool{}

		for _, set := range [][]string{f.Dead, f.Routed, f.Broke} {
			for _, id := range set {
				gone[id] = true
			}
		}

		for _, id := range f.Enemies {
			w, watching := watches[id]

			if gone[id] || !watching || !w.Noticed || w.Target == f.Quarry || chases[id] == f.Quarry {
				continue
			}

			chase := "nothing"
			if q, ok := chases[id]; ok {
				chase = fmt.Sprintf("%q", q)
			}

			return fmt.Errorf("combat snapshot: clock.live[%d] %s (after %q): its living enemy %s is aware of %q and chases %s; "+
				"a clock fight lets go of an enemy aware of another at the end of every step, so no save holds one",
				i, f.ID, f.Quarry, id, w.Target, chase)
		}
	}

	return nil
}

// clockFightsOf builds the saved clock fights as live encounters, resolving
// each quarry and enemy through the Resolver the game attached. It is
// Validate's last check and Restore's build: one code for both, so what
// Validate accepts is exactly what Restore makes.
//
// ONLY THE LIVING MUST RESOLVE (the raid R1 review's B1, BUG-68). A clock
// fight keeps the rows of the enemies that left it -- dead, routed, broken off
// -- for its whole life, and their entities can leave the map while it runs:
// a slain opportunist's remains go when his body rises (takeOffTheMap), a pack
// is sent home at daybreak. The save validates the live model with this very
// function, so a row that had to resolve refused every save until that fight
// ended -- a fight he is not in stopping a save, which Q5 (a) says never
// happens. So a gone enemy is saved as its id alone (as enemy_order, dead,
// routed and broke always were) and rebuilt from the map when its entity is
// still there, as an id-only row (goneRow) when it is not; a LIVING enemy that
// does not resolve is still refused (ErrUnresolvedRef).
func (c *Combat) clockFightsOf(k CombatClockSnapshot) ([]*encounter, error) {
	if len(k.Live) == 0 {
		return nil, nil
	}

	out := make([]*encounter, 0, len(k.Live))

	for i, f := range k.Live {
		q, err := b2bResolveQuarry(c.resolver, f.Quarry)
		if err != nil {
			return nil, fmt.Errorf("combat snapshot: clock.live[%d] %s: %w", i, f.ID, err)
		}

		e := &encounter{
			id: f.ID, target: q, driver: driverClock,
			enemyOrder: append([]string{}, f.EnemyOrder...),
			dead:       map[string]bool{}, routed: map[string]bool{}, broke: map[string]bool{},
			round: f.Round, sinceTurn: f.SinceTurn,
			initiator: f.Initiator, surprised: f.Surprised, surpriseWhy: f.SurpriseWhy,
		}

		for _, id := range f.Dead {
			e.dead[id] = true
		}

		for _, id := range f.Routed {
			e.routed[id] = true
		}

		for _, id := range f.Broke {
			e.broke[id] = true
		}

		for _, id := range f.Enemies {
			w, err := b2bResolveWatcher(c.resolver, id)

			switch {
			case err == nil:
			case e.gone(id):
				w = goneRow(id)
			default:
				return nil, fmt.Errorf("combat snapshot: clock.live[%d] %s: %w", i, f.ID, err)
			}

			e.enemies = append(e.enemies, w)
		}

		out = append(out, e)
	}

	return out, nil
}

// goneRow is a restored clock fight's row for an enemy that has left it --
// dead, routed or broken off -- and whose entity is no longer on the map
// (clockFightsOf). It answers to its id, which is all its row is ever read
// for: the participant row, the order, the sets. It stands nowhere
// (goneRowAt): the one reader that asks where a gone row stands is the Downed
// test (pruneOrEnd, stillIn), and a Downed man's remains are on the map until
// he stands again or is laid down, so his row resolves.
type goneRow string

// goneRowAt is where a goneRow stands: off any map, and finite, so the tile
// arithmetic of within() puts it out of every reach.
const goneRowAt = -1 << 20

// WatcherID is the gone enemy's id.
func (g goneRow) WatcherID() string { return string(g) }

// WatcherAt is nowhere on any map.
func (g goneRow) WatcherAt() (x, y float64) { return goneRowAt, goneRowAt }

// restoreClock puts the clock book and its fights back (Restore, after
// Validate has passed the whole snapshot).
func (c *Combat) restoreClock(k CombatClockSnapshot, fights []*encounter) {
	b := c.clockBook

	b.nextID = k.NextID
	b.started, b.ended, b.rounds, b.declines, b.actions = k.Started, k.Ended, k.Rounds, k.Declines, k.Actions
	b.joined, b.quickResolved, b.lastQuickAdvantage = k.Joined, k.QuickResolved, k.LastQuickAdvantage
	b.endedReason, b.endedEnemiesDead, b.endedPlayerDead = k.EndedReason, k.EndedEnemiesDead, k.EndedPlayerDead
	b.endedDisengaged, b.endedRouted, b.endedDawn = k.EndedDisengaged, k.EndedRouted, k.EndedDawn
	b.quarryDead, b.xpSuppressed, b.released, b.reentrantReads = k.EndedQuarryDead, k.XPSuppressed, k.Released, k.ReentrantReads
	b.actionsRound, b.lastActions = k.ActionsRound, actionsOf(k.LastActions)

	// What no clock fight ever keeps between steps, and nothing reads of the
	// clock's (his round and pace rows, his HUD log, his verb): a load starts
	// them empty, as CreateGame's model does.
	b.blowLog, b.lastRound, b.lastPace = nil, RoundRow{}, PaceRow{}

	k.RNG.RestoreInto(b.rng)

	c.clockFights = fights
}

// actionSnapshots and actionsOf are one round's blows, out to the snapshot's
// form and back: every field the provider's actions rows report.
func actionSnapshots(in []action) []CombatActionSnapshot {
	if len(in) == 0 {
		return nil
	}

	out := make([]CombatActionSnapshot, 0, len(in))

	for _, a := range in {
		out = append(out, CombatActionSnapshot{
			Round: a.round, Attacker: a.attacker, Target: a.target, Roll: a.roll, Mod: a.mod, Score: a.score,
			Band: a.band, Base: a.base, Damage: a.damage, TargetHealthAfter: a.targetHealthAfter,
			TargetHasBody: a.targetHasBody, AdvantageWhy: a.advantageWhy, Reaction: a.reaction,
			BandRolled: a.bandRolled, Blocked: a.blocked, Absorbed: a.absorbed, Class: a.class, Weapon: a.weapon,
		})
	}

	return out
}

func actionsOf(in []CombatActionSnapshot) []action {
	out := make([]action, 0, len(in))

	for _, a := range in {
		out = append(out, action{
			round: a.Round, attacker: a.Attacker, target: a.Target, roll: a.Roll, mod: a.Mod, score: a.Score,
			band: a.Band, base: a.Base, damage: a.Damage, targetHealthAfter: a.TargetHealthAfter,
			targetHasBody: a.TargetHasBody, advantageWhy: a.AdvantageWhy, reaction: a.Reaction,
			bandRolled: a.BandRolled, blocked: a.Blocked, absorbed: a.Absorbed, class: a.Class, weapon: a.Weapon,
		})
	}

	return out
}
