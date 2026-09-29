package d2world

import (
	"errors"
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2rand"
)

// ErrCombatFighting is Snapshot's refusal while a fight is running (plan rule
// 2: "You cannot save during a fight"). The encounter -- who is in it, the
// frozen round, the open turn -- is never saved, so a snapshot taken during one
// could only drop it.
var ErrCombatFighting = errors.New("combat snapshot refused: a fight is running")

// CombatSnapshot is the combat model's saved state between fights (M4.6 B2a):
// the number the next encounter takes, where the dice stand, and every record
// a finished fight leaves behind -- the counters, how the last one ended, its
// round and pace rows, its last round's blows and the HUD's blow log. All of
// it is reported, so a resumed game that lost any of it would report a
// different world from the one that was saved.
//
// Never saved: the encounter (Snapshot refuses while there is one), and what a
// fight leaves for the game screen to take -- the experience it earned and the
// world minutes paced rounds are owed. The game takes both every frame, so a
// save finds them empty; Snapshot refuses if it does not, rather than drop
// them. The pace window (decision and wall seconds, commits, the health it
// opened on) is reset when a fight ends and is refused the same way.
type CombatSnapshot struct {
	NextID int                `json:"next_id"`
	RNG    d2rand.StreamState `json:"rng"`

	Started        int `json:"started"`
	Ended          int `json:"ended"`
	Rounds         int `json:"rounds"`
	Declines       int `json:"declines"`
	Actions        int `json:"actions"`
	CommitsRefused int `json:"commits_refused"`
	CommitsByInput int `json:"commits_by_input"`
	CommitsByField int `json:"commits_by_field"`
	StepsOrdered   int `json:"steps_ordered"`
	Joined         int `json:"joined"`
	QuickResolved  int `json:"quick_resolved"`

	LastQuickAdvantage float64 `json:"last_quick_advantage"`

	EndedReason      string `json:"ended_reason"`
	EndedEnemiesDead int    `json:"ended_enemies_dead"`
	EndedPlayerDead  int    `json:"ended_player_dead"`
	EndedDisengaged  int    `json:"ended_disengaged"`
	EndedRouted      int    `json:"ended_routed"`
	EndedDawn        int    `json:"ended_dawn"`

	// LastActionVerb is the Action the player took in the round a fight ended
	// on, when his own blow ended it: finishRound never ran to clear it, so
	// the next fight's first round row reads it unless he acts first.
	LastActionVerb string `json:"last_action_verb"`

	LastRound    CombatRoundSnapshot    `json:"last_round"`
	LastPace     CombatPaceSnapshot     `json:"last_pace"`
	ActionsRound int                    `json:"actions_round"`
	LastActions  []CombatActionSnapshot `json:"last_actions"`
	BlowLog      []CombatBlowSnapshot   `json:"blow_log"`
}

// CombatRoundSnapshot is RoundRow, the last closed round's record.
type CombatRoundSnapshot struct {
	Encounter     string  `json:"encounter"`
	Round         int     `json:"round"`
	DecideSeconds float64 `json:"decide_seconds"`
	Action        string  `json:"action"`
	Move          bool    `json:"move"`
}

// CombatPaceSnapshot is PaceRow, the last closed fight's record.
type CombatPaceSnapshot struct {
	Encounter     string   `json:"encounter"`
	Rounds        int      `json:"rounds"`
	WallSeconds   float64  `json:"wall_seconds"`
	DecideSeconds float64  `json:"decide_seconds"`
	HealthOpen    int      `json:"health_open"`
	HealthClose   int      `json:"health_close"`
	EndReason     string   `json:"end_reason"`
	Enemies       int      `json:"enemies"`
	Kinds         []string `json:"kinds"`
	Initiator     string   `json:"initiator"`
	Surprised     bool     `json:"surprised"`
	Control       string   `json:"control"`
	Commits       int      `json:"commits"`
}

