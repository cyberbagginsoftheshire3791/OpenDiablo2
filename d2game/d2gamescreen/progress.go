package d2gamescreen

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2progress"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// T3, progression (23 Sep 2026): the game screen owns the hero's experience
// and talents, kept in his sidecar beside his kit, and applies them to the
// systems that already run -- his body (the meters, his maximum health), his
// torch (the light), the beasts' eyes (the notice radius), and the resolver
// (d2world.Edges). Experience comes from what fights did (combat's XPEvents)
// and from living to see dawn. See d2core/d2progress for why every number is
// invented and where it lives.

// talentsPath is the shipped progression table.
const talentsPath = "/data/strigoi/talents.json"

// levelNoticeSeconds is how long a level-up stays on screen.
const levelNoticeSeconds = 4.0

// bindProgress reads his progress from the sidecar's raw block (nil: none yet)
// and applies it. Called from bindKit, with the kit's own read.
func (v *Game) bindProgress(raw json.RawMessage) {
	v.progress = &d2progress.Progress{}

	if len(raw) > 0 {
		if err := json.Unmarshal(raw, v.progress); err != nil {
			v.Errorf("progress: %v -- starting him fresh", err)

			v.progress = &d2progress.Progress{}
		}
	}

	// His maximum health before talents, recorded once. His .od2 stores the
	// maximum WITH talents, so recomputing from this -- never adding to the
	// stored value -- is what stops the bonus compounding on every load.
	if v.progress.BaseMaxHealth <= 0 && v.localPlayer != nil && v.localPlayer.Stats != nil {
		v.progress.BaseMaxHealth = v.localPlayer.Stats.MaxHealth
	}

	// THE STAGE HE ARRIVED IN, so the first frame is not read as a night just
	// survived. StageNight is the zero value and every session starts at the
	// dawn epoch (02:45), so an uninitialised lastStage paid 50 experience on
	// every load -- a free level per reload (review finding; the playtest's
	// fresh-hero check caught it too).
	if v.worldClock != nil {
		v.lastStage = v.worldClock.Stage()
		v.dawnPaidDay = v.worldClock.DayIndex()
	}

	v.applyProgress(false)

	d2harness.Register(progressProvider{v})
}

// applyProgress puts his talents into the systems they change. picked is true
// only when a talent was just taken: a raised maximum then raises his health
// with it. On load it does not -- his .od2 may predate the pick (it is saved
// only on leaving), and adding the gain there healed him for free after a
// crash (review finding).
func (v *Game) applyProgress(picked bool) {
	if v.progress == nil || v.talents == nil {
		return
	}

	p, t := v.progress, v.talents

	if v.meters != nil {
		v.meters.SetConditioning(d2world.Conditioning{
			FatigueRate:       p.Effect(t, d2progress.FatigueRate),
			FoodWaterRate:     p.Effect(t, d2progress.FoodWaterRate),
			ShakenFatigue:     p.Effect(t, d2progress.ShakenFatigue),
			NoReactionFatigue: p.Effect(t, d2progress.NoReactionFatigue),
		})
	}

	if v.light != nil {
		v.light.SetCarriedBurnRate(p.Effect(t, d2progress.TorchBurnRate))
	}

	// Quiet Step moves the notice radius BY ITS DELTA, from whatever the radius
	// is now -- so a pick of any other talent leaves a radius a script (or a
	// later system) set exactly where it was (review finding). There is one
	// hero in v0, so the radius beasts notice HIM at is the radius.
	if v.notice != nil {
		delta := p.Effect(t, d2progress.NoticeRadius)
		if delta != v.noticeDelta {
			v.notice.SetRadius(v.notice.Dials().Radius - v.noticeDelta + delta)
			v.noticeDelta = delta
		}
	}

	if v.localPlayer != nil && v.localPlayer.Stats != nil && p.BaseMaxHealth > 0 {
		stats := v.localPlayer.Stats
		want := p.BaseMaxHealth + int(p.Effect(t, d2progress.MaxHealth))

		// A raised maximum raises his health by the same amount -- a new
		// talent is felt at once -- and a lowered one never leaves him above it.
		if gain := want - stats.MaxHealth; picked && gain > 0 && stats.Health > 0 {
			stats.Health += gain
		}

		stats.MaxHealth = want
		if stats.Health > want {
			stats.Health = want
		}
	}
}

// EdgeOf is the d2world.Edges seam: his talents as the resolver needs them.
// Fire and Iron counts only while his torch actually burns.
func (v *Game) EdgeOf(id string) d2world.Edge {
	if v.progress == nil || v.talents == nil || v.localPlayer == nil || id != v.localPlayer.ID() {
		return d2world.Edge{}
	}

	p, t := v.progress, v.talents

	e := d2world.Edge{
		RiposteDamage:  p.Effect(t, d2progress.RiposteDamage),
		ExtraBlocks:    int(p.Effect(t, d2progress.BlocksPerRound)),
		ExtraMove:      int(p.Effect(t, d2progress.MoveTiles)),
		CritBand:       int(p.Effect(t, d2progress.CritBand)),
		ExtraReactions: int(p.Effect(t, d2progress.Reactions)),
		AdvantageBonus: int(p.Effect(t, d2progress.AdvantageBonus)),
		KillNerve:      p.Effect(t, d2progress.KillNerve),
	}

	if v.torchLit() {
		e.LitNerve = p.Effect(t, d2progress.LitNerve)
	}

	return e
}

