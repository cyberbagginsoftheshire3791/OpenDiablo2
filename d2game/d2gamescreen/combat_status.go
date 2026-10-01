package d2gamescreen

import (
	"fmt"
	"strings"
)

// THE COMBAT STATUS (Josh, 30 Sep 2026). "I think we should have a combat
// status for the player, and you can't save while in combat." And on the
// questions that followed, the recommended option each time: what puts him in
// combat is "Fight, action, or chased", and it shows as a "Marker by the
// health globe". His ruling of 29 Sep stands behind it: "most games don't let
// you save mid combat. I think you've found out why." So the save REFUSES in
// combat, rather than carrying what a fight leaves half-done.
//
// HE IS IN COMBAT while any of these holds (combatTrigger, in this order):
//
//   - HIS FIGHT IS LIVE (CombatFight): Combat.Fighting, his own encounter
//     only. An open turn (Combat.Awaiting) is his fight's by construction
//     (both read Combat.his), and a fight he is not in -- the raid's clock
//     fights, the village's -- never makes it true by itself (the raid's R1).
//   - A HOSTILE IS CHASING HIM (CombatChased): Pursuit.ChasersOf names him. In
//     the game only a monster ever chases him: every chase of the game's own
//     is started from the notice model's watches (startChasesForTheAware), and
//     only the spawn tables watch him (Spawns, every member of every group
//     they place). A chase lives from Chase to Release whatever its route
//     does, so a re-path does not flicker it; a paced fight releases its
//     members' chases while it holds them (holdOne), and the fight is live.
//     "Chased" LATCHES (Josh, 30 Sep: it must hold for a few seconds; the R2
//     review's B1: with a second quarry near, a chase on him can drop for
//     0.4-0.8 s and come back): the grace below is the latch -- a chase
//     that drops and returns inside it never takes him out of combat
//     (TestAChaseThatFlickersStaysCombat).
//   - HIS OWN SWING (CombatSwing, Player.IsCasting) or HIS HIT OR BLOCK
//     REACTION (CombatReaction, Player.Reacting) IS PLAYING. Neither is in the
//     world file: a load stands him still (rule 4). The reaction is BUG-105's
//     case -- a blow on him as his fight ended -- and is closed by this.
//
// AND FOR A GRACE PERIOD AFTER THE LAST OF THESE ENDS: combatGraceSeconds of
// game time [DIAL] -- DefaultCombatGraceSeconds shipped, the harness's
// save.combat_grace. It is counted down at the end of every frame the screen
// is live (the escape menu pauses it as it pauses the world), on the frame's
// own seconds, and held full on every frame a trigger is seen.
//
// WHAT IT DOES:
//
//   - A save is refused COMBAT ("You can't save in combat."): the menu's SAVE
//     GAME and SAVE AND EXIT GAME (the exit entry reads EXIT WITHOUT SAVING, as
//     for every refusal), the dawn autosave (pending: every refusal is "not
//     now" to it, and it is taken on the first frame out of combat, that day),
//     and the close hook (it waits out only the grace's remainder, and leaves
//     without saving while his fight is live or a hostile chases him:
//     settleForClose). COMBAT comes right after DEAD in the save's order, and
//     it took over FIGHTING's two clauses that were the same thing -- his
//     fight running and his last swing playing (save.go, fightUnsettled).
//   - The HUD marks it by the health globe (d2player's combat marker), from
//     InCombat, while he cannot save for it.
//
// NOTHING OF IT IS IN THE WORLD FILE, and nothing needs to be: a save is never
// made in combat, so at every save the grace is 0 and every trigger clear
// (TestAnAcceptedSaveIsOutOfCombat) -- a resumed game starts out of combat, as
// the saved one was.

// Why he is in combat: the harness's combat_reason and the refusal's reason.
const (
	CombatFight    = "fight"    // his fight is live
	CombatChased   = "chased"   // a hostile is chasing him
	CombatSwing    = "swing"    // his own swing is playing
	CombatReaction = "reaction" // his hit or block reaction is playing
	CombatGrace    = "grace"    // none of those: the grace after the last is running
)