// CombatActionSnapshot is one blow of the last resolved round, every field the
// provider's actions rows report.
type CombatActionSnapshot struct {
	Round             int    `json:"round"`
	Attacker          string `json:"attacker"`
	Target            string `json:"target"`
	Roll              int    `json:"roll"`
	Mod               int    `json:"mod"`
	Score             int    `json:"score"`
	Band              string `json:"band"`
	Base              int    `json:"base"`
	Damage            int    `json:"damage"`
	TargetHealthAfter int    `json:"target_health_after"`
	TargetHasBody     bool   `json:"target_has_body"`
	AdvantageWhy      string `json:"advantage_why"`
	Reaction          string `json:"reaction"`
	BandRolled        string `json:"band_rolled"`
	Blocked           bool   `json:"blocked"`
	Absorbed          int    `json:"absorbed"`
	Class             string `json:"class"`
	Weapon            string `json:"weapon"`
}

// CombatBlowSnapshot is one line of the HUD's blow log (BlowLine).
type CombatBlowSnapshot struct {
	Round    int    `json:"round"`
	Attacker string `json:"attacker"`
	Target   string `json:"target"`
	Band     string `json:"band"`
	Damage   int    `json:"damage"`
	Reaction string `json:"reaction"`
	Killed   bool   `json:"killed"`
}

// Snapshot is the combat model between fights. It returns ErrCombatFighting
// during one, and an error naming the field if anything a fight leaves for
// the game screen, or its pace window, has not been emptied.
func (c *Combat) Snapshot() (CombatSnapshot, error) {
	if c.Fighting() {
		return CombatSnapshot{}, ErrCombatFighting
	}

	if err := c.checkBetweenFights(); err != nil {
		return CombatSnapshot{}, err
	}

	s := CombatSnapshot{
		NextID: c.nextID, RNG: d2rand.StateOf(c.rng),
		Started: c.started, Ended: c.ended, Rounds: c.rounds, Declines: c.declines, Actions: c.actions,
		CommitsRefused: c.commitsRefused, CommitsByInput: c.commitsByInput, CommitsByField: c.commitsByField,
		StepsOrdered: c.stepsOrdered, Joined: c.joined, QuickResolved: c.quickResolved,
		LastQuickAdvantage: c.lastQuickAdvantage,
		EndedReason:        c.endedReason, EndedEnemiesDead: c.endedEnemiesDead, EndedPlayerDead: c.endedPlayerDead,
		EndedDisengaged: c.endedDisengaged, EndedRouted: c.endedRouted, EndedDawn: c.endedDawn,
		LastActionVerb: c.lastActionVerb,
		LastRound: CombatRoundSnapshot{
			Encounter: c.lastRound.Encounter, Round: c.lastRound.Round, DecideSeconds: c.lastRound.DecideSeconds,
			Action: c.lastRound.Action, Move: c.lastRound.Move,
		},
		LastPace: CombatPaceSnapshot{
			Encounter: c.lastPace.Encounter, Rounds: c.lastPace.Rounds, WallSeconds: c.lastPace.WallSeconds,
			DecideSeconds: c.lastPace.DecideSeconds, HealthOpen: c.lastPace.HealthOpen,
			HealthClose: c.lastPace.HealthClose, EndReason: c.lastPace.EndReason, Enemies: c.lastPace.Enemies,
			Kinds: b2aStrings(c.lastPace.Kinds), Initiator: c.lastPace.Initiator,
			Surprised: c.lastPace.Surprised, Control: c.lastPace.Control, Commits: c.lastPace.Commits,
		},
		ActionsRound: c.actionsRound,
		LastActions:  make([]CombatActionSnapshot, 0, len(c.lastActions)),
		BlowLog:      make([]CombatBlowSnapshot, 0, len(c.blowLog)),
	}

	for _, a := range c.lastActions {
		s.LastActions = append(s.LastActions, CombatActionSnapshot{
			Round: a.round, Attacker: a.attacker, Target: a.target, Roll: a.roll, Mod: a.mod, Score: a.score,
			Band: a.band, Base: a.base, Damage: a.damage, TargetHealthAfter: a.targetHealthAfter,
			TargetHasBody: a.targetHasBody, AdvantageWhy: a.advantageWhy, Reaction: a.reaction,
			BandRolled: a.bandRolled, Blocked: a.blocked, Absorbed: a.absorbed, Class: a.class, Weapon: a.weapon,
		})
	}

	for _, b := range c.blowLog {
		s.BlowLog = append(s.BlowLog, CombatBlowSnapshot{
			Round: b.Round, Attacker: b.Attacker, Target: b.Target, Band: b.Band,
			Damage: b.Damage, Reaction: b.Reaction, Killed: b.Killed,
		})
	}

	return s, nil
}

