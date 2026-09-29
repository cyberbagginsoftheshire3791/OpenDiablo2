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
	case math.IsNaN(f.SinceTurn) || math.IsInf(f.SinceTurn, 0) || f.SinceTurn < 0:
		return fmt.Errorf("since_turn %v", f.SinceTurn)
	case f.Initiator != "enemy":
		return fmt.Errorf("initiator %q: a clock fight opens on its enemies' notice", f.Initiator)
	case !f.Surprised && f.SurpriseWhy != "":
		return fmt.Errorf("surprise_why %q in a fight that was not a surprise", f.SurpriseWhy)
	}

	// Each list is a set of entity ids. The order and the three sets may name
	// ids no longer among the enemies -- the dead that broke off at first light
	// leave the rows (pruneOrEnd), and a Downed man who stood again leaves his
	// old id behind (Rejoin) -- so only the enemies must resolve at a load.
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

// clockFightsOf builds the saved clock fights as live encounters, resolving
// each quarry and enemy through the Resolver the game attached. It is
// Validate's last check and Restore's build: one code for both, so what
// Validate accepts is exactly what Restore makes.
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

		for _, id := range f.Enemies {
			w, err := b2bResolveWatcher(c.resolver, id)
			if err != nil {
				return nil, fmt.Errorf("combat snapshot: clock.live[%d] %s: %w", i, f.ID, err)
			}

			e.enemies = append(e.enemies, w)
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

		out = append(out, e)
	}

	return out, nil
}

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