// DefaultCombatGraceSeconds is how long he stays in combat after the last of
// his fight, a chase, his swing and his reaction ends, in seconds of game time
// [DIAL] (Josh's brief, 30 Sep 2026: "clears a few seconds after"; 3 s). The
// harness's save.combat_grace sets it for a script.
//
// MEASURED BEFORE IT WAS SET (strigoi-measure-first; the notes' "The combat
// status"): a chase does not flicker as its hunter re-paths -- the chase stays
// in Pursuit from Chase to Release -- but inside a paced fight the members'
// chases are released while the fight holds them and taken again after, and
// the fight is live throughout; his swings and reactions come and go in
// fractions of a second inside the fight and just after it. Three seconds
// covers the moment after a fight (the fight's end applied, his last swing,
// a blow he took as it ended) with room over the longest of those measured.
const DefaultCombatGraceSeconds = 3.0

// combatGraceFloor ends a grace that frame slices have all but spent: a
// counted-down float never lands on 0 exactly (sixty slices of 1/60 do not sum
// to 1), and a grace of 1e-15 s would keep him in combat a frame too long.
const combatGraceFloor = 1e-9

// combatTrigger is what puts him in combat now, and a word on it for the log
// and the harness: the code (CombatFight, CombatChased, CombatSwing,
// CombatReaction), or "" when none does. His fight first, then a chase: the
// two a moment does not cure (settleForClose).
func (v *Game) combatTrigger() (code, detail string) {
	if v.combat != nil && v.combat.Fighting() {
		return CombatFight, "his fight " + v.combat.Encounter() + " is live"
	}

	p := v.localPlayer
	if p == nil {
		return "", ""
	}

	if v.pursuit != nil {
		if hunters := v.pursuit.ChasersOf(p.ID()); len(hunters) > 0 {
			return CombatChased, strings.Join(hunters, ", ") + " chasing him"
		}
	}

	switch {
	case p.IsCasting():
		return CombatSwing, "his swing is playing"
	case p.Reacting():
		return CombatReaction, "his hit or block reaction is playing"
	}

	return "", ""
}

// CombatReason is why he is in combat -- a trigger now, or CombatGrace while
// only the grace after the last one runs -- and a word on it; "" when he is
// not in combat.
func (v *Game) CombatReason() (code, detail string) {
	if code, detail = v.combatTrigger(); code != "" {
		return code, detail
	}

	if v.combatGrace > 0 {
		return CombatGrace, fmt.Sprintf("%.2f s left of the grace after %s", v.combatGrace, v.combatLast)
	}

	return "", ""
}

// InCombat is the combat status: he cannot save, and the HUD marks it
// (d2player.CombatHolder).
func (v *Game) InCombat() bool {
	code, _ := v.CombatReason()

	return code != ""
}

// advanceCombatStatus is the end of every frame (Advance): a trigger seen
// holds the grace full; none counts it down on a frame the screen is live, and
// at 0 he is out of combat. The edges are logged, one line each way, so a
// friend's log says how often and how long he could not save.
func (v *Game) advanceCombatStatus(elapsed float64) {
	if code, detail := v.combatTrigger(); code != "" {
		if v.combatLast == "" && v.combatGrace <= 0 {
			v.Infof("COMBAT in (%s): %s", code, detail)
		}

		v.combatLast = detail
		v.combatGrace = v.combatGraceSeconds

		return
	}

	if v.combatGrace > 0 && v.combatGraceRuns() {
		if v.combatGrace -= elapsed; v.combatGrace <= combatGraceFloor {
			v.combatGrace = 0
		}
	}

	if v.combatGrace <= 0 && v.combatLast != "" {
		v.Infof("COMBAT out: %.1f s after %s", v.combatGraceSeconds, v.combatLast)
		v.combatLast = ""
	}
}

// combatGraceRuns is whether this frame counts toward the grace: every frame
// but those under the escape menu of a single-player game, which pauses the
// world (screenLive's rule; a network game's menu pauses nothing).
func (v *Game) combatGraceRuns() bool {
	if v.escapeMenu == nil || !v.escapeMenu.IsOpen() {
		return true
	}

	return v.gameClient != nil && len(v.gameClient.Players) != 1
}

// settlesForClose is whether the close hook's settle may run frames to cure
// this refusal (the B5 review's A1; the combat status, rule 3 reworded): the
// moment after combat -- COMBAT for the grace, his swing or his reaction, each
// of which ends on its own within seconds -- or FIGHTING, the fight's own
// bookkeeping not yet settled. NEVER his live fight or a chase: being chased
// does not end on its own, and a close in combat leaves without saving.
func (v *Game) settlesForClose(r *SaveRefusal) bool {
	switch r.Code {
	case SaveRefusedFighting:
		return true
	case SaveRefusedCombat:
		code, _ := v.CombatReason()

		return code == CombatGrace || code == CombatSwing || code == CombatReaction
	}

	return false
}