// earnExperience runs once per live frame: what the fights did, and dawn.
func (v *Game) earnExperience() {
	if v.progress == nil || v.talents == nil || v.combat == nil {
		return
	}

	xp := 0

	for _, ev := range v.combat.TakeXPEvents() {
		switch ev.Kind {
		case "slain":
			xp += v.talents.SlainXP(ev.Row)

			v.note("slain")
			if ev.Row != "" {
				v.note("slain:" + ev.Row)
			}
		case "routed":
			xp += v.talents.RoutedXP(ev.Row)

			v.note("routed")
		}
	}

	// A night lived through: the stage turning from night to dawn with him on
	// his feet -- once per day, whatever the stage does at the boundary. The
	// night is the enemy; seeing it off is worth something.
	if v.worldClock != nil {
		stage, day := v.worldClock.Stage(), v.worldClock.DayIndex()
		if nightSurvived(v.lastStage, stage, day, v.dawnPaidDay, v.alive()) {
			xp += v.talents.XP.Night
			v.dawnPaidDay = day
			v.note("night_survived")

			// T4: a watch he promised the village, kept.
			v.dawnWatch()
		}

		v.lastStage = stage
	}

	// T8: a promised watch counts the minutes stood at the post.
	v.keepWatch()

	v.gainXP(xp)
}

// gainXP adds experience, announces a level, and saves.
func (v *Game) gainXP(xp int) {
	if xp <= 0 {
		return
	}

	if up := v.progress.Gain(v.talents, xp); up > 0 && v.gameControls != nil {
		v.gameControls.SetZoneChangeText(fmt.Sprintf(d2player.ProgressLevelUp, v.progress.Level(v.talents)))
		v.gameControls.ShowZoneChangeText()
		v.gameControls.HideZoneChangeTextAfter(levelNoticeSeconds)
	}

	v.saveKit()
}

func (v *Game) alive() bool {
	return v.localPlayer != nil && v.localPlayer.Stats != nil && v.localPlayer.Stats.Health > 0
}

// progressJSON is the block the sidecar carries.
func (v *Game) progressJSON() json.RawMessage {
	if v.progress == nil {
		return nil
	}

	data, err := json.Marshal(v.progress)
	if err != nil {
		return nil
	}

	return data
}

// --- d2player.ProgressHolder ------------------------------------------------

// Progress is his standing as the talent panel draws it.
func (v *Game) Progress() (*d2progress.Progress, *d2progress.Tree) { return v.progress, v.talents }

// PickTalent takes a talent -- for good; there is no respec (U1 §6.1).
func (v *Game) PickTalent(id string) error {
	if v.progress == nil || v.talents == nil {
		return nil
	}

	if err := v.progress.Pick(v.talents, id); err != nil {
		return err
	}

	v.applyProgress(true)
	v.saveKit()

	return nil
}

// --- the "progress" harness provider ---------------------------------------

// progressProvider reports his standing. It is its own small type so the
// Game does not become a provider for everything.
type progressProvider struct{ v *Game }

func (p progressProvider) HarnessName() string { return "progress" }

func (p progressProvider) HarnessState() map[string]interface{} {
	v := p.v
	state := map[string]interface{}{"bound": v.progress != nil}

	if v.progress == nil || v.talents == nil {
		return state
	}

	talents := append([]string{}, v.progress.Talents...)
	sort.Strings(talents)

	edge := d2world.Edge{}
	if v.localPlayer != nil {
		edge = v.EdgeOf(v.localPlayer.ID())
	}

	state["xp"] = v.progress.XP
	state["level"] = v.progress.Level(v.talents)
	state["next_at"] = v.talents.NextAt(v.progress.XP)
	state["picks_waiting"] = v.progress.PicksWaiting(v.talents)
	state["talents"] = talents
	state["base_max_health"] = v.progress.BaseMaxHealth
	state["edge"] = map[string]interface{}{
		"riposte_damage": edge.RiposteDamage, "extra_blocks": edge.ExtraBlocks,
		"extra_move": edge.ExtraMove, "crit_band": edge.CritBand,
		"extra_reactions": edge.ExtraReactions, "advantage_bonus": edge.AdvantageBonus,
		"kill_nerve": edge.KillNerve, "lit_nerve": edge.LitNerve,
	}

	return state
}

// HarnessSettableFields: grant_xp is the ONE arranging verb, and it is named
// for what it is. A script that proves the earning path kills something and
// reads the XP rise; a script that proves the talent panel needs a level it
// would otherwise spend minutes of fights reaching.
func (p progressProvider) HarnessSettableFields() []string { return []string{"grant_xp"} }

func (p progressProvider) HarnessSet(field string, value interface{}) error {
	if field != "grant_xp" {
		return fmt.Errorf("progress has no settable field %q", field)
	}

	n, ok := value.(float64)
	if !ok || n <= 0 {
		return fmt.Errorf("grant_xp wants a positive number, got %v", value)
	}

	if p.v.progress == nil {
		return fmt.Errorf("no hero is bound yet")
	}

	p.v.gainXP(int(n))

	return nil
}

// nightSurvived is the night-experience rule: the stage turning from night to
// dawn, on a day whose dawn has not already paid, with him alive.
func nightSurvived(last, now d2world.Stage, day, paidDay int, alive bool) bool {
	return alive && last == d2world.StageNight && now == d2world.StageDawn && day != paidDay
}