// checkBetweenFights refuses a snapshot that would drop something: what a
// fight leaves for the game screen to take, and the pace window end() resets.
func (c *Combat) checkBetweenFights() error {
	switch {
	case len(c.xpEvents) != 0:
		return fmt.Errorf("combat snapshot refused: %d experience event(s) not yet taken (TakeXPEvents)", len(c.xpEvents))
	case c.owedMinutes != 0:
		return fmt.Errorf("combat snapshot refused: %v world minute(s) owed by paced rounds not yet taken (TakeRoundMinutes)",
			c.owedMinutes)
	case c.killerIsPlayer:
		return fmt.Errorf("combat snapshot refused: a blow is mid-resolution (killerIsPlayer)")
	case c.paceOpen || c.wallSeconds != 0 || c.decisionSeconds != 0 || c.decisionSecondsRound != 0 ||
		c.paceCommits != 0 || c.paceHealthOpen != 0:
		return fmt.Errorf("combat snapshot refused: the pace window is open with no fight (open %v, wall %v, "+
			"decide %v/%v, commits %d, health %d)", c.paceOpen, c.wallSeconds, c.decisionSeconds,
			c.decisionSecondsRound, c.paceCommits, c.paceHealthOpen)
	}

	return nil
}

// Restore puts the saved combat model back between fights, stream included.
// worldSeed is the saved game's seed: the stream must be the one that game
// ran combat on (d2rand.StreamState.Check). It is checked whole first
// (Validate), so a refused snapshot changes nothing.
//
// The dials are not restored: the game builds the model with its shipped
// dials, and the harness's writes to them are test setup. Load order (B4):
// with the other systems' counters, after the entities are rebuilt; nothing it
// restores names a live entity except as a record (the last round's blows).
func (c *Combat) Restore(s CombatSnapshot, worldSeed int64) error {
	if err := c.Validate(s, worldSeed); err != nil {
		return err
	}

	c.nextID = s.NextID
	c.started, c.ended, c.rounds, c.declines, c.actions = s.Started, s.Ended, s.Rounds, s.Declines, s.Actions
	c.commitsRefused, c.commitsByInput, c.commitsByField = s.CommitsRefused, s.CommitsByInput, s.CommitsByField
	c.stepsOrdered, c.joined, c.quickResolved = s.StepsOrdered, s.Joined, s.QuickResolved
	c.lastQuickAdvantage = s.LastQuickAdvantage
	c.endedReason, c.endedEnemiesDead, c.endedPlayerDead = s.EndedReason, s.EndedEnemiesDead, s.EndedPlayerDead
	c.endedDisengaged, c.endedRouted, c.endedDawn = s.EndedDisengaged, s.EndedRouted, s.EndedDawn
	c.lastActionVerb = s.LastActionVerb

	r := s.LastRound
	c.lastRound = RoundRow{Encounter: r.Encounter, Round: r.Round, DecideSeconds: r.DecideSeconds, Action: r.Action, Move: r.Move}

	p := s.LastPace
	c.lastPace = PaceRow{
		Encounter: p.Encounter, Rounds: p.Rounds, WallSeconds: p.WallSeconds, DecideSeconds: p.DecideSeconds,
		HealthOpen: p.HealthOpen, HealthClose: p.HealthClose, EndReason: p.EndReason, Enemies: p.Enemies,
		Kinds: b2aStrings(p.Kinds), Initiator: p.Initiator, Surprised: p.Surprised,
		Control: p.Control, Commits: p.Commits,
	}

	c.actionsRound = s.ActionsRound
	c.lastActions = make([]action, 0, len(s.LastActions))

	for _, a := range s.LastActions {
		c.lastActions = append(c.lastActions, action{
			round: a.Round, attacker: a.Attacker, target: a.Target, roll: a.Roll, mod: a.Mod, score: a.Score,
			band: a.Band, base: a.Base, damage: a.Damage, targetHealthAfter: a.TargetHealthAfter,
			targetHasBody: a.TargetHasBody, advantageWhy: a.AdvantageWhy, reaction: a.Reaction,
			bandRolled: a.BandRolled, blocked: a.Blocked, absorbed: a.Absorbed, class: a.Class, weapon: a.Weapon,
		})
	}

	c.blowLog = make([]BlowLine, 0, len(s.BlowLog))
	for _, b := range s.BlowLog {
		c.blowLog = append(c.blowLog, BlowLine{
			Round: b.Round, Attacker: b.Attacker, Target: b.Target, Band: b.Band,
			Damage: b.Damage, Reaction: b.Reaction, Killed: b.Killed,
		})
	}

	s.RNG.RestoreInto(c.rng)

	return nil
}

