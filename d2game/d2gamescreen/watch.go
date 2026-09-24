package d2gamescreen

import (
	"fmt"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// T8, the watch (23 Sep 2026). S1 §8.2 moves the village's number for
// "standing the palisade watch". T4 paid a promised watch for being ALIVE at
// dawn, wherever he had spent the night. Now the watch is STOOD: the minutes of
// night he spends within the dialogue table's watch_radius of the headman are
// counted, he stands in the WATCH stance while he does (the meters' "watch":
// vigilant -- never caught head-down -- and tiring), and dawn judges the count
// against watch_minutes. A promise broken costs standing.

// watchSpeaker is the villager whose post the watch is kept at.
const watchSpeaker = "headman"

// headmanEntity finds the headman's stand-in sprite on the map. Looked up each
// frame rather than cached: a cached sprite outlives a map rebuild (review
// finding), and the entity list is short.
func (v *Game) headmanEntity() d2interface.MapEntity {
	if v.dialogue == nil || v.gameClient == nil || v.gameClient.MapEngine == nil {
		return nil
	}

	sp := v.dialogue.Speaker(watchSpeaker)
	if sp == nil {
		return nil
	}

	for _, e := range v.gameClient.MapEngine.Entities() {
		if d2mapentity.NameKey(e) == sp.StandIn {
			return e
		}
	}

	return nil
}

// atWatchPost reports him within the watch radius of the headman.
func (v *Game) atWatchPost() bool {
	post := v.headmanEntity()
	if post == nil || v.localPlayer == nil || v.dialogue == nil {
		return false
	}

	px, py := v.localPlayer.GetPositionF()
	hx, hy := post.GetPositionF()

	return math.Hypot(px-hx, py-hy) <= v.dialogue.Village.WatchRadius
}

// keepWatch runs once a frame: while a watch is promised, it is night and he
// is at the post, the minutes count and he stands the watch.
func (v *Game) keepWatch() {
	if v.worldClock == nil || v.standing == nil || v.meters == nil {
		return
	}

	now := v.worldClock.WorldMinutes()
	delta := now - v.watchClock
	v.watchClock = now

	// The first sample of a session counts nothing: the clock does not start
	// at zero, and counting from zero credited a whole night on the first
	// frame after a reload at the post (review finding, SEVERE).
	if !v.watchClockSet {
		v.watchClockSet, delta = true, 0
	}

	// A jump -- an hour's labour, a forage -- is judged at its end, so it is
	// credited at most watchJumpMinutes: time at work is not time on watch.
	delta = math.Min(delta, watchJumpMinutes)

	// By full day a night's minutes are spent, promise kept or not: a missed
	// dawn must not carry them into the next night.
	if v.worldClock.Stage() == d2world.StageDay {
		v.watchStood = 0
	}

	// Minutes fighting AT the post count: driving off what comes is the watch.
	on := v.standing.Has(d2dialogue.FlagWatch) && v.night() && v.alive() && v.atWatchPost()

	if on && delta > 0 {
		v.watchStood += delta
	}

	// The stance follows the post, and only takes over from idle -- labour,
	// forage and a fight's own stance are theirs to keep.
	switch act := v.meters.Activity(); {
	case on && act == d2world.ActivityIdle:
		v.meters.SetActivity(d2world.ActivityWatch)
	case !on && act == d2world.ActivityWatch:
		v.meters.SetActivity(d2world.ActivityIdle)
	}
}

// dawnWatch settles the night at dawn: the watch stood or broken, and the
// byre his to ask for again.
func (v *Game) dawnWatch() {
	if v.dialogue == nil || v.standing == nil {
		return
	}

	promised := v.standing.Has(d2dialogue.FlagWatch)
	kept := v.watchStood >= v.dialogue.Village.WatchMinutes
	moved := v.dialogue.DawnWatch(v.standing, v.watchStood)
	v.watchStood = 0

	if !promised {
		return
	}

	// Worded by kept-or-broken, not by the sign of the move: at the floor a
	// broken watch moves nothing and must not read as kept (review finding).
	if v.gameControls != nil {
		text := fmt.Sprintf(d2player.TalkWatchKept, moved)
		if !kept {
			text = fmt.Sprintf(d2player.TalkWatchBroken, -moved)
		}

		v.gameControls.SetZoneChangeText(text)
		v.gameControls.ShowZoneChangeText()
		v.gameControls.HideZoneChangeTextAfter(levelNoticeSeconds)
	}

	v.saveKit()
}

// watchJumpMinutes caps what one frame can credit to the watch [DIAL].
const watchJumpMinutes = 5.0