// Validate is Restore's check and nothing else (D4). It refuses during a
// fight -- a fight is never restored into -- and into a model holding what a
// fight leaves behind for the game screen, or an open pace window: the
// snapshot never carries those, so a restore over them would leave a resumed
// game owing experience or minutes that belong to no saved moment (a fresh
// model, CreateGame's, holds none). Then the snapshot itself: counts, the ids
// tied to them, and the stream -- on the seed a game seeded worldSeed runs
// combat on, at no more than d2rand.MaxDraws, so the rising's block swapped in
// is refused (the B2a review's B1).
func (c *Combat) Validate(s CombatSnapshot, worldSeed int64) error {
	if c.Fighting() {
		return fmt.Errorf("combat snapshot: restore refused during a fight")
	}

	if err := c.checkBetweenFights(); err != nil {
		return fmt.Errorf("combat snapshot: restore refused: %w", err)
	}

	if err := checkCombatSnapshot(s); err != nil {
		return err
	}

	if err := s.RNG.Check(worldSeed, d2rand.StreamCombat); err != nil {
		return fmt.Errorf("combat snapshot: %w", err)
	}

	return nil
}

// b2aStrings copies a list and KEEPS A NIL ONE NIL. The provider writes the
// pace row's kinds as they are, so a game that has closed no paced fight
// reports "kinds": null, and one whose last fight had no rowed enemy reports
// []: a copy that turned either into the other would report a different world
// after a load. JSON carries the difference (null and []), so this does too.
func b2aStrings(in []string) []string {
	if in == nil {
		return nil
	}

	return append([]string{}, in...)
}

func checkCombatSnapshot(s CombatSnapshot) error {
	counts := map[string]int{
		"started": s.Started, "ended": s.Ended, "rounds": s.Rounds, "declines": s.Declines, "actions": s.Actions,
		"commits_refused": s.CommitsRefused, "commits_by_input": s.CommitsByInput, "commits_by_field": s.CommitsByField,
		"steps_ordered": s.StepsOrdered, "joined": s.Joined, "quick_resolved": s.QuickResolved,
		"ended_enemies_dead": s.EndedEnemiesDead, "ended_player_dead": s.EndedPlayerDead,
		"ended_disengaged": s.EndedDisengaged, "ended_routed": s.EndedRouted, "ended_dawn": s.EndedDawn,
		"actions_round": s.ActionsRound,
	}

	for name, n := range counts {
		if n < 0 {
			return fmt.Errorf("combat snapshot: %s is %d", name, n)
		}
	}

	switch {
	case s.NextID != s.Started+1:
		// tryStart spends one number per fight and nothing else does, so the
		// next id is always one past the fights started.
		return fmt.Errorf("combat snapshot: next_id %d after %d fight(s) started; it is always one past", s.NextID, s.Started)
	case s.Ended != s.Started:
		return fmt.Errorf("combat snapshot: %d fight(s) started and %d ended; between fights every one has ended",
			s.Started, s.Ended)
	case !b2aFinite(s.LastQuickAdvantage, s.LastRound.DecideSeconds, s.LastPace.WallSeconds, s.LastPace.DecideSeconds):
		return fmt.Errorf("combat snapshot: a record holds a number that is not one")
	}

	return nil
}
